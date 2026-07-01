package config

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

// unavailableBackend simulates an OS keychain backend that cannot be used
// at all — including the case go-keyring's own ErrUnsupportedPlatform
// sentinel does NOT cover: an opaque error such as a missing D-Bus Secret
// Service session on headless Linux (see research.md §1).
type unavailableBackend struct {
	err error
}

func (b unavailableBackend) Set(string, string, string) error   { return b.err }
func (b unavailableBackend) Get(string, string) (string, error) { return "", b.err }
func (b unavailableBackend) Delete(string, string) error        { return b.err }

func withBackend(t *testing.T, b keyringBackend) {
	t.Helper()
	original := backend
	backend = b
	t.Cleanup(func() { backend = original })
}

func TestSave(t *testing.T) {
	t.Run("empty value is rejected", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		if err := Save(""); !errors.Is(err, ErrEmptyValue) {
			t.Fatalf("Save(\"\") error = %v, want ErrEmptyValue", err)
		}
	})

	t.Run("first save is retrievable directly via keyring.Get", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		if err := Save("first-key"); err != nil {
			t.Fatalf("Save() error = %v, want nil", err)
		}
		got, err := keyring.Get(serviceName, keyUsername)
		if err != nil {
			t.Fatalf("keyring.Get() error = %v, want nil", err)
		}
		if got != "first-key" {
			t.Fatalf("keyring.Get() = %q, want %q", got, "first-key")
		}
	})

	t.Run("second save overwrites the first", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		if err := Save("old-key"); err != nil {
			t.Fatalf("Save(old) error = %v, want nil", err)
		}
		if err := Save("new-key"); err != nil {
			t.Fatalf("Save(new) error = %v, want nil", err)
		}
		got, err := keyring.Get(serviceName, keyUsername)
		if err != nil {
			t.Fatalf("keyring.Get() error = %v, want nil", err)
		}
		if got != "new-key" {
			t.Fatalf("keyring.Get() = %q, want %q", got, "new-key")
		}
	})

	t.Run("backend unavailable surfaces ErrKeychainUnavailable", func(t *testing.T) {
		withBackend(t, unavailableBackend{err: keyring.ErrUnsupportedPlatform})

		if err := Save("some-key"); !errors.Is(err, ErrKeychainUnavailable) {
			t.Fatalf("Save() error = %v, want ErrKeychainUnavailable", err)
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("no stored value returns ErrNotConfigured", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		_, err := Load()
		if !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("Load() error = %v, want ErrNotConfigured", err)
		}
	})

	t.Run("returns a value seeded directly via keyring.Set", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		if err := keyring.Set(serviceName, keyUsername, "seeded-key"); err != nil {
			t.Fatalf("keyring.Set() error = %v, want nil", err)
		}
		got, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}
		if got != "seeded-key" {
			t.Fatalf("Load() = %q, want %q", got, "seeded-key")
		}
	})

	t.Run("round-trips the value passed to Save", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		if err := Save("round-trip-key"); err != nil {
			t.Fatalf("Save() error = %v, want nil", err)
		}
		got, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}
		if got != "round-trip-key" {
			t.Fatalf("Load() = %q, want %q", got, "round-trip-key")
		}
	})

	t.Run("backend unavailable surfaces ErrKeychainUnavailable", func(t *testing.T) {
		withBackend(t, unavailableBackend{err: keyring.ErrUnsupportedPlatform})

		if _, err := Load(); !errors.Is(err, ErrKeychainUnavailable) {
			t.Fatalf("Load() error = %v, want ErrKeychainUnavailable", err)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("removes a previously stored value", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		if err := keyring.Set(serviceName, keyUsername, "to-delete"); err != nil {
			t.Fatalf("keyring.Set() error = %v, want nil", err)
		}
		if err := Delete(); err != nil {
			t.Fatalf("Delete() error = %v, want nil", err)
		}
		if _, err := keyring.Get(serviceName, keyUsername); !errors.Is(err, keyring.ErrNotFound) {
			t.Fatalf("keyring.Get() error = %v, want keyring.ErrNotFound", err)
		}
	})

	t.Run("is idempotent when nothing is stored", func(t *testing.T) {
		keyring.MockInit()
		withBackend(t, goKeyringBackend{})

		if err := Delete(); err != nil {
			t.Fatalf("Delete() error = %v, want nil", err)
		}
	})

	t.Run("backend unavailable surfaces ErrKeychainUnavailable", func(t *testing.T) {
		withBackend(t, unavailableBackend{err: keyring.ErrUnsupportedPlatform})

		if err := Delete(); !errors.Is(err, ErrKeychainUnavailable) {
			t.Fatalf("Delete() error = %v, want ErrKeychainUnavailable", err)
		}
	})
}

// TestUnavailableBackend_OpaqueError verifies that Save, Load, and Delete
// all treat ANY non-sentinel backend error as "keychain unavailable" — not
// just keyring.ErrUnsupportedPlatform. This is the failure mode go-keyring
// actually produces on headless Linux (no D-Bus Secret Service session):
// an opaque connection error, not a sentinel (research.md §1).
func TestUnavailableBackend_OpaqueError(t *testing.T) {
	opaqueErr := errors.New("dbus: could not connect: no such file or directory")
	withBackend(t, unavailableBackend{err: opaqueErr})

	if err := Save("some-key"); !errors.Is(err, ErrKeychainUnavailable) || !errors.Is(err, opaqueErr) {
		t.Errorf("Save() error = %v, want wrapping both ErrKeychainUnavailable and the opaque cause", err)
	}
	if _, err := Load(); !errors.Is(err, ErrKeychainUnavailable) || !errors.Is(err, opaqueErr) {
		t.Errorf("Load() error = %v, want wrapping both ErrKeychainUnavailable and the opaque cause", err)
	}
	if err := Delete(); !errors.Is(err, ErrKeychainUnavailable) || !errors.Is(err, opaqueErr) {
		t.Errorf("Delete() error = %v, want wrapping both ErrKeychainUnavailable and the opaque cause", err)
	}
}
