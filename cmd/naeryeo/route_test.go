package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
)

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
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, findRoute)

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
	code := runRoute([]string{"--from", "강남역", "--to", "강남역"}, &stdout, &stderr, load, findRoute)

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
	code := runRoute([]string{"--from", "강남역"}, &stdout, &stderr, load, findRoute)

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
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, findRoute)

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
	code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, findRoute)

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
		name    string
		findErr error
		want    string
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
		{
			name:    "upstream unavailable",
			findErr: core.ErrUpstreamUnavailable,
			want:    "오류가 발생했습니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findRoute := func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error) {
				return core.RouteResult{}, tt.findErr
			}

			var stdout, stderr bytes.Buffer
			code := runRoute([]string{"--from", "강남역", "--to", "홍대입구역"}, &stdout, &stderr, load, findRoute)

			if code == 0 {
				t.Fatal("runRoute() code = 0, want non-zero")
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.want)
			}
		})
	}
}
