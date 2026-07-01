---

description: "Task list for 대중교통 경로 검색 (ODsay 연동 코어 로직)"
---

# Tasks: 대중교통 경로 검색 (ODsay 연동 코어 로직)

**Input**: Design documents from `/specs/002-odsay-route-search/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/core-package.md, quickstart.md

**Tests**: 예시가 아니라 필수다. 프로젝트 constitution(Principle II: Unit Tests Are
Mandatory)에 따라 모든 사용자 스토리에 테스트 태스크가 REQUIRED다.

**Organization**: 태스크는 spec.md의 사용자 스토리(P1~P2) 단위로 그룹화되어, 각 스토리를
독립적으로 구현·검증할 수 있다.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 병렬 실행 가능(다른 파일, 서로 의존성 없음)
- **[Story]**: 이 태스크가 속한 사용자 스토리(US1~US3)
- 모든 설명에 정확한 파일 경로 포함

## Path Conventions

`001-keychain-api-key`와 동일한 Go 표준 단일 프로젝트 레이아웃:
`internal/core/`, `cmd/naeryeo/`. 새 최상위 디렉터리는 만들지 않는다.

---

## Phase 1: Setup

**Purpose**: 새 의존성 여부 확인 및 베이스라인 확인

- [ ] T001 research.md §6에 따라 이 기능은 표준 라이브러리만 사용하며 새 모듈 의존성이
      필요 없음을 확인한다. 저장소 루트에서 `go build ./...`를 실행해 현재 베이스라인이
      정상 컴파일됨을 확인한다.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 모든 사용자 스토리가 공유하는 HTTP/에러 변환 골격 — 이 단계가 끝나기 전에는
어떤 사용자 스토리도 시작할 수 없다.

**⚠️ CRITICAL**: 아래 태스크 완료 전까지 Phase 3 이후 착수 금지

- [ ] T002 `internal/core/errors.go`를 새로 만들고 sentinel 에러(`ErrAPIKeyMissing`,
      `ErrAuthFailed`, `ErrNoRoute`, `ErrUpstreamUnavailable`)와 커스텀 타입
      (`ErrPointNotFound{Side, Name string}`, `ErrUpstreamRejected{Code, Message string}`,
      각각 `Error() string` 구현)을 정의한다. (data-model.md, contracts/core-package.md 참조)
- [ ] T003 `internal/core/client.go`를 새로 만들고 `RouteQuery{From, To string}`,
      `RouteResult{NoTravelNeeded bool; TotalTime, TransferCount, Fare int; Steps []RouteStep}`,
      `RouteStep{Description string}` 타입과 `Client{APIKey string; HTTPClient *http.Client;
      BaseURL string}` 구조체, `NewClient(apiKey string) *Client`(기본
      `BaseURL="https://api.odsay.com/v1/api"`, `HTTPClient`가 nil이면 10초 타임아웃의
      기본 클라이언트 사용)를 정의한다. (data-model.md 참조)
- [ ] T004 [P] `internal/core/doc.go`의 placeholder 문구를 제거하고 실제 `Client`/
      `FindRoute` 표면을 설명하도록 패키지 doc 주석을 갱신한다.
- [ ] T005 `internal/core/client.go`에 ODsay 응답을 담을 비공개 JSON 구조체
      (`searchStation` 결과 목록, `searchPubTransPathT`의 `result.path[].info`/`subPath[]`
      필드, research.md §2 참조)와 공용 HTTP 헬퍼 `doGet(ctx context.Context, rawURL string,
      out any) error`를 구현한다: 네트워크 오류·타임아웃·비-2xx(401/403 제외)·JSON 디코드
      실패는 `ErrUpstreamUnavailable`로, HTTP 401/403은 `ErrAuthFailed`로 매핑한다
      (research.md §3 미확인 사항 — 실제 키로 추후 재검증 필요함을 코드 주석에 명시).
      (depends on T002, T003)
- [ ] T006 `internal/core/client.go`에 ODsay 응답 바디의 에러 객체를 도메인 에러로 변환하는
      `classifyODsayError` 헬퍼를 구현한다: 코드 `3`→`&ErrPointNotFound{Side:"from"}`,
      `4`→`{Side:"to"}`, `5`→`{Side:"both"}`, `6`/`-99`→`ErrNoRoute`, `-98`→성공 경로에서
      `NoTravelNeeded` 판정에 쓰이는 특수 마커(에러 아님), `-8`/`-9`/그 외 미분류
      →`&ErrUpstreamRejected{Code, Message}`. (research.md §3 코드표 참조, depends on T002, T005)

**Checkpoint**: 공용 HTTP/에러 변환 골격 완성 — 사용자 스토리 구현 가능

---

## Phase 3: User Story 1 - 출발지·도착지로 대중교통 경로 검색 (Priority: P1) 🎯 MVP

**Goal**: 사용자가 `naeryeo route --from <출발지> --to <도착지>`로 실제 ODsay 데이터 기반
대중교통 경로(소요시간·환승 횟수·요금·단계별 안내)를 받는다. 동일 지점 입력 시 에러가 아닌
안내를 받는다.

**Independent Test**: `httptest.Server`로 ODsay 응답을 흉내 내어 `Client.FindRoute`를
직접 호출하고 반환된 `RouteResult`의 각 필드를 검증한다. CLI 레벨에서는 `runRoute`에 가짜
`load`/`findRoute` 함수를 주입해 출력 포맷을 검증한다.

### Tests for User Story 1 (MANDATORY per Constitution Principle II) ⚠️

> **NOTE: 아래 테스트를 먼저 작성하고, 구현 전에는 실패(또는 컴파일 실패)함을 확인한다**

- [ ] T007 [US1] `internal/core/client_test.go`를 새로 만들고 `httptest.Server`로
      `searchStation`+`searchPubTransPathT`를 흉내 내는 테이블 테스트를 추가한다:
      (a) 정상 경로 — `RouteResult`의 `TotalTime`/`TransferCount`(지하철+버스 환승 합)/
      `Fare`/`Steps`가 올바르게 매핑됨, (b) 환승 없는 경로 — `TransferCount == 0`,
      (c) ODsay가 코드 `-98`을 반환 — `RouteResult{NoTravelNeeded: true}, nil`(에러 아님)
      을 검증한다.
- [ ] T008 [P] [US1] `cmd/naeryeo/route_test.go`를 새로 만들고 `runRoute`에 가짜 `load`/
      `findRoute` 함수를 주입해 (a) 정상 결과가 소요시간/환승 횟수/요금/단계별 안내가 포함된
      README 스타일 출력으로 포매팅됨, (b) `NoTravelNeeded` 결과가 "이동이 필요 없습니다"
      류의 안내로 포매팅됨을 검증한다.

### Implementation for User Story 1

- [ ] T009 [US1] `internal/core/client.go`에 이름→좌표 변환
      `resolveStation(ctx context.Context, name string) (stationCoord, error)`를 구현한다
      (`searchStation` 호출 후 첫 후보를 채택; 후보 없음에 대한 에러 매핑은 US3에서 다룸).
      (depends on T005, T006)
- [ ] T010 [US1] `internal/core/client.go`에 `(c *Client) FindRoute(ctx context.Context,
      from, to string) (RouteResult, error)`의 성공 경로를 구현한다: `resolveStation`으로
      두 지점 좌표 확인 → `searchPubTransPathT` 호출(`OPT=0`으로 대표 경로를 `path[0]`으로
      수신, FR-014) → `classifyODsayError`가 `-98` 마커를 반환하면
      `RouteResult{NoTravelNeeded: true}, nil`, 그 외 성공이면 `path[0]`을 `RouteResult`로
      매핑(`Steps`는 `subPath[]`를 순서대로 사람이 읽을 수 있는 문장으로 변환, 예:
      "강남역에서 2호선 승차 → 신도림역에서 하차"). (depends on T009)
- [ ] T011 [P] [US1] 새 파일 `cmd/naeryeo/route.go`에 `runRoute(args []string, stdout,
      stderr io.Writer, load func() (string, error), findRoute func(ctx context.Context,
      apiKey, from, to string) (core.RouteResult, error)) int`를 구현한다:
      `flag.NewFlagSet("route", flag.ContinueOnError)`로 `--from`/`--to` 파싱, `load()`로
      API 키 조회, `findRoute`를 호출해 성공/`NoTravelNeeded` 두 경우를 README 스타일
      (총 소요시간, 환승 횟수, 단계별 안내, 요금)로 출력한다.
- [ ] T012 [US1] `cmd/naeryeo/main.go`의 `"route"` 분기를 `notImplemented(stderr, "route")`
      대신 `runRoute(args[1:], stdout, stderr, config.Load, func(ctx context.Context, apiKey,
      from, to string) (core.RouteResult, error) { return core.NewClient(apiKey).FindRoute(ctx,
      from, to) })` 호출로 교체한다. (depends on T010, T011)

**Checkpoint**: `naeryeo route --from X --to Y`가 실제 ODsay로 단독 동작 (MVP).

---

## Phase 4: User Story 2 - API 키 미설정 시 안내 (Priority: P1)

**Goal**: API 키가 없는 상태에서는 외부 호출 없이 setup 안내를, 저장된 키가 유효하지 않은
경우에는 이와 구분되는 "키가 유효하지 않음" 안내를 받는다.

**Independent Test**: `apiKey`가 빈 문자열일 때 `FindRoute`가 네트워크 호출 없이
`ErrAPIKeyMissing`을 반환하는지, ODsay가 401/403을 반환할 때 `ErrAuthFailed`를 반환하는지
직접 확인한다. CLI 레벨에서는 가짜 `load`/`findRoute`로 두 안내 문구가 서로 다름을 확인한다.

### Tests for User Story 2 (MANDATORY per Constitution Principle II) ⚠️

- [ ] T013 [US2] `internal/core/client_test.go`에 (a) `apiKey == ""`로 `FindRoute` 호출 →
      네트워크 호출이 전혀 발생하지 않고(핸들러 호출 카운터로 검증) `ErrAPIKeyMissing` 반환,
      (b) ODsay가 HTTP 401/403을 반환 → `ErrAuthFailed` 반환을 검증하는 테스트를 추가한다.
- [ ] T014 [P] [US2] `cmd/naeryeo/route_test.go`에 (a) `load`가 `config.ErrNotConfigured`를
      반환 → `findRoute`를 호출하지 않고(스파이 함수로 검증) "naeryeo setup을 먼저
      실행하세요" 안내와 0이 아닌 종료 코드, (b) `findRoute`가 `core.ErrAuthFailed`를 반환
      → "키 미설정"과는 다른 "저장된 API 키가 유효하지 않습니다" 안내와 0이 아닌 종료
      코드를 검증하는 테스트를 추가한다.

### Implementation for User Story 2

- [ ] T015 [US2] `internal/core/client.go`의 `FindRoute` 시작 부분에
      `if c.APIKey == "" { return RouteResult{}, ErrAPIKeyMissing }` 가드를 네트워크 호출
      이전에 추가한다. (depends on T010)
- [ ] T016 [US2] `cmd/naeryeo/route.go`의 `runRoute`에 에러 분기를 추가한다:
      `errors.Is(loadErr, config.ErrNotConfigured)`면 `findRoute` 호출 없이 setup 안내 후
      종료; `findRoute`가 반환한 에러가 `errors.Is(err, core.ErrAuthFailed)`면 "저장된 API
      키가 유효하지 않습니다" 안내로 응답. (depends on T011)

**Checkpoint**: 키 미설정/무효 상태 모두 사용자에게 서로 다른 문구로 안내됨.

---

## Phase 5: User Story 3 - 인식할 수 없는 지점·경로 없음에 대한 명확한 안내 (Priority: P2)

**Goal**: 오타/존재하지 않는 지점, 연결되지 않는 두 지점, 업스트림 장애 각각에 대해 서로
구분되는 명확한 에러 안내를 받는다.

**Independent Test**: ODsay 코드별로 흉내 낸 응답을 `FindRoute`에 주입해 각각 올바른 도메인
에러 타입으로 매핑되는지, 느린 응답에도 무한 대기 없이 에러가 반환되는지 확인한다.

### Tests for User Story 3 (MANDATORY per Constitution Principle II) ⚠️

- [ ] T017 [US3] `internal/core/client_test.go`에 (a) 코드 `3`/`4`/`5` →
      `ErrPointNotFound`의 `Side`가 각각 `"from"`/`"to"`/`"both"`, (b) 코드 `6`/`-99` →
      `ErrNoRoute`, (c) HTTP `500`/연결 거부/손상된 JSON → `ErrUpstreamUnavailable`,
      (d) 코드 `-8`/`-9`/미분류 코드 → `ErrUpstreamRejected`, (e) 짧은 데드라인의
      `context.Context`에 응답이 느린 핸들러(`time.Sleep`) → 무한 대기 없이 데드라인 근처
      에서 에러가 반환됨(테스트 자체에 별도 타임아웃 가드를 두어 행(hang) 여부를 판정)을
      검증하는 테스트를 추가한다.
- [ ] T018 [P] [US3] `cmd/naeryeo/route_test.go`에 `findRoute`가 각각
      `&core.ErrPointNotFound{Side:"from"}`(또는 `"to"`/`"both"`) / `core.ErrNoRoute` /
      `core.ErrUpstreamUnavailable`을 반환할 때 서로 다른 사용자 메시지와 0이 아닌 종료
      코드로 응답하는지 검증하는 테스트를 추가한다.

### Implementation for User Story 3

- [ ] T019 [US3] `internal/core/client.go`의 `resolveStation`이 빈 검색 결과(후보 없음)를
      `&ErrPointNotFound{Side: <호출 시점에 알 수 있는 from/to>}`로 매핑하도록 완성하고,
      `FindRoute`가 `classifyODsayError`(T006)의 결과를 그대로 호출자에게 전파하도록
      배선을 마무리한다. (depends on T006, T009)
- [ ] T020 [US3] `cmd/naeryeo/route.go`의 `runRoute`에 `errors.As(err, &pointErr)`로
      `*core.ErrPointNotFound`를 잡아 `Side`별 안내("출발지"/"도착지"/"출발지와 도착지
      모두"를 찾을 수 없음), `errors.Is(err, core.ErrNoRoute)` 안내("두 지점 사이에 대중교통
      경로가 없습니다"), 그 외(`ErrUpstreamUnavailable`/`ErrUpstreamRejected`)에 대한 일반
      실패 안내 분기를 추가한다. (depends on T016)

**Checkpoint**: 세 사용자 스토리 모두 독립적으로 동작 — 기능 전체 완성.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 스토리 전반에 걸친 마무리 및 미확인 사항 해소

- [ ] T021 [P] 저장소 루트에서 `just fmt`, `just lint`, `just test`(`just check`)를 실행하고
      발견된 문제를 모두 수정한다(constitution Principle III).
- [ ] T022 [P] `cmd/naeryeo/route.go`의 실제 출력 문구를 `README.md`의 `naeryeo route`
      예시(총 소요시간·환승 횟수·단계별 안내·요금 형식)와 비교해 맞춘다.
- [ ] T023 실제 발급받은 ODsay API 키로 `go run ./cmd/naeryeo route --from ... --to ...`를
      수동 실행해 `specs/002-odsay-route-search/quickstart.md` §2의 시나리오(정상 경로,
      동일 지점, 존재하지 않는 지점)를 검증한다. 특히 T005/T015에서 가정한 "HTTP 401/403 →
      인증 실패" 매핑이 실제 ODsay 동작과 일치하는지 확인하고, 다르면 `classifyODsayError`/
      `doGet`을 실제 신호에 맞게 수정한다(research.md §3 미확인 사항 해소).
- [ ] T024 커밋 제안 전, constitution Principle I(idiomatic Go)과 Principle II(테스트
      커버리지)를 기준으로 변경분을 자체 리뷰한다(AGENTS.md Required Workflow 5단계).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 의존성 없음 — 즉시 시작 가능
- **Foundational (Phase 2)**: Setup 완료에 의존 — 모든 사용자 스토리를 블록함
- **User Stories (Phase 3~5)**: 모두 Foundational 완료에 의존
  - US1(P1)이 `FindRoute`의 성공 경로 자체를 구현하므로, US2·US3는 US1이 만든 `FindRoute`
    골격(T010) 위에 에러 분기를 추가하는 구조다 — 완전한 병렬 착수보다는 US1 → US2/US3
    순서가 자연스럽다(README·spec의 우선순위 P1→P1→P2와도 일치)
  - `cmd/naeryeo/route.go`(T011)도 동일하게 US1에서 뼈대가 만들어진 뒤 US2/US3가 에러
    분기를 덧붙인다
- **Polish (Phase 6)**: 구현하기로 한 모든 사용자 스토리 완료에 의존

### User Story Dependencies

- **US1 (P1)**: Foundational 이후 바로 시작 가능. 다른 스토리에 의존하지 않음.
- **US2 (P1)**: `FindRoute`/`runRoute` 골격(T010, T011)이 있어야 에러 분기를 덧붙일 수
  있으므로 US1의 구현 태스크 완료 후 착수. 테스트(T013)는 골격 없이도 작성 가능(TDD로
  먼저 실패시키는 용도).
- **US3 (P2)**: US2와 동일한 이유로 US1 구현 완료 후 착수가 자연스럽다. US2와는 서로
  독립적인 에러 분기라 US2와 US3는 서로 병렬로 진행 가능하다.

### Within Each User Story

- 테스트를 먼저 작성하고 구현 전 실패(또는 컴파일 실패)를 확인한다
- `internal/core`의 로직 구현이 `cmd/naeryeo`의 CLI 배선보다 먼저(또는 병렬로)
- CLI 배선(`main.go` 수정)은 해당 스토리의 나머지 구현이 끝난 뒤 마지막에

### Parallel Opportunities

- Foundational의 T004(doc.go)는 T002/T003/T005/T006(client.go/errors.go)과 다른 파일이라
  병렬 가능
- US1의 T007(client_test.go)과 T008(route_test.go)은 다른 파일이라 병렬 가능; T011
  (route.go)도 T009/T010(client.go)과 다른 파일이라 병렬 가능(단, T012는 둘 다 끝난 뒤)
- US2·US3는 서로 다른 에러 분기를 다루므로 Foundational과 US1 구현 완료 후 두 스토리를
  서로 다른 개발자가 병렬로 진행 가능

---

## Parallel Example: User Story 1

```bash
# US1 테스트를 함께 실행:
Task: "internal/core/client_test.go에 FindRoute 성공/NoTravelNeeded 테스트 추가"
Task: "cmd/naeryeo/route_test.go에 runRoute 출력 포맷 테스트 추가"

# US1 구현을 함께 진행:
Task: "internal/core/client.go에 resolveStation·FindRoute 성공 경로 구현"
Task: "cmd/naeryeo/route.go에 runRoute 구현"
```

---

## Implementation Strategy

### MVP First (User Story 1만)

1. Phase 1: Setup 완료
2. Phase 2: Foundational 완료 (모든 스토리를 블록하는 단계이므로 필수)
3. Phase 3: User Story 1 완료
4. **STOP and VALIDATE**: 실제 ODsay 키로 `naeryeo route --from ... --to ...`가 동작하는지
   수동 확인
5. 필요하면 여기서 데모/배포

### Incremental Delivery

1. Setup + Foundational 완료 → 기반 준비
2. US1 추가 → 독립 검증 → 데모(MVP: 실제 경로 검색 가능)
3. US2 추가 → 독립 검증 → 데모(키 미설정/무효 상태에서도 친절한 안내)
4. US3 추가 → 독립 검증 → 데모(오타/경로없음/업스트림 장애까지 안전하게 처리)
5. 각 스토리는 이전 스토리를 깨지 않고 가치를 더한다

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
- T005/T015의 "HTTP 401/403 → 인증 실패" 매핑은 ODsay 공식 문서에서 확인되지 않은 최선의
  추정이다(research.md §3) — T023에서 실제 키로 반드시 재검증한다.
