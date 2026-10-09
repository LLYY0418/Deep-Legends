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
