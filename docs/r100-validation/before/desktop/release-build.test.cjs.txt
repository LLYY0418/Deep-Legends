"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path"), crypto = require("node:crypto");
const { verifyRiotKeyPolicy } = require("./verify-embedded-riot-key.cjs");
const { recordReleaseBuild } = require("./release-build.cjs");

// Only synthetic credentials are used; never read the developer's local key.
function syntheticCipher() {
  const key = crypto.createHash("sha256").update(["deep", "legends", "hexcore", "loot", "kr-riot-channel", "v1"].join("\x1f")).digest();
  const nonce = Buffer.alloc(12, 7), cipher = crypto.createCipheriv("aes-256-gcm", key, nonce);
  return Buffer.concat([nonce, cipher.update("synthetic-test-key"), cipher.final(), cipher.getAuthTag()]).toString("base64");
}
test("public policy rejects encrypted and plaintext keys; private requires a valid key", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "r81-key-policy-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const file = path.join(root, "backend.exe");
  fs.writeFileSync(file, "MZ\0no credentials\0");
  assert.equal(verifyRiotKeyPolicy(file, "public"), true);
  assert.throws(() => verifyRiotKeyPolicy(file, "private"), /ciphertext was not found/);
  assert.throws(() => verifyRiotKeyPolicy(file, "typo"), /Key mode/);
  fs.writeFileSync(file, `MZ\0${syntheticCipher()}\0`);
  assert.equal(verifyRiotKeyPolicy(file, "private"), true);
  assert.throws(() => verifyRiotKeyPolicy(file, "public"), /contains an embedded/);
  fs.writeFileSync(file, "MZ\0RGAPI-00000000-0000-0000-0000-000000000000\0");
  assert.throws(() => verifyRiotKeyPolicy(file, "public"), /contains an embedded/);
});

test("build receipt rejects a stale packaged backend and clears obsolete approval", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "r81-build-policy-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const fingerprint = "a1b2c3d4e5f6", directory = path.join(root, "dist", "desktop");
  const backend = path.join(root, "desktop", "backend", "loot-service.exe");
  const packaged = path.join(directory, "win-unpacked", "resources", "app.asar.unpacked", "backend", "loot-service.exe");
  for (const file of [backend, packaged]) { fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, `MZ\0${fingerprint}\0`); }
  fs.writeFileSync(path.join(directory, `Deep Legends 0.12.1.exe`), "portable");
  fs.writeFileSync(path.join(directory, `Deep Legends Setup 0.12.1.exe`), "installer");
  fs.writeFileSync(path.join(root, "desktop/package.json"), JSON.stringify({ version: "0.12.1" }));
  const receipt = recordReleaseBuild({ root, fingerprint, mode: "public" });
  assert.deepEqual(Object.keys(receipt.assets), ["Deep Legends Setup 0.12.1.exe"], "stale portable output must not enter the receipt");
  fs.rmSync(path.join(directory, "Deep Legends 0.12.1.exe"));
  assert.deepEqual(recordReleaseBuild({ root, fingerprint, mode: "public" }), receipt, "setup alone is sufficient");
  fs.rmSync(path.join(directory, "Deep Legends Setup 0.12.1.exe"));
  assert.throws(() => recordReleaseBuild({ root, fingerprint, mode: "public" }), /Setup build is missing/);
  assert.equal(fs.existsSync(path.join(directory, "release-build.json")), false);
  fs.writeFileSync(path.join(directory, "Deep Legends Setup 0.12.1.exe"), "");
  assert.throws(() => recordReleaseBuild({ root, fingerprint, mode: "public" }), /Empty build artifact/);
  fs.writeFileSync(path.join(directory, "Deep Legends Setup 0.12.1.exe"), "installer");
  recordReleaseBuild({ root, fingerprint, mode: "public" });
  fs.appendFileSync(packaged, "stale");
  assert.throws(() => recordReleaseBuild({ root, fingerprint, mode: "public" }), /Packaged backend differs/);
  assert.equal(fs.existsSync(path.join(directory, "release-build.json")), false);
  fs.writeFileSync(packaged, `MZ\0${fingerprint}\0${syntheticCipher()}\0`);
  assert.throws(() => recordReleaseBuild({ root, fingerprint, mode: "public" }), /contains an embedded/);
});
