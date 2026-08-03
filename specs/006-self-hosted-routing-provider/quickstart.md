# Quickstart: 검증 시나리오

**Feature**: 006-self-hosted-routing-provider | **Date**: 2026-07-31

구현이 끝났음을 증명하는 실행 가능한 검증 절차다. 각 시나리오는 spec의 User Story와
Success Criteria에 대응하며, 대부분은 자동 테스트로 옮겨진다 — 옮길 수 없는 것(실제 MOTIS
기동, 실측치)만 수동으로 남는다.

## 사전 조건

```bash
mise install          # 툴체인 (go, just, golangci-lint)
just check            # 기준선: 변경 전에도 green이어야 한다
```

실 MOTIS가 필요한 시나리오(S2, S6, S7)에는 Docker가 추가로 필요하다.

---

## S1. 자체 호스팅으로 경로 검색 — 상용 키 없이 (US1 / SC-001)

**자동 테스트로 검증.** `httptest.Server`가 MOTIS 역할을 한다.

```bash
just test
```

| 확인 | 대응 |
| --- | --- |
| ODsay 키가 키체인에 없는 상태에서 MOTIS 설정만으로 `route`가 성공 | US1 AS1, AS2 |
| 같은 설정에서 `route`와 MCP 툴이 동일 제공자를 사용 | US1 AS3, SC-005 |
| MOTIS 결과의 `--json` 성공 스키마가 ODsay와 동일 | US1 AS4, FR-011 |
| 요금 없는 결과에 `fareWon`이 부재하고 프로즈에 `요금 정보 없음` | US1 AS5, FR-010 |

---

## S2. 실 MOTIS 기동 후 end-to-end (US1 / SC-007)

**수동.** 문서(`docs/self-hosting.md`)가 실제로 동작하는지 확인하는 유일한 방법이다.

```bash
# 1. 데이터 준비 (문서의 절차를 그대로 따를 것)
cd deploy/motis
# GTFS + OSM pbf를 data/ 에 배치

# 2. 기동
docker compose up motis-import      # 그래프 빌드 — 소요 시간·RAM을 계측할 것
docker compose up -d motis-server

# 3. 연결
naeryeo setup --provider=motis --motis-url=http://localhost:8080

# 4. 대표 질의 3종
naeryeo route --from "강남역" --to "홍대입구역"     # 수도권 도시철도
naeryeo route --from "서면역" --to "해운대역"       # 지방 광역시 시내
naeryeo route --from "대전역" --to "광주송정역"     # 도시 간
```

**기록할 것** (research.md U2·U3 해소, spec FR-023):

- 그래프 빌드 소요 시간, 최대 RSS, 결과 디스크 사용량
- GTFS 피드의 실제 지역 커버리지 — 위 3종 중 실패하는 것이 있으면 문서에 한계로 명시(FR-024)

**이 측정 없이는 `docs/self-hosting.md`를 완료로 볼 수 없다.** FR-023이 요구하는 것은 추정치가
아니라 실측 기준값이다.

---

## S3. 문서만으로 구축 (US2 / SC-003, SC-004)

**수동.** 자체 호스팅 경험이 없는 사람에게 `docs/self-hosting.md`만 주고 구축시킨다.

| 확인 | 통과 기준 |
| --- | --- |
| 문서 바깥 지식이 필요했던 지점 | 0건 (SC-003) |
| 착수 전에 자원 요구치를 알 수 있었는가 | 예 (SC-004) |
| 도중에 자원 부족으로 중단됐는가 | 아니오 |
| 데이터의 갱신 주기·커버리지 한계가 명시돼 있는가 | 예 (FR-024) |
| 이미지 태그와 데이터 출처가 고정돼 있는가 | 예 (FR-022) |

---

## S4. 실패 진단 (US3 / SC-006, SC-009)

**자동 테스트로 검증.**

```bash
# 엔진 정지 상태 — httptest 서버를 닫고 호출
naeryeo route --from 강남역 --to 홍대입구역 --json
# → {"error":{"code":"motis_unavailable","message":"...","hint":"...","docs":"..."}}
```

| 확인 | 대응 |
| --- | --- |
| 연결 불가가 `motis_unavailable`(재시도 ✅)로 분류 | US3 AS1, AS2 |
| 세 신규 코드 모두 `docs` 링크를 가짐 (JSON·프로즈 양쪽) | US3 AS3, FR-017 |
| 출력 전체에 사설망 호스트·포트 문자열이 **없음** | US3 AS4, SC-006 |
| 제공자 미설정이 `provider_not_configured`로 분류 | US3 AS5, FR-014 |

호스트 비노출은 문자열 부분 일치로 단언한다 — 테스트가 쓴 `127.0.0.1:PORT`가 stdout·stderr
어디에도 나타나지 않아야 한다.

---

## S5. 기존 사용자 전환 (US4 / SC-010)

**자동 테스트로 검증.** fake 키체인에 ODsay 키를 넣고 설정 파일은 없는 상태를 만든다.

| 확인 | 대응 |
| --- | --- |
| 첫 검색이 `provider_not_configured`로 실패하고 setup을 안내 | US4 AS1, FR-031/032 |
| `setup --provider=odsay` 후 **키 재입력 없이** 동작 | US4 AS2, FR-033 |
| `setup --delete=odsay` 후에도 설정 파일이 남아 있음 | US4 AS3 |
| 제공자 상호 전환이 같은 절차 안에서 가능 | US4 AS4 |
| `naeryeo logout`이 unknown command | logout 제거 |

---

## S6. 지오코더 조합 (FR-030)

MOTIS × Kakao의 4개 조합이 모두 유효해야 한다.

| provider | geocoder | 기대 |
| --- | --- | --- |
| motis | none | 역·정류장 이름으로 검색 가능. 외부 호출 0건 (SC-002) |
| motis | kakao | 건물명·주소도 검색 가능 |
| odsay | none | 기존 동작 그대로 |
| odsay | kakao | 기존 동작 그대로 |

첫 행이 이 기능의 핵심 주장이다 — **MOTIS 내장 지오코더가 이름 해석을 담당하므로 Kakao 키
없이도 동작한다**(research.md R4). 자동 테스트로 MOTIS geocode 응답을 고정해 검증하고, 실
데이터에서의 매칭 품질은 S2에서 확인한다(U5).

---

## S7. 회귀 — ODsay 경로 불변 (spec 005 FR-007)

```bash
just test   # 기존 테스트가 수정 없이 통과해야 한다
```

| 확인 | 통과 기준 |
| --- | --- |
| `route_test.go`, `mcp_test.go`, `core/client_test.go`의 기존 케이스 | 수정 없이 통과 |
| ODsay 프로즈 출력 | 바이트 단위 동일 |
| 기존 에러 코드의 JSON 문서 | 바이트 단위 동일 (`docs`는 omitempty) |

기존 테스트를 **고쳐서** 통과시켜야 한다면 그것은 회귀다. `fareWon` 포인터화와 `Docs` 필드
추가는 둘 다 부재 시 와이어 포맷이 동일하도록 설계되었으므로, 테스트 수정이 필요하다면 설계가
어긋난 것이다.

---

## S8. 망라성 게이트 살아 있음 확인 (SC-004)

**의도적으로 실패시켜 보는 절차.**

```bash
# 1. internal/core/errors.go에 ErrMotis* 추가, 다른 건 아직 안 함
just test
# → TestErrorCodeExhaustive_EveryCoreErrorHasACode 실패해야 정상

# 2. taxonomy 등록 + classifyRouteError 분기 + coreErrorSamples 추가
just test
# → 통과
```

1번에서 실패를 **실제로 확인하고** 넘어간다. 게이트가 작동함을 증명하지 않고 통과만 시키면
SC-004를 검증한 것이 아니다.

---

## 완료 게이트

```bash
just fmt && just lint && just test
```

3개 OS(linux/macOS/windows) CI에서 전부 green이어야 한다 — 설정 파일 경로가 OS별로 갈리므로
이 매트릭스가 이번 기능의 실질적 게이트다(GYE-296).

## 수동 검증이 남는 항목

자동화할 수 없어 사람이 직접 확인해야 하는 것들이다. 이들이 끝나지 않으면 기능은 미완이다.

| 항목 | 시나리오 | 해소하는 미해결 |
| --- | --- | --- |
| 실 MOTIS 기동 + 대표 질의 3종 | S2 | U1, U5 |
| 그래프 빌드 자원 실측 | S2 | U2 |
| KTDB GTFS 경로·갱신 주기·커버리지 확인 | S2, S3 | U3 |
| 만료 타임테이블에서의 응답 관찰 | S2 | U4 |
| 문서만으로 구축 가능한지 제3자 검증 | S3 | — |
