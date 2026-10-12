'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {Census,install,requireClean,failures}=require('./r276-api-census.cjs');
test('API census rejects real 502 even when console guard has no error',()=>{
 const c=new Census();c.event(1,{method:'Network.requestWillBeSent',params:{requestId:'1',request:{url:'http://127.0.0.1:123/api/champions/rankings?mode=ranked',method:'GET'}}});
 c.event(1,{method:'Network.responseReceived',params:{requestId:'1',response:{status:502}}});assert.throws(()=>requireClean(c.report()),/>=400/);assert.equal(c.report().observations[0].path,'/api/champions/rankings');
});
test('only explicit ERR_ABORTED cancellations are recorded without failure',()=>{
 for(const [canceled,errorText,passes] of [[true,'net::ERR_ABORTED',true],[false,'net::ERR_ABORTED',false],[true,'net::ERR_CONNECTION_RESET',false],[false,'net::ERR_CONNECTION_RESET',false]]){
  const c=new Census();c.request('network',1,'s','x','http://localhost/api/status','GET');c.event(1,{sessionId:'s',method:'Network.loadingFailed',params:{requestId:'x',canceled,errorText}});
  if(passes){requireClean(c.report());assert.equal(c.report().canceledCount,1);}else assert.throws(()=>requireClean(c.report()),/failed/);
 }
});
test('HTTP 404/502 and unfinished uncanceled requests fail; canceled 204 needs no exception',()=>{
 for(const status of [404,502]){const c=new Census();c.request('network',1,'s','x','http://localhost/api/image','GET');c.event(1,{sessionId:'s',method:'Network.responseReceived',params:{requestId:'x',response:{status}}});assert.throws(()=>requireClean(c.report()),/>=400/);}
 const c=new Census();c.request('network',1,'s','x','http://localhost/api/status','GET');assert.throws(()=>requireClean(c.report()),/unanswered/);c.event(1,{sessionId:'s',method:'Network.responseReceived',params:{requestId:'x',response:{status:204}}});c.event(1,{sessionId:'s',method:'Network.loadingFailed',params:{requestId:'x',canceled:true,errorText:'net::ERR_ABORTED'}});requireClean(c.report());assert.equal(c.report().canceledCount,1);
});
test('CDP observer enables the real Network domain on every attached target',async()=>{
 class Socket extends EventTarget {constructor(){super();this.sent=[];}send(text){this.sent.push(JSON.parse(text));}}
 const target={WebSocket:Socket},c=install(target),ws=new target.WebSocket('ws://localhost/devtools/browser/test');
 ws.send(JSON.stringify({id:1,method:'Target.attachToTarget',params:{targetId:'page'}}));ws.dispatchEvent(new MessageEvent('message',{data:JSON.stringify({id:1,result:{sessionId:'session'}})}));
 assert.deepEqual(ws.sent.filter(x=>x.id<0).map(x=>x.method),['Network.enable']);assert.equal(c.sessions,1);
 ws.dispatchEvent(new MessageEvent('message',{data:JSON.stringify({sessionId:'session',method:'Network.requestWillBeSent',params:{requestId:'request',request:{url:'http://localhost/api/status',method:'GET'}}})}));
 c.event(1,{sessionId:'session',method:'Network.responseReceived',params:{requestId:'request',response:{status:200}}});ws.send(JSON.stringify({id:2,method:'Target.closeTarget',params:{targetId:'page'}}));await Promise.resolve();ws.dispatchEvent(new MessageEvent('message',{data:JSON.stringify({sessionId:'session',method:'Network.loadingFailed',params:{requestId:'request',canceled:true,errorText:'net::ERR_ABORTED'}})}));requireClean(c.report());
 assert.equal(c.report().canceledCount,1);
});
test('R264 fixture only matches the missing canonical ranked catalog request',()=>{
 const {r264API}=require('./r276-browser-api-fixtures.cjs'),url='http://localhost/api/champions/rankings?mode=ranked&tier=emerald_plus&position=all';assert.equal(r264API(new URL(url)).source,'synthetic');
 for(const other of [url.replace('ranked','arena'),url.replace('emerald_plus','gold'),url.replace('position=all','position=top'),'http://localhost/api/status'])assert.equal(r264API(new URL(other)),undefined);
});

test('census rejects mismatched guard identity',()=>{const c=new Census('r100');assert.throws(()=>requireClean(c.report(),'r264'),/identity mismatch/);});
test('cleanup cannot change an existing non-cancellation network failure',()=>{const c=new Census();c.request('network',1,'s','x','http://localhost/api/status','GET');c.event(1,{sessionId:'s',method:'Network.loadingFailed',params:{requestId:'x',canceled:false,errorText:'net::ERR_CONNECTION_RESET'}});c.cancel(1,'s','target-close');assert.throws(()=>requireClean(c.report()),/failed/);});

test('R264 502 is never an asserted injection',()=>{const c=new Census('r264');c.request('network',1,'s','x','http://localhost/api/champions/rankings','GET');c.event(1,{sessionId:'s',method:'Network.responseReceived',params:{requestId:'x',response:{status:502}}});assert.throws(()=>requireClean(c.report(),'r264'),/>=400/);});

test('explicit ERR_ABORTED cancellation is record-only regardless of received status',()=>{for(const status of [null,204,404,502,503]){const c=new Census('r100');c.request('network',1,'s','x','http://localhost/api/champion-asset','GET');if(status)c.event(1,{sessionId:'s',method:'Network.responseReceived',params:{requestId:'x',response:{status}}});c.event(1,{sessionId:'s',method:'Network.loadingFailed',params:{requestId:'x',canceled:true,errorText:'net::ERR_ABORTED'}});const result=requireClean(c.report(),'r100');assert.equal(result.canceledCount,1);assert.equal(result.otherFailures.length,0);}});

test('R265 exact missing protocol fixtures retain unrelated real requests',()=>{const {r265API}=require('./r276-browser-api-fixtures.cjs');assert(r265API(new URL('http://localhost/api/gameplay/summoner-spells'),'GET').spells.length);assert.deepEqual(r265API(new URL('http://localhost/api/gameplay/match-tiers'),'POST'),{tiers:[]});assert(r265API(new URL('http://localhost/api/champions/rankings?mode=ranked&tier=emerald_plus&position=all'),'GET').rows.length);for(const [path,method] of [['/api/gameplay/match-tiers','GET'],['/api/gameplay/summoner-spells','POST'],['/api/status','GET'],['/api/champions/rankings?mode=arena&tier=emerald_plus&position=all','GET']])assert.equal(r265API(new URL('http://localhost'+path),method),undefined);});
