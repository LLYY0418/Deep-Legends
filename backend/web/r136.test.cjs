"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require(path.join(__dirname, "..", "..", "desktop", "node_modules", "jsdom"));

const appSource = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
const gameplaySource = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const gameplayCSS = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");
const championCSS = fs.readFileSync(path.join(__dirname, "champions.css"), "utf8");
const liveHTML = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");

function mayhemFontSizes(css) {
  const selectors = [
    ".mayhem-performance-group h4", ".mayhem-performance-group dt", ".mayhem-performance-group dd",
    ".mayhem-performance-group dd > .mayhem-delta", ".mayhem-multikill-values small",
    ".mayhem-multikill-values b", ".mayhem-performance-note",
  ];
  return selectors.map((selector) => {
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const declaration = css.match(new RegExp(`${escaped} \\{([^}]+)\\}`));
    assert.ok(declaration, selector);
    const font = declaration[1].match(/font-size:\s*(\d+)px/);
    assert.ok(font, `${selector} font-size`);
    return [selector, Number(font[1])];
  });
}

test("R136 live status belongs to toolbar and leaves content without a fixed placeholder", () => {
  assert.match(liveHTML, /class="live-toolbar"[^>]*>.*data-live-status.*id="live-refresh"/);
  assert.doesNotMatch(liveHTML.match(/<div id="live-content"[^>]*>/)?.[0] || "", /data-live-status/);
  assert.doesNotMatch(functionSource(gameplaySource, "renderLive"), /nodes\.liveContent\.querySelector\("\[data-live-status\]"\)|<div data-live-status>/);
  assert.doesNotMatch(gameplayCSS, /^\[data-live-status\] \{ min-height/m);
  assert.match(gameplayCSS, /#live-content \{ padding-top: 0; \}/);
  assert.doesNotMatch(functionSource(gameplaySource, "renderLiveRefreshStatus"), /data-live-refresh/);
});

test("R136 Mayhem performance font sizes and mutation guard", () => {
  const expected = [13, 13, 14, 12, 12, 13, 12];
  assert.deepEqual(mayhemFontSizes(championCSS).map((row) => row[1]), expected);
  const allowed = new Set([12, 13, 14, 16, 20, 28]);
  for (const [selector, size] of mayhemFontSizes(championCSS)) assert.ok(allowed.has(size), selector);
  for (const [selector] of mayhemFontSizes(championCSS)) {
    const start = championCSS.indexOf(`${selector} {`);
    const end = championCSS.indexOf("}", start);
    const mutated = championCSS.slice(0, start) + championCSS.slice(start, end).replace(/font-size:\s*\d+px/, "font-size: 9px") + championCSS.slice(end);
    assert.ok(mayhemFontSizes(mutated).some((row) => !allowed.has(row[1])), `${selector} mutation must fail`);
  }
});

function functionSource(source, name) {
  const start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `missing ${name}`);
  const bodyStart = source.indexOf("{", source.indexOf(")", start));
  let depth = 0, quote = "", escaped = false;
  for (let index = bodyStart; index < source.length; index++) {
    const char = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth++;
    if (char === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`unclosed ${name}`);
}

function payload(count = 1) {
  return { summoner: { displayName: "测试玩家", summonerLevel: 21 },
    account: { loot: [{ lootId: "CHEST_1", category: "材料", count, asset: "/loot.png" }] },
    rewards: [], capabilities: [] };
}

function accountHarness(source = appSource) {
  const dom = new JSDOM('<div id="account"></div><span id="live"></span>', { url: "http://fixture/" });
  const { window } = dom;
  window.deepLegendsGameIcons = { iconFigure: () => "", prepareImages: () => {} };
  const el = { accountContent: window.document.querySelector("#account"), accountLiveState: window.document.querySelector("#live") };
  const state = { status: { connected: true, eventStream: true }, destroyed: false };
  let apiImpl = async () => payload();
  const deps = {
    state, el, window, api: (...args) => apiImpl(...args),
    escapeHTML: (value) => String(value ?? ""), formatNumber: (value) => String(value ?? 0),
    playerName: () => "测试玩家", lootCategorySlug: () => "material", lootCategoryIcon: () => "◇",
    lootCard: (item) => `<div class="loot-art"><img src="${item.asset}" alt=""><b>${item.count}</b></div>`,
    capabilityName: (value) => value, sourceStateLabel: (value) => value,
    rewardStatusLabel: (value) => value, formatDateTime: (value) => value,
    loadNextLootImage: () => {}, renderPanelError: () => { throw new Error("unexpected panel error"); },
  };
  const names = Object.keys(deps);
  const loadAccount = Function(...names, `async ${functionSource(source, "loadAccount")}; return loadAccount;`)(...names.map((name) => deps[name]));
  return { window, el, state, loadAccount, setAPI: (value) => { apiImpl = value; }, close: () => window.close() };
}

async function accountReloadResult(source = appSource, changed = false) {
  const h = accountHarness(source);
  try {
    await h.loadAccount();
    const firstImage = h.el.accountContent.querySelector(".loot-art img");
    assert.ok(firstImage);
    const mutations = [];
    const observer = new h.window.MutationObserver((items) => mutations.push(...items));
    observer.observe(h.el.accountContent, { childList: true, subtree: true });
    h.setAPI(async () => payload(changed ? 2 : 1));
    await h.loadAccount();
    await Promise.resolve();
    mutations.push(...observer.takeRecords());
    observer.disconnect();
    const loadingInserted = mutations.some((item) => [...item.addedNodes].some((node) =>
      node.nodeType === 1 && (node.matches?.(".account-loading") || node.querySelector?.(".account-loading"))));
    return { loadingInserted, sameImage: firstImage === h.el.accountContent.querySelector(".loot-art img"),
      count: h.el.accountContent.querySelector(".loot-art b")?.textContent };
  } finally { h.close(); }
}

test("R136 account background refresh retains identical image nodes and never reinserts skeleton", async () => {
  assert.deepEqual(await accountReloadResult(), { loadingInserted: false, sameImage: true, count: "1" });
  assert.deepEqual(await accountReloadResult(appSource, true), { loadingInserted: false, sameImage: false, count: "2" });
});

test("R136 account request sequence discards an older response that finishes last", async () => {
  const h = accountHarness();
  try {
    await h.loadAccount();
    const pending = [];
    h.setAPI(() => new Promise((resolve) => pending.push(resolve)));
    const older = h.loadAccount();
    const newer = h.loadAccount();
    pending[1](payload(3));
    await newer;
    pending[0](payload(2));
    await older;
    assert.equal(h.el.accountContent.querySelector(".loot-art b")?.textContent, "3");
  } finally { h.close(); }
});

test("R136 account skeleton and markup guards kill their original mutations", async () => {
  const skeletonGuard = 'if (!el.accountContent._accountMarkup) el.accountContent.innerHTML =';
  const skeletonMutant = appSource.replace(skeletonGuard, 'el.accountContent.innerHTML =');
  assert.notEqual(skeletonMutant, appSource);
  assert.equal((await accountReloadResult(skeletonMutant)).loadingInserted, true);
  const markupGuard = 'if (markup !== el.accountContent._accountMarkup) {';
  const markupMutant = appSource.replace(markupGuard, 'if (true) {');
  assert.notEqual(markupMutant, appSource);
  assert.equal((await accountReloadResult(markupMutant)).sameImage, false);
});

test("R136 pools background refresh preserves rows and picker options", async () => {
  const dom = new JSDOM('<div id="pools"></div><div class="pool-picker-wrap"><select id="picker"></select></div>', { url: "http://fixture/" });
  const { window } = dom;
  const el = { poolsContent: window.document.querySelector("#pools"), poolPicker: window.document.querySelector("#picker"), poolPageTabs: [] };
  const state = { destroyed: false };
  let items = [{ id: "pool-1", name: "三合一奖池", entryCount: 5, selected: true }];
  const deps = { state, el, Option: window.Option, api: async () => ({ items }), escapeHTML: String,
    formatNumber: String, selectPool: () => {}, loadPoolCatalog: async () => {}, loadHistory: async () => {},
    renderPanelError: () => { throw new Error("unexpected pool error"); } };
  const loadPools = Function(...Object.keys(deps), `async ${functionSource(appSource, "loadPools")}; return loadPools;`)(...Object.values(deps));
  try {
    await loadPools();
    const row = el.poolsContent.querySelector("tr td");
    const option = el.poolPicker.querySelector("option");
    const mutations = [];
    const observer = new window.MutationObserver((records) => mutations.push(...records));
    observer.observe(el.poolsContent, { childList: true, subtree: true });
    await loadPools();
    mutations.push(...observer.takeRecords()); observer.disconnect();
    assert.equal(el.poolsContent.querySelector("tr td"), row);
    assert.equal(el.poolPicker.querySelector("option"), option);
    assert.equal(mutations.length, 0);
    items = [{ ...items[0], entryCount: 6 }];
    await loadPools();
    assert.notEqual(el.poolsContent.querySelector("tr td"), row);
    assert.match(el.poolsContent.textContent, /6/);
  } finally { window.close(); }
});
