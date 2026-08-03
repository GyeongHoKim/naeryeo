package motis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// stubGeocoder stands in for the optional Kakao fallback.
type stubGeocoder struct {
	coord  core.Coordinate
	err    error
	calls  int
	querie []string
}

func (g *stubGeocoder) Resolve(_ context.Context, query string) (core.Coordinate, error) {
	g.calls++
	g.querie = append(g.querie, query)
	return g.coord, g.err
}

// motisStub serves the two endpoints the client uses. A nil handler for either
// one makes that endpoint return an empty result, which is the shape MOTIS
// uses for "nothing matched".
type motisStub struct {
	geocode func(text string) (int, string)
	plan    func(query map[string]string) (int, string)

	geocodeCalls int
	planCalls    int
	lastPlan     map[string]string
}

func (s *motisStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := map[string]string{}
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				q[k] = v[0]
			}
		}
		switch r.URL.Path {
		case geocodePath:
			s.geocodeCalls++
			status, body := http.StatusOK, `[]`
			if s.geocode != nil {
				status, body = s.geocode(q["text"])
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		case planPath:
			s.planCalls++
			s.lastPlan = q
			status, body := http.StatusOK, `{"itineraries":[],"direct":[]}`
			if s.plan != nil {
				status, body = s.plan(q)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func geocodeHit(lat, lon float64) string {
	b, _ := json.Marshal([]map[string]any{{"lat": lat, "lon": lon, "name": "stub"}})
	return string(b)
}

const planTwoLegs = `{
  "itineraries": [{
    "duration": 2520,
    "transfers": 1,
    "legs": [
      {"mode":"SUBWAY","routeShortName":"2호선","from":{"name":"강남역"},"to":{"name":"신도림역"},"duration":1500},
      {"mode":"SUBWAY","routeShortName":"경의중앙선","from":{"name":"신도림역"},"to":{"name":"홍대입구역"},"duration":1020}
    ]
  }],
  "direct": []
}`

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c := NewClient(baseURL)
	return c
}

func TestFindRoute_HappyPath(t *testing.T) {
	stub := &motisStub{
		geocode: func(text string) (int, string) {
			if text == "강남역" {
				return http.StatusOK, geocodeHit(37.4979, 127.0276)
			}
			return http.StatusOK, geocodeHit(37.5572, 126.9245)
		},
		plan: func(map[string]string) (int, string) { return http.StatusOK, planTwoLegs },
	}
	srv := stub.server(t)
	c := newTestClient(t, srv.URL)

	got, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
	if err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}

	if got.TotalTime != 42 {
		t.Errorf("TotalTime = %d, want 42 (2520s rounded to minutes)", got.TotalTime)
	}
	if got.TransferCount != 1 {
		t.Errorf("TransferCount = %d, want 1", got.TransferCount)
	}
	if got.FareKnown {
		t.Error("FareKnown = true, want false — MOTIS supplies no fare in v1")
	}
	if len(got.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(got.Steps))
	}
	for i, want := range []string{"강남역", "홍대입구역"} {
		idx := i
		if !strings.Contains(got.Steps[idx].Description, want) {
			t.Errorf("Steps[%d] = %q, want it to mention %q", idx, got.Steps[idx].Description, want)
		}
	}

	if stub.geocodeCalls != 2 {
		t.Errorf("geocode calls = %d, want 2 (one per endpoint)", stub.geocodeCalls)
	}
	if stub.planCalls != 1 {
		t.Errorf("plan calls = %d, want 1", stub.planCalls)
	}
	if want := "37.497900,127.027600"; stub.lastPlan["fromPlace"] != want {
		t.Errorf("fromPlace = %q, want %q", stub.lastPlan["fromPlace"], want)
	}
	if stub.lastPlan["numItineraries"] != "1" {
		t.Errorf("numItineraries = %q, want \"1\"", stub.lastPlan["numItineraries"])
	}
}

// TestFindRoute_ResolvesNamesWithoutAnyExternalService is the claim that makes
// self-hosting worth doing: with no geocoder configured, a station name still
// resolves, because MOTIS indexes its own stops.
func TestFindRoute_ResolvesNamesWithoutAnyExternalService(t *testing.T) {
	stub := &motisStub{
		geocode: func(string) (int, string) { return http.StatusOK, geocodeHit(37.5, 127.0) },
		plan:    func(map[string]string) (int, string) { return http.StatusOK, planTwoLegs },
	}
	srv := stub.server(t)
	c := newTestClient(t, srv.URL)
	if c.Geocoder != nil {
		t.Fatal("test precondition: no fallback geocoder must be configured")
	}

	if _, err := c.FindRoute(context.Background(), "강남역", "홍대입구역"); err != nil {
		t.Fatalf("FindRoute() error = %v, want nil with no geocoder configured", err)
	}
}

func TestFindRoute_GeocoderFallback(t *testing.T) {
	t.Run("falls back when MOTIS knows no such place", func(t *testing.T) {
		stub := &motisStub{
			geocode: func(text string) (int, string) {
				if text == "강남역" {
					return http.StatusOK, geocodeHit(37.4979, 127.0276)
				}
				return http.StatusOK, `[]` // MOTIS does not index this building
			},
			plan: func(map[string]string) (int, string) { return http.StatusOK, planTwoLegs },
		}
		srv := stub.server(t)
		geo := &stubGeocoder{coord: core.Coordinate{X: 127.1, Y: 37.6}}
		c := newTestClient(t, srv.URL)
		c.Geocoder = geo

		if _, err := c.FindRoute(context.Background(), "강남역", "아이디스 타워"); err != nil {
			t.Fatalf("FindRoute() error = %v, want nil", err)
		}
		if geo.calls != 1 {
			t.Fatalf("geocoder calls = %d, want 1", geo.calls)
		}
		if geo.querie[0] != "아이디스 타워" {
			t.Errorf("geocoder query = %q, want %q", geo.querie[0], "아이디스 타워")
		}
	})

	t.Run("no geocoder configured yields point not found", func(t *testing.T) {
		stub := &motisStub{}
		srv := stub.server(t)
		c := newTestClient(t, srv.URL)

		_, err := c.FindRoute(context.Background(), "없는곳", "홍대입구역")
		var notFound *core.ErrPointNotFound
		if !errors.As(err, &notFound) {
			t.Fatalf("error = %v, want *core.ErrPointNotFound", err)
		}
		if notFound.Side != "from" || notFound.Name != "없는곳" {
			t.Errorf("got side=%q name=%q, want from/없는곳", notFound.Side, notFound.Name)
		}
	})

	t.Run("geocoder not-found also yields point not found", func(t *testing.T) {
		stub := &motisStub{}
		srv := stub.server(t)
		c := newTestClient(t, srv.URL)
		c.Geocoder = &stubGeocoder{err: core.ErrGeocoderNotFound}

		_, err := c.FindRoute(context.Background(), "강남역", "없는곳")
		var notFound *core.ErrPointNotFound
		if !errors.As(err, &notFound) {
			t.Fatalf("error = %v, want *core.ErrPointNotFound", err)
		}
		if notFound.Side != "from" {
			t.Errorf("Side = %q, want from — the first endpoint fails first", notFound.Side)
		}
	})

	t.Run("geocoder auth failure propagates unchanged", func(t *testing.T) {
		stub := &motisStub{}
		srv := stub.server(t)
		c := newTestClient(t, srv.URL)
		c.Geocoder = &stubGeocoder{err: core.ErrGeocoderAuthFailed}

		_, err := c.FindRoute(context.Background(), "강남역", "홍대입구역")
		if !errors.Is(err, core.ErrGeocoderAuthFailed) {
			t.Fatalf("error = %v, want core.ErrGeocoderAuthFailed", err)
		}
	})
}

func TestFindRoute_RouteOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		planBody string
		wantErr  error
		wantTime int
	}{
		{
			name:     "no itineraries and no direct is no route",
			planBody: `{"itineraries":[],"direct":[]}`,
			wantErr:  core.ErrNoRoute,
		},
		{
			name:     "missing arrays entirely is no route",
			planBody: `{}`,
			wantErr:  core.ErrNoRoute,
		},
		{
			name: "direct-only walk is a usable result",
			planBody: `{"itineraries":[],"direct":[{"duration":600,"transfers":0,
				"legs":[{"mode":"WALK","from":{"name":"강남역"},"to":{"name":"역삼역"},"duration":600}]}]}`,
			wantTime: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &motisStub{
				geocode:  func(string) (int, string) { return http.StatusOK, geocodeHit(37.5, 127.0) },
				plan:     func(map[string]string) (int, string) { return http.StatusOK, tt.planBody },
				lastPlan: nil,
			}
			srv := stub.server(t)
			c := newTestClient(t, srv.URL)

			got, err := c.FindRoute(context.Background(), "강남역", "역삼역")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if got.TotalTime != tt.wantTime {
				t.Errorf("TotalTime = %d, want %d", got.TotalTime, tt.wantTime)
			}
			if got.FareKnown {
				t.Error("FareKnown = true, want false")
			}
		})
	}
}

func TestFindRoute_TransportFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantIs     error
		wantStatus int
	}{
		{name: "server error is retryable", status: http.StatusInternalServerError, body: `{}`, wantIs: core.ErrMotisUnavailable},
		{name: "bad gateway is retryable", status: http.StatusBadGateway, body: `{}`, wantIs: core.ErrMotisUnavailable},
		{name: "bad request is a rejection", status: http.StatusBadRequest, body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "not found is a rejection", status: http.StatusNotFound, body: `{}`, wantStatus: http.StatusNotFound},
		{name: "undecodable body is a rejection", status: http.StatusOK, body: `<html>not json</html>`, wantStatus: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &motisStub{
				geocode: func(string) (int, string) { return http.StatusOK, geocodeHit(37.5, 127.0) },
				plan:    func(map[string]string) (int, string) { return tt.status, tt.body },
			}
			srv := stub.server(t)
			c := newTestClient(t, srv.URL)

			_, err := c.FindRoute(context.Background(), "강남역", "역삼역")
			if err == nil {
				t.Fatal("error = nil, want a failure")
			}
			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("error = %v, want %v", err, tt.wantIs)
				}
				return
			}
			var rejected *core.ErrMotisRejected
			if !errors.As(err, &rejected) {
				t.Fatalf("error = %v, want *core.ErrMotisRejected", err)
			}
			if rejected.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rejected.Status, tt.wantStatus)
			}
		})
	}
}

func TestFindRoute_UnreachableEngine(t *testing.T) {
	// A port nothing listens on: the transport fails before any status.
	c := newTestClient(t, "http://127.0.0.1:1")

	_, err := c.FindRoute(context.Background(), "강남역", "역삼역")
	if !errors.Is(err, core.ErrMotisUnavailable) {
		t.Fatalf("error = %v, want core.ErrMotisUnavailable", err)
	}
}

// TestFindRoute_ErrorsNeverCarryTheEndpoint is the FR-018 guard at the source.
// Wrapping a *url.Error would embed the operator's host and port in the error
// chain, and from there it reaches --debug output, logs, and any caller that
// formats the error — including the MCP path, whose text lands in an AI's
// conversation history.
func TestFindRoute_ErrorsNeverCarryTheEndpoint(t *testing.T) {
	const host = "127.0.0.1:1"
	c := newTestClient(t, "http://"+host)

	_, err := c.FindRoute(context.Background(), "강남역", "역삼역")
	if err == nil {
		t.Fatal("error = nil, want a failure")
	}
	if strings.Contains(err.Error(), host) {
		t.Fatalf("error text leaks the engine endpoint: %q", err.Error())
	}
	for _, needle := range []string{"127.0.0.1", ":1", "http://"} {
		if strings.Contains(err.Error(), needle) {
			t.Errorf("error text contains %q, which reveals the operator's network: %q", needle, err.Error())
		}
	}
}

func TestFindRoute_GeocodeFailuresAreClassified(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantIs     error
		wantStatus int
	}{
		{name: "geocode 5xx is retryable", status: http.StatusServiceUnavailable, body: `[]`, wantIs: core.ErrMotisUnavailable},
		{name: "geocode 4xx is a rejection", status: http.StatusBadRequest, body: `[]`, wantStatus: http.StatusBadRequest},
		{name: "geocode garbage is a rejection", status: http.StatusOK, body: `nope`, wantStatus: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &motisStub{geocode: func(string) (int, string) { return tt.status, tt.body }}
			srv := stub.server(t)
			c := newTestClient(t, srv.URL)

			_, err := c.FindRoute(context.Background(), "강남역", "역삼역")
			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("error = %v, want %v", err, tt.wantIs)
				}
				return
			}
			var rejected *core.ErrMotisRejected
			if !errors.As(err, &rejected) {
				t.Fatalf("error = %v, want *core.ErrMotisRejected", err)
			}
			if rejected.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rejected.Status, tt.wantStatus)
			}
		})
	}
}

func TestDescribeLeg(t *testing.T) {
	tests := []struct {
		name string
		leg  leg
		want string
	}{
		{
			name: "subway names the line",
			leg:  leg{Mode: "SUBWAY", RouteShortName: "2호선", From: place{Name: "강남역"}, To: place{Name: "역삼역"}},
			want: "강남역에서 2호선 승차 → 역삼역에서 하차",
		},
		{
			name: "bus is labelled as a bus",
			leg:  leg{Mode: "BUS", RouteShortName: "160", From: place{Name: "강남역"}, To: place{Name: "역삼역"}},
			want: "강남역에서 160 버스 승차 → 역삼역에서 하차",
		},
		{
			name: "walk reports duration",
			leg:  leg{Mode: "WALK", From: place{Name: "강남역"}, To: place{Name: "역삼역"}, Duration: 600},
			want: "강남역에서 역삼역까지 도보 이동 (10분)",
		},
		{
			name: "missing route name falls back to headsign",
			leg:  leg{Mode: "BUS", Headsign: "강남 방면", From: place{Name: "A"}, To: place{Name: "B"}},
			want: "A에서 강남 방면 버스 승차 → B에서 하차",
		},
		{
			name: "missing headsign falls back to agency",
			leg:  leg{Mode: "RAIL", AgencyName: "코레일", From: place{Name: "A"}, To: place{Name: "B"}},
			want: "A에서 코레일 승차 → B에서 하차",
		},
		{
			name: "nothing to name yields a generic description",
			leg:  leg{Mode: "RAIL", From: place{Name: "A"}, To: place{Name: "B"}, Duration: 300},
			want: "A에서 B까지 이동 (5분)",
		},
		// A real engine labels the ends of a journey "START" and "END" rather
		// than echoing the query, so these two cases are what an actual walk
		// leg looks like — not a synthetic edge case.
		{
			name: "the origin is named rather than left as START",
			leg:  leg{Mode: "WALK", From: place{Name: "START"}, To: place{Name: "강남"}, Duration: 480},
			want: "출발지에서 강남까지 도보 이동 (8분)",
		},
		{
			name: "the destination is named rather than left as END",
			leg:  leg{Mode: "WALK", From: place{Name: "홍대입구"}, To: place{Name: "END"}, Duration: 60},
			want: "홍대입구에서 도착지까지 도보 이동 (1분)",
		},
		{
			name: "a transit leg between the two ends is named too",
			leg:  leg{Mode: "SUBWAY", RouteShortName: "9호선", From: place{Name: "START"}, To: place{Name: "END"}},
			want: "출발지에서 9호선 승차 → 도착지에서 하차",
		},
		{
			name: "a stop that merely contains START keeps its own name",
			leg:  leg{Mode: "WALK", From: place{Name: "STARTUP빌딩"}, To: place{Name: "END역"}, Duration: 120},
			want: "STARTUP빌딩에서 END역까지 도보 이동 (2분)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeLeg(tt.leg); got != tt.want {
				t.Errorf("describeLeg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	if got := NewClient("http://localhost:8080/").baseURL(); got != "http://localhost:8080" {
		t.Errorf("baseURL() = %q, want %q", got, "http://localhost:8080")
	}
}
