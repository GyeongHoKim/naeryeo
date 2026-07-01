package config

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const serviceName = "naeryeo"

// Credential identifies which stored API key an operation targets. Its value
// is used as the OS keychain "username" under the shared service name, so the
// two credentials live as independent keychain entries.
type Credential string

const (
	// ODsayAPIKey is the transit route-search key. Its value is unchanged
	// from the original single-key implementation so existing users' stored
	// keys continue to load without any migration.
	ODsayAPIKey Credential = "odsay-api-key"
	// GeocoderAPIKey is the place-search (geocoding) key used to resolve
	// building names and addresses that ODsay's station search cannot.
	GeocoderAPIKey Credential = "geocoder-api-key"
)

var (
	// ErrNotConfigured indicates no API key is currently stored.
	ErrNotConfigured = errors.New("config: no API key stored")
	// ErrKeychainUnavailable indicates the OS keychain backend cannot be
	// used (unsupported platform, no D-Bus Secret Service session, access
	// denied, etc). Callers must not fall back to plaintext storage.
	ErrKeychainUnavailable = errors.New("config: OS keychain unavailable")
	// ErrEmptyValue indicates an empty string was passed to Save.
	ErrEmptyValue = errors.New("config: API key must not be empty")
	// ErrValueTooLarge indicates the value exceeds the OS keychain's
	// platform-specific size limit.
	ErrValueTooLarge = errors.New("config: API key exceeds keychain size limit")
)

// keyringBackend is the minimal surface this package needs from an OS
// keychain provider. It exists so tests can substitute a fake backend,
// including one that simulates "no keychain backend available" — a state
// go-keyring's own MockInit cannot reproduce.
type keyringBackend interface {
	Set(service, username, password string) error
	Get(service, username string) (string, error)
	Delete(service, username string) error
}

type goKeyringBackend struct{}

func (goKeyringBackend) Set(service, username, password string) error {
	return keyring.Set(service, username, password)
}

func (goKeyringBackend) Get(service, username string) (string, error) {
	return keyring.Get(service, username)
}

func (goKeyringBackend) Delete(service, username string) error {
	return keyring.Delete(service, username)
}

var backend keyringBackend = goKeyringBackend{}

// wrapBackendErr translates a raw keyringBackend error into this package's
// sentinel errors. Any error that isn't ErrNotFound or ErrSetDataTooBig is
// treated as "keychain unavailable" — this also covers errors go-keyring
// does not expose as sentinels, such as a missing D-Bus Secret Service
// session on headless Linux.
func wrapBackendErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrNotConfigured
	case errors.Is(err, keyring.ErrSetDataTooBig):
		return fmt.Errorf("%w: %w", ErrValueTooLarge, err)
	default:
		return fmt.Errorf("%w: %w", ErrKeychainUnavailable, err)
	}
}

// Save stores apiKey for cred in the OS keychain, overwriting any previously
// stored value. It returns ErrEmptyValue if apiKey is empty and never falls
// back to plaintext storage if the keychain backend is unavailable.
func Save(cred Credential, apiKey string) error {
	if apiKey == "" {
		logger.Warn("config: save rejected: empty API key", "credential", string(cred))
		return ErrEmptyValue
	}
	err := wrapBackendErr(backend.Set(serviceName, string(cred), apiKey))
	if err != nil {
		logger.Error("config: save failed", "credential", string(cred), "error", err)
		return err
	}
	logger.Info("config: API key saved", "credential", string(cred))
	return nil
}

// Load returns the previously stored API key for cred. It returns
// ErrNotConfigured if no key has been stored for that credential.
func Load(cred Credential) (string, error) {
	value, err := backend.Get(serviceName, string(cred))
	if err != nil {
		wrapped := wrapBackendErr(err)
		if errors.Is(wrapped, ErrNotConfigured) {
			logger.Info("config: no API key stored", "credential", string(cred))
		} else {
			logger.Error("config: load failed", "credential", string(cred), "error", wrapped)
		}
		return "", wrapped
	}
	logger.Debug("config: API key loaded", "credential", string(cred))
	return value, nil
}

// Delete removes the stored API key for cred. It is idempotent: calling it
// when no key is stored is not an error.
func Delete(cred Credential) error {
	err := backend.Delete(serviceName, string(cred))
	if errors.Is(err, keyring.ErrNotFound) {
		logger.Info("config: delete: no API key was stored", "credential", string(cred))
		return nil
	}
	wrapped := wrapBackendErr(err)
	if wrapped != nil {
		logger.Error("config: delete failed", "credential", string(cred), "error", wrapped)
		return wrapped
	}
	logger.Info("config: API key deleted", "credential", string(cred))
	return nil
}
