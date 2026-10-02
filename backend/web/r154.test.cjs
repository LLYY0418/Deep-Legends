"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("../../desktop/node_modules/jsdom");

const source = fs.readFileSync(path.join(__dirname, "favorites-facade.js"), "utf8");
const champions = fs.readFileSync(path.join(__dirname, "champions.js"), "utf8");
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const tick = () => new Promise((resolve) => setImmediate(resolve));

function facade(sourceText = source) {
  const dom = new JSDOM(html, { url: "http://fixture/", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window;
  w.document.getElementById("favorites-facade-panel").hidden = false;
  w.requestAnimationFrame = (callback) => callback();
  const pending = new Map();
  w.fetch = (url) => new Promise((resolve) => pending.set(url, resolve));
  w.eval(sourceText);
  const respond = async (view, payload) => {
    const resolve = pending.get(`/api/facade/${view}`);
    assert.ok(resolve, `${view} request must be pending`);
    resolve({ ok: true, status: 200, json: async () => payload });
    await tick();
  };
  const grid = w.document.getElementById("facade-grid");
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: true } }));
  return { dom, w, grid, pending, respond };
}

const iconPayload = { total: 3, icons: [
  { id: 1, title: "已拥有甲", year: 2026, owned: true, sets: [] },
  { id: 2, title: "未拥有乙", year: 2025, owned: false, sets: [] },
  { id: 3, title: "已拥有丙", year: 2024, owned: true, sets: [] },
] };

async function checkViewTransition(sourceText) {
  const fx = facade(sourceText);
  try {
    await fx.respond("icons", iconPayload);
    assert.equal(fx.grid.querySelectorAll(".skin-card").length, 2);
    fx.w.deepLegendsFavoritesFacade.setView("banners");
    assert.ok(fx.pending.has("/api/facade/banners"));
    assert.equal(fx.grid.getAttribute("aria-busy"), "true");
    assert.equal(fx.grid.querySelectorAll(".skin-card").length, 0, "waiting banner grid must not retain icon cards");
  } finally { fx.dom.window.close(); }
}

async function checkKeyedCards(sourceText) {
  const fx = facade(sourceText);
  try {
    await fx.respond("icons", iconPayload);
    const first = fx.grid.querySelector('[data-facade-key="icons:1"]');
    const third = fx.grid.querySelector('[data-facade-key="icons:3"]');
    first.querySelector("img").classList.add("is-loaded");
    const toggle = fx.w.document.getElementById("facade-show-unowned");
    toggle.checked = true;
    toggle.dispatchEvent(new fx.w.Event("change", { bubbles: true }));
    assert.strictEqual(fx.grid.querySelector('[data-facade-key="icons:1"]'), first);
    assert.strictEqual(fx.grid.querySelector('[data-facade-key="icons:3"]'), third);
    assert.ok(first.querySelector("img").classList.contains("is-loaded"));
    assert.ok(fx.grid.querySelector('[data-facade-key="icons:2"]'));
    const sort = fx.w.document.getElementById("facade-sort");
    sort.value = "old";
    sort.dispatchEvent(new fx.w.Event("change", { bubbles: true }));
    assert.strictEqual(fx.grid.querySelector('[data-facade-key="icons:1"]'), first);
    assert.strictEqual(fx.grid.querySelector('[data-facade-key="icons:3"]'), third);
    toggle.checked = false;
    toggle.dispatchEvent(new fx.w.Event("change", { bubbles: true }));
    assert.equal(fx.grid.querySelector('[data-facade-key="icons:2"]'), null);
    assert.strictEqual(fx.grid.querySelector('[data-facade-key="icons:1"]'), first);
  } finally { fx.dom.window.close(); }
}

test("R154 switching to an unloaded view clears the old cards before fetch resolves", async () => {
  await checkViewTransition(source);
  const mutant = source.replace('      el.grid.replaceChildren();\n      el.grid.setAttribute("aria-busy", "true");', '      el.grid.setAttribute("aria-busy", "true");');
  assert.notEqual(mutant, source);
  await assert.rejects(checkViewTransition(mutant), { name: "AssertionError" });
});

test("R154 filters and sorting preserve already loaded card and image nodes", async () => {
  await checkKeyedCards(source);
  const mutant = source.replace('    const existing = new Map([...el.grid.querySelectorAll', '    el.grid.replaceChildren();\n    const existing = new Map([...el.grid.querySelectorAll');
  assert.notEqual(mutant, source);
  await assert.rejects(checkKeyedCards(mutant), { name: "AssertionError" });
});

test("R154 open circuit and ordinary timeout show different detail errors", () => {
  const fn = champions.match(/  function championDetailErrorMessage\(error\) \{[\s\S]*?\n  \}/)?.[0];
  assert.ok(fn);
  const message = Function(`${fn}; return championDetailErrorMessage;`)();
  assert.equal(message(new Error("hexdata circuit is open")), "上游暂时不可用，请稍后再试");
  assert.equal(message(new Error("上游暂时不可用，请稍后再试")), "上游暂时不可用，请稍后再试");
  assert.equal(message(Object.assign(new Error("本地请求超时，请重试"), { name: "TimeoutError" })), "网络超时，请重试");
  const mutant = fn.replace('if (message.includes("hexdata circuit is open") || message.includes("上游暂时不可用")) return "上游暂时不可用，请稍后再试";', '');
  const mutated = Function(`${mutant}; return championDetailErrorMessage;`)();
  assert.notEqual(mutated(new Error("hexdata circuit is open")), message(new Error("hexdata circuit is open")));
});
