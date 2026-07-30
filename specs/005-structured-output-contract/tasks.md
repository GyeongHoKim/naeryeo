---

description: "Task list for 005-structured-output-contract"
---

# Tasks: 구조화된 출력 계약 (`--json` + 에러 코드)

**Input**: Design documents from `/specs/005-structured-output-contract/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: 헌법 원칙 II(NON-NEGOTIABLE)에 따라 **모든 테스트 태스크는 필수**다. 신규 심볼은
이를 도입하는 커밋에 테스트를 동반해야 한다.

**Organization**: 태스크는 사용자 스토리별로 묶여 독립 구현·검증이 가능하다.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 병렬 실행 가능 (다른 파일, 미완료 의존 없음)
- **[Story]**: US1 / US2 / US3
- 모든 태스크에 정확한 파일 경로 포함

## Path Conventions

단일 Go 프로젝트. 저장소 루트 기준 `cmd/naeryeo/`, `internal/`, `skills/`.
plan.md의 Source Code 구조를 따른다.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 기준선 확보. 이 프로젝트는 툴체인(`justfile`, `.golangci.yml`,
`commitlint.config.js`)이 이미 갖춰져 있어 신규 설정 작업이 없다.

- [X] T001 `git rev-parse --abbrev-ref HEAD`로 `feature/005-structured-output-contract` 브랜치임을 확인하고 `just check`를 실행해 기준선이 green인지 확인 — 실패하는 게이트가 있으면 이번 변경에 착수하기 전에 그 사실을 사람에게 보고한다 (이후 회귀를 이번 변경에 귀속시키기 위함)

**Checkpoint**: 기준선 green 확인됨

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 세 스토리가 공유하는 분류 기반과 봉투 타입. 이 단계가 끝나면 **어떤 경로에서도
원본 오류가 노출되지 않는다**(`internal_error` 안전 기본값) — FR-005가 여기서 확보된다.

**⚠️ CRITICAL**: 이 단계 완료 전에는 어떤 사용자 스토리도 시작할 수 없다

- [X] T002 [P] `cmd/naeryeo/errcode.go` 신규 생성 — `errorCode` 타입과 상수 정의 ([contracts/error-codes.md](./contracts/error-codes.md) 표 기준), `failure` 구조체(`Code`/`Message`/`Hint`/`Side`/`Name`) 선언
  > **구현 중 조정**: 14종을 한 번에 선언하니 `golangci-lint`의 `unused`가 소비자 없는 상수 4종(`upstream_unavailable`, `upstream_rejected`, `credential_store_error`, `invalid_arguments`)을 잡아 Phase 2 게이트가 실패했다. 헌법 원칙 I(추측성 선언 금지)과 "Phase 경계에서 커밋 가능" 규칙에 맞춰, 각 상수를 **처음 사용하는 Phase에서 선언**하도록 옮겼다 — `invalid_arguments`는 T017, 나머지 3종은 T029·T030. 정본 표는 contracts/error-codes.md에 그대로 유지된다.
- [X] T003 [P] `cmd/naeryeo/mcp.go`에 `RouteError` 타입(`code`/`message`/`hint`/`side`/`name`, 모두 `omitempty` 단 code·message 필수) 추가하고 `RouteToolOutput`에 `Error *RouteError \`json:"error,omitempty"\`` 필드 추가 ([data-model.md](./data-model.md) §3)
- [X] T004 [P] `cmd/naeryeo/errcode_test.go` 신규 생성 — `classifyRouteError` 테이블 테스트를 **기존 10개 코드의 문구 패리티** 기준으로 작성 (구현 전이므로 실패해야 함)
- [X] T005 `cmd/naeryeo/errcode.go`에 `classifyRouteError(err error, geocoderConfigured bool) failure` 구현 — `errors.Is/As` 분기, `default:`는 `internal_error` 코드와 고정 문구 반환(**원본 에러 텍스트를 절대 포함하지 않음**)
- [X] T006 `cmd/naeryeo/errcode.go`에 `failure.Prose() string`(= `Message` + `"\n"` + `Hint`, Hint 없으면 Message만)과 `failure.toRouteError() *RouteError` 추가
- [X] T006a `cmd/naeryeo/errcode_test.go`에 `failure.Prose()`(Hint 있음/없음 두 경우)와 `failure.toRouteError()`(모든 필드 매핑 + `Hint`/`Side`/`Name` 빈 값이 `omitempty`로 사라지는지)의 단위 테스트 추가 — **헌법 원칙 II는 "동일 커밋에 테스트 동반"을 요구하므로, T003·T006이 도입한 심볼의 테스트가 Phase 3으로 밀리면 안 된다**
- [X] T007 `cmd/naeryeo/route.go`의 `routeErrorMessage`를 `classifyRouteError(...).Prose()`에 위임하는 얇은 래퍼로 재구성 — 기존 시그니처와 출력 문구를 그대로 유지
- [X] T008 `just test` 실행 — `cmd/naeryeo/route_test.go`, `cmd/naeryeo/mcp_test.go`의 기존 프로즈 테스트가 **수정 없이** 전부 통과하는지 확인 (SC-007 회귀 방지)

**Checkpoint**: 분류 기반 완성, 프로즈 출력 불변, 원본 오류 누출 경로 차단됨 — 스토리 구현 시작 가능.
이 시점에서 커밋해도 **Phase 2가 도입한 모든 심볼에 테스트가 동반**되어 헌법 원칙 II를 만족한다.

---

## Phase 3: User Story 1 - AI가 실패 원인을 코드로 구분해 다음 행동을 결정한다 (Priority: P1) 🎯 MVP

**Goal**: 실패 응답에 안정적인 코드를 실어, AI가 한국어 문장을 해석하지 않고도
"재시도 / 입력 재작성 / 키 재등록 / 콘솔 설정 확인"을 판별할 수 있게 한다. CLI(`--json`)와
MCP 두 진입점 모두에 적용한다.

**Independent Test**: 각 실패 상황을 유발한 뒤 `message` 문자열을 전혀 읽지 않고
`error.code`만으로 올바른 후속 행동을 고를 수 있는지 확인. 특히
`geocoder_rate_limited`(재시도)와 `geocoder_rejected`(입력 재작성)가 구분되어야 한다.

### Tests for User Story 1 (MANDATORY per Constitution Principle II) ⚠️

> 구현 전에 작성하고 **실패하는 것을 확인**할 것

- [X] T009 [P] [US1] `cmd/naeryeo/errcode_test.go`에 `*core.ErrGeocoderRejected`가 `RateLimited()` 여부로 `geocoder_rate_limited` / `geocoder_rejected`로 갈리는지 테이블 테스트 추가 (HTTP 429, Kakao code `-10`, 그 외 400 케이스)
- [X] T010 [P] [US1] `cmd/naeryeo/errcode_test.go`에 `core.ErrGeocoderAuthFailed` → `geocoder_auth_failed`, `core.ErrGeocoderForbidden` → `geocoder_forbidden`이 **서로 다른 코드**임을 검증하는 테스트 추가
- [X] T011 [P] [US1] `cmd/naeryeo/errcode_test.go`에 `*core.ErrPointNotFound` → `point_not_found` + `Side`/`Name` 전달 + 지오코더 미설정 시에만 `Hint`가 채워지는지 테스트 추가
- [X] T012 [P] [US1] `cmd/naeryeo/routejson_test.go`(신규)에 `--json` 실패 시 **stdout**에 파싱 가능한 문서 하나가 나오고 stderr가 비며 exit 1인지 검증하는 테스트 추가 ([contracts/cli-json.md](./contracts/cli-json.md) 매트릭스)
- [X] T013 [P] [US1] `cmd/naeryeo/routejson_test.go`에 `--from`/`--to` 누락과 알 수 없는 플래그가 `--json` 모드에서 `invalid_arguments` 문서로 나오고 stdout에 사용법 텍스트가 섞이지 않는지 테스트 추가 (FR-015)
- [X] T014 [P] [US1] `cmd/naeryeo/mcpjson_test.go`(신규)에 in-memory transport 종단 테스트 추가 — 실패 시 `IsError == true`이고 `StructuredContent`에 `error.code`가 존재하며 `Content[0].Text`가 프로즈인지 검증 ([contracts/mcp-tool.md](./contracts/mcp-tool.md))
- [X] T015 [P] [US1] CLI와 MCP가 같은 실패에 **같은 `code`와 같은 문구**를 내는지 비교하는 테스트 (FR-016)
  > **구현 중 조정**: 기존 `mcp_test.go`의 `TestFindTransitRouteTool_GeocoderMessagesMatchCLI`를 확장하는 대신, `mcpjson_test.go`에 `TestFindTransitRouteTool_FailureMatchesCLICodeAndMessage`를 신설했다. 기존 테스트는 프로즈 문구 회귀를 잡는 역할이 있어 무수정으로 통과시키는 편이 SC-007 검증에 유리하다.

### Implementation for User Story 1

- [X] T016 [US1] `cmd/naeryeo/main.go`에 `hasJSONFlag(args []string) bool` 추가 — 기존 `hasDebugFlag`와 같은 선스캔 패턴 ([research.md](./research.md) §R4)
- [X] T017 [US1] `cmd/naeryeo/route.go`의 `runRoute`에 `--json` 플래그 등록, JSON 모드일 때 `fs.SetOutput(io.Discard)`로 FlagSet 사용법 출력 억제, 플래그 파싱 실패·필수 인자 누락을 `invalid_arguments` 문서로 stdout 출력 후 exit 1
- [X] T018 [US1] `cmd/naeryeo/route.go`에 실패 시 JSON 분기 추가 — `classifyRouteError(...).toRouteError()`를 `RouteToolOutput{Error: ...}`에 담아 **stdout**에 직렬화, exit 1 (프로즈 분기는 기존 `reportRouteError` 그대로)
- [X] T019 [US1] `cmd/naeryeo/route.go`에서 `--json`과 `--debug`가 함께 지정된 경우 원본 에러 체인을 **stderr로만** 출력해 stdout 문서의 파싱 가능성을 보전 (FR-014, [research.md](./research.md) §R5)
- [X] T020 [US1] `cmd/naeryeo/mcp.go`의 `routeToolHandler` 실패 경로를 전환 — **error를 반환하지 않고**(`err = nil`) `&mcp.CallToolResult{IsError: true, Content: [TextContent{f.Prose()}]}`와 `RouteToolOutput{Error: f.toRouteError()}`를 반환. go-sdk v1.6.1 래퍼가 error 반환 시 결과를 폐기하므로 이 방식이 **필수**다 ([contracts/mcp-tool.md](./contracts/mcp-tool.md) 구현 계약)

**Checkpoint**: 실패 경로에서 CLI `--json`과 MCP 모두 안정적인 코드를 반환한다. `--json` 성공
경로는 아직 프로즈로 남아 있다 (US2에서 완성).

---

## Phase 4: User Story 2 - AI가 성공 결과를 파싱 없이 소비한다 (Priority: P2)

**Goal**: `--json` 성공 출력을 구조화하고, CLI와 MCP의 성공 문서가 **같은 Go 타입**에서
파생되어 스키마가 갈라질 수 없게 한다. US1과 합쳐져 `--json` 출력 모드가 완성된다.

**Independent Test**: 성공하는 경로 검색을 `--json`으로 실행해
`totalTimeMinutes`/`transferCount`/`fareWon`/`steps`가 담긴 문서 하나가 stdout에 나오는지,
그리고 그 구조가 MCP 도구의 `structuredContent`와 필드 단위로 일치하는지 확인.

### Tests for User Story 2 (MANDATORY per Constitution Principle II) ⚠️

- [X] T021 [P] [US2] `cmd/naeryeo/routejson_test.go`에 `--json` 성공 시 stdout에 성공 문서 하나 + exit 0 + `error` 키 부재를 검증하는 테스트 추가
- [X] T022 [P] [US2] `cmd/naeryeo/routejson_test.go`에 `NoTravelNeeded` 결과가 `{"noTravelNeeded": true}`로 직렬화되고 소요시간 0이 경로 실패로 오인되지 않는지 테스트 추가
- [X] T023 [P] [US2] `cmd/naeryeo/routejson_test.go`에 동일한 `core.RouteResult`로부터 CLI `--json` 출력과 MCP `StructuredContent`가 **동일한 JSON 문서**를 내는지 검증하는 테스트 추가 (FR-010, SC-005)
  > **구현 중 조정**: "바이트 동일"로 처음 작성했더니 실패했다. MCP `StructuredContent`는 클라이언트에서 map으로 디코드되어 재직렬화 시 키가 알파벳순으로 정렬되는 반면 CLI는 구조체 필드 순서를 유지한다. 키 순서는 계약이 아니고 키 집합과 값이 계약이므로, 디코드 후 `reflect.DeepEqual`로 비교하도록 고쳤다.

### Implementation for User Story 2

- [X] T024 [US2] `cmd/naeryeo/route.go`에 성공 시 JSON 분기 추가 — 기존 `toRouteToolOutput(result)`를 재사용해 봉투를 stdout에 직렬화하고 exit 0 (프로즈 분기는 `printRouteResult` 그대로 유지)
- [X] T025 [US2] `cmd/naeryeo/mcp.go`의 `toRouteToolOutput`이 성공 시 `Error`를 `nil`로 남기도록 하고, `route.go`의 JSON 성공 분기(T024)가 **이 함수를 그대로 호출**하도록 배선 — 두 진입점이 같은 변환 함수를 공유해 성공 문서가 갈라질 수 없게 만든다. 함수 주석에 CLI·MCP 공유임을 명시

**Checkpoint**: `--json` 출력 모드 완성. 성공·실패 모두 stdout의 단일 문서로 소비 가능.

---

## Phase 5: User Story 3 - 내부·업스트림 오류 원문이 어디로도 새지 않는다 (Priority: P3)

**Goal**: 현재 `default:` 분기로 원문이 노출되던 경로에 구체적인 코드를 부여하고,
새 에러가 코드 없이 추가되면 품질 게이트가 실패하게 만든다.

**Independent Test**: 감싸진(wrapped) 원인을 가진 에러를 주입해 프로즈·JSON·MCP 어느
출력에도 원문 문자열이 나타나지 않는지 확인. 그리고 `internal/core`에 임시 에러를 추가해
망라성 테스트가 실제로 실패하는지 확인 ([quickstart.md](./quickstart.md) §2).

### Tests for User Story 3 (MANDATORY per Constitution Principle II) ⚠️

- [X] T026 [P] [US3] `cmd/naeryeo/errcode_test.go`에 **wrapped 에러** (`fmt.Errorf("%w: internal db timeout at shard 7 (trace 0xdeadbeef)", &core.ErrUpstreamRejected{...})`)를 넘겨도 `Message`/`Hint`에 원문 조각이 나타나지 않는지 검증하는 테스트 추가 (FR-005, SC-003)
- [X] T027 [P] [US3] `cmd/naeryeo/errcode_test.go`에 키체인 조회 실패(`config.ErrNotConfigured`가 **아닌** 에러)가 `credential_store_error`로 분류되고 저장소 원본 문자열이 새지 않는지 테스트 추가
- [X] T028 [P] [US3] `cmd/naeryeo/errcode_exhaustive_test.go`(신규)에 `go/parser`+`go/ast`로 `internal/core/*.go`의 exported 에러 심볼(`var ErrXxx = errors.New(...)`, 포인터 리시버 `Error()`를 갖는 `type ErrXxx struct`)을 수집해, 테스트 로컬 대조표에 항목이 있고 각 샘플 값이 `internal_error`가 **아닌** 코드로 분류되는지 검사하는 게이트 추가. `core.ErrGeocoderNotFound`는 "표현 계층 미도달" 사유 주석과 함께 허용 목록에 등록 ([research.md](./research.md) §R3)

### Implementation for User Story 3

- [X] T029 [US3] `cmd/naeryeo/errcode.go`의 `classifyRouteError`에 `core.ErrUpstreamUnavailable` → `upstream_unavailable`, `*core.ErrUpstreamRejected` → `upstream_rejected` 분기 추가 — ODsay 원본 `Code`/`Message`는 문구에 포함하지 않음
- [X] T030 [US3] `cmd/naeryeo/route.go:47`의 `"API 키 조회 실패: %v"`와 `cmd/naeryeo/mcp.go:83`의 `fmt.Errorf("API 키 조회 실패: %w", loadErr)`를 `credential_store_error` 분류 경로로 교체 — 두 진입점이 같은 코드·문구를 내도록
- [X] T031 [US3] `cmd/naeryeo/route_test.go:337-341`의 `"upstream unavailable"` 케이스 기대값 수정 — 현재 bare sentinel(`core.ErrUpstreamUnavailable`)을 넘기고 `want: "오류가 발생했습니다"`(default 분기 문구)를 기대하고 있다. **wrapped 에러**를 넘겨 원본 체인이 실제로 새지 않는지 검증하도록 변경하고, 기대 문구를 `upstream_unavailable` 코드의 문구로 교체. 같은 파일 `deadUpstreamFindRoute`(179-213) 경로도 새 문구에 맞는지 함께 확인

**Checkpoint**: 누출 경로 4건 차단 완료. 새 에러 추가 시 게이트가 자동으로 막는다.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 문서 동기화와 최종 검증. AI용 스킬 문서가 갱신되어야 이 기능이 실제 가치를 낸다.

- [X] T032 `skills/naeryeo/SKILL.md`의 `## Usage` §Option A(현재 87-108행)를 갱신 — `--json`을 1순위 호출 형태로 제시하고, 예시 출력을 성공 JSON 문서로 교체, "`error` 키 유무로 성공/실패 판별" 규칙 명시 (FR-020)
- [X] T033 `skills/naeryeo/SKILL.md`의 `## Handling errors`(현재 129-143행)를 [contracts/error-codes.md](./contracts/error-codes.md) 표 기반으로 재작성 — `geocoder_rate_limited` ↔ `geocoder_rejected`, `geocoder_auth_failed` ↔ `geocoder_forbidden` 구분 명시, "`message` 문자열 매칭 금지", "모르는 코드는 재시도하지 말 것" 안내 포함 (FR-019)
- [X] T034 `skills/naeryeo/SKILL.md`의 `## Common Mistakes`(현재 145-155행)에서 프로즈 에러 문구를 인용한 부분을 코드 기준으로 정정 (FR-021)
- [X] T034a [P] `README.md`의 명령어 표(174행 부근 `naeryeo route --from <출발지> --to <도착지>`)에 `--json` 플래그 설명 한 줄 추가 — 사용자 대면 플래그이므로 문서화한다. spec이 요구한 항목은 아니며(Out of Scope에도 없음), 기존 `--debug`가 README에 없는 선례가 있으나 `--json`은 AI 소비 경로의 1순위 호출 형태이므로 예외로 둔다
- [X] T035 [P] `skills/naeryeo/SKILL.md`가 나열하는 코드 집합과 [contracts/error-codes.md](./contracts/error-codes.md)의 14종이 일치하는지 대조 — 문서에만 또는 코드에만 있는 항목 0건 (SC-008)
- [X] T036 [quickstart.md](./quickstart.md) §1–§10을 순서대로 수행해 전 항목 통과 확인 — 특히 §2(게이트가 실제로 실패했다가 복구)와 §7(`--debug` 조합에서 API 키 미노출, GYE-293 회귀 확인)
- [X] T037 `just check` 실행 — fmt / lint / test 전부 green (헌법 원칙 III)
- [X] T038 변경 diff와 Conventional Commits 형식 커밋 메시지를 사람에게 제시하고 **명시적 확인을 받은 뒤에만** 커밋 (헌법 원칙 IV — 에이전트 임의 커밋 금지)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 의존 없음 — 즉시 시작
- **Foundational (Phase 2)**: Setup 완료 후 — **모든 사용자 스토리를 블로킹**
- **US1 (Phase 3)**: Foundational 완료 후 시작
- **US2 (Phase 4)**: Foundational 완료 후 시작. US1과 **같은 파일**(`route.go`, `mcp.go`)을 수정하므로 순차 진행 권장
- **US3 (Phase 5)**: Foundational 완료 후 시작. `errcode.go` 분류 함수를 US1과 공유
- **Polish (Phase 6)**: 원하는 스토리가 모두 완료된 후

### User Story Dependencies

- **US1 (P1)**: Foundational 이후 독립 시작 가능. 다른 스토리에 의존하지 않음
- **US2 (P2)**: 논리적으로 독립이지만 **US1과 하나의 `--json` 출력 모드를 이룬다**.
  US1만 출하하면 `--json` 성공 경로가 프로즈로 남는 불완전 상태가 되므로,
  **MVP는 US1 + US2**로 잡는 것을 권장한다 (spec의 US2 "Why this priority" 참조)
- **US3 (P3)**: 독립 시작 가능. Foundational의 `internal_error` 안전 기본값이 이미
  FR-005를 확보하므로, US3는 그 위에 **구체적 코드와 게이트**를 얹는다

### Within Each User Story

- 테스트를 먼저 작성하고 **실패를 확인**한 뒤 구현 (헌법 원칙 II)
- 타입 정의 → 분류 로직 → 출력 배선 순
- 스토리 완료 후 다음 우선순위로 이동

### Parallel Opportunities

- **Phase 2**: T002(`errcode.go`), T003(`mcp.go`), T004(`errcode_test.go`)는 서로 다른 파일 — 병렬 가능. T006a는 T003·T006이 도입한 심볼을 검증하므로 그 뒤에 온다
- **Phase 3**: T009–T011(`errcode_test.go`)은 같은 파일이나 서로 다른 테스트 함수라 충돌 없이 병렬 작성 가능. T012–T013(`route_test.go`), T014–T015(`mcp_test.go`)도 마찬가지
- **Phase 5**: T026–T028 모두 `errcode_test.go`의 독립 함수 — 병렬 가능
- **주의**: T017–T019는 모두 `cmd/naeryeo/route.go`의 `runRoute`를 수정하므로 **병렬 불가**
- **주의**: T032–T034는 모두 `SKILL.md`를 수정하므로 **병렬 불가**. T034a는 `README.md`라 이들과 병렬 가능

---

## Parallel Example: Phase 2 (Foundational)

```bash
# 서로 다른 파일이므로 동시 진행 가능:
Task: "cmd/naeryeo/errcode.go 신규 생성 — ErrorCode 상수 14종 + failure 구조체"
Task: "cmd/naeryeo/mcp.go에 RouteError 타입 + RouteToolOutput.Error 필드 추가"
Task: "cmd/naeryeo/errcode_test.go 신규 생성 — 기존 10개 코드 문구 패리티 테이블 테스트"
```

## Parallel Example: User Story 1 tests

```bash
# 같은 파일이라도 독립 테스트 함수이므로 충돌 없이 작성 가능:
Task: "errcode_test.go — rate_limited vs rejected 분기 테스트"
Task: "errcode_test.go — auth_failed vs forbidden 구분 테스트"
Task: "errcode_test.go — point_not_found의 side/name/hint 테스트"
Task: "route_test.go — --json 실패 문서 stdout + exit 1 테스트"
Task: "mcp_test.go — structuredContent.error.code 종단 테스트"
```

---

## Implementation Strategy

### MVP (US1 + US2)

1. Phase 1 Setup — 기준선 green 확인
2. Phase 2 Foundational — 분류 기반 + 봉투 타입 (**블로킹**)
3. Phase 3 US1 — 실패 코드 체계 + `--json` 실패 + MCP 구조화 실패
4. Phase 4 US2 — `--json` 성공 문서
5. **정지 후 검증**: [quickstart.md](./quickstart.md) §3–§7 수행
6. 이 시점에서 AI 호출자는 성공·실패 모두 기계 판독으로 소비 가능

> US1 단독을 MVP로 잡지 않은 이유: `--json`을 선언한 호출자가 성공일 때만 프로즈를 받는
> 상태가 되어 출력 계약이 성립하지 않는다.

### Incremental Delivery

1. Setup + Foundational → 기존 동작 불변, 원본 누출 차단 확보
2. US1 + US2 → `--json` 출력 모드 완성 (MVP)
3. US3 → 누출 경로 구체화 + 망라성 게이트
4. Polish → SKILL.md 동기화 + quickstart 전 항목 검증

각 단계가 이전 단계를 깨지 않는다. Foundational 직후에도 프로즈 출력은 바이트 단위로 불변이다.

### 단독 개발자 기준 순서

`route.go`와 `mcp.go`를 여러 스토리가 함께 수정하므로 병렬 인력 투입 이득이 작다.
Phase 순서대로 진행하고, Phase 내부에서만 [P] 태스크를 묶어 처리하는 것을 권장한다.

---

## Notes

- [P] = 다른 파일 또는 충돌 없는 독립 함수, 미완료 의존 없음
- 구현 전 테스트가 **실패하는 것**을 반드시 확인할 것
- 커밋은 태스크 또는 논리적 묶음 단위로. **사람의 명시적 확인 없이 커밋하지 말 것** (헌법 원칙 IV)
- **커밋 경계는 Phase 경계와 맞추는 것을 권장한다** — 각 Phase가 자신이 도입한 심볼의
  테스트를 함께 담고 있어 헌법 원칙 II("동일 커밋에 테스트 동반")를 만족한다. Phase 중간에서
  끊으면 테스트 없는 심볼이 커밋될 수 있다
- 각 Checkpoint에서 멈춰 스토리를 독립 검증할 수 있다
- `internal/core`와 `internal/config`는 이번 범위에서 **수정하지 않는다** — 에러 값의 소유권은
  core에 남고, "그 에러가 무슨 코드인가"는 표현 계층의 판단이다 ([plan.md](./plan.md) Structure Decision)
