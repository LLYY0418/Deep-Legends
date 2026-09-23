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

const WEB = path.join(__dirname, "..", "backend", "web");
const SCRIPTS = ["runtime.js", "demo-data.js", "app.js", "favorites-facade.js", "gameplay.js", "champions.js", "friends.js", "suite.js"];
const gameplaySource = fs.readFileSync(process.env.R104_GAMEPLAY_SOURCE || path.join(WEB, "gameplay.js"), "utf8");
const suiteSource = fs.readFileSync(path.join(WEB, "suite.js"), "utf8");
const appStyles = fs.readFileSync(path.join(WEB, "app.css"), "utf8");
const gameplayStyles = fs.readFileSync(path.join(WEB, "gameplay.css"), "utf8");
const suiteStyles = fs.readFileSync(path.join(WEB, "suite.css"), "utf8");

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
  if (source.slice(Math.max(0, start - 6), start) === "async ") start -= 6;
  const bodyStart = source.indexOf("{", start);
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

function compileFunctions(source, names, dependencies) {
  const dependencyNames = Object.keys(dependencies);
  const factory = Function(
    ...dependencyNames,
    `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn { ${names.join(", ")} };`,
  );
  return factory(...dependencyNames.map((name) => dependencies[name]));
}

function collapsedBeaconStyles(css = appStyles) {
  const dom = new JSDOM(`<html data-sidebar="collapsed"><head><style>${css}</style><style>${gameplayStyles}</style></head><body><button id="section-live" class="section-tab"><span class="section-label">对局</span><span class="live-beacon"></span></button></body></html>`, { pretendToBeVisual: true });
  const label = dom.window.document.querySelector(".section-label");
  const beacon = dom.window.document.querySelector(".live-beacon");
  return {
    dom,
    labelDisplay: dom.window.getComputedStyle(label).display,
    beaconDisplay: dom.window.getComputedStyle(beacon).display,
  };
}

test("collapsed sidebar hides the label but keeps the live beacon rendered", () => {
  const rendered = collapsedBeaconStyles();
  assert.equal(rendered.labelDisplay, "none");
  assert.notEqual(rendered.beaconDisplay, "none");
  rendered.dom.window.close();

  const mutated = appStyles.replace(
    ':root[data-sidebar="collapsed"] .section-tab > span:not(.live-beacon),',
    ':root[data-sidebar="collapsed"] .section-tab > span,',
  );
  const regression = collapsedBeaconStyles(mutated);
  assert.equal(regression.beaconDisplay, "none", "the rendered-state guard must catch selectors that hide every span");
  regression.dom.window.close();
});

const openDemoWindows = new Set();
test.afterEach(() => {
  for (const w of openDemoWindows) w.close();
});

function bootDemoApp(options = {}) {
  const html = fs.readFileSync(path.join(WEB, "index.html"), "utf8");
  const errors = [];
	const eventSources = [];
  const dom = new JSDOM(html, { url: "http://127.0.0.1:1/?demo", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window;
  openDemoWindows.add(w);
  const mutationObservers = new Set();
  const NativeMutationObserver = w.MutationObserver;
  w.MutationObserver = class extends NativeMutationObserver {
    constructor(callback) {
      super(callback);
      mutationObservers.add(this);
    }
  };
  const closeWindow = w.close.bind(w);
  w.close = () => {
    if (!openDemoWindows.delete(w)) return;
    try {
      w.dispatchEvent(new w.CustomEvent("deep-legends:dispose"));
    } finally {
      // jsdom keeps queued observer microtasks after deleting window.document.
      // Keep real observer behavior during each test, but end it with its window.
      for (const observer of mutationObservers) observer.disconnect();
      mutationObservers.clear();
      closeWindow();
    }
  };
  w.onerror = (message, source, line, column, error) => { errors.push(String((error && error.stack) || message)); };
  w.addEventListener("unhandledrejection", (event) => { errors.push(String((event.reason && event.reason.stack) || event.reason)); });
  // jsdom 未实现的浏览器能力，用最小替身补齐（只影响可见性/尺寸，不影响渲染分支）。
  w.IntersectionObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.matchMedia = w.matchMedia || (() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }));
  w.scrollTo = () => {};
  w.HTMLElement.prototype.scrollIntoView = () => {};
  w.Element.prototype.scrollTo = function () {};
  w.structuredClone = globalThis.structuredClone;
  w.fetch = globalThis.fetch;
  w.Response = globalThis.Response;
  w.Headers = globalThis.Headers;
  w.Request = globalThis.Request;
	if (options.liveEvents) {
	  w.EventSource = class MockEventSource {
		static CLOSED = 2;
		constructor(url) { this.url = url; this.readyState = 1; eventSources.push(this); }
		close() { this.readyState = MockEventSource.CLOSED; }
	  };
	}
	if (options.matchCount) w.localStorage.setItem("lol-loot-match-count", String(options.matchCount));
  // 渲染故障会被 renderOverviewBody 兜住并写进 console.error，这里一并收集。
  w.console.error = (...args) => { errors.push(args.map((value) => (value && value.stack) || String(value)).join(" ")); };

  for (const file of SCRIPTS) {
    let source = fs.readFileSync(path.join(WEB, file), "utf8");
    if (file === "suite.js" && process.env.R137_SUITE_SOURCE) source = fs.readFileSync(process.env.R137_SUITE_SOURCE, "utf8");
    if (file === "suite.js" && options.suiteSourceTransform) source = options.suiteSourceTransform(source);
    if (file === "champions.js" && options.championsSourceTransform) source = options.championsSourceTransform(source);
    if (file === "gameplay.js" && options.gameplaySourceTransform) source = options.gameplaySourceTransform(source);
    if (file === "app.js" && options.championRankings) {
      const originalFetch = w.fetch;
      w.fetch = (url, ...args) => String(url).startsWith("/api/champions/rankings")
        ? Promise.resolve(new w.Response(JSON.stringify({ rows: options.championRankings }))) : originalFetch(url, ...args);
    }
    w.eval(source);
    if (file === "demo-data.js" && options.friendsPayload) {
      const demoFetch = w.fetch;
      w.fetch = (input, init) => {
        const url = typeof input === "string" ? input : input?.url || "";
        options.onRequest?.(url);
        return url.startsWith("/api/social/friends")
          ? Promise.resolve(new w.Response(JSON.stringify(options.friendsPayload()))) : demoFetch(input, init);
      };
    }
    if (file === "demo-data.js" && options.matchCount > 17) {
      const demoFetch = w.fetch;
      w.fetch = async (url, ...args) => {
        const response = await demoFetch(url, ...args);
        if (!String(url).startsWith('/api/gameplay/overview')) return response;
        const payload = await response.json(), original = payload.matches;
        payload.matches = Array.from({length:options.matchCount}, (_,index) => ({...JSON.parse(JSON.stringify(original[index % original.length])),gameId:1000000+index}));
        payload.pagination = {begIndex:0,count:options.matchCount,hasMore:false};
        return new w.Response(JSON.stringify(payload), {status:200});
      };
    }
	if (file === "demo-data.js" && options.facadeStateTransform) {
	  const demoFetch = w.fetch;
	  w.fetch = async (input, init) => {
		const url = typeof input === "string" ? input : input?.url || "";
		if (!url.startsWith("/api/facade/state") || String(init?.method || "GET").toUpperCase() !== "GET") return demoFetch(input, init);
		const response = await demoFetch(input, init);
		const payload = options.facadeStateTransform(await response.json());
		return new w.Response(JSON.stringify(payload), { status: response.status, headers: { "Content-Type": "application/json" } });
	  };
	}
  }
  w.document.dispatchEvent(new w.Event("DOMContentLoaded", { bubbles: true }));
  return { window: w, errors, eventSources };
}

const settled = () => new Promise((resolve) => setTimeout(resolve, 1500));

test("R86 DOM teardown disconnects pending observers without disabling live callbacks", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  let callbacks = 0;
  let disposals = 0;
  w.addEventListener("deep-legends:dispose", () => { disposals += 1; });
  const target = w.document.createElement("div");
  new w.MutationObserver(() => { callbacks += 1; }).observe(target, { childList: true });
  target.append(w.document.createElement("span"));
  await Promise.resolve();
  assert.equal(callbacks, 1, "the fixture must not suppress live observer callbacks");
  target.append(w.document.createElement("span"));
  w.close();
  w.close();
  await Promise.resolve();
  assert.equal(callbacks, 1, "queued callbacks cannot outlive their jsdom window");
  assert.equal(disposals, 1, "explicit close and afterEach cleanup must be idempotent");
  assert.deepEqual(errors, []);
});

async function visitTool(w, name) {
  w.document.querySelector(`[data-suite-tab="${name}"]`).click();
  const root = w.document.getElementById(`suite-${name}-root`);
  const deadline = Date.now() + 2500;
  while (root.classList.contains("suite-loading") && Date.now() < deadline) await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(root.classList.contains("suite-loading"), false, `${name} did not load`);
  await new Promise((resolve) => setTimeout(resolve, 40));
}


test("1110 征召默认值、时间输入、模式能力和总开关真实保存链", async () => {
  const { window: w, errors } = bootDemoApp();
  try {
    await settled();
    w.document.querySelector('[data-section="suite"]').click();
    await settled();
    await visitTool(w, "champselect");
    const root = w.document.querySelector("#suite-champselect-root");
    const change = async (selector, value) => {
      const input = root.querySelector(selector);
      assert.ok(input, selector);
      if (input.type === "checkbox") input.checked = value;
      else input.value = value;
      input.dispatchEvent(new w.Event("change", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 60));
    };
    const saved = async () => (await (await w.fetch("/api/watch/rules")).json()).champSelect;
    root.querySelector('[data-cs-group="aram"]').click();
    assert.equal(root.querySelector(".is-ban"), null, "大乱斗不能显示禁用卡");
    assert.equal(root.querySelector("[data-cs-bench-enabled]").checked, true);
    assert.equal(root.querySelector("[data-cs-bench-prefer]").checked, true);
    assert.equal(root.querySelector('[data-cs-time="hold"]').value, "1");
    root.querySelector('[data-cs-hold-delta="500"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    assert.equal((await saved()).groups.aram.bench.holdMs, 1500);
    root.querySelector('[data-cs-hold-delta="-500"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    assert.equal((await saved()).groups.aram.bench.holdMs, 1000);
    await change('[data-cs-time="hold"]', "1.75");
    assert.equal((await saved()).groups.aram.bench.holdMs, 1750, "手输不强制对齐步长");
    assert.equal(root.querySelector('[data-cs-time="hold"]').checkValidity(), true, "手输小数不能被原生 step 判无效");
    await change('[data-cs-time="hold"]', "0.1");
    assert.equal((await saved()).groups.aram.bench.holdMs, 1000);
    assert.equal(root.querySelector('[data-cs-time="lock"]').value, "10");
    await change('[data-cs-time="lock"]', "0.25");
    assert.equal((await saved()).groups.aram.pick.lockDelayMs, 250);
    assert.equal((await saved()).groups.aram.pick.delayMs, 500, "锁定等待不得改写亮出前延时");
    await change('[data-cs-time="lock"]', "");
    assert.equal((await saved()).groups.aram.pick.lockDelayMs, 250, "空值不得改写配置");
    root.querySelector('[data-cs-delay-key="lockDelayMs"][data-cs-delay-delta="500"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    assert.equal((await saved()).groups.aram.pick.lockDelayMs, 750, "步进按钮保存独立锁定等待");
    for (const id of ["ranked", "normal", "practice", "event"]) {
      root.querySelector(`[data-cs-group="${id}"]`).click();
      assert.equal(root.querySelector("[data-cs-bench-enabled]"), null, id);
      assert.equal(root.querySelector("[data-cs-bench-prefer]").matches(":disabled"), false, id);
    }
    await change("[data-cs-bench-prefer]", true);
    assert.equal((await saved()).groups.event.bench.preferFirst, true);
    assert.equal(root.querySelector('[data-cs-time="ban"]'), null, "立即锁定不显示等待框");
    root.querySelector('[data-cs-strategy="ban"][data-cs-strategy-value="show-then-lock"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    await change('.cs-seq-card.is-ban [data-cs-time="lock"]', "1.25");
    assert.equal((await saved()).groups.event.ban.lockDelayMs, 1250);
    await change('.cs-seq-card.is-ban [data-cs-time="lock"]', "99");
    assert.equal((await saved()).groups.event.ban.lockDelayMs, 10000);
    root.querySelector('[data-cs-group="arena"]').click();
    assert.equal(root.querySelector(".cs-bench-card"), null, "无交换能力不显示空卡");
    root.querySelector('[data-cs-group="aram"]').click();
    const expectedGroups = JSON.parse(JSON.stringify((await saved()).groups));
    for (const config of Object.values(expectedGroups)) {
      config.ban.enabled = false;
      config.pick.enabled = false;
    }
    await change("[data-cs-master]", false);
    assert.ok(root.classList.contains("cs-master-off"));
    assert.ok(root.querySelector(".cs-config-fields").disabled);
    for (const element of root.querySelectorAll(".cs-config-fields input, .cs-config-fields button")) {
      assert.ok(element.matches(":disabled"), element.outerHTML);
    }
    assert.equal(root.querySelector("[data-cs-master]").disabled, false);
    assert.equal(JSON.stringify((await saved()).groups), JSON.stringify(expectedGroups), "关闭总开关同步关闭所有禁用/选用，保留英雄序列和其它配置");
    assert.equal(root.querySelector('[data-cs-side-enabled="pick"]').checked, false);
    await change("[data-cs-master]", true);
    const definitions = await (await w.fetch("/api/champselect/groups")).json();
    for (const definition of definitions) {
      expectedGroups[definition.groupId].ban.enabled = definition.hasBan;
      expectedGroups[definition.groupId].pick.enabled = true;
    }
    assert.equal(JSON.stringify((await saved()).groups), JSON.stringify(expectedGroups), "开启总开关同步开启所有可用禁用/选用，大乱斗保持无禁用");
    assert.equal(root.querySelector('[data-cs-side-enabled="pick"]').checked, true);
    assert.equal(root.querySelector("[data-cs-bench-enabled]").matches(":disabled"), false);
    assert.equal(root.querySelector("[data-cs-bench-enabled]").checked, true);
    root.querySelector('[data-cs-group="normal"]').click();
    assert.equal(root.querySelector('[data-cs-side-enabled="ban"]').checked, true);
    assert.equal(root.querySelector('[data-cs-side-enabled="pick"]').checked, true);
    await change('[data-cs-side-enabled="pick"]', false);
    assert.equal((await saved()).groups.normal.pick.enabled, false, "总开关开启后仍可分别调整子开关");
    assert.equal((await saved()).enabled, true);
    assert.deepEqual(errors, []);
  } finally { w.close(); }
});

async function bootLiveTab() {
  const boot = bootDemoApp();
  await settled();
  boot.window.document.querySelector('[data-section="live"]').click();
  await settled();
  return boot;
}

test("平均段位只查询视口内卡片，并在分页加载期间停用", async () => {
  const dom = new JSDOM('<main id="app-scroll"><div id="matches"></div></main>', { url: "http://localhost/", pretendToBeVisual: true });
  const w = dom.window;
  const root = w.document.getElementById("app-scroll");
  const container = w.document.getElementById("matches");
  container.dataset.matchTierScope = "scope";
  root.getBoundingClientRect = () => ({ top: 0, left: 0, right: 300, bottom: 100, width: 300, height: 100 });
  const matches = [];
  for (let index = 0; index < 40; index += 1) {
    const article = w.document.createElement("article");
    article.className = "match-entry";
    const node = w.document.createElement("span");
    node.dataset.matchTierPending = "";
    node.dataset.gameId = String(index + 1);
    article.append(node);
    container.append(article);
    const top = index < 5 ? index * 18 : 200 + index * 18;
    article.getBoundingClientRect = () => ({ top, left: 0, right: 280, bottom: top + 16, width: 280, height: 16 });
    matches.push({ gameId: index + 1, createdAt: index + 1, duration: 1800 });
  }
  const requests = [];
  const state = { activeTab: "current", matchTiers: new Map(), matchTierFlights: new Set() };
  const { hydrateMatchTiers } = compileFunctions(gameplaySource, [
    "matchTierScrollRoot", "matchTierNodeIsVisible", "hydrateMatchTiers", "shouldHydrateMatchTiers", "scheduleMatchTierRetry",
  ], {
    window: w,
    document: w.document,
    state,
    riotTab: () => true,
    connected: () => true,
    matchTierCacheKey: (_tab, gameID) => String(gameID),
    applyMatchTierValue: () => {},
    tabServerID: () => "",
    matchTierFromScores: () => null,
    MATCH_TIERS_MAX_REFS: 24,
    MATCH_TIER_KR_COALESCE_MS: 0,
    MATCH_TIERS_PARALLEL_BATCHES: 2,
    api: async (_path, options) => {
      requests.push(JSON.parse(options.body));
      return {};
    },
  });
  const tab = { key: "current", data: { player: { playerRef: "player" }, matches } };
  await hydrateMatchTiers(container, tab, "scope");
  assert.equal(requests.length, 1);
  assert.deepEqual(requests[0].matches.map((match) => match.gameId), [1, 2, 3, 4, 5]);

  requests.length = 0;
  state.matchTiers.clear();
  tab.loadingMore = true;
  await hydrateMatchTiers(container, tab, "scope");
  assert.equal(requests.length, 0, "加载更多期间不得查询平均段位");

  tab.loadingMore = false;
  tab.filterPaging = true;
  await hydrateMatchTiers(container, tab, "scope");
  assert.equal(requests.length, 0, "客户端筛选自动翻页期间不得查询平均段位");
  w.close();
});

test("match-tier failures back off instead of retrying on the next render", async () => {
  const dom = new JSDOM('<main id="app-scroll"><div id="matches"><article class="match-entry"><span data-match-tier-pending data-game-id="1"></span></article></div></main>', { url: "http://localhost/", pretendToBeVisual: true });
  const w = dom.window;
  const container = w.document.getElementById("matches");
  const node = container.querySelector("[data-match-tier-pending]");
  node.closest(".match-entry").getBoundingClientRect = () => ({ top: 0, left: 0, right: 10, bottom: 10, width: 10, height: 10 });
  w.document.getElementById("app-scroll").getBoundingClientRect = () => ({ top: 0, left: 0, right: 100, bottom: 100, width: 100, height: 100 });
  const state = { activeTab: "current", matchTiers: new Map(), matchTierFlights: new Set(), matchTierFailures: new Map() };
  let requests = 0;
  const functions = compileFunctions(gameplaySource, ["matchTierScrollRoot", "matchTierNodeIsVisible", "noteMatchTierFailure", "hydrateMatchTiers", "shouldHydrateMatchTiers"], {
    window: w, document: w.document, state,
    riotTab: () => false, connected: () => true,
    matchTierCacheKey: () => "scope:1", applyMatchTierValue: () => {}, tabServerID: () => "HN1",
    matchTierFromScores: () => null, MATCH_TIERS_MAX_REFS: 24, MATCH_TIER_KR_COALESCE_MS: 0, MATCH_TIERS_PARALLEL_BATCHES: 2,
    MATCH_TIER_RETRY_BASE_MS: 1000, MATCH_TIER_MAX_BACKOFF_MS: 60000,
    api: async () => { requests += 1; throw new Error("timeout"); },
    // 一批失败后现在会安排重试（R127 复审第 5 条），harness 里不需要真的排程。
    scheduleMatchTierRetry: () => {},
    // 整批诊断现在失败也会上报（reason=failed），harness 里不需要真的发请求。
    recordMatchTierOverviewBatch: () => {},
  });
  const tab = { key: "current", data: { player: { playerRef: "player" }, matches: [{ gameId: 1, participants: [{ playerRef: "ref" }] }] } };
  await functions.hydrateMatchTiers(container, tab, "scope", [node]);
  await functions.hydrateMatchTiers(container, tab, "scope", [node]);
  assert.equal(requests, 1);
  assert.equal(state.matchTierFailures.get("scope:1")?.count, 1);
  w.close();
});

test("match-tier overview diagnostics contain aggregate counts only", async () => {
  const requests = [];
  const { recordMatchTierOverviewBatch } = compileFunctions(gameplaySource, ["recordMatchTierOverviewBatch"], {
    fetch: async (requestPath, options) => { requests.push([requestPath, JSON.parse(options.body)]); return { ok: true }; },
  });
  recordMatchTierOverviewBatch(36, 24, 19, 2431);
  await new Promise((resolve) => setImmediate(resolve));
  assert.deepEqual(requests, [["/api/diagnostics/client", {
    event: "match_tiers_overview_batch", reason: "complete", totalRefs: 36, uniqueRefs: 24, cacheHits: 19,
    // R127 P0-3：整批耗时必须上报，否则日志里看不出平均段位到底花了多久。
    durationMs: 2431,
  }]]);
  assert.doesNotMatch(JSON.stringify(requests), /playerRef|puuid|summoner/i);
});

test("non-match overview rerenders preserve the match-list node", () => {
  const dom = new JSDOM('<div id="overview"></div>', { url: "http://localhost/", pretendToBeVisual: true });
  const container = dom.window.document.getElementById("overview");
  const state = { tabs: [{ key: "current" }], settings: { maskNames: false } };
  let preparedStrip;
  const dependencies = {
    state,
    matchTierScope: () => "scope", filteredMatches: (matches) => matches,
    maskedProfileIcon: () => "", iconFigure: () => "", playerLabel: () => "Player", riotTab: () => false,
    emptyState: () => "", opggSummonerURL: () => "", loadOverview: () => {},
    matchListEmptyContent: () => "empty", matchSentinelShouldHide: () => true,
    summonerContextChip: () => "", summonerProChip: () => "", summonerRegionChip: () => "", renderSummonerHighlights: () => "",
    scheduleOverviewCurrentGame: () => {}, updateFriendPresenceChips: () => {},
    paginationCopyFor: () => "", renderCareerSections: () => "", renderMatchFilters: () => "",
    renderMatch: (match) => `<article data-match-id="${match.gameId}" class="match-entry">${match.gameId}</article>`, number: (value) => String(value),
    escapeHTML: (value) => String(value ?? ""), bindOverviewContent: () => {}, applyRenderedMetricStyles: () => {},
    prepareImages: root => { preparedStrip = root.querySelector(".summoner-strip"); }, ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, observeMatchTierVisibility: () => {},
    window: dom.window, document:dom.window.document,
  };
  const { renderOverviewBodyContent } = compileFunctions(gameplaySource, ["renderOverviewBodyContent", "reconcileFilteredMatchList"], dependencies);
  const matches = [{ gameId: 1 }];
  const tab = { key: "current", matchFilter: "all", matchViewRevision: 0, openMatches: new Set(), data: { player: { playerRef: "ref", backgroundSource: "gtimg", backgroundPath: "/skin.jpg" }, matches, pagination: {} } };
  renderOverviewBodyContent(container, tab);
  const firstList = container.querySelector(".match-list");
  renderOverviewBodyContent(container, tab);
  assert.equal(container.querySelector(".match-list"), firstList);

  const firstEntry = firstList.querySelector(".match-entry");
  const firstStrip = container.querySelector(".summoner-strip");
  tab.data.matches = [{ gameId: 1 }, { gameId: 2 }];
  tab.data.historicalRanks = [{ season: "S2025", tier: "MASTER" }];
  renderOverviewBodyContent(container, tab);
  assert.equal(container.querySelector(".match-list"), firstList);
  assert.equal(container.querySelector(".match-entry"), firstEntry, "streamed arrays must not recreate already loaded cards");
  assert.equal(container.querySelector(".summoner-strip"), firstStrip, "unchanged profile must retain image nodes");
  assert.equal(preparedStrip, firstStrip, "image initialization must see the final retained strip");
  assert.equal(firstList.querySelectorAll(".match-entry").length, 2);

  const artwork = firstStrip.querySelector(".summoner-strip-art");
  tab.data.player.summonerLevel = 300;
  renderOverviewBodyContent(container, tab);
  assert.notEqual(container.querySelector(".summoner-strip"), firstStrip);
  assert.equal(container.querySelector(".summoner-strip-art"), artwork, "profile metadata must not restart the same artwork");

  tab.openMatches.add("1");
  tab.matchViewRevision += 1;
  renderOverviewBodyContent(container, tab);
  assert.notEqual(container.querySelector(".match-list"), firstList, "match UI state changes must rebuild the list");

  const mutated = gameplaySource.replace("container._matchListViewRevision === Number(tab.matchViewRevision || 0)", "true");
  assert.notEqual(mutated, gameplaySource);
  const mutatedRender = compileFunctions(mutated, ["renderOverviewBodyContent", "reconcileFilteredMatchList"], dependencies).renderOverviewBodyContent;
  const mutatedContainer = dom.window.document.createElement("div");
  tab.openMatches.clear();
  tab.matchViewRevision = 0;
  mutatedRender(mutatedContainer, tab);
  const mutatedFirst = mutatedContainer.querySelector(".match-list");
  mutatedRender(mutatedContainer, tab);
  assert.equal(mutatedContainer.querySelector(".match-list"), mutatedFirst);
  tab.openMatches.add("1");
  tab.matchViewRevision += 1;
  mutatedRender(mutatedContainer, tab);
  assert.equal(mutatedContainer.querySelector(".match-list"), mutatedFirst, "the mutation must reproduce the stale-list bug");
  dom.window.close();
});

test("总览战绩卡：点击展开按钮只替换目标卡片并可收起", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const d = w.document;
  const list = d.querySelector(".match-list");
  const entries = [...d.querySelectorAll(".match-list .match-entry")];
  const button = entries[0]?.querySelector("[data-toggle-match]:not([disabled])");
  const untouched = entries.find((entry) => entry !== button?.closest(".match-entry"));
  assert.ok(list, "找不到战绩列表");
  assert.ok(button, "找不到可用的展开按钮");
  assert.ok(untouched, "找不到用于验证局部替换的其它战绩卡");
  const gameID = button.dataset.toggleMatch;
  const originalUntouched = untouched;
  button.click();
  await settled();
  assert.equal(d.querySelector(".match-list"), list, "展开不应重建整个战绩列表");
  const expanded = d.querySelector(`[data-match-id="${gameID}"]`);
  assert.ok(expanded?.querySelector(".match-detail"), "点击展开后目标卡片没有详情区");
  const expandedButton = expanded.querySelector(`[data-toggle-match="${gameID}"]`);
  assert.equal(expandedButton?.getAttribute("aria-expanded"), "true");
  assert.equal(expandedButton?.getAttribute("aria-controls"), `match-detail-${gameID}`);
  assert.equal(d.querySelector(`#match-detail-${gameID}`)?.closest(".match-entry"), expanded, "详情必须归属于目标战绩卡");
  assert.equal(d.querySelector(`[data-match-id]:not([data-match-id="${gameID}"])`), originalUntouched, "非目标战绩卡不应被重建");

  expandedButton.click();
  await settled();
  assert.equal(d.querySelector(".match-list"), list, "收起不应重建整个战绩列表");
  const collapsed = d.querySelector(`[data-match-id="${gameID}"]`);
  assert.equal(collapsed.querySelector(".match-detail"), null, "再次点击没有收起");
  assert.equal(collapsed.querySelector(`[data-toggle-match="${gameID}"]`)?.getAttribute("aria-expanded"), "false");
  assert.deepEqual(errors, []);
  w.close();

  const large = bootDemoApp({ matchCount: 200 });
  await settled();
  const largeDocument = large.window.document;
  const largeList = largeDocument.querySelector(".match-list");
  const largeEntries = [...largeDocument.querySelectorAll(".match-list .match-entry")];
  const largeButton = largeEntries[0]?.querySelector("[data-toggle-match]:not([disabled])");
  const largeUntouched = largeEntries.find((entry) => entry !== largeButton?.closest(".match-entry"));
  assert.ok(largeList && largeButton && largeUntouched, "200 场性能护栏找不到战绩目标");
  const createElement = largeDocument.createElement.bind(largeDocument);
  let createElementCalls = 0;
  largeDocument.createElement = (...args) => {
    createElementCalls += 1;
    return createElement(...args);
  };
  largeButton.click();
  await settled();
  largeDocument.createElement = createElement;
  assert.ok(createElementCalls < 500, `单次展开不应创建 ${createElementCalls} 个 DOM 元素`);
  assert.equal(largeDocument.querySelector(".match-list"), largeList, "200 场展开不应重建列表");
  assert.equal(largeDocument.querySelector(`[data-match-id]:not([data-match-id="${largeButton.dataset.toggleMatch}"])`), largeUntouched, "200 场展开不应重建非目标卡片");
  assert.deepEqual(large.errors, []);
  large.window.close();

  const mutated = bootDemoApp({
    gameplaySourceTransform: (source) => source.replace(
      "entry.replaceWith(replacement);",
      "rerender();",
    ),
  });
  await settled();
  const mutatedList = mutated.window.document.querySelector(".match-list");
  const mutatedButton = mutated.window.document.querySelector(".match-list .match-entry [data-toggle-match]:not([disabled])");
  assert.ok(mutatedList && mutatedButton, "变异副本找不到局部展开测试目标");
  mutatedButton.click();
  await settled();
  assert.equal(mutated.window.document.querySelector(".match-list"), mutatedList, "变异必须被局部替换回归测试捕获");
  mutated.window.close();
});

test("玩家覆盖层里的战绩详情也能展开", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  w.dispatchEvent(new w.CustomEvent("deep-legends:open-player", {
    detail: { source: "champions", playerRef: "player_00000000000000000000000000000001", gameName: "覆盖层玩家" },
  }));
  await settled();
  const overlay = w.document.getElementById("player-overlay");
  assert.equal(overlay.hidden, false, "玩家覆盖层没有打开");
  const button = overlay.querySelector(".match-list [data-toggle-match]:not([disabled])");
  assert.ok(button, "覆盖层里找不到可展开的战绩");
  button.click();
  await settled();
  assert.equal(overlay.querySelectorAll(".match-detail").length, 1, "覆盖层战绩详情没有展开");
  assert.deepEqual(errors, []);
  w.close();
});

test("演示数据下总览页渲染出真实内容，且渲染期没有异常", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  assert.ok(overview, "缺少 #overview-content 容器");
  assert.deepEqual(errors, [], `渲染期出现异常：\n${errors.join("\n")}`);
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "总览停在骨架屏上——渲染路径抛异常了");
  assert.ok(overview.querySelector(".summoner-strip"), "总览缺少召唤师信息条");
  assert.ok(overview.querySelector(".career-column .career-section"), "总览缺少生涯统计分区");
  assert.ok(overview.querySelector(".matches-column .match-list"), "总览缺少对局列表");
	// 三块近期表现都只吃首屏最近 20 场，不再显示或依赖后台赛季扫描进度。
	const recentSection = overview.querySelector(".recent-ranked-section");
	const abilitySection = overview.querySelector(".ability-section");
	const positionSection = overview.querySelector(".position-list")?.closest(".career-section");
	for (const [label, section] of [["近 20 场排位", recentSection], ["能力表现", abilitySection], ["位置偏好", positionSection]]) {
		assert.ok(section, `${label} 缺少分区`);
		const header = section.querySelector(":scope > header");
		assert.ok(header, `${label} 缺少标题栏`);
		assert.equal(header.querySelectorAll(".season-progress-badge").length, 0, `${label} 仍依赖赛季统计进度`);
		const buttons = [...header.querySelectorAll(".ranked-queue-button")];
		assert.deepEqual(buttons.map((button) => button.textContent), ["单双排", "灵活组排"], `${label} 的队列页签被删掉了`);
		assert.deepEqual(buttons.map((button) => button.classList.contains("is-active")), [true, false], `${label} 的默认选中态不对`);
	}
	assert.equal(recentSection.querySelector("h3")?.textContent, "近 20 场排位");
	assert.match(abilitySection.querySelector(".ability-meta > span")?.textContent || "", /14 场样本/);
	assert.equal(positionSection.querySelector(".career-sample-label")?.textContent, "近 20 场");
	assert.doesNotMatch(overview.querySelector(".recent-ranked-section > header")?.textContent || "", /实际\s*20\s*场/);
	// 「过去 30 天排位」整块已删除（与近 20 场排位吃的是同一批 20 场详情战绩）。
	assert.doesNotMatch(overview.textContent || "", /过去 30 天排位/);
	const positionRows = [...overview.querySelectorAll(".position-list .position-row")];
	assert.deepEqual(positionRows.map((row) => row.querySelector("strong")?.textContent), ["上单", "打野", "中单", "下路", "辅助"]);
	assert.deepEqual(positionRows.map((row) => row.querySelector(":scope > b")?.textContent), ["46%", "10%", "28%", "16%", "0%"]);
	assert.doesNotMatch(overview.querySelector(".position-list")?.textContent || "", /其他/);
  w.close();
});

test("好友真实数据恢复横条模式与计时，SSE 更新不重建战绩或串到其他玩家", async () => {
  const now = Date.now(), requests = [];
  let friend = {playerRef:"player_friend_123456789",gameName:"好友",tagLine:"1234",icon:1,groupId:1,
    availability:"dnd",product:"league_of_legends",gameStatus:"inGame",queueLabel:"排位赛 灵活排位",championName:"虚空掠夺者",gameStartedAt:now-852000};
  const { window: w, errors } = bootDemoApp({friendsPayload:()=>({groups:[{id:1,name:"默认分组"}],friends:[friend]}),onRequest:url=>requests.push(url)});
  w.Date.now = () => now;
  const waitFor = async predicate => {
    const deadline=Date.now()+10000;
    while(!predicate() && Date.now()<deadline) await new Promise(resolve=>setTimeout(resolve,20));
    assert.ok(predicate(),"expected friend UI state did not arrive");
  };
  try {
    await settled();
    w.document.getElementById("friends-toggle").click();
    await waitFor(()=>w.document.querySelector('[data-player-ref="player_friend_123456789"]'));
    w.document.querySelector('[data-player-ref="player_friend_123456789"]').click();
    const overview=w.document.getElementById("overview-content");
    await waitFor(()=>overview.querySelector('[data-friend-presence]:not([hidden])'));
    const strip=overview.querySelector(".summoner-strip"), list=overview.querySelector(".match-list"), chip=strip.querySelector("[data-friend-presence]");
    assert.equal(chip.textContent,"排位赛 灵活排位 · 虚空掠夺者 · 已进行 14:12");
    assert.equal(w.document.getElementById("friends-dock").classList.contains("is-open"),false);
    const historyRequests=()=>requests.filter(url=>url.startsWith('/api/gameplay/overview')).length;
    const reads=historyRequests();
    w.document.getElementById("app-scroll").scrollTop=330;
    w.Date.now=()=>now+5000;
    await waitFor(()=>chip.textContent.endsWith("14:17"));
    const refresh=async changes=>{
      friend={...friend,...changes};
      const before=requests.filter(url=>url.startsWith('/api/social/friends')).length;
      w.dispatchEvent(new w.CustomEvent("deep-legends:friends-updated"));
      await waitFor(()=>requests.filter(url=>url.startsWith('/api/social/friends')).length>before);
      await new Promise(resolve=>setTimeout(resolve,20));
    };
    await refresh({queueLabel:"海克斯大乱斗"});
    assert.match(chip.textContent,/海克斯大乱斗.*14:17/);
    assert.equal(overview.querySelector(".summoner-strip"),strip);
    assert.equal(overview.querySelector(".match-list"),list);
    assert.equal(w.document.getElementById("app-scroll").scrollTop,330);
    assert.equal(historyRequests(),reads,"presence must not reload history");
    assert.equal(requests.some(url=>url.startsWith('/api/gameplay/current-game')),false,"CN friend status needs no spectator probe");
    await refresh({gameStatus:"outOfGame"});assert.equal(chip.hidden,true);assert.equal(chip.querySelector("time"),null);
    await refresh({gameStatus:"inGame",availability:"offline"});assert.equal(chip.hidden,true);
    await refresh({availability:"dnd",playerRef:"player_different_server"});assert.equal(chip.hidden,true,"same display name with another reference must not match");
    await refresh({playerRef:"player_friend_123456789"});assert.equal(chip.hidden,false);
    w.dispatchEvent(new w.CustomEvent("deep-legends:friends-presence",{detail:{friends:[]}}));assert.equal(chip.hidden,true,"empty/disconnected snapshot clears old game");
    assert.deepEqual(errors,[]);
  } finally {w.close();}
});

test("演示数据下工具五个页签都渲染完成", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  w.document.querySelector('[data-section="suite"]').click();
  await settled();
  for (const name of ["watch", "rig", "facade", "sweep", "champselect"]) {
    await visitTool(w, name);
    const root = w.document.getElementById(`suite-${name}-root`);
    assert.ok(root, `工具缺少 ${name} 容器`);
    assert.equal(root.classList.contains("suite-loading"), false, `${name} 永久停在加载态`);
    assert.ok(root.textContent.trim().length > 20, `${name} 没有渲染出内容`);
  }
  const watchCards = [...w.document.querySelectorAll("#suite-watch-root [data-watch-card]")];
  assert.deepEqual(watchCards.map((card) => card.dataset.watchCard), ["accept", "promote-leader", "invitations", "auto-matchmaking", "reconnect", "position-broadcast", "play-again", "auto-honor", "skip-celebration"], "自动页九张规则卡未按分类完整渲染");
  assert.equal(w.document.querySelectorAll("#suite-watch-root [data-watch-toggle]").length, 9, "自动规则卡缺少独立开关");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .watch-rule.is-enabled").length, 6, "已启用规则卡缺少独立高亮背景");
  assert.equal(Number.parseFloat(w.document.querySelector('[data-watch-delay="autoAccept"]')?.style.getPropertyValue("--range-fill")), 15, "自动接受延时轨道未按当前值填充");
  assert.ok(w.document.querySelector("#suite-watch-root .watch-rules-head"), "自动页缺少设计稿中的规则标题区");
  assert.equal(w.document.querySelectorAll('#suite-watch-root [data-watch-choice="autoHonor.strategy"]').length, 4, "点赞策略未按设计稿渲染为四个胶囊选项");
  assert.equal(w.document.querySelectorAll('#suite-watch-root [data-watch-choice="positionBroadcast.visibility"]').length, 2, "阵营播报范围未按设计稿渲染为两个胶囊选项");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .watch-invite-summary > .watch-pill").length, 3, "邀请处理未保持三项紧凑摘要");
  const hiddenAcceptedPolicy = w.document.querySelector('.watch-invite-more [data-watch-policy-cycle="1700"]');
  assert.ok(hiddenAcceptedPolicy, "邀请详情缺少斗魂竞技场策略");
  hiddenAcceptedPolicy.click();
  await settled();
  assert.equal(w.document.querySelector('.watch-invite-summary > [data-watch-policy-cycle="1700"]')?.textContent, "斗魂竞技场 · 接受", "接受策略保存后没有移到卡片外层展示");
  assert.match(w.document.querySelector('[data-watch-card="auto-matchmaking"]')?.textContent || "", /最少人数[\s\S]*延时/, "自动匹配卡缺少设计稿参数");
  assert.ok(w.document.querySelector("#suite-rig-root .rig-layout"), "维护页未渲染");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-preview"), "生涯预览未渲染");
  assert.ok(w.document.querySelector("#suite-sweep-root .claim-row"), "领奖清单未渲染");
  assert.equal(w.document.querySelector("#suite-sweep-root > .suite-note.is-info"), null, "领奖页仍显示重新扫描下方的说明条");
  assert.equal(w.document.querySelector("#suite-sweep-root .claim-sub"), null, "领奖标题下仍显示描述文字");
  assert.deepEqual([...w.document.querySelectorAll(".suite-tab")].map((node) => node.dataset.suiteTab), ["watch", "rig", "champselect", "facade", "sweep"], "征召应紧跟维护");
  const suiteIcons = [...w.document.querySelectorAll(".suite-tab-icon svg")];
  assert.equal(suiteIcons.length, 5, "五个工具图标应统一使用 SVG");
  for (const icon of suiteIcons) {
    assert.equal(icon.getAttribute("viewBox"), "0 0 24 24");
    assert.equal(icon.getAttribute("width"), "26");
    assert.equal(icon.getAttribute("height"), "26");
    assert.equal(icon.getAttribute("focusable"), "false");
    assert.equal(icon.parentElement.getAttribute("aria-hidden"), "true");
  }
	assert.equal(w.document.querySelectorAll("#suite-champselect-root .cs-mode-item").length, 6, "征召页缺少六个模式分组");
	assert.equal(w.document.querySelectorAll("#suite-champselect-root .cs-seq-card").length, 3, "征召页缺少禁用、选用和备战席三张序列卡");
	assert.equal(w.document.querySelectorAll("#suite-champselect-root .cs-rail-slot.is-empty").length, 3, "征召传送带未按模式上限渲染空槽");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .phase-node").length, 6, "自动页相位轨没有按设计稿收敛为六个主阶段");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-avatar-level"), "生涯预览缺少头像等级牌");
  assert.equal(w.document.querySelectorAll("#suite-facade-root .facade-slot").length, 3, "生涯预览缺少三个勋章位");
	assert.equal(w.document.querySelector("#suite-facade-root .facade-signature")?.textContent, "峡谷先锋", "生涯预览没有显示挑战头衔");
	assert.deepEqual([...w.document.querySelectorAll("#suite-facade-root [data-facade-challenge-slot]")].map((node) => node.textContent), ["不破不立", "峡谷收藏家", "团队之星"], "生涯预览没有渲染后端解析的三个勋章名称");
	assert.doesNotMatch(w.document.getElementById("suite-facade-root").textContent, /勋章 1|勋章 2|勋章 3/, "生涯预览仍在显示勋章占位编号");
	assert.ok(w.document.querySelector('[data-facade-availability="spectating"]'), "生涯页缺少观战中状态");
	assert.ok(w.document.querySelector('[data-facade-clear="clear-title"]'), "展示清理缺少卸下头衔动作");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-background-card"), "生涯背景控制卡未渲染在右栏");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-chat-card"), "生涯聊天身份控制卡未渲染在右栏");
  assert.equal(w.document.querySelectorAll("#suite-facade-root .facade-film").length, 1, "生涯背景应只保留一个皮肤网格");
  assert.equal(w.document.querySelectorAll("#suite-facade-root [data-facade-chroma]").length, 0, "生涯背景仍提供炫彩选择");
  assert.doesNotMatch(w.document.getElementById("suite-facade-root").textContent, /炫彩/, "生涯背景仍宣称可以设置炫彩");
  assert.doesNotMatch(w.document.getElementById("suite-watch-root").textContent, /这些我们不做/, "自动页仍显示已要求移除的说明模块");
  assert.doesNotMatch(w.document.getElementById("suite-rig-root").textContent, /不做的能力/, "维护页仍显示已要求移除的说明模块");
  assert.deepEqual(errors, [], `工具页渲染期出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("2351 自定义暂停事件保留真实总开关和卡片高亮", async () => {
  const { window: w, errors } = bootDemoApp();
  try {
    await settled();
    w.document.querySelector('[data-section="suite"]').click();
    await settled();
    const root = w.document.getElementById("suite-watch-root");
    const enabled = root.querySelectorAll(".watch-rule.is-enabled").length;
    assert.ok(enabled > 0);
    w.dispatchEvent(new w.CustomEvent("deep-legends:watch", { detail: "watch:session:paused_custom" }));
    assert.equal(root.querySelector("[data-watch-master]").checked, true);
    assert.equal(root.querySelectorAll(".watch-rule.is-enabled:not(.is-paused)").length, enabled);
    assert.match(root.textContent, /自定义对局已暂停/);
    assert.doesNotMatch(root.textContent, /总开关已关闭/);
    w.dispatchEvent(new w.CustomEvent("deep-legends:watch", { detail: "watch:session:resumed" }));
    assert.doesNotMatch(root.textContent, /自定义对局已暂停/);
    assert.equal(root.querySelector("[data-watch-master]").checked, true);
    await new Promise((resolve) => w.requestAnimationFrame(resolve));
    assert.deepEqual(errors, []);
  } finally { w.close(); }
});

test("R123 生涯头像与旗帜入口切到收藏页对应视图", async () => {
  const { window: w, errors } = bootDemoApp();
  try {
    await settled();
    for (const view of ["banners", "icons"]) {
      w.document.querySelector('[data-section="suite"]').click();
      await visitTool(w, "facade");
      const entry = w.document.querySelector(`#suite-facade-root [data-facade-browse="${view}"]`);
      assert.ok(entry, `${view} 缺少生涯页入口`);
      entry.click();
      assert.equal(w.document.querySelector('[data-section="favorites"]').getAttribute("aria-selected"), "true");
      assert.equal(w.document.querySelector('[data-favorites-page="facade-collection"]').getAttribute("aria-selected"), "true");
      assert.equal(w.document.getElementById("favorites-facade-panel").hidden, false);
      assert.equal(w.document.getElementById(`facade-view-${view}`).getAttribute("aria-selected"), "true");
    }
    assert.deepEqual(errors, []);
  } finally { w.close(); }
});

test("R56 工具页状态、确认、下拉与领奖契约完整", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
  await visitTool(w, "sweep");
  await settled();

  const facadeSelects = [...w.document.querySelectorAll("#suite-facade-root .suite-select")];
  assert.equal(facadeSelects.length, 4, "生涯页应保留英雄和三个段位下拉");
  for (const select of facadeSelects) {
    assert.ok(select.parentElement?.classList.contains("select-wrap"), "生涯下拉未包在 .select-wrap 中");
    assert.ok(select.parentElement.querySelector(":scope > .native-select-menu .app-select-menu"), "生涯下拉未增强为应用菜单");
  }

  const facadeText = w.document.getElementById("suite-facade-root").textContent;
  assert.match(facadeText, /只在你点击后执行/);
  assert.match(facadeText, /“登录时重设”两项例外/);
  assert.match(facadeText, /生涯背景[\s\S]*你生涯页顶部的那张大图/);
  assert.match(facadeText, /好友悬浮卡[\s\S]*别人点你头像时看到的在线状态、签名和段位/);
  assert.match(facadeText, /生涯页展示[\s\S]*头像框、挑战勋章、赛季旗帜、表情轮盘/);
	assert.match(facadeText, /卸下全部勋章[\s\S]*保留旗帜和当前头衔[\s\S]*无法保留头衔，本次操作会中止并提示/);
	assert.doesNotMatch(facadeText, /切换上赛季旗帜|挑战旗帜配色/);
  assert.ok(w.document.querySelector('#suite-facade-root [data-facade-browse="icons"]'), "R123 生涯头像应有收藏页入口");
  assert.ok(w.document.querySelector('#suite-facade-root [data-facade-browse="banners"]'), "R123 生涯旗帜应有收藏页入口");
	assert.doesNotMatch(facadeText, /头衔可能同时卸下/);
  assert.doesNotMatch(facadeText, /展示位/);
  assert.equal(w.document.querySelector("[data-facade-owned]").checked, false, "只显示已拥有不应默认开启");

  const choiceRow = [...w.document.querySelectorAll("#suite-sweep-root .claim-row")].find((row) => row.querySelectorAll(".suite-tile").length === 3 && row.querySelector("[data-claim-choice]"));
  assert.ok(choiceRow, "演示数据缺少三选一奖励");
  assert.match(choiceRow.textContent, /3 选 1/);
  assert.doesNotMatch(choiceRow.textContent, /1 选 1|1 - 1 选/);

  const master = w.document.querySelector("[data-watch-master]");
  master.checked = false;
  master.dispatchEvent(new w.Event("change", { bubbles: true }));
  await new Promise((resolve) => setTimeout(resolve, 80));
  assert.equal(w.document.getElementById("suite-watch-metric").textContent, "已暂停");
  assert.match(w.document.getElementById("suite-watch-root").textContent, /自动规则已暂停/);
  assert.ok(w.document.querySelector("#suite-watch-root .watch-rule.is-paused"));

  let nativeConfirmCalls = 0;
  w.confirm = () => { nativeConfirmCalls += 1; throw new Error("native confirm must not run"); };
  const originalFetch = w.fetch;
  const requests = [];
  w.fetch = (...args) => { requests.push(String(args[0])); return originalFetch(...args); };
  w.document.querySelector('[data-facade-clear="clear-emotes"]').click();
  await new Promise((resolve) => w.requestAnimationFrame(resolve));
  const confirmation = w.document.querySelector(".suite-confirm-card");
  assert.ok(confirmation, "清空表情轮盘未打开应用内确认卡");
  assert.equal(w.document.querySelector("[inert]"), null, "确认卡不应阻塞页面其它内容");
  // P3-7：确认卡与 toast 共用右下角锚点，卡片打开时 toast 必须被抬到卡片上方，
  // 否则 P2-4 的「上一个生涯写入尚未完成，请稍候」在屏幕上被完全遮住。
  assert.equal(w.document.body.dataset.suiteConfirmOpen, "true", "确认卡打开时必须标记 body 以抬起 toast");
  assert.match(w.document.documentElement.style.getPropertyValue("--suite-confirm-clearance"), /^\d+px$/, "确认卡打开时必须写入实测避让高度");
  const confirmCancelButton = confirmation.querySelector("[data-suite-confirm-cancel]");
  const confirmAcceptButton = confirmation.querySelector("[data-suite-confirm-accept]");
  const pressTab = (shiftKey) => {
    const event = new w.KeyboardEvent("keydown", { key: "Tab", shiftKey, bubbles: true, cancelable: true });
    w.document.dispatchEvent(event);
    return event;
  };
  confirmCancelButton.focus();
  assert.equal(pressTab(false).defaultPrevented, true, "确认卡打开时 Tab 必须被焦点陷阱接管");
  assert.equal(w.document.activeElement, confirmAcceptButton, "Tab 必须移到卡片内的确认按钮，不能跑进背景");
  pressTab(false);
  assert.equal(w.document.activeElement, confirmCancelButton, "Tab 到末尾必须循环回第一个按钮");
  pressTab(true);
  assert.equal(w.document.activeElement, confirmAcceptButton, "Shift+Tab 必须反向循环");
  assert.ok(confirmation.contains(w.document.activeElement), "焦点必须始终留在确认卡内");
  confirmation.querySelector("[data-suite-confirm-cancel]").click();
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(w.document.body.dataset.suiteConfirmOpen, undefined, "确认卡关闭后必须撤销 toast 避让");
  assert.equal(w.document.documentElement.style.getPropertyValue("--suite-confirm-clearance"), "", "确认卡关闭后必须清掉避让高度");
  assert.equal(nativeConfirmCalls, 0);
	assert.equal(requests.filter((request) => request === "/api/facade/apply").length, 0, "取消确认后不应发出生涯写请求");
	w.document.querySelector('[data-facade-clear="clear-title"]').click();
	await new Promise((resolve) => w.requestAnimationFrame(resolve));
	const titleConfirmation = w.document.querySelector(".suite-confirm-card");
	assert.ok(titleConfirmation, "卸下头衔未打开应用内确认卡");
	titleConfirmation.querySelector("[data-suite-confirm-cancel]").click();
	await new Promise((resolve) => setTimeout(resolve, 20));
	assert.equal(requests.filter((request) => request === "/api/facade/apply").length, 0, "取消卸下头衔后不应发出生涯写请求");

  const clearObjectives = w.document.querySelector('[data-facade-clear="clear-objectives"]');
  assert.equal(clearObjectives.closest(".facade-action").querySelector("p").textContent, "把当前未读任务与活动系列标记为已读，不领取奖励、不更改任务进度。");
  assert.equal(w.document.querySelector("[data-objective-diagnostics]"), null);
  const facadeWrites = [];
  w.fetch = (...args) => {
    if (String(args[0]) === "/api/facade/apply") facadeWrites.push(JSON.parse(args[1].body));
    requests.push(String(args[0]));
    return originalFetch(...args);
  };
  clearObjectives.click();
  await settled();
  assert.equal(w.document.querySelector(".suite-confirm-card"), null, "清空任务数量提示不应二次确认");
  assert.equal(nativeConfirmCalls, 0);
  assert.deepEqual(facadeWrites, [{ action: "clear-objectives" }], "点击清空任务数量提示应直接提交一次");

  w.dispatchEvent(new w.CustomEvent("deep-legends:live-disconnected"));
  assert.match(w.document.getElementById("suite-rig-root").textContent, /事件流已断开/);

  requests.length = 0;
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: false, eventStream: false } }));
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: true, eventStream: true } }));
  await new Promise((resolve) => setTimeout(resolve, 80));
	const suiteEndpoints = ["/api/watch/rules", "/api/rig/status", "/api/facade/state?trigger=poll", "/api/claim/scan", "/api/champselect/groups", "/api/champselect/state", "/api/champions/catalog"];
  const restoredRequests = requests.filter((request) => suiteEndpoints.includes(request));
  assert.deepEqual(restoredRequests.sort(), ["/api/claim/scan", "/api/facade/state?trigger=poll", "/api/rig/status"], `离线恢复应强刷当前领奖页和 rig，并预加载生涯，实际 ${requests.join(", ")}`);
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: true, eventStream: true } }));
  await new Promise((resolve) => setTimeout(resolve, 80));
  assert.equal(requests.filter((request) => suiteEndpoints.includes(request)).length, restoredRequests.length, "重复在线状态不应再次请求工具接口");

  assert.match(appStyles, /\.button-danger\s*\{[^}]*background\s*:/s);
  assert.match(appStyles, /\.app-select-menu\s*\{[^}]*display\s*:\s*flex[^}]*flex-direction\s*:\s*column[^}]*max-height\s*:\s*min\(320px,\s*calc\(50vh\s*\/\s*var\(--ui-zoom,\s*1\)\)\)[^}]*overflow\s*:\s*hidden[^}]*overscroll-behavior\s*:\s*contain/s);
  assert.match(appStyles, /\.app-select-options\s*\{[^}]*flex\s*:\s*1\s+1\s+auto[^}]*overflow-y\s*:\s*auto[^}]*overscroll-behavior\s*:\s*contain/s);
  assert.match(fs.readFileSync(path.join(WEB, "app.js"), "utf8"), /selected\.offsetTop\s*-\s*optionsRoot\.offsetTop\s*-\s*\(optionsRoot\.clientHeight\s*-\s*selected\.offsetHeight\)\s*\/\s*2/);
  assert.match(fs.readFileSync(path.join(WEB, "app.js"), "utf8"), /pageScrollRoot\.scrollTop\s*=\s*pageScrollTop/, "下拉打开后没有恢复页面主滚动位置");
  assert.match(suiteStyles, /\.facade-film\s*\{[^}]*grid-template-columns\s*:\s*repeat\(auto-fill,[^}]*overflow-x\s*:\s*hidden[^}]*overflow-y\s*:\s*auto/s);
  assert.match(suiteStyles, /\.facade-preview-art\s*\{[^}]*aspect-ratio\s*:\s*1215\s*\/\s*717[^}]*height\s*:\s*auto/s);
  assert.match(suiteStyles, /@container suite-page \(max-width:\s*1060px\)\s*\{[^}]*\.facade-row\s*\{[^}]*grid-template-columns\s*:\s*1fr[^}]*\}/s, "中等宽度生涯设置行未切换为单列");
  assert.match(suiteStyles, /@media \(max-width:\s*1250px\)\s*\{[^}]*\.facade-row\s*\{[^}]*grid-template-columns\s*:\s*1fr[^}]*\}/s, "1250px 以下生涯设置行未切换为单列");
  assert.doesNotMatch(fs.readFileSync(path.join(WEB, "suite.js"), "utf8"), /\bconfirm\s*\(/);
	assert.doesNotMatch(suiteSource, /requestedAvailability\s*===\s*["']dnd["']\s*&&/, "在线状态回弹仍只处理游戏中");
  assert.deepEqual(errors, [], `R55 工具页渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("全部原生下拉都增强为可键盘操作的应用菜单，包含动态英雄段位", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const staticSelects = [...w.document.querySelectorAll(".select-wrap > select")];
  assert.ok(staticSelects.length >= 10, `静态下拉数量异常：${staticSelects.length}`);
  for (const select of staticSelects) {
    const menuRoot = select.parentElement.querySelector(":scope > .native-select-menu");
    assert.ok(menuRoot, `${select.id || select.getAttribute("aria-label") || "未命名下拉"} 未增强`);
    assert.equal(select.classList.contains("sr-only"), true);
    assert.equal(select.getAttribute("aria-hidden"), "true");
    assert.ok(menuRoot.querySelector('[role="menu"]'));
    assert.equal(menuRoot.querySelectorAll('[role="menuitemradio"]').length, select.options.length);
  }

  w.document.querySelector('[data-section="champions"]').click();
  await settled();
  const tierSelect = w.document.querySelector("#champions-panel [data-champion-tier]");
  assert.ok(tierSelect, "英雄页缺少段位下拉");
  const tierRoot = tierSelect.parentElement.querySelector(":scope > .native-select-menu");
  const trigger = tierRoot?.querySelector("[data-app-select-trigger]");
  const menu = tierRoot?.querySelector('[role="menu"]');
  assert.ok(trigger && menu, "动态段位下拉未增强为应用菜单");
  trigger.dispatchEvent(new w.KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
  assert.equal(trigger.getAttribute("aria-expanded"), "true");
  assert.equal(menu.hidden, false);
  const selectedOption = menu.querySelector('[role="menuitemradio"][aria-checked="true"]');
  assert.ok(selectedOption);
  selectedOption.dispatchEvent(new w.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  assert.equal(trigger.getAttribute("aria-expanded"), "false");
  assert.equal(menu.hidden, true);
  assert.deepEqual(errors, [], `下拉增强出现异常：\n${errors.join("\n")}`);
  w.close();
});

function assertR59FacadeTitleContracts(source) {
	const { facadeTitleText } = compileFunctions(source, ["facadeTitleText"], {});
	const dom = new JSDOM('<section class="facade-preview"><div class="facade-signature"></div></section>');
	const preview = dom.window.document.querySelector(".facade-signature");
	const uuid = "38a4e9d4-b2f2-2356-969f-e39316e18ede";
	preview.textContent = facadeTitleText({ challengeSummary: { title: { name: "炫彩达人", contentId: uuid, itemId: 123 } } });
	assert.equal(preview.textContent, "炫彩达人", "object title 没有使用可读的 name");
	preview.textContent = facadeTitleText({ challengeSummary: { title: null }, chat: { lol: { playerTitleSelected: uuid } } });
	assert.equal(preview.textContent, "未设置头衔", "缺少可读 title 时没有回退到未设置");
	assert.doesNotMatch(dom.window.document.querySelector(".facade-preview").textContent, new RegExp(uuid), "UUID 泄漏到生涯预览");
	assert.equal(facadeTitleText({ challengeSummary: { title: "峡谷先锋" } }), "峡谷先锋", "string title 回归");
	assert.equal(facadeTitleText({ challengeSummary: { title: 123 } }), "123", "number title 回归");
	dom.window.close();
}

test("R59 生涯头衔读取对象名称且绝不显示原始 UUID", () => {
	assertR59FacadeTitleContracts(suiteSource);
	const objectBranch = '\n\tif (summaryTitle && typeof summaryTitle === "object") {\n\t  const name = summaryTitle.name;\n\t  if (typeof name === "string" && name.trim()) return name.trim();\n\t}';
	const mutated = suiteSource.replace(objectBranch, "");
	assert.notEqual(mutated, suiteSource, "R59 object title mutation 未命中真实代码");
	assert.throws(() => assertR59FacadeTitleContracts(mutated), /object title 没有使用可读的 name/, "删除 object 分支后测试必须失败");
});

test("R59 生涯预览完整渲染时不泄漏 playerTitleSelected UUID", async () => {
	const uuid = "38a4e9d4-b2f2-2356-969f-e39316e18ede";
	const { window: w, errors } = bootDemoApp({
		facadeStateTransform: (facade) => {
			facade.challengeSummary.title = null;
			facade.chat.lol.playerTitleSelected = uuid;
			return facade;
		},
	});
	await settled();
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await settled();
	const facadeRoot = w.document.getElementById("suite-facade-root");
	assert.equal(facadeRoot.querySelector(".facade-signature")?.textContent, "未设置头衔");
	assert.doesNotMatch(facadeRoot.textContent, new RegExp(uuid), "完整生涯预览泄漏了 UUID");
	assert.deepEqual(errors, [], `R59 UUID 回退渲染出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R60 生涯事件刷新头衔、保护脏草稿且不在其它页签后台请求", async () => {
	const { window: w, errors, eventSources } = bootDemoApp({ liveEvents: true });
	await settled();
	let nextTitle = "客户端新头衔";
	let facadeRequests = 0;
	const originalFetch = w.fetch;
	w.fetch = async (input, init) => {
	  const url = typeof input === "string" ? input : input?.url || "";
	  if (!url.startsWith("/api/facade/state") || String(init?.method || "GET").toUpperCase() !== "GET") return originalFetch(input, init);
	  facadeRequests += 1;
	  const response = await originalFetch(input, init);
	  const payload = await response.json();
	  payload.challengeSummary.title = { name: nextTitle };
	  return new w.Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
	};

	w.document.querySelector('[data-section="suite"]').click();
	await settled();
	w.document.querySelector('[data-suite-tab="facade"]').click();
	await new Promise((resolve) => setTimeout(resolve, 80));
	const source = eventSources.at(-1);
	assert.ok(source?.onmessage, "应用没有建立可接收 facade:changed 的事件流");

	facadeRequests = 0;
	nextTitle = "胜利皮肤收藏家";
	source.onmessage({ data: "facade:changed" });
	await new Promise((resolve) => setTimeout(resolve, 900));
	assert.equal(facadeRequests, 1, "干净草稿收到 facade:changed 后没有重新请求状态");
	assert.equal(w.document.querySelector("#suite-facade-root .facade-signature")?.textContent, "胜利皮肤收藏家");

	const status = w.document.querySelector("#suite-facade-root [data-facade-status]");
	status.value = "这段正在编辑的签名不能丢";
	status.dispatchEvent(new w.Event("input", { bubbles: true }));
	facadeRequests = 0;
	nextTitle = "焕然一新";
	source.onmessage({ data: "facade:changed" });
	await new Promise((resolve) => setTimeout(resolve, 900));
	assert.equal(facadeRequests, 1, "脏草稿刷新没有请求最新 facade 状态");
	assert.equal(w.document.querySelector("#suite-facade-root [data-facade-status]")?.value, "这段正在编辑的签名不能丢", "外部刷新覆盖了用户正在编辑的草稿");
	assert.equal(w.document.querySelector("#suite-facade-root .facade-signature")?.textContent, "焕然一新", "保留脏草稿时没有刷新头衔");

	w.document.querySelector('[data-suite-tab="watch"]').click();
	facadeRequests = 0;
	source.onmessage({ data: "facade:changed" });
	await new Promise((resolve) => setTimeout(resolve, 900));
	assert.equal(facadeRequests, 0, "不在生涯页时仍后台请求 facade 状态");
	assert.match(suiteSource, /Date\.now\(\)\s*-\s*state\.facadeLoadedAt\s*<\s*30000/, "生涯页缺少 30 秒过期兜底");
	assert.deepEqual(errors, [], `R60 生涯刷新出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R65 生涯事件 20 连发只请求一次、仅时间戳变化不重建且按压期间不丢点击", async (t) => {
	let facadeVersion = 0;
	const { window: w, errors } = bootDemoApp({
		facadeStateTransform: (facade) => {
			facade.chat.lastSeenOnlineTimestamp = ++facadeVersion;
			return facade;
		},
	});
	t.after(() => w.close());
	await settled();
	w.document.querySelector('[data-section="suite"]').click();
	await settled();
	w.document.querySelector('[data-suite-tab="facade"]').click();
	await new Promise((resolve) => setTimeout(resolve, 100));
	const facadeRoot = w.document.getElementById("suite-facade-root");
	const originalFetch = w.fetch;
	let facadeRequests = 0;
	w.fetch = (input, init) => {
		const url = typeof input === "string" ? input : input?.url || "";
		if (url.startsWith("/api/facade/state") && String(init?.method || "GET").toUpperCase() === "GET") facadeRequests += 1;
		return originalFetch(input, init);
	};
	const stableNode = facadeRoot.querySelector(".facade-preview");
	let rebuilds = 0;
	const observer = new w.MutationObserver((mutations) => {
		rebuilds += mutations.filter((mutation) => mutation.type === "childList").length;
	});
	observer.observe(facadeRoot, { childList: true });
	for (let index = 0; index < 20; index += 1) w.dispatchEvent(new w.CustomEvent("deep-legends:facade-changed"));
	await new Promise((resolve) => setTimeout(resolve, 900));
	observer.disconnect();
	assert.ok(facadeRequests <= 1, `20 次 facade 事件触发了 ${facadeRequests} 次请求`);
	assert.equal(facadeRoot.querySelector(".facade-preview"), stableNode, "只有聊天时间戳变化时仍重建了 DOM");
	assert.equal(rebuilds, 0, `只有聊天时间戳变化时发生了 ${rebuilds} 次 DOM 子树重建`);

	const target = [...facadeRoot.querySelectorAll("[data-facade-skin]")].find((button) => !button.classList.contains("is-selected"));
	assert.ok(target, "演示数据缺少可点击的第二张皮肤");
	let clicked = 0;
	target.addEventListener("click", () => { clicked += 1; });
	const targetID = target.dataset.facadeSkin;
	target.dispatchEvent(new w.MouseEvent("mousedown", { bubbles: true }));
	w.dispatchEvent(new w.CustomEvent("deep-legends:facade-changed"));
	await new Promise((resolve) => setTimeout(resolve, 850));
	assert.equal(facadeRequests, 1, "鼠标仍按下时不应刷新 facade");
	target.dispatchEvent(new w.MouseEvent("mouseup", { bubbles: true }));
	target.click();
	assert.equal(clicked, 1, "mousedown 到 mouseup 之间的 facade 事件吞掉了 click");
	assert.equal(facadeRoot.querySelector("[data-facade-skin].is-selected")?.dataset.facadeSkin, targetID);
	await new Promise((resolve) => setTimeout(resolve, 50));
	assert.deepEqual(errors, [], `R64 facade 事件保护出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R64 生涯皮肤与身份控件局部更新，皮肤图片延迟解码", async () => {
	const { window: w, errors } = bootDemoApp({facadeStateTransform: facade => ({...facade, skins: facade.skins.map(skin => ({...skin, splashPath: `/lol-game-data/assets/demo/${skin.id}/splash.jpg`}))})});
	await settled();
	w.document.querySelector('[data-section="suite"]').click();
	await settled();
	w.document.querySelector('[data-suite-tab="facade"]').click();
	await new Promise((resolve) => setTimeout(resolve, 100));
	const root = w.document.getElementById("suite-facade-root");
	const hero = root.querySelector("[data-facade-hero]");
	const heroMenu = hero._appSelectRoot;
	const buttons = [...root.querySelectorAll("[data-facade-skin]")];
	const target = buttons.find((button) => !button.classList.contains("is-selected"));
	const previewBefore = root.querySelector("[data-suite-facade-art]")?.getAttribute("data-queued-src");
	assert.ok(target && previewBefore, "演示数据不足以验证皮肤切换");
	target.click();
	const buttonsAfter = [...root.querySelectorAll("[data-facade-skin]")];
	assert.equal(buttonsAfter.length, buttons.length);
	buttonsAfter.forEach((button, index) => assert.equal(button, buttons[index], "点击皮肤重建了皮肤按钮"));
	assert.equal(target.classList.contains("is-selected"), true);
	assert.equal(root.querySelector("[data-suite-facade-art]")?.getAttribute("data-queued-src"), previewBefore, "未应用时当前背景不应改变");
	for (const image of root.querySelectorAll(".facade-film img")) {
		assert.equal(image.getAttribute("loading"), "lazy");
		assert.equal(image.getAttribute("decoding"), "async");
	}

	const availabilityNode = root.querySelector("[data-facade-preview-availability]");
	root.querySelector('[data-facade-availability="away"]').click();
	assert.equal(root.querySelector("[data-facade-preview-availability]"), availabilityNode, "在线状态切换重建了预览");
	assert.equal(availabilityNode.textContent, "离开");
	const rankNode = root.querySelector("[data-facade-preview-rank]");
	const tier = root.querySelector('[data-facade-rank="tier"]');
	tier.value = "MASTER";
	tier.dispatchEvent(new w.Event("change", { bubbles: true }));
	assert.equal(root.querySelector("[data-facade-preview-rank]"), rankNode, "段位切换重建了预览");
	assert.match(rankNode.textContent, /大师/);

	const nextHero = [...hero.options].find((option) => option.value !== hero.value);
	assert.ok(nextHero, "演示数据缺少第二个英雄");
	hero.value = nextHero.value;
	hero.dispatchEvent(new w.Event("change", { bubbles: true }));
	assert.equal(root.querySelector("[data-facade-hero]"), hero, "换英雄时重建了英雄 select");
	assert.equal(hero._appSelectRoot, heroMenu, "换英雄时重建了增强下拉");
	assert.equal(root.querySelector("[data-suite-facade-art]")?.getAttribute("data-queued-src"), previewBefore, "换英雄时当前背景不应改变或清空");
	assert.deepEqual(errors, [], `R64 生涯局部更新出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R64 生涯头衔和勋章仅在真实值上使用强调色", async () => {
	const { window: w, errors } = bootDemoApp({
		facadeStateTransform: (facade) => {
			facade.challengeSummary.title = null;
			facade.challenges = [{ id: "1", name: "真实勋章" }];
			return facade;
		},
	});
	await settled();
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await settled();
	const root = w.document.getElementById("suite-facade-root");
	const title = root.querySelector(".facade-signature");
	const slots = [...root.querySelectorAll("[data-facade-challenge-slot]")];
	assert.equal(title.textContent, "未设置头衔");
	assert.equal(title.classList.contains("is-filled"), false, "头衔占位态被当作真实值上色");
	assert.equal(slots[0].classList.contains("is-filled"), true);
	assert.equal(slots[1].classList.contains("is-filled"), false, "勋章占位态被当作真实值上色");
	assert.match(suiteStyles, /\.facade-signature\s*\{[^}]*place-items:\s*center[^}]*text-align:\s*center/s);
	assert.match(suiteStyles, /\.facade-signature\.is-filled\s*\{[^}]*color:\s*var\(--primary-strong\)/s);
	assert.match(suiteStyles, /\.facade-slot\.is-filled\s*\{[^}]*color:\s*var\(--accent\)/s);
	assert.deepEqual(errors, [], `R64 头衔勋章样式出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R60 生涯勋章缺失槽位稳定回退为未设置", () => {
	const { facadeChallengeSlots } = compileFunctions(suiteSource, ["facadeChallengeSlots"], {
	  state: { facade: {} },
	  escapeHTML: (value) => String(value),
	});
	const dom = new JSDOM(`<div>${facadeChallengeSlots({ challenges: [{ id: "1", name: "唯一勋章" }] })}</div>`);
	assert.deepEqual([...dom.window.document.querySelectorAll("[data-facade-challenge-slot]")].map((node) => node.textContent), ["唯一勋章", "未设置", "未设置"]);
	assert.doesNotMatch(dom.window.document.body.textContent, /undefined|null/);
	dom.window.close();
});

test("R58 生涯英雄长下拉支持别名搜索，短下拉不显示搜索框", async () => {
	const { window: w, errors } = bootDemoApp();
	await settled();
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await settled();
	const heroSelect = w.document.querySelector("[data-facade-hero]");
	assert.ok(heroSelect.options.length > 20, `演示英雄选项不足以触发长下拉：${heroSelect.options.length}`);
	const root = heroSelect.parentElement.querySelector(":scope > .native-select-menu");
	const trigger = root.querySelector("[data-app-select-trigger]");
	const menu = root.querySelector("[data-app-select-menu]");
	const optionsRoot = root.querySelector("[data-app-select-options]");
	const search = root.querySelector("[data-app-select-search-input]");
	assert.ok(search, "长下拉没有搜索输入框");
	assert.ok(root.querySelector(".app-select-search-icon"), "长下拉没有应用内搜索图标");
	assert.match(appStyles, /\.app-select-menu\s*\{[^}]*display:\s*flex[^}]*flex-direction:\s*column[^}]*overflow:\s*hidden/s, "搜索栏和选项必须使用纵向弹性布局与独立滚动层");
	assert.match(appStyles, /\.app-select-options\s*\{[^}]*flex:\s*1\s+1\s+auto[^}]*min-height:\s*0[^}]*overflow-y:\s*auto/s, "英雄名称列表缺少可收缩的独立滚动容器");
	assert.match(appStyles, /\.app-select-search input\s*\{[^}]*font-size:\s*12px[^}]*font-weight:\s*400/s, "搜索提示语没有缩小并减轻字重");
	trigger.click();
	assert.equal(w.document.activeElement, search, "长下拉打开后没有自动聚焦搜索框");
	assert.equal(menu.scrollTop, 0, "打开长下拉时不应把搜索栏卷出菜单");
	assert.ok(optionsRoot.scrollTop >= 0, "长下拉应滚动选项列表而不是整个菜单");
	search.value = "adk";
	search.dispatchEvent(new w.Event("input", { bubbles: true }));
	const buttons = [...root.querySelectorAll("[data-native-select-value]")];
	const visible = buttons.filter((button) => !button.hidden).map((button) => button.querySelector("span")?.textContent);
	assert.deepEqual(visible, ["阿卡丽"], `别名搜索结果不精确：${visible.join(", ")}`);
	assert.ok(buttons.some((button) => button.hidden), "搜索过滤被短路，所有选项仍然可见");
	w.document.body.dispatchEvent(new w.Event("pointerdown", { bubbles: true }));
	assert.equal(trigger.getAttribute("aria-expanded"), "false");
	assert.equal(search.value, "", "关闭长下拉后没有清空搜索词");
	assert.ok(buttons.every((button) => !button.hidden), "关闭长下拉后没有恢复全部选项");
	trigger.click();
	const firstOpenButtons = [...optionsRoot.querySelectorAll('[role="menuitemradio"]')];
	w.document.body.dispatchEvent(new w.Event("pointerdown", { bubbles: true }));
	trigger.click();
	const secondOpenButtons = [...optionsRoot.querySelectorAll('[role="menuitemradio"]')];
	assert.equal(secondOpenButtons.length, firstOpenButtons.length);
	secondOpenButtons.forEach((button, index) => assert.equal(button, firstOpenButtons[index], "相同英雄选项第二次打开时仍重建按钮"));
	heroSelect.value = heroSelect.options[1].value;
	w.deepLegendsSelects.sync(heroSelect);
	assert.equal(optionsRoot.querySelector(`[data-native-select-value="${heroSelect.value}"]`)?.getAttribute("aria-checked"), "true", "未重建时没有同步选中态");

	const queueSelect = w.document.querySelector('[data-facade-rank="queue"]');
	const queueRoot = queueSelect.parentElement.querySelector(":scope > .native-select-menu");
	assert.equal(queueSelect.options.length, 2);
	assert.equal(queueRoot.querySelector("[data-app-select-search-input]"), null, "短下拉不应创建搜索框");
	assert.deepEqual(errors, [], `长下拉搜索出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R58 生涯空皮肤刷新保留草稿并在数据补齐后恢复", async (t) => {
	const draft = { hero: "99", skinId: 99001, ownedOnly: false };
	const state = { facadeDraft: draft, facade: { profile: {backgroundSkinId:99001}, skins: [], chat: {}, loginReset: {} } };
	const { hydrateFacadeDraft } = compileFunctions(suiteSource, ["hydrateFacadeDraft"], { state });
	hydrateFacadeDraft(true);
	assert.equal(state.facadeDraft.hero, "99");
	assert.equal(state.facadeDraft.skinId, 99001);
	state.facade = { skins: [{ id: 103028, championId: 103, owned: true }], profile: { backgroundSkinId: 103028 }, chat: {}, loginReset: {} };
	hydrateFacadeDraft(true);
	assert.equal(state.facadeDraft.hero, "103");
	assert.equal(state.facadeDraft.skinId, 103028);

	const unknownState = { facadeDraft: { hero: "99", skinId: 99001 }, facade: { profileUnavailable:true, skins: [], chat: {}, loginReset: {} } };
	compileFunctions(suiteSource, ["hydrateFacadeDraft"], { state: unknownState }).hydrateFacadeDraft(true);
	assert.equal(unknownState.facadeDraft.hero, "", "unknown current profile must not be replaced by a stale draft");
	assert.equal(unknownState.facadeDraft.skinId, 0);

	const { window: w, errors } = bootDemoApp();
	t.after(() => w.close());
	await settled();
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await settled();
	const initial = await (await w.fetch("/api/facade/state")).json();
	const originalFetch = w.fetch;
	let skinsReady = false;
	w.fetch = (input, init) => String(input).startsWith("/api/facade/state")
		? Promise.resolve(new w.Response(JSON.stringify({ ...initial, skins: skinsReady ? initial.skins : [] }), { status: 200, headers: { "Content-Type": "application/json" } }))
		: originalFetch(input, init);
	const refreshFacade = async () => {
		// Empty same-account snapshots must retain the draft; disconnect is a separate invalidation boundary.
		w.dispatchEvent(new w.CustomEvent("deep-legends:facade-changed"));
		await new Promise((resolve) => setTimeout(resolve, 1050));
	};
	const selectedBefore = w.document.querySelector("[data-facade-hero]").value;
	await refreshFacade();
	assert.equal(w.document.querySelector("[data-facade-hero]").value, selectedBefore, "同步未完成时背景英雄被清空");
	assert.doesNotMatch(w.document.querySelector(".facade-art-label").textContent, /未设置/);
	skinsReady = true;
	await refreshFacade();
	assert.equal(w.document.querySelector("[data-facade-hero]").value, selectedBefore, "皮肤数据补齐后背景没有恢复");
	assert.match(w.document.querySelector(".facade-art-label").textContent, /星之守护者 拉克丝/);
	assert.deepEqual(errors, [], `空皮肤恢复流程出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("顶部重新读取实际刷新生涯并丢弃未应用预览", async (t) => {
  const {window:w,errors}=bootDemoApp();
  t.after(()=>w.close());
  await settled();
  w.document.querySelector('[data-section="suite"]').click();
  await settled();
  w.document.querySelector('[data-suite-tab="facade"]').click();
  const current=await (await w.fetch('/api/facade/state')).json();
  const original=Number(current.profile.backgroundSkinId);
  for (let attempt=0;attempt<50 && !w.document.querySelector("[data-facade-skin]");attempt++) await new Promise(resolve=>setTimeout(resolve,10));
  const choice=[...w.document.querySelectorAll('[data-facade-skin]')].find(b=>Number(b.dataset.facadeSkin)!==original);
  assert.ok(choice);
  choice.click();
  assert.match(w.document.querySelector('.facade-art-label').textContent,/当前背景/);
  assert.equal(choice.classList.contains('is-selected'),true);
  const requests=[], fetch=w.fetch;
  w.fetch=(input,init)=>{requests.push(String(input));return fetch(input,init);};
  const detail={waitFor:[],reason:'manual'};
  w.dispatchEvent(new w.CustomEvent('deep-legends:hard-refresh',{detail}));
  assert.ok(detail.waitFor.length>0);
  await Promise.all(detail.waitFor);
  assert.ok(requests.includes('/api/facade/state?trigger=manual'));
  assert.equal(Number(w.document.querySelector('[data-facade-skin].is-selected')?.dataset.facadeSkin),original);
  assert.match(w.document.querySelector('.facade-art-label').textContent,/当前背景/);
  assert.deepEqual(errors,[]);
});

test("收藏账户条复用主页背景和圆头像，并只保留两项事实", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  w.document.querySelector('[data-section="favorites"]').click();
  w.document.querySelector('[data-favorites-page="account"]').click();
  await settled();
  const hero = w.document.querySelector("#account-content .account-hero");
	assert.equal(w.document.querySelector("#favorites-account-panel h2"), null, "账户页旧标题仍在");
	assert.ok(w.document.getElementById("account-live-state"), "账户连接状态被误删");
  assert.ok(hero, "收藏页缺少当前召唤师信息条");
  assert.ok(hero.querySelector(".summoner-strip-art.account-hero-art"), "收藏页缺少主页背景图");
  assert.ok(hero.querySelector(".game-icon.is-summoner-avatar"), "收藏页未使用圆形召唤师头像");
  assert.equal(hero.querySelector(".account-hex"), null, "旧六边形字符仍在");
  assert.equal(hero.querySelectorAll(".account-facts > div").length, 2, "账户事实栏不是两列内容");
  assert.doesNotMatch(hero.textContent, /主页背景/);
  assert.deepEqual(errors, [], `收藏账户条渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("游戏时间分布下方提供等栏宽分享按钮，并在选定路径后生成桌面双列长图", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  const activity = overview.querySelector(".career-column .activity-section");
  const button = overview.querySelector(".career-column [data-generate-overview-share]");
  assert.ok(activity, "总览缺少游戏时间分布");
  assert.ok(button, "游戏时间分布下方缺少生成分享图按钮");
  assert.equal(activity.nextElementSibling, button, "分享按钮必须紧跟游戏时间分布");
  assert.equal(button.parentElement.classList.contains("career-column"), true, "按钮应直接占满左侧生涯栏宽度");
  assert.ok(button.querySelector("svg"), "分享按钮文字前应有图标");
  assert.match(button.textContent, /生成分享图/);
	const matchList = overview.querySelector(".match-list");
	while (matchList.querySelectorAll(":scope > .match-entry").length < 25) {
		matchList.append(matchList.querySelector(":scope > .match-entry").cloneNode(true));
	}

  const calls = [];
  w.desktopShare = {
    async preparePngSave(fileName) {
      calls.push(["choose", fileName]);
      assert.equal(w.document.querySelector(".overview-share-surface"), null, "弹保存框前不应构造或挂载导出页面");
      return { canceled: false, token: "A".repeat(32), directory: "C:\\Users\\Player\\Pictures" };
    },
    async captureAndSavePng(payload) {
      calls.push(["capture", payload]);
      assert.equal(payload.token, "A".repeat(32));
      assert.match(payload.markup, /overview-share-watermark/);
      assert.match(payload.markup, /DEEP LEGENDS/);
      assert.match(payload.markup, /career-column/);
      assert.match(payload.markup, /matches-column/);
      assert.doesNotMatch(payload.markup, /data-generate-overview-share/);
      assert.doesNotMatch(payload.markup, /\sstyle="/, "导出 HTML 不应携带会被 CSP 拦截的内联 style");
		const exported = new JSDOM(payload.markup).window.document;
		assert.equal(exported.querySelectorAll(".match-list > .match-entry").length, 20, "默认设置应只导出前 20 场");
		assert.equal(exported.querySelector(".match-pagination"), null, "分享图不应保留分页提示");
      return { ok: true, fileName: "share.png", pixelWidth: 2976, pixelHeight: 7200 };
    },
  };
  button.click();
  await new Promise((resolve) => setTimeout(resolve, 40));
  assert.deepEqual(calls.map(([name]) => name), ["choose", "capture"], "必须先选择保存位置，再生成截图");
  assert.match(button.textContent, /分享图已保存/);
  assert.ok(button.classList.contains("is-success"));
	assert.equal(w.document.getElementById("setting-share-directory").textContent, "C:\\Users\\Player\\Pictures", "首次生成选定目录后设置页应立即显示真实位置");
  assert.deepEqual(errors, [], `分享图交互出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("分享图按 50 场设置截断当前已加载战绩", async () => {
	const { window: w } = bootDemoApp({ matchCount: 50 });
	await settled();
	const overview = w.document.getElementById("overview-content");
	const matchList = overview.querySelector(".match-list");
	while (matchList.querySelectorAll(":scope > .match-entry").length < 55) {
		matchList.append(matchList.querySelector(":scope > .match-entry").cloneNode(true));
	}
	let exportedCount = 0;
	w.desktopShare = {
		async preparePngSave() { return { canceled: false, token: "B".repeat(32), directory: "D:\\Shares" }; },
		async captureAndSavePng(payload) {
			exportedCount = new JSDOM(payload.markup).window.document.querySelectorAll(".match-list > .match-entry").length;
			return { ok: true };
		},
	};
	overview.querySelector("[data-generate-overview-share]").click();
	await new Promise((resolve) => setTimeout(resolve, 40));
	assert.equal(exportedCount, 50);
	w.close();
});

test("排位分区在负场缺失时也能渲染出胜率提示", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const ranks = w.document.querySelector("#overview-content .rank-list");
  assert.deepEqual(errors, [], `渲染期出现异常：\n${errors.join("\n")}`);
  assert.ok(ranks, "总览缺少排位分区");
  assert.ok(ranks.querySelectorAll(".rank-row").length >= 2, "排位分区应至少渲染单排与灵活两行");
  w.close();
});

test("渲染异常会切到可重试的错误态，而不是永远停在骨架屏", async () => {
  const { window: w } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  // 走完整的真实链路：让 /api/gameplay/overview 回一份 capabilities 形状非法的载荷，
  // renderCareerSections 会在 capabilities.find 上抛 TypeError。
  const previousFetch = w.fetch;
  w.fetch = (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    if (url.split("?")[0] === "/api/gameplay/overview") {
      const body = JSON.stringify({ player: { playerRef: "broken" }, matches: [], capabilities: 1 });
      return Promise.resolve(new w.Response(body, { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    return previousFetch(input, init);
  };
  w.document.getElementById("overview-refresh").click();
  await settled();
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "渲染失败后不应停在骨架屏");
  assert.match(overview.textContent, /总览渲染失败/, "渲染失败后应展示错误态");
  assert.ok(overview.querySelector("[data-gameplay-retry]"), "错误态应带重试按钮");
  w.close();
});

test("已有总览刷新时保留当前内容，不退回整页骨架", async () => {
  const { window: w } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  w.document.getElementById("overview-refresh").click();
  assert.ok(overview.querySelector(".summoner-strip"), "刷新时应继续展示已有总览");
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "已有数据刷新不应退回骨架屏");
  assert.match(overview.querySelector(".overview-refreshing")?.textContent || "", /正在刷新最新战绩/);
  await settled();
  w.close();
});

test("客户端退出后总览显示可重试提示，不退回空白页", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  assert.ok(overview.querySelector(".summoner-strip"), "断连前总览应已完成渲染");
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: false } }));
  assert.match(overview.textContent, /等待英雄联盟客户端/);
  assert.match(overview.textContent, /启动并登录客户端后会自动恢复/);
  assert.ok(overview.querySelector("[data-gameplay-retry]"), "断连提示应保留手动重试入口");
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "断连后不应退回骨架屏");
	assert.equal(w.document.querySelectorAll("#player-tabs [data-player-tab]").length, 0, "断连后不应残留国服玩家标签");
  await settled();
  assert.deepEqual(errors, [], `断连渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("推荐请求期间遮罩只覆盖 tab 下方面板，并按内容提示", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const previousFetch = w.fetch;
  let recommendations;
  let releaseRecommendations;
  w.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    const pathname = url.split("?")[0];
    if (pathname === "/api/gameplay/live") {
      const response = await previousFetch(input, init);
      const payload = await response.json();
      recommendations = payload.recommendations;
      delete payload.recommendations;
      return new w.Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
    }
    if (pathname === "/api/gameplay/recommendations") {
      return new Promise((resolve) => {
        releaseRecommendations = () => resolve(new w.Response(JSON.stringify({ recommendations }), { status: 200, headers: { "Content-Type": "application/json" } }));
      });
    }
    return previousFetch(input, init);
  };
  w.document.querySelector('[data-section="live"]').click();
  await new Promise((resolve) => setTimeout(resolve, 80));
  const live = w.document.getElementById("live-content");
  const loadingPanels = [...live.querySelectorAll(".recommendation-panel.is-loading")];
  assert.ok(loadingPanels.length >= 2, "符文和出装页签都应进入局部加载态");
  assert.match(loadingPanels.map((panel) => panel.textContent).join(" "), /正在读取符文推荐/);
  assert.match(loadingPanels.map((panel) => panel.textContent).join(" "), /正在读取海克斯、出装与技能/);
  assert.ok(loadingPanels.every((panel) => panel.querySelector(":scope > .panel-loading")), "遮罩必须是面板的直接子元素");
  assert.ok(!live.querySelector(".recommendation-tabs .panel-loading"), "tab 栏不应被遮罩包住");
  releaseRecommendations?.();
  await settled();
  assert.deepEqual(errors, [], `加载态渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("对局页出装推荐渲染，且不再出现「后期备选」", async () => {
  const { window: w, errors } = await bootLiveTab();
  const build = w.document.querySelector("#live-content .build-recommendation");
  assert.deepEqual(errors, [], `渲染期出现异常：\n${errors.join("\n")}`);
  assert.ok(build, "对局页缺少出装推荐区");
  assert.doesNotMatch(build.textContent, /后期备选/, "「后期备选」应已整块移除");
  assert.ok(build.querySelector(".build-summary-spells"), "缺少召唤师技能块");
  assert.ok(build.querySelector(".build-summary-starter"), "缺少出门装块");
  assert.ok(build.querySelector(".build-summary-boots"), "缺少鞋子块");
  w.close();
});

test("核心装只展示胜率与场次，不再展示选取率", async () => {
  const { window: w } = await bootLiveTab();
  const core = w.document.querySelector("#live-content .item-core-column");
  assert.ok(core, "缺少核心装分栏");
  const stats = core.querySelector(".config-option .option-stats");
  assert.ok(stats, "核心装缺少统计列");
  assert.ok(stats.classList.contains("is-depth"), "核心装统计列应使用两列（胜率/场次）版式");
  assert.doesNotMatch(core.textContent, /选取率/, "核心装不应再出现选取率");
  assert.match(core.textContent, /胜率/, "核心装应保留胜率");
  assert.match(core.textContent, /场次/, "核心装应保留场次");
  w.close();
});

test("技能加点的选取率/胜率/场次与技能图标同一行", async () => {
  const { window: w } = await bootLiveTab();
  const row = w.document.querySelector("#live-content .skill-plan .skill-priority-row");
  assert.ok(row, "技能加点缺少 skill-priority-row");
  assert.ok(row.querySelector(".skill-priority"), "该行应包含技能图标组");
  assert.ok(row.querySelector(".option-stats"), "该行应包含统计列——统计列还留在标题行上就说明没改对");
  assert.ok(!w.document.querySelector("#live-content .skill-plan-head .option-stats"), "统计列不应再挂在标题行");
  w.close();
});

test("对抗样本为空时给出说明文案，而不是一个「—」", async () => {
  const { window: w } = await bootLiveTab();
  const header = w.document.querySelector("#live-content .champion-matchups");
  assert.ok(header, "缺少优劣势对抗区");
  const empties = header.querySelectorAll(".matchups-empty");
  for (const node of empties) {
    assert.match(node.textContent, /暂无对线样本/);
    assert.ok(node.dataset.tooltip, "空状态应带解释性 tooltip");
  }
  assert.ok(!/^—$/.test(header.textContent.trim()), "不应只渲染一个破折号");
  w.close();
});

test("英雄位置统计为空时显示明确说明，而不是三项破折号", async () => {
  const boot = bootDemoApp();
  const w = boot.window;
  await settled();
  const previousFetch = w.fetch;
  w.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    if (url.split("?")[0] !== "/api/gameplay/live") return previousFetch(input, init);
    const response = await previousFetch(input, init);
    const payload = await response.json();
    payload.recommendations = payload.recommendations || {};
    payload.recommendations.hero = { emptyReason: "该英雄在这个位置没有统计样本" };
    return new w.Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
  };
  w.document.querySelector('[data-section="live"]').click();
  await settled();
  const summaries = [...w.document.querySelectorAll("#live-content .recommendation-champion-summary")];
  assert.ok(summaries.length > 0, "缺少推荐英雄摘要");
  assert.ok(summaries.every((summary) => summary.querySelector(".champion-stats-empty")?.textContent === "该英雄在这个位置没有统计样本"));
  assert.ok(summaries.every((summary) => !summary.querySelector(".champion-summary-main dl")), "空样本时仍渲染了三项破折号");
  w.close();
});

test("failed build timelines retry on reopening, successful timelines remain cached", async () => {
  const state = {matchTimelines: new Map(), matchTimelineFlights:new Set()};
  const tab = {data:{player:{playerRef:"safe-ref"}}};
  const match = {gameId:42}, subject = {participantId:2};
  let calls=0, renders=0;
  const api=async()=>{ calls++; if(calls===1) throw Error("temporary timeout"); return {available:true,itemGroups:[{}],skillOrder:[1]}; };
  const ensure = new Function("state","api","riotTab","connected","matchTimelineKey","tabServerID","rerenderMatch","recordTimelineClient",
    `return (${functionSource(gameplaySource,"ensureMatchTimeline")});`)(state,api,()=>true,()=>true,()=>"kr:42:2",()=>"",()=>renders++,()=>{});
  await ensure(match,subject,tab);
  assert.equal(state.matchTimelines.get("kr:42:2").available,false);
  await ensure(match,subject,tab);
  assert.equal(state.matchTimelines.get("kr:42:2").available,true);
  await ensure(match,subject,tab);
  assert.equal(calls,2);
  assert.equal(renders,2);
  assert.equal(state.matchTimelineFlights.size,0);
});

test("R86 ADD-1 card detail, metric, legacy damage controls and timeline stay local at 200 matches", async () => {
  async function check(mutation = "", external = false) {
    const { window: w, errors } = bootDemoApp({ matchCount: 200, gameplaySourceTransform(source) {
      const boundary = "function bindMatchDetailControls(container, tab) {";
      assert.ok(source.includes(boundary));
      // Test-only access to real renderers/bindings, not alternate implementations.
      source = source.replace(boundary, `window.__r86CardTest = {activeTab, renderTeamAnalysis, bindMatchDetailControls, matchPlayerGroups};\n${boundary}`);
      const selectors = { detail: "[data-match-detail]", metric: "[data-team-analysis-metric]", damage: "[data-damage-sort]" };
      if (selectors[mutation]) {
        source = source.replace(boundary, `${boundary}\nfor (const button of container.querySelectorAll(${JSON.stringify(selectors[mutation])})) button.addEventListener("click", () => { tab.matchViewRevision++; rerenderTab(tab); });`);
      } else if (mutation === "timeline") {
        source = source.replace("if (settled) {", "if (settled) { tab.matchViewRevision++; rerenderTab(tab);");
      }
      return source;
    } });
    try {
      await settled();
      const d = w.document, hooks = w.__r86CardTest, tab = hooks.activeTab();
      const match = tab.data.matches.find(item => !hooks.matchPlayerGroups(item).arena);
      assert.ok(match);
      // Demo names intentionally omit player references; provide one clickable
      // teammate so real link rebinding is covered as well as visual updates.
      match.participants[1].playerRef = "r86-fixture-teammate";
      const id = String(match.gameId);
      let list = d.querySelector(".match-list");
      if (external) {
        list = d.createElement("div"); d.body.append(list);
        w.deepLegendsMatchCards.mount(list, { matches: tab.data.matches, playerRef: tab.data.player.playerRef });
      }
      const card = () => list.querySelector(`[data-match-id="${id}"]`);
      card().querySelector("[data-toggle-match]").click();
      assert.ok(card().querySelector(".match-detail-tabs"));
      let releaseTimeline;
      const originalFetch = w.fetch;
      w.fetch = (url, ...args) => String(url).startsWith("/api/gameplay/match-timeline")
        ? new Promise(resolve => { releaseTimeline = resolve; }) : originalFetch(url, ...args);
      async function bounded(label, action) {
        const before = [...list.querySelectorAll(".match-entry")];
        assert.equal(before.length, 200, label);
        const parent = list.parentElement;
        let calls = 0;
        const create = d.createElement.bind(d);
        d.createElement = (...args) => { calls++; return create(...args); };
        try {
          action();
          await new Promise(resolve => setTimeout(resolve, 30));
          assert.ok(list.isConnected && list.parentElement === parent, `${label}: match-list was replaced`);
          const after = [...list.querySelectorAll(".match-entry")];
          assert.equal(after.length, 200, label);
          before.forEach((entry, index) => {
            if (entry.dataset.matchId !== id) assert.equal(after[index], entry, `${label}: another card was replaced`);
          });
          assert.ok(calls < 500, `${label}: createElement=${calls}, budget <500`);
        } finally { d.createElement = create; }
      }
      await bounded("detail-build", () => card().querySelector('[data-match-detail="build"]').click());
      assert.equal(card().querySelector('[data-match-detail="build"]').getAttribute("aria-selected"), "true");
      assert.equal(typeof releaseTimeline, "function", "a real timeline request must remain in flight");
      await bounded("timeline", () => releaseTimeline(new w.Response(JSON.stringify({ available: true, itemGroups: [{ minute: 5, events: [{ itemId: 1036 }] }], skillOrder: [1, 2] }))));
      assert.ok(card().querySelector(".timeline-route"), "settled timeline must update the current card");
      await bounded("detail-team", () => card().querySelector('[data-match-detail="team"]').click());
      await bounded("metric", () => card().querySelector('[data-team-analysis-metric="gold"]').click());
      assert.equal(card().querySelector('[data-team-analysis-metric="gold"]').getAttribute("aria-selected"), "true");
      await bounded("metric-keyboard", () => {
        const current = card().querySelector('[data-team-analysis-metric="gold"]');
        current.focus(); current.dispatchEvent(new w.KeyboardEvent("keydown", { key: "Home", bubbles: true }));
      });
      assert.equal(d.activeElement?.dataset.teamAnalysisMetric, "damage");

      if (!external) {
        // Legacy damage-sort controls are generated only by renderTeamAnalysis's
        // arena branch; current arena cards use renderArenaMatchOverview instead.
        // Mount the REAL legacy output on this card to guard its still-present
        // binding without inventing a new production UI route.
        const arena = tab.data.matches.find(item => hooks.matchPlayerGroups(item).arena);
        assert.ok(arena);
        const legacy = d.createElement("div");
        legacy.innerHTML = hooks.renderTeamAnalysis({ ...arena, gameId: match.gameId }, tab);
        card().append(legacy); hooks.bindMatchDetailControls(legacy, tab);
        const damage = legacy.querySelector("[data-damage-sort]");
        assert.ok(damage);
        const key = `${id}:${damage.dataset.team}`;
        await bounded("damage", () => damage.click());
        assert.equal(tab.damageSorts.get(key), "damageTaken");
      }
      await bounded("detail-keyboard", () => {
        const current = card().querySelector('[data-match-detail="team"]');
        current.focus(); current.dispatchEvent(new w.KeyboardEvent("keydown", { key: "Home", bubbles: true }));
      });
      assert.equal(d.activeElement?.dataset.matchDetail, "overview");
      const player = [...card().querySelectorAll("[data-player-ref]")].find(button => button.dataset.playerRef !== tab.data.player.playerRef);
      assert.ok(player, "expanded details need another player's clickable name");
      const playerRef = player.dataset.playerRef;
      player.click();
      if (external) assert.equal(d.getElementById("player-overlay").hidden, false);
      else assert.equal(hooks.activeTab().playerRef, playerRef, "single-card replacement must rebind player links");
      // Opening a player starts loadOverview; let that real navigation settle
      // before closing jsdom, rather than accepting teardown TypeErrors as kills.
      await settled();
      assert.deepEqual(errors, []);
    } finally { w.close(); }
  }
  await check();
  await check("", true);
  for (const mutant of ["detail", "metric", "damage", "timeline"]) {
    await assert.rejects(check(mutant), error => error.name === "AssertionError" && /match-list was replaced|another card was replaced|createElement=/.test(error.message), `${mutant} independent full-render bypass must fail the DOM guard`);
  }
});

test("R86 ADD optional cs-dialog computed zoom stays one inside the zoomed app frame", () => {
  function check(css) {
    const dom = new JSDOM(`<style>${appStyles}</style><style>${css}</style><div class="app-frame" id="app-frame"><div class="cs-dialog"></div></div>`);
    try {
      for (const zoom of [1, 2, 2.5]) {
        dom.window.document.documentElement.style.setProperty("--ui-zoom", String(zoom));
        assert.equal(dom.window.getComputedStyle(dom.window.document.querySelector(".cs-dialog")).zoom, "1");
      }
    } finally { dom.window.close(); }
  }
  check(suiteStyles);
  assert.throws(() => check(`${suiteStyles}\n.cs-dialog { zoom: var(--ui-zoom, 1); }`), { name: "AssertionError" });
});

test("R86 ADD-1 failed timelines wait for explicit retry and keep unrelated cards", async () => {
  async function check(mutate = false) {
    const { window: w, errors } = bootDemoApp({ gameplaySourceTransform: source => mutate
      ? source.replace("if (!state.matchTimelines.has(matchTimelineKey(match, subject, tab)))", "if (true)") : source });
    try {
      await settled();
      const d = w.document, list = d.querySelector(".match-list");
      const initial = [...list.querySelectorAll(".match-entry")];
      const id = initial[0].dataset.matchId;
      const card = () => list.querySelector(`[data-match-id="${id}"]`);
      card().querySelector("[data-toggle-match]").click();
      let requests = 0;
      const originalFetch = w.fetch;
      w.fetch = (url, ...args) => String(url).startsWith("/api/gameplay/match-timeline")
        ? Promise.resolve(new w.Response(JSON.stringify(++requests === 1
          ? { available: false, detail: "fixture upstream unavailable" }
          : { available: true, itemGroups: [{ minute: 5, events: [{ itemId: 1036 }] }] }))) : originalFetch(url, ...args);
      card().querySelector('[data-match-detail="build"]').click();
      await new Promise(resolve => setTimeout(resolve, 60));
      assert.equal(requests, 1, "rendering a failure must not silently retry it");
      assert.ok(card().querySelector("[data-timeline-retry]"));
      card().querySelector("[data-timeline-retry]").click();
      await new Promise(resolve => setTimeout(resolve, 60));
      assert.equal(requests, 2);
      assert.ok(card().querySelector(".timeline-route"));
      assert.equal(d.querySelector(".match-list"), list);
      const after = [...list.querySelectorAll(".match-entry")];
      initial.forEach((entry, index) => { if (entry.dataset.matchId !== id) assert.ok(after[index] === entry, "retry replaced an unrelated card"); });
      assert.deepEqual(errors, []);
    } finally { w.close(); }
  }
  await check();
  await assert.rejects(check(true), { name: "AssertionError", message: /rendering a failure must not silently retry it/ });
});

test("R86 ADD-1 late timeline cannot reveal a filtered-out card", async () => {
  async function check(mutate = false) {
    const { window: w } = bootDemoApp({ matchCount: 200, gameplaySourceTransform: source => mutate
      ? source.replace("function replaceMatchEntry(entry, tab, rerender) {", "function replaceMatchEntry(entry, tab, rerender) { entry.hidden = false;") : source });
    try {
      await settled();
      const d = w.document, list = d.querySelector(".match-list");
      const entry = list.querySelector(".match-entry"), id = entry.dataset.matchId;
      const card = () => list.querySelector(`[data-match-id="${id}"]`);
      entry.querySelector("[data-toggle-match]").click();
      let release;
      const original = w.fetch;
      w.fetch = (url, ...args) => String(url).startsWith("/api/gameplay/match-timeline")
        ? new Promise(resolve => { release = resolve; }) : original(url, ...args);
      card().querySelector('[data-match-detail="build"]').click();
      d.querySelector('[data-match-filter="arena"]').click();
      assert.equal(card().hidden, true, "the ordinary match must first be filtered out");
      const others = [...list.querySelectorAll(".match-entry")].filter(node => node.dataset.matchId !== id);
      release(new w.Response(JSON.stringify({ available: true })));
      await new Promise(resolve => setTimeout(resolve, 60));
      assert.equal(card().hidden, true, "late timeline must retain the filter's hidden state");
      assert.ok(others.every(node => node.isConnected));
    } finally { w.close(); }
  }
  await check();
  await assert.rejects(check(true), { name: "AssertionError", message: /late timeline must retain the filter's hidden state/ });
});

test("R86 hero search preserves input and coalesces five keystrokes (including bypass mutation)", async () => {
  async function check(mutate = false) {
    const { window: w, errors } = bootDemoApp({ championRankings: Array.from({ length: 170 }, (_, index) => ({ championId: index + 1, name: index === 102 ? "Ahri" : `Hero ${index + 1}`, key: `Hero${index + 1}`, winRate: 50, tier: 2 })), championsSourceTransform: (source) => {
      source = source.replace('function render() {', 'function render() { window.__r86Renders = (window.__r86Renders || 0) + 1;');
      source = source.replace('function objectRows(value) {', 'function objectRows(value) { window.__r86Rows = (window.__r86Rows || 0) + 1;');
      if (mutate) source = source.replace('root.addEventListener("input", (event) => {', 'root.addEventListener("input", () => { root.innerHTML = root.innerHTML; });\n  root.addEventListener("input", (event) => {');
      return source;
    } });
    try {
      await settled();
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
    await settled();
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
      await settled();
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
  const {window:w}=bootDemoApp({gameplaySourceTransform:mutate ? source=>source.replace('render(id = "") {','render(id = "") { id = "";') : undefined});
  try {
    await settled();
    const data=await (await w.fetch('/api/gameplay/overview')).json();
    const host=w.document.createElement('div');w.document.body.append(host);
    w.deepLegendsMatchCards.mount(host,{matches:data.matches,playerRef:data.player.playerRef});
    const before=[...host.querySelectorAll('.match-entry')];assert.ok(before.length>1);
    before[0].querySelector('[data-toggle-match]').click();
    const after=[...host.querySelectorAll('.match-entry')];
    assert.notEqual(after[0],before[0]);assert.equal(after[1],before[1]);
  }finally{w.dispatchEvent(new w.CustomEvent('deep-legends:dispose'));w.close()}
 }
 await check();await assert.rejects(check(true),{name:"AssertionError"});
});

test('R86 filtering 200 loaded matches hides entries without rebuilding or refetching', async () => {
 async function check(mutate=false) {
  const {window:w,errors}=bootDemoApp({matchCount:200,gameplaySourceTransform:mutate ? source=>source.replace('function reconcileFilteredMatchList(list, tab) {','function reconcileFilteredMatchList(list, tab) { list.innerHTML = list.innerHTML;') : undefined});
  try {
    await settled();
    const d=w.document, before=[...d.querySelectorAll('.match-list .match-entry')];
    assert.equal(before.length,200);
    let creates=0,requests=0;
    const create=d.createElement.bind(d), fetch=w.fetch;
    d.createElement=(...args)=>{creates++;return create(...args)};
    w.fetch=(url,...args)=>{if(String(url).startsWith('/api/gameplay/overview'))requests++;return fetch(url,...args)};
    d.querySelector('[data-match-filter="arena"]').click();
    await Promise.resolve();
    const hidden=before.filter(entry=>entry.hidden);
    assert.ok(hidden.length>0 && hidden.length<200);
    d.querySelector('[data-match-filter="all"]').click();
    await Promise.resolve();
    assert.deepEqual([...d.querySelectorAll('.match-list .match-entry')],before);
    assert.ok(before.every(entry=>!entry.hidden));
    assert.ok(creates<200,`created ${creates}`);
    assert.equal(requests,0,'complete unfiltered dataset needs no first-page request');
    assert.deepEqual(errors,[]);
  } finally {w.close()}
 }
 await check();await assert.rejects(check(true),{name:"AssertionError"});
});

test('R86 image listeners and tab scroll remain bounded including independent image-listener bypass', async () => {
  async function check(mutate=false) {
    const {window:w,errors}=bootDemoApp({matchCount:200,gameplaySourceTransform:source=>{
      source=source.replace('function renderOverviewBodyContent(container, tab) {',`function renderOverviewBodyContent(container, tab) {
        const oldAdd=window.EventTarget.prototype.addEventListener;
        let calls=0;
        window.EventTarget.prototype.addEventListener=function(...args){calls++;return oldAdd.apply(this,args)};
        try { return r86OriginalRender(container,tab); } finally {window.EventTarget.prototype.addEventListener=oldAdd;window.__r86ListenerMax=Math.max(window.__r86ListenerMax||0,calls);}
      }
      function r86OriginalRender(container, tab) {`);
      if(mutate)source=source.replace('function prepareImages(container) {','function prepareImages(container) { for(const image of container.querySelectorAll("img")){image.addEventListener("load",()=>{});image.addEventListener("error",()=>{});}');
      return source;
    }});
    try {
      await settled();
      assert.equal(w.document.querySelectorAll('.match-list .match-entry').length,200);
      assert.ok(w.__r86ListenerMax<1000,`listeners=${w.__r86ListenerMax}`);
      let styles=0;const original=w.getComputedStyle.bind(w);w.getComputedStyle=(...args)=>{styles++;return original(...args)};
      const tabs=w.document.querySelector('#player-tabs');
      for(let i=0;i<20;i++)tabs.dispatchEvent(new w.Event('scroll'));
      await new Promise(resolve=>w.requestAnimationFrame(()=>resolve()));
      assert.ok(styles<=2,`computed styles=${styles}`);
      assert.deepEqual(errors,[]);
    }finally{w.close()}
  }
  await check();await assert.rejects(check(true),{name:'AssertionError'});
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
    summonerContextChip: () => '', summonerProChip: () => '', summonerRegionChip: () => '', renderSummonerHighlights: () => '',
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
    summonerContextChip: () => '', summonerProChip: () => '', summonerRegionChip: () => '', renderSummonerHighlights: () => '',
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
