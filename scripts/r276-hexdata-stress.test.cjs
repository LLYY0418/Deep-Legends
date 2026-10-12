'use strict';
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict'),{test}=require('node:test');
const {tests,runStress}=require(process.env.R276_STRESS_MODULE||'./r276-hexdata-stress.cjs');
function fakeGo({forcedStatus=0}={}){
 const calls=[];
 const run=(program,args)=>{
  assert.equal(program,'go');calls.push(args);
  const names=args[args.indexOf('-run')+1].slice(2,-2).split('|');
  // Controlled virtual workload: each test consumes 90 seconds of a 600s
  // package fuse. No wall-clock sleep or disk-speed assumption.
  if(names.length*90>600)return {status:1,stdout:JSON.stringify({Action:'output',Output:'panic: test timed out after 10m0s'})+'\n'+JSON.stringify({Action:'fail',Elapsed:600})+'\n',stderr:''};
  const count=Number(args.find(a=>a.startsWith('-count=')).split('=')[1])*3;
  const events=names.flatMap(Test=>Array.from({length:count},()=>({Action:'pass',Package:'lol-loot-assistant/backend',Test,Elapsed:0.15})));
  events.push({Action:'pass',Package:'lol-loot-assistant/backend',Elapsed:names.length*90});
  return {status:forcedStatus,stdout:events.map(e=>JSON.stringify(e)).join('\n')+'\n',stderr:''};
 };
 return {calls,run};
}
test('per-test stress completes the full workload with the original default fuse',t=>{
 const out=fs.mkdtempSync(path.join(os.tmpdir(),'r276-hexdata-fuse-'));t.after(()=>fs.rmSync(out,{recursive:true,force:true}));
 const fake=fakeGo();let summary;
 assert.doesNotThrow(()=>{summary=runStress(out,fake.run);},'default package fuse must not cover ten unrelated workloads');
 assert.equal(fake.calls.length,10);
 assert.equal(summary.invocations.length,10);
 for(const [i,name] of tests.entries()){
  assert.deepEqual(fake.calls[i],['test','-race','-json','-count=200','-cpu=1,2,4','-run','^('+name+')$','./backend']);
  assert.equal(summary.rows[i].passed,600);assert.equal(summary.rows[i].failed,0);
  const raw=fs.readFileSync(path.join(out,summary.invocations[i].raw),'utf8').trim().split('\n').map(JSON.parse);
  assert.equal(raw.filter(e=>e.Test===name&&e.Action==='pass').length,600);
 }
 const old=fakeGo();const args=['test','-race','-json','-count=200','-cpu=1,2,4','-run','^('+tests.join('|')+')$','./backend'];
 const original=old.run('go',args);assert.equal(original.status,1);assert.match(original.stdout,/test timed out after 10m0s/);
});
test('a failed invocation remains fatal even with complete sample counters',t=>{
 const out=fs.mkdtempSync(path.join(os.tmpdir(),'r276-hexdata-failed-'));t.after(()=>fs.rmSync(out,{recursive:true,force:true}));
 const fake=fakeGo({forcedStatus:2});assert.throws(()=>runStress(out,fake.run),/fixed Hexdata restart stress failed/);
 const summary=JSON.parse(fs.readFileSync(path.join(out,'stress-summary.json'),'utf8'));
 assert.notEqual(summary.exitCode,0);assert.equal(summary.invocations.length,10);assert(summary.invocations.every(i=>i.exitCode===2));
});
