'use strict';
const assert = require('node:assert/strict');
// Explicit R309 user-approved delta to the immutable R287 workflow snapshot.
// No current-workflow input is used to construct the expected definitions.
function approvedWorkflow(baseline) {
  const expected = structuredClone(baseline), job = expected.jobs['quality-web'];
  job.strategy = {'fail-fast': false, matrix: {shard: [1, 2]}};
  const once = '${{ !cancelled() && matrix.shard == 1 }}';
  for (const step of job.steps) {
    if (['Audit browser process and directory cleanup', 'Validate frontend JavaScript', 'Test Riot relay Worker'].includes(step.name)) step.if = once;
    if (step.name === 'Test frontend and desktop renderers') step.run += ' --shard "${{ matrix.shard }}"';
    if (['renderer-timings-linux', 'renderer-budget-gate-linux'].includes(step.with?.name)) step.with.name += '-${{ matrix.shard }}';
    if (step.with?.name === 'r287-quality-web-audits') step.if = '${{ always() && matrix.shard == 1 }}';
  }
  const index = job.steps.findIndex(step => step.id === 'cal_before');
  assert(index >= 0, 'approved baseline must retain renderer calibrations');
  job.steps.splice(index, 0,
    {name: 'Verify renderer file partition', if: '${{ !cancelled() }}', run: 'node scripts/renderer-shards.cjs > "$RUNNER_TEMP/renderer-shards.json"'},
    {name: 'Prove renderer shard and total guards', if: once, run: 'node scripts/renderer-shard-mutations.cjs "$RUNNER_TEMP/renderer-shard-mutations"'});
  job.steps.push({uses: 'actions/upload-artifact@v4', if: '${{ always() && matrix.shard == 1 }}', with: {
    name: 'renderer-shard-mutations', path: '${{ runner.temp }}/renderer-shard-mutations/**', 'if-no-files-found': 'error'}});
  expected.jobs['quality-web-total'] = {
    needs: ['quality-web'], if: '${{ always() && !cancelled() }}', 'runs-on': 'ubuntu-24.04', permissions: {contents: 'read', actions: 'read'}, steps: [
      {uses: 'actions/checkout@v4'},
      {uses: 'actions/setup-node@v4', with: {'node-version': '22'}},
      ...['renderer-timings-linux-*', 'renderer-budget-gate-linux-*'].map(pattern => ({uses: 'actions/download-artifact@v4', if: '${{ !cancelled() }}', with: {
        pattern, 'merge-multiple': false, path: '${{ runner.temp }}/renderer-shards',
        'github-token': '${{ github.token }}', 'run-id': '${{ github.run_id }}'}})),
      {name: 'Enforce complete renderer coverage and total reference cost', if: '${{ !cancelled() }}',
        run: 'node scripts/renderer-shard-total.cjs "$RUNNER_TEMP/renderer-shards" "$RUNNER_TEMP/renderer-total.json"'},
      {uses: 'actions/upload-artifact@v4', if: 'always()', with: {name: 'renderer-total-linux', path: '${{ runner.temp }}/renderer-total.json', 'if-no-files-found': 'error'}}
    ]
  };
  expected.jobs['triage-legacy'].needs.splice(expected.jobs['triage-legacy'].needs.indexOf('quality-web') + 1, 0, 'quality-web-total');
  return expected;
}
function verifyRendererJobs(workflow) {
  const expected = approvedWorkflow(require('./r277-ci-legacy-baseline.json').workflow);
  for (const name of ['quality-web', 'quality-web-total']) assert.deepEqual(workflow.jobs?.[name], expected.jobs[name], 'approved renderer sharding must preserve every CI guard: ' + name);
  assert(workflow.jobs?.['triage-legacy']?.needs?.includes('quality-web-total'), 'triage must wait for the renderer total guard');
}
module.exports = {approvedWorkflow, verifyRendererJobs};
