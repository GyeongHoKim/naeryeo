# Contract: config 자격증명 API (internal/config)

기존 단일 키 API를 자격증명 파라미터 기반으로 확장한다. 저장 백엔드/에러 정책은 001에서
확립된 것을 그대로 승계한다.

## 타입

```go
type Credential string

const (
    ODsayAPIKey    Credential = "odsay-api-key"    // 기존 keyUsername 값과 동일
    GeocoderAPIKey Credential = "geocoder-api-key"
)
```

## 함수 시그니처(변경)

```go
func Save(cred Credential, apiKey string) error
func Load(cred Credential) (string, error)
func Delete(cred Credential) error
```

## 동작 계약

| 함수 | 입력 | 성공 | 실패 |
|---|---|---|---|
| `Save` | cred, 비어있지 않은 키 | 키체인 (naeryeo, cred) 항목에 저장/덮어쓰기, `nil` | `ErrEmptyValue`(빈 키) / `ErrValueTooLarge` / `ErrKeychainUnavailable` |
| `Load` | cred | 저장된 키 문자열 | `ErrNotConfigured`(미저장) / `ErrKeychainUnavailable` |
| `Delete` | cred | 삭제(멱등, 미저장도 성공) | `ErrKeychainUnavailable` |

## 불변식

- 두 자격증명은 서로 독립: `Save(GeocoderAPIKey, k)`가 `ODsayAPIKey` 항목을 건드리지 않는다.
- `ODsayAPIKey`의 상수 값은 기존 `"odsay-api-key"`와 동일해야 하며, 기존 사용자의 저장분이
  마이그레이션 없이 로드되어야 한다.
- 키체인 불가용 시 평문 폴백 금지(기존 정책).

## 테스트 계약(요지)

- 각 자격증명에 대해 Save→Load 왕복, 상호 독립성(하나 저장이 다른 하나에 영향 없음),
  Delete 멱등성, 빈 키 거부, fake backend로 keychain-unavailable 경로.
