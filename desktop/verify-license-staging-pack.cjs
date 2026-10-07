"use strict";
// Only the separate staging packer selects this hook in an isolated snapshot.
const fs = require("node:fs"), path = require("node:path");
function verifyStagingBackend(file, root = path.resolve(__dirname, "..")) {
  const binary = fs.readFileSync(file);
  const publicKey = "E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE";
  for (const marker of ["https://license-staging.yinxiaobia.net", "staging-2026-10", publicKey]) {
    if (!binary.includes(Buffer.from(marker))) throw Error("Staging backend is missing its owner-supplied origin or key");
  }
  const vectors = JSON.parse(fs.readFileSync(path.join(root, "backend/testdata/license-protocol-vectors.json"), "utf8"));
  const forbidden = ["test-issuer", "test-update", "licenseTestIssuer", "-----BEGIN PRIVATE KEY-----", "-----BEGIN ED25519 PRIVATE KEY-----"];
  const markers = forbidden.map(value => Buffer.from(value));
  for (const [kid, pub] of Object.entries(vectors.public_keys)) markers.push(Buffer.from(kid), Buffer.from(pub), Buffer.from(pub, "base64url"));
  markers.push(Buffer.from(vectors.device_seed_hex), Buffer.from(vectors.device_seed_hex, "hex"));
  if (markers.some(marker => marker.length && binary.includes(marker))) throw Error("Staging backend contains private/test trust material");
  require("./verify-embedded-riot-key.cjs").verifyRiotKeyPolicy(file, "public");
  return true;
}
module.exports = async function beforeStagingPack() {
  const backend = path.join(__dirname, "backend/loot-service.exe");
  verifyStagingBackend(backend);
  if (!require("./app-title.cjs").endsWith("-STAGING")) throw Error("Staging window title is missing its label");
  require("./generate-backend-digest.cjs").generateBackendDigest();
  require("./apply-update-timing-template.cjs").applyUpdateTiming();
  console.log(`STAGING setup compression: 7z level ${require("./verify-embedded-riot-key.cjs").configureSetupCompression()}`);
};
module.exports.verifyStagingBackend = verifyStagingBackend;
