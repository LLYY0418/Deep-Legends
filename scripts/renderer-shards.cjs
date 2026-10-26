'use strict';
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto'), assert = require('node:assert/strict');
const root = path.resolve(__dirname, '..');
const measured = require('./renderer-shard-costs.json');
const compare = (a, b) => a < b ? -1 : a > b ? 1 : 0;
function discover(project = root) {
  return ['backend/web', 'desktop', 'scripts'].flatMap(directory => fs.readdirSync(path.join(project, directory))
    .filter(name => name.endsWith('.test.cjs')).map(name => directory + '/' + name)).sort(compare);
}
const digest = files => crypto.createHash('sha256').update([...files].sort(compare).join('\n') + '\n').digest('hex');
const cost = file => measured.costs_ms[file] ?? measured.new_file_cost_ms;
function validate(shards, inventory) {
  assert.equal(shards.length, 2, 'renderer plan must have exactly two shards');
  assert.deepEqual(shards.map(shard => shard.index), [1, 2], 'renderer shard IDs must be exactly 1 and 2');
  const files = shards.flatMap(shard => shard.files);
  assert.equal(new Set(inventory).size, inventory.length, 'renderer inventory must be unique');
  assert.deepEqual([...files].sort(compare), [...inventory].sort(compare), 'renderer inventory must appear exactly once across two shards');
  const totals = shards.map(shard => shard.files.reduce((sum, file) => sum + cost(file), 0));
  assert.ok(totals.every(total => Number.isFinite(total) && total > 0), 'renderer shards must have positive measured costs');
  assert.ok(Math.max(...totals) / Math.min(...totals) <= 1.10, 'renderer shard measured file-sums must differ by at most ten percent');
  return totals;
}
function plan(inventory = discover()) {
  // LPT, using CI40217266825 attempt1 costs. New files cost exactly1000ms.
  // Descending cost, bytewise path tie-break, then lowest total / lowest ID.
  // Only paths and measured costs enter the decision; OS/iteration order do not.
  const shards = [1, 2].map(index => ({index, files: [], estimated_ms: 0}));
  for (const file of [...inventory].sort((a, b) => cost(b) - cost(a) || compare(a, b))) {
    const shard = shards[0].estimated_ms <= shards[1].estimated_ms ? shards[0] : shards[1];
    shard.files.push(file); shard.estimated_ms += cost(file);
  }
  validate(shards, inventory);
  return shards;
}
function metadata(shard, inventory) {
  return {index: shard.index, count: 2, inventory_sha256: digest(inventory), files_sha256: digest(shard.files),
    source_sha: process.env.GITHUB_SHA || null, run_id: process.env.GITHUB_RUN_ID || null, run_attempt: process.env.GITHUB_RUN_ATTEMPT || null};
}
module.exports = {discover, digest, cost, validate, plan, metadata, measured};
if (require.main === module) {
  const inventory = discover(), shards = plan(inventory);
  console.log(JSON.stringify({inventory: inventory.length, shards, balance: Math.max(...shards.map(s => s.estimated_ms)) / Math.min(...shards.map(s => s.estimated_ms))}, null, 2));
}
