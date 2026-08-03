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

## 미해결 항목 — 전부 해소됨 (2026-08-03 실측)

계측 환경: macOS(darwin 25.5.0), Apple Silicon, 물리 RAM 16 GB. Docker 29.4.0,
VM 할당 메모리 7.82 GB, 10 vCPU. MOTIS **2.11.0**, KTDB GTFS 전국 피드,
Geofabrik `south-korea-latest.osm.pbf`(284 MB, 2026-08-02판).

| # | 항목 | 결과 |
| --- | --- | --- |
| U1 | MOTIS `config.yml` 실제 스키마 | **해소** → R9 |
| U2 | 그래프 빌드 시간·RAM·디스크 | **해소** — 55초 / 최대 3.98 GiB / 1.5 GB → R10 |
| U3 | KTDB GTFS 경로·갱신 주기·커버리지 | **해소** → R11 |
| U4 | 만료 타임테이블에서의 plan 응답 | **해소** — HTTP 200 + 빈 결과(`no_route`) → R12 |
| U5 | 한국어 정류장명 매칭 품질 | **해소** — 대표 질의 3종 전부 성공 → R13 |

---

## R9. MOTIS 이미지·CLI·`config.yml` 실측 (U1 해소)

**이미지 태그**: 계획 단계에서 적어 둔 `v2.0.0`은 **존재하지 않는다**(`docker pull` 실패).
도커 태그는 git 태그와 달리 `v` 접두사가 **없다** — git `v2.11.0` ↔ 이미지 `2.11.0`.
최신 릴리스는 2.11.0(2026-07-31), 압축 해제 215 MB. 2.11.0의 breaking change는
지도 타일 스키마·글리프 관련이라 naeryeo(geocode+plan)에는 영향이 없다.

**이미지 구성**: `ENTRYPOINT=[]`, `CMD=[/motis server /data]`, `USER=motis`(uid 100).
바이너리는 **`/motis`이며 PATH에 없다** — `command: ["motis", ...]`는
"executable file not found in $PATH"로 실패한다.

**서브커맨드**:

```
motis config [PATHS...]              # CWD에 config.yml 생성. 확장자로 판별:
                                     #   ".osm.pbf" → OSM (street routing·geocoding·tiles)
                                     #   그 외      → static timetable
motis import -d <graph> -c <config>  # 전처리. import 후 원본 입력 불필요
motis server -d <graph>              # graph 폴더만으로 기동
```

**`config.yml` 스키마**: `osm`, `timetable`(`first_day: TODAY`, `num_days: 365`,
`datasets.<name>.path` 등), `elevators`, `street_routing`, `osr_footpath`,
`geocoding`, `reverse_geocoding`, 그리고 `tiles`.

**Decision**: 생성된 config를 그대로 쓰지 않고 **`deploy/motis/config.yml`을 체크인**한다.

**Rationale**: `motis config`가 만드는 `tiles` 블록은 `db_size: 274877906944`(**256 GB**)
스파이스 DB를 켜고, 프로파일 경로 `tiles-profiles/full.lua`를 **CWD 상대경로**로 적는데
그 파일은 이미지의 `/tiles-profiles`에 있어 데이터 디렉터리에서 해석되지 않는다. naeryeo는
타일을 전혀 호출하지 않으므로 이 블록을 제거하는 것이 곧 빌드 시간·디스크의 대부분을
없애는 조치다. 또한 체크인해 두어야 몇 달 뒤 재빌드가 같은 그래프를 낸다 —
`motis config`는 엔진 기본값을 따르고, 기본값은 움직인다.

**부수 확인**: OSM 파일 없이 GTFS만 넣으면 생성 config가 `geocoding: false`가 되고
`/api/v1/geocode`는 **404**를 반환한다. naeryeo는 모든 장소 이름을 이 엔드포인트로
해석하므로 **OSM 파일은 선택이 아니다**.

---

## R10. 그래프 빌드 자원 실측 (U2 해소, spec FR-023)

`tiles` 블록을 제거한 config 기준, 위 계측 환경에서 1회 측정:

| 항목 | 실측값 |
| --- | --- |
| 입력 | OSM 284 MB + GTFS zip 211 MB (압축 해제 약 1.48 GB) |
| 소요 시간 | **55초** |
| 최대 메모리 | **3.98 GiB** (3초 간격 `docker stats` 폴링 최대치) |
| 결과 디스크 | **1.5 GB** (`osr` 642 MB, `adr` 343 MB, `tt.bin` 280 MB, `way_matches.bin` 143 MB 등) |

import 태스크는 `osr`·`tt`·`adr`·`adr_extend`·`matches` 5개가 돌며, 시간의 대부분을
`osr`(도로망)이 쓴다. import 후 원본 osm.pbf·GTFS zip은 삭제해도 서버가 뜬다(약 500 MB 회수).

**주의**: 이 수치는 `tiles`를 끈 경우다. 생성된 기본 config를 그대로 쓰면 타일 빌드가
추가되어 시간·디스크가 전혀 다른 규모가 된다.

---

## R11. KTDB GTFS 실측 (U3 해소)

**공식 경로(1순위)**: 공지 "(안내) 2023년 3월 기준 GTFS 기반정보 제공 안내"
(2025-05-12 게시, 2025-05-30 제공 개시).
신청 경로는 국가교통DB 홈페이지 > 정보공개 > 자료신청 > 교통분석자료 신청 >
교통망 GIS DB > 대중교통 > 대중교통. **직접 다운로드 링크가 아니라 자료신청 절차**이며
이용자 만족도 조사 후 내려받는다.

**갱신 주기**: **명시 없음**. 공지에 "파일럿 자료"로 표기. 기준 시점은 **2023년 3월**.

**미러(2순위, 참고)**: Transitous `feeds/kr.json`의 유일한 한국 피드는 개인
유지관리자의 Dropbox 공유 링크다. 2026-08-03 기준 살아 있음(211 MB). R7의 결론대로
문서는 KTDB 원본을 1순위로 안내하고 이 링크는 참고로만 둔다.

**피드 내용**: 파일 **6개뿐** — `agency.txt`, `calendar.txt`, `routes.txt`,
`stop_times.txt`(1.51 GB), `stops.txt`, `trips.txt`.
`calendar_dates.txt`·`shapes.txt`·`transfers.txt` **없음** → 공휴일 예외·노선 형상·환승
규칙이 없다. agency는 `A1,KTDB` 하나.
레코드 수: routes 26,991 / stops 209,628 / trips 347,567.

**`route_type`이 GTFS 표준과 어긋난다**: 24,581개가 `0`(표준상 트램)인데 실제로는
시내·마을버스이고, `7`(표준상 푸니쿨라)은 항공 국내선이다. 실제 질의에서 KTX/SRT가
`AERIAL_LIFT`(type 6), 제주 시내버스가 `TRAM`(0)·`FUNICULAR`(5)로 보고되는 것을 확인했다.

`describeLeg`는 `isBus()`(=`BUS`/`COACH`/`TROLLEYBUS`)일 때만 수단 이름을 붙이고 그 외에는
전부 "N 승차"로 렌더한다. 따라서 **영향을 받는 것은 KTX가 아니라 실제 시내버스다** —
지하철·철도는 원래도 수단 이름 없이 노선명만 쓰므로 달라지는 것이 없고, 버스로 표기되지
않은 버스만 "6900 버스 승차"가 아니라 "6900 승차"가 된다. 문구가 덜 구체적일 뿐 출력이
깨지지는 않으며, 노선명은 모든 경우에 정확하다.

---

## R12. 만료 타임테이블에서의 응답 (U4 해소)

두 가지를 구분해 실측했다.

**(가) 과거 날짜만 있는 타임테이블** — `calendar.txt`가 `20200101~20201231`뿐인 최소
GTFS를 만들어 import·질의:

- `motis import`가 **경고 없이 성공한다**. 운행일이 하나도 없다는 신호를 주지 않는다.
- `plan`은 **HTTP 200 + `itineraries: []` + `direct: []`** 를 반환한다.
- → naeryeo는 이를 `core.ErrNoRoute` → **`no_route`** 로 분류한다.

**(나) 적재 범위 밖 질의 시각** — `first_day: TODAY`, `num_days: 365`이므로 창 밖 시각은:

```
HTTP 400
{"error":"query time 2027-12-01 09:00 is outside of loaded timetable window
          [2026-08-02 00:00, 2027-08-03 00:00["}
```

- → naeryeo는 4xx를 `*core.ErrMotisRejected{Status:400}` → **`motis_rejected`** 로 분류한다.

**Decision**: FR-016의 "시간표 최신 여부 확인" 힌트는 **`no_route`** 에 붙이는 것이 맞다.
(가)가 실사용자가 만나는 형태이고, 실패가 조용하기 때문이다.

**이번 KTDB 피드에는 (가)가 해당하지 않는다**. `calendar.txt`가
`B1,1,1,1,1,1,1,1,20170101,20301231` 단 한 줄 — 서비스 1개가 **2030년까지 매일** 운행으로
선언돼 있다. 따라서 시간표는 만료되지 않으며 오늘 날짜 질의가 정상 동작한다.
**그러나 시각표 내용은 2023년 3월 기준**이다. 즉 이 피드의 실제 위험은 `no_route`가 아니라
**3년 전 시각표를 오늘의 답으로 조용히 제시하는 것**이다. 실패보다 나쁜 실패 모드이므로
코드가 아니라 문서(FR-024)가 다뤄야 한다. 부작용으로 평일/주말·공휴일 구분도 없다.

---

## R13. 대표 질의 3종 (U5 해소, SC-007)

**지오코딩**: 강남역·홍대입구역·서면역·해운대역·대전역·광주송정역 6개 모두
**첫 매치가 정확한 `type=STOP`** 이었다. Kakao 없이 MOTIS 내장 색인만으로 해석된다
(SC-002 실증).

**범위가 R4의 예상보다 넓다 — 후속 기능의 전제가 바뀐다.** R4는 "건물명·주소 검색에서
Kakao를 MOTIS geocode로 대체"를 후속 과제로 남겼으나, `--geocoder=none` 상태에서 실측한
결과 **이미 동작한다**:

| 입력 | MOTIS 응답 | 결과 |
| --- | --- | --- |
| `아이디스 타워` | `type=PLACE` | `아이디스 타워 → 수지구청` 51분 경로 성공 |
| `테헤란로 152` | `type=ADDRESS` | `테헤란로 152 → 강남역` 11분 경로 성공 |

`osm.pbf`가 도로망뿐 아니라 건물·장소·주소까지 색인하기 때문이다(R4가 인용한 README 서술이
실제로 이 범위였다). 따라서 Kakao는 **기능을 여는 스위치가 아니라 MOTIS 색인에 없는 이름을
위한 적중률 보완**이다 — `internal/motis/client.go`의 `resolvePlace`가 MOTIS 결과가 있으면
거기서 반환하고 `c.Geocoder`를 아예 호출하지 않는 구조가 이를 뒷받침한다.

이 사실을 놓친 채 작성한 `docs/self-hosting.md` §7과 `README.md`의 "건물명·주소는 Kakao 키를
등록한 경우에만 동작합니다" 서술은 **거짓이었고 수정했다**. 자체 호스팅의 가치 주장이
실제보다 약하게 적혀 있던 셈이다.

**경로 검색** — 실제 `naeryeo route` CLI, 상용 API 키 없음:

| 질의 | 결과 |
| --- | --- |
| 강남역 → 홍대입구역 (수도권 도시철도) | 약 47분, 환승 0회, 서울2호선 |
| 서면역 → 해운대역 (지방 광역시) | 약 33분, 환승 0회, 부산2호선 |
| 대전역 → 광주송정역 (도시 간) | 약 93분, 환승 1회, KTX 경부선 + SRT 호남선 |

세 질의 모두 성공 — **커버리지 한계로 문서에 적을 실패 질의가 없다**.
요금은 세 결과 모두 부재해 `FareKnown=false` → 프로즈 `요금 정보 없음`,
JSON에서 `fareWon` 키 **부재**를 실측 확인했다(FR-010/FR-011).

**발견·수정된 출력 결함**: 여정의 양 끝 지점명이 MOTIS의 리터럴 `START`/`END`로 내려와
한국어 문구에 그대로 새어 나왔다 — "START에서 강남까지 도보 이동 (8분)". 테스트 픽스처는
모든 지점에 이름을 주므로 실엔진 검증에서만 드러났고, ODsay 경로에는 없던 현상이다.
`placeName()`으로 각각 `출발지`/`도착지`로 렌더하도록 고쳤다(테이블 테스트 4건 추가).

**소요 시간의 변동**: 도시 간 질의는 같은 날에도 질의 시각에 따라 93분~159분으로 갈렸다.
연결편 선택이 달라지기 때문이며 정상이다. 문서의 완료 판정 기준을 분 단위가 아니라
"경로가 나오는가"로 둔 이유다.

**실패 경로 실증**(엔진 정지 후): `motis_unavailable` 코드, `docs` 링크 포함,
프로즈 3줄(message/hint/docs), 출력 전체에 `localhost`·`8080` **0건**(SC-006).

**MCP 진입점도 실엔진으로 확인했다**(SC-005). 여기까지의 검증이 전부 `route` CLI를 통한
것이었고 MCP 쪽은 T023 단위 테스트("두 진입점이 같은 제공자를 쓴다")로만 담보돼 있었다.
`naeryeo mcp`에 stdio로 `initialize` → `tools/call find_transit_route`를 직접 흘려 넣어
같은 엔진에서 응답을 받았다.

```json
{"steps": ["출발지에서 신논현까지 도보 이동 (9분)",
           "신논현에서 서울9호선 승차 → 당산에서 하차", ...],
 "totalTimeMinutes": 36, "transferCount": 1}
```

`fareWon` 키 **부재**(FR-010/FR-011)와 `START`/`END` 치환이 CLI뿐 아니라 MCP
`structuredContent`에서도 성립함을 이로써 확인했다. 두 계약 모두 그동안 CLI 경로에서만
실측돼 있었다.

**`motis_rejected`를 naeryeo를 통해서도 재현했다.** 그전까지는 적재 창 밖 질의를 curl로
직접 쳐서 400을 본 것이 전부였고, 이는 R12에서 밝혔듯 **사용자가 도달할 수 없는 경로**다.
문서에 새로 쓴 "주소가 MOTIS 서버가 아닌 경우"를 검증하기 위해 루프백에 MOTIS가 아닌 HTTP
서버를 띄우고 두 단계를 확인했다.

| 단계 | 결과 |
| --- | --- |
| `setup --motis-url=<비-MOTIS>` | 저장 **거부** — "엔진이 요청을 거부했습니다. 주소가 MOTIS 서버가 맞는지 확인하세요" + docs 링크. **기존 설정 파일은 그대로 유지**됐다 |
| setup 우회 후 `route` | `motis_rejected` + docs 링크, 프로즈 3줄. 출력에 호스트·포트 **0건** |

setup 거부 문구는 `docs/self-hosting.md` §5에, 검색 시점 동작은 §8-B에 적어 둔 그대로다 —
문서가 실제 출력과 일치함을 확인한 셈이다.
