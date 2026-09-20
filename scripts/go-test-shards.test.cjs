"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { configuredShardCount, partitionTests } = require("./go-test-shards.cjs");

test("test sharding is stable and covers every test exactly once", () => {
  const names = Array.from({ length: 2009 }, (_, index) => `TestFixture${index}`);
  const first = partitionTests(names, 4);
  const second = partitionTests(names, 4);
  assert.deepEqual(first, second);
  assert.equal(first.length, 4);
  assert.deepEqual(first.flat().sort(), names.sort());
  assert.equal(new Set(first.flat()).size, names.length);
  assert.ok(Math.max(...first.map(shard => shard.length)) - Math.min(...first.map(shard => shard.length)) < 80);
});

test("known slow tests are spread across shards", () => {
  const slow = [
    "TestR99SeedQuotaDiagnosticsNeverContainIdentity",
    "TestR89DiskBudgetAfter2000Images",
    "TestR96PartialTruthDoesNotEndAsNone",
    "TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField",
  ];
  const shards = partitionTests([...slow, ...Array.from({ length: 100 }, (_, index) => `TestFast${index}`)], 4);
  assert.equal(new Set(slow.map(name => shards.findIndex(shard => shard.includes(name)))).size, 4);
});

test("shard count is bounded and configurable", () => {
  assert.equal(configuredShardCount({ GO_TEST_SHARDS: "1" }), 1);
  assert.equal(configuredShardCount({ GO_TEST_SHARDS: "16" }), 16);
  for (const value of ["0", "17", "four", ""]) {
    assert.throws(() => configuredShardCount({ GO_TEST_SHARDS: value }), /integer from 1 to 16/);
  }
});
