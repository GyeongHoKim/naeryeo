# Contract: `find_transit_route` MCP Tool (클라우드 트랙)

PlayMCP 개발가이드(2026.06.12)가 심사 반려 기준이므로, 이 계약의 모든 항목은 테스트로 고정한다.

## Tool 메타데이터

| 항목 | 값 | 가이드 근거 |
|---|---|---|
| `name` | `find_transit_route` | `[A-Za-z0-9_-]` 1~128자, "kakao" 미포함 ✅ |
| `description` | `Finds a public transit route between two places in South Korea — subway, bus, and intercity — via naeryeo(내려). Give a departure and a destination as station, stop, or place names in Korean; returns total duration, transfers, and step-by-step directions.` | 영문, 서비스명 영·국문 병기, ≤1024자 ✅ |
| `annotations.title` | `Find Korean transit route` | 5종 전부 명시 필수 |
| `annotations.readOnlyHint` | `true` | 환경 변경 없음 |
| `annotations.destructiveHint` | `false` (명시) | 비파괴 |
| `annotations.idempotentHint` | `true` | 동일 입력 반복 무해 |
| `annotations.openWorldHint` | `true` (명시) | 외부 실데이터 조회 |
| `inputSchema` | `from: string (required)`, `to: string (required)` — 한국어 장소명 | 필수 property ✅ |

## 입력 검증

| 케이스 | 동작 |
|---|---|
| `from`/`to` 빈 문자열·공백만 | 도구 오류: `출발지와 도착지를 모두 알려주세요.` |
| 256자 초과 | 도구 오류: 과도한 입력 안내 |
| 정상 | 지오코딩 → 경로 검색 |

## 결과 (성공): 마크다운 TextContent 단일

```markdown
**강남역 → 홍대입구역** · 약 39분 · 환승 1회

1. 신논현역까지 도보 13분
2. 신논현에서 9호선 승차 → 당산 하차
3. 당산에서 2호선 환승 → 홍대입구 하차

_데이터: KTDB·OSM 기반 시간표 — 실시간 지연 미반영_
```

- 요금은 값이 있을 때만 `· 요금 1,500원` 추가 (KTDB 요금 부재 시 생략)
- 구조화 JSON(structuredContent)은 넣지 않는다 — "원본 API 응답 지양·최소 크기"
- 광고성 문구 금지

## 결과 (실패): 분류된 한국어 안내 (내부 정보 비노출)

| 분류 | 사용자 메시지 |
|---|---|
| 출발지 미해석 | `출발지 "{입력}"을(를) 찾지 못했어요. 역·정류장 이름으로 다시 시도해 주세요.` |
| 도착지 미해석 | (동일 패턴, 도착지) |
| 경로 없음 | `해당 구간의 대중교통 경로를 찾지 못했어요.` |
| 백엔드 장애·타임아웃 | `경로 서버가 일시적으로 응답하지 않아요. 잠시 후 다시 시도해 주세요.` |

금지: MOTIS URL, HTTP 상태코드, 스택 트레이스, Go 에러 체인 원문.

## 성능

- 핸들러 전체 데드라인 2,500ms (p99 3,000ms 예산 내)
- 초과 시 "백엔드 장애" 분류 메시지로 응답 (서버 생존)

## 테스트로 고정할 항목 (tasks에서 구현)

1. 도구 목록 조회 시 위 메타데이터 필드가 정확히 이 값으로 노출된다 (annotations 5종 존재 포함)
2. name/description에 대소문자 무관 "kakao" 부재를 문자열 검사로 고정
3. 성공 경로: 가짜 MOTIS → 마크다운 형식(제목·소요·환승·단계 목록) 검증
4. 실패 4분류 각각의 사용자 메시지, 내부 정보 문자열 부재 검증
5. 데드라인 초과 시나리오(가짜 MOTIS 지연) → 3s 내 오류 응답
