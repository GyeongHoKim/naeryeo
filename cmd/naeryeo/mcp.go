package main

import (
	"context"
	"errors"
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

// RouteToolOutput is the envelope shared by the find_transit_route tool's
// structured output and the CLI's --json document. Success fields and the
// optional Error object live in one type on purpose: callers tell the two apart
// by the presence of "error" alone, and — because both entry points serialize
// this same type — the success schema cannot drift between them (spec 005
// FR-010).
//
// The shared type is also required by the MCP SDK: a ToolHandlerFor has exactly
// one output type, and the failure payload has to travel through it to reach
// structuredContent (see routeToolHandler).
type RouteToolOutput struct {
	NoTravelNeeded   bool        `json:"noTravelNeeded,omitempty" jsonschema:"true면 출발지와 도착지가 사실상 같은 위치라 이동이 필요 없음"`
	TotalTimeMinutes int         `json:"totalTimeMinutes,omitempty" jsonschema:"총 소요시간(분)"`
	TransferCount    int         `json:"transferCount,omitempty" jsonschema:"환승 횟수"`
	FareWon          int         `json:"fareWon,omitempty" jsonschema:"예상 요금(원)"`
	Steps            []string    `json:"steps,omitempty" jsonschema:"순서대로 나열된 단계별 이동 안내"`
	Error            *RouteError `json:"error,omitempty" jsonschema:"실패 시에만 존재. 있으면 나머지 필드는 비어 있음"`
}

// RouteError is the serialized form of a failure. Code is the stable
// identifier to branch on; Message is safe to relay to the user verbatim but
// must never be matched on as a string.
type RouteError struct {
	Code    string `json:"code" jsonschema:"안정적인 실패 코드. 이 값으로 후속 행동을 결정한다"`
	Message string `json:"message" jsonschema:"사용자에게 그대로 전달할 수 있는 실패 사유"`
	Hint    string `json:"hint,omitempty" jsonschema:"사용자가 취해야 할 조치가 있을 때만 존재"`
	Side    string `json:"side,omitempty" jsonschema:"point_not_found 전용. from/to/both"`
	Name    string `json:"name,omitempty" jsonschema:"point_not_found 전용. 인식하지 못한 입력값"`
	Docs    string `json:"docs,omitempty" jsonschema:"사용자가 직접 해결해야 하는 실패일 때만 존재하는 문서 URL. 있으면 사용자에게 그대로 전달한다"`
}

// toRouteToolOutput maps a core.RouteResult onto the envelope's success shape.
//
// Shared by both entry points: the MCP tool returns it as structured output and
// the CLI's --json mode serializes it to stdout. Routing both through one
// converter (rather than one per entry point) is what makes the success schema
// identical by construction instead of by convention — see
// TestRunRouteJSON_SuccessMatchesMCP.
//
// Error is left nil here; failures never travel through this function.
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
			// A failed search is reported through result (IsError + a coded
			// error object), not by returning err — see failureToolResult for
			// why — so err alone no longer tells the two apart.
			outcome := "success"
			if err != nil || (result != nil && result.IsError) {
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
			// Previously this wrapped loadErr and returned it, sending the raw
			// keychain error string to the AI caller. It now travels as a coded
			// failure like every other one (spec 005 FR-018).
			return failureToolResult(credentialStoreFailure())
		}

		// If loadErr is config.ErrNotConfigured, apiKey is "" and findRoute
		// (backed by core.Client.FindRoute) returns ErrAPIKeyMissing itself —
		// no separate "not configured" branch is needed here.
		routeResult, findErr := findRoute(ctx, apiKey, in.From, in.To)
		if findErr != nil {
			return failureToolResult(classifyRouteError(findErr, geocoderConfigured(loadGeocoder)))
		}
		return nil, toRouteToolOutput(routeResult), nil
	}
}

// failureToolResult renders a failure as a tool result, and it deliberately
// returns a nil error.
//
// That is not a style choice. ToolHandlerFor's wrapper in go-sdk v1.6.1
// (mcp/server.go) does this when a handler returns a non-nil error:
//
//	res, out, err := h(ctx, req, in)
//	if err != nil {
//	    var errRes CallToolResult   // the handler's res is discarded here
//	    errRes.SetError(err)
//	    return &errRes, nil
//	}
//
// The hand-built result is thrown away and replaced by one whose Content is
// err.Error() — i.e. the raw error chain, which is exactly the leak spec 005
// removes — and StructuredContent is left nil, so the error code never reaches
// the caller. Only the err == nil path preserves res, runs out through the
// output schema into res.StructuredContent, and leaves an already-populated
// Content alone. Returning the failure as data rather than as an error is
// therefore the only way to deliver both a coded payload and a readable message.
//
// TestFindTransitRouteTool_FailureCarriesStructuredCode fails if this is ever
// changed back or if an SDK upgrade alters the behavior above.
func failureToolResult(f failure) (*mcp.CallToolResult, RouteToolOutput, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: f.Prose()}},
	}, RouteToolOutput{Error: f.toRouteError()}, nil
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
