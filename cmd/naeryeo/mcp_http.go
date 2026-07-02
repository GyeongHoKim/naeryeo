package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/GyeongHoKim/naeryeo/internal/core"
	"github.com/GyeongHoKim/naeryeo/internal/motis"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The cloud (PlayMCP) track's tool contract lives in
// specs/005-playmcp-cloud-server/contracts/mcp-tool.md. Every constant
// below is pinned by tests in mcp_http_test.go because the PlayMCP dev
// guide treats violations (missing annotations, "kakao" in names, >1024
// char descriptions) as review-rejection criteria.
const (
	// cloudToolDescription is English-first with the service name in both
	// scripts ("naeryeo(내려)"), per the PlayMCP dev guide.
	cloudToolDescription = "Finds a public transit route between two places in South Korea — subway, bus, and intercity — via naeryeo(내려). Give a departure and a destination as station, stop, or place names in Korean; returns total duration, transfers, and step-by-step directions."

	// httpToolTimeout bounds one tool call end-to-end. PlayMCP requires
	// p99 <= 3,000ms; 2.5s leaves headroom for serialization and the
	// platform round-trip (research.md §4).
	httpToolTimeout = 2500 * time.Millisecond

	// maxPlaceNameLen rejects absurdly long place-name inputs before any
	// backend call (contracts/mcp-tool.md input validation).
	maxPlaceNameLen = 256

	// httpShutdownTimeout is how long in-flight requests get to drain on
	// SIGINT/SIGTERM.
	httpShutdownTimeout = 5 * time.Second
)

// cloudRouteInput is the cloud tool's input. It is separate from the stdio
// track's RouteToolInput so the PlayMCP-facing schema can carry
// English property descriptions without touching the local track.
type cloudRouteInput struct {
	From string `json:"from" jsonschema:"Departure place name in Korean — station, stop, or landmark (e.g. 강남역)"`
	To   string `json:"to" jsonschema:"Destination place name in Korean — station, stop, or landmark (e.g. 홍대입구역)"`
}

// cloudRouteFinder is the closure shape the HTTP track consumes — the
// MOTIS-backed twin of the stdio track's findRoute parameter. Note there
// is no API key: the cloud backend is self-hosted, not BYOK.
type cloudRouteFinder func(ctx context.Context, from, to string) (core.RouteResult, error)

// httpRouteToolHandler builds the cloud find_transit_route handler. The
// result is refined markdown text only (no structured JSON), per the
// PlayMCP dev guide's "minimal, human-readable result" rule.
func httpRouteToolHandler(logger *slog.Logger, finder cloudRouteFinder) mcp.ToolHandlerFor[cloudRouteInput, any] {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return func(ctx context.Context, _ *mcp.CallToolRequest, in cloudRouteInput) (result *mcp.CallToolResult, _ any, err error) {
		start := time.Now()
		defer func() {
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			logger.Info("mcp-http: tool call",
				"tool", "find_transit_route",
				"from", in.From, "to", in.To,
				"outcome", outcome,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()

		from := strings.TrimSpace(in.From)
		to := strings.TrimSpace(in.To)
		if from == "" || to == "" {
			return nil, nil, errors.New("출발지와 도착지를 모두 알려주세요")
		}
		if utf8.RuneCountInString(from) > maxPlaceNameLen || utf8.RuneCountInString(to) > maxPlaceNameLen {
			return nil, nil, errors.New("장소 이름이 너무 길어요 — 역·정류장 이름처럼 짧은 이름으로 다시 시도해 주세요")
		}

		ctx, cancel := context.WithTimeout(ctx, httpToolTimeout)
		defer cancel()

		route, findErr := finder(ctx, from, to)
		if findErr != nil {
			err = errors.New(cloudRouteErrorMessage(findErr))
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: renderRouteMarkdown(from, to, route)}},
		}, nil, nil
	}
}

// cloudRouteErrorMessage classifies a MOTIS-track error into one of the
// four user-facing Korean messages fixed by contracts/mcp-tool.md. It must
// never leak internals (backend URL, status codes, Go error chains) — the
// full error is logged by the handler's deferred log line instead.
func cloudRouteErrorMessage(err error) string {
	var pointErr *core.ErrPointNotFound
	switch {
	case errors.As(err, &pointErr):
		side := "출발지"
		if pointErr.Side == "to" {
			side = "도착지"
		}
		return fmt.Sprintf("%s %q을(를) 찾지 못했어요. 역·정류장 이름으로 다시 시도해 주세요.", side, pointErr.Name)
	case errors.Is(err, motis.ErrNoRoute):
		return "해당 구간의 대중교통 경로를 찾지 못했어요."
	default:
		// motis.ErrUnavailable, context.DeadlineExceeded, and anything
		// unexpected all read as a transient backend problem to the user.
		return "경로 서버가 일시적으로 응답하지 않아요. 잠시 후 다시 시도해 주세요."
	}
}

// buildHTTPMCPServer assembles the cloud-track MCP server with the
// PlayMCP-compliant tool definition: all five behavior-hint annotations
// set explicitly (contracts/mcp-tool.md).
func buildHTTPMCPServer(version string, logger *slog.Logger, finder cloudRouteFinder) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "naeryeo", Version: version}, &mcp.ServerOptions{Logger: logger})

	notDestructive := false
	openWorld := true
	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_transit_route",
		Description: cloudToolDescription,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Find Korean transit route",
			ReadOnlyHint:    true,
			DestructiveHint: &notDestructive,
			IdempotentHint:  true,
			OpenWorldHint:   &openWorld,
		},
	}, httpRouteToolHandler(logger, finder))

	if logger != nil {
		logger.Info("mcp-http: server initialized", "name", "naeryeo", "version", version)
	}
	return server
}

// newHTTPMux wires the MCP Streamable HTTP handler at "/" and a
// dependency-free liveness probe at /healthz (contracts/http-server.md).
// Stateless + JSONResponse match the PlayMCP dev guide's "Streamable HTTP,
// stateless recommended" requirements (research.md §1).
func newHTTPMux(server *mcp.Server, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: logger},
	))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// parseMCPFlags parses the mcp subcommand's flags. No flags (the common
// local case) leaves httpMode false so the stdio path stays untouched.
func parseMCPFlags(args []string) (httpMode bool, addrFlag string, err error) {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	httpFlag := fs.Bool("http", false, "serve MCP over Streamable HTTP instead of stdio")
	addr := fs.String("addr", "", "listen address for --http (default :$PORT or :8080)")
	if err := fs.Parse(args); err != nil {
		return false, "", fmt.Errorf("mcp: parse flags: %w", err)
	}
	return *httpFlag, *addr, nil
}

// resolveHTTPAddr picks the listen address: --addr wins, then :$PORT
// (containers/KC inject PORT), then :8080 (contracts/http-server.md).
func resolveHTTPAddr(addrFlag, portEnv string) string {
	if addrFlag != "" {
		return addrFlag
	}
	if portEnv != "" {
		return ":" + portEnv
	}
	return ":8080"
}

// motisURLFromEnv reads the mandatory MOTIS endpoint. The cloud track
// fails fast without it (FR-004) — a half-up server that cannot route
// would only fail later, during review.
func motisURLFromEnv(getenv func(string) string) (string, error) {
	u := strings.TrimSpace(getenv("NAERYEO_MOTIS_URL"))
	if u == "" {
		return "", errors.New("NAERYEO_MOTIS_URL 환경변수가 필요합니다 (자체 호스팅 MOTIS 백엔드의 base URL, 예: https://motis.example.com)")
	}
	return u, nil
}

// runMCPHTTPCommand is the --http twin of the stdio path in run(): it
// builds the MOTIS-backed server and serves until SIGINT/SIGTERM.
func runMCPHTTPCommand(addrFlag string, stderr io.Writer, logger *slog.Logger) int {
	motisURL, err := motisURLFromEnv(os.Getenv)
	if err != nil {
		if _, writeErr := fmt.Fprintln(stderr, "naeryeo mcp --http: "+err.Error()); writeErr != nil {
			return 1
		}
		return 1
	}

	client := motis.NewClient(motisURL)
	client.Logger = logger
	server := buildHTTPMCPServer(version, logger, client.FindRoute)
	addr := resolveHTTPAddr(addrFlag, os.Getenv("PORT"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runMCPHTTP(ctx, server, addr, logger); err != nil {
		logger.Error("mcp-http: server exited with error", "error", err)
		return 1
	}
	return 0
}

// runMCPHTTP serves the mux on addr until ctx is canceled, then drains
// in-flight requests for up to httpShutdownTimeout.
func runMCPHTTP(ctx context.Context, server *mcp.Server, addr string, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           newHTTPMux(server, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	if logger != nil {
		logger.Info("mcp-http: listening", "addr", addr)
	}

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
