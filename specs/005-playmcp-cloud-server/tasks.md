# Tasks: PlayMCP Cloud MCP Server (원격 Streamable HTTP 트랙)

**Input**: Design documents from `/specs/005-playmcp-cloud-server/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: 헌법 Principle II — 모든 신규 exported 심볼은 같은 커밋에 테이블 주도 테스트 동반. 아래 태스크의 "+ tests"는 선택이 아니라 필수다.

**Organization**: 유저 스토리별 그룹. US1만 완료해도 MVP(강남역→홍대입구역 e2e)가 성립한다.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: 신규 패키지 골격과 공용 테스트 픽스처

- [X] T001 [P] `internal/motis/doc.go` 생성 — 패키지 주석(클라우드 트랙 전용 MOTIS 백엔드 클라이언트, ODsay와의 관계, contracts/motis-api.md 참조)
- [X] T002 [P] contracts/motis-api.md의 실측 JSON을 테스트 픽스처로 저장: `internal/motis/testdata/geocode_gangnam.json`(STOP+PLACE 혼합 배열), `internal/motis/testdata/plan_gangnam_hongdae.json`(39분·환승1·legs 3+), `internal/motis/testdata/plan_empty.json`(itineraries 빈 배열)

## Phase 2: Foundational (모든 스토리의 전제)

**Purpose**: HTTP 호출 기반 — 이후 모든 스토리가 이 클라이언트 골격 위에 선다

- [X] T003 `internal/motis/client.go` — `Client{BaseURL, HTTPClient, Logger}` 구조체(기존 `core.Client` 패턴 준용), 개별 호출 타임아웃 1,200ms 기본값, 호출당 1줄 slog(엔드포인트·상태·duration_ms), 타임아웃/5xx/JSON 파손 → upstream 에러 분류. `internal/motis/client_test.go`에 httptest 기반 에러 분류 테이블 테스트 동반

**Checkpoint**: `just check` green — 이후 유저 스토리 착수 가능

## Phase 3: User Story 1 — PlayMCP 사용자의 자연어 경로 질의 (P1) 🎯 MVP

**Goal**: 원격 HTTP MCP 서버에 `find_transit_route(from, to)`를 호출하면 마크다운 경로 안내가 반환된다

**Independent Test**: quickstart §2·§3 — 서버 기동 후 MCP Inspector로 강남역→홍대입구역 호출, 마크다운 안내 확인

- [X] T004 [US1] `internal/motis/client.go`에 `Geocode(ctx, name) (Place, error)` — `/api/v1/geocode?text=&language=ko`, type=STOP 우선·없으면 첫 항목·빈 배열이면 미해석 에러 (contracts/motis-api.md §1). `Place{Name, Lat, Lon}` 정의 포함. client_test.go에 선택 규칙 테이블 테스트(STOP 우선/PLACE 폴백/빈 배열)
- [X] T005 [US1] `internal/motis/client.go`에 `Plan(ctx, from, to Place) (core.RouteResult, error)` — `/api/v3/plan?fromPlace=&toPlace=&numItineraries=1`, 매핑 규칙(초→분 반올림, transfers, legs→Steps, START/END를 Place.Name으로 치환, mode 한국어 표기, Fare=0) (data-model.md). plan 픽스처로 매핑 정확성 테이블 테스트
- [X] T006 [US1] `internal/motis/client.go`에 `FindRoute(ctx, fromName, toName) (core.RouteResult, error)` — Geocode×2 + Plan 조합, 에러 통과. 조합 시나리오 테스트(성공/출발 미해석/도착 미해석/경로 없음)
- [X] T007 [P] [US1] `cmd/naeryeo/render.go` — `renderRouteMarkdown(from, to string, r core.RouteResult) string`: contracts/mcp-tool.md 형식(볼드 제목·소요·환승, 번호 목록 단계, 요금 0이면 생략, 데이터 출처 각주). `cmd/naeryeo/render_test.go` 형식 검증 테이블 테스트
- [X] T008 [US1] `cmd/naeryeo/mcp_http.go` — `buildHTTPMCPServer(version, logger, finder)`: contracts/mcp-tool.md의 메타데이터(영문 description + "naeryeo(내려)", annotations 5종 명시) 그대로 도구 등록, 핸들러는 `context.WithTimeout(2.5s)` + finder 호출 + 마크다운 TextContent 반환(구조화 출력 없음). `cmd/naeryeo/mcp_http_test.go`에 가짜 finder로 성공 경로 테스트
- [X] T009 [US1] `cmd/naeryeo/mcp_http.go`에 `runMCPHTTP` — `mcp.NewStreamableHTTPHandler(..., &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: ...})`를 `/`에, `GET /healthz`→`200 ok`를 같은 mux에, SIGINT/SIGTERM graceful shutdown(5s). env 계약(contracts/http-server.md): `NAERYEO_MOTIS_URL` 필수(미설정 시 에러 반환), addr = `--addr` > `:$PORT` > `:8080`. 테스트: env 미설정 에러 메시지, healthz 200, httptest로 initialize→tools/list→tools/call 왕복
- [X] T010 [US1] `cmd/naeryeo/main.go` — `mcp` 서브커맨드에 `--http`/`--addr` 플래그 분기 추가(플래그 없으면 기존 stdio 경로 그대로). `cmd/naeryeo/main_test.go`에 분기 테스트 추가
- [X] T011 [US1] quickstart §1~§3 수동 검증: `just check` + `NAERYEO_MOTIS_URL=https://api.transitous.org`로 기동 + MCP Inspector로 강남역→홍대입구역 마크다운 안내 확인 (SC-001)

**Checkpoint**: US1 = 배포 가능한 MVP

## Phase 4: User Story 2 — 장소명 해석 견고화 (P2)

**Goal**: 다양한 표기를 흡수하고, 해석 실패를 어느 쪽 장소인지 명시한 한국어 안내로 반환

**Independent Test**: 축약형("강남")·미해석("asdfqwer역") 입력에 대한 응답 확인

- [X] T012 [US2] `cmd/naeryeo/mcp_http.go`에 에러→사용자 메시지 매퍼 — contracts/mcp-tool.md 실패 4분류(출발 미해석/도착 미해석/경로 없음/백엔드 장애), 내부 정보(URL·상태코드·에러 체인 원문) 불포함. mcp_http_test.go에 4분류 각각 + 내부 문자열 부재 검증
- [X] T013 [P] [US2] 입력 검증 — 빈 문자열·공백만·256자 초과를 도구 오류로 (contracts/mcp-tool.md). 핸들러 테스트 추가
- [X] T014 [P] [US2] `internal/motis/client_test.go`에 축약형 해석 acceptance 픽스처 추가 — "강남" geocode 응답(다중 후보)에서 STOP 최상위 선택 확인 (spec US2-AS1)

**Checkpoint**: 자연어 입력의 현실 케이스 흡수 완료

## Phase 5: User Story 3 — 심사·운영 관점의 안정 구동 (P3)

**Goal**: PlayMCP 심사 기준(메타데이터·지연·장애 격리) 기계 검증 + 기존 트랙 회귀 0

**Independent Test**: quickstart §3(Inspector 점검)·§4(장애 격리)·§5(컨테이너)

- [X] T015 [P] [US3] `cmd/naeryeo/mcp_http_test.go`에 가이드 규격 고정 테스트 — tools/list 결과에서: annotations 5종 전부 존재, name·description 대소문자 무관 "kakao" 부재, description ≤1024자 + "naeryeo(내려)" 포함, 툴명 `[A-Za-z0-9_-]` 매치 (SC-003)
- [X] T016 [US3] 장애 격리 테스트 — 지연(3s+ sleep)·연결 거부 가짜 MOTIS에서: 3초 이내 "일시적으로 응답하지 않아요" 안내, 서버 프로세스 생존, `/healthz` 계속 200 (SC-006)
- [X] T017 [P] [US3] 구조적 로깅 검증 — 도구 호출당 1줄(tool·from·to·outcome·duration_ms)이 slog로 남는지 로그 캡처 테스트 (FR-010; 기존 stdio 핸들러의 로깅 패턴 준용)
- [X] T018 [US3] `Dockerfile` (저장소 루트) — research.md §5: golang:1.26 멀티스테이지, `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 -trimpath -ldflags "-s -w -X main.version=..."`, `gcr.io/distroless/static-debian12:nonroot`, `ENTRYPOINT ["/naeryeo","mcp","--http"]`, EXPOSE 8080. 검증: `docker build --platform linux/amd64` + 컨테이너 기동 + healthz (quickstart §5, SC-005)
- [X] T019 [US3] stdio·CLI 회귀 확인 — `just test` 전체 green + `git diff`로 `cmd/naeryeo/mcp.go`·`internal/core/` ODsay 경로 무변경 확인 (SC-004). 변경이 불가피했다면 사유를 PR 설명에 기록

**Checkpoint**: 심사 요청 가능한 상태

## Phase 6: Polish & Cross-Cutting

- [X] T020 [P] `README.md`에 클라우드 트랙 섹션 추가 — 두 트랙 구분(로컬 BYOK vs PlayMCP), `naeryeo mcp --http` 사용법, `NAERYEO_MOTIS_URL`/`PORT` env 표
- [X] T021 [P] `skills/naeryeo/SKILL.md` 동기화 점검 — CLI 인터페이스 변화(`mcp --http` 플래그) 반영 필요 여부 확인 후 갱신 (CLAUDE.md: SKILL.md 수동 동기화 규칙)
- [X] T022 quickstart 전체(§1~§6) 최종 실행 — 게이트 3종 green, Inspector 왕복, 장애 격리, 컨테이너, 지연 참고치 기록
- [ ] T023 Conventional Commits 메시지 제안 + 인간 확인 후 커밋 (헌법 Principle IV — 에이전트 단독 커밋 금지)

## Dependencies

```text
Setup:        T001, T002                  (병렬)
Foundational: T003                        (T002 픽스처 사용)
US1:          T004 → T005 → T006          (client.go 순차)
              T007                        (T004~6과 병렬 가능 — 다른 파일)
              T008 → T009 → T010          (mcp_http.go 순차, T006·T007 완료 후)
              T011                        (US1 전체 후)
US2:          T012 → T013                 (T009 이후) / T014는 T004 이후 병렬
US3:          T015·T016·T017              (T009 이후, 상호 병렬)
              T018                        (T010 이후) / T019 (전체 후)
Polish:       T020·T021 병렬 → T022 → T023
```

## Parallel Execution Examples

- **Setup**: T001 ∥ T002
- **US1**: T007(render.go) ∥ T004~T006(motis client) — 서로 다른 파일
- **US3**: T015 ∥ T016 ∥ T017 — 전부 테스트 파일 추가
- **Polish**: T020(README) ∥ T021(SKILL.md)

## Implementation Strategy

1. **MVP 우선**: Phase 1→2→3 완주 시 배포 가능한 최소 제출물 (T011로 증명)
2. **증분 전달**: US2(견고화)·US3(심사 규격)는 MVP 위에 독립 증분 — 시간 압박 시 US3의 T015·T018(심사 직결)을 US2보다 먼저 당길 수 있음
3. **매 태스크 후**: `just check` (헌법 Principle III — 실패 게이트는 완료 차단)
4. **커밋**: 전 단계 완료 + 인간 확인 후에만 (T023)
