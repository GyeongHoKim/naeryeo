# Quickstart: 구조화된 출력 계약 검증

**Feature**: 005-structured-output-contract

이 기능이 실제로 동작하는지 종단으로 확인하는 절차. 상세 스키마는
[contracts/](./contracts/)를, 코드 목록은 [contracts/error-codes.md](./contracts/error-codes.md)를 참조.

## 사전 조건

```bash
mise install          # 툴체인 (최초 1회)
go build ./...
```

`jq`가 있으면 JSON 검증이 편하다 (`brew install jq`). 없어도 `python3 -m json.tool`로 대체 가능.

## 1. 자동화 게이트

```bash
just check            # fmt → lint → test 전부 통과해야 함
```

핵심 테스트가 실제로 도는지 확인:

```bash
go test ./cmd/naeryeo/ -run 'TestClassifyRouteError|TestErrorCodeExhaustive|TestRunRouteJSON|TestFindTransitRouteTool' -v
```

## 2. 망라성 게이트가 살아 있는지 (SC-004)

게이트가 형식만 갖춘 게 아닌지 직접 확인한다.

> ⚠️ 아래는 `internal/core/errors.go`를 **일시적으로 수정**한다. 되돌릴 때
> `git checkout`을 쓰면 그 파일의 다른 미커밋 변경까지 사라지므로, 아래처럼
> **추가한 줄만 직접 지우거나** `git stash`로 감싼다.

```bash
cp internal/core/errors.go /tmp/errors.go.bak     # 원본 보관

cat >> internal/core/errors.go <<'EOF'

// ErrProbeTemp is a temporary probe. Delete after verifying the gate.
var ErrProbeTemp = errors.New("core: probe")
EOF

go test ./cmd/naeryeo/ -run TestErrorCodeExhaustive
```

**기대**: 테스트 **실패**. 메시지가 `ErrProbeTemp`를 지목하고 코드 부여를 지시해야 한다.

```bash
cp /tmp/errors.go.bak internal/core/errors.go     # 되돌리기 (다른 변경 보존)
go test ./cmd/naeryeo/ -run TestErrorCodeExhaustive   # 다시 통과
git diff --stat internal/core/errors.go           # 비어 있어야 함
```

## 3. 하위 호환 — 프로즈 출력 (FR-007, SC-007)

```bash
naeryeo route --from "강남역" --to "홍대입구역"
```

**기대**: 기존과 동일한 프로즈. 성공은 stdout, 실패는 stderr, 실패 시 exit 1.

```bash
naeryeo route --from "강남역" --to "홍대입구역" >/dev/null
echo "exit=$?"
```

## 4. `--json` 성공 (US2)

```bash
naeryeo route --from "강남역" --to "홍대입구역" --json | jq .
```

**기대**: `totalTimeMinutes`·`transferCount`·`fareWon`·`steps`를 가진 문서 하나. `error` 키 없음.

stdout만 캡처해도 온전한지:

```bash
naeryeo route --from "강남역" --to "홍대입구역" --json 2>/dev/null | jq -e '.error == null' && echo "성공 판별 OK"
```

## 5. `--json` 실패가 stdout으로 나가는지 (FR-008, SC-006)

**키는 설정된 상태를 유지**하고, 인식 불가한 지점 이름으로 실패를 유도한다 — 키를 지우면
`point_not_found`가 아니라 `api_key_missing`이 나온다.

```bash
naeryeo route --from "존재하지않는장소일이삼사" --to "홍대입구역" --json 2>/dev/null
echo "exit=$?"
```

**기대**:

- stdout에 실패 문서 (`{"error":{"code":"point_not_found",...}}`)
- `exit=1`
- **stderr를 버려도 실패 이유가 남아 있음** ← 이 기능의 핵심

```bash
# 한 번의 캡처로 코드까지 뽑히는지
naeryeo route --from "존재하지않는장소일이삼사" --to "홍대입구역" --json 2>/dev/null | jq -r '.error.code'
```

## 6. 인자 검증 실패도 JSON으로 (FR-015)

```bash
naeryeo route --json 2>/dev/null | jq -r '.error.code'
```

**기대**: `invalid_arguments`. stdout이 파싱 가능한 문서 하나여야 한다 (사용법 텍스트가
섞이면 실패).

## 7. `--json` + `--debug` 조합 (FR-014)

```bash
naeryeo route --from "존재하지않는장소일이삼사" --to "홍대입구역" --json --debug 2>/dev/null | jq .
```

**기대**: stdout은 여전히 **파싱 가능한 JSON 문서 하나**. 진단 정보는 stderr로만.

```bash
naeryeo route --from "존재하지않는장소일이삼사" --to "홍대입구역" --json --debug 2>&1 >/dev/null | head -5
```

**기대**: stderr에 원본 체인. **ODsay/Kakao API 키가 평문으로 보이면 안 된다** (GYE-293 회귀 확인).

## 8. 원본 오류 미노출 (US3, SC-003)

경로 제공자의 미분류 오류는 단위 테스트로 확인하는 편이 확실하다 (실제 ODsay 500을
재현하기 어려움).

```bash
go test ./cmd/naeryeo/ -run 'TestClassifyRouteError' -v 2>&1 | grep -i 'upstream'
```

**기대**: wrapped 에러(`fmt.Errorf("%w: internal db timeout at shard 7", core.ErrUpstreamRejected)`)를
넘겨도 `message`·`hint` 어디에도 `shard 7`이 나타나지 않는다.

## 9. MCP 경로 (US1, FR-017)

```bash
go test ./cmd/naeryeo/ -run 'TestFindTransitRouteTool' -v
```

**기대**: 실패 응답에 `isError: true` + `structuredContent.error.code`가 존재하고, CLI와
동일한 코드·문구를 낸다.

수동 확인 (MCP 호스트에 등록된 경우): Claude Desktop/Code에서 인식 불가한 지점으로 경로를
물어보고, 도구 결과에 구조화된 에러가 실리는지 확인한다.

## 10. SKILL.md 일치 (SC-008)

구현이 실제로 내는 코드 집합을 정본으로 삼아 문서와 대조한다.

```bash
# 1) 구현이 정의한 코드 (errcode.go의 상수 리터럴)
grep -oE '"[a-z_]+"' cmd/naeryeo/errcode.go | tr -d '"' | sort -u > /tmp/codes-impl.txt

# 2) SKILL.md의 Handling errors 섹션이 나열하는 코드
sed -n '/^## Handling errors/,/^## /p' skills/naeryeo/SKILL.md \
  | grep -oE '`[a-z_]+`' | tr -d '`' | sort -u > /tmp/codes-doc.txt

diff /tmp/codes-impl.txt /tmp/codes-doc.txt
```

**기대**: `diff`가 비어 있음. 차이가 나오면 어느 쪽이 정본인지
[error-codes.md](./contracts/error-codes.md)로 판정한다.

> 코드 상수 외의 문자열 리터럴이 섞여 나올 수 있으므로, `diff` 결과는 기계 판정이 아니라
> **육안 확인의 출발점**으로 쓴다. 마크다운 표를 파싱하는 자동 테스트를 두지 않는 이유는
> [contracts/skill-md.md](./contracts/skill-md.md) 참조.

## 완료 판정

- [ ] `just check` 통과
- [ ] §2 게이트가 실제로 실패했다가 복구됨
- [ ] §3 프로즈 출력이 기존과 동일
- [ ] §5 실패 문서가 stdout, exit 1
- [ ] §6 인자 오류도 JSON
- [ ] §7 `--debug` 조합에서도 stdout 파싱 가능, 키 미노출
- [ ] §8 원본 오류 문자열 미노출
- [ ] §9 MCP 실패에 구조화된 코드 존재
- [ ] §10 SKILL.md와 코드 집합 일치
