"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { evaluate } = require("./renderer-budget-gate.cjs");
const { calibrate, workload, loadJSDOM } = require("./runner-calibration.cjs");

const reference = { workload_version: 1, iterations: 40, concurrency: 5, checksum: 49440, reference_ms: 4000, margin_budget_ms: 192000, hard_budget_ms: 240000 };
const timings = duration => ({ success: true, scope: "all", platform: "linux", concurrency: 5, summary: { duration_ms: duration } });
const calibration = median => ({ workload_version: 1, iterations: 40, concurrency: 5, checksum: 49440, median_ms: median });

test("same commit on a 1.6x slower runner passes when it is within margin at reference speed", () => {
  // 209.4s raw on a runner whose calibration is 1.6x the reference: 130.9s at reference speed.
  const result = evaluate(timings(209400), [calibration(6400), calibration(6400)], reference);
  assert.equal(result.ok, true, result.failures.join("; "));
  assert.equal(Math.round(result.factor * 100), 160);
  assert.equal(Math.round(result.normalized_ms), 130875);
});

test("a genuinely slower suite still fails the 192s margin on a reference-speed runner", () => {
  const result = evaluate(timings(193000), [calibration(4000), calibration(4000)], reference);
  assert.equal(result.ok, false);
  assert.match(result.failures.join("\n"), /exceeds the 192000ms margin/);
});

test("a faster runner makes the check stricter, not looser", () => {
  // 150s raw on a runner twice as fast as the reference is 300s of reference work.
  const result = evaluate(timings(150000), [calibration(2000), calibration(2000)], reference);
  assert.equal(result.ok, false);
  assert.equal(Math.round(result.normalized_ms), 300000);
});

test("the absolute 240s suite budget is never scaled", () => {
  const result = evaluate(timings(241000), [calibration(12000), calibration(12000)], reference);
  assert.equal(result.ok, false);
  assert.match(result.failures.join("\n"), /absolute 240000ms budget/);
});

test("calibration must run the exact reference workload before and after the suite", () => {
  for (const changed of [{ iterations: 80 }, { checksum: 1 }, { workload_version: 2 }, { concurrency: 4 }, { median_ms: 0 }]) {
    const result = evaluate(timings(120000), [{ ...calibration(4000), ...changed }, calibration(4000)], reference);
    assert.equal(result.ok, false, JSON.stringify(changed));
  }
  assert.equal(evaluate(timings(120000), [calibration(4000)], reference).ok, false);
  assert.equal(evaluate(timings(120000), [calibration(4000), calibration(4000)], { ...reference, reference_ms: null }).ok, false);
  assert.equal(evaluate(timings(120000), [calibration(4000), calibration(4000)], { ...reference, margin_budget_ms: 240000 }).ok, false);
  assert.equal(evaluate(timings(120000), [calibration(4000), calibration(4000)], { ...reference, hard_budget_ms: 300000 }).ok, false);
});

test("failed, partial or non-Linux renderer runs never pass", () => {
  for (const changed of [{ success: false }, { scope: "desktop" }, { platform: "win32" }, { summary: {} }]) {
    assert.equal(evaluate({ ...timings(120000), ...changed }, [calibration(4000), calibration(4000)], reference).ok, false, JSON.stringify(changed));
  }
});

test("runner speed change during the job is reported and the average is used", () => {
  const result = evaluate(timings(120000), [calibration(4000), calibration(6000)], reference);
  assert.equal(result.ok, true);
  assert.equal(result.factor, 1.25);
  assert.match(result.warnings.join("\n"), /runner speed changed/);
});

test("the reference workload checksum matches the committed reference", () => {
  assert.equal(workload(loadJSDOM(), 40), require("./renderer-calibration-reference.json").checksum);
});

test("calibration workers report one consistent checksum per round", async () => {
  const result = await calibrate({ iterations: 3, rounds: 2, concurrency: 2 });
  assert.equal(result.samples_ms.length, 2);
  assert.ok(result.samples_ms.every(round => round.length === 2 && round.every(ms => ms > 0)));
  assert.equal(result.checksum, workload(loadJSDOM(), 3));
  assert.ok(result.median_ms > 0);
});

test("CI calibrates before and after the renderer suite and gates with the reference-speed check", () => {
  const fs = require("node:fs"), path = require("node:path");
  const ci = fs.readFileSync(path.join(__dirname, "..", ".github", "workflows", "ci.yml"), "utf8");
  const before = ci.indexOf('node scripts/runner-calibration.cjs "$RUNNER_TEMP/runner-calibration-before.json"');
  const suite = ci.indexOf("node scripts/test-renderers.cjs all");
  const after = ci.indexOf('node scripts/runner-calibration.cjs "$RUNNER_TEMP/runner-calibration-after.json"');
  const gate = ci.indexOf('node scripts/renderer-budget-gate.cjs "$RUNNER_TEMP/renderer-timings-linux.json" "$RUNNER_TEMP/runner-calibration-before.json" "$RUNNER_TEMP/runner-calibration-after.json"');
  assert.ok(before > 0 && before < suite && suite < after && after < gate, JSON.stringify({ before, suite, after, gate }));
  assert.match(ci.slice(ci.lastIndexOf("- name:", gate), gate), /name: Enforce R252 renderer budget margin/);
  assert.doesNotMatch(ci, /RUNNER_CALIBRATION_ITERATIONS/, "CI must run the reference workload size");
});

// The 15 CI runs of 7d5e7b31 (2026-10-09) that set reference_ms:
// [run id, CPU model, renderer duration_ms, calibration median_ms before, after].
const observedRuns = [
  ["37878415874", "7763", 202211, 3605.638, 3404.898],
  ["37878419296", "7763", 208611, 3927.276, 3542.722],
  ["37878422975", "9V45", 124582, 2125.201, 2068.054],
  ["37878426748", "7763", 206418, 3724.57, 3583.799],
  ["37878430177", "9V74", 161338, 2774.697, 3114.031],
  ["37878433809", "9V45", 128619, 2268.855, 2333.75],
  ["37878437058", "7763", 211098, 3756.243, 3698.816],
  ["37878440316", "7763", 210169, 3828.884, 3602.803],
  ["37878443558", "9V74", 190370, 3261.235, 3200.662],
  ["37879575021", "7763", 214397, 3797.589, 3754.496],
  ["37879578739", "7763", 210783, 3715.116, 3719.034],
  ["37879582796", "7763", 207615, 3628.338, 3552.657],
  ["37880652353", "7763", 208899, 3472.032, 3762.583],
  ["37880655570", "9V74", 162031, 2809.77, 3075.387],
  ["37880658764", "6973P-C", 134683, 2211.77, 2041.914],
];

test("committed reference_ms is the fast runner class median from the CI runs of 7d5e7b31", () => {
  const committed = require("./renderer-calibration-reference.json");
  const fast = observedRuns.filter(([, cpu]) => cpu === "9V45" || cpu === "6973P-C")
    .map(([, , , before, after]) => (before + after) / 2).sort((a, b) => a - b);
  assert.equal(fast.length, 3);
  assert.equal(committed.reference_ms, Math.round(fast[1]));
});

test("every observed CI run of 7d5e7b31 passes at the committed reference on all four runner models", () => {
  const committed = require("./renderer-calibration-reference.json");
  for (const [run, cpu, duration, before, after] of observedRuns) {
    const result = evaluate(timings(duration), [calibration(before), calibration(after)], committed);
    assert.equal(result.ok, true, `${run} ${cpu}: ${result.failures.join("; ")}`);
    assert.ok(result.normalized_ms < 140000, `${run} ${cpu}: ${result.normalized_ms}`);
  }
});
