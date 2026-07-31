package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/motis"
)

// fakeSetup records everything runSetup would otherwise write to the keychain,
// the settings file, or the network. Every field is inspectable so a test can
// assert on what was stored as well as on what was printed.
type fakeSetup struct {
	saved    map[config.Credential]string
	deleted  []config.Credential
	settings *config.Settings

	saveErr   error
	deleteErr error
	probeErr  error

	stored     map[config.Credential]string
	probeCalls int
	probedURL  string
}

func newFakeSetup() *fakeSetup {
	return &fakeSetup{
		saved:  map[config.Credential]string{},
		stored: map[config.Credential]string{},
	}
}

func (f *fakeSetup) deps() setupDeps {
	return setupDeps{
		SaveCredential: func(c config.Credential, v string) error {
			if f.saveErr != nil {
				return f.saveErr
			}
			f.saved[c] = v
			f.stored[c] = v
			return nil
		},
		LoadCredential: func(c config.Credential) (string, error) {
			if v, ok := f.stored[c]; ok {
				return v, nil
			}
			return "", config.ErrNotConfigured
		},
		DeleteCredential: func(c config.Credential) error {
			if f.deleteErr != nil {
				return f.deleteErr
			}
			f.deleted = append(f.deleted, c)
			delete(f.stored, c)
			return nil
		},
		SaveSettings: func(s config.Settings) error {
			f.settings = &s
			return nil
		},
		ProbeMotis: func(_ context.Context, baseURL string) error {
			f.probeCalls++
			f.probedURL = baseURL
			return f.probeErr
		},
	}
}

func runSetupWith(t *testing.T, f *fakeSetup, args []string, stdin string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runSetup(args, strings.NewReader(stdin), &stdout, &stderr, f.deps())
	return code, stdout.String(), stderr.String()
}

// TestRunSetup_InteractiveMotisPath walks the whole wizard with a scripted
// stdin: provider, address, geocoder, confirmation. Driving it line-by-line
// (rather than through a terminal emulator) is the reason the wizard stayed a
// prompt loop instead of a TUI.
func TestRunSetup_InteractiveMotisPath(t *testing.T) {
	f := newFakeSetup()
	// Enter, Enter, Enter, Enter — every default accepted.
	code, stdout, stderr := runSetupWith(t, f, nil, "\n\n\n\n")

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if f.settings == nil {
		t.Fatal("no settings were saved")
	}
	if f.settings.RoutingProvider != config.ProviderMotis {
		t.Errorf("RoutingProvider = %q, want %q — self-hosting is the default choice",
			f.settings.RoutingProvider, config.ProviderMotis)
	}
	if f.settings.MotisURL != defaultMotisURL {
		t.Errorf("MotisURL = %q, want the offered default %q", f.settings.MotisURL, defaultMotisURL)
	}
	if f.settings.Geocoder != config.GeocoderNone {
		t.Errorf("Geocoder = %q, want %q", f.settings.Geocoder, config.GeocoderNone)
	}
	if len(f.saved) != 0 {
		t.Errorf("credentials saved = %v, want none — self-hosting needs no key", f.saved)
	}
	if f.probeCalls != 1 {
		t.Errorf("probe calls = %d, want 1", f.probeCalls)
	}
	if !strings.Contains(stdout, "저장 완료") {
		t.Errorf("stdout = %q, want a save confirmation", stdout)
	}
}

// TestRunSetup_SelfHostingNeverAsksForACommercialKey is User Story 1's first
// acceptance scenario stated as a test.
func TestRunSetup_SelfHostingNeverAsksForACommercialKey(t *testing.T) {
	f := newFakeSetup()
	code, stdout, stderr := runSetupWith(t, f, nil, "1\nhttp://motis.lan:8080\n1\n\n")

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	// The menu naturally *mentions* ODsay while offering it as the other
	// choice; what must not happen is being asked to supply its key. The
	// prompt label is the thing to look for.
	if strings.Contains(stdout, "ODsay API Key:") {
		t.Errorf("stdout prompted for an ODsay key on the self-hosting path:\n%s", stdout)
	}
	if strings.Contains(stdout, "발급받은 앱키가 필요합니다") {
		t.Errorf("stdout told the user to obtain a commercial key on the self-hosting path:\n%s", stdout)
	}
	if _, ok := f.saved[config.ODsayAPIKey]; ok {
		t.Error("an ODsay key was stored on the self-hosting path")
	}
	if f.probedURL != "http://motis.lan:8080" {
		t.Errorf("probed %q, want the address the user typed", f.probedURL)
	}
}

func TestRunSetup_InteractiveODsayPath(t *testing.T) {
	f := newFakeSetup()
	code, _, stderr := runSetupWith(t, f, nil, "2\nmy-odsay-key\n2\nmy-kakao-key\n\n")

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if got := f.saved[config.ODsayAPIKey]; got != "my-odsay-key" {
		t.Errorf("stored ODsay key = %q, want %q", got, "my-odsay-key")
	}
	if got := f.saved[config.GeocoderAPIKey]; got != "my-kakao-key" {
		t.Errorf("stored Kakao key = %q, want %q", got, "my-kakao-key")
	}
	if f.settings == nil || f.settings.Geocoder != config.GeocoderKakao {
		t.Errorf("settings = %+v, want geocoder kakao", f.settings)
	}
	if f.probeCalls != 0 {
		t.Errorf("probe calls = %d, want 0 — ODsay has no endpoint to probe", f.probeCalls)
	}
}

func TestRunSetup_TrimsSecretWhitespace(t *testing.T) {
	f := newFakeSetup()
	if code, _, stderr := runSetupWith(t, f, []string{"--provider=odsay", "--geocoder=none"},
		"  my-api-key  \n\n"); code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if got := f.saved[config.ODsayAPIKey]; got != "my-api-key" {
		t.Errorf("stored key = %q, want it trimmed to %q", got, "my-api-key")
	}
}

func TestRunSetup_NonInteractiveFlagTable(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		stdin        string
		wantCode     int
		wantProvider config.RoutingProvider
		wantGeocoder config.GeocoderChoice
		wantSaved    map[config.Credential]string
	}{
		{
			name:         "motis needs no stdin beyond the confirmation",
			args:         []string{"--provider=motis", "--motis-url=http://motis.lan:8080", "--geocoder=none"},
			stdin:        "\n",
			wantProvider: config.ProviderMotis,
			wantGeocoder: config.GeocoderNone,
			wantSaved:    map[config.Credential]string{},
		},
		{
			name:         "odsay reads its key from stdin",
			args:         []string{"--provider=odsay", "--geocoder=none"},
			stdin:        "odsay-key\n\n",
			wantProvider: config.ProviderODsay,
			wantGeocoder: config.GeocoderNone,
			wantSaved:    map[config.Credential]string{config.ODsayAPIKey: "odsay-key"},
		},
		{
			name:         "geocoder kakao reads its key from stdin",
			args:         []string{"--provider=motis", "--motis-url=http://x:8080", "--geocoder=kakao"},
			stdin:        "kakao-key\n\n",
			wantProvider: config.ProviderMotis,
			wantGeocoder: config.GeocoderKakao,
			wantSaved:    map[config.Credential]string{config.GeocoderAPIKey: "kakao-key"},
		},
		{
			name:     "unknown provider is rejected",
			args:     []string{"--provider=valhalla"},
			wantCode: 1,
		},
		{
			name:     "unknown geocoder is rejected",
			args:     []string{"--provider=motis", "--motis-url=http://x:8080", "--geocoder=naver"},
			stdin:    "\n",
			wantCode: 1,
		},
		{
			name:     "delete cannot be combined with configuration",
			args:     []string{"--delete=all", "--provider=motis"},
			wantCode: 1,
		},
		{
			name:     "unknown delete target is rejected",
			args:     []string{"--delete=everything"},
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSetup()
			code, _, stderr := runSetupWith(t, f, tt.args, tt.stdin)

			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d; stderr = %q", code, tt.wantCode, stderr)
			}
			if tt.wantCode != 0 {
				if f.settings != nil {
					t.Errorf("settings were saved despite a rejected invocation: %+v", f.settings)
				}
				return
			}
			if f.settings.RoutingProvider != tt.wantProvider {
				t.Errorf("RoutingProvider = %q, want %q", f.settings.RoutingProvider, tt.wantProvider)
			}
			if f.settings.Geocoder != tt.wantGeocoder {
				t.Errorf("Geocoder = %q, want %q", f.settings.Geocoder, tt.wantGeocoder)
			}
			if len(f.saved) != len(tt.wantSaved) {
				t.Fatalf("saved credentials = %v, want %v", f.saved, tt.wantSaved)
			}
			for cred, want := range tt.wantSaved {
				if f.saved[cred] != want {
					t.Errorf("saved[%s] = %q, want %q", cred, f.saved[cred], want)
				}
			}
		})
	}
}

// TestRunSetup_NoFlagAcceptsASecret is the structural guarantee behind
// FR-006: a secret handed in on the command line lands in shell history and in
// every `ps` listing on the machine, so no flag may accept one. Asserting on
// the FlagSet's own usage output keeps a future flag from quietly
// reintroducing the exposure.
func TestRunSetup_NoFlagAcceptsASecret(t *testing.T) {
	f := newFakeSetup()
	// An unknown flag makes the FlagSet print its full usage to stderr, which
	// is the complete list of flags this command accepts.
	_, _, stderr := runSetupWith(t, f, []string{"--definitely-not-a-flag"}, "")

	if !strings.Contains(stderr, "-provider") {
		t.Fatalf("stderr does not look like flag usage output, so this test proves nothing:\n%s", stderr)
	}
	for _, forbidden := range []string{"api-key", "apikey", "secret", "token", "password"} {
		if strings.Contains(strings.ToLower(stderr), forbidden) {
			t.Errorf("setup exposes a flag that looks like it accepts a secret (%q):\n%s", forbidden, stderr)
		}
	}
}

func TestRunSetup_ProbeRefusesUnusableEngines(t *testing.T) {
	tests := []struct {
		name     string
		probeErr error
		wantText string
	}{
		{
			name:     "unreachable engine",
			probeErr: errors.New("dial tcp: connection refused"),
			wantText: "연결할 수 없습니다",
		},
		{
			name:     "engine with no data loaded",
			probeErr: motis.ErrNoData,
			wantText: "데이터가 적재되지 않은",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSetup()
			f.probeErr = tt.probeErr

			code, _, stderr := runSetupWith(t, f,
				[]string{"--provider=motis", "--motis-url=http://motis.lan:8080"}, "\n\n")

			if code == 0 {
				t.Fatal("code = 0, want non-zero — an unusable engine must not be saved")
			}
			if f.settings != nil {
				t.Errorf("settings were saved despite a failed probe: %+v", f.settings)
			}
			if !strings.Contains(stderr, tt.wantText) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tt.wantText)
			}
			if !strings.Contains(stderr, selfHostingDocsURL) {
				t.Errorf("stderr = %q, want the self-hosting docs link", stderr)
			}
		})
	}
}

// TestRunSetup_ProbeFailureHidesTheAddress keeps the operator's private
// network out of setup's error output, the same way the route command's
// failures do.
func TestRunSetup_ProbeFailureHidesTheAddress(t *testing.T) {
	const addr = "http://motis.internal.example:18080"
	f := newFakeSetup()
	f.probeErr = errors.New("dial tcp 10.0.0.5:18080: connect: connection refused")

	_, _, stderr := runSetupWith(t, f, []string{"--provider=motis", "--motis-url=" + addr}, "\n\n")

	for _, needle := range []string{"motis.internal.example", "18080", "10.0.0.5"} {
		if strings.Contains(stderr, needle) {
			t.Errorf("stderr leaked %q from the operator's network:\n%s", needle, stderr)
		}
	}
}

func TestRunSetup_ConfirmationCanBeDeclined(t *testing.T) {
	f := newFakeSetup()
	code, stdout, _ := runSetupWith(t, f,
		[]string{"--provider=motis", "--motis-url=http://x:8080", "--geocoder=none"}, "n\n")

	if code == 0 {
		t.Fatal("code = 0, want non-zero when the user declines")
	}
	if f.settings != nil {
		t.Errorf("settings were saved after the user declined: %+v", f.settings)
	}
	if !strings.Contains(stdout, "저장하지 않았습니다") {
		t.Errorf("stdout = %q, want an explicit 'not saved' message", stdout)
	}
}

func TestRunSetup_EmptyKeyIsRejected(t *testing.T) {
	f := newFakeSetup()
	code, _, stderr := runSetupWith(t, f, []string{"--provider=odsay"}, "\n")

	if code == 0 {
		t.Fatal("code = 0, want non-zero for an empty key")
	}
	if !strings.Contains(stderr, "API 키를 입력해야 합니다") {
		t.Errorf("stderr = %q, want the empty-key message", stderr)
	}
	if f.settings != nil {
		t.Error("settings were saved despite the key prompt failing")
	}
}

func TestRunSetup_KeychainUnavailableIsReportedWithoutRawError(t *testing.T) {
	const raw = "keyring: exec gpg: no such file or directory"
	f := newFakeSetup()
	f.saveErr = errors.Join(config.ErrKeychainUnavailable, errors.New(raw))

	code, _, stderr := runSetupWith(t, f, []string{"--provider=odsay"}, "some-key\n")

	if code == 0 {
		t.Fatal("code = 0, want non-zero")
	}
	if strings.Contains(stderr, raw) {
		t.Errorf("stderr leaked the raw keychain error:\n%s", stderr)
	}
	if !strings.Contains(stderr, "키체인") {
		t.Errorf("stderr = %q, want a keychain-specific message", stderr)
	}
}

func TestRunSetup_Delete(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		stdin       string
		preexisting map[config.Credential]string
		wantDeleted []config.Credential
		wantMessage string
	}{
		{
			name:        "delete odsay only",
			args:        []string{"--delete=odsay"},
			preexisting: map[config.Credential]string{config.ODsayAPIKey: "k"},
			wantDeleted: []config.Credential{config.ODsayAPIKey},
			wantMessage: "삭제했습니다",
		},
		{
			name:        "delete kakao only",
			args:        []string{"--delete=kakao"},
			preexisting: map[config.Credential]string{config.GeocoderAPIKey: "k"},
			wantDeleted: []config.Credential{config.GeocoderAPIKey},
			wantMessage: "삭제했습니다",
		},
		{
			name:        "delete all",
			args:        []string{"--delete=all"},
			preexisting: map[config.Credential]string{config.ODsayAPIKey: "a", config.GeocoderAPIKey: "b"},
			wantDeleted: []config.Credential{config.ODsayAPIKey, config.GeocoderAPIKey},
			wantMessage: "삭제했습니다",
		},
		{
			name:        "deleting nothing says so",
			args:        []string{"--delete=odsay"},
			wantDeleted: []config.Credential{config.ODsayAPIKey},
			wantMessage: "삭제할 API 키가 없습니다",
		},
		{
			name:        "interactive delete via the third menu option",
			stdin:       "3\n1\n",
			preexisting: map[config.Credential]string{config.ODsayAPIKey: "k"},
			wantDeleted: []config.Credential{config.ODsayAPIKey},
			wantMessage: "삭제했습니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSetup()
			for k, v := range tt.preexisting {
				f.stored[k] = v
			}

			code, stdout, stderr := runSetupWith(t, f, tt.args, tt.stdin)

			if code != 0 {
				t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
			}
			if len(f.deleted) != len(tt.wantDeleted) {
				t.Fatalf("deleted = %v, want %v", f.deleted, tt.wantDeleted)
			}
			for i, want := range tt.wantDeleted {
				if f.deleted[i] != want {
					t.Errorf("deleted[%d] = %q, want %q", i, f.deleted[i], want)
				}
			}
			if !strings.Contains(stdout, tt.wantMessage) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tt.wantMessage)
			}
			// Deleting a credential must not un-configure the tool.
			if f.settings != nil {
				t.Errorf("settings were rewritten by a delete: %+v", f.settings)
			}
		})
	}
}

// TestRunSetup_GeocoderFlagMigration covers the one breaking change a returning
// user is most likely to hit by muscle memory.
func TestRunSetup_GeocoderFlagMigration(t *testing.T) {
	f := newFakeSetup()
	// The old boolean form, with nothing after it that flag can take as a value.
	code, _, stderr := runSetupWith(t, f, []string{"--geocoder"}, "")

	if code == 0 {
		t.Fatal("code = 0, want non-zero for the old flag form")
	}
	if !strings.Contains(stderr, "--geocoder=kakao") {
		t.Errorf("stderr = %q, want it to show the new flag form", stderr)
	}
}
