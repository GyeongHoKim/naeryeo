package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RouteToolInput is the find_transit_route tool's input, as seen by the MCP
// client (Claude). Field names and jsonschema tags double as the tool's
// argument documentation.
type RouteToolInput struct {
	From string `json:"from" jsonschema:"출발지 (역/정류장 이름 또는 주소)"`
	To   string `json:"to" jsonschema:"도착지 (역/정류장 이름 또는 주소)"`
}

// RouteToolOutput is the find_transit_route tool's structured success
// output.
type RouteToolOutput struct {
	NoTravelNeeded   bool     `json:"noTravelNeeded,omitempty" jsonschema:"true면 출발지와 도착지가 사실상 같은 위치라 이동이 필요 없음"`
	TotalTimeMinutes int      `json:"totalTimeMinutes,omitempty" jsonschema:"총 소요시간(분)"`
	TransferCount    int      `json:"transferCount,omitempty" jsonschema:"환승 횟수"`
	FareWon          int      `json:"fareWon,omitempty" jsonschema:"예상 요금(원)"`
	Steps            []string `json:"steps,omitempty" jsonschema:"순서대로 나열된 단계별 이동 안내"`
}

// toRouteToolOutput maps a core.RouteResult onto the tool's output shape.
func toRouteToolOutput(result core.RouteResult) RouteToolOutput {
	if result.NoTravelNeeded {
		return RouteToolOutput{NoTravelNeeded: true}
	}

	steps := make([]string, 0, len(result.Steps))
	for _, step := range result.Steps {
		steps = append(steps, step.Description)
	}
	return RouteToolOutput{
		TotalTimeMinutes: result.TotalTime,
		TransferCount:    result.TransferCount,
		FareWon:          result.Fare,
		Steps:            steps,
	}
}

// routeToolHandler builds the find_transit_route tool handler, closing over
// load/findRoute so it can be exercised by tests with fakes and wired to
// the real internal/config + internal/core in main.go. logger receives one
// completion-time log per tool call; a nil logger discards it (a fresh
// slog.New(slog.DiscardHandler) is used in that case, mirroring
// internal/core.Client's nil-defaulting Logger field).
func routeToolHandler(
	logger *slog.Logger,
	load func() (string, error),
	loadGeocoder func() (string, error),
	findRoute func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error),
) mcp.ToolHandlerFor[RouteToolInput, RouteToolOutput] {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return func(ctx context.Context, _ *mcp.CallToolRequest, in RouteToolInput) (result *mcp.CallToolResult, output RouteToolOutput, err error) {
		start := time.Now()
		defer func() {
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			logger.Info("mcp: tool call",
				"tool", "find_transit_route",
				"from", in.From, "to", in.To,
				"outcome", outcome,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()

		apiKey, loadErr := load()
		if loadErr != nil && !errors.Is(loadErr, config.ErrNotConfigured) {
			return nil, RouteToolOutput{}, fmt.Errorf("API 키 조회 실패: %w", loadErr)
		}

		// If loadErr is config.ErrNotConfigured, apiKey is "" and findRoute
		// (backed by core.Client.FindRoute) returns ErrAPIKeyMissing itself —
		// no separate "not configured" branch is needed here.
		routeResult, findErr := findRoute(ctx, apiKey, in.From, in.To)
		if findErr != nil {
			return nil, RouteToolOutput{}, errors.New(routeErrorMessage(findErr, geocoderConfigured(loadGeocoder)))
		}
		return nil, toRouteToolOutput(routeResult), nil
	}
}

// buildMCPServer assembles the MCP server and registers the
// find_transit_route tool. It takes load/findRoute as parameters (same
// shape as runRoute's) so it can be unit- and end-to-end-tested without
// touching internal/config or a real ODsay call. logger is also wired into
// mcp.ServerOptions so the SDK's own session-lifecycle logs are captured.
func buildMCPServer(
	version string,
	logger *slog.Logger,
	load func() (string, error),
	loadGeocoder func() (string, error),
	findRoute func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error),
) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "naeryeo", Version: version}, &mcp.ServerOptions{Logger: logger})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_transit_route",
		Description: "대한민국 대중교통(지하철·버스·시외버스)으로 두 지점 사이의 경로를 검색한다.",
	}, routeToolHandler(logger, load, loadGeocoder, findRoute))
	if logger != nil {
		logger.Info("mcp: server initialized", "name", "naeryeo", "version", version)
	}
	return server
}

// runMCP runs the assembled server over the real process stdio until the
// client disconnects. It is the thin, effectively untested glue invoked
// from main.go's "mcp" case — mcp.StdioTransport binds directly to the
// process's os.Stdin/os.Stdout, so nothing else in this process may write
// to stdout once this is running (research.md §3).
func runMCP(ctx context.Context, server *mcp.Server) error {
	return server.Run(ctx, &mcp.StdioTransport{})
}
