"use strict";
const fs = require("node:fs"), path = require("node:path"), assert = require("node:assert/strict"), crypto = require("node:crypto");
const root = path.resolve(__dirname, "../../../.."), title = path.join(root, "desktop/app-title.cjs"), receipt = require(path.join(root, "dist/R243-staging-public/staging-build.json"));
const read = fs.readFileSync, inputs = [];
fs.readFileSync = function(file, options) {
  const absolute = path.resolve(file), override = absolute === title;
  const value = override ? '"use strict";\nmodule.exports = "Deep Legends-STAGING";\n' : read(file, options);
  inputs.push({path:path.relative(root,absolute).replaceAll(path.sep,"/"),sha256:crypto.createHash("sha256").update(value).digest("hex"),allowed_staging_title_only:override});
  return value;
};
const fingerprint = require(path.join(root, "desktop/source-fingerprint.cjs")).sourceFingerprint(root);
fs.readFileSync = read;
assert.equal(fingerprint, receipt.fingerprint, "fingerprint must match checked source with build's sole STAGING title override");
const result = {fingerprint,checked_source_matches_build:true,staging_title_override:true,inputs};
fs.writeFileSync(path.join(__dirname,"fingerprint-inputs.json"),JSON.stringify(result,null,2)+"\n");
console.log(JSON.stringify({fingerprint,inputs:inputs.length,matched:true}));
