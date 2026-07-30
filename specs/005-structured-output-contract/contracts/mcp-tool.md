# Contract: MCP `find_transit_route` 도구 결과

**Feature**: 005-structured-output-contract | **SDK**: `modelcontextprotocol/go-sdk` v1.6.1

## 입력

변경 없음.

```go
type RouteToolInput struct {
    From string `json:"from"`
    To   string `json:"to"`
}
```

## 성공 결과

변경 없음 (기존 동작).

```json
{
  "content": [{ "type": "text", "text": "{\"totalTimeMinutes\":42,...}" }],
  "structuredContent": { "totalTimeMinutes": 42, "transferCount": 1, "fareWon": 1500, "steps": ["..."] },
  "isError": false
}
```

## 실패 결과 — **변경 지점**

### 현재 (결함)

```json
{
  "content": [{ "type": "text", "text": "경로 검색 중 오류가 발생했습니다: core: ODsay rejected the request (code 500): internal db timeout at shard 7" }],
  "isError": true
}
```

`structuredContent` 없음. AI는 한국어 문장 하나로 후속 행동을 결정해야 하고, 미분류 에러는
ODsay 서버 내부 메시지를 그대로 노출한다.

### 변경 후

```json
{
  "content": [{ "type": "text", "text": "장소 검색 요청이 일시적으로 제한되었습니다. 잠시 후 다시 시도하세요" }],
  "structuredContent": {
    "error": {
      "code": "geocoder_rate_limited",
      "message": "장소 검색 요청이 일시적으로 제한되었습니다. 잠시 후 다시 시도하세요"
    }
  },
  "isError": true
}
```

- `content[0].text` = 프로즈 (`message` + `hint`) — CLI 기본 모드와 동일 문자열
- `structuredContent.error` = CLI `--json`과 동일한 `RouteError`
- `isError` = `true`

## 구현 계약 (SDK 동작에 의존)

핸들러 시그니처는 `mcp.ToolHandlerFor[RouteToolInput, RouteToolOutput]`을 유지한다.
실패 시:

```go
return &mcp.CallToolResult{
    IsError: true,
    Content: []mcp.Content{&mcp.TextContent{Text: f.Prose()}},
}, RouteToolOutput{Error: f.toRouteError()}, nil   // ← err는 반드시 nil
```

**핸들러는 실패 시에도 error를 반환하지 않는다.** go-sdk v1.6.1의 `ToolHandlerFor` 래퍼는
핸들러가 error를 반환하면 핸들러가 만든 `*CallToolResult`를 **폐기하고** 빈 결과에
`SetError(err)`를 적용한다(`mcp/server.go:339-353`). 그 경로에서는 `structuredContent`가
사라지고 `content`가 **원본 에러 문자열**로 채워져, 이 기능이 막으려는 누출이 그대로
재현된다 (research.md §R1에 실측 결과 기록).

`err == nil` 경로에서만:

- 핸들러의 `res`가 보존되고 (`server.go:356-358`)
- `out`이 출력 스키마 검증을 거쳐 `res.StructuredContent`에 실리며 (`server.go:384`)
- `res.Content`가 이미 채워져 있으면 덮어쓰이지 않는다 (`server.go:387`)

### 회귀 방지

이 SDK 동작 의존은 **SDK 업그레이드 시 조용히 깨질 수 있다.** 실패 응답에
`structuredContent.error.code`가 실제로 존재하는지 검증하는 종단 테스트를 둔다
(in-memory transport 사용, `mcp_test.go`의 기존 `connectTestClient` 패턴).

## 자격증명 저장소 에러

`mcp.go:83`의 `fmt.Errorf("API 키 조회 실패: %w", loadErr)`는 키체인 원본 에러를 AI에게
그대로 보낸다. `credential_store_error` 코드로 분류해 같은 실패 결과 형식으로 반환한다
(FR-018).

## CLI와의 일치 (FR-016)

같은 실패에 대해 CLI와 MCP는 **같은 `code`와 같은 `message`** 를 낸다. 단일
`classifyRouteError`에서 파생되므로 구조적으로 보장되며, 기존
`TestFindTransitRouteTool_GeocoderMessagesMatchCLI`를 **코드까지 비교하도록 확장**해
회귀를 막는다.

## 출력 스키마

`RouteToolOutput`에 `Error *RouteError` 필드가 추가되므로 도구의 출력 JSON Schema가 바뀐다.
모든 필드가 optional이므로 기존 성공 응답은 계속 검증을 통과한다.
