# Phase 1 Data Model: MCP 경로 검색 서버

이 기능은 새로운 영속 데이터를 두지 않는다. "데이터"는 MCP 툴 호출의 입력/출력 스키마와,
002의 `core.RouteResult`를 MCP 응답으로 옮기는 매핑이다.

## Entity: RouteToolInput (툴 입력)

| Field | Type | Description | Validation |
|---|---|---|---|
| `From` | string (`json:"from"`) | 출발지 명칭(역/정류장 이름 또는 주소). | 빈 문자열이면 SDK가 스키마 검증에서 거부(필수 필드로 선언). |
| `To` | string (`json:"to"`) | 도착지 명칭. | 위와 동일. |

## Entity: RouteToolOutput (툴 출력)

| Field | Type | Description |
|---|---|---|
| `NoTravelNeeded` | bool (`json:"noTravelNeeded,omitempty"`) | `true`면 출발지·도착지가 사실상 동일/충분히 가까운 위치(002의 `RouteResult.NoTravelNeeded` 그대로). |
| `TotalTimeMinutes` | int (`json:"totalTimeMinutes,omitempty"`) | 총 소요시간(분). |
| `TransferCount` | int (`json:"transferCount,omitempty"`) | 총 환승 횟수. |
| `FareWon` | int (`json:"fareWon,omitempty"`) | 예상 요금(원). |
| `Steps` | []string (`json:"steps,omitempty"`) | 순서가 있는 단계별 이동 안내 문장(002의 `RouteStep.Description`을 그대로 옮김). |

성공 시에는 이 구조체가 `CallToolResult.StructuredContent`로, 실패 시에는 아래 에러 규칙에
따라 `CallToolResult.IsError=true` + 사람이 읽을 수 있는 메시지로 응답된다(둘 다 SDK가
`AddTool`을 통해 자동 처리, research.md §1~§2 참조).

## Errors (에러 사유 문구)

이 기능은 새 에러 타입을 정의하지 않는다. 002의 `core` 패키지 에러
(`ErrAPIKeyMissing`/`ErrAuthFailed`/`ErrPointNotFound`/`ErrNoRoute`/`ErrUpstreamUnavailable`/
`ErrUpstreamRejected`)와 001의 `config.ErrNotConfigured`를, `cmd/naeryeo/route.go`의
`reportRouteError`가 이미 만들어둔 것과 동일한 분기로 한국어 문구로 변환해 그대로 Go
`error`로 반환한다. 이 변환 로직은 CLI(`route.go`)와 MCP(`mcp.go`) 양쪽이 공유하는 하나의
함수로 추출한다(FR-012, contracts/mcp-tool.md 참조).

| 원인 | 대응 spec 요구사항 |
|---|---|
| API 키 미설정(`config.ErrNotConfigured`) | FR-004 |
| API 키 무효(`core.ErrAuthFailed`) | FR-005 |
| 지점 인식 불가(`*core.ErrPointNotFound`) | FR-006 |
| 경로 없음(`core.ErrNoRoute`) | FR-007 |
| 업스트림 장애(`core.ErrUpstreamUnavailable`/`core.ErrUpstreamRejected`) | FR-008 |

## Package Surface (cmd/naeryeo, 내부 전용 — 외부에 노출되는 Go API 아님)

```go
// buildMCPServer assembles the MCP server and registers the find_transit_route
// tool. It takes load/findRoute as parameters (same shape as runRoute's) so it
// can be unit- and end-to-end-tested without touching internal/config or a
// real ODsay call.
func buildMCPServer(
    version string,
    load func() (string, error),
    findRoute func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error),
) *mcp.Server

// runMCP runs the assembled server over the real process stdio until the
// client disconnects. It is the thin, effectively untested glue invoked from
// main.go's "mcp" case.
func runMCP(ctx context.Context, server *mcp.Server) error
```
