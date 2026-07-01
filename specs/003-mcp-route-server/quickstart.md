# Quickstart: MCP 경로 검색 서버 검증

## 사전 준비

- `mise install`로 pinned 툴체인 설치.
- 실제 검증(§3)에는 `naeryeo setup`으로 저장된 실제 ODsay API 키가 필요하다.

## 1. 단위/종단 간 테스트로 대부분의 경로 검증 (API 키 불필요)

```bash
just test
```

기대 결과: `cmd/naeryeo`의 MCP 관련 테스트가 `mcp.NewInMemoryTransports()`로 실제 MCP
클라이언트-서버 왕복을 흉내 내어(research.md §6) 다음을 모두 커버하며 통과한다.

- 정상 경로: `find_transit_route` 호출 결과에 소요시간·환승 횟수·요금·단계별 안내가 채워짐
- 이동 불필요: `noTravelNeeded: true`가 에러 없이 반환됨
- API 키 미설정: 명확한 에러 메시지와 함께 `IsError: true`
- API 키 무효: 키 미설정과 다른 문구로 `IsError: true`
- 지점 인식 불가: 출발지/도착지 중 어느 쪽인지 알 수 있는 문구로 `IsError: true`
- 경로 없음 / 업스트림 장애: 각각 구분되는 문구로 `IsError: true`, 이후 같은 세션에서
  다른 호출을 다시 보내도 정상 처리됨(서버가 죽지 않음, FR-009)

## 2. 도구 스키마 확인 (API 키 불필요)

인메모리 클라이언트로 `tools/list`를 호출해 `find_transit_route`의 입력/출력 스키마가
`from`/`to`(둘 다 필수)와 `noTravelNeeded`/`totalTimeMinutes`/`transferCount`/`fareWon`/
`steps` 필드를 포함하는지 확인한다.

## 3. Claude Desktop/Code로 수동 검증 (API 키 필요)

```bash
go build -o /tmp/naeryeo ./cmd/naeryeo
```

`claude_desktop_config.json`(또는 Claude Code의 동등 설정)에 README대로 등록:

```json
{
  "mcpServers": {
    "naeryeo": {
      "command": "/tmp/naeryeo",
      "args": ["mcp"]
    }
  }
}
```

Claude Desktop/Code를 재시작한 뒤 다음을 순서대로 물어본다:

1. "지금 강남역에서 홍대입구역까지 대중교통으로 어떻게 가?" → 총 소요시간·환승 횟수·요금·
   단계별 안내가 자연어로 설명됨.
2. "강남역에서 강남역까지 가는 법 알려줘" → 이동이 필요 없다는 안내.
3. "존재하지않는가짜지명123에서 홍대입구역까지" → 출발지를 인식할 수 없다는 안내, 그리고
   Claude Desktop/Code를 재시작하지 않고 곧바로 1번 질문을 다시 해도 정상 응답됨(FR-009).
4. `naeryeo logout` 실행 후 다시 1번 질문 → API 키가 설정되지 않았다는 안내(`naeryeo
   setup` 실행 유도).

## 4. 품질 게이트

```bash
just check   # fmt + lint + test
```
