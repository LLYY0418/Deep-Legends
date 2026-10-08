"use strict";
const fs = require("node:fs"), path = require("node:path"), crypto = require("node:crypto"), assert = require("node:assert/strict");
const root = path.resolve(__dirname, "../../../.."), binary = "/private/tmp/R252-0.12.77-no-license-public-02/macos/loot-service-public";
require(path.join(root, "desktop/verify-license-release.cjs")).verifyLicenseRelease(binary);
require(path.join(root, "desktop/verify-embedded-riot-key.cjs")).verifyRiotKeyPolicy(binary, "public");
require(path.join(root, "desktop/verify-build-fingerprint.cjs")).verifyBuildFingerprint(binary, "f2794a1d683c");
const asar = require(path.join(root, "desktop/node_modules/@electron/asar")), archive = "/private/tmp/R252-0.12.77-no-license-public-02/win-unpacked/resources/app.asar";
const strings = ["注册码", "授权到期", "DL-XXXXX", "license.yinxiaobia.net", "license-staging.yinxiaobia.net"];
const entries = asar.listPackage(archive).filter(e => /\.(?:cjs|js|html)$/.test(e));
for (const entry of entries) {
  const bytes = asar.extractFile(archive, entry.replace(/^\//, ""));
  for (const value of strings) for (const encoding of ["utf8", "utf16le"]) assert.ok(!bytes.includes(Buffer.from(value, encoding)), `${entry}: ${value}/${encoding}`);
}
const result = {version: "0.12.77", mode: "public", fingerprint: "f2794a1d683c", binary, sha256: crypto.createHash("sha256").update(fs.readFileSync(binary)).digest("hex"), license_absent: true, key_policy: true, actual_asar_script_and_html_entries: entries.length, asar_license_text_absent: true, execution_platform: process.platform, windows_execution: "未在 Windows 实跑"};
fs.writeFileSync(path.join(__dirname, "macos-public-audit-02.json"), JSON.stringify(result, null, 2) + "\n");
console.log(JSON.stringify(result));
