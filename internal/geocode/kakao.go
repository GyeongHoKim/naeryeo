package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

const defaultBaseURL = "https://dapi.kakao.com"

// keywordSearchResponse mirrors the subset of Kakao Local "search by
// keyword" response fields this package consumes (contracts/geocode-kakao.md).
type keywordSearchResponse struct {
	Documents []struct {
		X string `json:"x"` // longitude, encoded as a string
		Y string `json:"y"` // latitude, encoded as a string
	} `json:"documents"`
}

// Kakao resolves place names to coordinates via the Kakao Local keyword
// search API. It satisfies core.Geocoder. The zero value is not usable;
// construct one with NewKakao.
type Kakao struct {
	// apiKey authenticates requests via the "Authorization: KakaoAK <key>"
	// header. It is never logged.
	apiKey string
	// HTTPClient is used for all requests. If nil, a client with a 10s
	// timeout is used.
	HTTPClient *http.Client
	// BaseURL overrides the Kakao API base URL. If empty, defaultBaseURL is
	// used. Tests substitute an httptest.Server URL here.
	BaseURL string
	// Logger receives diagnostic logs (query, HTTP status, outcome). If nil,
	// logging is discarded. The API key is never logged.
	Logger *slog.Logger
}

// NewKakao returns a Kakao geocoder that authenticates with apiKey.
func NewKakao(apiKey string) *Kakao {
	return &Kakao{apiKey: apiKey}
}

func (k *Kakao) httpClient() *http.Client {
	if k.HTTPClient != nil {
		return k.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (k *Kakao) baseURL() string {
	if k.BaseURL != "" {
		return k.BaseURL
	}
	return defaultBaseURL
}

func (k *Kakao) logger() *slog.Logger {
	if k.Logger != nil {
		return k.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// Resolve returns the coordinate of the top keyword-search match for query.
// It returns the Geocoder error-contract sentinels defined in core:
// core.ErrGeocoderNotFound if nothing matches, core.ErrGeocoderAuthFailed if
// the API key is rejected (HTTP 401/403), and core.ErrGeocoderUnavailable for
// any network error, timeout, non-2xx status, or unparseable response.
func (k *Kakao) Resolve(ctx context.Context, query string) (core.Coordinate, error) {
	// size=1: only the representative (top-ranked) match is needed;
	// sort=accuracy is Kakao's default but set explicitly for clarity.
	rawURL := fmt.Sprintf("%s/v2/local/search/keyword.json?query=%s&size=1&sort=accuracy",
		k.baseURL(), url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return core.Coordinate{}, fmt.Errorf("%w: %w", core.ErrGeocoderUnavailable, err)
	}
	req.Header.Set("Authorization", "KakaoAK "+k.apiKey)

	resp, err := k.httpClient().Do(req)
	if err != nil {
		k.logger().Debug("geocode: request failed", "query", query, "error", err)
		return core.Coordinate{}, fmt.Errorf("%w: %w", core.ErrGeocoderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	k.logger().Debug("geocode: keyword search", "query", query, "status", resp.StatusCode)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return core.Coordinate{}, core.ErrGeocoderAuthFailed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return core.Coordinate{}, fmt.Errorf("%w: unexpected HTTP status %d", core.ErrGeocoderUnavailable, resp.StatusCode)
	}

	var body keywordSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return core.Coordinate{}, fmt.Errorf("%w: %w", core.ErrGeocoderUnavailable, err)
	}
	if len(body.Documents) == 0 {
		return core.Coordinate{}, core.ErrGeocoderNotFound
	}

	doc := body.Documents[0]
	x, err := strconv.ParseFloat(strings.TrimSpace(doc.X), 64)
	if err != nil {
		return core.Coordinate{}, fmt.Errorf("%w: parse longitude %q: %w", core.ErrGeocoderUnavailable, doc.X, err)
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(doc.Y), 64)
	if err != nil {
		return core.Coordinate{}, fmt.Errorf("%w: parse latitude %q: %w", core.ErrGeocoderUnavailable, doc.Y, err)
	}
	return core.Coordinate{X: x, Y: y}, nil
}
