<!--
Sync Impact Report
Version change: [TEMPLATE] → 1.0.0 (initial ratification)
Modified principles: n/a (first concrete fill of template placeholders)
Added sections:
  - I. Idiomatic Go First (NON-NEGOTIABLE)
  - II. Unit Tests Are Mandatory (NON-NEGOTIABLE)
  - III. Automated Quality Gates (just + golangci-lint v2)
  - IV. Commit Discipline via commitlint (NON-NEGOTIABLE)
  - Technology Stack & Tooling Requirements
  - Development Workflow & Quality Gates
  - Governance
Removed sections: none (template placeholders replaced)
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ no change needed (Constitution Check gate is generic and reads from this file at plan time)
  - .specify/templates/spec-template.md ✅ no change needed (feature-level template, not principle-specific)
  - .specify/templates/tasks-template.md ⚠ updated (test tasks were marked OPTIONAL; now MANDATORY per Principle II; Setup phase now references just/golangci-lint v2/commitlint)
  - .specify/templates/checklist-template.md ✅ no change needed (generic checklist scaffold)
Follow-up TODOs: none
-->

# naeryeo Constitution

## Core Principles

### I. Idiomatic Go First (NON-NEGOTIABLE)

All code MUST follow Effective Go conventions. Interfaces MUST be small and defined by
the consuming package rather than the implementing package. Composition MUST be
preferred over embedding-as-inheritance. Errors MUST be handled explicitly: ignoring an
error return is prohibited, and `panic` MUST NOT be used for expected or recoverable
error conditions. All code MUST be `gofmt`-formatted; this is the only accepted style.
An abstraction MUST justify its complexity cost before introduction — speculative
generality MUST be rejected in favor of the simplest design that solves the problem at
hand.

**Rationale**: Idiomatic Go is treated as a non-negotiable value, not a stylistic
preference. Consistent, simple, explicit code keeps the codebase approachable for any
Go contributor and prevents accumulation of clever-but-fragile abstractions.

### II. Unit Tests Are Mandatory (NON-NEGOTIABLE)

Every new exported function, method, and package MUST ship with unit tests in the same
commit that introduces it. Table-driven tests are the preferred pattern for covering
multiple cases. A feature is not "done" until its tests cover both the happy path and
meaningful edge cases. Test coverage MUST NOT regress without an explicit, written
justification recorded in the commit or PR description.

**Rationale**: Tests are the mechanism that makes refactors and agent-driven changes
safe. Treating them as optional invites silent regressions that are expensive to trace
back later.

### III. Automated Quality Gates (just + golangci-lint v2)

The project uses `just` (the Rust-based task runner) as the single entrypoint for
quality checks. The repository's `justfile` MUST define at minimum:

- `fmt` → runs `golangci-lint fmt` (v2 formatters, e.g. gofmt/goimports)
- `lint` → runs `golangci-lint run` against a v2-schema config (`version: "2"` in
  `.golangci.yml`, with `formatters` and `linters` declared as separate sections)
- `test` → runs `go test ./...` (with `-race` where feasible)

A coding agent MUST run `just fmt`, `just lint`, and `just test` (or an aggregate
`just check` recipe that runs all three) after any code change. All three MUST pass
before the agent may consider a task complete or propose a commit. A failing gate
blocks completion — it is never treated as a warning to note and move past.

**Rationale**: Mechanical, tool-enforced gates are more reliable than review-time
judgment calls, and they let a coding agent verify its own work without waiting on a
human for routine checks.

### IV. Commit Discipline via commitlint (NON-NEGOTIABLE)

All commit messages MUST conform to Conventional Commits, enforced by commitlint
(`commitlint.config.js` extending `@commitlint/config-conventional`). A coding agent
MUST NOT create a commit on its own initiative or without explicit human confirmation
of both the change and the exact commit message. Any proposed commit message MUST be
validated against commitlint rules before it is finalized.

**Rationale**: This guards against an agent overreaching into version-control actions
that are hard to reverse, while keeping commit history and changelog generation clean
and machine-parseable.

## Technology Stack & Tooling Requirements

- **Primary language**: Go. The version is pinned in `go.mod`; introducing a second
  general-purpose language requires a constitution amendment.
- **Task runner**: `just`. The `justfile` at the repository root is the single
  entrypoint for `fmt`/`lint`/`test`; ad-hoc shell scripts duplicating these gates are
  not permitted.
- **Linter/formatter**: `golangci-lint` v2. `.golangci.yml` MUST declare
  `version: "2"`; formatters (`gofmt`, `goimports`, etc.) MUST live under the
  `formatters:` section, never under `linters:`.
- **Commit linting**: `commitlint` + `@commitlint/config-conventional`, configured via
  `commitlint.config.js` at the repository root.

## Development Workflow & Quality Gates

The required sequence for any code change is:

1. Implement the change.
2. Run `just fmt`, then `just lint`, then `just test` (or `just check`).
3. Only once all three are green, present the diff and a proposed Conventional
   Commits-style message to the human.
4. Commit only after explicit human confirmation of the change and the message.

Self-review before proposing completion MUST verify compliance with Principle I
(idiomatic Go) and Principle II (test coverage). Any deviation from this sequence —
including skipping a gate — MUST be surfaced explicitly to the human, never silently
skipped.

## Governance

This constitution supersedes ad-hoc practices; where other project documentation
conflicts with it, this document governs. Amendments require explicit approval from
the project owner — no principle may be silently reinterpreted or waived by a coding
agent. Versioning follows semantic versioning: MAJOR for backward-incompatible removal
or redefinition of a principle, MINOR for adding a new principle or materially
expanding existing guidance, PATCH for wording or clarification changes. Every
`/speckit.plan` and `/speckit.analyze` run MUST verify that the fmt/lint/test gates and
the commitlint guardrail are actually wired into the plan and tasks, not merely
assumed. All commits MUST be consistent with the principles above; any complexity that
appears to violate Principle I MUST be justified in writing or rejected.

**Version**: 1.0.0 | **Ratified**: 2026-07-01 | **Last Amended**: 2026-07-01
