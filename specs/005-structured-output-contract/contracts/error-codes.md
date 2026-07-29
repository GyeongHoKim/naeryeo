# Contract: 에러 코드 taxonomy

**Feature**: 005-structured-output-contract

이 문서가 **에러 코드의 정본**이다. `skills/naeryeo/SKILL.md`와 구현은 여기서 파생된다.

## 안정성 보증

- 코드 문자열의 **변경·삭제는 breaking change**다.
- 코드 **추가는 non-breaking**이다. 호출자는 모르는 코드를 만나면 `message`를 그대로
  사용자에게 전달하고 재시도하지 않아야 한다.
- 코드는 `snake_case`이며 안정적이다. `message`는 **안정 계약이 아니다** — 문구는 개선될 수
  있으므로 호출자는 절대 문자열 매칭을 하지 않는다.

## 코드 표

| 코드 | 트리거 | AI의 후속 행동 | 재시도 |
| --- | --- | --- | :---: |
| `api_key_missing` | `core.ErrAPIKeyMissing`, `config.ErrNotConfigured` | 사용자에게 `naeryeo setup` 실행 안내 | ❌ |
| `auth_failed` | `core.ErrAuthFailed` | 사용자에게 `naeryeo setup` 재등록 안내 | ❌ |
| `geocoder_auth_failed` | `core.ErrGeocoderAuthFailed` | 사용자에게 `naeryeo setup --geocoder` 재등록 안내 | ❌ |
| `geocoder_forbidden` | `core.ErrGeocoderForbidden` | Kakao 콘솔에서 카카오맵(로컬) 활성화·도메인/IP 제한 확인 안내. **재등록은 무의미** | ❌ |
| `geocoder_rate_limited` | `*core.ErrGeocoderRejected` && `RateLimited()` | **잠시 후 동일 요청 재시도** | ✅ |
| `geocoder_rejected` | `*core.ErrGeocoderRejected` (그 외) | **입력 재작성** — 더 구체적인 주소나 인근 역·정류장명. 동일 입력 재시도 무의미 | ❌ |
| `point_not_found` | `*core.ErrPointNotFound` | `side`가 가리키는 지점만 다시 질의. `hint`가 있으면 사용자에게 전달 | ❌ |
| `no_route` | `core.ErrNoRoute` | 경로 없음을 사용자에게 보고 | ❌ |
| `geocoder_unavailable` | `core.ErrGeocoderUnavailable` | 잠시 후 재시도 | ✅ |
| `upstream_unavailable` | `core.ErrUpstreamUnavailable` | 잠시 후 재시도 | ✅ |
| `upstream_rejected` | `*core.ErrUpstreamRejected` | 사용자에게 보고. **경로 제공자 원문은 노출되지 않는다** | ❌ |
| `credential_store_error` | 키체인 조회 실패 (`config.ErrNotConfigured` 제외) | 사용자에게 보고 | ❌ |
| `invalid_arguments` | 플래그 파싱 실패, `--from`/`--to` 누락 | 호출 형태를 고쳐 재시도 | ❌ |
| `internal_error` | 위 어디에도 해당 없음 | 사용자에게 보고. **도달 불가여야 함** | ❌ |

### 갈라짐이 중요한 두 쌍

호출자가 반드시 구분해야 하는, 현재는 한국어 문장으로만 구별되는 분기다.

1. `geocoder_rate_limited` (재시도 ✅) ↔ `geocoder_rejected` (입력 재작성)
   — 같은 Go 타입(`*ErrGeocoderRejected`)에서 `RateLimited()`로 갈린다.
2. `geocoder_auth_failed` (재등록으로 해결) ↔ `geocoder_forbidden` (콘솔 설정 문제, 재등록 무의미)

## `message` / `hint` 규칙

- `message`: 사람에게 그대로 전달할 한국어 사유. **필수**.
- `hint`: 사용자가 취할 조치. 있을 때만 존재.
- 프로즈 모드에서는 `message + "\n" + hint`로 이어 붙여 **기존 출력과 바이트 동일**하게
  유지한다 (FR-007, FR-013).
- 어느 필드에도 외부 제공자·저장소의 **원본 문자열이 들어가지 않는다** (FR-005).

현재 `hint`를 갖는 유일한 코드는 `point_not_found`(지오코더 미설정 시)다.

## `point_not_found` 부가 필드

| 필드 | 값 |
| --- | --- |
| `side` | `from` \| `to` \| `both` |
| `name` | 인식에 실패한 입력 문자열 |

## 망라성 게이트

`internal/core`에 exported 에러가 추가되면 테스트가 실패한다 (data-model.md §4).
새 에러를 추가하는 작업은 이 표에 행을 추가해야 완료된다.

**허용 목록**: `core.ErrGeocoderNotFound` — `Geocoder` 인터페이스 계약 sentinel이며
`resolveStation`이 `*ErrPointNotFound`로 접어 표현 계층에 도달하지 않는다.

## 향후 확장

- `provider_not_configured` — GYE-294가 설정 파일 부재 상태를 도입할 때 추가
- `motis_*`, `docs` 필드 — GYE-295가 대체 경로 제공자를 도입할 때 추가

둘 다 이번 범위 밖이다 (spec Assumptions). 추가 시 위 게이트가 코드 부여를 강제한다.
