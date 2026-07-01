# Implementation Plan: MCP 경로 검색 서버

**Branch**: `003-mcp-route-server` | **Date**: 2026-07-01 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-mcp-route-server/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

`naeryeo mcp`가 공식 `github.com/modelcontextprotocol/go-sdk`(`mcp` 서브패키지)로 stdio MCP
서버를 띄우고, 단일 도구 `find_transit_route`를 통해 002의 `internal/core.Client.FindRoute`를
노출한다. 툴 핸들러가 평범한 Go `error`를 반환하면 SDK가 자동으로 `IsError`/에러 텍스트를
채워주므로, CLI(`route.go`)가 이미 만든 에러 사유별 한국어 문구 로직을 공용 함수로 추출해
MCP 핸들러와 공유한다(FR-012). `internal/core`/`internal/config`는 변경하지 않는다.

## Technical Context

**Language/Version**: Go 1.26.4 (고정, `go.mod`). SDK가 요구하는 최소 버전(`go 1.25.0`)을
충족한다(research.md §1).

**Primary Dependencies**: `github.com/modelcontextprotocol/go-sdk`(`mcp` 서브패키지, 공식
SDK, `v1.6.1` 확인). 그 외 새 의존성 없음. `internal/core`(002)/`internal/config`(001)를
그대로 재사용.

**Storage**: 해당 없음(상태 없음). 001이 저장한 API 키를 `config.Load()`로 조회하는
소비자 역할만 한다.

**Testing**: `go test -race ./...`(`just test`). SDK가 제공하는
`mcp.NewInMemoryTransports()` + `mcp.NewClient`로 실제 MCP 클라이언트-서버 왕복(JSON-RPC
직렬화 포함)을 재현하는 테이블 테스트(research.md §6). `load`/`findRoute`를 가짜 함수로
주입해 001/002의 실제 백엔드 없이 결정적으로 테스트한다(001/002와 동일한 패턴).

**Target Platform**: 기존과 동일한 크로스플랫폼 CLI(macOS/Windows/Linux). 로컬에서 MCP
클라이언트(Claude Desktop/Code)가 직접 프로세스를 spawn/종료하는 stdio 모델.

**Project Type**: 기존 단일 Go 모듈 레이아웃에 이어서 개발. 새 패키지 없이 `cmd/naeryeo`
안에 파일만 추가한다.

**Performance Goals**: spec SC-001과 동일 — 실제 연결된 두 지점에 대한 요청의 95%가 10초
이내 응답(002의 `Client.FindRoute` 타임아웃을 그대로 상속).

**Constraints**: stdout은 MCP 프로토콜 스트림 전용이며 다른 어떤 출력도 stdout에 써서는
안 된다(research.md §3, FR-011 관련 안정성의 전제 조건). 한 세션 내 여러 호출이 이전 호출의
실패에 영향받지 않아야 한다(FR-009) — 상태 없는 설계로 이미 보장됨(research.md §4).

**Scale/Scope**: 로컬 단일 사용자, 단일 MCP 클라이언트 연결 전제. 새 툴 1개
(`find_transit_route`)만 노출.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Idiomatic Go First** — PASS. 새 인터페이스를 도입하지 않는다 — SDK의 제네릭
  `AddTool`이 요구하는 함수 시그니처를 그대로 따르는 최소 핸들러 함수 하나만 추가한다.
  에러는 SDK 관례(평범한 `error` 반환)를 그대로 따르며 `panic` 없음. CLI/MCP 간 에러
  문구 중복을 피하기 위해 공용 함수로 추출하는 것은 실제 재사용 필요에 의한 것이지
  투기적 추상화가 아니다.
- **II. Unit Tests Are Mandatory** — PASS. `mcp.NewInMemoryTransports()` 기반 종단 간
  테이블 테스트로 성공·`NoTravelNeeded`·5종 에러 분기·연속 호출 안정성을 모두 커버할
  계획(quickstart.md §1).
- **III. Automated Quality Gates** — PASS. `just fmt`/`just lint`/`just test`(`just check`)
  그대로 적용.
- **IV. Commit Discipline** — PASS. 계획 단계에서 커밋 생성하지 않음.

위반 사항 없음 — Complexity Tracking 불필요.

## Project Structure

### Documentation (this feature)

```text
specs/003-mcp-route-server/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   └── mcp-tool.md
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

기존 Go 모듈 레이아웃을 그대로 사용하는 단일 프로젝트다. 새 최상위 디렉터리·새 패키지 모두
만들지 않는다 — `cmd/naeryeo`에 파일만 추가한다.

```text
cmd/naeryeo/
├── main.go             # 기존 라우팅에서 "mcp" 분기를 실제 구현으로 교체
├── route.go             # 기존 파일 — 에러 문구 변환 로직을 공용 함수로 추출(리팩터링)
├── mcp.go                # 새 파일: RouteToolInput/Output, buildMCPServer, runMCP, 툴 핸들러
└── mcp_test.go           # 새 파일: 인메모리 트랜스포트 기반 종단 간 테스트
```

**Structure Decision**: 001/002와 동일한 관례 — `cmd/`+`internal/` Go 표준 레이아웃 유지,
테스트는 같은 디렉터리에 `_test.go`로. `internal/core`/`internal/config`는 변경하지 않으며,
`cmd/naeryeo/route.go`만 에러 문구 로직 추출을 위해 리팩터링한다(기존 동작/테스트는
그대로 유지되어야 함).

## Complexity Tracking

*No violations — table intentionally omitted per template guidance ("Fill ONLY if Constitution Check has violations").*
