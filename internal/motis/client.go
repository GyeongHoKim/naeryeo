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

const (
	// geocodePath resolves a free-form name against MOTIS's own index of GTFS
	// stops and OSM addresses. The routing endpoint accepts only coordinates
	// or stop IDs, so this is not an optional nicety — it is how a name like
	// "강남역" becomes something /api/v6/plan will accept.
	geocodePath = "/api/v1/geocode"
	// planPath searches routes between two coordinates.
	planPath = "/api/v6/plan"
)

// errPlaceNotFound is the internal "this name matched nothing" signal, kept
// unexported so FindRoute can attach the right Side before it reaches a
// caller. It mirrors core.Client's errStationNotFound.
var errPlaceNotFound = errors.New("motis: no matching place")

// match is one hit from the geocode endpoint.
type match struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// place is an endpoint of a leg. Only the display name is consumed.
type place struct {
	Name string `json:"name"`
}

// leg is one segment of an itinerary.
type leg struct {
	Mode           string  `json:"mode"`
	From           place   `json:"from"`
	To             place   `json:"to"`
	Duration       float64 `json:"duration"` // seconds
	RouteShortName string  `json:"routeShortName"`
	Headsign       string  `json:"headsign"`
	AgencyName     string  `json:"agencyName"`
}

// itinerary is one candidate journey.
type itinerary struct {
	Duration  float64 `json:"duration"` // seconds
	Transfers int     `json:"transfers"`
	Legs      []leg   `json:"legs"`
}

// planResponse is the subset of /api/v6/plan this package consumes. Fares are
// not requested: MOTIS marks that parameter experimental, and the Korean GTFS
// feeds this is built against are not known to carry fare data, so asking for
// it would mean parsing a shape that may never arrive.
type planResponse struct {
	Itineraries []itinerary `json:"itineraries"`
	// Direct holds journeys that need no transit at all (a walk). It is
	// consulted when Itineraries is empty: answering "no route" when the two
	// points are a ten-minute walk apart would be wrong.
	Direct []itinerary `json:"direct"`
}

// Client searches transit routes via a self-hosted MOTIS instance.
//
// There is no API key field, and that is the point: the engine belongs to the
// user, so nothing here is authenticated and nothing here is billed.
type Client struct {
	// baseURL is the engine's root, without a trailing slash.
	baseURL_ string
	// HTTPClient is used for all requests. If nil, a client with a 10s
	// timeout is used.
	HTTPClient *http.Client
	// Logger receives diagnostic logs. If nil, logging is discarded.
	Logger *slog.Logger
	// Geocoder, when non-nil, resolves names MOTIS's own index does not know
	// — building names, POIs, and addresses outside its OSM extract. It is an
	// axis independent of the routing provider: leaving it nil simply means
	// only stops and addresses MOTIS indexes will resolve, which is enough
	// for station-name queries.
	Geocoder core.Geocoder
}

// NewClient returns a Client that talks to the MOTIS instance at baseURL.
func NewClient(baseURL string) *Client {
	return &Client{baseURL_: strings.TrimRight(strings.TrimSpace(baseURL), "/")}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *Client) baseURL() string { return c.baseURL_ }

func (c *Client) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// doGet issues a GET and decodes the JSON body into out.
//
// Note what it does NOT do: it never wraps the error returned by
// http.Client.Do. That error is a *url.Error whose message embeds the full
// request URL, which is the operator's private hostname and port. Letting it
// into the error chain would carry it to --debug output, to logs, and — via
// the MCP path — into an AI's conversation history (spec 006 FR-018). The
// same reasoning that made core.doGet redact ODsay's API key applies here to
// the endpoint itself; the difference is that there is nothing worth keeping,
// so the cause is dropped entirely rather than redacted.
//
// The URL is still logged at debug level, which goes to stderr and is opt-in.
func (c *Client) doGet(ctx context.Context, rawURL string, out any) error {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		c.logger().Debug("motis: request could not be built", "url", rawURL, "error", err)
		return core.ErrMotisUnavailable
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		c.logger().Debug("motis: request failed", "url", rawURL, "error", err,
			"duration_ms", time.Since(start).Milliseconds())
		return core.ErrMotisUnavailable
	}
	defer func() { _ = resp.Body.Close() }()

	c.logger().Debug("motis: HTTP GET", "url", rawURL, "status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds())

	switch {
	case resp.StatusCode >= 500:
		// The engine is up but broken or still starting. Worth retrying.
		return core.ErrMotisUnavailable
	case resp.StatusCode >= 400:
		// The engine understood us and said no. Retrying changes nothing.
		return &core.ErrMotisRejected{Status: resp.StatusCode}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return &core.ErrMotisRejected{Status: resp.StatusCode}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		// A 200 whose body is not what MOTIS documents means we are talking
		// to something else, or to a version that changed shape. Status 0
		// distinguishes it from an HTTP-level refusal.
		c.logger().Debug("motis: response could not be decoded", "url", rawURL, "error", err)
		return &core.ErrMotisRejected{}
	}
	return nil
}

// resolvePlace turns a free-form name into coordinates, trying MOTIS's own
// index first and the configured fallback geocoder second.
//
// This is the same two-step shape core.Client uses for ODsay (station search,
// then geocoder). MOTIS's index takes the place of ODsay's station search, so
// a user who only ever names stations needs no external service at all.
func (c *Client) resolvePlace(ctx context.Context, name string) (core.Coordinate, error) {
	coord, err := c.geocodeViaMotis(ctx, name)
	if !errors.Is(err, errPlaceNotFound) {
		// A hit, or a failure the fallback cannot remedy.
		return coord, err
	}
	if c.Geocoder == nil {
		return core.Coordinate{}, errPlaceNotFound
	}

	coord, err = c.Geocoder.Resolve(ctx, name)
	switch {
	case err == nil:
		c.logger().Debug("motis: fallback geocoder resolved name", "name", name)
		return coord, nil
	case errors.Is(err, core.ErrGeocoderNotFound):
		return core.Coordinate{}, errPlaceNotFound
	default:
		// ErrGeocoderAuthFailed / ErrGeocoderForbidden / ErrGeocoderUnavailable
		// and *ErrGeocoderRejected keep their own identity: they are the
		// geocoder's problem, not the routing engine's, and the CLI already
		// has distinct codes and advice for each.
		return core.Coordinate{}, err
	}
}

func (c *Client) geocodeViaMotis(ctx context.Context, name string) (core.Coordinate, error) {
	rawURL := fmt.Sprintf("%s%s?text=%s", c.baseURL(), geocodePath, url.QueryEscape(name))

	var matches []match
	if err := c.doGet(ctx, rawURL, &matches); err != nil {
		return core.Coordinate{}, err
	}
	c.logger().Debug("motis: geocode", "name", name, "candidates", len(matches))
	if len(matches) == 0 {
		return core.Coordinate{}, errPlaceNotFound
	}
	// The top match, mirroring how the ODsay path takes station[0].
	return core.Coordinate{X: matches[0].Lon, Y: matches[0].Lat}, nil
}

// ErrNoData reports that the engine answered but has no timetable or map data
// loaded — it is running, and it is not going to route anything.
//
// It is separate from core's error set because it never reaches a route
// search: setup is the only caller, and its job is to refuse an endpoint that
// would fail every subsequent query. Catching this at configuration time is
// what keeps "the engine has no data" from arriving later disguised as "there
// is no route between these points" (spec 006 FR-016).
var ErrNoData = errors.New("motis: the engine is reachable but has no data loaded")

// probeQuery is a name any Korean transit dataset must contain. It is only
// ever used to tell "index loaded" from "index empty", never to route.
const probeQuery = "서울역"

// Probe reports whether the engine is usable: reachable, answering, and
// carrying data. A nil return means setup may save this endpoint.
//
// The geocode endpoint is used rather than the routing one because it needs
// no coordinates to call and its empty result is unambiguous — a plan request
// returning nothing could mean either "no data" or "genuinely no route".
func (c *Client) Probe(ctx context.Context) error {
	rawURL := fmt.Sprintf("%s%s?text=%s", c.baseURL(), geocodePath, url.QueryEscape(probeQuery))

	var matches []match
	if err := c.doGet(ctx, rawURL, &matches); err != nil {
		return err
	}
	if len(matches) == 0 {
		return ErrNoData
	}
	return nil
}

// FindRoute searches for the representative transit route between from and to.
func (c *Client) FindRoute(ctx context.Context, from, to string) (result core.RouteResult, err error) {
	start := time.Now()
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		c.logger().Info("motis: route search",
			"from", from, "to", to, "outcome", outcome,
			"duration_ms", time.Since(start).Milliseconds())
	}()

	fromCoord, err := c.resolvePlace(ctx, from)
	if err != nil {
		if errors.Is(err, errPlaceNotFound) {
			return core.RouteResult{}, &core.ErrPointNotFound{Side: "from", Name: from}
		}
		return core.RouteResult{}, err
	}
	toCoord, err := c.resolvePlace(ctx, to)
	if err != nil {
		if errors.Is(err, errPlaceNotFound) {
			return core.RouteResult{}, &core.ErrPointNotFound{Side: "to", Name: to}
		}
		return core.RouteResult{}, err
	}

	// numItineraries=1: only the representative route is rendered, matching
	// what the ODsay path does with path[0].
	rawURL := fmt.Sprintf("%s%s?fromPlace=%s&toPlace=%s&numItineraries=1",
		c.baseURL(), planPath,
		url.QueryEscape(formatPlace(fromCoord)),
		url.QueryEscape(formatPlace(toCoord)))

	var resp planResponse
	if err := c.doGet(ctx, rawURL, &resp); err != nil {
		return core.RouteResult{}, err
	}

	chosen, ok := pickItinerary(resp)
	if !ok {
		return core.RouteResult{}, core.ErrNoRoute
	}
	return toRouteResult(chosen), nil
}

// formatPlace renders a coordinate the way /api/v6/plan expects it:
// "latitude,longitude".
func formatPlace(c core.Coordinate) string {
	return fmt.Sprintf("%.6f,%.6f", c.Y, c.X)
}

// pickItinerary returns the itinerary to render. Transit results win; a
// direct (walk-only) result is used when there are none, because "you can
// walk there in ten minutes" is a better answer than "no route exists".
func pickItinerary(resp planResponse) (itinerary, bool) {
	if len(resp.Itineraries) > 0 {
		return resp.Itineraries[0], true
	}
	if len(resp.Direct) > 0 {
		return resp.Direct[0], true
	}
	return itinerary{}, false
}

func toRouteResult(it itinerary) core.RouteResult {
	steps := make([]core.RouteStep, 0, len(it.Legs))
	for _, l := range it.Legs {
		steps = append(steps, core.RouteStep{Description: describeLeg(l)})
	}
	return core.RouteResult{
		TotalTime:     secondsToMinutes(it.Duration),
		TransferCount: it.Transfers,
		// FareKnown stays false: see planResponse. Reporting 0 here would
		// present every self-hosted journey as free (spec 006 FR-010).
		FareKnown: false,
		Steps:     steps,
	}
}

// secondsToMinutes rounds to the nearest minute, never below zero.
func secondsToMinutes(seconds float64) int {
	if seconds <= 0 {
		return 0
	}
	return int(seconds/60 + 0.5)
}

// describeLeg renders one leg as the Korean sentence the CLI prints. The
// phrasing follows internal/core's ODsay descriptions so a user cannot tell
// which engine answered from the shape of the output.
func describeLeg(l leg) string {
	fromName := strings.TrimSpace(l.From.Name)
	toName := strings.TrimSpace(l.To.Name)
	minutes := secondsToMinutes(l.Duration)

	if strings.EqualFold(l.Mode, "WALK") {
		return fmt.Sprintf("%s에서 %s까지 도보 이동%s", fromName, toName, durationSuffix(minutes))
	}

	service := serviceName(l)
	if service == "" {
		return fmt.Sprintf("%s에서 %s까지 이동%s", fromName, toName, durationSuffix(minutes))
	}
	if isBus(l.Mode) {
		return fmt.Sprintf("%s에서 %s 버스 승차 → %s에서 하차", fromName, service, toName)
	}
	return fmt.Sprintf("%s에서 %s 승차 → %s에서 하차", fromName, service, toName)
}

// serviceName picks the most useful label for a transit leg: the route number
// or line name if there is one, else where it is headed, else who runs it.
func serviceName(l leg) string {
	for _, candidate := range []string{l.RouteShortName, l.Headsign, l.AgencyName} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isBus(mode string) bool {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "BUS", "COACH", "TROLLEYBUS":
		return true
	default:
		return false
	}
}

func durationSuffix(minutes int) string {
	if minutes <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%d분)", minutes)
}
