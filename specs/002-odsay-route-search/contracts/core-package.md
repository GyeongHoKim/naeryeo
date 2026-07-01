# Contract: `internal/core` package surface

`cmd/naeryeo`의 `route` 서브커맨드와 향후 `mcp` 서버가 공유하는 안정 경계다. 이 계약이
바뀌면 두 소비자 모두를 함께 재검토해야 한다.

## API

```go
package core

type RouteResult struct {
    NoTravelNeeded bool
    TotalTime      int // 분
    TransferCount  int
    Fare           int // 원
    Steps          []RouteStep
}

type RouteStep struct {
    Description string
}

func NewClient(apiKey string) *Client
func (c *Client) FindRoute(ctx context.Context, from, to string) (RouteResult, error)

var (
    ErrAPIKeyMissing       error // apiKey가 빈 문자열
    ErrAuthFailed          error // ODsay가 인증 실패를 알림
    ErrNoRoute             error // 두 지점 사이에 경로 없음
    ErrUpstreamUnavailable error // 네트워크/서버 오류
)

type ErrPointNotFound struct {
    Side string // "from" | "to" | "both"
    Name string
}
func (e *ErrPointNotFound) Error() string

type ErrUpstreamRejected struct {
    Code    string
    Message string
}
func (e *ErrUpstreamRejected) Error() string
```

## 동작 계약

1. **`NewClient(apiKey string) *Client`** — 네트워크 호출을 하지 않는다. 항상 성공한다(단순
   구성).

2. **`FindRoute(ctx, from, to)`**
   - `c.APIKey == ""` → 네트워크 호출 없이 `RouteResult{}, ErrAPIKeyMissing`.
   - `from`/`to`가 실제 지점으로 인식되지 않음 → `RouteResult{}, &ErrPointNotFound{Side: ..., Name: ...}`.
   - `from`과 `to`가 사실상 동일/충분히 가까운 지점 → `RouteResult{NoTravelNeeded: true}, nil`
     (에러 아님).
   - 두 지점은 인식되지만 대중교통 경로가 없음 → `RouteResult{}, ErrNoRoute`.
   - ODsay 인증 실패(구현 시점에 실제 신호 확인, research.md §3) → `RouteResult{}, ErrAuthFailed`.
   - 네트워크 오류/타임아웃/ODsay 서버 오류 → `RouteResult{}, ErrUpstreamUnavailable`(원인
     `%w`로 보존).
   - 그 외 ODsay가 반환한 미분류 에러 → `RouteResult{}, &ErrUpstreamRejected{Code: ..., Message: ...}`.
   - 성공 → `RouteResult{...}, nil`, `Steps`는 이동 순서와 동일한 순서.
   - `ctx`가 취소/데드라인 초과되면 진행 중인 요청을 중단하고 그 사유를 반영한 에러를 반환한다
     (무한 대기 금지, FR-011).

## 호출자를 위한 가이드

- 호출자는 `errors.Is(err, core.ErrAPIKeyMissing)` 시 `naeryeo setup` 실행을 안내해야 한다
  (FR-007). 이 경우 `internal/config.Load()`가 이미 `ErrNotConfigured`를 반환했을 것이므로,
  호출자는 보통 `core.FindRoute`를 호출하기 전에 이를 알고 있지만, 방어적으로 `core`가 반환하는
  `ErrAPIKeyMissing`도 동일하게 처리해야 한다(빈 키를 실수로 넘긴 경우까지 방어).
- `errors.As(err, &pointErr)`로 `*core.ErrPointNotFound`를 잡아 `pointErr.Side`에 따라
  "출발지"/"도착지"/"출발지와 도착지 모두"를 인식하지 못했다고 안내해야 한다(FR-009).
- `RouteResult.NoTravelNeeded`가 `true`면 에러 처리 경로가 아니라 성공 경로에서 "이동이
  필요 없습니다" 같은 안내를 출력해야 한다(FR-012).
- `core.ErrUpstreamRejected`는 호출자가 세부 코드를 사용자에게 노출할 필요는 없지만, 로그나
  디버깅 목적으로 `Code`/`Message`를 남길 수 있다.
