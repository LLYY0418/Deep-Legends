'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm');
const {waitForFollowedClient,assertReadControls,assertRestartControls,followCapturePrelude}=require(process.env.R278_FOLLOW_SOURCE||'./r265-persisted-ui.cjs');
const before={defaultPage:'live',defaultFilter:'solo',matchCount:'20',maskNames:true,scale:'1.25',searchRegion:'cn',searchServer:'HN1'};
const follow={label:'艾欧尼亚',events:[{searchUI:true,event:'client_follow_client',trigger:'login',fields:{to_group:'players',search_to:'HN1',applied:true}}]};
function fixture(frame){
 let time=0,count=0,evaluations=0;const frames=[],status={clientRegion:'TENCENT',serverId:'HN1',clientView:{state:'ready'}},search={dataset:{region:'cn',serverId:''}},window={__r265FollowCaptureStartedMs:0,deepLegendsMatchCards:{live:{state:{status}}}};
 const context=vm.createContext({window,document:{querySelector:s=>s==='#player-search-region'?search:{textContent:'艾欧尼亚'}},performance:{now:()=>time},requestAnimationFrame:fn=>frames.push(fn),Promise,Number,JSON});
 const evaluate=async expression=>{evaluations++;time+=5;let done=false,value,error;Promise.resolve(vm.runInContext(expression,context)).then(v=>{value=v;done=true;},e=>{error=e;done=true;});
  while(!done){for(let i=0;i<8;i++)await Promise.resolve();if(done)break;const callback=frames.shift();assert(callback);count++;frame?.({count,status,search,window});callback();}if(error)throw error;return value;};
 return {evaluate,now:()=>time,get count(){return count;},get evaluations(){return evaluations;},status,search,window};
}
test('R278 ready-client gate waits for three stable frames after delayed client arrival',async()=>{
 const f=fixture(({count,status})=>{status.clientView.state=count<4?'no-client':'ready';});f.status.clientView.state='no-client';const r=await waitForFollowedClient({...f,phase:'read',noProgressMs:20,totalMs:100});assert.equal(r.done,true);assert(f.count>=6);assert.equal(r.snapshot.server,'HN1');
});
test('R278 ready-client gate rejects a client that never becomes ready with stalled snapshot',async()=>{
 const f=fixture();f.status.clientView.state='no-client';await assert.rejects(waitForFollowedClient({...f,phase:'restart',noProgressMs:10,totalMs:100}),error=>{assert.match(error.message,/no new progress/);assert.equal(error.pageWaitSnapshot.ready,false);return true;});
});
test('R278 ready-client gate enforces total bound when the client never stops changing',async()=>{
 const f=fixture(({count,status})=>{status.serverId='state-'+count;});await assert.rejects(waitForFollowedClient({...f,phase:'read',noProgressMs:20,totalMs:25}),/total limit reached/);
});
test('R278 read and restart reject stable manual override and any changed unrelated control',()=>{
 for(const check of [assertReadControls,assertRestartControls]){check({...before,searchServer:''},before,follow);assert.throws(()=>check(before,before,follow),/manual server override/);assert.throws(()=>check({...before,searchServer:'',scale:'1'},before,follow),assert.AssertionError);assert.throws(()=>check({...before,searchServer:''},before,{...follow,events:[]}),/client_follow_client login/);}
});
test('R278 diagnostic prelude captures the real startup report before readiness',()=>{
 const window={},context=vm.createContext({window,performance:{now:()=>0}});vm.runInContext(followCapturePrelude(),context);window.reportFlowDiagnostic=()=>{};vm.runInContext("function updateSearchRegionStatus(){window.reportFlowDiagnostic('client_follow_client','login',{to_group:'players',search_to:'HN1',applied:true});}updateSearchRegionStatus();",context);assert.equal(window.__r265FollowEvents.length,1);assert.equal(window.__r265FollowEvents[0].searchUI,true);assert.equal(window.__r265FollowEvents[0].trigger,'login');
});

test('R278 client readiness survives transient navigation context and still requires three stable frames',async()=>{
 const f=fixture();let attempts=0;
 const evaluate=async expression=>{if(attempts++<2)throw Error(JSON.stringify({code:-32000,message:'Inspected target navigated or closed'}));return f.evaluate(expression);};
 const result=await waitForFollowedClient({...f,evaluate,phase:'read',noProgressMs:20,totalMs:100});assert.equal(result.done,true);assert.equal(f.count,3);assert(attempts>=3);
});
test('R278 repeated navigation errors fail at the idle window with the original protocol snapshot',async()=>{
 let time=0;const evaluate=async()=>{time+=5;throw Error(JSON.stringify({code:-32000,message:'Not attached to an active page'}));};
 await assert.rejects(waitForFollowedClient({evaluate,phase:'read',now:()=>time,noProgressMs:10,totalMs:100}),error=>{assert.match(error.message,/no new progress/);assert.equal(error.pageWaitSnapshot.message,'Not attached to an active page');return true;});
});
test('R278 readiness preserves arbitrary CDP errors and product assertion failures',async()=>{
 for(const error of [Error(JSON.stringify({code:-32000,message:'Inspected target crashed'})),new assert.AssertionError({message:'product invariant'})])await assert.rejects(waitForFollowedClient({evaluate:async()=>{throw error;},phase:'read'}),e=>e===error);
});
