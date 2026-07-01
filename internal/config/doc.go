// Package config manages persistence of the ODsay API key via the host OS
// keychain (github.com/zalando/go-keyring). It exposes Save, Load, and
// Delete for the single stored API key. It has no plaintext-file fallback:
// if no OS keychain backend is available, all three functions return
// ErrKeychainUnavailable instead of falling back to disk.
package config
