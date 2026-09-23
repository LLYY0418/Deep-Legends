'use strict';
const test = require('node:test'), assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');
const read = name => fs.readFileSync(path.join(__dirname, name), 'utf8');
function extract(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  // These functions are top-level, with either two-space or tab indentation.
  const tail = source.slice(start), end = tail.indexOf('\n  }\n');
  assert.ok(end >= 0, name);
  return tail.slice(0, end + tail.slice(end).indexOf('}') + 1);
}
function compile(file, names, deps) { return Function(...Object.keys(deps), names.map(n => extract(read(file), n)).join('\n') + `\nreturn {${names.join(',')}}`)(...Object.values(deps)); }
const escapeHTML = s => String(s ?? '').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;');

test('R113 transient history errors remain retryable while unavailable and empty results are terminal',()=>{
  const {liveSnapshotComplete}=compile('gameplay.js',['liveSnapshotComplete'],{liveAugmentRecommendationSource:()=>''});
  const data={available:true,players:[{historyState:'failed'}]};
  assert.equal(liveSnapshotComplete(data),false);
  for(const state of ['empty','unavailable','ok']){data.players[0].historyState=state;assert.equal(liveSnapshotComplete(data),true);}
});

test('R113 selecting an exhausted dirty tab immediately replaces the old player content', () => {
  const dom = new JSDOM('<main>TheShy</main>');
  try {
    const old = { key:'shy', region:'kr' }, next = { key:'rookie', region:'kr', dirty:true, dirtyAttempts:3, data:{name:'Rookie'} };
    const state = { tabs:[old,next], activeTabs:{pro:'shy'}, tabHistories:{} }, calls=[];
    const {selectPlayerTab} = compile('gameplay.js',['selectPlayerTab'],{state, tabGroup:()=> 'pro', savePlayerScroll:()=>{}, renderPlayerTabs:()=>{},
      renderOverview:()=>{ dom.window.document.querySelector('main').textContent = state.tabs.find(t=>t.key===state.activeTabs.pro).data.name; calls.push('render'); },
      scheduleDirtyOverview:()=>calls.push('refresh'), loadOverview:()=>assert.fail('cached tab refetched')});
    selectPlayerTab('rookie');
    assert.equal(dom.window.document.querySelector('main').textContent,'Rookie');
    assert.deepEqual(calls,['render','refresh']); assert.equal(next.restoreScrollPending,true);
  } finally {dom.window.close();}
});

test('R113 local settlement leaves unrelated Korean tabs and their caches alone', () => {
  const own={current:true,key:'self'}, peer={playerRef:'peer'}, kr={region:'kr',playerRef:'rookie'}, unrelated={playerRef:'other'};
  const state={tabs:[own,peer,kr,unrelated],live:{players:[{playerRef:'peer'}]},section:'champions'};
  const {markOverviewAfterGame}=compile('gameplay.js',['markOverviewAfterGame'],{state,riotTab:t=>t.region==='kr',clearTimeout:()=>{},scheduleDirtyOverview:()=>{},activeTab:()=>own});
  markOverviewAfterGame('EndOfGame'); assert.equal(own.dirty,true); assert.equal(peer.dirty,true);
  assert.equal(kr.dirty,undefined); assert.equal(unrelated.dirty,undefined);
});

test('R113 a match phase preloads while another page or a hidden window is active', () => {
  const state={section:'overview',beacon:{phase:'Lobby'},liveGameGeneration:0}, timers=[], loads=[];
  const f=compile('gameplay.js',['handleGameplayPhase','queueLiveEventRefresh','liveGamePhase','normalizeLiveGameId'],{state,document:{hidden:true},connected:()=>true,
    recordLiveRefresh:()=>{},recordLiveObservation:()=>{},invalidateLiveForNewGame:()=>{},updateBeacon:p=>state.beacon.phase=p,markOverviewAfterGame:()=>{},renderLive:()=>{},scheduleBeaconPoll:()=>{},
    setTimeout:fn=>{timers.push(fn);return timers.length;},loadLive:(...args)=>loads.push(args)});
  f.handleGameplayPhase('ChampSelect','sse',true,113);
  assert.equal(timers.length,1); timers.shift()(); assert.equal(loads.length,1);
  state.liveEventTimer=0; state.liveRefreshQueued=false; state.livePhaseRefreshQueued=false; state.beacon.phase='InProgress';
  for(let i=0;i<100;i++)f.queueLiveEventRefresh('sse',false);
  assert.equal(timers.length,0,'same-phase in-game events must not become a request clock');
});

test('R113 background retries stop after the budget or a complete snapshot',()=>{
  const state={settings:{liveRefresh:true},section:'overview',beacon:{phase:'InProgress'},liveRetryAttempts:8}, timers=[];
  const {scheduleLiveRefresh}=compile('gameplay.js',['scheduleLiveRefresh'],{state,document:{hidden:true},connected:()=>true,clearTimeout:()=>{},syncLiveRetryBudget:()=>{},
    liveSnapshotComplete:d=>d?.complete===true,liveAugmentRecommendationSource:()=>'',liveRefreshDelayMs:()=>3000,setTimeout:fn=>{timers.push(fn);return 1},loadLive:()=>{}});
  scheduleLiveRefresh(); assert.equal(timers.length,0);
  state.liveRetryAttempts=0; state.live={phase:'InProgress',complete:true}; scheduleLiveRefresh(); assert.equal(timers.length,0);
  state.live.complete=false; scheduleLiveRefresh(); assert.equal(timers.length,1); timers[0](); assert.equal(state.liveRetryAttempts,1);
});

test('R113 non-leader matchmaking is an inline condition, never a repeated failure toast',()=>{
  const state={watchEvents:new Map(),watchFired:0}, toasts=[];
  const {handleWatchEvent,watchRuleCard}=compile('suite.js',['handleWatchEvent','watchRuleCard'],{state,toast:m=>toasts.push(m),renderWatch:()=>{},escapeHTML,
    suiteDisplayPhase:p=>p,watchRuleControl:()=>'',watchConflictNote:()=>'',checked:()=>''});
  for(let i=0;i<20;i++){handleWatchEvent('watch:armed:auto-matchmaking:5000');handleWatchEvent('watch:skipped:auto-matchmaking:not-leader');}
  assert.equal(toasts.length,0); assert.equal(state.watchEvents.get('auto-matchmaking').kind,'skipped');
  const html=watchRuleCard({action:'auto-matchmaking',title:'自动匹配'},{enabled:true},true,{});
  assert.match(html,/需要你是房主/);assert.doesNotMatch(html,/上次触发失败|高优先级/);
});

test('R113 ARAM live cards omit ranks and keep their three verified metrics',()=>{
  // R131 §2.1-5：R129 P1 抽出的历史状态判据按真实实现一起编译（不桩），
  // 否则这份「海斗卡片不显示段位」的钉子测不出同类 flicker 回归。
  const {renderLivePlayer}=compile('gameplay.js',['renderLivePlayer','liveHistoryStateOf','liveHistorySettled'],{state:{},maskedPlayerName:p=>p.name,liveDisplayedChampionId:()=>13,rankTitle:()=> '钻石 I',positionLabel:()=> '其他',
    renderLivePremadeTag:()=>'',iconFigure:()=>'',proBadgeAttributes:()=>'',renderProIdentityBadge:()=>'',escapeHTML,number:String,percent:v=>`${v}%`,kda:String});
  const html=renderLivePlayer({name:'我自己',isCurrent:true,rank:{tier:'DIAMOND'},modeStats:{games:10,wins:6,losses:4,winRate:60,kda:4}},0,false,13,[],'',true,true);
  const dom=new JSDOM(html);
  try {const doc=dom.window.document;
    assert.doesNotMatch(html,/钻石|未定级|其他/);assert.ok(doc.querySelector('.is-self .live-player-name'));
    assert.deepEqual([...doc.querySelectorAll('dt')].map(n=>n.textContent),['当前模式','胜率','KDA']);
    assert.equal(doc.querySelector('.live-hexdata-link'),null);
  }finally{dom.window.close();}
});

test('R113 silver source rows survive nine earlier gold rows in both recommendation views',()=>{
  const rows=Array.from({length:12},(_,i)=>({rarity:i<9?'gold':'silver',score:100-i,grade:'S',assets:[{id:i+1,name:`海克斯${i+1}`}]}));
  const live=compile('gameplay.js',['renderLiveAugmentRecommendations'],{liveChampionAugmentRows:()=>rows,normalizeAugmentRarity:r=>({key:r}),escapeHTML,
    augmentTooltipText:()=>'',renderLiveAugmentIcon:()=>'',percent:String,number:String});
  const html=live.renderLiveAugmentRecommendations({},'hextech');
  assert.match(html,/is-silver/);for(const id of [10,11,12])assert.match(html,new RegExp(`海克斯${id}`));
  // R116-B P0-6：renderRecommendedAugments 现在多了阶段筛选（stage=0 是英雄级
  // 汇总，与 R113 这条断言的语义无关），把三个新依赖按默认「汇总」注入即可。
  // R128 §2.3：慎选陈述（mayhemCautionNote）已从 UI 删除，不再需要注入。
  const champions=compile('champions.js',['renderRecommendedAugments'],{augmentMetaForAsset:()=>null,augmentRarityKey:r=>r,augmentGrade:()=> 'S',renderMayhemRecommendedAugment:e=>`<b>${e.item.assets[0].name}</b>`,state:{mayhemStage:0},normalizeMayhemStage:()=>0,mayhemAugmentStageRow:()=>null,renderMayhemStageChips:()=>''});
  const catalog=champions.renderRecommendedAugments(rows,{});for(const id of [10,11,12])assert.match(catalog,new RegExp(`海克斯${id}`));
});
