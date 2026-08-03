package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withConfigDir points settings storage at a temporary directory for the
// duration of one test. It substitutes the package's directory resolver
// rather than mutating HOME/XDG_CONFIG_HOME, because os.UserConfigDir reads
// different variables on each OS and this test suite runs on all three.
func withConfigDir(t *testing.T, dir string) {
	t.Helper()
	original := configDirFn
	configDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDirFn = original })
}

func TestLoadSettings_NoFileIsNotAnError(t *testing.T) {
	withConfigDir(t, t.TempDir())

	got, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error = %v, want nil — a missing file is a normal state", err)
	}
	if got.RoutingProvider != ProviderUnset {
		t.Fatalf("RoutingProvider = %q, want %q", got.RoutingProvider, ProviderUnset)
	}
	if got.Geocoder != GeocoderNone {
		t.Fatalf("Geocoder = %q, want %q", got.Geocoder, GeocoderNone)
	}
}

func TestLoadSettings_MalformedJSONLeaksNothing(t *testing.T) {
	dir := t.TempDir()
	withConfigDir(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, appDirName), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// The sentinel below is what a raw json.SyntaxError would quote back.
	broken := `{"routing_provider": "motis", SENTINEL_LEAK`
	if err := os.WriteFile(settingsPathIn(dir), []byte(broken), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error = %v, want nil — a corrupt file degrades to unset", err)
	}
	if got.RoutingProvider != ProviderUnset {
		t.Fatalf("RoutingProvider = %q, want %q", got.RoutingProvider, ProviderUnset)
	}
}

func TestLoadSettings_Table(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantProvider RoutingProvider
		wantMotisURL string
		wantGeocoder GeocoderChoice
	}{
		{
			name:         "motis with url",
			content:      `{"routing_provider":"motis","motis_url":"http://localhost:8080"}`,
			wantProvider: ProviderMotis,
			wantMotisURL: "http://localhost:8080",
			wantGeocoder: GeocoderNone,
		},
		{
			name:         "odsay with kakao geocoder",
			content:      `{"routing_provider":"odsay","geocoder":"kakao"}`,
			wantProvider: ProviderODsay,
			wantGeocoder: GeocoderKakao,
		},
		{
			name:         "unknown keys are ignored",
			content:      `{"routing_provider":"odsay","future_field":{"nested":1},"another":"x"}`,
			wantProvider: ProviderODsay,
			wantGeocoder: GeocoderNone,
		},
		{
			name:         "unrecognized provider degrades to unset",
			content:      `{"routing_provider":"valhalla"}`,
			wantProvider: ProviderUnset,
			wantGeocoder: GeocoderNone,
		},
		{
			name:         "motis without url degrades to unset",
			content:      `{"routing_provider":"motis"}`,
			wantProvider: ProviderUnset,
			wantGeocoder: GeocoderNone,
		},
		{
			name:         "motis with unusable url degrades to unset",
			content:      `{"routing_provider":"motis","motis_url":"not-a-url"}`,
			wantProvider: ProviderUnset,
			wantGeocoder: GeocoderNone,
		},
		{
			name:         "unrecognized geocoder degrades to none",
			content:      `{"routing_provider":"odsay","geocoder":"naver"}`,
			wantProvider: ProviderODsay,
			wantGeocoder: GeocoderNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			withConfigDir(t, dir)
			if err := os.MkdirAll(filepath.Join(dir, appDirName), 0o700); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			if err := os.WriteFile(settingsPathIn(dir), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			got, err := LoadSettings()
			if err != nil {
				t.Fatalf("LoadSettings() error = %v, want nil", err)
			}
			if got.RoutingProvider != tt.wantProvider {
				t.Errorf("RoutingProvider = %q, want %q", got.RoutingProvider, tt.wantProvider)
			}
			if got.MotisURL != tt.wantMotisURL {
				t.Errorf("MotisURL = %q, want %q", got.MotisURL, tt.wantMotisURL)
			}
			if got.Geocoder != tt.wantGeocoder {
				t.Errorf("Geocoder = %q, want %q", got.Geocoder, tt.wantGeocoder)
			}
		})
	}
}

func TestSaveSettings_RoundTrip(t *testing.T) {
	withConfigDir(t, t.TempDir())

	want := Settings{
		RoutingProvider: ProviderMotis,
		MotisURL:        "http://motis.lan:8080",
		Geocoder:        GeocoderKakao,
	}
	if err := SaveSettings(want); err != nil {
		t.Fatalf("SaveSettings() error = %v, want nil", err)
	}

	got, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error = %v, want nil", err)
	}
	if got != want {
		t.Fatalf("round-trip = %+v, want %+v", got, want)
	}
}

func TestSaveSettings_NormalizesTrailingSlash(t *testing.T) {
	withConfigDir(t, t.TempDir())

	if err := SaveSettings(Settings{
		RoutingProvider: ProviderMotis,
		MotisURL:        "http://localhost:8080/",
	}); err != nil {
		t.Fatalf("SaveSettings() error = %v, want nil", err)
	}

	got, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if got.MotisURL != "http://localhost:8080" {
		t.Fatalf("MotisURL = %q, want %q — the trailing slash must be normalized away",
			got.MotisURL, "http://localhost:8080")
	}
}

func TestSaveSettings_RejectsInvalidWithoutWritingAFile(t *testing.T) {
	tests := []struct {
		name string
		in   Settings
	}{
		{"unset provider", Settings{}},
		{"unrecognized provider", Settings{RoutingProvider: "valhalla"}},
		{"motis without url", Settings{RoutingProvider: ProviderMotis}},
		{"motis with relative url", Settings{RoutingProvider: ProviderMotis, MotisURL: "localhost:8080"}},
		{"motis with unsupported scheme", Settings{RoutingProvider: ProviderMotis, MotisURL: "ftp://x:21"}},
		{"motis with empty host", Settings{RoutingProvider: ProviderMotis, MotisURL: "http://"}},
		{"unrecognized geocoder", Settings{RoutingProvider: ProviderODsay, Geocoder: "naver"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			withConfigDir(t, dir)

			if err := SaveSettings(tt.in); err == nil {
				t.Fatal("SaveSettings() error = nil, want a validation error")
			}
			// A rejected save must not leave a partial file behind: the next
			// load would then report a state the user never confirmed.
			if _, err := os.Stat(settingsPathIn(dir)); !os.IsNotExist(err) {
				t.Fatalf("settings file exists after a rejected save (stat err = %v)", err)
			}
		})
	}
}

func TestSaveSettings_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	withConfigDir(t, dir)

	if err := SaveSettings(Settings{RoutingProvider: ProviderODsay}); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}

	info, err := os.Stat(settingsPathIn(dir))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %04o, want 0600", perm)
	}

	dirInfo, err := os.Stat(filepath.Join(dir, appDirName))
	if err != nil {
		t.Fatalf("Stat(dir) error = %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode = %04o, want 0700", perm)
	}
}

func TestSettingsPath_IsUnderUserConfigDir(t *testing.T) {
	// The real resolver is exercised here (not the test double) so the
	// three-OS CI matrix actually verifies the platform convention.
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("os.UserConfigDir() unavailable in this environment: %v", err)
	}

	got, err := SettingsPath()
	if err != nil {
		t.Fatalf("SettingsPath() error = %v", err)
	}
	if !strings.HasPrefix(got, base) {
		t.Errorf("SettingsPath() = %q, want a path under %q", got, base)
	}
	if filepath.Base(got) != settingsFileName {
		t.Errorf("SettingsPath() basename = %q, want %q", filepath.Base(got), settingsFileName)
	}
	if filepath.Base(filepath.Dir(got)) != appDirName {
		t.Errorf("SettingsPath() parent = %q, want %q", filepath.Base(filepath.Dir(got)), appDirName)
	}
}
