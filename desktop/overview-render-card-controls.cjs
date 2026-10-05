"use strict";
const assert = require("node:assert/strict");
const { bootDemoApp, settled } = require("./overview-render-helpers.cjs");

  async function checkCardControls(mutation = "", external = false) {
    const matchCount = mutation ? 30 : 200;
    const { window: w, errors } = bootDemoApp({ matchCount, gameplaySourceTransform(source) {
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
        assert.equal(before.length, matchCount, label);
        const parent = list.parentElement;
        let calls = 0;
        const create = d.createElement.bind(d);
        d.createElement = (...args) => { calls++; return create(...args); };
        try {
          action();
          await new Promise(resolve => setTimeout(resolve, 30));
          assert.ok(list.isConnected && list.parentElement === parent, `${label}: match-list was replaced`);
          const after = [...list.querySelectorAll(".match-entry")];
          assert.equal(after.length, matchCount, label);
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
module.exports = { checkCardControls };
