// R129：对局页海斗推荐每隔几秒整块重绘（图标闪成文字、页面跳动）的回归验证。
//
// 根因（工单 §1.3）：renderLive 的「markup 没变就不重绘」保护里，参与比较的 HTML
// 含隐藏的「详情」tab，而详情 tab 的战绩区按 state.liveLoading 输出骨架屏；判据词表
// 又写成 ready/empty/unavailable/failed，而后端只下发 ok/empty/failed/unavailable，
// 于是 historyState="ok" 且本模式无战绩时，每 3 秒刷新会整块重建两次。
//
// 这里的钉子：
//  P1 —— liveLoading 翻转不得改变 markup（真跑 renderLivePlayer / renderInsightMatches
//        的真实实现），词表与后端一致；并带一条对抗变异：把 P1 改回去，节点身份断言
//        必须失败（变异体确实触发面板重建）。
//  P2 —— 只有某个面板的数据变化时，只替换那个面板：build 面板的 <img> 节点对象不变，
//        滚动位置恢复，被替换的面板重新绑定与重新排队懒加载；外壳变化才整块重建。
//  P3 —— live_render_rebuild 按 60 秒聚合，记录各面板替换次数与触发来源。
//
// 说明：renderLive 的外围依赖（会话摘要、空态、目录渲染等）用桩，被测的
// renderLive / updateLivePanels / recordLiveRenderRebuild 与战绩渲染全部是真实实现。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require(path.join(__dirname, "..", "..", "desktop", "node_modules", "jsdom"));

const gameplayScript = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");

const FUNCTION_NAMES = [
  "liveHistoryStateOf", "liveHistorySettled", "insightScore", "renderInsightMatches",
  "renderLivePlayer", "proBadgeAttributes", "renderProIdentityBadge", "normalizeLiveGameId",
  "liveRenderTriggerLabel", "recordLiveRenderRebuild", "flushLiveRenderRebuild",
  "liveBodyChrome", "captureLiveScroll", "restoreLiveScroll", "updateLivePanels", "renderLive",
];

function functionSource(source, name) {
  const start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} 不在 gameplay.js 里`);
  const bodyStart = source.indexOf("{", source.indexOf(")", start));
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
    if (character === '"' || character === "'" || character === "`") { quote = character; continue; }
    if (character === "{") depth += 1;
    if (character === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} 的函数体括号不配对`);
}

function compile(names, dependencies, source = gameplayScript) {
  const keys = Object.keys(dependencies);
  const body = names.map((name) => functionSource(source, name)).join("\n");
  return Function(...keys, `"use strict";\n${body}\nreturn {${names.join(",")}};`)(...keys.map((key) => dependencies[key]));
}

// 海斗英雄选择快照：1 名本人玩家、historyState="ok"、当前模式没有近期对局与模式
// 战绩（工单 §1.2 的 with_stats: 0），推荐数据含 9 条海克斯。
function liveFixture(playerOverrides = {}, dataOverrides = {}) {
  const player = {
    id: "self", playerRef: "CN:1", isCurrent: true, teamId: 100,
    championId: 223, championName: "河流之王", position: "",
    historyState: "ok", recentGames: [], modeStats: {},
    ...playerOverrides,
  };
  return {
    available: true, phase: "ChampSelect", gameId: 9001, queueId: 3270,
    gameMode: "KIWI", mapId: 12, currentChampionId: 223, players: [player],
    recommendations: {
      augments: Array.from({ length: 9 }, (_, index) => ({
        id: 2000 + index, name: `海克斯 ${index + 1}`, path: `/assets/augments/${2000 + index}.png`,
      })),
    },
    ...dataOverrides,
  };
}

// markup 只由快照数据决定：详情面板走真实的 renderLivePlayer / renderInsightMatches，
// 出装面板是 9 个懒加载图标（data-queued-src，正是录像里闪成名称首字的那批节点）。
function buildMarkup(data, fns) {
  const players = data.players || [];
  const player = players[0] || {};
  const insight = `${fns.renderLivePlayer(player, 0, false, data.currentChampionId, players, "", false, true)}${fns.renderInsightMatches(player)}`;
  const augments = (data.recommendations?.augments || [])
    .map((row) => `<article class="live-augment-option is-gold"><img data-queued-src="${row.path}" alt="${row.name}"><span>${row.name}</span></article>`)
    .join("");
  const build = `<section class="live-augment-recommendations is-hextech"><header><h3>海克斯推荐</h3></header><div class="live-augment-columns">${augments}</div></section>`;
  const tab = (key, label, active) => `<button type="button" role="tab" id="recommendation-tab-${key}" class="${active ? "is-active" : ""}" aria-selected="${active}" tabindex="${active ? 0 : -1}" data-recommendation-tab="${key}">${label}</button>`;
  const panel = (key, content, active) => `<div id="recommendation-panel-${key}" class="recommendation-panel" role="tabpanel" aria-labelledby="recommendation-tab-${key}"${active ? "" : " hidden"}>${content}</div>`;
  const rosterNotice = data.champSelectNotice ? `<p class="live-roster-notice" role="note"><span>${data.champSelectNotice}</span></p>` : "";
  return `<section class="recommendation-area"><div class="recommendation-tab-row"><div class="recommendation-tabs" role="tablist" aria-label="推荐类型">${tab("build", "海克斯与出装", true)}${tab("insight", "详情", false)}</div>${rosterNotice}</div>${panel("build", build, true)}${panel("insight", insight, false)}</section>`;
}

function mountFixture(source = gameplayScript) {
  const dom = new JSDOM(
    '<!doctype html><html><body><div class="app-main"><div id="app-scroll"><div id="live-content"></div></div></div></body></html>',
    { url: "http://localhost/", pretendToBeVisual: true },
  );
  const { window } = dom;
  const document = window.document;
  const diagnostics = [];
  window.reportFlowDiagnostic = (event, reason, context) => diagnostics.push({ event, reason, context });

  const content = document.getElementById("live-content");
  const appScroll = document.getElementById("app-scroll");
  const appMain = document.querySelector(".app-main");
  const writes = { innerHTML: 0, scroll: [] };
  // 整块重建的唯一入口是 nodes.liveContent.innerHTML = …，给它装一个计数器。
  const innerHTMLDescriptor = Object.getOwnPropertyDescriptor(window.Element.prototype, "innerHTML");
  Object.defineProperty(content, "innerHTML", {
    configurable: true,
    get() { return innerHTMLDescriptor.get.call(this); },
    set(value) { writes.innerHTML += 1; innerHTMLDescriptor.set.call(this, value); },
  });
  // jsdom 不做布局，scrollTop 自己装读写探针，才能验证「替换后恢复滚动位置」。
  for (const [node, key] of [[appScroll, "app-scroll"], [appMain, "app-main"]]) {
    let top = 0;
    Object.defineProperty(node, "scrollTop", {
      configurable: true,
      get() { return top; },
      set(value) { top = Number(value) || 0; writes.scroll.push([key, top]); },
    });
  }

  const toolbar = { hidden: true };
  const state = {
    destroyed: false, section: "live", live: null, liveLoading: false, liveError: "",
    liveAwaitingGame: false, liveLoadSource: "interval", liveRenderTrigger: "",
    liveRenderSource: "direct", liveRenderRebuild: null, beacon: { phase: "ChampSelect" },
    settings: { maskNames: false, liveOrder: "team", detailPositionAlign: false },
  };
  const binds = [];
  const prepared = [];
  let markupImpl = () => "";
  const fns = compile(FUNCTION_NAMES, {
    state,
    nodes: { liveContent: content, liveRefresh: { closest: () => toolbar, textContent: "", setAttribute() {} } },
    document,
    window,
    connected: () => true,
    renderSessionSummary: () => {},
    emptyState: (title, detail) => `<empty><strong>${title}</strong><p>${detail}</p></empty>`,
    liveRecommendationMarkup: (data) => markupImpl(data),
    renderLiveRefreshStatus: () => "",
    renderRecommendationArea: () => "",
    bindLiveContent: () => binds.push("live-content"),
    bindLivePanelScope: (scope) => binds.push(scope?.id || "scope"),
    applyRenderedMetricStyles: () => {},
    prepareImages: (scope) => prepared.push(scope?.id || "scope"),
    loadLive: () => {},
    maskedPlayerName: (player) => player?.id || "玩家",
    iconFigure: (_kind, id) => `<icon data-id="${Number(id) || 0}"></icon>`,
    escapeHTML: (value) => String(value ?? ""),
    positionLabel: (value) => String(value || "位置未知"),
    rankTitle: () => "未定级",
    number: (value) => String(value ?? 0),
    percent: (value) => `${Math.round(Number(value) || 0)}%`,
    kda: (value) => `${Number(value) || 0}.00`,
    liveDisplayedChampionId: (player, currentChampionId = 0) => Number(player?.championId) || Number(currentChampionId) || 0,
    renderLivePremadeTag: () => "",
  }, source);
  markupImpl = (data) => buildMarkup(data, fns);

  const buildImages = () => [...content.querySelectorAll("#recommendation-panel-build img")];
  const insightChildren = () => [...content.querySelectorAll("#recommendation-panel-insight > *")];
  // 60 秒聚合定时器不能留着，否则 node --test 要等它才退出。
  const cleanup = () => {
    fns.flushLiveRenderRebuild();
    state.liveRenderRebuild = null;
    window.close();
  };
  return { dom, window, document, state, fns, content, appScroll, appMain, writes, binds, prepared, diagnostics, toolbar, buildImages, insightChildren, cleanup };
}

// 首帧：整块渲染一次，返回可比较的节点快照。
function firstRender(fx, data) {
  fx.state.live = data;
  fx.fns.renderLive();
  assert.equal(fx.writes.innerHTML, 1, "首帧应整块渲染一次");
  assert.deepEqual(fx.state.liveRenderRebuild?.counts || {}, { full: 1 });
  const build = fx.buildImages();
  assert.equal(build.length, 9, "九张海克斯图标都该在");
  return { build, insight: fx.insightChildren(), panel: fx.content.querySelector("#recommendation-panel-insight") };
}

function assertSameNodes(before, after, label) {
  assert.equal(after.length, before.length, `${label} 节点数变了`);
  after.forEach((node, index) => assert.equal(node, before[index], `${label} 第 ${index + 1} 个节点被重建了`));
}

// ---------------------------------------------------------------------------
// P1：markup 不得依赖 liveLoading，词表与后端一致
// ---------------------------------------------------------------------------

test("R129 P1 海斗英雄选择：liveLoading 翻转不重绘，图标与详情节点保持同一对象", (t) => {
  const fx = mountFixture();
  t.after(() => fx.cleanup());
  const data = liveFixture();
  fx.state.liveLoading = true;
  const before = firstRender(fx, data);

  // 3 秒刷新的收尾：liveLoading 落回 false 后再渲染一次。
  fx.state.liveLoading = false;
  fx.fns.renderLive();

  assert.equal(fx.writes.innerHTML, 1, "innerHTML 的 setter 只应在第一次被调用");
  assertSameNodes(before.build, fx.buildImages(), "海克斯图标");
  assert.equal(fx.content.querySelector("#recommendation-panel-insight"), before.panel, "详情面板外壳被换掉了");
  assertSameNodes(before.insight, fx.insightChildren(), "详情面板内容");
  assert.deepEqual(fx.state.liveRenderRebuild.counts, { full: 1 }, "数据没变时不得记录任何重建");
  assert.deepEqual(fx.diagnostics, [], "窗口内不该发诊断");
});

test("R129 P1 historyState 为 empty / failed / unavailable 时同样不重建", (t) => {
  for (const historyState of ["empty", "failed", "unavailable"]) {
    const fx = mountFixture();
    t.after(() => fx.cleanup());
    const data = liveFixture({ historyState });
    fx.state.liveLoading = true;
    const before = firstRender(fx, data);
    // 已就绪的状态不许再出骨架屏（那正是整块重绘的来源）。
    assert.doesNotMatch(before.panel.innerHTML, /is-history-pending|live-history-skeleton/, `${historyState} 不该渲染骨架`);

    fx.state.liveLoading = false;
    fx.fns.renderLive();
    assert.equal(fx.writes.innerHTML, 1, `${historyState}：不得整块重建`);
    assertSameNodes(before.build, fx.buildImages(), `${historyState}：海克斯图标`);
    assertSameNodes(before.insight, fx.insightChildren(), `${historyState}：详情内容`);
    assert.deepEqual(fx.state.liveRenderRebuild.counts, { full: 1 });
  }
});

test("R129 P1 词表与后端一致，源码里不再残留 ready/liveLoading 判据", () => {
  const fx = mountFixture();
  try {
    for (const historyState of ["ok", "empty", "failed", "unavailable", "OK", " Empty ", undefined, ""]) {
      assert.equal(fx.fns.liveHistorySettled({ historyState }), true, `${JSON.stringify(historyState)} 应视为已就绪`);
      assert.equal(fx.fns.liveHistorySettled({}), true, "字段缺失按 empty 处理");
    }
    for (const historyState of ["pending", "loading", "ready", "unknown"]) {
      assert.equal(fx.fns.liveHistorySettled({ historyState }), false, `${historyState} 应视为还在读取`);
    }
  } finally { fx.cleanup(); }
  // 后端从来不下发 "ready"；两处骨架判据也不再读 liveLoading。
  assert.doesNotMatch(gameplayScript, /\["ready", "empty", "unavailable", "failed"\]/);
  assert.doesNotMatch(gameplayScript, /state\.liveLoading === true && !player\.recentGames/);
  for (const name of ["renderLivePlayer", "renderInsightMatches", "renderLiveInsights", "renderRecommendationArea", "renderLiveAugmentRecommendations", "renderBuildRecommendation"]) {
    assert.doesNotMatch(functionSource(gameplayScript, name), /state\.liveLoading/, `${name} 不得把瞬时加载状态写进 markup`);
  }
  // 两处判据共用同一个 helper，不再各写一份词表。
  assert.equal((gameplayScript.match(/liveHistorySettled\(player\)/g) || []).length, 3, "helper 定义 + 两处调用");
});

test("R129 P1 renderLive 上方写明 markup 的取值纪律", () => {
  const index = gameplayScript.indexOf("function renderLive() {");
  assert.ok(index > 0);
  const above = gameplayScript.slice(Math.max(0, index - 700), index);
  assert.match(above, /只能由快照数据和用户选择决定/);
  assert.match(above, /不能包含 state\.liveLoading/);
  assert.match(above, /data-live-status/);
});

test("R129 P1 对抗变异：把骨架判据改回依赖 liveLoading，节点身份钉子必须失败", (t) => {
  const fixedInsight = `if (!liveHistorySettled(player)) return '<div class="insight-match-row is-history-pending"`;
  const fixedPlayer = "const historyPending = !liveHistorySettled(player);";
  // 变异体＝R129 修复前的原始判据：词表里是 "ready"（后端从来不下发），于是
  // historyState="ok" 被当成「还在读取」，加载期就出骨架屏。
  const mutatedInsight = `if (state.liveLoading === true && !player.recentGames?.length && !["ready", "empty", "unavailable", "failed"].includes(player.historyState)) return '<div class="insight-match-row is-history-pending"`;
  const mutatedPlayer = `const historyPending = state.liveLoading === true && !player.recentGames?.length && !Number(stats.games) && !["ready", "empty", "unavailable", "failed"].includes(player.historyState);`;
  assert.ok(gameplayScript.includes(fixedInsight) && gameplayScript.includes(fixedPlayer), "变异锚点没匹配上：改了生产代码要同步这里");
  const mutated = gameplayScript.replace(fixedInsight, mutatedInsight).replace(fixedPlayer, mutatedPlayer);
  assert.notEqual(mutated, gameplayScript);

  const fx = mountFixture(mutated);
  t.after(() => fx.cleanup());
  const data = liveFixture();
  fx.state.liveLoading = true;
  const before = firstRender(fx, data);
  assert.match(before.panel.innerHTML, /is-history-pending/, "变异体在加载期就该出骨架屏");

  fx.state.liveLoading = false;
  fx.fns.renderLive();

  // 这两条正是上面那条测试的断言：变异体下它们必须不成立。
  assert.equal(fx.state.liveRenderRebuild.counts.insight, 1, "变异体必然重绘详情面板");
  assert.notEqual(fx.insightChildren()[0], before.insight[0], "变异体下详情节点被重建");
  assert.notDeepEqual(fx.insightChildren().map((node) => node.outerHTML), before.insight.map((node) => node.outerHTML));
});

// ---------------------------------------------------------------------------
// P2：真实变化时也只替换变了的那个面板
// ---------------------------------------------------------------------------

test("R129 P2 只有详情数据变化时，build 面板图标不动、滚动位置恢复、面板重新绑定", (t) => {
  const fx = mountFixture();
  t.after(() => fx.cleanup());
  const data = liveFixture();
  const before = firstRender(fx, data);
  const insightPanel = before.panel;
  fx.appScroll.scrollTop = 120;
  fx.appMain.scrollTop = 30;
  fx.writes.scroll.length = 0;
  fx.binds.length = 0;
  fx.prepared.length = 0;

  // 队友战绩读完：详情面板内容变化，外壳（页签行、面板属性与顺序）不变。
  data.players[0].recentGames = [{ championId: 223, championName: "河流之王", kills: 3, deaths: 1, assists: 9, win: true, cs: 42 }];
  fx.fns.renderLive();

  assert.equal(fx.writes.innerHTML, 1, "不得整块重建");
  assert.equal(fx.content.querySelector("#recommendation-panel-insight"), insightPanel, "面板外壳应原地更新");
  assert.notEqual(fx.insightChildren()[0], before.insight[0], "详情内容应被替换");
  assert.match(insightPanel.innerHTML, /insight-match/, "新的战绩行要渲染出来");
  assertSameNodes(before.build, fx.buildImages(), "build 面板图标");
  assert.deepEqual(fx.state.liveRenderRebuild.counts, { full: 1, insight: 1 });
  assert.deepEqual(fx.writes.scroll, [["app-scroll", 120], ["app-main", 30]], "替换后要恢复滚动位置");
  assert.deepEqual(fx.binds, ["recommendation-panel-insight"], "只重绑被替换的那个面板");
  assert.deepEqual(fx.prepared, ["recommendation-panel-insight"], "只对被替换的面板重新排队懒加载");
});

test("R129 P2 外壳变化（提示条/页签行）时回退整块重建", (t) => {
  const fx = mountFixture();
  t.after(() => fx.cleanup());
  const data = liveFixture();
  firstRender(fx, data);
  const before = fx.buildImages();

  data.champSelectNotice = "海克斯大乱斗英雄选择阶段只展示本人信息";
  fx.fns.renderLive();

  assert.equal(fx.writes.innerHTML, 2, "外壳变了必须整块重建");
  assert.deepEqual(fx.state.liveRenderRebuild.counts, { full: 2 });
  assert.equal(fx.buildImages().length, 9);
  assert.notEqual(fx.buildImages()[0], before[0], "整块重建后节点当然是新的");
  assert.match(fx.content.innerHTML, /live-roster-notice/);
});

test("R129 P2 markup 完全没变时连面板都不碰", (t) => {
  const fx = mountFixture();
  t.after(() => fx.cleanup());
  const data = liveFixture();
  const before = firstRender(fx, data);
  fx.binds.length = 0;
  fx.prepared.length = 0;

  fx.fns.renderLive();
  fx.fns.renderLive();

  assert.equal(fx.writes.innerHTML, 1);
  assert.deepEqual(fx.state.liveRenderRebuild.counts, { full: 1 }, "重复渲染不得记录任何替换");
  assert.deepEqual(fx.binds, []);
  assert.deepEqual(fx.prepared, []);
  assertSameNodes(before.build, fx.buildImages(), "海克斯图标");
  assertSameNodes(before.insight, fx.insightChildren(), "详情内容");
});

// ---------------------------------------------------------------------------
// P3：live_render_rebuild 诊断
// ---------------------------------------------------------------------------

test("R129 P3 触发来源词表：interval / sse / manual / catalog，其余归为 direct", (t) => {
  const fx = mountFixture();
  t.after(() => fx.cleanup());
  const cases = [["interval", "interval"], ["manual", "manual"], ["sse", "sse"], ["event", "sse"], ["catalog", "catalog"], ["resync", "direct"], ["direct", "direct"], ["", "direct"], [undefined, "direct"]];
  for (const [input, expected] of cases) assert.equal(fx.fns.liveRenderTriggerLabel(input), expected, `${String(input)} 应归为 ${expected}`);
});

test("R129 P3 live_render_rebuild 每 60 秒聚合一次，记录面板与来源", (t) => {
  const fx = mountFixture();
  t.after(() => fx.cleanup());
  const data = liveFixture();
  fx.state.liveLoadSource = "interval";
  firstRender(fx, data);
  assert.equal(fx.state.liveRenderRebuild.counts.full, 1);
  assert.equal(fx.state.liveRenderRebuild.sources.interval, 1);
  assert.ok(fx.state.liveRenderRebuild.timer, "应挂上 60 秒聚合定时器");
  assert.deepEqual(fx.diagnostics, [], "窗口内不发诊断");

  // catalog（图标目录到达）的一次性覆盖优先于本轮加载来源。
  fx.state.liveRenderTrigger = "catalog";
  data.champSelectNotice = "提示";
  fx.fns.renderLive();
  assert.equal(fx.state.liveRenderSource, "catalog");
  assert.equal(fx.state.liveRenderTrigger, "", "一次性触发用完即清");
  assert.deepEqual(fx.state.liveRenderRebuild.counts, { full: 2 });
  assert.deepEqual(fx.state.liveRenderRebuild.sources, { interval: 1, catalog: 1 });
  assert.deepEqual(fx.diagnostics, [], "60 秒窗口内仍不发诊断");

  fx.fns.flushLiveRenderRebuild();
  assert.equal(fx.diagnostics.length, 1, "到点聚合出一条");
  const [entry] = fx.diagnostics;
  assert.equal(entry.event, "live_render_rebuild");
  assert.equal(entry.reason, "aggregated");
  assert.deepEqual(entry.context.counts, { full: 2 });
  assert.deepEqual(entry.context.sources, { interval: 1, catalog: 1 });
  assert.equal(entry.context.total, 2);
  assert.equal(entry.context.phase, "ChampSelect");
  assert.equal(entry.context.gameId, 9001);
  assert.ok(entry.context.windowMs >= 0);
  assert.equal(fx.state.liveRenderRebuild.timer, 0, "flush 后定时器要清掉");

  // 空窗口不产生诊断（数据没变期间计数应为 0，日志里也就没有这条）。
  fx.diagnostics.length = 0;
  fx.fns.flushLiveRenderRebuild();
  assert.deepEqual(fx.diagnostics, []);
});

test("R129 P3 sse 与 manual 触发按来源归类，dispose 前会 flush", () => {
  // handleGameplayPhase 与 rerenderCatalogViews 分别在渲染前打上一次性触发标记。
  assert.match(gameplayScript, /if \(phaseChanged \|\| gameChanged\) \{ state\.liveRenderTrigger = "sse"; renderLive\(\); \}/);
  assert.match(gameplayScript, /if \(state\.section === "live"\) \{ state\.liveRenderTrigger = "catalog"; renderLive\(\); \}/);
  // loadLive 记下本轮来源，dispose 时把未聚合的计数 flush 出去。
  assert.match(gameplayScript, /state\.liveLoadSource = liveRenderTriggerLabel\(source\);/);
  const dispose = functionSource(gameplayScript, "disposeGameplay");
  assert.match(dispose, /flushLiveRenderRebuild\(\);/);
});
