# Phase 1 Data Model: 건물명·주소(POI) 출발지/도착지 지원

spec의 Key Entities와 research 결정을 코드 수준 자료구조로 구체화한다. 기존 002/001의 타입을
재사용하고, 이 기능이 추가/변경하는 것만 기술한다.

## 1. Credential (internal/config) — 신규

키체인에 저장되는 자격증명의 식별자. 고정 상수 `keyUsername`을 타입으로 승격한다.

| 필드/상수 | 타입 | 값 | 설명 |
|---|---|---|---|
| `Credential` | `string`(정의 타입) | — | 키체인 username으로 쓰이는 자격증명 식별자 |
| `ODsayAPIKey` | `Credential` | `"odsay-api-key"` | 대중교통 경로 검색용 키(기존 값과 동일 → 기존 저장분 호환) |
| `GeocoderAPIKey` | `Credential` | `"geocoder-api-key"` | 장소 검색(지오코딩)용 키 |

**규칙/제약**:
- 서비스명은 `naeryeo`로 공통, username만 자격증명별로 분리 → 두 항목이 독립 저장.
- `ODsayAPIKey`의 문자열 값은 기존 `keyUsername`과 동일하게 유지해야 기존 사용자의 저장분이
  마이그레이션 없이 계속 로드된다(회귀 방지).
- `Save`는 빈 값 거부(`ErrEmptyValue`), 키체인 불가용 시 평문 폴백 없음(기존 정책 승계).

## 2. Coordinate (internal/core) — 신규

지오코더가 반환하는 지점 좌표. ODsay 경로 검색이 요구하는 경위도(EPSG:4326).

| 필드 | 타입 | 설명 |
|---|---|---|
| `X` | `float64` | 경도(longitude) |
| `Y` | `float64` | 위도(latitude) |

기존 `stationCandidate`도 `X`(경도)/`Y`(위도)를 갖는다. 폴백 결과는 `Coordinate`로 표현하고,
경로 검색 URL 조립 시 정류장/지오코더 어느 출처든 동일하게 `SX/SY/EX/EY`에 매핑된다.

## 3. Geocoder 인터페이스 (internal/core) — 신규

core가 소비하는 소형 인터페이스. 구현은 `internal/geocode`에 위치.

```go
type Geocoder interface {
    Resolve(ctx context.Context, query string) (Coordinate, error)
}
```

**계약**: `Geocoder` 구현체(`internal/geocode`)는 core 비공개 sentinel을 반환할 수 없으므로,
**geocode 패키지가 자체 공개 sentinel을 반환**하고 core의 폴백 코드가 이를 접는다.

- 성공: 대표(최상위) 후보 1건의 `Coordinate` 반환, `nil`.
- 결과 0건: `geocode.ErrNotFound` 반환 → core가 `errors.Is`로 판별해 내부
  `errStationNotFound`로 접음 → `FindRoute`가 `ErrPointNotFound{Side}`로 변환(FR-008).
- 인증 실패: `geocode.ErrAuthFailed` 반환 → core가 `ErrGeocoderAuthFailed`로 변환(FR-009).
- 그 외(타임아웃/네트워크/5xx/파싱): `geocode.ErrUnavailable` 반환 → core가
  `ErrUpstreamUnavailable`로 변환(FR-009).

> `errStationNotFound`/`ErrGeocoderAuthFailed`/`ErrUpstreamUnavailable`은 모두 core 소유다.
> geocode는 이들을 직접 반환하지 않으며, 자신의 공개 sentinel(`ErrNotFound`/`ErrAuthFailed`/
> `ErrUnavailable`)만 노출한다. 매핑은 core의 `resolveStation` 폴백 분기가 담당한다
> (contracts/core-geocoder.md, contracts/geocode-kakao.md와 일치).

## 4. Client 변경 (internal/core)

| 필드 | 타입 | 변경 | 설명 |
|---|---|---|---|
| `Geocoder` | `Geocoder` | 신규(선택) | `nil`이면 지오코더 폴백 비활성(기존 동작). 설정 시 정류장 검색 실패에 폴백 |

**resolveStation 흐름(변경)**:
1. 기존대로 `searchStation` 호출.
2. `errStationNotFound`이고 `c.Geocoder != nil`이면 `c.Geocoder.Resolve(ctx, name)` 호출.
   반환 `Coordinate`는 기존 `stationCandidate`로 변환해 반환한다(폴백 반환 타입 확정 —
   신규 좌표 타입을 도입하지 않고 `resolveStation`의 기존 시그니처 `(stationCandidate, error)`를
   유지): `stationCandidate{X: flexibleFloat(coord.X), Y: flexibleFloat(coord.Y)}`. `Name`은
   비워 둔다(현재 결과 표현에 정류장명이 쓰이지 않음).
   - `Resolve`가 `geocode.ErrNotFound`이면 `errStationNotFound`로 접어 전파(→ ErrPointNotFound).
   - `Resolve`가 `geocode.ErrAuthFailed`이면 `ErrGeocoderAuthFailed`로 변환해 전파.
   - `Resolve`가 `geocode.ErrUnavailable`이면 `ErrUpstreamUnavailable`로 변환해 전파.
3. `c.Geocoder == nil`이면 기존과 동일하게 `errStationNotFound` 전파.

`FindRoute`는 from/to 각각에 위 흐름을 독립 적용(혼합 입력 지원, spec Edge Case).

## 5. 에러

**core 소유 (internal/core/errors.go)** — 사용자 대면 도메인 에러:

| 심볼 | 종류 | 변환되는 사용자 결과 |
|---|---|---|
| `ErrGeocoderAuthFailed` | 신규 sentinel | "장소 검색 키가 유효하지 않음"(FR-009, 미검색과 구분) |
| `ErrPointNotFound` | 기존 재사용 | 어느 쪽(from/to) 미해석인지 안내(FR-008) |
| `ErrUpstreamUnavailable` | 기존 재사용 | 서비스 오류 안내(FR-009) |

**geocode 소유 (internal/geocode)** — core로 전달되는 저수준 sentinel(사용자 대면 아님):

| 심볼 | 발생 | core의 접기 |
|---|---|---|
| `geocode.ErrNotFound` | 검색 결과 0건 | `errStationNotFound`(비공개) → `ErrPointNotFound` |
| `geocode.ErrAuthFailed` | HTTP 401/403 | `ErrGeocoderAuthFailed` |
| `geocode.ErrUnavailable` | 타임아웃/네트워크/5xx/파싱 | `ErrUpstreamUnavailable` |

## 6. Kakao 응답 매핑 (internal/geocode)

Kakao 키워드 검색 응답 → `core.Coordinate`.

| Kakao 필드 | 매핑 |
|---|---|
| `documents[0].x` (문자열, 경도) | `Coordinate.X`(`ParseFloat`) |
| `documents[0].y` (문자열, 위도) | `Coordinate.Y`(`ParseFloat`) |
| `documents` 길이 0 | → `geocode.ErrNotFound`(core가 `errStationNotFound`로 접음) |
| HTTP 401/403 | → `geocode.ErrAuthFailed`(core가 `ErrGeocoderAuthFailed`로 변환) |
| 그 외 실패(타임아웃/네트워크/5xx/파싱) | → `geocode.ErrUnavailable`(core가 `ErrUpstreamUnavailable`로 변환) |

`place_name`/`address_name`/`road_address_name`은 현재 결과 표현에 불필요하므로 좌표만 사용
(향후 후보 표시 기능 도입 시 활용 여지 — 현재 범위 밖).
