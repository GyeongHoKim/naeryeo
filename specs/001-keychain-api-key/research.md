# Phase 0 Research: API 키 OS 키체인 저장/조회/삭제

## 1. OS 키체인 연동 라이브러리

**Decision**: `github.com/zalando/go-keyring`를 사용한다.

**Rationale**: README와 constitution에서 이미 지정된 라이브러리다(OS별 세부 구현을 추상화하고, macOS Keychain / Windows Credential Manager / Linux Secret Service(D-Bus)를 단일 API로 노출). 공식 문서(context7 `/zalando/go-keyring`)로 API를 확인했다:

- `keyring.Set(service, username, password string) error` — 저장. payload가 플랫폼 한도를 넘으면 `ErrSetDataTooBig` 반환(Windows는 2560바이트 제한).
- `keyring.Get(service, username string) (string, error)` — 조회. 없으면 `("", ErrNotFound)`.
- `keyring.Delete(service, username string) error` — 삭제. 없으면 `ErrNotFound`.
- `keyring.ErrUnsupportedPlatform` — 현재 OS에 지원되는 백엔드 자체가 없는 경우의 sentinel 에러.
- `keyring.MockInit()` — 실제 OS 백엔드 대신 인메모리 맵을 설치. 유닛 테스트/CI에서 실제 키체인 없이 왕복(round-trip)·`ErrNotFound` 동작을 검증하는 데 사용.

**중요한 세부사항 (Linux headless 처리에 직결)**: Linux/*BSD는 Secret Service D-Bus 인터페이스(주로 GNOME Keyring이 제공)를 사용하며 기본 `login` 컬렉션이 있어야 한다. **`ErrUnsupportedPlatform`은 "OS 자체에 지원 백엔드가 없는 경우"에만 반환되고, Linux에서 D-Bus 세션이나 Secret Service 데몬이 아예 없는 경우(headless 서버 등)에는 sentinel이 아닌 일반 `error`(D-Bus 연결 실패 등)가 반환된다.** 따라서 FR-006(키체인 백엔드 사용 불가 시 안전한 실패)을 만족시키려면 `ErrUnsupportedPlatform` 뿐 아니라, `ErrNotFound`/`ErrSetDataTooBig`가 아닌 그 외 모든 에러를 "백엔드 사용 불가"로 간주해 평문 폴백 없이 명확한 에러 메시지로 감싸야 한다.

**Alternatives considered**:
- OS별 네이티브 바인딩을 직접 작성 — go-keyring이 이미 세 OS를 커버하며 constitution/README에 명시된 선택이므로 재발명할 이유 없음(기각).
- 환경변수 기반 저장 — 평문 유사 수준의 노출 위험이 있어 "평문 파일 금지" 요구와 정신이 배치됨(기각).

## 2. 서비스/사용자 식별자 스킴

**Decision**: `service = "naeryeo"`, `username = "odsay-api-key"` 고정 상수를 keyring 엔트리 키로 사용한다.

**Rationale**: 스펙의 Assumptions에서 사용자당 단일 키만 지원하기로 범위를 한정했다. 두 값 모두 코드 상수로 고정하면 다중 프로필 지원 같은 미사용 확장 지점을 만들지 않고 최소 구현이 가능하다.

**Alternatives considered**: OS 로그인 사용자명을 `username`으로 사용 — 굳이 필요 없는 가변성만 추가하므로 기각. 다중 키/프로필 지원 — 스펙 범위 밖으로 명시적으로 제외됨.

## 3. 에러 모델 설계

**Decision**: `internal/config`는 go-keyring의 에러를 그대로 노출하지 않고, 이 패키지 자체의 sentinel 에러로 감싼다:
- `ErrNotConfigured` ← `keyring.ErrNotFound`를 감쌈 (FR-007: "키 없음" 상태를 다른 실패와 구분)
- `ErrKeychainUnavailable` ← `keyring.ErrUnsupportedPlatform` 및 그 외 백엔드 접근 실패를 감쌈 (FR-006)
- 그 외 에러(`ErrSetDataTooBig` 등)는 `%w`로 래핑해 그대로 전달

**Rationale**: 소비자 패키지(`cmd/naeryeo`, 향후 `internal/core`)가 `go-keyring`에 직접 의존하지 않고 `errors.Is(err, config.ErrNotConfigured)` 같은 자체 도메인 에러만으로 분기할 수 있다. Principle I(작은 인터페이스는 소비자가 정의)과 정합적 — 외부 라이브러리 타입이 패키지 경계를 넘어 새지 않는다.

**Alternatives considered**: `keyring.ErrNotFound` 등을 그대로 재노출 — 소비 패키지가 서드파티 라이브러리에 암묵적으로 결합되어 기각.

## 4. 콘솔 입력 방식 (`naeryeo setup`)

**Decision**: 표준 입력이 터미널(TTY)인 경우 마스킹 입력(echo 없음)을 사용하고, TTY가 아닌 경우(파이프/테스트) 일반 라인 읽기로 폴백한다.

**Rationale**: README 예시(`ODsay API Key: ****************`)가 마스킹 입력을 전제로 한다. 다만 `go-keyring`처럼 context7에 색인된 문서를 찾지 못했다 — `golang.org/x/term`은 이번 조회에서 매칭되는 라이브러리가 없었으므로(quota 문제 아님, 색인 없음), 이 부분은 안정적으로 오래 유지되어 온 표준 확장 패키지 지식(`term.ReadPassword(fd int) ([]byte, error)`, `term.IsTerminal(fd int) bool`)에 근거했음을 명시한다. 실제 구현 단계(`/speckit-tasks` 이후)에서 `go doc golang.org/x/term`으로 시그니처를 재확인할 것을 권장한다.

**Alternatives considered**: 항상 평문 echo로 입력받기 — 어깨너머 노출(shoulder surfing) 위험이 있어 보안 목적과 배치(기각). 이 결정은 필수 요구사항(FR)이 아니라 UX 세부사항이므로, 마스킹이 실패하는 예외적 환경에서도 기능 자체(저장/조회/삭제)는 동작해야 한다.

## 5. 테스트 전략

**Decision**: `keyring.MockInit()`으로 실제 OS 키체인 대신 인메모리 백엔드를 사용해 `internal/config`의 저장/조회/삭제/왕복/미존재 케이스를 전부 유닛 테스트로 커버한다. "백엔드 사용 불가" 경로(FR-006)는 go-keyring이 반환할 수 있는 에러를 모킹 가능한 형태로 주입할 수 있도록, go-keyring 호출을 작은 인터페이스로 감싸 테스트 더블을 만든다.

**Rationale**: constitution Principle II(단위 테스트 필수) 및 III(CI에서 `just test` 통과)를 만족하려면 실제 키체인 데몬이 없는 CI(headless Linux)에서도 전체 스위트가 결정적으로 통과해야 한다. `MockInit()`은 정상 경로에는 충분하지만, "백엔드 자체가 없음" 실패 경로까지 재현하지는 못하므로 별도 테스트 더블이 필요하다.

**Alternatives considered**: 실제 OS 키체인에 대해서만 통합 테스트 — CI 환경(headless)에서 결정적으로 실패/스킵되어 Principle III 위반 소지가 있어 기각. 대신 이런 테스트는 있다면 build tag로 분리해 선택적으로만 실행한다.
