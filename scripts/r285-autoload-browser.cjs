'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 for(const group of ['pro','kr']){
  const p=await page('dark');await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
  await p.evaluate(`(()=>{window.__r285Reads=[];window.__r285Reports=[];const base=fetch,report=window.reportFlowDiagnostic;window.reportFlowDiagnostic=(e,r,f)=>{if(e==='match_autoload_client')__r285Reports.push({reason:r,...f});report(e,r,f)};window.fetch=(url,options={})=>{if(String(url).includes('/api/gameplay/overview')){const input=JSON.parse(options.body||'{}'),beg=Number(input.begIndex||0);__r285Reads.push({beg,count:input.count});const player={...__r280Overview.player,playerRef:'kr-fixture',gameName:'Fixture',tagLine:'KR1',region:'kr',isCurrent:false};return Promise.resolve(new Response(JSON.stringify({...__r280Overview,player,capabilities:[],matches:Array.from({length:20},(_,i)=>({...__r280Overview.matches[i%__r280Overview.matches.length],gameId:80000-beg-i,createdAt:Date.now()-(beg+i)*600000})),pagination:{begIndex:beg,count:20,hasMore:true}}),{headers:{'Content-Type':'application/json'}}))}return base(url,options)};__r278Game.openPlayerByRiotId('Fixture','KR1','kr','',{group:'${group}'});})()`);
  await p.until('__r278Game.activeTab()?.data?.matches.length===20&&!__r278Game.activeTab().loading');
  for(let pageNo=2;pageNo<=5;pageNo++){
   if(pageNo===3){await p.evaluate("__r278App.activateSection('favorites');__r278App.activateSection('overview')");await p.until('__r278Game.state.section==="overview"');}
   // Native wheel input, with real browser scrolling and IntersectionObserver.
   await p.evaluate("document.getElementById('app-scroll').scrollTop=document.getElementById('app-scroll').scrollHeight");
   await p.call('Input.dispatchMouseEvent',{type:'mouseWheel',x:1200,y:900,deltaX:0,deltaY:700});
   await p.until(`__r278Game.activeTab().data.matches.length>=${pageNo*20}&&!__r278Game.activeTab().loadingMore`);
   if(pageNo===3){
    // A later stream header replaces only the summoner strip, through the real
    // patchOverviewProgress -> bindOverviewContent -> bindMatchSentinel chain.
    const r=await p.evaluate("(()=>{const g=__r278Game,t=g.activeTab();t.data={...t.data,player:{...t.data.player,summonerLevel:999}};g.patchOverviewProgress(t);return {bound:!!g.state['"+(group==='pro'?'proMatchObserver':'matchObserver')+"'],sentinel:!!document.querySelector('[data-match-sentinel]')}})()");
    assert(r.bound,'partial header binding disconnected the full-list sentinel: '+JSON.stringify(r));
   }
  }
  const r=await p.evaluate("({reads:__r285Reads,reports:__r285Reports,matches:__r278Game.activeTab().data.matches.length})");
  assert.equal(r.matches,100);assert.deepEqual(r.reads.map(r=>r.beg),[0,20,40,60,80]);results.push({group,...r});await p.shot('autoload-'+group+'-page5');
 }
 assert.deepEqual(errors,[]);
}).catch(e=>{console.error(e);process.exitCode=1});
