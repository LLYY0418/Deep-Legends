'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {compile, escapeHTML} = require('./r188-harness.cjs');
const source = fs.readFileSync(process.env.R207_GAMEPLAY_SOURCE || path.join(__dirname, 'gameplay.js'), 'utf8').replace(/^[ \t]*\/\/.*$/gm, '');
const payload = () => ({hero:{name:'海兽祭司'}, augments:[{id:1, rarity:'gold', assets:[{name:'测试海克斯'}], games:100, winRate:52}], build:{coreOptions:[{ids:[3071], games:100}], boots:[3006]}});
function snapshot(phase='ChampSelect', spells=[4,32], gameId=9014523366) {
  return {available:true, phase, gameId, queueId:2400, gameMode:'KIWI', mapId:12, players:[{isCurrent:true, teamId:100, championId:420, position:'other', spell1Id:spells[0], spell2Id:spells[1]}]};
}
function harness(apiImpl = async () => ({recommendations:payload()})) {
  const dom = new JSDOM('<main></main>');
  const state = {live:null, liveRecommendations:new Map(), liveRecommendationScopes:new Map(), liveRecommendationSpells:new Map(), liveRecommendationTraces:new Map(), liveRecommendationFlights:new Map(), liveRecommendationFailures:new Map(), liveRecommendationRosters:new Map(), liveRecommendationSkipDiagnostics:new Set(), livePositionOverride:new Map(), liveGameGeneration:0, recommendationTab:'build'};
  const calls=[], diagnostics=[];
  let f;
  const deps={state, URLSearchParams, USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS:false, R116D_MATCHUP_PHASES:['gamestart','inprogress'], R116D_ROSTER_SIDE_LIMIT:5,
    liveRecommendationTier:()=> 'emerald_plus', isHextechClassic:()=>false, isOrdinaryHextechMatch:()=>false, isHextechQualifier:()=>false, queueDefinitionFor:()=>null,
    api:async url=>{calls.push(url);return apiImpl(url)}, fetch:async(_url,opts)=>{diagnostics.push(JSON.parse(opts.body));return {ok:true}},
    renderLive:()=>{if(state.live)dom.window.document.querySelector('main').innerHTML=f.renderRecommendationArea(state.live)},
    ensurePerks(){}, ensureItems(){}, ensureSummonerSpells(){}, ensureSpecialistRunes(){}, clearRuneStarterRetries(){},
    escapeHTML, number:String, percent:n=>`${n}%`, gradeBadge:()=>'', gradeRank:()=>0, augmentTooltipText:()=>'', renderLiveAugmentIcon:()=>'',
    recommendationCapabilities:()=>({hasRunes:false,hasAugments:true}), recommendationTabSpecs:()=>[['build','海克斯与出装'],['insight','详情']], recommendationActiveTab:()=> 'build', recommendationPanelBusy:()=>false,
    renderLiveInsights:()=>'', renderChampionRecommendationHeader:hero=>hero?.name||'', renderRecommendationDataNotices:()=>'', renderLaneMatchupCard:()=>'',
    recommendationEmptyPanel:(title,detail)=>`<p>${title}</p><small>${detail}</small>`, LIVE_CORE_OPTION_LIMIT:5,
    renderArenaBuildOption:option=>`<b data-item="${option.ids.join('-')}">装备</b>`, renderConfigOption:option=>`<b data-item="${option.ids.join('-')}">装备 ${option.ids.join('-')}</b>`, buildItemSetPayload:()=>({blocks:[]}), renderCoreStats:()=>'', renderDepthStats:()=>'', renderRecommendationStats:()=>'', renderSkillPlan:()=>'',
  };
  const names=['liveRecommendationChampionId','liveRecommendationTarget','liveAugmentRecommendationSource','liveRecommendationsFor','liveRecommendationRoster','ensureLiveRecommendations','hasUsableLiveRecommendations','newLiveRecommendationTraceId','recordItemSetClientDiagnostic','recordLiveRecommendationSkip','liveRecommendationFlightActive','renderRecommendationArea','ensureLiveRecommendationForRender','recordLiveRecommendationRender','liveChampionAugmentRows','normalizeAugmentRarity','renderLiveAugmentRecommendations','renderBuildRecommendation','shouldResetLiveGameScopedState','resetLiveGameScopedState'];
  f=compile(source,names,deps);
  return {state,calls,diagnostics,dom,...f,render(data=state.live){state.live=data;dom.window.document.querySelector('main').innerHTML=f.renderRecommendationArea(data);return dom.window.document.querySelector('main')}};
}
const tick=()=>new Promise(resolve=>setImmediate(resolve));
function assertReady(h) {
  const panel=h.dom.window.document.querySelector('#recommendation-panel-build');
  assert.match(panel.textContent,/测试海克斯/);
  assert(panel.querySelector('[data-item="3071"]'));
  assert.doesNotMatch(panel.textContent,/暂无该英雄的海克斯样本|等待推荐出装与技能数据/);
}
test('R207 log replay ChampSelect → GameStart → gameflow → named live-client spells keeps real build and augments',async()=>{
 const h=harness();try{
  h.state.live=snapshot();await h.ensureLiveRecommendations(h.state.live);h.render();assertReady(h);
  for(const next of [snapshot('GameStart',[]),snapshot('InProgress',[]),snapshot('InProgress',[])]){
   if(next.phase==='InProgress')next.players[0].spell1Name='闪现';
   assert.equal(h.shouldResetLiveGameScopedState(h.state.live,next),false);h.render(next);await h.ensureLiveRecommendations(next);h.render();assertReady(h);
   assert.match(h.liveRecommendationTarget(next).key,/:4-32$/);
  }
  assert(h.diagnostics.some(d=>d.event==='live_recommendation_render'&&d.phase==='InProgress'&&d.exactHit));
  assert(h.calls.filter(url=>new URL(url,'http://fixture').searchParams.get('phase')==='InProgress').every(url=>!new URL(url,'http://fixture').searchParams.has('spell1Id')));
 }finally{h.dom.window.close()}
});
test('R207 drift fallback renders same-game successful cache and starts only one exact none request',async()=>{
 let release;const h=harness(()=>new Promise(resolve=>release=resolve));try{
  const data=snapshot('InProgress',[]);h.state.live=data;
  const target=h.liveRecommendationTarget(data), key=target.key.replace(/:none$/,':4-32');
  h.state.liveRecommendations.set(key,payload());h.state.liveRecommendationScopes.set(key,{gameId:data.gameId,championId:420,gameMode:'KIWI',mapId:12,tier:'emerald_plus',at:1});
  assert.equal(h.liveRecommendationsFor(data),h.state.liveRecommendations.get(key));
  for(let i=0;i<10;i++)h.render();assertReady(h);assert.equal(h.calls.length,1);assert.match(h.calls[0],/recommendationKey=.*none/);
  assert(h.diagnostics.some(d=>d.fallbackHit));release({recommendations:payload()});await tick();assertReady(h);
  for(const change of [{gameId:1},{championId:157},{gameMode:'ARAM'},{mapId:30},{tier:'diamond'}]){
   h.state.liveRecommendations.delete(target.key);h.state.liveRecommendationScopes.set(key,{gameId:data.gameId,championId:420,gameMode:'KIWI',mapId:12,tier:'emerald_plus',at:1,...change});assert.equal(h.liveRecommendationsFor(data),null,JSON.stringify(change));
  }
 }finally{h.dom.window.close()}
});
test('R207 cache cleared during unchanged polling recovers from render with single flight and failure backoff',async()=>{
 let release;const h=harness(()=>new Promise(resolve=>release=resolve));try{
  const data=snapshot('InProgress',[]);h.state.live=data;
  h.state.liveRecommendations.clear();for(let i=0;i<20;i++)h.render();assert.equal(h.calls.length,1);
  release({recommendations:payload()});await tick();assertReady(h);
  h.state.liveRecommendations.clear();h.state.liveRecommendationFailures.set(h.liveRecommendationTarget(data).key,Date.now());for(let i=0;i<10;i++)h.render();assert.equal(h.calls.length,1);
 }finally{h.dom.window.close()}
});
test('R207 gameId 0 wobble preserves cache; restored known different game resets',async()=>{
 const h=harness();try{
  h.state.live=snapshot('InProgress');await h.ensureLiveRecommendations(h.state.live);
  const zero=snapshot('InProgress',[],0);if(h.shouldResetLiveGameScopedState(h.state.live,zero))h.resetLiveGameScopedState(false,{reason:'game_changed',previous:h.state.live,next:zero});h.render(zero);await tick();
  assert.equal(h.state.liveRecommendations.size,1);assert.equal(h.diagnostics.filter(d=>d.event==='live_scope_reset'&&d.reason==='game_changed').length,0);assertReady(h);
  assert.equal(h.shouldResetLiveGameScopedState(zero,snapshot('InProgress',[],9014523366)),false);
  assert.equal(h.shouldResetLiveGameScopedState(zero,snapshot('InProgress',[],9014523367)),true);
 }finally{h.dom.window.close()}
});
test('R207 Classic spell changes retain original exact request and query; special modes freeze only after select',async()=>{
 for(const mode of ['CLASSIC','ARAM','CHERRY','KIWI']){
  const h=harness();try{
   const data=snapshot();Object.assign(data,{gameMode:mode,mapId:mode==='CLASSIC'?11:mode==='CHERRY'?30:12,queueId:mode==='CLASSIC'?420:mode==='CHERRY'?1700:2400});
   h.state.live=data;await h.ensureLiveRecommendations(data);const old=h.liveRecommendationTarget(data).key;
   data.players[0].spell2Id=11;const select=h.liveRecommendationTarget(data).key;assert.notEqual(select,old);
   data.phase='InProgress';data.players[0].spell2Id=12;const next=h.liveRecommendationTarget(data);await h.ensureLiveRecommendations(data);
   if(mode==='CLASSIC'){assert.match(next.key,/:4-12$/);const q=new URL(h.calls.at(-1),'http://fixture').searchParams;assert.equal(q.get('spell2Id'),'12')}
   else assert.equal(next.key,select);
  }finally{h.dom.window.close()}
 }
});
test('R207 empty diagnostics have complete fields and same-game same-key maximum five',()=>{
 const h=harness(()=>new Promise(()=>{}));try{
  for(let i=0;i<15;i++)h.render(snapshot('InProgress',[]));const rows=h.diagnostics.filter(d=>d.event==='live_recommendation_render');assert.equal(rows.length,5);
  for(const field of ['phase','gameId','championId','targetKey','exactHit','fallbackHit','cachedKeys','hasPayloadField','augmentRows','hasBuild','lastResetReason','msSinceReset'])assert(field in rows[0],field);
  h.resetLiveGameScopedState(true,{reason:'hard_refresh',previous:h.state.live,next:h.state.live});h.render();assert.equal(h.diagnostics.filter(d=>d.event==='live_recommendation_render').length,5);
  const reset=h.diagnostics.find(d=>d.event==='live_scope_reset');assert.equal(reset.reason,'hard_refresh');assert.equal(reset.previousGameId,9014523366);
 }finally{h.dom.window.close()}
});

test('R207 same champion in a new game cannot inherit prior fallback or spell pair',async()=>{
 const h=harness();try{
  h.state.live=snapshot();await h.ensureLiveRecommendations(h.state.live);h.render();
  const next=snapshot('ChampSelect',[],9014523367);assert(h.shouldResetLiveGameScopedState(h.state.live,next));h.resetLiveGameScopedState(false,{reason:'game_changed',previous:h.state.live,next});h.state.live=next;
  assert.equal(h.liveRecommendationsFor(next),null);assert.match(h.liveRecommendationTarget(snapshot('InProgress',[],9014523367)).key,/:none$/);
 }finally{h.dom.window.close()}
});
test('R207 initially missing gameId adopts its known scope and partial select spell IDs do not replace valid pair',async()=>{
 const h=harness();try{
  h.state.live=snapshot('ChampSelect',[4,32],0);await h.ensureLiveRecommendations(h.state.live);
  h.liveRecommendationTarget(snapshot('ChampSelect',[4],0));
  const next=snapshot('InProgress',[],9014523366);next.players[0].position='mid';
  assert.equal(h.shouldResetLiveGameScopedState(h.state.live,next),false);h.state.live=next;assert.match(h.liveRecommendationTarget(next).key,/:4-32$/);assert(h.liveRecommendationsFor(next));
 }finally{h.dom.window.close()}
});
