package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// TestClassifyRouteError_ProseMatchesPre005Wording pins every message that
// existed before this feature. routeErrorMessage now delegates to
// classifyRouteError, so a wording change here would silently alter the CLI's
// default output — these cases are what keep spec 005 FR-007 honest.
func TestClassifyRouteError_ProseMatchesPre005Wording(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		geocoderConfigend bool
		wantCode          errorCode
		wantProse         string
	}{
		{
			name:      "api key missing",
			err:       core.ErrAPIKeyMissing,
			wantCode:  codeAPIKeyMissing,
			wantProse: "API 키가 설정되지 않았습니다. naeryeo setup을 먼저 실행하세요",
		},
		{
			name:      "auth failed",
			err:       core.ErrAuthFailed,
			wantCode:  codeAuthFailed,
			wantProse: "저장된 API 키가 유효하지 않습니다. naeryeo setup으로 다시 등록하세요",
		},
		{
			name:      "geocoder auth failed",
			err:       core.ErrGeocoderAuthFailed,
			wantCode:  codeGeocoderAuthFailed,
			wantProse: "장소 검색 키가 유효하지 않습니다. naeryeo setup --geocoder로 다시 등록하세요",
		},
		{
			name:      "geocoder forbidden",
			err:       core.ErrGeocoderForbidden,
			wantCode:  codeGeocoderForbidden,
			wantProse: "장소 검색 서비스(Kakao) 권한이 거부되었습니다. Kakao 개발자 콘솔에서 해당 앱의 카카오맵(로컬) 서비스를 활성화했는지, 키의 도메인·IP 제한 설정을 확인하세요",
		},
		{
			name:      "no route",
			err:       core.ErrNoRoute,
			wantCode:  codeNoRoute,
			wantProse: "두 지점 사이에 대중교통 경로가 없습니다",
		},
		{
			name:      "geocoder unavailable",
			err:       core.ErrGeocoderUnavailable,
			wantCode:  codeGeocoderUnavailable,
			wantProse: "장소 검색 서비스에 연결할 수 없습니다. 잠시 후 다시 시도하세요",
		},
		{
			name:      "geocoder rejected as rate limit",
			err:       &core.ErrGeocoderRejected{Status: http.StatusTooManyRequests},
			wantCode:  codeGeocoderRateLimited,
			wantProse: "장소 검색 요청이 일시적으로 제한되었습니다. 잠시 후 다시 시도하세요",
		},
		{
			name:      "geocoder rejected as bad input",
			err:       &core.ErrGeocoderRejected{Status: http.StatusBadRequest},
			wantCode:  codeGeocoderRejected,
			wantProse: "입력하신 위치를 인식하지 못했습니다. 더 구체적인 주소(도로명·지번)나 인근 지하철역·정류장 이름으로 다시 시도하세요",
		},
		{
			name:              "point not found with geocoder configured has no hint",
			err:               &core.ErrPointNotFound{Side: "from", Name: "아이디스 타워"},
			geocoderConfigend: true,
			wantCode:          codePointNotFound,
			wantProse:         `출발지을(를) 찾을 수 없습니다: "아이디스 타워"`,
		},
		{
			name:      "point not found without geocoder appends the hint",
			err:       &core.ErrPointNotFound{Side: "to", Name: "수지구청"},
			wantCode:  codePointNotFound,
			wantProse: "도착지을(를) 찾을 수 없습니다: \"수지구청\"\n건물명·주소로 찾으려면 naeryeo setup --geocoder로 장소 검색 키를 설정하세요",
		},
		{
			name:      "both sides unresolved",
			err:       &core.ErrPointNotFound{Side: "both", Name: "???"},
			wantCode:  codePointNotFound,
			wantProse: "출발지와 도착지 모두을(를) 찾을 수 없습니다: \"???\"\n건물명·주소로 찾으려면 naeryeo setup --geocoder로 장소 검색 키를 설정하세요",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRouteError(tt.err, tt.geocoderConfigend)
			if got.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tt.wantCode)
			}
			if got.Prose() != tt.wantProse {
				t.Errorf("Prose() =\n%q\nwant\n%q", got.Prose(), tt.wantProse)
			}
			// routeErrorMessage is the pre-005 entry point; it must keep
			// producing exactly what Prose does.
			if msg := routeErrorMessage(tt.err, tt.geocoderConfigend); msg != tt.wantProse {
				t.Errorf("routeErrorMessage() =\n%q\nwant\n%q", msg, tt.wantProse)
			}
		})
	}
}

// TestClassifyRouteError_APIKeyMissingCoversConfigSentinel documents that a
// missing stored key reaches the same code whether core reports it or the
// config package does.
func TestClassifyRouteError_APIKeyMissingCoversConfigSentinel(t *testing.T) {
	got := classifyRouteError(config.ErrNotConfigured, false)
	if got.Code != codeAPIKeyMissing {
		t.Errorf("Code = %q, want %q", got.Code, codeAPIKeyMissing)
	}
}

// TestFailureProse covers the two shapes Prose has to render.
func TestFailureProse(t *testing.T) {
	tests := []struct {
		name string
		f    failure
		want string
	}{
		{
			name: "no hint returns the message alone",
			f:    failure{Message: "이유"},
			want: "이유",
		},
		{
			name: "hint goes on its own line",
			f:    failure{Message: "이유", Hint: "조치"},
			want: "이유\n조치",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.Prose(); got != tt.want {
				t.Errorf("Prose() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFailureToRouteError checks the projection carries every field and that
// the optional ones stay empty (so omitempty drops them from the document).
func TestFailureToRouteError(t *testing.T) {
	t.Run("all fields", func(t *testing.T) {
		f := failure{
			Code:    codePointNotFound,
			Message: "이유",
			Hint:    "조치",
			Side:    "from",
			Name:    "아이디스 타워",
		}
		got := f.toRouteError()
		if got == nil {
			t.Fatal("toRouteError() = nil, want non-nil")
		}
		if got.Code != "point_not_found" {
			t.Errorf("Code = %q, want %q", got.Code, "point_not_found")
		}
		if got.Message != "이유" || got.Hint != "조치" {
			t.Errorf("Message/Hint = %q/%q, want %q/%q", got.Message, got.Hint, "이유", "조치")
		}
		if got.Side != "from" || got.Name != "아이디스 타워" {
			t.Errorf("Side/Name = %q/%q, want %q/%q", got.Side, got.Name, "from", "아이디스 타워")
		}
	})

	t.Run("optional fields stay empty", func(t *testing.T) {
		got := failure{Code: codeNoRoute, Message: "이유"}.toRouteError()
		if got.Hint != "" || got.Side != "" || got.Name != "" {
			t.Errorf("optional fields = %q/%q/%q, want all empty", got.Hint, got.Side, got.Name)
		}
	})
}

// TestClassifyRouteError_NeverLeaksTheWrappedCause is the runtime half of
// FR-005: whatever the cause carries, none of it may reach a user-facing
// field. The exhaustiveness test below is the other half.
func TestClassifyRouteError_NeverLeaksTheWrappedCause(t *testing.T) {
	const secret = "internal db timeout at shard 7 (trace 0xdeadbeef)"

	for _, tt := range []struct {
		name string
		err  error
	}{
		{
			name: "wrapped upstream rejection",
			err:  fmt.Errorf("wrapped: %w", &core.ErrUpstreamRejected{Code: "500", Message: secret}),
		},
		{
			name: "wrapped upstream unavailability",
			err:  fmt.Errorf("%w: %s", core.ErrUpstreamUnavailable, secret),
		},
		{
			name: "wrapped geocoder rejection",
			err:  fmt.Errorf("%w", &core.ErrGeocoderRejected{Status: 400, Code: "-9", Message: secret}),
		},
		{
			name: "an error matching nothing at all",
			err:  errors.New(secret),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRouteError(tt.err, false)
			for field, v := range map[string]string{
				"Message": got.Message,
				"Hint":    got.Hint,
				"Prose()": got.Prose(),
			} {
				if strings.Contains(v, secret) || strings.Contains(v, "shard 7") {
					t.Errorf("%s leaked the wrapped cause: %q", field, v)
				}
			}
			if got.Message == "" {
				t.Error("Message is empty; every failure must carry a relayable reason")
			}
		})
	}
}
