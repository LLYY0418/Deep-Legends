'use strict';
const fs = require('node:fs'), path = require('node:path'), assert = require('node:assert/strict');
const sharding = require('./renderer-shards.cjs');
const {evaluate} = require('./renderer-budget-gate.cjs');
const reference = require('./renderer-calibration-reference.json');
// User-approved R309 total-work guard, sourced from CI40217266825 attempt1,
// SHA f460f269, timings artifact12608312251 / calibrations12608292530.
// All330 files succeeded (the old wall-time margin did not).
// 763574.097047 raw file-ms / (((1664.325+1602.135)/2)/2127)
// =994423.3845930878 reference file-ms. Exactly+8%, without rounding up.
const BASE_REFERENCE_FILE_SUM_MS = 994423.3845930878;
const MAX_REFERENCE_FILE_SUM_MS = 1073977.2553605349;
function evaluateTotal(bundles, {inventory = sharding.discover(), source_sha = process.env.GITHUB_SHA, run_id = process.env.GITHUB_RUN_ID} = {}) {
  const failures = [], results = [];
  const check = (condition, message) => { if (!condition) failures.push(message); };
  check(bundles.length === 2, 'aggregate requires both renderer shards');
  check(Boolean(source_sha) && Boolean(run_id), 'aggregate requires an explicit source SHA and workflow run');
  const expected = sharding.plan(inventory), seen = [], ids = [];
  let total_reference_file_sum_ms = 0;
  for (const {timings, calibrations} of bundles) {
    const meta = timings?.shard, gate = evaluate(timings, calibrations, reference);
    check(gate.ok, `shard ${meta?.index}: ${gate.failures.join('; ')}`);
    check(meta?.count === 2 && [1, 2].includes(meta?.index), 'invalid renderer shard identity');
    check(meta?.source_sha === source_sha && meta?.run_id === run_id, 'renderer shard belongs to another source or workflow run');
    // A single-shard retry legitimately combines attempts from the same run.
    check(/^[1-9]\d*$/.test(String(meta?.run_attempt)), 'renderer shard has an invalid run attempt');
    check(meta?.inventory_sha256 === sharding.digest(inventory), 'renderer shard inventory fingerprint mismatch');
    const files = timings?.files?.map(row => row.file) || [];
    ids.push(meta?.index); seen.push(...files);
    check(meta?.files_sha256 === sharding.digest(files), 'renderer shard file fingerprint mismatch');
    const selection = expected.find(shard => shard.index === meta?.index);
    check(Boolean(selection) && sharding.digest(files) === sharding.digest(selection.files) && files.length === selection.files.length,
      'renderer shard does not match its deterministic file selection');
    const counts = timings?.summary?.counts;
    check(timings?.summary?.success === true && counts?.tests > 0 && counts.failed === 0 && counts.cancelled === 0, 'renderer shard did not complete its tests');
    const raw_file_sum_ms = (timings?.files || []).reduce((sum, row) => sum + row.duration_ms, 0);
    const reference_file_sum_ms = raw_file_sum_ms / gate.factor;
    check(Number.isFinite(reference_file_sum_ms) && reference_file_sum_ms > 0, 'renderer shard reference file-sum is invalid');
    total_reference_file_sum_ms += reference_file_sum_ms;
    results.push({index: meta?.index, raw_file_sum_ms, factor: gate.factor, reference_file_sum_ms, gate, source: meta});
  }
  check(JSON.stringify(ids.sort()) === '[1,2]', 'aggregate requires each renderer shard exactly once');
  check(seen.length === inventory.length && new Set(seen).size === inventory.length && sharding.digest(seen) === sharding.digest(inventory),
    'aggregate renderer files must cover the full inventory exactly once');
  if (total_reference_file_sum_ms > MAX_REFERENCE_FILE_SUM_MS) failures.push('aggregate reference file-sum exceeds baseline plus eight percent');
  return {ok: failures.length === 0, failures, baseline_reference_file_sum_ms: BASE_REFERENCE_FILE_SUM_MS,
    limit_reference_file_sum_ms: MAX_REFERENCE_FILE_SUM_MS, total_reference_file_sum_ms, shards: results};
}
function loadBundles(directory) {
  const read = file => JSON.parse(fs.readFileSync(file, 'utf8'));
  return [1, 2].map(index => ({
    timings: read(path.join(directory, `renderer-timings-linux-${index}`, 'renderer-timings-linux.json')),
    calibrations: ['before', 'after'].map(phase => read(path.join(directory, `renderer-budget-gate-linux-${index}`, `runner-calibration-${phase}.json`)))
  }));
}
module.exports = {BASE_REFERENCE_FILE_SUM_MS, MAX_REFERENCE_FILE_SUM_MS, evaluateTotal, loadBundles};
if (require.main === module) {
  const [input, output] = process.argv.slice(2); let result;
  try { assert(input && output, 'aggregate requires input directory and output JSON'); result = evaluateTotal(loadBundles(input)); }
  catch (error) { result = {ok: false, failures: [error.stack || String(error)]}; }
  if (output) fs.writeFileSync(output, JSON.stringify(result, null, 2) + '\n');
  console.log(JSON.stringify(result, null, 2)); process.exitCode = result.ok ? 0 : 1;
}
