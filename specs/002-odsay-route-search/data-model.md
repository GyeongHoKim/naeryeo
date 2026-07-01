# Phase 1 Data Model: 대중교통 경로 검색

## Entity: RouteQuery (경로 검색 질의)

| Field | Type | Description | Validation |
|---|---|---|---|
| `From` | string | 출발지 명칭(역/정류장 이름 또는 주소). | 빈 문자열 불가 — 호출자(CLI/MCP)가 검증 후 넘긴다. |
| `To` | string | 도착지 명칭. | 빈 문자열 불가. |

## Entity: RouteResult (경로 검색 결과)

| Field | Type | Description |
|---|---|---|
| `NoTravelNeeded` | bool | `true`면 출발지·도착지가 사실상 동일/충분히 가까운 지점(ODsay `-98`). 이 경우 아래 나머지 필드는 모두 zero value이며 에러로 취급하지 않는다(FR-012). |
| `TotalTime` | int (분) | 총 소요시간. `ODsay info.totalTime` 그대로. |
| `TransferCount` | int | 총 환승 횟수. `info.subwayTransitCount + info.busTransitCount`. |
| `Fare` | int (원) | 예상 요금. `info.payment` 그대로. |
| `Steps` | []RouteStep | 순서가 있는 이동 단계 목록. `subPath[]`를 순서대로 변환. |

## Entity: RouteStep (이동 단계)

| Field | Type | Description |
|---|---|---|
| `Description` | string | 사람이 읽을 수 있는 한 단계 설명(예: "강남역에서 2호선 승차 → 신도림역에서 하차"). `trafficType`/`lane`/`startName`/`endName`으로부터 조립. |

간단한 문자열 필드 하나로 둔다 — CLI 출력(README 예시: "1. 강남역에서 2호선 승차 (구로디지털단지
방면)")과 향후 MCP 응답 모두 사람이 읽는 한 줄 문구로 충분하며, 구조화된 세부 필드(노선 코드 등)를
지금 요구하는 소비자가 없다(YAGNI).

## Errors (도메인 상태를 나타내는 값)

| Sentinel/타입 | 의미 | 대응 스펙 요구사항 |
|---|---|---|
| `core.ErrAPIKeyMissing` | 호출 시 `apiKey`가 빈 문자열 — 네트워크 호출 전에 즉시 반환됨. | FR-007 |
| `core.ErrAuthFailed` | ODsay가 인증 실패를 알려온 경우(구현 시점에 실제 코드 확인 필요, research.md §3). | FR-008 |
| `core.ErrPointNotFound{Side, Name}` | `Side`는 `"from"`/`"to"`/`"both"`. 어느 지점을 인식하지 못했는지 표현. | FR-009 |
| `core.ErrNoRoute` | 두 지점 사이에 대중교통 경로가 없음(ODsay `-99`, `6`). | FR-010 |
| `core.ErrUpstreamUnavailable` | 네트워크 오류, 타임아웃, ODsay `500` 등 — 원인 에러를 `%w`로 감쌈. | FR-011 |
| `core.ErrUpstreamRejected{Code, Message}` | 알려지지 않았거나 우리 쪽 요청 결함으로 보이는 ODsay 에러(`-8`,`-9`, 미분류 코드). 사용자에게는 일반적인 실패로 안내하되 원문 코드/메시지를 보존해 디버깅 가능하게 함. | (edge case — 미분류 실패의 포괄 처리) |

`ErrPointNotFound`/`ErrUpstreamRejected`는 값에 컨텍스트(어느 지점인지, 원본 코드)가 필요하므로
단순 `errors.New` sentinel이 아니라 필드를 가진 커스텀 에러 타입으로 정의하고, `errors.As`로
판별 가능하게 한다. 나머지는 `internal/config`(001)와 동일한 패키지 레벨 sentinel 패턴을 따른다.

## Package Surface (internal/core)

```go
package core

type Client struct {
    APIKey     string
    HTTPClient *http.Client // nil이면 기본 타임아웃(10s)의 클라이언트 사용
    BaseURL    string       // nil/빈 값이면 https://api.odsay.com/v1/api, 테스트에서 교체
}

func NewClient(apiKey string) *Client

// FindRoute는 from과 to 사이의 대표 대중교통 경로를 검색한다.
// apiKey가 비어 있으면(설정되지 않았으면) 네트워크 호출 없이 ErrAPIKeyMissing을 반환한다.
func (c *Client) FindRoute(ctx context.Context, from, to string) (RouteResult, error)
```

`route`/`mcp` 두 진입점은 각자 `internal/config.Load()`로 얻은 키(또는 미설정 시 빈 문자열)로
`core.NewClient(apiKey)`를 만들고 `FindRoute`를 호출하는 동일한 패턴을 공유한다(FR-013). 각
진입점은 반환된 에러 타입에 따라 자신의 표현 방식(CLI 텍스트 vs MCP 응답)으로만 다르게
포매팅하면 된다.
