"use strict";

const fs = require("node:fs");
const path = require("node:path");

const FINGERPRINT_PATTERN = /^[0-9a-f]{12}$/;

function verifyBuildFingerprint(binaryPath, expected) {
  const fingerprint = String(expected || "").trim();
  if (!FINGERPRINT_PATTERN.test(fingerprint)) {
    throw new Error(`invalid expected source fingerprint: ${fingerprint || "<empty>"}`);
  }
  const resolved = path.resolve(binaryPath || "");
  const binary = fs.readFileSync(resolved);
  if (!binary.includes(Buffer.from(fingerprint, "ascii"))) {
    throw new Error(`backend build fingerprint mismatch: ${fingerprint} is not embedded in ${resolved}`);
  }
  return fingerprint;
}

if (require.main === module) {
  const [, , binaryPath, expected] = process.argv;
  if (!binaryPath || !expected) {
    console.error("usage: node verify-build-fingerprint.cjs <backend-binary> <expected-source-fingerprint>");
    process.exitCode = 2;
  } else {
    try {
      const fingerprint = verifyBuildFingerprint(binaryPath, expected);
      console.log(`verified build fingerprint=${fingerprint}`);
    } catch (error) {
      console.error(error.message);
      process.exitCode = 1;
    }
  }
}

module.exports = { verifyBuildFingerprint };
