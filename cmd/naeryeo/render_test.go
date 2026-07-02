package main

import (
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

func TestRenderRouteMarkdown(t *testing.T) {
	route := core.RouteResult{
		TotalTime:     39,
		TransferCount: 1,
		Steps: []core.RouteStep{
			{Description: "신논현까지 도보 13분"},
			{Description: "신논현에서 9호선 승차 → 당산 하차"},
			{Description: "당산에서 2호선 승차 → 홍대입구 하차"},
		},
	}

	tests := []struct {
		name         string
		from, to     string
		result       core.RouteResult
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:   "fare omitted when zero (KTDB has no fares)",
			from:   "강남역",
			to:     "홍대입구역",
			result: route,
			wantContains: []string{
				"**강남역 → 홍대입구역** · 약 39분 · 환승 1회",
				"1. 신논현까지 도보 13분",
				"2. 신논현에서 9호선 승차 → 당산 하차",
				"3. 당산에서 2호선 승차 → 홍대입구 하차",
				"_데이터: KTDB·OSM 기반 시간표 — 실시간 지연 미반영_",
			},
			wantAbsent: []string{"요금"},
		},
		{
			name: "fare included when known, with thousands separator",
			from: "강남역",
			to:   "홍대입구역",
			result: core.RouteResult{
				TotalTime:     42,
				TransferCount: 1,
				Fare:          1500,
				Steps:         []core.RouteStep{{Description: "이동"}},
			},
			wantContains: []string{"· 요금 1,500원"},
		},
		{
			name:   "no travel needed",
			from:   "강남역",
			to:     "강남역",
			result: core.RouteResult{NoTravelNeeded: true},
			wantContains: []string{
				"**강남역 → 강남역**",
				"이동이 필요하지 않습니다",
			},
			wantAbsent: []string{"환승", "1."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderRouteMarkdown(tt.from, tt.to, tt.result)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("markdown missing %q:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("markdown should not contain %q:\n%s", absent, got)
				}
			}
		})
	}
}

func TestFormatKRW(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{950, "950"},
		{1500, "1,500"},
		{12345, "12,345"},
		{1234567, "1,234,567"},
	}
	for _, tt := range tests {
		if got := formatKRW(tt.in); got != tt.want {
			t.Errorf("formatKRW(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
