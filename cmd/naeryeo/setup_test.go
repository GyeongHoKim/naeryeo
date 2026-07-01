package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
)

func TestRunSetup(t *testing.T) {
	t.Run("valid key is trimmed and saved to the ODsay credential by default", func(t *testing.T) {
		var savedCred config.Credential
		var savedWith string
		save := func(cred config.Credential, apiKey string) error {
			savedCred = cred
			savedWith = apiKey
			return nil
		}

		var stdout, stderr bytes.Buffer
		code := runSetup(nil, strings.NewReader("  my-api-key  \n"), &stdout, &stderr, save)

		if code != 0 {
			t.Fatalf("runSetup() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if savedWith != "my-api-key" {
			t.Fatalf("save called with %q, want %q", savedWith, "my-api-key")
		}
		if savedCred != config.ODsayAPIKey {
			t.Fatalf("save targeted %q, want ODsayAPIKey", savedCred)
		}
		if !strings.Contains(stdout.String(), "ODsay API Key:") {
			t.Fatalf("prompt = %q, want it to mention ODsay", stdout.String())
		}
	})

	t.Run("--geocoder targets the geocoder credential with a Kakao prompt", func(t *testing.T) {
		var savedCred config.Credential
		var savedWith string
		save := func(cred config.Credential, apiKey string) error {
			savedCred = cred
			savedWith = apiKey
			return nil
		}

		var stdout, stderr bytes.Buffer
		code := runSetup([]string{"--geocoder"}, strings.NewReader("kakao-key\n"), &stdout, &stderr, save)

		if code != 0 {
			t.Fatalf("runSetup() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if savedCred != config.GeocoderAPIKey {
			t.Fatalf("save targeted %q, want GeocoderAPIKey", savedCred)
		}
		if savedWith != "kakao-key" {
			t.Fatalf("save called with %q, want %q", savedWith, "kakao-key")
		}
		if !strings.Contains(stdout.String(), "Kakao REST API Key:") {
			t.Fatalf("prompt = %q, want it to mention Kakao", stdout.String())
		}
	})

	t.Run("empty input does not call save", func(t *testing.T) {
		called := false
		save := func(config.Credential, string) error {
			called = true
			return nil
		}

		var stdout, stderr bytes.Buffer
		code := runSetup(nil, strings.NewReader("\n"), &stdout, &stderr, save)

		if code == 0 {
			t.Fatalf("runSetup() code = %d, want non-zero", code)
		}
		if called {
			t.Fatal("save should not be called for empty input")
		}
	})

	t.Run("keychain unavailable surfaces a clear error and non-zero exit", func(t *testing.T) {
		save := func(config.Credential, string) error {
			return config.ErrKeychainUnavailable
		}

		var stdout, stderr bytes.Buffer
		code := runSetup(nil, strings.NewReader("my-api-key\n"), &stdout, &stderr, save)

		if code == 0 {
			t.Fatal("runSetup() code = 0, want non-zero")
		}
		if stderr.Len() == 0 {
			t.Fatal("expected an error message on stderr")
		}
	})
}
