---

description: "Task list for API 키 OS 키체인 저장/조회/삭제"
---

# Tasks: API 키 OS 키체인 저장/조회/삭제

**Input**: Design documents from `/specs/001-keychain-api-key/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/config-package.md, quickstart.md

**Tests**: 예시가 아니라 필수다. 프로젝트 constitution(Principle II: Unit Tests Are
Mandatory)에 따라 모든 사용자 스토리에 테스트 태스크가 REQUIRED다.

**Organization**: 태스크는 spec.md의 사용자 스토리(P1~P2) 단위로 그룹화되어, 각 스토리를
독립적으로 구현·검증할 수 있다.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 병렬 실행 가능(다른 파일, 서로 의존성 없음)
- **[Story]**: 이 태스크가 속한 사용자 스토리(US1~US4)
- 모든 설명에 정확한 파일 경로 포함

## Path Conventions

Go 표준 단일 프로젝트 레이아웃, `plan.md`의 Project Structure를 그대로 따른다:
`internal/config/`, `cmd/naeryeo/`. 새 최상위 디렉터리는 만들지 않는다.

---

## Phase 1: Setup

**Purpose**: go-keyring 의존성을 모듈에 반영

- [X] T001 저장소 루트에서 `go get github.com/zalando/go-keyring@latest` 실행 후
      `go mod tidy`로 `go.mod`/`go.sum`을 갱신한다.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 모든 사용자 스토리가 공유하는 패키지 골격 — 이 단계가 끝나기 전에는 어떤
사용자 스토리도 시작할 수 없다.

**⚠️ CRITICAL**: 아래 태스크 완료 전까지 Phase 3 이후 착수 금지

- [X] T002 `internal/config/config.go`를 새로 만들고 다음을 정의한다: 서비스/사용자
      상수(`serviceName = "naeryeo"`, `keyUsername = "odsay-api-key"`), sentinel
      에러(`ErrNotConfigured`, `ErrKeychainUnavailable`, `ErrEmptyValue`,
      `ErrValueTooLarge`), go-keyring 호출을 감싸는 비공개 인터페이스
      `keyringBackend`(`Set(service, username, password string) error`,
      `Get(service, username string) (string, error)`,
      `Delete(service, username string) error`), 이를 실제 go-keyring
      패키지 함수로 구현하는 `goKeyringBackend` 구조체, 그리고 테스트에서
      교체 가능한 패키지 변수 `var backend keyringBackend = goKeyringBackend{}`.
      (data-model.md, contracts/config-package.md 참조)
- [X] T003 `internal/config/config.go`에 공용 에러 변환 헬퍼
      `wrapBackendErr(err error) error`를 구현한다: `nil`→`nil`,
      `errors.Is(err, keyring.ErrNotFound)`→`ErrNotConfigured`,
      `errors.Is(err, keyring.ErrSetDataTooBig)`→`ErrValueTooLarge`(래핑),
      `errors.Is(err, keyring.ErrUnsupportedPlatform)`→`ErrKeychainUnavailable`,
      그 외 모든 non-nil 에러(예: Linux에서 D-Bus 연결 실패처럼 sentinel이 아닌
      에러, research.md §1 참조)도 기본적으로 `ErrKeychainUnavailable`로
      래핑한다(`%w`로 원인 보존). (depends on T002)
- [X] T004 [P] `internal/config/doc.go`의 "placeholder" 문구를 제거하고 실제
      `Save`/`Load`/`Delete` 표면을 설명하도록 패키지 doc 주석을 갱신한다.

**Checkpoint**: 패키지 골격과 에러 모델이 준비됨 — 이제 사용자 스토리 구현 가능

---

## Phase 3: User Story 1 - API 키 최초 등록 (Priority: P1) 🎯 MVP

**Goal**: 사용자가 `naeryeo setup`으로 입력한 ODsay API 키가 OS 키체인에 저장된다.

**Independent Test**: `keyring.MockInit()`으로 인메모리 백엔드를 설치한 뒤
`config.Save`를 호출하고, `keyring.Get`을 직접 호출해 동일한 값이 저장되었는지
확인한다. CLI 레벨에서는 `runSetup`에 가짜 입력을 주입해 저장 함수 호출 여부와
종료 코드를 검증한다.

### Tests for User Story 1 (MANDATORY per Constitution Principle II) ⚠️

> **NOTE: 아래 테스트를 먼저 작성하고, 구현 전에는 실패(또는 컴파일 실패)함을 확인한다**

- [X] T005 [P] [US1] `internal/config/config_test.go`에 `Save`에 대한 테이블 기반
      테스트를 추가한다: 빈 문자열 입력 → `ErrEmptyValue`; 최초 저장 성공 후
      `keyring.Get`으로 직접 조회해 동일 값 확인; 기존 값이 있는 상태에서 재호출
      시 덮어쓰기 확인; `backend`를 `ErrUnsupportedPlatform`을 반환하는 가짜
      구현으로 교체했을 때 `ErrKeychainUnavailable` 확인.
- [X] T006 [P] [US1] `cmd/naeryeo/setup_test.go`에 `runSetup`에 대한 테스트를
      추가한다: 유효한 키가 담긴 stdin 입력 → trim된 값으로 주입된 save 함수가
      호출되고 종료 코드 0; save 함수가 `config.ErrKeychainUnavailable`을
      반환 → 0이 아닌 종료 코드와 stderr 에러 메시지; 빈 줄 입력 → save 함수를
      호출하지 않고 0이 아닌 종료 코드.

### Implementation for User Story 1

- [X] T007 [P] [US1] `internal/config/config.go`에 `Save(apiKey string) error`를
      구현한다: `apiKey == ""`면 `ErrEmptyValue` 반환, 아니면
      `backend.Set(serviceName, keyUsername, apiKey)`를 호출하고 결과를
      `wrapBackendErr`로 감싼다. (depends on T002-T004)
- [X] T008 [P] [US1] 새 파일 `cmd/naeryeo/setup.go`에
      `runSetup(args []string, stdin io.Reader, stdout, stderr io.Writer, save func(string) error) int`를
      구현한다: stdin에서 한 줄을 읽어 trim하고, 비어 있으면 에러 메시지 후
      종료 코드 1, 아니면 `save`를 호출해 성공/실패 메시지를 출력하고
      성공 시 0, 실패 시 1을 반환한다.
- [X] T009 [US1] `cmd/naeryeo/main.go`의 `"setup"` 분기를
      `notImplemented(stderr, "setup")` 대신
      `runSetup(args[1:], os.Stdin, stdout, stderr, config.Save)` 호출로
      교체한다. (depends on T007, T008)

**Checkpoint**: `naeryeo setup`이 단독으로 동작 — API 키가 실제로 키체인에
저장된다 (MVP).

---

## Phase 4: User Story 2 - 저장된 키 조회 (Priority: P1)

**Goal**: `route`/`mcp` 등 다른 구성요소가 나중에 재사용할 수 있도록 저장된 API
키를 조회하는 함수를 제공한다.

**Independent Test**: `keyring.MockInit()` 후 `keyring.Set`으로 값을 직접 심어두고
`config.Load()`가 정확히 동일한 값을 반환하는지, 아무 값도 없을 때
`ErrNotConfigured`를 반환하는지 확인한다.

### Tests for User Story 2 (MANDATORY per Constitution Principle II) ⚠️

- [X] T010 [P] [US2] `internal/config/config_test.go`에 `Load`에 대한 테이블
      기반 테스트를 추가한다: 저장된 값 없음 → `ErrNotConfigured`;
      `keyring.Set`으로 직접 심어둔 값을 정확히 반환; `config.Save` 호출 직후
      `config.Load`가 동일한 값을 반환(왕복 무결성, SC-005); `backend`를
      `ErrUnsupportedPlatform` 반환 가짜 구현으로 교체 시 `ErrKeychainUnavailable`.

### Implementation for User Story 2

- [X] T011 [US2] `internal/config/config.go`에 `Load() (string, error)`를
      구현한다: `backend.Get(serviceName, keyUsername)`를 호출하고 결과를
      `wrapBackendErr`로 감싼다. (depends on T002-T004)

**Checkpoint**: `Save`+`Load` 왕복이 가능 — 향후 `route`/`mcp` 기능이 재사용할
기반이 완성됨.

---

## Phase 5: User Story 3 - 저장된 키 삭제 (Priority: P2)

**Goal**: 사용자가 `naeryeo logout`으로 저장된 API 키를 완전히 제거할 수 있다.

**Independent Test**: 키를 심어둔 뒤 `config.Delete()`를 호출하고 `keyring.Get`을
직접 호출해 사라졌는지 확인한다. 키가 없는 상태에서 `Delete()` 호출 시 에러가
없는지도 확인한다.

### Tests for User Story 3 (MANDATORY per Constitution Principle II) ⚠️

- [X] T012 [P] [US3] `internal/config/config_test.go`에 `Delete`에 대한 테이블
      기반 테스트를 추가한다: 값이 있는 상태에서 삭제 후 `keyring.Get`으로
      직접 확인 시 없음; 값이 없는 상태에서 호출 시 `nil`(idempotent, FR-009);
      `backend`를 `ErrUnsupportedPlatform` 반환 가짜 구현으로 교체 시
      `ErrKeychainUnavailable`.
- [X] T013 [P] [US3] `cmd/naeryeo/logout_test.go`에 `runLogout`에 대한 테스트를
      추가한다: 삭제 함수가 성공 → 종료 코드 0과 성공 메시지; 삭제 함수가
      `config.ErrKeychainUnavailable` 반환 → 0이 아닌 종료 코드와 stderr
      에러 메시지.

### Implementation for User Story 3

- [X] T014 [P] [US3] `internal/config/config.go`에 `Delete() error`를
      구현한다: `backend.Delete(serviceName, keyUsername)`를 호출하고,
      `errors.Is(err, keyring.ErrNotFound)`인 경우 `nil`을 반환하며(FR-009),
      그 외 에러는 `wrapBackendErr`로 감싼다. (depends on T002-T004)
- [X] T015 [P] [US3] 새 파일 `cmd/naeryeo/logout.go`에
      `runLogout(args []string, stdout, stderr io.Writer, del func() error) int`를
      구현한다: `del`을 호출해 성공/실패 메시지를 출력하고 성공 시 0, 실패
      시 1을 반환한다.
- [X] T016 [US3] `cmd/naeryeo/main.go`의 `"logout"` 분기를
      `notImplemented(stderr, "logout")` 대신
      `runLogout(args[1:], stdout, stderr, config.Delete)` 호출로 교체한다.
      (depends on T014, T015)

**Checkpoint**: `setup`→`logout` 전체 CLI 왕복 플로우가 동작.

---

## Phase 6: User Story 4 - 키체인 사용 불가 환경에서 안전한 실패 (Priority: P2)

**Goal**: Secret Service가 없는 headless Linux 등에서 `naeryeo setup`이 평문
파일 폴백 없이 명확한 에러로 실패한다.

**Independent Test**: quickstart.md §3 절차대로 Secret Service가 없는 Linux
컨테이너에서 `naeryeo setup`을 실행해 0이 아닌 종료 코드와 에러 메시지를
확인하고, 컨테이너 파일시스템 전체에서 입력한 키 문자열이 평문으로 전혀
발견되지 않음을 확인한다.

### Tests for User Story 4 (MANDATORY per Constitution Principle II) ⚠️

- [X] T017 [P] [US4] `internal/config/config_test.go`에 `Save`/`Load`/`Delete`
      3개 함수를 모두 커버하는 결합 테이블 테스트를 추가한다: `backend`를
      `keyring.ErrUnsupportedPlatform`이 아닌 **임의의 불투명한 에러**(예:
      `errors.New("dbus: could not connect: no such file or directory")`,
      research.md §1에서 확인한 실제 headless Linux 실패 형태를 흉내)를
      반환하는 가짜 구현으로 교체했을 때도 세 함수 모두
      `ErrKeychainUnavailable`을 반환하고 `errors.Unwrap`으로 원인 에러가
      보존됨을 검증한다.

### Implementation for User Story 4

- [X] T018 [US4] `specs/001-keychain-api-key/quickstart.md` §3 절차에 따라
      Secret Service가 없는 Linux 컨테이너(예: 최소 `debian:stable`, D-Bus
      세션 없음)에서 `go run ./cmd/naeryeo setup`을 수동 실행해 (a) 0이 아닌
      종료 코드와 명확한 에러 메시지, (b) `grep -r`로 컨테이너 파일시스템
      전체를 검색해도 입력한 키 문자열이 어디에도 없음을 확인하고, 결과를
      PR 설명에 기록한다. (코드 변경 없음 — 검증 전용 태스크)

**Checkpoint**: 모든 사용자 스토리가 독립적으로 동작하며, 키체인 미지원
환경에서의 fail-closed 동작이 실제 환경에서 확인됨.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 스토리 전반에 걸친 마무리

- [X] T019 [P] 저장소 루트에서 `just fmt`, `just lint`, `just test`(`just check`)를
      실행하고 발견된 문제를 모두 수정한다(constitution Principle III).
- [X] T020 [P] `cmd/naeryeo/setup.go`/`logout.go`의 실제 출력 메시지를
      `README.md`의 문서화된 예시(예: "OS 키체인에 저장 완료")와 비교해
      문구를 맞춘다.
- [X] T021 커밋 제안 전, constitution Principle I(idiomatic Go)과 Principle
      II(테스트 커버리지)를 기준으로 변경분을 자체 리뷰한다(AGENTS.md
      Required Workflow 5단계).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 의존성 없음 — 즉시 시작 가능
- **Foundational (Phase 2)**: Setup 완료에 의존 — 모든 사용자 스토리를 블록함
- **User Stories (Phase 3~6)**: 모두 Foundational 완료에 의존
  - US1(P1)·US2(P1)는 서로 독립적이라 병렬 착수 가능
  - US3(P2)·US4(P2)는 US1의 산출물(Save, setup.go)을 활용하지만 자체
    구현/테스트는 Foundational 완료 후 바로 착수 가능
  - 우선순위 순서(P1 → P1 → P2 → P2)로 순차 진행도 가능
- **Polish (Phase 7)**: 구현하기로 한 모든 사용자 스토리 완료에 의존

### User Story Dependencies

- **US1 (P1)**: Foundational 이후 바로 시작 가능. 다른 스토리에 의존하지 않음.
- **US2 (P1)**: Foundational 이후 바로 시작 가능. US1과 독립적으로 테스트 가능
  (테스트가 `keyring.Set`으로 직접 시드하므로 US1 완료를 기다릴 필요 없음).
- **US3 (P2)**: Foundational 이후 바로 시작 가능. `logout` CLI 메시지 문구는
  US1의 `setup.go` 패턴을 참고하지만 코드 의존성은 없음.
- **US4 (P2)**: Foundational 이후 바로 시작 가능. T017은 US1·US2·US3의
  `wrapBackendErr` 사용 여부와 무관하게 독립적으로 작성 가능하나, 세 함수가
  모두 구현되어 있어야 결합 테스트가 의미를 가지므로 실질적으로는 US1~US3
  이후에 수행하는 것을 권장.

### Within Each User Story

- 테스트를 먼저 작성하고 구현 전 실패(또는 컴파일 실패)를 확인한다
- `internal/config`의 함수 구현이 `cmd/naeryeo`의 CLI 배선보다 먼저(또는 병렬로)
- CLI 배선(`main.go` 수정)은 해당 스토리의 나머지 구현이 끝난 뒤 마지막에

### Parallel Opportunities

- T004(doc.go)는 T002/T003(config.go)와 다른 파일이라 병렬 가능
- US1의 T005·T006은 서로 다른 파일이라 병렬 가능; T007·T008도 서로 다른
  파일이라 병렬 가능(단, T009는 둘 다 끝난 뒤)
- US3의 T012·T013, T014·T015도 각각 병렬 가능(단, T016은 둘 다 끝난 뒤)
- Foundational 완료 후 US1과 US2는 서로 다른 개발자가 완전히 병렬로 진행 가능

---

## Parallel Example: User Story 1

```bash
# US1 테스트를 함께 실행:
Task: "internal/config/config_test.go에 Save 테이블 테스트 추가"
Task: "cmd/naeryeo/setup_test.go에 runSetup 테스트 추가"

# US1 구현을 함께 진행:
Task: "internal/config/config.go에 Save 구현"
Task: "cmd/naeryeo/setup.go에 runSetup 구현"
```

---

## Implementation Strategy

### MVP First (User Story 1만)

1. Phase 1: Setup 완료
2. Phase 2: Foundational 완료 (모든 스토리를 블록하는 단계이므로 필수)
3. Phase 3: User Story 1 완료
4. **STOP and VALIDATE**: `naeryeo setup`이 실제 OS 키체인에 키를 저장하는지
   수동으로 확인
5. 필요하면 여기서 데모/배포

### Incremental Delivery

1. Setup + Foundational 완료 → 기반 준비
2. US1 추가 → 독립 검증 → 데모(MVP: 저장 가능)
3. US2 추가 → 독립 검증 → 데모(조회 가능, 향후 route/mcp가 재사용할 준비 완료)
4. US3 추가 → 독립 검증 → 데모(삭제까지 가능한 완전한 CLI 왕복)
5. US4 추가 → 독립 검증 → 데모(키체인 미지원 환경에서도 안전)
6. 각 스토리는 이전 스토리를 깨지 않고 가치를 더한다

---

## Notes

- [P] 태스크 = 서로 다른 파일, 의존성 없음
- [Story] 라벨은 태스크를 사용자 스토리로 추적 가능하게 함
- 각 사용자 스토리는 독립적으로 완결/검증 가능해야 함
- 구현 전 테스트가 실패(또는 컴파일 실패)하는지 확인
- 각 태스크 또는 논리적 그룹 완료 후 커밋(단, constitution Principle IV에 따라
  실제 커밋은 사람의 명시적 확인 후에만 생성)
- 체크포인트에서 멈춰 스토리를 독립적으로 검증할 것
- 지양할 것: 모호한 태스크, 동일 파일 충돌, 스토리 간 독립성을 해치는 교차 의존
