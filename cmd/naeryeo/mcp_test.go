package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

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
	findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
		return core.RouteResult{
			TotalTime:     42,
			TransferCount: 1,
			Fare:          1500,
			FareKnown:     true,
			Steps: []core.RouteStep{
				{Description: "강남역에서 2호선 승차 → 신도림역에서 하차"},
			},
		}, nil
	}

	server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
	session := connectTestClient(t, server)

	res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	if res.IsError {
		t.Fatalf("IsError = true, want false; content = %q", resultText(res))
	}

	out := decodeRouteToolOutput(t, res)
	if out.TotalTimeMinutes != 42 || out.TransferCount != 1 {
		t.Errorf("unexpected output: %+v", out)
	}
	if out.FareWon == nil || *out.FareWon != 1500 {
		t.Errorf("FareWon = %v, want a pointer to 1500", out.FareWon)
	}
	if len(out.Steps) != 1 || out.Steps[0] == "" {
		t.Errorf("unexpected Steps: %+v", out.Steps)
	}
}

func TestFindTransitRouteTool_NoTravelNeeded(t *testing.T) {
	findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
		return core.RouteResult{NoTravelNeeded: true}, nil
	}

	server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
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
	calls := 0
	findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
		calls++
		return core.RouteResult{TotalTime: calls, Fare: calls * 100, FareKnown: true}, nil
	}

	server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
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
		// Since spec 006 the credential is resolved before the search, so a
		// missing key never reaches a finder — it arrives as a pre-flight
		// failure from the provider resolver.
		resolve := failingProvider(classifyRouteError(core.ErrAPIKeyMissing, true))

		server := buildMCPServer("test", discardLogger, resolve, geoPresent)
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
		findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
			return core.RouteResult{}, core.ErrAuthFailed
		}

		server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
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
			findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
				return core.RouteResult{}, tt.findErr
			}

			server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
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
	shouldFail := true
	findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
		if shouldFail {
			return core.RouteResult{}, core.ErrUpstreamUnavailable
		}
		return core.RouteResult{TotalTime: 10}, nil
	}

	server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
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

// TestFindTransitRouteTool_TransportFailureNeverLeaksTheAPIKey covers the MCP
// half of GYE-293's exit criteria. This path is the more damaging of the two:
// the tool result is written into the model's context, so a leaked key would
// be persisted in conversation history and sent onward to the model provider.
func TestFindTransitRouteTool_TransportFailureNeverLeaksTheAPIKey(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	server := buildMCPServer("test", logger, staticProvider(deadUpstreamFindRoute), geoAbsent)
	session := connectTestClient(t, server)

	res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	if !res.IsError {
		t.Fatal("IsError = false, want true for the transport failure")
	}

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal CallToolResult: %v", err)
	}
	// The whole result is checked, not just its text content: any future field
	// that echoes the error would otherwise slip through.
	assertNoAPIKey(t, string(raw), leakKey)
	assertNoAPIKey(t, logs.String(), leakKey)
}

// TestFindTransitRouteTool_GeocoderMessagesMatchCLI verifies the MCP tool
// produces the exact same geocoder-related wording as the CLI, since both go
// through the shared routeErrorMessage (spec 004 FR-011).
func TestFindTransitRouteTool_GeocoderMessagesMatchCLI(t *testing.T) {

	t.Run("point-not-found without geocoder shows the shared FR-007 hint", func(t *testing.T) {
		findErr := &core.ErrPointNotFound{Side: "from", Name: "아이디스 타워"}
		findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
			return core.RouteResult{}, findErr
		}
		server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoAbsent)
		session := connectTestClient(t, server)

		res := callFindTransitRoute(t, session, "아이디스 타워", "수지구청")
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
		if want := routeErrorMessage(findErr, false); !strings.Contains(resultText(res), want) {
			t.Errorf("MCP content = %q, want it to contain the shared message %q", resultText(res), want)
		}
	})

	t.Run("geocoder auth failure has the distinct place-search-key message", func(t *testing.T) {
		findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
			return core.RouteResult{}, core.ErrGeocoderAuthFailed
		}
		server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
		session := connectTestClient(t, server)

		res := callFindTransitRoute(t, session, "아이디스 타워", "수지구청")
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
		if !strings.Contains(resultText(res), "장소 검색 키가 유효하지 않습니다") {
			t.Errorf("content = %q, want the geocoder auth-failed message", resultText(res))
		}
	})

	// A geocoder 4xx rejection must reach the AI caller as an actionable
	// natural-language message, never as a raw HTTP status/code/body.
	t.Run("geocoder rejection reaches the AI as actionable text, not HTTP internals", func(t *testing.T) {
		findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
			return core.RouteResult{}, &core.ErrGeocoderRejected{Status: 400, Code: "-2", Message: "query is required"}
		}
		server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
		session := connectTestClient(t, server)

		res := callFindTransitRoute(t, session, "아이디스 타워", "강남역")
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
		text := resultText(res)
		if !strings.Contains(text, "위치를 인식하지 못했습니다") {
			t.Errorf("content = %q, want an actionable reformulate-location message", text)
		}
		for _, leak := range []string{"400", "HTTP", "query is required"} {
			if strings.Contains(text, leak) {
				t.Errorf("content = %q, must not leak %q to the AI caller", text, leak)
			}
		}
	})
}

func TestFindTransitRouteTool_LogsToolCall(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	findRoute := func(ctx context.Context, from, to string) (core.RouteResult, error) {
		return core.RouteResult{TotalTime: 5}, nil
	}

	server := buildMCPServer("test", logger, staticProvider(findRoute), geoPresent)
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
