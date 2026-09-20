const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { JSDOM } = require(path.join(__dirname, "..", "..", "desktop", "node_modules", "jsdom"));

const root = path.resolve(__dirname, "..");
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const appScript = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
const appStyles = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
const script = fs.readFileSync(path.join(__dirname, "champions.js"), "utf8");
const styles = fs.readFileSync(path.join(__dirname, "champions.css"), "utf8");

test("R86 ADD-4 retired synergy panel selector stays absent without removing shared card styles", () => {
  const retired = ["arena", "synergy", "panel"].join("-");
  function check(css) {
    const dom = new JSDOM(`<style>${css}</style><div class="champion-list-card"></div>`);
    try {
      const rules = [...dom.window.document.styleSheets[0].cssRules];
      assert.ok(!rules.some(rule => rule.selectorText?.split(",").some(selector => selector.trim() === `.${retired}`)));
      for (const selector of [".champion-list-card", ".augment-directory", ".recommendation-section"]) {
        assert.ok(rules.some(rule => rule.selectorText?.split(",").map(part => part.trim()).includes(selector) && rule.style.getPropertyValue("overflow") === "hidden"));
      }
    } finally { dom.window.close(); }
  }
  check(styles);
  assert.throws(() => check(`${styles}\n.${retired} { overflow: hidden; }`), { name: "AssertionError" });
});
const gameplayScript = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const gameplayStyles = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");
const sharedBuildStyles = fs.readFileSync(path.join(__dirname, "build-item-row.css"), "utf8");
const friendsStyles = fs.readFileSync(path.join(__dirname, "friends.css"), "utf8");
const gameplayBackend = fs.readFileSync(path.join(root, "gameplay.go"), "utf8");
const featuresBackend = fs.readFileSync(path.join(root, "features.go"), "utf8");
const queueGroupsBackend = fs.readFileSync(path.join(root, "queue_groups.go"), "utf8");
const riotBackend = fs.readFileSync(path.join(root, "riot_api.go"), "utf8");
const sgpBackend = fs.readFileSync(path.join(root, "sgp_api.go"), "utf8");
const lcuAPIBackend = fs.readFileSync(path.join(root, "lcu_api.go"), "utf8");
const lcuBackend = fs.readFileSync(path.join(root, "lcu.go"), "utf8");
const lcuEventsBackend = fs.readFileSync(path.join(root, "lcu_events.go"), "utf8");
const connectionManagerBackend = fs.readFileSync(path.join(root, "connection_manager.go"), "utf8");
const catalogBackend = fs.readFileSync(path.join(root, "catalog.go"), "utf8");
const qq101Backend = fs.readFileSync(path.join(root, "qq101.go"), "utf8");
const rankInsightsBackend = fs.readFileSync(path.join(root, "rank_insights.go"), "utf8");
const seasonStatsBackend = fs.readFileSync(path.join(root, "season_stats.go"), "utf8");
const friendsScript = fs.readFileSync(path.join(__dirname, "friends.js"), "utf8");
const demoScript = fs.readFileSync(path.join(__dirname, "demo-data.js"), "utf8");
const backend = fs.readFileSync(path.join(root, "champions.go"), "utf8");
const structuredBackend = fs.readFileSync(path.join(root, "champions_structured.go"), "utf8");
const hexdataBackend = fs.readFileSync(path.join(root, "hexdata.go"), "utf8");
const overviewCacheBackend = fs.readFileSync(path.join(root, "overview_cache.go"), "utf8");
const mainSource = fs.readFileSync(path.join(root, "main.go"), "utf8");
const arenaMatchDetailSource = fs.readFileSync(path.join(root, "arena_match_detail.go"), "utf8");
const yourGGArenaSource = fs.readFileSync(path.join(root, "yourgg_arena.go"), "utf8");
const desktopPackage = JSON.parse(fs.readFileSync(path.join(root, "..", "desktop", "package.json"), "utf8"));
const riotKeyHook = fs.readFileSync(path.join(root, "..", "desktop", "verify-embedded-riot-key.cjs"), "utf8");
const specialistSource = fs.readFileSync(path.join(root, "specialist_runes.go"), "utf8");
const socialBackend = fs.readFileSync(path.join(root, "social.go"), "utf8");
const positionIcons = ["all", "top", "jungle", "middle", "bottom", "utility"].map((name) => fs.readFileSync(path.join(__dirname, "position-icons", `${name}.svg`), "utf8"));
const allPositionIcon = positionIcons[0];

test("R69 career-column reentry preserves match width through the 1600 DIP band", () => {
  assert.match(
    gameplayStyles,
    /@container gameplay-page \(min-width: 1021px\) and \(max-width: 1300px\)\s*\{\s*\.overview-layout\s*\{[^}]*grid-template-columns:\s*300px minmax\(0,\s*1fr\)/s,
  );
});

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
  if (source.slice(Math.max(0, start - 6), start) === "async ") start -= 6;
  const parametersStart = source.indexOf("(", start);
  assert.notEqual(parametersStart, -1, `${name} parameters not found`);
  let parametersEnd = -1;
  let parameterDepth = 0;
  let parameterQuote = "";
  let parameterEscaped = false;
  for (let index = parametersStart; index < source.length; index += 1) {
    const character = source[index];
    if (parameterQuote) {
      if (parameterEscaped) parameterEscaped = false;
      else if (character === "\\") parameterEscaped = true;
      else if (character === parameterQuote) parameterQuote = "";
      continue;
    }
    if (character === '"' || character === "'" || character === "`") {
      parameterQuote = character;
      continue;
    }
    if (character === "(") parameterDepth += 1;
    if (character === ")" && --parameterDepth === 0) {
      parametersEnd = index;
      break;
    }
  }
  assert.notEqual(parametersEnd, -1, `${name} parameters are not balanced`);
  const bodyStart = source.indexOf("{", parametersEnd);
  assert.notEqual(bodyStart, -1, `${name} body not found`);
  let depth = 0;
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
    if (character === '"' || character === "'" || character === "`") {
      quote = character;
      continue;
    }
    if (character === "{") depth += 1;
    if (character === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} body is not balanced`);
}

// Go 版 functionSource：匹配 `func Name(` 与 `func (recv T) Name(` 两种声明。
function goFunctionSource(source, name) {
  const declaration = new RegExp(`^func (?:\\([^)]*\\) )?${name}\\(`, "m");
  const match = declaration.exec(source);
  assert.notEqual(match, null, `${name} source not found`);
  const start = match.index;
  let depth = 0;
  for (let index = source.indexOf("{", start); index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} body is not balanced`);
}

function cssBlockAfter(source, marker) {
  const start = source.indexOf(marker);
  assert.notEqual(start, -1, `CSS marker not found: ${marker}`);
  const bodyStart = source.indexOf("{", start);
  assert.notEqual(bodyStart, -1, `CSS block body not found: ${marker}`);
  let depth = 0;
  for (let index = bodyStart; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] === "}" && --depth === 0) return source.slice(bodyStart + 1, index);
  }
  assert.fail(`CSS block is not balanced: ${marker}`);
}

function imageURLStub(source, path) {
  return source && path ? `/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(path)}` : "/image-unavailable.svg";
}

// 取某条 CSS 声明的数值。布局类断言要落在“最终尺寸”上（例如徽章不能再把整条
// 召唤师条撑高），只断言“写了某个属性”挡不住数值回归。
function cssNumber(source, marker, property) {
  const block = cssBlockAfter(source, marker);
  const match = new RegExp(`(?:^|;)\\s*${property}\\s*:\\s*(-?[\\d.]+)`, "m").exec(block);
  assert.ok(match, `${marker} 里找不到 ${property}`);
  return Number(match[1]);
}

function compileFunctions(source, names, dependencies = {}) {
  const bodies = names.map((name) => functionSource(source, name));
  const compiledDependencies = { ...dependencies };
  if (source.includes("function loadOverview(")) {
    compiledDependencies.riotTab ||= tab => tab?.region === "kr";
    for (const name of ["syncOverviewSupplementRefs", "overviewSupplementTarget"]) {
      if (!names.includes(name) && !compiledDependencies[name] && bodies.some(body => body.includes(`${name}(`))) bodies.push(functionSource(source, name));
    }
  }
  // Older focused tests deliberately compile only the function under test.
  // Keep their injected target semantics while production routes through the
  // R68 request target, which has its own dedicated behavioral tests.
  if (bodies.some((body) => body.includes("specialistRequestTarget(")) && !compiledDependencies.specialistRequestTarget) {
    compiledDependencies.specialistRequestTarget = (data) => compiledDependencies.liveRecommendationTarget?.(data) || null;
  }
  if (bodies.some((body) => body.includes("insightTeamLayout(")) && !compiledDependencies.insightTeamLayout) {
    compiledDependencies.insightTeamLayout = (players) => ({
      teams: new Map([100, 200].map((teamID) => [teamID, players.filter((player) => Number(player.teamId) === teamID)])),
      aligned: false,
      reason: "",
    });
  }
  // Compile the actual R75 helpers transitively for focused legacy render tests.
  for (const name of ["proRunesFor", "proRequestTarget", "proBadgeAttributes", "renderProIdentityBadge", "proContextFromButton"]) {
    if (!names.includes(name) && !compiledDependencies[name] && bodies.some(body => body.includes(`${name}(`))) bodies.push(functionSource(source, name));
  }
  const dependencyNames = Object.keys(compiledDependencies);
  const factory = Function(
    ...dependencyNames,
    `"use strict";\n${bodies.join("\n")}\nreturn { ${names.join(", ")} };`,
  );
  return factory(...dependencyNames.map((name) => compiledDependencies[name]));
}

function assertR45Contracts(js = gameplayScript, css = gameplayStyles, goSource = gameplayBackend) {
  const matchBaseEnd = css.indexOf("@container matches-column");
  assert.notEqual(matchBaseEnd, -1, "match-card container layer not found");
  const matchBase = css.slice(0, matchBaseEnd);
  assert.match(cssBlockAfter(matchBase, ".match-stats {"), /align-content:\s*start/);
  const arenaStats = cssBlockAfter(matchBase, ".match-stats.is-arena {");
  assert.match(arenaStats, /align-content:\s*start/);
  assert.match(arenaStats, /grid-template-rows:\s*repeat\(3,18px\)/);
  assert.match(arenaStats, /min-height:\s*54px/);

  const separatedBuild = cssBlockAfter(css, ".live-augment-recommendations + .build-recommendation {");
  assert.match(separatedBuild, /margin-top:\s*20px/);
  const separator = cssBlockAfter(css, ".live-augment-recommendations + .build-recommendation::before {");
  assert.match(separator, /height:\s*1px/);
  assert.match(separator, /content:\s*""/);
  assert.match(cssBlockAfter(css, ".build-summary-bar > .build-summary-skill {"), /grid-column:\s*1\s*\/\s*-1/);
  const arenaCards = cssBlockAfter(css, ".build-recommendation.is-arena-build .late-option-band .config-option,");
  assert.match(arenaCards, /padding:\s*7px 9px/);
  assert.match(arenaCards, /background:\s*var\(--bg\)/);
  assert.match(arenaCards, /border:\s*1px solid var\(--line\)/);

  const buildSource = functionSource(js, "renderBuildRecommendation");
  const prismIndex = buildSource.indexOf("${lateBands}");
  const coreIndex = buildSource.indexOf("${itemBuildLayout}");
  assert.ok(prismIndex >= 0 && coreIndex > prismIndex, "prismatic items must render before core routes");

  // R79 split the guard so the observed gameflow phase can be logged; the
  // cell-identity check now lives in the phase-reporting variant.
  const validationSource = goFunctionSource(goSource, "validateGameplayItemSetContextPhase");
  assert.match(validationSource, /gameplayLivePlayerIsCurrent\(playerReference, current\.PUUID, player\.CellID, session\.LocalPlayerCellID\)/);
  const liveHandlerSource = goFunctionSource(goSource, "loadGameplayLive");
  assert.match(liveHandlerSource, /localPlayerCellID = champSelect\.LocalPlayerCellID/);
  assert.match(liveHandlerSource, /gameplayLivePlayerIsCurrent\(reference, current\.PUUID, raw\.player\.CellID, localPlayerCellID\)/);
}

function assertR46Contracts({
	js = gameplayScript,
	appCSS = appStyles,
	goSource = gameplayBackend,
	hexSource = hexdataBackend,
	cacheSource = overviewCacheBackend,
} = {}) {
	const cachePut = goFunctionSource(cacheSource, "putLocked");
	assert.match(cachePut, /cache\.removeExpiredLocked\(entry\.at\)/);
	assert.match(cachePut, /for len\(cache\.entries\) > overviewQueryCacheMax/);
	assert.match(cachePut, /cache\.removeElementLocked\(cache\.recent\.Back\(\)\)/);

	assert.match(hexSource, /hexdataHeroMetricPattern\s*=\s*regexp\.MustCompile\(`[^`\n]*\[·，,\][^`\n]*`\)/);
	assert.match(hexSource, /hexdataGlobalMetric\s*=\s*regexp\.MustCompile\(`[^`\n]*\[·，,\][^`\n]*`\)/);
	assert.match(goFunctionSource(hexSource, "parseHexdataRarity"), /\[·，,\]/);

	assert.match(goSource, /Augments\s+\[\]championMetricRow\s+`json:"augments"`/);
	assert.doesNotMatch(goSource, /Augments\s+\[\]championMetricRow\s+`json:"augments,omitempty"`/);

	assert.equal((appCSS.match(/\.section-tab > span:not\(\.live-beacon\)/g) || []).length, 2);
	const refreshDelay = functionSource(js, "liveRefreshDelayMs");
	assert.match(refreshDelay, /normalized === "ChampSelect"\) return 3_000/);
	assert.doesNotMatch(refreshDelay, /return 20_000/);
	assert.match(goFunctionSource(goSource, "gameplayLiveUnsupportedReason"), /strings\.EqualFold\(strings\.TrimSpace\(gameMode\), "TFT"\)/);
}

function assertR47Contracts({ js = gameplayScript, css = gameplayStyles, goSource = gameplayBackend } = {}) {
  const arenaMode = goFunctionSource(goSource, "isArenaChampSelectMode");
  assert.match(arenaMode, /strings\.EqualFold\(strings\.TrimSpace\(gameMode\), "CHERRY"\)/);
  assert.match(goSource, /func filterArenaChampSelectPlayers[\s\S]{0,1200}puuid == "" \|\| strings\.EqualFold\(puuid, emptyLCUPlayerPUUID\)/);
  const liveHandler = goFunctionSource(goSource, "loadGameplayLive");
  assert.match(liveHandler, /if isArenaChampSelectMode\(response\.GameMode\)\s*\{[\s\S]*filterArenaChampSelectPlayers\(rawPlayers\)[\s\S]*ChampSelectNotice = arenaChampSelectNotice/);

  const insights = functionSource(js, "renderLiveInsights");
  assert.match(insights, /liveAugmentRecommendationSource\(data\) === "arena"/);
  assert.match(insights, /orderLivePlayers\(players, !arenaMode\)/);
  assert.match(insights, /if \(arenaMode\)/);
  assert.match(insights, /data\.phase === "ChampSelect" \? "己方小队" : "全部玩家"/);
  assert.match(insights, /live-teams is-insight is-arena/);
  assert.doesNotMatch(insights, /data\.champSelectNotice|live-roster-notice/);
  assert.match(functionSource(js, "renderRecommendationArea"), /data\.champSelectNotice[\s\S]*recommendation-tab-row/);
  assert.match(cssBlockAfter(css, ".live-teams.is-arena {"), /grid-template-columns:\s*1fr/);

  assert.doesNotMatch(socialBackend, /friendSpectatorReadProbe|spectatorReadProbePath/);
}

function assertR47AddendumContracts(css = gameplayStyles) {
  const wide = cssBlockAfter(css, "@container recommendation-area (max-width: 1080px)");
  assert.match(wide, /\.live-teams\.is-arena\s*\{[^}]*grid-template-columns:\s*1fr/s);
  assert.match(wide, /\.live-teams\.is-arena\.is-grouped\s*\{[^}]*grid-template-columns:\s*1fr/s);
  assert.match(wide, /\.recommendation-champion-summary\s*\{[^}]*grid-template-areas:\s*"summary" "positions" "matchups"/s);
  assert.match(wide, /\.champion-matchups\s*\{[^}]*border-top:\s*1px solid var\(--line\)[^}]*border-left:\s*0/s);

  const classicNarrow = cssBlockAfter(css, "@container recommendation-area (max-width: 840px)");
  assert.match(classicNarrow, /\.live-teams\.is-insight:not\(\.is-arena\)\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
  assert.match(classicNarrow, /\.live-teams\.is-insight:not\(\.is-arena\)\s*>\s*\.live-team\s*\{[^}]*display:\s*block/s);

  const narrow = cssBlockAfter(css, "@container recommendation-area (max-width: 700px)");
  assert.match(narrow, /\.live-player dl\s*\{[^}]*repeat\(3,minmax\(60px,1fr\)\)/s);
  assert.match(narrow, /\.recommendation-tabs\s*\{[^}]*width:\s*100%[^}]*overflow-x:\s*auto/s);
  assert.match(narrow, /\.champion-summary-main dl\s*\{[^}]*justify-content:\s*flex-start[^}]*gap:\s*12px/s);
}

function assertR49RequiredMutationContracts({
	js = gameplayScript,
	goSource = gameplayBackend,
	specialistGo = specialistSource,
} = {}) {
	const stepContext = goFunctionSource(specialistGo, "specialistStepContext");
	assert.match(stepContext, /context\.WithTimeout\(ctx, timeout\)/, "A-3 specialist steps need their own deadline");
	const specialistOutcome = goFunctionSource(specialistGo, "outcome");
	assert.match(specialistOutcome, /errors\.Is\(ctxErr, context\.DeadlineExceeded\)[\s\S]*?\|\| timedOut\s*\{\s*return specialistOutcomeTimeout/, "A-4 timeouts must remain retryable upstream failures");

	const itemSet = functionSource(js, "buildItemSetPayload");
	assert.match(itemSet, /const coreIDs = new Set\(core\.map\(\(\{ id \}\) => id\)\);/);
	assert.match(itemSet, /const depth = mergedItems\(depthOptions\)\.filter\(\(\{ id \}\) => !coreIDs\.has\(id\)\);/, "B-2 later groups must exclude core items");
	const priceSort = goFunctionSource(goSource, "sortGameplayItemSetBlocksByPrice");
	assert.match(priceSort, /sort\.SliceStable\(knownItems,[\s\S]*?return knownItems\[left\]\.PriceTotal < knownItems\[right\]\.PriceTotal/, "B-3 known item prices must be sorted ascending");

	const positionChip = functionSource(js, "liveCurrentPositionChip");
	assert.match(positionChip, /const resolved = livePositionDisplay\(target\?\.clientPosition \|\| self\?\.position\);/, "C-1 position chip must use the client position");
	assert.doesNotMatch(positionChip, /payload\.resolvedPosition/, "C-1 resolved recommendations must not relabel the client position");
	const recommendationHeader = functionSource(js, "renderChampionRecommendationHeader");
	assert.match(recommendationHeader, /displayRoleRate: explicitRate \|\| \(totalPositionPlay > 0 \? play \/ totalPositionPlay \* 100 : 0\)/);
	assert.match(recommendationHeader, /\.sort\(\(left, right\) => right\.displayRoleRate - left\.displayRoleRate \|\| \(Number\(right\.play\) \|\| 0\) - \(Number\(left\.play\) \|\| 0\)\)/, "C-2 position choices must sort by role share");

	const displayedChampion = functionSource(js, "liveDisplayedChampionId");
	assert.match(displayedChampion, /player\?\.isCurrent === true \? Number\(currentChampionId\) \|\| 0 : 0/, "E-1 only the current player may use currentChampionId");
}

function assertMayhemCSSContract(css) {
  assert.match(css, /\.champion-list-card\s*\{[^}]*container:\s*champion-list \/ inline-size/s);
  assert.match(css, /\.rune-board-panel\s*\{[^}]*container:\s*rune-board \/ inline-size/s);
  assert.match(css, /\.champion-build-board\s*\{[^}]*container:\s*champion-build \/ inline-size/s);
  assert.match(css, /\.mayhem-workspace\s*\{[^}]*grid-template-columns:\s*minmax\(320px,340px\) minmax\(0,1fr\)/s);
  assert.match(css, /@container mayhem-page \(max-width: 1020px\)[\s\S]{0,240}\.mayhem-workspace > \.mayhem-champions\s*\{\s*display:\s*none/s);
  assert.match(css, /\.mayhem-tier-entry\s*\{\s*display:\s*none\s*;?\s*\}/s);
  const narrow = cssBlockAfter(css, "@container mayhem-page (max-width: 1020px)");
  assert.match(narrow, /\.mayhem-tier-entry\s*\{\s*display:\s*flex/s);
}

function assertR6RecommendationContract(js, css, goSource = backend) {
  assert.match(js, /return count < 3;/);
  assert.match(js, /sorted\.slice\(0, 9\)/);
  assert.match(js, /class="augment-grade is-\$\{grade\}"/);
  assert.match(js, /objectRows\(row\.assets\)\.every\(\(asset\) => asset\.kind === "item"\)/);
  assert.match(js, /4: "SummonerFlash"/);
  assert.match(goSource, /if score <= rows\[index\]\.Score/);
  assert.match(css, /\.mayhem-recommend-grid\s*\{[^}]*display:\s*grid;[^}]*repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(css, /\.augment-grade\s*\{[^}]*width:\s*22px[^}]*height:\s*22px[^}]*border-radius:\s*4px/s);
  assert.match(css, /\.arena-augment-section \.arena-option-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(cssBlockAfter(css, "@container arena-pane (max-width: 820px)"), /\.arena-option-grid, \.arena-augment-section \.arena-option-grid\s*\{[^}]*repeat\(2,minmax\(0,1fr\)\)/s);
  assert.match(cssBlockAfter(css, "@container arena-pane (max-width: 540px)"), /\.arena-option-grid, \.arena-augment-section \.arena-option-grid\s*\{[^}]*grid-template-columns:\s*1fr/s);
}

function assertR13ItemColumnContract(championJS, championCSS, liveJS, liveCSS) {
  const itemStyles = `${championCSS}\n${liveCSS}\n${sharedBuildStyles}`;
  const liveItemStyles = `${liveCSS}\n${sharedBuildStyles}`;
  const ranked = functionSource(championJS, "renderRankedBuild");
  const coreIndex = ranked.indexOf("build-core-column");
  const depthIndex = ranked.indexOf("${depthGroups}");
  assert.ok(coreIndex >= 0 && depthIndex > coreIndex, "champion core column must precede depth columns");
  assert.match(ranked, /data-depth-count="\$\{depthCount\}"/);
  assert.match(itemStyles, /\.build-item-row\s*\{[^}]*minmax\(0,1\.6fr\) repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(itemStyles, /\.build-item-row\[data-depth-count="2"\]\s*\{[^}]*repeat\(2,minmax\(0,1fr\)\)/s);
  assert.match(itemStyles, /\.build-item-row\s*>\s*\.item-depth-columns\s*\{[^}]*display:\s*contents/s);

  const live = functionSource(liveJS, "renderBuildRecommendation");
	  assert.ok(live.indexOf("build-summary-starter") < live.indexOf("build-summary-spells") && live.indexOf("build-summary-spells") < live.indexOf("build-summary-boots"));
  assert.match(live, /data-depth-count="\$\{depthGroups\.length\}"/);
  assert.match(liveItemStyles, /\.build-item-row\s*\{[^}]*minmax\(0,1\.6fr\) repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(liveItemStyles, /\.build-item-row\[data-depth-count="2"\]\s*\{[^}]*repeat\(2,minmax\(0,1fr\)\)/s);
  assert.match(liveItemStyles, /\.build-item-row\s*>\s*\.item-depth-columns\s*\{[^}]*display:\s*contents/s);
}

function assertR15PerformanceAndBuildContracts() {
  assert.match(gameplayBackend, /shouldLoadOverviewHistory\(reference, playerRef, matches\)/);
  assert.match(gameplayBackend, /matchHistoryOn\(ctx, client, reference\.ServerID, playerRef, 0, maximumSummaryMatchCount, true\)/);
  assert.match(gameplayBackend, /capGameplayCoreOptions\(bundle\.Build\.CoreOptions\)/);
  assert.match(gameplayBackend, /func capGameplayCoreOptions[\s\S]*?if len\(options\) > championCoreRecommendationLimit \{[\s\S]*?return options\[:championCoreRecommendationLimit\]/);
  assert.match(gameplayBackend, /"event": "overview_load_cost"/);
  assert.match(gameplayBackend, /if shouldLoadOverviewHistory\(reference, playerRef, matches\)/);
  assert.match(rankInsightsBackend, /case <-flight\.done:\s*return flight\.entry/);
  assert.match(gameplayScript, /const MATCH_TIERS_MAX_REFS = 24/);
  assert.match(rankInsightsBackend, /matchTiersMaxRefs\s*=\s*24/);
  assert.match(gameplayScript, /shouldHydrateMatchTiers\(tab, activeKey\)/);
  assert.match(gameplayScript, /offset \+= MATCH_TIERS_MAX_REFS/);
  assert.match(gameplayScript, /function shouldHydrateMatchTiers\(tab, activeTabKey\)/);
  assert.match(gameplayScript, /return Boolean\(tab\?\.overlay \|\| tab\?\.key === activeTabKey\)/);
  assert.match(gameplayStyles, /\.build-summary-bar\s*\{[^}]*grid-template-columns:\s*repeat\(3,/s);
  assert.match(mainSource, /BuildFingerprint: buildFingerprint/);
  assert.match(appScript, /data\.buildFingerprint/);
  assert.match(appScript, /settingsBuildIdentity\.textContent\s*=/);
  assert.match(html, /id="settings-build-identity"/);
  assert.match(html, /href="\/build-item-row\.css"/);
	  assert.equal((styles.match(/^\s*\.build-item-row\s*\{/gm) || []).length, 0);
	  assert.equal((gameplayStyles.match(/^\s*\.build-item-row\s*\{/gm) || []).length, 0);
  assert.match(sharedBuildStyles, /\.build-item-row\s*\{/);
  assert.match(sharedBuildStyles, /\.build-item-row\[data-depth-count="2"\]/);
  assert.match(sharedBuildStyles, /border-left:\s*1px solid var\(--line\);\s*border-top:\s*0;\s*\}/);
  const { shouldHydrateMatchTiers } = compileFunctions(gameplayScript, ["shouldHydrateMatchTiers"]);
  assert.equal(shouldHydrateMatchTiers({ key: "current", overlay: false }, "current"), true);
  assert.equal(shouldHydrateMatchTiers({ key: "other", overlay: false }, "current"), false);
  assert.equal(shouldHydrateMatchTiers({ key: "other", overlay: true }, "current"), true);
}

test("R15 performance, fingerprint, and shared build CSS contracts", () => {
  assertR15PerformanceAndBuildContracts();
});

test("R15 mutation probes fail for every new guard", () => {
  assert.throws(() => assert.match(gameplayBackend.replace("maximumSummaryMatchCount, true)", "maximumSummaryMatchCount, false)"), /maximumSummaryMatchCount, true\)/));
  assert.throws(() => assert.match(gameplayBackend.replace("&& len(matches) == 0", ""), /&& len\(matches\) == 0/));
  assert.throws(() => assert.match(rankInsightsBackend.replace("return flight.entry", "return rankScoreEntry{}"), /return flight\.entry/));
  assert.throws(() => assert.match(gameplayBackend.replace("return options[:5]", "return options[:1]"), /return options\[:5\]/));
  const { shouldHydrateMatchTiers } = compileFunctions(gameplayScript, ["shouldHydrateMatchTiers"]);
  assert.equal(shouldHydrateMatchTiers({ key: "other", overlay: false }, "current"), false);
  const mutatedHydration = compileFunctions(gameplayScript.replace("return Boolean(tab?.overlay || tab?.key === activeTabKey);", "return true;"), ["shouldHydrateMatchTiers"]);
  assert.equal(mutatedHydration.shouldHydrateMatchTiers({ key: "other", overlay: false }, "current"), true);
  assert.throws(() => assert.match(gameplayScript.replace("const MATCH_TIERS_MAX_REFS = 24", "const MATCH_TIERS_MAX_REFS = 1"), /const MATCH_TIERS_MAX_REFS = 24/));
  assert.throws(() => assert.match(gameplayBackend.replace('"event": "overview_load_cost"', '"event": "overview_cost"'), /"event": "overview_load_cost"/));
  assert.throws(() => assert.match(gameplayStyles.replace("repeat(3,minmax(220px,1fr))", "repeat(2,minmax(220px,1fr))"), /repeat\(3,minmax\(220px,1fr\)/));
  assert.throws(() => assert.match(mainSource.replace("BuildFingerprint: buildFingerprint", "BuildFingerprint: \"\""), /BuildFingerprint: buildFingerprint/));
  assert.throws(() => assert.match(appScript.replace("settingsBuildIdentity.textContent", "settingsBuildIdentity.dataset.textContent"), /settingsBuildIdentity\.textContent/));
  assert.throws(() => assert.match(sharedBuildStyles.replace("border-top: 0;", ""), /border-left:\s*1px solid var\(--line\);\s*border-top:\s*0;\s*\}/));
});

function assertR7Contract(js, css, structuredSource, hexdataSource, gameplaySource, gameplayCSSSource) {
	assert.match(js, /kind === "augment" \|\| kind === "prism"/);
	assert.doesNotMatch(js, /快节奏团战/);
	assert.match(css, /\.arena-overview-strip\s*\{[^}]*min-height:\s*132px/s);
	assert.match(css, /\.arena-overview-metrics > div\s*\{[^}]*min-height:\s*70px/s);
	// 指标列按内容定宽，否则会 flex-grow 占满右半边把英雄原画整个盖住。
	assert.match(css, /\.arena-overview-metrics\s*\{[^}]*flex:\s*0 1 auto/s);
	assert.doesNotMatch(css, /\.arena-overview-strip\s*\{\s*min-height:\s*118px/s);
	assert.match(css, /\.arena-overview-secondary\s*\{[^}]*min-height:\s*22px/s);
	assert.match(structuredSource, /applyLocalAugmentGrades\(response\.Build\.PrismItems\)/);
	assert.match(hexdataSource, /hexdataChampionNickname\.ReplaceAllString\(name, ""\)/);
	assert.match(gameplaySource, /function renderMatchDetailFailure\(gameID, detailState\)/);
	assert.match(gameplaySource, /data-retry-match-detail/);
	const matchSource = functionSource(gameplaySource, "renderMatch");
	assert.match(matchSource, /match-build[^\n]+match-items[^\n]+scorePlacementChip\(subjectScore\)[^\n]+match-badges/);
	assert.doesNotMatch(matchSource, /renderMatchScoreCell\(subjectScore\)|scoreChip\(subjectScore\)/);
	assert.doesNotMatch(matchSource, /match-score-line|const rankChip/);
	assert.match(functionSource(gameplaySource, "updateStatus"), /if \(wasConnected\) \{[\s\S]*?ensurePerks\(true\);[\s\S]*?ensureItems\(\);[\s\S]*?ensureSummonerSpells\(\);/);
	assert.match(gameplayCSSSource, /\.match-detail-failure\s*\{[^}]*border-top:/s);
}

test("champion intelligence is a first-level accessible workspace", () => {
  assert.match(html, /id="section-champions"[^>]+role="tab"[^>]+aria-controls="champions-panel"/);
  assert.match(html, /id="champions-panel"[^>]+role="tabpanel"[^>]+aria-labelledby="section-champions"/);
  assert.match(html, /option value="champions">英雄<\/option>/);
  assert.match(html, /id="setting-champion-position"/);
});

test("diagnostics panel exports the active structured log with concise guidance", () => {
  assert.match(html, /id="export-diagnostics"[^>]+href="\/api\/diagnostics\/log"[^>]+download(?:="")?[^>]*role="button"/);
  assert.doesNotMatch(html, /id="export-diagnostics"[^>]+download="lol-loot-diagnostics\.jsonl"/);
  assert.match(html, /id="diagnostic-log-meta"/);
  assert.match(html, /复现问题后导出日志。/);
  assert.match(appScript, /diagnosticLogBytes/);
  assert.match(appScript, /diagnosticLogEvents/);
  assert.match(appScript, /function formatFileSize\(value\)/);
});

test("R39 diagnostic event whitelist mutation is caught", () => {
  assert.match(featuresBackend, /clientDiagnosticEvents = map\[string\]map\[string\]bool/);
  assert.match(featuresBackend, /"live_recommendations_skip":/);
  const oldWhitelist = featuresBackend.replace(
    "reasons, knownEvent := clientDiagnosticEvents[request.Event]",
    "knownEvent := request.Event == \"specialist_runes_client_skip\"\n\tvar reasons = specialistRuneClientReasons",
  );
  assert.throws(() => assert.match(oldWhitelist, /reasons, knownEvent := clientDiagnosticEvents\[request.Event\]/));
});

test("exported diagnostic log filename is timestamped so repeat exports never collide", () => {
  // 真实事故：导出文件名固定不变，用户删了旧日志重新导出，上传给第三方时
  // 被按文件名去重，内容仍是旧的且长期未被发现。文件名必须随导出时刻变化。
  assert.match(appScript, /function diagnosticExportFilename\(\)\s*\{[\s\S]*?padStart\(2,\s*"0"\)[\s\S]*?\}/);
  assert.match(appScript, /`lol-loot-diagnostics-\$\{stamp\}\.jsonl`/);
  assert.match(appScript, /el\.exportDiagnostics\.setAttribute\("download",\s*diagnosticExportFilename\(\)\)/);
  const filenameFn = new Function(`${appScript.match(/function diagnosticExportFilename\(\)\s*\{[\s\S]*?\n  \}/)[0]}\nreturn diagnosticExportFilename;`)();
  const first = filenameFn();
  assert.match(first, /^lol-loot-diagnostics-\d{4}-\d{4}\.jsonl$/);
});

test("renderer only calls the authenticated local champion API", () => {
  for (const endpoint of ["/api/champions/catalog", "/api/champions/rankings", "/api/champions/augments", "/api/champions/detail", "/api/champion-asset"]) {
    assert.ok(script.includes(endpoint), `missing local endpoint ${endpoint}`);
  }
  assert.doesNotMatch(script, /fetch\(["'`]https?:\/\//);
  assert.match(script, /loading="lazy"/);
});

function assertPositionRecommendationContract(championJS, championCSS, liveJS) {
  const detailSource = functionSource(championJS, "renderDetail");
  assert.match(championJS, /detailPosition:\s*null/);
  assert.match(championJS, /detailCache:\s*window\.deepLegendsRuntime\?\.createCache\(\{ max: 64, ttl: 300000 \}\)/);
  assert.match(championJS, /const positions = detailPositions\.length \? detailPositions : rowPositions;/);
  assert.match(championJS, /const hasPositions = state\.mode === "ranked" && positions\.length > 1/);
  assert.match(championJS, /positionIcon\(item\.position\)/);
  assert.match(championJS, /const positionStats = detailPositions\.find/);
  assert.match(championJS, /positionStats\?\.winRate \?\? row\.winRate/);
  assert.match(championJS, /state\.error = "请先选择分路"/);
  assert.match(championJS, /firstPositionOf\(ranked\) \|\| firstPositionOf\(\{ position: state\.selected\?\.position \}\) \|\| "mid"/);
	assert.doesNotMatch(functionSource(championJS, "resetTransientChampionState"), /state\.detailCache\.clear\(\)/);
  assert.match(championJS, /class="metric-win">\$\{percent\(item\.winRate\)\}/);
  assert.match(championJS, /class="metric-pick">占\$\{percent\(item\.roleRate\)\}/);
  assert.match(championJS, /class="champion-detail-positions" data-count="\$\{positions\.length\}"/);
	assert.match(championJS, /class="champion-tier-select select-wrap champion-detail-tier-select"/);
	assert.match(championJS, /function switchDetailTier\(tier\)/);
	assert.match(championJS, /refreshRankingsForDetailTier\(nextTier\)/);
	assert.match(championJS, /state\.detail = null;[\s\S]{0,160}switchDetailPosition\(position\)/);
	assert.match(championJS, /renderCounters\(detail\.counters, detail\.topPlayers, detail\.countersTier \|\| detail\.sampleTier\)/);
	assert.match(championJS, /class="counter-row"/);
	assert.match(championJS, /renderCounterGroup\("优势对抗"[\s\S]{0,160}renderCounterGroup\("劣势对抗"/);
	assert.match(championJS, /class="counter-champion"[\s\S]{0,420}场[\s\S]{0,180}percent\(row\.winRate\)/);
  assert.match(championJS, /spell-options\$\{rows\.length === 1 \? " is-single" : ""\}/);
  assert.match(functionSource(championJS, "closeDetail"), /state\.detailPosition = null;/);
  assert.doesNotMatch(championJS, /writeSetting\(["']detail-position/);
  assert.doesNotMatch(championCSS, /\.champion-detail-hero\.has-positions\s*\{[^}]*min-height:/s);
  assert.match(championCSS, /\.champion-detail-side\s*\{[^}]*grid-column:\s*3/s);
  assert.match(championCSS, /\.champion-detail-side\s*\{[^}]*align-content:\s*center/s);
  assert.match(championCSS, /\.champion-detail-positions\s*\{[^}]*display:\s*grid/s);
  assert.ok(detailSource.indexOf('${positionsMarkup}</div>') > detailSource.indexOf('class="champion-detail-side"'), "分路按钮必须位于右侧指标容器内");
  assert.doesNotMatch(cssBlockAfter(championCSS, ".champion-detail-positions {"), /grid-column|grid-row/);
  assert.match(championCSS, /\.champion-detail-positions\[data-count="3"\]\s*\{[^}]*grid-template-columns:\s*repeat\(3,minmax\(105px,1fr\)\)/s);
  assert.match(championCSS, /\.champion-detail-positions\[data-count="4"\][^}]+repeat\(2,minmax\(105px,1fr\)\)/s);
	const heroBelow1080 = cssBlockAfter(championCSS, "@media (max-width: 1079px)");
	assert.match(heroBelow1080, /\.champion-detail-positions\[data-count="3"\]\s*\{[^}]*grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/s);
	const heroAt1080 = cssBlockAfter(championCSS, "@media (max-width: 1080px)");
	assert.doesNotMatch(heroAt1080, /\.champion-detail-positions\[data-count="3"\]/);
	assert.equal(cssNumber(championCSS, ".champion-detail-hero", "min-height"), 144);
	assert.equal(cssNumber(championCSS, ".champion-detail-hero", "padding"), 14);
	assert.equal(cssNumber(championCSS, ".champion-detail-metrics > div", "min-height"), 50);
	assert.equal(cssNumber(championCSS, ".champion-detail-positions button", "min-height"), 44);
	const heroAt900 = cssBlockAfter(championCSS, "@media (max-width: 900px)");
	assert.match(heroAt900, /\.champion-detail-hero\s*\{[^}]*grid-template-columns:\s*104px minmax\(0,1fr\)/s);
	const heroAt480 = cssBlockAfter(championCSS, "@media (max-width: 480px)");
	assert.match(heroAt480, /\.champion-detail-positions button\s*\{[^}]*gap:\s*5px;[^}]*padding-inline:\s*7px/s);
	assert.match(heroAt480, /\.champion-detail-positions button small\s*\{[^}]*flex-wrap:\s*wrap;[^}]*font-size:\s*9px;[^}]*white-space:\s*normal/s);
	assert.match(championCSS, /\.counter-row\s*\{[^}]*repeat\(3,minmax\(0,1fr\)\)/s);
	assert.match(championCSS, /\.counter-champion strong\s*\{[^}]*white-space:\s*nowrap/s);
  assert.match(championCSS, /\.champion-detail-positions button\.is-active \.position-icon img\s*\{[^}]*opacity:\s*1;[^}]*filter:\s*none/s);
  assert.match(championCSS, /\.champion-detail-positions \.metric-win[^}]*\{ color: var\(--success\)/s);
  assert.match(championCSS, /\.champion-detail-positions \.metric-pick[^}]*\{ color: var\(--accent\)/s);
  assert.match(championCSS, /\.spell-options\.is-single \.spell-option\s*\{[^}]*width:\s*100%/s);
  assert.match(liveJS, /const baseKey = `\$\{championId\}:\$\{gameMode\}:\$\{mapId\}`;/);
  assert.match(liveJS, /const spellKey = \[Number\(self\?\.spell1Id\)[^;]+\.sort\(\(left, right\) => left - right\)\.join\("-"\) \|\| "none";/);
  assert.match(liveJS, /key: `\$\{championId\}:\$\{position\}:\$\{gameMode\}:\$\{mapId\}:\$\{tier\}:\$\{spellKey\}`/);
  assert.match(liveJS, /orderedPositions\.length >= 1 \?/);
  assert.match(functionSource(liveJS, "resetLivePositionOverrides"), /gameChanged \|\| leftChampionSelect/);
  assert.match(functionSource(liveJS, "resetRecommendationTabsOnChampionChange"), /state\.recommendationTab = "runes"/);
  assert.match(functionSource(liveJS, "resetRecommendationTabsOnChampionChange"), /state\.runeSourceTab = "opgg"/);
  assert.match(functionSource(liveJS, "selectLivePosition"), /state\.liveRecommendationFailures\.delete\(selectedTarget\.key\)/);
  assert.doesNotMatch(functionSource(liveJS, "selectLivePosition"), /state\.liveRecommendations\.delete/);
  assert.match(liveJS, /const positionCopy = positionLabel\(livePositionDisplay\(resolvedPosition\)\)/);
  assert.match(gameplayStyles, /\.live-position-chip img\.position-icon\s*\{[^}]*object-fit:\s*contain/s);
  assert.doesNotMatch(gameplayStyles, /\.live-position-chip \.position-icon img/);
	  assert.match(liveJS, /payload\.positionSource && payload\.positionSource !== "requested"/);
	  assert.doesNotMatch(liveJS, /当前分路 · 最近 10 局/);
}

test("position-aware recommendation contracts kill documented mutations", () => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-position-contract-"));
  const copies = {
    champion: path.join(temp, "champions.js"),
    styles: path.join(temp, "champions.css"),
    gameplay: path.join(temp, "gameplay.js"),
  };
  fs.copyFileSync(path.join(__dirname, "champions.js"), copies.champion);
  fs.copyFileSync(path.join(__dirname, "champions.css"), copies.styles);
  fs.copyFileSync(path.join(__dirname, "gameplay.js"), copies.gameplay);
  for (const copy of Object.values(copies)) assert.equal(fs.lstatSync(copy).isSymbolicLink(), false);
  try {
    const original = {
      champion: fs.readFileSync(copies.champion, "utf8"),
      styles: fs.readFileSync(copies.styles, "utf8"),
      gameplay: fs.readFileSync(copies.gameplay, "utf8"),
    };
    assertPositionRecommendationContract(original.champion, original.styles, original.gameplay);

    fs.writeFileSync(copies.champion, original.champion.replace("positions.length > 1", "positions.length > 0"));
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.champion, original.champion.replace("positionIcon(item.position)", "positionIcon(\"all\")"));
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.champion, original.champion.replace("positionStats?.winRate ?? row.winRate", "row.winRate"));
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.champion, original.champion.replace(/(function closeDetail\(\) \{[\s\S]*?)state\.detailPosition = null;/, "$1"));
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.champion, `${original.champion}\nwriteSetting("detail-position", state.detailPosition);`);
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.champion, original.champion.replace('firstPositionOf(ranked) || firstPositionOf({ position: state.selected?.position }) || "mid"', "firstPositionOf(ranked)"));
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.champion, original.champion.replace(' data-count="${positions.length}"', ""));
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.champion, original.champion.replace('${positionsMarkup}</div>', '</div>${positionsMarkup}'));
    assert.throws(() => assertPositionRecommendationContract(fs.readFileSync(copies.champion, "utf8"), original.styles, original.gameplay));
    fs.writeFileSync(copies.styles, original.styles.replace('repeat(2,minmax(105px,1fr)); }', 'repeat(4,minmax(105px,1fr)); }'));
    assert.throws(() => assertPositionRecommendationContract(original.champion, fs.readFileSync(copies.styles, "utf8"), original.gameplay));
	for (const [current, mutation] of [["min-height: 144px", "min-height: 176px"], ["min-height: 50px", "min-height: 62px"], ["min-height: 44px", "min-height: 48px"]]) {
      fs.writeFileSync(copies.styles, original.styles.replace(current, mutation));
      assert.throws(() => assertPositionRecommendationContract(original.champion, fs.readFileSync(copies.styles, "utf8"), original.gameplay));
    }
    fs.writeFileSync(copies.styles, original.styles.replace(/(\.champion-detail-positions button\.is-active \.position-icon img\s*\{[^}]*)filter:\s*none/, "$1filter: grayscale(.7)"));
    assert.throws(() => assertPositionRecommendationContract(original.champion, fs.readFileSync(copies.styles, "utf8"), original.gameplay));

    fs.writeFileSync(copies.gameplay, original.gameplay.replace("const baseKey = `${championId}:${gameMode}:${mapId}`;", "const baseKey = `${championId}:${position}:${gameMode}:${mapId}`;"));
    assert.throws(() => assertPositionRecommendationContract(original.champion, original.styles, fs.readFileSync(copies.gameplay, "utf8")));
	    fs.writeFileSync(copies.gameplay, original.gameplay.replace('const automatic = !target?.positionOverride && payload.positionSource && payload.positionSource !== "requested";', "const automatic = true;"));
    assert.throws(() => assertPositionRecommendationContract(original.champion, original.styles, fs.readFileSync(copies.gameplay, "utf8")));
	    fs.writeFileSync(copies.gameplay, original.gameplay.replace("orderedPositions.length >= 1", "orderedPositions.length > 1"));
    assert.throws(() => assertPositionRecommendationContract(original.champion, original.styles, fs.readFileSync(copies.gameplay, "utf8")));
    fs.writeFileSync(copies.gameplay, original.gameplay.replace(":${mapId}:${tier}:${spellKey}`", ":${mapId}`"));
    assert.throws(() => assertPositionRecommendationContract(original.champion, original.styles, fs.readFileSync(copies.gameplay, "utf8")));
  } finally {
    fs.rmSync(temp, { recursive: true, force: true });
	}
});

test("summoner spell card renders the first two real recommendations without placeholders", () => {
	const { renderSpellsCard } = compileFunctions(script, ["renderSpellsCard"], {
		assetImage: (asset) => `<img data-asset-id="${asset.id}">`,
		compactNumber: (value) => String(value),
		percent: (value) => `${value}%`,
	});
	const single = renderSpellsCard({
		summonerSpells: [{ assets: [{ id: 4 }, { id: 11 }], games: 321, pickRate: 78, winRate: 53 }],
	});
	assert.match(single, /class="spell-options is-single"/);
	assert.equal((single.match(/class="spell-option(?:\s|\")/g) || []).length, 1);
	assert.match(single, /data-asset-id="4"/);
	assert.match(single, /data-asset-id="11"/);
	assert.doesNotMatch(single, /暂无样本|占位|placeholder/);

	const double = renderSpellsCard({
		summonerSpells: [
			{ assets: [{ id: 4 }, { id: 12 }], games: 200, pickRate: 60, winRate: 52 },
			{ assets: [{ id: 4 }, { id: 14 }], games: 100, pickRate: 40, winRate: 51 },
		],
	});
	assert.doesNotMatch(double, /class="spell-options is-single"/);
	assert.equal((double.match(/class="spell-option(?:\s|\")/g) || []).length, 2);
	assert.match(double, /data-asset-id="12"/);
	assert.match(double, /data-asset-id="14"/);
});

test("ranked loadout side uses a balanced two-two-two height budget", () => {
	const { allocateLoadoutSideRows } = compileFunctions(script, ["allocateLoadoutSideRows"]);
	const row = (id) => ({ id, assets: [{ id }] });
	const abundant = {
		summonerSpells: [row("s1"), row("s2"), row("s3")],
		starterItems: [row("i1"), row("i2"), row("i3"), row("i4")],
		boots: [row("b1"), row("b2"), row("b3"), row("b4")],
	};
	const standard = allocateLoadoutSideRows(abundant);
	assert.deepEqual([standard.summonerSpells.length, standard.starterItems.length, standard.boots.length], [2, 2, 2]);
	assert.ok(standard.summonerSpells.length + standard.starterItems.length + standard.boots.length <= 6);
	const cases = [
		[{ summonerSpells: abundant.summonerSpells.slice(0, 1), starterItems: abundant.starterItems, boots: abundant.boots }, [1, 3, 2]],
		[{ summonerSpells: abundant.summonerSpells, starterItems: abundant.starterItems.slice(0, 1), boots: abundant.boots }, [3, 1, 2]],
		[{ summonerSpells: abundant.summonerSpells, starterItems: abundant.starterItems, boots: abundant.boots.slice(0, 1) }, [3, 2, 1]],
	];
	for (const [input, expected] of cases) {
		const result = allocateLoadoutSideRows(input);
		assert.deepEqual([result.summonerSpells.length, result.starterItems.length, result.boots.length], expected);
		assert.ok(result.summonerSpells.length + result.starterItems.length + result.boots.length <= 6);
	}
	const sparse = allocateLoadoutSideRows({ summonerSpells: abundant.summonerSpells.slice(0, 1), starterItems: abundant.starterItems.slice(0, 1), boots: abundant.boots.slice(0, 1) });
	assert.deepEqual([sparse.summonerSpells.length, sparse.starterItems.length, sparse.boots.length], [1, 1, 1]);
	const loadoutSide = cssBlockAfter(styles, ".loadout-side");
	assert.match(loadoutSide, /grid-template-rows:\s*repeat\(3,max-content\)/);
	assert.match(loadoutSide, /align-content:\s*start/);
	assert.doesNotMatch(loadoutSide, /grid-template-rows:\s*repeat\(3,minmax\(0,1fr\)\)/);
});

test("summoner spell recommendations do not add a low-sample badge", () => {
	const dependencies = {
		assetImage: (asset) => `<img data-asset-id="${asset.id}">`,
		compactNumber: (value) => String(value),
		percent: (value) => `${value}%`,
	};
	const render = compileFunctions(script, ["renderSpellsCard"], dependencies).renderSpellsCard;
	const build = { summonerSpells: [{ assets: [{ id: 4 }, { id: 11 }], games: 18, pickRate: 0.3, winRate: 22.22 }] };
	const markup = render(build);
	assert.match(markup, /<dt>场次<\/dt><dd>18<\/dd>/);
	assert.doesNotMatch(markup, /样本少|spell-sample-badge|仅 18 场/);
});

test("ranked recommendation rows never dim low sample sizes", () => {
  const { renderConfigOption } = compileFunctions(script, ["renderConfigOption"], {
    renderAssetButton: () => "<icon></icon>",
    summonerSpellAsset: (asset) => asset,
    renderOptionStats: () => "",
    renderDepthStats: () => "",
    percent: (value) => `${value}%`,
  });
  // 低样本一律不加视觉标记：整块压暗 + hover 才恢复被用户读成「图标全是灰的」。
  assert.doesNotMatch(renderConfigOption({ assets: [{ id: 1 }], games: 99 }, "item"), /is-low-sample/);
  assert.doesNotMatch(renderConfigOption({ assets: [{ id: 1 }], games: 100 }, "item"), /is-low-sample/);
  assert.doesNotMatch(styles, /is-low-sample/);
  assert.doesNotMatch(gameplayStyles, /is-low-sample/);
  assert.doesNotMatch(gameplayScript, /is-low-sample/);
});

test("backend pins Korean data and restricts remote image paths", () => {
  assert.match(backend, /\/api\/KR\/champions\/ranked/);
  assert.match(structuredBackend, /"ranked":\s+\{APIMode: "ranked", Region: "KR"/);
  assert.match(structuredBackend, /requestPath := "\/api\/" \+ spec\.Region \+ "\/champions\/"/);
  assert.match(backend, /strings\.HasPrefix\(requestPath, "\/meta\/images\/"\)/);
  assert.match(backend, /strings\.HasPrefix\(requestPath, "\/cdn\/"\)/);
  assert.match(backend, /Value: "challenger", Label: "最强王者"/);
});

test("champion layouts keep responsive and reduced-motion fallbacks", () => {
  for (const breakpoint of ["1260px", "1080px", "900px", "700px", "480px"]) assert.ok(styles.includes(`max-width: ${breakpoint}`));
  assert.match(styles, /prefers-reduced-motion:\s*reduce/);
  assert.match(styles, /\.augment-tier-groups\s*\{[^}]*display:\s*grid/s);
  assert.match(styles, /\.champion-rune-board\s*\{[^}]*grid-template-columns:/s);
  assert.doesNotMatch(styles, /\.augment-podium\s*\{/); // Retired podium; current augment tier groups above retain responsive coverage.
  assert.match(styles, /\.champion-table-scroll\s*\{[^}]*overflow-x:\s*auto/s);
  assert.match(styles, /\.champion-filter-bar\s*\{[^}]*grid-template-columns:/s);
  assertMayhemCSSContract(styles);
  assert.match(styles, /\.champion-rune-board\s*\{[^}]*clamp\([^}]*cqi/s);
  assert.doesNotMatch(styles, /min-width:\s*620px/);
  assert.doesNotMatch(styles, /\.rune-board-panel\s*\{[^}]*overflow-x:\s*auto/s);
  assert.doesNotMatch(styles, /\.champion-build-board \.config-icons\s*\{[^}]*overflow-x:\s*auto/s);
  assert.match(gameplayStyles, /\.skill-order\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fit/s);
});

test("mayhem redesign keeps the two-column workspace, atlas, and R6 recommendations", () => {
  assert.match(script, /data-mayhem-view="\$\{value\}"/);
  assert.match(script, /class="mayhem-workspace"/);
  assert.match(script, /renderChampionTable\(rows, "mayhem"\)/);
  assert.match(script, /function selectMayhemChampion\(row\)/);
	assert.match(script, /sameChampion && \(state\.mayhemDetailLoading \|\| state\.mayhemDetailKey === Number\(selected\.championId\) && state\.detail\)/);
  assert.match(script, /const token = state\.mayhemRequestToken \+ 1;[\s\S]{0,500}const current = \(\) => token === state\.mayhemRequestToken[\s\S]{0,500}if \(!current\(\)\) return;/);
  assert.match(script, /class="mayhem-detail-pane"/);
  assert.doesNotMatch(script, /data-champion-back[^\n]+海克斯大乱斗/);
  assert.match(script, /function renderMayhemAtlas\(items\)/);
  assert.match(script, /function renderMayhemAtlasDetail\(item\)/);
  assert.match(script, /\/api\/champions\/augment-detail\?id=/);
  assert.match(script, /\/api\/champions\/augment-rarity/);
  assert.match(script, /if \(state\.mayhemView === "atlas"\)[\s\S]{0,100}loadMayhemAtlas\(\)/);
  assert.match(script, /function usesHexdata\(source\)/);
  assert.match(script, /class="arena-option-grid mayhem-recommend-grid"/);
  assert.match(script, /return count < 3;/);
  assert.match(script, /class="augment-grade is-\$\{grade\}"/);
  // 指标改成与斗魂同款的带标签列，不再是一行串起来的文字。
  assert.match(script, /\["胜率", percent\(item\.winRate\), "is-win"\]/);
  assert.match(script, /\["样本", compactNumber\(item\.games\), ""\]/);
  assert.match(script, /\["综合评分", number\(item\.score, 1\), "is-score"\]/);
  assert.match(script, /按综合评分、胜率与样本展示各品质前三项/);
  assert.match(script, /if \(!usesHexdata\(state\.augments\?\.source\)\)/);
  assert.match(script, /state\.mayhemAugmentDetail = \{ source: state\.augments\?\.source \|\| "OP\.GG", champions: \[\] \};/);
  assert.match(script, /if \(!state\.mayhemAugmentID && first\)/);
  assert.match(script, /Patch \$\{escapeHTML\(citation\.patch\)\} · \$\{escapeHTML\(citation\.reportDate\)\} · build/);
  assert.match(script, /function renderMeasurementTechnique\(value\)/);
  assert.match(styles, /\.mayhem-atlas\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\) minmax\(0,1fr\)/s);
  assert.match(styles, /\.mayhem-recommend-grid\s*\{[^}]*repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(styles, /\.augment-grade\.is-S/);
  assert.match(styles, /\.hex-rank\.is-1\s*\{[^}]*clip-path:/s);
  assert.doesNotMatch(styles, /\.arena-option-rank\s*\{/);
	  assert.match(gameplayStyles, /\.live-augment-columns\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
	  assert.match(gameplayStyles, /\.live-augment-column > div\s*\{[^}]*repeat\(4,minmax\(0,1fr\)\)[^}]*gap:\s*8px/s);
	  assert.doesNotMatch(gameplayScript, /\["HexScore", row\.score/);
	  assert.match(gameplayScript, /\["win", "胜率", row\.winRate, percent\]/);
	  assert.match(gameplayScript, /\["pick", "选用率", row\.pickRate, percent\]/);
	  assert.match(gameplayScript, /\["games", "场次", row\.games, number\]/);
  assert.match(script, /class="mayhem-fit-metrics"/);
  assert.doesNotMatch(styles, /\.mayhem-fit-row > span/);
});

test("Mayhem recommendation and item ranking never request a guessed image URL", () => {
  const urls = [];
  const dependencies = {
    assetImage: (asset) => { urls.push(imageURLStub(asset?.source, asset?.path)); return "<img>"; },
    imageURL: imageURLStub,
    escapeHTML: (value) => String(value ?? ""),
    number: (value) => String(value ?? ""),
    percent: (value) => String(value ?? ""),
    compactNumber: (value) => String(value ?? ""),
    augmentRarityKey: (value) => String(value || "").toLowerCase(),
    arenaRarityKey: (value) => String(value || "").toLowerCase(),
    augmentGrade: (grade) => grade || "B",
    objectRows: (value) => Array.isArray(value) ? value : [],
    renderSourceCitation: () => "",
  };
  dependencies.renderAssetButton = (asset) => `<button>${dependencies.assetImage(asset)}</button>`;
  const { renderMayhemRecommendedAugment, renderMayhemItemRanking, renderArenaOptionCard } = compileFunctions(
    script,
    ["renderArenaOptionCard", "renderMayhemRecommendedAugment", "renderMayhemItemRanking"],
    dependencies,
  );
  assert.equal(typeof renderArenaOptionCard, "function");

  // 没有图标路径时只能落到本地占位图，绝不能拼一个注定 404 的远端地址。
  const recommended = renderMayhemRecommendedAugment({ item: { name: "无图推荐" }, meta: { name: "无图推荐" }, grade: "S" }, 0);
  const ranking = renderMayhemItemRanking([{ assets: [{ name: "无图装备" }] }]);
  assert.deepEqual(urls, ["/image-unavailable.svg"]);
  assert.match(recommended, /arena-option-card/);
  assert.match(ranking, /mayhem-item-name-only/);
  assert.match(ranking, /<dt>胜率<\/dt>/);
  assert.match(ranking, /<dt>场次<\/dt>/);
  assert.doesNotMatch(ranking, /<dt>综合评分<\/dt>|<dt>样本<\/dt>/);
  assert.doesNotMatch(ranking, /按综合评分排序的高表现装备/);

  urls.length = 0;
  renderMayhemRecommendedAugment({ item: { name: "有图推荐" }, meta: { name: "有图推荐", imageSource: "opgg", imagePath: "/augment.png" }, grade: "S" }, 0);
  renderMayhemItemRanking([{ assets: [{ name: "有图装备", source: "opgg", path: "/item.png" }] }]);
  assert.deepEqual(urls, [
    "/api/champion-asset?source=opgg&path=%2Faugment.png",
    "/api/champion-asset?source=opgg&path=%2Fitem.png",
  ]);
});

// 海斗与斗魂共用同一张卡片，指标列数不同但外壳必须一致。
test("Mayhem and Arena augment cards render the same card shell", () => {
  const dependencies = {
    renderAssetButton: (asset) => `<button>${asset.name}</button>`,
    arenaRarityKey: (value) => String(value || "unknown").toLowerCase(),
    augmentGrade: (grade) => grade || "B",
    escapeHTML: (value) => String(value ?? ""),
    percent: (value) => `${value}%`,
    number: (value) => String(value),
    compactNumber: (value) => String(value),
    assetImage: () => "<img>",
    objectRows: (value) => Array.isArray(value) ? value : [],
  };
  const { renderArenaOptionCard, renderMayhemRecommendedAugment } = compileFunctions(
    script,
    ["renderArenaOptionCard", "renderMayhemRecommendedAugment"],
    dependencies,
  );
  const arena = renderArenaOptionCard(
    { grade: "S", score: 87.6, winRate: 63.6, averagePlacement: 2.9, firstPlaceRate: 21, games: 24000, rarity: "prismatic", assets: [{ name: "掷骰狂人" }] },
    "augment", 0, [87.6],
  );
  const mayhem = renderMayhemRecommendedAugment(
    { item: { score: 87.6, winRate: 63.6, games: 24000, rarity: "prismatic", assets: [{ id: 2095, name: "掷骰狂人", source: "communitydragon", path: "/a.png" }] }, meta: { name: "掷骰狂人", rarity: "prismatic" }, grade: "S" },
    0,
  );
  for (const markup of [arena, mayhem]) {
    assert.match(markup, /class="arena-option-card[^"]*is-prismatic/);
    assert.match(markup, /<div class="arena-option-main">/);
    assert.match(markup, /class="augment-grade is-S">S<\/b>/);
    assert.match(markup, /class="arena-option-rarity is-prismatic">棱彩<\/small>/);
    assert.match(markup, /<dt>胜率<\/dt>/);
  }
  // 海斗是大乱斗口径，没有名次与吃鸡率。
  assert.match(arena, /data-metric-count="5"/);
  assert.match(mayhem, /data-metric-count="3"/);
  assert.doesNotMatch(mayhem, /平均名次|吃鸡率/);
  assert.match(styles, /\.arena-option-card\[data-metric-count="3"\] dl\s*\{[^}]*repeat\(3,minmax\(0,1fr\)\)/s);
});

// 用户要的是"直接在这边展示"：说明必须随卡片一起给出，而不是指去图鉴。
test("Mayhem augment cards carry the augment copy in the tooltip", () => {
  const { renderMayhemRecommendedAugment } = compileFunctions(
    script,
    ["renderArenaOptionCard", "renderMayhemRecommendedAugment", "renderAssetButton", "assetTooltip", "appendAssetNumbers"],
    {
      assetImage: () => "<img>",
      imageURL: imageURLStub,
      arenaRarityKey: (value) => String(value || "unknown").toLowerCase(),
      augmentGrade: (grade) => grade || "B",
      escapeHTML: (value) => String(value ?? ""),
      percent: (value) => `${value}%`,
      number: (value) => String(value),
      compactNumber: (value) => String(value),
      objectRows: (value) => Array.isArray(value) ? value : [],
    },
  );
  const markup = renderMayhemRecommendedAugment({
    item: { score: 87.6, winRate: 63.6, games: 24000, rarity: "prismatic", assets: [{ id: 2095, name: "掷骰狂人", source: "communitydragon", path: "/a.png", description: "这是掷骰狂人的效果说明。" }] },
    meta: { name: "掷骰狂人", rarity: "prismatic" },
    grade: "S",
  }, 0);
  assert.match(markup, /data-tooltip="掷骰狂人\n这是掷骰狂人的效果说明。"/);
  assert.doesNotMatch(markup, /品质/);
  assert.doesNotMatch(markup, /图鉴/);
});

test("live recommendation panels do not render upstream provenance footers", () => {
  assert.doesNotMatch(gameplayScript, /function renderRecommendationCitation\(/);
  assert.doesNotMatch(gameplayScript, /recommendation-source-line|数据来源 Hexdata|数据来源 OP\.GG/);
  assert.doesNotMatch(gameplayStyles, /\.recommendation-source-line/);
});

test("mayhem narrow layout moves the one champion list into an accessible dialog", () => {
  assert.match(script, /<dialog class="mayhem-tier-dialog"/);
  assert.match(script, /content\.append\(list\)/);
  assert.match(script, /marker\.before\(list\)/);
  assert.match(script, /dialog\.showModal\(\)/);
  assert.match(script, /dialog\.addEventListener\("close"/);
	assert.match(script, /returnFocus\?\.isConnected \? returnFocus : returnFocus \? root\.querySelector\("\[data-open-mayhem-tiers\]"\)/);
  assert.match(script, /data-open-mayhem-tiers/);
  assertMayhemCSSContract(styles);
});

test("arena narrow layout moves the champion list into an accessible dialog", () => {
  assert.match(script, /<dialog class="mayhem-tier-dialog arena-tier-dialog"/);
  assert.match(script, /aria-controls="arena-tier-dialog" data-open-arena-tiers/);
  assert.equal((script.match(/class="button button-primary tier-trigger"/g) || []).length, 2);
  assert.match(script, /class="icon-button control-icon-button tier-dialog-close"[^>]+aria-label="关闭英雄梯度"[^>]*><svg class="control-icon"/);
  assert.match(script, /function mountArenaTierDialog\(\)/);
  assert.match(script, /function openArenaTierDialog\(trigger\)/);
  assert.match(script, /function closeArenaTierDialog\(restoreFocus = true\)/);
  assert.match(script, /event\.key === "Escape" && state\.arenaDialogOpen[\s\S]{0,180}closeArenaTierDialog\(\)/);
  assert.match(script, /event\.key === "Escape" && state\.mayhemDialogOpen[\s\S]{0,180}closeMayhemTierDialog\(\)/);
  assert.match(script, /returnFocus\?\.isConnected \? returnFocus : returnFocus \? root\.querySelector\("\[data-open-arena-tiers\]"\)/);
  assert.match(styles, /\.arena-tier-entry\s*\{\s*display:\s*none/);
  assert.match(styles, /\.tier-dialog-close\s*\{[^}]*width:\s*34px[^}]*border:\s*1px solid var\(--line\)/s);
  assert.match(styles, /\.tier-trigger\s*\{[^}]*min-height:\s*36px/s);
  assert.match(styles, /\.arena-tier-dialog-content \.arena-champions\s*\{[^}]*height:\s*100%[^}]*min-height:\s*0/s);
  const arenaNarrow = cssBlockAfter(styles.slice(styles.lastIndexOf("@media (max-width: 1180px)")), "@media (max-width: 1180px)");
  assert.match(arenaNarrow, /\.arena-redesign-workspace > \.arena-champions\s*\{\s*display:\s*none/);
  assert.match(arenaNarrow, /\.arena-tier-entry\s*\{[^}]*display:\s*flex/);
  assert.match(script, /function resetTransientChampionState\(\{ restorePosition = false \} = \{\}\) \{\s*closeMayhemTierDialog\(false\);\s*closeArenaTierDialog\(false\);/);
});

test("mayhem CSS contract catches breakpoint and hidden-column mutations on a true copy", () => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-mayhem-css-"));
  const copy = path.join(temp, "champions.css");
  fs.copyFileSync(path.join(__dirname, "champions.css"), copy);
  assert.equal(fs.lstatSync(copy).isSymbolicLink(), false);
  try {
    const original = fs.readFileSync(copy, "utf8");
    fs.writeFileSync(copy, original.replace("max-width: 1020px", "max-width: 1019px"));
    assert.throws(() => assertMayhemCSSContract(fs.readFileSync(copy, "utf8")));
    fs.writeFileSync(copy, original.replace(".mayhem-workspace > .mayhem-champions { display: none; }", ".mayhem-workspace > .mayhem-champions { display: flex; }"));
    assert.throws(() => assertMayhemCSSContract(fs.readFileSync(copy, "utf8")));
  } finally {
    fs.rmSync(temp, { recursive: true, force: true });
  }
});

test("R6 A B C contracts reject recommendation, spell, and item-route mutations on true copies", () => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-r6-contract-"));
  const scriptCopy = path.join(temp, "champions.js");
  const styleCopy = path.join(temp, "champions.css");
  const backendCopy = path.join(temp, "champions.go");
  for (const filename of fs.readdirSync(root).filter((filename) => filename.endsWith(".go"))) {
    fs.copyFileSync(path.join(root, filename), path.join(temp, filename));
  }
  fs.copyFileSync(path.join(__dirname, "champions.js"), scriptCopy);
  fs.copyFileSync(path.join(__dirname, "champions.css"), styleCopy);
  assert.equal(fs.lstatSync(scriptCopy).isSymbolicLink(), false);
  assert.equal(fs.lstatSync(styleCopy).isSymbolicLink(), false);
  try {
    const originalScript = fs.readFileSync(scriptCopy, "utf8");
    const originalStyles = fs.readFileSync(styleCopy, "utf8");
    const originalBackend = fs.readFileSync(backendCopy, "utf8");
    assert.ok(fs.readdirSync(temp).filter((filename) => filename.endsWith(".go")).length > 1);
    assertR6RecommendationContract(originalScript, originalStyles, originalBackend);
    fs.writeFileSync(scriptCopy, originalScript.replace("return count < 3;", "return count < 4;"));
    assert.throws(() => assertR6RecommendationContract(fs.readFileSync(scriptCopy, "utf8"), originalStyles));
    fs.writeFileSync(scriptCopy, originalScript.replace('4: "SummonerFlash"', '4: "BrokenSpell"'));
    assert.throws(() => assertR6RecommendationContract(fs.readFileSync(scriptCopy, "utf8"), originalStyles));
    fs.writeFileSync(scriptCopy, originalScript.replace('objectRows(row.assets).every((asset) => asset.kind === "item")', "true"));
    assert.throws(() => assertR6RecommendationContract(fs.readFileSync(scriptCopy, "utf8"), originalStyles));
    fs.writeFileSync(styleCopy, originalStyles.replace(
      ".mayhem-recommend-grid { display: grid; grid-template-columns: repeat(3,minmax(0,1fr))",
      ".mayhem-recommend-grid { display: grid; grid-template-columns: repeat(5,minmax(0,1fr))",
    ));
    assert.throws(() => assertR6RecommendationContract(originalScript, fs.readFileSync(styleCopy, "utf8")));
    fs.writeFileSync(backendCopy, originalBackend.replace("if score <= rows[index].Score", "if true"));
    assert.throws(() => assertR6RecommendationContract(originalScript, originalStyles, fs.readFileSync(backendCopy, "utf8")));
  } finally {
    fs.rmSync(temp, { recursive: true, force: true });
  }
});

test("R13 item column contract kills wrapping and equal-width mutations on a true copy", () => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-r13-columns-"));
  const webCopy = path.join(temp, "web");
  fs.mkdirSync(webCopy);
  for (const filename of ["champions.js", "champions.css", "gameplay.js", "gameplay.css", "build-item-row.css"]) {
    fs.copyFileSync(path.join(__dirname, filename), path.join(webCopy, filename));
  }
  fs.copyFileSync(path.join(root, "prestige_chromas.json"), path.join(temp, "prestige_chromas.json"));
  fs.cpSync(path.join(root, "..", "desktop"), path.join(temp, "desktop"), {
    recursive: true,
    filter: (source) => path.basename(source) !== "node_modules",
  });
  try {
    const read = (filename) => fs.readFileSync(path.join(webCopy, filename), "utf8");
    const originals = Object.fromEntries(["champions.js", "champions.css", "gameplay.js", "gameplay.css", "build-item-row.css"].map((filename) => [filename, read(filename)]));
    assert.equal(fs.lstatSync(path.join(webCopy, "champions.css")).isSymbolicLink(), false);
    assert.ok(fs.existsSync(path.join(temp, "desktop", "package.json")));
    assert.ok(fs.existsSync(path.join(temp, "prestige_chromas.json")));
    assertR13ItemColumnContract(originals["champions.js"], originals["champions.css"], originals["gameplay.js"], originals["gameplay.css"]);

    fs.writeFileSync(path.join(webCopy, "build-item-row.css"), originals["build-item-row.css"].replace("minmax(0,1.6fr) repeat(3,minmax(0,1fr))", "minmax(0,1.6fr) repeat(2,minmax(0,1fr))"));
    const mutatedBuildStyles = read("build-item-row.css");
    assert.throws(() => assert.match(cssBlockAfter(mutatedBuildStyles, ".build-item-row"), /minmax\(0,1\.6fr\) repeat\(3,minmax\(0,1fr\)\)/));

    fs.writeFileSync(path.join(webCopy, "build-item-row.css"), originals["build-item-row.css"].replace(".build-item-row > .item-depth-columns { display: contents; }", ".build-item-row > .item-depth-columns { display: grid; }"));
    assert.throws(() => assert.match(read("build-item-row.css"), /display:\s*contents/));
  } finally {
    fs.rmSync(temp, { recursive: true, force: true });
  }
});

test("R6 win rates, spell fallback, patch fallback, and subtitles are explicit", () => {
  const { currentModePatch } = compileFunctions(script, ["currentModePatch"], { state: { rankings: {}, detail: { citation: { patch: "16.16" } } } });
  assert.equal(currentModePatch(), "16.16");
  const state = { detail: { currentPatch: "16.16.1" }, rankings: {} };
  const { summonerSpellAsset } = compileFunctions(script, ["summonerSpellAsset"], { state });
  assert.deepEqual(summonerSpellAsset({ id: 4, name: "4", kind: "spell" }), { id: 4, name: "闪现", kind: "spell", source: "ddragon", path: "/cdn/16.16.1/img/spell/SummonerFlash.png" });
  assert.equal(summonerSpellAsset({ name: "闪现", kind: "spell" }).path, "/cdn/16.16.1/img/spell/SummonerFlash.png");
  const { augmentGrade } = compileFunctions(script, ["augmentGrade"]);
  assert.equal(augmentGrade("", 90, [50, 90, 10, 80, 70, 60, 40, 30, 20]), "S");
  assert.equal(augmentGrade("", 80, [50, 90, 10, 80, 70, 60, 40, 30, 20]), "A");
  assert.equal(augmentGrade("", 50, [50, 90, 10, 80, 70, 60, 40, 30, 20]), "B");
  const routes = functionSource(script, "renderMayhemItemRoutes");
  assert.match(routes, /slice\(0, CORE_RECOMMENDATION_LIMIT\)/);
  assert.doesNotMatch(routes, /route-win-rate|renderConfigOption\(row, "route", true\)/);
  assert.match(routes, /renderConfigOption\(row, "route", false\)/);
  assert.doesNotMatch(functionSource(script, "renderMayhemOpeningConfiguration"), /option-win-rate|renderConfigOption\(row, kind, true\)/);
  assert.match(functionSource(script, "renderMayhemOpeningConfiguration"), /renderConfigOption\(row, kind, false\)/);
  assert.match(functionSource(script, "renderChampionSkillPlan"), /skill-win-rate/);
  const subtitles = ["renderMayhemOpeningConfiguration", "renderMayhemItemRoutes", "renderMayhemItemRanking", "renderRecommendedAugments", "renderMayhemAtlasDetail"].map((name) => functionSource(script, name)).join("\n");
  assert.doesNotMatch(subtitles, /OP\.GG|Hexdata|hexdata|your\.gg|HexScore|globalHexScore/);
  const arenaSources = ["arenaFirstFailureCopy", "arenaFirstUnavailableCopy", "renderArenaFirstPlaces"].map((name) => functionSource(script, name)).join("\n");
  assert.doesNotMatch(arenaSources, /OP\.GG|Hexdata|hexdata|YOUR\.GG|your\.gg/);
  assert.match(functionSource(script, "renderArenaAugments"), /return renderRecommendedAugments\(rows\)/);
  assert.doesNotMatch(functionSource(script, "renderArenaAugments"), /hex-rank|index \+ 1|slice\(3, 10\)/);
  assert.match(styles, /@media \(max-width: 1080px\)[\s\S]*?\.mayhem-atlas\s*\{\s*grid-template-columns:\s*minmax\(0,1fr\) minmax\(0,1fr\)/);
  assert.match(script, /一命三人，六强争胜/);
	assert.match(script, /随机英雄 · 全员海克斯/);
	assert.doesNotMatch(script, /快节奏团战/);
  assert.match(functionSource(script, "renderChampionRow"), /class="champion-row-art"/);
  assert.doesNotMatch(functionSource(script, "renderChampionRow"), /style=|--champion-row-art/);
  assert.match(styles, /\.arena-champions \.is-arena-table \.champion-row-art\s*\{[^}]*display:\s*none/s);
});

test("R7 prism grades, compact arena overview, recoverable match details, and clean names are explicit", () => {
	const { renderArenaOptionCard } = compileFunctions(script, ["renderArenaOptionCard"], {
		renderAssetButton: (asset) => `<button>${asset.name}</button>`,
		arenaRarityKey: () => "prismatic",
		augmentGrade: (grade) => grade || "B",
		escapeHTML: (value) => String(value ?? ""),
		percent: (value) => `${value}%`,
		number: (value) => String(value),
		compactNumber: (value) => String(value),
	});
	const row = { grade: "A", score: 80, winRate: 60, averagePlacement: 3, firstPlaceRate: 20, games: 1000, assets: [{ name: "测试装备" }] };
	const prism = renderArenaOptionCard(row, "prism", 0, [80]);
	assert.match(prism, /class="augment-grade is-A">A<\/b>/);
	assert.doesNotMatch(prism, /hex-rank/);
	const core = renderArenaOptionCard(row, "core", 0, [80]);
	assert.match(core, /class="augment-grade is-A">A<\/b>/);

	const { renderMatchDetailFailure } = compileFunctions(gameplayScript, ["renderMatchDetailFailure"], {
		escapeHTML: (value) => String(value ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;"),
	});
	assert.equal(renderMatchDetailFailure(123, { status: "loading" }), "");
	assert.equal(renderMatchDetailFailure("bad", { status: "failed", message: "error" }), "");
	const failure = renderMatchDetailFailure(123, { status: "failed", message: "<script>bad</script>" });
	assert.match(failure, /data-retry-match-detail="123"/);
	assert.match(failure, /&lt;script&gt;bad&lt;\/script&gt;/);
	assert.doesNotMatch(failure, /<script>/);
	assertR7Contract(script, styles, structuredBackend, hexdataBackend, gameplayScript, gameplayStyles);
});

test("R8 arena augment icons preserve semantic first-place tone without fabricated masks", () => {
  const assetButton = functionSource(script, "renderAssetButton");
  assert.match(assetButton, /augment-icon/);
  assert.match(assetButton, /asset\?\.kind/);
  assert.match(functionSource(script, "renderArenaItemSection"), /arena-\$\{kind\}-section/);
  assert.match(styles, /\.arena-prism-section \.arena-option-main/);
  assert.match(styles, /\.arena-core-section \.arena-option-main/);
  assert.match(styles, /\.arena-overview-metrics \.is-placement strong\s*\{[^}]*var\(--primary-strong\)/);
  assert.match(styles, /\.arena-overview-metrics \.is-first strong\s*\{[^}]*var\(--accent\)/);
  assert.doesNotMatch(styles, /\.arena-overview-metrics \.is-placement strong, \.arena-overview-metrics \.is-first strong/);
});

test("R8 augment assets render as ordinary artwork", () => {
  const { assetImage } = compileFunctions(script, ["assetImage"], {
    imageURL: (source, assetPath) => `/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(assetPath)}`,
    escapeHTML: (value) => String(value ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;"),
  });
  const whiteMask = assetImage({ source: "cdragon", path: "/lol-game-data/assets/ASSETS/Spells/foo_small.png?source=a&path=b", name: "白色海克斯" }, "augment-icon", false);

  const colored = assetImage({ source: "cdragon", path: "/lol-game-data/assets/ASSETS/Spells/arena_2026_s2_power.png", name: "彩色海克斯" }, "augment-icon", false);

  const mercy = assetImage({ source: "cdragon", path: "/lol-game-data/assets/ASSETS/Spells/mercy.png", name: "彩色特例" }, "augment-icon", false);
});

test("icon hotfix prefers Riot large artwork and carries a small fallback", () => {
  const { assetImage } = compileFunctions(script, ["assetImage"], {
    imageURL: (source, assetPath) => `/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(assetPath)}`,
    escapeHTML: (value) => String(value ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;"),
  });
  const large = assetImage({ source: "communitydragon", path: "/latest/game/assets/ux/cherry/augments/icons/foo_large.png", fallbackPath: "/latest/game/assets/ux/cherry/augments/icons/foo_small.png", name: "大图海克斯" }, "augment-icon");
  assert.match(large, /data-augment-fallback="[^"]*foo_small\.png/);
  const ordinarySmall = assetImage({ source: "communitydragon", path: "/latest/game/assets/ux/cherry/augments/icons/foo_small.png", name: "小图海克斯" }, "augment-icon");
});

test("icon hotfix fallback handler switches to small artwork", () => {
  const source = functionSource(script, "prepareImages");
  assert.match(source, /dataset\.augmentFallback/);
  assert.match(source, /augmentFallbackUsed/);
});

test("R8 rendered metric styles do not apply augment masks", () => {
});

test("R8 mayhem item configuration hides win rate when requested", () => {
  const { renderConfigOption } = compileFunctions(
    script,
    ["renderConfigOption"],
    { renderAssetButton: () => "<i></i>", renderOptionStats: () => "<dl><dd>通用统计</dd></dl>", renderOptionPickStats: (row) => row.pickRate > 0 ? "<dl><dd>选用率</dd></dl>" : "" }
  );
  const hidden = renderConfigOption({ assets: [{ name: "装备" }], pickRate: 12, winRate: 68 }, "item", false);
  assert.match(hidden, /选用率/);
  assert.doesNotMatch(hidden, /胜率/);
  const defaultStats = renderConfigOption({ assets: [{ name: "装备" }], pickRate: 12, winRate: 68 }, "item");
  assert.match(defaultStats, /通用统计/);
});

test("R7 repeated disconnected status events preserve fallback item and augment catalogs", () => {
	const state = {
		status: { connected: false }, tabs: [], overlay: [], live: {}, liveError: "", controllers: new Map(),
		activeTabs: { players: "", pro: "" }, tabHistories: { players: [], pro: [] },
		perks: { styles: [{ id: 1 }] }, items: { items: [{ id: 1001 }] }, summonerSpells: { spells: [{ id: 4 }] },
	};
	const loads = [];
	const { updateStatus } = compileFunctions(gameplayScript, ["updateStatus", "resetTencentTabsAfterDisconnect"], {
		state,
		resetLiveGameScopedState: () => {},
		connected: () => Boolean(state.status?.connected),
		riotTab: () => false,
		tabGroup: (tab) => tab?.group === "pro" ? "pro" : tab?.region === "kr" ? "kr" : "players",
		ensurePerks: (force) => loads.push(["perks", force]),
		ensureItems: () => loads.push(["items"]),
		ensureSummonerSpells: () => loads.push(["spells"]),
		updateBeacon: () => {}, renderPlayerTabs: () => {}, renderOverview: () => {}, renderLive: () => {}, renderOverlay: () => {},
	});
	const fallback = { perks: state.perks, items: state.items, spells: state.summonerSpells };
	updateStatus({ connected: false });
	assert.equal(state.perks, fallback.perks);
	assert.equal(state.items, fallback.items);
	assert.equal(state.summonerSpells, fallback.spells);
	assert.deepEqual(loads, []);

	state.status = { connected: true };
	updateStatus({ connected: false });
	assert.equal(state.perks, null);
	assert.equal(state.items, null);
	assert.equal(state.summonerSpells, null);
	assert.deepEqual(loads, [["perks", true], ["items"], ["spells"]]);
});

test("disconnection drops Tencent player identity while preserving Korean tabs", () => {
	let aborted = 0;
	const current = { key: "current", current: true, playerRef: "current-ref", playerRefs: new Set(["current-ref"]), label: "当前玩家#1", icon: 12, data: { player: {} }, loading: true };
	const cn = { key: "cn", current: false, region: "", label: "国服玩家" };
	const kr = { key: "kr", current: false, region: "kr", label: "韩服玩家", data: { player: {} } };
	const state = {
		tabs: [current, cn, kr], activeTabs: { players: "current", pro: "" }, tabHistories: { players: ["cn", "kr"], pro: [] },
		controllers: new Map([["overview:cn", { abort: () => { aborted += 1; } }]]),
	};
	const { resetTencentTabsAfterDisconnect } = compileFunctions(gameplayScript, ["resetTencentTabsAfterDisconnect"], {
		state, riotTab: (tab) => tab.region === "kr", tabGroup: (tab) => tab?.group === "pro" ? "pro" : tab?.region === "kr" ? "kr" : "players",
	});
	resetTencentTabsAfterDisconnect();
	assert.deepEqual(state.tabs.map((tab) => tab.key), ["current", "kr"]);
	assert.equal(state.activeTabs.players, "current");
	assert.equal(state.activeTabs.kr, "kr");
	assert.equal(current.label, "当前召唤师");
	assert.equal(current.playerRef, "");
	assert.equal(current.icon, 0);
	assert.equal(aborted, 1);
});

test("R7 contracts reject UI, parser, backend grade, and retry mutations on true copies", () => {
	const temp = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-r7-contract-"));
	for (const filename of fs.readdirSync(root).filter((filename) => filename.endsWith(".go"))) {
		fs.copyFileSync(path.join(root, filename), path.join(temp, filename));
	}
	const copies = {
		js: path.join(temp, "champions.js"), css: path.join(temp, "champions.css"),
		gameplay: path.join(temp, "gameplay.js"), gameplayCSS: path.join(temp, "gameplay.css"),
	};
	fs.copyFileSync(path.join(__dirname, "champions.js"), copies.js);
	fs.copyFileSync(path.join(__dirname, "champions.css"), copies.css);
	fs.copyFileSync(path.join(__dirname, "gameplay.js"), copies.gameplay);
	fs.copyFileSync(path.join(__dirname, "gameplay.css"), copies.gameplayCSS);
	for (const copy of Object.values(copies)) assert.equal(fs.lstatSync(copy).isSymbolicLink(), false);
	assert.ok(fs.readdirSync(temp).filter((filename) => filename.endsWith(".go")).length > 1);
	try {
		const original = {
			js: fs.readFileSync(copies.js, "utf8"), css: fs.readFileSync(copies.css, "utf8"),
			structured: fs.readFileSync(path.join(temp, "champions_structured.go"), "utf8"),
			hexdata: fs.readFileSync(path.join(temp, "hexdata.go"), "utf8"),
			gameplay: fs.readFileSync(copies.gameplay, "utf8"), gameplayCSS: fs.readFileSync(copies.gameplayCSS, "utf8"),
		};
		const check = (overrides = {}) => assertR7Contract(
			overrides.js || original.js, overrides.css || original.css,
			overrides.structured || original.structured, overrides.hexdata || original.hexdata,
			overrides.gameplay || original.gameplay, overrides.gameplayCSS || original.gameplayCSS,
		);
		check();
		assert.throws(() => check({ js: original.js.replaceAll('kind === "augment" || kind === "prism"', 'kind === "augment"') }));
		assert.throws(() => check({ css: original.css.replace(
			".arena-overview-strip { position: relative; display: flex; min-height: 132px",
			".arena-overview-strip { position: relative; display: flex; min-height: 154px",
		) }));
		assert.throws(() => check({ structured: original.structured.replace("applyLocalAugmentGrades(response.Build.PrismItems)", "") }));
		assert.throws(() => check({ hexdata: original.hexdata.replace('hexdataChampionNickname.ReplaceAllString(name, "")', "name") }));
		assert.throws(() => check({ gameplay: original.gameplay.replaceAll("data-retry-match-detail", "data-broken-retry") }));
		assert.throws(() => check({ gameplay: original.gameplay.replace("if (wasConnected) {", "if (true) {") }));
	} finally {
		fs.rmSync(temp, { recursive: true, force: true });
	}
});

test("mayhem atlas normalizes Riot rarity enums before filtering", () => {
  const state = {
    augmentQuery: "",
    augmentRarity: "gold",
    augments: {
      rows: [
        { id: 1, name: "白银", rarity: "kSilver", tier: 0 },
        { id: 2, name: "黄金", rarity: "kGold", tier: 1 },
        { id: 3, name: "棱彩", rarity: "kPrismatic", tier: 2 },
      ],
    },
  };
  const { augmentRarityKey, filteredAugments } = compileFunctions(
    script,
    ["augmentRarityKey", "filteredAugments"],
    {
      state,
      objectRows: (value) => Array.isArray(value) ? value : [],
      normalizeSearch: (value) => String(value || "").toLowerCase(),
    },
  );

  assert.equal(augmentRarityKey("kPrismatic"), "prismatic");
  assert.equal(augmentRarityKey("bronze"), "unknown");
  assert.deepEqual(filteredAugments().map((item) => [item.id, item.rarity]), [[2, "gold"]]);
});

test("search keeps its caret and IME state when the workspace rerenders", () => {
  assert.match(script, /setSelectionRange/);
  assert.match(script, /compositionstart/);
  assert.match(script, /compositionend/);
  assert.match(styles, /\.champion-search input\s*\{[^}]*direction:\s*ltr/s);
});

test("full Riot ID paste keeps the tag caret without selecting the tag", () => {
  const paste = functionSource(appScript, "absorbPastedRiotID");
  assert.match(paste, /setSelectionRange\?\.\(caret, caret\)/);
  assert.doesNotMatch(paste, /\.select\(\)/);
});

test("champion detail uses complete build, matchup, and augment ranking layouts", () => {
  assert.match(script, /class="champion-rune-board"/);
  assert.match(script, /class="arena-option-grid mayhem-recommend-grid"/);
  assert.match(script, /function renderMayhemOpeningConfiguration\(build, citation\)/);
  assert.match(script, /function renderMayhemItemRoutes\(build, citation\)/);
  assert.match(script, /data-counter-champion/);
	assert.match(script, /ADC_ITEM_ROUTE_LIMIT = 7/);
	assert.match(script, /DEFAULT_ITEM_ROUTE_LIMIT = 6/);
	assert.match(script, /Shoes are rendered separately and do not count toward these route limits/);
	assert.match(script, /DEFAULT_ITEM_ROUTE_LIMIT = 6/);
  assert.match(script, /data-tooltip=/);
  assert.match(script, /heroArtworkURL\(meta, source, path\)/);
  assert.match(script, /meta\?\.artworkSource && meta\?\.artworkPath/);
  assert.match(script, /data-rune-page="\$\{index\}"/);
  assert.match(script, /tier-icons\/\$\{key\}\.svg/);
  assert.match(script, /data-skill-count="\$\{Math\.max\(1, order\.length\)\}"/);
	assert.match(script, /class="counter-champion"/);
  assert.match(script, /function applyRenderedMetricStyles\(\)/);
  assert.doesNotMatch(script, /style="--skill-count:|style="width:\$\{width\}%/);
  assert.match(script, /class="champion-top3"/);
  assert.match(script, /class="metric-games"/);
  assert.match(script, /runeStyleIcon\(page\.primaryStyle, "rune-tab-style"\)/);
  assert.match(script, /runeStyleIcon\(page\.subStyle, "rune-tab-substyle"\)/);
  assert.match(script, /class="spell-option-icons"/);
  assert.match(script, /class="spell-option-stats"/);
  assert.match(script, /class="build-split"/);
  assert.match(script, /class="build-side-row"/);
  assert.match(script, /renderConfigOption\(row, "route", null, renderDepthStats\)/);
	assert.match(script, /class="counter-row"/);
	assert.match(script, /renderCounters\(detail\.counters, detail\.topPlayers, detail\.countersTier \|\| detail\.sampleTier\)/);
  assert.match(script, /rank-crests\/\$\{String\(player\.tier\)/);
  assert.match(script, /class="route-step"/);
  assert.match(gameplayScript, /class="route-step"/);
  assert.doesNotMatch(script, /rune-page-heading|配置\$\{/);
  assert.doesNotMatch(script, /for \(const asset of \(boots/);
  assert.match(script, /\[\["all", "全部"\], \["silver", "白银"\], \["gold", "黄金"\], \["prismatic", "棱彩"\]\]/);
});

test("arena keeps the champion list on wide screens and renders the redesigned data workspace", () => {
  assert.match(script, /function renderArena\(\)/);
  assert.match(script, /class="arena-redesign-workspace"/);
  assert.match(script, /renderChampionTable\(rows, "arena"\)/);
  assert.match(script, /class="arena-detail-pane"/);
  assert.match(script, /class="arena-detail-pane">[\s\S]{0,420}\$\{selected \? renderArenaDetailPane\(selected\)/);
  assert.match(script, /class="arena-overview-metrics"/);
  assert.match(script, /function renderArenaSortBar\(\)/);
  assert.match(script, /function renderArenaAugmentSection\(detail\)/);
  assert.match(script, /detail\.arenaAugmentGroups/);
  assert.match(script, /function renderArenaItemSection/);
  assert.match(script, /function renderArenaCoreSection/);
  assert.match(script, /function renderArenaFirstPlaces\(detail = \{\}\)/);
  assert.match(script, /data-arena-first-tab="pros"/);
  assert.match(script, /data-arena-first-tab="mine"/);
  assert.match(script, /matchCards\.mount\(container/);
  assert.doesNotMatch(script, /class="match-entry/);
  assert.match(script, /teamCompositions/);
  assert.match(script, /function teamName\(team\) \{[\s\S]{0,240}return escapeHTML\(name\);/);
  assert.doesNotMatch(script, /function renderArenaTeam(?:Podium|List)\(/);
  assert.match(script, /function renderArenaDetailTeams\(/);
  assert.doesNotMatch(styles, /\.arena-(?:synergy-head|team-podium|team-place|team-list(?:-head)?|team-row|team-identity)(?!-compact)/);
  assert.match(styles, /\.arena-detail-team-grid/);
  assert.doesNotMatch(script, /斗魂竞技场将在下一阶段接入|renderArenaPlaceholder/);
  assert.match(styles, /\.arena-redesign-workspace\s*\{[^}]*grid-template-columns:\s*minmax\(320px,340px\) minmax\(0,1fr\)/s);
  assert.match(styles, /\.arena-option-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3,minmax\(0,1fr\)\)/s);
  const arenaMarker = "@media (max-width: 1180px)";
  const arenaNarrow = cssBlockAfter(styles.slice(styles.lastIndexOf(arenaMarker)), arenaMarker);
  assert.match(arenaNarrow, /\.arena-redesign-workspace\s*\{[^}]*grid-template-columns:\s*1fr/);
  assert.match(arenaNarrow, /\.arena-redesign-workspace > \.arena-champions\s*\{[^}]*display:\s*none/);
  assert.match(styles, /\.arena-option-card \.is-win dd[^}]*var\(--success\)/);
  assert.match(styles, /\.arena-option-card \.is-placement dd[^}]*var\(--primary-strong\)/);
  assert.match(mainSource, /GET \/api\/champions\/arena-first-places/);
  assert.match(mainSource, /GET \/api\/champions\/arena\/match\/\{matchId\}[^\n]+a\.authorized\(a\.handleArenaMatchDetail\)/);
  assert.match(yourGGArenaSource, /\/kr\/api\/arena\/champions\//);
  assert.match(yourGGArenaSource, /sameYourGGArenaPlayer/);
  assert.match(backend, /\/kr\/api\/arena\/champions/);
  assert.match(backend, /fetchWithMetadata\(ctx, yourGGArenaHost, "\/kr\/api\/arena\/champions"/);
  assert.match(structuredBackend, /response\.TeamCompositions = p\.structuredSynergies\(id, payload\.Data\.Synergies\)/);
  assert.match(fs.readFileSync(path.join(root, "yourgg_arena_rankings.go"), "utf8"), /Region:\s*"KR"/);
});

test("YOUR.GG champion grades preserve all six source letters in project-style badges", () => {
  const render = new Function("tierDisplay", `${functionSource(script,"tierBadge")}; return tierBadge;`)(String);
  for (const grade of ["S","A","B","C","D","F"]) {
    const html=render(0,"",grade);
    assert.match(html,new RegExp(`yourgg-${grade.toLowerCase()}\\.svg`));
    assert.match(html,new RegExp(`alt="梯度 ${grade}"`));
    assert.match(fs.readFileSync(path.join(__dirname,"tier-icons",`yourgg-${grade.toLowerCase()}.svg`),"utf8"),new RegExp(`>${grade}</text>`));
  }
  assert.match(render(1),/\/tier-icons\/1\.svg/);
});

test("arena round 8 removals stay scoped to the arena workspace", () => {
  const arenaWorkspace = ["renderArena", "renderArenaDetailPane", "renderArenaAugmentSection", "renderArenaCoreSection"].map((name) => functionSource(script, name)).join("\n");
  assert.doesNotMatch(arenaWorkspace, /一位率|arena-source-notice|YOUR\.GG · 韩服样本|OP\.GG/);
  assert.doesNotMatch(styles, /\.arena-source-(?:tag|notice)|\.metric-kda|\.arena-sort-bar/);
  const coreSection = script.match(/function renderArenaCoreSection\([\s\S]*?(?=\n  function )/)?.[0] || "";
  assert.ok(coreSection, "renderArenaCoreSection source not found");
  assert.doesNotMatch(coreSection, /build\.skills|技能加点/);
  assert.match(styles, /\.arena-option-card\.is-silver\s*\{[^}]*#9AA7B8/);
  assert.match(styles, /\.arena-option-card\.is-gold\s*\{[^}]*#E3B341/);
  assert.match(styles, /\.arena-option-card\.is-prismatic\s*\{[^}]*#C77DFF[^}]*#5AA9FF[^}]*#FF8AC7/);
});

test("arena tolerates a missing catalog and formats unknown tiers correctly", () => {
  const source = script.match(/function tierDisplay\(value\) \{[\s\S]*?\n  \}/)?.[0];
  assert.ok(source, "tierDisplay source not found");
  const tierDisplay = Function(`"use strict"; ${source}; return tierDisplay;`)();
  assert.equal(tierDisplay(-1), "—");
  assert.equal(tierDisplay(null), "—");
  assert.equal(tierDisplay(undefined), "—");
  assert.equal(tierDisplay(""), "—");
  assert.equal(tierDisplay(0), "OP");
  assert.equal(tierDisplay(3), "3");
  assert.match(script, /function rankingRows\(\) \{ return objectRows\(state\.rankings\?\.rows\); \}/);
  assert.match(script, /if \(!catalog \|\| !Array\.isArray\(catalog\.tiers\) \|\| !Array\.isArray\(catalog\.champions\)\) return;/);
  assert.match(script, /row\.key \|\| meta\?\.slug \|\| row\.champion \|\| row\.championId/);
  assert.doesNotMatch(script, /Promise\.allSettled\(requests\)|queueMicrotask\(/);
  assert.match(script, /state\.arenaDetailError[\s\S]{0,500}renderArenaFirstPlaces\(\{\}\)/);
  assert.match(demoScript, /tier: index === 11 \? -1/);
  assert.match(demoScript, /const demoCatalogFailure = query\.get\("demoCatalogFailure"\) === "1"/);
  assert.match(demoScript, /pathname === "\/api\/champions\/catalog" && demoCatalogFailure[\s\S]{0,180}status: 503/);
  assert.match(demoScript, /arenaRankingsFixture\.rows\.map\(\(\{ key: _key, name: _name, imageSource: _imageSource, imagePath: _imagePath, \.\.\.row \}\) => row\)/);
  assert.match(demoScript, /const payload = demoCatalogFailure \? arenaRankingsWithoutCatalog\(\) : arenaRankingsFixture/);
});

test("match history keeps arena summaries compact and arena details purpose-built", () => {
	assert.match(gameplayScript, /grouping\.groups\.slice\(0,\s*4\)\.map\(/);
	assert.match(gameplayScript, /class="arena-rank-chip\$\{rankTone\}"/);
	const arenaTeamMetaSource = functionSource(gameplayScript, "arenaTeamMeta");
	assert.match(arenaTeamMetaSource, /iconPath: `\/arena-team-icons\/\$\{mascot\.file\}`/);
	assert.doesNotMatch(arenaTeamMetaSource, /\/api\/image|lol-game-data\/assets\/UX\/Cherry\/TeamIcons/);
  assert.match(gameplayScript, /if \(matchPlayerGroups\(match\)\.arena\) \{\s*return `<div[^`]+is-arena-detail[^`]+renderArenaMatchOverview\(match\)/s);
  assert.match(gameplayScript, /function renderArenaMatchOverview\(match\)/);
  assert.match(functionSource(gameplayScript, "renderArenaMatchOverview"), /img data-queued-src="\$\{escapeHTML\(team\.iconPath\)\}"/);
  assert.doesNotMatch(functionSource(gameplayScript, "renderArenaMatchOverview"), /assetIcon\(team\.iconPath/);
  assert.match(gameplayScript, /arena-detail-augments[^\n]+augmentIconFigure/);
	assert.match(gameplayScript, /arena-detail-columns[^\n]+玩家[^\n]+海克斯[^\n]+评分[^\n]+KDA[^\n]+伤害 \/ 承伤[^\n]+装备/);
	const arenaOverviewSource = functionSource(gameplayScript, "renderArenaMatchOverview");
	for (const marker of ["arena-detail-player", "participant-link", "arena-detail-augments", "arena-detail-kda", "arena-detail-damage", "arena-detail-items"]) {
		assert.match(arenaOverviewSource, new RegExp(marker));
	}
	assert.match(arenaOverviewSource, /renderMatchScoreCell\(record\)/);
	assert.ok(arenaOverviewSource.indexOf("renderMatchScoreCell(record)") < arenaOverviewSource.indexOf('class="arena-detail-kda"'));
	assert.match(arenaOverviewSource, /class="arena-detail-damage"[^>]+aria-label=[^>]+><b class="match-damage-value">\$\{plainInteger\(item\.damage\)\}<\/b><i aria-hidden="true">\/<\/i><b class="match-taken-value">\$\{plainInteger\(item\.damageTaken\)\}<\/b>/);
	assert.doesNotMatch(arenaOverviewSource, /<small>伤害<\/small>|<small>承伤<\/small>/);
	assert.match(functionSource(gameplayScript, "renderMatchLoadout"), /matchAugmentIDs\(subject, modeKind === "mayhem" \? 2 : 4\)[\s\S]{0,180}Array\.from\(\{ length: modeKind === "mayhem" \? 2 : 4 \}/);
	assert.match(gameplayScript, /function matchModeKind\(match, subject\)/);
	assert.match(gameplayScript, /const spells = \[[\s\S]{0,260}spellIconFigure\(subject\.spell2Id, "small"\)/);
  assert.match(gameplayScript, /const statRows = modeKind === "arena" \? `\$\{damageRow\}\$\{takenRow\}` : `\$\{participationRow\}\$\{csRow\}\$\{tierRow\}`/);
  assert.match(functionSource(gameplayScript, "renderMatch"), /damageRow|takenRow/);
	assert.doesNotMatch(gameplayScript, /<em>(?:Damage|DT)<\/em>/);
	assert.match(gameplayScript, /window\.deepLegendsMatchCards = Object\.freeze/);
	assert.match(gameplayScript, /disableExpand: options\.disableExpand === true/);
	assert.match(gameplayScript, /function matchHasCompleteParticipantStats\(match\)/);
	assert.match(gameplayScript, /function hydratedExternalMatch\(original, hydrated, playerRef\)/);
	assert.match(gameplayScript, /detailStates\.set\(id, \{ status: "loading"/);
	assert.match(gameplayScript, /detailStates\.set\(id, \{ status: "failed", message \}\)/);
	assert.match(gameplayScript, /for \(const mountedContainer of externalMatchViews\.keys\(\)\)/);
	assert.match(gameplayStyles, /\.match-stat-participation > b\s*\{[^}]*var\(--success\)/);
	assert.match(gameplayStyles, /\.match-taken-value\s*\{[^}]*var\(--muted\)/);
	assert.match(gameplayStyles, /\.match-loadout-mini\s*\{[^}]*grid-template-columns:\s*repeat\(2,26px\)/);
	assert.match(gameplayStyles, /\.match-summary\.is-arena\s*\{[^}]*minmax\(220px,1fr\)[^}]*var\(--match-roster-width(?:,\s*clamp\([^)]*\))?\)[^}]*40px/s);
	assert.match(gameplayStyles, /:root\s*\{[^}]*--match-roster-width:\s*clamp\(/s);
	// R88：名单等宽收窄，两队等分且间距固定。
	assert.match(gameplayStyles, /\.match-summary\s*\{[^}]*grid-template-columns:\s*108px minmax\(0,1fr\) var\(--match-roster-width,clamp\(150px,18cqi,190px\)\) 40px/s);
	assert.match(gameplayStyles, /\.match-players\s*\{[^}]*width:\s*100%;[^}]*gap:\s*2px 14px/s);
	assert.doesNotMatch(gameplayStyles, /\.match-players\s*\{[^}]*justify-content:\s*space-between/s);
	assert.doesNotMatch(gameplayStyles, /@container arena-first \(max-width: (?:720|520|420)px\)/);
	assert.match(gameplayStyles, /\.match-players\s*\{[^}]*grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/s);
	// ★战绩条左半边（英雄 / KDA / 击杀·CS·段位）的排布是设计基准，加宽玩家名单不许动它：
	// 第四条空的 1fr 轨道负责吃掉富余宽度，统计块靠左，KDA 保持在第二条 96px 轨道上。
	assert.match(gameplayStyles, /\.match-main\s*\{[^}]*grid-template-columns:\s*minmax\(0,auto\) minmax\(0,96px\) minmax\(0,auto\) minmax\(0,1fr\)/s);
	assert.doesNotMatch(gameplayStyles, /\.match-main\s*\{[^}]*justify-content:/s);
	assert.match(gameplayStyles, /\.match-stats\s*\{[^}]*justify-self:\s*start/s);
	assert.doesNotMatch(gameplayStyles, /\.match-stats\s*\{[^}]*(?:max-)?width:\s*min\(/s);
	assert.doesNotMatch(gameplayStyles, /var\(--match-roster-width\)/);
	assert.match(gameplayStyles, /\.match-players\.is-arena\s*\{[^}]*max-height:\s*98px[^}]*overflow:\s*hidden/);
	assert.match(gameplayStyles, /\.arena-team-row-compact\s*\{[^}]*padding:\s*1px 5px/);
	assert.match(gameplayStyles, /@container matches-column \(max-width: 720px\)[\s\S]{0,260}\.match-players, \.match-players\.is-arena\s*\{\s*display:\s*none/);
  assert.match(gameplayStyles, /\.match-main\s*\{[^}]*minmax\(0,96px\)/s);
	assert.match(gameplayStyles, /\.match-main\.is-arena\s*\{[^}]*minmax\(0,96px\)/s);
	assert.match(gameplayStyles, /\.match-main\.is-arena\s*\{[^}]*minmax\(0,1fr\)/s);
	assert.doesNotMatch(gameplayStyles, /\.match-main\.is-arena\s*\{[^}]*row-gap:\s*5px/);
	assert.match(gameplayStyles, /\.match-players\.is-arena\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
	assert.match(gameplayStyles, /\.arena-detail-columns\s*\{[^}]*grid-template-columns:\s*minmax\(140px,\.9fr\) 104px 96px 100px 116px 200px[^}]*justify-items:\s*center[^}]*column-gap:\s*12px[^}]*text-align:\s*center/s);
	assert.match(gameplayStyles, /\.arena-detail-player\s*\{[^}]*grid-template-columns:\s*minmax\(140px,\.9fr\) 104px 96px 100px 116px 200px[^}]*justify-items:\s*center[^}]*column-gap:\s*12px[^}]*text-align:\s*center/s);
	assert.match(gameplayStyles, /\.arena-detail-augments\s*\{[^}]*display:\s*grid[^}]*grid-template-columns:\s*repeat\(3,30px\)[^}]*justify-content:\s*center/s);
	assert.match(gameplayStyles, /\.arena-detail-damage\s*\{[^}]*display:\s*flex[^}]*justify-content:\s*center[^}]*white-space:\s*nowrap/s);
	assert.match(gameplayStyles, /\.arena-detail-columns > :first-child\s*\{[^}]*justify-self:\s*start[^}]*text-align:\s*left/s);
	assert.match(gameplayStyles, /\.arena-detail-player > \.participant-link\s*\{[^}]*width:\s*auto[^}]*justify-self:\s*start[^}]*justify-content:\s*flex-start/s);
	assert.match(gameplayStyles, /\.arena-detail-items\s*\{[^}]*justify-self:\s*stretch[^}]*justify-content:\s*flex-start[^}]*flex-wrap:\s*nowrap/s);
	assert.match(gameplayStyles, /\.arena-detail-player \.match-score-cell, \.match-table \.match-score-cell\s*\{[^}]*width:\s*86px[^}]*grid-template-columns:\s*34px 48px[^}]*justify-content:\s*center[^}]*justify-items:\s*center/s);
	assert.match(gameplayStyles, /\.match-table th, \.match-table td\s*\{[^}]*text-align:\s*center[^}]*vertical-align:\s*middle/s);
	assert.match(gameplayStyles, /\.match-table th:first-child, \.match-table td:first-child\s*\{[^}]*text-align:\s*left/s);
	assert.match(gameplayStyles, /\.match-table \.participant-link\s*\{[^}]*width:\s*100%[^}]*min-width:\s*0[^}]*justify-content:\s*flex-start/s);
	assert.match(gameplayStyles, /\.match-table \.table-items\s*\{[^}]*justify-content:\s*flex-start/s);
	assert.match(gameplayStyles, /\.matches-column\s*\{[^}]*container:\s*matches-column \/ inline-size/);
	assert.match(gameplayStyles, /@container matches-column \(max-width: 720px\)[\s\S]{0,260}\.arena-detail-columns, \.arena-detail-team\s*\{[^}]*min-width:\s*860px/);
	// Arena records now own the same matches-column container as player history.
	assert.doesNotMatch(gameplayStyles, /@container arena-first \(max-width: 640px\)/);
	assert.doesNotMatch(gameplayStyles, /\.arena-column-(?:augments|damage|items|score)[^}]*display:\s*none|\.arena-detail-(?:augments|damage|items)[^}]*display:\s*none/);
	assert.doesNotMatch(gameplayStyles, /@container matches-column \(max-width: 1080px\)[\s\S]{0,700}\.match-player-name\s*\{\s*display:\s*none/);
	assert.doesNotMatch(gameplayStyles, /@container arena-first[\s\S]{0,700}\.match-player-name\s*\{\s*display:\s*none/);
	assert.match(gameplayStyles, /@container matches-column \(max-width: 640px\)[\s\S]{0,180}\.match-main, \.match-main\.is-arena/);
	assert.match(gameplayStyles, /@container matches-column \(max-width: 640px\)[\s\S]{0,220}\.match-stats\s*\{\s*display:\s*grid/);
	assert.match(gameplayStyles, /\.match-stats\s*\{[^}]*display:\s*grid/);
	assert.match(gameplayStyles, /@container matches-column \(max-width: 640px\)[\s\S]{0,320}\.match-build\s*\{[^}]*column-gap:\s*4px/);
	assert.match(gameplayStyles, /@container matches-column \(max-width: 420px\)[\s\S]{0,520}\.match-summary\.is-arena \.match-main\s*\{[^}]*grid-column:\s*1 \/ -1[^}]*grid-row:\s*2/);
	assert.match(gameplayStyles, /\.match-entry\s*\{[^}]*min-height:\s*118px[^}]*contain-intrinsic-size:\s*118px/);
	assert.match(gameplayStyles, /\.arena-team-row-compact\s*\{[^}]*grid-template-columns:\s*18px minmax\(0,1fr\)/);
	assert.match(gameplayStyles, /\.arena-rank-chip\s*\{[^}]*width:\s*18px[^}]*height:\s*18px/);
	assert.match(gameplayStyles, /\.arena-rank-chip\.is-rank-1\s*\{[^}]*#2A1D05[^}]*#F5D372[^}]*#D9A441[^}]*#E8C468/);
	assert.match(gameplayStyles, /\.arena-rank-chip\.is-rank-2\s*\{[^}]*#1C2027[^}]*#DDE4EC[^}]*#AFBAC8[^}]*#C7D0DC/);
	assert.match(gameplayStyles, /\.arena-rank-chip\.is-rank-3\s*\{[^}]*#2A1A0E[^}]*#DCA070[^}]*#B87333[^}]*#C8834A/);
  assert.doesNotMatch(gameplayScript, /if \(matchPlayerGroups\(match\)\.arena\)[\s\S]{0,220}match-detail-tabs/);
});

test("arena match detail uses a validated Riot route and fail-closed hydration", () => {
	assert.match(arenaMatchDetailSource, /\^KR_\[0-9\]\+\$/);
	assert.match(arenaMatchDetailSource, /matchByIDWithCache/);
	assert.match(arenaMatchDetailSource, /publicizeMatchReferences/);
	assert.match(arenaMatchDetailSource, /"event":\s*"arena_match_detail"/);
	assert.doesNotMatch(arenaMatchDetailSource, /championDataCache|save|persist/i);
	assert.match(script, /loadMatchDetails:[\s\S]{0,300}\/api\/champions\/arena\/match\/KR_/);
	assert.doesNotMatch(script, /disableExpand:\s*true[\s\S]{0,100}YOUR\.GG/);
});

test("demo mode serves complete arena fixtures without stable account identifiers", () => {
  assert.match(demoScript, /championsCatalogFixture/);
  assert.match(demoScript, /arenaRankingsFixture/);
  assert.match(demoScript, /arenaDetailFixture/);
  assert.match(demoScript, /arenaFirstPlacesFixture/);
  assert.match(demoScript, /pathname === "\/api\/champions\/arena-first-places"/);
  assert.ok(demoScript.includes("pathname.match(/^\\/api\\/champions\\/arena\\/match\\/KR_"));
  assert.match(demoScript, /demoArenaMatchDetails/);
  assert.match(demoScript, /artworkSource: "ddragon"/);
  assert.match(demoScript, /champion\/splash\/\$\{key\}_0\.jpg/);
	assert.match(demoScript, /21 名玩家、3 人一队共 7 支小队/);
	assert.match(demoScript, /const placements = \[1, 2, 3, 4, 5, 6, 7\]/);
	assert.match(demoScript, /averageTier: \{ tier: "PLATINUM", division: "I", samples: 21 \}/);
  assert.doesNotMatch(demoScript, /summonerId\s*:/);
});

test("demo overview includes a match for every selectable game mode", () => {
  assert.match(demoScript, /queueId: mode\.queueId \?\? 420/);
  assert.match(demoScript, /queueLabel: mode\.queueLabel \|\| "单排\/双排"/);
  assert.match(demoScript, /modeGroup: mode\.modeGroup \|\| "solo"/);
  assert.match(demoScript, /const demoRemakeMatch[\s\S]*?match\.result = "remake"/);
  assert.doesNotMatch(demoScript, /match\.(?:participants|teams)\.forEach\(\(item\) => \{ item\.win = false;/);
  const modes = [
    [440, "灵活组排", "flex"],
    [2300, "海克斯大乱斗", "hextech-aram"],
    [2600, "海克斯大乱斗 海选赛", "hextech-qualifier"],
    [2400, "海克斯大乱斗 经典模式版", "hextech-classic"],
    [1700, "斗魂竞技场", "arena"],
    [450, "极地大乱斗", "aram"],
    [430, "匹配模式", "other"],
    [850, "人机对战", "other"],
    [900, "无限火力", "urf"],
    [700, "冠军杯赛", "other"],
    [1300, "极限闪击", "nexus-blitz"],
    [950, "末日人工智能", "other"],
    [1400, "特殊模式", "other"],
  ];
  for (const [queueId, queueLabel, modeGroup] of modes) {
    assert.match(demoScript, new RegExp(`queueId: ${queueId}, queueLabel: "${queueLabel}", modeGroup: "${modeGroup}"`));
  }
  assert.match(demoScript, /pagination: \{ begIndex: 0, count: 17, hasMore: false \}/);
  assert.match(demoScript, /\{ name: "match-history", state: "available", count: 17 \}/);
  assert.match(demoScript, /match\.result = "remake"/);
  assert.match(demoScript, /augmentIds: \[1205, 1141, 1002, 2087\]/);
  assert.match(demoScript, /const \{ primaryStyleId, subStyleId, perkIds, \.\.\.withoutRunes \} = item/);
});

test("demo mode exposes a dedicated Hextech ARAM live session", () => {
	assert.match(demoScript, /const hextechLiveDemo = query\.get\("demo"\) === "hextech"/);
	assert.match(demoScript, /const hextechLive = \{[\s\S]*queueId: 2300,[\s\S]*queueLabel: "海克斯大乱斗",[\s\S]*modeGroup: "hextech-aram",[\s\S]*gameMode: "KIWI",[\s\S]*mapId: 12,/);
	assert.match(demoScript, /resolvedMode: "hextech",[\s\S]*hasRunes: false,[\s\S]*hasAugments: true,[\s\S]*hasTopPlayers: false/);
	assert.match(demoScript, /hero: \{ tier: 1, winRate: 54\.72, pickRate: 7\.83 \}/);
	assert.match(demoScript, /const hextechLiveAugments = arenaLiveAugments\.map[\s\S]*score: Number\(\(92\.4 - index \* 3\.1\)\.toFixed\(1\)\)/);
	assert.match(demoScript, /"\/api\/gameplay\/live", \(\) => hextechLiveDemo \? hextechLive : arenaFullDemo \? arenaFullLive : arenaLiveDemo \? arenaLive : live/);
});

test("demo mode exposes a complete six-team Arena live session", () => {
	assert.match(demoScript, /const arenaFullDemo = query\.get\("demo"\) === "arena-full"/);
	assert.match(demoScript, /const arenaLivePlayers = arenaChampionPool\.slice\(0, 18\)/);
	assert.match(demoScript, /arenaGroup: String\(Math\.floor\(index \/ 3\) \+ 1\)/);
	assert.match(demoScript, /const arenaFullLive = \{[\s\S]*phase: "InProgress",[\s\S]*players: structuredClone\(arenaLivePlayers\),[\s\S]*arenaGrouped: true,[\s\S]*arenaMascotMapping: true/);
});

test("small overviews move career statistics into an accessible modal sheet", () => {
	assert.match(html, /id="career-dialog"[^>]+aria-labelledby="career-dialog-title"[^>]+aria-describedby="career-dialog-context"/);
	assert.match(html, /id="career-dialog-close"[^>]+aria-label="关闭生涯统计"/);
	assert.match(gameplayScript, /function renderCareerSections\(data(?:, tab)?\)/);
	assert.match(gameplayScript, /function renderCareerDialogSections\(data(?:, tab)?\)/);
	assert.match(gameplayScript, /function renderCareerDialogContent\(data, tab, layout\)/);
	assert.match(gameplayScript, /const primaryKeys = \["ranks", "champions", "activity"\]/);
	assert.match(gameplayScript, /const secondaryKeys = \["recent-ranked", "ability", "positions", "masteries", "recent-players"\]/);
	assert.match(gameplayScript, /layout === "single" \? renderCareerSections\(data, tab\) : renderCareerDialogSections\(data, tab\)/);
	assert.match(gameplayScript, /dialogWidth <= 640 \? "single" : "masonry"/);
	assert.match(gameplayScript, /aria-haspopup="dialog" aria-controls="career-dialog" data-open-career-dialog/);
	assert.match(gameplayScript, /nodes\.careerDialog\.showModal\(\)/);
	assert.match(gameplayScript, /new ResizeObserver\(syncWidth\)/);
	assert.match(gameplayScript, /nodes\.careerDialog\.open && width > 1020[\s\S]+closeCareerDialog\(\)/);
	assert.match(gameplayScript, /applyRenderedMetricStyles\(nodes\.careerDialogContent\)/);
	assert.match(gameplayScript, /event\.target === nodes\.careerDialog/);
	assert.match(gameplayScript, /event\.key !== "Escape"[^}]+event\.stopPropagation\(\)[^}]+closeCareerDialog\(\)/);
	assert.match(gameplayScript, /opener\?\.isConnected[^}]+opener\.focus\(\{ preventScroll: true \}\)/);
	assert.match(gameplayScript, /nodes\.playerOverlayBack[\s\S]+\[data-player-tab\]\[aria-selected="true"\]/);
	assert.match(gameplayStyles, /@container gameplay-page \(max-width: 1020px\)[\s\S]+\.overview-layout > \.career-column\s*\{\s*display:\s*none/);
	assert.match(gameplayStyles, /@container gameplay-page \(max-width: 1020px\)[\s\S]+\.overview-career-entry\s*\{\s*display:\s*flex/);
	assert.match(gameplayStyles, /\.career-dialog\s*\{[^}]*inset:\s*0[^}]*margin:\s*auto/s);
	assert.match(gameplayStyles, /\.career-dialog::backdrop\s*\{[^}]*background:\s*var\(--overlay\)/s);
	assert.match(gameplayStyles, /\.career-dialog-grid\s*\{[^}]*grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/s);
	assert.match(gameplayStyles, /\.career-dialog-grid\s*\{[^}]*grid-auto-rows:\s*max-content/s);
	assert.match(gameplayStyles, /\.career-dialog-stack\s*\{[^}]*display:\s*grid[^}]*gap:\s*10px/s);
	assert.match(gameplayStyles, /@container career-dialog \(max-width: 640px\)[\s\S]+\.career-dialog-grid\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/);
});

test("rank history and ability comparison stay inside the shared career surface", () => {
	assert.match(gameplayScript, /renderRanks\(data\.ranks \|\| \[\], data\.capabilities \|\| \[\], data\.historicalRanks \|\| \[\], data\.rankMilestones, data\.seasonStatsProgress, tab\)/);
	assert.match(gameplayScript, /function renderHistoricalRanks\(items\)/);
	assert.match(gameplayScript, /function renderRankMilestones\(milestones\)/);
	assert.match(gameplayScript, /return renderHistoricalRanks\(historicalRanks\) \|\| renderRankMilestones\(rankMilestones\)/);
	assert.match(gameplayScript, /index >= 5 \? " data-rank-history-extra hidden"/);
	assert.match(gameplayScript, /data-rank-history-toggle/);
	assert.match(gameplayScript, /expanded \? "收起" : "查看更多赛段段位"/);
	assert.match(gameplayScript, /function bindRankHistoryControls\(container\)/);
	assert.match(gameplayScript, /bindRankHistoryControls\(nodes\.careerDialogContent\)/);
	assert.match(gameplayScript, /OP\.GG 的历史赛段记录未提供胜负场，无法核验胜率/);
	assert.match(gameplayScript, /renderAbility\(abilityQueue\.ability, abilityQueue\.queueLabel/);
	assert.match(gameplayScript, /function renderAbility\(ability, fallbackQueueLabel = "", queueSwitcher = "", sampleGames = 0, queueGames = 0\)/);
	assert.match(gameplayScript, /ability-radar-baseline/);
	assert.match(gameplayScript, /ability-radar-player/);
	assert.match(gameplayScript, /ability-radar-metric is-metric-\$\{index\}" type="button"/);
	assert.match(gameplayScript, /ability-radar-controls" role="group" aria-label=/);
	assert.match(gameplayScript, /class="ability-radar"[^>]+aria-hidden="true" focusable="false"/);
	assert.doesNotMatch(gameplayScript, /class="ability-radar"[^>]+role="img"/);
	assert.doesNotMatch(gameplayScript, /ability-radar-metric[^>]+style=/);
	assert.match(gameplayStyles, /\.ability-radar-metric\.is-metric-0\s*\{[^}]*left:\s*50%[^}]*top:\s*12%/s);
	assert.match(gameplayStyles, /\.ability-radar-metric\.is-metric-3\s*\{[^}]*left:\s*66%[^}]*top:\s*91%/s);
	assert.match(gameplayStyles, /\.ability-radar-metric\.is-metric-6\s*\{[^}]*left:\s*21%[^}]*top:\s*25%/s);
	assert.match(gameplayScript, /item\.leaguePoints === null \|\| item\.leaguePoints === undefined \? "—"/);
	assert.match(gameplayScript, /对手基准来自这名玩家排位中的同位置对手样本聚合，不是全服或段位平均/);
	assert.match(gameplayScript, /当前玩家 \$\{abilityMetricValue\(metric, "player"\)\} · \$\{baselineLabel\}/);
	assert.match(demoScript, /baselineLabel: "近期同位置对手样本"/);
	assert.doesNotMatch(demoScript, /baselineLabel: "翡翠 II 平均"/);
	assert.doesNotMatch(gameplayScript, /评级 \$\{metric\.grade\} · 玩家/);
	assert.match(gameplayScript, /至少需要 3 场包含完整参与者数据的排位对局/);
	assert.match(gameplayStyles, /\.rank-history-head, \.rank-history-row\s*\{[^}]*grid-template-columns:\s*72px minmax\(0,1fr\) 52px/s);
	assert.match(gameplayStyles, /@container career-dialog \(max-width: 360px\)[\s\S]+\.rank-history-head, \.rank-history-row\s*\{[^}]*grid-template-columns:\s*64px minmax\(0,1fr\) 44px/);
	assert.match(gameplayStyles, /@container career-dialog \(max-width: 360px\)[\s\S]+\.rank-history-crest\s*\{\s*display:\s*none/);
	assert.match(gameplayStyles, /\.rank-milestones\s*\{[^}]*grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/s);
	assert.match(gameplayStyles, /@container career-dialog \(max-width: 260px\)[\s\S]+\.rank-milestones\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/);
	assert.match(gameplayStyles, /\.ability-radar-baseline\s*\{[^}]*stroke:/s);
	assert.match(gameplayStyles, /\.ability-radar-player\s*\{[^}]*var\(--primary-strong\)/s);
	assert.match(gameplayScript, /function radarPoint\(index, score, count = 7, radius = 90, centerX = 160, centerY = 144\)/);
	assert.match(gameplayScript, /viewBox="0 0 320 275"/);
	assert.match(gameplayStyles, /\.ability-radar-wrap\s*\{[^}]*max-width:\s*325px/s);
	assert.match(demoScript, /historicalRanks:\s*\[/);
	assert.match(demoScript, /仅韩服 OP\.GG 链路会返回这个多赛段形状；国服 historicalRanks 恒空，改走 rankMilestones/);
	assert.match(demoScript, /season:\s*"S2023 S1"/);
	assert.match(demoScript, /key:\s*"vspm"/);

	const dependencies = {
		escapeHTML: (value) => String(value),
		rankCrestIcon: (tier) => `[crest:${tier}]`,
		rankTitle: (rank) => `${rank.tier}${rank.division ? ` ${rank.division}` : ""}`,
	};
	const { renderRankMilestones } = compileFunctions(gameplayScript, ["renderRankMilestones"], dependencies);
	const cn = renderRankMilestones({
		peakTier: "diamond", peakDivision: "IV",
		previousSeason: [{ queueType: "RANKED_SOLO_5x5", tier: "emerald", division: "I", highestTier: "diamond", highestDivision: "IV" }],
	});
	assert.match(cn, /历史最高/);
	assert.match(cn, /\[crest:diamond\][\s\S]+diamond IV/);
	assert.match(cn, /上赛季/);
	assert.match(cn, /最高 diamond IV/);
	assert.equal(renderRankMilestones({}), "");
	assert.equal(renderRankMilestones({ peakTier: "", previousSeason: [] }), "");
	const sameTier = renderRankMilestones({ previousSeason: [{ queueType: "RANKED_SOLO_5x5", tier: "emerald", division: "IV", highestTier: "emerald", highestDivision: "I" }] });
	assert.doesNotMatch(sameTier, /最高 emerald/);
	const peakOnly = renderRankMilestones({ peakTier: "master", peakDivision: "I" });
	assert.match(peakOnly, /历史最高/);
	assert.doesNotMatch(peakOnly, /上赛季/);

	const { renderRankHistory } = compileFunctions(gameplayScript, ["renderRankHistory"], {
		renderHistoricalRanks: (items) => items.length ? "KR-HISTORY" : "",
		renderRankMilestones: (milestones) => milestones?.peakTier ? "CN-MILESTONES" : "",
	});
	assert.equal(renderRankHistory([{ season: "S2025" }], { peakTier: "diamond" }), "KR-HISTORY");
	assert.equal(renderRankHistory([], { peakTier: "diamond" }), "CN-MILESTONES");
	assert.equal(renderRankHistory([], null), "");
});

test("career ranked queue switches use one recent sample and ignore season scan state", () => {
  const tabSource = functionSource(gameplayScript, "newTabView");
  assert.match(tabSource, /rankedQueueRecent:\s*"420"/);
  assert.match(tabSource, /rankedQueueAbility:\s*"420"/);
  assert.match(tabSource, /rankedQueuePosition:\s*"420"/);
  const switcher = functionSource(gameplayScript, "rankedQueueSwitcher");
  assert.match(switcher, /data-ranked-queue-scope="\$\{scope\}"/);
  const binder = functionSource(gameplayScript, "bindRankedQueueControls");
  assert.match(binder, /rankedQueueRecent/);
  assert.match(binder, /rankedQueueAbility/);
  assert.match(binder, /rankedQueuePosition/);
  assert.doesNotMatch(gameplayScript, /function addRankedQueueTools/);
  assert.doesNotMatch(gameplayScript, /tab\.rankedQueue\s*=/);

	const { rankedQueueData } = compileFunctions(gameplayScript, ["rankedQueueData"], {
		rankedQueueLabel: (queueId) => queueId === 440 ? "灵活组排" : "单双排",
	});
	const { renderAbility, renderRecentRanked, renderPositionStats } = compileFunctions(gameplayScript, ["renderAbility", "renderRecentRanked", "renderPositionStats"], {
		escapeHTML: (value) => String(value),
		positionIcon: () => "",
		positionLabel: (value) => value || "位置",
		number: (value) => String(value),
		percent: (value) => `${value}%`,
		kda: (value) => String(value),
		radarPoint: (index) => [160 + index, 144 + index],
		radarPoints: () => "0,0 1,1 2,2",
		abilityMetricValue: (metric, key) => String(metric[key] ?? "—"),
	});
	for (const [key, label] of [["420", "单双排"], ["440", "灵活组排"]]) {
		const tab = { rankedQueueRecent: key, rankedQueueAbility: key, rankedQueuePosition: key };
		const unavailableSeasonPayload = {
			rankedQueues: { [key]: { recentRanked: { games: 0, queueLabel: label }, abilitySampleGames: 0, positions: [] } },
			seasonStatsProgress: { season: "S2026", complete: false, unavailable: true, scanned: 0 },
		};
		for (const scope of ["recent", "ability", "position"]) {
			const queue = rankedQueueData(unavailableSeasonPayload, tab, scope);
			assert.equal("collecting" in queue, false, `${label} 不应再读取赛季扫描状态`);
			assert.equal("seasonComplete" in queue, false);
			assert.equal("unavailable" in queue, false);
			assert.equal(queue.queueLabel, label);
		}
		for (const markup of [
			renderRecentRanked({ queueLabel: label }, Number(key), "SWITCH"),
			renderAbility(null, label, "SWITCH", 0, 0),
			renderPositionStats([], Number(key), "SWITCH", label, 0),
		]) {
			assert.match(markup, /首屏已加载|首屏最近|最近对局/);
			assert.doesNotMatch(markup, /统计中|season-progress-badge|本赛季/);
			assert.doesNotMatch(markup, label === "单双排" ? /灵活组排/ : /单双排/);
		}
	}

	assert.match(renderAbility(null, "单双排", "SWITCH", 2, 2), /近期对局中单双排样本不足（需 ≥3 场，当前 2 场）/);
	assert.doesNotMatch(renderAbility(null, "单双排", "SWITCH", 0, 2), /本赛季未参加|统计中/);
	assert.doesNotMatch(renderPositionStats([], 440, "SWITCH", "灵活组排", 2), /本赛季未参加|统计中/);
	assert.match(renderRecentRanked({ queueLabel: "单双排" }, 420, "SWITCH"), /当前样本未发现单双排对局/);

	const metrics = Array.from({ length: 7 }, (_, index) => ({ key: `metric-${index}`, label: `指标${index}`, playerScore: 50, player: 1, baseline: 1, grade: "A" }));
	const recentMarkups = [
		renderRecentRanked({ games: 7, wins: 4, losses: 3, queueLabel: "单双排", positions: [] }, 420, "SWITCH"),
		renderAbility({ metrics, queueLabel: "单双排", sampleGames: 7, baselineGames: 7 }, "单双排", "SWITCH", 7, 7),
		renderPositionStats([{ position: "top", games: 7, share: 100 }], 420, "SWITCH", "单双排", 7),
	];
	for (const markup of recentMarkups) {
		assert.match(markup, /近 7 场|7 场样本/);
		assert.doesNotMatch(markup, /season-progress-badge|统计中|已统计 137 场/);
	}
	assert.doesNotMatch(gameplayScript, /function renderSeasonProgressBadge/);
	const entries = functionSource(gameplayScript, "careerSectionEntries");
	assert.doesNotMatch(entries, /render(?:RecentRanked|Ability|PositionStats)\([^\n]*seasonStatsProgress/);
	assert.match(entries, /renderChampionStats\(championRows, championOverall, championProgress\)/);
	assert.match(entries, /const championProgress = opggSeason[\s\S]*: data\.seasonStatsProgress/);
});

test("season progress refreshes the active overview without resetting queue choices or scroll", async () => {
	const tab = {
		key: "other", playerRef: "public-ref", rankedQueueRecent: "440", rankedQueueAbility: "420", rankedQueuePosition: "440",
		data: { player: { playerRef: "public-ref" }, seasonStatsProgress: { season: "S26", scanned: 40, complete: false } },
	};
	const state = { section: "overview", seasonProgressRefreshes: new Map() };
	const scrollRoot = { scrollTop: 321, isConnected: true };
	const calls = [];
	const timers = [];
	let currentTab = tab;
	let switchDuringRefresh = false;
	const { handleSeasonProgress } = compileFunctions(gameplayScript, ["handleSeasonProgress"], {
		state,
		activeTab: () => currentTab,
		overviewGroupForSection: () => "players",
		overviewSectionForGroup: () => "overview",
		document: { getElementById: (id) => id === "app-scroll" ? scrollRoot : null },
		loadOverview: async (...args) => { calls.push(args); tab.data.seasonStatsProgress = { season: "S26", scanned: 180, complete: true }; scrollRoot.scrollTop = 0; if (switchDuringRefresh) currentTab = { key: "new-active" }; },
		requestAnimationFrame: (callback) => callback(),
		setTimeout: (callback, delay) => { timers.push({ callback, delay }); return timers.length; },
	});
	assert.equal(await handleSeasonProgress({ type: "season-progress", season: "S26", account: "public-ref", scanned: 180, complete: true }, 20_000), true);
	assert.equal(calls.length, 1);
	assert.deepEqual(calls[0].slice(1), [true, false, false, true]);
	assert.equal(scrollRoot.scrollTop, 321);
	assert.deepEqual([tab.rankedQueueRecent, tab.rankedQueueAbility, tab.rankedQueuePosition], ["440", "420", "440"]);

	tab.data.seasonStatsProgress = { season: "S26", scanned: 180, complete: false };
	assert.equal(await handleSeasonProgress({ type: "season-progress", season: "S26", account: "public-ref", scanned: 300, complete: false }, 25_000), false);
	assert.equal(calls.length, 1, "同一账号十秒内不应再次刷新");
	assert.equal(timers.length, 1);
	assert.equal(timers[0].delay, 5_000);
	assert.equal(await handleSeasonProgress({ type: "season-progress", season: "S26", account: "another-player", scanned: 500, complete: true }, 40_000), false);

	tab.data.seasonStatsProgress = { season: "S26", scanned: 180, complete: false };
	switchDuringRefresh = true;
	scrollRoot.scrollTop = 654;
	assert.equal(await handleSeasonProgress({ type: "season-progress", season: "S26", account: "public-ref", scanned: 500, complete: true }, 50_000), false);
	assert.equal(scrollRoot.scrollTop, 0, "页签切换后不得恢复旧页签的滚动位置");
});

test("live updates target explicit state slices and clear client state on disconnect", () => {
	assert.match(appScript, /const LIVE_UPDATE_STATE_SLICES = Object\.freeze\(\{/);
	for (const [eventType, slice] of [
		["account-updated", "account"], ["snapshot-updated", "collection"],
		["connection-state", "status"], ["season-progress", "overview-season"],
		["historical-ranks", "overview-ranks"],
	]) {
		assert.match(appScript, new RegExp(`"${eventType}": \\[[^\\]]*"${slice}"`));
	}
	assert.match(functionSource(appScript, "setupLiveUpdates"), /if \(!slices\.length\) return;/);
	assert.match(functionSource(appScript, "setupLiveUpdates"), /source\.onerror = \(\) =>/);
	const clearSource = functionSource(appScript, "clearDisconnectedClientState");
	for (const reset of ["state.account = null", "state.history = []", "state.pools = []", "state.poolItems = []", "state.items = []"]) {
		assert.match(clearSource, new RegExp(reset.replace(/[.[\]]/g, "\\$&")));
	}
	assert.match(gameplayScript, /deep-legends:live-disconnected/);
});

test("structured data-source attempts are visible while cancellations stay silent", () => {
	const { capabilityAttemptSummary } = compileFunctions(gameplayScript, ["capabilityAttemptSummary"]);
	const summary = capabilityAttemptSummary({ attempts: [
		{ source: "lcu", outcome: "failed", message: "客户端未连接" },
		{ source: "sgp", outcome: "success" },
	] });
	assert.equal(summary, "本机客户端失败：客户端未连接；腾讯 SGP成功");
	assert.match(functionSource(gameplayScript, "renderCapabilitySettings"), /filter\(\(item\) => item\?\.state !== "canceled"\)/);
	assert.match(functionSource(gameplayScript, "renderCapabilitySettings"), /capabilityAttemptSummary\(item\)/);
});

test("historical rank increments refresh only the matching active overview", async () => {
	const tab = { key: "kr", playerRef: "public-ref", data: { player: { playerRef: "public-ref" } } };
	const state = { section: "overview" };
	const scrollRoot = { scrollTop: 222, isConnected: true };
	const calls = [];
	const { handleOverviewIncremental } = compileFunctions(gameplayScript, ["handleOverviewIncremental"], {
		state,
		activeTab: () => tab,
		overviewGroupForSection: () => "players",
		overviewSectionForGroup: () => "overview",
		document: { getElementById: () => scrollRoot },
		loadOverview: async () => assert.fail("historical ranks must not request history again"),
		rerenderTab: (...args) => calls.push(args),
		requestAnimationFrame: (callback) => callback(),
	});
	assert.equal(await handleOverviewIncremental({ type: "historical-ranks", account: "another-player" }), false);
	assert.equal(calls.length, 0);
	assert.equal(await handleOverviewIncremental({ type: "historical-ranks", account: "public-ref", historicalRanks: [{season:"2025",tier:"MASTER"}] }), true);
	assert.equal(calls[0][0], tab);
	assert.equal(tab.data.historicalRanks[0].season, "2025");
	assert.equal(scrollRoot.scrollTop, 222);
});

test("summoner banner and recent ranked summary expose verified profile highlights", () => {
	assert.match(gameplayScript, /function highestCurrentRank\(ranks\)/);
	assert.match(gameplayScript, /function renderSummonerHighlights\(ranks, masteries, status\)/);
	assert.match(gameplayScript, /player\.backgroundSource && player\.backgroundPath/);
	assert.match(gameplayScript, /api\/champion-asset\?source=\$\{encodeURIComponent\(player\.backgroundSource\)\}&path=\$\{encodeURIComponent\(player\.backgroundPath\)\}/);
	// 客户端个人主页背景只有本机登录的国服账号才有；韩服和“看别人资料”必须走
	// 最高熟练度英雄原画兜底，两条链路都要挂上，少一条那边就是纯色卡片。
	assert.match(gameplayBackend, /func applyMasteryBackgroundFallback\(player \*gameplayPlayer, masteries \[\]gameplayMastery\)/);
	assert.match(gameplayBackend, /a\.completeOverviewBackground\(&response, playerRef\)/);
	assert.match(riotBackend, /a\.completeOverviewBackground\(&response, puuid\)/);
	assert.match(gameplayScript, /summoner-mastery-portrait/);
	assert.match(gameplayScript, /crest-and-banner-mastery-\$\{masteryLevel\}\.png/);
	assert.match(gameplayScript, /Math\.min\(10, Math\.floor\(Number\(mastery\?\.championLevel\)/);
	// 徽章图官方只导出 0~10 级，11 级以上复用 10 级外观，所以真实等级必须另外写出来——
	// 别把 masteryLevel（已截断）当成显示值。等级写在说明行里，不再往徽章上盖 Lv 牌。
	assert.match(gameplayScript, /Lv\.\$\{number\(mastery\.championLevel\)\}/);
	assert.doesNotMatch(gameplayStyles, /\.summoner-mastery-portrait > i\s*\{/);
	assert.match(gameplayScript, /summoner-highlight-rank/);
	assert.match(gameplayStyles, /\.summoner-strip-art\s*\{[^}]*inset:\s*-1px[^}]*object-fit:\s*cover[^}]*object-position:\s*50% 17%/s);
	assert.doesNotMatch(gameplayStyles, /\.summoner-strip-art\s*\{[^}]*transform:/s);
	assert.match(gameplayStyles, /\.game-icon\.is-summoner-avatar\s*\{[^}]*width:\s*72px/s);
	assert.match(gameplayStyles, /\.summoner-strip-copy h2\s*\{[^}]*font-size:\s*24px/s);
	// 熟练度徽章不许再糊住英雄头像：头像必须压在徽章之上。
	const masteryIconLayer = cssNumber(gameplayStyles, ".game-icon.is-summoner-mastery-icon", "z-index");
	const masteryCrestLayer = cssNumber(gameplayStyles, ".summoner-mastery-crest", "z-index");
	assert.ok(masteryIconLayer > masteryCrestLayer, `英雄头像 z-index(${masteryIconLayer}) 必须高于熟练度徽章(${masteryCrestLayer})，否则徽章会盖住脸`);
	// 头像是认英雄的主角，徽章只是等级注脚：头像必须明显比徽章大，且不能无节制变大
	// 把召唤师条重新撑高回改版前的 168px。
	const avatarSize = cssNumber(gameplayStyles, ".game-icon.is-summoner-mastery-icon", "width");
	const crestSize = cssNumber(gameplayStyles, ".summoner-mastery-crest", "width");
	assert.ok(avatarSize > crestSize, `熟练度头像(${avatarSize}px) 必须比徽章(${crestSize}px) 更大`);
	assert.ok(avatarSize >= 48, `头像不能缩回改版前的 40px，当前 ${avatarSize}px`);
	// 徽章尺寸的合理区间：小于 40px 旗面纹样糊成一团，大于 48px 会把召唤师条
	// 重新撑高（这条子曾经反复被用户点名"太小不协调" / "太高了"）。
	assert.ok(crestSize >= 40, `徽章太小旗面纹样看不清，当前 ${crestSize}px`);
	assert.ok(crestSize <= 48, `徽章过大会把召唤师条重新撑高，当前 ${crestSize}px`);
	// 熟练度整块的高度必须恰好卡在内容实际占据的高度（徽章 top + 徽章高度），
	// 不能留多余空白。2026-08-25 用户点出过"头像徽标离上面太近，上下间距不一样"——
	// 根因就是容器比内容高出一截、多余空白全沉在底部，subgrid 行高被撑到容器高度后
	// 头像贴着标签，称号却隔着"空白+行间距"两段。容器高度=内容高度时上下两侧
	// 天然只吃 subgrid 的行间距，自动对称，不用另外加 margin 去凑。
	const crestTop = cssNumber(gameplayStyles, ".summoner-mastery-crest", "top");
	const crestHeight = cssNumber(gameplayStyles, ".summoner-mastery-crest", "height");
	const portraitHeight = cssNumber(gameplayStyles, ".summoner-mastery-portrait", "height");
	assert.equal(portraitHeight, crestTop + crestHeight, `容器高度(${portraitHeight}px) 必须等于徽章底边(${crestTop + crestHeight}px)，否则底部会有留白，上下间距就不对称`);
	assert.ok(portraitHeight <= 92, "熟练度整块高度不能失控变回 98px 那版");
	// .summoner-highlight 的 min-height 已经删掉了：去掉标签行之后行数由 subgrid
	// 的三条行轨决定，再写死一个最小高度只会重新引入"容器比内容高"的留白问题。
	assert.doesNotMatch(cssBlockAfter(gameplayStyles, ".summoner-highlight "), /min-height/, "别再给高亮列写死 min-height，行高交给 subgrid");
	assert.ok(cssNumber(gameplayStyles, ".summoner-strip ", "min-height") <= 152, "召唤师条整体高度不能回到改版前的 168px");
	// 头像/徽章都比第一版更大了，只能靠收紧召唤师条自身的竖向内边距把总高度摁住，
	// 否则实际渲染高度会重新逼近改版前被否掉的 168px。
	assert.ok(cssNumber(gameplayStyles, ".summoner-strip {", "padding") <= 14, "召唤师条竖向内边距要收紧，否则整体高度会顶回 168px 附近");
	// 段位徽章的上下界改由上面那条 subgrid 对齐测试统一把关（用户后来明确要求
	// "段位图标可以放大点，不要违和"，68px 那条上限已经作废）。这里只保留
	// 「徽章图和它的容器同尺寸」这个不变量，避免图被容器裁掉或缩得对不上。
	assert.equal(
		cssNumber(gameplayStyles, ".summoner-highlight-rank .rank-crest-icon", "width"),
		cssNumber(gameplayStyles, ".summoner-highlight-rank ", "width"),
		"段位徽章图必须和容器同宽，否则会被裁切或留白",
	);
	assert.match(gameplayStyles, /@container gameplay-page \(max-width: 520px\)[\s\S]+\.summoner-strip-highlights\s*\{[^}]*grid-column:\s*1\/-1/s);
	// 换行到第二行时必须靠左，和上面的头像同一条左边缘——不能再甩到右边。
	const wrapped = cssBlockAfter(gameplayStyles, "@container gameplay-page (max-width: 520px)");
	assert.match(wrapped, /\.summoner-strip-highlights\s*\{[^}]*justify-content:\s*flex-start/s);
	assert.doesNotMatch(wrapped, /\.summoner-strip-highlights\s*\{[^}]*justify-content:\s*flex-end/s);
	assert.match(wrapped, /\.summoner-highlight\s*\{[^}]*min-width:\s*0/s);
	assert.match(gameplayScript, /\["recent-ranked", renderRecentRanked\(recentQueue\.recentRanked, recentQueue\.queueId/);
	assert.match(gameplayScript, /function renderRecentRanked\(stats, queueId = 0, queueSwitcher = ""\)/);
	assert.match(gameplayScript, /data-win-rate="\$\{Number\(stats\.winRate\) \|\| 0\}"/);
	assert.match(gameplayScript, /killParticipationGames/);
	assert.match(gameplayScript, /平均 K \/ D \/ A/);
	assert.match(gameplayScript, /位置胜率/);
	assert.match(gameplayScript, /node\.style\.setProperty\("--recent-win-rate", percent\(node\.dataset\.winRate\)\)/);
	assert.doesNotMatch(gameplayScript, /recent-ranked-ring[^>]+style=/);
	assert.match(gameplayStyles, /\.recent-ranked-ring\s*\{[^}]*conic-gradient/s);
	// 圆环必须永远和「平均 K/D/A」同一行；只有「KDA」+「击杀参与率」这一对
	// 打包进 .recent-ranked-metrics-tail，宽度不够时才整体掉到下一行——
	// 不能像旧版那样把整块指标（含平均 K/D/A）一起甩到圆环下面，那样太空。
	assert.match(gameplayScript, /<dl class="recent-ranked-metrics">/);
	assert.match(gameplayScript, /<div class="recent-ranked-metric" data-tooltip="每场平均击杀 \/ 死亡 \/ 助攻"[^>]*><dt>平均 K \/ D \/ A<\/dt>/);
	assert.match(gameplayScript, /<div class="recent-ranked-metrics-tail">/);
	assert.match(gameplayScript, /<div class="recent-ranked-metric" data-tooltip="总击杀与总助攻之和除以总死亡数"[^>]*><dt>KDA<\/dt>/);
	assert.match(gameplayScript, /<div class="recent-ranked-metric" data-tooltip="\$\{escapeHTML\(participationTooltip\)\}"[^>]*><dt>击杀参与率<\/dt>/);
	// 光测"字符串都出现过"挡不住把平均K/D/A挪进 tail 里（三列又变回一个整体，
	// 圆环又会被一起甩下去）——必须锁死嵌套顺序：<dl> 开标签后紧跟的必须是
	// 平均K/D/A 这个独立 div，而不是 tail 的开标签；tail 内部的第一个子元素
	// 必须是 KDA。dotall 让 .*? 能跨过 dd 里的插值内容。
	assert.match(
		gameplayScript,
		/<dl class="recent-ranked-metrics">\s*<div class="recent-ranked-metric" data-tooltip="每场平均击杀[^"]*"[^>]*>.*?<\/div>\s*<div class="recent-ranked-metrics-tail">\s*<div class="recent-ranked-metric" data-tooltip="总击杀[^"]*"[^>]*><dt>KDA<\/dt>/s,
		"平均K/D/A 必须是 <dl> 的直接子元素、排在 tail 之前；tail 里第一项必须是 KDA——顺序或嵌套错了就退回旧版三列一起换行",
	);
	// 「平均 K/D/A」必须是 .recent-ranked-metrics 的直接第一个子元素，且 KDA/
	// 击杀参与率必须包在 tail 里而不是和平均 K/D/A 平级——否则外层换行算法就
	// 没法把 tail 当成不可拆分的原子单元，会退回旧版"三列各自单独换行"的行为。
	assert.match(gameplayStyles, /\.recent-ranked-metrics\s*\{[^}]*display:\s*flex[^}]*flex-wrap:\s*wrap/s);
	// grow 但不 shrink：宽度富余时（比如生涯统计弹窗两栏平分 960px，圆环+三列
	// 吃不满整行）让「平均K/D/A」和 tail 各自撑开分隔线间距，不再挤成一坨；
	// shrink 必须钉死在 0，宽度紧张时不能被压缩产生截断，跟换行前的行为一致。
	assert.match(gameplayStyles, /\.recent-ranked-metrics-tail\s*\{[^}]*display:\s*flex[^}]*flex:\s*1 0 auto/s);
	assert.match(gameplayStyles, /\.recent-ranked-metric\s*\{[^}]*flex:\s*1 0 auto/s);
	assert.doesNotMatch(gameplayStyles, /\.recent-ranked-metrics-tail\s*\{[^}]*flex:\s*0 0 auto/s);
	assert.doesNotMatch(gameplayStyles, /\.recent-ranked-metric\s*\{[^}]*flex:\s*0 0 auto/s);
	assert.match(gameplayStyles, /\.recent-ranked-metrics > \.recent-ranked-metric:first-child\s*\{[^}]*border-left:\s*0/s);
	// 圆环不能再被塞进外层的换行组：.recent-ranked-content 本身不设
	// flex-wrap，换行只发生在 .recent-ranked-metrics 内部。
	assert.match(gameplayStyles, /\.recent-ranked-content\s*\{[^}]*display:\s*flex(?![^}]*flex-wrap)/s);
	assert.match(gameplayStyles, /\.recent-ranked-content > \.recent-ranked-record\s*\{[^}]*flex:\s*0 0 88px/s);
	// 弹窗窄到 260px 时圆环也放不下三列了，退化成整体竖排（含 tail 内部）。
	const dialogNarrowest = cssBlockAfter(gameplayStyles, "@container career-dialog (max-width: 260px)");
	assert.match(dialogNarrowest, /\.recent-ranked-metrics,\s*\.recent-ranked-metrics-tail\s*\{[^}]*flex-direction:\s*column/s);
	assert.match(dialogNarrowest, /\.recent-ranked-metrics > \.recent-ranked-metric:first-child\s*\{[^}]*border-top:\s*0/s);
	// 真机 Playwright 测出过：260px 时 grid-template-columns 是
	// minmax(0,1fr) auto，auto 那列（数值）先按内容定宽，会把 1fr 的标签列
	// 挤到比"平均 K / D / A"实际所需窄，配合 white-space: nowrap 直接截断成
	// "平均 K / …"。这个宽度已经挤不出"标签+数值同行都不截断"的空间了，
	// 所以放开标签换行（两行）来避免截断，别把这条 dt 覆盖规则删掉。
	assert.match(dialogNarrowest, /\.recent-ranked-metric dt\s*\{[^}]*white-space:\s*normal/s);
	assert.doesNotMatch(dialogNarrowest, /\.recent-ranked-metric dt\s*\{[^}]*white-space:\s*nowrap/s);
	assert.match(demoScript, /backgroundPath:\s*"\/cdn\/img\/champion\/splash\/Camille_0\.jpg"/);
	assert.match(demoScript, /recentRanked:\s*\{/);
});

// 两条 shape 探针只需为当前进程各留一份原始队列样本；完整字段反复 dump 会在
// 一分钟内冲掉真正需要的诊断事件。
test("ranked payload shape probes are one-shot and retain raw win-loss samples", () => {
	assert.match(gameplayBackend, /"event": "lcu_ranked_stats_shape"/);
	assert.match(sgpBackend, /"event": "sgp_ranked_stats_shape"/);
	assert.match(gameplayBackend, /rankedLCUShapeOnce\.Do\(/);
	assert.match(sgpBackend, /rankedShapeOnce\.Do\(/);
	assert.match(gameplayBackend, /"sample_queue_values": diagnosticRankedQueueSamples\(/);
	assert.match(sgpBackend, /"sample_queue_values": diagnosticRankedQueueSamples\(/);
	assert.doesNotMatch(gameplayBackend + sgpBackend, /"season_fields"|func diagnosticSeasonFields/);
	// queues 数组与 queueMap 对象两种形状都要覆盖，否则日志会假空。
	const entries = goFunctionSource(sgpBackend, "rankedQueueEntries");
	assert.match(entries, /object\["queues"\]/);
	assert.match(entries, /object\["queueMap"\]/);
});

// 客户端英雄方形头像源图自带装饰性暗角（实测 CommunityDragon 镜像的 v1/champion-icons/{id}.png：
// 抽查 5 个不同英雄，人物内容只占画布中心约 72%，四周一圈渐暗），边到边显示会露出黑边。
// 只有英雄头像有这个问题，召唤师头像/装备/符文/召唤师技能的源图是边到边的，不能被误伤放大。
// 英雄方形头像确实带一圈装饰性暗角（源图问题，不是 CSS bug），但这些图标本身就是贴近
// 头肩的紧凑构图、人物普遍不在画面正中，圆形裁切本身就会先吃掉四个角（暗角最重的地方）。
// 2026-08-25 试过放大 1.4 倍整体裁切去暗角，真机截图显示圆形裁切会连着下巴、额头一起
// 吃掉，脸就只剩眼睛和头发——完整看到脸比彻底去掉暗角更重要，所以维持原图比例。
// 这条测试钉住「不要重新引入这个放大裁切」，别人下次看到暗角又想放大时先看这条注释。
// 英雄方形头像源图自带一圈渐暗的装饰性暗角（CommunityDragon 的 v1/champion-icons；
// DDragon 方形图与 game assets 的 _square/_circle 实测是同一张底图，换源没用）。
// 实测径向亮度：r≤0.42w 稳定在 ~102，r=0.46w 掉到 69，r=0.49w 掉到 42——暗角从
// 0.42w 起。圆形裁切落在 0.42w 以内即可完全避开，对应放大 1/(0.42*2)≈1.19，取 1.18。
// 这条测试同时防两个方向的回归：
//   往大调 → 1.4 那版裁掉 29% 画面，圆形裁切连下巴一起吃掉，被用户否过；
//   往小调/删掉 → 暗角黑边直接回来，用户也否过（"黑边又改回来了"）。
// 两边都踩过坑，所以上下界都要钉住。
test("champion square icons crop out the baked-in vignette without eating the face", () => {
	assert.match(gameplayScript, /kind === "champion" \? "is-champion-art" : ""/);
	const scale = parseFloat(/\.game-icon\.is-champion-art img \{[^}]*transform:\s*scale\(([\d.]+)\)/.exec(gameplayStyles)?.[1] ?? "NaN");
	assert.ok(scale >= 1.12, `放大不足会露出源图暗角黑边，当前 ${scale}`);
	assert.ok(scale <= 1.25, `放大过头圆形裁切会吃掉下巴（1.4 那版就是这么被否的），当前 ${scale}`);
});


// 熟练度与最高段位两列必须逐行对齐（图像行、名称行、数值行各共用一条行轨）。
// 用 subgrid 实现：两列图像本来就不一样高（熟练度头像+徽章 vs 段位徽章），
// 写死高度会把其中一边撑变形；而 display: contents 会让 figure 不生成盒子，
// 熟练度那块的 data-tooltip 就失去 hover 目标了。两条歧路都别再走。
// 行数是 3 不是 4：顶上那行「最高熟练度 / 最高段位·单排双排」标签已经去掉了，
// 召唤师条高度是稀缺资源，图形本身已经能说明这两块的身份。
test("mastery and peak rank columns share row tracks so their labels, names and values line up", () => {
	const wrap = cssBlockAfter(gameplayStyles, ".summoner-strip-highlights ");
	assert.match(wrap, /display:\s*grid/);
	assert.match(wrap, /grid-template-rows:\s*auto auto auto[^a-z]/);
	assert.doesNotMatch(wrap, /display:\s*flex/, "改回 flex 就没有共享行轨了，两列会各自排各自的");
	assert.doesNotMatch(html + gameplayScript, /summoner-highlight-label/, "标签行已去掉，残留标记说明没删干净");
	const column = cssBlockAfter(gameplayStyles, ".summoner-highlight ");
	assert.match(column, /grid-template-rows:\s*subgrid/);
	assert.match(column, /grid-row:\s*span 3/);
	assert.doesNotMatch(column, /display:\s*contents/, "display: contents 会让 figure 不生成盒子，熟练度的 tooltip 就没了 hover 目标");
	const caption = cssBlockAfter(gameplayStyles, ".summoner-highlight figcaption");
	assert.match(caption, /grid-template-rows:\s*subgrid/);
	assert.match(caption, /grid-row:\s*span 2/);
	// 行间距（gap 的第一个值，行方向）不能缩回 2px——2026-08-25 用户反馈过
	// "熟练度头像徽标整体再往下去一点点"，根因是标签→图像只隔着这一份行间距，
	// 从 2px 抬到 4px 后头像才不会贴着标签，同时因为上下两侧共用同一份行间距，
	// 对称性（上一条测试锁的容器高度=内容高度）依然成立，不用额外加 margin。
	assert.ok(cssNumber(gameplayStyles, ".summoner-strip-highlights ", "gap") >= 4, "行间距不能缩回让头像贴着标签的 2px");
	// 段位徽章放大后仍要和熟练度那块体量相当，不能大到违和。2026-08-25 用户又反馈
	// 一次"段位图标再放大一点"（78→84px），下限跟着抬高，防止再缩回去。
	const crest = cssNumber(gameplayStyles, ".summoner-highlight-rank ", "width");
	assert.ok(crest >= 74, `段位徽章不能缩回用户反馈过太小的尺寸，当前 ${crest}px`);
	assert.ok(crest <= 92, `段位徽章不能大过熟练度整块的体量，当前 ${crest}px`);
});

// 演示数据必须如实反映真实生产环境会用到的英雄头像资源形状（CommunityDragon 镜像的
// rcp-be-lol-game-data 方形头像），而不是英雄页选用的 gtimg 头像特写图——2026-08-25
// 曾经因为这里接错源，排查头像裁切问题时把只在演示数据里才出现的构图差异误判成产品缺陷。
test("demo data routes champion icons through the same asset shape production actually serves", () => {
	assert.match(demoScript, /champion-icons\\\/\(\\d\+\)\\\.png\$/);
	assert.match(demoScript, /source=communitydragon&path=\$\{encodeURIComponent\(`\/latest\/plugins\/rcp-be-lol-game-data\/global\/default\/v1\/champion-icons\/\$\{match\[1\]\}\.png`\)\}/);
	assert.doesNotMatch(demoScript, /source=gtimg&path=\$\{encodeURIComponent\(`\/images\/lol\/act\/img\/champion/);
});


// 即使整季没有排位，三个可切换模块也必须保留：只有这样玩家选择单双排或
// 灵活组排后，才能看到该模式自己的完成状态，不能退回“二选一”的合并文案。
test("ranked queue cards stay visible so empty-season copy follows the selected mode", () => {
	assert.doesNotMatch(gameplayScript, /function hasRankedFootprint\(data\)/);
	assert.doesNotMatch(gameplayScript, /function renderNoRankedNotice\(\)/);
	assert.doesNotMatch(gameplayScript, /const rankedOnly =/);
	for (const [key, renderer] of [
		["recent-ranked", "renderRecentRanked"],
		["ability", "renderAbility"],
		["positions", "renderPositionStats"],
	]) {
		assert.match(gameplayScript, new RegExp(`\\["${key}", ${renderer}\\(`), `${key} 必须始终保留模式切换卡片`);
	}
	assert.doesNotMatch(gameplayScript, /没有单双排或灵活组排记录/);
});

test("expanded sidebar stays compact without the old navigation subtitle", () => {
	assert.match(appStyles, /--sidebar-width:\s*226px/);
	assert.match(appStyles, /:root\[data-sidebar="collapsed"\]\s*\{\s*--sidebar-width:\s*70px/);
	assert.match(html, /<div class="sidebar-brand-copy"><strong>DEEP <span class="brand-accent">LEGENDS<\/span><\/strong><\/div>/);
	assert.doesNotMatch(html, /战绩 · 英雄 · 对局 · 收藏/);
	for (const label of ["总览", "英雄", "对局", "收藏", "设置"]) {
		assert.match(html, new RegExp(`class="[^"]*section-tab[^"]*"[^>]+aria-label="${label}"`));
		assert.match(html, new RegExp(`aria-label="${label}"[^>]+data-tooltip="${label}"[^>]+data-sidebar-tooltip`));
	}
	// 字标第六版：内嵌 Beaufort for LOL Bold（英雄联盟官方定制字体，Riot 委托 Shinn
	// Type/Monotype 基于 Beaufort 定制扩展，见 web/beaufort-for-lol-notice.txt）。
	// 前五版依次被否：①②③④在 Windows 自带字体里打转（全大写工业风、金属浮雕渐变、
	// 跟随 h1/h2 的系统无衬线、Constantia 衬线），最后评价是"太普通"；⑤内嵌 Cinzel
	// （OFL 罗马碑刻体）用户认可效果，但后来想要更贴近官方视觉，本版换成了英雄联盟
	// 本尊在用的这款字体。这条钉死三件事：字体真的被内嵌（有 @font-face 且指向随包的
	// woff2，不是 CDN）、字标用它、以及不要再退回纯系统字体栈。
	const fontFace = cssBlockAfter(appStyles, "@font-face");
	assert.match(fontFace, /font-family:\s*"Beaufort for LOL"/);
	assert.match(fontFace, /src:\s*url\("beaufort-for-lol-bold\.woff2"\)\s*format\("woff2"\)/);
	assert.doesNotMatch(fontFace, /https?:\/\//, "字体必须随包内嵌，不能从网上取——这个 app 是本地离线运行的");
	const brandBlock = cssBlockAfter(appStyles, ".sidebar-brand-copy strong ");
	assert.match(brandBlock, /white-space:\s*nowrap/);
	assert.match(brandBlock, /font-family:\s*"Beaufort for LOL"/);
	assert.doesNotMatch(brandBlock, /var\(--font\)/);
	assert.match(brandBlock, /color:\s*var\(--ink\)/);
	// 别再叠加前几版被否掉的渐变/浮雕/拉伸特效——Beaufort 本身的字重和线条已经够有
	// 辨识度，不需要额外特效，这条防的是回归到那些被否掉的方向。
	assert.doesNotMatch(appStyles, /\.sidebar-brand-copy strong\s*\{[^}]*(?:Copperplate|Palatino|Bahnschrift|"Segoe UI Black"|"Arial Black"|Impact)/s);
	assert.doesNotMatch(appStyles, /\.sidebar-brand-copy strong\s*\{[^}]*(?:scaleX|width:\s*152%|text-shadow|linear-gradient|background-clip)/s);
	// 折行版的第二行标记、渐变金字版的 @supports 块都必须彻底移除，留着就是没删干净。
	assert.doesNotMatch(html, /sidebar-brand-copy[^]*?<i>/);
	assert.doesNotMatch(appStyles, /\.sidebar-brand-copy strong > i\b/);
	assert.doesNotMatch(appStyles, /@supports \(\(-webkit-background-clip: text\)/);
	assert.ok(cssNumber(appStyles, ".sidebar-brand-copy strong ", "font-size") >= 17, "字标必须比旧版的 17px 更大或至少持平");
	// 正字距沿用 R21 定的选择（当时是给碑刻体 Cinzel 用的），换成 Beaufort 之后
	// 沿用同样的字距，和全站 h1/h2 的负字距依然是故意不同的——字标是标识不是标题。
	const letterSpacing = parseFloat(/letter-spacing:\s*(-?[\d.]+)em/.exec(brandBlock)?.[1] ?? "NaN");
	assert.ok(letterSpacing > 0, `字标字距要正值让字母舒展，当前 ${letterSpacing}`);
	// 强调词必须用主题色点题，且要和主标题颜色不同——否则整行同色就没有强调可言。
	assert.match(cssBlockAfter(appStyles, ".sidebar-brand-copy strong .brand-accent"), /color:\s*var\(--primary-strong\)/);
	assert.match(appScript, /document\.documentElement\.dataset\.sidebar === "collapsed"[\s\S]+matchMedia\("\(max-width: 820px\)"\)[\s\S]+next\?\.dataset\.sidebarTooltip !== undefined[\s\S]+sidebarAnchor && compactSidebar/);
});

test("match history surfaces SGP failures and uses an accessible custom mode menu", () => {
	assert.match(gameplayScript, /\["failed", "unsupported"\]\.includes\(matchDetailsCapability\?\.state\)/);
	assert.match(gameplayScript, /完整对局参与者读取不完整，部分场次只能显示接口实际返回的玩家/);
	assert.match(gameplayScript, /escapeHTML\(matchDetailsCapability\.detail/);
	assert.match(gameplayScript, /Number\(match\.queueId\) !== 0/);
	assert.match(gameplayScript, /CUSTOM_GAME/);
	assert.match(gameplayScript, /String\(match\.modeGroup \|\| ""\)\.toLowerCase\(\) !== "custom"/);
	assert.match(gameplayScript, /if \(filter === "all"\) return matches/);
	assert.match(gameplayScript, /const MORE_MODE_OPTIONS = \[\s*\["aram", "极地大乱斗", null\]/);
	assert.match(gameplayScript, /\["hextech-qualifier", "海克斯大乱斗 海选赛", null\]/);
	assert.match(gameplayScript, /const direct = \[\["all", "全部"\], \["solo", "单排\/双排"\], \["flex", "灵活组排"\], \["hextech-aram", "海克斯大乱斗"\], \["arena", "斗魂竞技场"\]\]/);
	assert.match(gameplayScript, /\["hextech-classic", "海克斯大乱斗 经典模式版", null\]/);
	assert.match(gameplayScript, /riotTab\(tab\) \? MORE_MODE_OPTIONS\.filter\(\(\[key\]\) => key !== "hextech-qualifier"\)/);
	assert.match(gameplayScript, /if \(key === "aram"\) return matches\.filter\(\(match\) => matchModeKind\(match\) === "aram"\)/);
	assert.match(gameplayScript, /if \(filter === "hextech-aram"\) return matches\.filter\(isOrdinaryHextechMatch\)/);
	assert.match(gameplayScript, /aria-haspopup="menu" aria-expanded="false" data-app-select-trigger/);
	assert.match(gameplayScript, /role="menuitemradio" aria-checked=/);
	assert.match(gameplayScript, /\["ArrowDown", "ArrowUp", "Home", "End"\]/);
	assert.match(gameplayScript, /event\.key === "Escape"[^\n]+closeAppSelect\(root, true\)/);
	assert.match(gameplayScript, /event\.target\.closest\("\.app-select"\)/);
	assert.match(appStyles, /\.app-select-menu\s*\{[^}]*background:\s*var\(--surface-strong\)/s);
	assert.match(appStyles, /\.app-select-menu button\[aria-checked="true"\]\s*\{[^}]*var\(--primary-strong\)[^}]*var\(--primary-soft\)/s);
});

test("player tabs use arrow scrolling, hide the native scrollbar, and deduplicate searches", () => {
	assert.match(html, /id="player-tabs-prev"[^>]+aria-controls="player-tabs"/);
	assert.match(html, /id="player-tabs-next"[^>]+aria-controls="player-tabs"/);
	assert.match(gameplayScript, /prev\?\.addEventListener\("click", \(\) => scrollTabs\(-1\)\)/);
	assert.match(gameplayScript, /next\?\.addEventListener\("click", \(\) => scrollTabs\(1\)\)/);
	assert.match(gameplayScript, /tabs\.scrollBy\(\{ left: direction \* distance/);
	assert.match(gameplayStyles, /\.player-tabs::-webkit-scrollbar\s*\{\s*display:\s*none/);
	assert.match(gameplayStyles, /\.player-tabs\s*\{[^}]*scrollbar-width:\s*none/s);
	assert.match(gameplayScript, /sameTabServerScope\(tab, region, serverId\)/);
	assert.match(gameplayScript, /sameRiotID\(tab, gameName, tagLine\)/);
	assert.match(gameplayScript, /function findExistingTab\(\{ playerRef/);
	assert.match(gameplayScript, /playerRefs:\s*new Set\(\)/);
	assert.match(gameplayScript, /tab\.playerRefs\?\.has\(normalizedRef\)/);
	assert.match(gameplayScript, /rememberTabPlayerRef\(tab, resolvedPlayerRef\)/);
	assert.match(gameplayScript, /const loaded = tab\?\.data\?\.player \|\| \{\}/);
	assert.match(gameplayScript, /loaded\.gameName \? loaded : null/);
	assert.match(gameplayScript, /if \(existing\) \{ applyPlayerTabContext\(existing, tabContext\); selectPlayerTab\(existing\.key\); return; \}/);
});

test("player tab avatars have no tooltip while overflowing names retain their own hint", () => {
  const dom = new JSDOM('<div id="tabs"></div>');
  try {
    const tabs = dom.window.document.getElementById("tabs");
    const state = { tabs: [{ key: "current", current: true, icon: 17, label: "测试玩家的完整名字" }], activeTabs: { players: "current" }, settings: {} };
    const functions = compileFunctions(gameplayScript, ["renderPlayerTabWorkspace", "assetIcon", "proxyAsset"], {
      state, document: dom.window.document, overviewWorkspace: () => ({ tabs }), connected: () => true, tabGroup: () => "players", riotTab: () => false,
      assetPath: (_kind, id) => `/lol-game-data/assets/v1/profile-icons/${id}.jpg`,
      escapeHTML: (value) => String(value ?? ""), prepareImages: () => {}, requestAnimationFrame: () => {},
    });
    functions.renderPlayerTabWorkspace("players");
    const avatar = tabs.querySelector(".game-icon img");
    assert.ok(avatar);
    assert.equal(avatar.closest("[data-tooltip], [title]"), null, "hover must not inherit the tab's name tooltip");
    assert.equal(avatar.alt, "", "adjacent player name already labels this tab");
    assert.doesNotMatch(tabs.innerHTML, /profile 17/);
    const name = tabs.querySelector(".player-tab-name");
    assert.equal(name.dataset.tooltip, state.tabs[0].label);
    assert.equal(name.dataset.tooltipOverflow, "self");
    assert.equal(tabs.querySelector(".player-tab").getAttribute("role"), "tab");
  } finally { dom.window.close(); }
});

test("player overlays add durable tabs and non-self tabs can be reordered", () => {
	assert.match(html, /id="player-overlay-add"[^>]*>\+ 添加到总览<\/button>/);
	assert.match(gameplayScript, /nodes\.playerOverlayAdd\?\.addEventListener\("click", addOverlayPlayerToTabs\)/);
	assert.match(gameplayScript, /data-player-tab-wrap="\$\{escapeHTML\(tab\.key\)\}" draggable="\$\{!tab\.current\}"/);
	assert.match(gameplayScript, /addEventListener\("dragstart"/);
	assert.match(gameplayScript, /movePlayerTab\(focusedKey, event\.key === "ArrowRight" \? 1 : -1\)/);
	assert.match(functionSource(gameplayScript, "persistPlayerTabOrder"), /writeSetting\("player-tab-order", JSON\.stringify\(state\.playerTabOrder\)\)/);
	assert.match(functionSource(gameplayScript, "readPlayerTabOrder"), /JSON\.parse\(readSetting\("player-tab-order", "\[\]"\)\)/);
	assert.match(gameplayStyles, /\.player-tab-wrap\[draggable="true"\][^}]*cursor:\s*grab/s);
	assert.match(gameplayStyles, /\.player-overlay-add\s*\{[^}]*margin-left:\s*auto/s);

	const current = { key: "current", current: true, identity: "" };
	const a = { key: "a", identity: "a" };
	const b = { key: "b", identity: "b" };
	const c = { key: "c", identity: "c" };
	const writes = [];
	const state = { tabs: [current, a, b, c], playerTabOrder: ["b", "a"], draggedPlayerTab: "" };
	const ordering = compileFunctions(gameplayScript, ["applySavedPlayerTabOrder", "persistPlayerTabOrder", "movePlayerTab", "dropPlayerTab"], {
		state,
		playerTabOrderIdentity: (tab) => tab.identity || "",
		tabGroup: (tab) => tab?.group === "pro" ? "pro" : tab?.region === "kr" ? "kr" : "players",
		writeSetting: (key, value) => writes.push([key, value]),
		renderPlayerTabs: () => {},
		requestAnimationFrame: (callback) => callback(),
		overviewWorkspace: () => ({ tabs: { querySelector: () => ({ focus() {} }) } }),
		CSS: { escape: (value) => value },
	});
	ordering.applySavedPlayerTabOrder();
	assert.deepEqual(state.tabs.map((tab) => tab.key), ["current", "b", "a", "c"]);
	assert.equal(ordering.movePlayerTab("a", -1), true);
	assert.deepEqual(state.tabs.map((tab) => tab.key), ["current", "a", "b", "c"]);
	assert.equal(ordering.movePlayerTab("a", -1), false, "本人页签前不能插入其他页签");
	assert.equal(ordering.dropPlayerTab("c", "current", false), true);
	assert.deepEqual(state.tabs.map((tab) => tab.key), ["current", "c", "a", "b"]);
	assert.ok(writes.some(([key, value]) => key === "player-tab-order" && value === '["c","a","b"]'));

	let existing = null;
	const opened = [];
	const synced = [];
	const overlayState = { overlay: [{ label: "目标玩家" }] };
	const { addOverlayPlayerToTabs } = compileFunctions(gameplayScript, ["addOverlayPlayerToTabs"], {
		state: overlayState,
		overlayPlayerIdentity: () => ({ playerRef: "public-ref", gameName: "", tagLine: "", region: "kr", serverId: "" }),
		findExistingTab: () => existing,
		syncOverlayAddButton: (entry) => synced.push(entry),
		openPlayer: (...args) => opened.push(args),
		openPlayerByRiotId: () => assert.fail("playerRef 存在时不应回退 Riot ID"),
		showToast: () => {},
	});
	addOverlayPlayerToTabs();
	assert.deepEqual(opened[0], ["public-ref", "目标玩家", "kr", ""]);
	existing = { key: "already-open" };
	addOverlayPlayerToTabs();
	assert.equal(opened.length, 1, "已存在的玩家不能重复添加");
	assert.equal(synced.length, 2);
});

test("damage analysis sorts by a real metric and shows full colored values", () => {
  assert.match(gameplayScript, /damageSorts\?\.get\(sortKey\(team\)\)/);
  assert.match(gameplayScript, /data-team="\$\{escapeHTML\(team\)\}"/);
  assert.match(gameplayScript, /\.sort\(\(left, right\) => \(Number\(right\[groupMetric\]\)/);
  assert.match(gameplayScript, /data-damage-sort="\$\{currentMetric\}"/);
	assert.match(gameplayScript, /data-bar-width="\$\{value > 0 \? Math\.max\(1, share\) : 0\}"/);
	assert.match(gameplayScript, /data-blue-share="\$\{share\}"/);
	assert.match(gameplayScript, /function applyRenderedMetricStyles\(container\)/);
	assert.doesNotMatch(gameplayScript, /<i style="width:/);
	assert.match(gameplayStyles, /\.damage-bar\s*\{[^}]*background:\s*color-mix/s);
	  assert.match(gameplayStyles, /\.match-damage-value\s*\{[^}]*var\(--danger\)/);
  assert.match(gameplayStyles, /\.match-stat-damage\s*>\s*b,\s*\.match-stat-taken > b\s*\{[^}]*display:\s*inline-flex/);
  assert.match(gameplayStyles, /\.match-stat\s*\{[^}]*minmax\(0,1fr\)/s);
  assert.match(gameplayStyles, /\.match-list\s*\{[^}]*overflow-anchor:\s*auto/s);
	assert.match(gameplayScript, /tab\.paginationStalls = upstreamAdditions > 0 \? 0/);
  assert.match(gameplayScript, /function appendOverviewMatches\(tab, additions\)/);
  assert.match(gameplayScript, /if \(append\) appendOverviewMatches\(tab, appendAdditions\);/);
});

test("team analysis compares both sides across every requested metric", () => {
	const metricSource = gameplayScript.slice(gameplayScript.indexOf("const TEAM_ANALYSIS_METRICS"), gameplayScript.indexOf("const TEAM_ANALYSIS_POSITIONS"));
	for (const [key, label] of [
		["damage", "伤害"], ["damageTaken", "承伤"], ["gold", "金钱"], ["cs", "补刀"], ["killParticipation", "击杀参与率"],
		["championLevel", "等级"], ["kills", "击杀数"], ["deaths", "死亡数"], ["assists", "助攻数"],
		["wardsPlaced", "插眼数"], ["wardsKilled", "排眼数"], ["controlWardsBought", "真眼购买数"],
	]) assert.match(metricSource, new RegExp(`key: "${key}", label: "${label}"`));
	assert.match(gameplayScript, /teamAnalysisMetrics:\s*new Map\(\)/);
	assert.match(gameplayScript, /class="team-analysis-metrics" role="tablist"/);
	assert.match(gameplayScript, /data-team-analysis-metric="\$\{metric\.key\}"/);
	assert.match(gameplayScript, /class="team-duel-row"/);
	assert.match(gameplayScript, /class="team-duel-player is-\$\{tone\}"/);
	assert.match(gameplayScript, /tab\.teamAnalysisMetrics\.set\(gameID, metric\)/);
	assert.match(gameplayStyles, /\.team-analysis-metrics\s*\{[^}]*overflow-x:\s*auto/s);
	assert.match(gameplayStyles, /\.team-analysis-metrics button\[aria-selected="true"\]\s*\{[^}]*var\(--primary-strong\)[^}]*var\(--primary-soft\)[^}]*var\(--primary\)/s);
	assert.match(gameplayStyles, /\.team-duel-row\s*\{[^}]*grid-template-columns:\s*26px minmax\(44px,64px\) minmax\(120px,1fr\) minmax\(44px,64px\) 26px/s);
	assert.match(gameplayBackend, /ControlWardsBought \*int `json:"controlWardsBought,omitempty"`/);
	assert.match(demoScript, /controlWardsBought:\s*\(participantId \* 3\) % 7/);

	const { teamAnalysisMetricValue, teamAnalysisMetricLabel, teamAnalysisMembers } = compileFunctions(
		gameplayScript,
		["teamAnalysisMetricValue", "teamAnalysisMetricLabel", "teamAnalysisMembers"],
		{ number: (value) => String(value), TEAM_ANALYSIS_POSITIONS: ["top", "jungle", "middle", "bottom", "utility"] },
	);
	assert.equal(teamAnalysisMetricValue({ kills: 2, assists: 5 }, { key: "killParticipation" }, 10), 70);
	assert.equal(teamAnalysisMetricValue({}, { key: "controlWardsBought", nullable: true }, 0), null);
	assert.equal(teamAnalysisMetricValue({ controlWardsBought: 0 }, { key: "controlWardsBought", nullable: true }, 0), 0);
	assert.equal(teamAnalysisMetricLabel(0, { percent: false }), "0");
	assert.deepEqual(teamAnalysisMembers([
		{ participantId: 3, position: "middle" }, { participantId: 1, position: "top" }, { participantId: 2, position: "jungle" },
	]).map((item) => item.participantId), [1, 2, 3]);
});

test("team analysis exposes only metrics supported by each map and mode", () => {
	const functions = compileFunctions(
		gameplayScript,
		["matchQueueLabel", "isHextechClassic", "isHextechQualifier", "isOrdinaryHextechMatch", "matchModeKind", "isSummonersRiftMatch", "isARAMRelatedMatch", "teamAnalysisMetricsFor"],
		{
			TEAM_ANALYSIS_METRICS: [
				{ key: "damage" }, { key: "damageTaken" }, { key: "gold" }, { key: "cs", unavailableInARAM: true },
				{ key: "killParticipation" }, { key: "championLevel" }, { key: "kills" }, { key: "deaths" }, { key: "assists" },
				{ key: "wardsPlaced", summonersRiftOnly: true }, { key: "wardsKilled", summonersRiftOnly: true },
				{ key: "controlWardsBought", summonersRiftOnly: true },
			],
			SUMMONERS_RIFT_QUEUE_IDS: [400, 420, 430, 440, 490, 700, 720, 820, 830, 840, 850, 860, 870, 880, 890, 900, 1900],
			state: { queueGroups: [] },
			queueDefinitionFor: () => null,
			queueModeGroup: (match) => String(match?.modeGroup || "").toLowerCase(),
		},
	);
	const keys = (match) => functions.teamAnalysisMetricsFor(match).map((metric) => metric.key);
	const rift = keys({ queueId: 420, modeGroup: "solo", gameMode: "CLASSIC", mapId: 11 });
	assert.deepEqual(rift.slice(0, 3), ["damage", "damageTaken", "gold"]);
	assert.ok(rift.includes("cs"));
	assert.deepEqual(rift.slice(-3), ["wardsPlaced", "wardsKilled", "controlWardsBought"]);
	for (const match of [
		{ queueId: 450, modeGroup: "aram", gameMode: "ARAM", mapId: 12 },
		{ queueId: 480, gameMode: "ARAM", mapId: 12 },
		{ queueId: 2300, modeGroup: "hextech-aram", gameMode: "KIWI", mapId: 12 },
		{ queueId: 2400, gameMode: "ARAM_MAYHEM_CLASSIC", mapId: 12 },
	]) {
		const available = keys(match);
		assert.ok(!available.includes("cs"));
		assert.ok(!available.some((key) => ["wardsPlaced", "wardsKilled", "controlWardsBought"].includes(key)));
	}
	const nexusBlitz = keys({ queueId: 1300, modeGroup: "nexus-blitz", gameMode: "NEXUSBLITZ", mapId: 21 });
	assert.ok(nexusBlitz.includes("cs"));
	assert.ok(!nexusBlitz.includes("wardsPlaced"));
	const urf = keys({ queueId: 900, modeGroup: "urf", gameMode: "URF", mapId: 11 });
	assert.ok(urf.includes("cs"));
	assert.ok(urf.includes("controlWardsBought"));
});

test("match tier hydration is asynchronous and isolated by stable region, server, player and container", () => {
  assert.match(gameplayScript, /function matchTierScope\(tab\)/);
	assert.match(gameplayScript, /const serverID = riotTab\(tab\) \? "kr" : \(tabServerID\(tab\) \|\| "current"\)/);
	assert.match(gameplayScript, /return `\$\{region\}:\$\{serverID\}:\$\{playerRef\}`/);
	assert.doesNotMatch(functionSource(gameplayScript, "matchTierScope"), /tab\?\.key|tab\.key/);
  assert.match(gameplayScript, /container\.dataset\.matchTierScope = tierScope/);
  assert.match(gameplayScript, /if \(!container \|\| container\.dataset\.matchTierScope !== scope\) return/);
  assert.match(gameplayScript, /container\.querySelectorAll\("\[data-match-tier\]"\)/);
  assert.match(gameplayScript, /region: "kr"[\s\S]+playerRef,[\s\S]+matches,/);
  assert.match(gameplayScript, /const candidate = result\?\.\[gameID\]/);
  assert.match(gameplayScript, /playerRefs: refs, serverId: tabServerID\(tab\)/);
  assert.doesNotMatch(gameplayScript, /if \(riotTab\(tab\) \|\| !connected\(\)\) return/);
  assert.doesNotMatch(gameplayScript, /document\.querySelectorAll\(`\[data-match-tier\]/);
  assert.match(gameplayScript, /tab\.overviewRequestToken === requestToken/);
  assert.match(gameplayScript, /if \(seen\.has\(gameID\)\) return false;\s*seen\.add\(gameID\)/);
});

test("top search and player navigation preserve the selected Chinese server", () => {
  for (const [serverID, label] of [["HN1", "艾欧尼亚"], ["HN10", "黑色玫瑰"], ["NJ100", "联盟一区"], ["GZ100", "联盟二区"], ["CQ100", "联盟三区"], ["TJ100", "联盟四区"], ["TJ101", "联盟五区"], ["BGP2", "峡谷之巅"], ["PBE", "体验服"]]) {
    assert.match(html, new RegExp(`data-region-option="cn" data-server-id="${serverID}"[^>]*><b>${label}</b>`));
  }
  assert.match(html, /id="player-search-cn-toggle"[^>]+aria-expanded="false"[^>]+aria-controls="player-search-cn-options"/);
  assert.match(html, /id="player-search-cn-options"[^>]+role="group"[^>]+hidden/);
  assert.match(html, /id="player-search-follow-client"[^>]+data-region-option="cn" data-server-id=""[^>]+disabled/);
  assert.match(html, /data-region-option="kr" data-server-id=""/);
  assert.doesNotMatch(html.replace(/<[^>]*>/g, " "), /\b(?:HN1|HN10|NJ100|GZ100|CQ100|TJ100|TJ101|BGP2|PBE)\b/);
  assert.match(appScript, /function searchServerID\(\)/);
  assert.match(appScript, /function visibleRegionMenuEntries\(\)/);
  assert.match(appScript, /!entry\.disabled && !entry\.closest\("\[hidden\]"\)/);
  assert.match(appScript, /preference\("search-server-id", ""\)/);
  assert.match(appScript, /window\.addEventListener\("deep-legends:status"[^\n]+updateSearchRegionStatus/);
  assert.match(appScript, /savePreference\("search-server-id"/);
  assert.match(appScript, /detail: \{ gameName, tagLine, region, serverId, source: "search" \}/);
  assert.match(html, /id="player-search-go"[^>]*>[\s\S]*?<path d="M5 12h14"\/><path d="m12 5 7 7-7 7"\/>/);
  assert.match(appStyles, /#player-search-go\s*\{[^}]*background:\s*var\(--primary\)/s);
  assert.match(appScript, /for \(const input of \[el\.playerSearchName, el\.playerSearchTag\]\)[\s\S]*event\.key === "Enter"[\s\S]*submitPlayerSearch\(\)/);
  assert.match(appScript, /playerSearchGo\.addEventListener\("click", submitPlayerSearch\)/);
  assert.match(appScript, /“跟随客户端”需要英雄联盟客户端正在运行/);
  assert.match(mainSource, /ServerID\s+string\s+`json:"serverId,omitempty"`/);
  assert.match(mainSource, /ServerName\s+string\s+`json:"serverName,omitempty"`/);
  assert.match(mainSource, /response\.ServerID = clientTencentServerID\(client\)/);
  assert.match(mainSource, /response\.ServerName = tencentServerName\(response\.ServerID\)/);
  assert.match(gameplayScript, /serverId: tab\.serverId \|\| ""/);
  assert.match(gameplayScript, /tab\.serverId = payload\.player\?\.serverId/);
  assert.match(gameplayScript, /sourceServerID = tabServerID\(tab\)/);
  assert.match(gameplayScript, /serverId: tabServerID\(tab\), playerRef:/);
  assert.match(gameplayScript, /function tabServerLabel\(tab\)/);
	  assert.doesNotMatch(functionSource(gameplayScript, "renderPlayerTabs"), /player-tab-region/);
	  assert.match(gameplayStyles, /\.region-chip\.is-kr/);
  assert.match(appStyles, /\.region-server-grid\s*\{[^}]*grid-template-columns:\s*repeat\(2/s);
  assert.match(appStyles, /\.region-menu-section-kr\s*\{[^}]*var\(--accent\)/s);
  assert.match(appStyles, /\.player-search-region > button\[data-region="kr"\][^{]*\{[^}]*var\(--accent\)/s);
  assert.match(appStyles, /\.region-menu\s*\{[^}]*max-height:\s*calc\(100dvh[^}]*overflow-y:\s*auto/s);
});

test("Chinese server merge guidance uses one scoped tooltip constant", () => {
  assert.match(html, /id="player-search-cn-info"[^>]+data-tooltip[^>]+data-tooltip-side="menu"[^>]+data-tooltip-size="compact"/);
  assert.match(appScript, /const CN_SERVER_MERGE_NOTE = \[[\s\S]*联盟一区：[\s\S]*联盟五区：[\s\S]*独立运营，未参与合并。/);
  assert.match(appScript, /playerSearchCnInfo\.dataset\.tooltip = CN_SERVER_MERGE_NOTE/);
  assert.match(appScript, /tooltip\.dataset\.size = next\.dataset\.tooltipSize \|\| ""/);
  assert.match(appStyles, /\.global-tooltip\[data-size="compact"\]\s*\{[^}]*max-width:\s*min\(280px/s);
  assert.match(appScript, /anchor\.closest\("\.region-menu"\)\?\.getBoundingClientRect\(\)/);
  assert.doesNotMatch(appScript, /腾讯已将部分大区合并为联盟大区/);
  assert.doesNotMatch(appScript, /整理自公开资料|以英雄联盟官网公告为准/);
});

test("cross-server ranked data is an explicit unsupported state", () => {
	assert.match(gameplayScript, /renderRanks\(data\.ranks \|\| \[\], data\.capabilities \|\| \[\], data\.historicalRanks \|\| \[\], data\.rankMilestones, data\.seasonStatsProgress, tab\)/);
  assert.match(gameplayScript, /rankedCapability\?\.state === "unsupported"/);
  assert.match(gameplayScript, /跨服暂不支持排位/);
  assert.match(gameplayScript, /当前客户端的排位接口只能读取登录服务器/);
  assert.match(gameplayScript, /class="rank-unavailable"/);
  assert.match(gameplayScript, /crossServerRankUnsupported \? \{ unsupported: true \}/);
  assert.match(gameplayScript, /跨服暂不支持排位，无法计算本场平均段位/);
  assert.doesNotMatch(gameplayScript, /rank-unavailable[^`]+data-gameplay-retry/);
});

test("privacy UI separates explicit and opt-in automatic writes", () => {
  assert.match(appScript, /客户端操作需主动点击；自动规则仅在开启后执行。/);
  for (const flag of ["localOnly", "requiresPassword", "uploadsData"]) assert.ok(appScript.includes(`data.${flag}`));
  assert.doesNotMatch(appScript, /group\("明确点击后写入客户端"/);
});

test("live recommendations render and select every returned option", () => {
  assert.match(gameplayScript, /const opggItems = Array\.isArray\(runes\.opgg\) \? runes\.opgg :/);
	assert.match(gameplayScript, /const LIVE_CORE_OPTION_LIMIT = 5/);
	assert.match(gameplayScript, /const coreOptions = inUpstreamOrder\(build\.coreOptions \|\| \[\], LIVE_CORE_OPTION_LIMIT\)/);
	assert.doesNotMatch(gameplayScript, /LIVE_CORE_OPTION_PREVIEW_LIMIT|data-live-build-expand|liveBuildExpanded/);
	assert.match(gameplayScript, /const bootOptions = inUpstreamOrder\([^\n]+, 3\)/);
  // 「后期备选」已整条移除（前端渲染、装备方案分组、后端 LateItems/LastItems 链路）。
  assert.doesNotMatch(gameplayScript, /lateOptions|后期备选/);
  assert.match(gameplayScript, /const prismOptions = inUpstreamOrder\(build\.prismOptions \|\| \[\], 10\)/);
  assert.doesNotMatch(gameplayScript, /build\.itemRoutes|const itemRoutes/);
  assert.match(gameplayScript, /function buildItemSetPayload\(build, self, data\)/);
  assert.match(gameplayScript, /\/api\/gameplay\/item-sets\/apply/);
  assert.match(gameplayScript, /data-apply-item-set/);
  assert.match(gameplayScript, /liveRecommendations:\s*new Map\(\)/);
  assert.match(gameplayScript, /function ensureLiveRecommendations\(data\)/);
  for (const reason of ["no-target", "has-payload", "cached", "in-flight", "backoff"]) assert.match(functionSource(gameplayScript, "ensureLiveRecommendations"), new RegExp(`"${reason}"`));
  assert.match(functionSource(gameplayScript, "recordLiveRecommendationSkip"), /live_recommendations_skip/);
  assert.match(functionSource(gameplayScript, "recordLiveRecommendationSkip"), /console\.warn/);
  assert.match(gameplayScript, /liveRecommendationFlights:\s*new Map\(\)/);
  assert.match(gameplayScript, /function liveRecommendationFlightActive\(key/);
  assert.match(gameplayScript, /hasUsableLiveRecommendations\(response\?\.recommendations\)/);
  assert.match(gameplayScript, /\/api\/gameplay\/recommendations\?/);
  assert.match(mainSource, /GET \/api\/gameplay\/specialist-runes/);
  assert.match(gameplayScript, /specialistRunes:\s*new Map\(\)/);
	assert.match(gameplayScript, /specialistRuneFlights:\s*new Map\(\)/);
  assert.match(gameplayScript, /function ensureSpecialistRunes\(data\)/);
  assert.match(gameplayScript, /\/api\/gameplay\/specialist-runes\?/);
  assert.match(gameplayScript, /ensureLiveRecommendations\(state\.live\);\s*ensureSpecialistRunes\(state\.live\);/);
	assert.match(functionSource(gameplayScript, "ensureLiveRecommendations"), /const recommendations = response\.recommendations;[\s\S]*?state\.liveRecommendations\.set\(targetKey, recommendations\);[\s\S]{0,1200}ensureSpecialistRunes\(state\.live\);/);
  assert.match(gameplayScript, /Array\.isArray\(response\?\.runes\)/);
  assert.match(gameplayScript, /(?:liveRecommendation|specialistRequest)Target\(state\.live\)\?\.key === target(?:\.key|Key)/);
  assert.match(specialistSource, /specialistRuneCacheTTL\s*=\s*6 \* time\.Hour/);
	assert.match(specialistSource, /specialistRuneRequestBudget\s*=\s*specialistRunePlayerLimit \* \(2 \+ specialistRuneMatchScanMax\)/);
	assert.match(specialistSource, /specialistRuneMatchScanMax\s*=\s*10/);
	assert.match(specialistSource, /specialistAccountTimeout\s*=\s*5 \* time\.Second/);
	assert.match(specialistSource, /specialistMatchIDsTimeout\s*=\s*5 \* time\.Second/);
	assert.match(specialistSource, /specialistMatchDetailTimeout\s*=\s*4 \* time\.Second/);
	assert.match(specialistSource, /"event": "specialist_runes_handler"/);
	assert.match(specialistSource, /recordHandler\("skipped", "riot-key-missing"\)/);
  assert.doesNotMatch(gameplayScript, /runes\.opgg\.slice\(0,\s*2\)/);
  assert.doesNotMatch(gameplayScript, /runes\.specialists[^\n]+slice\(0,\s*2\)/);
  assert.doesNotMatch(gameplayScript, /(?:spellOptions|starterOptions|bootOptions|itemRoutes)\.slice\(/);
});

test("current champion endpoint keeps live recommendations usable without a roster identity", () => {
  const state = { livePositionOverride: new Map() };
  const { liveRecommendationTarget } = compileFunctions(gameplayScript, ["liveRecommendationChampionId", "liveRecommendationTarget"], {
    state,
    USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS: false,
    isARAMRelatedMatch: () => false,
    liveAugmentRecommendationSource: () => "arena",
	  liveRecommendationTier: () => "diamond",
  });
  const target = liveRecommendationTarget({
    players: [], currentChampionId: 64, queueId: 1750, gameMode: "CHERRY", mapId: 30, gameId: 99,
  });
  assert.equal(target?.championId, 64);
  assert.equal(target?.position, "other");
  assert.equal(target?.self, undefined);
  assert.equal(target?.tier, "diamond");
  assert.equal(target?.key, "64:other:CHERRY:30:diamond:none");
  assert.equal(liveRecommendationTarget({ players: [], currentChampionId: 0 }), null);
});

test("champion and live recommendations share the persisted tier", () => {
	assert.match(script, /tier: normalizeTier\(readSetting\("champion-tier", "emerald_plus"\)\)/);
	assert.match(script, /writeSetting\("champion-tier", tier\)/);
	assert.match(functionSource(gameplayScript, "liveRecommendationTier"), /readSetting\("champion-tier", "emerald_plus"\)/);
	assert.match(functionSource(gameplayScript, "liveRecommendationTarget"), /key: `\$\{championId\}:\$\{position\}:\$\{gameMode\}:\$\{mapId\}:\$\{tier\}:\$\{spellKey\}`/);
	assert.match(goFunctionSource(gameplayBackend, "gameplayRecommendationTier"), /if !spec\.UsesTier[\s\S]*championCounterFallbackTier[\s\S]*allowedChampionTiers/);
});

test("live recommendation requests recover from stale flights and empty payloads", async () => {
  const target = { key: "13:middle:CLASSIC:11:diamond:4-12", championId: 13, position: "middle", queueId: 420, gameMode: "CLASSIC", mapId: 11, tier: "diamond", gameId: 77, self: { spell1Id: 4, spell2Id: 12 } };
  let now = 100_001;
  let apiCalls = 0;
  let diagnosticCalls = 0;
	let lastAPIPath = "";
  const state = {
    live: {}, liveRecommendations: new Map(), liveRecommendationFlights: new Map(), liveRecommendationTraces: new Map(),
    liveRecommendationFailures: new Map(), liveRecommendationSkipDiagnostics: new Set(), liveGameGeneration: 0,
  };
  const functions = compileFunctions(gameplayScript, ["newLiveRecommendationTraceId", "recordItemSetClientDiagnostic", "ensureLiveRecommendations", "hasUsableLiveRecommendations", "liveRecommendationFlightActive", "recordLiveRecommendationSkip"], {
    state,
    Date: { now: () => now },
    liveRecommendationTarget: () => target,
    renderLive: () => {},
    ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, ensureSpecialistRunes: () => {},
    api: async (path) => { lastAPIPath = path; apiCalls += 1; return apiCalls === 1 ? { recommendations: { runes: { opgg: [] } } } : { recommendations: { runes: { opgg: [{ key: "verified" }] } } }; },
    URLSearchParams,
    fetch: async () => { diagnosticCalls += 1; return { ok: true }; },
  });

  // A stale marker is cleared at the 30-second boundary and cannot block the request.
  state.liveRecommendationFlights.set(target.key, 60_000);
  await functions.ensureLiveRecommendations({});
  assert.equal(apiCalls, 1);
	assert.match(lastAPIPath, /[?&]tier=diamond(?:&|$)/);
  assert.equal(state.liveRecommendations.has(target.key), false, "empty recommendations must not be cached");
  assert.equal(state.liveRecommendationFlights.has(target.key), false);

  now = 161_001;
  await functions.ensureLiveRecommendations({});
  assert.equal(apiCalls, 2, "an empty response must be retried after backoff");
  assert.equal(state.liveRecommendations.get(target.key)?.runes?.opgg?.[0]?.key, "verified");
  await functions.ensureLiveRecommendations({});
  assert.equal(apiCalls, 2, "a cached normal response must not trigger a second request");
  state.liveRecommendationSkipDiagnostics.clear();
  diagnosticCalls = 0;
  functions.recordLiveRecommendationSkip("cached", target, {});
  functions.recordLiveRecommendationSkip("cached", target, {});
  assert.equal(diagnosticCalls, 1, "skip diagnostics must deduplicate by reason and key");
});

test("recommendation tabs reset only when the champion changes", () => {
  const state = { recommendationTab: "build", recommendationTabTouched: true, runeSourceTab: "specialist" };
  const functions = compileFunctions(gameplayScript, ["resetRecommendationTabsOnChampionChange"], {
    state,
    liveRecommendationTarget: (data) => data ? { championId: data.championId, key: `${data.championId}:${data.spell1Id || 0}` } : null,
  });
  functions.resetRecommendationTabsOnChampionChange({ championId: 13, spell1Id: 4 }, { championId: 14, spell1Id: 4 });
  assert.deepEqual(state, { recommendationTab: "runes", recommendationTabTouched: false, runeSourceTab: "opgg" });
  state.recommendationTab = "build";
  state.recommendationTabTouched = true;
  state.runeSourceTab = "specialist";
  functions.resetRecommendationTabsOnChampionChange({ championId: 14, spell1Id: 4 }, { championId: 14, spell1Id: 12 });
  assert.deepEqual(state, { recommendationTab: "build", recommendationTabTouched: true, runeSourceTab: "specialist" });
  functions.resetRecommendationTabsOnChampionChange({ championId: 14 }, null);
  assert.equal(state.recommendationTab, "build", "a transient empty pick must not change the existing tab policy");
});

test("live game scoped state resets on a new game or champion select entry", () => {
  const state = {
    liveRecommendations: new Map([["old", { runes: {} }]]),
    specialistRunes: new Map([["old", []]]),
    liveRecommendationFailures: new Map([["old", 1]]),
    liveRecommendationFlights: new Map([["old", 1]]),
    specialistRuneFailures: new Map([["old", { reason: "request-failed", at: 1 }]]),
    specialistRuneFlights: new Map([["old", 1]]),
    livePositionOverride: new Map([["old", "mid"]]),
    liveRecommendationSkipDiagnostics: new Set(["cached:old"]),
    specialistPlayerTabs: new Map([["old", "player"]]),
    recommendationTab: "build", recommendationTabTouched: true, runeSourceTab: "specialist", selectedRecommendation: "specialist",
  };
  const functions = compileFunctions(gameplayScript, ["shouldResetLiveGameScopedState", "resetLiveGameScopedState"], { state });
  assert.equal(functions.shouldResetLiveGameScopedState({ gameId: 10, phase: "InProgress" }, { gameId: 11, phase: "InProgress" }), true);
  functions.resetLiveGameScopedState();
  for (const collection of [state.liveRecommendations, state.specialistRunes, state.liveRecommendationFailures, state.liveRecommendationFlights, state.specialistRuneFailures, state.specialistRuneFlights, state.livePositionOverride, state.liveRecommendationSkipDiagnostics, state.specialistPlayerTabs]) assert.equal(collection.size, 0);
  assert.equal(state.recommendationTab, "runes");
  assert.equal(state.runeSourceTab, "opgg");
  assert.equal(functions.shouldResetLiveGameScopedState({ gameId: 11, phase: "Lobby" }, { gameId: 11, phase: "ChampSelect" }), true);
  assert.equal(functions.shouldResetLiveGameScopedState({ gameId: 11, phase: "ChampSelect" }, { gameId: 11, phase: "ChampSelect" }), false, "ordinary same-game polling must not reset state");
  const loadSource = functionSource(gameplayScript, "loadLive");
  const mutated = loadSource.replace("if (shouldResetLiveGameScopedState(previousLive, nextLive)) resetLiveGameScopedState();", "if (false) resetLiveGameScopedState();");
  assert.throws(() => assert.match(mutated, /if \(shouldResetLiveGameScopedState\(previousLive, nextLive\)\) resetLiveGameScopedState\(\);/));
});

test("round 9 recommendation percentages preserve backend units and hide empty statistics", () => {
  const { rate, renderOptionStats, renderCoreStats } = compileFunctions(gameplayScript, ["rate", "renderOptionStats", "renderCoreStats"], {
    compactNumber: (value) => value == null ? "—" : String(value),
  });
  assert.equal(rate(0.42), "0.4%");
  assert.equal(rate(0), "0%");
  assert.equal(rate(null), "—");
  const zero = renderOptionStats({ pickRate: 0, winRate: 0, games: 0 });
  const missing = renderOptionStats({ pickRate: null, winRate: null, games: null });
  assert.equal(zero, "");
  assert.equal(missing, "");
  assert.equal(renderCoreStats({ pickRate: 0, winRate: 0, games: 0 }), "");
  assert.equal(renderCoreStats({ pickRate: null, winRate: null, games: null }), "");
  assert.match(renderOptionStats({ pickRate: 12.4, winRate: 53.2, games: 88 }), /选取率[\s\S]*胜率[\s\S]*场次/);
  assert.match(renderCoreStats({ winRate: 53.2, games: 88 }), /胜率[\s\S]*场次/);
  assert.match(functionSource(gameplayScript, "renderOptionStats"), /if \(!hasPick && !hasWin\) return "";/);
  assert.match(functionSource(gameplayScript, "renderCoreStats"), /if \(!hasPick && !hasWin\) return "";/);
});

test("live QQ101 depth rows preserve unavailable sample metadata", () => {
	const { renderConfigOption, renderDepthStats } = compileFunctions(gameplayScript, ["renderConfigOption", "renderDepthStats"], {
		state: { items: { items: [{ id: 1, name: "测试装备" }] } },
		renderSummonerSpellIcon: () => "<i></i>",
		renderItemIcon: () => "<i></i>",
		escapeHTML: (value) => String(value ?? ""),
		renderOptionStats: () => "",
		rate: (value) => `${value}%`,
		compactNumber: (value) => String(value),
	});
	const markup = renderConfigOption({ ids: [1], gamesUnavailable: true, stats: { winRate: 61.58, games: 0 } }, "item", renderDepthStats);
	assert.match(markup, /场次[\s\S]*未提供/);
	assert.doesNotMatch(markup, />0<\/dd>/);
	assert.match(functionSource(gameplayScript, "renderConfigOption"), /gamesUnavailable: option\.gamesUnavailable/);
});

test("live build item cards keep icon tooltips without visible item names", () => {
	const { renderConfigOption } = compileFunctions(gameplayScript, ["renderConfigOption"], {
		renderSummonerSpellIcon: (id) => `<span data-tooltip="技能 ${id}"></span>`,
		renderItemIcon: (id) => `<span data-tooltip="装备 ${id}"></span>`,
		renderOptionStats: () => "",
	});
	const markup = renderConfigOption({ ids: [3089] }, "item");
	assert.match(markup, /data-tooltip="装备 3089"/);
	assert.doesNotMatch(markup, /item-option-name|<small/);
});

test("round 9 match summaries show only placement while details align it beside scores", () => {
	const { ordinalLabel, scoreRankChip, scoreBadgeChip, scoreChip, scorePlacementChip, renderMatchScoreCell } = compileFunctions(
		gameplayScript,
		["ordinalLabel", "scoreRankChip", "scoreBadgeChip", "scoreChip", "scorePlacementChip", "renderMatchScoreCell"],
		{ number: (value) => String(value ?? "—") },
	);
	assert.deepEqual(
		[1, 2, 3, 4, 10, 11, 12, 13, 21, 22, 23].map(ordinalLabel),
		["1st", "2nd", "3rd", "4th", "10th", "11th", "12th", "13th", "21st", "22nd", "23rd"],
	);

  const scoreClass = (score) => scoreChip({ score }).match(/^<i class="([^"]+)"/)?.[1];
  assert.equal(scoreClass(4.9), "match-score is-poor");
  assert.equal(scoreClass(5), "match-score");
  assert.equal(scoreClass(6.4), "match-score");
  assert.equal(scoreClass(6.5), "match-score is-good");
  assert.equal(scoreClass(7.9), "match-score is-good");
  assert.equal(scoreClass(8), "match-score is-gold");
  assert.equal(scoreClass(8.1), "match-score is-gold");

	const goldRecord = { score: 8.1, rank: 1, total: 10, badge: "MVP" };
	assert.equal(scorePlacementChip(goldRecord), scoreBadgeChip(goldRecord));
	assert.equal(
		renderMatchScoreCell(goldRecord),
		`<span class="match-score-cell"><span class="match-score-value">${scoreChip(goldRecord)}</span><span class="match-score-badge">${scoreBadgeChip(goldRecord)}</span></span>`,
	);
	const svpRecord = { score: 7.9, rank: 2, total: 10, badge: "SVP" };
	assert.equal(scorePlacementChip(svpRecord), scoreBadgeChip(svpRecord));
	assert.equal(
		renderMatchScoreCell(svpRecord),
		`<span class="match-score-cell"><span class="match-score-value">${scoreChip(svpRecord)}</span><span class="match-score-badge">${scoreBadgeChip(svpRecord)}</span></span>`,
	);
	const neutralRecord = { score: 6.4, rank: 7, total: 10, badge: "" };
	assert.equal(scorePlacementChip(neutralRecord), scoreRankChip(7, 10));
	assert.equal(scorePlacementChip({ score: 8.4, rank: 1, total: 10, badge: "" }), "");
	assert.equal(
		renderMatchScoreCell(neutralRecord),
		`<span class="match-score-cell"><span class="match-score-value">${scoreChip(neutralRecord)}</span><span class="match-score-badge">${scoreRankChip(7, 10)}</span></span>`,
	);
	assert.equal(scoreBadgeChip({ badge: "" }), "");
});

test("round 9 match scores assign overall ranks and replace side leaders with MVP and SVP", () => {
	const { participantGroupKey, computeMatchScores } = compileFunctions(
		gameplayScript,
		["participantGroupKey", "computeMatchScores"],
	);
	const participant = (participantId, teamId, win, damage) => ({
		participantId, teamId, win, damage, kills: damage / 1000, assists: 0, deaths: 1,
		gold: damage, cs: damage / 100, csPerMinute: damage / 1000, visionScore: damage / 2000,
	});
	const scores = computeMatchScores({ duration: 1200, participants: [
		participant(1, 100, true, 10000),
		participant(2, 100, true, 8000),
		participant(3, 200, false, 11000),
		participant(4, 200, false, 9000),
	] });
	assert.deepEqual(
		[...scores.values()].map(({ rank, total, badge }) => ({ rank, total, badge })),
		[
			{ rank: 2, total: 4, badge: "MVP" },
			{ rank: 4, total: 4, badge: "" },
			{ rank: 1, total: 4, badge: "SVP" },
			{ rank: 3, total: 4, badge: "" },
		],
	);
});

test("round 9 match score ranks use hidden precision and keep exact ties deterministic", () => {
	const { participantGroupKey, computeMatchScores } = compileFunctions(
		gameplayScript,
		["participantGroupKey", "computeMatchScores"],
	);
	const participants = [
		{ participantId: 1, teamId: 100, win: true, kills: 10, assists: 0, deaths: 1, damage: 10000 },
		{ participantId: 2, teamId: 100, win: true, kills: 10, assists: 0, deaths: 1, damage: 10000 },
		{ participantId: 3, teamId: 200, win: false, kills: 9.96, assists: 0, deaths: 1, damage: 9960 },
		{ participantId: 4, teamId: 200, win: false, kills: 9.94, assists: 0, deaths: 1, damage: 9940 },
	];
	const scores = computeMatchScores({ duration: 1200, participants });
	assert.equal(scores.get(3).score, scores.get(4).score);
	assert.ok(scores.get(3).rawScore > scores.get(4).rawScore);
	assert.deepEqual([...scores.values()].map(({ rank }) => rank), [1, 2, 3, 4]);
});

test("round 9 augment rendering uses upstream images, descriptions, and all rarity tones", () => {
  const state = { perks: { augments: [] } };
  const dependencies = {
    state,
    plainText: (value) => String(value || ""),
    escapeHTML: (value) => String(value ?? ""),
    assetIcon: (path) => `<img data-path="${path}">`,
    iconFigure: (_kind, id) => `<span>fallback-${id}</span>`,
    pendingCatalogIcon: (name) => `<span>${name}</span>`,
  };
  const augmentFunctions = compileFunctions(
    gameplayScript,
    ["normalizeAugmentRarity", "augmentTooltipText", "wrapAugmentIcon", "augmentIconFigure"],
    dependencies,
  );
  for (const [rarity, tone] of Object.entries({ kBronze: "bronze", kSilver: "silver", kGold: "gold", kPrismatic: "prismatic", kEventChoice: "event" })) {
    state.perks.augments = [{ id: 7, name: "测试海克斯", rarity, iconPath: "/augment.png" }];
    assert.match(augmentFunctions.augmentIconFigure(7, "large"), new RegExp(`arena-augment-icon is-${tone}`));
  }

  const { renderLiveAugmentIcon } = compileFunctions(
    gameplayScript,
    ["normalizeAugmentRarity", "augmentTooltipText", "wrapAugmentIcon", "renderLiveAugmentIcon"],
    {
      plainText: dependencies.plainText,
      escapeHTML: dependencies.escapeHTML,
      remoteStaticIcon: (source, path, name) => `<img data-source="${source}" data-path="${path}" alt="${name}">`,
      augmentIconFigure: (id) => `<span>fallback-${id}</span>`,
    },
  );
  const rendered = renderLiveAugmentIcon({
    id: 52,
    rarity: "kGold",
    assets: [{ id: 52, name: "闪电打击", description: "获得总攻击速度。", source: "opgg", path: "/meta/images/lol/latest/augment/lightningstrikes_large.png" }],
  }, "hextech");
  assert.match(rendered, /data-path="\/meta\/images\/lol\/latest\/augment\/lightningstrikes_large\.png"/);
  assert.match(rendered, /arena-augment-icon is-gold/);
  assert.match(rendered, /获得总攻击速度。/);
});

test("round 9 recommendation modes drive tabs, fallbacks, and ranked-only sources", () => {
	  const state = { specialistRuneFailures: new Map(), specialistRuneFlights: new Map(), specialistRunes: new Map() };
	  state.queueGroups = [
	    { id: 1700, modeGroup: "arena", augmentSource: "arena" },
	    { id: 1710, modeGroup: "arena", augmentSource: "arena" },
	    { id: 1750, modeGroup: "arena", augmentSource: "arena" },
	    { id: 2300, modeGroup: "hextech-aram", augmentSource: "hextech" },
	    { id: 2400, modeGroup: "hextech-aram", augmentSource: "hextech" },
	    { id: 3270, modeGroup: "hextech-aram", augmentSource: "hextech" },
	  ];
  const functions = compileFunctions(
    gameplayScript,
    [
      "matchQueueLabel",
      "queueDefinitionFor",
      "queueModeGroup",
      "isHextechClassic",
      "isHextechQualifier",
      "isOrdinaryHextechMatch",
      "liveAugmentRecommendationSource",
      "recommendationQueueHasTopPlayers",
      "recommendationCapabilities",
      "recommendationTabSpecs",
      "recommendationActiveTab",
      "renderRecommendationDataNotices",
	  "specialistRuneFailure",
      "renderRuneRecommendations",
    ],
    {
      escapeHTML: (value) => String(value ?? ""),
	      state,
      liveRecommendationTarget: () => ({ key: "target" }),
      liveRecommendationsFor: (data) => data?.recommendations || ({ runes: { opgg: { key: "opgg" }, pros: [{ key: "pro" }] } }),
      specialistRunesFor: () => [{ key: "specialist" }],
      renderRuneSourceSection: (section) => `[${section.key}]`,
    },
  );
	  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 2300 }), "hextech");
	  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 2400, gameMode: "KIWI", mapId: 12 }), "hextech");
	  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 3270, gameMode: "KIWI", mapId: 12 }), "hextech");
  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 2400, gameMode: "ARAM_MAYHEM_CLASSIC", mapId: 12 }), "");
  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 2600, queueLabel: "海克斯大乱斗 海选赛", gameMode: "KIWI", mapId: 12 }), "hextech");
  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 1700 }), "arena");
  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 1750 }), "arena");
  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 0, gameMode: "kiwi", mapId: 12 }), "hextech");
  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 0, gameMode: "CHERRY", mapId: 30 }), "arena");
  assert.equal(functions.liveAugmentRecommendationSource({ queueId: 0, gameMode: "KIWI", mapId: 11 }), "");
  assert.deepEqual(functions.recommendationTabSpecs({ hasRunes: true, hasAugments: false }).map(([key]) => key), ["runes", "insight", "build"]);
	  assert.deepEqual(functions.recommendationTabSpecs({ hasRunes: true, hasAugments: true }).map(([key]) => key), ["runes", "build", "insight"]);
	  assert.deepEqual(functions.recommendationTabSpecs({ hasRunes: false, hasAugments: true }), [["build", "海克斯与出装"], ["insight", "详情"]]);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 420, gameMode: "CLASSIC", mapId: 11 }), true);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 440, gameMode: "CLASSIC", mapId: 11 }), true);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 0, gameMode: "CLASSIC", mapId: 11 }), true);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: -1, gameMode: "CLASSIC", mapId: 11 }), true);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 3100, gameMode: "CLASSIC", mapId: 11 }), true);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 0, gameMode: "ARAM", mapId: 12 }), false);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 420, recommendations: { hasTopPlayers: false } }), false);
	assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 400, recommendations: { hasTopPlayers: true } }), true);
  assert.equal(functions.recommendationQueueHasTopPlayers({ queueId: 400 }), false);
  assert.equal(functions.recommendationCapabilities({ queueId: 2300 }, { hasRunes: true, hasAugments: true }).hasRunes, false);
  assert.equal(functions.recommendationCapabilities({ queueId: 2300 }, {}).hasAugments, true);
  assert.equal(functions.recommendationCapabilities({ queueId: 1700 }, { hasRunes: true, hasAugments: true }).hasRunes, false);
  assert.equal(functions.recommendationCapabilities({ queueId: 2400, gameMode: "KIWI", mapId: 12 }, { hasRunes: true }).hasRunes, false);
  assert.equal(functions.recommendationCapabilities({ queueId: 2400, gameMode: "KIWI", mapId: 12 }, { hasRunes: true }).hasAugments, true);
  assert.equal(functions.recommendationCapabilities({ queueId: 2400, gameMode: "ARAM_MAYHEM_CLASSIC", mapId: 12 }, { hasRunes: true }).hasRunes, true);
  assert.equal(functions.recommendationCapabilities({ queueId: 2400, gameMode: "ARAM_MAYHEM_CLASSIC", mapId: 12 }, { hasRunes: true }).hasAugments, false);
  assert.equal(functions.recommendationCapabilities({ queueId: 420 }, { hasRunes: true }).hasRunes, true);
  assert.equal(functions.recommendationCapabilities({ queueId: 420 }, { hasItemDepths: true }).hasItemDepths, true);
  assert.equal(functions.recommendationCapabilities({ queueId: 450 }, { hasItemDepths: false }).hasItemDepths, false);
  assert.equal(functions.recommendationActiveTab(["runes", "insight", "build"], { hasAugments: true }), "build");
  state.recommendationTab = "runes";
  state.recommendationTabTouched = true;
  assert.equal(functions.recommendationActiveTab(["runes", "insight", "build"], { hasAugments: true }), "runes");
	assert.match(functions.renderRuneRecommendations({ queueId: 420 }, true), /class="rune-source-tabs"[\s\S]*data-rune-source="specialist"[\s\S]*class="rune-source-stack">\[opgg\]<\/div>/);
	assert.match(functions.renderRuneRecommendations({ queueId: 1700 }, true), /class="rune-source-tabs"[\s\S]*class="rune-source-stack">\[opgg\]<\/div>/);
	state.runeSourceTab = "specialist";
	const specialistSources = functions.renderRuneRecommendations({ queueId: 420 }, true);
	assert.match(specialistSources, /class="rune-source-tab-buttons" role="tablist"[\s\S]*<div class="rune-source-stack">\[specialist\]/);
	assert.doesNotMatch(specialistSources, /rune-source-note|当前分路 · 最近 10 局/);
	assert.doesNotMatch(specialistSources, /rune-source-section"><header>/);
  assert.match(functions.renderRecommendationDataNotices({ isFallback: true, resolvedMode: "aram" }), /参考极地大乱斗数据/);
  assert.match(functions.renderRecommendationDataNotices({ isFallback: true, resolvedMode: "aram" }), /出装与技能/);
  assert.match(functions.renderRecommendationDataNotices({ isStale: true, dataVersion: "13.23" }), /13\.23 版本，仅供参考/);
});

test("OPGG only uses the client fallback when no upstream positions exist", () => {
	const state = { runeSourceTab: "opgg", specialistRuneFailures: new Map(), specialistRunes: new Map() };
	let renderedItems = null;
	const { renderRuneRecommendations } = compileFunctions(gameplayScript, ["renderRuneRecommendations"], {
		state,
		liveRecommendationsFor: (data) => data.recommendations || {},
		liveRecommendationTarget: () => null,
		recommendationQueueHasTopPlayers: () => false,
		renderRuneSourceSection: (section) => {
			renderedItems = section.items;
			return "[opgg]";
		},
		escapeHTML: (value) => String(value ?? ""),
	});
	const clientRecommendation = { key: "client", title: "客户端符文" };
	renderRuneRecommendations({ recommendations: { positions: [{ position: "mid" }], runes: { opgg: [] } }, clientRecommendation }, true);
	assert.deepEqual(renderedItems, [], "已有 OPGG 位置时不能冒充客户端备用符文");
	renderRuneRecommendations({ recommendations: { positions: [], runes: { opgg: [] } }, clientRecommendation }, true);
	assert.equal(renderedItems.length, 1);
	assert.equal(renderedItems[0].title, "客户端内置（备用）");
});

test("live recommendations can temporarily use a champ-select pick intent", () => {
  assert.match(gameplayScript, /const USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS = true/);
  assert.match(gameplayScript, /function liveRecommendationChampionId\(player\)/);
  assert.match(gameplayScript, /player\?\.championLocked !== true/);
  assert.match(gameplayScript, /const championId = liveRecommendationChampionId\(self\)/);
  assert.match(gameplayScript, /championId: String\(target\.championId\)/);
	assert.match(gameplayScript, /if \(target\.gameId > 0\) query\.set\("gameId", String\(target\.gameId\)\)/);
  assert.match(gameplayBackend, /ChampionPickIntent\s+int64\s+`json:"championPickIntent,omitempty"`/);
  assert.match(gameplayBackend, /ChampionLocked\s+bool\s+`json:"championLocked"`/);
	assert.match(gameplayBackend, /pickIntent := positiveChampionPickIntent\(selected\.ChampionPickIntent\)/);
	assert.match(gameplayBackend, /ChampionPickPending: selected\.ChampionPickIntent < 0/);
  assert.match(gameplayBackend, /func gameplayLiveRecommendationTarget\(players \[\]gameplayLivePlayer, currentChampionID int64\)/);
  assert.match(gameplayBackend, /championID = player\.ChampionPickIntent/);
  assert.match(gameplayBackend, /if currentChampionID > 0 \{\s*return currentChampionID, position/s);
});

test("live recommendation champion ids reject negative pick sentinels and use resolved live fallback", () => {
  const state = { livePositionOverride: new Map() };
  const { liveRecommendationChampionId, liveRecommendationTarget } = compileFunctions(gameplayScript, ["liveRecommendationChampionId", "liveRecommendationTarget"], {
    state,
    USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS: true,
    isARAMRelatedMatch: () => false,
    liveAugmentRecommendationSource: () => "arena",
    liveRecommendationTier: () => "diamond",
  });
  for (const sentinel of [-1, -3, -999]) assert.equal(liveRecommendationChampionId({ championId: 0, championPickIntent: sentinel }), 0);
  assert.equal(liveRecommendationChampionId({ championId: 0, championPickIntent: 25 }), 25);
  const target = liveRecommendationTarget({
    players: [{ isCurrent: true, championId: 0, championPickIntent: -3, position: "other" }],
    currentChampionId: -3, resolvedChampionId: 25, queueId: 1750, gameMode: "CHERRY", mapId: 30,
  });
  assert.equal(target?.championId, 25);
});

test("random pick pending state has distinct empty-state copy", () => {
  const { renderRecommendationArea } = compileFunctions(gameplayScript, ["renderRecommendationArea"], {
    state: {
      recommendationTab: "build", recommendationTabTouched: false, runeSourceTab: "opgg", selectedRecommendation: "opgg",
      livePositionOverride: new Map(), liveRecommendations: new Map(), liveRecommendationFailures: new Map(), liveRecommendationFlights: new Map(),
    },
    liveRecommendationTarget: () => null,
    liveRecommendationsFor: () => ({}),
    isARAMRelatedMatch: () => false,
    liveAugmentRecommendationSource: () => "arena",
    recommendationCapabilities: () => ({ hasRunes: false, hasAugments: true, hasCounters: true, hasBanRate: true, hasTopPlayers: false }),
    recommendationTabSpecs: () => [["build", "海克斯与出装"], ["insight", "详情"]],
    recommendationActiveTab: () => "build",
    selectedRuneRecommendation: () => null,
    renderLiveInsights: () => "",
    renderRecommendationDataNotices: () => "",
    recommendationPanelBusy: () => false,
    liveRecommendationFlightActive: () => false,
    escapeHTML: (value) => String(value ?? ""),
    renderLiveAugmentRecommendations: () => "",
    renderBuildRecommendation: () => "",
    renderChampionRecommendationHeader: () => "",
    recommendationEmptyPanel: (title, copy) => `<strong>${title}</strong><p>${copy}</p>`,
  });
  const markup = renderRecommendationArea({ available: true, champSelectNotice: "斗魂英雄选择阶段只展示小队玩家信息", players: [{ isCurrent: true, championPickPending: true, championPickIntent: 0 }] });
  assert.match(markup, /随机待定/);
  assert.match(markup, /英雄尚未确定，锁定后自动加载/);
  assert.match(markup, /class="recommendation-tab-row"><div class="recommendation-tabs"[\s\S]*class="live-roster-notice"/);
  assert.ok(markup.indexOf("recommendation-tabs") < markup.indexOf("live-roster-notice"), "notice must follow tabs inside their shared row");
});

test("rune application creates or recycles only Deep Legends pages", () => {
  assert.doesNotMatch(html, /setting-confirm-runes|符文写入确认/);
  assert.doesNotMatch(gameplayScript, /confirmRunes|confirm-runes|result\.overwritten|覆盖当前或第一个可编辑页/);
  assert.match(gameplayScript, /sourceLabel: "OPGG"/);
  assert.match(gameplayScript, /sourceLabel: "绝活哥"/);
  assert.match(gameplayScript, /championName: self\.championName, source: recommendation\.sourceLabel/);
	assert.match(gameplayScript, /符文已新建并设为当前页/);
	assert.match(gameplayScript, /将新建符文页并设为当前页/);
	assert.match(gameplayBackend, /客户端符文页已达上限/);
	assert.match(gameplayBackend, /\[DL\]/);
	assert.match(gameplayBackend, /runePageRecycleLimit\s*=\s*5/);
  assert.match(gameplayScript, /button\.textContent = "再次新增"/);
  assert.match(gameplayScript, /正在读取韩服绝活哥符文/);
  assert.match(gameplayScript, /韩服绝活哥符文读取失败/);
  assert.match(gameplayScript, /暂无可核验的韩服绝活哥符文/);
  assert.match(gameplayScript, /config\.playerName/);
  assert.match(gameplayScript, /config\.tagLine/);
  assert.match(gameplayScript, /config\.playedAt/);
  assert.match(gameplayScript, /config\.region === "kr" \? "韩服"/);
  assert.match(gameplayScript, /config\.result === "win" \? "胜利"/);
  assert.match(gameplayStyles, /\.rune-choice-copy small/);
	assert.doesNotMatch(gameplayScript, /class="rune-choice-selector"[^\n]+aria-expanded=/);
	assert.doesNotMatch(gameplayScript, /class="rune-choice-selector"[^\n]+aria-controls=/);
	assert.match(gameplayScript, /class="rune-choice-detail">\$\{renderUnifiedRuneBoard\(config\)\}/);
	assert.doesNotMatch(gameplayScript, /selected \? renderUnifiedRuneBoard\(config\)/);
	assert.match(gameplayStyles, /\.unified-rune-row\s*\{[^}]*gap:\s*8px/s);
	assert.match(gameplayStyles, /\.unified-rune-board \.game-icon\.is-rune\s*\{[^}]*width:\s*var\(--rune-icon-size\)[^}]*height:\s*var\(--rune-icon-size\)[^}]*overflow:\s*hidden/s);
  assert.doesNotMatch(gameplayScript, /当前 OP\.GG 专家榜不返回玩家的完整符文/);
  assert.match(gameplayScript, /当前数据源不提供可核验的职业选手身份与完整符文/);
});

test("friend navigation uses only session-scoped player references", () => {
	assert.match(friendsScript, /data-player-ref="\$\{escapeHTML\(friend\.playerRef \|\| ""\)\}"/);
	assert.match(friendsScript, /detail: \{ playerRef: row\.dataset\.playerRef, gameName:/);
	assert.match(gameplayScript, /const playerRef = String\(event\.detail\?\.playerRef \|\| ""\)\.trim\(\)/);
	assert.match(gameplayScript, /if \(playerRef\) \{[\s\S]+source === "search"[\s\S]+openPlayer\(playerRef, label, region, serverId, tabContext\)/);
	assert.doesNotMatch(friendsScript, /friend\.puuid|friend\.summonerId/);
	assert.match(demoScript, /playerRef: `player_\$\{String\(icon\)/);
	assert.doesNotMatch(demoScript, /puuid: `demo-|summonerId: 0/);
});

test("position filters use icons and transient search state is reset", () => {
  assert.match(script, /function positionIcon\(value\)/);
  assert.match(script, /class="position-icon"/);
  assert.match(script, /\/position-icons\/\$\{name\}\.svg/);
  assert.match(gameplayScript, /\/position-icons\/\$\{name\}\.svg/);
  for (const icon of positionIcons) {
    assert.match(icon, /viewBox="0 0 34 34"/);
    assert.match(icon, /fill="#c8aa6e"/);
  }
  assert.doesNotMatch(allPositionIcon, /6\.5-6\.5|6\.5 6\.5/);
  assert.match(allPositionIcon, /fill-rule="evenodd"/);
  assert.doesNotMatch(script, /function positionMark\(/);
  assert.match(script, /normalizeSearch\(state\.query\)[\s\S]+state\.position !== "all"[\s\S]+state\.position = "all"/);
  assert.match(script, /previous === "champions" && state\.section !== "champions"/);
  assert.match(script, /state\.augmentRarity = "all"/);
});

test("rune shards use crisp login-independent Data Dragon assets", () => {
  assert.match(backend, /StatModsAttackSpeedIcon\.png/);
  assert.match(backend, /asset\.Kind, asset\.Source, asset\.Path = "perkShard", "ddragon", shardPath/);
  assert.match(gameplayScript, /function dataDragonRuneShardPath\(id\)/);
  assert.match(gameplayScript, /remoteStaticIcon\("ddragon", shardPath/);
  assert.match(gameplayScript, /5001:\s*"StatModsHealthPlusIcon\.png"/);
  assert.match(gameplayScript, /5011:\s*"StatModsHealthScalingIcon\.png"/);
  assert.match(gameplayScript, /5008:\s*"获得9适应之力/);
  assert.match(backend, /5005:\s*"获得10%攻击速度/);
  assert.match(script, /classList\.add\("has-loaded-image"\)/);
  assert.match(gameplayScript, /classList\.add\("has-loaded-image"\)/);
  assert.match(appStyles, /\.game-icon\.has-loaded-image > span, \.champion-asset\.has-loaded-image > span\s*\{\s*display:\s*none/);
  assert.match(styles, /\.champion-rune-column\.is-shards \.rune-icon img\s*\{[^}]*object-fit:\s*contain/s);
});

test("server-filtered flex switch makes one fresh first-page request", async () => {
	assert.match(gameplayScript, /async function updateMatchFilter\(tab, value\)/);
	assert.doesNotMatch(functionSource(gameplayScript, "updateMatchFilter"), /rankedQueueRecent|rankedQueueAbility|rankedQueuePosition/);
	assert.match(gameplayScript, /matchScrollTops:\s*new Map\(\)/);
	assert.match(gameplayScript, /rememberMatchScrollTop\(tab, tab\.matchFilter\)/);
	assert.match(gameplayScript, /function restoreMatchScrollTop\(tab\)[\s\S]+root\.scrollTop = scrollTop/);
	assert.match(gameplayScript, /loaded && !tab\.data\?\.pagination\?\.serverFiltered && tab\.data\?\.pagination\?\.filterFallback/);
	assert.match(gameplayScript, /while \(tab\.filterPagingToken === token && tab\.matchFilter === filter\)/);
	assert.match(gameplayScript, /visible >= state\.settings\.matchCount/);
	assert.match(gameplayScript, /正在查找更早的\$\{escapeHTML\(matchFilterDisplayLabel\(tab\)\)\}对局…第/);
  assert.match(gameplayScript, /function renderFilteredMatchView\(tab, restoreScroll = false\)[\s\S]*querySelector\("\.match-list"\)/);
  assert.doesNotMatch(gameplayScript, /data-match-filter[^\n]+rerender\(\)/);
	assert.match(gameplayScript, /upstreamAdditions = additions\.length/);
  assert.match(gameplayScript, /tab\.paginationStalls >= 2[\s\S]*pagination\.autoPaused = true/);
	assert.match(gameplayScript, /上游连续无响应，已暂停自动查找/);
  assert.match(gameplayScript, /AUTO_PAGE_DELAY_MS = 400/);
  assert.match(gameplayScript, /error\.status === 503[\s\S]*paginationBackoffMs/);

	const state = { settings: { matchCount: 20 }, matchObserver: null, overlayObserver: null };
	const loads = [];
	const tab = {
		matchFilter: "all", matchScrollTops: new Map(), openMatches: new Set(["open"]), matchDetailTabs: new Map([["open", "summary"]]),
		data: { matches: [], pagination: { hasMore: true, autoPaused: false } },
		nextBegIndex: 1, paginationStalls: 0, loading: false, loadingMore: false,
	};
	const { updateMatchFilter } = compileFunctions(gameplayScript, ["updateMatchFilter", "autoLoadMatchFilter"], {
		state,
		matchObserverKey: (currentTab) => currentTab.overlay ? "overlayObserver" : "matchObserver",
		rememberMatchScrollTop: () => {},
		renderFilteredMatchView: () => {},
		filteredMatches: (matches, currentTab) => matches.filter((match) => currentTab.matchFilter !== "flex" || Number(match.queueId) === 440),
		loadOverview: async (currentTab, force, append) => {
			loads.push({ filter: currentTab.matchFilter, force, append, begIndex: 0 });
			currentTab.data = { matches: [{ gameId: 1, queueId: 440 }], pagination: { hasMore: true, serverFiltered: true, filterFallback: false } };
			return true;
		},
	});
	await updateMatchFilter(tab, "flex");
	assert.deepEqual(loads, [{ filter: "flex", force: true, append: false, begIndex: 0 }]);
	assert.equal(tab.matchFilter, "flex");
	assert.equal(tab.filterPaging, false);
	assert.equal(tab.rankedQueueRecent, undefined, "right-side filter must not select the left recent queue");
	assert.equal(tab.rankedQueueAbility, undefined, "right-side filter must not select the left ability queue");
	assert.equal(tab.rankedQueuePosition, undefined, "right-side filter must not select the left position queue");
	assert.equal(tab.openMatches.size, 0);
	assert.equal(tab.matchDetailTabs.size, 0);
});

test("client-filter fallback alone may automatically page until flex appears", async () => {
	const state = { settings: { matchCount: 1 }, matchObserver: null, overlayObserver: null };
	let loads = 0;
	const tab = {
		matchFilter: "all", matchScrollTops: new Map(), openMatches: new Set(), matchDetailTabs: new Map(),
		data: { matches: [], pagination: { hasMore: true } }, nextBegIndex: 0, loading: false, loadingMore: false,
	};
	const { updateMatchFilter } = compileFunctions(gameplayScript, ["updateMatchFilter", "autoLoadMatchFilter"], {
		state,
		matchObserverKey: (currentTab) => currentTab.overlay ? "overlayObserver" : "matchObserver",
		rememberMatchScrollTop: () => {},
		renderFilteredMatchView: () => {},
		filteredMatches: (matches, currentTab) => matches.filter((match) => currentTab.matchFilter !== "flex" || Number(match.queueId) === 440),
		loadOverview: async (currentTab) => {
			loads += 1;
			if (loads === 1) currentTab.data = { matches: [], pagination: { hasMore: true, serverFiltered: false, filterFallback: true } };
			else currentTab.data.matches.push({ gameId: loads, queueId: loads === 4 ? 440 : 420 });
			currentTab.nextBegIndex = loads;
			currentTab.nextAutoAppendAt = 0;
			return true;
		},
	});
	await updateMatchFilter(tab, "flex");
	assert.equal(loads, 4);
	assert.equal(tab.filterPaging, false);
});

test("filtered empty state distinguishes active loading from idle history", () => {
	const { matchListEmptyContent } = compileFunctions(gameplayScript, ["matchListEmptyContent"], {
		emptyState: (title, detail) => `<div>${title}|${detail}</div>`,
		paginationCopyFor: () => '<span class="mini-loading"></span><span>正在查找</span>',
	});
	const paging = matchListEmptyContent({ filterPaging: true, data: { pagination: { hasMore: true } } }, null, "detail");
	assert.match(paging, /mini-loading/);
	assert.doesNotMatch(paging, /没有符合条件的对局/);
	const hasMore = matchListEmptyContent({ filterPaging: false, data: { pagination: { hasMore: true } } }, null, "detail");
	assert.doesNotMatch(hasMore, /mini-loading/);
	assert.match(hasMore, /当前已加载/);
	assert.doesNotMatch(hasMore, /没有符合条件的对局/);
	const exhausted = matchListEmptyContent({ filterPaging: false, data: { pagination: { hasMore: false } } }, null, "detail");
	assert.match(exhausted, /没有符合条件的对局/);
	const paused = matchListEmptyContent({ filterPaging: false, data: { pagination: { hasMore: true, autoPaused: true } } }, null, "detail");
	assert.match(paused, /当前已加载/);
});

test("filtered empty state renders the pagination copy only once", () => {
  const { matchSentinelShouldHide } = compileFunctions(gameplayScript, ["matchSentinelShouldHide"]);
  assert.equal(matchSentinelShouldHide({ filterPaging: true }, false), true);
  assert.equal(matchSentinelShouldHide({ filterPaging: true }, true), false);
  const renderSource = functionSource(gameplayScript, "renderOverviewBodyContent");
  assert.match(renderSource, /data-match-sentinel[^>]*\$\{sentinelHidden\}/);
  assert.match(renderSource, /matchSentinelShouldHide\(tab, matches\.length > 0\)/);
  const mutated = gameplayScript.replace("matchSentinelShouldHide(tab, matches.length > 0)", "false");
  assert.throws(() => assert.match(functionSource(mutated, "renderOverviewBodyContent"), /matchSentinelShouldHide\(tab, matches\.length > 0\)/));
});

test("ranked matches omit unavailable local LP records", () => {
  assert.match(gameplayScript, /const ranked = queueID === 420 \|\| queueID === 440/);
	assert.match(gameplayScript, /const lpChip = ranked && resultKind !== "remake" && Number\.isFinite\(lpValue\) && match\.lpDelta != null\s*\n\s*\? `<b class="lp-delta \$\{lpValue >= 0 \? "is-gain" : "is-drop"\}"/);
  assert.doesNotMatch(gameplayScript, /class="lp-delta is-unavailable"|>— LP<\/b>/);
  assert.doesNotMatch(gameplayStyles, /\.lp-delta\.is-unavailable/);
});

test("perk stat mod slots are separated from rune styles", () => {
	assert.match(gameplayBackend, /splitGameplayStatModSlots\(styles\)/);
	assert.match(gameplayBackend, /StatModSlots:\s*statModSlots/);
  assert.match(gameplayScript, /state\.perks\.statModSlots \|\| \[\]/);
  assert.match(demoScript, /statModSlots:\s*\[/);
  assert.doesNotMatch(gameplayScript, /const fragments = \[\s*\[5005, 5008, 5007\]/);
	assert.match(gameplayStyles, /\.build-detail > section > header h4[^}]+margin:\s*0/);
	assert.match(gameplayStyles, /\.build-detail > section > header/);
	assert.doesNotMatch(gameplayStyles, /\.build-detail section > header/);
	assert.match(gameplayStyles, /\.unified-rune-column > header[^}]+justify-content:\s*center/s);
	assert.doesNotMatch(gameplayScript, /按购买时间分组，含出售记录|已选图标完整展示，未选符文降低亮度/);
});

test("perk catalogs supply icons while entertainment recommendations come from the current hero", () => {
	assert.match(gameplayScript, /ensurePerks\(true\)/);
	assert.match(gameplayScript, /renderCapabilitySettings\(\);\s*ensurePerks\(\);/);
	assert.match(gameplayBackend, /gameplayPerkCatalogTTL = 30 \* time\.Minute/);
	assert.match(gameplayBackend, /cachedGameplayPerkCatalog\(r.Context\(\), cacheKey/);
	assert.match(gameplayScript, /function matchAugmentIDs\(subject, limit = 4\)/);
	assert.match(gameplayScript, /<h4>\$\{hasAugments \? "海克斯" : "符文"\}<\/h4>/);
	assert.doesNotMatch(gameplayScript, /\/api\/champions\/augments/);
	assert.match(gameplayScript, /return liveRecommendationsFor\(data\)\?\.augments \|\| \[\]/);
  assert.match(gameplayScript, /function queueDefinitionFor\(value\)/);
  assert.match(mainSource, /QueueGroups\s+\[\]queueGroupResponse/);
  assert.match(gameplayScript, /queueGroups: \[\]/);
  assert.doesNotMatch(gameplayScript, /new Set\(\[2300, 2400\]\)/);
  assert.doesNotMatch(gameplayScript, /new Set\(\[1700, 1710\]\)/);
  assert.match(queueGroupsBackend, /\{1750, "斗魂竞技场", "arena", "arena", "arena", 3\}/);
  const queueMutation = queueGroupsBackend.replace(/\s*\{1750, "斗魂竞技场", "arena", "arena", "arena", 3\},/, "");
  assert.throws(() => assert.match(queueMutation, /\{1750, "斗魂竞技场", "arena", "arena", "arena", 3\}/));
	assert.match(gameplayScript, /function liveAugmentRecommendationSource\(data\)/);
	assert.match(gameplayScript, /queueId: String\(target\.queueId\), gameMode: target\.gameMode, mapId: String\(target\.mapId\)/);
	assert.match(gameplayScript, /query\.set\("source", "mayhem"\)/);
	assert.match(gameplayScript, /remoteStaticIcon\(imageSource, imagePath, details\.name \|\| "海克斯", "large", false\)/);
	assert.doesNotMatch(script, /class="rune-board-panel"[^\n]+ hidden/);
	assert.doesNotMatch(script, /panel\.hidden = Number\(panel\.dataset\.runePagePanel\)/);
});

test("match statistic labels have an explicit faint tone in both themes", () => {
  assert.match(appStyles, /--faint:\s*oklch\(/);
  assert.match(appStyles, /:root\[data-theme="dark"\][\s\S]+--faint:\s*#[0-9A-Fa-f]{6}/);
});

test("tooltips use one viewport-aware portal instead of clipped pseudo elements", () => {
  assert.match(appScript, /function setupFloatingTooltips\(\)/);
  assert.match(appScript, /getBoundingClientRect\(\)/);
  assert.match(appScript, /window\.innerWidth - width - viewportPadding/);
  assert.match(appScript, /fitsAbove/);
	assert.match(appStyles, /\.global-tooltip\s*\{[^}]*position:\s*fixed[^}]*z-index:\s*55/s);
	assert.match(gameplayScript, /function assetIcon\([\s\S]+?data-tooltip=/);
	assert.doesNotMatch(gameplayScript, /title="/);
	assert.doesNotMatch(gameplayStyles, /\.rune-option-button::after|\.skill-icon-button::after|\.item-option-button::after/);
});

test("tooltips use the layered hextech surface in every theme", () => {
  const tooltipRule = appStyles.match(/\.global-tooltip\s*\{([^}]+)\}/s)?.[1] || "";
  assert.match(tooltipRule, /position:\s*fixed;\s*z-index:\s*55/);
  assert.match(tooltipRule, /background:\s*var\(--tooltip-surface\)/);
  assert.match(tooltipRule, /border:\s*1px solid var\(--tooltip-line\)/);
  assert.match(tooltipRule, /opacity:\s*0/);
  assert.match(tooltipRule, /transition:\s*opacity 120ms ease-out,\s*transform 120ms ease-out/);
  assert.doesNotMatch(tooltipRule, /border-top|backdrop-filter|0 0 0 1px/);
  assert.match(appStyles, /\.global-tooltip\[data-shown="true"\][^}]+opacity:\s*1/s);
  assert.match(appStyles, /\.global-tooltip \.tooltip-title/);
  assert.match(appStyles, /\.global-tooltip \.tooltip-body/);
  assert.match(appStyles, /data-placement="left"[^}]+transform-origin:\s*right center/s);
  assert.match(appStyles, /data-placement="right"[^}]+transform-origin:\s*left center/s);
  assert.match(appStyles, /prefers-reduced-motion:\s*reduce[\s\S]+\.global-tooltip/);
  for (const [theme, surface] of Object.entries({ dark: "0D1118", azure: "0C121B", emerald: "0D1116", violet: "0E0F17", crimson: "110E10", aurora: "0C1216", oled: "000000" })) {
    assert.match(appStyles, new RegExp(`:root\\[data-theme="${theme}"\\][\\s\\S]*?--tooltip-surface:\\s*#${surface}`, "i"));
  }
  assert.match(appStyles, /:root\s*\{[\s\S]*?--tooltip-surface:\s*#FFFFFF/);
  assert.match(appStyles, /prefers-color-scheme:\s*dark[\s\S]*?--tooltip-surface:\s*#0D1118/);
  assert.doesNotMatch(appStyles, /--tooltip-surface:[^;]*var\(--primary\)|light-dark\(/);
});

test("floating tooltip content is structured safely and shown only after placement", () => {
  assert.match(appScript, /titleNode\.textContent = title/);
  assert.match(appScript, /bodyNode\.textContent = body/);
  assert.match(appScript, /tooltip\.replaceChildren\(\.\.\.children\)/);
  assert.match(appScript, /tooltip\.dataset\.layout = roster \? "roster" : body \? "titled" : "single"/);
  assert.match(appScript, /delete tooltip\.dataset\.shown;[\s\S]*tooltip\.hidden = true/);
  const menuBranch = appScript.slice(appScript.indexOf('anchor.dataset.tooltipSide === "menu"'));
  const menuBeforeReturn = menuBranch.slice(0, menuBranch.indexOf("return;"));
  assert.match(menuBeforeReturn, /tooltip\.dataset\.placement = fitsRight \? "right" : "left"[\s\S]*tooltip\.dataset\.shown = "true"/);
  assert.match(appScript, /tooltip\.dataset\.placement = fitsAbove \? "top" : "bottom"[\s\S]*tooltip\.dataset\.shown = "true"/);
  assert.doesNotMatch(appScript, /tooltip\.innerHTML\s*=/);
  assert.match(appScript, /"国服大区合并对照"[\s\S]*"联盟一区：/);
});

test("round 9 section navigation restores each page scroll position after panel activation", () => {
  const panels = [{ id: "overview-panel", hidden: false }, { id: "live-panel", hidden: true }];
  const scrollCalls = [];
  const state = { section: "overview", sectionScroll: Object.create(null), favoritesPage: "collection", status: null };
  const el = {
    sectionTabs: [
      { dataset: { section: "overview" }, getAttribute: () => "overview-panel" },
      { dataset: { section: "live" }, getAttribute: () => "live-panel" },
    ],
    sectionPanels: panels,
    appScroll: {
      scrollTop: 240,
      children: [{}],
      scrollTo(options) {
        scrollCalls.push({ ...options, panelsReady: panels[0].hidden && !panels[1].hidden });
      },
    },
    currentSectionTitle: { textContent: "" },
    topbarSubtitle: { textContent: "" },
    pageIntro: { hidden: false },
  };
  state.sectionScroll.live = 75;
  const frames = [];
  let resize;
  class ResizeObserver { constructor(fn) { resize=fn; } observe() {} disconnect() { resize=null; } }
  const { activateSection } = compileFunctions(appScript, ["activateSection", "restoreSectionScroll"], {
    state,
    el,
    activateTab: (tab, _tabs, activate) => activate(tab),
    cancelDeferredImages: () => {},
    resetPoolControls: () => {},
    activateFavoritesPage: () => {},
    activateSettingsPage: () => {},
    loadPrivacy: () => {},
    renderNotice: () => {},
    renderLaunchpad: () => {},
		document: { getElementById: () => null },
    ResizeObserver,
    setTimeout: () => 1, clearTimeout: () => {},
    requestAnimationFrame: (callback) => frames.push(callback),
    window: { ResizeObserver, dispatchEvent: () => {}, matchMedia: () => ({ matches: false }), addEventListener: () => {}, removeEventListener: () => {} },
    CustomEvent: class CustomEvent { constructor(type, options) { this.type = type; this.detail = options?.detail; } },
  });
  activateSection("live");
  assert.equal(state.sectionScroll.overview, 240);
  assert.deepEqual(scrollCalls[0], { top: 75, behavior: "instant", panelsReady: true });

  // 加载态把页面压矮时浏览器会把 scrollTop 夹回 0；还原必须在后续帧里重试，
  // 直到真的回到目标位置为止（这正是「回到英雄页滚动条不还原」的成因）。
  scrollCalls.length = 0;
  el.appScroll.scrollTop = 0;
  while (frames.length) frames.shift()();
  resize(); // content grows after the initial clamped frame
  assert.ok(scrollCalls.length >= 2, "被夹回 0 之后必须继续重试");
  assert.ok(scrollCalls.every((call) => call.top === 75));
  el.appScroll.scrollTop = 75;
  resize(); // reaches target and disconnects
  scrollCalls.length = 0;
  while (frames.length) frames.shift()();
  assert.equal(scrollCalls.length, 0, "已经到位就必须停手，不能一直和用户抢滚动条");

  el.appScroll.scrollTop = 90;
  activateSection("overview");
  assert.equal(state.sectionScroll.live, 90);
  assert.equal(scrollCalls.length, 0, "总览由玩家页签恢复滚动，不再使用页面共享位置");
});

test("round 9 tooltip pointer suppression survives the click focus event", () => {
  class FakeNode {}
  class FakeElement extends FakeNode {
    constructor() {
      super();
      this.dataset = {};
      this.style = {};
      this.attributes = new Map();
      this.hidden = false;
      this.isConnected = true;
      this.offsetWidth = 120;
      this.offsetHeight = 48;
    }
    append(node) { this.appended = node; }
    replaceChildren(...children) { this.children = children; }
    setAttribute(name, value) { this.attributes.set(name, String(value)); }
    getAttribute(name) { return this.attributes.get(name) ?? null; }
    hasAttribute(name) { return this.attributes.has(name); }
    removeAttribute(name) { this.attributes.delete(name); }
    closest(selector) { return selector === "[data-tooltip]" && this.dataset.tooltip ? this : null; }
    querySelector() { return null; }
    contains(node) { return node === this; }
    matches(selector) { return selector === ":focus-visible" && this.focusVisible === true; }
    getBoundingClientRect() { return { left: 100, right: 140, top: 100, bottom: 140, width: 40, height: 40 }; }
  }
  const listeners = new Map();
  const created = [];
	  const document = {
	    body: new FakeElement(),
	    documentElement: { dataset: { sidebar: "expanded" } },
	    activeElement: null,
    createElement() { const element = new FakeElement(); created.push(element); return element; },
    addEventListener(type, listener) { listeners.set(type, listener); },
  };
  const frames = [];
  const requestAnimationFrame = (callback) => { frames.push(callback); return frames.length; };
  const flushFrames = () => { while (frames.length) frames.shift()(); };
  const { setupFloatingTooltips } = compileFunctions(appScript, ["setupFloatingTooltips"], {
    document,
	    window: { innerWidth: 800, innerHeight: 600, addEventListener: () => {}, matchMedia: () => ({ matches: false }) },
    Element: FakeElement,
    Node: FakeNode,
    requestAnimationFrame,
    cancelAnimationFrame: () => {},
  });
  setupFloatingTooltips();
  const tooltip = created[0];
  const anchor = new FakeElement();
  anchor.dataset.tooltip = "测试提示";
  listeners.get("pointerover")({ target: anchor });
  flushFrames();
  assert.equal(tooltip.hidden, false);
  assert.equal(tooltip.dataset.shown, "true");
  listeners.get("pointerdown")({ target: anchor, clientX: 120, clientY: 120 });
  assert.equal(tooltip.hidden, true);
  const replacement = new FakeElement();
  replacement.dataset.tooltip = "重绘后的提示";
  document.activeElement = replacement;
  listeners.get("focusout")({ target: anchor });
  anchor.isConnected = false;
  listeners.get("pointerout")({ target: anchor, relatedTarget: null });
  listeners.get("pointerover")({ target: replacement });
  listeners.get("focusin")({ target: replacement });
  assert.equal(tooltip.hidden, true);
  listeners.get("pointermove")({ target: replacement, clientX: 121, clientY: 121 });
  assert.equal(tooltip.hidden, true);
  listeners.get("pointermove")({ target: replacement, clientX: 132, clientY: 132 });
  flushFrames();
  assert.equal(tooltip.hidden, false);
});

test("round 9 friend refreshes and redundant static icon tooltips stay disabled", () => {
  assert.match(friendsScript, /setInterval\(updateDurations, 1_000\)/);
  assert.match(functionSource(friendsScript, "setOpen"), /if \(open\)[\s\S]*void loadFriends\(\)/);
  assert.match(functionSource(script, "assetImage"), /withTooltip = false/);
  assert.doesNotMatch(script, /data-champion-position="\$\{item\.value\}"[^>]+data-tooltip=/);
  assert.match(friendsScript, /deep-legends:tooltip-hide/);
});

test("player-name tooltips are gated by the actual ellipsized text", () => {
  assert.match(appScript, /target\.scrollWidth > target\.clientWidth \+ 1 \|\| target\.scrollHeight > target\.clientHeight \+ 1/);
  assert.match(appScript, /dataset\.tooltipOverflow/);
  assert.match(appScript, /\.connection-label/);
  for (const selector of ["self", ".recent-player-name", ".match-player-name", ".participant-name"]) {
    assert.match(gameplayScript, new RegExp(`data-tooltip-overflow="${selector.replace(".", "\\.")}"`));
  }
  assert.match(gameplayScript, /class="damage-name"[^>]+data-tooltip-overflow="self"/);
  assert.match(gameplayScript, /class="live-player-name"[^>]+data-tooltip-overflow="self"/);
  assert.match(friendsScript, /data-tooltip-overflow="\.friend-game-name"/);
  assert.match(friendsStyles, /\.friend-game-name[^}]+text-overflow:\s*ellipsis/);
  assert.match(gameplayScript, /const label = \(named\?\.textContent \|\| button\.dataset\.tooltip \|\| ""\)\.trim\(\)/);
  assert.doesNotMatch(`${html}\n${appScript}\n${script}\n${gameplayScript}\n${friendsScript}`, /\stitle="|\.title\s*=/);
});

test("champion detail tooltips include static numerical metadata and player aliases", () => {
  assert.match(script, /asset\?\.costs/);
  assert.match(script, /asset\?\.cooldowns/);
  assert.match(script, /asset\?\.ranges/);
  for (const [champion, alias] of [["vayne", "uzi"], ["ryze", "faker"], ["aatrox", "theshy"], ["drmundo", "bin"]]) {
    assert.match(backend, new RegExp(`"${champion}"[^\\n]+"${alias}"`));
  }
  assert.match(script, /bestScore === 100 \? item\.score === 100/);
});

test("network preload excludes Hexdata and starts only ranked and arena work", () => {
  assert.match(script, /function beginStartupPreload\(\)/);
  assert.match(script, /beginStartupPreload\(\);[\s\S]*\}\)\(\);/s);
  assert.match(script, /preload-ranked/);
  assert.doesNotMatch(script, /preload-(?:aram|mayhem)/);
  assert.match(script, /preload-arena/);
  assert.match(script, /if \(hydrated\) \{\s*state\.error = "";\s*stampRankings\(\);\s*renderLoadedWorkspace\(\);\s*return;/s);
  assert.match(script, /state\.preloaded\.arena/);
});

test("champion data survives section switches and re-entry renders instantly while fresh", () => {
  assert.match(script, /function rankingsFresh\(\)/);
	  assert.match(script, /if \(rankingsFresh\(\)\) \{ renderLoadedWorkspace\(\); return; \}/);
	  assert.match(script, /tier: normalizeTier\(readSetting\("champion-tier", "emerald_plus"\)\)/);
	  assert.match(script, /writeSetting\("champion-tier", tier\)/);
	  assert.doesNotMatch(script, /resetTransientChampionState\(\{ restorePosition: true \}\);[\s\S]{0,120}state\.tier = "emerald_plus";/);
	  assert.doesNotMatch(script, /resetTransientChampionState\(\{ restorePosition: true \}\);[\s\S]{0,120}state\.mode = "ranked";/);
});

test("champion mode, searches, and tabs persist while transient detail state resets", () => {
	for (const [field, key] of [
		["augmentQuery", "champion-augment-query"],
		["mayhemView", "champion-mayhem-view"], ["arenaFirstTab", "champion-arena-first-tab"], ["runePage", "champion-rune-page"],
	]) {
		assert.match(script, new RegExp(`${field}: normalize[A-Za-z]+\\(readSetting\\("${key}"`));
		assert.match(script, new RegExp(`writeSetting\\("${key}"`));
	}
	const resetStart = script.indexOf("function resetTransientChampionState");
	const reset = script.slice(resetStart, script.indexOf('root.addEventListener("click"', resetStart));
	assert.doesNotMatch(reset, /state\.(?:query|augmentQuery|mayhemView)\s*=/);
	assert.match(reset, /state\.detail = null/);
	assert.match(reset, /state\.selected = null/);
	assert.match(reset, /state\.workspaceRequestToken \+= 1/);
	assert.ok(script.includes("detailChampionID: normalizeChampionDetailID(readSetting(`champion-detail-id-${initialMode}`, \"\"))"));
	assert.doesNotMatch(script, /readSetting\("champion-(?:loading|error|request-token)/);
});

test("ranked detail always keeps matchup and top-player cards", () => {
	assert.match(script, /class="counter-empty"/);
	assert.match(script, /该段位暂无足够对线样本/);
  assert.match(script, /暂时没有该英雄的高场次玩家样本/);
  assert.match(script, /detail\.countersTier \|\| detail\.sampleTier/);
	assert.match(styles, /\.counter-row/);
});

test("watch settings replace the legacy convenience controls and frontend writer", () => {
  assert.doesNotMatch(html, /id="setting-auto-accept"/);
  assert.doesNotMatch(html, /id="setting-auto-play-again"/);
  assert.doesNotMatch(html, /id="setting-auto-reconnect"/);
  assert.match(html, /data-navigate-suite="watch"/);
  assert.doesNotMatch(gameplayScript, /\/api\/gameplay\/convenience/);
  assert.doesNotMatch(gameplayScript, /function bindConvenienceSettings\(\)/);
  assert.match(appScript, /convenience:accept/);
  assert.match(appScript, /已自动接受对局/);
});

test("disconnected collection, account, live and pool pages hide filters", () => {
  assert.match(appScript, /overviewTabIsCurrent/);
	assert.match(appScript, /accountStatus:/);
  assert.match(appScript, /poolHeading:/);
  assert.match(appScript, /登录国服客户端并进入大厅后，这里会按类别展示战利品/);
  assert.match(gameplayScript, /登录国服客户端并进入英雄选择或对局后/);
	assert.match(appStyles, /#favorites-account-panel > \.account-status-row\[hidden\] \+ #account-content\s*\{[^}]*padding-top:\s*0/s);
  assert.match(appStyles, /scrollbar-gutter:\s*stable/);
});

test("account page removes the duplicate title and sizes facts to unwrapped content", () => {
	assert.doesNotMatch(html, /<h2>账户与物品<\/h2>/);
	assert.match(html, /id="account-live-state"/);
	assert.match(appStyles, /\.account-hero\s*\{[^}]*grid-template-columns:\s*72px minmax\(180px,1fr\) max-content/s);
	assert.match(appStyles, /\.account-facts\s*\{[^}]*width:\s*max-content[^}]*grid-template-columns:\s*repeat\(2,max-content\)/s);
	assert.match(appStyles, /\.account-facts dd\s*\{[^}]*white-space:\s*nowrap/s);
	assert.doesNotMatch(appStyles, /\.account-facts\s*\{[^}]*grid-template-columns:\s*1fr/s);
});

test("optional themes keep a neutral background and accent-only primary", () => {
	assert.match(html, /自动主题随时段切换/);
	assert.doesNotMatch(html, /浅色与深色之间切换/);
  assert.match(html, /option value="crimson">血月红<\/option>/);
  assert.match(html, /option value="aurora">极光青<\/option>/);
  assert.match(appScript, /"crimson", "aurora"/);
  assert.match(appStyles, /data-theme="crimson"/);
  assert.match(appStyles, /data-theme="aurora"/);
});

// 图鉴详情：说明必须是海克斯本身的效果文案。hexdata 页面的 <meta name="description">
// 是"…海克斯大乱斗胜率 53.2%，选取率 0.4%…"这类统计摘要，被当成说明展示过。
test("mayhem atlas description comes from the guide paragraph, not the SEO blurb", () => {
  assert.match(hexdataBackend, /Description: hexdataAugmentGuideDescription\(document\)/);
  assert.match(hexdataBackend, /func hexdataAugmentGuideDescription\(/);
  assert.doesNotMatch(hexdataBackend, /func hexdataPageDescription\(/);
  const guide = goFunctionSource(hexdataBackend, "hexdataAugmentGuideDescription");
  assert.match(guide, /这类高分高样本英雄上考虑。/);
  assert.match(guide, /如果你的英雄机制/);
  const description = functionSource(script, "mayhemAtlasDescription");
  assert.match(description, /state\.mayhemAugmentDetail/);
  assert.match(description, /AUGMENT_DESCRIPTION_PENDING/);
  assert.match(description, /上游未提供这个海克斯的说明。/);
  assert.match(script, /class="mayhem-atlas-description">\$\{escapeHTML\(mayhemAtlasDescription\(item\)\)\}/);
});

// 左右两列必须共用同一个高度上限并各自滚动：右侧 12 位适配英雄比左侧列表长时，
// sticky 定位让它溢出并盖住下方的品质概率面板。
test("mayhem atlas columns share one height cap and scroll independently", () => {
  assert.match(styles, /\.mayhem-atlas\s*\{[^}]*--atlas-column-height:/s);
  assert.match(styles, /\.mayhem-atlas-browser,\s*\.mayhem-atlas-detail\s*\{[^}]*max-height:\s*var\(--atlas-column-height\)/s);
  assert.doesNotMatch(styles, /\.mayhem-atlas-detail\s*\{[^}]*position:\s*sticky/s);
  assert.match(styles, /\.mayhem-atlas-list\s*\{[^}]*min-height:\s*0[^}]*overflow-y:\s*auto/s);
  assert.match(styles, /\.mayhem-fit-list\s*\{[^}]*min-height:\s*0[^}]*overflow-y:\s*auto/s);
  assert.match(styles, /\.mayhem-atlas-detail-body\s*\{[^}]*min-height:\s*0/s);
  assert.match(script, /<div class="mayhem-atlas-detail-body">\$\{body\}<\/div>/);
});

// 用户明确要求：品质概率拿不到就别展示，不要留一个只有报错文案的空壳。
test("mayhem rarity panel disappears when the read fails", () => {
  const panel = functionSource(script, "renderMayhemRarityPanel");
  assert.match(panel, /if \(state\.mayhemRarityError && !stages\.length\) return "";/);
  assert.doesNotMatch(panel, /data-load-mayhem-rarity>重试/);
});

// 海克斯 ID 在整份目录里唯一，按图标目录（/Cherry/ 与 /Kiwi/）过滤不是模式判据：
// 370 个海斗海克斯里 217 个是 /Cherry/ 图标，过滤后名称退化成 "1022" 这样的裸 ID。
test("augment metadata is looked up by ID without an icon-directory filter", () => {
  assert.doesNotMatch(structuredBackend, /filterGameplayAugmentsForMode/);
  assert.doesNotMatch(structuredBackend, /gameplayAugmentIndexForMode/);
  assert.doesNotMatch(hexdataBackend, /gameplayAugmentIndexForMode/);
  assert.doesNotMatch(backend, /gameplayAugmentIndexForMode/);
  assert.match(hexdataBackend, /byID := gameplayAugmentIndexAll\(catalog\)/);
  assert.match(backend, /byID := gameplayAugmentIndexAll\(catalog\)/);
});

// 海克斯的 _large.png 全彩大图只存在于游戏侧资源里；连着客户端时逐个 404，
// 前端只能回落到白描 mask。mask 底下再垫一层品质渐变，整块就成了纯色方块。
test("augment icons survive a client asset miss and never render as a solid tile", () => {
  const image = goFunctionSource(mainSource, "handleImage");
  const candidates = goFunctionSource(mainSource, "communityDragonImagePaths");
  assert.match(image, /if err != nil \{[\s\S]{0,400}a\.serveCommunityDragonImage\(w, r, assetPath\)/);
  assert.doesNotMatch(image, /if err != nil \{\s*http\.NotFound\(w, r\)/);
  assert.match(mainSource, /"\/latest\/game\/" \+ gameRelative/);
  assert.ok(candidates.indexOf("gamePath,") < candidates.indexOf("pluginPath,"), "game _large candidate must precede plugin _large");
  assert.ok(candidates.indexOf("pluginPath,") < candidates.indexOf('strings.TrimSuffix(gamePath, "_large.png") + "_small.png"'), "every _large candidate must precede every _small candidate");
  assert.doesNotMatch(script, /isColoredAugment/);
});

// 斗魂说明：@f1@/@f2@ 是客户端本局计数器，dataValues 里查不到的变量真值在
// calculations 公式里。两者都不处理时会留下"这个回合的伤害提升："这样的断句。
test("arena augment descriptions drop runtime counters and render calculations", () => {
  assert.match(structuredBackend, /arenaSpellReferencePattern\s*=\s*regexp\.MustCompile\(`@\[A-Za-z\]\[A-Za-z0-9_\.: \*\]\{0,80\}@`\)/);
  assert.match(structuredBackend, /func arenaCalculationText\(/);
  assert.match(structuredBackend, /func arenaDropUnresolvedSentences\(/);
  assert.match(structuredBackend, /Calculations map\[string\]json\.RawMessage `json:"calculations"`/);
  assert.match(structuredBackend, /renderArenaAugmentDescription\(item\.Desc, item\.DataValues, item\.Calculations\)/);
  const render = goFunctionSource(structuredBackend, "renderArenaAugmentDescription");
  assert.match(render, /arenaSpellReferencePattern\.ReplaceAllString\(value, arenaUnresolvedMarker\)/);
  assert.match(structuredBackend, /12:\s*"最大生命值"/);
});

// 客户端那份 cherry-augments.json 也没有 description 字段（CommunityDragon 的镜像
// 就是客户端原文件），所以"启动客户端就能看说明"是假承诺。
test("mayhem augments without upstream copy do not promise the client has it", () => {
  assert.doesNotMatch(backend, /启动 LOL 客户端后可查看说明/);
  assert.match(backend, /const augmentOfflineDescription = "海克斯图鉴中可读取说明"/);
  assert.match(script, /const AUGMENT_DESCRIPTION_PENDING = "海克斯图鉴中可读取说明"/);
});

test("specialist rune loading never blocks the outer OPGG rune panel", () => {
	let pending = false;
	const { recommendationPanelBusy } = compileFunctions(gameplayScript, ["recommendationPanelBusy"], {
		liveRecommendationFlightActive: () => pending,
		specialistRuneFlightActive: () => { throw new Error("specialist flight must stay inside its own tab"); },
	});
	const target = { key: "64:top" };
	assert.equal(recommendationPanelBusy("runes", target, {}), false);
	pending = true;
	assert.equal(recommendationPanelBusy("runes", target, {}), true);
	assert.equal(recommendationPanelBusy("runes", target, { runes: { opgg: [] } }), false);
	assert.doesNotMatch(functionSource(gameplayScript, "recommendationPanelBusy"), /specialistRuneFlightActive/);
});

test("live session shows the current position only when position capability exists", () => {
	const { liveCurrentPositionChip } = compileFunctions(gameplayScript, ["liveCurrentPositionChip"], {
		liveRecommendationsFor: (data) => data.recommendations || {},
		liveRecommendationTarget: (data) => ({ position: "mid", clientPosition: data.players?.find((player) => player.isCurrent)?.position || "other" }),
		specialistPosition: () => "mid",
		livePositionDisplay: (value) => value,
		positionIcon: (value) => `<icon data-position="${value}"></icon>`,
		positionLabel: (value) => ({ top: "上路", mid: "中路" }[value] || value),
		escapeHTML: (value) => String(value ?? ""),
	});
	const visible = liveCurrentPositionChip({ players: [{ isCurrent: true, position: "top" }], recommendations: { positions: [{ position: "top" }], resolvedPosition: "mid" } });
	assert.match(visible, /class="live-current-position"/);
	assert.match(visible, /当前位置：上路/);
	assert.doesNotMatch(visible, /当前位置：中路/);
	assert.equal(liveCurrentPositionChip({ players: [], recommendations: { positions: [] } }), "");
	assert.doesNotMatch(functionSource(gameplayScript, "liveCurrentPositionChip"), /queueId|420|440/);
	assert.match(functionSource(gameplayScript, "renderSessionSummary"), /liveCurrentPositionChip\(data\)/);
	assert.match(gameplayStyles, /\.live-current-position\s*\{[^}]*border-radius:\s*999px[^}]*font-weight:\s*800/s);
});

test("live position choices sort by role share and derive missing shares from play", () => {
	let recommendation = {
		resolvedPosition: "support", positionSource: "opgg-primary",
		positions: [{ position: "top", roleRate: 0, play: 20 }, { position: "support", roleRate: 0, play: 80 }],
	};
	const { renderChampionRecommendationHeader } = compileFunctions(gameplayScript, ["renderChampionRecommendationHeader"], {
		state: { live: {} },
		escapeHTML: (value) => String(value ?? ""),
		liveRecommendationChampionId: () => 64,
		liveRecommendationTarget: () => ({ position: "support", clientPosition: "top", positionOverride: false }),
		liveRecommendationsFor: () => recommendation,
		livePositionValue: (value) => String(value || ""),
		livePositionDisplay: (value) => String(value || ""),
		positionLabel: (value) => String(value || ""),
		positionIcon: () => "",
		iconFigure: () => "",
		rate: (value) => `${Math.round(value)}%`,
		percent: (value) => `${value}%`,
		number: (value) => String(value),
		liveAugmentRecommendationSource: () => "",
	});
	const markup = renderChampionRecommendationHeader({ emptyReason: "暂无" }, { championName: "李青", position: "top" }, {});
	assert.ok(markup.indexOf('data-live-position="support"') < markup.indexOf('data-live-position="top"'));
	assert.match(markup, /live-position-rate">80%/);
	assert.match(markup, /live-position-rate">20%/);
	assert.match(markup, /已按80% 场次占比/);
	recommendation = {
		resolvedPosition: "support", positionSource: "requested",
		positions: [{ position: "mid", roleRate: 95 }, { position: "support", roleRate: 5 }],
	};
	const nicheMarkup = renderChampionRecommendationHeader({ emptyReason: "暂无" }, { championName: "瑞兹", position: "support" }, {});
	assert.match(nicheMarkup, /当前展示的是support数据（该英雄此分路占比 5%）/);
	assert.match(gameplayStyles, /@container recommendation-area \(max-width: 700px\)[\s\S]*?\.live-position-rate\s*\{[^}]*display:\s*none/s);
});

test("live build recommendations use a wide core column and only the available depth columns", () => {
  const { renderBuildRecommendation } = compileFunctions(gameplayScript, ["renderBuildRecommendation"], {
    LIVE_CORE_OPTION_LIMIT: 5,
    state: {},
    escapeHTML: (value) => String(value ?? ""),
    recommendationCapabilities: (data) => ({ hasAugments: false, hasItemDepths: data?.hasItemDepths === true }),
    liveAugmentRecommendationSource: () => "",
    liveRecommendationsFor: () => ({}),
    recommendationEmptyPanel: (title) => `<empty>${title}</empty>`,
    renderConfigOption: (option, kind) => `<option data-kind="${kind}" data-id="${option.id}"></option>`,
    buildItemSetPayload: () => ({ blocks: [] }),
    liveRecommendationChampionId: () => 1,
    positionLabel: () => "中路",
    renderRecommendationStats: () => "",
    renderSkillPlan: () => "",
    renderCoreStats: () => "",
    renderDepthStats: () => "",
  });
  const build = {
    coreOptions: Array.from({ length: 6 }, (_, index) => ({ id: `core-${index}`, stats: { winRate: 60 - index } })),
    fourthOptions: [{ id: "fourth", gamesUnavailable: true, stats: { winRate: 59 } }],
    fifthOptions: [{ id: "fifth", stats: { winRate: 58 } }],
    sixthOptions: [{ id: "sixth", stats: { winRate: 57 } }],
    itemSource: "OP.GG",
    itemWindow: "当前版本",
    itemChainStatus: "ready",
    fourthSample: 802,
    fifthSample: 20,
    sixthSample: 12,
  };
  const markup = renderBuildRecommendation(build, { championName: "瑞兹" }, [], { phase: "InProgress", hasItemDepths: true });
  assert.match(markup, /<div class="build-core-ranking-row"><div class="item-build-layout">/);
  const layoutStart = markup.indexOf('<div class="item-build-layout">');
  const coreStart = markup.indexOf('class="item-core-column"', layoutStart);
  const depthStart = markup.indexOf('class="item-depth-columns"', layoutStart);
  assert.ok(layoutStart >= 0 && coreStart > layoutStart && depthStart > coreStart);
  assert.match(markup, /build-item-row" data-depth-count="3"/);
  assert.doesNotMatch(markup, /OP\.GG · 当前版本/);
  assert.match(markup, /item-depth-columns"><section><h4><span>第四件<\/span>[\s\S]*<h4><span>第五件<\/span>[\s\S]*<h4><span>第六件<\/span>/);
	assert.match(markup, /上游未提供样本量，按上游推荐顺序展示/);
	assert.doesNotMatch(markup, /样本少/);
	  assert.equal((markup.match(/data-kind="route"/g) || []).length, 5);
	  assert.match(markup, /data-apply-item-set/);
  assert.doesNotMatch(markup, /item-depth-grid/);
  // 上游没有第六件（辅助位普遍如此）就整列不展示，而不是留一列空态文案。
  const missingSixth = renderBuildRecommendation({ ...build, sixthOptions: [], sixthSample: 0 }, { championName: "瑞兹" }, [], { phase: "InProgress", hasItemDepths: true });
  assert.match(missingSixth, /build-item-row" data-depth-count="2"/);
  assert.match(missingSixth, /data-id="fourth"/);
  assert.match(missingSixth, /data-id="fifth"/);
  assert.doesNotMatch(missingSixth, /data-id="sixth"/);
  assert.doesNotMatch(missingSixth, /第六件/);
  // 第四/第五件缺失才是真正的「暂不可用」，空态文案必须保留。
  const missingFifth = renderBuildRecommendation({ ...build, fifthOptions: [], fifthSample: 0 }, { championName: "瑞兹" }, [], { phase: "InProgress", hasItemDepths: true });
  assert.match(missingFifth, /<h4><span>第五件<\/span><\/h4><p class="build-group-empty">该阶段暂无可用样本<\/p>/);
  const unsupportedDepths = renderBuildRecommendation(build, { championName: "瑞兹" }, [], { phase: "InProgress", hasItemDepths: false });
  assert.doesNotMatch(unsupportedDepths, /第四件|第五件|第六件|item-depth-columns/);
  assert.match(unsupportedDepths, /item-core-column/);
  assert.doesNotMatch(renderBuildRecommendation({ coreOptions: [{ id: "core" }] }, { championName: "瑞兹" }, [], { phase: "InProgress" }), /item-depth-columns/);
  assert.match(sharedBuildStyles, /\.build-item-row\s*\{[^}]*grid-template-columns:\s*minmax\(0,1\.6fr\) repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(sharedBuildStyles, /\.build-item-row\[data-depth-count="2"\]\s*\{[^}]*repeat\(2,minmax\(0,1fr\)\)/s);
  assert.match(sharedBuildStyles, /\.build-item-row\[data-depth-count="1"\]/);
  assert.match(sharedBuildStyles, /\.build-item-row\s*>\s*\.item-depth-columns\s*\{[^}]*display:\s*contents/s);
  assert.match(sharedBuildStyles, /\.build-item-row\s*\{[^}]*--build-row-height:\s*64px/s);
	assert.match(gameplayStyles, /\.build-core-ranking-row\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
  const wideRanking = cssBlockAfter(gameplayStyles, "@container recommendation-area (min-width: 1081px)");
  assert.match(wideRanking, /\.build-core-ranking-row \.build-item-row\[data-depth-count="3"\]\s*\{[^}]*grid-template-columns:\s*310px repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(wideRanking, /\.build-core-ranking-row \.option-stats\.is-depth\s*\{[^}]*repeat\(2,minmax\(48px,64px\)\)[^}]*column-gap:\s*0/s);
	assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list\s*\{[^}]*grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/s);
  assert.match(gameplayStyles, /\.config-item\s*\{[^}]*width:\s*var\(--option-icon-size,42px\)/s);
  assert.doesNotMatch(gameplayStyles, /\.item-option-name\s*\{/);
  assert.doesNotMatch(functionSource(gameplayScript, "renderConfigOption"), /item-option-name/);
  const compact = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 1080px)");
  assert.match(compact, /\.build-core-ranking-row\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
  const medium = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 900px)");
  assert.match(medium, /\.live-item-ranking \.mayhem-ranking-list\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
  assert.doesNotMatch(gameplayStyles, /\.item-build-layout\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
  assert.doesNotMatch(gameplayStyles, /\.item-depth-grid\s*\{/);
});

test("live rune source tabs keep stable widths when the contextual note appears", () => {
	assert.match(gameplayStyles, /\.rune-source-tab-buttons\s*\{[^}]*flex:\s*1 1 auto/s);
	assert.match(gameplayStyles, /\.rune-source-tab\s*\{[^}]*flex:\s*1 1 0[^}]*min-width:\s*0/s);
	assert.match(gameplayStyles, /\.rune-source-note\s*\{[^}]*margin:\s*0 8px 2px[^}]*text-overflow:\s*ellipsis/s);
  const narrow = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 700px)");
  assert.match(narrow, /\.rune-source-note\s*\{[^}]*display:\s*none/s);
});

test("disabled controls use the not-allowed cursor throughout the web UI", () => {
  for (const styles of [appStyles, gameplayStyles, friendsStyles]) {
    assert.doesNotMatch(styles, /(?:\[disabled\]|:disabled)[^{}]*\{[^}]*cursor:\s*(?:wait|default)/s);
  }
  assert.match(appStyles, /button:disabled, select:disabled, \[aria-disabled="true"\]\s*\{[^}]*cursor:\s*not-allowed/s);
});

test("arena live build aligns prism and core cards with the three Arena metrics", () => {
  const state = { items: { items: [{ id: 447101, name: "棱彩测试装备" }] } };
  const { renderBuildRecommendation } = compileFunctions(gameplayScript, ["renderArenaBuildOption", "renderBuildRecommendation"], {
    LIVE_CORE_OPTION_LIMIT: 5,
    state,
    escapeHTML: (value) => String(value ?? ""),
    rate: (value) => value == null ? "—" : `${value}%`,
    renderItemIcon: (id) => `<item data-id="${id}"></item>`,
    recommendationCapabilities: () => ({ hasAugments: true }),
    isARAMRelatedMatch: () => false,
    liveAugmentRecommendationSource: () => "arena",
    liveRecommendationsFor: () => ({}),
    recommendationEmptyPanel: (title) => `<empty>${title}</empty>`,
    renderConfigOption: () => { throw new Error("Arena item cards must not use the ranked option row"); },
    buildItemSetPayload: () => ({ blocks: [] }),
    liveRecommendationChampionId: () => 1,
    positionLabel: () => "中路",
  });
  const stats = { winRate: 63.6, averagePlacement: 2.88, firstPlaceRate: 25.1, games: 1055 };
  const markup = renderBuildRecommendation({
    coreOptions: [{ ids: [6630, 3071, 3053], grade: "A", stats }],
    prismOptions: [{ ids: [447101], grade: "B", stats }],
    fourthOptions: [{ ids: [3026], stats }],
    fifthOptions: [{ ids: [3065], stats }],
    sixthOptions: [{ ids: [3089], stats }],
    itemChainStatus: "ready",
  }, { championName: "瑞兹" }, [], { phase: "InProgress" });
  assert.match(markup, /build-recommendation is-arena-build/);
  assert.match(markup, /class="live-arena-build-section is-prismatic"[\s\S]*棱彩测试装备/);
  assert.match(markup, /class="live-arena-build-section is-core"[\s\S]*data-id="6630"[\s\S]*data-id="3071"[\s\S]*data-id="3053"/);
  assert.ok(markup.indexOf('class="live-arena-build-section is-prismatic"') < markup.indexOf('class="live-arena-build-section is-core"'));
  assert.match(markup, /class="augment-grade is-A">A<\/b>/);
  assert.match(markup, /class="augment-grade is-B">B<\/b>/);
  assert.equal((markup.match(/data-metric-count="3"/g) || []).length, 2);
  for (const label of ["胜率", "平均名次", "吃鸡率"]) assert.match(markup, new RegExp(`<dt>${label}<\\/dt>`));
  assert.match(markup, /<dd>63\.60%<\/dd>[\s\S]*<dd>2\.88<\/dd>[\s\S]*<dd>25\.10%<\/dd>/);
  assert.doesNotMatch(markup, /第四件|第五件|第六件|综合评分|<dt>样本<\/dt>|item-depth-columns/);
	  assert.match(gameplayStyles, /\.live-arena-build-section\.is-core \.live-arena-build-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3,minmax\(0,1fr\)\)/s);
	  assert.match(gameplayStyles, /\.live-arena-build-section\.is-prismatic \.live-arena-build-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3,minmax\(0,1fr\)\)/s);
	  const compactArenaBuild = cssBlockAfter(gameplayStyles, "@container (max-width: 820px)");
	  assert.match(compactArenaBuild, /\.live-arena-build-section\.is-core \.live-arena-build-grid,[\s\S]*\.live-arena-build-section\.is-prismatic \.live-arena-build-grid\s*\{[^}]*grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/s);
  assert.match(gameplayStyles, /\.live-arena-build-option \.item-option-button > \.game-icon\.is-large\s*\{[^}]*width:\s*48px[^}]*height:\s*48px[^}]*border-radius:\s*7px/s);
	assert.match(gameplayStyles, /\.live-arena-build-option \.arena-option-main > strong\s*\{[^}]*font-size:\s*13px/s);
	assert.match(gameplayStyles, /\.live-arena-build-option\.arena-option-card dl\s*\{[^}]*column-gap:\s*0/s);
	assert.match(gameplayStyles, /\.live-arena-build-option\.arena-option-card dt\s*\{[^}]*font-size:\s*9px/s);
	assert.match(gameplayStyles, /\.live-arena-build-option\.arena-option-card dd\s*\{[^}]*font-size:\s*12px/s);
	  const narrowArenaBuild = cssBlockAfter(gameplayStyles, "@container build-recommendation (max-width: 500px)");
	  assert.match(narrowArenaBuild, /\.live-arena-build-section\.is-core \.arena-option-card\s*\{[^}]*padding:\s*5px/s);
	  assert.match(narrowArenaBuild, /\.live-arena-build-section\.is-core \.arena-option-card \.item-option-button,[\s\S]*\.item-option-button > \.game-icon\.is-large\s*\{[^}]*width:\s*28px[^}]*height:\s*28px[^}]*flex-basis:\s*28px/s);
	  assert.match(narrowArenaBuild, /\.live-arena-build-section\.is-core \.arena-option-card \.augment-grade\s*\{[^}]*width:\s*18px[^}]*height:\s*18px[^}]*flex-basis:\s*18px/s);
});

test("live core routes render at most five rows without an expansion control", () => {
  const { renderBuildRecommendation } = compileFunctions(gameplayScript, ["renderBuildRecommendation"], {
    LIVE_CORE_OPTION_LIMIT: 5,
    state: {},
    escapeHTML: (value) => String(value ?? ""),
    recommendationCapabilities: (data) => ({ hasAugments: Boolean(data?.hasAugments) }),
    liveAugmentRecommendationSource: (data) => data?.hasAugments ? "arena" : "",
    liveRecommendationsFor: () => ({}),
    recommendationEmptyPanel: (title) => `<empty>${title}</empty>`,
    renderConfigOption: (option) => `<option data-normal-core="${option.id}"></option>`,
    renderArenaBuildOption: (option) => `<article data-arena-core="${option.id}"></article>`,
    buildItemSetPayload: () => ({ blocks: [] }),
    liveRecommendationChampionId: () => 1,
    positionLabel: () => "中路",
    renderRecommendationStats: () => "",
    renderSkillPlan: () => "",
    renderCoreStats: () => "",
    renderDepthStats: () => "",
  });
  const build = { coreOptions: Array.from({ length: 18 }, (_, index) => ({ id: index + 1 })) };
  const render = (arena) => renderBuildRecommendation(build, { championName: "瑞兹" }, [], { phase: "InProgress", hasAugments: arena });

  const standard = render(false);
  assert.equal((standard.match(/data-normal-core=/g) || []).length, 5);
  const arena = render(true);
  assert.equal((arena.match(/data-arena-core=/g) || []).length, 5);
  assert.doesNotMatch(`${standard}${arena}${gameplayStyles}`, /data-live-build-expand|live-build-expand|展开全部|收起/);
});

test("live recommendation option groups keep the upstream order and never re-sort by win rate", () => {
  const { renderBuildRecommendation } = compileFunctions(gameplayScript, ["renderBuildRecommendation"], {
    LIVE_CORE_OPTION_LIMIT: 5,
    state: {},
    escapeHTML: (value) => String(value ?? ""),
    recommendationCapabilities: () => ({ hasAugments: false }),
    liveAugmentRecommendationSource: () => "",
    liveRecommendationsFor: () => ({}),
    recommendationEmptyPanel: () => "",
    renderConfigOption: (option, kind) => `<option data-kind="${kind}" data-id="${option.id}"></option>`,
    buildItemSetPayload: () => ({ blocks: [] }),
    liveRecommendationChampionId: () => 1,
    positionLabel: () => "中路",
    renderRecommendationStats: () => "",
    renderSkillPlan: () => "",
    renderCoreStats: () => "",
    renderDepthStats: () => "",
  });
  // 真实回归场景：格雷福斯打野的召唤师技能，上游第一行是 4718 场的
  // 闪现+惩戒（48.5%），第二行是 2 场的虚弱+惩戒（50%）。按胜率排序会让
  // 那 2 场的长尾行顶掉唯一正确的推荐——用户看到的就是「打野居然没有闪现」。
  const upstream = (prefix) => [
    { id: `${prefix}-head`, stats: { pickRate: 99.96, winRate: 48.5, games: 4718 } },
    { id: `${prefix}-tail`, stats: { pickRate: 0.04, winRate: 50, games: 2 } },
    { id: `${prefix}-third`, stats: { pickRate: 0.02, winRate: 100, games: 1 } },
  ];
  const markup = renderBuildRecommendation({ spellOptions: upstream("spell"), starterOptions: upstream("starter"), bootOptions: upstream("boot") }, { championName: "瑞兹" }, [], { phase: "InProgress" });
	for (const [prefix, limit] of [["spell", 2], ["starter", 3], ["boot", 3]]) {
    const head = markup.indexOf(`data-id="${prefix}-head"`);
    const tail = markup.indexOf(`data-id="${prefix}-tail"`);
    const third = markup.indexOf(`data-id="${prefix}-third"`);
    assert.ok(head >= 0, `${prefix} dropped the upstream leading row`);
    if (limit === 1) {
      assert.ok(tail < 0 && third < 0, `${prefix} should keep only the upstream leading row`);
    } else if (limit === 2) {
      assert.ok(tail > head && third < 0, `${prefix} should keep the first two upstream rows in order`);
    } else {
      assert.ok(tail > head && third > tail, `${prefix} should keep all three upstream rows in order`);
    }
  }
  assert.doesNotMatch(gameplayScript, /byWinRate/);
});

test("mayhem opening sections follow actual data instead of the augment capability", () => {
  const { renderBuildRecommendation } = compileFunctions(gameplayScript, ["renderBuildRecommendation"], {
    LIVE_CORE_OPTION_LIMIT: 5,
    state: {},
    escapeHTML: (value) => String(value ?? ""),
    recommendationCapabilities: () => ({ hasAugments: true }),
    liveAugmentRecommendationSource: (data) =>
      String(data?.gameMode || "").toUpperCase() === "CHERRY" ? "arena" : "hextech",
    liveRecommendationsFor: () => ({}),
    recommendationEmptyPanel: () => "",
    renderConfigOption: (option, kind) => `<option data-kind="${kind}" data-id="${option.id}"></option>`,
    buildItemSetPayload: () => ({ blocks: [] }),
    liveRecommendationChampionId: () => 1,
    positionLabel: () => "",
    renderRecommendationStats: () => "",
    renderSkillPlan: () => "",
    renderCoreStats: () => "",
    renderDepthStats: () => "",
  });
	  const mayhem = renderBuildRecommendation({
	    spellOptions: [{ id: "real-spell" }], starterOptions: [{ id: "real-starter" }], bootOptions: [],
	  }, { championName: "薇恩" }, [], { phase: "InProgress" });
	  assert.match(mayhem, /build-recommendation is-mayhem-build/);
	  assert.match(mayhem, /build-summary-bar is-mayhem/);
	  assert.ok(mayhem.indexOf("build-summary-starter") < mayhem.indexOf("build-summary-spells") && mayhem.indexOf("build-summary-spells") < mayhem.indexOf("build-summary-boots"));
	  assert.match(mayhem, /build-recommendation-heading"><h3>装备推荐<\/h3>/);
  assert.match(mayhem, /build-summary-spells/);
  assert.match(mayhem, /data-id="real-spell"/);
	  assert.match(mayhem, /build-summary-starter/);
	  assert.match(mayhem, /data-id="real-starter"/);
	  assert.doesNotMatch(mayhem, /data-apply-item-set|应用装备方案/);

	  const arena = renderBuildRecommendation({ spellOptions: [], starterOptions: [], bootOptions: [{ id: "arena-boot" }], skillPriority: ["Q", "W", "E"] }, { championName: "薇恩" }, [], { phase: "InProgress", queueId: 1700, gameMode: "CHERRY", mapId: 30 });
	  assert.doesNotMatch(arena, /build-summary-spells|build-summary-starter/);
	  assert.match(arena, /build-recommendation is-arena-build/);
	  assert.match(arena, /build-summary-bar is-arena/);
	  assert.ok(arena.indexOf("build-summary-boots") < arena.indexOf("build-summary-skill"));
	  assert.match(gameplayStyles, /\.build-summary-bar\.is-arena\s*\{[^}]*grid-template-columns:\s*minmax\(220px,\.7fr\) minmax\(0,1\.3fr\)/s);
	  assert.match(gameplayStyles, /\.build-summary-bar\.is-arena > \.build-summary-skill\s*\{[^}]*display:\s*flex[^}]*flex-direction:\s*column/s);
	  assert.match(gameplayStyles, /\.build-summary-bar\.is-arena > \.build-summary-skill > \.skill-priority-row\s*\{[^}]*margin-top:\s*auto/s);
	  assert.match(gameplayStyles, /\.build-summary-bar\.is-arena > \.build-summary-skill > \.skill-order\s*\{[^}]*margin-bottom:\s*auto/s);
	  assert.match(gameplayStyles, /\.config-item\s*\{[^}]*justify-items:\s*start/s);
	});

test("item set payload uses three core routes, removes cross-group duplicates, and follows recommendation position", () => {
	const { buildItemSetPayload } = compileFunctions(gameplayScript, ["buildItemSetPayload"], {
		liveRecommendationChampionId: () => 68,
		liveRecommendationsFor: () => ({}),
		liveRecommendationTarget: () => null,
		state: { liveRecommendationTraces: new Map() },
		positionLabel: (position) => ({ top: "上路", middle: "中路" }[position] || position),
	});
	const starterIDs = Array.from({ length: 24 }, (_, index) => 1000 + index);
	const payload = buildItemSetPayload({
		position: "top",
		starterOptions: [{ ids: starterIDs }, { ids: [1000, 3340] }],
		bootOptions: [{ ids: [3006] }, { ids: [3006, 3117] }],
		coreOptions: [{ ids: [3071, 3153, 3006] }, { ids: [3153, 6333] }, { ids: [6692, 3071] }, { ids: [999999] }],
		fourthOptions: [{ ids: [3071, 3153, 3078] }],
		fifthOptions: [{ ids: [3153, 6333, 3089] }],
		sixthOptions: [{ ids: [6333, 6692] }],
		prismOptions: [{ ids: [447101, 447102] }, { ids: [447102, 447103] }],
	}, { championName: "兰博", position: "middle" }, { mapId: 11 });
	assert.equal(payload.title, "上路");
	assert.equal(payload.position, "top");
	assert.equal(payload.selfPosition, "middle");
	assert.deepEqual(payload.blocks.map((block) => block.type), ["出门装", "鞋子选择", "核心装", "后续装备", "棱彩装备"]);
	assert.equal(payload.blocks[0].items.length, 20);
	assert.deepEqual(payload.blocks[2].items.map(({ id }) => id), [3071, 3153, 3006, 6333, 6692]);
	assert.deepEqual(payload.blocks[3].items.map(({ id }) => id), [3078, 3089]);
	assert.deepEqual(payload.blocks[4].items.map(({ id }) => id), [447101, 447102, 447103]);
	assert.doesNotMatch(JSON.stringify(payload), /999999/, "fourth core route must be ignored");
	const coreSet = new Set(payload.blocks[2].items.map(({ id }) => id));
	assert.deepEqual(payload.blocks[3].items.filter(({ id }) => coreSet.has(id)), []);
	for (const block of payload.blocks) {
		assert.ok(block.items.length <= 20, `${block.type} exceeded the server limit`);
		assert.equal(new Set(block.items.map(({ id }) => id)).size, block.items.length, `${block.type} contains duplicates`);
	}
});

test("item set payload preserves recommendation trace and resolved position context", () => {
	const { buildItemSetPayload } = compileFunctions(gameplayScript, ["buildItemSetPayload"], {
		liveRecommendationChampionId: () => 64,
		liveRecommendationsFor: () => ({
			traceId: "rec-trace-1234", recommendationKey: "64:support:ranked", requestedPosition: "adc",
			resolvedPosition: "support", positionSource: "opgg-primary",
		}),
		liveRecommendationTarget: () => ({ key: "fallback-key", position: "top", tier: "emerald_plus" }),
		state: { liveRecommendationTraces: new Map([["fallback-key", "fallback-trace"]]) },
		positionLabel: (position) => ({ support: "辅助", top: "上路" }[position] || position),
	});
	const payload = buildItemSetPayload({ position: "top", coreOptions: [{ ids: [3071] }] }, { championName: "李青", position: "jungle" }, { mapId: 11, queueId: 420, gameId: 987, gameMode: "CLASSIC" });
	assert.equal(payload.traceId, "rec-trace-1234");
	assert.equal(payload.recommendationKey, "64:support:ranked");
	assert.equal(payload.requestedPosition, "adc");
	assert.equal(payload.resolvedPosition, "support");
	assert.equal(payload.positionSource, "opgg-primary");
	assert.equal(payload.position, "support");
	assert.equal(payload.title, "辅助");
	assert.equal(payload.gameId, 987);
	assert.equal(payload.queueId, 420);
});

test("item set drops an empty post-core block and reports applied group and item counts", async () => {
	const payloadFunctions = compileFunctions(gameplayScript, ["buildItemSetPayload"], {
		liveRecommendationChampionId: () => 64,
		liveRecommendationsFor: () => ({}),
		liveRecommendationTarget: () => null,
		state: { liveRecommendationTraces: new Map() },
		positionLabel: () => "中路",
	});
	const payload = payloadFunctions.buildItemSetPayload({
		position: "mid",
		coreOptions: [{ ids: [3071, 3153] }],
		fourthOptions: [{ ids: [3071] }],
		fifthOptions: [{ ids: [3153] }],
	}, { championName: "李青", position: "jungle" }, { mapId: 11 });
	assert.deepEqual(payload.blocks.map(({ type }) => type), ["核心装"]);

	const state = { live: { players: [{ isCurrent: true }] } };
	let toast = "";
	const diagnostics = [];
	const { applyItemSet } = compileFunctions(gameplayScript, ["applyItemSet"], {
		state,
		liveRecommendationsFor: () => ({ build: {} }),
		buildItemSetPayload: () => ({ traceId: "rec-trace-1234", recommendationKey: "64:middle", championId: 64, blocks: [{ items: [{ id: 1 }, { id: 2 }] }, { items: [{ id: 3 }] }] }),
		api: async () => ({ title: "DL · 李青 · 中路", traceId: "rec-trace-1234" }),
		recordItemSetClientDiagnostic: (...args) => diagnostics.push(args),
		showToast: (message) => { toast = message; },
	});
	const button = { disabled: false, textContent: "应用装备方案", dataset: {} };
	await applyItemSet({ currentTarget: button });
	assert.match(toast, /2 组 \/ 3 件/);
	assert.equal(button.textContent, "已写入");
	assert.match(button.dataset.tooltip, /无法确认游戏是否已加载/);
	assert.deepEqual(diagnostics.map(([event, reason]) => [event, reason]), [
		["item_set_apply_request", "submitted"],
		["item_set_apply_request", "succeeded"],
	]);
	assert.equal(diagnostics[1][2].traceId, "rec-trace-1234");
	assert.equal(diagnostics[1][2].blockCount, 2);
	assert.equal(diagnostics[1][2].itemCount, 3);
});

test("item set apply tooltip carries the in-game shop clipping workaround without repeating it in the toast", async () => {
	let toast = "";
	const { applyItemSet } = compileFunctions(gameplayScript, ["applyItemSet"], {
		state: { live: { players: [{ isCurrent: true }] } },
		liveRecommendationsFor: () => ({ build: {} }),
		buildItemSetPayload: () => ({ traceId: "rec-trace-9001", recommendationKey: "127:middle", championId: 127, blocks: [{ items: [{ id: 1 }, { id: 2 }] }] }),
		api: async () => ({ title: "DL · 中路" }),
		recordItemSetClientDiagnostic: () => {},
		showToast: (message) => { toast = message; },
	});
	const button = { disabled: false, textContent: "应用装备方案", dataset: {} };
	await applyItemSet({ currentTarget: button });
	// Three sources (this app, Akari, a hand-made client set) all clip on first
	// shop open, so the limitation is explained in the hover tooltip, not popped
	// up as a toast on every single apply.
	assert.doesNotMatch(toast, /点一下任意装备即可归位/);
	assert.match(button.dataset.tooltip, /点一下任意装备即可归位/);
	assert.match(button.dataset.tooltip, /客户端渲染问题/);
	assert.match(button.dataset.tooltip, /与写入内容无关/);
});

test("item set apply failure records the same trace and item counts", async () => {
	const diagnostics = [];
	let toast = "";
	const { applyItemSet } = compileFunctions(gameplayScript, ["applyItemSet"], {
		state: { live: { players: [{ isCurrent: true }] } },
		liveRecommendationsFor: () => ({ build: {} }),
		buildItemSetPayload: () => ({ traceId: "rec-trace-5678", recommendationKey: "64:middle", championId: 64, blocks: [{ items: [{ id: 1 }] }] }),
		api: async () => { throw new Error("write failed"); },
		recordItemSetClientDiagnostic: (...args) => diagnostics.push(args),
		showToast: (message) => { toast = message; },
	});
	const button = { disabled: false, textContent: "应用装备方案", dataset: {} };
	await applyItemSet({ currentTarget: button });
	assert.deepEqual(diagnostics.map(([event, reason]) => [event, reason]), [
		["item_set_apply_request", "submitted"],
		["item_set_apply_request", "failed"],
	]);
	assert.equal(diagnostics[1][2].traceId, "rec-trace-5678");
	assert.equal(diagnostics[1][2].blockCount, 1);
	assert.equal(diagnostics[1][2].itemCount, 1);
	assert.equal(toast, "write failed");
	assert.equal(button.textContent, "应用装备方案");
});

test("live mayhem item ranking renders the passed-through rows without a new request", () => {
  const { renderLiveItemRanking } = compileFunctions(gameplayScript, ["renderLiveItemRanking"], {
    renderItemIcon: (id) => `<item data-id="${id}"></item>`,
    escapeHTML: (value) => String(value ?? ""),
    number: (value) => String(value ?? ""),
    rate: (value) => `${value}%`,
    compactNumber: (value) => String(value ?? ""),
  });
  const markup = renderLiveItemRanking([
    { score: 70, games: 500, winRate: 51, assets: [{ id: 3006, name: "狂战士胫甲" }] },
    { score: 90, games: 200, winRate: 55, assets: [{ id: 126697, name: "狂妄" }] },
  ]);
  assert.ok(markup.indexOf('data-id="126697"') < markup.indexOf('data-id="3006"'));
  assert.match(markup, /装备排行/);
  assert.match(markup, /<dt>胜率<\/dt>/);
  assert.match(markup, /<dt>场次<\/dt>/);
  assert.doesNotMatch(markup, /<dt>综合评分<\/dt>|<dt>样本<\/dt>/);
  assert.doesNotMatch(markup, /按综合评分排序的高表现装备/);
  assert.match(gameplayBackend, /ItemRanking\s+\[\]championMetricRow\s+`json:"itemRanking,omitempty"`/);
  assert.match(gameplayBackend, /result\.ItemRanking = append\(\[\]championMetricRow\(nil\), detail\.ItemRanking\.\.\.\)/);
	assert.doesNotMatch(functionSource(gameplayScript, "renderLiveItemRanking"), /\bapi\(|\bfetch\(/);
});

test("live Mayhem builds keep only core equipment beside a two-column ranking", () => {
  const { renderBuildRecommendation } = compileFunctions(gameplayScript, ["renderBuildRecommendation"], {
    LIVE_CORE_OPTION_LIMIT: 5,
    state: {},
    recommendationCapabilities: () => ({ hasAugments: true }),
    liveAugmentRecommendationSource: () => "hextech",
    liveRecommendationsFor: () => ({
      itemRanking: [{ score: 90, games: 1000, winRate: 60, assets: [{ id: 3071, name: "黑色切割者" }] }],
    }),
    recommendationEmptyPanel: (title) => `<empty>${title}</empty>`,
    escapeHTML: (value) => String(value ?? ""),
    renderConfigOption: (option, kind) => `<option data-kind="${kind}" data-id="${option.id}"></option>`,
    buildItemSetPayload: () => ({ blocks: [] }),
    liveRecommendationChampionId: () => 1,
    positionLabel: () => "其他",
    renderRecommendationStats: () => "",
    renderSkillPlan: () => "",
    renderCoreStats: () => "",
    renderDepthStats: () => "",
    renderLiveItemRanking: () => '<section class="recommendation-section live-item-ranking"><div class="mayhem-ranking-list"><article>排行</article><article>排行</article></div></section>',
  });
  const markup = renderBuildRecommendation({
    coreOptions: [{ id: "core", stats: { winRate: 60 } }],
    fourthOptions: [{ id: "fourth" }],
    fifthOptions: [{ id: "fifth" }],
    sixthOptions: [{ id: "sixth" }],
    spellOptions: [{ ids: [4, 32] }],
    itemChainStatus: "ready",
  }, { championName: "卡蜜尔" }, [], { phase: "ChampSelect" });
  assert.match(markup, /build-core-ranking-row is-mayhem-build-row/);
  assert.match(markup, /data-kind="route" data-id="core"/);
  assert.doesNotMatch(markup, /第四件|第五件|第六件|data-id="fourth"|data-id="fifth"|data-id="sixth"/);
  assert.match(markup, /live-item-ranking/);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list article\s*\{[^}]*grid-template-columns:\s*20px 42px minmax\(82px,1fr\) max-content/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list\s*\{[^}]*gap:\s*6px;[^}]*padding:\s*0 4px 12px/s);
  assert.match(gameplayStyles, /\.live-item-ranking\.recommendation-section > header\s*\{[^}]*border-bottom:\s*0/s);
  assert.match(gameplayStyles, /\.live-item-ranking > header h3\s*\{[^}]*font-size:\s*13px;[^}]*line-height:\s*18px/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list article\s*\{[^}]*min-height:\s*64px;[^}]*background:\s*color-mix\(in oklab,var\(--bg\) 55%,transparent\);[^}]*border:\s*0;[^}]*border-radius:\s*8px/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list article:nth-child\(odd\)\s*\{[^}]*border:\s*0/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list article > strong\s*\{[^}]*font-size:\s*13px/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list article dl\s*\{[^}]*grid-column:\s*auto;[^}]*repeat\(2,minmax\(48px,64px\)\)[^}]*column-gap:\s*0;[^}]*white-space:\s*nowrap/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list dt\s*\{[^}]*font-size:\s*9px/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list dd\s*\{[^}]*font-size:\s*12px/s);
  assert.match(gameplayStyles, /\.live-item-ranking \.mayhem-ranking-list dl > div:last-child dd\s*\{[^}]*color:\s*var\(--muted\)/s);
  assert.doesNotMatch(styles, /\.mayhem-ranking-list dl > div\s*\{[^}]*border-left/s);
  const wideMayhemBuild = cssBlockAfter(gameplayStyles, "@container recommendation-area (min-width: 1081px)");
  assert.match(wideMayhemBuild, /\.build-core-ranking-row\.is-mayhem-build-row\s*\{[^}]*grid-template-columns:\s*minmax\(0,\.75fr\) minmax\(0,1\.25fr\)/s);
  assert.match(wideMayhemBuild, /\.build-core-ranking-row\.is-mayhem-build-row > \.live-item-ranking\s*\{[^}]*align-self:\s*stretch;[^}]*border-left:\s*1px solid var\(--line\)/s);
  assert.match(wideMayhemBuild, /\.build-core-ranking-row\.is-mayhem-build-row > \.live-item-ranking > header\s*\{[^}]*min-height:\s*41px;[^}]*padding-block:\s*8px/s);
  assert.match(wideMayhemBuild, /\.build-core-ranking-row \.option-stats dt\s*\{[^}]*font-size:\s*9px/s);
  assert.match(wideMayhemBuild, /\.build-core-ranking-row \.option-stats dd\s*\{[^}]*font-size:\s*12px/s);
});

test("overview re-entry keeps its pages for the two minute freshness window", () => {
  const { shouldReloadOverview } = compileFunctions(gameplayScript, ["shouldReloadOverview"]);
  const now = 1_000_000;
  assert.equal(shouldReloadOverview({ data: {}, loadedAt: now - 25_000 }, now), false);
  assert.equal(shouldReloadOverview({ data: {}, loadedAt: now - 120_000 }, now), true);
  assert.equal(shouldReloadOverview({ data: null, loadedAt: now - 1 }, now), true);
  const activateSource = functionSource(gameplayScript, "activateSection");
  assert.match(activateSource, /shouldReloadOverview\(tab\)/);
  assert.match(activateSource, /loadOverview\(tab, true\)/);
});

test("R63 forced overview refresh merges its first page into already loaded pages", async () => {
	const oldMatches = Array.from({ length: 60 }, (_, index) => ({ gameId: index + 1, marker: "old" }));
	const tab = {
		key: "player-1", current: false, playerRef: "ref", matchFilter: "all", nextBegIndex: 60,
		data: { player: { playerRef: "ref" }, matches: oldMatches, pagination: { begIndex: 40, count: 20, hasMore: true } },
	};
	const state = { controllers: new Map(), settings: { matchCount: 20 }, lastCapabilities: [] };
	const { loadOverview } = compileFunctions(gameplayScript, ["loadOverview"], {
		state,
		loadOPGGSeasonSummary: async () => false,
    loadOverviewCurrentGame: async () => false,
		tabGroup: () => "players",
		tabReady: () => true,
		rerenderTab: () => {},
		showLoadingMoreState: () => {},
		api: async () => ({ player: { playerRef: "ref" }, matches: [{ gameId: 1, marker: "fresh" }, { gameId: 61, marker: "fresh" }], pagination: { begIndex: 0, count: 20, hasMore: true } }),
		normalizedPagination: (payload) => ({ ...payload.pagination, nextBegIndex: 20 }),
		rememberTabPlayerRef: () => {},
		playerLabel: () => "Player",
		renderCapabilitySettings: () => {},
		showToast: () => {},
		appendOverviewMatches: () => {},
		MAX_BROWSE_MATCHES: 1000,
		AUTO_PAGE_DELAY_MS: 400,
		AUTO_PAGE_MAX_BACKOFF_MS: 8000,
	});
	assert.equal(await loadOverview(tab, true, false, false, true), true);
	assert.equal(tab.data.matches.length, 61);
	assert.equal(tab.data.matches[0].marker, "fresh");
	assert.equal(tab.data.matches.filter((match) => match.gameId === 1).length, 1);
	assert.equal(tab.nextBegIndex, 60);
});

test("R63 match-tier scope ignores transient tab keys", () => {
	const { matchTierScope } = compileFunctions(gameplayScript, ["matchTierScope"], {
		riotTab: () => false,
		tabServerID: () => "HN1",
	});
	const player = { playerRef: "public-ref" };
	assert.equal(matchTierScope({ key: "tab-a", data: { player } }), matchTierScope({ key: "tab-b", data: { player } }));
});

test("R61 append uses an independent controller and queues timed revalidation", async () => {
	const tab = { key: "player-1", loadingMore: true, data: { pagination: { hasMore: true } }, openMatches: new Set(), matchDetailTabs: new Map() };
	const state = { section: "champions", liveTimer: 0, tabs: [tab], settings: { defaultMatchFilter: "all" } };
	const reloads = [];
	let overviewRenders = 0;
	const { activateSection } = compileFunctions(gameplayScript, ["activateSection"], {
        scheduleLiveRefresh: () => {},
		state,
		clearTimeout: () => {},
		closeOverlay: () => {},
		setPlayerGroupMenu: (open) => { assert.equal(open, false, "section changes close the group menu"); },
		renderBeacon: () => {},
		activeTab: () => tab,
		overviewGroupForSection: () => "players",
		tabReady: () => true,
		shouldReloadOverview: () => true,
		loadOverview: (...args) => { reloads.push(args); },
		renderOverview: () => { overviewRenders += 1; },
		connected: () => false,
		loadLive: () => {},
		renderLive: () => {},
	});
	activateSection("overview");
	assert.equal(tab.reloadAfterAppend, true);
	assert.equal(reloads.length, 0, "timed revalidation must not preempt an in-flight append");
	assert.equal(overviewRenders, 1);

	const controllerState = { controllers: new Map() };
	const pending = [];
	const fakeFetch = (_path, options) => new Promise((resolve) => pending.push({ resolve, signal: options.signal }));
	const { api } = compileFunctions(gameplayScript, ["api"], { state: controllerState, fetch: fakeFetch });
	const append = api("/append", {}, "overview-more:player-1", 1000);
	const appendSignal = pending[0].signal;
	const reload = api("/reload", {}, "overview:player-1", 1000);
	assert.equal(appendSignal.aborted, false);
	assert.equal(controllerState.controllers.size, 2);
	for (const request of pending) request.resolve({ ok: true, status: 200, json: async () => ({}) });
	await Promise.all([append, reload]);
	const loadSource = functionSource(gameplayScript, "loadOverview");
	assert.match(loadSource, /const requestKey = `\$\{append \? "overview-more" : "overview"\}:\$\{tab\.key\}`/);
	assert.match(loadSource, /reloadAfterAppend[\s\S]*loadOverview\(tab, true, false, false, true\)/);
});

test("R61 partial and budget-limited pagination stays resumable", () => {
	const { normalizedPagination, paginationCopyFor } = compileFunctions(gameplayScript, ["normalizedPagination", "paginationCopyFor"], {
		MAX_BROWSE_MATCHES: 200,
		number: (value) => String(value),
		escapeHTML: (value) => String(value ?? ""),
	});
	const partial = normalizedPagination({ matches: Array(20), pagination: { begIndex: 0, count: 20, hasMore: true, partial: true } }, 0);
	assert.equal(partial.hasMore, true);
	assert.equal(partial.exhaustedReason, "");
	assert.match(paginationCopyFor({ data: { matches: Array(20), pagination: partial } }), /上游中断，已加载 20 条，点击继续/);
	const budget = normalizedPagination({ matches: Array(40), pagination: { begIndex: 0, count: 40, hasMore: true, budgetExceeded: true } }, 0);
	assert.equal(budget.hasMore, true);
	assert.match(paginationCopyFor({ data: { matches: Array(40), pagination: budget } }), /本次只读到 40 条，点这里继续/);
});

test("desktop packaging has a static embedded Riot ciphertext gate", () => {
  assert.equal(desktopPackage.build.beforePack, "verify-embedded-riot-key.cjs");
  assert.match(riotKeyHook, /aes-256-gcm/);
  assert.match(riotKeyHook, /loot-service\.exe/);
  assert.match(riotKeyHook, /embedded Riot API key ciphertext was not found/);
  assert.doesNotMatch(riotKeyHook, /console\.log\([^)]*plain/);
});

test("champion detail keeps conditional fourth/fifth items out of core routes", () => {
  const dependencies = {
    state: { selected: { position: "adc" }, position: "all" },
    ADC_ITEM_ROUTE_LIMIT: 7,
    DEFAULT_ITEM_ROUTE_LIMIT: 6,
	    CORE_RECOMMENDATION_LIMIT: 5,
    renderConfigOption: (row, kind) => `<option data-kind="${kind}" data-id="${row.id}"></option>`,
    renderDepthStats: () => "",
    escapeHTML: (value) => String(value ?? ""),
  };
  const { buildItemRoutes, championItemAttemptSummary, renderBuildDepthGroups, renderRankedBuild } = compileFunctions(script, ["buildItemRoutes", "championItemAttemptSummary", "renderBuildDepthGroups", "renderRankedBuild"], dependencies);
  const build = {
    coreItems: Array.from({ length: 18 }, (_, index) => ({ assets: [{ path: `/core-${index}` }] })),
    fourthItems: [{ id: "fourth", assets: [{ path: "/late-4" }] }],
    fifthItems: [{ id: "fifth", assets: [{ path: "/late-5" }] }],
    sixthItems: [{ id: "sixth", assets: [{ path: "/late-6" }] }],
    itemSource: "OP.GG",
    itemWindow: "当前版本",
    itemChainStatus: "ready",
    fourthSample: 802,
    fifthSample: 20,
    sixthSample: 12,
  };
	  assert.equal(buildItemRoutes(build).length, 5);
  assert.deepEqual(buildItemRoutes(build)[0].assets.map((asset) => asset.path), ["/core-0"]);
  const groups = renderBuildDepthGroups(build);
  assert.match(groups, /champion-item-depth-columns/);
  assert.match(groups, /第四件[\s\S]*第五件/);
  assert.match(groups, /第六件/);
  assert.equal((groups.match(/class="config-option-list"/g) || []).length, 3);
	assert.doesNotMatch(groups, /样本少|该阶段共 20 场，仅供参考/);
	  const rankedMarkup = renderRankedBuild(build);
	  assert.match(rankedMarkup, /OP\.GG · 当前版本/);
	  const qqFallbackMarkup = renderRankedBuild({ ...build, itemSource: "QQ101", itemWindow: "国服 16.17" });
	  const qqFallbackDOM = new JSDOM(qqFallbackMarkup).window.document;
	  assert.equal(qqFallbackDOM.querySelector(".champion-build-board > header")?.textContent.includes("QQ101"), false);
	  assert.doesNotMatch(qqFallbackDOM.querySelector(".champion-item-depth-columns")?.textContent || "", /QQ101 · 国服 16\.17/);
  assert.match(rankedMarkup, /data-depth-count="3"/);
  const missingSixth = { ...build, sixthItems: [], sixthSample: 0 };
  const missingSixthGroups = renderBuildDepthGroups(missingSixth);
  assert.match(missingSixthGroups, /data-id="fourth"/);
  assert.match(missingSixthGroups, /data-id="fifth"/);
  assert.doesNotMatch(missingSixthGroups, /data-id="sixth"/);
  // 没有第六件就整列不展示；网格列数必须跟着变成 2，否则会留一条空白列。
  assert.doesNotMatch(missingSixthGroups, /第六件/);
  assert.match(renderRankedBuild(missingSixth), /data-depth-count="2"/);
  const missingFifthGroups = renderBuildDepthGroups({ ...build, fifthItems: [], fifthSample: 0 });
  assert.match(missingFifthGroups, /<h4><span>第五件<\/span><\/h4><div class="config-option-list"><\/div><p class="muted build-depth-empty">该阶段暂无可用样本<\/p>/);
  const unavailable = renderBuildDepthGroups({ itemChainStatus: "unavailable" });
  assert.match(unavailable, /第四件推荐暂不可用，稍后重试/);
  assert.match(unavailable, /第五件推荐暂不可用，稍后重试/);
  assert.doesNotMatch(unavailable, /第六件/);
  assert.equal((unavailable.match(/class="build-depth-column"/g) || []).length, 2);
  const failedBuild = {
    itemChainStatus: "unavailable",
    itemAttempts: [
      { source: "qq101", outcome: "failed", message: "QQ101 returned an empty response" },
      { source: "opgg", outcome: "failed", message: "champion provider returned HTTP 404" },
    ],
  };
  assert.equal(championItemAttemptSummary(failedBuild), "QQ101：返回为空；OP.GG：champion provider returned HTTP 404");
  const failedMarkup = renderBuildDepthGroups(failedBuild);
  assert.match(failedMarkup, /第四件推荐暂不可用：QQ101：返回为空；OP\.GG：champion provider returned HTTP 404/);
  assert.match(failedMarkup, /第五件推荐暂不可用：QQ101：返回为空；OP\.GG：champion provider returned HTTP 404/);
  assert.doesNotMatch(championItemAttemptSummary({ itemAttempts: [{ source: "opgg", outcome: "success" }] }), /OP\.GG/);
  const nonRankedBuild = functionSource(script, "renderBuildBoard");
  assert.match(nonRankedBuild, /const depthGroups = renderBuildDepthGroups\(build\)/);
  assert.match(nonRankedBuild, /class="route-options"[\s\S]*\$\{depthGroups\}/);
  assert.doesNotMatch(nonRankedBuild, /item-option-groups">\$\{itemSections\.join\(""\)\}<\/div>\$\{depthGroups\}/);
  const rankedBuild = functionSource(script, "renderRankedBuild");
  assert.match(rankedBuild, /const depthGroups = renderBuildDepthGroups\(build\)/);
  const rankedRoutesStart = rankedBuild.indexOf('<div class="build-routes">');
  const rankedDepthStart = rankedBuild.indexOf("${depthGroups}");
  assert.ok(rankedRoutesStart >= 0 && rankedDepthStart > rankedRoutesStart, "ranked depth groups must stay after the core column inside build-routes");
  const depthCSS = cssBlockAfter(styles, ".champion-item-depth-columns");
  assert.match(depthCSS, /grid-auto-rows:\s*auto/);
  assert.match(depthCSS, /align-content:\s*start/);
  assert.doesNotMatch(depthCSS, /grid-auto-flow:\s*column/);
});

test("mayhem champion detail uses the same five-row core recommendation limit", () => {
  const { renderMayhemItemRoutes } = compileFunctions(script, ["renderMayhemItemRoutes"], {
	    CORE_RECOMMENDATION_LIMIT: 5,
    objectRows: (value) => Array.isArray(value) ? value : [],
    sortedGradeRows: (rows) => rows,
    renderConfigOption: (row) => `<option data-mayhem-core="${row.id}"></option>`,
  });
  const coreItems = Array.from({ length: 18 }, (_, index) => ({ id: index + 1, assets: [{ kind: "item", path: `/item-${index + 1}.png` }] }));
  const markup = renderMayhemItemRoutes({ coreItems });
	  assert.equal((markup.match(/data-mayhem-core=/g) || []).length, 5);
	  assert.match(markup, /data-mayhem-core="5"/);
	  assert.doesNotMatch(markup, /data-mayhem-core="6"/);
	  assert.match(script, /const CORE_RECOMMENDATION_LIMIT = 5/);
});

test("hero detail rune workspace renders only the active rune board", () => {
  const { renderRuneWorkspace } = compileFunctions(script, ["allocateLoadoutSideRows", "renderRuneWorkspace"], {
    activeRunePage: () => 1,
    runeStyleIcon: (style, className) => `<i class="${className}">${style?.id || ""}</i>`,
    compactNumber: (value) => String(value ?? 0),
    percent: (value) => `${value ?? 0}%`,
    renderRuneTree: (page) => `<tree data-page="${page.id}"></tree>`,
    renderSpellsCard: () => "<spells></spells>",
    renderSkillsCard: () => "<skills></skills>",
  });
  const markup = renderRuneWorkspace([
    { id: "page-a", primaryStyle: { id: 8000 }, subStyle: { id: 8100 }, winRate: 51, games: 10, pickRate: 20 },
    { id: "page-b", primaryStyle: { id: 8200 }, subStyle: { id: 8300 }, winRate: 52, games: 20, pickRate: 30 },
  ], {});
  assert.equal((markup.match(/class="rune-board-panel"/g) || []).length, 1);
  assert.match(markup, /data-rune-page-panel="1"/);
  assert.match(markup, /<tree data-page="page-b"><\/tree>/);
  assert.doesNotMatch(markup, /data-page="page-a"/);
});

test("champion list rerenders preserve the inner table scroll position", () => {
  const previousList = { scrollTop: 287 };
  const nextList = { scrollTop: 0 };
  let queryCount = 0;
  const { render } = compileFunctions(script, ["render"], {
    root: { querySelector: () => queryCount++ === 0 ? previousList : nextList },
    state: { mode: "ranked", selected: null, listScrollInner: 0 },
    renderWorkspace: () => {},
    renderDetail: () => {},
    prepareImages: () => {},
    applyRenderedMetricStyles: () => {},
    mountArenaMatchCards: () => {},
    mountMayhemTierDialog: () => {},
    mountArenaTierDialog: () => {},
	requestAnimationFrame: (callback) => callback(),
	window: { deepLegendsSelects: { enhance() {} } },
  });
  render();
  assert.equal(nextList.scrollTop, 287);
});

test("ranked, arena, and mayhem champion rows all expose selected state", () => {
  const { renderChampionRow } = compileFunctions(script, ["renderChampionRow"], {
    state: { selected: { championId: 7 } },
    championMeta: () => ({ nameZh: "测试英雄", titleZh: "测试", nameEn: "Test", imageSource: "", imagePath: "" }),
    heroArtworkURL: () => "/art.png",
    imageURL: () => "/champion.png",
    tierBadge: () => "<tier></tier>",
    positionIcon: () => "<position></position>",
    positionLabel: () => "中路",
    number: (value) => String(value ?? 0),
    percent: (value) => `${value ?? 0}%`,
    compactNumber: (value) => String(value ?? 0),
    escapeHTML: (value) => String(value ?? ""),
  });
  for (const metrics of [true, "arena", "mayhem"]) {
    const row = renderChampionRow({ championId: 7, name: "测试英雄", tier: "S", winRate: 52, pickRate: 5, banRate: 1, play: 1000, averagePlacement: 3 }, metrics, false, 1);
    assert.match(row, /class="champion-row is-selected"/);
    assert.match(row, /aria-current="true"/);
  }
  assert.match(styles, /\.champion-table tbody tr\.is-selected > td\s*\{/);
  assert.match(styles, /\.champion-table tbody tr\.is-selected:hover > td/);
});

test("match loadout follows the OPGG mode matrix instead of trusting stray augment data", () => {
  const { matchModeKind, renderMatchLoadout } = compileFunctions(gameplayScript, ["matchQueueLabel", "queueDefinitionFor", "queueModeGroup", "isHextechClassic", "isHextechQualifier", "isOrdinaryHextechMatch", "matchModeKind", "matchAugmentIDs", "renderMatchLoadout"], {
    state: { queueGroups: [{ id: 1750, modeGroup: "arena", augmentSource: "arena" }, { id: 2300, modeGroup: "hextech-aram", augmentSource: "hextech" }, { id: 2400, modeGroup: "hextech-aram", augmentSource: "hextech" }] },
    spellIconFigure: (id) => `<spell>${id}</spell>`,
    perkIconFigure: (id) => `<perk>${id}</perk>`,
    perkStyleIconFigure: (id) => `<style>${id}</style>`,
    augmentIconFigure: (id) => `<augment>${id}</augment>`,
  });
  const subject = { spell1Id: 4, spell2Id: 32, perkIds: [8010], subStyleId: 8400, augmentIds: [901, 902, 903, 904] };
  assert.equal(matchModeKind({ queueId: 1700, modeGroup: "arena" }, subject), "arena");
  assert.equal(matchModeKind({ queueId: 1750 }, subject), "arena");
  assert.equal(matchModeKind({ queueId: 2300, modeGroup: "hextech-aram" }, subject), "mayhem");
  assert.equal(matchModeKind({ queueId: 2400, modeGroup: "hextech-aram", gameMode: "KIWI", mapId: 12 }, subject), "mayhem");
  assert.equal(matchModeKind({ queueId: 2600, modeGroup: "hextech-qualifier", gameMode: "KIWI", mapId: 12 }, subject), "mayhem");
  assert.equal(matchModeKind({ queueId: 2700, modeGroup: "other", gameMode: "ARAM_MAYHEM_CLASSIC", mapId: 12 }, subject), "mayhem-classic");
  assert.equal(matchModeKind({ queueId: 450, modeGroup: "aram" }, {}), "aram");
  assert.equal(matchModeKind({ queueId: 420, gameMode: "CLASSIC" }, subject), "standard");

  const mayhem = renderMatchLoadout(subject, "mayhem");
  assert.equal(mayhem.usesAugments, true);
  assert.equal(mayhem.layout, "mayhem");
  assert.equal((mayhem.loadout.match(/<augment>/g) || []).length, 2);
  assert.equal((mayhem.loadout.match(/<spell>/g) || []).length, 2);
  assert.match(mayhem.loadout, /<augment>901<\/augment>.*<augment>902<\/augment>/);
  assert.doesNotMatch(mayhem.loadout, /<augment>90[34]<\/augment>/);
  assert.doesNotMatch(mayhem.loadout, /<perk>|<style>/);

  const arena = renderMatchLoadout(subject, "arena");
  assert.equal(arena.usesAugments, true);
  assert.equal((arena.loadout.match(/<augment>/g) || []).length, 4);
  assert.doesNotMatch(arena.loadout, /<spell>|<perk>|<style>/);
  assert.match(cssBlockAfter(gameplayStyles, ".match-loadout-mini.is-augments.is-mayhem {"), /grid-template-columns:\s*repeat\(2,30px\)/);
  assert.match(functionSource(gameplayScript, "renderBuild"), /matchAugmentIDs\(subject, 6\)/);

  const classic = renderMatchLoadout(subject, "mayhem-classic");
  assert.equal(classic.usesAugments, false);
  assert.match(classic.loadout, /<spell>4<\/spell>.*<perk>8010<\/perk>/);
  assert.doesNotMatch(classic.loadout, /<augment>/);

  const standard = renderMatchLoadout(subject, "standard");
  assert.equal(standard.usesAugments, false);
  assert.match(standard.loadout, /<spell>4<\/spell>.*<perk>8010<\/perk>/);
  assert.doesNotMatch(standard.loadout, /<augment>/);
  assert.match(functionSource(gameplayScript, "renderMatch"), /renderMatchLoadout\(subject, modeKind\)/);
});

test("match filters keep ordinary, qualifier, and classic Hextech ARAM mutually exclusive", () => {
  const MORE_MODE_OPTIONS = [
    ["aram", "极地大乱斗", null],
    ["hextech-qualifier", "海克斯大乱斗 海选赛", null],
    ["match", "匹配模式", [400, 430, 490]],
    ["hextech-classic", "海克斯大乱斗 经典模式版", null],
    ["bots", "人机对战", [820, 830, 840, 850, 860, 870, 880, 890]],
    ["urf", "无限火力", [900, 1900]],
    ["clash", "冠军杯赛", [700, 720]],
    ["nexus-blitz", "极限闪击", [1300]],
    ["doombots", "末日人工智能", [950, 960]],
    ["special", "特殊模式", null],
  ];
	const functions = compileFunctions(
    gameplayScript,
    ["riotTab", "moreModeOptions", "matchQueueLabel", "isHextechClassic", "isHextechQualifier", "isOrdinaryHextechMatch", "matchModeKind", "isRecognizedModeMatch", "renderMatchFilters", "filteredMatches"],
	    { MORE_MODE_OPTIONS, escapeHTML: (value) => String(value ?? ""), state: { queueGroups: [] }, queueDefinitionFor: () => null, queueModeGroup: (match) => String(match?.modeGroup || "").toLowerCase() },
  );
  const matches = [
    { queueId: 2400, queueLabel: "海克斯大乱斗", modeGroup: "hextech-aram", gameMode: "KIWI", mapId: 12 },
    { queueId: 2600, queueLabel: "海克斯大乱斗 海选赛", modeGroup: "other", gameMode: "KIWI", mapId: 12 },
    { queueId: 2700, queueLabel: "海克斯大乱斗 经典模式版", modeGroup: "other", gameMode: "ARAM_MAYHEM_CLASSIC", mapId: 12 },
    { queueId: 450, queueLabel: "极地大乱斗", modeGroup: "aram", gameMode: "ARAM", mapId: 12 },
    { queueId: 1900, queueLabel: "无限火力", modeGroup: "urf", gameMode: "ARURF", mapId: 11 },
    { queueId: 2900, queueLabel: "无限火力", modeGroup: "urf", gameMode: "ARURF", mapId: 11 },
    { queueId: 1400, queueLabel: "特殊模式", modeGroup: "other", gameMode: "SPECIAL", mapId: 22 },
  ];
  const queuesFor = (matchFilter, region = "") => functions.filteredMatches(matches, { matchFilter, region }).map((match) => match.queueId);
  assert.deepEqual(queuesFor("hextech-aram"), [2400]);
  assert.deepEqual(queuesFor("more:hextech-qualifier"), [2600]);
  assert.deepEqual(queuesFor("more:hextech-classic"), [2700]);
  assert.deepEqual(queuesFor("more:aram"), [450]);
  assert.deepEqual(queuesFor("more:urf"), [1900, 2900]);
  assert.deepEqual(queuesFor("more:special"), [1400]);
  assert.deepEqual(queuesFor("more:hextech-qualifier", "kr"), []);

  const cnOptions = functions.moreModeOptions({ region: "" });
  assert.deepEqual(cnOptions.slice(0, 2).map(([key]) => key), ["aram", "hextech-qualifier"]);
  assert.equal(functions.moreModeOptions({ region: "kr" }).some(([key]) => key === "hextech-qualifier"), false);
  assert.match(functions.renderMatchFilters({ matchFilter: "more:hextech-qualifier", region: "" }), /data-app-select-value="more:hextech-qualifier"/);
  assert.doesNotMatch(functions.renderMatchFilters({ matchFilter: "all", region: "kr" }), /more:hextech-qualifier/);
});

test("remade matches use a neutral result instead of win or loss", () => {
  const { matchResultKind, matchResultWord } = compileFunctions(gameplayScript, ["matchResultKind", "matchResultWord"]);
  assert.equal(matchResultKind({ result: "remake" }), "remake");
  assert.equal(matchResultKind({ result: "loss" }), "loss");
  assert.equal(matchResultKind({ result: "unexpected" }), "unknown");
  assert.equal(matchResultWord("remake"), "重开");
  assert.equal(matchResultWord("loss"), "失败");
  assert.equal(matchResultWord("remake", 2), "第 2 名");
  const renderMatchSource = functionSource(gameplayScript, "renderMatch");
  assert.match(renderMatchSource, /class="match-entry is-\$\{resultKind\}"/);
  assert.match(renderMatchSource, /ranked && resultKind !== "remake"/);
  assert.match(functionSource(gameplayScript, "renderRuneChoice"), /config\.result === "remake" \? "重开"/);
  assert.match(gameplayStyles, /\.match-entry\.is-remake\s*\{[^}]*background:\s*color-mix\(in oklab,var\(--muted\) 28%,var\(--surface-strong\)\)[^}]*border-color:\s*color-mix\(in oklab,var\(--muted\) 68%,var\(--line\)\)[^}]*box-shadow:\s*inset 4px 0 0 var\(--muted\)/s);
  assert.match(gameplayStyles, /\.is-remake \.match-result-meta strong, \.is-remake \.result-word\s*\{[^}]*color:\s*color-mix\(in oklab,var\(--muted\) 70%,var\(--ink\)\)/s);
});

test("live rune matchups occupy the champion strip right side without wrapping their facts", () => {
  const header = functionSource(gameplayScript, "renderChampionRecommendationHeader");
  assert.match(header, /class="recommendation-champion-summary\$\{hasCounters/);
  assert.match(header, /class="champion-summary-main"/);
  assert.match(header, /class="champion-matchups"/);
  assert.match(header, /class="champion-summary-stats"/);
  assert.match(header, /const values = \(items \|\| \[\]\)\.slice\(0, 3\)/);
  assert.match(header, /\$\{percent\(item\.winRate\)\} 胜率/);
  assert.match(header, /\$\{number\(item\.games\)\} 场/);
  assert.match(header, /\["win", "胜率", stats\.winRate, true\][\s\S]*\["pick", "选取率", stats\.pickRate, true\]/);
  assert.match(header, /class="is-\$\{tone\}"/);
  assert.match(header, /hasBanRate/);
  assert.match(gameplayStyles, /\.recommendation-champion-summary\s*\{[^}]*grid-template-areas:\s*"summary matchups" "positions matchups"/s);
  assert.match(gameplayStyles, /\.recommendation-champion-summary\.is-no-counters\s*\{[^}]*grid-template-areas:\s*"summary" "positions"/s);
  assert.match(gameplayStyles, /\.champion-matchups\s*\{[^}]*grid-area:\s*matchups[^}]*border-left:\s*1px solid var\(--line\)/s);
  assert.match(gameplayStyles, /\.champion-summary-main > div strong\s*\{[^}]*white-space:\s*nowrap/s);
	  assert.match(gameplayStyles, /\.champion-summary-main\s*\{[^}]*gap:\s*0/s);
	  assert.match(gameplayStyles, /\.champion-summary-main > \.game-icon\s*\{[^}]*margin-right:\s*11px/s);
	  assert.doesNotMatch(gameplayStyles, /\.champion-summary-main > div\s*\{[^}]*min-width:\s*112px/s);
	  assert.match(gameplayStyles, /\.champion-summary-main dl\s*\{[^}]*gap:\s*12px;[^}]*margin:[^}]*15px;[^}]*padding-left:\s*15px;[^}]*border-left:\s*1px solid var\(--line\)/s);
  assert.match(gameplayStyles, /\.champion-summary-main dd\s*\{[^}]*white-space:\s*nowrap/s);
  assert.match(gameplayStyles, /\.recommendation-matchup b, \.recommendation-matchup small\s*\{[^}]*white-space:\s*nowrap/s);
  assert.match(gameplayStyles, /\.champion-summary-main \.is-win dd\s*\{\s*color:\s*var\(--success\)/s);
	  assert.match(gameplayStyles, /\.champion-summary-main \.is-pick dd\s*\{\s*color:\s*var\(--accent\)/s);
	  assert.match(gameplayStyles, /\.champion-summary-main \.is-ban dd\s*\{\s*color:\s*var\(--danger\)/s);
  const compact = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 1080px)");
  assert.match(compact, /\.recommendation-champion-summary\s*\{[^}]*grid-template-areas:\s*"summary" "positions" "matchups"/s);
	  const narrow = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 700px)");
	  assert.match(narrow, /\.champion-summary-main dl\s*\{[^}]*justify-content:\s*flex-start[^}]*gap:\s*12px/s);
	  assert.doesNotMatch(narrow, /\.champion-summary-main dl\s*\{[^}]*justify-content:\s*space-between/);
});

test("live match statistics keep fixed geometry across modes", () => {
  const matchBaseEnd = gameplayStyles.indexOf("@container matches-column");
  assert.notEqual(matchBaseEnd, -1);
  const matchBase = gameplayStyles.slice(0, matchBaseEnd);
  const stats = cssBlockAfter(matchBase, ".match-stats {");
  assert.match(stats, /align-content:\s*start/);
  assert.match(stats, /grid-template-rows:\s*repeat\(3,18px\)/);
	const arenaStats = cssBlockAfter(matchBase, ".match-stats.is-arena {");
	assert.match(arenaStats, /align-content:\s*start/);
	assert.match(arenaStats, /grid-template-rows:\s*repeat\(3,18px\)/);
	assert.match(arenaStats, /min-height:\s*54px/);
	assert.match(gameplayScript, /class="match-stats\$\{modeKind === "arena" \? " is-arena" : ""\}"/);
	assert.match(gameplayStyles, /@container matches-column \(max-width: 640px\)[\s\S]{0,320}\.match-stats\.is-arena\s*\{[^}]*repeat\(3,18px\)[^}]*min-height:\s*54px/s);
  const tier = cssBlockAfter(gameplayStyles, ".match-stat-tier .match-tier-value");
  assert.equal(cssNumber(gameplayStyles, ".match-stat-tier .match-tier-value", "min-height"), 19);
  assert.equal(cssNumber(gameplayStyles, ".match-champion", "min-height"), 63);
  assert.equal(cssNumber(gameplayStyles, ".match-loadout-mini", "min-height"), 63);
  assert.match(tier, /min-height:\s*19px/);
});

test("live matchup rows use equal-width three-column grids", () => {
  const matchupRow = cssBlockAfter(gameplayStyles, ".champion-matchups > div");
  assert.match(matchupRow, /display:\s*grid/);
  assert.match(matchupRow, /grid-template-columns:\s*58px repeat\(3,minmax\(0,1fr\)\)/);
});

test("live recommendation capability flags hide unsupported matchup and ban-rate blocks", () => {
	  const { renderChampionRecommendationHeader } = compileFunctions(gameplayScript, ["liveChampionTierBadge", "renderChampionRecommendationHeader"], {
    escapeHTML: (value) => String(value ?? ""),
    liveRecommendationChampionId: () => 64,
    liveRecommendationTarget: () => ({ position: "top", positionOverride: true }),
    liveRecommendationsFor: () => ({
      hasCounters: false,
      hasBanRate: false,
      resolvedPosition: "top",
      positionSource: "requested",
      positions: [],
    }),
    livePositionValue: (value) => String(value || ""),
    livePositionDisplay: (value) => String(value || ""),
    positionLabel: (value) => String(value || "位置未知"),
    positionIcon: () => "",
    iconFigure: () => '<span class="champion-icon"></span>',
    rate: (value) => `${value ?? 0}%`,
    percent: (value) => `${value ?? 0}%`,
	    number: (value) => String(value ?? 0),
	    liveAugmentRecommendationSource: (data) => data?.gameMode === "CHERRY" ? "arena" : data?.gameMode === "KIWI" ? "hextech" : "",
	  });
	  const markup = renderChampionRecommendationHeader(
	    { tier: 1, winRate: 52, pickRate: 8, banRate: 3, strongAgainst: [{ championId: 1, championName: "测试" }], weakAgainst: [] },
	    { championName: "测试英雄", position: "top" },
	    { players: [], gameMode: "KIWI", mapId: 12 },
	  );
	  assert.match(markup, /recommendation-champion-summary is-no-counters/);
	  assert.match(markup, /is-no-counters has-tier/);
	  assert.match(markup, /class="champion-summary-tier"[\s\S]*>梯度<[\s\S]*\/tier-icons\/1\.svg/);
	  assert.doesNotMatch(markup, /champion-matchups/);
	  assert.doesNotMatch(markup, /class="is-ban"/);
	  const arenaMarkup = renderChampionRecommendationHeader(
	    { tier: 0, winRate: 56, pickRate: 9 },
	    { championName: "斗魂英雄", position: "other" },
	    { players: [], gameMode: "CHERRY", mapId: 30 },
	  );
	  assert.match(arenaMarkup, /is-no-counters has-tier/);
	  assert.match(arenaMarkup, /class="champion-summary-tier"[\s\S]*\/tier-icons\/op\.svg/);
	  const rankedMarkup = renderChampionRecommendationHeader(
	    { tier: 2, winRate: 52, pickRate: 8 },
	    { championName: "排位英雄", position: "top" },
	    { players: [], gameMode: "CLASSIC", mapId: 11 },
	  );
	  assert.doesNotMatch(rankedMarkup, /champion-summary-tier|has-tier/);
	  assert.match(gameplayBackend, /Tier\s+\*int\s+`json:"tier,omitempty"`/);
	  assert.match(goFunctionSource(gameplayBackend, "gameplayRecommendationsFromResolvedDetail"), /Tier:\s+detail\.ArenaStats\.Tier[\s\S]*Tier:\s+heroStats\.Tier/);
	  assert.match(goFunctionSource(hexdataBackend, "loadMayhemDetail"), /response\.Stats\.Tier = &tier/);
	  assert.match(gameplayStyles, /\.recommendation-champion-summary\.is-no-counters\.has-tier\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\) 72px[^}]*grid-template-areas:\s*"summary tier" "positions tier"/s);
	  assert.match(gameplayStyles, /\.champion-summary-tier\s*\{[^}]*grid-area:\s*tier[^}]*place-content:\s*center[^}]*border-left:\s*1px solid var\(--line\)/s);
	});

test("live skill plan distinguishes primary, secondary, final, and ultimate controls", () => {
  const { renderSkillPlan } = compileFunctions(gameplayScript, ["renderSkillPlan"], {
    escapeHTML: (value) => String(value ?? ""),
    assetIcon: () => '<img alt="技能">',
    abilityTooltip: (slot, ability) => `${slot} · ${ability.name || slot}`,
  });
  const markup = renderSkillPlan(
    { skillPriority: ["Q", "E", "W"] },
    [
      { slot: "Q", name: "技能 Q", iconPath: "/q.png" },
      { slot: "W", name: "技能 W", iconPath: "/w.png" },
      { slot: "E", name: "技能 E", iconPath: "/e.png" },
      { slot: "R", name: "技能 R", iconPath: "/r.png" },
    ],
  );
  const classes = [...markup.matchAll(/<button class="([^"]+)"/g)].map((match) => match[1]);
  assert.deepEqual(classes, ["skill-icon-button is-priority", "skill-icon-button", "skill-icon-button is-secondary", "skill-icon-button is-ultimate"]);
  assert.match(gameplayStyles, /\.skill-icon-button\s*\{[^}]*color:\s*var\(--ink\)/s);
  assert.match(gameplayStyles, /\.skill-order \.is-q b\s*\{[^}]*#57A6FF/s);
  assert.match(gameplayStyles, /\.skill-order \.is-w b\s*\{[^}]*#61C992/s);
  assert.match(gameplayStyles, /\.skill-order \.is-e b\s*\{[^}]*#E3B341/s);
});

test("live skill plan keeps four distinct controls on narrow screens", () => {
  const compact = cssBlockAfter(gameplayStyles, "@container (max-width: 820px)");
  assert.match(compact, /\.build-summary-bar\.is-arena\s*\{[^}]*grid-template-columns:\s*1fr/s);
  assert.match(compact, /\.build-summary-bar\.is-arena > \.build-summary-skill\s*\{[^}]*grid-column:\s*1[^}]*border-left:\s*0/s);
  const narrow = cssBlockAfter(gameplayStyles, "@media (max-width: 620px)");
  assert.match(narrow, /\.skill-priority-row\s*\{[^}]*flex-wrap:\s*wrap/s);
  assert.match(narrow, /\.skill-priority\s*\{[^}]*flex-basis:\s*100%/s);
  assert.match(narrow, /\.skill-icon-button\s*\{[^}]*min-width:\s*48px[^}]*flex:\s*1 1 48px/s);
  assert.match(narrow, /\.skill-priority-row > \.option-stats\s*\{[^}]*margin-left:\s*auto/s);
  const buildNarrow = cssBlockAfter(gameplayStyles, "@container build-recommendation (max-width: 500px)");
  assert.match(buildNarrow, /\.skill-icon-button\s*\{[^}]*grid-template-columns:\s*minmax\(0,42px\) minmax\(0,12px\)[^}]*gap:\s*3px/s);
  assert.match(buildNarrow, /\.skill-copy \.skill-name, \.skill-copy small\s*\{[^}]*display:\s*none/s);
});

test("live augments group before limiting, retaining the top three of each rarity", () => {
	  const state = { liveRecommendationFailures: new Map() };
	  const rows = Array.from({ length: 12 }, (_, index) => ({
    rarity: ["silver", "gold", "prismatic"][index % 3],
    grade: ["C", "B", "A", "S", "S", "S", "A", "A", "B", "S", "B", "A"][index],
	    score: 100 - index,
	    assets: [{ id: index + 1, name: `推荐${index + 1}` }],
	  }));
	  let renderedRows = rows;
	  const { renderLiveAugmentRecommendations } = compileFunctions(gameplayScript, ["augmentTooltipText", "renderLiveAugmentRecommendations"], {
	    state,
	    escapeHTML: (value) => String(value ?? ""),
	    liveChampionAugmentRows: () => renderedRows,
    normalizeAugmentRarity: (value) => ({ silver: { key: "silver", label: "白银" }, gold: { key: "gold", label: "黄金" }, prismatic: { key: "prismatic", label: "棱彩" } }[String(value)] || { key: "unknown", label: "海克斯" }),
    plainText: (value) => String(value || ""),
    renderLiveAugmentIcon: (row) => `<span data-augment="${row.assets?.[0]?.id || 0}"></span>`,
    liveRecommendationTarget: () => null,
    liveRecommendationFlightActive: () => false,
    recommendationEmptyPanel: (title) => `<empty>${title}</empty>`,
    percent: (value) => `${value ?? 0}%`,
    number: (value) => String(value ?? 0),
  });
	  const markup = renderLiveAugmentRecommendations({ players: [] }, "arena");
	  assert.match(markup, /class="live-augment-columns"/);
	  assert.match(markup, /<h3>海克斯推荐<\/h3>/);
  assert.equal((markup.match(/class="live-augment-column is-/g) || []).length, 3);
  assert.equal((markup.match(/class="live-augment-option is-/g) || []).length, 9);
  assert.equal((markup.match(/class="augment-grade is-/g) || []).length, 9);
  for (const index of [4, 10, 7, 5, 8, 2, 6, 3, 12]) assert.match(markup, new RegExp(`推荐${index}(?!\\d)`));
  for (const index of [1, 9, 11]) assert.doesNotMatch(markup, new RegExp(`推荐${index}(?!\\d)`));
  assert.ok(markup.indexOf("推荐4") < markup.indexOf("推荐10") && markup.indexOf("推荐10") < markup.indexOf("推荐7"));
  assert.ok(markup.indexOf("推荐5") < markup.indexOf("推荐8") && markup.indexOf("推荐8") < markup.indexOf("推荐2"));
  assert.ok(markup.indexOf("推荐6") < markup.indexOf("推荐3") && markup.indexOf("推荐3") < markup.indexOf("推荐12"));
  const source = functionSource(gameplayScript, "renderLiveAugmentRecommendations");
  assert.doesNotMatch(source, /liveChampionAugmentRows\(data, source\)\.slice/);
	  assert.match(source, /\.sort\(/);
	  assert.match(markup, /class="augment-grade is-S"[^>]*>S<\/b>/);
	  assert.match(markup, /<article class="live-augment-option is-silver" tabindex="0" data-tooltip=/);
	  const metriclessCards = [...markup.matchAll(/<article class="live-augment-option[\s\S]*?<\/article>/g)].map((match) => match[0]);
	  assert.equal(metriclessCards.length, 9);
	  assert.doesNotMatch(metriclessCards.join(""), /<small>|—/);

	  renderedRows = [{ rarity: "silver", grade: "S", pickRate: 8, winRate: 64, assets: [{ id: 97, name: "斗魂推荐" }] }];
	  const arenaMarkup = renderLiveAugmentRecommendations({ players: [] }, "arena");
	  const arenaCard = arenaMarkup.match(/<article class="live-augment-option[\s\S]*?<\/article>/)?.[0] || "";
	  assert.ok(arenaCard.indexOf('class="live-augment-stat is-win"') < arenaCard.indexOf('class="live-augment-stat is-pick"'), "Arena stats must show win rate before pick rate");
	  assert.match(arenaCard, /class="live-augment-stat is-win"[\s\S]*胜率[\s\S]*64%/);
	  assert.match(arenaCard, /class="live-augment-stat is-pick"[\s\S]*选用率[\s\S]*8%/);
	  assert.doesNotMatch(arenaCard.replace(/\s(?:data-tooltip|aria-label)="[^"]*"/g, ""), /白银|黄金|棱彩|HexScore/);

	  renderedRows = [{ rarity: "silver", grade: "S", score: 92.4, winRate: 62, games: 12480, assets: [{ id: 98, name: "海斗推荐" }] }];
	  const hextechMarkup = renderLiveAugmentRecommendations({ players: [] }, "hextech");
	  const hextechCard = hextechMarkup.match(/<article class="live-augment-option[\s\S]*?<\/article>/)?.[0] || "";
	  assert.match(hextechCard, /class="live-augment-stat is-win"[\s\S]*胜率[\s\S]*62%/);
	  assert.match(hextechCard, /class="live-augment-stat is-games"[\s\S]*场次[\s\S]*12480 场/);
	  assert.doesNotMatch(hextechCard.replace(/\s(?:data-tooltip|aria-label)="[^"]*"/g, ""), /白银|黄金|棱彩|HexScore/);

	  renderedRows = [{ rarity: "silver", grade: "S", assets: [{ id: 99, name: "仅有白银" }] }];
	  const sparseMarkup = renderLiveAugmentRecommendations({ players: [] }, "arena");
	  assert.equal((sparseMarkup.match(/class="live-augment-column is-/g) || []).length, 1);
	  assert.match(sparseMarkup, /class="live-augment-column is-silver"/);
	  assert.doesNotMatch(sparseMarkup, /live-augment-column is-(?:gold|prismatic|unknown)/);
	  const sparseCard = sparseMarkup.match(/<article class="live-augment-option[\s\S]*?<\/article>/)?.[0] || "";
	  assert.match(sparseCard, /class="augment-grade is-S"/);
	  assert.doesNotMatch(sparseCard, /<small>|—/);

	  assert.match(gameplayStyles, /\.live-augment-columns\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
	  assert.match(gameplayStyles, /\.live-augment-column > div\s*\{[^}]*repeat\(4,minmax\(0,1fr\)\)[^}]*gap:\s*8px[^}]*padding:\s*10px 11px 12px/s);
	  assert.match(gameplayStyles, /\.live-augment-column \.live-augment-option\s*\{[^}]*border:\s*1px solid var\(--augment-accent\)[^}]*border-radius:\s*8px/s);
	  const mediumAugments = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 900px)");
	  assert.match(mediumAugments, /\.live-augment-column > div\s*\{[^}]*repeat\(2,minmax\(0,1fr\)\)/s);
	  const narrowAugments = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 620px)");
	  assert.match(narrowAugments, /\.live-augment-column > div\s*\{[^}]*grid-template-columns:\s*minmax\(0,1fr\)/s);
	  assert.match(gameplayStyles, /\.live-augment-option \.arena-augment-icon > \.game-icon\.is-large\s*\{[^}]*width:\s*48px[^}]*height:\s*48px[^}]*border-radius:\s*5px/s);
	  assert.match(gameplayStyles, /\.live-augment-option \.arena-augment-icon\s*\{[^}]*border-radius:\s*5px/s);
	  const augmentStats = cssBlockAfter(gameplayStyles, ".live-augment-option small.live-augment-stats {");
	  assert.match(augmentStats, /display:\s*flex/);
	  assert.match(augmentStats, /margin-top:\s*5px/);
	  assert.match(cssBlockAfter(gameplayStyles, ".live-augment-stat > b {"), /font-size:\s*12px/);
	  assert.match(gameplayStyles, /\.live-augment-stat\.is-win > b\s*\{[^}]*color:\s*var\(--success\)/s);
	  assert.match(gameplayStyles, /\.live-augment-stat\.is-pick > b\s*\{[^}]*color:\s*var\(--accent\)/s);
});

test("arena detail augments use the shared three-column option grid", () => {
  const state = { arenaRarity: "all", arenaExpanded: { augments: false } };
  const { renderArenaAugmentSection } = compileFunctions(script, ["renderArenaAugmentSection"], {
    state,
    arenaRarityKey: (value) => ({ 0: "silver", 4: "gold", 8: "prismatic", silver: "silver", gold: "gold", prismatic: "prismatic" }[String(value)] || "unknown"),
    sortedGradeRows: (rows) => rows,
    renderArenaOptionCard: (row) => `<article data-augment="${row.assets?.[0]?.id || 0}"></article>`,
    renderArenaExpand: () => "",
  });
  const markup = renderArenaAugmentSection({
    arenaAugmentGroups: [{
      rarity: 8,
      rows: [
        { rarity: "prismatic", assets: [{ id: 1 }] },
        { rarity: "prismatic", assets: [{ id: 2 }] },
        { rarity: "prismatic", assets: [{ id: 3 }] },
      ],
    }],
  });
  assert.match(markup, /class="arena-option-grid"/);
  assert.equal((markup.match(/data-augment="/g) || []).length, 3);
  assert.doesNotMatch(markup, /arena-augment-list|augment-podium/);
  assert.match(styles, /\.arena-option-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3,minmax\(0,1fr\)\)/s);
});

test("arena synergy cards show one readable paired champion label", () => {
  const { renderArenaTeamNames } = compileFunctions(script, ["renderArenaTeamNames"], {
    escapeHTML: (value) => String(value ?? "").replaceAll("&", "&amp;").replaceAll('"', "&quot;"),
  });
  const markup = renderArenaTeamNames({ champions: [{ name: "凯尔" }, { name: "莎弥拉" }] });
  assert.equal((markup.match(/arena-team-name/g) || []).length, 1);
  assert.match(markup, />凯尔 \+ 莎弥拉<\/span>/);
  assert.match(markup, /data-tooltip="凯尔 \+ 莎弥拉"/);
  assert.match(styles, /\.arena-synergy-grid article\s*\{[^}]*grid-template-columns:\s*22px auto minmax\(90px,1fr\) 54px 54px 110px/s);
});

test("live rune choices use the keystone title and make the whole card selectable", () => {
  const state = {
    selectedRecommendation: "",
    perks: { styles: [
      { id: 8000, name: "精密", slots: [{ perks: [{ id: 8010, name: "征服者" }] }] },
      { id: 8400, name: "坚决", slots: [] },
    ] },
  };
  const { renderRuneChoice } = compileFunctions(gameplayScript, ["runeConfigurationTitle", "renderRuneChoice"], {
    state,
    escapeHTML: (value) => String(value ?? ""),
    iconFigure: (kind, id, label, size) => `<span class="game-icon is-${size} is-${kind}" data-id="${id}" aria-label="${label}"></span>`,
    rankTitle: () => "",
    relativeTime: () => "",
    rate: (value) => `${value ?? 0}%`,
    compactNumber: (value) => String(value ?? 0),
    renderUnifiedRuneBoard: () => "<runes></runes>",
  });
  const markup = renderRuneChoice({ key: "opgg", title: "征服者 · 精密 + 坚决", primaryStyleId: 8000, subStyleId: 8400, selectedPerkIds: [8010], championId: 266, championName: "阿狸", stats: { winRate: 52, games: 100, pickRate: 18 } });
  assert.match(markup, /class="game-icon is-small is-champion" data-id="266"/);
  assert.match(markup, /<strong>征服者 \+ 坚决<\/strong>/);
  assert.match(markup, /<article class="rune-choice-card" role="radio" aria-checked="false" tabindex="0" data-rune-choice="opgg">/);
  assert.doesNotMatch(markup, /<button class="rune-choice-selector"/);
  assert.match(gameplayStyles, /\.rune-choice-selector\s*\{[^}]*grid-template-columns:\s*18px 26px minmax\(150px,1fr\) auto/);
  assert.match(gameplayStyles, /\.rune-choice-champion-placeholder\s*\{/);
  assert.match(gameplayStyles, /\.rune-choice-card\[aria-checked="true"\] \.radio-mark/);
  assert.match(functionSource(gameplayScript, "renderRecommendationArea"), /已选择：\$\{escapeHTML\(runeConfigurationTitle\(selected\)\)\}/);
});

test("specialist matchups place the colored result dot beside the opponent", () => {
  const specialistPlayerTabs = new Map([["target", "已经离开的绝活哥"]]);
  const { renderSpecialistPlayers } = compileFunctions(gameplayScript, ["renderSpecialistPlayers"], {
    state: { specialistPlayerTabs },
    liveRecommendationTarget: () => ({ key: "target" }),
    escapeHTML: (value) => String(value ?? ""),
    relativeTime: () => "刚刚",
    compactNumber: (value) => String(value ?? 0),
    rate: (value) => `${value ?? 0}%`,
	    renderUnifiedRuneBoard: () => "<runes></runes>",
	    runeConfigurationTitle: () => "征服者 + 坚决",
	    renderItemIcon: (id) => `<item data-id="${id}"></item>`,
	    iconFigure: (kind, id, label, size) => `<span class="game-icon is-${size} is-${kind}" data-id="${id}">${label}</span>`,
	    rankTitle: ({ tier, division }) => `${String(tier).toLowerCase() === "master" ? "大师" : tier}${division && String(tier).toLowerCase() !== "master" ? ` ${division}` : ""}`,
	    rankCrestIcon: (tier) => `<img class="rank-crest-icon" src="/rank-crests/${String(tier).toLowerCase()}.png">`,
	    livePositionDisplay: (value) => value === "mid" ? "middle" : value,
	    positionLabel: (value) => value === "middle" ? "中路" : value,
	  });
	const markup = renderSpecialistPlayers(Array.from({ length: 4 }, (_, index) => ({ playerName: "绝活哥", result: index % 2 ? "loss" : "win", position: "mid", opponentChampionId: 99, opponentChampionName: "对位英雄", opponentPlayerName: "对线玩家", opponentTagLine: "KR1", opponentTier: "master", opponentDivision: "I", opponentWinRate: 57, itemIds: [1, 2, 3, 4, 5, 6, 7, 8] })));
	  assert.match(markup, /class="specialist-opponent is-win"><span class="specialist-result-dot" aria-hidden="true"><\/span><span class="game-icon is-small is-champion" data-id="99">/);
	  assert.doesNotMatch(markup, /specialist-game-selector"><span class="specialist-result-dot"/);
	assert.equal((markup.match(/class="specialist-game-row/g) || []).length, 4, "active specialist must keep every returned game selectable");
	assert.match(markup, /class="specialist-game-choice"><span class="radio-mark"[^>]*><\/span>[\s\S]*<strong>征服者 \+ 坚决<\/strong>/);
	assert.match(markup, /class="specialist-game-meta"><span>中路<\/span><\/span>/);
	assert.doesNotMatch(markup, /<time>|刚刚|天前/);
	assert.doesNotMatch(markup, /胜（中路）|负（中路）|specialist-game-selector/);
	assert.match(markup, /对线玩家#KR1/);
	assert.match(markup, /class="specialist-opponent-rank"><img class="rank-crest-icon" src="\/rank-crests\/master\.png">/);
	assert.match(markup, /大师 · 胜率 <b class="win-rate-value">57%<\/b>/);
	assert.doesNotMatch(markup, /大师 I/);
	assert.match(markup, /最终装备/);
	assert.doesNotMatch(markup, /场次/);
	assert.match(markup, /<item data-id="7"><\/item>/);
	assert.doesNotMatch(markup, /<item data-id="8"><\/item>/);
  assert.match(markup, /class="specialist-player-tab is-active" data-specialist-player="绝活哥"/);
  assert.equal(specialistPlayerTabs.get("target"), "绝活哥");
  assert.match(gameplayStyles, /\.specialist-opponent\.is-win\s*\{[^}]*color:\s*var\(--accent\)/);
  assert.match(gameplayStyles, /\.specialist-opponent\.is-loss\s*\{[^}]*color:\s*var\(--danger\)/);
	assert.match(gameplayStyles, /\.specialist-opponent-rank \.rank-crest-icon\s*\{[^}]*width:\s*18px[^}]*height:\s*18px/);
	const withoutRank = renderSpecialistPlayers([{ playerName: "另一位绝活哥", result: "win", opponentPlayerName: "仍显示姓名" }]);
	assert.match(withoutRank, /仍显示姓名/);
	assert.doesNotMatch(withoutRank, /specialist-opponent-rank|胜率/);
});

test("specialist second game selection redraws and applies that game's exact runes", async () => {
	const specialistItems = [
		{ playerName: "绝活哥", result: "win", position: "top", selectedPerkIds: [11, 12, 13], primaryStyleId: 8000, subStyleId: 8400, opponentPlayerName: "第一位对手" },
		{ playerName: "绝活哥", result: "loss", position: "top", selectedPerkIds: [21, 22, 23], primaryStyleId: 8100, subStyleId: 8300, opponentPlayerName: "第二位对手" },
	];
	const opgg = { key: "opgg", title: "OPGG 默认", selectedPerkIds: [1, 2, 3], primaryStyleId: 8000, subStyleId: 8400 };
	const live = { players: [{ isCurrent: true, championId: 164, championName: "卡蜜尔" }], recommendations: { runes: { opgg: [opgg], specialists: specialistItems } } };
	const state = { live, selectedRecommendation: "opgg", specialistPlayerTabs: new Map() };
	let redraws = 0;
	let applied;
	const dependencies = {
		state,
		liveRecommendationTarget: () => ({ key: "target" }),
		liveRecommendationsFor: (data) => data?.recommendations || {},
		recommendationQueueHasTopPlayers: () => true,
		specialistRunesFor: () => specialistItems,
		liveRecommendationChampionId: () => 164,
		renderLive: () => { redraws += 1; },
		escapeHTML: (value) => String(value ?? ""),
		relativeTime: () => "3天前",
		renderUnifiedRuneBoard: (config) => `<runes data-perks="${config.selectedPerkIds.join(",")}"></runes>`,
		runeConfigurationTitle: () => "征服者 + 坚决",
		renderItemIcon: () => "",
		iconFigure: () => "",
		rankTitle: () => "",
		rankCrestIcon: () => "",
		livePositionDisplay: (value) => value,
		positionLabel: (value) => value === "top" ? "上路" : value,
		showToast: () => {},
		api: async (_path, options) => { applied = JSON.parse(options.body); return {}; },
	};
	const { renderSpecialistPlayers, selectedRuneRecommendation, bindRuneChoiceButtons, applyRunes } = compileFunctions(
		gameplayScript,
		["renderSpecialistPlayers", "selectedRuneRecommendation", "bindRuneChoiceButtons", "applyRunes"],
		dependencies,
	);

	const initialMarkup = renderSpecialistPlayers(specialistItems);
	const choiceKeys = [...initialMarkup.matchAll(/class="specialist-game-row[^"]*"[^>]*data-rune-choice="([^"]+)"/g)].map((match) => match[1]);
	assert.deepEqual(choiceKeys, ["specialist-0", "specialist-1"]);
	const listeners = [];
	const buttons = choiceKeys.map((runeChoice) => ({
		dataset: { runeChoice },
		addEventListener(type, listener) { if (type === "click") listeners.push(listener); },
	}));
	bindRuneChoiceButtons({ querySelectorAll: (selector) => selector === "[data-rune-choice]" ? buttons : [] });
	listeners[1]();
	assert.equal(state.selectedRecommendation, "specialist-1");
	assert.equal(redraws, 1);
	const selected = selectedRuneRecommendation(live);
	assert.deepEqual(selected.selectedPerkIds, [21, 22, 23]);
	assert.equal(selected.sourceLabel, "绝活哥");

	const redrawnMarkup = renderSpecialistPlayers(specialistItems);
	assert.match(redrawnMarkup, /<div class="specialist-game-row is-win"[^>]*data-rune-choice="specialist-0"/);
	assert.match(redrawnMarkup, /<div class="specialist-game-row is-loss is-selected"[^>]*data-rune-choice="specialist-1"/);
	const applyButton = { disabled: false, textContent: "应用所选符文" };
	await applyRunes({ currentTarget: applyButton });
	assert.deepEqual(applied.selectedPerkIds, [21, 22, 23]);
	assert.equal(applied.primaryStyleId, 8100);
	assert.equal(applied.subStyleId, 8300);
	assert.equal(applied.source, "绝活哥");
	assert.equal(applyButton.disabled, false);
});

test("specialist opponent lives in the selectable head and the rune content is single-column", () => {
	const renderSource = functionSource(gameplayScript, "renderSpecialistPlayers");
	const rowStart = renderSource.indexOf('<div class="specialist-game-row ');
	const rowEnd = renderSource.indexOf('</div></div></div>`', rowStart);
	const opponent = renderSource.indexOf('class="specialist-opponent ', rowStart);
	assert.ok(rowStart >= 0 && rowEnd > rowStart, "specialist row must carry the radio contract");
	assert.match(renderSource.slice(rowStart, rowEnd), /role="radio"[\s\S]*aria-checked=[^>]+[\s\S]*data-rune-choice/);
	assert.ok(opponent > rowStart && opponent < rowEnd, "opponent must be inside the selectable row");
	assert.doesNotMatch(renderSource.slice(rowStart, rowEnd), /<button\b/);
	const contentRule = cssBlockAfter(gameplayStyles, ".specialist-game-content");
	assert.match(contentRule, /display:\s*block/);
	assert.doesNotMatch(contentRule, /minmax\(190px,250px\)|grid-template-columns/);
	const runesRule = cssBlockAfter(gameplayStyles, ".specialist-game-runes");
	assert.doesNotMatch(runesRule, /overflow-x:\s*auto/);
	const compactBoardRule = cssBlockAfter(gameplayStyles, ".specialist-game-runes .unified-rune-board");
	assert.match(compactBoardRule, /--rune-header-height:\s*46px/);
	assert.match(compactBoardRule, /--rune-row-height:\s*46px/);
	assert.match(compactBoardRule, /min-height:\s*252px/);
	const compact = cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 700px)");
	assert.match(compact, /\.specialist-game-head\s*\{[^}]*grid-template-columns:\s*1fr/s);
	assert.doesNotMatch(compact, /\.specialist-game-content\s*\{[^}]*grid-template-columns|\.specialist-opponent\s*\{[^}]*border-(?:top|left)/s);
});

test("live rune style icons preserve their 4:5 artwork ratio", () => {
	  assert.match(gameplayStyles, /--rune-style-icon-width:\s*32px/);
	  assert.match(gameplayStyles, /--rune-style-icon-height:\s*40px/);
	  for (const name of ["precision", "domination", "sorcery", "inspiration", "resolve"]) {
	    const svg = fs.readFileSync(path.join(__dirname, "rune-styles", `${name}.svg`), "utf8");
	    assert.match(svg, /viewBox="0 0 24 30"/);
	  }
	  const styleRule = cssBlockAfter(gameplayStyles, ".game-icon.is-rune-style");
	  assert.match(styleRule, /flex-basis:\s*var\(--rune-style-icon-width\)/);
	  assert.match(styleRule, /width:\s*var\(--rune-style-icon-width\)/);
	  assert.match(styleRule, /height:\s*var\(--rune-style-icon-height\)/);
	  const scopedStyleRule = cssBlockAfter(gameplayStyles, ".unified-rune-column > header > .game-icon.is-rune-style");
	  assert.match(scopedStyleRule, /min-width:\s*var\(--rune-style-icon-width\)/);
	  assert.match(scopedStyleRule, /max-height:\s*var\(--rune-style-icon-height\)/);
	  const runeRule = cssBlockAfter(gameplayStyles, ".unified-rune-board .game-icon.is-rune");
	  assert.match(runeRule, /width:\s*var\(--rune-icon-size\)/);
	  assert.match(runeRule, /height:\s*var\(--rune-icon-size\)/);
	  const headerRule = cssBlockAfter(gameplayStyles, ".unified-rune-column > header");
	  assert.match(headerRule, /height:\s*var\(--rune-header-height\)/);
	  assert.match(headerRule, /min-height:\s*var\(--rune-header-height\)/);
	  const boardRule = cssBlockAfter(gameplayStyles, ".unified-rune-board");
	  assert.match(boardRule, /--rune-icon-size:\s*32px/);
	  assert.match(boardRule, /--rune-header-height:\s*52px/);
	  assert.match(boardRule, /min-height:\s*284px/);
	  const compactBoardRule = cssBlockAfter(gameplayStyles, ".rune-detail .unified-rune-board");
	  assert.match(compactBoardRule, /--rune-icon-size:\s*30px/);
	  assert.match(compactBoardRule, /min-height:\s*252px/);
});

test("arena medium cards keep one-line identities and compact recommendation icons", () => {
	assert.match(styles, /\.arena-option-main > strong\s*\{[^}]*font-size:\s*15px/s);
	assert.match(styles, /\.arena-option-card \.recommend-icon\s*\{[^}]*width:\s*48px[^}]*height:\s*48px/s);
	assert.match(styles, /\.arena-section-icon\s*\{[^}]*font-size:\s*18px[^}]*line-height:\s*1/s);
	const medium = cssBlockAfter(styles, "@container arena-pane (max-width: 820px)");
	assert.match(medium, /\.arena-option-grid, \.arena-augment-section \.arena-option-grid\s*\{[^}]*repeat\(2,minmax\(0,1fr\)\)/s);
	assert.doesNotMatch(medium, /\.arena-option-main[^}]*display:\s*grid|\.arena-option-card dl[^}]*repeat\(2/);
	assert.doesNotMatch(styles, /@media \(max-width:\s*900px\)\s*\{[^}]*\.arena-augment-section \.arena-option-grid\s*\{\s*grid-template-columns:\s*1fr/s);
	const arenaNarrow = cssBlockAfter(styles, "@container arena-pane (max-width: 540px)");
	assert.match(arenaNarrow, /\.arena-overview-metrics\s*\{[^}]*width:\s*100%[^}]*min-width:\s*0[^}]*repeat\(2,minmax\(0,1fr\)\)/s);
	const mayhemMedium = cssBlockAfter(styles, "@container mayhem-pane (max-width: 760px)");
	assert.match(mayhemMedium, /\.mayhem-overview-strip\s*\{[^}]*min-height:\s*94px/s);
	assert.doesNotMatch(mayhemMedium, /\.mayhem-overview-strip[^}]*flex-direction:\s*column/);
});

test("mayhem and arena hero strips match the ranked artwork treatment", () => {
	const rankedArt = cssBlockAfter(styles, ".champion-detail-art");
	const mayhemArt = cssBlockAfter(styles, ".mayhem-overview-art");
	const arenaArt = cssBlockAfter(styles, ".arena-overview-art");
	for (const rule of [rankedArt, mayhemArt, arenaArt]) {
		assert.match(rule, /object-fit:\s*cover/);
		assert.match(rule, /object-position:\s*center 20%/);
		assert.match(rule, /transform:\s*scale\(1\.015\)/);
		assert.doesNotMatch(rule, /opacity:/);
	}
	const rankedShade = cssBlockAfter(styles, ".champion-detail-art-shade");
	const mayhemShade = cssBlockAfter(styles, ".mayhem-overview-shade");
	const arenaShade = cssBlockAfter(styles, ".arena-overview-shade");
	for (const shade of [mayhemShade, arenaShade]) assert.equal(shade.match(/background:\s*([^;]+);/)?.[1], rankedShade.match(/background:\s*([^;]+);/)?.[1]);
	assert.match(cssBlockAfter(styles, ".mayhem-overview-strip"), /min-height:\s*108px/);
	assert.match(cssBlockAfter(styles, ".arena-overview-strip"), /min-height:\s*132px/);
	const mayhemNarrow = cssBlockAfter(styles, "@container mayhem-pane (max-width: 520px)");
	assert.match(mayhemNarrow, /\.mayhem-overview-identity\s*\{[^}]*flex:\s*0 0 auto/s);
});

test("build rows keep a fixed 42px icon size and wrap whole depth groups", () => {
  assert.doesNotMatch(gameplayScript, /fitBuildOptionIcons|mountAdaptiveBuildIcons|--option-icon-fit/);
  assert.doesNotMatch(script, /fitBuildOptionIcons|mountAdaptiveBuildIcons|--option-icon-fit/);
  assert.match(functionSource(gameplayScript, "renderConfigOption"), /data-icon-count/);
  assert.match(functionSource(script, "renderConfigOption"), /data-icon-count/);
  const liveIcons = cssBlockAfter(gameplayStyles, ".config-icons");
  assert.match(liveIcons, /flex:\s*0 0 auto/);
  assert.match(liveIcons, /flex-wrap:\s*nowrap/);
  assert.match(liveIcons, /overflow:\s*visible/);
  assert.match(liveIcons, /--option-icon-size:\s*42px/);
  const itemButton = cssBlockAfter(gameplayStyles, ".config-icons .item-option-button");
  assert.match(itemButton, /flex:\s*0 0 var\(--option-icon-size\)/);
  assert.match(itemButton, /height:\s*var\(--option-icon-size\)/);
  assert.doesNotMatch(`${gameplayStyles}\n${styles}`, /--option-icon-fit|--resolved-option-icon-size/);
  assert.match(cssBlockAfter(styles, ".champion-build-board .config-icons"), /--option-icon-size:\s*42px/);
  assert.match(styles, /(?:^|\n)\.build-routes\s*\{[^}]*overflow:\s*visible/s);
  for (const width of [980, 760, 500]) assert.match(sharedBuildStyles, new RegExp(`@container \\(max-width:\\s*${width}px\\)`));
  const compactBuildRows = cssBlockAfter(sharedBuildStyles, "@container (max-width: 980px)");
  for (const count of [1, 2, 3]) {
    assert.match(compactBuildRows, new RegExp(`\\.build-item-row\\[data-depth-count="${count}"\\]`));
  }
  assert.match(compactBuildRows, /grid-template-columns:\s*minmax\(0,1fr\)/);
  assert.doesNotMatch(sharedBuildStyles, /@container \(max-width:\s*420px\)/);
  assert.match(sharedBuildStyles, /\.item-core-column h3[\s\S]*\.item-depth-columns > section h4[^{]*\{[^}]*font-size:\s*13px/s);
  assert.match(sharedBuildStyles, /\.build-item-row\s*\{[^}]*align-items:\s*stretch/s);
  assert.match(sharedBuildStyles, /\.item-core-column \+ \.item-depth-columns > section:first-child/);
  assert.match(styles, /@container champion-build \(min-width:\s*981px\)[\s\S]*?data-depth-count="3"[^}]*calc\(\(100% - 20px\)\/3\) repeat\(3,minmax\(0,1fr\)\)/s);
  assert.match(gameplayStyles, /@container build-recommendation \(max-width:\s*360px\)[\s\S]*?\.item-core-column \.config-option[^}]*flex-direction:\s*column/s);
  assert.match(styles, /@container champion-build \(max-width:\s*360px\)[\s\S]*?\.build-core-column \.config-option[^}]*flex-direction:\s*column/s);
  for (const slot of ["fourthOptions", "fifthOptions", "sixthOptions"]) assert.match(demoScript, new RegExp(`${slot}: \\[`));
  assert.match(`${script}\n${gameplayScript}`, /sixthItems|sixthOptions|第六件/);
  assert.match(script, /const cores = \(build\.coreItems \|\| \[\]\)\.slice\(0, CORE_RECOMMENDATION_LIMIT\)/);
});

test("session summary uses a generic auto refresh status", () => {
	  const summary = { hidden: true, innerHTML: "" };
	  const state = { liveLoading: false, settings: { liveRefresh: true, liveInterval: 3 } };
	  const { renderSessionSummary } = compileFunctions(gameplayScript, ["renderSessionSummary", "liveAutoRefreshStopped"], {
	    nodes: { liveSessionSummary: summary },
	    state,
	    connected: () => true,
	    phaseLabel: () => "英雄选择",
		    liveModeLabel: () => "经典模式",
		    liveMapLabel: () => "召唤师峡谷",
		    liveCurrentPositionChip: () => "",
		    escapeHTML: (value) => String(value ?? ""),
	  });
	  renderSessionSummary({ available: true, phase: "ChampSelect", queueLabel: "单排/双排", mapId: 11 });
	  assert.match(summary.innerHTML, /自动刷新已开启/);
	  state.liveLoading = true;
	  renderSessionSummary({ available: true, phase: "ChampSelect", queueLabel: "单排/双排", mapId: 11 });
	  assert.match(summary.innerHTML, /自动刷新已开启/);
	  assert.doesNotMatch(summary.innerHTML, /正在刷新/);
	  assert.doesNotMatch(summary.innerHTML, /15\s*秒/);
	  state.settings.liveRefresh = false;
	  renderSessionSummary({ available: true, phase: "ChampSelect", queueLabel: "单排/双排", mapId: 11 });
	  assert.match(summary.innerHTML, /手动刷新/);
});

test("live page defaults to a 3 second refresh and resets recommendations to runes on re-entry", () => {
  const { normalizeLiveInterval } = compileFunctions(gameplayScript, ["normalizeLiveInterval"]);
  assert.equal(normalizeLiveInterval(undefined), 3);
  assert.equal(normalizeLiveInterval("3"), 3);
  assert.equal(normalizeLiveInterval("15"), 15);
  assert.match(gameplayScript, /readSetting\("live-interval", "3"\)/);

  const state = {
    section: "overview",
    liveTimer: 0,
    tabs: [{ openMatches: new Set([1]), matchDetailTabs: new Map([[1, "build"]]), matchFilter: "solo" }],
    settings: { defaultMatchFilter: "all" },
    recommendationTab: "build",
    recommendationTabTouched: true,
    beacon: { active: false, acked: false, phase: "" },
  };
  const { activateSection } = compileFunctions(gameplayScript, ["activateSection"], {
        scheduleLiveRefresh: () => {},
    state,
    clearTimeout: () => {},
    closeOverlay: () => {},
    setPlayerGroupMenu: (open) => { assert.equal(open, false, "section changes close the group menu"); },
    connected: () => false,
    renderLive: () => {},
    renderBeacon: () => {},
  });
  activateSection("live");
  assert.equal(state.recommendationTab, "runes");
  assert.equal(state.recommendationTabTouched, false);
  assert.equal(state.tabs[0].matchFilter, "solo", "保留玩家内容状态以恢复原滚动锚点");
});

test("live beacon reacts to active phase changes and has a one second fallback", () => {
  const state = { section: "overview", beacon: { active: true, acked: true, phase: "ChampSelect" } };
  const { updateBeacon } = compileFunctions(gameplayScript, ["updateBeacon"], {
    state,
    beaconPhases: new Set(["ChampSelect", "GameStart", "InProgress", "Reconnect"]),
    renderBeacon: () => {},
  });
  updateBeacon("GameStart");
  assert.equal(state.beacon.active, true);
  assert.equal(state.beacon.acked, false);
  assert.equal(state.beacon.phase, "GameStart");
  updateBeacon("None");
  assert.equal(state.beacon.active, false);
  assert.match(gameplayScript, /const BEACON_FAST_POLL_MS = 1_000/);
  assert.match(gameplayScript, /const BEACON_DISCONNECTED_POLL_MS = 1_000/);
  assert.match(functionSource(gameplayScript, "updateStatus"), /scheduleBeaconPoll\(0\)/);
	  assert.match(functionSource(gameplayScript, "pollGameflowPhase"), /!connected\(\)[\s\S]*BEACON_DISCONNECTED_POLL_MS/);
	  assert.match(gameplayStyles, /:root\[data-sidebar="collapsed"\] #section-live\s*\{[^}]*overflow:\s*visible/s);
	  assert.match(gameplayStyles, /:root\[data-sidebar="collapsed"\] #section-live \.live-beacon\s*\{[^}]*right:\s*7px/s);
	  assertR46Contracts();
});

test("R46 live refresh delays follow the game phase", () => {
	const { liveRefreshDelayMs } = compileFunctions(gameplayScript, ["normalizeLiveInterval", "liveRefreshDelayMs"]);
	assert.equal(liveRefreshDelayMs("ChampSelect", 60), 3_000);
	assert.equal(liveRefreshDelayMs("InProgress", 3), 3_000);
	assert.equal(liveRefreshDelayMs("Reconnect", 5), 5_000);
	assert.equal(liveRefreshDelayMs("Lobby", 15), 15_000);
	assert.equal(liveRefreshDelayMs("", 999), 3_000);
});

test("R46 TFT live sessions render one explicit unsupported state", () => {
	const content = { innerHTML: "", querySelector: () => null };
	const state = {
		live: { available: true, unsupported: true, unsupportedReason: "暂时不支持此模式，敬请期待" },
		liveError: "",
		liveLoading: false,
	};
	const toolbar = { hidden: true };
	const { renderLive } = compileFunctions(gameplayScript, ["renderLive"], {
		state,
		nodes: { liveRefresh: { closest: () => toolbar, setAttribute() {} }, liveContent: content },
		connected: () => true,
		renderSessionSummary: () => {},
		emptyState: (title, detail) => `<empty><strong>${title}</strong><p>${detail}</p></empty>`,
	});
	renderLive();
	assert.equal(toolbar.hidden, false);
	assert.match(content.innerHTML, /暂时不支持此模式，敬请期待/);
	assert.match(content.innerHTML, /当前模式不会进入阵容与推荐分析/);
	assert.doesNotMatch(content.innerHTML, /live-team|地图未知/);
	assert.match(gameplayBackend, /Unsupported\s+bool\s+`json:"unsupported"`/);
	assert.match(gameplayBackend, /UnsupportedReason\s+string\s+`json:"unsupportedReason,omitempty"`/);
});

test("R47 Arena live insights use one honest roster with phase-specific context", () => {
  const state = { settings: { liveOrder: "position" } };
  const { renderLiveInsights } = compileFunctions(gameplayScript, ["orderLivePlayers", "livePremadeRoster", "clusterPremadePlayers", "arenaLivePlayerGroups", "renderLiveRecentPositions", "renderLiveInsights"], {
    state,
    isARAMRelatedMatch: () => false,
    liveAugmentRecommendationSource: (data) => String(data?.gameMode || "").toUpperCase() === "CHERRY" ? "arena" : "",
    renderLivePlayer: (player) => `<player>${player.id}</player>`,
    renderInsightMatches: () => "",
    recordLiveRosterRendered: () => {},
    escapeHTML: (value) => String(value ?? ""),
  });
  const players = [
    { id: "red-top", teamId: 200, position: "top" },
    { id: "blue-jungle", teamId: 100, position: "jungle" },
    { id: "blue-middle", teamId: 100, position: "middle" },
  ];

  const champSelect = renderLiveInsights({
    phase: "ChampSelect",
    gameMode: "CHERRY",
    mapId: 30,
    champSelectNotice: "斗魂英雄选择阶段只展示小队玩家信息",
    players,
  });
  assert.match(champSelect, /class="live-teams is-insight is-arena"/);
  assert.match(champSelect, /<h3>己方小队<\/h3>/);
  assert.doesNotMatch(champSelect, /class="live-roster-notice"/);
  assert.doesNotMatch(champSelect, /<h3>我方<\/h3>|<h3>对方<\/h3>/);
  assert.equal((champSelect.match(/class="live-team/g) || []).length, 2, "one team section plus the live-teams wrapper");

  const inProgress = renderLiveInsights({ phase: "InProgress", gameMode: "CHERRY", mapId: 30, players });
  assert.match(inProgress, /<h3>全部玩家<\/h3>/);
  assert.doesNotMatch(inProgress, /己方小队|live-roster-notice/);

	  const classic = renderLiveInsights({ phase: "InProgress", gameMode: "CLASSIC", mapId: 11, players });
	  assert.match(classic, /<h3>蓝方<\/h3>/);
	  assert.match(classic, /<h3>红方<\/h3>/);
	  assert.match(classic, /无法确定你所在阵营，按蓝方\/红方展示/);
  assert.doesNotMatch(classic, /live-teams is-insight is-arena/);
});

test("R58 Arena live playerlist groups six teams and malformed shapes fall back to one roster", () => {
  const state = { settings: { liveOrder: "team" } };
  const { arenaLivePlayerGroups, renderLiveInsights } = compileFunctions(gameplayScript, ["orderLivePlayers", "livePremadeRoster", "clusterPremadePlayers", "arenaLivePlayerGroups", "renderLiveRecentPositions", "renderLiveInsights"], {
    state,
    isARAMRelatedMatch: () => false,
    liveAugmentRecommendationSource: () => "arena",
    renderLivePlayer: (player) => `<player data-id="${player.id}"></player>`,
    renderInsightMatches: () => "",
    recordLiveRosterRendered: () => {},
    escapeHTML: (value) => String(value ?? ""),
    arenaTeamMeta: (group) => ({ name: `吉祥物 ${group.subteamId}` }),
  });
  const players = Array.from({ length: 18 }, (_, index) => ({ id: `p${index + 1}`, arenaGroup: String(Math.floor(index / 3) + 1), teamId: 100 }));
  const grouped = renderLiveInsights({ phase: "InProgress", gameMode: "CHERRY", arenaGrouped: true, players });
  assert.equal((grouped.match(/class="live-team is-arena"/g) || []).length, 6);
  assert.equal((grouped.match(/<player /g) || []).length, 18);
  for (let index = 1; index <= 6; index += 1) assert.match(grouped, new RegExp(`<h3>小队 ${index}<\\/h3>`));

  const disconnected = players.slice(0, 17);
  const groupedWithDisconnect = renderLiveInsights({ phase: "InProgress", gameMode: "CHERRY", arenaGrouped: true, players: disconnected });
  assert.equal((groupedWithDisconnect.match(/class="live-team is-arena"/g) || []).length, 6);
  assert.equal((groupedWithDisconnect.match(/<player /g) || []).length, 17);
  const reconnect = renderLiveInsights({ phase: "Reconnect", gameMode: "CHERRY", arenaGrouped: true, players: disconnected });
  assert.equal((reconnect.match(/class="live-team is-arena"/g) || []).length, 6);
  const mapped = renderLiveInsights({ phase: "InProgress", gameMode: "CHERRY", arenaGrouped: true, arenaMascotMapping: true, players });
  for (let index = 1; index <= 6; index += 1) assert.match(mapped, new RegExp(`<h3>吉祥物 ${index}<\\/h3>`));

  const named = renderLiveInsights({ phase: "InProgress", gameMode: "CHERRY", arenaGrouped: true, arenaMascotMapping: true, arenaGroupNames: { "1": "客户端队名" }, players });
	assert.match(named, /<h3>客户端队名<\/h3>/);
	assert.doesNotMatch(named, /<h3>吉祥物 1<\/h3>/);

  const malformed = players.map((player, index) => ({ ...player, arenaGroup: index < 15 ? player.arenaGroup : "5" }));
  assert.equal(arenaLivePlayerGroups({ phase: "InProgress", arenaGrouped: true }, malformed).length, 0);
  const fallback = renderLiveInsights({ phase: "InProgress", gameMode: "CHERRY", arenaGrouped: true, players: malformed });
  assert.equal((fallback.match(/class="live-team is-arena"/g) || []).length, 1);
  assert.match(fallback, /<h3>全部玩家<\/h3>/);
  assert.doesNotMatch(fallback, /<h3>小队 /);
  assert.match(cssBlockAfter(gameplayStyles, ".live-teams.is-arena.is-grouped {"), /grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/);
  assert.match(functionSource(gameplayScript, "renderLiveInsights"), /if \(groups\.length >= 2\)/);
});

test("R49 Arena player cards use the current champion fallback, hide lane copy, and put self first", () => {
  const renderedChampionIds = [];
  const { renderLivePlayer } = compileFunctions(gameplayScript, ["liveDisplayedChampionId", "livePremadeRoster", "renderLivePremadeTag", "renderLivePlayer"], {
	state: { liveLoading: false, settings: {} },
    maskedPlayerName: (player) => player.id,
    iconFigure: (_kind, id) => { renderedChampionIds.push(id); return `<champion data-id="${id}"></champion>`; },
    escapeHTML: (value) => String(value ?? ""),
    positionLabel: (value) => String(value || "位置未知"),
    rankTitle: () => "黄金 IV",
    number: (value) => String(value ?? 0),
    percent: (value) => `${value ?? 0}%`,
    kda: (value) => String(value ?? 0),
  });
  const selfCard = renderLivePlayer({ id: "self", isCurrent: true, championId: 0, championPickIntent: 0, position: "" }, 0, true, 64);
  const teammateCard = renderLivePlayer({ id: "mate", isCurrent: false, isAlly: true, championId: 0, championPickIntent: 0, position: "" }, 1, true, 64);
  assert.deepEqual(renderedChampionIds, [64, 0]);
  assert.doesNotMatch(selfCard, /未定级|白银/);
  assert.doesNotMatch(selfCard, /位置未知| · /);
  assert.doesNotMatch(teammateCard, /data-id="64"/);
	assert.match(selfCard, /class="live-player is-self"[\s\S]*class="live-player-copy"[\s\S]*class="self-chip">自己<\/span>[\s\S]*<\/div><dl>/);
	assert.doesNotMatch(selfCard, /<\/dl><span class="self-chip"/);
	assert.match(teammateCard, /class="live-player is-ally"/);
	const dom = new JSDOM(`<style>.live-player { ${cssBlockAfter(gameplayStyles, ".live-player {")} }</style>${selfCard}${teammateCard}`);
	const cards = dom.window.document.querySelectorAll(".live-player");
	assert.equal(cards[0].querySelector(".live-player-copy > .live-player-identity > .self-chip")?.textContent, "自己");
	assert.equal(dom.window.getComputedStyle(cards[0]).gridTemplateColumns, dom.window.getComputedStyle(cards[1]).gridTemplateColumns);

  const state = { settings: { liveOrder: "team" } };
  const { orderLivePlayers } = compileFunctions(gameplayScript, ["orderLivePlayers"], { state });
  const ordered = orderLivePlayers([{ id: "mate" }, { id: "self", isCurrent: true }, { id: "other" }], false);
  assert.deepEqual(ordered.map((player) => player.id), ["self", "mate", "other"]);
	const notice = cssBlockAfter(gameplayStyles, ".live-roster-notice {");
	assert.match(notice, /width:\s*auto/);
	assert.match(notice, /background:\s*color-mix/);
	assert.match(notice, /border:\s*1px solid color-mix/);
	assert.ok(cssNumber(gameplayStyles, ".live-roster-notice {", "font-size") >= 13);
	assert.doesNotMatch(notice, /grid-column/);
	assert.match(cssBlockAfter(gameplayStyles, ".live-player {"), /grid-template-columns:\s*48px minmax\(100px,1fr\) minmax\(220px,auto\)/);
	assert.doesNotMatch(cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 1080px)"), /\.live-player\s*\{/);
	assert.match(cssBlockAfter(gameplayStyles, ".recommendation-tab-row {"), /justify-content:\s*space-between/);
	assert.match(cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 700px)"), /\.recommendation-tab-row\s*\{[^}]*flex-wrap:\s*wrap/s);
	assert.match(cssBlockAfter(gameplayStyles, "@container recommendation-area (max-width: 700px)"), /\.live-player\s*\{[^}]*grid-template-columns:\s*42px minmax\(0,1fr\)/s);
  assert.match(gameplayBackend, /arenaChampSelectNotice\s*=\s*"我的小队：英雄选择阶段客户端只提供本小队信息，其余小队进入对局后仍不提供小队归属"/);
  assert.doesNotMatch(functionSource(gameplayScript, "renderLiveInsights"), /live-roster-notice/);
});

test("R66 live premade hints cluster players and render a rich, fail-closed tag", () => {
	const dependencies = {
		LIVE_PREMADE_MIN_SHARED_GAMES: 5,
		liveDisplayedChampionId: (player, current) => Number(player.championId) || (player.isCurrent ? current : 0),
		maskedPlayerName: (player) => player.name,
		proxyAsset: (path) => `/api/image?path=${encodeURIComponent(path)}`,
		assetPath: (kind, id) => kind === "profile"
			? `/lol-game-data/assets/v1/profile-icons/${id}.jpg`
			: `/lol-game-data/assets/v1/champion-icons/${id}.png`,
		escapeHTML: (value) => String(value ?? "").replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;"),
	};
	const { livePremadeRoster, renderLivePremadeTag, clusterPremadePlayers } = compileFunctions(
		gameplayScript,
		["livePremadeRoster", "renderLivePremadeTag", "clusterPremadePlayers"],
		dependencies,
	);
	const players = [
		{ id: "a", name: "甲", premadeGroup: "1", premadeSize: 2, profileIconId: 11, championId: 22, championName: "艾希" },
		{ id: "solo", name: "独行", championId: 64 },
		{ id: "b", name: "乙", premadeGroup: "1", premadeSize: 2, profileIconId: 12, championId: 81, championName: "伊泽瑞尔" },
	];
	assert.deepEqual(clusterPremadePlayers(players).map((player) => player.id), ["a", "b", "solo"]);
	assert.equal(livePremadeRoster(players, players[0]).length, 2);
	const tag = renderLivePremadeTag(players[0], players, 0);
	assert.match(tag, /class="premade-team-tag is-color-0"/);
	assert.match(tag, />预组 ×2<\/span>/);
	assert.match(tag, /data-tooltip-roster=/);
	assert.match(tag, /profile-icons%2F11\.jpg/);
	assert.match(tag, /champion-icons%2F22\.png/);

	const inconsistent = [{ ...players[0], premadeSize: 3 }, players[2]];
	assert.equal(renderLivePremadeTag(inconsistent[0], inconsistent, 0), "");
	assert.match(functionSource(gameplayScript, "renderLivePremadeTag"), /members\.length < 2/);
	assert.doesNotMatch(functionSource(gameplayScript, "clusterPremadePlayers"), /CHERRY|queueId|gameMode/);
	assert.match(functionSource(appScript, "setupFloatingTooltips"), /dataset\.tooltipRoster/);
	assert.match(gameplayStyles, /\.premade-team-tag\.is-color-11/);
	for (const color of ["#48e5db", "#628aff", "#d4de17", "#2eda3e", "#ff9f1c", "#da4e2e", "#bc2ebc", "#fa4e80", "#0b3d91", "#7f0000", "#8b4513", "#555"]) {
		assert.match(gameplayStyles, new RegExp(color));
	}
});

test("R66 premade backend guards reject cache and inference mutations", () => {
	const assertContracts = (source) => {
		assert.match(source, /TeamParticipantID\s+int64\s+`json:"teamParticipantId"`/);
		assert.match(goFunctionSource(source, "livePremadeAssignments"), /inferenceEnabled := covered > len\(inputs\)\/2/);
		assert.match(goFunctionSource(source, "livePremadeAssignments"), /shared >= threshold/);
		assert.match(goFunctionSource(source, "livePremadeAssignments"), /union\(left, right\)/);
		assert.match(goFunctionSource(source, "livePlayerMatches"), /return a\.cachedLivePlayerMatches/);
		assert.match(goFunctionSource(source, "loadGameplayLive"), /applyLivePremadeAssignments\(response\.Players, premadeInputs, phase, arenaMode\)/);
		assert.match(goFunctionSource(source, "applyLivePremadeAssignments"), /if arenaMode \|\| \(phase != "InProgress" && phase != "Reconnect"\)/);
	};
	assertContracts(gameplayBackend);
	for (const [before, after] of [
		["inferenceEnabled := covered > len(inputs)/2", "inferenceEnabled := true"],
		["if shared >= threshold", "if shared >= 999"],
		["return a.cachedLivePlayerMatches(ctx, key", "return a.loadLivePlayerMatches(ctx, client, reference, playerRef, isCurrent, names); /*"],
		["if arenaMode || (phase != \"InProgress\" && phase != \"Reconnect\")", "if phase != \"InProgress\" && phase != \"Reconnect\""],
	]) {
		const mutated = gameplayBackend.replace(before, after);
		assert.notEqual(mutated, gameplayBackend, `mutation target not found: ${before}`);
		assert.throws(() => assertContracts(mutated));
	}
});

test("Arena random champion IDs use the official unselected champion icon", () => {
	const { assetPath } = compileFunctions(gameplayScript, ["assetPath"], { state: { perks: null } });
	for (const id of [0, -1, -3]) {
		assert.equal(assetPath("champion", id), "/lol-game-data/assets/v1/champion-icons/-1.png");
	}
	const mutated = gameplayScript.replace('if (kind === "champion" && Number(id) <= 0) return "/lol-game-data/assets/v1/champion-icons/-1.png";', "");
	assert.notEqual(mutated, gameplayScript);
	const mutatedAssetPath = compileFunctions(mutated, ["assetPath"], { state: { perks: null } }).assetPath;
	assert.notEqual(mutatedAssetPath("champion", -3), "/lol-game-data/assets/v1/champion-icons/-1.png");
});

test("R56 recommendation loading is centered by its overlay without viewport overflow", () => {
	assert.match(gameplayStyles, /\.recommendation-panel\s*\{[^}]*min-width:\s*0/s);
	// 视口单位要除以 --ui-zoom：被 .app-frame 的 CSS zoom 缩放之后，
	// 60dvh 仍是未缩放的视口值，直接用会比视口高出 zoom 倍。
	assert.match(cssBlockAfter(gameplayStyles, ".recommendation-panel.is-loading {"), /min-height:\s*min\(420px,calc\(60dvh \/ var\(--ui-zoom, 1\)\)\)/);
	assert.match(cssBlockAfter(gameplayStyles, ".recommendation-panel.is-loading > :not(.panel-loading) {"), /display:\s*none/);
	const overlay = cssBlockAfter(gameplayStyles, ".panel-loading {");
	assert.match(overlay, /display:\s*grid/);
	assert.match(overlay, /place-items:\s*center/);
	const content = cssBlockAfter(gameplayStyles, ".panel-loading-content {");
	assert.match(content, /display:\s*grid/);
	assert.match(content, /min-height:\s*min\(220px,100%\)/);
	assert.match(content, /max-height:\s*min\(220px,100%\)/);
	assert.match(content, /place-content:\s*center/);
	assert.match(content, /justify-items:\s*center/);
	assert.doesNotMatch(content, /position:\s*sticky|100vh|(?:^|;)\s*top\s*:|(?:^|;)\s*height\s*:/);
	assert.match(functionSource(gameplayScript, "renderRecommendationArea"), /class="panel-loading-content"/);
});

test("R56 missing recommendation metrics remove the whole metric item and empty wrappers", () => {
	let recommendation = { hasCounters: false, hasBanRate: true, resolvedPosition: "top", positions: [] };
	const { renderChampionRecommendationHeader } = compileFunctions(gameplayScript, ["renderChampionRecommendationHeader"], {
		state: { live: {} },
		escapeHTML: (value) => String(value ?? ""),
		liveRecommendationChampionId: () => 64,
		liveRecommendationTarget: () => ({ position: "top", positionOverride: true }),
		liveRecommendationsFor: () => recommendation,
		livePositionValue: (value) => String(value || ""),
		livePositionDisplay: (value) => String(value || ""),
		positionLabel: (value) => String(value || "位置未知"),
		positionIcon: () => "",
		iconFigure: () => '<span class="champion-icon"></span>',
		rate: (value) => value === null || value === undefined || String(value).trim() === "" ? "—" : `${value}%`,
		percent: (value) => `${value}%`,
		number: (value) => String(value),
		liveAugmentRecommendationSource: () => "",
	});
	const missingPick = renderChampionRecommendationHeader(
		{ winRate: 51, pickRate: null, banRate: 3 },
		{ championName: "测试英雄", position: "top" },
		{},
	);
	assert.match(missingPick, /<dt>胜率<\/dt>/);
	assert.match(missingPick, /<dt>禁用率<\/dt>/);
	assert.doesNotMatch(missingPick, /选取率|—/);
	const presentPick = renderChampionRecommendationHeader(
		{ winRate: 51, pickRate: 0, banRate: 3 },
		{ championName: "测试英雄", position: "top" },
		{},
	);
	assert.match(presentPick, /<dt>选取率<\/dt><dd>0%<\/dd>/);
	const empty = renderChampionRecommendationHeader(
		{ winRate: null, pickRate: "", banRate: Number.NaN },
		{ championName: "测试英雄", position: "top" },
		{},
	);
	assert.doesNotMatch(empty, /champion-summary-stats|选取率|—/);

	const arenaDependencies = {
		state: { detail: null, arenaDetailLoading: false, arenaDetailError: null },
		championMeta: () => ({}),
		escapeHTML: (value) => String(value ?? ""),
		heroArtworkURL: () => "/art.png",
		heroArtworkFallbackURL: () => "/fallback.png",
		imageURL: () => "/portrait.png",
		tierBadge: () => "",
		tierDisplay: () => "A",
		percent: (value) => value === null || value === undefined || String(value).trim() === "" ? "—" : `${value}%`,
		number: (value) => String(value ?? ""),
		compactNumber: (value) => value === null || value === undefined || String(value).trim() === "" ? "—" : String(value),
		arenaOverviewMetric: () => "<metric></metric>",
		renderArenaSortBar: () => "<sort></sort>",
		renderDetailSkeleton: () => "",
		renderError: () => "",
		renderArenaFirstPlaces: () => "",
		renderArenaDetailContent: () => "",
	};
	const { renderArenaDetailPane } = compileFunctions(script, ["renderArenaDetailPane"], arenaDependencies);
	const arenaMarkup = renderArenaDetailPane({ championId: 1, name: "测试", pickRate: null, banRate: 4, play: 200 });
	const arenaSecondary = /<div class="arena-overview-secondary">([\s\S]*?)<\/div>/.exec(arenaMarkup)?.[1] || "";
	assert.match(arenaSecondary, /禁用率[\s\S]*4%/);
	assert.match(arenaSecondary, /样本[\s\S]*200/);
	assert.doesNotMatch(arenaSecondary, /选用率|—/);

	const { renderRuneWorkspace } = compileFunctions(script, ["allocateLoadoutSideRows", "renderRuneWorkspace"], {
		activeRunePage: () => 0,
		runeStyleIcon: () => "",
		compactNumber: (value) => String(value ?? ""),
		percent: (value) => `${value}%`,
		renderRuneTree: () => "",
		renderSpellsCard: () => "",
		renderSkillsCard: () => "",
		renderAssetButton: () => "<icon></icon>",
	});
	const sideCards = renderRuneWorkspace([], {
		starterItems: [{ assets: [{ id: 1 }], pickRate: null, winRate: 52 }],
		boots: [{ assets: [{ id: 2 }], pickRate: undefined, winRate: "" }],
	});
	assert.match(sideCards, /class="side-stats"[\s\S]*<dt>胜率<\/dt>/);
	assert.doesNotMatch(sideCards, /<dt>选用率<\/dt>/);
	assert.equal((sideCards.match(/class="side-stats"/g) || []).length, 1);

	const { renderSpellsCard } = compileFunctions(script, ["renderSpellsCard"], {
		assetImage: () => "<icon></icon>",
		compactNumber: (value) => value == null ? "—" : String(value),
		percent: (value) => value == null ? "—" : `${value}%`,
	});
	const emptySpells = renderSpellsCard({ summonerSpells: [{ assets: [], games: null, pickRate: "", winRate: undefined }] });
	assert.doesNotMatch(emptySpells, /spell-option-stats|选用率|—/);
	assert.match(renderSpellsCard({ summonerSpells: [{ assets: [], pickRate: 7 }] }), /<dt>选用率<\/dt><dd class="metric-pick">7%<\/dd>/);

	const { renderSkillsCard } = compileFunctions(script, ["renderSkillsCard"], {
		percent: (value) => value == null ? "—" : `${value}%`,
		renderChampionSkillPlan: () => "<plan></plan>",
	});
	const partialSkills = renderSkillsCard({ skills: [{ pickRate: null, winRate: 53 }] });
	assert.match(partialSkills, /<span>胜率 <b class="metric-win">53%<\/b><\/span>/);
	assert.doesNotMatch(partialSkills, /选用率|—/);
	const emptySkills = renderSkillsCard({ skills: [{ pickRate: null, winRate: "" }] });
	assert.doesNotMatch(emptySkills, /skill-head-stats|选用率|—/);
});

test("player header retains region and uses friend presence without CN spectator probes", () => {
 const functions = compileFunctions(gameplayScript, ["summonerRegionChip"], {riotTab:()=>false,escapeHTML:String,tabServerTitle:()=>"黑色玫瑰 HN10",tabServerLabel:()=>"黑色玫瑰"});
 assert.match(functions.summonerRegionChip({}), /黑色玫瑰/);
 assert.match(gameplayScript, /friendPresenceForTab|player-live-chip|deep-legends:friends-presence/);
 assert.match(gameplayStyles, /player-live-chip/);
 assert.doesNotMatch(functionSource(gameplayScript, "updateFriendPresenceChips"), /loadOverview|loadOverviewCurrentGame|renderOverview|\bapi\(/);
});

test("R46 restores only a persisted champion ID and clears unavailable selections", () => {
	const state = {
		selected: null,
		detailChampionID: 266,
		mode: "ranked",
		mayhemView: "champions",
		rankings: { rows: [{ championId: 266, key: "aatrox" }] },
	};
	const writes = [];
	const opened = [];
	let renders = 0;
	const functions = compileFunctions(script, ["restorePersistedChampionSelection", "renderLoadedWorkspace"], {
		state,
		rankingRows: () => state.rankings.rows,
		writeSetting: (key, value) => writes.push([key, value]),
		openDetail: (row) => { opened.push(row.championId); },
		prepareArenaSelection: () => null,
		prepareMayhemSelection: () => null,
		render: () => { renders += 1; },
		loadArenaDetail: () => {},
		loadMayhemDetail: () => {},
	});
	functions.renderLoadedWorkspace();
	assert.deepEqual(opened, [266]);
	assert.equal(renders, 0);
	assert.deepEqual(writes, []);

	state.selected = null;
	state.detailChampionID = 999;
	functions.renderLoadedWorkspace();
	assert.deepEqual(opened, [266]);
	assert.equal(renders, 1);
	assert.equal(state.detailChampionID, 0);
	assert.deepEqual(writes, [["champion-detail-id-ranked", ""]]);

	assert.match(functionSource(script, "openDetail"), /state\.detailChampionID = championID;[\s\S]*writeSetting\(`champion-detail-id-\$\{state\.mode\}`, championID\)/);
	assert.match(functionSource(script, "closeDetail"), /state\.detailChampionID = 0;[\s\S]*writeSetting\(`champion-detail-id-\$\{state\.mode\}`, ""\)/);
	assert.doesNotMatch(functionSource(script, "resetTransientChampionState"), /detailChampionID\s*=/);
	for (const key of ["champion-loading", "champion-detail", "champion-error", "champion-request-token"]) {
		assert.doesNotMatch(script, new RegExp(`readSetting\\("${key}"`));
	}
});

test("R61 champion re-entry renders persisted detail on the first frame", async () => {
	const dom = new JSDOM('<main id="champion-root"></main>');
	const rootNode = dom.window.document.getElementById("champion-root");
	const snapshots = [];
	const state = {
		section: "champions", mode: "ranked", mayhemView: "champions", detailChampionID: 266,
		selected: null, rankings: { rows: [{ championId: 266, key: "aatrox" }] }, preload: null,
	};
	let functions;
	const render = () => {
		rootNode.innerHTML = state.selected ? '<section class="champion-detail-content"></section>' : '<table class="champion-table"></table>';
		snapshots.push(rootNode.innerHTML);
	};
	functions = compileFunctions(script, ["restorePersistedChampionSelection", "renderLoadedWorkspace", "enterChampionSection"], {
		state,
		rankingRows: () => state.rankings.rows,
		writeSetting: () => {},
		openDetail: (row) => { state.selected = row; render(); return Promise.resolve(); },
		prepareArenaSelection: () => null,
		prepareMayhemSelection: () => null,
		render,
		loadArenaDetail: () => {},
		loadMayhemDetail: () => {},
		adoptCatalog: () => {},
		stampRankings: () => {},
		rankingsFresh: () => true,
		loadWorkspace: () => {},
	});
	await functions.enterChampionSection();
	assert.match(snapshots[0], /champion-detail-content/);
	assert.doesNotMatch(snapshots[0], /champion-table/);
});

test("R61 stale rankings re-entry runs loadWorkspace(true) without flashing the list", async () => {
	const dom = new JSDOM('<main id="champion-root"></main>');
	const rootNode = dom.window.document.getElementById("champion-root");
	const snapshots = [];
	const apiCalls = [];
	let resolveRankings;
	const rankingsGate = new Promise((resolve) => { resolveRankings = resolve; });
	const row = { championId: 266, key: "aatrox" };
	const state = {
		section: "champions", mode: "ranked", tier: "emerald_plus", position: "all", mayhemView: "champions",
		detailChampionID: 266, selected: null, rankings: { rows: [row] }, rankingsKey: "ranked|diamond_plus|all",
		rankingsAt: Date.now(), preload: null, preloaded: {}, catalog: { tiers: [], champions: [] },
		workspaceRequestToken: 0, loading: false, error: "",
	};
	const render = () => {
		rootNode.innerHTML = state.selected ? '<section class="champion-detail-content"></section>' : '<table class="champion-table"></table>';
		snapshots.push(rootNode.innerHTML);
	};
	const api = (url) => {
		apiCalls.push(url);
		if (url === "/api/champions/catalog") return Promise.resolve({ tiers: [], champions: [] });
		return rankingsGate;
	};
	const functions = compileFunctions(script, [
		"rankingsCacheKey", "stampRankings", "rankingsFresh", "restorePersistedChampionSelection",
		"renderLoadedWorkspace", "enterChampionSection", "loadWorkspace",
	], {
		state,
		rankingRows: () => state.rankings.rows,
		writeSetting: () => {},
		openDetail: (selected) => { state.selected = selected; render(); return Promise.resolve(); },
		prepareArenaSelection: () => null,
		prepareMayhemSelection: () => null,
		render,
		loadArenaDetail: () => {},
		loadMayhemDetail: () => {},
		adoptCatalog: () => {},
		api,
	});

	assert.equal(functions.rankingsFresh(), false, "fixture must exercise the stale rankings branch");
	await functions.enterChampionSection();
	assert.equal(state.loading, true, "loadWorkspace must be running while the rankings request is held");
	assert.deepEqual(apiCalls, [
		"/api/champions/catalog",
		"/api/champions/rankings?mode=ranked&tier=emerald_plus&position=all",
	], "force=true must refresh catalog even when a cached catalog exists");
	assert.match(snapshots[0], /champion-detail-content/);
	assert.doesNotMatch(snapshots[0], /champion-table/);

	resolveRankings({ rows: [row] });
	await new Promise((resolve) => setImmediate(resolve));
});

test("R61 detail cache survives leaving and prevents a second detail request", async () => {
	const state = {
		mode: "ranked", tier: "emerald_plus", position: "all", selected: null, detail: null,
		detailChampionID: 0, detailPosition: null, detailCache: new Map(), detailRequestToken: 0,
		workspaceRequestToken: 0, arenaRequestToken: 0, mayhemRequestToken: 0, mayhemAugmentRequestToken: 0,
	};
	let requests = 0;
	const functions = compileFunctions(script, ["openDetail", "resetTransientChampionState"], {
		state,
		normalizeChampionDetailID: Number,
		writeSetting: () => {},
		selectArenaChampion: () => {},
		selectMayhemChampion: () => {},
		championMeta: () => ({ slug: "aatrox" }),
		normalizeRunePage: () => 0,
		readSetting: (_key, fallback) => fallback,
		firstPositionOf: () => "top",
		appScroll: { scrollTop: 0, scrollTo: () => {} },
		render: () => {},
		root: { querySelector: () => null },
		api: async () => { requests += 1; return { position: "top", build: {} }; },
		closeMayhemTierDialog: () => {},
		closeArenaTierDialog: () => {},
		resetArenaControls: () => {},
		normalizePosition: (value) => value,
	});
	const row = { championId: 266, key: "aatrox", position: "top" };
	await functions.openDetail(row);
	functions.resetTransientChampionState({ restorePosition: true });
	await functions.openDetail(row);
	assert.equal(requests, 1);
	assert.equal(state.detailCache.size, 1);
});

test("same champion route click overrides the previous detail position", async () => {
  const state = {
    mode: "ranked", tier: "emerald_plus", position: "all", selected: null, detail: null,
    detailChampionID: 0, detailPosition: null, detailCache: new Map(), detailRequestToken: 0,
  };
  const requests = [];
  const functions = compileFunctions(script, ["openDetail"], {
    state,
    normalizeChampionDetailID: Number,
    writeSetting: () => {},
    selectArenaChampion: () => {},
    selectMayhemChampion: () => {},
    championMeta: () => ({ slug: "aatrox" }),
    normalizeRunePage: () => 0,
    readSetting: (_key, fallback) => fallback,
    firstPositionOf: (row) => ["top", "jungle", "mid", "adc", "support"].includes(row?.position) ? row.position : "",
    appScroll: { scrollTop: 0, scrollTo: () => {} },
    render: () => {},
    root: { querySelector: () => null },
    api: async (url) => {
      requests.push(url);
      const position = new URL(`http://local${url}`).searchParams.get("position");
      return { position, build: {} };
    },
  });
  await functions.openDetail({ championId: 266, key: "aatrox", position: "top" });
  await functions.openDetail({ championId: 266, key: "aatrox", position: "adc" });
  assert.equal(state.detailPosition, "adc");
  assert.match(requests[0], /[?&]position=top(?:&|$)/);
  assert.match(requests[1], /[?&]position=adc(?:&|$)/);
});

test("item route limit follows the active bottom or adc detail before the selected row", () => {
  const state = { detail: null, detailPosition: "bottom", selected: { position: "top" }, position: "top" };
  const { buildItemRoutes } = compileFunctions(script, ["buildItemRoutes"], {
    state, ADC_ITEM_ROUTE_LIMIT: 7, DEFAULT_ITEM_ROUTE_LIMIT: 6, CORE_RECOMMENDATION_LIMIT: 5,
  });
  const build = { coreItems: [{ assets: Array.from({ length: 8 }, (_, index) => ({ path: `/item-${index}` })) }] };
  assert.equal(buildItemRoutes(build)[0].assets.length, 7);
  state.detailPosition = "adc";
  assert.equal(buildItemRoutes(build)[0].assets.length, 7);
  state.detail = { position: "top" };
  assert.equal(buildItemRoutes(build)[0].assets.length, 6, "loaded detail position must be authoritative");
});

test("R61 detail tier persists and keeps the refreshed rankings cache fresh", () => {
	const stored = { "champion-tier": "emerald_plus" };
	const state = {
		mode: "ranked", tier: "emerald_plus", position: "mid", selected: { championId: 13, position: "mid" },
		detailPosition: "mid", detail: { position: "mid" }, rankings: { rows: [{}] }, rankingsKey: "", rankingsAt: 0,
	};
	const functions = compileFunctions(script, ["rankingsCacheKey", "rankingsFresh", "switchDetailTier"], {
		state,
		normalizeTier: (value) => value,
		writeSetting: (key, value) => { stored[key] = value; },
		refreshRankingsForDetailTier: () => { state.rankingsKey = `${state.mode}|${state.tier}|${state.position}`; state.rankingsAt = Date.now(); },
		firstPositionOf: () => "mid",
		openDetail: () => {},
		render: () => {},
		switchDetailPosition: () => {},
	});
	functions.switchDetailTier("diamond_plus");
	state.tier = stored["champion-tier"];
	assert.equal(state.tier, "diamond_plus");
	assert.equal(functions.rankingsFresh(), true);
	assert.match(script, /playerDetour:\s*false/);
	assert.match(script, /state\.playerDetour = true;[\s\S]*deep-legends:open-player/);
});

test("R70 section event runs after synchronous panel activation without snapshot delay", () => {
	const source = functionSource(appScript, "activateSection");
	assert.match(source, /const applyPanels = \(\) => \{[\s\S]*window\.dispatchEvent\(new CustomEvent\("deep-legends:section"[\s\S]*\};[\s\S]*\bapplyPanels\(\)/);
	assert.ok(source.indexOf('window.dispatchEvent(new CustomEvent("deep-legends:section"') < source.indexOf("    applyPanels();"));
	assert.doesNotMatch(source, /startViewTransition/);
});

test("R47 ranked detail recovers to the list when champion metadata is unavailable", async () => {
  const state = {
    mode: "ranked",
    selected: { championId: 1 },
    detail: { stale: true },
    detailChampionID: 266,
    loading: true,
    error: "stale error",
  };
  const writes = [];
  let renders = 0;
  const { openDetail } = compileFunctions(script, ["openDetail"], {
    state,
    normalizeChampionDetailID: (value) => Number(value) || 0,
    writeSetting: (key, value) => writes.push([key, value]),
    championMeta: () => null,
    render: () => { renders += 1; },
  });
  await openDetail({ championId: 266, key: "" });
  assert.equal(state.selected, null);
  assert.equal(state.detail, null);
  assert.equal(state.detailChampionID, 0);
  assert.equal(state.loading, false);
  assert.equal(state.error, "");
  assert.equal(renders, 1);
  assert.deepEqual(writes, [["champion-detail-id-ranked", 266], ["champion-detail-id-ranked", ""]]);
});

test("champion detail core routes hide pick rate and retain win rate with games", () => {
  const rankedBuild = functionSource(script, "renderRankedBuild");
  const genericBuild = functionSource(script, "renderBuildBoard");
  assert.match(rankedBuild, /renderConfigOption\(row, "route", null, renderDepthStats\)/);
  assert.doesNotMatch(rankedBuild, /renderOptionStats\(row\)/);
	assert.match(genericBuild, /renderConfigOption\(row, "route", null, renderDepthStats\)/);
	const { renderDepthStats } = compileFunctions(script, ["renderDepthStats"], {
		percent: (value) => `${value}%`,
		compactNumber: (value) => String(value),
	});
	const core = renderDepthStats({ pickRate: 44.98, winRate: 59.65, games: 10355 });
	assert.doesNotMatch(core, /选取率|44\.98%/);
	assert.match(core, /胜率[\s\S]*59\.65%[\s\S]*场次[\s\S]*10355/);
});

test("R63 keeps the OP.GG source only in the build header", () => {
	const { championItemAttemptSummary, renderBuildDepthGroups } = compileFunctions(script, ["championItemAttemptSummary", "renderBuildDepthGroups"], {
		escapeHTML: (value) => String(value ?? ""),
		renderConfigOption: () => "",
	});
	const depthMarkup = renderBuildDepthGroups({ itemChainStatus: "ready", itemSource: "OP.GG", itemWindow: "当前版本", fourthItems: [], fifthItems: [] });
	assert.doesNotMatch(depthMarkup, /item-chain-source|OP\.GG · 当前版本/);
	assert.match(functionSource(script, "renderRankedBuild"), /build-primary-source">OP\.GG · 当前版本/);
});

test("QQ101 item depths disclose unavailable samples and flag extreme rates", () => {
  const { renderDepthStats } = compileFunctions(script, ["renderDepthStats"], {
    percent: (value) => `${value}%`,
    compactNumber: (value) => Number(value) > 0 ? String(value) : "—",
  });
  const unavailable = renderDepthStats({ winRate: 61.58, pickRate: 25.89, gamesUnavailable: true });
  assert.match(unavailable, /样本量/);
  assert.match(unavailable, /腾讯官方数据不提供样本量/);
  assert.match(unavailable, /未提供/);
  assert.doesNotMatch(unavailable, /<dd>—<\/dd>/);

  const fullWin = renderDepthStats({ winRate: 100, pickRate: 100, gamesUnavailable: true });
  assert.match(fullWin, /is-low-confidence/);
  assert.match(fullWin, /样本极少/);
  const zeroWin = renderDepthStats({ winRate: 0, pickRate: 100, gamesUnavailable: true });
  assert.match(zeroWin, /<dd>0%<\/dd>/);
  assert.match(zeroWin, /样本极少/);

  const opgg = renderDepthStats({ winRate: 50, games: 18 });
  assert.match(opgg, /<dt>场次<\/dt><dd>18<\/dd>/);
  assert.doesNotMatch(opgg, /腾讯官方数据不提供样本量|样本极少|is-low-confidence/);
});

test("R61 unnamed loot remains visible by raw ID with a completion hint", () => {
	const functions = compileFunctions(appScript, ["lootToken", "lootName", "lootNamePending", "lootTypeLabel", "lootImagePaths", "lootCategorySlug", "lootCategoryIcon", "lootCard"], {
		escapeHTML: (value) => String(value ?? ""),
		formatNumber: (value) => String(value),
	});
	const markup = functions.lootCard({ lootId: "CHEST_224", localizedName: "未命名战利品", count: 1 });
	assert.match(markup, /CHEST_224/);
	assert.match(markup, /客户端数据暂未同步，可稍后重试/);
	assert.match(markup, /is-name-pending/);
	assert.doesNotMatch(markup, />未命名战利品</);
	const emptyShell = functions.lootCard({ lootId: "", lootName: "", type: "", localizedDescription: "不得展示的说明", count: 30 });
	assert.match(emptyShell, /客户端返回的空白条目/);
	assert.doesNotMatch(emptyShell, /名称待补全|is-name-pending|不得展示的说明/);
	assert.match(appScript, /const displayLoot = \[\.\.\.loot\];/);
	assert.deepEqual(functions.lootImagePaths({ lootId: "CHEST_generic", asset: "/old/chest_generic.png" }), ["/loot-icons/promotion-chest.png", "/old/chest_generic.png"]);
	assert.deepEqual(functions.lootImagePaths({ lootId: "CHEST_promotion" }), ["/loot-icons/promotion-chest.png"]);
});

test("0912 named ward, icon and legacy loot render their resolved names", () => {
  const functions = compileFunctions(appScript, ["lootToken", "lootName", "lootNamePending", "lootTypeLabel", "lootImagePaths", "lootCategorySlug", "lootCategoryIcon", "lootCard"], {
    escapeHTML: (value) => String(value ?? ""), formatNumber: (value) => String(value),
  });
  const fixtures = (name) => JSON.parse(fs.readFileSync(path.join(root, "testdata", "loot-names-0912", name), "utf8"));
  const translations = fixtures("trans.json");
  const items = [
    ...fixtures("ward-skins.json").map((ward) => ({ lootId: `WARD_SKIN_RENTAL_${ward.id}`, displayName: ward.name, asset: ward.wardImagePath, category: "守卫" })),
    ...fixtures("summoner-icons.json").map((icon) => ({ lootId: `SUMMONER_ICON_${icon.id}`, displayName: icon.title, asset: icon.imagePath, category: "图标" })),
    ...["chest_128", "material_clashtickets"].map((id) => ({ lootId: id.toUpperCase(), displayName: translations[`loot_name_${id}`], asset: `/fe/lol-loot/assets/loot_item_icons/${id}.png` })),
  ];
  assert.equal(items.length, 7);
  for (const item of items) {
    const markup = functions.lootCard({ ...item, localizedName: "未命名战利品", count: 1 });
    assert.ok(markup.includes(item.displayName), item.lootId);
    assert.doesNotMatch(markup, /名称待补全|is-name-pending|>未命名战利品</);
    assert.ok(functions.lootImagePaths(item).includes(item.asset));
  }
});

test("R61 account renderer directly retains unnamed loot and kills the old filter", async () => {
	const renderAccountLoot = async (source) => {
		const accountContent = { innerHTML: "", querySelectorAll: () => [] };
		const state = { status: { connected: true, eventStream: true }, destroyed: false };
		const { loadAccount } = compileFunctions(source, ["loadAccount"], {
			state,
			el: { accountContent, accountLiveState: { textContent: "", className: "" } },
			api: async () => ({
				summoner: { summonerLevel: 1 },
				account: { loot: [{ lootId: "CHEST_224", localizedName: "未命名战利品", category: "材料", count: 1 }] },
				rewards: [], capabilities: [],
			}),
			lootCategorySlug: () => "material",
			lootCategoryIcon: () => "◇",
			lootCard: (item) => `<article>${item.lootId}</article>`,
			escapeHTML: (value) => String(value ?? ""),
			formatNumber: (value) => String(value ?? 0),
			playerName: () => "测试玩家",
			capabilityName: (value) => value,
			sourceStateLabel: (value) => value,
			rewardStatusLabel: (value) => value,
			formatDateTime: (value) => value,
			renderPanelError: (_node, _title, error) => { throw error; },
			loadNextLootImage: () => {},
			window: { deepLegendsGameIcons: { iconFigure: () => "", prepareImages: () => {} } },
		});
		await loadAccount();
		return accountContent.innerHTML;
	};
	assert.match(await renderAccountLoot(appScript), /CHEST_224/);
	const oldFilter = appScript.replace(
		"const displayLoot = [...loot];",
		'const displayLoot = loot.filter((item) => item.displayName && item.displayName !== "未命名战利品");',
	);
	assert.notEqual(oldFilter, appScript, "B-1 mutation target not found");
	await assert.rejects(async () => assert.match(await renderAccountLoot(oldFilter), /CHEST_224/));
});

test("R61 every mandatory contract rejects its documented production mutation", () => {
	const originals = {
		sgp: sgpBackend,
		gameplayGo: gameplayBackend,
		lcu: lcuAPIBackend,
		lcuEvents: lcuEventsBackend,
		connection: connectionManagerBackend,
		catalog: catalogBackend,
		structured: structuredBackend,
		qq101: qq101Backend,
		championsGo: backend,
		appJS: appScript,
		championsJS: script,
		gameplayJS: gameplayScript,
	};
	const assertContracts = (sources) => {
		assert.match(sources.sgp, /for retry := 0; retry <= 2; retry\+\+/); // A-1
		assert.match(sources.sgp, /retryableSGPStatus\(response\.StatusCode\)/);
		const historyPageKey = goFunctionSource(sources.sgp, "sgpHistoryPageCacheKey"); // A-2
		assert.match(historyPageKey, /fmt\.Sprintf\("%s\|%s\|%d\|%d\|%s", serverID, puuid, startIndex, pageSize, strings\.Join\(tags, ","\)\)/);
		assert.match(sources.sgp, /cachedHistoryPage\(serverID, puuid, pageStart, pageSize, tags\)/);
		assert.match(sources.sgp, /cacheHistoryPage\(serverID, puuid, pageStart, pageSize, tags/);
		assert.match(sources.gameplayGo, /if timeout == nil\s*\{\s*timeout = context\.WithTimeout/);
		assert.match(sources.gameplayGo, /timeout\(r\.Context\(\), budget\)/); // A-3
		assert.match(sources.sgp, /sgpPageSize\s+= 50/); // A-4
		assert.match(sources.sgp, /shouldSample = cost\.claimParticipantShapeSample\(\)/); // A-5
		assert.match(sources.gameplayGo, /"event": "tencent_riot_id_lookup"/); // A-6
		assert.match(sources.gameplayGo, /所选服务器没有找到该玩家/);
		assert.match(sources.gameplayJS, /const requestKey = `\$\{append \? "overview-more" : "overview"\}:\$\{tab\.key\}`/); // A-7 frontend key
		assert.match(sources.sgp, /return "canceled"/); // A-7 backend kind
		assert.match(sources.gameplayGo, /pagination\.Partial = partialErr != nil/); // A-8
		assert.match(sources.gameplayGo, /pagination\.HasMore = true/);
		assert.match(sources.sgp, /"start_index": startIndex/); // A-9
		assert.match(sources.sgp, /"count":\s+count/);
		assert.match(sources.lcuEvents, /stat\.Count\+\+/); // A-10
		assert.match(sources.connection, /dropped\["top_uris"\] = streamErr\.TopURIs/);
		assert.match(sources.gameplayGo, /"identity", "queue_labels", "champion_names", "detailed_matches", "season_snapshot",\s*"ranks", "mastery", "recent_ranked", "recent_players", "serialize"/); // A-11
		assert.match(sources.gameplayGo, /"event": "overview_phases_ms"/);

		assert.match(sources.appJS, /const displayLoot = \[\.\.\.loot\];/); // B-1
		assert.match(sources.lcu, /global\/zh_cn\/v1\/loot\.json/); // B-2
		assert.match(sources.lcu, /"CHEST_PROMOTION":\s+"紫色宝箱"/);
		assert.match(sources.lcu, /"event": "loot_category_assigned"/); // B-3
		for (const event of ["loot_map_shape", "loot_name_fallback", "loot_category_assigned"]) { // B-4
			assert.match(sources.lcu, new RegExp(`"event": "${event}"`));
		}
		const lootEnrichment = goFunctionSource(sources.lcu, "enrichLootItemsWithMetadata");
		assert.match(lootEnrichment, /"loot_id_prefix":\s+lootIDPrefix\(item\.LootID\)/);
		assert.doesNotMatch(lootEnrichment, /"loot_id":\s+item\.LootID/);
		assert.doesNotMatch(lootEnrichment, /"event": "loot_name_fallback"[\s\S]{0,400}"count":\s+item\.Count/);
		assert.match(sources.catalog, /NewObservedLootAPI\(client, observe\)\.PlayerLoot\(\)/);

		const depthResolver = goFunctionSource(sources.structured, "resolveRankedItemDepths");
		assert.match(depthResolver, /opggDepths, depthFetchedAt, opggErr := p\.loadOPGGDepthRows/); // C-1
		assert.match(depthResolver, /if opggErr == nil/);
		assert.match(sources.structured, /"event": "opgg_item_depths_parsed_zero"/);
		assert.doesNotMatch(sources.structured, /"event": "opgg_item_depths_failed"[^\n]+"errorKind"/);
		assert.match(sources.structured, /roleRate = percentOf\(raw\.Stats\.Play, payload\.Data\.Summary\.AverageStats\.Play\)/); // C-2
		assert.match(goFunctionSource(sources.qq101, "mergeQQ101PositionShares"), /if !merged\[item\.Position\]/); // C-3
		const rankedBuild = functionSource(sources.championsJS, "renderRankedBuild");
		const depthGroups = functionSource(sources.championsJS, "renderBuildDepthGroups");
		assert.match(rankedBuild, /build-primary-source">OP\.GG · 当前版本/); // C-4
		assert.doesNotMatch(rankedBuild, /\$\{sourceNote\}/);
		assert.doesNotMatch(depthGroups, /item-chain-source|sourceNote/);
		assert.match(sources.championsJS, /R60 cleanup marker: GamesUnavailable/); // C-6
		assert.match(sources.championsGo, /R60 cleanup marker: this legacy HTML parser/);
		assert.match(sources.qq101, /R60 cleanup marker: _runeinfo and _skill/);

		const enter = functionSource(sources.championsJS, "enterChampionSection");
		assert.ok(enter.indexOf("restorePersistedChampionSelection()") < enter.indexOf("render();")); // D-1
		assert.doesNotMatch(functionSource(sources.championsJS, "resetTransientChampionState"), /detailCache\.clear/); // D-2
		assert.match(functionSource(sources.championsJS, "switchDetailTier"), /writeSetting\("champion-tier", nextTier\)/); // D-3
		const activate = functionSource(sources.appJS, "activateSection");
		assert.match(activate, /const applyPanels = \(\) => \{[\s\S]*window\.dispatchEvent\(new CustomEvent\("deep-legends:section"[\s\S]*\bapplyPanels\(\)/); // D-4
		assert.match(sources.championsJS, /playerDetour:\s*false/); // D-5
		assert.match(sources.championsJS, /state\.playerDetour = true;[\s\S]*deep-legends:open-player/);
	};
	assertContracts(originals);
	const mutations = [
		["A-1 retry budget", "sgp", "for retry := 0; retry <= 2; retry++", "for retry := 0; retry < 1; retry++"],
		["A-2 pageSize key dimension", "sgp", 'fmt.Sprintf("%s|%s|%d|%d|%s", serverID, puuid, startIndex, pageSize, strings.Join(tags, ","))', 'fmt.Sprintf("%s|%s|%d|%s", serverID, puuid, startIndex, strings.Join(tags, ","))'],
		["A-3 overview deadline", "gameplayGo", "timeout(r.Context(), budget)", "context.WithCancel(r.Context())"],
		["A-4 50-row cold page", "sgp", "sgpPageSize         = 50", "sgpPageSize         = 20"],
		["A-5 one-shot participant sample", "sgp", "shouldSample = cost.claimParticipantShapeSample()", "shouldSample = true"],
		["A-6 lookup diagnostic", "gameplayGo", '"event": "tencent_riot_id_lookup"', '"event": "tencent_lookup_removed"'],
		["A-7 append request key", "gameplayJS", 'const requestKey = `${append ? "overview-more" : "overview"}:${tab.key}`;', 'const requestKey = `overview:${tab.key}`;'],
		["A-7 canceled kind", "sgp", 'return "canceled"', 'return "other"'],
		["A-8 resumable partial", "gameplayGo", "pagination.HasMore = true", "pagination.HasMore = false"],
		["A-9 page fields", "sgp", '"start_index": startIndex', '"start_removed": startIndex'],
		["A-10 URI counters", "lcuEvents", "stat.Count++", "// stat.Count removed"],
		["A-11 complete phase set", "gameplayGo", '"ranks", "mastery", "recent_ranked"', '"ranks", "mastery_removed", "recent_ranked"'],
		["B-1 retain unknown loot", "appJS", "const displayLoot = [...loot];", "const displayLoot = loot.filter((item) => lootName(item) !== item.lootId);"],
		["B-2 hard-coded promotion fallback", "lcu", '"CHEST_PROMOTION":        "紫色宝箱"', '"CHEST_PROMOTION_REMOVED": "紫色宝箱"'],
		["B-3 category diagnostic", "lcu", '"event": "loot_category_assigned"', '"event": "loot_category_removed"'],
		["B-4 fallback diagnostic privacy", "lcu", '"loot_id_prefix": lootIDPrefix(item.LootID)', '"loot_id_prefix": item.LootID'],
		["C-1 OP.GG priority", "structured", "if opggErr == nil {", "if qq101Ready {"],
		["C-1 parsed-zero diagnostic", "structured", '"event": "opgg_item_depths_parsed_zero"', '"event": "opgg_item_depths_failed"'],
		["C-2 role-rate fallback", "structured", "roleRate = percentOf(raw.Stats.Play, payload.Data.Summary.AverageStats.Play)", "roleRate = 0"],
		["C-3 QQ101 append-only", "qq101", "if !merged[item.Position] {", "if merged[item.Position] {"],
		["C-4 source label placement", "championsJS", '<section class="build-depth-column"><h4><span>${label}</span></h4>', '<section class="build-depth-column"><span class="item-chain-source">${build?.itemSource} · ${build?.itemWindow}</span><h4><span>${label}</span></h4>'],
		["C-6 cleanup annotations", "championsJS", "R60 cleanup marker: GamesUnavailable", "cleanup marker removed"],
		["D-1 restore before render", "championsJS", "const restored = restorePersistedChampionSelection();\n      if (restored) state.selected = restored;\n      render();", "render();\n      const restored = restorePersistedChampionSelection();\n      if (restored) state.selected = restored;"],
		["D-2 preserve detail cache", "championsJS", "function resetTransientChampionState({ restorePosition = false } = {}) {\n    closeMayhemTierDialog(false);", "function resetTransientChampionState({ restorePosition = false } = {}) {\n    closeMayhemTierDialog(false);\n    state.detailCache.clear();"],
		["D-3 persist detail tier", "championsJS", 'writeSetting("champion-tier", nextTier);', 'void nextTier;'],
		["D-4 transition callback timing", "appJS", 'window.dispatchEvent(new CustomEvent("deep-legends:section", { detail: { name } }));', '// section event removed from applyPanels'],
		["D-5 player detour state", "championsJS", "state.playerDetour = true;", "state.playerDetour = false;"],
	];
	for (const [label, key, before, after] of mutations) {
		const mutatedValue = originals[key].replace(before, after);
		assert.notEqual(mutatedValue, originals[key], `${label}: mutation target not found`);
		assert.throws(() => assertContracts({ ...originals, [key]: mutatedValue }), label);
	}
});

test("R63 mandatory contracts reject every documented production regression", () => {
	const originals = {
		structured: structuredBackend,
		championsJS: script,
		gameplayJS: gameplayScript,
		gameplayGo: gameplayBackend,
		rankGo: rankInsightsBackend,
		lcuGo: lcuBackend,
		seasonGo: seasonStatsBackend,
		mainGo: mainSource,
		lootGo: lcuAPIBackend,
	};
	const assertContracts = (sources) => {
		const ordering = goFunctionSource(sources.structured, "orderStructuredRecommendationCandidates");
		assert.match(ordering, /leftPick := candidates\[i\]\.row\.PickRate[\s\S]*rightPick := candidates\[j\]\.row\.PickRate[\s\S]*return leftPick > rightPick/); // A-1
		const depthLoader = goFunctionSource(sources.structured, "loadOPGGDepthRows");
		assert.match(depthLoader, /patch, freshnessMarker/); // A-3 shared version + snapshot marker
		assert.match(goFunctionSource(sources.structured, "resolveRankedItemDepths"), /depthFetchedAt\.Before\(response\.FetchedAt\)/);
		// Both cached and freshly fetched depths must preserve the oldest time.
		// Matching one Before() alone lets a mutant in the other branch survive.
		assert.doesNotMatch(goFunctionSource(sources.structured, "resolveRankedItemDepths"), /depthFetchedAt\.After\(response\.FetchedAt\)/);
		assert.doesNotMatch(functionSource(sources.championsJS, "renderBuildDepthGroups"), /item-chain-source|sourceNote/); // A-4
		assert.match(functionSource(sources.championsJS, "renderRankedBuild"), /build-primary-source">OP\.GG · 当前版本/);

		assert.match(goFunctionSource(sources.lcuGo, "RequestJSON"), /httptrace\.WithClientTrace\(ctx, requestTrace\.clientTrace\(\)\)/); // B-1
		assert.match(goFunctionSource(sources.lcuGo, "getBytes"), /httptrace\.WithClientTrace\(ctx, requestTrace\.clientTrace\(\)\)/);
		assert.match(sources.gameplayGo, /value := a\.playerRankScore\(ctx,/); // B-2a overview
		assert.match(sources.gameplayGo, /a\.playerRankScore\(ctx, client, playerRef/); // B-2a live
		assert.match(goFunctionSource(sources.gameplayGo, "loadQueueLabelsContext"), /if client\.queueLabelsLoaded \{\s*result := cloneQueueLabels\(client\.queueLabels\)\s*client\.queueLabelsMu\.Unlock\(\)\s*return result/); // B-2b
		assert.doesNotMatch(functionSource(sources.gameplayJS, "updateFriendPresenceChips"), /loadOverview|renderOverview|\bapi\(/); // presence updates only its own fragment
		const tierScope = functionSource(sources.gameplayJS, "matchTierScope");
		assert.match(tierScope, /return `\$\{region\}:\$\{serverID\}:\$\{playerRef\}`/); // B-3
		assert.doesNotMatch(tierScope, /tab\?\.key|tab\.key/);
		assert.match(sources.rankGo, /rankScoreNegativeCacheTTL = 60 \* time\.Second/); // B-4
		assert.match(goFunctionSource(sources.rankGo, "playerRankScoreWithCacheStatus"), /entry\.negative = true[\s\S]*cache\.put\(cacheKey, entry\)/);
		assert.match(sources.rankGo, /var globalMatchTiersRankSemaphore = make\(chan struct\{\}, matchTiersRankConcurrency\)/);
		assert.match(functionSource(sources.gameplayJS, "hydrateMatchTiers"), /if \(failure && Number\(failure\.nextRetryAt \|\| 0\) > Date\.now\(\)\)/);
		assert.match(functionSource(sources.gameplayJS, "shouldReloadOverview"), />= 120_000/); // B-5
		assert.match(functionSource(sources.gameplayJS, "loadOverview"), /preserveLoadedPages = force[\s\S]*mergedMatches = \[\.\.\.freshMatches, \.\.\.previousMatches\.filter/);
		const overviewRenderer = functionSource(sources.gameplayJS, "renderOverviewBodyContent");
		assert.match(overviewRenderer, /const preserveMatchList = Boolean\(retainedMatchList/); // B-6
		assert.match(overviewRenderer, /preserveMatchList[\s\S]*retainedMatchList\.remove\(\)[\s\S]*container\.querySelector\("\.match-list"\)\?\.replaceWith\(retainedMatchList\)/);
		assert.match(goFunctionSource(sources.gameplayGo, "cacheRecentRankedSample"), /len\(a\.recentRankedSamples\) > recentRankedSampleCacheMax[\s\S]*removeRecentRankedSampleLocked/); // B-7
		assert.match(goFunctionSource(sources.seasonGo, "cacheSeasonQuerySnapshotLocked"), /len\(a\.seasonQuerySnapshots\) > seasonQuerySnapshotsMax[\s\S]*removeSeasonQuerySnapshotLocked/);
		assert.match(goFunctionSource(sources.mainGo, "recordDiagnostic"), /eventName == "ranked_winrate_resolved"[\s\S]*aggregateRankedWinrateDiagnostic/); // B-8

		assert.match(goFunctionSource(sources.structured, "loadStructuredCounters"), /structuredCountersForChampion\(payload\.Data\.Counters, id\)/); // C-1
		assert.match(sources.structured, /response\.Counters = p\.structuredCountersForChampion\(payload\.Data\.Counters, id\)/);
		assert.doesNotMatch(sources.structured, /opggCountersForPosition|countersPositionScoped/);
		const counters = goFunctionSource(sources.structured, "structuredCountersForChampion");
		assert.match(counters, /row\.WinRate < 50/); // C-2
		assert.match(counters, /rows\[index\]\.WinRate > 50/);

		const lootFallback = goFunctionSource(sources.lootGo, "enrichLootItemsWithMetadata");
		for (const field of ["raw_key_empty", "type_empty", "display_categories", "unnamed_type_counts"]) assert.match(lootFallback, new RegExp(`"${field}"`)); // D-2
		assert.doesNotMatch(lootFallback, /"loot_id":\s*item\.LootID|"count":\s*item\.Count/);
		for (const prefix of ["CHEST_", "MATERIAL_", "CURRENCY_", "CHAMPION_", "SKIN_", "STATSTONE_", "EMOTE_", "WARD_", "COMPANION_", "TFT_"]) assert.match(goFunctionSource(sources.lootGo, "lootIDPrefix"), new RegExp(`"${prefix}"`));
	};
	assertContracts(originals);
	const mutations = [
		["A-1 pick-rate order", "structured", "leftPick := candidates[i].row.PickRate\n\t\trightPick := candidates[j].row.PickRate", "leftPick := candidates[i].row.WinRate\n\t\trightPick := candidates[j].row.WinRate"],
		["A-3 shared snapshot key", "structured", "patch, freshnessMarker", "patch"],
		["A-3 honest oldest timestamp", "structured", "depthFetchedAt.Before(response.FetchedAt)", "depthFetchedAt.After(response.FetchedAt)"],
		["A-4 one source label", "championsJS", '<section class="build-depth-column"><h4><span>${label}</span></h4>', '<section class="build-depth-column"><span class="item-chain-source">${sourceNote}</span><h4><span>${label}</span></h4>'],
		["B-1 request trace", "lcuGo", "ctx = httptrace.WithClientTrace(ctx, requestTrace.clientTrace())", "// trace attachment removed"],
		["B-2a overview rank cache", "gameplayGo", "value := a.playerRankScore(ctx,", "value := directRankLookup(ctx,"],
		["B-2a live rank cache", "gameplayGo", "a.playerRankScore(ctx, client, playerRef", "a.directRankLookup(ctx, client, playerRef"],
		["B-2b queue cache", "gameplayGo", "if client.queueLabelsLoaded {", "if false {"],
		["B-3 stable tier scope", "gameplayJS", "return `${region}:${serverID}:${playerRef}`;", "return `${region}:${tab.key}:${serverID}:${playerRef}`;"],
		["B-4 negative cache", "rankGo", "entry.negative = true\n\t\tcache.put(cacheKey, entry)", "entry.negative = true"],
		["B-4 global concurrency gate", "rankGo", "var globalMatchTiersRankSemaphore = make(chan struct{}, matchTiersRankConcurrency)", "// per-handler gate restored"],
		["B-4 frontend backoff", "gameplayJS", "if (failure && Number(failure.nextRetryAt || 0) > Date.now())", "if (false)"],
		["B-5 two-minute freshness", "gameplayJS", ">= 120_000", ">= 20_000"],
		["B-5 preserve loaded pages", "gameplayJS", "const preserveLoadedPages = force && sameFilter && !pageWasPending && previousMatches.length > freshMatches.length;", "const preserveLoadedPages = false;"],
		["B-6 preserve match DOM", "gameplayJS", "const preserveMatchList = Boolean(retainedMatchList", "const preserveMatchList = Boolean(false && retainedMatchList"],
		["B-7 recent ranked LRU", "gameplayGo", "for len(a.recentRankedSamples) > recentRankedSampleCacheMax {", "for false {"],
		["B-7 season snapshot LRU", "seasonGo", "for len(a.seasonQuerySnapshots) > seasonQuerySnapshotsMax {", "for false {"],
		["B-8 minute aggregation", "mainGo", 'if eventName, _ := event["event"].(string); eventName == "ranked_winrate_resolved" {', "if false {"],
		["C-1 complete counter payload", "structured", "structuredCountersForChampion(payload.Data.Counters, id)", "structuredCountersForChampion(payload.Data.Summary.Positions[0].Counters, id)"],
		["C-2 weak sign", "structured", "row.WinRate < 50 && len(result.WeakAgainst) < 5", "len(result.WeakAgainst) < 5"],
		["C-2 strong sign", "structured", "rows[index].WinRate > 50", "rows[index].WinRate < 50"],
		["D-2 raw-key diagnostic", "lootGo", '"raw_key_empty":            item.rawKeyEmpty,', "// raw key diagnostic removed"],
		["D-2 runtime privacy", "lootGo", '"loot_id_prefix": lootIDPrefix(item.LootID)', '"loot_id": item.LootID'],
	];
	for (const [label, key, before, after] of mutations) {
		const mutated = originals[key].replace(before, after);
		assert.notEqual(mutated, originals[key], `${label}: mutation target not found`);
		assert.throws(() => assertContracts({ ...originals, [key]: mutated }), label);
	}
});

test("live rune secondary style row stays aligned beneath the primary keystone row", () => {
  const { renderUnifiedRuneBoard } = compileFunctions(gameplayScript, ["renderUnifiedRuneBoard"], {
    state: {
      perks: {
        styles: [{ id: 1, name: "主系", slots: [{ perks: [{ id: 11 }] }] }, { id: 2, name: "副系", slots: [{ perks: [{ id: 21 }] }, { perks: [{ id: 22 }] }] }],
        statModSlots: [],
      },
    },
    escapeHTML: (value) => String(value ?? ""),
    renderRuneStyleIcon: (style) => `<style>${style.id}</style>`,
    renderRuneOption: (perk) => `<perk>${perk.id}</perk>`,
    iconFigure: () => "",
  });
  const markup = renderUnifiedRuneBoard({ primaryStyleId: 1, subStyleId: 2, perkIds: [] });
  const secondary = markup.slice(markup.indexOf('class="unified-rune-column is-secondary"'));
  assert.ok(secondary.indexOf('class="unified-rune-row is-spacer"') < secondary.indexOf("<header"));
});

test("R16 build, rune, skill, and Korean overview guards preserve the corrected contracts", () => {
  assert.doesNotMatch(gameplayStyles, /--rune-icon-size:\s*(3[3-9]|[4-9]\d)px/);
  assert.match(gameplayStyles, /\.config-option\.is-route \.config-icons\s*\{[^}]*flex-wrap:\s*nowrap/s);
  assert.match(sharedBuildStyles, /--build-row-height:\s*64px/);
  assert.doesNotMatch(script, /class="route-row"/);
  assert.match(functionSource(script, "renderRankedBuild"), /renderConfigOption\(row, "route", null, renderDepthStats\)/);
  assert.match(styles, /\.champion-build-board \.config-option\s*\{[^}]*min-height:\s*var\(--build-row-height,\s*60px\)[^}]*flex-wrap:\s*nowrap/s);
  assert.match(gameplayStyles, /\.config-option\s*\{[^}]*min-height:\s*var\(--build-row-height,\s*60px\)/s);
	  assert.match(gameplayScript, /const timeout = riotTab\(tab\) \? 190_000 : 25_000;/);
  for (const slot of ["fourth", "fifth"]) {
    assert.match(gameplayScript, new RegExp(`inUpstreamOrder\\(build\\.${slot}Options \\|\\| \\[\\], 5\\)`));
  }

  const { renderDepthStats: renderChampionDepthStats } = compileFunctions(script, ["renderDepthStats"], {
    percent: (value) => `${value}%`,
    compactNumber: (value) => String(value),
  });
	const championDepth = renderChampionDepthStats({ pickRate: 12.3, winRate: 55.38, games: 6233 });
	assert.doesNotMatch(championDepth, /<dt>选取率<\/dt>/);
  assert.match(championDepth, /<dt>胜率<\/dt>/);
  assert.match(championDepth, /<dt>场次<\/dt>/);
  assert.match(script, /renderConfigOption\(row, "item", null, renderDepthStats\)/);

  const { renderDepthStats: renderLiveDepthStats, renderRecommendationStats } = compileFunctions(gameplayScript, ["renderDepthStats", "renderRecommendationStats", "renderOptionStats"], {
    rate: (value) => `${value}%`,
    compactNumber: (value) => String(value),
  });
  const liveDepth = renderLiveDepthStats({ winRate: 55.38, games: 6233 });
  assert.doesNotMatch(liveDepth, /选用率|选取率/);
  assert.match(liveDepth, /<dt>胜率<\/dt>/);
  assert.match(liveDepth, /<dt>场次<\/dt>/);
  assert.match(gameplayScript, /optionList\(options, "item", empty, renderDepthStats\)/);

  const recommendationStats = renderRecommendationStats({ pickRate: 12.3, winRate: 55.1, games: 88 });
  assert.match(recommendationStats, /class="option-stats"/);
  assert.doesNotMatch(recommendationStats, /class="recommendation-stats"/);
  const statOrder = [...recommendationStats.matchAll(/<dt>([^<]+)<\/dt>/g)].map((match) => match[1]);
  assert.deepEqual(statOrder, ["选取率", "胜率", "场次"]);
  assert.match(recommendationStats, /class="is-pick"/);
  assert.match(recommendationStats, /class="is-win"/);
  assert.match(recommendationStats, /class="is-games"/);
});

test("R16 addendum guards reject every documented regression on true copies", () => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-r16-guards-"));
  const webCopy = path.join(temp, "web");
  fs.mkdirSync(webCopy);
  for (const filename of ["champions.js", "champions.css", "gameplay.js", "gameplay.css", "build-item-row.css"]) {
    fs.copyFileSync(path.join(__dirname, filename), path.join(webCopy, filename));
  }
  fs.copyFileSync(path.join(root, "prestige_chromas.json"), path.join(temp, "prestige_chromas.json"));
  fs.cpSync(path.join(root, "..", "desktop"), path.join(temp, "desktop"), { recursive: true, filter: (source) => path.basename(source) !== "node_modules" });
  fs.cpSync(path.join(root, "data"), path.join(temp, "data"), { recursive: true });
  try {
    const read = (filename) => fs.readFileSync(path.join(webCopy, filename), "utf8");
    const originals = Object.fromEntries(["champions.js", "champions.css", "gameplay.js", "gameplay.css", "build-item-row.css"].map((filename) => [filename, read(filename)]));
    assert.equal(fs.lstatSync(path.join(webCopy, "champions.js")).isSymbolicLink(), false);
    assert.ok(fs.existsSync(path.join(temp, "desktop", "package.json")));
    assert.ok(fs.existsSync(path.join(temp, "prestige_chromas.json")));
    assert.ok(fs.existsSync(path.join(temp, "data")));

    fs.writeFileSync(path.join(webCopy, "gameplay.css"), originals["gameplay.css"].replace("--rune-icon-size: 32px", "--rune-icon-size: 36px"));
    assert.throws(() => assert.doesNotMatch(read("gameplay.css"), /--rune-icon-size:\s*(3[3-9]|[4-9]\d)px/));

    fs.writeFileSync(path.join(webCopy, "gameplay.css"), originals["gameplay.css"].replace(".config-option.is-route .config-icons { flex-wrap: nowrap;", ".config-option.is-route .config-icons { flex-wrap: wrap;"));
    assert.throws(() => assert.match(read("gameplay.css"), /\.config-option\.is-route \.config-icons\s*\{[^}]*flex-wrap:\s*nowrap/s));

    fs.writeFileSync(path.join(webCopy, "champions.js"), originals["champions.js"].replace('renderConfigOption(row, "item", null, renderDepthStats)', 'renderConfigOption(row, "item")'));
    assert.throws(() => assert.match(read("champions.js"), /renderConfigOption\(row, "item", null, renderDepthStats\)/));

    fs.writeFileSync(path.join(webCopy, "gameplay.js"), originals["gameplay.js"].replace('optionList(options, "item", empty, renderDepthStats)', 'optionList(options, "item", empty)'));
    assert.throws(() => assert.match(read("gameplay.js"), /optionList\(options, "item", empty, renderDepthStats\)/));

    fs.writeFileSync(path.join(webCopy, "gameplay.js"), originals["gameplay.js"].replace("return renderOptionStats(stats);", 'return `<dl class="recommendation-stats"></dl>`;'));
    assert.throws(() => assert.match(functionSource(read("gameplay.js"), "renderRecommendationStats"), /return renderOptionStats\(stats\);/));

    for (const slot of ["fourth", "fifth", "sixth"]) {
      const mutated = originals["gameplay.js"].replace(`inUpstreamOrder(build.${slot}Options || [], 5)`, `inUpstreamOrder(build.${slot}Options || [], 3)`);
      fs.writeFileSync(path.join(webCopy, "gameplay.js"), mutated);
      assert.throws(() => assert.match(read("gameplay.js"), new RegExp(`inUpstreamOrder\\(build\\.${slot}Options \\|\\| \\[\\], 5\\)`)));
    }

    fs.writeFileSync(path.join(webCopy, "build-item-row.css"), originals["build-item-row.css"].replace("--build-row-height: 64px", "--build-row-height: 40px"));
    assert.throws(() => assert.match(read("build-item-row.css"), /--build-row-height:\s*64px/));

		fs.writeFileSync(path.join(webCopy, "gameplay.js"), originals["gameplay.js"].replace("const timeout = riotTab(tab) ? 190_000 : 25_000;", "const timeout = 10_000;"));
		assert.throws(() => assert.match(read("gameplay.js"), /const timeout = riotTab\(tab\) \? 190_000 : 25_000;/));

		const compileMilestones = (source) => compileFunctions(source, ["renderRankMilestones"], {
			escapeHTML: (value) => String(value),
			rankCrestIcon: (tier) => tier,
			rankTitle: (rank) => rank.tier,
		});
		fs.writeFileSync(path.join(webCopy, "gameplay.js"), originals["gameplay.js"].replace('if (!items.length) return "";', 'if (false) return "";'));
		assert.throws(() => assert.equal(compileMilestones(read("gameplay.js")).renderRankMilestones({}), ""));

		fs.writeFileSync(path.join(webCopy, "gameplay.js"), originals["gameplay.js"].replace(
			"return renderHistoricalRanks(historicalRanks) || renderRankMilestones(rankMilestones);",
			"return renderRankMilestones(rankMilestones) || renderHistoricalRanks(historicalRanks);",
		));
		const mutatedHistory = compileFunctions(read("gameplay.js"), ["renderRankHistory"], {
			renderHistoricalRanks: () => "KR-HISTORY",
			renderRankMilestones: () => "CN-MILESTONES",
		});
		assert.throws(() => assert.equal(mutatedHistory.renderRankHistory([{}], {}), "KR-HISTORY"));
  } finally {
    fs.rmSync(temp, { recursive: true, force: true });
  }
});

test("R45 contracts cover desktop Arena geometry, build layout, and cell identity wiring", () => {
  assertR45Contracts();
});

test("R45 mutation probes reject every documented A, B, and C regression", () => {
  const probes = [
    ["A cell fallback", () => assertR45Contracts(gameplayScript, gameplayStyles, gameplayBackend.replace(
      "!gameplayLivePlayerIsCurrent(playerReference, current.PUUID, player.CellID, session.LocalPlayerCellID)",
      "false",
    ))],
    ["B desktop Arena rows", () => assertR45Contracts(gameplayScript, gameplayStyles.replace(
      ".match-stats.is-arena { align-content: start; grid-template-rows: repeat(3,18px); min-height: 54px; }",
      "",
    ))],
    ["C1 title spacing", () => assertR45Contracts(gameplayScript, gameplayStyles.replace(
      ".live-augment-recommendations + .build-recommendation { position: relative; margin-top: 20px; }",
      "",
    ))],
    ["C1 divider", () => assertR45Contracts(gameplayScript, gameplayStyles.replace(
      '.live-augment-recommendations + .build-recommendation::before { position: absolute; top: -11px; right: 0; left: 0; height: 1px; background: var(--line); content: ""; }',
      "",
    ))],
    ["C2 skill row", () => assertR45Contracts(gameplayScript, gameplayStyles.replace(
      ".build-summary-bar > .build-summary-skill { grid-column: 1 / -1;",
      ".build-summary-bar > .build-summary-skill { grid-column: auto;",
    ))],
    ["C3 prism order", () => assertR45Contracts(gameplayScript.replace(
      '${lateBands}<div class="build-core-ranking-row${rankingLayoutClass}">${itemBuildLayout}',
      '<div class="build-core-ranking-row${rankingLayoutClass}">${itemBuildLayout}${lateBands}',
    ))],
    ["C4 Arena card style", () => assertR45Contracts(gameplayScript, gameplayStyles.replace(
      ".build-recommendation.is-arena-build .item-core-column .config-option { padding: 7px 9px; background: var(--bg); border: 1px solid var(--line); }",
      ".build-recommendation.is-arena-build .item-core-column .config-option { }",
    ))],
    ["C5 local cell assignment", () => assertR45Contracts(gameplayScript, gameplayStyles, gameplayBackend.replace(
      "localPlayerCellID = champSelect.LocalPlayerCellID",
      "localPlayerCellID = nil",
    ))],
    ["C5 local cell argument", () => assertR45Contracts(gameplayScript, gameplayStyles, gameplayBackend.replace(
      "gameplayLivePlayerIsCurrent(reference, current.PUUID, raw.player.CellID, localPlayerCellID)",
      "gameplayLivePlayerIsCurrent(reference, current.PUUID, raw.player.CellID, nil)",
    ))],
  ];
	  for (const [label, probe] of probes) assert.throws(probe, label);
});

test("R46 contracts cover cache eviction, parsing, beacon, refresh, and TFT handling", () => {
	assertR46Contracts();
});

test("R46 mutation probes reject all six required regressions", () => {
	const globalMetricIndex = hexdataBackend.indexOf("hexdataGlobalMetric");
	assert.notEqual(globalMetricIndex, -1);
	const globalMetricTail = hexdataBackend.slice(globalMetricIndex).replace("[·，,]", "·");
	const hexdataWithoutGlobalSeparatorTolerance = hexdataBackend.slice(0, globalMetricIndex) + globalMetricTail;
	const probes = [
		["B-1 overview cache eviction", () => assertR46Contracts({
			cacheSource: overviewCacheBackend.replace(
				"for len(cache.entries) > overviewQueryCacheMax {",
				"if len(cache.entries) > overviewQueryCacheMax {",
			),
		})],
		["C-1 separator tolerance", () => assertR46Contracts({ hexSource: hexdataWithoutGlobalSeparatorTolerance })],
		["C-3 explicit empty augment array", () => assertR46Contracts({
			goSource: gameplayBackend.replace('`json:"augments"`', '`json:"augments,omitempty"`'),
		})],
		["F collapsed live beacon selector", () => assertR46Contracts({
			appCSS: appStyles.replace(
				':root[data-sidebar="collapsed"] .section-tab > span:not(.live-beacon),',
				':root[data-sidebar="collapsed"] .section-tab span,',
			),
		})],
		["G-2 phase refresh delay", () => assertR46Contracts({
			js: gameplayScript.replace(
				'if (normalized === "ChampSelect") return 3_000;',
				'if (normalized === "ChampSelect") return 20_000;',
			),
		})],
		["H semantic TFT detection", () => assertR46Contracts({
			goSource: gameplayBackend.replace(
				'strings.EqualFold(strings.TrimSpace(gameMode), "TFT")',
				'strings.EqualFold(strings.TrimSpace(gameMode), "CLASSIC")',
			),
		})],
	];
	for (const [label, probe] of probes) assert.throws(probe, label);
});

test("R47 contracts cover Arena champ-select filtering and the single-roster layout", () => {
  assertR47Contracts();
});

test("R47 A-group mutation probes reject mode and single-roster regressions", () => {
  const probes = [
    ["A-1 CHERRY-only filter", () => assertR47Contracts({
      goSource: gameplayBackend.replace(
        'strings.EqualFold(strings.TrimSpace(gameMode), "CHERRY")',
        'strings.EqualFold(strings.TrimSpace(gameMode), "CLASSIC")',
      ),
    })],
    ["A-2 Arena branch", () => assertR47Contracts({
      js: gameplayScript.replace("if (arenaMode) {", "if (false) {"),
    })],
    ["A-2 Arena single-roster fallback columns", () => assertR47Contracts({
      css: gameplayStyles.replace(".live-teams.is-arena { grid-template-columns: 1fr; }", ".live-teams.is-arena { grid-template-columns: repeat(auto-fit,minmax(320px,1fr)); }"),
    })],
  ];
  for (const [label, probe] of probes) assert.throws(probe, label);
});

test("R49 all seven required mutation probes fail on true copied files", () => {
	const temp = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-r49-guards-"));
	const webCopy = path.join(temp, "web");
	fs.mkdirSync(webCopy);
	const copies = {
		js: path.join(webCopy, "gameplay.js"),
		goSource: path.join(temp, "gameplay.go"),
		specialistGo: path.join(temp, "specialist_runes.go"),
	};
	fs.copyFileSync(path.join(__dirname, "gameplay.js"), copies.js);
	fs.copyFileSync(path.join(root, "gameplay.go"), copies.goSource);
	fs.copyFileSync(path.join(root, "specialist_runes.go"), copies.specialistGo);
	try {
		for (const filename of Object.values(copies)) assert.equal(fs.lstatSync(filename).isSymbolicLink(), false);
		const originals = Object.fromEntries(Object.entries(copies).map(([key, filename]) => [key, fs.readFileSync(filename, "utf8")]));
		const readCopies = () => Object.fromEntries(Object.entries(copies).map(([key, filename]) => [key, fs.readFileSync(filename, "utf8")]));
		assertR49RequiredMutationContracts(readCopies());

		const probes = [
			["A-3 remove per-step timeout", "specialistGo", "context.WithTimeout(ctx, timeout)", "context.WithCancel(ctx)"],
			["A-4 classify timeout as no-position", "specialistGo", "return specialistOutcomeTimeout", "return specialistOutcomeNoPositionSample"],
			["B-2 remove cross-group deduplication", "js", "const depth = mergedItems(depthOptions).filter(({ id }) => !coreIDs.has(id));", "const depth = mergedItems(depthOptions);"],
			["B-3 remove ascending price order", "goSource", "return knownItems[left].PriceTotal < knownItems[right].PriceTotal", "return false"],
			["C-1 relabel position with recommendation result", "js", "const resolved = livePositionDisplay(target?.clientPosition || self?.position);", "const resolved = livePositionDisplay(payload.resolvedPosition || target?.position || self?.position);"],
			["C-2 remove role-share ordering", "js", ".sort((left, right) => right.displayRoleRate - left.displayRoleRate || (Number(right.play) || 0) - (Number(left.play) || 0));", ".sort((left, right) => (Number(right.play) || 0) - (Number(left.play) || 0));"],
			["E-1 remove current champion fallback", "js", "return Number(player?.championId) || Number(player?.championPickIntent) || (player?.isCurrent === true ? Number(currentChampionId) || 0 : 0);", "return Number(player?.championId) || Number(player?.championPickIntent) || 0;"],
		];
		for (const [label, key, before, after] of probes) {
			assert.notEqual(originals[key].indexOf(before), -1, `${label}: mutation target not found`);
			const mutated = originals[key].replace(before, after);
			assert.notEqual(mutated, originals[key], `${label}: source was not mutated`);
			fs.writeFileSync(copies[key], mutated);
			assert.throws(() => assertR49RequiredMutationContracts(readCopies()), label);
			fs.writeFileSync(copies[key], originals[key]);
		}
	} finally {
		fs.rmSync(temp, { recursive: true, force: true });
	}
});

test("R47 addendum scopes recommendation breakpoints to its inline-size container", () => {
  assertR47AddendumContracts();
});

test("R47 addendum mutation probes reject media-query regressions", () => {
  const wideMedia = gameplayStyles.replace(
    "@container recommendation-area (max-width: 1080px)",
    "@media (max-width: 1080px)",
  );
  const classicNarrowMedia = gameplayStyles.replace(
    "@container recommendation-area (max-width: 840px)",
    "@media (max-width: 840px)",
  );
  const narrowMedia = gameplayStyles.replace(
    "@container recommendation-area (max-width: 700px)",
    "@media (max-width: 700px)",
  );
  assert.throws(() => assertR47AddendumContracts(wideMedia), "1080px breakpoint must remain a recommendation container query");
  assert.throws(() => assertR47AddendumContracts(classicNarrowMedia), "840px breakpoint must remain a recommendation container query");
  assert.throws(() => assertR47AddendumContracts(narrowMedia), "700px breakpoint must remain a recommendation container query");
});

test("R45 item-set branding uses DL in demo and LCU metadata", () => {
  assert.match(demoScript, /title: `DL · \$\{request\.title \|\| "推荐出装"\}`/);
  assert.doesNotMatch(demoScript, /title: `Deep Legends ·/);
  assert.match(goFunctionSource(gameplayBackend, "newLCUItemSet"), /StartedFrom:\s*"DL"/);
});

test("R71 item-set action uses game recommendations without verbose manual-cleanup notes", () => {
  const build = functionSource(gameplayScript, "renderBuildRecommendation");
  assert.doesNotMatch(build, /item-set-manual-note|若客户端配装列表出现其他工具/);
  assert.match(build, /游戏装备方案/);
  assert.match(functionSource(gameplayScript, "applyItemSet"), /payload\.storage\s*=\s*"recommended"/);
  // Preserve the legacy API contract; the new game-file schema has its own test.
  assert.match(goFunctionSource(gameplayBackend, "newLCUItemSet"), /SortRank:\s*100/);
});

test("R58 mutation probes reject stale-roster and Arena fallback regressions", () => {
	const assertStaleGuard = (source) => {
		const handler = goFunctionSource(source, "loadGameplayLive");
		assert.match(handler, /if phase == "ChampSelect" \{\s*\/\/ gameData remains populated with the previous match[\s\S]*response\.DroppedStaleGameData = sessionErr == nil\s*\} else \{\s*response\.GameID = session\.GameData\.GameID/);
	};
	assertStaleGuard(gameplayBackend);
	const mutatedBackend = gameplayBackend.replace(
		'if phase == "ChampSelect" {\n\t\t// gameData remains populated with the previous match',
		'if false {\n\t\t// gameData remains populated with the previous match',
	);
	assert.notEqual(mutatedBackend, gameplayBackend, "A mutation target must exist");
	assert.throws(() => assertStaleGuard(mutatedBackend), "deleting the ChampSelect guard must fail the contract");

	const fallbackGuard = 'if (grouped.size < 2 || [...grouped.values()].some((group) => group.length > 3)) return [];';
	assert.match(functionSource(gameplayScript, "arenaLivePlayerGroups"), /grouped\.size < 2[\s\S]*group\.length > 3/);
	const mutatedScript = gameplayScript.replace(fallbackGuard, "");
	assert.notEqual(mutatedScript, gameplayScript, "B fallback mutation target must exist");
	const { arenaLivePlayerGroups: mutatedGroups } = compileFunctions(mutatedScript, ["arenaLivePlayerGroups"]);
	const malformed = Array.from({ length: 18 }, (_, index) => ({ arenaGroup: String(Math.min(5, Math.floor(index / 3) + 1)) }));
	assert.throws(
		() => assert.equal(mutatedGroups({ phase: "InProgress", arenaGrouped: true }, malformed).length, 0),
		"deleting the invalid-shape fallback must fail the acceptance assertion",
	);
});

// 「过去 30 天排位」整块已删除：它与「近 20 场排位」共用同一批 20 场详情战绩，
// 只是把落在 30 天内的那部分再算一次胜率/KDA，窗口还不精确（会标「以上」）。
test("the duplicated 30-day ranked card is gone from every layer", () => {
	for (const source of [gameplayScript, gameplayStyles, demoScript]) {
		assert.doesNotMatch(source, /sevenDayRank|renderSevenDay|seven-day-content/);
	}
	assert.doesNotMatch(gameplayScript, /过去 30 天排位/);
	assert.doesNotMatch(gameplayBackend, /SevenDayRank/);
	assert.doesNotMatch(riotBackend, /SevenDayRank/);
});

// 三张排位卡片的页签左边原本还印着一行「灵活组排 / 单双排」文字，
// 与右边高亮的页签重复。页签本身已经把当前队列表达清楚了。
test("ranked queue switchers drop the redundant text label", () => {
	const toolbars = gameplayScript.match(/<div class="career-section-tools">[\s\S]*?<\/div>/g) || [];
	assert.equal(toolbars.length, 6, "近 20 场排位、能力表现和位置偏好各只有数据态与空态");
	for (const toolbar of toolbars) {
		assert.equal(toolbar, '<div class="career-section-tools">${tools}</div>');
	}
	// 页签本体必须还在，否则就不是「去掉提示」而是把切换功能一起删了。
	assert.match(gameplayScript, /class="ranked-queue-button[\s\S]*?data-ranked-queue="440"[\s\S]*?灵活组排/);
	assert.doesNotMatch(gameplayScript, /function renderSeasonProgressBadge|season-progress-badge/);
	const narrowCareer = cssBlockAfter(gameplayStyles, "@container career-dialog (max-width: 360px)");
	assert.match(narrowCareer, /\.career-section > header\s*\{[^}]*flex-wrap:\s*wrap/s);
	assert.match(narrowCareer, /\.career-section-tools\s*\{[^}]*width:\s*100%[^}]*flex-wrap:\s*wrap/s);
});

test("specialist rune client skips are observable and stale cooldowns recover", async () => {
		const ensureSource = functionSource(gameplayScript, "ensureSpecialistRunes");
		assert.match(ensureSource, /live-specialist-runes:\$\{target\.key\}`[,\s]+30000\)/);
	for (const reason of ["no-target", "no-top-players", "embedded", "cached", "cached-empty", "in-flight", "riot-key-missing-cooldown", "recent-failure-cooldown"]) {
		assert.match(ensureSource, new RegExp(`"${reason}"`));
	}
	const diagnosticSource = functionSource(gameplayScript, "recordSpecialistRuneClientSkip");
	assert.match(diagnosticSource, /\/api\/diagnostics\/client/);
	assert.match(diagnosticSource, /method:\s*"POST"/);

	const target = { key: "13:middle", championId: 13, position: "middle" };
	let now = 1_000;
	const state = {
		live: {}, specialistRunes: new Map(), specialistRuneFlights: new Map(),
		specialistRuneFailures: new Map(),
	};
	const skipped = [];
	let apiCalls = 0;
	const functions = compileFunctions(gameplayScript, ["specialistRuneFailure", "specialistRuneFlightActive", "ensureSpecialistRunes"], {
		state,
		Date: { now: () => now },
		liveRecommendationTarget: () => target,
		recommendationQueueHasTopPlayers: () => true,
		liveRecommendationsFor: () => ({ runes: { specialists: [] } }),
		recordSpecialistRuneClientSkip: (reason) => skipped.push(reason),
		renderLive: () => {},
		api: async () => {
		  apiCalls += 1;
		  return apiCalls === 1 ? { reason: "riot-key-missing", runes: [] } : { runes: [{ key: "verified" }] };
		},
		ensurePerks: () => {},
		URLSearchParams,
	});
	await functions.ensureSpecialistRunes({ queueId: 420 });
	assert.equal(apiCalls, 1);
	assert.equal(state.specialistRunes.has(target.key), false, "Riot Key 缺失不能写入空缓存");

	now = 50_000;
	await functions.ensureSpecialistRunes({ queueId: 420 });
	assert.equal(apiCalls, 1);
	assert.deepEqual(skipped, ["riot-key-missing-cooldown"]);

	now = 62_000;
	await functions.ensureSpecialistRunes({ queueId: 420 });
	assert.equal(apiCalls, 2, "Riot Key 缺失冷却满 60 秒后必须重试");
	assert.equal(state.specialistRunes.get(target.key)?.[0]?.key, "verified");

	state.specialistRuneFlights.set(target.key, 1_000);
	assert.equal(functions.specialistRuneFlightActive(target.key, 62_000), false);
	assert.equal(state.specialistRuneFlights.has(target.key), false, "陈旧 flight 必须清除");
	state.specialistRuneFlights.set(target.key, 61_500);
	assert.equal(functions.specialistRuneFlightActive(target.key, 62_000), true);
});

test("specialist upstream failures remain retryable and are never cached as empty samples", async () => {
	const target = { key: "64:mid", championId: 64, position: "mid" };
	let now = 1_000;
	let apiCalls = 0;
	const state = {
		live: {}, specialistRunes: new Map(), specialistRuneFlights: new Map(),
		specialistRuneFailures: new Map(),
	};
	const skipped = [];
	const functions = compileFunctions(gameplayScript, ["specialistRuneFailure", "specialistRuneFlightActive", "ensureSpecialistRunes"], {
		state,
		Date: { now: () => now },
		liveRecommendationTarget: () => target,
		recommendationQueueHasTopPlayers: () => true,
		liveRecommendationsFor: () => ({ runes: { specialists: [] } }),
		recordSpecialistRuneClientSkip: (reason) => skipped.push(reason),
		renderLive: () => {},
		api: async () => {
			apiCalls += 1;
			return apiCalls === 1 ? { reason: "upstream-timeout", runes: [] } : { reason: "no-position-sample", runes: [] };
		},
		ensurePerks: () => {},
		URLSearchParams,
	});
	await functions.ensureSpecialistRunes({ queueId: 420 });
	assert.equal(state.specialistRunes.has(target.key), false, "超时不能写入空样本缓存");
	assert.equal(state.specialistRuneFailures.get(target.key)?.reason, "upstream-timeout");

	now = 30_000;
	await functions.ensureSpecialistRunes({ queueId: 420 });
	assert.equal(apiCalls, 1);
	assert.deepEqual(skipped, ["recent-failure-cooldown"]);

	now = 62_000;
	await functions.ensureSpecialistRunes({ queueId: 420 });
	assert.equal(apiCalls, 2);
	assert.deepEqual(state.specialistRunes.get(target.key), [], "仅真实无位置样本可以缓存空数组");
});

test("specialist retryable outcomes have accurate copy and a retry control", () => {
	for (const [reason, title, message] of [
		["upstream-timeout", "韩服接口响应超时", "请稍后重试"],
		["upstream-throttled", "请求过于频繁", "约 1 分钟后可重试"],
		["upstream-error", "上游数据异常", "请稍后重试"],
	]) {
		const state = { live: {}, specialistRuneFailures: new Map([["target", { reason, at: 1 }]]), specialistRunes: new Map() };
		const functions = compileFunctions(gameplayScript, ["specialistRuneFailure", "renderRuneSourceSection"], {
			state,
			liveRecommendationTarget: () => ({ key: "target", position: "mid" }),
			escapeHTML: (value) => String(value ?? ""),
			positionLabel: (value) => value,
			livePositionDisplay: (value) => value,
			renderSpecialistPlayers: () => "",
			renderRuneChoice: () => "",
		});
		const rendered = functions.renderRuneSourceSection({ key: "specialist", items: [], failed: true }, true);
		assert.match(rendered, new RegExp(title));
		assert.match(rendered, new RegExp(message));
		assert.match(rendered, /data-retry-specialist-runes/);
	}
});

test("queues without top-player data keep an explicit specialist empty state", () => {
	const state = { live: {}, specialistRuneFailures: new Map(), specialistRuneFlights: new Map(), specialistRunes: new Map(), runeSourceTab: "specialist" };
	const functions = compileFunctions(gameplayScript, ["specialistRuneFailure", "renderRuneRecommendations", "renderRuneSourceSection"], {
		state,
		escapeHTML: (value) => String(value ?? ""),
		liveRecommendationTarget: () => ({ key: "target" }),
		liveRecommendationsFor: () => ({ runes: { opgg: [] } }),
		recommendationQueueHasTopPlayers: () => false,
		specialistRunesFor: () => [],
		renderSpecialistPlayers: () => "",
		renderRuneChoice: () => "",
	});
	const rendered = functions.renderRuneRecommendations({ queueId: 1700 }, true);
	assert.match(rendered, /data-rune-source="specialist"/);
	assert.match(rendered, /当前队列没有可用的绝活哥榜单/);
	assert.match(rendered, /仅支持单双排、灵活组排和召唤师峡谷自定义对局/);
});

test("collection status wording is timeless and promotion chests use dedicated artwork", () => {
	assert.doesNotMatch(appScript, /收藏与奖池核对结果有效|收藏与奖池已更新/);
	assert.match(functionSource(appScript, "renderNotice"), /el\.notice\.hidden = true;\s*\}/);
	assert.match(cssBlockAfter(appStyles, ".select-menu-popover button"), /border:\s*0/);
	assert.doesNotMatch(html, /<p class="eyebrow">客户端仓库<\/p>|<p>按类别展示客户端物品。<\/p>|<p class="eyebrow">三合一奖池<\/p>/);
	assert.match(lcuAPIBackend, /"CHEST_PROMOTION":\s+"紫色宝箱"/);
	assert.match(appScript, /CHEST_PROMOTION:\s*"\/loot-icons\/promotion-chest\.png"/);
	assert.match(lcuAPIBackend, /"CHEST_GENERIC":\s+"海克斯科技宝箱"/);
	assert.match(appScript, /CHEST_GENERIC:\s*"\/loot-icons\/promotion-chest\.png"/);
	assert.match(demoScript, /lootId:\s*"CHEST_CHAMPION_MASTERY",\s*displayName:\s*"战利品宝箱"/);
	assert.match(demoScript, /lootId:\s*"CHEST_PROMOTION",\s*displayName:\s*"紫色宝箱"/);
	assert.ok(fs.existsSync(path.join(__dirname, "loot-icons", "promotion-chest.png")), "紫色宝箱专属素材缺失");
});

// 离开英雄页再回来时滚动位置要还原：一次性 scrollTo 会被随后渲染的加载态
// （内容比原来矮得多）夹回 0，必须持续重试到真正到位，并让用户随时打断。
test("returning to a section restores its scroll position past the loading skeleton", () => {
	const restore = functionSource(appScript, "restoreSectionScroll");
	assert.match(restore, /const target = Number\(state\.sectionScroll\[name\] \|\| 0\)/);
	assert.match(restore, /requestAnimationFrame\(apply\)/);
	assert.match(restore, /Date\.now\(\) > deadline/);
	for (const event of ["wheel", "touchstart", "keydown", "pointerdown"]) {
		assert.ok(restore.includes(`"${event}"`), `${event} 必须能打断还原，不能和用户抢滚动条`);
	}
	assert.match(appScript, /if \(previousSection !== name\) restoreSectionScroll\(name\)/);
	assert.match(appScript, /state\.sectionScroll\[previousSection\] = el\.appScroll\.scrollTop/);

	// 英雄表自己还有一层内滚动：还原完成前不能把加载态的 0 采样回去。
	const championRender = functionSource(script, "render");
	assert.match(championRender, /if \(list && !state\.listScrollRestorePending\) state\.listScrollInner = list\.scrollTop/);
	assert.match(championRender, /state\.listScrollRestorePending = false/);
	assert.match(script, /state\.listScrollRestorePending = \(state\.listScrollInner \|\| 0\) > 0/);
});

test("captured Arena equipment matches YOUR.GG preview order and expands to every source item", () => {
  const capture = JSON.parse(fs.readFileSync(path.join(root, "testdata/arena-items/yourgg-122-20260909.json"), "utf8")).response;
  const catalog = new Map(JSON.parse(fs.readFileSync(path.join(root, "testdata/arena-items/catalog-20260909.json"), "utf8")).map(item => [item.id, item]));
  const state = { arenaExpanded: {prism:false,core:false} };
  const functions = compileFunctions(script, ["objectRows", "sortedArenaItemRows", "renderArenaItemSection", "renderArenaCoreSection", "renderArenaExpand", "renderArenaOptionCard", "augmentGrade", "percent"], {
    state, escapeHTML: x => String(x ?? ""), number: x => String(x ?? "—"), compactNumber: x => String(x ?? "—"),
    renderAssetButton: asset => `<span data-item-id="${asset.id}">${asset.name}</span>`,
    sortedArenaRows: rows => rows, renderArenaSlimRow: () => "",
  });
  const toRows = values => values.map(value => ({
    assets:[{id:value.itemId,kind:"item",name:catalog.get(value.itemId)?.name || "未知装备"}],
    tier:value.tier, score:value.score, games:value.matches, winRate:value.winRate*100,
    firstPlaceRate:value.firstPlacementRate*100, averagePlacement:value.averagePlacement,
  }));
  const ids = document => [...document.querySelectorAll("[data-item-id]")].map(el => Number(el.dataset.itemId));
  for (const [kind, values, expectedPreview] of [
    ["prism", capture.prismaticItems, [443056,447106,443193,446632,447112,447107]],
    ["core", capture.coreItems, [226695,224401,223075,223026,223143,226696]],
  ]) {
    const rows = toRows(values);
    const render = () => kind === "prism" ? functions.renderArenaItemSection("棱彩装备", "单件", rows, kind) : functions.renderArenaCoreSection({coreItems:rows});
    let dom = new JSDOM(render());
    assert.deepEqual(ids(dom.window.document), expectedPreview, `${kind}: same-tier order must follow matches, not score`);
    assert.equal(dom.window.document.querySelectorAll(".arena-option-card").length, 6);
    assert.equal(dom.window.document.querySelector("[data-arena-expand]").textContent, `展开全部 ${values.length} 条`);
    dom.window.close();
    state.arenaExpanded[kind] = true;
    dom = new JSDOM(render());
    assert.deepEqual(ids(dom.window.document).sort((a,b)=>a-b), values.map(row=>row.itemId).sort((a,b)=>a-b), `${kind}: expansion must retain every ID`);
    assert.equal(dom.window.document.querySelectorAll(".arena-option-card").length, values.length);
    assert.equal(dom.window.document.querySelector("[data-arena-expand]").textContent, "收起");
    assert.doesNotMatch(dom.window.document.body.textContent, /三件套出装路线/);
    dom.window.close();
  }
});

test("Arena equipment sorting retains low samples, missing metadata and ungraded items", () => {
  const {sortedArenaItemRows} = compileFunctions(script,["objectRows","sortedArenaItemRows"]);
  const rows = [{tier:"A",score:99,games:1,id:1},{tier:"A",score:10,games:100,id:2},{tier:"S",games:0,id:3},{games:50,id:4}];
  assert.deepEqual(sortedArenaItemRows(rows).map(row=>row.id), [3,2,1,4]);
  assert.deepEqual(rows.map(row=>row.id),[1,2,3,4],"do not mutate source array");
});

test("R86 champion index preserves numeric/first-match/key semantics and invalidates catalogs", () => {
  const first = { id: 1, slug: "Ahri", key: "狐", nameZh: "阿狸" };
  const zero = { id: 0, key: "ZERO" };
  const state = { catalog: { champions: [null, false, 7, first, { id: "01", slug: "other", key: "Ahri" }, zero, { id: "NaN", key: "É" }] } };
  let arrays = 0;
  const { championMeta, championMetaByKey } = compileFunctions(script, ["championIndex", "championMeta", "championMetaByKey"], {
    state, objectRows: (value) => { arrays++; return Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []; },
  });
  for (const id of [1, "01", " 1 ", "1.0"]) assert.equal(championMeta(id), first);
  assert.equal(championMeta(null), zero);
  assert.equal(championMeta(""), zero);
  assert.equal(championMeta(NaN), null);
  assert.equal(championMeta(999), null);
  assert.equal(championMetaByKey("AHRI"), first);
  assert.equal(championMetaByKey("狐"), first);
  assert.equal(championMetaByKey(" Ahri "), null);
  assert.equal(championMetaByKey("ＡＨＲＩ"), null);
  assert.equal(championMetaByKey("é").key, "É");
  assert.equal(arrays, 1);
  const replacement = { id: 1, key: "NEW" };
  state.catalog = { champions: [replacement] };
  assert.equal(championMeta(1), replacement);
  assert.equal(championMetaByKey("Ahri"), null);
  assert.equal(arrays, 2);
  state.catalog = null;
  assert.equal(championMeta(1), null);
});
