'use strict';
const test = require('node:test'), assert = require('node:assert/strict');
const sharding = require('./renderer-shards.cjs'), total = require('./renderer-shard-total.cjs');
const {evaluate} = require('./renderer-budget-gate.cjs'), reference = require('./renderer-calibration-reference.json');
const context = {inventory: Array.from({length: 20}, (_, n) => `scripts/fixture-${String(n).padStart(2, '0')}.test.cjs`), source_sha: 'a'.repeat(40), run_id: '123'};
const calibration = factor => ({...reference, median_ms: reference.reference_ms * factor});
function bundles(referenceCost = 1000000, factors = [1, 1]) {
  return sharding.plan(context.inventory).map((selection, n) => {
    const factor = factors[n], files = selection.files.map(file => ({file, duration_ms: referenceCost / context.inventory.length * factor}));
    return {timings: {scope: 'all', platform: 'linux', concurrency: 5, file_budget_ms: 100000, suite_budget_ms: 270000,
      success: true, shard: {...sharding.metadata(selection, context.inventory), source_sha: context.source_sha, run_id: context.run_id, run_attempt: '1'}, files,
      summary: {success: true, duration_ms: 120000 * factor, counts: {tests: files.length, passed: files.length, failed: 0, cancelled: 0, skipped: 0}}}, calibrations: [calibration(factor), calibration(factor)]};
  });
}
test('R309 aggregate pins the specified first-attempt source and exact eight percent allowance', () => {
  const source = sharding.measured.source;
  assert.equal(total.BASE_REFERENCE_FILE_SUM_MS, 994423.3845930878);
  assert.equal(total.MAX_REFERENCE_FILE_SUM_MS, 1073977.2553605349, 'total reference-speed limit must remain baseline plus exactly eight percent');
  assert.equal(total.MAX_REFERENCE_FILE_SUM_MS, total.BASE_REFERENCE_FILE_SUM_MS * 1.08);
  assert.equal(source.raw_file_sum_ms / (((source.calibration_before_ms + source.calibration_after_ms) / 2) / source.reference_ms), total.BASE_REFERENCE_FILE_SUM_MS);
  assert.ok(Math.abs(Object.values(sharding.measured.costs_ms).reduce((sum, n) => sum + n, 0) - source.raw_file_sum_ms) < 0.000001);
});
test('R309 aggregate sums each shard at its own calibrated speed and fails above the total limit', () => {
  const result = total.evaluateTotal(bundles(1000000, [1, 2]), context);
  assert.equal(result.ok, true, result.failures.join('; '));
  assert.equal(result.total_reference_file_sum_ms, 1000000);
  assert.deepEqual(result.shards.map(s => s.factor), [1, 2]);
  const retried = bundles(); retried[1].timings.shard.run_attempt = '2';
  assert.equal(total.evaluateTotal(retried, context).ok, true, 'a single-shard retry must retain the successful sibling artifact from its earlier attempt');
  assert.equal(total.evaluateTotal(bundles(1073977), context).ok, true);
  assert.equal(total.evaluateTotal(bundles(1073978), context).ok, false, 'aggregate total above baseline plus eight percent must fail');
  assert.match(total.evaluateTotal(bundles(1073978), context).failures.join('\n'), /exceeds baseline plus eight percent/);
});
test('R309 aggregate rejects missing shards, duplicate coverage and foreign revisions', () => {
  const original = bundles();
  assert.equal(total.evaluateTotal(original.slice(0, 1), context).ok, false);
  assert.equal(total.evaluateTotal([original[0], original[0]], context).ok, false);
  for (const mutate of [b => b[0].timings.files.pop(), b => b[0].timings.files.push(b[1].timings.files[0]),
    b => b[0].timings.shard.source_sha = 'b'.repeat(40), b => b[0].timings.shard.run_id = '124',
    b => b[0].timings.shard.inventory_sha256 = 'bad', b => b[0].timings.shard.files_sha256 = 'bad',
    b => b[0].calibrations.pop(), b => b[0].calibrations[0].checksum++, b => b[0].timings.success = false,
    b => b[0].timings.summary.success = false, b => delete b[0].timings.shard.run_attempt,
    b => b[0].timings.shard.run_attempt = '0']) {
    const broken = structuredClone(original); mutate(broken);
    assert.equal(total.evaluateTotal(broken, context).ok, false);
  }
});
test('R309 shard inputs preserve all three original gate limits exactly', () => {
  const sample = bundles()[0];
  assert.equal(evaluate(sample.timings, sample.calibrations, reference).ok, true);
  for (const changed of [{file_budget_ms: 100001}, {suite_budget_ms: 270001}, {summary: {...sample.timings.summary, duration_ms: 192001}}])
    assert.equal(evaluate({...sample.timings, ...changed}, sample.calibrations, reference).ok, false);
  const exact = {...sample.timings, files: [{file: 'fixture.test.cjs', duration_ms: 100000}], summary: {...sample.timings.summary, duration_ms: 270000}};
  const slow = [calibration(2), calibration(2)];
  assert.equal(evaluate(exact, slow, reference).ok, true);
  assert.equal(evaluate({...exact, summary: {...exact.summary, duration_ms: 270001}}, slow, reference).ok, false);
  assert.equal(evaluate({...exact, files: [{file: 'fixture.test.cjs', duration_ms: 100001}]}, slow, reference).ok, false);
});
