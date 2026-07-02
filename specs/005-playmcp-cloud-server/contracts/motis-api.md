# Contract: MOTIS 백엔드 소비 (internal/motis)

실측 기준: api.transitous.org (MOTIS, 한국 = KTDB GTFS 피드), 2026-07-03. 자체 호스팅 MOTIS도 동일 API를 서빙한다(같은 소프트웨어).

## 1. 지오코딩

```
GET {BASE}/api/v1/geocode?text={장소명}&language=ko
```

**응답** (JSON 배열, 실측):
```json
[
  {
    "type": "STOP",            // STOP | PLACE | ADDRESS
    "name": "강남역",
    "id": "kr-korea_BS_1100_121000097",
    "lat": 37.49993,
    "lon": 127.02632,
    "score": -34.65,
    "areas": [{"name": "서울특별시", "adminLevel": 4.0}, ...]
  },
  ...
]
```

**선택 규칙**: 배열 순서상 첫 번째 `type == "STOP"` 항목. STOP이 없으면 첫 항목. 빈 배열 → 장소 미해석(`ErrPointNotFound` 상당).

**"역" 접미사 폴백** (라이브 실측 2026-07-03): KTDB 피드는 정류장을 "홍대입구"처럼 접미사 없이 명명하는데 사용자는 "홍대입구역"으로 묻는다. 빈 결과 + 이름이 "역"으로 끝나면 접미사를 뗀 이름으로 1회 재시도한다("역" 단독 입력은 재시도하지 않음).

**주의**: 숫자가 `3.74999E1` 같은 지수 표기로 올 수 있음 — Go `encoding/json`의 float64 디코딩은 지수 표기를 기본 처리하므로 별도 대응 불필요.

## 2. 경로 계획

```
GET {BASE}/api/v3/plan?fromPlace={lat},{lon}&toPlace={lat},{lon}&numItineraries=1
```

**응답** (관심 필드만, 실측):
```json
{
  "itineraries": [
    {
      "duration": 2340,                    // 초
      "startTime": "2026-07-03T00:00:00Z", // UTC ISO8601
      "endTime":   "2026-07-03T00:39:00Z",
      "transfers": 1,
      "legs": [
        {
          "mode": "WALK",                  // WALK|SUBWAY|BUS|TRAM|RAIL|...
          "from": {"name": "START", "lat": ..., "lon": ...},
          "to":   {"name": "신논현", "stopId": "..."},
          "duration": 780,                 // 초
          "distance": 825.0                // m
        },
        ...
      ]
    }
  ]
}
```

**매핑 규칙** ([data-model.md](../data-model.md)):
- `itineraries` 빈 배열 → 경로 없음
- `itineraries[0]`만 사용 (`numItineraries=1`)
- leg의 `from.name == "START"` / `to.name == "END"`는 사용자 입력 장소명(geocode 결과 Place.Name)으로 치환
- mode 한국어 표기: WALK→도보, SUBWAY/TRAM/RAIL→지하철/전철, BUS→버스 (미지 mode는 원문 유지)

## 클라이언트 계약 (internal/motis.Client)

| 항목 | 계약 |
|---|---|
| 구성 | `Client{BaseURL string, HTTPClient *http.Client, Logger *slog.Logger}` — `internal/core.Client`와 동형 패턴 |
| 개별 호출 타임아웃 | 1,200ms (HTTPClient.Timeout) |
| `Geocode(ctx, name)` | → `Place{Name, Lat, Lon}` 또는 `ErrPointNotFound` 상당 / `ErrUpstreamUnavailable` 상당 |
| `Plan(ctx, from, to Place)` | → `core.RouteResult` 또는 경로없음 / upstream 에러 |
| `FindRoute(ctx, fromName, toName)` | Geocode×2 → Plan 조합. cmd 레이어가 쓰는 단일 진입 메서드 |
| 에러 | URL·상태코드는 로그 전용. 반환 에러는 core의 기존 분류 체계 재사용 |
| 로깅 | 호출당 1줄: 엔드포인트 종류, 상태코드, duration_ms (기존 core.doGet 패턴 준용) |

## 테스트로 고정할 항목 (httptest 가짜 MOTIS)

1. geocode: STOP 우선 선택 / STOP 없음 → 첫 항목 / 빈 배열 → 미해석 에러
2. plan: 실측 JSON 픽스처 → RouteResult(39분, 환승1, 단계 3+개) 매핑 정확성
3. START/END 치환, 초→분 반올림, mode 한국어 표기
4. 타임아웃·5xx·JSON 파손 → upstream 에러 분류
5. itineraries 빈 배열 → 경로 없음 분류
