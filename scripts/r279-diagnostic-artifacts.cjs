'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { diagnosticShards } = require('./r276-browser-catalog.cjs');
const shards = [...Object.keys(diagnosticShards), 'strict'];
function artifactName(shard, runId, attempt) {
  assert(shards.includes(shard), 'unknown diagnostic shard');
  return `r279-quick-${shard}-${runId}-${attempt}`;
}
function selectArtifacts({ jobs, artifacts, runId, sha, workflowSha }) {
  assert(/^\d+$/.test(String(runId)), 'invalid diagnostic run ID');
  assert(/^[a-f0-9]{40}$/.test(sha), 'invalid resolved candidate SHA');
  assert(/^[a-f0-9]{40}$/.test(workflowSha), 'invalid workflow SHA');
  const selected = shards.map(shard => {
    const jobName = shard === 'strict' ? 'strict' : `browser (${shard})`;
    const executions = jobs.filter(j => j.name === jobName && j.started_at);
    assert(executions.length, `${shard}: missing job execution`);
    // GitHub copies unchanged jobs into later attempts with their original timestamps.
    // run_attempt alone would incorrectly discard their still-valid earlier artifacts.
    const newestStart = Math.max(...executions.map(j => Date.parse(j.started_at)));
    assert(Number.isFinite(newestStart), `${shard}: invalid job timestamp`);
    const latest = executions.filter(j => Date.parse(j.started_at) === newestStart);
    assert(latest.every(j => j.status === 'completed' && j.conclusion === 'success'), `${shard}: latest execution must succeed`);
    const end = Math.max(...latest.map(j => Date.parse(j.completed_at)));
    assert(Number.isFinite(end) && end >= newestStart, `${shard}: invalid completion timestamp`);
    const prefix = `r279-quick-${shard}-${runId}-`;
    const candidates = artifacts.filter(a => {
      if (!a.name.startsWith(prefix)) return false;
      const attempt = Number(a.name.slice(prefix.length));
      assert(Number.isSafeInteger(attempt) && attempt > 0, `${shard}: invalid artifact attempt`);
      assert.equal(String(a.workflow_run?.id), String(runId), `${shard}: wrong artifact run`);
      assert.equal(a.workflow_run?.head_sha, workflowSha, `${shard}: wrong workflow SHA`);
      const created = Date.parse(a.created_at);
      return created >= newestStart && created <= end;
    });
    assert.equal(candidates.length, 1, `${shard}: latest execution requires one unique artifact; refusing stale fallback`);
    const a = candidates[0];
    assert(!a.expired, `${shard}: latest artifact expired`);
    assert(Number.isSafeInteger(a.id) && a.id > 0, `${shard}: invalid immutable artifact ID`);
    return { shard, artifactId: a.id, artifactName: a.name, attempt: Number(a.name.slice(prefix.length)), startedAt: new Date(newestStart).toISOString(), completedAt: new Date(end).toISOString() };
  });
  assert.equal(new Set(selected.map(a => a.artifactId)).size, shards.length, 'duplicate immutable artifact ID');
  return { schema: 1, runId: String(runId), sha, workflowSha, selected };
}
function verifySources(read, selection) {
  assert.equal(selection.schema, 1, 'missing diagnostic artifact selection');
  assert.deepEqual(selection.selected.map(a => a.shard), shards, 'exact diagnostic artifact identity coverage required');
  assert.equal(new Set(selection.selected.map(a => a.artifactId)).size, shards.length, 'duplicate immutable artifact ID');
  for (const item of selection.selected) {
    const source = read(`${item.shard}/artifact-source.json`);
    assert.deepEqual(source, { schema: 1, shard: item.shard, runId: selection.runId, attempt: item.attempt, sha: selection.sha, workflowSha: selection.workflowSha }, `${item.shard}: artifact identity mismatch`);
    assert.equal(item.artifactName, artifactName(item.shard, selection.runId, item.attempt), 'artifact name identity mismatch');
  }
}
async function list(endpoint, key, env, fetcher = fetch) {
  const all = [];
  for (let page = 1; ; page++) {
    const response = await fetcher(`https://api.github.com/repos/${env.GITHUB_REPOSITORY}/actions/runs/${env.GITHUB_RUN_ID}/${endpoint}${endpoint.includes('?') ? '&' : '?'}per_page=100&page=${page}`, { headers: { Authorization: `Bearer ${env.GH_TOKEN}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28' } });
    assert(response.ok, `GitHub diagnostic evidence list failed: HTTP ${response.status}`);
    const rows = (await response.json())[key]; assert(Array.isArray(rows), 'invalid diagnostic evidence API response');
    all.push(...rows); if (rows.length < 100) return all;
  }
}
async function main(args, env = process.env) {
  const [command, root, shard] = args;
  if (command === 'stamp') {
    const source = { schema: 1, shard, runId: env.GITHUB_RUN_ID, attempt: Number(env.GITHUB_RUN_ATTEMPT), sha: env.DIAGNOSTIC_SHA, workflowSha: env.GITHUB_SHA };
    artifactName(shard, source.runId, source.attempt);
    fs.mkdirSync(path.join(root, shard), { recursive: true });
    fs.writeFileSync(path.join(root, shard, 'artifact-source.json'), JSON.stringify(source, null, 2) + '\n');
  } else if (command === 'select') {
    assert(env.GH_TOKEN, 'diagnostic artifact read token missing');
    const [jobs, artifacts] = await Promise.all([list('jobs?filter=all', 'jobs', env), list('artifacts', 'artifacts', env)]);
    const selection = selectArtifacts({ jobs, artifacts, runId: env.GITHUB_RUN_ID, sha: env.DIAGNOSTIC_SHA, workflowSha: env.GITHUB_SHA });
    fs.mkdirSync(root, { recursive: true });
    fs.writeFileSync(path.join(root, 'artifact-selection.json'), JSON.stringify(selection, null, 2) + '\n');
    fs.appendFileSync(env.GITHUB_OUTPUT, `ids=${selection.selected.map(a => a.artifactId).join(',')}\n`);
    console.log(JSON.stringify(selection));
  } else throw Error('Expected stamp <root> <shard> or select <root>');
}
module.exports = { artifactName, selectArtifacts, verifySources, list };
if (require.main === module) main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exitCode = 1; });
