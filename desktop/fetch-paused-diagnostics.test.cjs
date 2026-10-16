'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {PausedRequests,timeoutDiagnostic,safePath}=require(process.env.R275_FETCH_HELPER||'../scripts/fetch-paused-diagnostics.cjs');
const params=(id='raw-request')=>({requestId:id,networkId:id,resourceType:'Script',request:{url:'http://localhost/demo-data.js?token=SECRET&key=PRIVATE',headers:{Authorization:'SECRET',Cookie:'SECRET'}}});
test('R275 held Fetch request fails the original page wait and names only safe diagnostics',async()=>{
 let time=0;const tracker=new PausedRequests(()=>time);tracker.attach('raw-session','raw-target');tracker.enter(params(),'raw-session');
 const source=fs.readFileSync(path.join(__dirname,'../scripts/r265-browser.cjs'),'utf8'),start=source.indexOf('const until=async expr=>'),end=source.indexOf('\n  const click=',start);
 let evaluates=0,waits=0;
 const until=vm.runInNewContext(source.slice(start,end)+'until',{pausedRequests:tracker,timeoutDiagnostic,JSON,Promise,Error,
  // This consumer fixture retains its original 200 virtual polls; the real shared
  // watchdog is exercised separately in r279-harness-adapters.test.cjs.
  waitForPage:async(evaluate,expr,options)=>{assert.equal(options.idleMs,10000);assert.equal(options.hardMs,60000);for(let n=0;n<200;n++){if(await evaluate(expr))return true;waits++;time+=50;}return false;},evaluate:async()=>{evaluates++;return evaluates<=200?false:{ready:'complete'};},setTimeout:(done,ms)=>{assert.equal(ms,50);waits++;time+=ms;done();}});
 await assert.rejects(until('fixtureReady'),error=>{assert.match(error.message,/timeout fixtureReady/);assert.match(error.message,/request-1/);assert.match(error.message,/demo-data.js/);assert.match(error.message,/"dwellMs":10000/);assert.match(error.message,/"release":"not-started"/);assert.match(error.message,/session-1/);assert.match(error.message,/target-1/);assert.doesNotMatch(error.message,/SECRET|PRIVATE|raw-request|raw-session|raw-target|token=|key=/);return true;});
 assert.equal(waits,200);assert.equal(evaluates,201);
});
test('R275 completed continue, fulfill and fail requests leave no pending entry',()=>{
 const tracker=new PausedRequests(()=>20);tracker.attach('s','t');
 for(const method of ['Fetch.continueRequest','Fetch.fulfillRequest','Fetch.failRequest']){const key=tracker.enter(params(method),'s');tracker.release(key,method);assert.equal(tracker.snapshot()[0].release,method);tracker.complete(key);assert.deepEqual(tracker.snapshot(),[]);}
});
test('R275 target close and canceled network request clear only their own page',()=>{
 const tracker=new PausedRequests(()=>0);tracker.attach('s1','t1');tracker.attach('s2','t2');tracker.enter(params('one'),'s1');tracker.enter(params('two'),'s2');tracker.cancel('s1','one');assert.equal(tracker.snapshot().length,1);assert.equal(tracker.snapshot()[0].session,'session-2');tracker.enter(params('three'),'s1');tracker.close('s1');assert.equal(tracker.snapshot().length,1);tracker.close('s2');assert.deepEqual(tracker.snapshot(),[]);tracker.attach('s3','t3');tracker.enter(params('four'),'s3');assert.equal(tracker.snapshot()[0].session,'session-3');
 for(const url of ['file:///Users/person/private/key','https://example.test/person.js','https://example.test/api/player/PUUID?token=SECRET','https://example.test/Users/person/private','data:text/plain,SECRET'])assert.doesNotMatch(safePath(url),/person|PUUID|SECRET|key|Users/);
});
test('R275 R265 tracks entry, release responses, close/cancel and failure artifacts',()=>{
 const source=fs.readFileSync(path.join(__dirname,'../scripts/r265-browser.cjs'),'utf8');
 for(const marker of ['pausedRequests.enter(m.params,m.sessionId)','pausedRequests.release(diagnosticKey,method)','await send(method,params,m.sessionId);pausedRequests.complete(diagnosticKey)','pausedRequests.cancel(m.sessionId,m.params.requestId)','pausedRequests.close(sessionId)','pendingFetch:pausedRequests.snapshot()'])assert.ok(source.includes(marker),marker);
});
