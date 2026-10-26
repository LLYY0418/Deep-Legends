'use strict';
const fs = require('node:fs'), path = require('node:path'), os = require('node:os'), crypto = require('node:crypto'), assert = require('node:assert/strict');
const {spawnSync} = require('node:child_process');
const root = path.resolve(__dirname, '..'), output = path.resolve(process.argv[2] || '');
assert(process.argv[2], 'mutation output directory required'); fs.mkdirSync(output, {recursive: true});
const tests = ['renderer-shards.test.cjs', 'renderer-shard-total.test.cjs', 'renderer-shard-contract.test.cjs'];
const modules = ['renderer-shards.cjs', 'renderer-shard-total.cjs', 'renderer-shard-contract.cjs'];
const workflow = '.github/workflows/ci.yml';
const guarded = [...modules.map(name => 'scripts/' + name), workflow, 'scripts/renderer-budget-gate.cjs', 'scripts/r277-ci-legacy-baseline.json'];
const hashes = () => Object.fromEntries(guarded.map(file => [file, crypto.createHash('sha256').update(fs.readFileSync(path.join(root, file))).digest('hex')]));
const before = hashes(), rows = [];
const yaml = require('../desktop/node_modules/js-yaml');
function change(source, from, to) { assert.equal(source.split(from).length, 2, 'mutation anchor must be unique'); return source.replace(from, to); }
const mutations = [
  {name: 'dropped-renderer-file', file: 'scripts/renderer-shards.cjs', message: 'renderer inventory must appear exactly once across two shards',
    mutate: source => change(source, '  validate(shards, inventory);', '  shards[0].files.pop();\n  validate(shards, inventory);')},
  {name: 'widened-total-constant', file: 'scripts/renderer-shard-total.cjs', message: 'total reference-speed limit must remain baseline plus exactly eight percent',
    mutate: source => change(source, 'const MAX_REFERENCE_FILE_SUM_MS = 1073977.2553605349;', 'const MAX_REFERENCE_FILE_SUM_MS = 1200000;')},
  {name: 'bypassed-total-comparison', file: 'scripts/renderer-shard-total.cjs', message: 'aggregate total above baseline plus eight percent must fail',
    mutate: source => change(source, 'if (total_reference_file_sum_ms > MAX_REFERENCE_FILE_SUM_MS)', 'if (false)')},
  {name: 'skipped-total-job', file: workflow, message: 'approved renderer sharding must preserve every CI guard',
    mutate: source => { const ci = yaml.load(source); ci.jobs['quality-web-total'].if = 'false'; return yaml.dump(ci); }},
  {name: 'skipped-total-step', file: workflow, message: 'approved renderer sharding must preserve every CI guard',
    mutate: source => { const ci = yaml.load(source); ci.jobs['quality-web-total'].steps.find(step => step.run?.includes('renderer-shard-total.cjs')).if = 'false'; return yaml.dump(ci); }}
];
function execute(name, mutation) {
  const sandbox = fs.mkdtempSync(path.join(os.tmpdir(), 'renderer-shard-mutant-'));
  try {
    fs.mkdirSync(path.join(sandbox, 'backend')); fs.symlinkSync(path.join(root, 'backend/web'), path.join(sandbox, 'backend/web'), 'dir');
    fs.symlinkSync(path.join(root, 'desktop'), path.join(sandbox, 'desktop'), 'dir');
    fs.mkdirSync(path.join(sandbox, 'scripts'));
    for (const file of fs.readdirSync(path.join(root, 'scripts'))) fs.symlinkSync(path.join(root, 'scripts', file), path.join(sandbox, 'scripts', file));
    for (const file of [...modules, ...tests]) { const target = path.join(sandbox, 'scripts', file); fs.unlinkSync(target); fs.copyFileSync(path.join(root, 'scripts', file), target); }
    fs.mkdirSync(path.join(sandbox, '.github/workflows'), {recursive: true}); fs.copyFileSync(path.join(root, workflow), path.join(sandbox, workflow));
    if (mutation) { const target = path.join(sandbox, mutation.file); assert(!fs.lstatSync(target).isSymbolicLink()); fs.writeFileSync(target, mutation.mutate(fs.readFileSync(target, 'utf8'))); }
    const result = spawnSync(process.execPath, ['--test', ...tests.map(file => path.join(sandbox, 'scripts', file))],
      {cwd: sandbox, env: {...process.env, NODE_OPTIONS: '', R287_MUTATION_PATH: '1'}, encoding: 'utf8', timeout: 20000, maxBuffer: 16 * 1024 * 1024});
    const log = (result.stdout || '') + (result.stderr || ''); fs.writeFileSync(path.join(output, name + '.log'), log);
    const assertionVerified = mutation ? result.status !== 0 && !result.error && /AssertionError|ERR_ASSERTION/.test(log)
      && log.includes(mutation.message) && !/SyntaxError|Cannot find module|MODULE_NOT_FOUND/.test(log) : result.status === 0 && !result.error;
    const row = {name, exit: result.status, error: result.error?.message || null, assertionVerified, expectedMessage: mutation?.message || null};
    rows.push(row); fs.writeFileSync(path.join(output, 'results.json'), JSON.stringify({rows, unchanged_source: JSON.stringify(before) === JSON.stringify(hashes())}, null, 2) + '\n');
    console.log(JSON.stringify(row)); assert(assertionVerified, name + ' must satisfy its exact assertion oracle');
  } finally { fs.rmSync(sandbox, {recursive: true, force: true}); }
}
execute('baseline'); for (const mutation of mutations) execute(mutation.name, mutation); execute('restored');
assert.deepEqual(hashes(), before, 'renderer shard mutations must preserve the checkout');
