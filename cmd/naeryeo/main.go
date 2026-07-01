package main

import (
	"fmt"
	"io"
	"os"

	"github.com/GyeongHoKim/naeryeo/internal/config"
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
		return notImplemented(stderr, "route")
	case "mcp":
		return notImplemented(stderr, "mcp")
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

// notImplemented reports that cmd is a recognized but unfinished subcommand.
func notImplemented(w io.Writer, cmd string) int {
	if _, err := fmt.Fprintf(w, "naeryeo %s: not yet implemented\n", cmd); err != nil {
		return 1
	}
	return 1
}

func printUsage(w io.Writer) {
	if _, err := fmt.Fprintln(w, "usage: naeryeo <setup|logout|route|mcp|--version>"); err != nil {
		return
	}
}
