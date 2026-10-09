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

const { waitForOverview, waitForWatch, waitForFacade } = require("./renderer-wait.cjs");

installWindowCleanup(test);

function assertR59FacadeTitleContracts(source) {
	const { facadeTitleText } = compileFunctions(source, ["facadeTitleText"], {});
	const dom = new JSDOM('<section class="facade-preview"><div class="facade-signature"></div></section>');
	const preview = dom.window.document.querySelector(".facade-signature");
	const uuid = "38a4e9d4-b2f2-2356-969f-e39316e18ede";
	preview.textContent = facadeTitleText({ challengeSummary: { title: { name: "炫彩达人", contentId: uuid, itemId: 123 } } });
	assert.equal(preview.textContent, "炫彩达人", "object title 没有使用可读的 name");
	preview.textContent = facadeTitleText({ challengeSummary: { title: null }, chat: { lol: { playerTitleSelected: uuid } } });
	assert.equal(preview.textContent, "未设置头衔", "缺少可读 title 时没有回退到未设置");
	assert.doesNotMatch(dom.window.document.querySelector(".facade-preview").textContent, new RegExp(uuid), "UUID 泄漏到生涯预览");
	assert.equal(facadeTitleText({ challengeSummary: { title: "峡谷先锋" } }), "峡谷先锋", "string title 回归");
	assert.equal(facadeTitleText({ challengeSummary: { title: 123 } }), "123", "number title 回归");
	dom.window.close();
}

test("R59 生涯头衔读取对象名称且绝不显示原始 UUID", () => {
	assertR59FacadeTitleContracts(suiteSource);
	const objectBranch = '\n\tif (summaryTitle && typeof summaryTitle === "object") {\n\t  const name = summaryTitle.name;\n\t  if (typeof name === "string" && name.trim()) return name.trim();\n\t}';
	const mutated = suiteSource.replace(objectBranch, "");
	assert.notEqual(mutated, suiteSource, "R59 object title mutation 未命中真实代码");
	assert.throws(() => assertR59FacadeTitleContracts(mutated), /object title 没有使用可读的 name/, "删除 object 分支后测试必须失败");
});

test("R59 生涯预览完整渲染时不泄漏 playerTitleSelected UUID", async () => {
	const uuid = "38a4e9d4-b2f2-2356-969f-e39316e18ede";
	const { window: w, errors } = bootDemoApp({
		facadeStateTransform: (facade) => {
			facade.challengeSummary.title = null;
			facade.chat.lol.playerTitleSelected = uuid;
			return facade;
		},
	});
	await waitForOverview(w);
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await waitForFacade(w);
	const facadeRoot = w.document.getElementById("suite-facade-root");
	assert.equal(facadeRoot.querySelector(".facade-signature")?.textContent, "未设置头衔");
	assert.doesNotMatch(facadeRoot.textContent, new RegExp(uuid), "完整生涯预览泄漏了 UUID");
	assert.deepEqual(errors, [], `R59 UUID 回退渲染出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R60 生涯事件刷新头衔、保护脏草稿且不在其它页签后台请求", async () => {
	const { window: w, errors, eventSources } = bootDemoApp({ liveEvents: true });
	await waitForOverview(w);
	let nextTitle = "客户端新头衔";
	let facadeRequests = 0;
	const originalFetch = w.fetch;
	w.fetch = async (input, init) => {
	  const url = typeof input === "string" ? input : input?.url || "";
	  if (!url.startsWith("/api/facade/state") || String(init?.method || "GET").toUpperCase() !== "GET") return originalFetch(input, init);
	  facadeRequests += 1;
	  const response = await originalFetch(input, init);
	  const payload = await response.json();
	  payload.challengeSummary.title = { name: nextTitle };
	  return new w.Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
	};

	w.document.querySelector('[data-section="suite"]').click();
	await waitForWatch(w);
	w.document.querySelector('[data-suite-tab="facade"]').click();
	await new Promise((resolve) => setTimeout(resolve, 80));
	const source = eventSources.at(-1);
	assert.ok(source?.onmessage, "应用没有建立可接收 facade:changed 的事件流");

	facadeRequests = 0;
	nextTitle = "胜利皮肤收藏家";
	source.onmessage({ data: "facade:changed" });
	await new Promise((resolve) => setTimeout(resolve, 900));
	assert.equal(facadeRequests, 1, "干净草稿收到 facade:changed 后没有重新请求状态");
	assert.equal(w.document.querySelector("#suite-facade-root .facade-signature")?.textContent, "胜利皮肤收藏家");

	const status = w.document.querySelector("#suite-facade-root [data-facade-status]");
	status.value = "这段正在编辑的签名不能丢";
	status.dispatchEvent(new w.Event("input", { bubbles: true }));
	facadeRequests = 0;
	nextTitle = "焕然一新";
	source.onmessage({ data: "facade:changed" });
	await new Promise((resolve) => setTimeout(resolve, 900));
	assert.equal(facadeRequests, 1, "脏草稿刷新没有请求最新 facade 状态");
	assert.equal(w.document.querySelector("#suite-facade-root [data-facade-status]")?.value, "这段正在编辑的签名不能丢", "外部刷新覆盖了用户正在编辑的草稿");
	assert.equal(w.document.querySelector("#suite-facade-root .facade-signature")?.textContent, "焕然一新", "保留脏草稿时没有刷新头衔");

	w.document.querySelector('[data-suite-tab="watch"]').click();
	facadeRequests = 0;
	source.onmessage({ data: "facade:changed" });
	await new Promise((resolve) => setTimeout(resolve, 900));
	assert.equal(facadeRequests, 0, "不在生涯页时仍后台请求 facade 状态");
	assert.match(suiteSource, /Date\.now\(\)\s*-\s*state\.facadeLoadedAt\s*<\s*30000/, "生涯页缺少 30 秒过期兜底");
	assert.deepEqual(errors, [], `R60 生涯刷新出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R65 生涯事件 20 连发只请求一次、仅时间戳变化不重建且按压期间不丢点击", async (t) => {
	let facadeVersion = 0;
	const { window: w, errors } = bootDemoApp({
		facadeStateTransform: (facade) => {
			facade.chat.lastSeenOnlineTimestamp = ++facadeVersion;
			return facade;
		},
	});
	t.after(() => w.close());
	await waitForOverview(w);
	w.document.querySelector('[data-section="suite"]').click();
	await waitForWatch(w);
	w.document.querySelector('[data-suite-tab="facade"]').click();
	await new Promise((resolve) => setTimeout(resolve, 100));
	const facadeRoot = w.document.getElementById("suite-facade-root");
	const originalFetch = w.fetch;
	let facadeRequests = 0;
	w.fetch = (input, init) => {
		const url = typeof input === "string" ? input : input?.url || "";
		if (url.startsWith("/api/facade/state") && String(init?.method || "GET").toUpperCase() === "GET") facadeRequests += 1;
		return originalFetch(input, init);
	};
	const stableNode = facadeRoot.querySelector(".facade-preview");
	let rebuilds = 0;
	const observer = new w.MutationObserver((mutations) => {
		rebuilds += mutations.filter((mutation) => mutation.type === "childList").length;
	});
	observer.observe(facadeRoot, { childList: true });
	for (let index = 0; index < 20; index += 1) w.dispatchEvent(new w.CustomEvent("deep-legends:facade-changed"));
	await new Promise((resolve) => setTimeout(resolve, 900));
	observer.disconnect();
	assert.ok(facadeRequests <= 1, `20 次 facade 事件触发了 ${facadeRequests} 次请求`);
	assert.equal(facadeRoot.querySelector(".facade-preview"), stableNode, "只有聊天时间戳变化时仍重建了 DOM");
	assert.equal(rebuilds, 0, `只有聊天时间戳变化时发生了 ${rebuilds} 次 DOM 子树重建`);

	const target = [...facadeRoot.querySelectorAll("[data-facade-skin]")].find((button) => !button.classList.contains("is-selected"));
	assert.ok(target, "演示数据缺少可点击的第二张皮肤");
	let clicked = 0;
	target.addEventListener("click", () => { clicked += 1; });
	const targetID = target.dataset.facadeSkin;
	target.dispatchEvent(new w.MouseEvent("mousedown", { bubbles: true }));
	w.dispatchEvent(new w.CustomEvent("deep-legends:facade-changed"));
	await new Promise((resolve) => setTimeout(resolve, 850));
	assert.equal(facadeRequests, 1, "鼠标仍按下时不应刷新 facade");
	target.dispatchEvent(new w.MouseEvent("mouseup", { bubbles: true }));
	target.click();
	assert.equal(clicked, 1, "mousedown 到 mouseup 之间的 facade 事件吞掉了 click");
	assert.equal(facadeRoot.querySelector("[data-facade-skin].is-selected")?.dataset.facadeSkin, targetID);
	await new Promise((resolve) => setTimeout(resolve, 50));
	assert.deepEqual(errors, [], `R64 facade 事件保护出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R64 生涯皮肤与身份控件局部更新，皮肤图片延迟解码", async () => {
	const { window: w, errors } = bootDemoApp({facadeStateTransform: facade => ({...facade, skins: facade.skins.map(skin => ({...skin, splashPath: `/lol-game-data/assets/demo/${skin.id}/splash.jpg`}))})});
	await waitForOverview(w);
	w.document.querySelector('[data-section="suite"]').click();
	await waitForWatch(w);
	w.document.querySelector('[data-suite-tab="facade"]').click();
	await new Promise((resolve) => setTimeout(resolve, 100));
	const root = w.document.getElementById("suite-facade-root");
	const hero = root.querySelector("[data-facade-hero]");
	const heroMenu = hero._appSelectRoot;
	const buttons = [...root.querySelectorAll("[data-facade-skin]")];
	const target = buttons.find((button) => !button.classList.contains("is-selected"));
	const previewBefore = root.querySelector("[data-suite-facade-art]")?.getAttribute("data-queued-src");
	assert.ok(target && previewBefore, "演示数据不足以验证皮肤切换");
	target.click();
	const buttonsAfter = [...root.querySelectorAll("[data-facade-skin]")];
	assert.equal(buttonsAfter.length, buttons.length);
	buttonsAfter.forEach((button, index) => assert.equal(button, buttons[index], "点击皮肤重建了皮肤按钮"));
	assert.equal(target.classList.contains("is-selected"), true);
	assert.equal(root.querySelector("[data-suite-facade-art]")?.getAttribute("data-queued-src"), previewBefore, "未应用时当前背景不应改变");
	for (const image of root.querySelectorAll(".facade-film img")) {
		assert.equal(image.getAttribute("loading"), "lazy");
		assert.equal(image.getAttribute("decoding"), "async");
	}

	const availabilityNode = root.querySelector("[data-facade-preview-availability]");
	root.querySelector('[data-facade-availability="away"]').click();
	assert.equal(root.querySelector("[data-facade-preview-availability]"), availabilityNode, "在线状态切换重建了预览");
	assert.equal(availabilityNode.textContent, "离开");
	const rankNode = root.querySelector("[data-facade-preview-rank]");
	const tier = root.querySelector('[data-facade-rank="tier"]');
	tier.value = "MASTER";
	tier.dispatchEvent(new w.Event("change", { bubbles: true }));
	assert.equal(root.querySelector("[data-facade-preview-rank]"), rankNode, "段位切换重建了预览");
	assert.match(rankNode.textContent, /大师/);

	const nextHero = [...hero.options].find((option) => option.value !== hero.value);
	assert.ok(nextHero, "演示数据缺少第二个英雄");
	hero.value = nextHero.value;
	hero.dispatchEvent(new w.Event("change", { bubbles: true }));
	assert.equal(root.querySelector("[data-facade-hero]"), hero, "换英雄时重建了英雄 select");
	assert.equal(hero._appSelectRoot, heroMenu, "换英雄时重建了增强下拉");
	assert.equal(root.querySelector("[data-suite-facade-art]")?.getAttribute("data-queued-src"), previewBefore, "换英雄时当前背景不应改变或清空");
	assert.deepEqual(errors, [], `R64 生涯局部更新出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R64 生涯头衔和勋章仅在真实值上使用强调色", async () => {
	const { window: w, errors } = bootDemoApp({
		facadeStateTransform: (facade) => {
			facade.challengeSummary.title = null;
			facade.challenges = [{ id: "1", name: "真实勋章" }];
			return facade;
		},
	});
	await waitForOverview(w);
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await waitForFacade(w);
	const root = w.document.getElementById("suite-facade-root");
	const title = root.querySelector(".facade-signature");
	const slots = [...root.querySelectorAll("[data-facade-challenge-slot]")];
	assert.equal(title.textContent, "未设置头衔");
	assert.equal(title.classList.contains("is-filled"), false, "头衔占位态被当作真实值上色");
	assert.equal(slots[0].classList.contains("is-filled"), true);
	assert.equal(slots[1].classList.contains("is-filled"), false, "勋章占位态被当作真实值上色");
	assert.match(suiteStyles, /\.facade-signature\s*\{[^}]*place-items:\s*center[^}]*text-align:\s*center/s);
	assert.match(suiteStyles, /\.facade-signature\.is-filled\s*\{[^}]*color:\s*var\(--primary-strong\)/s);
	assert.match(suiteStyles, /\.facade-slot\.is-filled\s*\{[^}]*color:\s*var\(--accent\)/s);
	assert.deepEqual(errors, [], `R64 头衔勋章样式出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R60 生涯勋章缺失槽位稳定回退为未设置", () => {
	const { facadeChallengeSlots } = compileFunctions(suiteSource, ["facadeChallengeSlots"], {
	  state: { facade: {} },
	  escapeHTML: (value) => String(value),
	});
	const dom = new JSDOM(`<div>${facadeChallengeSlots({ challenges: [{ id: "1", name: "唯一勋章" }] })}</div>`);
	assert.deepEqual([...dom.window.document.querySelectorAll("[data-facade-challenge-slot]")].map((node) => node.textContent), ["唯一勋章", "未设置", "未设置"]);
	assert.doesNotMatch(dom.window.document.body.textContent, /undefined|null/);
	dom.window.close();
});

test("R58 生涯英雄长下拉支持别名搜索，短下拉不显示搜索框", async () => {
	const { window: w, errors } = bootDemoApp();
	await waitForOverview(w);
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await waitForFacade(w);
	const heroSelect = w.document.querySelector("[data-facade-hero]");
	assert.ok(heroSelect.options.length > 20, `演示英雄选项不足以触发长下拉：${heroSelect.options.length}`);
	const root = heroSelect.parentElement.querySelector(":scope > .native-select-menu");
	const trigger = root.querySelector("[data-app-select-trigger]");
	const menu = root.querySelector("[data-app-select-menu]");
	const optionsRoot = root.querySelector("[data-app-select-options]");
	const search = root.querySelector("[data-app-select-search-input]");
	assert.ok(search, "长下拉没有搜索输入框");
	assert.ok(root.querySelector(".app-select-search-icon"), "长下拉没有应用内搜索图标");
	assert.match(appStyles, /\.app-select-menu\s*\{[^}]*display:\s*flex[^}]*flex-direction:\s*column[^}]*overflow:\s*hidden/s, "搜索栏和选项必须使用纵向弹性布局与独立滚动层");
	assert.match(appStyles, /\.app-select-options\s*\{[^}]*flex:\s*1\s+1\s+auto[^}]*min-height:\s*0[^}]*overflow-y:\s*auto/s, "英雄名称列表缺少可收缩的独立滚动容器");
	assert.match(appStyles, /\.app-select-search input\s*\{[^}]*font-size:\s*12px[^}]*font-weight:\s*400/s, "搜索提示语没有缩小并减轻字重");
	trigger.click();
	assert.equal(w.document.activeElement, search, "长下拉打开后没有自动聚焦搜索框");
	assert.equal(menu.scrollTop, 0, "打开长下拉时不应把搜索栏卷出菜单");
	assert.ok(optionsRoot.scrollTop >= 0, "长下拉应滚动选项列表而不是整个菜单");
	search.value = "adk";
	search.dispatchEvent(new w.Event("input", { bubbles: true }));
	const buttons = [...root.querySelectorAll("[data-native-select-value]")];
	const visible = buttons.filter((button) => !button.hidden).map((button) => button.querySelector("span")?.textContent);
	assert.deepEqual(visible, ["阿卡丽"], `别名搜索结果不精确：${visible.join(", ")}`);
	assert.ok(buttons.some((button) => button.hidden), "搜索过滤被短路，所有选项仍然可见");
	w.document.body.dispatchEvent(new w.Event("pointerdown", { bubbles: true }));
	assert.equal(trigger.getAttribute("aria-expanded"), "false");
	assert.equal(search.value, "", "关闭长下拉后没有清空搜索词");
	assert.ok(buttons.every((button) => !button.hidden), "关闭长下拉后没有恢复全部选项");
	trigger.click();
	const firstOpenButtons = [...optionsRoot.querySelectorAll('[role="menuitemradio"]')];
	w.document.body.dispatchEvent(new w.Event("pointerdown", { bubbles: true }));
	trigger.click();
	const secondOpenButtons = [...optionsRoot.querySelectorAll('[role="menuitemradio"]')];
	assert.equal(secondOpenButtons.length, firstOpenButtons.length);
	secondOpenButtons.forEach((button, index) => assert.equal(button, firstOpenButtons[index], "相同英雄选项第二次打开时仍重建按钮"));
	heroSelect.value = heroSelect.options[1].value;
	w.deepLegendsSelects.sync(heroSelect);
	assert.equal(optionsRoot.querySelector(`[data-native-select-value="${heroSelect.value}"]`)?.getAttribute("aria-checked"), "true", "未重建时没有同步选中态");

	const queueSelect = w.document.querySelector('[data-facade-rank="queue"]');
	const queueRoot = queueSelect.parentElement.querySelector(":scope > .native-select-menu");
	assert.equal(queueSelect.options.length, 2);
	assert.equal(queueRoot.querySelector("[data-app-select-search-input]"), null, "短下拉不应创建搜索框");
	assert.deepEqual(errors, [], `长下拉搜索出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("R58 生涯空皮肤刷新保留草稿并在数据补齐后恢复", async (t) => {
	const draft = { hero: "99", skinId: 99001, ownedOnly: false };
	const state = { facadeDraft: draft, facade: { profile: {backgroundSkinId:99001}, skins: [], chat: {}, loginReset: {} } };
	const { hydrateFacadeDraft } = compileFunctions(suiteSource, ["hydrateFacadeDraft"], { state });
	hydrateFacadeDraft(true);
	assert.equal(state.facadeDraft.hero, "99");
	assert.equal(state.facadeDraft.skinId, 99001);
	state.facade = { skins: [{ id: 103028, championId: 103, owned: true }], profile: { backgroundSkinId: 103028 }, chat: {}, loginReset: {} };
	hydrateFacadeDraft(true);
	assert.equal(state.facadeDraft.hero, "103");
	assert.equal(state.facadeDraft.skinId, 103028);

	const unknownState = { facadeDraft: { hero: "99", skinId: 99001 }, facade: { profileUnavailable:true, skins: [], chat: {}, loginReset: {} } };
	compileFunctions(suiteSource, ["hydrateFacadeDraft"], { state: unknownState }).hydrateFacadeDraft(true);
	assert.equal(unknownState.facadeDraft.hero, "", "unknown current profile must not be replaced by a stale draft");
	assert.equal(unknownState.facadeDraft.skinId, 0);

	const { window: w, errors } = bootDemoApp();
	t.after(() => w.close());
	await waitForOverview(w);
	w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
	await waitForFacade(w);
	const initial = await (await w.fetch("/api/facade/state")).json();
	const originalFetch = w.fetch;
	let skinsReady = false;
	w.fetch = (input, init) => String(input).startsWith("/api/facade/state")
		? Promise.resolve(new w.Response(JSON.stringify({ ...initial, skins: skinsReady ? initial.skins : [] }), { status: 200, headers: { "Content-Type": "application/json" } }))
		: originalFetch(input, init);
	const refreshFacade = async () => {
		// Empty same-account snapshots must retain the draft; disconnect is a separate invalidation boundary.
		w.dispatchEvent(new w.CustomEvent("deep-legends:facade-changed"));
		await new Promise((resolve) => setTimeout(resolve, 1050));
	};
	const selectedBefore = w.document.querySelector("[data-facade-hero]").value;
	await refreshFacade();
	assert.equal(w.document.querySelector("[data-facade-hero]").value, selectedBefore, "同步未完成时背景英雄被清空");
	assert.doesNotMatch(w.document.querySelector(".facade-art-label").textContent, /未设置/);
	skinsReady = true;
	await refreshFacade();
	assert.equal(w.document.querySelector("[data-facade-hero]").value, selectedBefore, "皮肤数据补齐后背景没有恢复");
	assert.match(w.document.querySelector(".facade-art-label").textContent, /星之守护者 拉克丝/);
	assert.deepEqual(errors, [], `空皮肤恢复流程出现异常：\n${errors.join("\n")}`);
	w.close();
});

test("顶部重新读取实际刷新生涯并丢弃未应用预览", async (t) => {
  const {window:w,errors}=bootDemoApp();
  t.after(()=>w.close());
  await waitForOverview(w);
  w.document.querySelector('[data-section="suite"]').click();
  await waitForWatch(w);
  w.document.querySelector('[data-suite-tab="facade"]').click();
  const current=await (await w.fetch('/api/facade/state')).json();
  const original=Number(current.profile.backgroundSkinId);
  for (let attempt=0;attempt<50 && !w.document.querySelector("[data-facade-skin]");attempt++) await new Promise(resolve=>setTimeout(resolve,10));
  const choice=[...w.document.querySelectorAll('[data-facade-skin]')].find(b=>Number(b.dataset.facadeSkin)!==original);
  assert.ok(choice);
  choice.click();
  assert.match(w.document.querySelector('.facade-art-label').textContent,/当前背景/);
  assert.equal(choice.classList.contains('is-selected'),true);
  const requests=[], fetch=w.fetch;
  w.fetch=(input,init)=>{requests.push(String(input));return fetch(input,init);};
  const detail={waitFor:[],reason:'manual'};
  w.dispatchEvent(new w.CustomEvent('deep-legends:hard-refresh',{detail}));
  assert.ok(detail.waitFor.length>0);
  await Promise.all(detail.waitFor);
  assert.ok(requests.includes('/api/facade/state?trigger=manual'));
  assert.equal(Number(w.document.querySelector('[data-facade-skin].is-selected')?.dataset.facadeSkin),original);
  assert.match(w.document.querySelector('.facade-art-label').textContent,/当前背景/);
  assert.deepEqual(errors,[]);
});
