# Specification Quality Checklist: 구조화된 출력 계약 (`--json` + 에러 코드)

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-07-30
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Validation Notes

**Iteration 1 — issues found and fixed:**

1. *Implementation detail leak*: 초안이 Go 타입명(`*ErrGeocoderRejected` 등)과 구체
   JSON 필드명(`error`, `message`, `hint`, `docs`)을 그대로 사용했다. 실패 상황을
   자연어로 서술하고, 필드는 Key Entities에서 역할 단위로 기술하도록 재작성.
   에러 코드 문자열 자체는 **외부에 공개되는 계약**(HTTP 상태 코드와 동일한 성격)이나,
   FR-003에서 코드 문자열 대신 "실패 상황 → 올바른 후속 행동" 표로 표현해 명명은
   설계 단계로 넘겼다.
2. *Edge case가 요구사항으로 이어지지 않음*: "인자 검증 실패 시 기계 판독 모드 동작"이
   Edge Cases에만 있고 대응 FR이 없었다. FR-015 신설.
3. *`--json`/`--debug` 플래그명 노출*: 사양 본문에서 "기계 판독 모드"/"진단 모드"로
   일반화. 제목과 Input에만 사용자 표현 그대로 유지.

**결정 사항 (clarification 없이 정보에 근거한 기본값으로 처리):**

- 실패 문서의 출력 스트림 → 표준 출력 (Assumptions에 근거 기술)
- 진단 모드 + 기계 판독 모드 조합 시 진단 정보의 위치 → 별도 스트림
- 미래 기능용 코드·필드 사전 예약 → **범위에서 제외** (헌법 원칙 I). GYE-292 원안과
  의도적으로 다른 결정이므로 Assumptions에 근거를 명시했다. 이 판단을 뒤집으려면
  `/speckit-clarify`로 재검토할 것.

## Notes

- All items pass. Ready for `/speckit-clarify` (optional) or `/speckit-plan`.
