# Contract: CLI `--json` 출력

**Feature**: 005-structured-output-contract | **Command**: `naeryeo route`

## 플래그

```
naeryeo route --from <출발지> --to <도착지> [--json] [--debug]
```

| 플래그 | 기본 | 의미 |
| --- | --- | --- |
| `--json` | off | 출력 형식을 JSON 문서로 전환 |
| `--debug` | off | 진단 정보 추가 (기존 플래그, 동작 불변) |

두 플래그는 **조합 가능**하다 (FR-014).

## 스트림·종료 코드 매트릭스

| 모드 | 결과 | stdout | stderr | exit |
| --- | --- | --- | --- | :---: |
| 기본 | 성공 | 기존 프로즈 | — | 0 |
| 기본 | 실패 | — | `naeryeo route: <message>[\n<hint>]` | 1 |
| 기본 + `--debug` | 실패 | — | 위 + `\n[debug] <원본 체인>` | 1 |
| `--json` | 성공 | 성공 문서 | — | 0 |
| `--json` | 실패 | **실패 문서** | — | 1 |
| `--json --debug` | 실패 | 실패 문서 | `[debug] <원본 체인>` | 1 |

**핵심 규칙**

- `--json`이면 성공·실패 **모두 stdout에 JSON 문서 하나** (FR-008). 실패 시 stdout이 비지
  않는다 — 출력을 한 번만 캡처하는 호출자가 실패 이유를 잃지 않기 위함.
- 실패는 원인과 무관하게 **exit 1** (FR-009). 코드별 분화 없음.
- `--json` 미지정 시 출력·스트림·종료 코드는 **현재와 완전히 동일**하다 (FR-007).
  예외: 기존에 `default:` 분기로 원본 에러가 새던 경우의 문구는 바뀐다 (FR-005).
- `--json` 지정 시 `--debug` 원본 체인은 **stderr로만** 나가 stdout 문서의 파싱 가능성을
  보전한다 (R5).

## 성공 문서

```json
{
  "totalTimeMinutes": 42,
  "transferCount": 1,
  "fareWon": 1500,
  "steps": [
    "강남역에서 2호선 승차 (구로디지털단지 방면)",
    "신도림역에서 2호선 → 경의중앙선 환승",
    "홍대입구역 하차"
  ]
}
```

이동 불필요:

```json
{ "noTravelNeeded": true }
```

**필드 존재 규칙 — 없는 필드는 "0"이지 "알 수 없음"이 아니다.**
모든 성공 필드는 `omitempty`라, 제로값이면 문서에서 빠진다. 환승이 없는 직통 경로는
`transferCount`가, 무료 구간은 `fareWon`이 나타나지 않는다.

```json
{ "totalTimeMinutes": 18, "steps": ["강남역에서 2호선 승차 → 역삼역에서 하차"] }
```

위 문서는 "환승 0회, 요금 0원"을 뜻한다. 호출자는 키 존재를 확인하지 말고 구조체로
역직렬화해 제로값을 읽어야 한다.

> `omitempty`를 떼지 않는 이유: 성공과 실패가 **하나의 봉투 타입**이므로, 제로값을
> 내보내면 모든 실패 문서에도 `"totalTimeMinutes":0,"transferCount":0,"fareWon":0`이
> 따라붙는다. "실패 문서에는 성공 필드가 나타나지 않는다"는 불변식(data-model.md §3)이
> 깨지는 쪽이 더 큰 손해라 판단했다.
>
> 이 봉투는 MCP 도구의 성공 출력과 **같은 Go 타입**에서 직렬화된다 (data-model.md §3).
> 스키마가 갈라질 수 없다.

## 실패 문서

```json
{
  "error": {
    "code": "geocoder_rate_limited",
    "message": "장소 검색 요청이 일시적으로 제한되었습니다. 잠시 후 다시 시도하세요"
  }
}
```

`point_not_found` (지오코더 미설정):

```json
{
  "error": {
    "code": "point_not_found",
    "message": "출발지을(를) 찾을 수 없습니다: \"아이디스 타워\"",
    "hint": "건물명·주소로 찾으려면 naeryeo setup --geocoder로 장소 검색 키를 설정하세요",
    "side": "from",
    "name": "아이디스 타워"
  }
}
```

인자 검증 실패 (FR-015):

```json
{
  "error": {
    "code": "invalid_arguments",
    "message": "--from과 --to를 모두 입력해야 합니다"
  }
}
```

## 성공/실패 판별

호출자는 **`error` 키의 유무만** 확인한다. exit code로도 동일하게 판별 가능하다.
`message` 문자열 매칭은 금지 — 문구는 안정 계약이 아니다.

## 파싱 불변식

- stdout은 정확히 **하나의 JSON 문서**이며 그 외 바이트가 없다.
- `--json` 지정 시 FlagSet의 사용법 출력은 억제된다 (`fs.SetOutput(io.Discard)`) — R4.
- 로깅(`NAERYEO_LOG_LEVEL`, `--debug`)은 항상 stderr로만 나간다 (기존 `newLogger` 계약).

## 하위 호환

`--json` 미지정 경로를 검증하는 기존 테스트는 FR-005 해당 케이스를 제외하고 **수정 없이
통과해야 한다** (SC-007).
