# Implementation Plan: 구조화된 출력 계약 (`--json` + 에러 코드)

**Branch**: `feature/005-structured-output-contract` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-structured-output-contract/spec.md`

## Summary

경로 검색 실패를 **호출자의 후속 행동과 1:1 대응하는 안정적인 에러 코드**로 분류하고, CLI에
`--json` 출력 모드를 추가한다. 분류는 `cmd/naeryeo`의 단일 함수
`classifyRouteError(err, geocoderConfigured) → failure`가 담당하고, 프로즈·CLI JSON·MCP
세 표현이 모두 여기서 파생되어 두 진입점의 코드·문구 일치가 구조적으로 보장된다.

CLI `--json`과 MCP 도구는 **같은 Go 타입**(성공 필드 + 선택적 `error` 객체를 갖는 봉투)을
직렬화하므로 성공 스키마가 갈라질 수 없다. MCP 실패 응답은 핸들러가 error를 반환하지 않고
`IsError`를 직접 세우는 방식으로 `structuredContent`를 싣는다 — go-sdk v1.6.1 래퍼가
error 반환 시 핸들러의 결과를 폐기하기 때문이며, 이는 실측으로 확인했다(research.md §R1).

부수적으로, 현재 `routeErrorMessage`의 `default:` 분기로 **원본 오류가 그대로 노출되던
4개 경로**(`upstream_unavailable`, `upstream_rejected`, `credential_store_error`,
미분류)를 함께 고친다. 새 에러가 코드 없이 추가되면 `go/ast` 기반 테스트가 실패한다.

## Technical Context

**Language/Version**: Go 1.26.4 (기존 모듈 `github.com/GyeongHoKim/naeryeo`, go.mod 기준)

**Primary Dependencies**: 신규 의존 **없음**. 기존 `github.com/modelcontextprotocol/go-sdk`
v1.6.1(MCP), 표준 라이브러리 `encoding/json`(직렬화), `flag`(플래그), `go/parser`+`go/ast`
(**테스트 전용** 망라성 게이트).

**Storage**: N/A — 이 기능은 영속 데이터를 추가하지 않는다.

**Testing**: `go test -race ./...`. 기존 주입 패턴 유지(가짜 `load`/`findRoute` 함수,
MCP in-memory transport). 외부 서비스 호출 없음.

**Target Platform**: 데스크톱 CLI + MCP stdio 서버 (macOS/Windows/Linux).

**Project Type**: Single project — CLI + 내부 라이브러리 패키지. 002/003/004와 동일 구조.

**Performance Goals**: N/A — 출력 형식 변경이며 네트워크 호출 수·응답 시간에 영향 없음.
`geocoderConfigured`는 기존대로 에러 경로에서만 키체인을 조회한다.

**Constraints**:
- 기본(프로즈) 출력의 내용·스트림·종료 코드가 **바이트 단위로 불변** (FR-007, SC-007).
  단 FR-005 해당 케이스는 예외.
- `--json` 시 stdout은 **정확히 하나의 JSON 문서** — 로그·사용법·진단 정보가 섞이면 안 됨.
- 어떤 출력에도 외부 제공자·저장소 **원본 문자열 미노출** (FR-005).

**Scale/Scope**: 에러 코드 14종. 수정 6개 파일(`route.go`, `route_test.go`, `mcp.go`,
`mcp_test.go`, `main.go`, `SKILL.md` — 이 중 테스트 파일 2개) + `README.md` 1줄,
신규 2개 파일(`errcode.go`, `errcode_test.go`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Idiomatic Go First** ✅ — 신규 의존성 없음. 새 패키지를 만들지 않고 소비자가 있는
  `cmd/naeryeo`에 분류 로직을 둔다(R6). 새 타입(`ErrorCode`, `failure`, `RouteError`)은
  각각 "안정 계약 식별자", "세 표현의 단일 원천", "직렬화 투영"이라는 구체 필요로
  정당화된다 — 추측성 일반화 아님. 사용처가 없는 `docs` 필드와 `provider_not_configured`
  코드는 **의도적으로 제외**(spec Assumptions). 에러는 `errors.Is/As`로 명시 처리하고
  `panic` 없음.
- **II. Unit Tests Are Mandatory** ✅ — 신규 exported 심볼(`RouteError`, `RouteToolOutput.Error`)과
  신규 비exported 심볼(`classifyRouteError`, `failure`, `hasJSONFlag`) 모두 동일 커밋에
  테이블 기반 테스트 동반. 해피패스 + 에지(wrapped 에러, `RateLimited()` 분기, 인자 검증
  실패, `--json`/`--debug` 조합) 커버. 망라성 게이트(T-06)가 커버리지 회귀를 구조적으로 막는다.
- **III. Automated Quality Gates** ✅ — `just fmt`/`lint`/`test` 그대로 적용. 새 게이트 불필요
  (망라성 검사는 `just test`에 포함되는 일반 Go 테스트).
- **IV. Commit Discipline** ✅ — Conventional Commits, 변경+테스트 동일 커밋, 인간 확인 후 커밋.
  자동 커밋 훅은 `git-config.yml`에서 전부 `enabled: false`.

**위반 없음** → Complexity Tracking 비움.

### Post-Design 재확인 (Phase 1 이후)

설계 산출물을 확인한 결과 위 판정 유지. 특기 사항:

- 봉투 타입(성공 필드 + `error`)은 R1의 SDK 제약에서 **파생된 필요**이지 선호가 아니다.
  두 타입으로 나누면 MCP 경로에서 `structuredContent`를 실을 방법이 없다.
- `go/ast` 파싱은 테스트 전용이며 프로덕션 바이너리·의존 그래프에 영향이 없다.
  `golang.org/x/tools`를 도입하지 않은 것도 원칙 I에 따른 선택(R3).

## Project Structure

### Documentation (this feature)

```text
specs/005-structured-output-contract/
├── plan.md              # 본 파일
├── research.md          # Phase 0 산출물 (SDK 실측 결과 포함)
├── data-model.md        # Phase 1 산출물
├── quickstart.md        # Phase 1 산출물
├── contracts/           # Phase 1 산출물
│   ├── error-codes.md   # 코드 taxonomy 정본
│   ├── cli-json.md      # CLI --json 문서·스트림·종료 코드
│   ├── mcp-tool.md      # MCP 도구 결과 (SDK 동작 의존 명시)
│   └── skill-md.md      # SKILL.md 갱신 계약
├── checklists/
│   └── requirements.md  # /speckit-specify 산출물
└── tasks.md             # /speckit-tasks 산출물 (본 명령 아님)
```

### Source Code (repository root)

```text
cmd/naeryeo/
├── errcode.go                  # 신규: errorCode 상수, failure 타입,
│                               #       classifyRouteError(), Prose()/toRouteError()
├── errcode_test.go             # 신규: 코드별 테이블 테스트 + 문구 패리티
│                               #       + wrapped 에러 미노출 검증
├── errcode_exhaustive_test.go  # 신규: go/ast 망라성 게이트
├── route.go                    # 변경: --json 플래그, 출력 분기(프로즈/JSON),
│                               #       routeErrorMessage를 classifyRouteError 위로 재구성,
│                               #       인자 검증 실패의 JSON 경로 (FR-015)
├── routejson_test.go           # 신규: --json 성공/실패, 스트림·exit,
│                               #       인자 검증, --debug 조합, CLI↔MCP 성공 동일성
├── route_test.go               # 변경: 337-341 "upstream unavailable" 기대값 수정
│                               #       (bare sentinel → wrapped 에러로 실제 누출 검증),
│                               #       credential_store_error 분류 테스트 추가
├── mcp.go                      # 변경: RouteToolOutput에 Error 필드, RouteError 타입,
│                               #       실패 경로를 err==nil + IsError 방식으로 전환,
│                               #       키체인 에러를 credential_store_error로 분류
├── mcpjson_test.go             # 신규: structuredContent.error.code 종단 검증,
│                               #       CLI와 코드·문구 일치 비교
├── mcp_test.go                 # 변경 없음 (기존 프로즈 테스트가 그대로 통과)
└── main.go                     # 변경: hasJSONFlag 선스캔 (hasDebugFlag와 같은 패턴, R4)

skills/naeryeo/SKILL.md  # 변경: --json을 1순위 호출로, 에러 산문 → 코드 표
                         #       (contracts/skill-md.md)

internal/core/           # 변경 없음 — 에러 정의는 그대로. 분류는 표현 계층 책임
internal/config/         # 변경 없음
```

**Structure Decision**: 기존 단일 프로젝트 구조를 유지한다. 새 패키지를 만들지 않고
`cmd/naeryeo`에 `errcode.go`를 추가하는데, 이는 에러 코드가 표현 계층 계약이고 소비자
(`route.go`, `mcp.go`)가 모두 같은 패키지에 있기 때문이다(헌법 원칙 I — 소비 패키지가 계약을
정의, 소비자 하나짜리 추상 패키지 금지). `internal/core`는 손대지 않는다 — 에러 값의 소유권은
core에 남고, "그 에러가 무슨 코드인가"는 표현 계층의 판단이다.

## 구현 순서 (의존 관계)

`/speckit-tasks`가 상세 분해하겠지만, 아래 순서가 강제된다.

```
1. errcode.go — ErrorCode 상수 + failure + classifyRouteError
   └─ 2. errcode_test.go — 테이블 테스트 + 망라성 게이트
      ├─ 3. route.go — routeErrorMessage를 classify 위로 재구성 (프로즈 불변 유지)
      │     └─ 4. route.go — --json 플래그 + 출력 분기 + main.go 선스캔
      └─ 5. mcp.go — RouteError/봉투 + err==nil 실패 경로
            └─ 6. mcp_test.go — structuredContent 종단 검증 + CLI 일치 확장
                  └─ 7. SKILL.md 갱신
```

3번(프로즈 재구성)을 4번(`--json`)보다 먼저 하는 이유: 기존 프로즈 테스트가 그대로 통과하는지를
먼저 확인해야 FR-007/SC-007 회귀를 조기에 잡는다.

## 위험 요소

| 위험 | 영향 | 완화 |
| --- | --- | --- |
| go-sdk 업그레이드 시 `err==nil` + `IsError` 방식이 깨짐 | MCP 실패에서 `structuredContent` 소실 → 원본 누출 재발 | in-memory transport 종단 테스트로 `structuredContent.error.code` 존재를 검증 (contracts/mcp-tool.md) |
| `go/ast` 게이트가 `Err` 접두 관례에 의존 | 다른 이름의 에러 심볼을 놓침 | 관례는 현재 `internal/core` 전체가 지키고 있음. 놓쳐도 `internal_error` 이중 방어로 FR-005는 유지 (R3) |
| 프로즈 출력이 미묘하게 달라짐 | SC-007 위반, 사용자 스크립트 파손 | 기존 프로즈 테스트를 **수정 없이** 통과시키는 것을 3번 단계의 완료 조건으로 둠 |
| GYE-294·295와 SKILL.md 충돌 | 병합 시 문서 손실 | 먼저 병합되는 쪽 기준 rebase (contracts/skill-md.md) |

## Complexity Tracking

> Constitution Check 위반 없음 — 비움.
