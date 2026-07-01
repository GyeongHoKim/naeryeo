package main

import (
	"bytes"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
)

func TestRunLogout(t *testing.T) {
	t.Run("deletes an existing key", func(t *testing.T) {
		load := func() (string, error) { return "some-key", nil }
		deleted := false
		del := func() error {
			deleted = true
			return nil
		}

		var stdout, stderr bytes.Buffer
		code := runLogout(nil, &stdout, &stderr, load, del)

		if code != 0 {
			t.Fatalf("runLogout() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if !deleted {
			t.Fatal("expected del to be called")
		}
		if stdout.Len() == 0 {
			t.Fatal("expected a success message on stdout")
		}
	})

	t.Run("no stored key reports nothing to delete without failing", func(t *testing.T) {
		load := func() (string, error) { return "", config.ErrNotConfigured }
		del := func() error { return nil }

		var stdout, stderr bytes.Buffer
		code := runLogout(nil, &stdout, &stderr, load, del)

		if code != 0 {
			t.Fatalf("runLogout() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if stdout.Len() == 0 {
			t.Fatal("expected an informational message on stdout")
		}
	})

	t.Run("keychain unavailable surfaces a clear error and non-zero exit", func(t *testing.T) {
		load := func() (string, error) { return "", config.ErrKeychainUnavailable }
		del := func() error { return config.ErrKeychainUnavailable }

		var stdout, stderr bytes.Buffer
		code := runLogout(nil, &stdout, &stderr, load, del)

		if code == 0 {
			t.Fatal("runLogout() code = 0, want non-zero")
		}
		if stderr.Len() == 0 {
			t.Fatal("expected an error message on stderr")
		}
	})
}
