# Implementation Plan: 건물명·주소(POI) 출발지/도착지 지원

**Branch**: `004-poi-address-search` | **Date**: 2026-07-02 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-poi-address-search/spec.md`

## Summary

역/정류장으로 해석되지 않는 건물명·주소(POI) 입력을, 정류장 검색 실패 시 외부 지오코더
(Kakao Local 키워드 검색)로 폴백해 좌표로 해석한 뒤 기존 좌표 기반 경로 검색으로 잇는다.
지오코더는 별도 API 키를 요구하며, 이 키는 OS 키체인에 ODsay 키와 별개 항목으로 저장한다
(`setup --geocoder`로 등록). core는 소비자 정의 `Geocoder` 인터페이스로 지오코더를 선택적
주입받아, 지오코더 미설정 시 기존 정류장 전용 동작을 그대로 유지한다(회귀 없음). 사용자 대면
기능 변경이므로 `README.md`(지오코딩 필요성·선택성·설정법, "사용 API" 섹션 정정)도 함께
갱신한다.

## Technical Context

**Language/Version**: Go (기존 모듈 `github.com/GyeongHoKim/naeryeo`, go.mod 기준)

**Primary Dependencies**: 표준 라이브러리 `net/http`+`encoding/json`(외부 HTTP), 기존
`github.com/zalando/go-keyring`(키체인). 새 서드파티 의존 없음.

**Storage**: OS 키체인(go-keyring). ODsay 키(`odsay-api-key`)와 지오코더 키
(`geocoder-api-key`)를 동일 서비스명 `naeryeo` 아래 별도 username으로 저장.

**Testing**: `go test -race ./...`, `net/http/httptest`로 외부 API 모킹, 가짜 Geocoder/backend
주입. 실제 외부 서비스 통합 테스트 없음.

**Target Platform**: 데스크톱 CLI + MCP stdio 서버(macOS/Windows/Linux, 키체인 가용 환경).

**Project Type**: Single project — CLI + 라이브러리(내부 패키지). 002/003과 동일 구조.

**Performance Goals**: 경로 1건 조회 기준 기존 SC(수 초 내 응답) 유지. 지오코더 폴백은 정류장
검색 실패 시에만 추가 1회 호출(FR-003: 정류장 성공 시 지오코더 호출 없음).

**Constraints**: 무한 대기 금지(context 전파 + HTTP 타임아웃 재사용, FR-009). 키체인
불가용 시 평문 저장 폴백 금지(기존 config 정책 유지).

**Scale/Scope**: 단일 사용자 CLI. 입력당 최대 2개 지점(from/to) 해석.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Idiomatic Go First** ✅ — 소비자 측 소형 인터페이스(`core.Geocoder`), 명시적 에러 처리
  (sentinel + `errors.Is/As`), 표준 라이브러리만 사용, 새 추상화(`internal/geocode`)는 실제
  두 번째 외부 연동이라는 구체 필요로 정당화됨(speculative 아님).
- **II. Unit Tests Are Mandatory** ✅ — 신규 exported 심볼(config 자격증명 API, `core.Geocoder`/
  `Coordinate`/폴백 분기, `geocode.Kakao`, CLI 플래그 처리) 모두 동일 커밋에 테이블 기반 테스트
  동반. 해피패스+에지(0건/다건/인증실패/불가용/nil 지오코더) 커버.
- **III. Automated Quality Gates** ✅ — `just fmt`/`lint`/`test` 게이트 그대로 적용. 새 게이트
  불필요.
- **IV. Commit Discipline** ✅ — Conventional Commits, 변경+테스트 동일 커밋, 인간 확인 후 커밋.

**위반 없음** → Complexity Tracking 비움.

## Project Structure

### Documentation (this feature)

```text
specs/004-poi-address-search/
├── plan.md              # 본 파일
├── research.md          # Phase 0 산출물
├── data-model.md        # Phase 1 산출물
├── quickstart.md        # Phase 1 산출물
├── contracts/           # Phase 1 산출물
│   ├── config-credential.md
│   ├── core-geocoder.md
│   ├── geocode-kakao.md
│   ├── cli.md
│   └── docs-readme.md   # README 갱신 계약(사용자 문서)
├── checklists/
│   └── requirements.md  # /speckit-specify 산출물
└── tasks.md             # /speckit-tasks 산출물(본 명령 아님)
```

### Source Code (repository root)

```text
cmd/naeryeo/
├── main.go              # 변경: config.Load(ODsayAPIKey), findRoute 클로저 내부에서 지오코더 주입,
│                        #       runRoute/buildMCPServer에 loadGeocoder 인자 전달
├── setup.go             # 변경: args를 flag로 파싱(--geocoder) → 대상 자격증명/프롬프트 분기
├── setup_test.go        # 변경: 플래그별 대상 검증
├── logout.go            # 변경: args를 flag로 파싱(--geocoder) → 대상 자격증명 분기
├── logout_test.go       # 변경
├── route.go             # 변경: routeErrorMessage(err, geocoderConfigured) 확장 → FR-007 힌트
├── route_test.go        # 변경
├── mcp.go               # 변경: loadGeocoder 인자, routeErrorMessage 확장 반영(CLI와 문구 통일)
└── ...                  # findRoute 클로저 타입은 불변(지오코더는 클로저 내부 주입) — contracts/cli.md

internal/config/
├── config.go            # 변경: Credential 타입 + Save/Load/Delete(cred, ...) 파라미터화
└── config_test.go       # 변경: 자격증명별 독립 저장/조회/삭제

internal/core/
├── client.go            # 변경: Geocoder 인터페이스, Coordinate, Client.Geocoder, resolveStation 폴백
├── errors.go            # 변경: ErrGeocoderAuthFailed 추가
└── client_test.go       # 변경: 가짜 Geocoder 주입 폴백 테스트

internal/geocode/        # 신규 패키지
├── kakao.go             # geocode.Kakao: core.Geocoder 구현(Kakao 키워드 검색)
├── kakao_test.go        # httptest 기반 테이블 테스트
├── errors.go            # geocode.ErrNotFound/ErrAuthFailed/ErrUnavailable(공개 sentinel)
└── doc.go

README.md                # 변경: 지오코딩 필요성·선택성·설정법 추가, "사용 API" 섹션 정정
                         #       (모순 제거), 명령어 표·아키텍처 갱신 — contracts/docs-readme.md
```

**Structure Decision**: 기존 단일 프로젝트 구조를 유지하고 외부 연동 하나(`internal/geocode`)를
추가한다. core는 자신이 소비하는 `Geocoder` 인터페이스만 정의하고, Kakao 구현과 config 조회는
각각 `internal/geocode`와 `cmd/naeryeo` 진입점이 담당해 계층 결합을 낮춘다(002 원칙 계승).

## Complexity Tracking

> Constitution Check 위반 없음 — 비움.
