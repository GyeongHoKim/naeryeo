package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// TestFindTransitRouteTool_FailureCarriesStructuredCode is the end-to-end guard
// for contracts/mcp-tool.md. It matters more than it looks: the SDK's
// ToolHandlerFor wrapper DISCARDS a hand-built CallToolResult whenever the
// handler also returns an error, replacing it with SetError(err) over an empty
// result. That path drops structuredContent and puts the raw error text in
// content — exactly the leak this feature removes. If a future SDK bump or a
// refactor reintroduces an error return here, this test fails.
func TestFindTransitRouteTool_FailureCarriesStructuredCode(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		geocoderKey func() (string, error)
		wantCode    errorCode
	}{
		{
			name:        "rate limited",
			err:         &core.ErrGeocoderRejected{Status: http.StatusTooManyRequests},
			geocoderKey: loadGeoPresent,
			wantCode:    codeGeocoderRateLimited,
		},
		{
			name:        "rejected input",
			err:         &core.ErrGeocoderRejected{Status: http.StatusBadRequest},
			geocoderKey: loadGeoPresent,
			wantCode:    codeGeocoderRejected,
		},
		{
			name:        "no route",
			err:         core.ErrNoRoute,
			geocoderKey: loadGeoPresent,
			wantCode:    codeNoRoute,
		},
		{
			name:        "point not found",
			err:         &core.ErrPointNotFound{Side: "from", Name: "아이디스 타워"},
			geocoderKey: loadGeoAbsent,
			wantCode:    codePointNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := buildMCPServer("test", discardLogger, okLoad, tt.geocoderKey, failingRoute(tt.err))
			session := connectTestClient(t, server)
			res := callFindTransitRoute(t, session, "출발", "도착")

			if !res.IsError {
				t.Fatal("IsError = false, want true")
			}
			if res.StructuredContent == nil {
				t.Fatal("StructuredContent is nil; the failure code never reached the caller")
			}
			out := decodeRouteToolOutput(t, res)
			if out.Error == nil {
				t.Fatal("structuredContent has no error object")
			}
			if out.Error.Code != string(tt.wantCode) {
				t.Errorf("Code = %q, want %q", out.Error.Code, tt.wantCode)
			}
			// The text content stays human-readable so a client that only
			// renders text still shows something useful.
			if got := resultText(res); got != out.Error.Message && !strings.Contains(got, out.Error.Message) {
				t.Errorf("content text = %q, want it to carry the message %q", got, out.Error.Message)
			}
		})
	}
}

// TestFindTransitRouteTool_FailureMatchesCLICodeAndMessage extends the pre-005
// wording-parity guarantee to the code (spec 005 FR-016, SC-005). Both entry
// points derive from classifyRouteError, so a divergence here means someone
// bypassed it.
func TestFindTransitRouteTool_FailureMatchesCLICodeAndMessage(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		geocoderKey func() (string, error)
		configured  bool
	}{
		{
			name:        "point not found without geocoder",
			err:         &core.ErrPointNotFound{Side: "from", Name: "아이디스 타워"},
			geocoderKey: loadGeoAbsent,
			configured:  false,
		},
		{
			name:        "point not found with geocoder",
			err:         &core.ErrPointNotFound{Side: "to", Name: "수지구청"},
			geocoderKey: loadGeoPresent,
			configured:  true,
		},
		{
			name:        "geocoder auth failure",
			err:         core.ErrGeocoderAuthFailed,
			geocoderKey: loadGeoPresent,
			configured:  true,
		},
		{
			name:        "geocoder forbidden",
			err:         core.ErrGeocoderForbidden,
			geocoderKey: loadGeoPresent,
			configured:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// What the CLI would produce for the same failure.
			var stdout, stderr bytes.Buffer
			cliCode := runRoute(
				[]string{"--from", "출발", "--to", "도착", "--json"},
				&stdout, &stderr, okLoad, tt.geocoderKey, failingRoute(tt.err),
			)
			if cliCode != 1 {
				t.Fatalf("CLI code = %d, want 1", cliCode)
			}
			cli := decodeEnvelope(t, stdout.String())
			if cli.Error == nil {
				t.Fatal("CLI produced no error object")
			}

			// What the MCP tool produces.
			server := buildMCPServer("test", discardLogger, okLoad, tt.geocoderKey, failingRoute(tt.err))
			session := connectTestClient(t, server)
			res := callFindTransitRoute(t, session, "출발", "도착")
			mcpOut := decodeRouteToolOutput(t, res)
			if mcpOut.Error == nil {
				t.Fatal("MCP produced no error object")
			}

			if cli.Error.Code != mcpOut.Error.Code {
				t.Errorf("code drift: CLI = %q, MCP = %q", cli.Error.Code, mcpOut.Error.Code)
			}
			if cli.Error.Message != mcpOut.Error.Message {
				t.Errorf("message drift:\nCLI = %q\nMCP = %q", cli.Error.Message, mcpOut.Error.Message)
			}
			if cli.Error.Hint != mcpOut.Error.Hint {
				t.Errorf("hint drift: CLI = %q, MCP = %q", cli.Error.Hint, mcpOut.Error.Hint)
			}
			// And the prose both render must still agree with routeErrorMessage.
			if want := routeErrorMessage(tt.err, tt.configured); resultText(res) != want {
				t.Errorf("MCP content = %q, want the shared prose %q", resultText(res), want)
			}
		})
	}
}

// TestFindTransitRouteTool_SuccessHasNoErrorKey keeps the envelope's success
// half clean: callers decide success by the absence of "error" alone.
func TestFindTransitRouteTool_SuccessHasNoErrorKey(t *testing.T) {
	findRoute := func(context.Context, string, string, string) (core.RouteResult, error) {
		return core.RouteResult{TotalTime: 42, TransferCount: 1, Fare: 1500}, nil
	}
	server := buildMCPServer("test", discardLogger, okLoad, loadGeoPresent, findRoute)
	session := connectTestClient(t, server)
	res := callFindTransitRoute(t, session, "강남역", "홍대입구역")

	if res.IsError {
		t.Fatal("IsError = true, want false")
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte(`"error"`)) {
		t.Errorf("success document contains an error key: %s", raw)
	}
}
