"use strict";
// 前端渲染冒烟测试：用 jsdom 加载真实的 index.html + 全部前端脚本，打开演示数据，
// 断言总览页真的渲染出内容。
//
// 存在的理由：`renderOverviewBody` 里任何一个运行期异常（例如引用了不存在的变量）
// 都会让容器永远停在骨架屏上，界面表现为“总览一直在加载”，而单元测试和 go test
// 全都照样通过。这个测试直接跑真实渲染路径，是唯一能挡住该类故障的护栏。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("jsdom");

const { WEB, SCRIPTS, gameplaySource, suiteSource, appStyles, gameplayStyles, suiteStyles, functionSource, compileFunctions, collapsedBeaconStyles, bootDemoApp, settled, installWindowCleanup, visitTool, bootLiveTab } = require("./overview-render-helpers.cjs");

installWindowCleanup(test);

test("推荐请求期间遮罩只覆盖 tab 下方面板，并按内容提示", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const previousFetch = w.fetch;
  let recommendations;
  let releaseRecommendations;
  w.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    const pathname = url.split("?")[0];
    if (pathname === "/api/gameplay/live") {
      const response = await previousFetch(input, init);
      const payload = await response.json();
      recommendations = payload.recommendations;
      delete payload.recommendations;
      return new w.Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
    }
    if (pathname === "/api/gameplay/recommendations") {
      return new Promise((resolve) => {
        releaseRecommendations = () => resolve(new w.Response(JSON.stringify({ recommendations }), { status: 200, headers: { "Content-Type": "application/json" } }));
      });
    }
    return previousFetch(input, init);
  };
  w.document.querySelector('[data-section="live"]').click();
  await new Promise((resolve) => setTimeout(resolve, 80));
  const live = w.document.getElementById("live-content");
  const loadingPanels = [...live.querySelectorAll(".recommendation-panel.is-loading")];
  assert.ok(loadingPanels.length >= 2, "符文和出装页签都应进入局部加载态");
  assert.match(loadingPanels.map((panel) => panel.textContent).join(" "), /正在读取符文推荐/);
  assert.match(loadingPanels.map((panel) => panel.textContent).join(" "), /正在读取海克斯、出装与技能/);
  assert.ok(loadingPanels.every((panel) => panel.querySelector(":scope > .panel-loading")), "遮罩必须是面板的直接子元素");
  assert.ok(!live.querySelector(".recommendation-tabs .panel-loading"), "tab 栏不应被遮罩包住");
  releaseRecommendations?.();
  await settled();
  assert.deepEqual(errors, [], `加载态渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("对局页出装推荐渲染，且不再出现「后期备选」", async () => {
  const { window: w, errors } = await bootLiveTab();
  const build = w.document.querySelector("#live-content .build-recommendation");
  assert.deepEqual(errors, [], `渲染期出现异常：\n${errors.join("\n")}`);
  assert.ok(build, "对局页缺少出装推荐区");
  assert.doesNotMatch(build.textContent, /后期备选/, "「后期备选」应已整块移除");
  assert.ok(build.querySelector(".build-summary-spells"), "缺少召唤师技能块");
  assert.ok(build.querySelector(".build-summary-starter"), "缺少出门装块");
  assert.ok(build.querySelector(".build-summary-boots"), "缺少鞋子块");
  w.close();
});

test("核心装只展示胜率与场次，不再展示选取率", async () => {
  const { window: w } = await bootLiveTab();
  const core = w.document.querySelector("#live-content .item-core-column");
  assert.ok(core, "缺少核心装分栏");
  const stats = core.querySelector(".config-option .option-stats");
  assert.ok(stats, "核心装缺少统计列");
  assert.ok(stats.classList.contains("is-depth"), "核心装统计列应使用两列（胜率/场次）版式");
  assert.doesNotMatch(core.textContent, /选取率/, "核心装不应再出现选取率");
  assert.match(core.textContent, /胜率/, "核心装应保留胜率");
  assert.match(core.textContent, /场次/, "核心装应保留场次");
  w.close();
});

test("技能加点的选取率/胜率/场次与技能图标同一行", async () => {
  const { window: w } = await bootLiveTab();
  const row = w.document.querySelector("#live-content .skill-plan .skill-priority-row");
  assert.ok(row, "技能加点缺少 skill-priority-row");
  assert.ok(row.querySelector(".skill-priority"), "该行应包含技能图标组");
  assert.ok(row.querySelector(".option-stats"), "该行应包含统计列——统计列还留在标题行上就说明没改对");
  assert.ok(!w.document.querySelector("#live-content .skill-plan-head .option-stats"), "统计列不应再挂在标题行");
  w.close();
});

test("对抗样本为空时给出说明文案，而不是一个「—」", async () => {
  const { window: w } = await bootLiveTab();
  const header = w.document.querySelector("#live-content .champion-matchups");
  assert.ok(header, "缺少优劣势对抗区");
  const empties = header.querySelectorAll(".matchups-empty");
  for (const node of empties) {
    assert.match(node.textContent, /暂无对线样本/);
    assert.ok(node.dataset.tooltip, "空状态应带解释性 tooltip");
  }
  assert.ok(!/^—$/.test(header.textContent.trim()), "不应只渲染一个破折号");
  w.close();
});

test("英雄位置统计为空时显示明确说明，而不是三项破折号", async () => {
  const boot = bootDemoApp();
  const w = boot.window;
  await settled();
  const previousFetch = w.fetch;
  w.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    if (url.split("?")[0] !== "/api/gameplay/live") return previousFetch(input, init);
    const response = await previousFetch(input, init);
    const payload = await response.json();
    payload.recommendations = payload.recommendations || {};
    payload.recommendations.hero = { emptyReason: "该英雄在这个位置没有统计样本" };
    return new w.Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
  };
  w.document.querySelector('[data-section="live"]').click();
  await settled();
  const summaries = [...w.document.querySelectorAll("#live-content .recommendation-champion-summary")];
  assert.ok(summaries.length > 0, "缺少推荐英雄摘要");
  assert.ok(summaries.every((summary) => summary.querySelector(".champion-stats-empty")?.textContent === "该英雄在这个位置没有统计样本"));
  assert.ok(summaries.every((summary) => !summary.querySelector(".champion-summary-main dl")), "空样本时仍渲染了三项破折号");
  w.close();
});

test("failed build timelines retry on reopening, successful timelines remain cached", async () => {
  const state = {matchTimelines: new Map(), matchTimelineFlights:new Set()};
  const tab = {data:{player:{playerRef:"safe-ref"}}};
  const match = {gameId:42}, subject = {participantId:2};
  let calls=0, renders=0;
  const api=async()=>{ calls++; if(calls===1) throw Error("temporary timeout"); return {available:true,itemGroups:[{}],skillOrder:[1]}; };
  const ensure = new Function("state","api","riotTab","connected","matchTimelineKey","tabServerID","rerenderMatch","recordTimelineClient",
    `return (${functionSource(gameplaySource,"ensureMatchTimeline")});`)(state,api,()=>true,()=>true,()=>"kr:42:2",()=>"",()=>renders++,()=>{});
  await ensure(match,subject,tab);
  assert.equal(state.matchTimelines.get("kr:42:2").available,false);
  await ensure(match,subject,tab);
  assert.equal(state.matchTimelines.get("kr:42:2").available,true);
  await ensure(match,subject,tab);
  assert.equal(calls,2);
  assert.equal(renders,2);
  assert.equal(state.matchTimelineFlights.size,0);
});


test("R86 ADD optional cs-dialog computed zoom stays one inside the zoomed app frame", () => {
  function check(css) {
    const dom = new JSDOM(`<style>${appStyles}</style><style>${css}</style><div class="app-frame" id="app-frame"><div class="cs-dialog"></div></div>`);
    try {
      for (const zoom of [1, 2, 2.5]) {
        dom.window.document.documentElement.style.setProperty("--ui-zoom", String(zoom));
        assert.equal(dom.window.getComputedStyle(dom.window.document.querySelector(".cs-dialog")).zoom, "1");
      }
    } finally { dom.window.close(); }
  }
  check(suiteStyles);
  assert.throws(() => check(`${suiteStyles}\n.cs-dialog { zoom: var(--ui-zoom, 1); }`), { name: "AssertionError" });
});

test("R86 ADD-1 failed timelines wait for explicit retry and keep unrelated cards", async () => {
  async function check(mutate = false) {
    const { window: w, errors } = bootDemoApp({ gameplaySourceTransform: source => mutate
      ? source.replace("if (!state.matchTimelines.has(matchTimelineKey(match, subject, tab)))", "if (true)") : source });
    try {
      await settled();
      const d = w.document, list = d.querySelector(".match-list");
      const initial = [...list.querySelectorAll(".match-entry")];
      const id = initial[0].dataset.matchId;
      const card = () => list.querySelector(`[data-match-id="${id}"]`);
      card().querySelector("[data-toggle-match]").click();
      let requests = 0;
      const originalFetch = w.fetch;
      w.fetch = (url, ...args) => String(url).startsWith("/api/gameplay/match-timeline")
        ? Promise.resolve(new w.Response(JSON.stringify(++requests === 1
          ? { available: false, detail: "fixture upstream unavailable" }
          : { available: true, itemGroups: [{ minute: 5, events: [{ itemId: 1036 }] }] }))) : originalFetch(url, ...args);
      card().querySelector('[data-match-detail="build"]').click();
      await new Promise(resolve => setTimeout(resolve, 60));
      assert.equal(requests, 1, "rendering a failure must not silently retry it");
      assert.ok(card().querySelector("[data-timeline-retry]"));
      card().querySelector("[data-timeline-retry]").click();
      await new Promise(resolve => setTimeout(resolve, 60));
      assert.equal(requests, 2);
      assert.ok(card().querySelector(".timeline-route"));
      assert.equal(d.querySelector(".match-list"), list);
      const after = [...list.querySelectorAll(".match-entry")];
      initial.forEach((entry, index) => { if (entry.dataset.matchId !== id) assert.ok(after[index] === entry, "retry replaced an unrelated card"); });
      assert.deepEqual(errors, []);
    } finally { w.close(); }
  }
  await check();
  await assert.rejects(check(true), { name: "AssertionError", message: /rendering a failure must not silently retry it/ });
});
