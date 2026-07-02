# Research: PlayMCP Cloud MCP Server

**Date**: 2026-07-03 · **Feature**: [spec.md](./spec.md)

모든 항목은 이 세션에서 1차 소스(공식 Notion 가이드, pkg.go.dev 문서, Transitous 프로덕션 API 실측)로 확인했다. NEEDS CLARIFICATION 잔여 0건.

## §1. Streamable HTTP 서버 — go-sdk v1.6.1로 충분한가

- **Decision**: 기존 의존성 `github.com/modelcontextprotocol/go-sdk v1.6.1`의 `mcp.NewStreamableHTTPHandler(getServer func(*http.Request) *mcp.Server, opts *mcp.StreamableHTTPOptions)`를 사용한다. 옵션은 `Stateless: true`(Mcp-Session-Id 미검증, 임시 세션), `JSONResponse: true`(application/json 응답 — 스펙 §2.1.5 허용 형식), `Logger: slog`.
- **Rationale**: PlayMCP 개발가이드 요구가 "Streamable HTTP만 + Remote만 + Stateless 권장"인데 SDK 옵션이 1:1로 대응한다. 신규 의존성 0개. 프로토콜 버전 협상(2025-03-26~2025-11-25)은 SDK가 처리 — v1.6.1은 이 범위의 스펙 리비전을 지원한다(`go doc`으로 로컬 모듈에서 확인).
- **Alternatives considered**: `mark3labs/mcp-go`(별도 의존성 추가, 이점 없음), 직접 JSON-RPC 구현(스펙 협상·수명주기 재구현 부담 — 기각).
- **주의**: 기본 활성인 localhost DNS rebinding 보호는 KC(원격 호스트) 환경에서 영향 없음. CrossOriginProtection은 서버-서버 호출(PlayMCP → KC)이라 불필요.

## §2. 도구 정의 — PlayMCP 개발가이드 매핑

- **Decision**: 도구명 `find_transit_route`(기존 stdio와 동일 — `[A-Za-z0-9_]` 충족, "kakao" 미포함). `mcp.Tool.Annotations = &mcp.ToolAnnotations{Title: "Find Korean transit route", ReadOnlyHint: true, DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: ptr(true)}` — 5종 전부 명시. Description(영문, "naeryeo(내려)" 병기, ≤1024자): "Finds a public transit route between two places in South Korea — subway, bus, and intercity — via naeryeo(내려). Give a departure and a destination as station, stop, or place names in Korean; returns total duration, transfers, and step-by-step directions."
- **Rationale**: `go doc`으로 v1.6.1의 `ToolAnnotations` 필드( DestructiveHint `*bool`, IdempotentHint `bool`, OpenWorldHint `*bool`, ReadOnlyHint, Title) 확인 — 가이드의 5종 힌트가 전부 표현 가능. 경로 검색은 읽기전용·비파괴·멱등·외부세계(실데이터) 상호작용이므로 위 값이 의미상 정확하다.
- **결과 형식**: `CallToolResult.Content = []TextContent{마크다운}`. 구조화 출력(auto JSON)은 가이드의 "API 응답 그대로 지양·최소 크기" 요구와 충돌하므로 클라우드 트랙에서 미사용. stdio 트랙은 기존 구조화 출력 유지.

## §3. MOTIS 백엔드 API — Transitous 프로덕션 실측 (2026-07-03)

- **Decision**: MOTIS v3/v1 REST API 소비. 두 엔드포인트만 사용:
  - `GET /api/v1/geocode?text={장소명}&language=ko` → JSON 배열. 각 항목: `type`(STOP/PLACE/ADDRESS), `name`, `lat`, `lon`, `score`, `areas[]`. **type=STOP 우선, 없으면 첫 항목** 선택.
  - `GET /api/v3/plan?fromPlace={lat},{lon}&toPlace={lat},{lon}&numItineraries=1` → `itineraries[]`: `duration`(초), `startTime/endTime`(ISO8601 UTC), `transfers`(int), `legs[]`: `mode`(WALK/SUBWAY/BUS/TRAM…), `from.name`, `to.name`, `duration`(초), `distance`(m).
- **Rationale/실측 근거**: `api.transitous.org`에 강남역→홍대입구역 질의 → 200 OK, 1.14s(EU 서버 왕복 포함), 39분·환승1회·신논현 도보 접근이라는 타당한 경로 반환. 지오코딩 "강남역" → type=STOP `강남역`(lat 37.4999, lon 127.0263) 1순위 반환. 한국 데이터(KTDB GTFS)가 이 API 형태로 실제 서빙됨을 확인.
- **Alternatives considered**: MOTIS GraphQL(불필요한 복잡도), 좌표 직접 입력만 받기(자연어 UX 파괴 — 기각), Kakao Local 지오코딩(외부 키·약관 관리 재발생 — 기본값에서 제외, 품질 미달 시 폴백으로만 재논의: spec Assumptions).
- **요금(Fare)**: KTDB GTFS에 요금 데이터가 없음 → RouteResult.Fare=0으로 두고 마크다운 렌더러가 0이면 요금 줄을 생략.

## §4. 타임아웃 예산 (p99 ≤ 3,000ms)

- **Decision**: 도구 핸들러에서 `context.WithTimeout(ctx, 2500ms)`. MOTIS HTTP 클라이언트 개별 호출 타임아웃 1,200ms. 호출 체인 = geocode(from) + geocode(to) + plan ≤ 3회.
- **Rationale**: 자체 VM(한국 리전)은 Transitous EU 실측(1.1s)보다 훨씬 빠를 것이나, 최악 경로(3회 순차 호출)도 2.5s 예산 안에 들어오도록 개별 상한을 둔다. 500ms는 직렬화·플랫폼 왕복 여유.
- **Alternatives considered**: geocode 병렬화(가능하나 1차 구현은 순차 — 예산 내 여유 확인 후 필요 시 병렬화. Principle I: 필요 전 최적화 기각).

## §5. 컨테이너 (KC "Git 소스 빌드")

- **Decision**: 저장소 루트 `Dockerfile`, 멀티스테이지: `golang:1.26` 빌드(`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`, `-trimpath -ldflags "-s -w -X main.version=..."`) → `gcr.io/distroless/static-debian12:nonroot` 런타임. `ENTRYPOINT ["/naeryeo", "mcp", "--http"]`. 포트는 `PORT` env(기본 8080) 수신.
- **Rationale**: KC 가이드 확인 사항 — Git 소스 빌드는 루트 Dockerfile을 그대로 빌드하고, 이미지는 linux/amd64여야 함(arm64 활성화 실패 명시). go-keyring의 OS 의존(dbus 등)은 CGO=0 정적 빌드에서 링크만 되고 클라우드 모드에서는 호출 경로가 없음. distroless/static은 ca-certificates·tzdata 포함(외부 TLS·Asia/Seoul 시간 표기 필요).
- **Alternatives considered**: scratch(ca-certificates 수동 복사 필요 — distroless가 더 단순), alpine(불필요한 셸·패키지 — 공격면 축소 우선).

## §6. 진입점/플래그 설계

- **Decision**: 기존 `naeryeo mcp` 서브커맨드에 `--http` 플래그 추가(플래그 없으면 기존 stdio 그대로). `--addr` 생략 시 `:$PORT`(기본 `:8080`). `NAERYEO_MOTIS_URL` 미설정 + `--http`면 기동 즉시 명확한 에러로 종료(FR-004).
- **Rationale**: Linear(GYE-170)에 기록된 방향과 일치, 서브커맨드 증식 없이 한 진입점에서 분기. stdio 경로는 코드 변경이 플래그 분기 한 줄 수준이라 회귀 위험 최소.
- **Alternatives considered**: 별도 `naeryeo serve` 서브커맨드(SKILL.md/문서 동기화 부담 증가), 별도 바이너리(빌드·릴리스 파이프라인 복잡화 — 기각).

## §7. 헬스체크·수명주기

- **Decision**: 같은 `http.ServeMux`에 `GET /healthz` → `200 ok`(의존성 무점검 liveness). MCP 핸들러는 `/`(KC가 발급하는 Endpoint URL 경로를 그대로 수용). SIGINT/SIGTERM에 `http.Server.Shutdown` graceful 종료.
- **Rationale**: PlayMCP "정보 불러오기"가 Endpoint URL로 직접 initialize를 호출하므로 MCP는 루트에 있어야 안전. liveness는 라우팅 백엔드 상태와 분리(백엔드 장애 시에도 서버는 살아서 정중한 도구 오류를 반환 — FR-009, SC-006).
