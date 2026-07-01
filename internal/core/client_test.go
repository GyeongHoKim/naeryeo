package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
				{TrafficType: 1, StartName: "강남역", EndName: "신도림역", Lane: []laneInfo{{Name: "2호선"}}},
				{TrafficType: 1, StartName: "신도림역", EndName: "홍대입구역", Lane: []laneInfo{{Name: "경의중앙선"}}},
			},
		}}}}
		resp.Result.Path[0].Info.TotalTime = 42
		resp.Result.Path[0].Info.Payment = 1500
		resp.Result.Path[0].Info.SubwayTransitCount = 1
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
	if len(got.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(got.Steps))
	}
	if got.Steps[0].Description == "" {
		t.Error("Steps[0].Description is empty")
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
		writeJSON(t, w, resp)
	}

	srv := newTestServer(t, station, path)
	c := &Client{APIKey: "test-key", BaseURL: srv.URL}

	got, err := c.FindRoute(context.Background(), "A", "B")
	if err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}
	if got.TransferCount != 0 {
		t.Errorf("TransferCount = %d, want 0", got.TransferCount)
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

func TestFindRoute_ConnectionRefused(t *testing.T) {
	c := &Client{APIKey: "test-key", BaseURL: "http://127.0.0.1:1"}

	_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("FindRoute() error = %v, want ErrUpstreamUnavailable", err)
	}
}
