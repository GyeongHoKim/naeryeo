package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// coreErrorSamples pairs every exported error symbol in internal/core with a
// value classifyRouteError can be handed. Adding an error to core without
// adding it here fails TestErrorCodeExhaustive_EveryCoreErrorHasACode, which is
// the point: the compiler cannot tell us that a new failure mode has no code,
// so this table does (spec 005 FR-004, SC-004).
//
// Keep the keys spelled exactly as the identifiers in internal/core.
var coreErrorSamples = map[string]error{
	"ErrAPIKeyMissing":       core.ErrAPIKeyMissing,
	"ErrAuthFailed":          core.ErrAuthFailed,
	"ErrNoRoute":             core.ErrNoRoute,
	"ErrUpstreamUnavailable": core.ErrUpstreamUnavailable,
	"ErrGeocoderAuthFailed":  core.ErrGeocoderAuthFailed,
	"ErrGeocoderForbidden":   core.ErrGeocoderForbidden,
	"ErrGeocoderUnavailable": core.ErrGeocoderUnavailable,
	"ErrGeocoderRejected":    &core.ErrGeocoderRejected{Status: 400},
	"ErrPointNotFound":       &core.ErrPointNotFound{Side: "from", Name: "x"},
	"ErrUpstreamRejected":    &core.ErrUpstreamRejected{Code: "500", Message: "x"},
}

// notReachableFromPresentation lists exported core errors that deliberately
// have no code, with the reason. An entry here is a claim that the error can
// never reach a user-facing rendering — not a way to skip work.
var notReachableFromPresentation = map[string]string{
	// resolveStation folds this into *ErrPointNotFound before returning, so it
	// is part of the Geocoder interface's contract rather than a failure the
	// CLI or MCP server ever renders.
	"ErrGeocoderNotFound": "folded into *ErrPointNotFound by core.resolveStation",
}

// exportedCoreErrorNames parses internal/core and returns every exported error
// symbol: sentinels declared as `var ErrX = errors.New(...)` and struct types
// named ErrX that implement error.
//
// Parsing the source is the only option available — Go cannot enumerate a
// package's exported vars at runtime, and pulling in golang.org/x/tools just to
// get type information would add a dependency for a test-only check.
func exportedCoreErrorNames(t *testing.T) []string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", "internal", "core"))
	if err != nil {
		t.Fatalf("resolve internal/core: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read internal/core: %v", err)
	}

	var names []string
	seen := map[string]bool{}
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// ParseFile rather than the deprecated ParseDir; build tags do not
		// matter here because every non-test file in internal/core is compiled
		// on every platform.
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec: // var ErrX = errors.New(...)
					for _, n := range s.Names {
						if strings.HasPrefix(n.Name, "Err") && n.IsExported() {
							add(n.Name)
						}
					}
				case *ast.TypeSpec: // type ErrX struct { ... }
					if strings.HasPrefix(s.Name.Name, "Err") && s.Name.IsExported() {
						add(s.Name.Name)
					}
				}
			}
		}
	}

	if len(names) == 0 {
		t.Fatal("found no exported error symbols in internal/core; the parser is broken, not core")
	}
	return names
}

// TestErrorCodeExhaustive_EveryCoreErrorHasACode is the gate spec 005 FR-004
// asks for. It fails in two directions: a core error with no sample, and a
// sample that falls through to internal_error.
func TestErrorCodeExhaustive_EveryCoreErrorHasACode(t *testing.T) {
	for _, name := range exportedCoreErrorNames(t) {
		if reason, ok := notReachableFromPresentation[name]; ok {
			t.Logf("%s: intentionally uncoded (%s)", name, reason)
			continue
		}

		sample, ok := coreErrorSamples[name]
		if !ok {
			t.Errorf(
				"core.%s has no entry in coreErrorSamples.\n"+
					"A new error reached internal/core without a user-facing code. Add it to\n"+
					"specs/005-structured-output-contract/contracts/error-codes.md, give it a\n"+
					"branch in classifyRouteError, and add a sample here. If it can never reach\n"+
					"the CLI or the MCP server, add it to notReachableFromPresentation with a reason.",
				name,
			)
			continue
		}

		got := classifyRouteError(sample, false)
		if got.Code == codeInternalError {
			t.Errorf(
				"core.%s classifies as %q — it needs its own code in classifyRouteError",
				name, codeInternalError,
			)
		}
		if got.Message == "" {
			t.Errorf("core.%s produced an empty Message", name)
		}
	}
}

// TestErrorCodeExhaustive_SamplesStayInSyncWithCore catches the reverse drift:
// a sample or allowlist entry left behind after an error is renamed or removed.
func TestErrorCodeExhaustive_SamplesStayInSyncWithCore(t *testing.T) {
	live := map[string]bool{}
	for _, name := range exportedCoreErrorNames(t) {
		live[name] = true
	}

	for name := range coreErrorSamples {
		if !live[name] {
			t.Errorf("coreErrorSamples has %q, which no longer exists in internal/core", name)
		}
	}
	for name := range notReachableFromPresentation {
		if !live[name] {
			t.Errorf("notReachableFromPresentation has %q, which no longer exists in internal/core", name)
		}
	}
}
