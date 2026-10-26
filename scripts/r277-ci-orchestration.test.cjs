'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'..'),yaml=require('../desktop/node_modules/js-yaml'),ciText=fs.readFileSync(path.join(root,'.github/workflows/ci.yml'),'utf8'),ci=yaml.load(ciText);
const adapter=require('./r277-ci-orchestrate.cjs');
test('R287 approved CI snapshot pins every reassigned proof and preserves product budgets',()=>{
 const bytes=fs.readFileSync(path.join(__dirname,'r277-ci-legacy-baseline.json'));
 assert.equal(require('node:crypto').createHash('sha256').update(bytes).digest('hex'),require('./pinned-contracts-registry.json').legacyBaselineSha);
 const baseline=JSON.parse(bytes);assert.match(baseline.approval,/R287.*approved/);
 assert.equal(baseline.ci_cleanup_sha,'dfd6fc2a830ea999b36b7365f6ea75e12bb06624');
 assert.deepEqual(ci,require('./renderer-shard-contract.cjs').approvedWorkflow(baseline.workflow),'R287 approved exact CI job definitions plus the user-authorized R309 renderer-only delta must remain pinned');
 assert(!ciText.includes('codex/probe-0.12.85-'));
 require('./lib/ci-contracts.cjs').verifyMutationRouting(ci);
});
test('R277 actual mutations are independent steps with always-uploaded original failures',()=>{
 const job={steps:['r277-proof-linux-1','r277-proof-linux-2'].flatMap(n=>ci.jobs[n].steps)};for(const n of ['r277-proof-linux-1','r277-proof-linux-2'])assert.equal(ci.jobs[n]['runs-on'],'ubuntu-24.04');
 for(const runner of ['r277-ci-branch-mutations.cjs','r277-client-mutations.py --out','r277-http-refresh-proof.py --out','r277-riot-scope-proof.py --out','r277-wire-diagnostic-mutations.py --out','r277-riot-proof.py --out','r277-ci-orchestrate.cjs frontend-mutants','r277-ci-cn-score.cjs mutants --out','r277-ci-orchestrate.cjs deferred-unit --out'])assert.equal(job.steps.filter(step=>step.run?.includes(runner)).length,1,runner);
 for(const name of ['r277-proof-linux-1','r277-proof-linux-2','windows-build','r277-browser-matrix','r277-browser-mutations']){
  const current=ci.jobs[name];assert(!current['continue-on-error']);
  for(const step of current.steps){assert(!step['continue-on-error']);assert(!step.run?.includes('|| true'));}
  const upload=current.steps.find(step=>step.uses==='actions/upload-artifact@v4'&&String(step.with?.path).includes('/history/reports/r277/ci-integration/'));assert.equal(upload.if,'always()');assert.equal(upload.with['if-no-files-found'],'error');assert(upload.with.path.includes('/history/reports/r277/ci-integration/'));assert(upload.with.path.endsWith('/**'));
 }
});
test('R277 job-level environment uses no unavailable runner context',()=>{
 for(const name of ['r277-proof-linux-1','r277-proof-linux-2','windows-build','r277-browser-matrix','r277-browser-mutations']){
  for(const value of Object.values(ci.jobs[name].env||{}))assert(!String(value).includes('runner.'),'runner context unavailable in job-level env');
 }
 const source=fs.readFileSync(path.join(__dirname,'r277-ci-orchestrate.cjs'),'utf8');assert(source.includes('process.env.RUNNER_TEMP'));
});
test('R277 Python proof I/O works with simulated Windows cp1252 defaults',()=>{
 const result=require('node:child_process').spawnSync(process.platform==='win32'?'python':'python3',[path.join(__dirname,'r277-python-encoding.test.py'),'-v'],{cwd:root,encoding:'utf8'});assert.equal(result.status,0,(result.stdout||'')+(result.stderr||''));
});
test('Windows P11 is native race/GC proof and newly declared R277 tests are measured',()=>{
 const job=ci.jobs['windows-build'];assert.equal(job['runs-on'],'windows-latest');assert.equal(job.steps.find(s=>s.name==='Measure every native R277 Go test including reopen and L5').env.GODEBUG,'invalidptr=1');
 assert(job.steps.some(step=>step.run?.includes('r277-installer-callbacks.py --native --out')));
 assert(job.steps.some(step=>step.run?.includes('r277-ci-proof.py --out')));
 assert(job.steps.some(step=>step.name==='Require native Windows race compiler'));
 const callbacks=fs.readFileSync(path.join(__dirname,'r277-installer-callbacks.py'),'utf8');assert(callbacks.includes('invalid pointer found on stack'));assert(callbacks.includes('native_fixed("restored-native-race", cwd=copy)'));assert(callbacks.includes('output.count(marker) != 2'));
 const fixture=fs.readFileSync(path.join(root,'installer/internal/p11callbacktest/fixture_windows.go'),'utf8');assert(fixture.includes('SetWindowLongPtrW'));assert(fixture.includes('default_reentry=1 default_gc=1'));assert(fixture.includes('syscall.SyscallN(callback, child, 0x0020, child, 0x60)'));
 const helper=fs.readFileSync(path.join(__dirname,'r277-ci-proof.py'),'utf8');assert(helper.includes("proof.run('r277-native-measurements', '^TestR277', race=True)"));assert(helper.includes("terminal.get(test) != 'pass'"));
});
test('Linux and Windows strict browser matrix cannot inherit measure-only or baseline assertions',()=>{
 const job=ci.jobs['r277-browser-matrix'];assert.deepEqual(job.strategy.matrix.os,['ubuntu-latest','windows-latest']);assert.equal(job.env.R277_MEASURE_ONLY,'0');
 for(const mode of ['build-browser','browser','deferred-browser','cleanup-browser'])assert.equal(job.steps.filter(step=>step.run?.includes(`r277-ci-orchestrate.cjs ${mode} --backend`)).length,1);
 assert(!job.steps.some(s=>s.run?.includes('browser-mutants')));
 const mutation=ci.jobs['r277-browser-mutations'];assert.deepEqual(mutation.strategy.matrix,{os:['ubuntu-latest','windows-latest'],shard:[1,2,3]});
 assert(mutation.steps.some(s=>s.run?.includes('browser-mutants --shard ${{ matrix.shard }}')));
 const cleanup=job.steps.find(step=>step.name?.startsWith('Clean only R277'));assert.equal(cleanup.if,'always()');
 const source=fs.readFileSync(path.join(__dirname,'r277-ci-orchestrate.cjs'),'utf8');assert(source.includes("R277_MEASURE_ONLY:'0'"));assert(source.includes('delete env.R266_MATRIX_BASELINE'));
});
test('R277 browser output adapter only changes the report destination, preserving frontend assertions',()=>{
 const command=adapter.browserMutationCommand();assert(command.length>0);
 assert.equal(command.length,1);assert(command[0].endsWith('r277-frontend-browser-mutations.cjs'));
 assert.deepEqual(adapter.parse(['browser','--out','evidence','--backend','fixture']).mode,'browser');assert.throws(()=>adapter.parse(['browser','--out','evidence']),/backend/);assert.throws(()=>adapter.parse(['contracts','--out','evidence','--unknown','x']),/unknown option/);
});

test('R277 Windows performance is actual same-fixture old/new append, record-only without a timing gate',()=>{
 const job=ci.jobs['r277-browser-matrix'],step=job.steps.find(s=>s.run?.includes('r277-perf-compare.cjs'));assert(step);assert.equal(step.if,"${{ !cancelled() && steps.fixture_build.outcome == 'success' && matrix.os == 'windows-latest' }}");assert(step.run.includes('--backend'));assert(!step.run.includes('--chrome'));assert(!step.run.includes('C:/Program Files'));assert(step.run.includes('p9-before-after'));
 const source=fs.readFileSync(path.join(__dirname,'r277-perf-compare.cjs'),'utf8');assert(source.includes('867ca1d7cb627a23af79e6de489a0d4b3c7389ea'));assert(source.includes("record_only:true"));assert(source.includes('fixture_sha256'));assert(source.includes("require('./r277-ci-orchestrate.cjs').chrome(env)"));
 const checks=fs.readFileSync(path.join(__dirname,'r277-perf-browser-checks.cjs'),'utf8');assert(checks.includes("assert.equal(sample.entries,sample.matches"));assert(checks.includes("sample.nodes<=25000"));assert(checks.includes('crypto.subtle.digest'));
});

test('R277 CN proofs require actual HTTP completion and four real mutants, with awaited cleanup',()=>{
 const linux={steps:['r277-proof-linux-1','r277-proof-linux-2'].flatMap(n=>ci.jobs[n].steps)},browser=ci.jobs['r277-browser-matrix'];
 assert.equal(linux.steps.filter(s=>s.run?.includes('r277-ci-cn-score.cjs mutants --out')).length,1);
 assert.equal(browser.steps.filter(s=>s.run?.includes('r277-ci-cn-score.cjs browser --backend')).length,1);
 const source=fs.readFileSync(path.join(__dirname,'r277-ci-cn-score.cjs'),'utf8');
 for(const guard of ["assert.equal(result.results.length,4)","assert.equal(proofs.length,2", "assert.equal(exit.code,0", "ready+'.done'", "await require('./chrome-test-cleanup.cjs').stopChild(child)"])assert(source.includes(guard),guard);
 assert(!source.includes('child.kill('));
 const command=require('./r277-ci-cn-score.cjs').mutationCommand();assert.equal(command.length,1);assert(command[0].endsWith('r277-frontend-cn-score-mutations.cjs'));
});
