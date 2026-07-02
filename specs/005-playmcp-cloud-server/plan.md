# Implementation Plan: PlayMCP Cloud MCP Server (원격 Streamable HTTP 트랙)

**Branch**: `feature/play-mcp` | **Date**: 2026-07-03 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-playmcp-cloud-server/spec.md`

## Summary

naeryeo에 두 번째 MCP 진입점을 추가한다: PlayMCP in KC에 배포 가능한 **Streamable HTTP(stateless) MCP 서버**. 경로 검색은 기존 ODsay 경로(로컬 BYOK)와 분리된 **자체 호스팅 MOTIS 백엔드**(장소명 해석 = MOTIS 내장 지오코딩, 경로 = `/api/v3/plan`)를 사용한다. 도구 정의는 PlayMCP 개발가이드(annotations 5종, 영문 description + "naeryeo(내려)" 병기, 마크다운 결과, p99 3s)를 준수하고, 저장소 루트 Dockerfile로 KC "Git 소스 빌드"에서 바로 빌드·기동된다. 기존 stdio/CLI 경로는 변경하지 않는다.

## Technical Context

**Language/Version**: Go 1.26.4 (go.mod 고정)

**Primary Dependencies**:
- `github.com/modelcontextprotocol/go-sdk v1.6.1` (기존 의존성) — `mcp.NewStreamableHTTPHandler` + `StreamableHTTPOptions{Stateless: true, JSONResponse: true}` 제공 확인(research.md §1)
- 표준 라이브러리 `net/http`, `log/slog` — 신규 외부 의존성 **0개**

**Storage**: N/A (stateless — 키체인도 클라우드 모드에서는 미사용)

**Testing**: `go test -race ./...` (just test), 테이블 주도 + `httptest` 가짜 MOTIS 서버

**Target Platform**: linux/amd64 컨테이너 (PlayMCP in KC) / 로컬 macOS·Windows·Linux는 기존 트랙 그대로

**Project Type**: 단일 Go 모듈 CLI + 서버 (기존 구조 유지)

**Performance Goals**: 도구 호출 p99 ≤ 3,000ms (PlayMCP 필수). 도구 내부 예산: 전체 컨텍스트 데드라인 2,500ms(500ms 여유), MOTIS 개별 HTTP 호출 타임아웃 1,200ms

**Constraints**:
- Streamable HTTP만 허용 (SSE 단독/stdio 불가), stateless 권장 → `Stateless: true`
- MCP 프로토콜 2025-03-26 ~ 2025-11-25 (go-sdk v1.6.1이 이 범위를 협상함)
- 도구명 `[A-Za-z0-9_-]`, "kakao" 문자열 금지, annotations 5종 전부 명시
- 결과는 정제된 마크다운 텍스트 (구조화 JSON 원본 노출 금지)
- MOTIS 엔드포인트는 `NAERYEO_MOTIS_URL` env var (미설정 시 기동 실패)

**Scale/Scope**: 공모전 심사·투표 트래픽 (수백~수천 호출/일). 동시성은 stateless 핸들러 + Go HTTP 서버 기본 동시 처리로 충분

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|---|---|---|
| I. Idiomatic Go First | 신규 추상화는 `RouteFinder` 소비자 측 함수 타입 1개(기존 `findRoute` 클로저 패턴 연장). MOTIS 클라이언트는 기존 `core.Client`와 같은 구조(옵션 구조체 + 명시적 에러). 신규 외부 의존성 없음 | ✅ PASS |
| II. Unit Tests Are Mandatory | 신규 패키지 `internal/motis`는 `httptest` 기반 테이블 주도 테스트와 같은 커밋. HTTP 진입점(`buildHTTPMCPServer`, 마크다운 렌더러, env 파싱)도 동일 | ✅ PASS |
| III. Automated Quality Gates | 변경마다 `just fmt` → `just lint` → `just test`. tasks.md에 게이트 태스크 명시 예정 | ✅ PASS |
| IV. Commit Discipline | Conventional Commits, 인간 확인 후 커밋. tasks.md 마지막 단계에 명시 | ✅ PASS |

**Post-Phase-1 재점검**: 설계 산출물(계약 3건, data-model)에 새 추상화 추가 없음 — 위 판정 유지. 위반 없음 → Complexity Tracking 불필요.

## Project Structure

### Documentation (this feature)

```text
specs/005-playmcp-cloud-server/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── mcp-tool.md      # find_transit_route 도구 계약 (PlayMCP 가이드 준수 명세)
│   ├── http-server.md   # HTTP 진입점 계약 (엔드포인트, env, 헬스체크, 종료)
│   └── motis-api.md     # MOTIS 백엔드 소비 계약 (geocode/plan, 실측 응답 기반)
└── tasks.md             # Phase 2 output (/speckit-tasks — 이 커맨드가 만들지 않음)
```

### Source Code (repository root)

```text
cmd/naeryeo/
├── main.go              # [수정] "mcp" 서브커맨드에 --http/--addr 플래그 추가 분기
├── mcp.go               # [유지] stdio 조립 — 변경 없음 (회귀 0)
├── mcp_http.go          # [신규] HTTP 트랙: buildHTTPMCPServer(도구 정의+annotations),
│                        #        runMCPHTTP(http.Server, /healthz, graceful shutdown)
├── mcp_http_test.go     # [신규] 도구 메타데이터 규격, 핸들러 성공/실패, healthz
├── render.go            # [신규] core.RouteResult → 정제된 마크다운 렌더러
└── render_test.go       # [신규]

internal/motis/          # [신규 패키지] MOTIS 백엔드 클라이언트
├── doc.go
├── client.go            # Client{BaseURL, HTTPClient, Logger} —
│                        #   Geocode(ctx, name) (Place, error)
│                        #   Plan(ctx, from, to Place) (core.RouteResult, error)
│                        #   FindRoute(ctx, fromName, toName) — geocode×2 + plan 조합
└── client_test.go       # httptest 가짜 MOTIS로 테이블 주도

internal/core/           # [수정 최소화] 기존 ODsay 로직 그대로.
│                        # RouteResult/Step 도메인 모델을 motis가 재사용 (수정 없음)

Dockerfile               # [신규] 멀티스테이지: golang:1.26 빌드(CGO=0, amd64)
                         #        → distroless/static + ENTRYPOINT ["naeryeo","mcp","--http"]
```

**Structure Decision**: 기존 "진입점(cmd) ↔ 코어(internal)" 분리를 그대로 연장한다. 클라우드 트랙은 (a) `internal/motis` 신규 패키지 하나, (b) `cmd/naeryeo`의 HTTP 조립 파일 2개, (c) 루트 Dockerfile로 완결된다. `internal/core`는 도메인 모델 제공자로만 관여하며 ODsay 코드는 건드리지 않는다 — provider 추상화는 인터페이스 신설 대신 기존 `findRoute` 클로저 패턴(소비자 정의 함수 시그니처)을 재사용해 Principle I(투기적 일반화 금지)을 지킨다.

## 설계 핵심 결정 (요약 — 상세는 research.md)

1. **Transport**: `mcp.NewStreamableHTTPHandler(getServer, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})` — 가이드의 "Streamable HTTP + stateless 권장"에 SDK 옵션이 정확히 대응.
2. **도구 결과**: 구조화 출력(현 stdio 방식) 대신 **마크다운 TextContent** — 가이드 "정제된 텍스트, 원본 API 응답 지양" 준수. stdio 트랙은 기존 구조화 출력 유지(회귀 없음).
3. **지오코딩**: MOTIS `/api/v1/geocode` (type=STOP 우선 선택, 없으면 최상위 후보). 외부 키 불필요 — Kakao Local은 클라우드 트랙에서 미사용.
4. **타임아웃 예산**: 도구 핸들러 진입 시 `context.WithTimeout(2.5s)` → geocode×2 + plan 각 1.2s. 초과 시 사용자향 한국어 안내.
5. **에러 노출**: MOTIS URL·상태코드·스택은 로그에만. 사용자에게는 분류된 한국어 메시지(장소 없음/경로 없음/일시 장애).
6. **Dockerfile**: 멀티스테이지, `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`(KC amd64 필수), distroless/static + tzdata·ca-certificates. `PORT` env(기본 8080) 수신.
