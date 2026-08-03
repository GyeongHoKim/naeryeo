package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// withSettingsFile points config's settings storage at a temp directory and
// writes content into it. An empty content leaves the file absent, which is
// the state a user has before ever running setup — and, since spec 006, the
// state an upgrading user is in regardless of what their keychain holds.
func withSettingsFile(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	config.SetConfigDirForTest(t, dir)
	if content == "" {
		return
	}
	appDir := filepath.Join(dir, "naeryeo")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

// motisEcho is a stub MOTIS that resolves any name and returns one itinerary.
func motisEcho(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/geocode":
			_, _ = w.Write([]byte(`[{"lat":37.5,"lon":127.0,"name":"stub"}]`))
		case "/api/v6/plan":
			_, _ = w.Write([]byte(`{"itineraries":[{"duration":1800,"transfers":0,
				"legs":[{"mode":"SUBWAY","routeShortName":"2호선","from":{"name":"강남역"},"to":{"name":"홍대입구역"}}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- provider_not_configured -------------------------------------------------

// TestProviderNotConfigured_BothEntryPointsAgree is the FR-002/FR-014 pair:
// with nothing configured, the CLI and the MCP tool must produce the same code
// and the same words, because they run the same resolver.
func TestProviderNotConfigured_BothEntryPointsAgree(t *testing.T) {
	withSettingsFile(t, "")
	resolve := newProviderResolver(discardLogger)

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역", "--json"},
		&stdout, &stderr, resolve, geoAbsent)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	cli := decodeEnvelope(t, stdout.String())
	if cli.Error == nil {
		t.Fatal("CLI produced no error object")
	}
	if cli.Error.Code != string(codeProviderNotConfigured) {
		t.Fatalf("CLI code = %q, want %q", cli.Error.Code, codeProviderNotConfigured)
	}

	server := buildMCPServer("test", discardLogger, resolve, geoAbsent)
	session := connectTestClient(t, server)
	res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	if !res.IsError {
		t.Fatal("MCP IsError = false, want true")
	}
	mcpOut := decodeRouteToolOutput(t, res)
	if mcpOut.Error == nil {
		t.Fatal("MCP produced no structured error")
	}

	if mcpOut.Error.Code != cli.Error.Code {
		t.Errorf("codes differ: CLI %q, MCP %q", cli.Error.Code, mcpOut.Error.Code)
	}
	if mcpOut.Error.Message != cli.Error.Message {
		t.Errorf("messages differ:\n CLI %q\n MCP %q", cli.Error.Message, mcpOut.Error.Message)
	}
	if mcpOut.Error.Hint != cli.Error.Hint {
		t.Errorf("hints differ:\n CLI %q\n MCP %q", cli.Error.Hint, mcpOut.Error.Hint)
	}
	if cli.Error.Docs == "" {
		t.Error("Docs is empty; the user needs somewhere to go")
	}
}

// TestProviderNotConfigured_StoredKeyDoesNotImplyAProvider is the migration
// decision made executable: an upgrading user with an ODsay key in the
// keychain is still "not configured", because inferring the provider from a
// leftover credential would be a permanent exception serving a one-time event.
func TestProviderNotConfigured_StoredKeyDoesNotImplyAProvider(t *testing.T) {
	withSettingsFile(t, "")
	config.SetCredentialsForTest(t, map[config.Credential]string{
		config.ODsayAPIKey: "an-existing-key",
	})

	resolve := newProviderResolver(discardLogger)
	find, preflight := resolve()

	if find != nil {
		t.Fatal("a finder was produced from a stored key alone")
	}
	if preflight == nil {
		t.Fatal("no preflight failure; the run would proceed with no provider")
	}
	if preflight.Code != codeProviderNotConfigured {
		t.Errorf("Code = %q, want %q", preflight.Code, codeProviderNotConfigured)
	}
	if !strings.Contains(preflight.Hint, "naeryeo setup") {
		t.Errorf("Hint = %q, want it to name the setup command", preflight.Hint)
	}
}

// TestReconfiguringODsayReusesTheStoredKey is the other half of the migration
// contract: re-running setup must not require the user to find their key
// again, so the transition costs exactly one command.
func TestReconfiguringODsayReusesTheStoredKey(t *testing.T) {
	withSettingsFile(t, `{"routing_provider":"odsay"}`)
	config.SetCredentialsForTest(t, map[config.Credential]string{
		config.ODsayAPIKey: "an-existing-key",
	})

	find, preflight := newProviderResolver(discardLogger)()
	if preflight != nil {
		t.Fatalf("preflight failure = %+v, want nil — the stored key should be reused", preflight)
	}
	if find == nil {
		t.Fatal("no finder was produced")
	}
}

// --- provider selection ------------------------------------------------------

// TestProviderSelection_RouteAndMCPUseTheSameEngine covers SC-005 directly: the
// stub counts requests, so if either entry point reached a different provider
// the counts would not match.
func TestProviderSelection_RouteAndMCPUseTheSameEngine(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/api/v1/geocode":
			_, _ = w.Write([]byte(`[{"lat":37.5,"lon":127.0}]`))
		default:
			_, _ = w.Write([]byte(`{"itineraries":[{"duration":600,"transfers":0,"legs":[]}]}`))
		}
	}))
	defer srv.Close()

	withSettingsFile(t, `{"routing_provider":"motis","motis_url":"`+srv.URL+`"}`)
	resolve := newProviderResolver(discardLogger)

	var stdout, stderr bytes.Buffer
	if code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"},
		&stdout, &stderr, resolve, geoAbsent); code != 0 {
		t.Fatalf("CLI code = %d, want 0; stderr = %q", code, stderr.String())
	}
	afterCLI := hits
	if afterCLI == 0 {
		t.Fatal("the CLI never reached the configured engine")
	}

	server := buildMCPServer("test", discardLogger, resolve, geoAbsent)
	session := connectTestClient(t, server)
	res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	if res.IsError {
		t.Fatalf("MCP call failed: %s", resultText(res))
	}
	if hits <= afterCLI {
		t.Fatal("the MCP path did not reach the same engine the CLI used")
	}
}

// TestProviderSelection_NoFallbackToTheOtherProvider is FR-008 stated
// negatively. Cost control is the reason someone self-hosts; silently
// answering from a paid API when their engine is down would defeat it.
func TestProviderSelection_NoFallbackToTheOtherProvider(t *testing.T) {
	var odsayHits int
	odsay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		odsayHits++
		_, _ = w.Write([]byte(`{"result":{"station":[]}}`))
	}))
	defer odsay.Close()

	// A MOTIS URL that nothing is listening on, plus a perfectly good stored
	// ODsay key. The run must fail rather than quietly switch.
	withSettingsFile(t, `{"routing_provider":"motis","motis_url":"http://127.0.0.1:1"}`)
	config.SetCredentialsForTest(t, map[config.Credential]string{
		config.ODsayAPIKey: "a-valid-looking-key",
	})

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역", "--json"},
		&stdout, &stderr, newProviderResolver(discardLogger), geoAbsent)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	got := decodeEnvelope(t, stdout.String())
	if got.Error == nil || got.Error.Code != string(codeMotisUnavailable) {
		t.Fatalf("error = %+v, want code %q", got.Error, codeMotisUnavailable)
	}
	if odsayHits != 0 {
		t.Errorf("the ODsay endpoint was contacted %d times; there must be no fallback", odsayHits)
	}
}

// --- self-hosting failures ---------------------------------------------------

func TestSelfHostingFailures_CodesAndDocs(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		useDead  bool
		wantCode errorCode
	}{
		{
			name:     "engine down",
			useDead:  true,
			wantCode: codeMotisUnavailable,
		},
		{
			name: "engine refuses the request",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			wantCode: codeMotisRejected,
		},
		{
			name: "engine returns something that is not MOTIS",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<html>hello</html>`))
			},
			wantCode: codeMotisRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "http://127.0.0.1:1"
			if !tt.useDead {
				srv := httptest.NewServer(tt.handler)
				defer srv.Close()
				url = srv.URL
			}
			withSettingsFile(t, `{"routing_provider":"motis","motis_url":"`+url+`"}`)

			var stdout, stderr bytes.Buffer
			code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역", "--json"},
				&stdout, &stderr, newProviderResolver(discardLogger), geoAbsent)

			if code != 1 {
				t.Fatalf("code = %d, want 1", code)
			}
			got := decodeEnvelope(t, stdout.String())
			if got.Error == nil {
				t.Fatal("no error object")
			}
			if got.Error.Code != string(tt.wantCode) {
				t.Errorf("Code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if got.Error.Docs == "" {
				t.Error("Docs is empty; a self-hosting failure must say where to look")
			}
		})
	}
}

// TestSelfHostingFailures_NeverRevealTheOperatorsNetwork is SC-006. The MCP
// path matters most: its output is written into a model's context and travels
// onward from there.
func TestSelfHostingFailures_NeverRevealTheOperatorsNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	withSettingsFile(t, `{"routing_provider":"motis","motis_url":"`+srv.URL+`"}`)
	resolve := newProviderResolver(discardLogger)

	var stdout, stderr bytes.Buffer
	runRoute([]string{"--from", "강남역", "--to", "홍대입구역", "--json", "--debug"},
		&stdout, &stderr, resolve, geoAbsent)

	if strings.Contains(stdout.String(), host) {
		t.Errorf("the JSON document leaked the engine address %q:\n%s", host, stdout.String())
	}

	server := buildMCPServer("test", discardLogger, resolve, geoAbsent)
	session := connectTestClient(t, server)
	res := callFindTransitRoute(t, session, "강남역", "홍대입구역")
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(raw), host) {
		t.Errorf("the MCP result leaked the engine address %q:\n%s", host, raw)
	}
}

// TestSelfHostingFailures_ProseCarriesTheDocsLink covers the human half of
// FR-017 — the link has to be readable without --json.
func TestSelfHostingFailures_ProseCarriesTheDocsLink(t *testing.T) {
	withSettingsFile(t, `{"routing_provider":"motis","motis_url":"http://127.0.0.1:1"}`)

	var stdout, stderr bytes.Buffer
	runRoute([]string{"--from", "강남역", "--to", "홍대입구역"},
		&stdout, &stderr, newProviderResolver(discardLogger), geoAbsent)

	if !strings.Contains(stderr.String(), selfHostingDocsURL) {
		t.Errorf("stderr = %q, want the self-hosting docs link", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("prose rendered %d lines, want 3 (reason / action / docs):\n%s", len(lines), stderr.String())
	}
}

// --- provider x geocoder combinations ---------------------------------------

// TestProviderGeocoderCombinations walks all four valid configurations. The
// first row is the claim this whole feature rests on: with MOTIS and no
// geocoder, a station name still resolves and nothing external is contacted.
func TestProviderGeocoderCombinations(t *testing.T) {
	tests := []struct {
		name     string
		provider config.RoutingProvider
		geocoder config.GeocoderChoice
	}{
		{"motis without place search", config.ProviderMotis, config.GeocoderNone},
		{"motis with place search", config.ProviderMotis, config.GeocoderKakao},
		{"odsay without place search", config.ProviderODsay, config.GeocoderNone},
		{"odsay with place search", config.ProviderODsay, config.GeocoderKakao},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var external int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/api/v1/geocode"):
					_, _ = w.Write([]byte(`[{"lat":37.5,"lon":127.0}]`))
				case strings.HasPrefix(r.URL.Path, "/api/v6/plan"):
					_, _ = w.Write([]byte(`{"itineraries":[{"duration":600,"transfers":0,"legs":[]}]}`))
				default: // ODsay shapes
					external++
					if strings.Contains(r.URL.Path, "searchStation") {
						_, _ = w.Write([]byte(`{"result":{"station":[{"stationName":"강남역","x":127.0,"y":37.5}]}}`))
						return
					}
					_, _ = w.Write([]byte(`{"result":{"path":[{"info":{"totalTime":30,"payment":1500,
						"busTransitCount":0,"subwayTransitCount":1},"subPath":[]}]}}`))
				}
			}))
			defer upstream.Close()

			settings := `{"routing_provider":"` + string(tt.provider) + `","geocoder":"` + string(tt.geocoder) + `"`
			if tt.provider == config.ProviderMotis {
				settings += `,"motis_url":"` + upstream.URL + `"`
			}
			settings += `}`
			withSettingsFile(t, settings)

			creds := map[config.Credential]string{}
			if tt.provider == config.ProviderODsay {
				creds[config.ODsayAPIKey] = "k"
			}
			if tt.geocoder == config.GeocoderKakao {
				creds[config.GeocoderAPIKey] = "k"
			}
			config.SetCredentialsForTest(t, creds)

			find, preflight := newProviderResolver(discardLogger)()
			if preflight != nil {
				t.Fatalf("preflight failure = %+v, want a usable finder", preflight)
			}

			// The same invocation shape for every combination: the caller does
			// not change how it asks based on which engine answers (FR-012).
			if tt.provider == config.ProviderODsay {
				// The ODsay client's base URL is not injectable from here, so
				// this row asserts only that a finder was produced.
				return
			}
			if _, err := find(context.Background(), "강남역", "홍대입구역"); err != nil {
				t.Fatalf("FindRoute() error = %v, want nil", err)
			}
			if tt.geocoder == config.GeocoderNone && external != 0 {
				t.Errorf("an external service was contacted %d times with no geocoder configured", external)
			}
		})
	}
}

// TestMotisFareIsAbsentNotZero closes the loop from the engine through to the
// document a caller reads.
func TestMotisFareIsAbsentNotZero(t *testing.T) {
	srv := motisEcho(t)
	withSettingsFile(t, `{"routing_provider":"motis","motis_url":"`+srv.URL+`"}`)

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역", "--json"},
		&stdout, &stderr, newProviderResolver(discardLogger), geoAbsent)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "fareWon") {
		t.Errorf("fareWon is present for a provider with no fare data:\n%s", stdout.String())
	}

	var prose bytes.Buffer
	stderr.Reset()
	runRoute([]string{"--from", "강남역", "--to", "홍대입구역"},
		&prose, &stderr, newProviderResolver(discardLogger), geoAbsent)
	if !strings.Contains(prose.String(), "요금 정보 없음") {
		t.Errorf("prose = %q, want an explicit 'no fare information'", prose.String())
	}
	if strings.Contains(prose.String(), "요금: 0원") {
		t.Errorf("prose reported a free trip for missing data:\n%s", prose.String())
	}
}

func TestGeocoderConfigured(t *testing.T) {
	t.Run("kakao selected with a stored key", func(t *testing.T) {
		withSettingsFile(t, `{"routing_provider":"odsay","geocoder":"kakao"}`)
		config.SetCredentialsForTest(t, map[config.Credential]string{config.GeocoderAPIKey: "k"})
		if !geocoderConfigured() {
			t.Error("geocoderConfigured() = false, want true")
		}
	})

	t.Run("kakao selected but no key stored", func(t *testing.T) {
		withSettingsFile(t, `{"routing_provider":"odsay","geocoder":"kakao"}`)
		config.SetCredentialsForTest(t, map[config.Credential]string{})
		if geocoderConfigured() {
			t.Error("geocoderConfigured() = true, want false when the key is missing")
		}
	})

	t.Run("geocoder not selected", func(t *testing.T) {
		withSettingsFile(t, `{"routing_provider":"odsay","geocoder":"none"}`)
		config.SetCredentialsForTest(t, map[config.Credential]string{config.GeocoderAPIKey: "k"})
		if geocoderConfigured() {
			t.Error("geocoderConfigured() = true, want false when place search is off")
		}
	})
}

func TestResolverReportsCredentialStoreFailures(t *testing.T) {
	withSettingsFile(t, `{"routing_provider":"odsay"}`)
	config.SetCredentialErrorForTest(t, errors.New("keyring: exec gpg: no such file"))

	find, preflight := newProviderResolver(discardLogger)()
	if find != nil {
		t.Fatal("a finder was produced despite an unreadable keychain")
	}
	if preflight == nil || preflight.Code != codeCredentialStoreError {
		t.Fatalf("preflight = %+v, want code %q", preflight, codeCredentialStoreError)
	}
	if strings.Contains(preflight.Message+preflight.Hint, "gpg") {
		t.Errorf("the raw keychain error leaked into the failure: %+v", preflight)
	}
}

var _ = core.ErrNoRoute // keep the core import meaningful if assertions move
