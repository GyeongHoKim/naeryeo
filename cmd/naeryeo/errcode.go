package main

import (
	"errors"
	"fmt"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// errorCode identifies a route-search failure by the action its caller should
// take next, not by which internal error produced it. Two failures that share
// a Go type but call for opposite responses get different codes (a geocoder
// call-frequency limit is worth retrying, a malformed location is not), and
// failures with different internal causes but the same response share one.
//
// The string values are the external contract — they appear in the CLI's
// --json output and in the MCP tool's structuredContent, and AI callers branch
// on them. Changing or removing a value is a breaking change; adding one is
// not. The human-readable Message that travels alongside a code is NOT part of
// the contract: callers must never match on its text.
//
// specs/005-structured-output-contract/contracts/error-codes.md is the
// canonical table; classifyRouteError below is its implementation, and
// TestErrorCodeExhaustive_EveryCoreErrorHasACode fails if a new core error
// lands without a code.
type errorCode string

const (
	// codeAPIKeyMissing: no routing key is stored. Tell the user to run setup.
	codeAPIKeyMissing errorCode = "api_key_missing"
	// codeAuthFailed: the stored routing key was rejected. Re-register it.
	codeAuthFailed errorCode = "auth_failed"
	// codeGeocoderAuthFailed: the place-search key was rejected as
	// unauthenticated. Re-registering it can fix this.
	codeGeocoderAuthFailed errorCode = "geocoder_auth_failed"
	// codeGeocoderForbidden: the place-search key is valid but the request was
	// denied by the provider's app settings. Re-registering will NOT help —
	// kept distinct from codeGeocoderAuthFailed for exactly that reason.
	codeGeocoderForbidden errorCode = "geocoder_forbidden"
	// codeGeocoderRateLimited: a transient call-frequency limit. Retrying the
	// same request shortly is the correct response.
	codeGeocoderRateLimited errorCode = "geocoder_rate_limited"
	// codeGeocoderRejected: the place-search request itself was refused. The
	// input must be reformulated; retrying it unchanged is pointless. This is
	// the counterpart to codeGeocoderRateLimited — same Go type, opposite
	// action.
	codeGeocoderRejected errorCode = "geocoder_rejected"
	// codePointNotFound: one or both endpoints could not be resolved. Side and
	// Name say which, so the caller can re-ask about just that endpoint.
	codePointNotFound errorCode = "point_not_found"
	// codeNoRoute: both endpoints resolved but no transit route connects them.
	// Report it; do not retry.
	codeNoRoute errorCode = "no_route"
	// codeGeocoderUnavailable: the place-search service could not be reached.
	codeGeocoderUnavailable errorCode = "geocoder_unavailable"
	// codeUpstreamUnavailable: the routing service could not be reached.
	// Retrying later may work.
	codeUpstreamUnavailable errorCode = "upstream_unavailable"
	// codeUpstreamRejected: the routing service refused the request for a
	// reason that maps to nothing else. Its own error text — which can carry
	// server internals like shard ids and trace handles — is never surfaced.
	codeUpstreamRejected errorCode = "upstream_rejected"
	// codeCredentialStoreError: the OS keychain could not be read at all.
	// Distinct from codeAPIKeyMissing, which means the store was readable and
	// simply held nothing.
	codeCredentialStoreError errorCode = "credential_store_error"
	// codeInvalidArguments: the command was called incorrectly (missing or
	// unparseable flags). Fix the invocation and retry. Unlike the codes above
	// this one never comes from classifyRouteError — the CLI raises it before a
	// search is ever attempted.
	codeInvalidArguments errorCode = "invalid_arguments"
	// codeInternalError is the safe default for an error that matches nothing
	// above. It must be unreachable in practice — the exhaustiveness test
	// exists to keep it that way — but it is what guarantees that no unclassified
	// error can ever reach the user with its original text attached.
	codeInternalError errorCode = "internal_error"
)

// failure is the single source every user-facing rendering of a route-search
// error derives from: the CLI's prose output, the CLI's --json document, and
// the MCP tool result. Deriving all three from one value is what keeps the two
// entry points from drifting apart (spec 005 FR-016).
//
// Neither Message nor Hint ever contains text from an upstream provider or the
// credential store. Original error chains are reachable only via --debug, which
// writes to stderr.
type failure struct {
	// Code is the stable identifier callers branch on.
	Code errorCode
	// Message is the human-readable Korean reason, safe to relay verbatim.
	Message string
	// Hint is the action the user should take, when one applies. Empty
	// otherwise. Kept separate from Message so an AI caller can tell "why it
	// failed" apart from "what to do about it".
	Hint string
	// Side and Name are set only for codePointNotFound: which endpoint failed
	// ("from"/"to"/"both") and the input that could not be resolved.
	Side string
	Name string
}

// Prose renders the failure the way the CLI's default (non-JSON) output and
// the MCP tool's text content present it: the reason, then the hint on its own
// line when there is one. This reproduces the pre-005 wording exactly, which is
// what lets the existing prose tests pass unchanged (spec 005 FR-007).
func (f failure) Prose() string {
	if f.Hint == "" {
		return f.Message
	}
	return f.Message + "\n" + f.Hint
}

// classifyRouteError turns a core/config error into the single failure value
// every entry point renders from. It is the only place that decides what a
// failure means, which is what keeps the CLI and the MCP server from answering
// the same failure differently (spec 005 FR-016).
//
// The switch is ordered by specificity, not by package: *ErrGeocoderRejected is
// examined before the plain geocoder sentinels because it splits into two codes
// on its own, and the default deliberately discards err rather than formatting
// it — an unclassified cause must never reach the caller with its text intact
// (FR-005). TestErrorCodeExhaustive_EveryCoreErrorHasACode makes reaching that
// default a build-gate failure rather than a silent leak.
func classifyRouteError(err error, geocoderConfigured bool) failure {
	var pointErr *core.ErrPointNotFound
	var rejErr *core.ErrGeocoderRejected
	var upstreamErr *core.ErrUpstreamRejected

	switch {
	case errors.Is(err, core.ErrAPIKeyMissing), errors.Is(err, config.ErrNotConfigured):
		return failure{
			Code:    codeAPIKeyMissing,
			Message: "API 키가 설정되지 않았습니다. naeryeo setup을 먼저 실행하세요",
		}
	case errors.Is(err, core.ErrAuthFailed):
		return failure{
			Code:    codeAuthFailed,
			Message: "저장된 API 키가 유효하지 않습니다. naeryeo setup으로 다시 등록하세요",
		}
	case errors.Is(err, core.ErrGeocoderAuthFailed):
		return failure{
			Code:    codeGeocoderAuthFailed,
			Message: "장소 검색 키가 유효하지 않습니다. naeryeo setup --geocoder로 다시 등록하세요",
		}
	case errors.Is(err, core.ErrGeocoderForbidden):
		return failure{
			Code:    codeGeocoderForbidden,
			Message: "장소 검색 서비스(Kakao) 권한이 거부되었습니다. Kakao 개발자 콘솔에서 해당 앱의 카카오맵(로컬) 서비스를 활성화했는지, 키의 도메인·IP 제한 설정을 확인하세요",
		}
	case errors.As(err, &pointErr):
		f := failure{
			Code:    codePointNotFound,
			Message: fmt.Sprintf("%s을(를) 찾을 수 없습니다: %q", sideLabel(pointErr.Side), pointErr.Name),
			Side:    pointErr.Side,
			Name:    pointErr.Name,
		}
		// The hint only helps when setting up a geocoder would actually change
		// the outcome (spec 004 FR-007).
		if !geocoderConfigured {
			f.Hint = "건물명·주소로 찾으려면 naeryeo setup --geocoder로 장소 검색 키를 설정하세요"
		}
		return f
	case errors.Is(err, core.ErrNoRoute):
		return failure{
			Code:    codeNoRoute,
			Message: "두 지점 사이에 대중교통 경로가 없습니다",
		}
	case errors.As(err, &rejErr):
		// One Go type, two opposite responses. A call-frequency limit is not
		// the caller's fault, so tell it to retry; anything else means the
		// location could not be resolved, so tell it to reformulate. The HTTP
		// status, provider code, and body stay out of both messages.
		if rejErr.RateLimited() {
			return failure{
				Code:    codeGeocoderRateLimited,
				Message: "장소 검색 요청이 일시적으로 제한되었습니다. 잠시 후 다시 시도하세요",
			}
		}
		return failure{
			Code:    codeGeocoderRejected,
			Message: "입력하신 위치를 인식하지 못했습니다. 더 구체적인 주소(도로명·지번)나 인근 지하철역·정류장 이름으로 다시 시도하세요",
		}
	case errors.Is(err, core.ErrGeocoderUnavailable):
		return failure{
			Code:    codeGeocoderUnavailable,
			Message: "장소 검색 서비스에 연결할 수 없습니다. 잠시 후 다시 시도하세요",
		}
	case errors.Is(err, core.ErrUpstreamUnavailable):
		return failure{
			Code:    codeUpstreamUnavailable,
			Message: "경로 검색 서비스에 연결할 수 없습니다. 잠시 후 다시 시도하세요",
		}
	case errors.As(err, &upstreamErr):
		// The routing provider's own Code and Message stay out of this: they
		// carry server internals (shard ids, trace handles) that mean nothing
		// to the user and should not reach an AI caller either. --debug still
		// has them, on stderr.
		return failure{
			Code:    codeUpstreamRejected,
			Message: "경로 검색 서비스가 요청을 처리하지 못했습니다. 출발지와 도착지를 다시 확인해 주세요",
		}
	default:
		// err is intentionally dropped here. Diagnosing it is --debug's job,
		// and --debug writes to stderr where it cannot corrupt a --json
		// document or reach an MCP client.
		return failure{
			Code:    codeInternalError,
			Message: "경로 검색 중 오류가 발생했습니다",
		}
	}
}

// credentialStoreFailure describes a keychain that could not be read.
//
// The underlying error is dropped on purpose: keychain errors are OS-level
// strings that mean nothing to the user, and both entry points used to
// interpolate them straight into their output (route.go's "API 키 조회 실패: %v"
// and mcp.go's wrapped equivalent). Sharing one constructor is what makes the
// CLI and the MCP server answer this the same way (spec 005 FR-018).
func credentialStoreFailure() failure {
	return failure{
		Code:    codeCredentialStoreError,
		Message: "저장된 API 키를 불러오지 못했습니다",
		Hint:    "OS 키체인 접근 권한을 확인한 뒤 naeryeo setup으로 키를 다시 등록하세요",
	}
}

// toRouteError projects the failure onto its serialized form. Hint, Side, and
// Name are omitempty in RouteError, so the empty values here simply drop out of
// the document rather than appearing as empty strings.
func (f failure) toRouteError() *RouteError {
	return &RouteError{
		Code:    string(f.Code),
		Message: f.Message,
		Hint:    f.Hint,
		Side:    f.Side,
		Name:    f.Name,
	}
}
