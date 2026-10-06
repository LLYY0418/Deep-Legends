'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile,escapeHTML}=require('./r188-harness.cjs'),{expand}=require('./r211-harness-support.cjs');
const source=read('gameplay.js');
const functions=(names,deps)=>compile(source,expand(source,names,deps),deps);
const iconFigure=(_kind,id,name,_class,tooltip=true)=>`<span${tooltip?' data-tooltip="hero"':''}><img src="/${id}" alt="${name}"></span>`;
const row=(id,opponents=[])=>({championId:id,championName:`英雄${id}`,games:40-id,wins:20,losses:10,winRate:60,kda:3,opponents});
test('R231 table single expansion and sorting preserve every main row/avatar; more is aligned and appended',()=>{
 const dom=new JSDOM('<main></main>'),document=dom.window.document,root=document.querySelector('main');
 const data={queue:'420',overall:row(0),rows:[row(1,Array.from({length:20},(_,i)=>row(i+20))),row(2,[row(44)]),row(3)]},tab={overviewSubpageState:{data}};
 const f=functions(['renderChampionTable','bindChampionTable'],{document,escapeHTML,iconFigure,prepareImages:()=>{},openOverviewSubpage:()=>{}});
 root.innerHTML=f.renderChampionTable(data,tab);f.bindChampionTable(root,tab);
 const rows=[...root.querySelectorAll('tr[data-table-row^="champion:"]')],images=rows.map(r=>r.querySelector('img')),overall=root.querySelector('.is-overall');
 const more=root.querySelector('[data-table-more]');assert.equal(more.parentElement.tagName,'TH');assert(more.parentElement.matches('.champion-table-hero'));assert.equal(more.closest('tr').firstElementChild.tagName,'TD');assert.equal(more.closest('tr').firstElementChild.textContent,'');
 const firstOpponent=root.querySelector('tr[data-table-row^="opponent:"]');more.click();assert.equal(root.querySelectorAll('tr[data-table-row^="opponent:"]').length,15);assert.equal(root.querySelector('tr[data-table-row^="opponent:"]'),firstOpponent);
 root.querySelector('[data-table-expand="2"]').click();assert.equal(root.querySelectorAll('tr[data-table-row^="opponent:"]').length,1);assert.equal(root.querySelector('[data-table-expand="1"]').getAttribute('aria-expanded'),'false');assert.equal(root.querySelector('[data-table-expand="2"]').getAttribute('aria-expanded'),'true');
 root.querySelector('[data-table-sort="games"]').click();assert.equal(root.querySelector('.is-overall'),overall);assert.deepEqual([...root.querySelectorAll('tr[data-table-row^="champion:"]')].map(r=>r.firstElementChild.textContent),['1','2','3']);
 for(let i=0;i<rows.length;i++){assert(rows[i].isConnected);assert.equal(rows[i].querySelector('img'),images[i]);assert.equal(images[i].getAttribute('src'),`/${i+1}`)}
 dom.window.close();
});
test('R231 arrows/count, unique multikill gradients and masked build avatar tooltip',()=>{
 const state={settings:{maskNames:true}},f=functions(['renderMasteries','renderChampionStats','renderMultiKillTag','renderBuildPlayers','matchPlayerGroups'],{state,escapeHTML,iconFigure,number:String,compactNumber:String,kda:String,percent:String});
 const mastery=f.renderMasteries([],true,37),champions=f.renderChampionStats([],{games:20},{tableSupported:true});assert.match(mastery,/37 个英雄/);assert.doesNotMatch(mastery,/最高分|›/);assert.match(champions,/<svg viewBox="0 0 16 16"/);
 const dom=new JSDOM(f.renderMultiKillTag(5,9)+f.renderMultiKillTag(5,9));const ids=[...dom.window.document.querySelectorAll('linearGradient')].map(e=>e.id);assert.equal(new Set(ids).size,4);for(const path of dom.window.document.querySelectorAll('.mk-plate'))assert(ids.includes(path.getAttribute('fill').slice(5,-1)));
 const match={gameId:9,participants:[{participantId:1,teamId:100,gameName:'SecretSelf',tagLine:'S',championId:1},{participantId:2,teamId:200,gameName:'SecretOther',tagLine:'O',championId:2}]};dom.window.document.body.innerHTML=f.renderBuildPlayers(match,match.participants[0],match.participants[1]);
 assert.equal(dom.window.document.querySelectorAll('.build-player [data-tooltip]').length,0);assert(!dom.window.document.body.innerHTML.includes('Secret'));dom.window.close();
});
test('R231 season progress fetches only snapshot and keeps match list/banners/career container',async()=>{
 const dom=new JSDOM('<main><section class="summoner-strip"></section><aside class="career-column"><section class="rank-career-section">old</section><section class="champion-performance">old</section></aside><div class="match-list"></div></main>'),document=dom.window.document,root=document.querySelector('main'),tab={key:'self',data:{player:{playerRef:'ref'},ranks:[],seasonStatsProgress:{season:'S26',scanned:10,complete:false}}},calls=[];
 const state={section:'overview',seasonProgressRefreshes:new Map()},refs=[...root.children];
 const f=functions(['handleSeasonProgress','refreshSeasonSummary'],{state,document,overviewContainer:()=>root,overviewGroupForSection:()=> 'players',overviewSectionForGroup:()=> 'overview',activeTab:()=>tab,api:async url=>{calls.push(url);return {seasonStatsProgress:{season:'S26',scanned:30,complete:true},seasonChampionStats:[],seasonOverall:{}}},careerSectionEntries:()=>[['ranks','<section class="rank-career-section">new</section>'],['champions','<section class="champion-performance">new</section>']],bindOverviewContent:()=>{},prepareImages:()=>{},reportOverviewCardReady:()=>{},requestAnimationFrame:fn=>fn()});
 assert.equal(await f.handleSeasonProgress({type:'season-progress',season:'S26',account:'ref',scanned:30,complete:true},10000),true);assert.equal(calls.length,1);assert.match(calls[0],/^\/api\/gameplay\/season-summary\?/);assert(refs.every(e=>e.isConnected));assert.equal(root.querySelector('.champion-performance').textContent,'new');dom.window.close();
});

async function checkSeasonProgressThrottle(gameplaySource) {
  let now = 10_000, nextTimer = 0;
  const timers = new Map(), reads = [];
  const progress = {season: 'S26', scanned: 10, complete: false};
  const tab = {key: 'self', data: {player: {playerRef: 'ref'}, seasonStatsProgress: progress}};
  const state = {section: 'overview', seasonProgressRefreshes: new Map()};
  let snapshot = {...progress};
  const f = compile(gameplaySource, ['handleSeasonProgress'], {
    state, Date: {now: () => now}, document: {getElementById: () => null},
    overviewGroupForSection: () => 'players', overviewSectionForGroup: () => 'overview',
    activeTab: () => tab, markSeasonRefreshPending: () => {}, requestAnimationFrame: fn => fn(),
    setTimeout: (fn, delay) => { const id = ++nextTimer; timers.set(id, {fn, at: now + delay}); return id; },
    clearTimeout: id => timers.delete(id),
    refreshSeasonSummary: async () => { reads.push(now); Object.assign(progress, snapshot); },
  });
  const send = (scanned, complete = false) => {
    snapshot = {season: 'S26', scanned, complete};
    return f.handleSeasonProgress({type: 'season-progress', account: 'ref', ...snapshot});
  };
  const advance = async target => {
    for (;;) {
      const pending = [...timers].sort((a, b) => a[1].at - b[1].at)[0];
      if (!pending || pending[1].at > target) break;
      now = pending[1].at; timers.delete(pending[0]); pending[1].fn();
      await Promise.resolve();
    }
    now = target;
  };

  assert.equal(await send(20), true);
  now = 10_500;
  assert.equal(await send(30), false, '3 秒内不应刷新中间进度');
  now = 11_500;
  assert.equal(await send(40), false);
  assert.equal(timers.size, 1, '连续事件应共用一个尾部定时器');
  assert.equal([...timers.values()][0].at, 13_000, '窗口从上次刷新计时，不随事件顺延');
  assert.equal(state.seasonProgressRefreshes.get('ref').detail.scanned, 40);
  await advance(12_999);
  assert.deepEqual(reads, [10_000]);
  await advance(13_000);
  assert.deepEqual(reads, [10_000, 13_000]);
  assert.equal(progress.scanned, 40);

  now = 13_300;
  assert.equal(await send(50), false);
  assert.equal(timers.size, 1);
  now = 13_400;
  assert.equal(await send(60, true), true, '完成事件立即刷新');
  assert.equal(timers.size, 0, '完成事件取消待执行的尾部刷新');
  await advance(16_000);
  assert.deepEqual(reads, [10_000, 13_000, 13_400]);
  assert.equal(progress.complete, true);
}

test('R231 season progress throttles intermediate events for 3 seconds and completion bypasses it', async () => {
  await checkSeasonProgressThrottle(source);
});

test('R231 throttle test rejects the zero-window mutation', async () => {
  const mutant = source.replace('const remaining = 3_000 - (now - previous.lastAt);',
    'const remaining = 0 - (now - previous.lastAt);');
  assert.notEqual(mutant, source);
  await assert.rejects(() => checkSeasonProgressThrottle(mutant),
    error => error instanceof assert.AssertionError && error.message.includes('3 秒内不应刷新'));
});

test('R231 abort on soft reset leaves mayhem idle, then next load returns score',async()=>{
 const tab={key:'one',playerRef:'ref',mayhemRating:{status:'idle',playerRef:'ref'}},state={tabs:[tab],overlay:[],controllers:new Map(),liveRequestToken:0};let reject,requests=0;
 state.controllers.set('mayhem-rating:one',{abort:()=>reject?.(Object.assign(Error('cancel'),{name:'RequestCancelled'}))});
 const f=functions(['loadMayhemRating','mayhemRatingStatus','softResetGameplayState'],{state,riotTab:()=>false,tabServerID:()=> 'HN1',rerenderTab:()=>{},resetLiveGameScopedState:()=>{},api:()=>{requests++;return requests===1 ? new Promise((_,r)=>reject=r) : Promise.resolve({available:true,rating:2300})}});
 const first=f.loadMayhemRating(tab);assert.equal(tab.mayhemRating.status,'loading');f.softResetGameplayState();await first;assert.equal(tab.mayhemRating.status,'idle');await f.loadMayhemRating(tab);assert.equal(requests,2);assert.equal(tab.mayhemRating.data.rating,2300);
});
test('R231 collapse resets selected build player; detail-tab changes retain it; filter clears all',async()=>{
 const dom=new JSDOM('<article><button data-toggle-match="9"></button></article>'),tab={openMatches:new Set(['9']),matchDetailTabs:new Map([['9','build']]),buildPlayers:new Map([['9',2]]),matchFilter:'all',data:{pagination:{hasMore:false},matches:[]}},diagnostics=[];
 const f=functions(['bindMatchEntryControls','updateMatchFilter','buildSubject'],{state:{controllers:new Map()},fetch:async(_url,options)=>{diagnostics.push(JSON.parse(options.body));return {}},replaceMatchEntry:()=>{},riotTab:()=>false,rememberMatchScrollTop:()=>{},matchObserverKey:()=> 'observer',renderFilteredMatchView:()=>{},loadOverview:()=>{throw Error('unneeded network')}});
 const self={participantId:1},other={participantId:2},match={gameId:9,participants:[self,other]};assert.equal(f.buildSubject(match,self,tab),other);tab.matchDetailTabs.set('9','overview');assert.equal(f.buildSubject(match,self,tab),other);
 f.bindMatchEntryControls(dom.window.document.querySelector('article'),tab,()=>{});dom.window.document.querySelector('button').click();assert.equal(f.buildSubject(match,self,tab),self);tab.buildPlayers.set('9',2);await f.updateMatchFilter(tab,'flex');assert.equal(tab.buildPlayers.size,0);assert.deepEqual(diagnostics.map(e=>e.reason),['reset-collapse','reset-filter']);dom.window.close();
});
test('R231 actual main and overlay enter both detail pages three times and restore identical nodes without overview/tier requests',async()=>{
 const {bootDemoApp,settled}=require('../../desktop/overview-render-helpers.cjs');const {window:w,errors}=bootDemoApp();const original=w.fetch,calls=[];
 w.fetch=async(url,options)=>{calls.push(String(url));if(String(url).startsWith('/api/gameplay/masteries'))return new Response(JSON.stringify({available:true,items:[],totalChampions:173}));if(String(url).startsWith('/api/gameplay/champion-table'))return new Response(JSON.stringify({available:true,queue:'420',rows:[],overall:{games:0}}));const response=await original(url,options);if(String(url).startsWith('/api/gameplay/overview')){const data=await response.json();data.seasonStatsProgress.tableSupported=true;data.seasonOverall=data.overall;data.seasonChampionStats=data.championStats;data.masteryChampionCount=37;data.capabilities.push({name:"champion-mastery",state:"available"});return new Response(JSON.stringify(data))}return response};
 try {await settled();const nodes=['.career-column','.match-list','.summoner-strip'].map(sel=>w.document.querySelector(sel));assert(nodes.every(Boolean));const baseline=calls.filter(url=>/overview|match-tiers/.test(url)).length;
 for(const page of ['masteries','champion-table'])for(let i=0;i<3;i++){assert(w.document.querySelector(`[data-overview-subpage="${page}"]`),JSON.stringify({page,i,errors,headers:[...w.document.querySelectorAll(".career-section header")].map(e=>e.textContent),calls:calls.filter(e=>e.includes("overview"))}));w.document.querySelector(`[data-overview-subpage="${page}"]`).click();await new Promise(r=>setTimeout(r,30));assert(w.document.querySelector('[data-overview-return]'),JSON.stringify({page,i,errors,text:w.document.querySelector('#overview-content')?.textContent?.slice(0,400),calls:calls.slice(-6)}));w.document.querySelector('[data-overview-return]').click();await new Promise(r=>setTimeout(r,30));for(let j=0;j<nodes.length;j++)assert.equal(w.document.querySelector(['.career-column','.match-list','.summoner-strip'][j]),nodes[j])}
 assert.equal(calls.filter(url=>/overview|match-tiers/.test(url)).length,baseline);
 w.dispatchEvent(new w.CustomEvent('deep-legends:open-player',{detail:{playerRef:'r231-overlay-player',gameName:'OverlayFixture',serverId:'HN1',source:'champions'}}));await new Promise(r=>setTimeout(r,150));
 const root=w.document.querySelector('#player-overlay-content'),overlayNodes=['.career-column','.match-list','.summoner-strip'].map(sel=>root.querySelector(sel));assert(overlayNodes.every(Boolean));const overlayBaseline=calls.filter(url=>/overview|match-tiers/.test(url)).length;
 for(const page of ['masteries','champion-table'])for(let i=0;i<3;i++){root.querySelector(`[data-overview-subpage="${page}"]`).click();await new Promise(r=>setTimeout(r,30));root.querySelector('[data-overview-return]').click();await new Promise(r=>setTimeout(r,30));for(let j=0;j<overlayNodes.length;j++)assert.equal(root.querySelector(['.career-column','.match-list','.summoner-strip'][j]),overlayNodes[j])}
 assert.equal(calls.filter(url=>/overview|match-tiers/.test(url)).length,overlayBaseline);assert.deepEqual(errors,[]);
 } finally {w.dispatchEvent(new w.Event('beforeunload'));w.close()}
});

test('R231 actual external card restores self after collapse and retains opponent across detail tabs',async()=>{
 const {bootDemoApp,settled}=require('../../desktop/overview-render-helpers.cjs');const {window:w,errors}=bootDemoApp();
 try {await settled();const overview=await (await w.fetch('/api/gameplay/overview')).json();const match=overview.matches[0],root=w.document.createElement('main');w.document.body.append(root);const view=w.deepLegendsMatchCards.mount(root,{matches:[match],playerRef:overview.player.playerRef,key:'r231-external'});
 const click=selector=>{const button=root.querySelector(selector);assert(button,selector);button.click()};const settle=()=>new Promise(r=>setTimeout(r,20));
 click('[data-toggle-match]');click('[data-match-detail="build"]');await settle();const other=root.querySelector('.build-player:not(:has(.build-player-self))');assert(other);other.click();await settle();const selected=root.querySelector('.build-player.is-selected').dataset.buildPlayer;
 click('[data-match-detail="overview"]');click('[data-match-detail="build"]');assert.equal(root.querySelector('.build-player.is-selected').dataset.buildPlayer,selected);
 click('[data-toggle-match]');click('[data-toggle-match]');click('[data-match-detail="build"]');assert(root.querySelector('.build-player.is-selected .build-player-self'));assert.deepEqual(errors,[]);view.destroy();
 } finally {w.close()}
});
