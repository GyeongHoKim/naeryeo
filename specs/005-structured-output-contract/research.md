# Phase 0 Research: 구조화된 출력 계약

**Feature**: 005-structured-output-contract | **Date**: 2026-07-30

---

## R1. MCP 실패 응답에 구조화된 코드를 싣는 방법

**Decision**: 핸들러가 **error를 반환하지 않고**(`err == nil`), `*mcp.CallToolResult`를 직접
구성해 `IsError: true` + 한국어 메시지를 `Content`에 넣고, 구조화된 실패 페이로드는 **`Out`
타입(봉투형)으로 반환**한다.

**Rationale**: go-sdk v1.6.1의 `ToolHandlerFor` 래퍼(`mcp/server.go:339-353`)는 핸들러가
error를 반환하면 핸들러가 만든 `res`를 **버리고 빈 `CallToolResult`를 새로 만든다**:

```go
res, out, err := h(ctx, req, in)
if err != nil {
    ...
    var errRes CallToolResult   // 핸들러의 res는 여기서 폐기됨
    errRes.SetError(err)
    return &errRes, nil
}
```

`err == nil`인 경로에서만 핸들러의 `res`가 보존되고(`server.go:356-358`), `out`이 스키마
검증을 거쳐 `res.StructuredContent`에 실린다(`server.go:384`). `res.Content`가 이미 채워져
있으면 덮어쓰지 않는다(`server.go:387`).

**실측 검증** (v1.6.1, in-memory transport로 3개 시나리오 확인 후 프로브 삭제):

| 방식 | 결과 |
| --- | --- |
| A. 결과 직접 구성 **+ error 반환** | `structuredContent: null`, content = **원본 에러 문자열** — 결과가 통째로 폐기됨 |
| B. 봉투형 `Out` + `IsError` 수동 설정, error 미반환 | `isError: true`, content = 한국어 메시지, `structuredContent` = 실패 페이로드 ✅ |
| B. 동일 타입의 성공 경로 | `{"totalTimeMinutes": 42}` — 성공 문서 깨끗함 ✅ |

> ⚠️ **GYE-292의 구현 노트 정정**: 이슈는 "`SetError`는 Content가 이미 채워져 있으면
> 덮어쓰지 않으므로, 핸들러에서 `*mcp.CallToolResult`를 직접 구성해 구조화된 에러 content를
> 넣는 방식이 SDK와 잘 맞는다"고 적었다. `SetError`의 그 동작 자체는 사실이나(`protocol.go:132-137`),
> **핸들러가 error를 함께 반환하면 그 결과가 래퍼에서 폐기되므로 적용되지 않는다.** 위 표의
> 방식 A가 정확히 그 경우이며, 실측 결과 이 기능이 막으려는 "원본 에러 노출"이 그대로
> 재현된다. `SetError`의 Content 보존은 서버 미들웨어처럼 **호출자가 직접 결과를 만들고
> SetError를 부르는** 경우에만 의미가 있다.

**Alternatives considered**:

- *방식 A (GYE-292 원안)* — 위와 같이 실측에서 실패. 기각.
- *`Out`을 `any`로 선언* — 래퍼의 출력 스키마 해석을 피할 수 있으나(`server.go:307`),
  도구 출력 스키마를 잃어 MCP 클라이언트가 성공 결과 구조를 알 수 없게 된다. 기각.
- *untyped `ToolHandler`로 등록* — 완전한 제어를 얻지만 입력 스키마 자동 생성·검증까지
  직접 해야 한다. 얻는 것 대비 비용이 크다. 기각.
- *`MCPGODEBUG=seterroroverwrite=1`* — 1.6.0 이전 동작 복원 플래그이며 1.8.0에서 제거 예정
  (`protocol.go:118-123`). 사라질 동작에 의존. 기각.

---

## R2. CLI와 MCP가 같은 문서 구조를 갖는 방법 (FR-010, FR-016)

**Decision**: 성공 필드와 선택적 `error` 객체를 함께 갖는 **단일 봉투 타입**을 정의하고,
CLI `--json`과 MCP `Out`이 **같은 Go 타입**을 쓴다.

```jsonc
// 성공 — error 키 없음
{ "totalTimeMinutes": 42, "transferCount": 1, "fareWon": 1500, "steps": ["..."] }

// 실패 — 성공 필드 없음
{ "error": { "code": "geocoder_rate_limited", "message": "..." } }
```

**Rationale**: R1의 방식 B는 실패 페이로드를 `Out` 타입으로 실어야 하므로, 성공과 실패가
하나의 Go 타입이어야 한다. 이는 FR-010("성공 문서 구조가 MCP와 동일")을 **타입 동일성으로
구조적으로 보장**한다 — 두 스키마를 동기화하는 테스트가 아니라, 애초에 갈라질 수 없는 구조다.
`error` 키의 유무만으로 성공/실패를 판별할 수 있어 호출자 분기도 단순하다.

**Alternatives considered**:

- *성공/실패를 서로 다른 두 타입으로* — GYE-292 원안. R1 방식 B에서는 `Out`이 하나뿐이라
  성립하지 않고, 동등성 테스트로 스키마 동기화를 지켜야 한다. 기각.
- *`error`를 최상위 문자열 코드로 (`{"error": "geocoder_rejected", "message": "..."}`)* —
  GYE-292 원안의 평평한 형태. 성공 필드와 같은 레벨에 섞여 필드명 충돌 위험이 있고,
  `hint`/`side`/`name`까지 최상위로 올라와 성공 스키마를 오염시킨다. 중첩 객체로 변경.

---

## R3. 코드 누락을 자동 검출하는 게이트 (FR-004)

**Decision**: 테스트에서 **`go/parser`+`go/ast`(표준 라이브러리)로 `internal/core`의 소스를
파싱**해 exported 에러 심볼을 전부 수집하고, 테스트 로컬 표에 각 심볼의 샘플 값이 등록되어
있는지 + 그 값이 `internal_error`가 아닌 코드로 분류되는지 검사한다.

**Rationale**: Go는 패키지 수준 var를 런타임 리플렉션으로 열거할 수 없어, "모든 에러가
매핑되었는가"를 코드로 확인할 방법이 소스 파싱밖에 없다. 수집 대상:

1. `var ErrXxx = errors.New(...)` — exported, `Err` 접두 식별자
2. `type ErrXxx struct` + 포인터 리시버 `Error() string` 메서드

새 에러가 추가되면 표에 항목이 없어 테스트가 실패하고, 실패 메시지가 "코드를 부여하라"고
지시한다. 새 의존성이 없고(표준 라이브러리), 테스트 전용이라 프로덕션 바이너리에 영향이 없다.

**이중 방어**: 분류 함수 자체도 `default`에서 원본 에러를 노출하지 않고 `internal_error`
코드와 고정 문구를 반환한다. 게이트를 우회하더라도 FR-005(원문 미노출)는 런타임에 지켜진다.

**허용 목록**: `core.ErrGeocoderNotFound`는 `Geocoder` 인터페이스의 계약 sentinel로,
`resolveStation`이 `*ErrPointNotFound`로 접어버려 표현 계층에 도달하지 않는다. 표에 "표현
계층 미도달" 사유와 함께 명시적으로 등록한다 — 조용히 빠뜨리는 것과 달리 의도가 기록된다.

**Alternatives considered**:

- *`golang.org/x/tools/go/packages`* — 타입 정보까지 얻어 더 정확하지만 새 의존성이 필요하다.
  `Err` 접두 관례로 충분히 잡히므로 기각.
- *`core`에 코드 레지스트리를 두고 core가 자기 코드를 반환* — 표현 계층 관심사를 core에
  밀어넣고, 레지스트리 자체가 드리프트할 수 있다. 기각.
- *`go-check-sumtype` 등 린터* — 에러 값(인터페이스 구현)에는 적용되지 않는다. 기각.

---

## R4. 인자 검증 실패의 기계 판독 출력 (FR-015)

**Decision**: `main.go`의 기존 `hasDebugFlag` 선스캔과 **같은 패턴**으로 `--json`을 FlagSet
파싱 이전에 감지한다. 기계 판독 모드에서는 FlagSet의 출력을 버리고(`fs.SetOutput(io.Discard)`),
파싱 실패·필수 인자 누락을 `invalid_arguments` 코드의 실패 문서로 stdout에 낸다.

**Rationale**: `flag.ContinueOnError`는 파싱 실패 시 사용법을 `fs.Output()`에 쓰므로,
플래그가 파싱된 뒤에야 `--json` 여부를 알 수 있는 구조로는 FR-015를 만족할 수 없다.
`main.go:60-70`이 이미 `--debug`에 대해 동일한 선스캔을 하고 있어 새 패턴이 아니다.

**코드 추가**: `invalid_arguments`는 spec FR-003 표에 없다. FR-003은 "각각 구별되는 코드를
가져야 한다"는 **최소 집합**이고, FR-015가 요구하는 항목이므로 taxonomy에 추가한다.

---

## R5. 진단 모드(`--debug`)와의 조합 (FR-014)

**Decision**: 기계 판독 모드에서 `--debug`의 원본 에러 체인은 **stderr로만** 나가고, stdout의
JSON 문서에는 포함하지 않는다.

**Rationale**: FR-008의 "stdout에 문서 하나" 불변식을 깨지 않는 유일한 방식이다. 현재
`reportRouteError`는 `\n[debug] %v`를 같은 stderr 메시지에 이어붙이는데(`route.go:88-90`),
기계 판독 모드에서는 메시지가 stdout으로 가므로 두 스트림이 분리된다. `--debug`는 이미
`newLogger`를 통해 stderr 로깅을 켜므로(`main.go:38-45`) 스트림 선택이 일관된다.

GYE-293이 소스 레벨에서 ODsay 키를 가려두었으므로, 원본 체인을 stderr에 내보내도 키는 새지
않는다.

---

## R6. 분류 로직의 위치

**Decision**: `cmd/naeryeo/errcode.go`에 두고, 기존 `routeErrorMessage`를 이를 감싸는 형태로
재구성한다. 새 패키지를 만들지 않는다.

**Rationale**: 에러 코드는 **표현 계층 계약**이고, 소비자는 `route.go`와 `mcp.go` 둘 다
`cmd/naeryeo` 안에 있다. 헌법 원칙 I(소비 패키지가 인터페이스를 정의, 추상화는 복잡도를
정당화해야 함)에 따라 `internal/core`에 밀어넣지 않고, 소비자가 하나뿐인 새 패키지도 만들지
않는다. `routeErrorMessage`가 이미 같은 자리에서 두 진입점에 공유되고 있다(spec 002 FR-013,
spec 003 FR-012).

**구조**: 단일 분류 함수가 `failure` 값을 반환하고, 세 소비자가 각자 필요한 표현으로 투영한다.

```
classifyRouteError(err, geocoderConfigured) → failure{Code, Message, Hint, Side, Name}
   ├─ 프로즈 (route.go 기본)   : Message + "\n" + Hint   ← 기존 출력과 바이트 동일
   ├─ JSON  (route.go --json)  : 봉투{Error: {...}}
   └─ MCP   (mcp.go)           : Content=프로즈, StructuredContent=봉투{Error: {...}}
```

이 구조가 FR-016(두 진입점의 코드·문구 일치)을 **단일 원천으로 구조적으로 보장**한다.

---

## R7. 에러 코드 목록 (확정)

`internal_error`를 포함해 총 14개. 상세 매핑은 [contracts/error-codes.md](./contracts/error-codes.md) 참조.

| # | 코드 | 트리거 | 현재 상태 |
| --- | --- | --- | --- |
| 1 | `api_key_missing` | `core.ErrAPIKeyMissing`, `config.ErrNotConfigured` | 문구 있음 |
| 2 | `auth_failed` | `core.ErrAuthFailed` | 문구 있음 |
| 3 | `geocoder_auth_failed` | `core.ErrGeocoderAuthFailed` | 문구 있음 |
| 4 | `geocoder_forbidden` | `core.ErrGeocoderForbidden` | 문구 있음 |
| 5 | `geocoder_rate_limited` | `*core.ErrGeocoderRejected` + `RateLimited()` | 문구 있음 |
| 6 | `geocoder_rejected` | `*core.ErrGeocoderRejected` 그 외 | 문구 있음 |
| 7 | `point_not_found` | `*core.ErrPointNotFound` | 문구 있음 (+side/name/hint) |
| 8 | `no_route` | `core.ErrNoRoute` | 문구 있음 |
| 9 | `geocoder_unavailable` | `core.ErrGeocoderUnavailable` | 문구 있음 |
| 10 | `upstream_unavailable` | `core.ErrUpstreamUnavailable` | ❌ **default로 누출** |
| 11 | `upstream_rejected` | `*core.ErrUpstreamRejected` | ❌ **default로 누출 (ODsay 원문)** |
| 12 | `credential_store_error` | 키체인 조회 실패 (`ErrNotConfigured` 제외) | ❌ **원문 누출** (route.go:47, mcp.go:83) |
| 13 | `invalid_arguments` | 플래그 파싱 실패, `--from`/`--to` 누락 | 프로즈만 |
| 14 | `internal_error` | 위 어디에도 해당 없음 (도달 불가여야 함) | ❌ **원문 누출** |

10·11·12·14가 이 기능이 고치는 기존 결함이다.

---

## 미해결 항목

없음. Technical Context에 NEEDS CLARIFICATION 없음.
