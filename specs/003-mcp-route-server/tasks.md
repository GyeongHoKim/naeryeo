---

description: "Task list for MCP 경로 검색 서버"
---

# Tasks: MCP 경로 검색 서버

**Input**: Design documents from `/specs/003-mcp-route-server/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/mcp-tool.md, quickstart.md

**Tests**: 예시가 아니라 필수다. 프로젝트 constitution(Principle II: Unit Tests Are
Mandatory)에 따라 모든 사용자 스토리에 테스트 태스크가 REQUIRED다.

**Organization**: 태스크는 spec.md의 사용자 스토리(P1~P2) 단위로 그룹화되어, 각 스토리를
독립적으로 구현·검증할 수 있다.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 병렬 실행 가능(다른 파일, 서로 의존성 없음)
- **[Story]**: 이 태스크가 속한 사용자 스토리(US1~US3)
- 모든 설명에 정확한 파일 경로 포함

## Path Conventions

001/002와 동일한 Go 표준 단일 프로젝트 레이아웃. 새 패키지 없이 `cmd/naeryeo/`에 파일만
추가한다. `internal/core`/`internal/config`는 변경하지 않는다.

---

## Phase 1: Setup

**Purpose**: MCP SDK 의존성 추가

- [X] T001 저장소 루트에서 `go get github.com/modelcontextprotocol/go-sdk/mcp@latest`
      실행(research.md §1에서 `v1.6.1` 확인) 후 `go mod tidy`로 `go.mod`/`go.sum`을
      갱신한다. `go build ./...`로 베이스라인이 정상 컴파일됨을 확인한다.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: CLI/MCP 두 진입점이 공유하는 에러 문구 함수와 툴 스키마 타입 — 이 단계가
끝나기 전에는 어떤 사용자 스토리도 시작할 수 없다.

**⚠️ CRITICAL**: 아래 태스크 완료 전까지 Phase 3 이후 착수 금지

- [X] T002 `cmd/naeryeo/route.go`를 리팩터링한다: `reportRouteError`의 `switch` 안에 있던
      사유 문구 생성 로직을 프리픽스 없는 순수 함수
      `routeErrorMessage(err error) string`(예: `"API 키가 설정되지 않았습니다. naeryeo
      setup을 먼저 실행하세요"`, `"naeryeo route: "` 같은 CLI 전용 프리픽스는 포함하지
      않음)로 추출한다. `reportRouteError`는 `"naeryeo route: "+routeErrorMessage(err)`를
      stderr에 쓰도록 수정한다. **기존 `cmd/naeryeo/route_test.go`는 수정 없이 그대로
      통과해야 한다**(문구 내용은 동일하게 유지, 프리픽스만 호출부에서 재조립).
- [X] T003 [P] 새 파일 `cmd/naeryeo/mcp.go`에 `RouteToolInput{From, To string}`,
      `RouteToolOutput{NoTravelNeeded bool; TotalTimeMinutes, TransferCount, FareWon int;
      Steps []string}` 구조체를 `json`/`jsonschema` 태그와 함께 정의한다(data-model.md,
      contracts/mcp-tool.md 참조).
- [X] T004 `cmd/naeryeo/mcp.go`에 `toRouteToolOutput(result core.RouteResult)
      RouteToolOutput` 매핑 헬퍼를 구현한다: `TotalTime`→`TotalTimeMinutes`,
      `TransferCount`→`TransferCount`, `Fare`→`FareWon`, 각 `RouteStep.Description`을
      `Steps` 문자열 슬라이스로, `NoTravelNeeded`는 그대로 옮긴다. (depends on T003)

**Checkpoint**: 공용 에러 문구 함수와 툴 스키마 타입 준비 완료 — 사용자 스토리 구현 가능

---

## Phase 3: User Story 1 - Claude Desktop/Code에서 자연어로 경로 질문 (Priority: P1) 🎯 MVP

**Goal**: 사용자가 Claude Desktop/Code에서 자연어로 경로를 물어보면 `naeryeo mcp`가
`find_transit_route` 도구로 응답하고, 한 세션 안에서 연속 질문도 처리한다.

**Independent Test**: `mcp.NewInMemoryTransports()`로 실제 MCP 클라이언트를 서버에 연결해
`find_transit_route`를 호출하고, `CallToolResult`의 구조화된 출력이 기대한 값과 일치하는지
직접 확인한다.

### Tests for User Story 1 (MANDATORY per Constitution Principle II) ⚠️

> **NOTE: 아래 테스트를 먼저 작성하고, 구현 전에는 실패(또는 컴파일 실패)함을 확인한다**

- [X] T005 [US1] 새 파일 `cmd/naeryeo/mcp_test.go`에 `mcp.NewInMemoryTransports()` +
      `mcp.NewClient` + `(*mcp.ClientSession).CallTool`로 실제 MCP 왕복을 재현하는 테스트를
      추가한다: (a) `load`/`findRoute`가 성공을 반환할 때 `find_transit_route` 결과의
      `totalTimeMinutes`/`transferCount`/`fareWon`/`steps`가 기대값과 일치, (b)
      `findRoute`가 `core.RouteResult{NoTravelNeeded: true}`를 반환할 때 결과의
      `noTravelNeeded`가 `true`이고 `IsError`가 `false`, (c) 같은 `ClientSession`으로
      연속 두 번 `CallTool`을 호출해도 각각 독립적으로 정상 처리됨을 검증한다.

### Implementation for User Story 1

- [X] T006 [US1] `cmd/naeryeo/mcp.go`에
      `routeToolHandler(ctx context.Context, req *mcp.CallToolRequest, in RouteToolInput)
      (*mcp.CallToolResult, RouteToolOutput, error)`를 구현한다: 클로저로 주입된 `load`를
      호출해 API 키를 얻고(로드 실패가 `config.ErrNotConfigured`가 아니면 즉시 에러 반환),
      `findRoute(ctx, apiKey, in.From, in.To)`를 호출해 에러면
      `errors.New(routeErrorMessage(err))`를 반환(T002 재사용 — apiKey가 빈 문자열이면
      `core.FindRoute` 자체가 `ErrAPIKeyMissing`을 반환하므로 별도의 "키 없음" 분기를
      새로 만들 필요가 없다), 성공이면 `toRouteToolOutput`으로 매핑해 반환한다.
      (depends on T002, T004)
- [X] T007 [US1] `cmd/naeryeo/mcp.go`에
      `buildMCPServer(version string, load func() (string, error), findRoute
      func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error))
      *mcp.Server`를 구현한다: `mcp.NewServer(&mcp.Implementation{Name: "naeryeo",
      Version: version}, nil)`로 서버를 만들고, `mcp.AddTool(server, &mcp.Tool{Name:
      "find_transit_route", Description: "..."}, routeToolHandler를 `load`/`findRoute`에
      바인딩한 클로저)`로 도구를 등록한다. (depends on T006)
- [X] T008 [US1] `cmd/naeryeo/mcp.go`에 `runMCP(ctx context.Context, server *mcp.Server)
      error`를 구현한다(`server.Run(ctx, &mcp.StdioTransport{})`를 그대로 호출). `cmd/
      naeryeo/main.go`의 `"mcp"` 분기를 `notImplemented(stderr, "mcp")` 대신
      `buildMCPServer`+`runMCP` 호출(실인자: `version`, `config.Load`, `core.NewClient(apiKey)
      .FindRoute`를 감싸는 클로저)로 교체한다. (depends on T007)

**Checkpoint**: `naeryeo mcp`가 실제 Claude Desktop/Code에 연결되어 경로 검색에 응답
(MVP).

---

## Phase 4: User Story 2 - API 키 문제에 대한 안내 (Priority: P1)

**Goal**: API 키 미설정/무효 상태에서도 Claude가 사용자에게 그대로 전달할 수 있는, 서로
구분되는 사유를 받는다.

**Independent Test**: `load`가 `config.ErrNotConfigured`를 반환하는 경우와 `findRoute`가
`core.ErrAuthFailed`를 반환하는 경우 각각에 대해 `find_transit_route` 호출 결과의
`IsError`와 메시지 문구를 확인한다.

### Tests for User Story 2 (MANDATORY per Constitution Principle II) ⚠️

- [X] T009 [US2] `cmd/naeryeo/mcp_test.go`에 (a) `load`가 `config.ErrNotConfigured`를
      반환할 때 `CallTool` 결과의 `IsError == true`이고 텍스트에 "naeryeo setup"이
      포함됨, (b) `findRoute`가 `core.ErrAuthFailed`를 반환할 때 `IsError == true`이고
      (a)와는 다른, "유효하지 않습니다"가 포함된 문구임을 검증하는 테스트를 추가한다.

### Implementation for User Story 2

이미 T006에서 `routeErrorMessage`(T002)를 통해 두 경우 모두 처리되도록 구현되어 있다 —
이 스토리는 그 동작을 T009 테스트로 검증하는 것으로 완결된다(추가 구현 태스크 없음).

**Checkpoint**: 키 미설정/무효 상태 모두 Claude가 사용자에게 구분해서 설명할 수 있는 사유로
응답됨.

---

## Phase 5: User Story 3 - 인식 불가 지점·경로 없음·서비스 장애에 대한 안내 (Priority: P2)

**Goal**: 오타/존재하지 않는 지점, 경로 없음, 업스트림 장애 상황에서도 서버가 죽지 않고
구분되는 사유로 응답하며, 다음 요청도 계속 처리한다.

**Independent Test**: 세 가지 실패를 각각 재현해 서로 다른 사유가 담긴 응답을 받는지, 그리고
그 이후 같은 세션에서 정상 요청을 다시 보내도 처리되는지 확인한다.

### Tests for User Story 3 (MANDATORY per Constitution Principle II) ⚠️

- [X] T010 [US3] `cmd/naeryeo/mcp_test.go`에 (a) `findRoute`가
      `&core.ErrPointNotFound{Side:"from"}`(또는 `"to"`)를 반환 → `IsError == true`이고
      어느 지점인지 알 수 있는 문구, (b) `core.ErrNoRoute` 반환 → `IsError == true`이고
      "경로가 없습니다" 류 문구, (c) `core.ErrUpstreamUnavailable` 반환 → `IsError ==
      true`이면서, 같은 `ClientSession`으로 곧바로 이어서 성공하는 `findRoute`로 다시
      `CallTool`을 호출하면 정상 결과가 반환됨(서버가 죽지 않고 다음 요청을 처리함,
      FR-009)을 검증하는 테스트를 추가한다.

### Implementation for User Story 3

이미 T006에서 완성되어 있다 — 이 스토리는 T010 테스트로 검증만 추가한다(추가 구현
태스크 없음).

**Checkpoint**: 모든 사용자 스토리가 독립적으로 동작 — 기능 전체 완성.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 스토리 전반에 걸친 마무리

- [X] T011 [P] 저장소 루트에서 `just fmt`, `just lint`, `just test`(`just check`)를
      실행하고 발견된 문제를 모두 수정한다(constitution Principle III).
- [X] T012 [P] `README.md`의 "Claude Desktop/Code에 연결하기" 절과 명령어 표가 실제 동작과
      어긋나지 않는지 확인한다(도구 이름 `find_transit_route`는 README에 노출되지 않아도
      되는 구현 세부사항이므로, README 자체의 수정은 필요하지 않을 가능성이 높다 — 확인만
      한다).
- [ ] T013 `specs/003-mcp-route-server/quickstart.md` §3(실제 Claude Desktop/Code +
      ODsay 키로 수동 검증)은 이 개발 환경에서 실행할 수 없다(API 키·실제 MCP 클라이언트
      부재). 사용자가 직접 실제 환경에서 검증해야 함을 커밋/PR 설명에 명시한다.
- [X] T014 커밋 제안 전, constitution Principle I(idiomatic Go)과 Principle II(테스트
      커버리지)를 기준으로 변경분을 자체 리뷰한다(AGENTS.md Required Workflow 5단계).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 의존성 없음 — 즉시 시작 가능
- **Foundational (Phase 2)**: Setup 완료에 의존 — 모든 사용자 스토리를 블록함
- **User Stories (Phase 3~5)**: 모두 Foundational 완료에 의존
  - US1(P1)이 `routeToolHandler`/`buildMCPServer`/`runMCP`를 전부 구현하므로, US2(P1)·
    US3(P2)는 순수하게 테스트만 추가하는 단계다 — US1 구현 완료 후에 착수한다.
- **Polish (Phase 6)**: 구현하기로 한 모든 사용자 스토리 완료에 의존

### User Story Dependencies

- **US1 (P1)**: Foundational 이후 바로 시작 가능. 다른 스토리에 의존하지 않음.
- **US2 (P1)**: T006(US1)이 만든 `routeToolHandler`가 있어야 검증할 대상이 존재하므로
  US1 구현 완료 후 착수.
- **US3 (P2)**: US2와 동일한 이유로 US1 구현 완료 후 착수. US2와는 서로 다른 에러 분기를
  검증하므로 US2와 US3는 서로 병렬로 진행 가능.

### Within Each User Story

- 테스트를 먼저 작성하고 구현 전 실패(또는 컴파일 실패)를 확인한다
- US2/US3는 구현이 이미 완료된 상태이므로 테스트만 추가하면 된다

### Parallel Opportunities

- Foundational의 T003(mcp.go 타입 정의)은 T002(route.go 리팩터링)와 다른 파일이라 병렬
  가능; T004는 T003 완료 후 순차
- Foundational 완료 후 US1 구현(T006~T008)은 순차적(같은 파일, 서로 의존)이지만, US2/US3의
  테스트 작성(T009, T010)은 US1 완료 후 서로 병렬로 진행 가능

---

## Parallel Example: Foundational → User Story 1

```bash
# Foundational에서 서로 다른 파일 작업을 함께 진행:
Task: "cmd/naeryeo/route.go에서 routeErrorMessage 추출"
Task: "cmd/naeryeo/mcp.go에 RouteToolInput/RouteToolOutput 정의"

# US1 완료 후, US2/US3 테스트를 함께 진행:
Task: "cmd/naeryeo/mcp_test.go에 API 키 문제 케이스 테스트 추가"
Task: "cmd/naeryeo/mcp_test.go에 지점 인식 불가/경로없음/업스트림 장애 케이스 테스트 추가"
```

---

## Implementation Strategy

### MVP First (User Story 1만)

1. Phase 1: Setup 완료
2. Phase 2: Foundational 완료 (모든 스토리를 블록하는 단계이므로 필수)
3. Phase 3: User Story 1 완료
4. **STOP and VALIDATE**: quickstart.md §1~§2(자동 테스트)로 우선 검증, 이후 실제
   Claude Desktop/Code 환경에서 수동 확인(사용자 몫, T013 참조)
5. 필요하면 여기서 데모/배포

### Incremental Delivery

1. Setup + Foundational 완료 → 기반 준비
2. US1 추가 → 독립 검증 → 데모(MVP: 자연어로 경로 질문·답변 가능)
3. US2 추가(테스트만) → 키 문제 상황이 실제로 잘 구분되는지 확인
4. US3 추가(테스트만) → 예외 상황에서도 서버가 죽지 않는지 확인
5. 각 스토리는 이전 스토리를 깨지 않고 신뢰도를 더한다

---

## Notes

- [P] 태스크 = 서로 다른 파일, 의존성 없음
- [Story] 라벨은 태스크를 사용자 스토리로 추적 가능하게 함
- 각 사용자 스토리는 독립적으로 완결/검증 가능해야 함
- 구현 전 테스트가 실패(또는 컴파일 실패)하는지 확인
- 각 태스크 또는 논리적 그룹 완료 후 커밋(단, constitution Principle IV에 따라 실제 커밋은
  사람의 명시적 확인 후에만 생성)
- 체크포인트에서 멈춰 스토리를 독립적으로 검증할 것
- 지양할 것: 모호한 태스크, 동일 파일 충돌, 스토리 간 독립성을 해치는 교차 의존
- US2/US3에 별도 "구현" 태스크가 없는 것은 누락이 아니라, `routeErrorMessage`(T002)가
  이미 모든 에러 분기를 다루도록 설계되어 US1 구현(T006)에서 자연히 함께 완성되기
  때문이다(001/002에서도 반복된 패턴).
