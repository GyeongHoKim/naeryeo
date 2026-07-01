# Contract: core Geocoder 인터페이스 & 폴백 (internal/core)

core가 소비하는 지오코더 인터페이스와, `FindRoute`의 정류장→지오코더 폴백 계약.

## 타입

```go
type Coordinate struct {
    X float64 // 경도(longitude)
    Y float64 // 위도(latitude)
}

type Geocoder interface {
    Resolve(ctx context.Context, query string) (Coordinate, error)
}
```

## Client 확장

```go
type Client struct {
    // ... 기존 필드 ...
    Geocoder Geocoder // 선택. nil이면 지오코더 폴백 비활성(기존 동작)
}
```

## Resolve 구현 계약(geocode 측이 지켜야 함)

| 상황 | 반환 |
|---|---|
| 최소 1건 매칭 | 대표 후보 1건의 `Coordinate`, `nil` |
| 매칭 0건 | `Coordinate{}`, `errStationNotFound`(core 내부 sentinel과 동등한 not-found 신호) |
| 인증 실패(HTTP 401/403) | `Coordinate{}`, `ErrGeocoderAuthFailed` |
| 타임아웃/네트워크/5xx/파싱 실패 | `Coordinate{}`, `ErrUpstreamUnavailable` |

> 참고: `errStationNotFound`는 core 비공개 sentinel이다. geocode 패키지가 이를 직접 반환할 수
> 없으므로, 실제로는 core 측 폴백 코드가 "geocode의 not-found 에러"를 `errStationNotFound`로
> 접는다. geocode는 이를 위한 공개 sentinel(예: `geocode.ErrNotFound`)을 노출하고, core가
> `errors.Is`로 판별해 매핑한다.

## FindRoute 폴백 계약

- from/to 각각에 대해: `searchStation` 실패(not found) 시 `Geocoder != nil`이면
  `Geocoder.Resolve` 호출 → 성공 좌표로 경로 검색 계속.
- `Geocoder.Resolve`가 not-found → 해당 Side의 `ErrPointNotFound`.
- `Geocoder.Resolve`가 `ErrGeocoderAuthFailed`/`ErrUpstreamUnavailable` → 그대로 전파.
- `Geocoder == nil`이면 기존과 100% 동일(회귀 없음).
- 정류장 검색이 성공하면 `Geocoder.Resolve`를 호출하지 않는다(FR-003).

## 테스트 계약(요지)

가짜 `Geocoder` 주입으로:
- 정류장 성공 → 지오코더 미호출(호출 카운트 0).
- 정류장 실패 + 지오코더 성공 → 정상 경로 결과.
- 정류장 실패 + 지오코더 not-found → `ErrPointNotFound{Side}`.
- 정류장 실패 + 지오코더 auth 실패 → `ErrGeocoderAuthFailed`.
- 정류장 실패 + `Geocoder==nil` → `ErrPointNotFound{Side}`(기존 동작).
- from은 정류장, to는 지오코더로 해석되는 혼합 입력 → 정상.
