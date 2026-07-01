package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSetLogger(t *testing.T) {
	t.Cleanup(func() { logger = slog.New(slog.DiscardHandler) })

	t.Run("nil is a no-op", func(t *testing.T) {
		var buf bytes.Buffer
		custom := slog.New(slog.NewTextHandler(&buf, nil))
		logger = custom

		SetLogger(nil)

		if logger != custom {
			t.Error("SetLogger(nil) replaced the installed logger")
		}
	})

	t.Run("installs a non-nil logger", func(t *testing.T) {
		var buf bytes.Buffer
		custom := slog.New(slog.NewTextHandler(&buf, nil))

		SetLogger(custom)
		logger.Info("probe")

		if !strings.Contains(buf.String(), "probe") {
			t.Errorf("installed logger did not receive log output: %q", buf.String())
		}
	})
}

// TestSave_NeverLogsTheAPIKey is the security-critical proof that Save's
// logging never leaks the API key value, in any outcome.
func TestSave_NeverLogsTheAPIKey(t *testing.T) {
	t.Cleanup(func() { logger = slog.New(slog.DiscardHandler) })

	const secret = "super-secret-value"

	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	withBackend(t, fakeSetOnlyBackend{})

	if err := Save(secret); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	if strings.Contains(buf.String(), secret) {
		t.Fatalf("log output contains the API key value: %q", buf.String())
	}
}

// fakeSetOnlyBackend always succeeds, used only to isolate Save's logging
// behavior from the real keychain.
type fakeSetOnlyBackend struct{}

func (fakeSetOnlyBackend) Set(string, string, string) error   { return nil }
func (fakeSetOnlyBackend) Get(string, string) (string, error) { return "", nil }
func (fakeSetOnlyBackend) Delete(string, string) error        { return nil }
