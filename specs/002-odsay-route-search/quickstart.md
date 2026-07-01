# Quickstart: 대중교통 경로 검색 검증

## 사전 준비

- `mise install`로 pinned 툴체인 설치.
- `naeryeo setup`으로 실제 발급받은 ODsay API 키가 저장되어 있어야 한다(001 기능). 없으면
  아래 1번 시나리오만 키 없이도 검증 가능하다.

## 1. 단위 테스트로 대부분의 경로 검증 (API 키 불필요)

```bash
just test
```

기대 결과: `internal/core` 테스트가 `httptest.Server`로 흉내 낸 ODsay 응답을 통해 아래를
모두 커버하며 통과한다.

- 정상 경로: 소요시간·환승 횟수·요금·단계별 안내가 채워진 `RouteResult` 반환
- 환승 없는 경로: `TransferCount == 0`
- 출발지/도착지가 700m 이내(ODsay `-98`) → `NoTravelNeeded == true`, 에러 아님
- `apiKey`가 빈 문자열 → 네트워크 호출 없이 `ErrAPIKeyMissing`
- 출발지 미인식(코드 `3`) / 도착지 미인식(코드 `4`) / 둘 다 미인식(코드 `5`) → `ErrPointNotFound`의
  `Side` 필드가 각각 `"from"`/`"to"`/`"both"`
- 경로 없음(코드 `-99`, `6`) → `ErrNoRoute`
- 서버 오류(코드 `500`) / `httptest.Server`가 연결을 끊는 경우 → `ErrUpstreamUnavailable`
- `context.Context`에 짧은 데드라인을 준 상태에서 응답이 느린 서버 → 무한 대기 없이 에러 반환

## 2. 실제 ODsay API로 수동 검증 (API 키 필요)

```bash
go run ./cmd/naeryeo route --from "강남역" --to "홍대입구역"
```

기대 결과: README에 문서화된 형태(총 소요시간, 환승 횟수, 단계별 안내, 요금)로 결과가
출력된다.

```bash
go run ./cmd/naeryeo route --from "강남역" --to "강남역"
```

기대 결과: 에러가 아니라 "이동이 필요 없습니다" 류의 안내가 출력된다.

```bash
go run ./cmd/naeryeo route --from "존재하지않는가짜지명123" --to "홍대입구역"
```

기대 결과: 출발지를 인식하지 못했다는 에러 메시지가 출력되고, 프로세스는 0이 아닌 종료 코드로
끝난다.

## 3. API 키 미설정 상태 검증

```bash
go run ./cmd/naeryeo logout   # 혹시 키가 남아 있다면 제거
go run ./cmd/naeryeo route --from "강남역" --to "홍대입구역"
```

기대 결과: 외부 API 호출 없이 즉시 `naeryeo setup`을 먼저 실행하라는 안내가 출력된다.

## 4. 품질 게이트

```bash
just check   # fmt + lint + test
```
