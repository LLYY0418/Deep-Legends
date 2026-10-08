"use strict";
// Local research inputs are opt-in and never required by product or CI tests.
const fs = require("node:fs"), path = require("node:path");
const evidenceRoot = path.resolve(process.env.DEEP_LEGENDS_EVIDENCE_ROOT || path.join(__dirname, "..", "output", "evidence"));
function evidencePath(...parts) { return path.join(evidenceRoot, ...parts); }
function requireEvidence(...parts) {
  for (const relative of parts) {
    const file = evidencePath(relative);
    if (!fs.existsSync(file)) throw new Error(`Missing local evidence: ${file}. Set DEEP_LEGENDS_EVIDENCE_ROOT to your existing report directory.`);
  }
}
module.exports = { evidencePath, requireEvidence };
