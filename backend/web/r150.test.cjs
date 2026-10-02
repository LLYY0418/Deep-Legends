const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const read = (name) => fs.readFileSync(process.env[`R150_${name.replace(/[^A-Za-z0-9]/g, "_").toUpperCase()}_SOURCE`] || path.join(__dirname, name), "utf8");
const script = read("champions.js");
const gameplay = read("gameplay.js");
const runtime = read("runtime.js");
const css = read("app.css");
const championCSS = read("champions.css");
const gradeSource = runtime.slice(runtime.indexOf("  const GRADE_ORDER ="), runtime.indexOf("  window.deepLegendsRuntime ="));
const grades = Function(`${gradeSource}\nreturn { gradeBadge, gradeRank };`)();

function functionSource(source, name) {
  const start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} missing`);
  const open = source.indexOf("{", start);
  let depth = 0, quote = "", escaped = false;
  for (let i = open; i < source.length; i += 1) {
    const char = source[i];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return source.slice(start, i + 1);
  }
  assert.fail(`${name} unbalanced`);
}

function compile(source, names, deps = {}) {
  const injected = { gradeBadge: grades.gradeBadge, gradeRank: grades.gradeRank, ...deps };
  return Function(...Object.keys(injected), `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn { ${names.join(",")} };`)(...Object.values(injected));
}

function renderArenaHeader(source = script, rowGrade = "OP") {
  const state = { arenaDetailLoading: true, detail: { arenaStats: { tier: 2, rank: 22, games: 505, winRate: 53.27, averagePlacement: 3.36, pickRate: 14.38 } }, arenaSort: "winRate" };
  const { renderArenaDetailPane } = compile(source, ["renderArenaDetailPane", "arenaOverviewMetric", "renderArenaSortBar"], {
    state,
    escapeHTML: String,
    championMeta: () => null,
    heroArtworkURL: () => "/hero.png",
    heroArtworkFallbackURL: () => "/fallback.png",
    imageURL: () => "/portrait.png",
    percent: (value) => `${Number(value).toFixed(2)}%`,
    number: (value) => Number(value).toFixed(2),
    compactNumber: String,
    renderDetailSkeleton: () => "",
  });
  return renderArenaDetailPane({ championId: 3, name: "正义巨像", rank: 1, grade: rowGrade, winRate: 55.96, averagePlacement: 3.26, firstPlaceRate: 22, banRate: 10.45, play: 3288 });
}

function checkHeader(source = script) {
  const markup = renderArenaHeader(source);
  for (const value of ["55.96%", "3.26", "总榜第 1", "3288", "10.45%", "/tier-icons/op.svg"]) assert.ok(markup.includes(value), `${value} missing`);
  for (const value of ["53.27%", "3.36", "总榜第 22", "505 场", "14.38%", "2 档", "/tier-icons/2.svg", "选用率"]) assert.ok(!markup.includes(value), `${value} leaked from OP.GG detail`);
  const s = renderArenaHeader(source, "S");
  assert.match(s, /yourgg-s\.svg/);
  assert.doesNotMatch(s, /\/tier-icons\/op\.svg/);
}

test("R150 Arena hero header uses the selected YOUR.GG list row, including OP versus S", () => {
  checkHeader();
  const mutated = script.replace('arenaOverviewMetric("胜率", percent(row.winRate)', 'arenaOverviewMetric("胜率", percent(state.detail.arenaStats.winRate)');
  assert.notEqual(mutated, script);
  assert.throws(() => checkHeader(mutated));
});

test("R150 one shared letter scale renders seven hexagons and no numeric badges", () => {
  const icons = ["op", "yourgg-s", "yourgg-a", "yourgg-b", "yourgg-c", "yourgg-d", "yourgg-f"];
  ["OP", "S", "A", "B", "C", "D", "F"].forEach((grade, index) => assert.match(grades.gradeBadge(grade), new RegExp(`/tier-icons/${icons[index]}\\.svg`)));
  for (const absent of ["", null, undefined, 1, "E", "?"]) assert.equal(grades.gradeBadge(absent), "");
  assert.equal((runtime.match(/const GRADE_ORDER =/g) || []).length, 1);
  assert.ok(grades.gradeRank("OP") < grades.gradeRank("S"));
  for (const source of [script, gameplay]) assert.doesNotMatch(source, /tierBadge\(|liveChampionTierBadge\(|tier-icons\/[1-5]\.svg|class="augment-grade/);
  assert.match(functionSource(script, "renderMayhemAtlasDetail"), /gradeBadge\(item\.grade, "atlas-detail-grade"\)/);
  assert.doesNotMatch(functionSource(script, "renderMayhemAtlasDetail"), /\$\{escapeHTML\(item\.grade\)\} 级/);
  const mutated = gradeSource.replace('grade === "OP" ? "op"', 'grade === "OP" || grade === "S" ? "op"');
  const wrong = Function(`${mutated}\nreturn gradeBadge;`)();
  assert.throws(() => assert.match(wrong("S"), /yourgg-s\.svg/));
});

function checkCard(source = script) {
  const { renderArenaOptionCard } = compile(source, ["renderArenaOptionCard"], {
    arenaRarityKey: () => "prismatic",
    renderAssetButton: () => "<button>海克斯</button>",
    percent: (value) => `${value}%`,
    number: String,
    compactNumber: String,
    escapeHTML: String,
  });
  const row = { grade: "S", winRate: 60, averagePlacement: 2.9, firstPlaceRate: 20, games: 1000, assets: [{ name: "海克斯" }] };
  const noScore = renderArenaOptionCard(row, "augment", 0);
  assert.match(noScore, /data-metric-count="4"/);
  assert.doesNotMatch(noScore, /综合评分|augment-grade|>—</);
  assert.match(noScore, /tier-badge arena-option-grade"[^>]*yourgg-s\.svg/);
  const official = renderArenaOptionCard({ ...row, score: 87.6 }, "prism", 0);
  assert.match(official, /综合评分[\s\S]*87\.6/);
  assert.match(official, /data-metric-count="5"/);
  const officialAugment = renderArenaOptionCard({ ...row, grade: "OP", score: 87.6 }, "augment", 0);
  assert.match(officialAugment, /综合评分[\s\S]*87\.6/);
  assert.match(officialAugment, /data-metric-count="5"/);
  assert.match(officialAugment, /tier-badge arena-option-grade"[^>]*tier-icons\/op\.svg/);
}

test("R150 local augment score stays hidden; official item score and hexagon remain", () => {
  checkCard();
  assert.match(script, /按档位与样本展示前九项/);
  assert.doesNotMatch(functionSource(script, "mayhemAugmentConfidenceMetrics"), /官方档位/);
  assert.match(championCSS, /\.arena-augment-section \.arena-option-card\[data-metric-count="4"\] dl \{ grid-template-columns: repeat\(2,minmax\(0,1fr\)\)/);
  const scoreMutation = script.replace("...(Number(row.score) > 0 ?", "...(true ?");
  assert.notEqual(scoreMutation, script);
  assert.throws(() => checkCard(scoreMutation));
  const squareMutation = script.replace('gradeBadge(row.grade, "arena-option-grade")', '`<b class="augment-grade is-S">S</b>`');
  assert.notEqual(squareMutation, script);
  assert.throws(() => checkCard(squareMutation));
});

test("R150 grade sorting uses letter then official score then samples", () => {
  const { sortedGradeRows, sortedArenaItemRows } = compile(script, ["sortedGradeRows", "sortedArenaItemRows"], { objectRows: (rows) => rows });
  const rows = [{ id: 1, grade: "S", score: 90, games: 1 }, { id: 2, grade: "OP", score: 1, games: 1 }, { id: 3, grade: "S", score: 90, games: 10 }, { id: 4, grade: "", score: 100, games: 100 }];
  for (const sort of [sortedGradeRows, sortedArenaItemRows]) assert.deepEqual(sort(rows).map((row) => row.id), [2, 3, 1, 4]);
  assert.deepEqual(rows.map((row) => row.id), [1, 2, 3, 4]);
  assert.doesNotMatch(functionSource(script, "renderAugmentGroups"), /Number\(item\.tier\)/);
});

function checkSidebar(source = css) {
  const expanded = Number(source.match(/--sidebar-width:\s*(\d+)px/)?.[1]);
  assert.ok(expanded > 70 && expanded <= 214 && expanded < 226);
  assert.match(source, /:root\[data-sidebar="collapsed"\] \{ --sidebar-width: 70px; \}/);
  const brand = source.match(/\.sidebar-brand \{[^}]*padding: 4px (\d+)px 13px;/)?.[1];
  const nav = source.match(/\.section-tab \{[^}]*padding: 0 (\d+)px;/)?.[1];
  assert.equal(brand, nav);
  assert.equal(Number(brand), 11);
  assert.match(source, /\.sidebar-brand::after \{[^}]*right: 11px;[^}]*left: 11px;/);
  assert.match(source, /\.sidebar-brand-copy strong \{[^}]*white-space: nowrap;/);
}

test("R150 sidebar stays at most 214px with aligned logo and collapsed width", () => {
  checkSidebar();
  assert.throws(() => checkSidebar(css.replace("--sidebar-width: 214px", "--sidebar-width: 226px")));
  assert.throws(() => checkSidebar(css.replace("padding: 4px 11px 13px", "padding: 4px 4px 13px")));
});
