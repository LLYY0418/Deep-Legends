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

const { waitForOverview, waitForRender, waitForWatch } = require("./renderer-wait.cjs");

installWindowCleanup(test);

test("演示数据下工具五个页签都渲染完成", async () => {
  const { window: w, errors } = bootDemoApp();
  await waitForOverview(w, 17);
  w.document.querySelector('[data-section="suite"]').click();
  await waitForWatch(w);
  for (const name of ["watch", "rig", "facade", "sweep", "champselect"]) {
    await visitTool(w, name);
    const root = w.document.getElementById(`suite-${name}-root`);
    assert.ok(root, `工具缺少 ${name} 容器`);
    assert.equal(root.classList.contains("suite-loading"), false, `${name} 永久停在加载态`);
    assert.ok(root.textContent.trim().length > 20, `${name} 没有渲染出内容`);
  }
  const watchCards = [...w.document.querySelectorAll("#suite-watch-root [data-watch-card]")];
  assert.deepEqual(watchCards.map((card) => card.dataset.watchCard), ["accept", "promote-leader", "invitations", "auto-matchmaking", "reconnect", "position-broadcast", "play-again", "auto-honor", "skip-celebration"], "自动页九张规则卡未按分类完整渲染");
  assert.equal(w.document.querySelectorAll("#suite-watch-root [data-watch-toggle]").length, 9, "自动规则卡缺少独立开关");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .watch-rule.is-enabled").length, 6, "已启用规则卡缺少独立高亮背景");
  assert.equal(Number.parseFloat(w.document.querySelector('[data-watch-delay="autoAccept"]')?.style.getPropertyValue("--range-fill")), 15, "自动接受延时轨道未按当前值填充");
  assert.ok(w.document.querySelector("#suite-watch-root .watch-rules-head"), "自动页缺少设计稿中的规则标题区");
  assert.equal(w.document.querySelectorAll('#suite-watch-root [data-watch-choice="autoHonor.strategy"]').length, 4, "点赞策略未按设计稿渲染为四个胶囊选项");
  assert.equal(w.document.querySelectorAll('#suite-watch-root [data-watch-choice="positionBroadcast.visibility"]').length, 2, "阵营播报范围未按设计稿渲染为两个胶囊选项");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .watch-invite-summary > .watch-pill").length, 3, "邀请处理未保持三项紧凑摘要");
  const hiddenAcceptedPolicy = w.document.querySelector('.watch-invite-more [data-watch-policy-cycle="1700"]');
  assert.ok(hiddenAcceptedPolicy, "邀请详情缺少斗魂竞技场策略");
  hiddenAcceptedPolicy.click();
  await waitForRender(w, () => w.document.querySelector('.watch-invite-summary > [data-watch-policy-cycle="1700"]')?.textContent === "斗魂竞技场 · 接受", "accepted invitation policy did not render");
  assert.equal(w.document.querySelector('.watch-invite-summary > [data-watch-policy-cycle="1700"]')?.textContent, "斗魂竞技场 · 接受", "接受策略保存后没有移到卡片外层展示");
  assert.match(w.document.querySelector('[data-watch-card="auto-matchmaking"]')?.textContent || "", /最少人数[\s\S]*延时/, "自动匹配卡缺少设计稿参数");
  assert.ok(w.document.querySelector("#suite-rig-root .rig-layout"), "维护页未渲染");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-preview"), "生涯预览未渲染");
  assert.ok(w.document.querySelector("#suite-sweep-root .claim-row"), "领奖清单未渲染");
  assert.equal(w.document.querySelector("#suite-sweep-root > .suite-note.is-info"), null, "领奖页仍显示重新扫描下方的说明条");
  assert.equal(w.document.querySelector("#suite-sweep-root .claim-sub"), null, "领奖标题下仍显示描述文字");
  assert.deepEqual([...w.document.querySelectorAll(".suite-tab")].map((node) => node.dataset.suiteTab), ["watch", "champselect", "facade", "sweep", "rig"], "维护应移至领奖后面");
  const suiteIcons = [...w.document.querySelectorAll(".suite-tab-icon svg")];
  assert.equal(suiteIcons.length, 5, "五个工具图标应统一使用 SVG");
  for (const icon of suiteIcons) {
    assert.equal(icon.getAttribute("viewBox"), "0 0 24 24");
    assert.equal(icon.getAttribute("width"), "26");
    assert.equal(icon.getAttribute("height"), "26");
    assert.equal(icon.getAttribute("focusable"), "false");
    assert.equal(icon.parentElement.getAttribute("aria-hidden"), "true");
  }
	assert.equal(w.document.querySelectorAll("#suite-champselect-root .cs-mode-item").length, 6, "征召页缺少六个模式分组");
	assert.equal(w.document.querySelectorAll("#suite-champselect-root .cs-seq-card").length, 3, "征召页缺少禁用、选用和备战席三张序列卡");
	assert.equal(w.document.querySelectorAll("#suite-champselect-root .cs-rail-slot.is-empty").length, 3, "征召传送带未按模式上限渲染空槽");
  assert.equal(w.document.querySelectorAll("#suite-watch-root .phase-node").length, 6, "自动页相位轨没有按设计稿收敛为六个主阶段");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-avatar-level"), "生涯预览缺少头像等级牌");
  assert.equal(w.document.querySelectorAll("#suite-facade-root .facade-slot").length, 3, "生涯预览缺少三个勋章位");
	assert.equal(w.document.querySelector("#suite-facade-root .facade-signature")?.textContent, "峡谷先锋", "生涯预览没有显示挑战头衔");
	assert.deepEqual([...w.document.querySelectorAll("#suite-facade-root [data-facade-challenge-slot]")].map((node) => node.textContent), ["不破不立", "峡谷收藏家", "团队之星"], "生涯预览没有渲染后端解析的三个勋章名称");
	assert.doesNotMatch(w.document.getElementById("suite-facade-root").textContent, /勋章 1|勋章 2|勋章 3/, "生涯预览仍在显示勋章占位编号");
	assert.ok(w.document.querySelector('[data-facade-availability="spectating"]'), "生涯页缺少观战中状态");
	assert.ok(w.document.querySelector('[data-facade-clear="clear-title"]'), "展示清理缺少卸下头衔动作");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-background-card"), "生涯背景控制卡未渲染在右栏");
  assert.ok(w.document.querySelector("#suite-facade-root .facade-chat-card"), "生涯聊天身份控制卡未渲染在右栏");
  assert.equal(w.document.querySelectorAll("#suite-facade-root .facade-film").length, 1, "生涯背景应只保留一个皮肤网格");
  assert.equal(w.document.querySelectorAll("#suite-facade-root [data-facade-chroma]").length, 0, "生涯背景仍提供炫彩选择");
  assert.doesNotMatch(w.document.getElementById("suite-facade-root").textContent, /炫彩/, "生涯背景仍宣称可以设置炫彩");
  assert.doesNotMatch(w.document.getElementById("suite-watch-root").textContent, /这些我们不做/, "自动页仍显示已要求移除的说明模块");
  assert.doesNotMatch(w.document.getElementById("suite-rig-root").textContent, /不做的能力/, "维护页仍显示已要求移除的说明模块");
  assert.deepEqual(errors, [], `工具页渲染期出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("2351 自定义暂停事件保留真实总开关和卡片高亮", async () => {
  const { window: w, errors } = bootDemoApp();
  try {
    await waitForOverview(w, 17);
    w.document.querySelector('[data-section="suite"]').click();
    await waitForWatch(w);
    const root = w.document.getElementById("suite-watch-root");
    const enabled = root.querySelectorAll(".watch-rule.is-enabled").length;
    assert.ok(enabled > 0);
    w.dispatchEvent(new w.CustomEvent("deep-legends:watch", { detail: "watch:session:paused_custom" }));
    assert.equal(root.querySelector("[data-watch-master]").checked, true);
    assert.equal(root.querySelectorAll(".watch-rule.is-enabled:not(.is-paused)").length, enabled);
    assert.match(root.textContent, /自定义对局已暂停/);
    assert.doesNotMatch(root.textContent, /总开关已关闭/);
    w.dispatchEvent(new w.CustomEvent("deep-legends:watch", { detail: "watch:session:resumed" }));
    assert.doesNotMatch(root.textContent, /自定义对局已暂停/);
    assert.equal(root.querySelector("[data-watch-master]").checked, true);
    await new Promise((resolve) => w.requestAnimationFrame(resolve));
    assert.deepEqual(errors, []);
  } finally { w.close(); }
});

test("R149 收藏页仍可独立进入头像与旗帜视图", async () => {
  const { window: w, errors } = bootDemoApp();
  try {
    await waitForOverview(w, 17);
    for (const view of ["banners", "icons"]) {
      w.document.querySelector('[data-section="suite"]').click();
      await visitTool(w, "facade");
      assert.equal(w.document.querySelector("#suite-facade-root .facade-icon-card, #suite-facade-root .facade-banner-card"), null);
      w.document.querySelector('[data-section="favorites"]').click();
      w.document.querySelector('[data-favorites-page="facade-collection"]').click();
      w.document.getElementById(`facade-view-${view}`).click();
      await waitForRender(w, () => w.document.getElementById(`facade-view-${view}`).getAttribute("aria-selected") === "true"
        && !w.document.getElementById("favorites-facade-panel").hidden, "facade collection did not open");
      assert.equal(w.document.querySelector('[data-section="favorites"]').getAttribute("aria-selected"), "true");
      assert.equal(w.document.querySelector('[data-favorites-page="facade-collection"]').getAttribute("aria-selected"), "true");
      assert.equal(w.document.getElementById("favorites-facade-panel").hidden, false);
      assert.equal(w.document.getElementById(`facade-view-${view}`).getAttribute("aria-selected"), "true");
    }
    assert.deepEqual(errors, []);
  } finally { w.close(); }
});

test("R56 工具页状态、确认、下拉与领奖契约完整", async () => {
  const { window: w, errors } = bootDemoApp();
  await waitForOverview(w, 17);
  w.document.querySelector('[data-section="suite"]').click();
  await visitTool(w, "facade");
  await visitTool(w, "sweep");
  await settled();

  const facadeSelects = [...w.document.querySelectorAll("#suite-facade-root .suite-select")];
  assert.equal(facadeSelects.length, 4, "生涯页应保留英雄和三个段位下拉");
  for (const select of facadeSelects) {
    assert.ok(select.parentElement?.classList.contains("select-wrap"), "生涯下拉未包在 .select-wrap 中");
    assert.ok(select.parentElement.querySelector(":scope > .native-select-menu .app-select-menu"), "生涯下拉未增强为应用菜单");
  }

  const facadeText = w.document.getElementById("suite-facade-root").textContent;
  assert.match(facadeText, /只在你点击后执行/);
  assert.match(facadeText, /“登录时重设”两项例外/);
  assert.match(facadeText, /生涯背景[\s\S]*你生涯页顶部的那张大图/);
  assert.match(facadeText, /好友悬浮卡[\s\S]*别人点你头像时看到的在线状态、签名和段位/);
  assert.match(facadeText, /生涯页展示[\s\S]*头像框、挑战勋章、表情轮盘/);
	assert.match(facadeText, /卸下全部勋章[\s\S]*保留旗帜和当前头衔[\s\S]*无法保留头衔，本次操作会中止并提示/);
	assert.doesNotMatch(facadeText, /切换上赛季旗帜|挑战旗帜配色/);
  assert.equal(w.document.querySelector('#suite-facade-root [data-facade-browse]'), null);
  assert.equal(w.document.querySelector('#suite-facade-root [data-facade-rank-banner]'), null);
	assert.doesNotMatch(facadeText, /头衔可能同时卸下/);
  assert.doesNotMatch(facadeText, /展示位/);
  assert.equal(w.document.querySelector("[data-facade-owned]").checked, false, "只显示已拥有不应默认开启");

  const choiceRow = [...w.document.querySelectorAll("#suite-sweep-root .claim-row")].find((row) => row.querySelectorAll(".suite-tile").length === 3 && row.querySelector("[data-claim-choice]"));
  assert.ok(choiceRow, "演示数据缺少三选一奖励");
  assert.match(choiceRow.textContent, /3 选 1/);
  assert.doesNotMatch(choiceRow.textContent, /1 选 1|1 - 1 选/);

  const master = w.document.querySelector("[data-watch-master]");
  master.checked = false;
  master.dispatchEvent(new w.Event("change", { bubbles: true }));
  await new Promise((resolve) => setTimeout(resolve, 80));
  assert.equal(w.document.getElementById("suite-watch-metric").textContent, "已暂停");
  assert.match(w.document.getElementById("suite-watch-root").textContent, /自动规则已暂停/);
  assert.ok(w.document.querySelector("#suite-watch-root .watch-rule.is-paused"));

  let nativeConfirmCalls = 0;
  w.confirm = () => { nativeConfirmCalls += 1; throw new Error("native confirm must not run"); };
  const originalFetch = w.fetch;
  const requests = [];
  w.fetch = (...args) => { requests.push(String(args[0])); return originalFetch(...args); };
  w.document.querySelector('[data-facade-clear="clear-emotes"]').click();
  await new Promise((resolve) => w.requestAnimationFrame(resolve));
  const confirmation = w.document.querySelector(".suite-confirm-card");
  assert.ok(confirmation, "清空表情轮盘未打开应用内确认卡");
  assert.equal(w.document.querySelector("[inert]"), null, "确认卡不应阻塞页面其它内容");
  // P3-7：确认卡与 toast 共用右下角锚点，卡片打开时 toast 必须被抬到卡片上方，
  // 否则 P2-4 的「上一个生涯写入尚未完成，请稍候」在屏幕上被完全遮住。
  assert.equal(w.document.body.dataset.suiteConfirmOpen, "true", "确认卡打开时必须标记 body 以抬起 toast");
  assert.match(w.document.documentElement.style.getPropertyValue("--suite-confirm-clearance"), /^\d+px$/, "确认卡打开时必须写入实测避让高度");
  const confirmCancelButton = confirmation.querySelector("[data-suite-confirm-cancel]");
  const confirmAcceptButton = confirmation.querySelector("[data-suite-confirm-accept]");
  const pressTab = (shiftKey) => {
    const event = new w.KeyboardEvent("keydown", { key: "Tab", shiftKey, bubbles: true, cancelable: true });
    w.document.dispatchEvent(event);
    return event;
  };
  confirmCancelButton.focus();
  assert.equal(pressTab(false).defaultPrevented, true, "确认卡打开时 Tab 必须被焦点陷阱接管");
  assert.equal(w.document.activeElement, confirmAcceptButton, "Tab 必须移到卡片内的确认按钮，不能跑进背景");
  pressTab(false);
  assert.equal(w.document.activeElement, confirmCancelButton, "Tab 到末尾必须循环回第一个按钮");
  pressTab(true);
  assert.equal(w.document.activeElement, confirmAcceptButton, "Shift+Tab 必须反向循环");
  assert.ok(confirmation.contains(w.document.activeElement), "焦点必须始终留在确认卡内");
  confirmation.querySelector("[data-suite-confirm-cancel]").click();
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(w.document.body.dataset.suiteConfirmOpen, undefined, "确认卡关闭后必须撤销 toast 避让");
  assert.equal(w.document.documentElement.style.getPropertyValue("--suite-confirm-clearance"), "", "确认卡关闭后必须清掉避让高度");
  assert.equal(nativeConfirmCalls, 0);
	assert.equal(requests.filter((request) => request === "/api/facade/apply").length, 0, "取消确认后不应发出生涯写请求");
	w.document.querySelector('[data-facade-clear="clear-title"]').click();
	await new Promise((resolve) => w.requestAnimationFrame(resolve));
	const titleConfirmation = w.document.querySelector(".suite-confirm-card");
	assert.ok(titleConfirmation, "卸下头衔未打开应用内确认卡");
	titleConfirmation.querySelector("[data-suite-confirm-cancel]").click();
	await new Promise((resolve) => setTimeout(resolve, 20));
	assert.equal(requests.filter((request) => request === "/api/facade/apply").length, 0, "取消卸下头衔后不应发出生涯写请求");

  const clearObjectives = w.document.querySelector('[data-facade-clear="clear-objectives"]');
  assert.equal(clearObjectives.closest(".facade-action").querySelector("p").textContent, "把当前未读任务与活动系列标记为已读，不领取奖励、不更改任务进度。");
  assert.equal(w.document.querySelector("[data-objective-diagnostics]"), null);
  const facadeWrites = [];
  w.fetch = (...args) => {
    if (String(args[0]) === "/api/facade/apply") facadeWrites.push(JSON.parse(args[1].body));
    requests.push(String(args[0]));
    return originalFetch(...args);
  };
  clearObjectives.click();
  await settled();
  assert.equal(w.document.querySelector(".suite-confirm-card"), null, "清空任务数量提示不应二次确认");
  assert.equal(nativeConfirmCalls, 0);
  assert.deepEqual(facadeWrites, [{ action: "clear-objectives" }], "点击清空任务数量提示应直接提交一次");

  w.dispatchEvent(new w.CustomEvent("deep-legends:live-disconnected"));
  assert.match(w.document.getElementById("suite-rig-root").textContent, /事件流已断开/);

  requests.length = 0;
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: false, eventStream: false } }));
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: true, eventStream: true } }));
  await new Promise((resolve) => setTimeout(resolve, 80));
	const suiteEndpoints = ["/api/watch/rules", "/api/rig/status", "/api/facade/state?trigger=poll", "/api/claim/scan", "/api/champselect/groups", "/api/champselect/state", "/api/champions/catalog"];
  const restoredRequests = requests.filter((request) => suiteEndpoints.includes(request));
  assert.deepEqual(restoredRequests.sort(), ["/api/claim/scan", "/api/facade/state?trigger=poll", "/api/rig/status"], `离线恢复应强刷当前领奖页和 rig，并预加载生涯，实际 ${requests.join(", ")}`);
  w.dispatchEvent(new w.CustomEvent("deep-legends:status", { detail: { connected: true, eventStream: true } }));
  await new Promise((resolve) => setTimeout(resolve, 80));
  assert.equal(requests.filter((request) => suiteEndpoints.includes(request)).length, restoredRequests.length, "重复在线状态不应再次请求工具接口");

  assert.match(appStyles, /\.button-danger\s*\{[^}]*background\s*:/s);
  assert.match(appStyles, /\.app-select-menu\s*\{[^}]*display\s*:\s*flex[^}]*flex-direction\s*:\s*column[^}]*max-height\s*:\s*min\(320px,\s*calc\(50vh\s*\/\s*var\(--ui-zoom,\s*1\)\)\)[^}]*overflow\s*:\s*hidden[^}]*overscroll-behavior\s*:\s*contain/s);
  assert.match(appStyles, /\.app-select-options\s*\{[^}]*flex\s*:\s*1\s+1\s+auto[^}]*overflow-y\s*:\s*auto[^}]*overscroll-behavior\s*:\s*contain/s);
  assert.match(fs.readFileSync(path.join(WEB, "app.js"), "utf8"), /selected\.offsetTop\s*-\s*optionsRoot\.offsetTop\s*-\s*\(optionsRoot\.clientHeight\s*-\s*selected\.offsetHeight\)\s*\/\s*2/);
  assert.match(fs.readFileSync(path.join(WEB, "app.js"), "utf8"), /pageScrollRoot\.scrollTop\s*=\s*pageScrollTop/, "下拉打开后没有恢复页面主滚动位置");
  assert.match(suiteStyles, /\.facade-film\s*\{[^}]*grid-template-columns\s*:\s*repeat\(auto-fill,[^}]*overflow-x\s*:\s*hidden[^}]*overflow-y\s*:\s*auto/s);
  assert.match(suiteStyles, /\.facade-preview-art\s*\{[^}]*aspect-ratio\s*:\s*1215\s*\/\s*717[^}]*height\s*:\s*auto/s);
  assert.match(suiteStyles, /@container suite-page \(max-width:\s*1060px\)\s*\{[^}]*\.facade-row\s*\{[^}]*grid-template-columns\s*:\s*1fr[^}]*\}/s, "中等宽度生涯设置行未切换为单列");
  assert.match(suiteStyles, /@media \(max-width:\s*1250px\)\s*\{[^}]*\.facade-row\s*\{[^}]*grid-template-columns\s*:\s*1fr[^}]*\}/s, "1250px 以下生涯设置行未切换为单列");
  assert.doesNotMatch(fs.readFileSync(path.join(WEB, "suite.js"), "utf8"), /\bconfirm\s*\(/);
	assert.doesNotMatch(suiteSource, /requestedAvailability\s*===\s*["']dnd["']\s*&&/, "在线状态回弹仍只处理游戏中");
  assert.deepEqual(errors, [], `R55 工具页渲染出现异常：\n${errors.join("\n")}`);
  w.close();
});

test("全部原生下拉都增强为可键盘操作的应用菜单，包含动态英雄段位", async () => {
  const { window: w, errors } = bootDemoApp();
  await waitForOverview(w, 17);
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
  await waitForRender(w, () => w.document.querySelector('#champions-panel [data-champion-tier]')?.parentElement.querySelector('[data-app-select-trigger]'), "champion tier menu did not render");
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
