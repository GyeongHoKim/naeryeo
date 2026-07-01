# Contract: geocode.Kakao (internal/geocode)

`core.Geocoder`를 구현하는 Kakao Local 키워드 검색 클라이언트.

## 생성자

```go
func NewKakao(apiKey string) *Kakao
```

- `apiKey`: Kakao REST API 키(`KakaoAK` 헤더에 사용).
- 테스트 주입용으로 `BaseURL`, `HTTPClient`(선택) 필드를 노출(002 `core.Client` 패턴과 동일).

## 외부 HTTP 계약(Kakao)

- 메서드/URL: `GET https://dapi.kakao.com/v2/local/search/keyword.json`
- 헤더: `Authorization: KakaoAK <REST_API_KEY>`
- 쿼리: `query=<검색어>`(필수). `size=1`, `sort=accuracy`로 대표 후보만 요청(불필요 데이터
  최소화).
- 응답(JSON) 관심 필드:
  ```json
  {
    "meta": { "total_count": 0 },
    "documents": [
      { "place_name": "...", "address_name": "...", "road_address_name": "...",
        "x": "127.xxxxxx", "y": "37.xxxxxx" }
    ]
  }
  ```

## Resolve 매핑

| 조건 | 반환 |
|---|---|
| `documents` 비어있지 않음 | `Coordinate{X: parse(documents[0].x), Y: parse(documents[0].y)}`, nil |
| `documents` 길이 0 | `core.Coordinate{}`, `core.ErrGeocoderNotFound` |
| HTTP 401 (키 무효) | `core.Coordinate{}`, `core.ErrGeocoderAuthFailed` |
| HTTP 403 (키는 유효하나 요청 거부 — 서비스 미활성/도메인·IP 제한) | `core.Coordinate{}`, `core.ErrGeocoderForbidden` |
| 2xx 아님(그 외)·네트워크·타임아웃·JSON 파싱 실패 | `core.Coordinate{}`, `core.ErrGeocoderUnavailable` |

> 이 sentinel들은 core 소유(에러 계약)다. geocode는 core를 단방향 import해 이들을 반환하며,
> core의 `resolveStation` 폴백이 `core.ErrGeocoderNotFound`만 `errStationNotFound`로 접고
> auth/unavailable은 그대로 전파한다(core-geocoder.md 참조).

## 좌표 파싱

- Kakao `x`/`y`는 문자열. `strconv.ParseFloat`로 변환하며 파싱 실패는
  `core.ErrGeocoderUnavailable`로 취급(예상치 못한 응답 형식 = 업스트림 이상).
- `x`=경도→`Coordinate.X`, `y`=위도→`Coordinate.Y`.

## 로깅/보안

- API 키를 로그에 남기지 않는다(002 `redactURL`과 동일 원칙; 키는 헤더이므로 URL 리댁션과
  별개로 절대 로깅 금지).
- context 전파 + HTTP 타임아웃으로 무한 대기 방지(FR-009).

## 테스트 계약(요지)

`httptest.Server`로 각 분기(1건/다건→첫 건/0건/401→AuthFailed/403→Forbidden/500/깨진 JSON)
테이블 테스트.
`Authorization` 헤더가 `KakaoAK <key>` 형식으로 전송되는지, `query`가 URL 인코딩되는지 검증.
