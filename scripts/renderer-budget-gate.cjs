"use strict";

// R252 renderer budget margin, evaluated against the speed of the runner that
// ran the suite (option B). The same commit measured 131.9s on one GitHub
// runner and 209.4s on another, every file slower by the same ~1.6x, so a raw
// 192s line failed or passed by the luck of the runner draw.
//
// Rules (none of the budgets is changed):
//  - the raw suite must still finish within the absolute 240s budget;
//  - the suite time converted to the reference runner speed must stay within
//    192s (20% margin under 240s). factor = measured calibration / reference
//    calibration; on a faster runner factor < 1 and the check gets stricter.
//  - calibrations must run the exact reference workload (version, iterations,
//    concurrency, checksum) before and after the suite.
const fs = require("node:fs");

function finitePositive(value) {
  return typeof value === "number" && Number.isFinite(value) && value > 0;
}

function evaluate(timings, calibrations, reference) {
  const failures = [];
  const warnings = [];
  if (!finitePositive(reference?.reference_ms)) failures.push("reference_ms is not set; collect CI calibrations first");
  if (reference?.margin_budget_ms !== 192000) failures.push(`margin budget must stay 192000ms, got ${reference?.margin_budget_ms}`);
  if (reference?.hard_budget_ms !== 240000) failures.push(`hard budget must stay 240000ms, got ${reference?.hard_budget_ms}`);
  if (!timings || timings.success !== true) failures.push("renderer suite did not succeed");
  if (timings?.scope !== "all") failures.push(`renderer scope must be all, got ${timings?.scope}`);
  if (timings?.platform !== "linux") failures.push(`renderer platform must be linux, got ${timings?.platform}`);
  const raw = timings?.summary?.duration_ms;
  if (!finitePositive(raw)) failures.push("renderer duration_ms missing");
  else if (raw > 240000) failures.push(`raw renderer suite ${raw}ms exceeds the absolute 240000ms budget`);

  if (!Array.isArray(calibrations) || calibrations.length !== 2) failures.push("exactly two calibrations (before and after the suite) are required");
  const medians = [];
  for (const [index, calibration] of (Array.isArray(calibrations) ? calibrations : []).entries()) {
    const label = index === 0 ? "before" : "after";
    if (calibration?.workload_version !== reference?.workload_version) failures.push(`${label} calibration workload_version ${calibration?.workload_version} != ${reference?.workload_version}`);
    if (calibration?.iterations !== reference?.iterations) failures.push(`${label} calibration iterations ${calibration?.iterations} != ${reference?.iterations}`);
    if (calibration?.checksum !== reference?.checksum) failures.push(`${label} calibration checksum ${calibration?.checksum} != ${reference?.checksum}`);
    if (calibration?.concurrency !== reference?.concurrency || calibration?.concurrency !== timings?.concurrency) failures.push(`${label} calibration concurrency ${calibration?.concurrency} must equal reference ${reference?.concurrency} and suite ${timings?.concurrency}`);
    if (!finitePositive(calibration?.median_ms)) failures.push(`${label} calibration median_ms missing`);
    else medians.push(calibration.median_ms);
  }

  let factor = null, normalized = null;
  if (medians.length === 2 && finitePositive(reference?.reference_ms)) {
    const spread = Math.max(...medians) / Math.min(...medians);
    if (spread > 1.35) warnings.push(`runner speed changed during the job: before/after calibration differ by ${spread.toFixed(2)}x`);
    factor = (medians[0] + medians[1]) / 2 / reference.reference_ms;
    if (finitePositive(raw)) {
      normalized = raw / factor;
      if (normalized > 192000) failures.push(`renderer suite ${Math.round(normalized)}ms at reference runner speed exceeds the 192000ms margin (raw ${Math.round(raw)}ms, runner factor ${factor.toFixed(3)})`);
    }
  }
  return { ok: failures.length === 0, failures, warnings, raw_ms: raw ?? null, factor, normalized_ms: normalized, calibration_medians_ms: medians,
    reference_ms: reference?.reference_ms ?? null, margin_budget_ms: 192000, hard_budget_ms: 240000 };
}

module.exports = { evaluate };

if (require.main === module) {
  const [timingsPath, beforePath, afterPath, outputPath] = process.argv.slice(2);
  const read = file => JSON.parse(fs.readFileSync(file, "utf8"));
  const result = evaluate(read(timingsPath), [read(beforePath), read(afterPath)], require("./renderer-calibration-reference.json"));
  const text = JSON.stringify(result, null, 2) + "\n";
  if (outputPath) fs.writeFileSync(outputPath, text);
  process.stdout.write(text);
  for (const warning of result.warnings) console.log(`::warning::${warning}`);
  if (!result.ok) {
    for (const failure of result.failures) console.error(`::error::${failure}`);
    process.exitCode = 1;
  } else {
    console.log(`R252 renderer margin >=20% at reference runner speed: ${Math.round(result.normalized_ms)}ms (raw ${Math.round(result.raw_ms)}ms, factor ${result.factor.toFixed(3)})`);
  }
}
