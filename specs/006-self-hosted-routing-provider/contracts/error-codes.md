# Contract: 에러 코드 taxonomy — 006 delta

**Feature**: 006-self-hosted-routing-provider

정본은 `specs/005-structured-output-contract/contracts/error-codes.md`다. 이 문서는 그 위에
**추가되는 것만** 기술한다. 005의 안정성 보증(코드 추가는 non-breaking, `message`는 계약이
아님, 호출자는 문자열 매칭 금지)이 그대로 적용된다.

005 문서의 "향후 확장" 절이 예고한 `provider_not_configured`, `motis_*`, `docs` 필드가 여기서
실현된다. **구현 시 005 문서의 그 절을 갱신해 이 문서를 가리키게 한다.**

## 추가되는 코드 3개

| 코드 | 트리거 | AI의 후속 행동 | 재시도 | `docs` |
| --- | --- | --- | :---: | :---: |
| `provider_not_configured` | 설정 파일 부재 / `routing_provider` 미인식 / provider가 `motis`인데 `motis_url` 무효 | 사용자에게 `naeryeo setup` 실행 안내. **저장된 키가 있어도 추정하지 말 것** | ❌ | ✅ |
| `motis_unavailable` | `core.ErrMotisUnavailable` — 연결 거부·타임아웃·DNS 실패·5xx | **잠시 후 재시도.** 반복 실패하면 사용자에게 엔진 기동 상태 확인 안내 | ✅ | ✅ |
| `motis_rejected` | `*core.ErrMotisRejected` — 4xx, 해석 불가한 본문 | 사용자에게 보고. 재시도 무의미 — 엔진 설정·데이터 문제 | ❌ | ✅ |

### 문구

```text
provider_not_configured
  message: 경로 검색 제공자가 설정되지 않았습니다
  hint:    naeryeo setup을 실행해 자체 호스팅(MOTIS) 또는 ODsay 중 하나를 선택하세요
  docs:    <self-hosting 문서 URL>

motis_unavailable
  message: 자체 호스팅한 경로 검색 엔진에 연결할 수 없습니다. 잠시 후 다시 시도하세요
  hint:    엔진이 실행 중인지 확인하세요
  docs:    <self-hosting 문서 URL>

motis_rejected
  message: 자체 호스팅한 경로 검색 엔진이 요청을 처리하지 못했습니다
  hint:    엔진의 시간표·지도 데이터가 정상적으로 적재되었는지 확인하세요
  docs:    <self-hosting 문서 URL>
```

**어느 문구에도 사용자의 호스트명·포트·경로가 들어가지 않는다**(spec FR-018). MOTIS는 API 키가
없어 자격증명 유출 위험은 없지만, 사설망 구조가 AI 대화 기록에 남는 것을 막는다. HTTP 상태
코드도 문구에 넣지 않는다 — `--debug`(stderr)에만 남는다.

## `docs` 필드 신설

```go
type RouteError struct {
    // ... 기존 필드 ...
    Docs string `json:"docs,omitempty"`
}
```

- 값은 **self-hosting 문서의 절대 URL 하나**. 다른 코드들은 비워 둔다.
- `omitempty`이므로 기존 코드의 JSON 문서는 **바이트 단위로 불변**이다.
- 프로즈 출력에서는 `Prose()`가 `message` / `hint` 다음 **세 번째 줄**로 덧붙인다. 기존 코드는
  `Docs`가 빈 문자열이라 출력이 바뀌지 않는다(spec 005 FR-007 유지).

**URL 상수는 한 곳에서만 정의한다.** 문서 경로가 바뀌면 세 코드가 함께 따라가야 하고, 문자열이
흩어지면 링크가 조용히 갈라진다.

## 기존 코드에 대한 영향

| 코드 | 변화 |
| --- | --- |
| `api_key_missing` | **의미가 좁아진다.** 이제 "ODsay를 선택했는데 키가 없음"만 뜻한다. "아무것도 설정 안 됨"은 `provider_not_configured`가 가져간다 |
| `no_route` | MOTIS에서도 발생. `itineraries`와 `direct`가 모두 비었을 때 (data-model.md §5-2). 시간표 만료와 구별이 필요할 수 있음 — research.md U4 |
| `point_not_found` | MOTIS 경로에서는 **MOTIS geocode + Kakao 폴백이 모두 실패**했을 때. 기존 hint("`setup --geocoder`") 규칙은 그대로 — Kakao 미설정일 때만 붙는다 |
| `upstream_*` | **ODsay 전용으로 남는다.** MOTIS 실패는 `motis_*`로 간다. 두 계열을 합치지 않는 이유는 조치가 다르기 때문 — ODsay는 사용자가 손댈 수 없고, MOTIS는 사용자 자신의 서버다 |
| 그 외 | 변화 없음 |

## 망라성 게이트

`internal/core`에 `ErrMotisUnavailable`, `ErrMotisRejected`를 추가하는 순간
`TestErrorCodeExhaustive_EveryCoreErrorHasACode`가 실패한다. 해소 순서:

1. `internal/core/errors.go`에 두 심볼 추가 → **게이트 실패 확인** (FR-020이 작동함을 증명)
2. 이 문서의 표에 행 추가
3. `classifyRouteError`에 분기 추가
4. `errcode_exhaustive_test.go`의 `coreErrorSamples`에 샘플 추가
5. 게이트 통과 확인

**1번에서 실패하는 것을 실제로 확인하고 넘어간다.** 게이트가 살아 있음을 증명하지 않고 통과만
시키면 spec SC-004("코드 부여 없이 추가하면 반드시 실패")를 검증한 것이 아니다.

`provider_not_configured`는 `internal/core` 에러가 아니라 `cmd/naeryeo`가 설정을 읽고 직접
만드는 failure이므로 이 게이트의 대상이 아니다. 대신 "설정 미비 상태에서 `route`와 `mcp`가
모두 이 코드를 낸다"는 별도 테스트로 강제한다.

## 테스트 계약

| 단언 | 근거 |
| --- | --- |
| 설정 파일 없음 → `route --json`과 MCP 툴이 **같은** `provider_not_configured`를 반환 | FR-002, spec 005 FR-016 |
| MOTIS 연결 실패 → `motis_unavailable`, `docs` 비어 있지 않음 | FR-015, FR-017 |
| MOTIS 4xx → `motis_rejected` | FR-016 |
| 위 3개 코드의 `message`·`hint`·`docs` 어디에도 테스트가 쓴 MOTIS 호스트·포트 문자열이 없음 | FR-018, SC-006 |
| 프로즈 모드에서 위 3개 코드가 3줄(message/hint/docs)로 렌더링됨 | FR-017 |
| 기존 코드들의 프로즈·JSON 출력이 변경 전과 바이트 동일 | spec 005 FR-007 |
| `ErrMotis*` 추가 후 샘플 없이 게이트를 돌리면 실패 | SC-004 |
