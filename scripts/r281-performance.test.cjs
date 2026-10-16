"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const {evidence,SIGNATURE}=require('./lib/performance-failure.cjs'),{triage,classify}=require('./ci-failure-triage.cjs');
const source=fs.readFileSync(require.resolve('./r266-browser.cjs'),'utf8');
for(const [name,variable,limit] of [['click long tasks','clickTasks',100],['collection long task','collectionMax',200],['five filter cycles long tasks','cycleTasks',1000]])test(name+' keeps the exact boundary and reports operation/task/CPU evidence',()=>{
 const assertion=source.match(new RegExp('assert\\('+variable+'<='+limit+",'[^']+'\\+"+variable+'\\)'));assert(assertion,'original threshold assertion must remain');
 vm.runInNewContext(assertion[0],{assert,[variable]:limit});assert.throws(()=>vm.runInNewContext(assertion[0],{assert,[variable]:limit+1}),{code:'ERR_ASSERTION'});
 assert.equal(evidence({name,value:limit,limit,tasks:[]}),null);
 const failure=evidence({name,value:limit+1,limit,tasks:[{start:10,duration:limit+1}],operations:[{start:5,end:20,operation:'select hero',iteration:3}],iteration:2,cpu:'CPU fixture'});
 assert.equal(failure.signature,SIGNATURE);assert.equal(failure.cpu,'CPU fixture');assert.deepEqual(failure.tasks,[{start:10,duration:limit+1,operation:'select hero',iteration:3}]);
 const report=triage({json:['fixture/failure.json'],changed:new Set()}, {json:()=>({error:assertion[0],signature:SIGNATURE,file:'scripts/r266-browser.cjs',performanceFailures:[failure]})});
 assert.equal(report.failures[0].category,'performance');assert.deepEqual(report.failures[0].performanceFailures,[failure]);assert.equal(report.failures[0].inDiff,false);
});
test('historical 112/126ms click failures have the performance signature without changing rerun policy',()=>{for(const ms of [112,126])assert.equal(classify('AssertionError: click long tasks '+ms),'performance');assert.equal(classify('WAIT_STALLED timeout'),'wait-timeout');});
