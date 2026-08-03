---
description: "Task list for 006-self-hosted-routing-provider"
---

# Tasks: 자체 호스팅 경로 검색 제공자

**Input**: Design documents from `/specs/006-self-hosted-routing-provider/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: 프로젝트 헌법 원칙 II(Unit Tests Are Mandatory)에 따라 테스트 태스크는 **필수**다.
신규 exported 심볼은 그것을 도입하는 **같은 커밋**에 테스트를 동반해야 한다.

**Organization**: 태스크는 spec.md의 User Story 단위로 묶여 있어 각 스토리를 독립적으로
구현·검증·전달할 수 있다.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 병렬 실행 가능 (다른 파일, 미완료 태스크에 의존하지 않음)
- **[Story]**: 해당 태스크가 속한 User Story (US1~US4)
- 모든 설명에 정확한 파일 경로 포함

## Path Conventions

기존 단일 Go 모듈 레이아웃. `cmd/naeryeo/`(진입점), `internal/`(로직), `docs/`·`deploy/`(문서·배포).
테스트는 Go 관례대로 대상과 같은 패키지의 `_test.go`에 둔다 — 별도 `tests/` 트리를 만들지 않는다.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 신규 패키지·디렉터리의 자리를 만들고 변경 전 기준선을 확보한다

- [X] T001 브랜치 `feature/006-self-hosted-routing-provider`에서 `just check`를 실행해 변경 전 기준선이 green임을 확인하고 결과를 기록한다 (회귀 판정의 기준점)
- [X] T002 [P] `internal/motis/doc.go`를 생성한다 — 패키지 주석에 "MOTIS(자체 호스팅 라우팅 엔진) 어댑터. `internal/core`를 단방향 의존하며 `core.RouteResult`로 매핑한다"를 명시 (`internal/geocode/doc.go`의 서술 방식을 따를 것)
- [X] T003 [P] `docs/self-hosting.md`를 제목·목차만 갖춘 자리표시자로 생성한다 — Foundational 단계의 `docsURL` 상수가 이 경로를 가리켜야 하므로 **내용보다 경로 확정이 먼저**다 (plan.md §7)
- [X] T004 [P] `deploy/motis/README.md`를 자리표시자로 생성하고 `docs/self-hosting.md`로 연결한다

**Checkpoint**: 신규 경로가 확정되어 Foundational 코드가 이를 참조할 수 있다

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 모든 User Story가 의존하는 공유 인프라 — 설정 저장소, 에러 taxonomy, 공유 타입 변경

**⚠️ CRITICAL**: 이 단계가 끝나기 전에는 어떤 User Story도 시작할 수 없다

### 설정 저장소 (contracts/settings-file.md)

> **작성 순서**: T008(테스트)을 먼저 작성해 **컴파일 실패**를 확인한 뒤 T005~T007을 구현한다.
> Phase 3~6과 같은 TDD 순서이며, 번호 순서와 작성 순서가 다른 것은 의도적이다 — 커밋은
> 아래 "Commit Boundaries"의 C-1 하나로 묶인다.

- [X] T005 `internal/config/settings.go`에 `Settings`, `RoutingProvider`(`ProviderUnset`/`ProviderMotis`/`ProviderODsay`), `GeocoderChoice`(`GeocoderNone`/`GeocoderKakao`) 타입과 `os.UserConfigDir()` 기반 경로 해석 함수를 정의한다 — 이 타입이 spec FR-001의 "제공자 선택"을 표현하는 값이다 (spec FR-001, data-model.md §1)
- [X] T006 `internal/config/settings.go`에 `LoadSettings`/`SaveSettings`를 구현한다 — 저장 시 검증(provider 허용값, `motis_url` 절대 URL·scheme·host·후행 슬래시 정규화), 로드 시 관대한 처리(파일 부재·JSON 파싱 실패·미인식 값 → `ProviderUnset`, 알 수 없는 키 무시), 디렉터리 `0700`·파일 `0600`. **키체인을 거치지 않는다** — 저장할 비밀이 없는 사용자에게 키체인 접근을 요구하지 않기 위함 (spec FR-004)
- [X] T007 `internal/config/settings.go`에서 JSON 파싱 실패 시 **원본 파싱 에러 문자열을 반환값에 담지 않도록** 한다 (spec FR-019 — 저장소 원문 비노출)
- [X] T008 [P] `internal/config/settings_test.go`에 contracts/settings-file.md의 "테스트 계약" 표 8개 항목을 테이블 주도 테스트로 작성한다 — round-trip, 파일 부재, 손상 JSON, 알 수 없는 키, 권한 0600(POSIX 한정), 후행 슬래시 정규화, 무효 URL 시 **파일 미생성**, `os.UserConfigDir()` 하위 해석

### 에러 taxonomy — 망라성 게이트를 의도적으로 깨뜨렸다가 통과시킨다

> **작성 순서**: T015(테스트)는 T013 구현과 짝을 이루며, 설정 저장소와 마찬가지로 테스트를
> 먼저 작성해도 좋다. 어느 쪽이든 커밋은 C-2 하나로 묶인다 — T010의 실패 상태를 커밋하지
> 않는다.

- [X] T009 `internal/core/errors.go`에 `ErrMotisUnavailable`(sentinel)과 `ErrMotisRejected{Status int}`(구조체)를 추가한다 — `ErrMotisRejected`는 **Status만 보존**하고 본문·URL은 담지 않는다 (data-model.md §3)
- [X] T010 `just test`를 실행해 `TestErrorCodeExhaustive_EveryCoreErrorHasACode`가 **실패하는 것을 확인하고 출력을 기록한다** — 게이트가 살아 있음의 증명이며 spec FR-020·SC-004가 요구하는 검증이다. 실패를 보지 않고 다음으로 넘어가지 말 것
- [X] T011 `cmd/naeryeo/errcode.go`에 self-hosting 문서 URL 상수를 **한 곳에만** 정의하고 — 값과 버전 고정 방식을 여기서 확정한다. 브랜치 고정 링크는 문서가 이동하면 조용히 깨지므로 릴리스 태그 기준 링크 또는 안정 리다이렉트 중 하나를 택하고 근거를 주석에 남긴다 (spec FR-022) — `failure` 구조체에 `Docs string` 필드를, `RouteError`(`cmd/naeryeo/mcp.go`)에 `Docs string \`json:"docs,omitempty"\`` 필드를 추가한다 (contracts/error-codes.md). 완료 조건에 `specs/006-self-hosted-routing-provider/contracts/error-codes.md`의 `<self-hosting 문서 URL>` 자리표시자 3곳을 확정된 실제 값으로 치환하는 것을 포함한다
- [X] T012 `cmd/naeryeo/errcode.go`의 `failure.Prose()`가 `Docs`가 비어 있지 않을 때만 세 번째 줄로 덧붙이도록 수정한다 — 기존 코드는 `Docs`가 빈 문자열이라 **출력이 바이트 단위로 불변**이어야 한다 (spec 005 FR-007)
- [X] T013 `cmd/naeryeo/errcode.go`에 `codeProviderNotConfigured`/`codeMotisUnavailable`/`codeMotisRejected` 상수와 `classifyRouteError`의 `ErrMotis*` 분기, 그리고 `providerNotConfiguredFailure()` 생성자를 추가한다 — 문구는 contracts/error-codes.md의 "문구" 절 그대로, **호스트·포트·HTTP 상태를 문구에 넣지 않는다**. `provider_not_configured`가 spec FR-014의 "다른 실패와 구별되는 신호"다
- [X] T014 `cmd/naeryeo/errcode_exhaustive_test.go`의 `coreErrorSamples`에 `ErrMotisUnavailable`·`ErrMotisRejected` 샘플을 추가하고 `just test`로 게이트가 **통과함**을 확인한다 (T010의 실패가 해소됨)
- [X] T015 [P] `cmd/naeryeo/errcode_test.go`에 신규 3개 코드의 분류·문구·`Docs` 존재를 테이블 주도로 검증하는 테스트를 추가한다

### 공유 타입 — 요금 "정보 없음"

- [X] T016 `internal/core/client.go`의 `RouteResult`에 `FareKnown bool`을 추가하고 ODsay 경로(`toRouteResult`)가 항상 `true`로 채우도록 한다 (data-model.md §2)
- [X] T017 `cmd/naeryeo/mcp.go`의 `RouteToolOutput.FareWon`을 `int` → `*int`로 바꾸고 `toRouteToolOutput`이 `FareKnown`에 따라 `nil` 또는 값을 설정하도록 한다 — 값이 있을 때의 와이어 포맷은 **변경 전과 동일**해야 한다
- [X] T018 `cmd/naeryeo/route.go`의 `printRouteResult`에 `FareKnown == false`일 때 `요금: N원` 대신 `요금 정보 없음`을 출력하는 분기를 추가한다 — `NoTravelNeeded`를 먼저 분기하는 순서 유지
- [X] T019 `just check`를 실행해 기존 테스트가 **한 줄도 수정되지 않은 채** 전부 통과함을 확인한다 — 수정이 필요하다면 T016~T018의 설계가 어긋난 것이다 (spec FR-013, SC-008, quickstart.md S7)

**Checkpoint**: 설정 저장소·에러 taxonomy·공유 타입이 준비됨. User Story 구현을 시작할 수 있다

---

## Phase 3: User Story 1 - 상용 API 키 없이 자체 호스팅으로 경로 검색 (Priority: P1) 🎯 MVP

**Goal**: 상용 경로 검색 키가 전혀 없는 사용자가 자기가 운영하는 MOTIS를 연결해 경로 검색을
끝까지 수행한다. 설정 어느 단계에서도 상용 API 키를 요구받지 않는다.

**Independent Test**: ODsay 키가 키체인에 없는 깨끗한 환경에서 MOTIS만 설정한 뒤
`naeryeo route`를 실행한다. 키 요구 없이 결과가 나오면 통과.

### Tests for User Story 1 (MANDATORY per Constitution Principle II) ⚠️

> **먼저 작성하고 실패를 확인한 뒤 구현할 것**

- [X] T020 [P] [US1] `internal/motis/client_test.go`에 `httptest.Server`로 `/api/v1/geocode` + `/api/v6/plan` 정상 응답을 고정하고 `FindRoute`가 `core.RouteResult`를 반환하는 happy path 테스트를 작성한다 (`internal/geocode/kakao_test.go`의 서버 고정 패턴을 따를 것)
- [X] T021 [P] [US1] `internal/motis/client_test.go`에 테이블 주도 케이스를 추가한다 — geocode 빈 결과 + Kakao 폴백 성공/실패, `itineraries`·`direct` 모두 빈 경우 → `core.ErrNoRoute`, `direct`만 있는 경우 → 성공, 요금 부재 → `FareKnown == false`
- [X] T022 [P] [US1] `cmd/naeryeo/setup_test.go`를 재작성해 fake stdin으로 MOTIS 선택 → URL 입력 → 지오코더 선택 → 요약 확인까지 전 단계를 구동하는 테스트를 작성한다 — **어느 단계에서도 상용 API 키를 요구하지 않음**을 함께 단언한다 (spec FR-003, pty 불필요)
- [X] T023 [P] [US1] `cmd/naeryeo/main_test.go`에 동일 설정에서 `route` 경로와 MCP 툴 핸들러가 **같은 제공자**를 사용함을 검증하는 테스트를 추가한다 (spec FR-002, SC-005)
- [X] T024 [P] [US1] `cmd/naeryeo/routejson_test.go`에 MOTIS 결과의 `--json` 성공 스키마가 ODsay와 동일하고 `fareWon` 키가 **부재**함을 검증하는 테스트를 추가한다 (spec FR-010, FR-011)

### Implementation for User Story 1

- [X] T025 [US1] `internal/motis/client.go`에 `Client` 구조체(`BaseURL`, `HTTPClient`, `Logger`, `Geocoder core.Geocoder`)와 `NewClient(baseURL string) *Client`를 구현한다 — `core.Client`의 nil 기본값 처리(10초 타임아웃, `slog.DiscardHandler`) 관례를 그대로 따를 것
- [X] T026 [US1] `internal/motis/client.go`에 `resolvePlace`를 구현한다 — `GET /api/v1/geocode?text=<name>` → 첫 매치의 `lat`/`lon`, 빈 배열이면 `Geocoder`(Kakao) 폴백, 그래도 없으면 내부 not-found 신호. Kakao 축은 **현행 그대로** 선택적 폴백으로 남는다 (spec FR-028, data-model.md §5-1, `core.Client.resolveStation`과 동형)
- [X] T027 [US1] `internal/motis/client.go`에 `FindRoute(ctx, from, to)`를 구현한다 — 양쪽 `resolvePlace` 후 `GET /api/v6/plan?fromPlace=<lat,lon>&toPlace=<lat,lon>&numItineraries=1`, 응답을 `core.RouteResult`로 매핑(`duration` 초→분, `transfers` 그대로, `FareKnown=false`), not-found는 `*core.ErrPointNotFound{Side}`로 — 결과 항목이 ODsay와 같은 집합이어야 한다 (spec FR-009, data-model.md §5-2)
- [X] T028 [US1] `internal/motis/client.go`에 leg → `core.RouteStep` 문구 변환을 구현한다 — WALK/지하철/BUS/그 외 4분기, `routeShortName` → `headsign` → `agencyName` 순 대체 (data-model.md §5-3)
- [X] T029 [US1] `internal/motis/client.go`의 HTTP 계층에서 실패를 분류한다 — transport 에러·타임아웃·5xx → `core.ErrMotisUnavailable`, 4xx·디코드 실패 → `*core.ErrMotisRejected{Status}`. **에러를 감쌀 때 요청 URL이 실리지 않도록** 한다 (spec FR-018; `core.doGet`이 `*url.Error`를 감싸며 겪은 문제를 반복하지 말 것)
- [X] T030 [US1] `cmd/naeryeo/setup.go`를 재작성한다 — 번호 프롬프트 루프 기반 다단계 마법사(제공자 → 제공자별 입력 → 지오코더 → 요약/확인), MOTIS가 1번이자 Enter 기본값, MOTIS URL 기본값 `http://localhost:8080`. **TUI 프레임워크를 도입하지 않는다**. 기본 선택지가 MOTIS인 것은 spec FR-037의 "기본 제시 경로"이며, 무설정 상태의 암묵 동작을 뜻하지 않는다 — 설정 파일이 없으면 여전히 `provider_not_configured`다 (spec FR-005, FR-037, contracts/cli-interface.md)
- [X] T031 [US1] `cmd/naeryeo/setup.go`에 비대화식 플래그를 추가한다 — `--provider`, `--motis-url`, `--geocoder`(bool→string, breaking). **시크릿을 받는 플래그는 만들지 않는다**; 비밀값은 stdin으로만 (spec FR-006, FR-036)
- [X] T032 [US1] `cmd/naeryeo/setup.go`에 MOTIS 도달성 프로브를 구현한다 — 저장 **전에** `GET {motis_url}/api/v1/geocode?text=서울역`(타임아웃 5초), 연결 실패/4xx·5xx/매치 0건을 각각 구분해 저장을 거부하고 문서 링크를 안내한다 (contracts/settings-file.md, spec FR-016)
- [X] T033 [US1] `cmd/naeryeo/main.go`에 `type routeFinder func(ctx, from, to) (core.RouteResult, error)`와 `newRouteFinder(logger) (routeFinder, *failure)`를 구현한다 — `LoadSettings`로 제공자를 고르고, MOTIS면 `motis.NewClient(settings.MotisURL)`, ODsay면 키체인에서 키를 읽어 `core.NewClient`, 양쪽 모두 지오코더 설정 시 Kakao 주입 (plan.md §1)
- [X] T034 [US1] `cmd/naeryeo/route.go`와 `cmd/naeryeo/mcp.go`가 `newRouteFinder`를 **공유**하도록 배선하고, 기존 `apiKey` 파라미터 흐름을 제거한다 — 두 진입점의 제공자 불일치가 구조적으로 불가능해지는 지점 (spec FR-002)

**Checkpoint**: US1 완료 — 상용 키 없이 MOTIS만으로 경로 검색이 동작하고, `route`/`mcp`가 같은 제공자를 쓴다. **MVP 지점**

---

## Phase 4: User Story 2 - 문서만 보고 자체 호스팅 환경을 구축 (Priority: P2)

**Goal**: 자체 호스팅 경험이 없는 사용자가 naeryeo 문서만 따라 MOTIS를 띄우고 연결한다.

**Independent Test**: 제3자에게 `docs/self-hosting.md`만 주고 구축시킨 뒤, 문서 바깥 지식이
필요했던 지점을 센다. 0건이면 통과.

> **⚠️ 실측 선행**: T035~T038은 실제로 MOTIS를 한 번 띄워야 풀린다(research.md U1~U3).
> 추정치로 문서를 쓰면 spec FR-023(실측 기준값)을 만족하지 못한다.

### 검증 기준 고정 for User Story 2

> US2의 산출물은 문서이므로 자동 테스트가 성립하지 않는다. 대신 수용 기준을 문서 자체에
> 고정하고 T043의 제3자 검증으로 확인한다.

- [X] T035 [P] [US2] quickstart.md S3의 확인 항목 5개를 `docs/self-hosting.md` 완료 판정 체크리스트로 문서 말미에 명시한다 — 문서 자체의 수용 기준이므로 자동 테스트 대신 검증 절차로 고정한다

### Implementation for User Story 2

- [X] T036 [US2] MOTIS 컨테이너를 1회 기동해 `motis config <osm.pbf> <gtfs.zip>`이 생성하는 `config.yml`의 실제 스키마를 확인하고 research.md U1을 해소한다
- [X] T037 [US2] KTDB GTFS의 공식 확보 경로·갱신 주기·지역 커버리지를 확인해 research.md U3을 해소한다 — Transitous의 Dropbox 링크는 **참고**로만 두고 원본 경로를 1순위로 삼는다 (research.md R7)
- [X] T038 [US2] `deploy/motis/compose.yaml`을 작성한다 — 이미지 태그 **고정**(`latest` 금지), import와 server를 분리한 2단계 구성, GTFS/OSM은 호스트 바인드 마운트로 교체 가능하게
- [X] T039 [US2] 실제 그래프 빌드를 1회 수행하며 소요 시간·최대 RSS·디스크 사용량을 계측해 research.md U2를 해소한다 (spec FR-023의 직접 입력)
- [X] T040 [US2] `docs/self-hosting.md`를 완성한다 — 데이터 확보처, 실행 절차, naeryeo 연결 방법(`naeryeo setup --provider=motis --motis-url=...`), T039의 실측 자원 요구치 (spec FR-021, FR-022, FR-023)
- [X] T041 [US2] `docs/self-hosting.md`에 데이터 한계 절을 추가한다 — GTFS 갱신 주기, 실시간 정보 부재, 지역 커버리지, 상용 서비스와 결과가 다를 수 있음 (spec FR-024, Assumptions)
- [X] T041a [US2] `docs/self-hosting.md`와 `README.md`에 **잔여 외부 의존** 절을 추가한다 — 경로 검색은 자체 호스팅으로 외부 의존이 사라지지만 건물명·주소 검색은 여전히 외부 장소 검색 서비스를 쓴다는 점, 역·정류장 이름만 쓰면 그 의존조차 없다는 점, 그 선택이 제공자와 독립이라는 점 (spec FR-029, FR-030). T041의 데이터 한계 절과는 **분리된 절**로 쓴다 — 하나는 데이터의 품질·범위, 다른 하나는 의존성 축을 다룬다
- [X] T042 [US2] `README.md`를 개편한다 — 제공자 개념 도입, 자체 호스팅을 **1순위 경로**로 서술, ODsay는 동등한 대안으로, `logout` 안내 제거(`README.md:172-173`), `--json` 문서화, `docs/self-hosting.md` 링크 (spec FR-025, FR-035, FR-037)
- [X] T043 [US2] 제3자에게 문서만 주고 구축을 시켜 quickstart.md S3의 5개 항목을 검증하고 결과를 기록한다 (spec SC-003, SC-004)
  - **방법**: 문서 작성자와 분리된 리뷰 에이전트에게 `docs/self-hosting.md`만 읽게 하고(소스·README·specs 접근 금지), 문서 밖 지식이 필요한 지점을 세게 했다. 17건 지적(BLOCKER 4, MAJOR 7, MINOR 6).
  - **1차 판정 결과**: 항목 1(문서 밖 지식 0건) **불합격** — 저장소 체크아웃 필요성 미기재, `data/` 디렉터리 생성 단계 누락, 상대 경로의 기준점 미명시, 바이너리 설치 안내 부재.
  - **수정 완료**: §2-1(체크아웃·빌드·Docker 요건) 신설, `mkdir -p deploy/motis/data` 추가, "모든 명령은 저장소 최상위에서" 전역 규칙 명시, GTFS 리네임 명령·내용 검증법 추가, OSM `curl -f` 및 크기 확인 추가, 메모리 권장치 4 GiB→**8 GB**(실측 3.98 GiB에 여유 없음), 다운로드에 이미지 215 MB 반영, import 성공 판정 기준 추가, `(health: starting)` 타이밍 설명, §8-A(빌드 단계 실패: OOM·디스크·포트 충돌·파일명·불완전 다운로드) 신설, 재빌드 시 **서버 선정지** 순서 수정, 원본 삭제 팁에 재신청 비용 경고 연결, §9의 재현성 주장을 "엔진은 고정·데이터는 미고정"으로 정정.
  - **기각 1건**: "`localhost` 대신 `127.0.0.1`을 쓰라"(MAJOR 주장)는 실측 기각. IPv6 우선 해석 문제는 **컨테이너 내부 healthcheck에 한정**되며 이미 수정돼 있다. 호스트에서 `localhost:8080`은 curl·naeryeo 모두 200으로 동작하고, 도구 기본값(`defaultMotisURL`)도 `localhost`라 문서만 바꾸면 오히려 불일치가 생긴다.
  - **2차 검증(사실 대조)**: 별도 에이전트가 문서의 모든 주장을 소스·실엔진과 대조해 9건을 추가로 찾았다. 전부 자체 실측으로 재확인 후 반영.
    - **§7이 거짓이었다(MAJOR)**: "건물명·주소 검색은 Kakao 키가 있어야 동작한다"는 서술이 사실과 다르다. MOTIS 색인은 OSM의 건물(`type=PLACE`)·주소(`type=ADDRESS`)까지 담아, `--geocoder=none`에서 `아이디스 타워 → 수지구청`(51분)·`테헤란로 152 → 강남역`(11분)이 그대로 동작한다. `README.md`에도 같은 오류가 있어 함께 고쳤다. 자체 호스팅의 가치를 실제보다 약하게 적고 있던 셈이다(research.md R13 갱신).
    - **§6의 인과가 반대였다(MAJOR)**: "route_type 오표기 탓에 KTX에서 '버스'가 빠진다"고 썼으나, `describeLeg`는 `isBus()`일 때만 수단 이름을 붙이므로 KTX·지하철은 원래 노선명만 나온다. 실제 피해자는 **버스로 표기되지 않은 시내버스**(`6900 승차`)다.
    - **§8 `motis_rejected` 진단이 도달 불가 경로였다(MAJOR)**: "질의 날짜가 창을 벗어난 경우"라고 했으나 naeryeo에는 날짜 옵션이 없다(`route.go`는 `--from/--to/--debug/--json`뿐, plan URL에 `time` 미포함). 실제 발생 경로는 (가) 주소가 MOTIS가 아님(200이지만 디코드 실패 → `client.go:156-162`도 이 코드) (나) 그래프 만료. 둘 다로 다시 썼다.
    - MINOR 5건: `route_type` 6=곤돌라(KTX·SRT)·7=강삭철도(항공)로 정정, Compose 버전 `v2`→실측 `v5.1.2`, 적재 창 `365일`→실측 366일(빌드 전날 00시 시작), `point_not_found` 코드명 명시(§8이 "코드별 정리"라면서 이 코드만 이름이 없었다), setup의 미문서화 거부 2종(`엔진에 연결할 수 없습니다`/`주소가 MOTIS 서버가 맞는지`) 추가.
  - **미해소**: SC-003의 "사람인 제3자" 조건은 여전히 미충족. 릴리스 전 실사용자 1명의 구축을 권장한다.
  - **릴리스 전 주의**: `selfHostingDocsURL`이 가리키는 `blob/main/docs/self-hosting.md`는 이 브랜치가 병합되기 전까지 404다. 실패 출력에 실리는 링크이므로 병합 전 릴리스 금지.

**Checkpoint**: US2 완료 — 문서만으로 자체 호스팅 환경을 구축할 수 있다

---

## Phase 5: User Story 3 - 엔진이 멈췄을 때 원인과 조치를 즉시 안다 (Priority: P3)

**Goal**: 자체 호스팅 엔진이 정지·오설정 상태일 때 사람과 AI 모두 무엇이 잘못됐고 무엇을
해야 하는지 즉시 안다.

**Independent Test**: 엔진을 내린 상태·주소를 틀린 상태·제공자 미설정 상태를 각각 만들고,
사람용 출력과 기계 판독 출력이 올바른 후속 행동으로 이어지는지 확인한다.

### Tests for User Story 3 (MANDATORY) ⚠️

- [X] T044 [P] [US3] `cmd/naeryeo/routejson_test.go`에 제공자 미설정 상태에서 `route --json`과 MCP 툴이 **같은** `provider_not_configured` 코드·문구를 반환함을 검증하는 테스트를 추가한다 (spec 005 FR-016)
- [X] T045 [P] [US3] `cmd/naeryeo/routejson_test.go`에 MOTIS 연결 불가 → `motis_unavailable`, 4xx → `motis_rejected`이고 **세 코드 모두 `docs`가 비어 있지 않음**을 검증하는 테스트를 추가한다 — 출력에 담긴 안내만으로 복구 경로에 도달할 수 있어야 한다 (spec FR-015, FR-016, FR-017, SC-009)
- [X] T045a [P] [US3] `cmd/naeryeo/routejson_test.go`에 **폴백이 일어나지 않음**을 단언하는 테스트를 추가한다 — 설정은 MOTIS이고 키체인에 ODsay 키도 저장된 상태에서 MOTIS가 연결 불가일 때, 결과가 `motis_unavailable`이고 ODsay 엔드포인트로의 요청이 **0건**임을 확인한다 (spec FR-008 "자동 대체 금지"). 폴백 금지는 사용자가 비용을 통제하려고 자체 호스팅을 고른 이유 자체이므로, 부정 테스트가 없으면 나중에 "친절한 폴백"이 추가되는 것을 막을 장치가 없다
- [X] T046 [P] [US3] `cmd/naeryeo/routejson_test.go`에 실패 시 stdout·stderr 전체에 테스트가 쓴 MOTIS 호스트·포트 문자열(`127.0.0.1:PORT`)이 **나타나지 않음**을 부분 일치로 단언하는 테스트를 추가한다 (spec FR-018, SC-006)
- [X] T047 [P] [US3] `cmd/naeryeo/route_test.go`에 신규 3개 코드가 프로즈 모드에서 message/hint/docs **3줄**로 렌더링되고, 기존 코드들은 변경 전과 바이트 동일함을 검증하는 테스트를 추가한다

### Implementation for User Story 3

- [X] T048 [US3] `cmd/naeryeo/route.go`에 `newRouteFinder`가 반환한 사전 실패(`provider_not_configured` 등)를 검색 시도 **이전에** 프로즈·JSON 양쪽으로 보고하는 경로를 구현한다 (contracts/cli-interface.md)
- [X] T049 [US3] `cmd/naeryeo/mcp.go`의 툴 핸들러에 동일한 사전 실패 경로를 구현한다 — `failureToolResult`를 재사용해 구조화된 코드가 `structuredContent`로 전달되게 한다
- [X] T050 [US3] 만료된(과거 날짜만 있는) 타임테이블에서 MOTIS `plan`이 어떤 응답을 주는지 실측해 research.md U4를 해소하고, 필요하면 제공자가 MOTIS일 때만 `no_route`의 hint에 "시간표 데이터가 최신인지 확인" 문구를 덧붙인다 (spec FR-016)
  - 실측 완료(research.md R12): 만료 시간표는 `import`가 경고 없이 통과하고 `plan`이 HTTP 200 + 빈 결과를 반환 → `no_route`. 적재 창 밖 시각은 HTTP 400 → `motis_rejected`.
  - **코드 변경은 하지 않기로 결정.** hint를 제공자별로 가르려면 `classifyRouteError`가 제공자를 알아야 하고 이는 테스트 포함 12개 호출부에 파급된다. 반면 실제 배포되는 KTDB 피드는 `calendar.txt`가 2030년까지라 이 경로를 타지 않는다. 대신 `docs/self-hosting.md` §8의 `no_route` 항목에 "다른 질의도 전부 no_route라면 GTFS부터 의심하라"를 명시했다.

**Checkpoint**: US3 완료 — 자체 호스팅 실패가 코드·문구·문서 링크로 진단 가능하고 내부망 정보가 새지 않는다

---

## Phase 6: User Story 4 - 기존 사용자가 한 번의 재설정으로 전환 (Priority: P4)

**Goal**: 기존 ODsay 사용자가 업그레이드 후 명확한 안내를 받고, 초기 설정 1회로 기존 사용을
이어가거나 자체 호스팅으로 옮긴다.

**Independent Test**: 키체인에 ODsay 키가 있고 설정 파일은 없는 상태에서 업그레이드 후
정상 사용까지의 단계 수를 센다. 1이면 통과.

### Tests for User Story 4 (MANDATORY) ⚠️

- [X] T051 [P] [US4] `cmd/naeryeo/route_test.go`에 fake 키체인에 ODsay 키가 있고 설정 파일이 없는 상태에서 첫 검색이 `provider_not_configured`로 실패하고 setup을 안내함을 검증하는 테스트를 추가한다 (spec FR-031, FR-032)
- [X] T052 [P] [US4] `cmd/naeryeo/setup_test.go`에 `--provider=odsay` 재선택 시 **키 재입력 없이** 저장된 키가 재사용됨을 검증하는 테스트를 추가한다 — 정상 사용까지 초기 설정 1회로 끝나야 한다 (spec FR-033, SC-010)
- [X] T053 [P] [US4] `cmd/naeryeo/setup_test.go`에 `--delete=odsay|kakao|all`의 멱등 동작과 "삭제할 키가 없습니다" 구분, 그리고 **설정 파일이 그대로 남음**을 검증하는 테이블 주도 테스트를 추가한다 (data-model.md §1)
- [X] T054 [P] [US4] `cmd/naeryeo/main_test.go`에 `naeryeo logout`이 unknown command로 실패하고 usage 문자열에 `logout`이 없음을 검증하는 테스트를 추가한다
- [X] T055 [P] [US4] `cmd/naeryeo/setup_test.go`에 시크릿을 받는 플래그가 **존재하지 않음**을 FlagSet 순회로 단언하는 테스트를 추가한다 (spec FR-006)

### Implementation for User Story 4

- [X] T056 [US4] `cmd/naeryeo/setup.go`에 `--delete=odsay|kakao|all`을 구현한다 — 기존 `logout.go`의 멱등 동작과 "삭제할 키가 없습니다" 구분을 그대로 옮기고, **설정 파일은 건드리지 않는다** (spec FR-007, FR-035)
- [X] T057 [US4] `cmd/naeryeo/setup.go`에 대화식 마법사 첫 화면의 3번 선택지("저장된 자격증명 삭제")를 구현한다 (spec FR-035 — 삭제 기능은 초기 설정 절차 안에서 제공된다)
- [X] T058 [US4] `cmd/naeryeo/logout.go`와 `cmd/naeryeo/logout_test.go`를 삭제하고, `cmd/naeryeo/main.go`의 `logout` 케이스(`main.go:149`)와 usage 문자열(`main.go:197`)을 갱신한다 (spec FR-035 — 삭제 전용 최상위 명령 제거)
- [X] T059 [US4] `cmd/naeryeo/setup.go`에 `--geocoder`를 구버전 형식(값 없는 bool)으로 호출했을 때 "이제 `--geocoder=kakao` 형태로 지정해야 합니다"라는 마이그레이션 안내를 출력하도록 한다 — 조용히 실패해서는 안 된다 (spec FR-036, contracts/cli-interface.md)
- [X] T060 [US4] `README.md`에 마이그레이션 안내 절을 추가한다 — "업그레이드 후 `naeryeo setup`을 한 번 다시 실행해야 합니다"와 그 이유, 저장된 키는 삭제되지 않고 재사용된다는 점, 그리고 FR-034가 요구하는 세 가지 breaking change(제공자 재설정 강제 / 삭제 전용 명령 제거 / 장소 검색 플래그 형태 변경) (spec FR-034)
- [X] T060a [US4] 릴리스 노트에 breaking change가 실리도록 보장한다. 이 저장소는 `@semantic-release/changelog`를 쓰지 않고 `release-notes-generator`가 **커밋 메시지에서** 노트를 생성하므로(`.releaserc.json`), CHANGELOG 파일을 만드는 대신 해당 변경을 담은 커밋에 `BREAKING CHANGE:` 푸터를 넣어야 한다 — FR-034의 세 항목 각각에 "무엇을 해야 하는가"를 한 줄씩 포함한다. T069의 커밋 메시지 제안에 이 푸터가 포함되어야 하며, commitlint를 통과하는지 확인한다 (spec FR-034, 헌법 원칙 IV)

**Checkpoint**: US4 완료 — 기존 사용자가 안내를 따라 1회 재설정으로 복구된다

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 여러 스토리에 걸치는 마무리와 문서 정합성

- [X] T061 [P] `skills/naeryeo/SKILL.md`를 개편한다 — 제공자 개념(setup에서 MOTIS/ODsay 선택, env var 언급 없음), 자체 호스팅 문서 링크, 에러 코드별 분기 안내(`docs` 필드가 있으면 사용자에게 그대로 전달), 에러 메시지 임의 재구성 금지 (spec FR-026)
- [X] T062 [P] `skills/naeryeo/SKILL.md`에 **AI가 사용자 동의 없이 자체 호스팅 환경을 설치·기동하지 않도록** 명시적 금지 문구를 추가한다 (spec FR-027)
- [X] T063 [P] `skills/naeryeo/SKILL.md`의 `naeryeo logout`(`:67`)·`naeryeo logout --geocoder`(`:81`) 언급을 제거하고 `setup --delete` 형식으로 교체한다
- [X] T064 [P] `specs/005-structured-output-contract/contracts/error-codes.md`의 "향후 확장" 절(`:70-75`)을 갱신해 `provider_not_configured`·`motis_*`·`docs`가 실현되었음을 명시하고 이 기능의 contracts/error-codes.md를 가리키게 한다
- [X] T065 provider × geocoder 4개 조합(motis×none, motis×kakao, odsay×none, odsay×kakao)이 모두 동작함을 검증하는 테스트를 추가한다 — 특히 **motis×none에서 역·정류장 이름 검색이 외부 호출 없이 동작**하는 것이 이 기능의 핵심 주장이다 (spec FR-030, SC-002, quickstart.md S6). 4개 조합 모두에 **동일한 인자**(`--from`, `--to`)를 주어 호출하고, 호출 형태가 제공자에 따라 달라지지 않음을 단언한다 (spec FR-012)
- [X] T066 quickstart.md S2를 수행한다 — 실 MOTIS로 대표 질의 3종(강남역→홍대입구역, 서면역→해운대역, 대전역→광주송정역)을 실행하고 결과를 기록한다. 실패하는 질의가 있으면 커버리지 한계로 `docs/self-hosting.md`에 명시한다 (spec SC-007, research.md U5 해소)
- [X] T067 3개 OS(linux/macOS/windows) CI에서 `just check`가 전부 green임을 확인한다 — 설정 파일 경로가 OS별로 갈리므로 이 매트릭스가 이번 기능의 실질적 게이트다 (GYE-296)
  - PR #12, 워크플로 실행 30799925162에서 확인: ubuntu-latest 1m16s / macos-latest 1m4s / windows-latest 2m19s **전부 pass**. `os.UserConfigDir()`가 OS마다 다른 경로를 주는데도 `internal/config` 테스트가 세 곳 모두에서 통과했다.
- [X] T068 spec.md의 SC-001~SC-011을 하나씩 대조해 미충족 항목이 없는지 확인하고 결과를 기록한다
- [X] T069 `just fmt` → `just lint` → `just test`를 실행해 전부 green임을 확인하고, 변경 diff와 Conventional Commits 형식의 커밋 메시지를 사람에게 제시한다 (헌법 원칙 III·IV — **확인 없이 커밋하지 않는다**)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 의존 없음 — 즉시 시작 가능
- **Foundational (Phase 2)**: Setup 완료에 의존 — **모든 User Story를 블록한다**
- **User Stories (Phase 3~6)**: Foundational 완료에 의존
- **Polish (Phase 7)**: 원하는 User Story들이 완료된 뒤

### User Story Dependencies

| Story | 선행 | 비고 |
| --- | --- | --- |
| US1 (P1) | Foundational | 다른 스토리에 의존하지 않음. **MVP** |
| US2 (P2) | Foundational | 코드상 US1에 의존하지 않으나, T040의 연결 절차를 검증하려면 US1이 있어야 실효적 |
| US3 (P3) | Foundational, US1 | MOTIS 클라이언트가 있어야 실패를 유발할 수 있음 |
| US4 (P4) | Foundational, US1 | setup 재설계(T030) 위에 `--delete`를 얹음. T059는 T031(플래그 타입 변경)이 선행되어야 함 |

### Within Each User Story

- 테스트를 먼저 작성하고 **실패를 확인한 뒤** 구현한다 (헌법 원칙 II)
- 클라이언트 → 배선 → 진입점 순서
- 스토리가 끝날 때마다 `just check`가 green이어야 다음 스토리로 넘어간다

### Parallel Opportunities

- **Phase 1**: T002, T003, T004 동시 진행 가능
- **Phase 2**: T008(설정 테스트)은 T005~T007과 다른 파일이므로 구현 직후 병렬 가능. T015는 T013 이후 독립
- **Phase 3**: T020~T024(테스트 5개)를 모두 병렬 작성 가능. 구현은 T025→T026→T027→T028→T029가 같은 파일이라 순차, T030~T032(setup)는 별개 파일이라 클라이언트 작업과 병렬 가능
- **Phase 4**: T036·T037(실측·조사)은 서로 독립. T041·T041a는 같은 파일을 건드리므로 순차
- **Phase 5**: T044~T047(테스트 5개, T045a 포함) 병렬 가능
- **Phase 6**: T051~T055(테스트 5개) 병렬 가능
- **Phase 7**: T061~T064 병렬 가능
- Foundational 완료 후 인원이 있다면 US1(클라이언트)과 US2(문서·실측)를 동시에 진행하는 것이 총 소요를 가장 크게 줄인다 — US2의 실측 작업은 코드와 무관하게 시간이 오래 걸린다

---

## Parallel Example: User Story 1

```bash
# 테스트 5개를 한꺼번에 작성 (전부 다른 파일):
Task: "internal/motis/client_test.go에 geocode+plan happy path 테스트"
Task: "internal/motis/client_test.go에 폴백·요금·no-route 테이블"   # 같은 파일 — 순차
Task: "cmd/naeryeo/setup_test.go에 마법사 fake stdin 테스트"
Task: "cmd/naeryeo/main_test.go에 route/mcp 제공자 일치 테스트"
Task: "cmd/naeryeo/routejson_test.go에 fareWon 부재 테스트"

# 구현 시 병렬 가능한 두 갈래:
Task: "internal/motis/client.go 구현 (T025~T029)"
Task: "cmd/naeryeo/setup.go 재작성 (T030~T032)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup 완료
2. Phase 2: Foundational 완료 — **T010에서 게이트 실패를 반드시 눈으로 확인**
3. Phase 3: User Story 1 완료
4. **정지 후 검증**: 상용 키 없는 환경에서 MOTIS만으로 경로 검색이 되는가
5. 이 시점에서 이미 "외부 사업자 정책에 종속되지 않는 naeryeo"라는 핵심 가치가 성립한다

### Incremental Delivery

1. Setup + Foundational → 기반 준비
2. US1 추가 → 독립 검증 → **MVP**
3. US2 추가 → 문서만으로 구축 가능해짐 → 실사용자 유입 가능
4. US3 추가 → 운영 중 실패가 진단 가능해짐
5. US4 추가 → 기존 사용자 전환 안전망 완성
6. Polish → 릴리스 준비 완료

**US2 없이 릴리스하지 말 것**: US1만으로는 "가능하지만 아무도 못 쓰는" 상태다. 자체 호스팅은
문서가 곧 기능이다.

### 실측 작업의 배치

T036·T037·T039·T050·T066은 실제로 MOTIS를 띄워야 풀린다. 코드 작업과 성격이 다르고 시간이
오래 걸리므로, **Foundational이 끝나는 즉시 착수해 US1 구현과 병렬로 진행**하는 것이 좋다.
이 작업들이 끝나지 않으면 `docs/self-hosting.md`를 완료로 볼 수 없다(spec FR-023).

---

## Commit Boundaries (헌법 원칙 II)

헌법은 "신규 exported 심볼은 **그것을 도입하는 같은 커밋**에 테스트를 동반한다"를 MUST로
규정한다. 아래 묶음은 **하나의 커밋**으로 만든다 — 구현 태스크와 테스트 태스크가 분리
번호를 갖는다고 해서 분리 커밋을 뜻하지 않는다.

| 커밋 | 태스크 | 도입되는 exported 심볼 |
| --- | --- | --- |
| C-1 | T005 – T008 | `config.Settings`, `config.RoutingProvider`, `config.GeocoderChoice`, `config.LoadSettings`, `config.SaveSettings` |
| C-2 | T009 – T015 | `core.ErrMotisUnavailable`, `core.ErrMotisRejected` |
| C-3 | T016 – T019 | `core.RouteResult.FareKnown`(필드), `RouteToolOutput.FareWon` 타입 변경 |
| C-4 | T020 – T021, T025 – T029 | `motis.Client`, `motis.NewClient`, `motis.Client.FindRoute` |
| C-5 | T022, T030 – T032 | (setup — exported 심볼 없음. 그래도 테스트와 함께 묶는다) |
| C-6 | T023 – T024, T033 – T034 | (배선 — exported 심볼 없음) |

**T010의 위치에 주의**: T010은 게이트가 실패하는 것을 **작업 트리에서** 확인하는 단계이며,
실패 상태를 커밋하지 않는다. C-2 커밋은 T014까지 끝나 게이트가 통과한 뒤에 만든다.
T010의 실패 출력은 커밋 메시지 본문이나 PR 설명에 인용해 SC-004가 검증되었음을 남긴다.

US2(문서)·US3·US4의 나머지 태스크는 exported 심볼을 도입하지 않으므로 논리적 묶음 단위로
자유롭게 커밋한다. 단 **어떤 커밋도 사람 확인 없이 만들지 않는다**(헌법 원칙 IV).

---

## Success Criteria 대조 기록 (T068, 2026-08-03)

실측 환경과 수치는 research.md R9~R13 참조.

| SC | 판정 | 근거 |
| --- | --- | --- |
| SC-001 상용 계정 0으로 검색 성공 | ✅ | `--provider=motis --geocoder=none` 설정으로 대표 질의 3종 성공 (R13) |
| SC-002 외부 유료 호출 0건 | ✅ | 지오코더 미설정 상태에서 역명 6개가 MOTIS 내장 색인으로 해석됨 (R13) |
| SC-003 문서만으로 구축 | ⚠️ | T043에서 독립 리뷰 에이전트로 검증·수정 완료(17건 중 16건 반영, 1건 실측 기각). 다만 "사람인 제3자" 조건은 미충족 |
| SC-004 착수 전 자원 파악 | ✅ | docs/self-hosting.md §2에 실측표(55초 / 3.98 GiB / 1.5 GB) |
| SC-005 두 진입점 제공자 불일치 0건 | ✅ | `newRouteFinder` 단일 경로 + T023 테스트 |
| SC-006 사설망 호스트·포트 노출 0건 | ✅ | 엔진 정지 후 실제 출력에서 `localhost`·`8080` 0건 (R13) |
| SC-007 대표 질의 집합 반환 | ✅ | 수도권·지방 광역시·도시 간 3종 전부 성공 (R13) |
| SC-008 기존 테스트 무수정 통과 | ✅ | `just check` green, 기존 테스트 수정 없음 |
| SC-009 출력 안내만으로 복구 | ✅ | 3개 신규 코드 모두 `docs` 링크 포함, 프로즈 3줄 실측 확인 |
| SC-010 초기 설정 1회 | ✅ | T051·T052 테스트 |
| SC-011 자체 호스팅·상용 동등 제시 | ✅ | README 제공자 표에서 대등 서술 |

**미충족 1건**: SC-003은 T043(제3자 구축 검증)에 종속된다. 문서는 완성되었고 자체 수행한
구축은 문서 절차만으로 성공했으나, "작성자가 아닌 사람"이라는 조건은 만족할 수 없다.

---

## Notes

- `[P]` = 다른 파일, 의존 없음
- `[Story]` 라벨로 태스크를 스토리에 추적 가능하게 연결
- 구현 전 테스트 실패를 확인할 것
- 각 태스크 또는 논리적 묶음마다 커밋하되, **사람 확인 없이 커밋하지 않는다**(헌법 원칙 IV)
- 어느 Checkpoint에서든 멈춰 해당 스토리를 독립 검증할 수 있다
- 기존 테스트를 **고쳐서** 통과시켜야 한다면 그것은 회귀다 — 설계를 다시 볼 것
