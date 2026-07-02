# Data Model: PlayMCP Cloud MCP Server

**Date**: 2026-07-03 · **Feature**: [spec.md](./spec.md)

원칙: 도메인 모델은 기존 `internal/core`의 것을 그대로 재사용한다. 이 기능이 새로 도입하는 타입은 MOTIS 응답 디코딩용 내부 타입과 장소 1개뿐이다.

## 재사용 (internal/core — 수정 없음)

### RouteResult (기존)
경로 검색의 공용 결과. ODsay·MOTIS 두 백엔드가 동일 타입으로 수렴한다.

| 필드 | 타입 | 의미 | MOTIS 매핑 |
|---|---|---|---|
| `NoTravelNeeded` | bool | 출발≈도착, 이동 불필요 | 최상위 itinerary의 duration이 극소(도보 단일 leg < 60s)면 true — 1차 구현은 false 고정, ODsay -98 대응 케이스는 MOTIS에 없음 |
| `TotalTime` | int (분) | 총 소요 시간 | `itineraries[0].duration`(초) ÷ 60 반올림 |
| `TransferCount` | int | 환승 횟수 | `itineraries[0].transfers` |
| `Fare` | int (원) | 예상 요금 | KTDB GTFS에 요금 없음 → 0 (렌더러가 0이면 요금 줄 생략) |
| `Steps` | []Step | 구간별 안내 | `legs[]` 각각 → Step 1개 |

### Step (기존)
| 필드 | 타입 | 의미 | MOTIS 매핑 |
|---|---|---|---|
| `Description` | string | 사람이 읽는 구간 안내 | mode·구간명·시간으로 조립: 예) `홍대입구(2호선)에서 하차` / `강남역까지 도보 13분` |

### 기존 에러 계약 (internal/core — 재사용)
| 에러 | MOTIS 트랙에서의 발생 조건 |
|---|---|
| `*ErrPointNotFound{Side}` | geocode 결과 0건 (from/to 구분) |
| `ErrRouteNotFound` (동등물) | plan의 `itineraries`가 빈 배열 |
| `ErrUpstreamUnavailable` | MOTIS 타임아웃·5xx·연결 실패·JSON 파손 |

> 존재하지 않는 에러 타입이 있으면 core의 기존 에러 세트에 맞춰 tasks 단계에서 정확한 이름으로 정합(코드 확인 후). 원칙: **cmd 레이어는 에러 분류만 보고 한국어 안내문을 고른다** — 내부 정보 비노출(FR-009).

## 신규 (internal/motis)

### Place
지오코딩 결과로 확정된 위치. plan 호출의 입력.

| 필드 | 타입 | 의미 |
|---|---|---|
| `Name` | string | 해석된 표준 이름 (예: "강남역") |
| `Lat` | float64 | 위도 |
| `Lon` | float64 | 경도 |

**검증 규칙**: geocode 응답에서 `type == "STOP"` 항목 우선, 없으면 첫 항목. 응답 0건 → `*ErrPointNotFound`.

### MOTIS 응답 디코딩 타입 (비공개)
`geocodeMatch`, `planResponse`, `itinerary`, `leg` — [contracts/motis-api.md](./contracts/motis-api.md)의 실측 JSON 형태를 그대로 미러링하는 소문자 내부 구조체. 공개 API는 `Place`와 `core.RouteResult`만 노출한다.

## 상태 전이

없음 — 전 구간 stateless. 요청 1건 = geocode(from) → geocode(to) → plan → RouteResult 렌더링. 서버·세션·캐시 상태를 저장하지 않는다.
