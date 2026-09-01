"use strict";

// electron-builder invokes this hook before packaging. The Windows backend
// cannot be executed from a macOS build host, so validate the embedded GCM
// ciphertext directly without ever printing or persisting the plaintext key.
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const backendPath = path.join(__dirname, "backend", "loot-service.exe");
const keySeed = Buffer.from(["deep", "legends", "hexcore", "loot", "kr-riot-channel", "v1"].join("\x1f"), "utf8");
const cipherKey = crypto.createHash("sha256").update(keySeed).digest();
const base64Pattern = /[A-Za-z0-9+/]{48,}={0,2}/g;
const templatePath = path.join(__dirname, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi");
const { buildFingerprint } = require("./apply-portable-template.cjs");

function validCiphertext(encoded) {
  let raw;
  try {
    raw = Buffer.from(encoded, "base64");
  } catch (_) {
    return false;
  }
  if (raw.length <= 12) return false;
  try {
    const decipher = crypto.createDecipheriv("aes-256-gcm", cipherKey, raw.subarray(0, 12));
    decipher.setAuthTag(raw.subarray(raw.length - 16));
    const plain = Buffer.concat([decipher.update(raw.subarray(12, raw.length - 16)), decipher.final()]).toString("utf8").trim();
    return plain.length > 0;
  } catch (_) {
    return false;
  }
}

function verifyEmbeddedRiotKey() {
  if (!fs.existsSync(backendPath)) throw new Error(`embedded backend is missing: ${backendPath}`);
  const binary = fs.readFileSync(backendPath).toString("latin1");
  const candidates = binary.match(base64Pattern) || [];
  if (!candidates.some(validCiphertext)) {
    throw new Error("embedded Riot API key ciphertext was not found or could not be decrypted");
  }
  return true;
}

function verifyPortableTemplate() {
  if (!fs.existsSync(templatePath)) throw new Error(`electron-builder portable 模板缺失：${templatePath}`);
  const script = fs.readFileSync(templatePath, "utf8");
  if (script.includes("@@BUILD_FINGERPRINT@@")) throw new Error("portable 模板仍包含未替换的构建指纹占位符");
  const match = script.match(/DEEP_LEGENDS_BUILD_FINGERPRINT\s+"([0-9a-f]{12})"/i);
  if (!match) throw new Error("portable 模板缺少 12 位构建指纹");
  const expected = buildFingerprint();
  if (match[1].toLowerCase() !== expected) throw new Error(`portable 模板指纹过期：${match[1]} != ${expected}`);
  return true;
}

const beforePack = async function beforePack() {
  verifyEmbeddedRiotKey();
  verifyPortableTemplate();
};
beforePack.verifyEmbeddedRiotKey = verifyEmbeddedRiotKey;
beforePack.verifyPortableTemplate = verifyPortableTemplate;
module.exports = beforePack;

if (require.main === module) {
  verifyEmbeddedRiotKey();
  verifyPortableTemplate();
  console.log("embedded Riot API key ciphertext verified");
}
