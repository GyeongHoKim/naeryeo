# Contract: `skills/naeryeo/SKILL.md` 갱신

**Feature**: 005-structured-output-contract

`SKILL.md`는 AI 에이전트가 읽는 문서다. 코드 taxonomy를 만들어도 이 문서가 산문으로 에러를
중복 정의하는 한 드리프트가 계속된다 — 이 기능의 실질 가치가 여기서 실현된다.

## 갱신 대상

### 1. `## Usage` §Option A (현재 `SKILL.md:87-108`)

`--json`을 **1순위 호출 형태**로 제시한다 (FR-020).

```bash
naeryeo route --from "강남역" --to "홍대입구역" --json
```

예시 출력을 프로즈에서 성공 JSON 문서로 교체하고, "`error` 키가 있으면 실패"라는 판별
규칙을 명시한다. 사람에게 보여줄 용도의 프로즈 형태는 부차 옵션으로 남긴다.

### 2. `## Handling errors` (현재 `SKILL.md:129-143`)

영어 산문 5종 나열을 **코드 표 기반으로 재작성**한다 (FR-019).
[error-codes.md](./error-codes.md)의 표에서 파생하며, 반드시 명시할 것:

- `geocoder_rate_limited` (재시도 ✅) ↔ `geocoder_rejected` (입력 재작성) 구분
- `geocoder_auth_failed` (재등록으로 해결) ↔ `geocoder_forbidden` (콘솔 설정, 재등록 무의미) 구분
  — 현재 `SKILL.md:142-143`이 한 항목으로 뭉뚱그려 놓았다
- `message`는 사용자에게 그대로 전달하고, **문자열 매칭은 하지 말 것**
- `hint`가 있으면 사용자에게 함께 전달할 것
- 모르는 코드를 만나면 `message`를 전달하고 재시도하지 말 것

### 3. `## Common Mistakes` (현재 `SKILL.md:145-155`)

프로즈 에러 문구를 인용하는 부분(`--from과 --to를 모두 입력해야 합니다` 등)을 코드 기준으로
정정한다 (FR-021).

## 검증

문서가 나열하는 코드 집합과 구현의 코드 집합이 일치해야 한다 (SC-008).
문서에만 있거나 코드에만 있는 항목이 0건.

> 이 일치는 사람이 확인한다 — 마크다운 표를 파싱해 대조하는 테스트는 문서 서식 변경에
> 취약하고, 얻는 것 대비 유지 비용이 크다 (헌법 원칙 I). 대신 error-codes.md를 정본으로
> 두어 참조 지점을 하나로 만든다.

## 충돌 주의

GYE-294·GYE-295도 `SKILL.md`를 수정한다 (`naeryeo logout` 제거, provider 스위치 추가).
먼저 병합되는 쪽을 기준으로 rebase한다.
