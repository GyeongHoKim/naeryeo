# naeryeo (내려)

<p align="center">
  <img src="./naeryeo.png" alt="naeryeo" width="320">
</p>

Claude Desktop, Claude Code 등 MCP 클라이언트에서 자연어로 대한민국 대중교통 경로를 물어볼 수 있는 CLI 겸 MCP 서버입니다.

## 설치

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
| `naeryeo setup` | API 키 등록 |
| `naeryeo logout` | 저장된 API 키 삭제 |
| `naeryeo route --from <출발지> --to <도착지>` | 경로 검색 (CLI 모드) |
| `naeryeo mcp` | MCP stdio 서버로 기동 |
| `naeryeo --version` | 버전 확인 |

## 아키텍처

```
naeryeo/
  cmd/naeryeo/       # 서브커맨드 진입점 (setup, route, mcp)
  internal/core/     # 경로 검색 로직 (ODsay 클라이언트, 공용 도메인 모델)
  internal/config/   # OS 키체인 연동 (go-keyring) 기반 API 키 저장/조회
```

CLI와 MCP 모드는 같은 `internal/core` 로직을 공유하며, 진입점만 다릅니다.

## 왜 stdio인가

MCP 스펙(2025-11-25)에서 stdio는 로컬 단일 사용자 시나리오의 공식 권장 transport입니다. `naeryeo`는 로컬에서 실행되는 개인용 도구라 별도의 HTTP 서버 생명주기 관리 없이, MCP 클라이언트가 프로세스를 직접 spawn하고 정리해주는 stdio 모델이 가장 단순하고 안전합니다.

## 사용 API

대중교통 경로 검색에는 [ODsay API](https://lab.odsay.com)를 사용합니다. 카카오·네이버 지도 API는 대중교통 경로 검색을 지원하지 않아 제외했습니다. Google Directions API는 국내 지도 반출 규제(「공간정보의 구축 및 관리 등에 관한 법률」)로 국내 대중교통 길찾기에 사용할 수 없습니다.

## 라이선스

MIT