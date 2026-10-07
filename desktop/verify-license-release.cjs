"use strict";
// Build-time checks only: this module is deliberately absent from build.files.
const fs = require("node:fs"), path = require("node:path");
function testTrustMarkers(root) {
  const markers = ["test-issuer", "test-update", "licenseTestIssuer"];
  const staging = path.join(root, "backend/license_config_staging.go");
  if (fs.existsSync(staging)) {
    for (const value of fs.readFileSync(staging,"utf8").matchAll(/"([A-Za-z0-9_-]{43})"/g)) markers.push(value[1]);
  }
  const fixture = path.join(root, "backend/testdata/license-protocol-vectors.json");
  if (fs.existsSync(fixture)) {
    const vectors = JSON.parse(fs.readFileSync(fixture,"utf8"));
    for (const [kid, pub] of Object.entries(vectors.public_keys || {})) markers.push(kid, pub);
    if (/^[a-f0-9]{64}$/.test(vectors.device_seed_hex || "")) markers.push(vectors.device_seed_hex);
  }
  return markers.flatMap(value => [Buffer.from(value), ...(/^[A-Za-z0-9_-]{43}$/.test(value) ? [Buffer.from(value,"base64url")] : []), ...(/^[a-f0-9]{64}$/.test(value) ? [Buffer.from(value,"hex")] : [])]);
}
function verifyLicenseRelease(file, root = path.resolve(__dirname,".."), enabled = false) {
  const binary = fs.readFileSync(file);
  if (/(?:license|manage)-staging\./i.test(binary.toString("latin1"))) throw Error("Release backend contains staging license origin");
  for (const marker of testTrustMarkers(root)) {
    if (marker.length && binary.includes(marker)) throw Error("Release backend contains a test trust marker");
  }
  if (!enabled) {
    const flag = path.join(root,"desktop/license-build.cjs");
    if (fs.existsSync(flag) && require(flag).enabled !== false) throw Error("Release desktop must disable license build");
    if (/license(?:-staging)?\.yinxiaobia\.net/i.test(binary.toString("latin1"))) throw Error("Disabled backend contains license origin");
    for (const name of ["license_config_release.go", "license_config_staging.go", "license_privacy.go"]) {
      const file = path.join(root, "backend", name);
      if (!fs.existsSync(file)) continue;
      const source = fs.readFileSync(file, "utf8");
      for (const match of source.matchAll(/"([^"\n]+)"/g)) {
        const value = match[1];
        if (!(/^[A-Za-z0-9_-]{43}$/.test(value) || /^(?:release|staging)-20/.test(value) || value.startsWith("激活时"))) continue;
        const markers = [Buffer.from(value)];
        if (/^[A-Za-z0-9_-]{43}$/.test(value)) markers.push(Buffer.from(value,"base64url"));
        if (markers.some(marker => binary.includes(marker))) throw Error("Disabled backend contains license kid, public key or privacy disclosure");
      }
    }
  }
  const title = path.join(root,"desktop/app-title.cjs");
  if (fs.existsSync(title) && require(title) !== "Deep Legends") throw Error("Release title must not use a staging label");
  return true;
}
module.exports = { verifyLicenseRelease, testTrustMarkers };
if (require.main === module) {
  try { verifyLicenseRelease(process.argv[2]); console.log("Release license isolation verified"); }
  catch(error) { console.error(error.message); process.exitCode=1; }
}
