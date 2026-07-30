package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
	"github.com/GyeongHoKim/naeryeo/internal/geocode"
)

// version is overwritten via -ldflags at build time (see .goreleaser.yml).
var version = "dev"

func main() {
	logger := newLogger(os.Args[1:], os.Stderr, os.Getenv("NAERYEO_LOG_LEVEL"))
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, logger))
}

// newLogger builds the process-wide logger. It always writes to w (stderr
// in production) — stdout is reserved for user-facing CLI output and, for
// the mcp subcommand, the MCP JSON-RPC stream itself, so logs must never go
// there. CLI logs are quiet by default unless NAERYEO_LOG_LEVEL is set.
// MCP logs default to Info because stderr is captured by the MCP host.
func newLogger(args []string, w io.Writer, level string) *slog.Logger {
	// A --debug flag on any subcommand (e.g. `naeryeo route ... --debug`)
	// forces verbose logging to stderr so the per-request diagnostics
	// (geocoder URL, HTTP status, provider error body) become visible without
	// setting NAERYEO_LOG_LEVEL. It overrides the quiet CLI default below, but
	// must still honor the per-command format contract — mcp stays JSON (its
	// stderr is captured by the MCP host) while other commands use text.
	if hasDebugFlag(args) {
		opts := &slog.HandlerOptions{Level: slog.LevelDebug}
		if isMCPCommand(args) {
			return slog.New(slog.NewJSONHandler(w, opts))
		}
		return slog.New(slog.NewTextHandler(w, opts))
	}
	if strings.TrimSpace(level) == "" && !isMCPCommand(args) {
		return slog.New(slog.DiscardHandler)
	}

	opts := &slog.HandlerOptions{Level: parseLogLevel(level)}

	var handler slog.Handler
	if isMCPCommand(args) {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}
	return slog.New(handler)
}

func isMCPCommand(args []string) bool {
	return len(args) > 0 && args[0] == "mcp"
}

// hasDebugFlag reports whether a --debug (or -debug) flag appears anywhere in
// args. It is scanned here, before subcommand flag parsing, because the logger
// is built up front in main; the route subcommand also registers --debug in
// its own FlagSet so the flag parses cleanly and appears in its usage.
func hasDebugFlag(args []string) bool {
	return hasFlagNamed(args, "debug")
}

// hasJSONFlag reports whether --json (or -json) appears anywhere in args. Like
// hasDebugFlag it has to be answered BEFORE the subcommand's FlagSet parses,
// because a parse failure is itself something --json must report as a JSON
// document — by the time flag.Parse returns an error it has already written
// usage text to its output (spec 005 FR-015, research.md R4).
func hasJSONFlag(args []string) bool {
	return hasFlagNamed(args, "json")
}

// hasFlagNamed reports whether the boolean flag name is enabled anywhere in
// args, accepting every form the flag package does: --name, -name, and
// --name=value / -name=value.
//
// The =value forms matter because this scan runs before flag.Parse and is the
// only thing consulted for --json; matching only the bare form would silently
// ignore --json=true and fall back to prose. Among valid values a later
// occurrence wins, mirroring flag's own last-one-wins behavior.
//
// An unparseable value (--json=, --json=x) returns immediately rather than
// letting a later token overrule it: flag.Parse aborts on the first malformed
// value, so nothing after it can change the outcome — the command is going to
// fail, and reporting that failure in the format the caller asked for beats
// falling back to prose.
func hasFlagNamed(args []string, name string) bool {
	found := false
	for _, a := range args {
		var value string
		switch {
		case a == "--"+name, a == "-"+name:
			found = true
			continue
		case strings.HasPrefix(a, "--"+name+"="):
			value = strings.TrimPrefix(a, "--"+name+"=")
		case strings.HasPrefix(a, "-"+name+"="):
			value = strings.TrimPrefix(a, "-"+name+"=")
		default:
			continue
		}
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return true
		}
		found = enabled
	}
	return found
}

// parseLogLevel parses NAERYEO_LOG_LEVEL, defaulting to Info.
func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// run dispatches to the naeryeo subcommands. It is split out from main so it
// can be exercised by tests without invoking os.Exit.
func run(args []string, stdout, stderr io.Writer, logger *slog.Logger) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}

	config.SetLogger(logger)
	logger.Info("naeryeo: dispatch", "command", args[0])

	loadODsay := func() (string, error) { return config.Load(config.ODsayAPIKey) }
	loadGeocoder := func() (string, error) { return config.Load(config.GeocoderAPIKey) }

	switch args[0] {
	case "setup":
		return runSetup(args[1:], os.Stdin, stdout, stderr, config.Save)
	case "logout":
		return runLogout(args[1:], stdout, stderr, config.Load, config.Delete)
	case "route":
		return runRoute(args[1:], stdout, stderr, loadODsay, loadGeocoder, newFindRoute(logger))
	case "mcp":
		// mcp.StdioTransport binds directly to the process's real
		// os.Stdin/os.Stdout — the stdout passed into run() is intentionally
		// not used here (research.md §3 of specs/003-mcp-route-server).
		server := buildMCPServer(version, logger, loadODsay, loadGeocoder, newFindRoute(logger))
		if err := runMCP(context.Background(), server); err != nil {
			logger.Error("mcp: server exited with error", "error", err)
			return 1
		}
		return 0
	case "--version":
		if _, err := fmt.Fprintln(stdout, version); err != nil {
			return 1
		}
		return 0
	default:
		if _, err := fmt.Fprintf(stderr, "naeryeo: unknown command %q\n", args[0]); err != nil {
			return 1
		}
		printUsage(stderr)
		return 1
	}
}

// newFindRoute builds the route-search function shared by the route and mcp
// entry points. It constructs a core.Client for the ODsay key and, if a
// geocoder key is configured, injects a Kakao geocoder so that From/To names
// ODsay's station search does not recognize (building names, addresses) are
// resolved via the fallback. When no geocoder key is stored the client's
// Geocoder stays nil and behavior is unchanged (spec 004 FR-012).
func newFindRoute(logger *slog.Logger) func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
	return func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		client := core.NewClient(apiKey)
		client.Logger = logger
		if gk, err := config.Load(config.GeocoderAPIKey); err == nil && gk != "" {
			kakao := geocode.NewKakao(gk)
			kakao.Logger = logger
			client.Geocoder = kakao
		}
		return client.FindRoute(ctx, from, to)
	}
}

func printUsage(w io.Writer) {
	if _, err := fmt.Fprintln(w, "usage: naeryeo <setup|logout|route|mcp|--version>"); err != nil {
		return
	}
}
