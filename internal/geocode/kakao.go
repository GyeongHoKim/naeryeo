package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

const defaultBaseURL = "https://dapi.kakao.com"
const keywordPath = "/v2/local/search/keyword.json"
const addressPath = "/v2/local/search/address.json"

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

// Resolve returns the coordinate of the top match for query, trying keyword
// search first and falling back to address search when the keyword endpoint
// finds nothing. It returns the Geocoder error-contract sentinels defined in
// core: core.ErrGeocoderNotFound if nothing matches either endpoint,
// core.ErrGeocoderAuthFailed if the key is rejected (HTTP 401),
// core.ErrGeocoderForbidden if the request is denied (HTTP 403), and
// core.ErrGeocoderUnavailable for any network error, timeout, other non-2xx
// status, or unparseable response.
func (k *Kakao) Resolve(ctx context.Context, query string) (core.Coordinate, error) {
	coord, err := k.doSearch(ctx, keywordPath, query)
	if errors.Is(err, core.ErrGeocoderNotFound) {
		return k.doSearch(ctx, addressPath, query)
	}
	return coord, err
}

// doSearch issues a GET against the Kakao Local endpoint at path (e.g.
// /v2/local/search/keyword.json or /v2/local/search/address.json), parses
// the response, and returns the coordinate of the top match.
func (k *Kakao) doSearch(ctx context.Context, path, query string) (core.Coordinate, error) {
	// size=1: only the representative (top-ranked) match is needed;
	// sort=accuracy is Kakao's default but set explicitly for clarity.
	rawURL := fmt.Sprintf("%s%s?query=%s&size=1&sort=accuracy",
		k.baseURL(), path, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return core.Coordinate{}, fmt.Errorf("%w: %w", core.ErrGeocoderUnavailable, err)
	}
	req.Header.Set("Authorization", "KakaoAK "+k.apiKey)

	resp, err := k.httpClient().Do(req)
	if err != nil {
		k.logger().Debug("geocode: request failed", "path", path, "query", query, "error", err)
		return core.Coordinate{}, fmt.Errorf("%w: %w", core.ErrGeocoderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	k.logger().Debug("geocode: search", "path", path, "query", query, "status", resp.StatusCode)

	if resp.StatusCode == http.StatusUnauthorized {
		return core.Coordinate{}, core.ErrGeocoderAuthFailed
	}
	if resp.StatusCode == http.StatusForbidden {
		// 403 = key accepted but request denied (service not enabled for the
		// app, or domain/IP restriction). Distinct from an invalid key (401).
		return core.Coordinate{}, core.ErrGeocoderForbidden
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// 4xx = Kakao rejected the request itself. For the local API this is
		// almost always HTTP 400, which folds several distinct causes into one
		// status (e.g. code -2 invalid params, -10 call-frequency exceeded).
		// Read the body to recover the provider code/message so the caller can
		// show the real reason instead of a generic "unavailable".
		code, message := readErrorBody(resp.Body)
		k.logger().Debug("geocode: request rejected", "path", path,
			"query", query, "status", resp.StatusCode, "code", code, "message", message)
		return core.Coordinate{}, &core.ErrGeocoderRejected{Status: resp.StatusCode, Code: code, Message: message}
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

// readErrorBody extracts a provider error code and message from a Kakao error
// response. Kakao's REST errors use either {"errorType","message"} (dapi local)
// or {"code","msg"} (platform APIs); a body that is not one of those shapes is
// returned verbatim (trimmed) as the message so nothing is lost. Reads are
// capped so a misbehaving upstream cannot stream an unbounded body.
func readErrorBody(r io.Reader) (code, message string) {
	raw, err := io.ReadAll(io.LimitReader(r, 2<<10))
	if err != nil {
		return "", ""
	}

	var eb struct {
		ErrorType string      `json:"errorType"`
		Message   string      `json:"message"`
		Code      json.Number `json:"code"`
		Msg       string      `json:"msg"`
	}
	if err := json.Unmarshal(raw, &eb); err == nil {
		code = eb.ErrorType
		if code == "" && eb.Code.String() != "" {
			code = eb.Code.String()
		}
		message = eb.Message
		if message == "" {
			message = eb.Msg
		}
	}
	if message == "" {
		message = strings.TrimSpace(string(raw))
	}
	return code, message
}
