// Package config manages persistence of the ODsay API key via the host OS
// keychain (github.com/zalando/go-keyring). It has no plaintext-file
// fallback: if no OS keychain backend is available, callers must fail closed
// with an error.
//
// This package is currently a placeholder; real implementation lands via a
// dedicated feature spec (see .specify/).
package config
