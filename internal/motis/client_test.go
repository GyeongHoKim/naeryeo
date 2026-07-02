package motis

import (
	"context"
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

// newFakeMOTIS starts an httptest server that answers /api/v1/geocode with
// geocodeBody and /api/v3/plan with planBody, and returns a Client wired
// to it.
func newFakeMOTIS(t *testing.T, geocodeBody, planBody string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/geocode":
			_, _ = w.Write([]byte(geocodeBody))
		case "/api/v3/plan":
			_, _ = w.Write([]byte(planBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL)
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
			name:     "prefers first STOP even with earlier PLACE",
			body:     `[{"type":"PLACE","name":"강남약국","lat":37.1,"lon":127.1},{"type":"STOP","name":"강남역","lat":37.49993,"lon":127.02632}]`,
			wantName: "강남역",
			wantLat:  37.49993,
		},
		{
			name:     "falls back to first match when no STOP",
			body:     `[{"type":"PLACE","name":"어딘가","lat":35.0,"lon":129.0},{"type":"ADDRESS","name":"어딘가길","lat":36.0,"lon":128.0}]`,
			wantName: "어딘가",
			wantLat:  35.0,
		},
		{
			name:     "real measured fixture with exponent-notation numbers",
			body:     fixture(t, "geocode_gangnam.json"),
			wantName: "강남역",
			wantLat:  37.499930000000006,
		},
		{
			name:    "empty array is ErrPlaceNotFound",
			body:    `[]`,
			wantErr: ErrPlaceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeMOTIS(t, tt.body, `{}`)
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

func TestGeocodeRetriesWithoutStationSuffix(t *testing.T) {
	t.Run("역-suffixed miss retries stripped and succeeds", func(t *testing.T) {
		var queries []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			text := r.URL.Query().Get("text")
			queries = append(queries, text)
			if text == "홍대입구" {
				_, _ = w.Write([]byte(`[{"type":"STOP","name":"홍대입구","lat":37.557,"lon":126.924}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`)) // "홍대입구역" is not in the feed
		}))
		t.Cleanup(srv.Close)

		c := NewClient(srv.URL)
		got, err := c.Geocode(context.Background(), "홍대입구역")
		if err != nil {
			t.Fatalf("Geocode() unexpected error: %v", err)
		}
		if got.Name != "홍대입구" {
			t.Errorf("Name = %q, want 홍대입구", got.Name)
		}
		want := []string{"홍대입구역", "홍대입구"}
		if len(queries) != 2 || queries[0] != want[0] || queries[1] != want[1] {
			t.Errorf("queries = %v, want %v", queries, want)
		}
	})

	t.Run("both misses stay ErrPlaceNotFound", func(t *testing.T) {
		c := newFakeMOTIS(t, `[]`, `{}`)
		_, err := c.Geocode(context.Background(), "없는곳역")
		if !errors.Is(err, ErrPlaceNotFound) {
			t.Fatalf("Geocode() error = %v, want ErrPlaceNotFound", err)
		}
	})

	t.Run("bare 역 does not retry with empty text", func(t *testing.T) {
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			calls++
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(srv.URL)
		_, err := c.Geocode(context.Background(), "역")
		if !errors.Is(err, ErrPlaceNotFound) {
			t.Fatalf("Geocode() error = %v, want ErrPlaceNotFound", err)
		}
		if calls != 1 {
			t.Errorf("geocode calls = %d, want 1 (no empty-text retry)", calls)
		}
	})
}

func TestPlanMapsItineraryOntoRouteResult(t *testing.T) {
	c := newFakeMOTIS(t, `[]`, fixture(t, "plan_gangnam_hongdae.json"))

	got, err := c.Plan(context.Background(),
		Place{Name: "강남역", Lat: 37.4979, Lon: 127.0276},
		Place{Name: "홍대입구역", Lat: 37.5568, Lon: 126.9237},
	)
	if err != nil {
		t.Fatalf("Plan() unexpected error: %v", err)
	}

	if got.TotalTime != 39 { // 2340s → 39min
		t.Errorf("TotalTime = %d, want 39", got.TotalTime)
	}
	if got.TransferCount != 1 {
		t.Errorf("TransferCount = %d, want 1", got.TransferCount)
	}
	if got.Fare != 0 {
		t.Errorf("Fare = %d, want 0 (KTDB has no fares)", got.Fare)
	}

	wantSteps := []string{
		"신논현까지 도보 13분",         // START → 신논현, 780s → 13min
		"신논현에서 9호선 승차 → 당산 하차", // routeShortName wins over mode
		"당산에서 2호선 승차 → 홍대입구 하차",
		"홍대입구역까지 도보 2분", // END replaced by user-facing name
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
	c := newFakeMOTIS(t, `[]`, fixture(t, "plan_empty.json"))

	_, err := c.Plan(context.Background(), Place{Name: "a"}, Place{Name: "b"})
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want ErrNoRoute", err)
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
				_, _ = w.Write([]byte(`[]`))
			},
			timeout: 50 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			t.Cleanup(srv.Close)
			c := NewClient(srv.URL)
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

func TestFindRoute(t *testing.T) {
	t.Run("success composes geocode×2 and plan", func(t *testing.T) {
		c := newFakeMOTIS(t, fixture(t, "geocode_gangnam.json"), fixture(t, "plan_gangnam_hongdae.json"))

		got, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
		if err != nil {
			t.Fatalf("FindRoute() unexpected error: %v", err)
		}
		if got.TotalTime != 39 || got.TransferCount != 1 {
			t.Errorf("RouteResult = %+v, want 39min/1transfer", got)
		}
	})

	t.Run("from-side miss is ErrPointNotFound{Side: from}", func(t *testing.T) {
		c := newFakeMOTIS(t, `[]`, `{}`)

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
			if r.URL.Path != "/api/v1/geocode" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			calls++
			if calls == 1 {
				_, _ = w.Write([]byte(`[{"type":"STOP","name":"강남역","lat":37.5,"lon":127.0}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient(srv.URL)
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

		c := NewClient(srv.URL)
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
		{"TRAM", "지하철"},
		{"BUS", "버스"},
		{"RAIL", "기차"},
		{"HIGHSPEED_RAIL", "기차"},
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
