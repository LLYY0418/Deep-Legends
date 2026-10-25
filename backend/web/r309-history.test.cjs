'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),{JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile}=require('./r188-harness.cjs'),af=require('./history-filters.js');
test('R309 ordinary twenty-match network pages display every loaded row without search conditions',async()=>{
 const dom=new JSDOM('<main><div class="matches-column"><div class="match-list"></div><div data-match-sentinel></div></div></main>'),document=dom.window.document,root=document.querySelector('main'),list=root.querySelector('.match-list');
 const tab={key:'ordinary-pages',playerRef:'fixture-player',region:'cn',serverId:'HN1',matchFilter:'all',advancedConditions:{},openMatches:new Set()},state={controllers:new Map(),settings:{matchCount:20}},requests=[],noop=()=>{};
 let f;
 const render=()=>{if(tab.data){f.reconcileFilteredMatchList(list,tab);f.appendOverviewMatches(tab,[]);}};
 f=compile(read('gameplay.js'),['loadOverview','normalizedPagination','loadNextMatchWindow','revealLoadedMatchWindow','filteredMatches','matchRenderWindow','reconcileFilteredMatchList','appendOverviewMatches','paginationCopyFor','matchSentinelShouldHide','bindMatchSentinel'],{
  document,window:{},state,MAX_BROWSE_MATCHES:1000,AUTO_PAGE_DELAY_MS:400,AUTO_PAGE_MAX_BACKOFF_MS:8000,number:String,escapeHTML:require('./r188-harness.cjs').escapeHTML,
  connected:()=>true,tabReady:()=>true,riotTab:()=>false,tabGroup:()=> 'cn',overviewContainer:()=>root,rerenderTab:render,matchObserverKey:()=> 'matchObserver',
  showLoadingMoreState:noop,showToast:message=>assert.fail(message),loadOPGGSeasonSummary:noop,loadOverviewCurrentGame:noop,loadMayhemRating:noop,syncOverviewSupplementRefs:noop,rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture',renderCapabilitySettings:noop,computeOverviewStreak:()=>null,reportDeferredMatchesCard:noop,
  renderMatch:match=>`<article class="match-entry" data-match-id="${match.gameId}"></article>`,bindOverviewContent:noop,applyRenderedMetricStyles:noop,prepareImages:noop,patchLPChip:noop,boundMatchDOM:noop,observeMatchTierVisibility:noop,fitMatchTags:noop,matchTierScope:()=> 'season',
  api:async(path,options)=>{
   assert.equal(path,'/api/gameplay/overview');const request=JSON.parse(options.body);requests.push(request);assert.equal(request.count,20);assert.equal(request.begIndex,(requests.length-1)*20);assert.equal(request.matchFilter,'all');assert(requests.length<=3,'unexpected additional page');
   return {player:{playerRef:'fixture-player',region:'cn',serverId:'HN1'},ranks:[],matches:Array.from({length:20},(_,i)=>({gameId:request.begIndex+i+1,queueId:420})),pagination:{begIndex:request.begIndex,count:20,hasMore:requests.length<3}};
  },
 });
 const assertRows=count=>{assert.equal(tab.data.matches.length,count);assert.equal(list.querySelectorAll('.match-entry').length,count);assert.deepEqual([...list.children].map(row=>Number(row.dataset.matchId)),Array.from({length:count},(_,i)=>i+1));assert.deepEqual(tab.advancedConditions,{});};
 try{
  assert.equal(await f.loadOverview(tab),true);assertRows(20);
  for(const count of [40,60]){const more=root.querySelector('[data-load-more]');assert(more,'production pagination exposes the next page');more.click();for(let turn=0;turn<10&&tab.loadingMore;turn++)await new Promise(setImmediate);assert.equal(tab.loadingMore,false);assertRows(count);}
  assert.deepEqual(requests.map(request=>[request.begIndex,request.count]),[[0,20],[20,20],[40,20]]);assert.equal(root.querySelector('[data-load-more]'),null);assert.equal(Boolean(tab.data.pagination.hasMore),false);
 }finally{dom.window.close();}
});
test('R309 revealing loaded rows consumes scroll intent before the sentinel is rebound',async()=>{
 const dom=new JSDOM('<div id="app-scroll"><main><div class="match-list"></div><div data-match-sentinel></div></main></div>'),w=dom.window,scroll=w.document.getElementById('app-scroll'),root=scroll.firstChild,list=root.querySelector('.match-list');
 const tab={data:{matches:Array.from({length:60},(_,i)=>({gameId:i+1,queueId:420})),pagination:{hasMore:true}},openMatches:new Set()},frames=[],timers=[],observers=[];let network=0;
 class IO{constructor(callback){this.callback=callback;observers.push(this);}observe(){}disconnect(){}}
 const f=compile(read('gameplay.js'),['bindMatchSentinel','revealLoadedMatchWindow','appendOverviewMatches','reconcileFilteredMatchList','filteredMatches','matchRenderWindow'],{
  document:w.document,window:{IntersectionObserver:IO,requestAnimationFrame:fn=>frames.push(fn)},IntersectionObserver:IO,state:{},matchObserverKey:()=> 'matchObserver',overviewContainer:()=>root,
  clearTimeout(){},setTimeout:fn=>(timers.push(fn),timers.length),loadOverview:async()=>{network++;return true;},
  renderMatch:match=>`<article class="match-entry" data-match-id="${match.gameId}"></article>`,bindOverviewContent(){},applyRenderedMetricStyles(){},prepareImages(){},patchLPChip(){},
  boundMatchDOM(){},observeMatchTierVisibility(){},fitMatchTags(){},matchTierScope:()=> 'season',paginationCopyFor:()=> '',matchSentinelShouldHide:()=>false,
 });
 const flush=async()=>{assert(frames.length<=1);while(frames.length)frames.shift()();assert(timers.length<=1);while(timers.length)await timers.shift()();};
 const near=()=>observers.at(-1).callback([{isIntersecting:true}]);
 try{
  f.reconcileFilteredMatchList(list,tab);f.bindMatchSentinel(root,tab);assert.equal(list.children.length,20);near();await flush();assert.equal(list.children.length,20);
  scroll.dispatchEvent(new w.WheelEvent('wheel',{deltaY:100}));await flush();assert.equal(list.children.length,40);assert.equal(observers.length,2);
  near();await flush();assert.equal(list.children.length,40,'a new observer must await a second user gesture');assert.equal(network,0);
  scroll.dispatchEvent(new w.WheelEvent('wheel',{deltaY:100}));await flush();assert.equal(list.children.length,60);near();await flush();assert.equal(list.children.length,60);assert.equal(network,0);
 }finally{dom.window.close();}
});
test('R309 clearing 1000 rows renders twenty and loaded windows reveal twenty before any network',()=>{
 const dom=new JSDOM('<main><div class="match-list"></div></main>'),root=dom.window.document.querySelector('main'),tab={data:{matches:Array.from({length:1000},(_,i)=>({gameId:i+1}))},openMatches:new Set()};let network=0;
 const f=compile(read('gameplay.js'),['matchRenderWindow','revealLoadedMatchWindow','loadNextMatchWindow'],{filteredMatches:rows=>rows,overviewContainer:()=>root,reconcileFilteredMatchList:(list,t)=>list.innerHTML=f.matchRenderWindow(t.data.matches,t).map(m=>`<article class="match-entry" data-match-id="${m.gameId}"></article>`).join(''),appendOverviewMatches(){},loadOverview:()=>network++});
 root.querySelector('.match-list').innerHTML=f.matchRenderWindow(tab.data.matches,tab).map(()=>'<article class="match-entry"></article>').join('');assert.equal(root.querySelectorAll('.match-entry').length,20);
 for(let count=40;count<=1000;count+=20){f.loadNextMatchWindow(tab);assert.equal(root.querySelectorAll('.match-entry').length,count);assert.equal(network,0);}f.loadNextMatchWindow(tab);assert.equal(network,1);tab.filteredVisibleCount=20;tab.openMatches.add('900');assert.equal(f.matchRenderWindow(tab.data.matches,tab).length,21);dom.window.close();
});
test('R309 searching keeps stop and clear nodes across progress; stopping label updates in place',()=>{const dom=new JSDOM('<div id="counts"></div>'),node=dom.window.document.querySelector('div'),tab={advancedConditions:{result:{values:['win']}},data:{matches:[]},advancedSearch:{running:true,scanned:20}},ctx={rows:()=>[],subject:()=>({}),hasMore:()=>true};node.innerHTML=af.counts(tab,ctx);const stop=node.querySelector('[data-af-stop]'),progress=node.querySelector('progress');for(let i=0;i<10;i++){tab.advancedSearch.scanned+=20;af.updateCounts(node,tab,ctx);assert(stop.isSameNode(node.querySelector('[data-af-stop]')));assert(progress.isSameNode(node.querySelector('progress')));}tab.advancedSearch.stopping=true;af.updateCounts(node,tab,ctx);assert.equal(stop.disabled,true);assert.equal(stop.textContent,'正在停止…');tab.advancedSearch.running=false;af.updateCounts(node,tab,ctx);const clear=node.querySelector('[data-af-clear]');af.updateCounts(node,tab,ctx);assert(clear.isSameNode(node.querySelector('[data-af-clear]')));dom.window.close();});
const local=(y,m,d)=>new Date(y,m-1,d,13,14,15).getTime();
test('R309 calendar ranges clamp month end across leap years and year boundaries; presets round-trip',()=>{
 for(const [date,n,want]of [[[2026,3,31],1,[2026,2,28]],[[2024,3,31],1,[2024,2,29]],[[2026,1,31],2,[2025,11,30]],[[2026,10,25],2,[2026,8,25]],[[2026,10,25],3,[2026,7,25]],[[2026,10,25],4,[2026,6,25]],[[2026,10,25],5,[2026,5,25]]])assert.equal(af.calendarMonthStart(local(...date),n),local(...want));
 const now=local(2026,10,25),ctx={now:()=>now,seasonStart:'2026-01-08T08:00:00Z',subject:()=>({})};assert.equal(af.timeStart('season',ctx),Date.parse('2026-01-08T00:00:00Z'));assert.equal(af.timeOptions({}).includes('season'),false);assert.equal(af.timeOptions({seasonStart:'bad'}).includes('season'),false);
 const normalized=af.normalize({time:{values:['m2','m3','m4','m5','season','unknown','d30']}});assert.deepEqual(normalized.time.values,['m2','m3','m4','m5','season','d30']);
 const tab={advancedConditions:{time:{values:['m3'],not:true}}},range=af.serverTimeRange(tab,ctx);assert.equal(range.startTime,1);assert.equal(range.endTime,Math.floor(local(2026,7,25)/1000)-1);assert.equal(af.scanLimit(tab,ctx),300);tab.advancedConditions.time.not=false;assert.equal(af.scanLimit(tab,ctx),1000);assert.equal(af.scanLimit(tab,{foreign:()=>true}),100);
 const store=new Map(),storage={getItem:k=>store.get(k),setItem:(k,v)=>store.set(k,v)};assert(af.savePreset(storage,'三个月',{time:{values:['m3']}}));assert(af.savePreset(storage,'旧筛选',{time:{values:['d30']}}));assert.deepEqual(af.readPresets(storage).map(p=>p.conditions.time.values[0]),['m3','d30']);
});
async function scanFixture({count,crossing}){const now=Date.parse('2026-10-25T00:00:00Z'),start=Date.parse('2026-07-25T00:00:00Z'),all=Array.from({length:count},(_,i)=>({gameId:i+1,createdAt:i>=crossing?start-86400000:Date.parse('2026-08-01T00:00:00Z'),result:'win'})),rows=[],reports=[];let next=0;const tab={advancedConditions:{time:{values:['m3'],not:false}},data:{matches:rows,pagination:{hasMore:true}}};const ctx={rows:()=>rows,subject:()=>({}),now:()=>now,isActive:()=>true,hasMore:()=>next<all.length,cursor:()=>next,render(){},load:async n=>{rows.push(...all.slice(next,next+n));next=Math.min(next+n,all.length);return true;},diagnostic:(event,fields)=>reports.push({event,...fields})};await af.find(tab,ctx);return {tab,ctx,reports,rows};}
test('R309 positive time scans past 300 to the boundary; upstream exhaustion explains dates',async()=>{const h=await scanFixture({count:900,crossing:450});assert.equal(h.tab.advancedSearch.scanned,460);assert.equal(h.reports.length,1);assert.equal(h.reports[0].stop_reason,'time_boundary');assert.equal(h.reports[0].time_value,'m3');assert.equal(h.reports[0].oldest_created_at_day,'2026-07-24');assert.match(af.blocked(h.tab,h.ctx).copy,/已查完近 3 个月/);
 const upstream=await scanFixture({count:600,crossing:Infinity});assert.equal(upstream.tab.advancedSearch.scanned,600);assert.equal(upstream.reports[0].stop_reason,'upstream_window');assert.match(af.blocked(upstream.tab,upstream.ctx).copy,/2026-08-01.*近 3 个月从 2026-07-25.*600 场/);
 upstream.tab.advancedConditions.result={values:['loss']};assert.match(af.empty(upstream.tab,upstream.ctx),/近 3 个月的 600 场对局中没有符合条件的对局/);
});

test('R309 committing conditions preserves the focused trigger and an already open mode menu',()=>{
 const dom=new JSDOM('<main></main>'),d=dom.window.document,bar=d.querySelector('main'),tab={advancedConditions:{result:{values:['win']}}},ctx={rows:()=>[],subject:()=>({}),container:()=>bar,hasMore:()=>true};
 bar.innerHTML=af.render(tab,()=>ctx)+'<div data-match-more-select><button>更多模式</button><div data-app-select-menu></div></div>'+af.renderConditions(tab,()=>ctx);
 const trigger=bar.querySelector('[data-af-open]'),more=bar.querySelector('[data-match-more-select]'),menu=more.querySelector('[data-app-select-menu]');trigger.focus();
 tab.advancedConditions={time:{values:['m3']},result:{values:['win']}};af.syncControls(bar,tab,ctx);
 assert.equal(d.activeElement,trigger);assert.equal(bar.querySelector('[data-af-open]'),trigger);assert.equal(bar.querySelector('[data-match-more-select]'),more);assert.equal(menu.hidden,false);assert.equal(trigger.querySelector('b').textContent,'2');
 tab.advancedConditions={};af.syncControls(bar,tab,ctx);assert.equal(d.activeElement,trigger);assert.equal(bar.querySelector('[data-af-conditions]'),null);assert.equal(trigger.querySelector('b'),null);assert.equal(menu.hidden,false);dom.window.close();
});
