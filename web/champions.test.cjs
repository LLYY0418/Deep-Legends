const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const appScript = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
const appStyles = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");
const script = fs.readFileSync(path.join(__dirname, "champions.js"), "utf8");
const styles = fs.readFileSync(path.join(__dirname, "champions.css"), "utf8");
const gameplayScript = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const gameplayStyles = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");
const backend = fs.readFileSync(path.join(root, "champions.go"), "utf8");
const mainSource = fs.readFileSync(path.join(root, "main.go"), "utf8");
const specialistSource = fs.readFileSync(path.join(root, "specialist_runes.go"), "utf8");
const positionIcons = ["all", "top", "jungle", "middle", "bottom", "utility"].map((name) => fs.readFileSync(path.join(__dirname, "position-icons", `${name}.svg`), "utf8"));
const allPositionIcon = positionIcons[0];

test("champion intelligence is a first-level accessible workspace", () => {
  assert.match(html, /id="section-champions"[^>]+role="tab"[^>]+aria-controls="champions-panel"/);
  assert.match(html, /id="champions-panel"[^>]+role="tabpanel"[^>]+aria-labelledby="section-champions"/);
  assert.match(html, /option value="champions">英雄<\/option>/);
  assert.match(html, /id="setting-champion-position"/);
});

test("renderer only calls the authenticated local champion API", () => {
  for (const endpoint of ["/api/champions/catalog", "/api/champions/rankings", "/api/champions/augments", "/api/champions/detail", "/api/champion-asset"]) {
    assert.ok(script.includes(endpoint), `missing local endpoint ${endpoint}`);
  }
  assert.doesNotMatch(script, /fetch\(["'`]https?:\/\//);
  assert.match(script, /loading="lazy"/);
});

test("backend pins Korean data and restricts remote image paths", () => {
  assert.match(backend, /\/api\/KR\/champions\/ranked/);
  assert.match(backend, /"region": \{"kr"\}/);
  assert.match(backend, /strings\.HasPrefix\(requestPath, "\/meta\/images\/"\)/);
  assert.match(backend, /strings\.HasPrefix\(requestPath, "\/cdn\/"\)/);
  assert.match(backend, /Value: "challenger", Label: "最强王者"/);
});

test("champion layouts keep responsive and reduced-motion fallbacks", () => {
  for (const breakpoint of ["1260px", "1080px", "900px", "700px", "480px"]) assert.ok(styles.includes(`max-width: ${breakpoint}`));
  assert.match(styles, /prefers-reduced-motion:\s*reduce/);
  assert.match(styles, /\.augment-tier-groups\s*\{[^}]*display:\s*grid/s);
  assert.match(styles, /\.champion-rune-board\s*\{[^}]*grid-template-columns:/s);
  assert.match(styles, /\.augment-podium\s*\{[^}]*grid-template-columns:/s);
  assert.match(styles, /\.champion-table-scroll\s*\{[^}]*overflow-x:\s*auto/s);
  assert.match(styles, /\.aram-workspace\s*\{[^}]*grid-template-columns:\s*minmax\(270px,320px\)\s+minmax\(0,1fr\)/s);
  assert.match(styles, /\.champion-filter-bar\s*\{[^}]*grid-template-columns:/s);
  assert.match(styles, /\.rune-board-panel\s*\{[^}]*container-type:\s*inline-size/s);
  assert.match(styles, /\.champion-rune-board\s*\{[^}]*clamp\([^}]*cqi/s);
  assert.doesNotMatch(styles, /min-width:\s*620px/);
  assert.doesNotMatch(styles, /\.rune-board-panel\s*\{[^}]*overflow-x:\s*auto/s);
  assert.doesNotMatch(styles, /\.champion-build-board \.config-icons\s*\{[^}]*overflow-x:\s*auto/s);
  assert.match(gameplayStyles, /\.skill-order\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fit/s);
});

test("search keeps its caret and IME state when the workspace rerenders", () => {
  assert.match(script, /setSelectionRange/);
  assert.match(script, /compositionstart/);
  assert.match(script, /compositionend/);
  assert.match(styles, /\.champion-search input\s*\{[^}]*direction:\s*ltr/s);
});

test("champion detail uses complete build, matchup, and augment ranking layouts", () => {
  assert.match(script, /class="champion-rune-board"/);
  assert.match(script, /class="augment-podium"/);
  assert.match(script, /data-counter-champion/);
  assert.match(script, /routeLimit[^\n]+=== "adc" \? 7 : 6/);
  assert.match(script, /data-tooltip=/);
  assert.match(script, /heroArtworkURL\(meta, source, path\)/);
  assert.match(script, /data-rune-page="\$\{index\}"/);
  assert.match(script, /tier-icons\/\$\{key\}\.svg/);
  assert.match(script, /class="champion-top3"/);
  assert.match(script, /class="metric-games"/);
  assert.match(script, /runeStyleIcon\(page\.primaryStyle, "rune-tab-style"\)/);
  assert.match(script, /runeStyleIcon\(page\.subStyle, "rune-tab-substyle"\)/);
  assert.match(script, /class="spell-option-icons"/);
  assert.match(script, /class="spell-option-stats"/);
  assert.match(script, /class="build-split"/);
  assert.match(script, /class="build-side-row"/);
  assert.match(script, /class="route-row"/);
  assert.match(script, /class="counter-meter"/);
  assert.match(script, /class="counter-row"/);
  assert.match(script, /renderTopPlayers\(ranked\)/);
  assert.match(script, /rank-crests\/\$\{String\(player\.tier\)/);
  assert.match(script, /class="route-step"/);
  assert.match(gameplayScript, /class="route-step"/);
  assert.doesNotMatch(script, /rune-page-heading|配置\$\{/);
  assert.doesNotMatch(script, /for \(const asset of \(boots/);
  assert.match(script, /\[\["all", "全部"\], \["prismatic", "棱彩"\], \["gold", "黄金"\], \["silver", "白银"\]\]/);
});

test("arena uses live three-person synergy data and shared detail components", () => {
  assert.match(script, /function renderArena\(\)/);
  assert.match(script, /class="aram-workspace arena-workspace"/);
  assert.match(script, /class="arena-team-podium"/);
  assert.match(script, /teamCompositions/);
  assert.match(script, /detail\.arenaAugments/);
  assert.match(script, /state\.mode !== "arena"\) itemSections\.push/);
  assert.doesNotMatch(script, /斗魂竞技场将在下一阶段接入|renderArenaPlaceholder/);
  assert.match(styles, /\.arena-team-podium\s*\{[^}]*grid-template-columns:\s*repeat\(3/s);
  assert.match(styles, /\.arena-detail-team-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3/s);
  assert.match(backend, /\/zh-cn\/lol\/modes\/arena/);
  assert.match(backend, /`"teamData":`/);
  assert.match(backend, /`"arenaCombinations":`/);
  assert.match(backend, /Region:\s+"GLOBAL"/);
});

test("match history keeps arena summaries compact and arena details purpose-built", () => {
  assert.match(gameplayScript, /grouping\.groups\.slice\(0,\s*4\)\.map/);
  assert.match(gameplayScript, /if \(matchPlayerGroups\(match\)\.arena\) \{\s*return `<div[^`]+is-arena-detail[^`]+renderArenaMatchOverview\(match\)/s);
  assert.match(gameplayScript, /function renderArenaMatchOverview\(match\)/);
  assert.match(gameplayScript, /arena-detail-augments[^\n]+augmentIconFigure/);
  assert.doesNotMatch(gameplayScript, /if \(matchPlayerGroups\(match\)\.arena\)[\s\S]{0,220}match-detail-tabs/);
});

test("match tier hydration is asynchronous and isolated by region, server, tab and container", () => {
  assert.match(gameplayScript, /function matchTierScope\(tab\)/);
  assert.match(gameplayScript, /`cn:\$\{tabServerID\(tab\) \|\| "current"\}`/);
  assert.match(gameplayScript, /return `\$\{region\}:\$\{tab\?\.key[^`]+:\$\{playerRef\}`/);
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
  assert.doesNotMatch(html.replace(/<[^>]*>/g, " "), /\b(?:HN1|HN10|NJ100|GZ100|CQ100|TJ100|TJ101|BGP2|PBE|KR)\b/);
  assert.match(appScript, /function searchServerID\(\)/);
  assert.match(appScript, /function visibleRegionMenuEntries\(\)/);
  assert.match(appScript, /!entry\.disabled && !entry\.closest\("\[hidden\]"\)/);
  assert.match(appScript, /preference\("search-server-id", ""\)/);
  assert.match(appScript, /window\.addEventListener\("deep-legends:status"[^\n]+updateSearchRegionStatus/);
  assert.match(appScript, /savePreference\("search-server-id"/);
  assert.match(appScript, /detail: \{ gameName, tagLine, region, serverId, source: "search" \}/);
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
  assert.match(gameplayStyles, /\.player-tab-region\.is-kr/);
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
  assert.match(gameplayScript, /renderRanks\(data\.ranks \|\| \[\], data\.capabilities \|\| \[\]\)/);
  assert.match(gameplayScript, /rankedCapability\?\.state === "unsupported"/);
  assert.match(gameplayScript, /跨服暂不支持排位/);
  assert.match(gameplayScript, /当前客户端的排位接口只能读取登录服务器/);
  assert.match(gameplayScript, /class="rank-unavailable"/);
  assert.match(gameplayScript, /crossServerRankUnsupported \? \{ unsupported: true \}/);
  assert.match(gameplayScript, /跨服暂不支持排位，无法计算本场平均段位/);
  assert.doesNotMatch(gameplayScript, /rank-unavailable[^`]+data-gameplay-retry/);
});

test("privacy UI separates explicit and opt-in automatic writes", () => {
  assert.match(appScript, /group\("明确点击后写入客户端", data\.explicitWrites \|\| \[\]\)/);
  assert.match(appScript, /group\("设置开启后自动写入客户端", data\.automaticWrites \|\| \[\]\)/);
  assert.match(appScript, /group\("外部读取", data\.externalReads \|\| \[\]\)/);
});

test("live recommendations render and select every returned option", () => {
  assert.match(gameplayScript, /const opggItems = Array\.isArray\(runes\.opgg\) \? runes\.opgg :/);
  assert.match(gameplayScript, /const itemRoutes = build\.itemRoutes \|\|/);
  assert.match(gameplayScript, /function buildItemSetPayload\(build, self, data\)/);
  assert.match(gameplayScript, /\/api\/gameplay\/item-sets\/apply/);
  assert.match(gameplayScript, /data-apply-item-set/);
  assert.match(gameplayScript, /liveRecommendations:\s*new Map\(\)/);
  assert.match(gameplayScript, /function ensureLiveRecommendations\(data\)/);
  assert.match(gameplayScript, /\/api\/gameplay\/recommendations\?/);
  assert.match(mainSource, /GET \/api\/gameplay\/specialist-runes/);
  assert.match(gameplayScript, /specialistRunes:\s*new Map\(\)/);
  assert.match(gameplayScript, /specialistRuneFlights:\s*new Set\(\)/);
  assert.match(gameplayScript, /function ensureSpecialistRunes\(data\)/);
  assert.match(gameplayScript, /\/api\/gameplay\/specialist-runes\?/);
  assert.match(gameplayScript, /ensureLiveRecommendations\(state\.live\);\s*ensureSpecialistRunes\(state\.live\);/);
  assert.match(gameplayScript, /if \(!Array\.isArray\(response\)\)/);
  assert.match(gameplayScript, /liveRecommendationTarget\(state\.live\)\?\.key === target\.key/);
  assert.match(specialistSource, /specialistRuneCacheTTL\s*=\s*6 \* time\.Hour/);
  assert.match(specialistSource, /specialistRuneRequestBudget\s*=\s*24/);
  assert.match(specialistSource, /specialistRuneMatchScanMax\s*=\s*8/);
  assert.doesNotMatch(gameplayScript, /runes\.opgg\.slice\(0,\s*2\)/);
  assert.doesNotMatch(gameplayScript, /runes\.specialists[^\n]+slice\(0,\s*2\)/);
  assert.doesNotMatch(gameplayScript, /(?:spellOptions|starterOptions|bootOptions|itemRoutes)\.slice\(/);
});

test("rune application creates a new page without confirmation or overwrite", () => {
  assert.doesNotMatch(html, /setting-confirm-runes|符文写入确认/);
  assert.doesNotMatch(gameplayScript, /confirmRunes|confirm-runes|result\.overwritten|覆盖当前或第一个可编辑页/);
  assert.match(gameplayScript, /sourceLabel: "OPGG"/);
  assert.match(gameplayScript, /sourceLabel: "绝活哥"/);
  assert.match(gameplayScript, /championName: self\.championName, source: recommendation\.sourceLabel/);
  assert.match(gameplayScript, /符文已新建并设为当前页/);
  assert.match(gameplayScript, /将新建符文页并设为当前页/);
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
  assert.doesNotMatch(gameplayScript, /当前 OP\.GG 专家榜不返回玩家的完整符文/);
  assert.match(gameplayScript, /当前数据源不提供可核验的职业选手身份与完整符文/);
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
  assert.match(gameplayScript, /5008:\s*"获得9适应之力/);
  assert.match(backend, /5005:\s*"获得10%攻击速度/);
  assert.match(script, /classList\.add\("has-loaded-image"\)/);
  assert.match(gameplayScript, /classList\.add\("has-loaded-image"\)/);
  assert.match(appStyles, /\.game-icon\.has-loaded-image > span, \.champion-asset\.has-loaded-image > span\s*\{\s*display:\s*none/);
  assert.match(styles, /\.champion-rune-column\.is-shards \.rune-icon img\s*\{[^}]*object-fit:\s*contain/s);
});

test("tooltips use one viewport-aware portal instead of clipped pseudo elements", () => {
  assert.match(appScript, /function setupFloatingTooltips\(\)/);
  assert.match(appScript, /getBoundingClientRect\(\)/);
  assert.match(appScript, /window\.innerWidth - width - viewportPadding/);
  assert.match(appScript, /fitsAbove/);
  assert.match(appStyles, /\.global-tooltip\s*\{[^}]*position:\s*fixed[^}]*z-index:\s*55/s);
  assert.doesNotMatch(gameplayStyles, /\.rune-option-button::after|\.skill-icon-button::after|\.item-option-button::after/);
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

test("network preload starts before the champion workspace is opened", () => {
  assert.match(script, /function beginStartupPreload\(\)/);
  assert.match(script, /beginStartupPreload\(\);\s*\}\)\(\);/s);
  assert.match(script, /preload-ranked/);
  assert.match(script, /preload-aram/);
  assert.match(script, /preload-arena/);
  assert.match(script, /if \(hydrated\) \{\s*stampRankings\(\);\s*render\(\);\s*return;/s);
  assert.match(script, /state\.preloaded\.arena/);
});

test("champion data survives section switches and re-entry renders instantly while fresh", () => {
  assert.match(script, /function rankingsFresh\(\)/);
  assert.match(script, /if \(rankingsFresh\(\)\) \{ render\(\); return; \}/);
  assert.match(script, /resetTransientChampionState\(\{ restorePosition: true \}\);[\s\S]*state\.mode = "ranked";[\s\S]*state\.tier = "emerald_plus";/);
});

test("ranked detail always keeps matchup and top-player cards", () => {
  assert.match(script, /该段位暂无足够对线样本/);
  assert.match(script, /暂时没有该英雄的高场次玩家样本/);
  assert.match(script, /detail\.countersTier \|\| detail\.sampleTier/);
  assert.match(styles, /\.counter-group \.counter-empty/);
});

test("convenience settings persist through the local gameplay API", () => {
  assert.match(html, /id="setting-auto-accept"/);
  assert.match(html, /id="setting-auto-play-again"/);
  assert.match(html, /id="setting-auto-reconnect"/);
  assert.match(gameplayScript, /\/api\/gameplay\/convenience/);
  assert.match(gameplayScript, /function bindConvenienceSettings\(\)/);
  assert.match(appScript, /convenience:accept/);
  assert.match(appScript, /已自动接受对局/);
});

test("disconnected collection, account, live and pool pages hide filters", () => {
  assert.match(appScript, /overviewTabIsCurrent/);
  assert.match(appScript, /accountHeading:/);
  assert.match(appScript, /poolHeading:/);
  assert.match(appScript, /登录国服客户端并进入大厅后，这里会按类别展示战利品/);
  assert.match(gameplayScript, /登录国服客户端并进入英雄选择或对局后/);
  assert.match(appStyles, /#favorites-account-panel > \.page-heading\[hidden\] \+ #account-content\s*\{[^}]*padding-top:\s*0/s);
  assert.match(appStyles, /scrollbar-gutter:\s*stable/);
});

test("optional themes keep a neutral background and accent-only primary", () => {
  assert.match(html, /option value="crimson">血月红<\/option>/);
  assert.match(html, /option value="aurora">极光青<\/option>/);
  assert.match(appScript, /"crimson", "aurora"/);
  assert.match(appStyles, /data-theme="crimson"/);
  assert.match(appStyles, /data-theme="aurora"/);
});
