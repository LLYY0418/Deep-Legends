const {viewStatus,clientView}=require('./r258-client-view-fixture.cjs');
'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const gameplay=fs.readFileSync(process.env.R258_GAMEPLAY_SOURCE||path.join(__dirname,'gameplay.js'),'utf8');
const app=fs.readFileSync(process.env.R258_APP_SOURCE||path.join(__dirname,'app.js'),'utf8');
const identity={connected:true,identityReady:true,sgpReady:true,clientRegion:'TENCENT',serverId:'HN1',summoner:{gameName:'Fixture',tagLine:'CN',profileIconId:1,summonerLevel:123,playerRef:'player_self'}};
function harness(){
 const html=fs.readFileSync(process.env.R258_HTML_SOURCE||path.join(__dirname,'default/index.html'),'utf8');
 const dom=new JSDOM(html,{pretendToBeVisual:true}),document=dom.window.document;
 const nodes={playerTabs:document.getElementById('player-tabs'),overviewContent:document.getElementById('overview-content'),liveContent:document.getElementById('live-content'),playerOverlay:document.getElementById('player-overlay'),playerOverlayContent:document.getElementById('player-overlay-content'),playerOverlayTitle:document.getElementById('player-overlay-title')};
 const current={key:'current',current:true,region:'',regionResolved:false,label:'当前召唤师',icon:0,data:null};
 const other={key:'other',region:'jp1',group:'kr',label:'Other',data:{matches:[{gameId:2}]}};
 const state={status:{connected:false},tabs:[current,other],settings:{},activeGroup:'players',section:'overview',activeTabs:{players:'current',kr:'other',pro:''},tabHistories:{players:[],kr:[],pro:[]},controllers:new Map(),overlay:[]};
 const timers=new Map(),loads=[];let sequence=0;
 const noop=()=>{};
 const el={startupLoading:document.getElementById('startup-loading'),appFrame:document.getElementById('app-frame')};
 const f=compile(gameplay,['resetPlayerHistoryConditions','cancelAdvancedMatchSearch','disposeMatchRowHeight','selfTabReady','updateStatus','updateClientView','connected','tabGroup','riotTab','tabReady','activeTab','summonerLabel','renderPlayerTabWorkspace','resetTencentTabsAfterDisconnect','updateDisconnectedPlayerDOM','activateOverviewView','overviewRenderCacheFields','resetOverviewRenderCache','renderOverviewBodyContent','renderSelfIdentityHeader','handleSelfOverviewTimeout'],{state,nodes,document,window:dom.window,CustomEvent:dom.window.CustomEvent,localStorage:{setItem:noop},overviewWorkspace:()=>({tabs:nodes.playerTabs,content:nodes.overviewContent}),overviewGroupForSection:()=>state.activeGroup,renderPlayerTabs:()=>f.renderPlayerTabWorkspace(state.activeGroup),activateOverviewTabPanel:()=>{f.activateOverviewView(nodes.overviewContent,f.activeTab());if(f.activeTab())f.renderOverviewBodyContent(nodes.overviewContent,f.activeTab());},loadOverview:(tab,force)=>{tab.overviewCardLoad={ready:new Set()};loads.push([tab,force]);},loadLive:noop,syncOverlayAddButton:noop,scheduleBeaconPoll:noop,updateBeacon:noop,resetLiveGameScopedState:noop,renderOverviewSubpage:noop,matchTierScope:()=> 'self',tabServerLabel:()=> '艾欧尼亚',number:String,escapeHTML,assetPath:(_kind,id)=>`/profile/${id}`,assetIcon:(_path,label)=>`<span class="game-icon">${escapeHTML(label)}</span>`,prepareImages:noop,requestAnimationFrame:noop,updatePlayerTabScrollControls:noop,setTimeout:(fn,delay)=>{timers.set(++sequence,{fn,delay});return sequence;},clearTimeout:id=>timers.delete(id),rerenderTab:tab=>{f.renderOverviewBodyContent(nodes.overviewContent,tab);},emptyState:(title,error,retry)=>`<section class="error-card">${title} ${error}<button data-gameplay-retry>重试</button></section>`});
 const overlay=compile(app,['updateReadingOverlay','showReadingOverlay'],{el});
 return {dom,document,nodes,current,other,state,timers,loads,el,...f,...overlay,close:()=>dom.window.close()};
}

test('R258 startup is hidden from HTML through discovery and headers render synchronously at readiness',()=>{
 const h=harness();try{
 assert.equal(h.el.startupLoading.hidden,true);
 for(const clientDiscovery of ['process-not-found','credentials-unreadable','probe-failed']){h.updateStatus(viewStatus({connected:false,clientDiscovery}));h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,true);assert(!h.el.appFrame.hasAttribute('inert'));assert(!h.nodes.playerTabs.querySelector('[data-player-tab="current"]'));}
 h.updateStatus(viewStatus({...identity,sgpReady:false}));assert(!h.nodes.playerTabs.querySelector('[data-player-tab="current"]'));assert.equal(h.loads.length,0);
 h.updateStatus(viewStatus(identity));assert.match(h.nodes.playerTabs.textContent,/Fixture#CN/);assert.match(h.nodes.overviewContent.querySelector('.summoner-strip').textContent,/Fixture#CN.*艾欧尼亚.*123/);assert.equal(h.loads.length,1);assert(h.nodes.overviewContent.querySelector('.gameplay-skeleton'));assert.equal(h.el.startupLoading.hidden,true);
 const timeout=[...h.timers.values()].find(row=>row.delay===15000);assert(timeout);timeout.fn();assert.match(h.nodes.overviewContent.textContent,/超时.*重试/);assert(h.nodes.overviewContent.querySelector('.summoner-strip'));assert.equal(h.el.startupLoading.hidden,true);
 }finally{h.close();}
});

test('R258 disconnect removes only self DOM, retains other list nodes, and false-positive restore skips history reload',()=>{
 const h=harness();try{
 h.updateStatus(viewStatus(identity));
 const otherList=h.document.createElement('div');otherList.className='match-list';otherList.textContent='other matches';
 const fragment=h.document.createDocumentFragment();fragment.append(otherList);h.nodes.overviewContent._overviewViews.set(h.other,{content:fragment,values:{}});
 h.current.data={player:{playerRef:'player_self'},matches:[{gameId:1}]};h.current.overviewCardLoad.selfReady=true;
 const selfList=h.document.createElement('div');selfList.className='match-list';h.nodes.overviewContent.append(selfList);
 h.updateStatus(viewStatus({...identity,connected:false,clientDiscovery:'exiting'}));assert(!h.nodes.playerTabs.querySelector('[data-player-tab="current"]'));assert.equal(h.current.closed,true);assert.equal(h.current.data.matches[0].gameId,1);assert.equal(otherList.textContent,'other matches');assert.equal(h.el.startupLoading.hidden,true);
 h.updateStatus(viewStatus(identity));assert.equal(h.loads.length,1);assert.equal(h.nodes.overviewContent.querySelector('.match-list'),selfList);
 h.updateStatus(viewStatus({...identity,connected:false,clientDiscovery:'exiting'}));h.updateStatus(viewStatus({connected:false,clientDiscovery:'credentials-unreadable'}));h.updateReadingOverlay();h.updateStatus(viewStatus({connected:false,clientDiscovery:'process-not-found'}));h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,true);
 }finally{h.close();}
});

test('R258 disconnect leaves a currently viewed searched list unchanged',()=>{
 const h=harness();try{h.updateStatus(viewStatus(identity));h.state.activeGroup='kr';h.nodes.overviewContent._overviewViewTab=h.other;h.nodes.overviewContent.innerHTML='<div class="match-list">retained</div>';const list=h.nodes.overviewContent.firstChild;h.updateStatus(viewStatus({...identity,connected:false,clientDiscovery:'exiting'}));assert.equal(h.nodes.overviewContent.firstChild,list);assert.equal(h.other.data.matches[0].gameId,2);}finally{h.close();}
});

test('R258 manual selection within ten seconds remains selected on self readiness',()=>{
 const h=harness();try{h.state.activeGroup='kr';h.state.lastManualTabAt=Date.now();h.updateStatus(viewStatus(identity));assert.equal(h.state.activeGroup,'kr');assert.equal(h.state.activeTabs.kr,'other');}finally{h.close();}
});

test('R258 startup visibility diagnostics count actual appearances',async()=>{
 const dom=new JSDOM('<div id="startup-loading" hidden></div>');try{
 const events=[];dom.window.reportFlowDiagnostic=(...event)=>events.push(event);
 compile(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),['installStartupVisibilityDiagnostics'],{}).installStartupVisibilityDiagnostics(dom.window,dom.window.document);
 assert.equal(events.length,0);const overlay=dom.window.document.getElementById('startup-loading');overlay.hidden=false;await new Promise(setImmediate);assert.deepEqual(events,[['self_tab_client','overlay-shown']]);overlay.hidden=true;await new Promise(setImmediate);overlay.hidden=false;await new Promise(setImmediate);assert.equal(events.length,2);
 }finally{dom.window.close();}
});


test('R258 removing the top local overlay restores a retained foreign overlay without rebuilding it',()=>{
 const h=harness();try{
 h.updateStatus(viewStatus(identity));
 const foreign={key:'foreign-overlay',region:'jp1',label:'Foreign'},local={key:'local-overlay',region:'',label:'Local'};
 h.state.overlay=[foreign,local];h.nodes.playerOverlay.hidden=false;h.nodes.playerOverlay.dataset.entryKey=local.key;
 const list=h.document.createElement('div');list.className='match-list';list.textContent='foreign retained';const fragment=h.document.createDocumentFragment();fragment.append(list);
 h.nodes.playerOverlayContent._overviewViewTab=local;h.nodes.playerOverlayContent._overviewViews=new Map([[foreign,{content:fragment,values:{}}]]);
 h.updateStatus(viewStatus({...identity,connected:false,clientDiscovery:'exiting'}));
 assert.deepEqual(h.state.overlay,[foreign]);assert.equal(h.nodes.playerOverlayContent.firstChild,list);assert.equal(h.nodes.playerOverlay.hidden,false);assert.equal(h.nodes.playerOverlay.dataset.entryKey,foreign.key);
 }finally{h.close();}
});

test('R258 cold overview starts before catalogs and optional summaries, capped at twenty',()=>{
 const calls=[],tab={key:'current',current:true,matchFilter:'all'},state={status:{connected:true,clientRegion:'TENCENT'},settings:{matchCount:50},controllers:new Map()};
 const f=compile(gameplay,['loadOverview'],{state,tabGroup:()=> 'players',tabReady:()=>true,connected:()=>true,riotTab:()=>false,clearTimeout:()=>{},performance,rerenderTab:()=>{},loadOPGGSeasonSummary:()=>calls.push('opgg'),loadOverviewCurrentGame:()=>calls.push('current-game'),loadMayhemRating:()=>calls.push('rating'),api:url=>{calls.push(url);return new Promise(()=>{});}});
 void f.loadOverview(tab,true);
 assert.equal(calls.length,1);assert.match(calls[0],/^\/api\/gameplay\/overview\?count=20&/);
 assert.doesNotMatch(gameplay.slice(gameplay.lastIndexOf('  bindSettings();')),/ensurePerks\(\)/);
});

test('R258 ranked supplements require the same account and history head and wait behind a refresh',async()=>{
 const tab={key:'current',data:{player:{playerRef:'player_self'},matches:[{gameId:100}],recentRanked:{games:5},rankedQueues:{420:{seasonGames:20}}}};
 const state={tabs:[tab],destroyed:false};let renders=0;
 const f=compile(gameplay,['handleOverviewIncremental'],{state,connected:()=>true,rerenderTab:()=>renders++});
 const detail={type:'overview-incremental',account:'player_self',headGameId:100,recentRanked:{games:12},rankedQueues:{420:{recentRanked:{games:12}}}};
 assert.equal(await f.handleOverviewIncremental({...detail,account:'other'}),false);
 assert.equal(await f.handleOverviewIncremental({...detail,headGameId:99}),false);
 assert.equal(await f.handleOverviewIncremental({...detail,recentRanked:{games:0}}),false);
 tab.loading=true;assert.equal(await f.handleOverviewIncremental(detail),false);assert.equal(tab.pendingHistoricalRanks,detail);assert.equal(renders,0);
 tab.loading=false;assert.equal(await f.handleOverviewIncremental(detail),true);assert.equal(tab.data.recentRanked.games,12);assert.equal(tab.data.rankedQueues[420].seasonGames,20);
});

test('R258 a late catalog response cannot replace a changed client version',async()=>{
 const state={controllers:new Map()},pending=[];let renders=0;
 const f=compile(gameplay,['ensureItems','ensureSummonerSpells'],{state,api:()=>new Promise(resolve=>pending.push(resolve)),recordItemSetClientDiagnostic:()=>{},rerenderCatalogViews:()=>renders++,clearTimeout:()=>{},setTimeout:()=>0});
 const a=f.ensureItems(),b=f.ensureSummonerSpells();
 state.itemsRequestToken++;state.summonerSpellsRequestToken++;state.items={items:[{id:2}]};state.summonerSpells={spells:[{id:2}]};
 pending[0]({items:[{id:1}]});pending[1]({spells:[{id:1}]});await Promise.all([a,b]);
 assert.equal(state.items.items[0].id,2);assert.equal(state.summonerSpells.spells[0].id,2);assert.equal(renders,0);
});

test('R258 committed catalogs survive same-version reconnect and expire on an actual version change',()=>{
 const h=harness();try {
 h.updateStatus(viewStatus({...identity,clientVersion:'26.19.1'}));h.state.perks={perks:[1]};h.state.items={items:[1]};h.state.summonerSpells={spells:[1]};
 h.updateStatus(viewStatus({...identity,clientVersion:'26.19.1',connected:false,clientDiscovery:'exiting'}));
 assert.equal(h.state.items.items[0],1);assert.equal(h.state.perks.perks[0],1);
 h.updateStatus(viewStatus({...identity,clientVersion:'26.19.1'}));assert.equal(h.state.summonerSpells.spells[0],1);
 h.updateStatus(viewStatus({...identity,clientVersion:'26.20.1'}));assert.equal(h.state.items,null);assert.equal(h.state.perks,null);assert.equal(h.state.summonerSpells,null);
 }finally{h.close();}
});

test('R258 client-view SSE alone builds and removes self without any status pull; stale status cannot resurrect it',()=>{
 const h=harness();try {
 let source,pulls=0;const appState={},events=[];
 class EventSource {constructor(){source=this}addEventListener(){}close(){}}
 const window={EventSource,dispatchEvent:e=>{if(e.type==='deep-legends:client-view'){events.push(e.detail);h.updateClientView(e.detail);}}};
 const f=compile(app,['setupLiveUpdates','acceptClientView'],{state:appState,window,EventSource,CustomEvent:h.dom.window.CustomEvent,clearTimeout:()=>{},renderClientConnection:()=>{},renderLaunchpad:()=>{},clearDisconnectedClientState:()=>{},refreshStatus:()=>pulls++,LIVE_UPDATE_STATE_SLICES:{},updateUI:{},resyncLiveState:()=>{}});
 f.setupLiveUpdates();h.state.status={connected:false};
 const view={type:'client-view',state:'ready',region:'TENCENT',serverId:'HN1',summoner:identity.summoner,sgpReady:true,generation:100};
 source.onmessage({data:JSON.stringify(view)});assert(h.nodes.playerTabs.querySelector('[data-player-tab="current"]'));assert.equal(h.loads.length,1);assert.equal(pulls,0);
 source.onmessage({data:JSON.stringify({...view,state:'exiting',generation:101})});assert(!h.nodes.playerTabs.querySelector('[data-player-tab="current"]'));assert.equal(pulls,0);
 source.onmessage({data:JSON.stringify(view)});h.updateStatus({...identity,clientView:view});assert(!h.nodes.playerTabs.querySelector('[data-player-tab="current"]'));assert.equal(h.state.status.clientView.generation,101);
 h.updateStatus(identity);assert(!h.nodes.playerTabs.querySelector('[data-player-tab="current"]'));assert.equal(events.length,2);assert.equal(h.el.startupLoading.hidden,true);
 }finally{h.close();}
});

test('R258 status legacy fields alone never decide connection visibility',()=>{
 const h=harness();try {h.updateStatus(identity);assert.equal(h.selfTabReady(),false);assert.equal(h.connected(),false);assert.equal(h.loads.length,0);}finally{h.close();}
});

test('R258 closing an inactive tab retains the active list and tab nodes without broad renderers',()=>{
 const h=harness();try {
 h.updateStatus(viewStatus(identity));h.state.activeGroup='kr';h.state.activeTabs.kr='other';h.nodes.overviewContent._overviewViewTab=h.other;
 h.nodes.overviewContent.innerHTML='<div class="match-list">kept</div>';const list=h.nodes.overviewContent.firstChild;
 const closed={key:'closed',region:'jp1',group:'kr',label:'Closed'};h.state.tabs.push(closed);let local=0;
 const f=compile(gameplay,['closePlayerTab','tabGroup','riotTab'],{state:h.state,savePlayerScroll:()=>{},clearTimeout:()=>{},playerTabOrderIdentity:t=>t.key,writeSetting:()=>{},activeTab:()=>h.other,renderPlayerTabs:()=>h.renderPlayerTabWorkspace('kr'),activateOverviewTabPanel:()=>local++,renderOverview:()=>assert.fail('broad overview render')});
 h.renderPlayerTabWorkspace('kr');const keptTab=h.nodes.playerTabs.querySelector('[data-player-tab-wrap="other"]');
 f.closePlayerTab('closed');assert.equal(local,0);assert.equal(h.nodes.overviewContent.firstChild,list);assert.equal(h.nodes.playerTabs.querySelector('[data-player-tab-wrap="other"]'),keptTab);
 }finally{h.close();}
});

test('R258 row placeholder observes only a collapsed summary and disposes its observer with the view',()=>{
 const dom=new JSDOM('<div class="match-list"><article class="match-entry"><div class="match-summary"></div><div class="match-details"></div></article></div>');try {
 const root=dom.window.document.querySelector('.match-list'),summary=root.querySelector('.match-summary');let observer,disconnects=0;
 class ResizeObserver{constructor(callback){observer=this;this.callback=callback;}observe(target){this.target=target;}disconnect(){disconnects++;}}
 const f=compile(gameplay,['observeMatchRowHeight','disposeMatchRowHeight'],{ResizeObserver});
 f.observeMatchRowHeight(root);assert.equal(observer.target,summary);observer.callback([{target:summary,borderBoxSize:[{blockSize:122}]}]);assert.equal(root.style.getPropertyValue('--match-row-height'),'124px');
 f.observeMatchRowHeight(root);assert.equal(disconnects,0);root.querySelector('.match-details').textContent='expanded';observer.callback([{target:summary,borderBoxSize:[{blockSize:122}]}]);assert.equal(root.style.getPropertyValue('--match-row-height'),'124px');
 f.disposeMatchRowHeight(root);assert.equal(disconnects,1);assert.equal(root._rowHeightObserver,undefined);
 }finally{dom.window.close();}
});


test('R258 automatic self follow cancels the foreign search and account changes reset only self conditions',()=>{
 const af=require('./history-filters.js'),previous=globalThis.deepLegendsHistoryFilters;
 globalThis.deepLegendsHistoryFilters=af;const h=harness();
 try {
  h.state.activeGroup='kr';h.state.lastManualTabAt=Date.now()-20000;
  h.other.advancedSearch={running:true,token:1};h.other.advancedConditions={hero:{values:['103'],not:false}};
  h.updateStatus(viewStatus(identity));assert.equal(h.other.advancedSearch.running,false);assert.deepEqual(h.other.advancedConditions.hero.values,['103']);
  h.current.advancedConditions={hero:{values:['103'],not:false}};h.current.matchViews=new Map([['solo',{matches:[{gameId:999}]}]]);
  h.updateStatus(viewStatus({...identity,summoner:{...identity.summoner,playerRef:'player_second'}}));
  assert.deepEqual(h.current.advancedConditions,{});assert.equal(h.current.matchViews.size,0);assert.deepEqual(h.other.advancedConditions.hero.values,['103']);
 } finally {h.close();if(previous===undefined)delete globalThis.deepLegendsHistoryFilters;else globalThis.deepLegendsHistoryFilters=previous;}
});

test('R258 account reset cancels a pending mode page before the advanced module loads',async()=>{
 const previous=globalThis.deepLegendsHistoryFilters;delete globalThis.deepLegendsHistoryFilters;
 let finish,requests=0,renders=0,aborts=0;
 const tab={key:'current',matchFilter:'solo',data:{matches:[],pagination:{hasMore:true}},nextBegIndex:20};
 const state={controllers:new Map([['overview-more:current',{abort:()=>aborts++}]]),settings:{matchCount:20}};
 const f=compile(gameplay,['resetPlayerHistoryConditions','cancelAdvancedMatchSearch','autoLoadMatchFilter'],{state,
  disposeMatchRowHeight:()=>{},filteredMatches:()=>[],renderFilteredMatchView:()=>renders++,
  loadOverview:()=>{requests++;return new Promise(resolve=>{finish=resolve;});}});
 try {
  const pending=f.autoLoadMatchFilter(tab);assert.equal(requests,1);
  f.resetPlayerHistoryConditions(tab);const token=tab.filterPagingToken;
  assert.equal(aborts,1);assert.equal(tab.filterPaging,false);assert.equal(tab.filterPagingPage,0);
  tab.data={matches:[{gameId:2}],pagination:{hasMore:true}};
  tab.filterPaging=true;tab.filterPagingPage=7;const before=renders;
  finish(true);await pending;
  assert.equal(requests,1);assert.equal(tab.filterPagingToken,token);
  assert.equal(tab.filterPaging,true);assert.equal(tab.filterPagingPage,7);assert.equal(renders,before);
  assert.equal(tab.data.matches[0].gameId,2);
 } finally {if(previous!==undefined)globalThis.deepLegendsHistoryFilters=previous;}
});

test('R258 catalogs arriving together invalidate views once at the next frame',()=>{
 const state={},frames=[];let renders=0;
 const f=compile(gameplay,['scheduleCatalogViews'],{state,requestAnimationFrame:fn=>frames.push(fn),rerenderCatalogViews:()=>renders++});
 f.scheduleCatalogViews();f.scheduleCatalogViews();f.scheduleCatalogViews();
 assert.equal(frames.length,1);assert.equal(renders,0);
 frames.shift()();assert.equal(renders,1);assert.equal(state.catalogRenderPending,false);
 f.scheduleCatalogViews();assert.equal(frames.length,1);
 state.destroyed=true;frames.shift()();assert.equal(renders,1);
});
