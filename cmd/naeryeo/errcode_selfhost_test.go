package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// TestSelfHostingDocsURL_PathExists keeps the docs link in the error contract
// honest. The URL is published to users and AI callers, so the document it
// names has to be where it claims — a rename that silently 404s every
// self-hosting failure would otherwise pass every other test in the suite.
func TestSelfHostingDocsURL_PathExists(t *testing.T) {
	const wantSuffix = "/docs/self-hosting.md"
	if !strings.HasSuffix(selfHostingDocsURL, wantSuffix) {
		t.Fatalf("selfHostingDocsURL = %q, want it to end in %q", selfHostingDocsURL, wantSuffix)
	}

	repoRelative := strings.TrimPrefix(wantSuffix, "/")
	path, err := filepath.Abs(filepath.Join("..", "..", repoRelative))
	if err != nil {
		t.Fatalf("resolve %s: %v", repoRelative, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("selfHostingDocsURL points at %s, but that file does not exist in the repo: %v",
			repoRelative, err)
	}
}

func TestClassifyRouteError_SelfHostingCodes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode errorCode
		wantDocs bool
	}{
		{
			name:     "unreachable engine is retryable",
			err:      core.ErrMotisUnavailable,
			wantCode: codeMotisUnavailable,
			wantDocs: true,
		},
		{
			name:     "wrapped unreachable engine still classifies",
			err:      errors.New("wrapped: " + core.ErrMotisUnavailable.Error()),
			wantCode: codeInternalError, // a lookalike string must NOT match
			wantDocs: false,
		},
		{
			name:     "engine rejected the request",
			err:      &core.ErrMotisRejected{Status: 400},
			wantCode: codeMotisRejected,
			wantDocs: true,
		},
		{
			name:     "undecodable response classifies as rejected",
			err:      &core.ErrMotisRejected{},
			wantCode: codeMotisRejected,
			wantDocs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRouteError(tt.err, false)
			if got.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", got.Code, tt.wantCode)
			}
			if got.Message == "" {
				t.Error("Message is empty; every failure must carry a reason")
			}
			if tt.wantDocs && got.Docs != selfHostingDocsURL {
				t.Errorf("Docs = %q, want %q", got.Docs, selfHostingDocsURL)
			}
			if !tt.wantDocs && got.Docs != "" {
				t.Errorf("Docs = %q, want empty", got.Docs)
			}
		})
	}
}

func TestProviderNotConfiguredFailure(t *testing.T) {
	f := providerNotConfiguredFailure()

	if f.Code != codeProviderNotConfigured {
		t.Fatalf("Code = %q, want %q", f.Code, codeProviderNotConfigured)
	}
	if f.Code == codeAPIKeyMissing {
		t.Fatal("provider_not_configured must stay distinct from api_key_missing")
	}
	if !strings.Contains(f.Hint, "naeryeo setup") {
		t.Errorf("Hint = %q, want it to name the setup command", f.Hint)
	}
	if f.Docs != selfHostingDocsURL {
		t.Errorf("Docs = %q, want %q", f.Docs, selfHostingDocsURL)
	}
}

// TestProse_DocsRendersAsThirdLine pins the prose layout the self-hosting
// codes add, and — more importantly — pins that codes without Docs are
// unchanged. The second half is what keeps spec 005 FR-007 true.
func TestProse_DocsRendersAsThirdLine(t *testing.T) {
	t.Run("message, hint and docs render on three lines", func(t *testing.T) {
		got := failure{Message: "이유", Hint: "조치", Docs: "https://example.test/doc"}.Prose()
		want := "이유\n조치\nhttps://example.test/doc"
		if got != want {
			t.Fatalf("Prose() = %q, want %q", got, want)
		}
	})

	t.Run("docs without hint still renders on its own line", func(t *testing.T) {
		got := failure{Message: "이유", Docs: "https://example.test/doc"}.Prose()
		want := "이유\nhttps://example.test/doc"
		if got != want {
			t.Fatalf("Prose() = %q, want %q", got, want)
		}
	})

	t.Run("failures without docs are byte-identical to before", func(t *testing.T) {
		if got, want := (failure{Message: "이유"}).Prose(), "이유"; got != want {
			t.Fatalf("Prose() = %q, want %q", got, want)
		}
		if got, want := (failure{Message: "이유", Hint: "조치"}).Prose(), "이유\n조치"; got != want {
			t.Fatalf("Prose() = %q, want %q", got, want)
		}
	})
}

// TestSelfHostingFailures_HideNetworkDetail is the FR-018 guard at the
// classification layer: whatever the engine or the transport said, the user-
// facing failure must not carry the operator's host, port, or path.
func TestSelfHostingFailures_HideNetworkDetail(t *testing.T) {
	const host = "motis.internal.example"
	const port = "18080"

	errs := []error{
		core.ErrMotisUnavailable,
		&core.ErrMotisRejected{Status: 503},
	}

	for _, err := range errs {
		f := classifyRouteError(err, false)
		rendered := f.Prose() + " " + f.Message + " " + f.Hint + " " + f.Docs
		for _, needle := range []string{host, port} {
			if strings.Contains(rendered, needle) {
				t.Errorf("failure for %v leaked %q into user-facing output: %q", err, needle, rendered)
			}
		}
	}
}
