# Quickstart: API 키 OS 키체인 저장/조회/삭제 검증

## 사전 준비

- 로컬 개발 환경에 `mise install`로 pinned 툴체인(go, just 등)이 설치되어 있어야 한다.
- macOS/Windows/Secret Service가 있는 Linux 데스크톱 중 하나에서 실행한다(순수 headless Linux는 "백엔드 사용 불가" 경로 검증용으로 별도 사용).

## 1. 단위 테스트로 정상/예외 경로 검증

```bash
just test
```

기대 결과: `internal/config` 패키지 테스트가 `keyring.MockInit()` 기반 인메모리 백엔드로 아래를 모두 커버하며 통과한다.

- 빈 문자열 `Save` 시 `ErrEmptyValue`
- 정상 `Save` → `Load` 왕복 시 동일 값 반환
- `Save` 재호출(덮어쓰기) 후 `Load`가 최신 값만 반환
- 키가 없는 상태에서 `Load` 시 `ErrNotConfigured`
- `Delete` → 이후 `Load` 시 `ErrNotConfigured`
- 키가 없는 상태에서 `Delete` 호출 시 에러 없음(idempotent)
- 백엔드 사용 불가를 흉내 낸 테스트 더블에서 `Save`/`Load`/`Delete` 모두 `ErrKeychainUnavailable`

## 2. 실제 OS 키체인에서 수동 검증 (지원 환경)

```bash
go run ./cmd/naeryeo setup
# 프롬프트에 임의의 테스트 키 문자열 입력 (예: test-odsay-key-123)

go run ./cmd/naeryeo route --from "강남역" --to "홍대입구역"
# (route 기능이 아직 구현되지 않았다면, 대신 Load()를 호출하는 임시 디버그 경로나 단위 테스트로 대체 검증)

go run ./cmd/naeryeo logout
```

기대 결과:
- `setup` 이후 OS 키체인 UI(예: macOS 키체인 접근 프로그램, GNOME Seahorse, Windows 자격 증명 관리자)에서 `naeryeo` 서비스 항목이 확인된다.
- 파일시스템 전체에서 입력한 테스트 키 문자열을 검색해도 어떤 평문 파일에서도 발견되지 않는다(SC-002 수동 재현: `grep -r "test-odsay-key-123" ~ 2>/dev/null`이 아무것도 반환하지 않아야 한다).
- `logout` 이후 같은 OS 키체인 UI에서 해당 항목이 사라져 있다.

## 3. 키체인 백엔드 사용 불가 환경에서의 안전한 실패 검증

Secret Service가 없는 headless Linux 컨테이너(예: 최소 `debian:stable` 이미지, D-Bus 세션 없음)에서:

```bash
go run ./cmd/naeryeo setup
# 임의의 문자열 입력
```

기대 결과:
- 명령이 성공하지 않고, 키체인 백엔드를 사용할 수 없다는 취지의 에러 메시지와 함께 0이 아닌 종료 코드로 끝난다.
- 컨테이너 파일시스템 어디에도 입력한 키가 평문으로 기록되지 않는다.

## 4. 품질 게이트

```bash
just check   # fmt + lint + test
```

세 단계 모두 통과해야 이 기능의 구현이 완료된 것으로 간주한다(constitution Principle III).
