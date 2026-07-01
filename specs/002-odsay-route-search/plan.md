# Implementation Plan: 대중교통 경로 검색 (ODsay 연동 코어 로직)

**Branch**: `002-odsay-route-search` | **Date**: 2026-07-01 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-odsay-route-search/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

`internal/core`가 ODsay REST API를 감싸 이름 기반(역/정류장/주소) 출발지·도착지를 대표
대중교통 경로(총 소요시간, 환승 횟수, 요금, 단계별 안내)로 변환하는 `Client.FindRoute`를
제공한다. 내부적으로 이름→좌표 변환(`searchStation`)과 좌표 기반 경로 검색
(`searchPubTransPathT`) 두 단계를 조합한다. API 키는 `internal/config`가 아니라 호출자가
문자열로 주입하며, 빈 키·미인식 지점·경로 없음·업스트림 장애를 서로 다른 에러 타입으로
구분해 `route`/`mcp` 두 진입점이 동일한 판단 로직을 공유하도록 한다(FR-013).

## Technical Context

**Language/Version**: Go 1.26.4 (고정, `go.mod`)

**Primary Dependencies**: 표준 라이브러리만 사용(`net/http`, `encoding/json`, `context`).
ODsay는 공식 Go SDK가 없는 단순 REST+JSON API이며(research.md §6), 새 의존성을 정당화할
근거가 없다. `001-keychain-api-key`의 `internal/config`는 이 패키지가 직접 import하지
않는다(research.md §4) — 호출자가 `config.Load()` 결과를 문자열로 넘긴다.

**Storage**: 해당 없음(상태 없는 외부 API 호출). ODsay 자체가 유일한 외부 데이터 소스.

**Testing**: `go test -race ./...`(`just test`). `net/http/httptest.Server`로 ODsay
엔드포인트(`searchStation`, `searchPubTransPathT`)를 흉내 내어 이름→좌표→경로 전 과정을
테이블 기반으로 검증(research.md §7). 실제 `api.odsay.com`에 대한 통합 테스트는 CI에
포함하지 않는다.

**Target Platform**: 기존과 동일한 크로스플랫폼 CLI(macOS/Windows/Linux). ODsay 자체가
대한민국 국내 대중교통만 다룬다(spec Assumptions).

**Project Type**: 기존 단일 Go 모듈 레이아웃(`cmd/naeryeo` + `internal/*`)에 이어서 개발.

**Performance Goals**: spec SC-001 — 실제 연결된 두 지점에 대한 요청의 95%가 10초 이내 응답
(ODsay가 정상 응답하는 경우 기준). 이를 위해 기본 HTTP 클라이언트에 10초 타임아웃을 두고,
호출자가 `context.Context`로 더 짧은 데드라인을 강제할 수 있게 한다(research.md §5).

**Constraints**: 무한 대기 금지(FR-011); 두 진입점(`route`/`mcp`) 간 판단 로직 일관성
(FR-013, SC-005); ODsay 무료 요청 한도의 정확한 수치는 공식 문서에서 확인되지 않아 특정
한도를 가정하지 않는다(research.md §3 미확인 사항).

**Scale/Scope**: 단일 사용자 CLI 도구, 요청 1건씩 순차 처리. 동시성 요구사항 없음.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Idiomatic Go First** — PASS. `Client`는 구현체가 ODsay 하나뿐이므로 인터페이스를
  두지 않고 구체 타입 + 교체 가능한 `BaseURL`/`HTTPClient` 필드로 테스트 가능성을 확보한다
  (투기적 인터페이스 추상화 없음). 에러는 sentinel + 커스텀 타입(`ErrPointNotFound`,
  `ErrUpstreamRejected`)으로 명시적으로 표현하며 `panic` 없음.
- **II. Unit Tests Are Mandatory** — PASS. `httptest.Server` 기반 테이블 테스트로 성공
  경로·`NoTravelNeeded`·5종 에러 분기·컨텍스트 타임아웃을 모두 커버할 계획(quickstart.md §1).
  `cmd/naeryeo`의 `route` 배선도 같은 커밋에서 테스트 추가.
- **III. Automated Quality Gates** — PASS. `just fmt`/`just lint`/`just test`(`just check`)
  그대로 적용.
- **IV. Commit Discipline** — PASS. 계획 단계에서 커밋 생성하지 않음.

위반 사항 없음 — Complexity Tracking 불필요.

**주의(재확인 필요)**: research.md §3에서 밝힌 대로, ODsay의 "API 키 인증 실패" 에러 코드는
공개 문서에서 확인되지 않았다. Phase 1 설계는 이를 감안해 `ErrUpstreamRejected`라는 포괄
경로를 두었으므로 구현이 막히지는 않지만, FR-008(인증 실패를 키 미설정과 구분)의 완전한
구현은 실제 발급 키로 검증하는 태스크가 `/speckit-tasks`에 포함되어야 한다.

## Project Structure

### Documentation (this feature)

```text
specs/002-odsay-route-search/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   └── core-package.md
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

기존 Go 모듈 레이아웃을 그대로 사용하는 단일 프로젝트다. 새 최상위 디렉터리는 만들지 않는다.

```text
internal/core/
├── doc.go            # 기존 placeholder 패키지 doc (내용 갱신)
├── errors.go          # ErrAPIKeyMissing/ErrAuthFailed/ErrNoRoute/ErrUpstreamUnavailable
│                       # sentinel + ErrPointNotFound/ErrUpstreamRejected 커스텀 타입
├── client.go           # Client, NewClient, FindRoute, ODsay HTTP 호출 + JSON→도메인 매핑
└── client_test.go      # httptest 기반 테이블 테스트 (성공/NoTravelNeeded/에러 5종/타임아웃)

cmd/naeryeo/
├── main.go            # 기존 라우팅에서 "route" 분기를 실제 구현으로 교체
├── route.go            # route 서브커맨드: --from/--to 파싱 → config.Load → core.FindRoute → 출력
└── route_test.go
```

**Structure Decision**: `001-keychain-api-key`와 동일한 관례를 따른다 — 별도 `src/`/`tests/`
최상위 디렉터리 없이 `cmd/`+`internal/` Go 표준 레이아웃을 유지하고, 테스트는 각 패키지와
같은 디렉터리에 `_test.go`로 둔다.

## Complexity Tracking

*No violations — table intentionally omitted per template guidance ("Fill ONLY if Constitution Check has violations").*
