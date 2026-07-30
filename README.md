# 내려(naeryeo CLI/MCP)

<p align="center">
  <img src="./naeryeo.png" alt="naeryeo" width="320">
</p>

Claude Desktop, Claude Code 등 MCP 클라이언트에서 자연어로 대한민국 대중교통 경로를 물어볼 수 있는 CLI 겸 MCP 서버입니다.

naeryeo는 두 가지 방식으로 쓸 수 있습니다:

- **Skills & CLI** — SKILLS를 사용하면 AI 에이전트가 필요한 시점에 context로 올립니다(권장).
- **MCP 서버** — MCP 서버가 더 익숙하신 분들도 사용 가능하도록 stdio mcp 서버를 지원합니다.

두 방식 모두 `naeryeo` 바이너리와 ODsay API 키가 필요합니다.

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

### 1-2. API 키 등록

경로 검색에는 ODsay API 키가 필요합니다. [ODsay](https://lab.odsay.com)에서 앱키를 발급받은 뒤:

```bash
naeryeo setup
```

```
ODsay API Key: ****************
OS 키체인에 저장 완료
```

API 키는 OS 키체인(macOS Keychain / Windows Credential Manager / Linux Secret Service)에만 저장되며, 평문 파일에는 저장되지 않고 외부로도 전송되지 않습니다.

**(선택) 건물명·주소 검색용 키** — 아래 1-4 참고.

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
- **삭제**: `naeryeo logout --geocoder`로 장소 검색 키만 지울 수 있습니다(ODsay 키는 유지).

---

## 2. MCP 서버(stdio)

Claude Desktop 등 MCP 클라이언트에 붙여, 대화 중 자연어로 경로를 물어보는 방식입니다.

### 2-1. 설치 및 키 등록

바이너리 설치와 API 키 등록은 **[1-1 설치](#1-1-설치)·[1-2 API 키 등록](#1-2-api-키-등록)과 동일**합니다(`naeryeo` 바이너리 설치 후 `naeryeo setup`). 건물명·주소 검색이 필요하면 [1-4](#1-4-건물명주소로도-검색하기-선택)의 `naeryeo setup --geocoder`도 함께 등록하세요.

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

설정 파일에는 API 키를 넣지 않습니다 — 어느 방식이든 `naeryeo setup`으로 OS 키체인에 저장된 값을 그대로 재사용합니다.

### 2-3. 사용

등록 후 Claude Desktop에서 이렇게 물어보면 됩니다:

> "지금 강남역에서 홍대입구역까지 대중교통으로 어떻게 가?"

---

## 명령어

| 명령어 | 설명 |
| --- | --- |
| `naeryeo setup` | ODsay(경로 검색) API 키 등록 |
| `naeryeo setup --geocoder` | 장소 검색(Kakao) API 키 등록 — 건물명·주소 검색용 (선택) |
| `naeryeo logout` | 저장된 ODsay API 키 삭제 |
| `naeryeo logout --geocoder` | 저장된 장소 검색 API 키 삭제 |
| `naeryeo route --from <출발지> --to <도착지>` | 경로 검색 (CLI 모드) |
| `naeryeo route ... --json` | 결과를 JSON 문서 하나로 출력 — 성공·실패 모두 표준 출력, 실패 시 `error.code`로 원인 구분 (AI 에이전트용) |
| `naeryeo mcp` | MCP stdio 서버로 기동 |
| `naeryeo --version` | 버전 확인 |

## 아키텍처

```
naeryeo/
  cmd/naeryeo/       # 서브커맨드 진입점 (setup, route, mcp)
  internal/core/     # 경로 검색 로직 (ODsay 클라이언트, 공용 도메인 모델, Geocoder 인터페이스)
  internal/geocode/  # 장소 검색(지오코딩) 연동 — 건물명·주소 → 좌표 (Kakao)
  internal/config/   # OS 키체인 연동 (go-keyring) 기반 API 키 저장/조회
```

CLI와 MCP 모드는 같은 `internal/core` 로직을 공유하며, 진입점만 다릅니다.

## 사용 API

- **대중교통 경로 검색**: [ODsay API](https://lab.odsay.com)를 사용합니다. 카카오·네이버 지도 API는 대중교통 경로 검색 자체를 지원하지 않아 이 용도로는 쓰지 않으며, Google Directions API는 국내 지도 반출 규제(「공간정보의 구축 및 관리 등에 관한 법률」)로 국내 대중교통 길찾기에 사용할 수 없습니다.
- **건물명·주소 → 좌표(지오코딩, 선택)**: ODsay에는 건물명·주소를 좌표로 바꾸는 기능이 없어, 정류장으로 인식되지 않는 이름은 [Kakao Local 키워드 검색](https://developers.kakao.com/docs/latest/en/local/dev-guide)으로 좌표를 얻습니다. 이는 경로 검색이 아니라 이름→좌표 해석에만 쓰이며, 장소 검색 키를 등록하지 않으면 이 단계는 비활성화됩니다.

## 라이선스

MIT
