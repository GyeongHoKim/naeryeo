# Contract: HTTP 진입점 (`naeryeo mcp --http`)

## 기동

| 항목 | 계약 |
|---|---|
| 커맨드 | `naeryeo mcp --http` (플래그 없으면 기존 stdio — 동작 불변) |
| 바인드 주소 | `--addr` > `:$PORT` > `:8080` 순 결정 |
| `NAERYEO_MOTIS_URL` | **필수**. 미설정 시 즉시 비0 종료 + 명확한 에러 메시지 (FR-004). 값은 스킴 포함 base URL (예: `https://motis.example.com`) |
| 로깅 | slog JSON을 stdout으로 (HTTP 모드는 stdio 충돌 없음). 요청별: 요청 ID, 도구명, 소요 ms, 성공/실패 |

## 엔드포인트

| 경로 | 계약 |
|---|---|
| `/` (모든 메서드) | MCP Streamable HTTP 핸들러 (`mcp.NewStreamableHTTPHandler`, `Stateless: true`, `JSONResponse: true`). PlayMCP "정보 불러오기"의 initialize 요청이 이 경로로 온다 |
| `GET /healthz` | `200 OK`, body `ok`. 의존성 무점검 liveness — MOTIS 장애와 무관하게 200 |

## 프로토콜

- MCP 버전 협상: go-sdk v1.6.1 기본 동작 (2025-03-26 ~ 2025-11-25 범위 포함)
- 세션: stateless — `Mcp-Session-Id` 미요구, 어떤 호출 순서로 와도 동작
- 사용자 인증 없음 (개인정보 미취급 — 가이드상 OAuth 불필요 조건)

## 수명주기

- SIGINT/SIGTERM → `http.Server.Shutdown` (진행 중 요청 드레인, 최대 5s)
- 도구 핸들러 패닉 → recover는 net/http 기본(고루틴 단위) — 프로세스 생존, 500 응답

## 테스트로 고정할 항목

1. `--http` + env 미설정 → 에러 종료 (메시지에 `NAERYEO_MOTIS_URL` 포함)
2. `/healthz` → 200 `ok`
3. httptest로 initialize → tools/list → tools/call 왕복 (stateless: 세션 헤더 없이)
4. stdio 경로 기존 테스트 전부 통과 (회귀 0)
