'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const js=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8'),css=fs.readFileSync(path.join(__dirname,'gameplay.css'),'utf8');
function renderer() {
 const state={settings:{liveOrder:'win-rate'}},events=[];
 const deps={state,window:{reportFlowDiagnostic:(...args)=>events.push(args)},escapeHTML,
 liveAugmentRecommendationSource:d=>d.gameMode==='CHERRY'?'arena':'',recordLiveRosterRendered:()=>{},isSummonersRiftMatch:()=>false,
 insightTeamLayout:()=>({teams:new Map(),reason:''}),renderLiveRosterNoticeBars:()=>'',renderLiveTeamPortraitTags:()=>'',
 renderLivePlayer:p=>`<article class="live-player">${p.playerRef}</article>`,renderInsightMatches:()=>'<div class="history"></div>',renderLiveRecentPositions:()=>'',isARAMRelatedMatch:()=>false,champSelectEnemyPlaceholder:()=>false,arenaTeamMeta:()=>null};
 return {...compile(js,['orderLivePlayers','clusterPremadePlayers','livePremadeRoster','arenaLivePlayerGroups','renderLiveInsights'],deps),state,events};
}
test('R224 18-player Arena dedup and squad-first across every sort with stable refresh',()=>{
 const f=renderer(),players=Array.from({length:18},(_,i)=>({playerRef:`p${i}`,teamId:100,isCurrent:i===0,mySquad:[0,4,11].includes(i),position:i%2?'top':'middle',modeStats:{winRate:100-i,kda:i}}));
 for(const order of ['team','win-rate','kda','position']){
  f.state.settings.liveOrder=order;
  const data={gameId:224,gameMode:'CHERRY',phase:'InProgress',players:[...players,{...players[0],championId:999}]};
  const dom=new JSDOM(f.renderLiveInsights(data));
  const keys=[...dom.window.document.querySelectorAll('[data-live-player-row^="card:"]')].map(row=>row.dataset.livePlayerRow);
  assert.equal(keys.length,18);assert.equal(keys[0],'card:p0');assert.deepEqual(new Set(keys.slice(0,3)),new Set(['card:p0','card:p4','card:p11']));
  assert.equal(f.renderLiveInsights(data),f.renderLiveInsights(data));dom.window.close();
 }
 assert.equal(f.events.length,1);assert.deepEqual(f.events[0],['live_roster_duplicate_dropped','deduplicated',{gameId:224,queueId:0,count:1}]);
 const groups=players.map((p,i)=>({...p,arenaGroup:String(Math.floor(i/3)+1),isCurrent:i===14}));
 const ordered=f.arenaLivePlayerGroups({phase:'InProgress',arenaGrouped:true},groups);
 assert.equal(ordered[0].key,'5');assert.equal(ordered[0].players[0].playerRef,'p14');
 const dom=new JSDOM(f.renderLiveInsights({phase:'ChampSelect',gameMode:'CHERRY',players:players.slice(0,3).reverse()}));
 assert.equal(dom.window.document.querySelector('[data-live-player-row]').dataset.livePlayerRow,'card:p0');dom.window.close();
});
function patchFixture(){
 const dom=new JSDOM('<div id="app-scroll"><main><div data-live-body></div></main></div>',{pretendToBeVisual:true}),document=dom.window.document,content=document.querySelector('main'),reports=[];
 const funcs=compile(js,['liveBodyChrome','captureLiveScroll','restoreLiveScroll','stampLiveRows','preserveLiveImages','patchLiveRosterPanel','updateLivePanels'],{document,nodes:{liveContent:content},bindLiveContent(){},bindLivePanelScope(){},applyRenderedMetricStyles(){},prepareImages(){},recordLiveRenderRebuild:(parts,stats)=>reports.push({parts,stats})});
 return {dom,document,content,reports,...funcs};
}
function markup(keys=['a','b'],{notice='',selected='build',loading=false,label='详情',extra=false}={}){
 return `<div data-live-notice="squad">${notice?`<p class="arena-my-squad-notice">${notice}</p>`:''}</div><section class="recommendation-area"><div data-live-notice="data"></div><div class="recommendation-tab-row"><div class="recommendation-tabs"><button data-recommendation-tab="build" aria-selected="${selected==='build'}">出装</button><button data-recommendation-tab="insight" aria-selected="${selected==='insight'}">${label}</button>${extra?'<button data-recommendation-tab="runes">符文</button>':''}</div><div data-lane-matchup-slot></div><div data-live-notice="roster"></div></div><div id="recommendation-panel-build" class="recommendation-panel${loading?' is-loading':''}" ${selected!=='build'?'hidden':''}><img src="/build.png"></div><div id="recommendation-panel-insight" class="recommendation-panel" ${selected!=='insight'?'hidden':''}><div class="live-player-list">${keys.map(k=>`<article data-live-player-row="card:${k}"><img src="/${k}.png"></article><div data-live-player-row="history:${k}"></div>`).join('')}</div></div>${extra?'<div id="recommendation-panel-runes" class="recommendation-panel" hidden></div>':''}</section>`;
}
test('R224 notice, tabs, loading attributes and panel count update without remounting images/rows',()=>{
 const f=patchFixture();try{
  f.content.firstChild.innerHTML=markup();f.stampLiveRows(f.content);
  const image=f.content.querySelector('img'),row=f.content.querySelector('[data-live-player-row]'),tab=f.content.querySelector('[data-recommendation-tab]');tab.focus();
  for(const options of [{notice:'小队数据尚未完整，正在自动重试'},{selected:'insight',loading:true,label:'详情 18',extra:true},{}]) {
   assert.equal(f.updateLivePanels(markup(['a','b'],options)),true);
   assert.equal(f.content.querySelector('img'),image);assert.equal(f.content.querySelector('[data-live-player-row]'),row);assert.equal(f.content.querySelector('[data-recommendation-tab]'),tab);assert.equal(f.document.activeElement,tab);
  }
  assert.ok(f.reports.every(r=>!r.parts.includes('full')&&!r.parts.includes('build')&&!r.parts.includes('insight')));
 }finally{f.dom.window.close();}
});
test('R224 roster insert/delete/reorder keeps existing row and image identity, duplicate keys only first',()=>{
 const f=patchFixture();try{
  f.content.firstChild.innerHTML=markup();f.stampLiveRows(f.content);
  const rows=new Map([...f.content.querySelectorAll('[data-live-player-row]')].map(r=>[r.dataset.livePlayerRow,r]));
  const image=rows.get('card:a').firstChild;
  assert.equal(f.updateLivePanels(markup(['b','a','c'])),true);
  assert.equal(f.content.querySelector('[data-live-player-row="card:a"]'),rows.get('card:a'));assert.equal(rows.get('card:a').firstChild,image);
  assert.equal(f.updateLivePanels(markup(['c','a','a'])),true);
  assert.equal(f.content.querySelectorAll('[data-live-player-row="card:a"]').length,1);assert.equal(f.content.querySelector('[data-live-player-row="card:b"]'),null);
  assert.equal(f.content.querySelector('[data-live-player-row="card:a"]'),rows.get('card:a'));
  assert.ok(f.reports.every(r=>(r.stats?.imagesRecreated||0)===0&&(r.stats?.rowsReplaced||0)===0));
 }finally{f.dom.window.close();}
});
test('R224 all identity containers use one gap including compact widths and remove badge margins',()=>{
 const dom=new JSDOM(`<style>${css}</style>`);try {
  for(const cls of ['live-player-identity','summoner-name-meta','summoner-level-row','player-overlay-title','current-game-name-row','player-tab-copy']){
   const rules=[...dom.window.document.styleSheets[0].cssRules].filter(r=>r.selectorText===`.${cls}`);
   assert.ok(rules.length);assert.ok(rules.some(rule=>rule.style.getPropertyValue('gap')==='var(--identity-chip-gap)'));for(const rule of rules)if(rule.style.getPropertyValue('gap'))assert.equal(rule.style.getPropertyValue('gap'),'var(--identity-chip-gap)');
  }
  assert.match(css,/--identity-chip-gap:\s*6px/);
  for(const cls of ['self-chip','premade-team-tag','player-tab-region','my-squad-chip','pro-identity-chip','region-chip','player-tab-hidden','match-autofill-chip','summoner-streak']){
   for(const match of css.matchAll(new RegExp(`\\.${cls}[^{}]*\\{([^}]*)\\}`,'g'))){ const margin=match[1].match(/margin-left:\s*([^;]+)/)?.[1]?.trim(); assert.ok(!margin || /^(0|0px)$/.test(margin), `${cls}: ${margin}`); }
  }
 }finally{dom.window.close();}
});
test('R224 runtime forwards safe counts, true source and structural shell label only',async()=>{
 const dom=new JSDOM('',{url:'http://localhost'}),sent=[];try{
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),{window:dom.window,document:dom.window.document,fetch:async(_url,o)=>{sent.push(JSON.parse(o.body));return {ok:true,status:204}},setTimeout:()=>1,clearTimeout(){},setInterval:()=>1,clearInterval(){},Date,URL,URLSearchParams,AbortController,console});
  dom.window.reportFlowDiagnostic('live_roster_duplicate_dropped','deduplicated',{count:2,gameId:224,queueId:1750,playerRef:'secret'});
  dom.window.reportFlowDiagnostic('live_render_rebuild','aggregated',{counts:{banner:1,tabs:1},sources:{timer:1,selection:1},shellNode:'div.recommendation-area',playerName:'secret'});
  await new Promise(setImmediate);assert.equal(sent.length,2);assert.equal(sent[0].count,2);assert.equal(sent[1].shellNode,'div.recommendation-area');assert.deepEqual(sent[1].counts,{banner:1,tabs:1});assert.doesNotMatch(JSON.stringify(sent),/secret/);
 }finally{dom.window.close();}
});
