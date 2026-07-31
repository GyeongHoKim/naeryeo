# Implementation Plan: 자체 호스팅 경로 검색 제공자

**Branch**: `feature/006-self-hosted-routing-provider` | **Date**: 2026-07-31 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-self-hosted-routing-provider/spec.md`

## Summary

MOTIS(오픈소스 멀티모달 라우팅 엔진)를 사용자가 자기 장비에서 운영하고 naeryeo가 그것을
경로 제공자로 쓸 수 있게 한다. 경로 제공자 선택을 **평문 설정 파일**로 영속화해 CLI와 MCP가
같은 값을 읽게 하고(FR-002), `setup`을 제공자·자격증명·지오코더를 한 자리에서 처리하는
다단계 마법사로 재설계하며(FR-005), `logout`을 `setup --delete`로 흡수한다. 새로 생기는
실패 상황은 spec 005가 만든 에러 코드 taxonomy 위에 얹어 `--json`으로 AI가 판별할 수 있게
한다(FR-014~020).

**핵심 설계 판단 3가지** (research.md에 근거 기록):

1. **MOTIS의 내장 지오코더(`/api/v1/geocode`)를 이름 해석에 사용한다.** MOTIS의 plan
   엔드포인트는 좌표 또는 stop ID만 받고 "강남역" 같은 이름을 받지 않는다. 즉 이름 해석은
   선택 기능이 아니라 MOTIS 제공자가 동작하기 위한 **필수 부품**이다. MOTIS 지오코더는
   ODsay의 `searchStation`과 같은 자리를 차지하고, 기존 Kakao 지오코더는 지금과 똑같이
   **선택적 폴백**으로 남는다 — spec FR-028(장소 검색 현행 유지)과 FR-030(독립 축)이
   그대로 성립한다.
2. **MOTIS 클라이언트는 `internal/motis`에 두고 `internal/core`를 단방향 의존한다.**
   `internal/geocode`(Kakao)가 이미 쓰고 있는 패턴을 그대로 따른다. `core`를 순수 도메인
   패키지로 분리하는 대규모 리네임은 하지 않는다 — 동작이 바뀌지 않는 리네임에 모든 테스트
   파일을 건드리는 비용을 정당화할 수 없다(헌법 원칙 I).
3. **"요금 정보 없음"을 0원과 구조적으로 구분한다.** MOTIS의 요금 지원은 실험적이고 KTDB
   GTFS에는 요금 정보가 없을 가능성이 높다. `RouteResult`에 `FareKnown`을 두고 JSON에서는
   `fareWon`을 포인터로 바꿔 **부재**로 표현한다(FR-010).

## Technical Context

**Language/Version**: Go 1.26.4 (`go.mod`)

**Primary Dependencies**: 표준 라이브러리(`net/http`, `encoding/json`, `flag`, `log/slog`),
`github.com/zalando/go-keyring` v0.2.8, `github.com/modelcontextprotocol/go-sdk` v1.6.1.
**새 Go 의존성은 추가하지 않는다** — MOTIS 연동은 `net/http` + `encoding/json`으로 충분하고,
설정 파일도 JSON이라 stdlib로 끝난다.

**External service (new)**: MOTIS — 사용자가 self-host. 이미지 `ghcr.io/motis-project/motis`,
HTTP :8080, `GET /api/v6/plan`, `GET /api/v1/geocode`. 데이터는 OSM pbf(Geofabrik
south-korea) + GTFS(KTDB, Transitous `feeds/kr.json` 경유).

**Storage**:

- 비밀값(ODsay 키, Kakao 키) → OS 키체인 (기존 `internal/config`, 변경 없음)
- 비밀 아닌 설정(provider, MOTIS URL, geocoder 선택) → `os.UserConfigDir()/naeryeo/config.json`
  (신규). 분리 근거는 FR-004 — MOTIS 사용자는 저장할 비밀이 하나도 없으므로 키체인 프롬프트를
  띄우면 안 된다.

**Testing**: `go test -race ./...` via `just test`. MOTIS/geocode는 `httptest.Server`로,
설정 파일은 `t.TempDir()` + 주입된 경로로, setup 마법사는 fake stdin으로, 키체인은 기존
`keyringBackend` fake로 검증. 전부 기존 패턴 재사용.

**Target Platform**: macOS / Windows / Linux (GYE-296이 세운 3-OS CI 매트릭스). 설정 파일
경로가 OS별로 갈리므로 이 매트릭스가 이번 기능의 실질적 게이트다.

**Project Type**: 단일 Go 모듈 — CLI + MCP stdio 서버, 공용 `internal/` 로직.

**Performance Goals**: MOTIS는 로컬/사내망이므로 ODsay 대비 지연이 줄어든다. 회귀 기준만
유지: 경로 검색 1회당 상위 API 호출 수가 늘지 않을 것(이름 해석 2회 + plan 1회 = ODsay와 동일).
HTTP 타임아웃은 기존 10초를 그대로 쓴다.

**Constraints**:

- `--json` 모드의 stdout에는 문서 하나만 — 진단·경고는 전부 stderr (spec 005 FR-008/FR-014).
- MCP 모드에서 stdout은 JSON-RPC 전용.
- 실패 메시지에 사용자 사설망 호스트·포트가 절대 실리지 않을 것 (FR-018).
- 기존 ODsay 프로즈 출력은 바이트 단위로 불변 (spec 005 FR-007).

**Scale/Scope**: 1인 데스크톱 도구. 코드 규모 추정 — 신규 `internal/motis`(~350줄+테스트),
`internal/config/settings.go`(~120줄+테스트), `cmd/naeryeo/setup.go` 재작성(~250줄+테스트),
`logout.go`/`logout_test.go` 삭제, `errcode.go`에 코드 3개 추가, 문서 3종(README,
SKILL.md, docs/self-hosting.md).

## Constitution Check

*GATE: Phase 0 이전 통과 필수. Phase 1 이후 재확인.*

| 원칙 | 게이트 | 판정 | 근거 |
| --- | --- | --- | --- |
| **I. Idiomatic Go First** | 인터페이스는 작고 **소비자 패키지**가 정의 | ✅ PASS | 새 인터페이스를 만들지 않는다. 라우팅 이음매는 소비자(`cmd/naeryeo`)의 함수 타입 `routeFinder func(ctx, from, to) (core.RouteResult, error)`이고, `core.Geocoder`(기존, 소비자 정의)를 MOTIS 클라이언트가 그대로 재사용한다 |
| | 추측성 추상화 금지 | ✅ PASS | 제공자 이음매는 **두 번째 구현이 실제로 들어오는 커밋에서** 도입된다. 인터페이스를 먼저 만들고 구현을 나중에 붙이지 않는다 |
| | 에러 명시적 처리, `panic` 금지 | ✅ PASS | 신규 에러는 기존 sentinel/구조체 패턴(`ErrGeocoder*` 형태)을 따르고 `errors.Is/As`로 분류 |
| | 컴포지션 우선 | ✅ PASS | `motis.Client`는 `core.Client`를 임베드하지 않는다. `core`의 도메인 타입만 소비 |
| **II. Unit Tests Are Mandatory** | 신규 exported 심볼과 동일 커밋에 테스트 | ✅ PASS | 아래 §6 참조. 신규 exported: `motis.Client`/`NewClient`/`FindRoute`, `config.Settings`/`LoadSettings`/`SaveSettings`, `core.ErrMotis*` |
| | 테이블 주도 선호 | ✅ PASS | 에러 분류, 설정 파싱, setup 플래그 조합 전부 테이블 |
| | 커버리지 비회귀 | ✅ PASS | 삭제되는 `logout.go`는 테스트와 함께 사라지므로 비율 왜곡 없음 |
| **III. Automated Quality Gates** | `just fmt` → `just lint` → `just test` 전부 green | ✅ 계획됨 | tasks 단계의 모든 체크포인트와 완료 조건에 포함 |
| **IV. Commit Discipline** | Conventional Commits + 사람 확인 후 커밋 | ✅ 계획됨 | 에이전트가 임의 커밋하지 않는다. 이 계획도 커밋 제안까지만 |

**추가 게이트 (spec 005가 세운 것, 이번 기능이 반드시 통과해야 함)**:

- `TestErrorCodeExhaustive_EveryCoreErrorHasACode` — `internal/core`에 exported 에러를
  추가하면 코드 부여를 강제한다. 신규 `ErrMotis*`는 이 게이트를 **먼저 깨뜨린 뒤**
  taxonomy 등록으로 통과시키는 순서로 진행한다(FR-020이 요구하는 동작 그 자체).
- `TestRunRouteJSON_SuccessMatchesMCP` — CLI `--json`과 MCP 성공 스키마 동일성. `fareWon`
  타입 변경이 이 테스트를 통과해야 한다(FR-011).

**Complexity Tracking**: 위반 없음 — 표 생략.

## Project Structure

### Documentation (this feature)

```text
specs/006-self-hosted-routing-provider/
├── plan.md              # 이 파일
├── research.md          # Phase 0 — MOTIS API/배포 조사와 설계 결정
├── data-model.md        # Phase 1 — 엔티티와 상태 전이
├── quickstart.md        # Phase 1 — 검증 시나리오
├── contracts/
│   ├── settings-file.md # 설정 파일 스키마와 경로
│   ├── cli-interface.md # setup/route 명령 표면 (breaking change 포함)
│   └── error-codes.md   # spec 005 taxonomy에 대한 delta
├── checklists/
│   └── requirements.md  # /speckit-specify 산출물
└── tasks.md             # Phase 2 — /speckit-tasks가 생성 (이 명령은 만들지 않음)
```

### Source Code (repository root)

```text
cmd/naeryeo/
├── main.go              # 수정: logout 케이스 제거, usage 문자열, newFindRoute → 제공자 선택
├── setup.go             # 재작성: 다단계 마법사 + 비대화식 플래그 + --delete
├── setup_test.go        # 재작성
├── logout.go            # 삭제 (logout 제거)
├── logout_test.go       # 삭제
├── route.go             # 수정: 사전 실패(provider 미설정) 경로, fareWon 포인터화
├── errcode.go           # 수정: provider_not_configured / motis_unavailable / motis_rejected + Docs 필드
├── errcode_exhaustive_test.go  # 수정: 신규 core 에러 샘플 추가
└── mcp.go               # 수정: RouteToolOutput.FareWon *int, RouteError.Docs

internal/config/
├── config.go            # 변경 없음 (키체인)
├── settings.go          # 신규: Settings 로드/저장, os.UserConfigDir 기반 경로
└── settings_test.go     # 신규

internal/core/
├── client.go            # RouteResult.FareKnown 추가 (ODsay 동작은 불변)
└── errors.go            # 수정: ErrMotisUnavailable, ErrMotisRejected 추가

internal/motis/
├── client.go            # 신규: /api/v1/geocode + /api/v6/plan → core.RouteResult
├── client_test.go       # 신규 (httptest)
└── doc.go               # 신규

internal/geocode/        # 변경 없음 (Kakao)

docs/
└── self-hosting.md      # 신규: MOTIS 구축 레시피 (FR-021~024)

deploy/motis/
├── compose.yaml         # 신규: 검증된 docker compose 레시피
└── README.md            # 신규: deploy 디렉터리 안내 (docs/self-hosting.md로 연결)

README.md                # 개편: provider 개념, logout 제거, self-hosting 1순위 (FR-025)
skills/naeryeo/SKILL.md  # 개편: provider 개념, 코드 분기, AI 임의 설치 금지 (FR-026/027)
```

**Structure Decision**: 기존 단일 Go 모듈 레이아웃을 유지한다. 신규 `internal/motis`는
`internal/geocode`가 이미 확립한 패턴(외부 제공자 어댑터가 `internal/core`의 도메인 타입과
에러 계약을 단방향 소비)을 그대로 따르므로 새로운 구조 개념을 도입하지 않는다. 문서 산출물은
`docs/`(사람이 읽는 레시피)와 `deploy/motis/`(그대로 실행하는 파일)로 나눈다 — 실패 안내가
가리키는 링크는 `docs/self-hosting.md` 하나로 고정한다.

## 설계 상세

### 1. 제공자 이음매

현재 `newFindRoute`의 시그니처는 ODsay 전용이다:

```go
func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error)
```

`apiKey`가 파라미터로 흐르는 형태는 MOTIS(키 없음)를 표현할 수 없다. 이음매를 **자격증명이
포함되지 않은 형태**로 좁힌다:

```go
// cmd/naeryeo — 소비자가 정의하는 이음매
type routeFinder func(ctx context.Context, from, to string) (core.RouteResult, error)

// 설정과 자격증명을 읽어 제공자를 고른다. 검색 이전에 확정되는 실패
// (provider 미설정 / 키 없음 / 키체인 오류)는 여기서 failure로 확정된다.
func newRouteFinder(logger *slog.Logger) (routeFinder, *failure)
```

`route`와 `mcp`가 이 함수 하나를 공유하므로 두 경로의 제공자 불일치가 **구조적으로**
불가능해진다(FR-002, SC-005). 인터페이스가 아니라 함수 타입인 이유는 메서드가 하나뿐이고
소비자가 필요로 하는 것이 정확히 그 하나이기 때문이다(헌법 원칙 I).

### 2. MOTIS 클라이언트의 이름 해석

```text
FindRoute(ctx, from, to)
  ├─ resolvePlace(from) ─┐
  │    1) GET /api/v1/geocode?text=<from>   (MOTIS 내장 — GTFS 정류장 + OSM 주소)
  │    2) 매치 없음 && Geocoder != nil → Kakao 폴백 (기존 core.Geocoder 계약 재사용)
  │    3) 그래도 없음 → *core.ErrPointNotFound{Side:"from"}
  ├─ resolvePlace(to) ───┘
  └─ GET /api/v6/plan?fromPlace=<lat,lon>&toPlace=<lat,lon>&...
       → itineraries[0] → core.RouteResult
```

`core.Client`(ODsay)의 `resolveStation` → `geocodeFallback` 흐름과 **같은 모양**이다.
`core.Geocoder` 인터페이스와 그 에러 계약(`ErrGeocoderNotFound` / `ErrGeocoderAuthFailed` /
`ErrGeocoderUnavailable`)을 그대로 재사용하므로 Kakao 쪽은 한 줄도 바뀌지 않는다.

### 3. 실패 분류

| 상황 | core 에러 | 코드 | 재시도 | docs |
| --- | --- | --- | :---: | :---: |
| 설정 파일 없음 / provider 미지정 | (cmd 레이어에서 직접 생성) | `provider_not_configured` | ❌ | ✅ |
| MOTIS 연결 불가·타임아웃·5xx | `core.ErrMotisUnavailable` | `motis_unavailable` | ✅ | ✅ |
| MOTIS 4xx / 응답 해석 불가 | `*core.ErrMotisRejected` | `motis_rejected` | ❌ | ✅ |

`RouteError`에 `docs` 필드를 추가한다(spec 005 contracts가 예고해 둔 확장). 프로즈 출력에도
같은 링크를 텍스트로 싣는다(FR-017).

**FR-016(응답은 하나 처리 불가)의 처리**: MOTIS가 타임테이블 미적재 상태에서 어떤 응답을
내는지는 문서에 없다(research.md R6, 미해결). 런타임 추측에 의존하지 않고 **setup 시점
검증**으로 결정론적으로 만든다 — `setup --provider=motis`가 URL 저장 전에 도달성 확인과
지오코드 프로브를 수행하고, 엔진이 응답하지만 데이터가 없으면 저장을 거부한다. 런타임에
남는 애매함은 구현 중 실측해 `motis_rejected`의 hint로 흡수한다.

### 4. 설정 파일

`os.UserConfigDir()/naeryeo/config.json`:

```json
{
  "routing_provider": "motis",
  "motis_url": "http://localhost:8080",
  "geocoder": "none"
}
```

JSON을 쓰는 이유는 stdlib로 끝나기 때문이다 — YAML/TOML은 새 의존성을 요구하고, 사람이 손으로
편집하는 것을 1차 인터페이스로 삼지 않으므로(설정은 `setup`이 쓴다) 가독성 이득이 비용을
정당화하지 못한다. 자세한 스키마와 검증 규칙은
[contracts/settings-file.md](./contracts/settings-file.md).

### 5. setup 명령 표면 (breaking change)

```bash
naeryeo setup                                    # 대화식 마법사
naeryeo setup --provider=motis --motis-url=URL   # 비밀값 없음
naeryeo setup --provider=odsay                   # 키는 stdin
naeryeo setup --geocoder=kakao                   # 키는 stdin
naeryeo setup --geocoder=none
naeryeo setup --delete=odsay|kakao|all           # logout 대체
```

- **기본 제공자는 MOTIS**다 — 마법사의 1번 선택지이자 Enter 기본값이고, 문서의 1순위 경로다.
  단, 설정 파일이 없을 때 MOTIS로 **암묵 폴백하지는 않는다**. 무설정 상태는 여전히
  `provider_not_configured`로 실패한다(spec FR-008/FR-031 — 사용자가 직전에 고른 "재설정
  강제"). "기본값"은 *선택지의 기본*이지 *무설정의 기본*이 아니다.
- 시크릿을 받는 플래그는 만들지 않는다(FR-006). 셸 히스토리·`ps` 노출 회피.
- `--geocoder`가 bool → 문자열로 바뀐다(breaking, 허용됨).
- TUI 프레임워크를 도입하지 않는다 — 번호 프롬프트 루프로 구현해 기존 fake stdin 테스트
  모델을 유지한다.

전체 표면은 [contracts/cli-interface.md](./contracts/cli-interface.md).

### 6. 테스트 전략

| 대상 | 방식 | 근거 |
| --- | --- | --- |
| `motis.Client` | `httptest.Server`로 geocode/plan 응답 고정, 테이블 주도 | `geocode/kakao_test.go`가 쓰는 패턴 그대로 |
| 이름 해석 폴백 | MOTIS geocode 빈 결과 + fake `core.Geocoder` 조합 | `core/client_test.go`의 폴백 테스트와 동형 |
| 요금 없음 | plan 응답에 요금 부재 → `FareKnown=false` → JSON에 `fareWon` 부재 | FR-010 |
| 호스트 비노출 | 사설망 URL로 실패 유발 후 출력 전체에 호스트·포트 부재 단언 | FR-018, SC-006 |
| 설정 파일 | `t.TempDir()` + 경로 주입, OS별 경로는 `os.UserConfigDir` 계약만 단언 | 3-OS CI |
| setup 마법사 | fake stdin으로 전 단계, 플래그 조합 테이블 | FR-005/006 |
| 제공자 일치 | 같은 설정으로 `route`와 `mcp` 양쪽 실행 → 동일 제공자 단언 | FR-002, SC-005 |
| 망라성 | 기존 `TestErrorCodeExhaustive_*`에 `ErrMotis*` 샘플 추가 | FR-020, SC-004 |

### 7. 진행 순서 (의존 관계)

```text
① 설정 파일(internal/config/settings.go)
   └─② core 에러 + taxonomy 코드 (망라성 게이트 통과)
      ├─③ internal/motis 클라이언트
      └─④ setup 재설계 + logout 제거
         └─⑤ newRouteFinder 배선 (route·mcp 공유)
            └─⑥ 문서 (docs/self-hosting.md → README → SKILL.md)
```

③과 ④는 병렬 가능하다. ⑥의 `docs/self-hosting.md`는 ②의 `docs` 필드 링크 대상이므로
경로를 먼저 확정하고 내용은 나중에 채워도 된다. 각 단계 종료 시 `just check`가 green이어야
다음 단계로 넘어간다(헌법 원칙 III).

## Phase 0 → Phase 1 아티팩트

- [research.md](./research.md) — MOTIS API·배포 조사, 설계 결정, 미해결 항목
- [data-model.md](./data-model.md) — 엔티티, 상태 전이, 검증 규칙
- [contracts/settings-file.md](./contracts/settings-file.md)
- [contracts/cli-interface.md](./contracts/cli-interface.md)
- [contracts/error-codes.md](./contracts/error-codes.md)
- [quickstart.md](./quickstart.md) — 검증 시나리오

## Constitution Re-Check (Phase 1 이후)

| 원칙 | 재확인 결과 |
| --- | --- |
| I | 설계 산출물에 새 인터페이스가 없다. `core.Geocoder` 재사용 + 소비자 정의 함수 타입 하나. `internal/motis`는 `internal/core` 단방향 의존 — 순환 없음 |
| II | contracts의 모든 항목이 테스트 가능한 형태로 서술되어 있고, quickstart의 시나리오가 자동 테스트로 옮겨질 수 있는 단위로 쪼개져 있다 |
| III | 설계 순서 §7의 각 단계 종료 조건에 `just check`가 포함된다 |
| IV | 이 계획은 커밋을 만들지 않는다. 커밋은 사람 확인 후 |

**판정: PASS** — Complexity Tracking 항목 없음.

## 참고한 Linear 이슈

사용자 지시에 따라 아래 이슈를 근거로 삼았다. 이 계획은 세 이슈의 범위를 spec 006 하나로
통합한 것이다.

| 이슈 | 상태 | 이 계획에 반영된 부분 |
| --- | --- | --- |
| [GYE-294](https://linear.app/gyeongho/issue/GYE-294) | Backlog | 설정 저장소 이원화, setup 다단계화, logout 제거, 비대화식 플래그, 재설정 강제 → §4·§5 |
| [GYE-295](https://linear.app/gyeongho/issue/GYE-295) | Backlog | 제공자 이음매, MOTIS 클라이언트, MOTIS 에러 정의, SKILL.md 갱신 → §1·§2·§3 |
| [GYE-288](https://linear.app/gyeongho/issue/GYE-288) | Backlog | Docker 레시피 검증, 데이터 확보처, 실측 자원 요구치, README → `deploy/motis/`·`docs/self-hosting.md` |
| [GYE-292](https://linear.app/gyeongho/issue/GYE-292) | Done | 이 계획이 올라타는 taxonomy·`--json` 기반. 예고된 확장(`provider_not_configured`, `motis_*`, `docs`)을 여기서 실현 |
| [GYE-293](https://linear.app/gyeongho/issue/GYE-293) | Done | 원본 URL 비노출 원칙을 MOTIS 엔드포인트로 확장 → FR-018 |

**계획과 이슈가 갈리는 지점 1건**: GYE-295는 `internal/core`에 routing provider **인터페이스**를
도입하는 것을 범위로 잡았으나, 이 계획은 인터페이스 대신 소비자 측 함수 타입을 쓴다. 메서드가
하나뿐인 이음매에 인터페이스 선언을 추가하면 `core`가 자기 소비자의 형태를 알게 되어 헌법 원칙
I("소비하는 패키지가 인터페이스를 정의한다")과 어긋난다. 결과적으로 얻는 것 — 제공자 교체
가능성 — 은 동일하다.
