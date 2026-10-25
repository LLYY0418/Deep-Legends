const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require(path.join(__dirname, "..", "..", "desktop", "node_modules", "jsdom"));

const read = (name) => fs.readFileSync(path.join(__dirname, name), "utf8");
const championsScript = fs.readFileSync(process.env.R116B_CHAMPIONS_SOURCE || path.join(__dirname, "champions.js"), "utf8");
const gameplayScript = read("gameplay.js");
const runtimeScript = read("runtime.js");
const gradeSource = runtimeScript.slice(runtimeScript.indexOf("  const GRADE_ORDER ="), runtimeScript.indexOf("  window.deepLegendsRuntime ="));
const sharedGrades = Function(`${gradeSource}\nreturn { gradeBadge, gradeRank };`)();
const indexHTML = read("index.html");
const championsStyles = read("champions.css");
const gameplayStyles = read("gameplay.css");
const championsBackend = fs.readFileSync(path.join(__dirname, "..", "champions.go"), "utf8");
const aramkitBackend = fs.readFileSync(path.join(__dirname, "..", "aramkit_detail.go"), "utf8");
const gameplayBackend = fs.readFileSync(path.join(__dirname, "..", "gameplay.go"), "utf8");
const realDetail = JSON.parse(fs.readFileSync(path.join(__dirname, "..", "testdata", "r307", "detail-response.json"), "utf8"));
const heroFixture = {augments:realDetail.recommendedAugments};
const upstreamAugment = heroFixture.augments[0];
const upstreamItem = realDetail.itemRanking[0];
const upstreamSpell = realDetail.build.summonerSpells[0];
function augmentRow(row, overrides={}) {return {...structuredClone(row),...overrides};}
function detailFixture(overrides={}) {const d=structuredClone(realDetail);d.recommendedAugments=d.recommendedAugments.slice(0,3);for(const k of ["starterItems","boots","summonerSpells"])d.build[k]=d.build[k].slice(0,1);return {...d,...overrides};}

const catalogFixture = {
  champions: [{ id: 157, key: "yasuo", slug: "yasuo", nameZh: "疾风剑豪", nameEn: "Yasuo", titleZh: "疾风剑豪", imageSource: "ddragon", imagePath: "/cdn/img/champion/yasuo.png" }],
  tiers: [],
};

const rankingsFixture = {
  mode: "aram-mayhem", region: "CN", source: "ARAMKit", patch: "16.21", fetchedAt: "2026-09-20T00:00:00Z",
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
      : target.startsWith("/api/champions/augments") ? { source: "ARAMKit", rows: [] }
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
  if(source.includes("R309 deferred history renderer: "+name)||source.includes("R309 deferred live renderer: "+name))source=require("./r277-deferred-fixture.cjs").join(source);
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
  dependencies = { recordLiveRecommendationRender: () => {}, ensureLiveRecommendationForRender: () => {}, ...dependencies };
  dependencies = { gradeBadge: sharedGrades.gradeBadge, gradeRank: sharedGrades.gradeRank, ...dependencies };
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
    assert.doesNotMatch(source.replace('<div class="aramkit-scope"><span>统计口径</span>', ""), /统计口径/);
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

test("R116-B 详情页两 tab 只渲染当前页内容，隐藏 tab 不占任何布局高度", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  const panels = () => [...document.querySelectorAll(".mayhem-detail-panel")];
  const tabs = [...document.querySelectorAll("[data-mayhem-detail-tab]")].map((button) => button.dataset.mayhemDetailTab);
  assert.deepEqual(tabs, ["overview", "build"]);

  // 概览：推荐海克斯 + 开局配置；构筑与表现的内容一个节点都不能在 DOM 里。
  assert.equal(panels().length, 1, "同一时刻只允许一个面板节点");
  assert.equal(panels()[0].dataset.mayhemDetailPanel, "overview");
  assert.ok(document.querySelector(".mayhem-augment-ranking"));
  assert.equal(document.querySelector(".mayhem-opening-config"), null);
  assert.equal(document.querySelector(".mayhem-item-ranking"), null);
  assert.equal(document.querySelector(".mayhem-performance"), null);
  assert.equal(document.querySelectorAll(".mayhem-detail-content [hidden]").length, 0, "详情面板里不许用 hidden 面板占位（R33 收藏页那类问题）");

  document.querySelector('[data-mayhem-detail-tab="build"]').click();
  assert.equal(panels().length, 1);
  assert.equal(panels()[0].dataset.mayhemDetailPanel, "build");
  assert.ok(document.querySelector(".aramkit-items"));
  assert.ok(document.querySelector(".aramkit-core"));
  assert.ok(document.querySelector(".mayhem-skill-plan"), "技能加点属于构筑 tab");
  assert.equal(document.querySelector(".mayhem-augment-ranking"), null);
  assert.ok(document.querySelector(".mayhem-opening-config"));
  assert.equal(document.querySelector(".mayhem-performance"), null);

  document.querySelector('[data-mayhem-detail-tab="overview"]').click();
  assert.equal(panels().length, 1);
  assert.equal(panels()[0].dataset.mayhemDetailPanel, "overview");
  assert.equal(document.querySelector('[data-mayhem-detail-tab="overview"]').getAttribute("aria-selected"), "true");
  assert.equal(document.querySelector('[data-mayhem-detail-tab="build"]').getAttribute("aria-selected"), "false");
});

test("R128 §2.3 详情页没有口径页脚，切页签时工具条节点不被重新挂载", async (t) => {
  const { window, document } = await mountMayhemDetail();
  t.after(() => window.close());
  assert.equal(document.querySelector("[data-mayhem-detail-footer]"), null, "页脚已删除");
  assert.doesNotMatch(document.querySelector(".mayhem-detail-content").textContent, /统计口径|上游公布的/);
  const toolbar = document.querySelector(".mayhem-detail-toolbar");
  assert.ok(toolbar, "工具条必须存在");
  for (const tab of ["build", "overview"]) {
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

// R309: historical case ID retained; the current assertion follows the new requirement.
test("R116-B 图鉴入口按钮在详情面板顶部，复用既有视图状态机而不是新路由", () => {
  const toolbar = functionSource(championsScript, "mayhemDetailToolbar");
  assert.doesNotMatch(toolbar, /mayhem-atlas-entry|data-mayhem-view="atlas"/);
  assert.doesNotMatch(toolbar, /离开当前英雄/);
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
  assert.match(championsScript, /class="mayhem-atlas-list-entry"/, "窄屏图鉴目录使用列表入口");
});

// ---------------------------------------------------------------------------
// C. P0-1：收益率 + 「出 vs 不出」对照（真实快照）
// ---------------------------------------------------------------------------

test("R307 装备排行保留真实胜率与场次，旧指标不制造空值", async(t)=>{
 const {window,document}=await mountMayhemDetail();t.after(()=>window.close());document.querySelector('[data-mayhem-detail-tab="build"]').click();
 const row=[...document.querySelectorAll('.aramkit-single')].find(n=>n.textContent.includes(upstreamItem.assets[0].name));assert.ok(row);
 const cells=Object.fromEntries([...row.querySelectorAll(':scope > div')].map(n=>[n.querySelector('small').textContent,n.firstChild.textContent]));
 assert.equal(cells['胜率'],upstreamItem.winRate.toFixed(2)+'%');assert.equal(cells['选取率'],upstreamItem.pickRate.toFixed(2)+'%');assert.doesNotMatch(row.textContent,/较基准|不出时|置信|undefined|NaN/);
});





// ---------------------------------------------------------------------------
// D. P0-2：样本置信度 + 排序护栏（R63 的老毛病）
// ---------------------------------------------------------------------------



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
  // R311 gives Arena a dedicated switch; Mayhem still preserves upstream order.
  assert.doesNotMatch(routes, /arenaOrderRows/);
  assert.doesNotMatch(recommended, /arenaOrderRows/);
  assert.match(functionSource(championsScript, "arenaOrderRows"), /window\.deepLegendsRuntime\.orderArenaRows/);
  assert.match(championsScript, /const sorted = arenaOrderRows\(filtered\);/);
  assert.match(runtimeScript, /mode === "tier" \? grade \|\| score \|\| games : games \|\| score \|\| grade/);
});



// ---------------------------------------------------------------------------
// E. P0-4：召唤师技能带胜率，出门装/鞋子保持只有选用率
// ---------------------------------------------------------------------------

test("R307 开局配置保留装备胜率场次和召唤师技能双速率", async(t)=>{
 const {window,document}=await mountMayhemDetail();t.after(()=>window.close());document.querySelector('[data-mayhem-detail-tab="build"]').click();const sections=[...document.querySelectorAll('.mayhem-opening-grid > section')];
 assert.deepEqual(sections.map(n=>n.querySelector('h4').textContent),['出门装','鞋子','召唤师技能']);
 const stats=n=>[...n.querySelectorAll('.config-option-list > article:first-child .option-stats dt')].map(x=>x.textContent);
 assert.deepEqual(stats(sections[0]),['胜率','场次']);assert.deepEqual(stats(sections[1]),['胜率','选取率']);assert.deepEqual(stats(sections[2]),['胜率','选取率']);
 assert.ok(sections[2].textContent.includes(upstreamSpell.winRate.toFixed(2)+'%'));assert.ok(sections[2].textContent.includes(upstreamSpell.pickRate.toFixed(2)+'%'));assert.doesNotMatch(sections[2].textContent,/undefined|NaN/);
});

test("R307 ARAMKit 装备指标与 RSC 三态开关均保留", () => {
  const opening = functionSource(championsScript, "renderMayhemOpeningConfiguration");
  assert.match(opening, /Number\(row.winRate\)>0/);
  assert.match(opening, /Number\(row.pickRate\)>0/);
  assert.match(opening, /group\("出门装",build.starterItems,"item",3,true\)/);assert.match(opening, /group\("鞋子",build.boots,"item",3\)/);assert.match(opening, /group\("召唤师技能",build.summonerSpells,"spell",2\)/);
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
      assetImage: (asset) => `<img alt="${asset?.name || ""}">`, number: String,
      renderAssetButton: (asset) => `<button>${asset?.name || ""}</button>`,
    },
  );
  const build = (spells) => ({
    starterItems: [{ assets: [{ name: "多兰之刃" }], pickRate: 41.2 }],
    boots: [{ assets: [{ name: "狂战士胫甲" }], pickRate: 55.5 }],
    summonerSpells: spells,
  });
  const fallback = renderMayhemOpeningConfiguration(build([{ assets: [{ id: 4, name: "闪现" }], pickRate: 90.78, winRate: 0, games: 2153718 }]), {});
  assert.match(fallback, /<dt>选取率<\/dt><dd>选取 90\.78%<\/dd>/);
  assert.doesNotMatch(fallback, /胜率|0\.00%/, "回退行没有胜率，就不许渲染胜率格");
  const withWinRate = renderMayhemOpeningConfiguration(build([{ assets: [{ id: 4, name: "闪现" }], pickRate: 90.78, winRate: 56.94, games: 2153718 }]), {});
  assert.match(withWinRate, /<dt>选取率<\/dt><dd>选取 90\.78%<\/dd>/);
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

test("R307 ARAMKit 官方字母徽章正常显示，不出现内部枚举",async(t)=>{
 const {window,document}=await mountMayhemDetail();t.after(()=>window.close());const card=document.querySelector('.mayhem-recommend-grid .arena-option-card');
 assert.match(card.innerHTML,/yourgg-s\.svg/);assert.doesNotMatch(card.textContent,/官方档位|hang|insufficient/);assert.match(document.querySelector('.mayhem-overview-metrics').innerHTML,/梯度[\s\S]*yourgg-s\.svg/);
});

test("R116-B P0-5 stats.grade 缺失 时梯度徽章整块隐藏，不用行级档位冒充", async (t) => {
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
  assert.match(functionSource(championsScript, "mayhemHeroTier"), /detail\?\.stats\?\.grade/);
  assert.match(functionSource(championsScript, "mayhemHeroTier"), /return grade \? \{ grade, locallyCalculated: false \} : null;/);
});

// ---------------------------------------------------------------------------
// G. P0-6：阶段筛选 chips —— 纯本地重渲染，不发请求
// ---------------------------------------------------------------------------

test("R307 切阶段只改真实数字与徽章不发请求，切回汇总恢复原值",async(t)=>{
 const {window,document,calls}=await mountMayhemDetail();t.after(()=>window.close());const chips=[...document.querySelectorAll('[data-mayhem-stage]')];assert.deepEqual(chips.map(n=>n.dataset.mayhemStage),['0','1','2','3','4']);
 const card=()=>document.querySelector('.mayhem-recommend-grid .arena-option-card');const summary=card().textContent;assert.ok(summary.includes(upstreamAugment.winRate.toFixed(2)+'%'));const before=calls.length;
 for(const stage of [1,3]){document.querySelector(`[data-mayhem-stage="${stage}"]`).click();const row=upstreamAugment.stages.find(r=>r.stage===stage);assert.equal(calls.length,before);assert.ok(card().textContent.includes(row.winRate.toFixed(2)+'%'));assert.ok(card().textContent.includes('选取率'+row.pickRate.toFixed(2)+'%'));assert.notEqual(card().textContent,summary);assert.match(document.querySelector(`[data-mayhem-stage="${stage}"]`).className,/is-active/);assert.ok(card().querySelector('.tier-badge').src.includes('yourgg-'+row.grade.toLowerCase()+'.svg'));}
 document.querySelector('[data-mayhem-stage="0"]').click();assert.equal(calls.length,before);assert.equal(card().textContent,summary);assert.doesNotMatch(summary,/选取率/);
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
    renderLaneMatchupCard: () => "",
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
// ---------------------------------------------------------------------------

test("R307 后端阶段单位与父行一致，实时推荐继续剥离阶段",()=>{
 assert.match(championsBackend,/Stages\s+\[\]championMetricStageRow `json:"stages,omitempty"`/);const a=championsBackend.indexOf('type championMetricStageRow struct {');const stage=championsBackend.slice(a,championsBackend.indexOf('\n}',a));
 for(const k of ['stage','winRate','pickRate','games','grade'])assert.ok(stage.includes('json:"'+k));
 assert.match(aramkitBackend,/WinRate:.*WinRate \* 100/);assert.match(aramkitBackend,/PickRate:.*PickRate \* 100/);
 assert.match(gameplayBackend,/result\.Augments = gameplayAugmentRowsWithoutStages\(detail\.RecommendedAugments\)/);assert.match(goFunctionSource(gameplayBackend,'gameplayAugmentRowsWithoutStages'),/row\.Stages = nil/);assert.equal((gameplayScript.match(/stages/g)||[]).length,0);
});

test("R307 阶段助手按号取行，缺失字段不继承父行，显式零值保留",()=>{
 const state={mode:'aram-mayhem',mayhemStage:3};const f=compile(['normalizeMayhemStage','mayhemActiveStage','mayhemAugmentStageRow','mayhemAugmentStageItem'],{state,objectRows:v=>Array.isArray(v)?v:[]});
 const item={...structuredClone(upstreamAugment),stages:[{stage:3,winRate:61.66,games:373}]};assert.equal(f.mayhemActiveStage(),3);assert.equal(f.mayhemAugmentStageRow(item,0),null);assert.equal(f.mayhemAugmentStageRow(item,1),null);assert.equal(f.mayhemAugmentStageItem(item,0),item);
 const row=f.mayhemAugmentStageItem(item,3);assert.equal(row.winRate,61.66);assert.equal(row.games,373);assert.equal(row.rarity,item.rarity);for(const key of ['pickRate','grade']){assert.equal(row[key],undefined);assert.equal(Object.hasOwn(row,key),true);}
 const zero=f.mayhemAugmentStageItem({...item,stages:[{stage:3,winRate:0,pickRate:0,games:0}]},3);for(const k of ['winRate','pickRate','games'])assert.equal(zero[k],0);
 for(const stage of [7,-1,'x',null,undefined,2.5])assert.equal(f.mayhemAugmentStageRow(item,stage),null);
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

test("R307 换英雄后阶段回汇总，RSC 无阶段行时不锁死（真实点击）",async(t)=>{
 const staged=detailFixture({recommendedAugments:[{...structuredClone(upstreamAugment),assets:[{...upstreamAugment.assets[0],name:'新英雄测试海克斯'}],winRate:51.23,stages:[{stage:3,winRate:80,pickRate:4,games:300,grade:'A'}]}]});const stageless=detailFixture({recommendedAugments:detailFixture().recommendedAugments.slice(1).map(({stages,...r})=>r)});
 const rankings={...rankingsFixture,rows:[rankingsFixture.rows[0],{championId:222,key:'jinx',name:'金克丝',rank:2},{championId:64,key:'leesin',name:'李青',rank:3}]};const catalog={...catalogFixture,champions:[...catalogFixture.champions,{id:222,key:'jinx',slug:'jinx',nameZh:'金克丝'},{id:64,key:'leesin',slug:'leesin',nameZh:'李青'}]};
 const {window,document,calls}=await mountMayhemDetail(detailFixture(),{rankings,catalog,detailFor:url=>url.includes('champion=jinx')?staged:url.includes('champion=leesin')?stageless:detailFixture()});t.after(()=>window.close());const card=()=>document.querySelector('.mayhem-recommend-grid .arena-option-card');const active=()=>document.querySelector('[data-mayhem-stage].is-active')?.dataset.mayhemStage;
 document.querySelector('[data-mayhem-stage="3"]').click();assert.equal(active(),'3');assert.ok(card().textContent.includes(upstreamAugment.stages[2].winRate.toFixed(2)+'%'));const before=calls.length;
 document.querySelector('[data-champion-row="222"]').click();await ticks(12);assert.equal(active(),'0');assert.match(card().textContent,/新英雄测试海克斯/);assert.match(card().textContent,/51\.23%/);assert.doesNotMatch(card().textContent,/80\.00%/);
 document.querySelector('[data-mayhem-stage="3"]').click();assert.equal(active(),'3');assert.match(card().textContent,/80\.00%/);
 document.querySelector('[data-champion-row="64"]').click();await ticks(12);assert.equal(document.querySelectorAll('[data-mayhem-stage]').length,0);assert.equal(document.querySelectorAll('.mayhem-recommend-grid .arena-option-card').length,2);const text=document.querySelector('[data-mayhem-augments]').textContent;assert.doesNotMatch(text,/这个阶段没有可展示|mayhem-inline-empty|80\.00%|51\.23%/);assert.match(text,/秘术冲拳/);assert.ok(calls.length>before);
});



test("R307 阶段徽章取阶段字母，切回汇总恢复父行徽章",async(t)=>{
 const detail=detailFixture();detail.recommendedAugments[0].stages[2].grade='A';const {window,document}=await mountMayhemDetail(detail);t.after(()=>window.close());const badge=()=>document.querySelector('.mayhem-recommend-grid .arena-option-card .tier-badge');assert.match(badge().src,/yourgg-s\.svg/);assert.equal(badge().alt,'梯度 S');document.querySelector('[data-mayhem-stage="3"]').click();assert.match(badge().src,/yourgg-a\.svg/);assert.equal(badge().alt,'梯度 A');assert.doesNotMatch(badge().closest('article').textContent,/官方档位|undefined|NaN/);document.querySelector('[data-mayhem-stage="0"]').click();assert.match(badge().src,/yourgg-s\.svg/);
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
  assert.match(summary, /class="tier-badge arena-option-grade"[^>]*yourgg-s\.svg/, "汇总视图使用英雄级徽章");
  // 上游给 insufficient 时后端不给字母档位（实测英雄 157 是 4/499 条阶段行）。
  state.mayhemStage = 3;
  const insufficient = renderMayhemRecommendedAugment(entryFor({ stage: 3, winRate: 61.66, games: 373, hexLabel: "样本过少", stageBaselineWinRate: 57.05, deltaWinRate: 0.046, sampleTier: "medium" }), 0);
  assert.doesNotMatch(insufficient, /tier-badge arena-option-grade/, "阶段行没有官方档位 → 字母徽章整块隐藏");
  assert.doesNotMatch(insufficient, />S</, "绝不拿英雄级的 S 去配阶段级的「样本过少」");
  assert.doesNotMatch(insufficient, /官方档位/);
  assert.doesNotMatch(insufficient, /undefined|NaN/);
  const graded = renderMayhemRecommendedAugment(entryFor({ stage: 3, winRate: 61.66, games: 373, hexLabel: "顶级", grade: "A", stageBaselineWinRate: 57.05, deltaWinRate: 0.046, sampleTier: "medium" }), 0);
  assert.match(graded, /class="tier-badge arena-option-grade"[^>]*yourgg-a\.svg/);
  assert.doesNotMatch(graded, /官方档位/);
  const noScore = entryFor({ stage: 3, winRate: 61.66, grade: "A" });
  noScore.item.score = 0;
  assert.doesNotMatch(renderMayhemRecommendedAugment(noScore, 0), /综合评分/, "缺少 hexScore 时不展示空评分格");
  // 斗魂路径（stage 恒为 0）不受影响。
  state.mode = "arena";
  state.mayhemStage = 3;
  assert.match(renderMayhemRecommendedAugment(entryFor({ stage: 3, hexLabel: "顶级", grade: "A" }), 0), /class="tier-badge arena-option-grade"[^>]*yourgg-s\.svg/);
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
