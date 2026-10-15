'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {stagingIssues}=require('./r279-staged-gates.cjs'),{timingComparison,runnerMetadata}=require('./lib/runner-diagnostics.cjs');
const {staticGates}=require('./preflight.cjs'),{buildIndex,select}=require('./lib/preflight-select.cjs');
test('new and edited tests must be explicitly staged; no implicit git add',()=>{
 assert.equal(stagingIssues({changed:['scripts/new.test.cjs'],staged:[],unstaged:[]}).length,1);
 assert.equal(stagingIssues({changed:['scripts/new.test.cjs'],staged:['scripts/new.test.cjs'],unstaged:['scripts/new.test.cjs']}).length,1);
 assert.deepEqual(stagingIssues({changed:['scripts/new.test.cjs'],staged:['scripts/new.test.cjs'],unstaged:[]}),[]);
});
test('static gate order and pinned checks precede test execution',()=>{
 const gates=staticGates({changed:[],staticGates:['pinned-contracts']},'HEAD');assert.equal(gates[0].args[0],'scripts/r279-staged-gates.cjs');assert(gates.some(g=>g.args.includes('scripts/pinned-contracts-check.cjs')));assert(gates.every(g=>g.command!=='sh'));
});
test('uniform 1.46 slowdown is recorded as distribution, single 3x file is named',()=>{
 const reference={a:10,b:10,c:10,d:10,e:10};const uniform=timingComparison(Object.keys(reference).map(file=>({file,duration_ms:14.6})),reference);assert.equal(uniform.kind,'uniform-slowdown');assert.equal(uniform.outliers.length,0);
 const single=timingComparison(Object.keys(reference).map(file=>({file,duration_ms:file==='a'?30:10})),reference);assert.equal(single.kind,'file-outliers');assert.equal(single.outliers[0].file,'a');assert(runnerMetadata().cpu_model);
});
test('mixed layout/settings warning is advisory; tests remain selected',()=>{
 const files=['backend/web/app.js','backend/web/app.css'];const index=buildIndex({files,read:()=>'',guards:['r117']});const selection=select(index,files);assert(selection.scopeWarnings.length);assert(selection.browserGuards.some(g=>g.name==='r277-strict'));
});
test('legacy printed renderer JSON reaches the triage summary without changing its job',()=>{
 const data={scope:'all',runner:{cpu_model:'fixture CPU'},timingComparison:{kind:'uniform-slowdown',medianRatio:1.46,outliers:[]},files:[]};
 const log='other output\n'+JSON.stringify(data,null,2)+'\nremaining log';
 const {timingReportsFromLog}=require('./lib/runner-diagnostics.cjs');assert.deepEqual(timingReportsFromLog(log),[data]);
 const {triage,markdown}=require('./ci-failure-triage.cjs');const report=triage({log:['legacy.log'],changed:null},{text:()=>log});assert.equal(report.runner.cpu_model,'fixture CPU');assert.match(markdown(report),/均匀变慢/);
});
