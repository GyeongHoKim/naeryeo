# Contract: `find_transit_route` MCP tool

Claude Desktop/Code(MCP 클라이언트)가 이 서버에 대해 호출할 수 있는 유일한 도구다. 이
계약은 이 기능이 MCP 클라이언트에게 제공하는 안정 경계이며, `internal/core`(002)의
`Client.FindRoute` 계약을 그대로 감싼다.

## Tool

- **Name**: `find_transit_route`
- **Description**: 대한민국 대중교통(지하철·버스·시외버스)으로 두 지점 사이의 경로를 검색한다.

### Input schema (구조체 → 자동 추론)

```go
type RouteToolInput struct {
    From string `json:"from" jsonschema:"출발지 (역/정류장 이름 또는 주소)"`
    To   string `json:"to" jsonschema:"도착지 (역/정류장 이름 또는 주소)"`
}
```

### Output schema (성공 시, 구조체 → 자동 추론)

```go
type RouteToolOutput struct {
    NoTravelNeeded   bool     `json:"noTravelNeeded,omitempty" jsonschema:"true면 출발지와 도착지가 사실상 같은 위치라 이동이 필요 없음"`
    TotalTimeMinutes int      `json:"totalTimeMinutes,omitempty" jsonschema:"총 소요시간(분)"`
    TransferCount    int      `json:"transferCount,omitempty" jsonschema:"환승 횟수"`
    FareWon          int      `json:"fareWon,omitempty" jsonschema:"예상 요금(원)"`
    Steps            []string `json:"steps,omitempty" jsonschema:"순서대로 나열된 단계별 이동 안내"`
}
```

## 동작 계약

1. **입력 검증**: `from`/`to` 중 하나라도 비어 있으면 SDK가 스키마 검증 단계에서 거부한다
   (핸들러 코드는 이 케이스를 별도로 처리하지 않는다).
2. **API 키 미설정**: 저장된 키가 없으면(FR-004) 핸들러는 성공 결과를 반환하지 않고, "API
   키가 설정되지 않았습니다. naeryeo setup을 먼저 실행하세요"를 포함하는 한국어 메시지의
   `error`를 반환한다 → SDK가 `CallToolResult.IsError=true`로 변환.
3. **API 키 무효**: `core.ErrAuthFailed`를 받으면(FR-005) "저장된 API 키가 유효하지
   않습니다..." 문구의 에러를 반환하며, 2번의 메시지와 겹치지 않는 별도 문구를 사용한다.
4. **지점 인식 불가**: `*core.ErrPointNotFound`를 받으면(FR-006) 어느 지점(출발지/도착지/
   둘 다)인지 명시하는 에러를 반환한다.
5. **경로 없음**: `core.ErrNoRoute`를 받으면(FR-007) "두 지점 사이에 대중교통 경로가
   없습니다" 문구의 에러를 반환한다.
6. **업스트림 장애**: `core.ErrUpstreamUnavailable`/`core.ErrUpstreamRejected`를 받으면
   (FR-008) 일반적인 실패 문구의 에러를 반환하되, 핸들러/서버 프로세스는 계속 살아 있어야
   하고 다음 호출을 정상적으로 받을 수 있어야 한다(FR-009).
7. **이동 불필요**: `core.RouteResult.NoTravelNeeded == true`면(FR-003) 에러가 아니라
   `RouteToolOutput{NoTravelNeeded: true}`(나머지 필드는 zero value)를 성공으로 반환한다.
8. **정상 경로**: 그 외 성공 시 `RouteResult`의 `TotalTime`/`TransferCount`/`Fare`/`Steps`를
   각각 `TotalTimeMinutes`/`TransferCount`/`FareWon`/`Steps`로 그대로 옮겨 반환한다.
9. **CLI와의 일관성**: 동일한 `from`/`to`에 대해 이 도구와 `naeryeo route --from --to`는
   같은 `core.FindRoute` 호출 결과에서 파생되므로 항상 같은 성공/실패 판정을 내린다(FR-012).

## 클라이언트(Claude)를 위한 가이드

- 에러 메시지는 사람이 읽는 한국어 문장이며, Claude는 이를 그대로 사용자에게 자연어로
  풀어 설명하면 된다 — 별도의 에러 코드 파싱이 필요 없다.
- `NoTravelNeeded: true`는 실패가 아니라 성공 응답의 한 형태이므로, "에러가 났다"가 아니라
  "이동할 필요가 없다"고 사용자에게 안내해야 한다.
