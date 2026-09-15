'use strict';
const test=require('node:test'), assert=require('node:assert/strict'), fs=require('node:fs'), vm=require('node:vm'), path=require('node:path');
const {JSDOM}=require('../desktop/node_modules/jsdom');
const src=fs.readFileSync(process.env.R91_ADDENDUM_GAMEPLAY_SOURCE||path.join(__dirname,'gameplay.js'),'utf8');
const suite=fs.readFileSync(process.env.R91_ADDENDUM_SUITE_SOURCE||path.join(__dirname,'suite.js'),'utf8');
function extract(name,source=src){let start=source.indexOf(`function ${name}(`);assert.ok(start>=0,name);if(source.slice(start-6,start)==='async ')start-=6;return source.slice(start,source.indexOf('\n  }',start)+4);}
const flush=()=>new Promise(setImmediate);
const payload=(n=20,beg=0)=>({player:{playerRef:'opaque',region:'kr',gameName:'Fixture',tagLine:'KR1'},matches:Array.from({length:n},(_,i)=>({gameId:beg+i+1})),pagination:{begIndex:beg,count:n,hasMore:true}});
function harness(responses){
 const jobs=new Map(),requests=[],toasts=[];let sequence=0,now=0;
 const state={controllers:new Map(),settings:{matchCount:20},tabs:[],overlay:[]};
 const dom=new JSDOM('<main></main>'),root=dom.window.document.querySelector('main');
 const render=tab=>{root.innerHTML=`<div>${(tab.data?.matches||[]).map(m=>`<article>${m.gameId}</article>`).join('')}</div>`+(tab.error?`<div class="notice is-warning">${tab.error}</div>`:'')+(tab.initialPageError?`<div class="notice is-warning">${tab.initialPageError}</div>`:'')+(tab.data?.pagination?.moreError?`<div class="notice is-warning">${tab.data.pagination.moreError}</div>`:'');};
 const noop=()=>{};
 const context={state,Headers,AbortController,TextDecoder,Uint8Array,Date,MAX_BROWSE_MATCHES:1000,AUTO_PAGE_DELAY_MS:400,AUTO_PAGE_MAX_BACKOFF_MS:8000,
  setTimeout:(fn,ms)=>{jobs.set(++sequence,{fn,ms,at:now+ms});return sequence;},clearTimeout:id=>jobs.delete(id),
  fetch:async(url,options)=>{requests.push({url,options});const next=responses.shift();assert.ok(next,'unexpected request');return typeof next==='function'?next(options):next;},
  tabReady:()=>true,riotTab:()=>true,tabGroup:()=> 'kr',rerenderTab:render,appendOverviewMatches:render,showLoadingMoreState:noop,
  showToast:x=>toasts.push(x),loadOPGGSeasonSummary:noop,loadOverviewCurrentGame:noop,syncOverviewSupplementRefs:noop,rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture',renderCapabilitySettings:noop,
  normalizedPagination:(p,beg)=>({...p.pagination,nextBegIndex:beg+p.pagination.count}),
 };
 vm.runInNewContext(['api','loadOverview'].map(n=>extract(n)).join('\n'),context);
 return {...context,root,jobs,requests,toasts,close:()=>dom.window.close(),async advance(ms){now+=ms;for(const [id,job] of [...jobs])if(job.at<=now){jobs.delete(id);job.fn();}await flush();}};
}
const response=(status,body,headers={})=>new Response(typeof body==='string'?body:JSON.stringify(body),{status,headers});
for(const append of [false,true]){
 test(`R91 addendum ${append?'append':'first partial'} 429 stays silent and retries after Retry-After`,async()=>{
  const h=harness([response(429,{error:'额度恢复',kind:'rate-limited',retryAfter:5},{'Retry-After':'5'}),response(200,payload(20,append?20:0))]);
  try{
   const tab={key:'fixture',region:'kr',data:payload(5),nextBegIndex:20,initialPagePending:!append};h.state.tabs=[tab];const old=tab.data;
   assert.equal(await h.loadOverview(tab,!append,append,false,true),false);
   assert.equal(tab.initialPageError,'');assert.equal(tab.error,'');assert.equal(tab.data,old);assert.equal(tab.data.pagination.autoPaused,undefined);assert.equal(tab.data.pagination.moreError,undefined);
   assert.equal(h.toasts.length,0);assert.equal(h.root.querySelector('.notice'),null);
   assert.ok([...h.jobs.values()].some(x=>x.ms===5000));await h.advance(4999);assert.equal(h.requests.length,1);await h.advance(1);assert.equal(h.requests.length,2);
   assert.equal(tab.quotaRetry,null);assert.equal(tab.initialPageError,'');assert.equal(tab.data.matches.length,append?25:20);
  }finally{h.close();}
 });
 for(const status of [500,503]) test(`R91 addendum ${append?'append':'first partial'} ${status} remains visible`,async()=>{
  const h=harness([response(status,{error:'真实失败',kind:'rate-limited',retryAfter:5})]);
  try{const tab={key:'fixture',region:'kr',data:payload(5),initialPagePending:!append};await h.loadOverview(tab,!append,append,false,true);
   assert.match(append?tab.data.pagination.moreError:tab.initialPageError,/真实失败/);assert.ok(h.root.querySelector('.notice'));assert.equal(h.toasts.length,append?1:0);assert.equal(tab.quotaRetry,null);
  }finally{h.close();}
 });
}
test('R91 addendum header-only 429 and JSON fallback parse while 503 stays HTTP',async()=>{
 const h=harness([response(429,'wait',{'Retry-After':'5'}),response(429,{error:'wait',retryAfter:7}),response(503,{error:'busy',kind:'rate-limited',retryAfter:5})]);
 try{for(const [seconds,kind] of [[5,'rate-limited'],[7,'rate-limited'],[5,'http']])await assert.rejects(h.api('/fixture'),e=>e.retryAfter===seconds&&e.errorKind===kind);}finally{h.close();}
});
test('R91 addendum stream displays five before completion with exactly one count-20 request',async()=>{
 let controller;
 const stream=new ReadableStream({start(c){controller=c;}}),encode=x=>new TextEncoder().encode(JSON.stringify(x)+'\n');
 const h=harness([new Response(stream,{headers:{'Content-Type':'application/x-ndjson'}})]);
 try{const tab={key:'fixture',region:'kr',riotId:{gameName:'Fixture',tagLine:'KR1'}};const pending=h.loadOverview(tab);await flush();
  assert.equal(JSON.parse(h.requests[0].options.body).count,20);
  controller.enqueue(encode({type:'progress',overview:payload(5)}));await flush();assert.equal(tab.data.matches.length,5);assert.equal(h.root.querySelectorAll('article').length,5);assert.equal(tab.loading,true);
  controller.enqueue(encode({type:'complete',overview:payload(20)}));controller.close();assert.equal(await pending,true);assert.equal(tab.data.matches.length,20);assert.equal(tab.initialPagePending,false);assert.equal(h.requests.length,1);
 }finally{h.close();}
});
test('R91 addendum streaming append preserves cursor until completion without stalling',async()=>{
 let controller;const stream=new ReadableStream({start(c){controller=c;}});const h=harness([new Response(stream,{headers:{'Content-Type':'application/x-ndjson'}})]);
 try{const tab={key:'fixture',region:'kr',data:payload(20),nextBegIndex:20,paginationStalls:1};const pending=h.loadOverview(tab,false,true);await flush();
  const send=(type,n)=>controller.enqueue(new TextEncoder().encode(JSON.stringify({type,overview:payload(n,20)})+'\n'));
  send('progress',5);await flush();assert.equal(tab.nextBegIndex,20);assert.equal(tab.data.matches.length,25);
  send('complete',20);controller.close();await pending;assert.equal(tab.data.matches.length,40);assert.equal(tab.nextBegIndex,40);assert.equal(tab.paginationStalls,0);assert.notEqual(tab.data.pagination.autoPaused,true);
 }finally{h.close();}
});
test('R91 addendum stream quota after partial data is silent; a truncated stream is visible',async()=>{
 for(const quota of [true,false]){const frames=[{type:'progress',overview:payload(5)}];if(quota)frames.push({type:'error',status:429,error:'quota',kind:'rate-limited',retryAfter:5});
  const h=harness([response(200,frames.map(x=>JSON.stringify(x)).join('\n')+'\n',{'Content-Type':'application/x-ndjson'})]);
  try{const tab={key:'fixture',region:'kr',riotId:{gameName:'Fixture',tagLine:'KR1'}};await h.loadOverview(tab);assert.equal(tab.data.matches.length,5);assert.equal(h.toasts.length,0);if(quota){assert.equal(tab.initialPageError,'');assert.ok(tab.quotaRetry);}else assert.match(tab.initialPageError,/中断/);}finally{h.close();}
 }
});
test('R91 addendum closed, replaced, reset and changed-filter retries cannot issue stale reads',async()=>{
 for(const action of ['close','replace','filter','reset']){
  const h=harness([response(429,'quota',{'Retry-After':'5'}),response(200,payload())]);
  try{const tab={key:'fixture',region:'kr',data:payload(5),matchFilter:'all'};h.state.tabs=[tab];await h.loadOverview(tab,true);
   if(action==='close')tab.closed=true;else if(action==='filter')tab.matchFilter='solo';else if(action==='reset')tab.overviewRequestToken++;else await h.loadOverview(tab,true);
   const count=h.requests.length;await h.advance(5000);assert.equal(h.requests.length,count);
  }finally{h.close();}
 }
});
test('R91 addendum existing history remains readable during polling; new history still skeletons; arena hides rank',()=>{
 const state={liveLoading:false,settings:{}};const deps={state,number:String,percent:String,kda:String,escapeHTML:String,iconFigure:()=>'<img>',maskedPlayerName:()=> 'Player',liveDisplayedChampionId:()=>1,renderLivePremadeTag:()=>'',rankTitle:()=> '白银 I',positionLabel:()=> '上单'};
 vm.runInNewContext(['renderInsightMatches','insightScore','renderLivePlayer'].map(n=>extract(n)).join('\n'),deps);
 const player={playerRef:'p',historyState:'ok',rank:{tier:'SILVER'},modeStats:{games:1,wins:1,kda:3},recentGames:[{championId:1,kills:3,deaths:1,assists:0,win:true}]};
 const before=deps.renderInsightMatches(player)+deps.renderLivePlayer(player,0,true);state.liveLoading=true;
 assert.equal(deps.renderInsightMatches(player)+deps.renderLivePlayer(player,0,true),before);assert.doesNotMatch(before,/白银|未定级/);assert.match(deps.renderLivePlayer(player,0,false),/白银 I/);
 assert.match(deps.renderInsightMatches({historyState:'pending',recentGames:[]}),/skeleton/);
});
test('R91 addendum facade defaults and missing rank fields agree with controls',()=>{
 const state={facade:{}};const h={state};vm.runInNewContext(['rankLabel','hydrateFacadeDraft','facadeIdentityDirty'].map(n=>extract(n,suite)).join('\n'),h);
 assert.equal(h.rankLabel({}),'未定级 · 单双排');h.hydrateFacadeDraft();assert.equal(state.facadeDraft.tier,'UNRANKED');assert.equal(state.facadeDraft.division,'I');assert.equal(h.facadeIdentityDirty(),false);assert.doesNotMatch(h.rankLabel({tier:'GOLD'}),/undefined/);
});
test('R91 addendum unchanged polling preserves roster DOM nodes',()=>{
 const dom=new JSDOM('<button id="refresh"></button><div id="content"></div>');const doc=dom.window.document;
 const state={section:'live',beacon:{phase:'ChampSelect'},live:{available:true,phase:'ChampSelect'},settings:{}};
 const h={state,nodes:{liveRefresh:doc.querySelector('button'),liveContent:doc.querySelector('div')},connected:()=>true,renderSessionSummary:()=>{},renderLiveRefreshStatus:()=>state.liveLoading?'<span>正在刷新</span>':'',renderRecommendationArea:()=>'<article>known match</article>',bindLiveContent:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{}};
 vm.runInNewContext(extract('renderLive'),h);h.renderLive();const row=h.nodes.liveContent.querySelector('article');state.liveLoading=true;h.renderLive();assert.equal(h.nodes.liveContent.querySelector('article'),row);state.liveLoading=false;h.renderLive();assert.equal(h.nodes.liveContent.querySelector('article'),row);dom.window.close();
});
test('R91 addendum Loading and early incomplete arena poll every five seconds then converge',()=>{
 for(const phase of ['GameStart','InProgress','Reconnect']){
  const state={section:'live',beacon:{phase},live:{phase,gameId:91,available:true,gameMode:'CHERRY',arenaGrouped:false,arenaGroupingUnavailable:true,arenaGroupingRetryable:true,players:[{historyState:'ok'}]},settings:{liveRefresh:true}};
  let job;const h={state,document:{hidden:false},Date,connected:()=>true,liveGamePhase:p=>['GameStart','InProgress','Reconnect'].includes(p),liveAugmentRecommendationSource:()=> 'arena',clearTimeout:()=>{job=null;},setTimeout:(fn,delay)=>{job={fn,delay};return 1;},loadLive:()=>{}};
  vm.runInNewContext(['syncLiveRetryBudget','liveSnapshotComplete','scheduleLiveRefresh'].map(n=>extract(n)).join('\n'),h);
  for(let i=0;i<12;i++){h.scheduleLiveRefresh();assert.equal(job.delay,5000);job.fn();assert.equal(state.liveRetryAttempts,0);}
  state.liveRetryStartedAt=Date.now()-60001;
  for(let i=0;i<8;i++){h.scheduleLiveRefresh();assert.equal(job.delay,20000);job.fn();}
  h.scheduleLiveRefresh();assert.equal(job,null);state.liveRetryAttempts=0;state.live.arenaGrouped=true;h.scheduleLiveRefresh();assert.equal(job,null);
 }
});
