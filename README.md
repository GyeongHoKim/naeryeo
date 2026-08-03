# 내려(naeryeo CLI/MCP)

<p align="center">
  <img src="./naeryeo.png" alt="naeryeo" width="320">
</p>

Claude Desktop, Claude Code 등 MCP 클라이언트에서 자연어로 대한민국 대중교통 경로를 물어볼 수 있는 CLI 겸 MCP 서버입니다.

naeryeo는 두 가지 방식으로 쓸 수 있습니다:

- **Skills & CLI** — SKILLS를 사용하면 AI 에이전트가 필요한 시점에 context로 올립니다(권장).
- **MCP 서버** — MCP 서버가 더 익숙하신 분들도 사용 가능하도록 stdio mcp 서버를 지원합니다.

두 방식 모두 `naeryeo` 바이너리와 **경로 검색 제공자 하나**가 필요합니다.

### 경로 검색 제공자 — 둘 중 하나를 고릅니다

| 제공자 | 설명 | 필요한 것 |
| --- | --- | --- |
| **자체 호스팅 (MOTIS)** | 오픈소스 경로 검색 엔진을 직접 운영합니다. 계정도 API 키도 없고, 호출 횟수 제한과 과금 정책에서 자유로우며, 검색 내용이 외부로 나가지 않습니다 | 엔진을 띄울 장비 ([자체 호스팅 안내](./docs/self-hosting.md)) |
| **ODsay** | 상용 대중교통 API입니다. 가장 빨리 시작할 수 있습니다 | [ODsay](https://lab.odsay.com) 앱키 |

어느 쪽을 고르든 사용법과 출력 형식은 동일합니다. 자체 호스팅을 권장하지만, 인프라를
운영할 여건이 아니라면 ODsay가 여전히 가장 빠른 경로입니다.

> **기존 사용자께**: v1부터 제공자를 **명시적으로 선택**해야 합니다. 키가 이미 저장돼
> 있어도 `naeryeo setup`을 한 번 다시 실행해야 합니다 — 자세한 내용은
> [마이그레이션](#기존-사용자-마이그레이션)을 참고하세요.

---

## 1. Skills & CLI

### 1-1. 설치

**SKILL을 설치**

[skills.sh](https://skills.sh)에서 naeryeo를 검색하거나 아래 명령어를 실행해 주세요

```bash
npx skills add GyeongHoKim/naeryeo
```

혹은 skills 폴더 하위에 있는 SKILL.md 를 복사해서 원하는 위치에 붙여넣으셔도 됩니다

**바이너리를 설치**

SKILL이 정상적으로 반영되었다면 AI 에이전트는 `naeryeo` 명령어를 CLI로 사용하게 됩니다. 따라서 그 전에 아래 명령어를 실행하여 바이너리를 설치합니다.

```bash
# macOS / Linux 사용자의 경우 (Homebrew)
brew install --cask GyeongHoKim/tap/naeryeo
```

```powershell
# Windows 사용자의 경우 (Scoop)
scoop bucket add naeryeo https://github.com/GyeongHoKim/scoop-bucket
scoop install naeryeo
```

```bash
# npm 으로 설치하고 싶은 경우
npm install -g @gyeonghokim/naeryeo
```

바이너리를 웹 브라우저에서 다운받고 싶다면 [GitHub Release](https://github.com/GyeongHoKim/naeryeo/releases)에서 플랫폼별 아카이브를 받을 수도 있습니다.  
다만, 이 경우에는 해당 바이너리가 해당 머신의 OS Path로 잡히도록 세팅하거나 혹은 바이너리를 기본적으로 여러분의 OS가 기본으로 path로 잡는 경로로 옮겨 주셔야 합니다.

### 1-2. 초기 설정

```bash
naeryeo setup
```

제공자 선택 → 제공자별 입력 → 장소 검색 설정을 한 번에 마칩니다. Enter만 눌러도 자체
호스팅 기본값으로 진행됩니다.

```
경로 검색 제공자를 선택하세요.
  1) 자체 호스팅 (MOTIS) — API 키가 필요 없습니다  [기본]
  2) ODsay — 앱키 발급이 필요합니다
  3) 저장된 자격증명 삭제
선택 [1]:

MOTIS 서버 주소를 입력하세요. API 키는 필요하지 않습니다.
주소 [http://localhost:8080]:

  연결 확인 중... 정상

건물명·주소로도 검색하시겠습니까? (역·정류장 이름만 쓸 경우 필요 없습니다)
  1) 사용 안 함  [기본]
  2) Kakao 장소 검색 사용 — REST API 키가 필요합니다
선택 [1]:

설정 요약
  경로 검색: 자체 호스팅 (MOTIS) — http://localhost:8080
  장소 검색: 사용 안 함
저장하시겠습니까? [Y/n]:

저장 완료
```

MOTIS를 고르면 저장 전에 **엔진이 실제로 응답하고 데이터가 적재되어 있는지** 확인합니다.
엔진은 떠 있는데 시간표 데이터가 없는 상태로 저장되는 일을 막기 위함입니다.

**비대화식 (CI·자동화·AI용)**

```bash
naeryeo setup --provider=motis --motis-url=http://motis.lan:8080   # 시크릿 없음
echo "$ODSAY_KEY" | naeryeo setup --provider=odsay                 # 키는 stdin
echo "$KAKAO_KEY" | naeryeo setup --geocoder=kakao
naeryeo setup --geocoder=none
naeryeo setup --delete=odsay|kakao|all
```

**시크릿을 받는 플래그는 없습니다.** 명령행에 적은 키는 셸 히스토리와 `ps` 출력에 평문으로
남기 때문에, 비밀값은 stdin으로만 받습니다.

API 키는 OS 키체인(macOS Keychain / Windows Credential Manager / Linux Secret
Service)에만 저장됩니다. 제공자 선택과 MOTIS 주소는 비밀값이 아니므로 설정 파일
(`~/.config/naeryeo/config.json`, macOS는 `~/Library/Application Support/naeryeo/`,
Windows는 `%AppData%\naeryeo\`)에 저장됩니다 — 저장할 비밀이 하나도 없는 자체 호스팅
사용자에게 키체인 잠금 해제를 요구하지 않기 위함입니다.

### 1-3. 사용

```bash
naeryeo route --from "강남역" --to "홍대입구역"
```

```
강남역 → 홍대입구역 (약 42분, 환승 1회)

1. 강남역에서 2호선 승차 (구로디지털단지 방면)
2. 신도림역에서 2호선 → 경의중앙선 환승
3. 홍대입구역 하차

요금: 1,500원
```

### 1-4. 건물명·주소로도 검색하기 (선택)

경로 검색 엔진(ODsay)은 **역·정류장 이름만** 인식합니다. "XXX 롯데시네마" 같은 건물명이나 도로명·지번 주소는 좌표로 바꿔 줄 별도의 장소 검색 서비스가 필요합니다. (이 설정은 CLI·MCP 두 방식 모두에 적용됩니다.)

- **없어도 되는 경우**: "강남역", "역삼"처럼 역·정류장 이름으로만 물어본다면 이 설정은 필요 없습니다. 장소 검색 키가 없어도 역/정류장 검색은 그대로 동작합니다.
- **쓰고 싶은 경우**:
  1. [Kakao Developers](https://developers.kakao.com)에서 애플리케이션을 만들고 **REST API 키**를 발급받은 뒤, 앱의 **카카오맵(로컬) 서비스를 활성화**합니다.
  2. 키를 등록합니다:

     ```bash
     naeryeo setup --geocoder
     ```

     ```
     Kakao REST API Key: ****************
     OS 키체인에 저장 완료
     ```

     장소 검색 키는 ODsay 키와 **별도 항목**으로 OS 키체인에 저장됩니다.
  3. 이제 건물명·주소로도 검색할 수 있습니다:

     ```bash
     naeryeo route --from "XXX 시네마" --to "강남역"
     ```

- **미설정 시**: 건물명·주소를 입력했는데 장소 검색 키가 없으면 "찾을 수 없습니다"와 함께 `naeryeo setup --geocoder`로 키를 설정하라는 안내가 표시됩니다.
- **삭제**: `naeryeo setup --delete=kakao`로 장소 검색 키만 지울 수 있습니다(다른 자격증명과 제공자 설정은 유지).
- **제공자와 독립**: 자체 호스팅이든 ODsay든 이 설정은 따로 켜고 끌 수 있습니다.

---

## 2. MCP 서버(stdio)

Claude Desktop 등 MCP 클라이언트에 붙여, 대화 중 자연어로 경로를 물어보는 방식입니다.

### 2-1. 설치 및 설정

바이너리 설치와 초기 설정은 **[1-1 설치](#1-1-설치)·[1-2 초기 설정](#1-2-초기-설정)과 동일**합니다. 건물명·주소 검색이 필요하면 [1-4](#1-4-건물명주소로도-검색하기-선택)의 `naeryeo setup --geocoder=kakao`도 함께 등록하세요.

CLI와 MCP 서버는 **같은 설정을 읽습니다.** 한쪽은 자체 호스팅, 다른 쪽은 ODsay로 갈리는 일이 구조적으로 없습니다.

### 2-2. 연결

`claude_desktop_config.json`에 아래 중 **설치 방식에 맞는 하나**를 추가하세요.

**바이너리를 설치한 경우** (Homebrew · Scoop · npm 전역 설치 — `naeryeo`가 PATH에 있음):

```json
{
  "mcpServers": {
    "naeryeo": {
      "command": "naeryeo",
      "args": ["mcp"]
    }
  }
}
```

**전역 설치 없이 npm으로 그때그때 실행하는 경우** (npx):

```json
{
  "mcpServers": {
    "naeryeo": {
      "command": "npx",
      "args": ["-y", "@gyeonghokim/naeryeo", "mcp"]
    }
  }
}
```

설정 파일에는 API 키를 넣지 않습니다 — 어느 방식이든 `naeryeo setup`이 저장한 값을 그대로 재사용합니다.

### 2-3. 사용

등록 후 Claude Desktop에서 이렇게 물어보면 됩니다:

> "지금 강남역에서 홍대입구역까지 대중교통으로 어떻게 가?"

---

## 명령어

| 명령어 | 설명 |
| --- | --- |
| `naeryeo setup` | 제공자·자격증명·장소 검색을 한 번에 설정하는 마법사 |
| `naeryeo setup --provider=motis --motis-url=<주소>` | 자체 호스팅 엔진으로 설정 — 질문은 건너뛰고 저장 여부만 확인합니다 |
| `naeryeo setup --provider=odsay` | ODsay로 설정 — 키는 stdin으로 입력 |
| `naeryeo setup --geocoder=kakao\|none` | 장소 검색 설정 — Kakao 키는 stdin으로 입력 |
| `naeryeo setup --delete=odsay\|kakao\|all` | 저장된 자격증명 삭제 (설정은 유지) |
| `naeryeo route --from <출발지> --to <도착지>` | 경로 검색 (CLI 모드) |
| `naeryeo route ... --json` | 결과를 JSON 문서 하나로 출력 — 성공·실패 모두 표준 출력, 실패 시 `error.code`로 원인 구분 (AI 에이전트용) |
| `naeryeo mcp` | MCP stdio 서버로 기동 |
| `naeryeo --version` | 버전 확인 |

## 자체 호스팅을 골랐을 때 남는 외부 의존

경로 검색은 자체 호스팅으로 외부 의존이 **사라집니다**. 장소 이름 해석도 대부분 함께
사라집니다.

MOTIS는 GTFS 정류장뿐 아니라 **OSM에서 온 건물·장소·주소까지** 색인합니다. 아래 셋 모두
외부 호출 0건으로 동작하는 것을 실측했습니다.

| 입력 | 외부 서비스 |
| --- | --- |
| 역·정류장 이름 (`강남역`) | **아니오** |
| 건물명 (`아이디스 타워`) | **아니오** |
| 도로명 주소 (`테헤란로 152`) | **아니오** |

Kakao 장소 검색은 **MOTIS 색인에 없는 이름을 위한 예비 수단**입니다. naeryeo는 항상 MOTIS에
먼저 묻고, 비어 있을 때만 — 키를 등록해 둔 경우에만 — Kakao에 다시 묻습니다. 켜지 않아도
건물명·주소 검색은 동작하며, 켜면 적중률이 조금 올라갑니다.

자세한 내용은 [자체 호스팅 안내](./docs/self-hosting.md#7-남는-외부-의존)에 있습니다.

## 기존 사용자 마이그레이션

v1은 **breaking change**입니다. 업그레이드 후 첫 검색은 실패하며, 아래 한 번의 재설정으로
해결됩니다.

```bash
naeryeo setup
```

| 바뀐 것 | 왜 | 무엇을 해야 하나 |
| --- | --- | --- |
| 제공자를 명시적으로 골라야 합니다 | 키가 저장돼 있다고 해서 제공자를 추정하면, 그 예외가 마이그레이션이 끝난 뒤에도 코드에 영구히 남습니다 | `naeryeo setup`을 한 번 실행. **저장된 키는 지워지지 않고 그대로 재사용**되므로 재발급은 필요 없습니다 |
| `naeryeo logout`이 없어졌습니다 | 설정이 키 하나에서 제공자·주소·지오코더·자격증명 넷으로 늘어나, 이를 바꾸는 명령을 하나로 유지하는 편이 낫습니다 | `naeryeo setup --delete=odsay\|kakao\|all` |
| `--geocoder`가 값을 받습니다 | 불리언으로는 "사용 안 함"을 명시적으로 고를 수 없습니다 | `--geocoder=kakao` 또는 `--geocoder=none` |

첫 실행 시 나오는 안내는 이렇습니다:

```
naeryeo route: 경로 검색 제공자가 설정되지 않았습니다
naeryeo setup을 실행해 자체 호스팅(MOTIS) 또는 ODsay 중 하나를 선택하세요
https://github.com/GyeongHoKim/naeryeo/blob/main/docs/self-hosting.md
```

## 아키텍처

```
naeryeo/
  cmd/naeryeo/       # 서브커맨드 진입점 (setup, route, mcp) + 제공자 선택·에러 코드
  internal/core/     # 공용 도메인 모델, 에러 계약, Geocoder 인터페이스, ODsay 클라이언트
  internal/motis/    # 자체 호스팅 MOTIS 연동 — 이름 해석(/api/v1/geocode) + 경로(/api/v6/plan)
  internal/geocode/  # 장소 검색(지오코딩) 연동 — 건물명·주소 → 좌표 (Kakao)
  internal/config/   # OS 키체인 기반 자격증명 + 평문 설정 파일(제공자·주소·지오코더)
  deploy/motis/      # 자체 호스팅 MOTIS 실행 레시피
  docs/              # 자체 호스팅 안내 문서
```

CLI와 MCP 모드는 **같은 제공자 선택 로직**을 공유합니다 — 두 진입점이 서로 다른 엔진을
쓰는 상태가 만들어질 수 없습니다. 경로 제공자(MOTIS/ODsay)와 장소 검색(Kakao/사용 안 함)은
독립된 축이라 네 조합이 모두 유효합니다.

## 사용 API

- **대중교통 경로 검색 (자체 호스팅)**: [MOTIS](https://github.com/motis-project/motis) — 오픈소스 멀티모달 라우팅 엔진입니다. GTFS 시간표와 OpenStreetMap 데이터를 사용자가 직접 준비해 운영하며, 외부로 나가는 호출이 없습니다. 구축 방법은 [docs/self-hosting.md](./docs/self-hosting.md).
- **대중교통 경로 검색 (상용)**: [ODsay API](https://lab.odsay.com)를 사용합니다. 카카오·네이버 지도 API는 대중교통 경로 검색 자체를 지원하지 않아 이 용도로는 쓰지 않으며, Google Directions API는 국내 지도 반출 규제(「공간정보의 구축 및 관리 등에 관한 법률」)로 국내 대중교통 길찾기에 사용할 수 없습니다.
- **건물명·주소 → 좌표(지오코딩, 선택)**: ODsay에는 건물명·주소를 좌표로 바꾸는 기능이 없어, 정류장으로 인식되지 않는 이름은 [Kakao Local 키워드 검색](https://developers.kakao.com/docs/latest/en/local/dev-guide)으로 좌표를 얻습니다. 이는 경로 검색이 아니라 이름→좌표 해석에만 쓰이며, 장소 검색 키를 등록하지 않으면 이 단계는 비활성화됩니다.

## 라이선스

MIT
