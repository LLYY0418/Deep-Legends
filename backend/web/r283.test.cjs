'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,extract}=require('./r188-harness.cjs');
const game=fs.readFileSync(process.env.R283_GAMEPLAY_SOURCE||__dirname+'/gameplay.js','utf8');
test('R283 P1 merges preserve delta and all retained tail rows',()=>{
 const f=compile(game,['mergeMatchLP','mergeCurrentHistoryHead']);
 const a={gameId:1,lpDelta:18,revision:'a'},b={gameId:2},c={gameId:3};
 assert.deepEqual(f.mergeCurrentHistoryHead([a,b,c],[{gameId:9},{gameId:1,revision:'a'}]).map(m=>m.gameId),[9,1,2,3]);
 assert.equal(f.mergeMatchLP(a,{gameId:1}).lpDelta,18);
 assert(!extract(game,'handleLPRecorded').includes('loadOverview('));
});
test('R283 P2 manual refresh preserves SSE and waits for backend completion',async()=>{
 const app=fs.readFileSync(process.env.R283_APP_SOURCE||__dirname+'/app.js','utf8');
 assert(!extract(app,'softResetShellState').includes('setupLiveUpdates()'));
 const dom=new JSDOM('<button><span class="refresh-icon"></span><span>重新读取</span></button>'),w=dom.window,state={status:{syncing:false,connected:true}},labels=[],reports=[];
 const button=w.document.querySelector('button');let reads=0,done;
 const deps={state,el:{refresh:button},window:w,CustomEvent:w.CustomEvent,showToast(){},softResetShellState(){},api:async()=>{},refreshStatus:async()=>{reads++;state.status.syncing=reads<2},setTimeout,reportFlowDiagnostic(){},renderStatus(){f.setRefreshBusy(state.status.syncing||state.manualRefreshing)}};
 w.addEventListener('deep-legends:hard-refresh',e=>e.detail.waitFor.push(new Promise(r=>done=r)));
 w.reportFlowDiagnostic=(...v)=>reports.push(v);
 const f=compile(app,['runManualRefresh','setRefreshBusy'],deps),task=f.runManualRefresh();
 await new Promise(r=>setImmediate(r));assert.equal(button.lastElementChild.textContent,'正在获取');assert(state.status.syncing);done();await task;
 assert.equal(button.lastElementChild.textContent,'重新读取');assert.equal(reports[0][2].label_changes,2);assert(reads>=2);dom.window.close();
});
test('R283 P15 empty overview group renders without touching null tab',()=>{
 let rendered=0;
 const state={section:'live',beacon:{}},deps={state,clearTimeout,clearRuneStarterRetries(){},setPlayerGroupMenu(){},closeOverlay(){},overviewGroupForSection:()=> 'foreign',activeTab:()=>null,tabReady:()=>true,renderOverview(){rendered++},shouldReloadOverview:()=>true,scheduleLiveRefresh(){},renderBeacon(){}};
 const f=compile(game,['activateSection'],deps);assert.doesNotThrow(()=>f.activateSection('overview'));assert.equal(rendered,1);
});
test('R283 P5 prestige paints first screen without idle callbacks',()=>{
 const app=fs.readFileSync(process.env.R283_APP_SOURCE||__dirname+'/app.js','utf8');
 for(const total of [24,48,49,461]) {
  const frames=[],state={renderGeneration:1,renderFrames:new Set()},deps={state,requestAnimationFrame:fn=>(frames.push(fn),frames.length)};
  const f=compile(app,['runPrestigeTasks'],deps);let cards=0,groups=0;
  f.runPrestigeTasks(Array.from({length:total},()=>()=>cards++),1,()=>groups++);
  assert.equal(cards,total<=48?total:24);assert.equal(groups,total<=48?1:0);
  while(frames.length){const before=cards;frames.shift()();assert(cards-before<=24)}assert.equal(cards,total);assert.equal(groups,1);
 }
});
test('R283 P6 P7 P8 P9 P10 layout guards',()=>{
 const css=fs.readFileSync(process.env.R283_GAMEPLAY_CSS_SOURCE||__dirname+'/gameplay.css','utf8'),appcss=fs.readFileSync(process.env.R283_APP_CSS_SOURCE||__dirname+'/app.css','utf8');
 assert.match(css,/summoner-strip-copy>div\s*\{[^}]*display:flex/);assert(!css.includes('flex:0 0 80px'));assert(css.includes('.summoner-level-value'));
 assert.match(appcss,/\.global-tooltip\s*\{[^}]*zoom:\s*var\(--ui-zoom/);assert(!appcss.includes('font-size: calc(11.5px * var(--ui-zoom'));
 assert.match(css,/\[data-theme="light"\] :is\(\.arena-augment-icon,\.augment-icon\)\s*\{background:#1d2533/);
 assert.match(appcss,/facade-detail-dialog \.dialog-art\s*\{[^}]*width: 100%[^}]*margin: 0/);
 assert.match(css,/match-loadout-mini\.is-augments\.is-mayhem\s*\{[^}]*grid-auto-flow:column/);
});
test('R283 P11 rift dual queues and ARAM solo only',()=>{
 const arena=require(process.env.R283_ARENA_SOURCE||'./live-arena.js');globalThis.deepLegendsMatchCards={live:{rankTitle:r=>r.tier+' '+(r.division||'')}};
 try {
  const solo={tier:'PLATINUM',division:'I'},flex={tier:'DIAMOND',division:'IV'};
  for(const player of [{soloRank:solo,flexRank:flex},{soloRank:solo},{}]) {
   const html=arena.identityLine(player,'rift','上单 · 旧段位');assert(html.includes('单双 '));assert(html.includes('组排 '));assert(html.includes('data-tooltip-overflow="self"'));
  }
  const aram=arena.identityLine({soloRank:solo,flexRank:flex},'aram','');assert(aram.includes('platinum.png'));assert(!aram.includes('diamond.png'));
  assert.equal(arena.identityLine({},'arena',''),'');
 }finally{delete globalThis.deepLegendsMatchCards}
});
test('R283 P13 career identity and explicit selection',()=>{
 const css=fs.readFileSync(process.env.R283_SUITE_CSS_SOURCE||__dirname+'/suite.css','utf8');
 assert.match(css,/\.facade-identity\s*\{[^}]*display:flex/);assert(css.includes('.facade-commit button {font-size:13px;}'));
 assert.match(css,/\.facade-film button\.is-selected\s*\{[^}]*border:2px/);assert(css.includes('.facade-film button.is-selected::after'));
});
test('R283 P14 collection return resets scroll without changing choices',()=>{
 const app=fs.readFileSync(process.env.R283_APP_SOURCE||__dirname+'/app.js','utf8');
 const state={sectionScroll:{favorites:500},favoritesPage:'account',view:'chromas'},calls=[];
 compile(app,['restoreSectionScroll'],{state,el:{appScroll:{scrollTo:arg=>calls.push(arg)}}}).restoreSectionScroll('favorites');
 assert.equal(calls[0].top,0);assert.equal(state.favoritesPage,'account');assert.equal(state.view,'chromas');
 const body=extract(app,'resetCollectionReturn');assert(body.includes('stopHoverVideo()'));assert(body.includes('scrollLeft=0'));assert(body.includes('closeNativeSelectMenu'));
 assert(extract(app,'activateSection').includes('activateFavoritesPage(state.favoritesPage, fresh)'));
 let loads=0,rescans=0,resets=0;
 const tab={dataset:{favoritesPage:'collection'},getAttribute:()=> 'collection'},panel={id:'collection'},returnState={favoritesPage:'collection',section:'favorites',view:'chromas',query:'安妮'};
 const f=compile(app,['activateFavoritesPage'],{state:returnState,el:{favoritesTabs:[tab],favoritesCollectionPanel:panel,favoritesAccountPanel:{},favoritesPoolsPanel:{},favoritesFacadePanel:{},topbarSubtitle:{},pageIntro:{}},activateTab:(tab,_tabs,callback)=>callback(tab),resetCollectionControls:()=>resets++,triggerCollectionRescanIfDirty:()=>rescans++,loadSkins:()=>loads++});
 f.activateFavoritesPage('collection',true);assert.equal(loads,1);assert.equal(rescans,1);assert.equal(resets,0);assert.equal(returnState.view,'chromas');assert.equal(returnState.query,'安妮');
});
test('R283 P17 one strict outage message, no row retry or duplicate stopped note',()=>{
 const state={beacon:{phase:'InProgress'},liveRetryAttempts:8},f=compile(game,['renderLiveRefreshStatus'],{state,liveGamePhase:()=>true,escapeHTML:String,serviceOutageIcon:()=>'<svg></svg>'});
 for(const queueId of [420,1750])for(const down of [true,false]) {
  const data={phase:'InProgress',queueId,historyServiceDown:down,players:Array.from({length:18},()=>({historyState:'failed'}))};
  const html=f.renderLiveRefreshStatus(data);assert.equal((new JSDOM(html).window.document.body.textContent.match(/官方战绩服务暂时中断/g)||[]).length,down?1:0);assert(!html.includes('数据尚未完整'));if(down)assert(html.includes('data-tooltip-overflow="self"'));
 }
 assert(!extract(game,'renderInsightMatches').includes('data-live-history-retry'));assert(!game.includes('bindLiveNode(scope?.querySelector("[data-live-history-retry]"'));
 const css=fs.readFileSync(process.env.R283_GAMEPLAY_CSS_SOURCE||__dirname+'/gameplay.css','utf8');assert(!css.includes('.insight-retry'));
});
test('R283 P12 bravery exactly matches appendix SVG without transport metadata',()=>{
 const svg=fs.readFileSync(process.env.R283_BRAVERY_SOURCE||__dirname+'/bravery.svg','utf8');assert(svg.includes('id="bv2-ring"'));assert(svg.includes('id="bv2-mark"'));assert(!/c2pa|metadata/i.test(svg));
});
test('R283 P1 unchanged SSE and LP patches retain row and image nodes',()=>{
 const dom=new JSDOM('<main><div class="match-list"></div></main>'),document=dom.window.document,list=document.querySelector('.match-list'),reports=[];
 const tab={current:true,matchFilter:'all',data:{player:{playerRef:'self',isCurrent:true},matches:[{gameId:1,queueId:420,revision:'v1',lpDelta:18},{gameId:2,queueId:440,revision:'v1',lpPending:true}],pagination:{}},openMatches:new Set()},state={tabs:[tab],overlay:[]};
 const deps={state,document,window:{reportFlowDiagnostic:(e,r,f)=>reports.push(f)},filteredMatches:r=>r,bindOverviewContent(){},applyRenderedMetricStyles(){},prepareImages(){},boundMatchDOM(){},overviewContainer:()=>document.querySelector('main'),syncOverviewStreak(){},computeOverviewStreak:()=>null,appendOverviewMatches(){},bindMatchFilterControls(){},renderMatchFilters:()=>'',renderMatch:m=>`<article class="match-entry" data-match-id="${m.gameId}"><img src="/image"><span class="result-word">${m.lpDelta!=null?'<b class="lp-delta">+18 LP</b>':'<b class="lp-delta is-pending"></b>'}</span></article>`};
 let f;deps.renderFilteredMatchView=()=>f.reconcileFilteredMatchList(list,tab);
 f=compile(game,['reconcileFilteredMatchList','handleOverviewMatches','handleLPRecorded'],deps);f.reconcileFilteredMatchList(list,tab);const row=list.firstChild,images=[...list.querySelectorAll('img')];
 f.handleOverviewMatches({account:'self',replace:true,matches:[{gameId:1,queueId:420,revision:'v2'},{gameId:2,queueId:440,revision:'v2',lpPending:true}]});
 assert.equal(list.firstChild,row);assert(images.every((img,i)=>img===list.querySelectorAll('img')[i]));assert.equal(reports.at(-1).lost,0);assert.equal(reports.at(-1).images_recreated,0);
 const pendingRow=list.children[1];f.handleLPRecorded({matches:[{gameId:2,lpDelta:20,lpPending:false}]});assert.equal(list.children[1],pendingRow);assert.equal(pendingRow.querySelector('.lp-delta').textContent,'+20 LP');assert(!pendingRow.querySelector('.is-pending'));assert(images.every((img,i)=>img===list.querySelectorAll('img')[i]));dom.window.close();
});
