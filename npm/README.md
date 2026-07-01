# @gyeonghokim/naeryeo

Claude Desktop·Claude Code 등 MCP 클라이언트에서 자연어로 대한민국 대중교통 경로를 물어볼 수 있는 CLI 겸 MCP stdio 서버입니다. [ODsay API](https://lab.odsay.com) 기반이며, 건물명·주소 검색은 Kakao Local(선택)로 보강합니다.

이 npm 패키지는 설치 시 플랫폼에 맞는 **미리 빌드된 `naeryeo` 바이너리**를 [GitHub Release](https://github.com/GyeongHoKim/naeryeo/releases)에서 내려받습니다(게시물은 [provenance](https://docs.npmjs.com/generating-provenance-statements) 서명).

```bash
# 전역 설치
npm install -g @gyeonghokim/naeryeo

# 또는 설치 없이 실행
npx @gyeonghokim/naeryeo mcp
```

설정·사용법·명령어·MCP 연결·아키텍처 등 **전체 문서는 프로젝트 README를 참고하세요**:
https://github.com/GyeongHoKim/naeryeo#readme

> `npm install --ignore-scripts` 환경이나 오프라인에서는 바이너리 다운로드가 동작하지 않습니다. 그 경우 Homebrew·Scoop 또는 GitHub Release 바이너리를 사용하세요.

## 라이선스

MIT
