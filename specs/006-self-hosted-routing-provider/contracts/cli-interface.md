# Contract: CLI 명령 표면

**Feature**: 006-self-hosted-routing-provider

**⚠️ 이 문서는 breaking change를 포함한다.** v1 릴리스로 나가는 것이 전제다(spec Assumptions).

## 명령 목록 (변경 후)

```text
usage: naeryeo <setup|route|mcp|--version>
```

| 명령 | 상태 |
| --- | --- |
| `naeryeo setup` | **재설계** — 다단계 마법사 |
| `naeryeo route` | 변경 없음 (출력에 요금 부재 케이스 추가) |
| `naeryeo mcp` | 변경 없음 |
| `naeryeo --version` | 변경 없음 |
| ~~`naeryeo logout`~~ | **삭제** → `setup --delete` |

## `naeryeo setup`

### 대화식 (인자 없음)

```text
$ naeryeo setup

경로 검색 제공자를 선택하세요.
  1) 자체 호스팅 (MOTIS) — API 키가 필요 없습니다  [기본]
  2) ODsay — 앱키 발급이 필요합니다
  3) 저장된 자격증명 삭제
선택 [1]: ⏎

MOTIS 서버 주소를 입력하세요.
주소 [http://localhost:8080]: ⏎

  연결 확인 중... 정상

건물명·주소로도 검색하시겠습니까? (역·정류장 이름만 쓸 경우 필요 없습니다)
  1) 사용 안 함  [기본]
  2) Kakao 장소 검색 사용 — REST API 키가 필요합니다
선택 [1]: 2
Kakao REST API Key: ****************

설정 요약
  경로 검색: 자체 호스팅 (MOTIS) — http://localhost:8080
  장소 검색: Kakao
저장하시겠습니까? [Y/n]: ⏎

저장 완료
```

**구현 제약**:

- **TUI 프레임워크를 도입하지 않는다.** 번호 프롬프트 루프로 구현한다. 근거: 현재
  `runSetup(args, stdin, stdout, stderr, save)`의 라인 기반 시그니처가 fake stdin 주입
  테스트를 가능하게 하고 있고(`setup_test.go`), TUI를 넣으면 이 테스트 모델이 통째로 깨져
  pty/teatest 하네스가 필요해진다. 헌법 원칙 I.
- 부수 효과로 **대화식과 비대화식이 같은 코드 경로**가 된다 — tty든 파이프든 동일 동작.
- 기본값(`[1]`, `[http://localhost:8080]`)은 Enter로 수락된다. MOTIS가 1번인 것이 사용자
  지시("기본 프로바이더는 MOTIS")의 반영 지점이다.

### 비대화식

플래그가 주어진 단계는 프롬프트를 건너뛴다.

```bash
naeryeo setup --provider=motis --motis-url=http://motis.lan:8080   # 시크릿 없음
naeryeo setup --provider=odsay                                     # 키는 stdin
naeryeo setup --geocoder=kakao                                     # 키는 stdin
naeryeo setup --geocoder=none
naeryeo setup --delete=odsay|kakao|all
```

| 플래그 | 타입 | 허용 값 | 비고 |
| --- | --- | --- | --- |
| `--provider` | string | `motis` \| `odsay` | |
| `--motis-url` | string | 절대 URL | `--provider=motis`일 때만 유효 |
| `--geocoder` | string | `kakao` \| `none` | **breaking: 기존 bool** |
| `--delete` | string | `odsay` \| `kakao` \| `all` | 다른 플래그와 함께 쓸 수 없음 |

**시크릿을 받는 플래그는 존재하지 않는다** (spec FR-006). `--api-key` 같은 것을 두지 않는
이유는 셸 히스토리와 `ps` 출력에 평문으로 남기 때문이며, 이는 GYE-293(키 유출 차단)이 세운
방향과 정반대다. 비밀값은 언제나 stdin으로만 받는다:

```bash
echo "$ODSAY_KEY" | naeryeo setup --provider=odsay
echo "$KAKAO_KEY" | naeryeo setup --geocoder=kakao
```

### `--geocoder`의 breaking change

| 이전 | 이후 |
| --- | --- |
| `naeryeo setup --geocoder` (bool, Kakao 키 등록) | `naeryeo setup --geocoder=kakao` |
| — | `naeryeo setup --geocoder=none` (지오코더 사용 안 함으로 설정) |

`--geocoder`를 값 없이 넘기면 `flag` 패키지가 다음 인자를 값으로 먹으므로, 구버전 습관으로
`naeryeo setup --geocoder`만 치면 파싱 실패한다. 이때 **"이제 `--geocoder=kakao` 형태로
지정해야 합니다"라는 안내를 낸다** — 조용한 실패는 마이그레이션 비용을 사용자에게 떠넘긴다.

### `--delete` (logout 대체)

```bash
naeryeo setup --delete=odsay   # ODsay 키만 삭제
naeryeo setup --delete=kakao   # Kakao 키만 삭제
naeryeo setup --delete=all     # 둘 다 삭제
```

- 기존 `logout`의 멱등 동작과 "삭제할 키가 없습니다" 구분을 **그대로 유지**한다
  (spec 001 FR-009, 현 `logout.go:47-50`).
- **설정 파일은 건드리지 않는다.** 제공자 선택은 남는다(data-model.md §1).

### 종료 코드

| 상황 | 코드 |
| --- | --- |
| 저장 성공 / 삭제 성공 | 0 |
| 플래그 조합 오류, 값 검증 실패 | 1 |
| MOTIS 도달성 프로브 실패 | 1 |
| 키체인 사용 불가 | 1 |
| 사용자가 요약 단계에서 취소 | 1 |

`setup`은 `--json`을 지원하지 않는다 — 대화형 설정은 사람과의 대화가 목적이고, spec 005가
기계 판독 모드의 적용 범위를 경로 검색 명령으로 한정했다.

## `naeryeo route` — 출력 변화

### 요금 정보가 없는 경우 (MOTIS)

**프로즈**:

```text
강남역 → 홍대입구역 (약 42분, 환승 1회)

1. 강남역에서 2호선 승차 → 신도림역에서 하차
2. 신도림역에서 경의중앙선 승차 → 홍대입구역에서 하차

요금 정보 없음
```

기존 `요금: 1,500원` 자리에 `요금 정보 없음`이 들어간다. **ODsay 경로에서는 이 줄이 나오지
않는다** — `FareKnown`이 항상 `true`라 기존 출력이 바이트 단위로 유지된다.

**JSON**: `fareWon` 필드가 **부재**한다.

```json
{"totalTimeMinutes":42,"transferCount":1,"steps":["...","..."]}
```

`fareWon`을 `0`으로 채우지 않는 것이 spec FR-010("0 같은 유효값으로 위장 금지")의 요구다.
Go 타입은 `int` → `*int`로 바뀌지만 값이 있을 때의 와이어 포맷은 동일하므로 spec 005의 성공
스키마 계약은 유지된다.

### 사전 실패 (제공자 미설정)

```bash
$ naeryeo route --from 강남역 --to 홍대입구역
naeryeo route: 경로 검색 제공자가 설정되지 않았습니다
naeryeo setup을 실행해 자체 호스팅(MOTIS) 또는 ODsay 중 하나를 선택하세요
https://github.com/GyeongHoKim/naeryeo/blob/main/docs/self-hosting.md
```

```bash
$ naeryeo route --from 강남역 --to 홍대입구역 --json
{"error":{"code":"provider_not_configured","message":"...","hint":"...","docs":"..."}}
```

종료 코드 1, JSON은 stdout (spec 005 FR-008).

## `naeryeo mcp` — 변화

명령 표면은 그대로다. 내부적으로 `route`와 **같은** `routeFinder`를 쓰므로 제공자가 자동으로
일치한다. 툴 스키마에서 바뀌는 것은 둘:

- `RouteToolOutput.fareWon` — 부재 가능 (위와 동일)
- `RouteError.docs` — 신규 선택 필드

## 테스트 계약

| 단언 | 근거 |
| --- | --- |
| `naeryeo logout`이 unknown command로 실패하고 usage에 `logout`이 없음 | logout 제거 |
| 마법사의 모든 단계가 fake stdin으로 구동됨 (pty 불필요) | 기존 주입 패턴 유지 |
| 플래그 조합 테이블: provider×url×geocoder×delete의 유효/무효 케이스 | FR-005/006 |
| 시크릿을 받는 플래그가 **존재하지 않음** (FlagSet 순회로 단언) | FR-006 |
| `--delete` 후 설정 파일이 그대로 남아 있음 | data-model.md §1 |
| `--geocoder` 구형식 사용 시 마이그레이션 안내가 출력됨 | 마이그레이션 비용 |
| `FareKnown=false`인 결과의 프로즈에 `요금 정보 없음`이, JSON에 `fareWon` 키가 없음 | FR-010 |
| ODsay 경로의 프로즈 출력이 변경 전과 바이트 동일 | spec 005 FR-007 |
| 설정 미비 상태에서 `route`와 MCP 툴이 같은 코드·문구를 반환 | FR-002 |
