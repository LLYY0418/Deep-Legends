"use strict";
// 前端渲染冒烟测试：用 jsdom 加载真实的 index.html + 全部前端脚本，打开演示数据，
// 断言总览页真的渲染出内容。
//
// 存在的理由：`renderOverviewBody` 里任何一个运行期异常（例如引用了不存在的变量）
// 都会让容器永远停在骨架屏上，界面表现为“总览一直在加载”，而单元测试和 go test
// 全都照样通过。这个测试直接跑真实渲染路径，是唯一能挡住该类故障的护栏。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("jsdom");

const { WEB, SCRIPTS, gameplaySource, suiteSource, appStyles, gameplayStyles, suiteStyles, functionSource, compileFunctions, collapsedBeaconStyles, bootDemoApp, settled, installWindowCleanup, visitTool, bootLiveTab } = require("./overview-render-helpers.cjs");

const { waitForOverview } = require("./renderer-wait.cjs");

installWindowCleanup(test);

test("R86 hero search preserves input and coalesces five keystrokes (including bypass mutation)", async () => {
  async function check(mutate = false) {
    const { window: w, errors } = bootDemoApp({ championRankings: Array.from({ length: 170 }, (_, index) => ({ championId: index + 1, name: index === 102 ? "Ahri" : `Hero ${index + 1}`, key: `Hero${index + 1}`, winRate: 50, tier: 2 })), championsSourceTransform: (source) => {
      source = source.replace('function render() {', 'function render() { window.__r86Renders = (window.__r86Renders || 0) + 1;');
      source = source.replace('function objectRows(value) {', 'function objectRows(value) { window.__r86Rows = (window.__r86Rows || 0) + 1;');
      if (mutate) source = source.replace('root.addEventListener("input", (event) => {', 'root.addEventListener("input", () => { root.innerHTML = root.innerHTML; });\n  root.addEventListener("input", (event) => {');
      return source;
    } });
    try {
      await waitForOverview(w, 17);
      w.document.querySelector('[data-section="champions"]').click();
      await settled();
      const input = w.document.querySelector('[data-champion-search]');
      assert.ok(input);
      input.focus();
      const start = w.__r86Renders || 0;
      w.__r86Rows = 0;
      for (const value of ["a", "ah", "ahr", "ahri", "Ahri"]) {
        input.value = value;
        input.dispatchEvent(new w.Event("input", { bubbles: true }));
        assert.equal(w.document.querySelector('[data-champion-search]'), input);
      }
      await new Promise((resolve) => setTimeout(resolve, 250));
      assert.equal(w.document.querySelector('[data-champion-search]'), input);
      assert.equal(w.document.activeElement, input);
      assert.ok((w.__r86Renders || 0) - start <= 2);
      assert.ok(w.__r86Rows <= 5, `objectRows calls: ${w.__r86Rows}`);
      assert.equal(w.localStorage.getItem('lol-loot-champion-query-ranked'), "Ahri");
      assert.ok(w.document.querySelector('[data-champion-results] [data-champion-row]'));
      assert.deepEqual(errors, []);
    } finally { w.close(); }
  }
  await check();
  await assert.rejects(check(true), { name: "AssertionError" });
});

test("R86 tools fetch only active tab plus rig, then load claims on demand", async () => {
 async function check(mutate=false) {
  const { window: w, errors } = bootDemoApp({suiteSourceTransform:mutate ? source=>source.replace('function loadActiveTab(force = false) {','function loadActiveTab(force = false) { void api("/api/claim/scan").catch(()=>{});') : undefined});
  try {
    await waitForOverview(w, 17);
    const urls = [], original = w.fetch;
    w.fetch = (url, ...args) => { urls.push(String(url)); return original(url, ...args); };
    w.document.querySelector('[data-section="suite"]').click();
    await new Promise((resolve) => setTimeout(resolve, 250));
    for (const forbidden of ["/api/claim/scan", "/api/facade/state", "/api/champions/catalog", "/api/champselect/state"]) {
      assert.ok(!urls.some((url) => url.startsWith(forbidden)), `${forbidden} was loaded eagerly`);
    }
    w.document.querySelector('[data-suite-tab="sweep"]').click();
    await new Promise((resolve) => setTimeout(resolve, 250));
    assert.ok(urls.some((url) => url.startsWith("/api/claim/scan")));
    assert.deepEqual(errors, []);
  } finally { w.close(); }
 }
 await check();await assert.rejects(check(true),{name:"AssertionError"});
});

// Advance browser time without sleeping or shortening production intervals.
function r86Clock(w) {
  let now = Date.now(), id = 100000;
  const timers = new Map();
  const originalClear = w.clearTimeout.bind(w);
  w.Date.now = () => now;
  w.setTimeout = (fn, delay = 0) => { const key = ++id; timers.set(key, { at: now + Number(delay), fn }); return key; };
  w.clearTimeout = (key) => { timers.delete(key); originalClear(key); };
  return async (ms) => {
    const end = now + ms;
    for (let safety = 0; safety < 2000; safety++) {
      const next = [...timers].filter(([,t]) => t.at <= end).sort((a,b) => a[1].at-b[1].at)[0];
      if (!next) break;
      now = next[1].at; timers.delete(next[0]); next[1].fn();
      for (let i=0;i<30;i++) await Promise.resolve();
    }
    now = end;
  };
}

test("R86 healthy SSE reduces phase polling while closed SSE retains one-second fallback", async () => {
  for (const mutate of [false, true]) {
    const {window:w,eventSources}=bootDemoApp({liveEvents:true,gameplaySourceTransform: mutate ? s => s.replace('  let beaconPollTimer = 0;', '  window.addEventListener("deep-legends:live-frame", () => { void api("/api/gameplay/phase").catch(() => {}); });\n  let beaconPollTimer = 0;') : undefined});
    try {
      await waitForOverview(w, 17);
      const source=eventSources.at(-1);
      assert.ok(source);
      const advance=r86Clock(w), original=w.fetch;
      let calls=0;
      w.fetch=(url,...args)=>{if(String(url).startsWith('/api/gameplay/phase')){calls++;return Promise.resolve(new w.Response(JSON.stringify({phase:'None'})))}return original(url,...args)};
      w.dispatchEvent(new w.CustomEvent('deep-legends:live-disconnected'));
      for(let i=0;i<30;i++){source.onmessage({data:'keepalive'});await advance(1000)}
      if(mutate){assert.ok(calls>4,'independent eager path must exceed budget');continue}
      assert.ok(calls<=4,`healthy polls=${calls}`);
      calls=0;source.readyState=2;source.onerror();
      await advance(30000);
      assert.ok(calls>=20,`closed polls=${calls}`);
    } finally {w.dispatchEvent(new w.CustomEvent('deep-legends:dispose'));w.close()}
  }
});

test("R86 external match expansion preserves unrelated cards", async () => {
 async function check(mutate=false) {
  const {window:w}=bootDemoApp({gameplaySourceTransform:mutate ? source=>source.replace('render(id = "", options = {}) {','render(id = "", options = {}) { id = "";') : undefined});
  try {
    await waitForOverview(w, 17);
    const data=await (await w.fetch('/api/gameplay/overview')).json();
    const host=w.document.createElement('div');w.document.body.append(host);
    w.deepLegendsMatchCards.mount(host,{matches:data.matches,playerRef:data.player.playerRef});
    const before=[...host.querySelectorAll('.match-entry')];assert.ok(before.length>1);
    const summary=before[0].querySelector('.match-summary');
    before[0].querySelector('[data-toggle-match]').click();
    const after=[...host.querySelectorAll('.match-entry')];
    assert.equal(after[0],before[0]);assert.equal(after[0].querySelector('.match-summary'),summary);
    assert.ok(after[0].querySelector('.match-detail'));assert.equal(after[1],before[1]);
  }finally{w.dispatchEvent(new w.CustomEvent('deep-legends:dispose'));w.close()}
 }
 await check();await assert.rejects(check(true),{name:"AssertionError"});
});



test('R86 scroll restoration is resize-driven, restores clamped position and yields to user input', () => {
  const source=fs.readFileSync(path.join(WEB,'app.js'),'utf8');
  function check(mutate=false, userCancels=false) {
    const w=new JSDOM('<div></div>').window;
    try {
      let resize, max=0, top=0, frames=0;const queue=[];
      const appScroll={children:[{}],scrollTo({top:value}){top=Math.min(max,value)},get scrollTop(){return top}};
      class RO {constructor(fn){resize=fn}observe(){}disconnect(){}}
      w.ResizeObserver=RO;
      let body=functionSource(source,'restoreSectionScroll');
      if(mutate)body=body.replace('requestAnimationFrame(apply);','requestAnimationFrame(apply); requestAnimationFrame(function churn(){if(!cancelled)requestAnimationFrame(churn)});');
      const run=new Function('window','ResizeObserver','requestAnimationFrame','setTimeout','clearTimeout','state','el',`return (${body});`)(w,RO,fn=>{queue.push(fn);return queue.length},()=>1,()=>{}, {section:'champions',sectionScroll:{champions:600}}, {appScroll});
      run('champions');
      for(let i=0;i<20&&queue.length;i++){queue.shift()();frames++}
      assert.ok(frames<=3,`restore ran ${frames} frames while height unchanged`);
      assert.equal(top,0,'short skeleton clamps the target');
      if(userCancels)w.dispatchEvent(new w.Event('wheel'));
      max=1200;resize();
      assert.equal(top,userCancels?0:600);
    }finally{w.close()}
  }
  check();check(false,true);assert.throws(()=>check(true),{name:'AssertionError'});
});

test('R87 P9 external champselect events preserve modal scroll, input and composition', async () => {
  async function check(mutate = source => source, scrollOnly = false) {
    const {window:w,errors}=bootDemoApp({suiteSourceTransform:mutate});
    try {
      let shows=0;
      w.HTMLDialogElement.prototype.showModal=function(){shows++;this.setAttribute('open','');this.querySelector('button')?.focus();};
      w.HTMLDialogElement.prototype.close=function(){this.removeAttribute('open');};
      await settled();w.document.querySelector('[data-section="suite"]').click();await visitTool(w,'champselect');
      const root=w.document.querySelector('#suite-champselect-root');
      root.querySelector('[data-cs-open-dialog="pick"]').click();await new Promise(r=>setTimeout(r,40));
      const modal=root.querySelector('.cs-dialog'), input=modal.querySelector('[data-cs-dialog-search]');
      modal.scrollTop=200;input.focus();input.setSelectionRange(0,0);
      const external=async () => {
        w.dispatchEvent(new w.CustomEvent('deep-legends:gameflow',{detail:{phase:'ChampSelect',changed:true}}));
        await new Promise(r=>setTimeout(r,45));
      };
      await external();
      if(scrollOnly) {assert.equal(root.querySelector('.cs-dialog').scrollTop,200,'modal scroll reset');return;}
      input.dispatchEvent(new w.CompositionEvent('compositionstart',{bubbles:true}));
      for(let i=0;i<6;i++) {
        if(i<4) {input.value+='ahri'[i];input.dispatchEvent(new w.InputEvent('input',{bubbles:true,isComposing:true}));input.setSelectionRange(input.value.length,input.value.length);}
        await external();
      }
      input.dispatchEvent(new w.CompositionEvent('compositionend',{bubbles:true,data:'ahri'}));
      assert.equal(root.querySelector('[data-cs-dialog-search]').value,'ahri','typed text was replaced');
      assert.equal(w.document.activeElement,input,'search focus was lost');
      assert.equal(root.querySelector('.cs-dialog'),modal,'modal subtree was replaced');
      assert.equal(modal.scrollTop,200);assert.equal(input.selectionStart,4);assert.equal(shows,1,'open modal was shown again');
      modal.querySelector('[data-cs-dialog-close]').click();assert.equal(root.querySelector('.cs-dialog'),null);
      assert.deepEqual(errors,[]);
    } finally {w.close();}
  }
  await check();
  const reset = source => source.replace('if (existing) {\n      // Runtime events', 'if (existing) { existing.remove(); return syncChampSelectDialog();\n      // Runtime events');
  assert.notEqual(reset(suiteSource),suiteSource);
  await assert.rejects(check(reset,true),error=>error.name==='AssertionError' && /modal scroll reset/.test(error.message));
  await assert.rejects(check(reset),error=>error.name==='AssertionError' && /typed text|search focus/.test(error.message));
});

test('R104 quota recovery keeps the first-load skeleton and existing matches visible', () => {
  const dom = new JSDOM('<div id="overview"></div>', { url: 'http://localhost/' });
  const container = dom.window.document.getElementById('overview');
  const dependencies = {
    state: { tabs: [{ key: 'current' }], settings: { maskNames: false } },
    matchTierScope: () => 'scope', filteredMatches: (matches) => matches,
    maskedProfileIcon: () => '', iconFigure: () => '', playerLabel: () => 'Player', riotTab: () => false,
    emptyState: () => '', opggSummonerURL: () => '', loadOverview: () => {},
    matchListEmptyContent: () => 'empty', matchSentinelShouldHide: () => true,
    summonerContextChip: () => '', summonerProChip: () => '', summonerRegionChip: () => '', renderSummonerHighlights: () => '', renderOverviewStreak: () => '',
    scheduleOverviewCurrentGame: () => {}, updateFriendPresenceChips: () => {},
    paginationCopyFor: () => '', renderCareerSections: () => '', renderMatchFilters: () => '',
    renderMatch: (match) => `<article data-match-id="${match.gameId}" class="match-entry">${match.gameId}</article>`, number: (value) => String(value),
    escapeHTML: (value) => String(value ?? ''), bindOverviewContent: () => {}, applyRenderedMetricStyles: () => {},
    prepareImages: () => {}, ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, observeMatchTierVisibility: () => {},
    window: dom.window, document: dom.window.document,
  };
  const { renderOverviewBodyContent } = compileFunctions(gameplaySource, ['renderOverviewBodyContent'], dependencies);
  const retry = { retryAt: Date.now() + 5000, timer: 0 };
  const emptyTab = { key: 'current', matchFilter: 'all', quotaRetry: retry, data: null, error: '' };
  renderOverviewBodyContent(container, emptyTab);
  assert.ok(container.querySelector('[data-quota-recovery]'));
  assert.ok(container.querySelector('.gameplay-skeleton'));

  const dataTab = {
    key: 'current', matchFilter: 'all', matchViewRevision: 0, openMatches: new Set(), quotaRetry: retry, error: '',
    data: { player: { playerRef: 'ref' }, matches: [{ gameId: 1 }], pagination: { hasMore: false } },
  };
  renderOverviewBodyContent(container, dataTab);
  assert.ok(container.querySelector('[data-quota-recovery]'));
  assert.ok(container.querySelector('.match-list .match-entry'));
  dom.window.close();
});

test("R112 KR average-tier errors retry only visible active cards, then cache real values", async () => {
 const dom=new JSDOM('<main id="app-scroll"><div id="matches" data-match-tier-scope="scope"><article class="match-entry"><span data-match-tier data-match-tier-pending data-game-id="1"><span class="match-tier-value"></span></span></article></div></main>',{url:"http://localhost/"});
 const w=dom.window,d=w.document,container=d.getElementById("matches"),node=container.querySelector('[data-match-tier]');
 const rect=()=>({top:0,left:0,right:100,bottom:100,width:100,height:100});
 node.closest('.match-entry').getBoundingClientRect=rect;d.getElementById('app-scroll').getBoundingClientRect=rect;
 const state={activeTab:'current',matchTiers:new Map(),matchTierFlights:new Set(),matchTierFailures:new Map()};
 const tab={key:'current',data:{player:{playerRef:'public-player'},matches:[{gameId:1,createdAt:100,duration:1800}]}};
 let timer, delay, calls=0, fail=true;
 w.setTimeout=(fn,ms)=>{timer=fn;delay=ms;return 1;};
 const f=compileFunctions(gameplaySource,['matchTierScrollRoot','matchTierNodeIsVisible','noteMatchTierFailure','hydrateMatchTiers','shouldHydrateMatchTiers','scheduleMatchTierRetry','applyMatchTierValue'],{
  window:w,document:d,state,riotTab:()=>true,connected:()=>false,matchTierCacheKey:()=> 'scope:1',matchTierContent:value=>value?.tier||'—',matchTierTitle:()=>'',MATCH_TIER_RETRY_BASE_MS:1000,MATCH_TIER_MAX_BACKOFF_MS:60000,MATCH_TIER_KR_COALESCE_MS:0,MATCH_TIERS_PARALLEL_BATCHES:2,
  api:async()=>{calls++;if(fail)throw Error('503');return {'1':{tier:'CHALLENGER',lp:2100}};},
 });
 try {
  await f.hydrateMatchTiers(container,tab,'scope');
  assert.equal(calls,1);assert.equal(state.matchTiers.has('scope:1'),false);assert.equal(node.hasAttribute('data-match-tier-pending'),true);assert.ok(delay>=29000 && delay<=31000);
  await f.hydrateMatchTiers(container,tab,'scope');assert.equal(calls,1,'cooldown survives rerender');
  state.activeTab='other';timer();await Promise.resolve();assert.equal(calls,1,'inactive tab does not retry');
  state.activeTab='current';state.matchTierFailures.get('scope:1').nextRetryAt=0;
  fail=false;await f.hydrateMatchTiers(container,tab,'scope');
  assert.equal(calls,2);assert.equal(state.matchTiers.get('scope:1').tier,'CHALLENGER');assert.equal(node.querySelector('.match-tier-value').textContent,'CHALLENGER');assert.equal(node.hasAttribute('data-match-tier-pending'),false);
  await f.hydrateMatchTiers(container,tab,'scope',[node]);assert.equal(calls,2,'successful tier must not be queried twice');
  // A new failed row gets at most two automatic retries (three attempts total).
  state.matchTiers.clear();state.matchTierFailures.clear();node.setAttribute('data-match-tier-pending','');fail=true;tab.matchTierRetryTimer=null;timer=null;
  for(let attempt=1;attempt<=3;attempt++) {
   await f.hydrateMatchTiers(container,tab,'scope');
   assert.equal(state.matchTierFailures.get('scope:1').count,attempt);
   if(attempt<3){assert.ok(timer);state.matchTierFailures.get('scope:1').nextRetryAt=0;tab.matchTierRetryTimer=null;timer=null;}
  }
  assert.equal(timer,null,'no endless background retries');
  const settledCalls=calls;state.matchTierFailures.get('scope:1').nextRetryAt=0;
  node.setAttribute('data-match-tier-pending','');
  await f.hydrateMatchTiers(container,tab,'scope',[node]);
  assert.equal(calls,settledCalls,'rerender does not bypass the retry limit');
 } finally {w.close();}
});

// P1-7：韩服总览每个进度帧都全量重建并重新序列化整个生涯栏，只为发现「没变」。
// 验收：同一份 data（只改 matches）连投 5 帧，renderCareerSections 只跑 1 次，
// 且 .career-column 是同一个节点对象。
// 对抗变异：把签名固定成常量（永远命中），下面「ranks 真的变了」那段必须 FAIL。
test('R117 career column is rebuilt only when its own inputs change', () => {
  const dom = new JSDOM('<div id="overview"></div>', { url: 'http://localhost/' });
  const container = dom.window.document.getElementById('overview');
  let careerRenders = 0;
  const dependencies = {
    state: { tabs: [{ key: 'current' }], settings: { maskNames: false } },
    matchTierScope: () => 'scope', filteredMatches: (matches) => matches,
    maskedProfileIcon: () => '', iconFigure: () => '', playerLabel: () => 'Player', riotTab: () => true,
    emptyState: () => '', opggSummonerURL: () => '', loadOverview: () => {},
    matchListEmptyContent: () => 'empty', matchSentinelShouldHide: () => true,
    summonerContextChip: () => '', summonerProChip: () => '', summonerRegionChip: () => '', renderSummonerHighlights: () => '', renderOverviewStreak: () => '',
    scheduleOverviewCurrentGame: () => {}, updateFriendPresenceChips: () => {},
    paginationCopyFor: () => '', renderMatchFilters: () => '',
    renderCareerSections: () => { careerRenders += 1; return '<section class="career-probe">生涯</section>'; },
    renderMatch: (match) => `<article data-match-id="${match.gameId}" class="match-entry">${match.gameId}</article>`, number: (value) => String(value),
    escapeHTML: (value) => String(value ?? ''), bindOverviewContent: () => {}, applyRenderedMetricStyles: () => {},
    prepareImages: () => {}, ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, observeMatchTierVisibility: () => {},
    window: dom.window, document: dom.window.document,
  };
  // reconcileFilteredMatchList 一起编译进来：进度帧里战绩列表要走真实的增量协调，
  // 桩掉它就等于没验证「非战绩重渲染时列表节点被保留」这半边。
  const { renderOverviewBodyContent } = compileFunctions(gameplaySource, ['renderOverviewBodyContent', 'reconcileFilteredMatchList'], dependencies);
  // 进度帧的真实形状：data 里的 player / ranks / capabilities 都是同一批对象引用，
  // 每帧只有 matches 被换掉。签名靠的正是这种对象身份稳定性。
  const sharedData = {
    player: { playerRef: 'ref', region: 'kr', gameName: 'Fixture', summonerLevel: 300 },
    capabilities: [], ranks: [{ queue: 'RANKED_SOLO', tier: 'MASTER' }],
    matches: [{ gameId: 1 }], pagination: { hasMore: false },
  };
  const makeTab = (matches) => ({
    key: 'current', matchFilter: 'all', matchViewRevision: 0, openMatches: new Set(), quotaRetry: null, error: '',
    data: { ...sharedData, matches },
  });
  renderOverviewBodyContent(container, makeTab([{ gameId: 1 }]));
  const firstColumn = container.querySelector('.career-column');
  assert.ok(firstColumn, '生涯栏必须渲染出来');
  assert.equal(careerRenders, 1, '首帧必须渲染一次生涯栏');
  assert.ok(firstColumn.querySelector('.career-probe'), '生涯栏内容必须落到 .career-column 里');
  // 连投 5 帧，每帧只有 matches 变了——这正是韩服进度帧的典型形状。
  for (let frame = 2; frame <= 6; frame += 1) renderOverviewBodyContent(container, makeTab([{ gameId: frame }]));
  assert.equal(careerRenders, 1, `只改 matches 不得重建生涯栏，实际渲染 ${careerRenders} 次`);
  assert.equal(container.querySelector('.career-column'), firstColumn, '.career-column 必须仍是同一个节点对象');
  assert.equal(container.querySelector('.match-entry').dataset.matchId, '6', '战绩列表仍要跟着每帧更新');
  // 生涯栏自己的输入真的变了就必须重建：签名写死成常量会让这条 FAIL。
  const changed = makeTab([{ gameId: 9 }]);
  changed.data.ranks = [{ queue: 'RANKED_SOLO', tier: 'CHALLENGER' }];
  assert.notEqual(changed.data.player, null);
  renderOverviewBodyContent(container, changed);
  assert.equal(careerRenders, 2, 'ranks 变了必须重建生涯栏');
  assert.notEqual(container.querySelector('.career-column'), firstColumn, '重建后必须是新的节点对象');
  dom.window.close();
});
