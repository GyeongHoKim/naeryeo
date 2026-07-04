package tmap

import (
	"bytes"
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

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// defaultCallTimeout mirrors internal/motis's budget: the tool chain
// (geocode from + geocode to + plan, sequential) is bounded end-to-end by
// the handler's 2.5s context deadline, so each call needs headroom under
// that while tolerating a first-call TLS handshake.
const defaultCallTimeout = 2000 * time.Millisecond

const defaultBaseURL = "https://apis.openapi.sk.com"

var (
	// ErrPlaceNotFound indicates a place name matched no POI.
	ErrPlaceNotFound = errors.New("tmap: no matching place")
	// ErrNoRoute indicates both places resolved but TMAP returned no
	// itinerary connecting them.
	ErrNoRoute = errors.New("tmap: no transit route between the two points")
	// ErrQuotaExceeded indicates SK Open API rejected the request with
	// HTTP 429 — the app key's daily/monthly call quota (the free 대중교통
	// tier is 10 calls/day) has been used up. Re-registering the same key
	// will NOT help; the caller must wait for the quota to reset or move
	// to a paid tier, so this is kept distinct from ErrUnavailable.
	ErrQuotaExceeded = errors.New("tmap: API call quota exceeded")
	// ErrUnavailable indicates a network error, timeout, non-2xx status
	// (other than 429), or unparseable response from the TMAP backend.
	ErrUnavailable = errors.New("tmap: backend unavailable")
)

// Place is a geocoded location — the input Plan() needs.
type Place struct {
	Name string
	Lat  float64
	Lon  float64
}

// Client calls SK Open API's TMAP products. The zero value is not usable;
// construct with NewClient. Mirrors internal/motis.Client's shape
// (BaseURL/HTTPClient/Logger with nil-safe logger).
type Client struct {
	// BaseURL overrides the SK Open API base URL. If empty, defaultBaseURL
	// is used. Tests substitute an httptest.Server URL here.
	BaseURL string
	// AppKey authenticates every request via the "appKey" header. It is
	// never logged.
	AppKey string
	// HTTPClient is the HTTP client used for API calls. NewClient sets a
	// timeout client; tests may substitute their own.
	HTTPClient *http.Client
	// Logger receives one Debug line per HTTP call. Nil discards logs.
	Logger *slog.Logger
}

// NewClient builds a Client for SK Open API with the default per-call
// timeout.
func NewClient(appKey string) *Client {
	return &Client{
		AppKey:     appKey,
		HTTPClient: &http.Client{Timeout: defaultCallTimeout},
	}
}

func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return defaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c *Client) logger() *slog.Logger {
	if c.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.Logger
}

// flexibleFloat unmarshals a JSON number TMAP may encode as either a JSON
// number or a quoted string — the POI search response quotes coordinates
// ("37.49804637") while the transit routes response does not.
type flexibleFloat float64

func (f *flexibleFloat) UnmarshalJSON(data []byte) error {
	v, err := strconv.ParseFloat(strings.Trim(string(data), `"`), 64)
	if err != nil {
		return fmt.Errorf("tmap: parse coordinate: %w", err)
	}
	*f = flexibleFloat(v)
	return nil
}

// poiSearchResponse mirrors the subset of GET /tmap/pois this package
// consumes.
type poiSearchResponse struct {
	SearchPoiInfo struct {
		Pois struct {
			POI []poiCandidate `json:"poi"`
		} `json:"pois"`
	} `json:"searchPoiInfo"`
}

type poiCandidate struct {
	Name string `json:"name"`
	// DataKind "1" is the place itself (e.g. a station); "2" is a
	// sub-entry such as a numbered exit. Stations are preferred as the
	// routing endpoint.
	DataKind string        `json:"dataKind"`
	FrontLat flexibleFloat `json:"frontLat"` // road-side entry point, used for routing
	FrontLon flexibleFloat `json:"frontLon"`
}

// planRequest is the POST /transit/routes request body.
type planRequest struct {
	StartX string `json:"startX"`
	StartY string `json:"startY"`
	EndX   string `json:"endX"`
	EndY   string `json:"endY"`
	Count  int    `json:"count"`
	Lang   int    `json:"lang"`
	Format string `json:"format"`
}

// planResponse mirrors the fields of POST /transit/routes this package
// consumes.
type planResponse struct {
	MetaData struct {
		Plan struct {
			Itineraries []itinerary `json:"itineraries"`
		} `json:"plan"`
	} `json:"metaData"`
}

type itinerary struct {
	TotalTime     int   `json:"totalTime"` // seconds
	TransferCount int   `json:"transferCount"`
	Legs          []leg `json:"legs"`
	Fare          *struct {
		Regular struct {
			TotalFare int `json:"totalFare"`
		} `json:"regular"`
	} `json:"fare"`
}

type leg struct {
	Mode        string   `json:"mode"` // WALK | SUBWAY | BUS | EXPRESSBUS | TRAIN | AIRPLANE | FERRY | ...
	Route       string   `json:"route"`
	SectionTime int      `json:"sectionTime"` // seconds
	Start       legPoint `json:"start"`
	End         legPoint `json:"end"`
}

type legPoint struct {
	Name string `json:"name"`
}

// classifyStatus maps an HTTP status code onto the package's error
// taxonomy. 2xx returns nil; every other code is a request-level failure
// distinguishable from a valid-but-empty result.
func classifyStatus(code int) error {
	switch {
	case code >= 200 && code <= 299:
		return nil
	case code == http.StatusTooManyRequests:
		return ErrQuotaExceeded
	default:
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, code)
	}
}

// newRequest builds an HTTP request carrying the appKey header and, for
// POST, a JSON-encoded body.
func (c *Client) newRequest(ctx context.Context, method, rawURL string, body any) (*http.Request, error) {
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: encode request: %w", ErrUnavailable, err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", ErrUnavailable, err)
	}
	req.Header.Set("appKey", c.AppKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// do issues req and decodes the JSON response body into out. Every failure
// mode other than a valid response maps onto ErrUnavailable or
// ErrQuotaExceeded; callers never see transport details.
func (c *Client) do(req *http.Request, out any) (err error) {
	start := time.Now()
	statusCode := 0
	defer func() {
		c.logger().Debug("tmap: HTTP call",
			"url", req.URL.String(),
			"method", req.Method,
			"status", statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err != nil,
		)
	}()

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
	if statusErr := classifyStatus(resp.StatusCode); statusErr != nil {
		return statusErr
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode response: %w", ErrUnavailable, err)
	}
	return nil
}

// Geocode resolves a Korean place name (station, stop, or landmark) to a
// Place via GET /tmap/pois. Selection rule: the first result with
// DataKind "1" (the place itself, e.g. a station) wins; if none is
// present the first result wins; an empty result is ErrPlaceNotFound.
func (c *Client) Geocode(ctx context.Context, name string) (Place, error) {
	q := url.Values{
		"version":       {"1"},
		"searchKeyword": {name},
		"searchType":    {"all"},
		"page":          {"1"},
		"count":         {"20"},
		"resCoordType":  {"WGS84GEO"},
		"reqCoordType":  {"WGS84GEO"},
	}
	req, err := c.newRequest(ctx, http.MethodGet, c.baseURL()+"/tmap/pois?"+q.Encode(), nil)
	if err != nil {
		return Place{}, fmt.Errorf("geocode %q: %w", name, err)
	}
	var resp poiSearchResponse
	if err := c.do(req, &resp); err != nil {
		return Place{}, fmt.Errorf("geocode %q: %w", name, err)
	}
	candidates := resp.SearchPoiInfo.Pois.POI
	if len(candidates) == 0 {
		return Place{}, fmt.Errorf("geocode %q: %w", name, ErrPlaceNotFound)
	}

	chosen := candidates[0]
	for _, p := range candidates {
		if p.DataKind == "1" {
			chosen = p
			break
		}
	}
	return Place{Name: chosen.Name, Lat: float64(chosen.FrontLat), Lon: float64(chosen.FrontLon)}, nil
}

// Plan finds the best transit itinerary between two resolved places via
// POST /transit/routes and maps it onto the shared core domain model. An
// empty itineraries array is ErrNoRoute.
func (c *Client) Plan(ctx context.Context, from, to Place) (core.RouteResult, error) {
	body := planRequest{
		StartX: strconv.FormatFloat(from.Lon, 'f', -1, 64),
		StartY: strconv.FormatFloat(from.Lat, 'f', -1, 64),
		EndX:   strconv.FormatFloat(to.Lon, 'f', -1, 64),
		EndY:   strconv.FormatFloat(to.Lat, 'f', -1, 64),
		Count:  1,
		Lang:   0,
		Format: "json",
	}
	req, err := c.newRequest(ctx, http.MethodPost, c.baseURL()+"/transit/routes", body)
	if err != nil {
		return core.RouteResult{}, fmt.Errorf("plan: %w", err)
	}
	var resp planResponse
	if err := c.do(req, &resp); err != nil {
		return core.RouteResult{}, fmt.Errorf("plan: %w", err)
	}
	itineraries := resp.MetaData.Plan.Itineraries
	if len(itineraries) == 0 {
		return core.RouteResult{}, ErrNoRoute
	}

	best := itineraries[0]
	fare := 0
	if best.Fare != nil {
		fare = best.Fare.Regular.TotalFare
	}
	steps := make([]core.RouteStep, 0, len(best.Legs))
	for _, l := range best.Legs {
		steps = append(steps, core.RouteStep{Description: describeLeg(l, from.Name, to.Name)})
	}
	return core.RouteResult{
		TotalTime:     (best.TotalTime + 30) / 60, // seconds → minutes, rounded
		TransferCount: best.TransferCount,
		Fare:          fare,
		Steps:         steps,
	}, nil
}

// FindRoute is the single entry point cmd/naeryeo uses: it resolves both
// names then plans the route. Geocode misses are translated into
// *core.ErrPointNotFound with the correct side so the cmd layer can reuse
// one error taxonomy across the ODsay, MOTIS, and TMAP backends.
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

// describeLeg renders one TMAP leg as a Korean step description. TMAP's
// synthetic "출발지"/"도착지" endpoint names are replaced with the
// user-facing place names.
func describeLeg(l leg, fromName, toName string) string {
	src := l.Start.Name
	if src == "출발지" {
		src = fromName
	}
	dst := l.End.Name
	if dst == "도착지" {
		dst = toName
	}

	if l.Mode == "WALK" {
		minutes := (l.SectionTime + 30) / 60
		if minutes < 1 {
			minutes = 1
		}
		return fmt.Sprintf("%s까지 도보 %d분", dst, minutes)
	}

	line := l.Route
	if line == "" {
		line = modeKorean(l.Mode)
	}
	return fmt.Sprintf("%s에서 %s 승차 → %s 하차", src, line, dst)
}

// modeKorean maps a TMAP transit mode onto its Korean rider-facing word.
// Unknown modes pass through unchanged rather than guessing.
func modeKorean(mode string) string {
	switch mode {
	case "SUBWAY", "SUBWAYBUS":
		return "지하철"
	case "BUS", "EXPRESSBUS", "WIDEAREA":
		return "버스"
	case "TRAIN":
		return "기차"
	case "FERRY":
		return "여객선"
	case "AIRPLANE":
		return "항공"
	default:
		return mode
	}
}
