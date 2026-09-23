// R116-D 前端验证：P1-2/P1-3 的克制与协同行内小条、P1-5 的队伍画像缺口标签、
// 以及「只为本人英雄拉一次 hero-json」这条请求纪律在前端的那一半。
//
// 方法论沿用仓库既有做法（champions.test.cjs / r116b.test.cjs 的 compileFunctions）：
// 按函数名从 gameplay.js 里切出源码、用 Function 注入依赖后单独跑，这样既能钉住
// 渲染结果，又不需要拉起整个应用。
//
// 三处对抗变异里的前端那一处（「没命中就不渲染任何 DOM 节点」退化成「渲染一个
// 占位卡片」）在本文件里用源码替换真跑，断言变异体确实产出了 DOM。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const gameplayScript = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const gameplayStyles = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");

// ---------------------------------------------------------------------------
// 编译工具
// ---------------------------------------------------------------------------

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
  // async function 的 async 前缀必须一起带上，否则切出来的函数体里有 await
  // 就是语法错误（champions.test.cjs 的 functionSource 同样处理了这一条）。
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

function compileFunctions(source, names, dependencies = {}) {
  const bodies = names.map((name) => functionSource(source, name));
  const dependencyNames = Object.keys(dependencies);
  const factory = Function(...dependencyNames, `"use strict";\n${bodies.join("\n")}\nreturn { ${names.join(", ")} };`);
  return factory(...dependencyNames.map((name) => dependencies[name]));
}

const escapeHTML = (value) => String(value ?? "").replace(/[&<>"']/g, (character) => (
  { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[character]
));

// stripLineComments 去掉 // 行注释，用于「源码里不许出现某个字符串」这类断言：
// 注释里刻意写了被禁止的写法（解释为什么不能这么干），不剔掉就会自己咬自己。
function stripLineComments(source) {
  return source.split("\n").map((line) => {
    const position = line.indexOf("//");
    return position >= 0 ? line.slice(0, position) : line;
  }).join("\n");
}

// ---------------------------------------------------------------------------
// P1-2/P1-3/P1-5 的阵容取数：liveRecommendationRoster
// ---------------------------------------------------------------------------

// R116D_MATCHUP_PHASES / R116D_ROSTER_SIDE_LIMIT 是模块作用域的常量，
// compileFunctions 只切函数体，所以要从源码里把真实取值抠出来注入——
// 写死一份副本会让测试和生产各说各话。
function moduleConstant(source, name) {
  const match = source.match(new RegExp(`const ${name} = (\\[[^\\]]*\\]|\\d+);`));
  assert.ok(match, `${name} is not declared as a module constant any more`);
  return JSON.parse(match[1].replace(/'/g, '"'));
}

function rosterDeps(source = gameplayScript) {
  return compileFunctions(source, ["liveRecommendationChampionId", "liveRecommendationRoster"], {
    USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS: true,
    R116D_MATCHUP_PHASES: moduleConstant(source, "R116D_MATCHUP_PHASES"),
    R116D_ROSTER_SIDE_LIMIT: moduleConstant(source, "R116D_ROSTER_SIDE_LIMIT"),
    liveAugmentRecommendationSource: (data) => (String(data?.gameMode || "").toUpperCase() === "KIWI" ? "hextech" : data?.augmentSource || ""),
  });
}

function mayhemLive(overrides = {}) {
  return {
    phase: "InProgress", gameMode: "KIWI", mapId: 12, queueId: 2400,
    players: [
      { isCurrent: true, teamId: 100, championId: 157, championName: "疾风剑豪" },
      { teamId: 100, championId: 4, championName: "卡牌大师" },
      { teamId: 100, championId: 12, championName: "牛头酋长" },
      { teamId: 100, championId: 412, championName: "魂锁典狱长" },
      { teamId: 100, championId: 222, championName: "暴走萝莉" },
      { teamId: 200, championId: 200, championName: "虚空女皇" },
      { teamId: 200, championId: 90, championName: "虚空之眼" },
      { teamId: 200, championId: 7, championName: "诡术妖姬" },
      { teamId: 200, championId: 8, championName: "迅捷斥候" },
      { teamId: 200, championId: 9, championName: "末日使者" },
    ],
    ...overrides,
  };
}

test("R116-D roster sends both sides in game and only allies during champion select", () => {
  const { liveRecommendationRoster } = rosterDeps();

  const inProgress = liveRecommendationRoster(mayhemLive());
  assert.deepEqual(inProgress.allyIds, [4, 12, 157, 222, 412]);
  assert.deepEqual(inProgress.enemyIds, [7, 8, 9, 90, 200]);
  assert.equal(inProgress.matchupAllowed, true);
  assert.equal(inProgress.phase, "InProgress");

  const reconnect = liveRecommendationRoster(mayhemLive({ phase: "Reconnect" }));
  assert.equal(reconnect.matchupAllowed, true, "Reconnect has the full ten players too");
  assert.deepEqual(reconnect.enemyIds, [7, 8, 9, 90, 200]);

  // Anti-scope 第 4 条：ChampSelect 阶段一律不下发敌方 ID。探测未证实
  // theirTeam 非空（docs/r116-probe-findings.md §4.4 仍是「待填」）。
  const champSelect = liveRecommendationRoster(mayhemLive({ phase: "ChampSelect" }));
  assert.equal(champSelect.matchupAllowed, false);
  assert.deepEqual(champSelect.enemyIds, [], "enemy ids must never leave the client during champion select");
  assert.deepEqual(champSelect.allyIds, [4, 12, 157, 222, 412], "our own side is unaffected");

  for (const phase of ["GameStart", "None", "", "ChampSelect"]) {
    assert.equal(liveRecommendationRoster(mayhemLive({ phase })).matchupAllowed, false, `${phase} must not expose the enemy roster`);
  }
});

test("R116-D roster stays empty outside mayhem and for oversized sides", () => {
  const { liveRecommendationRoster } = rosterDeps();

  // 克制/协同来自 hexdata hero-json、画像来自 hexdata postmatch，别的模式后端
  // 根本没有这两份数据，前端也就不该传 roster（传了也只是被后端丢掉）。
  assert.deepEqual(liveRecommendationRoster(mayhemLive({ gameMode: "CLASSIC", mapId: 11 })), {
    phase: "", allyIds: [], enemyIds: [], matchupAllowed: false, signature: "",
  });
  assert.deepEqual(liveRecommendationRoster(mayhemLive({ gameMode: "CHERRY", mapId: 30 })).allyIds, []);
  assert.deepEqual(liveRecommendationRoster({ phase: "InProgress", players: [] }).allyIds, []);

  // 一侧超过 5 人（斗魂 8 人、极地 10 人同队等）→ 整侧留空。后端对超长的 ID 串
  // 一律丢弃，前端提前不传，避免缓存签名白白抖动。
  const eight = Array.from({ length: 8 }, (_, index) => ({ teamId: 100, championId: 10 + index }));
  const oversized = liveRecommendationRoster(mayhemLive({ players: [...eight, { isCurrent: true, teamId: 200, championId: 157 }] }));
  assert.deepEqual(oversized.enemyIds, [], "an oversized side must not be sent at all");
  assert.deepEqual(oversized.allyIds, [157], "the other side is unaffected by an oversized opponent");

  // 未锁定时用 pick intent：ChampSelect 阶段大部分时间 championId 还是 0。
  const pending = liveRecommendationRoster(mayhemLive({
    phase: "ChampSelect",
    players: [
      { isCurrent: true, teamId: 100, championId: 0, championPickIntent: 157 },
      { teamId: 100, championId: 0, championPickIntent: 4 },
      { teamId: 100, championId: 0, championPickIntent: -3 }, // 随机待定，不是英雄
    ],
  }));
  assert.deepEqual(pending.allyIds, [4, 157]);
});

// ---------------------------------------------------------------------------
// 请求纪律：Anti-scope 第 1 条
// ---------------------------------------------------------------------------

test("R116-D sends champion ids only and never fetches per-champion hexdata", () => {
  // 前端不许出现任何按英雄 ID 拼 hexdata 详情页的写法：那是评审 4.2/4.4 里
  // 最危险的一项（10 请求 / 12 MB / 最坏排队 11 秒 / 熔断对取消失效）。
  // 注释里会出现这个路径（说明为什么不能这么拉），所以先把 // 注释剔掉再断言。
  const code = stripLineComments(gameplayScript);
  assert.doesNotMatch(code, /\/api\/hexdata\/heroes\//, "gameplay.js must not address the hexdata hero endpoint in code");
  assert.doesNotMatch(gameplayScript, /hexdata\.com\.cn/, "gameplay.js must not reach the hexdata origin directly");
  // roster 参数只是 ID 列表，三个参数名都要在，且没有第四个「按英雄拉数据」的入口。
  for (const name of ["allyChampionIds", "enemyChampionIds", "liveRecommendationRoster"]) {
    assert.match(gameplayScript, new RegExp(name), `${name} is missing from gameplay.js`);
  }
});

test("R116-D recommendation request carries the roster and refetches when it changes", async () => {
  const target = {
    key: "157:other:KIWI:12:diamond:none", championId: 157, position: "other", queueId: 2400,
    gameMode: "KIWI", mapId: 12, tier: "diamond", gameId: 77, augmentSource: "hextech", self: {},
  };
  const state = {
    live: {}, liveGameGeneration: 0,
    liveRecommendations: new Map(), liveRecommendationTraces: new Map(), liveRecommendationFlights: new Map(),
    liveRecommendationFailures: new Map(), liveRecommendationRosters: new Map(),
  };
  const paths = [];
  const skips = [];
  let roster = { phase: "ChampSelect", allyIds: [4, 157], enemyIds: [], matchupAllowed: false, signature: "ChampSelect|4,157|" };

  const functions = compileFunctions(gameplayScript, ["ensureLiveRecommendations"], {
    state,
    Date: { now: () => 1000 },
    liveRecommendationTarget: () => target,
    liveRecommendationRoster: () => roster,
    renderLive: () => {},
    ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, ensureSpecialistRunes: () => {},
    hasUsableLiveRecommendations: () => true,
    liveRecommendationFlightActive: () => false,
    recordLiveRecommendationSkip: (reason) => skips.push(reason),
    newLiveRecommendationTraceId: () => "trace-1",
    recordItemSetClientDiagnostic: () => {},
    api: async (requestPath) => { paths.push(requestPath); return { recommendations: { source: "hextech-aram" } }; },
    URLSearchParams,
  });

  await functions.ensureLiveRecommendations({});
  assert.equal(paths.length, 1, "one refresh must issue exactly one recommendation request");
  const first = new URL(`http://local${paths[0]}`).searchParams;
  assert.equal(first.get("phase"), "ChampSelect");
  assert.equal(first.get("allyChampionIds"), "4,157");
  assert.equal(first.get("enemyChampionIds"), null, "enemy ids must not be sent during champion select");
  assert.equal(first.get("source"), "mayhem");

  // 阵容没变 → 命中缓存，不再请求。
  await functions.ensureLiveRecommendations({});
  assert.equal(paths.length, 1, "an unchanged roster must not trigger a second request");
  assert.equal(skips.at(-1), "cached");

  // 阵容变了（进入对局、拿到敌方 5 人）→ 必须重新取，否则提示停在旧阵容上。
  roster = { phase: "InProgress", allyIds: [4, 12, 157, 222, 412], enemyIds: [7, 8, 9, 90, 200], matchupAllowed: true, signature: "InProgress|4,12,157,222,412|7,8,9,90,200" };
  await functions.ensureLiveRecommendations({});
  assert.equal(paths.length, 2, "a changed roster must invalidate the cached recommendations");
  const second = new URL(`http://local${paths[1]}`).searchParams;
  assert.equal(second.get("phase"), "InProgress");
  assert.equal(second.get("allyChampionIds"), "4,12,157,222,412");
  assert.equal(second.get("enemyChampionIds"), "7,8,9,90,200");

  await functions.ensureLiveRecommendations({});
  assert.equal(paths.length, 2, "the refreshed roster must be cached again");
});

test("R116-D degrades to no roster params when the roster helper is unavailable", async () => {
  // 拿不到阵容（liveRecommendationRoster 返回 null）时安静降级：既不发 phase /
  // allyChampionIds / enemyChampionIds 三个参数，也不抛错，后端因此一个克制/协同
  // 提示都不生成。R116-B 评审整改（D 账本 §11.7-2）删掉了源码里那道
  // `typeof liveRecommendationRoster === "function"` 护栏——它防的不是生产情况，
  // 而是聚焦测试漏注入依赖；漏注入现在会直接抛 ReferenceError（好事），降级语义
  // 由 liveRecommendationRoster 自己负责，所以下面显式注入「返回 null」。
  const target = { key: "13:middle:CLASSIC:11:diamond:4-12", championId: 13, position: "middle", queueId: 420, gameMode: "CLASSIC", mapId: 11, tier: "diamond", gameId: 77, self: { spell1Id: 4, spell2Id: 12 } };
  const state = {
    live: {}, liveGameGeneration: 0,
    liveRecommendations: new Map(), liveRecommendationTraces: new Map(), liveRecommendationFlights: new Map(),
    liveRecommendationFailures: new Map(),
  };
  const paths = [];
  const functions = compileFunctions(gameplayScript, ["ensureLiveRecommendations"], {
    state,
    Date: { now: () => 1000 },
    liveRecommendationTarget: () => target,
    // 评审整改（D 账本 §11.7-2）：源码里的 typeof 护栏已删，漏注入只会抛
    // ReferenceError，所以「拿不到阵容」这一支必须显式注入成返回 null。
    liveRecommendationRoster: () => null,
    renderLive: () => {},
    ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, ensureSpecialistRunes: () => {},
    hasUsableLiveRecommendations: () => true,
    liveRecommendationFlightActive: () => false,
    recordLiveRecommendationSkip: () => {},
    newLiveRecommendationTraceId: () => "trace-2",
    recordItemSetClientDiagnostic: () => {},
    api: async (requestPath) => { paths.push(requestPath); return { recommendations: { source: "opgg" } }; },
    URLSearchParams,
  });
  await functions.ensureLiveRecommendations({});
  assert.equal(paths.length, 1);
  const query = new URL(`http://local${paths[0]}`).searchParams;
  for (const name of ["phase", "allyChampionIds", "enemyChampionIds"]) {
    assert.equal(query.get(name), null, `${name} must not be sent when the roster is unknown`);
  }
  assert.match(paths[0], /[?&]tier=diamond(?:&|$)/);
  // 缓存 key 的格式没有被改动（champions.test.cjs 钉死了它）。
  assert.match(gameplayScript, /key: `\$\{championId\}:\$\{position\}:\$\{gameMode\}:\$\{mapId\}:\$\{tier\}:\$\{spellKey\}`/);
});

// ---------------------------------------------------------------------------
// 渲染：行内小条与队伍画像标签
// ---------------------------------------------------------------------------

function insightDeps(payload, overrides = {}, source = gameplayScript) {
  // R116-B 评审整改（清 D 账本 §11.7-1 的技术债）：七个渲染 helper 已从
  // renderLiveInsights 的函数体内提到模块作用域，所以这里要把它们一起编译进来
  // （原先嵌套在体内时 functionSource 按大括号配对会一并切到，不需要列名字）。
  const compiled = compileFunctions(source, [
    "renderLiveInsights", "renderLiveRosterNoticeBars", "renderLiveTeamPortraitTags",
    "liveRosterChampionName", "liveNoticeBar", "liveEvidenceSuffix", "liveConfidenceText", "liveDeltaPoints",
  ], {
    state: { settings: { detailPositionAlign: false, liveOrder: "team" } },
    // R128 §2.3：统计口径说明已从 UI 删除，不再需要 deepLegendsShared 桩。
    window: {},
    isARAMRelatedMatch: () => false,
    liveAugmentRecommendationSource: () => "hextech",
    orderLivePlayers: (players) => players,
    clusterPremadePlayers: (players) => players,
    livePremadeRoster: () => [],
    arenaLivePlayerGroups: () => [],
    renderLiveRecentPositions: () => "",
    renderLivePlayer: (player) => `<player data-id="${player.championId}"></player>`,
    renderInsightMatches: () => "",
    recordLiveRosterRendered: () => {},
    insightTeamLayout: (players) => ({
      teams: new Map([100, 200].map((teamID) => [teamID, players.filter((player) => Number(player.teamId) === teamID)])),
      aligned: false, reason: "",
    }),
    escapeHTML,
    ...overrides,
  });
  // renderLiveInsights(data, payload)：payload 是第二个参数，这里把它绑定好，
  // 调用方只传 data，和 champions.test.cjs 的单参数调用形式保持可比。
  return (data) => compiled.renderLiveInsights(data, payload);
}

const insightPlayers = [
  { isCurrent: true, teamId: 100, championId: 157, championName: "疾风剑豪" },
  { teamId: 100, championId: 4, championName: "卡牌大师" },
  { teamId: 200, championId: 200, championName: "虚空女皇" },
  { teamId: 200, championId: 90, championName: "虚空之眼" },
];

test("R116-D renders inline bars for hits and nothing at all for misses", () => {
  const payload = {
    matchupNotices: [
      { direction: "weak", opponentChampionId: 200, counterDelta: 0.03639, evidence: "supported", confidenceLow: 0.525157, confidenceHigh: 0.535102 },
      { direction: "strong", opponentChampionId: 90, counterDelta: 0.033973, evidence: "supported", confidenceLow: 0.597728, confidenceHigh: 0.603732 },
    ],
    synergyNotices: [
      { teammateChampionId: 4, synergyDelta: 0.078605, evidence: "supported", confidenceLow: 0.638803, confidenceHigh: 0.644849 },
    ],
  };
  const markup = insightDeps(payload)(mayhemLive({ players: insightPlayers }));
  assert.match(markup, /class="roster-notice-strip"/);
  assert.equal((markup.match(/class="roster-notice /g) || []).length, 3, "two matchups plus one synergy");
  assert.match(markup, /roster-notice is-weak/);
  assert.match(markup, /roster-notice is-strong/);
  assert.match(markup, /roster-notice is-synergy/);
  // counterDelta 是上游 0..1 原值，展示成百分点；符号由 direction 决定。
  assert.match(markup, /被 <b>虚空女皇<\/b> 克制 · 胜率 −3\.6 个百分点/);
  assert.match(markup, /克制 <b>虚空之眼<\/b> · 胜率 \+3\.4 个百分点/);
  assert.match(markup, /与 <b>卡牌大师<\/b> 协同 · 胜率 \+7\.9 个百分点/);
  // evidence / confidenceLow / confidenceHigh 必须可见（评审 6.5）。
  assert.match(markup, /上游置信区间 52\.5%–53\.5%/);
  assert.match(markup, /上游置信区间 63\.9%–64\.5%/);
  // 绝不做「阵容协同 +X%」总分（Anti-scope 第 2 条）。
  assert.doesNotMatch(markup, /阵容协同\s*\+/, "no aggregate synergy score is allowed");

  // 没命中 → 一个相关节点都不渲染，也不出现「暂无数据」占位卡片。
  for (const empty of [{}, { matchupNotices: [], synergyNotices: [] }, { matchupNotices: null, synergyNotices: null }]) {
    const blank = insightDeps(empty)(mayhemLive({ players: insightPlayers }));
    for (const needle of ["roster-notice", "live-roster-supplement", "team-portrait", "暂无数据", "暂无克制", "undefined", "NaN"]) {
      assert.doesNotMatch(blank, new RegExp(needle), `empty payload leaked "${needle}" into the DOM`);
    }
  }

  // 后端给出了 notice、但客户端认不出这个英雄名 → 丢弃该条，不渲染裸 ID。
  const unknown = insightDeps({ matchupNotices: [{ direction: "weak", opponentChampionId: 999, counterDelta: 0.03, evidence: "supported" }] })(mayhemLive({ players: insightPlayers }));
  assert.doesNotMatch(unknown, /roster-notice/, "an unresolvable champion id must not be rendered");
  assert.doesNotMatch(unknown, /999/);
});

test("R116-D degrades notices whose evidence is not supported", () => {
  const payload = {
    matchupNotices: [{ direction: "weak", opponentChampionId: 200, counterDelta: 0.03639, evidence: "underpowered", confidenceLow: 0.5, confidenceHigh: 0.54 }],
    synergyNotices: [{ teammateChampionId: 4, synergyDelta: 0.07, evidence: "", confidenceLow: 0, confidenceHigh: 0 }],
  };
  const markup = insightDeps(payload)(mayhemLive({ players: insightPlayers }));
  assert.match(markup, /roster-notice is-unverified/);
  assert.doesNotMatch(markup, /roster-notice is-weak/, "a non-significant row must not wear the confident weak tone");
  // 未达显著性时不许把效果量当结论展示。
  assert.doesNotMatch(markup, /3\.6 个百分点/);
  assert.match(markup, /上游证据等级：underpowered（未达显著性）/);
  assert.match(markup, /上游未标注证据等级/);
});

test("R116-D renders team portrait tags in our own header only", () => {
  const portrait = {
    labels: [{ key: "frontline", label: "缺前排" }, { key: "crowdControl", label: "缺控制" }],
    rosterSize: 5, resolvedHeroes: 5, heroPoolSize: 173,
    measurementTechnique: "队伍画像门槛是 173 位英雄赛后每局均值的算术平均；这是本地约定的启发式判据，不是上游官方口径",
  };
  const markup = insightDeps({ teamPortrait: portrait })(mayhemLive({ players: insightPlayers }));
  assert.match(markup, /class="team-portrait-tags"/);
  assert.equal((markup.match(/class="team-portrait-tag"/g) || []).length, 2);
  assert.match(markup, /data-portrait-gap="frontline">缺前排</);
  assert.match(markup, /data-portrait-gap="crowdControl">缺控制</);
  // 标签只挂在我方队伍头部，不能挂到对方那一栏。
  const ourHeader = markup.slice(markup.indexOf("<h3>我方</h3>"), markup.indexOf("<h3>对方</h3>"));
  assert.match(ourHeader, /team-portrait-tags/);
  assert.doesNotMatch(markup.slice(markup.indexOf("<h3>对方</h3>")), /team-portrait-tags/);
  // R128 §2.3：队伍画像的口径说明不再进 UI（撤销 R116-D 的可见披露要求）；
  // tooltip 只留可核对的入队人数，缺口标签本身保留。
  assert.doesNotMatch(markup, /mayhem-measurement/);
  assert.doesNotMatch(markup, /本地约定的启发式判据/);
  assert.match(markup, /data-tooltip="本队 5 位英雄进入统计"/);

  // 阵容均衡 → labels 为空 → 一个标签节点都不渲染（不是渲染一个空标签）。
  for (const balanced of [{ teamPortrait: { labels: [], resolvedHeroes: 5 } }, { teamPortrait: null }, {}]) {
    const blank = insightDeps(balanced)(mayhemLive({ players: insightPlayers }));
    assert.doesNotMatch(blank, /team-portrait/, "a balanced comp must not render portrait markup");
    assert.doesNotMatch(blank, /mayhem-measurement/);
  }
  // 后端给了空文案的标签也不许渲染成空壳。
  const emptyLabel = insightDeps({ teamPortrait: { labels: [{ key: "frontline", label: "  " }] } })(mayhemLive({ players: insightPlayers }));
  assert.doesNotMatch(emptyLabel, /team-portrait/);
});

test("R116-D insight rendering is unchanged when no payload is passed", () => {
  // champions.test.cjs 用单参数调用本函数；payload 为 undefined 时必须与今天完全一致。
  const markup = insightDeps(undefined)(mayhemLive({ players: insightPlayers }));
  assert.match(markup, /<h3>我方<\/h3>/);
  assert.match(markup, /<h3>对方<\/h3>/);
  assert.equal((markup.match(/class="live-team /g) || []).length, 2);
  for (const needle of ["roster-notice", "live-roster-supplement", "team-portrait", "mayhem-measurement", "undefined"]) {
    assert.doesNotMatch(markup, new RegExp(needle), `a missing payload leaked "${needle}"`);
  }
});

// 对抗变异（前端那一处）：把「没命中就不渲染」改成「没命中也渲染一个占位小条」，
// 上面那条空 payload 断言必须能抓到。这里用源码替换真跑一次变异体。
// 锚点的缩进在 R116-B 评审整改后从 6 空格变成 4 空格：那七个 helper 已经从
// renderLiveInsights 的函数体内提到模块作用域（D 账本 §11.7-1），early return 跟着
// 搬进了 renderLiveRosterNoticeBars。
test("R116-D placeholder mutation is detected by the empty-payload assertions", () => {
  const anchor = '    if (!bars.length) return "";';
  assert.ok(gameplayScript.includes(anchor), "the no-hit early return moved; update this mutation harness");
  const mutated = gameplayScript.replace(anchor, '    if (!bars.length) return `<div class="roster-notice-strip"><span class="roster-notice is-unverified">暂无克制数据</span></div>`;');
  assert.notEqual(mutated, gameplayScript);
  const markup = insightDeps({})(mayhemLive({ players: insightPlayers }));
  // 用变异后的源码重新编译同一个函数，跑同一份空 payload。
  const mutatedMarkup = insightDeps({}, {}, mutated)(mayhemLive({ players: insightPlayers }));
  assert.match(mutatedMarkup, /roster-notice/, "the mutant must actually produce a placeholder node, otherwise this harness proves nothing");
  // 生产实现同场景必须是干净的——两条断言合起来才说明「空态退化」可被检测。
  assert.doesNotMatch(markup, /roster-notice/);
});

// ---------------------------------------------------------------------------
// P1-4 阶段二：前端也不许接线
// ---------------------------------------------------------------------------

test("R116-D stage two stays out of the client until the probe verdict lands", () => {
  for (const needle of ["nextItem", "terminalItemTrios", "下一步出装", "下一件推荐"]) {
    assert.ok(!gameplayScript.includes(needle), `gameplay.js must not render ${needle} before docs/r116-probe-findings.md §3.4 is filled in`);
  }
  // 阶段一的诊断事件是后端埋点，前端不参与，也不该出现任何 UI 文案。
  assert.ok(!gameplayScript.includes("live_client_items_parsed"), "stage one is backend diagnostics only");
});

// ---------------------------------------------------------------------------
// 样式纪律
// ---------------------------------------------------------------------------

test("R116-D styles reuse tokens and existing values only", () => {
  const start = gameplayStyles.indexOf(".live-roster-supplement {");
  assert.notEqual(start, -1, "the R116-D style block is missing");
  const block = gameplayStyles.slice(start);
  // 不许写死新 hex 色值（R117 的样式棘轮盯着重复 hex 的预算，已经顶格）。
  assert.doesNotMatch(block, /#[0-9A-Fa-f]{3,8}\b/, "R116-D styles must not hard-code hex colors");
  // 每个用到的变量都必须是既有变量。
  const declared = new Set([...gameplayStyles.matchAll(/--([\w-]+)\s*:/g)].map((match) => match[1]));
  const appStyles = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
  for (const match of appStyles.matchAll(/--([\w-]+)\s*:/g)) declared.add(match[1]);
  for (const name of new Set([...block.matchAll(/var\(--([\w-]+)/g)].map((match) => match[1]))) {
    assert.ok(declared.has(name), `R116-D references an undefined CSS variable --${name}`);
  }
  // 新类名必须在源码里以字面量出现，否则会抬高「CSS 里定义但源码从未引用」的预算。
  for (const name of ["live-roster-supplement", "roster-notice-strip", "roster-notice", "is-weak", "is-strong", "is-synergy", "is-unverified", "team-portrait-tags", "team-portrait-tag"]) {
    assert.ok(gameplayScript.includes(name), `${name} is styled but never referenced literally in gameplay.js`);
    assert.ok(block.includes(`.${name}`) || gameplayStyles.includes(`.${name}`), `${name} has no style rule`);
  }
});
