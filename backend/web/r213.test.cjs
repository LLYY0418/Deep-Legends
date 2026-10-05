'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {compile, escapeHTML} = require('./r188-harness.cjs');
const source = fs.readFileSync(process.env.R213_GAMEPLAY_SOURCE || path.join(__dirname, 'gameplay.js'), 'utf8').replace(/^[ \t]*\/\/.*$/gm, '');

function snapshot() {
  return {available:true, phase:'ChampSelect', gameId:213, queueId:440, players:Array.from({length:4}, (_, i) => ({
    playerRef:`fixture-${i}`, championId:i+1, championName:`Hero${i}`, historyState:'ok',
    rank:i<3 ? {tier:['GOLD','SILVER','SILVER'][i], division:'IV'} : null,
    premadeGroup:i<2 ? '1' : '', premadeSize:i<2 ? 2 : 0, premadeSource:'session',
    privateHistory:i===3, autofill:i===2, proPlayer:i===1 ? {name:'FixturePro'} : null,
    recentGames:Array.from({length:10}, (_, j) => ({gameId:j+1, championId:j+1, championName:`Hero${j}`})),
  }))};
}

function harness() {
  const dom = new JSDOM('<div id="content"></div>'), document = dom.window.document;
  const nodes = {liveContent:document.getElementById('content')};
  const state = {section:'live', liveLoading:true, liveProgressRequestId:'current', liveExpectedGameId:213,
    liveLoadSource:'interval', liveAwaitingGame:false, beacon:{phase:'ChampSelect'}, recommendationTab:'insight'};
  const diagnostics = [], jobs = new Map(); let id = 0, f;
  const dependencies = {state, nodes, document, window:{reportFlowDiagnostic:(event, reason, fields)=>diagnostics.push({event,reason,...fields})},
    connected:()=>true, liveSnapshotBehindPhase:d=>d.phase!==state.beacon.phase,
    setTimeout:(fn, delay)=>{jobs.set(++id,{fn,delay});return id;}, clearTimeout:key=>jobs.delete(key), escapeHTML,
    renderLiveRefreshStatus:()=>'', renderSessionSummary:()=>{}, bindLiveContent:()=>{}, applyRenderedMetricStyles:()=>{}, prepareImages:()=>{},
    captureLiveScroll:()=>[], restoreLiveScroll:()=>{}, bindLivePanelScope:()=>{},
    liveRecommendationTarget:()=>null, liveRecommendationsFor:()=>null, liveAugmentRecommendationSource:()=>'',
    recommendationCapabilities:()=>({hasRunes:false,hasAugments:false}), recommendationActiveTab:()=> 'insight', recommendationPanelBusy:()=>false,
    recommendationTabSpecs:()=>state.extraTab ? [['insight','详情'],['build','出装']] : [['insight','详情']],
    renderLaneMatchupCard:()=>'', renderRecommendationDataNotices:()=>'',
    renderLiveInsights:data=>`<div class="live-player-list">${data.players.map((p,i)=>`<article data-live-player-row="card:${p.playerRef}">${f.renderLivePremadeTag(p,data.players)}<span>${p.rank?.tier || '未定级'}${p.autofill?'补位':''}${p.proPlayer?.name || ''}${p.privateHistory?'隐藏战绩':''}</span>${p.recentGames?.map(m=>`<img data-queued-src="hero-${m.championId}" alt="${m.championName}">`).join('') || ''}</article>`).join('')}</div>`,
    maskedPlayerName:(_p,i)=>`Fixture${i}`, proxyAsset:x=>x, assetPath:(_kind,id)=>`hero-${id}`, liveDisplayedChampionId:p=>p.championId,
  };
  f = compile(source, ['applyLivePlayerProgress','liveGamePhase','normalizeLiveGameId','renderLive','liveRecommendationMarkup','renderRecommendationArea','recommendationEmptyPanel',
    'livePremadeRoster','renderLivePremadeTag','liveBodyChrome','updateLivePanels','patchLiveRosterPanel','preserveLiveImages','stampLiveRows',
    'liveRenderTriggerLabel','recordLiveRenderRebuild','flushLiveRenderRebuild','recordLiveProgressApply','flushLiveProgressApply'], dependencies);
  return {...f, state, nodes, diagnostics, jobs, document, close:()=>dom.window.close()};
}

test('R213 same-game skeleton and ready-row snapshots leave ranks, premades, private history and markup intact', () => {
  const h=harness(); try {
    const previous=h.state.live=snapshot(), baseline=JSON.stringify(previous);
    const markup=h.liveRecommendationMarkup(previous);
    assert.match(markup,/组队 ×2/); assert.match(markup,/GOLD/); assert.match(markup,/隐藏战绩/);
    for (const ready of [false,true]) {
      const next={...previous,players:previous.players.map((p,i)=>({playerRef:p.playerRef,championId:p.championId,historyState:ready && i===0 ? 'ok':'pending'}))};
      assert.equal(h.applyLivePlayerProgress({requestId:'current',live:next}),false);
      assert.equal(h.state.live,previous); assert.equal(JSON.stringify(h.state.live),baseline);
      assert.equal(h.liveRecommendationMarkup(h.state.live),markup);
    }
    assert.equal(h.state.liveRenderRebuild,undefined,'ignored snapshots do not render');
    h.flushLiveProgressApply();
    assert.equal(h.diagnostics[0].ignored_same_game,1,'count complete requests, not every player event');
    assert.equal(h.diagnostics[0].applied,0);
  } finally {h.close();}
});

test('R213 first load and awaiting/new-queue transitions continue applying per-player progress', () => {
  for (const initial of ['empty','awaiting','new-game','new-queue']) {
    const h=harness(); try {
      if(initial!=='empty')h.state.live=snapshot();
      if(initial==='awaiting')h.state.liveAwaitingGame=true;
      if(initial==='new-game')h.state.live.gameId=212;
      if(initial==='new-queue')h.state.live.queueId=420;
      const next=snapshot(); next.players=next.players.map(p=>({playerRef:p.playerRef,championId:p.championId,historyState:'pending'}));
      assert.equal(h.applyLivePlayerProgress({requestId:'current',live:next}),true,initial);
      assert.equal(h.state.live,next); assert(h.nodes.liveContent.querySelector('[data-live-body]'));
      const ready={...next,players:next.players.map((p,i)=>i===0 ? snapshot().players[0] : p)};
      assert.equal(h.applyLivePlayerProgress({requestId:'current',live:ready}),true,'first skeleton must not block the next ready player');
      assert.equal(h.state.live.players[0].historyState,'ok');assert.equal(h.state.live.players[1].historyState,'pending');
      h.flushLiveProgressApply();h.flushLiveRenderRebuild();
      assert.equal(h.diagnostics.find(d=>d.event==='live_progress_apply').applied,2);
      assert.equal(h.diagnostics.find(d=>d.event==='live_render_rebuild').sources.progress,2);
      // The next background request must not inherit the initial-load exception.
      h.state.liveProgressRequestId='next-request';
      assert.equal(h.applyLivePlayerProgress({requestId:'next-request',live:next}),false);
    } finally {h.close();}
  }
});

test('R213 rejects stale request, game and phase before the same-game shortcut', () => {
  const h=harness();try {
    const old=h.state.live=snapshot();
    for(const [requestId,patch] of [['old',{}],['current',{gameId:212}],['current',{phase:'InProgress'}]]) {
      assert.equal(h.applyLivePlayerProgress({requestId,live:{...old,...patch}}),false);assert.equal(h.state.live,old);
    }
    h.flushLiveProgressApply();assert.equal(h.diagnostics[0].ignored_stale,3);assert.equal(h.diagnostics[0].ignored_same_game,0);
    h.state.destroyed=true;assert.equal(h.applyLivePlayerProgress({requestId:'current',live:old}),false);assert.equal(h.jobs.size,0);
  }finally{h.close();}
});

test('R224 tab and panel membership updates preserve original loaded and queued image objects', () => {
  const h=harness();try {
    h.state.live=snapshot();h.renderLive();
    const images=[...h.nodes.liveContent.querySelectorAll('img')];assert.equal(images.length,40);
    images[0].setAttribute('src','hero-1');images[0].removeAttribute('data-queued-src');images[0].dataset.loaded='true';
    h.flushLiveRenderRebuild();h.diagnostics.length=0;h.state.extraTab=true;h.renderLive();
    assert.equal(h.nodes.liveContent.querySelectorAll('.recommendation-panel').length,2);
    const next=[...h.nodes.liveContent.querySelectorAll('img')];next.forEach((img,i)=>assert.equal(img,images[i]));
    assert.equal(next[0].dataset.loaded,'true');h.flushLiveRenderRebuild();
    assert.equal(h.diagnostics[0].imagesRecreated,0);assert.equal(h.diagnostics[0].counts.full || 0,0);
    assert.equal(h.diagnostics[0].counts.tabs,1);
  }finally{h.close();}
});

test('R224 banners update independently; broken lane shell and other structure remain diagnosable', () => {
  const h=harness();try {
    h.state.live=snapshot();h.renderLive();h.flushLiveRenderRebuild();h.diagnostics.length=0;
    h.state.live.arenaMySquadNotice='fixture notice';h.renderLive();h.flushLiveRenderRebuild();assert.equal(h.diagnostics.at(-1).counts.banner,1);assert.equal(h.diagnostics.at(-1).counts.full || 0,0);
    h.nodes.liveContent.querySelector('[data-lane-matchup-slot]').remove();h.nodes.liveContent._recommendationMarkup='changed';
    h.renderLive();h.flushLiveRenderRebuild();assert.equal(h.diagnostics.at(-1).fullReasons['lane-slot'],1);
    h.nodes.liveContent.querySelector('.recommendation-area').setAttribute('data-chrome','changed');h.nodes.liveContent._recommendationMarkup='changed';
    h.renderLive();h.flushLiveRenderRebuild();assert.equal(h.diagnostics.at(-1).fullReasons.other,1);assert.equal(h.diagnostics.at(-1).shellNode,'section.recommendation-area');
  }finally{h.close();}
});

test('R213 progress counters aggregate for 60 seconds and clear their timer on flush', () => {
  const h=harness();try {
    h.recordLiveProgressApply('applied');h.recordLiveProgressApply('ignored_stale');
    h.recordLiveProgressApply('ignored_same_game','one');h.recordLiveProgressApply('ignored_same_game','one');h.recordLiveProgressApply('ignored_same_game','two');
    assert.equal(h.jobs.size,1);assert.equal([...h.jobs.values()][0].delay,60000);assert.equal(h.diagnostics.length,0);
    [...h.jobs.values()][0].fn();assert.equal(h.jobs.size,0);assert.equal(h.state.liveProgressApply,null);
    assert.deepEqual(h.diagnostics[0],{event:'live_progress_apply',reason:'aggregated',applied:1,ignored_stale:1,ignored_same_game:2,windowMs:h.diagnostics[0].windowMs});
    h.flushLiveProgressApply();assert.equal(h.diagnostics.length,1);
    assert.match(source,/flushLiveProgressApply\(\);\s*state\.liveProgressSnapshot = null;/);
  }finally{h.close();}
});

test('R213 runtime admits bounded progress/rebuild counters and excludes identities', async () => {
  const dom=new JSDOM('<body></body>',{url:'http://localhost'}), bodies=[];
  try {
    vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),{window:dom.window,document:dom.window.document,
      fetch:async(_url,options)=>{bodies.push(JSON.parse(options.body));return {ok:true,status:204};},setTimeout:()=>1,clearTimeout:()=>{},setInterval:()=>1,clearInterval:()=>{},Date,URL,URLSearchParams,AbortController,console});
    dom.window.reportFlowDiagnostic('live_progress_apply','aggregated',{applied:2,ignored_same_game:3,ignored_stale:-1,windowMs:60000,playerRef:'SECRET',requestId:'SECRET'});
    dom.window.reportFlowDiagnostic('live_render_rebuild','aggregated',{counts:{full:1},sources:{progress:2,SECRET:99},fullReasons:{'tab-row':2,other:1,SECRET:99},imagesRecreated:0});
    await new Promise(setImmediate);assert.equal(bodies.length,2);
    assert.equal(bodies[0].ignored_same_game,3);assert.equal(bodies[0].ignored_stale,0);
    assert.deepEqual(bodies[1].sources,{progress:2});assert.deepEqual(bodies[1].fullReasons,{'tab-row':2,other:1});
    assert.doesNotMatch(JSON.stringify(bodies),/SECRET|playerRef|requestId/);
  }finally{dom.window.close();}
});

function simulateLoadedImages(root) {
  for (const img of root.querySelectorAll('img')) {
    img.setAttribute('src',img._liveAssetKey || img.getAttribute('data-queued-src') || img.getAttribute('src'));
    img.setAttribute('data-image-ready','true');
    img.removeAttribute('data-queued-src');
  }
}

for (const fullRebuild of [false,true]) {
  test(`R215 ${fullRebuild?'after full rebuild':'control without full rebuild'} rank change replaces only one loaded roster row`, () => {
    const h=harness();try {
      h.state.live=snapshot();h.renderLive();simulateLoadedImages(h.nodes.liveContent);
      if (fullRebuild) {h.state.extraTab=true;h.renderLive();simulateLoadedImages(h.nodes.liveContent);}
      h.flushLiveRenderRebuild();h.diagnostics.length=0;
      const rows=[...h.nodes.liveContent.querySelectorAll('[data-live-player-row^="card:"]')];
      const images=[...h.nodes.liveContent.querySelectorAll('img')];
      h.state.live.players[0].rank={tier:'PLATINUM',division:'IV'};h.renderLive();h.flushLiveRenderRebuild();
      const next=[...h.nodes.liveContent.querySelectorAll('[data-live-player-row^="card:"]')];
      const report=h.diagnostics[0];assert.equal(report.rowsReplaced,1);assert.equal(report.imagesRecreated,0);
      assert.notEqual(next[0],rows[0]);for(let i=1;i<rows.length;i++)assert.equal(next[i],rows[i]);
      [...h.nodes.liveContent.querySelectorAll('img')].forEach((img,i)=>assert.equal(img,images[i]));
      for(const row of next)assert.doesNotMatch(row._liveMarkup,/data-image-ready| src=/,'row signature describes fresh markup, not image queue attributes');
    }finally{h.close();}
  });
}

function panelHarness(markup) {
  const dom=new JSDOM(`<main><div data-live-body>${markup}</div></main>`);
  const root=dom.window.document.querySelector('main'), reports=[];
  const f=compile(source,['liveBodyChrome','updateLivePanels','patchLiveRosterPanel','preserveLiveImages','stampLiveRows'],{
    document:dom.window.document,nodes:{liveContent:root},captureLiveScroll:()=>[],restoreLiveScroll:()=>{},
    bindLivePanelScope:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{},recordLiveRenderRebuild:(_parts,stats)=>reports.push(stats),
  });
  f.stampLiveRows(root);
  return {...f,root,reports,close:()=>dom.window.close()};
}

test('R215 unchanged rune row survives a later panel change after its loaded image was moved', () => {
  const markup=(label,revision)=>`<section id="recommendation-panel-runes" class="recommendation-panel"><header>${revision}</header><div class="specialist-game-row" data-rune-choice="reused"><img data-queued-src="rune-source"><span>${label}</span></div></section>`;
  const h=panelHarness(markup('initial',0));try {
    simulateLoadedImages(h.root);const image=h.root.querySelector('img');
    assert.equal(h.updateLivePanels(markup('updated',1)),true);simulateLoadedImages(h.root);
    const row=h.root.querySelector('[data-rune-choice="reused"]');
    assert.equal(h.updateLivePanels(markup('updated',2)),true);
    assert.equal(h.root.querySelector('[data-rune-choice="reused"]'),row);
    assert.equal(h.root.querySelector('img'),image);assert.doesNotMatch(row._liveMarkup,/data-image-ready| src=/);
    assert(h.reports.every(report=>report.imagesRecreated===0));
  }finally{h.close();}
});

test('R215 lane slot stamps fresh row markup before moving loaded image nodes', () => {
  const markup=label=>`<div data-lane-matchup-slot><article data-live-player-row="lane:fixture"><img data-queued-src="lane-source"><span>${label}</span></article></div>`;
  const h=panelHarness(markup('initial'));try {
    simulateLoadedImages(h.root);const image=h.root.querySelector('img');
    assert.equal(h.updateLivePanels(markup('updated')),true);
    const row=h.root.querySelector('[data-live-player-row]');
    assert.equal(h.root.querySelector('img'),image);assert.equal(image.getAttribute('data-image-ready'),'true');
    assert.doesNotMatch(row._liveMarkup,/data-image-ready| src=/);
    assert.match(row._liveMarkup,/data-queued-src="lane-source"/);
    const before=row._liveMarkup;h.stampLiveRows(h.root);assert.equal(row._liveMarkup,before,'post-move stamp must not overwrite a clean signature');
  }finally{h.close();}
});
