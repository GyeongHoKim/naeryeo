# Phase 1 Data Model: API 키 OS 키체인 저장/조회/삭제

이 기능은 관계형/영속 데이터베이스를 두지 않는다. "데이터"는 OS 키체인이 관리하는 단일 비밀 엔트리이며, 아래는 그 엔트리의 논리 모델이다.

## Entity: StoredAPIKey (키체인 엔트리)

| Field | Type | Description | Validation |
|---|---|---|---|
| `Service` | string (상수) | 키체인 엔트리를 식별하는 서비스명. | 고정값 `"naeryeo"`. 사용자 입력으로 변경 불가. |
| `Username` | string (상수) | 서비스 내에서 엔트리를 식별하는 키. | 고정값 `"odsay-api-key"`. 다중 프로필 미지원(스펙 Assumptions). |
| `Value` | string | 사용자가 입력한 ODsay API 키 원문. | 빈 문자열 저장 불가(FR-010). 길이 상한은 OS 키체인 백엔드 한도(Windows 2560바이트)를 따르며, 초과 시 `ErrValueTooLarge`로 거부. |

State는 없다 — 엔트리는 "존재함" 또는 "존재하지 않음" 두 상태만 가지며, 존재할 때 항상 최신 `Set` 호출 값으로 덮어써진다(FR-008).

## Errors (도메인 상태를 나타내는 값)

| Sentinel | 의미 | 대응 스펙 요구사항 |
|---|---|---|
| `config.ErrNotConfigured` | 조회/삭제 시점에 저장된 키가 없음. | FR-007, FR-009 |
| `config.ErrKeychainUnavailable` | OS 키체인 백엔드 자체를 사용할 수 없음(지원 안 됨/D-Bus 세션 없음/접근 거부 등). | FR-006 |
| `config.ErrEmptyValue` | 빈 문자열을 키로 저장하려 시도함. | FR-010 |
| `config.ErrValueTooLarge` | 플랫폼 저장 한도를 초과하는 값. | (edge case) |

## Package Surface (internal/config)

```go
package config

// Save는 apiKey를 OS 키체인에 저장한다(기존 값이 있으면 덮어쓴다).
// apiKey가 비어 있으면 ErrEmptyValue를 반환한다.
func Save(apiKey string) error

// Load는 저장된 API 키를 반환한다.
// 저장된 값이 없으면 ErrNotConfigured를 반환한다.
func Load() (string, error)

// Delete는 저장된 API 키를 제거한다.
// 이미 없는 상태에서 호출해도 에러를 반환하지 않는다(idempotent, FR-009).
func Delete() error
```

`Save`/`Load`/`Delete`는 내부적으로 go-keyring 호출을 감싸는 작은 인터페이스(`keyringBackend`)를 통해 구현되며, 테스트에서는 이 인터페이스를 대체해 "백엔드 사용 불가" 경로까지 결정적으로 재현한다(research.md §5 참조).
