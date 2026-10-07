"use strict";
const fs = require("node:fs"), path = require("node:path");
const { backendHash } = require("./backend-integrity.cjs");
function generateBackendDigest(root = __dirname) {
  const digest = backendHash(path.join(root, "backend", "loot-service.exe"));
  fs.writeFileSync(path.join(root, "backend-digest.cjs"), `"use strict";\nmodule.exports = ${JSON.stringify(digest)};\n`);
  return digest;
}
module.exports = { generateBackendDigest };
if (require.main === module) generateBackendDigest();
