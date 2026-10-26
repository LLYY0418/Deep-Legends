'use strict';
const test = require('node:test'), assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path');
const yaml = require('../desktop/node_modules/js-yaml'), contract = require('./renderer-shard-contract.cjs');
const workflow = () => yaml.load(fs.readFileSync(path.join(__dirname, '../.github/workflows/ci.yml'), 'utf8'));
test('R309 renderer workflow has two independent gates and an unskippable total guard', () => {
  const ci = workflow();
  assert.doesNotThrow(() => contract.verifyRendererJobs(ci));
  assert.deepEqual(ci, contract.approvedWorkflow(require('./r277-ci-legacy-baseline.json').workflow), 'approved renderer sharding must preserve every CI guard');
  assert.deepEqual(ci.jobs['quality-web'].strategy, {'fail-fast': false, matrix: {shard: [1, 2]}});
  const {expectedJobs} = require('./release-readiness.cjs'), names = expectedJobs(ci);
  assert.equal(names.filter(name => name.startsWith('quality-web (')).length, 2);
  assert(names.includes('quality-web-total'));
  assert.equal(names.length, 22, 'readiness must require every original job plus both renderer shards and the total guard');
  const steps = ci.jobs['quality-web'].steps;
  for (const name of ['Test Riot relay Worker', 'Audit browser process and directory cleanup', 'Validate frontend JavaScript'])
    assert.equal(steps.find(step => step.name === name).if, '${{ !cancelled() && matrix.shard == 1 }}');
  const aggregate = ci.jobs['quality-web-total'];
  assert.deepEqual(aggregate.needs, ['quality-web']);
  assert.equal(aggregate.if, '${{ always() && !cancelled() }}');
  assert.equal(aggregate.steps.find(step => step.run?.includes('renderer-shard-total.cjs')).if, '${{ !cancelled() }}');
  assert.equal(aggregate.steps.filter(step => step.uses === 'actions/download-artifact@v4').length, 2);
  assert(aggregate.steps.filter(step => step.uses === 'actions/download-artifact@v4').every(step => step.with['github-token'] === '${{ github.token }}' && step.with['merge-multiple'] === false));
});
test('R309 workflow verification rejects a skipped total guard or missing shard', () => {
  for (const mutate of [ci => ci.jobs['quality-web-total'].if = 'false',
    ci => ci.jobs['quality-web-total'].steps.find(step => step.run?.includes('renderer-shard-total.cjs')).if = 'false',
    ci => ci.jobs['quality-web'].strategy.matrix.shard.pop(), ci => delete ci.jobs['quality-web-total'],
    ci => ci.jobs['quality-web'].steps.find(step => step.name === 'Test Riot relay Worker').if = '${{ !cancelled() }}']) {
    const broken = workflow(); mutate(broken);
    assert.throws(() => contract.verifyRendererJobs(broken), /approved renderer sharding must preserve every CI guard/);
  }
});
