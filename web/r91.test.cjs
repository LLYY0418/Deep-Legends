"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { JSDOM } = require("../desktop/node_modules/jsdom");
const source = fs.readFileSync(process.env.R91_GAMEPLAY_SOURCE || path.join(__dirname, "gameplay.js"), "utf8");
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
  const jobs = new Map(), requests = [];
  const noop = () => {};
  const context = { window: {}, state, nodes, document, Date: { now: () => 100000 }, connected: () => true,
    setTimeout: (fn, delay) => { jobs.set(++serial, { fn, delay }); return serial; }, clearTimeout: id => jobs.delete(id),
    api: url => new Promise((resolve, reject) => {
      const controller = new AbortController(); state.controllers.set("live", controller);
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
  const names = ["updateLiveLoadingVisibility", "loadLive", "liveRecommendationMarkup", "renderLive", "handleGameplayPhase", "queueLiveEventRefresh", "shouldResetLiveGameScopedState", "resetLiveGameScopedState", "softResetGameplayState", "syncLiveRetryBudget", "liveSnapshotComplete", "liveAutoRefreshStopped", "renderLiveRefreshStatus", "scheduleLiveRefresh", "normalizeLiveInterval", "liveRefreshDelayMs"];
  for (const name of ["liveGamePhase", "invalidateLiveForNewGame", "resetDisconnectedLive", "normalizeLiveGameId", "liveGameIdComparison", "recordLiveObservation", "liveSnapshotBehindPhase"]) if (source.includes(`function ${name}(`)) names.push(name);
  vm.runInNewContext(names.map(extract).join("\n"), context);
  nodes.liveRefresh.addEventListener("click", () => context.loadLive(true, "manual"));
  context.renderLive();
  return { ...context, requests, jobs, close: () => dom.window.close(), text: () => nodes.liveContent.textContent,
    async fire() { const entry = [...jobs.entries()].find(([, job]) => job.delay <= 1000); assert.ok(entry, "queued phase refresh"); jobs.delete(entry[0]); entry[1].fn(); await Promise.resolve(); },
  };
}
const flush = () => new Promise(setImmediate);

test("R91 new game removes old recommendations and summary before the deferred response", async () => {
  for (const [previous, next] of [["EndOfGame", "ChampSelect"], ["Lobby", "InProgress"], ["InProgress", "ChampSelect"], ["Matchmaking", "GameStart"], ["None", "Reconnect"]]) {
    const h = harness();
    try {
      h.state.beacon.phase = previous;
      h.handleGameplayPhase(next);
      assert.doesNotMatch(h.text(), /recommendation-90/);
      assert.match(h.text(), /正在识别新对局/);
      assert.equal(h.nodes.liveSessionSummary.textContent, "");
      assert.ok(h.nodes.liveContent.querySelector("button"), "transition has a manual exit");
      assert.equal(h.state.live, null);
      assert.equal(h.state.liveRecommendations.size + h.state.specialistRunes.size + h.state.proRunes.size, 0);
      await h.fire();
      assert.doesNotMatch(h.text(), /recommendation-90/);
      h.requests[0].resolve(snapshot(91, next)); await flush();
      assert.match(h.text(), /recommendation-91/);
      assert.doesNotMatch(h.text(), /正在识别新对局/);
    } finally { h.close(); }
  }
});

test("R91 same game refresh and active phase progression preserve the current content", async () => {
  for (const [previous, next] of [["InProgress", "InProgress"], ["ChampSelect", "GameStart"], ["GameStart", "InProgress"], ["InProgress", "Reconnect"], ["Reconnect", "InProgress"]]) {
    const h = harness();
    try {
      h.state.beacon.phase = previous; h.state.live.phase = previous;
      h.handleGameplayPhase(next);
      const pending = h.loadLive();
      assert.match(h.text(), /recommendation-90/);
      assert.doesNotMatch(h.nodes.liveContent.innerHTML, /gameplay-skeleton|正在识别新对局/);
      assert.equal(h.state.liveGameGeneration, 0);
      h.requests[0].resolve(snapshot(90, next)); await pending;
      assert.match(h.text(), /recommendation-90/);
    } finally { h.close(); }
  }
});

test("R91 repeated manual refresh immediately aborts and replaces only the live request with visible feedback", async () => {
  for (const settle of ["resolve", "reject"]) {
    const h = harness();
    try {
      const overview = new AbortController(), collection = new AbortController();
      h.state.controllers.set("overview:current", overview); h.state.controllers.set("collection", collection);
      h.state.tabs = [{ loading: true, loadingMore: true, paginationStalls: 2, nextAutoAppendAt: 123, data: { pagination: { hasMore: true, autoPaused: true } } }];
      const tabsBefore = JSON.stringify(h.state.tabs);
      h.state.recommendationTab = "build"; h.state.recommendationTabTouched = true; h.state.runeSourceTab = "specialist";
      const pending = h.loadLive();
      await h.loadLive(false, "event");
      assert.equal(h.requests.length, 1, "background refresh still coalesces");
      assert.equal(h.requests[0].controller.signal.aborted, false);
      h.nodes.liveRefresh.click();
      assert.equal(h.requests[0].controller.signal.aborted, true);
      assert.equal(h.requests.length, 2, "manual retry cannot wait for the stalled request to settle");
      assert.equal(h.requests[1].url, "/api/gameplay/live?refresh=1");
      assert.match(h.nodes.liveRefresh.textContent, /正在刷新/);
      assert.equal(h.nodes.liveRefresh.getAttribute("aria-busy"), "true");
      assert.doesNotMatch(h.nodes.liveContent.querySelector("[data-live-status]").textContent, /正在刷新/);
      const delayed = [...h.jobs.entries()].find(([, job]) => job.delay === 240);
      assert.ok(delayed); h.jobs.delete(delayed[0]); delayed[1].fn();
      assert.match(h.text(), /正在刷新/); assert.match(h.text(), /recommendation-90/);
      assert.equal(overview.signal.aborted, false); assert.equal(collection.signal.aborted, false);
      assert.equal(h.state.controllers.get("overview:current"), overview); assert.equal(h.state.controllers.get("collection"), collection);
      assert.equal(JSON.stringify(h.state.tabs), tabsBefore);
      assert.equal(h.state.recommendationTab, "build"); assert.equal(h.state.runeSourceTab, "specialist");
      assert.equal(h.state.liveRecommendations.size, 1); assert.equal(h.state.liveGameGeneration, 0);
      if (settle === "resolve") h.requests[0].resolve(snapshot(89));
      else h.requests[0].reject(Object.assign(new Error("cancelled"), { name: "RequestCancelled" }));
      await pending;
      assert.equal(h.state.liveLoading, true); assert.equal(h.state.live.gameId, 90); assert.equal(h.state.liveError, "");
      h.requests[1].resolve(snapshot()); await flush();
      assert.equal(h.requests.length, 2, "old finalizer cannot start an extra queued request");
      assert.equal(h.nodes.liveRefresh.textContent, "刷新对局");
      assert.equal(h.nodes.liveRefresh.getAttribute("aria-busy"), "false");
    } finally { h.close(); }
  }
});

test("R91 boundary cancels the old request and its late settlement cannot own the new state", async () => {
  for (const settle of ["resolve", "reject"]) {
    const h = harness();
    try {
      const old = h.loadLive(); const oldRequest = h.requests[0];
      h.handleGameplayPhase("ChampSelect");
      assert.equal(oldRequest.controller.signal.aborted, true);
      await h.fire();
      assert.equal(h.requests.length, 2, "new request does not wait for old request timeout");
      if (settle === "resolve") oldRequest.resolve(snapshot());
      else oldRequest.reject(Object.assign(new Error("cancelled"), { name: "RequestCancelled" }));
      await old;
      assert.equal(h.state.liveLoading, true);
      assert.equal(h.state.live, null); assert.equal(h.state.liveError, "");
      h.requests[1].resolve(snapshot(91, "ChampSelect")); await flush();
      assert.match(h.text(), /recommendation-91/);
    } finally { h.close(); }
  }
});

test("R91 failed, timed out and cancelled new-game requests have an explicit working retry", async () => {
  for (const name of ["Error", "TimeoutError", "RequestCancelled"]) {
    const h = harness();
    try {
      h.handleGameplayPhase("ChampSelect"); await h.fire();
      h.requests[0].reject(Object.assign(new Error("读取中断"), { name })); await flush();
      assert.match(h.text(), /读取失败/); assert.doesNotMatch(h.text(), /recommendation-90/);
      h.nodes.liveContent.querySelector("[data-gameplay-retry]").click();
      assert.equal(h.requests.length, 2); assert.match(h.requests[1].url, /refresh=1/);
      h.requests[1].resolve(snapshot(91, "ChampSelect")); await flush();
      assert.match(h.text(), /recommendation-91/);
    } finally { h.close(); }
  }
});

test("R91 empty new-game snapshots stay explicit and bounded instead of showing an empty recommendation panel", async () => {
  const h = harness();
  try {
    h.state.beacon.phase = "Lobby"; h.handleGameplayPhase("InProgress"); await h.fire();
    h.requests[0].resolve({ phase: "InProgress", available: false, players: [] }); await flush();
    assert.match(h.text(), /正在识别新对局/); assert.doesNotMatch(h.text(), /recommendation-empty|recommendation-90/);
    h.state.liveRetryAttempts = 8; h.scheduleLiveRefresh(); h.renderLive();
    assert.equal(h.jobs.size, 0); assert.match(h.text(), /已停止自动重试/);
    assert.ok(h.nodes.liveContent.querySelector("button"));
  } finally { h.close(); }
});

test("R91 soft reset invalidates the aborted loader before a replacement starts", async () => {
  const h = harness();
  try {
    const old = h.loadLive();
    h.softResetGameplayState();
    const current = h.loadLive(true);
    h.requests[0].resolve(snapshot(89)); await old;
    assert.equal(h.state.liveLoading, true); assert.equal(h.state.live.gameId, 90);
    h.requests[1].resolve(snapshot()); await current;
    assert.equal(h.state.liveLoading, false);
  } finally { h.close(); }
});

test("R91 direct loader detects a boundary already observed by the beacon", async () => {
  const h = harness();
  try {
    h.state.live.phase = "EndOfGame"; h.state.beacon.phase = "InProgress";
    const pending = h.loadLive();
    assert.doesNotMatch(h.text(), /recommendation-90/); assert.match(h.text(), /正在识别新对局/);
    h.requests[0].resolve(snapshot(91)); await pending;
  } finally { h.close(); }
});

test("R91 hidden phase transitions invalidate immediately and preload before foregrounding", async () => {
  const h = harness();
  try {
    Object.defineProperty(h.document, "hidden", { value: true, configurable: true });
    h.handleGameplayPhase("ChampSelect");
    assert.equal(h.state.live, null); assert.equal(h.state.liveRefreshQueued, true);
    assert.equal(h.requests.length, 0); assert.equal(h.jobs.size, 1);
    await h.fire(); assert.equal(h.requests.length, 1);
    Object.defineProperty(h.document, "hidden", { value: false });
    h.renderLive(); assert.match(h.text(), /正在识别新对局/);
    h.queueLiveEventRefresh("visibility");
    h.requests[0].resolve(snapshot(91, "ChampSelect")); await flush();
    assert.match(h.text(), /recommendation-91/);
  } finally { h.close(); }
});

test("R91 event-stream disconnection cancels stale work and offers recovery", async () => {
  const h = harness();
  try {
    assert.match(source, /addEventListener\("deep-legends:live-disconnected", resetDisconnectedLive\)/);
    const pending = h.loadLive();
    h.resetDisconnectedLive();
    assert.equal(h.requests[0].controller.signal.aborted, true);
    assert.match(h.text(), /读取失败.*实时连接已中断/);
    assert.equal(h.nodes.liveSessionSummary.textContent, "");
    h.requests[0].resolve(snapshot()); await pending;
    assert.doesNotMatch(h.text(), /recommendation-90/);
    h.nodes.liveContent.querySelector("[data-gameplay-retry]").click();
    h.requests[1].resolve(snapshot(91)); await flush();
    assert.match(h.text(), /recommendation-91/);
  } finally { h.close(); }
});

test("R91 lagging inactive responses cannot erase a new-game signal; leaving the game settles the transition", async () => {
  const h = harness();
  try {
    h.handleGameplayPhase("ChampSelect"); await h.fire();
    h.requests[0].resolve({ phase: "Lobby", available: false }); await flush();
    assert.equal(h.state.beacon.phase, "ChampSelect"); assert.match(h.text(), /正在识别新对局/);
    assert.ok([...h.jobs.values()].some(job => job.delay === 3000));
    h.handleGameplayPhase("Lobby"); await h.fire();
    h.requests[1].resolve({ phase: "Lobby", available: false }); await flush();
    assert.equal(h.state.liveAwaitingGame, false); assert.doesNotMatch(h.text(), /正在识别新对局/);
  } finally { h.close(); }
});

test("R91 an empty available roster stays pending; unsupported mode and known gameId changes settle safely", async () => {
  const h = harness();
  try {
    h.handleGameplayPhase("ChampSelect"); await h.fire();
    h.requests[0].resolve({ phase: "ChampSelect", available: true, players: [] }); await flush();
    assert.match(h.text(), /正在识别新对局/);
    const unsupported = h.loadLive(true);
    h.requests[1].resolve({ phase: "ChampSelect", unsupported: true, unsupportedReason: "此模式暂不支持" }); await unsupported;
    assert.equal(h.state.liveAwaitingGame, false); assert.match(h.text(), /此模式暂不支持/);
    h.state.live = snapshot(); h.state.beacon.phase = "InProgress";
    const previousGeneration = h.state.liveGameGeneration;
    const next = h.loadLive(); h.requests[2].resolve(snapshot(91)); await next;
    assert.ok(h.state.liveGameGeneration > previousGeneration);
    assert.match(h.text(), /recommendation-91/);
  } finally { h.close(); }
});

test("R91 cancellation without a replacement cannot leave the first load blank", async () => {
  const h = harness();
  try {
    h.state.live = null;
    const pending = h.loadLive();
    h.requests[0].reject(Object.assign(new Error("cancelled"), { name: "RequestCancelled" })); await pending;
    assert.match(h.text(), /读取失败.*读取已中断/);
    assert.ok(h.nodes.liveContent.querySelector("[data-gameplay-retry]"));
    assert.equal(h.state.liveLoading, false);
  } finally { h.close(); }
});
