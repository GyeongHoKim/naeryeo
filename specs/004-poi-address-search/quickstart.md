# Quickstart / Validation: 건물명·주소(POI) 출발지/도착지 지원

이 기능이 종단 간 동작함을 확인하는 실행 가이드. 상세 계약은 `contracts/`와 `data-model.md`
참조.

## 사전 준비

- ODsay API 키(기존) — 발급: https://lab.odsay.com
- Kakao REST API 키(신규) — 발급: https://developers.kakao.com (애플리케이션 → REST API 키)
- 로컬 빌드: `just build` 또는 `go run ./cmd/naeryeo`

## 1. 키 등록

```bash
naeryeo setup                # ODsay API Key 입력
naeryeo setup --geocoder     # Kakao REST API Key 입력
```

기대: 각 명령이 "OS 키체인에 저장 완료" 출력. 두 키는 독립 항목으로 저장된다.

## 2. 건물명으로 경로 검색(핵심 시나리오, SC-001)

```bash
# just 사용 시 공백 포함 인자는 안쪽 작은따옴표로 감싼다(justfile 주석 참조)
go run ./cmd/naeryeo route --from "아이디스 타워" --to "수지구청"
```

기대: 정류장으로 해석되지 않는 "아이디스 타워"가 Kakao로 좌표 해석되어, 소요시간·환승·요금·
단계별 안내가 포함된 경로 결과가 출력된다(기존에는 "출발지를 찾을 수 없습니다"로 실패).

## 3. 회귀 확인(SC-002 / FR-003)

```bash
go run ./cmd/naeryeo route --from "강남역" --to "홍대입구역"
```

기대: 정류장으로 해석 가능한 입력은 기존과 동일하게 성공. 지오코더는 호출되지 않는다
(디버그 로그 `NAERYEO_LOG_LEVEL=debug`로 지오코더 호출 부재 확인 가능).

## 4. 지오코더 미설정 시 안내(FR-007)

```bash
naeryeo logout --geocoder                     # 지오코더 키 제거
go run ./cmd/naeryeo route --from "아이디스 타워" --to "수지구청"
```

기대: "출발지를 찾을 수 없습니다" + "건물명·주소로 찾으려면 `naeryeo setup --geocoder`로 장소
검색 키를 설정하세요" 힌트가 함께 출력된다.

## 5. 지오코더 인증 실패 안내(FR-009)

```bash
naeryeo setup --geocoder                      # 일부러 잘못된 키 입력
go run ./cmd/naeryeo route --from "아이디스 타워" --to "수지구청"
```

기대: "장소를 찾을 수 없음"과 구분되는 "장소 검색 키가 유효하지 않습니다" 취지의 안내.

## 6. 품질 게이트

```bash
just check    # fmt + lint + test 모두 green 이어야 완료
```

기대: `internal/geocode`, `internal/core`, `internal/config`, `cmd/naeryeo`의 신규/변경 테스트
포함 전체 통과, 커버리지 회귀 없음.
