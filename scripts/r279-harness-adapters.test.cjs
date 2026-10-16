'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),Module=require('node:module');
const root=path.resolve(__dirname,'..');
// Execute each real source adapter, but parse its final harness instead of
// launching Chromium. The nested deferred adapter executes both real layers.
function compileAdapter(file,env={},mutate=s=>s){
 let compiled=0;
 const filename=path.join(root,'scripts',file),read=file=>{const source=fs.readFileSync(file,'utf8');return file.endsWith(path.sep+'r266-browser.cjs')?mutate(source):source;};
 const localFS={...fs,readFileSync:(file,encoding)=>encoding==='utf8'?read(String(file)):fs.readFileSync(file,encoding)};
 class CapturedModule{static _nodeModulePaths=Module._nodeModulePaths;constructor(name){this.filename=name;this.paths=[];this.exports={};}_compile(source,name){
  if(source.includes("let source=fs.readFileSync(path.join(__dirname,'r266-browser.cjs')"))execute(source,name,this);
  else {new vm.Script(source,{filename:name});compiled++;}
 }}
 function execute(source,name,module={exports:{},paths:[]}){
  const req=id=>id==='node:module'?CapturedModule:id==='node:fs'?localFS:Module.createRequire(name)(id);
  const run=vm.runInNewContext(Module.wrap(source),{process:{env,cwd:()=>root},console:{log(){}}},{filename:name});
  run(module.exports,req,module,name,path.dirname(name));
 }
 execute(read(filename),filename);assert.equal(compiled,1,'exactly one final harness must compile');
}
for(const [file,env]of [['r277-frontend-browser.cjs',{}],['r277-deferred-assets-browser.cjs',{}],['r277-deferred-assets-browser.cjs',{R277_DEFERRED_FAULT:'503'}],['r277-perf-browser.cjs',{}],['r279-startup-browser.cjs',{}]])
 test('real adapter anchors and transformed syntax '+file+' '+JSON.stringify(env),()=>compileAdapter(file,env));
test('mutation: breaking the legacy declaration must fail the actual deferred 503 adapter',()=>{
 assert.throws(()=>compileAdapter('r277-deferred-assets-browser.cjs',{R277_DEFERRED_FAULT:'503'},s=>s.replace('let currentCase,clickEvidence;','let currentCase,clickEvidence,extra;')),/Unexpected browser harness anchor: let currentCase,clickEvidence;/);
});

// Exercise the extracted consumer with the real shared watchdog and a virtual
// clock as well, so the compatibility stubs above cannot mask broken wiring.
test('real R265 page watchdog retains held-fetch diagnostics after exactly 10s idle',async()=>{
 const {waitForPage,PAGE_PROGRESS_EXPRESSION}=require('./lib/wait-progress.cjs');
 const {PausedRequests,timeoutDiagnostic}=require('./fetch-paused-diagnostics.cjs');
 let time=0,polls=0,sleeps=0;const tracker=new PausedRequests(()=>time);tracker.attach('raw-session','raw-target');
 tracker.enter({requestId:'raw-request',networkId:'raw-request',resourceType:'Script',request:{url:'http://localhost/demo-data.js?token=SECRET'}},'raw-session');
 const source=fs.readFileSync(path.join(__dirname,'r265-browser.cjs'),'utf8'),start=source.indexOf('const until=async expr=>'),end=source.indexOf('\n  const click=',start);
 const evaluate=async expr=>{if(expr===PAGE_PROGRESS_EXPRESSION)return {ready:'complete'};if(expr==='fixtureReady'){polls++;return false;}return {ready:'complete'};};
 const until=vm.runInNewContext(source.slice(start,end)+'until',{pausedRequests:tracker,timeoutDiagnostic,JSON,Promise,Error,evaluate,
  waitForPage:(evaluate,expr,options)=>waitForPage(evaluate,expr,{...options,now:()=>time,sleep:async ms=>{assert.equal(ms,50);time+=ms;sleeps++;}})});
 await assert.rejects(until('fixtureReady'),error=>{assert.match(error.message,/timeout fixtureReady/);assert.match(error.message,/demo-data.js/);assert.match(error.message,/"dwellMs":10000/);assert.doesNotMatch(error.message,/SECRET|raw-request|raw-session|raw-target/);return true;});
 assert.equal(time,10000);assert.equal(sleeps,200);assert.equal(polls,201,'shared watchdog rechecks the condition at the idle boundary');
});
