package config

import "log/slog"

var logger = slog.New(slog.DiscardHandler)

// SetLogger installs l as this package's logger. Passing nil is a no-op.
// Log lines from this package never include the API key value itself.
func SetLogger(l *slog.Logger) {
	if l != nil {
		logger = l
	}
}
