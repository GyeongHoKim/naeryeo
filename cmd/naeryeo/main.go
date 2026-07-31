package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
	"github.com/GyeongHoKim/naeryeo/internal/geocode"
	"github.com/GyeongHoKim/naeryeo/internal/motis"
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

	switch args[0] {
	case "setup":
		return runSetup(args[1:], os.Stdin, stdout, stderr, setupDeps{
			SaveCredential:   config.Save,
			LoadCredential:   config.Load,
			DeleteCredential: config.Delete,
			SaveSettings:     config.SaveSettings,
			ProbeMotis:       probeMotis,
		})
	case "route":
		return runRoute(args[1:], stdout, stderr, newProviderResolver(logger), geocoderConfigured)
	case "mcp":
		// mcp.StdioTransport binds directly to the process's real
		// os.Stdin/os.Stdout — the stdout passed into run() is intentionally
		// not used here (research.md §3 of specs/003-mcp-route-server).
		server := buildMCPServer(version, logger, newProviderResolver(logger), geocoderConfigured)
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

// routeFinder searches a route with the provider already chosen and its
// credentials already resolved. Both engines' FindRoute methods have exactly
// this shape, so each satisfies it as a method value with no adapter.
//
// It is a function type rather than an interface because the consumer needs
// exactly one operation. Declaring an interface in internal/core would also
// invert the dependency the constitution asks for — the consuming package
// defines the abstraction, and here the consumer is this one.
type routeFinder func(ctx context.Context, from, to string) (core.RouteResult, error)

// providerResolver yields the finder for this run, or the failure that
// prevents one from existing — no provider configured, no key stored, an
// unreadable keychain. Resolving before the search is what lets those be
// reported with the same coded machinery as a search failure.
type providerResolver func() (routeFinder, *failure)

// newProviderResolver reads the stored settings and builds the matching
// client. The route and mcp entry points share one of these, which is what
// makes it structurally impossible for the two to disagree about which engine
// answered (spec 006 FR-002).
//
// A stored credential never implies a provider: with no settings file this
// returns provider_not_configured even when an ODsay key is sitting in the
// keychain. Inferring the provider would put a permanent exception into the
// state model to smooth over a one-time migration (spec 006 FR-031).
func newProviderResolver(logger *slog.Logger) providerResolver {
	return func() (routeFinder, *failure) {
		settings, err := config.LoadSettings()
		if err != nil {
			f := providerNotConfiguredFailure()
			return nil, &f
		}

		geocoder := newGeocoder(settings, logger)

		switch settings.RoutingProvider {
		case config.ProviderMotis:
			client := motis.NewClient(settings.MotisURL)
			client.Logger = logger
			client.Geocoder = geocoder
			return client.FindRoute, nil

		case config.ProviderODsay:
			key, keyErr := config.Load(config.ODsayAPIKey)
			switch {
			case errors.Is(keyErr, config.ErrNotConfigured):
				f := classifyRouteError(core.ErrAPIKeyMissing, geocoder != nil)
				return nil, &f
			case keyErr != nil:
				f := credentialStoreFailure()
				return nil, &f
			}
			client := core.NewClient(key)
			client.Logger = logger
			client.Geocoder = geocoder
			return client.FindRoute, nil

		default:
			f := providerNotConfiguredFailure()
			return nil, &f
		}
	}
}

// newGeocoder returns the optional place-search backend, or nil when none is
// configured. It is an axis independent of the routing provider: every
// provider/geocoder combination is valid, and a nil result simply means names
// resolve only as far as the routing engine's own index reaches (spec 006
// FR-030).
func newGeocoder(settings config.Settings, logger *slog.Logger) core.Geocoder {
	if settings.Geocoder != config.GeocoderKakao {
		return nil
	}
	key, err := config.Load(config.GeocoderAPIKey)
	if err != nil || key == "" {
		logger.Warn("naeryeo: geocoder selected but no key stored; place search is disabled")
		return nil
	}
	kakao := geocode.NewKakao(key)
	kakao.Logger = logger
	return kakao
}

// geocoderConfigured reports whether place search is actually usable. The
// point_not_found hint ("set up a geocoder") is only worth showing when doing
// so would change the outcome (spec 004 FR-007).
func geocoderConfigured() bool {
	settings, err := config.LoadSettings()
	if err != nil || settings.Geocoder != config.GeocoderKakao {
		return false
	}
	key, keyErr := config.Load(config.GeocoderAPIKey)
	return keyErr == nil && key != ""
}

// probeMotis checks that a candidate MOTIS endpoint is reachable AND has data
// loaded, so setup can refuse an address that would fail at every later
// search. See setup.go for why this lives at configuration time rather than
// on the search path.
func probeMotis(ctx context.Context, baseURL string) error {
	client := motis.NewClient(baseURL)
	return client.Probe(ctx)
}

func printUsage(w io.Writer) {
	if _, err := fmt.Fprintln(w, "usage: naeryeo <setup|route|mcp|--version>"); err != nil {
		return
	}
}
