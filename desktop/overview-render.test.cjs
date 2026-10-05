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

installWindowCleanup(test);

test("R86 DOM teardown disconnects pending observers without disabling live callbacks", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  let callbacks = 0;
  let disposals = 0;
  w.addEventListener("deep-legends:dispose", () => { disposals += 1; });
  const target = w.document.createElement("div");
  new w.MutationObserver(() => { callbacks += 1; }).observe(target, { childList: true });
  target.append(w.document.createElement("span"));
  await Promise.resolve();
  assert.equal(callbacks, 1, "the fixture must not suppress live observer callbacks");
  target.append(w.document.createElement("span"));
  w.close();
  w.close();
  await Promise.resolve();
  assert.equal(callbacks, 1, "queued callbacks cannot outlive their jsdom window");
  assert.equal(disposals, 1, "explicit close and afterEach cleanup must be idempotent");
  assert.deepEqual(errors, []);
});



test("1110 征召默认值、时间输入、模式能力和总开关真实保存链", async () => {
  const { window: w, errors } = bootDemoApp();
  try {
    await settled();
    w.document.querySelector('[data-section="suite"]').click();
    await settled();
    await visitTool(w, "champselect");
    const root = w.document.querySelector("#suite-champselect-root");
    const change = async (selector, value) => {
      const input = root.querySelector(selector);
      assert.ok(input, selector);
      if (input.type === "checkbox") input.checked = value;
      else input.value = value;
      input.dispatchEvent(new w.Event("change", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 60));
    };
    const saved = async () => (await (await w.fetch("/api/watch/rules")).json()).champSelect;
    root.querySelector('[data-cs-group="aram"]').click();
    assert.equal(root.querySelector(".is-ban"), null, "大乱斗不能显示禁用卡");
    assert.equal(root.querySelector("[data-cs-bench-enabled]").checked, true);
    assert.equal(root.querySelector("[data-cs-bench-prefer]").checked, true);
    assert.equal(root.querySelector('[data-cs-time="hold"]').value, "1");
    root.querySelector('[data-cs-hold-delta="500"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    assert.equal((await saved()).groups.aram.bench.holdMs, 1500);
    root.querySelector('[data-cs-hold-delta="-500"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    assert.equal((await saved()).groups.aram.bench.holdMs, 1000);
    await change('[data-cs-time="hold"]', "1.75");
    assert.equal((await saved()).groups.aram.bench.holdMs, 1750, "手输不强制对齐步长");
    assert.equal(root.querySelector('[data-cs-time="hold"]').checkValidity(), true, "手输小数不能被原生 step 判无效");
    await change('[data-cs-time="hold"]', "0.1");
    assert.equal((await saved()).groups.aram.bench.holdMs, 1000);
    assert.equal(root.querySelector('[data-cs-time="lock"]').value, "10");
    await change('[data-cs-time="lock"]', "0.25");
    assert.equal((await saved()).groups.aram.pick.lockDelayMs, 250);
    assert.equal((await saved()).groups.aram.pick.delayMs, 500, "锁定等待不得改写亮出前延时");
    await change('[data-cs-time="lock"]', "");
    assert.equal((await saved()).groups.aram.pick.lockDelayMs, 250, "空值不得改写配置");
    root.querySelector('[data-cs-delay-key="lockDelayMs"][data-cs-delay-delta="500"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    assert.equal((await saved()).groups.aram.pick.lockDelayMs, 750, "步进按钮保存独立锁定等待");
    for (const id of ["ranked", "normal", "practice", "event"]) {
      root.querySelector(`[data-cs-group="${id}"]`).click();
      assert.equal(root.querySelector("[data-cs-bench-enabled]"), null, id);
      assert.equal(root.querySelector("[data-cs-bench-prefer]").matches(":disabled"), false, id);
    }
    await change("[data-cs-bench-prefer]", true);
    assert.equal((await saved()).groups.event.bench.preferFirst, true);
    assert.equal(root.querySelector('[data-cs-time="ban"]'), null, "立即锁定不显示等待框");
    root.querySelector('[data-cs-strategy="ban"][data-cs-strategy-value="show-then-lock"]').click();
    await new Promise((resolve) => setTimeout(resolve, 60));
    await change('.cs-seq-card.is-ban [data-cs-time="lock"]', "1.25");
    assert.equal((await saved()).groups.event.ban.lockDelayMs, 1250);
    await change('.cs-seq-card.is-ban [data-cs-time="lock"]', "99");
    assert.equal((await saved()).groups.event.ban.lockDelayMs, 10000);
    root.querySelector('[data-cs-group="arena"]').click();
    assert.equal(root.querySelector(".cs-bench-card"), null, "无交换能力不显示空卡");
    root.querySelector('[data-cs-group="aram"]').click();
    const expectedGroups = JSON.parse(JSON.stringify((await saved()).groups));
    for (const config of Object.values(expectedGroups)) {
      config.ban.enabled = false;
      config.pick.enabled = false;
    }
    await change("[data-cs-master]", false);
    assert.ok(root.classList.contains("cs-master-off"));
    assert.ok(root.querySelector(".cs-config-fields").disabled);
    for (const element of root.querySelectorAll(".cs-config-fields input, .cs-config-fields button")) {
      assert.ok(element.matches(":disabled"), element.outerHTML);
    }
    assert.equal(root.querySelector("[data-cs-master]").disabled, false);
    assert.equal(JSON.stringify((await saved()).groups), JSON.stringify(expectedGroups), "关闭总开关同步关闭所有禁用/选用，保留英雄序列和其它配置");
    assert.equal(root.querySelector('[data-cs-side-enabled="pick"]').checked, false);
    await change("[data-cs-master]", true);
    const definitions = await (await w.fetch("/api/champselect/groups")).json();
    for (const definition of definitions) {
      expectedGroups[definition.groupId].ban.enabled = definition.hasBan;
      expectedGroups[definition.groupId].pick.enabled = true;
    }
    assert.equal(JSON.stringify((await saved()).groups), JSON.stringify(expectedGroups), "开启总开关同步开启所有可用禁用/选用，大乱斗保持无禁用");
    assert.equal(root.querySelector('[data-cs-side-enabled="pick"]').checked, true);
    assert.equal(root.querySelector("[data-cs-bench-enabled]").matches(":disabled"), false);
    assert.equal(root.querySelector("[data-cs-bench-enabled]").checked, true);
    root.querySelector('[data-cs-group="normal"]').click();
    assert.equal(root.querySelector('[data-cs-side-enabled="ban"]').checked, true);
    assert.equal(root.querySelector('[data-cs-side-enabled="pick"]').checked, true);
    await change('[data-cs-side-enabled="pick"]', false);
    assert.equal((await saved()).groups.normal.pick.enabled, false, "总开关开启后仍可分别调整子开关");
    assert.equal((await saved()).enabled, true);
    assert.deepEqual(errors, []);
  } finally { w.close(); }
});


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
    "matchTierScrollRoot", "matchTierNodeIsVisible", "hydrateMatchTiers", "shouldHydrateMatchTiers", "scheduleMatchTierRetry",
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
    MATCH_TIER_KR_COALESCE_MS: 0,
    MATCH_TIERS_PARALLEL_BATCHES: 2,
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

test("match-tier failures back off instead of retrying on the next render", async () => {
  const dom = new JSDOM('<main id="app-scroll"><div id="matches"><article class="match-entry"><span data-match-tier-pending data-game-id="1"></span></article></div></main>', { url: "http://localhost/", pretendToBeVisual: true });
  const w = dom.window;
  const container = w.document.getElementById("matches");
  const node = container.querySelector("[data-match-tier-pending]");
  node.closest(".match-entry").getBoundingClientRect = () => ({ top: 0, left: 0, right: 10, bottom: 10, width: 10, height: 10 });
  w.document.getElementById("app-scroll").getBoundingClientRect = () => ({ top: 0, left: 0, right: 100, bottom: 100, width: 100, height: 100 });
  const state = { activeTab: "current", matchTiers: new Map(), matchTierFlights: new Set(), matchTierFailures: new Map() };
  let requests = 0;
  const functions = compileFunctions(gameplaySource, ["matchTierScrollRoot", "matchTierNodeIsVisible", "noteMatchTierFailure", "hydrateMatchTiers", "shouldHydrateMatchTiers"], {
    window: w, document: w.document, state,
    riotTab: () => false, connected: () => true,
    matchTierCacheKey: () => "scope:1", applyMatchTierValue: () => {}, tabServerID: () => "HN1",
    matchTierFromScores: () => null, MATCH_TIERS_MAX_REFS: 24, MATCH_TIER_KR_COALESCE_MS: 0, MATCH_TIERS_PARALLEL_BATCHES: 2,
    MATCH_TIER_RETRY_BASE_MS: 1000, MATCH_TIER_MAX_BACKOFF_MS: 60000,
    api: async () => { requests += 1; throw new Error("timeout"); },
    // 一批失败后现在会安排重试（R127 复审第 5 条），harness 里不需要真的排程。
    scheduleMatchTierRetry: () => {},
    // 整批诊断现在失败也会上报（reason=failed），harness 里不需要真的发请求。
    recordMatchTierOverviewBatch: () => {},
  });
  const tab = { key: "current", data: { player: { playerRef: "player" }, matches: [{ gameId: 1, participants: [{ playerRef: "ref" }] }] } };
  await functions.hydrateMatchTiers(container, tab, "scope", [node]);
  await functions.hydrateMatchTiers(container, tab, "scope", [node]);
  assert.equal(requests, 1);
  assert.equal(state.matchTierFailures.get("scope:1")?.count, 1);
  w.close();
});

test("match-tier overview diagnostics contain aggregate counts only", async () => {
  const requests = [];
  const { recordMatchTierOverviewBatch } = compileFunctions(gameplaySource, ["recordMatchTierOverviewBatch"], {
    fetch: async (requestPath, options) => { requests.push([requestPath, JSON.parse(options.body)]); return { ok: true }; },
  });
  recordMatchTierOverviewBatch(36, 24, 19, 2431);
  await new Promise((resolve) => setImmediate(resolve));
  assert.deepEqual(requests, [["/api/diagnostics/client", {
    event: "match_tiers_overview_batch", reason: "complete", totalRefs: 36, uniqueRefs: 24, cacheHits: 19,
    // R127 P0-3：整批耗时必须上报，否则日志里看不出平均段位到底花了多久。
    durationMs: 2431,
  }]]);
  assert.doesNotMatch(JSON.stringify(requests), /playerRef|puuid|summoner/i);
});

test("non-match overview rerenders preserve the match-list node", () => {
  const dom = new JSDOM('<div id="overview"></div>', { url: "http://localhost/", pretendToBeVisual: true });
  const container = dom.window.document.getElementById("overview");
  const state = { tabs: [{ key: "current" }], settings: { maskNames: false } };
  let preparedStrip;
  const dependencies = {
    state,
    matchTierScope: () => "scope", filteredMatches: (matches) => matches,
    maskedProfileIcon: () => "", iconFigure: () => "", playerLabel: () => "Player", riotTab: () => false,
    emptyState: () => "", opggSummonerURL: () => "", loadOverview: () => {},
    matchListEmptyContent: () => "empty", matchSentinelShouldHide: () => true,
    summonerContextChip: () => "", summonerProChip: () => "", summonerRegionChip: () => "", renderSummonerHighlights: () => "", renderOverviewStreak: () => "",
    scheduleOverviewCurrentGame: () => {}, updateFriendPresenceChips: () => {},
    paginationCopyFor: () => "", renderCareerSections: () => "", renderMatchFilters: () => "",
    renderMatch: (match) => `<article data-match-id="${match.gameId}" class="match-entry">${match.gameId}</article>`, number: (value) => String(value),
    escapeHTML: (value) => String(value ?? ""), bindOverviewContent: () => {}, applyRenderedMetricStyles: () => {},
    prepareImages: root => { preparedStrip = root.querySelector(".summoner-strip"); }, ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, observeMatchTierVisibility: () => {},
    window: dom.window, document:dom.window.document,
  };
  const { renderOverviewBodyContent } = compileFunctions(gameplaySource, ["renderOverviewBodyContent", "reconcileFilteredMatchList"], dependencies);
  const matches = [{ gameId: 1 }];
  const tab = { key: "current", matchFilter: "all", matchViewRevision: 0, openMatches: new Set(), data: { player: { playerRef: "ref", backgroundSource: "gtimg", backgroundPath: "/skin.jpg" }, matches, pagination: {} } };
  renderOverviewBodyContent(container, tab);
  const firstList = container.querySelector(".match-list");
  renderOverviewBodyContent(container, tab);
  assert.equal(container.querySelector(".match-list"), firstList);

  const firstEntry = firstList.querySelector(".match-entry");
  const firstStrip = container.querySelector(".summoner-strip");
  tab.data.matches = [{ gameId: 1 }, { gameId: 2 }];
  tab.data.historicalRanks = [{ season: "S2025", tier: "MASTER" }];
  renderOverviewBodyContent(container, tab);
  assert.equal(container.querySelector(".match-list"), firstList);
  assert.equal(container.querySelector(".match-entry"), firstEntry, "streamed arrays must not recreate already loaded cards");
  assert.equal(container.querySelector(".summoner-strip"), firstStrip, "unchanged profile must retain image nodes");
  assert.equal(preparedStrip, firstStrip, "image initialization must see the final retained strip");
  assert.equal(firstList.querySelectorAll(".match-entry").length, 2);

  const artwork = firstStrip.querySelector(".summoner-strip-art");
  tab.data.player.summonerLevel = 300;
  renderOverviewBodyContent(container, tab);
  assert.notEqual(container.querySelector(".summoner-strip"), firstStrip);
  assert.equal(container.querySelector(".summoner-strip-art"), artwork, "profile metadata must not restart the same artwork");

  tab.openMatches.add("1");
  tab.matchViewRevision += 1;
  renderOverviewBodyContent(container, tab);
  assert.notEqual(container.querySelector(".match-list"), firstList, "match UI state changes must rebuild the list");

  const mutated = gameplaySource.replace("container._matchListViewRevision === Number(tab.matchViewRevision || 0)", "true");
  assert.notEqual(mutated, gameplaySource);
  const mutatedRender = compileFunctions(mutated, ["renderOverviewBodyContent", "reconcileFilteredMatchList"], dependencies).renderOverviewBodyContent;
  const mutatedContainer = dom.window.document.createElement("div");
  tab.openMatches.clear();
  tab.matchViewRevision = 0;
  mutatedRender(mutatedContainer, tab);
  const mutatedFirst = mutatedContainer.querySelector(".match-list");
  mutatedRender(mutatedContainer, tab);
  assert.equal(mutatedContainer.querySelector(".match-list"), mutatedFirst);
  tab.openMatches.add("1");
  tab.matchViewRevision += 1;
  mutatedRender(mutatedContainer, tab);
  assert.equal(mutatedContainer.querySelector(".match-list"), mutatedFirst, "the mutation must reproduce the stale-list bug");
  dom.window.close();
});


test("玩家覆盖层里的战绩详情也能展开", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  w.dispatchEvent(new w.CustomEvent("deep-legends:open-player", {
    detail: { source: "champions", playerRef: "player_00000000000000000000000000000001", gameName: "覆盖层玩家" },
  }));
  await settled();
  const overlay = w.document.getElementById("player-overlay");
  assert.equal(overlay.hidden, false, "玩家覆盖层没有打开");
  const button = overlay.querySelector(".match-list [data-toggle-match]:not([disabled])");
  assert.ok(button, "覆盖层里找不到可展开的战绩");
  button.click();
  await settled();
  assert.equal(overlay.querySelectorAll(".match-detail").length, 1, "覆盖层战绩详情没有展开");
  assert.deepEqual(errors, []);
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

test("好友真实数据恢复横条模式与计时，SSE 更新不重建战绩或串到其他玩家", async () => {
  const now = Date.now(), requests = [];
  let friend = {playerRef:"player_friend_123456789",gameName:"好友",tagLine:"1234",icon:1,groupId:1,
    availability:"dnd",product:"league_of_legends",gameStatus:"inGame",queueLabel:"排位赛 灵活排位",championName:"虚空掠夺者",gameStartedAt:now-852000};
  const { window: w, errors } = bootDemoApp({friendsPayload:()=>({groups:[{id:1,name:"默认分组"}],friends:[friend]}),onRequest:url=>requests.push(url)});
  w.Date.now = () => now;
  const waitFor = async predicate => {
    const deadline=Date.now()+10000;
    while(!predicate() && Date.now()<deadline) await new Promise(resolve=>setTimeout(resolve,20));
    assert.ok(predicate(),"expected friend UI state did not arrive");
  };
  try {
    await settled();
    w.document.getElementById("friends-toggle").click();
    await waitFor(()=>w.document.querySelector('[data-player-ref="player_friend_123456789"]'));
    w.document.querySelector('[data-player-ref="player_friend_123456789"]').click();
    const overview=w.document.getElementById("overview-content");
    await waitFor(()=>overview.querySelector('[data-friend-presence]:not([hidden])'));
    const strip=overview.querySelector(".summoner-strip"), list=overview.querySelector(".match-list"), chip=strip.querySelector("[data-friend-presence]");
    assert.equal(chip.textContent,"排位赛 灵活排位 · 虚空掠夺者 · 已进行 14:12");
    assert.equal(w.document.getElementById("friends-dock").classList.contains("is-open"),false);
    const historyRequests=()=>requests.filter(url=>url.startsWith('/api/gameplay/overview')).length;
    const reads=historyRequests();
    w.document.getElementById("app-scroll").scrollTop=330;
    w.Date.now=()=>now+5000;
    await waitFor(()=>chip.textContent.endsWith("14:17"));
    const refresh=async changes=>{
      friend={...friend,...changes};
      const before=requests.filter(url=>url.startsWith('/api/social/friends')).length;
      w.dispatchEvent(new w.CustomEvent("deep-legends:friends-updated"));
      await waitFor(()=>requests.filter(url=>url.startsWith('/api/social/friends')).length>before);
      await new Promise(resolve=>setTimeout(resolve,20));
    };
    await refresh({queueLabel:"海克斯大乱斗"});
    assert.match(chip.textContent,/海克斯大乱斗.*14:17/);
    assert.equal(overview.querySelector(".summoner-strip"),strip);
    assert.equal(overview.querySelector(".match-list"),list);
    assert.equal(w.document.getElementById("app-scroll").scrollTop,330);
    assert.equal(historyRequests(),reads,"presence must not reload history");
    assert.equal(requests.some(url=>url.startsWith('/api/gameplay/current-game')),false,"CN friend status needs no spectator probe");
    await refresh({gameStatus:"outOfGame"});assert.equal(chip.hidden,true);assert.equal(chip.querySelector("time"),null);
    await refresh({gameStatus:"inGame",availability:"offline"});assert.equal(chip.hidden,true);
    await refresh({availability:"dnd",playerRef:"player_different_server"});assert.equal(chip.hidden,true,"same display name with another reference must not match");
    await refresh({playerRef:"player_friend_123456789"});assert.equal(chip.hidden,false);
    w.dispatchEvent(new w.CustomEvent("deep-legends:friends-presence",{detail:{friends:[]}}));assert.equal(chip.hidden,true,"empty/disconnected snapshot clears old game");
    assert.deepEqual(errors,[]);
  } finally {w.close();}
});
