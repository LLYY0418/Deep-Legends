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

function compile(names, dependencies = {}, script = source) {
  names = [...names];
  if (!names.includes("overviewSupplementTarget") && names.some(name => functionSource(script, name).includes("overviewSupplementTarget("))) names.push("overviewSupplementTarget");
  dependencies = { riotTab: tab => tab?.region === "kr", ...dependencies };
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
  assert.match(functions.specialistRequestTarget({}).key, /^222:adc:/);

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

test("R68 optional insight alignment sorts complete positions and fails closed on duplicates", () => {
  const { insightTeamLayout } = compile(["insightTeamLayout"]);
  const positions = ["utility", "bottom", "middle", "jungle", "top"];
  const players = [100, 200].flatMap((teamId) => positions.map((position) => ({ teamId, position })));
  const aligned = insightTeamLayout(players, true);
  assert.equal(aligned.aligned, true);
  assert.deepEqual(aligned.teams.get(100).map((player) => player.position), ["top", "jungle", "middle", "bottom", "utility"]);
  const duplicate = players.map((player, index) => index === 0 ? { ...player, position: "top" } : player);
  const fallback = insightTeamLayout(duplicate, true);
  assert.equal(fallback.aligned, false);
  assert.match(fallback.reason, /重复或缺失/);
});

test("R68 rendered-roster diagnostic carries queueId and absolute counts", () => {
  const state = { liveRosterRenderDiagnostics: new Set() };
  let body;
  const { recordLiveRosterRendered } = compile(["recordLiveRosterRendered"], {
    state,
    fetch: (_url, options) => { body = JSON.parse(options.body); return Promise.resolve(); },
  });
  recordLiveRosterRendered({ phase: "ChampSelect", gameId: 68, queueId: 420, players: [{}, {}] }, 1, 1);
  assert.deepEqual({ queueId: body.queueId, rendered100: body.rendered100, rendered200: body.rendered200 }, { queueId: 420, rendered100: 1, rendered200: 1 });
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
  assert.match(session, /客户端直接给出的组队信息/);
  assert.doesNotMatch(session, /推测/);
  const inferred = renderLivePremadeTag({ ...base[0], premadeSource: "inferred" }, [{ ...base[0], premadeSource: "inferred" }, { ...base[1], premadeSource: "inferred" }]);
  assert.match(inferred, />预组 ×2<\/span>/);
  assert.match(inferred, /推测/);
  const both = renderLivePremadeTag({ ...base[0], premadeSource: "both" }, [{ ...base[0], premadeSource: "both" }, { ...base[1], premadeSource: "both" }]);
  assert.match(both, />组队 ×2<\/span>/);
  assert.match(both, /最近战绩也支持这一判断/);
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
  const { JSDOM } = require("../desktop/node_modules/jsdom");
  const functions = compile(["renderLiveRecentPositions", "renderLivePlayer"], {
    state: {}, number: String, escapeHTML: (value) => String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;"),
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
  assert.match(insights, /orderedPlayers, renderLiveRecentPositions\(player, data.queueId\)\)/);
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
  const state = { live: { mapId: 11 }, specialistPlayerTabs: new Map(), selectedRecommendation: "pro-1" };
  const fn = compile(["renderSpecialistPlayers", "renderRuneSourceSection", "proRuneRecordLabel"], {
    state, proRequestTarget: () => ({key:"69:mid"}), escapeHTML: x=>String(x??""), relativeTime:()=>"刚刚",
    runeConfigurationTitle:()=>"电刑 + 坚决", renderUnifiedRuneBoard:()=>"<runes></runes>", renderItemIcon:id=>`<item>${id}</item>`, iconFigure:()=>"<icon></icon>",
  });
  const row = { key:"pro-1", title:"T1 Faker", playerName:"Faker", championId:69, position:"中路", opponentPlayerName:"GEN Chovy", eventLabel:"LCK · 2026-09-06 · T1 vs GEN 第 1 局", winKnown:false, recordGames:2, recordWins:1, recordPartial:true, selectedComplete:false, itemIds:[3364,2031,6692] };
  let html=fn.renderRuneSourceSection({key:"pro",items:[row],proStatus:{}},true);
  assert.match(html,/specialist-player-tabs/);assert.match(html,/data-player-source="pro"/);
  assert.match(html,/T1 Faker/);assert.match(html,/aria-label="1胜1负，仅已确认场次"/);assert.doesNotMatch(html,/%/);
  assert.match(html,/specialist-game-row is-unknown is-selected/);assert.match(html,/该局胜负未获官方确认/);
  assert.match(html,/LCK · 2026-09-06/);assert.match(html,/GEN Chovy/);assert.match(html,/最终装备/);
  assert.match(html,/上游未提供完整槽位/);assert.doesNotMatch(html,/specialist-opponent-rank/);
  html=fn.renderSpecialistPlayers([{...row,recordGames:3,recordWins:2}],"pro");assert.match(html,/aria-label="2胜1负，仅已确认场次"/);
  html=fn.renderRuneSourceSection({key:"pro",items:[row],proStatus:{readAt:new Date(Date.now()-600000).toISOString()}},true);
  assert.doesNotMatch(html,/缓存数据|无法连接职业赛事数据源/);assert.match(html,/data-retry-pro-runes/);assert.match(html,/当前显示上次读取结果/);
  html=fn.renderRuneSourceSection({key:"pro",items:[],proStatus:{reason:"upstream-timeout"}},true);assert.match(html,/上游超时/);assert.match(html,/data-retry-pro-runes/);
});

test("R75 incomplete pro disables actual action markup and refuses POST", async () => {
  const selection={sourceKey:"pro",selectedComplete:false,selectedPerkIds:[8112,8143,8137,8106,8401,8444,5008,5011]};
  const state={recommendationTab:"runes",live:{available:true,phase:"ChampSelect",players:[{isCurrent:true,championId:69,championName:"卡西奥佩娅"}]}};
  let posts=0;
  const f=compile(["renderRecommendationArea","applyRunes"],{
    state,liveRecommendationTarget:()=>({key:"69:mid"}),liveRecommendationsFor:()=>({}),liveAugmentRecommendationSource:()=>null,
    recommendationCapabilities:()=>({hasRunes:true}),recommendationTabSpecs:()=>[["runes","符文"]],recommendationActiveTab:()=>"runes",
    selectedRuneRecommendation:()=>selection,recommendationPanelBusy:()=>false,renderLiveInsights:()=>"",renderRuneRecommendations:()=>"",
    renderChampionRecommendationHeader:()=>"",renderBuildRecommendation:()=>"",renderRecommendationDataNotices:()=>"",runeConfigurationTitle:()=>"职业符文",escapeHTML:x=>String(x),
    showToast:()=>{},api:async()=>{posts++},
  });
  assert.match(f.renderRecommendationArea(state.live),/data-apply-runes="selected" disabled/);
  await f.applyRunes({currentTarget:{}});assert.equal(posts,0);
  selection.selectedComplete=true;
  assert.match(f.renderRecommendationArea(state.live),/data-apply-runes="selected" disabled/);
  await f.applyRunes({currentTarget:{}});assert.equal(posts,0,"false complete flag must not allow eight perks");
  selection.selectedPerkIds.push(5005);
  assert.doesNotMatch(f.renderRecommendationArea(state.live),/data-apply-runes="selected" disabled/);
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
 const {loadOverview}=compile(['loadOverview'],{tabReady:()=>true,activeTab:()=>tab,rerenderTab:()=>calls.push('render'),loadOPGGSeasonSummary:()=>calls.push('season'),loadOverviewCurrentGame:()=>calls.push('current'),state:{},api:()=>assert.fail('cached overview must not request history')});
 await loadOverview(tab);
 assert.deepEqual(calls,['render','season','current']);
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
