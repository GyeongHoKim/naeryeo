package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// version is overwritten via -ldflags at build time (see .goreleaser.yml).
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches to the naeryeo subcommands. It is split out from main so it
// can be exercised by tests without invoking os.Exit.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}

	switch args[0] {
	case "setup":
		return runSetup(args[1:], os.Stdin, stdout, stderr, config.Save)
	case "logout":
		return runLogout(args[1:], stdout, stderr, config.Load, config.Delete)
	case "route":
		return runRoute(args[1:], stdout, stderr, config.Load,
			func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
				return core.NewClient(apiKey).FindRoute(ctx, from, to)
			})
	case "mcp":
		// mcp.StdioTransport binds directly to the process's real
		// os.Stdin/os.Stdout — the stdout passed into run() is intentionally
		// not used here (research.md §3 of specs/003-mcp-route-server).
		server := buildMCPServer(version, config.Load,
			func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
				return core.NewClient(apiKey).FindRoute(ctx, from, to)
			})
		if err := runMCP(context.Background(), server); err != nil {
			if _, werr := fmt.Fprintf(stderr, "naeryeo mcp: %v\n", err); werr != nil {
				return 1
			}
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
