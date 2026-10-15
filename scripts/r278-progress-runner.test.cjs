'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{EventEmitter}=require('node:events');
const {runProgressChild}=require(process.env.R278_PROGRESS_CHILD_SOURCE||'./lib/progress-child.cjs');
function fixture(){
 const directory=fs.mkdtempSync(path.join(os.tmpdir(),'r278-progress-test-')),timers=new Map(),child=new EventEmitter();child.stdout=new EventEmitter();child.stderr=new EventEmitter();let time=0,sequence=0,watch,closed=false,stopped=false;
 const clock={now:()=>time,setTimeout(fn,ms){const id=++sequence;timers.set(id,{at:time+ms,fn});return id;},clearTimeout:id=>timers.delete(id)};
 const tick=ms=>{const end=time+ms;for(;;){const next=[...timers].sort((a,b)=>a[1].at-b[1].at)[0];if(!next||next[1].at>end)break;time=next[1].at;timers.delete(next[0]);next[1].fn();}time=end;};
 const promise=runProgressChild({command:'controlled-child',directory,logFile:path.join(directory,'browser-mutants.log'),ids:['p7-compact-floor'],idleMs:10,totalMs:100,clock,spawnChild:()=>child,watchDirectory:(_d,_opts,fn)=>{watch=fn;return {close(){closed=true;}};},stop:async()=>{stopped=true;child.emit('close',null,'SIGTERM');},writeOutput(){},writeError(){}});promise.catch(()=>{});
 return {directory,child,promise,tick,watch:()=>watch(),get stopped(){return stopped;},get closed(){return closed;},cleanup(){fs.rmSync(directory,{recursive:true,force:true});}};
}
test('R278 child with continuous stdout survives the replaced fixed timeout window',async()=>{
 const f=fixture();try{for(let i=0;i<9;i++){f.tick(8);f.child.stdout.emit('data',Buffer.from('progress\n'));}f.child.emit('close',0,null);const r=await f.promise;assert.equal(r.exit,0);assert.equal(r.progress.elapsedMs,72);assert.equal(f.stopped,false);assert.equal(f.closed,true);}finally{f.cleanup();}
});
test('R278 child without progress fails at the idle window with every mutant state',async()=>{
 const f=fixture();try{f.tick(10);assert.equal(f.stopped,true,'idle child must fail within its progress window');await assert.rejects(f.promise,error=>{assert.match(error.message,/no progress for 10ms/);assert.equal(error.progressSnapshot.idleMs,10);assert.deepEqual(error.progressSnapshot.mutants,[{id:'p7-compact-floor',state:'pending'}]);return true;});assert.equal(f.stopped,true);assert.equal(f.closed,true);}finally{f.cleanup();}
});
test('R278 new result file is progress but cannot indefinitely preserve an idle child',async()=>{
 const f=fixture();try{f.tick(8);fs.writeFileSync(path.join(f.directory,'browser-mutant-p7-compact-floor-result.json'),JSON.stringify({error:'compact region must show two characters'}));f.watch();f.tick(8);assert.equal(f.stopped,false);f.tick(2);await assert.rejects(f.promise,error=>{assert.equal(error.progressSnapshot.lastProgress.kind,'result-files');assert.equal(error.progressSnapshot.mutants[0].state,'failure-recorded-awaiting-target-verification');return true;});}finally{f.cleanup();}
});
test('R278 runner log and stderr do not masquerade as mutant progress',async()=>{
 const f=fixture();try{f.tick(8);f.child.stderr.emit('data','diagnostic\n');f.watch();f.tick(2);await assert.rejects(f.promise,/no progress for 10ms/);}finally{f.cleanup();}
});
test('R278 progressing child still fails at the independent total ceiling',async()=>{
 const f=fixture();try{for(let i=0;i<12;i++){f.tick(8);f.child.stdout.emit('data','p');}f.tick(4);await assert.rejects(f.promise,/total limit 100ms/);assert.equal(f.stopped,true);}finally{f.cleanup();}
});
test('R278 nonzero mutant child remains a failure even after recorded progress',async()=>{
 const f=fixture();try{f.child.stdout.emit('data','p');f.child.emit('close',1,null);const r=await f.promise;assert.equal(r.exit,1);assert.equal(f.stopped,false);}finally{f.cleanup();}
});
test('R278 actual asynchronous child records stdout and final target-verified mutation state',async()=>{
 const directory=fs.mkdtempSync(path.join(os.tmpdir(),'r278-live-progress-'));
 try{
  const script="const fs=require('node:fs'),path=require('node:path');console.log('controlled progress');fs.writeFileSync(path.join(process.argv[1],'browser-mutations-result.json'),JSON.stringify({results:[{id:'fixture',status:'killed'}]}));";
  const r=await runProgressChild({command:process.execPath,args:['-e',script,directory],directory,logFile:path.join(directory,'browser-mutants.log'),ids:['fixture'],writeOutput(){},writeError(){}});
  assert.equal(r.exit,0);assert(r.progress.stdoutBytes>0);assert.deepEqual(r.progress.mutants,[{id:'fixture',state:'killed'}]);
 }finally{fs.rmSync(directory,{recursive:true,force:true});}
});
