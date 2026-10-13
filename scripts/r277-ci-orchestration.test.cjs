'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'..'),yaml=require('../desktop/node_modules/js-yaml'),ciText=fs.readFileSync(path.join(root,'.github/workflows/ci.yml'),'utf8'),ci=yaml.load(ciText);
const adapter=require('./r277-ci-orchestrate.cjs');
test('R277 independent jobs leave every legacy shard, timing budget and branch gate unchanged',()=>{
 // Exact CI-only baseline, hashed independently of HEAD and available in shallow CI clones.
 const bytes=fs.readFileSync(path.join(__dirname,'r277-ci-legacy-baseline.json'));assert.equal(require('node:crypto').createHash('sha256').update(bytes).digest('hex'),'d44263b1cd692a9c25bcc1fab52a1f95b77ebca90f66e81ff6a7000b6918d256');
 const baseline=JSON.parse(bytes);assert.equal(baseline.ci_cleanup_sha,'dfd6fc2a830ea999b36b7365f6ea75e12bb06624');const old=baseline.workflow;
 for(const [name,job] of Object.entries(old.jobs))assert.deepEqual(ci.jobs[name],job,`legacy ${name} must not change`);
 assert.deepEqual(ci.on,old.on);assert.equal((ciText.match(/startsWith\(github.ref, 'refs\/heads\/codex\/release-'\)/g)||[]).length,5);
 assert(!ciText.includes("github.ref == 'refs/heads/codex/ci-hardening-r276'"));
});
test('R277 actual mutations are independent steps with always-uploaded original failures',()=>{
 const job=ci.jobs['r277-proof-linux'];assert.equal(job['runs-on'],'ubuntu-latest');
 for(const runner of ['r277-ci-branch-mutations.cjs','r277-client-mutations.py --out','r277-http-refresh-proof.py --out','r277-riot-scope-proof.py --out','r277-wire-diagnostic-mutations.py --out','r277-riot-proof.py --out','r277-ci-orchestrate.cjs frontend-mutants','r277-ci-cn-score.cjs mutants --out','r277-ci-orchestrate.cjs deferred-unit --out'])assert.equal(job.steps.filter(step=>step.run?.includes(runner)).length,1,runner);
 for(const name of ['r277-proof-linux','r277-native-windows','r277-browser-matrix']){
  const current=ci.jobs[name];assert(!current['continue-on-error']);
  for(const step of current.steps){assert(!step['continue-on-error']);assert(!step.run?.includes('|| true'));}
  const upload=current.steps.find(step=>step.uses==='actions/upload-artifact@v4');assert.equal(upload.if,'always()');assert.equal(upload.with['if-no-files-found'],'error');assert(upload.with.path.includes('/history/reports/r277/ci-integration/'));assert(upload.with.path.endsWith('/**'));
 }
});
test('R277 job-level environment uses no unavailable runner context',()=>{
 for(const name of ['r277-proof-linux','r277-native-windows','r277-browser-matrix']){
  for(const value of Object.values(ci.jobs[name].env||{}))assert(!String(value).includes('runner.'),'runner context unavailable in job-level env');
 }
 const source=fs.readFileSync(path.join(__dirname,'r277-ci-orchestrate.cjs'),'utf8');assert(source.includes('process.env.RUNNER_TEMP'));
});
test('Windows P11 is native race/GC proof and newly declared R277 tests are measured',()=>{
 const job=ci.jobs['r277-native-windows'];assert.equal(job['runs-on'],'windows-latest');assert.equal(job.env.GODEBUG,'invalidptr=1');
 assert(job.steps.some(step=>step.run?.includes('r277-installer-callbacks.py --native --out')));
 assert(job.steps.some(step=>step.run?.includes('r277-ci-proof.py --out')));
 assert(job.steps.some(step=>step.name==='Require native Windows race compiler'));
 const helper=fs.readFileSync(path.join(__dirname,'r277-ci-proof.py'),'utf8');assert(helper.includes("proof.run('r277-native-measurements', '^TestR277', race=True)"));assert(helper.includes("terminal.get(test) != 'pass'"));
});
test('Linux and Windows strict browser matrix cannot inherit measure-only or baseline assertions',()=>{
 const job=ci.jobs['r277-browser-matrix'];assert.deepEqual(job.strategy.matrix.os,['ubuntu-latest','windows-latest']);assert.equal(job.env.R277_MEASURE_ONLY,'0');
 for(const mode of ['build-browser','browser','browser-mutants','deferred-browser','cleanup-browser'])assert.equal(job.steps.filter(step=>step.run?.includes(`r277-ci-orchestrate.cjs ${mode} --backend`)).length,1);
 const cleanup=job.steps.find(step=>step.name?.startsWith('Clean only R277'));assert.equal(cleanup.if,'always()');
 const source=fs.readFileSync(path.join(__dirname,'r277-ci-orchestrate.cjs'),'utf8');assert(source.includes("R277_MEASURE_ONLY:'0'"));assert(source.includes('delete env.R266_MATRIX_BASELINE'));
});
test('R277 browser output adapter only changes the report destination, preserving frontend assertions',()=>{
 const command=adapter.browserMutationCommand();assert(command.length>0);
 assert.equal(command.length,1);assert(command[0].endsWith('r277-frontend-browser-mutations.cjs'));
 assert.deepEqual(adapter.parse(['browser','--out','evidence','--backend','fixture']).mode,'browser');assert.throws(()=>adapter.parse(['browser','--out','evidence']),/backend/);assert.throws(()=>adapter.parse(['contracts','--out','evidence','--unknown','x']),/unknown option/);
});

test('R277 Windows performance is actual same-fixture old/new append, record-only without a timing gate',()=>{
 const job=ci.jobs['r277-browser-matrix'],step=job.steps.find(s=>s.run?.includes('r277-perf-compare.cjs'));assert(step);assert.equal(step.if,"matrix.os == 'windows-latest'");assert(step.run.includes('--backend'));assert(!step.run.includes('--chrome'));assert(!step.run.includes('C:/Program Files'));assert(step.run.includes('p9-before-after'));
 const source=fs.readFileSync(path.join(__dirname,'r277-perf-compare.cjs'),'utf8');assert(source.includes('867ca1d7cb627a23af79e6de489a0d4b3c7389ea'));assert(source.includes("record_only:true"));assert(source.includes('fixture_sha256'));assert(source.includes("require('./r277-ci-orchestrate.cjs').chrome(env)"));
 const checks=fs.readFileSync(path.join(__dirname,'r277-perf-browser-checks.cjs'),'utf8');assert(checks.includes("assert.equal(sample.entries,sample.matches"));assert(checks.includes("sample.nodes<=25000"));assert(checks.includes('crypto.subtle.digest'));
});

test('R277 CN proofs require actual HTTP completion and four real mutants, with awaited cleanup',()=>{
 const linux=ci.jobs['r277-proof-linux'],browser=ci.jobs['r277-browser-matrix'];
 assert.equal(linux.steps.filter(s=>s.run?.includes('r277-ci-cn-score.cjs mutants --out')).length,1);
 assert.equal(browser.steps.filter(s=>s.run?.includes('r277-ci-cn-score.cjs browser --backend')).length,1);
 const source=fs.readFileSync(path.join(__dirname,'r277-ci-cn-score.cjs'),'utf8');
 for(const guard of ["assert.equal(result.results.length,4)","assert.equal(proofs.length,2", "assert.equal(exit.code,0", "ready+'.done'", "await require('./chrome-test-cleanup.cjs').stopChild(child)"])assert(source.includes(guard),guard);
 assert(!source.includes('child.kill('));
 const command=require('./r277-ci-cn-score.cjs').mutationCommand();assert.equal(command.length,1);assert(command[0].endsWith('r277-frontend-cn-score-mutations.cjs'));
});
