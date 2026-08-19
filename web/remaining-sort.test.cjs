const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const source = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");

function functionSource(name) {
  const marker = `function ${name}(`;
  const start = source.indexOf(marker);
  assert.notEqual(start, -1, `${name} must exist in app.js`);
  const bodyStart = source.indexOf("{", start);
  let depth = 0;
  for (let index = bodyStart; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    else if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) return source.slice(start, index + 1);
    }
  }
  throw new Error(`${name} has an unterminated body`);
}

const context = {
  compareRarity: (left, right) => left.rank - right.rank,
  rarityGroup: (skin) => skin.group || "unranked",
  releaseTime: () => 0,
  localeCompare: (left, right) => String(left).localeCompare(String(right), "zh-CN"),
};
vm.createContext(context);
vm.runInContext(`${functionSource("poolRarityGroup")}; ${functionSource("comparePoolRarity")}; ${functionSource("resolvedTheme")}; ${functionSource("defaultDescending")}; ${functionSource("chromaDisplayName")}; ${functionSource("chromaVisibilityCounts")};`, context);

test("青年瑞兹与海克斯科技安妮在奖池中归入限定", () => {
  assert.equal(context.poolRarityGroup({ id: 13001, group: "unranked" }), "limited");
  assert.equal(context.poolRarityGroup({ id: 1010, group: "mythic" }), "limited");
});

test("奖池限定排在传说之前，其余按奖池品质顺序比较", () => {
  const limited = { id: 2001, name: "限定", group: "limited" };
  const legendary = { id: 2002, name: "传说", group: "legendary" };
  const epic = { id: 2003, name: "史诗", group: "epic" };
  assert.ok(context.comparePoolRarity(limited, legendary) < 0);
  assert.ok(context.comparePoolRarity(legendary, epic) < 0);
});

test("自动主题严格按本地时间在 06:00 和 18:00 切换", () => {
  const localTime = (hour, minute) => new Date(2026, 7, 9, hour, minute, 0, 0);
  assert.equal(context.resolvedTheme("auto", localTime(5, 59)), "dark");
  assert.equal(context.resolvedTheme("auto", localTime(6, 0)), "light");
  assert.equal(context.resolvedTheme("auto", localTime(17, 59)), "light");
  assert.equal(context.resolvedTheme("auto", localTime(18, 0)), "dark");
  assert.equal(context.resolvedTheme("light", localTime(23, 0)), "light");
  assert.equal(context.resolvedTheme("dark", localTime(12, 0)), "dark");
});

test("品质与数量排序默认从高到低", () => {
  for (const sort of ["acquired", "rarity", "mastery", "skinCount", "chromaCount"]) {
    assert.equal(context.defaultDescending(sort), true, sort);
  }
  assert.equal(context.defaultDescending("name"), false);
  assert.equal(context.defaultDescending("champion"), false);
});

test("普通炫彩始终平铺且不再提示悬停展开", () => {
  const css = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
  assert.doesNotMatch(source, /悬停展开|滚轮横移/);
  assert.doesNotMatch(css, /--chroma-stack-offset|--chroma-depth-y|--chroma-tilt/);
  assert.match(css, /\.chroma-stack-cards[^}]*scroll-snap-type:\s*x proximity/);
});

test("炫彩横栏保留页面纵向滚动，仅在 Shift 滚轮时横移", () => {
  const navigation = functionSource("enableHorizontalNavigation");
  const css = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
  assert.match(navigation, /!event\.shiftKey/);
  assert.match(navigation, /previousButton\.disabled/);
  assert.match(navigation, /nextButton\.disabled/);
  assert.match(source, /show-prestige-chromas/);
  assert.match(css, /\.chroma-mini-card[^}]*width:\s*320px;[^}]*height:\s*212px/);
  assert.match(css, /\.chroma-stack-cards::\-webkit-scrollbar[^}]*display:\s*none/);
});

test("炫彩横栏包含臻彩并只显示去掉所属皮肤后的短名称", () => {
  assert.equal(context.chromaDisplayName({ id: 64032, name: "神龙尊者 圣龙李青 翠屏", parentSkinName: "神龙尊者 圣龙李青" }), "翠屏");
  assert.equal(context.chromaDisplayName({ id: 64033, name: "神龙尊者 圣龙李青（登龙）", parentSkinName: "神龙尊者 圣龙李青" }), "登龙");
  assert.equal(context.chromaDisplayName({ id: 1, name: "龙王之怒", parentSkinName: "龙" }), "龙王之怒");
  assert.match(source, /for \(const chroma of items\) \{/);
  assert.match(source, /is-prestige-chroma/);
  assert.match(source, /chroma-mini-name/);
  assert.match(source, /chroma-prestige-badge/);
  assert.match(source, /chromaImageSources\(chroma, "ordinary"\)/);
  assert.match(source, /openChromaDetails\(chroma, parent\.items, \{ artworkMode: "ordinary" \}\)/);
});

test("臻彩详情可在普通炫彩图与独立原画之间切换", () => {
  const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
  assert.match(html, /id="skin-dialog-artwork"/);
  assert.match(source, /function ordinaryChromaImageSources/);
  assert.match(source, /detailArtworkMode === "prestige" \? "ordinary" : "prestige"/);
  assert.match(source, /openChromaDetails\(chroma, candidates, \{ artworkMode: "prestige" \}\)/);
});

test("卡片等待图片时显示加载中，失败后才显示暂无预览", () => {
  const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
  assert.match(html, /class="image-fallback" role="img">加载中</);
  assert.match(functionSource("deferCardImageSources"), /fallback\.textContent = "加载中"/);
  assert.match(functionSource("loadImageSources"), /fallback\.dataset\.emptyText \|\| "暂无预览"/);
});

test("详情与炫彩导航共用全局图标尺寸令牌", () => {
  const css = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
  const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
  assert.match(css, /--control-icon-size:\s*24px/);
  assert.match(css, /\.control-icon[^}]*width:\s*var\(--control-icon-size\)/);
  assert.match(html, /id="skin-dialog-fullscreen"[^>]*>[\s\S]*?<svg class="control-icon"/);
  assert.match(source, /navigationIcon\("previous"\)/);
});

test("奖池不再使用三合一优先层级并在切换拥有状态时重置品质", () => {
  const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
  assert.doesNotMatch(html, /三合一优先/);
  assert.doesNotMatch(source, /poolPriority/);
  assert.match(source, /state\.poolQuality = "all";\s*el\.poolQuality\.value = "all";/);
});

test("主滚动区域从顶部栏下方开始且不显示滚动条箭头", () => {
  const css = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
  const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
  assert.match(html, /<header class="topbar">[\s\S]*?<div id="app-scroll" class="app-scroll">[\s\S]*?<main class="shell">/);
  assert.match(css, /\.app-main[^}]*overflow:\s*hidden/);
  assert.match(css, /\.app-scroll[^}]*overflow-y:\s*auto/);
  assert.match(css, /\*::\-webkit-scrollbar-button[^}]*display:\s*none/);
  assert.match(source, /el\.appScroll\.scrollTo/);
});

test("收藏临时筛选会恢复默认值，奖池详情按实际展示顺序导航", () => {
  const reset = functionSource("resetCollectionControls");
  assert.match(reset, /state\.qualitySelections\.clear\(\)/);
  assert.match(reset, /state\.showUnownedChromas = true/);
  assert.match(reset, /state\.showPrestigeChromas = true/);
  assert.doesNotMatch(source, /savePreference\("show-(?:unowned|prestige)-chromas"/);
  assert.match(source, /const displayOrder = Array\.from\(groups\.values\(\)\)\.flat\(\)/);
  assert.match(source, /detailItems: displayOrder/);
});

test("皮肤详情只在英雄分组时按英雄切换，其余视图按当前展示顺序", () => {
  assert.match(functionSource("renderItems"), /makeSkinCard\(visible\[index\], \{ detailItems: visible \}\)/);
  assert.match(functionSource("renderRarityGroups"), /const displayOrder = groups\.flatMap/);
  assert.match(functionSource("renderRarityGroups"), /detailItems: displayOrder/);
  assert.match(functionSource("renderChampionGroups"), /detailItems: group\.skins/);
  assert.match(functionSource("openSkinDetails"), /state\.detailItems = candidates \|\| state\.items/);
});

test("最小窗口使用应用内排序菜单并在工具栏内换行", () => {
  const css = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
  const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
  assert.match(html, /id="sort-menu" class="select-menu-popover"/);
  assert.match(css, /\.select-menu-popover[^}]*right:\s*0/);
  assert.match(css, /@media \(max-width: 900px\)[\s\S]*?\.filters \{ flex-wrap: wrap/);
});

test("炫彩显隐不再扫描整棵 DOM 或触发布局测量", () => {
  const visibility = functionSource("applyChromaVisibility");
  const css = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
  assert.doesNotMatch(visibility, /querySelectorAll|dispatchEvent|scrollWidth|clientWidth/);
  assert.match(visibility, /classList\.toggle\("hide-unowned"/);
  assert.match(visibility, /classList\.toggle\("hide-prestige"/);
  assert.doesNotMatch(css, /:has\(\.chroma-mini-card/);
});

test("大量炫彩的显隐统计保持线性且远低于一帧交互预算", () => {
  const items = Array.from({ length: 20000 }, (_, index) => ({ owned: index % 3 === 0, isPrestige: index % 11 === 0 }));
  const started = performance.now();
  const counts = context.chromaVisibilityCounts(items);
  const elapsed = performance.now() - started;
  assert.equal(counts.all, 20000);
  assert.ok(counts.owned > 0 && counts.ordinary > counts.ownedOrdinary);
  assert.ok(elapsed < 100, `20,000 条统计耗时 ${elapsed.toFixed(2)}ms`);
});

test("卡片资源采用可视区延迟加载且远程臻彩受独立并发限制", () => {
  assert.match(source, /new IntersectionObserver/);
  assert.match(source, /rootMargin:\s*"620px 0px"/);
  assert.match(source, /state\.activePrestigeCardImages < 2/);
  assert.match(functionSource("makeChromaStack"), /deferCardImageSources/);
  assert.match(source, /function makeSkinCard[\s\S]{0,1800}deferCardImageSources/);
});
