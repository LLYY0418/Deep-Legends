'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {planExecution,makespan}=require('./lib/preflight-cost.cjs');
function sample(extra={}){return {changed:[{file:'backend/web/app.js',status:'M'}],nodeTests:[{file:'slow.test.cjs'},{file:'other.test.cjs'}],goTestFiles:[],goTestNames:[],browserGuards:[],broad:false,...extra};}
test('cost share catches a small file count that contains most measured work',()=>{
 const plan=planExecution(sample(),{costs:{'slow.test.cjs':200000,'other.test.cjs':200000,'unrelated.test.cjs':1000}});assert(plan.deferAssociated);assert.equal(plan.nodeTests.length,0);assert.equal(plan.deferred.nodeTests.length,2);assert(plan.estimate.nodeFileCostShare>0.99);
});
test('directly changed tests still execute while associated expensive work is deferred',()=>{
 const s=sample({changed:[{file:'slow.test.cjs',status:'M'},{file:'backend/direct_test.go',status:'M'}],goTestFiles:[{file:'backend/direct_test.go'},{file:'backend/associated_test.go'}],goTestNames:['TestDirect','TestOther']});
 const p=planExecution(s,{costs:{'slow.test.cjs':200000,'other.test.cjs':200000},goNamesByFile:new Map([['backend/direct_test.go',['TestDirect']]])});assert.deepEqual(p.nodeTests.map(x=>x.file),['slow.test.cjs']);assert.deepEqual(p.goTestNames,['TestDirect']);assert.deepEqual(p.deferred.goTestNames,['TestOther']);
});
test('large unprofiled Go scope defers; it cannot hide behind a zero Node ratio',()=>{
 const p=planExecution(sample({nodeTests:[],goTestNames:Array.from({length:100},(_,i)=>'Test'+i)}),{costs:{}});assert(p.deferAssociated);assert.equal(p.estimate.goDurationMs,null);assert.equal(p.goTestNames.length,0);
});
test('browser request is explicit and expensive work needs the full override',()=>{
 const s=sample({nodeTests:[],browserGuards:[{name:'r266'},{name:'r275-offline'}]});const p=planExecution(s,{costs:{},browsers:true});assert(p.deferBrowser);assert.equal(p.deferred.browserGuards.length,2);assert.equal(planExecution(s,{costs:{},browsers:true,full:true}).deferBrowser,false);
 assert.equal(planExecution(sample({nodeTests:[]}),{costs:{}}).deferAssociated,false);
});
test('bounded worker prediction and unknown costs are explicit; full keeps original selection',()=>{
 assert.equal(makespan([{ms:8},{ms:7},{ms:2}],2),9);const s=sample();const p=planExecution(s,{costs:{},full:true});assert.deepEqual(p.nodeTests,s.nodeTests);assert.equal(p.estimate.unknownNodeFiles.length,2);assert(p.estimate.warning.includes('prediction'));
});
test('mutation: ignoring cost, dropping direct regressions, or running expensive browsers breaks safeguards',()=>{
 const costs={'slow.test.cjs':200000,'other.test.cjs':200000};const p=planExecution(sample({changed:[{file:'slow.test.cjs',status:'M'}],browserGuards:[{name:'r266'},{name:'r275-offline'}]}),{costs,browsers:true});
 function verify(x){assert(x.deferAssociated);assert(x.deferBrowser);assert.deepEqual(x.nodeTests.map(r=>r.file),['slow.test.cjs']);assert.equal(x.deferred.nodeTests.length,1);}
 verify(p);for(const mutate of [x=>x.deferAssociated=false,x=>x.deferBrowser=false,x=>x.nodeTests=[]]){const x=structuredClone(p);mutate(x);assert.throws(()=>verify(x),{name:'AssertionError'});}
});

test('unknown associated cost is explicit and deferred while unknown changed tests remain mandatory',()=>{
 const s=sample({changed:[{file:'slow.test.cjs',status:'M'}]});const p=planExecution(s,{costs:{}});assert(p.deferAssociated);assert(p.reasons.some(r=>r.includes('unknown')));assert.deepEqual(p.nodeTests.map(row=>row.file),['slow.test.cjs']);assert.deepEqual(p.estimate.unknownNodeFiles,['slow.test.cjs','other.test.cjs']);
});
