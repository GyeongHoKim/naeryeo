package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
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
// there. Output is JSON for the mcp subcommand (consumed by Claude
// Desktop/Code's log capture or other tooling) and human-readable text
// otherwise. level defaults to Info if unset or unparseable.
func newLogger(args []string, w io.Writer, level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLogLevel(level)}

	var handler slog.Handler
	if len(args) > 0 && args[0] == "mcp" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}
	return slog.New(handler)
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
		return runSetup(args[1:], os.Stdin, stdout, stderr, config.Save)
	case "logout":
		return runLogout(args[1:], stdout, stderr, config.Load, config.Delete)
	case "route":
		return runRoute(args[1:], stdout, stderr, config.Load,
			func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
				client := core.NewClient(apiKey)
				client.Logger = logger
				return client.FindRoute(ctx, from, to)
			})
	case "mcp":
		// mcp.StdioTransport binds directly to the process's real
		// os.Stdin/os.Stdout — the stdout passed into run() is intentionally
		// not used here (research.md §3 of specs/003-mcp-route-server).
		server := buildMCPServer(version, logger, config.Load,
			func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
				client := core.NewClient(apiKey)
				client.Logger = logger
				return client.FindRoute(ctx, from, to)
			})
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

func printUsage(w io.Writer) {
	if _, err := fmt.Fprintln(w, "usage: naeryeo <setup|logout|route|mcp|--version>"); err != nil {
		return
	}
}
