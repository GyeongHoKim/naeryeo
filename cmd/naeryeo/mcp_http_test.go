package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GyeongHoKim/naeryeo/internal/core"
	"github.com/GyeongHoKim/naeryeo/internal/motis"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeRoute is the canonical successful RouteResult used across handler tests.
var fakeRoute = core.RouteResult{
	TotalTime:     39,
	TransferCount: 1,
	Steps: []core.RouteStep{
		{Description: "신논현까지 도보 13분"},
		{Description: "신논현에서 9호선 승차 → 당산 하차"},
	},
}

func okFinder(context.Context, string, string) (core.RouteResult, error) {
	return fakeRoute, nil
}

// callTool invokes the handler directly (bypassing HTTP) with a fake finder.
func callTool(t *testing.T, finder cloudRouteFinder, from, to string) (*mcp.CallToolResult, error) {
	t.Helper()
	handler := httpRouteToolHandler(nil, finder)
	result, _, err := handler(context.Background(), nil, cloudRouteInput{From: from, To: to})
	return result, err
}

func TestHTTPRouteToolHandlerSuccessReturnsMarkdownOnly(t *testing.T) {
	result, err := callTool(t, okFinder, "강남역", "홍대입구역")
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want *mcp.TextContent", result.Content[0])
	}
	for _, want := range []string{"**강남역 → 홍대입구역**", "약 39분", "환승 1회", "1. 신논현까지 도보 13분"} {
		if !strings.Contains(text.Text, want) {
			t.Errorf("markdown missing %q:\n%s", want, text.Text)
		}
	}
	if result.StructuredContent != nil {
		t.Errorf("StructuredContent = %v, want nil (markdown text only)", result.StructuredContent)
	}
}

func TestHTTPRouteToolHandlerInputValidation(t *testing.T) {
	long := strings.Repeat("가", maxPlaceNameLen+1)
	tests := []struct {
		name     string
		from, to string
		wantMsg  string
	}{
		{"empty from", "", "강남역", "출발지와 도착지를 모두"},
		{"whitespace to", "강남역", "   ", "출발지와 도착지를 모두"},
		{"overlong from", long, "강남역", "장소 이름이 너무 길어요"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finderCalled := false
			finder := func(context.Context, string, string) (core.RouteResult, error) {
				finderCalled = true
				return core.RouteResult{}, nil
			}
			_, err := callTool(t, finder, tt.from, tt.to)
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantMsg)
			}
			if finderCalled {
				t.Error("finder must not be called on invalid input")
			}
		})
	}
}

func TestCloudRouteErrorMessages(t *testing.T) {
	backendURL := "http://internal-motis:8080"
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "from not found",
			err:     &core.ErrPointNotFound{Side: "from", Name: "없는곳"},
			wantMsg: `출발지 "없는곳"을(를) 찾지 못했어요`,
		},
		{
			name:    "to not found",
			err:     &core.ErrPointNotFound{Side: "to", Name: "없는곳"},
			wantMsg: `도착지 "없는곳"을(를) 찾지 못했어요`,
		},
		{
			name:    "no route",
			err:     fmt.Errorf("plan: %w", motis.ErrNoRoute),
			wantMsg: "해당 구간의 대중교통 경로를 찾지 못했어요.",
		},
		{
			name:    "backend unavailable",
			err:     fmt.Errorf("geocode: %w: GET %s: HTTP 502", motis.ErrUnavailable, backendURL),
			wantMsg: "경로 서버가 일시적으로 응답하지 않아요",
		},
		{
			name:    "deadline exceeded",
			err:     context.DeadlineExceeded,
			wantMsg: "경로 서버가 일시적으로 응답하지 않아요",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloudRouteErrorMessage(tt.err)
			if !strings.Contains(got, tt.wantMsg) {
				t.Errorf("message = %q, want containing %q", got, tt.wantMsg)
			}
			// FR-009: internals must never leak into user-facing text.
			for _, leak := range []string{backendURL, "HTTP", "502", "motis:", "core:"} {
				if strings.Contains(got, leak) {
					t.Errorf("message leaks internal detail %q: %q", leak, got)
				}
			}
		})
	}
}

// TestCloudToolMetadataCompliance pins the PlayMCP dev-guide rules that
// cause review rejection (contracts/mcp-tool.md, SC-003).
func TestCloudToolMetadataCompliance(t *testing.T) {
	tools := listToolsViaHTTP(t)
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	tool := tools[0]

	name, _ := tool["name"].(string)
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`).MatchString(name) {
		t.Errorf("tool name %q violates [A-Za-z0-9_-]{1,128}", name)
	}

	desc, _ := tool["description"].(string)
	if utf8.RuneCountInString(desc) > 1024 {
		t.Errorf("description is %d runes, must be <= 1024", utf8.RuneCountInString(desc))
	}
	if !strings.Contains(desc, "naeryeo(내려)") {
		t.Errorf("description must carry the service name in both scripts: %q", desc)
	}

	for _, s := range []string{name, desc} {
		if strings.Contains(strings.ToLower(s), "kakao") {
			t.Errorf("%q must not contain \"kakao\" (case-insensitive)", s)
		}
	}

	ann, _ := tool["annotations"].(map[string]any)
	if ann == nil {
		t.Fatal("annotations missing — all five hints are mandatory")
	}
	wantHints := map[string]any{
		"title":           "Find Korean transit route",
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   true,
	}
	for key, want := range wantHints {
		got, present := ann[key]
		if !present {
			t.Errorf("annotations.%s missing — must be set explicitly", key)
			continue
		}
		if got != want {
			t.Errorf("annotations.%s = %v, want %v", key, got, want)
		}
	}

	if _, hasInput := tool["inputSchema"]; !hasInput {
		t.Error("inputSchema missing")
	}
	if _, hasOutput := tool["outputSchema"]; hasOutput {
		t.Error("outputSchema present — cloud tool returns markdown text only")
	}
}

// TestStatelessHTTPRoundTrip exercises the full Streamable HTTP path with
// raw JSON-RPC POSTs (no session header — stateless per the dev guide).
func TestStatelessHTTPRoundTrip(t *testing.T) {
	srv := httptest.NewServer(newHTTPMux(buildHTTPMCPServer("test", nil, okFinder), nil))
	t.Cleanup(srv.Close)

	var resp jsonRPCResponse
	postJSONRPC(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_transit_route","arguments":{"from":"강남역","to":"홍대입구역"}}}`, &resp)

	if resp.Error != nil {
		t.Fatalf("JSON-RPC error: %+v", resp.Error)
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	mustUnmarshal(t, resp.Result, &result)
	if result.IsError {
		t.Fatalf("isError = true, content: %+v", result.Content)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("content = %+v, want single text item", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, "약 39분") {
		t.Errorf("markdown missing duration: %s", result.Content[0].Text)
	}
}

// TestBackendFailureIsolation pins SC-006: with a dead/slow backend the
// tool answers politely within the latency budget and the process (and
// /healthz) stays alive.
func TestBackendFailureIsolation(t *testing.T) {
	// Finder blocks until the handler's own deadline fires.
	blockingFinder := func(ctx context.Context, _, _ string) (core.RouteResult, error) {
		<-ctx.Done()
		return core.RouteResult{}, ctx.Err()
	}
	srv := httptest.NewServer(newHTTPMux(buildHTTPMCPServer("test", nil, blockingFinder), nil))
	t.Cleanup(srv.Close)

	start := time.Now()
	var resp jsonRPCResponse
	postJSONRPC(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_transit_route","arguments":{"from":"강남역","to":"홍대입구역"}}}`, &resp)
	elapsed := time.Since(start)

	if elapsed >= 3*time.Second {
		t.Errorf("tool call took %v, must stay under 3s (p99 budget)", elapsed)
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	mustUnmarshal(t, resp.Result, &result)
	if !result.IsError {
		t.Fatal("isError = false, want polite error result")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "일시적으로 응답하지 않아요") {
		t.Errorf("content = %+v, want transient-backend message", result.Content)
	}

	// Server must still be alive and healthy.
	healthResp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz after failure: %v", err)
	}
	defer func() { _ = healthResp.Body.Close() }()
	if healthResp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", healthResp.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(newHTTPMux(buildHTTPMCPServer("test", nil, okFinder), nil))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want \"ok\"", body)
	}
}

// TestToolCallLogging pins FR-010: one structured log line per tool call
// with tool, outcome, and duration.
func TestToolCallLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := httpRouteToolHandler(logger, okFinder)
	if _, _, err := handler(context.Background(), nil, cloudRouteInput{From: "강남역", To: "홍대입구역"}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var entry map[string]any
	line := lastLogLine(t, buf.String())
	mustUnmarshal(t, []byte(line), &entry)

	if entry["tool"] != "find_transit_route" {
		t.Errorf("log tool = %v, want find_transit_route", entry["tool"])
	}
	if entry["outcome"] != "success" {
		t.Errorf("log outcome = %v, want success", entry["outcome"])
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Error("log missing duration_ms")
	}
}

func TestParseMCPFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantHTTP bool
		wantAddr string
		wantErr  bool
	}{
		{"no flags keeps stdio", nil, false, "", false},
		{"--http", []string{"--http"}, true, "", false},
		{"--http with addr", []string{"--http", "--addr", ":9090"}, true, ":9090", false},
		{"unknown flag errors", []string{"--bogus"}, false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpMode, addr, err := parseMCPFlags(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if httpMode != tt.wantHTTP || addr != tt.wantAddr {
				t.Errorf("= (%v, %q), want (%v, %q)", httpMode, addr, tt.wantHTTP, tt.wantAddr)
			}
		})
	}
}

func TestResolveHTTPAddr(t *testing.T) {
	tests := []struct {
		addrFlag, portEnv, want string
	}{
		{":9090", "3000", ":9090"}, // --addr wins
		{"", "3000", ":3000"},      // then $PORT
		{"", "", ":8080"},          // then default
	}
	for _, tt := range tests {
		if got := resolveHTTPAddr(tt.addrFlag, tt.portEnv); got != tt.want {
			t.Errorf("resolveHTTPAddr(%q, %q) = %q, want %q", tt.addrFlag, tt.portEnv, got, tt.want)
		}
	}
}

func TestMotisURLFromEnv(t *testing.T) {
	t.Run("missing env is a clear error", func(t *testing.T) {
		_, err := motisURLFromEnv(func(string) string { return "" })
		if err == nil || !strings.Contains(err.Error(), "NAERYEO_MOTIS_URL") {
			t.Fatalf("err = %v, want mention of NAERYEO_MOTIS_URL", err)
		}
	})
	t.Run("set env passes through trimmed", func(t *testing.T) {
		got, err := motisURLFromEnv(func(string) string { return " https://motis.example.com " })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://motis.example.com" {
			t.Errorf("url = %q", got)
		}
	})
}

func TestRunMCPHTTPCommandFailsFastWithoutEnv(t *testing.T) {
	t.Setenv("NAERYEO_MOTIS_URL", "")

	var stderr bytes.Buffer
	code := runMCPHTTPCommand("", &stderr, slog.New(slog.DiscardHandler))
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "NAERYEO_MOTIS_URL") {
		t.Errorf("stderr = %q, want mention of NAERYEO_MOTIS_URL", stderr.String())
	}
}

func TestRunDispatchesMCPHTTPWithoutEnv(t *testing.T) {
	t.Setenv("NAERYEO_MOTIS_URL", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"mcp", "--http"}, &stdout, &stderr, slog.New(slog.DiscardHandler))
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "NAERYEO_MOTIS_URL") {
		t.Errorf("stderr = %q, want mention of NAERYEO_MOTIS_URL", stderr.String())
	}
}

// --- helpers ---

type jsonRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// postJSONRPC POSTs one JSON-RPC message to the stateless MCP endpoint and
// decodes the application/json response (JSONResponse mode).
func postJSONRPC(t *testing.T, url, body string, out *jsonRPCResponse) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body: %s", resp.StatusCode, raw)
	}
	mustUnmarshal(t, raw, out)
}

func listToolsViaHTTP(t *testing.T) []map[string]any {
	t.Helper()
	srv := httptest.NewServer(newHTTPMux(buildHTTPMCPServer("test", nil, okFinder), nil))
	t.Cleanup(srv.Close)

	var resp jsonRPCResponse
	postJSONRPC(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, &resp)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	var result struct {
		Tools []map[string]any `json:"tools"`
	}
	mustUnmarshal(t, resp.Result, &result)
	return result.Tools
}

func mustUnmarshal(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
}

// lastLogLine returns the final non-empty line of a log buffer (the
// tool-call completion line, after any init lines).
func lastLogLine(t *testing.T, s string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 || lines[len(lines)-1] == "" {
		t.Fatal("no log lines captured")
	}
	var last string
	for _, l := range lines {
		if strings.Contains(l, "tool call") {
			last = l
		}
	}
	if last == "" {
		t.Fatalf("no tool-call log line in: %s", s)
	}
	return last
}
