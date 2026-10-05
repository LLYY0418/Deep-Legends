"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp, settled, installWindowCleanup } = require("./overview-render-helpers.cjs");
installWindowCleanup(test);

test("R86 ADD-1 late timeline cannot reveal a filtered-out card", async () => {
  async function check(mutate = false) {
    const { window: w } = bootDemoApp({ matchCount: mutate ? 30 : 200, gameplaySourceTransform: source => mutate
      ? source.replace("function replaceMatchEntry(entry, tab, rerender, retained = false) {", "function replaceMatchEntry(entry, tab, rerender, retained = false) { entry.hidden = false;") : source });
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
