package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no arguments prints usage", args: nil, wantCode: 1, wantStderr: "usage: naeryeo"},
		{name: "version flag", args: []string{"--version"}, wantCode: 0, wantStdout: "dev"},
		{name: "unknown command", args: []string{"bogus"}, wantCode: 1, wantStderr: "unknown command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tt.args, &stdout, &stderr, slog.New(slog.DiscardHandler))
			if got != tt.wantCode {
				t.Errorf("run(%v) = %d, want %d", tt.args, got, tt.wantCode)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want substring %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRun_LogsDispatch(t *testing.T) {
	var stdout, stderr bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&stderr, nil))

	run([]string{"--version"}, &stdout, &stderr, logger)

	if !strings.Contains(stderr.String(), "naeryeo: dispatch") {
		t.Errorf("stderr = %q, want a dispatch log line", stderr.String())
	}
	if !strings.Contains(stderr.String(), "command=--version") {
		t.Errorf("stderr = %q, want the dispatched command name", stderr.String())
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"bogus", slog.LevelInfo},
	}
	for _, tt := range tests {
		if got := parseLogLevel(tt.in); got != tt.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNewLogger_FormatByCommand(t *testing.T) {
	var buf bytes.Buffer

	mcpLogger := newLogger([]string{"mcp"}, &buf, "info")
	mcpLogger.Info("test")
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Errorf("mcp logger output = %q, want JSON", buf.String())
	}

	buf.Reset()
	routeLogger := newLogger([]string{"route"}, &buf, "info")
	routeLogger.Info("test")
	if strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Errorf("route logger output = %q, want text, not JSON", buf.String())
	}
}

func TestNewLogger_CLIDefaultIsQuiet(t *testing.T) {
	var buf bytes.Buffer

	logger := newLogger([]string{"route"}, &buf, "")
	logger.Info("test")
	logger.Error("test")

	if buf.Len() != 0 {
		t.Errorf("CLI logger output with unset NAERYEO_LOG_LEVEL = %q, want empty", buf.String())
	}
}

func TestNewLogger_DebugFlagEnablesVerboseCLILogging(t *testing.T) {
	var buf bytes.Buffer

	// --debug must override the quiet CLI default (empty NAERYEO_LOG_LEVEL) and
	// emit at Debug level so per-request diagnostics reach stderr.
	logger := newLogger([]string{"route", "--from", "a", "--to", "b", "--debug"}, &buf, "")
	logger.Debug("diagnostic")

	if !strings.Contains(buf.String(), "diagnostic") {
		t.Errorf("logger output with --debug = %q, want the debug line to be emitted", buf.String())
	}
}

func TestHasDebugFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"double dash", []string{"route", "--debug"}, true},
		{"single dash", []string{"route", "-debug"}, true},
		{"absent", []string{"route", "--from", "a"}, false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasDebugFlag(tt.args); got != tt.want {
				t.Errorf("hasDebugFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
