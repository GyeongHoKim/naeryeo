// postinstall: download the prebuilt naeryeo binary for this platform from the
// GitHub Release matching this package's version, verify its sha256 against the
// release checksums.txt, and place it next to the launcher shim in ./bin.
//
// Zero runtime dependencies by design: uses global fetch (Node 18+),
// node:crypto for hashing, and the system `tar` for extraction (bsdtar ships
// with macOS, Linux, and Windows 10 1803+ and handles both .tar.gz and .zip).
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const REPO = "GyeongHoKim/naeryeo";
const RELEASES = `https://github.com/${REPO}/releases`;

const here = dirname(fileURLToPath(import.meta.url));
const pkgRoot = join(here, "..");
const { version } = JSON.parse(readFileSync(join(pkgRoot, "package.json"), "utf8"));

function fail(msg) {
  console.error(`\n[naeryeo] 설치 실패: ${msg}\n`);
  process.exit(1);
}

// On Windows, bare `tar` on PATH is often Git's GNU tar, which cannot extract
// .zip and misreads drive-letter paths (`C:` looks like a remote host). The
// bundled bsdtar at System32\tar.exe handles both .tar.gz and .zip, so prefer
// it explicitly. macOS/Linux `tar` (also bsdtar/GNU) handles .tar.gz fine.
function resolveTar() {
  if (process.platform === "win32") {
    const sys = join(process.env.SystemRoot || "C:\\Windows", "System32", "tar.exe");
    if (existsSync(sys)) return sys;
  }
  return "tar";
}

// Unstamped/dev checkout (e.g. `npm install` inside the repo). Nothing to fetch.
if (!version || version === "0.0.0") {
  console.error("[naeryeo] 개발용(0.0.0) 버전이라 바이너리 다운로드를 건너뜁니다.");
  process.exit(0);
}

const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
const archMap = { x64: "amd64", arm64: "arm64" };
const goos = osMap[process.platform];
const goarch = archMap[process.arch];

if (!goos || !goarch || (goos === "windows" && goarch === "arm64")) {
  fail(
    `지원하지 않는 플랫폼입니다: ${process.platform}/${process.arch}. ` +
      `Homebrew·Scoop 또는 GitHub Release 바이너리를 사용하세요: ${RELEASES}`,
  );
}

const ext = goos === "windows" ? "zip" : "tar.gz";
const assetName = `naeryeo_${version}_${goos}_${goarch}.${ext}`;
const binName = goos === "windows" ? "naeryeo.exe" : "naeryeo";
const base = `${RELEASES}/download/v${version}`;

async function download(url) {
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) fail(`다운로드 실패 (HTTP ${res.status}): ${url}`);
  return Buffer.from(await res.arrayBuffer());
}

async function main() {
  console.error(`[naeryeo] ${assetName} 다운로드 중...`);
  const [archive, checksumsTxt] = await Promise.all([
    download(`${base}/${assetName}`),
    download(`${base}/checksums.txt`).then((b) => b.toString("utf8")),
  ]);

  const want = checksumsTxt
    .split("\n")
    .map((line) => line.trim().split(/\s+/))
    .find((parts) => parts[1] === assetName)?.[0];
  if (!want) fail(`checksums.txt에서 ${assetName} 항목을 찾을 수 없습니다.`);
  const got = createHash("sha256").update(archive).digest("hex");
  if (got !== want) fail(`체크섬 불일치 (${assetName}): expected ${want}, got ${got}`);

  const tmp = mkdtempSync(join(tmpdir(), "naeryeo-"));
  try {
    writeFileSync(join(tmp, assetName), archive);

    // Run tar with cwd=tmp and a relative archive name so no drive-letter path
    // is ever passed as an argument (avoids GNU tar's host:path heuristic).
    const res = spawnSync(resolveTar(), ["-xf", assetName], { cwd: tmp, stdio: "inherit" });
    if (res.error || res.status !== 0) {
      const detail = res.error ? res.error.message : `exit ${res.status}`;
      fail(
        `아카이브 추출 실패(${detail}). 시스템 tar가 필요합니다 ` +
          "(macOS·Linux 기본 제공, Windows는 10 1803 이상 기본 제공).",
      );
    }

    const extracted = join(tmp, binName);
    if (!existsSync(extracted)) fail(`아카이브에 ${binName}이(가) 없습니다.`);

    const binDir = join(pkgRoot, "bin");
    mkdirSync(binDir, { recursive: true });
    const dest = join(binDir, binName);
    copyFileSync(extracted, dest); // copy (not rename) — tmpdir may be a different filesystem
    if (goos !== "windows") chmodSync(dest, 0o755);

    console.error(`[naeryeo] 설치 완료: ${dest}`);
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}

main().catch((err) => fail(err && err.message ? err.message : String(err)));
