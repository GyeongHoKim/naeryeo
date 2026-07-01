package main

import (
	"bytes"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
)

func TestRunLogout(t *testing.T) {
	t.Run("deletes an existing key on the ODsay credential by default", func(t *testing.T) {
		var loadedCred, deletedCred config.Credential
		load := func(cred config.Credential) (string, error) { loadedCred = cred; return "some-key", nil }
		del := func(cred config.Credential) error { deletedCred = cred; return nil }

		var stdout, stderr bytes.Buffer
		code := runLogout(nil, &stdout, &stderr, load, del)

		if code != 0 {
			t.Fatalf("runLogout() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if loadedCred != config.ODsayAPIKey || deletedCred != config.ODsayAPIKey {
			t.Fatalf("targeted load=%q del=%q, want ODsayAPIKey", loadedCred, deletedCred)
		}
		if stdout.Len() == 0 {
			t.Fatal("expected a success message on stdout")
		}
	})

	t.Run("--geocoder targets the geocoder credential", func(t *testing.T) {
		var loadedCred, deletedCred config.Credential
		load := func(cred config.Credential) (string, error) { loadedCred = cred; return "kakao-key", nil }
		del := func(cred config.Credential) error { deletedCred = cred; return nil }

		var stdout, stderr bytes.Buffer
		code := runLogout([]string{"--geocoder"}, &stdout, &stderr, load, del)

		if code != 0 {
			t.Fatalf("runLogout() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if loadedCred != config.GeocoderAPIKey || deletedCred != config.GeocoderAPIKey {
			t.Fatalf("targeted load=%q del=%q, want GeocoderAPIKey", loadedCred, deletedCred)
		}
	})

	t.Run("no stored key reports nothing to delete without failing", func(t *testing.T) {
		load := func(config.Credential) (string, error) { return "", config.ErrNotConfigured }
		del := func(config.Credential) error { return nil }

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
		load := func(config.Credential) (string, error) { return "", config.ErrKeychainUnavailable }
		del := func(config.Credential) error { return config.ErrKeychainUnavailable }

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
