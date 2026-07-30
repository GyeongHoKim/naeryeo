package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// loadGeoPresent / loadGeoAbsent are the two geocoder-key states the route
// entry point branches on for the FR-007 hint.
func loadGeoPresent() (string, error) { return "kakao-key", nil }
func loadGeoAbsent() (string, error)  { return "", config.ErrNotConfigured }

func TestRunRoute_Success(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{
			TotalTime:     42,
			TransferCount: 1,
			Fare:          1500,
			Steps: []core.RouteStep{
				{Description: "강남역에서 2호선 승차 → 신도림역에서 하차"},
				{Description: "신도림역에서 경의중앙선 승차 → 홍대입구역에서 하차"},
			},
		}, nil
	}

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)

	if code != 0 {
		t.Fatalf("runRoute() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"42", "환승 1회", "1,500원", "강남역에서 2호선 승차"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want substring %q", out, want)
		}
	}
}

func TestRouteErrorMessage(t *testing.T) {
	const hint = "setup --geocoder"

	t.Run("point not found without geocoder appends the FR-007 hint", func(t *testing.T) {
		msg := routeErrorMessage(&core.ErrPointNotFound{Side: "from", Name: "아이디스 타워"}, false)
		if !strings.Contains(msg, "출발지") {
			t.Errorf("msg = %q, want it to name the from side", msg)
		}
		if !strings.Contains(msg, hint) {
			t.Errorf("msg = %q, want the FR-007 hint %q", msg, hint)
		}
	})

	t.Run("point not found with geocoder omits the hint", func(t *testing.T) {
		msg := routeErrorMessage(&core.ErrPointNotFound{Side: "to", Name: "아이디스 타워"}, true)
		if strings.Contains(msg, hint) {
			t.Errorf("msg = %q, should not append the hint when a geocoder key exists", msg)
		}
	})

	t.Run("geocoder auth failure has its own message regardless of configured flag", func(t *testing.T) {
		for _, configured := range []bool{true, false} {
			msg := routeErrorMessage(core.ErrGeocoderAuthFailed, configured)
			if !strings.Contains(msg, "장소 검색 키가 유효하지 않습니다") {
				t.Errorf("msg = %q, want the geocoder auth-failed message", msg)
			}
		}
	})

	t.Run("geocoder unavailable has a distinct message", func(t *testing.T) {
		msg := routeErrorMessage(core.ErrGeocoderUnavailable, true)
		if !strings.Contains(msg, "장소 검색 서비스에 연결할 수 없습니다") {
			t.Errorf("msg = %q, want the geocoder-unavailable message", msg)
		}
	})

	// A geocoder rejection message is shared with the MCP tool result, whose
	// audience is an AI caller. It must be actionable and must never leak HTTP
	// status/code/body — those belong in the logs.
	noLeak := func(t *testing.T, msg string) {
		t.Helper()
		for _, leak := range []string{"400", "HTTP", "code", "코드", "--debug"} {
			if strings.Contains(msg, leak) {
				t.Errorf("msg = %q, must not leak %q to the (possibly AI) caller", msg, leak)
			}
		}
	}

	t.Run("rate-limit rejection tells the caller to retry shortly", func(t *testing.T) {
		msg := routeErrorMessage(&core.ErrGeocoderRejected{Status: 400, Code: "-10", Message: "call frequency exceeded"}, true)
		if !strings.Contains(msg, "다시 시도") || !strings.Contains(msg, "일시적") {
			t.Errorf("msg = %q, want a retry-shortly message", msg)
		}
		if strings.Contains(msg, "call frequency exceeded") {
			t.Errorf("msg = %q, must not echo the provider body", msg)
		}
		noLeak(t, msg)
	})

	t.Run("bad-request rejection tells the caller to reformulate the location", func(t *testing.T) {
		msg := routeErrorMessage(&core.ErrGeocoderRejected{Status: 400, Code: "-2", Message: "query is required"}, true)
		if !strings.Contains(msg, "위치를 인식하지 못했습니다") || !strings.Contains(msg, "구체적") {
			t.Errorf("msg = %q, want a reformulate-location message", msg)
		}
		if strings.Contains(msg, "query is required") {
			t.Errorf("msg = %q, must not echo the provider body", msg)
		}
		noLeak(t, msg)
	})

	t.Run("geocoder forbidden points at app service settings, not re-registration", func(t *testing.T) {
		msg := routeErrorMessage(core.ErrGeocoderForbidden, true)
		if !strings.Contains(msg, "권한이 거부") || !strings.Contains(msg, "활성화") {
			t.Errorf("msg = %q, want it to point at enabling the service", msg)
		}
		if strings.Contains(msg, "다시 등록") {
			t.Errorf("msg = %q, should NOT tell the user to re-register the key for a 403", msg)
		}
	})
}

func TestRunRoute_FR007Hint(t *testing.T) {
	findNotFound := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{}, &core.ErrPointNotFound{Side: "from", Name: "아이디스 타워"}
	}
	load := func() (string, error) { return "test-key", nil }

	t.Run("no geocoder key -> hint shown", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		runRoute([]string{"--from", "아이디스 타워", "--to", "수지구청"}, &stdout, &stderr, load, loadGeoAbsent, findNotFound)
		if !strings.Contains(stderr.String(), "setup --geocoder") {
			t.Errorf("stderr = %q, want the FR-007 hint", stderr.String())
		}
	})

	t.Run("geocoder key present -> hint omitted", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		runRoute([]string{"--from", "아이디스 타워", "--to", "수지구청"}, &stdout, &stderr, load, loadGeoPresent, findNotFound)
		if strings.Contains(stderr.String(), "setup --geocoder") {
			t.Errorf("stderr = %q, should not show the hint when a geocoder key exists", stderr.String())
		}
	})
}

func TestRunRoute_DebugFlagAppendsRawErrorChain(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{}, &core.ErrGeocoderRejected{Status: 400, Code: "-10", Message: "limit"}
	}

	t.Run("--debug appends the raw error chain", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		runRoute([]string{"--from", "아이디스 타워", "--to", "강남역", "--debug"}, &stdout, &stderr, load, loadGeoPresent, findRoute)
		if !strings.Contains(stderr.String(), "[debug]") {
			t.Errorf("stderr = %q, want the [debug] raw error chain", stderr.String())
		}
	})

	t.Run("without --debug the raw chain is omitted", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		runRoute([]string{"--from", "아이디스 타워", "--to", "강남역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)
		if strings.Contains(stderr.String(), "[debug]") {
			t.Errorf("stderr = %q, should not include the raw chain without --debug", stderr.String())
		}
	})
}

// leakKey is an ODsay API key containing characters url.QueryEscape rewrites,
// so a redaction that only matched the raw key string would not satisfy the
// assertions below.
const leakKey = "super/secret+odsay=key"

// deadUpstreamFindRoute routes through a real core.Client aimed at a closed
// port. The resulting error is the genuine transport failure that used to
// carry the ODsay API key (GYE-293) rather than a hand-built stand-in, so
// these tests keep verifying the real thing if core's error wrapping changes.
func deadUpstreamFindRoute(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
	client := core.NewClient(apiKey)
	client.BaseURL = "http://127.0.0.1:1"
	return client.FindRoute(ctx, from, to)
}

func assertNoAPIKey(t *testing.T, s, apiKey string) {
	t.Helper()
	for _, form := range []string{apiKey, url.QueryEscape(apiKey), url.PathEscape(apiKey)} {
		if strings.Contains(s, form) {
			t.Errorf("output leaked the API key (as %q):\n%s", form, s)
		}
	}
}

// TestRunRoute_TransportFailureNeverLeaksTheAPIKey covers the CLI half of
// GYE-293's exit criteria. --debug matters most here: it prints the raw error
// chain, so it is the path a presentation-layer-only fix would have missed.
func TestRunRoute_TransportFailureNeverLeaksTheAPIKey(t *testing.T) {
	load := func() (string, error) { return leakKey, nil }

	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "default output", args: []string{"--from", "강남역", "--to", "홍대입구역"}},
		{name: "--debug output", args: []string{"--from", "강남역", "--to", "홍대입구역", "--debug"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runRoute(tt.args, &stdout, &stderr, load, loadGeoAbsent, deadUpstreamFindRoute)

			if code != 1 {
				t.Fatalf("runRoute() code = %d, want 1; stderr = %q", code, stderr.String())
			}
			assertNoAPIKey(t, stdout.String()+stderr.String(), leakKey)
		})
	}
}

func TestWithThousandsSeparator(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{1500, "1,500"},
		{1234567, "1,234,567"},
		{-2000, "-2,000"},
	}
	for _, tt := range tests {
		if got := withThousandsSeparator(tt.in); got != tt.want {
			t.Errorf("withThousandsSeparator(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRunRoute_NoTravelNeeded(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{NoTravelNeeded: true}, nil
	}

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "강남역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)

	if code != 0 {
		t.Fatalf("runRoute() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "이동이 필요 없") {
		t.Errorf("stdout = %q, want a no-travel-needed message", stdout.String())
	}
}

func TestRunRoute_MissingFlags(t *testing.T) {
	called := false
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		called = true
		return core.RouteResult{}, nil
	}
	load := func() (string, error) { return "test-key", nil }

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)

	if code == 0 {
		t.Fatal("runRoute() code = 0, want non-zero")
	}
	if called {
		t.Fatal("findRoute should not be called when --to is missing")
	}
}

func TestRunRoute_APIKeyNotConfigured(t *testing.T) {
	load := func() (string, error) { return "", config.ErrNotConfigured }
	called := false
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		called = true
		return core.RouteResult{}, nil
	}

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)

	if code == 0 {
		t.Fatal("runRoute() code = 0, want non-zero")
	}
	if called {
		t.Fatal("findRoute should not be called when the API key is not configured")
	}
	if !strings.Contains(stderr.String(), "naeryeo setup") {
		t.Errorf("stderr = %q, want a hint to run naeryeo setup", stderr.String())
	}
}

func TestRunRoute_AuthFailedIsDistinctFromMissingKey(t *testing.T) {
	load := func() (string, error) { return "stored-but-invalid-key", nil }
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		return core.RouteResult{}, core.ErrAuthFailed
	}

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)

	if code == 0 {
		t.Fatal("runRoute() code = 0, want non-zero")
	}
	if strings.Contains(stderr.String(), "설정되지 않았습니다") {
		t.Errorf("stderr = %q, should not reuse the 'not configured' message for an invalid key", stderr.String())
	}
	if !strings.Contains(stderr.String(), "유효하지 않습니다") {
		t.Errorf("stderr = %q, want a distinct 'invalid key' message", stderr.String())
	}
}

func TestRunRoute_ErrorMessages(t *testing.T) {
	load := func() (string, error) { return "test-key", nil }

	tests := []struct {
		name     string
		findErr  error
		want     string
		dontWant string
	}{
		{
			name:    "point not found",
			findErr: &core.ErrPointNotFound{Side: "from", Name: "존재하지않는가짜지명"},
			want:    "출발지",
		},
		{
			name:    "no route",
			findErr: core.ErrNoRoute,
			want:    "경로가 없습니다",
		},
		// The upstream cases pass WRAPPED errors on purpose. Before spec 005
		// these fell through routeErrorMessage's default branch, which
		// formatted the error with %v and printed the provider's own text —
		// including things like "internal db timeout at shard 7". Handing a
		// bare sentinel to this test would not have caught that, because a bare
		// sentinel has nothing to leak. dontWant is what actually verifies it.
		{
			name:     "upstream unavailable",
			findErr:  fmt.Errorf("%w: dial tcp 127.0.0.1:1: connect: connection refused", core.ErrUpstreamUnavailable),
			want:     "경로 검색 서비스에 연결할 수 없습니다",
			dontWant: "connection refused",
		},
		{
			name:     "upstream rejected",
			findErr:  fmt.Errorf("searching route: %w", &core.ErrUpstreamRejected{Code: "500", Message: "Server Error: internal db timeout at shard 7 (trace 0xdeadbeef)"}),
			want:     "경로 검색 서비스가 요청을 처리하지 못했습니다",
			dontWant: "shard 7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
				return core.RouteResult{}, tt.findErr
			}

			var stdout, stderr bytes.Buffer
			code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)

			if code == 0 {
				t.Fatal("runRoute() code = 0, want non-zero")
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.want)
			}
			if tt.dontWant != "" && strings.Contains(stderr.String(), tt.dontWant) {
				t.Errorf("stderr leaked the wrapped cause %q:\n%s", tt.dontWant, stderr.String())
			}
		})
	}
}

// TestRunRoute_CredentialStoreErrorIsClassified covers the other path that used
// to interpolate a raw error into user-facing output — route.go's
// "API 키 조회 실패: %v" for a keychain that could not be read at all.
func TestRunRoute_CredentialStoreErrorIsClassified(t *testing.T) {
	const storeErr = "keyring: exec gpg: no such file or directory"
	load := func() (string, error) { return "", errors.New(storeErr) }
	called := false
	findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
		called = true
		return core.RouteResult{}, nil
	}

	var stdout, stderr bytes.Buffer
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, loadGeoPresent, findRoute)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if called {
		t.Error("findRoute was called despite the credential store failing")
	}
	if strings.Contains(stderr.String(), storeErr) {
		t.Errorf("stderr leaked the keychain error:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "불러오지 못했습니다") {
		t.Errorf("stderr = %q, want the classified credential-store message", stderr.String())
	}
}
