'use strict';
const test = require('node:test'), assert = require('node:assert/strict');
const { diagnosticShards } = require('./r276-browser-catalog.cjs');
const { artifactName, selectArtifacts, verifySources, list } = require('./r279-diagnostic-artifacts.cjs');
const sha = 'a'.repeat(40), workflowSha = 'b'.repeat(40), runId = '123';
function fixture() {
  const jobs = [], artifacts = [], sources = {};
  for (const [i, shard] of [...Object.keys(diagnosticShards), 'strict'].entries()) {
    jobs.push({ name: shard === 'strict' ? 'strict' : `browser (${shard})`, status: 'completed', conclusion: 'success', started_at: '2026-10-16T01:00:00Z', completed_at: '2026-10-16T01:05:00Z', run_attempt: 1 });
    artifacts.push({ id: i + 1, name: artifactName(shard, runId, 1), created_at: '2026-10-16T01:04:00Z', expired: false, workflow_run: { id: Number(runId), head_sha: workflowSha } });
    sources[`${shard}/artifact-source.json`] = { schema: 1, shard, runId, attempt: 1, sha, workflowSha };
  }
  return { jobs, artifacts, sources, runId, sha, workflowSha };
}
function retry(f, success = true) {
  f.jobs.push({ ...f.jobs[0], started_at: '2026-10-16T01:10:00Z', completed_at: '2026-10-16T01:15:00Z', conclusion: success ? 'success' : 'failure', run_attempt: 2 });
  f.artifacts.push({ ...f.artifacts[0], id: 100, name: artifactName('diagnostic-r266', runId, 2), created_at: '2026-10-16T01:14:00Z' });
  f.sources['diagnostic-r266/artifact-source.json'].attempt = 2;
}
test('old failure plus new success selects new immutable ID while untouched shards keep prior evidence', () => {
  const f = fixture(); f.jobs[0].conclusion = 'failure'; retry(f);
  // Simulate copied jobs reporting run_attempt 2 although their execution stayed in attempt 1.
  f.jobs.push(...f.jobs.slice(1, 6).map(j => ({ ...j, run_attempt: 2 })));
  const selected = selectArtifacts(f);
  assert.equal(selected.selected[0].artifactId, 100);
  assert.deepEqual(selected.selected.slice(1).map(x => x.attempt), [1, 1, 1, 1, 1]);
  verifySources(file => f.sources[file], selected);
});
test('old success cannot hide new failure or missing new artifact', () => {
  const failed = fixture(); retry(failed, false); assert.throws(() => selectArtifacts(failed), /latest execution must succeed/);
  const missing = fixture(); retry(missing); missing.artifacts.pop(); assert.throws(() => selectArtifacts(missing), /refusing stale fallback/);
});
test('wrong run, workflow SHA, missing shard, expired or ambiguous latest evidence must fail', () => {
  for (const mutate of [f => f.artifacts[0].workflow_run.id = 999, f => f.artifacts[0].workflow_run.head_sha = 'c'.repeat(40), f => f.jobs.shift(), f => f.artifacts[0].expired = true, f => f.artifacts.push({ ...f.artifacts[0], id: 99 }), f => f.artifacts[1].id = f.artifacts[0].id]) {
    const f = fixture(); mutate(f); assert.throws(() => selectArtifacts(f), { name: 'AssertionError' });
  }
});
test('downloaded source must match selected candidate SHA, run, attempt and shard', () => {
  for (const mutate of [s => s.sha = 'c'.repeat(40), s => s.runId = '999', s => s.attempt = 2, s => s.shard = 'strict']) {
    const f = fixture(), selected = selectArtifacts(f); mutate(f.sources['diagnostic-r266/artifact-source.json']);
    assert.throws(() => verifySources(file => f.sources[file], selected), /artifact identity mismatch/);
  }
});
test('artifact API pagination retains all immutable IDs and rejects HTTP failure', async () => {
  let calls = 0;
  const rows = await list('artifacts', 'artifacts', { GITHUB_REPOSITORY: 'o/r', GITHUB_RUN_ID: runId, GH_TOKEN: 'fixture' }, async url => {
    calls++; assert(url.includes(`page=${calls}`)); return { ok: true, json: async () => ({ artifacts: calls === 1 ? Array.from({ length: 100 }, (_, i) => ({ id: i })) : [{ id: 100 }] }) };
  });
  assert.equal(rows.length, 101); assert.equal(calls, 2);
  await assert.rejects(list('artifacts', 'artifacts', {}, async () => ({ ok: false, status: 403 })), /HTTP 403/);
});
test('source mutants selecting stale IDs, ignoring failed jobs or skipping source identity are killed', () => {
  const fs = require('node:fs'), Module = require('node:module');
  const file = require.resolve('./r279-diagnostic-artifacts.cjs'), source = fs.readFileSync(file, 'utf8');
  function load(text) { const m = new Module(file, module); m.filename = file; m.paths = module.paths; m._compile(text, file); return m.exports; }
  const mutants = [
    [source.replace('const a = candidates[0];', 'const a = artifacts.find(a => a.name.startsWith(prefix));'), api => { const f = fixture(); retry(f); assert.equal(api.selectArtifacts(f).selected[0].artifactId, 100, 'latest immutable artifact must win'); }],
    [source.replace("latest.every(j => j.status === 'completed' && j.conclusion === 'success')", 'true'), api => { const f = fixture(); retry(f, false); assert.throws(() => api.selectArtifacts(f), /latest execution must succeed/, 'new failure must remain red'); }],
    [source.replace('function verifySources(read, selection) {', 'function verifySources(read, selection) { return;'), api => { const f = fixture(), selected = selectArtifacts(f); f.sources['strict/artifact-source.json'].sha = 'c'.repeat(40); assert.throws(() => api.verifySources(p => f.sources[p], selected), /artifact identity mismatch/, 'wrong SHA must fail'); }]
  ];
  for (const [mutant, oracle] of mutants) { assert.notEqual(mutant, source); assert.throws(() => oracle(load(mutant)), { name: 'AssertionError' }); }
});
