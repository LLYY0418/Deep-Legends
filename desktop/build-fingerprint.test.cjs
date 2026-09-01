"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { verifyBuildFingerprint } = require("./verify-build-fingerprint.cjs");

test("built backend must contain the exact source fingerprint", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-fingerprint-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const binary = path.join(root, "loot-service.exe");
  fs.writeFileSync(binary, Buffer.concat([Buffer.from([0, 1, 2]), Buffer.from("a1b2c3d4e5f6"), Buffer.from([3, 4, 5])]));
  assert.equal(verifyBuildFingerprint(binary, "a1b2c3d4e5f6"), "a1b2c3d4e5f6");
  assert.throws(() => verifyBuildFingerprint(binary, "000000000000"), /build fingerprint mismatch/);
  assert.throws(() => verifyBuildFingerprint(binary, "dev"), /invalid expected source fingerprint/);
});
