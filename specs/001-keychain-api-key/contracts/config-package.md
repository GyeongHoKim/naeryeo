# Contract: `internal/config` package surface

이 기능이 나머지 코드베이스(특히 `cmd/naeryeo`의 `setup`/`logout`, 그리고 향후 `internal/core`)에 제공하는 계약이다. 이 문서에 정의된 함수 시그니처와 에러 의미는 다른 기능이 의존하는 안정적 경계이므로, 변경 시 의존하는 기능들의 스펙도 함께 재검토해야 한다.

## API

```go
package config

func Save(apiKey string) error
func Load() (string, error)
func Delete() error

var (
    ErrNotConfigured       error // Load/Delete 시점에 저장된 키가 없음
    ErrKeychainUnavailable error // OS 키체인 백엔드를 사용할 수 없음
    ErrEmptyValue          error // 빈 문자열을 저장하려 시도함
    ErrValueTooLarge       error // 플랫폼 저장 한도 초과
)
```

## 동작 계약

1. **`Save(apiKey string) error`**
   - `apiKey == ""` → `ErrEmptyValue`를 반환하고 아무것도 저장하지 않는다.
   - 키체인 백엔드를 사용할 수 없음 → `ErrKeychainUnavailable`을 반환하고, 평문 파일 등 다른 방식으로 폴백하지 않는다.
   - 그 외 성공 시 → `nil`을 반환하며, 기존에 저장된 값이 있었다면 덮어쓴다.
   - 후속 호출 보장: `Save(x)` 성공 직후 `Load()`는 항상 `x, nil`을 반환한다(왕복 무결성, SC-005).

2. **`Load() (string, error)`**
   - 저장된 값이 없음 → `"", ErrNotConfigured`.
   - 키체인 백엔드를 사용할 수 없음 → `"", ErrKeychainUnavailable`.
   - 성공 시 → 마지막으로 `Save`된 값과 정확히 동일한 문자열을 반환한다.

3. **`Delete() error`**
   - 저장된 값이 없음 → `nil`(에러 아님, idempotent — FR-009).
   - 키체인 백엔드를 사용할 수 없음 → `ErrKeychainUnavailable`.
   - 성공 시 → `nil`. 이후 `Load()`는 `ErrNotConfigured`를 반환해야 한다.

## 호출자를 위한 가이드

- 호출자는 `errors.Is(err, config.ErrNotConfigured)` 등으로 분기해야 하며, go-keyring의 에러 타입에 직접 의존해서는 안 된다(이 패키지가 그 의존성을 캡슐화한다).
- `cmd/naeryeo setup`은 `Save`를, `cmd/naeryeo logout`은 `Delete`를 호출한다. 향후 `route`/`mcp`는 `Load`를 호출해 ODsay 클라이언트에 전달할 키를 얻는다.
- `ErrKeychainUnavailable`을 받은 CLI 서브커맨드는 0이 아닌 종료 코드로 종료하고, 원인을 사용자에게 설명해야 한다(FR-006, User Story 4).
