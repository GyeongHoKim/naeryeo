# Phase 0: Research — 자체 호스팅 경로 검색 제공자

**Feature**: 006-self-hosted-routing-provider | **Date**: 2026-07-31

조사 출처는 각 항목에 명시했다. Context7(`ctx7`)에는 MOTIS가 색인되어 있지 않아
(검색 결과가 Moti/Motion 등 무관한 라이브러리로 나옴) 업스트림 저장소와 OpenAPI 명세를
직접 조회했다.

---

## R1. 자체 호스팅 엔진 선택

**Decision**: MOTIS (`motis-project/motis`).

**Rationale**:

- 국내 대중교통 데이터로 실제 운영되는 선례가 있다 — Transitous 프로젝트가 MOTIS를 쓰고
  `feeds/kr.json`에 한국 GTFS 피드를 등록해 두었다.
- GTFS(정적) + GTFS-RT/SIRI/GBFS(실시간) + OSM(도보·자전거)을 한 프로세스가 소화하므로
  naeryeo가 필요로 하는 "이름 → 좌표 → 경로"가 **엔진 하나 안에서** 끝난다(R3 참조).
- 사전 빌드 바이너리와 컨테이너 이미지(`ghcr.io/motis-project/motis`)를 제공해 사용자가
  컴파일할 필요가 없다 — spec SC-003("문서만으로 구축") 달성에 결정적.
- REST/JSON + OpenAPI 명세를 제공하므로 Go 표준 라이브러리만으로 연동된다(새 의존성 0).

**Alternatives considered**:

- **OpenTripPlanner (OTP)** — 기능적으로 대등하고 GTFS 지원이 성숙하지만 JVM 런타임을
  요구한다. "문서만 보고 자기 장비에 띄운다"는 목표에서 JVM 힙 튜닝은 실패 지점을 하나
  더 만든다.
- **Valhalla** — 도로 라우팅 중심이고 대중교통 지원이 부수적이다. 이 기능의 핵심 사용
  사례(지하철·버스 환승)에 맞지 않는다.
- **자체 라우팅 엔진 구현** — 명세의 Out of Scope. 논외.

**출처**: <https://github.com/motis-project/motis>,
<https://github.com/public-transport/transitous/blob/main/feeds/kr.json>

---

## R2. MOTIS 배포·기동 방법

**확인된 사실**:

| 항목 | 값 | 확실도 |
| --- | --- | --- |
| 컨테이너 이미지 | `ghcr.io/motis-project/motis` | 확인됨 |
| 기동 절차 | `motis config <osm.pbf> <gtfs.zip>` → `motis import` → `motis server` | 확인됨 (README 인용) |
| HTTP 포트 | 8080 | 확인됨 |
| 바이너리 배포 | `https://github.com/motis-project/motis/releases/latest/download/motis-${TARGET}.tar.bz2` | 확인됨 |
| `config.yml` 스키마 | **미확인** — README는 "generates a minimal config.yml"이라고만 함 | 미해결 → R7 |
| RAM/디스크 요구치 | **미확인** — "low memory usage", "planet-sized deployments on affordable hardware"라는 정성적 서술만 | 미해결 → R7 |

**Decision**: `deploy/motis/compose.yaml`은 **이미지 태그를 고정**하고(`latest` 금지),
import와 server를 분리한 2단계 구성으로 작성한다. 데이터 볼륨은 호스트 바인드 마운트로 두어
사용자가 GTFS/OSM 파일을 직접 갈아끼울 수 있게 한다.

**Rationale**: MOTIS README가 명시적으로 경고한다 — "Ensure a valid timetable is used. If
the timetable is outdated, it will not contain any trips to consider for upcoming dates."
즉 데이터 신선도가 곧 기능 정상성이므로, 데이터 교체가 쉬운 구조여야 하고 문서가 갱신 주기를
알려야 한다(spec FR-024).

**출처**: <https://github.com/motis-project/motis> (README),
<https://github.com/motis-project/motis/pkgs/container/motis>

---

## R3. MOTIS HTTP API — 경로 검색

**확인된 사실** (OpenAPI 명세 직접 조회):

- **엔드포인트**: `GET /api/v6/plan`
- **필수 파라미터**:
  - `fromPlace` — `"latitude,longitude[,level]"` 튜플 **또는 stop ID**
  - `toPlace` — 동일
- **주요 선택 파라미터**: `time`(date-time, 기본 현재), `arriveBy`(bool, 기본 false),
  `transitModes`(Mode 배열, 기본 TRANSIT), `numItineraries`(기본 5), `maxTransfers`,
  `maxTravelTime`(분), `withFares`(bool, **experimental**)
- **응답 최상위**: `from`, `to`, `direct[]`, `itineraries[]`, `previousPageCursor`,
  `nextPageCursor`, `requestParameters`, `debugOutput`
- **Itinerary**: `duration`, `startTime`, `endTime`, `transfers`, `legs[]`
- **Leg**: `mode`, `from`, `to`, `duration`, `routeShortName`, `headsign`, `agencyName`,
  `interlineWithPreviousLeg` (+ `detailedLegs=true`일 때 `legGeometry`, `steps`)

**Decision (중요)**: **`fromPlace`가 자유 텍스트 이름을 받지 않는다**는 것이 이 기능의 설계를
좌우한다. ODsay는 `searchStation`으로 이름을 받아 주지만 MOTIS의 plan은 좌표/ID만 받는다.
따라서 **이름 해석 단계가 MOTIS 제공자에게는 선택이 아니라 필수**다.

**Rationale**: 이 사실을 놓치면 "MOTIS만 설정한 사용자가 '강남역'을 못 넣는다"는 상태로
구현이 끝난다 — spec User Story 1의 Acceptance Scenario 2가 바로 실패한다. R4가 그
해법이다.

**출처**: <https://raw.githubusercontent.com/motis-project/motis/master/openapi.yaml>

---

## R4. MOTIS HTTP API — 지오코딩, 그리고 spec FR-028과의 관계

**확인된 사실**:

- `GET /api/v1/geocode` — 파라미터 `text`(필수), `language`, `type`, `mode`, `place`(좌표
  bias), `placeBias`, `numResults`, `min`/`max`(bounding box). 응답은 `Match` 배열.
- `GET /api/v1/reverse-geocode`도 존재.
- MOTIS의 지오코더는 **GTFS 정류장과 OSM 주소를 함께 색인**한다(README의 "multimodal
  routing, geocoding, and map tiles" 및 osm.pbf가 "street network, addresses,
  indoor-routing" 용도라는 서술).

**Decision**: MOTIS 제공자의 이름 해석은 `/api/v1/geocode`가 담당한다. 이는
**ODsay `searchStation`이 차지하던 자리**이며, 기존 Kakao 지오코더(`internal/geocode`)는
지금과 똑같이 **선택적 폴백**으로 남는다.

```text
ODsay 제공자:  이름 → [ODsay searchStation] → 실패 시 [Kakao(선택)] → 좌표
MOTIS 제공자:  이름 → [MOTIS geocode]       → 실패 시 [Kakao(선택)] → 좌표
```

**Rationale — spec FR-028을 위반하지 않는다**:

사용자는 직전 결정에서 "장소 검색은 현행 유지(Kakao 별도 등록), 외부 의존 0은 후속 기능"을
택했다. 여기서 MOTIS geocode를 쓰는 것은 그 결정과 **충돌하지 않는다**:

- FR-028이 말하는 "장소 검색"은 **선택 기능인 Kakao 지오코더 축**이다. 그 축은 손대지 않는다.
- MOTIS geocode는 제공자 내부의 이름 해석이며, ODsay 사용자가 `searchStation`을 별도 설정
  없이 쓰는 것과 같은 위상이다.
- 오히려 이 선택이 spec **SC-002**("역·정류장 이름만 쓰는 사용자는 외부 호출 0건")를
  성립시킨다 — MOTIS geocode는 사용자의 자체 호스팅 엔진 안에서 실행되므로 외부 호출이 아니다.

**후속 기능의 범위가 좁아진다**: Q1의 옵션 B("외부 의존 0")로 남는 일은 이제
"**건물명·주소 검색에서 Kakao를 MOTIS geocode로 대체**"뿐이다. MOTIS가 OSM 주소를 색인하므로
기술적으로 가능하지만, 국내 건물명·상호 검색 품질이 Kakao Local에 못 미칠 가능성이 크다 —
그 비교 측정이 후속 기능의 핵심 작업이 된다.

**출처**: 위 openapi.yaml, <https://github.com/motis-project/motis>

---

## R5. 요금 정보

**확인된 사실**: `withFares` 파라미터가 존재하며 명세에 "Optional. **Experimental.** If set
to true, the response will contain fare information."로 기술되어 있다. 실제 응답 구조는
명세 조회 범위에서 확인되지 않았다.

**Decision**: v1에서는 **`withFares`를 사용하지 않고, MOTIS 경로의 요금을 "정보 없음"으로
표현한다.**

구현:

- `core.RouteResult`에 `FareKnown bool` 추가. ODsay 경로는 항상 `true`.
- `RouteToolOutput.FareWon`을 `int` → `*int`로 변경. `nil`이면 JSON에서 필드 자체가 사라진다
  (`omitempty` 유지). 값이 있을 때의 와이어 포맷은 지금과 동일하므로 spec 005의 성공 스키마
  계약이 깨지지 않는다.
- 프로즈 출력: `FareKnown=false`면 `요금: N원` 줄 대신 `요금 정보 없음`을 출력한다. ODsay는
  항상 `true`이므로 **기존 프로즈 출력은 바이트 단위로 불변**(spec 005 FR-007).

**Rationale**:

1. 실험적 기능에 v1 계약을 걸지 않는다.
2. KTDB GTFS에 `fare_attributes.txt`/GTFS-Fares v2가 포함되어 있다는 근거가 없다. 요금을
   요청해도 값이 안 올 가능성이 높다.
3. 부수 효과로 **기존 결함이 함께 고쳐진다** — 지금은 `FareWon int` + `omitempty` 탓에
   "요금 0원"과 "요금 필드 없음"이 구별되지 않는다. 포인터화가 spec FR-010("0으로 위장 금지")을
   충족시키면서 이 모호함도 제거한다.

**Alternatives considered**: `withFares=true`를 켜고 값이 오면 쓰는 안 — 응답 구조가 미확인
상태라 파싱 코드를 추측으로 쓰게 되고, 실험적 스키마가 바뀌면 조용히 깨진다. 후속 작업으로
남긴다.

---

## R6. MOTIS 실패 신호의 분류

**확인된 사실**: 전용 health/readiness 엔드포인트는 명세에 **없다**.

**Decision**: 아래 3단으로 분류한다.

| 상황 | 감지 방법 | core 에러 |
| --- | --- | --- |
| 연결 거부·타임아웃·DNS 실패·5xx | transport 에러 또는 status ≥ 500 | `ErrMotisUnavailable` |
| 4xx, JSON 파싱 실패, 예상 밖 스키마 | status 4xx 또는 decode 실패 | `*ErrMotisRejected{Status}` |
| 두 지점 해석됐으나 itineraries 비어 있음 | `len(itineraries)==0 && len(direct)==0` | `ErrNoRoute` (기존) |

**도달성 검증은 setup 시점으로 옮긴다**: `setup --provider=motis`가 URL을 저장하기 **전에**
`/api/v1/geocode?text=<프로브>`를 1회 호출한다.

- 응답 없음 → 저장 거부, "엔진에 연결할 수 없습니다" + 문서 링크
- 응답은 오지만 매치 0건 → 저장 거부, "엔진은 응답하지만 시간표/지도 데이터가 적재되지
  않았습니다" + 문서 링크

이것이 spec **FR-016**("응답은 하나 처리 불가한 상태를 '경로 없음'과 구별")을 충족시키는
결정론적 지점이다. 런타임에 매번 프로브를 돌리면 검색 1회당 왕복이 늘어나므로 하지 않는다.

**미해결 → 구현 중 실측 필요**: 타임테이블이 만료된(과거 날짜만 있는) MOTIS가 plan 요청에
어떤 응답을 주는지. 빈 `itineraries`라면 위 표에서 `no_route`로 떨어지는데, 이는 "경로가
없다"가 아니라 "데이터가 낡았다"이다. 실측 후 `no_route`의 hint에 제공자가 MOTIS일 때만
"시간표 데이터가 최신인지 확인" 문구를 덧붙이는 것으로 흡수한다.

---

## R7. 한국 GTFS·OSM 데이터 확보처

**확인된 사실**:

- Transitous `feeds/kr.json`의 유일한 한국 피드:

  ```json
  {
    "name": "korea",
    "type": "http",
    "url": "https://www.dropbox.com/scl/fi/l1rnl88xnegpmiuy44kmc/GTFS_DataSet.zip?rlkey=jub5anlfpsdoi9mgfy4qp7x9w&dl=1",
    "license": { "url": "https://www.ktdb.go.kr" }
  }
  ```

- 라이선스 주체는 KTDB(한국교통연구원 교통DB). 피드 자체에 날짜/버전 마커가 없다.
- OSM: Geofabrik `south-korea-latest.osm.pbf`.

**Decision**: 문서는 **KTDB 원본 경로를 1순위로, Transitous 미러 URL을 참고로** 안내한다.
Dropbox 링크를 유일한 경로로 문서화하지 않는다.

**Rationale**: 위 URL은 개인/프로젝트 Dropbox 공유 링크로, 만료되거나 rlkey가 회전되면 문서가
조용히 깨진다. spec **FR-022**(재현 가능성)와 **FR-024**(데이터 한계 명시)가 요구하는 것은
"지금 되는 링크"가 아니라 "다시 구할 수 있는 경로"다.

**미해결 → 구현 중 확인 필요**:

1. KTDB에서 GTFS를 직접 내려받는 공식 경로와 그 갱신 주기 (Linear GYE-288이 "2023-03보다
   최신판이 있는지 KTDB에 확인"을 작업으로 잡아 둔 항목)
2. 해당 GTFS의 지역 커버리지 — 전국인지 수도권 중심인지. spec **SC-007**의 대표 질의
   3종(수도권 도시철도 / 지방 광역시 시내 / 도시 간)이 이 커버리지 검증 그 자체다.
3. `config.yml`의 실제 스키마, 그래프 빌드 시간·RAM·디스크 실측치 (spec FR-023). 문서에
   실측값을 쓰려면 실제로 돌려 봐야 하며, 이는 문서 작성 작업의 선행 조건이다.

**출처**: <https://github.com/public-transport/transitous/blob/main/feeds/kr.json>,
<https://www.ktdb.go.kr>, <https://download.geofabrik.de/asia/south-korea.html>

---

## R8. 설정 저장소 — 형식과 위치

**Decision**: `os.UserConfigDir()/naeryeo/config.json`, 파일 권한 `0600`, 디렉터리 `0700`.
인코딩은 **JSON**(stdlib `encoding/json`).

경로 결과:

| OS | 경로 |
| --- | --- |
| macOS | `~/Library/Application Support/naeryeo/config.json` |
| Linux | `~/.config/naeryeo/config.json` (또는 `$XDG_CONFIG_HOME/naeryeo/`) |
| Windows | `%AppData%\naeryeo\config.json` |

**Rationale**:

- **키체인에 넣지 않는 이유**: provider 이름과 URL은 비밀값이 아니다. 키체인에 두면 저장할
  비밀이 하나도 없는 MOTIS 사용자에게까지 키체인 잠금 해제 프롬프트가 붙는다(spec FR-004).
  헤드리스 Linux에서 Secret Service가 없으면 아예 설정조차 못 하게 된다.
- **JSON인 이유**: YAML/TOML은 새 의존성을 요구한다. 헌법의 "추상화/복잡도 정당화" 원칙에서,
  사람이 손으로 편집하는 것을 1차 경로로 삼지 않는 파일(`setup`이 쓴다)에 의존성 하나를 더
  거는 것은 정당화되지 않는다.
- **`os.UserConfigDir()`인 이유**: 3-OS 규약을 stdlib가 이미 알고 있다. 직접 분기하면 GYE-296이
  세운 3-OS CI에서 깨질 표면만 늘어난다.

**Alternatives considered**: 환경변수(`NAERYEO_MOTIS_URL`) — Linear GYE-289의 원안이었고
**취소된 설계**다. `claude_desktop_config.json`이 `naeryeo mcp`를 로그인 셸을 거치지 않고
스폰하므로 사용자의 `.zshrc` export가 전달되지 않아 CLI와 MCP의 제공자가 갈린다. spec
FR-002가 이를 명시적으로 금지한다.

---

## R9. "기본 제공자 = MOTIS"의 정확한 의미

**사용자 지시**: "이제부터 기본 프로바이더는 MOTIS 셀프호스팅으로 두고"

**Decision**: "기본"은 **선택지의 기본값**으로 구현한다 — 무설정 상태의 암묵 폴백이 아니다.

| 반영되는 곳 | 동작 |
| --- | --- |
| `setup` 마법사 | MOTIS가 1번 선택지이자 Enter 기본값 |
| MOTIS URL 입력 | `http://localhost:8080`이 제시되고 Enter로 수락 가능 |
| README / SKILL.md | 자체 호스팅을 1순위 경로로 서술 (spec FR-025) |
| 설정 파일이 없을 때 | **`provider_not_configured`로 실패**. MOTIS로 폴백하지 않는다 |

**Rationale**: 마지막 행이 지시와 어긋나 보일 수 있으나, 사용자가 직전 turn에서 Q2에 대해
"B(재설정 강제)"를 택했고 그것이 spec FR-031로 굳어졌다. 무설정 시 MOTIS로 폴백하면 기존
ODsay 사용자가 업그레이드 직후 "연결할 수 없음"(존재하지 않는 localhost:8080)을 만나게 되어,
"설정을 다시 하라"는 정확한 안내 대신 엉뚱한 실패를 받는다. 두 지시를 모두 만족시키는 해석은
"선택지의 기본값"이다.

---

## 미해결 항목 요약

계획 진행을 막지 않지만 구현 중 실측·확인이 필요한 항목이다. `/speckit-tasks`가 이들을
선행 작업으로 뽑아야 한다.

| # | 항목 | 차단하는 산출물 | 해소 방법 |
| --- | --- | --- | --- |
| U1 | MOTIS `config.yml` 실제 스키마 | `deploy/motis/compose.yaml` | 컨테이너 1회 기동 후 생성물 확인 |
| U2 | 그래프 빌드 시간·RAM·디스크 실측치 | `docs/self-hosting.md` (FR-023) | 실제 빌드 1회 수행하며 계측 |
| U3 | KTDB GTFS 공식 경로·갱신 주기·지역 커버리지 | `docs/self-hosting.md` (FR-022/024) | KTDB 포털 확인 + 피드 내용 검사 |
| U4 | 만료된 타임테이블에서의 plan 응답 형태 | `no_route` hint 문구 (FR-016) | 과거 GTFS로 재현 후 응답 관찰 |
| U5 | MOTIS geocode의 한국어 정류장명 매칭 품질 | 대표 질의 스모크 (SC-007) | 실데이터로 3개 대표 질의 실행 |
