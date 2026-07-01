---
description: "Task list for 건물명·주소(POI) 출발지/도착지 지원"
---

# Tasks: 건물명·주소(POI) 출발지/도착지 지원

**Input**: Design documents from `/specs/004-poi-address-search/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: 프로젝트 헌법(Principle II: Unit Tests Are Mandatory)에 따라 모든 사용자 스토리에
테스트 task가 **필수**다. 변경/신규 exported 심볼은 같은 커밋에 테이블 기반 테스트를 동반한다.

**Organization**: 사용자 스토리 단위로 그룹화해 각 스토리를 독립적으로 구현·검증할 수 있게 한다.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 병렬 가능(다른 파일, 미완료 task에 의존 없음)
- **[Story]**: US1/US2/US3 — 해당 사용자 스토리
- 각 task에 정확한 파일 경로 포함

## Path Conventions

기존 단일 프로젝트 구조: `cmd/naeryeo/`, `internal/config/`, `internal/core/`, 신규
`internal/geocode/`, 루트 `README.md`. (plan.md의 소스 구조 기준.)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 프로젝트는 이미 스캐폴딩됨. 이 기능은 새 서드파티 의존 없이 표준 라이브러리
(`net/http`+`encoding/json`)와 기존 `go-keyring`만 사용한다.

- [ ] T001 [P] go.mod 확인: Kakao 클라이언트는 표준 라이브러리로 구현하므로 신규 의존 추가가
      없음을 확인(필요 시 `go mod tidy`만 실행). 신규 패키지 디렉터리 `internal/geocode/` 생성
      및 `internal/geocode/doc.go`에 패키지 주석 작성.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: `internal/config`를 자격증명 기반으로 파라미터화한다. 이는 US1(지오코더 키 로드),
US2(지오코더 키 저장), 그리고 기존 호출부 컴파일을 모두 좌우하는 선행 리팩터다.

**⚠️ CRITICAL**: 이 phase 완료 전에는 어떤 사용자 스토리도 시작할 수 없다.

- [ ] T002 `internal/config/config.go`에 `Credential` 정의 타입과 상수 `ODsayAPIKey`
      (`"odsay-api-key"`, 기존 `keyUsername` 값 그대로), `GeocoderAPIKey`(`"geocoder-api-key"`)를
      추가하고, `Save`/`Load`/`Delete` 시그니처를 `(cred Credential, ...)`로 파라미터화. 키체인
      username을 cred로 사용(서비스명 `naeryeo` 유지). (contracts/config-credential.md)
- [ ] T003 `internal/config/config_test.go` 갱신: 자격증명별 Save→Load 왕복, 두 자격증명 상호
      독립성(하나 저장이 다른 하나에 영향 없음), Delete 멱등성, 빈 키 거부, fake backend로
      keychain-unavailable 경로, `ODsayAPIKey == "odsay-api-key"` 회귀(기존 저장분 호환) 검증.
- [ ] T004 기존 호출부를 새 config 시그니처에 맞춰 갱신하되 **동작 변경 없이**
      `config.ODsayAPIKey`를 바인딩: `cmd/naeryeo/main.go`의 `config.Load`/`config.Save`/
      `config.Delete` 배선을 ODsay 자격증명으로 감싸 `runSetup`/`runLogout`/`runRoute`/
      `buildMCPServer`에 전달(각 함수 시그니처는 이 단계에서 유지). (cmd/naeryeo/main.go)
- [ ] T005 `just check`(fmt+lint+test) 그린 확인 — 자격증명 리팩터 후 기존 기능 회귀 0
      (역/정류장 검색·setup·logout·mcp 동작 불변).

**Checkpoint**: config 자격증명 기반 완성 — 사용자 스토리 구현 시작 가능.

---

## Phase 3: User Story 1 - 건물명·주소로 경로 검색 (Priority: P1) 🎯 MVP

**Goal**: 정류장으로 해석되지 않는 건물명·주소를, 지오코더 키가 있을 때 좌표로 해석해 경로를
반환한다(핵심 가치). core는 소비자 정의 `Geocoder`를 선택적 주입받아 정류장 검색 실패 시 폴백.

**Independent Test**: `internal/core.FindRoute`에 가짜 `Geocoder`를 주입하고, 정류장 미검색
+ 지오코더 성공 조합에서 정상 경로 결과가 나오는지 확인. `geocode.Kakao`는 `httptest`로 Kakao
응답을 흉내 내어 좌표 해석을 검증.

### Tests for User Story 1 (MANDATORY per Constitution Principle II) ⚠️

> 구현 전에 작성하고 FAIL을 확인한 뒤 구현으로 넘어간다.

- [ ] T006 [P] [US1] `internal/geocode/kakao_test.go`: `httptest.Server`로 Kakao 키워드 검색
      응답 분기 테이블 테스트 — 1건/다건→첫 건/0건→`ErrNotFound`/401·403→`ErrAuthFailed`/
      500→`ErrUnavailable`/깨진 JSON→`ErrUnavailable`. `Authorization: KakaoAK <key>` 헤더 형식과
      `query` URL 인코딩 전송 검증. (contracts/geocode-kakao.md)
- [ ] T007 [P] [US1] `internal/core/client_test.go`: 가짜 `Geocoder` 주입 폴백 테스트 —
      정류장 성공→지오코더 미호출(호출 카운트 0), 정류장 실패+지오코더 성공→경로 결과,
      +`geocode.ErrNotFound`→`ErrPointNotFound{Side}`, +`geocode.ErrAuthFailed`→
      `ErrGeocoderAuthFailed`, +`Geocoder==nil`→`ErrPointNotFound{Side}`(기존 동작),
      from=정류장·to=지오코더 혼합 입력→정상. (contracts/core-geocoder.md)

### Implementation for User Story 1

- [ ] T008 [P] [US1] `internal/geocode/errors.go`: 공개 sentinel `ErrNotFound`/`ErrAuthFailed`/
      `ErrUnavailable` 정의. (data-model.md §5)
- [ ] T009 [US1] `internal/core`에 `Coordinate{X,Y float64}` 타입, `Geocoder` 소비자 인터페이스
      (`Resolve(ctx, query) (Coordinate, error)`)를 `client.go`에, `ErrGeocoderAuthFailed`
      sentinel을 `errors.go`에 추가. (contracts/core-geocoder.md, data-model.md §2·§3·§5)
- [ ] T010 [US1] `internal/geocode/kakao.go`: `NewKakao(apiKey)`(+ 테스트용 `BaseURL`/`HTTPClient`
      필드), `Resolve` 구현 — Kakao 키워드 검색 GET(`size=1`,`sort=accuracy`), `documents[0].x/y`
      `ParseFloat`→`core.Coordinate`, §3 에러 매핑, context 전파·타임아웃, **API 키 무로깅**.
      (depends: T008, T009; contracts/geocode-kakao.md)
- [ ] T011 [US1] `internal/core/client.go`: `Client.Geocoder Geocoder` 필드 추가 +
      `resolveStation` 폴백 분기 — `errStationNotFound`이고 `Geocoder!=nil`이면 `Resolve` 호출,
      반환 `Coordinate`를 `stationCandidate{X: flexibleFloat(...), Y: flexibleFloat(...)}`로 변환,
      `geocode.ErrNotFound`→`errStationNotFound`, `ErrAuthFailed`→`ErrGeocoderAuthFailed`,
      `ErrUnavailable`→`ErrUpstreamUnavailable`로 접기. from/to 독립 적용. (depends: T009;
      data-model.md §4)
- [ ] T012 [US1] `cmd/naeryeo/main.go`: route·mcp의 `findRoute` 클로저 내부에서
      `config.Load(config.GeocoderAPIKey)` 조회 → 키가 있으면 `geocode.NewKakao(gk)`를
      `core.Client.Geocoder`에 주입, `ErrNotConfigured`면 주입 생략. `findRoute` 클로저 타입은
      불변. (depends: T010, T011; contracts/cli.md "cmd 배선")

**Checkpoint**: 지오코더 키가 키체인에 있으면 건물명·주소가 경로로 해석됨(SC-001). 정류장
입력은 지오코더 미호출로 기존과 동일(SC-002/SC-003, FR-003).

---

## Phase 4: User Story 2 - 장소 검색 API 키 설정 (Priority: P1)

**Goal**: `setup --geocoder`로 Kakao 키를 등록하고 `logout --geocoder`로 삭제한다. ODsay 키와
독립된 키체인 항목으로 관리.

**Independent Test**: `setup --geocoder`가 `GeocoderAPIKey` 자격증명으로 저장하고, 플래그가
없으면 `ODsayAPIKey`로 저장하는지 fake save/load/del로 검증. `logout --geocoder`도 대칭 확인.

### Tests for User Story 2 (MANDATORY per Constitution Principle II) ⚠️

- [ ] T013 [P] [US2] `cmd/naeryeo/setup_test.go`: `--geocoder` 유무에 따라 저장 대상 자격증명이
      `GeocoderAPIKey`/`ODsayAPIKey`로 갈리는지, `--geocoder`일 때 프롬프트가 "Kakao REST API
      Key: "인지 검증. (contracts/cli.md)
- [ ] T014 [P] [US2] `cmd/naeryeo/logout_test.go`: `--geocoder` 유무에 따라 load/delete 대상
      자격증명이 갈리는지, "삭제함"/"삭제할 키 없음" 문구 유지 검증.

### Implementation for User Story 2

- [ ] T015 [US2] `cmd/naeryeo/setup.go`: 첫 인자(`args []string`)를 `flag.FlagSet`으로 파싱해
      `--geocoder` 처리, 대상 `config.Credential`과 프롬프트 문구 분기. `runSetup` 시그니처를
      자격증명 인지형(`save func(config.Credential, string) error`)으로 조정하고 `main.go`가
      `config.Save`를 직접 전달하도록 배선 갱신. (cmd/naeryeo/setup.go, cmd/naeryeo/main.go)
- [ ] T016 [US2] `cmd/naeryeo/logout.go`: `--geocoder` 플래그 파싱, load/delete 대상 자격증명
      분기. `runLogout` 시그니처를 자격증명 인지형으로 조정하고 `main.go` 배선 갱신.
      (cmd/naeryeo/logout.go, cmd/naeryeo/main.go)

**Checkpoint**: 사용자가 지오코더 키를 독립적으로 등록·삭제 가능. US1과 결합 시 건물명 검색이
실제 키로 동작.

---

## Phase 5: User Story 3 - 해석 실패·미설정 안내 (Priority: P2)

**Goal**: 지오코더 미설정 시 FR-007 힌트를, 인증 실패 시 구분되는 안내를 제공하고, CLI·MCP가
동일한 문구를 낸다(FR-011).

**Independent Test**: `routeErrorMessage`를 직접 단위 호출해 `(ErrPointNotFound, false)`→힌트
포함, `(ErrPointNotFound, true)`→힌트 없음, `(ErrGeocoderAuthFailed, _)`→인증 실패 문구를 확인.
route/mcp 진입점이 `loadGeocoder`로 설정 여부를 계산해 같은 문구를 내는지 검증.

### Tests for User Story 3 (MANDATORY per Constitution Principle II) ⚠️

- [ ] T017 [P] [US3] `cmd/naeryeo/route_test.go`: `routeErrorMessage(err, geocoderConfigured)`
      단위 테스트 — 세 조합(힌트 포함/미포함/인증 실패 문구). fake load/loadGeocoder(키 있음·없음)
      + fake findRoute(`ErrPointNotFound`/`ErrGeocoderAuthFailed`)로 route 출력 검증.
- [ ] T018 [P] [US3] `cmd/naeryeo/mcp_test.go`: MCP 도구 핸들러가 `loadGeocoder` 반영해 route와
      **동일한** 힌트/인증 실패 문구를 반환하는지 검증(FR-011 문구 통일).

### Implementation for User Story 3

- [ ] T019 [US3] `routeErrorMessage`를 `(err error, geocoderConfigured bool) string`으로 확장 —
      `ErrPointNotFound`+`!geocoderConfigured`면 FR-007 힌트("`naeryeo setup --geocoder` ...")
      추가, `ErrGeocoderAuthFailed`면 인증 실패 문구. `runRoute`와 `routeToolHandler`/
      `buildMCPServer`에 `loadGeocoder func() (string, error)` 파라미터 추가, 에러 경로에서만
      `geocoderConfigured` 계산 후 `routeErrorMessage`에 전달. `main.go`가
      `config.Load(config.GeocoderAPIKey)` 클로저를 전달. (cmd/naeryeo/route.go,
      cmd/naeryeo/mcp.go, cmd/naeryeo/main.go; contracts/cli.md "cmd 배선")

**Checkpoint**: 미설정·미해석·인증 실패가 서로 구분되는 안내로 전달됨(SC-004).

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 사용자 문서·리스크 검증·최종 게이트.

- [ ] T020 [P] `README.md` 갱신: 지오코딩 필요성·선택성·설정 절차(신규 하위 섹션), 명령어 표에
      `setup/logout --geocoder` 행, 아키텍처에 `internal/geocode`, **"사용 API" 섹션 정정**
      (경로 검색=ODsay / 지오코딩=Kakao 역할 구분으로 모순 제거). (contracts/docs-readme.md)
- [ ] T021 [US1] **리스크 검증** — 실제 ODsay 키로 `searchStation` 무매칭 응답 형태(0건 vs error
      code 3/4/5)를 실측. 코드 3/4/5가 사용되면 `resolveStation`이 정류장 not-found 계열을
      `errStationNotFound`로 정규화해 지오코더 폴백 진입을 보장하도록 보완하고 회귀 테스트 추가.
      (internal/core/client.go, internal/core/client_test.go; research.md §3 리스크)
- [ ] T022 `quickstart.md`의 6개 시나리오 종단 검증(키 등록→건물명 검색→회귀→미설정 힌트→인증
      실패→`just check`).
- [ ] T023 `just check` 최종 그린 + 커버리지 회귀 없음 확인(Constitution Principle II·III).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 의존 없음 — 즉시 시작.
- **Foundational (Phase 2)**: Setup 완료 후 — **모든 사용자 스토리를 차단**.
- **User Stories (Phase 3~5)**: 모두 Foundational 완료에 의존.
  - US1(P1)과 US2(P1)는 서로 독립적으로 진행 가능(다른 파일군: US1=core/geocode/main 클로저,
    US2=setup/logout).
  - US3(P2)는 US1의 core 에러(`ErrGeocoderAuthFailed`, T009)와 route/mcp 배선에 의존하므로 US1
    이후 진행 권장.
- **Polish (Phase 6)**: 원하는 스토리 완료 후.

### User Story Dependencies

- **US1 (P1)**: Foundational 후 시작. 다른 스토리에 의존 없음(가짜 Geocoder로 독립 테스트).
- **US2 (P1)**: Foundational 후 시작. US1과 독립(키 저장/삭제만).
- **US3 (P2)**: Foundational 후 시작 가능하나 US1의 `ErrGeocoderAuthFailed`·route/mcp 구조에
  의존 → 실질적으로 US1 뒤.

### Within Each User Story

- 테스트 먼저 작성·FAIL 확인 후 구현.
- geocode/core 타입·에러(T008, T009) → 구현체(T010) → core 폴백(T011) → cmd 배선(T012).
- US2: 테스트(T013,T014) → 구현(T015,T016).
- US3: 테스트(T017,T018) → 구현(T019).

### Parallel Opportunities

- **Foundational**: T002→T003는 같은 패키지라 순차, T004는 T002 이후.
- **US1 테스트**: T006, T007 병렬(다른 파일).
- **US1 구현**: T008 [P]는 T009와 다른 패키지라 병렬 가능. T010은 T008·T009 후, T011은 T009 후
  (T010·T011은 서로 다른 파일이나 각각 선행 의존이 있어 조건부 병렬), T012는 T010·T011 후.
- **US2**: T013, T014 병렬. T015, T016 병렬(다른 파일; 단 둘 다 main.go 배선을 건드리면 main.go는
  순차 병합).
- **US3**: T017, T018 병렬.
- **Polish**: T020(README) [P]는 코드와 독립.
- **스토리 간**: Foundational 후 US1과 US2를 서로 다른 담당자가 병렬 진행 가능.

---

## Parallel Example: User Story 1

```bash
# US1 테스트를 함께 작성(다른 파일):
Task: "internal/geocode/kakao_test.go — Kakao 응답 분기 테이블 테스트"
Task: "internal/core/client_test.go — 가짜 Geocoder 폴백 테스트"

# US1 타입/에러 스캐폴드를 병렬로:
Task: "internal/geocode/errors.go — 공개 sentinel 3종"
Task: "internal/core: Coordinate·Geocoder·ErrGeocoderAuthFailed"  # client.go/errors.go
```

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1 Setup → Phase 2 Foundational(config 자격증명) 완료.
2. Phase 3 US1 완료 → 지오코더 키가 있을 때 건물명·주소 검색 동작(핵심 가치).
3. **STOP & VALIDATE**: 가짜 Geocoder + httptest로 US1 독립 검증.

### Incremental Delivery

1. Setup + Foundational → 기반 완성(회귀 0 확인, T005).
2. US1 → 건물명 라우팅(MVP). 3. US2 → 키 등록/삭제 UX. 4. US3 → 안내 문구.
5. Polish → README·리스크 검증·최종 게이트.

### Notes

- [P] = 다른 파일, 의존 없음. `main.go` 배선은 여러 task가 건드리므로 순차 병합 주의.
- 각 task 또는 논리 그룹 완료 후 `just fmt`/`lint`/`test` 통과 시 커밋(인간 확인 후, 헌법
  Principle IV).
- 변경/신규 exported 심볼은 같은 커밋에 테스트 동반, 커버리지 회귀 금지.
