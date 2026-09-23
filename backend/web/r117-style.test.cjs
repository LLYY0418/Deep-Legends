const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const root = __dirname;
const cssFiles = fs.readdirSync(root).filter((name) => name.endsWith(".css"));
const cssText = cssFiles.map((name) => fs.readFileSync(path.join(root, name), "utf8")).join("\n");
const sourceFiles = fs.readdirSync(root).filter((name) => /\.(?:js|html)$/.test(name) && !name.endsWith(".test.cjs"));
const sourceText = sourceFiles.map((name) => fs.readFileSync(path.join(root, name), "utf8")).join("\n");
const hexText = `${cssText}\n${sourceText}`;
const classes = new Set([...cssText.matchAll(/\.([A-Za-z_][\w-]*)/g)].map((match) => match[1]));
const deadClasses = [
  "augment-toolbar", "brand-mark", "build-depth-sample-badge", "build-empty", "build-side-group",
  "champion-search-row", "current-game-unavailable", "facade-icon-badge", "final-build", "icon-label-button",
  "live-augment-grid", "live-orbit", "live-waiting", "mayhem-champion-board", "mayhem-skill-grid",
  "panel-actions", "pool-actions", "privacy-list", "recommended-augment-list", "sync-copy",
];
const tokenHexes = [
  "7E8896", "9AA7B8", "E3B341", "C77DFF", "5AA9FF", "FF8AC7", "B98255", "55C8D7",
  "F2CB5C", "D9A441", "DFE7EE", "9FB2C1", "DFA678", "B4794C", "F5D372", "E8C468",
  "DDE4EC", "AFBAC8", "C7D0DC", "DCA070", "B87333", "C8834A", "14100A",
];

function values(property) {
  return new Set([...cssText.matchAll(new RegExp(`${property}\\s*:\\s*([^;{}]+)`, "g"))].map((match) => match[1].trim()));
}

function declaredVariables() {
  return new Set([...cssText.matchAll(/--([\w-]+)\s*:/g)].map((match) => match[1]));
}

function referencedVariables() {
  const refs = new Set([...cssText.matchAll(/var\(--([\w-]+)/g)].map((match) => match[1]));
  const dynamic = new Set([...sourceText.matchAll(/setProperty\(\s*["']--([\w-]+)/g)].map((match) => match[1]));
  return { refs, dynamic };
}

test("R117 CSS variables resolve, including fallback references", () => {
  const declared = declaredVariables();
  const { refs, dynamic } = referencedVariables();
  const missing = [...refs].filter((name) => !declared.has(name) && !dynamic.has(name));
  assert.deepEqual(missing, [], `undefined CSS variables: ${missing.join(", ")}`);
});

test("R117 token colors have one source definition and no bypass drift", () => {
  for (const hex of tokenHexes) {
    const count = (hexText.match(new RegExp(`#${hex}`, "gi")) || []).length;
    assert.equal(count, 1, `expected #${hex} once across frontend sources, got ${count}`);
  }
  assert.equal((cssText.match(/#17130A/gi) || []).length, 0, "dark on-primary bypass remains");
  assert.equal((cssText.match(/#6F7885/gi) || []).length, 0, "rarity drift color remains");
});

test("R117 confirmed dead classes are absent while dynamic loot-table stays guarded", () => {
  for (const name of deadClasses) assert.equal(classes.has(name), false, `dead class remains: ${name}`);
  assert.equal(classes.has("loot-table"), true, "loot-table is retained for possible dynamic templates");
});

test("R117 CSS budgets do not exceed the audited baseline", () => {
  assert.ok(values("border-radius").size <= 33, "border-radius budget increased");
  assert.ok(values("padding").size <= 205, "padding budget increased");
  assert.ok(values("gap").size <= 54, "gap budget increased");
});

// P3-2 / P3-3 的固定白名单（23 个 hex、20 个类名）只能防旧问题复发，换一个白名单之外
// 但同类的新违规完全测不出来（独立验收追加的两条）。这里补全量扫描的预算棘轮：
// hex 扫 CSS 与非测试 JS/HTML，类名引用也扫同一批源码；动态类名仍按既有口径保留。
// 实测口径为 8 个 CSS + 14 个 JS/HTML 文件，重复 hex 种类数基线为 41，阈值写死以防预算漂移。
// 93 个未引用类名里大部分是 is-0/is-S/category-*/loot-* 这类字符串拼接出来的动态类名；棘轮只保证不再变差。
test("R117 repeated token hexes and unreferenced class names stay within the audited baseline", () => {
  const hexCounts = new Map();
  for (const match of hexText.matchAll(/#([0-9A-Fa-f]{6}|[0-9A-Fa-f]{3})\b/g)) {
    const hex = match[1].toUpperCase();
    hexCounts.set(hex, (hexCounts.get(hex) || 0) + 1);
  }
  const repeatedHexes = [...hexCounts.entries()].filter(([, count]) => count > 1);
  assert.ok(repeatedHexes.length <= 41, `hex literals defined in more than one place increased: ${repeatedHexes.length}`);

  const referenced = new Set([...sourceText.matchAll(/[A-Za-z_][\w-]*/g)].map((match) => match[0]));
  const unreferenced = [...classes].filter((name) => !referenced.has(name));
  assert.ok(unreferenced.length <= 93, `unreferenced CSS class names increased: ${unreferenced.length}`);

  // 白名单本身仍然要成立：token 色必须单一来源，已确认的死类名必须真的不在。
  for (const hex of tokenHexes) assert.equal(hexCounts.get(hex) || 0, 1, `#${hex} must have exactly one definition`);
  for (const name of deadClasses) assert.equal(classes.has(name), false, `dead class remains: ${name}`);
});

test("R117 duplicate declaration groups stay within the audited baseline", () => {
  const groups = new Map();
  for (const match of cssText.matchAll(/([^{}]+)\{([^{}]+)\}/g)) {
    const selector = match[1].replace(/\s+/g, " ").trim();
    const body = match[2].replace(/\s+/g, " ").trim();
    const key = `${selector}{${body}}`;
    if (body) groups.set(key, (groups.get(key) || 0) + 1);
  }
  const duplicateGroups = [...groups.values()].filter((count) => count > 1).length;
  assert.ok(duplicateGroups <= 45, `duplicate declaration groups increased: ${duplicateGroups}`);
});
