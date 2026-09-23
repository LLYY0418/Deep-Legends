// R128：全项目去除统计口径与多余说明 + 海斗详情页 tab 一级化 + 表现指标重排
// + 概览顺序与推荐卡标题行。
//
// 三类断言：
//  1. 全局扫描钉子（§2.6）：生产 JS 里不得再出现「统计口径」「不构成因果」
//     「仅描述赛后关联」——这是 §2.5 写进 AGENTS.md / CLAUDE.md 那条规则的唯一
//     自动化护栏，扫描自身带一个「塞回去必须被抓到」的自检。
//  2. 删除清单（§2.3-A / §2.3-B）：六处口径调用点、实现本体、shared.js、样式，
//     以及七处方法论/免责说明逐条核对；同时核对 §2.3-C 的保留项没有被动到。
//  3. 结构与文案（§2.1 / §2.2 / §2.4）：详情 tab 等分一级化、表现面板逐行排版与
//     多杀合并行、概览三段顺序、推荐卡标题行去掉「N 个」。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const webRoot = __dirname;
const repoRoot = path.join(__dirname, "..", "..");
const read = (...parts) => fs.readFileSync(path.join(...parts), "utf8");
const readWeb = (name) => read(webRoot, name);

const championsScript = readWeb("champions.js");
const gameplayScript = readWeb("gameplay.js");
const championsStyles = readWeb("champions.css");
const gameplayStyles = readWeb("gameplay.css");
const indexHTML = readWeb("index.html");

// 生产脚本＝backend/web 下的 .js，排除测试文件（测试里必须能写这些短语当断言）。
const productionScripts = fs.readdirSync(webRoot)
  .filter((name) => name.endsWith(".js") && !name.endsWith(".test.cjs"))
  .map((name) => ({ name, text: readWeb(name) }));

// 在源码里切出某个函数的片段（到下一个锚点为止），用来做「形状」断言，
// 避免整文件正则把别处的同名字符串误判成命中。
function sliceBetween(source, startAnchor, endAnchor) {
  const start = source.indexOf(startAnchor);
  assert.notEqual(start, -1, `找不到起点锚点：${startAnchor}`);
  const end = source.indexOf(endAnchor, start + startAnchor.length);
  assert.notEqual(end, -1, `找不到终点锚点：${endAnchor}`);
  return source.slice(start, end);
}

// ---------------------------------------------------------------------------
// 1. §2.6 全局扫描钉子
// ---------------------------------------------------------------------------

const BANNED_PHRASES = ["统计口径", "不构成因果", "仅描述赛后关联"];

test("R128 §2.6 生产 JS 里不再出现口径/因果/关联类说明文案", () => {
  assert.ok(productionScripts.length >= 10, "扫描口径没覆盖到 backend/web 的脚本");
  for (const { name, text } of productionScripts) {
    for (const phrase of BANNED_PHRASES) {
      assert.equal(text.includes(phrase), false, `${name} 里出现了「${phrase}」`);
    }
  }
});

test("R128 §2.6 对抗变异：把禁用短语塞回 champions.js，扫描必须抓到", () => {
  const injected = productionScripts.map((item) => (
    item.name === "champions.js" ? { ...item, text: `${item.text}\n// 统计口径\n` } : item
  ));
  const hits = injected
    .filter((item) => BANNED_PHRASES.some((phrase) => item.text.includes(phrase)))
    .map((item) => item.name);
  assert.deepEqual(hits, ["champions.js"], "扫描逻辑抓不到重新塞回来的口径文案");
  // 未注入时同一套判据必须是空的，否则上面那条只是恒真。
  const clean = productionScripts.filter((item) => BANNED_PHRASES.some((phrase) => item.text.includes(phrase)));
  assert.deepEqual(clean, []);
});

// ---------------------------------------------------------------------------
// 2. §2.3-A 统计口径：六处调用点 + 实现本体 + shared.js + 样式
// ---------------------------------------------------------------------------

test("R128 §2.3-A 口径说明的实现、调用点、样式与 shared.js 全部删除", () => {
  for (const [name, text] of [["champions.js", championsScript], ["gameplay.js", gameplayScript]]) {
    assert.doesNotMatch(text, /renderMeasurementTechnique/, `${name} 仍在调用口径渲染`);
    assert.doesNotMatch(text, /deepLegendsShared/, `${name} 仍在引用共享模块`);
    assert.doesNotMatch(text, /mayhem-measurement/);
    assert.doesNotMatch(text, /recommendation-measurement/);
    assert.doesNotMatch(text, /measurementTechnique/, `${name} 仍在消费后端口径字段`);
  }
  // 详情页页脚函数与节点一并删除（它是口径说明在英雄页的落点）。
  assert.doesNotMatch(championsScript, /mayhemDetailFooter|data-mayhem-detail-footer/);
  // 队伍画像 tooltip 不再拼口径说明，只留可核对的入队人数。
  const portrait = sliceBetween(gameplayScript, "function renderLiveTeamPortraitTags(", "function renderLiveInsights(");
  assert.doesNotMatch(portrait, /measurementTechnique/);
  assert.match(portrait, /本队 \$\{resolved\} 位英雄进入统计/);
  // 文件与引用：shared.js 删除，index.html 不再引入。
  assert.equal(fs.existsSync(path.join(webRoot, "shared.js")), false, "shared.js 应已删除");
  assert.doesNotMatch(indexHTML, /shared\.js|deepLegendsShared/);
  assert.match(indexHTML, /<script src="\/runtime\.js" defer><\/script>\s*<script src="\/image-queue\.js" defer><\/script>/, "删掉引入后其余脚本顺序不变");
  // Go 侧嵌入清单同步（否则 go test 会因文件缺失而红）。
  assert.doesNotMatch(read(repoRoot, "backend", "static_assets_test.go"), /"shared\.js"/);
  // 样式一并删除。
  assert.doesNotMatch(championsStyles, /mayhem-measurement|mayhem-detail-footer/);
  assert.doesNotMatch(gameplayStyles, /recommendation-measurement|mayhem-measurement/);
});

// ---------------------------------------------------------------------------
// 3. §2.3-B 方法论/免责说明：七处删除 + 唯一保留的一句累计说明
// ---------------------------------------------------------------------------

test("R128 §2.3-B 方法论与免责说明逐条消失", () => {
  // 海克斯推荐下方的「数据显示：N 项海克斯…」整块删除（函数、上限常量、样式）。
  assert.doesNotMatch(championsScript, /mayhemCautionNote|mayhemCautionItem|MAYHEM_CAUTION_LIMIT/);
  assert.doesNotMatch(championsScript, /按官方推荐顺序列出前|同品质中位数/);
  assert.doesNotMatch(championsStyles, /mayhem-caution-note/);
  // 表现指标：每格里的累计说明删除，副标题长句删除。
  assert.doesNotMatch(championsScript, /累计次数（受出场场次影响，不可跨英雄直接比较）/);
  assert.doesNotMatch(championsScript, /赛后数据的每局均值/);
  assert.doesNotMatch(championsStyles, /\.mayhem-performance-group dd small/);
  // 该英雄专属品质概率副标题缩成一句事实描述；全服分布面板副标题删除。
  assert.doesNotMatch(championsScript, /全服整体分布见「海克斯图鉴/);
  assert.match(championsScript, /<p>各阶段白银 \/ 黄金 \/ 棱彩选取概率<\/p>/);
  assert.doesNotMatch(championsScript, /玩家实际选择的样本分布，不代表抽取、刷新或保底概率/);
  // 我的海斗出装计数上的口径 tooltip 删除，只留「本人 N 场」。
  assert.doesNotMatch(championsScript, /本机保存的本赛季海克斯大乱斗对局|不是全服数据，也不含其他账号/);
  const personal = sliceBetween(championsScript, "function renderMayhemPersonalBuilds(", "function resetArenaControls(");
  assert.match(personal, /<span class="section-count">本人 \$\{compactNumber\(payload\.sampleGames\)\} 场<\/span>/);
  assert.doesNotMatch(personal, /data-tooltip/);
  // 对局阵容克制条下的覆盖率免责说明删除。
  assert.doesNotMatch(gameplayScript, /只列出上游标记为显著的对位与配合/);
  // 海斗段位卡的位置胜率块整块不渲染。
  assert.doesNotMatch(gameplayScript, /海克斯大乱斗没有分路，不提供位置胜率与能力雷达口径/);
  assert.match(gameplayScript, /const positionRow = isMayhem\s*\?\s*""/);
});

test("R128 §2.2 表现 tab 底部只保留用户指定的那一句累计说明", () => {
  const panel = sliceBetween(championsScript, "function renderMayhemPerformancePanel(", "function mayhemPerformanceGroups(");
  const copy = "双杀/三杀/四杀/五杀为累计次数，受出场场次影响，不可跨英雄直接比较。";
  assert.ok(panel.includes(`'<p class="mayhem-performance-note">${copy}</p>'`), "底部说明文案必须逐字一致");
  // 只在确实有累计项时出现。
  assert.match(panel, /metrics\.some\(\(metric\) => metric\?\.cumulative === true\)/);
  // 只在表现 tab：说明拼在 .mayhem-performance 这个 section 内部末尾。
  assert.ok(panel.includes('</div>${cumulativeNote}</section>`'), "说明必须落在表现面板 section 之内");
  assert.equal((championsScript.match(/mayhem-performance-note/g) || []).length, 1, "说明只出现一处");
  assert.match(championsStyles, /\.mayhem-performance-note \{/);
});

test("R128 §2.3-C 状态/错误/数据回退提示保留不动", () => {
  assert.match(readWeb("pro-players.js"), /刷新失败，当前显示上次读取的数据/);
  assert.match(readWeb("suite.js"), /皮肤目录暂时读取失败/);
  // 数据回退：告诉用户「当前看到的不是本模式数据」，删了会误导。
  assert.match(gameplayScript, /当前模式的出装与技能暂参考极地大乱斗数据/);
  assert.match(gameplayScript, /版本，仅供参考/);
  // 悬浮才出现的公式说明与「本地估算」「第三方估算」小标签。
  assert.match(gameplayScript, /对手基准来自这名玩家排位中的同位置对手样本聚合/);
  assert.match(gameplayScript, /第三方估算/);
  assert.match(championsScript, /本地估算/);
});

// ---------------------------------------------------------------------------
// 4. §2.1 / §2.2 / §2.4 结构与文案
// ---------------------------------------------------------------------------

test("R128 §2.1 概览/构筑/表现 tab 改成等分的一级切换", () => {
  const toolbar = sliceBetween(championsScript, "function mayhemDetailToolbar(", "function mayhemDetailPanelMarkup(");
  // tab 组带上真实页签数，CSS 据此等分；图鉴入口仍是独立按钮。
  assert.ok(toolbar.includes('data-mayhem-detail-tabs style="--tab-count:${specs.length}"'));
  assert.match(toolbar, /class="mayhem-atlas-entry" data-mayhem-view="atlas"/);
  assert.doesNotMatch(toolbar, /data-tooltip|<small|<p>/, "tab 区不得新增说明文字或提示气泡");

  assert.match(championsStyles, /\.mayhem-detail-tabs \{[^}]*flex: 1 1 auto;[^}]*grid-template-columns: repeat\(var\(--tab-count\),minmax\(0,1fr\)\)/);
  assert.match(championsStyles, /\.mayhem-detail-tabs \{[^}]*--tab-count: 3;/, "变量要有声明来源（R117 的变量解析钉子）");
  assert.match(championsStyles, /\.mayhem-detail-tabs button \{[^}]*min-height: 42px;[^}]*font-size: 13px;[^}]*font-weight: 750/);
  assert.doesNotMatch(championsStyles, /\.mayhem-detail-tabs button \{[^}]*color: var\(--muted\)/, "未选中态不得再用 --muted");
  assert.match(championsStyles, /\.mayhem-detail-tabs button\.is-active \{[^}]*color: var\(--primary\);[^}]*box-shadow: inset 0 -2px 0 var\(--primary\);/);
  assert.match(championsStyles, /\.mayhem-atlas-entry \{[^}]*min-height: 42px;[^}]*flex: 0 0 auto/);
  // 窄屏：tab 组独占一行、按钮 44px，图鉴入口换到下一行。
  const narrow = championsStyles.slice(championsStyles.lastIndexOf("@media (max-width: 700px)"));
  assert.match(narrow, /\.mayhem-detail-tabs \{ flex: 1 0 100%; \}/);
  assert.match(narrow, /\.mayhem-detail-tabs button \{ min-height: 44px; \}/);
  assert.match(narrow, /\.mayhem-performance-groups \{ grid-template-columns: 1fr; \}/);
});

test("R128 §2.2 表现指标改成逐行列表，多杀四项合并成一行", () => {
  const group = sliceBetween(championsScript, "function mayhemPerformanceGroup(", "function mayhemPerformanceValue(");
  // 每行一个指标：数值与「较平均」是两个独立元素，中间由 CSS 给间距。
  assert.ok(group.includes('<dd><b>${mayhemPerformanceValue(metric)}</b>${delta ? `<span class="mayhem-delta'), "数值与「较平均」必须分成两个元素");
  // 累计项合并成一行「多杀累计」，四个数字横排，不带 ± 标签。
  assert.ok(group.includes('<dt>多杀累计</dt>'));
  assert.ok(group.includes('class="mayhem-multikill-values"'));
  assert.match(group, /rows\.filter\(\(metric\) => metric\?\.cumulative !== true\)/);
  assert.match(group, /rows\.filter\(\(metric\) => metric\?\.cumulative === true\)/);
  // 累计行只有弱化的数字，不带「较平均」标签（现有逻辑本来就不给累计项算 delta）。
  const cumulativeCell = group.slice(group.indexOf('<div class="is-cumulative">'));
  assert.ok(cumulativeCell.length > 0, "找不到累计行的 markup");
  assert.doesNotMatch(cumulativeCell, /mayhem-delta|较平均/);
  // 分组宽屏两列、组内逐行；累计值弱化。
  assert.match(championsStyles, /\.mayhem-performance-groups \{[^}]*grid-template-columns: repeat\(2,minmax\(0,1fr\)\)/);
  assert.match(championsStyles, /\.mayhem-performance-group dl > div \{[^}]*grid-template-columns: minmax\(0,1fr\) auto;[^}]*gap: 8px/);
  assert.doesNotMatch(championsStyles, /\.mayhem-performance-group dl \{[^}]*auto-fit/);
  assert.match(championsStyles, /\.mayhem-multikill-values b \{[^}]*color: var\(--muted\)/);
});

test("R128 §2.4 概览顺序与推荐卡标题行", () => {
  const overview = sliceBetween(championsScript, "function mayhemOverviewTabMarkup(", "function mayhemBuildTabMarkup(");
  const order = ["renderRecommendedAugments", "renderMayhemOpeningConfiguration", "renderMayhemHeroRarityPanel"].map((name) => overview.indexOf(name));
  assert.ok(order.every((index) => index > 0), `概览三段缺一不可：${order}`);
  assert.ok(order[0] < order[1] && order[1] < order[2], "顺序必须是 海克斯推荐 → 开局配置 → 该英雄专属品质概率");

  const augments = sliceBetween(championsScript, "function renderRecommendedAugments(", "function mayhemActiveStage(");
  // 标题行删掉「N 个」，chips 成为 header 最后一个子元素（靠 space-between 贴右）。
  assert.ok(augments.includes("${chips}</header>"), "阶段 chips 必须是 header 的最后一个子元素");
  assert.doesNotMatch(augments, /section-count/, "标题行不再有计数");
  assert.doesNotMatch(augments, /entries\.length\} 个/);
  // 推荐卡本体保持现状：收益率/置信度/官方档位/阶段筛选一个都不动。
  assert.match(championsScript, /mayhemAugmentConfidenceMetrics/);
  assert.match(championsScript, /\["官方档位", escapeHTML\(label\), "is-hex-label"\]/);
  assert.match(championsScript, /function renderMayhemStageChips\(items\)/);
  assert.match(championsStyles, /\.recommendation-section > header \{[^}]*justify-content: space-between/);
});

test("R128 §2.5 规则写进 AGENTS.md 与 CLAUDE.md，旧工单要求标注撤销", () => {
  for (const file of ["AGENTS.md", "CLAUDE.md"]) {
    const text = read(repoRoot, file);
    assert.match(text, /界面上不添加「统计口径」/, `${file} 缺少界面文案红线`);
    assert.match(text, /数据来源与算法说明写在代码注释或 docs 里，不进 UI/, `${file} 缺少落地方式`);
    assert.match(text, /确需提示用户的只限：错误\/失败、数据回退/, `${file} 缺少例外范围`);
  }
  const review = read(repoRoot, "docs", "r116-proposal-feasibility-review.md");
  assert.match(review, /已被 2026-09-22 用户指示撤销（R128/);
  const worklist = read(repoRoot, "docs", "history", "worklists", "R116-B-P0六项与详情页三tab重整-工单.md");
  assert.match(worklist, /已被 2026-09-22 用户指示撤销（R128/);
});
