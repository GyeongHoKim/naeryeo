package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	// appDirName is the per-user directory this project owns under
	// os.UserConfigDir().
	appDirName = "naeryeo"
	// settingsFileName holds the non-secret settings. JSON keeps this to the
	// standard library: YAML or TOML would add a dependency for a file the
	// user is not expected to hand-edit — `naeryeo setup` writes it.
	settingsFileName = "config.json"
)

// RoutingProvider identifies which engine answers route searches.
type RoutingProvider string

const (
	// ProviderUnset means no usable provider is configured. It is the value
	// a missing, unreadable, or incomplete settings file resolves to, and it
	// is what the CLI turns into a "run setup first" failure. Storing a
	// credential does NOT imply a provider — the choice is always explicit.
	ProviderUnset RoutingProvider = ""
	// ProviderMotis is a self-hosted MOTIS instance. It needs a base URL and
	// no credential at all.
	ProviderMotis RoutingProvider = "motis"
	// ProviderODsay is the commercial ODsay API. It needs an API key from the
	// OS keychain and no URL.
	ProviderODsay RoutingProvider = "odsay"
)

// GeocoderChoice identifies which place-search backend resolves building
// names and addresses that the routing provider itself cannot. It is an axis
// independent of RoutingProvider: all four combinations are valid.
type GeocoderChoice string

const (
	// GeocoderNone disables place search. Station and stop names still
	// resolve — the routing provider handles those itself.
	GeocoderNone GeocoderChoice = "none"
	// GeocoderKakao uses the Kakao Local API, which needs a REST key from the
	// OS keychain.
	GeocoderKakao GeocoderChoice = "kakao"
)

// Settings is the non-secret configuration every entry point reads. It lives
// in a plain file rather than the OS keychain on purpose: a provider name and
// a URL are not secrets, and putting them in the keychain would raise an
// unlock prompt for self-hosting users who have no secret to store at all —
// and would lock out headless Linux, where no Secret Service may exist.
//
// API keys are NOT here. They stay in the keychain via Save/Load/Delete.
type Settings struct {
	RoutingProvider RoutingProvider `json:"routing_provider"`
	MotisURL        string          `json:"motis_url,omitempty"`
	Geocoder        GeocoderChoice  `json:"geocoder,omitempty"`
}

// ErrInvalidSettings indicates SaveSettings was handed a value that would
// leave the tool in an unusable state. Nothing is written when it is
// returned: a partial file would report a state the user never confirmed.
var ErrInvalidSettings = errors.New("config: settings are not valid")

// configDirFn resolves the per-user configuration root. It is a variable so
// tests can point it at a temp directory; production always uses
// os.UserConfigDir, which already knows each platform's convention
// (~/Library/Application Support, $XDG_CONFIG_HOME, %AppData%). Hand-rolling
// that split would only add surface for the three-OS CI matrix to break.
var configDirFn = os.UserConfigDir

// settingsPathIn returns the settings file path under an explicit root.
func settingsPathIn(root string) string {
	return filepath.Join(root, appDirName, settingsFileName)
}

// SettingsPath returns the absolute path of the settings file.
func SettingsPath() (string, error) {
	root, err := configDirFn()
	if err != nil {
		return "", fmt.Errorf("config: locate user config dir: %w", err)
	}
	return settingsPathIn(root), nil
}

// LoadSettings reads the stored settings. It is deliberately forgiving: a
// missing file, an unreadable one, malformed JSON, and unrecognized values
// all resolve to a zero Settings rather than an error, because every one of
// those means the same thing to the caller — nothing usable is configured,
// so tell the user to run setup. Returning a distinct error per cause would
// multiply failure codes that all end in the same instruction.
//
// It never returns the underlying parse or I/O error text. Those strings
// carry file paths and decoder internals that mean nothing to the user and
// must not reach an AI caller (spec 006 FR-019).
//
// Unknown JSON keys are ignored so a settings file written by a newer
// naeryeo does not break an older one.
func LoadSettings() (Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		logger.Warn("config: settings path unavailable")
		return unsetSettings(), nil
	}

	raw, err := os.ReadFile(path) // #nosec G304 -- path is derived from os.UserConfigDir, not user input
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("config: settings unreadable", "path", path)
		} else {
			logger.Info("config: no settings file", "path", path)
		}
		return unsetSettings(), nil
	}

	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		// The decoder error is dropped, not wrapped: it quotes the offending
		// bytes of the user's file.
		logger.Warn("config: settings file is not valid JSON", "path", path)
		return unsetSettings(), nil
	}

	return sanitize(s), nil
}

// unsetSettings is the value every "nothing usable is configured" path
// resolves to. Geocoder is spelled out rather than left as the zero string so
// callers can compare against GeocoderNone without special-casing "".
func unsetSettings() Settings {
	return Settings{RoutingProvider: ProviderUnset, Geocoder: GeocoderNone}
}

// sanitize downgrades any field that cannot be trusted to its zero value.
// The provider is cleared when it is unrecognized, and also when it is motis
// but the URL is unusable — a provider we cannot actually reach is not a
// configured provider, and reporting it as one would turn a setup problem
// into a confusing connection failure at search time.
func sanitize(s Settings) Settings {
	switch s.RoutingProvider {
	case ProviderMotis:
		normalized, err := normalizeMotisURL(s.MotisURL)
		if err != nil {
			logger.Warn("config: stored MOTIS URL is unusable; treating provider as unset")
			return Settings{RoutingProvider: ProviderUnset, Geocoder: sanitizeGeocoder(s.Geocoder)}
		}
		s.MotisURL = normalized
	case ProviderODsay:
		s.MotisURL = ""
	default:
		return Settings{RoutingProvider: ProviderUnset, Geocoder: sanitizeGeocoder(s.Geocoder)}
	}

	s.Geocoder = sanitizeGeocoder(s.Geocoder)
	return s
}

func sanitizeGeocoder(g GeocoderChoice) GeocoderChoice {
	if g == GeocoderKakao {
		return GeocoderKakao
	}
	return GeocoderNone
}

// normalizeMotisURL validates a MOTIS base URL and strips any trailing
// slash, so callers can join paths without producing a double slash.
func normalizeMotisURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: MOTIS URL is required when the routing provider is %q", ErrInvalidSettings, ProviderMotis)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: MOTIS URL cannot be parsed", ErrInvalidSettings)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w: MOTIS URL must start with http:// or https://", ErrInvalidSettings)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: MOTIS URL has no host", ErrInvalidSettings)
	}

	return strings.TrimRight(trimmed, "/"), nil
}

// Validate reports whether s is a usable configuration. setup calls it before
// prompting for anything expensive so an invalid combination fails early.
func (s Settings) Validate() error {
	switch s.RoutingProvider {
	case ProviderMotis:
		if _, err := normalizeMotisURL(s.MotisURL); err != nil {
			return err
		}
	case ProviderODsay:
		// No URL and no further validation: the API key lives in the keychain
		// and is checked when it is actually used.
	default:
		return fmt.Errorf("%w: routing provider must be %q or %q", ErrInvalidSettings, ProviderMotis, ProviderODsay)
	}

	switch s.Geocoder {
	case GeocoderNone, GeocoderKakao, "":
	default:
		return fmt.Errorf("%w: geocoder must be %q or %q", ErrInvalidSettings, GeocoderKakao, GeocoderNone)
	}

	return nil
}

// SaveSettings validates s and writes it, creating the directory if needed.
// An invalid value is rejected without touching the filesystem.
//
// The file is written whole via a temp file and rename so an interrupted
// write cannot leave a truncated document that the next load would silently
// read as "nothing configured".
func SaveSettings(s Settings) error {
	if err := s.Validate(); err != nil {
		logger.Warn("config: settings save rejected", "provider", string(s.RoutingProvider))
		return err
	}

	if s.RoutingProvider == ProviderMotis {
		normalized, err := normalizeMotisURL(s.MotisURL)
		if err != nil {
			return err
		}
		s.MotisURL = normalized
	} else {
		s.MotisURL = ""
	}
	if s.Geocoder == "" {
		s.Geocoder = GeocoderNone
	}

	path, err := SettingsPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: create settings directory: %w", err)
	}

	encoded, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode settings: %w", err)
	}
	encoded = append(encoded, '\n')

	tmp, err := os.CreateTemp(dir, settingsFileName+".*")
	if err != nil {
		return fmt.Errorf("config: create temp settings file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: write settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: close settings: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("config: set settings permissions: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("config: install settings file: %w", err)
	}

	logger.Info("config: settings saved", "provider", string(s.RoutingProvider), "geocoder", string(s.Geocoder))
	return nil
}
