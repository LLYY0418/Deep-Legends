// R116-B（第二轮重做）：P0-1~P0-6 + P1-6 + 英雄详情页三-tab 重整的验证。
//
// 两类测试：
//  1. 全应用 jsdom 冒烟——按 index.html 的真实加载顺序注入 runtime.js →
//     champions.js，用真实夹具（backend/testdata/r116/hexdata-hero-157.json，按后端
//     的单位换算逐字段搬成响应形状）驱动整条渲染与点击链路。tab 切换、阶段 chips、
//     请求预算都在这里断言。（R128 §2.3：口径页脚已删除，shared.js 也随之移除。）
//  2. 聚焦渲染测试——沿用仓库既有的 compileFunctions 方法论，单独钉住降级红线
//     （字段缺失就整块隐藏，绝不出现 undefined / NaN% / 0.0%）与 P0-2 排序护栏。
//
// 对抗变异：把 R116B_CHAMPIONS_SOURCE 指向一份「把比较器改回 score 降序」的副本，
// 排序护栏那条测试必须 FAIL（执行账本里附了实际输出）。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require(path.join(__dirname, "..", "..", "desktop", "node_modules", "jsdom"));

const read = (name) => fs.readFileSync(path.join(__dirname, name), "utf8");
const championsScript = fs.readFileSync(process.env.R116B_CHAMPIONS_SOURCE || path.join(__dirname, "champions.js"), "utf8");
const gameplayScript = read("gameplay.js");
const runtimeScript = read("runtime.js");
const indexHTML = read("index.html");
const championsStyles = read("champions.css");
const gameplayStyles = read("gameplay.css");
const championsBackend = fs.readFileSync(path.join(__dirname, "..", "champions.go"), "utf8");
const hexdataBackend = fs.readFileSync(path.join(__dirname, "..", "hexdata.go"), "utf8");
const gameplayBackend = fs.readFileSync(path.join(__dirname, "..", "gameplay.go"), "utf8");
const heroFixture = JSON.parse(fs.readFileSync(path.join(__dirname, "..", "testdata", "r116", "hexdata-hero-157.json"), "utf8"));

// ---------------------------------------------------------------------------
// 夹具：把真实上游响应按后端的单位约定搬成前端消费的响应形状
// （winRate/pickRate/withoutItemWinRate/stageBaselineWinRate ×100，
//   deltaWinRate/wilsonLowerWinRate 保持上游 0..1）——与 hexdata.go 的
// hexdataItemMetricRows / hexdataAugmentMetricRows / hexdataAugmentStageMetricRows
// 逐字对应，换算写错这里就会红。
// ---------------------------------------------------------------------------

// meta.samplePolicy：low 1..249 / medium 250..999 / high ≥1000。
const sampleTierOf = (games) => (games >= 1000 ? "high" : games >= 250 ? "medium" : "low");
const rarityOf = (value) => ({ 棱彩: "prismatic", 黄金: "gold", 白银: "silver" })[value] || "silver";

function itemRow(row) {
  return {
    assets: [{ id: Number(row.itemId), kind: "item", name: row.itemName, source: "hexdata", path: row.itemImageUrl }],
    score: row.hexScore, winRate: row.winRate * 100, pickRate: row.pickRate * 100, games: row.games,
    deltaWinRate: row.deltaWinRate, wilsonLowerWinRate: row.wilsonLowerWinRate, hexTier: row.hexTier,
    hexLabel: row.hexLabel, hexTierColor: row.hexTierColor, officialTier: row.tier,
    coreDelta: row.coreDelta, averageIndex: row.averageIndex, withoutItemWinRate: row.withoutItemWinRate * 100,
    sampleTier: sampleTierOf(row.games),
  };
}

// 后端 hexdataOfficialGrade 的映射，只用于把真实夹具搬成响应形状：hang→S / top→A /
// elite→B / npc→C / trap→F；insufficient 与未知取值不给字母 → omitempty → 键不存在
// （前端据此整块隐藏徽章，见评审整改 B4）。Go 侧的同一份映射由 hexdata_r116b_test.go
// 独立钉住，这里不是第二份实现，只是夹具构造。
const officialGradeOf = (hexTier) => ({ hang: "S", top: "A", elite: "B", npc: "C", trap: "F" })[hexTier] || "";

function augmentRow(row, overrides = {}) {
  const parentGrade = officialGradeOf(row.hexTier);
  return {
    assets: [{ id: Number(row.augmentId), kind: "augment", name: row.augmentName, description: row.augmentDescription, source: "hexdata", path: row.augmentIconUrl }],
    rarity: rarityOf(row.rarity), ...(parentGrade ? { grade: parentGrade } : {}), score: row.hexScore,
    winRate: row.pairWinRate * 100, pickRate: row.pickRate * 100, games: row.games,
    deltaWinRate: row.deltaWinRate, wilsonLowerWinRate: row.wilsonLowerWinRate,
    hexLabel: row.hexLabel, hexTierColor: row.hexTierColor, officialTier: row.tier,
    sampleTier: sampleTierOf(row.games),
    // 阶段行的形状与后端 championMetricStageRow 逐字对应：带 pickRate（工单 P0-6
    // 「实现要求」第 1 条点名要重渲染的三个值之一；评审整改 B6 曾以「前端零渲染」
    // 裁掉，主控判定工单字面优先、已恢复下发并渲染），B4 起多一个 grade（该阶段
    // 自己的官方字母档位）。取不到的键就让它「不存在」（omitempty 的效果），绝不
    // 写成空串或 0——评审整改 A3 的兜底正是靠「键在不在」来区分「没有」与「等于 0」。
    stages: (row.stages || []).map((stage) => ({
      stage: stage.stage, winRate: stage.winRate * 100,
      ...(Number(stage.pickRate) > 0 ? { pickRate: stage.pickRate * 100 } : {}),
      deltaWinRate: stage.deltaWinRate, wilsonLowerWinRate: stage.wilsonLowerWinRate,
      stageBaselineWinRate: stage.stageBaselineWinRate * 100, games: stage.games,
      hexLabel: stage.hexLabel,
      ...(officialGradeOf(stage.hexTier) ? { grade: officialGradeOf(stage.hexTier) } : {}),
      sampleTier: sampleTierOf(stage.games),
    })),
    ...overrides,
  };
}

const upstreamAugment = heroFixture.augments[0];   // 掷骰狂人（棱彩，含 stages 1..4）
const upstreamItem = heroFixture.items[0];         // 毁坏仪式
const upstreamSpell = heroFixture.summonerSpellPairs[0];

// 表现面板：覆盖后端 hexdataPerformanceFields 的全部五种 format、四组、
// 四个 cumulative 项，以及一项 hasDelta=false（均值不可用）。
function performanceFixture() {
  return {
    heroCount: 173,
    groups: ["战斗 KDA", "伤害输出", "生存辅助", "经济节奏"],
    metrics: [
      { key: "kda", label: "KDA", group: "战斗 KDA", value: 3.4143, format: "decimal2", deltaPercent: 12.4, hasDelta: true },
      { key: "killParticipation", label: "击杀参与率", group: "战斗 KDA", value: 0.76618, format: "percent", deltaPercent: -3.2, hasDelta: true },
      { key: "doubleKills", label: "双杀累计次数", group: "战斗 KDA", value: 4904195, format: "count", cumulative: true },
      { key: "pentaKills", label: "五杀累计次数", group: "战斗 KDA", value: 17543, format: "count", cumulative: true },
      { key: "avgDamage", label: "平均伤害", group: "伤害输出", value: 39885.22, format: "int", deltaPercent: 5.1, hasDelta: true },
      { key: "damageShare", label: "团队伤害占比", group: "伤害输出", value: 0.19214, format: "percent", hasDelta: false },
      { key: "survivability", label: "生存评分", group: "生存辅助", value: 0.93794, format: "decimal3", deltaPercent: 0, hasDelta: true },
      { key: "avgGold", label: "平均金币", group: "经济节奏", value: 16548.59, format: "int", deltaPercent: -1.5, hasDelta: true },
    ],
  };
}

function detailFixture(overrides = {}) {
  return {
    mode: "hextech-aram", region: "CN", source: "Hexdata + OP.GG RSC", patch: "16.18",
    measurementTechnique: "上游公布的统计口径：样本为国服七区海克斯大乱斗，置信区间为 wilson_95",
    citation: { patch: "16.18", reportDate: "2026-09-18", buildId: "hexdata-test" },
    stats: { tier: 2, winRate: 56.7509 },
    recommendedAugments: [
      augmentRow(upstreamAugment),
      augmentRow(heroFixture.augments[1] || upstreamAugment, { rarity: "gold" }),
      augmentRow(heroFixture.augments[2] || upstreamAugment, { rarity: "silver" }),
    ],
    // 后端直出顺序＝官方口径（sample_tier → wilson_lower_bound → pick_rate → games）：
    // 高样本行在前，低样本行在后，即使低样本行的胜率与 score 都更高。
    itemRanking: [
      itemRow(upstreamItem),
      { assets: [{ id: 99999, kind: "item", name: "低样本对照装备", source: "hexdata", path: "/low.png" }], score: 999, winRate: 100, pickRate: 0.4, games: 10, sampleTier: "low" },
    ],
    build: {
      starterItems: [{ assets: [{ id: 1055, kind: "item", name: "多兰之刃", source: "hexdata", path: "/1055.png" }], pickRate: 41.2, games: 120000 }],
      boots: [{ assets: [{ id: 3006, kind: "item", name: "狂战士胫甲", source: "hexdata", path: "/3006.png" }], pickRate: 55.5, games: 120000 }],
      summonerSpells: [{
        assets: upstreamSpell.spellIds.map((id, index) => ({ id: Number(id), kind: "spell", name: upstreamSpell.spellNames[index], source: "hexdata" })),
        winRate: upstreamSpell.winRate * 100, pickRate: upstreamSpell.pickRate * 100, games: upstreamSpell.games,
        deltaWinRate: upstreamSpell.deltaWinRate, wilsonLowerWinRate: upstreamSpell.wilsonLowerWinRate, sampleTier: sampleTierOf(upstreamSpell.games),
      }],
      skills: [{ skillPriority: ["Q", "E", "W"], skillOrder: ["Q", "E", "W", "Q", "Q", "R"], winRate: 60.1, pickRate: 70.5, games: 90000 }],
      coreItems: [
        { assets: [{ id: 3153, kind: "item", name: "破败王者之刃", source: "hexdata", path: "/3153.png" }, { id: 6673, kind: "item", name: "不朽盾弓", source: "hexdata", path: "/6673.png" }], winRate: 58.4, pickRate: 12.3, games: 80000, score: 80 },
        { assets: [{ id: 3031, kind: "item", name: "无尽之刃", source: "hexdata", path: "/3031.png" }], winRate: 57.1, pickRate: 9.8, games: 60000, score: 90 },
      ],
    },
    performance: performanceFixture(),
    ...overrides,
  };
}

const catalogFixture = {
  champions: [{ id: 157, key: "yasuo", slug: "yasuo", nameZh: "疾风剑豪", nameEn: "Yasuo", titleZh: "疾风剑豪", imageSource: "ddragon", imagePath: "/cdn/img/champion/yasuo.png" }],
  tiers: [],
};

const rankingsFixture = {
  mode: "aram-mayhem", region: "CN", source: "Hexdata", patch: "16.18", fetchedAt: "2026-09-20T00:00:00Z",
  rows: [{ championId: 157, key: "yasuo", name: "疾风剑豪", rank: 1, tier: 2, play: 3333412, winRate: 56.75, pickRate: 15.03, tierLocallyCalculated: false }],
  tierBands: [{ tier: 1, label: "前15%" }, { tier: 2, label: "15%-35%" }],
};

const ticks = (count = 8) => new Promise((resolve) => {
  let left = count;
  const step = () => { if (--left <= 0) resolve(); else setImmediate(step); };
  setImmediate(step);
});

// 全应用 jsdom：按 index.html 的真实脚本顺序注入，fetch 全量记账。
// rankings / catalog / detailFor 是给「换英雄」这类多英雄场景用的（评审整改 A1）：
// detailFor 收到的是完整请求 URL，可以按 champion= 参数返回不同的详情负载。
async function mountMayhemDetail(detail = detailFixture(), { mode = "aram-mayhem", rankings = rankingsFixture, catalog = catalogFixture, detailFor = null } = {}) {
  const dom = new JSDOM(indexHTML, { url: "http://localhost/", runScripts: "outside-only", pretendToBeVisual: true });
  const window = dom.window;
  const calls = [];
  window.fetch = async (url) => {
    const target = String(url);
    calls.push(target);
    const payload = target.startsWith("/api/champions/catalog") ? catalog
      : target.startsWith("/api/champions/rankings") ? rankings
      : target.startsWith("/api/champions/detail") ? (detailFor ? detailFor(target) : detail)
      : target.startsWith("/api/champions/augments") ? { source: "Hexdata", rows: [] }
      : {};
    return { ok: true, status: 200, json: async () => payload, text: async () => "" };
  };
  window.localStorage.setItem("lol-loot-champion-mode", mode);
  window.eval(runtimeScript);
  window.eval(championsScript);
  window.dispatchEvent(new window.CustomEvent("deep-legends:section", { detail: { name: "champions" } }));
  await ticks(12);
  return { dom, window, calls, document: window.document };
}

function functionSource(source, name) {
  const start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} not found`);
  let depth = 0;
  let quote = "";
  let escaped = false;
  const bodyStart = source.indexOf("{", source.indexOf(")", start));
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
  assert.fail(`${name} body is not balanced`);
}

// Go 侧的函数体提取（后端契约断言用）。
function goFunctionSource(source, name) {
  const start = source.indexOf(`func ${name}(`);
  assert.notEqual(start, -1, `${name} 不在 Go 源码里`);
  const bodyStart = source.indexOf("{", source.indexOf(")", start));
  let depth = 0;
  for (let index = bodyStart; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    else if (source[index] === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} 的 Go 函数体括号不配对`);
}

function compile(names, dependencies = {}, source = championsScript) {
  const keys = Object.keys(dependencies);
  return Function(...keys, `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn {${names.join(",")}};`)(...keys.map((key) => dependencies[key]));
}

// ---------------------------------------------------------------------------
// A. R128 §2.3：口径/方法论说明已从全项目 UI 删除（撤销 R116-B P0-3）
// ---------------------------------------------------------------------------

test("R128 §2.3 index.html 不再引入 shared.js，其余静态加载顺序不变", () => {
  const order = [...indexHTML.matchAll(/<script src="\/([\w.-]+\.js)" defer>/g)].map((match) => match[1]);
  assert.equal(order.includes("shared.js"), false, "shared.js 已删除，不得再被引入");
  assert.equal(fs.existsSync(path.join(__dirname, "shared.js")), false, "shared.js 文件应已删除");
  for (const name of ["runtime.js", "section-loader.js", "gameplay.js"]) assert.ok(order.includes(name), `${name} 不在 index.html 的静态引入里`);
  assert.ok(order.indexOf("runtime.js") < order.indexOf("gameplay.js"));
  assert.ok(order.indexOf("runtime.js") < order.indexOf("section-loader.js"));
});

test("R128 §2.3 口径说明的实现、调用点与样式在前端全部消失", () => {
  for (const source of [championsScript, gameplayScript]) {
    assert.doesNotMatch(source, /renderMeasurementTechnique/);
    assert.doesNotMatch(source, /deepLegendsShared/);
    assert.doesNotMatch(source, /mayhem-measurement/);
    assert.doesNotMatch(source, /recommendation-measurement/);
    assert.doesNotMatch(source, /统计口径/);
  }
  assert.doesNotMatch(gameplayStyles, /recommendation-measurement/);
  assert.doesNotMatch(championsStyles, /mayhem-measurement|mayhem-detail-footer|mayhem-caution-note/);
  // 详情页不再有页脚节点与页脚函数，推荐区不再有常驻页脚。
  assert.doesNotMatch(championsScript, /mayhemDetailFooter|data-mayhem-detail-footer/);
  const area = functionSource(gameplayScript, "renderRecommendationArea");
  assert.doesNotMatch(area, /measurementTechnique/);
  assert.match(area, /panel\(key, content\[key\] \|\| ""\)\)\.join\(""\)\}<\/section>/, "面板序列之后直接收尾，不再拼页脚");
});

// ---------------------------------------------------------------------------
// B. 三-tab 重整：只渲染当前 tab、工具条节点跨 tab 不重挂载（R128 起已无口径页脚）
// ---------------------------------------------------------------------------

test("R116-B 详情页三 tab 只渲染当前页内容，隐藏 tab 不占任何布局高度", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const panels = () => [...document.querySelectorAll(".mayhem-detail-panel")];
  const tabs = [...document.querySelectorAll("[data-mayhem-detail-tab]")].map((button) => button.dataset.mayhemDetailTab);
  assert.deepEqual(tabs, ["overview", "build", "performance"]);

  // 概览：推荐海克斯 + 开局配置；构筑与表现的内容一个节点都不能在 DOM 里。
  assert.equal(panels().length, 1, "同一时刻只允许一个面板节点");
  assert.equal(panels()[0].dataset.mayhemDetailPanel, "overview");
  assert.ok(document.querySelector(".mayhem-augment-ranking"));
  assert.ok(document.querySelector(".mayhem-opening-config"));
  assert.equal(document.querySelector(".mayhem-item-ranking"), null);
  assert.equal(document.querySelector(".mayhem-performance"), null);
  assert.equal(document.querySelectorAll(".mayhem-detail-content [hidden]").length, 0, "详情面板里不许用 hidden 面板占位（R33 收藏页那类问题）");

  document.querySelector('[data-mayhem-detail-tab="build"]').click();
  assert.equal(panels().length, 1);
  assert.equal(panels()[0].dataset.mayhemDetailPanel, "build");
  assert.ok(document.querySelector(".mayhem-item-ranking"));
  assert.ok(document.querySelector(".mayhem-item-routes"));
  assert.ok(document.querySelector(".mayhem-skill-plan"), "技能加点属于构筑 tab");
  assert.equal(document.querySelector(".mayhem-augment-ranking"), null);
  assert.equal(document.querySelector(".mayhem-opening-config"), null);
  assert.equal(document.querySelector(".mayhem-performance"), null);

  document.querySelector('[data-mayhem-detail-tab="performance"]').click();
  assert.equal(panels().length, 1);
  assert.ok(document.querySelector(".mayhem-performance"));
  assert.equal(document.querySelector(".mayhem-item-ranking"), null);
  assert.equal(document.querySelector(".mayhem-augment-ranking"), null);
  assert.equal(document.querySelector('[data-mayhem-detail-tab="performance"]').getAttribute("aria-selected"), "true");
  assert.equal(document.querySelector('[data-mayhem-detail-tab="overview"]').getAttribute("aria-selected"), "false");
});

test("R128 §2.3 详情页没有口径页脚，切页签时工具条节点不被重新挂载", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  assert.equal(document.querySelector("[data-mayhem-detail-footer]"), null, "页脚已删除");
  assert.doesNotMatch(document.querySelector(".mayhem-detail-content").textContent, /统计口径|上游公布的/);
  const toolbar = document.querySelector(".mayhem-detail-toolbar");
  assert.ok(toolbar, "工具条必须存在");
  for (const tab of ["build", "performance", "overview"]) {
    document.querySelector(`[data-mayhem-detail-tab="${tab}"]`).click();
    assert.equal(document.querySelector(".mayhem-detail-toolbar"), toolbar, `切到 ${tab} 后工具条被重挂了`);
    assert.equal(document.querySelectorAll(".mayhem-detail-panel").length, 1);
    assert.equal(document.querySelector("[data-mayhem-detail-footer]"), null);
  }
});

test("R116-B 表现 tab 只在后端下发指标时出现，缺数据不留空页签", async (t) => {
  const { window, document } = await mountMayhemDetail(detailFixture({ performance: null }));
  t.after(() => window.close());
  const tabs = [...document.querySelectorAll("[data-mayhem-detail-tab]")].map((button) => button.dataset.mayhemDetailTab);
  assert.deepEqual(tabs, ["overview", "build"], "performance 为 null 时整个 tab 不渲染");
  assert.equal(document.querySelector(".mayhem-performance"), null);
});

test("R116-B 图鉴入口按钮在详情面板顶部，复用既有视图状态机而不是新路由", () => {
  const toolbar = functionSource(championsScript, "mayhemDetailToolbar");
  assert.match(toolbar, /class="mayhem-atlas-entry" data-mayhem-view="atlas"/);
  assert.match(toolbar, /离开当前英雄/);
  // 点击后走的是既有的 mayhemView 分支（原地切换），没有新增路由或页面。
  assert.match(championsScript, /const mayhemView = event\.target\.closest\("\[data-mayhem-view\]"\)/);
  assert.match(championsScript, /state\.mayhemView === "atlas" \? renderMayhemAtlas\(filteredAugments\(\)\)/);
  assert.doesNotMatch(championsScript, /location\.(hash|assign|pathname) *=/);
  // 图鉴与稀有度分布不在详情面板里（它们是跨英雄的全局维度）。
  const pane = functionSource(championsScript, "renderMayhemDetailPane");
  assert.doesNotMatch(pane, /renderMayhemAtlas|renderMayhemRarityPanel/);
  for (const name of ["mayhemOverviewTabMarkup", "mayhemBuildTabMarkup"]) {
    assert.doesNotMatch(functionSource(championsScript, name), /renderMayhemAtlas|renderMayhemRarityPanel/);
  }
  assert.match(championsStyles, /\.mayhem-atlas-entry\s*\{[^}]*border:\s*1px dashed/s, "入口按钮要在视觉上区别于三个 tab");
});

// ---------------------------------------------------------------------------
// C. P0-1：收益率 + 「出 vs 不出」对照（真实快照）
// ---------------------------------------------------------------------------

test("R116-B P0-1 装备排行同一行给出胜率、较基准、不出时胜率三个互不相同的数字", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  document.querySelector('[data-mayhem-detail-tab="build"]').click();
  const row = [...document.querySelectorAll(".mayhem-ranking-list article")].find((article) => article.textContent.includes(upstreamItem.itemName));
  assert.ok(row, "真实夹具里的第一件装备没有渲染出来");
  const cells = [...row.querySelectorAll("dl > div")].map((cell) => ({ label: cell.querySelector("dt").textContent, value: cell.querySelector("dd").textContent }));
  const byLabel = Object.fromEntries(cells.map((cell) => [cell.label, cell.value]));
  // 上游 items[0]：winRate 0.616255 / deltaWinRate 0.048746 / withoutItemWinRate 0.496522。
  assert.equal(byLabel["胜率"], "61.63%");
  assert.equal(byLabel["较基准"], "+4.9%", "deltaWinRate 是 0..1 原值，展示要 ×100");
  assert.equal(byLabel["不出时"], "49.65%");
  const distinct = new Set([byLabel["胜率"], byLabel["较基准"], byLabel["不出时"]]);
  assert.equal(distinct.size, 3, "三个数字必须互不相同");
  assert.match(row.querySelector(".mayhem-delta").className, /is-delta-up/);
  // 高样本行给 Wilson 下界 tooltip（0.615451 → 61.55%），不单独占一行。
  assert.match(row.querySelector(".metric-win").getAttribute("data-tooltip"), /95% 置信区间下界 61\.55%/);
});

test("R116-B P0-1 收益率为 0 或字段缺失时不显示「较基准」标签", () => {
  const { mayhemDeltaLabel, mayhemDeltaCell } = compile(["mayhemDeltaLabel", "mayhemDeltaCell", "mayhemDeltaTone"], { number: String, percent: (value) => `${value}%` });
  for (const value of [0, "0", null, undefined, "", NaN, "abc", {}]) {
    assert.equal(mayhemDeltaLabel(value), "", `${JSON.stringify(value)} 不该产出标签`);
    assert.equal(mayhemDeltaCell(value), "");
  }
  assert.equal(mayhemDeltaLabel(0.048746), "+4.9%");
  assert.equal(mayhemDeltaLabel(-0.0123), "-1.2%");
  assert.match(mayhemDeltaCell(-0.0123), /is-delta-down/);
  assert.doesNotMatch(mayhemDeltaCell(-0.0123), /0\.0%/);
  // 评审整改 A2：被 toFixed(1) 抹成 0 的小值同样不许出标签。工单 P0-1 判据禁止的是
  // 「+0.0%」这个字符串本身，不是「精确等于 0」这一个取值；原来的实现只挡 value === 0，
  // 于是 |deltaWinRate| < 0.00005 的行照样渲染出「较基准 +0.0%」。舍入边界两侧都试：
  // 0.00004 → "0.0"（必须挡掉），0.0006 → "0.1"（必须保留，不能一刀切按阈值砍）。
  for (const value of [1e-5, -1e-5, 4e-5, -4e-5, 0.00004999, -0.00004999]) {
    assert.equal(mayhemDeltaLabel(value), "", `${value} 会被一位小数抹成 0.0，不该出标签`);
    assert.equal(mayhemDeltaCell(value), "");
  }
  assert.equal(mayhemDeltaLabel(0.0006), "+0.1%");
  assert.equal(mayhemDeltaLabel(-0.0006), "-0.1%");
  assert.equal(mayhemDeltaLabel(0.02), "+2.0%");
  // 评审整改 B6：装备排行的「较基准」带上口径说明，别让它是个没有定义的词。这一格
  // 的基准是英雄整体胜率——实测 4 个英雄共 479 条装备行，deltaWinRate =
  // winRate − heroWinRate 零误差（英雄级口径；阶段行的基准另有一格可见数字）。
  const cell = mayhemDeltaCell(0.048746);
  assert.match(cell, /data-tooltip="较基准 = 这件装备的胜率/);
  assert.match(cell, /该英雄整体胜率" data-tooltip-size="compact"/);
  assert.match(cell, /tabindex="0"/, "tooltip 必须键盘可达，与 Wilson 下界同一套写法");
  assert.doesNotMatch(cell, /undefined|NaN/);
  assert.equal(mayhemDeltaCell(0), "", "拿不到值时整格不渲染，tooltip 也不许留下");
});

test("R116-B P0-1 海克斯卡带较基准与官方档位，字段缺失时整块隐藏", () => {
  const deps = {
    state: { mode: "aram-mayhem", mayhemStage: 0 },
    escapeHTML: (value) => String(value ?? ""),
    percent: (value) => (Number.isFinite(Number(value)) ? `${Number(value).toFixed(2)}%` : "—"),
    number: (value, digits = 1) => (Number.isFinite(Number(value)) ? Number(value).toFixed(digits) : "—"),
    compactNumber: (value) => String(value ?? "—"),
    objectRows: (value) => (Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []),
  };
  const { mayhemAugmentConfidenceMetrics } = compile(["mayhemAugmentConfidenceMetrics", "mayhemDeltaLabel", "mayhemDeltaTone", "mayhemSampleTierLabel", "mayhemWilsonLabel"], deps);
  const full = mayhemAugmentConfidenceMetrics({ deltaWinRate: 0.111527, hexLabel: "夯", sampleTier: "high", wilsonLowerWinRate: 0.675997 });
  assert.deepEqual(full.map(([label]) => label), ["较基准", "官方档位", "95%下界"]);
  assert.equal(full[0][1], "+11.2%");
  assert.equal(full[1][1], "夯", "官方档位展示 hexLabel（中文），不是 hexTier 枚举名");
  assert.equal(full[2][1], "67.60%");
  // 全部字段缺失 → 一格都不加，卡片保持原来的三格，不会出现 undefined / NaN%。
  assert.deepEqual(mayhemAugmentConfidenceMetrics({}), []);
  assert.deepEqual(mayhemAugmentConfidenceMetrics({ deltaWinRate: 0, hexLabel: "", sampleTier: "", wilsonLowerWinRate: 0 }), []);
  // 低样本行只给「样本极少」，不再叠一个没有参考意义的 Wilson 下界。
  const low = mayhemAugmentConfidenceMetrics({ sampleTier: "low", wilsonLowerWinRate: 0.42, deltaWinRate: 0.2 });
  assert.deepEqual(low.map(([label, value]) => `${label}=${value}`), ["较基准=+20.0%", "置信=样本极少"]);
  assert.match(low[1][2], /is-low-confidence/);
  const rendered = low.map(([label, value, tone]) => `${label}|${value}|${tone}`).join(" ");
  assert.doesNotMatch(rendered, /undefined|NaN/);
});

// ---------------------------------------------------------------------------
// D. P0-2：样本置信度 + 排序护栏（R63 的老毛病）
// ---------------------------------------------------------------------------

test("R116-B P0-2 低样本行带「样本极少」徽记，且排不到高样本行前面", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  document.querySelector('[data-mayhem-detail-tab="build"]').click();
  const rows = [...document.querySelectorAll(".mayhem-ranking-list article")];
  assert.equal(rows.length, 2);
  // 后端直出顺序：5000+ 场的真实装备在前，10 场 100% 的对照行在后。
  assert.match(rows[0].textContent, new RegExp(upstreamItem.itemName));
  assert.match(rows[1].textContent, /低样本对照装备/);
  assert.match(rows[1].textContent, /样本极少/);
  assert.match(rows[1].className, /is-low-confidence/);
  assert.doesNotMatch(rows[0].textContent, /样本极少/);
  // 低样本行没有 Wilson 下界 tooltip（下界对 10 场样本没有参考意义）。
  assert.equal(rows[1].querySelector(".metric-win").getAttribute("data-tooltip"), null);
  // 排序护栏：低样本行即使胜率 100%、score 999，也不能插到高样本行前面。
  assert.ok(rows[0].textContent.includes("61.63%"), "高样本行必须仍在第一位");
});

test("R116-B P0-2 海斗路径不再有任何前端重排（对抗变异必须打红这条）", () => {
  const ranking = functionSource(championsScript, "renderMayhemItemRanking");
  assert.doesNotMatch(ranking, /\.sort\(/, "装备排行不得重排后端直出顺序");
  assert.match(ranking, /objectRows\(rows\)\.slice\(0, 8\)/);
  const routes = functionSource(championsScript, "renderMayhemItemRoutes");
  assert.doesNotMatch(routes, /sortedGradeRows|\.sort\(/);
  assert.match(functionSource(championsScript, "mayhemRouteRows"), /return objectRows\(rows\);/);
  const recommended = functionSource(championsScript, "renderRecommendedAugments");
  assert.doesNotMatch(recommended, /\.sort\(/, "海克斯推荐只按稀有度各取前三，保持后端顺序");
  assert.match(recommended, /return count < 3;/);
  // 护栏只动海斗调用点：sortedGradeRows 函数体与斗魂/YOUR.GG 三处边界原样保留。
  const sorted = functionSource(championsScript, "sortedGradeRows");
  assert.match(sorted, /gradeRank\(augmentGrade\(left\.tier \|\| left\.grade, left\.score, scores\)\)/);
  assert.match(championsScript, /const sorted = sortedGradeRows\(filtered\);/, "斗魂 renderArenaAugmentSection 仍用 sortedGradeRows");
  assert.match(championsScript, /YOUR\.GG's default equipment order is tier, then sample count \(not score\)\./);
  assert.match(functionSource(championsScript, "sortedArenaRows"), /const metric = state\.arenaSort;/);
});

test("R116-B P0-2 sampleTier / wilsonLowerWinRate 缺失时对应元素不渲染", () => {
  const deps = { percent: (value) => (Number.isFinite(Number(value)) ? `${Number(value).toFixed(2)}%` : "—"), number: String, compactNumber: String, escapeHTML: (value) => String(value ?? "") };
  const { mayhemSampleCell, mayhemWilsonTooltip, mayhemWilsonLabel, mayhemWithoutItemCell } = compile(["mayhemSampleCell", "mayhemWilsonTooltip", "mayhemWilsonLabel", "mayhemSampleTierLabel", "mayhemWithoutItemCell"], deps);
  for (const row of [{}, { sampleTier: "" }, { sampleTier: "medium" }, { sampleTier: "high" }, null, undefined]) {
    assert.equal(mayhemSampleCell(row), "");
    assert.equal(mayhemWilsonTooltip(row), "");
  }
  // meta 不可用 → sampleTier 为空 → 即使有 wilson 值也不渲染（分档是它的前提）。
  assert.equal(mayhemWilsonLabel({ wilsonLowerWinRate: 0.52 }), "");
  assert.equal(mayhemWilsonLabel({ sampleTier: "high", wilsonLowerWinRate: 0 }), "");
  assert.equal(mayhemWilsonLabel({ sampleTier: "low", wilsonLowerWinRate: 0.52 }), "");
  assert.match(mayhemWilsonTooltip({ sampleTier: "high", wilsonLowerWinRate: 0.615451 }), /95% 置信区间下界 61\.55%/);
  // withoutItemWinRate 缺失或为 0：整格不渲染（0 会被读成「不出这件必败」）。
  assert.equal(mayhemWithoutItemCell({}), "");
  assert.equal(mayhemWithoutItemCell({ withoutItemWinRate: 0 }), "");
  assert.match(mayhemWithoutItemCell({ withoutItemWinRate: 49.6522 }), /49\.65%/);
  for (const markup of [mayhemSampleCell({ sampleTier: "low" }), mayhemWilsonTooltip({ sampleTier: "high", wilsonLowerWinRate: 0.5 }), mayhemWithoutItemCell({ withoutItemWinRate: 49.65 })]) {
    assert.doesNotMatch(markup, /undefined|NaN/);
  }
});

// ---------------------------------------------------------------------------
// E. P0-4：召唤师技能带胜率，出门装/鞋子保持只有选用率
// ---------------------------------------------------------------------------

test("R116-B P0-4 召唤师技能同时显示胜率与选用率，出门装与鞋子行为不变", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const sections = [...document.querySelectorAll(".mayhem-opening-grid > section")];
  assert.deepEqual(sections.map((section) => section.querySelector("h4").textContent), ["出门装", "鞋子", "召唤师技能"]);
  const statsOf = (section) => [...section.querySelectorAll(".option-stats dt")].map((node) => node.textContent);
  assert.deepEqual(statsOf(sections[0]), ["选用率"], "出门装仍只有选用率");
  assert.deepEqual(statsOf(sections[1]), ["选用率"], "鞋子仍只有选用率");
  assert.deepEqual(statsOf(sections[2]), ["选用率", "胜率"], "召唤师技能要同时给出胜率与选用率（三态开关传 null）");
  // 真实夹具：spellIds ["4","32"] → winRate 0.569432 / pickRate 0.907758。
  const spellText = sections[2].textContent;
  assert.match(spellText, /56\.94%/);
  assert.match(spellText, /90\.78%/);
  assert.doesNotMatch(spellText, /undefined|NaN/);
});

test("R116-B P0-4 只有召唤师技能那一组打开了胜率开关", () => {
  const opening = functionSource(championsScript, "renderMayhemOpeningConfiguration");
  assert.match(opening, /renderConfigOption\(row, kind, false\)/);
  assert.match(opening, /renderConfigOption\(row, "spell", null, spellStats\)/);
  assert.equal((opening.match(/renderConfigOption\(/g) || []).length, 2);
  // 三态开关本身没被改（工单 P0-4 明写不要动这个函数）。
  assert.match(functionSource(championsScript, "renderConfigOption"), /function renderConfigOption\(row, kind, showWinRate = null, statsRenderer = null\)/);
  assert.match(functionSource(championsScript, "renderConfigOption"), /showWinRate === false \? renderOptionPickStats\(row\) : renderOptionStats\(row\)/);
});

test("R116-B P0-4 上游回退到 pickRate-only 数据时只显示选用率，不整块报错", () => {
  // 判据 2：summonerSpellPairs 为空时后端自动回退到只有 pickRate 的那一份数据
  // （winRate 为 0）。既有的 renderOptionStats 只用 hasPick/hasWin 决定「整块要不要
  // 渲染」，之后两格是无条件输出的，直接复用它会渲染出「胜率 0.00%」——显示不存在
  // 的数据。所以召唤师技能走 renderConfigOption 的第四个参数（statsRenderer），
  // 逐格判断「大于 0 才渲染」。这里编译真实实现来钉住这条降级。
  const { renderMayhemOpeningConfiguration } = compile(
    ["renderMayhemOpeningConfiguration", "renderConfigOption", "renderOptionPickStats", "renderOptionStats"],
    {
      objectRows: (value) => (Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []),
      percent: (value) => (Number.isFinite(Number(value)) ? `${Number(value).toFixed(2)}%` : "—"),
      escapeHTML: (value) => String(value ?? ""),
      summonerSpellAsset: (asset) => asset,
      renderAssetButton: (asset) => `<button>${asset?.name || ""}</button>`,
    },
  );
  const build = (spells) => ({
    starterItems: [{ assets: [{ name: "多兰之刃" }], pickRate: 41.2 }],
    boots: [{ assets: [{ name: "狂战士胫甲" }], pickRate: 55.5 }],
    summonerSpells: spells,
  });
  const fallback = renderMayhemOpeningConfiguration(build([{ assets: [{ id: 4, name: "闪现" }], pickRate: 90.78, winRate: 0, games: 2153718 }]), {});
  assert.match(fallback, /<dt>选用率<\/dt><dd>90\.78%<\/dd>/);
  assert.doesNotMatch(fallback, /胜率|0\.00%/, "回退行没有胜率，就不许渲染胜率格");
  const withWinRate = renderMayhemOpeningConfiguration(build([{ assets: [{ id: 4, name: "闪现" }], pickRate: 90.78, winRate: 56.94, games: 2153718 }]), {});
  assert.match(withWinRate, /<dt>选用率<\/dt><dd>90\.78%<\/dd>/);
  assert.match(withWinRate, /<dt>胜率<\/dt><dd>56\.94%<\/dd>/);
  // 两个速率都取不到 → 整块空，不留一个空壳 dl。
  const empty = renderMayhemOpeningConfiguration(build([{ assets: [{ id: 4, name: "闪现" }], pickRate: 0, winRate: 0 }]), {});
  const spellSection = empty.slice(empty.indexOf("<h4>召唤师技能</h4>"));
  assert.doesNotMatch(spellSection, /option-stats/, "取不到任何速率时不渲染统计块");
  assert.match(spellSection, /闪现/, "图标与名称照常渲染，只是没有统计数字");
});

// ---------------------------------------------------------------------------
// F. P0-5：官方档位用 hexLabel，本地估算要标注，官方缺失要隐藏
// ---------------------------------------------------------------------------

test("R116-B P0-5 官方档位展示中文 hexLabel，绝不展示内部枚举名 hexTier", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const card = [...document.querySelectorAll(".mayhem-recommend-grid .arena-option-card")][0];
  assert.match(card.textContent, /官方档位夯/, "上游 hexLabel 是「夯」");
  assert.doesNotMatch(card.textContent, /hang/, "hexTier 是内部枚举名，不许露出");
  assert.doesNotMatch(document.querySelector(".mayhem-detail-pane").textContent, /\bhang\b|\btop\b|insufficient/);
  // 英雄级梯度用官方 stats.tier（fixture 里是 2）。
  assert.match(document.querySelector(".mayhem-overview-metrics").textContent, /梯度T2/);
});

test("R116-B P0-5 stats.tier 为 nil 时梯度徽章整块隐藏，不用行级档位冒充", async (t) => {
  const detail = detailFixture();
  detail.stats = { winRate: 56.7509 };
  const { window, document } = await mountMayhemDetail(detail);
  t.after(() => window.close());
  const metrics = document.querySelector(".mayhem-overview-metrics");
  assert.doesNotMatch(metrics.textContent, /梯度/, "官方档位不可用就整块隐藏");
  assert.equal(document.querySelector(".mayhem-overview-identity .tier-badge"), null);
  assert.equal(document.querySelector(".mayhem-overview-identity .tier-badge-fallback"), null, "也不许退化成「—」占位");
  assert.doesNotMatch(metrics.textContent, /undefined|NaN|T—/);
});

test("R116-B P0-5 榜单行 tierLocallyCalculated 时显示「本地估算」小字", () => {
  const row = functionSource(championsScript, "renderChampionRow");
  assert.match(row, /row\.tierLocallyCalculated === true \? '<small class="mayhem-tier-local">本地估算<\/small>'/);
  assert.match(championsStyles, /\.mayhem-tier-local\s*\{[^}]*color:\s*var\(--warning\)/s);
  // 后端红线：insights 不可用时 Stats.Tier 必须留 nil（已有 Go 测试钉住），
  // 这里锁住前端确实读了这个字段而不是自己算。
  assert.match(functionSource(championsScript, "mayhemHeroTier"), /Number\(detail\?\.stats\?\.tier\)/);
  assert.match(functionSource(championsScript, "mayhemHeroTier"), /return Number\.isFinite\(official\) && official > 0 \? \{ tier: official, locallyCalculated: false \} : null;/);
});

// ---------------------------------------------------------------------------
// G. P0-6：阶段筛选 chips —— 纯本地重渲染，不发请求
// ---------------------------------------------------------------------------

test("R116-B P0-6 切阶段只改数字不发请求，切回汇总恢复原值", async (t) => {
  const { window, document, calls } = await mountMayhemDetail();
  t.after(() => window.close());
  const chips = [...document.querySelectorAll("[data-mayhem-stage]")];
  assert.deepEqual(chips.map((chip) => chip.dataset.mayhemStage), ["0", "1", "2", "3", "4"]);
  const firstCard = () => document.querySelector(".mayhem-recommend-grid .arena-option-card");
  const summary = firstCard().textContent;
  // 汇总视图用的是英雄级 pairWinRate（0.679036 → 67.90%）。
  assert.match(summary, /67\.90%/);
  const before = calls.length;

  chips[1].click();
  assert.equal(calls.length, before, "点阶段 chip 不许产生任何新请求");
  const stageOne = firstCard().textContent;
  // 阶段 1：winRate 0.705363 → 70.54%，pickRate 0.02498 → 2.50%，deltaWinRate
  // 0.137788 → +13.8%，games 59256。工单 P0-6「实现要求」第 1 条点名的三个值
  // （winRate / deltaWinRate / pickRate）必须全部随 chip 重渲染，少一个都不算达标。
  assert.match(stageOne, /70\.54%/);
  assert.match(stageOne, /\+13\.8%/);
  assert.match(stageOne, /选取率2\.50%/, "阶段视图必须渲染该阶段的 pickRate（工单 P0-6 实现要求第 1 条）");
  assert.notEqual(stageOne, summary);
  assert.match(document.querySelector('[data-mayhem-stage="1"]').className, /is-active/);

  document.querySelector('[data-mayhem-stage="3"]').click();
  assert.equal(calls.length, before, "切到阶段 3 同样不许发请求");
  // 阶段 3 样本只有 373 场（medium），官方档位从「夯」变成「顶级」。
  assert.match(firstCard().textContent, /61\.66%/);
  assert.match(firstCard().textContent, /官方档位顶级/);

  document.querySelector('[data-mayhem-stage="0"]').click();
  assert.equal(calls.length, before);
  assert.equal(firstCard().textContent, summary, "切回汇总必须恢复英雄级数值");
  // 汇总视图不含「选取率」格：工单 P0-6 第 2 条只要求默认视图展示英雄级汇总的
  // winRate，多选一格属超范围视觉改动。选取率只在阶段视图出现（下面钉住）。
  assert.doesNotMatch(summary, /选取率/, "汇总视图不该出现选取率格");
});

test("R116-B P0-6 缺某个阶段的 augment 在该阶段视图下整条不渲染，绝不填 0", async (t) => {
  const detail = detailFixture();
  // 真实上游形态：阶段 1 只有 121/126 条 augment 有 stage 行。
  const [first, ...rest] = detail.recommendedAugments;
  const droppedName = first.assets[0].name;
  detail.recommendedAugments = [{ ...first, stages: first.stages.filter((stage) => stage.stage !== 1) }, ...rest];
  const { window, document } = await mountMayhemDetail(detail);
  t.after(() => window.close());
  const cards = () => [...document.querySelectorAll(".mayhem-recommend-grid .arena-option-card")];
  assert.equal(cards().length, 3, "汇总视图三条都在");
  // 其余两条仍带阶段 1，所以 chip 照常出现；缺阶段 1 的那条在阶段 1 视图下整条消失。
  document.querySelector('[data-mayhem-stage="1"]').click();
  assert.equal(cards().length, 2);
  assert.equal(cards().some((card) => card.textContent.includes(droppedName)), false, "缺阶段 1 的 augment 不得渲染");
  const stageText = document.querySelector("[data-mayhem-augments]").textContent;
  assert.doesNotMatch(stageText, /0\.00%/, "绝不许用 0 顶替缺失的阶段数值");
  assert.doesNotMatch(stageText, /undefined|NaN/);
  // 全部行都缺阶段 2 时：阶段 2 的 chip 整块消失（不给一个点了没反应的控件），
  // 其余阶段照常工作。
  const emptied = detailFixture();
  emptied.recommendedAugments = emptied.recommendedAugments.map((row) => ({ ...row, stages: row.stages.filter((stage) => stage.stage !== 2) }));
  const second = await mountMayhemDetail(emptied);
  try {
    const doc = second.document;
    const stageChips = [...doc.querySelectorAll("[data-mayhem-stage]")].map((chip) => chip.dataset.mayhemStage);
    assert.deepEqual(stageChips, ["0", "1", "3", "4"]);
    assert.equal(doc.querySelectorAll(".mayhem-recommend-grid .arena-option-card").length, 3);
    doc.querySelector('[data-mayhem-stage="3"]').click();
    assert.equal(doc.querySelectorAll(".mayhem-recommend-grid .arena-option-card").length, 3);
    assert.doesNotMatch(doc.querySelector("[data-mayhem-augments]").textContent, /0\.00%|undefined|NaN/);
  } finally { second.window.close(); }
});

test("R116-B P0-6 没有阶段数据时 chips 整块不渲染（斗魂路径）", () => {
  const deps = {
    objectRows: (value) => (Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []),
    normalizeMayhemStage: (value) => (Number.isInteger(Number(value)) && Number(value) >= 0 && Number(value) <= 4 ? Number(value) : 0),
    MAYHEM_STAGE_OPTIONS: [[0, "汇总"], [1, "阶段 1"]],
  };
  const mayhemAugmentStageRow = (item, stage) => (Number(stage) > 0 ? (item?.stages || []).find((row) => Number(row.stage) === Number(stage)) || null : null);
  const { renderMayhemStageChips } = compile(["renderMayhemStageChips"], { ...deps, mayhemAugmentStageRow, state: { mode: "aram-mayhem", mayhemStage: 0 } });
  assert.equal(renderMayhemStageChips([{ winRate: 50 }]), "", "没有 stages 就不给一个点了没反应的控件");
  assert.equal(renderMayhemStageChips([]), "");
  const { renderMayhemStageChips: arenaChips } = compile(["renderMayhemStageChips"], { ...deps, mayhemAugmentStageRow, state: { mode: "arena", mayhemStage: 0 } });
  assert.equal(arenaChips([{ stages: [{ stage: 1 }] }]), "", "斗魂路径不显示海斗的阶段筛选");
  assert.match(renderMayhemStageChips([{ stages: [{ stage: 1 }, { stage: 2 }] }]), /data-mayhem-stage="0"/);
});

// ---------------------------------------------------------------------------
// H. P1-6：表现指标面板
// ---------------------------------------------------------------------------

test("R128 §2.2 表现面板逐行排版：数值与「较平均」分开，多杀合并一行，说明只在底部一次", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  document.querySelector('[data-mayhem-detail-tab="performance"]').click();
  const groups = [...document.querySelectorAll(".mayhem-performance-group")];
  assert.deepEqual(groups.map((group) => group.querySelector("h4").textContent), ["战斗 KDA", "伤害输出", "生存辅助", "经济节奏"]);
  const cells = [...document.querySelectorAll(".mayhem-performance-group dl > div")];
  const byLabel = Object.fromEntries(cells.map((cell) => [cell.querySelector("dt").textContent, cell.querySelector("dd")]));
  // 五种 format 逐个核对（decimal2 / percent / int / decimal3）。
  assert.equal(byLabel["KDA"].querySelector("b").textContent, "3.41");
  assert.equal(byLabel["击杀参与率"].querySelector("b").textContent, "76.62%", "percent 的 value 是上游 0..1，要 ×100");
  assert.equal(byLabel["平均伤害"].querySelector("b").textContent, "39,885");
  assert.equal(byLabel["生存评分"].querySelector("b").textContent, "0.938");
  // 数值与「较平均」是两个独立元素，不会再粘成「3.41较平均 +12.4%」。
  const kdaDelta = byLabel["KDA"].querySelector(".mayhem-delta");
  assert.ok(kdaDelta, "「较平均」必须是独立元素");
  assert.equal(kdaDelta.textContent, "较平均 +12.4%");
  assert.notEqual(byLabel["KDA"].querySelector("b"), kdaDelta);
  assert.equal(byLabel["击杀参与率"].querySelector(".mayhem-delta").textContent, "较平均 -3.2%");
  assert.match(kdaDelta.className, /is-delta-up/);
  // hasDelta=false / delta 真为 0 → 不渲染「较平均」。
  assert.doesNotMatch(byLabel["团队伤害占比"].textContent, /较平均/);
  assert.doesNotMatch(byLabel["生存评分"].textContent, /较平均/);
  // 四个累计项合并成战斗组里的一行「多杀累计」，不带 ±%。
  const multikill = byLabel["多杀累计"];
  assert.ok(multikill, "累计项合并成一行");
  assert.match(multikill.parentElement.className, /is-cumulative/);
  assert.deepEqual([...multikill.querySelectorAll("span")].map((span) => span.textContent), ["双杀4,904,195", "五杀17,543"]);
  assert.doesNotMatch(multikill.textContent, /较平均/);
  assert.equal(cells.filter((cell) => cell.className.includes("is-cumulative")).length, 1, "四个累计项只占一行");
  // 说明挪到表现 tab 底部，只出现一次；每格里不再重复。
  const notes = [...document.querySelectorAll(".mayhem-performance-note")];
  assert.equal(notes.length, 1);
  assert.equal(notes[0].textContent, "双杀/三杀/四杀/五杀为累计次数，受出场场次影响，不可跨英雄直接比较。");
  assert.match(notes[0].parentElement.className, /mayhem-performance/);
  for (const group of groups) assert.doesNotMatch(group.textContent, /累计次数（受出场场次影响/);
  // 副标题长句删除，只留标题与对比英雄数。
  assert.equal(document.querySelector(".mayhem-performance header p"), null);
  assert.match(document.querySelector(".mayhem-performance").textContent, /对比 173 位英雄/);
  assert.doesNotMatch(document.querySelector(".mayhem-performance").textContent, /undefined|NaN|N\/A/);
});

test("R116-B P1-6 某组指标全缺时该组整体隐藏，不留空态", () => {
  const deps = {
    objectRows: (value) => (Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []),
    escapeHTML: (value) => String(value ?? ""),
    percent: (value) => `${Number(value).toFixed(2)}%`,
    number: (value, digits = 1) => Number(value).toFixed(digits),
    compactNumber: (value) => String(value),
  };
  const names = ["renderMayhemPerformancePanel", "mayhemPerformanceGroups", "mayhemPerformanceGroup", "mayhemPerformanceValue", "mayhemPerformanceDelta", "mayhemPerformanceDeltaTone"];
  const { renderMayhemPerformancePanel } = compile(names, deps);
  const panel = performanceFixture();
  const full = renderMayhemPerformancePanel(panel);
  assert.match(full, /经济节奏/);
  // 后端整组不下发（Groups 里没有、metrics 里也没有）→ 前端不留空组。
  const dropped = { ...panel, groups: panel.groups.filter((group) => group !== "经济节奏"), metrics: panel.metrics.filter((metric) => metric.group !== "经济节奏") };
  const without = renderMayhemPerformancePanel(dropped);
  assert.doesNotMatch(without, /经济节奏/);
  assert.equal((without.match(/<section class="mayhem-performance-group"/g) || []).length, 3, "缺的那一组不得留空壳");
  // Groups 里声明了但指标全缺 → 同样不渲染空组。
  const declaredOnly = { ...panel, metrics: panel.metrics.filter((metric) => metric.group !== "生存辅助") };
  assert.doesNotMatch(renderMayhemPerformancePanel(declaredOnly), /生存辅助/);
  // 整个面板取不到 → 返回空串（tab 因此不出现），不留 0 或 N/A。
  for (const empty of [null, undefined, {}, { metrics: [] }, { metrics: [{ key: "kda", label: "KDA", group: "战斗 KDA", value: 3.4, format: "unknown-format" }] }]) {
    assert.equal(renderMayhemPerformancePanel(empty), "", `${JSON.stringify(empty)} 该整块隐藏`);
  }
});

test("R116-B P1-6 后端把 22 项里的四项累计计数标成 Cumulative，均值不在前端算", () => {
  assert.equal((hexdataBackend.match(/Cumulative: true/g) || []).length, 4);
  for (const key of ["doubleKills", "tripleKills", "quadraKills", "pentaKills"]) {
    assert.match(hexdataBackend, new RegExp(`Key: "${key}"[\\s\\S]{0,120}Cumulative: true`), `${key} 必须标记为累计计数`);
  }
  // 均值在后端算（工单 P1-6-2）：前端不得出现 173 英雄表或本地均值计算。
  // 均值在后端算（工单 P1-6-2）：前端不得自己拉 postmatch 全量表或本地求均值。
  assert.doesNotMatch(championsScript, /\/api\/hexdata\/postmatch|heroAverages|averageOfAllHeroes/);
  const panelSource = ["renderMayhemPerformancePanel", "mayhemPerformanceValue", "mayhemPerformanceDelta", "mayhemPerformanceGroups"].map((name) => functionSource(championsScript, name)).join("\n");
  assert.doesNotMatch(panelSource, /reduce\(|\/\s*metrics\.length/, "前端不得自己算均值");
  assert.match(championsBackend, /DeltaPercent float64\s+`json:"deltaPercent,omitempty"`/);
  assert.match(championsBackend, /HasDelta\s+bool\s+`json:"hasDelta,omitempty"`/);
});

// ---------------------------------------------------------------------------
// I. 对抗变异：空描述、以及 P0-3 的按钮置空回归
// ---------------------------------------------------------------------------

test("R116-B 对抗变异：augmentDescription 为空字符串时不渲染空描述", async (t) => {
  const detail = detailFixture();
  detail.recommendedAugments = detail.recommendedAugments.map((row) => ({ ...row, assets: row.assets.map((asset) => ({ ...asset, description: "" })) }));
  const { window, document } = await mountMayhemDetail(detail);
  t.after(() => window.close());
  const card = document.querySelector(".mayhem-recommend-grid .arena-option-card");
  assert.ok(card);
  const tooltip = card.querySelector("[data-tooltip]")?.getAttribute("data-tooltip") || "";
  assert.doesNotMatch(tooltip, /\n\s*$/, "空描述不得留下一个空行");
  assert.doesNotMatch(card.textContent, /undefined|null/);
  // 说明为空时卡片仍然给出名称，而不是一段空白。
  assert.match(card.textContent, new RegExp(upstreamAugment.augmentName));
});

test("R128 §2.3 推荐区没有常驻口径页脚，且 hasAugments 分支行为不变", () => {
  const area = functionSource(gameplayScript, "renderRecommendationArea");
  assert.doesNotMatch(area, /recommendation-measurement|measurementTechnique/);
  // Anti-scope 第 3 条：按钮置空的条件一字未改，只补了注释。
  const build = functionSource(gameplayScript, "renderBuildRecommendation");
  assert.match(build, /const action = capabilities\.hasAugments \? "" : `<footer class="recommendation-action item-set-action">/);
  assert.match(build, /R116-B P0-3-2：这个 `capabilities\.hasAugments \? "" : <footer>` 是有意设计/);
  assert.match(build, /海斗\/斗魂的海克斯局不支持写入客户端装备方案/);
  assert.equal((build.match(/const action = capabilities\.hasAugments \? "" :/g) || []).length, 1, "条件本身只能有一处，且一字未改");
});

test("R116-B P0-3 海斗局 hasAugments=true 时写入按钮为空，经典局仍显示按钮", () => {
  const dependencies = {
    state: { recommendationTab: "build", recommendationTabTouched: true, liveRecommendations: new Map(), liveRecommendationFailures: new Map() },
    window: {},
    escapeHTML: (value) => String(value ?? ""),
    liveRecommendationTarget: () => ({ key: "157:mid", championId: 157 }),
    // 后端仍然可能下发口径说明；R128 起前端一律不渲染它。
    liveRecommendationsFor: () => ({ measurementTechnique: "上游公布的口径说明" }),
    liveAugmentRecommendationSource: () => "hextech",
    recommendationCapabilities: () => ({ hasRunes: false, hasAugments: true, hasItemDepths: false }),
    recommendationTabSpecs: () => [["build", "海克斯与出装"], ["insight", "详情"]],
    recommendationActiveTab: () => "build",
    recommendationPanelBusy: () => false,
    liveRecommendationFlightActive: () => false,
    selectedRuneRecommendation: () => null,
    renderLiveInsights: () => "",
    renderRecommendationDataNotices: () => "",
    renderLiveAugmentRecommendations: () => "",
    // 真实 renderBuildRecommendation 内部自己算 capabilities；这里用 data 上的
    // 标记复现同一条分支：hasAugments=true → 按钮为空，经典局 → 显示按钮。
    renderBuildRecommendation: (build, self, abilities, data) => (data?.__hasAugments ? "" : '<footer class="recommendation-action item-set-action">游戏装备方案</footer>'),
    renderChampionRecommendationHeader: () => "",
    recommendationEmptyPanel: (title) => `<strong>${title}</strong>`,
  };
  const { renderRecommendationArea } = compile(["renderRecommendationArea"], dependencies, gameplayScript);
  const mayhem = renderRecommendationArea({ available: true, phase: "ChampSelect", players: [], __hasAugments: true });
  assert.doesNotMatch(mayhem, /统计口径|上游公布的|measurement/, "口径说明不得再进推荐区");
  assert.doesNotMatch(mayhem, /item-set-action/, "海斗局不显示写入客户端装备方案的按钮");
  assert.match(mayhem, /id="recommendation-panel-build"/);

  const classic = compile(["renderRecommendationArea"], {
    ...dependencies,
    liveRecommendationsFor: () => ({}),
    recommendationCapabilities: () => ({ hasRunes: false, hasAugments: false, hasItemDepths: true }),
    recommendationTabSpecs: () => [["insight", "详情"], ["build", "出装与技能"]],
  }, gameplayScript).renderRecommendationArea({ available: true, phase: "ChampSelect", players: [], __hasAugments: false });
  assert.match(classic, /item-set-action/, "经典局仍然显示按钮");
  assert.doesNotMatch(classic, /recommendation-measurement/);
});

// ---------------------------------------------------------------------------
// J. 后端契约：stages 下发与单位（与 hexdata_r116b_test.go 对应的前端侧）
// ---------------------------------------------------------------------------

test("R116-B P0-6 后端下发 stages 且单位与父行一致", () => {
  assert.match(championsBackend, /Stages \[\]championMetricStageRow `json:"stages,omitempty"`/);
  // 切片终点用函数定义（"func hexdataOfficialGrade("）而不是裸标识符：整改 B4 之后
  // 结构体上方的注释里也出现了这个名字，用裸标识符会切出一个空串。
  const stageStruct = championsBackend.slice(championsBackend.indexOf("type championMetricStageRow struct {"), championsBackend.indexOf("func hexdataOfficialGrade("));
  assert.ok(stageStruct.includes("SampleTier"), "stage 结构切片切空了，检查上面的锚点");
  for (const field of ['Stage                int     `json:"stage"`', '`json:"winRate,omitempty"`', '`json:"deltaWinRate,omitempty"`', '`json:"wilsonLowerWinRate,omitempty"`', '`json:"stageBaselineWinRate,omitempty"`', '`json:"games,omitempty"`', '`json:"hexLabel,omitempty"`', '`json:"grade,omitempty"`', '`json:"sampleTier,omitempty"`']) {
    assert.match(stageStruct, new RegExp(field.replace(/[`[\]{}()*+?.\\|^]/g, "\\$&").replace(/\s+/g, "\\s+")), `stage 结构缺少 ${field}`);
  }
  // 主控裁定（推翻评审整改 B6 的这一半）：工单 P0-6「实现要求」第 1 条明写点击阶段
  // chip 后要重新渲染该阶段的 winRate/deltaWinRate/pickRate 三个值，所以阶段行必须
  // 带 pickRate。B6 以「前端零渲染」为由裁掉它，但正确解法是把这一格渲染出来
  // （champions.js 阶段视图的「选取率」），而不是让工单点名的字段少一个。
  assert.match(stageStruct, /`json:"pickRate,omitempty"`/, "阶段行必须下发 pickRate（工单 P0-6 实现要求第 1 条）");
  // 上游 0..1 的字段不得在后端 ×100，前端要自己换算（与父行同一条约定）。
  const converter = goFunctionSource(hexdataBackend, "hexdataAugmentStageMetricRows");
  assert.match(converter, /WinRate:\s+row\.WinRate \* 100/);
  // pickRate 与 winRate 同属百分数口径：后端 ×100 直出，前端不再换算。
  assert.match(converter, /PickRate:\s+row\.PickRate \* 100/, "阶段行 pickRate 必须按百分数口径 ×100");
  assert.match(converter, /StageBaselineWinRate: row\.StageBaselineWinRate \* 100/);
  assert.match(converter, /DeltaWinRate:\s+row\.DeltaWinRate,/);
  assert.match(converter, /WilsonLowerWinRate:\s+row\.WilsonLowerWinRate,/);
  // 评审整改 B4：阶段行的字母档位由后端算，映射只有 hexdataOfficialGrade 一份实现，
  // 前端不复制 hexTier/hexLabel → 字母的对照表。
  assert.match(converter, /Grade:\s+hexdataOfficialGrade\(row\.HexTier\)/);
  // 注：这里原本有一条 assert.doesNotMatch(converter, /PickRate/)（评审整改 B6 把阶段
  // 选取率裁掉时加的）。主控已按工单 P0-6「实现要求」第 1 条恢复下发，反向断言删除，
  // 正向断言见上面那条 `PickRate: row.PickRate * 100`。
  assert.match(converter, /if row\.Stage <= 0 \{\s+continue\s+\}/, "非法阶段号必须丢弃，不能当成阶段 0 渲染");
  assert.doesNotMatch(converter, /hexdataMinimumSample/, "阶段行不按最小样本过滤，低样本靠徽记披露");
  // sampleTier 对阶段行也要算，否则阶段视图里的低样本徽记会缺失。
  assert.match(goFunctionSource(hexdataBackend, "applyHexdataSampleTiers"), /rows\[index\]\.Stages\[stageIndex\]\.SampleTier = snapshot\.SamplePolicy\.sampleTier/);
  // 评审整改 B5：局内推荐 bundle 拷海克斯行时必须清掉 Stages（gameplay.js 对
  // stages 零消费，而这是延迟敏感链路）。
  assert.match(gameplayBackend, /result\.Augments = gameplayAugmentRowsWithoutStages\(detail\.RecommendedAugments\)/);
  assert.match(goFunctionSource(gameplayBackend, "gameplayAugmentRowsWithoutStages"), /row\.Stages = nil/);
  assert.equal((gameplayScript.match(/stages/g) || []).length, 0, "gameplay.js 一旦开始消费 stages，B5 的剥离就要重新评估");
});

test("R116-B 前端阶段助手按 stage 号取行，缺失返回 null 而不是 0 值", () => {
  const deps = {
    objectRows: (value) => (Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []),
    state: { mode: "aram-mayhem", mayhemStage: 3 },
  };
  const { mayhemAugmentStageRow, mayhemAugmentStageItem, mayhemActiveStage } = compile(["mayhemAugmentStageRow", "mayhemAugmentStageItem", "mayhemActiveStage", "normalizeMayhemStage"], deps);
  const item = { winRate: 67.9, games: 91113, score: 95.8, rarity: "prismatic", stages: [{ stage: 2, winRate: 63.05, games: 31196 }, { stage: 3, winRate: 61.66, games: 373, hexLabel: "顶级" }] };
  assert.equal(mayhemActiveStage(), 3);
  assert.equal(mayhemAugmentStageRow(item, 0), null, "汇总视图返回 null，调用方用父行数值");
  assert.equal(mayhemAugmentStageRow(item, 1), null, "缺阶段 1 时返回 null，绝不返回一个填 0 的对象");
  assert.deepEqual(mayhemAugmentStageRow(item, 3), { stage: 3, winRate: 61.66, games: 373, hexLabel: "顶级" });
  assert.equal(mayhemAugmentStageItem(item, 0), item);
  const staged = mayhemAugmentStageItem(item, 3);
  assert.equal(staged.winRate, 61.66);
  assert.equal(staged.games, 373);
  assert.equal(staged.hexLabel, "顶级");
  assert.equal(staged.score, 95.8, "阶段维度根本没有的字段（综合评分）继续沿用父行");
  assert.equal(staged.rarity, "prismatic");
  // 评审整改 A3：阶段维度拥有、但这一条阶段行没有的键，一律置 undefined，绝不静默
  // 沿用父行（英雄级汇总）的值。后端 championMetricStageRow 的展示字段全带
  // omitempty，Go 的零值不进 JSON → 「没有」在前端就是「键不存在」；无条件展开会让
  // 英雄级的 deltaWinRate/hexLabel/sampleTier 挂在「阶段 3」的 chip 下（口径混用）。
  for (const key of ["deltaWinRate", "wilsonLowerWinRate", "stageBaselineWinRate", "sampleTier", "grade", "pickRate"]) {
    assert.equal(staged[key], undefined, `阶段行缺 ${key} 时不许继承父行的值`);
    assert.equal(Object.hasOwn(staged, key), true, `${key} 必须是「显式 undefined」，否则 mayhemDeltaLabel 一类助手会读到父行值`);
  }
  // 父行确实带着这些值（证明上面挡住的不是「本来就没有」）。
  const parent = { ...item, deltaWinRate: 0.111527, hexLabel: "夯", sampleTier: "high", grade: "S", wilsonLowerWinRate: 0.675997 };
  const merged = mayhemAugmentStageItem({ ...parent, stages: [{ stage: 3, winRate: 61.66, games: 373 }] }, 3);
  assert.equal(merged.deltaWinRate, undefined, "英雄级的 +11.2% 不许出现在阶段 3 的卡上");
  assert.equal(merged.hexLabel, undefined);
  assert.equal(merged.grade, undefined);
  assert.equal(merged.sampleTier, undefined);
  assert.equal(merged.winRate, 61.66, "阶段行带着的键照常覆盖");
  // 键存在但值是 0（上游真的给了 0）与「键不存在」必须区分开：前者照抄，后者隐藏。
  const zeroed = mayhemAugmentStageItem({ ...parent, stages: [{ stage: 3, deltaWinRate: 0, winRate: 0, games: 0 }] }, 3);
  assert.equal(zeroed.deltaWinRate, 0);
  assert.equal(zeroed.winRate, 0);
  assert.equal(zeroed.games, 0);
  // 非法阶段号一律归零（回到汇总），不会渲染出一个不存在的「阶段 7」。
  for (const value of [7, -1, "x", null, undefined, 2.5]) {
    assert.equal(mayhemAugmentStageRow(item, value), null);
  }
});

// ---------------------------------------------------------------------------
// K. 独立评审整改（R116-B-REVIEW-FINDINGS.md 的 A1 / A3 / B4 / B6）
//    每条都对着评审给的「最现实触发路径」写，不是对着实现写。
// ---------------------------------------------------------------------------

test("R116-B 评审整改 A1 记住的阶段号在新英雄上没有阶段行时退回汇总，不锁死成空态", () => {
  const deps = {
    state: { mode: "aram-mayhem", mayhemStage: 3 },
    objectRows: (value) => (Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []),
    augmentMetaForAsset: () => null,
    augmentRarityKey: (value) => String(value || "unknown"),
    augmentGrade: () => "S",
    renderMayhemRecommendedAugment: (entry) => `<b>${entry.item.assets[0].name}</b>`,
    renderMayhemStageChips: () => "",
  };
  const { renderRecommendedAugments } = compile(["renderRecommendedAugments", "mayhemAugmentStageRow", "normalizeMayhemStage"], deps);
  // 主源熔断走 RSC 兜底时的形状：augment 行一条 stages 都没有（评审 A1 点名的路径）。
  const stageless = [
    { assets: [{ name: "海克斯A" }], rarity: "prismatic", winRate: 60.5 },
    { assets: [{ name: "海克斯B" }], rarity: "gold", winRate: 55.2 },
  ];
  const markup = renderRecommendedAugments(stageless, {});
  assert.doesNotMatch(markup, /这个阶段没有可展示的海克斯样本/, "不许留下没有出口的空态");
  assert.doesNotMatch(markup, /mayhem-inline-empty/);
  assert.match(markup, /海克斯A/);
  assert.match(markup, /海克斯B/);
  assert.equal(deps.state.mayhemStage, 0, "钳制必须写回状态，否则 chips 的 aria-pressed 与实际视图不一致");
  // 阶段行存在时钳制不介入（不能把 P0-6 的正常路径一起砍掉）。
  deps.state.mayhemStage = 3;
  const staged = [{ assets: [{ name: "海克斯C" }], rarity: "prismatic", stages: [{ stage: 3, winRate: 61.66 }] }];
  assert.match(renderRecommendedAugments(staged, {}), /海克斯C/);
  assert.equal(deps.state.mayhemStage, 3);
  // 只有部分行有该阶段时，仍然按阶段过滤（P0-6 判据，不能被钳制吃掉）。
  deps.state.mayhemStage = 3;
  const mixed = [
    { assets: [{ name: "海克斯D" }], rarity: "prismatic", stages: [{ stage: 3, winRate: 58 }] },
    { assets: [{ name: "海克斯E" }], rarity: "gold" },
  ];
  const filtered = renderRecommendedAugments(mixed, {});
  assert.match(filtered, /海克斯D/);
  assert.doesNotMatch(filtered, /海克斯E/, "缺阶段 3 的行仍然整条不渲染");
  assert.equal(deps.state.mayhemStage, 3);
  // 斗魂路径（mode !== "aram-mayhem"）恒为汇总，钳制不参与。
  deps.state = { mode: "arena", mayhemStage: 3 };
  assert.match(renderRecommendedAugments(stageless, {}), /海克斯A/);
  assert.equal(deps.state.mayhemStage, 3, "斗魂路径不该去改海斗的阶段状态");
});

test("R116-B 评审整改 A1 换英雄后阶段筛选回到汇总（真实点击链路）", async (t) => {
  // 三个英雄覆盖两条路径：
  //  157 → 四个阶段齐全，用户在这里点了「阶段 3」；
  //  222 → 同样有阶段数据：换英雄后必须回到汇总，而不是把「阶段 3」带过去
  //        （state 初始化处第 41-43 行的注释就是这么写的，但重置原来只在换模式时发生）；
  //  64  → 主源熔断走 RSC 兜底的形状：augment 行里连 stages 键都没有，chips 会整块
  //        消失，此时若还留着阶段号就会锁死成没有出口的空态（评审 A1 点名的路径）。
  const stagedDetail = detailFixture({
    recommendedAugments: [augmentRow(heroFixture.augments[4]), augmentRow(heroFixture.augments[5])],
  });
  const stagelessDetail = detailFixture({
    recommendedAugments: [augmentRow(heroFixture.augments[1]), augmentRow(heroFixture.augments[2])]
      .map(({ stages, ...rest }) => rest),
  });
  const rankings = {
    ...rankingsFixture,
    rows: [
      rankingsFixture.rows[0],
      { championId: 222, key: "jinx", name: "金克丝", rank: 2, tier: 2, play: 1200000, winRate: 52.14, pickRate: 8.2, tierLocallyCalculated: false },
      { championId: 64, key: "leesin", name: "李青", rank: 3, tier: 2, play: 900000, winRate: 51.02, pickRate: 11.4, tierLocallyCalculated: false },
    ],
  };
  const catalog = {
    ...catalogFixture,
    champions: [
      ...catalogFixture.champions,
      { id: 222, key: "jinx", slug: "jinx", nameZh: "金克丝", nameEn: "Jinx", titleZh: "金克丝", imageSource: "ddragon", imagePath: "/cdn/img/champion/jinx.png" },
      { id: 64, key: "leesin", slug: "leesin", nameZh: "李青", nameEn: "Lee Sin", titleZh: "李青", imageSource: "ddragon", imagePath: "/cdn/img/champion/leesin.png" },
    ],
  };
  const { window, document, calls } = await mountMayhemDetail(detailFixture(), {
    rankings,
    catalog,
    detailFor: (target) => (target.includes("champion=jinx") ? stagedDetail
      : target.includes("champion=leesin") ? stagelessDetail : detailFixture()),
  });
  t.after(() => window.close());
  const cards = () => [...document.querySelectorAll(".mayhem-recommend-grid .arena-option-card")];
  const section = () => document.querySelector("[data-mayhem-augments]");
  const activeChip = () => document.querySelector("[data-mayhem-stage].is-active")?.dataset.mayhemStage ?? null;

  // 英雄 157：点「阶段 3」（掷骰狂人阶段 3 = 61.66%）。
  document.querySelector('[data-mayhem-stage="3"]').click();
  assert.match(cards()[0].textContent, /61\.66%/, "阶段 3 的数值渲染出来了");
  assert.equal(activeChip(), "3");
  const before = calls.length;

  // 换到 222（男爵之手 汇总 61.41% / 阶段 3 80.00%，两个数字一眼可辨）。
  document.querySelector('[data-champion-row="222"]').click();
  await ticks(12);
  assert.equal(activeChip(), "0", "换英雄后必须回到英雄级汇总，不许把上一个英雄的阶段号带过去");
  assert.match(section().textContent, /男爵之手/, "渲染的是新英雄的数据");
  assert.match(cards()[0].textContent, /61\.41%/, "汇总视图用父行 pairWinRate 0.6141");
  assert.doesNotMatch(cards()[0].textContent, /80\.00%/, "不许沿用记住的阶段 3");
  assert.doesNotMatch(section().textContent, /阶段基准/, "汇总视图没有阶段基准这一格");
  assert.doesNotMatch(section().textContent, /61\.66%/, "不许残留上一个英雄的数值");

  // 在 222 上再点阶段 3：这条链路本身必须照常工作（重置不等于砍掉 P0-6）。
  document.querySelector('[data-mayhem-stage="3"]').click();
  assert.equal(activeChip(), "3");
  assert.match(cards()[0].textContent, /80\.00%/, "阶段 3 的极端值 0.8 → 80.00%");
  assert.match(cards()[0].textContent, /阶段基准57\.05%/);

  // 换到 64（一条阶段行都没有）：chips 整块消失，卡片区必须是英雄级汇总，
  // 而不是「0 个 + 这个阶段没有可展示的海克斯样本」这种没有出口的空态。
  document.querySelector('[data-champion-row="64"]').click();
  await ticks(12);
  assert.doesNotMatch(section().textContent, /这个阶段没有可展示的海克斯样本/, "换英雄后不许锁死成没有出口的空态");
  assert.doesNotMatch(section().textContent, /mayhem-inline-empty/);
  assert.equal(cards().length, 2, "两条 augment 都以英雄级汇总渲染");
  assert.match(section().textContent, /秘术冲拳/);
  assert.equal(document.querySelectorAll("[data-mayhem-stage]").length, 0, "没有阶段数据时 chips 整块不渲染");
  assert.doesNotMatch(section().textContent, /80\.00%|61\.66%/, "不许残留前两个英雄的数值");
  assert.ok(calls.length > before, "换英雄本来就要重新取详情（这不是阶段切换）");
});

test("R116-B 评审整改 A3 阶段行缺 deltaWinRate 时不渲染「较基准」，也不拿英雄级的值顶", async (t) => {
  const detail = detailFixture();
  const [first, ...rest] = detail.recommendedAugments;
  // 后端 championMetricStageRow 的字段全带 omitempty：上游某阶段不给 deltaWinRate
  // （或给了 0）→ Go 零值 → JSON 里根本没有这个键。
  const stages = first.stages.map((stage) => (stage.stage === 3 ? (({ deltaWinRate, ...kept }) => kept)(stage) : stage));
  detail.recommendedAugments = [{ ...first, stages }, ...rest];
  const { window, document } = await mountMayhemDetail(detail);
  t.after(() => window.close());
  const card = () => document.querySelector(".mayhem-recommend-grid .arena-option-card");
  // 汇总视图：英雄级 deltaWinRate 0.111527 → 「较基准 +11.2%」。
  assert.match(card().textContent, /较基准\+11\.2%/);
  document.querySelector('[data-mayhem-stage="3"]').click();
  const text = card().textContent;
  assert.match(text, /61\.66%/, "阶段 3 自己的胜率照常渲染");
  assert.doesNotMatch(text, /较基准/, "阶段行缺 deltaWinRate → 整格不渲染");
  assert.doesNotMatch(text, /\+11\.2%/, "更不许把英雄级的收益率挂在「阶段 3」的卡上");
  assert.doesNotMatch(text, /阶段基准/, "基准格依附于「较基准」，一起消失");
  assert.doesNotMatch(text, /undefined|NaN/);
  // 同一视图里其它 augment（阶段行字段齐全）照常显示较基准，不是整块降级。
  const others = [...document.querySelectorAll(".mayhem-recommend-grid .arena-option-card")].slice(1);
  assert.equal(others.length, 2, "另外两条 augment 仍然在阶段 3 视图里");
  for (const other of others) assert.match(other.textContent, /较基准/, "阶段行字段齐全的行照常显示较基准");
});

test("R116-B 评审整改 B4 阶段视图的徽章与官方档位同口径，综合评分标明英雄级", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const card = () => document.querySelector(".mayhem-recommend-grid .arena-option-card");
  const badge = () => card().querySelector(".augment-grade");
  // 汇总视图：父行 hexTier hang → 徽章 S，官方档位「夯」，综合评分是英雄级的 95.8。
  assert.equal(badge().textContent, "S");
  assert.match(badge().className, /is-S/);
  assert.match(card().textContent, /官方档位夯/);
  assert.match(card().textContent, /综合评分95\.8/);
  assert.doesNotMatch(card().textContent, /综合评分（英雄级）/);
  assert.doesNotMatch(card().textContent, /阶段基准/);
  document.querySelector('[data-mayhem-stage="3"]').click();
  // 阶段 3：hexTier top → 徽章 A + 官方档位「顶级」。整改前这里是徽章 S（父行 hang）
  // 配官方档位「顶级」（阶段 top），同一张卡两个官方档位表述互相矛盾。
  assert.equal(badge().textContent, "A", "徽章必须跟着阶段行走");
  assert.match(badge().className, /is-A/);
  assert.match(card().textContent, /官方档位顶级/);
  assert.doesNotMatch(card().textContent, /综合评分95\.8/, "英雄级评分必须带口径限定词");
  assert.match(card().textContent, /综合评分（英雄级）95\.8/);
  // B6：阶段基准可见，用户能自己验算 61.66% − 57.05% ≈ +4.6%。
  assert.match(card().textContent, /阶段基准57\.05%/);
  assert.match(card().textContent, /较基准\+4\.6%/);
  assert.doesNotMatch(card().textContent, /undefined|NaN/);
  document.querySelector('[data-mayhem-stage="0"]').click();
  assert.equal(badge().textContent, "S", "切回汇总恢复英雄级徽章");
  assert.match(card().textContent, /官方档位夯/);
});

test("R116-B 评审整改 B4 阶段行没有官方档位时字母徽章整块隐藏", () => {
  const state = { mode: "aram-mayhem", mayhemStage: 0 };
  const deps = {
    state,
    objectRows: (value) => (Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []),
    percent: (value) => (Number.isFinite(Number(value)) ? `${Number(value).toFixed(2)}%` : "—"),
    number: (value, digits = 1) => (Number.isFinite(Number(value)) ? Number(value).toFixed(digits) : "—"),
    compactNumber: (value) => String(value ?? "—"),
    escapeHTML: (value) => String(value ?? ""),
    arenaRarityKey: (value) => String(value || "unknown").toLowerCase(),
    renderAssetButton: (asset) => `<button>${asset?.name || ""}</button>`,
  };
  const { renderMayhemRecommendedAugment } = compile([
    "renderMayhemRecommendedAugment", "renderArenaOptionCard", "augmentGrade", "mayhemActiveStage",
    "mayhemAugmentStageItem", "mayhemAugmentStageRow", "normalizeMayhemStage",
    "mayhemAugmentConfidenceMetrics", "mayhemDeltaLabel", "mayhemDeltaTone", "mayhemSampleTierLabel", "mayhemWilsonLabel",
  ], deps);
  const entryFor = (stageRow) => ({
    item: {
      score: 95.8, winRate: 67.9, games: 91113, rarity: "prismatic", grade: "S", hexLabel: "夯",
      deltaWinRate: 0.111527, sampleTier: "high", assets: [{ id: 2095, name: "掷骰狂人" }],
      stages: [stageRow],
    },
    meta: { name: "掷骰狂人", rarity: "prismatic" },
    grade: "S",
  });
  const summary = renderMayhemRecommendedAugment(entryFor({ stage: 3, winRate: 61.66, games: 373, hexLabel: "样本过少", stageBaselineWinRate: 57.05, deltaWinRate: 0.046, sampleTier: "medium" }), 0);
  assert.match(summary, /class="augment-grade is-S">S<\/b>/, "汇总视图照旧用英雄级徽章");
  // 上游给 insufficient 时后端不给字母档位（实测英雄 157 是 4/499 条阶段行）。
  state.mayhemStage = 3;
  const insufficient = renderMayhemRecommendedAugment(entryFor({ stage: 3, winRate: 61.66, games: 373, hexLabel: "样本过少", stageBaselineWinRate: 57.05, deltaWinRate: 0.046, sampleTier: "medium" }), 0);
  assert.doesNotMatch(insufficient, /augment-grade/, "阶段行没有官方档位 → 字母徽章整块隐藏");
  assert.doesNotMatch(insufficient, />S</, "绝不拿英雄级的 S 去配阶段级的「样本过少」");
  assert.match(insufficient, /官方档位/);
  assert.match(insufficient, /样本过少/);
  assert.match(insufficient, /<dt>阶段基准<\/dt><dd>57\.05%<\/dd>/);
  assert.doesNotMatch(insufficient, /undefined|NaN/);
  // 阶段行带着官方档位时徽章就用它（后端 hexdataOfficialGrade 的唯一实现）。
  const graded = renderMayhemRecommendedAugment(entryFor({ stage: 3, winRate: 61.66, games: 373, hexLabel: "顶级", grade: "A", stageBaselineWinRate: 57.05, deltaWinRate: 0.046, sampleTier: "medium" }), 0);
  assert.match(graded, /class="augment-grade is-A">A<\/b>/);
  assert.match(graded, /<dt>官方档位<\/dt><dd>顶级<\/dd>/);
  // 斗魂路径（stage 恒为 0）不受影响。
  state.mode = "arena";
  state.mayhemStage = 3;
  assert.match(renderMayhemRecommendedAugment(entryFor({ stage: 3, hexLabel: "顶级", grade: "A" }), 0), /class="augment-grade is-S">S<\/b>/);
});

test("R116-B 评审整改 B6 阶段基准是一格可见数字，装备排行的较基准带口径 tooltip", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  document.querySelector('[data-mayhem-detail-tab="build"]').click();
  const row = [...document.querySelectorAll(".mayhem-ranking-list article")].find((article) => article.textContent.includes(upstreamItem.itemName));
  const delta = [...row.querySelectorAll("dl > div")].find((cell) => cell.querySelector("dt").textContent === "较基准");
  assert.match(delta.querySelector("dd").getAttribute("data-tooltip"), /较基准 = 这件装备的胜率/);
  assert.match(delta.querySelector("dd").getAttribute("data-tooltip"), /该英雄整体胜率/);
  assert.equal(delta.querySelector("dd").getAttribute("tabindex"), "0", "tooltip 必须键盘可达");
  assert.equal(delta.querySelector("dd").getAttribute("data-tooltip-size"), "compact");
  // 海克斯卡：卡片外壳 overflow:hidden 装不下 tooltip（P0-2 的 Wilson 下界同理），
  // 所以基准值用可见的一格给出。
  document.querySelector('[data-mayhem-detail-tab="overview"]').click();
  const card = () => document.querySelector(".mayhem-recommend-grid .arena-option-card");
  document.querySelector('[data-mayhem-stage="3"]').click();
  const cells = [...card().querySelectorAll("dl > div")].map((cell) => [cell.querySelector("dt").textContent, cell.querySelector("dd").textContent]);
  assert.deepEqual(cells.map(([label]) => label), ["胜率", "选取率", "样本", "综合评分（英雄级）", "较基准", "阶段基准", "官方档位"]);
  const byLabel = Object.fromEntries(cells);
  assert.equal(byLabel["胜率"], "61.66%");
  // 工单 P0-6「实现要求」第 1 条：阶段 chip 要重渲染 winRate/deltaWinRate/pickRate。
  // 上游 stage3 pickRate 0.000163 → 后端 ×100 = 0.0163 → percent() 两位小数 = 0.02%。
  assert.equal(byLabel["选取率"], "0.02%", "阶段视图必须渲染该阶段自己的 pickRate，不是父行的英雄级选取率");
  assert.equal(byLabel["较基准"], "+4.6%");
  assert.equal(byLabel["阶段基准"], "57.05%", "上游 stageBaselineWinRate 0.570537 → 百分数");
  assert.equal(byLabel["官方档位"], "顶级");
  assert.equal(card().getAttribute("data-metric-count"), "7", "阶段视图比汇总多「选取率」与「阶段基准」两格");
  assert.match(championsStyles, /\.arena-option-card\[data-metric-count="7"\] dl\s*\{[^}]*repeat\(3,minmax\(0,1fr\)\)/s, "七格时也要三列，不能退回五列");
  // 恢复阶段 pickRate 后，阶段视图最多 8 格（再叠一格「置信」或「95%下界」），
  // 这一档也必须有栅格规则，否则第 8 格会掉到默认的 auto 列宽上。
  assert.match(championsStyles, /\.arena-option-card\[data-metric-count="8"\] dl\s*\{[^}]*repeat\(3,minmax\(0,1fr\)\)/s, "八格时也要三列（工单 P0-6 要求阶段视图渲染 pickRate）");
});

test("R116-B 评审整改 A2 表现面板的「较平均」同样挡掉被舍入成 0 的小值", () => {
  const { mayhemPerformanceDelta } = compile(["mayhemPerformanceDelta"], {});
  // deltaPercent 已经是百分数单位（后端算好），一位小数；|value| < 0.05 会被抹成 0.0。
  for (const value of [0, -0, 0.02, -0.02, 0.049, -0.049, 1e-9]) {
    assert.equal(mayhemPerformanceDelta({ hasDelta: true, deltaPercent: value }), "", `deltaPercent=${value} 会渲染成「较平均 +0.0%」，必须挡掉`);
  }
  assert.equal(mayhemPerformanceDelta({ hasDelta: true, deltaPercent: 0.06 }), "+0.1%", "舍入后还有信息量的值要保留");
  assert.equal(mayhemPerformanceDelta({ hasDelta: true, deltaPercent: -0.06 }), "-0.1%");
  assert.equal(mayhemPerformanceDelta({ hasDelta: true, deltaPercent: 12.4 }), "+12.4%");
  // 既有的两条硬要求不能被这次整改带偏：只看 hasDelta，累计项永不带 ±%。
  assert.equal(mayhemPerformanceDelta({ hasDelta: false, deltaPercent: 12.4 }), "");
  assert.equal(mayhemPerformanceDelta({ deltaPercent: 12.4 }), "");
  assert.equal(mayhemPerformanceDelta({ hasDelta: true, cumulative: true, deltaPercent: 12.4 }), "");
  assert.equal(mayhemPerformanceDelta({ hasDelta: true, deltaPercent: "abc" }), "");
  assert.equal(mayhemPerformanceDelta(null), "");
  assert.equal(mayhemPerformanceDelta(undefined), "");
});

// ---------------------------------------------------------------------------
// L. 追加：清掉 R116-D 账本 §11.7 记下的两笔技术债（当初为避开与 R116-F 的并行
//    冲突而做的妥协，F 已收尾）
// ---------------------------------------------------------------------------

test("R116-B 评审整改 D-债 renderLiveInsights 的 helper 已提到模块作用域，typeof 护栏已删", () => {
  const helpers = ["liveRosterChampionName", "liveDeltaPoints", "liveConfidenceText", "liveEvidenceSuffix", "liveNoticeBar", "renderLiveRosterNoticeBars", "renderLiveTeamPortraitTags"];
  for (const name of helpers) {
    assert.match(gameplayScript, new RegExp(`^  function ${name}\\(`, "m"), `${name} 必须是模块作用域的函数声明，不是函数体内的箭头函数`);
  }
  const insights = functionSource(gameplayScript, "renderLiveInsights");
  for (const name of ["rosterChampionName", "deltaPoints", "confidenceText", "evidenceSuffix", "noticeBar", "renderRosterNoticeBars", "renderTeamPortraitTags"]) {
    assert.doesNotMatch(insights, new RegExp(`const ${name} =`), `${name} 还留在 renderLiveInsights 体内`);
  }
  assert.match(insights, /renderLiveRosterNoticeBars\(payload, players\)/);
  assert.match(insights, /renderLiveTeamPortraitTags\(payload\)/);
  // 提到模块作用域的直接收益：helper 能被单独编译、单独断言（原来只能整段带着
  // renderLiveInsights 一起编译）。
  const compiled = compile(["liveDeltaPoints", "liveConfidenceText", "liveEvidenceSuffix", "liveNoticeBar", "liveRosterChampionName"], { escapeHTML: (value) => String(value ?? "") }, gameplayScript);
  assert.equal(compiled.liveDeltaPoints(0.0346), "3.5");
  assert.equal(compiled.liveDeltaPoints(undefined), "0.0");
  assert.equal(compiled.liveConfidenceText({ confidenceLow: 0.01, confidenceHigh: 0.02 }), "上游置信区间 1.0%–2.0%");
  assert.equal(compiled.liveConfidenceText({}), "");
  assert.equal(compiled.liveConfidenceText({ confidenceLow: 0, confidenceHigh: 0 }), "");
  assert.equal(compiled.liveEvidenceSuffix({ evidence: "supported" }), "");
  assert.equal(compiled.liveEvidenceSuffix({}), "上游未标注证据等级");
  assert.match(compiled.liveEvidenceSuffix({ evidence: "anecdotal" }), /未达显著性/);
  assert.equal(compiled.liveRosterChampionName([{ championId: 157, championName: "疾风剑豪" }], 157), "疾风剑豪");
  assert.equal(compiled.liveRosterChampionName([{ championPickIntent: 157, championName: "疾风剑豪" }], 157), "疾风剑豪");
  assert.equal(compiled.liveRosterChampionName([{ championId: 157, championName: "疾风剑豪" }], 0), "");
  assert.equal(compiled.liveRosterChampionName(undefined, 157), "", "拿不到名单就返回空串，调用方据此整条丢弃");
  assert.match(compiled.liveNoticeBar("is-weak", "被 <b>卡牌大师</b> 克制", "上游置信区间 1.0%–2.0%"), /class="roster-notice is-weak"/);
  assert.doesNotMatch(compiled.liveNoticeBar("is-weak", "被克制", ""), /data-tooltip/, "没有置信区间就不给空 tooltip");
  // typeof 护栏删掉：漏注入依赖必须直接抛 ReferenceError，而不是静默降级把问题盖住。
  assert.doesNotMatch(gameplayScript, /typeof liveRecommendationRoster/);
  assert.match(functionSource(gameplayScript, "ensureLiveRecommendations"), /const roster = liveRecommendationRoster\(data\);/);
  // 降级行为本身没变（拿不到阵容就不发 phase / allyChampionIds / enemyChampionIds
  // 三个参数）：那条断言留在 r116d.test.cjs 的 "degrades to no roster params" 里，
  // 只是从「靠 typeof 护栏兜住漏注入」改成「显式注入返回 null」。
});
