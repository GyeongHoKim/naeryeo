# Phase 1: Data Model — 자체 호스팅 경로 검색 제공자

**Feature**: 006-self-hosted-routing-provider | **Date**: 2026-07-31

---

## 1. Settings (신규) — `internal/config`

비밀이 아닌 사용자 설정. 키체인과 **분리된 저장소**에 산다(research.md R8).

```go
// Settings is the non-secret configuration shared by every entry point.
type Settings struct {
    RoutingProvider RoutingProvider `json:"routing_provider"`
    MotisURL        string          `json:"motis_url,omitempty"`
    Geocoder        GeocoderChoice  `json:"geocoder,omitempty"`
}

type RoutingProvider string
const (
    ProviderUnset RoutingProvider = ""       // 파일 부재 또는 값 없음
    ProviderMotis RoutingProvider = "motis"
    ProviderODsay RoutingProvider = "odsay"
)

type GeocoderChoice string
const (
    GeocoderNone  GeocoderChoice = "none"    // 기본값
    GeocoderKakao GeocoderChoice = "kakao"
)
```

### 필드 규칙

| 필드 | 필수 | 검증 | 위반 시 |
| --- | --- | --- | --- |
| `routing_provider` | ✅ | `motis` \| `odsay` | 그 외 값·부재 → `ProviderUnset`으로 취급 → `provider_not_configured` |
| `motis_url` | provider가 `motis`일 때만 | 절대 URL, scheme `http`/`https`, 호스트 비어 있지 않음. 후행 `/` 제거 후 저장 | 저장 시 거부(setup), 로드 시 `ProviderUnset`과 동일 처리 |
| `geocoder` | ❌ | `kakao` \| `none` \| 부재 | 부재·미인식 → `GeocoderNone` |

**알 수 없는 필드는 무시한다** — 구버전 naeryeo가 신버전이 쓴 파일을 만나도 죽지 않게. 반대로
신버전이 구버전 파일을 만나면 필수 필드 부재 → `provider_not_configured` → setup 안내(FR-032).

### 상태 전이

```text
[파일 없음]
   │ setup --provider=motis --motis-url=...
   ├──────────────────────────────► [motis 설정됨]
   │ setup --provider=odsay (+ 키체인에 키 저장)      │
   └──────────────────────────────► [odsay 설정됨]   │
                                          │           │
                                          └───────────┴──► setup 재실행으로 상호 전환

[임의 상태] ──setup --delete=all──► 자격증명만 삭제. Settings는 유지
```

**`--delete`는 Settings를 지우지 않는다.** 삭제 대상은 키체인의 자격증명이다. 제공자 선택을
되돌리려면 `setup`을 다시 실행한다 — 삭제와 재설정을 한 동작에 섞으면 "키만 지우려다 제공자
설정까지 잃는" 사고가 난다.

### 노출 규칙 (FR-018)

`MotisURL`은 **사용자 출력·AI 응답 어디에도 실리지 않는다**. 사설망 호스트명·포트가 대화
기록에 남는 것을 막기 위함이다. 예외는 둘:

- `setup`이 저장 직후 사용자에게 확인차 되보여 주는 요약 (사용자 본인이 방금 입력한 값)
- `--debug` 진단 로그 (stderr 전용)

---

## 2. RouteResult 확장 — `internal/core`

```go
type RouteResult struct {
    NoTravelNeeded bool
    TotalTime      int  // minutes
    TransferCount  int
    Fare           int  // KRW — FareKnown이 false면 의미 없음
    FareKnown      bool // 신규: 제공자가 요금을 제공했는가
    Steps          []RouteStep
}
```

| 제공자 | `FareKnown` | 근거 |
| --- | --- | --- |
| ODsay | 항상 `true` | `path[].info.payment`를 항상 제공. 기존 동작·출력 불변 |
| MOTIS | 항상 `false` (v1) | `withFares`가 experimental이고 KTDB GTFS에 요금 데이터 확인 안 됨 (research.md R5) |

**불변식**: `NoTravelNeeded == true`이면 나머지 필드는 zero value이고 `FareKnown`도 `false`다.
이 조합은 "요금 정보 없음"이 아니라 "이동 자체가 불필요"를 뜻하므로, 렌더링은 `NoTravelNeeded`를
먼저 분기한다.

---

## 3. 신규 도메인 에러 — `internal/core`

`ErrGeocoder*` 계열이 이미 확립한 형태를 그대로 따른다. `internal/motis`가 `internal/core`를
단방향 의존하며 이 심볼들을 반환한다.

```go
// ErrMotisUnavailable: 연결 거부·타임아웃·DNS 실패·5xx. 잠시 후 재시도가 유효하다.
var ErrMotisUnavailable = errors.New("core: MOTIS is unavailable")

// ErrMotisRejected: 4xx 응답, 해석 불가한 본문, 예상 밖 스키마.
// Status만 보존한다 — 본문·URL은 담지 않는다(FR-018, FR-019).
type ErrMotisRejected struct {
    Status int
}
func (e *ErrMotisRejected) Error() string {
    return fmt.Sprintf("core: MOTIS rejected the request (HTTP %d)", e.Status)
}
```

**`ErrGeocoderRejected`와의 차이**: 후자는 제공자 코드·메시지를 보존한다(Kakao의 `-10` 같은
분기 신호가 필요했기 때문). `ErrMotisRejected`는 `Status`만 보존한다 — MOTIS는 사용자 자신의
서버라 분기에 쓸 표준 에러 코드 체계가 없고, 본문을 보존하면 내부망 정보가 새는 경로만 생긴다.

**게이트 상호작용**: 이 두 심볼을 `internal/core`에 추가하는 순간
`TestErrorCodeExhaustive_EveryCoreErrorHasACode`가 **실패한다**. 이는 결함이 아니라 spec
FR-020이 요구하는 동작이며, taxonomy 등록(§4)과 `coreErrorSamples` 추가로 해소한다.

---

## 4. failure / RouteError 확장 — `cmd/naeryeo`

```go
type failure struct {
    Code    errorCode
    Message string
    Hint    string
    Side    string
    Name    string
    Docs    string // 신규: 조치에 필요한 문서 URL
}
```

```go
type RouteError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Hint    string `json:"hint,omitempty"`
    Side    string `json:"side,omitempty"`
    Name    string `json:"name,omitempty"`
    Docs    string `json:"docs,omitempty"` // 신규
}
```

**`Prose()` 규칙 변경**: `Docs`가 있으면 마지막 줄로 덧붙인다.

```text
<Message>
<Hint>          ← 있을 때만
<Docs>          ← 있을 때만
```

기존 코드들은 `Docs`가 빈 문자열이므로 **프로즈 출력이 바이트 단위로 불변**이다(spec 005
FR-007). 신규 3개 코드만 3번째 줄을 갖는다.

---

## 5. MOTIS 응답 → 도메인 매핑

`internal/motis`가 수행하는 변환. 필드명은 MOTIS OpenAPI 기준(research.md R3).

### 5-1. 지오코딩 (`GET /api/v1/geocode?text=<name>`)

| MOTIS | 도메인 | 비고 |
| --- | --- | --- |
| `Match[0].lat`, `Match[0].lon` | `core.Coordinate{Y, X}` | 첫 번째 매치 사용 — ODsay station search가 `station[0]`을 쓰는 것과 동일한 정책 |
| 빈 배열 | 내부 not-found 신호 | Kakao 폴백 → 그래도 없으면 `*core.ErrPointNotFound` |

### 5-2. 경로 검색 (`GET /api/v6/plan`)

요청 파라미터:

| 파라미터 | 값 | 근거 |
| --- | --- | --- |
| `fromPlace` | `"<lat>,<lon>"` | R3 — 좌표 튜플 |
| `toPlace` | `"<lat>,<lon>"` | |
| `numItineraries` | `1` | 대표 경로 1건만 쓴다. ODsay가 `path[0]`만 쓰는 것과 동일 |

응답 매핑:

| MOTIS | `core.RouteResult` | 변환 |
| --- | --- | --- |
| `itineraries[0].duration` | `TotalTime` | 초 → 분 (반올림) |
| `itineraries[0].transfers` | `TransferCount` | 그대로 |
| — | `Fare` / `FareKnown` | `0` / `false` (R5) |
| `itineraries[0].legs[]` | `Steps[]` | leg 1개 → `RouteStep` 1개 (§5-3) |
| `itineraries` 비어 있고 `direct`도 비어 있음 | — | `core.ErrNoRoute` |

**`direct[]` 취급**: 대중교통 없이 도보만으로 닿는 경로다. `itineraries`가 비어 있고 `direct`만
있으면 그것을 사용한다 — "이동은 가능한데 경로 없음"이라고 답하는 것보다 정확하다.

### 5-3. Leg → RouteStep 문구

기존 ODsay 단계 문구(`describeSubPath`)의 어조를 맞춘다. 사람이 읽는 한국어 문장이며 안정
계약이 아니다.

| leg `mode` | 문구 |
| --- | --- |
| `WALK` | `<from.name>에서 <to.name>까지 도보 이동 (N분)` |
| 지하철 계열 (`SUBWAY`, `METRO`) | `<from.name>에서 <routeShortName> 승차 → <to.name>에서 하차` |
| `BUS` | `<from.name>에서 <routeShortName> 버스 승차 → <to.name>에서 하차` |
| 그 외 | `<from.name>에서 <to.name>까지 이동 (N분)` |

`routeShortName`이 비어 있으면 `headsign` → `agencyName` 순으로 대체하고, 셋 다 없으면
"그 외" 문구로 떨어진다.

---

## 6. 엔티티 관계

```text
Settings ──(routing_provider)──► routeFinder 선택
   │                                  │
   │                                  ├─ ProviderODsay ─► core.Client ──┐
   │                                  └─ ProviderMotis ─► motis.Client ─┤
   │                                                                    │
   ├─(motis_url)──────────────────────────────────► motis.Client.BaseURL│
   │                                                                    ▼
   └─(geocoder)──► 키체인[GeocoderAPIKey] ──► geocode.Kakao ──► core.Geocoder
                                                    (양쪽 클라이언트가 동일하게 소비)
                                                                        │
                    키체인[ODsayAPIKey] ──► core.Client.APIKey           ▼
                                                              core.RouteResult
                                                                        │
                                                        ┌───────────────┴───────────────┐
                                                        ▼                               ▼
                                              RouteToolOutput (CLI --json)      RouteToolOutput (MCP)
                                                        └─────── 동일 타입 ──────┘
```

**핵심 불변식 3가지**:

1. `geocoder` 축은 `routing_provider` 축과 **독립**이다 (spec FR-030). 4개 조합이 모두 유효.
2. `core.RouteResult`가 **유일한 합류점**이다. 제공자별 결과 타입을 만들지 않는다 (FR-011).
3. `routeFinder`를 `route`와 `mcp`가 **공유**한다. 제공자 불일치가 구조적으로 불가능 (FR-002).
