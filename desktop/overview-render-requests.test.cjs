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

test("收藏账户条复用主页背景和圆头像，并只保留两项事实", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  w.document.querySelector('[data-section="favorites"]').click();
  w.document.querySelector('[data-favorites-page="account"]').click();
  await settled();
  const hero = w.document.querySelector("#account-content .account-hero");
	assert.equal(w.document.querySelector("#favorites-account-panel h2"), null, "账户页旧标题仍在");
	assert.ok(w.document.getElementById("account-live-state"), "账户连接状态被误删");
  assert.ok(hero, "收藏页缺少当前召唤师信息条");
  assert.ok(hero.querySelector(".summoner-strip-art.account-hero-art"), "收藏页缺少主页背景图");
  assert.ok(hero.querySelector(".game-icon.is-summoner-avatar"), "收藏页未使用圆形召唤师头像");
  assert.equal(hero.querySelector(".account-hex"), null, "旧六边形字符仍在");
  assert.equal(hero.querySelectorAll(".account-facts > div").length, 2, "账户事实栏不是两列内容");
  assert.doesNotMatch(hero.textContent, /主页背景/);
  assert.deepEqual(errors, [], `收藏账户条渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("游戏时间分布下方提供等栏宽分享按钮，并在选定路径后生成桌面双列长图", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  const activity = overview.querySelector(".career-column .activity-section");
  const button = overview.querySelector(".career-column [data-generate-overview-share]");
  assert.ok(activity, "总览缺少游戏时间分布");
  assert.ok(button, "游戏时间分布下方缺少生成分享图按钮");
  assert.equal(activity.nextElementSibling, button, "分享按钮必须紧跟游戏时间分布");
  assert.equal(button.parentElement.classList.contains("career-column"), true, "按钮应直接占满左侧生涯栏宽度");
  assert.ok(button.querySelector("svg"), "分享按钮文字前应有图标");
  assert.match(button.textContent, /生成分享图/);
	const matchList = overview.querySelector(".match-list");
	while (matchList.querySelectorAll(":scope > .match-entry").length < 25) {
		matchList.append(matchList.querySelector(":scope > .match-entry").cloneNode(true));
	}

  const calls = [];
  w.desktopShare = {
    async preparePngSave(fileName) {
      calls.push(["choose", fileName]);
      assert.equal(w.document.querySelector(".overview-share-surface"), null, "弹保存框前不应构造或挂载导出页面");
      return { canceled: false, token: "A".repeat(32), directory: "C:\\Users\\Player\\Pictures" };
    },
    async captureAndSavePng(payload) {
      calls.push(["capture", payload]);
      assert.equal(payload.token, "A".repeat(32));
      assert.match(payload.markup, /overview-share-watermark/);
      assert.match(payload.markup, /DEEP LEGENDS/);
      assert.match(payload.markup, /career-column/);
      assert.match(payload.markup, /matches-column/);
      assert.doesNotMatch(payload.markup, /data-generate-overview-share/);
      assert.doesNotMatch(payload.markup, /\sstyle="/, "导出 HTML 不应携带会被 CSP 拦截的内联 style");
		const exported = new JSDOM(payload.markup).window.document;
		assert.equal(exported.querySelectorAll(".match-list > .match-entry").length, 20, "默认设置应只导出前 20 场");
		assert.equal(exported.querySelector(".match-pagination"), null, "分享图不应保留分页提示");
      return { ok: true, fileName: "share.png", pixelWidth: 2976, pixelHeight: 7200 };
    },
  };
  button.click();
  await new Promise((resolve) => setTimeout(resolve, 40));
  assert.deepEqual(calls.map(([name]) => name), ["choose", "capture"], "必须先选择保存位置，再生成截图");
  assert.match(button.textContent, /分享图已保存/);
  assert.ok(button.classList.contains("is-success"));
	assert.equal(w.document.getElementById("setting-share-directory").textContent, "C:\\Users\\Player\\Pictures", "首次生成选定目录后设置页应立即显示真实位置");
  assert.deepEqual(errors, [], `分享图交互出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("分享图按 50 场设置截断当前已加载战绩", async () => {
	const { window: w } = bootDemoApp({ matchCount: 50 });
	await settled();
	const overview = w.document.getElementById("overview-content");
	const matchList = overview.querySelector(".match-list");
	while (matchList.querySelectorAll(":scope > .match-entry").length < 55) {
		matchList.append(matchList.querySelector(":scope > .match-entry").cloneNode(true));
	}
	let exportedCount = 0;
	w.desktopShare = {
		async preparePngSave() { return { canceled: false, token: "B".repeat(32), directory: "D:\\Shares" }; },
		async captureAndSavePng(payload) {
			exportedCount = new JSDOM(payload.markup).window.document.querySelectorAll(".match-list > .match-entry").length;
			return { ok: true };
		},
	};
	overview.querySelector("[data-generate-overview-share]").click();
	await new Promise((resolve) => setTimeout(resolve, 40));
	assert.equal(exportedCount, 50);
	w.close();
});

test("排位分区在负场缺失时也能渲染出胜率提示", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const ranks = w.document.querySelector("#overview-content .rank-list");
  assert.deepEqual(errors, [], `渲染期出现异常：\n${errors.join("\n")}`);
  assert.ok(ranks, "总览缺少排位分区");
  assert.ok(ranks.querySelectorAll(".rank-row").length >= 2, "排位分区应至少渲染单排与灵活两行");
  w.close();
});

test("渲染异常会切到可重试的错误态，而不是永远停在骨架屏", async () => {
  const { window: w } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  // 走完整的真实链路：让 /api/gameplay/overview 回一份 capabilities 形状非法的载荷，
  // renderCareerSections 会在 capabilities.find 上抛 TypeError。
  const previousFetch = w.fetch;
  w.fetch = (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    if (url.split("?")[0] === "/api/gameplay/overview") {
      const body = JSON.stringify({ player: { playerRef: "broken" }, matches: [], capabilities: 1 });
      return Promise.resolve(new w.Response(body, { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    return previousFetch(input, init);
  };
  w.document.getElementById("overview-refresh").click();
  await settled();
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "渲染失败后不应停在骨架屏");
  assert.match(overview.textContent, /总览渲染失败/, "渲染失败后应展示错误态");
  assert.ok(overview.querySelector("[data-gameplay-retry]"), "错误态应带重试按钮");
  w.close();
});

test("已有总览刷新时保留当前内容，不退回整页骨架", async () => {
  const { window: w } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  w.document.getElementById("overview-refresh").click();
  assert.ok(overview.querySelector(".summoner-strip"), "刷新时应继续展示已有总览");
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "已有数据刷新不应退回骨架屏");
  assert.match(overview.querySelector(".overview-refreshing")?.textContent || "", /正在刷新最新战绩/);
  await settled();
  w.close();
});

test("客户端退出后清空本人总览，国服空分组显示启动入口", async () => {
  const { window: w, errors, eventSources } = bootDemoApp({ liveEvents: true });
  await settled();
  const overview = w.document.getElementById("overview-content");
  assert.ok(overview.querySelector(".summoner-strip"), "断连前总览应已完成渲染");
  const previousFetch = w.fetch;
  w.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : input?.url || "";
    if (url.split("?")[0] === "/api/client-installations") {
      return new w.Response(JSON.stringify({ items: [
        { id: "tcls", name: "国服纯净入口", available: true },
        { id: "wegame", name: "WeGame", available: true },
      ] }));
    }
    const response = await previousFetch(input, init);
    if (url.split("?")[0] !== "/api/status") return response;
    const status = await response.json();
    return new w.Response(JSON.stringify({ ...status, clientView:{type:"client-view",state:"no-client",generation:status.clientView.generation+1}, connected: false, identityReady: false, snapshotReady: false, clientDiscovery: "process-not-found" }));
  };
  eventSources.at(-1).onmessage({ data: "resync-required" });
  await settled();
  assert.equal(overview.textContent, "");
  assert.equal(overview.querySelector("[data-gameplay-retry]"), null);
  assert.equal(w.document.getElementById("client-launchpad").hidden, false);
  assert.match(w.document.getElementById("client-launchpad").textContent, /国服纯净入口/);
  assert.match(w.document.getElementById("client-launchpad").textContent, /WeGame/);
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "断连后不应退回骨架屏");
	assert.equal(w.document.querySelectorAll("#player-tabs [data-player-tab]").length, 0, "断连后不应残留国服玩家标签");
  await settled();
  assert.deepEqual(errors, [], `断连渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});
