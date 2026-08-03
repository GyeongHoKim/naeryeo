# Contract: 설정 파일

**Feature**: 006-self-hosted-routing-provider

비밀이 아닌 설정의 정본. 자격증명(ODsay 키, Kakao 키)은 **여기 담기지 않는다** — OS 키체인에
그대로 남는다.

## 경로

`os.UserConfigDir()` 하위 `naeryeo/config.json`.

| OS | 실제 경로 |
| --- | --- |
| macOS | `~/Library/Application Support/naeryeo/config.json` |
| Linux | `$XDG_CONFIG_HOME/naeryeo/config.json`, 미설정 시 `~/.config/naeryeo/config.json` |
| Windows | `%AppData%\naeryeo\config.json` |

- 디렉터리 권한 `0700`, 파일 권한 `0600` (Windows에서는 무의미하나 호출은 무해).
- `os.UserConfigDir()`가 실패하면(HOME 미설정 등) 설정을 읽을 수 없는 것으로 취급하고
  `provider_not_configured`로 떨어진다. 임의 경로로 폴백하지 않는다.

## 스키마

```json
{
  "routing_provider": "motis",
  "motis_url": "http://localhost:8080",
  "geocoder": "none"
}
```

| 키 | 타입 | 허용 값 | 필수 | 기본 |
| --- | --- | --- | :---: | --- |
| `routing_provider` | string | `"motis"` \| `"odsay"` | ✅ | — |
| `motis_url` | string | 절대 URL (`http`/`https`) | provider가 `motis`일 때 | — |
| `geocoder` | string | `"kakao"` \| `"none"` | ❌ | `"none"` |

### 검증 규칙

**저장 시** (`setup`) — 아래를 만족하지 않으면 파일을 쓰지 않고 실패한다:

1. `routing_provider`가 허용 값 중 하나
2. `routing_provider == "motis"`이면 `motis_url`이:
   - `url.Parse` 성공
   - scheme이 `http` 또는 `https`
   - `Host`가 비어 있지 않음
   - 후행 `/`는 제거 후 저장 (`http://x:8080/` → `http://x:8080`)
3. `routing_provider == "motis"`이면 도달성 프로브 통과 (아래 참조)
4. `geocoder == "kakao"`이면 키체인에 Kakao 키가 저장되어 있음

**로드 시** — 관대하게 읽되 모호하면 미설정으로 떨어진다:

| 상황 | 결과 |
| --- | --- |
| 파일 없음 | `ProviderUnset` → `provider_not_configured` |
| JSON 파싱 실패 | `ProviderUnset` → `provider_not_configured` (원본 파싱 에러는 노출하지 않음) |
| `routing_provider`가 미인식 값 | `ProviderUnset` |
| provider가 `motis`인데 `motis_url`이 없거나 무효 | `ProviderUnset` |
| `geocoder`가 미인식 값 또는 부재 | `GeocoderNone` |
| **알 수 없는 키가 존재** | **무시하고 정상 로드** |

마지막 행이 중요하다 — 구버전 naeryeo가 신버전이 쓴 파일을 만나 죽지 않게 한다. 반대 방향
(신버전이 구버전 파일을 읽음)은 필수 필드 부재로 `provider_not_configured`가 되어 setup을
안내한다.

## 도달성 프로브 (provider = motis)

`setup`이 URL을 저장하기 **전에** 1회 수행한다. 런타임 검색 경로에서는 수행하지 않는다.

```text
GET {motis_url}/api/v1/geocode?text=서울역
```

| 결과 | setup의 반응 |
| --- | --- |
| 2xx + 매치 ≥ 1건 | 저장 진행 |
| 2xx + 매치 0건 | **저장 거부** — "엔진은 응답하지만 시간표·지도 데이터가 적재되지 않은 것으로 보입니다" + 문서 링크 |
| 4xx/5xx | **저장 거부** — "엔진이 요청을 거부했습니다" + 문서 링크 |
| 연결 실패·타임아웃 | **저장 거부** — "엔진에 연결할 수 없습니다" + 문서 링크 |

프로브 타임아웃은 5초. 실패 메시지에 **입력한 URL을 그대로 되쓰지 않는다** — 사용자가 방금
입력한 값이므로 요약 확인 단계에서는 보여 주되, 에러 문구에는 싣지 않아 로그·전송 기록에
남는 경로를 줄인다.

이 프로브가 spec FR-016("응답은 하나 처리 불가한 상태를 경로 없음과 구별")을 만족시키는
결정론적 지점이다.

## 마이그레이션

**기존 사용자를 위한 자동 생성은 하지 않는다.** 키체인에 ODsay 키가 남아 있어도 설정 파일을
추정 생성하지 않는다(spec FR-031). 업그레이드 후 첫 실행은 `provider_not_configured`로
실패하고 `naeryeo setup`을 안내한다(FR-032).

저장된 자격증명은 이 과정에서 **삭제하지 않는다**(FR-033). 사용자가 setup에서 `odsay`를 다시
선택하면 기존 키가 그대로 재사용되고, 키 재입력을 요구하지 않는다.

## 테스트 계약

| 단언 | 근거 |
| --- | --- |
| `LoadSettings`가 파일 부재 시 `ProviderUnset`을 반환하고 에러를 반환하지 않음 | 부재는 정상 상태 |
| 손상된 JSON에서도 원본 파싱 에러 문자열이 반환값·출력에 나타나지 않음 | FR-019 |
| 알 수 없는 키가 있는 파일이 정상 로드됨 | 전방 호환 |
| `SaveSettings` 후 `LoadSettings`가 같은 값을 돌려줌 (round-trip) | — |
| 저장된 파일 권한이 `0600` (POSIX에서만 단언) | — |
| `motis_url` 후행 슬래시가 정규화됨 | — |
| 무효한 `motis_url`로 `SaveSettings` 호출 시 파일이 **생성되지 않음** | 부분 저장 금지 |
| 3개 OS CI에서 경로가 `os.UserConfigDir()` 하위로 해석됨 | GYE-296 매트릭스 |
