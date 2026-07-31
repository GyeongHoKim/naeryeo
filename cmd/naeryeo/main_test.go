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

func TestNewLogger_DebugFlagPreservesJSONForMCP(t *testing.T) {
	var buf bytes.Buffer

	// --debug must not override the mcp JSON-format contract: the host captures
	// mcp stderr and parses it as structured JSON.
	logger := newLogger([]string{"mcp", "--debug"}, &buf, "")
	logger.Debug("diagnostic")

	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Errorf("logger output for mcp+--debug = %q, want JSON", buf.String())
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

// TestHasFlagNamed_ValueForms covers the -name=value forms. These matter for
// --json in particular: the prescan is the only thing runRoute consults, so a
// missed form silently downgrades machine output to prose.
func TestHasFlagNamed_ValueForms(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "bare long", args: []string{"--json"}, want: true},
		{name: "bare short", args: []string{"-json"}, want: true},
		{name: "explicit true", args: []string{"--json=true"}, want: true},
		{name: "explicit 1", args: []string{"--json=1"}, want: true},
		{name: "explicit false", args: []string{"--json=false"}, want: false},
		{name: "explicit 0", args: []string{"--json=0"}, want: false},
		{name: "short explicit false", args: []string{"-json=false"}, want: false},
		// flag.Parse rejects these; reporting that rejection as JSON is more
		// useful to a caller who clearly asked for it than falling back to prose.
		{name: "empty value", args: []string{"--json="}, want: true},
		{name: "garbage value", args: []string{"--json=maybe"}, want: true},
		// flag.Parse aborts at the malformed token, so a later valid value is
		// never reached and must not overrule it.
		{name: "garbage then false", args: []string{"--json=maybe", "--json=false"}, want: true},
		{name: "false then garbage", args: []string{"--json=false", "--json=maybe"}, want: true},
		// Among valid values the last occurrence wins, as in the flag package.
		{name: "false then bare", args: []string{"--json=false", "--json"}, want: true},
		{name: "bare then false", args: []string{"--json", "--json=false"}, want: false},
		// Must not match a different flag that merely shares a prefix.
		{name: "unrelated prefix", args: []string{"--jsonify"}, want: false},
		{name: "absent", args: []string{"--from", "강남역"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasJSONFlag(tt.args); got != tt.want {
				t.Errorf("hasJSONFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
			// The same helper backs --debug, so the forms must behave identically.
			debugArgs := make([]string, len(tt.args))
			for i, a := range tt.args {
				debugArgs[i] = strings.ReplaceAll(a, "json", "debug")
			}
			if got := hasDebugFlag(debugArgs); got != tt.want {
				t.Errorf("hasDebugFlag(%v) = %v, want %v", debugArgs, got, tt.want)
			}
		})
	}
}

// TestLogoutCommandIsGone pins the breaking change: the delete-only command
// was folded into setup, so invoking it must fail loudly rather than silently
// doing nothing, and the usage line must not advertise it (spec 006 FR-035).
func TestLogoutCommandIsGone(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"logout"}, &stdout, &stderr, discardLogger)

	if code == 0 {
		t.Fatal("code = 0, want non-zero for a removed command")
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("stderr = %q, want an unknown-command error", stderr.String())
	}
	if strings.Contains(stderr.String(), "logout|") || strings.Contains(stderr.String(), "|logout") {
		t.Errorf("usage still advertises logout:\n%s", stderr.String())
	}
}

func TestUsageListsTheCurrentCommands(t *testing.T) {
	var usage bytes.Buffer
	printUsage(&usage)

	if strings.Contains(usage.String(), "logout") {
		t.Errorf("usage mentions the removed logout command: %q", usage.String())
	}
	for _, want := range []string{"setup", "route", "mcp", "--version"} {
		if !strings.Contains(usage.String(), want) {
			t.Errorf("usage = %q, want it to list %q", usage.String(), want)
		}
	}
}
