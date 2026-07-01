# Phase 0 Research: 대중교통 경로 검색 (ODsay 연동 코어 로직)

이 조사는 ODsay LAB 공식 가이드(`https://lab.odsay.com/guide/guide`, WebFetch로 확인, 2026-07-01
조회)를 근거로 한다. context7에는 ODsay가 색인되어 있지 않아(니치한 국내 API) 공식 웹 문서를
직접 조회했다.

## 1. 좌표 기반 경로 검색 — 이름 검색이 아니다

**Decision**: 경로 검색 자체(`GET https://api.odsay.com/v1/api/searchPubTransPathT`)는
위경도 좌표(`SX`,`SY`,`EX`,`EY`)만 받는다. 역/정류장 "이름"을 직접 받는 파라미터는 없다.
따라서 `internal/core`는 두 단계 호출을 조합해야 한다:
1. 이름 → 좌표 변환: `GET https://api.odsay.com/v1/api/searchStation` (대중교통 정류장 검색)
2. 좌표 → 경로: `GET https://api.odsay.com/v1/api/searchPubTransPathT`

**Rationale**: README/spec의 "역/정류장 이름 또는 주소" 입력 요구사항을 만족하려면 이름 검색
단계가 필수다. ODsay는 이 변환을 자체 API로 제공하므로 별도의 서드파티 지오코더는 필요 없다.

**Alternatives considered**: 사용자가 직접 좌표를 입력하게 하기 — spec Assumptions에서 이미
텍스트 명칭 입력을 전제로 범위를 정했으므로 기각. Naver/Daum/Google 지도 API로 좌표 변환 —
ODsay 문서가 대안으로 언급하지만, ODsay 자체 정류장 검색 API로 충분하고 추가 서드파티 키
관리 부담을 피할 수 있어 기각(추후 주소 기반 검색 정확도가 부족하면 재검토).

**미확인 사항 (구현 시점에 확인 필요)**: `searchStation`의 정확한 요청 파라미터명(검색어
파라미터, 도시 코드 등)과 응답 JSON의 전체 필드 목록은 공개 가이드 페이지에 전부 나와 있지
않고 로그인 후 API Reference/Console에서 확인해야 한다. 실제 발급받은 API 키로 최초 호출 시
개발자가 응답을 직접 확인해 매핑을 검증해야 한다.

## 2. 경로 검색 API 계약

**Decision**: `searchPubTransPathT` 요청/응답 스키마는 다음과 같이 확인됨:

- 필수: `apiKey`, `SX`, `SY`, `EX`, `EY`
- 선택: `OPT`(0=추천경로 정렬, 1=타입별 정렬 — **기본값 0을 사용해 추천경로를 그대로 대표
  경로로 채택**, FR-014), `SearchType`(0=도시내), `SearchPathType`(0=모두/1=지하철/2=버스 —
  기본값 0으로 모든 수단 포함, spec FR-002)
- 응답: `result.path[]` 배열(각 원소가 하나의 경로 후보). 각 경로는
  `info.{totalTime, payment, busTransitCount, subwayTransitCount, totalDistance,
  firstStartStation, lastEndStation}`와 `subPath[]`(구간별 `trafficType`,`sectionTime`,
  `distance`,`lane`,`startName`,`endName`,`stationCount`,`passStopList`)로 구성.

**Rationale**: `OPT=0`을 그대로 사용하면 ODsay가 이미 "대표 경로"를 `path[0]`으로 정렬해
주므로, spec FR-014("여러 후보 중 대표 경로 하나 선택")를 별도 로직 없이 만족한다.

**Alternatives considered**: `OPT=1`(타입별 정렬) 후 자체 기준으로 대표 경로 선택 — ODsay의
추천 알고리즘을 재구현하는 셈이라 불필요한 복잡도(기각).

## 3. 에러 코드 매핑

**Decision**: 확인된 에러 코드를 spec의 요구사항에 다음과 같이 매핑한다.

| ODsay 코드 | 의미 | 매핑되는 도메인 결과 |
|---|---|---|
| `3` | 출발지 정류장 없음 | `ErrPointNotFound{Side: "from"}` (FR-009) |
| `4` | 도착지 정류장 없음 | `ErrPointNotFound{Side: "to"}` (FR-009) |
| `5` | 출·도착지 모두 없음 | `ErrPointNotFound{Side: "both"}` (FR-009) |
| `6` | 서비스 지역이 아님 | `ErrNoRoute`(사유 문구에 "서비스 지역 아님" 포함) (FR-010) |
| `-98` | 출·도착지가 700m 이내 | **에러가 아님** — `RouteResult.NoTravelNeeded = true` (FR-012) |
| `-99` | 검색결과 없음 | `ErrNoRoute` (FR-010) |
| `-8` | 입력값 형식/범위 오류 | `ErrUpstreamRejected`(내부 버그로 취급 — 우리가 보낸 좌표 형식이 잘못된 것이므로 사용자 탓이 아님) |
| `-9` | 필수 입력값 누락 | `ErrUpstreamRejected`(위와 동일 사유) |
| `500` | 서버 내부 오류 | `ErrUpstreamUnavailable` (FR-011) |

**Rationale**: `-98`(700m 이내)을 "출발지=도착지"(FR-012)의 실제 판정 근거로 사용하기로
했다. 단순 문자열 비교("강남역" == "강남역")보다 더 견고하다 — 이름은 다르지만 같은 지점을
가리키는 경우(예: "강남역"과 "강남역 2번출구")까지 자연스럽게 같은 결과로 처리되기 때문이다.

**Alternatives considered**: 클라이언트에서 `from`/`to` 문자열을 정규화해 직접 비교 —
이름이 다른데 실제로는 같은/매우 가까운 지점인 경우를 놓친다는 점에서 `-98` 활용이 더
정확하다고 판단해 기각.

**미확인 사항 (구현 시점에 확인 필요)**: **API 키가 유효하지 않거나 만료된 경우, 또는 요청
한도를 초과한 경우에 대한 에러 코드는 공개 가이드 페이지에 명시되어 있지 않았다** (HTTP 상태
코드 언급도 없음). FR-008("키가 유효하지 않음을 '키 미설정'과 구분해 안내")을 정확히
구현하려면, 실제 발급받은 API 키로 잘못된 키를 넣어 호출해보고 실제 반환되는 코드/상태를
확인해야 한다. 이 조사에서 그 값을 추측해 하드코딩하지 않는다 — 대신 코드에 "알 수 없는 ODsay
에러 코드"를 위한 포괄적 처리 경로(`ErrUpstreamRejected`)를 두고, 실제 인증 실패 코드가
확인되는 대로 `ErrAuthFailed`로 세분화할 수 있는 구조로 설계한다(data-model.md 참조).

## 4. 인증 방식

**Decision**: 모든 요청에 `apiKey=<발급된 키>` 쿼리 파라미터를 포함한다(HTTP 헤더 아님).
`internal/core`의 `Client`는 `internal/config`에 의존하지 않고 문자열 `apiKey`를 생성자
인자로 받는다. `apiKey == ""`인 경우 네트워크 호출 없이 즉시 `ErrAPIKeyMissing`을 반환한다.

**Rationale**: `internal/core`가 `internal/config`를 직접 import하지 않으면 두 패키지의
결합이 느슨해지고, 코어 로직만 독립적으로 테스트하기 쉬워진다(가짜 apiKey 문자열만 주입하면
됨). 대신 "키가 없으면 네트워크를 타지 않는다"는 요구사항(FR-007)은 `apiKey == ""`라는 단순
조건으로 코어 안에서 보장되므로, `route`/`mcp` 두 진입점 모두 동일하게 이 보장을 받는다
(FR-013). 각 진입점은 `config.Load()`가 반환한 값(성공 시 키 문자열, `ErrNotConfigured` 시
빈 문자열)을 그대로 넘기기만 하면 된다.

**Alternatives considered**: `internal/core`가 `internal/config.Load()`를 직접 호출 —
계층 간 결합이 강해지고, 코어 로직 테스트마다 키체인 모킹이 필요해져 기각.

## 5. 타임아웃/재시도 정책

**Decision**: `Client.FindRoute(ctx, ...)`는 `context.Context`를 받아 `http.NewRequestWithContext`로
모든 외부 호출에 전파한다. 별도 재시도 로직은 두지 않는다(호출자가 필요시 자체 재시도).
기본 HTTP 클라이언트에는 합리적인 기본 타임아웃(예: 10초, SC-001과 정합)을 둔 `http.Client`를
사용하되, 호출자가 `Client.HTTPClient` 필드로 교체할 수 있게 한다.

**Rationale**: FR-011("무한정 대기하거나 예기치 않게 종료되지 않음")을 만족시키는 가장 단순한
방법이다. `context.Context` 전파는 Go 표준 관용구이며, CLI(`route`)는 자체 데드라인 없이
호출해도 되고, 향후 MCP 서버는 요청별 타임아웃을 상위에서 제어할 수 있다.

**Alternatives considered**: 패키지 내부에 재시도/백오프 로직 내장 — 이 시점에는 요구사항에
없는 speculative 기능이라 기각(constitution: 정당화되지 않는 추상화 금지).

## 6. HTTP 클라이언트 구현 방식 — 표준 라이브러리만 사용

**Decision**: 외부 SDK 없이 `net/http` + `encoding/json`만으로 ODsay 클라이언트를 구현한다.

**Rationale**: ODsay는 단순 REST+JSON API이고, 별도 공식 Go SDK도 없다(웹 검색 결과 확인).
표준 라이브러리로 충분하며 새 의존성을 추가할 이유가 없다(constitution Principle I: 정당화
되지 않는 복잡도 도입 금지).

**Alternatives considered**: 서드파티 HTTP 클라이언트 래퍼(resty 등) 도입 — 표준 라이브러리로
충분한 규모라 기각.

## 7. 테스트 전략

**Decision**: `net/http/httptest.Server`로 ODsay 엔드포인트를 흉내 내어 `Client.FindRoute`를
종단 간(이름 검색 + 경로 검색) 테이블 기반 테스트로 검증한다. 실제 ODsay 서버에 대한 통합
테스트는 두지 않는다(API 키가 CI에 없고, 있어도 외부 서비스 의존은 결정적이지 않아 constitution
Principle III의 CI 안정성 요구와 배치될 수 있음).

**Rationale**: `internal/config`(001)에서 이미 검증된 패턴(실제 백엔드 대신 테스트 더블/서버
사용)과 일관된다. `httptest.Server`의 URL을 `Client.BaseURL`에 주입해 두 단계 호출(정류장
검색 → 경로 검색) 모두를 결정적으로 재현할 수 있다.

**Alternatives considered**: 실제 `api.odsay.com`을 호출하는 통합 테스트를 build tag로 분리 —
현재 범위에서는 불필요한 복잡도로 판단해 보류(필요해지면 향후 추가 가능).
