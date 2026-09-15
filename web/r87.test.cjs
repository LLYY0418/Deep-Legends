'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const read = file => fs.readFileSync(path.join(process.env.R87_TEST_WEB_ROOT || __dirname, file), 'utf8');
function extract(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  const end = source.indexOf('\n  }', start) + 4;
  return source.slice(start, end);
}
function compile(file, names, deps = {}, mutation = s => s) {
  const source = mutation(read(file));
  const context = { ...deps };
  vm.runInNewContext(names.map(n => extract(source, n)).join('\n') + `\nObject.assign(globalThis,{${names.join(',')}});`, context);
  return context;
}
function timers() {
  let now = 100000, seq = 0;
  const jobs = new Map();
  return { Date: { now: () => now }, jobs,
    setTimeout(fn, ms) { jobs.set(++seq, { fn, at: now + ms }); return seq; },
    clearTimeout(id) { jobs.delete(id); },
    async advance(ms) { now += ms; for (const [id, job] of [...jobs]) if (job.at <= now) { jobs.delete(id); await job.fn(); } await new Promise(setImmediate); },
  };
}
function liveHarness() {
  const timer = timers(), calls = [], reports = [], listeners = new Map();
  const state = { section: 'overview', beacon: { phase: 'Lobby' }, tabs: [], settings: {}, controllers: new Map() };
  const document = { hidden: true };
  const deps = { ...timer, state, document, connected: () => true, fetch: async (_url, options) => { reports.push(JSON.parse(options.body)); },
    api: async url => { calls.push(url); return { phase: 'InProgress', players: [] }; },
    updateBeacon: phase => { state.beacon.phase = phase; }, scheduleBeaconPoll() {}, markOverviewAfterGame() {},
    renderLive() {}, resetLiveGameScopedState() {}, shouldResetLiveGameScopedState: () => false, resetLivePositionOverrides() {}, resetRecommendationTabsOnChampionChange() {},
    renderCapabilitySettings() {}, liveRecommendationsFor: () => null, ensureLiveRecommendations() {}, ensureSpecialistRunes() {}, ensureProRunes() {}, scheduleLiveRefresh() {},
    activeTab: () => state.tabs[0], overviewGroupForSection: () => 'cn', loadOverview: async () => true,
    window: { reportFlowDiagnostic: (event, reason, fields) => reports.push({event, reason, ...fields}), addEventListener: (event, fn) => listeners.set(event, fn) },
  };
  const context = compile('gameplay.js', ['recordLiveRefresh', 'queueLiveEventRefresh', 'loadLive', 'liveGamePhase', 'normalizeLiveGameId', 'liveGameIdComparison', 'recordLiveObservation', 'liveSnapshotBehindPhase', 'invalidateLiveForNewGame', 'syncLiveRetryBudget', 'handleGameplayPhase', 'refreshAfterResync', 'pollGameflowPhase'], deps);
  context.BEACON_IDLE_POLL_MS = 12000; context.BEACON_DISCONNECTED_POLL_MS = 1000; context.beaconPollDelay = () => 12000;
  const source = read('gameplay.js'); const start = source.indexOf('  window.addEventListener("deep-legends:gameflow",');
  vm.runInNewContext(source.slice(start, source.indexOf('\n  });', start) + 6), context);
  return { state, document, timer, calls, reports, context, listeners };
}

test('R87 hidden SSE phase is loaded when visibility resumes with overview resync pending', async () => {
  const h = liveHarness();
  h.listeners.get('deep-legends:gameflow')({ detail: { phase: 'InProgress' } });
  assert.equal(h.calls.length, 0); assert.equal(h.state.liveRefreshQueued, true);
  h.document.hidden = false; h.state.resyncPending = true;
  h.context.refreshAfterResync();
  assert.equal(h.state.liveRefreshQueued, true, 'overview resync must preserve queued live work until it starts');
  await h.timer.advance(1000);
  assert.deepEqual(h.calls, ['/api/gameplay/live']); assert.equal(h.state.liveRefreshQueued, false);
  for (const reason of ['phase', 'queue', 'load']) assert.ok(h.reports.some(r => r.reason === reason && 'hidden' in r && 'section' in r && 'phaseChanged' in r && 'liveRefreshQueued' in r));
});

test('R87 phase poll independently loads once and repeated unchanged phase causes no roster polling', async () => {
  const h = liveHarness(); h.document.hidden = false;
  h.context.pollGameflowPhase(); await new Promise(setImmediate); await h.timer.advance(1000);
  h.context.pollGameflowPhase(); await new Promise(setImmediate); await h.timer.advance(1000);
  assert.equal(h.calls.filter(c => c === '/api/gameplay/live').length, 1);
});

test('R87 all tabs become dirty, inactive players stay off network, retry is bounded and delayed', async () => {
  const timer = timers(), calls = [];
  const self = { current: true, data: { matches: [{ gameId: 1 }] } }, other = { data: { matches: [{ gameId: 2 }] } };
  const state = { section: 'overview', tabs: [self, other] }; let active = self;
  const h = compile('gameplay.js', ['markOverviewAfterGame', 'scheduleDirtyOverview'], { ...timer, state, document: { hidden: false }, connected: () => true,
    activeTab: () => active, loadOverview: async (tab, force) => { assert.equal(force, true); calls.push(tab); return true; } });
  h.markOverviewAfterGame('WaitingForStats'); h.markOverviewAfterGame('PreEndOfGame'); h.markOverviewAfterGame('EndOfGame');
  assert.equal(other.dirty, true); assert.equal(other.dirtyTimer, undefined); assert.equal(calls.length, 0);
  await timer.advance(6999); assert.equal(calls.length, 0);
  await timer.advance(1); await timer.advance(8000); await timer.advance(8000); await timer.advance(60000);
  assert.equal(calls.length, 3); assert.ok(calls.every(tab => tab === self));
  active = other; h.scheduleDirtyOverview(other); await timer.advance(0);
  assert.equal(calls.at(-1), other);
});

test('R87 suite fetch and stalled response bodies time out at 15 seconds', async () => {
  for (const stalledBody of [false, true]) {
    const timer = timers(); let signal;
    const h = compile('suite.js', ['api'], { ...timer, AbortController, fetch: (_url, options) => {
      signal = options.signal;
      return stalledBody ? Promise.resolve({ ok: true, text: () => new Promise(() => {}) }) : new Promise(() => {});
    } });
    const pending = h.api('/api/claim/execute'); const failed = assert.rejects(pending, error => error.errorKind === 'timeout' && /超时/.test(error.message));
    assert.ok([...timer.jobs.values()].some(job => job.at === 115000), 'request must arm a 15 second timeout');
    await timer.advance(14999); assert.equal(signal.aborted, false);
    await timer.advance(1); await failed; assert.equal(signal.aborted, true); assert.equal(timer.jobs.size, 0);
  }
});

test('R87 claim timeout advances progress, continues remaining claims and clears watchdog', async () => {
  const state = { selectedClaims: new Set(['slow', 'ok']), claimChoices: new Map(), claimFailures: new Map() }, reports = [];
  let calls = 0, cleared = false;
  const h = compile('suite.js', ['executeClaims', 'recordClaimProgress'], { state, Date, renderClaims() {}, toast() {}, loadClaims() {},
    setInterval: () => 1, clearInterval: () => { cleared = true; }, fetch: async (_url, options) => reports.push(JSON.parse(options.body)),
    api: async () => { if (++calls === 1) throw Object.assign(new Error('领取超时'), { errorKind: 'timeout' }); return { results: [{ ok: true }] }; },
  });
  await h.executeClaims();
  assert.equal(calls, 2); assert.equal(state.claimProgress.done, 2); assert.equal(state.claimProgress.failed, 1);
  assert.equal(state.claiming, false); assert.equal(cleared, true); assert.ok(state.claimFailures.has('slow')); assert.equal(state.selectedClaims.has('ok'), false);
  assert.deepEqual(reports.map(r => r.reason), ['begin', 'item-timeout', 'end']);
});

test('R87 pending loot is distinguished from pool matching, and all end phases share settlement display', () => {
  const loot = compile('app.js', ['lootName', 'lootNamePending']);
  assert.equal(loot.lootNamePending({ dataPending: true }), true);
  assert.match(read('app.js'), /客户端数据暂未同步，可稍后重试/); assert.match(read('app.js'), /查看皮肤池匹配失败条目/);
  const h = compile('suite.js', ['suiteDisplayPhase']);
  for (const phase of ['WaitingForStats', 'PreEndOfGame', 'EndOfGame']) assert.equal(h.suiteDisplayPhase(phase), 'EndOfGame');
});
