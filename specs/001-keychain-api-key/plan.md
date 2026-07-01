# Implementation Plan: API 키 OS 키체인 저장/조회/삭제

**Branch**: `001-keychain-api-key` | **Date**: 2026-07-01 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-keychain-api-key/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

`internal/config` 패키지가 `github.com/zalando/go-keyring`를 감싸 ODsay API 키를 OS 키체인(macOS Keychain / Windows Credential Manager / Linux Secret Service)에 저장·조회·삭제하는 3개 함수(`Save`/`Load`/`Delete`)와 도메인 sentinel 에러를 노출한다. 백엔드 사용 불가 시 평문 파일 폴백 없이 `ErrKeychainUnavailable`로 실패한다. `cmd/naeryeo`의 `setup`/`logout` 서브커맨드가 이 패키지를 소비하며, `route`/`mcp`는 이후 기능에서 `Load`를 재사용한다.

## Technical Context

**Language/Version**: Go 1.26.4 (고정, `go.mod`)

**Primary Dependencies**: `github.com/zalando/go-keyring` (OS 키체인 추상화; context7 문서로 `Set`/`Get`/`Delete`/`ErrNotFound`/`ErrSetDataTooBig`/`ErrUnsupportedPlatform`/`MockInit` 확인). Linux는 D-Bus Secret Service 필요(`libsecret`/GNOME Keyring), 없으면 sentinel이 아닌 일반 에러를 반환하므로 `internal/config`에서 이를 흡수해야 함(research.md §1 참조).

**Storage**: OS 키체인(파일/DB 아님). 엔트리 키: `service="naeryeo"`, `username="odsay-api-key"` 고정 상수.

**Testing**: `go test -race ./...`(`just test`). `keyring.MockInit()`로 정상/미존재 경로를, 별도 테스트 더블(작은 `keyringBackend` 인터페이스)로 "백엔드 사용 불가" 경로를 커버. 테이블 기반 유닛 테스트.

**Target Platform**: 크로스플랫폼 CLI — macOS, Windows, (Secret Service 있는) Linux 데스크톱. Secret Service 없는 headless Linux는 명시적으로 실패하는 것이 정상 동작.

**Project Type**: 단일 Go 모듈 내 CLI + 내부 라이브러리 패키지 (기존 레이아웃 `cmd/naeryeo` + `internal/*` 그대로 사용, 새 프로젝트 구조 아님).

**Performance Goals**: 해당 없음 — 로컬 동기 키체인 호출 1회로, 응답성은 OS 키체인 자체의 지연에 종속(사용자 체감상 즉시).

**Constraints**: 평문 파일에 키를 절대 기록하지 않음; 저장/조회/삭제 과정에서 네트워크로 전송하지 않음; 백엔드 사용 불가 시 폴백 없이 fail-closed.

**Scale/Scope**: OS 계정당 API 키 1개. `internal/config` 패키지 1개 + `cmd/naeryeo`의 `setup`/`logout` 서브커맨드 배선.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Idiomatic Go First** — PASS. `internal/config`는 `Save/Load/Delete` 3개 함수와 sentinel 에러만 노출하는 최소 표면적을 가진다. go-keyring 호출은 작은 비공개 인터페이스(`keyringBackend`)로 감싸 테스트 대체를 가능케 하되, 이는 소비자(테스트)가 필요로 해서 도입하는 것이지 투기적 추상화가 아니다. 에러는 전부 명시적으로 처리·래핑하며 `panic` 없음.
- **II. Unit Tests Are Mandatory** — PASS. `Save`/`Load`/`Delete` 각각과 sentinel 에러 4종을 테이블 기반 테스트로 커버할 계획(quickstart.md §1). `cmd/naeryeo`의 `setup`/`logout` 배선도 동일 커밋에서 테스트를 추가한다.
- **III. Automated Quality Gates** — PASS. `just fmt`/`just lint`/`just test`(`just check`)를 구현 완료 기준으로 그대로 적용. 새 도구 불필요.
- **IV. Commit Discipline** — PASS. 계획 단계에서는 커밋을 생성하지 않으며, 구현 후 Conventional Commits 메시지를 사람 확인 후에만 커밋한다.

위반 사항 없음 — Complexity Tracking 불필요.

## Project Structure

### Documentation (this feature)

```text
specs/001-keychain-api-key/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   └── config-package.md
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

기존 Go 모듈 레이아웃(`cmd/naeryeo`, `internal/config`, `internal/core`)을 그대로 사용하는 단일 프로젝트다. 새 최상위 디렉터리는 만들지 않는다.

```text
internal/config/
├── doc.go            # 기존 placeholder 패키지 doc (내용 갱신)
├── config.go          # Save/Load/Delete + sentinel 에러 + keyringBackend 인터페이스
└── config_test.go     # 테이블 기반 유닛 테스트 (keyring.MockInit + 테스트 더블)

cmd/naeryeo/
├── main.go            # 기존 라우팅에서 "setup"/"logout" 분기를 실제 구현으로 교체
├── setup.go            # setup 서브커맨드: 프롬프트 입력 → config.Save
├── setup_test.go
├── logout.go            # logout 서브커맨드: config.Delete
└── logout_test.go
```

**Structure Decision**: 별도 `src/`나 `tests/` 최상위 디렉터리를 새로 만들지 않고, README와 constitution이 이미 확정한 `cmd/`+`internal/` 레이아웃을 그대로 따른다(Go 표준 프로젝트 레이아웃). 테스트는 Go 관례대로 각 패키지와 같은 디렉터리에 `_test.go`로 둔다.

## Complexity Tracking

*No violations — table intentionally omitted per template guidance ("Fill ONLY if Constitution Check has violations").*
