'use strict';
const test = require('node:test'), assert = require('node:assert/strict');
const sharding = require('./renderer-shards.cjs');
test('R309 renderer planning covers every discovered test exactly once and balances measured work', () => {
  const inventory = sharding.discover(), shards = sharding.plan(inventory);
  assert.deepEqual(shards.flatMap(s => s.files).sort(), [...inventory].sort(), 'renderer inventory must appear exactly once across two shards');
  assert.equal(new Set(shards.flatMap(s => s.files)).size, inventory.length);
  const sums = sharding.validate(shards, inventory);
  assert.ok(Math.max(...sums) / Math.min(...sums) <= 1.10);
  assert.deepEqual(sharding.plan([...inventory].reverse()), shards, 'OS discovery order must not affect the renderer partition');
  assert.equal(sharding.measured.source.run, 40217266825);
  assert.equal(sharding.measured.source.attempt, 1);
  assert.equal(Object.keys(sharding.measured.costs_ms).length, 330);
  assert(sharding.measured.source.note.includes('normalized suite margin failed'));
});
test('R309 renderer partition rejects missing, repeated and foreign files', () => {
  const inventory = sharding.discover(), shards = sharding.plan(inventory);
  const drop = structuredClone(shards); drop[0].files.pop();
  assert.throws(() => sharding.validate(drop, inventory), /renderer inventory must appear exactly once/, 'renderer coverage must reject a dropped file');
  const duplicate = structuredClone(shards); duplicate[1].files.push(duplicate[0].files[0]);
  assert.throws(() => sharding.validate(duplicate, inventory), /renderer inventory must appear exactly once/);
  const foreign = structuredClone(shards); foreign[0].files[0] = 'scripts/not-a-renderer.test.cjs';
  assert.throws(() => sharding.validate(foreign, inventory), /renderer inventory must appear exactly once/);
  assert.throws(() => sharding.validate(shards.slice(0, 1), inventory), /exactly two/);
  assert.equal(sharding.cost('scripts/newly-added.test.cjs'), 1000);
  assert.notEqual(sharding.digest(inventory.slice(1)), sharding.digest(inventory));
});
