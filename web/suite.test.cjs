"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const source = fs.readFileSync(path.join(__dirname, "suite.js"), "utf8");

function functionSource(script, name) {
  const marker = `function ${name}(`;
  const start = script.indexOf(marker);
  assert.notEqual(start, -1, `missing ${name}`);
  const bodyStart = script.indexOf("{", script.indexOf(")", start));
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < script.length; index += 1) {
    const char = script[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return script.slice(start, index + 1);
  }
  assert.fail(`unbalanced ${name}`);
}

function compile(names, dependencies = {}, script = source) {
  const keys = Object.keys(dependencies);
  return Function(...keys, `"use strict";\n${names.map((name) => functionSource(script, name)).join("\n")}\nreturn {${names.join(",")}};`)(...keys.map((key) => dependencies[key]));
}

const escapeHTML = (value) => String(value ?? "").replace(/[&<>'"]/g, "_");

test("career manual reread resets stale preview even when server payload is unchanged", async () => {
  const current = { connected: true, profile: { backgroundSkinId: 21069 }, skins: [{ id: 21069, championId: 21, name: "当前皮肤" }, { id: 67004, championId: 67, name: "待应用皮肤" }] };
  const state = { connected: true, facade: structuredClone(current) };
  const helpers = compile(["hydrateFacadeDraft", "facadeRenderSignature", "facadeBackgroundDirty"], { state });
  helpers.hydrateFacadeDraft();
  state.facadeDraft.skinId = 67004;
  state.facadeDraft.hero = "67";
  let rendered = 0;
  const deps = { state, ...helpers, api: async (url, options) => {
    assert.equal(url, "/api/facade/state?trigger=manual");
    assert.equal(options.method, undefined, "reread must never apply a background");
    return structuredClone(current);
  }, renderFacade: () => rendered++, scheduleFacadeChallengeRetry() {}, roots: { facade: {} }, errorCard: () => { throw Error("unexpected load failure"); } };
  const load = Function(...Object.keys(deps), `async ${functionSource(source, "loadFacade")}; return loadFacade;`)(...Object.values(deps));
  await load(true, false, "manual");
  assert.equal(state.facadeDraft.skinId, 21069);
  assert.equal(state.facadeDraft.hero, "21");
  assert.equal(rendered, 1);
});

test("global reread joins the active career refresh and does not refresh hidden tools", async () => {
  const state={active:true,connected:true,tab:"facade"};
  const calls=[], task=Promise.resolve();
  const {handleSuiteHardRefresh}=compile(["handleSuiteHardRefresh"],{state,loadFacade:(...args)=>{calls.push(args);return task;}});
  const event={detail:{waitFor:[]}};
  handleSuiteHardRefresh(event);
  assert.deepEqual(calls,[[true,false,"manual"]]);
  assert.deepEqual(event.detail.waitFor,[task]);
  state.active=false;handleSuiteHardRefresh(event);
  state.active=true;state.tab="watch";handleSuiteHardRefresh(event);
  assert.equal(calls.length,1);
  assert.match(source,/addEventListener\("deep-legends:hard-refresh", handleSuiteHardRefresh\)/);
});

test("career SSE updates a clean background while preserving edited chat fields", async () => {
  const state = { connected: true, facade: { connected: true, profile: { backgroundSkinId: 21069 }, skins: [] } };
  const helpers = compile(["hydrateFacadeDraft", "facadeRenderSignature", "facadeBackgroundDirty"], { state });
  helpers.hydrateFacadeDraft();
  state.facadeDraft.statusMessage = "尚未提交的签名";
  const deps = { state, ...helpers, api: async () => ({ connected: true, profile: { backgroundSkinId: 18080 }, skins: [] }), renderFacade() {}, scheduleFacadeChallengeRetry() {}, roots: { facade: {} }, errorCard: () => { throw Error("unexpected load failure"); } };
  const load = Function(...Object.keys(deps), `async ${functionSource(source, "loadFacade")}; return loadFacade;`)(...Object.values(deps));
  await load(true, true, "sse");
  assert.equal(state.facadeDraft.skinId, 18080);
  assert.equal(state.facadeDraft.hero, "18");
  assert.equal(state.facadeDraft.statusMessage, "尚未提交的签名");
});

test("career filtering never silently selects another background", () => {
  const state = { facade: { skins: [{id:21069,championId:21,owned:false},{id:21060,championId:21,owned:true}] }, facadeDraft: {skinId:21069,hero:"21",ownedOnly:true} };
  const film = {};
  let visible = [];
  const { rebuildFacadeFilm } = compile(["facadeVisibleSkins", "rebuildFacadeFilm"], {state, roots:{facade:{querySelector:()=>film}},facadeFilmHTML:items=>{visible=items;return "filtered";},bindFacadeSkinControls(){},updateFacadeBackgroundPreview(){}});
  rebuildFacadeFilm();
  assert.deepEqual(visible.map(s=>s.id),[21060]);
  assert.equal(state.facadeDraft.skinId,21069);
  state.facadeDraft.hero="67";
  rebuildFacadeFilm();
  assert.deepEqual(visible,[]);
  assert.equal(state.facadeDraft.skinId,21069);
});

test("career apply invalidates an older GET and reports unconfirmed writes honestly", async () => {
  const state = {connected:true,facade:{profile:{backgroundSkinId:21069},skins:[]}};
  const helpers=compile(["hydrateFacadeDraft","facadeRenderSignature","facadeBackgroundDirty"],{state});
  helpers.hydrateFacadeDraft();
  let resolveGet, resolvePost, notice="", requests=0;
  const deps={state,...helpers,api:(_url,options)=>{requests++;return new Promise(resolve=>{if(options.method==="POST")resolvePost=resolve;else resolveGet=resolve;});},renderFacade(){},toast:message=>{notice=message;},scheduleFacadeChallengeRetry(){},roots:{facade:{}},errorCard:()=>{throw Error("unexpected failure");}};
  const {loadFacade,applyFacade}=Function(...Object.keys(deps),`async ${functionSource(source,"loadFacade")}\nasync ${functionSource(source,"applyFacade")}\nreturn {loadFacade,applyFacade};`)(...Object.values(deps));
  const read=loadFacade(true,false,"manual");
  const write=applyFacade({action:"background",skinId:67004});
  await loadFacade(true,false,"manual");
  await applyFacade({action:"background",skinId:18080});
  assert.equal(requests,2,"no overlapping writes or reads during a mutation");
  resolvePost({profile:{backgroundSkinId:67004},skins:[]});await write;
  resolveGet({profile:{backgroundSkinId:21069},skins:[]});await read;
  assert.equal(state.facadeDraft.skinId,67004,"late read must not undo applied skin");
  const unconfirmed=applyFacade({action:"background",skinId:18080});
  resolvePost({profile:{backgroundSkinId:67004},skins:[]});await unconfirmed;
  assert.match(notice,/尚未确认/);
  assert.equal(state.facadeApplying,false);
});

test("career missing catalog never substitutes another champion and new undated MF skin stays first", () => {
  const state = { facade: { profile: { backgroundSkinId: 21069 }, skins: [{ id: 67004, championId: 67, owned: true }] } };
  const { hydrateFacadeDraft, facadeVisibleSkins } = compile(["hydrateFacadeDraft", "facadeVisibleSkins"], { state });
  hydrateFacadeDraft();
  assert.equal(state.facadeDraft.skinId, 21069);
  assert.deepEqual(facadeVisibleSkins(state.facade.skins, state.facadeDraft), []);
  const skins = [
    { id: 21000, championId: 21 }, { id: 21033, championId: 21, releaseDate: "2022-03-31" },
    { id: 21032, championId: 21, releaseDate: "2024-01-24" }, { id: 21060, championId: 21, releaseDate: "2025-04-01" },
    { id: 21069, championId: 21 },
  ];
  assert.deepEqual(facadeVisibleSkins(skins, { hero: "21" }).map(s => s.id), [21069, 21060, 21032, 21033, 21000]);
  assert.equal(skins.at(-1).releaseDate, undefined, "fallback must not invent a verified release date");
});
const checked = (value) => value ? " checked" : "";
const formatDelay = (value) => `${(Number(value || 0) / 1000).toFixed(Number(value || 0) % 1000 ? 1 : 0)} s`;

function railDragFixture() {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const dom = new JSDOM('<fieldset id="root"></fieldset>');
  const root = dom.window.document.querySelector("#root");
  const state = { champSelectLane: { pick: "middle" } };
  const settings = { enabled: true };
  const definition = { groupId: "ranked", positions: ["middle", "top", "default"] };
  const config = { ban: { champions: { default: [141, 121, 11, 104, 56] } }, pick: { champions: { middle: [5, 804], top: [99] } } };
  const saves = [];
  const helpers = compile(["bindChampSelectRailDrag", "champSelectPoolLane", "champSelectPoolFor", "champSelectSlotsHTML", "champSelectStatusCopy", "champSelectSideName"], {
    state, escapeHTML, champSelectSettings: () => settings, champSelectSelectedDefinition: () => definition, champSelectSelectedConfig: () => config,
    champSelectChampionMeta: (id) => ({ nameZh: `英雄${id}` }), champSelectChampionImage: () => '<img alt="英雄">', renderChampSelect() {}, saveChampSelect: async (message) => { saves.push(message); },
  });
  root.innerHTML = ["ban", "pick"].map((side) => helpers.champSelectSlotsHTML(side, helpers.champSelectPoolFor(side), 5, {}, {})).join("");
  helpers.bindChampSelectRailDrag(root);
  const row = (side, index) => root.querySelector(`[data-cs-rail-side="${side}"][data-cs-rail-index="${index}"]`);
  const fire = (element, name) => {
    const event = new dom.window.Event(name, { bubbles: true, cancelable: true });
    Object.defineProperty(event, "dataTransfer", { value: { setData() {}, getData: () => "arbitrary-external-data" } });
    element.dispatchEvent(event);
    return event;
  };
  return { dom, root, state, settings, definition, config, saves, row, fire };
}

test("1405 rail drag swaps exactly two champions and persists the active pool", async () => {
  for (const side of ["ban", "pick"]) {
    const f = railDragFixture();
    const pool = side === "ban" ? f.config.ban.champions.default : f.config.pick.champions.middle;
    const before = [...pool], last = pool.length - 1;
    assert.equal(f.root.querySelector("img").draggable, false, "drag a card, not an image URL");
    assert.equal(f.root.querySelector(".is-empty").hasAttribute("draggable"), false);
    f.fire(f.row(side, 0), "dragstart");
    assert.equal(f.fire(f.row(side, last), "dragover").defaultPrevented, true);
    assert.ok(f.row(side, last).classList.contains("is-drop-target"));
    f.fire(f.row(side, last), "drop");
    await new Promise(setImmediate);
    [before[0], before[last]] = [before[last], before[0]];
    assert.deepEqual(pool, before, "swap, not insertion/reordering of intervening champions");
    assert.equal(f.saves.length, 1);
    assert.deepEqual(f.config.pick.champions.top, [99]);
    assert.equal(f.state.champSelectRailDrag, null);
    f.dom.window.close();
  }
});

test("1405 invalid, canceled and stale rail drags never change or save settings", () => {
  for (const scenario of ["external", "cross-side", "same-slot", "off", "disabled", "group", "lane", "changed-pool", "expired", "cancel"]) {
    const f = railDragFixture();
    if (scenario !== "external") f.fire(f.row("pick", 0), "dragstart");
    if (scenario === "off") f.settings.enabled = false;
    if (scenario === "disabled") f.root.disabled = true;
    if (scenario === "group") f.definition.groupId = "normal";
    if (scenario === "lane") f.state.champSelectLane.pick = "top";
    if (scenario === "changed-pool") f.config.pick.champions.middle[0] = 99;
    if (scenario === "expired") f.state.champSelectRailDrag.expiresAt = 0;
    if (scenario === "cancel") f.fire(f.row("pick", 0), "dragend");
    const before = JSON.stringify(f.config);
    f.fire(f.row(scenario === "cross-side" ? "ban" : "pick", scenario === "same-slot" ? 0 : 1), "drop");
    assert.equal(JSON.stringify(f.config), before, scenario);
    assert.equal(f.saves.length, 0, scenario);
    assert.equal(f.state.champSelectRailDrag, null, scenario);
    f.dom.window.close();
  }
});

test("1405 time unit is inside the field and immutable while seconds remain editable", async () => {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const config = { ban: { delayMs: 2000 }, pick: { delayMs: 500 }, bench: { holdMs: 1000 } };
  const saved = [];
  const helpers = compile(["champSelectTimeInputHTML", "champSelectSideName", "bindChampSelectTimeInputs"], {
    champSelectSettings: () => ({ enabled: true }), champSelectSelectedConfig: () => config, toast() {}, saveChampSelect: async () => { saved.push(true); },
  });
  const dom = new JSDOM(["ban", "pick", "hold"].map((kind) => helpers.champSelectTimeInputHTML(kind, 1000)).join(""));
  const root = dom.window.document.body;
  helpers.bindChampSelectTimeInputs(root);
  for (const input of root.querySelectorAll("input")) {
    const field = input.closest(".cs-time-field");
    assert.equal(input.readOnly, false);
    assert.equal(input.type, "number");
    assert.equal(field.querySelector(".cs-time-unit").textContent, "s");
    input.value = "2.25";
    input.dispatchEvent(new dom.window.Event("change"));
    assert.equal(field.querySelector(".cs-time-unit").textContent, "s");
    input.value = "";
    input.dispatchEvent(new dom.window.Event("change"));
    assert.equal(input.value, "2.25", "empty input reverts without removing the unit");
  }
  await new Promise(setImmediate);
  assert.deepEqual([config.ban.delayMs, config.pick.delayMs, config.bench.holdMs], [2250, 2250, 2250]);
  assert.equal(saved.length, 3);
  dom.window.close();
});

test("manual takeover stays visible beneath the relinquished champion", () => {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const helpers = compile(["champSelectSideName", "champSelectChampionMeta", "champSelectChampionImage", "champSelectStatusCopy", "champSelectSlotsHTML"], { window: {}, escapeHTML });
  const html = helpers.champSelectSlotsHTML("pick", [5, 804], 3, { pickStates: { 5: "manual-takeover", 804: "available" } }, { champions: [] });
  const dom = new JSDOM(html);
  const first = dom.window.document.querySelector('[data-cs-champion-id="5"]');
  assert.equal(first.querySelector(".cs-champion-state").textContent, "玩家主动切换");
  assert.match(first.querySelector(".cs-champion-state").title, /本轮已停止自动操作.*下一局恢复/);
  assert.equal(first.classList.contains("is-live"), false);
  assert.equal(dom.window.document.querySelector('[data-cs-champion-id="804"] .cs-champion-state').textContent, "可选");
  dom.window.close();
});

test("pick show-then-lock edits a separate default ten-second lock wait", async () => {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const config = { pick: { enabled: true, strategy: "show-then-lock", delayMs: 500 }, ban: {}, bench: {} };
  const saved = [];
  const helpers = compile(["renderChampSelectSideCard", "champSelectTimeInputHTML", "champSelectSideName", "bindChampSelectTimeInputs"], {
    state: { champSelectCatalog: {} }, checked, champSelectPoolFor: () => [5], champSelectStrategyHTML: () => "", champSelectLaneTabsHTML: () => "", champSelectSlotsHTML: () => "",
    champSelectSettings: () => ({ enabled: true }), champSelectSelectedConfig: () => config, toast() {}, saveChampSelect: async () => saved.push(true),
  });
  for (const groupId of ["ranked", "normal", "arena"]) {
    const dom = new JSDOM(helpers.renderChampSelectSideCard("pick", { groupId, pickLimit: 5 }, config, {}));
    const input = dom.window.document.querySelector('[data-cs-time="lock"]');
    assert.equal(input.value, "10");
    assert.match(input.getAttribute("aria-label"), /亮出后锁定等待/);
    assert.equal(dom.window.document.querySelector("[data-cs-delay]").dataset.csDelayKey, "lockDelayMs");
    dom.window.close();
  }
  const dom = new JSDOM(helpers.champSelectTimeInputHTML("lock", 10000));
  helpers.bindChampSelectTimeInputs(dom.window.document.body);
  const input = dom.window.document.querySelector("input");
  input.value = "7.5";
  input.dispatchEvent(new dom.window.Event("change"));
  await new Promise(setImmediate);
  assert.equal(config.pick.lockDelayMs, 7500);
  assert.equal(config.pick.delayMs, 500, "changing lock wait must not delay initial hover");
  assert.equal(saved.length, 1);
  dom.window.close();
});

test("1505 timeline resolves champion names without changing delays, attempts or diagnostic IDs", () => {
  const catalog = { champions: [
    { id: 141, nameZh: "影流之镰", nameEn: "Kayn" },
    { id: 5, nameZh: "德邦总管" }, { id: 56, nameZh: "永恒梦魇" },
    { id: 804, nameZh: "不破之誓", nameEn: "Yunara" },
    { id: 999, nameEn: "Catalog English Name" },
  ] };
  const { champSelectRecordMessage: message } = compile(["champSelectRecordMessage", "champSelectChampionMeta"]);
  for (const [text, id, expected] of [
    ["跳过英雄 5：队友已经选择", 5, "跳过德邦总管：队友已经选择"],
    ["已排定：2.0 秒后禁用英雄 141", 141, "已排定：2.0 秒后禁用影流之镰"],
    ["已发送提前预选英雄 804，等待客户端确认", 804, "已发送提前预选不破之誓，等待客户端确认"],
    ["已确认亮出英雄 141", 141, "已确认亮出影流之镰"],
    ["已确认禁用并锁定英雄 141", 141, "已确认禁用并锁定影流之镰"],
    ["已确认选用并锁定英雄 804", 804, "已确认选用并锁定不破之誓"],
    ["已换取英雄 56", 56, "已换取永恒梦魇"],
    ["英雄 804 的请求未在客户端生效（第 1/2 次）", 804, "不破之誓 的请求未在客户端生效（第 1/2 次）"],
    ["已选用英雄 -3", -3, "已选用勇敢举动"],
    ["已亮出英雄 999", 999, "已亮出Catalog English Name"],
    ["已亮出英雄 123456", 123456, "已亮出未知英雄（资料暂缺）"],
    ["已顺延前 4 个不可用备选", undefined, "已顺延前 4 个不可用备选"],
  ]) {
    const record = Object.freeze({ message: text, championId: id });
    assert.equal(message(record, catalog), expected);
    assert.equal(record.message, text, "do not rewrite source/diagnostic message");
    assert.equal(record.championId, id);
  }
  assert.equal(message({ message: "跳过英雄 56：队友当前预选" }, catalog), "跳过永恒梦魇：队友当前预选", "legacy records without structured IDs");
  assert.equal(message({ message: "英雄 56", championId: 5 }, catalog), "英雄 56", "do not replace a different champion token");
  assert.equal(message({ message: "英雄 804" }, null), "未知英雄（资料暂缺）");
  assert.equal(message({ message: "英雄 804" }, catalog), "不破之誓", "late catalog supplies actual name on next render");
  assert.equal(message(null, null), "");
});

test("1505 timeline escapes catalog names and preserves reverse chronological order", () => {
  const state = { champSelectCatalog: { champions: [{ id: 141, nameZh: '<img src=x onerror="alert(1)">' }] } };
  const { renderChampSelectTimeline } = compile(["renderChampSelectTimeline", "champSelectRecordMessage", "champSelectChampionMeta"], { state, escapeHTML });
  const records = [
    { at: "2026-09-11T03:28:14Z", kind: "warn", message: "已发送禁用英雄 141，等待客户端确认", championId: 141 },
    { at: "2026-09-11T03:28:16Z", kind: "ok", message: "已确认禁用并锁定英雄 141", championId: 141 },
  ];
  const html = renderChampSelectTimeline({ records });
  assert.ok(html.indexOf("已确认") < html.indexOf("已发送"));
  assert.ok(html.includes(escapeHTML(state.champSelectCatalog.champions[0].nameZh)));
  assert.doesNotMatch(html, /<img|英雄 141/);
  assert.equal(records[0].kind, "warn");
});

test("1129 ranked ban has five shared slots while picks keep lane-specific pools", () => {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const state = { champSelectLane: { ban: "jungle", pick: "middle" }, champSelectCatalog: {} };
  const definition = { groupId: "ranked", name: "排位", positions: ["top", "jungle", "middle", "bottom", "utility", "default"], hasBan: true, banLimit: 5, pickLimit: 5 };
  const config = {
    ban: { enabled: true, champions: { default: [141, 104], jungle: [5, 11, 56, 121, 804] }, legacyLaneChampions: { top: [99] } },
    pick: { enabled: true, champions: { middle: [804], top: [5, 11], default: [99] } },
  };
  const helpers = compile(["champSelectPoolLane", "champSelectPoolFor", "champSelectConfiguredCount", "champSelectLaneTabsHTML", "champSelectLaneName", "champSelectSideName", "champSelectStatusCopy", "champSelectSlotsHTML", "renderChampSelectSideCard", "openChampSelectDialog"], {
    state, checked, escapeHTML, champSelectSelectedDefinition: () => definition, champSelectSelectedConfig: () => config,
    champSelectStrategyHTML: () => "", champSelectTimeInputHTML: () => "", champSelectChampionMeta: (id) => ({ nameZh: `英雄${id}` }), champSelectChampionImage: () => "", renderChampSelect: () => {},
  });
  const ban = new JSDOM(helpers.renderChampSelectSideCard("ban", definition, config, {}));
  assert.equal(ban.window.document.querySelectorAll(".cs-rail-slot").length, 5);
  assert.equal(ban.window.document.querySelectorAll("[data-cs-lane]").length, 0);
  assert.match(ban.window.document.body.textContent, /所有位置共用 5 个禁用备选/);
  assert.equal(helpers.champSelectConfiguredCount(config, "ban"), 2, "count must ignore old/backup pools");
  assert.deepEqual(helpers.champSelectPoolFor("ban"), [141, 104]);
  assert.deepEqual(helpers.champSelectPoolFor("pick"), [804]);
  state.champSelectLane.pick = "top";
  assert.deepEqual(helpers.champSelectPoolFor("pick"), [5, 11]);
  assert.deepEqual(helpers.champSelectPoolFor("ban"), [141, 104]);
  const pick = new JSDOM(helpers.renderChampSelectSideCard("pick", definition, config, {}));
  assert.equal(pick.window.document.querySelectorAll("[data-cs-lane]").length, 6);
  helpers.openChampSelectDialog("ban");
  assert.equal(state.champSelectDialog.lane, "default");
  assert.deepEqual(state.champSelectDialog.draft, [141, 104]);
  state.champSelectDialog.draft.splice(0, 1);
  assert.deepEqual(config.ban.champions.default, [141, 104], "dialog cancel must not edit live pool");
  // This is also the pool used by the remove-button handler.
  helpers.champSelectPoolFor("ban").splice(0, 1);
  assert.deepEqual(config.ban.champions.default, [104]);
  assert.deepEqual(config.pick.champions.top, [5, 11]);
  ban.window.close(); pick.window.close();
});

test("1129 ban dialog saves and clears only shared pool without dropping migration backup", async () => {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const definition = { groupId: "ranked", name: "排位", positions: ["top", "middle", "default"], banLimit: 5, pickLimit: 5 };
  const backup = { top: [11, 56] };
  const config = { ban: { champions: { default: [141] }, legacyLaneChampions: backup }, pick: { champions: { middle: [804] } } };
  const state = { champSelectGroups: [definition], champSelectDialog: { group: "ranked", side: "ban", lane: "default", draft: [141], query: "" }, champSelectPosition: "all" };
  let saved = 0;
  const dom = new JSDOM('<div id="root"></div>');
  const root = dom.window.document.querySelector("#root");
  const helpers = compile(["renderChampSelectDialog", "bindChampSelectDialog", "champSelectSideName", "champSelectLaneName"], {
    state, roots: { champselect: root }, escapeHTML, champSelectSelectedDefinition: () => definition, champSelectSelectedConfig: () => config,
    champSelectFilteredChampions: () => [], champSelectChampionMeta: () => ({ nameZh: "影流之镰" }), champSelectChampionImage: () => "", updateChampSelectDialog: () => {}, closeChampSelectDialog: () => { state.champSelectDialog = null; }, champSelectSettings: () => ({ groups: { ranked: config } }), saveChampSelect: async () => { saved++; },
  });
  root.innerHTML = helpers.renderChampSelectDialog();
  assert.equal(root.querySelector("h3").textContent, "编辑禁用序列 · 排位");
  assert.ok(root.querySelector("header [data-cs-dialog-close].icon-button.control-icon-button svg.control-icon"));
  assert.equal(root.querySelector("header [data-cs-dialog-close]").textContent, "");
  helpers.bindChampSelectDialog();
  root.querySelector("[data-cs-dialog-clear]").click();
  root.querySelector("[data-cs-dialog-save]").click();
  await new Promise(setImmediate);
  assert.equal(saved, 1);
  assert.deepEqual(config.ban.champions.default, []);
  assert.deepEqual(config.ban.legacyLaneChampions, backup);
  assert.deepEqual(config.pick.champions.middle, [804]);
  dom.window.close();
});

test("0911 preselection follows auto-pick without an extra toggle and distinguishes planning", () => {
  const { renderChampSelectSideCard, champSelectStatusCopy } = compile(["renderChampSelectSideCard", "champSelectStatusCopy"], {
    state: { champSelectCatalog: {} }, checked, champSelectSideName: (side) => side === "ban" ? "禁用" : "选用",
    champSelectPoolFor: () => [804], champSelectStrategyHTML: () => "", champSelectTimeInputHTML: () => "",
    champSelectLaneTabsHTML: () => "", champSelectSlotsHTML: () => "",
  });
  const definition = { groupId: "practice", hasBan: true, pickLimit: 5, banLimit: 5 };
  const config = { ban: { enabled: true }, pick: { enabled: true } };
  const runtime = { active: true, groupId: "practice", actionType: "pick", pickIntent: true };
  const pick = renderChampSelectSideCard("pick", definition, config, runtime);
  assert.match(pick, /预选阶段，自动提前亮出序列英雄/);
  assert.equal((pick.match(/type="checkbox"/g) || []).length, 1, "only existing auto-pick enable switch");
  assert.doesNotMatch(pick, /data-cs-(show-intent|preselect)/);
  assert.match(renderChampSelectSideCard("ban", definition, config, runtime), /等待正式禁用阶段/);
  config.pick.enabled = false;
  assert.match(renderChampSelectSideCard("pick", definition, config, runtime), /自动选用未开启/);
  assert.equal(champSelectStatusCopy("available", "pick", true, true), "提前预选中");
  assert.equal(champSelectStatusCopy("verify-hover", "ban", false), "待亮出验证");
  assert.equal(champSelectStatusCopy("own-intent", "ban", false), "自己准备选用");
});

test("2351 custom pause retains enabled highlighting and never reports master off", () => {
  const state = { watch: { customPaused: true }, phase: "Lobby", watchEvents: new Map() };
  const { watchRuleCard } = compile(["watchRuleCard", "watchConflictNote"], { state, checked, watchRuleControl: () => "" });
  const definition = { action: "auto-accept", key: "autoAccept", title: "自动接受", phase: "ReadyCheck" };
  const enabled = watchRuleCard(definition, { enabled: true }, true);
  assert.match(enabled, /watch-rule is-enabled/);
  assert.match(enabled, /自定义对局已暂停/);
  assert.doesNotMatch(enabled, /is-paused|总开关已关闭/);
  const disabled = watchRuleCard(definition, { enabled: true }, false);
  assert.match(disabled, /is-paused/);
  assert.match(disabled, /总开关已关闭/);
  assert.doesNotMatch(functionSource(source, "renderWatch"), /watchRuleCard\([^\n]*masterEnabled &&/);
});

test("2351 teammate picks and empty client ban list have distinct explanations", () => {
  const { champSelectStatusCopy } = compile(["champSelectStatusCopy"]);
  assert.equal(champSelectStatusCopy("teammate-picked", "pick", false), "队友已经选择");
  assert.equal(champSelectStatusCopy("list-empty", "ban", false), "等待禁用列表");
});

test("1412 settings saves serialize and stale GET never undoes a ban toggle", async () => {
  const state = { watch: { schemaVersion: 3, champSelect: { enabled: true, groups: { practice: { ban: { enabled: false } } } } } };
  let finishGet;
  const posts = [];
  const api = (_url, init) => {
    if (!init) return new Promise((resolve) => { finishGet = resolve; });
    return new Promise((resolve) => { posts.push({ payload: JSON.parse(init.body), resolve }); });
  };
  const helpers = Function("state", "api", "renderWatch", "roots", "errorCard",
    `async ${functionSource(source, "loadWatch")}
     async ${functionSource(source, "persistWatchSettings")}
     ${functionSource(source, "watchSettingsPayload")}
     return {loadWatch, persistWatchSettings};`)(state, api, () => {}, { watch: {} }, () => "");
  const stale = JSON.parse(JSON.stringify(state.watch));
  const read = helpers.loadWatch(true);
  state.watch.champSelect.groups.practice.ban.enabled = true;
  const first = helpers.persistWatchSettings();
  await new Promise(setImmediate);
  state.watch.champSelect.groups.practice.ban.delayMs = 2000;
  const second = helpers.persistWatchSettings();
  finishGet(stale);
  await read;
  assert.equal(state.watch.champSelect.groups.practice.ban.enabled, true, "late GET erased toggle");
  assert.equal(posts.length, 1, "POSTs raced each other");
  posts[0].resolve(posts[0].payload);
  await first;
  await new Promise(setImmediate);
  assert.equal(posts.length, 2);
  assert.equal(state.watch.champSelect.groups.practice.ban.delayMs, 2000, "old save response erased later edit");
  posts[1].resolve(posts[1].payload);
  await second;
  assert.equal(state.watch.champSelect.groups.practice.ban.enabled, true);
  assert.equal(state.watch.champSelect.groups.practice.ban.delayMs, 2000);
  assert.equal(state.watchPendingSaves, 0);
});

test("1412 enabled master keeps cards bright without hover", () => {
  const css = fs.readFileSync(path.join(__dirname, "suite.css"), "utf8");
  assert.doesNotMatch(css, /\.cs-seq-card\.is-off[^{}]*\{/);
  assert.match(css, /\.cs-config-fields:disabled\s*\{[^}]*opacity/);
});

test("2143 capabilities only show supported highlighted tags and avoidance is unified", () => {
  const { champSelectCapabilitiesHTML } = compile(["champSelectCapabilitiesHTML"]);
  const ranked = champSelectCapabilitiesHTML({ hasBan: true, positions: ["top", "default"], hasBench: false });
  assert.match(ranked, /is-success">有禁用环节/);
  assert.match(ranked, /is-success">选用按分路/);
  assert.doesNotMatch(ranked, /无备战席|上限|共用默认池/);
  const aram = champSelectCapabilitiesHTML({ hasBan: false, positions: ["default"], hasBench: true });
  assert.equal(aram, '<span class="suite-chip is-success">有备战席</span>');
  assert.doesNotMatch(functionSource(source, "renderChampSelectSideCard"), /data-cs-avoid|避让队友预选/);
  assert.equal((functionSource(source, "renderChampSelect").match(/data-cs-avoid/g) || []).length, 1);
  const binding = functionSource(source, "bindChampSelectControls");
  assert.match(binding, /config.ban.avoidTeammateIntent = input.checked/);
  assert.match(binding, /config.pick.avoidTeammateIntent = input.checked/);
});

test('R74 missing skin catalog preserves the actual background ID, not an unset claim',()=>{
 const state={facade:{skins:[],profile:{backgroundSkinId:99042}}};
 const {hydrateFacadeDraft}=compile(['hydrateFacadeDraft'],{state});
 hydrateFacadeDraft();
 assert.equal(state.facadeDraft.skinId,99042);
 assert.equal(state.facadeDraft.hero,'99');
});

test('R74 missing career catalog renders a retry state across the grid',()=>{
 const state={facade:{skinsUnavailable:true}};
 const {facadeFilmHTML}=compile(['facadeFilmHTML'],{state,escapeHTML,imageURL:x=>x});
 const html=facadeFilmHTML([],{skinId:0});
 assert.match(html,/data-facade-catalog-retry/);
 assert.match(html,/facade-catalog-empty/);
 assert.doesNotMatch(html,/没有符合筛选/);
});

test('R74 successful career mutations render their response, including a missing catalog',async()=>{
 for(const name of ['applyFacade','applyFacadeIdentity']) for(const unavailable of [false,true]) {
  const state={facadeDraft:{availability:'chat',statusMessage:'fixture'}};
  let renders=0,hydrates=0,message='';
  const apply=Function('state','api','hydrateFacadeDraft','renderFacade','toast',`async ${functionSource(source,name)}; return ${name};`)(
   state,async()=>({skinsUnavailable:unavailable,profile:{backgroundSkinId:1000},chat:{availability:'chat'}}),()=>hydrates++,()=>renders++,value=>{message=value;});
  await apply({action:'background',skinId:1000});
  assert.equal(renders,1,`${name}: ${message}`);
  assert.equal(hydrates,1);
  assert.match(message,/已应用/);
  assert.equal(state.facadeLoadedAt===0,unavailable);
 }
});

test("fixed play-again delay shows only the wait time without a progress meter", () => {
  const { watchRuleControl } = compile(["watchRuleControl"], { escapeHTML, formatDelay });
  const html = watchRuleControl({ key: "autoPlayAgain", title: "快速下一把", control: "fixed-delay", displayDelayMs: 1575 }, { enabled: true });
  assert.doesNotMatch(html, /type="range"/);
  assert.doesNotMatch(html, /watch-delay-meter|data-watch-meter/);
  assert.match(html, /watch-delay-fixed-note/);
  assert.match(html, /需等待[\s\S]*1\.6 秒/);
  assert.match(html, /后可开始下一把/);
});

test("R68 matchmaking conflict note appears only when both rules are enabled", () => {
  const { watchConflictNote } = compile(["watchConflictNote"]);
  const definition = { key: "autoMatchmaking" };
  assert.match(watchConflictNote(definition, { promoteLeader: { enabled: true }, autoMatchmaking: { enabled: true } }), /watch-conflict-note/);
  assert.equal(watchConflictNote(definition, { promoteLeader: { enabled: false }, autoMatchmaking: { enabled: true } }), "");
  assert.equal(watchConflictNote(definition, { promoteLeader: { enabled: true }, autoMatchmaking: { enabled: false } }), "");
});

test("claim scan artifacts and failures stay out of the default reward list", () => {
  const state = {
    claimFilter: "all", claimFailures: new Map([["grant:failed", { message: "failed" }]]), selectedClaims: new Set(), claimChoices: new Map(), claiming: false,
  };
  const helpers = compile(["claimSelectionKeys", "claimHasFailure", "claimVisible", "claimActionable", "choiceValid", "selectableClaimKeys"], { state });
  const informational = { key: "event:empty", source: "event", title: "未领取活动奖励", items: [], actionable: false, detail: "明细不可解析" };
  const actionable = { key: "grant:one", source: "grant", title: "奖励", items: [{}], actionable: true };
  const failed = { key: "grant:failed", source: "grant", title: "失败奖励", items: [{}], actionable: true };
  assert.equal(helpers.claimVisible(informational), false);
  assert.equal(helpers.claimVisible(failed), false);
  assert.equal(helpers.claimVisible(failed, "failed"), true);
  assert.deepEqual(helpers.selectableClaimKeys([informational, actionable, failed]), ["grant:one"]);
});

test("independent grants stay independent even when their item type and display group match", () => {
  const { claimSelectionKeys, claimActionable, claimPresentationItems } = compile(["claimSelectionKeys", "claimActionable", "claimPresentationItems"]);
  const grants = [
    { key: "grant:orange", source: "grant", displayGroup: "pass", title: "通行证奖励", items: [{ id: "orange", quantity: 25 }], actionable: true },
    { key: "grant:blue", source: "grant", displayGroup: "pass", title: "通行证奖励", items: [{ id: "blue", quantity: 750 }], actionable: true },
    { key: "grant:honor", source: "grant", title: "荣誉奖励", items: [{ id: "honor" }], actionable: true },
  ];
  const rows = claimPresentationItems(grants);
  assert.equal(rows.length, 3);
  assert.equal(rows[0].title, "通行证奖励");
  assert.deepEqual(rows[0].claimKeys, ["grant:orange"]);
  assert.deepEqual(rows[0].items.map((item) => item.id), ["orange"]);
  assert.deepEqual(claimSelectionKeys(rows[1]), ["grant:blue"]);
  assert.deepEqual(claimSelectionKeys(rows[2]), ["grant:honor"]);
  assert.doesNotMatch(source, /未命名奖励组/);
});

test("R68 canceled watch event clears the armed countdown state", () => {
  const state = { watchPriority: false, watchEvents: new Map(), watchFired: 0 };
  const { handleWatchEvent } = compile(["handleWatchEvent"], {
    state, watchDefinitions: [], renderWatch: () => {}, toast: () => {},
  });
  handleWatchEvent("watch:armed:reconnect:5000");
  assert.equal(state.watchEvents.get("reconnect").kind, "armed");
  handleWatchEvent("watch:canceled:reconnect");
  assert.equal(state.watchEvents.get("reconnect").kind, "canceled");
});

test("R68 suite mutation probes execute the real helper logic", () => {
  const noFixedNote = source.replace('<span class="watch-delay-fixed-note">后可开始下一把</span>', "");
  const { watchRuleControl } = compile(["watchRuleControl"], { escapeHTML, formatDelay }, noFixedNote);
  assert.throws(() => assert.match(watchRuleControl({ key: "autoPlayAgain", title: "快速下一把", control: "fixed-delay", displayDelayMs: 1575 }, {}), /watch-delay-fixed-note/));

  const selectsEverything = source.replace("failed || !claimActionable(item)", "failed").replace("claimActionable(item) && choiceValid(item)", "choiceValid(item)").replace("if (!claimActionable(item)) return false;", "");
  const state = { claimFilter: "all", claimChoices: new Map(), claimFailures: new Map() };
  const helpers = compile(["claimSelectionKeys", "claimHasFailure", "claimVisible", "claimActionable", "choiceValid", "selectableClaimKeys"], { state }, selectsEverything);
  assert.throws(() => assert.deepEqual(helpers.selectableClaimKeys([{ key: "event:empty", actionable: false }]), []));
});

test("R69 facade load diagnostics distinguish poll, SSE, and manual retries", () => {
  const load = functionSource(source, "loadFacade");
  const refresh = functionSource(source, "refreshFacadeFromEvent");
  assert.match(load, /trigger = "poll"/);
  assert.match(load, /\/api\/facade\/state\?trigger=\$\{encodeURIComponent\(loadTrigger\)\}/);
  assert.match(refresh, /loadFacade\(true, preserveDraft, "sse"\)/);
  assert.match(source, /facade: \(\) => loadFacade\(true, false, "manual"\)/);
});

test('career skins retain dates and quest tiers without sinking new undated skins',()=>{
 const {facadeVisibleSkins}=compile(['facadeVisibleSkins']);
 const skins=[{id:99,championId:1},{id:3,championId:1,releaseDate:'2025-01-01'},
  {id:2,championId:1,releaseDate:'2024-01-01',owned:true},
  {id:4,championId:1,parentSkinId:2,isVariant:true,releaseSortDate:'2024-01-01'}];
 assert.deepEqual(facadeVisibleSkins(skins,{hero:'1'}).map(s=>s.id),[99,3,2,4]);
 assert.deepEqual(facadeVisibleSkins(skins,{hero:'1',ownedOnly:true}).map(s=>s.id),[2]);
 assert.equal(skins[0].id,99,'sorting must not mutate source');
});

test('Ashe verified 2026 release precedes Spirit Blossom and base skin',()=>{
 const {facadeVisibleSkins}=compile(['facadeVisibleSkins']);
 const dates=JSON.parse(fs.readFileSync(path.join(__dirname,'../data/skin_release_dates.json'),'utf8')).dates;
 for(const row of JSON.parse(fs.readFileSync(path.join(__dirname,'../data/skin_release_overrides.json'),'utf8')).records) dates[row.id]=row.date;
 const skins=[22000,22076,22084,22067].map(id=>({id,championId:22,releaseDate:dates[id]||''}));
 assert.deepEqual(facadeVisibleSkins(skins,{hero:'22'}).map(s=>s.id),[22084,22076,22067,22000]);
});

test("suite champselect tab after maintenance activates the real panel", () => {
  const makeClassList = () => ({ toggle() {} });
  const tabs = ["watch", "rig", "champselect", "facade", "sweep"].map((name) => ({ dataset: { suiteTab: name }, classList: makeClassList(), setAttribute() {}, focus() {} }));
  const panels = ["watch", "rig", "facade", "sweep", "champselect"].map((name) => ({ dataset: { suitePanel: name }, hidden: false }));
  const state = { tab: "watch", scroll: {}, connected: true, active: false };
  const standardMetrics = { hidden: false };
  const champSelectMetrics = { hidden: true };
  const { activateTab } = compile(["activateTab"], {
    tabCopy: { watch: "自动", rig: "维护", facade: "生涯", sweep: "领奖", champselect: "征召" }, state, appScroll: null,
    writePreference() {}, tabs, panels, standardMetrics, champSelectMetrics, subtitle: null, requestAnimationFrame(callback) { callback(); }, loadFacade() {}, loadChampSelect() {},
  });
  activateTab("champselect");
  assert.equal(state.tab, "champselect");
  assert.equal(panels.find((item) => item.dataset.suitePanel === "champselect").hidden, false);
  assert.equal(panels.find((item) => item.dataset.suitePanel === "watch").hidden, true);
  assert.equal(standardMetrics.hidden, true);
  assert.equal(champSelectMetrics.hidden, false);
});

test("R78 conveyor renders one fixed ghost slot when four of five are configured", () => {
  const window = { deepLegendsChampionAsset: { imageHTML: (_asset, className) => `<span class="${className}"></span>` } };
  const functions = compile(["champSelectSideName", "champSelectChampionMeta", "champSelectChampionImage", "champSelectStatusCopy", "champSelectSlotsHTML"], { window, escapeHTML });
  const catalog = { champions: [1, 2, 3, 4].map((id) => ({ id, nameZh: `英雄${id}`, imageSource: "client", imagePath: `/champions/${id}.png` })) };
  const html = functions.champSelectSlotsHTML("pick", [1, 2, 3, 4], 5, { pickStates: {} }, catalog);
  assert.equal((html.match(/cs-rail-slot/g) || []).length, 5);
  assert.equal((html.match(/is-empty/g) || []).length, 1);
  assert.equal((html.match(/cs-rail-arrow/g) || []).length, 4);
});

test("R78 watch persistence includes champSelect without dropping existing settings", () => {
  const state = { watch: { schemaVersion: 3, masterEnabled: true, rules: { autoAccept: { enabled: true } }, facade: { statusMessage: "fixture" }, champSelect: { enabled: true, groups: { ranked: {} } } } };
  const { watchSettingsPayload } = compile(["watchSettingsPayload"], { state });
  assert.deepEqual(watchSettingsPayload(), state.watch);
});

test("R78 UI keeps CSP-safe markup and container responsiveness", () => {
  const r78Source = source.slice(source.indexOf("function champSelectLaneName"), source.indexOf("function handleWatchEvent"));
  assert.doesNotMatch(r78Source, /style\s*=/i);
  const css = fs.readFileSync(path.join(__dirname, "suite.css"), "utf8");
  assert.match(css, /@container suite-page \(max-width: 900px\)[\s\S]*\.cs-body/);
});

function assertR78TradeControls(script = source) {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const { renderChampSelectBenchCard } = compile(["renderChampSelectBenchCard", "champSelectTimeInputHTML", "champSelectSideName"], { checked, formatDelay }, script);
  for (const groupId of ["ranked", "normal", "practice", "aram", "event", "arena"]) {
    const hasBench = groupId === "aram";
    const hasTrade = groupId !== "arena";
    const dom = new JSDOM(renderChampSelectBenchCard({ groupId, hasBench, hasTrade }, { bench: { handleTrade: true } }, {}));
    const doc = dom.window.document;
    assert.equal(Boolean(doc.querySelector("[data-cs-bench-enabled]")), hasBench, groupId);
    assert.equal(Boolean(doc.querySelector(".cs-bench-card")), hasBench || hasTrade, groupId);
    if (hasBench || hasTrade) {
      assert.equal(doc.querySelector("[data-cs-bench-prefer]").disabled, false, groupId);
      assert.equal(doc.querySelector("[data-cs-bench-trade]").disabled, !hasTrade, groupId);
    }
    dom.window.close();
  }
}

test("R78 exchange checkbox is independent from bench controls in every mode", () => {
  assertR78TradeControls();
});

test("R78 exchange control regression probe rejects the old bench capability gate", () => {
  const mutant = source.replace("const tradeApplicable = Boolean(definition?.hasTrade);", "const tradeApplicable = Boolean(definition?.hasBench);");
  assert.notEqual(mutant, source);
  assert.throws(() => assertR78TradeControls(mutant), assert.AssertionError);
});

test("R78 ranked pick renders an editable sixth unassigned-position pool", () => {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const helpers = compile(["champSelectLaneTabsHTML", "champSelectLaneName", "champSelectSideName"], {
    state: { champSelectLane: { pick: "default" } },
  });
  const html = helpers.champSelectLaneTabsHTML("pick", {
    positions: ["top", "jungle", "middle", "bottom", "utility", "default"],
  }, { pick: { champions: { default: [9] } } });
  const dom = new JSDOM(html);
  const tabs = dom.window.document.querySelectorAll('[role="tab"]');
  assert.equal(tabs.length, 6);
  assert.equal(tabs[5].dataset.csLane, "default");
  assert.equal(tabs[5].getAttribute("aria-selected"), "true");
  assert.equal(tabs[5].disabled, false);
  assert.match(tabs[5].textContent, /未分配位置\s+1/);
  dom.window.close();
});

test("R99 icon dialog is body mounted, sliced, CSP safe, and restores focus on every exit", async () => {
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const script = process.env.R99_SUITE_SOURCE ? fs.readFileSync(process.env.R99_SUITE_SOURCE, "utf8") : source;
  for (const method of ["escape", "backdrop", "button", "native", "apply"]) {
    const dom = new JSDOM('<main id="facade"><button data-facade-icons>头像</button></main>', { pretendToBeVisual: true });
    const { window } = dom, { document } = window;
    window.HTMLDialogElement.prototype.showModal = function () { this.open = true; };
    window.HTMLDialogElement.prototype.close = function () { this.open = false; this.dispatchEvent(new window.Event("close")); };
    const frames = [], modal = [], root = document.getElementById("facade"), opener = root.querySelector("button");
    window.desktopTheme = { setModalOpen: value => modal.push(value) };
    const state = { facade: { connected: true, summoner: { profileIconId: 1 } }, facadeDraft: {} };
    let requests = 0;
    const icons = Array.from({ length: 500 }, (_, i) => ({ id: i + 1, title: `头像 <${i}>`, year: 2026, disabled: i === 3, owned: true, sets: ["系列"], searchTerms: ["touxiang"] }));
    const deps = { state, document, window, roots: { facade: root }, imageURL: value => `/api/image?path=${encodeURIComponent(value)}`, escapeHTML, requestAnimationFrame: callback => frames.push(callback), performance, api: async () => { requests++; return { icons, iconOwnershipUnavailable: false }; }, applyFacade: async request => {
      assert.deepEqual(request, { action: "icon", iconId: 1 });
      state.facade.summoner.profileIconId = request.iconId;
      root.innerHTML = '<button data-facade-icons>新触发按钮</button>';
      return true;
    } };
    const helpers = Function(...Object.keys(deps), `${functionSource(script,"facadeIconImage")}\nasync ${functionSource(script,"openFacadeIconPicker")}\nreturn {openFacadeIconPicker};`)(...Object.values(deps));
    opener.focus(); await helpers.openFacadeIconPicker(opener);
    const dialog = document.querySelector("dialog");
    assert.equal(dialog.parentElement, document.body);
    assert.equal(root.contains(dialog), false);
    assert.equal(dialog.querySelectorAll("[style]").length, 0);
    const firstCount = dialog.querySelectorAll("[data-picker-icon]").length;
    assert.ok(firstCount > 0 && firstCount <= 72, `first-frame count ${firstCount}`);
    assert.equal(dialog.querySelector("[data-picker-owned]"), null);
    while (frames.length) frames.shift()();
    assert.equal(dialog.querySelector('[data-picker-icon="4"]').disabled, false);
    // A facade rerender must leave the modal subtree alive.
    const grid = dialog.querySelector("[data-picker-grid]");
    root.append(document.createElement("span"));
    assert.equal(dialog.querySelector("[data-picker-grid]"), grid);
    if (method === "escape") dialog.dispatchEvent(new window.KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
    if (method === "backdrop") dialog.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    if (method === "button") dialog.querySelector("[data-picker-close]").click();
    if (method === "native") dialog.close();
    if (method === "apply") { dialog.querySelector("[data-picker-apply]").click(); await new Promise(setImmediate); }
    for (const frame of frames.splice(0)) frame();
    assert.equal(document.querySelector("dialog"), null);
    assert.equal(document.activeElement, method === "apply" ? root.querySelector("[data-facade-icons]") : opener, `${method} did not restore focus`);
    assert.deepEqual(modal, [true, false]);
    await helpers.openFacadeIconPicker(opener);
    assert.equal(requests, 1, "catalog should be lazy and cached across opens");
    document.querySelector("dialog").close();
    dom.window.close();
  }
});

test("R99 disconnect closes the picker and invalidates the catalog", () => {
  let closed = 0;
  const state = { facadeIconCatalog: { icons: [1] }, facadeIconDialog: { close() { closed++; } }, connected: true };
  const { setConnected } = compile(["setConnected"], { state, offline: {}, panels: [], metrics: { rig: {}, claim: {} } });
  setConnected(false);
  assert.equal(closed, 1);
  assert.equal(state.facadeIconCatalog, null);
});
