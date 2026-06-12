#!/usr/bin/env node

const fs = require("fs");
const os = require("os");
const path = require("path");
const { execFileSync } = require("child_process");

const version = require("../package.json").version;
const platformMap = { darwin: "Darwin", linux: "Linux", win32: "Windows" };
const archMap = { x64: "x86_64", arm64: "arm64" };
const platform = platformMap[process.platform];
const arch = archMap[process.arch];

if (!platform || !arch) {
  console.error(`Unsupported platform: ${process.platform}-${process.arch}`);
  process.exit(1);
}

const ext = process.platform === "win32" ? ".zip" : ".tar.gz";
const binExt = process.platform === "win32" ? ".exe" : "";
const archiveName = `captainbi-cli_${version}_${platform}_${arch}${ext}`;
const base = process.env.CAPTAINBI_CLI_DOWNLOAD_BASE || "https://github.com/kirkzwy/captainbi-cli/releases/download";
const url = `${base}/v${version}/${archiveName}`;
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "captainbi-cli-"));
const archive = path.join(tmp, archiveName);
const outDir = path.join(__dirname, "bin");
const dest = path.join(outDir, `cbi${binExt}`);

try {
  fs.mkdirSync(outDir, { recursive: true });
  execFileSync("curl", ["--fail", "--location", "--silent", "--show-error", "--output", archive, url], { stdio: "inherit" });
  if (process.platform === "win32") {
    execFileSync("powershell", ["-Command", `Expand-Archive -Path '${archive}' -DestinationPath '${tmp}'`], { stdio: "inherit" });
  } else {
    execFileSync("tar", ["-xzf", archive, "-C", tmp], { stdio: "inherit" });
  }
  const found = findBinary(tmp, `cbi${binExt}`);
  fs.copyFileSync(found, dest);
  fs.chmodSync(dest, 0o755);
  console.log(`captainbi-cli v${version} installed`);
} catch (err) {
  console.error(`Failed to install cbi: ${err.message}`);
  console.error("For local development, run `go build -o npm/bin/cbi .` before using the npm wrapper.");
  process.exit(1);
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}

function findBinary(dir, name) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      try {
        return findBinary(p, name);
      } catch (_) {}
    } else if (entry.name === name || entry.name.startsWith("cbi")) {
      return p;
    }
  }
  throw new Error("extracted binary not found");
}
