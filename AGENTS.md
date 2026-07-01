# AGENTS.md

Instructions for coding agents (Claude Code, Codex, Cursor, etc.) working in this repository. The binding source of truth for these rules is `.specify/memory/constitution.md`; this file is a working summary for day-to-day agent use. If the two ever disagree, the constitution wins.

## Project Overview

naeryeo ("내려") is a Go CLI + MCP server that answers natural-language questions about Korean public-transit routes (bus/subway/inter-city), wrapping the ODsay API. It exposes both a CLI (`naeryeo route ...`) and an MCP stdio server (`naeryeo mcp`) for Claude Desktop / Claude Code, sharing the same core routing logic. See `README.md` for the full user-facing product description.

**Current state**: tooling scaffolding is in place (`go.mod`, `justfile`, `.golangci.yml`, `mise.toml`, `commitlint.config.js`/husky, `.goreleaser.yml`, CI), plus a minimal `cmd/naeryeo` stub and placeholder `internal/core`/`internal/config` packages. The real ODsay client and OS-keychain integration are not implemented yet. `.specify/` holds the spec-kit workflow (spec → plan → tasks) driving that work.

## Setup Commands

Run `mise install` once to fetch the pinned toolchain (go, node, just, golangci-lint, goreleaser). The required task-runner entrypoints are:

- `go mod tidy` — sync dependencies
- `just fmt` — format via `golangci-lint fmt` (v2 formatters: gofmt/goimports)
- `just lint` — lint via `golangci-lint run` against a v2-schema `.golangci.yml`
- `just test` — `go test ./...` (with `-race` where feasible)
- `just check` — aggregate recipe running all three above

Do not create ad-hoc shell scripts that duplicate these gates — `just` is the single entrypoint.

## Code Style

- Idiomatic Go per Effective Go. Small interfaces defined by the consuming package, not the implementer. Composition over embedding-as-inheritance.
- Handle every error explicitly — never ignore an error return. Never use `panic` for expected or recoverable errors.
- `gofmt`-formatted only; this is the sole accepted style.
- No speculative abstractions — an abstraction must justify its complexity before it's introduced.

(Full rationale: constitution.md, Principle I — non-negotiable.)

## Testing Instructions

- Every new exported function, method, and package ships with unit tests in the same commit that introduces it.
- Prefer table-driven tests; cover the happy path and meaningful edge cases.
- Run `just test` (or `just check`) before considering any change complete. A failing gate blocks completion — never note it and move on.
- Test coverage must not regress without an explicit, written justification in the commit/PR description.

(Full rationale: constitution.md, Principle II — non-negotiable.)

## Required Workflow for Any Change

1. Implement the change.
2. Run `just fmt`, then `just lint`, then `just test` (or `just check`). All three must be green.
3. Self-review against Principle I (idiomatic Go) and Principle II (test coverage).
4. Present the diff and a proposed Conventional Commits-style message to the human.
5. Commit only after explicit human confirmation of both the change and the exact message.
