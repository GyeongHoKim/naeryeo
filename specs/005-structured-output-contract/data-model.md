# Phase 1 Data Model: 구조화된 출력 계약

**Feature**: 005-structured-output-contract | **Date**: 2026-07-30

이 기능은 영속 데이터를 추가하지 않는다. 아래는 **출력 계약을 이루는 값 타입**과 그 불변식이다.
모두 `cmd/naeryeo` 패키지(표현 계층)에 위치한다 — research.md §R6 참조.

---

## 1. `ErrorCode` (string)

에러 코드. 한 번 공개되면 호환성 계약의 일부가 된다.

| 속성 | 값 |
| --- | --- |
| 표현 | `snake_case` 문자열 상수 |
| 집합 | 14개 (research.md §R7 / contracts/error-codes.md) |
| 안정성 | 변경·삭제는 breaking change. 추가는 minor |

**불변식**

- 모든 코드는 **호출자의 후속 행동**과 1:1 대응한다 (FR-002). 내부 에러 타입과의 대응은
  1:N(같은 타입이 두 코드로 갈림) 또는 N:1(다른 타입이 한 코드로 모임)일 수 있다.
  - 1:N 예: `*core.ErrGeocoderRejected` → `geocoder_rate_limited` | `geocoder_rejected`
  - N:1 예: `core.ErrAPIKeyMissing`, `config.ErrNotConfigured` → `api_key_missing`
- `internal_error`는 도달 불가여야 한다. 도달했다면 §4 게이트의 누락을 뜻한다.

---

## 2. `failure` (분류 결과, 내부 값 타입)

`classifyRouteError(err error, geocoderConfigured bool) failure`의 반환값. 프로즈·JSON·MCP
세 표현이 모두 여기서 파생된다 — **단일 원천**.

| 필드 | 타입 | 필수 | 설명 |
| --- | --- | --- | --- |
| `Code` | `ErrorCode` | ✅ | §1 |
| `Message` | `string` | ✅ | 사람에게 그대로 전달할 한국어 사유 |
| `Hint` | `string` | | 사용자가 취할 조치. 없으면 `""` |
| `Side` | `string` | | `point_not_found` 전용: `from`/`to`/`both` |
| `Name` | `string` | | `point_not_found` 전용: 인식 실패한 입력값 |

**불변식**

- `Message`·`Hint` 어디에도 외부 제공자·저장소의 **원본 문자열이 포함되지 않는다** (FR-005).
  원본 값은 `--debug` 경로에서 stderr로만 나간다 (R5).
- `Side`/`Name`은 `Code == point_not_found`일 때만 채워진다 (FR-012).
- `Hint`는 조치가 실제로 도움이 될 때만 채워진다. 현재 해당하는 코드는 둘이다 —
  `point_not_found`(지오코더 미설정일 때만, 기존 FR-007 조건 유지)와
  `credential_store_error`(항상).

**상태 전이**: 없음 (순수 값).

---

## 3. 출력 봉투 (`RouteToolOutput`)

CLI `--json`과 MCP 도구 출력이 **공유하는 단일 Go 타입** (R2). 기존
`cmd/naeryeo/mcp.go`의 `RouteToolOutput`을 확장한다.

| 필드 | JSON 키 | 타입 | 설명 |
| --- | --- | --- | --- |
| `NoTravelNeeded` | `noTravelNeeded` | `bool` | 이동 불필요 |
| `TotalTimeMinutes` | `totalTimeMinutes` | `int` | 총 소요시간(분) |
| `TransferCount` | `transferCount` | `int` | 환승 횟수 |
| `FareWon` | `fareWon` | `int` | 예상 요금(원) |
| `Steps` | `steps` | `[]string` | 단계별 안내 |
| `Error` | `error` | `*RouteError` | **신규**. 실패 시에만 존재 |

모든 필드 `omitempty`.

**불변식**

- `Error != nil` ⟺ 실패. 호출자는 **`error` 키 하나만 보고** 성공/실패를 판별한다.
- `Error != nil`이면 성공 필드는 모두 제로값 → 직렬화 시 나타나지 않는다.
- CLI와 MCP가 **같은 타입**을 쓰므로 FR-010의 스키마 동일성이 구조적으로 보장된다 —
  두 스키마를 손으로 맞추는 동기화 작업이 필요 없다. 다만 FR-010은 자동 검증도 요구하므로,
  같은 `core.RouteResult`에서 두 경로가 바이트 동일한 JSON을 내는지 확인하는 회귀 테스트를
  둔다(tasks T023). 타입 동일성이 1차 보장, 테스트가 2차 방어다.

### `RouteError`

| 필드 | JSON 키 | 타입 | 필수 |
| --- | --- | --- | --- |
| `Code` | `code` | `string` | ✅ |
| `Message` | `message` | `string` | ✅ |
| `Hint` | `hint` | `string` | |
| `Side` | `side` | `string` | |
| `Name` | `name` | `string` | |

`failure`의 직렬화 투영. `Hint`/`Side`/`Name`은 `omitempty`.

> `docs`(문서 링크) 필드는 도입하지 않는다 — spec Assumptions 참조. 사용처가 생기는
> GYE-295 시점에 추가한다. 추가는 optional 필드이므로 non-breaking.

---

## 4. 코드 망라성 게이트 (테스트 전용)

프로덕션 타입이 아니라 **테스트가 유지하는 표**다 (R3).

| 항목 | 내용 |
| --- | --- |
| 입력 | `go/parser`로 파싱한 `internal/core/*.go`의 exported 에러 심볼 |
| 대조표 | 심볼 이름 → 샘플 값 (테스트 로컬) |
| 검사 1 | 발견된 모든 심볼이 대조표에 존재 |
| 검사 2 | 각 샘플 값이 `internal_error`가 **아닌** 코드로 분류됨 |
| 허용 목록 | `ErrGeocoderNotFound` — 표현 계층 미도달 (사유 주석 필수) |

**불변식**: `internal/core`에 exported 에러가 추가되면 검사 1이 실패한다 (FR-004, SC-004).

---

## 관계도

```
core/config 에러
      │
      ▼
classifyRouteError(err, geocoderConfigured)      ← 단일 분류 지점
      │
      ▼
   failure {Code, Message, Hint, Side, Name}     ← 단일 원천
      │
      ├──────────────┬──────────────────┐
      ▼              ▼                  ▼
   프로즈         RouteError        RouteError
 (route.go 기본)  → 봉투            → 봉투
                 (route.go --json)  (mcp.go StructuredContent)
                                     + 프로즈 → mcp Content
```
