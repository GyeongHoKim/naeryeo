package config

import "testing"

// This file exposes the two seams cmd/naeryeo's tests need to exercise the
// provider resolver end to end: where settings are read from, and what the OS
// keychain holds. Both are already injectable inside this package (configDirFn
// and backend); these helpers only make them reachable from another package's
// tests, and each one restores the original via t.Cleanup so tests stay
// independent.
//
// It lives in a non-test file because Go does not export test-only symbols
// across package boundaries. It takes *testing.T, so it cannot be called from
// production code by accident.

// SetConfigDirForTest points settings storage at dir for the duration of t.
func SetConfigDirForTest(t *testing.T, dir string) {
	t.Helper()
	original := configDirFn
	configDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDirFn = original })
}

// mapBackend is an in-memory keychain. A non-nil err makes every operation
// fail, which is how a test reproduces "the keychain cannot be read at all"
// without an unusable machine.
type mapBackend struct {
	values map[string]string
	err    error
}

func (b *mapBackend) Set(_, username, password string) error {
	if b.err != nil {
		return b.err
	}
	b.values[username] = password
	return nil
}

func (b *mapBackend) Get(_, username string) (string, error) {
	if b.err != nil {
		return "", b.err
	}
	v, ok := b.values[username]
	if !ok {
		return "", ErrNotConfigured
	}
	return v, nil
}

func (b *mapBackend) Delete(_, username string) error {
	if b.err != nil {
		return b.err
	}
	delete(b.values, username)
	return nil
}

// SetCredentialsForTest replaces the keychain with an in-memory one holding
// exactly creds.
func SetCredentialsForTest(t *testing.T, creds map[Credential]string) {
	t.Helper()
	values := make(map[string]string, len(creds))
	for k, v := range creds {
		values[string(k)] = v
	}
	swapBackend(t, &mapBackend{values: values})
}

// SetCredentialErrorForTest makes every keychain operation fail with err.
func SetCredentialErrorForTest(t *testing.T, err error) {
	t.Helper()
	swapBackend(t, &mapBackend{values: map[string]string{}, err: err})
}

func swapBackend(t *testing.T, b keyringBackend) {
	t.Helper()
	original := backend
	backend = b
	t.Cleanup(func() { backend = original })
}
