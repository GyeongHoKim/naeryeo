# Phase 0 Research: MCP 경로 검색 서버

## 1. Go MCP SDK 선택

**Decision**: 공식 SDK `github.com/modelcontextprotocol/go-sdk`(`mcp` 서브패키지)를 사용한다.
context7(`/modelcontextprotocol/go-sdk`, 최신 태그 `v1.6.1` 확인, 2026-07-01 조회)로 API를
검증했다. 로컬 모듈 캐시(`go get .../go-sdk/mcp@latest`로 내려받은 소스)를 직접 열어
`NewServer`/`AddTool`/`Run`/`StdioTransport`의 실제 시그니처가 문서와 일치함을 재확인했다.
SDK는 `go 1.25.0` 이상을 요구하며, 이 프로젝트는 이미 `go 1.26.4`라 문제 없다.

핵심 API:
```go
server := mcp.NewServer(&mcp.Implementation{Name: "naeryeo", Version: version}, nil)
mcp.AddTool(server, &mcp.Tool{Name: "...", Description: "..."}, handler)
err := server.Run(ctx, &mcp.StdioTransport{})
```
핸들러 시그니처(제네릭): `func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error)`.
`In`/`Out`은 구조체 태그(`json`, `jsonschema`)로부터 입출력 스키마가 자동 추론된다.

**Rationale**: Anthropic이 유지하는 공식 SDK이며, `go-keyring`(001)·표준 라이브러리(002)와
같은 원칙으로 "검증된 외부 라이브러리에 위임"하는 이 프로젝트의 기존 패턴과 정합적이다.
직접 JSON-RPC/stdio 프레이밍을 구현하는 것은 정당화되지 않는 복잡도(constitution Principle
I 위반)이므로 제외한다.

**Alternatives considered**: 커뮤니티 SDK(예: `mark3labs/mcp-go`) — 공식 SDK가 안정적으로
존재하는 이상 추가 검토 없이 기각. 프로토콜을 직접 구현 — 위 이유로 기각.

## 2. 에러를 클라이언트에 전달하는 방식 — 그냥 Go 에러를 반환하면 된다

**Decision**: 툴 핸들러가 평범한 `error`를 반환하면 SDK가 자동으로
`CallToolResult.IsError = true`를 설정하고 에러 메시지를 텍스트 콘텐츠로 채워 클라이언트에
전달한다(로컬 소스 `protocol.go`의 `CallToolResult.IsError` 필드 주석으로 확인: "When using
a ToolHandlerFor, this field is automatically set when the tool handler returns an error,
and the error string is included as text in the Content field."). 별도의 구조화된 에러
페이로드를 손수 만들 필요가 없다.

**Rationale**: spec의 FR-004~FR-008(키 미설정/키 무효/지점 인식 불가/경로 없음/업스트림
장애 각각을 구분되는 사유로 전달)은 곧 "에러 메시지 문구를 구분되게 작성"하는 문제로
단순화된다. 이는 `cmd/naeryeo/route.go`가 이미 구현한 `reportRouteError`의 분기 로직과
거의 동일하다 — 두 진입점이 `package main` 안에 있으므로, 에러 원인을 사람이 읽을 수 있는
한국어 문구로 변환하는 로직을 **하나의 공용 함수로 추출해 CLI(`route.go`)와 MCP(`mcp.go`)
양쪽에서 재사용**한다(FR-012, 002의 FR-013과 동일한 "두 진입점 판단 로직 공유" 원칙).

**Alternatives considered**: 구조화된 에러 코드/필드를 `Out` 구조체에 별도로 정의 — MCP
클라이언트(Claude)는 결국 텍스트를 사람이 읽고 자연어로 설명하므로, 사람이 읽을 수 있는
에러 문자열이면 충분하고 구조화된 에러 스키마는 이 시점에는 정당화되지 않는 복잡도라 기각.

## 3. stdout은 프로토콜 전용 — 다른 어떤 것도 stdout에 쓰면 안 된다

**Decision**: `mcp.StdioTransport{}`는 서버 프로세스의 `os.Stdin`/`os.Stdout`에 직접
바인딩되어 개행 구분 JSON을 주고받는다(로컬 소스 확인). 따라서 `naeryeo mcp` 실행 경로에서는
`fmt.Println`류의 어떤 코드도 stdout에 출력해서는 안 된다 — 프로토콜 스트림이 깨진다. 진단이
필요하면 `os.Stderr`로만 출력한다.

**Rationale**: README의 "왜 stdio인가" 절이 이미 이 모델을 전제하고 있다. `cmd/naeryeo`의
기존 `run(args, stdout, stderr)` 함수 시그니처는 `setup`/`logout`/`route`가 응답을 stdout에
쓰는 것을 전제로 하지만, `mcp` 분기는 그 관례를 따르지 않고 SDK가 프로세스의 실제
`os.Stdout`을 직접 장악하게 둔다 — `mcp` 분기 진입 이후 `run`이 받은 `stdout io.Writer`
파라미터는 더 이상 사용하지 않는다.

**Alternatives considered**: 없음 — 이것은 MCP stdio 전송 모델 자체의 하드 제약이다.

## 4. 동시 요청 처리 — 이미 안전하다

**Decision**: SDK 문서(`docs/protocol.md`, Concurrency Heuristics)에 따르면 `tools/call`
요청은 서로 비동기적으로(동시에) 처리될 수 있다 — 엄격한 순차 처리가 보장되지 않는다. 별도
동기화 없이 그대로 둔다.

**Rationale**: 002에서 만든 `core.Client`는 내부 상태를 갖지 않고 매 호출마다 독립적인 HTTP
요청만 수행하며, `net/http.Client`는 동시 사용에 안전하도록 설계되어 있다. 따라서 SDK가
여러 `tools/call`을 동시에 디스패치하더라도 `core.Client.FindRoute`를 그대로 재사용하는 데
문제가 없다. spec의 FR-009("여러 요청을 순서대로 처리")는 "한 요청의 실패가 이후 요청에
영향을 주지 않는다"는 의미로 해석하며, 이는 상태 없는 설계 자체로 이미 만족된다.

**Alternatives considered**: 요청을 명시적으로 직렬화하는 큐/뮤텍스 도입 — 상태가 없어
경쟁 상태가 발생할 여지가 없으므로 정당화되지 않는 복잡도라 기각.

## 5. 툴 스키마 설계

**Decision**: 툴 이름은 `find_transit_route` 하나만 노출한다. 입력은
`{from string, to string}`(둘 다 필수), 출력은 002의 `core.RouteResult`를 그대로 반영한
구조체(`noTravelNeeded`, `totalTimeMinutes`, `transferCount`, `fareWon`, `steps []string`)로
정의한다. `jsonschema` 구조체 태그로 각 필드에 사람이 읽을 수 있는 설명을 붙여 Claude가
언제/어떻게 이 툴을 호출해야 하는지 스스로 판단할 수 있게 한다.

**Rationale**: spec은 단일 도구(경로 검색)만 요구한다. 여러 개의 세분화된 도구(예: 지점
검색 도구와 경로 검색 도구를 분리)는 지금 요구되지 않는 확장이므로 배제한다(YAGNI).

**Alternatives considered**: `core.RouteStep`을 그대로 구조체 배열로 노출 — Claude 입장에서는
사람이 읽을 수 있는 문자열 배열이면 충분하고 추가 필드(노선 코드 등)를 소비할 곳이 없어
`[]string`으로 단순화한다(002의 `data-model.md`가 이미 `RouteStep`을 `Description string`
하나로 최소화해둔 것과 동일한 방향).

## 6. `cmd/naeryeo` 진입점 구조

**Decision**: `internal/core`/`internal/config`에는 변경이 필요 없다. 새 파일
`cmd/naeryeo/mcp.go`에 서버 구성(`buildMCPServer`, 테스트 가능)과 실행(`runMCP`, 실제
`os.Stdin`/`os.Stdout`을 SDK에 넘기는 얇은 glue) 두 층을 분리한다 — 001/002에서 이미 써온
"테스트 가능한 핵심 로직 + main.go의 얇은 배선" 패턴을 그대로 따른다.

**Rationale**: 실제 stdio 트랜스포트(SDK가 프로세스의 진짜 stdin/stdout을 장악)는
서브프로세스를 띄우고 JSON-RPC를 주고받아야 해서 그 자체로는 단위 테스트하기 번거롭다.
대신 로컬 소스에서 `mcp.NewInMemoryTransports()`(서로 연결된 인메모리 트랜스포트 쌍을
반환)와 `mcp.NewClient` + `(*ClientSession).CallTool`을 확인했다 — 이를 이용하면 실제
`os.Stdin`/`os.Stdout` 없이도 진짜 MCP 클라이언트-서버 왕복(JSON-RPC 직렬화/역직렬화
포함)을 테이블 테스트로 검증할 수 있다: 서버를 한쪽 트랜스포트로 고루틴에서 `Run`시키고,
클라이언트를 다른 쪽 트랜스포트로 `Connect`한 뒤 `CallTool`을 호출해 `CallToolResult`를
검증한다. 이 방식을 `buildMCPServer(findRoute, load)`(서버 조립, 테스트 가능)의 종단 간
테스트로 채택한다.

**Alternatives considered**: 툴 핸들러 함수를 SDK를 거치지 않고 직접 호출하는 순수 단위
테스트만 — `CallToolRequest`/`CallToolResult`를 손으로 만들어야 해서 스키마 추론·에러
래핑 등 SDK가 실제로 해주는 변환을 검증하지 못한다. 인메모리 트랜스포트 방식이 비용 대비
더 신뢰도 높은 테스트라 이를 채택하고, 손으로 만드는 방식은 사용하지 않는다.
