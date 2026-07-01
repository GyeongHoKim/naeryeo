#!/usr/bin/env node
// Launcher shim: exec the prebuilt binary that postinstall placed next to this
// file, passing through args and stdio (so `naeryeo mcp` works as an MCP stdio
// server). If the binary is missing, postinstall did not run — most commonly
// because of `--ignore-scripts` or an unsupported platform.
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const binName = process.platform === "win32" ? "naeryeo.exe" : "naeryeo";
const bin = join(here, binName);

if (!existsSync(bin)) {
  console.error(
    "[naeryeo] 바이너리를 찾을 수 없습니다. postinstall이 실행되지 않았을 수 있습니다" +
      "(--ignore-scripts) 또는 미지원 플랫폼입니다.\n" +
      "  해결: `npm rebuild @gyeonghokim/naeryeo`, 또는 Homebrew/Scoop 설치를 사용하세요:\n" +
      "  https://github.com/GyeongHoKim/naeryeo",
  );
  process.exit(1);
}

const res = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (res.error) {
  console.error(`[naeryeo] 실행 실패: ${res.error.message}`);
  process.exit(1);
}
process.exit(res.status ?? 0);
