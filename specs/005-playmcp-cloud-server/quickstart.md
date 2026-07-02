# Quickstart: PlayMCP Cloud MCP Server 검증 가이드

이 문서는 구현 완료 후 기능이 end-to-end로 동작함을 증명하는 실행 절차다. 계약 상세는 [contracts/](./contracts/), 데이터 매핑은 [data-model.md](./data-model.md) 참조.

## 사전 조건

- Go 1.26.4 (`mise install`)
- MOTIS 엔드포인트 1개 — 아래 중 하나:
  - 자체 호스팅 MOTIS (GYE-176 인프라 작업 산출물)
  - 임시 검증용: `https://api.transitous.org` (공용, 한국 피드 서빙 중 — 최종 제출은 자체 호스팅으로)

## 1. 품질 게이트 (필수 — 헌법 Principle III)

```bash
just check   # fmt + lint + test 전부 green이어야 함
```

기대: 기존 stdio/CLI 테스트 포함 전체 통과 (SC-004 회귀 0건).

## 2. HTTP 서버 기동

```bash
NAERYEO_MOTIS_URL=https://api.transitous.org go run ./cmd/naeryeo mcp --http
```

기대: `:8080` 리슨 로그. env 미설정으로 실행하면 `NAERYEO_MOTIS_URL`을 언급하는 에러와 함께 즉시 종료(FR-004).

```bash
curl -s http://localhost:8080/healthz   # → ok
```

## 3. MCP 왕복 검증 (MCP Inspector)

```bash
npx @modelcontextprotocol/inspector --transport http --server-url http://localhost:8080/
```

확인 항목 (SC-003):
- initialize 성공 (stateless — 세션 헤더 불요)
- tools/list에 `find_transit_route` 1건, annotations 5종 전부 표시
- tools/call `{from: "강남역", to: "홍대입구역"}` → 마크다운 경로 안내 (SC-001)
- tools/call `{from: "asdfqwer역", to: "강남역"}` → 장소 미해석 한국어 안내

## 4. 실패 격리 검증 (SC-006)

```bash
NAERYEO_MOTIS_URL=http://localhost:9  go run ./cmd/naeryeo mcp --http   # 죽은 백엔드
```

tools/call → 3초 이내 "경로 서버가 일시적으로 응답하지 않아요" 안내, 서버 프로세스 생존, `/healthz` 계속 200.

## 5. 컨테이너 검증 (SC-005)

```bash
docker build --platform linux/amd64 -t naeryeo-cloud .
docker run --rm -p 8080:8080 -e NAERYEO_MOTIS_URL=https://api.transitous.org naeryeo-cloud
curl -s http://localhost:8080/healthz   # → ok
```

기대: KC "Git 소스 빌드"와 동일 조건(루트 Dockerfile, amd64)으로 빌드·기동.

## 6. 지연 예산 검증 (SC-002)

```bash
for i in $(seq 1 20); do
  curl -s -o /dev/null -w "%{time_total}\n" -X POST http://localhost:8080/ \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_transit_route","arguments":{"from":"강남역","to":"홍대입구역"}}}'
done
```

기대: 전 호출 3.0s 미만 (자체 호스팅 MOTIS 기준; 공용 Transitous로는 참고치).

## 7. 배포 후 (범위 밖 절차의 진입점)

1. PlayMCP in KC → "+ 새 MCP 서버 등록" → Git 소스 빌드(이 저장소) → Endpoint URL 확보
2. PlayMCP 개발자 콘솔 → 등록 → "정보 불러오기" 성공 확인 → 임시 등록 → 도구함 테스트 → 심사 요청 (~7/7)
