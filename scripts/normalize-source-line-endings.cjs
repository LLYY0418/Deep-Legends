"use strict";

const fs = require("node:fs");
const path = require("node:path");

// Limit writes to build source text; never traverse dependencies, generated
// artifacts or symlinks. Byte replacement preserves UTF-8 BOMs and binary assets.
const sourceDirs = new Set(["backend", "desktop", "installer", "scripts", "tools"]);
const excludedDirs = new Set(["node_modules", "vendor", "dist", "files", "_shots"]);
const textExtension = /\.(go|mod|sum|js|cjs|mjs|json|css|html|manifest|ps1|sh|nsi|nsh|txt)$/i;
function normalizeSourceLineEndings(root) {
  let changed = 0;
  function visit(directory, top = false) {
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      if (entry.name.startsWith(".") || entry.isSymbolicLink()) continue;
      const file = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        if (!excludedDirs.has(entry.name) && (!top || sourceDirs.has(entry.name))) visit(file);
      } else if (entry.isFile() && textExtension.test(entry.name) && !entry.name.startsWith("riot_key.local")) {
        const bytes = fs.readFileSync(file);
        if (bytes.includes(0) || !bytes.includes(Buffer.from("\r\n"))) continue;
        const normalized = Buffer.from(bytes.toString("latin1").replace(/\r\n/g, "\n"), "latin1");
        fs.writeFileSync(file, normalized);
        changed++;
      }
    }
  }
  visit(root, true);
  return changed;
}
module.exports = { normalizeSourceLineEndings };
if (require.main === module) {
  const count = normalizeSourceLineEndings(path.resolve(__dirname, ".."));
  console.log(`Source line endings: normalized ${count} files to LF.`);
}
