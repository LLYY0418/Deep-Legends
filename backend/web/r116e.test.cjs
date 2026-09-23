// R116-E 前端验证：海克斯大乱斗页签（用户裁决新增的 UI 入口）、
// rankedQueueLabel 既存缺陷的闭合、以及 P2-6 静态查询在英雄详情页「构筑」tab 的渲染。
//
// 方法论沿用仓库既有做法（champions.test.cjs / r116b / r116d 的 compileFunctions）：
// 按函数名从源码里切出函数体、用 Function 注入依赖后单独跑。
//
// ★本文件的一条硬纪律：**不给 rankedQueueLabel 注入任何桩**。
// 这个函数以前在生产代码里根本没定义（全仓只有 champions.test.cjs 里一个测试桩），
// 真机命中即 ReferenceError，而测试之所以没抓到正是因为测试自己塞了桩。
// 所以下面一律编译真实实现，并且专门写一条「把定义删掉必须 FAIL」的对抗变异。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const gameplayScript = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const championsScript = fs.readFileSync(path.join(__dirname, "champions.js"), "utf8");
const backendMain = fs.readFileSync(path.join(__dirname, "..", "main.go"), "utf8");
const gameplayBackend = fs.readFileSync(path.join(__dirname, "..", "gameplay.go"), "utf8");
const seasonBackend = fs.readFileSync(path.join(__dirname, "..", "season_stats.go"), "utf8");

// ---------------------------------------------------------------------------
// 编译工具
// ---------------------------------------------------------------------------

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
  if (source.slice(Math.max(0, start - 6), start) === "async ") start -= 6;
  const parametersStart = source.indexOf("(", start);
  let depth = 0;
  let parametersEnd = -1;
  for (let index = parametersStart; index < source.length; index += 1) {
    const character = source[index];
    if (character === "(") depth += 1;
    else if (character === ")") {
      depth -= 1;
      if (depth === 0) { parametersEnd = index; break; }
    }
  }
  assert.notEqual(parametersEnd, -1, `${name} parameters not closed`);
  const bodyStart = source.indexOf("{", parametersEnd);
  assert.notEqual(bodyStart, -1, `${name} body not found`);
  let bodyDepth = 0;
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
    if (character === "{") bodyDepth += 1;
    else if (character === "}") {
      bodyDepth -= 1;
      if (bodyDepth === 0) return source.slice(start, index + 1);
    }
  }
  assert.fail(`${name} body not closed`);
  return "";
}

// compileFunctions 按名字编译真实实现，并自动把「同源码里能找到的函数依赖」
// 一起带进来（沿用 champions.test.cjs 里 R75/R116-B 的做法）。
// 传进来的 dependencies 优先级最高——用来注入 state / escapeHTML 这类非函数环境。
function compileFunctions(source, names, dependencies = {}) {
  const bodies = names.map((name) => functionSource(source, name));
  const resolved = { ...dependencies };
  const transitive = [
    "rankedQueueLabel", "isMayhemQueueId", "rankedQueueNoun",
    "mayhemPersonalAssetNames", "objectRows", "percent", "compactNumber",
    "renderMayhemPersonalBuilds",
  ];
  for (const name of transitive) {
    if (resolved[name] !== undefined) continue;
    if (!bodies.some((body) => body.includes(`${name}(`))) continue;
    if (!source.includes(`function ${name}(`)) continue;
    bodies.push(functionSource(source, name));
  }
  const dependencyNames = Object.keys(resolved);
  const factory = Function(...dependencyNames, `"use strict";\n${bodies.join("\n")}\nreturn { ${names.join(", ")} };`);
  return factory(...dependencyNames.map((name) => resolved[name]));
}

const escapeHTML = (value) => String(value ?? "").replace(/[&<>"']/g, (character) => (
  { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[character]
));

// stripLineComments 去掉 // 行注释：注释里刻意写了被禁止的写法
// （解释为什么不能这么干），不剔掉就会自己咬自己。
function stripLineComments(source) {
  return source.split("\n").map((line) => {
    const position = line.indexOf("//");
    return position >= 0 ? line.slice(0, position) : line;
  }).join("\n");
}

// MAYHEM_QUEUE_TAB_KEY 是模块级常量，聚焦编译时不会跟着进作用域；
// 从生产源码里读出来注入，而不是在测试里另写一个字面量（那就会各说各话）。
const MAYHEM_QUEUE_TAB_KEY = gameplayScript.match(/const MAYHEM_QUEUE_TAB_KEY = "(\d+)";/)[1];

const renderDependencies = {
  escapeHTML,
  number: (value) => String(value ?? "—"),
  percent: (value) => `${value}%`,
  kda: (value) => String(value ?? "—"),
  compactNumber: (value) => String(value ?? "—"),
  positionLabel: (value) => value || "位置",
  positionIcon: () => "",
  iconFigure: () => "",
};

// ---------------------------------------------------------------------------
// 1. rankedQueueLabel：既存生产缺陷的闭合
// ---------------------------------------------------------------------------

test("R116-E rankedQueueLabel is defined in production source and resolves every queue", () => {
  // 缺陷本体：gameplay.js 调用 rankedQueueLabel，但生产代码里从来没有定义过。
  // 这条断言钉住「定义真的在 gameplay.js 里」，不是靠测试注入的桩。
  assert.match(gameplayScript, /function rankedQueueLabel\(/, "rankedQueueLabel must be defined in gameplay.js itself");
  // 对抗变异：把定义删掉，编译出来的对象里就不该再有这个函数。
  const mutated = gameplayScript.replace(/function rankedQueueLabel\([\s\S]*?\n  \}\n/, "");
  assert.doesNotMatch(mutated, /function rankedQueueLabel\(/, "the mutation must actually remove the definition");
  assert.throws(() => compileFunctions(mutated, ["rankedQueueLabel"], {}), /rankedQueueLabel source not found/);

  // 不注入任何桩，编译真实实现。
  const { rankedQueueLabel } = compileFunctions(gameplayScript, ["rankedQueueLabel"], {});
  assert.equal(rankedQueueLabel(420), "单双排");
  assert.equal(rankedQueueLabel("420"), "单双排");
  assert.equal(rankedQueueLabel(440), "灵活组排");
  // 三个海斗队列 ID 在官方目录里同名，UI 上合并成一个页签，所以名字必须一致。
  for (const queueId of [2300, 2400, 3270, "2400"]) {
    assert.equal(rankedQueueLabel(queueId), "海克斯大乱斗", `queue ${queueId}`);
  }
  // 未知队列一律返回空串：绝不把没核实过的队列默默标成「单双排」。
  for (const queueId of [3220, 450, 1700, 0, -1, "abc", null, undefined, NaN]) {
    assert.equal(rankedQueueLabel(queueId), "", `queue ${String(queueId)} must not be invented`);
  }
  assert.equal(rankedQueueLabel(3220, "兜底"), "兜底", "an explicit fallback is the caller's choice, not a silent default");
  // 3220 是「极地大乱斗」(aram 组)，不是海斗。
  const { isMayhemQueueId } = compileFunctions(gameplayScript, ["isMayhemQueueId"], {});
  for (const queueId of [2300, 2400, 3270]) assert.equal(isMayhemQueueId(queueId), true, `queue ${queueId}`);
  for (const queueId of [3220, 420, 440, 450, 0]) assert.equal(isMayhemQueueId(queueId), false, `queue ${queueId}`);
});

test("R116-E the scattered 440 ternaries are gone from gameplay.js", () => {
  const code = stripLineComments(gameplayScript);
  assert.doesNotMatch(code, /===\s*440\s*\?\s*"灵活组排"\s*:\s*"单双排"/, "queue names must go through rankedQueueLabel");
  assert.doesNotMatch(code, /queueId\s*===\s*440\s*\?/);
  // rankedQueueLabel 必须真的被调用，否则就是定义了但没接线。
  assert.ok((code.match(/rankedQueueLabel\(/g) || []).length >= 3, "rankedQueueLabel must be wired into the queue renderers");
});

// ---------------------------------------------------------------------------
// 2. 队列切换器：第三个 tab
// ---------------------------------------------------------------------------

test("R116-E the queue switcher grows a third mayhem tab only where it is honest", () => {
  const { rankedQueueSwitcher } = compileFunctions(gameplayScript, ["rankedQueueSwitcher"], { MAYHEM_QUEUE_TAB_KEY });
  const mayhemPayload = { rankedQueues: { 420: {}, 440: {}, 2300: {} } };
  const classicPayload = { rankedQueues: { 420: {}, 440: {} } };

  const recent = rankedQueueSwitcher({ rankedQueueRecent: "2300" }, "2300", "recent", mayhemPayload);
  assert.match(recent, /data-ranked-queue="2300"[^>]*>海克斯大乱斗</);
  assert.match(recent, /data-ranked-queue="420"[^>]*>单双排</);
  assert.match(recent, /data-ranked-queue="440"[^>]*>灵活组排</);
  assert.match(recent, /class="ranked-queue-button is-active"[^>]*data-ranked-queue="2300"/);
  // 加了非排位队列之后，「排位模式」这个无障碍标签就不准确了。
  assert.match(recent, /aria-label="对局模式"/);
  assert.doesNotMatch(recent, /aria-label="排位模式"/);
  assert.match(recent, /data-ranked-queue-scope="recent"/);

  // 没有海斗样本 → 不出第三个页签（不给用户一个点进去全是空的按钮）。
  const withoutMayhem = rankedQueueSwitcher({}, "420", "recent", classicPayload);
  assert.doesNotMatch(withoutMayhem, /海克斯大乱斗/);
  assert.match(withoutMayhem, /aria-label="排位模式"/);

  // 位置偏好与能力表现依赖分路口径，海斗没有分路 → 这两个 scope 永远只有两个页签。
  // 否则用户切过去之后整块被隐藏，页签本身就再也点不回来（死胡同）。
  for (const scope of ["ability", "position"]) {
    const markup = rankedQueueSwitcher({}, "420", scope, mayhemPayload);
    assert.doesNotMatch(markup, /海克斯大乱斗/, `${scope} must not offer a mayhem tab`);
    assert.match(markup, /aria-label="排位模式"/, `${scope} only offers ranked queues`);
    assert.match(markup, new RegExp(`data-ranked-queue-scope="${scope}"`));
  }

  // 不传 data 时回落到 tab.data（覆盖层与生涯弹窗都走这条路）。
  assert.match(rankedQueueSwitcher({ data: mayhemPayload }, "420", "recent"), /海克斯大乱斗/);
  // 旧载荷没有 rankedQueues → 不出海斗页签。
  assert.doesNotMatch(rankedQueueSwitcher({}, "420", "recent", {}), /海克斯大乱斗/);
});

test("R116-E the switcher click gate accepts the mayhem tab in the recent scope only", () => {
  const binder = functionSource(gameplayScript, "bindRankedQueueControls");
  // 老门禁 `queue !== "420" && queue !== "440"` 会把海斗按钮的点击直接吞掉。
  assert.doesNotMatch(stripLineComments(binder), /queue !== "420" && queue !== "440"/);
  assert.match(binder, /MAYHEM_QUEUE_TAB_KEY/);
  assert.match(binder, /scope === "recent" \? \["420", "440", MAYHEM_QUEUE_TAB_KEY\] : \["420", "440"\]/);
  assert.equal(MAYHEM_QUEUE_TAB_KEY, "2300", "the mayhem tab key must stay aligned with the backend's seasonMayhemPrimaryQueueID");
  assert.match(seasonBackend, /const seasonMayhemPrimaryQueueID = int64\(2300\)/);
  // 三个海斗队列合并成一个页签：源码里只允许出现一个海斗按钮。
  const switcher = functionSource(gameplayScript, "rankedQueueSwitcher");
  assert.equal((switcher.match(/>海克斯大乱斗</g) || []).length, 1, "one merged mayhem button, not three");
  assert.equal((switcher.match(/data-ranked-queue="\$\{MAYHEM_QUEUE_TAB_KEY\}"/g) || []).length, 1);
  assert.match(seasonBackend, /var seasonMayhemQueueIDs = \[\]int64\{2300, 2400, 3270\}/);
});

// ---------------------------------------------------------------------------
// 3. rankedQueueData：海斗页签的数据形状
// ---------------------------------------------------------------------------

test("R116-E rankedQueueData labels the mayhem tab and marks lanes as not applicable", () => {
  const { rankedQueueData } = compileFunctions(gameplayScript, ["rankedQueueData"], {});
  const data = {
    rankedQueues: {
      420: { recentRanked: { games: 7, queueLabel: "单双排" }, positions: [{ position: "top", games: 7, share: 100 }], abilitySampleGames: 7 },
      2300: { recentRanked: { games: 5, queueLabel: "海克斯大乱斗" }, positions: [], ability: null, abilitySampleGames: 0, positionQueueId: 2300, positionQueueLabel: "海克斯大乱斗" },
    },
  };
  const tab = { rankedQueueRecent: "2300", rankedQueueAbility: "420", rankedQueuePosition: "420" };
  const mayhem = rankedQueueData(data, tab, "recent");
  assert.equal(mayhem.queueId, 2300);
  assert.equal(mayhem.queueLabel, "海克斯大乱斗");
  assert.equal(mayhem.mayhem, true);
  assert.equal(mayhem.positionsApplicable, false, "ARAM has no lanes, the UI must not pretend otherwise");
  assert.deepEqual(mayhem.positions, []);
  assert.equal(mayhem.queueGames, 5);

  const solo = rankedQueueData(data, { rankedQueueRecent: "420" }, "recent");
  assert.equal(solo.queueLabel, "单双排");
  assert.equal(solo.mayhem, false);
  assert.equal(solo.positionsApplicable, true);
  assert.equal(solo.positions.length, 1);

  // 后端没下发这个 key 时也不能编名字：标签走 rankedQueueLabel，未知就是空串。
  const missing = rankedQueueData({ rankedQueues: { 420: {} } }, { rankedQueueRecent: "9999" }, "recent");
  assert.equal(missing.queueLabel, "");
  assert.equal(missing.queueId, 9999);

  // 旧载荷（没有 rankedQueues）：海斗页签宁可空着也不复用合并字段。
  const legacy = rankedQueueData({ recentRanked: { games: 3 } }, { rankedQueueRecent: "2300" }, "recent");
  assert.equal(legacy.queueGames, 0);
  assert.equal(legacy.queueLabel, "海克斯大乱斗");
  assert.equal(legacy.mayhem, true);
});

// ---------------------------------------------------------------------------
// 4. 文案：海斗不是排位
// ---------------------------------------------------------------------------

test("R116-E card headings stay truthful once mayhem games are counted", () => {
  const { renderRecentRanked } = compileFunctions(gameplayScript, ["renderRecentRanked"], renderDependencies);
  const solo = renderRecentRanked({ games: 7, wins: 4, losses: 3, positions: [] }, 420, "SWITCH");
  assert.match(solo, /近 7 场排位/);
  assert.match(solo, /位置胜率/);
  assert.doesNotMatch(solo, /海克斯大乱斗/);

  const mayhem = renderRecentRanked({ games: 7, wins: 4, losses: 3, queueLabel: "海克斯大乱斗", positions: [] }, 2300, "SWITCH");
  assert.match(mayhem, /近 7 场海克斯大乱斗/);
  assert.doesNotMatch(mayhem, /场排位/, "a mayhem card must not call itself ranked");
  // R128 §2.3-B：海斗没有分路，这一格整块不渲染——既不写「位置数据不足」，
  // 也不再留一句口径解释占位。
  assert.doesNotMatch(mayhem, /位置数据不足/);
  assert.doesNotMatch(mayhem, /海克斯大乱斗没有分路/);
  assert.doesNotMatch(mayhem, /recent-ranked-positions/);

  const empty = renderRecentRanked({ games: 0 }, 2300, "SWITCH");
  assert.match(empty, /近 0 场海克斯大乱斗/);
  assert.match(empty, /当前样本未发现海克斯大乱斗对局/);
  assert.match(empty, /本赛季已扫描到的海克斯大乱斗对局/);
  // 队列名解析不出来时不许默默写「单双排」。
  const unknown = renderRecentRanked({ games: 0 }, 9999, "SWITCH");
  assert.match(unknown, /当前样本未发现对局/);
  assert.doesNotMatch(unknown, /单双排/);
});

test("R116-E the season champion card no longer calls every game ranked", () => {
  const { renderChampionStats } = compileFunctions(gameplayScript, ["renderChampionStats"], renderDependencies);
  // 放开队列过滤后 seasonChampionStats 里也含海斗场次，兜底文案不能再写「场排位」。
  const fallback = renderChampionStats([{ games: 12 }], {}, { complete: true });
  assert.match(fallback, /12 场对局/);
  assert.doesNotMatch(fallback, /场排位/);
  // 后端给了 message 时照旧直接用（「已统计 N 场」本身不含「排位」二字）。
  const withMessage = renderChampionStats([{ games: 12 }], {}, { season: "S26", message: "已统计 573 场" });
  assert.match(withMessage, /S26 · 已统计 573 场/);
  assert.doesNotMatch(withMessage, /场排位/);
  // 韩服 OP.GG 链路只有排位，那条「N 场排位」是 careerSectionEntries 里造的，
  // 不在本函数内，保持原样。
  assert.match(gameplayScript, /\$\{number\(opggSeason\.overall\.games\)\} 场排位/);
});

// ---------------------------------------------------------------------------
// 5. 后端门禁：海斗绝不回退到混合队列
// ---------------------------------------------------------------------------

test("R116-E the backend refuses to fall back to all-queue position stats", () => {
  // positionStatsForQueue 的早退以前是 `return positionStats(matches, playerRef)`，
  // 那会把混了全部队列的分路数据挂在「海克斯大乱斗」标签下。
  assert.doesNotMatch(gameplayBackend, /if queueID != 420 && queueID != 440 \{\n\t\treturn positionStats\(matches, playerRef\)/);
  const positionGate = stripGoComments(functionSourceGo(gameplayBackend, "positionStatsForQueue"));
  assert.match(positionGate, /return nil/);
  assert.doesNotMatch(positionGate, /return positionStats\(matches, playerRef\)/);
  // liveRecentPositions 的 nil 是有意的，注释必须写清楚，否则后来人会当成漏改。
  const liveGate = functionSourceGo(gameplayBackend, "liveRecentPositions");
  assert.match(liveGate, /seasonClassicRankedQueue\(queueID\)/);
  assert.match(gameplayBackend, /非 420\/440 一律返回 nil 是\*\*有意的\*\*/);
  // recentRankedSummaryForQueue 的早退必须按队列过滤，不能退回混合汇总。
  const forQueue = stripGoComments(functionSourceGo(gameplayBackend, "recentRankedSummaryForQueue"));
  assert.match(forQueue, /recentRankedSummaryForQueues\(matches, playerRef, cached, \[\]int64\{queueID\}/);
  assert.doesNotMatch(forQueue, /return recentRankedSummary\(matches, playerRef, cached\)/);
  // 海斗是 ARAM 系：后端据此不下发位置与能力雷达。
  assert.match(gameplayBackend, /func \(tab gameplayRankedQueueTab\) gameplayRankedTabHasPositions\(\) bool/);
  assert.match(gameplayBackend, /len\(tab\.QueueIDs\) == 1 && !seasonMayhemQueue\(tab\.QueueIDs\[0\]\)/);
});

// stripGoComments 去掉 Go 的 // 行注释：门禁的注释里刻意抄了一遍被禁止的写法
// （解释「以前是什么、为什么不能这样」），不剔掉就会自己咬自己。
function stripGoComments(source) {
  return source.split("\n").map((line) => {
    const position = line.indexOf("//");
    return position >= 0 ? line.slice(0, position) : line;
  }).join("\n");
}

// functionSourceGo 按 Go 的括号配平切出一个函数的源码，供上面那几条门禁断言用。
function functionSourceGo(source, name) {
  const start = source.indexOf(`func ${name}(`);
  assert.notEqual(start, -1, `${name} not found in gameplay.go`);
  const bodyStart = source.indexOf("{", source.indexOf(")", start));
  let depth = 0;
  for (let index = bodyStart; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    else if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) return source.slice(start, index + 1);
    }
  }
  assert.fail(`${name} body not closed`);
  return "";
}

// ---------------------------------------------------------------------------
// 6. P2-6：英雄详情页「构筑」tab 的个人海斗出装静态查询
// ---------------------------------------------------------------------------

test("R116-E personal mayhem builds render only when the backend says they are available", () => {
  const state = { selected: { championId: 157 }, mayhemPersonalBuildsKey: 157, mayhemPersonalBuilds: null };
  const { renderMayhemPersonalBuilds } = compileFunctions(championsScript, ["renderMayhemPersonalBuilds"], { state, escapeHTML, percent: (value) => `${value}%`, compactNumber: (value) => String(value) });

  // 拿不到 / 样本不足 / 英雄不匹配：整块不进 DOM。
  for (const payload of [
    null,
    { available: false, reason: "样本不足" },
    { available: true, groups: [] },
  ]) {
    state.mayhemPersonalBuilds = payload;
    assert.equal(renderMayhemPersonalBuilds({}), "", `payload ${JSON.stringify(payload)} must render nothing`);
  }
  // 缓存属于别的英雄时也不能拿来渲染。
  state.mayhemPersonalBuilds = { available: true, sampleGames: 30, minimumSample: 10, groups: [{ augmentId: 5001, games: 12, wins: 7, winRate: 58, combos: [{ itemIds: [3031, 6672], games: 12, wins: 7, winRate: 58, share: 100 }] }] };
  state.mayhemPersonalBuildsKey = 222;
  assert.equal(renderMayhemPersonalBuilds({}), "", "another champion's samples must not be shown here");
  state.mayhemPersonalBuildsKey = 157;

  const detail = {
    recommendedAugments: [{ assets: [{ id: 5001, name: "海克斯之刃" }] }],
    itemRanking: [{ assets: [{ id: 3031, name: "无尽之刃" }] }],
  };
  const markup = renderMayhemPersonalBuilds(detail);
  assert.match(markup, /data-mayhem-personal-builds/);
  assert.match(markup, /我的海斗出装/);
  // 后端没给 itemNames 时回落到详情页自带的装备行（3031 → 无尽之刃），
  // 目录里也没有的（6672）才显示 ID——三种来源都不猜。
  assert.match(markup, /海克斯之刃 → 无尽之刃 · 装备 6672/);
  assert.match(markup, /<dt>场次<\/dt><dd>12<\/dd>/);
  assert.match(markup, /<dt>胜率<\/dt><dd class="metric-win">58%<\/dd>/);
  // R128 §2.3-B：计数上的口径 tooltip 已删除，只留「本人 N 场」这个事实计数。
  assert.doesNotMatch(markup, /本机保存的本赛季海克斯大乱斗对局/);
  assert.doesNotMatch(markup, /不是全服数据，也不含其他账号/);
  assert.match(markup, /本人 30 场/);

  // 后端直接给了名字时优先用它（覆盖全目录，不止本英雄那一百来件）。
  state.mayhemPersonalBuilds.groups[0].combos[0].itemNames = ["无尽之刃", "破败王者之刃"];
  assert.match(renderMayhemPersonalBuilds(detail), /海克斯之刃 → 无尽之刃 · 破败王者之刃/);
  // 名字只给了一半时整组回落到既有来源：一半有名一半是 ID 更容易被误读成
  // 「这几件才是核心」，所以后端那句「要么全给要么不给」在前端也要守住。
  state.mayhemPersonalBuilds.groups[0].combos[0].itemNames = ["无尽之刃"];
  assert.match(renderMayhemPersonalBuilds(detail), /海克斯之刃 → 无尽之刃 · 装备 6672/);
  // 详情页目录也没有时，只剩 ID，不编名字。
  assert.match(renderMayhemPersonalBuilds({ recommendedAugments: [], itemRanking: [] }), /海克斯 5001 → 装备 3031 · 装备 6672/);
  // 海克斯名解析不出来时显示 ID，不编名字。
  state.mayhemPersonalBuilds.groups[0].augmentId = 9999;
  assert.match(renderMayhemPersonalBuilds({ recommendedAugments: [], itemRanking: [] }), /海克斯 9999 →/);
});

test("R116-E the build tab keeps its three existing blocks and appends the personal query", () => {
  const { mayhemBuildTabMarkup } = compileFunctions(championsScript, ["mayhemBuildTabMarkup"], {
    state: { selected: { championId: 157 }, mayhemPersonalBuildsKey: 0, mayhemPersonalBuilds: null },
    escapeHTML,
    percent: (value) => `${value}%`,
    compactNumber: (value) => String(value),
    renderMayhemItemRanking: () => "ITEM-RANKING",
    renderMayhemItemRoutes: () => "ITEM-ROUTES",
    renderMayhemSkillPlan: () => "SKILL-PLAN",
  });
  const markup = mayhemBuildTabMarkup({ itemRanking: [], build: {} });
  // 既有三段一字未动、顺序未变。
  assert.equal(markup.indexOf("ITEM-RANKING") < markup.indexOf("ITEM-ROUTES"), true);
  assert.equal(markup.indexOf("ITEM-ROUTES") < markup.indexOf("SKILL-PLAN"), true);
  // 只在末尾追加个人查询的挂载点；没有样本时它是空的（输出与改动前一致）。
  assert.match(markup, /SKILL-PLAN<div data-mayhem-personal-builds-host><\/div>$/);
});

test("R116-E the personal build query is lazy-loaded and wired end to end", () => {
  // 懒加载挂在渲染后钩子上，与既有的 mayhem-rarity 同一套口径：
  // 不在渲染函数里发请求，也不为「用户从没点开构筑 tab」白花一次本地读盘。
  assert.match(championsScript, /data-mayhem-personal-builds-host\]"\) && !state\.mayhemPersonalBuildsLoading\) void loadMayhemPersonalBuilds\(/);
  const loader = functionSource(championsScript, "loadMayhemPersonalBuilds");
  assert.match(loader, /\/api\/gameplay\/season-mayhem-builds\?championId=/);
  // 同一个英雄只拉一次；失败也记住 key，避免每次重渲染都重打一次。
  assert.match(loader, /state\.mayhemPersonalBuildsLoading \|\| state\.mayhemPersonalBuildsKey === key\) return/);
  // 后端处理器与路由都在（跨层接线检查）。
  assert.match(gameplayBackend, /func \(a \*app\) handleGameplaySeasonMayhemBuilds\(/);
  assert.match(backendMain, /mux\.HandleFunc\("GET \/api\/gameplay\/season-mayhem-builds", a\.authorized\(a\.handleGameplaySeasonMayhemBuilds\)\)/);
  // 换英雄 / 重置时必须清掉上一个英雄的样本，不能串号。
  assert.match(championsScript, /state\.mayhemPersonalBuildsKey = 0;/);
});

// ---------------------------------------------------------------------------
// 7. 零新增 CSS：R117 的样式棘轮三个预算都卡在实测上限，本块必须复用既有类名
// ---------------------------------------------------------------------------

test("R116-E adds no new CSS class of its own", () => {
  const championsStyles = fs.readFileSync(path.join(__dirname, "champions.css"), "utf8");
  const gameplayStyles = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");
  // 个人海斗出装块复用「装备排行」那套网格，所以它的类名必须都已经在 CSS 里。
  for (const name of ["recommendation-section", "mayhem-ranking-list", "section-count", "arena-section-icon", "mayhem-item-name-only", "metric-win"]) {
    assert.ok(
      championsStyles.includes(`.${name}`) || gameplayStyles.includes(`.${name}`),
      `${name} must already exist in CSS; R116-E is not allowed to add new spacing values`,
    );
  }
  assert.doesNotMatch(championsStyles, /mayhem-personal-builds\s*\{/, "R116-E must not add a new CSS rule for this block");
  // 海斗页签复用既有的 .ranked-queue-button，也不新增样式。
  assert.match(gameplayStyles, /\.ranked-queue-button/);
  assert.doesNotMatch(gameplayStyles, /ranked-queue-button--mayhem|is-mayhem-queue/);
});
