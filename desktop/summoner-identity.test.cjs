"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("jsdom");

const WEB = path.join(__dirname, "..", "web");
const appSource = fs.readFileSync(path.join(WEB, "app.js"), "utf8");
const gameplaySource = fs.readFileSync(path.join(WEB, "gameplay.js"), "utf8");
const gameplayStyles = fs.readFileSync(path.join(WEB, "gameplay.css"), "utf8");

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
  if (source.slice(Math.max(0, start - 6), start) === "async ") start -= 6;
  const parametersStart = source.indexOf("(", start);
  let parameterDepth = 0;
  let bodyStart = -1;
  for (let index = parametersStart; index < source.length; index += 1) {
    if (source[index] === "(") parameterDepth += 1;
    if (source[index] === ")" && --parameterDepth === 0) {
      bodyStart = source.indexOf("{", index + 1);
      break;
    }
  }
  assert.notEqual(bodyStart, -1, `${name} body start not found`);
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const character = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === quote) quote = "";
      continue;
    }
    if (character === '"' || character === "'" || character === "`") {
      quote = character;
      continue;
    }
    if (character === "{") depth += 1;
    if (character === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} body is not balanced`);
}

function compileFunctions(source, names, dependencies = {}) {
  const dependencyNames = Object.keys(dependencies);
  const factory = Function(
    ...dependencyNames,
    `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn { ${names.join(", ")} };`,
  );
  return factory(...dependencyNames.map((name) => dependencies[name]));
}

function liveUpdateMapping(source = appSource) {
  const match = /const LIVE_UPDATE_STATE_SLICES = Object\.freeze\((\{[\s\S]*?\})\);/.exec(source);
  assert.ok(match, "LIVE_UPDATE_STATE_SLICES source not found");
  return Function(`"use strict"; return (${match[1]});`)();
}

function summonerLabel(summoner) {
  return `${summoner.gameName || summoner.displayName || "当前召唤师"}${summoner.tagLine ? `#${summoner.tagLine}` : ""}`;
}

test("summoner-updated is whitelisted for status and overview-player", () => {
  const mapping = liveUpdateMapping();
  assert.deepEqual(mapping["summoner-updated"], ["status", "overview-player"]);
});

test("live-update debounce accumulates both events and updates the current overview in place", async () => {
  const dom = new JSDOM("<!doctype html><body></body>", { url: "http://localhost/" });
  try {
    const w = dom.window;
    const mapping = liveUpdateMapping();
    const matches = [{ gameId: 1 }];
    const ranks = [{ queueType: "RANKED_SOLO_5x5" }];
    const current = {
      key: "current", current: true, label: "旧名字#OLD", icon: 10,
      data: { player: { gameName: "旧名字", tagLine: "OLD", displayName: "旧显示名", profileIconId: 10, summonerLevel: 30, serverName: "艾欧尼亚" }, matches, ranks },
    };
    const searched = { key: "searched", current: false, data: { player: { gameName: "别人", profileIconId: 99 } } };
    const gameplayState = { status: null, tabs: [current, searched] };
    const { applyOverviewPlayerIdentity } = compileFunctions(gameplaySource, ["applyOverviewPlayerIdentity"], {
      state: gameplayState,
      summonerLabel,
      renderPlayerTabs: () => {},
      rerenderTab: () => {},
    });
    w.addEventListener("deep-legends:overview-player", (event) => applyOverviewPlayerIdentity(event.detail.summoner));

    const timers = [];
    const appState = {
      liveUpdateSlices: new Set(), liveUpdateTimer: 0, destroyed: false,
      status: { connected: true, summoner: {} }, section: "favorites", favoritesPage: "account",
    };
    let statusRefreshes = 0;
    let accountLoads = 0;
    const nextSummoner = { gameName: "新名字", tagLine: "CN1", displayName: "新显示名", profileIconId: 22, summonerLevel: 31 };
    const { queueLiveUpdateSlices, flushLiveUpdateSlices } = compileFunctions(appSource, ["flushLiveUpdateSlices", "queueLiveUpdateSlices"], {
      state: appState,
      window: w,
      CustomEvent: w.CustomEvent,
      refreshStatus: async () => { statusRefreshes += 1; appState.status = { connected: true, summoner: nextSummoner }; gameplayState.status = appState.status; },
      loadSkins: async () => {},
      loadAccount: async () => { accountLoads += 1; },
      loadPools: async () => {},
      setTimeout: (callback, delay) => { const timer = { callback, delay, canceled: false }; timers.push(timer); return timer; },
      clearTimeout: (timer) => { if (timer) timer.canceled = true; },
    });

    queueLiveUpdateSlices(mapping["account-updated"]);
    queueLiveUpdateSlices(mapping["summoner-updated"]);
    assert.equal(timers.length, 1, "events share a fixed window instead of postponing refresh");
    assert.equal(timers[0].canceled, false);
    assert.equal(timers[0].delay, 180);
    assert.deepEqual([...appState.liveUpdateSlices].sort(), ["account", "overview-player", "status"]);
    await flushLiveUpdateSlices();

    assert.equal(statusRefreshes, 1);
    assert.equal(accountLoads, 1);
    assert.equal(current.data.player.gameName, "新名字");
    assert.equal(current.data.player.profileIconId, 22);
    assert.equal(current.data.matches, matches);
    assert.equal(current.data.ranks, ranks);
    assert.equal(searched.data.player.gameName, "别人");
    assert.equal(searched.data.player.profileIconId, 99);
  } finally {
    dom.window.close();
  }
});

test("updateStatus refreshes a connected current-tab header without resetting catalogs", () => {
  const perks = { styles: [{ id: 1 }] };
  const items = { items: [{ id: 1001 }] };
  const spells = { spells: [{ id: 4 }] };
  const current = { key: "current", current: true, label: "旧名字#OLD", icon: 10 };
  const state = {
    status: { connected: true }, tabs: [current], queueGroups: [],
    perks, items, summonerSpells: spells, section: "overview",
  };
  let tabRenders = 0;
  let catalogLoads = 0;
  const { updateStatus } = compileFunctions(gameplaySource, ["updateStatus"], {
    state,
    connected: () => Boolean(state.status?.connected),
    summonerLabel,
    renderPlayerTabs: () => { tabRenders += 1; },
    ensurePerks: () => { catalogLoads += 1; },
    ensureItems: () => { catalogLoads += 1; },
    ensureSummonerSpells: () => { catalogLoads += 1; },
  });

  updateStatus({ connected: true, summoner: { gameName: "新名字", tagLine: "CN1", profileIconId: 22 } });
  assert.equal(current.label, "新名字#CN1");
  assert.equal(current.icon, 22);
  assert.equal(tabRenders, 1);
  assert.equal(catalogLoads, 0);
  assert.equal(state.perks, perks);
  assert.equal(state.items, items);
  assert.equal(state.summonerSpells, spells);
});

test("forced overview aborts a loading request and starts a newer request", async () => {
  let aborted = 0;
  let requests = 0;
  let renders = 0;
  const tab = {
    key: "current", current: true, loading: true, loadingMore: false,
    data: { player: {}, matches: [], pagination: { hasMore: false } }, matchFilter: "all",
  };
  const state = {
    controllers: new Map([["overview:current", { abort: () => { aborted += 1; } }]]),
    settings: { matchCount: 20 }, lastCapabilities: [],
  };
  const { loadOverview } = compileFunctions(gameplaySource, ["loadOverview"], {
    state,
    loadOPGGSeasonSummary: async () => false,
    loadOverviewCurrentGame: async () => false,
    tabGroup: () => "players",
    tabReady: () => true,
    rerenderTab: () => { renders += 1; },
    showToast: () => {},
    normalizedPagination: () => ({ begIndex: 0, count: 0, hasMore: false, nextBegIndex: 0 }),
    api: async () => { requests += 1; return { player: { playerRef: "player_ref", gameName: "新名字", profileIconId: 22 }, matches: [], capabilities: [] }; },
    rememberTabPlayerRef: () => {},
    playerLabel: (player) => player.gameName,
    renderCapabilitySettings: () => {},
    appendOverviewMatches: () => {},
    AUTO_PAGE_DELAY_MS: 400,
  });

  const loaded = await loadOverview(tab, true);
  assert.equal(loaded, true);
  assert.equal(aborted, 1);
  assert.equal(requests, 1);
  assert.equal(tab.loading, false);
  assert.equal(tab.overviewRequestToken, 1);
  assert.ok(renders >= 1);
});

test("overview-player changes only five identity fields on the current tab", () => {
  const matches = [{ gameId: 1 }];
  const ranks = [{ tier: "GOLD" }];
  const current = {
    key: "current", current: true, label: "旧名字#OLD", icon: 10,
    data: {
      player: { gameName: "旧名字", tagLine: "OLD", displayName: "旧显示名", profileIconId: 10, summonerLevel: 30, serverName: "艾欧尼亚", backgroundPath: "/keep.jpg" },
      matches, ranks,
    },
  };
  const searched = { key: "searched", current: false, data: { player: { gameName: "别人", tagLine: "CN2", profileIconId: 99 } } };
  const searchedBefore = structuredClone(searched);
  const state = { status: { connected: true }, tabs: [current, searched] };
  let tabRenders = 0;
  let contentRenders = 0;
  const { applyOverviewPlayerIdentity } = compileFunctions(gameplaySource, ["applyOverviewPlayerIdentity"], {
    state,
    summonerLabel,
    renderPlayerTabs: () => { tabRenders += 1; },
    rerenderTab: () => { contentRenders += 1; },
  });

  const changed = applyOverviewPlayerIdentity({ gameName: "新名字", tagLine: "CN1", displayName: "新显示名", profileIconId: 22, summonerLevel: 31 });
  assert.equal(changed, true);
  assert.deepEqual(
    Object.fromEntries(["gameName", "tagLine", "displayName", "profileIconId", "summonerLevel"].map((field) => [field, current.data.player[field]])),
    { gameName: "新名字", tagLine: "CN1", displayName: "新显示名", profileIconId: 22, summonerLevel: 31 },
  );
  assert.equal(current.data.player.serverName, "艾欧尼亚");
  assert.equal(current.data.player.backgroundPath, "/keep.jpg");
  assert.equal(current.data.matches, matches);
  assert.equal(current.data.ranks, ranks);
  assert.deepEqual(searched, searchedBefore);
  assert.equal(tabRenders, 1);
  assert.equal(contentRenders, 1);
});

test("overview-player identity slice also updates background fields in place", () => {
  const matches = [{ gameId: 7 }];
  const ranks = [{ tier: "PLATINUM" }];
  const current = {
    key: "current", current: true, label: "旧名字#OLD", icon: 10,
    data: { player: { gameName: "旧名字", profileIconId: 10, backgroundPath: "/old.jpg" }, matches, ranks },
  };
  const other = { key: "other", current: false, data: { player: { gameName: "别人", backgroundPath: "/other.jpg" } } };
  const state = { status: { connected: true }, tabs: [current, other] };
  const { applyOverviewPlayerIdentity } = compileFunctions(gameplaySource, ["applyOverviewPlayerIdentity"], {
    state, summonerLabel, renderPlayerTabs: () => {}, rerenderTab: () => {},
  });
  applyOverviewPlayerIdentity({ gameName: "新名字", profileIconId: 22, backgroundSkinId: 164001, backgroundSkinName: "新背景", backgroundSource: "gtimg", backgroundPath: "/new.jpg" });
  assert.equal(current.data.player.backgroundSkinId, 164001);
  assert.equal(current.data.player.backgroundPath, "/new.jpg");
  assert.equal(current.data.matches, matches);
  assert.equal(current.data.ranks, ranks);
  assert.equal(other.data.player.backgroundPath, "/other.jpg");
});

test("identity-ready status removes the global startup lock while snapshot loads", () => {
  const dom = new JSDOM('<!doctype html><div id="app-frame" inert></div><div id="startup-loading"></div>');
  try {
    const appFrame = dom.window.document.querySelector("#app-frame");
    const startupLoading = dom.window.document.querySelector("#startup-loading");
    const state = { status: { connected: true, identityReady: true, snapshotReady: false }, loading: false, overlayForced: false, overlaySuppressed: false, overlayBaselineAttempt: "", statusDelay: 0 };
    let hidden = 0;
    const { updateReadingOverlay } = compileFunctions(appSource, ["updateReadingOverlay"], {
      state,
      hideReadingOverlay: () => { hidden += 1; appFrame.removeAttribute("inert"); },
      showReadingOverlay: () => { startupLoading.hidden = false; appFrame.setAttribute("inert", ""); },
      snapshotRetryText: () => "",
    });
    updateReadingOverlay(false);
    assert.equal(hidden, 1);
    assert.equal(appFrame.hasAttribute("inert"), false);
  } finally {
    dom.window.close();
  }
});

test("overview refresh button exposes a stable disabled loading state", () => {
  const dom = new JSDOM('<button id="overview-refresh">刷新战绩</button>');
  try {
    const nodes = { overviewRefresh: dom.window.document.getElementById("overview-refresh") };
    const { setOverviewRefreshLoading } = compileFunctions(gameplaySource, ["setOverviewRefreshLoading"], { overviewWorkspace: () => ({ refresh: nodes.overviewRefresh }) });
    setOverviewRefreshLoading("players", true);
    assert.equal(nodes.overviewRefresh.disabled, true);
    assert.equal(nodes.overviewRefresh.classList.contains("is-loading"), true);
    assert.equal(nodes.overviewRefresh.getAttribute("aria-busy"), "true");
    setOverviewRefreshLoading("players", false);
    assert.equal(nodes.overviewRefresh.disabled, false);
    assert.equal(nodes.overviewRefresh.classList.contains("is-loading"), false);
    assert.equal(nodes.overviewRefresh.hasAttribute("aria-busy"), false);
    assert.match(gameplayStyles, /#overview-refresh\s*\{[^}]*min-width:\s*98px/);
    assert.match(gameplayStyles, /#overview-refresh\.is-loading::before\s*\{[^}]*animation:\s*spin/);
  } finally {
    dom.window.close();
  }
});

test("manual reread awaits the backend identity refresh before hard-refresh dispatch", () => {
  const start = appSource.indexOf('el.refresh.addEventListener("click"');
  const end = appSource.indexOf("el.startupLoadingRetry", start);
  assert.ok(start >= 0 && end > start, "manual refresh handler source not found");
  const handler = appSource.slice(start, end);
  const backendRefresh = handler.indexOf('await api("/api/refresh"');
  const hardRefresh = handler.indexOf('window.dispatchEvent(new CustomEvent("deep-legends:hard-refresh"');
  assert.ok(backendRefresh >= 0 && hardRefresh > backendRefresh, "hard-refresh ran before the awaited backend identity refresh");
  assert.match(handler, /catch \(error\)[\s\S]*window\.dispatchEvent/);
});
