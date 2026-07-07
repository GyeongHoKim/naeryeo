package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.odsay.com/v1/api"

// errStationNotFound is an internal signal from resolveStation meaning a
// name matched no real station/stop. FindRoute turns it into
// *ErrPointNotFound with the right Side.
var errStationNotFound = errors.New("core: no matching station")

// errNoTravelNeeded is an internal signal from classifyODsayError for ODsay
// code -98 (start/end within 700m). FindRoute turns it into a successful
// RouteResult{NoTravelNeeded: true}, not an error returned to callers.
var errNoTravelNeeded = errors.New("core: start and end are effectively the same location")

// flexibleFloat unmarshals a JSON number that ODsay may encode as either a
// JSON number or a quoted string (observed inconsistently across public
// Korean transit APIs).
type flexibleFloat float64

func (f *flexibleFloat) UnmarshalJSON(data []byte) error {
	v, err := strconv.ParseFloat(strings.Trim(string(data), `"`), 64)
	if err != nil {
		return fmt.Errorf("core: parse coordinate: %w", err)
	}
	*f = flexibleFloat(v)
	return nil
}

// flexibleString unmarshals a JSON value that may be either a quoted
// string or a bare number, storing it as a string either way. ODsay error
// codes are documented inconsistently as both.
type flexibleString string

func (s *flexibleString) UnmarshalJSON(data []byte) error {
	*s = flexibleString(strings.Trim(string(data), `"`))
	return nil
}

// odsayErrorBody mirrors the `error` object ODsay embeds in a JSON response
// when a request fails (research.md §3).
type odsayErrorBody struct {
	Code    flexibleString `json:"code"`
	Message string         `json:"message"`
}

// stationSearchResponse mirrors the searchStation endpoint's response
// shape (research.md §1; exact field set to be confirmed against a real
// API key, see plan.md's Constitution Check note).
type stationSearchResponse struct {
	Result *struct {
		Station []stationCandidate `json:"station"`
	} `json:"result"`
	Error *odsayErrorBody `json:"error"`
}

type stationCandidate struct {
	Name string        `json:"stationName"`
	X    flexibleFloat `json:"x"`
	Y    flexibleFloat `json:"y"`
}

// pathSearchResponse mirrors the searchPubTransPathT endpoint's response
// shape (research.md §2).
type pathSearchResponse struct {
	Result *struct {
		Path []pathCandidate `json:"path"`
	} `json:"result"`
	Error *odsayErrorBody `json:"error"`
}

type pathCandidate struct {
	Info struct {
		TotalTime          int `json:"totalTime"`
		Payment            int `json:"payment"`
		BusTransitCount    int `json:"busTransitCount"`
		SubwayTransitCount int `json:"subwayTransitCount"`
	} `json:"info"`
	SubPath []subPathSegment `json:"subPath"`
}

type subPathSegment struct {
	TrafficType int        `json:"trafficType"`
	Distance    float64    `json:"distance"`
	SectionTime int        `json:"sectionTime"`
	StartName   string     `json:"startName"`
	EndName     string     `json:"endName"`
	Lane        []laneInfo `json:"lane"`
}

type laneInfo struct {
	Name string `json:"name"`
}

// doGet issues an HTTP GET against rawURL and decodes the JSON response
// body into out. Network failures, timeouts, non-2xx statuses (other than
// 401/403), and malformed JSON are all reported as ErrUpstreamUnavailable.
// HTTP 401/403 are reported as ErrAuthFailed (research.md §3 — the exact
// signal ODsay uses for an invalid API key is unconfirmed; this is the
// best-effort mapping to verify against a real key, see tasks.md T023).
func (c *Client) doGet(ctx context.Context, rawURL string, out any) (err error) {
	start := time.Now()
	statusCode := 0
	defer func() {
		c.logger().Debug("core: HTTP GET",
			"url", redactURL(rawURL),
			"status", statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
	}()

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if reqErr != nil {
		return fmt.Errorf("%w: %w", ErrUpstreamUnavailable, reqErr)
	}

	resp, doErr := c.httpClient().Do(req)
	if doErr != nil {
		return fmt.Errorf("%w: %w", ErrUpstreamUnavailable, doErr)
	}
	statusCode = resp.StatusCode
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("%w: %w", ErrUpstreamUnavailable, cerr)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuthFailed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: unexpected HTTP status %d", ErrUpstreamUnavailable, resp.StatusCode)
	}

	if decErr := json.NewDecoder(resp.Body).Decode(out); decErr != nil {
		return fmt.Errorf("%w: %w", ErrUpstreamUnavailable, decErr)
	}
	return nil
}

// classifyODsayError maps an ODsay error object (research.md §3's code
// table) onto this package's domain errors. from/to fill in
// ErrPointNotFound.Name. It returns nil if body is nil (no error).
func (c *Client) classifyODsayError(body *odsayErrorBody, from, to string) error {
	if body == nil {
		return nil
	}
	c.logger().Warn("core: ODsay rejected request",
		"code", string(body.Code), "message", body.Message, "from", from, "to", to)
	switch string(body.Code) {
	case "3":
		return &ErrPointNotFound{Side: "from", Name: from}
	case "4":
		return &ErrPointNotFound{Side: "to", Name: to}
	case "5":
		return &ErrPointNotFound{Side: "both", Name: from + " / " + to}
	case "6", "-99":
		return ErrNoRoute
	case "-98":
		return errNoTravelNeeded
	default:
		return &ErrUpstreamRejected{Code: string(body.Code), Message: body.Message}
	}
}

// RouteResult is the outcome of a successful route search.
type RouteResult struct {
	// NoTravelNeeded is true when From and To are effectively the same
	// location (ODsay code -98). The remaining fields are zero-valued in
	// that case; this is not an error.
	NoTravelNeeded bool
	TotalTime      int // minutes
	TransferCount  int
	Fare           int // KRW
	Steps          []RouteStep
}

// RouteStep is one human-readable leg of a route.
type RouteStep struct {
	Description string
}

// Coordinate is a longitude/latitude pair (EPSG:4326), as required by
// ODsay's coordinate-based route search.
type Coordinate struct {
	X float64 // longitude
	Y float64 // latitude
}

// Geocoder resolves a free-form place name (building name, address, POI)
// into a Coordinate. It is defined here — in the consuming package — so
// core stays decoupled from any concrete geocoding backend; implementations
// live elsewhere (internal/geocode). A Geocoder MUST return a not-found
// error that unwraps to its own package's not-found sentinel when the name
// matches nothing, so resolveStation can map it to ErrPointNotFound.
type Geocoder interface {
	Resolve(ctx context.Context, query string) (Coordinate, error)
}

// Client searches transit routes via the ODsay API.
type Client struct {
	APIKey string
	// HTTPClient is used for all requests. If nil, a client with a 10s
	// timeout is used.
	HTTPClient *http.Client
	// BaseURL overrides the ODsay API base URL. If empty, defaultBaseURL is
	// used. Tests substitute an httptest.Server URL here.
	BaseURL string
	// Logger receives diagnostic logs (redacted URLs, HTTP status, ODsay
	// error codes, search outcomes). If nil, logging is discarded. Never
	// logs c.APIKey.
	Logger *slog.Logger
	// Geocoder, when non-nil, is used as a fallback to resolve a From/To
	// name that ODsay's station search does not recognize. When nil, no
	// fallback occurs and an unrecognized name yields ErrPointNotFound
	// exactly as before (spec 004 FR-012).
	Geocoder Geocoder
}

// NewClient returns a Client configured to authenticate with apiKey.
func NewClient(apiKey string) *Client {
	return &Client{APIKey: apiKey}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

// resolveStation resolves name to coordinates. It first tries ODsay's
// station search; if that recognizes no station AND a Geocoder is
// configured, it falls back to the geocoder so building names and addresses
// resolve too (spec 004). With no Geocoder configured it behaves exactly as
// before: an unrecognized name yields errStationNotFound.
func (c *Client) resolveStation(ctx context.Context, name string) (stationCandidate, error) {
	station, err := c.searchStationByName(ctx, name)
	if !errors.Is(err, errStationNotFound) {
		// Success, or a non-not-found error (auth/unavailable/rejected) that
		// the geocoder cannot remedy — return either as-is.
		return station, err
	}
	if c.Geocoder == nil {
		return stationCandidate{}, errStationNotFound
	}
	return c.geocodeFallback(ctx, name)
}

// searchStationByName looks up name via ODsay's station search and returns
// the first candidate's coordinates. It returns errStationNotFound if no
// candidate matches.
func (c *Client) searchStationByName(ctx context.Context, name string) (stationCandidate, error) {
	u := fmt.Sprintf("%s/searchStation?apiKey=%s&stationName=%s",
		c.baseURL(), url.QueryEscape(c.APIKey), url.QueryEscape(name))

	var resp stationSearchResponse
	if err := c.doGet(ctx, u, &resp); err != nil {
		return stationCandidate{}, err
	}
	if err := c.classifyODsayError(resp.Error, name, name); err != nil {
		if errors.Is(err, errNoTravelNeeded) {
			// -98 is meaningless for a single-point lookup; treat as not found.
			return stationCandidate{}, errStationNotFound
		}
		// Any ODsay application-level error from the station-search endpoint
		// (codes 3/4/5/6/-99/-8/-9 or any unknown code) means this name did
		// not match a transit stop. Fold to errStationNotFound so the
		// geocoder fallback gets a chance; when no geocoder is configured,
		// resolveStation still yields errStationNotFound and FindRoute
		// reports ErrPointNotFound with the correct side.
		return stationCandidate{}, errStationNotFound
	}
	candidateCount := 0
	if resp.Result != nil {
		candidateCount = len(resp.Result.Station)
	}
	c.logger().Debug("core: station search", "name", name, "candidates", candidateCount)
	if candidateCount == 0 {
		return stationCandidate{}, errStationNotFound
	}
	return resp.Result.Station[0], nil
}

// geocodeFallback resolves name via c.Geocoder and adapts the result to a
// stationCandidate (reusing the station path's coordinate type, so the rest
// of FindRoute is unchanged). A geocoder "not found" is folded back into
// errStationNotFound so FindRoute reports ErrPointNotFound for the right
// side; auth/unavailable errors propagate as their core sentinels.
func (c *Client) geocodeFallback(ctx context.Context, name string) (stationCandidate, error) {
	coord, err := c.Geocoder.Resolve(ctx, name)
	switch {
	case err == nil:
		c.logger().Debug("core: geocoder resolved name", "name", name)
		return stationCandidate{X: flexibleFloat(coord.X), Y: flexibleFloat(coord.Y)}, nil
	case errors.Is(err, ErrGeocoderNotFound):
		return stationCandidate{}, errStationNotFound
	default:
		// ErrGeocoderAuthFailed / ErrGeocoderUnavailable — surface as-is.
		return stationCandidate{}, err
	}
}

// FindRoute searches for the representative transit route between from and
// to. It returns ErrAPIKeyMissing without making any network call if the
// client has no API key configured (FR-007).
func (c *Client) FindRoute(ctx context.Context, from, to string) (result RouteResult, err error) {
	start := time.Now()
	defer func() {
		c.logger().Info("core: route search",
			"from", from, "to", to,
			"outcome", routeOutcome(result, err),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	if c.APIKey == "" {
		return RouteResult{}, ErrAPIKeyMissing
	}

	fromStation, err := c.resolveStation(ctx, from)
	if err != nil {
		if errors.Is(err, errStationNotFound) {
			return RouteResult{}, &ErrPointNotFound{Side: "from", Name: from}
		}
		return RouteResult{}, err
	}
	toStation, err := c.resolveStation(ctx, to)
	if err != nil {
		if errors.Is(err, errStationNotFound) {
			return RouteResult{}, &ErrPointNotFound{Side: "to", Name: to}
		}
		return RouteResult{}, err
	}

	u := fmt.Sprintf("%s/searchPubTransPathT?apiKey=%s&SX=%f&SY=%f&EX=%f&EY=%f&OPT=0",
		c.baseURL(), url.QueryEscape(c.APIKey), fromStation.X, fromStation.Y, toStation.X, toStation.Y)

	var resp pathSearchResponse
	if err := c.doGet(ctx, u, &resp); err != nil {
		return RouteResult{}, err
	}
	if err := c.classifyODsayError(resp.Error, from, to); err != nil {
		if errors.Is(err, errNoTravelNeeded) {
			return RouteResult{NoTravelNeeded: true}, nil
		}
		return RouteResult{}, err
	}
	if resp.Result == nil || len(resp.Result.Path) == 0 {
		return RouteResult{}, ErrNoRoute
	}

	return toRouteResult(resp.Result.Path[0]), nil
}

func toRouteResult(p pathCandidate) RouteResult {
	steps := make([]RouteStep, 0, len(p.SubPath))
	for _, sp := range p.SubPath {
		steps = append(steps, RouteStep{Description: describeSubPath(sp)})
	}
	return RouteResult{
		TotalTime:     p.Info.TotalTime,
		TransferCount: transferCount(p.Info.SubwayTransitCount, p.Info.BusTransitCount),
		Fare:          p.Info.Payment,
		Steps:         steps,
	}
}

// transferCount converts ODsay's per-mode boarding counts into a transfer
// count. subwayTransitCount/busTransitCount are the number of subway/bus
// *boardings*, so the number of transfers is one less than the total number
// of boardings (a single ride is zero transfers). Zero boardings (e.g. a
// walk-only path) yields zero, never a negative count.
func transferCount(subwayBoardings, busBoardings int) int {
	boardings := subwayBoardings + busBoardings
	if boardings <= 1 {
		return 0
	}
	return boardings - 1
}

func describeSubPath(sp subPathSegment) string {
	laneName := ""
	if len(sp.Lane) > 0 {
		laneName = sp.Lane[0].Name
	}
	startName := strings.TrimSpace(sp.StartName)
	endName := strings.TrimSpace(sp.EndName)

	switch sp.TrafficType {
	case 1: // subway
		return fmt.Sprintf("%s에서 %s 승차 → %s에서 하차", startName, laneName, endName)
	case 2: // bus
		return fmt.Sprintf("%s에서 %s 버스 승차 → %s에서 하차", startName, laneName, endName)
	case 3: // walk
		return describeWalkSubPath(sp, startName, endName)
	default:
		return describeGenericSubPath(sp, startName, endName)
	}
}

func describeWalkSubPath(sp subPathSegment, startName, endName string) string {
	switch {
	case startName != "" && endName != "":
		return fmt.Sprintf("%s에서 %s까지 도보 이동%s", startName, endName, segmentDetail(sp))
	case startName != "":
		return fmt.Sprintf("%s에서 도보 이동%s", startName, segmentDetail(sp))
	case endName != "":
		return fmt.Sprintf("%s까지 도보 이동%s", endName, segmentDetail(sp))
	default:
		return "도보" + movementSummary(sp)
	}
}

func describeGenericSubPath(sp subPathSegment, startName, endName string) string {
	switch {
	case startName != "" && endName != "":
		return fmt.Sprintf("%s에서 %s까지 이동%s", startName, endName, segmentDetail(sp))
	case startName != "":
		return fmt.Sprintf("%s에서 이동%s", startName, segmentDetail(sp))
	case endName != "":
		return fmt.Sprintf("%s까지 이동%s", endName, segmentDetail(sp))
	default:
		return "이동" + movementSummary(sp)
	}
}

func segmentDetail(sp subPathSegment) string {
	distance := int(sp.Distance + 0.5)
	details := make([]string, 0, 2)
	if sp.SectionTime > 0 {
		details = append(details, fmt.Sprintf("%d분", sp.SectionTime))
	}
	if distance > 0 {
		details = append(details, fmt.Sprintf("%dm", distance))
	}
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, ", ") + ")"
}

func movementSummary(sp subPathSegment) string {
	distance := int(sp.Distance + 0.5)
	switch {
	case sp.SectionTime > 0 && distance > 0:
		return fmt.Sprintf(" %d분 이동 (%dm)", sp.SectionTime, distance)
	case sp.SectionTime > 0:
		return fmt.Sprintf(" %d분 이동", sp.SectionTime)
	case distance > 0:
		return fmt.Sprintf(" %dm 이동", distance)
	default:
		return " 이동"
	}
}
