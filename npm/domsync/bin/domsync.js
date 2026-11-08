#!/usr/bin/env node
const { spawnSync } = require("node:child_process");
const path = require("node:path");

const map = {
  "darwin-arm64": "@neiromaster/domsync-darwin-arm64",
  "darwin-x64": "@neiromaster/domsync-darwin-x64",
  "linux-arm64": "@neiromaster/domsync-linux-arm64",
  "linux-x64": "@neiromaster/domsync-linux-x64",
  "win32-arm64": "@neiromaster/domsync-win32-arm64",
  "win32-x64": "@neiromaster/domsync-win32-x64",
};
const pkg = map[`${process.platform}-${process.arch}`];
if (!pkg) {
  console.error(`domsync: unsupported platform ${process.platform}-${process.arch}`);
  process.exit(1);
}
let bin;
try {
  const dir = path.dirname(require.resolve(`${pkg}/package.json`));
  bin = path.join(dir, process.platform === "win32" ? "domsync.exe" : "domsync");
} catch {
  console.error(`domsync: missing optional dependency ${pkg}; reinstall without --omit=optional`);
  process.exit(1);
}
const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`domsync: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status ?? 1);
