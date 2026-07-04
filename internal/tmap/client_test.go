package tmap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// fixture reads a testdata file or fails the test.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// newFakeTMAP starts an httptest server that answers GET /tmap/pois with
// poisBody and POST /transit/routes with planBody, and returns a Client
// wired to it.
func newFakeTMAP(t *testing.T, poisBody, planBody string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tmap/pois":
			_, _ = w.Write([]byte(poisBody))
		case "/transit/routes":
			_, _ = w.Write([]byte(planBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient("test-app-key")
	c.BaseURL = srv.URL
	return c
}

func TestGeocodeSelection(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantName string
		wantLat  float64
		wantErr  error
	}{
		{
			name:     "prefers first dataKind=1 even with earlier dataKind=2",
			body:     `{"searchPoiInfo":{"pois":{"poi":[{"name":"강남역 1번출구","dataKind":"2","frontLat":"37.1","frontLon":"127.1"},{"name":"강남역","dataKind":"1","frontLat":"37.498","frontLon":"127.028"}]}}}`,
			wantName: "강남역",
			wantLat:  37.498,
		},
		{
			name:     "falls back to first match when no dataKind=1",
			body:     `{"searchPoiInfo":{"pois":{"poi":[{"name":"어딘가 3번출구","dataKind":"2","frontLat":"35.0","frontLon":"129.0"},{"name":"어딘가 4번출구","dataKind":"2","frontLat":"36.0","frontLon":"128.0"}]}}}`,
			wantName: "어딘가 3번출구",
			wantLat:  35.0,
		},
		{
			name:     "real measured fixture with quoted string coordinates",
			body:     fixture(t, "pois_gangnam.json"),
			wantName: "강남역[2호선]",
			wantLat:  37.49804637,
		},
		{
			name:    "empty array is ErrPlaceNotFound",
			body:    fixture(t, "pois_empty.json"),
			wantErr: ErrPlaceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeTMAP(t, tt.body, `{}`)
			got, err := c.Geocode(context.Background(), "강남역")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Geocode() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Geocode() unexpected error: %v", err)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Lat != tt.wantLat {
				t.Errorf("Lat = %v, want %v", got.Lat, tt.wantLat)
			}
		})
	}
}

func TestPlanMapsItineraryOntoRouteResult(t *testing.T) {
	c := newFakeTMAP(t, `{}`, fixture(t, "plan_gangnam_hongdae.json"))

	got, err := c.Plan(context.Background(),
		Place{Name: "강남역", Lat: 37.4979, Lon: 127.0276},
		Place{Name: "홍대입구역", Lat: 37.5563, Lon: 126.9228},
	)
	if err != nil {
		t.Fatalf("Plan() unexpected error: %v", err)
	}

	if got.TotalTime != 42 { // 2528s + 30, floor-divided by 60 → 42min
		t.Errorf("TotalTime = %d, want 42", got.TotalTime)
	}
	if got.TransferCount != 0 {
		t.Errorf("TransferCount = %d, want 0", got.TransferCount)
	}
	if got.Fare != 1650 {
		t.Errorf("Fare = %d, want 1650", got.Fare)
	}

	wantSteps := []string{
		"강남까지 도보 1분",               // 출발지 → 강남, 59s → 1min
		"강남에서 수도권2호선 승차 → 홍대입구 하차", // route wins over mode
		"홍대입구역까지 도보 2분",            // 도착지 replaced by user-facing name, 113s → 2min
	}
	if len(got.Steps) != len(wantSteps) {
		t.Fatalf("len(Steps) = %d, want %d (%+v)", len(got.Steps), len(wantSteps), got.Steps)
	}
	for i, want := range wantSteps {
		if got.Steps[i].Description != want {
			t.Errorf("Steps[%d] = %q, want %q", i, got.Steps[i].Description, want)
		}
	}
}

func TestPlanEmptyItinerariesIsErrNoRoute(t *testing.T) {
	c := newFakeTMAP(t, `{}`, fixture(t, "plan_empty.json"))

	_, err := c.Plan(context.Background(), Place{Name: "a"}, Place{Name: "b"})
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want ErrNoRoute", err)
	}
}

func TestPlanSendsCoordinatesAsRequestBody(t *testing.T) {
	var captured planRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("test-app-key")
	c.BaseURL = srv.URL

	_, err := c.Plan(context.Background(),
		Place{Name: "강남역", Lat: 37.4979, Lon: 127.0276},
		Place{Name: "홍대입구역", Lat: 37.5563, Lon: 126.9228},
	)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want ErrNoRoute (empty itineraries)", err)
	}

	want := planRequest{StartX: "127.0276", StartY: "37.4979", EndX: "126.9228", EndY: "37.5563", Count: 1, Lang: 0, Format: "json"}
	if captured != want {
		t.Errorf("request body = %+v, want %+v", captured, want)
	}
}

func TestRequestsCarryAppKeyHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("appKey")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"searchPoiInfo":{"pois":{"poi":[]}}}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("my-secret-key")
	c.BaseURL = srv.URL

	_, _ = c.Geocode(context.Background(), "강남역")
	if gotKey != "my-secret-key" {
		t.Errorf("appKey header = %q, want %q", gotKey, "my-secret-key")
	}
}

func TestUpstreamFailuresAreErrUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
	}{
		{
			name: "HTTP 500",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		},
		{
			name: "malformed JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{not json`))
			},
		},
		{
			name: "timeout",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(200 * time.Millisecond)
				_, _ = w.Write([]byte(`{}`))
			},
			timeout: 50 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			t.Cleanup(srv.Close)
			c := NewClient("test-app-key")
			c.BaseURL = srv.URL
			if tt.timeout > 0 {
				c.HTTPClient = &http.Client{Timeout: tt.timeout}
			}

			_, err := c.Geocode(context.Background(), "강남역")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Geocode() error = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestQuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("test-app-key")
	c.BaseURL = srv.URL

	_, err := c.Geocode(context.Background(), "강남역")
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("Geocode() error = %v, want ErrQuotaExceeded", err)
	}
}

func TestFindRoute(t *testing.T) {
	t.Run("success composes geocode×2 and plan", func(t *testing.T) {
		c := newFakeTMAP(t, fixture(t, "pois_gangnam.json"), fixture(t, "plan_gangnam_hongdae.json"))

		got, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
		if err != nil {
			t.Fatalf("FindRoute() unexpected error: %v", err)
		}
		if got.TotalTime != 42 || got.TransferCount != 0 {
			t.Errorf("RouteResult = %+v, want 42min/0transfer", got)
		}
	})

	t.Run("from-side miss is ErrPointNotFound{Side: from}", func(t *testing.T) {
		c := newFakeTMAP(t, fixture(t, "pois_empty.json"), `{}`)

		_, err := c.FindRoute(context.Background(), "없는곳", "강남역")
		var pnf *core.ErrPointNotFound
		if !errors.As(err, &pnf) {
			t.Fatalf("FindRoute() error = %v, want *core.ErrPointNotFound", err)
		}
		if pnf.Side != "from" || pnf.Name != "없는곳" {
			t.Errorf("ErrPointNotFound = %+v, want Side=from Name=없는곳", pnf)
		}
	})

	t.Run("to-side miss is ErrPointNotFound{Side: to}", func(t *testing.T) {
		// First geocode call (from) succeeds, second (to) returns empty.
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path != "/tmap/pois" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			calls++
			if calls == 1 {
				_, _ = w.Write([]byte(`{"searchPoiInfo":{"pois":{"poi":[{"name":"강남역","dataKind":"1","frontLat":"37.5","frontLon":"127.0"}]}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"searchPoiInfo":{"pois":{"poi":[]}}}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient("test-app-key")
		c.BaseURL = srv.URL
		_, err := c.FindRoute(context.Background(), "강남역", "없는곳")
		var pnf *core.ErrPointNotFound
		if !errors.As(err, &pnf) {
			t.Fatalf("FindRoute() error = %v, want *core.ErrPointNotFound", err)
		}
		if pnf.Side != "to" || pnf.Name != "없는곳" {
			t.Errorf("ErrPointNotFound = %+v, want Side=to Name=없는곳", pnf)
		}
	})

	t.Run("upstream failure passes through as ErrUnavailable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(srv.Close)

		c := NewClient("test-app-key")
		c.BaseURL = srv.URL
		_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("FindRoute() error = %v, want ErrUnavailable", err)
		}
	})
}

func TestModeKorean(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"SUBWAY", "지하철"},
		{"SUBWAYBUS", "지하철"},
		{"BUS", "버스"},
		{"EXPRESSBUS", "버스"},
		{"TRAIN", "기차"},
		{"FERRY", "여객선"},
		{"AIRPLANE", "항공"},
		{"GONDOLA", "GONDOLA"}, // unknown passes through
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			if got := modeKorean(tt.mode); got != tt.want {
				t.Errorf("modeKorean(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}
