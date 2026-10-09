"use strict";

// Each mutation must make scripts/renderer-budget-gate.test.cjs fail.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, "..");
const gate = fs.readFileSync(path.join(__dirname, "renderer-budget-gate.cjs"), "utf8");
const mutations = [
  ["raw time instead of reference speed", "normalized = raw / factor;", "normalized = raw;"],
  ["factor inverted", "/ 2 / reference.reference_ms;", "/ 2 / reference.reference_ms; factor = 1 / factor;"],
  ["absolute 240s budget dropped", "else if (raw > 240000)", "else if (false)"],
  ["checksum not checked", "if (calibration?.checksum !== reference?.checksum)", "if (false)"],
  ["iterations not checked", "if (calibration?.iterations !== reference?.iterations)", "if (false)"],
  ["only one calibration required", "calibrations.length !== 2", "calibrations.length < 1"],
  ["failed suite accepted", "if (!timings || timings.success !== true)", "if (!timings)"],
  ["margin widened", "if (normalized > 192000)", "if (normalized > 240000)"],
];
let failures = 0;
for (const [name, from, to] of mutations) {
  if (!gate.includes(from)) { console.error(`mutation anchor missing: ${name}`); failures++; continue; }
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "gate-mutant-"));
  fs.mkdirSync(path.join(dir, "scripts"));
  for (const file of ["renderer-budget-gate.test.cjs", "runner-calibration.cjs", "renderer-calibration-reference.json"]) fs.copyFileSync(path.join(__dirname, file), path.join(dir, "scripts", file));
  fs.writeFileSync(path.join(dir, "scripts", "renderer-budget-gate.cjs"), gate.replace(from, to));
  fs.mkdirSync(path.join(dir, ".github", "workflows"), { recursive: true });
  fs.copyFileSync(path.join(root, ".github", "workflows", "ci.yml"), path.join(dir, ".github", "workflows", "ci.yml"));
  fs.symlinkSync(path.join(root, "desktop"), path.join(dir, "desktop"));
  const result = spawnSync(process.execPath, ["--test", "--test-name-pattern=^(?!calibration workers)", path.join(dir, "scripts", "renderer-budget-gate.test.cjs")], { encoding: "utf8", timeout: 120000 });
  const killed = result.status !== 0;
  console.log(`${killed ? "KILLED" : "SURVIVED"} ${name}`);
  if (!killed) failures++;
  fs.rmSync(dir, { recursive: true, force: true });
}
process.exitCode = failures ? 1 : 0;
