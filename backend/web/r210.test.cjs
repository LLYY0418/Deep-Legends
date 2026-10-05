'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {compile, escapeHTML} = require('./r188-harness.cjs');
const read = name => fs.readFileSync(path.join(__dirname, name), 'utf8');
const source = fs.readFileSync(process.env.R210_GAMEPLAY_SOURCE || path.join(__dirname, 'gameplay.js'), 'utf8');
const flush = () => new Promise(resolve => setImmediate(resolve));

test('R210 Riot Key is the third row with help, an embedded reveal button and shared field styles', () => {
  const dom = new JSDOM(read('index.html'));
  try {
    const doc = dom.window.document, card = doc.querySelector('.setting-riot-key-card'), network = doc.querySelector('.setting-network-card');
    assert.equal(card.parentElement, network.parentElement);
    assert(card.parentElement.classList.contains('settings-grid'));
    assert.equal(card.previousElementSibling, network);
    assert.equal(card.parentElement.children.length, 3);
    const helpIcon = card.querySelector('h3 .setting-riot-key-help');
    assert.equal(helpIcon.getAttribute('aria-label'), 'Riot API Key 说明');
    assert.match(helpIcon.dataset.tooltip, /共用一份官方接口额度/);
    assert.doesNotMatch(card.textContent, /共用一份官方接口额度|24 小时|Register Product/);
    const help = card.querySelector('a');
    assert.equal(help.href, 'https://developer.riotgames.com/');
    assert.equal(help.target, '_blank'); assert.equal(help.rel, 'noopener noreferrer');
    assert.equal(card.querySelectorAll('p').length, 1);
    assert.equal(help.parentElement.textContent.trim(), '申请：用拳头（Riot）账号登录 Riot 开发者平台 ↗');
    const input = doc.getElementById('setting-riot-key-input'), eye = doc.getElementById('setting-riot-key-reveal');
    assert(input.parentElement.contains(eye));
    assert.equal(input.placeholder, '粘贴 RGAPI- 开头的 Key');
    assert(input.classList.contains('setting-network-field')); assert(doc.getElementById('setting-proxy-mode').classList.contains('setting-network-field'));
    const css = read('app.css');
    assert.match(css, /\.setting-network-controls \.setting-network-field, [^{]+\.app-select-trigger \{[^}]*min-height: var\(--setting-field-height\)[^}]*border: 1px solid var\(--setting-field-border\)[^}]*border-radius: 10px/);
    assert.doesNotMatch(css, /\.setting-riot-key-card \{ margin-top:/);
    assert.match(css, /\.setting-riot-key-input \{[^}]*width: 300px/);
    assert.match(css, /\.setting-riot-key-reveal \{[^}]*width: 32px; height: 32px[^}]*background: transparent/);
    assert.equal(eye.querySelectorAll('svg').length, 2);
    assert.equal(doc.getElementById('setting-riot-key-save').className, doc.getElementById('setting-proxy-save').className);
  } finally { dom.window.close(); }
});

test('R210 real settings events toggle visibility and save an empty masked field without echo', async () => {
  const {harness} = require('./update-harness.cjs');
  const h = harness(undefined, (url, options = {}) => url === '/api/riot-key' ? Promise.resolve({ok:true,status:200,json:async()=>({status:options.method==='DELETE'?'unconfigured':'configured',source:options.method==='DELETE'?'relay':'personal'})}) : undefined);
  try {
    const helpIcon = h.w.document.querySelector('.setting-riot-key-help'), tooltip = h.get('global-tooltip');
    assert.equal(tooltip.hidden, true);
    helpIcon.dispatchEvent(new h.w.Event('pointerover', {bubbles:true}));
    assert.equal(tooltip.hidden, false); assert.match(tooltip.textContent, /共用一份官方接口额度/);
    assert.equal(helpIcon.getAttribute('aria-describedby'), tooltip.id);
    helpIcon.dispatchEvent(new h.w.Event('pointerout', {bubbles:true}));
    assert.equal(tooltip.hidden, true);
    const input = h.get('setting-riot-key-input'), eye = h.get('setting-riot-key-reveal');
    input.value = 'RGAPI-DEMO-NOT-A-REAL-KEY'; eye.click();
    assert.equal(input.type, 'text'); assert.equal(eye.getAttribute('aria-pressed'), 'true'); assert.equal(eye.getAttribute('aria-label'), '隐藏 Riot Key');
    eye.click(); assert.equal(input.type, 'password'); assert.equal(eye.getAttribute('aria-pressed'), 'false');
    eye.click(); h.get('setting-riot-key-save').click(); await flush();
    assert.equal(input.value, ''); assert.equal(input.type, 'password'); assert.equal(eye.getAttribute('aria-pressed'), 'false');
    assert.equal(h.get('setting-riot-key-state').textContent, '已配置');
    h.get('setting-riot-key-clear').click(); await flush(); assert.equal(h.get('setting-riot-key-state').textContent, '使用内置服务');
  } finally { h.close(); }
});

function liveHarness() {
  const dom = new JSDOM('<div class="live-toolbar"><div data-live-status></div><button id="refresh"></button></div><div id="content"></div>', {pretendToBeVisual:true});
  const document = dom.window.document;
  const nodes = {liveContent:document.getElementById('content'),liveRefresh:document.getElementById('refresh')};
  const state = {section:'live',beacon:{phase:'Reconnect'},settings:{maskNames:false,liveRefresh:false},controllers:new Map(),liveRecommendations:new Map(),livePositionOverride:new Map(),recommendationTab:'insight'};
  const diagnostics = [], jobs = new Map(); let serial=0, response, pending, f;
  const noop = () => {};
  const dependencies = {state,nodes,document,window:dom.window,escapeHTML,connected:()=>true,
    api:async()=>pending || response,fetch:async(_url,options)=>{diagnostics.push(JSON.parse(options.body));return {ok:true}},
    setTimeout:(fn,delay)=>{jobs.set(++serial,{fn,delay});return serial},clearTimeout:id=>jobs.delete(id),
    recordLiveRefresh:noop,updateBeacon:phase=>state.beacon.phase=phase,markOverviewAfterGame:noop,scheduleBeaconPoll:noop,
    resetRecommendationTabsOnChampionChange:noop,renderCapabilitySettings:noop,clearRuneStarterRetries:noop,
    liveRecommendationsFor:()=>null,ensureLiveRecommendations:noop,ensureSpecialistRunes:noop,ensureProRunes:noop,ensureLaneMatchupCandidates:noop,
    liveRecommendationTarget:()=>null,liveAugmentRecommendationSource:()=>'',recommendationCapabilities:()=>({hasRunes:false,hasAugments:false}),
    recommendationTabSpecs:()=>[['insight','详情'],['build','出装']],recommendationActiveTab:()=> 'insight',recommendationPanelBusy:()=>false,
    renderLiveInsights:data=>`<div data-live-body>${(data.players||[]).map((p,i)=>f.renderLivePlayer(p,i)).join('')}</div>`,
    renderChampionRecommendationHeader:()=>'',renderRuneRecommendations:()=>'',renderBuildRecommendation:()=>'',renderRecommendationDataNotices:()=>'',renderLaneMatchupCard:()=>'',
    renderSessionSummary:noop,renderLiveRefreshStatus:()=>'',recordLiveRenderRebuild:noop,updateLivePanels:()=>false,
    bindLiveContent:noop,applyRenderedMetricStyles:noop,prepareImages:noop,stampLiveRows:noop,syncLiveRetryBudget:noop,scheduleLiveRefresh:noop,queueLiveEventRefresh:noop,
    iconFigure:()=>'',rankTitle:()=>'',renderLivePremadeTag:()=>'',proBadgeAttributes:()=>'',renderProIdentityBadge:()=>'',number:String,percent:n=>`${n}%`,kda:String,
  };
  const names=['loadLive','updateLiveLoadingVisibility','renderLive','preserveLiveImages','liveRecommendationMarkup','liveRenderTriggerLabel','handleGameplayPhase','liveGamePhase','normalizeLiveGameId','liveGameIdComparison','recordLiveObservation','liveSnapshotBehindPhase','invalidateLiveForNewGame','shouldResetLiveGameScopedState','resetLiveGameScopedState','resetLivePositionOverrides','renderRecommendationArea','recommendationEmptyPanel','emptyState','renderLivePlayer','liveDisplayedChampionId','renderInsightMatches','liveHistoryStateOf','liveHistorySettled','champSelectEnemyPlaceholder','positionLabel','maskedPlayerName','playerLabel'];
  f = compile(source,names,dependencies);
  const payload=require('../testdata/r197-post-game-reveal.json');
  const snapshot=(phase,gameId=9014822625)=>({...payload[['Reconnect','WaitingForStats'].includes(phase)?'before':'after'],phase,gameId});
  return {state,diagnostics,dom,...f,snapshot,async load(value){response=value;pending=null;await f.loadLive();},set pending(value){pending=value},text:()=>nodes.liveContent.textContent,players:()=>nodes.liveContent.querySelectorAll('.live-player').length,close:()=>dom.window.close()};
}

test('R210 log replay clears ten ended players immediately in Lobby and rejects old None roster', async () => {
  const h=liveHarness();try {
    for(const phase of ['Reconnect','WaitingForStats','EndOfGame']) { h.handleGameplayPhase(phase,'sse',true,9014822625); await h.load(h.snapshot(phase)); assert.equal(h.players(),10,phase); }
    assert.match(h.text(),/Fixture9/); h.state.liveRecommendations.set('old',{});
    h.handleGameplayPhase('Lobby','sse',true,0);
    assert.equal(h.players(),0,'Lobby phase signal must immediately clear the roster'); assert.match(h.text(),/等待进入对局/); assert.equal(h.state.liveAwaitingGame,false); assert.equal(h.state.liveRecommendations.size,0,'R207 scoped reset clears ended recommendations');
    await h.load(h.snapshot('Lobby')); assert.equal(h.players(),0);
    h.handleGameplayPhase('None','sse',true,0); await h.load(h.snapshot('None')); assert.equal(h.players(),0); assert.match(h.text(),/等待进入对局/); assert.equal(h.state.live.gameId,0);
    const reset=h.diagnostics.filter(d=>d.event==='live_scope_reset'&&d.reason==='left_end_of_game'); assert.equal(reset.length,1); assert.equal(reset[0].previousGameId,9014822625); assert.equal(reset[0].previousPhase,'EndOfGame'); assert.equal(reset[0].phase,'Lobby'); assert.equal(reset[0].clearedRecommendations,1);
    h.handleGameplayPhase('ChampSelect','sse',true,9014823000); await h.load(h.snapshot('ChampSelect',9014823000)); assert.equal(h.players(),10); assert.equal(h.state.live.gameId,9014823000); assert.doesNotMatch(h.text(),/等待进入对局/);
  }finally{h.close()}
});

test('R210 phase exit fences an in-flight EndOfGame response and subsequent stale end snapshot', async () => {
  const h=liveHarness();try {
    await h.load(h.snapshot('EndOfGame'));
    let release; h.pending=new Promise(resolve=>release=resolve); const old=h.loadLive();
    h.handleGameplayPhase('Lobby','sse',true,0); assert.equal(h.players(),0);
    release(h.snapshot('EndOfGame')); await old; assert.equal(h.players(),0); assert.equal(h.state.liveLoading,false);
    await h.load(h.snapshot('EndOfGame')); assert.equal(h.players(),0,'late old end-game phase may not repopulate lobby');
    await h.load(h.snapshot('None')); assert.equal(h.players(),0);
  }finally{h.close()}
});
