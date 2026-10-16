'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const checks=require('./r272-browser-checks.cjs');
async function fixture({max=217,diagnosticFails=false}={}){
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r272-perf-diagnostics-')),file=path.join(dir,'fixture.json'),prior=process.env.R272_OVERVIEW_FIXTURE,results=[],stop=new Error('stop after original passing case');
 fs.writeFileSync(file,JSON.stringify({matches:Array.from({length:20},()=>({revision:'fixture',participants:Array.from({length:10},()=>({}))}))}));process.env.R272_OVERVIEW_FIXTURE=file;
 let snapshots=0,error;
 const sample={sample:'after-failure',startedAt:100,observedAt:1800,tasks:[{start:1600,duration:217,offset:1500}],readyState:'complete',fontStatus:'loaded'};
 try{await checks({results,until:async()=>true,shot:async()=>{},evaluate:async expression=>{
  if(expression==='__r272Samples')return Array.from({length:4},()=>({text:'loading',pending:1,at:800}));
  if(expression.includes('const tasks=__r272Tasks.filter'))return {total:290,max,requests:[{cards:'1',accept:'application/x-ndjson'}]};
  if(expression.includes("sample:'after-failure'")){snapshots++;if(diagnosticFails)throw new Error('CDP evidence unavailable');return sample;}
  if(expression==='window.__r272Start=0')throw stop;
  return true;
 }});}catch(e){error=e;}finally{if(prior===undefined)delete process.env.R272_OVERVIEW_FIXTURE;else process.env.R272_OVERVIEW_FIXTURE=prior;fs.rmSync(dir,{recursive:true,force:true});}
 return {error,results,snapshots,sample,stop};
}
test('R272 original 200ms failure retains the exact error and records task timing after failure',async()=>{
 const f=await fixture();assert.equal(f.error?.code,'ERR_ASSERTION');assert.match(f.error.message,/"max":217/);assert.equal(f.snapshots,1);assert.equal(f.results.length,1);assert.equal(f.results[0].r272,'cards-performance-failure');assert.equal(f.results[0].theme,'dark');assert.equal(f.results[0].region,'');assert.deepEqual(f.results[0].snapshot,f.sample);assert.equal(f.results[0].errorCode,'ERR_ASSERTION');
});
test('R272 a failed diagnostic request cannot replace or swallow the original performance failure',async()=>{
 const f=await fixture({diagnosticFails:true});assert.equal(f.error?.code,'ERR_ASSERTION');assert.match(f.error.message,/"max":217/);assert.equal(f.results[0].snapshot.sample,'unavailable');assert.match(f.results[0].snapshot.error,/CDP evidence unavailable/);
});
test('R272 passing performance path does not add a measurement or diagnostic call',async()=>{
 const f=await fixture({max:100});assert.equal(f.error,f.stop);assert.equal(f.snapshots,0);assert.equal(f.results.length,1);assert.equal(f.results[0].r272,'cards-real-decoder');assert.equal(f.results[0].perf.max,100);
});
