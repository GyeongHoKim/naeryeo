# Phase 0 Research: 건물명·주소(POI) 출발지/도착지 지원

이 조사는 spec(`spec.md`) 및 Clarifications(2026-07-02)를 근거로, 이름→좌표 해석 단계를
정류장 검색 실패 시 외부 지오코더로 폴백하도록 확장하기 위한 기술 결정을 정리한다. 기존
기능(002 경로 검색, 001 키체인 저장)의 코드/문서를 재사용 기반으로 삼는다.

## 1. 외부 지오코딩 서비스 선택 — Kakao Local 키워드 검색

**Decision**: 건물명·주소·POI를 좌표로 해석하는 외부 서비스로 **Kakao Local "Search by
keyword"**(`GET https://dapi.kakao.com/v2/local/search/keyword.json`)를 사용한다.

- 인증: `Authorization: KakaoAK <REST_API_KEY>` 요청 헤더(쿼리 파라미터 아님).
- 필수 파라미터: `query`(검색어). 선택: `page`(1–45), `size`(1–15, 기본 15), `sort`
  (`accuracy` 기본 / `distance`), 반경 검색용 `x`/`y`/`radius`.
- 응답: `documents[]` 배열. 각 원소의 `place_name`(장소명), `address_name`(지번 주소),
  `road_address_name`(도로명 주소), `x`(경도, 문자열), `y`(위도, 문자열). `meta.total_count`,
  `meta.is_end` 등 페이지 메타 포함.
- 사용 방식: `documents[0]`의 `x`(경도)·`y`(위도)를 대표 좌표로 채택(spec FR-004).

**Rationale**: ODsay 공식 가이드(002 research.md §1)가 건물명·주소 좌표 변환을
"Naver/Daum(Kakao)/Google 지도 API"로 먼저 구하라고 명시했고, ODsay 자체에는 지오코딩
엔드포인트가 없음이 재확인되었다. Kakao Local은 (a) 단일 REST+JSON, (b) `KakaoAK` 헤더
하나로 인증되는 단순 모델, (c) 한국 건물명/POI 커버리지가 강하고, (d) 좌표를 ODsay가 요구하는
경위도(EPSG:4326)로 반환한다. spec 입력에서 사용자가 예시로 지목한 서비스이기도 하다.

**Alternatives considered**:
- **Naver 지도(지역 검색/지오코딩)**: `X-NCP-APIGW-API-KEY-ID` + `X-NCP-APIGW-API-KEY` 두 개
  자격증명이 필요해 인증/키 관리가 더 복잡. 키체인 저장 항목이 2개로 늘어나 setup UX가
  나빠짐 → 기각.
- **Google Geocoding/Places**: 과금 및 결제수단 등록 필요, 국내 건물명 대비 과한 범용성 →
  현재 범위에 부적합해 기각.
- **좌표를 사용자가 직접 입력**: spec은 텍스트 명칭 입력을 전제 → 기각.

**미확인 사항 (구현 시점에 확인 필요)**: Kakao 일일 쿼터/요율 제한은 문서에 명시가 없어, 실제
발급 키로 확인 필요. 인증 실패(잘못된 키) 시 반환되는 HTTP 상태/에러 바디도 실제 키로
검증해야 한다(현재 매핑은 §3의 best-effort). 좌표 문자열이 항상 경위도인지(일부 카카오
API는 WTM/WGS84 선택 가능) 키워드 검색은 기본 WGS84 경위도를 반환한다는 문서 기술을 실측으로
확인한다.

## 2. 키 저장 확장 — config 자격증명 파라미터화

**Decision**: `internal/config`의 저장 API를 **자격증명(Credential) 식별자 기반으로
파라미터화**한다. 현재 고정 상수(`keyUsername = "odsay-api-key"`)를 자격증명 타입으로
승격한다:

```go
type Credential string
const (
    ODsayAPIKey    Credential = "odsay-api-key"
    GeocoderAPIKey Credential = "geocoder-api-key"
)
func Save(cred Credential, apiKey string) error
func Load(cred Credential) (string, error)
func Delete(cred Credential) error
```

기존 호출부(`setup.go`, `logout.go`, `route.go`, `mcp.go`)는 `config.ODsayAPIKey`를 넘기도록
갱신한다. 키체인 서비스명(`serviceName = "naeryeo"`)은 그대로 두고, username만 자격증명별로
분리한다(같은 서비스 아래 두 항목).

**Rationale**: 키체인 접근 코드 경로를 하나로 유지하면서(중복 제거) 두 자격증명을 독립적으로
등록·조회·삭제할 수 있다(spec FR-006). 모든 호출부가 internal이라 시그니처 변경의 파급이
저장소 내부에 갇힌다. go-keyring 백엔드는 (service, username) 쌍으로 항목을 구분하므로 username
분리만으로 충분하다.

**Alternatives considered**:
- `SaveGeocoder`/`LoadGeocoder`/`DeleteGeocoder` 별도 함수 추가: 키체인 로직이 자격증명 수만큼
  복제됨 → constitution(불필요한 복잡도 금지)에 반해 기각.
- 서비스명을 자격증명별로 분리(`naeryeo-geocoder`): username 분리로 충분한데 서비스명까지
  나누면 향후 열거/일괄 삭제가 번거로워짐 → 기각.

## 3. 지오코더 에러 매핑

**Decision**: `Geocoder`의 에러 계약 sentinel은 **core가 소유**한다. geocode는 core를 단방향
import해 이 core sentinel들을 반환하고, core의 `resolveStation` 폴백이 `errors.Is`로 분류한다.

| 상황 | geocode 반환(= core sentinel) | core 처리 | 사용자 결과 |
|---|---|---|---|
| `documents` 비어 있음(0건) | `core.ErrGeocoderNotFound` | `errStationNotFound`(비공개)로 접음 | `ErrPointNotFound{Side}` (FR-008) |
| HTTP 401(키 무효) | `core.ErrGeocoderAuthFailed` | 그대로 전파 | "키가 유효하지 않음. 재등록"(FR-009) |
| HTTP 403(키는 유효하나 요청 거부) | `core.ErrGeocoderForbidden` | 그대로 전파 | "앱의 지도/로컬 서비스 활성화·제한 확인"(FR-009) |
| 타임아웃/네트워크/기타 비2xx/JSON 파싱 실패 | `core.ErrGeocoderUnavailable` | 그대로 전파 | "장소 검색 서비스 연결 불가"(FR-009) |

**Rationale**: "검색 결과 없음"을 기존 정류장 미검색과 동일한 `errStationNotFound`로 접으면
FindRoute의 기존 not-found 처리 경로를 그대로 재사용할 수 있다(중복 최소화). 인증/불가용은
별도 sentinel로 분리해 spec FR-009의 "서비스 오류 ≠ 장소 없음" 구분을 만족한다.

**설계 정정(구현 중 발견)**: 초기 계획은 "geocode가 자체 공개 sentinel을 소유하고 core가
`errors.Is`로 접는다"였으나, 이는 **import 순환**을 만든다 — geocode는 `core.Coordinate`
때문에 core를 import하는데, core가 `geocode.ErrNotFound`를 참조하면 상호 import가 된다. 따라서
sentinel(및 `Coordinate`, 인터페이스)을 모두 소비자인 core가 소유하도록 정정했다. core는 구현
패키지를 import하지 않으므로 단방향(geocode→core)만 남아 순환이 없다.

**실측 확인(2026-07-02)**: 실제 Kakao 키로 검증한 결과, 잘못된/미인가 요청은 **HTTP 403**과
`{"errorType":"NotAuthorizedError","message":"App(...) disabled OPEN_MAP_AND_LOCAL service."}`
바디를 반환했다(키 자체는 유효하나 앱에 카카오맵/로컬 서비스 미활성). 이에 따라 **401=키 무효
(`ErrGeocoderAuthFailed`, 재등록이 해법)와 403=요청 거부(`ErrGeocoderForbidden`, 앱 설정이 해법)를
분리**했다 — 사용자 조치가 다르기 때문. 바디 문자열 파싱에 의존하지 않고 상태코드로만 구분해
Kakao 특화 결합을 피한다.

**리스크 (구현 시 검증 필요) — 정류장 에러 코드가 폴백을 건너뛸 가능성**: 현재
`classifyODsayError`(`internal/core/client.go:157-177`)는 `resolveStation`(정류장 검색)과
`FindRoute`(경로 검색)에 공유된다. 만약 ODsay `searchStation`이 error 객체 코드 `3`/`4`/`5`를
반환하면 `resolveStation`은 `errStationNotFound`가 아니라 `*ErrPointNotFound`를 **직접** 반환하고,
폴백 분기(`errors.Is(err, errStationNotFound)` 검사)를 건너뛰어 **지오코더 폴백이 동작하지
않는다**. `searchStation`이 실제로 이 코드들을 내는지는 002 시점에도 미확인이었다(코드 주석이
이 엔드포인트 응답 형태를 "실제 키로 확인 필요"로 명시). **대응**: 실제 ODsay 키로
`searchStation`의 무매칭 응답(0건 vs error code)을 실측하는 검증 task를 두고, 필요 시
`resolveStation`에서 정류장 not-found 계열(코드 3/4/5 및 빈 결과)을 모두 `errStationNotFound`로
정규화해 폴백 진입을 보장한다.

## 4. core 폴백 통합 — 소비자 정의 Geocoder 인터페이스

**Decision**: `internal/core`가 **자신이 소비하는 작은 인터페이스**를 정의한다:

```go
// core 패키지 내부
type Coordinate struct{ X, Y float64 } // 경도 X, 위도 Y
type Geocoder interface {
    Resolve(ctx context.Context, query string) (Coordinate, error)
}
```

`Client`에 선택적 `Geocoder Geocoder` 필드를 추가한다. `resolveStation`이
`errStationNotFound`를 반환할 때 `c.Geocoder != nil`이면 지오코더로 폴백한다. 지오코더가
`nil`이면 현재와 동일하게 동작(회귀 없음, spec FR-012). Kakao 구현체는 별도 패키지
`internal/geocode`(`geocode.Kakao`)에 두고, core는 geocode를 import하지 않는다(결합 최소화,
002 research §4의 "core는 config를 import하지 않는다" 원칙과 일관).

**Rationale**: 소비자 측 인터페이스 정의는 관용적 Go이며(constitution Principle I), core를
Kakao·config 세부와 분리해 가짜 Geocoder로 폴백 로직을 결정적으로 단위 테스트할 수 있게 한다.
`internal/geocode`를 새로 두는 것은 speculative 추상화가 아니라 실제 두 번째 외부 연동이라는
구체 필요에 대응하는 것이다.

**Alternatives considered**:
- Kakao 클라이언트를 core 패키지 안에 직접 구현: core가 Kakao HTTP 세부에 결합되어 테스트·교체
  용이성이 떨어짐 → 기각.
- core가 config를 직접 읽어 키를 조회: 002에서 확립한 계층 분리 위반 → 기각.

## 5. 진입점 배선(cmd) — 지오코더는 선택적 주입

**Decision**: `route`/`mcp` 진입점이 ODsay 키(필수)와 지오코더 키(선택)를 각각
`config.Load(config.ODsayAPIKey)` / `config.Load(config.GeocoderAPIKey)`로 조회한다. 지오코더
키가 있으면 `geocode.NewKakao(key)`를 만들어 `core.Client.Geocoder`에 주입하고, 없으면 주입하지
않는다(nil).

**FR-007 안내(정류장 미검색 + 지오코더 미설정)**: core는 이 상태를 특별히 구분하지 않고 기존
`ErrPointNotFound`를 반환한다. **진입점이** 지오코더 키 설정 여부를 알고 있으므로, findRoute가
`ErrPointNotFound`를 반환했고 지오코더 키가 없었던 경우에 한해 "정류장으로 찾을 수 없으며
건물명·주소 검색에는 `naeryeo setup --geocoder`가 필요합니다" 힌트를 덧붙인다. 이렇게 하면
core는 단순하게 유지되고, CLI/MCP가 각자 표현을 책임진다(002의 `routeErrorMessage` 공유 패턴과
정합).

**Rationale**: "지오코더 설정 여부"는 진입점이 이미 가진 정보이므로 core에 상태 플래그를 새로
넣지 않아도 된다(불필요한 도메인 복잡도 회피).

## 6. CLI 표면 — setup/logout 대상 플래그

**Decision**: 기존 `setup`/`logout`에 `--geocoder` 불리언 플래그를 추가한다. 플래그가 없으면
대상은 `config.ODsayAPIKey`(기존 동작 유지), 있으면 `config.GeocoderAPIKey`. `setup --geocoder`는
프롬프트 문구를 "Kakao REST API Key: "로 바꾸고, `logout --geocoder`는 지오코더 키를 삭제한다.

**Rationale**: Clarifications(2026-07-02)에서 확정. 단일 명령 진입점 유지 + `logout` 대칭 확장.
`route`는 플래그 추가 없이 내부적으로 지오코더 폴백을 자동 사용한다.

## 7. 테스트 전략

**Decision**: 기존 패턴(002/001) 재사용.
- `internal/geocode`: `httptest.Server`로 Kakao 키워드 검색 응답을 흉내 내어 `Kakao.Resolve`를
  테이블 기반 테스트(정상 1건/다건→첫 건/0건→not found/401→auth failed/5xx→unavailable).
- `internal/core`: 가짜 `Geocoder`(성공/ not found/ auth 실패/ nil)를 주입해 `FindRoute`의 폴백
  분기를 검증. 실제 Kakao 서버 의존 없음.
- `internal/config`: 자격증명별(Save/Load/Delete) 독립 저장·조회·삭제를 fake backend로 검증.
- `cmd/naeryeo`: `setup --geocoder`/`logout --geocoder` 플래그 파싱 및 대상 자격증명 선택,
  route의 FR-007 힌트 문구를 fake load/findRoute로 검증.

**Rationale**: 외부 서비스 의존 없는 결정적 테스트로 constitution Principle II·III(테스트 필수 +
CI 안정성)를 만족한다.
