'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),vm=require('node:vm');
const {PausedRequests,timeoutDiagnostic}=require(process.env.R275_FETCH_HELPER||'../scripts/fetch-paused-diagnostics.cjs');
const {r265API}=require('../scripts/r276-browser-api-fixtures.cjs');
const source=fs.readFileSync(path.join(__dirname,'../scripts/r265-browser.cjs'),'utf8');
const handlerStart=source.indexOf("if(m.method==='Fetch.requestPaused')"),handlerEnd=source.indexOf('\n });\n async function page',handlerStart);
// Capture the existing async handler promise without changing its branches.
const handler=source.slice(handlerStart,handlerEnd).replace('void(async()=>','globalThis.completion=(async()=>');
const lifecycle=source.slice(source.indexOf("if(m.method==='Network.loadingFailed'"),source.indexOf("if(m.method==='Runtime.bindingCalled'"));
function paused(url='http://127.0.0.1:1/api/champion-asset?token=SECRET&key=PRIVATE'){
 let time=0,resolve,reject,method;
 const tracker=new PausedRequests(()=>time);tracker.attach('raw-session','raw-target');
 const context={m:{method:'Fetch.requestPaused',sessionId:'raw-session',params:{requestId:'raw-request',networkId:'raw-network',resourceType:'Image',request:{url,method:'GET',headers:{Cookie:'SECRET',Authorization:'SECRET'}}}},
  r265API,pausedRequests:tracker,ready:{baseUrl:'http://127.0.0.1:1'},URL,Buffer,Promise,Error,closed:new Set(),canceled:new Set(),errors:[],
  send:command=>{method=command;return new Promise((yes,no)=>{resolve=yes;reject=no;});},setTimeout:(done,ms)=>{assert.equal(ms,50);time+=ms;done();}};
 vm.runInNewContext(handler,context);
 return {context,tracker,method:()=>method,advance:ms=>{time+=ms;},resolve:()=>resolve({}),reject:error=>reject(error)};
}
test('R275 actual R265 held fulfill request keeps the original timeout and failure artifact',async t=>{
 const directory=fs.mkdtempSync(path.join(os.tmpdir(),'r275-held-fetch-'));t.after(()=>fs.rmSync(directory,{recursive:true,force:true}));
 const p=paused();assert.equal(p.method(),'Fetch.fulfillRequest');
 try{
  let calls=0,waits=0;
  const start=source.indexOf('const until=async expr=>'),end=source.indexOf('\n  const click=',start);
  const until=vm.runInNewContext(source.slice(start,end)+'until',{pausedRequests:p.tracker,timeoutDiagnostic,JSON,Promise,Error,
   // Model the original diagnostic consumer's virtual window; real watchdog
   // progress/idle behaviour has a separate integrated regression test.
   waitForPage:async(evaluate,expr,options)=>{assert.equal(options.idleMs,10000);assert.equal(options.hardMs,60000);for(let n=0;n<200;n++){if(await evaluate(expr))return true;waits++;p.advance(50);}return false;},evaluate:async()=>++calls<=200?false:{ready:'complete'},setTimeout:(done,ms)=>{assert.equal(ms,50);waits++;p.advance(ms);done();}});
  const error=await until('fixtureReady').then(()=>assert.fail('held request must time out'),error=>error);
  const catchStart=source.indexOf('main().catch(')+'main().catch('.length,catchEnd=source.indexOf(').finally(',catchStart);
  const proc={exitCode:0},writeFailure=vm.runInNewContext('('+source.slice(catchStart,catchEnd)+')',{fs,path,out:directory,pausedRequests:p.tracker,results:[],errors:[],violations:[],JSON,console:{error(){}},captureFailure:undefined,process:proc});
  await writeFailure(error);const artifact=JSON.parse(fs.readFileSync(path.join(directory,'failure.json'),'utf8'));
  assert.equal(proc.exitCode,1);assert.equal(waits,200);assert.equal(calls,201);assert.match(artifact.error,/timeout fixtureReady.*request-1/);
  assert.deepEqual(artifact.pendingFetch,[{id:'request-1',resourceType:'Image',path:'/api/champion-asset',session:'session-1',target:'target-1',release:'Fetch.fulfillRequest',dwellMs:10000}]);
  assert.doesNotMatch(JSON.stringify(artifact),/SECRET|PRIVATE|raw-request|raw-network|raw-session|raw-target|token=|key=|Cookie|Authorization/);
  if(process.env.R275_FETCH_FAILURE_OUTPUT)fs.copyFileSync(path.join(directory,'failure.json'),process.env.R275_FETCH_FAILURE_OUTPUT);
  console.log('R275_HELD_FETCH_FAILURE '+JSON.stringify(artifact));
 }finally{p.resolve();await p.context.completion;}
 assert.deepEqual(p.tracker.snapshot(),[]);
});
test('R275 actual R265 handler clears successful continue, fulfill and fail responses',async()=>{
 for(const [url,method] of [['http://127.0.0.1:1/local.css','Fetch.continueRequest'],['http://127.0.0.1:1/api/champion-asset','Fetch.fulfillRequest'],['https://blocked.example.test/image.png','Fetch.failRequest']]){
  const p=paused(url);assert.equal(p.method(),method);assert.equal(p.tracker.snapshot()[0].release,method);p.resolve();await p.context.completion;assert.deepEqual(p.tracker.snapshot(),[]);assert.deepEqual(p.context.errors,[]);
 }
});
test('R275 actual R265 cancellation and target detach clean pending interception entries',async()=>{
 const canceled=paused();vm.runInNewContext(lifecycle,{...canceled.context,m:{method:'Network.loadingFailed',sessionId:'raw-session',params:{canceled:true,requestId:'raw-network'}}});assert.deepEqual(canceled.tracker.snapshot(),[]);canceled.reject(Error('Invalid InterceptionId'));await canceled.context.completion;assert.deepEqual(canceled.context.errors,[]);
 const closed=paused();vm.runInNewContext(lifecycle,{...closed.context,m:{method:'Target.detachedFromTarget',params:{sessionId:'raw-session'}}});assert.deepEqual(closed.tracker.snapshot(),[]);closed.context.closed.add('raw-session');closed.reject(Error('Target closed'));await closed.context.completion;assert.deepEqual(closed.context.errors,[]);
});
test('R275 actual R265 retains ordinary CDP errors and ignores unrelated cancellations',async()=>{
 const failed=paused();failed.reject(Error('synthetic uncanceled CDP failure'));await failed.context.completion;assert.equal(failed.context.errors.length,1);assert.equal(failed.context.errors[0].message,'synthetic uncanceled CDP failure');assert.equal(failed.tracker.snapshot().length,1);failed.tracker.close('raw-session');
 const other=paused();vm.runInNewContext(lifecycle,{...other.context,m:{method:'Network.loadingFailed',sessionId:'raw-session',params:{canceled:true,requestId:'other-network'}}});assert.equal(other.tracker.snapshot().length,1);other.resolve();await other.context.completion;assert.deepEqual(other.tracker.snapshot(),[]);assert.deepEqual(other.context.errors,[]);
});
