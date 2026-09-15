"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { JSDOM } = require("../desktop/node_modules/jsdom");
const source = fs.readFileSync(process.env.R94_GAMEPLAY_SOURCE || path.join(__dirname, "gameplay.js"), "utf8");
const extract = name => {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (source.slice(start - 6, start) === "async ") start -= 6;
  return source.slice(start, source.indexOf("\n  }", start) + 4);
};
const snapshot = (gameId = 90, phase = "InProgress") => ({ gameId, phase, available: true, currentChampionId: gameId, players: [{ isCurrent: true, championId: gameId, historyState: "ok" }] });
function harness() {
  const dom = new JSDOM('<div class="live-toolbar"><div id="summary"></div><button id="refresh">刷新对局</button></div><div id="content"></div>', { pretendToBeVisual: true });
  const document = dom.window.document;
  const nodes = { liveContent: document.querySelector("#content"), liveRefresh: document.querySelector("#refresh"), liveSessionSummary: document.querySelector("#summary") };
  const state = { section: "live", beacon: { phase: "InProgress" }, live: snapshot(), settings: { liveRefresh: true }, controllers: new Map(), tabs: [], liveGameGeneration: 0, liveRecommendations: new Map([["old", {}]]), specialistRunes: new Map([["old", {}]]), proRunes: new Map([["old", {}]]) };
  let serial = 0;
  const jobs = new Map(), requests = [], observations = [], renders = [];
  const noop = () => {};
  const context = { window: { reportFlowDiagnostic: (event, reason, fields) => observations.push({ event, reason, ...fields }) }, state, nodes, document, Date: { now: () => 100000 }, connected: () => true,
    setTimeout: (fn, delay) => { jobs.set(++serial, { fn, delay }); return serial; }, clearTimeout: id => jobs.delete(id),
    api: (url, _options, key) => new Promise((resolve, reject) => {
      const controller = new AbortController(); state.controllers.set(key || "live", controller);
      requests.push({ url, resolve, reject, controller });
    }),
    recordLiveRefresh: noop, updateBeacon: phase => { state.beacon.phase = phase; }, markOverviewAfterGame: noop, scheduleBeaconPoll: noop,
    resetLivePositionOverrides: noop, resetRecommendationTabsOnChampionChange: noop, renderCapabilitySettings: noop,
    liveRecommendationsFor: () => null, ensureLiveRecommendations: noop, ensureSpecialistRunes: noop, ensureProRunes: noop,
    liveAugmentRecommendationSource: () => "", escapeHTML: String,
    renderSessionSummary: data => { nodes.liveSessionSummary.textContent = data ? `game-${data.gameId}` : ""; },
    renderRecommendationArea: data => `<article>recommendation-${data.gameId || "empty"}</article>`,
    emptyState: (title, copy, retry) => `<div>${title}<p>${copy}</p>${retry ? '<button data-gameplay-retry>重试</button>' : ''}</div>`,
    bindLiveContent: noop, applyRenderedMetricStyles: noop, prepareImages: noop,
  };
  const names = ["loadLive", "renderLive", "handleGameplayPhase", "queueLiveEventRefresh", "shouldResetLiveGameScopedState", "resetLiveGameScopedState", "softResetGameplayState", "syncLiveRetryBudget", "liveSnapshotComplete", "liveAutoRefreshStopped", "renderLiveRefreshStatus", "scheduleLiveRefresh", "normalizeLiveInterval", "liveRefreshDelayMs"];
  for (const name of ["liveGamePhase", "invalidateLiveForNewGame", "resetDisconnectedLive", "normalizeLiveGameId", "liveGameIdComparison", "recordLiveObservation", "liveSnapshotBehindPhase"]) if (source.includes(`function ${name}(`)) names.push(name);
  vm.runInNewContext(names.map(extract).join("\n") + "\n" + extract("pollGameflowPhase"), context);
  context.beaconPollDelay = () => 12000;
  const render = context.renderLive;
  context.renderLive = () => { renders.push({ live: state.live, awaiting: state.liveAwaitingGame, loading: state.liveLoading }); render(); };
  nodes.liveRefresh.addEventListener("click", () => context.loadLive(true, "manual"));
  context.renderLive();
  return { ...context, requests, jobs, observations, renders, close: () => dom.window.close(), text: () => nodes.liveContent.textContent,
    async fire() { const entry = [...jobs.entries()].find(([, job]) => job.delay <= 1000); assert.ok(entry, "queued phase refresh"); jobs.delete(entry[0]); entry[1].fn(); await Promise.resolve(); },
  };
}
const flush = () => new Promise(setImmediate);

test("R94 same-phase live responses with changed gameId invoke the transition before replacement", async () => {
  const h = harness();
  try {
    const pending = h.loadLive();
    h.requests[0].resolve(snapshot(91)); await pending;
    assert.ok(h.renders.some(render => render.live === null && render.awaiting), "gameId jump must pass through the R91 transition");
    assert.equal(h.state.live.gameId, 91); assert.equal(h.state.liveLoading, false);
    assert.ok(h.state.liveGameGeneration > 0);
    const diagnostic = h.observations.find(row => row.reason === "invalidate" && row.gameIdComparison === "different");
    assert.ok(diagnostic); assert.equal(diagnostic.phase, "InProgress");
    assert.equal(diagnostic.cachedGameId, 90); assert.equal(diagnostic.gameId, 91);
  } finally { h.close(); }
});

test("R94 unchanged phase and gameId preserve content and recommendation generation", async () => {
  const h = harness();
  try {
    h.handleGameplayPhase("InProgress", "poll", false, 90);
    const pending = h.loadLive(); h.requests[0].resolve(snapshot()); await pending;
    assert.equal(h.state.liveGameGeneration, 0);
    assert.equal(h.renders.some(render => render.live === null), false);
    assert.ok(h.observations.some(row => row.reason === "received" && row.gameIdComparison === "same"));
  } finally { h.close(); }
});

test("R94 identity polling catches a game jump even when all intermediate phases were missed", async () => {
  const h = harness();
  try {
    h.scheduleLiveRefresh(); assert.equal(h.jobs.size, 0, "R90 completed roster has stopped full refreshes");
    h.pollGameflowPhase(); assert.equal(h.requests[0].url, "/api/gameplay/phase");
    h.requests[0].resolve({ phase: "InProgress", gameId: 91 }); await flush();
    assert.equal(h.state.live, null); assert.match(h.text(), /正在识别新对局/);
    await h.fire(); assert.equal(h.requests[1].url, "/api/gameplay/live?refresh=1");
    h.handleGameplayPhase("InProgress", "sse", false, 91);
    assert.equal(h.requests[1].controller.signal.aborted, false, "repeated identity cannot keep restarting the new request");
    h.requests[1].resolve(snapshot(91)); await flush();
    assert.match(h.text(), /recommendation-91/);
    assert.equal(h.jobs.size, 0, "complete new game returns to stopped refresh policy");
  } finally { h.close(); }
});

test("R94 SSE game identity aborts a pending old game; stale replacement data cannot undo it", async () => {
  const h = harness();
  try {
    const old = h.loadLive();
    h.handleGameplayPhase("InProgress", "sse", false, 91);
    assert.equal(h.requests[0].controller.signal.aborted, true);
    await h.fire();
    h.requests[0].resolve(snapshot()); await old;
    assert.equal(h.state.liveLoading, true); assert.equal(h.state.live, null);
    h.requests[1].resolve(snapshot()); await flush();
    assert.equal(h.state.live, null); assert.equal(h.state.liveExpectedGameId, 91);
    assert.match(h.text(), /正在识别新对局/);
    const retry = h.loadLive(true); h.requests[2].resolve(snapshot(91)); await retry;
    assert.equal(h.state.live.gameId, 91);
  } finally { h.close(); }
});

test("R94 unknown IDs do not clear a known game and diagnostics distinguish first observations", () => {
  const h = harness();
  try {
    for (const id of [undefined, 0, -1, "bad"]) h.handleGameplayPhase("InProgress", "sse", false, id);
    assert.equal(h.state.live.gameId, 90); assert.equal(h.state.liveGameGeneration, 0);
    assert.ok(h.observations.some(row => row.gameIdComparison === "unavailable"));
    h.state.live = null; h.handleGameplayPhase("InProgress", "poll", false, 91);
    assert.ok(h.observations.some(row => row.gameIdComparison === "first"));
  } finally { h.close(); }
});

test("R94 same-game phase advance labels the retained snapshot and rejects a late selection response", async () => {
  const h = harness();
  try {
    h.state.live = snapshot(90, "ChampSelect"); h.state.beacon.phase = "ChampSelect";
    const pending = h.loadLive(); h.handleGameplayPhase("InProgress", "sse", false, 90);
    assert.equal(h.state.liveGameGeneration, 0); assert.match(h.text(), /同步当前对局/);
    h.requests[0].resolve(snapshot(90, "ChampSelect")); await pending;
    assert.equal(h.state.beacon.phase, "InProgress");
  } finally { h.close(); }
});

test("R94 a delayed poll cannot reverse a newer SSE identity or phase", async () => {
  for (const next of [{ phase: "InProgress", gameId: 91 }, { phase: "ChampSelect", gameId: 0 }]) {
    const h = harness();
    try {
      h.pollGameflowPhase();
      h.handleGameplayPhase(next.phase, "sse", false, next.gameId);
      h.requests[0].resolve({ phase: "InProgress", gameId: 90 }); await flush();
      assert.equal(h.state.beacon.phase, next.phase); assert.equal(h.state.liveExpectedGameId, next.gameId);
      assert.equal(h.state.live, null);
      assert.ok(h.observations.some(row => row.reason === "stale-response" && row.source === "poll"));
    } finally { h.close(); }
  }
});

test("R94 a changed identity in a new selection snapshot is not mistaken for a late same-game response", async () => {
  const h = harness();
  try {
    const pending = h.loadLive(); h.requests[0].resolve(snapshot(91, "ChampSelect")); await pending;
    assert.equal(h.state.live.gameId, 91); assert.equal(h.state.beacon.phase, "ChampSelect");
    assert.ok(h.renders.some(row => row.live === null && row.awaiting));
  } finally { h.close(); }
});

test("R94 SSE decoder retains legacy phase events and game identity payloads", () => {
  const appSource = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
  const start = appSource.indexOf("function gameplayEventDetail(");
  const context = {};
  vm.runInNewContext(appSource.slice(start, appSource.indexOf("\n  }", start) + 4), context);
  const decode = payload => JSON.parse(JSON.stringify(context.gameplayEventDetail(payload)));
  assert.deepEqual(decode('gameflow:{"phase":"InProgress","gameId":8980593274}'), { phase: "InProgress", gameId: 8980593274 });
  assert.deepEqual(decode("gameflow:ChampSelect"), { phase: "ChampSelect" });
  assert.deepEqual(decode("champselect:changed"), { changed: true });
  for (const payload of ['gameflow:{bad', 'gameflow:{"gameId":91}', 'ready']) assert.equal(decode(payload), null);
});

function diagnosticRuntime(slow = false) {
  const sent = [], jobs = new Map(), pending = []; let id = 0;
  const context = { window: {},
    setTimeout: (fn, ms) => { jobs.set(++id, { fn, ms }); return id; }, clearTimeout: id => jobs.delete(id),
    fetch: (_url, options) => { const body = JSON.parse(options.body); sent.push(body); return slow ? new Promise(resolve => pending.push(resolve)) : Promise.resolve({ status: 204 }); },
  };
  vm.runInNewContext(fs.readFileSync(process.env.R94_RUNTIME_SOURCE || path.join(__dirname, "runtime.js"), "utf8"), context);
  return { ...context.window, sent, jobs, pending,
    async tick() { const entry = [...jobs].find(([, job]) => job.ms === 1000); assert.ok(entry); jobs.delete(entry[0]); entry[1].fn(); await flush(); },
  };
}
const observation = (gameId = 8980593274, cachedGameId = 8980574903) => ({ phase: "InProgress", previousPhase: "InProgress", source: "poll", gameId, cachedGameId, gameIdComparison: gameId === cachedGameId ? "same" : "different", phaseChanged: false, invalidated: false, receivedAt: 1789370200123 });

test("R94 diagnostics preserve every repeated phase and identity comparison in bounded batches", async () => {
  const h = diagnosticRuntime();
  for (let i = 0; i < 17; i++) h.reportFlowDiagnostic("gameflow_phase_client", "received", observation(i < 12 ? 8980574903 : 8980593274));
  assert.equal(h.sent.length, 0);
  await h.tick(); await h.tick(); await h.tick();
  assert.deepEqual(h.sent.map(body => body.observations.length), [8, 8, 1]);
  const rows = h.sent.flatMap(body => body.observations);
  assert.equal(rows.length, 17); assert.equal(rows.filter(row => row.gameIdComparison === "same").length, 12);
  assert.equal(rows[12].gameId, 8980593274); assert.equal(rows[12].cachedGameId, 8980574903);
  assert.equal(rows[12].phase, "InProgress"); assert.equal(rows[12].source, "poll");
  assert.equal(rows[12].receivedAt, 1789370200123);
});

test("R94 diagnostics use one request during a stall and expose queue overflow while retaining transitions", async () => {
  const h = diagnosticRuntime(true);
  h.reportFlowDiagnostic("gameflow_phase_client", "received", observation()); await h.tick();
  for (let i = 0; i < 80; i++) h.reportFlowDiagnostic("gameflow_phase_client", "received", observation(8980574903));
  h.reportFlowDiagnostic("gameflow_phase_client", "invalidate", { ...observation(), invalidated: true });
  assert.equal(h.sent.length, 1); assert.equal(h.jobs.size, 0);
  h.pending.shift()({ status: 204 }); await flush();
  for (let i = 0; i < 8; i++) { await h.tick(); h.pending.shift()({ status: 204 }); await flush(); }
  assert.equal(h.sent.length, 9); assert.equal(h.sent[1].transportDropped, 17);
  assert.equal(h.sent.slice(1).flatMap(row => row.observations).length, 64);
  assert.ok(h.sent.at(-1).observations.some(row => row.invalidated));
  assert.equal(h.jobs.size, 0);
});

test("R94 largest diagnostic batch fits backend limit and strips unrelated data; export sends queued observations", async () => {
  const h = diagnosticRuntime();
  for (let i = 0; i < 8; i++) h.reportFlowDiagnostic("gameflow_phase_client", "stale-response", { ...observation(Number.MAX_SAFE_INTEGER, Number.MAX_SAFE_INTEGER - 1), phase: "P".repeat(64), previousPhase: "Q".repeat(64), source: "interval", playerRef: "private", token: "secret", receivedAt: 1e13 });
  await h.tick();
  assert.ok(Buffer.byteLength(JSON.stringify(h.sent[0])) < 4096);
  assert.doesNotMatch(JSON.stringify(h.sent), /private|secret/);
  // Flush invokes the pending batch immediately, before the one-second timer.
  h.reportFlowDiagnostic("gameflow_phase_client", "received", observation());
  const flushing = h.flushFlowDiagnostics(); await flush();
  const waiter = [...h.jobs.values()].find(job => job.ms === 25); assert.ok(waiter); waiter.fn(); await flushing;
  assert.equal(h.sent.at(-2).observations[0].gameId, 8980593274);
  assert.equal(h.sent.at(-1).event, "diagnostic_delivery_client"); assert.equal(h.sent.at(-1).transportPending, 0);
});

test("R94 hidden same-phase SSE identity jump resumes one forced refresh when visible", async () => {
  const h = harness();
  try {
    const context = { state: h.state, document: h.document, window: { addEventListener: (_name, fn) => { context.receive = fn; } }, handleGameplayPhase: h.handleGameplayPhase,
      clearTimeout: h.clearTimeout, queueLiveEventRefresh: h.queueLiveEventRefresh, scheduleBeaconPoll() {}, refreshAfterResync() {} };
    const sseStart = source.indexOf('  window.addEventListener("deep-legends:gameflow",');
    const visibleStart = source.indexOf('  document.addEventListener("visibilitychange",');
    vm.runInNewContext(source.slice(sseStart, source.indexOf('\n  });', sseStart) + 6), context);
    vm.runInNewContext(source.slice(visibleStart, source.indexOf('\n  });', visibleStart) + 6), context);
    Object.defineProperty(h.document, "hidden", { configurable: true, value: true });
    context.receive({ detail: { phase: "InProgress", gameId: 91 } });
    assert.equal(h.state.live, null); assert.equal(h.requests.length, 0); assert.match(h.text(), /正在识别新对局/);
    Object.defineProperty(h.document, "hidden", { configurable: true, value: false });
    h.document.dispatchEvent(new h.document.defaultView.Event("visibilitychange"));
    await h.fire(); assert.equal(h.requests[0].url, "/api/gameplay/live?refresh=1");
    h.requests[0].resolve(snapshot(91)); await flush(); assert.equal(h.state.live.gameId, 91);
  } finally { h.close(); }
});

test("R94 failed diagnostic batches release their slot without retries and recover on the next batch", async () => {
  const h = diagnosticRuntime(true);
  h.reportFlowDiagnostic("gameflow_phase_client", "received", observation()); await h.tick();
  h.reportFlowDiagnostic("gameflow_phase_client", "received", observation());
  h.pending.shift()({ status: 503 }); await flush();
  assert.equal(h.sent.length, 1); assert.equal([...h.jobs.values()].filter(job => job.ms === 750).length, 0);
  await h.tick(); assert.equal(h.sent[1].transportFailed, 1); assert.equal(h.sent[1].transportDropped, 1);
  h.pending.shift()({ status: 204 }); await flush();
  assert.equal(h.jobs.size, 0);
});
