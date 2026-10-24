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
const realDetail=JSON.parse(fs.readFileSync(path.join(__dirname,'..','testdata','r307','detail-response.json'),'utf8'));
function detailFixture(overrides={}){return {...structuredClone(realDetail),...overrides};}

const catalogFixture = {
  champions: [{ id: 157, key: "yasuo", slug: "yasuo", nameZh: "疾风剑豪", nameEn: "Yasuo", titleZh: "疾风剑豪", imageSource: "ddragon", imagePath: "/cdn/img/champion/yasuo.png" }],
  tiers: [],
};

const rankingsFixture = {
  mode: "aram-mayhem", region: "CN", source: "ARAMKit", patch: "16.18", fetchedAt: "2026-09-20T00:00:00Z",
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
      : target.startsWith("/api/champions/augments") ? { source: "ARAMKit", rows: [] }
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
  assert.equal(section.querySelector("header p").textContent, "各品质前三项");
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

test("R307 概览顺序为海克斯推荐、组合、开局配置与英雄调整",async(t)=>{
 const {window,document}=await mountMayhemDetail(detailFixture({aramkit:{...realDetail.aramkit,balance:{modifiers:[{key:'abilityHaste',unit:'number',value:10}]}}}));t.after(()=>window.close());assert.equal(document.querySelector('.mayhem-opening-config'),null);const selectors=['.mayhem-augment-ranking','.aramkit-combos','.aramkit-balance'];const nodes=selectors.map(s=>document.querySelector(s));assert.ok(nodes.every(Boolean));for(let i=1;i<nodes.length;i++)assert.ok(nodes[i-1].compareDocumentPosition(nodes[i])&4);for(const node of nodes)assert.equal(node.closest('[data-mayhem-detail-panel="overview"]'),document.querySelector('[data-mayhem-detail-panel="overview"]'));
});



test("R128 详情页样式走既有变量，不引入 hex 字面值，废弃样式已清除", () => {
  const start = championsStyles.indexOf("/* ---- R128");
  assert.ok(start >= 0, "R128 的样式段不在 champions.css 里");
  const block = championsStyles.slice(start);
  assert.equal((block.match(/#[0-9A-Fa-f]{3,8}\b/g) || []).length, 0, "新增段不许出现 hex 字面值");
  // 三 tab 等分铺满，选中态一眼可辨；表现分组宽屏两列。
  assert.match(block, /\.mayhem-detail-tabs \{[^}]*--tab-count: 2;/);
  assert.match(block, /\.mayhem-detail-tabs \{[^}]*flex: 1 1 auto;/);
  assert.match(block, /\.mayhem-detail-tabs button \{[^}]*min-height: 42px;/);
  assert.match(block, /\.mayhem-detail-tabs button\.is-active \{[^}]*box-shadow: inset 0 -2px 0 var\(--primary\);/);
  assert.match(block, /@media \(max-width: 700px\) \{[\s\S]*\.mayhem-detail-tabs \{ flex: 1 0 100%; \}/);
  // 口径页脚、统计口径与慎选陈述的样式随文案一起删除。
  assert.doesNotMatch(championsStyles, /mayhem-caution-note|mayhem-measurement|mayhem-detail-footer/);
  // 新增类名必须在源码里被引用（R117 的未引用类名棘轮）。
  for (const name of ["mayhem-detail-tabs", "mayhem-detail-toolbar"]) {
    assert.ok(championsScript.includes(name), `${name} 在 CSS/JS 之间失去了引用`);
  }
});
