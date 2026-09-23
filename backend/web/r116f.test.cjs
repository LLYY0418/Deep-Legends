// R116-F：P1「该英雄专属品质概率」的前端验证；P2「负向推荐（慎选）陈述」已被
// R128 §2.3-B 从 UI 删除，这里只保留「不再渲染」的反向钉子与后端判据契约。
//
// 三类断言：
//  1. 全应用 jsdom 冒烟——按 index.html 的真实加载顺序注入 runtime.js →
//     champions.js，用英雄 157 的真实数值（backend/testdata/r116/
//     hexdata-hero-157-stage-rarity.json 上核算出来的四阶段分布、37 条慎选命中）
//     驱动整条渲染与切 tab 链路。
//  2. 降级红线——字段缺失/为空/非法就整块隐藏，绝不出现 undefined / NaN% / 0.00%。
//  3. 契约钉子——P1 不替换全服分布面板、不为它多发请求、前端不自己重算分布。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require(path.join(__dirname, "..", "..", "desktop", "node_modules", "jsdom"));

const read = (name) => fs.readFileSync(path.join(__dirname, name), "utf8");
const championsScript = read("champions.js");
const championsStyles = read("champions.css");
const runtimeScript = read("runtime.js");
const indexHTML = read("index.html");
const hexdataBackend = fs.readFileSync(path.join(__dirname, "..", "hexdata.go"), "utf8");
const championsBackend = fs.readFileSync(path.join(__dirname, "..", "champions.go"), "utf8");

// ---------------------------------------------------------------------------
// 夹具：数值全部来自英雄 157 的真实上游响应（2026-09-20 抓取）
// ---------------------------------------------------------------------------

// 四阶段 × 三稀有度：后端把上游 0..1 的 stages[].pickRate 分组求和后 ×100 下发。
// 阶段 1 只有 121 条 augment 有 stage 行（不是 126），total 是实测的 1.000003×100。
const heroStageRarityFixture = [
  { stage: 1, silver: 8.1226, gold: 44.9009, prismatic: 46.9768, augments: 121, total: 100.0003 },
  { stage: 2, silver: 28.6254, gold: 43.7865, prismatic: 27.5884, augments: 126, total: 100.0003 },
  { stage: 3, silver: 28.8998, gold: 43.9486, prismatic: 27.1517, augments: 126, total: 100.0001 },
  { stage: 4, silver: 29.0356, gold: 43.7424, prismatic: 27.2221, augments: 126, total: 100.0001 },
];

function augmentRow(id, name, rarity, overrides = {}) {
  return {
    assets: [{ id, kind: "augment", name, description: "真实描述", source: "hexdata", path: `/assets/augments/${id}.png` }],
    rarity, grade: "S", score: 90 - id, winRate: 57.4, pickRate: 3.8, games: 91113,
    deltaWinRate: 0.111527, wilsonLowerWinRate: 0.675997, hexLabel: "夯", sampleTier: "high",
    ...overrides,
  };
}

// 七条命中行：真实数据里英雄 157 有 37 条命中，而展示出来的只有每品质前三张卡
// （9 张），命中的那 37 条一条都不在这 9 张里。这里刻意复现这个形状：
// 慎选陈述必须读未裁剪的全量行，否则整块提示在真实数据上永远不会出现。
function cautionRows() {
  const rows = [
    augmentRow(1, "掷骰狂人", "prismatic"),
    augmentRow(2, "秘术冲拳", "prismatic"),
    augmentRow(3, "魔法转物理", "silver"),
    augmentRow(4, "灵魂虹吸", "gold"),
    augmentRow(5, "男爵之手", "prismatic"),
    augmentRow(6, "会心防御", "silver"),
    augmentRow(7, "暴击飞弹", "gold"),
  ];
  const flagged = [
    ["升级：无尽之刃", "gold", 19.77, 2.346, -0.0005],
    ["关键暴击", "gold", 7.47, 2.346, -0.0002],
    ["虹吸", "silver", 8.78, 1.42015, -0.0082],
    ["战争交响乐", "prismatic", 5.72, 1.619, -0.0143],
    ["质变：黄金阶", "silver", 3.42, 1.42015, -0.0134],
    ["最终形态", "prismatic", 3.76, 1.619, -0.0097],
    ["罪恶快感", "gold", 5.43, 2.346, -0.0068],
  ];
  return [
    ...rows,
    ...flagged.map(([name, rarity, pickRate, median, delta], index) => augmentRow(
      100 + index, name, rarity,
      { pickRate, deltaWinRate: delta, caution: true, cautionMedianPickRate: median },
    )),
  ];
}

function detailFixture(overrides = {}) {
  return {
    mode: "hextech-aram", region: "CN", source: "Hexdata + OP.GG RSC", patch: "16.18",
    measurementTechnique: "上游公布的统计口径：样本为国服七区海克斯大乱斗，置信区间为 wilson_95",
    citation: { patch: "16.18", reportDate: "2026-09-18", buildId: "hexdata-test" },
    stats: { tier: 2, winRate: 56.7509 },
    recommendedAugments: cautionRows(),
    itemRanking: [augmentRow(9001, "毁坏仪式", "", { winRate: 61.63, games: 1406649 })],
    build: {
      starterItems: [{ assets: [{ id: 1055, kind: "item", name: "多兰之刃", source: "hexdata" }], pickRate: 41.2, games: 120000 }],
      boots: [{ assets: [{ id: 3006, kind: "item", name: "狂战士胫甲", source: "hexdata" }], pickRate: 55.5, games: 120000 }],
      summonerSpells: [],
      skills: [{ skillPriority: ["Q", "E", "W"], skillOrder: ["Q", "E", "W"], winRate: 60.1, pickRate: 70.5, games: 90000 }],
      coreItems: [],
    },
    heroStageRarity: heroStageRarityFixture,
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

const ticks = (count = 12) => new Promise((resolve) => {
  let left = count;
  const step = () => { if (--left <= 0) resolve(); else setImmediate(step); };
  setImmediate(step);
});

async function mountMayhemDetail(detail = detailFixture()) {
  const dom = new JSDOM(indexHTML, { url: "http://localhost/", runScripts: "outside-only", pretendToBeVisual: true });
  const window = dom.window;
  const calls = [];
  window.fetch = async (url) => {
    const target = String(url);
    calls.push(target);
    const payload = target.startsWith("/api/champions/catalog") ? catalogFixture
      : target.startsWith("/api/champions/rankings") ? rankingsFixture
      : target.startsWith("/api/champions/detail") ? detail
      : target.startsWith("/api/champions/augments") ? { source: "Hexdata", rows: [] }
      : {};
    return { ok: true, status: 200, json: async () => payload, text: async () => "" };
  };
  window.localStorage.setItem("lol-loot-champion-mode", "aram-mayhem");
  window.eval(runtimeScript);
  window.eval(championsScript);
  window.dispatchEvent(new window.CustomEvent("deep-legends:section", { detail: { name: "champions" } }));
  await ticks();
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

function compile(names, dependencies = {}, source = championsScript) {
  const keys = Object.keys(dependencies);
  return Function(...keys, `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn {${names.join(",")}};`)(...keys.map((key) => dependencies[key]));
}

// ---------------------------------------------------------------------------
// A. P1：该英雄专属品质概率（英雄口径）
// ---------------------------------------------------------------------------

test("R116-F P1 概览 tab 渲染该英雄专属的四阶段品质概率，数值与真实分布一致", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const panel = document.querySelector(".mayhem-hero-rarity");
  assert.ok(panel, "英雄专属品质概率面板没有渲染");
  assert.equal(panel.closest('[data-mayhem-detail-panel="overview"]'), panel.closest(".mayhem-detail-panel"), "面板必须在概览 tab 内");
  assert.match(panel.querySelector("h3").textContent, /该英雄专属品质概率/);
  // R128 §2.3-B：副标题缩成一句事实描述，不再指向全服口径面板。
  assert.equal(panel.querySelector("header p").textContent, "各阶段白银 / 黄金 / 棱彩选取概率");
  const stages = [...panel.querySelectorAll(".mayhem-rarity-stages article")];
  assert.equal(stages.length, 4, "四个阶段各一列");
  // 阶段 1 的真实值：白银 8.12% / 黄金 44.90% / 棱彩 46.98%（全服口径是 10.6 / 58.5 / 30.9）。
  const first = stages[0];
  assert.match(first.querySelector("b").textContent, /第 1 阶段/);
  assert.equal(first.querySelector(".is-silver").textContent, "白银 8.12%");
  assert.equal(first.querySelector(".is-gold").textContent, "黄金 44.90%");
  assert.equal(first.querySelector(".is-prismatic").textContent, "棱彩 46.98%");
  // 分母披露：阶段 1 只有 121 条有 stage 行，缺失的 5 条是跳过的、不是按 0 计入的。
  assert.match(first.querySelector("small").textContent, /121 项海克斯/);
  assert.match(first.querySelector("small").textContent, /三项合计 100\.00%/);
  assert.equal(stages[3].querySelector(".is-prismatic").textContent, "棱彩 27.22%");
  assert.match(stages[3].querySelector("small").textContent, /126 项海克斯/);
  for (const stage of stages) {
    assert.doesNotMatch(stage.textContent, /undefined|NaN|null/, "任何一格都不许出现未换算或缺失的值");
  }
  // R128 §2.3：口径页脚已删除，详情页不再有跨 tab 常驻的说明节点。
  assert.equal(document.querySelector("[data-mayhem-detail-footer]"), null);
});

test("R116-F P1 分布缺失/为空/非法时整块隐藏，不留空壳也不显示 0.00%", async (t) => {
  for (const heroStageRarity of [undefined, null, [], [{ stage: 0, silver: 0, gold: 0, prismatic: 0, augments: 0, total: 0 }], [{ stage: 1, silver: 0, gold: 0, prismatic: 0, total: 0 }], "nope", [{}]]) {
    const { window, document } = await mountMayhemDetail(detailFixture({ heroStageRarity }));
    try {
      assert.equal(document.querySelector(".mayhem-hero-rarity"), null, `heroStageRarity=${JSON.stringify(heroStageRarity)} 时不该渲染面板`);
      assert.equal(document.querySelector(".mayhem-rarity-stages"), null, "全服口径那套栅格也不该被英雄面板拉起来");
      // 面板隐藏时其余内容照常渲染（降级只影响自己那一段）。
      assert.ok(document.querySelector(".mayhem-augment-ranking"), "海克斯推荐卡不受影响");
    } finally {
      window.close();
    }
  }
});

test("R116-F P1 只渲染合法阶段，缺组的阶段不冒充 0", async (t) => {
  const rows = [
    { stage: 2, silver: 28.6254, gold: 43.7865, prismatic: 27.5884, augments: 126, total: 100.0003 },
    { stage: -1, silver: 10, gold: 10, prismatic: 10, augments: 3, total: 30 },
    { stage: 1, silver: null, gold: null, prismatic: null, augments: 0, total: 0 },
  ];
  const { window, document } = await mountMayhemDetail(detailFixture({ heroStageRarity: rows }));
  t.after(() => window.close());
  const stages = [...document.querySelectorAll(".mayhem-hero-rarity .mayhem-rarity-stages article")];
  assert.equal(stages.length, 1, "非法阶段号与三组全空的阶段都不渲染");
  assert.match(stages[0].querySelector("b").textContent, /第 2 阶段/);
});

test("R116-F P1 保留全服口径面板：英雄面板不替换它、不复用它的类名、不多发请求", async (t) => {
  // 全服口径那一份原样留在海克斯图鉴里（工单实现要求第 2 条：不要替换）。
  assert.match(championsScript, /function renderMayhemRarityPanel\(\)/);
  assert.match(functionSource(championsScript, "renderMayhemAtlas"), /\$\{renderMayhemRarityPanel\(\)\}/);
  assert.match(championsScript, /class="augment-directory mayhem-rarity-panel"/);
  // 英雄面板用的是自己的类名：.mayhem-rarity-panel 是「该去拉全服分布了」的触发器
  // （champions.js 里按这个类名判断是否 loadMayhemRarity），英雄面板挂上它就会
  // 在详情页多打一条 /api/champions/augment-rarity。
  const heroPanel = functionSource(championsScript, "renderMayhemHeroRarityPanel");
  assert.match(heroPanel, /class="recommendation-section mayhem-hero-rarity"/);
  assert.doesNotMatch(heroPanel, /mayhem-rarity-panel/);
  const { window, document, calls } = await mountMayhemDetail();
  t.after(() => window.close());
  assert.ok(document.querySelector(".mayhem-hero-rarity"));
  assert.equal(calls.filter((url) => url.includes("augment-rarity")).length, 0, "英雄口径是纯本地聚合，不许为它多发请求");
  const before = calls.length;
  await ticks(4);
  assert.equal(calls.length, before, "渲染完成后不该再有补充请求");
  // 前端不自己重算分布：面板只消费后端下发的百分数，碰都不碰 pickRate。
  assert.doesNotMatch(heroPanel, /pickRate/);
  assert.doesNotMatch(functionSource(championsScript, "mayhemHeroRarityStage"), /pickRate/);
  // Anti-scope 第 1 条：不做概率转移矩阵、不做四阶段组合枚举。
  assert.doesNotMatch(championsScript, /转移矩阵|组合枚举|transitionMatrix/i);
});

test("R116-F P1 英雄面板随 tab 局部替换进出", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  assert.ok(document.querySelector(".mayhem-hero-rarity"));
  document.querySelector('[data-mayhem-detail-tab="build"]').click();
  assert.equal(document.querySelector(".mayhem-hero-rarity"), null, "英雄面板只属于概览 tab");
  document.querySelector('[data-mayhem-detail-tab="overview"]').click();
  assert.ok(document.querySelector(".mayhem-hero-rarity"), "切回概览要恢复");
});

// ---------------------------------------------------------------------------
// B. R128 §2.3-B / §2.4：慎选陈述已删除，推荐卡标题行与概览顺序
// ---------------------------------------------------------------------------

test("R128 §2.3-B 慎选陈述不再渲染，海克斯推荐卡下方没有方法论说明", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  assert.equal(document.querySelector(".mayhem-caution-note"), null);
  const section = document.querySelector(".mayhem-augment-ranking");
  assert.ok(section, "海克斯推荐卡本身保留");
  assert.doesNotMatch(section.textContent, /数据显示|仅描述赛后关联|不构成因果结论|按官方推荐顺序/);
  // 推荐卡里只剩标题与那一句副标题，没有多出来的说明段。
  assert.equal(section.querySelector("header p").textContent, "按综合评分、胜率与样本展示各品质前三项");
  assert.equal(section.querySelectorAll("p").length, 1, "推荐卡里不许再有第二段说明文字");
  // 前端不再保留慎选陈述的渲染函数与上限常量，样式也一并删除。
  assert.doesNotMatch(championsScript, /mayhemCautionNote|mayhemCautionItem|MAYHEM_CAUTION_LIMIT/);
  assert.doesNotMatch(championsStyles, /mayhem-caution-note/);
});

test("R128 §2.4 海克斯推荐标题行右侧不再有「N 个」计数", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const header = document.querySelector(".mayhem-augment-ranking > header");
  assert.ok(header);
  assert.equal(header.querySelector(".section-count"), null, "标题行不再显示「N 个」");
  assert.doesNotMatch(header.innerHTML, /section-count/);
  // 源码层面：阶段 chips 之后直接收尾，不再拼计数 span。
  const source = functionSource(championsScript, "renderRecommendedAugments");
  assert.match(source, /\$\{chips\}<\/header>/);
  assert.doesNotMatch(source, /entries\.length\} 个/);
});

test("R128 §2.4 概览顺序为海克斯推荐 → 开局配置 → 该英雄专属品质概率", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const panel = document.querySelector('[data-mayhem-detail-panel="overview"]');
  const augments = document.querySelector(".mayhem-augment-ranking");
  const opening = document.querySelector(".mayhem-opening-config");
  const rarity = document.querySelector(".mayhem-hero-rarity");
  assert.ok(augments && opening && rarity, "概览三段都必须渲染");
  const follows = (before, after) => Boolean(before.compareDocumentPosition(after) & 4);
  assert.ok(follows(augments, opening), "开局配置必须排在海克斯推荐之后");
  assert.ok(follows(opening, rarity), "该英雄专属品质概率必须排在开局配置之后");
  for (const node of [augments, opening, rarity]) assert.equal(node.closest('[data-mayhem-detail-panel="overview"]'), panel, "三段都属于概览 tab");
});

test("R116-F P2 判据仍在后端：caution 与中位数由后端下发（前端不再渲染陈述）", () => {
  // R128 §2.3-B：前端的 mayhemCautionNote / mayhemCautionItem 已删除；判据本体与
  // 下发字段保留在后端，界面上不再出现这段方法论陈述。
  assert.doesNotMatch(championsScript, /mayhemCautionNote|mayhemCautionItem|MAYHEM_CAUTION_LIMIT/);
  // 判据本体在后端，且三条都在（工单 P2 实现要求第 1 条）。
  assert.match(hexdataBackend, /func markMayhemNegativeRecommendations\(rows \[\]championMetricRow\) int/);
  const criteria = hexdataBackend.slice(hexdataBackend.indexOf("func markMayhemNegativeRecommendations("));
  assert.match(criteria, /row\.PickRate <= median \|\| row\.DeltaWinRate >= 0 \|\| row\.SampleTier == "low"/);
  assert.match(hexdataBackend, /func mayhemPickRateMedianByRarity\(rows \[\]championMetricRow\) map\[string\]float64/);
  // sampleTier 复用 R116-B 的分档，判据函数里没有第二套阈值。
  assert.doesNotMatch(criteria.slice(0, criteria.indexOf("\n}\n")), /250|1000/);
  // 下发字段：caution 与中位数，都是 omitempty（没有命中就不占体积）。
  assert.match(championsBackend, /Caution\s+bool\s+`json:"caution,omitempty"`/);
  assert.match(championsBackend, /CautionMedianPickRate float64 `json:"cautionMedianPickRate,omitempty"`/);
  assert.match(championsBackend, /HeroStageRarity \[\]hexdataHeroStageRarityRow `json:"heroStageRarity,omitempty"`/);
  // P1 的聚合函数签名与工单逐字一致，且只用 stages[].pickRate。
  assert.match(hexdataBackend, /func computeHeroStageRarityDistribution\(augments \[\]hexdataAugmentRowV2\) map\[int\]map\[string\]float64/);
  const distribution = hexdataBackend.slice(hexdataBackend.indexOf("func computeHeroStageRarityDistribution("), hexdataBackend.indexOf("func hexdataHeroStageRarityRows("));
  assert.match(distribution, /groups\[rarity\] \+= stage\.PickRate/);
  assert.doesNotMatch(distribution, /augment\.PickRate/, "绝不能拿 augment 顶层的 pickRate 分组（实测求和 3.79）");
  assert.match(distribution, /if stage\.Stage <= 0 \{\s+continue\s+\}/, "非法阶段号必须跳过");
});

test("R128 详情页样式走既有变量，不引入 hex 字面值，废弃样式已清除", () => {
  const start = championsStyles.indexOf("/* ---- R128");
  assert.ok(start >= 0, "R128 的样式段不在 champions.css 里");
  const block = championsStyles.slice(start);
  assert.equal((block.match(/#[0-9A-Fa-f]{3,8}\b/g) || []).length, 0, "新增段不许出现 hex 字面值");
  // 三 tab 等分铺满，选中态一眼可辨；表现分组宽屏两列。
  assert.match(block, /\.mayhem-detail-tabs \{[^}]*--tab-count: 3;/);
  assert.match(block, /\.mayhem-detail-tabs \{[^}]*flex: 1 1 auto;/);
  assert.match(block, /\.mayhem-detail-tabs button \{[^}]*min-height: 42px;/);
  assert.match(block, /\.mayhem-detail-tabs button\.is-active \{[^}]*box-shadow: inset 0 -2px 0 var\(--primary\);/);
  assert.match(block, /\.mayhem-performance-groups \{[^}]*repeat\(2,minmax\(0,1fr\)\)/);
  assert.match(block, /\.mayhem-performance-group dl > div \{[^}]*grid-template-columns: minmax\(0,1fr\) auto;/);
  assert.match(block, /@media \(max-width: 700px\) \{[\s\S]*\.mayhem-detail-tabs \{ flex: 1 0 100%; \}/);
  assert.match(block, /@container mayhem-pane \(max-width: 760px\) \{\s*\.mayhem-performance-groups \{ grid-template-columns: 1fr; \}/);
  // 口径页脚、统计口径与慎选陈述的样式随文案一起删除。
  assert.doesNotMatch(championsStyles, /mayhem-caution-note|mayhem-measurement|mayhem-detail-footer/);
  // 新增类名必须在源码里被引用（R117 的未引用类名棘轮）。
  for (const name of ["mayhem-hero-rarity", "mayhem-performance-note", "mayhem-multikill-values"]) {
    assert.ok(championsScript.includes(name), `${name} 在 CSS/JS 之间失去了引用`);
  }
});
