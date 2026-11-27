"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { WEB, bootDemoApp, installWindowCleanup } = require("./overview-render-helpers.cjs");
const { waitForOverview, waitForRender } = require("./renderer-wait.cjs");
const { waitFor } = require("../scripts/lib/wait-progress.cjs");

installWindowCleanup(test);

test("R320 menu prefetch, KR overview, return and last tab close use the real renderer", async () => {
  const { window: w, errors } = bootDemoApp({ deferredMatchDetails: true });
  const d = w.document;
  const get = (selector) => d.querySelector(selector);
  const requests = [], lookups = [];
  let prewarmRead = false;
  const scroll = get("#app-scroll");
  scroll.scrollTo = ({ top }) => { scroll.scrollTop = top; };
  const baseFetch = w.fetch;
  const append = d.head.append.bind(d.head);
  d.head.append = (...nodes) => {
    append(...nodes);
    for (const node of nodes) {
      if (node.dataset.sectionStyle === "ladder") queueMicrotask(() => node.onload());
      if (node.dataset.sectionModule === "ladder") {
        queueMicrotask(() => {
          w.eval(fs.readFileSync(path.join(WEB, "ladder.js"), "utf8"));
          node.onload();
        });
      }
    }
  };
  w.requestIdleCallback = () => 1;
  w.cancelIdleCallback = () => {};
  w.fetch = async (input, init) => {
    const url = new URL(input, w.location.href);
    if (url.pathname === "/api/ladder") {
      requests.push(url);
      const page = Number(url.searchParams.get("page")), tier = url.searchParams.get("tier");
      const rows = ["Warm Fixture", "Flag Fixture"].map((gameName, i) => ({
        gameName, tagLine: "KR1", rank: (page - 1) * 100 + i + 1, profileIconId: 23, tier: "master",
        lp: 246, level: 800, wins: 38, losses: 30, winRate: 56, champions: [],
        proMark: true, ...(i === 0 ? { proPlayer: { team: "T1", name: "Roster Fixture" } } : {}),
      }));
      return new Response(JSON.stringify({ page, tier, rows, totalSummoners: 300, fetchedAt: new Date().toISOString() }));
    }
    if (url.pathname === "/api/ladder/prewarm") {
      return { ok: true, status: 200, json: async () => {
        prewarmRead = true;
        return { playerRef: "player_prewarmed" };
      } };
    }
    if (url.pathname === "/api/gameplay/overview" && init?.body) lookups.push(JSON.parse(init.body));
    return baseFetch(input, init);
  };
  await waitForOverview(w);
  assert.equal(requests.length, 0);
  assert.equal(get('[data-section-module="ladder"]'), null);
  const menuLadder = get("#player-search-ladder");
  assert.equal(menuLadder.previousElementSibling.id, "player-search-pro");
  get("#player-search-region").click();
  await waitForRender(w, () => requests.length === 1, "opening region picker did not prefetch ladder");
  assert.equal(requests[0].searchParams.get("prefetch"), "1");
  assert.equal(get("#ladder-panel").hidden, true);
  menuLadder.click();
  await waitForRender(w, () => get("[data-ladder-row]"), "ladder did not render");
  assert.equal(requests.length, 1);
  assert.equal(get("#ladder-panel").hidden, false);

  get('[data-ladder-tier="master"]').click();
  await waitForRender(w, () => requests.some((url) => url.searchParams.get("tier") === "master") && get('[data-ladder-page="2"]'));
  get('[data-ladder-page="2"]').click();
  await waitForRender(w, () => get('[data-ladder-page="2"][aria-current]'));
  get("#ladder-pro-only").click();
  const warm = get("[data-ladder-row] button");
  get("#app-scroll").scrollTop = 185;
  warm.dispatchEvent(new w.MouseEvent("pointerover", { bubbles: true }));
  await waitFor(() => prewarmRead, { label: "ladder hover did not read its warm reference" });
  warm.click();

  await waitForRender(w, () => lookups.some((req) => req.playerRef === "player_prewarmed"));
  assert.equal(get("#overview-panel").hidden, false);
  assert.equal(get("#player-group-button").dataset.playerGroup, "kr");
  assert.equal(get("#ladder-return").hidden, false);
  assert.equal(get("#pro-players-return").hidden, true);
  assert.equal(get("#overview-panel .player-tab-region").textContent, "韩服");
  assert.match(get("#overview-content .pro-tag").textContent, /T1.*Roster Fixture/);
  assert.ok(get("#overview-content .pro-mark"));
  get("#ladder-return").click();
  assert.equal(get("#ladder-panel").hidden, false);
  assert.equal(get('[data-ladder-tier="master"]').getAttribute("aria-pressed"), "true");
  assert.equal(get("#ladder-pro-only").getAttribute("aria-checked"), "true");
  assert.ok(get('[data-ladder-page="2"][aria-current]'));
  assert.equal(d.activeElement, warm);
  assert.equal(get("#app-scroll").scrollTop, 185);

  get("[data-ladder-row]:nth-child(2) button").click();
  await waitForRender(w, () => lookups.some((req) => req.gameName === "Flag Fixture" && req.region === "kr" && req.exactRiotId === true));
  assert.equal(d.querySelectorAll("#player-tabs [data-close-player]").length, 2);
  assert.ok(get("#overview-content .pro-mark"));
  assert.equal(get("#overview-content .pro-tag"), null);
  get("#player-tabs [data-close-player]").click();
  assert.equal(get("#overview-panel").hidden, false);
  get("#player-tabs [data-close-player]").click();
  assert.equal(get("#ladder-panel").hidden, false);
  assert.deepEqual(errors, []);
});
