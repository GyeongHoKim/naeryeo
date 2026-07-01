package config

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "naeryeo"
	keyUsername = "odsay-api-key"
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

// Save stores apiKey in the OS keychain, overwriting any previously stored
// value. It returns ErrEmptyValue if apiKey is empty and never falls back
// to plaintext storage if the keychain backend is unavailable.
func Save(apiKey string) error {
	if apiKey == "" {
		logger.Warn("config: save rejected: empty API key")
		return ErrEmptyValue
	}
	err := wrapBackendErr(backend.Set(serviceName, keyUsername, apiKey))
	if err != nil {
		logger.Error("config: save failed", "error", err)
		return err
	}
	logger.Info("config: API key saved")
	return nil
}

// Load returns the previously stored API key. It returns ErrNotConfigured
// if no key has been stored.
func Load() (string, error) {
	value, err := backend.Get(serviceName, keyUsername)
	if err != nil {
		wrapped := wrapBackendErr(err)
		if errors.Is(wrapped, ErrNotConfigured) {
			logger.Info("config: no API key stored")
		} else {
			logger.Error("config: load failed", "error", wrapped)
		}
		return "", wrapped
	}
	logger.Debug("config: API key loaded")
	return value, nil
}

// Delete removes the stored API key. It is idempotent: calling it when no
// key is stored is not an error.
func Delete() error {
	err := backend.Delete(serviceName, keyUsername)
	if errors.Is(err, keyring.ErrNotFound) {
		logger.Info("config: delete: no API key was stored")
		return nil
	}
	wrapped := wrapBackendErr(err)
	if wrapped != nil {
		logger.Error("config: delete failed", "error", wrapped)
		return wrapped
	}
	logger.Info("config: API key deleted")
	return nil
}
