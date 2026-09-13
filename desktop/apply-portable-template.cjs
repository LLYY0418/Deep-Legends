"use strict";

// electron-builder reads its portable NSIS template from node_modules. Apply
// the checked-in template after dependency installation and stamp it with the
// current backend/shell fingerprint so stale portable caches cannot be reused.
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

// Tests supply an isolated desktop root. They must never temporarily overwrite
// the real backend or builder template while a user may be packaging it.
function buildFingerprint(desktopRoot = __dirname) {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(path.join(desktopRoot, "backend", "loot-service.exe")));
  for (const name of ["main.cjs", "preload.cjs", "proxy-resolution.cjs"]) {
    hash.update(fs.readFileSync(path.join(desktopRoot, name)));
  }
  return hash.digest("hex").slice(0, 12);
}

function applyPortableTemplate(desktopRoot = __dirname) {
  const source = path.join(desktopRoot, "nsis", "portable.nsi");
  const target = path.join(desktopRoot, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi");
  const backend = path.join(desktopRoot, "backend", "loot-service.exe");
  if (!fs.existsSync(source)) throw new Error(`定制 portable 模板缺失：${source}`);
  if (!fs.existsSync(target)) throw new Error(`electron-builder 模板位置不存在（是否漏了 npm ci？）：${target}`);
  if (!fs.existsSync(backend)) throw new Error(`后端二进制缺失，请先构建 Go：${backend}`);
  const fingerprint = buildFingerprint(desktopRoot);
  const script = fs.readFileSync(source, "utf8");
  if (!script.includes("@@BUILD_FINGERPRINT@@")) throw new Error("定制模板缺少 @@BUILD_FINGERPRINT@@ 占位符");
  fs.writeFileSync(target, script.replaceAll("@@BUILD_FINGERPRINT@@", fingerprint), "utf8");
  return fingerprint;
}

module.exports = { applyPortableTemplate, buildFingerprint };

if (require.main === module) {
  process.stdout.write(`portable template applied, fingerprint=${applyPortableTemplate()}\n`);
}
