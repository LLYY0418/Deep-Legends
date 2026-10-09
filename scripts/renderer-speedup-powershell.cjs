"use strict";
const fs = require("node:fs"), path = require("node:path"), assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
module.exports = function runBatch(engine, harness, files, directory, env) {
  const manifest = path.join(directory, "script-paths.json"), output = path.join(directory, "batch-results.json");
  fs.writeFileSync(manifest, JSON.stringify(files));
  const result = spawnSync(engine, ["-NoProfile", "-NonInteractive", "-File", harness, "-ScriptPathsFile", manifest, "-BatchOutputFile", output],
    { encoding: "utf8", timeout: 45000, env });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  const rows = JSON.parse(fs.readFileSync(output, "utf8").replace(/^\uFEFF/, ""));
  assert.equal(rows.length, files.length, "every original variant must finish");
  return rows;
};
