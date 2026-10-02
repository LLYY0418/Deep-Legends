"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const source = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const cssSource = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");
const demoSource = fs.readFileSync(path.join(__dirname, "demo-data.js"), "utf8");

test("flow render diagnostics follow displayed data and explain skipped mounts", () => {
  const reports = [];
  const root = { innerHTML: "", querySelectorAll: () => [], querySelector: () => null };
  const container = { dataset: { matchTierScope: "scope" }, querySelector: () => root };
  const tab = { region: "kr", currentGameTrace: "cg-1234567890123-2", currentGame: { traceId: "cg-1234567890123-1", forceRefresh: true, data: { status: "none" } } };
  const { updateCurrentGameCard } = compile(["updateCurrentGameCard"], {
    window: { reportFlowDiagnostic: (...args) => reports.push(args) },
    overviewContainer: () => container, matchTierScope: () => "scope",
    currentGameMarkup: () => "", prepareImages: () => {},
  });
  updateCurrentGameCard(tab);
  assert.equal(reports.at(-1)[1], "rendered");
  assert.equal(reports.at(-1)[2].traceId, tab.currentGame.traceId);
  assert.equal(reports.at(-1)[2].forceRefresh, true);
  assert.equal(root.hidden, true);
  container.dataset.matchTierScope = "other";
  updateCurrentGameCard(tab);
  assert.equal(reports.at(-1)[1], "render-scope-mismatch");
  container.querySelector = () => null;
  updateCurrentGameCard(tab);
  assert.equal(reports.at(-1)[1], "render-no-root");
  assert.ok(reports.every(report => report[2].forceRefresh === true));
});

test("flow loader logs invalid response without inventing an idle state", async () => {
  const reports = [];
  const tab = { region: "kr", data: { player: { playerRef: "fixture" } } };
  const { loadOverviewCurrentGame } = compile(["loadOverviewCurrentGame"], {
    window: { reportFlowDiagnostic: (...args) => reports.push(args) },
    state: {}, api: async () => ({ status: "active", teams: null }), updateCurrentGameCard: () => {},
  });
  await loadOverviewCurrentGame(tab);
  assert.deepEqual(reports.map(report => report[1]), ["request", "invalid-response", "failed"]);
  assert.equal(tab.currentGame.error, true);
  assert.equal(tab.currentGame.data, undefined);
});

test("2351 failed current game probe does not render a misleading status row", () => {
  const { currentGameMarkup } = compile(["currentGameMarkup"]);
  const tab = { region: "kr", data: { player: { playerRef: "player" } }, currentGame: { ref: "player", error: true } };
  assert.equal(currentGameMarkup(tab), "");
  assert.equal(tab.currentGame.error, true, "probe failure must remain distinct from idle");
  tab.currentGame = { ref: "player", data: { status: "none" } };
  assert.equal(currentGameMarkup(tab), "");
});

test("manual roster retry reaches the backend and diagnostics without exporting identity", async () => {
  const reports = [], requests = [];
  const tab = { region: "kr", key: "friend", data: { player: { playerRef: "sensitive-player" } }, currentGame: { ref: "sensitive-player", at: Date.now() } };
  const { loadOverviewCurrentGame } = compile(["loadOverviewCurrentGame"], {
    window: { reportFlowDiagnostic: (...args) => reports.push(args) }, state: {}, updateCurrentGameCard: () => {},
    api: async (_url, options) => { requests.push(JSON.parse(options.body)); return { status: "none", source: "OP.GG" }; },
  });
  await loadOverviewCurrentGame(tab);
  assert.equal(requests.length, 0);
  assert.equal(reports.at(-1)[1], "cached");
  await loadOverviewCurrentGame(tab, true);
  assert.equal(requests.length, 1);
  assert.equal(requests[0].forceRefresh, true);
  assert.equal(reports.at(-1)[2].source, "OP.GG");
  assert.equal(reports.at(-1)[2].teamsReceived, 0);
  assert.ok(reports.filter(report => report[1] !== "cached").every(report => report[2].forceRefresh === true));
  assert.equal(tab.currentGame.forceRefresh, true);
  assert.doesNotMatch(JSON.stringify(reports), /sensitive-player/);
});

test("2326 manual and automatic current-game failures retain their own trigger", async () => {
  for (const force of [true, false]) {
    const reports = [];
    const tab = { region: "kr", key: "friend", data: { player: { playerRef: "private-reference" } } };
    const { loadOverviewCurrentGame } = compile(["loadOverviewCurrentGame"], {
      window: { reportFlowDiagnostic: (...args) => reports.push(args) }, state: {}, updateCurrentGameCard: () => {},
      api: async () => ({ status: "active", teams: null }),
    });
    await loadOverviewCurrentGame(tab, force);
    assert.deepEqual(reports.map(report => report[1]), ["request", "invalid-response", "failed"]);
    assert.ok(reports.every(report => report[2].forceRefresh === force));
    assert.equal(tab.currentGame.forceRefresh, force);
    assert.doesNotMatch(JSON.stringify(reports), /private-reference/);
  }
});




function functionSource(script, name) {
  const marker = `function ${name}(`;
  const markerStart = script.indexOf(marker);
  const asyncStart = script.lastIndexOf("async ", markerStart);
  const start = asyncStart >= 0 && asyncStart + 6 === markerStart ? asyncStart : markerStart;
  assert.notEqual(start, -1, `missing ${name}`);
  const bodyStart = script.indexOf("{", start);
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

// R128 §2.3：统计口径说明已从 UI 删除，shared.js 随之移除；聚焦测试不再需要
// window.deepLegendsShared 桩。

function compile(names, dependencies = {}, script = source) {
  names = [...names];
  for (const name of ["proBadgeAttributes", "renderProIdentityBadge", "proContextFromButton"]) if (!dependencies[name] && !names.includes(name) && names.some(n => functionSource(script, n).includes(name + "("))) names.push(name);
  if (!names.includes("overviewSupplementTarget") && names.some(name => functionSource(script, name).includes("overviewSupplementTarget("))) names.push("overviewSupplementTarget");
  // R116-B 评审整改（清 R116-D 账本 §11.7-1 的技术债）：renderLiveInsights 的七个
  // 渲染 helper 已从函数体内提到模块作用域，这里按名字传递编译真实实现（顺序＝先
  // 调用方后被调用方，names 是边扫边追加的），而不是给每个聚焦测试塞一份桩。
  for (const name of ["renderLiveRosterNoticeBars", "renderLiveTeamPortraitTags", "liveRosterChampionName", "liveNoticeBar", "liveEvidenceSuffix", "liveConfidenceText", "liveDeltaPoints"]) if (!dependencies[name] && !names.includes(name) && names.some(n => functionSource(script, n).includes(name + "("))) names.push(name);
  // R129 P1：历史状态判据抽成 liveHistoryStateOf / liveHistorySettled，编译
  // renderLivePlayer / renderInsightMatches 时按名字带上真实实现，不塞桩。
  for (const name of ["liveHistoryStateOf", "liveHistorySettled"]) if (!dependencies[name] && !names.includes(name) && names.some(n => functionSource(script, n).includes(name + "("))) names.push(name);
  for (const name of ["recommendedRuneSpellIDs", "renderRuneSpellPair", "renderRuneEquipment", "retainRuneStarterItems"]) if (!dependencies[name] && !names.includes(name) && names.some(n => functionSource(script, n).includes(name + "("))) names.push(name);
  for (let length = -1; length !== names.length;) {
    length = names.length;
    for (const name of ["bindLiveNode", "champSelectEnemyPlaceholder", "stampLiveRows", "preserveLiveImages", "patchLiveRosterPanel", "liveClientPositionsPending", "clearRuneStarterRetries", "runeStarterTargetActive", "laneMatchupEnemies", "laneMatchupTier", "laneMatchupInference", "laneMatchupAvailability", "ensureLaneMatchupPositions", "laneMatchupPairKey", "laneMatchupOwnLocked", "ensureLaneMatchupPair", "recordLaneMatchupCandidateSkip", "laneMatchupUnavailableReason", "recordLaneMatchupCardDiagnostic", "liveRecommendationTier"]) if (!dependencies[name] && !names.includes(name) && names.some(n => functionSource(script, n).includes(name + "("))) names.push(name);
  }
  dependencies = { readSetting: (_key, fallback) => fallback, recordItemSetClientDiagnostic: () => {}, document: { hidden: false }, setTimeout, clearTimeout, riotTab: tab => tab?.region === "kr", isARAMRelatedMatch: () => false, isSummonersRiftMatch: data => Number(data?.mapId) === 11, clusterPremadePlayers: players => players, window: {}, ...dependencies };
  const keys = Object.keys(dependencies);
  return Function(...keys, `"use strict";\n${names.map((name) => functionSource(script, name)).join("\n")}\nreturn {${names.join(",")}};`)(...keys.map((key) => dependencies[key]));
}


test("R68 specialist request uses OPGG-resolved position unless the user overrides it", async () => {
  let target = { championId: 222, position: "top", clientPosition: "top", positionOverride: "", gameMode: "CLASSIC", mapId: 11, tier: "emerald_plus", spellKey: "4-7", key: "old" };
  const payload = { resolvedPosition: "adc", positionSource: "opgg-primary", runes: {} };
  const state = { specialistRunes: new Map(), specialistRuneFailures: new Map(), specialistRuneFlights: new Map(), liveGameGeneration: 1, live: {} };
  const apiCalls = [];
  const functions = compile(["specialistPosition", "specialistRequestTarget", "ensureSpecialistRunes"], {
    liveRecommendationTarget: () => target,
    liveRecommendationsFor: () => payload,
    recommendationQueueHasTopPlayers: () => true,
    recordSpecialistRuneClientSkip: () => {},
    specialistRuneFlightActive: () => false,
    specialistRuneFailure: () => null,
    state,
    renderLive: () => {},
    api: async (url) => { apiCalls.push(url); return []; },
    ensurePerks: () => {},
  });
  assert.equal(functions.specialistPosition({}, target), "adc");
  await functions.ensureSpecialistRunes({});
  assert.match(apiCalls[0], /championId=222&position=adc/);
  assert.equal(functions.specialistRequestTarget({}).key, "222:adc");

  target = { ...target, positionOverride: "mid" };
  state.specialistRunes.clear();
  state.specialistRuneFlights.clear();
  await functions.ensureSpecialistRunes({});
  assert.match(apiCalls[1], /position=mid/);
});

test("R68 live insight labels are relative to the current player's absolute team", () => {
  const state = { settings: { liveOrder: "team", detailPositionAlign: false } };
  const recorded = [];
  const functions = compile(["orderLivePlayers", "insightTeamLayout", "renderLiveRecentPositions", "renderLiveInsights"], {
    state,
    liveAugmentRecommendationSource: () => "",
    clusterPremadePlayers: (players) => players,
    recordLiveRosterRendered: (_data, one, two) => recorded.push([one, two]),
    renderLivePlayer: (player) => `<article>${player.isCurrent ? '<span class="self-chip">自己</span>' : ""}${player.name}</article>`,
    renderInsightMatches: () => "<div></div>",
    escapeHTML: (value) => String(value),
    arenaLivePlayerGroups: () => [],
  });
  const data = { players: [{ name: "foe", teamId: 100 }, { name: "me", teamId: 200, isCurrent: true }, { name: "ally", teamId: 200 }] };
  const html = functions.renderLiveInsights(data);
  const own = html.slice(html.indexOf("<h3>我方</h3>"), html.indexOf("<h3>对方</h3>"));
  const foe = html.slice(html.indexOf("<h3>对方</h3>"));
  assert.match(own, /self-chip/);
  assert.doesNotMatch(foe, /self-chip/);
  assert.deepEqual(recorded, [[1, 2]], "render diagnostic must retain absolute 100/200 counts");

  const team100 = functions.renderLiveInsights({ players: [{ name: "me", teamId: 100, isCurrent: true }, { name: "foe", teamId: 200 }] });
  assert.match(team100.slice(team100.indexOf("<h3>我方</h3>"), team100.indexOf("<h3>对方</h3>")), /self-chip/);
  assert.match(functions.renderLiveInsights({ players: [{ name: "blue", teamId: 100 }, { name: "red", teamId: 200 }] }), /无法确定你所在阵营/);
});

test("live insight sorts both teams by known positions while retaining duplicate order", () => {
  const { insightTeamLayout } = compile(["insightTeamLayout"]);
  const positions = ["utility", "bottom", "middle", "jungle", "top"];
  const players = [100, 200].flatMap((teamId) => positions.map((position) => ({ teamId, position })));
  const aligned = insightTeamLayout(players, true);
  assert.equal(aligned.aligned, true);
  assert.deepEqual(aligned.teams.get(100).map((player) => player.position), ["top", "jungle", "middle", "bottom", "utility"]);
  const duplicate = [
    { teamId: 200, position: "middle", name: "mid-1" },
    { teamId: 200, position: "jungle", name: "jungle-1" },
    { teamId: 200, position: "top", name: "top" },
    { teamId: 200, position: "jungle", name: "jungle-2" },
    { teamId: 200, position: "middle", name: "mid-2" },
  ];
  assert.deepEqual(insightTeamLayout(duplicate, true).teams.get(200).map((player) => player.name),
    ["top", "jungle-1", "jungle-2", "mid-1", "mid-2"]);
  assert.deepEqual(insightTeamLayout([{ teamId: 100, position: "", name: "unknown" }, { teamId: 100, position: "top", name: "top" }], true).teams.get(100).map((player) => player.name),
    ["top", "unknown"]);
});

test("live insight keeps current-player highlight after position ordering", () => {
  const state = { settings: { liveOrder: "team" } };
  const { renderLiveInsights } = compile(["orderLivePlayers", "insightTeamLayout", "renderLiveRecentPositions", "renderLiveInsights"], {
    state,
    liveAugmentRecommendationSource: () => "",
    recordLiveRosterRendered: () => {},
    renderLivePlayer: (player) => `<article class="live-player${player.isCurrent ? " is-self" : ""}">${player.name}</article>`,
    renderInsightMatches: () => '<div class="insight-match-row"></div>',
    escapeHTML: String,
    arenaLivePlayerGroups: () => [],
  });
  const html = renderLiveInsights({ mapId: 11, players: [
    { name: "self", teamId: 100, position: "bottom", isCurrent: true },
    { name: "top", teamId: 100, position: "top" },
    { name: "foe", teamId: 200, position: "top" },
  ] });
  const own = html.slice(html.indexOf("<h3>我方</h3>"), html.indexOf("<h3>对方</h3>"));
  assert.ok(own.indexOf(">top</article>") < own.indexOf(">self</article>"));
  assert.match(own, /class="live-player is-self">self/);
  assert.match(cssSource, /\.live-player-list\.is-insight \.insight-match-row \{[^}]*align-content: flex-end;[^}]*align-items: flex-end;/);
});

test("R68 rendered-roster diagnostic carries queueId and absolute counts", () => {
  const state = { liveRosterRenderDiagnostics: new Set() };
  let body;
  const { recordLiveRosterRendered } = compile(["recordLiveRosterRendered"], {
    state,
    fetch: (_url, options) => { body = JSON.parse(options.body); return Promise.resolve(); },
  });
  recordLiveRosterRendered({ phase: "ChampSelect", gameId: 68, queueId: 420, players: [{ hidden: true }, { privateHistory: true }, { privateHistory: true }] }, 1, 2);
  assert.deepEqual({ queueId: body.queueId, rendered100: body.rendered100, rendered200: body.rendered200, hiddenIdentityRendered: body.hiddenIdentityRendered, privateHistoryRendered: body.privateHistoryRendered }, { queueId: 420, rendered100: 1, rendered200: 2, hiddenIdentityRendered: 1, privateHistoryRendered: 2 });
});

test("R68 gameplay mutation probes execute real position and team logic", () => {
  const wrongPriority = source.replace("return target.positionOverride || resolved || target.clientPosition || target.position;", "return target.position || resolved || target.clientPosition;");
  const { specialistPosition } = compile(["specialistPosition"], { liveRecommendationsFor: () => ({ resolvedPosition: "bottom" }), liveRecommendationTarget: () => null }, wrongPriority);
  assert.throws(() => assert.equal(specialistPosition({}, { position: "top", clientPosition: "top", positionOverride: "" }), "bottom"));

  const noOverride = source.replace("return target.positionOverride || resolved || target.clientPosition || target.position;", "return resolved || target.clientPosition || target.position;");
  const overrideFunctions = compile(["specialistPosition"], { liveRecommendationsFor: () => ({ resolvedPosition: "bottom" }), liveRecommendationTarget: () => null }, noOverride);
  assert.throws(() => assert.equal(overrideFunctions.specialistPosition({}, { position: "mid", clientPosition: "top", positionOverride: "mid" }), "mid"));

  const hardcodedTeam = source.replace("const selfTeam = Number(selfPlayer?.teamId) || 100;", "const selfTeam = 100;");
  const state = { settings: { liveOrder: "team", detailPositionAlign: false } };
  const functions = compile(["orderLivePlayers", "insightTeamLayout", "renderLiveRecentPositions", "renderLiveInsights"], {
    state, liveAugmentRecommendationSource: () => "", clusterPremadePlayers: (players) => players,
    recordLiveRosterRendered: () => {}, renderLivePlayer: (player) => player.isCurrent ? '<span class="self-chip">自己</span>' : "foe",
    renderInsightMatches: () => "", escapeHTML: String, arenaLivePlayerGroups: () => [],
  }, hardcodedTeam);
  const html = functions.renderLiveInsights({ players: [{ teamId: 100 }, { teamId: 200, isCurrent: true }] });
  assert.throws(() => assert.match(html.slice(html.indexOf("<h3>我方</h3>"), html.indexOf("<h3>对方</h3>")), /self-chip/));

  const relativeDiagnostic = source
    .replace("    recordLiveRosterRendered(data, orderedPlayers.filter((player) => player.teamId === 100).length, orderedPlayers.filter((player) => player.teamId === 200).length);\n", "")
    .replace("    const foeTeam = selfTeam === 200 ? 100 : 200;", "    const foeTeam = selfTeam === 200 ? 100 : 200;\n    recordLiveRosterRendered(data, orderedPlayers.filter((player) => player.teamId === selfTeam).length, orderedPlayers.filter((player) => player.teamId === foeTeam).length);");
  const relativeRecorded = [];
  const relativeFunctions = compile(["orderLivePlayers", "insightTeamLayout", "renderLiveRecentPositions", "renderLiveInsights"], {
    state, liveAugmentRecommendationSource: () => "", clusterPremadePlayers: (players) => players,
    recordLiveRosterRendered: (_data, one, two) => relativeRecorded.push([one, two]), renderLivePlayer: (player) => player.isCurrent ? '<span class="self-chip">自己</span>' : "foe",
    renderInsightMatches: () => "", escapeHTML: String, arenaLivePlayerGroups: () => [],
  }, relativeDiagnostic);
  relativeFunctions.renderLiveInsights({ players: [{ teamId: 100 }, { teamId: 200, isCurrent: true }, { teamId: 200 }] });
  assert.throws(() => assert.deepEqual(relativeRecorded, [[1, 2]]));

  const noQueueId = source.replace(", queueId: Number(data?.queueId || 0), playersReceived", ", playersReceived");
  let diagnosticBody;
  const { recordLiveRosterRendered } = compile(["recordLiveRosterRendered"], {
    state: { liveRosterRenderDiagnostics: new Set() },
    fetch: (_url, options) => { diagnosticBody = JSON.parse(options.body); return Promise.resolve(); },
  }, noQueueId);
  recordLiveRosterRendered({ phase: "ChampSelect", gameId: 68, queueId: 420, players: [] }, 0, 0);
  assert.throws(() => assert.equal(diagnosticBody.queueId, 420));
});

test("R69 premade tags distinguish direct session groups from inferred groups", () => {
  const dependencies = {
    LIVE_PREMADE_MIN_SHARED_GAMES: 5,
    maskedPlayerName: (player) => player.name,
    liveDisplayedChampionId: (player) => player.championId,
    proxyAsset: (value) => value,
    assetPath: (_kind, id) => String(id),
    escapeHTML: (value) => String(value),
  };
  const { renderLivePremadeTag } = compile(["livePremadeRoster", "renderLivePremadeTag"], dependencies);
  const base = [
    { name: "甲", championId: 1, premadeGroup: "1", premadeSize: 2 },
    { name: "乙", championId: 2, premadeGroup: "1", premadeSize: 2 },
  ];
  const session = renderLivePremadeTag({ ...base[0], premadeSource: "session" }, [{ ...base[0], premadeSource: "session" }, { ...base[1], premadeSource: "session" }]);
  assert.match(session, />组队 ×2<\/span>/);
  assert.match(session, /组队 2 人/);
  assert.doesNotMatch(session, /推测/);
  const inferred = renderLivePremadeTag({ ...base[0], premadeSource: "inferred" }, [{ ...base[0], premadeSource: "inferred" }, { ...base[1], premadeSource: "inferred" }]);
  assert.match(inferred, />预组 ×2<\/span>/);
  assert.match(inferred, /预组队 2 人/);
  const both = renderLivePremadeTag({ ...base[0], premadeSource: "both" }, [{ ...base[0], premadeSource: "both" }, { ...base[1], premadeSource: "both" }]);
  assert.match(both, />组队 ×2<\/span>/);
  assert.match(both, /组队 2 人/);
  assert.doesNotMatch(both, /最近战绩/);
});

test("R69 history rows distinguish unavailable failed empty and pending states", () => {
  const render = (liveLoading, historyState) => compile(["renderInsightMatches"], {
    state: { liveLoading },
    insightScore: () => "",
    escapeHTML: String,
    number: String,
    iconFigure: () => "",
  }).renderInsightMatches({ historyState, recentGames: [] });
  const unavailable = render(false, "unavailable");
  assert.match(unavailable, /客户端未公开该玩家/);
  assert.doesNotMatch(unavailable, /暂无最近战绩/);
  const failed = render(false, "failed");
  assert.match(failed, /读取失败/);
  assert.match(failed, /data-live-history-retry/);
  assert.match(render(false, "empty"), /该玩家当前模式暂无最近战绩/);
  const pending = render(true, "pending");
  assert.match(pending, /live-history-skeleton/);
  assert.doesNotMatch(pending, /未公开|读取失败|暂无最近战绩/);
});

test("live detail shows at most ten newest recent games", () => {
  const { renderInsightMatches } = compile(["insightScore", "renderInsightMatches"], {
    escapeHTML: String, number: String, iconFigure: (_kind, id) => `<i data-champion="${id}"></i>`,
  });
  const recentGames = Array.from({ length: 12 }, (_, index) => ({
    championId: index + 1, championName: `Champion ${index + 1}`,
    win: true, kills: 1, deaths: 1, assists: 1,
  }));
  const markup = renderInsightMatches({ historyState: "ok", recentGames });
  assert.equal((markup.match(/class="insight-match is-/g) || []).length, 10);
  assert.match(markup, /data-champion="1"/);
  assert.match(markup, /data-champion="10"/);
  assert.doesNotMatch(markup, /data-champion="11"|data-champion="12"/);
});

test("R69 current-position chip preserves client position and discloses specialist fallback", () => {
  const render = (resolvedPosition) => compile(["specialistPosition", "liveCurrentPositionChip"], {
    liveRecommendationsFor: () => ({ positions: [{ position: "top" }], resolvedPosition }),
    liveRecommendationTarget: () => ({ clientPosition: "top", position: "top", positionOverride: "" }),
    livePositionDisplay: (value) => value === "adc" ? "bottom" : value,
    positionIcon: () => "",
    positionLabel: (value) => ({ top: "上路", bottom: "下路" })[value] || value,
    escapeHTML: String,
  }).liveCurrentPositionChip({ players: [{ isCurrent: true, position: "top" }] });
  const same = render("top");
  assert.match(same, /当前位置：上路/);
  assert.doesNotMatch(same, /live-position-mismatch/);
  const different = render("adc");
  assert.match(different, /当前位置：上路/);
  assert.match(different, /live-position-mismatch/);
  assert.match(different, /客户端报告的位置是 上路，但该英雄在 上路 没有样本，下方数据按 下路 展示/);
});

test("R71 ranked recent-position samples stay inside the player card, without an extra grid row", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const functions = compile(["renderLiveRecentPositions", "renderLivePlayer"], {
    state: {settings:{}}, number: String, escapeHTML: (value) => String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;"),
    maskedPlayerName: () => "Player", liveDisplayedChampionId: () => 13,
    rankTitle: () => "黄金", positionLabel: () => "中路", renderLivePremadeTag: () => "",
    iconFigure: () => "", percent: (value) => `${value}%`, kda: String,
  });
  const player = { championName: "Ryze", recentPositions: [{ position: "middle", label: "中单", games: 1 }, { position: "utility", label: "辅助", games: 0 }] };
  for (const queue of [420, 440]) {
    const roles = functions.renderLiveRecentPositions(player, queue);
    const card = functions.renderLivePlayer(player, 0, false, 13, [], roles);
    const dom = new JSDOM(`<section class="live-team">${card}<div class="insight-match-row"></div></section>`);
    try {
      const team = dom.window.document.querySelector(".live-team");
      assert.equal(team.children.length, 2, "header and history must remain the only two subgrid rows");
      const badges = team.querySelector(".live-player-copy .live-recent-positions");
      assert.equal(badges.textContent, "常用位置：中路");
      assert.equal(badges.querySelector(".live-position-sample").textContent, "中路");
      assert.doesNotMatch(badges.outerHTML, /近|10|十场|<b>/);
      assert.doesNotMatch(team.textContent, /辅助/);
    } finally { dom.window.close(); }
  }
  assert.equal(functions.renderLiveRecentPositions(player, 450), "");
  assert.equal(functions.renderLiveRecentPositions({ recentPositions: [] }, 420), "");
  const insights = functionSource(source, "renderLiveInsights");
  assert.match(insights, /orderedPlayers, renderLiveRecentPositions\(player, data.queueId\),/);
});


test("R72 role badges use explicit lane names, deduplicate and hide unknown or empty samples", () => {
  const { renderLiveRecentPositions } = compile(["renderLiveRecentPositions"]);
  const html = renderLiveRecentPositions({ recentPositions: [
    { position: "top", games: 1 }, { position: "top", games: 2 },
    { position: "jungle", games: 1 }, { position: "middle", games: 1 },
    { position: "bottom", games: 1 }, { position: "utility", games: 1 },
    { position: "other", label: "<script>bad</script>", games: 1 }, null,
  ] }, 440);
  assert.equal((html.match(/live-position-sample/g) || []).length, 5);
  for (const lane of ["上路", "打野", "中路", "下路", "辅助"]) assert.ok(html.includes(`>${lane}</span>`));
  assert.doesNotMatch(html, /script|近|场/);
  assert.equal(renderLiveRecentPositions({ recentPositions: {} }, 420), "");
  assert.equal(renderLiveRecentPositions({ recentPositions: [{ position: "other", games: 1 }] }, 420), "");
});


test("R73 demo ranked players expose recent-position samples used by role badges", () => {
  const livePlayerStart = demoSource.indexOf("const livePlayer = (");
  const livePlayerEnd = demoSource.indexOf("const runePage =", livePlayerStart);
  assert.notEqual(livePlayerStart, -1);
  assert.notEqual(livePlayerEnd, -1);
  const livePlayerFixture = demoSource.slice(livePlayerStart, livePlayerEnd);
  assert.match(livePlayerFixture, /recentPositions:\s*\[\{\s*position,\s*games:\s*Math\.max\(1,\s*recentGames\.length\)\s*\}\]/);
});

test("R73 classic ranked details stay side-by-side at medium widths and safely stack only when narrow", () => {
  assert.doesNotMatch(cssSource, /@container recommendation-area \(max-width:\s*1080px\)\s*\{\s*\.live-teams\s*\{/);
  assert.doesNotMatch(cssSource, /@container \(max-width:\s*980px\)[\s\S]{0,220}\.live-teams\.is-insight\s*\{\s*grid-template-columns:\s*1fr/);
  assert.match(cssSource, /@container recommendation-area \(max-width:\s*840px\)/);
  assert.match(cssSource, /\.live-teams\.is-insight:not\(\.is-arena\)\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0,1fr\);[\s\S]*?grid-template-rows:\s*none;/);
  assert.match(cssSource, /\.live-teams\.is-insight:not\(\.is-arena\)\s*>\s*\.live-team\s*\{[\s\S]*?display:\s*block;[\s\S]*?grid-row:\s*auto;/);
  assert.match(cssSource, /\.live-teams\.is-insight:not\(\.is-arena\)\s+\.live-player-list\.is-insight\s*\{\s*display:\s*grid;/);
});

test('OPGG season query is tab-scoped, single-flight and never reloads Riot history',async()=>{
 const state={destroyed:false};const calls=[];const rendered=[];
 let resolve;
 const pending=new Promise(r=>{resolve=r;});
 const helpers=compile(['loadOPGGSeasonSummary'],{state,api:async(...args)=>{calls.push(args);return pending;},rerenderTab:t=>rendered.push(t.key)});
 const tab={key:'pro:A',data:{player:{region:'kr',playerRef:'player_A'}}};
 const load=helpers.loadOPGGSeasonSummary(tab);
 assert.equal(await helpers.loadOPGGSeasonSummary(tab),false);
 assert.equal(calls.length,1);
 assert.equal(calls[0][0],'/api/gameplay/season-summary');
 resolve({source:'OP.GG',queue:'RANKED',season:'S2026',overall:{games:573},champions:[{games:81,kills:8.8}]});
 assert.equal(await load,true);
 assert.equal(tab.opggSeason.data.overall.games,573);
 assert.deepEqual(rendered,['pro:A']);
 assert.equal(await helpers.loadOPGGSeasonSummary(tab),false);
 assert.equal(calls.length,1);
 await helpers.loadOPGGSeasonSummary({data:{player:{region:'tencent',playerRef:'CN'}}});
 await helpers.loadOPGGSeasonSummary({data:{player:{region:'kr',playerRef:'private',privateHistory:true}}});
 assert.equal(calls.length,1);
});

test('OPGG late response cannot overwrite another account and failure has no crawl fallback',async()=>{
 const state={destroyed:false};let resolve,calls=0;
 const tab={key:'A',data:{player:{region:'kr',playerRef:'player_A'}}};
 const helpers=compile(['loadOPGGSeasonSummary'],{state,api:()=>{calls++;return new Promise(r=>{resolve=r;});},rerenderTab:()=>assert.fail('stale result rendered')});
 const load=helpers.loadOPGGSeasonSummary(tab);
 tab.data={player:{region:'kr',playerRef:'player_B'}};
 resolve({source:'OP.GG',queue:'RANKED',season:'S2026',overall:{games:573},champions:[]});
 assert.equal(await load,false);assert.equal(tab.opggSeason,undefined);
 const failed=compile(['loadOPGGSeasonSummary'],{state,api:async()=>{calls++;throw Error('upstream unavailable');},rerenderTab:()=>{}});
 await failed.loadOPGGSeasonSummary(tab);await failed.loadOPGGSeasonSummary(tab);
 assert.equal(calls,2,'failure must not cause retries or match-by-match backfill');
});

test('OPGG career rendering uses full season total and only the matching KR tab',()=>{
 let args;
 const dependencies={number:String,rankedQueueData:()=>({}),rankedQueueSwitcher:()=>'',renderRanks:()=>'',renderRecentRanked:()=>'',renderAbility:()=>'',renderChampionStats:(...x)=>{args=x;return '';},renderPositionStats:()=>'',renderMasteries:()=>'',renderRecentPlayers:()=>'',renderActivity:()=>'',renderOverviewShareButton:()=>''};
 const {careerSectionEntries}=compile(['careerSectionEntries'],dependencies);
 const data={player:{region:'kr',playerRef:'A'},overall:{games:20},championStats:[{games:2}],seasonStatsProgress:{unavailable:true,message:'近期样本'}};
 const tab={opggSeason:{playerRef:'A',data:{season:'S2026',overall:{games:573},champions:[{games:81,kills:8.8}]}}};
 careerSectionEntries(data,tab);assert.equal(args[1].games,573);assert.equal(args[0][0].kills,8.8);assert.match(args[2].message,/^573 场排位$/);
 careerSectionEntries({...data,player:{region:'kr',playerRef:'B'}},tab);assert.equal(args[1].games,undefined);assert.equal(args[2].seasonOnly,true);
 careerSectionEntries({...data,player:{region:'tencent',playerRef:'A'}},tab);assert.equal(args[1].games,20);
});

test('current-game queries are single-flight, account scoped, and reject late identity changes',async()=>{
 let resolve,calls=0,paint=0;
 const tab={key:'fixture',region:'kr',data:{player:{playerRef:'first'}}};
 const {loadOverviewCurrentGame}=compile(['loadOverviewCurrentGame'],{state:{destroyed:false},riotTab:tab=>tab.region==='kr',api:()=>{calls++;return new Promise(r=>resolve=r)},updateCurrentGameCard:()=>paint++});
 const first=loadOverviewCurrentGame(tab);await loadOverviewCurrentGame(tab);assert.equal(calls,1);
 tab.data.player.playerRef='second';resolve({status:'active',teams:[]});await first;assert.equal(tab.currentGame,undefined);
 const second=loadOverviewCurrentGame(tab);resolve({status:'none',source:'OP.GG'});await second;assert.equal(tab.currentGame.ref,'second');assert.equal(tab.currentGame.data.status,'none');
 await loadOverviewCurrentGame(tab);assert.equal(calls,2);
 tab.data.player.privateHistory=true;await loadOverviewCurrentGame(tab,true);assert.equal(calls,2);assert.ok(paint>=2);
 tab.region="cn";await loadOverviewCurrentGame(tab,true);assert.equal(calls,2,"CN requests are removed");
});

test("R75 pro target is independent of specialist rankings and rejects non-SR modes", () => {
  const state = { proRunes: new Map() };
  const { proRequestTarget, proRunesFor } = compile(["proRequestTarget", "proRunesFor"], {
    state, liveRecommendationTarget: data => data.selected === false ? null : ({ championId: 69, position: "mid" }),
    liveRecommendationsFor: () => ({ runes: { pros: [] } }),
  });
  assert.ok(proRequestTarget({ queueId: 430, hasTopPlayers: false }));
  assert.ok(proRequestTarget({ mapId: 11, gameMode: "CLASSIC", queueId: 0 }));
  assert.equal(proRequestTarget({ mapId: 12, gameMode: "ARAM" }), null);
  assert.equal(proRequestTarget({ mapId: 11, gameMode: "URF" }), null);
  assert.equal(proRequestTarget({ selected: false }), null);
  state.proRunes.set("69:mid", { pros: [{ key: "old", playedAt: Date.now() - 31*86400000 }, { key: "new", playedAt: Date.now() - 1000 }] });
  assert.deepEqual(proRunesFor({ mapId: 11 }).map(x => x.key), ["new"]);
});

test("R75 pro rows use shared game renderer, neutral unknown, real event and sample threshold", () => {
  const state = { live: { mapId: 11 }, specialistPlayerTabs: new Map(), selectedRecommendation: "pro-1", summonerSpells: { spells: [{ id: 4, name: "闪现" }, { id: 11, name: "惩戒" }] } };
  const fn = compile(["renderSpecialistPlayers", "renderRuneSourceSection", "proRuneRecordLabel"], {
    state, proRequestTarget: () => ({key:"69:mid"}), specialistRequestTarget: () => ({key:"69:mid"}), positionLabel: value => value, livePositionDisplay: value => value, escapeHTML: x=>String(x??""), relativeTime:()=>"刚刚",
    runeConfigurationTitle:()=>"电刑 + 坚决", renderUnifiedRuneBoard:()=>"<runes></runes>", renderItemIcon:id=>`<item>${id}</item>`, renderSummonerSpellIcon:id=>`<spell>${id}</spell>`, iconFigure:()=>"<icon></icon>",
  });
  const row = { key:"pro-1", title:"T1 Faker", playerName:"Faker", championId:69, position:"中路", opponentPlayerName:"GEN Chovy", eventLabel:"LCK · 2026-09-06 · T1 vs GEN 第 1 局", winKnown:false, recordGames:2, recordWins:1, recordPartial:true, selectedComplete:false, itemIds:[3364,2031,6692] };
  let html=fn.renderRuneSourceSection({key:"pro",items:[row],proStatus:{}},true);
  assert.match(html,/specialist-player-tabs/);assert.match(html,/data-player-source="pro"/);
  assert.match(html,/T1 Faker/);assert.match(html,/aria-label="1胜1负，仅已确认场次"/);assert.doesNotMatch(html,/%/);
  assert.match(html,/specialist-game-row is-unknown is-selected/);assert.match(html,/该局胜负未获官方确认/);
  assert.match(html,/LCK · 2026-09-06/);assert.match(html,/GEN Chovy/);assert.match(html,/最终装备/);
  assert.match(html,/上游未提供完整槽位/);assert.doesNotMatch(html,/specialist-opponent-rank/);
  assert.doesNotMatch(html,/specialist-game-spells/,"pro record without verified spell IDs must omit the entire spell container");
  assert.doesNotMatch(html,/rune-spell-row/,"pro records must not render a separate spell row");
  html=fn.renderSpecialistPlayers([{...row,recordGames:3,recordWins:2}],"pro");assert.match(html,/aria-label="2胜1负，仅已确认场次"/);
  html=fn.renderSpecialistPlayers([{...row,spell1Id:4,spell2Id:11}],"specialist");
  assert.match(html,/<div class="specialist-game-spells"><spell>4<\/spell><spell>11<\/spell><\/div>/);
  assert.doesNotMatch(html,/rune-spell-row|召唤师技能|闪现|惩戒/);
  html=fn.renderRuneSourceSection({key:"pro",items:[row],proStatus:{readAt:new Date(Date.now()-600000).toISOString()}},true);
  assert.doesNotMatch(html,/缓存数据|无法连接职业赛事数据源/);assert.match(html,/data-retry-pro-runes/);assert.match(html,/当前显示上次读取结果/);
  html=fn.renderRuneSourceSection({key:"pro",items:[],proStatus:{reason:"upstream-timeout"}},true);assert.match(html,/上游超时/);assert.match(html,/data-retry-pro-runes/);
});

test("R171 spell pairs stay with their own rune source and require two verified IDs", () => {
  const state = { summonerSpells: { spells: [{ id: 4, name: "闪现" }, { id: 11, name: "惩戒" }] } };
  const { recommendedRuneSpellIDs, renderRuneSpellPair } = compile(["recommendedRuneSpellIDs", "renderRuneSpellPair"], {
    state,
    liveRecommendationsFor: () => ({ build: { spellOptions: [{ ids: [4, 11] }, { ids: [3, 11] }] } }),
    renderSummonerSpellIcon: id => `<icon data-spell="${id}"></icon>`,
    escapeHTML: value => String(value),
  });
  assert.deepEqual(recommendedRuneSpellIDs({}, { sourceLabel: "OPGG" }), [4, 11]);
  assert.deepEqual(recommendedRuneSpellIDs({}, { sourceLabel: "绝活哥", spell1Id: 7, spell2Id: 4 }), [7, 4]);
  assert.deepEqual(recommendedRuneSpellIDs({}, { sourceLabel: "职业选手" }), []);
  assert.deepEqual(recommendedRuneSpellIDs({}, { sourceLabel: "绝活哥", spell1Id: 4 }), []);
  assert.equal(renderRuneSpellPair([4, 0]), "");
  assert.equal(renderRuneSpellPair([undefined, 11]), "");
  const html = renderRuneSpellPair([4, 11]);
  assert.match(html, /data-spell="4".*闪现.*data-spell="11".*惩戒/s);
  assert.doesNotMatch(html, /占位/);
});

test("R171 apply optionally writes the selected source's spells and reports partial success", async () => {
  const state = { live: { players: [{ isCurrent: true, championId: 64, championName: "李青" }] }, applyRuneSpells: true };
  const recommendation = { sourceLabel: "绝活哥", championId: 64, primaryStyleId: 8000, subStyleId: 8100, selectedPerkIds: [1,2,3,4,5,6,7,8,9], spell1Id: 4, spell2Id: 11 };
  const posts = [], toasts = [];
  let spellApplied = true;
  const { applyRunes } = compile(["applyRunes"], {
    state, selectedRuneRecommendation: () => recommendation, liveRecommendationChampionId: player => player.championId,
    liveRecommendationsFor: () => ({ build: { spellOptions: [{ ids: [3, 6] }] } }),
    api: async (_, options) => { posts.push(JSON.parse(options.body)); return { applied: true, spellApplied }; },
    showToast: message => toasts.push(message),
  });
  const button = { disabled: false, textContent: "应用所选符文" };
  await applyRunes({ currentTarget: button });
  assert.deepEqual([posts[0].spell1Id, posts[0].spell2Id], [4, 11]);
  assert.equal(toasts.at(-1), "符文和召唤师技能已应用");
  spellApplied = false;
  await applyRunes({ currentTarget: button });
  assert.equal(toasts.at(-1), "符文已应用，召唤师技能未能同步");
  state.applyRuneSpells = false;
  await applyRunes({ currentTarget: button });
  assert.equal(posts.at(-1).spell1Id, undefined);
  assert.equal(toasts.at(-1), "符文已新建并设为当前页");
  state.applyRuneSpells = true;
  recommendation.sourceLabel = "职业选手";
  recommendation.spell1Id = 0;
  await applyRunes({ currentTarget: button });
  assert.equal(posts.at(-1).spell1Id, undefined, "pro source must not borrow OPGG spells");
});

test("R171 rune card shows OPGG spells once and only complete specialist rows", () => {
  const state = { live: {}, specialistRuneFailures: new Map(), summonerSpells: { spells: [{ id: 4, name: "闪现" }, { id: 11, name: "惩戒" }] } };
  const { renderRuneSourceSection } = compile(["renderRuneSourceSection"], {
    state, liveRecommendationsFor: () => ({ build: { spellOptions: [{ ids: [4, 11] }] } }),
    specialistRequestTarget: () => null, specialistRuneFailure: () => null,
    renderRuneChoice: config => `<choice>${config.key}</choice>`, renderSpecialistPlayers: () => "",
    renderSummonerSpellIcon: id => `<icon data-spell="${id}"></icon>`, escapeHTML: value => String(value),
  });
  const opgg = renderRuneSourceSection({ key: "opgg", title: "OPGG", items: [{ key: "a" }, { key: "b" }] }, true);
  assert.equal((opgg.match(/class="rune-spell-row"/g) || []).length, 1);
  assert.match(opgg, /data-spell="4".*闪现.*data-spell="11".*惩戒/s);
  assert.ok(opgg.indexOf("rune-spell-row") < opgg.indexOf("<choice>a</choice>"));

  const { renderRuneSpellPair } = compile(["renderRuneSpellPair"], {
    state, renderSummonerSpellIcon: id => `<icon data-spell="${id}"></icon>`, escapeHTML: value => String(value),
  });
  for (const ids of [[0, 11], [4, 0], [undefined, 11], [4, undefined]]) {
    assert.equal(renderRuneSpellPair(ids), "", `missing pair ${ids} must not render a row`);
  }
});

test("R75 incomplete pro disables actual action markup and refuses POST", async () => {
  const selection={sourceKey:"pro",selectedComplete:false,selectedPerkIds:[8112,8143,8137,8106,8401,8444,5008,5011]};
  const state={recommendationTab:"runes",live:{available:true,phase:"ChampSelect",players:[{isCurrent:true,championId:69,championName:"卡西奥佩娅"}]}};
  let posts=0;
  const f=compile(["renderRecommendationArea","applyRunes"],{
    state,liveRecommendationTarget:()=>({key:"69:mid"}),liveRecommendationsFor:()=>({}),liveAugmentRecommendationSource:()=>null,
    recommendationCapabilities:()=>({hasRunes:true}),recommendationTabSpecs:()=>[["runes","符文"]],recommendationActiveTab:()=>"runes",
    selectedRuneRecommendation:()=>selection,recommendationPanelBusy:()=>false,renderLiveInsights:()=>"",renderRuneRecommendations:()=>"",
    renderChampionRecommendationHeader:()=>"",renderBuildRecommendation:()=>"",renderRecommendationDataNotices:()=>"",renderLaneMatchupCard:()=>"",runeConfigurationTitle:()=>"职业符文",escapeHTML:x=>String(x),
    showToast:()=>{},api:async()=>{posts++},
  });
  assert.match(f.renderRecommendationArea(state.live),/data-apply-runes="selected" disabled/);
  await f.applyRunes({currentTarget:{}});assert.equal(posts,0);
  selection.selectedComplete=true;
  assert.match(f.renderRecommendationArea(state.live),/data-apply-runes="selected" disabled/);
  await f.applyRunes({currentTarget:{}});assert.equal(posts,0,"false complete flag must not allow eight perks");
  selection.selectedPerkIds.push(5005);
  assert.doesNotMatch(f.renderRecommendationArea(state.live),/data-apply-runes="selected" disabled/);
  selection.sourceLabel="职业选手"; selection.spell1Id=4; selection.spell2Id=11;
  assert.match(f.renderRecommendationArea(state.live),/data-apply-rune-spells checked/);
  state.applyRuneSpells=false;
  assert.match(f.renderRecommendationArea(state.live),/data-apply-rune-spells(?! checked)/);
  selection.spell2Id=0;
  assert.doesNotMatch(f.renderRecommendationArea(state.live),/data-apply-rune-spells/);
  state.live.phase="InProgress";assert.match(f.renderRecommendationArea(state.live),/data-apply-runes="selected" disabled/);
});

test("R75 async pro retains marked cache on failure and prevents old generation overwrite", async () => {
  const data={mapId:11};const state={live:data,liveGameGeneration:1,section:"live",proRunes:new Map(),proRuneFlights:new Map()};
  let resolve, fail=false, calls=0;
  const f=compile(["proRequestTarget","ensureProRunes"],{state,liveRecommendationTarget:()=>({championId:69,position:"mid"}),liveRecommendationsFor:()=>null,api:()=>{calls++;if(fail)return Promise.reject(new Error("offline"));return new Promise(r=>resolve=r)},renderLive:()=>{},ensurePerks:()=>{},ensureItems:()=>{},setTimeout:()=>0,clearTimeout:()=>{}});
  const pending=f.ensureProRunes(data);await f.ensureProRunes(data);assert.equal(calls,1);
  state.liveGameGeneration++;state.proRuneFlights.clear();resolve({pros:[{key:"obsolete"}]});await pending;assert.equal(state.proRunes.size,0);
  state.proRunes.set("69:mid",{pros:[{key:"cached"}],receivedAt:0,readAt:new Date().toISOString()});fail=true;
  await f.ensureProRunes(data,true);const cache=state.proRunes.get("69:mid");assert.equal(cache.pros[0].key,"cached");assert.equal(cache.stale,true);assert.equal(cache.reason,"network-unavailable");
  assert.match(source,/addEventListener\("deep-legends:pro-runes"/);assert.match(source,/entry\.receivedAt = 0/);
});

test("R76 pro degradation uses game failure ratio, not a nonempty reason", () => {
  const {renderRuneSourceSection:render} = compile(["renderRuneSourceSection"], {
    escapeHTML:x=>String(x??""), renderSpecialistPlayers:()=>"<div>可用推荐</div>",
  });
  const fresh={readAt:new Date().toISOString(),reason:"network-unavailable",stale:false};
  const show=status=>render({key:"pro",items:[{}],proStatus:{...fresh,...status}},true);
  for(const [totalGames,failedGames] of [[99,1],[10,1],[0,0],[99,0]]) {
    const html=show({totalGames,failedGames});
    assert.doesNotMatch(html,/无法连接|请检查网络|缓存数据|部分职业赛事数据加载失败|data-retry-pro-runes/,`${failedGames}/${totalGames} is not whole-source failure`);
    assert.match(html,/可用推荐/);
  }
  const degraded=show({totalGames:30,failedGames:10});
  assert.doesNotMatch(degraded,/部分职业赛事数据加载失败|暂未补齐/);
  assert.doesNotMatch(degraded,/10\/30/); assert.match(degraded,/data-retry-pro-runes/);
  assert.doesNotMatch(degraded,/无法连接|请检查网络|缓存数据/);
  for(const status of [{stale:true}, {readAt:new Date(Date.now()-600000).toISOString()}]) {
    const html=show({...status,totalGames:99,failedGames:1});
    assert.doesNotMatch(html,/缓存数据|无法连接职业赛事数据源/);assert.match(html,/data-retry-pro-runes/);assert.match(html,/当前显示上次读取结果/);
  }
  assert.doesNotMatch(show({stale:true,reason:"cache-write-failed"}),/缓存写入失败/);
  assert.match(show({totalGames:99,failedGames:1,outcomesLoading:true}),/正在核对/);
  const empty=proStatus=>render({key:"pro",items:[],proStatus},true);
  assert.match(empty({reason:"preparing",preparing:true}),/正在准备职业选手数据/);
  assert.doesNotMatch(empty({reason:"preparing",preparing:true}),/data-retry-pro-runes/);
  assert.match(empty({reason:"no-sample"}),/近 30 天暂无该英雄在当前所选位置/);
  assert.match(empty({reason:"network-unavailable",stale:true}),/无法连接职业赛事数据源/);
});

test("pro tabs keep every player and display latest five games per player without relative dates", () => {
  const state={live:{},specialistPlayerTabs:new Map(),selectedRecommendation:""};
  const {renderSpecialistPlayers:render}=compile(["renderSpecialistPlayers","proRuneRecordLabel"],{
    state,proRequestTarget:()=>({key:"69:mid"}),escapeHTML:x=>String(x??""),
    runeConfigurationTitle:()=>"相位猛冲 + 精密",renderUnifiedRuneBoard:()=>"<runes></runes>",renderItemIcon:id=>`<item>${id}</item>`,iconFigure:()=>"<icon></icon>",
    relativeTime:()=>{throw Error("relative time must not be rendered");},
  });
  const rows=[];
  for(let player=0;player<7;player++) for(let game=0;game<8;game++) rows.push({
    title:`Team${player} Player${player}`,playerName:`Player${player}`,key:`p${player}-g${game}`,playedAt:1000+game,
    championId:69,position:"中路",itemIds:[6657],eventLabel:`赛事第 ${game} 局`,recordGames:8,recordWins:4,
  });
  for(let player=0;player<7;player++) {
    state.specialistPlayerTabs.set("pro:69:mid",`Team${player} Player${player}`);
    const html=render(rows,"pro");
    assert.equal((html.match(/data-specialist-player=/g)||[]).length,7,"no player-count cap");
    assert.equal((html.match(/data-rune-choice=/g)||[]).length,5,"five games per player");
    assert.equal((html.match(/最终装备/g)||[]).length,5);
    assert.deepEqual([...html.matchAll(/data-rune-choice="([^"]+)"/g)].map(m=>m[1]),[7,6,5,4,3].map(g=>`p${player}-g${g}`));
    assert.doesNotMatch(html,/<time>|天前|小时前|分钟前/);
    assert.match(html,/aria-label="4胜4负"/);
  }
});

test("rune source order is OPGG, professional, specialist with no specialist subtitle",()=>{
  const state={runeSourceTab:"specialist",specialistRuneFailures:new Map(),specialistRunes:new Map(),proRunes:new Map(),proRuneFlights:new Map()};
  const {renderRuneRecommendations:render}=compile(["renderRuneRecommendations"],{
    state,liveRecommendationsFor:()=>({runes:{opgg:[]}}),liveRecommendationTarget:()=>({}),specialistRequestTarget:()=>({key:"69:mid"}),
    specialistRuneFailure:()=>null,recommendationQueueHasTopPlayers:()=>true,specialistRunesFor:()=>[],specialistRuneFlightActive:()=>false,
    proRequestTarget:()=>({key:"69:mid"}),proRunesFor:()=>[],escapeHTML:x=>String(x??""),renderRuneSourceSection:()=>"<section></section>",
  });
  const html=render({},true);
  assert.deepEqual([...html.matchAll(/data-rune-source="([^"]+)"/g)].map(m=>m[1]),["opgg","pro","specialist"]);
  assert.doesNotMatch(html,/当前分路|最近 10 局|rune-source-note/);
});

test("professional request follows the displayed position and explicit selection wins over fallback",()=>{
  let target={championId:69,position:"other"};
  const {proRequestTarget:request}=compile(["proRequestTarget"],{
    liveRecommendationTarget:()=>target,liveRecommendationsFor:()=>({resolvedPosition:"mid"}),
  });
  assert.equal(request({mapId:11}).position,"mid");
  target={...target,position:"top",positionOverride:"top"};
  assert.equal(request({mapId:11}).position,"top");
  assert.equal(request({mapId:11}).key,"69:top");
});

test('unknown pro outcomes are not presented as a zero-game performance record',()=>{
 const {proRuneRecordLabel}=compile(['proRuneRecordLabel']);
 assert.match(proRuneRecordLabel({recordGames:2,recordWins:null}),/胜负待确认/);
 assert.match(proRuneRecordLabel({recordGames:2,recordWins:3}),/胜负待确认/);
 assert.match(proRuneRecordLabel({recordGames:0,recordWins:0,recordPartial:true}),/胜负待确认/);
 assert.match(proRuneRecordLabel({recordGames:2,recordWins:1,recordPartial:true}),/aria-label="1胜1负，仅已确认场次"/);
 assert.match(proRuneRecordLabel({recordGames:4,recordWins:3}),/class="is-win">3<.*class="is-loss">1</);
});

test('cached overview re-enters independent season and current-game loaders without requesting history',async()=>{
 const tab={data:{player:{region:'kr',playerRef:'cached'}},loading:false};
 const calls=[];
 const {loadOverview}=compile(['loadOverview'],{tabReady:()=>true,activeTab:()=>tab,rerenderTab:()=>calls.push('render'),loadOPGGSeasonSummary:()=>calls.push('season'),loadOverviewCurrentGame:()=>calls.push('current'),loadMayhemRating:()=>calls.push('mayhem'),state:{},api:()=>assert.fail('cached overview must not request history')});
 await loadOverview(tab);
 assert.deepEqual(calls,['render','season','current','mayhem']);
});

test("live specialist overview uses full identity without merging tags or enabling tournament names", () => {
  const state = { live: {}, specialistPlayerTabs: new Map(), selectedRecommendation: "" };
  const { renderSpecialistPlayers: render } = compile(["renderSpecialistPlayers", "proRuneRecordLabel"], {
    state, specialistRequestTarget: () => ({ key: "69:mid" }), proRequestTarget: () => ({ key: "69:mid" }),
    escapeHTML: value => String(value ?? ""), runeConfigurationTitle: () => "符文",
    renderUnifiedRuneBoard: () => "<runes/>", renderItemIcon: () => "", iconFigure: () => "<icon/>",
  });
  const items = [
    { key: "first", playerName: "same", tagLine: "KR1", region: "kr", itemIds: [] },
    { key: "second", playerName: "same", tagLine: "KR2", region: "kr", itemIds: [] },
  ];
  let html = render(items);
  assert.equal((html.match(/data-specialist-overview/g) || []).length, 2);
  assert.match(html, /data-specialist-player="same#KR1"/);
  assert.match(html, /data-specialist-player="same#KR2"/);
  assert.match(html, /data-rune-choice="first"/);
  assert.doesNotMatch(html, /data-rune-choice="second"/);
  state.specialistPlayerTabs.set("69:mid", "same#KR2");
  html = render(items);
  assert.match(html, /data-rune-choice="second"/);
  assert.doesNotMatch(html, /data-rune-choice="first"/);
  assert.doesNotMatch(render([{ ...items[0], title: "T1 Faker" }], "pro"), /data-specialist-overview/);
  assert.doesNotMatch(render([{ ...items[0], tagLine: "" }]), /data-specialist-overview/);
  assert.doesNotMatch(render([{ ...items[0], region: "cn" }]), /data-specialist-overview/);
});

test('TheShy Camille official eight-perk response shows known shards without inventing slots', () => {
  const state={perks:{styles:[],statModSlots:[{perks:[{id:5008},{id:5005}]},{perks:[{id:5008},{id:5010}]},{perks:[{id:5011}]}]}};
  const {renderUnifiedRuneBoard}=compile(['renderUnifiedRuneBoard'],{state,escapeHTML:String,renderRuneOption:(perk,on)=>`<i data-id="${perk.id}" data-selected="${on}"></i>`});
  const html=renderUnifiedRuneBoard({selectedComplete:false,selectedPerkIds:[8437,8401,8444,8451,8347,8345,5008,5011]});
  const [board,known]=html.split('<div class="pro-known-shards"');
  assert.doesNotMatch(board,/data-selected="true"/);
  assert.match(known,/槽位未确认/);
  assert.equal((known.match(/data-selected="true"/g)||[]).length,2);
  assert.match(known,/data-id="5008"/);assert.match(known,/data-id="5011"/);
});

test('KR unavailable season never renders recent20 as champion season statistics',()=>{
 const {renderChampionStats}=compile(['renderChampionStats'],{escapeHTML:String});
 const html=renderChampionStats([],{games:20,winRate:50},{seasonOnly:true,unavailable:true,message:'本赛季英雄统计暂不可用，请刷新重试'});
 assert.match(html,/本赛季英雄统计暂不可用/);
 assert.doesNotMatch(html,/champion-stat-row|50%|全部英雄/);
});

test('resolved repeated pro shards render in both legal rows, not in the bottom strip', () => {
  const state={perks:{styles:[],statModSlots:[{perks:[{id:5008},{id:5005}]},{perks:[{id:5008},{id:5010}]},{perks:[{id:5011},{id:5001}]}]}};
  const {renderUnifiedRuneBoard}=compile(['renderUnifiedRuneBoard'],{state,escapeHTML:String,renderRuneOption:(perk,on)=>`<i data-id="${perk.id}" data-selected="${on}"></i>`});
  const html=renderUnifiedRuneBoard({selectedComplete:true,statModIds:[5008,5008,5011],selectedPerkIds:[8437,8401,8444,8451,8347,8345,5008,5008,5011],shardResolution:'deduplicated-unique'});
  assert.equal((html.match(/data-id="5008" data-selected="true"/g)||[]).length,2);
  assert.equal((html.match(/data-selected="true"/g)||[]).length,3);
  assert.doesNotMatch(html,/pro-known-shards|槽位未确认/);
});

test("R167 lane card renders only a locked enemy in the current player's lane", () => {
  const state = { laneMatchupCandidates: new Map(), recommendationTab: "insight" };
  const player = (teamId, position, championId, extra = {}) => ({ teamId, position, championId, championLocked: championId > 0, ...(teamId === 100 ? { isAlly: true } : {}), ...extra });
  const data = { phase: "ChampSelect", available: true, players: [
    player(100, "middle", 69, { isCurrent: true, rank: { tier: "GOLD" } }),
    player(100, "middle", 7),
    player(200, "top", 64, { championName: "李青" }),
    player(200, "middle", 103, { championName: "阿狸" }),
  ] };
  let payload = {};
  const helpers = compile(["laneMatchupContext", "laneMatchupOwnChampionId", "laneMatchupTier", "laneMatchupCandidateKey", "renderLaneMatchupCard", "renderRecommendationArea"], {
    state, livePositionValue: value => ({ middle: "mid", bottom: "adc", utility: "support" })[value] || value || "",
    liveRecommendationTier: () => "emerald_plus", iconFigure: (_kind, id) => `<img data-champion-id="${id}">`,
    rate: value => `${Number(value).toFixed(1)}%`, escapeHTML: value => String(value ?? ""),
    liveRecommendationTarget: current => current.players[0].championId ? { key: "self" } : null,
    liveRecommendationsFor: () => payload, liveAugmentRecommendationSource: () => "",
    recommendationCapabilities: () => ({ hasRunes: false, hasAugments: false }),
    recommendationTabSpecs: () => [["insight", "详情"]], recommendationActiveTab: () => "insight",
    selectedRuneRecommendation: () => null, recommendationPanelBusy: () => false,
    renderLiveInsights: () => "<div>玩家列表</div>", renderChampionRecommendationHeader: () => "",
    renderBuildRecommendation: () => "", renderRecommendationDataNotices: () => "",
    recommendationEmptyPanel: () => "",
  });
  const render = hero => { const row = [...(hero?.weakAgainst || []), ...(hero?.strongAgainst || [])].find(row => row.championId === 103); state.laneMatchupPairs = new Map([["69:103:mid:emerald_plus", { status: "succeeded", data: row }]]); return helpers.renderLaneMatchupCard(data, hero); };
  let html = render({ weakAgainst: [{ championId: 103, winRate: 43.7, championName: "阿狸" }] });
  assert.match(html, /data-lane-matchup-card/);
  assert.match(html, /偏劣势 43\.7%/);
  assert.equal((html.match(/lane-matchup-result/g) || []).length, 1);
  assert.equal((html.match(/data-champion-id="103"/g) || []).length, 1);
  assert.doesNotMatch(html, /data-champion-id="64"|data-champion-id="7"/);
  payload = { hero: { weakAgainst: [{ championId: 103, winRate: 43.7 }] } };
  const insightHTML = helpers.renderRecommendationArea(data);
  assert.ok(insightHTML.indexOf("data-lane-matchup-card") < insightHTML.indexOf("玩家列表"));
  html = render({ strongAgainst: [{ championId: 103, winRate: 57.4 }] });
  assert.match(html, /偏优势 57\.4%/);
  assert.equal(render({ weakAgainst: [{ championId: 64, winRate: 41 }] }), "");
  payload = {};
  state.laneMatchupPairs.clear();
  assert.doesNotMatch(helpers.renderRecommendationArea(data), /data-lane-matchup-card/);
  data.phase = "InProgress";
  assert.equal(render({ weakAgainst: [{ championId: 103, winRate: 43.7 }] }), "");
  data.phase = "ChampSelect";
  data.players[0].position = "";
  assert.equal(render({ weakAgainst: [{ championId: 103, winRate: 43.7 }] }), "");
  data.players[0].position = "middle";
  data.players[3].championLocked = false;
  assert.equal(render({ weakAgainst: [{ championId: 103, winRate: 43.7 }] }), "");
});

// gameplay.go's isAlly := isCurrent || (arenaMode && ...): in standard ranked/flex select,
// isAlly is true ONLY for the current player, never for the other four teammates. A teammate
// who ends up in the same lane as the current player (autofill/duo mishap) therefore also has
// isAlly falsy here — exactly like a real enemy. laneMatchupContext must tell them apart by
// teamId, not by isAlly, or a teammate's pick would be shown as "the enemy laner".
test("R167 lane card tells a same-lane teammate apart from the real enemy when isAlly is falsy for both", () => {
  const state = { laneMatchupCandidates: new Map(), recommendationTab: "insight" };
  const data = { phase: "ChampSelect", available: true, players: [
    { isCurrent: true, teamId: 100, position: "middle", championId: 69, isAlly: true, rank: { tier: "GOLD" } },
    // Real backend semantics: not current, so isAlly is omitted/falsy despite being an ally.
    { teamId: 100, position: "middle", championId: 999, championLocked: true, championName: "队友英雄" },
    { teamId: 200, position: "middle", championId: 103, championLocked: true, championName: "阿狸" },
  ] };
  const helpers = compile(["laneMatchupContext", "laneMatchupOwnChampionId", "laneMatchupTier", "laneMatchupCandidateKey", "renderLaneMatchupCard"], {
    state, livePositionValue: value => ({ middle: "mid", bottom: "adc", utility: "support" })[value] || value || "",
    liveRecommendationTier: () => "emerald_plus", iconFigure: (_kind, id) => `<img data-champion-id="${id}">`,
    rate: value => `${Number(value).toFixed(1)}%`, escapeHTML: value => String(value ?? ""),
  });
  state.laneMatchupPairs = new Map([["69:103:mid:emerald_plus", {status:"succeeded", data:{winRate:43.7}}]]);
  const html = helpers.renderLaneMatchupCard(data, { weakAgainst: [{ championId: 103, winRate: 43.7, championName: "阿狸" }] });
  assert.match(html, /data-lane-matchup-card/);
  assert.match(html, /偏劣势 43\.7%/);
  assert.equal((html.match(/data-champion-id="103"/g) || []).length, 1, "must pick the team-200 player as the enemy");
  assert.doesNotMatch(html, /data-champion-id="999"/, "must not treat the same-lane ally with falsy isAlly as the enemy");
});

test("R167 candidate request uses the enemy champion, current lane and tier, once per mapping", async () => {
  const data = { phase: "ChampSelect", available: true, players: [
    { isCurrent: true, isAlly: true, teamId: 100, position: "middle", championId: 0, rank: { tier: "GOLD" } },
    { teamId: 200, position: "top", championId: 64, championLocked: true },
    { teamId: 200, position: "middle", championId: 103, championLocked: true, championName: "阿狸" },
  ] };
  const state = { live: data, section: "live", liveGameGeneration: 1, laneMatchupCandidates: new Map() };
  const requests = [];
  const diagnostics = [];
  const finishes = [];
  const helpers = compile(["laneMatchupContext", "laneMatchupOwnChampionId", "laneMatchupTier", "laneMatchupCandidateKey", "recordLaneMatchupCandidateDiagnostic", "ensureLaneMatchupCandidates", "renderLaneMatchupCard"], {
    state, livePositionValue: value => ({ middle: "mid", bottom: "adc", utility: "support" })[value] || value || "",
    liveRecommendationTier: () => "emerald_plus", URLSearchParams,
    recordItemSetClientDiagnostic: (event, reason, context) => diagnostics.push({ event, reason, context }),
    api: path => { requests.push(path); return new Promise(resolve => { finishes.push(resolve); }); },
    renderLive: () => {}, iconFigure: (_kind, id) => `<img data-champion-id="${id}">`,
    rate: value => `${Number(value).toFixed(1)}%`, escapeHTML: String,
  });
  const first = helpers.ensureLaneMatchupCandidates(data);
  const duplicate = helpers.ensureLaneMatchupCandidates(data);
  assert.equal(requests.length, 1, "pending request is single flight");
  assert.deepEqual(diagnostics.slice(0, 2).map((item) => item.reason), ["requested", "in-flight"]);
  const query = new URL(requests[0], "http://local").searchParams;
  assert.equal(query.get("champion"), "103");
  assert.equal(query.get("position"), "mid");
  assert.equal(query.get("tier"), "emerald_plus");
  finishes[0]({ counters: { weakAgainst: [{ championId: 69, name: "卡西奥佩娅", winRate: 43, games: 120 }] } });
  await Promise.all([first, duplicate]);
  await helpers.ensureLaneMatchupCandidates(data);
  assert.equal(requests.length, 1, "later live polls reuse the cached matchup");
  assert.equal(diagnostics.filter((item) => item.reason === "cached").length, 1, "cache diagnostics are deduplicated across polls");
  assert.ok(diagnostics.some((item) => item.reason === "succeeded" && item.context.rowCount === 1));
  assert.ok(diagnostics.every((item) => item.event === "lane_matchup_candidate_fetch"));
  assert.match(helpers.renderLaneMatchupCard(data, {}), /卡西奥佩娅/);
  assert.match(helpers.renderLaneMatchupCard(data, {}), /\+7\.0%/);
  data.players[0].championPickIntent = 69;
  assert.match(helpers.renderLaneMatchupCard(data, {}), /卡西奥佩娅/, "own preview retains candidates");
  data.players[0].championPickIntent = 0;
  delete data.players[0].rank;
  data.players[1].position = "middle";
  data.players[2].position = "top";
  const swapped = helpers.ensureLaneMatchupCandidates(data);
  assert.equal(requests.length, 2, "position swap requests the new opposing champion");
  assert.equal(new URL(requests[1], "http://local").searchParams.get("champion"), "64");
  assert.equal(new URL(requests[1], "http://local").searchParams.get("tier"), "emerald_plus", "uses recommendation tier even without rank");
  finishes[1]({ counters: { weakAgainst: [] } });
  await swapped;
  data.phase = "InProgress";
  await helpers.ensureLaneMatchupCandidates(data);
  assert.equal(helpers.renderLaneMatchupCard(data, {}), "");
  assert.equal(requests.length, 2);
});

test("R168 candidate failure and skipped context emit distinguishable diagnostics", async () => {
  const data = { phase: "ChampSelect", queueId: 440, gameId: 123, players: [
    { isCurrent: true, teamId: 100, position: "top", championId: 0 },
    { teamId: 200, position: "top", championId: 62, championLocked: true },
  ] };
  const state = { live: data, section: "live", liveGameGeneration: 1, laneMatchupCandidates: new Map() };
  const diagnostics = [];
  const { ensureLaneMatchupCandidates } = compile(["laneMatchupContext", "laneMatchupOwnChampionId", "laneMatchupTier", "laneMatchupCandidateKey", "recordLaneMatchupCandidateDiagnostic", "ensureLaneMatchupCandidates"], {
    state, livePositionValue: value => value || "", URLSearchParams,
    api: async () => { throw Object.assign(Error("upstream failed"), { status: 503 }); },
    recordItemSetClientDiagnostic: (event, reason, context) => diagnostics.push({ event, reason, context }),
    renderLive() {},
  });
  await ensureLaneMatchupCandidates(data);
  assert.deepEqual(diagnostics.map((item) => item.reason), ["requested", "failed"]);
  assert.equal(diagnostics[1].context.httpStatus, 503);
  assert.equal(diagnostics[1].context.enemyChampionId, 62);
  data.players[0].championId = 8;
  data.players[0].championLocked = true;
  await ensureLaneMatchupCandidates(data);
  await ensureLaneMatchupCandidates(data);
  assert.equal(diagnostics.filter((item) => item.reason === "own-champion-selected").length, 1);
  assert.doesNotMatch(JSON.stringify(diagnostics), /upstream failed/);
});

// P0-1：赛季扫描没跑完时不得用半成品战绩覆盖上游真实胜负场；而且降级说明必须真的
// 渲染到用户屏幕上——工单原则「后端设置了降级说明 ≠ 已披露」。
test("R117 rank win-rate degradation is disclosed on screen, not only set on the backend", () => {
  const helpers = compile(["renderRanks"], {
    escapeHTML: (value) => String(value ?? ""),
    number: (value) => String(value ?? 0),
    percent: (value) => `${value}%`,
    rankCrestIcon: (tier) => `<img class="rank-crest-icon" data-tier="${tier}">`,
    rankTitle: (rank) => rank.tier,
    renderRankMMRPopover: () => "",
    renderRankHistory: () => "",
  });
  const capability = (detail) => [{ name: "ranked-stats", state: "failed", detail }];
  const incomplete = [{ queueType: "RANKED_SOLO_5x5", tier: "大师", leaguePoints: 120, wins: 107, losses: 0, winRate: -1 }];

  // 扫描仍在进行：不得出现胜率数字，必须是「正在统计中」这条明确降级标记。
  const collecting = helpers.renderRanks(incomplete, capability("客户端未返回排位负场，胜率暂不展示"), [], [], { collecting: true });
  assert.doesNotMatch(collecting, /win-rate-value/, "半成品战绩不得渲染成胜率");
  assert.match(collecting, /107胜 · 正在统计中/);
  assert.match(collecting, /data-tooltip="正在后台按当前队列统计本赛季战绩，完成后会自动更新胜率"/);

  // 扫描完成但上游确实没给负场：必须把后端写的 capability.detail 披露出来。
  const degraded = helpers.renderRanks(incomplete, capability("上游未返回排位负场，已按赛季战绩聚合补全胜率"), [], [], { collecting: false });
  assert.doesNotMatch(degraded, /win-rate-value/);
  assert.match(degraded, /107胜 · 负场未提供/);
  assert.match(degraded, /胜率暂不可用/);
  assert.match(degraded, /data-tooltip="上游未返回排位负场，已按赛季战绩聚合补全胜率"/, "capability.detail 必须落到屏幕上");

  // 对抗变异：把 capability.Detail 改成空串必须有测试察觉——兜底文案要顶上。
  const emptyDetail = helpers.renderRanks(incomplete, capability(""), [], [], { collecting: false });
  assert.match(emptyDetail, /data-tooltip="上游未提供负场，无法计算胜率"/, "detail 为空时必须用兜底说明，不能留空 tooltip");

  // 胜率已知时照常渲染数字，且不再挂降级提示。
  const known = helpers.renderRanks(
    [{ queueType: "RANKED_SOLO_5x5", tier: "大师", leaguePoints: 120, wins: 60, losses: 47, winRate: 57 }],
    capability(""), [], [], null);
  assert.match(known, /胜率 <b class="win-rate-value">57%<\/b>/);
  assert.match(known, /60胜 47负/);
  assert.doesNotMatch(known, /胜率暂不可用/);
});

test("R173 rune equipment places opening items, divider and seven final slots in exact order", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const { renderRuneEquipment } = compile(["renderRuneEquipment"], {
    renderItemIcon: id => '<i data-item="' + id + '"></i>',
  });
  const html = renderRuneEquipment({ starterItemIds: [1055, 2003, 2003, 1086, 1001, 1056, 9999], itemIds: [3006, 3072, 3031, 3085, 3036, 3139, 3364, 8888] });
  const dom = new JSDOM(html), equipment = dom.window.document.querySelector(".specialist-game-items");
  assert.equal(equipment.children[0].textContent, "装备");
  assert.deepEqual([...equipment.children[1].children].map(node => node.classList.contains("route-divider") ? "|" : Number(node.dataset.item)),
    [1055, 2003, 2003, 1086, 1001, 1056, "|", 3006, 3072, 3031, 3085, 3036, 3139, 3364]);
  assert.equal(equipment.querySelector(".route-divider").getAttribute("aria-hidden"), "true");
  assert.equal(equipment.querySelectorAll("[data-tooltip],[title]").length, 0);
});

test("R173 absent opening items retains byte-identical final equipment markup", () => {
  const renderItemIcon = id => '<i data-item="' + id + '"></i>';
  const { renderRuneEquipment } = compile(["renderRuneEquipment"], { renderItemIcon });
  const itemIDs = [1055, 2003, 3006], old = '<div class="specialist-game-items"><span>最终装备</span><div>' + itemIDs.map(renderItemIcon).join("") + '</div></div>';
  assert.equal(renderRuneEquipment({ itemIds: itemIDs }), old);
  assert.equal(renderRuneEquipment({ itemIds: itemIDs, starterItemIds: [] }), old);
  assert.equal(renderRuneEquipment({ itemIds: [], starterItemIds: [1055] }), "");
});

test("R173 delayed equipment patch preserves row identity, order and selected state", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const dom = new JSDOM('<section><div data-rune-choice="new" class="specialist-game-row is-selected" aria-checked="true"><div class="specialist-game-items"><span>最终装备</span><div></div></div></div><div data-rune-choice="old" class="specialist-game-row" aria-checked="false"><div class="specialist-game-items"><span>最终装备</span><div></div></div></div></section>');
  const document = dom.window.document, root = document.querySelector("section"), before = [...root.children];
  let remembered = 0;
  const { patchRuneStarterEquipment } = compile(["patchRuneStarterEquipment"], {
    document, nodes: { liveContent: root }, renderItemIcon: id => '<i data-item="' + id + '"></i>',
    prepareImages: () => {}, rememberLiveRecommendationMarkup: () => remembered++,
  });
  patchRuneStarterEquipment([{ key: "old", itemIds: [3031], starterItemIds: [2003, 2003] }, { key: "new", itemIds: [3006], starterItemIds: [1055] }]);
  assert.deepEqual([...root.children], before);
  assert.deepEqual([...root.children].map(row => row.dataset.runeChoice), ["new", "old"]);
  assert.equal(before[0].getAttribute("aria-checked"), "true");
  assert.equal(before[0].classList.contains("is-selected"), true);
  assert.equal(before[1].getAttribute("aria-checked"), "false");
  assert.deepEqual([...before[0].querySelectorAll("[data-item]")].map(node => Number(node.dataset.item)), [1055, 3006]);
  assert.equal(remembered, 1);
});

test("R173 second stage loads only visible rows and leaves rune list available while pending", async () => {
  for (const [source, cap] of [["specialist", 3], ["pro", 5]]) {
    const runes = Array.from({ length: 12 }, (_, index) => ({ key: "row-" + index, playedAt: 1700000000000 + index, itemIds: [3006] }));
    const state = { live: { phase: "ChampSelect" }, section: "live", runeSourceTab: source, liveGameGeneration: 1 };
    let resolve, request, patches = 0;
    const funcs = compile(["ensureRuneStarterItems"], {
      state, nodes: { liveContent: { querySelectorAll: () => runes.slice(0, cap).map(row => ({ dataset: { runeChoice: row.key } })) } },
      specialistRequestTarget: () => ({ key: "64:mid", championId: 64, position: "mid" }),
      proRequestTarget: () => ({ key: "64:mid", championId: 64, position: "mid" }),
      specialistRunesFor: () => runes, proRunesFor: () => runes,
      api: (_url, options) => { request = JSON.parse(options.body); return new Promise(done => resolve = done); },
      patchRuneStarterEquipment: rows => { patches++; assert.equal(rows, runes); },
    });
    const pending = funcs.ensureRuneStarterItems(state.live);
    assert.equal(request.rows.length, cap);
    assert.deepEqual(request.rows.map(row => row.key), runes.slice(0, cap).map(row => row.key));
    assert.ok(runes.every(row => row.starterItemIds === undefined));
    await funcs.ensureRuneStarterItems(state.live);
    resolve({ starters: [{ key: runes[0].key, playedAt: runes[0].playedAt, starterItemIds: [1055, 2003] }, { key: runes[1].key, playedAt: 1, starterItemIds: [9999] }] });
    await pending;
    assert.deepEqual(runes[0].starterItemIds, [1055, 2003]);
    assert.equal(runes[1].starterItemIds, undefined, "recycled row keys must match timestamp too");
    assert.equal(patches, 1);
    assert.deepEqual(runes.map(row => row.key), Array.from({ length: 12 }, (_, index) => "row-" + index));
  }
});

test("R173 optional failure and previous game response leave current equipment unchanged", async () => {
  const runes = [{ key: "row-1", playedAt: 1700000000000, itemIds: [3006] }];
  const state = { live: { phase: "ChampSelect" }, section: "live", runeSourceTab: "specialist", liveGameGeneration: 1 };
  let resolve, patchCount = 0, calls = 0;
  const funcs = compile(["ensureRuneStarterItems"], {
    state, nodes: { liveContent: { querySelectorAll: () => [{ dataset: { runeChoice: "row-1" } }] } },
    specialistRequestTarget: () => ({ key: "64:mid", championId: 64, position: "mid" }),
    specialistRunesFor: () => runes,
    api: () => { calls++; return new Promise(done => resolve = done); },
    patchRuneStarterEquipment: () => patchCount++,
  });
  const pending = funcs.ensureRuneStarterItems(state.live);
  state.liveGameGeneration++;
  resolve({ starters: [{ key: "row-1", playedAt: runes[0].playedAt, starterItemIds: [1055] }] });
  await pending;
  assert.equal(runes[0].starterItemIds, undefined);
  assert.equal(patchCount, 0);
  state.runeStarterRequests.clear();
  const failing = compile(["ensureRuneStarterItems"], {
    state, nodes: { liveContent: { querySelectorAll: () => [{ dataset: { runeChoice: "row-1" } }] } },
    specialistRequestTarget: () => ({ key: "64:mid", championId: 64, position: "mid" }), specialistRunesFor: () => runes,
    api: async () => { calls++; throw new Error("HTTP 429"); }, patchRuneStarterEquipment: () => patchCount++,
  });
  await failing.ensureRuneStarterItems(state.live);
  await failing.ensureRuneStarterItems(state.live);
  assert.equal(calls, 2, "failed optional fetch must enter cooldown");
  assert.equal(patchCount, 0);
  assert.equal(runes[0].starterItemIds, undefined);
});

test("R173 a refreshed pro list retains completed opening items without affecting source order", () => {
  const { retainRuneStarterItems } = compile(["retainRuneStarterItems"]);
  const previous = [{ key: "pro-1", playedAt: 1, starterItemIds: [1055, 2003] }, { key: "pro-2", playedAt: 2, starterItemIds: [] }];
  const next = [{ key: "pro-2", playedAt: 2 }, { key: "pro-1", playedAt: 1 }, { key: "pro-1", playedAt: 3 }];
  retainRuneStarterItems(next, previous);
  assert.deepEqual(next.map(row => row.key), ["pro-2", "pro-1", "pro-1"]);
  assert.deepEqual(next[0].starterItemIds, []);
  assert.deepEqual(next[1].starterItemIds, [1055, 2003]);
  assert.notEqual(next[1].starterItemIds, previous[0].starterItemIds);
  assert.equal(next[2].starterItemIds, undefined);
});

function r174EquipmentRenderer() {
  const { renderRuneEquipment } = compile(["renderRuneEquipment"], {
    renderItemIcon: id => '<i data-item="' + id + '"></i>',
    renderSummonerSpellIcon: id => '<i data-spell="' + id + '"></i>',
  });
  return renderRuneEquipment;
}

test("R174 equipment then spells are direct children and spells contain only two icons", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const html = r174EquipmentRenderer()({ itemIds: [3006, 3031], spell1Id: 4, spell2Id: 11 });
  const row = new JSDOM(html).window.document.querySelector(".specialist-game-items");
  assert.equal(row.children.length, 3);
  assert.equal(row.children[0].tagName, "SPAN");
  assert.equal(row.children[0].textContent, "最终装备");
  assert.deepEqual([...row.children[1].children].map(node => Number(node.dataset.item)), [3006, 3031]);
  const spells = row.lastElementChild;
  assert.equal(spells.className, "specialist-game-spells");
  assert.deepEqual([...spells.children].map(node => Number(node.dataset.spell)), [4, 11]);
  assert.equal(spells.querySelectorAll("small,span,label,[data-tooltip],[title]").length, 0);
  assert.equal(spells.textContent, "");
  assert.doesNotMatch(html, /召唤师技能|rune-spell-row/);
});

test("R174 opening inventory and divider precede final inventory and rightmost spells", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const html = r174EquipmentRenderer()({ starterItemIds: [1055, 2003, 2003], itemIds: [3006, 3031, 3364], spell1Id: 4, spell2Id: 11 });
  const row = new JSDOM(html).window.document.querySelector(".specialist-game-items");
  assert.equal(row.children[0].textContent, "装备");
  assert.deepEqual([...row.children].map(node => node.className || node.tagName), ["SPAN", "DIV", "specialist-game-spells"]);
  assert.deepEqual([...row.children[1].children].map(node => node.classList.contains("route-divider") ? "|" : Number(node.dataset.item)), [1055, 2003, 2003, "|", 3006, 3031, 3364]);
  assert.deepEqual([...row.lastElementChild.children].map(node => Number(node.dataset.spell)), [4, 11]);
});

test("R174 invalid or absent spell pair retains byte-identical equipment markup", () => {
  const render = r174EquipmentRenderer();
  const oldFinal = '<div class="specialist-game-items"><span>最终装备</span><div><i data-item="3006"></i></div></div>';
  const oldOpening = '<div class="specialist-game-items"><span>装备</span><div><i data-item="1055"></i><span class="route-divider" aria-hidden="true"></span><i data-item="3006"></i></div></div>';
  for (const [spell1Id, spell2Id] of [[undefined, undefined], [undefined, 11], [4, 0], [0, 11], [4, 4], [-1, 4], [4.5, 11], [4, Infinity], [4, 100001]]) {
    assert.equal(render({ itemIds: [3006], spell1Id, spell2Id }), oldFinal);
    assert.equal(render({ itemIds: [3006], starterItemIds: [1055], spell1Id, spell2Id }), oldOpening);
    assert.equal(render({ itemIds: [], spell1Id, spell2Id }), "");
  }
  assert.match(render({ itemIds: [3006], spell1Id: "4", spell2Id: "11" }), /specialist-game-spells/);
});

test("R174 spells without final inventory render one right-aligned container without labels or empty group", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const html = r174EquipmentRenderer()({ starterItemIds: [1055], itemIds: [], spell1Id: 4, spell2Id: 11 });
  const row = new JSDOM(html).window.document.querySelector(".specialist-game-items");
  assert.ok(row, "spells cannot disappear when final equipment is absent");
  assert.equal(row.children.length, 1);
  assert.equal(row.children[0].className, "specialist-game-spells");
  assert.equal(row.querySelector(":scope > span"), null);
  assert.equal(row.querySelectorAll("[data-item]").length, 0);
  assert.deepEqual([...row.children[0].children].map(node => Number(node.dataset.spell)), [4, 11]);
});

test("R174 full specialist and pro templates have no standalone spell row", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const state = { live: {}, specialistPlayerTabs: new Map(), selectedRecommendation: "game-1" };
  const funcs = compile(["renderSpecialistPlayers", "proRuneRecordLabel"], {
    state, proRequestTarget: () => ({ key: "64:mid" }), specialistRequestTarget: () => ({ key: "64:mid" }),
    positionLabel: x => x, livePositionDisplay: x => x, escapeHTML: x => String(x ?? ""),
    runeConfigurationTitle: () => "符文", renderUnifiedRuneBoard: () => "<runes></runes>",
    renderItemIcon: id => '<i data-item="' + id + '"></i>',
    renderSummonerSpellIcon: id => '<i data-spell="' + id + '"></i>', iconFigure: () => "",
  });
  for (const source of ["specialist", "pro"]) {
    const html = funcs.renderSpecialistPlayers([{ key: "game-1", playerName: "Fixture", itemIds: [3006], spell1Id: 4, spell2Id: 11, recordGames: 1, recordWins: 1 }], source);
    const doc = new JSDOM(html).window.document;
    assert.equal(doc.querySelectorAll(".rune-spell-row").length, 0);
    assert.equal(doc.querySelectorAll(".specialist-game-items > .specialist-game-spells").length, 1);
    assert.equal(doc.querySelectorAll("[data-spell]").length, 2);
  }
});

test("R174 async opening inventory replacement preserves rightmost spell pair and selected rows", () => {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const render = r174EquipmentRenderer();
  const first = { key: "new", itemIds: [3006], spell1Id: 4, spell2Id: 11 };
  const second = { key: "old", itemIds: [3031], spell1Id: 4, spell2Id: 7 };
  const dom = new JSDOM('<section><div class="specialist-game-row is-selected" aria-checked="true" data-rune-choice="new">' + render(first) + '</div><div class="specialist-game-row" aria-checked="false" data-rune-choice="old">' + render(second) + '</div></section>');
  const document = dom.window.document, root = document.querySelector("section"), rows = [...root.children];
  const beforePairs = rows.map(row => [...row.querySelectorAll("[data-spell]")].map(node => Number(node.dataset.spell)));
  const { patchRuneStarterEquipment } = compile(["patchRuneStarterEquipment"], {
    document, nodes: { liveContent: root }, prepareImages: () => {}, rememberLiveRecommendationMarkup: () => {},
    renderItemIcon: id => '<i data-item="' + id + '"></i>', renderSummonerSpellIcon: id => '<i data-spell="' + id + '"></i>',
  });
  patchRuneStarterEquipment([{ ...first, starterItemIds: [1055, 2003] }, { ...second, starterItemIds: [2003, 2003] }]);
  assert.deepEqual([...root.children], rows);
  assert.equal(rows[0].classList.contains("is-selected"), true);
  assert.equal(rows[0].getAttribute("aria-checked"), "true");
  assert.equal(rows[1].getAttribute("aria-checked"), "false");
  assert.deepEqual(rows.map(row => [...row.querySelectorAll("[data-spell]")].map(node => Number(node.dataset.spell))), beforePairs);
  for (const row of rows) {
    assert.equal(row.querySelector(".specialist-game-items").lastElementChild.className, "specialist-game-spells");
    assert.equal(row.querySelectorAll(".specialist-game-spells").length, 1);
    assert.equal(row.querySelectorAll(".rune-spell-row").length, 0);
  }
});

test("R174 standalone spell renderer remains byte-identical; R179 inventory keeps a fixed height", () => {
  const { renderRuneSpellPair } = compile(["renderRuneSpellPair"], {
    state: { summonerSpells: { spells: [{ id: 4, name: "闪现" }, { id: 11, name: "惩戒" }] } },
    escapeHTML: x => String(x), renderSummonerSpellIcon: id => '<i data-spell="' + id + '"></i>',
  });
  assert.equal(renderRuneSpellPair([4, 11]), '<div class="rune-spell-row"><span>召唤师技能</span><div class="rune-spell-items"><span class="rune-spell-item"><i data-spell="4"></i><small>闪现</small></span><span class="rune-spell-item"><i data-spell="11"></i><small>惩戒</small></span></div></div>');
  const rule = cssSource.match(/\.specialist-game-spells\s*\{([^}]+)\}/)[1];
  for (const declaration of ["display: flex", "flex: 0 0 auto", "flex-wrap: nowrap", "margin-left: auto", "min-width: auto"]) assert.ok(rule.includes(declaration), declaration);
  assert.match(cssSource, /\.specialist-game-items > div:not\(\.specialist-game-spells\).*flex-wrap: nowrap/);
});

function r177FakeClock() {
  let now = 0, serial = 0;
  const timers = new Map();
  return {
    timers,
    setTimeout(fn, ms) { const id = ++serial; timers.set(id, { fn, at: now + ms }); return id; },
    clearTimeout(id) { timers.delete(id); },
    async advance(ms) {
      now += ms;
      for (const [id, timer] of [...timers]) if (timer.at <= now) { timers.delete(id); timer.fn(); }
      // Drain the async api/ensure/finally chain without using real timers.
      for (let index = 0; index < 12; index++) await Promise.resolve();
    },
  };
}

function r177StarterHarness(responses) {
  const { JSDOM } = require("../../desktop/node_modules/jsdom");
  const runes = [{ key: "row-1", playedAt: 1000, itemIds: [3006, 3031], spell1Id: 4, spell2Id: 11 }];
  const render = r174EquipmentRenderer();
  const dom = new JSDOM(`<section><div class="specialist-game-row" data-rune-choice="row-1">${render(runes[0])}</div></section>`, { pretendToBeVisual: true });
  const state = { section: "live", live: { phase: "ChampSelect" }, runeSourceTab: "specialist", liveGameGeneration: 1, runeStarterRequests: new Map() };
  const clock = r177FakeClock(), calls = [];
  let targetKey = "64:mid";
  const funcs = compile(["ensureRuneStarterItems", "patchRuneStarterEquipment", "clearRuneStarterRetries", "resetLiveGameScopedState"], {
    state, nodes: { liveContent: dom.window.document.querySelector("section") }, document: dom.window.document,
    setTimeout: clock.setTimeout, clearTimeout: clock.clearTimeout,
    specialistRequestTarget: () => ({ key: targetKey, championId: 64, position: "mid" }),
    proRequestTarget: () => ({ key: targetKey, championId: 64, position: "mid" }),
    specialistRunesFor: () => runes, proRunesFor: () => runes,
    renderRuneEquipment: render, renderItemIcon: id => `<i data-item="${id}"></i>`, renderSummonerSpellIcon: id => `<i data-spell="${id}"></i>`, prepareImages() {}, rememberLiveRecommendationMarkup() {},
    api: async (_url, options) => { calls.push(JSON.parse(options.body)); const response = responses[Math.min(calls.length - 1, responses.length - 1)]; if (response instanceof Error) throw response; return response; },
  });
  return { state, clock, calls, funcs, runes, dom, changeTarget(key) { targetKey = key; } };
}

test("R177 starter rate hint retries at eight seconds twice and keeps one flight", async () => {
  const h = r177StarterHarness([{ starters: [], retryAfterSeconds: 8 }]);
  try {
    await h.funcs.ensureRuneStarterItems(h.state.live);
    assert.equal(h.calls.length, 1);
    assert.equal(h.clock.timers.size, 1);
    await h.funcs.ensureRuneStarterItems(h.state.live);
    await h.clock.advance(7999);
    assert.equal(h.calls.length, 1);
    await h.clock.advance(1);
    assert.equal(h.calls.length, 2);
    assert.equal(h.clock.timers.size, 1);
    await h.clock.advance(8000);
    assert.equal(h.calls.length, 3);
    assert.equal(h.clock.timers.size, 0);
    await h.clock.advance(90000);
    assert.equal(h.calls.length, 3);
    assert.deepEqual(h.calls.map(call => call.rows), Array(3).fill([{ key: "row-1", playedAt: 1000 }]));
  } finally { h.dom.window.close(); }
});

test("R177 starter waiting retry rejects a new generation source target or invisible page", async () => {
  for (const change of [
    h => h.state.liveGameGeneration++,
    h => h.state.runeSourceTab = "opgg",
    h => h.changeTarget("103:mid"),
    h => h.state.section = "overview",
    h => h.state.live.phase = "Lobby",
  ]) {
    const h = r177StarterHarness([{ starters: [], retryAfterSeconds: 8 }]);
    try {
      await h.funcs.ensureRuneStarterItems(h.state.live);
      assert.equal(h.clock.timers.size, 1);
      change(h);
      await h.clock.advance(8000);
      assert.equal(h.calls.length, 1);
      assert.equal(h.clock.timers.size, 0);
    } finally { h.dom.window.close(); }
  }
  for (const cancel of [h => h.funcs.resetLiveGameScopedState(), h => h.funcs.clearRuneStarterRetries(), async h => { h.changeTarget("103:mid"); await h.funcs.ensureRuneStarterItems(h.state.live); }]) {
    const h = r177StarterHarness([{ starters: [], retryAfterSeconds: 8 }]);
    try {
      await h.funcs.ensureRuneStarterItems(h.state.live);
      const oldTimer = [...h.clock.timers.keys()][0];
      await cancel(h);
      assert.equal(h.clock.timers.has(oldTimer), false, "scope change cancels old timer immediately");
    } finally { h.funcs.clearRuneStarterRetries(); h.dom.window.close(); }
  }
});

test("R177 starter no quota hint or network failure never schedules a retry", async () => {
  for (const response of [{ starters: [] }, new Error("network failed")]) {
    const h = r177StarterHarness([response]);
    try {
      await h.funcs.ensureRuneStarterItems(h.state.live);
      await h.clock.advance(90000);
      assert.equal(h.calls.length, 1);
      assert.equal(h.clock.timers.size, 0);
    } finally { h.dom.window.close(); }
  }
});

test("R177 starter retry success patches opening items and retains rightmost R174 spells", async () => {
  const h = r177StarterHarness([{ starters: [], retryAfterSeconds: 8 }, { starters: [{ key: "row-1", playedAt: 1000, starterItemIds: [1055, 2003] }], retryAfterSeconds: 8 }]);
  try {
    await h.funcs.ensureRuneStarterItems(h.state.live);
    await h.clock.advance(8000);
    assert.deepEqual(h.runes[0].starterItemIds, [1055, 2003]);
    const row = h.dom.window.document.querySelector(".specialist-game-items");
    assert.deepEqual([...row.querySelectorAll("[data-item]")].map(icon => Number(icon.dataset.item)), [1055, 2003, 3006, 3031]);
    assert.ok(row.querySelector(".route-divider"));
    assert.equal(row.lastElementChild.className, "specialist-game-spells");
    assert.deepEqual([...row.lastElementChild.children].map(icon => Number(icon.dataset.spell)), [4, 11]);
    assert.equal(h.clock.timers.size, 0, "completed row does not retry even with a stale hint");
  } finally { h.dom.window.close(); }
});

function r177LaneHarness(enemies, shares) {
  const data = { phase: "ChampSelect", gameId: 123, queueId: 440, players: [
    { isCurrent: true, isAlly: true, teamId: 100, position: "mid", championId: 0, rank: { tier: "GOLD" } },
    ...enemies.map(enemy => ({ teamId: 200, championLocked: true, position: "", ...enemy })),
  ] };
  const state = { live: data, section: "live", liveGameGeneration: 1, laneMatchupCandidates: new Map(), laneMatchupLanes: new Map() };
  const requests = [], diagnostics = [];
  const funcs = compile(["ensureLaneMatchupCandidates", "laneMatchupContext", "renderLaneMatchupCard", "recordLaneMatchupCandidateDiagnostic", "laneMatchupCandidateKey", "laneMatchupOwnChampionId"], {
    state, livePositionValue: value => ({ middle: "mid", bottom: "adc", utility: "support" })[value] || value || "", URLSearchParams,
    api: async path => { requests.push(path); if (path.includes("champion-lanes")) { if (shares instanceof Error) throw shares; return shares; } return { counters: { weakAgainst: [{ championId: 69, name: "卡西奥佩娅", winRate: 43 }] } }; },
    recordItemSetClientDiagnostic: (_event, reason, context) => diagnostics.push({ reason, context }), renderLive() {},
    iconFigure: (_kind, id) => `<i data-champion="${id}"></i>`, rate: value => `${value}%`, escapeHTML: String,
  });
  return { data, state, requests, diagnostics, funcs };
}

test("R177 unique unknown enemy lane produces a card and caches champion tier shares", async () => {
  const h = r177LaneHarness([{ championId: 103 }], { 103: [{ position: "mid", rate: 0.7 }], 64: [{ position: "mid", rate: 0.8 }] });
  await h.funcs.ensureLaneMatchupCandidates(h.data);
  assert.match(h.funcs.renderLaneMatchupCard(h.data, {}), /data-lane-matchup-card/);
  assert.equal(h.funcs.laneMatchupContext(h.data).enemy.championId, 103);
  await h.funcs.ensureLaneMatchupCandidates(h.data);
  assert.equal(h.requests.filter(url => url.includes("champion-lanes")).length, 1);
  assert.equal(new URL(h.requests[0], "http://local").searchParams.get("tier"), "emerald_plus");
  h.data.players[1].championId = 64;
  await h.funcs.ensureLaneMatchupCandidates(h.data);
  const requests = h.requests.filter(url => url.includes("champion-lanes"));
  assert.equal(requests.length, 2);
  assert.equal(new URL(requests[1], "http://local").searchParams.get("champions"), "64");
  h.data.players[1].championId = 103;
  await h.funcs.ensureLaneMatchupCandidates(h.data);
  assert.equal(h.requests.filter(url => url.includes("champion-lanes")).length, 2);
});

test("R177 competing lane shares require half and double advantage or remain ambiguous", async () => {
  for (const [first, second, visible] of [[0.6, 0.4, false], [0.49, 0.25, false], [0.5, 0.25, true], [0.3, 0.1, true], [0.24, 0.1, false]]) {
    const h = r177LaneHarness([{ championId: 103 }, { championId: 64 }], { 103: [{ position: "mid", rate: first }], 64: [{ position: "mid", rate: second }] });
    await h.funcs.ensureLaneMatchupCandidates(h.data);
    assert.equal(Boolean(h.funcs.renderLaneMatchupCard(h.data, {})), visible, `shares ${first}/${second}`);
    if (first >= 0.25 && second >= 0.25 && !visible) assert.ok(h.diagnostics.some(event => event.reason === "lane-ambiguous"));
    if (!visible) assert.equal(h.requests.filter(url => url.includes("champions/detail")).length, 0);
  }
});

test("R177 known enemy position wins over conflicting cached inference without lane requests", async () => {
  const h = r177LaneHarness([{ championId: 103, position: "mid" }, { championId: 64 }], {});
  h.state.laneMatchupLanes.set("64:emerald_plus", { status: "succeeded", rows: [{ position: "mid", rate: 0.9 }] });
  await h.funcs.ensureLaneMatchupCandidates(h.data);
  assert.equal(h.funcs.laneMatchupContext(h.data).enemy.championId, 103);
  assert.equal(h.requests.filter(url => url.includes("champion-lanes")).length, 0);
  assert.equal(new URL(h.requests[0], "http://local").searchParams.get("champion"), "103");
});

test("R177 missing competitor shares or failed provider cannot invent an enemy lane", async () => {
  for (const shares of [{}, new Error("failed"), { 103: [{ position: "mid", rate: 0.7 }] }, { 103: [{ position: "mid", rate: 70 }], 64: [] }]) {
    const h = r177LaneHarness([{ championId: 103 }, { championId: 64 }], shares);
    await h.funcs.ensureLaneMatchupCandidates(h.data);
    assert.equal(h.funcs.renderLaneMatchupCard(h.data, {}), "");
    assert.equal(h.requests.filter(url => url.includes("champions/detail")).length, 0);
    await h.funcs.ensureLaneMatchupCandidates(h.data);
    assert.equal(h.requests.filter(url => url.includes("champion-lanes")).length, 1, "failed share fetch is cached too");
  }
});

test("R177 unavailable diagnostics follow shape changes and cap at twenty per game", async () => {
  const h = r177LaneHarness([], {});
  h.data.players[0].position = "";
  for (let locked = 0; locked <= 5; locked++) {
    for (let known = 0; known <= 5; known++) {
      h.data.players = [h.data.players[0], ...Array.from({ length: 5 }, (_, index) => ({ teamId: 200, championId: index + 1, championLocked: index < locked, position: index < known ? "top" : "" }))];
      await h.funcs.ensureLaneMatchupCandidates(h.data);
      await h.funcs.ensureLaneMatchupCandidates(h.data);
    }
  }
  const events = h.diagnostics.filter(event => event.reason === "context-unavailable");
  assert.equal(events.length, 20);
  assert.equal(events[0].context.selfPosition, "");
  assert.equal(events[0].context.enemyLockedCount, 0);
  assert.equal(events[0].context.allyPositionKnownCount, 0);
  assert.ok(new Set(events.map(event => `${event.context.enemyLockedCount}:${event.context.enemyPositionKnownCount}`)).size > 1);
});

test("R177 a pending timed retry stays single flight and a source switch cancels it", async () => {
  let release;
  const pending = new Promise(resolve => { release = resolve; });
  const h = r177StarterHarness([{ starters: [], retryAfterSeconds: 8 }, pending]);
  try {
    await h.funcs.ensureRuneStarterItems(h.state.live);
    await h.clock.advance(8000);
    assert.equal(h.calls.length, 2);
    const [key, entry] = [...h.state.runeStarterRequests][0];
    assert.equal(entry.pending, true);
    await h.funcs.ensureRuneStarterItems(h.state.live, key);
    await h.funcs.ensureRuneStarterItems(h.state.live);
    assert.equal(h.calls.length, 2, "a retry cannot bypass an active request");
    let aborted = 0;
    h.state.controllers = new Map([[`live-rune-starters:${key}`, { abort() { aborted++; } }]]);
    h.state.runeSourceTab = "opgg";
    h.funcs.clearRuneStarterRetries();
    assert.equal(aborted, 1);
    release({ starters: [{ key: "row-1", playedAt: 1000, starterItemIds: [1055] }], retryAfterSeconds: 8 });
    for (let i = 0; i < 12; i++) await Promise.resolve();
    assert.equal(h.runes[0].starterItemIds, undefined);
    assert.equal(h.clock.timers.size, 0);
  } finally { h.dom.window.close(); }
});

test("R177 partial starter patches do not create a fresh retry budget for remaining rows", async () => {
  const h = r177StarterHarness([{ starters: [{ key: "row-1", playedAt: 1000, starterItemIds: [1055] }], retryAfterSeconds: 8 }, { starters: [], retryAfterSeconds: 8 }]);
  try {
    const row = { key: "row-2", playedAt: 2000, itemIds: [3111], spell1Id: 4, spell2Id: 7 };
    h.runes.push(row);
    const element = h.dom.window.document.createElement("div");
    element.className = "specialist-game-row";
    element.dataset.runeChoice = row.key;
    element.innerHTML = r174EquipmentRenderer()(row);
    h.dom.window.document.querySelector("section").append(element);
    await h.funcs.ensureRuneStarterItems(h.state.live);
    await h.funcs.ensureRuneStarterItems(h.state.live);
    assert.equal(h.calls.length, 1);
    await h.clock.advance(8000);
    await h.clock.advance(8000);
    await h.funcs.ensureRuneStarterItems(h.state.live);
    assert.equal(h.calls.length, 3);
    assert.deepEqual(h.calls.slice(1).map(call => call.rows), Array(2).fill([{ key: "row-2", playedAt: 2000 }]));
    assert.ok(h.calls.every(call => call.source === "specialist" && call.championId === 64 && call.position === "mid"));
  } finally { h.dom.window.close(); }
});
