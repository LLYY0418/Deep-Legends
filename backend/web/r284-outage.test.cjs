'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,escapeHTML}=require('./r188-harness.cjs');
const source=()=>fs.readFileSync(process.env.R284_GAME_SOURCE||__dirname+'/gameplay.js','utf8');
test('R284 P3 exhausted outage footer has the approved reason and no action',()=>{
 const f=compile(source(),['paginationCopyFor','normalizedPagination'],{MAX_BROWSE_MATCHES:1000,number:String,escapeHTML,historyServiceState:()=>({kind:'official'}),matchesPending:()=>false});
 for(const count of [20,4]){const tab={data:{matches:Array.from({length:count},()=>({})),pagination:{hasMore:true,historyServiceDown:true,localHasMore:false}}};const html=f.paginationCopyFor(tab);assert(html.includes('官方战绩服务暂时中断，部分玩家战绩无法读取，恢复后点「刷新对局」'));assert(!html.includes('<button'));tab.data.pagination.localHasMore=true;assert(f.paginationCopyFor(tab).includes('data-load-more'),'retained tail must remain browsable');}
 const p=f.normalizedPagination({matches:[],pagination:{begIndex:20,count:0,hasMore:false,historyServiceDown:true,localHasMore:false}},20);assert.equal(p.hasMore,true,'unknown upstream tail must remain resumable');assert.equal(p.nextBegIndex,20);
});
test('R284 P3 outage sentinel blocks with a diagnostic, retained and normal tails still load',()=>{
 for(const down of [true,false])for(const local of [true,false]){
  const dom=new JSDOM('<div id="app-scroll"><main><div data-match-sentinel></div></main></div>');try{const w=dom.window,container=w.document.querySelector('main'),root=w.document.querySelector('#app-scroll'),calls=[],events=[],tasks=[];let observed;
   const IO=class{constructor(cb){observed=cb}observe(){}disconnect(){}};const tab={data:{pagination:{hasMore:true,historyServiceDown:down,localHasMore:local}}};
   const f=compile(source(),['bindMatchSentinel'],{state:{},document:w.document,window:{IntersectionObserver:IO,requestAnimationFrame:f=>f(),reportFlowDiagnostic:(e,r,v)=>events.push({e,r,...v})},IntersectionObserver:IO,matchObserverKey:()=> 'observer',overviewContainer:()=>container,loadOverview:()=>{calls.push(1);return Promise.resolve(true)},setTimeout:f=>{tasks.push(f);return 1},clearTimeout(){}});
   f.bindMatchSentinel(container,tab);observed([{isIntersecting:true}]);root.dispatchEvent(new w.WheelEvent('wheel',{deltaY:10}));for(const f of tasks)f();assert.equal(calls.length,down&&!local?0:1);if(down&&!local)assert(events.some(e=>e.blocked_reason==='service-down'));
  }finally{dom.window.close()}
 }
});
test('R284 P3 recovered outage appends once at the retained cursor only when at bottom',async()=>{
 for(const bottom of [true,false]){const calls=[],root={scrollHeight:1000,clientHeight:500,scrollTop:bottom?500:0},tab={nextBegIndex:20,data:{matches:[{}],pagination:{hasMore:true,historyServiceDown:true},capabilities:[{historyStatus:'official'}]}};
  const f=compile(source(),['retryHistoryRegion'],{Date,document:{getElementById:()=>root},window:{reportFlowDiagnostic(){}},overviewContainer:()=>({closest:()=>null,querySelectorAll:()=>[]}),appendOverviewMatches(){},loadOverview:(t,force,append)=>{calls.push({cursor:t.nextBegIndex,force,append});return Promise.resolve(true)}});
  await f.retryHistoryRegion(tab,'outage-recovery');assert.deepEqual(calls,bottom?[{cursor:20,force:false,append:true}]:[]);assert.equal(tab.historyRecoveryPending,false);assert.equal(tab.data.pagination.historyServiceDown,false);
 }
});
test('R284 P3 normal mode filtering keeps the three page automatic search limit',async()=>{
 let calls=0;const tab={matchFilter:'solo',nextBegIndex:20,data:{matches:[],pagination:{hasMore:true}}};
 const f=compile(source(),['autoLoadMatchFilter'],{state:{settings:{matchCount:20}},filteredMatches:()=>[],renderFilteredMatchView(){},loadOverview:async()=>{calls++;tab.nextBegIndex+=20;return true},setTimeout,Date});
 await f.autoLoadMatchFilter(tab,'solo');assert.equal(calls,3);assert.equal(tab.nextBegIndex,80);assert.equal(tab.data.pagination.autoPaused,true);assert.equal(tab.filterPaging,false);
});
