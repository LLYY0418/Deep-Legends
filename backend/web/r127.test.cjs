'use strict';
// R127：战绩列表图标长时间是文字占位。
//
// P1-a 验收场景（工单原文）：模拟 5 张永不返回的远程图 + 20 张本地图，本地图
// 必须在远程图超时前全部完成。对抗变异：把分道去掉，该测试必须 FAIL——下面用
// 去掉闸门后的同一份源码重跑同一场景，断言本地图被饿死。
//
// P0-2 验收：图片失败上报必须带 queueWaitMs / loadMs / activeSlowCount /
// 图片来源，并且能穿过 runtime.js 的传输白名单真正落到 POST body 里。
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { JSDOM } = require('../../desktop/node_modules/jsdom');

const read = name => fs.readFileSync(path.join(__dirname, name), 'utf8');
const flush = () => new Promise(resolve => setImmediate(resolve));

function queueWindow(source) {
  const dom = new JSDOM('<body></body>', { url: 'http://fixture/', runScripts: 'outside-only' });
  const w = dom.window;
  const reports = [];
  w.reportFlowDiagnostic = (event, reason, fields) => reports.push({ event, reason, fields });
  w.eval(source);
  const add = url => {
    const img = w.document.createElement('img');
    img.setAttribute('data-queued-src', url);
    w.document.body.append(img);
    return img;
  };
  const dispose = () => { w.dispatchEvent(new w.Event('deep-legends:dispose')); w.close(); };
  return { w, reports, add, dispose };
}

// 强化符文图标：本机客户端没有（LCU 400），后端会回退到 raw.communitydragon.org。
// 前端看到的是不带 source= 的 /api/image，只能靠 augments/icons 路径与 art=2 识别。
const augmentURL = id => `/api/image?path=${encodeURIComponent(`/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/${id}_large.png`)}&art=2`;
const localURL = id => `/api/image?path=${encodeURIComponent(`/lol-game-data/assets/v1/champion-icons/${id}.png`)}`;
const remoteURL = id => `/api/champion-asset?source=communitydragon&path=${encodeURIComponent(`/latest/game/assets/ux/kiwi/augments/icons/${id}.png`)}`;

async function starvationScenario(source) {
  const h = queueWindow(source);
  try {
    const remote = Array.from({ length: 5 }, (_, index) => h.add(augmentURL(index)));
    const local = Array.from({ length: 20 }, (_, index) => h.add(localURL(index)));
    let peakRemote = 0, peakTotal = 0;
    for (let round = 0; round < 60; round++) {
      await flush();
      const inFlight = [...remote, ...local].filter(img => img.hasAttribute('src') && img.dataset.imageReady !== 'true');
      const remoteInFlight = inFlight.filter(img => remote.includes(img));
      peakRemote = Math.max(peakRemote, remoteInFlight.length);
      peakTotal = Math.max(peakTotal, inFlight.length);
      assert.ok(inFlight.length <= h.w.deepLegendsImageQueueLimit, `在途图片 ${inFlight.length} 超过总名额`);
      // 远程图永不返回：只放行本地图的 load。
      for (const img of inFlight) if (local.includes(img)) img.dispatchEvent(new h.w.Event('load'));
      if (local.every(img => img.dataset.imageReady === 'true')) break;
    }
    return {
      total: h.w.deepLegendsImageQueueLimit,
      remoteLimit: h.w.deepLegendsImageQueueRemoteLimit,
      peakRemote, peakTotal,
      localReady: local.filter(img => img.dataset.imageReady === 'true').length,
      allLocalReady: local.every(img => img.dataset.imageReady === 'true'),
      remoteReady: remote.filter(img => img.dataset.imageReady === 'true').length,
    };
  } finally { h.dispose(); }
}

test('R127 永不返回的远程符文图标不会饿死本机英雄/技能/装备图标', async () => {
  const result = await starvationScenario(read('image-queue.js'));
  assert.equal(result.total, 5, '总名额必须仍是 5：加上 SSE 才不超过浏览器同源 6 条连接');
  assert.equal(result.remoteLimit, 2, '远程道必须单独限额');
  assert.ok(result.peakRemote <= result.remoteLimit, `远程图峰值 ${result.peakRemote} 超过远程道名额 ${result.remoteLimit}`);
  assert.ok(result.peakTotal <= result.total, `在途峰值 ${result.peakTotal} 超过总名额 ${result.total}`);
  assert.equal(result.localReady, 20, '20 张本地图必须在远程图超时前全部完成');
  assert.equal(result.remoteReady, 0, '远程图模拟 communitydragon 不可达，不该有任何一张完成');
});

test('R127 对抗变异：去掉分道后同一场景必须饿死本地图', async () => {
  const source = read('image-queue.js');
  const gate = 'if (lane === "remote" && activeLaneCount("remote") >= REMOTE_LANE_LIMIT) continue;';
  assert.ok(source.includes(gate), '生产源码里必须存在远程道闸门');
  const result = await starvationScenario(source.replace(gate, ''));
  assert.equal(result.localReady, 0, '去掉分道后本地图仍然全部完成，说明这个场景测不到分道');
  assert.equal(result.peakRemote, result.total, '去掉分道后远程图应当占满全部名额');
});

test('R127 图片失败上报排队时长、加载时长、慢图数量与来源', async () => {
  const h = queueWindow(read('image-queue.js'));
  try {
    const base = 1700000000000;
    let now = base;
    h.w.Date.now = () => now;
    const first = h.add(remoteURL('first'));
    const second = h.add(remoteURL('second'));
    await flush();
    assert.equal(first.getAttribute('src'), remoteURL('first'));
    assert.equal(second.getAttribute('src'), remoteURL('second'));
    const third = h.add(remoteURL('third'));
    await flush();
    assert.equal(third.hasAttribute('src'), false, '远程道只有 2 个名额，第三张必须继续排队');
    now = base + 1200; // 排队 1.2 秒后才有名额空出来
    second.dispatchEvent(new h.w.Event('load'));
    assert.equal(third.getAttribute('src'), remoteURL('third'), '名额一空出来就必须立刻放行排队中的远程图');
    now = base + 5200; // first 已占名额 5.2 秒 → 慢图；third 已加载 4 秒
    third.dispatchEvent(new h.w.Event('error'));
    const report = h.reports.find(item => item.fields?.endpoint === 'image');
    assert.ok(report, '图片失败必须上报 local_request_client');
    assert.equal(report.event, 'local_request_client');
    assert.equal(report.reason, 'failed', 'reason 必须是 failed，后端白名单已放行');
    assert.equal(report.fields.queueWaitMs, 1200);
    assert.equal(report.fields.loadMs, 4000);
    assert.equal(report.fields.activeSlowCount, 1, '只统计仍占着名额的其它慢图');
    assert.equal(report.fields.imageSource, 'communitydragon');
    assert.equal(report.fields.errorKind, 'network');
  } finally { h.dispose(); }
});

test('R127 本机图超时上报 timeout，来源记为 lcu', async () => {
  const h = queueWindow(read('image-queue.js'));
  try {
    const timers = new Map();
    let timerID = 0;
    h.w.setTimeout = (fn, ms) => { timers.set(++timerID, { fn, ms }); return timerID; };
    h.w.clearTimeout = id => timers.delete(id);
    const img = h.add(localURL(7));
    await flush();
    assert.equal(img.getAttribute('src'), localURL(7));
    const pending = [...timers.values()];
    assert.equal(pending.length, 1, 'admission 只应挂一个前端超时定时器');
    assert.equal(pending[0].ms, 10000);
    pending[0].fn();
    const report = h.reports.find(item => item.fields?.endpoint === 'image');
    assert.ok(report, '前端超时也必须留下诊断');
    assert.equal(report.fields.errorKind, 'timeout', '前端 10 秒超时必须报 timeout 而不是 network');
    assert.equal(report.fields.imageSource, 'lcu', '/api/image?path=… 是本机客户端来源');
    assert.equal(report.fields.activeSlowCount, 0);
  } finally { h.dispose(); }
});

function transportHarness(source) {
  const posts = [];
  let now = 100000;
  const context = {
    Date: { now: () => now },
    setTimeout: () => 1,
    clearTimeout: () => {},
    window: {},
    AbortController,
    document: { hidden: false },
    state: { section: 'overview' },
    connected: () => true,
    fetch: async (url, options) => { posts.push(JSON.parse(options.body)); return { status: 204 }; },
  };
  vm.runInNewContext(source, context);
  return { posts, context, advance: ms => { now += ms; } };
}

test('R127 图片诊断字段能穿过 runtime.js 传输白名单', async () => {
  const fields = {
    endpoint: 'image', httpStatus: 0, errorKind: 'timeout',
    queueWaitMs: 4210, loadMs: 8001, activeSlowCount: 2, imageSource: 'communitydragon',
    url: '/api/image?path=/secret/profile-icons/4379.jpg',
  };
  const harness = transportHarness(read('runtime.js'));
  harness.context.window.reportFlowDiagnostic('local_request_client', 'failed', fields);
  await flush();
  assert.equal(harness.posts.length, 1, '失败诊断必须真的发出去');
  const body = harness.posts[0];
  assert.equal(body.event, 'local_request_client');
  assert.equal(body.reason, 'failed');
  assert.equal(body.endpoint, 'image');
  assert.equal(body.queueWaitMs, 4210);
  assert.equal(body.loadMs, 8001);
  assert.equal(body.activeSlowCount, 2);
  assert.equal(body.imageSource, 'communitydragon');
  assert.equal(body.errorKind, 'timeout');
  assert.doesNotMatch(JSON.stringify(harness.posts), /secret|profile-icons|4379|url/, '诊断不得携带资源路径');

  // 对抗变异：把新字段的放行删掉，body 里就必须看不到它们。
  const mutated = transportHarness(read('runtime.js').replace(
    'for (const key of ["queueWaitMs", "loadMs", "activeSlowCount"])',
    'for (const key of [])'));
  mutated.context.window.reportFlowDiagnostic('local_request_client', 'failed', fields);
  await flush();
  assert.equal(mutated.posts.length, 1);
  assert.equal(mutated.posts[0].queueWaitMs, undefined, '变异体证明这些字段确实由白名单放行');
  assert.equal(mutated.posts[0].loadMs, undefined);
  assert.equal(mutated.posts[0].activeSlowCount, undefined);
});

/* ---------- R127 P1-b.3：平均段位批次并行 + 每批立即回填 ---------- */

const extractFunction = (source, name) => {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, `gameplay.js 里必须有 ${name}`);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  return source.slice(start, source.indexOf('\n  }', start) + 4);
};

// 用真实生产源码跑国服平均段位回填：两批各 24 人，第一批先返回。
function batchScenario(source) {
  const maxRefs = Number(/const MATCH_TIERS_MAX_REFS = (\d+);/.exec(source)[1]);
  const parallel = Number(/const MATCH_TIERS_PARALLEL_BATCHES = (\d+);/.exec(source)[1]);
  const gameIDs = [1, 2];
  const refsOf = gameID => Array.from({ length: maxRefs }, (_, index) => `p${gameID}-${index}`);
  const matches = gameIDs.map(gameID => ({ gameId: gameID, participants: refsOf(gameID).map(playerRef => ({ playerRef })) }));
  const applied = [], diagnostics = [], apiCalls = [];
  const state = { matchTiers: new Map(), matchTierFailures: new Map(), matchTierFlights: new Set(), activeTab: 'current', activeTabs: { players: 'current' } };
  const context = {
    MATCH_TIERS_MAX_REFS: maxRefs, MATCH_TIERS_PARALLEL_BATCHES: parallel, state,
    riotTab: () => false, connected: () => true, tabGroup: () => 'players',
    shouldHydrateMatchTiers: () => true, matchTierScope: () => 'players:HN1:ref',
    matchTierScrollRoot: () => null, matchTierNodeIsVisible: () => true,
    matchTierCacheKey: (tab, gameID) => `cn:${gameID}`,
    tabServerID: () => 'HN1',
    noteMatchTierFailure: () => {}, scheduleMatchTierRetry: () => {},
    applyMatchTierValue: (container, scope, gameID, value, retryable = false) => applied.push({ gameID, value, retryable }),
    matchTierFromScores: scores => (scores.length ? { score: Math.round(scores.reduce((a, b) => a + b, 0) / scores.length), samples: scores.length } : null),
    api: (url, options, key) => new Promise((resolve, reject) => apiCalls.push({ url, body: JSON.parse(options.body), key, resolve, reject, settled: false })),
    fetch: async (url, options) => { diagnostics.push(JSON.parse(options.body)); return { status: 204 }; },
  };
  vm.runInNewContext([extractFunction(source, 'hydrateMatchTiers'), extractFunction(source, 'recordMatchTierOverviewBatch')].join('\n'), context);
  const container = { dataset: { matchTierScope: 'players:HN1:ref' }, querySelectorAll: () => [] };
  const tab = { key: 'current', overlay: false, loadingMore: false, filterPaging: false, data: { matches, player: {} } };
  const nodes = gameIDs.map(gameID => ({ dataset: { gameId: String(gameID) }, isConnected: true }));
  const hydration = context.hydrateMatchTiers(container, tab, 'players:HN1:ref', nodes);
  // 断言失败时也要能把在途批次兑现掉，否则 finally 会等一个永不 settle 的 promise。
  const settle = async () => {
    for (let round = 0; round < 10; round++) {
      const outstanding = apiCalls.filter(call => !call.settled);
      for (const call of outstanding) { call.settled = true; call.resolve({ cacheHits: 0, players: {} }); }
      await new Promise(resolve => setImmediate(resolve));
      if (!outstanding.length) break;
    }
    await Promise.race([hydration.catch(() => {}), new Promise(resolve => setTimeout(resolve, 1000))]);
  };
  return { maxRefs, parallel, refsOf, applied, diagnostics, apiCalls, hydration, settle };
}

test('R127 国服平均段位两批并行发出，第一批返回就回填对应场次', async () => {
  const scenario = batchScenario(read('gameplay.js'));
  try {
    assert.ok(scenario.parallel >= 2, '批与批之间必须并行，不能再串行 await');
    await flush();
    assert.equal(scenario.apiCalls.length, scenario.parallel, '两批必须同时在途');
    assert.equal(scenario.apiCalls[0].body.playerRefs.join(','), scenario.refsOf(1).join(','), '第一批必须是前 24 个去重玩家');
    assert.equal(scenario.apiCalls[1].body.playerRefs.join(','), scenario.refsOf(2).join(','), '第二批必须是剩下的去重玩家');
    assert.equal(scenario.apiCalls[0].body.serverId, 'HN1');
    // 第一批返回：只回填 game1，绝不等第二批。
    scenario.apiCalls[0].resolve({ cacheHits: 3, players: Object.fromEntries(scenario.refsOf(1).map(ref => [ref, { score: 1500 }])) });
    await flush(); await flush();
    assert.equal(scenario.applied.map(item => item.gameID).join(','), '1', '第一批返回后必须立刻回填它能算出的对局');
    assert.equal(scenario.applied[0].value.score, 1500);
    assert.equal(scenario.diagnostics.length, 0, '还有批次在途时不该上报整批完成');
    scenario.apiCalls[1].resolve({ cacheHits: 0, players: Object.fromEntries(scenario.refsOf(2).map(ref => [ref, { score: 1200 }])) });
    await scenario.hydration;
    assert.equal(scenario.applied.map(item => item.gameID).join(','), '1,2');
    assert.equal(scenario.applied[1].value.score, 1200);
    assert.equal(scenario.diagnostics.length, 1);
    const batch = scenario.diagnostics[0];
    assert.equal(batch.event, 'match_tiers_overview_batch');
    assert.equal(batch.totalRefs, 2 * scenario.maxRefs);
    assert.equal(batch.uniqueRefs, 2 * scenario.maxRefs);
    assert.equal(batch.cacheHits, 3);
    assert.ok(Number.isFinite(batch.durationMs), 'R127 P0-3：必须上报整批耗时 durationMs');
  } finally { await scenario.settle(); }
});

test('R127 对抗变异：把并行批次改回串行后第二批不再同时在途', async () => {
  const source = read('gameplay.js');
  const gate = 'Math.min(MATCH_TIERS_PARALLEL_BATCHES, batches.length)';
  assert.ok(source.includes(gate), '生产源码里必须存在并行批次闸门');
  const scenario = batchScenario(source.replace(gate, '1'));
  try {
    await flush();
    assert.equal(scenario.apiCalls.length, 1, '串行变异体只应有一批在途');
    scenario.apiCalls[0].resolve({ cacheHits: 0, players: Object.fromEntries(scenario.refsOf(1).map(ref => [ref, { score: 1500 }])) });
    await flush(); await flush();
    assert.equal(scenario.applied.map(item => item.gameID).join(','), '1');
    assert.ok(scenario.apiCalls[1], '串行变异体也必须在第一批返回后发出第二批');
    scenario.apiCalls[1].resolve({ cacheHits: 0, players: {} });
    await scenario.hydration;
  } finally { await scenario.settle(); }
});

/* ---------- R127 P1-c.4 / P1-c.5：韩服同屏合并与来源未收录 ---------- */

test('R127 韩服同屏对局合并成一次请求，并区分「来源暂未收录」', async () => {
  const source = read('gameplay.js');
  const coalesce = Number(/const MATCH_TIER_KR_COALESCE_MS = (\d+);/.exec(source)[1]);
  assert.ok(coalesce > 0 && coalesce <= 500, `合并窗口 ${coalesce}ms 不能为 0，也不该拖慢首屏`);
  const gameIDs = ['11', '12', '13'];
  const matches = gameIDs.map((gameID, index) => ({
    gameId: Number(gameID), createdAt: 1800000000000 + index, duration: 1500 + index,
    startedAt: 1800000180000 + index, endedAt: 1800000190000 + index,
  }));
  const nodes = gameIDs.map(gameID => ({ dataset: { gameId: gameID }, isConnected: true }));
  const applied = [], apiCalls = [];
  const state = { matchTiers: new Map(), matchTierFailures: new Map(), matchTierFlights: new Set(), activeTab: 'player-1', activeTabs: { kr: 'player-1' }, destroyed: false };
  const container = {
    dataset: { matchTierScope: 'kr:kr:player-1' }, isConnected: true,
    querySelectorAll: selector => (selector === '[data-match-tier-pending]' ? nodes : []),
  };
  const context = {
    MATCH_TIER_KR_COALESCE_MS: coalesce, state,
    riotTab: () => true, connected: () => false, tabGroup: () => 'kr',
    shouldHydrateMatchTiers: () => true, matchTierScope: () => 'kr:kr:player-1',
    matchTierScrollRoot: () => null, matchTierNodeIsVisible: () => true,
    matchTierCacheKey: (tab, gameID) => `kr:${gameID}`,
    applyMatchTierValue: (c, s, gameID, value) => applied.push({ gameID, value }),
    noteMatchTierFailure: () => {}, scheduleMatchTierRetry: () => {},
    api: (url, options, key) => new Promise(resolve => apiCalls.push({ body: JSON.parse(options.body), key, resolve })),
    window: { setTimeout: (fn, ms) => setTimeout(fn, ms) },
  };
  vm.runInNewContext(extractFunction(source, 'hydrateMatchTiers'), context);
  const tab = {
    key: 'player-1', overlay: false, loadingMore: false, filterPaging: false, riotId: {},
    data: { matches, player: { playerRef: 'public-ref', gameName: 'Faker', tagLine: 'KR1' } },
  };
  // 可见性观测器分两次回调（真机日志里是 4 场、2 场、1 场）。
  const first = context.hydrateMatchTiers(container, tab, 'kr:kr:player-1', [nodes[0], nodes[1]]);
  const second = context.hydrateMatchTiers(container, tab, 'kr:kr:player-1', [nodes[2]]);
  await new Promise(resolve => setTimeout(resolve, coalesce + 60));
  assert.equal(apiCalls.length, 1, '同一屏的可见对局必须合并成一次请求');
  assert.equal(apiCalls[0].body.matches.length, 3, '合并后必须带上这段时间内新可见的场次');
  assert.equal(apiCalls[0].body.region, 'kr');
  assert.equal(apiCalls[0].body.playerRef, 'public-ref');
  const sent = Object.fromEntries(apiCalls[0].body.matches.map(match => [String(match.gameId), match]));
  assert.equal(sent['11'].startAt, matches[0].startedAt, '必须把真正的开局时间传给后端');
  assert.equal(sent['11'].endAt, matches[0].endedAt, '必须把结束时间传给后端（OP.GG created_at 是结束口径）');
  apiCalls[0].resolve({ 11: { tier: 'GOLD', division: 'I', lp: 3 }, 12: { sourceStale: true } });
  await Promise.all([first, second]);
  const byGame = Object.fromEntries(applied.map(item => [item.gameID, item.value]));
  assert.equal(byGame['11'].tier, 'GOLD');
  assert.equal(byGame['12'].sourceStale, true, '来源落后要能被前端识别成「暂未收录」');
  assert.equal(byGame['13'], null, '没有结果的对局仍然是空值');
});

test('R127 一批失败不连累另一批：成功批照常回填，失败批留白等重试', async () => {
  const scenario = batchScenario(read('gameplay.js'));
  try {
    await flush();
    assert.equal(scenario.apiCalls.length, 2, '两批必须同时在途');
    // 第二批成功、第一批超时：这是并行批次最常见的部分失败形状。
    scenario.apiCalls[1].resolve({ cacheHits: 0, players: Object.fromEntries(scenario.refsOf(2).map(ref => [ref, { score: 1200 }])) });
    scenario.apiCalls[0].reject(new Error('timeout'));
    await scenario.hydration;
    const byGame = Object.fromEntries(scenario.applied.map(item => [item.gameID, item]));
    assert.equal(byGame['2'].value.score, 1200, '成功批的对局必须照常回填');
    assert.equal(byGame['1'].value, null, '失败批涉及的对局必须留白，不能拿残缺数据算平均');
    assert.equal(byGame['1'].retryable, true, '失败批必须保留 pending 标记，否则这一屏会永久显示横线');
    assert.equal(scenario.diagnostics.length, 1);
    assert.equal(scenario.diagnostics[0].reason, 'failed', '有批次失败就不能报 complete');
    assert.equal(scenario.diagnostics[0].uniqueRefs, 2 * scenario.maxRefs);
  } finally { await scenario.settle(); }
});
