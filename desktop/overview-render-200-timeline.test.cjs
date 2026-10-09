"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp, settled, installWindowCleanup } = require("./overview-render-helpers.cjs");
const { waitForOverview } = require("./renderer-wait.cjs");

installWindowCleanup(test);

test("R86 ADD-1 late timeline cannot reveal a filtered-out card", async () => {
  async function check(mutate = false) {
    const { window: w } = bootDemoApp({ matchCount: mutate ? 30 : 200, gameplaySourceTransform: source => mutate
      ? source.replace("function replaceMatchEntry(entry, tab, rerender, retained = false) {", "function replaceMatchEntry(entry, tab, rerender, retained = false) { if(entry.parentNode && !entry.parentNode.isConnected)document.querySelector(\".match-list\").replaceWith(entry.parentNode);") : source });
    try {
      await waitForOverview(w, mutate ? 30 : 200);
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
      assert.equal(list.isConnected,false,"the ordinary filter must be stashed");
      const others = [...list.querySelectorAll(".match-entry")].filter(node => node.dataset.matchId !== id);
      release(new w.Response(JSON.stringify({ available: true })));
      await new Promise(resolve => setTimeout(resolve, 60));
      assert.equal(list.isConnected,false,"late timeline must not mount the stashed filter");
      d.querySelector('[data-match-filter="all"]').click();
      assert.ok(others.every(node=>node.isConnected));
    } finally { w.close(); }
  }
  await check();
  await assert.rejects(check(true), { name: "AssertionError", message: /late timeline must not mount the stashed filter/ });
});
