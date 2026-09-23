"use strict";
// R123：生涯头像/旗帜写入退场后的收藏页只读子页验收测试。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const facadeSource = fs.readFileSync(path.join(__dirname, "favorites-facade.js"), "utf8");
const suiteSource = fs.readFileSync(path.join(__dirname, "suite.js"), "utf8");
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");

function functionSource(script, name) {
  const marker = `function ${name}(`;
  const start = script.indexOf(marker);
  assert.notEqual(start, -1, `missing ${name}`);
  const bodyStart = script.indexOf("{", script.indexOf(")", start));
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < script.length; index += 1) {
    const char = script[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return script.slice(start, index + 1);
  }
  assert.fail(`unbalanced ${name}`);
}
const names = ["formatCount", "imageURL", "iconImageURL", "facadeCollectionIconRows", "facadeCollectionBannerRows", "facadeCollectionIconFields", "facadeCollectionBannerFields", "facadeCollectionMetaParts", "facadeCollectionMetaText"];
const helpers = Function("window", `"use strict";\n${names.map((name) => functionSource(facadeSource, name)).join("\n")}\nreturn {${names.join(",")}};`)({ deepLegendsChampionSearch: null });

const icons = [
  { id: 1, title: "甲", year: 2023, owned: true, sets: ["系列一"], searchTerms: ["jia"] },
  { id: 2, title: "乙", year: 2026, owned: false, sets: [], searchTerms: ["yi"] },
  { id: 3, title: "丙", year: 2025, owned: false, sets: ["系列一"], searchTerms: ["bing"] },
];
const banners = [
  { id: "3", localizedName: "甲旗", owned: true, isTencentOnly: false },
  { id: "4", localizedName: "乙旗", owned: false, isTencentOnly: true, idSecondary: "GOLD" },
];
const iconFilters = (overrides = {}) => ({ query: "", group: "all", set: "", sort: "new", descending: false, showUnowned: true, ...overrides });
const bannerFilters = (overrides = {}) => ({ query: "", group: "all", sort: "id", descending: false, showUnowned: true, ...overrides });

test("R123 retired write surfaces are gone from the suite page", () => {
  for (const marker of ["openFacadeIconPicker", "openFacadeBannerPicker", "data-facade-icons", "data-facade-banners", "设为头像", "旗帜已应用", "立即更改", "facade-picker"]) {
    assert.equal(suiteSource.includes(marker), false, `suite.js still contains ${marker}`);
  }
  assert.match(suiteSource, /data-facade-browse="icons"/);
  assert.match(suiteSource, /data-facade-browse="banners"/);
  assert.match(suiteSource, /在收藏页浏览头像与旗帜/);
});

test("R123 favorites page exposes the facade collection subpage", () => {
  assert.match(html, /data-favorites-page="facade-collection"/);
  assert.match(html, /id="favorites-facade-panel"/);
  assert.match(html, /id="facade-detail-dialog"/);
  assert.match(html, /src="\/favorites-facade\.js"/);
  assert.match(html, /头像旗帜与拥有状态/);
  assert.match(html, /<div id="facade-list-meta"[^>]*><\/div>/, "断连占位文案不得预置「等待客户端数据」");
  assert.match(facadeSource, /el\.toolbar\.hidden = true;/);
  assert.match(facadeSource, /el\.listRow\.hidden = true;/);
  assert.doesNotMatch(html, /facade-detail-dialog[\s\S]{0,400}?(设为头像|立即更改|apply)/);
});

test("R123 owned counts are derived from catalog data, not hardcoded", () => {
  const rows = helpers.facadeCollectionIconRows(icons, iconFilters(), 2026);
  assert.equal(rows.length, 3);
  const owned = rows.filter((icon) => icon.owned).length;
  assert.equal(owned, 1);
  // 对抗变异：把 owned 判定取反后，统计必须跟着变。
  const flipped = icons.map((icon) => ({ ...icon, owned: !icon.owned }));
  assert.equal(helpers.facadeCollectionIconRows(flipped, iconFilters(), 2026).filter((icon) => icon.owned).length, 2);
  const bannerOwned = helpers.facadeCollectionBannerRows(banners, bannerFilters()).filter((banner) => banner.owned).length;
  assert.equal(bannerOwned, 1);
});

test("R123 unowned entries hide when the toggle is off (R130: icons default to owned-only)", () => {
  // R130 P5：头像默认关闭「显示未拥有」，旗帜仍默认打开；HTML 里不再预置 checked，
  // 初始状态一律由 syncControls 从 filters 写回。
  assert.match(facadeSource, /icons: \{[^}]*showUnowned: false \}/, "头像默认必须只显示已拥有");
  assert.match(facadeSource, /banners: \{[^}]*showUnowned: true \}/, "旗帜默认必须显示全部");
  assert.doesNotMatch(html, /id="facade-show-unowned"[^>]*checked/, "复选框不得在 HTML 里预置勾选");
  const withUnowned = helpers.facadeCollectionIconRows(icons, iconFilters(), 2026);
  assert.equal(withUnowned.length, 3);
  const ownedOnly = helpers.facadeCollectionIconRows(icons, iconFilters({ showUnowned: false }), 2026);
  assert.deepEqual(ownedOnly.map((icon) => icon.id), [1]);
  const bannersOwnedOnly = helpers.facadeCollectionBannerRows(banners, bannerFilters({ showUnowned: false }));
  assert.deepEqual(bannersOwnedOnly.map((banner) => banner.id), ["3"]);
  const locked = helpers.facadeCollectionIconFields(icons[1], false);
  assert.equal(locked.locked, true);
  assert.equal(locked.state, "未拥有");
  const ownedFields = helpers.facadeCollectionIconFields(icons[0], false);
  assert.equal(ownedFields.locked, false);
  assert.equal(ownedFields.state, "已拥有");
});

test("R123 ownership-unavailable payloads downgrade state badges and locks", () => {
  const fields = helpers.facadeCollectionIconFields(icons[1], true);
  assert.equal(fields.locked, false, "拥有状态未知时不得套用未拥有灰阶");
  assert.equal(fields.state, "拥有状态未知");
  // R130 P5：摘要行去掉「· 已显示 N」，参数表也少了一个 visible。
  const meta = helpers.facadeCollectionMetaText("头像", 5099, 0, true);
  assert.match(meta, /拥有状态未读取/);
  const known = helpers.facadeCollectionMetaText("头像", 5099, 128, false);
  assert.equal(known, "共 5,099 款头像 · 已拥有 128");
});

test("R123 filters and sorts follow the worklist dimensions", () => {
  const recent = helpers.facadeCollectionIconRows(icons, iconFilters({ group: "recent" }), 2026);
  assert.deepEqual(recent.map((icon) => icon.id), [2, 3]);
  const set = helpers.facadeCollectionIconRows(icons, iconFilters({ set: "系列一" }), 2026);
  assert.deepEqual(set.map((icon) => icon.id), [3, 1]);
  const query = helpers.facadeCollectionIconRows(icons, iconFilters({ query: "bing" }), 2026);
  assert.deepEqual(query.map((icon) => icon.id), [3]);
  const byName = helpers.facadeCollectionIconRows(icons, iconFilters({ sort: "name" }), 2026);
  assert.deepEqual(byName.map((icon) => icon.id), [3, 1, 2]);
  const unownedFirst = helpers.facadeCollectionIconRows(icons, iconFilters({ sort: "unowned" }), 2026);
  assert.equal(unownedFirst[0].owned, false);
  const tencent = helpers.facadeCollectionBannerRows(banners, bannerFilters({ group: "tencent" }));
  assert.deepEqual(tencent.map((banner) => banner.id), ["4"]);
  const bannerFields = helpers.facadeCollectionBannerFields(banners[1], false);
  assert.equal(bannerFields.hero, "国服专属");
  assert.equal(bannerFields.meta, "ID 4 · GOLD");
});

test("R124 ownership-unavailable catalog survives a previously closed unowned toggle", () => {
  // 工单证据数据：拥有状态不可用时后端 owned 一律 false；用户此前关过「显示未拥有」。
  const unknownIcons = icons.map((icon) => ({ ...icon, owned: false }));
  const unknownBanners = banners.map((banner) => ({ ...banner, owned: false }));
  assert.equal(helpers.facadeCollectionIconRows(unknownIcons, iconFilters({ showUnowned: false }), 2026, true).length, 3);
  assert.equal(helpers.facadeCollectionBannerRows(unknownBanners, bannerFilters({ showUnowned: false }), true).length, 2);
  // 对抗变异：去掉修复条件后上面两条会返回 0；拥有状态已知时开关仍必须生效。
  assert.equal(helpers.facadeCollectionIconRows(unknownIcons, iconFilters({ showUnowned: false }), 2026, false).length, 0);
  assert.equal(helpers.facadeCollectionBannerRows(unknownBanners, bannerFilters({ showUnowned: false }), false).length, 0);
  // R130 P5-4：syncControls 不再改写用户保存的值。R124 原来在这里断言的正是那条
  // 会把用户设置永久改掉的复位语句，现在整个 syncControls 里都不允许再对
  // filters.showUnowned 赋值；实际生效值只在 facadeCollection*Rows 里按
  // 「ownershipUnavailable || filters.showUnowned」计算，开关显示仍从记忆值写回。
  const syncControls = functionSource(facadeSource, "syncControls");
  assert.doesNotMatch(syncControls, /filters\.showUnowned\s*=[^=]/, "syncControls 不得再改写用户保存的开关值");
  assert.match(syncControls, /el\.showUnowned\.checked = filters\.showUnowned;/, "开关显示状态仍然从记忆值写回");
});

test("R125 icon tiles render at native square size", () => {
  const css = fs.readFileSync(process.env.R138_APP_CSS || path.join(__dirname, "app.css"), "utf8");
  assert.match(css, /\.facade-grid\.is-icons \{[^}]*repeat\(auto-fill, 92px\)/, "头像格子必须固定 92px，不随窗口拉伸");
  assert.match(css, /\.facade-grid\.is-icons \.skin-art \{[^}]*aspect-ratio: 1\/1/, "头像格子必须正方形");
  assert.match(css, /\.facade-grid\.is-icons \.skin-art img \{ object-fit: contain; \}/, "头像不得裁剪");
  const iconRule = css.match(/\.facade-grid\.is-icons\s*\{([^}]*)\}/)?.[1] || "";
  const gap = iconRule.match(/\bgap:\s*(\d+)px\s+(\d+)px\s*;/);
  assert.ok(gap, "头像网格必须显式设置行列间距");
  assert.ok(Number(gap[1]) >= 18 && Number(gap[2]) >= 14, `头像间距应明显变大，实际 ${gap[1]}px ${gap[2]}px`);
  assert.doesNotMatch(iconRule, /gap:\s*12px\s+10px/, "不能退回拥挤的旧间距");
  assert.match(css, /\.facade-grid\.is-banners\s*\{[^}]*gap:\s*20px 16px/, "R140 旗帜间距应同步加大");
  assert.match(facadeSource, /classList\.toggle\("is-icons", state\.view === "icons"\)/);
  assert.match(facadeSource, /if \(state\.view === "icons"\) card\.title = fields\.title;/);
});
