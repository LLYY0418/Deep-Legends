"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("jsdom");
const source = fs.readFileSync(path.join(__dirname, "../backend/web/champions.js"), "utf8");

function workspace(stored = {}) {
  const dom = new JSDOM('<div id="champions-root"></div><div id="champions-panel"></div>', {url:"http://localhost/", runScripts:"outside-only", pretendToBeVisual:true});
  const w = dom.window;
  for (const [key, value] of Object.entries(stored)) w.localStorage.setItem(key, value);
  w.eval(fs.readFileSync(path.join(__dirname, "../backend/web/runtime.js"), "utf8"));
  w.eval(source.replace('  render();\n  beginStartupPreload();\n  adoptCatalog(state.preload?.catalog);', `
    render = () => {};
    loadWorkspace = async () => {};
    api = async () => ({position:"top", build:{}});
    championMeta = id => ({slug:"champion"+id});
    selectArenaChampion = row => {state.selected = row};
    selectMayhemChampion = row => {state.selected = row};
    window.testWorkspace = {state, openDetail, restorePersistedChampionSelection, renderMayhemRarityPanel, loadMayhemRarity, loadRankings, switchChampionMode, setAPI: value => {api=value}};
  `));
  return dom;
}

test("2024 real mode buttons isolate champion and search memories across all modes and reload", async () => {
  const dom = workspace();
  const w = dom.window, api = w.testWorkspace, s = api.state;
  const root = w.document.querySelector("#champions-root");
  const switchMode = mode => {
    root.innerHTML = `<button data-champion-mode="${mode}">${mode}</button>`;
    root.firstElementChild.click();
  };
  const search = value => {
    root.innerHTML = '<input data-champion-search>';
    root.firstElementChild.value = value;
    root.firstElementChild.dispatchEvent(new w.Event("input", {bubbles:true}));
  };
  try {
    for (const [mode, id, query] of [["ranked",103,"阿狸"],["aram-mayhem",81,"伊泽"],["arena",50,"斯维因"]]) {
      switchMode(mode);
      assert.equal(s.detailChampionID, 0);
      assert.equal(s.query, "");
      await api.openDetail({championId:id, key:"champion"+id, position:"top"});
      search(query);
    }
    for (const [mode, id, query] of [["ranked",103,"阿狸"],["aram-mayhem",81,"伊泽"],["arena",50,"斯维因"]]) {
      switchMode(mode);
      assert.equal(s.selected, null, "previous-mode runtime selection must be reset");
      assert.equal(s.detailChampionID, id);
      assert.equal(s.query, query);
      s.rankings = {rows:[{championId:103},{championId:81},{championId:50}]};
      assert.equal(api.restorePersistedChampionSelection().championId, id);
    }
    const stored = Object.fromEntries(Object.keys(w.localStorage).map(key => [key,w.localStorage.getItem(key)]));
    const reloaded = workspace(stored);
    assert.equal(reloaded.window.testWorkspace.state.detailChampionID, 50);
    assert.equal(reloaded.window.testWorkspace.state.query, "斯维因");
    reloaded.window.close();
  } finally { w.close(); }
});

test("2024 legacy memory migrates only to its saved mode, once", () => {
  const dom = workspace({"lol-loot-champion-mode":"aram-mayhem", "lol-loot-champion-detail-id":"81", "lol-loot-champion-query":"伊泽"});
  try {
    const w = dom.window;
    assert.equal(w.testWorkspace.state.detailChampionID,81);
    assert.equal(w.localStorage.getItem("lol-loot-champion-detail-id-ranked"),null);
    assert.equal(w.localStorage.getItem("lol-loot-champion-detail-id-arena"),null);
    assert.equal(w.localStorage.getItem("lol-loot-champion-mode-preferences-v2"),"1");
  } finally { dom.window.close(); }
});

test("2024 fast ranked filters supersede requests without stuck loading or cross-mode writes", async () => {
  const dom = workspace();
  try {
    const api = dom.window.testWorkspace, s = api.state, pending = [];
    s.catalog = {};
    api.setAPI(() => new Promise((resolve,reject)=>pending.push({resolve,reject})));
    const first = api.loadRankings();
    s.position = "mid";
    const second = api.loadRankings();
    assert.equal(pending.length,2,"new filter must not be blocked by old loading state");
    pending[1].resolve({rows:[{championId:103}]}); await second;
    pending[0].reject(new Error("old request cancelled")); await first;
    assert.equal(s.loading,false);
    assert.equal(s.error,"");
    assert.equal(s.rankings.rows[0].championId,103);
    const third = api.loadRankings();
    api.switchChampionMode("arena");
    const arenaRankings = {rows:[{championId:50}]};
    s.rankings = arenaRankings;
    pending[2].resolve({rows:[{championId:13}]}); await third;
    assert.equal(s.rankings,arenaRankings,"late ranked response must not replace arena rankings");
  } finally { dom.window.close(); }
});

test("2024 neutral glyph coloring preserves alpha and never recolors colored/opaque/unknown art", () => {
  const dom = new JSDOM("",{runScripts:"outside-only"});
  try {
    dom.window.eval(fs.readFileSync(path.join(__dirname,"../backend/web/augment-artwork.js"),"utf8"));
    const {tintPixels} = dom.window.deepLegendsAugmentArtwork;
    const pixels = new Uint8ClampedArray(10*10*4);
    for(let i=0;i<60;i++) pixels.set([120,145,149,255],i*4);
    const golden = pixels.slice();
    assert.equal(tintPixels(golden,10,10,"gold"),true);
    assert.ok(golden[0]>golden[1] && golden[1]>golden[2]);
    for(let i=3;i<pixels.length;i+=4) assert.equal(golden[i],pixels[i]);
    const prismatic = pixels.slice();
    assert.equal(tintPixels(prismatic,10,10,"prismatic"),true);
    assert.ok(prismatic[0]<prismatic[2]);
    const colored = pixels.slice(); colored.set([255,20,80,255],0);
    const original = colored.slice();
    assert.equal(tintPixels(colored,10,10,"gold"),false);
    assert.deepEqual(colored,original);
    assert.equal(tintPixels(pixels.slice(),10,10,"unknown"),false);
    const opaque = new Uint8ClampedArray(400).fill(200);
    assert.equal(tintPixels(opaque,10,10,"gold"),false);
  } finally { dom.window.close(); }
});

test("2024 rarity UI says selection distribution, retains all stages, hides failed empty data", () => {
  const dom = workspace();
  try {
    const {state, renderMayhemRarityPanel:render} = dom.window.testWorkspace;
    assert.match(render(),/不代表抽取、刷新或保底概率/);
    state.mayhemRarityData = {stages:[1,2,3,4].map(stage=>({stage,silver:10,gold:60,prismatic:30,games:1000}))};
    const html = render();
    assert.equal((html.match(/次选择/g)||[]).length,4);
    assert.doesNotMatch(html,/海克斯品质概率|查看四阶段/);
    state.mayhemRarityData=null; state.mayhemRarityError="invalid source";
    assert.equal(render(),"");
  } finally { dom.window.close(); }
});

test("R86 search keeps the input and exposes all-position request failures", async () => {
  const dom = workspace();
  try {
    const w = dom.window, api = w.testWorkspace, s = api.state;
    s.catalog = {};
    s.rankings = { rows: [{ championId: 103, name: "阿狸" }] };
    s.position = "all";
    const root = w.document.getElementById("champions-root");
    root.innerHTML = '<input data-champion-search><span class="champion-result-count"></span><div data-champion-results></div>';
    const input = root.querySelector("input");
    api.setAPI(async () => { throw Error("R86 catalog unavailable"); });
    await api.loadRankings(true);
    assert.equal(root.querySelector("input"), input);
    assert.match(root.querySelector(".is-error").textContent, /R86 catalog unavailable/);
    assert.ok(root.querySelector("[data-champion-retry]"));
    assert.equal(s.loading, false);
  } finally { dom.window.close(); }
});
