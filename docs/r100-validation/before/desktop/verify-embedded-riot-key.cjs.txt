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

function verifyRiotKeyPolicy(file = backendPath, mode = process.env.DEEP_LEGENDS_KEY_MODE || "private") {
  if (!["public", "private"].includes(mode)) throw new Error("Key mode must be public or private");
  if (!fs.existsSync(file)) throw new Error(`embedded backend is missing: ${file}`);
  const binary = fs.readFileSync(file).toString("latin1");
  const candidates = binary.match(base64Pattern) || [];
  const encrypted = candidates.some(validCiphertext);
  const plaintext = /RGAPI-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i.test(binary);
  if (mode === "public" && (encrypted || plaintext)) {
    throw new Error("Public build contains an embedded Riot API key; rebuild with public key mode");
  }
  if (mode === "private" && !encrypted) {
    throw new Error("embedded Riot API key ciphertext was not found or could not be decrypted");
  }
  return true;
}

function verifyEmbeddedRiotKey() { return verifyRiotKeyPolicy(backendPath, "private"); }

function verifyPortableTemplate(desktopRoot = __dirname) {
  const templatePath = path.join(desktopRoot, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi");
  if (!fs.existsSync(templatePath)) throw new Error(`electron-builder portable 模板缺失：${templatePath}`);
  const script = fs.readFileSync(templatePath, "utf8");
  if (script.includes("@@BUILD_FINGERPRINT@@")) throw new Error("portable 模板仍包含未替换的构建指纹占位符");
  const match = script.match(/DEEP_LEGENDS_BUILD_FINGERPRINT\s+"([0-9a-f]{12})"/i);
  if (!match) throw new Error("portable 模板缺少 12 位构建指纹");
  const expected = buildFingerprint(desktopRoot);
  if (match[1].toLowerCase() !== expected) throw new Error(`portable 模板指纹过期：${match[1]} != ${expected}`);
  return true;
}

// Prefer archive size as requested: keep level 9 and the NSIS-compatible BCJ
// decoder path. Explicit caller overrides remain available.
function configureSetupCompression(env = process.env) {
  const level = env.ELECTRON_BUILDER_COMPRESSION_LEVEL ?? "9";
  if (!/^[0-9]$/.test(level)) throw new Error("ELECTRON_BUILDER_COMPRESSION_LEVEL must be a single digit 0-9");
  env.ELECTRON_BUILDER_COMPRESSION_LEVEL = level;
  return level;
}

const beforePack = async function beforePack() {
  verifyRiotKeyPolicy();
  console.log(`Setup compression: 7z level ${configureSetupCompression()} (default 9)`);
};
beforePack.verifyEmbeddedRiotKey = verifyEmbeddedRiotKey;
beforePack.verifyRiotKeyPolicy = verifyRiotKeyPolicy;
beforePack.verifyPortableTemplate = verifyPortableTemplate;
beforePack.configureSetupCompression = configureSetupCompression;
module.exports = beforePack;

if (require.main === module) {
  verifyRiotKeyPolicy();
  console.log("Riot API key distribution policy verified");
}
