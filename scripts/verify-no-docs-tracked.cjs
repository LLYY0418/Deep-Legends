"use strict";
const fs = require("node:fs"), path = require("node:path");
const { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, "..");
const policyFiles = new Set(["scripts/verify-no-docs-tracked.cjs", "scripts/verify-no-docs-tracked.test.cjs"]);
// This optional local snapshot copy is the only allowed file operation on notes.
// The exact statement excludes nested repository metadata and tolerates absence.
const optionalCopy = 'if(fs.existsSync(path.join(root,"docs")))fs.cpSync(path.join(root,"docs"),path.join(snapshot,"docs"),{recursive:true,filter:file=>!file.split(path.sep).includes(".git")});';
function scanSource(file, source) {
  const violations = [];
  const lines = source.split(/\r?\n/);
  let index = 0, line = 1;
  let previousLiteral = "", previousEnd = -1, previousLine = 1;
  const python = file.endsWith(".py");
  while (index < source.length) {
    const start = index, current = source[index];
    if ((!python && source.startsWith("//", index)) || (python && current === "#")) {
      while (index < source.length && source[index] !== "\n") index++;
    } else if (!python && source.startsWith("/*", index)) {
      index = source.indexOf("*/", index + 2);
      index = index < 0 ? source.length : index + 2;
    } else if (["'", '"', "`"].includes(current)) {
      const delimiter = python && source.startsWith(current.repeat(3), index) ? current.repeat(3) : current;
      const at = line;
      index += delimiter.length;
      let value = "";
      while (index < source.length && !source.startsWith(delimiter, index)) {
        if (source[index] === "\\" && delimiter !== "`") { value += source.slice(index, index + 2); index += 2; }
        else value += source[index++];
      }
      index += delimiter.length;
      value = value.replace(/\\u\{([0-9a-f]{1,6})\}/gi, (_, hex) => String.fromCodePoint(parseInt(hex, 16)))
        .replace(/\\U([0-9a-f]{8})/g, (_, hex) => String.fromCodePoint(parseInt(hex, 16)))
        .replace(/\\u([0-9a-f]{4})/gi, (_, hex) => String.fromCharCode(parseInt(hex, 16)))
        .replace(/\\x([0-9a-f]{2})/gi, (_, hex) => String.fromCharCode(parseInt(hex, 16)))
        .replace(/\\\\/g, "/").replace(/\\\//g, "/");
      const joined = previousEnd >= 0 && /^\s*\+\s*$/.test(source.slice(previousEnd, start));
      if (joined) value = previousLiteral + value;
      const literalLine = joined ? previousLine : at;
      previousLiteral = value; previousEnd = index; previousLine = literalLine;
      const statement = lines[at - 1]?.trim();
      const permitted = file === "scripts/build-no-license-audit.cjs" && statement === optionalCopy;
      if (/(?:^|\/)docs(?:\/|$)/.test(value) && !permitted) {
        violations.push({ file, line: literalLine, message: "source path depends on local notes", value });
      }
    } else index++;
    line += (source.slice(start, index).match(/\n/g) || []).length;
  }
  return violations;
}
function verify(directory = root) {
  const listing = spawnSync("git", ["ls-files", "-z"], { cwd: directory, encoding: "utf8" });
  if (listing.error || listing.status !== 0) throw new Error(listing.error?.message || listing.stderr || "Cannot list tracked files");
  const files = listing.stdout.split("\0").filter(Boolean), violations = [];
  for (const file of files) {
    if (file === "docs" || file.startsWith("docs/") || ["CLAUDE.md", "AGENTS.md"].includes(file)) {
      violations.push({ file, line: 1, message: "local notes or assistant instructions are tracked" });
    }
    if (/\.(?:go|cjs|js|py)$/.test(file) && !policyFiles.has(file)) {
      // Missing tracked sources are also errors; staged deletions are absent from ls-files.
      violations.push(...scanSource(file, fs.readFileSync(path.join(directory, file), "utf8")));
    }
  }
  return violations;
}
module.exports = { scanSource, verify };
if (require.main === module) {
  try {
    const violations = verify(process.argv[2] || root);
    for (const item of violations) console.error(`${item.file}:${item.line}: ${item.message}${item.value ? ` (${item.value})` : ""}`);
    if (violations.length) process.exitCode = 1;
    else console.log("Tracked source is independent of local notes and assistant instructions.");
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
