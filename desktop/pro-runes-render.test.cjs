"use strict";
// Real page and scripts, public R75 capture; only the LCU shell is demo data.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("jsdom");
const WEB = path.join(__dirname, "..", "web");
const capture = JSON.parse(fs.readFileSync(path.join(__dirname, "..", "testdata/r75/pro-response.json"), "utf8"));
const settle = () => new Promise(resolve => setTimeout(resolve, 1000));

for (const coverage of [null, {totalGames:99,failedGames:1}, {totalGames:30,failedGames:10}]) {
test(`R75/R76 actual page: pro rows, degradation ${coverage ? coverage.failedGames+"/"+coverage.totalGames : "capture"}, incomplete and offline`, async () => {
  const dom = new JSDOM(fs.readFileSync(path.join(WEB, "index.html"), "utf8"), { url: "http://127.0.0.1:1/?demo", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window;
  try {
    const errors = [], requests = [];
    let offline = false;
    w.onerror = (_, __, ___, ____, error) => errors.push(String(error));
    w.console.error = (...args) => errors.push(args.join(" "));
    w.IntersectionObserver = class { observe() {} unobserve() {} disconnect() {} };
    w.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
    w.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} });
    w.scrollTo = () => {};
    w.HTMLElement.prototype.scrollIntoView = () => {};
    w.Element.prototype.scrollTo = () => {};
    w.CSS ||= {}; w.CSS.escape ||= value => String(value).replace(/[^a-zA-Z0-9_-]/g, c => `\\${c}`);
    w.structuredClone = structuredClone;
    w.fetch = fetch; w.Response = Response; w.Headers = Headers; w.Request = Request;
    // Keep the immutable capture inside its original 30-day validation window.
    w.Date.now = () => Date.parse("2026-09-09T04:16:00Z");
    for (const file of ["runtime.js", "demo-data.js", "app.js", "gameplay.js", "champions.js", "friends.js", "suite.js"]) {
      w.eval(fs.readFileSync(path.join(WEB, file), "utf8"));
      if (file !== "demo-data.js") continue;
      const demoFetch = w.fetch;
      w.fetch = async (input, init) => {
        const url = typeof input === "string" ? input : input.url;
        requests.push({ url, method: init?.method || "GET" });
        if (url.startsWith("/api/gameplay/pro-runes")) {
          if (offline) throw new Error("simulated offline");
          return new Response(JSON.stringify(coverage ? {...capture,...coverage,readAt:"2026-09-09T04:16:00Z",stale:false,reason:"network-unavailable",preparing:false,outcomesLoading:false} : capture), { headers: { "Content-Type": "application/json" } });
        }
        const response = await demoFetch(input, init);
        if (!url.startsWith("/api/gameplay/live")) return response;
        const payload = await response.json();
        const self = payload.players.find(player => player.isCurrent);
        self.championId = 69; self.championName = "卡西奥佩娅"; self.position = "mid";
        payload.recommendations.hasTopPlayers = false;
        payload.recommendations.resolvedPosition = "mid";
        return new Response(JSON.stringify(payload), { headers: { "Content-Type": "application/json" } });
      };
    }
    w.document.dispatchEvent(new w.Event("DOMContentLoaded", { bubbles: true }));
    await settle();
    w.document.querySelector('[data-section="live"]').click();
    await settle();
    const tab = w.document.querySelector('[data-rune-source="pro"]');
    assert.ok(tab, "hasTopPlayers=false must not hide pro");
    tab.click();
    await settle();
    const live = w.document.querySelector("#live-content");
    assert.match(live.textContent, /T1 Faker/);
    assert.match(live.textContent, /LCK · 2026-09-06/);
    assert.match(live.textContent, /DK ShowMaker/);
    assert.ok(live.querySelector(".specialist-game-row"));
    assert.ok(requests.some(r => r.url.includes("championId=69")));
    if (coverage) {
      const note = live.querySelector(".pro-rune-status")?.textContent || "";
      assert.doesNotMatch(note,/无法连接|请检查网络|缓存数据/);
      if (coverage.failedGames===1) {
        assert.doesNotMatch(note,/1\/99|暂未补齐/);
        assert.doesNotMatch(note,/部分职业赛事数据加载失败/);
        assert.equal(live.querySelector("[data-retry-pro-runes]"),null);
      } else {
        assert.doesNotMatch(note,/部分职业赛事数据加载失败/);
        assert.doesNotMatch(note,/10\/30|暂未补齐/);
        assert.ok(live.querySelector("[data-retry-pro-runes]"));
      }
    }
    const zeka = [...live.querySelectorAll("[data-specialist-player]")].find(el => el.textContent.includes("Zeka"));
    assert.ok(zeka); zeka.click();
    const incomplete = capture.pros.find(row => row.selectedComplete === false);
    const choice = live.querySelector(`[data-rune-choice="${incomplete.key}"]`);
    assert.ok(choice, "captured eight-perk game must render"); choice.click();
    assert.match(live.textContent, /上游未提供完整槽位/);
    assert.equal(live.querySelector("[data-apply-runes]").disabled, true);
    live.querySelector("[data-apply-runes]").click();
    assert.ok(!requests.some(r => r.method === "POST" && r.url.includes("runes")));
    offline = true;
    w.dispatchEvent(new w.Event("offline"));
    const retry = live.querySelector("[data-retry-pro-runes]");
    assert.ok(retry, "offline cache must be marked and retryable");
    retry.click(); await settle();
    assert.doesNotMatch(live.textContent, /缓存数据 ·|无法连接职业赛事数据源/);
    assert.match(live.querySelector("[data-retry-pro-runes]").getAttribute("data-tooltip"), /上次读取结果/);
    assert.deepEqual(errors, []);
    if (process.env.R75_RENDER_HTML) {
      for (const name of ["app.css", "gameplay.css", "champions.css", "friends.css", "suite.css"]) {
        const style = w.document.createElement("style");
        style.textContent = fs.readFileSync(path.join(WEB, name), "utf8");
        w.document.head.append(style);
      }
      w.document.querySelectorAll("script, link[rel=stylesheet]").forEach(el => el.remove());
      fs.writeFileSync(process.env.R75_RENDER_HTML, dom.serialize());
    }
  } finally { w.close(); }
});

}
