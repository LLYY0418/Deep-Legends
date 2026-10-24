"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp } = require("./overview-render-helpers.cjs");
const { waitForOverview } = require("./renderer-wait.cjs");
const capture = require("../backend/testdata/r75/pro-response.json");

test("R293 pro and specialist rows share one pair that moves between adaptive spell locations, omitting invalid pairs entirely", async () => {
  const {window:w, errors} = bootDemoApp({gameplaySourceTransform:source => source.replace(/\}\)\(\);\s*$/, "window.__r293={state,renderSpecialistPlayers,patchRuneStarterEquipment};\n})();")});
  try {
    await waitForOverview(w);
    const {state,renderSpecialistPlayers,patchRuneStarterEquipment} = w.__r293;
    state.perks = {styles:[],statModSlots:[]};
    state.live = {players:[]};
    const root = w.document.createElement("div");w.document.getElementById("live-content").append(root);
    const config = {...structuredClone(capture.pros[0]),spell1Id:4,spell2Id:12,build:{skillOrder:["Q","W","E"]}};
    for (const source of ["pro","specialist"]) {
      root.innerHTML = renderSpecialistPlayers([config],source);
      assert.equal(root.querySelector(".specialist-game-runes").classList.contains("has-skills"),source==="pro");
      for (const selector of [".rune-shards > .rune-corner-spells > [data-card-spells]"]) {
        const pair = root.querySelector(selector);
        assert.ok(pair,selector);
        assert.equal(pair.querySelectorAll(":scope > .item-option-button").length,2);
        assert.equal(pair.querySelectorAll("small,label").length,0,"no spell captions outside the icons");
        assert.equal(pair.querySelectorAll("[data-tooltip]").length,2,"original icon tooltips are retained");
      }
      assert.equal(root.querySelectorAll(".specialist-game-footer").length,1);
      if (source==="specialist") assert.equal(root.querySelector(".sg-skills"),null);
    }
    root.innerHTML = renderSpecialistPlayers([{...config,selectedComplete:false}],"pro");
    const row = root.querySelector(".specialist-game-row"), skills = row.querySelector(".sg-skills"), spells = row.querySelector(".sg-spells"), corner = row.querySelector(".rune-corner-spells");
    assert.equal(row.querySelector(".specialist-game-footer").nextElementSibling.className,"pro-rune-status");
    patchRuneStarterEquipment([{...config,starterItemIds:[1055,2003]}]);
    assert.equal(root.querySelector(".specialist-game-row"),row);
    assert.equal(row.querySelector(".sg-skills"),skills);
    assert.equal(row.querySelector(".sg-spells"),spells);
    assert.equal(row.querySelector(".rune-corner-spells"),corner);
    assert.deepEqual([...row.querySelectorAll(".sg-items .sg-label")].map(n=>n.textContent),["出门装","最终装备"]);
    state.perks = null;
    root.innerHTML = renderSpecialistPlayers([config],"pro");
    assert.equal(root.querySelectorAll(".rune-corner-spells,.sg-spells").length,1,"loading the rune catalog must retain the movable spell pair");
    for (const pair of [[undefined,undefined],[4,4],[4,0],[4,100001],[4.5,12],[4,Infinity]]) {
      root.innerHTML = renderSpecialistPlayers([{...config,spell1Id:pair[0],spell2Id:pair[1]}],"pro");
      assert.equal(root.querySelectorAll(".rune-corner-spells,.sg-spells").length,0);
    }
    assert.deepEqual(errors,[]);
  } finally { w.close(); }
});
