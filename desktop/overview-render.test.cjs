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

const WEB = path.join(__dirname, "..", "web");
const SCRIPTS = ["demo-data.js", "app.js", "gameplay.js", "champions.js", "friends.js", "suite.js"];
const gameplaySource = fs.readFileSync(path.join(WEB, "gameplay.js"), "utf8");
const appStyles = fs.readFileSync(path.join(WEB, "app.css"), "utf8");
const gameplayStyles = fs.readFileSync(path.join(WEB, "gameplay.css"), "utf8");

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
  if (source.slice(Math.max(0, start - 6), start) === "async ") start -= 6;
  const bodyStart = source.indexOf("{", start);
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const character = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === quote) quote = "";
      continue;
    }
    if (character === '"' || character === "'" || character === "`") {
      quote = character;
      continue;
    }
    if (character === "{") depth += 1;
    if (character === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} body is not balanced`);
}

function compileFunctions(source, names, dependencies) {
  const dependencyNames = Object.keys(dependencies);
  const factory = Function(
    ...dependencyNames,
    `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn { ${names.join(", ")} };`,
  );
  return factory(...dependencyNames.map((name) => dependencies[name]));
}

function collapsedBeaconStyles(css = appStyles) {
  const dom = new JSDOM(`<html data-sidebar="collapsed"><head><style>${css}</style><style>${gameplayStyles}</style></head><body><button id="section-live" class="section-tab"><span class="section-label">对局</span><span class="live-beacon"></span></button></body></html>`, { pretendToBeVisual: true });
  const label = dom.window.document.querySelector(".section-label");
  const beacon = dom.window.document.querySelector(".live-beacon");
  return {
    dom,
    labelDisplay: dom.window.getComputedStyle(label).display,
    beaconDisplay: dom.window.getComputedStyle(beacon).display,
  };
}

test("collapsed sidebar hides the label but keeps the live beacon rendered", () => {
  const rendered = collapsedBeaconStyles();
  assert.equal(rendered.labelDisplay, "none");
  assert.notEqual(rendered.beaconDisplay, "none");
  rendered.dom.window.close();

  const mutated = appStyles.replace(
    ':root[data-sidebar="collapsed"] .section-tab > span:not(.live-beacon),',
    ':root[data-sidebar="collapsed"] .section-tab > span,',
  );
  const regression = collapsedBeaconStyles(mutated);
  assert.equal(regression.beaconDisplay, "none", "the rendered-state guard must catch selectors that hide every span");
  regression.dom.window.close();
});

function bootDemoApp(options = {}) {
  const html = fs.readFileSync(path.join(WEB, "index.html"), "utf8");
  const errors = [];
  const dom = new JSDOM(html, { url: "http://127.0.0.1:1/?demo", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window;
  w.onerror = (message, source, line, column, error) => { errors.push(String((error && error.stack) || message)); };
  w.addEventListener("unhandledrejection", (event) => { errors.push(String((event.reason && event.reason.stack) || event.reason)); });
  // jsdom 未实现的浏览器能力，用最小替身补齐（只影响可见性/尺寸，不影响渲染分支）。
  w.IntersectionObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.matchMedia = w.matchMedia || (() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }));
  w.scrollTo = () => {};
  w.HTMLElement.prototype.scrollIntoView = () => {};
  w.Element.prototype.scrollTo = function () {};
  w.structuredClone = globalThis.structuredClone;
  w.fetch = globalThis.fetch;
  w.Response = globalThis.Response;
  w.Headers = globalThis.Headers;
  w.Request = globalThis.Request;
	if (options.matchCount) w.localStorage.setItem("lol-loot-match-count", String(options.matchCount));
  // 渲染故障会被 renderOverviewBody 兜住并写进 console.error，这里一并收集。
  w.console.error = (...args) => { errors.push(args.map((value) => (value && value.stack) || String(value)).join(" ")); };

  for (const file of SCRIPTS) {
    w.eval(fs.readFileSync(path.join(WEB, file), "utf8"));
  }
  w.document.dispatchEvent(new w.Event("DOMContentLoaded", { bubbles: true }));
  return { window: w, errors };
}

const settled = () => new Promise((resolve) => setTimeout(resolve, 1500));

async function bootLiveTab() {
  const boot = bootDemoApp();
  await settled();
  boot.window.document.querySelector('[data-section="live"]').click();
  await settled();
  return boot;
}

test("平均段位只查询视口内卡片，并在分页加载期间停用", async () => {
  const dom = new JSDOM('<main id="app-scroll"><div id="matches"></div></main>', { url: "http://localhost/", pretendToBeVisual: true });
  const w = dom.window;
  const root = w.document.getElementById("app-scroll");
  const container = w.document.getElementById("matches");
  container.dataset.matchTierScope = "scope";
  root.getBoundingClientRect = () => ({ top: 0, left: 0, right: 300, bottom: 100, width: 300, height: 100 });
  const matches = [];
  for (let index = 0; index < 40; index += 1) {
    const article = w.document.createElement("article");
    article.className = "match-entry";
    const node = w.document.createElement("span");
    node.dataset.matchTierPending = "";
    node.dataset.gameId = String(index + 1);
    article.append(node);
    container.append(article);
    const top = index < 5 ? index * 18 : 200 + index * 18;
    article.getBoundingClientRect = () => ({ top, left: 0, right: 280, bottom: top + 16, width: 280, height: 16 });
    matches.push({ gameId: index + 1, createdAt: index + 1, duration: 1800 });
  }
  const requests = [];
  const state = { activeTab: "current", matchTiers: new Map(), matchTierFlights: new Set() };
  const { hydrateMatchTiers } = compileFunctions(gameplaySource, [
    "matchTierScrollRoot", "matchTierNodeIsVisible", "hydrateMatchTiers", "shouldHydrateMatchTiers",
  ], {
    window: w,
    document: w.document,
    state,
    riotTab: () => true,
    connected: () => true,
    matchTierCacheKey: (_tab, gameID) => String(gameID),
    applyMatchTierValue: () => {},
    tabServerID: () => "",
    matchTierFromScores: () => null,
    MATCH_TIERS_MAX_REFS: 24,
    api: async (_path, options) => {
      requests.push(JSON.parse(options.body));
      return {};
    },
  });
  const tab = { key: "current", data: { player: { playerRef: "player" }, matches } };
  await hydrateMatchTiers(container, tab, "scope");
  assert.equal(requests.length, 1);
  assert.deepEqual(requests[0].matches.map((match) => match.gameId), [1, 2, 3, 4, 5]);

  requests.length = 0;
  state.matchTiers.clear();
  tab.loadingMore = true;
  await hydrateMatchTiers(container, tab, "scope");
  assert.equal(requests.length, 0, "加载更多期间不得查询平均段位");

  tab.loadingMore = false;
  tab.filterPaging = true;
  await hydrateMatchTiers(container, tab, "scope");
  assert.equal(requests.length, 0, "客户端筛选自动翻页期间不得查询平均段位");
  w.close();
});

test("演示数据下总览页渲染出真实内容，且渲染期没有异常", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  assert.ok(overview, "缺少 #overview-content 容器");
  assert.deepEqual(errors, [], `渲染期出现异常：\n${errors.join("\n")}`);
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "总览停在骨架屏上——渲染路径抛异常了");
  assert.ok(overview.querySelector(".summoner-strip"), "总览缺少召唤师信息条");
  assert.ok(overview.querySelector(".career-column .career-section"), "总览缺少生涯统计分区");
  assert.ok(overview.querySelector(".matches-column .match-list"), "总览缺少对局列表");
	// 三块近期表现都只吃首屏最近 20 场，不再显示或依赖后台赛季扫描进度。
	const recentSection = overview.querySelector(".recent-ranked-section");
	const abilitySection = overview.querySelector(".ability-section");
	const positionSection = overview.querySelector(".position-list")?.closest(".career-section");
	for (const [label, section] of [["近 20 场排位", recentSection], ["能力表现", abilitySection], ["位置偏好", positionSection]]) {
		assert.ok(section, `${label} 缺少分区`);
		const header = section.querySelector(":scope > header");
		assert.ok(header, `${label} 缺少标题栏`);
		assert.equal(header.querySelectorAll(".season-progress-badge").length, 0, `${label} 仍依赖赛季统计进度`);
		const buttons = [...header.querySelectorAll(".ranked-queue-button")];
		assert.deepEqual(buttons.map((button) => button.textContent), ["单双排", "灵活组排"], `${label} 的队列页签被删掉了`);
		assert.deepEqual(buttons.map((button) => button.classList.contains("is-active")), [true, false], `${label} 的默认选中态不对`);
	}
	assert.equal(recentSection.querySelector("h3")?.textContent, "近 20 场排位");
	assert.match(abilitySection.querySelector(".ability-meta > span")?.textContent || "", /14 场样本/);
	assert.equal(positionSection.querySelector(".career-sample-label")?.textContent, "近 20 场");
	assert.doesNotMatch(overview.querySelector(".recent-ranked-section > header")?.textContent || "", /实际\s*20\s*场/);
	// 「过去 30 天排位」整块已删除（与近 20 场排位吃的是同一批 20 场详情战绩）。
	assert.doesNotMatch(overview.textContent || "", /过去 30 天排位/);
	const positionRows = [...overview.querySelectorAll(".position-list .position-row")];
	assert.deepEqual(positionRows.map((row) => row.querySelector("strong")?.textContent), ["上单", "打野", "中单", "下路", "辅助"]);
	assert.deepEqual(positionRows.map((row) => row.querySelector(":scope > b")?.textContent), ["46%", "10%", "28%", "16%", "0%"]);
	assert.doesNotMatch(overview.querySelector(".position-list")?.textContent || "", /其他/);
  w.close();
});

test("演示数据下随行四个页签都渲染完成", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  w.document.querySelector('[data-section="suite"]').click();
  await settled();
  for (const name of ["watch", "rig", "facade", "sweep"]) {
    const root = w.document.getElementById(`suite-${name}-root`);
    assert.ok(root, `随行缺少 ${name} 容器`);
    assert.equal(root.classList.contains("suite-loading"), false, `${name} 永久停在加载态`);
    assert.ok(root.textContent.trim().length > 20, `${name} 没有渲染出内容`);
  }
  const watchCards = [...w.document.querySelectorAll("#suite-watch-root [data-watch-card]")];
  assert.deepEqual(watchCards.map((card) => card.dataset.watchCard), ["accept", "promote-leader", "invitations", "auto-matchmaking", "reconnect", "position-broadcast", "play-again", "auto-honor", "skip-celebration"], "值守九张规则卡未按分类完整渲染");
  assert.equal(w.document.querySelectorAll("#suite-watch-root [data-watch-toggle]").length, 9, "值守规则卡缺少独立开关");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .watch-rule.is-enabled").length, 6, "已启用规则卡缺少独立高亮背景");
  assert.equal(Number.parseFloat(w.document.querySelector('[data-watch-delay="autoAccept"]')?.style.getPropertyValue("--range-fill")), 15, "自动接受延时轨道未按当前值填充");
  assert.ok(w.document.querySelector("#suite-watch-root .watch-rules-head"), "值守缺少设计稿中的规则标题区");
  assert.equal(w.document.querySelectorAll('#suite-watch-root [data-watch-choice="autoHonor.strategy"]').length, 4, "点赞策略未按设计稿渲染为四个胶囊选项");
  assert.equal(w.document.querySelectorAll('#suite-watch-root [data-watch-choice="positionBroadcast.visibility"]').length, 2, "阵营播报范围未按设计稿渲染为两个胶囊选项");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .watch-invite-summary > .watch-pill").length, 3, "邀请处理未保持三项紧凑摘要");
  const hiddenAcceptedPolicy = w.document.querySelector('.watch-invite-more [data-watch-policy-cycle="1700"]');
  assert.ok(hiddenAcceptedPolicy, "邀请详情缺少斗魂竞技场策略");
  hiddenAcceptedPolicy.click();
  await settled();
  assert.equal(w.document.querySelector('.watch-invite-summary > [data-watch-policy-cycle="1700"]')?.textContent, "斗魂竞技场 · 接受", "接受策略保存后没有移到卡片外层展示");
  assert.match(w.document.querySelector('[data-watch-card="auto-matchmaking"]')?.textContent || "", /最少人数[\s\S]*延时/, "自动匹配卡缺少设计稿参数");
  assert.ok(w.document.querySelector("#suite-rig-root .rig-layout"), "整备装置台未渲染");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-preview"), "门面预览未渲染");
  assert.ok(w.document.querySelector("#suite-sweep-root .claim-row"), "拾遗合流清单未渲染");
  assert.equal(w.document.querySelector("#suite-sweep-root > .suite-note.is-info"), null, "拾遗仍显示重新扫描下方的说明条");
  assert.equal(w.document.querySelector("#suite-sweep-root .claim-sub"), null, "拾遗奖励标题下仍显示描述文字");
  assert.deepEqual([...w.document.querySelectorAll(".suite-tab > span:first-child")].map((node) => node.textContent), ["◉", "⬢", "◈", "✦"], "随行页签未使用设计稿图形符号");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .phase-node").length, 6, "值守相位轨没有按设计稿收敛为六个主阶段");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-avatar-level"), "门面预览缺少头像等级牌");
  assert.equal(w.document.querySelectorAll("#suite-facade-root .facade-slot").length, 3, "门面预览缺少三个勋章位");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-background-card"), "门面背景控制卡未渲染在右栏");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-chat-card"), "门面聊天身份控制卡未渲染在右栏");
  assert.equal(w.document.querySelectorAll("#suite-facade-root .facade-film").length, 1, "门面背景应只保留一条皮肤胶片");
  assert.equal(w.document.querySelectorAll("#suite-facade-root [data-facade-chroma]").length, 0, "门面背景仍提供炫彩选择");
  assert.doesNotMatch(w.document.getElementById("suite-facade-root").textContent, /炫彩/, "门面背景仍宣称可以设置炫彩");
  assert.doesNotMatch(w.document.getElementById("suite-watch-root").textContent, /这些我们不做/, "值守页仍显示已要求移除的说明模块");
  assert.doesNotMatch(w.document.getElementById("suite-rig-root").textContent, /不做的能力/, "整备页仍显示已要求移除的说明模块");
  assert.deepEqual(errors, [], `随行渲染期出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("全部原生下拉都增强为可键盘操作的应用菜单，包含动态英雄段位", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const staticSelects = [...w.document.querySelectorAll(".select-wrap > select")];
  assert.ok(staticSelects.length >= 10, `静态下拉数量异常：${staticSelects.length}`);
  for (const select of staticSelects) {
    const menuRoot = select.parentElement.querySelector(":scope > .native-select-menu");
    assert.ok(menuRoot, `${select.id || select.getAttribute("aria-label") || "未命名下拉"} 未增强`);
    assert.equal(select.classList.contains("sr-only"), true);
    assert.equal(select.getAttribute("aria-hidden"), "true");
    assert.ok(menuRoot.querySelector('[role="menu"]'));
    assert.equal(menuRoot.querySelectorAll('[role="menuitemradio"]').length, select.options.length);
  }

  w.document.querySelector('[data-section="champions"]').click();
  await settled();
  const tierSelect = w.document.querySelector("#champions-panel [data-champion-tier]");
  assert.ok(tierSelect, "英雄页缺少段位下拉");
  const tierRoot = tierSelect.parentElement.querySelector(":scope > .native-select-menu");
  const trigger = tierRoot?.querySelector("[data-app-select-trigger]");
  const menu = tierRoot?.querySelector('[role="menu"]');
  assert.ok(trigger && menu, "动态段位下拉未增强为应用菜单");
  trigger.dispatchEvent(new w.KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
  assert.equal(trigger.getAttribute("aria-expanded"), "true");
  assert.equal(menu.hidden, false);
  const selectedOption = menu.querySelector('[role="menuitemradio"][aria-checked="true"]');
  assert.ok(selectedOption);
  selectedOption.dispatchEvent(new w.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  assert.equal(trigger.getAttribute("aria-expanded"), "false");
  assert.equal(menu.hidden, true);
  assert.deepEqual(errors, [], `下拉增强出现异常：\n${errors.join("\n")}`);
  w.close();
});

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

test("客户端退出后总览显示可重试提示，不退回空白页", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const overview = w.document.getElementById("overview-content");
  assert.ok(overview.querySelector(".summoner-strip"), "断连前总览应已完成渲染");
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: false } }));
  assert.match(overview.textContent, /等待英雄联盟客户端/);
  assert.match(overview.textContent, /启动并登录客户端后会自动恢复/);
  assert.ok(overview.querySelector("[data-gameplay-retry]"), "断连提示应保留手动重试入口");
  assert.ok(!overview.querySelector(".gameplay-skeleton"), "断连后不应退回骨架屏");
	assert.equal(w.document.querySelectorAll("#player-tabs [data-player-tab]").length, 0, "断连后不应残留国服玩家标签");
  await settled();
  assert.deepEqual(errors, [], `断连渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

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
