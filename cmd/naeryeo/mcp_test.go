package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var discardLogger = slog.New(slog.DiscardHandler)

// connectTestClient wires an in-memory MCP client-server pair around
// server, mirroring the pattern used by the SDK's own examples
// (tool_example_test.go's connect helper). This exercises the real
// JSON-RPC round trip without touching a subprocess or real stdio.
func connectTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callFindTransitRoute(t *testing.T, session *mcp.ClientSession, from, to string) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "find_transit_route",
		Arguments: map[string]any{"from": from, "to": to},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	return res
}

func decodeRouteToolOutput(t *testing.T, res *mcp.CallToolResult) RouteToolOutput {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	var out RouteToolOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal StructuredContent: %v", err)
	}
	return out
}

func resultText(res *mcp.CallToolResult) string {
	var s string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			s += tc.Text
		}
	}
	return s
}

func TestFindTransitRouteTool_Success(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{
			TotalTime:     42,
			TransferCount: 1,
			Fare:          1500,
			Steps: []core.RouteStep{
				{Description: "강남역에서 2호선 승차 → 신도림역에서 하차"},
			},
		}, nil
	}

	server := buildMCPServer("test", discardLogger, load, findRoute)
	session := connectTestClient(t, server)

	res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	if res.IsError {
		t.Fatalf("IsError = true, want false; content = %q", resultText(res))
	}

	out := decodeRouteToolOutput(t, res)
	if out.TotalTimeMinutes != 42 || out.TransferCount != 1 || out.FareWon != 1500 {
		t.Errorf("unexpected output: %+v", out)
	}
	if len(out.Steps) != 1 || out.Steps[0] == "" {
		t.Errorf("unexpected Steps: %+v", out.Steps)
	}
}

func TestFindTransitRouteTool_NoTravelNeeded(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{NoTravelNeeded: true}, nil
	}

	server := buildMCPServer("test", discardLogger, load, findRoute)
	session := connectTestClient(t, server)

	res := callFindTransitRoute(t, session, "강남역", "강남역")
	if res.IsError {
		t.Fatalf("IsError = true, want false; content = %q", resultText(res))
	}
	out := decodeRouteToolOutput(t, res)
	if !out.NoTravelNeeded {
		t.Errorf("NoTravelNeeded = false, want true; out = %+v", out)
	}
}

func TestFindTransitRouteTool_ConsecutiveCalls(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }
	calls := 0
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		calls++
		return core.RouteResult{TotalTime: calls, Fare: calls * 100}, nil
	}

	server := buildMCPServer("test", discardLogger, load, findRoute)
	session := connectTestClient(t, server)

	first := decodeRouteToolOutput(t, callFindTransitRoute(t, session, "A", "B"))
	second := decodeRouteToolOutput(t, callFindTransitRoute(t, session, "C", "D"))

	if first.TotalTimeMinutes == second.TotalTimeMinutes {
		t.Errorf("expected two independent calls, got identical results: %+v, %+v", first, second)
	}
	if calls != 2 {
		t.Errorf("findRoute called %d times, want 2", calls)
	}
}

func TestFindTransitRouteTool_APIKeyProblems(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		load := func() (string, error) { return "", config.ErrNotConfigured }
		findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
			return core.NewClient(apiKey).FindRoute(ctx, from, to)
		}

		server := buildMCPServer("test", discardLogger, load, findRoute)
		session := connectTestClient(t, server)

		res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
		if !strings.Contains(resultText(res), "naeryeo setup") {
			t.Errorf("content = %q, want a hint to run naeryeo setup", resultText(res))
		}
	})

	t.Run("invalid key is a distinct message", func(t *testing.T) {
		load := func() (string, error) { return "stored-but-invalid", nil }
		findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
			return core.RouteResult{}, core.ErrAuthFailed
		}

		server := buildMCPServer("test", discardLogger, load, findRoute)
		session := connectTestClient(t, server)

		res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
		text := resultText(res)
		if strings.Contains(text, "설정되지 않았습니다") {
			t.Errorf("content = %q, should not reuse the not-configured message for an invalid key", text)
		}
		if !strings.Contains(text, "유효하지 않습니다") {
			t.Errorf("content = %q, want a distinct invalid-key message", text)
		}
	})
}

func TestFindTransitRouteTool_PointAndRouteErrors(t *testing.T) {
	tests := []struct {
		name    string
		findErr error
		want    string
	}{
		{
			name:    "point not found",
			findErr: &core.ErrPointNotFound{Side: "from", Name: "존재하지않는가짜지명"},
			want:    "출발지",
		},
		{
			name:    "no route",
			findErr: core.ErrNoRoute,
			want:    "경로가 없습니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			load := func() (string, error) { return "test-key", nil }
			findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
				return core.RouteResult{}, tt.findErr
			}

			server := buildMCPServer("test", discardLogger, load, findRoute)
			session := connectTestClient(t, server)

			res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
			if !res.IsError {
				t.Fatal("IsError = false, want true")
			}
			if !strings.Contains(resultText(res), tt.want) {
				t.Errorf("content = %q, want substring %q", resultText(res), tt.want)
			}
		})
	}
}

// TestFindTransitRouteTool_UpstreamFailureThenRecovery verifies the server
// survives an upstream failure and correctly serves the next call in the
// same session (spec 003 FR-009).
func TestFindTransitRouteTool_UpstreamFailureThenRecovery(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }
	shouldFail := true
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		if shouldFail {
			return core.RouteResult{}, core.ErrUpstreamUnavailable
		}
		return core.RouteResult{TotalTime: 10}, nil
	}

	server := buildMCPServer("test", discardLogger, load, findRoute)
	session := connectTestClient(t, server)

	failed := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	if !failed.IsError {
		t.Fatal("IsError = false, want true for the upstream failure")
	}

	shouldFail = false
	recovered := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	if recovered.IsError {
		t.Fatalf("IsError = true, want false for the recovered call; content = %q", resultText(recovered))
	}
	out := decodeRouteToolOutput(t, recovered)
	if out.TotalTimeMinutes != 10 {
		t.Errorf("TotalTimeMinutes = %d, want 10", out.TotalTimeMinutes)
	}
}

func TestFindTransitRouteTool_LogsToolCall(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	load := func() (string, error) { return "test-key", nil }
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{TotalTime: 5}, nil
	}

	server := buildMCPServer("test", logger, load, findRoute)
	session := connectTestClient(t, server)

	callFindTransitRoute(t, session, "강남역", "홍대입구역")

	logged := buf.String()
	if !strings.Contains(logged, `"tool":"find_transit_route"`) {
		t.Errorf("log output = %q, want a find_transit_route tool call record", logged)
	}
	if !strings.Contains(logged, `"from":"강남역"`) || !strings.Contains(logged, `"to":"홍대입구역"`) {
		t.Errorf("log output = %q, want from/to attributes", logged)
	}
}
