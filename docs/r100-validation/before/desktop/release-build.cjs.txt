"use strict";

const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { verifyRiotKeyPolicy } = require("./verify-embedded-riot-key.cjs");
const { verifyBuildFingerprint } = require("./verify-build-fingerprint.cjs");
const sha256 = (bytes) => crypto.createHash("sha256").update(bytes).digest("hex");

// Record the exact outputs of a completed local build. This file stays in
// dist/desktop for local A/B attribution; the release manifest uses its setup hash.
function recordReleaseBuild({ root = path.resolve(__dirname, ".."), fingerprint, mode = process.env.DEEP_LEGENDS_KEY_MODE || "private" } = {}) {
  const directory = path.join(root, "dist", "desktop");
  const receiptPath = path.join(directory, "release-build.json");
  fs.rmSync(receiptPath, { force: true });
  const backend = path.join(root, "desktop", "backend", "loot-service.exe");
  const packaged = path.join(directory, "win-unpacked", "resources", "app.asar.unpacked", "backend", "loot-service.exe");
  verifyBuildFingerprint(backend, fingerprint);
  verifyRiotKeyPolicy(backend, mode);
  verifyRiotKeyPolicy(packaged, mode);
  const backendHash = sha256(fs.readFileSync(backend));
  if (sha256(fs.readFileSync(packaged)) !== backendHash) throw new Error("Packaged backend differs from the verified build");
  const version = JSON.parse(fs.readFileSync(path.join(root, "desktop/package.json"), "utf8")).version;
  if (!/^\d+\.\d+\.\d+(?:[-+][\dA-Za-z.-]+)?$/.test(version)) throw new Error("Invalid build version");
  const names = [`Deep Legends Setup ${version}.exe`];
  const assets = {};
  for (const name of names) {
    const file = path.join(directory, name);
    if (!fs.existsSync(file)) throw new Error(`Setup build is missing: ${name}`);
    const bytes = fs.readFileSync(file);
    if (!bytes.length) throw new Error(`Empty build artifact: ${name}`);
    assets[name] = sha256(bytes);
  }
  const receipt = { schema: 1, version, fingerprint, mode, backendSHA256: backendHash, assets };
  fs.writeFileSync(receiptPath, JSON.stringify(receipt, null, 2) + "\n");
  return receipt;
}

function verifyPublicReleaseBuild(root, fingerprint, assets) {
  const receiptPath = path.join(root, "dist", "desktop", "release-build.json");
  if (!fs.existsSync(receiptPath)) throw new Error("缺少构建验证记录，请先完整执行 public 构建");
  const receipt = JSON.parse(fs.readFileSync(receiptPath, "utf8"));
  if (receipt.schema !== 1 || receipt.mode !== "public" || receipt.fingerprint !== fingerprint) {
    throw new Error("公开发布只接受当前指纹的 public 构建，禁止发布含个人 Key 的 private 包");
  }
  const backend = path.join(root, "desktop", "backend", "loot-service.exe");
  verifyBuildFingerprint(backend, fingerprint);
  verifyRiotKeyPolicy(backend, "public");
  if (sha256(fs.readFileSync(backend)) !== receipt.backendSHA256) throw new Error("Backend changed after build verification");
  for (const { input, bytes } of assets) {
    if (receipt.assets?.[input] !== sha256(bytes)) throw new Error(`发布文件与已验证构建不一致：${input}`);
  }
}

module.exports = { recordReleaseBuild, verifyPublicReleaseBuild };
if (require.main === module) {
  try {
    const receipt = recordReleaseBuild({ fingerprint: process.argv[2] });
    console.log(`Build distribution verified: ${receipt.mode} · ${receipt.fingerprint}`);
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
