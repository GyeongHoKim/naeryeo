# Contract: CLI 표면 (cmd/naeryeo)

## setup — 대상 플래그 추가

```
naeryeo setup                # 대상: ODsay 키(기존 동작), 프롬프트 "ODsay API Key: "
naeryeo setup --geocoder     # 대상: 지오코더 키, 프롬프트 "Kakao REST API Key: "
```

- 플래그 없음 → `config.Save(config.ODsayAPIKey, key)`.
- `--geocoder` → `config.Save(config.GeocoderAPIKey, key)`.
- 빈 입력 거부·키체인 불가용 안내 등 나머지 동작은 기존과 동일.
- 성공 메시지: "OS 키체인에 저장 완료"(대상 무관 동일).

## logout — 대상 플래그 추가

```
naeryeo logout               # 대상: ODsay 키(기존 동작)
naeryeo logout --geocoder    # 대상: 지오코더 키
```

- 플래그에 따라 `config.Load`/`config.Delete` 대상 자격증명 선택.
- "삭제함"/"삭제할 키 없음" 구분 메시지(FR-009 스타일)는 기존 로직 재사용.

## route — 동작/메시지

```
naeryeo route --from <출발지> --to <도착지>
```

- 플래그 추가 없음. 진입점이 내부적으로:
  1. `config.Load(config.ODsayAPIKey)` (필수 — 없으면 기존 "setup 먼저" 안내).
  2. `config.Load(config.GeocoderAPIKey)` (선택 — 있으면 `geocode.NewKakao`를
     `core.Client.Geocoder`에 주입, `ErrNotConfigured`면 주입 생략).
- **FR-007 힌트**: findRoute가 `ErrPointNotFound`를 반환하고 **지오코더 키가 설정되지
  않았던** 경우, 기존 "출발지/도착지를 찾을 수 없습니다" 메시지에 이어
  "건물명·주소로 찾으려면 `naeryeo setup --geocoder`로 장소 검색 키를 설정하세요" 힌트를
  덧붙인다. 지오코더 키가 있었으면 힌트를 붙이지 않는다.
- 지오코더 인증 실패(`ErrGeocoderAuthFailed`) → "장소 검색 키가 유효하지 않습니다.
  `naeryeo setup --geocoder`로 다시 등록하세요"(ODsay 인증 실패 문구와 대칭).

## mcp — 배선 대칭

- `mcp` 진입점도 route와 동일하게 ODsay 키(필수)+지오코더 키(선택)를 조회해
  `core.Client.Geocoder`를 주입한다. MCP 도구의 에러 문구는 002/003의 공유
  `routeErrorMessage` 경로를 통해 CLI와 동일 사유를 제공한다.

## cmd 배선 (시그니처 확정)

리뷰 지적(설계 문서가 진입점 배선을 시그니처 수준에서 미명세)에 대응해, 아래를 확정한다.
목표: (a) 지오코더를 per-call 주입, (b) FR-007 힌트를 위해 "지오코더 키 설정 여부"를 진입점이
알게 하고, (c) CLI와 MCP가 **동일한** 힌트 문구를 내도록(FR-011) 공유 코드로 표현.

### 원칙: 지오코더 주입은 `findRoute` 클로저 내부에서

`findRoute` 클로저는 `main.go`에서 생성되며 이미 `internal/config`에 접근할 수 있다. 따라서
지오코더 주입을 클로저 안에서 처리하면 **`findRoute`의 기존 시그니처
`func(ctx, apiKey, from, to) (core.RouteResult, error)`는 바뀌지 않는다.** `route`/`mcp` 두
클로저 모두 다음을 수행한다:

```
gk, err := config.Load(config.GeocoderAPIKey)
client := core.NewClient(apiKey)
client.Logger = logger
if err == nil && gk != "" {           // ErrNotConfigured면 주입 생략
    client.Geocoder = geocode.NewKakao(gk)
}
return client.FindRoute(ctx, from, to)
```

### FR-007 힌트: `loadGeocoder` 파라미터 + `routeErrorMessage` 확장

진입점이 "지오코더 키 설정 여부"를 알아야 하므로:

1. `runRoute`와 `routeToolHandler`/`buildMCPServer`에 **`loadGeocoder func() (string, error)`**
   파라미터를 추가한다(기존 `load`와 대칭, 테스트에서 fake 주입 용이). `main.go`는
   `func() (string, error) { return config.Load(config.GeocoderAPIKey) }`를 넘긴다.
2. `findRoute`가 에러를 반환한 경우에 한해 진입점이 `loadGeocoder()`를 호출해
   `geocoderConfigured`(반환 키가 비어있지 않고 `ErrNotConfigured`가 아님)를 계산한다
   (해피패스에서는 호출하지 않아 불필요한 키체인 접근 없음).
3. 공유 함수 시그니처를 **`routeErrorMessage(err error, geocoderConfigured bool) string`** 으로
   확장한다. `err`가 `ErrPointNotFound`이고 `!geocoderConfigured`이면 기존 "출발지/도착지를
   찾을 수 없습니다" 뒤에 FR-007 힌트를 덧붙인다. CLI(`route.go`)와 MCP(`mcp.go`)가 이 함수를
   공유하므로 힌트 문구가 자동으로 동일해진다(FR-011).
   - CLI는 `"naeryeo route: " + routeErrorMessage(err, cfg)` 형태로 접두 유지.
   - MCP는 `errors.New(routeErrorMessage(err, cfg))` 그대로.
4. `ErrGeocoderAuthFailed`는 설정 여부와 무관하게 `routeErrorMessage`가 에러 타입만으로 처리
   (인증 실패 문구). `geocoderConfigured` 인자는 `ErrPointNotFound` 분기에서만 참조된다.

### 영향 받는 시그니처 요약

| 심볼 | 변경 |
|---|---|
| `config.Load/Save/Delete` | `Credential` 첫 인자 추가 |
| `runRoute` | `loadGeocoder func() (string, error)` 파라미터 추가 |
| `buildMCPServer` / `routeToolHandler` | `loadGeocoder func() (string, error)` 파라미터 추가 |
| `routeErrorMessage` | `(err error, geocoderConfigured bool) string`으로 확장 |
| `runSetup` / `runLogout` | `args []string`를 실제로 파싱(`--geocoder`) — 현재 `_ []string`로 무시 중 |
| `findRoute` 클로저 타입 | **불변**(지오코더 주입은 클로저 내부에서) |

> 주의: 현재 `runSetup`(`setup.go:16`)과 `runLogout`(`logout.go:15`)은 첫 인자를 `_ []string`로
> 무시한다. `--geocoder` 플래그를 위해 이 인자를 `flag.FlagSet`으로 파싱하도록 바꿔야 하며,
> `main.go`의 `runSetup(args[1:], ...)`/`runLogout(args[1:], ...)` 호출은 이미 잔여 인자를
> 넘기고 있으므로 호출부 변경은 없다.

## 테스트 계약(요지)

- `setup`/`logout`: `--geocoder` 유무에 따라 올바른 자격증명이 대상이 되는지 fake save/load/del로
  검증(플래그 파싱 포함).
- `route`: fake load/loadGeocoder(지오코더 키 있음/없음)와 fake findRoute(ErrPointNotFound/
  ErrGeocoderAuthFailed) 조합으로 힌트 문구 유무·인증 실패 문구를 검증.
- `routeErrorMessage`: `(ErrPointNotFound, false)` → 힌트 포함, `(ErrPointNotFound, true)` →
  힌트 없음, `(ErrGeocoderAuthFailed, _)` → 인증 실패 문구를 직접 단위 검증(CLI/MCP 공유 보장).
