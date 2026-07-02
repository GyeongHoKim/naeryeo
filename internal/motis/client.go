package motis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// defaultCallTimeout guards each individual MOTIS HTTP call against
// eating the whole latency budget (e.g. a hung connection). The tool
// chain (geocode from + geocode to + plan, sequential) is bounded
// end-to-end by the handler's 2.5s context deadline derived from
// PlayMCP's required p99 <= 3,000ms (research.md §4); this per-call
// value only needs to be under that budget while leaving room for a
// first-call TLS handshake to a distant backend.
const defaultCallTimeout = 2000 * time.Millisecond

var (
	// ErrPlaceNotFound indicates a place name matched nothing in the MOTIS
	// geocoder. FindRoute maps it onto *core.ErrPointNotFound with the
	// right side; it is exported so direct Geocode callers can classify.
	ErrPlaceNotFound = errors.New("motis: no matching place")
	// ErrNoRoute indicates both places resolved but MOTIS returned no
	// itinerary connecting them.
	ErrNoRoute = errors.New("motis: no transit route between the two points")
	// ErrUnavailable indicates a network error, timeout, non-2xx status,
	// or unparseable response from the MOTIS backend.
	ErrUnavailable = errors.New("motis: backend unavailable")
)

// Place is a geocoded location — the input plan() needs. Name carries the
// resolved canonical name (e.g. "강남역"), which also replaces MOTIS's
// synthetic "START"/"END" leg endpoints in step descriptions.
type Place struct {
	Name string
	Lat  float64
	Lon  float64
}

// Client calls a self-hosted MOTIS server. The zero value is not usable;
// construct with NewClient. Mirrors internal/core.Client's shape
// (BaseURL/HTTPClient/Logger with nil-safe logger).
type Client struct {
	// BaseURL is the MOTIS server base URL without a trailing slash,
	// e.g. "https://motis.example.com".
	BaseURL string
	// HTTPClient is the HTTP client used for API calls. NewClient sets a
	// 1.2s-timeout client; tests may substitute their own.
	HTTPClient *http.Client
	// Logger receives one Debug line per HTTP call. Nil discards logs.
	Logger *slog.Logger
}

// NewClient builds a Client for the given MOTIS base URL with the default
// per-call timeout.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: defaultCallTimeout},
	}
}

func (c *Client) logger() *slog.Logger {
	if c.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.Logger
}

// geocodeMatch mirrors one element of /api/v1/geocode's response array
// (contracts/motis-api.md §1). MOTIS encodes numbers in exponent notation
// (3.74999E1), which encoding/json's float64 decoding handles natively.
type geocodeMatch struct {
	Type string  `json:"type"` // STOP | PLACE | ADDRESS
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// planResponse mirrors the fields of /api/v3/plan this package consumes
// (contracts/motis-api.md §2).
type planResponse struct {
	Itineraries []itinerary `json:"itineraries"`
}

type itinerary struct {
	Duration  int   `json:"duration"` // seconds
	Transfers int   `json:"transfers"`
	Legs      []leg `json:"legs"`
}

type leg struct {
	Mode           string   `json:"mode"` // WALK | SUBWAY | BUS | TRAM | RAIL | ...
	RouteShortName string   `json:"routeShortName"`
	From           legPoint `json:"from"`
	To             legPoint `json:"to"`
	Duration       int      `json:"duration"` // seconds
}

type legPoint struct {
	Name string `json:"name"`
}

// doGet issues a GET against path (already query-encoded) and decodes the
// JSON body into out. Every failure mode other than a valid non-empty
// response maps onto ErrUnavailable; callers never see transport details.
func (c *Client) doGet(ctx context.Context, rawURL string, out any) (err error) {
	start := time.Now()
	statusCode := 0
	defer func() {
		c.logger().Debug("motis: HTTP GET",
			"url", rawURL,
			"status", statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err != nil,
		)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %w", ErrUnavailable, err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("%w: close body: %w", ErrUnavailable, closeErr)
		}
	}()

	statusCode = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode response: %w", ErrUnavailable, err)
	}
	return nil
}

// Geocode resolves a Korean place name (station, stop, or landmark) to a
// Place via GET /api/v1/geocode. Selection rule (contracts/motis-api.md §1):
// the first match with type STOP wins; if no STOP is present the first
// match wins; an empty result is ErrPlaceNotFound.
//
// Riders habitually append "역" to station names ("홍대입구역") while the
// KTDB feed names stops without it ("홍대입구" — verified against the live
// feed, 2026-07-03). On an empty result for a "역"-suffixed name, one
// retry is made with the suffix stripped.
func (c *Client) Geocode(ctx context.Context, name string) (Place, error) {
	place, err := c.geocodeOnce(ctx, name)
	if errors.Is(err, ErrPlaceNotFound) {
		if stripped := strings.TrimSuffix(name, "역"); stripped != name && stripped != "" {
			if retried, retryErr := c.geocodeOnce(ctx, stripped); retryErr == nil {
				return retried, nil
			}
		}
	}
	return place, err
}

func (c *Client) geocodeOnce(ctx context.Context, name string) (Place, error) {
	q := url.Values{"text": {name}, "language": {"ko"}}
	var matches []geocodeMatch
	if err := c.doGet(ctx, c.BaseURL+"/api/v1/geocode?"+q.Encode(), &matches); err != nil {
		return Place{}, fmt.Errorf("geocode %q: %w", name, err)
	}
	if len(matches) == 0 {
		return Place{}, fmt.Errorf("geocode %q: %w", name, ErrPlaceNotFound)
	}

	chosen := matches[0]
	for _, m := range matches {
		if m.Type == "STOP" {
			chosen = m
			break
		}
	}
	return Place{Name: chosen.Name, Lat: chosen.Lat, Lon: chosen.Lon}, nil
}

// Plan finds the best transit itinerary between two resolved places via
// GET /api/v3/plan and maps it onto the shared core domain model
// (data-model.md). An empty itineraries array is ErrNoRoute.
func (c *Client) Plan(ctx context.Context, from, to Place) (core.RouteResult, error) {
	q := url.Values{
		"fromPlace":      {fmt.Sprintf("%f,%f", from.Lat, from.Lon)},
		"toPlace":        {fmt.Sprintf("%f,%f", to.Lat, to.Lon)},
		"numItineraries": {"1"},
	}
	var resp planResponse
	if err := c.doGet(ctx, c.BaseURL+"/api/v3/plan?"+q.Encode(), &resp); err != nil {
		return core.RouteResult{}, fmt.Errorf("plan: %w", err)
	}
	if len(resp.Itineraries) == 0 {
		return core.RouteResult{}, ErrNoRoute
	}

	best := resp.Itineraries[0]
	steps := make([]core.RouteStep, 0, len(best.Legs))
	for _, l := range best.Legs {
		steps = append(steps, core.RouteStep{Description: describeLeg(l, from.Name, to.Name)})
	}
	return core.RouteResult{
		TotalTime:     (best.Duration + 30) / 60, // seconds → minutes, rounded
		TransferCount: best.Transfers,
		Fare:          0, // KTDB GTFS carries no fares (research.md §3)
		Steps:         steps,
	}, nil
}

// FindRoute is the single entry point cmd/naeryeo uses: it resolves both
// names then plans the route. Geocode misses are translated into
// *core.ErrPointNotFound with the correct side so the cmd layer can reuse
// one error taxonomy across the ODsay and MOTIS backends.
func (c *Client) FindRoute(ctx context.Context, from, to string) (core.RouteResult, error) {
	fromPlace, err := c.Geocode(ctx, from)
	if err != nil {
		if errors.Is(err, ErrPlaceNotFound) {
			return core.RouteResult{}, &core.ErrPointNotFound{Side: "from", Name: from}
		}
		return core.RouteResult{}, err
	}
	toPlace, err := c.Geocode(ctx, to)
	if err != nil {
		if errors.Is(err, ErrPlaceNotFound) {
			return core.RouteResult{}, &core.ErrPointNotFound{Side: "to", Name: to}
		}
		return core.RouteResult{}, err
	}
	return c.Plan(ctx, fromPlace, toPlace)
}

// describeLeg renders one MOTIS leg as a Korean step description.
// MOTIS's synthetic "START"/"END" endpoint names are replaced with the
// user-facing place names (contracts/motis-api.md §2).
func describeLeg(l leg, fromName, toName string) string {
	src := l.From.Name
	if src == "START" {
		src = fromName
	}
	dst := l.To.Name
	if dst == "END" {
		dst = toName
	}

	if l.Mode == "WALK" {
		minutes := (l.Duration + 30) / 60
		if minutes < 1 {
			minutes = 1
		}
		return fmt.Sprintf("%s까지 도보 %d분", dst, minutes)
	}

	line := l.RouteShortName
	if line == "" {
		line = modeKorean(l.Mode)
	}
	return fmt.Sprintf("%s에서 %s 승차 → %s 하차", src, line, dst)
}

// modeKorean maps a MOTIS transit mode onto its Korean rider-facing word.
// Unknown modes pass through unchanged rather than guessing.
func modeKorean(mode string) string {
	switch mode {
	case "SUBWAY", "TRAM", "METRO":
		return "지하철"
	case "BUS", "COACH":
		return "버스"
	case "RAIL", "HIGHSPEED_RAIL", "LONG_DISTANCE", "REGIONAL_RAIL", "REGIONAL_FAST_RAIL":
		return "기차"
	case "FERRY":
		return "여객선"
	case "AIRPLANE":
		return "항공"
	default:
		return mode
	}
}
