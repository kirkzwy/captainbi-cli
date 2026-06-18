#!/usr/bin/env node

const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const ext = process.platform === "win32" ? ".exe" : "";
const bin = path.join(__dirname, "bin", "cbi" + ext);

if (!fs.existsSync(bin)) {
  console.error(`cbi binary not found at ${bin}. Run: node ${path.join(__dirname, "install.js")}`);
  process.exit(1);
}

try {
  execFileSync(bin, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  process.exit(err.status || 1);
}
