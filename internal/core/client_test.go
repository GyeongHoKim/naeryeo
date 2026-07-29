package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newTestServer wires station and path search handlers into one
// httptest.Server, routed by the ODsay path suffix.
func newTestServer(t *testing.T, station, path http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	if station != nil {
		mux.HandleFunc("/searchStation", station)
	}
	if path != nil {
		mux.HandleFunc("/searchPubTransPathT", path)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
}

func stationHandler(t *testing.T, byName map[string]stationCandidate) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("stationName")
		cand, ok := byName[name]
		if !ok {
			writeJSON(t, w, stationSearchResponse{Result: &struct {
				Station []stationCandidate `json:"station"`
			}{Station: nil}})
			return
		}
		writeJSON(t, w, stationSearchResponse{Result: &struct {
			Station []stationCandidate `json:"station"`
		}{Station: []stationCandidate{cand}}})
	}
}

func TestFindRoute_Success(t *testing.T) {
	station := stationHandler(t, map[string]stationCandidate{
		"강남역":   {Name: "강남역", X: 127.0276, Y: 37.4979},
		"홍대입구역": {Name: "홍대입구역", X: 126.9236, Y: 37.5572},
	})

	path := func(w http.ResponseWriter, r *http.Request) {
		resp := pathSearchResponse{Result: &struct {
			Path []pathCandidate `json:"path"`
		}{Path: []pathCandidate{{
			SubPath: []subPathSegment{
				{TrafficType: 3, Distance: 120, SectionTime: 2},
				{TrafficType: 1, StartName: "강남역", EndName: "신도림역", Lane: []laneInfo{{Name: "2호선"}}},
				{TrafficType: 1, StartName: "신도림역", EndName: "홍대입구역", Lane: []laneInfo{{Name: "경의중앙선"}}},
				{TrafficType: 3, Distance: 80, SectionTime: 1},
			},
		}}}}
		resp.Result.Path[0].Info.TotalTime = 42
		resp.Result.Path[0].Info.Payment = 1500
		resp.Result.Path[0].Info.SubwayTransitCount = 2 // two subway boardings = one transfer
		writeJSON(t, w, resp)
	}

	srv := newTestServer(t, station, path)
	c := &Client{APIKey: "test-key", BaseURL: srv.URL}

	got, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
	if err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}
	if got.NoTravelNeeded {
		t.Fatal("NoTravelNeeded = true, want false")
	}
	if got.TotalTime != 42 {
		t.Errorf("TotalTime = %d, want 42", got.TotalTime)
	}
	if got.Fare != 1500 {
		t.Errorf("Fare = %d, want 1500", got.Fare)
	}
	if got.TransferCount != 1 {
		t.Errorf("TransferCount = %d, want 1", got.TransferCount)
	}
	if len(got.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(got.Steps))
	}
	wantSteps := []string{
		"도보 2분 이동 (120m)",
		"강남역에서 2호선 승차 → 신도림역에서 하차",
		"신도림역에서 경의중앙선 승차 → 홍대입구역에서 하차",
		"도보 1분 이동 (80m)",
	}
	for i, want := range wantSteps {
		if got.Steps[i].Description != want {
			t.Errorf("Steps[%d].Description = %q, want %q", i, got.Steps[i].Description, want)
		}
	}
}

func TestFindRoute_NoTransfer(t *testing.T) {
	station := stationHandler(t, map[string]stationCandidate{
		"A": {Name: "A", X: 127.0, Y: 37.0},
		"B": {Name: "B", X: 127.1, Y: 37.1},
	})
	path := func(w http.ResponseWriter, r *http.Request) {
		resp := pathSearchResponse{Result: &struct {
			Path []pathCandidate `json:"path"`
		}{Path: []pathCandidate{{
			SubPath: []subPathSegment{{TrafficType: 2, StartName: "A", EndName: "B", Lane: []laneInfo{{Name: "1000번"}}}},
		}}}}
		resp.Result.Path[0].Info.TotalTime = 10
		resp.Result.Path[0].Info.Payment = 1200
		resp.Result.Path[0].Info.BusTransitCount = 1 // a single boarding must be zero transfers
		writeJSON(t, w, resp)
	}

	srv := newTestServer(t, station, path)
	c := &Client{APIKey: "test-key", BaseURL: srv.URL}

	got, err := c.FindRoute(context.Background(), "A", "B")
	if err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}
	if got.TransferCount != 0 {
		t.Errorf("TransferCount = %d, want 0 (one boarding is zero transfers)", got.TransferCount)
	}
}

func TestTransferCount(t *testing.T) {
	tests := []struct {
		subway, bus, want int
	}{
		{0, 0, 0}, // walk-only
		{1, 0, 0}, // single subway ride (the reported bug)
		{0, 1, 0}, // single bus ride
		{2, 0, 1}, // subway → subway transfer
		{1, 1, 1}, // subway → bus transfer
		{2, 1, 2}, // three boardings, two transfers
	}
	for _, tt := range tests {
		if got := transferCount(tt.subway, tt.bus); got != tt.want {
			t.Errorf("transferCount(subway=%d, bus=%d) = %d, want %d", tt.subway, tt.bus, got, tt.want)
		}
	}
}

func TestFindRoute_NoTravelNeeded(t *testing.T) {
	station := stationHandler(t, map[string]stationCandidate{
		"강남역": {Name: "강남역", X: 127.0276, Y: 37.4979},
	})
	path := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, pathSearchResponse{Error: &odsayErrorBody{Code: "-98", Message: "too close"}})
	}

	srv := newTestServer(t, station, path)
	c := &Client{APIKey: "test-key", BaseURL: srv.URL}

	got, err := c.FindRoute(context.Background(), "강남역", "강남역")
	if err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}
	if !got.NoTravelNeeded {
		t.Fatal("NoTravelNeeded = false, want true")
	}
	if got.TotalTime != 0 || got.Fare != 0 || got.TransferCount != 0 || len(got.Steps) != 0 {
		t.Errorf("expected zero-value fields when NoTravelNeeded, got %+v", got)
	}
}

func TestFindRoute_SlowUpstreamRespectsContextDeadline(t *testing.T) {
	station := stationHandler(t, map[string]stationCandidate{
		"A": {Name: "A", X: 127.0, Y: 37.0},
		"B": {Name: "B", X: 127.1, Y: 37.1},
	})
	path := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	}

	srv := newTestServer(t, station, path)
	c := &Client{APIKey: "test-key", BaseURL: srv.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.FindRoute(ctx, "A", "B")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FindRoute() error = nil, want a timeout-related error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FindRoute() did not return within the test's own guard timeout — looks hung")
	}
}

func TestFindRoute_APIKeyMissing(t *testing.T) {
	called := false
	station := func(w http.ResponseWriter, r *http.Request) { called = true }
	path := func(w http.ResponseWriter, r *http.Request) { called = true }

	srv := newTestServer(t, station, path)
	c := &Client{APIKey: "", BaseURL: srv.URL}

	_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
	if !errors.Is(err, ErrAPIKeyMissing) {
		t.Fatalf("FindRoute() error = %v, want ErrAPIKeyMissing", err)
	}
	if called {
		t.Fatal("no HTTP call should have been made when the API key is missing")
	}
}

func TestFindRoute_AuthFailed(t *testing.T) {
	station := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	srv := newTestServer(t, station, nil)
	c := &Client{APIKey: "bad-key", BaseURL: srv.URL}

	_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("FindRoute() error = %v, want ErrAuthFailed", err)
	}
}

func TestFindRoute_PointNotFound(t *testing.T) {
	tests := []struct {
		name       string
		stationMap map[string]stationCandidate
		wantSide   string
	}{
		{
			name:       "from not found",
			stationMap: map[string]stationCandidate{"홍대입구역": {Name: "홍대입구역", X: 126.9, Y: 37.5}},
			wantSide:   "from",
		},
		{
			name:       "to not found",
			stationMap: map[string]stationCandidate{"강남역": {Name: "강남역", X: 127.0, Y: 37.5}},
			wantSide:   "to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			station := stationHandler(t, tt.stationMap)
			srv := newTestServer(t, station, nil)
			c := &Client{APIKey: "test-key", BaseURL: srv.URL}

			_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
			var pointErr *ErrPointNotFound
			if !errors.As(err, &pointErr) {
				t.Fatalf("FindRoute() error = %v, want *ErrPointNotFound", err)
			}
			if pointErr.Side != tt.wantSide {
				t.Errorf("Side = %q, want %q", pointErr.Side, tt.wantSide)
			}
		})
	}
}

func TestFindRoute_NoRouteAndUpstreamErrors(t *testing.T) {
	validStations := map[string]stationCandidate{
		"강남역":   {Name: "강남역", X: 127.0, Y: 37.5},
		"홍대입구역": {Name: "홍대입구역", X: 126.9, Y: 37.5},
	}

	tests := []struct {
		name    string
		path    http.HandlerFunc
		checkFn func(t *testing.T, err error)
	}{
		{
			name: "ODsay code 6 (out of service area) maps to ErrNoRoute",
			path: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, pathSearchResponse{Error: &odsayErrorBody{Code: "6", Message: "out of service area"}})
			},
			checkFn: func(t *testing.T, err error) {
				if !errors.Is(err, ErrNoRoute) {
					t.Fatalf("error = %v, want ErrNoRoute", err)
				}
			},
		},
		{
			name: "ODsay code -99 (no result) maps to ErrNoRoute",
			path: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, pathSearchResponse{Error: &odsayErrorBody{Code: "-99", Message: "no result"}})
			},
			checkFn: func(t *testing.T, err error) {
				if !errors.Is(err, ErrNoRoute) {
					t.Fatalf("error = %v, want ErrNoRoute", err)
				}
			},
		},
		{
			name: "HTTP 500 maps to ErrUpstreamUnavailable",
			path: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			checkFn: func(t *testing.T, err error) {
				if !errors.Is(err, ErrUpstreamUnavailable) {
					t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
				}
			},
		},
		{
			name: "malformed JSON maps to ErrUpstreamUnavailable",
			path: func(w http.ResponseWriter, r *http.Request) {
				if _, err := fmt.Fprint(w, "{not valid json"); err != nil {
					t.Logf("write malformed response: %v", err)
				}
			},
			checkFn: func(t *testing.T, err error) {
				if !errors.Is(err, ErrUpstreamUnavailable) {
					t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
				}
			},
		},
		{
			name: "unclassified ODsay code maps to ErrUpstreamRejected",
			path: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, pathSearchResponse{Error: &odsayErrorBody{Code: "-9", Message: "missing required param"}})
			},
			checkFn: func(t *testing.T, err error) {
				var rejected *ErrUpstreamRejected
				if !errors.As(err, &rejected) {
					t.Fatalf("error = %v, want *ErrUpstreamRejected", err)
				}
				if rejected.Code != "-9" {
					t.Errorf("Code = %q, want %q", rejected.Code, "-9")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			station := stationHandler(t, validStations)
			srv := newTestServer(t, station, tt.path)
			c := &Client{APIKey: "test-key", BaseURL: srv.URL}

			_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
			if err == nil {
				t.Fatal("FindRoute() error = nil, want non-nil")
			}
			tt.checkFn(t, err)
		})
	}
}

// fakeGeocoder is a test double for the Geocoder interface. It records how
// many times it was called so tests can assert the geocoder is NOT consulted
// when station search already succeeds (FR-003).
type fakeGeocoder struct {
	coord Coordinate
	err   error
	calls int
}

func (f *fakeGeocoder) Resolve(_ context.Context, _ string) (Coordinate, error) {
	f.calls++
	return f.coord, f.err
}

// okPathHandler returns a minimal valid single-leg route, enough for
// FindRoute to succeed once both endpoints resolve.
func okPathHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := pathSearchResponse{Result: &struct {
			Path []pathCandidate `json:"path"`
		}{Path: []pathCandidate{{
			SubPath: []subPathSegment{{TrafficType: 2, StartName: "A", EndName: "B", Lane: []laneInfo{{Name: "1000번"}}}},
		}}}}
		resp.Result.Path[0].Info.TotalTime = 10
		resp.Result.Path[0].Info.Payment = 1200
		writeJSON(t, w, resp)
	}
}

func TestFindRoute_StationSuccessSkipsGeocoder(t *testing.T) {
	station := stationHandler(t, map[string]stationCandidate{
		"강남역":   {Name: "강남역", X: 127.0, Y: 37.5},
		"홍대입구역": {Name: "홍대입구역", X: 126.9, Y: 37.5},
	})
	srv := newTestServer(t, station, okPathHandler(t))
	geo := &fakeGeocoder{coord: Coordinate{X: 1, Y: 1}}
	c := &Client{APIKey: "test-key", BaseURL: srv.URL, Geocoder: geo}

	if _, err := c.FindRoute(context.Background(), "강남역", "홍대입구역"); err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}
	if geo.calls != 0 {
		t.Errorf("geocoder was called %d times, want 0 when station search succeeds (FR-003)", geo.calls)
	}
}

func TestFindRoute_GeocoderFallbackSuccess(t *testing.T) {
	// Neither endpoint is a known station: both must resolve via the geocoder.
	station := stationHandler(t, map[string]stationCandidate{})
	srv := newTestServer(t, station, okPathHandler(t))
	geo := &fakeGeocoder{coord: Coordinate{X: 127.11, Y: 37.33}}
	c := &Client{APIKey: "test-key", BaseURL: srv.URL, Geocoder: geo}

	got, err := c.FindRoute(context.Background(), "아이디스 타워", "수지구청")
	if err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}
	if got.TotalTime != 10 || got.Fare != 1200 {
		t.Errorf("unexpected route result %+v", got)
	}
	if geo.calls != 2 {
		t.Errorf("geocoder calls = %d, want 2 (from + to)", geo.calls)
	}
}

func TestFindRoute_GeocoderMixedInput(t *testing.T) {
	// from resolves as a station, to falls back to the geocoder.
	station := stationHandler(t, map[string]stationCandidate{
		"강남역": {Name: "강남역", X: 127.0, Y: 37.5},
	})
	srv := newTestServer(t, station, okPathHandler(t))
	geo := &fakeGeocoder{coord: Coordinate{X: 127.11, Y: 37.33}}
	c := &Client{APIKey: "test-key", BaseURL: srv.URL, Geocoder: geo}

	if _, err := c.FindRoute(context.Background(), "강남역", "아이디스 타워"); err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}
	if geo.calls != 1 {
		t.Errorf("geocoder calls = %d, want 1 (only the to side)", geo.calls)
	}
}

func TestFindRoute_GeocoderErrorMapping(t *testing.T) {
	tests := []struct {
		name    string
		geo     *fakeGeocoder
		checkFn func(t *testing.T, err error)
	}{
		{
			name: "geocoder not found maps to ErrPointNotFound on the from side",
			geo:  &fakeGeocoder{err: ErrGeocoderNotFound},
			checkFn: func(t *testing.T, err error) {
				var pointErr *ErrPointNotFound
				if !errors.As(err, &pointErr) {
					t.Fatalf("error = %v, want *ErrPointNotFound", err)
				}
				if pointErr.Side != "from" {
					t.Errorf("Side = %q, want %q", pointErr.Side, "from")
				}
			},
		},
		{
			name: "geocoder auth failure propagates as ErrGeocoderAuthFailed",
			geo:  &fakeGeocoder{err: ErrGeocoderAuthFailed},
			checkFn: func(t *testing.T, err error) {
				if !errors.Is(err, ErrGeocoderAuthFailed) {
					t.Fatalf("error = %v, want ErrGeocoderAuthFailed", err)
				}
			},
		},
		{
			name: "geocoder unavailable propagates as ErrGeocoderUnavailable",
			geo:  &fakeGeocoder{err: ErrGeocoderUnavailable},
			checkFn: func(t *testing.T, err error) {
				if !errors.Is(err, ErrGeocoderUnavailable) {
					t.Fatalf("error = %v, want ErrGeocoderUnavailable", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			station := stationHandler(t, map[string]stationCandidate{})
			srv := newTestServer(t, station, okPathHandler(t))
			c := &Client{APIKey: "test-key", BaseURL: srv.URL, Geocoder: tt.geo}

			_, err := c.FindRoute(context.Background(), "아이디스 타워", "수지구청")
			if err == nil {
				t.Fatal("FindRoute() error = nil, want non-nil")
			}
			tt.checkFn(t, err)
		})
	}
}

func TestFindRoute_NoGeocoderKeepsLegacyNotFound(t *testing.T) {
	// With no Geocoder configured, an unrecognized name is ErrPointNotFound,
	// exactly as before this feature (FR-012 regression guard).
	station := stationHandler(t, map[string]stationCandidate{
		"수지구청": {Name: "수지구청", X: 127.1, Y: 37.3},
	})
	srv := newTestServer(t, station, okPathHandler(t))
	c := &Client{APIKey: "test-key", BaseURL: srv.URL} // Geocoder is nil

	_, err := c.FindRoute(context.Background(), "아이디스 타워", "수지구청")
	var pointErr *ErrPointNotFound
	if !errors.As(err, &pointErr) {
		t.Fatalf("FindRoute() error = %v, want *ErrPointNotFound", err)
	}
	if pointErr.Side != "from" {
		t.Errorf("Side = %q, want %q", pointErr.Side, "from")
	}
}

// TestFindRoute_StationErrorCodeEngagesGeocoder guards the spec 004 research
// §3 risk: if ODsay's searchStation reports any error code instead of an
// empty result, resolveStation must normalize it to the not-found signal so
// the geocoder fallback engages. Every ODsay error code from the station
// search — not just the "not found" codes 3/4/5 — should trigger the
// fallback, because they all mean the name did not match a transit stop.
func TestFindRoute_StationErrorCodeEngagesGeocoder(t *testing.T) {
	codes := []struct {
		code, label string
	}{
		{"3", "from not found"},
		{"4", "to not found"},
		{"5", "both not found"},
		{"6", "out of service area"},
		{"-99", "no result"},
		{"-8", "format error"},
		{"-9", "missing required param"},
	}

	for _, tc := range codes {
		stationHandler := func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, stationSearchResponse{Error: &odsayErrorBody{Code: flexibleString(tc.code), Message: tc.label}})
		}

		t.Run("geocoder configured, code "+tc.code+" falls back", func(t *testing.T) {
			srv := newTestServer(t, stationHandler, okPathHandler(t))
			geo := &fakeGeocoder{coord: Coordinate{X: 127.11, Y: 37.33}}
			c := &Client{APIKey: "test-key", BaseURL: srv.URL, Geocoder: geo}

			if _, err := c.FindRoute(context.Background(), "아이디스 타워", "수지구청"); err != nil {
				t.Fatalf("FindRoute() error = %v, want nil (fallback should engage for code %s)", err, tc.code)
			}
			if geo.calls == 0 {
				t.Errorf("geocoder was never called; code %s did not normalize to a not-found signal", tc.code)
			}
		})

		t.Run("no geocoder, code "+tc.code+" reports ErrPointNotFound", func(t *testing.T) {
			srv := newTestServer(t, stationHandler, okPathHandler(t))
			c := &Client{APIKey: "test-key", BaseURL: srv.URL} // no Geocoder

			_, err := c.FindRoute(context.Background(), "아이디스 타워", "수지구청")
			var pointErr *ErrPointNotFound
			if !errors.As(err, &pointErr) {
				t.Fatalf("FindRoute() error = %v, want *ErrPointNotFound", err)
			}
			if pointErr.Side != "from" {
				t.Errorf("Side = %q, want %q", pointErr.Side, "from")
			}
		})
	}
}

func TestFindRoute_ConnectionRefused(t *testing.T) {
	c := &Client{APIKey: "test-key", BaseURL: "http://127.0.0.1:1"}

	_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("FindRoute() error = %v, want ErrUpstreamUnavailable", err)
	}
}

// hangUpHandler answers a request by closing the TCP connection without
// writing a response, so the client's in-flight request fails at the
// transport layer (EOF/reset) rather than with an HTTP status.
func hangUpHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("ResponseWriter is not a http.Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		if err := conn.Close(); err != nil {
			t.Errorf("close hijacked conn: %v", err)
		}
	}
}

// TestFindRoute_TransportErrorNeverLeaksTheAPIKey is the security-critical
// proof for GYE-293: ODsay takes the API key as a query parameter, so any
// transport-layer failure yields a *url.Error whose message embeds the full
// request URL. Nothing that reaches a caller — the CLI's --debug dump or the
// MCP tool's error text — may contain the key.
//
// leakKey deliberately contains characters that url.QueryEscape rewrites, so
// a redaction that only searches for the raw key string does not pass.
func TestFindRoute_TransportErrorNeverLeaksTheAPIKey(t *testing.T) {
	const leakKey = "super/secret+odsay=key"

	stations := map[string]stationCandidate{
		"강남역":   {Name: "강남역", X: 127.0, Y: 37.5},
		"홍대입구역": {Name: "홍대입구역", X: 126.9, Y: 37.5},
	}

	tests := []struct {
		name    string
		baseURL func(t *testing.T) string
	}{
		{
			name:    "connection refused on station search",
			baseURL: func(*testing.T) string { return "http://127.0.0.1:1" },
		},
		{
			name: "upstream hangs up during station search",
			baseURL: func(t *testing.T) string {
				return newTestServer(t, hangUpHandler(t), nil).URL
			},
		},
		{
			name: "upstream hangs up during path search",
			baseURL: func(t *testing.T) string {
				return newTestServer(t, stationHandler(t, stations), hangUpHandler(t)).URL
			},
		},
		{
			name:    "unparseable base URL fails request construction",
			baseURL: func(*testing.T) string { return "http://[::1]:namedport" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Debug level, so the per-request log line that echoes the failing
			// URL and error is exercised too, not just the returned error.
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			c := &Client{APIKey: leakKey, BaseURL: tt.baseURL(t), Logger: logger}

			_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
			if !errors.Is(err, ErrUpstreamUnavailable) {
				t.Fatalf("FindRoute() error = %v, want ErrUpstreamUnavailable", err)
			}
			assertNoAPIKey(t, err.Error(), leakKey)
			assertNoAPIKey(t, logs.String(), leakKey)
		})
	}
}

// assertNoAPIKey fails if s contains apiKey in any form the request URL could
// carry it: verbatim, percent-encoded, or with '+' standing in for a space.
func assertNoAPIKey(t *testing.T, s, apiKey string) {
	t.Helper()
	for _, form := range []string{apiKey, url.QueryEscape(apiKey), url.PathEscape(apiKey)} {
		if strings.Contains(s, form) {
			t.Errorf("output leaked the API key (as %q):\n%s", form, s)
		}
	}
}
