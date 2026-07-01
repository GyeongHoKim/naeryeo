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

## 에러 계약 sentinel (core 소유)

`Geocoder`의 에러 계약은 core가 소유한다. 구현체는 core를 단방향 import해 이 sentinel들을
반환하고, core는 구현 패키지를 import하지 않아 순환이 없다.

```go
// internal/core/errors.go
var (
    ErrGeocoderNotFound    = errors.New("core: geocoder found no matching place")
    ErrGeocoderAuthFailed  = errors.New("core: geocoder rejected the API key")   // HTTP 401
    ErrGeocoderForbidden   = errors.New("core: geocoder denied the request")     // HTTP 403
    ErrGeocoderUnavailable = errors.New("core: geocoder is unavailable")
)
```

> **401 vs 403**: 401은 키 자체가 무효/만료 → 재등록이 해법(`ErrGeocoderAuthFailed`). 403은
> 키는 유효하나 요청이 거부됨(앱에 지도/로컬 서비스 미활성, 도메인·IP 제한 등) → 재등록해도
> 안 고쳐지고 provider 앱 설정을 고쳐야 함(`ErrGeocoderForbidden`). 사용자 조치가 다르므로
> 분리한다.

## Resolve 구현 계약(geocode 측이 지켜야 함)

| 상황 | 반환 |
|---|---|
| 최소 1건 매칭 | 대표 후보 1건의 `Coordinate`, `nil` |
| 매칭 0건 | `Coordinate{}`, `core.ErrGeocoderNotFound` |
| 키 무효(HTTP 401) | `Coordinate{}`, `core.ErrGeocoderAuthFailed` |
| 요청 거부(HTTP 403) | `Coordinate{}`, `core.ErrGeocoderForbidden` |
| 타임아웃/네트워크/기타 5xx·비2xx/파싱 실패 | `Coordinate{}`, `core.ErrGeocoderUnavailable` |

> `errStationNotFound`는 core 비공개 sentinel이다. geocode는 이를 직접 반환하지 않고 위 공개
> sentinel을 반환하며, core의 `resolveStation` 폴백이 `core.ErrGeocoderNotFound`를
> `errStationNotFound`로 접는다. auth/forbidden/unavailable은 그대로 전파된다.

## FindRoute 폴백 계약

- from/to 각각에 대해: `searchStation` 실패(not found) 시 `Geocoder != nil`이면
  `Geocoder.Resolve` 호출 → 성공 좌표로 경로 검색 계속.
- `Geocoder.Resolve`가 not-found → 해당 Side의 `ErrPointNotFound`.
- `Geocoder.Resolve`가 `core.ErrGeocoderAuthFailed`/`core.ErrGeocoderForbidden`/`core.ErrGeocoderUnavailable` → 그대로 전파.
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
