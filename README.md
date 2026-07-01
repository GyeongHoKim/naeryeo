# 내려(naeryeo CLI/MCP)

<p align="center">
  <img src="./naeryeo.png" alt="naeryeo" width="320">
</p>

Claude Desktop, Claude Code 등 MCP 클라이언트에서 자연어로 대한민국 대중교통 경로를 물어볼 수 있는 CLI 겸 MCP 서버입니다.

## 설치

### Skills 디렉터리 (Claude Code · Cursor 등 · 권장)

Claude Code, Cursor 같은 AI 에이전트를 쓴다면 [skills.sh](https://skills.sh)를 통해 한 번에 설치하는 것을 권장합니다:

```bash
npx skills add GyeongHoKim/naeryeo
```

설치된 스킬이 에이전트에게 naeryeo 설치·설정·사용법을 안내합니다. 이후 아래 방법 중 하나로 `naeryeo` 바이너리를 설치하고 `naeryeo setup`으로 ODsay 키를 등록하면 됩니다. 건물명·주소로도 검색하고 싶다면 `naeryeo setup --geocoder`로 장소 검색 키를 추가로 등록하세요(선택).

### Homebrew (macOS / Linux)

```bash
brew install --cask GyeongHoKim/tap/naeryeo
```

### Scoop (Windows)

```powershell
scoop bucket add naeryeo https://github.com/GyeongHoKim/scoop-bucket
scoop install naeryeo
```

### npm (Node.js 환경)

```bash
npx naeryeo mcp
```

## 시작하기

### 1. API 키 발급

[ODsay API](https://lab.odsay.com)에서 앱키를 발급받으세요.

### 2. Setup

```bash
naeryeo setup
```

```
ODsay API Key: ****************
OS 키체인에 저장 완료
```

API 키는 OS 키체인(macOS Keychain / Windows Credential Manager / Linux Secret Service)에만 저장되며, 평문 파일에는 저장되지 않고 외부로도 전송되지 않습니다. 키체인을 사용할 수 없는 환경(예: Secret Service가 없는 headless Linux)에서는 보안을 위해 `naeryeo setup`이 에러를 내고 동작을 거부합니다 — 평문 파일로의 폴백은 지원하지 않습니다.

### 3. CLI로 바로 써보기

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

### 4. 건물명·주소로도 검색하기 (선택)

경로 검색 엔진(ODsay)은 **역·정류장 이름만** 인식합니다. "XXX 롯데시네마" 같은 건물명이나 도로명·지번 주소는 좌표로 바꿔 줄 별도의 장소 검색 서비스가 필요합니다.

- **없어도 되는 경우**: "강남역", "역삼"처럼 역·정류장 이름으로만 물어본다면 이 설정은 필요 없습니다. 장소 검색 키가 없어도 역/정류장 검색은 그대로 동작합니다.
- **쓰고 싶은 경우**:
  1. [Kakao Developers](https://developers.kakao.com)에서 애플리케이션을 만들고 **REST API 키**를 발급받습니다.
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

## Claude Desktop / Claude Code에 연결하기

`claude_desktop_config.json`에 다음을 추가하세요:

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

설정 파일에는 API 키를 넣지 않습니다 — `naeryeo setup`으로 OS 키체인에 저장된 값을 그대로 재사용합니다.

등록 후 Claude Desktop에서 이렇게 물어보면 됩니다:

> "지금 강남역에서 홍대입구역까지 대중교통으로 어떻게 가?"

## 명령어

| 명령어 | 설명 |
| --- | --- |
| `naeryeo setup` | ODsay(경로 검색) API 키 등록 |
| `naeryeo setup --geocoder` | 장소 검색(Kakao) API 키 등록 — 건물명·주소 검색용 (선택) |
| `naeryeo logout` | 저장된 ODsay API 키 삭제 |
| `naeryeo logout --geocoder` | 저장된 장소 검색 API 키 삭제 |
| `naeryeo route --from <출발지> --to <도착지>` | 경로 검색 (CLI 모드) |
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

## 왜 stdio인가

MCP 스펙(2025-11-25)에서 stdio는 로컬 단일 사용자 시나리오의 공식 권장 transport입니다. `naeryeo`는 로컬에서 실행되는 개인용 도구라 별도의 HTTP 서버 생명주기 관리 없이, MCP 클라이언트가 프로세스를 직접 spawn하고 정리해주는 stdio 모델이 가장 단순하고 안전합니다.

## 사용 API

- **대중교통 경로 검색**: [ODsay API](https://lab.odsay.com)를 사용합니다. 카카오·네이버 지도 API는 대중교통 경로 검색 자체를 지원하지 않아 이 용도로는 쓰지 않으며, Google Directions API는 국내 지도 반출 규제(「공간정보의 구축 및 관리 등에 관한 법률」)로 국내 대중교통 길찾기에 사용할 수 없습니다.
- **건물명·주소 → 좌표(지오코딩, 선택)**: ODsay에는 건물명·주소를 좌표로 바꾸는 기능이 없어, 정류장으로 인식되지 않는 이름은 [Kakao Local 키워드 검색](https://developers.kakao.com/docs/latest/en/local/dev-guide)으로 좌표를 얻습니다. 이는 경로 검색이 아니라 이름→좌표 해석에만 쓰이며, 장소 검색 키를 등록하지 않으면 이 단계는 비활성화됩니다.

## 라이선스

MIT
