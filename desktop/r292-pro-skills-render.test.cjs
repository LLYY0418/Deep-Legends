"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path"), vm = require("node:vm");
const { bootDemoApp } = require("./overview-render-helpers.cjs");
const { waitForOverview } = require("./renderer-wait.cjs");
const WEB = path.join(__dirname, "../backend/web");
const capture = require("../backend/testdata/r75/pro-response.json");
const qFirst = ["Q","W","E","Q","Q","R","Q","W","Q","W","W","R","W","E","E","R","E","E"];

for (const [name, order, expected] of [
  ["Q maxes first", qFirst, ["Q","W","E"]],
  ["W maxes first", qFirst.map(x => x === "Q" ? "W" : x === "W" ? "Q" : x), ["W","Q","E"]],
  ["unfinished skills rank by points", ["R","W","Q","E","W","Q","W"], ["W","Q","E"]],
  ["one learned basic skill has no main/secondary", ["R","Q","Q","R"], []],
]) {
  test(`R292 shared skill priority: ${name}`, () => {
    const context = { window: new EventTarget() };
    vm.runInNewContext(fs.readFileSync(path.join(WEB,"runtime.js"),"utf8"), context);
    assert.deepEqual(Array.from(context.window.deepLegendsRuntime.skillPriority(order)), expected);
    vm.runInNewContext(fs.readFileSync(path.join(WEB,"section-loader.js"),"utf8"), context);
    vm.runInNewContext(fs.readFileSync(path.join(WEB,"match-details.js"),"utf8"), context);
    const renderer = context.window.deepLegendsMatchDetails.create({SKILL_SLOT_LETTERS:{1:"Q",2:"W",3:"E",4:"R"}});
    const summary = renderer.skillPrioritySummary(order.map((key,index) => ({slot:"QWER".indexOf(key)+1,level:index+1})));
    if (expected.length) {
      assert.match(summary,new RegExp(`主升.*>${expected[0]}<.*副升.*>${expected[1]}<`));
    } else assert.equal(summary, "");
  });
}

test("R293 real pro footer keeps 14/15/18 actual steps beside equipment without ability introductions or missing-data placeholders", async () => {
  const {window:w, errors} = bootDemoApp({gameplaySourceTransform:source => source.replace(/\}\)\(\);\s*$/, "window.__r292={state,renderSpecialistPlayers};\n})();")});
  try {
    await waitForOverview(w);
    const {state,renderSpecialistPlayers} = w.__r292;
    state.live = {players:[],recommendations:{hero:{abilities:[..."QWER"].map(slot => ({slot,name:`技能${slot}`,description:`说明${slot}`,iconPath:`/fixture/${slot}.png`}))}}};
    const rows = [14,15,18,0].map((length,index) => ({...structuredClone(capture.pros[0]),key:`r292-${index}`,playedAt:100-index,build:length ? {skillOrder:qFirst.slice(0,length)} : undefined}));
    const root = w.document.createElement("div");w.document.body.append(root);
    root.innerHTML = renderSpecialistPlayers(rows,"pro");
    assert.equal(root.querySelectorAll("[role=tab]").length,1,"same player groups all its games");
    const games = root.querySelectorAll(".specialist-game-row");
    assert.equal(games.length,4);
    for (const [index,length] of [14,15,18].entries()) {
      const game = games[index], plan = game.querySelector(".specialist-game-footer > .sg-skills");
      assert.ok(plan);
      assert.equal(game.querySelectorAll(".skill-icon-button,.skill-plan,h3").length,0);
      assert.equal(plan.querySelector(".skill-priority-summary")?.textContent,"主Q副W");
      assert.equal(plan.querySelectorAll(".skill-order > span").length,length);
      assert.equal(plan.querySelector(".skill-order > span:last-child small").textContent,String(length));
      assert.ok(game.querySelector(".sg-items").compareDocumentPosition(plan)&w.Node.DOCUMENT_POSITION_FOLLOWING);
      assert.equal(plan.querySelector(".skill-order > span").getAttribute("data-tooltip"),"第 1 级：升级 Q");
    }
    assert.equal(games[3].querySelector(".sg-skills"),null,"missing skill data has no placeholder block");
    state.live.recommendations.hero.abilities = [];
    root.innerHTML = renderSpecialistPlayers(rows,"pro");
    assert.equal(root.querySelectorAll(".skill-icon-button,.skill-letter").length,0);
    assert.equal(root.querySelector(".skill-priority-summary")?.textContent,"主Q副W");
    assert.deepEqual(errors,[]);
  } finally { w.close(); }
});
