(async () => {
  "use strict";
  if (window.deepLegendsPreferenceStorage?.ready) await window.deepLegendsPreferenceStorage.ready;

  const escapeHTML = window.deepLegendsRuntime.escapeHTML;
  const gradeBadge = window.deepLegendsRuntime.gradeBadge;
  const gradeRank = window.deepLegendsRuntime.gradeRank;

  const nodes = Object.fromEntries([
    "player-tabs", "player-tabs-prev", "player-tabs-next", "overview-content", "overview-refresh",
    "player-groups",
    "live-content", "live-refresh", "live-session-summary", "toast",
    "player-overlay", "player-overlay-back", "player-overlay-title", "player-overlay-add", "player-overlay-content",
    "career-dialog", "career-dialog-close", "career-dialog-context", "career-dialog-content",
    "setting-default-page", "setting-match-count", "setting-default-match-filter", "setting-live-refresh", "setting-live-interval", "setting-live-order", "setting-mask-names",
    "setting-confirm-replay", "gameplay-settings-status",
  ].map((id) => [camel(id), document.getElementById(id)]));

  function newTabView() {
    return {
      matchFilter: normalizeMatchFilter(readSetting("default-match-filter", "all")),
	  rankedQueueRecent: "420", rankedQueueAbility: "420", rankedQueuePosition: "420",
	  matchScrollTops: new Map(),
	  openMatches: new Set(), matchDetailTabs: new Map(), damageSorts: new Map(), teamAnalysisMetrics: new Map(),
	  matchViewRevision: 0,
	  mayhemRating: { status: "idle", playerRef: "", data: null, error: "" },
	  mayhemRatingRequestToken: 0,
      playerRefs: new Set(),
      nextBegIndex: 0, paginationStalls: 0, advancedConditions: {},
    };
  }

  const state = {
    status: null,
    section: "overview",
    tabs: [{ key: "current", playerRef: "", label: "当前召唤师", current: true, loading: false, data: null, error: "", ...newTabView() }],
    activeGroup: readSetting("last-client-player-group", "players") === "kr" ? "kr" : "players",
    activeTabs: { players: "current", kr: "", pro: "" },
    // 两套总览分别保存页签切换历史，关闭页签时只回到同一类账号。
    tabHistories: { players: [], kr: [], pro: [] },
    // 覆盖层栈：在非总览页点击玩家名称时，于当前页面之上展示该玩家的总览。
    overlay: [],
    friendPresence: new Map(),
    playerTabOrder: readPlayerTabOrder(),
    draggedPlayerTab: "",
    controllers: new Map(),
    perks: null,
    perksLoading: false,
    perksRequestToken: 0,
    augmentCatalog: null,
    augmentCatalogLoading: false,
    augmentDescriptions: new Map(),
    perkStatsRefreshes: new Map(),
    items: null,
    itemsLoading: false,
    summonerSpells: null,
    summonerSpellsLoading: false,
    live: null,
    liveLoading: false,
    liveLoadingVisible: false,
    // R129 P3：本轮加载来源、待消费的一次性触发覆盖、以及重建诊断聚合桶。
    liveLoadSource: "direct",
    liveRenderTrigger: "",
    liveRenderSource: "direct",
    liveRenderRebuild: null,
    liveLoadingVisibleTimer: 0,
    liveAwaitingGame: false,
    liveExpectedGameId: 0,
    liveGameRefreshQueued: false,
    liveError: "",
    liveTimer: 0,
    liveRecommendations: new Map(),
    liveRecommendationTraces: new Map(),
    liveRecommendationScopes: new Map(),
    liveRecommendationSpells: new Map(),
    liveRecommendationRenderDiagnostics: new Map(),
    liveRecommendationRenderPhase: "",
    liveRecommendationScopeGameId: 0,
    liveScopeLastReset: null,
    liveRecommendationFlights: new Map(),
    liveRecommendationFailures: new Map(),
    // R116-D：推荐缓存 key 的格式被 champions.test.cjs 钉死（不含阵容），
    // 所以「阵容变了要重取」用这张旁表记录：签名不同就当缓存失效。
    liveRecommendationRosters: new Map(),
    laneMatchupCandidates: new Map(),
    laneMatchupPairs: new Map(),
    laneMatchupCardDiagnostics: new Set(),
    laneMatchupLanes: new Map(),
    liveRecommendationSkipDiagnostics: new Set(),
    specialistRuneSkipDiagnostics: new Set(),
    liveRosterRenderDiagnostics: new Set(),
    livePositionOverride: new Map(),
    proRunes: new Map(),
    proRuneFlights: new Map(),
    proRuneTimer: 0,
    specialistRunes: new Map(),
    specialistRuneFlights: new Map(),
    specialistRuneFailures: new Map(),
    runeStarterRequests: new Map(),
	seasonProgressRefreshes: window.deepLegendsRuntime?.createCache({ max: 32, ttl: 3600000, dispose: (entry) => clearTimeout(entry.timer) }) || new Map(),
    specialistPlayerTabs: new Map(),
    liveGameGeneration: 0,
    runeSourceTab: "opgg",
    recommendationTab: "runes",
    recommendationTabTouched: false,
    selectedRecommendation: "opgg",
    matchObserver: null,
    proMatchObserver: null,
    overlayObserver: null,
	matchTierObserver: null,
	proMatchTierObserver: null,
	overlayTierObserver: null,
    careerDialogOpener: null,
    careerDialogObserver: null,
    overviewShareStatus: "idle",
    overviewShareResetTimer: 0,
    // 平均段位缓存按区域、页签、玩家与 gameId 隔离；null 表示查过但不可用。
    matchTiers: window.deepLegendsRuntime?.createCache({ max: 256, ttl: 600000 }) || new Map(),
    matchTierFlights: new Set(),
    matchTierFailures: window.deepLegendsRuntime?.createCache({ max: 256, ttl: 60000 }) || new Map(),
    // 对局时间线缓存（全员装备路线 + 技能加点）：key = region:server:gameId。
    matchTimelines: window.deepLegendsRuntime?.createCache({ max: 128, ttl: 1800000 }) || new Map(),
    matchTimelineFlights: new Set(),
    beacon: { active: false, acked: false, phase: "" },
    lastCapabilities: [],
    queueGroups: [],
    settings: {
      defaultPage: readSetting("default-page", "overview"),
      matchCount: normalizeMatchCount(readSetting("match-count", "20")),
      defaultMatchFilter: normalizeMatchFilter(readSetting("default-match-filter", "all")),
      liveRefresh: readSetting("live-refresh", "true") !== "false",
      liveInterval: normalizeLiveInterval(readSetting("live-interval", "3")),
      liveOrder: normalizeLiveOrder(readSetting("live-order", "team")),
      maskNames: readSetting("mask-names", "false") === "true",
      // 回放默认直接在客户端内播放，不再弹确认框（可在设置中重新开启）。
      confirmReplay: readSetting("confirm-replay", "false") === "true",
    },
  };
  const externalMatchViews = new Map();

  function camel(value) { return value.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase()); }
  function readSetting(key, fallback) {
    try {
      const name=`lol-loot-${key}`,value=(globalThis.deepLegendsPreferenceStorage || localStorage).getItem(name);
      if(value==null)return fallback;
      if(key==="default-match-filter" && !["all","solo","flex"].includes(value) || ["mask-names","confirm-replay"].includes(key) && !["true","false"].includes(value)) {(globalThis.deepLegendsPreferenceStorage || localStorage).removeItem(name);return fallback;}
      return value;
    } catch (_) { return fallback; }
  }
  function writeSetting(key, value) { try { (globalThis.deepLegendsPreferenceStorage || localStorage).setItem(`lol-loot-${key}`, String(value)); } catch (_) {} }
  function normalizeMatchCount(value) { const parsed = Number(value); return [10, 20, 30, 40, 50].includes(parsed) ? parsed : 20; }
  function normalizeMatchFilter(value) { return ["all", "solo", "flex"].includes(value) ? value : "all"; }
  function normalizeLiveInterval(value) { const parsed = Number(value); return [3, 5, 10, 15, 30, 60].includes(parsed) ? parsed : 3; }
  function liveRefreshDelayMs(phase, configuredSeconds) {
    const normalized = String(phase || "").trim();
    if (normalized === "ChampSelect") return 3_000;
    return normalizeLiveInterval(configuredSeconds) * 1_000;
  }
  function liveClientPositionsPending(data) {
    return (data?.capabilities || []).some(capability => capability.name === "live-client-positions" && capability.state === "pending");
  }
  function liveSnapshotComplete(data) {
    if (liveClientPositionsPending(data)) return false;
    if (!data?.available || !Array.isArray(data.players) || !data.players.length) return false;
    if ([1700, 1710, 1750].includes(Number(data.queueId)) && ["GameStart", "InProgress", "Reconnect"].includes(data.phase)) {
      if (data.players.length < (Number(data.queueId) === 1750 ? 18 : 16)) return false;
      if (data.players.some(player => player.hidden || player.identityUnresolved || (player.gameName && !player.playerRef))) return false;
    }
    // Mirror the 5v5 queues in queue_groups.go.
    if ([400, 420, 430, 440, 450, 480, 490, 700, 720, 900, 930, 1900, 2300, 2400, 3220, 3270].includes(Number(data.queueId))) {
      if (data.players.some(player => player.hidden || player.identityUnresolved)) return false;
      const blue = data.players.filter((player) => Number(player.teamId) === 100).length;
      const red = data.players.filter((player) => Number(player.teamId) === 200).length;
      if (data.players.length !== 10 || blue !== 5 || red !== 5) return false;
    }
    if (data.players.some((player) => ["pending", "failed"].includes(player.historyState))) return false;
    return liveAugmentRecommendationSource(data) !== "arena" || data.arenaGrouped === true || data.arenaGroupingUnavailable === true && data.arenaGroupingRetryable !== true;
  }

  // R129 P1：后端 liveHistoryState 只产出 ok / empty / failed / unavailable
  function liveHistoryStateOf(player) {
    return String(player?.historyState || "empty").trim().toLowerCase();
  }

  function liveHistorySettled(player) {
    return ["ok", "empty", "failed", "unavailable"].includes(liveHistoryStateOf(player));
  }
  function syncLiveRetryBudget() {
    const key = `${state.beacon.phase || state.live?.phase || ""}:${state.live?.gameId || 0}`;
    if (state.liveRetryKey !== key) { state.liveRetryKey = key; state.liveRetryAttempts = 0; state.liveRetryStartedAt = Date.now(); }
    const positionKey = `${Number(state.liveGameGeneration || 0)}:${state.live?.gameId || 0}`;
    if (state.livePositionRetryKey !== positionKey) { state.livePositionRetryKey = positionKey; state.livePositionRetryAttempts = 0; }
  }
  function liveAutoRefreshStopped(data) {
    if (!["InProgress", "Reconnect"].includes(data?.phase)) return "";
    if (liveClientPositionsPending(data)) return "";
    if (data?.arenaGroupingUnavailable && !data?.arenaGrouped && Number(state.liveRetryAttempts || 0) < 8) return data.arenaGroupingRetryable ? "小队数据尚未完整，正在自动重试" : "暂时无法确认小队，可手动刷新";
    if (liveSnapshotComplete(data)) return "对局中数据不再变化，已停止自动刷新";
    if (Number(state.liveRetryAttempts || 0) >= 8) return "数据尚未完整，已停止自动重试，可手动刷新";
    return "";
  }
  function renderLiveRefreshStatus(data) {
    const message = data?.phase && liveGamePhase(data.phase) && data.phase !== state.beacon.phase
      ? "对局阶段已变化，正在同步当前对局…"
      : state.liveLoadingVisible ? "正在刷新…"
      : "";
    if(message)return `<div class="live-refresh-status" role="status"><span>${escapeHTML(message)}</span></div>`;
    if(data?.historyServiceDown===true) {
      const reason='官方战绩服务暂时中断，部分玩家战绩无法读取，恢复后点「刷新对局」';
      return `<div class="live-refresh-status is-service-outage" role="status">${serviceOutageIcon()}<span data-tooltip="${escapeHTML(reason)}" data-tooltip-overflow="self">${reason}</span></div>`;
    }
    return "";
  }
  function normalizeLiveOrder(value) { return ["team", "position", "kda", "win-rate"].includes(value) ? value : "team"; }
  function liveRecommendationTier() {
    const tier = String(readSetting("champion-tier", "emerald_plus")).trim();
    return ["all", "challenger", "grandmaster", "master_plus", "master", "diamond_plus", "diamond", "emerald_plus", "emerald", "platinum_plus", "platinum", "gold_plus", "gold", "silver", "bronze", "iron"].includes(tier)
      ? tier
      : "emerald_plus";
  }
  const numberFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 });
  function number(value) { const parsed = Number(value); return Number.isFinite(parsed) ? numberFormatter.format(parsed) : "—"; }
  function plainInteger(value) { const parsed = Number(value); return Number.isFinite(parsed) ? String(Math.round(parsed)) : "—"; }
  function percent(value) { if (value === null || value === undefined || String(value).trim() === "") return "—"; const parsed = Number(value); return Number.isFinite(parsed) ? `${Math.round(parsed)}%` : "—"; }
  function compactNumber(value) { if (value === null || value === undefined || String(value).trim() === "") return "—"; const parsed = Number(value); if (!Number.isFinite(parsed)) return "—"; if (parsed >= 10000) return `${(parsed / 10000).toFixed(parsed >= 100000 ? 0 : 1)}万`; return number(parsed); }
  function kda(value) { const parsed = Number(value); return Number.isFinite(parsed) ? parsed.toFixed(2) : "—"; }

  const MAX_BROWSE_MATCHES = 1000;
  const LIVE_PREMADE_MIN_SHARED_GAMES = 5;
  // Keep the client batch size aligned with rank_insights.go's request cap.
  const MATCH_TIERS_MAX_REFS = 24;
  // R127 P1-b.3：同时在途的批次数。两批就能把串行等待砍掉一半，再多会和本机
  // 客户端的段位查询抢连接。
  const MATCH_TIERS_PARALLEL_BATCHES = 2;
  // R127 P1-c.4：韩服平均位请求的合并窗口。观测器分几次回调送进来的可见对局
  // 会在这段窗口里攒成一次请求，省掉两次只是「在等第一次」的往返。
  const MATCH_TIER_KR_COALESCE_MS = 120;
  const MATCH_TIER_RETRY_BASE_MS = 1_000;
  const MATCH_TIER_MAX_BACKOFF_MS = 60_000;
  const AUTO_PAGE_DELAY_MS = 400;
  const AUTO_PAGE_MAX_BACKOFF_MS = 8000;
  const OVERVIEW_SHARE_STATUS = Object.freeze({
    idle: ["生成分享图", "download"],
    choosing: ["选择保存位置…", "loading"],
    rendering: ["正在生成超清图片…", "loading"],
    success: ["分享图已保存", "success"],
    error: ["生成失败，请重试", "error"],
  });

  const CN_SERVER_LABELS = Object.freeze({
    HN1: "艾欧尼亚", HN10: "黑色玫瑰", NJ100: "联盟一区", GZ100: "联盟二区", CQ100: "联盟三区",
    TJ100: "联盟四区", TJ101: "联盟五区", BGP2: "峡谷之巅", PBE: "体验服",
  });

  // 临时行为开关：英雄选择阶段点击预选英雄后立即加载推荐。
  // 恢复“必须锁定英雄”时只需改为 false；符文/装备写入仍由后端强制限制在 ChampSelect。
  const USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS = true;
  const LIVE_CORE_OPTION_LIMIT = 5;

  async function api(path, options = {}, key = path, timeout = 30000) {
    const overviewRequest=String(key).startsWith('overview:');
    if(overviewRequest && typeof window!=="undefined")window.dispatchEvent(new CustomEvent('deep-legends:manual-refresh-metric',{detail:{overview_requests:1}}));
    state.controllers.get(key)?.abort();
    const controller = new AbortController();
    state.controllers.set(key, controller);
    let timedOut = false;
    let errorStage = "network";
    let responseStatus = 0;
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, timeout);
    try {
      const { onProgress, ...requestOptions } = options;
      const headers = new Headers(options.headers || {});
      headers.set("Accept", onProgress ? "application/x-ndjson" : "application/json");
      if(onProgress)headers.set("X-Overview-Cards","1");
      const responseError = (status, body, retryHeader) => {
        let payload = {};
        try { payload = typeof body === "string" ? JSON.parse(body) : body; } catch (_) {}
        const error = new Error(payload?.error || (typeof body === "string" ? body.trim() : "") || `本地服务返回 HTTP ${status}`);
        error.status = status;
        error.kind = payload?.kind;
        error.errorKind = status === 429 ? "rate-limited" : "http";
        const rawRetry = payload?.retryAfter ?? retryHeader;
        const seconds = Number(rawRetry);
        error.retryAfter = Math.max(1, Number.isFinite(seconds) && seconds > 0 ? seconds : (Date.parse(rawRetry) - Date.now()) / 1000 || 5);
        return error;
      };
      if (options.body) headers.set("Content-Type", "application/json");
      const response = await fetch(path, { ...requestOptions, headers, signal: controller.signal });
      responseStatus = response.status;
      errorStage = "read";
      if (response.status === 401) {
        const error = new Error("页面会话已过期，刷新页面即可重新连接");
        error.status = 401;
        error.errorKind = "http";
        throw error;
      }
      if (!response.ok) {
        throw responseError(response.status, await response.text(), response.headers?.get("Retry-After"));
      }
      errorStage = "decode";
      let payload;
      if (onProgress && response.headers?.get("Content-Type")?.includes("application/x-ndjson")) {
        const reader = response.body.getReader(), decoder = new TextDecoder();
        let buffer = "", complete = false;
        const stream = {matches:new Map(),overview:{}};
        const receive = line => {
          if (!line.trim()) return;
          if (controller.signal.aborted || state.controllers.get(key) !== controller || state.destroyed) { const error = new Error("请求已取消"); error.name = "RequestCancelled"; throw error; }
          const frame = JSON.parse(line);
          if (frame.type === "error") throw responseError(frame.status, frame, frame.retryAfter);
          if (frame.type === "progress" || frame.type === "cards") onProgress(resolveOverviewFrame(stream,frame.overview),frame.type);
          else if (frame.type === "complete") { payload = resolveOverviewFrame(stream,frame.overview); complete = true; }
        };
        try {
          while (true) {
            const { value, done } = await reader.read();
            buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
            let split;
            while ((split = buffer.indexOf("\n")) >= 0) { const line = buffer.slice(0, split); buffer = buffer.slice(split + 1); receive(line); }
            if (done) { receive(buffer); break; }
          }
          if (!complete) throw new Error("战绩读取中断，请重试补齐");
        } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
      } else payload = response.status === 204 ? null : await response.json();
      if (controller.signal.aborted || state.controllers.get(key) !== controller || state.destroyed) {
        const aborted = new Error("请求已取消");
        aborted.name = "RequestCancelled";
        throw aborted;
      }
      return payload;
    } catch (error) {
      if (state.controllers.get(key) !== controller || state.destroyed) {
        const cancelled = new Error("请求已取消");
        cancelled.name = "RequestCancelled";
        cancelled.errorKind = "canceled";
        throw cancelled;
      }
      if (timedOut) {
        const timeoutError = new Error("本地请求超时，请重试");
        timeoutError.name = "TimeoutError";
        timeoutError.errorKind = "timeout";
        throw timeoutError;
      }
      if (error.name === "AbortError") {
        const aborted = new Error("请求已取消");
        aborted.name = "RequestCancelled";
        aborted.errorKind = "canceled";
        throw aborted;
      }
      error.errorKind ||= errorStage === "decode" && error.name !== "SyntaxError" ? "read" : errorStage;
      if (responseStatus) error.status ||= responseStatus;
      throw error;
    } finally {
      clearTimeout(timer);
      if(overviewRequest && typeof window!=="undefined" && (controller.signal.aborted || state.controllers.get(key)!==controller))window.dispatchEvent(new CustomEvent('deep-legends:manual-refresh-metric',{detail:{canceled_requests:1}}));
      if (state.controllers.get(key) === controller) state.controllers.delete(key);
    }
  }

  function resolveOverviewFrame(stream, overview) {
    if (!overview || typeof overview !== "object") throw new Error("战绩帧无效，请重试");
    const next={...stream.overview,...overview};
    if (Array.isArray(overview.matches)) next.matches=overview.matches.map(match=>{
      const key=String(match.gameId),reference=Object.keys(match).every(key=>["gameId","revision"].includes(key));
      if(reference && Object.hasOwn(match,'revision')){const full=stream.matches.get(key);if(!full || (full.revision || '')!==(match.revision || '')){const error=new Error("战绩引用需要重新读取");error.referenceFallback=full?"mismatch":"missing";throw error;};return full;}
      stream.matches.set(key,match);return match;
    });
    stream.overview=next;
    return next;
  }

  function connected() { return state.status?.clientView?.state === "ready"; }

  const RIOT_REGIONS = { kr: "韩服", jp1: "日服", na1: "美服", euw1: "欧西", eun1: "欧北东", br1: "巴西", la1: "拉北", la2: "拉南", me1: "中东", oc1: "大洋洲", ru: "俄服", sg2: "东南亚", tr1: "土耳其", tw2: "台服", vn2: "越南" };
  function riotRegion(region) { return Object.hasOwn(RIOT_REGIONS, String(region || "").toLowerCase()); }
  function clientRegion() { const region = String(state.status?.clientRegion || "").toLowerCase(); return riotRegion(region) ? region : ""; }
  const PLAYER_GROUPS = { players: "国服", kr: "外服", pro: "职业" };
  const PLAYER_GROUP_EMPTY_TIPS = { kr: "在顶栏搜索里选外服服务器并搜索玩家", pro: "从职业选手目录打开账号" };
  let playerGroupMenuOpen = false;
  let playerGroupOpenTimer = 0;
  let playerGroupCloseTimer = 0;
  function overviewGroupForSection() { return state.activeGroup || "players"; }
  function overviewSectionForGroup() { return "overview"; }
  function overviewWorkspace() {
    return { tabs: nodes.playerTabs, prev: nodes.playerTabsPrev, next: nodes.playerTabsNext, content: nodes.overviewContent, refresh: nodes.overviewRefresh };
  }
  function activeTab(group = overviewGroupForSection()) {
    const tabs = visiblePlayerTabs(group);
    return tabs.find(tab => tab.key === state.activeTabs[group]) || tabs[0] || null;
  }
  function visiblePlayerTabs(group) {
    return state.tabs.filter(tab => (tabGroup(tab) === group || group === "players" && tab.current && tab.regionResolved === false)
      && (!tab.current || !tab.closed && selfTabReady()) && (group !== "players" || connected() || riotTab(tab)));
  }
  function playerGroupCount(group) {
    return visiblePlayerTabs(group).length;
  }
  function visiblePlayerGroups() {
    return Object.keys(PLAYER_GROUPS);
  }
  function selectPlayerGroup(group) {
    state.lastManualTabAt = Date.now();
    if (!visiblePlayerGroups().includes(group)) return;
    if(state.activeGroup!==group)delete activeTab()?.overviewDialogReturn;
    const tab = activeTab(group);
    if (tab) selectPlayerTab(tab.key);
    else { if(globalThis.deepLegendsHistoryFilters)cancelAdvancedMatchSearch(activeTab()); state.activeGroup = group; if(typeof window!=="undefined")window.dispatchEvent(new CustomEvent("deep-legends:overview-group",{detail:{group}}));renderPlayerTabs(); renderOverview(group); }
  }
  function playerGroupButton() { return document.getElementById("player-group-button"); }
  function playerGroupMenu() { return document.getElementById("player-group-menu"); }
  function enabledPlayerGroupItems() {
    return [...(playerGroupMenu()?.querySelectorAll('[data-player-group]:not([aria-disabled="true"])') || [])];
  }
  function focusPlayerGroupItem(target = "active") {
    requestAnimationFrame(() => {
      if (!playerGroupMenuOpen || state.destroyed) return;
      const items = enabledPlayerGroupItems();
      const active = items.find(item => item.dataset.playerGroup === (target === "active" ? overviewGroupForSection() : target));
      const item = target === "first" ? items[0] : target === "last" ? items.at(-1) : active || items[0];
      item?.focus();
    });
  }
  function setPlayerGroupMenu(open, focus = "") {
    clearTimeout(playerGroupOpenTimer);
    clearTimeout(playerGroupCloseTimer);
    playerGroupOpenTimer = 0;
    playerGroupCloseTimer = 0;
    playerGroupMenuOpen = Boolean(open);
    const picker = nodes.playerGroups;
    const button = playerGroupButton();
    const menu = playerGroupMenu();
    picker?.classList.toggle("is-open", playerGroupMenuOpen);
    button?.setAttribute("aria-expanded", String(playerGroupMenuOpen));
    if (menu) menu.hidden = !playerGroupMenuOpen;
    if (playerGroupMenuOpen && focus) focusPlayerGroupItem(focus);
  }
  function schedulePlayerGroupOpen() {
    clearTimeout(playerGroupCloseTimer);
    clearTimeout(playerGroupOpenTimer);
    playerGroupOpenTimer = setTimeout(() => setPlayerGroupMenu(true), 120);
  }
  function schedulePlayerGroupClose() {
    clearTimeout(playerGroupOpenTimer);
    clearTimeout(playerGroupCloseTimer);
    playerGroupCloseTimer = setTimeout(() => setPlayerGroupMenu(false), 200);
  }
  function choosePlayerGroup(group) {
    if (!visiblePlayerGroups().includes(group)) return;
    setPlayerGroupMenu(false);
    selectPlayerGroup(group);
    requestAnimationFrame(() => playerGroupButton()?.focus());
  }
  function selectAdjacentPlayerGroup(key) {
    const groups = visiblePlayerGroups();
    if (!groups.length) return;
    const index = Math.max(0, groups.indexOf(overviewGroupForSection()));
    const next = key === "Home" ? 0 : key === "End" ? groups.length - 1 : (index + (key === "ArrowRight" ? 1 : -1) + groups.length) % groups.length;
    selectPlayerGroup(groups[next]);
    requestAnimationFrame(() => playerGroupButton()?.focus());
  }

  function matchObserverKey(tab) {
    if (tab?.overlay) return "overlayObserver";
    return tabGroup(tab) === "pro" ? "proMatchObserver" : "matchObserver";
  }

  function matchTierObserverKey(tab) {
    if (tab?.overlay) return "overlayTierObserver";
    return tabGroup(tab) === "pro" ? "proMatchTierObserver" : "matchTierObserver";
  }

  // 韩服页签通过 Riot 官方 API 查询，不依赖本机客户端连接。
  function riotTab(tab) { return riotRegion(tab?.region || tab?.data?.player?.region); }
  function tabReady(tab) { return (!tab?.current || !tab.closed && selfTabReady()) && (connected() || riotTab(tab)); }
  function tabServerID(tab) {
    if (riotTab(tab)) return "";
    const serverID = String(tab?.data?.player?.serverId || tab?.serverId || "").toUpperCase();
    return CN_SERVER_LABELS[serverID] ? serverID : "";
  }
  function tabServerLabel(tab) {
    if (tab?.current && tab.regionResolved === false) return "";
    if (riotTab(tab)) return RIOT_REGIONS[tab.region || tab.data?.player?.region];
    return CN_SERVER_LABELS[tabServerID(tab)] || tab?.data?.player?.serverName || "国服";
  }
  function tabServerTitle(tab) {
    if (riotTab(tab)) return `${tabServerLabel(tab)} (${String(tab.region || tab.data?.player?.region).toUpperCase()})`;
    const serverID = tabServerID(tab);
    return serverID ? `国服 · ${tabServerLabel(tab)} (${serverID})` : "国服";
  }

  function shouldReloadOverview(tab, now = Date.now()) {
    return Boolean(tab?.dirty) || !tab?.data || now - Number(tab.loadedAt || 0) >= 120_000;
  }

  function matchTierScope(tab) {
    const region = riotTab(tab) ? tab.region : "cn";
    const serverID = riotTab(tab) ? tab.region : (tabServerID(tab) || "current");
    const playerRef = tab?.data?.player?.playerRef || tab?.playerRef || `${tab?.riotId?.gameName || ""}#${tab?.riotId?.tagLine || ""}` || "unknown";
    return `${region}:${serverID}:${playerRef}`;
  }

  function matchTierCacheKey(tab, gameID) {
    return `${matchTierScope(tab)}:${String(gameID || "")}`;
  }

  function activateSection(name) {
    const previous = state.section;
    if (previous === "live" && name !== "live" && state.autoLiveEntered) state.autoLiveLeft = true;
    state.section = name;
    if (name !== "live") clearRuneStarterRetries();
    clearTimeout(state.liveTimer);
    clearTimeout(state.perksAugmentTimer);
    clearTimeout(state.currentGameTimer);
    // 切换主页面仅关闭覆盖层；玩家筛选/详情保留，保证返回时滚动锚点不变。
    if (previous !== name) {
      setPlayerGroupMenu(false);
      closeOverlay();
      if (name === "live") {
        state.recommendationTab = "runes";
        state.recommendationTabTouched = false;
      }
    }
    if (name === "live") { state.beacon.acked = true; renderBeacon(); }
    if (name === "overview") {
      const group = overviewGroupForSection(name);
      const tab = activeTab(group);
      if (tab && previous !== name) tab.restoreScrollPending = true;
      if (!tab) {
        renderOverview(group);
      } else if (tabReady(tab)) {
        if (shouldReloadOverview(tab) && tab.loadingMore) {
          renderOverview(group);
        } else if (tab.dirty) { renderOverview(group); scheduleDirtyOverview(tab); }
        else if (shouldReloadOverview(tab)) {renderOverview(group);if(tab.data)void probeOverviewHead(tab);else void loadOverview(tab);}
        else renderOverview(group);
        if(tab.historyRecoveryPending && !tab.loading)retryHistoryRegion(tab,"outage-recovery");
      } else renderOverview(group);
    }
    if (name === "live") {
      if (connected()) {
        renderLive("navigation");
        if (!liveSnapshotComplete(state.live) || state.live?.phase !== state.beacon.phase || state.beacon.phase === "ChampSelect") loadLive();
        else { ensureLiveRecommendations(state.live); scheduleLiveRefresh(); }
      }
      else renderLive("navigation");
    }
    else scheduleLiveRefresh();
    renderBeacon();
  }

  function selfTabReady(status = state.status) {
    return status?.clientView?.state === "ready";
  }

  function restoreCommittedClientStatus() {
    const status = window.deepLegendsStatusSnapshot?.();
    if (status) updateStatus(status);
  }

  function updateStatus(status) {
    // Status continues to carry collection/settings data. Legacy connection
    // fields never enter the tab reducer; only the versioned view can do that.
    const other = {...status};
    for (const key of ["clientView", "connected", "identityReady", "sgpReady", "clientDiscovery", "connectionState", "clientRegion", "serverId", "summoner", "clientVersion"]) delete other[key];
    state.status = {...state.status, ...other};
    if (Array.isArray(other.queueGroups) && other.queueGroups.length) state.queueGroups = other.queueGroups;
    if (status.clientView) updateClientView(status.clientView);
  }

  function updateClientView(view) {
    if (view?.type !== "client-view" || !["ready", "exiting", "no-client"].includes(view.state) || !Number.isSafeInteger(view.generation) || view.generation <= Number(state.status?.clientView?.generation || 0)) return false;
    applyClientViewStatus({...state.status, clientView:view, clientRegion:view.region || "", serverId:view.serverId || "", summoner:view.summoner || {}, clientVersion:view.clientVersion || ""});
    return true;
  }

  function applyClientViewStatus(status) {
    const previous = state.status;
    const wasConnected = connected();
    const wasReady = selfTabReady(previous);
    state.status = status;
    if (status.clientVersion && state.catalogVersion && state.catalogVersion !== status.clientVersion) {
      state.perksRequestToken = Number(state.perksRequestToken || 0) + 1;
      state.itemsRequestToken = Number(state.itemsRequestToken || 0) + 1;
      state.summonerSpellsRequestToken = Number(state.summonerSpellsRequestToken || 0) + 1;
      for (const key of ["perks", "items", "summoner-spells"]) state.controllers.get(key)?.abort();
      state.perks = null; state.items = null; state.summonerSpells = null;
      state.perksLoading = false; state.itemsLoading = false; state.summonerSpellsLoading = false;
      state.itemsRetryAt = 0; state.summonerSpellsRetryAt = 0;
    }
    if (status.clientVersion) state.catalogVersion = status.clientVersion;

    if (Array.isArray(status.queueGroups) && status.queueGroups.length) state.queueGroups = status.queueGroups;
    const current = state.tabs.find(tab => tab.current);
    if (!selfTabReady(status)) {
      if (!wasConnected) return;
      for (const name of ["perks", "items", "summonerSpells"]) {
        state[`${name}RequestToken`] = Number(state[`${name}RequestToken`] || 0) + 1;
        state[`${name}Loading`] = false;
      }
      for (const key of ["perks", "items", "summoner-spells"]) state.controllers.get(key)?.abort();
      if(globalThis.deepLegendsHistoryFilters)for(const tab of [...state.tabs,...state.overlay])if(!riotTab(tab))cancelAdvancedMatchSearch(tab);
      resetTencentTabsAfterDisconnect();
      state.overlay = state.overlay.filter(entry => riotTab(entry));
      state.liveRequestToken = Number(state.liveRequestToken || 0) + 1;
      state.controllers.get("live")?.abort();
      state.liveLoading = false;
      const previousLive = state.live;
      state.live = null;
      state.liveAwaitingGame = false;
      state.liveExpectedGameId = 0;
      state.liveGameRefreshQueued = false;
      state.liveError = "";
      resetLiveGameScopedState(false, { reason: "disconnect", previous: previousLive, next: null });
      updateBeacon("None");
      updateDisconnectedPlayerDOM();
      return;
    }
    if (!wasReady) state.followSelfPending = true;
    if (!selfTabReady(status) || !current) return;
    const wasActive = Object.values(state.activeTabs).includes(current.key);
    const nextRegion = clientRegion();
    const nextResolved = Boolean(status.clientRegion || status.serverId);
    const platformChanged = (current.region || "") !== nextRegion || current.serverId !== (nextRegion ? "" : status.serverId || "");
    const restored = previous?.clientView?.state === "exiting" && !platformChanged && Boolean(status.summoner?.playerRef) && current.identity?.playerRef === status.summoner?.playerRef && Boolean(current.data);
    const nextLabel = summonerLabel(status.summoner || {});
    const identityChanged = Boolean(current.identity?.playerRef && current.identity.playerRef !== status.summoner?.playerRef);
    if (platformChanged || identityChanged) {
      resetPlayerHistoryConditions(current);
      resetCareerState(current); current.playerRef = ""; current.playerRefs = new Set(); current.loadedAt = 0;
    }
    current.region = nextRegion;
    current.serverId = nextRegion ? "" : status.serverId || "";
    current.regionResolved = nextResolved;
    current.group = nextRegion ? "kr" : "players";
    current.closed = false;
    if (wasActive || !state.activeTabs[current.group]) state.activeTabs[current.group] = current.key;
    current.label = nextLabel;
    current.icon = Number(status.summoner?.profileIconId) || 0;
    current.identity = status.summoner || {};
    if (nextRegion && status.summoner?.gameName) current.riotId = {gameName:status.summoner.gameName,tagLine:status.summoner.tagLine || ""};
    try { localStorage.setItem("lol-loot-last-client-player-group", current.group); } catch (_) {}
    if (state.followSelfPending || platformChanged || identityChanged) {
      const fromGroup=state.activeGroup;
      state.followSelfPending = false;
      {
        if(globalThis.deepLegendsHistoryFilters && activeTab()!==current)cancelAdvancedMatchSearch(activeTab());
        state.activeGroup = current.group;
        state.activeTabs[current.group] = current.key;
      }
      if(typeof window!=="undefined")window.reportFlowDiagnostic?.("client_follow_client",!wasReady?"login":platformChanged?"platform-change":"identity-change",{from_group:fromGroup,to_group:current.group,search_from:previous?.serverId || previous?.clientRegion || "",search_to:current.serverId || current.region,applied:true});
    }
    // Identity and its header render in this task; network cards follow independently.
    if (!wasReady) current.headerReported = false;
    renderPlayerTabs();
    if ((!wasReady || platformChanged || identityChanged) && state.section === "overview" && current.key === state.activeTabs[state.activeGroup]) {
      if (restored) activateOverviewView(nodes.overviewContent, activeTab());
      else activateOverviewTabPanel(state.activeGroup);
    }
    if (!wasReady || platformChanged || identityChanged) {
      if (!restored && state.section === "overview") {
        clearTimeout(current.selfOverviewTimer);
        current.selfOverviewTimer = setTimeout(() => {
          current.selfOverviewTimer = 0;
          if (selfTabReady() && !current.overviewCardLoad?.selfReady) handleSelfOverviewTimeout();
        }, 15_000);
        void loadOverview(current, true);
      }
      if (state.section === "live") loadLive();
      scheduleBeaconPoll(0);
    }
  }

  function resetCareerState(tab) {
    tab.careerGeneration = Number(tab.careerGeneration || 0) + 1;
    tab.data = null;
    tab.loadedAt = 0;
    tab.recentStatsReceived = false;
    tab.matchesReceived = false;
    tab.overviewCardLoad = null;
    tab.initialPagePending = true;
    for (const key of ['overviewView','overviewMatchSnapshot','pendingHistoricalRanks','opggSeason','opggSeasonAttemptRef','seasonRefreshPending','seasonRanksFilled','confirmedStats','careerStatsSource','nonRankedConfirmed','casualMode','rankedHistorySeen','casualHistory']) delete tab[key];
  }

  function resetTencentTabsAfterDisconnect() {
    globalThis.deepLegendsHistoryRecovery?.cancelAll();
    const current = state.tabs.find(tab => tab.current);
    if (current) {
      for (const [key, controller] of state.controllers || []) if (key.endsWith(`:${current.key}`) || key.includes(`:${current.key}:`)) controller.abort();
      for (const timer of [current.quotaRetry?.timer, current.appendTimer, current.matchTierRetryTimer, current.selfOverviewTimer, current.historyServerRetryTimer, state.currentGameTimer]) clearTimeout(timer);
      current.quotaRetry = null;
      current.overviewRequestToken = Number(current.overviewRequestToken || 0) + 1;
      current.closed = true;
      current.loading = false; current.loadingMore = false;
      current.currentGamePending = "";
      resetCareerState(current);
    }
    for (const group of ["players", "kr", "pro"]) {
      if (state.activeTabs[group] === current?.key) state.activeTabs[group] = state.tabs.find(tab => !tab.current && tabGroup(tab) === group)?.key || "";
    }
  }

  function updateDisconnectedPlayerDOM() {
    const container = nodes.overviewContent;
    if (container?._overviewViewTab && container._overviewViewTab !== activeTab()) {
      const previous = container._overviewViewTab;
      savePlayerScroll(previous);
      previous.restoreScrollPending = true;
      activateOverviewView(container, activeTab());
    }
    renderPlayerTabs();
    if (nodes.liveContent) nodes.liveContent.replaceChildren();
    const toolbar = nodes.liveRefresh?.closest(".live-toolbar");
    if (toolbar) toolbar.hidden = true;
    if (nodes.playerOverlay) {
      const entry = state.overlay[state.overlay.length - 1];
      if (nodes.playerOverlay.dataset.entryKey !== entry?.key) {
        activateOverviewView(nodes.playerOverlayContent, entry || null);
        nodes.playerOverlay.hidden = !entry;
        if (entry) {
          nodes.playerOverlay.dataset.entryKey = entry.key;
          if (nodes.playerOverlayTitle) nodes.playerOverlayTitle.textContent = state.settings.maskNames ? privacyName(entry.data?.player || entry.identity || {playerRef:entry.key},{context:"identity"}) : entry.label;
          syncOverlayAddButton(entry);
        } else {
          delete nodes.playerOverlay.dataset.entryKey;
        }
      }
    }
    window.reportFlowDiagnostic?.("self_tab_client", "removed",{startedAt:Date.now()});
    window.dispatchEvent(new CustomEvent("deep-legends:overview-tab", {detail:{current:!activeTab() || Boolean(activeTab()?.current),group:state.activeGroup}}));
  }

  function rerenderTab(tab, reason = "unspecified") {
    if(tab)tab.overviewRenderReason=reason;
    if (typeof tab?.externalRender === "function") { tab.externalRender(); return; }
    if (tab.overlay) { renderOverlay(); return; }
    renderPlayerTabs();
    const group = tabGroup(tab);
    if (tab.key === state.activeTabs[group]) renderOverview(group);
  }

  function applyOverviewPlayerIdentity(summoner = state.status?.summoner) {
	const tab = state.tabs.find((candidate) => candidate.current === true);
	if (!tab?.data?.player || !summoner) return false;
	const player = tab.data.player;
	const identity = {
	  gameName: String(summoner.gameName || ""),
	  tagLine: String(summoner.tagLine || ""),
	  displayName: String(summoner.displayName || ""),
	  profileIconId: Number(summoner.profileIconId) || 0,
	  summonerLevel: Number(summoner.summonerLevel) || 0,
	};
	const backgroundFields = ["backgroundSkinId", "backgroundSkinName", "backgroundSource", "backgroundPath", "backgroundPosterPath", "backgroundVideoPath"];
    if (backgroundFields.some((field) => Object.prototype.hasOwnProperty.call(summoner, field)) &&
        (summoner.backgroundPath || summoner.backgroundPosterPath)) {
	  Object.assign(identity, {
		backgroundSkinId: Number(summoner.backgroundSkinId) || 0,
		backgroundSkinName: String(summoner.backgroundSkinName || ""),
		backgroundSource: String(summoner.backgroundSource || ""),
		backgroundPath: String(summoner.backgroundPath || ""),
		backgroundPosterPath: String(summoner.backgroundPosterPath || ""),
		backgroundVideoPath: String(summoner.backgroundVideoPath || ""),
	  });
      if (Number(summoner.backgroundSkinId) === Number(player.backgroundSkinId)) {
        if(!summoner.backgroundPosterPath)identity.backgroundPosterPath = player.backgroundPosterPath || "";
        if(!summoner.backgroundVideoPath)identity.backgroundVideoPath = player.backgroundVideoPath || "";
      }
	}
	const fields = Object.keys(identity);
	const playerChanged = fields.some((field) => player[field] !== identity[field]);
	const label = summonerLabel(summoner);
	const headerChanged = tab.label !== label || Number(tab.icon || 0) !== identity.profileIconId;
	if (!playerChanged && !headerChanged) return false;
	if (playerChanged) tab.data = { ...tab.data, player: { ...player, ...identity } };
	tab.label = label;
	tab.icon = identity.profileIconId;
	renderPlayerTabs();
	rerenderTab(tab,"applyOverviewPlayerIdentity");
	return true;
  }

  function normalizedPagination(payload, requestedBegIndex) {
    const source = payload?.pagination || {};
    const begIndex = Number.isFinite(Number(source.begIndex)) ? Math.max(0, Number(source.begIndex)) : requestedBegIndex;
    const fallbackCount = Array.isArray(payload?.matches) ? payload.matches.length : 0;
    const count = Number.isFinite(Number(source.count)) ? Math.max(0, Number(source.count)) : fallbackCount;
    const nextBegIndex = begIndex + count;
    const reachedLimit = nextBegIndex >= MAX_BROWSE_MATCHES;
    const stalled = count <= 0 || nextBegIndex <= requestedBegIndex;
    const upstreamHasMore = source.hasMore === true || source.partial === true || source.budgetExceeded === true;
    return {
      ...source, begIndex, count,
      hasMore: (upstreamHasMore || source.historyServiceDown) && !reachedLimit && (!stalled || source.historyServiceDown),
      exhaustedReason: reachedLimit ? `已达到单个玩家 ${number(MAX_BROWSE_MATCHES)} 场的浏览上限` : !upstreamHasMore || stalled && !source.partial && !source.budgetExceeded ? "已展示全部可查询战绩" : "",
      nextBegIndex,
    };
  }

  function showLoadingMoreState(tab) {
    const container = overviewContainer(tab);
    const sentinel = container?.querySelector("[data-match-sentinel]");
    if (!sentinel) return;
    sentinel.classList.add("is-loading");
	sentinel.innerHTML = tab.filterPaging ? paginationCopyFor(tab) : '<span class="mini-loading" aria-hidden="true"></span><span>正在加载下一批战绩…</span>';
  }

  function overviewContainer(tab) {
    if (tab?.overlay) return nodes.playerOverlayContent;
    const group = tabGroup(tab);
    return group === overviewGroupForSection() && tab?.key === state.activeTabs[group] ? overviewWorkspace(group).content : null;
  }

  function paginationCopyFor(tab) {
    if((tab.data?.pagination?.historyServiceDown || historyServiceState(tab)?.kind==="official") && !tab.data?.pagination?.localHasMore)return '<span>战绩服务暂时中断</span>';
    if(matchesPending(tab) || globalThis.deepLegendsHistoryFilters?.active(tab) || !(tab.data?.matches?.length) && (tab.initialPageError || tab.error))return "";
	const data = tab.data || {};
	const pagination = data.pagination || { hasMore: false };
	if (tab.quotaRetry && tab.quotaRetry.retryAt>Date.now()) return `<span>外服查询额度恢复中，约 <b data-quota-seconds>${Math.max(0, Math.ceil((tab.quotaRetry.retryAt - Date.now()) / 1000))}</b> 秒后自动继续；已加载战绩仍可查看</span>`;
	if (tab.filterPaging) return `<span class="mini-loading" aria-hidden="true"></span><span>正在查找更早的${escapeHTML(matchFilterDisplayLabel(tab))}对局…第 ${number(Math.max(2, Number(tab.filterPagingPage) || 2))} 页</span>`;
	if (tab.loadingMore) return '<span class="mini-loading" aria-hidden="true"></span><span>正在加载下一批战绩…</span>';
    if (pagination.hasMore) {
      if (pagination.moreError) return `<span>这一页读取失败</span><button class="text-button" type="button" data-load-more>重试</button>`;
      if (pagination.budgetExceeded) return `<span>本次只读到 ${number((data.matches || []).length)} 条，点这里继续</span><button class="text-button" type="button" data-load-more>继续加载</button>`;
      if (pagination.partial) return `<span>上游中断，已加载 ${number((data.matches || []).length)} 条，点击继续</span><button class="text-button" type="button" data-load-more>继续加载</button>`;
      return pagination.moreError
        ? `<span>这一页读取失败</span><button class="text-button" type="button" data-load-more>重试</button>`
        : pagination.autoPaused
          ? `<span>${escapeHTML(pagination.pauseReason || "自动加载已暂停")}</span><button class="text-button" type="button" data-load-more>继续查找</button>`
        : '<span>继续向下滚动，自动加载更多</span><button class="text-button" type="button" data-load-more>加载更多</button>';
    }
    return `<span>${escapeHTML(pagination.exhaustedReason || "已展示全部可查询战绩")} · 共 ${number((data.matches || []).length)} 场</span>`;
  }

  function matchesPending(tab) {
    return Boolean(tab?.loading && (!tab.matchesReceived || tab.data?.pagination?.partial) && !tab.overviewCardLoad?.ready?.has('matches') && !(tab.data?.matches?.length));
  }
  function renderPendingMatches() {
    const line='<span class="skeleton-line"></span>',square='<span class="skeleton-square"></span>';
    const roster='<div class="match-team-list">'+Array.from({length:5},()=>'<div class="match-player">'+square+line+'</div>').join('')+'</div>';
    const row=`<article class="match-entry match-pending match-skeleton" aria-hidden="true"><div class="match-summary">
      <div class="match-result-meta">${line.repeat(3)}</div>
      <div data-match-layout class="match-main"><div class="match-champion"><span class="game-icon is-large is-pending"></span><div class="match-loadout-mini">${square.repeat(4)}</div></div><div class="match-kda"><strong>${line}</strong><small>${line}</small></div><div data-match-metrics class="match-stats">${line.repeat(3)}</div><div class="match-build"><div class="match-items">${Array.from({length:8},()=>'<span class="item-slot is-empty"></span>').join('')}</div><div class="match-badges"><span class="match-tags-collection"><button data-match-tags-more hidden></button></span></div></div></div>
      <div class="match-players">${roster.repeat(2)}</div><div class="match-actions">${square.repeat(2)}</div>
    </div></article>`;
    return row.repeat(5);
  }
  function matchSentinelShouldHide(tab, hasVisibleMatch) {
    return Boolean(matchesPending(tab) || !hasVisibleMatch && (tab?.filterPaging || tab?.loadingMore || tab?.quotaRetry || historyServiceState(tab)));
  }

  function matchListEmptyContent(tab, historyCapability, specialModeEmpty) {
    if(historyServiceState(tab)?.kind==="official")return renderHistoryServiceStatus(tab);
    if (matchesPending(tab)) return renderPendingMatches();
    if (historyServiceState(tab)) return renderHistoryServiceStatus(tab);
    if(!(tab.data?.matches?.length) && (tab.initialPageError || tab.error))return `<div class="af-empty" role="alert"><strong>战绩暂不可用</strong><p>${escapeHTML(tab.initialPageError || tab.error)}</p><button type="button" data-gameplay-retry>重试</button></div>`;
    if (globalThis.deepLegendsHistoryFilters?.active(tab)) return globalThis.deepLegendsHistoryFilters.empty(tab,advancedFilterContext(tab));
	if (historyCapability && historyCapability.state !== "available") {
	  if (historyCapability.historyStatus === "official") return renderHistoryServiceStatus(tab);
	  return emptyState("战绩暂时无法读取", historyCapability.detail || "客户端暂未返回该玩家的战绩，请稍后重试。", true);
	}
	if (tab.filterPaging || tab.loadingMore || tab.quotaRetry) {
	  return `<div class="match-filter-paging" role="status">${paginationCopyFor(tab)}</div>`;
	}
	if (tab.data?.pagination?.hasMore) {
	  return emptyState("当前已加载的战绩中没有该模式", "可继续查询更早的战绩。", false);
	}
	return emptyState("没有符合条件的对局", specialModeEmpty, false);
  }

  function bindMatchSentinel(container, tab) {
    // Header/card binders also call this with a partial subtree. They do not
    // own the full list observer and must leave its scroll listeners alive.
    if (!container?.querySelector("[data-match-sentinel]") && overviewContainer(tab) !== container) return;
    const observerKey = matchObserverKey(tab);
    state[observerKey]?.disconnect();
    state[observerKey] = null;
    const sentinel = container?.querySelector("[data-match-sentinel]");
    const advanced=globalThis.deepLegendsHistoryFilters?.active(tab);
    const hasLoadedMore=advanced && filteredMatches(tab.data?.matches || [],tab).length>Number(tab.filteredVisibleCount || 20);
    const serviceDown=tab.data?.pagination?.historyServiceDown || tab.historyFailureClass==='official' || tab.data?.capabilities?.some?.(c=>c.historyStatus==='official');
	if (!sentinel || (!tab.data?.pagination?.hasMore && !hasLoadedMore) || tab.data?.pagination?.autoPaused && !serviceDown || !("IntersectionObserver" in window)) return;
    const scrollRoot = container.closest(".player-overlay-scroll") || document.getElementById("app-scroll");
    // A restored scroll position, detail collapse, or a short first page can
    // expose the sentinel without the user asking for another 20 API calls.
    let near = false, requested = Boolean(tab.matchAutoloadRequested), pointerScrolling = false;
    let downward = false, guardReported = false;
    const generation = tab.matchSentinelGeneration = Number(tab.matchSentinelGeneration || 0) + 1;
    let lastTop = Number(scrollRoot?.scrollTop || 0);
    const cleanup = () => {
      observer.disconnect();
      scrollRoot?.removeEventListener("wheel", onWheel);
      scrollRoot?.removeEventListener("touchmove", onTouch);
      scrollRoot?.removeEventListener("keydown", onKey);
      scrollRoot?.removeEventListener("pointerdown", onPointer);
      scrollRoot?.removeEventListener("scroll", onScroll);
    };
    const append = () => {
      const guard = !near ? "near" : !requested ? "requested" : tab.appendFramePending ? "frame-pending" : overviewContainer(tab) !== container ? "container" : "";
      if (guard) {
        if (downward && !guardReported) {
          const rect = sentinel.getBoundingClientRect(), rootRect = scrollRoot?.getBoundingClientRect();
          if (!sentinel.hidden && (near || rootRect && rect.top <= rootRect.bottom + 48 && rect.bottom >= rootRect.top)) {
            guardReported = true;
            window.reportFlowDiagnostic?.('match_autoload_client','guard',{bound:true,near,requested,started:false,blocked_reason:guard,generation});
          }
        }
        return;
      }
      tab.matchAutoloadRequested=requested;
      const blocked=serviceDown && !tab.data?.pagination?.localHasMore?'service-down':tab.loading || tab.loadingMore?'loading':tab.quotaRetry?'quota':tab.data?.pagination?.autoPaused?'paused':!tab.data?'not-ready':!tab.data.pagination?.hasMore && !advanced?'no-more':'';
      window.reportFlowDiagnostic?.('match_autoload_client',blocked?'blocked':'requested',{bound:true,near,requested,started:false,blocked_reason:blocked});
      if(blocked)return;
      tab.appendFramePending = true;
      const schedule = window.requestAnimationFrame || ((callback) => setTimeout(callback, 0));
      schedule(() => {
        const delay = Math.max(0, Number(tab.nextAutoAppendAt || 0) - Date.now());
        clearTimeout(tab.appendTimer);
        tab.appendTimer = setTimeout(async () => {
          tab.appendFramePending = false;
          tab.appendTimer = 0;
          if (overviewContainer(tab) === container) {
            if(advanced) {tab.filteredVisibleCount=Number(tab.filteredVisibleCount || 20)+20;reconcileFilteredMatchList(container.querySelector('.match-list'),tab);appendOverviewMatches(tab,[]);}
            else {
              const token=Number(tab.overviewRequestToken || 0),promise=loadOverview(tab,false,true);
              if(Number(tab.overviewRequestToken || 0)!==token){window.reportFlowDiagnostic?.('match_autoload_client','started',{bound:true,near,requested:true,started:true,blocked_reason:''});requested=false;tab.matchAutoloadRequested=false;cleanup();if(state[observerKey]?.check===append)state[observerKey]=null;}
              const started=await promise;if(started && state[observerKey]?.check===append){requested=false;tab.matchAutoloadRequested=false;}
            }
          }
        }, delay);
      });
    };
    const onWheel = event => { if (event.deltaY > 0) { downward = requested = true; append(); } };
    const onTouch = () => { pointerScrolling = true; };
    const onPointer = event => { pointerScrolling = event.target === scrollRoot; };
    const onKey = event => {
      if (["INPUT", "TEXTAREA", "SELECT"].includes(event.target?.tagName)) return;
      if (["ArrowDown", "PageDown", "End", " "].includes(event.key)) { downward = requested = true; append(); }
    };
    const onScroll = () => {
      const top = Number(scrollRoot.scrollTop || 0);
      if (top > lastTop) { downward = requested = true; append(); }
      lastTop = top;
    };
    const observer = new IntersectionObserver(entries => {
      near = entries.some(entry => entry.isIntersecting);
      append();
    }, { root: scrollRoot, rootMargin: "0px 0px 48px 0px" });
    scrollRoot?.addEventListener("wheel", onWheel, {passive:true});
    scrollRoot?.addEventListener("touchmove", onTouch, {passive:true});
    scrollRoot?.addEventListener("pointerdown", onPointer, {passive:true});
    scrollRoot?.addEventListener("keydown", onKey);
    scrollRoot?.addEventListener("scroll", onScroll, {passive:true});
    observer.observe(sentinel);
    state[observerKey] = { disconnect: cleanup,check:append };
    window.reportFlowDiagnostic?.("match_autoload_client","bound",{bound:true,near,requested,started:false});
  }

  function boundMatchDOM(list, tab) {
    if(!list || !window.IntersectionObserver)return;
    list._virtualObserver?.disconnect();list.dataset.matchVirtualizer='';
    const root=list.closest('.player-overlay-scroll') || document.getElementById('app-scroll');
    const pending=new Map();let frame=0;
    const stats={collapsed:0,hydrated:0,height_mismatch_max_px:0};
    const zoom=entry=>Number(entry.currentCSSZoom) || Number(getComputedStyle(document.documentElement).getPropertyValue('--ui-zoom')) || 1;
    const flush=()=>{
      frame=0;
      const measured=[...pending].map(([entry,hydrate])=>({entry,hydrate,height:entry.getBoundingClientRect().height/zoom(entry)}));pending.clear();
      for(const {entry,hydrate,height} of measured){
        if(!entry.isConnected)continue;
        if(hydrate){
          if(!Object.hasOwn(entry.dataset,'virtualMatch'))continue;
          const match=(tab.data?.matches || []).find(m=>String(m.gameId)===entry.dataset.matchId);if(!match)continue;
          const template=document.createElement('template');template.innerHTML=renderMatch(match,tab.data?.player?.playerRef || '',tab);
          entry.replaceChildren(...template.content.firstElementChild.childNodes);delete entry.dataset.virtualMatch;entry.style.minHeight='';
          bindOverviewContent(entry,tab);applyRenderedMetricStyles(entry);prepareImages(entry);stats.hydrated++;
          requestAnimationFrame(()=>{if(entry.isConnected)stats.height_mismatch_max_px=Math.max(stats.height_mismatch_max_px,Math.abs(entry.getBoundingClientRect().height/zoom(entry)-height));});
        }else{
          if(Object.hasOwn(entry.dataset,'virtualMatch') || tab.openMatches?.has(entry.dataset.matchId) || !(height>0))continue;
          entry.style.minHeight=`${height}px`;entry.dataset.virtualMatch='';
          entry.innerHTML='<div class="match-virtual-skeleton gameplay-skeleton" aria-label="正在显示对局"><span></span><span></span><span></span></div>';stats.collapsed++;
        }
      }
      if(Date.now()-Number(list._virtualReportedAt || 0)>=60000){list._virtualReportedAt=Date.now();window.reportFlowDiagnostic?.('match_virtual_client','summary',{...stats,zoom:zoom(list)});}
    };
    const queue=(entry,hydrate)=>{if(pending.get(entry)===true && !hydrate)return;pending.set(entry,hydrate);if(!frame)frame=requestAnimationFrame(flush);};
    const near=new window.IntersectionObserver(events=>{for(const e of events)if(e.isIntersecting)queue(e.target,true);},{root,rootMargin:'2400px 0px'});
    const far=new window.IntersectionObserver(events=>{for(const e of events)if(!e.isIntersecting)queue(e.target,false);},{root,rootMargin:'3200px 0px'});
    list._virtualObserver={disconnect(){near.disconnect();far.disconnect();if(frame)cancelAnimationFrame(frame);pending.clear();}};
    for(const entry of list.querySelectorAll(':scope > .match-entry:not(.match-pending)')){near.observe(entry);far.observe(entry);}
  }

  function appendOverviewMatches(tab, additions, reconciled = false) {
    const container = overviewContainer(tab);
    if (!container) return;
    const list = container.querySelector(".match-list");
    if (list && additions.length && !globalThis.deepLegendsHistoryFilters?.active(tab)) {
      const visible = filteredMatches(additions, tab);
      if (visible.length) {
        if (!list.querySelector(".match-entry")) list.innerHTML = "";
        const staging = document.createElement("div");
        const playerRef = tab.data?.player?.playerRef || tab.playerRef || "";
        staging.innerHTML = visible.map((match) => renderMatch(match, playerRef, tab)).join("");
        bindOverviewContent(staging, tab);
        applyRenderedMetricStyles(staging);
        prepareImages(staging);
        while (staging.firstChild) list.appendChild(staging.firstChild);
      }
    }
    boundMatchDOM(list,tab);
    let sentinel = container.querySelector("[data-match-sentinel]");
    const pagination = tab.data?.pagination || {};
    if (!sentinel && ((tab.data?.matches || []).length || pagination.hasMore)) {
      const column = container.querySelector(".matches-column");
      if (column) {
        sentinel = document.createElement("div");
        sentinel.dataset.matchSentinel = "";
        column.appendChild(sentinel);
      }
    }
    if (sentinel) {
      sentinel.className = `match-pagination${tab.loadingMore ? " is-loading" : ""}`;
      sentinel.setAttribute("aria-live", "polite");
      sentinel.innerHTML = paginationCopyFor(tab);
      sentinel.hidden = matchSentinelShouldHide(tab, Boolean(list?.querySelector(".match-entry:not([hidden])")));
      sentinel.querySelector("[data-load-more]")?.addEventListener("click", () => loadOverview(tab, false, true, true));
    }
    observeMatchTierVisibility(container, tab, container.dataset.matchTierScope || matchTierScope(tab));
    fitMatchTags(container);
    bindMatchSentinel(container, tab);
    if (globalThis.deepLegendsHistoryFilters?.active(tab) && !(pagination.historyServiceDown && !pagination.localHasMore)) {
      const ctx=advancedFilterContext(tab),af=globalThis.deepLegendsHistoryFilters;
      if(list && !reconciled)reconcileFilteredMatchList(list,tab);
      af.refresh(container,tab,ctx);
      const counts=container.querySelector("[data-af-counts]");if(counts)counts.innerHTML=af.counts(tab,ctx);
      if(sentinel){sentinel.hidden=false;sentinel.innerHTML=af.footer(tab,ctx);}
    } else if (tab.advancedMenu?.open) {
      globalThis.deepLegendsHistoryFilters?.refresh(container,tab,advancedFilterContext(tab));
    }
    if (list) list._matchData = new Map((tab.data?.matches || []).map(match => [String(match.gameId), match]));
    container._matchListMatches = tab.data?.matches;
    container._matchListFilter = tab.matchFilter;
    container._matchListViewRevision = Number(tab.matchViewRevision || 0);
  }

  // Preserve committed rows and headers; previews never advance the cursor.
  async function requestOverviewWithReferenceRetry(tab,body,request) {
    try { return await request(body); } catch(error) {
      const identity=tab.riotId || (tab.data?.player?.gameName && tab.data?.player?.tagLine?{gameName:tab.data.player.gameName,tagLine:tab.data.player.tagLine}:null);
      if(error.status!==404 || !/引用|reference/i.test(error.message) || !identity || !body)throw error;
      window.reportFlowDiagnostic?.('overview_reference_expired','retry',{endpoint:'overview',append:Number(JSON.parse(body).begIndex)>0,tab_type:tabGroup(tab),key_change_ms:state.lastRiotKeyChangeAt?Date.now()-state.lastRiotKeyChangeAt:-1});
      const input=JSON.parse(body);delete input.playerRef;
      return await request(JSON.stringify({...input,gameName:identity.gameName,tagLine:identity.tagLine,region:tab.region || tab.data?.player?.region || ''}));
    }
  }

  function finishOverviewLoad(tab) {
    tab.initialPagePending=false;tab.initialPageError="";tab.matchesReceived=true;tab.recentStatsReceived=true;
    if(tab.overviewCardLoad?.ready)for(const key of ['matches','filterbar','highlights'])tab.overviewCardLoad.ready.add(key);
  }

  async function loadOverview(tab, force = false, append = false, manual = false, quiet = false, scanCount = 0, retryDetails = false, background = false) {
    if (tab.closed || state.destroyed || tab.current && !connected()) return false;
    if (append && (tab.data?.pagination?.historyServiceDown || tab.historyFailureClass==='official' || tab.data?.capabilities?.some?.(c=>c.historyStatus==='official')) && !tab.data?.pagination?.localHasMore) {
      window.reportFlowDiagnostic?.('match_autoload_client','blocked',{bound:true,near:true,requested:true,started:false,blocked_reason:'service-down'});
      return false;
    }
    if(manual || append)background=false;
    if(tab.overviewBackground && !background && (tab.loading || tab.loadingMore)){
      state.controllers.get(`overview:${tab.key}`)?.abort();state.controllers.get(`overview-more:${tab.key}`)?.abort();
      state.controllers.get(`overview-search:${tab.key}`)?.abort();
      tab.overviewRequestToken=Number(tab.overviewRequestToken || 0)+1;tab.loading=false;tab.loadingMore=false;
    }
    if(riotTab(tab) && (tab.loading || tab.loadingMore))return false;
    tab.overviewBackground=background;
    if(manual || !tab.data)globalThis.deepLegendsHistoryRecovery?.reset(tab);
    if (tab.quotaRetry && !force) {
      if (!manual) return false;
      if (Date.now() < tab.quotaRetry.retryAt) {
        showToast(`外服查询额度恢复中，约 ${Math.max(0, Math.ceil((tab.quotaRetry.retryAt - Date.now()) / 1000))} 秒后自动继续`);
        if(typeof nodes!=='undefined' && nodes.toast)nodes.toast.innerHTML=`<span data-quota-toast data-quota-recovery>外服查询额度恢复中，约 <b data-quota-seconds>${Math.ceil((tab.quotaRetry.retryAt-Date.now())/1000)}</b> 秒后自动继续</span>`;
        if(typeof document!=='undefined')scheduleQuotaCountdown(tab);
        return false;
      }
      append = tab.quotaRetry.append;
      force = !append;
    }
    if (append && tab.initialPagePending && !tab.quotaRetry) { append = false; force = true; }
	if (!tabReady(tab)) { rerenderTab(tab,"loadOverview"); return false; }
	if (force) {
      if(!quiet){clearTimeout(tab.historyServerRetryTimer);clearTimeout(tab.historyCountdownTimer);tab.historyServerRetryTimer=0;tab.historyRetryAt=0;}
      for (const match of tab.data?.matches || []) {
        if (tab.openMatches?.has(String(match.gameId)) && tab.matchDetailTabs?.get(String(match.gameId)) === "build") {
          void ensureMatchTimeline(match, matchSubject(match, tab.data?.player?.playerRef), tab);
        }
      }
	  tab.reloadAfterAppend = false;
	  state.controllers.get(`overview:${tab.key}`)?.abort();
	  state.controllers.get(`overview-more:${tab.key}`)?.abort();
	  tab.loading = false;

	  tab.loadingMore = false;
	}
	if (append) {
	  if (tab.loading || tab.loadingMore || !tab.data?.pagination?.hasMore) return false;
	  if (tab.data.pagination.autoPaused && !manual) return false;
      const retryDelay = Math.max(0, Number(tab.nextAutoAppendAt || 0) - Date.now());
      if (manual && tab.data.pagination.moreError && retryDelay > 0) {
        showToast(`请求退避中，请在 ${Math.max(1, Math.ceil(retryDelay / 1000))} 秒后重试`);
		return false;
      }
      if (manual) {
        tab.paginationStalls = 0;
        tab.data.pagination = { ...tab.data.pagination, autoPaused: false, pauseReason: "", moreError: "" };
      }
      tab.loadingMore = true;
    } else {
	  if (tab.loading) return false;
	  if (tab.data && !force && tab.seasonRefreshPending) {
		tab.seasonRefreshPending = false;
		return refreshSeasonSummary(tab);
	  }
	  if (tab.data && !force) {
        tab.overviewCardLoad = {startedAt:(typeof performance !== "undefined" ? performance.now() : Date.now()),ready:new Set(),cached:true};
        rerenderTab(tab,"loadOverview");
        void loadOPGGSeasonSummary(tab);
        void loadOverviewCurrentGame(tab);
        void loadMayhemRating(tab,true);
        return false;
      }
      tab.loading = true;
	}
    if (!append) tab.matchesReceived=Boolean(tab.data?.matches?.length);
    if (!append && !tab.data) tab.recentStatsReceived=false;
    if (!append) tab.overviewCardLoad = {startedAt:(typeof performance !== "undefined" ? performance.now() : Date.now()),ready:new Set(),cached:false};
    clearTimeout(tab.quotaRetry?.timer);
    tab.quotaRetry = null;
    const requestToken = Number(tab.overviewRequestToken || 0) + 1;
    cancelOverviewProgress(tab);
    tab.overviewRequestToken = requestToken;
    tab.detailRetryRunning=retryDetails;
    tab.error = "";
    if(!append && riotTab(tab) && typeof document!=="undefined") {tab.historyFailureClass="";tab.historyWaitStarted=Date.now();clearTimeout(tab.historyWaitTimer);tab.historyWaitTimer=setTimeout(()=>{if(tab.loading && tab.overviewRequestToken===requestToken && !tab.closed && !state.destroyed)rerenderTab(tab,"loadOverview");},3000);}
    if (!append) tab.initialPageError = "";
	let appendAdditions = [];
	let loaded = false;
    if (!append && tab.overviewMatchSnapshot && tab.data) {
      tab.data = { ...tab.data, matches: tab.overviewMatchSnapshot.matches, pagination: tab.overviewMatchSnapshot.pagination };
    }
    const pageWasPending = Boolean(tab.initialPagePending);
    const refreshSnapshot = !append && tab.data ? { matches: [...(tab.data.matches || [])], pagination: tab.data.pagination, nextBegIndex:tab.nextBegIndex } : null;
    if (!append) tab.overviewMatchSnapshot = refreshSnapshot;
    let sawPreview = false;
    const baseMatches = [...(tab.data?.matches || [])];
    if (append) showLoadingMoreState(tab);
    else if (!quiet) rerenderTab(tab,"loadOverview");
    const initialKRPage = !append && !tab.data && riotTab(tab) && Number(state.settings.matchCount) > 5;
    const requestCount = retryDetails ? Number(tab.data?.historyRequested || tab.data?.pagination?.count || 10) : scanCount > 0 ? Math.min(20, scanCount) : !append && (!tab.data || tab.initialPagePending) ? Math.min(riotTab(tab) ? 10 : 20, state.settings.matchCount) : state.settings.matchCount;
    if (initialKRPage) tab.initialPagePending = true;
    try {
      let begIndex = append ? Math.max(0, Number(tab.nextBegIndex ?? (Number(tab.data?.pagination?.begIndex || 0) + Number(tab.data?.pagination?.count || 0)))) : 0;
      const verificationTime={};
      const verification = { ...verificationTime, ...(background?{background:true}:{}), ...(tabGroup(tab) === "pro" && !append ? { expectedTier: tab.expectedTier || "" } : {}), ...(!append && tab.dirty ? { expectGameId: String(tab.dirtyGameId || "0") } : {}) };
      const body = tab.current && connected() ? null : JSON.stringify(tab.riotId && !tab.playerRef
		? { ...verification, gameName: tab.riotId.gameName, tagLine: tab.riotId.tagLine, region: tab.region || "", serverId: tab.serverId || "", count: requestCount, begIndex, force, freshHistory:manual, ...(retryDetails?{retryDetails:true}:{}), matchFilter: tab.matchFilter }
		: { ...verification, playerRef: tab.playerRef, serverId: tab.serverId || "", count: requestCount, begIndex, force, freshHistory:manual, ...(retryDetails?{retryDetails:true}:{}), matchFilter: tab.matchFilter });
      const retryHistory = riotTab(tab) || tab.current && connected() && state.status?.clientRegion !== "TENCENT";
      const timeout = retryHistory ? 190_000 : 25_000;
      const onProgress = (partial,kind="progress") => {
        if (tab.overviewRequestToken !== requestToken || tab.closed || state.destroyed) return;
        if (!partial?.player) return;
        if(partial.matches?.length)tab.matchesReceived=true;
        partial={...partial,matches:partial.matches || []};
        if (append || (tab.data && (tab.data.pagination?.filter || "all") !== (tab.matchFilter || "all"))) return;
        tab.lpRenderSource="complete";
        const previous = tab.data;
        const previousByID=new Map((previous?.matches || []).map(match=>[String(match.gameId),match]));
        partial.matches=partial.matches.map(match=>mergeMatchLP(previousByID.get(String(match.gameId)),match));
        const incoming = new Set(partial.matches.map(match => String(match.gameId)));
        const updates = new Map(partial.matches.map(match => [String(match.gameId), match]));
        const matches = partial.matchesDelta ? [...(previous?.matches || []).filter(match=>!incoming.has(String(match.gameId))),...partial.matches].sort((a,b)=>Number(b.createdAt)-Number(a.createdAt)) : kind === "cards" && !partial.matches.length ? [...(previous?.matches || [])] : append
          ? [...baseMatches.filter(match => !incoming.has(String(match.gameId))), ...partial.matches]
          : refreshSnapshot?.matches.length
            ? [...partial.matches,...(previous?.matches || []).filter(match=>!incoming.has(String(match.gameId)))]
            : [...partial.matches];
        sawPreview = true;
        const playerProgress = Object.fromEntries(Object.entries(partial.player).filter(([key, value]) => value != null && !(partial.profilePending && ["summonerLevel", "profileIconId"].includes(key))));
        if (!partial.player.isCurrent && previous?.player?.backgroundPath && previous.masteries?.length && !partial.masteries?.length) {
          for (const key of Object.keys(playerProgress)) if (key.startsWith("background")) delete playerProgress[key];
        }
        tab.data = previous
          ? { ...previous, player: { ...previous.player, ...playerProgress }, matches,
              pagination: {...previous.pagination,...partial.pagination} }
          : { ...partial, player: playerProgress, matches };
        for (const key of ["ranks", "masteries"]) {
          if (Array.isArray(partial[key]) && partial[key].length > 0) tab.data[key] = partial[key];
        }
        if(partial.historyRequested>0){tab.careerStatsSource=kind==='cards'?'cards':'progress';tab.recentStatsReceived=true;for(const key of ['historyRequested','historyLoaded','recentRanked','ability','positions','rankedQueues','activityHours','recentPlayers','overall','championStats'])if(Object.hasOwn(partial,key))tab.data[key]=partial[key];tab.data.pagination={...tab.data.pagination,...partial.pagination};}
        if (partial.capabilities?.length) {
          const readyNames = new Set(partial.capabilities.map(item => item.name));
          tab.data.capabilities = [...(previous?.capabilities || []).filter(item => !readyNames.has(item.name)), ...partial.capabilities];
        }
        tab.initialPagePending = !append;
        const ref = partial.player.playerRef;
        if (ref) { rememberTabPlayerRef(tab, ref); tab.playerRef = ref; }
        tab.label = playerLabel(partial.player);
        if(typeof requestAnimationFrame==='function')scheduleOverviewProgress(tab);else {rerenderTab(tab,"loadOverview");reportDeferredMatchesCard(tab);}
      };
      const requestKey = `${append ? "overview-more" : "overview"}:${tab.key}`;
      const requestOverview=async(requestBody,noReference=false)=>tab.current && connected()
		? await api(`/api/gameplay/overview?count=${requestCount}&begIndex=${begIndex}&force=${force ? 1 : 0}&freshHistory=${manual ? 1 : 0}&matchFilter=${encodeURIComponent(tab.matchFilter || "all")}${verification.expectGameId ? `&expectGameId=${encodeURIComponent(verification.expectGameId)}` : ""}`, {onProgress,headers:noReference?{"X-Overview-No-Reference":"1"}:{}}, requestKey, timeout)
        : await api("/api/gameplay/overview", { method: "POST", body:requestBody, onProgress,headers:noReference?{"X-Overview-No-Reference":"1"}:{} }, requestKey, timeout);
      let payload;
      try{payload=await requestOverviewWithReferenceRetry(tab,body,requestOverview);}catch(error){
        if(!error.referenceFallback)throw error;
        window.reportFlowDiagnostic?.("overview_reference_fallback",error.referenceFallback,{count:1});
        payload=await requestOverview(body,true);
      }
      if (tab.overviewRequestToken !== requestToken) return false;
      tab.lastOverviewPageBytes=typeof TextEncoder==='function'?new TextEncoder().encode(JSON.stringify(payload)).length:encodeURIComponent(JSON.stringify(payload)).replace(/%[A-F0-9]{2}/g,'x').length;
      tab.lpRenderSource="complete";
      if(!payload.pagination?.historyServiceDown && !(Array.isArray(payload.capabilities)?payload.capabilities:[]).some(c=>c.historyStatus==='official') && tab.historyFailureClass==='official')tab.historyFailureClass='';
      tab.matchesReceived=true;tab.recentStatsReceived=true;tab.careerStatsSource="complete";
      if (payload.proMismatch) markProMismatch(tab);
      if (append) {
        const seen = new Set(baseMatches.map((match) => String(match.gameId)));
        const additions = (payload.matches || []).filter((match) => {
          const gameID = String(match.gameId);
          if (seen.has(gameID)) return false;
          seen.add(gameID);
          return true;
        });
        const pagination = normalizedPagination(payload, begIndex);
		const upstreamAdditions = additions.length;
        tab.paginationStalls = upstreamAdditions > 0 || pagination.historyServiceDown ? 0 : Number(tab.paginationStalls || 0) + 1;
		if (pagination.hasMore && tab.paginationStalls >= 2) {
		  pagination.autoPaused = true;
		  pagination.pauseReason = "上游连续无响应，已暂停自动查找";
        }
        tab.nextBegIndex = pagination.nextBegIndex;
        delete pagination.nextBegIndex;
        tab.data = { ...tab.data, matches: [...baseMatches, ...additions], pagination: { ...pagination, moreError: "" } };
        appendAdditions = additions;
        tab.paginationBackoffMs = 0;
        tab.nextAutoAppendAt = Date.now() + AUTO_PAGE_DELAY_MS;
      } else {
        const pagination = normalizedPagination(payload, 0);
        const previousData = tab.data;
        const previousMatches = previousData?.matches || [];
        const oldByID=new Map(previousMatches.map(match=>[String(match.gameId),match]));
        const freshMatches = (payload.matches || []).map(match=>mergeMatchLP(oldByID.get(String(match.gameId)),match));
        const sameFilter = (previousData?.pagination?.filter || "all") === (pagination.filter || "all");
        const historyUnavailable = Array.isArray(payload.capabilities) && payload.capabilities.some(item => item.name === "match-history" && item.state === "failed");
        const preserveLoadedPages = force && sameFilter && previousMatches.length > freshMatches.length;
        const preservePages = preserveLoadedPages || historyUnavailable && sameFilter && previousMatches.length > freshMatches.length;
        let mergedMatches = freshMatches;
        let mergedPagination = pagination;
        if (preservePages) {
          const freshIDs = new Set(freshMatches.map((match) => String(match.gameId)));
          mergedMatches = [...freshMatches, ...previousMatches.filter((match) => !freshIDs.has(String(match.gameId)))].slice(0, MAX_BROWSE_MATCHES);
          mergedPagination = { ...(refreshSnapshot?.pagination || previousData.pagination || {}), historyServiceDown:pagination.historyServiceDown, localHasMore:pagination.localHasMore, moreError: "" };
        }
        tab.nextBegIndex = preservePages ? Number(refreshSnapshot?.nextBegIndex ?? tab.nextBegIndex ?? (Number(mergedPagination.begIndex || 0) + Number(mergedPagination.count || 0))) : pagination.nextBegIndex;
        tab.paginationStalls = 0;
        delete mergedPagination.nextBegIndex;
        const header = { player: payload.player, profilePending: payload.profilePending,
          ranks: payload.ranks, masteries: payload.masteries, capabilities: payload.capabilities };
        for (const key of ["historicalRanks", "rankMilestones"]) {
          if (Object.prototype.hasOwnProperty.call(payload, key)) header[key] = payload[key];
        }
        const unavailable = name => {
          const capability = (Array.isArray(payload.capabilities) ? payload.capabilities : []).find(item => item.name === name);
          return capability ? capability.state !== "available" : Boolean(payload.pagination?.partial);
        };
        if (previousData) {
          for (const [key, name] of [["ranks", "ranked-stats"], ["masteries", "champion-mastery"]]) {
            if (!payload[key]?.length && unavailable(name)) header[key] = previousData[key] || payload[key];
          }
          if ((!payload.player?.backgroundPath && payload.profilePending) ||
              (unavailable("champion-mastery") && (!payload.player?.backgroundPath || previousData.masteries?.length) && !payload.player?.isCurrent)) {
            header.player = { ...payload.player };
            for (const key of ["backgroundSkinId", "backgroundSkinName", "backgroundSource", "backgroundPath", "backgroundPosterPath", "backgroundVideoPath"]) {
              if (previousData.player?.[key] !== undefined) header.player[key] = previousData.player[key];
            }
          }
        }
        tab.data = { ...(previousData || {}), ...payload,
          ...header, matches: mergedMatches, pagination: mergedPagination };
        tab.loadedAt = Date.now();
        tab.paginationBackoffMs = 0;
		tab.nextAutoAppendAt = Date.now() + AUTO_PAGE_DELAY_MS;
	  }
	  loaded = true;finishOverviewLoad(tab);reportDeferredMatchesCard(tab);
      if ((payload.pagination?.filter || tab.matchFilter || "all") === "all") tab.overviewStreak = computeOverviewStreak(tab.data?.matches || [], tab.data?.pagination?.hasMore, payload.player?.playerRef);
      if (!append) {
        tab.expectedGamePresent = payload.expectedGamePresent === true;
        if (payload.latestAllGameId != null) tab.latestAllGameId = String(payload.latestAllGameId);
        else if ((payload.pagination?.filter || tab.matchFilter || "all") === "all") tab.latestAllGameId = String(payload.matches?.[0]?.gameId || "");
      }
      if (!append) tab.initialPagePending = false;
      const resolvedPlayerRef = payload.player?.playerRef || "";
      syncOverviewSupplementRefs(tab);
      rememberTabPlayerRef(tab, resolvedPlayerRef);
      tab.playerRef = resolvedPlayerRef || tab.playerRef;
      tab.region = payload.player?.region || tab.region || "";
      if (tab.region === "kr" && payload.player?.proPlayer) {
        const badge = payload.player.proPlayer;
        const oldGroup = tabGroup(tab);
        applyPlayerTabContext(tab, { group: "pro", proTeam: badge.teamCode, proPlayer: badge.playerName, proSecondary: badge.secondary });
        if (!tab.overlay && state.activeTabs[oldGroup] === tab.key) { state.activeTabs.pro = tab.key; state.activeGroup = "pro"; }
      }
      tab.serverId = payload.player?.serverId || tab.serverId || "";
      tab.serverName = payload.player?.serverName || tab.serverName || "";
      const patches=state.riotMatchPatches?.get(resolvedPlayerRef);if(patches)handleOverviewMatches(patches);
      tab.label = playerLabel(payload.player || {});
      tab.icon = payload.player?.profileIconId || 0;
      tab.error = "";
		if (!append) {
		  state.lastCapabilities = Array.isArray(payload.capabilities) ? payload.capabilities : [];
		  renderCapabilitySettings();
		  void loadMayhemRating(tab, true);
		}
    } catch (error) {
      if (tab.overviewRequestToken !== requestToken) return false;
      if (!retryDetails && !append && sawPreview && refreshSnapshot?.matches.length && tab.data) {
        tab.data = { ...tab.data, matches: refreshSnapshot.matches, pagination: refreshSnapshot.pagination };
      }
      if (error.errorKind === "rate-limited") {
        tab.error = "";
        tab.initialPageError = "";
        const filter = tab.matchFilter, count = state.settings.matchCount;
        const delay=Math.max(1000,Number(error.retryAfter || 5)*1000);
        const retry = { timer: 0, append, retryAt: Date.now() + delay, scheduleDelay:delay };
        tab.quotaRetry = retry;
        tab._quotaFilterContext=()=>advancedFilterContext(tab);
        if(tab.advancedSearch?.running)tab.searchQuotaUntil=retry.retryAt;
        retry.requestToken=requestToken;retry.filter=filter;retry.count=count;
        scheduleQuotaRetry(tab);
        return false;
      }
      if (error.name !== "RequestCancelled") {
        tab.detailRetryNotBefore=Date.now()+Math.max(0,Number(error.retryAfter || 0)*1000);
        if (!append && error.status === 404 && !/引用/.test(error.message)) markProMismatch(tab);
        if (append) {
          if(error.status===503 && !riotTab(tab)){tab.historyFailureClass="official";if(tab.advancedSearch)tab.advancedSearch.stopReason="outage";}
          if (error.status === 503) {
            tab.paginationBackoffMs = Math.min(AUTO_PAGE_MAX_BACKOFF_MS, Math.max(AUTO_PAGE_DELAY_MS * 2, Number(tab.paginationBackoffMs || 0) * 2));
            tab.nextAutoAppendAt = Date.now() + tab.paginationBackoffMs;
          }
          const retryCopy = error.status === 503
            ? `${error.message}；已退避 ${Math.max(1, Math.ceil(tab.paginationBackoffMs / 1000))} 秒`
            : error.message;
          tab.data.pagination = { ...(tab.data.pagination || {}), hasMore: true, autoPaused: true, pauseReason: "", moreError: retryCopy, historyServiceDown:!riotTab(tab) && error.status===503, localHasMore:false };
          showToast(`更多战绩读取失败：${error.message}`);
        }
        else if (tab.initialPagePending && tab.data) tab.initialPageError = error.message || "请求失败";
        else if (!quiet) tab.error = error.message;
        if(riotTab(tab) && (error.kind==="relay-backoff" || ["network","timeout"].includes(error.errorKind) || error.name==="TypeError" || error.name==="AbortError" || /战绩服务暂时不可用|数据读取超时/.test(error.message)))tab.historyFailureClass="relay";
      }
	} finally {
	  const reloadAfterAppend = tab.overviewRequestToken === requestToken && append && tab.reloadAfterAppend;
      if (tab.overviewRequestToken === requestToken) {
        cancelOverviewProgress(tab);
        if (!append) {delete tab.overviewMatchSnapshot;clearTimeout(tab.historyWaitTimer);}
        tab.loading = false;

        tab.loadingMore = false;
        if (tab.pendingHistoricalRanks && !tab.closed && !state.destroyed) {
          const pending = tab.pendingHistoricalRanks; delete tab.pendingHistoricalRanks;
          setTimeout(() => { if (!tab.closed && !state.destroyed) void handleOverviewIncremental(pending); }, 0);
        }
        if (append) appendOverviewMatches(tab, appendAdditions);
        else rerenderTab(tab,"loadOverview");
        if(tab.seasonRecoveryPending && !tab.closed && !state.destroyed){delete tab.seasonRecoveryPending;void loadOverview(tab,true,false,true,true,0,false,!historyRecoveryActive(tab));}
        else if(tab.historyRecoveryPending && historyRecoveryActive(tab))void retryHistoryRegion(tab,'outage-recovery');
        delete tab.detailRetryRunning;
	  }
	  if (reloadAfterAppend) {
		tab.reloadAfterAppend = false;
		void loadOverview(tab, true, false, false, true);
	  }
	}
	if (loaded && !append) {
      void loadOPGGSeasonSummary(tab);
      void loadOverviewCurrentGame(tab);
    }
    if (loaded && !append) tab.initialPagePending = false;
    return loaded;
  }

  function overviewSupplementTarget(tab) {
    const player = tab?.data?.player;
    if (!riotRegion(player?.region || tab?.region) || player?.privateHistory || tab?.closed) return null;
    const playerRef = player?.playerRef || tab?.playerRef || "";
    const gameName = player?.gameName || tab?.riotId?.gameName || "";
    const tagLine = player?.tagLine || tab?.riotId?.tagLine || "";
    if (!playerRef && (!gameName || !tagLine)) return null;
    const key = gameName && tagLine ? `${tab.region || player?.region}:${gameName.toLowerCase()}#${tagLine.toLowerCase()}` : playerRef;
    return { key, body: playerRef ? { playerRef } : { gameName, tagLine, region: tab.region || player?.region } };
  }

  function syncOverviewSupplementRefs(tab) {
    const target = overviewSupplementTarget(tab), ref = tab.data?.player?.playerRef;
    if (!target || !ref) return;
    if (tab.opggSeason?.key === target.key) tab.opggSeason.playerRef = ref;
    if (tab.currentGame?.key === target.key) tab.currentGame.ref = ref;
  }

  // One aggregated OP.GG query, independent of Riot history and pagination.
  // Keep its result on this tab; never refresh the entire overview on completion.
  async function loadOPGGSeasonSummary(tab, force = false) {
    if (tab.current && !connected()) return;
    const target = overviewSupplementTarget(tab);
    if (!target || state.destroyed) return false;
    const ref = target.key;
    if (tab.opggSeasonPending === ref) return false;
    const same = tab.opggSeason?.key === ref;
    const age = Date.now() - Number(tab.opggSeasonAttemptAt || 0);
    if (!force && tab.opggSeasonAttemptRef === ref && age < (same ? 600_000 : 30_000)) return false;
    tab.opggSeasonPending = ref;
    tab.opggSeasonAttemptRef = ref;
    tab.opggSeasonAttemptAt = Date.now();
    try {
      const summary = await api("/api/gameplay/season-summary", { method: "POST", body: JSON.stringify({ ...target.body, force }) }, `opgg-season:${tab.key}`, 17_000);
      if (state.destroyed || overviewSupplementTarget(tab)?.key !== ref) return false;
      if (summary?.source !== "OP.GG" || summary?.queue !== "RANKED" || !summary?.season || !Array.isArray(summary.champions) || !Number.isFinite(summary.overall?.games)) return false;
      tab.opggSeason = { key: ref, playerRef: tab.data?.player?.playerRef || ref, data: summary };
      tab.opggSeasonStale = false;
      return true;
    } catch (error) {
      if (error.name !== "RequestCancelled" && overviewSupplementTarget(tab)?.key === ref) {
        tab.opggSeasonStale = same;
        // Retain an available season aggregate, otherwise show unavailable;
        // never replace season statistics with a recent-match sample.
      }
      return false;
    } finally {
      if (tab.opggSeasonPending === ref) tab.opggSeasonPending = "";
      if (!state.destroyed && overviewSupplementTarget(tab)?.key === ref) updateCareerCards(tab,"opgg_season");
    }
  }

	// R132 P1：整季统计「完成」这一条必须让排位卡立刻拿到补全后的胜率。
	// 页签不在前台时先记下，回到这个页签再强制刷新一次。
	function markSeasonRefreshPending(detail) {
	  const account = String(detail?.account || "");
	  if (!account) return;
	  for (const tab of [...(state.tabs || []), ...(state.overlay || [])]) {
		if (!tab?.data) continue;
		const tabAccount = String(tab.data?.player?.playerRef || tab.playerRef || "");
		if (tabAccount !== account) continue;
        const progress=tab.data.seasonStatsProgress || {};
        if(detail.complete===true || detail.rankedComplete===true || detail.paused!==progress.paused) {
          tab.seasonRefreshPending = true;
          if(state.section!=="overview" || activeTab(overviewGroupForSection())!==tab)void refreshSeasonSummary(tab,detail.snapshot || null);
        }
	  }
	}

  async function refreshSeasonSummary(tab, snapshot = null) {
    const playerRef=tab.data?.player?.playerRef || tab.playerRef;
    if(!playerRef)return false;
    try {
      const data=snapshot || await api(`/api/gameplay/season-summary?playerRef=${encodeURIComponent(playerRef)}`,{},`season-summary:${tab.key}`,5000);
      if(tab.closed || (tab.data?.player?.playerRef || tab.playerRef)!==playerRef)return false;
      tab.data={...tab.data,seasonChampionStats:data.seasonChampionStats,seasonOverall:data.seasonOverall,seasonStatsProgress:data.seasonStatsProgress,seasonQueueStats:data.seasonQueueStats};
      tab.data.rankedQueues={...(tab.data.rankedQueues || {})};
      for(const [key,queue] of Object.entries(data.rankedQueues || {}))tab.data.rankedQueues[key]={...tab.data.rankedQueues[key],seasonGames:queue.seasonGames};
      if(data.seasonStatsProgress?.rankedComplete) {
        tab.seasonRanksFilled ||= new Set();
        tab.data.ranks=(tab.data.ranks || []).map(rank=>{
          const key=rank.queueType==="RANKED_SOLO_5x5" ? "420" : rank.queueType==="RANKED_FLEX_SR" ? "440" : "";
          const queue=data.seasonQueueStats?.[key];
          if(!queue || !(Number(rank.winRate)<0 || tab.seasonRanksFilled.has(key)))return rank;
          tab.seasonRanksFilled.add(key);return {...rank,wins:queue.wins,losses:queue.losses,winRate:queue.winRate};
        });
      }
      const container=overviewContainer(tab),root=container?._overviewSubpageStash || container;
      updateCareerCards(tab,"season_summary");
      if(container) {delete container._careerSignature;reportOverviewCardReady(container,tab);}
      return true;
    }catch(error){if(error.name!=="RequestCancelled")tab.seasonRefreshPending=true;return false;}
  }

  function mergeMatchLP(previous, next) {
    if(previous?.lpDelta!=null && next.lpDelta==null)return {...next,lpDelta:previous.lpDelta,lpPending:false};
    return next;
  }
  function mergeCurrentHistoryHead(matches, head) {
    const previous=new Map(matches.map(match=>[String(match.gameId),match]));
    const seen=new Set(head.map(match=>String(match.gameId)));
    return [...head.map(match=>mergeMatchLP(previous.get(String(match.gameId)),match)),...matches.filter(match=>!seen.has(String(match.gameId)))];
  }
  function matchContentVersion(match) {
    const stable=value=>Array.isArray(value)?value.map(stable):value && typeof value==='object'?Object.fromEntries(Object.keys(value).sort().filter(key=>!['revision','lpDelta','lpPending'].includes(key)).map(key=>[key,stable(value[key])])):value;
    return JSON.stringify(stable(match))+":"+(window.deepLegendsPlayerTags?.signature(match) || "");
  }
  function renderLPChip(match) {
    const ranked=[420,440].includes(Number(match.queueId)),resultKind=match.result,lpValue=Number(match.lpDelta);
    const lpChip = ranked && resultKind !== "remake" && Number.isFinite(lpValue) && match.lpDelta != null
      ? `<b class="lp-delta ${lpValue >= 0 ? "is-gain" : "is-drop"}" data-tooltip="这场排位的胜点变化\n由助手在对局结算时记录" data-tooltip-size="compact">${lpValue >= 0 ? "+" : "−"}${Math.abs(lpValue)} LP</b>`
      : "";
    return lpChip || (ranked && resultKind!=="remake" && match.lpPending?'<b class="lp-delta is-pending skeleton-block" aria-hidden="true"></b>':'');
  }
  function patchLPChip(entry, match) {
    const previous=entry.querySelector('.lp-delta'),html=renderLPChip(match);
    if(previous?.outerHTML===html)return;
    if(!html){previous?.remove();return;}
    if(previous)previous.outerHTML=html;else entry.querySelector('.result-word')?.insertAdjacentHTML('beforeend',html);
  }
  function handleLPRecorded(detail) {
    if(!Array.isArray(detail?.matches))return;
    const patches=new Map(detail.matches.map(match=>[String(match.gameId),match]));
    for(const tab of [...(state.tabs || []),...(state.overlay || [])]) {
      if(!tab.data || !(tab.current || tab.data.player?.isCurrent))continue;
      const patch=matches=>matches.map(match=>patches.has(String(match.gameId))?mergeMatchLP(match,{...match,...patches.get(String(match.gameId))}):match);
      tab.data.matches=patch(tab.data.matches || []);
      for(const view of tab.matchViews?.values() || []) {view.matches=patch(view.matches || []);if(view.list)for(const entry of view.list.querySelectorAll('.match-entry')){const match=view.matches.find(m=>String(m.gameId)===entry.dataset.matchId);if(match)patchLPChip(entry,match);}}
      tab.lpRenderSource='lp-recorded';
      const container=overviewContainer(tab),root=container?._overviewSubpageStash || container,list=root?.querySelector('.match-list');
      if(list)reconcileFilteredMatchList(list,tab);
    }
  }

  function syncOverviewStreak(tab) {
    if(typeof document==="undefined")return;
    const container=overviewContainer(tab),root=container?._overviewSubpageStash || container;
    const meta=root?.querySelector(".summoner-name-meta");if(!meta)return;
    const previous=meta.querySelector(".summoner-streak"),template=document.createElement("template");
    template.innerHTML=renderOverviewStreak(tab.overviewStreak);const next=template.content.firstElementChild;
    if((previous?.outerHTML || "")===(next?.outerHTML || ""))return;
    if(!next)previous?.remove();else if(previous)previous.replaceWith(next);else meta.appendChild(next);
    // The next header render must use the new result; the existing header,
    // avatar and profile controls remain mounted during this SSE update.
    if(container)delete container._stripHTML;
  }

  function handleOverviewMatches(detail) {
    if(!Array.isArray(detail?.matches))return;
    state.riotMatchPatches ||= new Map();
    const generation=Number(detail.historyGeneration || 0),previous=state.riotMatchPatches.get(detail.account);
    if(generation && generation<Number(previous?.historyGeneration || 0))return;
    state.riotMatchPatches.set(detail.account,detail);
    while(state.riotMatchPatches.size>32)state.riotMatchPatches.delete(state.riotMatchPatches.keys().next().value);
    for(const tab of [...(state.tabs || []),...(state.overlay || [])]) {
      if((tab.data?.player?.playerRef || tab.playerRef)!==detail.account || !tab.data)continue;
      if(detail.replace) {
        if(generation && generation<Number(tab.data.historyGeneration || 0))continue;
        if((tab.matchFilter || "all")!=="all")continue;
        tab.lpRenderSource="overview-matches";
        const previous=tab.data.matches || [],matches=mergeCurrentHistoryHead(previous,detail.matches);
        const hasTail=previous.length>detail.matches.length;
        tab.data={...tab.data,historyGeneration:generation,matches,pagination:{...(detail.pagination || tab.data.pagination),count:matches.length,hasMore:hasTail ? Boolean(tab.data.pagination?.hasMore) : Boolean(detail.pagination?.hasMore)}};
        tab.overviewStreak=computeOverviewStreak(matches,tab.data.pagination.hasMore,tab.data.player?.playerRef);
        syncOverviewStreak(tab);
        renderFilteredMatchView(tab);continue;
      }
      tab.lpRenderSource="patch";
      const changed=new Map(detail.matches.map(match=>[String(match.gameId),match]));
      tab.data.matches=tab.data.matches.map(match=>changed.has(String(match.gameId))?mergeMatchLP(match,changed.get(String(match.gameId))):match);
      renderFilteredMatchView(tab);
      if((tab.matchFilter || "all")==="all") {tab.overviewStreak=computeOverviewStreak(tab.data.matches,tab.data.pagination?.hasMore,tab.data.player?.playerRef);syncOverviewStreak(tab);}
    }
  }
  function handleHistoryServiceRecovered(detail) {
    if(state.section==="live" && state.live?.historyServiceDown)void loadLive(true,"outage-recovery");
    for(const tab of [...state.tabs,...state.overlay]){
      if(riotTab(tab) || detail?.serverId && tabServerID(tab)!==detail.serverId)continue;
      if(tab.data?.seasonStatsProgress?.paused) {
        if(tab.loading){tab.seasonRecoveryPending=true;continue;}
        // A forced head refresh also restarts persisted backfill, including hidden tabs.
        void loadOverview(tab,true,false,true,true,0,false,!historyRecoveryActive(tab));
        continue;
      }
      if(!historyServiceState(tab))continue;
      tab.historyRecoveryPending=true;
      if(historyRecoveryActive(tab) && !tab.loading)retryHistoryRegion(tab,'outage-recovery');
    }
  }
  async function retryHistoryRegion(tab,reason='manual') {
    if(tab.historyRetryPending)return false;
    if(reason==='outage-recovery' && tab.data?.matches?.length && tab.data.pagination?.hasMore){
      tab.historyRecoveryPending=false;
      delete tab.historyOutageReported;
      if(tab.historyFailureClass==='official')tab.historyFailureClass='';
      tab.data.capabilities=(tab.data.capabilities || []).map(c=>c.historyStatus==='official'?{...c,historyStatus:'',state:'available'}:c);
      tab.data.pagination={...tab.data.pagination,historyServiceDown:false,localHasMore:false,autoPaused:false,moreError:''};
      const container=overviewContainer(tab),root=container?.closest('.player-overlay-scroll') || document.getElementById('app-scroll');
      const bottom=root && root.scrollHeight-root.clientHeight-root.scrollTop<=64;
      window.reportFlowDiagnostic?.('history_outage_ui','recovered',{source:'circuit'});
      for(const notice of container?.querySelectorAll('.service-outage-compact') || [])notice.remove();
      appendOverviewMatches(tab,[]);
      return bottom?loadOverview(tab,false,true,false,true):true;
    }
    const started=Date.now(),manual=reason==='manual';
    const wasOfficial=tab.data?.pagination?.historyServiceDown || historyServiceState(tab)?.kind==='official';
    const priorCursor=tab.nextBegIndex;
    const recoveryRoot=wasOfficial && (overviewContainer(tab)?.closest('.player-overlay-scroll') || document.getElementById('app-scroll'));
    const wasBottom=recoveryRoot && recoveryRoot.scrollHeight-recoveryRoot.clientHeight-recoveryRoot.scrollTop<=64;
    tab.historyRetryPending=manual;
    const feedback=()=>{const container=overviewContainer(tab);for(const button of container?.querySelectorAll('[data-gameplay-retry]') || []){button.disabled=Boolean(tab.historyRetryPending);button.innerHTML=tab.historyRetryPending?'<span class="mini-loading" aria-hidden="true"></span>正在重试':tab.historyRetryStillDown?'仍未恢复':'立即重试';}};
    feedback();
    tab.historyRecoveryPending=false;
    if(reason==='outage-recovery')delete tab.historyOutageReported;
    if(reason==='outage-recovery')window.reportFlowDiagnostic?.('history_outage_ui','recovered',{source:'circuit'});
    window.reportFlowDiagnostic?.('overview_reload_reason',reason,{matches_before:tab.data?.matches?.length || 0,matches_after:tab.data?.matches?.length || 0});
    const result=await loadOverview(tab,true,false,manual,true,0,true);
    if(manual){await new Promise(resolve=>setTimeout(resolve,Math.max(0,600-(Date.now()-started))));tab.historyRetryPending=false;tab.historyRetryStillDown=Boolean(historyServiceState(tab));feedback();
      window.reportFlowDiagnostic?.('history_outage_retry',tab.historyRetryStillDown?'still_down':'recovered',{result:tab.historyRetryStillDown?'still_down':'recovered',duration_ms:Date.now()-started});
      if(wasOfficial && !tab.historyRetryStillDown && tab.data?.pagination){
        tab.historyRecoveryPending=false;
        tab.data.pagination={...tab.data.pagination,historyServiceDown:false,localHasMore:false,autoPaused:false,moreError:''};
        if(Number(priorCursor)>0)tab.nextBegIndex=priorCursor;
        appendOverviewMatches(tab,[]);
        if(wasBottom && tab.data.pagination.hasMore && historyRecoveryActive(tab))void loadOverview(tab,false,true,false,true);
      }
      if(tab.historyRetryStillDown){clearTimeout(tab.historyRetryFeedbackTimer);tab.historyRetryFeedbackTimer=setTimeout(()=>{tab.historyRetryStillDown=false;feedback();},2500);}
    }
    return result;
  }

  async function handleRelayRecovered() {
    for(const tab of [...(state.tabs || []),...(state.overlay || [])]){
      if(tab.mayhemRating?.status==='error'){tab.mayhemRating={...tab.mayhemRating,status:'idle',at:0};tab.mayhemRatingManual=true;}
      if(riotTab(tab) && !tab.loading)void loadOverview(tab,true,false,false,true,0,false,!historyRecoveryActive(tab));
    }
  }
  function serviceOutageIcon() {
    return '<svg class="service-outage-icon" viewBox="0 0 64 64" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 40h8"/><rect x="12" y="30" width="14" height="20" rx="3"/><path d="M26 35h5M26 45h5"/><rect x="38" y="30" width="14" height="20" rx="3"/><path d="M42 35h2M42 45h2"/><path d="M52 40h8"/><path d="M34.5 20v6M29 22l2.5 4.5M40 22l-2.5 4.5"/></svg>';
  }
  function historyServiceState(tab) {
    const capability = (tab.data?.capabilities || []).find(item => item.historyStatus === "official");
    if (capability || tab.data?.pagination?.historyServiceDown || tab.historyFailureClass==="official") return {kind:"official"};
    if (riotTab(tab) && (tab.historyFailureClass === "relay" || [tab.error,tab.initialPageError,...(tab.data?.capabilities || []).map(item=>item.detail)].some(message=>String(message || "").includes("战绩服务暂时不可用")))) return {kind:"relay",seconds:Math.max(0,Math.ceil(((tab.historyRetryAt || 0)-Date.now())/1000))};
    return null;
  }
  function renderHistoryServiceStatus(tab,compact=false) {
    const status=historyServiceState(tab);if(!status)return "";
    if(!tab.historyOutageReported){tab.historyOutageReported=true;if(typeof window!=='undefined')window.reportFlowDiagnostic?.('history_outage_ui','shown',{source:tab.data?.capabilities?.some(c=>c.historyStatus==='official' && !c.retryAfter)?'circuit':'fresh-503',durationMs:Math.max(0,Date.now()-Number(tab.historyWaitStarted || Date.now()))});}
    const official=status.kind==="official",title=official?"战绩服务暂时中断":"外服战绩连接较慢";
    const copy=official?"英雄联盟官方战绩服务器暂时无法返回对局数据，游戏客户端里的战绩也会受影响。恢复后会自动重新读取。":"暂时没有读取到对局，恢复后会自动补齐。";
    const waiting=Boolean(tab.historyRetryPending || tab.loading || tab.loadingMore),retryCopy=tab.historyRetryPending?'<span class="mini-loading" aria-hidden="true"></span>正在重试':tab.historyRetryStillDown?"仍未恢复":waiting?"读取中":null,disabled=waiting?' disabled aria-busy="true"':'';
    if(compact)return `<div class="service-outage-compact" role="status">${serviceOutageIcon()}<span>${official?"官方":""}${title}${official?"，以下为已保存的对局":""}</span><button class="text-button" type="button" data-gameplay-retry${disabled}>${retryCopy || (official?"重试":"重试补齐")}</button></div>`;
    return `<div class="service-outage" role="status"><div class="service-outage-disc">${serviceOutageIcon()}</div><strong>${title}</strong><p>${copy}</p><button class="text-button" type="button" data-gameplay-retry${disabled}>${retryCopy || "立即重试"}</button></div>`;
  }
  function historyStatsPending(data,tab) {
    // 本服原始战绩可能先于聚合到达；保留此前 complete 已确认的统计。
    if(tab?.loading && !tab.recentStatsReceived && !(Number(data?.historyRequested)>0))return true;
    const partial=Boolean(data?.pagination?.partial || tab?.initialPagePending || tab?.initialPageError || tab?.loading || data?.capabilities?.some(item=>item.name==="match-history" && item.state==="failed"));
    const k=Number(data?.historyLoaded ?? data?.matches?.length ?? 0),n=Number(data?.historyRequested || data?.pagination?.count || k);
    return partial && !(k>=6 && k>=Math.ceil(.6*n));
  }

  function championStatsPending(items,overall,progress,tab) {
    if(progress?.foreign)return false;
    if(Number(overall?.games)>0 || (items || []).some(item=>Number(item.games)>0))return false;
    if(historyServiceState(tab))return true;
    if(progress && !progress.unavailable)return Boolean(progress.collecting && !progress.complete && !Number(progress.scanned));
    return Boolean(tab?.loading && !tab.recentStatsReceived && !(Number(tab.data?.historyRequested)>0));
  }

  function historyRecoveryActive(tab) {
    return !tab.closed && !state.destroyed && !document.hidden && (tab.overlay ? state.overlay.at(-1)===tab && !nodes.playerOverlay.hidden : state.section==='overview' && !state.overlay.length) && Boolean(overviewContainer(tab));
  }
  function cancelHistoryRecovery(tab) {
    cancelOverviewProgress(tab);
    globalThis.deepLegendsHistoryRecovery?.cancel(tab);
    clearTimeout(tab?.quotaCountdownTimer);if(tab)tab.quotaCountdownTimer=0;
    clearTimeout(tab?.searchCountdownTimer);
    if(tab && riotTab(tab) && tab.quotaRetry){clearTimeout(tab.quotaRetry.timer);tab.quotaRetry.timer=0;}
  }
  function scheduleQuotaCountdown(tab) {
    clearTimeout(tab.quotaCountdownTimer);tab.quotaCountdownTimer=0;
    if(!tab.quotaRetry || !historyRecoveryActive(tab))return;
    const seconds=Math.max(0,Math.ceil((tab.quotaRetry.retryAt-Date.now())/1000));
    const container=typeof overviewContainer==='function'?overviewContainer(tab):null;if(!container)return;
    for(const node of container?.querySelectorAll('[data-quota-seconds]') || [])node.textContent=String(seconds);
    for(const node of container?.querySelectorAll('[data-quota-recovery]') || [])node.hidden=seconds===0;
    const toast=typeof document!=='undefined'?document.querySelector('[data-quota-toast]'):null;
    if(toast){toast.hidden=seconds===0;const number=toast.querySelector('[data-quota-seconds]');if(number)number.textContent=String(seconds);}
    appendOverviewMatches(tab,[]);
    globalThis.deepLegendsHistoryFilters?.refresh(container,tab,advancedFilterContext(tab));
    if(seconds>0)tab.quotaCountdownTimer=setTimeout(()=>scheduleQuotaCountdown(tab),1000);
  }

  function scheduleQuotaRetry(tab) {
    const retry=tab.quotaRetry;if(!retry || retry.timer || riotTab(tab) && !historyRecoveryActive(tab))return;
    globalThis.deepLegendsSections?.load('history-recovery').catch(()=>{});
    scheduleQuotaCountdown(tab);
    const delay=Math.max(0,retry.retryAt-Date.now());delete retry.scheduleDelay;
    retry.timer=setTimeout(()=>{
      retry.timer=0;
      void globalThis.deepLegendsHistoryRecovery?.resumeQuota(tab,retry,{state,active:historyRecoveryActive,riot:riotTab,load:loadOverview,filter:(...args)=>autoLoadMatchFilter(...args)});
    },delay);
  }
  function scheduleHistoryServerRetry(tab) {
    if(typeof riotTab==='function' && riotTab(tab)){
      if(tab.loading)return;
      if(tab.quotaRetry){scheduleQuotaRetry(tab);return;}
      const schedule=()=>{if(tab.loading || tab.quotaRetry || !historyRecoveryActive(tab))return;globalThis.deepLegendsHistoryRecovery?.schedule(tab,{...(tab.initialPageError?{delays:[30000,60000,120000,300000]}:{}),pending:()=>!tab.quotaRetry && (historyStatsPending(tab.data,tab) || Boolean(tab.data?.pagination?.partial || tab.initialPageError || tab.historyFailureClass)),active:()=>historyRecoveryActive(tab),notBefore:()=>Math.max(tab.quotaRetry?.retryAt || 0,tab.detailRetryNotBefore || 0),quotaBlocked:()=>Boolean(tab.quotaRetry),abort:()=>state.controllers.get(`overview:${tab.key}`)?.abort(),retry:()=>loadOverview(tab,true,false,false,true,0,true),report:round=>window.reportFlowDiagnostic?.('history_detail_recovery','round',{round})});};
      if(globalThis.deepLegendsHistoryRecovery)schedule();else if(!tab.detailRecoveryLoading && typeof document!=='undefined'){tab.detailRecoveryLoading=true;globalThis.deepLegendsSections?.load('history-recovery').then(schedule).catch(()=>{}).finally(()=>{tab.detailRecoveryLoading=false;});}
      return;
    }
    clearTimeout(tab.historyServerRetryTimer);clearTimeout(tab.historyCountdownTimer);
    tab.historyRetryAt=0;tab.historyServerRetryTimer=0;
    if(historyServiceState(tab))return;
    if(tab.data?.matches?.length && (tab.initialPageError || tab.data.pagination?.partial)){
      const schedule=()=>globalThis.deepLegendsHistoryRecovery?.schedule(tab,{delays:[30000,60000,120000,300000],pending:()=>Boolean(tab.initialPageError || tab.data?.pagination?.partial),active:()=>historyRecoveryActive(tab),notBefore:()=>tab.detailRetryNotBefore || 0,abort:()=>state.controllers.get(`overview:${tab.key}`)?.abort(),retry:()=>loadOverview(tab,true,false,false,true,0,true)});
      if(globalThis.deepLegendsHistoryRecovery)schedule();else globalThis.deepLegendsSections?.load('history-recovery').then(schedule).catch(()=>{});
    }
  }
  function relaySlowState(tab) {
    return riotTab(tab) && [tab.error,tab.initialPageError,...(tab.data?.capabilities || []).map(item=>item.detail)].some(message=>String(message || "").includes("战绩服务暂时不可用"));
  }
  function renderRelaySlowNotice(tab) {
    return relaySlowState(tab) ? '<div class="notice is-warning relay-slow-notice" role="status"><p>外服战绩服务较慢</p></div>' : "";
  }

	async function handleSeasonProgress(detail, now = Date.now()) {
	  if (!detail || detail.type !== "season-progress") return false;
	  markSeasonRefreshPending(detail);
      if (state.section !== "overview") return false;
	  const group = overviewGroupForSection();
	  const tab = activeTab(group);
	  const progress = tab?.data?.seasonStatsProgress;
	  if (!tab?.data || !progress?.season || String(detail.season || "") !== String(progress.season)) { markSeasonRefreshPending(detail); return false; }
	  const account = String(detail.account || "");
	  const tabAccount = String(tab.data?.player?.playerRef || tab.playerRef || "");
	  if (account && tabAccount && account !== tabAccount) { markSeasonRefreshPending(detail); return false; }
	  const completed = detail.complete === true && progress.complete === false || detail.rankedComplete === true && !progress.rankedComplete || typeof detail.paused === "boolean" && detail.paused !== Boolean(progress.paused);
	  const advanced = Number(detail.scanned || 0) > Number(progress.scanned || 0);
	  if (!completed && !advanced) return false;
	  const refreshKey = account || tabAccount || tab.key;
	  const previous = state.seasonProgressRefreshes.get(refreshKey) || { lastAt: 0, timer: 0 };
	  const remaining = 3_000 - (now - previous.lastAt);
	  // 只对「又扫了一批」的中间进度节流；「完成」不等节流窗口，否则回补在
	  // 上一次刷新后几秒内完成时，排位卡要多挂最长 10 秒的「正在统计中」。
	  if (remaining > 0 && !completed) {
		previous.detail = detail;
		if (!previous.timer) {
		  previous.timer = setTimeout(() => {
			previous.timer = 0;
			void handleSeasonProgress(previous.detail || detail);
		  }, remaining);
		}
		state.seasonProgressRefreshes.set(refreshKey, previous);
		return false;
	  }
	  if (previous.timer) { clearTimeout(previous.timer); previous.timer = 0; }
	  tab.seasonRefreshPending = false;
	  previous.lastAt = now;
	  state.seasonProgressRefreshes.set(refreshKey, previous);
	  const tabKey = tab.key;
	  const scrollRoot = document.getElementById("app-scroll");
	  const scrollTop = Number(scrollRoot?.scrollTop || 0);
	  if(detail.snapshot)await refreshSeasonSummary(tab,detail.snapshot);else await refreshSeasonSummary(tab);
	  if (state.section !== overviewSectionForGroup(group) || activeTab(group)?.key !== tabKey) return false;
	  requestAnimationFrame(() => {
		if (scrollRoot?.isConnected) scrollRoot.scrollTop = scrollTop;
	  });
	  return true;
	}

  async function handleOverviewIncremental(detail) {
    if (detail?.type === "overview-incremental") {
      if (!connected() || state.destroyed || !detail.account) return false;
      const tab = state.tabs.find(item => !item.closed && item.data && String(item.data.player?.playerRef || item.playerRef || "") === String(detail.account));
      if (!tab || !detail.rankedQueues || !Number(detail.recentRanked?.games)) return false;
      if (String(detail.headGameId || "0") !== String(tab.data.matches?.[0]?.gameId || "0")) return false;
      if (tab.loading || tab.loadingMore) { tab.pendingHistoricalRanks = detail; return false; }
      const queues = {...tab.data.rankedQueues};
      for (const key of ["420", "440"]) {
        if (detail.rankedQueues[key] && Number(detail.rankedQueues[key].recentRanked?.games || 0) >= Number(queues[key]?.recentRanked?.games || 0)) queues[key] = {...queues[key], ...detail.rankedQueues[key], seasonGames: queues[key]?.seasonGames};
      }
      tab.data = {...tab.data, rankedQueues: queues, recentRanked: Number(detail.recentRanked.games) >= Number(tab.data.recentRanked?.games || 0) ? detail.recentRanked : tab.data.recentRanked};
      updateCareerCards(tab,"overview_incremental");
      return true;
    }

    if (detail?.type !== "historical-ranks" || !Array.isArray(detail.historicalRanks) || !detail.account) return false;
    const tab = state.tabs.find(item => !item.closed && String(item.data?.player?.playerRef || item.playerRef || item.identity?.playerRef || "") === String(detail.account));
    if (!tab) return false;
    if (!tab.data || tab.loading || tab.loadingMore) { tab.pendingHistoricalRanks = detail; return false; }
    if (JSON.stringify(tab.data.historicalRanks) === JSON.stringify(detail.historicalRanks)) return false;
    tab.data = { ...tab.data, historicalRanks: detail.historicalRanks };
    updateCareerCards(tab,"overview_incremental");
    return true;
  }

  function renderPlayerTabs() {
    const groups = visiblePlayerGroups();
    if (!groups.includes(state.activeGroup)) {
      state.activeGroup = groups[0] || "players";
      if (activeTab()) activeTab().restoreScrollPending = true;
    }
    const group = overviewGroupForSection();
    if (nodes.playerGroups) {
      const button = playerGroupButton();
      const menu = playerGroupMenu();
      const count = playerGroupCount(group);
      if (button) {
        const groupLabel = activeTab()?.regionResolved === false ? "正在识别服务器…" : PLAYER_GROUPS[group];
        button.dataset.playerGroup = group;
        button.setAttribute("aria-label", `当前玩家分组：${groupLabel}，${count} 个玩家。展开切换分组`);
        button.setAttribute("aria-expanded", String(playerGroupMenuOpen));
        button.innerHTML = `<span class="player-group-dot is-${group}" aria-hidden="true"></span><span class="player-group-label">${groupLabel}</span><span class="player-group-count">${count}</span>`;
      }
      if (menu) {
        const focusedGroup = menu.contains(document.activeElement) ? document.activeElement.dataset.playerGroup : "";
        menu.innerHTML = Object.keys(PLAYER_GROUPS).map(key => {
          const itemCount = playerGroupCount(key);
          const disabled = false;
          const tip = disabled ? PLAYER_GROUP_EMPTY_TIPS[key] || "当前没有可用玩家" : "";
          return `<button type="button" role="menuitemradio" aria-checked="${key === group}" aria-disabled="${disabled}" ${disabled ? "disabled" : ""} tabindex="-1" data-player-group="${key}"${tip ? ` data-tooltip="${escapeHTML(tip)}"` : ""}><span class="player-group-dot is-${key}" aria-hidden="true"></span><span>${PLAYER_GROUPS[key]}</span><small>${itemCount}</small></button>`;
        }).join("");
        menu.hidden = !playerGroupMenuOpen;
        if (focusedGroup && playerGroupMenuOpen) {
          clearTimeout(playerGroupCloseTimer);
          focusPlayerGroupItem(focusedGroup);
        }
      }
      nodes.playerGroups.classList.toggle("is-open", playerGroupMenuOpen);
    }
    renderPlayerTabWorkspace(group);
    const back = document.getElementById("pro-players-return");
    if (back) back.hidden = group !== "pro";
    window.dispatchEvent(new CustomEvent("deep-legends:active-player-tab", { detail: { group, key: activeTab()?.key || "", proCount: state.tabs.filter(tab => tabGroup(tab) === "pro").length } }));
  }

  function renderPlayerTabWorkspace(group) {
    const workspace = overviewWorkspace(group);
    if (!workspace.tabs) return;
    const indexedTabs = visiblePlayerTabs(group).map((tab) => ({ tab, index: state.tabs.indexOf(tab) }));
    const mixedRegions=group==="kr" && new Set(indexedTabs.map(({tab})=>tab.region || tab.data?.player?.region)).size>1;
    const markup = indexedTabs.length ? indexedTabs.map(({ tab, index }) => {
      const selected = tab.key === state.activeTabs[group];
      const masked = state.settings.maskNames;
      const label = masked ? privacyName(tab.data?.player || tab.identity || tab.riotId || {playerRef:tab.key},{index,context:"identity"}) : tab.label;
      const icon = !masked && tab.icon ? assetIcon(assetPath("profile", tab.icon), "", "", false) : '<span class="player-tab-placeholder" aria-hidden="true">◉</span>';
      const regionBadge = mixedRegions ? `<small class="player-tab-region">${escapeHTML(tabServerLabel(tab))}</small>` : "";
      const selfBadge = tab.current ? '<span class="player-tab-self" data-tooltip="当前登录的召唤师" data-tooltip-size="compact" aria-hidden="true">★</span>' : "";
      return `<span class="player-tab-wrap${selected ? " is-active" : ""}${tab.current ? " is-self" : ""}" data-player-tab-wrap="${escapeHTML(tab.key)}" draggable="${!tab.current}"><button class="player-tab" type="button" role="tab" aria-selected="${selected}" tabindex="${selected ? 0 : -1}" data-player-tab="${escapeHTML(tab.key)}">${selfBadge}${icon}<span class="player-tab-copy"><span class="player-tab-name" data-tooltip="${escapeHTML(label+' · '+(typeof tabServerLabel==='function'?tabServerLabel(tab):tab.region || '国服'))}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(label)}</span>${regionBadge}</span>${tab.loading ? '<span class="mini-loading" aria-label="正在读取"></span>' : ""}</button>${tab.current ? "" : `<button class="player-tab-close" type="button" aria-label="关闭 ${escapeHTML(label)}" data-close-player="${escapeHTML(tab.key)}">×</button>`}</span>`;
    }).join("") : `<span class="player-tab-skeleton">${group === "pro" ? "请选择职业选手账号…" : "暂无可用玩家页签"}</span>`;
    const template = document.createElement("template");
    template.innerHTML = markup;
    const existing = new Map([...workspace.tabs.querySelectorAll("[data-player-tab-wrap]")].map(node => [node.dataset.playerTabWrap, node]));
    const wanted = [...template.content.children];
    for (let index = 0; index < wanted.length; index++) {
      const fresh = wanted[index], key = fresh.dataset.playerTabWrap;
      let node = existing.get(key);
      const signature = fresh.innerHTML.replace(/aria-selected="(true|false)"/g, "").replace(/tabindex="-?\d+"/g, "");
      if (node && node._tabMarkup === signature) {
        node.className = fresh.className;
        for (const name of ["aria-selected", "tabindex"]) node.querySelector("[data-player-tab]").setAttribute(name, fresh.querySelector("[data-player-tab]").getAttribute(name));
      } else {
        const oldImage = node?.querySelector("img"), newImage = fresh.querySelector("img");
        if (oldImage && newImage && oldImage.dataset.queuedSrc === newImage.dataset.queuedSrc) newImage.replaceWith(oldImage);
        node?.replaceWith(fresh);
        node = fresh;
      }
      node._tabMarkup = signature;
      existing.delete(key);
      if (workspace.tabs.children[index] !== node) workspace.tabs.insertBefore(node, workspace.tabs.children[index] || null);
    }
    for (const node of existing.values()) node.remove();
    while (workspace.tabs.children.length > wanted.length) workspace.tabs.lastElementChild.remove();
    prepareImages(workspace.tabs);
    const self = indexedTabs.find(({tab}) => tab.current)?.tab;
    if (self && !self.headerReported && workspace.tabs.isConnected) {
      self.headerReported = true;
      if (typeof window !== "undefined") window.reportFlowDiagnostic?.("self_tab_client", "ready",{startedAt:Date.now()});
    }
    const selectionChanged = workspace.tabs._activeKey !== state.activeTabs[group];
    workspace.tabs._activeKey = state.activeTabs[group];
    requestAnimationFrame(() => updatePlayerTabScrollControls(group, selectionChanged));
  }

  function tabGroup(tab) {
    if (tab?.current && tab.regionResolved === false) return "";
    return tab?.group === "pro" ? "pro" : riotTab(tab) ? "kr" : "players";
  }

  function normalizePlayerTabContext(context = {}) {
    const group = context?.group === "pro" ? "pro" : riotRegion(context?.region) || context?.group === "kr" ? "kr" : "players";
    if (group !== "pro") return { group };
    return {
      group,
      proTeam: String(context.proTeam || "").trim().slice(0, 64),
      proSecondary: context.proSecondary === true,
      proPlayer: String(context.proPlayer || "").trim().slice(0, 48),
      expectedTier: String(context.expectedTier || "").toUpperCase().slice(0, 16),
    };
  }

  function applyPlayerTabContext(tab, context = {}) {
    if (!tab) return;
    const normalized = normalizePlayerTabContext(context);
    tab.group = normalized.group;
    if (normalized.group === "pro") {
      tab.proTeam = normalized.proTeam || tab.proTeam || "";
      tab.proPlayer = normalized.proPlayer || tab.proPlayer || "";
      tab.proSecondary = normalized.proSecondary;
      tab.expectedTier = normalized.expectedTier || tab.expectedTier || "";
    } else {
      delete tab.proTeam;
      delete tab.proPlayer;
    }
  }

  function updatePlayerTabScrollControls(group, revealActive = false) {
    const { tabs, prev, next } = overviewWorkspace(group);
    if (!tabs || !prev || !next) return;
    // Judge against the full shell: visible arrows must not keep themselves visible
    // after a resize/group switch when all tabs now fit without them.
    const shell = tabs.parentElement;
    const gap = parseFloat(getComputedStyle(shell).columnGap) || 0;
    const available = shell.clientWidth ? shell.clientWidth - gap * 2 : tabs.clientWidth;
    const overflow = tabs.scrollWidth > available + 2;
    prev.hidden = !overflow;
    next.hidden = !overflow;
    if (revealActive) tabs.querySelector('[data-player-tab][aria-selected="true"]')?.scrollIntoView({ block: "nearest", inline: "nearest" });
    prev.disabled = !overflow || tabs.scrollLeft <= 1;
    next.disabled = !overflow || tabs.scrollLeft + tabs.clientWidth >= tabs.scrollWidth - 1;
  }

  function sameTabServerScope(tab, region, serverId) {
    const leftRegion = String(tab?.region || tab?.data?.player?.region || "").toLowerCase();
    const rightRegion = String(region || "").toLowerCase();
    if (riotRegion(leftRegion) || riotRegion(rightRegion)) return leftRegion === rightRegion;
    const leftServer = tabServerID(tab);
    const rightServer = String(serverId || "").toUpperCase();
    // “跟随客户端”会在首次响应后补成具体子服务器，两者属于同一作用域。
    return !leftServer || !rightServer || leftServer === rightServer;
  }

  function sameRiotID(tab, gameName, tagLine) {
    const loaded = tab?.data?.player || {};
    let candidate = tab?.riotId?.gameName ? tab.riotId : loaded.gameName ? loaded : null;
    if (!candidate) candidate = riotIDFromLabel(tab?.label);
    if (!candidate) return false;
    return String(candidate.gameName || "").trim().toLocaleLowerCase("zh-CN") === String(gameName || "").trim().toLocaleLowerCase("zh-CN")
      && String(candidate.tagLine || "").trim().toLocaleLowerCase("zh-CN") === String(tagLine || "").trim().toLocaleLowerCase("zh-CN");
  }




  function markProMismatch(tab) {
    if (tabGroup(tab) !== "pro" || !tab.riotId) return;
    tab.proMismatch = true;
    window.dispatchEvent(new CustomEvent("deep-legends:pro-verification", { detail: { ...tab.riotId, mismatch: true } }));
  }

  function summonerProChip(tab) {
    if (state.settings.maskNames || tabGroup(tab) !== "pro") return "";
    const label = [tab.proTeam, tab.proPlayer].filter(Boolean).join(" ");
    return label ? `<span class="pro-identity-chip${tab.proSecondary ? " is-secondary" : ""}">${escapeHTML(label)}${tab.proSecondary ? " · 二队" : ""}</span>${tab.proMismatch ? '<span class="pro-verification-warning">该账号与公开来源记录不一致</span>' : ""}` : "";
  }

  function summonerRegionChip(tab) {
    if (!tabServerLabel(tab)) return "";
    return `<span class="region-chip${riotTab(tab) ? " is-kr" : ""}" data-tooltip="${escapeHTML(tabServerTitle(tab))}" data-tooltip-size="compact">${escapeHTML(tabServerLabel(tab))}</span>`;
  }




  function riotIDFromLabel(label) {
    const value = String(label || "").trim();
    const separator = value.lastIndexOf("#");
    return separator > 0 && separator < value.length - 1
      ? { gameName: value.slice(0, separator), tagLine: value.slice(separator + 1) }
      : null;
  }

  function rememberTabPlayerRef(tab, playerRef) {
    if (!tab || tab.overlay) return;
    if (!(tab.playerRefs instanceof Set)) tab.playerRefs = new Set();
    for (const value of [tab.playerRef, tab.data?.player?.playerRef, playerRef]) {
      const normalized = String(value || "").trim();
      if (normalized) tab.playerRefs.add(normalized);
    }
  }

  function readPlayerTabOrder() {
    try {
      const parsed = JSON.parse(readSetting("player-tab-order", "[]"));
      if(!Array.isArray(parsed) || parsed.length>64 || parsed.some(item=>typeof item!=="string" || item.length>240))throw Error();
      return parsed;
    } catch (_) {
      try{localStorage.removeItem("lol-loot-player-tab-order");}catch{}
      return [];
    }
  }

  function playerTabOrderIdentity(tab) {
    if (!tab || tab.current) return "";
    const loaded = tab.data?.player || {};
    const parsedLabel = riotIDFromLabel(tab.label);
    const gameName = String(loaded.gameName || tab.riotId?.gameName || parsedLabel?.gameName || "").trim().toLocaleLowerCase("zh-CN");
    const tagLine = String(loaded.tagLine || tab.riotId?.tagLine || parsedLabel?.tagLine || "").trim().toLocaleLowerCase("zh-CN");
    const region = String(tab.region || loaded.region || "").trim().toLowerCase();
    const scope = riotRegion(region) ? region : `cn:${tabServerID(tab) || "current"}`;
    const group = tabGroup(tab) === "pro" ? "pro:" : "";
    if (gameName) return `${group}${scope}:riot:${gameName}#${tagLine}`;
    const playerRef = String(loaded.playerRef || tab.playerRef || "").trim();
    return playerRef ? `${group}${scope}:ref:${playerRef}` : "";
  }

  function applySavedPlayerTabOrder() {
    const current = state.tabs.find((tab) => tab.current);
    if (!current) return;
    const ranks = new Map((state.playerTabOrder || []).map((identity, index) => [identity, index]));
    const others = state.tabs.filter((tab) => !tab.current).map((tab, index) => ({ tab, index, rank: ranks.get(playerTabOrderIdentity(tab)) }));
    others.sort((left, right) => {
      const leftRanked = Number.isInteger(left.rank);
      const rightRanked = Number.isInteger(right.rank);
      if (leftRanked && rightRanked) return left.rank - right.rank;
      if (leftRanked !== rightRanked) return leftRanked ? -1 : 1;
      return left.index - right.index;
    });
    state.tabs = [current, ...others.map((item) => item.tab)];
  }

  function persistPlayerTabOrder() {
    const opened = state.tabs.map(playerTabOrderIdentity).filter(Boolean);
    const remaining = (state.playerTabOrder || []).filter((identity) => !opened.includes(identity));
    state.playerTabOrder = [...opened, ...remaining].slice(0, 64);
    writeSetting("player-tab-order", JSON.stringify(state.playerTabOrder));
  }

  function movePlayerTab(key, direction) {
    const tab = state.tabs.find((item) => item.key === key && !item.current);
    if (!tab) return false;
    const peers = state.tabs.filter((item) => !item.current && tabGroup(item) === tabGroup(tab));
    const fromPeer = peers.findIndex((item) => item.key === key);
    const toPeer = Math.max(0, Math.min(peers.length - 1, fromPeer + direction));
    if (toPeer === fromPeer) return false;
    const targetKey = peers[toPeer].key;
    const from = state.tabs.findIndex((item) => item.key === key);
    const [moving] = state.tabs.splice(from, 1);
    const target = state.tabs.findIndex((item) => item.key === targetKey);
    state.tabs.splice(target + (direction > 0 ? 1 : 0), 0, moving);
    persistPlayerTabOrder();
    renderPlayerTabs();
    requestAnimationFrame(() => overviewWorkspace(tabGroup(tab)).tabs?.querySelector(`[data-player-tab="${CSS.escape(key)}"]`)?.focus());
    return true;
  }

  function dropPlayerTab(key, targetKey, placeAfter) {
    const from = state.tabs.findIndex((tab) => tab.key === key && !tab.current);
    const target = state.tabs.findIndex((tab) => tab.key === targetKey);
    if (from < 1 || target < 0 || key === targetKey || tabGroup(state.tabs[from]) !== tabGroup(state.tabs[target])) return false;
    const [tab] = state.tabs.splice(from, 1);
    const adjustedTarget = state.tabs.findIndex((item) => item.key === targetKey);
    const targetIsCurrent = state.tabs[adjustedTarget]?.current;
    const insertion = Math.max(1, Math.min(state.tabs.length, adjustedTarget + (placeAfter || targetIsCurrent ? 1 : 0)));
    state.tabs.splice(insertion, 0, tab);
    persistPlayerTabOrder();
    renderPlayerTabs();
    return true;
  }

  function findExistingTab({ playerRef = "", gameName = "", tagLine = "", region = "", serverId = "", group = "players" } = {}) {
    const normalizedRef = String(playerRef || "").trim();
    const normalizedName = String(gameName || "").trim();
    const normalizedGroup = group === "pro" ? "pro" : riotRegion(region) ? "kr" : "players";
    return state.tabs.find((tab) => tabGroup(tab) === normalizedGroup && sameTabServerScope(tab, region, serverId) && (
      (normalizedRef && (tab.playerRefs?.has(normalizedRef) || tab.playerRef === normalizedRef || tab.data?.player?.playerRef === normalizedRef))
      || (normalizedName && sameRiotID(tab, normalizedName, tagLine))
    )) || null;
  }

  function savePlayerScroll(tab = activeTab()) {
    if (state.section !== "overview") return;
    if (tab && !tab.restoreScrollPending) tab.overviewScrollTop = Number(document.getElementById("app-scroll")?.scrollTop || 0);
  }
  function restorePlayerScroll(tab) {
    if (!tab || state.section !== overviewSectionForGroup(tabGroup(tab))) return;
    tab.restoreScrollPending = true;
    requestAnimationFrame(() => {
      if (activeTab() !== tab || state.section !== overviewSectionForGroup(tabGroup(tab))) return;
      const root = document.getElementById("app-scroll");
      if (root) root.scrollTop = Number(tab.overviewScrollTop || 0);
      tab.restoreScrollPending = Boolean(tab.loading && !tab.data);
    });
  }
  window.addEventListener("deep-legends:before-section", () => {savePlayerScroll();for(const tab of [...state.tabs,...state.overlay]){delete tab.overviewDialogReturn;cancelHistoryRecovery(tab);}if(globalThis.deepLegendsHistoryFilters)for(const tab of [...state.tabs,...state.overlay])cancelAdvancedMatchSearch(tab);});
  document.addEventListener('visibilitychange',()=>{for(const tab of [...state.tabs,...state.overlay]){if(document.hidden)cancelHistoryRecovery(tab);else if(historyRecoveryActive(tab))scheduleHistoryServerRetry(tab);}});

  function selectPlayerTab(key) {
    state.lastManualTabAt = Date.now();
    const tab = state.tabs.find((item) => item.key === key);
    if (!tab) return;
    if(activeTab()!==tab){delete activeTab()?.overviewDialogReturn;cancelHistoryRecovery(activeTab());}
    const group = tabGroup(tab);
    if(globalThis.deepLegendsHistoryFilters && activeTab() !== tab) cancelAdvancedMatchSearch(activeTab());
    savePlayerScroll();
    const currentKey = state.activeTabs[group];
    // 记录页签切换历史：关闭页签时返回上一个查看的页签。
    if (currentKey !== key && currentKey) {
      state.tabHistories[group] = (state.tabHistories[group] || []).filter((item) => item !== currentKey);
      state.tabHistories[group].push(currentKey);
      if (state.tabHistories[group].length > 20) state.tabHistories[group].shift();
    }
    state.activeTabs[group] = key;
    state.activeGroup = group;
    tab.restoreScrollPending = true;
    // 每个页签的筛选、展开状态与详情页签独立保留：来回切换玩家页签
    // 离开主页面再返回也保留，避免内容高度变化破坏滚动位置。
    renderPlayerTabs();
    if (tab.dirty) { renderOverview(group); scheduleDirtyOverview(tab); }
    else if (!tab.data && !tab.loading) loadOverview(tab);
    else renderOverview(group);
  }

  // 两个服务器的玩家互不相通：新页签/覆盖层继承来源页面的服务器标签，
  // 韩服总览里点到的玩家一定按韩服查询，国服页面同理。
  function rememberActiveTab(group) {
    delete activeTab()?.overviewDialogReturn;
    cancelHistoryRecovery(activeTab());
    if(globalThis.deepLegendsHistoryFilters) cancelAdvancedMatchSearch(activeTab());
    savePlayerScroll();
    const key = state.activeTabs[group];
    if (!key) return;
    state.tabHistories[group] = (state.tabHistories[group] || []).filter((item) => item !== key);
    state.tabHistories[group].push(key);
    if (state.tabHistories[group].length > 20) state.tabHistories[group].shift();
  }

  function openPlayer(playerRef, label, region, serverId = "", context = {}) {
    if (!playerRef) return;
    const riotId = riotIDFromLabel(label);
    const tabContext = normalizePlayerTabContext({ ...context, region });
    const existing = findExistingTab({ playerRef, gameName: riotId?.gameName, tagLine: riotId?.tagLine, region, serverId, group: tabContext.group });
    if (existing) { applyPlayerTabContext(existing, tabContext); selectPlayerTab(existing.key); return; }
    const tab = { key: `player-${state.tabs.length}-${Date.now()}`, playerRef, riotId, region: region || "", serverId: serverId || "", label: label || "隐藏玩家", current: false, loading: false, data: null, error: "", ...newTabView() };
    applyPlayerTabContext(tab, tabContext);
    rememberTabPlayerRef(tab, playerRef);
    rememberActiveTab(tabContext.group);
    state.tabs.push(tab);
    applySavedPlayerTabOrder();
    state.activeTabs[tabContext.group] = tab.key;
    state.activeGroup = tabContext.group;
    tab.restoreScrollPending = true;
    renderPlayerTabs();
    activateOverviewTabPanel(tabContext.group);
    loadOverview(tab);
  }

  // 按 Riot ID 打开玩家（顶部搜索）。
  function openPlayerByRiotId(gameName, tagLine, region, serverId = "", context = {}) {
    const label = `${gameName}${tagLine ? `#${tagLine}` : ""}`;
    const tabContext = normalizePlayerTabContext({ ...context, region });
    const existing = findExistingTab({ gameName, tagLine, region, serverId, group: tabContext.group });
    if (existing) { applyPlayerTabContext(existing, tabContext); selectPlayerTab(existing.key); return; }
    const tab = { key: `player-${state.tabs.length}-${Date.now()}`, playerRef: "", riotId: { gameName, tagLine }, region: region || "", serverId: serverId || "", label, current: false, loading: false, data: null, error: "", ...newTabView() };
    applyPlayerTabContext(tab, tabContext);
    rememberActiveTab(tabContext.group);
    state.tabs.push(tab);
    applySavedPlayerTabOrder();
    state.activeTabs[tabContext.group] = tab.key;
    state.activeGroup = tabContext.group;
    tab.restoreScrollPending = true;
    renderPlayerTabs();
    activateOverviewTabPanel(tabContext.group);
    loadOverview(tab);
  }

  /* ---------- 覆盖层：非总览页内查看玩家总览，左上角返回上一层 ---------- */

  function openPlayerOverlay(init) {
    if (init.region === "kr" && init.group === "pro") {
      window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: "overview" } }));
      openPlayer(init.playerRef, init.label, init.region, init.serverId || "", init);
      return;
    }
    const top = state.overlay[state.overlay.length - 1];
    if (top && init.playerRef && top.playerRef === init.playerRef && (top.serverId || "") === (init.serverId || "")) return;
    if (top && init.riotId && top.riotId && top.riotId.gameName === init.riotId.gameName && top.riotId.tagLine === init.riotId.tagLine && (top.region || "") === (init.region || "") && (top.serverId || "") === (init.serverId || "")) return;
    cancelHistoryRecovery(top || activeTab());
    delete (top || activeTab())?.overviewDialogReturn;
    const entry = {
      key: `overlay-${Date.now()}-${state.overlay.length}`, overlay: true, current: false,
      playerRef: "", region: "", serverId: "", label: "隐藏玩家", loading: false, data: null, error: "",
      ...newTabView(), ...init,
    };
    if(globalThis.deepLegendsHistoryFilters)cancelAdvancedMatchSearch(top || activeTab());
    state.overlay.push(entry);
    renderOverlay();
    loadOverview(entry);
  }

  function overlayBack() {
    const closed = state.overlay.pop();
    if (closed) {
      delete closed.overviewDialogReturn;
      cancelHistoryRecovery(closed);
      if(globalThis.deepLegendsHistoryFilters)cancelAdvancedMatchSearch(closed);
      closed.closed = true;
      clearTimeout(closed.quotaRetry?.timer);
      state.controllers.get(`overview:${closed.key}`)?.abort();
    }
    renderOverlay();
  }

  function closeOverlay() {
    while (state.overlay.length) {
      const closed = state.overlay.pop();
      delete closed.overviewDialogReturn;
      cancelHistoryRecovery(closed);
      if(globalThis.deepLegendsHistoryFilters)cancelAdvancedMatchSearch(closed);
      closed.closed = true;
      clearTimeout(closed.quotaRetry?.timer);
      state.controllers.get(`overview:${closed.key}`)?.abort();
    }
    if (nodes.playerOverlay) renderOverlay();
  }

  function renderOverlay() {
    if (!nodes.playerOverlay) return;
    state.overlayObserver?.disconnect();
    state.overlayObserver = null;
    const entry = state.overlay[state.overlay.length - 1];
    if (!entry) {
      nodes.playerOverlay.hidden = true;
      delete nodes.playerOverlay.dataset.entryKey;
      activateOverviewView(nodes.playerOverlayContent, null);
      nodes.playerOverlayContent.innerHTML = "";
      const tab=activeTab();if(tab)scheduleHistoryServerRetry(tab);
      return;
    }
    const entryChanged = nodes.playerOverlay.dataset.entryKey !== entry.key;
    nodes.playerOverlay.dataset.entryKey = entry.key;
    nodes.playerOverlay.hidden = false;
    const regionChip = `<span class="region-chip${riotTab(entry) ? " is-kr" : ""}" data-tooltip="${escapeHTML(tabServerTitle(entry))}" data-tooltip-size="compact">${escapeHTML(tabServerLabel(entry))}</span>`;
    const hiddenBadge = entry.data?.player?.privateHistory === true ? '<span class="player-tab-hidden">隐藏战绩</span>' : "";
    const overlayLabel = state.settings.maskNames ? privacyName(entry.data?.player || {displayName:entry.label},{context:"overlay"}) : entry.label;
    nodes.playerOverlayTitle.innerHTML = `<strong data-tooltip="${escapeHTML(overlayLabel)}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(overlayLabel)}</strong>${regionChip}${hiddenBadge}${state.overlay.length > 1 ? `<small>第 ${state.overlay.length} 层</small>` : ""}`;
    syncOverlayAddButton(entry);
    renderOverviewBody(nodes.playerOverlayContent, entry);
    const scroller = nodes.playerOverlay.querySelector(".player-overlay-scroll");
    if (scroller && entryChanged) scroller.scrollTop = 0;
  }

  function overlayPlayerIdentity(entry) {
    const player = entry?.data?.player || {};
    const riotId = entry?.riotId || (player.gameName ? { gameName: player.gameName, tagLine: player.tagLine || "" } : riotIDFromLabel(entry?.label));
    return {
      playerRef: String(player.playerRef || entry?.playerRef || "").trim(),
      gameName: String(riotId?.gameName || "").trim(),
      tagLine: String(riotId?.tagLine || "").trim(),
      region: String(player.region || entry?.region || "").trim(),
      serverId: String(player.serverId || entry?.serverId || "").trim(),
    };
  }

  function syncOverlayAddButton(entry) {
    if (!nodes.playerOverlayAdd) return;
    const identity = overlayPlayerIdentity(entry);
    const ready = Boolean(identity.playerRef || identity.gameName);
    const existing = ready ? findExistingTab(identity) : null;
    nodes.playerOverlayAdd.disabled = !ready || Boolean(existing);
    nodes.playerOverlayAdd.textContent = existing ? "已添加到总览" : ready ? "+ 添加到总览" : "正在读取玩家";
  }

  function addOverlayPlayerToTabs() {
    const entry = state.overlay[state.overlay.length - 1];
    if (!entry) return;
    const identity = overlayPlayerIdentity(entry);
    const existing = findExistingTab(identity);
    if (existing) {
      syncOverlayAddButton(entry);
      return;
    }
    if (identity.playerRef) openPlayer(identity.playerRef, entry.label, identity.region, identity.serverId);
    else if (identity.gameName) openPlayerByRiotId(identity.gameName, identity.tagLine, identity.region, identity.serverId);
    else return;
    syncOverlayAddButton(entry);
    showToast("已添加到总览");
  }

  function opggSummonerURL(tab) {
    if (!tab?.riotId?.gameName) return "";
    const region = ({kr:"kr",jp1:"jp",na1:"na",euw1:"euw",eun1:"eune",br1:"br",la1:"lan",la2:"las",me1:"me",oc1:"oce",ru:"ru",sg2:"sg",tr1:"tr",tw2:"tw",vn2:"vn"})[tab.region];
    if (!region) return "";
    const slug = `${tab.riotId.gameName}-${tab.riotId.tagLine || ""}`.replace(/-$/, "");
    return `https://op.gg/zh-cn/lol/summoners/${encodeURIComponent(region)}/${encodeURIComponent(slug)}`;
  }

  function closePlayerTab(key) {
    savePlayerScroll();
    const index = state.tabs.findIndex((tab) => tab.key === key && !tab.current);
    if (index < 0) return;
    delete state.tabs[index].overviewDialogReturn;
    cancelHistoryRecovery(state.tabs[index]);
    if(globalThis.deepLegendsHistoryFilters)cancelAdvancedMatchSearch(state.tabs[index]);
    state.tabs[index].closed = true;
    clearTimeout(state.tabs[index].quotaRetry?.timer);
    for(const key of ["historyWaitTimer","historyCountdownTimer","historyServerRetryTimer"])clearTimeout(state.tabs[index][key]);
    state.controllers.get(`overview:${key}`)?.abort();
    state.controllers.get(`overview-more:${key}`)?.abort();
    state.controllers.get(`opgg-season:${key}`)?.abort();
    state.controllers.get(`current-game:${key}`)?.abort();
    const [closedTab] = state.tabs.splice(index, 1);
    const group = tabGroup(closedTab);
    const wasActive = state.activeTabs[group] === key;
    const closedIdentity = playerTabOrderIdentity(closedTab);
    state.playerTabOrder = (state.playerTabOrder || []).filter((identity) => identity !== closedIdentity);
    writeSetting("player-tab-order", JSON.stringify(state.playerTabOrder));
    state.tabHistories[group] = (state.tabHistories[group] || []).filter((item) => item !== key);
    if (state.activeTabs[group] === key) {
      // 返回进入这个页签之前查看的页签；历史里已关闭的项跳过。
      let previous = "";
      while (state.tabHistories[group].length && !previous) {
        const candidate = state.tabHistories[group].pop();
        if (state.tabs.some((tab) => tab.key === candidate && tabGroup(tab) === group)) previous = candidate;
      }
      state.activeTabs[group] = previous || state.tabs.find(tab => tabGroup(tab) === group)?.key || "";
    }
    if (activeTab(group)) activeTab(group).restoreScrollPending = true;
    renderPlayerTabs();
    if(group==='pro' && !state.tabs.some(tab=>tabGroup(tab)==='pro')) {
      window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:'pro-players'}}));
      return;
    }
    if (wasActive) activateOverviewTabPanel(group);
  }

  function renderOverview(group) {
    if (state.destroyed || state.section !== "overview") return;
    group = group || overviewGroupForSection();
    if (state.destroyed || state.section !== "overview" || group !== overviewGroupForSection()) return;
    const workspace = overviewWorkspace(group);
    for (const key of ["matchObserver", "proMatchObserver", "matchTierObserver", "proMatchTierObserver"]) { state[key]?.disconnect(); state[key] = null; }
    const tab = activeTab(group);
    activateOverviewView(workspace.content, tab);
    window.dispatchEvent(new CustomEvent("deep-legends:overview-tab", { detail: { current: Boolean(tab?.current) || !tab && group !== "pro", group } }));
    workspace.content.closest(".gameplay-panel")?.classList.remove("is-disconnected");
    if (!tab) {
      workspace.content.innerHTML = group === "pro" ? emptyState("尚未打开职业选手账号", "请返回职业选手目录选择账号。", false, "◆") : "";
      return;
    }
    if (!tabReady(tab)) { workspace.content.innerHTML = ""; return; }
    renderOverviewBody(workspace.content, tab);
    if (tab.restoreScrollPending) restorePlayerScroll(tab);
  }

  function activateOverviewTabPanel(group = overviewGroupForSection()) {
    if (state.destroyed || state.section !== "overview" || group !== overviewGroupForSection()) return;
    const workspace = overviewWorkspace(group), tab = activeTab(group);
    if (!workspace.content) return;
    const changed = workspace.content._overviewViewTab !== tab;
    if (changed) for (const key of group === "pro" ? ["proMatchObserver", "proMatchTierObserver"] : ["matchObserver", "matchTierObserver"]) { state[key]?.disconnect(); state[key] = null; }
    activateOverviewView(workspace.content, tab);
    window.dispatchEvent(new CustomEvent("deep-legends:overview-tab", {detail:{current:Boolean(tab?.current) || !tab && group !== "pro",group}}));
    workspace.content.closest(".gameplay-panel")?.classList.remove("is-disconnected");
    if (!tab || !tabReady(tab)) { workspace.content.replaceChildren(); return; }
    // Restore cached nodes. The body reconciler only touches this active panel;
    // inactive player lists and the other workspaces never enter this update.
    renderOverviewBody(workspace.content, tab);
    if (tab.restoreScrollPending) restorePlayerScroll(tab);
    if(tab.historyRecoveryPending && !tab.loading)void retryHistoryRegion(tab,"outage-recovery");
  }

  // renderOverviewBody 把某个玩家页签（总览页签或覆盖层条目）的总览
  // 渲染到指定容器；总览页与覆盖层共用同一套展示与交互。

  function overviewRenderCacheFields() {
    return ["_overviewSubpageStash", "_stripHTML", "_backgroundArt", "_careerSignature", "_careerChildHTML", "_careerHTML",
      "_careerObjectTokens", "_careerObjectCounter", "_matchListMatches", "_matchListFilter", "_matchListViewRevision"];
  }

  function resetOverviewRenderCache(container) {
    delete container.dataset.matchTierScope;
    for (const field of overviewRenderCacheFields()) delete container[field];
  }

  function activateOverviewView(container, tab) {
    const fields = overviewRenderCacheFields();
    const views = container._overviewViews || (container._overviewViews = new Map());
    for (const cached of views.keys()) {
      if (cached.closed && !cached.current || !state.tabs.includes(cached) && !state.overlay.includes(cached)) { disposeMatchRowHeight(views.get(cached).content); views.delete(cached); }
    }
    if (container._overviewViewTab === tab) return;
    const previous = container._overviewViewTab;
    cancelOverviewProgress(previous);
    if (previous && !previous.closed && (state.tabs.includes(previous) || state.overlay.includes(previous))) {
      const content = container.ownerDocument.createDocumentFragment();
      while (container.firstChild) content.appendChild(container.firstChild);
      const values = Object.fromEntries(fields.map((field) => [field, container[field]]));
      views.delete(previous);
      views.set(previous, { content, scope: container.dataset.matchTierScope, values });
    } else {
      disposeMatchRowHeight(container);
      container.replaceChildren();
    }
    const restored = tab && views.get(tab);
    if (restored) {
      views.delete(tab);
      container.appendChild(restored.content);
      container.dataset.matchTierScope = restored.scope || "";
      for (const field of fields) container[field] = restored.values[field];
    } else {
      resetOverviewRenderCache(container);
    }
    container._overviewViewTab = tab;
    // Keep only a few inactive player views; evicted nodes can be rebuilt normally.
    while (views.size > 4) {const key=views.keys().next().value;disposeMatchRowHeight(views.get(key).content);views.delete(key);}
  }

  // Queue labels preserve verified IDs; partial history never means zero.
  const MAYHEM_QUEUE_TAB_KEY = "2300";

  function rankedQueueLabel(queueId, fallback = "") {
    const id = Number(queueId);
    if (!Number.isFinite(id)) return String(fallback || "");
    if (id === 420) return "单双排";
    if (id === 440) return "灵活组排";
    if (isMayhemQueueId(id)) return "海克斯大乱斗";
    return String(fallback || "");
  }

  function isMayhemQueueId(queueId) { return [2300, 2400, 3270].includes(Number(queueId)); }

  function rankedQueueNoun(queueId) { return isMayhemQueueId(queueId) ? "海斗" : "排位"; }

  function rankedQueueData(data, tab, scope = "recent") {
    const stateKey = { recent: "rankedQueueRecent", ability: "rankedQueueAbility", position: "rankedQueuePosition" }[scope] || "rankedQueueRecent";
    const key = String(tab?.[stateKey] || "420");
    const queue = data?.rankedQueues?.[key];
    if (!data?.rankedQueues) {
      if (key === "440") return { recentRanked: { queueId: 440, queueLabel: "灵活组排" }, ability: null, abilitySampleGames: 0, queueGames: 0, positions: [], queueId: 440, queueLabel: "灵活组排", positionQueueId: 440, positionQueueLabel: "灵活组排" };
      if (isMayhemQueueId(key)) return { recentRanked: { queueId: Number(key), queueLabel: rankedQueueLabel(key) }, ability: null, abilitySampleGames: 0, queueGames: 0, positions: [], queueId: Number(key), queueLabel: rankedQueueLabel(key), positionQueueId: Number(key), positionQueueLabel: rankedQueueLabel(key), mayhem: true, positionsApplicable: false };
      const recentRanked = data?.recentRanked || {};
      return { recentRanked, ability: data?.ability || null, abilitySampleGames: Number(data?.ability?.sampleGames || recentRanked.games || 0), queueGames: Number(recentRanked.games || 0), positions: data?.positions || [], queueId: Number(recentRanked.queueId || 420), queueLabel: recentRanked.queueLabel || "单双排", positionQueueId: Number(recentRanked.queueId || 420), positionQueueLabel: recentRanked.queueLabel || "单双排" };
    }
    if (!queue) return { recentRanked: { queueId: Number(key), queueLabel: rankedQueueLabel(key) }, ability: null, abilitySampleGames: 0, queueGames: 0, positions: [], queueId: Number(key), queueLabel: rankedQueueLabel(key), positionQueueId: Number(key), positionQueueLabel: rankedQueueLabel(key), mayhem: isMayhemQueueId(key), positionsApplicable: !isMayhemQueueId(key) };
    const mayhem = isMayhemQueueId(key);
    return { recentRanked: queue.recentRanked || {}, ability: queue.ability || null, abilitySampleGames: Number(queue.abilitySampleGames || queue.ability?.sampleGames || 0), queueGames: Number(queue.recentRanked?.games || 0), positions: queue.positions || [], queueId: Number(key), queueLabel: queue.recentRanked?.queueLabel || rankedQueueLabel(key), positionQueueId: Number(queue.positionQueueId || key), positionQueueLabel: queue.positionQueueLabel || rankedQueueLabel(Number(queue.positionQueueId || key)), mayhem, positionsApplicable: !mayhem };
  }

  function rankedQueueSwitcher(tab, activeQueue, scope = "recent", data = null) {
    const stateKey = { recent: "rankedQueueRecent", ability: "rankedQueueAbility", position: "rankedQueuePosition" }[scope] || "rankedQueueRecent";
    const selected = String(activeQueue || tab?.[stateKey] || "420");
    const payload = data || tab?.data || null;
    const mayhemAvailable = scope === "recent" && !!payload?.rankedQueues?.[MAYHEM_QUEUE_TAB_KEY];
    const mayhemButton = mayhemAvailable ? `<button type="button" class="ranked-queue-button${selected === MAYHEM_QUEUE_TAB_KEY ? " is-active" : ""}" data-ranked-queue="${MAYHEM_QUEUE_TAB_KEY}" aria-pressed="${selected === MAYHEM_QUEUE_TAB_KEY}">海斗</button>` : "";
    const groupLabel = mayhemAvailable ? "对局模式" : "排位模式";
    return `<div class="ranked-queue-switcher" role="group" aria-label="${groupLabel}" data-ranked-queue-scope="${scope}"><button type="button" class="ranked-queue-button${selected === "420" ? " is-active" : ""}" data-ranked-queue="420" aria-pressed="${selected === "420"}">单双排</button><button type="button" class="ranked-queue-button${selected === "440" ? " is-active" : ""}" data-ranked-queue="440" aria-pressed="${selected === "440"}">灵活组排</button>${mayhemButton}</div>`;
  }

  function casualOverview(data,tab) {
    if(globalThis.deepLegendsOverviewCasual)return globalThis.deepLegendsOverviewCasual.select(data,tab);
    if(!tab.casualModulePending && !tab.loading && data.capabilities?.some(c=>c.name==='ranked-stats' && c.state==='available') && !(data.ranks || []).some(r=>Number(r.wins)+Number(r.losses)>0) && globalThis.deepLegendsSections){
      tab.casualModulePending=true;
      void globalThis.deepLegendsSections.load('overview-casual').then(()=>{if(!tab.closed && tab.data)updateCareerCards(tab,'casual_ready');}).catch(()=>{}).finally(()=>{tab.casualModulePending=false;});
    }
    return null;
  }

  function careerSectionEntries(data, tab) {
    const seasonProgress = data.seasonStatsProgress;
    const pendingMatches=matchesPending(tab),pendingAggregates=Boolean(tab?.loading && (tab.initialPagePending || !data.matches?.length) && !tab.recentStatsReceived && !(Number(data?.historyRequested)>0)),pendingStats=!pendingMatches && !pendingAggregates && historyStatsPending(data,tab);
    const partialHistory=Boolean(data.pagination?.partial || tab?.initialPagePending || tab?.initialPageError || tab?.loading || data.capabilities?.some(item=>item.name==="match-history" && item.state==="failed"));
    const recentHistoryCapability = (data.capabilities || []).find((item) => item.name === "seven-day-history");
    const casual = !pendingMatches && !pendingAggregates && !pendingStats ? casualOverview(data,tab) : null;
    const recentQueue = rankedQueueData(data, tab, "recent");
    const abilityQueue = rankedQueueData(data, tab, "ability");
    const positionQueue = rankedQueueData(data, tab, "position");
    const hasSeason = data.seasonStatsProgress && !data.seasonStatsProgress.unavailable;
    const opggSeason = riotRegion(data.player?.region) && !data.player?.privateHistory && tab?.opggSeason?.playerRef === data.player?.playerRef ? tab.opggSeason.data : null;
    const missingKRSeason = riotRegion(data.player?.region) && !opggSeason;
    const championRows = opggSeason?.champions || (missingKRSeason ? [] : hasSeason ? (data.seasonChampionStats || []) : (data.championStats || []));
    const championOverall = opggSeason?.overall || (missingKRSeason ? {} : hasSeason ? (data.seasonOverall || {}) : (data.overall || {}));
    const games = { 'recent-ranked':Number(recentQueue.recentRanked?.games || 0), ability:Number(abilityQueue.abilitySampleGames || 0), champions:Number(championOverall?.games || 0) };
    for (const [card,count] of Object.entries(games)) (typeof window!=='undefined'?window:globalThis).reportFlowDiagnostic?.('career_cards_render','render',{card,games:count,source:tab.careerStatsSource || (tab.loading?'retained':'cache'),generation:Number(tab.careerGeneration || 0)});
    const championProgress = opggSeason ? { season: opggSeason.season, complete: true, tableSupported:true, foreign:true, scannedGames:opggSeason.overall.games, message: `${tab.opggSeasonStale ? "缓存 · " : ""}${number(opggSeason.overall.games)} 场排位` } : missingKRSeason ? { seasonOnly: true, foreign:true, tableSupported:!data.player?.privateHistory, unavailable: true, message: data.player?.privateHistory ? "该玩家战绩不可公开查询" : tab?.opggSeasonPending || !tab?.opggSeasonAttemptRef ? "" : "本赛季英雄统计暂不可用，请刷新重试" } : data.seasonStatsProgress;
    return [
	  ["ranks", renderRanks(data.ranks || [], data.capabilities || [], data.historicalRanks || [], data.rankMilestones, data.seasonStatsProgress, tab)],
	  ["recent-ranked", renderRecentRanked(recentQueue.recentRanked, recentQueue.queueId, rankedQueueSwitcher(tab, recentQueue.queueId, "recent", data),partialHistory)],
	  ["ability", renderAbility(abilityQueue.ability, abilityQueue.queueLabel, rankedQueueSwitcher(tab, abilityQueue.queueId, "ability"), abilityQueue.abilitySampleGames, abilityQueue.queueGames)],
      ["champions", renderChampionStats(championRows, championOverall, championProgress)],
      ["masteries", renderMasteries(data.masteries || [], (data.capabilities || []).some(item => item.name === "champion-mastery" && item.state === "available"), data.masteryChampionCount)],
	  ["positions", renderPositionStats(positionQueue.positions, positionQueue.positionQueueId || positionQueue.queueId, rankedQueueSwitcher(tab, positionQueue.queueId, "position"), positionQueue.positionQueueLabel, positionQueue.queueGames,partialHistory)],
      ["recent-players", renderRecentPlayers(data.recentPlayers || [], recentHistoryCapability,false,partialHistory)],
      ["activity", renderActivity(data.activityHours || []).replace("</section>", `${renderOverviewShareButton()}</section>`)],
    ].filter(([key])=>!casual || !['ranks','ability','positions'].includes(key) || key==='ranks' && (data.rankMilestones?.peakTier || data.rankMilestones?.previousSeason?.some(r=>r.tier) || data.historicalRanks?.some(r=>r.tier))).map(([key,markup])=> {
      if(casual){
        const label=casual.selected?.label || '';
        if(key==='ranks'){
          const history=renderRankMilestones(data.rankMilestones,true) || renderRankMilestones({previousSeason:(data.historicalRanks || []).filter(r=>r.tier).slice(0,1).map(r=>({...r,queueType:'RANKED_SOLO_5x5'}))});
          markup=`<section class="career-section rank-career-section"><header><h3>排位${renderRankWindowNote(seasonProgress)}</h3></header>${history}</section>`;
        }
        if(key==='recent-ranked'){
          const tools=casual.modes.length>1?`<div class="ranked-queue-switcher" role="group" aria-label="对局模式">${casual.modes.map(m=>`<button type="button" class="ranked-queue-button${m.key===tab.casualMode?' is-active':''}" data-casual-mode="${m.key}" aria-pressed="${m.key===tab.casualMode}">${escapeHTML(m.label)}</button>`).join('')}</div>`:escapeHTML(label);
          markup=renderRecentRanked(casual.stats,2300,tools,partialHistory).replaceAll('近期海斗','近期对局');
        }
        if(key==='champions')markup=renderChampionStats(casual.champions,casual.overall,{message:label});
      }

      const capability={ranks:'ranked-stats',masteries:'champion-mastery'}[key];
      if(tab.loading && key!=='ranks' && capability && !data.masteries?.length && !data.capabilities?.some(c=>c.name===capability))return [key,renderCareerPendingMarkup(markup.replace('<section',`<section data-career-key="${key}"`))];
      const titles={'recent-ranked':'近期排位',ability:'能力表现',positions:'位置偏好','recent-players':'最近一起玩',activity:'游戏时间分布',champions:'英雄胜率'};
      if((historyServiceState(tab)?.kind==="official" || !(data.matches?.length) && (tab.initialPageError || tab.error) && !tab.loading) && titles[key])return [key,`<section data-career-key="${key}" class="career-section ${key}-section${key==='champions'?' champion-performance':''}"><header><h3>${titles[key]}</h3></header><div class="ability-unavailable"><strong>暂不可用</strong></div></section>`];
      if((pendingMatches || pendingAggregates || pendingStats || key==='champions' && championStatsPending(championRows,championOverall,championProgress,tab)) && titles[key])return [key,renderCareerPendingMarkup(markup.replace('<section',`<section data-career-key="${key}"`))];
      markup=markup.replace('<section',`<section data-career-key="${key}"`);
      return [key,!pendingStats && partialHistory && ['ability','positions','recent-players','activity'].includes(key)?markup.replace('</h3>',`</h3><small class="history-sample-label">基于最近 ${Number(data.historyLoaded ?? data.matches?.length ?? 0)} 场</small>`):markup];
    });
  }

  function renderCareerSections(data, tab) {
    return careerSectionEntries(data, tab).map(([, markup]) => markup).join("");
  }

  function renderCareerDialogSections(data, tab) {
    const items=careerSectionEntries(data,tab).map(([key,markup],index)=>`<div class="career-dialog-item is-${key}" data-career-order="${index}">${markup}</div>`).join('');
    return `<div class="career-dialog-stack">${items}</div><div class="career-dialog-stack"></div>`;
  }

  function distributeCareerDialog() {
    const root=nodes.careerDialogContent;
    if (!root || root.dataset.careerLayout !== 'masonry') return;
    const columns=[...root.querySelectorAll(':scope>.career-dialog-stack')];
    if(columns.length!==2)return;
    const items=[...root.querySelectorAll('.career-dialog-item')].sort((a,b)=>Number(a.dataset.careerOrder)-Number(b.dataset.careerOrder));
    const heights=[0,0],ordered=[[],[]];
    for(const item of items){const column=heights[0]<=heights[1]?0:1;ordered[column].push(item);heights[column]+=item.getBoundingClientRect().height+10*(Number(document.documentElement.dataset.uiScale)||1);}
    columns.forEach((column,index)=>ordered[index].forEach((item,position)=>{if(column.children[position]!==item)column.insertBefore(item,column.children[position] || null);}));
  }


  // Turn render exceptions into retryable errors rather than endless skeletons.
  function renderOverviewBody(container, tab) {
    if(!window.deepLegendsPlayerTags)void window.deepLegendsSections?.load("player-tags").then(()=>window.deepLegendsPlayerTags?.refresh()).catch(()=>{});
    window.reportFlowDiagnostic?.('overview_rerender_client','render',{reason:tab.overviewRenderReason || 'renderOverview',tabType:tab.current?'self':tabGroup(tab)==='pro' || tab.data?.player?.proPlayer?'pro':riotTab(tab)?'riot':'cn',filterRetained:Boolean(container.querySelector('.match-filterbar')),modalOpen:Boolean(tab.advancedMenu?.open)});
    if(tab.advancedMenu?.open && container._afBoundTab===tab){tab.overviewRenderDeferred=true;globalThis.deepLegendsHistoryFilters?.refresh(container,tab,advancedFilterContext(tab));return;}
    try {
      const scroll=container.closest('.player-overlay-scroll') || document.getElementById('app-scroll'),top=scroll?.scrollTop;
      const retain=container._overviewViewTab===tab;
      activateOverviewView(container, tab);
      renderOverviewBodyContent(container, tab);
      if(retain && scroll && top>0 && scroll.scrollTop===0){
        window.reportFlowDiagnostic?.('overview_scroll_reset','restored',{render_reason:tab.overviewRenderReason || 'renderOverview',tab_type:tab.current?'self':tabGroup(tab),scroll_before:top,scroll_after:scroll.scrollTop});scroll.scrollTop=top;
      }
      scheduleHistoryServerRetry(tab);
      if (tab.current && tab.data && connected()) window.dispatchEvent(new CustomEvent("deep-legends:overview-ready"));
    } catch (error) {
      console.error("总览渲染失败", error);
      container.innerHTML = emptyState("总览渲染失败", `页面渲染时出现异常：${error?.message || error}`, true);
      container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => historyServiceState(tab)?retryHistoryRegion(tab):loadOverview(tab, true));
    }
  }

  function renderCareerPendingMarkup(markup) {
    return markup.replace('class="career-section','class="is-career-pending career-section').replace(/(<section[^>]*>(?:\s*<header>[\s\S]*?<\/header>)?)([\s\S]*)(<\/section>)$/, '$1<div class="career-pending gameplay-skeleton" aria-hidden="true">$2</div>$3');
  }

  function syncMatchFilterPending(filterbar, tab) {
    const pending=matchesPending(tab);
    filterbar.classList.toggle('gameplay-skeleton',pending);
    if(pending)filterbar.setAttribute('aria-label','正在读取筛选');else filterbar.removeAttribute('aria-label');
  }

  function renderPendingMatchFilters(tab = {matchFilter:'all'}) {
    return renderMatchFilters(tab).replace('class="match-filterbar"','class="match-filterbar gameplay-skeleton" aria-label="正在读取筛选"');
  }

  function renderOverviewPendingPage(tab) {
    const pending={...tab,loading:true,initialPagePending:true,matchesReceived:false,recentStatsReceived:false};
    const data={player:tab.identity || tab.riotId || {},matches:[],capabilities:[]};
    return `<div class="overview-layout${riotTab(tab)?' is-riot-overview':''}"><aside class="career-column">${renderCareerSections(data,pending)}</aside><section class="matches-column"><div class="overview-career-entry"><button class="career-dialog-trigger" type="button"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9m5 10V5m5 14v-7m5 7V8"/></svg><span>生涯统计</span></button></div>${renderPendingMatchFilters(tab)}<div class="match-list">${renderPendingMatches()}</div><div class="match-pagination" data-match-sentinel hidden></div></section></div>`;
  }

  function renderSummonerStrip(data,tab,pending=false) {
    const player=data?.player || tab.identity || tab.riotId || {},masked=state.settings.maskNames;
    const profileName=masked?privacyName(player,{index:Math.max(0,state.tabs.findIndex(t=>t.key===tab.key)),context:"identity"}):(player.gameName || player.displayName || tab.riotId?.gameName || tab.label || '');
    const tag=`<span class="summoner-tag${!masked && player.tagLine?'':' is-identity-placeholder'}">${!masked && player.tagLine?'#'+escapeHTML(player.tagLine):'&nbsp;'}</span>`;
    const meta=tag+summonerRegionChip(tab)+renderOverviewStreak(tab.overviewStreak)+(!masked?summonerProChip(tab):'');
    const copy=!masked && !player.hidden && profileName?`${profileName}${player.tagLine?'#'+player.tagLine:''}`:'';
    const avatar=pending && !player.profileIconId?'<span class="game-icon is-summoner-avatar is-pending"></span>':masked?maskedProfileIcon('summoner-avatar'):iconFigure('profile',player.profileIconId,profileName,'summoner-avatar',false).replace(`alt="${escapeHTML(profileName)}"`,'alt=""');
    const art=window.deepLegendsOverviewArt?.render(player,tab.key,!pending && !tab.loading && !tab.initialPagePending) || (player.backgroundSource && player.backgroundPath?`<img class="summoner-strip-art" data-queued-src="/api/champion-asset?source=${encodeURIComponent(player.backgroundSource)}&path=${encodeURIComponent(player.backgroundPath)}" alt="" aria-hidden="true" decoding="async" data-game-image>`:'');
    const highlights=renderSummonerHighlights(data?.ranks || [],data?.masteries || [],{pending:pending || Boolean(tab.loading || tab.initialPagePending),capabilities:data?.capabilities || []});
    return `<section class="summoner-strip${pending?' is-pending-strip':''}">${art}${avatar}<div class="summoner-strip-copy"><div><h2${copy?` class="summoner-copy-name" data-tooltip="${escapeHTML(profileName)}" data-tooltip-overflow="self" role="button" tabindex="0" data-copy-summoner="${escapeHTML(copy)}"`:''}>${profileName?escapeHTML(profileName):'<span class="skeleton-line"></span>'}</h2><div class="summoner-name-meta">${meta}</div></div><p class="summoner-level-row"><span>${Number(player.summonerLevel)>0?`召唤师等级 <b class="summoner-level-value">${number(player.summonerLevel)}</b>`:pending?'<span class="skeleton-line"></span>':''}${player.hidden?' · 身份已隐藏':''}</span>${player.privateHistory?'<span class="player-tab-hidden">隐藏战绩</span>':''}</p><p class="player-live-chip" data-friend-presence="${escapeHTML(tab.key)}" hidden></p></div>${highlights}</section>`;
  }

  function renderSelfIdentityHeader(tab) {return renderSummonerStrip(null,tab,true);}

  function cancelOverviewProgress(tab) {
    if(!tab)return;
    if(tab.progressFrame)cancelAnimationFrame(tab.progressFrame);
    tab.progressFrame=0;
    tab.progressFrameGeneration=Number(tab.progressFrameGeneration || 0)+1;
  }

  function patchOverviewProgress(tab, requestToken = tab.overviewRequestToken) {
    if(tab.closed || state.destroyed || tab.overviewRequestToken!==requestToken)return;
    const container=overviewContainer(tab);
    if(!container || container._overviewViewTab!==tab || tab.overlay && (state.overlay.at(-1)!==tab || nodes.playerOverlay.hidden)){
      tab.overviewProgressPending=true;
      return;
    }
    if(tab.overviewSubpage || tab.advancedMenu?.open){tab.overviewProgressPending=true;tab.overviewRenderDeferred=true;return;}
    if(!container?.querySelector('.overview-layout')){rerenderTab(tab,'stream-first');return;}
    const template=document.createElement('template'),strip=renderSummonerStrip(tab.data,tab);
    if(container._stripHTML!==strip){template.innerHTML=strip;const fresh=template.content.firstElementChild;container.querySelector('.summoner-strip')?.replaceWith(fresh);container._stripHTML=strip;bindOverviewContent(fresh,tab,container);prepareImages(fresh);}
    updateCareerCards(tab,'stream');
    const list=container.querySelector('.match-list');reconcileFilteredMatchList(list,tab);
    syncMatchFilterPending(container.querySelector('.match-filterbar'),tab);
    reportOverviewCardReady(container,tab);reportDeferredMatchesCard(tab);
    tab.overviewProgressPending=false;
  }
  function scheduleOverviewProgress(tab) {
    if(tab.progressFrame)return;
    const requestToken=tab.overviewRequestToken;
    const generation=tab.progressFrameGeneration=Number(tab.progressFrameGeneration || 0)+1;
    tab.progressFrame=requestAnimationFrame(()=>{if(tab.progressFrameGeneration!==generation)return;tab.progressFrame=0;patchOverviewProgress(tab,requestToken);});
  }

  function renderOverviewBodyContent(container, tab) {
    document.querySelectorAll(".mastery-details-popover").forEach(node=>node.remove());
    if (tab.overviewSubpage) { renderOverviewSubpage(container, tab); return; }
    if (container._overviewSubpageStash) {
      container.replaceChildren(container._overviewSubpageStash);
      delete container._overviewSubpageStash;
      reportOverviewCardReady(container,tab);
      prepareImages(container);
      return;
    }
    const tierScope = matchTierScope(tab);
    const quotaMessage = tab.quotaRetry && tab.quotaRetry.retryAt>Date.now() ? `<p role="status" data-quota-recovery>外服查询额度恢复中，约 <b data-quota-seconds>${Math.max(0, Math.ceil((tab.quotaRetry.retryAt - Date.now()) / 1000))}</b> 秒后自动开始；${tab.data?.matches?.length?"已加载的战绩仍可查看。":""}</p>` : "";
    const sameOverview = container.dataset.matchTierScope === tierScope;
    container.dataset.matchTierScope = tierScope;
    if (!tab.data && !tab.error) {
      disposeMatchRowHeight(container);
      container.innerHTML = quotaMessage + renderSelfIdentityHeader(tab) + (historyServiceState(tab) ? renderHistoryServiceStatus(tab) : tab.quotaRetry ? "" : renderOverviewPendingPage(tab));
      container.querySelector('[data-gameplay-retry]')?.addEventListener('click',()=>historyServiceState(tab)?retryHistoryRegion(tab):loadOverview(tab,true));
      if(typeof fitMatchTags==='function')fitMatchTags(container);
      return;
    }
    if (tab.error) {
      const opggEscape = tab.riotId && riotTab(tab) ? '<div class="opgg-escape"><button class="text-button" type="button" data-open-opgg>在 OP.GG 中查看该玩家 ↗</button></div>' : "";

      disposeMatchRowHeight(container);
      container.innerHTML = renderSelfIdentityHeader(tab) + (tab.proMismatch ? summonerProChip(tab) : "") + (historyServiceState(tab) ? renderHistoryServiceStatus(tab) : emptyState("战绩读取失败",tab.error,true)) + opggEscape;
      container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => historyServiceState(tab)?retryHistoryRegion(tab):loadOverview(tab, true));
      container.querySelector("[data-open-opgg]")?.addEventListener("click", () => window.open(opggSummonerURL(tab), "_blank", "noopener"));
      return;
    }
    const data = tab.data;
    const player = data.player || {};
    const rawMatches = data.matches || [];
    const matches = filteredMatches(rawMatches, tab);
    if(globalThis.deepLegendsHistoryRecovery?.update(container,tab,sameOverview,{entries:()=>careerSectionEntries(data,tab),reconcile:reconcileFilteredMatchList,prepare:prepareImages,ready:reportOverviewCardReady})){
      return;
    }
    const retainedMatchList = container.querySelector(".match-list");
    const retainedFilterbar = sameOverview && container._afBoundTab === tab
      && container._matchListViewRevision === Number(tab.matchViewRevision || 0)
      ? container.querySelector(".match-filterbar") : null;
    const preserveMatchList = Boolean(retainedMatchList
      && sameOverview
      && container._matchListViewRevision === Number(tab.matchViewRevision || 0));
    const retainMatchListContainer = Boolean(retainedMatchList && sameOverview
      && (preserveMatchList || tab.catalogViewRevision === Number(tab.matchViewRevision || 0)));
    if (retainMatchListContainer) retainedMatchList.remove();
    const historyCapability = (data.capabilities || []).find((item) => item.name === "match-history");
    const matchDetailsCapability = (data.capabilities || []).find((item) => item.name === "match-details");
    const matchDetailsWarning = !historyServiceState(tab) && !riotTab(tab) && ["failed", "unsupported"].includes(matchDetailsCapability?.state) && (matches.length || historyCapability?.state === "available")
      ? `<div class="notice is-warning match-details-warning" role="alert"><div class="notice-symbol" aria-hidden="true">!</div><p>${escapeHTML(matchDetailsCapability.detail || "完整对局参与者读取不完整，部分场次只能显示接口实际返回的玩家")}</p></div>`
      : "";
    const pagination = data.pagination || { hasMore: false };
    const specialModeEmpty = tab.matchFilter === "more:special"
      ? "自定义对局不会展示；这里只保留客户端实际返回的其他特殊模式。"
      : "可以切换上方游戏类型，或刷新读取最新战绩。";
    const emptyMatches = matchListEmptyContent(tab, historyCapability, specialModeEmpty);
    const sentinelHidden = matchSentinelShouldHide(tab, matches.length > 0) ? " hidden" : "";
    const backgroundArt=JSON.stringify([player.backgroundSource,player.backgroundPath,player.backgroundSkinId,player.backgroundPosterPath,player.backgroundVideoPath]);
    const paginationCopy = paginationCopyFor(tab);
    if (!container._careerObjectTokens) container._careerObjectTokens = new WeakMap();
    const careerSignature = [
      Boolean(state.settings.maskNames),
      data.player, data.capabilities, data.ranks, data.historicalRanks, data.rankMilestones, data.seasonStatsProgress,
      data.seasonChampionStats, data.seasonOverall, data.championStats, data.overall,
      data.masteries, data.activityHours, data.recentPlayers, data.positions,
      data.recentRanked, data.ability, data.rankedQueues, (tab?.nonRankedConfirmed || data.capabilities?.some(c=>c.name==="ranked-stats" && c.state==="available") && (data.ranks || []).every(r=>r.wins===0 && r.losses===0)) ? data.matches : null, data.seasonQueueStats, tab?.casualMode, tab?.opggSeason?.data,
      tab?.opggSeasonStale, tab?.opggSeasonPending, tab?.rankedQueueRecent,
      tab?.rankedQueueAbility, tab?.rankedQueuePosition, tab?.mayhemRating,tab?.loading,historyStatsPending(data,tab),matchesPending(tab),
    ].map((value) => {
      if (value && typeof value === "object") {
        if (!container._careerObjectTokens.has(value)) { container._careerObjectCounter = (container._careerObjectCounter || 0) + 1; container._careerObjectTokens.set(value, container._careerObjectCounter); }
        return `object:${container._careerObjectTokens.get(value)}`;
      }
      return `${typeof value}:${String(value)}`;
    }).join("|");
    const careerSections = sameOverview && container._careerSignature === careerSignature && container.querySelector(".career-column") ? "" : renderCareerSections(data, tab);
    const stripHTML = renderSummonerStrip(data,tab);
    const retainedStrip = sameOverview && container._stripHTML === stripHTML ? container.querySelector(".summoner-strip") : null;
    const retainedArt = !retainedStrip && sameOverview && container._backgroundArt === backgroundArt
      ? container.querySelector(".summoner-strip-art") : null;
    const retainedCareer = sameOverview && container._careerSignature === careerSignature ? container.querySelector(".career-column") : null;
    const oldCareerChildren = sameOverview && !retainedCareer ? [...(container.querySelector(".career-column")?.children || [])] : [];
    retainedStrip?.remove(); retainedCareer?.remove();
    retainedArt?.remove();
    retainedFilterbar?.remove();
    disposeMatchRowHeight(container);
      container.innerHTML = `
      ${quotaMessage}
      ${stripHTML}
      <div class="overview-layout${riotTab(tab)?' is-riot-overview':''}">
        <aside class="career-column" aria-label="生涯统计">
          ${careerSections}
        </aside>
        <section class="matches-column" aria-label="最近对局">
          ${matches.length ? renderHistoryServiceStatus(tab,true) : ""}
          <div class="overview-career-entry"><button class="career-dialog-trigger" type="button" aria-haspopup="dialog" aria-controls="career-dialog" data-open-career-dialog><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9m5 10V5m5 14v-7m5 7V8"/></svg><span>生涯统计</span></button></div>
          ${matchesPending(tab)?renderPendingMatchFilters(tab):renderMatchFilters(tab)}
          ${matchDetailsWarning}
          ${riotTab(tab) ? '<div data-current-game hidden></div>' : ""}
          <div class="match-list">${preserveMatchList ? "" : (matches.length ? (globalThis.deepLegendsHistoryFilters?.active(tab)?matches.slice(0,Number(tab.filteredVisibleCount || 20)):matches).map((match) => renderMatch(match, player.playerRef, tab)).join("") : emptyMatches)}</div>
          <div class="match-pagination${tab.loadingMore ? " is-loading" : ""}" data-match-sentinel aria-live="polite"${sentinelHidden}>${paginationCopy}</div>
        </section>
      </div>`;
    const newCareerChildren = [...(container.querySelector(".career-column")?.children || [])];
    const careerChildHTML = newCareerChildren.map(child => child.outerHTML);
    if (tab.loading && rawMatches.length && !matchesPending(tab)) {
      container.insertAdjacentHTML("beforeend", '<div class="overview-refreshing" role="status"><span class="mini-loading" aria-hidden="true"></span><span>正在刷新最新战绩，当前内容仍可查看</span></div>');
    }
    if (retainedFilterbar) {syncMatchFilterPending(retainedFilterbar,tab);container.querySelector(".match-filterbar")?.replaceWith(retainedFilterbar);}
    bindOverviewContent(container, tab);
    applyRenderedMetricStyles(container);
    if (preserveMatchList) {
      container.querySelector(".match-list")?.replaceWith(retainedMatchList);
      reconcileFilteredMatchList(retainedMatchList, tab);
    } else if (retainMatchListContainer) {
      const replacement = container.querySelector(".match-list");
      if (replacement) {
        retainedMatchList.replaceChildren(...replacement.childNodes);
        replacement.replaceWith(retainedMatchList);
      }
    }
    if (retainedStrip) container.querySelector(".summoner-strip")?.replaceWith(retainedStrip);
    if (retainedArt) container.querySelector(".summoner-strip-art")?.replaceWith(retainedArt);
    if (retainedCareer) container.querySelector(".career-column")?.replaceWith(retainedCareer);
    oldCareerChildren.forEach((child, index) => {
      if (container._careerChildHTML?.[index] === careerChildHTML[index]) newCareerChildren[index]?.replaceWith(child);
    });
    prepareImages(container);
    if(container.querySelector('[data-career-key]'))for(const [key,markup] of typeof careerSectionEntries==='function'?careerSectionEntries(data,tab):[]){const card=[...(container.querySelector('.career-column')?.children || [])].find(node=>node.dataset.careerKey===key);if(card)card._careerMarkup=markup;}
    container._careerChildHTML = careerChildHTML;
    container._stripHTML = stripHTML;
    container._backgroundArt = backgroundArt;
    container._careerSignature = careerSignature;
    container._careerHTML = careerSections;
    const renderedList = container.querySelector(".match-list");
    if (renderedList) {
      renderedList._matchData = new Map(rawMatches.map(match => [String(match.gameId), match]));
      if(!preserveMatchList){boundMatchDOM(renderedList,tab);reportMatchFilterView(tab,matches);}
      if(!preserveMatchList)window.reportFlowDiagnostic?.('lp_chip_render','render',{ranked_matches:matches.filter(m=>[420,440].includes(Number(m.queueId))).length,with_delta:matches.filter(m=>m.lpDelta!=null).length,chips:renderedList.querySelectorAll('.lp-delta').length,revision_skipped:0,source:"complete",lost:0,images_recreated:renderedList.querySelectorAll("img").length});
    }
    container._matchListMatches = rawMatches;
    container._matchListFilter = tab.matchFilter;
    container._matchListViewRevision = Number(tab.matchViewRevision || 0);
    reportOverviewCardReady(container,tab);
    updateFriendPresenceChips();
    if (tab.overviewCardLoad?.ready?.has("matches")) scheduleOverviewCurrentGame(tab);
    if (tab.overviewCardLoad?.ready?.has("matches") && ((data.matches || []).length || tab.currentGame?.data?.status === "active")) { ensurePerks(); ensureItems(); ensureSummonerSpells(); observeMatchTierVisibility(container, tab, tierScope); }
  }

  function reportDeferredMatchesCard(tab) {
    const load=tab.overviewCardLoad;
    if(!tab.current || !load || load.ready.has('matches') || !tab.data?.matches?.length || state.section==='overview' && activeTab(overviewGroupForSection())===tab)return;
    load.ready.add('matches');window.reportFlowDiagnostic?.('overview_card_ready','ready',{card:'matches',source:load.cached?'snapshot':'network',isSelf:true,overviewVisibleAtReady:false,startedAt:Date.now(),durationMs:Math.round(performance.now()-load.startedAt)});
  }

  function reportOverviewCardReady(container, tab) {
    const load=tab.overviewCardLoad,data=tab.data;
    if(!load || !data) return;
    const opgg=tab.opggSeason?.data;
    const position=rankedQueueData(data,tab,"position");
    const cards=[["matches",Array.isArray(data.matches) && (data.matches.length > 0 || tab.matchesReceived && data.pagination && !data.pagination.partial),"network"],["ranks",(data.ranks||[]).length,"network"],
      ["champions",opgg ? Number(opgg.overall?.games)>0 : Number(data.seasonStatsProgress?.scanned)>0 || (data.seasonChampionStats||data.championStats||[]).length,opgg ? "opgg" : data.seasonStatsProgress ? "snapshot" : "network"],
      ["masteries",(data.masteries||[]).length,"network"],
      ["positions",(position.positions||[]).length,"network"]];
    for(const [card,ready,source] of cards) if(ready && !load.ready.has(card) && container.querySelector(card === "matches" ? ".match-list" : ".career-column")) {
      load.ready.add(card);
      if(tab.current && container.isConnected && !load.selfReady) {
       load.selfReady = true;
       clearTimeout(tab.selfOverviewTimer); tab.selfOverviewTimer = 0;
       window.dispatchEvent(new CustomEvent("deep-legends:self-tab-ready"));
      }
      window.reportFlowDiagnostic?.("overview_card_ready","ready",{card,startedAt:Date.now(),isSelf:Boolean(tab.current),overviewVisibleAtReady:!tab.current || state.section==="overview" && activeTab(overviewGroupForSection())===tab,durationMs:Math.round((typeof performance !== "undefined" ? performance.now() : Date.now())-load.startedAt),source:load.cached ? "snapshot" : source});
    }
    if(load.ready.has("matches") && container.isConnected)void window.deepLegendsLoadHistoryFilters?.(true);
  }

  function friendPresenceForTab(tab) {
    if (!tab || riotTab(tab)) return null;
    // Match only server-scoped opaque references, never a shared display name.
    for (const ref of [tab.playerRef, ...(tab.playerRefs || []), tab.data?.player?.playerRef]) {
      const friend = state.friendPresence.get(ref);
      if (friend) return friend;
    }
    return null;
  }

  function friendGameDuration(started) {
    const seconds = Math.max(0, Math.floor((Date.now() - started) / 1000));
    const minutes = Math.floor(seconds / 60);
    const clock = minutes >= 60 ? `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, "0")}` : String(minutes);
    return `已进行 ${clock}:${String(seconds % 60).padStart(2, "0")}`;
  }

  function friendPresenceMarkup(tab) {
    const friend = friendPresenceForTab(tab);
    if (!friend || !["chat", "dnd"].includes(friend.availability) || friend.gameStatus !== "inGame"
      || (friend.product && friend.product !== "league_of_legends")) return "";
    const label = [friend.queueLabel || "游戏中", friend.championName].filter(Boolean).join(" · ");
    const started = Number(friend.gameStartedAt);
    const clock = Number.isFinite(started) && started > 0
      ? ` · <time data-player-game-start="${started}">${friendGameDuration(started)}</time>` : "";
    return `${escapeHTML(label)}${clock}`;
  }

  function updateFriendPresenceChips() {
    if (state.destroyed) return;
    const tabs = new Map([...state.tabs, ...state.overlay].map(tab => [tab.key, tab]));
    for (const node of document.querySelectorAll("[data-friend-presence]")) {
      const markup = friendPresenceMarkup(tabs.get(node.dataset.friendPresence));
      if (node.innerHTML !== markup) node.innerHTML = markup;
      node.hidden = !markup;
    }
  }

  window.addEventListener("deep-legends:friends-presence", (event) => {
    if (state.destroyed) return;
    const friends = Array.isArray(event.detail?.friends) ? event.detail.friends : [];
    state.friendPresence = new Map(friends.filter(friend => friend?.playerRef).map(friend => [friend.playerRef, friend]));
    updateFriendPresenceChips();
  });

  function currentGameMarkup(tab) {
    if (!riotTab(tab)) return "";
    const entry = tab.currentGame;
    if (!entry || entry.ref !== tab.data?.player?.playerRef) return "";
    // A failed probe is not evidence that this player is playing. Keep the
    // error state for refresh/retry logic, without a misleading standalone row.
    if (entry.error) return "";
    const game = entry.data;
    if (game?.status !== "active") return "";
    const elapsed = game.startedAt ? Math.max(0, Math.floor((Date.now() - Date.parse(game.startedAt)) / 1000)) : null;
    const clock = elapsed !== null && Number.isFinite(elapsed) ? `<time data-current-game-start="${escapeHTML(game.startedAt)}">已进行 ${Math.floor(elapsed / 60)}:${String(elapsed % 60).padStart(2, "0")}</time>` : "";
    const positions = { top: "上路", jungle: "打野", middle: "中路", bottom: "下路", utility: "辅助" };
    const mask = state.settings.maskNames;
    const playerRow = (player, index) => {
      const name = mask ? privacyName(player,{championName:player.championName,index,context:"live"}) : player.gameName || player.displayName || "隐藏玩家";
      const tag = !mask && player.tagLine ? `#${player.tagLine}` : "";
      const position = positions[player.preferredPosition];
      const positionTip = `首选位置：${position}`;
      const positionIcon = position ? `<span class="current-game-position" role="img" aria-label="${positionTip}" data-tooltip="${positionTip}"><img src="/position-icons/${player.preferredPosition}.svg" alt=""></span>` : "";
      const streakTip = player.streak === "win" ? "最近处于连胜状态" : player.streak === "loss" ? "最近处于连败状态" : "";
      const streakSVG = player.streak === "win"
        ? '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M13 2s1 4-2 7c-2.5 2.5-4 4.5-4 7a5 5 0 0 0 10 0c0-2-1-3.8-3-5.5.1 2-1 3.4-2 4.2.4-3.4-2-5.2-2-5.2"/></svg>'
        : '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m4 7 6 6 4-4 6 6"/><path d="M15 15h5v-5"/></svg>';
      const streak = streakTip ? `<span class="current-game-streak is-${player.streak}" role="img" aria-label="${streakTip}" data-tooltip="${streakTip}">${streakSVG}</span>` : "";
      const rank = player.rank ? `${rankCrestIcon(player.rank.tier)}<span><span>${escapeHTML(rankTitle(player.rank))}</span><small>${number(player.rank.leaguePoints)} LP</small></span>` : '<span class="current-game-missing">段位未提供</span>';
      const recent = (player.recent || []).map(recent => {
        const spellNames = state.summonerSpells ? (recent.spells || []).map(id => (state.summonerSpells.spells || []).find(candidate => Number(candidate.id) === Number(id))?.name).filter(Boolean) : [];
        const tooltip = [recent.championName || "英雄", recent.win === true ? "胜利" : recent.win === false ? "失败" : "胜负未提供", spellNames.length ? spellNames.join(" / ") : ""].filter(Boolean).join(" · ");
        return `<span class="current-game-recent-item${recent.win === true ? " is-win" : recent.win === false ? " is-loss" : ""}" data-tooltip="${escapeHTML(tooltip)}">${iconFigure("champion", recent.championId, recent.championName, "small", false)}</span>`;
      }).join("");
      const identity = player.playerRef && !mask ? `<button type="button" data-current-game-player="${escapeHTML(player.playerRef)}" ${proBadgeAttributes(player)} data-label="${escapeHTML(name + tag)}">${escapeHTML(name)}</button>` : `<strong>${escapeHTML(name)}</strong>`;
      return `<article class="current-game-player"><div class="current-game-player-line">${iconFigure("champion", player.championId, player.championName, "current-game-champion", false)}<div class="current-game-spells">${(player.spells || []).map(id => spellIconFigure(id, "small")).join("")}</div><div class="current-game-runes">${(player.runes || []).map((id, i) => i === 1 ? perkStyleIconFigure(id, "small") : perkIconFigure(id, "small")).join("")}</div><div class="current-game-identity"><span class="current-game-name-row">${identity}${renderProIdentityBadge(player)}</span><small>${player.summonerLevel > 0 ? `Lv.${number(player.summonerLevel)}` : "等级未提供"}</small></div></div>${recent ? `<div class="current-game-recent" aria-label="近期对局">${recent}</div>` : ""}<div class="current-game-badges">${positionIcon}${streak}</div><div class="current-game-rank">${rank}</div></article>`;
    };
    const markup = `<section class="current-game-card" aria-label="正在进行的游戏"><header><div><h3>正在进行的游戏</h3><span class="current-game-live">进行中</span><span>${escapeHTML(game.queue || "游戏模式未提供")}</span><span>${escapeHTML(game.map || "")}</span>${clock}</div><div class="current-game-actions">${opggSummonerURL(tab) ? '<button type="button" class="text-button" data-current-game-spectate="kr">前往OPGG观战</button>' : ""}<button type="button" class="text-button" data-current-game-retry aria-label="刷新当前对局">↻</button></div></header><div class="current-game-teams">${(game.teams || []).map((team, index) => `<section class="current-game-team is-${team.side === "blue" ? "blue" : "red"}"><h4>${team.side === "blue" ? "蓝色队伍" : "红色队伍"}${team.averageRank ? `<span>平均段位：${escapeHTML(rankTitle(team.averageRank))}${team.averageLP != null ? ` · ${number(team.averageLP)} LP` : ""}</span>` : ""}</h4>${(team.players || []).map((p, i) => playerRow(p, index * 5 + i)).join("")}${team.missingPlayers > 0 ? `<p class="current-game-roster-pending" role="status">还有 ${number(team.missingPlayers)} 位玩家信息暂未返回</p>` : ""}${!(team.players || []).length ? '<p class="current-game-roster-pending">该队阵容未提供</p>' : ""}</section>`).join("")}</div></section>`;
    return markup;
  }
  function updateCurrentGameCard(tab) {
    if (!riotTab(tab)) return;
    const container = overviewContainer(tab);
    const root = container?.querySelector("[data-current-game]");
    const traceId = tab.currentGame?.traceId || tab.currentGameTrace;
    const forceRefresh = tab.currentGame?.forceRefresh === true;
    if (!root || container.dataset.matchTierScope !== matchTierScope(tab)) {
      if (typeof window !== "undefined") window.reportFlowDiagnostic?.("current_game_client", !root ? "render-no-root" : "render-scope-mismatch", { traceId, forceRefresh });
      return;
    }
    try { root.innerHTML = currentGameMarkup(tab); } catch (error) {
      if (typeof window !== "undefined") window.reportFlowDiagnostic?.("current_game_client", "render-failed", { traceId, forceRefresh });
      throw error;
    }
    root.hidden = !root.innerHTML;
    if (typeof window !== "undefined") window.reportFlowDiagnostic?.("current_game_client", "rendered", { traceId, forceRefresh, hidden: root.hidden, source: tab.currentGame?.data?.source, phase: tab.currentGame?.error ? "error" : tab.currentGame?.data?.status, rendered100: root.querySelectorAll(".current-game-team.is-blue .current-game-player").length, rendered200: root.querySelectorAll(".current-game-team.is-red .current-game-player").length });
    root.querySelectorAll("[data-current-game-retry]").forEach(button => button.addEventListener("click", () => {
      button.disabled = true;
      void loadOverviewCurrentGame(tab, true).finally(() => { if (button.isConnected) button.disabled = false; });
    }));
    root.querySelector("[data-current-game-spectate]")?.addEventListener("click", () => {
      const url = opggSummonerURL(tab);
      if (url) window.open(url + "/ingame", "_blank", "noopener,noreferrer");
    });
    root.querySelectorAll("[data-current-game-player]").forEach(button => button.addEventListener("click", () => openPlayerOverlay({ playerRef: button.dataset.currentGamePlayer, label: button.dataset.label, region: tab.region || "", serverId: tabServerID(tab) })));
    prepareImages(root);
  }
  async function loadOverviewCurrentGame(tab, force = false) {
    if (tab.current && !connected() || !riotTab(tab) || document.hidden || !document.hasFocus() || !tab.overlay && (state.section !== "overview" || activeTab() !== tab)) return;
    let traceID = tab?.currentGameTrace;
    const report = (reason, fields = {}) => { if (typeof window !== "undefined") window.reportFlowDiagnostic?.("current_game_client", reason, { traceId: traceID, forceRefresh: force, ...fields }); };
    const target = overviewSupplementTarget(tab);
    const ref = target?.key;
    if (!ref || tab.data?.player?.privateHistory && riotTab(tab) || tab.externalRender || state.destroyed) { report("gated", { gate: !ref ? "no-reference" : state.destroyed ? "destroyed" : tab.externalRender ? "external-render" : "private-kr" }); return; }
    if (tab.currentGamePending === ref) { report("in-flight"); return; }
    if (!force && (tab.currentGame?.key || tab.currentGame?.ref) === ref && Date.now() - tab.currentGame.at < 30_000) { report("cached", { cacheAgeMs: Date.now() - tab.currentGame.at, phase: tab.currentGame.error ? "error" : tab.currentGame.data?.status }); return; }
    const requestGeneration = Number(tab.overviewRequestToken || 0);
    tab.currentGamePending = ref;
    state.currentGameTraceSequence = ((state.currentGameTraceSequence || 0) + 1) % 1000000;
    tab.currentGameTrace = `cg-${Date.now()}-${state.currentGameTraceSequence}`;
    traceID = tab.currentGameTrace;
    const started = Date.now();
    report("request", { forceRefresh: force });
    try {
      const data = await api("/api/gameplay/current-game", { method: "POST", body: JSON.stringify({ ...target.body, traceId: tab.currentGameTrace, forceRefresh: force }) }, `current-game:${tab.key}`, 14_000);
      if (!["active", "none", "unsupported"].includes(data?.status) || data.status === "active" && !Array.isArray(data.teams)) {
        report("invalid-response");
        const error = new Error("当前对局响应无效");
        error.errorKind = "invalid-response";
        throw error;
      }
      if (overviewSupplementTarget(tab)?.key !== ref || state.destroyed || tab.closed || tab.current && !connected() || Number(tab.overviewRequestToken || 0) !== requestGeneration) { report("stale", { gate: state.destroyed ? "destroyed" : "reference-changed" }); return; }
      tab.currentGame = { key: ref, ref: tab.data?.player?.playerRef || ref, at: Date.now(), data, traceId: traceID, forceRefresh: force };
      report("received", { phase: data.status, source: data.source, teamsReceived: (data.teams || []).length, durationMs: Date.now()-started, playersReceived: (data.teams || []).reduce((sum, team) => sum + (team.players || []).length, 0) });
      if (data.status === "active") { ensurePerks(); ensureSummonerSpells(); }
    } catch (error) {
      report(error.name === "RequestCancelled" ? "canceled" : "failed", { durationMs: Date.now()-started, httpStatus: error.status || 0, errorKind: error.errorKind || "other" });
      if (error.name !== "RequestCancelled" && overviewSupplementTarget(tab)?.key === ref && !state.destroyed && !tab.closed && (!tab.current || connected()) && Number(tab.overviewRequestToken || 0) === requestGeneration) tab.currentGame = { key: ref, ref: tab.data?.player?.playerRef || ref, at: Date.now(), error: true, traceId: traceID, forceRefresh: force };
    } finally {
      if (Number(tab.overviewRequestToken || 0) === requestGeneration) { if (tab.currentGamePending === ref) tab.currentGamePending = ""; updateCurrentGameCard(tab); }
    }
  }
  function scheduleOverviewCurrentGame(tab) {
    if (tab.current && !connected() || !riotTab(tab)) { clearTimeout(state.currentGameTimer); return; }
    updateCurrentGameCard(tab);
    if (tab.overlay || tab.externalRender || state.section !== "overview" || activeTab() !== tab || document.hidden || !document.hasFocus()) { clearTimeout(state.currentGameTimer); return; }
    clearTimeout(state.currentGameTimer);
    void loadOverviewCurrentGame(tab);
    void loadOPGGSeasonSummary(tab);
    state.currentGameTimer = setTimeout(() => {
      if (!state.destroyed && state.section === "overview" && activeTab() === tab && !document.hidden && document.hasFocus()) scheduleOverviewCurrentGame(tab);
    }, 60_000);
  }

  function highestCurrentRank(ranks) {
    const tierOrder = { iron: 1, bronze: 2, silver: 3, gold: 4, platinum: 5, emerald: 6, diamond: 7, master: 8, grandmaster: 9, challenger: 10 };
    const divisionOrder = { IV: 1, III: 2, II: 3, I: 4 };
    return (ranks || []).filter((rank) => rank?.tier).reduce((best, rank) => {
      if (!best) return rank;
      const rankTier = tierOrder[String(rank.tier).toLowerCase()] || 0;
      const bestTier = tierOrder[String(best.tier).toLowerCase()] || 0;
      if (rankTier !== bestTier) return rankTier > bestTier ? rank : best;
      const rankDivision = divisionOrder[String(rank.division).toUpperCase()] || 0;
      const bestDivision = divisionOrder[String(best.division).toUpperCase()] || 0;
      if (rankDivision !== bestDivision) return rankDivision > bestDivision ? rank : best;
      return Number(rank.leaguePoints || 0) > Number(best.leaguePoints || 0) ? rank : best;
    }, null);
  }

  function renderSummonerHighlights(ranks, masteries, status) {
    const highestRank = highestCurrentRank(ranks);
    const mastery = (masteries || []).reduce((best, item) => Number(item?.championPoints || 0) > Number(best?.championPoints || 0) ? item : best, null);
    const masteryLevel = Math.max(0, Math.min(10, Math.floor(Number(mastery?.championLevel) || 0)));
    const masteryCrestPath = `/latest/plugins/rcp-fe-lol-collections/global/default/images/item-element/crest-and-banner-mastery-${masteryLevel}.png`;
    const masteryTooltip = mastery ? `${mastery.championName || "英雄"} · 熟练度 ${number(mastery.championLevel)} 级\n${number(mastery.championPoints)} 熟练度点数` : "";
    const missing = (name, label, empty) => {
      if (!status) return "";
      const capability = status.capabilities.find(item => item.name === name);
      if(!capability && status.pending)return '<figure class="summoner-highlight is-pending-highlight"><span class="skeleton-square"></span><figcaption><span class="skeleton-line"></span><span class="skeleton-line"></span></figcaption></figure>';
      const text = capability?.state === "available" ? empty
        : !capability && status.pending ? "正在读取…" : "暂不可用";
      return `<figure class="summoner-highlight"><figcaption><strong>${label}</strong><small>${text}</small></figcaption></figure>`;
    };
    const masteryMarkup = mastery ? `<figure class="summoner-highlight is-mastery" data-tooltip="${escapeHTML(masteryTooltip)}" data-tooltip-size="compact">
      <span class="summoner-mastery-portrait" data-mastery-level="${masteryLevel}">${iconFigure("champion", mastery.championId, mastery.championName, "summoner-mastery-icon", false)}<img class="summoner-mastery-crest" data-queued-src="/api/champion-asset?source=communitydragon&path=${encodeURIComponent(masteryCrestPath)}" alt="" aria-hidden="true" loading="lazy" decoding="async" data-game-image></span>
      <figcaption><strong>${escapeHTML(mastery.championName || "英雄")}</strong><small>Lv.${number(mastery.championLevel)} · ${compactNumber(mastery.championPoints)} 点</small></figcaption>
    </figure>` : missing("champion-mastery", "英雄熟练度", "暂无熟练度");
    const queueLabel = highestRank?.queueLabel || (highestRank?.queueType === "RANKED_FLEX_SR" ? "灵活组排" : "单排/双排");
    const rankTooltip = highestRank ? `最高段位 · ${queueLabel}\n${rankTitle(highestRank)} · ${number(highestRank.leaguePoints)} LP` : "";
    const rankMarkup = highestRank ? `<figure class="summoner-highlight is-rank" data-tooltip="${escapeHTML(rankTooltip)}" data-tooltip-size="compact">
      <span class="summoner-highlight-rank" aria-hidden="true">${rankCrestIcon(highestRank.tier)}</span>
      <figcaption><strong>${escapeHTML(queueLabel)}</strong><small>${escapeHTML(rankTitle(highestRank))} ${number(highestRank.leaguePoints)} LP</small></figcaption>
    </figure>` : missing("ranked-stats", "当前段位", "未定级");
    if (!masteryMarkup && !rankMarkup) return "";
    return `<div class="summoner-strip-highlights">${masteryMarkup}${rankMarkup}</div>`;
  }

  function renderHistoricalRanks(items) {
    const available=(items || []).filter(item=>item?.season && item?.tier);
    const historyWinRate=item=>item.winRate!=null && Number(item.winRate)>=0?Number(item.winRate):item.wins!=null && item.losses!=null && Number(item.wins)>=0 && Number(item.losses)>=0 && Number(item.wins)+Number(item.losses)>0?100*Number(item.wins)/(Number(item.wins)+Number(item.losses)):null;
    const hasWinRate=available.some(item=>historyWinRate(item)!=null);
    const rows = available.map((item, index) => {
      const rate=historyWinRate(item),winRateKnown=rate!=null;
      const winRate = winRateKnown ? percent(rate) : "—";
      const leaguePoints = item.leaguePoints === null || item.leaguePoints === undefined ? "—" : number(item.leaguePoints);
      return `<div class="rank-history-row"${index >= 5 ? " data-rank-history-extra hidden" : ""}><span class="rank-season">${escapeHTML(item.season)}</span><span class="rank-history-tier"><span class="rank-history-crest" aria-hidden="true">${rankCrestIcon(item.tier)}</span><b>${escapeHTML(rankTitle(item))}</b><span>${leaguePoints} LP</span></span>${hasWinRate?`<b class="rank-history-winrate">${winRate}</b>`:""}</div>`;
    }).join("");
    if (!rows) return "";
    const expandable = items.filter((item) => item?.season && item?.tier).length > 5;
    const toggle = expandable ? `<button class="rank-history-toggle" type="button" aria-expanded="false" data-rank-history-toggle><span>查看更多赛段段位</span><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m7 10 5 5 5-5"/></svg></button>` : "";
    return `<div class="rank-history${hasWinRate?"":" no-winrate"}"><div class="rank-history-head"><span>赛段</span><span>段位与 LP</span>${hasWinRate?"<span>胜率</span>":""}</div><div class="rank-history-list">${rows}</div>${toggle}</div>`;
  }

  function renderRankMilestones(milestones, allowFlex=false) {
    const items = [];
    if (milestones?.peakTier) {
      const peak = { tier: milestones.peakTier, division: milestones.peakDivision };
      items.push(`<div class="rank-milestone"><span class="rank-milestone-crest" aria-hidden="true">${rankCrestIcon(peak.tier)}</span><div><small>历史最高</small><strong>${escapeHTML(rankTitle(peak))}</strong></div></div>`);
    }
    const previous = (milestones?.previousSeason || []).find((item) => (allowFlex || item?.queueType === "RANKED_SOLO_5x5") && item?.tier);
    if (previous) {
      const highest = previous.highestTier && String(previous.highestTier).toLowerCase() !== String(previous.tier).toLowerCase()
        ? `<span>最高 ${escapeHTML(rankTitle({ tier: previous.highestTier, division: previous.highestDivision }))}</span>`
        : "";
      items.push(`<div class="rank-milestone"><span class="rank-milestone-crest" aria-hidden="true">${rankCrestIcon(previous.tier)}</span><div><small>上赛季</small><strong>${escapeHTML(rankTitle(previous))}</strong>${highest}</div></div>`);
    }
    if (!items.length) return "";
    return `<div class="rank-milestones">${items.join("")}</div>`;
  }

  function renderRankHistory(historicalRanks, rankMilestones) {
    return renderHistoricalRanks(historicalRanks) || renderRankMilestones(rankMilestones);
  }

  function radarPoint(index, score, count = 7, radius = 90, centerX = 160, centerY = 144) {
    const angle = -Math.PI / 2 + (Math.PI * 2 * index) / count;
    const distance = radius * Math.max(0, Math.min(100, Number(score) || 0)) / 100;
    return [centerX + Math.cos(angle) * distance, centerY + Math.sin(angle) * distance];
  }

  function radarPoints(metrics, scoreOf) {
    return metrics.map((metric, index) => radarPoint(index, scoreOf(metric), metrics.length).map((value) => value.toFixed(1)).join(",")).join(" ");
  }

  function abilityMetricValue(metric, key) {
    if (metric?.unavailable || metric?.[key] == null) return "—";
    const value = Number(metric[key]);
    if (!Number.isFinite(value)) return "—";
    const digits = ["dpm", "gpm"].includes(metric.key) ? 0 : metric.key === "kda" ? 2 : 1;
    return `${value.toFixed(digits)}${metric.unit || ""}`;
  }

  function renderAbility(ability, fallbackQueueLabel = "", queueSwitcher = "", sampleGames = 0, queueGames = 0) {
    const metrics = Array.isArray(ability?.metrics) ? ability.metrics.filter((item) => item?.key) : [];
    const queueLabel = ability?.queueLabel || fallbackQueueLabel || "近期排位";
	const tools = queueSwitcher;
    if (metrics.length !== 7) {
      const playedQueue = Number(queueGames) > 0 || Number(sampleGames) > 0;
	  const title = ability?.historyPending ? "战绩未读全" : Number(sampleGames) > 0
		  ? `近期对局中${queueLabel}样本不足`
		  : playedQueue ? `暂无足够的${queueLabel}同位置样本` : `当前最近对局未发现${queueLabel}样本`;

      return `<section class="career-section ability-section"><header><h3>能力表现</h3><div class="career-section-tools">${tools}</div></header><div class="ability-unavailable"><strong>${escapeHTML(title)}</strong></div></section>`;
    }
    const rings = [25, 50, 75, 100].map((score) => `<polygon points="${radarPoints(metrics, () => score)}"/>`).join("");
    const axes = metrics.map((_, index) => {
      const [x, y] = radarPoint(index, 100, metrics.length);
      return `<line x1="160" y1="144" x2="${x.toFixed(1)}" y2="${y.toFixed(1)}"/>`;
    }).join("");
    const markers = metrics.map((metric, index) => {
      if (metric.unavailable) return "";
      const [x, y] = radarPoint(index, metric.playerScore, metrics.length);
      return `<circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="3"/>`;
    }).join("");
    const complete = metrics.every((metric) => !metric.unavailable);
    const baselineLabel = ability.baselineLabel || "近期同位置对手样本";
    const controls = metrics.map((metric, index) => {
      const tooltip = `${metric.label}\n当前玩家 ${abilityMetricValue(metric, "player")} · ${baselineLabel} ${abilityMetricValue(metric, "baseline")}\n`;
      return `<button class="ability-radar-metric is-metric-${index}" type="button" aria-label="${escapeHTML(tooltip.replaceAll("\n", "，"))}" data-tooltip="${escapeHTML(tooltip)}" data-tooltip-size="ability"><b>${escapeHTML(metric.grade)}</b><span>${escapeHTML(metric.label)}</span></button>`;
    }).join("");
    const summary = `${number(ability.sampleGames)} 场当前玩家 · ${number(ability.baselineGames)} 场同位置对手`;
    const sourceDetail = summary;
    return `<section class="career-section ability-section"><header><h3>能力表现</h3><div class="career-section-tools">${tools}</div></header><div class="ability-meta"><div class="ability-legend"><span><i class="is-player"></i>当前玩家</span><span><i class="is-baseline"></i>${escapeHTML(baselineLabel)}</span></div><span data-tooltip="${escapeHTML(sourceDetail)}" data-tooltip-size="compact">${escapeHTML(`${ability.positionLabel ? `${ability.positionLabel} · ` : ""}${number(ability.sampleGames)} 场样本`)}</span></div><div class="ability-radar-wrap"><svg class="ability-radar" viewBox="0 0 320 275" aria-hidden="true" focusable="false"><g class="ability-radar-grid">${rings}${axes}</g>${complete ? `<polygon class="ability-radar-player" points="${radarPoints(metrics, (metric) => metric.playerScore)}"/>` : ""}${complete ? `<polygon class="ability-radar-baseline" points="${radarPoints(metrics, () => 50)}"/>` : ""}<g class="ability-radar-points">${markers}</g></svg><div class="ability-radar-controls" role="group" aria-label="当前玩家与近期同位置对手样本的七维能力数据">${controls}</div></div></section>`;
  }

	function renderRecentRanked(stats, queueId = 0, queueSwitcher = "", partialHistory = false) {
	  const games = Number(stats?.games || 0);
	  const isMayhem = isMayhemQueueId(queueId);
	  const heading = isMayhem ? "近期海斗" : "近期排位";
	const tools = `<small class="history-sample-label">近 ${number(Math.min(20,games))} 场</small>`+queueSwitcher;
    if (!games) {
      const queueLabel = stats?.queueLabel || rankedQueueLabel(queueId);
	  const title = stats?.historyPending ? "战绩未读全" : `当前样本未发现${queueLabel}对局`;

	  return `<section class="career-section recent-ranked-section"><header><h3>${heading}</h3><div class="career-section-tools">${tools}</div></header><div class="ability-unavailable"><strong>${escapeHTML(title)}</strong></div></section>`;
    }
    const participationKnown = Number(stats.killParticipationGames || 0) > 0;
    const participation = participationKnown ? percent(stats.killParticipation) : "—";
    const participationTooltip = participationKnown ? "击杀参与率" : "击杀参与率暂不可用";
    const positions = (stats.positions || []).slice(0, 2).map((item) => `<span class="recent-ranked-position is-${escapeHTML(item.position || "other")}" data-tooltip="${escapeHTML(`${number(item.games)} 场 · ${number(item.wins)} 胜`)}" data-tooltip-size="compact"><b>${escapeHTML(item.label || positionLabel(item.position))}</b><strong>${percent(item.winRate)}</strong></span>`).join("");
    const positionRow = isMayhem
      ? ""
      : `<div class="recent-ranked-positions"><span>位置胜率</span><div>${positions || "<small>位置数据不足</small>"}</div></div>`;
    return `<section class="career-section recent-ranked-section">
	  <header><h3>${heading}</h3><div class="career-section-tools">${tools}</div></header>
      <div class="recent-ranked-content">
        <div class="recent-ranked-record"><div class="recent-ranked-ring" data-win-rate="${Number(stats.winRate) || 0}"><strong>${percent(stats.winRate)}</strong><span>胜率</span></div><small><b>${number(stats.wins)} 胜</b><i>${number(stats.losses)} 负</i></small></div>
        <dl class="recent-ranked-metrics">
          <div class="recent-ranked-metric" data-tooltip="每场平均击杀 / 死亡 / 助攻" data-tooltip-size="compact"><dt>平均 K / D / A</dt><dd><b>${number(stats.kills)}</b><i>/</i><b>${number(stats.deaths)}</b><i>/</i><b>${number(stats.assists)}</b></dd></div>
          <div class="recent-ranked-metrics-tail">
            <div class="recent-ranked-metric" data-tooltip="总击杀与总助攻之和除以总死亡数" data-tooltip-size="compact"><dt>KDA</dt><dd>${kda(stats.kda)}:1</dd></div>
            <div class="recent-ranked-metric" data-tooltip="${escapeHTML(participationTooltip)}" data-tooltip-size="compact"><dt>击杀参与率</dt><dd>${participation}</dd></div>
          </div>
        </dl>
      </div>
      ${positionRow}
    </section>`;
  }

  function mayhemRatingStatus(tab) {
    const playerRef = String(tab?.data?.player?.playerRef || tab?.playerRef || "").trim();
    const current = tab?.mayhemRating || {};
    if (current.playerRef && current.playerRef !== playerRef) return { status: "idle", playerRef };
    return { status: current.status || "idle", playerRef, data: current.data || null, error: current.error || "", at: current.at || 0 };
  }

  function renderRankMMRPopover(tab) {
    // R132 P4：ARAMKit 只收录国服，韩服总览不再放这个入口。
    if (riotTab(tab) || !tabServerID(tab)) return "";
    const rating = mayhemRatingStatus(tab);
    let mayhemValue = "准备查询";
    let mayhemDetail = "打开国服玩家总览后会自动读取第三方估算分";
    if (!rating.playerRef) {
      mayhemValue = "身份不可用";
      mayhemDetail = "当前页面没有可用于查询的玩家引用";
    } else if (rating.status === "loading") {
      mayhemValue = '<span class="rank-mmr-loading"><span class="mini-loading" aria-hidden="true"></span>查询中</span>';
      mayhemDetail = "正在读取第三方估算分";
    } else if (rating.status === "ready" && rating.data?.available) {
      const margin = Number(rating.data.marginOfError) > 0 ? `<small>±${number(rating.data.marginOfError)}</small>` : "";
      mayhemValue = `<strong>${number(rating.data.rating)}</strong>${margin}`;
      mayhemDetail = `第三方估算，非 Riot 官方匹配分${rating.data.matches?.length ? ` · 返回 ${number(rating.data.matches.length)} 场近期明细` : ""}`;
    } else if (rating.status === "ready") {
      mayhemValue = escapeHTML(rating.data?.unavailableReason || "未收录");
      mayhemDetail = "ARAMKit 未返回可核验的分数";
    } else if (rating.status === "error") {
      mayhemValue = '暂不可用 <button type="button" class="text-button" data-mayhem-rating-retry>重试</button>';
      mayhemDetail = rating.error || "第三方查询暂不可用";
    }
    return `<div class="rank-mmr-control">
      <button class="rank-mmr-trigger" type="button" aria-label="查看海斗隐藏分说明" aria-expanded="false" aria-haspopup="true" data-rank-mmr-trigger>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3 20 12 12 21 4 12Z"/><circle cx="12" cy="12" r="2.5"/></svg>
      </button>
      <div class="rank-mmr-popover" role="tooltip" aria-label="海斗隐藏分">
        <header><strong>海斗隐藏分</strong><span>第三方估算</span></header>
        <dl>
          <div class="is-mayhem" data-tooltip="${escapeHTML(mayhemDetail)}" data-tooltip-size="compact"><dt>海克斯大乱斗</dt><dd>${mayhemValue}</dd></div>
        </dl>

      </div>
    </div>`;
  }

  function renderRankWindowNote(progress) {
    const oldest=Number(progress?.rankedOldestAt),date=new Date(oldest);
    const truncated=progress?.upstreamCapped && Number.isFinite(oldest) && oldest>Date.UTC(2026,0,8) && Number.isFinite(date.getTime());
    const copy="统计的是当前整个赛季的排位数据"+(truncated?`\n官方接口仅提供 ${date.getMonth()+1}月${date.getDate()}日 之后的对局，更早的场次暂时无法计入`:"");
    return `<button type="button" class="season-data-note" aria-label="数据说明" data-tooltip="${escapeHTML(copy)}" data-tooltip-size="compact"><svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6.25"/><path d="M8 7v4M8 4.5v.5"/></svg></button>`;
  }

  function renderRanks(ranks, capabilities, historicalRanks, rankMilestones, seasonProgress = null, tab = null) {
    const rankedCapability = capabilities.find((item) => item.name === "ranked-stats");
    const mmr = renderRankMMRPopover(tab);
    if (!ranks.length && (['failed','error','canceled'].includes(rankedCapability?.state) || (tab?.initialPageError || tab?.error) && !tab?.loading)) {
      return `<section class="career-section rank-career-section"><header><h3>排位${renderRankWindowNote(seasonProgress)}</h3></header><div class="rank-unavailable"><strong>暂不可用</strong><button type="button" data-gameplay-retry>重试</button></div></section>`;
    }
    if (rankedCapability?.state === "unsupported") {
      return `<section class="career-section rank-career-section"><header><h3>排位${renderRankWindowNote(seasonProgress)}</h3><div class="rank-header-tools"><span>能力边界</span>${mmr}</div></header><div class="rank-unavailable"><strong>${escapeHTML(rankedCapability.detail || "跨服暂不支持排位")}</strong><small>当前客户端的排位接口只能读取登录服务器。</small></div></section>`;
    }
    const expected = ["RANKED_SOLO_5x5", "RANKED_FLEX_SR"];
    const rows = expected.map((queue) => {
      const rank = ranks.find((item) => item.queueType === queue);
      const label = queue === "RANKED_SOLO_5x5" ? "单排/双排" : "灵活组排";
      if ((!rank || !rank.tier) && !rankedCapability && tab?.loading) return `<div class="rank-row gameplay-skeleton"><span class="rank-crest"></span><div><span></span><span></span></div><div class="rank-record"><span></span><span></span></div></div>`;
      if (!rank || !rank.tier) return `<div class="rank-row"><span class="rank-crest is-unranked" aria-hidden="true">◇</span><div><strong>${label}</strong></div><b>未定级</b></div>`;
      const recordKnown = Number(rank.winRate) >= 0;
      const paused = !recordKnown && seasonProgress?.paused === true;
      const collecting = !recordKnown && !paused && seasonProgress?.collecting === true;
      const recordState = recordKnown ? "known" : paused ? "paused" : collecting ? "collecting" : "unavailable";
      if(tab) {
        tab.rankRecordStates ||= {};
        if(tab.rankRecordStates[queue]!==recordState) {
          tab.rankRecordStates[queue]=recordState;
          window.reportFlowDiagnostic?.("rank_record_state",recordState,{queue:queue==="RANKED_SOLO_5x5"?"solo":"flex",state:recordState});
        }
      }
      const record = recordKnown ? `${number(rank.wins)}胜 ${number(rank.losses)}负` : paused ? `${number(rank.wins)}胜 · 统计已暂停` : collecting ? `${number(rank.wins)}胜 · 正在统计中` : `${number(rank.wins)}胜 · 负场未提供`;
	  const unavailableDetail = paused ? "官方战绩服务暂时中断，恢复后自动继续统计" : collecting ? "正在统计中" : (rankedCapability?.detail || "上游未提供负场，无法计算胜率");
	  return `<div class="rank-row"><span class="rank-crest" aria-hidden="true">${rankCrestIcon(rank.tier)}</span><div><strong>${label}</strong><small>${escapeHTML(rankTitle(rank))} · ${number(rank.leaguePoints)} LP</small></div><div class="rank-record"><b>${record}</b><span${recordKnown ? "" : ` data-tooltip="${escapeHTML(unavailableDetail)}" data-tooltip-size="compact"`}>${recordKnown ? `胜率 <b class="win-rate-value">${percent(rank.winRate)}</b>` : collecting ? "正在统计中" : "胜率暂不可用"}</span></div></div>`;
    }).join("");
    return `<section class="career-section rank-career-section"><header><h3>排位${renderRankWindowNote(seasonProgress)}</h3><div class="rank-header-tools">${mmr}</div></header><div class="rank-list${rows.includes('gameplay-skeleton')?' career-pending':''}">${rows}</div>${riotTab(tab)?renderRankHistory(historicalRanks,null):renderRankMilestones(rankMilestones)}</section>`;
  }

  async function loadMayhemRating(tab, quietStart = false) {
    const current = mayhemRatingStatus(tab);
    const permanent = current.status === "ready" && (current.data?.available && Date.now()-Number(current.at || 0)<24*3600_000 || current.data?.unavailableReason === "未收录" && Date.now()-Number(current.at || 0)<600_000);
    if (!current.playerRef || current.status === "loading" || current.refreshing || permanent || riotTab(tab) || !tabServerID(tab)) return;
    if(current.status==='error')tab.mayhemRatingManual=true;
    const requestToken = Number(tab.mayhemRatingRequestToken || 0) + 1;
    tab.mayhemRatingRequestToken = requestToken;
    tab.mayhemRating = { status: current.data?.available ? "ready" : "loading", refreshing:true, playerRef: current.playerRef, data: current.data?.available ? current.data : null, error: "" };
    updateMayhemRatingPopover(tab);
    try {
      const params = new URLSearchParams({ playerRef: current.playerRef, ...(quietStart?{prefetch:"1"}:tab.mayhemRatingManual?{retry:"1"}:{}) });
      tab.mayhemRatingManual=false;
      const data = await api(`/api/gameplay/mayhem-rating?${params}`, {}, `mayhem-rating:${tab.key}`, 45000);
      if (tab.closed || tab.mayhemRatingRequestToken !== requestToken || mayhemRatingStatus(tab).playerRef !== current.playerRef) return;
      tab.mayhemRating = { status: data.available || data.unavailableReason === "未收录" ? "ready" : "error", playerRef: current.playerRef, data, error: "", at: Date.now() };
    } catch (error) {
      if (tab.closed || tab.mayhemRatingRequestToken !== requestToken) return;
      if (error.name === "RequestCancelled") { tab.mayhemRating = { status: "idle", playerRef: current.playerRef, data: null, error: "" }; updateMayhemRatingPopover(tab); return; }
      tab.mayhemRating = { status: "error", playerRef: current.playerRef, data: null, error: "查询暂不可用", at: Date.now() };
    }
    updateMayhemRatingPopover(tab);
  }

  function bindMayhemRatingControls(container,tab) {
    for (const control of container.querySelectorAll(".rank-mmr-control")) {
      control.dataset.ratingTab=tab.key;
      const trigger = control.querySelector("[data-rank-mmr-trigger]");
      const setExpanded = (expanded) => {trigger?.setAttribute("aria-expanded", String(expanded)); if(expanded && mayhemRatingStatus(tab).status !== "loading") void loadMayhemRating(tab);};
      control.querySelector("[data-mayhem-rating-retry]")?.addEventListener("click", () => {tab.mayhemRatingManual=true;tab.mayhemRating = {...mayhemRatingStatus(tab),status:"idle"};void loadMayhemRating(tab);});
      let hideTimer=0,pinned=false;
      const cancelHide=()=>clearTimeout(hideTimer);
      trigger?.addEventListener('click',event=>{event.stopPropagation();pinned=!pinned;cancelHide();setExpanded(pinned);});
      control.addEventListener("mouseenter", () => {cancelHide();setExpanded(true);});
      control.addEventListener("mouseleave", () => {cancelHide();if(!pinned)hideTimer=setTimeout(()=>setExpanded(false),150);});
      const outside=event=>{if(!control.isConnected){document.removeEventListener('pointerdown',outside);return;}if(!control.contains(event.target)){pinned=false;cancelHide();setExpanded(false);}};
      document.addEventListener('pointerdown',outside);
      control.addEventListener("focusin", () => setExpanded(true));
      control.addEventListener("focusout", (event) => { if (!pinned && !control.contains(event.relatedTarget)) setExpanded(false); });
    }
  }

  function updateMayhemRatingPopover(tab) {
    if(typeof document==="undefined") {rerenderTab(tab,"updateMayhemRatingPopover");return;}
    const template=document.createElement("template");template.innerHTML=renderRankMMRPopover(tab);
    const fresh=template.content.querySelector(".rank-mmr-popover");
    for(const control of document.querySelectorAll(".rank-mmr-control")) {
      if(control.dataset.ratingTab!==tab.key || !fresh)continue;
      control.querySelector(".rank-mmr-popover").innerHTML=fresh.innerHTML;
      control.querySelector("[data-mayhem-rating-retry]")?.addEventListener("click",()=>{tab.mayhemRatingManual=true;tab.mayhemRating={...mayhemRatingStatus(tab),status:"idle"};void loadMayhemRating(tab);});
    }
  }

  function updateCareerCards(tab, reason) {
    const container=overviewContainer(tab),root=(container?._overviewSubpageStash || container)?.querySelector('.career-column');if(!root || !tab.data)return;
    if(tab.advancedMenu?.open){tab.overviewRenderDeferred=true;globalThis.deepLegendsHistoryFilters?.refresh(container,tab,advancedFilterContext(tab));return;}
    const template=document.createElement('template'),changed=[];
    const entries=careerSectionEntries(tab.data,tab),keys=new Set(entries.map(([key])=>key));
    for(const child of [...root.children])if(!keys.has(child.dataset.careerKey))child.remove();
    for(const [index,[key,markup]] of entries.entries()){
      const old=[...root.children].find(node=>node.dataset.careerKey===key);
      if(old?._careerMarkup===markup)continue;
      template.innerHTML=markup;const fresh=template.content.firstElementChild;if(!fresh)continue;fresh._careerMarkup=markup;
      if(old?.outerHTML===fresh.outerHTML){old._careerMarkup=markup;continue;}
      if(old)old.replaceWith(fresh);else root.insertBefore(fresh,root.children[index] || null);changed.push(fresh);
    }
    container._careerSignature=null;
    for(const fresh of changed){bindOverviewContent(fresh,tab,container);applyRenderedMetricStyles(fresh);prepareImages(fresh);}
  }

  function renderSeasonDataNote(progress, queue, games, ranks = []) {
    // R257: explicitly requested copy; keep it here when cleaning old UI notes.
    const copy = {title:"数据说明", limit:"受官方接口限制，仅能查询到 {date} 之后的对局。", counted:"已统计 {games} 场", official:" · 官方总场次 {games} 场", future:"此后的新对局会自动累计。"};
    const oldest=Number(queue==="mayhem" ? progress?.mayhemOldestAt : progress?.rankedOldestAt);
    if (!progress?.upstreamCapped || progress.foreign || !Number.isFinite(oldest) || oldest<=0) return "";
    const date=new Date(oldest);
    if (!Number.isFinite(date.getTime())) return "";
    const dateLabel=`${date.getFullYear()===new Date().getFullYear() ? "" : `${date.getFullYear()}年`}${date.getMonth()+1}月${date.getDate()}日`;
    const type=queue==="440" ? "RANKED_FLEX_SR" : "RANKED_SOLO_5X5";
    const row=(ranks || []).find(rank=>String(rank.queueType).toUpperCase()===type);
    const official=Number.isInteger(row?.wins) && row.wins>=0 && Number.isInteger(row?.losses) && row.losses>=0 ? row.wins+row.losses : null;
    const counted=copy.counted.replace("{games}",String(games));
    const countLine=queue==="mayhem" ? counted : official!==null && official>=games ? counted+copy.official.replace("{games}",String(official)) : "";
    const lines=[copy.title,copy.limit.replace("{date}",dateLabel),countLine,copy.future].filter(Boolean);
    return `<button type="button" class="season-data-note" data-season-data-note="${escapeHTML(JSON.stringify(lines))}" aria-label="${copy.title}" aria-expanded="false"><svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6.25"/><path d="M8 7v4M8 4.5v.5"/></svg></button>`;
  }

  function bindSeasonDataNotes(container) {
    const doc=container.ownerDocument,win=doc.defaultView;
    if (doc._seasonDataNotes) return;
    // Reuse the R241 body portal, theme, shadow and arrow; one panel per document.
    const panel=doc.createElement("div");panel.className="match-tags-popover season-data-note-popover";panel.id="season-data-note-popover";
    panel.setAttribute("role","tooltip");panel.hidden=true;doc.body.append(panel);
    let anchor=null,touchUntil=0;
    const observer=new win.MutationObserver(()=>{if(anchor && !anchor.isConnected)close();});
    const close=()=>{observer.disconnect();if(anchor){anchor.setAttribute("aria-expanded","false");anchor.removeAttribute("aria-describedby");}anchor=null;panel.hidden=true;panel.replaceChildren();};
    const show=button=>{
      close();anchor=button;
      const lines=JSON.parse(button.dataset.seasonDataNote);
      panel.replaceChildren(...lines.map((line,index)=>{const node=doc.createElement(index===0 ? "strong" : "p");node.textContent=line;return node;}));
      panel.hidden=false;button.setAttribute("aria-expanded","true");button.setAttribute("aria-describedby",panel.id);
      const scale=Number(win.getComputedStyle(panel).zoom)||1;
      const rect=button.getBoundingClientRect(),width=panel.offsetWidth*scale,height=panel.offsetHeight*scale,padding=12,gap=8;
      const left=Math.max(padding,Math.min(rect.left,win.innerWidth-width-padding));
      const above=rect.bottom+gap+height>win.innerHeight-padding && rect.top>=height+gap+padding;
      panel.dataset.placement=above ? "top" : "bottom";
      panel.style.left=`${left/scale}px`;panel.style.top=`${Math.max(padding,above ? rect.top-height-gap : Math.min(rect.bottom+gap,win.innerHeight-height-padding))/scale}px`;
      panel.style.setProperty("--tag-arrow-left",`${Math.max(8,Math.min(width-8,rect.left+rect.width/2-left))/scale}px`);
      observer.observe(doc.body,{childList:true,subtree:true});
    };
    const buttonOf=node=>node?.closest?.("[data-season-data-note]");
    doc.addEventListener("pointerdown",event=>{if(event.pointerType==="touch")touchUntil=Date.now()+800;});
    doc.addEventListener("pointerover",event=>{const button=buttonOf(event.target);if(button && Date.now()>touchUntil)show(button);});
    doc.addEventListener("pointerout",event=>{if(anchor?.contains(event.target) && !anchor.contains(event.relatedTarget))close();});
    doc.addEventListener("focusin",event=>{const button=buttonOf(event.target);if(button && Date.now()>touchUntil)show(button);});
    doc.addEventListener("focusout",event=>{if(anchor?.contains(event.target))close();});
    doc.addEventListener("click",event=>{const button=buttonOf(event.target);if(button){event.stopPropagation();anchor===button ? close() : show(button);}else if(anchor && !panel.contains(event.target))close();},true);
    doc.addEventListener("keydown",event=>{if(event.key==="Escape")close();});
    doc.addEventListener("scroll",close,true);win.addEventListener("resize",close);win.addEventListener("blur",close);
    doc._seasonDataNotes={close};
  }

  function renderChampionStats(items, overall, progress, pending=false) {
    if (progress?.seasonOnly && progress.unavailable) return `<section class="career-section champion-performance"><header><h3>英雄胜率</h3><span>本赛季${progress.tableSupported ? overviewDetailArrow("champion-table", "查看英雄数据表") : ""}</span></header>${progress.message && progress.message !== "当前数据源不提供赛季统计" ? `<p class="section-empty">${escapeHTML(progress.message)}</p>` : ""}</section>`;
    if(pending)return renderCareerPendingMarkup(renderChampionStats(items,overall,progress));
    const rankedGames = (items || []).reduce((total, item) => total + Number(item.games || 0), 0);
    const tableGames = Number(overall?.games) || rankedGames;
    const overallRow = `<div class="champion-stat-row is-overall"><span class="overall-champion-mark" aria-hidden="true">全</span><div><strong>全部英雄</strong><small>CS ${number(overall.cs)} (${number(overall.csPerMinute)})</small></div><div><b>${kda(overall.kda)}:1 KDA</b><small>${number(overall.kills)} / ${number(overall.deaths)} / ${number(overall.assists)}</small></div><div><b class="win-rate-value">${percent(overall.winRate)}</b><small>${number(overall.games)} 场</small></div></div>`;
    const rows = items.slice(0, 8).map((item) => `<div class="champion-stat-row">${iconFigure("champion", item.championId, item.championName)}<div><strong>${escapeHTML(item.championName)}</strong><small>CS ${number(item.cs)} (${number(item.csPerMinute)})</small></div><div><b>${kda(item.kda)}:1 KDA</b><small>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}</small></div><div><b class="win-rate-value">${percent(item.winRate)}</b><small>${number(item.games)} 场</small></div></div>`).join("");
    const label = progress?.message ? `${progress.season ? `${escapeHTML(progress.season)} · ` : ""}${escapeHTML(progress.message)}` : (progress?.complete ? `本赛季 · ${number(rankedGames)} 场对局` : `已统计 ${number(rankedGames)} 场`);
    return `<section class="career-section champion-performance"><header><h3>英雄胜率</h3><span>${label}${progress?.tableSupported && (tableGames>=20 || progress.foreign) ? overviewDetailArrow("champion-table", "查看英雄数据表") : ""}</span></header><div>${overallRow}${rows || '<p class="section-empty">暂无英雄统计</p>'}</div></section>`;
  }

  function renderPositionStats(items, queueId = 0, queueSwitcher = "", sourceLabel = "", queueGames = 0, partialHistory = false) {
    const aliases = { top: "top", jungle: "jungle", middle: "middle", mid: "middle", bottom: "bottom", adc: "bottom", utility: "utility", support: "utility" };
    const values = new Map((items || []).map((item) => {
      const key = String(item?.position || "").trim().toLowerCase();
      return [aliases[key] || key, item];
    }));
    const positions = [
      ["top", "上单"],
      ["jungle", "打野"],
      ["middle", "中单"],
      ["bottom", "下路"],
      ["utility", "辅助"],
    ];
	const sampleLabel = !partialHistory && Number(queueGames) > 0 ? `<span class="career-sample-label">近 ${number(Math.min(20, queueGames))} 场</span>` : "";
	const tools = `${sampleLabel}${queueSwitcher}`;
    const hasSamples = [...values.values()].some((item) => Number(item?.games || 0) > 0 || Number(item?.share || 0) > 0);
    if (!hasSamples) {
      const queueLabel = sourceLabel || rankedQueueLabel(queueId);
	  const title = Number(queueGames)<0 ? "战绩未读全" : `暂无可统计的${queueLabel}位置样本`;

	  return `<section class="career-section"><header><h3>位置偏好</h3><div class="career-section-tools">${tools}</div></header><div class="ability-unavailable"><strong>${escapeHTML(title)}</strong></div></section>`;
    }
    const rows = positions.map(([position, label]) => {
      const share = Math.max(0, Math.min(100, Number(values.get(position)?.share) || 0));
      return `<div class="position-row"><span>${positionIcon(position)}</span><div><strong>${label}</strong><progress max="100" value="${share}">${percent(share)}</progress></div><b>${percent(share)}</b></div>`;
    }).join("");
	return `<section class="career-section"><header><h3>位置偏好</h3><div class="career-section-tools">${tools}</div></header><div class="position-list">${rows}</div></section>`;
  }

  function masteryMarkProgress(item) {
    if (item.markRequiredForNextLevel == null && !item.nextSeasonMilestone?.requireGradeCounts) return {slots:0,earned:0};
    let slots = Math.max(0, Math.min(20, Number(item.markRequiredForNextLevel) || 0));
    let earned = Math.min(slots, Math.max(0, Number(item.tokensEarned) || 0));
    // Observed Riot/OP.GG: tokensEarned is a bank; dot slots come from the next
    // season milestone grade requirements rather than marks needed for a level.
    const requirements = item.nextSeasonMilestone?.requireGradeCounts;
    if (requirements) {
      const order=["S+","S","S-","A+","A","A-","B+","B","B-","C+","C","C-","D+","D","D-"];
      const available=[...(item.milestoneGrades||[])]; slots=0;earned=0;
      for (const [grade,count] of Object.entries(requirements)) for(let i=0;i<Math.max(0,Math.min(20,Number(count)||0));i++) {
        slots++;
        const index=available.findIndex(value=>order.includes(value) && order.includes(grade) && order.indexOf(value)<=order.indexOf(grade));
        if(index>=0){earned++;available.splice(index,1);}
      }
    }
    return {slots:Math.min(20,slots),earned:Math.min(20,earned)};
  }

  function masteryMarks(item) {
    const {slots,earned}=masteryMarkProgress(item);
    return `<span class="mastery-marks" aria-label="${earned} / ${slots} 标记">${Array.from({length:slots},(_,i)=>`<svg class="mastery-mark${i < earned ? " is-earned" : ""}" viewBox="0 0 14 14" aria-hidden="true"><path d="M7 1 13 7 7 13 1 7Z"/></svg>`).join("")}</span>`;
  }

  function masteryGradeBoxes(item) {
    const {slots,earned}=masteryMarkProgress(item);
    return `<div class="mastery-grade-progress">${Array.from({length:slots},(_,i)=>`<b class="mastery-grade-box${i<earned ? " is-earned" : ""}">S</b>`).join("")}<span>里程碑 ${item.championSeasonMilestone ?? "—"}</span></div>`;
  }

  function masteryDetailsPortrait(item) {
    const level = Math.max(0,Math.min(10,Number(item.championLevel)||0));
    const path = `/latest/plugins/rcp-fe-lol-collections/global/default/images/item-element/crest-and-banner-mastery-${level}.png`;
    return `<span class="summoner-mastery-portrait mastery-details-portrait" data-mastery-level="${level}">${iconFigure("champion",item.championId,item.championName,"summoner-mastery-icon",false)}<img class="summoner-mastery-crest mastery-details-crest" data-queued-src="/api/champion-asset?source=communitydragon&path=${encodeURIComponent(path)}" alt="" aria-hidden="true" data-game-image><b class="mastery-details-level">${number(item.championLevel)}</b></span>`;
  }

  function masteryTooltipContent(item) {
    const order = ["S+","S","S-","A+","A","A-","B+","B","B-","C+","C","C-","D+","D","D-"];
    const grade = [...(item.milestoneGrades || [])].filter(g=>order.includes(g)).sort((a,b)=>order.indexOf(a)-order.indexOf(b))[0] || "—";
    const since = item.championPointsSinceLastLevel, until = item.championPointsUntilNextLevel;
    const required = since != null && until != null ? Math.max(0, Number(since)+Number(until)) : null;
    const date = Number(item.lastPlayTime)>0 ? new Date(Number(item.lastPlayTime)).toLocaleDateString("sv-SE") : "—";
    return `<header>${masteryDetailsPortrait(item)}<strong>${escapeHTML(item.championName)}</strong></header>${masteryGradeBoxes(item)}${required != null ? `<progress value="${Math.max(0,Number(since))}" max="${Math.max(1,required)}"></progress><p>${number(since)} / ${number(required)} 点</p>` : ""}<p>最近游玩：${date}</p>`;
  }

  function renderMasteryDetails(data) {
    const items = [...(data.items || [])].sort((a,b)=>Number(b.championPoints)-Number(a.championPoints));
    const summary = `<div class="overview-detail-summary"><div><small>总专精分数</small><strong>${number(data.totalScore)}</strong></div><div><small>总英雄分数</small><strong>${number(data.totalPoints)}</strong></div><div><small>专精英雄</small><strong>${number(items.length)} / ${number(data.totalChampions)}</strong></div></div>`;
    return `${summary}<div class="mastery-details-grid">${items.map(item=>{
      return `<article class="mastery-details-card" tabindex="0" data-mastery-detail="${item.championId}">${masteryDetailsPortrait(item)}<strong>${escapeHTML(item.championName)}</strong>${masteryMarks(item)}<small>${number(item.championPoints)}</small></article>`;
    }).join("")}</div>`;
  }

  function bindMasteryDetails(container, data) {
    for (const image of container.querySelectorAll(".mastery-details-crest")) image.addEventListener("error",()=>{image.hidden=true;image.parentElement.classList.add("is-crest-missing");});
    for (const card of container.querySelectorAll("[data-mastery-detail]")) {
      const item = (data.items || []).find(item=>String(item.championId)===card.dataset.masteryDetail);
      let popover;
      const close = () => {popover?.remove();popover=null;};
      const open = () => {
        close(); if (!item) return;
        popover=document.createElement("div");popover.className="mastery-details-popover";popover.setAttribute("role","tooltip");popover.innerHTML=masteryTooltipContent(item);document.body.append(popover);
        for (const image of popover.querySelectorAll(".mastery-details-crest")) image.addEventListener("error",()=>{image.hidden=true;image.parentElement.classList.add("is-crest-missing");});
        prepareImages(popover);
        const anchor=card.getBoundingClientRect(),box=popover.getBoundingClientRect();
        popover.style.left=`${Math.max(8,Math.min(window.innerWidth-box.width-8,anchor.left))}px`;
        popover.style.top=`${anchor.bottom+box.height+8<=window.innerHeight ? anchor.bottom+8 : Math.max(8,anchor.top-box.height-8)}px`;
      };
      card.addEventListener("mouseenter",open);card.addEventListener("mouseleave",close);card.addEventListener("focus",open);card.addEventListener("blur",close);
    }
  }

  function championTableNumber(value, digits = 1) {
    return typeof value === "number" && Number.isFinite(value) ? value.toFixed(digits) : "-";
  }

  function championTableSortedRows(data, tab) {
    const key = tab.championTableSort || "games", direction = tab.championTableDirection ?? -1;
    return [...(data.rows || [])].sort((a,b) => {
      const av=a[key],bv=b[key];
      if (av == null) return bv == null ? Number(a.championId)-Number(b.championId) : 1;
      if (bv == null) return -1;
      return direction*(Number(av)-Number(bv)) || (key === "games" ? Number(b.winRate || 0)-Number(a.winRate || 0) || Number(b.kda || 0)-Number(a.kda || 0) : 0) || Number(a.championId)-Number(b.championId);
    });
  }

  function renderChampionTable(data, tab) {
    const queue=data.queue || tab.championTableQueue || "420", mayhem=queue==="mayhem";
    const rows=championTableSortedRows(data,tab);
    if (tab.championTableExpandedId == null) tab.championTableExpandedId=!mayhem && rows[0] ? String(rows[0].championId) : "";
    if (!(tab.championTableOpponentLimits instanceof Map)) tab.championTableOpponentLimits=new Map();
    const n=championTableNumber, pct=value=>value==null ? "-" : `${n(value*100)}%`;
    const columns=[["games","场次"],["kda","KDA"],["score","评分"],["damagePerMinute","伤害"],["tankShare","承伤"]];
    if(!mayhem)columns.push(["controlWards","守卫"],["cs","补刀"]);
    columns.push(["gold","金币"]);
    const topValues = Object.fromEntries(["score","damagePerMinute","tankShare","controlWards","cs","gold","doubleKills","tripleKills","quadraKills","pentaKills"].map(key=>[key,new Set([...new Set(rows.map(row=>row[key]).filter(value=>Number.isFinite(value) && value>0))].sort((a,b)=>b-a).slice(0,3))]));
    const cell=(top,bottom="",tone="")=>`<td${tone ? ` class="${tone}"` : ""}><b>${top}</b>${bottom ? `<small>${bottom}</small>` : ""}</td>`;
    const rowMarkup=(row,index,kind="")=> {
      const id=String(row.championId),expanded=tab.championTableExpandedId===id,opponent=kind==="opponent";
      const multi=["doubleKills","tripleKills","quadraKills","pentaKills"];
      const highlight=key=>!kind && topValues[key]?.has(row[key]) ? "is-top-metric" : "";
      const expandable=!mayhem && !kind && row.championId && row.opponents?.length;
      return `<tr data-table-row="${kind || "champion"}:${id}" class="${kind ? `is-${kind}` : ""}"${expandable ? ` tabindex="0" data-table-expand="${id}" aria-expanded="${expanded}" aria-label="${escapeHTML(row.championName)}的对位"` : ""}><td>${opponent ? "" : index || ""}</td><th class="champion-table-hero" scope="row">${opponent ? '<span class="champion-table-opponent-arrow" aria-hidden="true">↳</span>' : ""}${row.championId ? iconFigure("champion",row.championId,row.championName,"small") : ""}<span>${escapeHTML(row.championName || "所有英雄")}</span></th>${cell(`${row.games}场 · ${n(row.winRate)}%`,`<span class="champion-table-winbar"><i data-table-winrate="${Math.max(0,Math.min(100,Number(row.winRate)||0))}"></i></span>${row.wins}胜 / ${row.losses}负`,row.winRate>=60 ? "is-hot" : "")}${cell(row.kda==null ? "-" : `${n(row.kda,2)}:1`,`${n(row.kills)} / ${n(row.deaths)} / ${n(row.assists)} · ${pct(row.kp)}`,row.kda>=3 ? "is-good" : "")}${cell(n(row.score),row.rank==null ? "-" : `#${n(row.rank)}`,highlight("score"))}${cell(n(row.damagePerMinute),pct(row.damageShare),highlight("damagePerMinute"))}${cell(pct(row.tankShare),"",highlight("tankShare"))}${mayhem ? "" : cell(`${n(row.controlWards)} (${n(row.wardsPlaced)}/${n(row.wardsKilled)})`,"",highlight("controlWards"))+cell(n(row.cs),n(row.csPerMinute),highlight("cs"))}${cell(n(row.gold,0),n(row.goldPerMinute),highlight("gold"))}${multi.map(key=>`<td class="table-multi-single ${highlight(key)}"><b>${n(row[key],0)}</b></td>`).join("")}<td class="table-multi-combined">${multi.map((key,i)=>`<b class="${highlight(key)}">${["双","三","四","五"][i]} ${n(row[key],0)}</b>`).join(" · ")}</td></tr>`;
    };
    const body=rowMarkup(data.overall || {championName:"所有英雄",games:0,wins:0,losses:0,winRate:0},0,"overall")+rows.map((row,index)=>{
      let content=rowMarkup(row,index+1);
      if(!mayhem && tab.championTableExpandedId===String(row.championId)) {
        const limit=tab.championTableOpponentLimits.get(String(row.championId)) || 5;
        content+=(row.opponents || []).slice(0,limit).map(opp=>rowMarkup(opp,0,"opponent").replace(`opponent:${opp.championId}`,`opponent:${row.championId}:${opp.championId}`)).join("");
        if((row.opponents || []).length>limit)content+=`<tr class="is-opponent" data-table-row="more:${row.championId}"><td></td><th class="champion-table-hero"><button type="button" class="text-button" data-table-more="${row.championId}">显示更多对位</button></th><td colspan="${columns.length+5}"></td></tr>`;
      }
      return content;
    }).join("");
    return `<nav class="champion-table-tabs" aria-label="英雄数据队列">${[["420","单排/双排"],["440","灵活组排"],["mayhem","海克斯大乱斗"]].map(([key,label])=>`<button type="button" data-table-queue="${key}" class="${key===queue ? "is-active" : ""}" aria-pressed="${key===queue}">${label}</button>`).join("")}</nav><div class="champion-table-wrap"><table class="champion-data-table"><thead><tr><th>#</th><th class="champion-table-hero">英雄</th>${columns.map(([key,label])=>`<th><button type="button" data-table-sort="${key}">${label}${(tab.championTableSort || "games")===key ? (tab.championTableDirection===1 ? " ↑" : " ↓") : ""}</button></th>`).join("")}${["双杀","三杀","四杀","五杀"].map(label=>`<th class="table-multi-single">${label}</th>`).join("")}<th class="table-multi-combined">多杀</th></tr></thead><tbody>${body}</tbody></table></div>${rows.length ? "" : '<p class="section-empty">本赛季没有该模式对局</p>'}`;
  }

  function championTableQueueTabs(queue) {
    return `<nav class="champion-table-tabs" aria-label="英雄数据队列">${[["420","单排/双排"],["440","灵活组排"],["mayhem","海克斯大乱斗"]].map(([key,label])=>`<button type="button" data-table-queue="${key}" class="${key===queue ? "is-active" : ""}" aria-pressed="${key===queue}">${label}</button>`).join("")}</nav>`;
  }

  function bindChampionTable(container, tab) {
    const body=container.querySelector("tbody"), rowCache=new Map();
    const remember=()=>{ for(const row of body?.children || [])rowCache.set(row.dataset.tableRow,row); };
    remember();
    const redraw=(anchor=null)=>{
      if(!body)return;
      const before=anchor?.getBoundingClientRect().top;
      const scroll=container.closest(".player-overlay-scroll") || document.getElementById("app-scroll");
      remember();
      const template=document.createElement("template");
      template.innerHTML=renderChampionTable(tab.overviewSubpageState.data,tab);
      const desired=[...template.content.querySelector("tbody").children];
      const inserted=[];
      const nodes=desired.map(row=>{
        const cached=rowCache.get(row.dataset.tableRow);
        if(cached) { if(row.hasAttribute("aria-expanded"))cached.setAttribute("aria-expanded",row.getAttribute("aria-expanded")); if(row.dataset.tableRow.startsWith("champion:"))cached.firstElementChild.textContent=row.firstElementChild.textContent; return cached; }
        rowCache.set(row.dataset.tableRow,row);inserted.push(row);return row;
      });
      // Move existing nodes, preserving queued/loaded images and their listeners.
      body.replaceChildren(...nodes);
      if(anchor && scroll && Number.isFinite(before))scroll.scrollTop+=anchor.getBoundingClientRect().top-before;
      for(const row of inserted){applyRenderedMetricStyles(row);prepareImages(row);}
      for(const button of container.querySelectorAll("[data-table-sort]")) {
        const key=button.dataset.tableSort;
        const fresh=template.content.querySelector(`[data-table-sort="${key}"]`);
        button.textContent=fresh.textContent;
      }
    };
    for(const button of container.querySelectorAll("[data-table-sort]"))button.addEventListener("click",()=>{const key=button.dataset.tableSort;tab.championTableDirection=(tab.championTableSort || "games")===key ? -(tab.championTableDirection ?? -1) : -1;tab.championTableSort=key;redraw();});
    const activate=event=>{
      const more=event.target.closest("[data-table-more]");
      if(more){const id=more.dataset.tableMore;tab.championTableOpponentLimits.set(id,(tab.championTableOpponentLimits.get(id)||5)+10);redraw(more.closest("tr"));return;}
      const row=event.target.closest("[data-table-expand]");if(!row)return;
      const id=row.dataset.tableExpand;tab.championTableExpandedId=tab.championTableExpandedId===id ? "" : id;redraw(row);
    };
    body?.addEventListener("click",activate);
    body?.addEventListener("keydown",event=>{if(["Enter"," "].includes(event.key) && event.target.matches("[data-table-expand]")){event.preventDefault();activate(event);}});
    for(const button of container.querySelectorAll("[data-table-queue]"))button.addEventListener("click",()=>{tab.championTableQueue=button.dataset.tableQueue;tab.championTableExpandedId=null;tab.championTableOpponentLimits=new Map();void openOverviewSubpage(tab,"champion-table");});
  }

  function bindOverviewSubpageLinks(root, tab, beforeOpen) {
    for(const button of root.querySelectorAll('[data-overview-subpage]')) {
      if(button._overviewSubpageBound)continue;button._overviewSubpageBound=true;
      button.addEventListener('click',async()=>{
        const page=button.dataset.overviewSubpage;let ok=false;
        if(beforeOpen)tab.overviewDialogReturn={from:'dialog',scrollTop:Number(root.scrollTop || 0),layout:root.dataset.careerLayout};
        else if(!tab.overviewSubpage)delete tab.overviewDialogReturn;
        try{beforeOpen?.();await openOverviewSubpage(tab,page);ok=tab.overviewSubpage===page && tab.overviewSubpageState?.status==='ready';}
        finally{window.reportFlowDiagnostic?.('overview_subpage_open','open',{page,from:beforeOpen?'dialog':'page',ok});}
      });
    }
  }

  async function openOverviewSubpage(tab, page) {
    const container=overviewContainer(tab),scroll=container?.closest(".player-overlay-scroll") || document.getElementById("app-scroll");
    if (!tab.overviewSubpage) tab.overviewReturnScroll=Number(scroll?.scrollTop||0);
    if(page==="champion-table" && !tab.championTableQueue) {
      const queues=tab.data?.rankedQueues || {};
      const solo=queues["420"],flex=queues["440"];
      const seasonGames=(type,queue)=>{
        const rank=(tab.data?.ranks||[]).find(row=>String(row.queueType).toUpperCase()===type);
        return Number.isFinite(rank?.wins) && Number.isFinite(rank?.losses) ? rank.wins+rank.losses : Number(queue?.recentRanked?.games||0);
      };
      tab.championTableQueue=seasonGames("RANKED_SOLO_5X5",solo)===0 && seasonGames("RANKED_FLEX_SR",flex)>0 ? "440" : "420";
    }
    tab.overviewSubpage=page;
    const token=Number(tab.overviewSubpageRequestToken||0)+1;tab.overviewSubpageRequestToken=token;
    tab.overviewSubpageState={status:"loading",page};rerenderTab(tab,"openOverviewSubpage");if (scroll) scroll.scrollTop=0;
    try {
      const data=await api(`/api/gameplay/${page}?playerRef=${encodeURIComponent(tab.data?.player?.playerRef || tab.playerRef || "")}${page==="champion-table" ? `&queue=${encodeURIComponent(tab.championTableQueue || "420")}` : ""}`,{},`overview-subpage:${tab.key}`,30000);
      if (tab.closed || tab.overviewSubpage!==page || tab.overviewSubpageRequestToken!==token) return;
      tab.overviewSubpageState={status:"ready",page,data};
    } catch(error) {
      if (tab.overviewSubpage!==page || tab.overviewSubpageRequestToken!==token) return;
      if (error.name==="RequestCancelled") { tab.overviewSubpageState={status:"idle",page}; return; }
      tab.overviewSubpageState={status:"failed",page,error:error.message};
    }
    rerenderTab(tab,"openOverviewSubpage");
  }

  function renderOverviewSubpage(container, tab) {
    if (!container._overviewSubpageStash) {
      const fragment=container.ownerDocument.createDocumentFragment();
      while(container.firstChild)fragment.appendChild(container.firstChild);
      container._overviewSubpageStash=fragment;
    }
    const current=tab.overviewSubpageState||{};
    const title=tab.overviewSubpage==="masteries" ? "英雄熟练度" : "英雄数据表";
    const note=tab.overviewSubpage==="champion-table" && current.status==="ready" && current.data?.available ? renderSeasonDataNote(current.data.seasonStatsProgress,current.data.queue || tab.championTableQueue || "420",Number(current.data.overall?.games || 0),tab.data?.ranks) : "";
    const heading=tab.overviewSubpage==="champion-table" ? `<div class="champion-table-title"><h2>${title}</h2>${note}</div>` : `<h2>${title}</h2>`;
    container.innerHTML=`<section class="overview-detail-page"><header><button type="button" class="text-button" data-overview-return><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m10 3-5 5 5 5"/></svg><span>返回总览</span></button>${heading}</header>${tab.overviewSubpage==="champion-table" && !(current.status==="ready" && current.data?.available) ? championTableQueueTabs(tab.championTableQueue || "420") : ""}${current.status==="ready" && current.data?.available ? (tab.overviewSubpage==="masteries" ? renderMasteryDetails(current.data) : renderChampionTable(current.data,tab)) : current.status==="loading" ? '<p class="muted">正在读取…</p>' : `<p class="section-empty">${escapeHTML(current.error || current.data?.detail || "详情读取失败")} <button type="button" class="text-button" data-subpage-retry>重试</button></p>`}</section>`;
    if(note)bindSeasonDataNotes(container);
    container.querySelector("[data-overview-return]")?.addEventListener("click",()=>{
      const dialogReturn=tab.overviewDialogReturn;delete tab.overviewDialogReturn;
      tab.overviewSubpage="";rerenderTab(tab,"renderOverviewSubpage");
      const scroll=container.closest(".player-overlay-scroll")||document.getElementById("app-scroll");if(scroll)scroll.scrollTop=Number(tab.overviewReturnScroll||0);
      document.querySelectorAll(".mastery-details-popover").forEach(node=>node.remove());
      const opener=container.querySelector('[data-open-career-dialog]');
      if(dialogReturn?.from==='dialog' && opener?.offsetParent && !tab.closed)openCareerDialog(container,tab,opener,dialogReturn);
    });
    container.querySelector("[data-subpage-retry]")?.addEventListener("click",()=>openOverviewSubpage(tab,tab.overviewSubpage));
    if(tab.overviewSubpage==="champion-table")bindChampionTable(container,tab);else if(current.status==="ready")bindMasteryDetails(container,current.data);
    applyRenderedMetricStyles(container);
    prepareImages(container);
  }

  function overviewDetailArrow(page, label) {
    return `<button type="button" class="overview-detail-arrow" data-overview-subpage="${page}" data-tooltip="查看全部" aria-label="${label}"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M6 3.5 10.5 8 6 12.5"/></svg></button>`;
  }

  function renderMasteries(items, supported = false, championCount = null) {
    const rows = items.map((item) => `<div class="mastery-row">${iconFigure("champion", item.championId, item.championName, "", false)}<div><strong>${escapeHTML(item.championName)}</strong><small>${compactNumber(item.championPoints)} 熟练度</small></div><b>Lv.${number(item.championLevel)}</b></div>`).join("");
    return `<section class="career-section mastery-career-section"><header><h3>英雄熟练度</h3><span>${Number.isFinite(championCount) ? `${number(championCount)} 个英雄` : ""}${supported ? overviewDetailArrow("masteries", "查看全部英雄熟练度") : ""}</span></header><div class="mastery-list">${rows || '<p class="section-empty">客户端未提供熟练度</p>'}</div></section>`;
  }

  function renderRecentPlayers(items, capability,pending=false,partialHistory=false) {
    if(pending)return '<section class="career-section"><header><h3>最近一起玩</h3></header><div class="ability-unavailable"><strong>战绩未读全</strong></div></section>';
    const rows = items.map((item, index) => {
      const label = maskedListName(item, index);
      const icon = state.settings.maskNames ? maskedProfileIcon() : iconFigure("profile", item.profileIconId, "");
      return `<button class="recent-player" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}" ${proBadgeAttributes(item)}` : "disabled"} data-tooltip="${escapeHTML(label)}" data-tooltip-overflow=".recent-player-name" data-tooltip-size="compact">${icon}<span><strong class="recent-player-name">${escapeHTML(label)}</strong><small>共同对局 ${number(item.games)} 场</small></span><span aria-hidden="true">›</span></button>`;
    }).join("");
    const emptyCopy = capability?.state === "failed"
      ? escapeHTML(capability.detail || "最近 30 天的参与者数据不完整，暂时无法生成可靠结果。")
      : "暂无重复同场玩家";
    return `<section class="career-section"><header><h3>最近一起玩</h3>${partialHistory ? "" : "<span>最近 30 天</span>"}</header><div class="recent-player-list">${rows || `<p class="section-empty">${emptyCopy}</p>`}</div></section>`;
  }

  function renderActivity(hours,pending=false) {
    if(pending)return '<section class="career-section activity-section"><header><h3>游戏时间分布</h3></header><div class="ability-unavailable"><strong>战绩未读全</strong></div></section>';
    const max = Math.max(1, ...hours.map(Number));
    const cells = Array.from({ length: 24 }, (_, hour) => {
      const value = Number(hours[hour] || 0);
      const intensity = value ? Math.max(1, Math.ceil(value / max * 4)) : 0;
      return `<span class="activity-cell level-${intensity}" data-tooltip="${hour}:00 · ${value} 场" data-tooltip-size="compact"><b>${hour}</b></span>`;
    }).join("");
    return `<section class="career-section activity-section"><header><h3>游戏时间分布</h3><span>本地时间</span></header><div class="activity-legend"><span>上午</span><span>下午</span></div><div class="activity-grid">${cells}</div></section>`;
  }

  function overviewShareIcon(kind) {
    if (kind === "loading") return '<span class="mini-loading" aria-hidden="true"></span>';
    if (kind === "success") return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 4 4L19 6"/></svg>';
    if (kind === "error") return '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v6m0 4h.01"/></svg>';
    return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5M12 15V3"/></svg>';
  }

  function renderOverviewShareButton() {
    const status = OVERVIEW_SHARE_STATUS[state.overviewShareStatus] ? state.overviewShareStatus : "idle";
    const [label, icon] = OVERVIEW_SHARE_STATUS[status];
    return `<button class="overview-share-action is-${status}" type="button" data-generate-overview-share aria-live="polite"${["choosing", "rendering"].includes(status) ? " disabled" : ""}>${overviewShareIcon(icon)}<span>${label}</span></button>`;
  }

  function setOverviewShareStatus(status) {
    state.overviewShareStatus = OVERVIEW_SHARE_STATUS[status] ? status : "idle";
    const [label, icon] = OVERVIEW_SHARE_STATUS[state.overviewShareStatus];
    for (const button of document.querySelectorAll("[data-generate-overview-share]")) {
      button.className = `overview-share-action is-${state.overviewShareStatus}`;
      button.disabled = ["choosing", "rendering"].includes(state.overviewShareStatus);
      button.innerHTML = `${overviewShareIcon(icon)}<span>${label}</span>`;
    }
  }

  function resetOverviewShareStatus(delay = 0) {
    clearTimeout(state.overviewShareResetTimer);
    state.overviewShareResetTimer = setTimeout(() => setOverviewShareStatus("idle"), delay);
  }

  function overviewShareFileName(tab) {
    const player = tab?.data?.player || {};
    const playerName = String(state.settings.maskNames ? privacyName(player,{context:"identity"}) : player.gameName || player.displayName || "overview")
      .replace(/[<>:"/\\|?*\x00-\x1f]/g, "-")
      .replace(/[. ]+$/g, "")
      .trim()
      .slice(0, 48) || "overview";
    return `Deep-Legends-${playerName}-${new Date().toISOString().slice(0, 10)}.png`;
  }

  function createOverviewShareMarkup(container, matchLimit = state.settings.matchCount) {
    if (!container?.querySelector(".summoner-strip") || !container.querySelector(".overview-layout")) throw new Error("当前总览还没有可分享的内容。");
    const clone = container.cloneNode(true);
    clone.removeAttribute("id");
    clone.removeAttribute("aria-live");
    clone.classList.add("overview-share-content");
    for (const node of clone.querySelectorAll("[id]")) node.removeAttribute("id");
    for (const node of clone.querySelectorAll("[style]")) node.removeAttribute("style");
    for (const node of clone.querySelectorAll(".overview-share-action, .overview-career-entry, .overview-refreshing, .global-tooltip")) node.remove();
    for (const node of clone.querySelectorAll("[data-tooltip]")) node.removeAttribute("data-tooltip");
    const limit = normalizeMatchCount(matchLimit);
    const matchEntries = [...clone.querySelectorAll(".match-list > .match-entry")];
    for (const entry of matchEntries.slice(limit)) entry.remove();
    clone.querySelector(".match-pagination")?.remove();

    const surface = document.createElement("div");
    surface.className = "overview-share-surface";
    surface.append(clone);
    const watermark = document.createElement("footer");
    watermark.className = "overview-share-watermark";
    watermark.textContent = "DEEP LEGENDS";
    surface.append(watermark);
    return surface.outerHTML;
  }

  async function generateOverviewShare(container, tab) {
    if (["choosing", "rendering"].includes(state.overviewShareStatus)) return;
    clearTimeout(state.overviewShareResetTimer);
    const bridge = window.desktopShare;
    if (!bridge?.preparePngSave || !bridge?.captureAndSavePng) {
      setOverviewShareStatus("error");
      showToast("请在 Deep Legends 桌面客户端中生成分享图");
      resetOverviewShareStatus(2600);
      return;
    }
    setOverviewShareStatus("choosing");
    try {
      // 首次生成先选择并记住目录；之后直接分配不重名的文件路径。
      const destination = await bridge.preparePngSave(overviewShareFileName(tab));
      if (destination?.canceled) {
        setOverviewShareStatus("idle");
        return;
      }
      if (destination?.directory) {
        window.dispatchEvent(new CustomEvent("deep-legends:share-directory-changed", { detail: { directory: destination.directory } }));
      }
      setOverviewShareStatus("rendering");
      const result = await bridge.captureAndSavePng({
        token: destination?.token,
        markup: createOverviewShareMarkup(container),
        theme: document.documentElement.dataset.theme || "",
        density: document.documentElement.dataset.density || "",
      });
      if (!result?.ok) throw new Error("桌面客户端未能保存分享图。");
      setOverviewShareStatus("success");
      const dimensions = result.pixelWidth && result.pixelHeight ? ` · ${result.pixelWidth} × ${result.pixelHeight}` : "";
      showToast(`分享图已保存${dimensions}`);
      resetOverviewShareStatus(3000);
    } catch (error) {
      setOverviewShareStatus("error");
      showToast(error?.message || "分享图生成失败，请重试");
      resetOverviewShareStatus(3000);
    }
  }

  function bindOverviewShareControls(root, container, tab) {
    for (const button of root?.querySelectorAll("[data-generate-overview-share]") || []) {
      if(button._overviewShareBound)continue;
      button._overviewShareBound=true;
      button.addEventListener("click", () => generateOverviewShare(container, tab));
    }
  }

  function closeCareerDialog() {
    state.careerMasonryObserver?.disconnect();
    if (nodes.careerDialog?.open) nodes.careerDialog.close();
  }

  function renderCareerDialogContent(data, tab, layout, force = false) {
    if (!nodes.careerDialogContent || !force && nodes.careerDialogContent.dataset.careerLayout === layout && nodes.careerDialogContent._careerData === data) return;
    nodes.careerDialogContent._careerData = data;
    const scrollTop = nodes.careerDialogContent.scrollTop;
    nodes.careerDialogContent.innerHTML = layout === "single" ? renderCareerSections(data, tab) : renderCareerDialogSections(data, tab);
    nodes.careerDialogContent.dataset.careerLayout = layout;
    bindPlayerLinks(nodes.careerDialogContent, tab, closeCareerDialog);
    bindOverviewSubpageLinks(nodes.careerDialogContent,tab,closeCareerDialog);
    bindOverviewShareControls(nodes.careerDialogContent, overviewContainer(tab), tab);
    bindRankHistoryControls(nodes.careerDialogContent);
    bindRankedQueueControls(nodes.careerDialogContent, tab);
    bindMayhemRatingControls(nodes.careerDialogContent,tab);
    applyRenderedMetricStyles(nodes.careerDialogContent);
    prepareImages(nodes.careerDialogContent);
    nodes.careerDialogContent.scrollTop = scrollTop;
    state.careerMasonryObserver?.disconnect();
    let frame=0;
    const schedule=()=>{if(frame)return;frame=requestAnimationFrame(()=>{frame=0;distributeCareerDialog();});};
    if(typeof ResizeObserver==='function') {
      state.careerMasonryObserver=new ResizeObserver(schedule);
      for(const item of nodes.careerDialogContent.querySelectorAll('.career-dialog-item'))state.careerMasonryObserver.observe(item);
    }
    schedule();
  }

  function openCareerDialog(container, tab, opener, returning = null) {
    if (!nodes.careerDialog || !nodes.careerDialogContent || !tab.data || nodes.careerDialog.open) return;
    const data = tab.data;
    const returnScroll=Math.max(0,Number(returning?.scrollTop || 0));
    const player = data.player || {};
    const playerIndex = Math.max(1, state.tabs.findIndex((item) => item.key === tab.key));
    const maskProfile = state.settings.maskNames;
    const profileName = maskProfile ? privacyName(player,{index:playerIndex,context:"identity"}) : (player.gameName || player.displayName || "隐藏玩家");
    const riotID = !maskProfile && player.tagLine ? `${profileName}#${player.tagLine}` : profileName;

    state.careerDialogTab = tab;
    state.careerDialogOpener = opener;
    nodes.careerDialogContext.textContent = `${riotID} · ${tabServerLabel(tab)}`;
    nodes.careerDialogContent.dataset.careerLayout = "";
    nodes.careerDialogContent.scrollTop = 0;

    const syncWidth = () => {
      const width = container.querySelector(".overview-layout")?.clientWidth || container.clientWidth;
      if (width <= 0) return;
      if (nodes.careerDialog.open && !container.querySelector("[data-open-career-dialog]")?.offsetParent) {
        closeCareerDialog();
        return;
      }
      const roundedWidth = Math.round(width);
      nodes.careerDialog.style.setProperty("--career-dialog-width", `${roundedWidth}px`);
      const dialogWidth = Math.min(roundedWidth, Math.max(0, (window.innerWidth - 24) / (Number(getComputedStyle(document.documentElement).getPropertyValue("--ui-zoom")) || 1)));
      renderCareerDialogContent(data, tab, dialogWidth <= 520 ? "single" : "masonry");
    };
    state.careerDialogObserver?.disconnect();
    state.careerDialogObserver = "ResizeObserver" in window ? new ResizeObserver(syncWidth) : null;
    state.careerDialogObserver?.observe(container);
    syncWidth();
    nodes.careerDialog.showModal();
    nodes.careerDialogContent.scrollTop = returnScroll;
    requestAnimationFrame(()=>{if(nodes.careerDialog.open){distributeCareerDialog();nodes.careerDialogContent.scrollTop=returnScroll;}});
    window.desktopTheme?.setModalOpen?.(true);
    requestAnimationFrame(() => nodes.careerDialogClose?.focus({ preventScroll: true }));
  }

  // “更多模式”固定清单（对照 OP.GG 的模式下拉）。海选赛是国服独有
  // 队列，因此韩服页签会在渲染和筛选时同时移除该项。
  const MORE_MODE_OPTIONS = [
    ["aram", "极地大乱斗", null],
    ["hextech-qualifier", "海克斯大乱斗 海选赛", null],
    ["match", "匹配模式", null],
    ["hextech-classic", "海克斯大乱斗 经典模式版", null],
    ["bots", "人机对战", null],
    ["urf", "无限火力", null],
    ["clash", "冠军杯赛", null],
    ["nexus-blitz", "极限闪击", null],
    ["doombots", "末日人工智能", null],
    ["special", "特殊模式", null],
  ];

  function moreModeOptions(tab) {
    return riotTab(tab) ? MORE_MODE_OPTIONS.filter(([key]) => key !== "hextech-qualifier") : MORE_MODE_OPTIONS;
  }

  function matchQueueLabel(match) {
    return String(match?.queueLabel || "").trim();
  }

  function queueDefinitionFor(value) {
    const queueID = Number(value?.queueId ?? value);
    if (!Number.isFinite(queueID) || queueID <= 0) return null;
    return (state.queueGroups || []).find((item) => Number(item?.id) === queueID) || null;
  }

  function queueModeGroup(value) {
    return String(value?.modeGroup || queueDefinitionFor(value)?.modeGroup || "").toLowerCase();
  }

  function isHextechClassic(match) {
    const modeGroup = String(match?.modeGroup || "").toLowerCase();
    const gameMode = String(match?.gameMode || "").toUpperCase();
    const label = matchQueueLabel(match);
    return modeGroup === "hextech-classic"
      || gameMode === "ARAM_MAYHEM_CLASSIC"
      || label.includes("经典模式版")
      || /(?:aram mayhem|hextech aram).*classic/i.test(label);
  }

  function isHextechQualifier(match) {
    const modeGroup = String(match?.modeGroup || "").toLowerCase();
    const label = matchQueueLabel(match);
    return modeGroup === "hextech-qualifier"
      || label.includes("海选赛")
      || /(?:aram mayhem|hextech aram).*qualifier/i.test(label);
  }

  function isOrdinaryHextechMatch(match) {
    if (isHextechClassic(match) || isHextechQualifier(match)) return false;
    const modeGroup = queueModeGroup(match);
    const gameMode = String(match?.gameMode || "").toUpperCase();
    const mapID = Number(match?.mapId);
    const hasMayhemShape = ["ARAM_MAYHEM", "KIWI"].includes(gameMode) && (!mapID || mapID === 12);
    return queueDefinitionFor(match)?.augmentSource === "hextech" || modeGroup === "hextech-aram" || hasMayhemShape;
  }

  // 与 OP.GG 当前战绩组件保持一致：竞技场只展示海克斯，海克斯大乱斗
  // 展示召唤师技能与海克斯，经典模式版及其他模式展示技能与符文。
  function matchModeKind(match, subject) {
    const modeGroup = queueModeGroup(match);
    const gameMode = String(match?.gameMode || "").toUpperCase();
    if (modeGroup === "arena" || queueDefinitionFor(match)?.augmentSource === "arena" || ["ARENA", "CHERRY"].includes(gameMode) || Number(subject?.subteamId) > 0) return "arena";
    if (isHextechClassic(match)) return "mayhem-classic";
    if (isOrdinaryHextechMatch(match) || isHextechQualifier(match)) return "mayhem";
    if (modeGroup === "aram" || gameMode === "ARAM") return "aram";
    return "standard";
  }

  function isRecognizedModeMatch(match) {
    const modeGroup = queueModeGroup(match);
    const gameMode = String(match?.gameMode || "").toUpperCase();
    if (matchModeKind(match) !== "standard") return true;
    if (["solo", "flex", "match", "bots", "urf", "clash", "nexus-blitz", "doombots"].includes(modeGroup)) return true;
    if (queueDefinitionFor(match)) return true;
    if (MORE_MODE_OPTIONS.some(([key]) => key === modeGroup)) return true;
    return ["URF", "ARURF", "NEXUSBLITZ"].includes(gameMode);
  }

  function renderMatchFilters(tab) {
    const direct = [["all", "全部"], ["solo", "单排/双排"], ["flex", "灵活组排"], ["hextech-aram", "海克斯大乱斗"], ["arena", "斗魂竞技场"]];
    const modeOptions = moreModeOptions(tab);
    const selectedMore = String(tab.matchFilter).startsWith("more:") ? modeOptions.find(([key]) => tab.matchFilter === `more:${key}`) : null;
    const moreActive = Boolean(selectedMore);
    const triggerLabel = selectedMore?.[1] || "更多模式";
    const options = modeOptions.map(([key, label]) => {
      const selected = tab.matchFilter === `more:${key}`;
      return `<button type="button" role="menuitemradio" aria-checked="${selected}" data-app-select-value="more:${key}"><span>${escapeHTML(label)}</span><span class="app-select-check" aria-hidden="true">✓</span></button>`;
    }).join("");
	return `<div class="match-filterbar"><div class="match-filter-tabs" role="group" aria-label="游戏类型">${direct.map(([value, label]) => `<button class="match-filter${tab.matchFilter === value ? " is-active" : ""}" type="button" aria-pressed="${tab.matchFilter === value}" data-match-filter="${value}">${label}</button>`).join("")}</div><div class="match-filter-tools">${globalThis.deepLegendsHistoryFilters?.render(tab, () => advancedFilterContext(tab)) || '<button class="app-select-trigger" type="button" data-history-filter-load>筛选</button>'}<div class="app-select match-more-filter${moreActive ? " is-active" : ""}" data-match-more-select><button class="app-select-trigger" type="button" aria-haspopup="menu" aria-expanded="false" data-app-select-trigger><span>${escapeHTML(triggerLabel)}</span></button><div class="app-select-menu" role="menu" aria-label="更多游戏类型" data-app-select-menu hidden>${options}</div></div></div>${globalThis.deepLegendsHistoryFilters?.renderConditions(tab, () => advancedFilterContext(tab)) || ""}</div>`;
  }

  function matchFilterDisplayLabel(tab) {
	const direct = { all: "全部", solo: "单排/双排", flex: "灵活组排", "hextech-aram": "海克斯大乱斗", arena: "斗魂竞技场" };
	const filter = String(tab?.matchFilter || "all");
	if (direct[filter]) return direct[filter];
	if (filter.startsWith("more:")) return moreModeOptions(tab).find(([key]) => filter === `more:${key}`)?.[1] || "所选模式";
	return "所选模式";
  }

  function filteredMatches(matches, tab) {
    const apply = rows => globalThis.deepLegendsHistoryFilters?.active(tab) ? globalThis.deepLegendsHistoryFilters.filter(rows, tab, advancedFilterContext(tab)) : rows;
    // 后端会统一剔除自定义对局；这里保留防御性过滤，避免旧缓存或演示数据
    // 把 queueId=0 / CUSTOM_GAME 混入统计和“全部”列表。
    matches = matches.filter((match) => {
      const gameType = String(match.gameType || "").toUpperCase();
      const gameMode = String(match.gameMode || "").toUpperCase();
      return Number(match.queueId) !== 0
        && !["CUSTOM", "CUSTOM_GAME"].includes(gameType)
        && !["CUSTOM", "CUSTOM_GAME"].includes(gameMode)
        && String(match.modeGroup || "").toLowerCase() !== "custom";
    });
    const filter = String(tab.matchFilter || "all");
    if (filter === "all") return apply(matches);
    if (filter.startsWith("more:")) {
      const key = filter.slice(5);
      const option = moreModeOptions(tab).find(([value]) => value === key);
      if (!option) return apply([]);
      if (key === "aram") return apply(matches.filter((match) => matchModeKind(match) === "aram"));
      if (key === "hextech-qualifier") return apply(matches.filter(isHextechQualifier));
      if (key === "hextech-classic") return apply(matches.filter(isHextechClassic));
      if (option[2]) return apply(matches.filter((match) => option[2].includes(Number(match.queueId)) || queueModeGroup(match) === key));
      if (key !== "special") return apply(matches.filter((match) => queueModeGroup(match) === key));
      // 特殊模式只承接真正未识别的模式，不能再收纳带经典海克斯等
      // 明确语义、但后端旧缓存仍标成 other 的对局。
      return apply(matches.filter((match) => !isRecognizedModeMatch(match)));
    }
    // 兼容旧版 queue:ID 设置值。
    if (filter.startsWith("queue:")) return apply(matches.filter((match) => String(match.queueId) === filter.slice(6)));
    if (filter === "hextech-aram") return apply(matches.filter(isOrdinaryHextechMatch));
    if (filter === "arena") return apply(matches.filter((match) => matchModeKind(match) === "arena"));
    return apply(matches.filter((match) => match.modeGroup === filter));
  }

  function closeAppSelect(root, restoreFocus = false) {
    const trigger = root?.querySelector("[data-app-select-trigger]");
    const menu = root?.querySelector("[data-app-select-menu]");
    if (!trigger || !menu) return;
    menu.hidden = true;
    trigger.setAttribute("aria-expanded", "false");
    if (restoreFocus) trigger.focus();
  }

  function appSelectOptions(root) {
    return [...(root?.querySelectorAll('[role="menuitemradio"]:not(:disabled)') || [])];
  }

  function openAppSelect(root, edge = "selected") {
    const trigger = root?.querySelector("[data-app-select-trigger]");
    const menu = root?.querySelector("[data-app-select-menu]");
    if (!trigger || !menu) return;
    for (const other of document.querySelectorAll(".app-select")) if (other !== root) closeAppSelect(other);
    menu.hidden = false;
    trigger.setAttribute("aria-expanded", "true");
    const options = appSelectOptions(root);
    const target = edge === "last" ? options.at(-1) : edge === "first" ? options[0] : options.find((item) => item.getAttribute("aria-checked") === "true") || options[0];
    target?.focus();
  }

  function moveAppSelectFocus(root, key) {
    const options = appSelectOptions(root);
    if (!options.length) return;
    const current = options.indexOf(document.activeElement);
    const next = key === "Home" ? 0 : key === "End" ? options.length - 1 : (Math.max(0, current) + (key === "ArrowDown" ? 1 : -1) + options.length) % options.length;
    options[next].focus();
  }

  function bindAppSelect(root, onSelect) {
    if (!root) return;
    const trigger = root.querySelector("[data-app-select-trigger]");
    const menu = root.querySelector("[data-app-select-menu]");
    trigger?.addEventListener("click", () => trigger.getAttribute("aria-expanded") === "true" ? closeAppSelect(root) : openAppSelect(root));
    trigger?.addEventListener("keydown", (event) => {
      if (!["ArrowDown", "ArrowUp"].includes(event.key)) return;
      event.preventDefault();
      openAppSelect(root, event.key === "ArrowUp" ? "last" : "selected");
    });
    menu?.addEventListener("click", (event) => {
      const option = event.target.closest("[data-app-select-value]");
      if (!option) return;
      closeAppSelect(root, true);
      onSelect(option.dataset.appSelectValue);
    });
    menu?.addEventListener("keydown", (event) => {
      if (event.key === "Escape") { event.preventDefault(); closeAppSelect(root, true); return; }
      if (event.key === "Tab") { closeAppSelect(root); return; }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      moveAppSelectFocus(root, event.key);
    });
  }

  // 分组键：斗魂竞技场等模式按小队（subteamId）分组，其余按红蓝方。
  function participantGroupKey(item) {
    return Number(item?.subteamId) > 0 ? `s${item.subteamId}` : `t${item?.teamId}`;
  }

  // Scores are computed in Go. This adapter only indexes the response for rows.
  function readMatchScores(match) {
    return new Map((match.participants || []).filter(item => item.score && Number.isFinite(item.score.value)).map(item => {const record={ ...item.score, score: item.score.value };Object.defineProperty(record,'participantId',{value:item.participantId});return [Number(item.participantId),record];}));
  }

  function scoreBadgeChip(record) {
    if (!record?.badge) return "";
    return `<b class="match-badge-chip is-${record.badge.toLowerCase()}">${record.badge}</b>`;
  }

  function scoreChip(record) {
    if (!record) return "";
    const tone = record.score >= 8 ? " is-gold" : record.score >= 6.5 ? " is-good" : record.score < 5 ? " is-poor" : "";
    const safe = value => String(value).replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
    const labels = { kda: "KDA", kp: "参团", damage: "伤害", gold: "经济", cs: "分均补刀", vision: "视野" };
    const parts = (record.parts || []).filter(part => Number.isFinite(part.value) && Number.isFinite(part.norm)).map(part => ({ label: labels[part.key] || part.key, value: part.key === "kp" ? `${(part.value * 100).toFixed(1)}%` : part.value.toFixed(2), norm: Math.max(0,Math.min(1,part.norm)) }));
    const tooltip = `评分：全场第${record.rank || "—"}名`;
    return `<i class="match-score${tone}" data-tooltip="${safe(tooltip)}" data-score-participant="${Number(record.participantId) || 0}" data-tooltip-score="${safe(JSON.stringify({ value: record.score.toFixed(1), parts }))}" data-tooltip-size="compact" tabindex="0">${record.score.toFixed(1)}</i>`;
  }

  function scorePlacementChip(record, card = false) {
    const badge = scoreBadgeChip(record);
    if (badge) return badge;
    if(card && (!Number.isInteger(Number(record?.rank)) || Number(record?.rank)<1 || !Number.isInteger(Number(record?.total)) || Number(record?.total)<Number(record.rank)))return "";
    return !card && Number(record?.rank) === 1 ? "" : scoreRankChip(record?.rank, record?.total);
  }

  function renderMatchScoreCell(record) {
    const label = scorePlacementChip(record);
    return `<span class="match-score-cell"><span class="match-score-value">${scoreChip(record)}</span><span class="match-score-badge">${label}</span></span>`;
  }

	function renderMatchDetailFailure(gameID, detailState) {
		if (detailState?.status !== "failed") return "";
		const id = Number(gameID);
		if (!Number.isSafeInteger(id) || id <= 0) return "";
		const message = detailState?.message || "完整详情读取失败，请稍后重试";
		return `<div class="match-detail-failure" role="status"><span><strong>完整详情未加载</strong><small>${escapeHTML(message)}</small></span><button class="text-button" type="button" data-retry-match-detail="${id}">重试</button></div>`;
	}

  // 装备栏按原始槽位渲染：前 6 格是装备（空位保留占位框），第 7 格固定
  // 是饰品（守卫 / 扫描等），与 OP.GG 一致用圆形并留出间隔。
  function renderItemSlots(items) {
    const slots = Array.from({ length: 7 }, (_, index) => Number(items?.[index]) || 0);
    return slots.map((id, index) => {
      const trinket = index === 6 ? " is-trinket" : "";
      if (!id) return `<span class="item-slot is-empty${trinket}" aria-hidden="true"></span>`;
      return `<span class="item-slot${trinket}">${itemIconFigure(id, "slot")}</span>`;
    }).join("");
  }

  function ordinalLabel(rank) {
    const value = Math.trunc(Number(rank));
    if (!Number.isFinite(value) || value <= 0) return "—";
    const remainder = value % 100;
    const suffix = remainder >= 11 && remainder <= 13
      ? "th"
      : ({ 1: "st", 2: "nd", 3: "rd" }[value % 10] || "th");
    return `${value}${suffix}`;
  }

  // 竞技场客户端使用固定的中立生物队名。playerSubteamId 与客户端队伍
  // 槽位一一对应；名称和徽标均取自 lol-game-data，而不是展示英文枚举。
  const ARENA_TEAM_MASCOTS = [
    null,
    { name: "魄罗", file: "poro.svg" },
    { name: "小兵", file: "minion.svg" },
    { name: "河道蟹", file: "scuttle.svg" },
    { name: "石甲虫", file: "krug.svg" },
    { name: "锋喙鸟", file: "raptor.svg" },
    { name: "暗影狼", file: "wolf.svg" },
    { name: "魔沼蛙", file: "gromp.svg" },
    { name: "哨兵", file: "sentinel.svg" },
  ];

  function arenaTeamMeta(group) {
    const id = Number(group?.subteamId) || 0;
    const mascot = ARENA_TEAM_MASCOTS[id] || null;
    return mascot
      ? { ...mascot, iconPath: `/arena-team-icons/${mascot.file}` }
      : { name: id ? `小队 ${id}` : "竞技场小队", iconPath: "" };
  }

  // 多小队分组（斗魂竞技场 21 人、3 人一队）：优先用 subteamId / placement；
  // 老数据没有小队字段时按每队人数推断切块。普通模式返回红蓝两组。
  function matchPlayerGroups(match) {
    const participants = match.participants || [];
    if (participants.some((item) => Number(item.subteamId) > 0)) {
      const groups = new Map();
      for (const item of participants) {
        const key = Number(item.subteamId) || 0;
        if (!groups.has(key)) groups.set(key, { subteamId: key, placement: 0, players: [] });
        const group = groups.get(key);
        group.players.push(item);
        if (!group.placement && Number(item.placement) > 0) group.placement = Number(item.placement);
      }
      const subject = participants.find((item) => Number(item.participantId) === Number(match.subjectParticipantId) || item.isCurrent);
      const subjectSubteam = Number(subject?.subteamId) || 0;
      return { arena: true, groups: [...groups.values()].sort((left, right) => {
        if (subjectSubteam > 0 && left.subteamId !== right.subteamId) {
          if (left.subteamId === subjectSubteam) return -1;
          if (right.subteamId === subjectSubteam) return 1;
        }
        return (left.placement || 99) - (right.placement || 99) || left.subteamId - right.subteamId;
      }) };
    }
    if (match.modeGroup === "arena" || participants.length > 10) {
      return { arena: true, groups: [{ subteamId: 0, placement: 0, players: participants }] };
    }
    return { arena: false, groups: [100, 200].map((teamId) => ({ teamId, players: participants.filter((item) => item.teamId === teamId) })) };
  }

  function arenaGroupLabel(group) {
    if (!(Number(group.subteamId) > 0)) return "小队归属未提供";
    const team = arenaTeamMeta(group);
    return group.placement > 0 ? `第 ${group.placement} 名 · ${team.name}` : team.name;
  }

  // 卡片右侧的玩家名单：普通模式两列（红蓝方，图标 + 名称）；
  // 斗魂竞技场按名次预览前四支队伍，完整队伍在展开详情中展示。
  function renderMatchPlayers(match) {
    const grouping = matchPlayerGroups(match);
    const playerButton = (item, index) => {
      const fullName = playerParticipantName(item, index);
      const visibleName = grouping.arena && !state.settings.maskNames ? (item.gameName || item.displayName || "隐藏玩家").split("#")[0] : fullName;
      return `<button type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}" ${proBadgeAttributes(item)}` : "disabled"} data-tooltip="${escapeHTML(fullName)}" ${grouping.arena ? "" : 'data-tooltip-overflow=".match-player-name"'} data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "tiny", false)}<span class="match-player-name${isCurrentMatchParticipant(item, match) ? " is-current-player" : ""}">${escapeHTML(visibleName)}</span></button>`;
    };
    if (grouping.arena) {
      const rows = grouping.groups.slice(0, 4).map((group) => {
        const placement = Number(group.placement) || 0;
        const rankTone = placement >= 1 && placement <= 3 ? ` is-rank-${placement}` : "";
        const rank = `<b class="arena-rank-chip${rankTone}" aria-label="${placement ? `第 ${placement} 名` : "名次未知"}">${placement || "—"}</b>`;
        return `<div class="arena-team-row-compact${placement === 1 ? " is-first" : ""}" data-tooltip="${escapeHTML(arenaGroupLabel(group))}" data-tooltip-size="compact">${rank}<span class="arena-team-roster" data-team-size="${Math.min(group.players.length, 3)}">${group.players.map((item, index) => playerButton(item, index)).join("")}</span></div>`;
      }).join("");
      return `<div class="match-players is-arena">${rows}</div>`;
    }
    const lists = grouping.groups.map((group) => `<div class="match-team-list">${group.players.map((item, index) => playerButton(item, index)).join("")}</div>`).join("");
    return `<div class="match-players">${lists}</div>`;
  }

  // 本场评分排名 chip：前三名有专属配色（金 / 银 / 铜）。
  function scoreRankChip(rank, total) {
    if (!rank) return "";
    const tone = rank <= 3 ? ` is-rank-${rank}` : "";
    return `<b class="match-rank-chip${tone}" data-tooltip="评分：全场第${number(rank)}名" data-tooltip-size="compact">${ordinalLabel(rank)}</b>`;
  }

  // 补位标签：只认后端下发的布尔 true。R121 P1-1 起后端总开关默认关闭，正常
  function autofillChip(player) {
    if (player?.autofill !== true) return "";
    return `<b class="match-autofill-chip" data-tooltip="对局数据里没有这名玩家的个人位置（推算位置：${escapeHTML(positionLabel(player.position))}）" data-tooltip-size="compact">补位</b>`;
  }

  function matchAugmentIDs(subject, limit = 4) {
    return (subject?.augmentIds || []).map(Number).filter((id) => id > 0).slice(0, limit);
  }

  function renderMatchLoadout(subject, modeKind = "standard") {
    const emptyLoadoutSlot = '<span class="match-loadout-slot is-empty" aria-hidden="true"></span>';
    const spells = [
      spellIconFigure(subject.spell1Id, "small"),
      spellIconFigure(subject.spell2Id, "small"),
    ].map((content) => content || emptyLoadoutSlot);
    const runes = [
      subject.perkIds?.[0] ? perkIconFigure(subject.perkIds[0], "small") : "",
      subject.subStyleId ? perkStyleIconFigure(subject.subStyleId, "small") : "",
    ].map((content) => content || emptyLoadoutSlot);
    const augmentIDs = matchAugmentIDs(subject, 4);
    const augmentSlots = Array.from({ length: 4 }, (_, index) => augmentIDs[index]
      ? augmentIconFigure(augmentIDs[index], "small")
      : '<span class="arena-augment-slot is-empty" aria-hidden="true"></span>');
    const usesAugments = modeKind === "arena" || modeKind === "mayhem";
    const loadout = modeKind === "arena"
      ? augmentSlots.join("")
      : modeKind === "mayhem"
        ? augmentIDs.map(id => augmentIconFigure(id, "small")).join("")
        : [...spells, ...runes].join("");
    return { augmentIDs, usesAugments, layout: modeKind === "mayhem" ? "mayhem" : usesAugments ? "augments" : "standard", loadout };
  }

  // 平均段位：优先用数据里带的 averageTier（演示数据），否则读缓存；
  // 均未命中时输出占位符，由 hydrateMatchTiers 懒加载后回填。
  function matchTierContent(value) {
    if (value?.unsupported) return '<b class="match-tier-unknown">不支持</b>';
    if (value?.sourceStale) return '<b class="match-tier-unknown">来源暂未收录</b>';
    if (!value || !value.tier) return '<b class="match-tier-unknown">—</b>';
    const tierName = String(value.tier).toLowerCase();
    const crest = ["iron", "bronze", "silver", "gold", "platinum", "emerald", "diamond", "master", "grandmaster", "challenger"].includes(tierName)
      ? `<img class="match-tier-crest" src="/rank-crests/${tierName}.png" alt="" decoding="async">` : "";
    // 大师及以上无小段位，附加平均胜点（OP.GG 韩服数据提供）。
    const apexLP = ["master", "grandmaster", "challenger"].includes(tierName) && Number(value.lp) > 0 ? ` ${number(value.lp)}` : "";
    return `${crest}<b>${escapeHTML(rankTitle({ tier: value.tier, division: value.division }))}${apexLP}</b>`;
  }

  function matchTierTitle(value) {
    if (value?.unsupported) return "跨服暂不支持排位，无法计算本场平均段位";
    // R127 P1-c.5：来源确实还没收录这场（OP.GG 不自动更新），明确说出来，
    // 不要用一条横线让人以为查询坏了。
    if (value?.sourceStale) return "OP.GG 还没有收录这场对局的平均段位（它不会自动更新），不是查询失败";
    const base = "这场对局玩家的平均段位";
    if (!value) return `${base}\n暂时无法获取`;
    return Number(value.samples) > 0 ? `${base}\n共 ${number(value.samples)} 名玩家参与统计` : base;
  }

  function matchTierFromScores(scores) {
    if (!scores.length) return null;
    const average = Math.round(scores.reduce((total, score) => total + score, 0) / scores.length);
    if (average >= 2800) return { score: average, tier: "MASTER", division: "", samples: scores.length };
    const tiers = ["IRON", "BRONZE", "SILVER", "GOLD", "PLATINUM", "EMERALD", "DIAMOND"];
    const tier = tiers[Math.min(Math.floor(Math.max(0, average) / 400), tiers.length - 1)];
    const division = ["IV", "III", "II", "I"][Math.floor((Math.max(0, average) % 400) / 100)];
    return { score: average, tier, division, samples: scores.length };
  }

  function matchResultKind(match) {
    const result = String(match?.result || "").toLowerCase();
    return ["win", "loss", "remake"].includes(result) ? result : "unknown";
  }

  function matchResultWord(resultKind, arenaPlacement = 0) {
    if (Number(arenaPlacement) > 0) return `第 ${Number(arenaPlacement)} 名`;
    return resultKind === "win" ? "胜利" : resultKind === "loss" ? "失败" : resultKind === "remake" ? "重开" : "结果未知";
  }

  function matchKeywordMetadata(key) {
    return {
      unstoppable:["势不可挡","展现了顶级操作与策略，表现无人能挡，带领队伍取得胜利"],
      leader:["领袖","根据局势果断决策与指挥，为胜利发挥主导作用"],
      victorious:["胜者","凭借前中期的突出发挥带领队伍获胜"],
      latebloomer:["大器晚成","随着比赛推进不断提升表现，最终取胜"],
      resilience:["坚韧","在困境中展现顽强意志，成功逆转不利局面"],
      dedication:["奉献","为胜利做出的努力与付出转化成了积极结果"],
      average:["平凡","整体表现稳定，顺应比赛节奏发挥"],
      rollercoaster:["过山车","整场发挥起伏较大，表现忽高忽低"],
      decline:["下坡路","随着时间推移影响力逐渐减弱"],
      innocent:["竭尽全力","展现了个人最佳操作与表现"],
      unlucky:["不走运","表现出色但因运气不佳未能取得好结果"],
      slowstarter:["慢热","后期持续成长，但因时间有限未能产生明显影响"],
      unyielding:["不屈之志","面对挑战毫不放弃，展现坚韧毅力"],
      struggling:["挣扎","遭遇困境，未能发挥理想水平"],
    }[key] || null;
  }

  let multikillGradientSeq = 0;
  function renderMultiKillTag(count, gameID) {
    const level=Math.min(5,Math.floor(Number(count)||0));if(level<2)return "";
    const designs={
      2:[44,22,"M2 11 8 2H36L42 11 36 20H8Z",""],
      3:[54,24,"M4 8 12 3H42L50 8V16L42 21H12L4 16Z","M0 5 12 9 4 17ZM54 5 42 9 50 17Z"],
      4:[64,26,"M13 5H51L56 13 51 22H13L8 13Z","M1 4 18 8 12 12 2 11 12 15 18 19 9 23 0 13ZM63 4 46 8 52 12 62 11 52 15 46 19 55 23 64 13Z"],
      5:[74,30,"M16 8 21 2 29 7 37 0 45 7 53 2 58 8 63 16 57 27H17L11 16Z","M1 5 13 11 8 2 22 15 16 26 2 21 9 16ZM73 5 61 11 66 2 52 15 58 26 72 21 65 16Z"],
    };
    const [width,height,path,wings]=designs[level],id=`mk-${level}-${Number(gameID)||0}-${++multikillGradientSeq}`;
    return `<b class="multikill-tag is-mk-${level}" data-af-multikill="${level}"><svg viewBox="0 0 ${width} ${height}" aria-hidden="true"><defs><linearGradient id="${id}" x2="0" y2="1"><stop class="mk-stop-start"/>${level===5 ? '<stop offset=".5" class="mk-stop-middle"/>' : ""}<stop offset="1" class="mk-stop-end"/></linearGradient>${level===5 ? `<linearGradient id="${id}-flame"><stop class="mk-flame-start"/><stop offset="1" class="mk-flame-end"/></linearGradient>` : ""}</defs>${wings ? `<path class="mk-wings" d="${wings}"${level===5 ? ` fill="url(#${id}-flame)"` : ""}/>` : ""}<path class="mk-plate" d="${path}" fill="url(#${id})"/>${level===2 ? '<path class="mk-highlight" d="M8 4H36"/>' : ""}</svg><b>${["","","双杀","三杀","四杀","五杀"][level]}</b></b>`;
  }

  function arenaPlacementResult(match, placement) {
    if (!(Number(placement)>0)) return "unknown";
    const teams=new Set((match.participants || []).map(p=>Number(p.subteamId)).filter(id=>id>0));
    const queue=Number(match.queueId);
    const count=Math.max(teams.size, queue===1750 ? 6 : [1700,1710,1701,1704,1720,1731,1732,1740].includes(queue) ? 8 : 0);
    if(count<2)return "unknown";
    return Number(placement)<=Math.ceil(count/2) ? "win" : "loss";
  }

  function matchDataTags(match, subject) {
    const players=match.participants || [],tags=[];
    const map=Number(match.mapId || 0),group=String(match.modeGroup || "").toLowerCase();
    const fallback=["solo","flex","match"].includes(group) ? "rift" : ["aram","hextech-aram","hextech-classic","hextech-qualifier"].includes(group) ? "aram" : group==="arena" ? "arena" : "other";
    const modeSet=map===11 ? "rift" : map===12 ? "aram" : map===30 ? "arena" : map===0 ? fallback : "other";
    if(!subject || players.length<2)return tags;
    const sameTeam=p=>Number(subject.subteamId)>0 ? p.subteamId===subject.subteamId : p.teamId===subject.teamId;
    const team=players.filter(sameTeam),numeric=v=>v!=null && Number.isFinite(Number(v));
    const value=(p,key)=>numeric(p[key]) ? Number(p[key]) : null;
    const kp=p=>{const mates=players.filter(q=>Number(p.subteamId)>0 ? q.subteamId===p.subteamId : q.teamId===p.teamId);const kills=mates.reduce((n,q)=>n+Number(q.kills || 0),0);return kills>0 ? (Number(p.kills || 0)+Number(p.assists || 0))/kills : null;};
    const add=(label,priority,kind,description,star=false)=>tags.push({label,priority,kind,description,star});
    const leaders=[
      ["damage","输出",1800,1750,"damage"],["efficiency","伤转率",1800,930,"efficiency"],
      ["timeCCingOthers","控制",1750,1700,"control"],["totalHeal","治疗",1600,1550,"heal"],
      ["totalDamageShieldedOnTeammates","护盾",1500,1450,"shield"],["damageTaken","承伤",1400,1350,"tank"],
      ["kills","击杀",1200,1150,"kills"],["kp","参团",1100,1050,"participation"],
      ["damageDealtToTurrets","拆塔",900,850,"tower"],["gold","经济",700,650,"gold"],["cs","补兵",600,0,"cs"]
    ];
    for(const [key,label,globalPriority,teamPriority,kind] of leaders) {
      if(["kp","damageDealtToTurrets","gold","cs"].includes(key) && modeSet!=="rift")continue;
      if(key==="efficiency" && !["rift","aram"].includes(modeSet))continue;
      const read=p=>key==="efficiency" ? (Number(p.gold)>0 && numeric(p.damage) ? Number(p.damage)/Number(p.gold) : null) : key==="kp" ? kp(p) : value(p,key);
      const own=read(subject);if(!(own>0))continue;
      // Incomplete optional observations cannot establish a maximum.
      const global=players.every(p=>read(p)!=null) && players.every(p=>read(p)<=own);
      const local=team.length>1 && team.every(p=>read(p)!=null) && team.every(p=>read(p)<=own);
      if(!global && (!local || !teamPriority))continue;
      const formatted=key==="kp" ? `${(own*100).toFixed(1)}%` : key==="efficiency" ? own.toFixed(2) : String(Math.round(own));
      const teamDamage=team.reduce((sum,p)=>sum+Number(p.damage || 0),0);
      const detail=`${global ? "全场" : "队内"}最高${label}：${formatted}${key==="damage" && teamDamage>0 ? `，占队伍伤害的 ${(own/teamDamage*100).toFixed(1)}%` : ""}`;
      add(label,global ? globalPriority : teamPriority,kind,detail,global);
    }
    if(modeSet==="rift" && value(subject,"soloKills")>=2)add(`单杀×${subject.soloKills}`,1700,"solo",`单杀：${subject.soloKills} 次`);
    const minutes=Number(match.durationSeconds || match.duration || 0)/60;
    if(minutes>0 && modeSet==="rift") {
      for(const [key,label,divisor,priority,kind] of [["killsNearEnemyTurret","越塔",5,1660,"dive"],["killsUnderOwnTurret","塔之子",6,1640,"defense"]]) {
        const n=value(subject,key);if(n>0 && n>=minutes/divisor)add(`${label}×${n}`,priority,kind,`${label}击杀：${n} 次`);
      }
    }
    const advantage=value(subject,"maxCsAdvantageOnLaneOpponent");
    if(modeSet==="rift" && advantage>=40)add(`压刀×${advantage}`,750+10*advantage,"advantage",`最大对位补刀领先：${advantage}`);
    const hooks=value(subject,"knockEnemyIntoTeamAndKill");
    if(["rift","aram"].includes(modeSet) && hooks>=6)add(`好钩×${hooks}`,660,"hook",`将敌人击入队伍并击杀：${hooks} 次`);
    return tags;
  }

  function renderMatchTags(match, subject, record, tab) {
    if(match.result==="remake" || match.result==="unknown" || !subject)return "";
    const tags=matchDataTags(match,subject).map(tag=>({...tag,priority:20000+tag.priority,markup:`<b class="match-data-tag is-data-${tag.kind}${tag.star ? " is-global" : ""}" data-tooltip="${escapeHTML(tag.description)}" data-tooltip-size="compact">${escapeHTML(tag.label)}</b>`}));
    const add=(markup,label,priority,description=label)=>{if(markup)tags.push({markup,label,priority,description});};
    const evaluation=scorePlacementChip(record,true);add(evaluation,record?.badge || "评分排名",60000);
    const levels=[[5,"pentaKills",20000],[4,"quadraKills",1300],[3,"tripleKills",300],[2,"doubleKills",100]];
    const observed=levels.some(([,key])=>subject[key]!=null);
    let higher=0;
    for(const [level,key,priority] of levels) {
      const count=observed ? Math.max(0,Number(subject[key] || 0)-higher) : Number(subject.multiKill)===level ? 1 : 0;
      if(observed)higher=Number(subject[key] || 0);
      if(count) {const label=["","","双杀","三杀","四杀","五杀"][level];add(renderMultiKillTag(level,match.gameId),label,40000+level*1000+Math.min(count,99),`${label}：${count} 次`);}
    }
    const cached=state.matchTimelines?.get(matchTimelineKey(match,subject,tab))?.tags?.find(row=>Number(row.participantId)===Number(subject.participantId));
    const key=cached ? cached.keyword : subject.keyword,meta=matchKeywordMetadata(key);
    if(meta && ["unstoppable","latebloomer","resilience","unlucky","unyielding","struggling"].includes(key) && !subject.keywordDisabled && !cached?.disabled)add(`<b class="match-keyword-tag is-tag-${key}" data-tooltip="${escapeHTML(meta[1])}" data-tooltip-size="compact">${meta[0]}</b>`,meta[0],30000,meta[1]);
    if(subject.deaths===0)add('<b class="match-zero-deaths">零阵亡</b>',"零阵亡",10000,"阵亡：0 次");
    if(!tags.length)return "";
    tags.sort((a,b)=>b.priority-a.priority);
    return `<span class="match-tags-collection">${tags.map(tag=>`<span data-match-tag${tag.markup.includes("data-tooltip=") ? "" : ` data-tooltip="${escapeHTML(tag.description)}" data-tooltip-size="compact"`}${tag.priority===60000 ? ' data-match-tag-pinned=""' : ""}>${tag.markup}</span>`).join("")}<button type="button" class="match-tags-more" data-match-tags-more aria-label="查看其余标签" aria-expanded="false" hidden></button></span>`;
  }

  // One body-level panel survives list/filter DOM moves; rows hold no global listeners.
  function bindMatchTagsPopover(collection) {
    const doc=collection.ownerDocument,win=doc.defaultView;
    if(!doc._matchTagsPopover) {
      const panel=doc.createElement("div");panel.className="match-tags-popover";panel.id="match-tags-popover";
      panel.setAttribute("role","tooltip");panel.hidden=true;doc.body.append(panel);
      let anchor=null,leaveTimer=0,touchUntil=0;
      const observer=new win.MutationObserver(()=>{if(anchor && !anchor.isConnected)close();});
      const cancelLeave=()=>{win.clearTimeout(leaveTimer);leaveTimer=0;};
      const close=()=>{observer.disconnect();cancelLeave();if(anchor){anchor.setAttribute("aria-expanded","false");anchor.removeAttribute("aria-describedby");}anchor=null;panel.hidden=true;panel.replaceChildren();};
      const show=button=>{
        const hidden=[...button.parentElement.querySelectorAll("[data-match-tag]")].filter(item=>item.hidden);
        if(!hidden.length)return close();
        close();anchor=button;panel.replaceChildren(...hidden.map(item=>{const clone=item.cloneNode(true);clone.hidden=false;clone.removeAttribute("data-match-tag");
          // Cloned multikill gradients need distinct IDs while the source stays in the row.
          for(const node of clone.querySelectorAll("[id]")) {const old=node.id,next=`${panel.id}-${old}`;node.id=next;for(const target of clone.querySelectorAll("*"))for(const attr of [...target.attributes])if(attr.value.includes(`url(#${old})`))target.setAttribute(attr.name,attr.value.replaceAll(`url(#${old})`,`url(#${next})`));}
          if(clone.matches("[data-tooltip]"))clone.tabIndex=0;clone.querySelectorAll("[data-tooltip]").forEach(tag=>tag.tabIndex=0);return clone;}));
        panel.hidden=false;button.setAttribute("aria-expanded","true");button.setAttribute("aria-describedby",panel.id);
        const scale=Number(win.getComputedStyle(panel).zoom)||1;
        const rect=button.getBoundingClientRect(),padding=12,gap=8,width=panel.offsetWidth*scale,height=panel.offsetHeight*scale;
        const above=rect.top>=height+gap+padding;
        const left=Math.max(padding,Math.min(win.innerWidth-width-padding,rect.left+rect.width/2-width/2));
        panel.dataset.placement=above ? "top" : "bottom";
        panel.style.left=`${left/scale}px`;panel.style.top=`${Math.max(padding,above ? rect.top-height-gap : Math.min(win.innerHeight-height-padding,rect.bottom+gap))/scale}px`;
        panel.style.setProperty("--tag-arrow-left",`${Math.max(8,Math.min(width-8,rect.left+rect.width/2-left))/scale}px`);
        observer.observe(doc.body,{childList:true,subtree:true});
      };
      const buttonOf=node=>node?.closest?.("[data-match-tags-more]");
      doc.addEventListener("pointerdown",event=>{if(event.pointerType==="touch")touchUntil=Date.now()+800;});
      doc.addEventListener("pointerover",event=>{const button=buttonOf(event.target);if(panel.contains(event.target)||button===anchor)cancelLeave();else if(button && Date.now()>touchUntil)show(button);});
      doc.addEventListener("pointerout",event=>{if(!anchor)return;const next=event.relatedTarget;if(panel.contains(next)||anchor.contains(next))return; if(panel.contains(event.target)||anchor.contains(event.target))leaveTimer=win.setTimeout(close,60);});
      doc.addEventListener("focusin",event=>{const button=buttonOf(event.target);if(button && Date.now()>touchUntil)show(button);});
      doc.addEventListener("focusout",event=>{if(anchor && (anchor.contains(event.target)||panel.contains(event.target)) && !panel.contains(event.relatedTarget))close();});
      doc.addEventListener("click",event=>{const button=buttonOf(event.target);if(button){event.stopPropagation();anchor===button ? close() : show(button);}else if(anchor && !panel.contains(event.target))close();});
      doc.addEventListener("keydown",event=>{if(event.key==="Escape")close();});
      doc.addEventListener("scroll",close,true);win.addEventListener("resize",close);win.addEventListener("blur",close);
      doc._matchTagsPopover={close,get anchor(){return anchor;}};
    }
    return doc._matchTagsPopover;
  }

  function fitMatchTags(container, onlyNew = false) {
    const fits=[];
    const batch=jobs=>{
      const measured=[];
      for(const {collection,popup} of jobs){
        if(collection.isConnected===false || typeof collection.checkVisibility==="function" && !collection.checkVisibility({contentVisibilityAuto:true}))continue;
        const width=collection.clientWidth;if(!width)continue;
        const main=collection.closest?.('.match-entry')?.querySelector('.match-main');
        const rowWidth=main?.clientWidth,items=collection._tagFitWidth===width?null:[...collection.querySelectorAll("[data-match-tag]")],more=items?collection.querySelector("[data-match-tags-more]"):null;
        measured.push({collection,popup,width,main,rowWidth,items,more});
      }
      // Reset every changed row before any geometry read can force layout.
      for(const job of measured){
        const {collection,popup,width,main,rowWidth,items,more}=job;
        if(main && main._iconFitWidth!==rowWidth){main._iconFitWidth=rowWidth;main.removeAttribute('data-icon-overlap');}
        if(!items || !more)continue;
        collection._tagFitWidth=width;
        if(popup.anchor===more)popup.close();
        for(const item of items)item.hidden=false;more.hidden=true;
      }
      for(const job of measured){
        const {main,items,more}=job;
        const icons=main?.querySelector('.match-loadout-mini'),kda=main?.querySelector('.match-kda');
        if(icons && kda){const a=icons.getBoundingClientRect(),b=kda.getBoundingClientRect();job.overlap=a.width && b.width && a.right>b.left+.5 && a.left<b.right-.5 && a.bottom>b.top+.5 && a.top<b.bottom-.5;}
        if(items && more)job.widths=items.map(item=>item.getBoundingClientRect().width+4);
      }
      for(const {main,overlap,items,more,width,widths} of measured){
        if(overlap && !main.hasAttribute?.('data-icon-overlap'))main.setAttribute('data-icon-overlap','');
        if(!widths)continue;
        let used=0,hidden=0;
        for(let i=0;i<items.length;i++) {
          const reserve=i<items.length-1 ? 32 : 0;
          if(!items[i].hasAttribute("data-match-tag-pinned") && (hidden || used+widths[i]+reserve>width)){items[i].hidden=true;hidden++;}
          else used+=widths[i];
        }
        more.hidden=!hidden;more.textContent=`+${hidden}`;
        more.setAttribute("aria-label",`查看其余 ${hidden} 个标签`);
      }
    };
    const scheduler=container.ownerDocument || container;
    const queueFit=job=>{
      const pending=scheduler._tagFitJobs ||= new Map();pending.set(job.collection,job);
      if(scheduler._tagFitFrame)return;
      scheduler._tagFitFrame=requestAnimationFrame(()=>{scheduler._tagFitFrame=0;const jobs=[...pending.values()];pending.clear();batch(jobs);});
    };
    for(const collection of container.querySelectorAll(".match-tags-collection")) {
      if(onlyNew && collection._tagObserver)continue;
      const popup=bindMatchTagsPopover(collection);
      fits.push({collection,popup});
      if(typeof ResizeObserver!=="undefined" && !collection._tagObserver) {
        collection._tagObserver=new ResizeObserver(()=>queueFit({collection,popup}));collection._tagObserver.observe(collection);
        const main=collection.closest?.('.match-entry')?.querySelector('.match-main');
        if(main){
          collection._tagObserver.observe(main);
          for(const target of main.querySelectorAll('.match-champion,.match-loadout-mini,.match-kda'))collection._tagObserver.observe(target);
        }
      }
    }
    batch(fits);
  }

  function matchTagsBackgroundAllowed(tab) {
    return !state.destroyed && !document.hidden && !riotTab(tab) && connected() && state.section==="overview" && !tab.closed && !tab.overviewSubpage && activeTab(tabGroup(tab))===tab && !["ChampSelect","InProgress"].includes(state.beacon?.phase || state.live?.phase);
  }

  function hydrateVisibleMatchTags(container, tab) {
    if(!matchTagsBackgroundAllowed(tab) || tab.matchTagsHydrating)return;
    const root=matchTierScrollRoot(container);
    // Index once: querying the full card tree separately for every match is
    // quadratic in large histories. Preserve the first-node lookup rule.
    const nodes=new Map();
    for(const node of container.querySelectorAll("[data-match-id]")){const id=node.getAttribute("data-match-id");if(!nodes.has(id))nodes.set(id,node);}
    const matches=(tab.data?.matches || []).filter(match=>!match.tagsAvailable && match.result!=="remake" && match.result!=="unknown" && ![1700,1710].includes(Number(match.queueId)) && matchTierNodeIsVisible(nodes.get(String(match.gameId)),root));
    const diagnosticBatch=`timeline-${Date.now()}-${Math.random().toString(36).slice(2,10)}`;
    tab.matchTagsHydrating=true;let next=0;
    const keys=new Set(),monitor=setInterval(()=>{if(!matchTagsBackgroundAllowed(tab) || !container.isConnected)for(const key of keys)state.controllers?.get(key)?.abort();},200);
    const worker=async()=>{while(next<matches.length && matchTagsBackgroundAllowed(tab) && container.isConnected) {
      const match=matches[next++],subject=matchSubject(match,tab.data?.player?.playerRef);if(!subject)continue;
      const key=`match-timeline:${matchTimelineKey(match,subject,tab)}`;keys.add(key);
      try {await ensureMatchTimeline(match,subject,tab,diagnosticBatch);}finally{keys.delete(key);}
    }};
    void Promise.all([worker(),worker()]).finally(()=>{clearInterval(monitor);tab.matchTagsHydrating=false;recordTimelineClient("complete",diagnosticBatch);});
  }

  function renderMatch(match, playerRef, tab) {
    const subject = matchSubject(match, playerRef);
    if (!subject) return "";
    const resultKind = matchResultKind(match);
    const win = resultKind === "win";
    // 参团率按本队成员实际击杀合计（斗魂竞技场按小队），只有拿到
    // 完整参与者名单时才计算，且封顶 100%，避免数据缺失时出现离谱数字。
    const groupKills = (match.participants || [])
      .filter((item) => participantGroupKey(item) === participantGroupKey(subject))
      .reduce((sum, item) => sum + (Number(item.kills) || 0), 0);
    const participation = (match.participants || []).length > 1 && groupKills > 0
      ? Math.min(100, Math.round((Number(subject.kills) + Number(subject.assists)) * 100 / groupKills))
      : null;
    const scores = readMatchScores(match);
    const subjectScore = scores.get(Number(subject.participantId));
    const matchCardOptions = tab?.matchCardOptions || {};
    const detailState = matchCardOptions.detailStates?.get(String(match.gameId));
		const detailLoading = detailState?.status === "loading";
		const detailFailed = detailState?.status === "failed";
		const canExpand = !matchCardOptions.disableExpand && !detailLoading && !detailFailed;
    const canReplay = !matchCardOptions.disableReplay;
    const open = canExpand && tab.openMatches.has(String(match.gameId));
    const expandLabel = canExpand ? `${open ? "收起" : "展开"}这场对局` : detailLoading ? "正在读取这场对局的完整详情" : detailFailed ? "这场对局的完整详情读取失败" : "该样本不支持展开详情";
    const expandTooltip = canExpand ? (open ? "收起详情" : "展开详情") : detailState?.message || matchCardOptions.expandDisabledReason || "该样本不支持展开详情";
    // 左侧：模式 + 时间（未知时整行不展示）+ 结果 + 排位胜点记录状态 + 时长。
    const timeLabel = relativeTime(match.createdAt);
    const timeRow = timeLabel ? `<time data-tooltip="${escapeHTML(exactTime(match.createdAt))}" data-tooltip-size="compact">${escapeHTML(timeLabel)}</time>` : "";
    const queueID = Number(match.queueId);
    const modeKind = matchModeKind(match, subject);
    const ranked = queueID === 420 || queueID === 440;
    const lpChip = renderLPChip(match);
    // 斗魂竞技场展示小队名次代替胜负字样。
    const arenaPlacement = modeKind === "arena" && Number(subject.placement) > 0 ? Number(subject.placement) : 0;
    const resultWord = matchResultWord(resultKind, arenaPlacement);
    // 中部数据列：击杀参与率 / 分均补刀 / 平均段位，悬停展示计算说明。
    const participationTitle = participation === null
      ? "击杀参与率：需要完整的参与者数据才能计算"
      : `击杀参与率：(击杀 ${number(subject.kills)} + 助攻 ${number(subject.assists)}) ÷ 队伍总击杀 ${number(groupKills)}`;
    const laneCS = Number.isFinite(Number(subject.laneCs)) ? Number(subject.laneCs) : null;
    const jungleCS = Number.isFinite(Number(subject.jungleCs)) ? Number(subject.jungleCs) : null;
    const csTitle = laneCS !== null && jungleCS !== null
      ? `小兵 ${number(laneCS)} + 野怪 ${number(jungleCS)}\n每分钟CS${number(subject.csPerMinute)}个`
      : `共补刀 ${number(subject.cs)} 个\n每分钟CS${number(subject.csPerMinute)}个`;
    // 平均段位在总览首屏之后异步回填；缓存必须与区域和页签身份绑定。
    const tierKey = matchTierCacheKey(tab, match.gameId);
    const rankCapability = (tab.data?.capabilities || []).find((item) => item.name === "ranked-stats");
    const crossServerRankUnsupported = !riotTab(tab) && rankCapability?.state === "unsupported" && rankCapability.detail === "跨服暂不支持排位";
    const tierValue = crossServerRankUnsupported ? { unsupported: true } : (match.averageTier ?? (state.matchTiers.has(tierKey) ? state.matchTiers.get(tierKey) : undefined));
    const tierPending = tierValue === undefined ? ' data-match-tier-pending=""' : "";
    const participationRow = `<span class="match-stat match-stat-participation" data-tooltip="${escapeHTML(participationTitle)}" data-tooltip-size="compact"><em>击杀参与率</em><b>${participation === null ? "—" : percent(participation)}</b></span>`;
    const csRow = `<span class="match-stat" data-tooltip="${escapeHTML(csTitle)}" data-tooltip-size="compact"><em>CS</em><b>${number(subject.cs)} <span class="match-stat-secondary">(${number(subject.csPerMinute)})</span></b></span>`;
    const tierRow = `<span class="match-stat match-stat-tier" data-match-tier data-game-id="${match.gameId}"${tierPending} data-tooltip="${escapeHTML(matchTierTitle(tierValue))}" data-tooltip-size="compact"><em>平均段位</em><span class="match-tier-value">${tierValue === undefined ? '<b class="match-tier-unknown">…</b>' : matchTierContent(tierValue)}</span></span>`;
    const damageRow = `<span class="match-stat match-stat-damage" data-tooltip="对英雄造成的总伤害" data-tooltip-size="compact"><em>伤害</em><b>${plainInteger(subject.damage)}</b></span>`;
    const takenRow = `<span class="match-stat match-stat-taken" data-tooltip="承受的总伤害" data-tooltip-size="compact"><em>承伤</em><b>${plainInteger(subject.damageTaken)}</b></span>`;
    // 斗魂竞技场用伤害/承伤替代召唤师峡谷的参团率、CS、平均段位；
    // 数据缺失时保留明确占位，避免卡片列高度随模式变化而漂移。
    const statRows = modeKind === "arena" ? `${damageRow}${takenRow}` : `${participationRow}${csRow}${tierRow}`;
    const { usesAugments, layout, loadout } = renderMatchLoadout(subject, modeKind);
    const detailFailure = renderMatchDetailFailure(match.gameId, detailState);
    return `<article class="match-entry is-${resultKind}" data-match-id="${match.gameId}">
      <div class="match-summary${arenaPlacement ? " is-arena" : ""}">
        <div class="match-result-meta"><strong>${escapeHTML(match.queueLabel)}</strong>${timeRow}<span class="result-word">${resultWord}${lpChip}</span><small>${formatDuration(match.duration)}</small></div>
        <div data-match-layout class="match-main${arenaPlacement ? " is-arena" : ""}">
          <div class="match-champion">${iconFigure("champion", subject.championId, subject.championName, "large")}<span class="champion-level">${number(subject.championLevel)}</span><div class="match-loadout-mini${usesAugments ? " is-augments" : ""}${layout === "mayhem" ? " is-mayhem" : ""}">${loadout}</div></div>
          <div class="match-kda"><strong><span>${number(subject.kills)}</span> / <em>${number(subject.deaths)}</em> / <span>${number(subject.assists)}</span></strong><small>${kda(subject.kda)}:1 KDA</small></div>
	          ${statRows ? `<div data-match-metrics class="match-stats${modeKind === "arena" ? " is-arena" : ""}">${statRows}</div>` : ""}
          <div class="match-build"><div class="match-items">${renderItemSlots(subject.itemIds)}</div><div class="match-badges">${renderMatchTags(match,subject,subjectScore,tab)}</div></div>
        </div>
        ${renderMatchPlayers(match)}
        <div class="match-actions">
          <button class="match-expand" type="button" aria-expanded="${open}" aria-controls="match-detail-${match.gameId}" data-toggle-match="${match.gameId}" aria-label="${escapeHTML(expandLabel)}" data-tooltip="${escapeHTML(expandTooltip)}" data-tooltip-size="compact" ${canExpand ? "" : "disabled"}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg></button>
          <button class="match-replay" type="button" data-replay="${match.gameId}" aria-label="${canReplay ? "观看这场对局的回放" : "该样本不支持回放"}" data-tooltip="${canReplay ? "观看回放" : escapeHTML(matchCardOptions.replayDisabledReason || "该样本不支持回放")}" data-tooltip-size="compact" ${canReplay ? "" : "disabled"}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5.5v13l11-6.5z"/></svg></button>
        </div>
		</div>
		${detailFailure}
		${open ? renderMatchDetail(match, subject, tab) : ""}
	</article>`;
  }

  /* ---------- 平均段位懒加载 ---------- */

  function applyMatchTierValue(container, scope, gameID, value, retryable = false) {
    if (!container || container.dataset.matchTierScope !== scope) return;
    for (const node of container.querySelectorAll("[data-match-tier]")) {
      if (String(node.dataset.gameId || "") !== String(gameID)) continue;
      if (retryable) node.setAttribute("data-match-tier-pending", "");
      else node.removeAttribute("data-match-tier-pending");
      node.dataset.tooltip = matchTierTitle(value);
      const slot = node.querySelector(".match-tier-value");
      if (slot) slot.innerHTML = matchTierContent(value);
    }
  }

  function matchTierScrollRoot(container) {
	return container?.closest(".player-overlay-scroll") || document.getElementById("app-scroll");
  }

  function matchTierNodeIsVisible(node, root) {
	const target = node?.closest?.(".match-entry") || node;
	if (!target?.getBoundingClientRect) return false;
	const rect = target.getBoundingClientRect();
	const rootRect = root?.getBoundingClientRect?.() || { top: 0, left: 0, right: window.innerWidth, bottom: window.innerHeight };
	return rect.width > 0 && rect.height > 0 && rect.bottom > rootRect.top && rect.top < rootRect.bottom && rect.right > rootRect.left && rect.left < rootRect.right;
  }

  function observeMatchTierVisibility(container, tab, scope = matchTierScope(tab)) {
	const observerKey = matchTierObserverKey(tab);
	state[observerKey]?.disconnect();
	state[observerKey] = null;
	if (!container || tab.loadingMore || tab.filterPaging) return;
	const pending = [...container.querySelectorAll("[data-match-tier-pending]")];
	if (!pending.length) return;
	const root = matchTierScrollRoot(container);
	if (!("IntersectionObserver" in window)) {
	  void hydrateMatchTiers(container, tab, scope, pending.filter((node) => matchTierNodeIsVisible(node, root)));
	  return;
	}
	const observer = new IntersectionObserver((entries) => {
	  const visible = entries.filter((entry) => entry.isIntersecting).map((entry) => entry.target);
	  for (const node of visible) observer.unobserve(node);
	  if (visible.length) void hydrateMatchTiers(container, tab, scope, visible);
	}, { root, rootMargin: "0px" });
	for (const node of pending) observer.observe(node);
	state[observerKey] = observer;
  }

  async function hydrateMatchTiers(container, tab, scope = matchTierScope(tab), candidates = null) {
    // 国服依赖本机客户端；韩服由后端一次读取 OP.GG 对局页，不需要连接客户端。
    if (!container || tab.loadingMore || tab.filterPaging) return;
    if (!riotTab(tab) && !connected()) return;
    const activeKey = tab.overlay ? tab.key : (state.activeTabs?.[tabGroup(tab)] ?? state.activeTab);
    if (!shouldHydrateMatchTiers(tab, activeKey)) return;
	const root = matchTierScrollRoot(container);
	const pendingNodes = (candidates ? [...candidates] : [...container.querySelectorAll("[data-match-tier-pending]")])
	  .filter((node) => node?.isConnected !== false && matchTierNodeIsVisible(node, root));
    const pendingMatches = new Map();
    for (const node of pendingNodes) {
      const gameID = String(node.dataset.gameId || "");
      if (!gameID) continue;
      const cacheKey = matchTierCacheKey(tab, gameID);
      if (state.matchTiers.has(cacheKey)) { applyMatchTierValue(container, scope, gameID, state.matchTiers.get(cacheKey)); continue; }
      const failure = state.matchTierFailures?.get(cacheKey);
      if (riotTab(tab) && failure?.count >= 3) {
        applyMatchTierValue(container, scope, gameID, null);
        continue;
      }
      if (failure && Number(failure.nextRetryAt || 0) > Date.now()) {
        applyMatchTierValue(container, scope, gameID, null, riotTab(tab));
        if (riotTab(tab)) scheduleMatchTierRetry(container, tab, scope);
        continue;
      }
      if (state.matchTierFlights.has(cacheKey)) continue;
      const match = (tab.data?.matches || []).find((item) => String(item.gameId) === gameID);
      if (!match) continue;
      pendingMatches.set(gameID, { match, cacheKey });
    }

    if (riotTab(tab)) {
      const player = tab.data?.player || {};
      const playerRef = player.playerRef || tab.playerRef || "";
      if (!playerRef) {
        for (const [gameID, entry] of pendingMatches) {
          state.matchTiers.set(entry.cacheKey, null);
          applyMatchTierValue(container, scope, gameID, null);
        }
        return;
      }
      let entries = [...pendingMatches.entries()];
      if (!entries.length) return;
      // R127 P1-c.4：同一屏的可见对局合并成一次请求。可见性观测器会分几次回调
      if (MATCH_TIER_KR_COALESCE_MS > 0) {
        await new Promise((resolve) => window.setTimeout(resolve, MATCH_TIER_KR_COALESCE_MS));
        if (state.destroyed || !container.isConnected || container.dataset.matchTierScope !== scope) return;
        entries = entries.filter(([, entry]) => !state.matchTiers.has(entry.cacheKey) && !state.matchTierFlights.has(entry.cacheKey));
        for (const node of container.querySelectorAll("[data-match-tier-pending]")) {
          const gameID = String(node.dataset.gameId || "");
          if (!gameID || !matchTierNodeIsVisible(node, root)) continue;
          const cacheKey = matchTierCacheKey(tab, gameID);
          if (state.matchTierFlights.has(cacheKey) || state.matchTiers.has(cacheKey)) continue;
        // 合并窗口里新扫到的场次同样要尊重失败冷却，否则会提前烧掉重试次数。
        const cooldown = state.matchTierFailures?.get(cacheKey);
        if (cooldown && Number(cooldown.nextRetryAt || 0) > Date.now()) continue;
          if (entries.some(([existing]) => existing === gameID)) continue;
          const match = (tab.data?.matches || []).find((item) => String(item.gameId) === gameID);
          if (!match) continue;
          entries.push([gameID, { match, cacheKey }]);
        }
        if (!entries.length) return;
      }
      for (const [, entry] of entries) state.matchTierFlights.add(entry.cacheKey);
      try {
        const matches = entries.map(([, entry]) => ({
          gameId: entry.match.gameId,
          createdAt: entry.match.createdAt,
          duration: entry.match.duration,
          // R127 P1-c.2：真正的开局/结束时间。后端会拿这三个时间基准分别和
          // OP.GG 的 created_at 比较、取差值最小的；只给 createdAt（房间创建
          // 时间）会整页对不上。
          startAt: Number(entry.match.startedAt) || 0,
          endAt: Number(entry.match.endedAt) || 0,
        }));
        const result = await api("/api/gameplay/match-tiers", {
          method: "POST",
          body: JSON.stringify({
            region: tab.region,
            playerRef,
            gameName: player.gameName || tab.riotId?.gameName || "",
            tagLine: player.tagLine || tab.riotId?.tagLine || "",
            matches,
          }),
        }, `match-tiers:${scope}:${entries.map(([gameID]) => gameID).join(",")}`, 20000);
        for (const [gameID, entry] of entries) {
          const candidate = result?.[gameID];
          // R127 P1-c.5：来源明确落后时后端会给一个没有 tier、但带 sourceStale 的
          // 条目，界面显示「来源暂未收录」，而不是一条让人以为出 bug 的横线。
          const value = candidate?.tier ? candidate : (candidate?.sourceStale ? { sourceStale: true } : null);
          state.matchTiers.set(entry.cacheKey, value);
          state.matchTierFailures?.delete(entry.cacheKey);
          applyMatchTierValue(container, scope, gameID, value);
        }
      } catch (_) {
        for (const [gameID, entry] of entries) {
          noteMatchTierFailure(entry.cacheKey);
          // The backend shares a 30-second upstream failure cooldown. Do not
          // turn a timeout/503 into a ten-minute cached "no rank" result.
          const failure = state.matchTierFailures.get(entry.cacheKey);
          failure.nextRetryAt = Math.max(failure.nextRetryAt, Date.now() + 30_000);
          state.matchTierFailures.set(entry.cacheKey, failure);
          applyMatchTierValue(container, scope, gameID, null, true);
        }
      } finally {
        for (const [, entry] of entries) state.matchTierFlights.delete(entry.cacheKey);
        scheduleMatchTierRetry(container, tab, scope);
      }
      return;
    }

    // 国服把当前可见页签内所有待解析玩家全局去重，按后端上限分批查询。
    const entries = [...pendingMatches.entries()];
    const refsByGame = new Map();
    const allRefs = new Set();
    let totalRefs = 0;
    for (const [gameID, entry] of entries) {
      const refs = [...new Set((entry.match?.participants || []).map((item) => item.playerRef).filter(Boolean))];
      refsByGame.set(gameID, refs);
      totalRefs += refs.length;
      for (const ref of refs) allRefs.add(ref);
      state.matchTierFlights.add(entry.cacheKey);
    }
    for (const [gameID, entry] of entries) {
      if (!refsByGame.get(gameID).length) {
        state.matchTiers.set(entry.cacheKey, null);
        applyMatchTierValue(container, scope, gameID, null);
      }
    }
    const uniqueRefs = [...allRefs];
    if (!uniqueRefs.length) {
      for (const [, entry] of entries) state.matchTierFlights.delete(entry.cacheKey);
      return;
    }
    const playerScores = new Map();
    let cacheHits = 0;
    // R127 P1-b.3：批与批之间不再串行 await。最多 MATCH_TIERS_PARALLEL_BATCHES 批
    // 同时在途，每批一返回就立刻回填「参与者已全部有结果」的对局，不等其它批次。
    const batches = [];
    for (let offset = 0; offset < uniqueRefs.length; offset += MATCH_TIERS_MAX_REFS) {
      batches.push({ offset, refs: uniqueRefs.slice(offset, offset + MATCH_TIERS_MAX_REFS) });
    }
    const outstanding = new Set(uniqueRefs);
    const failedRefs = new Set();
    const applied = new Set();
    let batchFailed = false;
    const startedAt = Date.now();
    const applyReadyGames = () => {
      for (const [gameID, entry] of entries) {
        if (applied.has(gameID)) continue;
        const refs = refsByGame.get(gameID) || [];
        if (!refs.length) { applied.add(gameID); continue; }
        if (refs.some((ref) => outstanding.has(ref))) continue;
        // 参与者里有人查询失败时平均值会偏，宁可留白等重试，也不写一个错的平均段位。
        if (refs.some((ref) => failedRefs.has(ref))) continue;
        applied.add(gameID);
        const scores = refs.map((ref) => playerScores.get(ref)).filter((score) => Number.isFinite(score));
        const value = matchTierFromScores(scores);
        state.matchTiers.set(entry.cacheKey, value);
        state.matchTierFailures?.delete(entry.cacheKey);
        applyMatchTierValue(container, scope, gameID, value);
      }
    };
    try {
      let cursor = 0;
      const drainBatches = async () => {
        while (cursor < batches.length) {
          const batch = batches[cursor++];
          try {
            const result = await api("/api/gameplay/match-tiers", {
              method: "POST",
              body: JSON.stringify({ playerRefs: batch.refs, serverId: tabServerID(tab) }),
            }, `match-tiers:${scope}:${batch.offset}:${batch.refs.join(",")}`, 20000);
            cacheHits += Math.max(0, Number(result?.cacheHits) || 0);
            for (const [ref, value] of Object.entries(result?.players || {})) {
              if (Number.isFinite(Number(value?.score))) playerScores.set(ref, Number(value.score));
            }
          } catch (_) {
            // Settle score batches independently; a failed batch must not clear other flights.
            batchFailed = true;
            for (const ref of batch.refs) failedRefs.add(ref);
          }
          for (const ref of batch.refs) outstanding.delete(ref);
          applyReadyGames();
        }
      };
      await Promise.all(Array.from({ length: Math.min(MATCH_TIERS_PARALLEL_BATCHES, batches.length) }, drainBatches));
      applyReadyGames();
      for (const [gameID, entry] of entries) {
        if (applied.has(gameID)) continue;
        noteMatchTierFailure(entry.cacheKey);
        // retryable：保留 pending 标记并安排重试，一批失败不该把整屏对局永久打成横线。
        applyMatchTierValue(container, scope, gameID, null, true);
      }
      recordMatchTierOverviewBatch(totalRefs, uniqueRefs.length, cacheHits, Date.now() - startedAt, batchFailed);
    } finally {
      for (const [, entry] of entries) state.matchTierFlights.delete(entry.cacheKey);
      if (batchFailed) scheduleMatchTierRetry(container, tab, scope);
    }
  }

  function scheduleMatchTierRetry(container, tab, scope) {
    if (tab.matchTierRetryTimer || state.destroyed || container.dataset.matchTierScope !== scope) return;
    const failures = [...container.querySelectorAll("[data-match-tier-pending]")]
      .map(node => state.matchTierFailures?.get(matchTierCacheKey(tab, node.dataset.gameId)))
      .filter(failure => failure && failure.count < 3);
    if (!failures.length) return;
    const delay = Math.max(1000, Math.min(...failures.map(failure => failure.nextRetryAt)) - Date.now());
    tab.matchTierRetryTimer = window.setTimeout(() => {
      tab.matchTierRetryTimer = null;
      if (state.destroyed || !container.isConnected || container.closest("[hidden]") || container.dataset.matchTierScope !== scope) return;
      const activeKey = tab.overlay ? tab.key : (state.activeTabs?.[tabGroup(tab)] ?? state.activeTab);
      if (!shouldHydrateMatchTiers(tab, activeKey)) return;
      void hydrateMatchTiers(container, tab, scope);
    }, delay);
  }

  function recordMatchTierOverviewBatch(totalRefs, uniqueRefs, cacheHits, durationMs, failed) {
    void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({
        // 有批次失败就如实报 failed，不能把不完整的整批说成 complete。
        event: "match_tiers_overview_batch", reason: failed ? "failed" : "complete",
        totalRefs: Math.max(0, Number(totalRefs) || 0),
        uniqueRefs: Math.max(0, Number(uniqueRefs) || 0),
        cacheHits: Math.max(0, Number(cacheHits) || 0),
        // R127 P0-3：从第一批请求发出到最后一批返回的耗时。以前只报人数与缓存
        // 命中，日志里根本看不出平均段位花了多久（8 秒是靠时间戳相减推出来的）。
        durationMs: Math.max(0, Number(durationMs) || 0),
      }),
    }).catch(() => {});
  }

  function noteMatchTierFailure(cacheKey, now = Date.now()) {
    if (!state.matchTierFailures) state.matchTierFailures = new Map();
    const previous = state.matchTierFailures.get(cacheKey);
    const count = Number(previous?.count || 0) + 1;
    const delay = Math.min(MATCH_TIER_MAX_BACKOFF_MS, MATCH_TIER_RETRY_BASE_MS * (2 ** Math.min(10, count - 1)));
    state.matchTierFailures.set(cacheKey, { count, nextRetryAt: now + delay });
  }

  function shouldHydrateMatchTiers(tab, activeTabKey) {
    return Boolean(tab?.overlay || tab?.key === activeTabKey);
  }

  let matchDetailRenderer = null;
  function ensureMatchDetailRenderer() {
    if(matchDetailRenderer)return Promise.resolve();
    return window.deepLegendsSections.load('match-details').then(()=>{
      if(!matchDetailRenderer)matchDetailRenderer=window.deepLegendsMatchDetails.create({state,readMatchScores,escapeHTML,number,kda,plainInteger,compactNumber,plainText,iconFigure,playerParticipantName,proBadgeAttributes,autofillChip,renderMatchScoreCell,renderItemIcons,matchPlayerGroups,isCurrentMatchParticipant,arenaTeamMeta,arenaPlacementResult,augmentIconFigure,arenaGroupLabel,matchModeKind,positionLabel,itemIconFigure,renderRuneStyleIcon,perkIconFigure,isSummonersRiftMatch,TEAM_ANALYSIS_METRICS,TEAM_ANALYSIS_POSITIONS,SKILL_SLOT_LETTERS,PERK_EFFECT_OVERRIDES});
    });
  }

  function renderMatchDetail(match, subject, tab) {
    if(!matchDetailRenderer){
      if(!tab.matchDetailModulePending && !tab.matchDetailModuleError){
        tab.matchDetailModulePending=true;tab.matchDetailModuleError=false;
        void ensureMatchDetailRenderer().catch(()=>{tab.matchDetailModuleError=true;}).finally(()=>{
          tab.matchDetailModulePending=false;
          if(!tab.closed && !state.destroyed)for(const openID of tab.openMatches)rerenderMatch(tab,openID);
        });
      }
      return `<div id="match-detail-${match.gameId}" class="match-detail" role="status">${tab.matchDetailModuleError?'详情资源加载失败，请收起后重新展开重试':'正在读取对局详情…'}</div>`;
    }
    const active = tab.matchDetailTabs.get(String(match.gameId)) || "overview";
    const panelID = `match-detail-body-${match.gameId}`;
    const detailTab = (value, label) => `<button id="match-detail-tab-${value}-${match.gameId}" class="${active === value ? "is-active" : ""}" type="button" role="tab" aria-selected="${active === value}" aria-controls="${panelID}" tabindex="${active === value ? 0 : -1}" data-match-detail="${value}" data-game-id="${match.gameId}">${label}</button>`;
    return `<div id="match-detail-${match.gameId}" class="match-detail"><div class="match-detail-head"><div class="match-detail-tabs" role="tablist" aria-label="对局详情">${detailTab("overview", "概览")}${detailTab("team", "队伍分析")}${detailTab("build", "构建")}</div></div><div id="${panelID}" role="tabpanel" aria-labelledby="match-detail-tab-${active}-${match.gameId}">${active === "overview" ? (matchPlayerGroups(match).arena ? renderArenaMatchOverview(match) : renderMatchOverview(match)) : active === "team" ? renderTeamAnalysis(match, tab) : renderBuild(match, subject, tab)}</div></div>`;
  }

  function isCurrentMatchParticipant(item, match) {
    const subjectID = Number(match?.subjectParticipantId);
    return subjectID > 0 && Number(item.participantId) === subjectID;
  }





  function renderMatchOverview(match) {
    // R277 deferred detail renderer: renderMatchOverview
    return matchDetailRenderer?.renderMatchOverview(match) || "";
  }

	function renderArenaMatchOverview(match) {
    // R277 deferred detail renderer: renderArenaMatchOverview
    return matchDetailRenderer?.renderArenaMatchOverview(match) || "";
  }

	const TEAM_ANALYSIS_METRICS = Object.freeze([
	  { key: "damage", label: "伤害" },
	  { key: "damageTaken", label: "承伤" },
	  { key: "gold", label: "金钱" },
	  { key: "cs", label: "补刀", unavailableInARAM: true },
	  { key: "killParticipation", label: "击杀参与率", percent: true },
	  { key: "championLevel", label: "等级" },
	  { key: "kills", label: "击杀数" },
	  { key: "deaths", label: "死亡数" },
	  { key: "assists", label: "助攻数" },
	  { key: "wardsPlaced", label: "插眼数", summonersRiftOnly: true },
	  { key: "wardsKilled", label: "排眼数", summonersRiftOnly: true },
	  { key: "controlWardsBought", label: "真眼购买数", nullable: true, summonersRiftOnly: true },
	]);
	const TEAM_ANALYSIS_POSITIONS = Object.freeze(["top", "jungle", "middle", "bottom", "utility"]);
	const SUMMONERS_RIFT_QUEUE_IDS = Object.freeze([
	  400, 420, 430, 440, 490, 700, 720,
	  820, 830, 840, 850, 860, 870, 880, 890,
	  900, 1900,
	]);

  function isARAMRelatedMatch(match) {
    return ["aram", "mayhem", "mayhem-classic"].includes(matchModeKind(match));
  }

	function isSummonersRiftMatch(match) {
	  const mapID = Number(match?.mapId);
	  if (mapID > 0) return mapID === 11;
	  const queueID = Number(match?.queueId);
	  const modeGroup = String(match?.modeGroup || "").toLowerCase();
	  return SUMMONERS_RIFT_QUEUE_IDS.includes(queueID) || modeGroup === "solo" || modeGroup === "flex";
	}











	function renderTeamAnalysis(match, tab) {
    // R277 deferred detail renderer: renderTeamAnalysis
    return matchDetailRenderer?.renderTeamAnalysis(match, tab) || "";
  }

  /* ---------- 对局时间线（装备路线 + 技能加点） ---------- */

  function matchTimelineKey(match, subject, tab) {
    return `${riotTab(tab) ? "kr" : `cn:${tabServerID(tab) || "current"}`}:${match.gameId}`;
  }

  function recordTimelineClient(reason, diagnosticBatch = "") {
    void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({event: "match_timeline_client", reason,diagnosticBatch}),
    }).catch(() => {});
  }

  async function ensureMatchTimeline(match, subject, tab, diagnosticBatch = "") {
    const participantId = Number(subject?.participantId);
    if (!match || !participantId) { recordTimelineClient("missing-participant",diagnosticBatch); return; }
    // 国服时间线需要本机客户端（或 SGP 令牌）；未连接时不请求。
    if (!riotTab(tab) && !connected()) return;
    const key = matchTimelineKey(match, subject, tab);
    if (state.matchTimelines.get(key)?.available || state.matchTimelineFlights.has(key)) return;
    state.matchTimelines.delete(key); // Replace a previous failure with a visible loading state.
    state.matchTimelineFlights.add(key);
    let settled = false;
    try {
      const result = await api("/api/gameplay/match-timeline", {
        method: "POST",
        body: JSON.stringify({
          diagnosticBatch,gameId: Number(match.gameId), region: riotTab(tab) ? tab.region : "",
          serverId: tabServerID(tab), playerRef: tab.data?.player?.playerRef || tab.playerRef || "",
        }),
      }, `match-timeline:${key}`, 25000);
      state.matchTimelines.set(key, result && typeof result === "object" ? result : { available: false });
      if(Array.isArray(result?.tags) && result.tags.length>0)match.tagsAvailable=true;
      if (!result?.available) recordTimelineClient("unavailable",diagnosticBatch);
      settled = true;
    } catch (error) {
      if (error.name !== "RequestCancelled") {
        recordTimelineClient("request-failed",diagnosticBatch);
        state.matchTimelines.set(key, { available: false, detail: error.message });
        settled = true;
      }
    } finally {
      state.matchTimelineFlights.delete(key);
      if (settled) {
        rerenderMatch(tab, String(match.gameId));
      }
    }
  }

  // 装备路线：按分钟分组的购买/出售记录，组间用箭头衔接（同 OP.GG）。
  function renderItemRoute(timeline) {
    // R277 deferred detail renderer: renderItemRoute
    return matchDetailRenderer?.renderItemRoute(timeline) || "";
  }

  const SKILL_SLOT_LETTERS = { 1: "Q", 2: "W", 3: "E", 4: "R" };

  // 主升/副升摘要：R 之外，按“先点满”的顺序排列。
  function skillPrioritySummary(order) {
    // R277 deferred detail renderer: skillPrioritySummary
    return matchDetailRenderer?.skillPrioritySummary(order) || "";
  }

  // 技能加点：一格一次加点，格内是技能字母（按技能着色）+ 第几级，
  // 与 OP.GG 的顺序条一致，直接从左往右读。
  function renderSkillOrder(timeline) {
    // R277 deferred detail renderer: renderSkillOrder
    return matchDetailRenderer?.renderSkillOrder(timeline) || "";
  }

  // R195 Windows samples confirm Lethal Tempo damage in var2. Its first
  // template incorrectly formats the variables as time; var1 stays hidden.
  const PERK_EFFECT_OVERRIDES = Object.freeze({ 8008: ["已造成的伤害：@eogvar2@"] });







  function renderRuneYield(subject) {
    // R277 deferred detail renderer: renderRuneYield
    return matchDetailRenderer?.renderRuneYield(subject) || "";
  }

  function renderRuneEffects(subject) {
    // R277 deferred detail renderer: renderRuneEffects
    return matchDetailRenderer?.renderRuneEffects(subject) || "";
  }

  function bindRuneEffectLinks(container) {
    for (const item of container.querySelectorAll(".rune-split .rune-option-button.is-selected")) item.tabIndex = 0;
    if (container._runeEffectsBound) return;
    container._runeEffectsBound = true;
    const link = (event, active) => {
      const target = event.target.closest?.(".eff[data-perk-id], .rune-option-button.is-selected[data-perk-id]");
      const split = target?.closest(".rune-split");
      if (!split || (event.relatedTarget && target.contains(event.relatedTarget))) return;
      const fromEffect = target.classList.contains("eff");
      for (const item of split.querySelectorAll(fromEffect ? ".rune-option-button.is-selected[data-perk-id]" : ".eff[data-perk-id]")) {
        item.classList.toggle(fromEffect ? "is-linked" : "is-hover", active && item.dataset.perkId === target.dataset.perkId);
      }
    };
    for (const name of ["pointerover", "focusin"]) container.addEventListener(name, event => link(event, true));
    for (const name of ["pointerout", "focusout"]) container.addEventListener(name, event => link(event, false));

  }

  function renderBuildAugments(ids) {
    return `<div class="aug-grid">${ids.map((id, index) => {
      const augment = augmentMetadata(id) || { id, name: `海克斯 ${id}` };
      const rarity = normalizeAugmentRarity(augment.rarity);
      const entry = state.augmentDescriptions.get(Number(id));
      const description = entry?.status === "ok" ? plainText(entry.description) : "";
      const loading = !entry || entry.status === "loading";
      const tooltip = [augment.name, description].filter(Boolean).join("\n");
      return `<article class="aug is-${rarity.key}${!loading && !description ? " is-bare" : ""}" data-augment-id="${Number(id)}" data-tooltip="${escapeHTML(tooltip)}" tabindex="0"><div class="aug-icon">${augmentIconFigure(id, "large")}<span class="aug-order">${index + 1}</span></div><div class="aug-body"><div class="aug-title"><strong>${escapeHTML(augment.name)}</strong><span class="rar">${escapeHTML(rarity.label)}</span></div>${description ? `<p>${escapeHTML(description)}</p>` : loading ? '<div aria-hidden="true"><div class="skel"></div><div class="skel"></div></div>' : ""}</div></article>`;
    }).join("")}</div>`;
  }

  async function ensureAugmentDescriptions(ids, retry = false) {
    const eligible = [...new Set(ids.map(Number))].filter(id => {const e=state.augmentDescriptions.get(id);return id>0 && (!e || retry && e.status==="retry" && !e.timer && !e.inFlight);});
    if(!eligible.length || state.destroyed)return;
    const next=id=>Number(state.augmentDescriptions.get(id)?.retryCount ?? -1)+1,attempt=next(eligible[0]),missing=eligible.filter(id=>next(id)===attempt).slice(0,6);
    for(const id of missing){const old=state.augmentDescriptions.get(id);state.augmentDescriptions.set(id,{status:old?"retry":"loading",inFlight:true,retryCount:attempt});}
    let result;
    try {result=await api(`/api/gameplay/augment-descriptions?ids=${missing.join(",")}&retry=${attempt}`,{},`augment-descriptions:${missing.join(",")}:${attempt}`,30000);}catch{}
    if(state.destroyed)return;
    for(const id of missing){
      const item=result?.items?.find(item=>Number(item.id)===id);
      const entry=item?.status==="ok" && item.description ? {...item,status:"ok"} : item?.status==="unavailable" ? {status:"unavailable",source:"none",error_kind:"not_found"} : {status:"retry",source:"none",error_kind:item?.error_kind || "no_response"};
      entry.retryCount=attempt;state.augmentDescriptions.set(id,entry);
      window.reportFlowDiagnostic?.("augment_description_client",entry.status,{augment_id:id,retry_count:attempt,source:entry.source || "none",error_kind:entry.error_kind || "none",gave_up:entry.status==="retry" && attempt>=3});
      if(entry.status==="retry" && attempt<3){entry.timer=setTimeout(()=>{entry.timer=null;if(!state.destroyed)void ensureAugmentDescriptions([id],true);},[3000,10000,30000][attempt]);entry.timer?.unref?.();}
    }
    rerenderAugmentDescriptionViews(missing);
    const rest=eligible.filter(id=>!missing.includes(id));if(rest.length)await ensureAugmentDescriptions(rest,retry);
  }

  function rerenderAugmentDescriptionViews(ids) {
    const roots=new Set([document,...externalMatchViews.keys()]);
    for(const tab of [...(state.tabs || []),...(state.overlay || [])]){const workspace=tab.overlay?nodes.playerOverlayContent:overviewWorkspace(tabGroup(tab)).content;const cached=workspace?._overviewViews?.get(tab)?.content;if(cached)roots.add(cached);for(const view of tab.matchViews?.values() || [])if(view.list)roots.add(view.list);}
    for(const root of roots)for(const node of root.querySelectorAll(".build-detail .aug[data-augment-id]")){
      const id=Number(node.dataset.augmentId);if(!ids.includes(id))continue;
      const scroll=[];for(let p=node.parentElement;p;p=p.parentElement)if(p.scrollTop)scroll.push([p,p.scrollTop]);
      const template=document.createElement("template");template.innerHTML=renderBuildAugments([id]);const card=template.content.querySelector(".aug");
      card.querySelector(".aug-order").textContent=node.querySelector(".aug-order").textContent;node.replaceWith(card);for(const [p,top] of scroll)p.scrollTop=top;
    }
  }

  async function ensureBuildData(match, tab, retry = false) {
    const subject = matchSubject(match, tab.data?.player?.playerRef);
    const ids = matchAugmentIDs(buildSubject(match, subject, tab), 6);
    if (ids.length) void ensureAugmentDescriptions(ids, retry);
    if (!match.perkStatsStale || !riotTab(tab)) return;
    const key = `${tab.region}:${match.gameId}`;
    if (!state.perkStatsRefreshes.has(key)) {
      state.perkStatsRefreshes.set(key, (async () => {
        try { return await api("/api/gameplay/match", { method: "POST", body: JSON.stringify({ gameId: Number(match.gameId), participantId: Number(subject.participantId), region: tab.region, refresh: "perk-stats" }) }, `perk-stats:${key}`, 25000); }
        catch { return null; }
      })());
    }
    const fresh = await state.perkStatsRefreshes.get(key);
    if (!fresh || !match.perkStatsStale) return;
    for (const participant of match.participants || []) {
      const replacement = fresh.participants?.find(item => Number(item.participantId) === Number(participant.participantId));
      if (replacement?.perkStats) participant.perkStats = replacement.perkStats;
    }
    match.perkStatsStale = false;
    rerenderMatch(tab, String(match.gameId));
  }

  function recordBuildPlayerSelection(gameID, reason, isSelf) {
    void fetch("/api/diagnostics/client", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({event:"build_player_selection",reason,gameId:Number(gameID)||0,isSelf})}).catch(()=>{});
  }
  function resetBuildPlayerSelection(tab, id, reason) {
    // Diagnostics describe the resulting selection: clearing it restores self.
    if (id) { tab.buildPlayers?.delete(id); recordBuildPlayerSelection(id,reason,true); }
    else { for(const key of tab.buildPlayers?.keys() || []) recordBuildPlayerSelection(key,reason,true); tab.buildPlayers?.clear(); }
  }

  function buildSubject(match, subject, tab) {
    const selected = tab.buildPlayers?.get(String(match.gameId));
    return (match.participants || []).find(item => Number(item.participantId) === Number(selected)) || subject;
  }

  function renderBuildPlayers(match, self, selected) {
    if ((match.participants || []).length < 2) return "";
    const layout = matchPlayerGroups(match);
    const order = rows => [...rows].sort((a,b) => {
      const roles = ["top","jungle","middle","bottom","utility"];
      const rank = item => { const index = roles.indexOf(item.position); return index < 0 ? 99 : index; };
      return rank(a) - rank(b) || Number(a.participantId) - Number(b.participantId);
    });
    const avatar = item => {
      const own = Number(item.participantId) === Number(self?.participantId), active = Number(item.participantId) === Number(selected?.participantId);
      const title = maskedPlayerName(item, (match.participants || []).indexOf(item));
      return `<button type="button" role="tab" aria-selected="${active}" tabindex="${active ? 0 : -1}" class="build-player${active ? " is-selected" : ""}" data-build-player="${item.participantId}" data-game-id="${match.gameId}" data-tooltip="${escapeHTML(title)}\n${escapeHTML(item.championName || "")} · ${number(item.championLevel || 0)} 级">${iconFigure("champion",item.championId,item.championName,"",false)}${own ? '<i class="build-player-self" aria-hidden="true"></i>' : ""}</button>`;
    };
    const groups = layout.groups.map((group,index) => `<div class="build-player-group${layout.arena ? " is-arena-group" : ` is-team-${Number(group.teamId) === 100 ? "blue" : "red"}`}${layout.arena && group.players.some(item=>Number(item.participantId)===Number(self?.participantId)) ? " is-self-team" : ""}">${layout.arena ? `<b class="build-group-rank is-rank-${group.placement}">${group.placement || "—"}</b>` : ""}${order(group.players).map(avatar).join("")}</div>`);
    return `<div class="build-player-selector${layout.arena ? " is-arena" : ""}" role="tablist" aria-label="构建玩家">${groups.join(layout.arena ? "" : '<span class="build-player-vs">VS</span>')}</div>`;
  }

  function recordBuildSpellShape(match, subject) {
    // R277 deferred detail renderer: recordBuildSpellShape
    return matchDetailRenderer?.recordBuildSpellShape(match, subject);
  }

  function renderBuild(match, subject, tab) {
    const self = subject;
    subject = buildSubject(match, subject, tab);
    const selector = renderBuildPlayers(match, self, subject);
    const augmentIDs = matchAugmentIDs(subject, 6);
    if (augmentIDs.length) void ensureAugmentDescriptions(augmentIDs);
    const hasAugments = augmentIDs.length > 0;
    const runeContent = hasAugments
      ? renderBuildAugments(augmentIDs)
      : subject.perkIds?.length
        ? `<div class="rune-split"><div class="rune-split-tree">${renderUnifiedRuneBoard(subject)}</div>${renderRuneEffects(subject)}</div>`
        : '<div class="detail-empty"><strong>这场对局没有符文或海克斯数据</strong><p>部分娱乐模式会关闭符文系统。</p></div>';
    const allTimeline = state.matchTimelines.get(matchTimelineKey(match, subject, tab));
    const participantTimeline = allTimeline?.participants?.find(item => Number(item.participantId) === Number(subject.participantId));
    const timeline = participantTimeline ? { ...allTimeline, ...participantTimeline } : allTimeline?.participants ? { ...allTimeline, itemGroups: [], skillOrder: [] } : allTimeline;
    const timelineOfficial=timeline?.historyStatus==="official";
    const timelineLoading = timeline === undefined && (riotTab(tab) || connected());
    const unavailableCopy = (fallback) => timelineOfficial?"":`<div class="detail-empty compact"><p>${escapeHTML(timeline?.detail || fallback)}</p></div>`;
    const routeContent = timelineLoading
      ? '<p class="muted rune-loading-copy">正在读取这场对局的时间线…</p>'
      : timeline?.available && timeline.itemGroups?.length
        ? renderItemRoute(timeline)
        : unavailableCopy("这场对局暂时读取不到装备购买记录。");
    const skillContent = timelineLoading
      ? '<p class="muted rune-loading-copy">正在读取这场对局的时间线…</p>'
      : timeline?.available && timeline.skillOrder?.length
        ? renderSkillOrder(timeline)
        : unavailableCopy("这场对局暂时读取不到技能加点记录，不会用英雄默认顺序代替。");
    recordBuildSpellShape(match, subject);
    const spellIcons = [subject.spell1Id,subject.spell2Id].filter(id=>Number(id)>0).map(id=>spellIconFigure(id,"small")).join("");
    const buildSpells = spellIcons ? `<div class="build-spells">${spellIcons}</div>` : "";
    const skillSummary = timeline?.available && timeline.skillOrder?.length ? skillPrioritySummary(timeline.skillOrder) : "";
    return `<div class="build-detail">${selector}${timelineOfficial?`<div class="service-outage-inline" role="status">${serviceOutageIcon()}<span>官方战绩服务暂时中断，恢复后可重新加载</span><button type="button" class="text-button" data-timeline-retry="${escapeHTML(String(match.gameId))}">重新加载</button></div>`:""}
      <section><header><h4>装备路线</h4>${timeline && !timeline.available && !timelineOfficial ? `<button type="button" class="text-button" data-timeline-retry="${escapeHTML(String(match.gameId))}">重试加载</button>` : ""}</header>${routeContent}</section>
      <section><header><h4>技能加点</h4>${skillSummary || ""}</header><div class="build-skill-body"><div class="build-skill-order">${skillContent}</div>${buildSpells}</div></section>
      <section class="rune-detail"><header><h4>${hasAugments ? "海克斯" : "符文"}</h4>${hasAugments ? "" : renderRuneYield(subject)}</header>${runeContent}</section>
    </div>`;
  }

  function rerenderCatalogViews() {
    // Catalog arrival changes every icon even when the match objects are the
    // same. Invalidate retained match DOM before the normal scoped redraw.
    for (const tab of new Set([...(state.tabs || []), ...(state.overlay || [])])) {
      tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;
      tab.catalogViewRevision = tab.matchViewRevision;
    }
    if (state.section === "overview") renderOverview();
    if (state.section === "live") { state.liveRenderTrigger = "catalog"; renderLive(); }
    if (state.overlay.length) renderOverlay();
    for (const [container, view] of externalMatchViews) {
      if (!container.isConnected) externalMatchViews.delete(container);
      else view.render();
    }
  }

  function scheduleCatalogViews() {
    if (state.destroyed || state.catalogRenderPending) return;
    state.catalogRenderPending = true;
    const flush = () => {
      state.catalogRenderPending = false;
      if (!state.destroyed) rerenderCatalogViews();
    };
    if (typeof requestAnimationFrame === "function") requestAnimationFrame(flush);
    else flush();
  }

  // 图标目录（符文/装备/召唤师技能）：客户端连接时来自本机客户端；
  // 未连接时后端会回退到 Data Dragon，因此不再限制连接状态。
  async function ensureAugments() {
    if (state.destroyed || state.augmentCatalog || state.augmentCatalogLoading) return;
    state.augmentCatalogLoading = true;
    try {
      const result = await api("/api/gameplay/augments", {}, "augments", 5000);
      if (state.destroyed || !Array.isArray(result?.augments) || !result.augments.length) return;
      state.augmentCatalog = new Map(result.augments.map((item) => [Number(item.id), item]));
      scheduleCatalogViews();
    } catch (_) {
      // The full perk catalog remains the existing fallback for this index.
    } finally {
      state.augmentCatalogLoading = false;
    }
  }

  async function ensurePerks(force = false) {
    if (!force && (state.perks || state.perksLoading)) return;
    const requestToken = Number(state.perksRequestToken || 0) + 1;
    state.perksRequestToken = requestToken;
    state.perksLoading = true;
    try {
      const result = await api("/api/gameplay/perks", {}, "perks", 15000);
      if (state.perksRequestToken !== requestToken) return;
      const hasNestedPerks = Array.isArray(result?.styles) && result.styles.some((style) => (style.slots || []).some((slot) => (slot.perks || []).length));
      const hasFlatPerks = Array.isArray(result?.perks) && result.perks.length > 0;
      state.perks = Array.isArray(result?.styles) && result.styles.length && (hasNestedPerks || hasFlatPerks) ? result : null;
    }
    catch (_) { if (state.perksRequestToken === requestToken) state.perks = null; }
    finally {
      if (state.perksRequestToken === requestToken) {
        state.perksLoading = false;
        if (state.perks) scheduleCatalogViews();
        clearTimeout(state.perksAugmentTimer);
        if (state.perks?.augmentsPending && !state.destroyed) state.perksAugmentTimer = setTimeout(() => { if (!state.destroyed) void ensurePerks(true); }, 1000);
      }
    }
  }

  window.addEventListener("deep-legends:augment-index-updated", () => {
    if (state.perks && !state.perksLoading && !state.destroyed) void ensurePerks(true);
  });

  async function ensureItems() {
    if (state.destroyed || state.items || state.itemsLoading || Date.now() < Number(state.itemsRetryAt || 0)) return;
    const requestToken = Number(state.itemsRequestToken || 0) + 1;
    state.itemsRequestToken = requestToken;
    state.itemsLoading = true;
    try {
      const result = await api("/api/gameplay/items", {}, "items", 15000);
      if (state.itemsRequestToken !== requestToken) return;
      if (!Array.isArray(result?.items) || !result.items.length) throw Object.assign(new Error("empty catalog"), { kind: "empty" });
      state.items = result;
      state.itemsMissingIDs = new Set();
      recordItemSetClientDiagnostic("catalog_client", "loaded", { endpoint: "items", httpStatus: 200, itemCount: result.items.length, errorKind: "none" });
    }
    catch (error) {
      if (state.itemsRequestToken !== requestToken) return;
      state.items = null;
      recordItemSetClientDiagnostic("catalog_client", "failed", { endpoint: "items", httpStatus: Number(error?.status || 0), errorKind: error?.errorKind || error?.kind || (error?.name === "AbortError" ? "timeout" : error?.status ? "http" : "network") });
    }
    finally {
      if (state.itemsRequestToken !== requestToken) return;
      state.itemsLoading = false;
      if (state.items) {
        state.itemsRetryAt = 0;
        scheduleCatalogViews();
      } else {
        state.itemsRetryAt = Date.now() + 30_000;
        clearTimeout(state.itemsRetryTimer);
        state.itemsRetryTimer = setTimeout(() => { void ensureItems(); }, 30_000);
      }
    }
  }

  async function ensureSummonerSpells() {
    if (state.summonerSpells || state.summonerSpellsLoading || Date.now() < Number(state.summonerSpellsRetryAt || 0)) return;
    const requestToken = Number(state.summonerSpellsRequestToken || 0) + 1;
    state.summonerSpellsRequestToken = requestToken;
    state.summonerSpellsLoading = true;
    try {
      const result = await api("/api/gameplay/summoner-spells", {}, "summoner-spells", 15000);
      if (state.summonerSpellsRequestToken !== requestToken) return;
      state.summonerSpells = Array.isArray(result?.spells) && result.spells.length ? result : null;
    }
    catch (_) { if (state.summonerSpellsRequestToken === requestToken) state.summonerSpells = null; }
    finally {
      if (state.summonerSpellsRequestToken !== requestToken) return;
      state.summonerSpellsLoading = false;
      if (state.summonerSpells) {
        state.summonerSpellsRetryAt = 0;
        scheduleCatalogViews();
      } else {
        state.summonerSpellsRetryAt = Date.now() + 30_000;
        setTimeout(() => {
          if (!state.summonerSpells && !state.summonerSpellsLoading) ensureSummonerSpells();
        }, 30_000);
      }
    }
  }

  // 从玩家按钮中提取干净的显示名：图标占位符（aria-hidden 的首字母）
  // 不能混入，否则新页签的名称会莫名多出一个字。
  function playerButtonLabel(button) {
    const named = button.matches(".live-player-name") ? button : button.querySelector(".match-player-name, .participant-name, .recent-player-name");
    const label = (named?.textContent || button.dataset.tooltip || "").trim();
    if (label) return label;
    const clone = button.cloneNode(true);
    for (const hidden of clone.querySelectorAll('[aria-hidden="true"], .game-icon')) hidden.remove();
    return clone.textContent.trim();
  }

  function bindPlayerLinks(container, tab, beforeOpen) {
    // 点击页面内的玩家名称时沿用当前页面所属服务器（两服玩家互不相通）；
    // 在覆盖层或非总览页内点击时，继续以覆盖层方式打开。
    const sourceRegion = tab.region || "";
    const sourceServerID = tabServerID(tab);
    for (const button of container.querySelectorAll("[data-player-ref]")) button.addEventListener("click", () => {
      const label = playerButtonLabel(button);
      beforeOpen?.();
      if (tab.overlay) openPlayerOverlay({ playerRef: button.dataset.playerRef, region: sourceRegion, serverId: sourceServerID, label, ...proContextFromButton(button) });
      else openPlayer(button.dataset.playerRef, label, sourceRegion, sourceServerID, proContextFromButton(button));
    });
  }


  function ensureHistoryFilterStyles() {
    if(state.historyFilterStylesReady)return Promise.resolve();
    return Promise.resolve(globalThis.deepLegendsSections?.loadStyle('history-filters')).then(()=>{state.historyFilterStylesReady=true;});
  }
  async function ensureAdvancedFilters(preload=false) {
    if(state.destroyed)return;
    const styles=preload?Promise.resolve():ensureHistoryFilterStyles();
    if(globalThis.deepLegendsHistoryFilters)return styles;
    if(state.historyFiltersFlight)return Promise.all([state.historyFiltersFlight,styles]);
    state.historyFiltersFlight=Promise.all([styles,new Promise((resolve,reject)=>{
      const script=document.createElement("script");script.src="/history-filters.js";
      script.onload=()=>{if(globalThis.deepLegendsHistoryFilters)resolve();else reject(new Error("筛选加载失败"));};
      script.onerror=()=>{script.remove();reject(new Error("筛选加载失败"));};document.head.append(script);
    })]).then(()=>{
      for(const tab of [...state.tabs,...state.overlay])if(!tab.closed && tab.data && overviewContainer(tab))renderFilteredMatchView(tab);
    }).catch(()=>{state.historyFiltersFlight=null;showToast("筛选加载失败，请重试");});
    return state.historyFiltersFlight;
  }
  window.deepLegendsLoadHistoryFilters=ensureAdvancedFilters;

  function resetPlayerHistoryConditions(tab) {
    cancelAdvancedMatchSearch(tab);
    for(const prefix of ["overview", "overview-more", "overview-search"])state.controllers.get(`${prefix}:${tab.key}`)?.abort();
    tab.overviewRequestToken=Number(tab.overviewRequestToken || 0)+1;
    tab.filterPagingToken=Number(tab.filterPagingToken || 0)+1;
    tab.filterPaging=false;tab.filterPagingPage=0;
    tab.advancedConditions={};tab.advancedMenu=null;tab.advancedSearch=null;
    for(const view of tab.matchViews?.values() || [])disposeMatchRowHeight(view.list);
    tab.matchViews=new Map();tab.matchScrollTops=new Map();
    tab.openMatches=new Set();tab.matchDetailTabs=new Map();tab.buildPlayers=new Map();
    tab.nextBegIndex=0;tab.matchViewRevision=Number(tab.matchViewRevision || 0)+1;
  }

  function cancelAdvancedMatchSearch(tab, waitForSettle = false) {
    if(!tab)return;
    const search=tab.advancedSearch;
    globalThis.deepLegendsHistoryFilters?.cancel(tab,()=>{
      state.controllers.get(`overview-more:${tab.key}`)?.abort();
      state.controllers.get(`overview-search:${tab.key}`)?.abort();
      tab.overviewRequestToken=Number(tab.overviewRequestToken || 0)+1;
      tab.loadingMore=false;
    });
    if(search && !waitForSettle)search.running=false;
  }

  async function ensureHistoryFilterSearch(tab) {
    if(state.historyFilterCatalog || state.historyFilterCatalogFlight)return;
    const catalog=window.deepLegendsChampionSearch?.getCatalog?.();
    if(catalog?.champions) {state.historyFilterCatalog=catalog;return;}
    state.historyFilterCatalogFlight=true;
    try {const value=await api("/api/champions/catalog",{},"history-filter-catalog",25000);if(Array.isArray(value?.champions))state.historyFilterCatalog=value;}
    catch(error){if(error.name!=="RequestCancelled")showToast("英雄搜索读取失败");}
    finally {state.historyFilterCatalogFlight=false;}
    if(tab?.advancedMenu?.open && !tab.closed && overviewContainer(tab)) {
      globalThis.deepLegendsHistoryFilters?.refresh(overviewContainer(tab),tab,advancedFilterContext(tab));
    }
  }

  function advancedHistoryState(tab) {
    const signature=JSON.stringify([tab.matchFilter || 'all',tab.advancedConditions || {}]);
    if(tab.advancedHistory?.signature!==signature)tab.advancedHistory={signature,next:0,hasMore:true,seen:new Set(),scanned:new Set((tab.data?.matches || []).map(m=>String(m.gameId)))};
    return tab.advancedHistory;
  }
  function advancedHistoryHasMore(tab) {
    const scan=advancedHistoryState(tab);
    return !scan.active && tab.data?.pagination?.hasMore || (scan.active || (tab.matchFilter || 'all')!=='all') && scan.hasMore;
  }
  async function loadAdvancedHistory(tab,count,range={}) {
    const scan=advancedHistoryState(tab),bounded=riotTab(tab) && range.endTime>0;
    if(!scan.active && tab.data?.pagination?.hasMore && !bounded) {
      const before=new Set((tab.data?.matches || []).map(m=>String(m.gameId)));
      const loaded=await loadOverview(tab,false,true,true,false,count);
      scan.lastScanned=(tab.data?.matches || []).filter(m=>!before.has(String(m.gameId))).length;
      for(const m of tab.data?.matches || [])scan.scanned.add(String(m.gameId));
      return loaded;
    }
    scan.active=true;
    if(!scan.hasMore || tab.closed)return false;
    const generation=tab.overviewRequestToken,body={playerRef:tab.playerRef || tab.data?.player?.playerRef,count,begIndex:scan.next,matchFilter:'all',region:tab.region || '',serverId:tab.serverId || '',...(bounded?range:{})};
    const payload=await api('/api/gameplay/overview',{method:'POST',body:JSON.stringify(body)},`overview-search:${tab.key}`,riotTab(tab)?190000:45000);
    if(tab.closed || tab.overviewRequestToken!==generation || tab.advancedHistory!==scan)return false;
    const pagination=normalizedPagination(payload,scan.next),rows=payload.matches || [];
    scan.next=pagination.nextBegIndex;scan.hasMore=Boolean(pagination.hasMore);
    scan.lastScanned=0;
    for(const match of rows){const id=String(match.gameId);if(!scan.seen.has(id)){scan.seen.add(id);scan.lastScanned++;}scan.scanned.add(id);}
    const merged=new Map((tab.data?.matches || []).map(m=>[String(m.gameId),m]));
    for(const match of rows)merged.set(String(match.gameId),match);
    tab.data={...tab.data,matches:[...merged.values()].sort((a,b)=>Number(b.createdAt)-Number(a.createdAt))};
    tab.lastOverviewPageBytes=new TextEncoder().encode(JSON.stringify(payload)).length;
    if(!rows.length && scan.hasMore)throw Error('历史分页没有推进');
    return true;
  }

  function advancedFilterContext(tab) {
    const af=globalThis.deepLegendsHistoryFilters;
    const rows=()=>filteredMatches(tab.data?.matches || [],{...tab,advancedConditions:{}});
    const isActive=()=>!state.destroyed && !tab.closed && (tab.overlay ? state.overlay.at(-1)===tab : activeTab(overviewGroupForSection())===tab) && (!tab.current || connected());
    return {rows,version:()=>tab.data?.matches || tab,subject:match=>matchSubject(match,tab.data?.player?.playerRef || tab.playerRef),tags:matchDataTags,
      seasonStart:tab.data?.seasonStart,storage:{getItem:key=>localStorage.getItem(key),setItem:(key,value)=>localStorage.setItem(key,value),removeItem:key=>localStorage.removeItem(key)},
      hasMore:()=>advancedHistoryHasMore(tab),pageScanned:()=>advancedHistoryState(tab).lastScanned,scannedCount:()=>advancedHistoryState(tab).scanned.size,
      cursor:()=>tab.advancedHistory?.active?tab.advancedHistory.next:Number(tab.nextBegIndex || Number(tab.data?.pagination?.begIndex || 0)+Number(tab.data?.pagination?.count || 0)),
      heroSearch:(query,id,label)=>window.deepLegendsChampionSearch?.scoreOption(query,id,label) || window.deepLegendsRuntime.scoreChampionSearchOption(query,id,label,state.historyFilterCatalog?.champions?.find(row=>Number(row.id)===Number(id))),
      ensureSearch:()=>{void ensureHistoryFilterSearch(tab);},
      stylesReady:()=>Boolean(state.historyFilterStylesReady),ensureStyles:ensureHistoryFilterStyles,
      icon:(id,label)=>iconFigure("champion",id,label,"small",false),positionIcon,multiTag:renderMultiKillTag,
      container:()=>overviewContainer(tab),prepare:prepareImages,error:showToast,
      diagnostic:(reason,fields)=>window.reportFlowDiagnostic?.(reason==='history_search_client'?reason:'overview_rerender_client',reason==='history_search_client'?fields.stop_reason:reason,fields),
      flush:()=>{if(tab.overviewRenderDeferred){tab.overviewRenderDeferred=false;rerenderTab(tab,'filter_closed');}},
      isActive,
      stop:()=>cancelAdvancedMatchSearch(tab,true),foreign:()=>riotTab(tab),outage:()=>Boolean(historyServiceState(tab)),quotaUntil:()=>tab.quotaRetry?.retryAt || tab.searchQuotaUntil || 0,pageBytes:()=>tab.lastOverviewPageBytes || 0,
      load:(count,range)=>loadAdvancedHistory(tab,count,range),
      find:()=>af.find(tab,advancedFilterContext(tab)),
      render:()=>{if(isActive()) {renderFilteredMatchView(tab);prepareImages(overviewContainer(tab));}},
      progress:()=>{if(isActive()){appendOverviewMatches(tab,[]);clearTimeout(tab.searchCountdownTimer);const seconds=Math.ceil((tab.searchQuotaUntil-Date.now())/1000);if(seconds>0)tab.searchCountdownTimer=setTimeout(()=>{if(!tab.closed && isActive())advancedFilterContext(tab).progress();},1000);}},
      change:()=>{cancelAdvancedMatchSearch(tab);
        const container=overviewContainer(tab),deferred=tab.overviewRenderDeferred;
        tab.filteredVisibleCount=20;tab.matchViewRevision=Number(tab.matchViewRevision || 0)+1;
        tab.overviewRenderDeferred=false;
        if(!deferred && isActive() && container?._overviewViewTab===tab && container._afBoundTab===tab && !tab.overviewSubpage)renderFilteredMatchView(tab);
        else rerenderTab(tab,'filter_commit');
        prepareImages(overviewContainer(tab));
        if(af.active(tab))void af.find(tab,advancedFilterContext(tab));},
    };
  }

  function bindMatchFilterControls(container, tab) {
    container.querySelector("[data-history-filter-load]")?.addEventListener("click",async()=>{
      tab.advancedMenu = { ...tab.advancedMenu, open:true, page:"conditions", category:"", query:"", renaming:-1 };
      await ensureAdvancedFilters();
    });
    for (const button of container.querySelectorAll("[data-match-filter]")) button.addEventListener("click", () => updateMatchFilter(tab, button.dataset.matchFilter));
    if (globalThis.deepLegendsHistoryFilters && container.querySelector('[data-af-root]')) {globalThis.deepLegendsHistoryFilters.bind(container,tab,()=>advancedFilterContext(tab));globalThis.deepLegendsHistoryFilters.decorate(container,tab,advancedFilterContext(tab));}
    bindAppSelect(container.querySelector("[data-match-more-select]"), (value) => {
      if (value) updateMatchFilter(tab, value);
    });
  }

  function stashMatchFilterDOM(tab, view) {
    const container=overviewContainer(tab),list=container?.querySelector(".match-list");
    if(!list)return;
    const fragment=document.createDocumentFragment();
    const placeholder=list.cloneNode(false);list.replaceWith(placeholder);fragment.appendChild(list);
    Object.assign(view,{fragment,list,openMatches:new Set(tab.openMatches),matchDetailTabs:new Map(tab.matchDetailTabs),buildPlayers:new Map(tab.buildPlayers || [])});
    while(tab.matchViews.size>4) {const oldest=tab.matchViews.keys().next().value;tab.matchViews.delete(oldest);}
  }

  function restoreMatchFilterDOM(tab, view) {
    if(tab.advancedMenu?.open){tab.overviewRenderDeferred=true;globalThis.deepLegendsHistoryFilters?.refresh(overviewContainer(tab),tab,advancedFilterContext(tab));return false;}
    const container=overviewContainer(tab),list=container?.querySelector(".match-list");
    if(!list || !view.list)return false;
    list.replaceWith(view.list);view.fragment?.appendChild(list);
    tab.openMatches=new Set(view.openMatches || []);tab.matchDetailTabs=new Map(view.matchDetailTabs || []);tab.buildPlayers=new Map(view.buildPlayers || []);
    const filterbar=container.querySelector(".match-filterbar");if(filterbar)filterbar.outerHTML=renderMatchFilters(tab);
    bindMatchFilterControls(container,tab);
    container._matchListMatches=tab.data.matches;container._matchListFilter=tab.matchFilter;container._matchListViewRevision=Number(tab.matchViewRevision || 0);
    if(globalThis.deepLegendsHistoryFilters) {reconcileFilteredMatchList(view.list,tab);globalThis.deepLegendsHistoryFilters.decorate(view.list,tab,advancedFilterContext(tab));}
    bindMatchSentinel(container,tab);restoreMatchScrollTop(tab);
    requestAnimationFrame(()=>fitMatchTags(view.list,true));
    return true;
  }

  async function probeOverviewHead(tab) {
    if(tab.headProbeRunning || tab.loading || tab.loadingMore || !tab.data || historyServiceState(tab) || tab.quotaRetry)return;
    tab.headProbeRunning=true;
    const token=tab.overviewRequestToken,filter=tab.matchFilter,before=tab.data.matches?.length || 0;
    window.reportFlowDiagnostic?.('overview_reload_reason','section-return',{matches_before:before,matches_after:before});
    try{
      const payload=await api('/api/gameplay/history-head',{method:'POST',body:JSON.stringify({playerRef:tab.data.player?.playerRef || tab.playerRef,serverId:tab.serverId || '',count:5,matchFilter:filter,freshHistory:true})},`head-probe:${tab.key}`,25000);
      if(tab.closed || token!==tab.overviewRequestToken || filter!==tab.matchFilter)return;
      const seen=new Set(tab.data.matches.map(m=>String(m.gameId))),additions=(payload.matches || []).filter(m=>!seen.has(String(m.gameId)));
      if(additions.length){tab.data={...tab.data,matches:[...additions,...tab.data.matches]};renderFilteredMatchView(tab);}
      tab.loadedAt=Date.now();
      window.reportFlowDiagnostic?.('overview_reload_reason','head-probe',{matches_before:before,matches_after:tab.data.matches.length});
    }catch(_){}finally{tab.headProbeRunning=false;}
  }

  async function refreshMatchFilterHead(tab, filter, view) {
    if(tab.closed || view.refreshing)return;
    view.refreshing=true;
    try {
      const count=Number(state.settings?.matchCount || 20),playerRef=tab.data?.player?.playerRef || tab.playerRef;
      const data=await api("/api/gameplay/overview",{method:"POST",body:JSON.stringify({playerRef,serverId:tab.serverId || "",count,begIndex:0,matchFilter:filter,freshHistory:true})},`filter-head:${tab.key}:${filter}`,25000);
      if(tab.closed || tab.matchViews.get(filter)!==view)return;
      const known=new Set((view.matches || []).map(match=>String(match.gameId)));
      const additions=(data.matches || []).filter(match=>!known.has(String(match.gameId)));
      view.at=Date.now();
      if(!additions.length)return;
      view.matches=[...additions,...view.matches];
      const player=tab.data?.player?.playerRef || tab.playerRef;
      const fragment=document.createDocumentFragment();
      for(const match of additions){const template=document.createElement("template");template.innerHTML=renderMatch(match,player,tab);const entry=template.content.firstElementChild;bindOverviewContent(entry,tab);prepareImages(entry);fragment.appendChild(entry);}
      view.list.prepend(fragment);view.list._matchData=new Map(view.matches.map(match=>[String(match.gameId),match]));
      if(tab.matchFilter===filter) {tab.data={...tab.data,matches:view.matches};const container=overviewContainer(tab);if(container)container._matchListMatches=view.matches;fitMatchTags(view.list);}
    } catch(error) {if(error.name!=="RequestCancelled")view.refreshFailed=true;}
    finally {view.refreshing=false;}
  }

	async function updateMatchFilter(tab, value) {
    if (!value || value === tab.matchFilter) return;
    if(globalThis.deepLegendsHistoryFilters) cancelAdvancedMatchSearch(tab);
    try {
    rememberMatchScrollTop(tab, tab.matchFilter);
    if (!(tab.matchViews instanceof Map)) tab.matchViews = new Map();
    if (tab.data && !tab.loading && !tab.loadingMore && !tab.initialPagePending && !tab.quotaRetry) {
      const previous=tab.matchFilter || "all";
      const view={ matches: tab.data.matches, pagination: tab.data.pagination, nextBegIndex: tab.nextBegIndex, at: tab.matchViews.get(previous)?.at || Date.now() };
      tab.matchViews.delete(previous);tab.matchViews.set(previous,view);
      stashMatchFilterDOM(tab,view);
    }
    clearTimeout(tab.quotaRetry?.timer);
    tab.quotaRetry = null;
    tab.initialPagePending = false;
    tab.matchFilter = value;
    tab.openMatches.clear();
    tab.matchDetailTabs.clear();
    resetBuildPlayerSelection(tab, "", "reset-filter");
    tab.paginationStalls = 0;
    clearTimeout(tab.appendTimer);
    tab.appendTimer = 0;
    tab.appendFramePending = false;
    const observerKey = matchObserverKey(tab);
    state[observerKey]?.disconnect();
    state[observerKey] = null;
	if (tab.data?.pagination?.hasMore) {
	  tab.data.pagination = {
		...tab.data.pagination,
		autoPaused: false,
		pauseReason: "",
		moreError: "",
	  };
	}
	const token = Number(tab.filterPagingToken || 0) + 1;
	tab.filterPagingToken = token;
    const cachedView = tab.matchViews?.get(value);
    if (cachedView && Date.now() - cachedView.at < 300_000) {
      state.controllers.get(`overview:${tab.key}`)?.abort();
      state.controllers.get(`overview-more:${tab.key}`)?.abort();
      state.controllers.get(`overview-search:${tab.key}`)?.abort();
      tab.overviewRequestToken = Number(tab.overviewRequestToken || 0) + 1;
      tab.loading = false; tab.loadingMore = false;
      delete tab.overviewMatchSnapshot;
      tab.data = { ...tab.data, matches: cachedView.matches, pagination: cachedView.pagination };
      tab.nextBegIndex = cachedView.nextBegIndex;
      tab.filterPaging = false; tab.filterPagingPage = 0;
      tab.matchViews.delete(value);tab.matchViews.set(value,cachedView);
      if(!restoreMatchFilterDOM(tab,cachedView))renderFilteredMatchView(tab, true);
      if(Date.now()-cachedView.at>30_000)setTimeout(()=>{void refreshMatchFilterHead(tab,value,cachedView);},0);
      return;
    }
    // A complete unfiltered dataset needs no new server pagination coordinate.
    // Partial/server-filtered datasets retain their existing server request path.
    if (tab.data?.pagination && !tab.data.pagination.hasMore && !tab.data.pagination.partial && !tab.data.pagination.serverFiltered) {
      tab.filterPaging = false;
      tab.filterPagingPage = 0;
      renderFilteredMatchView(tab, true);
      return;
    }
	tab.filterPaging = true;
	tab.filterPagingPage = 1;
	renderFilteredMatchView(tab, true);
	const loaded = await loadOverview(tab, true, false, false, true);
	if (tab.filterPagingToken !== token || tab.matchFilter !== value) return;
	tab.filterPaging = false;
	tab.filterPagingPage = 0;
	if (loaded && !tab.data?.pagination?.serverFiltered && tab.data?.pagination?.filterFallback) {
	  await autoLoadMatchFilter(tab, value);
	  return;
	}
	renderFilteredMatchView(tab);
    } finally { if(globalThis.deepLegendsHistoryFilters?.active(tab)) void globalThis.deepLegendsHistoryFilters.find(tab,advancedFilterContext(tab)); }
  }

  async function autoLoadMatchFilter(tab, filter = tab.matchFilter) {
    if(globalThis.deepLegendsHistoryFilters?.active(tab)) return;
	if (tab.data?.pagination?.serverFiltered) return;
	const token = Number(tab.filterPagingToken || 0) + 1;
	tab.filterPagingToken = token;
	tab.filterPaging = true;
    let pages = 0;
	try {
	  while (tab.filterPagingToken === token && tab.matchFilter === filter) {
		const pagination = tab.data?.pagination;
		const visible = filteredMatches(tab.data?.matches || [], tab).length;
		if (!pagination?.hasMore || pagination.autoPaused || visible >= state.settings.matchCount) break;
        if (pages >= 3) {
          tab.data.pagination = { ...pagination, autoPaused: true, pauseReason: "当前已查询范围内没有更多该模式对局，可继续查找" };
          break;
        }
		if (tab.loading || tab.loadingMore) {
		  await new Promise((resolve) => setTimeout(resolve, 50));
		  continue;
		}
		tab.filterPagingPage = Math.floor(Math.max(0, Number(tab.nextBegIndex || 0)) / Math.max(1, state.settings.matchCount)) + 1;
		renderFilteredMatchView(tab);
		const loaded = await loadOverview(tab, false, true);
        pages++;
		if (!loaded) break;
		const delay = Math.max(0, Number(tab.nextAutoAppendAt || 0) - Date.now());
		if (delay > 0) await new Promise((resolve) => setTimeout(resolve, delay));
	  }
	} finally {
	  if (tab.filterPagingToken === token) {
		tab.filterPaging = false;
		tab.filterPagingPage = 0;
		renderFilteredMatchView(tab);
	  }
	}
  }

  function matchScrollRoot(tab) {
    const container = overviewContainer(tab);
    return container?.closest(".player-overlay-scroll") || document.getElementById("app-scroll");
  }

  function rememberMatchScrollTop(tab, filter) {
    const root = matchScrollRoot(tab);
    if (!root || !filter) return;
    if (!(tab.matchScrollTops instanceof Map)) tab.matchScrollTops = new Map();
    tab.matchScrollTops.set(filter, root.scrollTop);
  }

  function restoreMatchScrollTop(tab) {
    const filter = tab.matchFilter;
    const scrollTop = Number(tab.matchScrollTops?.get(filter) || 0);
    requestAnimationFrame(() => {
      if (tab.matchFilter !== filter) return;
      const root = matchScrollRoot(tab);
      if (root) root.scrollTop = scrollTop;
    });
  }

  function renderFilteredMatchView(tab, restoreScroll = false) {
    const container = overviewContainer(tab);
    const data = tab.data;
    if (!container || !data) return;
    if(tab.advancedMenu?.open && container._afBoundTab===tab){tab.overviewRenderDeferred=true;globalThis.deepLegendsHistoryFilters?.refresh(container,tab,advancedFilterContext(tab));return;}
    const filterbar = container.querySelector(".match-filterbar");
    if (filterbar) filterbar.outerHTML = renderMatchFilters(tab);
    bindMatchFilterControls(container, tab);
    const list = container.querySelector(".match-list");
    if (list) {
      reconcileFilteredMatchList(list, tab);
    }
    appendOverviewMatches(tab, [], true);
    if (restoreScroll) restoreMatchScrollTop(tab);
  }

  function reportMatchFilterView(tab,visible) {
    const allRows=filteredMatches(tab.matchViews?.get('all')?.matches || [],tab),ids=new Set((tab.data?.matches || []).map(m=>String(m.gameId)));
    window.reportFlowDiagnostic?.('match_filter_view','render',{filter:tab.matchFilter || 'all',source:tab.data.pagination?.serverFiltered?'server':tab.data.pagination?.filterFallback?'client-fallback':'view-cache',count:visible.length,latest_game_id:visible[0]?.gameId || 0,latest_ended_at:visible[0]?.endedAt || 0,all_list_count:allRows.length,missing_count:allRows.filter(m=>!ids.has(String(m.gameId))).length});
  }
  function retainFallbackFilterMatches(tab) {
    if(!tab.data?.pagination?.filterFallback || (tab.matchFilter || 'all')==='all')return;
    const all=tab.matchViews?.get('all')?.matches || [],known=filteredMatches(all,tab);
    const merged=mergeCurrentHistoryHead(known,tab.data.matches || []);
    tab.data.matches=merged.sort((a,b)=>Number(b.createdAt)-Number(a.createdAt) || Number(b.gameId)-Number(a.gameId));
  }
  function reconcileFilteredMatchList(list, tab) {
    if(!list)return;
    retainFallbackFilterMatches(tab);
    const raw=tab.data?.matches || [],advanced=globalThis.deepLegendsHistoryFilters?.active(tab);
    const previous=list._matchData || new Map(raw.map(match=>[String(match.gameId),match]));
    const current=new Map(raw.map(match=>[String(match.gameId),match]));
    const allVisible=filteredMatches(raw,tab),visible=advanced?allVisible.slice(0,Number(tab.filteredVisibleCount || 20)):allVisible;
    const visibleIDs=new Set(visible.map(match=>String(match.gameId)));
    const entries=new Map([...list.children].filter(entry=>entry.classList.contains('match-entry') && !entry.classList.contains('match-pending')).map(entry=>[entry.dataset.matchId,entry]));
    const cache=tab.filteredNodeCache ||= new Map(),removed=new Set();
    for(const [id,entry] of entries) {
      if(!current.has(id) || !visibleIDs.has(id)) {
        removed.add(entry);entries.delete(id);
        if(advanced && current.has(id)){cache.delete(id);cache.set(id,{entry,match:previous.get(id)});if(cache.size>60)cache.delete(cache.keys().next().value);}
      }
    }
    if(removed.size>50) {
      const parent=list.parentNode,next=list.nextSibling,remaining=[...list.children].filter(node=>!removed.has(node));
      list.remove();list.replaceChildren(...remaining);parent?.insertBefore(list,next);
    } else for(const node of removed)node.remove();
    while(cache.size>60)cache.delete(cache.keys().next().value);
    const playerRef=tab.data?.player?.playerRef || tab.playerRef || '';
    let revisionSkipped=0,imagesRecreated=0;
    const beforeLP=new Set([...previous].filter(([,m])=>m.lpDelta!=null).map(([id])=>id));
    for(const match of visible) {
      const id=String(match.gameId),cached=cache.get(id);
      let entry=entries.get(id),old=previous.get(id);
      if(!entry && cached?.match===match){entry=cached.entry;old=cached.match;cache.delete(id);entries.set(id,entry);}
      const changed=old!==match && (!old || matchContentVersion(old)!==matchContentVersion(match));
      if(entry && !changed)patchLPChip(entry,match);
      const wasOpen=entry?.querySelector('[data-toggle-match][aria-expanded="true"]');
      if(entry && !changed && (!wasOpen || tab.openMatches.has(id))){if(old?.revision===match.revision)revisionSkipped++;continue;}
      const template=document.createElement('template');template.innerHTML=renderMatch(match,playerRef,tab);
      const replacement=template.content.firstElementChild;imagesRecreated+=replacement.querySelectorAll("img").length;bindOverviewContent(replacement,tab);applyRenderedMetricStyles(replacement);prepareImages(replacement);
      if(entry?.parentNode===list)entry.replaceWith(replacement);entries.set(id,replacement);
    }
    let position=0;
    for(const match of visible){const entry=entries.get(String(match.gameId));entry.hidden=false;if(list.children[position]!==entry){const placeholder=list.children[position];if(placeholder?.classList.contains('match-pending'))placeholder.replaceWith(entry);else list.insertBefore(entry,placeholder || null);}position++;}
    if(visible.length){for(const child of [...list.children])if(!child.matches('.match-entry') || !tab.loading && child.classList.contains('match-pending'))child.remove();}
    else {
      const capability=(tab.data?.capabilities || []).find(item=>item.name==='match-history');
      const html=matchListEmptyContent(tab,capability,tab.matchFilter==='more:special'?'自定义对局不会展示；这里只保留客户端实际返回的其他特殊模式。':'可以切换上方游戏类型，或刷新读取最新战绩。');
      if(list._emptyHTML!==html || list.querySelector('.match-entry') || !list.firstChild)list.innerHTML=html;
      list._emptyHTML=html;
    }
    if(globalThis.deepLegendsHistoryFilters)globalThis.deepLegendsHistoryFilters.decorate(list,tab,advancedFilterContext(tab));
    reportMatchFilterView(tab,visible);
    list._matchData=current;
    boundMatchDOM(list,tab);
    window.reportFlowDiagnostic?.('lp_chip_render','render',{ranked_matches:visible.filter(m=>[420,440].includes(Number(m.queueId))).length,with_delta:visible.filter(m=>m.lpDelta!=null).length,chips:list.querySelectorAll('.lp-delta').length,revision_skipped:revisionSkipped,source:tab.lpRenderSource || "complete",lost:[...beforeLP].filter(id=>current.has(id) && current.get(id).lpDelta==null).length,images_recreated:imagesRecreated});
  }

  function rerenderMatch(tab, id) {
    if (state.destroyed) return;
    if (typeof tab?.externalRender === "function") return tab.externalRender(id);
    const workspace = tab.overlay ? nodes.playerOverlayContent : overviewWorkspace(tabGroup(tab)).content;
    const container = overviewContainer(tab);
    // Inactive views retain their DOM in a fragment. Update that same detail so
    // switching back does not restore a loading skeleton or rebuild the list.
    const cached = workspace?._overviewViews?.get(tab)?.content;
    const scope = tab.overlay && state.overlay[state.overlay.length - 1] !== tab ? cached : container || cached;
    const entry = [...(scope?.querySelectorAll(".match-entry") || [])].find(item => item.dataset.matchId === String(id));
    if (entry) {replaceMatchEntry(entry,tab,()=>rerenderTab(tab,"rerenderMatch"),scope===cached);return;}
    for(const [filter,view] of tab.matchViews || []) {
      const retainedEntry=view.list?.querySelector(`[data-match-id="${id}"]`);
      if(!retainedEntry)continue;
      const retainedTab={...tab,matchFilter:filter,data:{...tab.data,matches:view.matches},openMatches:view.openMatches || tab.openMatches,matchDetailTabs:view.matchDetailTabs || tab.matchDetailTabs,buildPlayers:view.buildPlayers || tab.buildPlayers};
      replaceMatchEntry(retainedEntry,retainedTab,()=>{},true);
    }
  }

  function replaceMatchEntry(entry, tab, rerender, retained = false) {
    const id = String(entry?.dataset.matchId || "");
    const match = (tab.data?.matches || []).find((item) => String(item.gameId) === id);
    if (!match || !entry.isConnected && !retained) {
      rerender();
      return;
    }
    // 展开、收起和卡内交互共用单卡替换，不改变 matchViewRevision；该 revision
    // 只描述筛选、排序等会改变整张列表形态的状态。
    const template = document.createElement("template");
    template.innerHTML = renderMatch(match, tab.data?.player?.playerRef || "", tab).trim();
    const replacement = template.content.firstElementChild;
    if (!replacement) {
      rerender();
      return;
    }
    // Interaction changes only the detail panel and the expand state. Keep
    // the card summary, its images, tier hydration, focus and replay handler.
    const currentToggle = entry.querySelector("[data-toggle-match]");
    const nextToggle = replacement.querySelector("[data-toggle-match]");
    if (currentToggle && nextToggle) {
      for (const name of ["aria-expanded", "aria-label", "data-tooltip"]) currentToggle.setAttribute(name, nextToggle.getAttribute(name));
    }
    const oldImages = new Map();
    for (const child of [...entry.children]) {
      if (child.matches(".match-summary")) continue;
      for (const image of child.querySelectorAll("img[data-queued-src]")) {
        const key = image.dataset.queuedSrc;
        if (!oldImages.has(key)) oldImages.set(key, []);
        oldImages.get(key).push(image);
      }
      child.remove();
    }
    for (const child of [...replacement.children]) {
      if (child.matches(".match-summary")) continue;
      for (const fresh of child.querySelectorAll("img[data-queued-src]")) {
        const retained = oldImages.get(fresh.dataset.queuedSrc)?.shift();
        if (retained) { retained.className = fresh.className; fresh.replaceWith(retained); }
      }
      entry.appendChild(child);
      bindMatchDetailControls(child, tab);
      bindPlayerLinks(child, tab);
      applyRenderedMetricStyles(child);
    }
    prepareImages(entry);
  }

	function bindMatchDetailControls(container, tab) {
    bindRuneEffectLinks(container);
    // Overview/external list producers render no controls in collapsed cards.
    // Detached build widgets retain the generic binding path below.
    if (tab.openMatches?.size === 0
      && (container.dataset?.matchTierScope || tab.externalRender)) return;
    for (const button of container.querySelectorAll("[data-build-player]")) {
      button.addEventListener("click", () => {
        if (!(tab.buildPlayers instanceof Map)) tab.buildPlayers = new Map();
        tab.buildPlayers.set(String(button.dataset.gameId), Number(button.dataset.buildPlayer));
        recordBuildPlayerSelection(button.dataset.gameId, "select", Boolean(button.querySelector(".build-player-self")));
        rerenderMatch(tab, String(button.dataset.gameId));
        overviewContainer(tab)?.querySelector(`[data-build-player="${button.dataset.buildPlayer}"][data-game-id="${button.dataset.gameId}"]`)?.focus();
      });
      button.addEventListener("keydown", event => {
        if (!["ArrowLeft","ArrowRight"].includes(event.key)) return;
        event.preventDefault();
        const buttons = [...button.closest('[role="tablist"]').querySelectorAll("[data-build-player]")];
        const next = buttons[(buttons.indexOf(button) + (event.key === "ArrowRight" ? 1 : -1) + buttons.length) % buttons.length];
        next.tabIndex = 0; button.tabIndex = -1; next.focus();
      });
    }

    for (const button of container.querySelectorAll("[data-timeline-retry]")) button.addEventListener("click", () => {
      const match = (tab.data?.matches || []).find((item) => String(item.gameId) === button.dataset.timelineRetry);
      if (!match) return;
      const subject = matchSubject(match, tab.data?.player?.playerRef);
      state.matchTimelines.delete(matchTimelineKey(match, subject, tab));
      void ensureMatchTimeline(match, subject, tab);
      rerenderMatch(tab, String(match.gameId));
    });

    for (const button of container.querySelectorAll("[data-match-detail]")) button.addEventListener("click", () => {
      tab.matchDetailTabs.set(button.dataset.gameId, button.dataset.matchDetail);
      if (button.dataset.matchDetail === "build") {
        ensurePerks(); ensureItems();
        const match = (tab.data?.matches || []).find((item) => String(item.gameId) === String(button.dataset.gameId));
        if (match) { ensureMatchTimeline(match, matchSubject(match, tab.data?.player?.playerRef), tab); ensureBuildData(match, tab, true); }
      }
      rerenderMatch(tab, String(button.dataset.gameId));
    });
	  for (const button of container.querySelectorAll("[data-damage-sort]")) button.addEventListener("click", () => {
      const gameID = String(button.dataset.gameId || "");
      if (!gameID) return;
      const team = String(button.dataset.team || "");
      tab.damageSorts.set(`${gameID}:${team}`, button.dataset.damageSort === "damageTaken" ? "damage" : "damageTaken");
		rerenderMatch(tab, gameID);
	  });
	  for (const button of container.querySelectorAll("[data-team-analysis-metric]")) button.addEventListener("click", () => {
		const gameID = String(button.dataset.gameId || "");
		const metric = String(button.dataset.teamAnalysisMetric || "");
		if (!gameID || !TEAM_ANALYSIS_METRICS.some((item) => item.key === metric)) return;
		tab.teamAnalysisMetrics.set(gameID, metric);
		rerenderMatch(tab, gameID);
	  });
	  for (const group of container.querySelectorAll(".team-analysis-metrics")) group.addEventListener("keydown", (event) => {
		if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
		const tabs = [...group.querySelectorAll('[role="tab"]')];
		const current = tabs.indexOf(document.activeElement);
		if (current < 0) return;
		event.preventDefault();
		const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
		const gameID = tabs[next].dataset.gameId;
		const metric = tabs[next].dataset.teamAnalysisMetric;
		const parent = tabs[next].closest(".match-entry")?.parentElement;
		tabs[next].click();
		parent?.querySelector(`[data-game-id="${gameID}"][data-team-analysis-metric="${metric}"]`)?.focus();
	  });
    for (const group of container.querySelectorAll(".match-detail-tabs")) group.addEventListener("keydown", (event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      const tabs = [...group.querySelectorAll('[role="tab"]')];
      const current = tabs.indexOf(document.activeElement);
      if (current < 0) return;
      event.preventDefault();
      const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
      const gameID = tabs[next].dataset.gameId;
      const detail = tabs[next].dataset.matchDetail;
      const parent = tabs[next].closest(".match-entry")?.parentElement;
      tabs[next].click();
      parent?.querySelector(`[data-game-id="${gameID}"][data-match-detail="${detail}"]`)?.focus();
    });
    // 已展开且停在“构建”页签的对局：确保时间线已请求（幂等，命中缓存
    // 或已在途时直接返回），避免请求被取消后一直停留在加载文案。
    for (const gameId of tab.openMatches) {
      if ((tab.matchDetailTabs.get(String(gameId)) || "overview") !== "build") continue;
      const match = (tab.data?.matches || []).find((item) => String(item.gameId) === String(gameId));
      if (!match) continue;
      const subject = matchSubject(match, tab.data?.player?.playerRef);
      ensureBuildData(match, tab);
      // Rebinding a failed card is not a retry. Keep its retry button visible;
      // explicit retry/reopening still calls ensureMatchTimeline above.
      if (!state.matchTimelines.has(matchTimelineKey(match, subject, tab))) ensureMatchTimeline(match, subject, tab);
    }
  }

  async function ensureScoreDetails(match, tab) {
    if(!match || match.scorePartsLoaded || !match.participants?.some(p=>p.score && !Array.isArray(p.score.parts)))return;
    const playerRef=tab.data?.player?.playerRef || tab.playerRef || '',region=tab.region || tab.data?.player?.region || '',serverId=tab.serverId || tab.data?.player?.serverId || '';
    if(!playerRef || (!riotTab(tab) && !serverId))return;
    const flights=tab.scorePartsFlights ||= new Map(),id=String(match.gameId),key=[region,serverId,playerRef,id].join('|');
    if(flights.has(key))return flights.get(key);
    const subject=matchSubject(match,tab.data?.player?.playerRef);
    if(!subject?.participantId)return;
    const flight=(async()=>{
      try {
        const fresh=await api('/api/gameplay/match',{method:'POST',body:JSON.stringify({gameId:Number(match.gameId),participantId:Number(subject.participantId),region,serverId,playerRef,refresh:'score-parts'})},`score-parts:${tab.key}:${id}`,25000);
        if(tab.closed || !(tab.data?.matches || []).includes(match) || String(fresh?.gameId)!==id || Number(fresh?.subjectParticipantId)!==Number(subject.participantId))return;
        if(playerRef!==(tab.data?.player?.playerRef || tab.playerRef || '') || serverId!==(tab.serverId || tab.data?.player?.serverId || '') || region!==(tab.region || tab.data?.player?.region || ''))return;
        const freshSubject=fresh.participants?.find(p=>Number(p.participantId)===Number(subject.participantId));
        if(freshSubject?.playerRef!==playerRef || !freshSubject.score || !Array.isArray(freshSubject.score.parts))return;
        for(const participant of match.participants || []){const score=fresh.participants?.find(p=>p.participantId===participant.participantId)?.score;if(score)participant.score=score;}
        match.scorePartsLoaded=true;
        const scope=overviewContainer(tab),records=readMatchScores(match);
        for(const node of scope?.querySelectorAll(`[data-match-id="${id}"] [data-score-participant]`) || []){
          const record=records.get(Number(node.dataset.scoreParticipant));if(!record)continue;
          const template=document.createElement('template');template.innerHTML=scoreChip(record);
          node.dataset.tooltipScore=template.content.firstElementChild.dataset.tooltipScore;
          window.dispatchEvent(new CustomEvent('deep-legends:score-parts',{detail:{target:node}}));
        }
      } catch(error) {if(error.name!=='RequestCancelled')match.scorePartsLoaded=false;}
      finally {flights.delete(key);}
    })();flights.set(key,flight);return flight;
  }

  function bindScoreDetails(container, tab) {
    for(const candidate of container.querySelectorAll('.match-score,[data-match-tag-pinned]')){
      const node=candidate.matches('[data-match-tag-pinned]') ? candidate.querySelector('[data-tooltip]') || candidate : candidate;
      if(node._scorePartsBound)continue;node._scorePartsBound=true;
      if(candidate.matches('[data-match-tag-pinned]')){
        const entry=node.closest('.match-entry'),match=tab.data?.matches?.find(m=>String(m.gameId)===entry?.dataset.matchId),subject=match && matchSubject(match,tab.data?.player?.playerRef);
        const record=match && readMatchScores(match).get(Number(subject?.participantId));
        if(record){const template=document.createElement('template');template.innerHTML=scoreChip(record);node.dataset.scoreParticipant=String(subject.participantId);node.dataset.tooltipScore=template.content.firstElementChild.dataset.tooltipScore;node.dataset.tooltip=template.content.firstElementChild.dataset.tooltip;node.tabIndex=0;}
      }
      const load=()=>{const entry=node.closest('.match-entry'),match=tab.data?.matches?.find(m=>String(m.gameId)===entry?.dataset.matchId);void ensureScoreDetails(match,tab);};
      node.addEventListener('pointerenter',load);node.addEventListener('focus',load);
    }
  }

  function bindMatchEntryControls(entry, tab, rerender, toggleButtons) {
    if (!entry) return;
    for (const button of toggleButtons || entry.querySelectorAll("[data-toggle-match]")) {
      if (button._matchToggleBound) continue;
      button._matchToggleBound = true;
      button.addEventListener("click", () => {
      const id = String(button.dataset.toggleMatch || "");
      if (tab.openMatches.has(id)) {
        tab.openMatches.delete(id);
        // 收起后清除页签记忆，重新展开时回到“概览”。
        tab.matchDetailTabs.delete(id);
          resetBuildPlayerSelection(tab, id, "reset-collapse");
      } else {
        tab.openMatches.add(id);
        tab.matchDetailModuleError=false;
        if(typeof ensureScoreDetails==='function')void ensureScoreDetails((tab.data?.matches || []).find(m=>String(m.gameId)===id),tab);
        if(riotTab(tab)) {const match=(tab.data?.matches||[]).find(item=>String(item.gameId)===id);if(match)void ensureMatchTimeline(match,matchSubject(match,tab.data?.player?.playerRef),tab);}
      }
      replaceMatchEntry(entry, tab, rerender);
      });
    }
  }

  function bindOverviewContent(container, tab, overview = container) {
    bindScoreDetails(container,tab);
    requestAnimationFrame(()=>{if(container.isConnected)fitMatchTags(container,true);});
    setTimeout(()=>hydrateVisibleMatchTags(container,tab),0);
    const scroll=matchTierScrollRoot(container);
    if(scroll && scroll._matchTagTab!==tab) {if(scroll._matchTagHandler)scroll.removeEventListener("scroll",scroll._matchTagHandler);scroll._matchTagTab=tab;scroll._matchTagHandler=()=>hydrateVisibleMatchTags(overviewContainer(tab)||container,tab);scroll.addEventListener("scroll",scroll._matchTagHandler,{passive:true});}
    bindSummonerCopy(container);
    bindOverviewSubpageLinks(container,tab);
    container.querySelector("[data-complete-overview]")?.addEventListener("click", () => loadOverview(tab, true, false, true, true));
    const rerender = () => rerenderTab(tab,"bindOverviewContent");
    bindPlayerLinks(container, tab);
    bindOverviewShareControls(container, overview, tab);
    container.querySelector("[data-open-career-dialog]")?.addEventListener("click", (event) => openCareerDialog(container, tab, event.currentTarget));
    container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => historyServiceState(tab)?retryHistoryRegion(tab):loadOverview(tab, true));
    bindMatchFilterControls(container, tab);
    container.querySelector("[data-load-more]")?.addEventListener("click", () => loadOverview(tab, false, true, true));
    bindRankHistoryControls(container);
    bindRankedQueueControls(container, tab);
    bindMayhemRatingControls(container,tab);
    bindMatchSentinel(container, tab);
    // One tree walk covers every card; per-card selectors repeat that work on
    // large histories. Keep the event closure scoped to its original entry.
    for (const button of container.querySelectorAll("[data-toggle-match]")) {
      const entry = button.closest(".match-entry");
      if (entry) bindMatchEntryControls(entry, tab, rerender, [button]);
    }
    bindMatchDetailControls(container, tab);
    for (const button of container.querySelectorAll("[data-replay]")) button.addEventListener("click", () => replay(button));
  }

  function bindRankedQueueControls(container, tab) {
    for(const button of container.querySelectorAll('[data-casual-mode]'))button.addEventListener('click',()=>{tab.casualMode=button.dataset.casualMode;updateCareerCards(tab,'casual_mode');if(nodes.careerDialog?.open)renderCareerDialogContent(tab.data,tab,nodes.careerDialogContent.clientWidth<=520?'single':'masonry',true);});
    for (const button of container.querySelectorAll("[data-ranked-queue]")) button.addEventListener("click", () => {
      const queue = button.dataset.rankedQueue;
      const scope = button.closest("[data-ranked-queue-scope]")?.dataset.rankedQueueScope || "recent";
      const stateKey = { recent: "rankedQueueRecent", ability: "rankedQueueAbility", position: "rankedQueuePosition" }[scope] || "rankedQueueRecent";
      // 只接受切换器真的渲染出来的那几个页签标识；海克斯大乱斗用
      // MAYHEM_QUEUE_TAB_KEY（三个队列 ID 合并成一个页签）。
      const allowed = scope === "recent" ? ["420", "440", MAYHEM_QUEUE_TAB_KEY] : ["420", "440"];
      if (!allowed.includes(queue) || tab[stateKey] === queue) return;
      tab[stateKey] = queue;
      updateCareerCards(tab,"ranked_queue");
      if (nodes.careerDialog?.open && tab.data && nodes.careerDialogContent) {
        nodes.careerDialogContent.dataset.careerLayout = "";
    nodes.careerDialogContent.scrollTop = 0;
        const width = nodes.careerDialogContent.clientWidth || nodes.careerDialog.clientWidth || window.innerWidth;
        renderCareerDialogContent(tab.data, tab, width <= 520 ? "single" : "masonry");
      }
    });
  }

  function bindRankHistoryControls(container) {
    for (const button of container.querySelectorAll("[data-rank-history-toggle]")) {if(button._rankHistoryBound)continue;button._rankHistoryBound=true;button.addEventListener("click", () => {
      const expanded = button.getAttribute("aria-expanded") !== "true";
      button.setAttribute("aria-expanded", String(expanded));
      for (const row of button.closest(".rank-history")?.querySelectorAll("[data-rank-history-extra]") || []) row.hidden = !expanded;
      button.querySelector("span").textContent = expanded ? "收起" : "查看更多赛段段位";
    });}
  }

  function sleep(duration) { return new Promise((resolve) => setTimeout(resolve, duration)); }

  // 回放：直接交给客户端播放；尚未下载时先触发下载，
  // 下载完成后自动启动回放，全程不改按钮图标，仅用提示条反馈进度。
  async function replay(button) {
    const gameID = Number(button.dataset.replay);
    if (!gameID) return;
    if (state.settings.confirmReplay && !confirm("由英雄联盟客户端下载或启动这场回放？")) return;
    button.disabled = true;
    button.classList.add("is-busy");
    try {
      const result = await api("/api/gameplay/replay", { method: "POST", body: JSON.stringify({ gameId: gameID, action: "auto" }) }, `replay:${gameID}`, 15000);
      if (result.action === "watch") {
        showToast("正在由英雄联盟客户端启动回放");
      } else {
        showToast("正在下载回放，完成后自动开始播放");
        await watchReplayWhenReady(gameID);
      }
    } catch (error) { showToast(error.message); }
    finally { button.disabled = false; button.classList.remove("is-busy"); }
  }

  async function watchReplayWhenReady(gameID) {
    for (let attempt = 0; attempt < 40; attempt++) {
      await sleep(3000);
      const metadata = await api(`/api/gameplay/replay?gameId=${gameID}`, {}, `replay-meta:${gameID}`, 8000).catch(() => null);
      if (!metadata) continue;
      if (metadata.state === "incompatible") { showToast("该回放与当前客户端版本不兼容"); return; }
      if (metadata.state === "watch") {
        await api("/api/gameplay/replay", { method: "POST", body: JSON.stringify({ gameId: gameID, action: "watch" }) }, `replay:${gameID}`, 15000).catch((error) => showToast(error.message));
        showToast("回放下载完成，正在启动播放");
        return;
      }
    }
    showToast("回放仍在下载，可稍后再点一次回放按钮");
  }

  function updateLiveLoadingVisibility(force, source) {
    clearTimeout(state.liveLoadingVisibleTimer);
    state.liveLoadingVisibleTimer = 0;
    state.liveLoadingVisible = false;
    if (!(force === true || source === "manual")) return;
    state.liveLoadingVisibleTimer = setTimeout(() => {
      state.liveLoadingVisibleTimer = 0;
      if (state.liveLoading) {
        state.liveLoadingVisible = true;
        renderLive("timer");
      }
    }, 240);
  }

  function championDisplayName(id) {
    const catalog = window.deepLegendsChampionSearch?.getCatalog?.() || state.historyFilterCatalog;
    return catalog?.champions?.find(p => Number(p.id || p.championId) === Number(id))?.nameZh || "";
  }

  function noteChampSelectLatency(stage, started) {
    if (!started) return;
    void fetch("/api/gameplay/champselect-latency", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ stage, durationMs: Math.max(0, Math.round(performance.now() - started)) }) }).catch(() => {});
  }

  function noteChampSelectReady(stage, currentTargetMatches) {
    const version = Number(state.champSelectLite?.version || 0);
    if (!currentTargetMatches || state.live?.phase !== "ChampSelect" || !version || !state.champSelectRecommendationStarted) return;
    const recorded = state.champSelectReadyVersions ||= new Map();
    if (recorded.get(stage) === version) return;
    recorded.set(stage, version);
    noteChampSelectLatency(stage, state.champSelectRecommendationStarted);
  }

  function mergeChampSelectFields(data, afterVersion = -1) {
    const lite = state.champSelectLite;
    if (!lite || Number(lite.version) <= afterVersion || data?.phase !== "ChampSelect" || Number(data.gameId || 0) !== Number(lite.gameId || 0)) return data;
    const next = { ...data, players: (data.players || []).map(p => ({ ...p })) };
    const cells = new Map(lite.cells.map(cell => [Number(cell.cellId), cell]));
    let changedSelf = false;
    for (const p of next.players) {
      if (p.cellId == null) continue;
      const cell = cells.get(Number(p.cellId));
      if (!cell) continue;
      const id = Number(cell.championId) || Number(cell.pickIntent) || 0;
      const changed = Number(p.championId || p.championPickIntent || 0) !== id;
      Object.assign(p, { championId: Number(cell.championId) || 0, championPickIntent: Math.max(0, Number(cell.pickIntent) || 0), championLocked: cell.locked === true, championPickPending: Number(cell.pickIntent) < 0, position: livePositionDisplay(cell.assignedPosition), spell1Id: cell.spell1Id, spell2Id: cell.spell2Id });
      if (changed) p.championName = cell.championName || championDisplayName(id);
      if (p.isCurrent) { changedSelf = changed; next.currentChampionId = p.championId; next.resolvedChampionId = id; }
    }
    if (changedSelf) { delete next.recommendations; delete next.clientRecommendation; delete next.championAbilities; }
    const target = liveRecommendationTarget(next);
    const client = state.clientRuneCache?.get(target?.key);
    if (client) next.clientRecommendation = client;
    return next;
  }

  async function ensureFastClientRecommendation(data) {
    if (data?.phase !== "ChampSelect") return;
    const target = liveRecommendationTarget(data);
    if (!target) return;
    const cache = state.clientRuneCache ||= new Map(), flights = state.clientRuneFlights ||= new Map(), failures = state.clientRuneFailures ||= new Map();
    if (cache.has(target.key)) { if (liveRecommendationTarget(state.live)?.key === target.key) state.live.clientRecommendation = cache.get(target.key); noteChampSelectReady("client_ready", liveRecommendationTarget(state.live)?.key === target.key); return; }
    if (flights.has(target.key) || failures.has(target.key) && Date.now() - failures.get(target.key) < 60000) return;
    const generation = state.liveGameGeneration;
    flights.set(target.key, generation);
    try {
      const query = new URLSearchParams({ championId: target.championId, position: target.clientPosition, mapId: target.mapId });
      const result = await api(`/api/gameplay/client-recommendation?${query}`, {}, `client-recommendation:${target.key}`, 5000);
      if (generation !== state.liveGameGeneration || Number(result.championId) !== target.championId) return;
      if (!result.clientRecommendation) throw Error("客户端推荐尚未就绪");
      cache.set(target.key, result.clientRecommendation);
      failures.delete(target.key);
      if (liveRecommendationTarget(state.live)?.key === target.key) {
        state.live.clientRecommendation = result.clientRecommendation;
        ensurePerks(); renderLive("recommendation"); noteChampSelectReady("client_ready", true);
      }
    } catch (error) {if(generation === state.liveGameGeneration && error.name !== "RequestCancelled") failures.set(target.key, Date.now());}
    finally {if(flights.get(target.key) === generation) flights.delete(target.key);}
  }

  function applyChampSelectLite(detail) {
    if (state.destroyed || !connected() || !Array.isArray(detail?.cells) || Number(detail.version) <= Number(state.champSelectLite?.version || 0)) return false;
    if (state.beacon.phase !== "ChampSelect" || state.live && state.live.phase !== "ChampSelect") return false;
    const expected = normalizeLiveGameId(state.liveExpectedGameId || state.live?.gameId);
    if (expected && Number(detail.gameId) !== expected) return false;
    const started = performance.now() - Math.max(0, Date.now() - Number(detail.eventAtMs || Date.now())), previous = state.live;
    state.champSelectLite = detail;
    state.champSelectRecommendationStarted = started;
    if (!previous) return true;
    state.live = mergeChampSelectFields(previous);
    if (state.liveProgressSnapshot) state.liveProgressSnapshot.live = state.live;
    resetRecommendationTabsOnChampionChange(previous, state.live);
    renderLive("sse");
    noteChampSelectLatency("card_render", started);
    noteChampSelectLatency("recommendation_loading", started);
    void ensureFastClientRecommendation(state.live);
    void ensureLiveRecommendations(state.live);
    void ensureProRunes(state.live);
    void ensureSpecialistRunes(state.live);
    void ensureLaneMatchupCandidates(state.live);
    return true;
  }
  window.addEventListener("deep-legends:champselect-lite", event => applyChampSelectLite(event.detail));

  async function loadLive(force = false, source = "direct") {
    recordLiveRefresh("load", source);
    if (state.destroyed) return;
    if (!connected()) { renderLive(state.liveLoadSource); return; }
    invalidateLiveForNewGame(state.live?.phase, state.beacon.phase, state.liveExpectedGameId, source);
    if (state.liveLoading && !force) return;
    state.liveLoadSource = liveRenderTriggerLabel(source);
    if (state.liveLoading) state.controllers.get("live")?.abort();
    state.liveRefreshQueued = false;
    state.livePhaseRefreshQueued = false;
    state.liveGameRefreshQueued = false;
    if (force) {state.liveRetryAttempts = 0;state.clientRuneFailures?.clear();}
    state.liveLoadStartedAt = Date.now();
    state.liveLiteRequestVersion = Number(state.champSelectLite?.version || 0);
    const requestToken = state.liveRequestToken = Number(state.liveRequestToken || 0) + 1;
    state.liveProgressRequestId = `${Date.now()}-${requestToken}-${Math.random().toString(36).slice(2)}`;
    state.liveProgressSnapshot = null;
    state.liveLoading = true;
    updateLiveLoadingVisibility(force, source);
    state.liveError = "";
    renderLive(state.liveLoadSource);
    try {
      let previousLive = state.live;
      let nextLive = await api(`/api/gameplay/live?requestId=${encodeURIComponent(state.liveProgressRequestId)}${force ? "&refresh=1" : ""}`, {}, "live", 45000);
      if (state.liveRequestToken !== requestToken || !connected()) return;
      if (state.liveAwaitingGame && normalizeLiveGameId(state.liveExpectedGameId)
          && normalizeLiveGameId(nextLive.gameId) !== normalizeLiveGameId(state.liveExpectedGameId)) {
        recordLiveObservation("stale-response", nextLive.phase, nextLive.gameId, source);
        return;
      }
      if (liveSnapshotBehindPhase(nextLive) || state.livePhaseRefreshQueued && nextLive.phase !== state.beacon.phase) return;
      if (state.liveAwaitingGame && liveGamePhase(state.beacon.phase) && !liveGamePhase(nextLive.phase)) return;
      if (invalidateLiveForNewGame(previousLive?.phase, nextLive.phase, nextLive.gameId, source, requestToken)) previousLive = null;
      if (nextLive.phase && !liveGamePhase(nextLive.phase) && !["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(nextLive.phase)) {
        nextLive = { phase: nextLive.phase, available: false, gameId: 0, players: [], capabilities: nextLive.capabilities };
      }
      if (shouldResetLiveGameScopedState(previousLive, nextLive)) resetLiveGameScopedState(false, { reason: (Number(previousLive?.gameId) || Number(state.liveRecommendationScopeGameId)) > 0 && Number(nextLive.gameId) > 0 && (Number(previousLive?.gameId) || Number(state.liveRecommendationScopeGameId)) !== Number(nextLive.gameId) ? "game_changed" : "enter_champselect", previous: previousLive, next: nextLive, source });
      if (previousLive?.phase === "ChampSelect" && nextLive.phase !== "ChampSelect") { for (const key of state.laneMatchupPairs?.keys?.() || []) state.controllers?.get?.(`lane-pair:${key}`)?.abort?.(); state.laneMatchupPairs?.clear?.(); state.laneMatchupCardDiagnostics?.clear?.(); state.laneMatchupCandidates?.clear?.(); state.laneMatchupDiagnosticSkips?.clear?.(); }
      resetLivePositionOverrides(previousLive, nextLive);
      resetRecommendationTabsOnChampionChange(previousLive, nextLive);
      if (previousLive && previousLive.queueId === nextLive.queueId) {
        const histories = new Map((previousLive.players || []).filter(p => p.playerRef).map(p => [p.playerRef, p]));
        for (const player of nextLive.players || []) {
          const old = histories.get(player.playerRef);
          if (player.historyState === "pending" && old && old.historyState !== "pending") {
            for (const field of ["recentGames", "modeStats", "recentRankedRecord", "recentPositions"]) player[field] = old[field];
          }
        }
      }
      nextLive = mergeChampSelectFields(nextLive, state.liveLiteRequestVersion);
      retainArenaBravery(previousLive, nextLive);
      state.live = nextLive;
      state.liveProgressSnapshot = null;
      state.liveExpectedGameId = normalizeLiveGameId(nextLive.gameId);
      state.liveAwaitingGame = state.liveAwaitingGame && liveGamePhase(nextLive.phase) && !nextLive.unsupported
        && !(nextLive.available && (nextLive.players?.length || Number(nextLive.currentChampionId) > 0 || Number(nextLive.resolvedChampionId) > 0));
      updateBeacon(String(state.live?.phase || ""));
      state.lastCapabilities = state.live.capabilities || state.lastCapabilities;
      renderCapabilitySettings();
      const recommendations = liveRecommendationsFor(state.live);
      if (state.live.clientRecommendation || recommendations?.runes) ensurePerks();
      if (recommendations?.build) { ensureItems(); ensureSummonerSpells(); }
      void ensureFastClientRecommendation(state.live);
      ensureLiveRecommendations(state.live);
      ensureSpecialistRunes(state.live);
      void ensureProRunes(state.live);
      void ensureLaneMatchupCandidates(state.live);
    } catch (error) {
      if (state.liveRequestToken === requestToken && (error.name !== "RequestCancelled" || !state.live || state.liveAwaitingGame)) {
        state.liveError = error.name === "RequestCancelled" ? "读取已中断，请重试。" : (error.message || "未能读取对局，请重试。");
      }
    } finally {
      if (state.liveRequestToken !== requestToken) return;
      state.liveLoading = false;
      clearTimeout(state.liveLoadingVisibleTimer);
      state.liveLoadingVisibleTimer = 0;
      state.liveLoadingVisible = false;
      syncLiveRetryBudget();
      renderLive(state.liveLoadSource);
      scheduleLiveRefresh();
      if (state.liveRefreshQueued) queueLiveEventRefresh("event", Boolean(state.livePhaseRefreshQueued));
    }
  }

  function applyLivePlayerProgress(detail) {
    if (state.destroyed) return false;
    const next = mergeChampSelectFields(detail?.live, state.liveLiteRequestVersion);
    const stale = () => { recordLiveProgressApply("ignored_stale"); return false; };
    if (!state.liveLoading || detail?.requestId !== state.liveProgressRequestId || !connected() || !next?.available || !liveGamePhase(next.phase)) return stale();
    if (liveSnapshotBehindPhase(next) || state.livePhaseRefreshQueued && next.phase !== state.beacon.phase) return stale();
    const expected = normalizeLiveGameId(state.liveExpectedGameId);
    if (expected && normalizeLiveGameId(next.gameId) !== expected) return stale();
    if (!Array.isArray(next.players) || !next.players.length) return stale();
    const nextGameId = normalizeLiveGameId(next.gameId);
    // Initial-load snapshots may continue filling players within their request.
    const initialProgress = state.liveProgressSnapshot?.requestId === detail.requestId && state.liveProgressSnapshot?.live === state.live;
    if (!initialProgress && !state.liveAwaitingGame && state.live?.available && state.live.players?.length
        && nextGameId && normalizeLiveGameId(state.live.gameId) === nextGameId && state.live.queueId === next.queueId) {
      recordLiveProgressApply("ignored_same_game", detail.requestId);
      return false;
    }
    retainArenaBravery(state.live, next);
    state.live = next;
    state.liveProgressSnapshot = { requestId: detail.requestId, live: next };
    state.liveAwaitingGame = false;
    state.liveError = "";
    recordLiveProgressApply("applied");
    state.liveRenderSource = "progress";
    state.liveRenderTrigger = "progress";
    renderLive();
    void ensureFastClientRecommendation(state.live);
    void ensureLiveRecommendations(state.live);
    void ensureProRunes(state.live);
    void ensureSpecialistRunes(state.live);
    return true;
  }
  window.addEventListener("deep-legends:live-player-progress", event => applyLivePlayerProgress(event.detail));

  function liveGamePhase(phase) {
    return ["ChampSelect", "GameStart", "InProgress", "Reconnect"].includes(phase);
  }

  function normalizeLiveGameId(value) {
    const id = Number(value);
    return Number.isSafeInteger(id) && id > 0 ? id : 0;
  }

  function liveGameIdComparison(gameId) {
    const current = normalizeLiveGameId(gameId);
    const cached = normalizeLiveGameId(state.live?.gameId);
    return !current ? "unavailable" : !cached ? "first" : current === cached ? "same" : "different";
  }

  function recordLiveObservation(reason, phase, gameId, source, invalidated = false) {
    window.reportFlowDiagnostic?.("gameflow_phase_client", reason, {
      phase: String(phase || ""), previousPhase: String(state.beacon.phase || ""), source,
      gameId: normalizeLiveGameId(gameId), cachedGameId: normalizeLiveGameId(state.live?.gameId),
      gameIdComparison: liveGameIdComparison(gameId), phaseChanged: Boolean(phase) && phase !== state.beacon.phase,
      invalidated, receivedAt: Date.now(),
    });
  }

  function liveSnapshotBehindPhase(data) {
    if (state.liveScopeLastReset?.reason === "left_end_of_game" && !liveGamePhase(state.beacon.phase)
        && !["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(state.beacon.phase)
        && ["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(data?.phase)) return true;
    const cachedId = normalizeLiveGameId(state.live?.gameId);
    const incomingId = normalizeLiveGameId(data?.gameId);
    if (cachedId && incomingId && cachedId !== incomingId) return false;
    return ["InProgress", "Reconnect"].includes(state.beacon.phase)
      && ["ChampSelect", "GameStart"].includes(data?.phase);
  }

  function invalidateLiveForNewGame(previousPhase, nextPhase, gameId = 0, source = "direct", owningRequest = 0) {
    const previousId = normalizeLiveGameId(state.live?.gameId) || normalizeLiveGameId(state.liveExpectedGameId);
    const nextId = normalizeLiveGameId(gameId);
    const gameChanged = previousId > 0 && nextId > 0 && previousId !== nextId;
    const phaseBoundary = Boolean(previousPhase) && previousPhase !== nextPhase && liveGamePhase(nextPhase)
      && (!liveGamePhase(previousPhase) || nextPhase === "ChampSelect");
    const leftEndOfGame = ["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(previousPhase)
      && Boolean(nextPhase) && !liveGamePhase(nextPhase) && !["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(nextPhase);
    const invalidated = gameChanged || phaseBoundary || leftEndOfGame;
    recordLiveObservation("invalidate", nextPhase, gameId, source, invalidated);
    if (!invalidated) return false;
    // A completed current response may clear the old snapshot before publishing
    // its new data, without aborting itself or losing ownership of its finally.
    if (!owningRequest || owningRequest !== state.liveRequestToken) {
      state.liveRequestToken = Number(state.liveRequestToken || 0) + 1;
      state.controllers.get("live")?.abort();
      state.liveLoading = false;
      clearTimeout(state.liveLoadingVisibleTimer);
      state.liveLoadingVisibleTimer = 0;
      state.liveLoadingVisible = false;
      state.liveGameRefreshQueued = gameChanged;
    }
    clearTimeout(state.liveTimer);
    clearTimeout(state.liveEventTimer);
    state.liveTimer = 0;
    state.liveEventTimer = 0;
    const previousLive = state.live;
    state.live = null;
    state.liveExpectedGameId = leftEndOfGame ? 0 : nextId;
    state.liveAwaitingGame = !leftEndOfGame;
    state.liveRetryAttempts = 0;
    state.liveRetryKey = "";
    state.liveError = "";
    resetLiveGameScopedState(false, { reason: leftEndOfGame ? "left_end_of_game" : gameChanged ? "game_changed" : nextPhase === "ChampSelect" ? "enter_champselect" : "await_game", previous: previousLive, next: { gameId: leftEndOfGame ? 0 : nextId, phase: nextPhase }, source });
    renderLive("resync");
    return true;
  }

  function resetDisconnectedLive() {
    const previousLive = state.live;
    state.liveRequestToken = Number(state.liveRequestToken || 0) + 1;
    state.controllers.get("live")?.abort();
    state.liveLoading = false;
    clearTimeout(state.liveLoadingVisibleTimer);
    state.liveLoadingVisibleTimer = 0;
    state.liveLoadingVisible = false;
    state.live = null;
    state.liveAwaitingGame = false;
    state.liveExpectedGameId = 0;
    state.liveGameRefreshQueued = false;
    state.liveError = "实时连接已中断，请重试。";
    resetLiveGameScopedState(false, { reason: "resync", previous: previousLive, next: null, source: "realtime-disconnect" });
    renderLive("resync");
    scheduleLiveRefresh();
  }

  function retainArenaBravery(previous, next) {
    if (!previous || !next || Number(previous.queueId) !== Number(next.queueId) || ![1700,1710,1750].includes(Number(next.queueId)) && next.gameMode !== "CHERRY") return;
    if (Number(previous.gameId)>0 && Number(next.gameId)>0 && String(previous.gameId)!==String(next.gameId) || next.phase === "ChampSelect" && previous.phase !== "ChampSelect") return;
    if (!["ChampSelect","GameStart","InProgress","Reconnect"].includes(next.phase)) return;
    for (const player of next.players || []) {
      const old=(previous.players || []).find(p=>player.playerRef && p.playerRef===player.playerRef);
      if (old?.bravery && (player.isCurrent || player.isAlly || player.mySquad) && Number(player.championId)<=0) player.bravery=true;
    }
  }

  function liveBraveryPortrait(player, arenaMode, phase) {
    return Boolean(arenaMode && (player?.isCurrent || player?.isAlly || player?.mySquad) && ["ChampSelect","GameStart","InProgress","Reconnect"].includes(phase)
      && (phase === "ChampSelect" || !(Number(player?.championId)>0)) && (player?.bravery || Number(player?.championId) === -3 || Number(player?.championPickIntent) === -3));
  }

  function liveDisplayedChampionId(player, currentChampionId = 0) {
    if (player?.championPickPending === true) return -3;
    player = { ...player, championId: Number(player?.championId) > 0 ? Number(player.championId) : 0, championPickIntent: Number(player?.championPickIntent) > 0 ? Number(player.championPickIntent) : 0 };
    return Number(player?.championId) || Number(player?.championPickIntent) || (player?.isCurrent === true ? Number(currentChampionId) || 0 : 0);
  }

  function liveRecommendationChampionId(player) {
    const lockedChampionId = Number(player?.championId) || 0;
    if (lockedChampionId > 0) return lockedChampionId;
    if (USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS && player?.championLocked !== true) {
      const intent = Number(player?.championPickIntent) || 0;
      return intent > 0 ? intent : 0;
    }
    return 0;
  }

  function resetLivePositionOverrides(previous, next) {
    if (!previous || !next) return;
    const gameChanged = String(previous.gameId || "") !== String(next.gameId || "");
    const leftChampionSelect = String(previous.phase || "") === "ChampSelect" && String(next.phase || "") !== "ChampSelect";
    if (gameChanged || leftChampionSelect) state.livePositionOverride.clear();
  }

  function shouldResetLiveGameScopedState(previous, next) {
    if (!previous || !next) return false;
    const previousId = Number(previous.gameId) || Number(state.liveRecommendationScopeGameId) || 0;
    const nextId = Number(next.gameId) || 0;
    const gameChanged = previousId > 0 && nextId > 0 && previousId !== nextId;
    const enteringChampionSelect = String(previous.phase || "") !== "ChampSelect" && String(next.phase || "") === "ChampSelect";
    return gameChanged || enteringChampionSelect;
  }

  function resetLiveGameScopedState(preserveTabs = false, context = null) {
    context ||= {};
    const previous = context.previous || state.live;
    const next = context.next;
    const reason = context.reason || (preserveTabs ? "hard_refresh" : "resync");
    state.liveScopeLastReset = { reason, at: Date.now() };
    if (typeof fetch === "function") void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ event: "live_scope_reset", reason, source: String(context.source || ""),
        previousGameId: Number(previous?.gameId) || 0, gameId: Number(next?.gameId) || 0,
        previousPhase: String(previous?.phase || ""), phase: String(next?.phase || ""),
        clearedRecommendations: state.liveRecommendations?.size || 0 }),
    }).catch(() => {});
    state.champSelectLite = null;
    state.champSelectReadyVersions?.clear();
    state.clientRuneCache?.clear();
    state.clientRuneFlights?.clear();
    state.clientRuneFailures?.clear();
    state.liveRecommendationScopeGameId = 0;
    state.liveRecommendationRenderPhase = "";
    const recommendationTab = state.recommendationTab;
    const recommendationTabTouched = state.recommendationTabTouched;
    const runeSourceTab = state.runeSourceTab;
    state.liveGameGeneration = Number(state.liveGameGeneration || 0) + 1;
    for (const key of state.liveRecommendationFlights?.keys?.() || []) state.controllers?.get?.(`live-recommendations:${key}`)?.abort?.();
    for (const key of state.specialistRuneFlights?.keys?.() || []) state.controllers?.get?.(`live-specialist-runes:${key}`)?.abort?.();
    clearRuneStarterRetries();
    state.laneMatchupUnavailableCount = 0;
    for (const key of state.laneMatchupPairs?.keys?.() || []) state.controllers?.get?.(`lane-pair:${key}`)?.abort?.();
    for (const [key, entry] of state.laneMatchupLanes || []) if (entry.status === "pending") { state.controllers?.get?.(entry.controllerKey)?.abort?.(); state.laneMatchupLanes.delete(key); }
    for (const key of state.runeStarterRequests?.keys?.() || []) state.controllers?.get?.(`live-rune-starters:${key}`)?.abort?.();
    state.runeStarterRequests?.clear?.();
    clearTimeout(state.proRuneTimer);
    for (const key of state.proRuneFlights?.keys?.() || []) state.controllers?.get?.(`live-pro-runes:${key}`)?.abort?.();
    for (const collection of [state.liveRecommendationScopes, state.liveRecommendationSpells, state.proRunes, state.proRuneFlights, state.liveRecommendations, state.liveRecommendationTraces, state.specialistRunes, state.liveRecommendationFailures, state.liveRecommendationFlights, state.specialistRuneFailures, state.specialistRuneFlights, state.livePositionOverride, state.liveRecommendationSkipDiagnostics, state.specialistRuneSkipDiagnostics, state.liveRosterRenderDiagnostics, state.specialistPlayerTabs, state.liveRecommendationRosters, state.laneMatchupCandidates, state.laneMatchupPairs, state.laneMatchupCardDiagnostics, state.laneMatchupDiagnosticSkips]) collection?.clear?.();
    state.recommendationTab = "runes";
    state.recommendationTabTouched = false;
    state.runeSourceTab = "opgg";
    state.selectedRecommendation = "opgg";
 state.autoConfigViewKey = ""; state.autoConfig = null;
    if (preserveTabs && recommendationTabTouched) {
      state.recommendationTab = recommendationTab;
      state.recommendationTabTouched = true;
      state.runeSourceTab = runeSourceTab;
    }
  }

  function softResetGameplayState() {
    for (const tab of [...(state.tabs || []), ...(state.overlay || [])]) {
      clearTimeout(tab.quotaRetry?.timer); tab.quotaRetry = null;
      tab.overviewRequestToken = Number(tab.overviewRequestToken || 0) + 1;
      if(tab.mayhemRating?.status==="loading") {tab.mayhemRatingRequestToken=Number(tab.mayhemRatingRequestToken||0)+1;tab.mayhemRating={status:"idle",playerRef:tab.mayhemRating.playerRef};}
      if(tab.overviewSubpageState?.status==="loading") {tab.overviewSubpageRequestToken=Number(tab.overviewSubpageRequestToken||0)+1;tab.overviewSubpageState={status:"idle",page:tab.overviewSubpage};}
    }
    state.liveRequestToken = Number(state.liveRequestToken || 0) + 1;
    for (const controller of state.controllers.values()) controller.abort();
    state.controllers.clear();
    for (const tab of state.tabs) {
      tab.loading = false;
      tab.loadingMore = false;
      tab.paginationStalls = 0;
      tab.paginationBackoffMs = 0;
      tab.nextAutoAppendAt = 0;
      tab.error = "";
      if (tab.data?.pagination) {
        tab.data.pagination = { ...tab.data.pagination, autoPaused: false, pauseReason: "", moreError: "" };
      }
    }
    state.liveLoading = false;
    clearTimeout(state.liveLoadingVisibleTimer);
    state.liveLoadingVisibleTimer = 0;
    state.liveLoadingVisible = false;
    state.liveError = "";
    resetLiveGameScopedState(true, { reason: "hard_refresh", previous: state.live, next: state.live, source: "manual" });
  }

  async function handleHardRefresh() {
    softResetGameplayState();
    const group = overviewGroupForSection();
    const tab = activeTab(group);
    const tasks = [];
    if (state.section === "overview" && tab && tabReady(tab)) tasks.push(loadOverview(tab, true));
    else if (state.section === "overview") renderOverview(group);
    if (connected()) tasks.push(loadLive(true));
    else renderLive("manual");
    scheduleBeaconPoll(0);
    const started=performance.now(),deadline=started+8000;
    let settled=false;void Promise.allSettled(tasks).then(()=>settled=true);
    const pending=()=>[...state.controllers.keys()].some(key=>key==='live'||key.startsWith('overview:'));
    while(!state.destroyed&&connected()&&(!settled||pending())&&performance.now()<deadline)await new Promise(resolve=>setTimeout(resolve,20));
    if(!state.destroyed&&connected()&&(!settled||pending())){
      window.reportFlowDiagnostic?.("manual_refresh_client","timeout",{stage:"hard-refresh-wait",busy_ms:Math.round(performance.now()-started)});
      for(const [key,controller] of state.controllers)if(key==='live'||key.startsWith('overview:'))controller.abort();
    }
  }

  function setOverviewRefreshLoading(group, loading) {
	const active = Boolean(loading);
	const refresh = overviewWorkspace(group).refresh;
	if (!refresh) return;
	refresh.disabled = active;
	refresh.classList.toggle("is-loading", active);
	if (active) refresh.setAttribute("aria-busy", "true");
	else refresh.removeAttribute("aria-busy");
  }

  function resetRecommendationTabsOnChampionChange(previous, next) {
    const previousTarget = liveRecommendationTarget(previous);
    const nextTarget = liveRecommendationTarget(next);
    if (!previousTarget || !nextTarget || previousTarget.championId <= 0 || nextTarget.championId <= 0) return;
    if (previousTarget.championId === nextTarget.championId) return;
    state.recommendationTab = "runes";
    state.recommendationTabTouched = false;
    state.runeSourceTab = "opgg";
  }

  // R116-D P1-2/P1-3/P1-5：把客户端已经给出的阵容英雄 ID 交给后端做交集。
  const R116D_MATCHUP_PHASES = ["inprogress", "reconnect"];
  const R116D_ROSTER_SIDE_LIMIT = 5;

  function liveRecommendationRoster(data) {
    const empty = { phase: "", allyIds: [], enemyIds: [], matchupAllowed: false, signature: "" };
    if (liveAugmentRecommendationSource(data) !== "hextech") return empty;
    const players = Array.isArray(data?.players) ? data.players : [];
    if (!players.length) return empty;
    const phase = String(data?.phase || "").trim();
    const self = players.find((player) => player.isCurrent);
    const selfTeam = Number(self?.teamId) > 0 ? Number(self.teamId) : 100;
    // 未锁定时用 pick intent：ChampSelect 阶段大部分时间 championId 还是 0，
    // 只认锁定值会让协同提示在选人阶段几乎永远不出现。
    const idsOf = (rows) => [...new Set(rows.map((player) => liveRecommendationChampionId(player)).filter((id) => Number(id) > 0))].sort((left, right) => left - right);
    const allyIds = idsOf(players.filter((player) => Number(player.teamId) === selfTeam));
    const matchupAllowed = R116D_MATCHUP_PHASES.includes(phase.toLowerCase());
    const enemyIds = matchupAllowed
      ? idsOf(players.filter((player) => Number(player.teamId) > 0 && Number(player.teamId) !== selfTeam))
      : [];
    const capped = (ids) => (ids.length > R116D_ROSTER_SIDE_LIMIT ? [] : ids);
    const allies = capped(allyIds);
    const enemies = capped(enemyIds);
    return {
      phase, allyIds: allies, enemyIds: enemies, matchupAllowed,
      signature: `${phase}|${allies.join(",")}|${enemies.join(",")}`,
    };
  }

  function liveRecommendationTarget(data) {
    const self = (data?.players || []).find((player) => player.isCurrent);
    const championId = liveRecommendationChampionId(self) > 0
      ? liveRecommendationChampionId(self)
      : Number(data?.currentChampionId) > 0
        ? Number(data.currentChampionId)
        : Number(data?.resolvedChampionId) > 0 ? Number(data.resolvedChampionId) : 0;
    if (championId <= 0) return null;
    const clientPosition = self?.position || "other";
    const augmentSource = liveAugmentRecommendationSource(data);
    const queueId = Number(data?.queueId) || 0;
    const gameMode = String(data?.gameMode || "").trim().toUpperCase();
    const mapId = Number(data?.mapId) || 0;
    const tier = liveRecommendationTier();
    // queueId can be 0 during early ChampSelect and is filled by the next LCU
    // snapshot; keep the cache stable while mode/map provide the real identity.
    const baseKey = `${championId}:${gameMode}:${mapId}`;
    const positionOverride = state.livePositionOverride.get(baseKey) || "";
    const position = positionOverride || clientPosition;
    const gameId = Number(data?.gameId) || 0;
    const scopeGameId = gameId > 0 ? gameId : Number(state.liveRecommendationScopeGameId || 0);
    if (gameId > 0) {
      // An initial unknown ID can become known without a game boundary. The
      // generation and reset policy already fence these successful responses.
      if (!state.liveRecommendationScopeGameId) {
        for (const scope of state.liveRecommendationScopes?.values?.() || []) if (!scope.gameId) scope.gameId = gameId;
      }
      state.liveRecommendationScopeGameId = gameId;
    }
    let spellKey = [Number(self?.spell1Id) || 0, Number(self?.spell2Id) || 0].filter((value) => value > 0).sort((left, right) => left - right).join("-") || "none";
    // R207: special modes retain the last champion-select spell pair for this
    // game. Live-client names and gameflow omissions must not replace that pair.
    if (augmentSource || gameMode === "ARAM" || mapId === 12) {
      const spells = state.liveRecommendationSpells ||= new Map();
      const scopeKey = baseKey;
      if (data?.phase === "ChampSelect" && Number(self?.spell1Id) > 0 && Number(self?.spell2Id) > 0) spells.set(scopeKey, spellKey);
      if (data?.phase !== "ChampSelect") spellKey = spells.get(scopeKey) || "none";
    }
	return { self, championId, position, clientPosition, positionOverride, augmentSource, queueId, gameMode, mapId, tier, gameId, scopeGameId, baseKey, spellKey, key: `${championId}:${position}:${gameMode}:${mapId}:${tier}:${spellKey}` };
  }

  function livePositionValue(value) {
    return ({ middle: "mid", bottom: "adc", utility: "support" })[String(value || "").toLowerCase()] || String(value || "").toLowerCase();
  }
  function livePositionDisplay(value) {
    return ({ mid: "middle", adc: "bottom", support: "utility" })[livePositionValue(value)] || livePositionValue(value);
  }

  function selectLivePosition(position) {
    const data = state.live;
    const target = liveRecommendationTarget(data);
    if (!target || !["top", "jungle", "mid", "adc", "support"].includes(position)) return;
    state.livePositionOverride.set(target.baseKey, position);
    const selectedTarget = liveRecommendationTarget(data);
    if (!selectedTarget) return;
    state.liveRecommendationFailures.delete(selectedTarget.key);
    ensureLiveRecommendations(data);
    void ensureProRunes(data);
    renderLive("selection");
  }

  function liveRecommendationsFor(data) {
    if (data?.recommendations) return data.recommendations;
    const target = liveRecommendationTarget(data);
    if (!target) return null;
    const exact = state.liveRecommendations.get(target.key);
    if (exact) return exact;
    // Metadata belongs to a successful response, not the render-time snapshot.
    // Never borrow from another game, champion, mode, map, or tier.
    let latest = null;
    let latestAt = -1;
    for (const [key, scope] of state.liveRecommendationScopes || []) {
      if (!(target.scopeGameId > 0) || scope.gameId !== target.scopeGameId
          || scope.championId !== target.championId || scope.gameMode !== target.gameMode
          || scope.mapId !== target.mapId || scope.tier !== target.tier) continue;
      const payload = state.liveRecommendations.get(key);
      if (payload && scope.at >= latestAt) { latest = payload; latestAt = scope.at; }
    }
    return latest;
  }

  function specialistRunesFor(data) {
    const target = specialistRequestTarget(data);
    if (target && state.specialistRunes.has(target.key)) return state.specialistRunes.get(target.key);
    const embedded = liveRecommendationsFor(data)?.runes?.specialists;
    return Array.isArray(embedded) ? embedded : [];
  }

  function specialistPosition(data, target = liveRecommendationTarget(data)) {
    if (!target) return "";
    const resolved = liveRecommendationsFor(data)?.resolvedPosition;
    return target.positionOverride || resolved || target.clientPosition || target.position;
  }

  function specialistRequestTarget(data) {
    const target = liveRecommendationTarget(data);
    if (!target) return null;
    const position = specialistPosition(data, target);
    return { ...target, position, key: `${target.championId}:${position}` };
  }

  function liveAugmentRecommendationSource(data) {
    if (isHextechClassic(data)) return "";
    if (isOrdinaryHextechMatch(data) || isHextechQualifier(data)) return "hextech";
    const catalogSource = queueDefinitionFor(data)?.augmentSource;
    if (catalogSource === "arena" || catalogSource === "hextech") return catalogSource;
    const gameMode = String(data?.gameMode || "").trim().toUpperCase();
    const mapID = Number(data?.mapId);
    if (gameMode === "KIWI" && mapID === 12) return "hextech";
    if (gameMode === "CHERRY" && mapID === 30) return "arena";
    return "";
  }

  function newLiveRecommendationTraceId() {
    const random = Math.random().toString(36).slice(2, 10);
    return `rec-${Date.now().toString(36)}-${random}`;
  }

  function recordItemSetClientDiagnostic(event, reason, context = {}) {
    const body = {
      event, reason, key: String(context.recommendationKey || context.key || ""),
      traceId: String(context.traceId || ""), championId: Number(context.championId || 0),
      queueId: Number(context.queueId || 0), mapId: Number(context.mapId || 0), gameId: Number(context.gameId || 0),
      position: String(context.position || ""), requestedPosition: String(context.requestedPosition || ""),
      resolvedPosition: String(context.resolvedPosition || ""), positionSource: String(context.positionSource || ""),
      gameMode: String(context.gameMode || ""), tier: String(context.tier || ""),
      blockCount: Number(context.blockCount || 0), itemCount: Number(context.itemCount || 0),
      endpoint: String(context.endpoint || ""), itemId: Number(context.itemId || 0),
      httpStatus: Number(context.httpStatus || 0), errorKind: String(context.errorKind || ""),
	  rowCount: Number(context.rowCount || 0), enemyChampionId: Number(context.enemyChampionId || 0),
      selfPosition: String(context.selfPosition || ""), enemyLockedCount: Number(context.enemyLockedCount || 0),
      enemyPositionKnownCount: Number(context.enemyPositionKnownCount || 0), allyPositionKnownCount: Number(context.allyPositionKnownCount || 0),
    };
    if (event === "lane_matchup_card") Object.assign(body, { placement: context.placement === "tab-row" ? "tab-row" : "", mode: String(context.mode || ""), shown: context.shown === true, hiddenReason: String(context.hiddenReason || ""), ownLocked: context.ownLocked === true });
    void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).catch(() => {});
  }

  async function ensureLiveRecommendations(data) {
    const target = liveRecommendationTarget(data);
    if (!target) { recordLiveRecommendationSkip("no-target", null, data); return; }
    const targetKey = target.key;
    // R116-D：阵容（我方 5 人 + 视阶段而定的敌方 5 人）。拿不到阵容就不传 roster
    const roster = liveRecommendationRoster(data);
    const rosterSignature = String(roster?.signature || "");
    if (data?.recommendations) { if (hasUsableLiveRecommendations(data.recommendations)) noteChampSelectReady("build_ready", liveRecommendationTarget(state.live)?.key === targetKey); recordLiveRecommendationSkip("has-payload", target, data); return; }
    // 缓存命中还要再比一次阵容签名：选人阶段英雄是逐个锁定的，阵容变了而
    // key 没变（key 的格式被既有测试钉死、不含阵容），提示就会停在旧阵容上。
    if (state.liveRecommendations.has(targetKey) && (!rosterSignature || state.liveRecommendationRosters?.get?.(targetKey) === rosterSignature)) { noteChampSelectReady("build_ready", liveRecommendationTarget(state.live)?.key === targetKey); recordLiveRecommendationSkip("cached", target, data); return; }
    if (liveRecommendationFlightActive(targetKey)) { recordLiveRecommendationSkip("in-flight", target, data); return; }
    const failedAt = Number(state.liveRecommendationFailures.get(targetKey) || 0);
    if (failedAt > 0 && Date.now() - failedAt < 60_000) { recordLiveRecommendationSkip("backoff", target, data); return; }
    const gameGeneration = Number(state.liveGameGeneration || 0);
    const traceId = newLiveRecommendationTraceId();
    state.liveRecommendationTraces.set(targetKey, traceId);
    state.liveRecommendationFlights.set(targetKey, Date.now());
    // 立刻重绘一次，好让各个页签在等待期间显示加载遮罩，而不是先闪一屏空状态。
    if (liveRecommendationTarget(state.live)?.key === targetKey) renderLive("recommendation");
		const query = new URLSearchParams({
		  championId: String(target.championId), position: target.position,
		  queueId: String(target.queueId), gameMode: target.gameMode, mapId: String(target.mapId), tier: target.tier,
      traceId, recommendationKey: targetKey,
		});
		if (target.gameId > 0) query.set("gameId", String(target.gameId));
    // R207: only champion select refines special-mode builds with spell IDs.
    if (!(target.augmentSource || target.gameMode === "ARAM" || target.mapId === 12) || data?.phase === "ChampSelect") {
      if (Number(target.self?.spell1Id) > 0) query.set("spell1Id", String(target.self.spell1Id));
      if (Number(target.self?.spell2Id) > 0) query.set("spell2Id", String(target.self.spell2Id));
    }
	if (target.augmentSource === "hextech") query.set("source", "mayhem");
	// R116-D P1-2/P1-3/P1-5：只把阵容的英雄 ID 交给后端做交集。
	if (roster?.phase) query.set("phase", roster.phase);
	if (roster?.allyIds?.length) query.set("allyChampionIds", roster.allyIds.join(","));
	if (roster?.enemyIds?.length) query.set("enemyChampionIds", roster.enemyIds.join(","));
    try {
      const response = await api(`/api/gameplay/recommendations?${query}`, target.self?.championLocked ? {} : { priority: "low" }, `live-recommendations:${targetKey}`, 20000);
      if (gameGeneration !== Number(state.liveGameGeneration || 0)) return;
      if (!hasUsableLiveRecommendations(response?.recommendations)) throw new Error("推荐数据不完整");
      const recommendations = response.recommendations;
      recommendations.traceId = recommendations.traceId || traceId;
      recommendations.recommendationKey = recommendations.recommendationKey || targetKey;
      state.liveRecommendations.set(targetKey, recommendations);
      (state.liveRecommendationScopes ||= new Map()).set(targetKey, {
        gameId: target.scopeGameId || target.gameId, championId: target.championId,
        gameMode: target.gameMode, mapId: target.mapId, tier: target.tier, at: Date.now(),
      });
      // R116-D: track roster provenance separately from stable recommendation keys.
      if (rosterSignature) state.liveRecommendationRosters?.set?.(targetKey, rosterSignature);
      noteChampSelectReady("build_ready", liveRecommendationTarget(state.live)?.key === targetKey);
      state.liveRecommendationFailures.delete(targetKey);
      recordItemSetClientDiagnostic("live_recommendations_client", "received", {
        ...target, traceId: recommendations.traceId, recommendationKey: recommendations.recommendationKey,
        requestedPosition: recommendations.requestedPosition || target.position, resolvedPosition: recommendations.resolvedPosition,
        positionSource: recommendations.positionSource, position: recommendations.build?.position || recommendations.resolvedPosition,
      });
      ensurePerks();
      ensureItems();
      ensureSummonerSpells();
	  // The first specialist check races this async recommendation request. Once
	  ensureSpecialistRunes(state.live);
      if (liveRecommendationTarget(state.live)?.key === targetKey) {
        renderLive("recommendation");
        recordItemSetClientDiagnostic("live_recommendations_client", "rendered", {
          ...target, traceId: recommendations.traceId, recommendationKey: recommendations.recommendationKey,
          requestedPosition: recommendations.requestedPosition || target.position, resolvedPosition: recommendations.resolvedPosition,
          positionSource: recommendations.positionSource, position: recommendations.build?.position || recommendations.resolvedPosition,
        });
      }
    } catch (error) {
      if (gameGeneration === Number(state.liveGameGeneration || 0) && error.name !== "RequestCancelled") {
        state.liveRecommendationFailures.set(targetKey, Date.now());
        recordItemSetClientDiagnostic("live_recommendations_client", "failed", {
          ...target, traceId, recommendationKey: targetKey, requestedPosition: target.position,
        });
      }
    } finally {
      if (gameGeneration === Number(state.liveGameGeneration || 0)) state.liveRecommendationFlights.delete(targetKey);
      if (liveRecommendationTarget(state.live)?.key === targetKey) renderLive("recommendation");
    }
  }

  function hasUsableLiveRecommendations(recommendations) {
    if (!recommendations || typeof recommendations !== "object") return false;
    return ["runes", "build", "hero", "augments"].some((key) => {
      const branch = recommendations[key];
      if (Array.isArray(branch)) return branch.length > 0;
      if (!branch || typeof branch !== "object") return false;
      return Object.keys(branch).some((childKey) => {
        const value = branch[childKey];
        return Array.isArray(value) ? value.length > 0 : value != null && (typeof value !== "object" || Object.keys(value).length > 0);
      });
    });
  }

  function liveRecommendationFlightActive(key, now = Date.now()) {
    if (!key || !state.liveRecommendationFlights.has(key)) return false;
    const startedAt = Number(state.liveRecommendationFlights.get(key) || 0);
    if (startedAt > 0 && now - startedAt < 30_000) return true;
    state.liveRecommendationFlights.delete(key);
    return false;
  }

  function recordLiveRecommendationSkip(reason, target, data) {
    const key = String(target?.key || "");
    const dedupKey = `${reason}:${key}`;
    if (state.liveRecommendationSkipDiagnostics.has(dedupKey)) return;
    state.liveRecommendationSkipDiagnostics.add(dedupKey);
    void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ event: "live_recommendations_skip", reason, key, championId: Number(target?.championId || 0), queueId: Number(data?.queueId || 0) }),
    }).then((response) => {
      if (!response.ok) console.warn("live recommendation diagnostic rejected", { reason, key, status: response.status });
    }).catch((error) => {
      console.warn("live recommendation diagnostic failed", { reason, key, error: error?.message || String(error) });
    });
  }

  function proRequestTarget(data) {
    const target = liveRecommendationTarget(data);
    if (!target) return null;
    const mode = String(data?.gameMode || "").toUpperCase();
    const map = Number(data?.mapId || 0);
    if ((map && map !== 11) || (mode && mode !== "CLASSIC")) return null;
    if (map !== 11 && mode !== "CLASSIC" && ![400, 420, 430, 440, 490, 700, 720].includes(Number(data?.queueId))) return null;
    const position = target.positionOverride || liveRecommendationsFor(data)?.resolvedPosition || target.position;
    return { ...target, position, key: `${target.championId}:${position}` };
  }

  function proRunesFor(data) {
    const target = proRequestTarget(data);
    if (!target) return [];
    const entry = state.proRunes?.get(target.key);
    const runes = entry?.pros || liveRecommendationsFor(data)?.runes?.pros || [];
    const cutoff = Date.now() - 30 * 86400000;
    return runes.filter((rune) => !rune.playedAt || Number(rune.playedAt) > cutoff);
  }

  async function ensureProRunes(data, force = false) {
    const target = proRequestTarget(data);
    if (!target || state.proRuneFlights.has(target.key)) return;
    const cached = state.proRunes.get(target.key);
    if (!force && cached && Date.now() - cached.receivedAt < (cached.preparing || cached.outcomesLoading ? 3000 : cached.reason ? 60000 : 300000)) { if (!cached.preparing && !cached.outcomesLoading && !cached.stale && !cached.reason) noteChampSelectReady("pros_ready", proRequestTarget(state.live)?.key === target.key); return; }
    const generation = Number(state.liveGameGeneration || 0);
    state.proRuneFlights.set(target.key, true);
    try {
      const query = new URLSearchParams({ championId: String(target.championId), position: target.position || "", ...(force ? { refresh: "1" } : {}) });
      const response = await api(`/api/gameplay/pro-runes?${query}`, {}, `live-pro-runes:${target.key}`, 28000);
      if (generation !== Number(state.liveGameGeneration || 0)) return;
      if (!Array.isArray(response?.pros)) throw new Error("职业符文数据不完整");
      retainRuneStarterItems(response.pros, cached?.pros);
      if (!response.preparing && !response.outcomesLoading && !response.stale && !response.reason) noteChampSelectReady("pros_ready", proRequestTarget(state.live)?.key === target.key);
      state.proRunes.set(target.key, { ...response, receivedAt: Date.now() });
      if (response.pros.length) { ensurePerks(); ensureItems(); }
    } catch (error) {
      if (generation !== Number(state.liveGameGeneration || 0) || error.name === "RequestCancelled") return;
      state.proRunes.set(target.key, { ...cached, pros: cached?.pros || [], stale: true, reason: "network-unavailable", receivedAt: Date.now() });
    } finally {
      if (generation === Number(state.liveGameGeneration || 0)) {
        state.proRuneFlights.delete(target.key);
        if (proRequestTarget(state.live)?.key === target.key) {
          renderLive("rune");
          clearTimeout(state.proRuneTimer);
          const entry = state.proRunes.get(target.key);
          // SSE is primary; bounded polling recovers a missed/reconnected event.
          state.proRuneTimer = setTimeout(() => { if (state.section === "live") void ensureProRunes(state.live); }, entry?.preparing || entry?.outcomesLoading ? 3200 : 300000);
        }
      }
    }
  }

  async function ensureSpecialistRunes(data) {
    const target = specialistRequestTarget(data);
	if (!target) { recordSpecialistRuneClientSkip("no-target", null, data); return; }
	if (!recommendationQueueHasTopPlayers(data)) { recordSpecialistRuneClientSkip("no-top-players", target, data); return; }
    const embedded = liveRecommendationsFor(data)?.runes?.specialists;
	if (Array.isArray(embedded) && embedded.length) { noteChampSelectReady("specialists_ready", specialistRequestTarget(state.live)?.key === target.key); recordSpecialistRuneClientSkip("embedded", target, data); return; }
	if (state.specialistRunes.has(target.key)) {
      if (!state.specialistRuneFailures.has(target.key)) noteChampSelectReady("specialists_ready", specialistRequestTarget(state.live)?.key === target.key);
	  recordSpecialistRuneClientSkip(state.specialistRunes.get(target.key)?.length ? "cached" : "cached-empty", target, data);
	  return;
	}
	const now = Date.now();
	if (specialistRuneFlightActive(target.key, now)) { recordSpecialistRuneClientSkip("in-flight", target, data); return; }
	const specialistFailure = specialistRuneFailure(state.specialistRuneFailures.get(target.key));
	if (specialistFailure && now - specialistFailure.at < 60_000) {
	  recordSpecialistRuneClientSkip(specialistFailure.reason === "riot-key-missing" ? "riot-key-missing-cooldown" : "recent-failure-cooldown", target, data);
	  return;
	}
	state.specialistRuneFailures.delete(target.key);
	const gameGeneration = Number(state.liveGameGeneration || 0);
	const keyGeneration = Number(state.riotKeyGeneration || 0);
	state.specialistRuneFlights.set(target.key, now);
	    if (specialistRequestTarget(state.live)?.key === target.key) renderLive("rune");
    const query = new URLSearchParams({ championId: String(target.championId), position: target.position });
    try {
	  const response = await api(`/api/gameplay/specialist-runes?${query}`, {}, `live-specialist-runes:${target.key}`, 30000);
	  if (gameGeneration !== Number(state.liveGameGeneration || 0) || keyGeneration !== Number(state.riotKeyGeneration || 0)) return;
	  const runes = Array.isArray(response) ? response : Array.isArray(response?.runes) ? response.runes : null;
      if (!runes) throw new Error("绝活哥符文数据不完整");
	  if (response?.reason === "riot-key-missing" || response?.reason === "riot-key-invalid" || response?.reason === "riot-relay-unavailable") {
		state.specialistRuneFailures.set(target.key, { reason: response.reason, at: Date.now() });
		state.specialistRunes.delete(target.key);
	  } else if (response?.reason === "no-position-sample" && !runes.length) {
		state.specialistRuneFailures.set(target.key, { reason: "no-position-sample", at: Date.now() });
		state.specialistRunes.set(target.key, []);
	  } else if (String(response?.reason || "").startsWith("upstream-")) {
		state.specialistRuneFailures.set(target.key, { reason: String(response.reason), at: Date.now() });
		state.specialistRunes.delete(target.key);
	  } else {
		retainRuneStarterItems(runes, state.specialistRunes.get(target.key));
		state.specialistRunes.set(target.key, runes);
        noteChampSelectReady("specialists_ready", specialistRequestTarget(state.live)?.key === target.key);
		state.specialistRuneFailures.delete(target.key);
	  }
      if (runes.length) ensurePerks();
	      if (specialistRequestTarget(state.live)?.key === target.key) renderLive("rune");
    } catch (error) {
	  if (gameGeneration === Number(state.liveGameGeneration || 0) && keyGeneration === Number(state.riotKeyGeneration || 0) && error.name !== "RequestCancelled") state.specialistRuneFailures.set(target.key, { reason: "request-failed", at: Date.now() });
    } finally {
      if (gameGeneration === Number(state.liveGameGeneration || 0) && keyGeneration === Number(state.riotKeyGeneration || 0) && state.specialistRuneFlights.get(target.key) === now) state.specialistRuneFlights.delete(target.key);
	      if (specialistRequestTarget(state.live)?.key === target.key) renderLive("rune");
    }
  }

  function retainRuneStarterItems(runes, previous) {
    const known = new Map((previous || []).filter(row => Array.isArray(row.starterItemIds)).map(row => [String(row.key), row]));
    for (const row of runes) {
      const cached = known.get(String(row.key));
      if (row.starterItemIds === undefined && cached && Number(cached.playedAt) === Number(row.playedAt)) row.starterItemIds = [...cached.starterItemIds];
    }
  }

  function clearRuneStarterRetries(keepSource = "", keepTarget = "") {
    for (const [key, entry] of state.runeStarterRequests || []) {
      if (entry.source === keepSource && entry.targetKey === keepTarget) continue;
      clearTimeout(entry.timer);
      entry.timer = 0;
      entry.cancelled = true;
      if (entry.pending) state.controllers?.get?.(`live-rune-starters:${key}`)?.abort?.();
    }
  }

  function runeStarterTargetActive(entry) {
    if (entry.cancelled || entry.generation !== Number(state.liveGameGeneration || 0)) return false;
    if (state.section !== "live" || !["ChampSelect", "InProgress"].includes(state.live?.phase) || document.hidden) return false;
    if (state.runeSourceTab !== entry.source) return false;
    const target = entry.source === "pro" ? proRequestTarget(state.live) : specialistRequestTarget(state.live);
    return target?.key === entry.targetKey;
  }

  async function ensureRuneStarterItems(data, retryKey = "") {
    const source = state.runeSourceTab;
    const target = source === "pro" ? proRequestTarget(data) : source === "specialist" ? specialistRequestTarget(data) : null;
    clearRuneStarterRetries(source, target?.key || "");
    if (!target) return;
    const runes = source === "pro" ? proRunesFor(data) : specialistRunesFor(data);
    if (!Array.isArray(runes)) return;
    // Only records rendered for the active player need the second-stage request.
    const visible = new Set([...nodes.liveContent.querySelectorAll(".specialist-game-row[data-rune-choice]")].map(row => row.dataset.runeChoice));
    const rows = runes.filter(row => visible.has(String(row.key)) && row.starterItemIds === undefined)
      .slice(0, source === "pro" ? 5 : 3).map(row => ({ key: String(row.key), playedAt: Number(row.playedAt) }));
    if (!rows.length) return;
    state.runeStarterRequests ||= new Map();
    const requestKey = retryKey || `${source}:${target.key}:${rows.map(row => `${row.key}:${row.playedAt}`).join(",")}`;
    const previous = state.runeStarterRequests.get(requestKey);
    if (previous?.pending || (!retryKey && previous && Date.now() - previous.at < 90000)) return;
    // A partial response changes the missing-row signature. Polls must still
    // respect the pending retry for that batch instead of starting a new one.
    if (!retryKey && [...state.runeStarterRequests.values()].some(entry => entry.source === source && entry.targetKey === target.key && (entry.pending || entry.timer || Date.now() - entry.at < 90000 && entry.rows?.some(old => rows.some(row => row.key === old.key && row.playedAt === old.playedAt))))) return;
    if (retryKey && (!previous || !runeStarterTargetActive(previous) || previous.retries >= 2)) return;
    const generation = Number(state.liveGameGeneration || 0);
    const entry = { source, targetKey: target.key, generation, rows: previous?.rows || rows, pending: true, at: Date.now(), retries: (previous?.retries || 0) + (retryKey ? 1 : 0), timer: 0, cancelled: false };
    state.runeStarterRequests.set(requestKey, entry);
    let retryAfter = 0;
    try {
      const response = await api("/api/gameplay/rune-starters", { method: "POST", body: JSON.stringify({ source, championId: target.championId, position: target.position, rows }) }, `live-rune-starters:${requestKey}`, 32000);
      if (!runeStarterTargetActive(entry) || !Array.isArray(response?.starters)) return;
      const patches = new Map(response.starters.map(row => [String(row.key), row]));
      const current = source === "pro" ? proRunesFor(state.live) : specialistRunesFor(state.live);
      for (const config of current || []) {
        const patch = patches.get(String(config.key));
        if (!patch || Number(patch.playedAt) !== Number(config.playedAt) || !Array.isArray(patch.starterItemIds)) continue;
        config.starterItemIds = patch.starterItemIds.map(Number).filter(id => Number.isInteger(id) && id > 0).slice(0, 6);
      }
      patchRuneStarterEquipment(current || []);
      if (Number.isFinite(Number(response.retryAfterSeconds)) && Number(response.retryAfterSeconds) > 0
        && (current || []).some(row => visible.has(String(row.key)) && row.starterItemIds === undefined)) {
        retryAfter = Math.min(45, Math.max(5, Number(response.retryAfterSeconds)));
      }
    } catch (_) {
      // Opening items are optional; network failures keep the existing cooldown.
    } finally {
      if (state.runeStarterRequests.get(requestKey) === entry) {
        entry.pending = false;
        entry.at = Date.now();
        if (retryAfter && entry.retries < 2 && runeStarterTargetActive(entry)) {
          entry.timer = setTimeout(() => {
            entry.timer = 0;
            if (state.runeStarterRequests.get(requestKey) === entry && runeStarterTargetActive(entry)) void ensureRuneStarterItems(state.live, requestKey);
          }, retryAfter * 1000);
        }
      }
    }
  }

  function patchRuneStarterEquipment(configs) {
    const byKey = new Map(configs.map(config => [String(config.key), config]));
    for (const row of nodes.liveContent.querySelectorAll(".specialist-game-row[data-rune-choice]")) {
      const config = byKey.get(row.dataset.runeChoice);
      if (!config?.starterItemIds?.length) continue;
      const next = document.createElement("div");
      next.innerHTML = renderRuneEquipment(config);
      const starter=next.querySelector(".sg-starters");if(!starter)continue;
      const current=row.querySelector(".sg-starters");
      if(current)current.replaceWith(starter);
      else {const footer=row.querySelector(".specialist-game-footer"),final=row.querySelector(".sg-items");if(final)final.before(starter);else if(footer)footer.prepend(starter);else{const host=row.querySelector(".specialist-game-runes");if(!host)continue;const footer=document.createElement("div");footer.className="specialist-game-footer";footer.append(starter);host.append(footer);}}
      prepareImages(starter);
    }
    rememberLiveRecommendationMarkup();
  }

	function specialistRuneFailure(value) {
	  if (!value) return null;
	  if (typeof value === "number") return { reason: "request-failed", at: value };
	  if (typeof value === "string") return { reason: value, at: 0 };
	  const at = Number(value.at || 0);
	  return at > 0 ? { reason: String(value.reason || "request-failed"), at } : null;
	}

	function specialistRuneFlightActive(key, now = Date.now()) {
	  if (!state.specialistRuneFlights.has(key)) return false;
	  const startedAt = Number(state.specialistRuneFlights.get(key) || 0);
	  if (startedAt > 0 && now - startedAt < 60_000) return true;
	  state.specialistRuneFlights.delete(key);
	  return false;
	}

	function recordSpecialistRuneClientSkip(reason, target, data) {
	  const dedupKey = `${reason}:${String(target?.key || "")}`;
	  if (state.specialistRuneSkipDiagnostics.has(dedupKey)) return;
	  state.specialistRuneSkipDiagnostics.add(dedupKey);
	  const body = JSON.stringify({
		event: "specialist_runes_client_skip", reason,
		championId: Number(target?.championId || 0), position: String(target?.position || ""),
		queueId: Number(data?.recommendation?.queueId ?? data?.queueId ?? 0),
	  });
	  void fetch("/api/diagnostics/client", {
		method: "POST", credentials: "same-origin",
		headers: { "Accept": "application/json", "Content-Type": "application/json" }, body,
	  }).catch(() => {});
	}

	function recordLiveRosterRendered(data, rendered100, rendered200) {
	  const phase = String(data?.phase || "");
	  const gameId = Number(data?.gameId || 0);
	  const playersReceived = Array.isArray(data?.players) ? data.players.length : 0;
	  const hiddenIdentityRendered = Array.isArray(data?.players) ? data.players.filter((player) => player.hidden === true).length : 0;
	  const privateHistoryRendered = Array.isArray(data?.players) ? data.players.filter((player) => player.privateHistory === true).length : 0;
	  const key = `${gameId}:${phase}:${playersReceived}:${rendered100}:${rendered200}:${hiddenIdentityRendered}:${privateHistoryRendered}`;
	  if (state.liveRosterRenderDiagnostics?.has(key)) return;
	  state.liveRosterRenderDiagnostics ||= new Set();
	  state.liveRosterRenderDiagnostics.add(key);
	  void fetch("/api/diagnostics/client", {
	    method: "POST", credentials: "same-origin", headers: { "Accept": "application/json", "Content-Type": "application/json" },
	    body: JSON.stringify({ event: "live_roster_rendered", reason: "render", phase, gameId, queueId: Number(data?.queueId || 0), playersReceived, rendered100, rendered200, hiddenIdentityRendered, privateHistoryRendered }),
	  }).catch(() => {});
	}

  function liveModeLabel(data) {
    const mode = String(data?.gameMode || "").trim().toUpperCase();
    return ({ CLASSIC: "经典模式", ARAM: "极地大乱斗", KIWI: "海克斯大乱斗", CHERRY: "斗魂竞技场", URF: "无限火力", ARURF: "无限火力", NEXUSBLITZ: "极限闪击" })[mode]
      || String(data?.queueLabel || "").trim() || "游戏模式未知";
  }

  function liveMapLabel(mapId) {
    return ({ 11: "召唤师峡谷", 12: "嚎哭深渊", 21: "极限闪击", 30: "斗魂竞技场" })[Number(mapId)] || "地图未知";
  }

  function practicePlayerPosition(player, data) {
    const position = livePositionDisplay(player?.position);
    if (String(data?.gameMode || "").toUpperCase() !== "PRACTICETOOL" || Number(data?.mapId) !== 11 || !player?.isCurrent || ["top","jungle","middle","bottom","utility"].includes(position)) return position;
    const payload = liveRecommendationsFor(data);
    if (payload?.positionSource === "fallback" && !payload?.positions?.length) return position;
    return livePositionDisplay(payload?.resolvedPosition) || position;
  }
	function liveCurrentPositionChip(data) {
	  const recommendation = liveRecommendationsFor(data) || {};
	  const positions = Array.isArray(recommendation.positions) ? recommendation.positions : [];
	  if (!positions.length) return "";
	  const self = (data?.players || []).find((player) => player.isCurrent);
	  const target = liveRecommendationTarget(data);
	  if (String(data?.gameMode || "").toUpperCase() === "PRACTICETOOL") {
        const position = practicePlayerPosition(self, data);
        return ["top","jungle","middle","bottom","utility"].includes(position) ? `<span class="live-current-position">${positionIcon(position)}<span>当前位置：${escapeHTML(positionLabel(position))}</span></span>` : "";
      }
      const resolved = livePositionDisplay(target?.clientPosition || self?.position);
	  if (!["top", "jungle", "middle", "bottom", "utility"].includes(resolved)) return "";
	  const specialist = livePositionDisplay(specialistPosition(data, target));
	  const mismatch = ["top", "jungle", "middle", "bottom", "utility"].includes(specialist) && specialist !== resolved;
	  const marker = mismatch ? `<span class="live-position-mismatch" tabindex="0" data-tooltip="${escapeHTML(`客户端报告的位置是 ${positionLabel(resolved)}，但该英雄在 ${positionLabel(resolved)} 没有样本，下方数据按 ${positionLabel(specialist)} 展示`)}" data-tooltip-size="compact" aria-label="位置数据说明">!</span>` : "";
	  return `<span class="live-current-position">${positionIcon(resolved)}<span>当前位置：${escapeHTML(positionLabel(resolved))}</span>${marker}</span>`;
	}

  function renderSessionSummary(data) {
    if (!nodes.liveSessionSummary) return;
    if (!connected() || !data) {
      nodes.liveSessionSummary.hidden = true;
      nodes.liveSessionSummary.innerHTML = "";
      return;
    }
    const phase = phaseLabel(data.phase);
	    const note = liveAutoRefreshStopped(data) || (state.settings.liveRefresh ? "自动刷新已开启" : "手动刷新");
    nodes.liveSessionSummary.hidden = false;
	    nodes.liveSessionSummary.innerHTML = data.available
	      ? `<span class="state-chip success">${escapeHTML(phase)}</span><div class="live-session-copy"><strong>${escapeHTML(data.queueLabel || "当前对局")}</strong><span>${escapeHTML(liveModeLabel(data))} · ${escapeHTML(liveMapLabel(data.mapId))} · ${escapeHTML(note)}</span></div>${liveCurrentPositionChip(data)}`
	      : `<span class="state-chip">${escapeHTML(phase)}</span><div class="live-session-copy"><strong>${phase === "大厅" ? "等待进入对局" : escapeHTML(phase)}</strong><span>进入英雄选择或游戏后自动读取 · ${escapeHTML(note)}</span></div>`;
  }

  function liveRecommendationMarkup(data) {
    return `<div data-live-notice="squad">${data?.arenaMySquadNotice ? `<p class="arena-my-squad-notice">${escapeHTML(data.arenaMySquadNotice)}</p>` : ""}</div>${renderRecommendationArea(data)}`;
  }

  // R129 P1：参与 markup 比较与增量替换的 HTML 只能由快照数据和用户选择决定，
  // 不能包含 state.liveLoading；瞬时加载状态只更新工具栏的 data-live-status。
  function renderLive(source = "direct") {
    if (state.destroyed || (state.section && state.section !== "live")) return;
    state.liveRenderSource = state.liveRenderTrigger || liveRenderTriggerLabel(source);
    state.liveRenderTrigger = "";
    const toolbar = nodes.liveRefresh?.closest(".live-toolbar");
    const status = toolbar?.querySelector?.("[data-live-status]");
    if (!connected()) {
      // 未连接时收起工具栏（刷新按钮无意义），只保留居中的提示卡。
      if (toolbar) toolbar.hidden = true;
      if (status) status.innerHTML = "";
      renderSessionSummary(null);
      nodes.liveContent.innerHTML = emptyState("等待英雄联盟客户端", "登录国服客户端并进入英雄选择或对局后，这里会自动展示队伍信息与推荐配置。", false);
      return;
    }
    if (toolbar) toolbar.hidden = false;
    const statusMarkup = renderLiveRefreshStatus(state.live || { phase: state.beacon.phase });
    if (status && status.innerHTML !== statusMarkup) {
      status.innerHTML = statusMarkup;
      recordLiveRenderRebuild(["status"]);
    }
    if (nodes.liveRefresh) {
      nodes.liveRefresh.textContent = state.liveLoading && state.liveLoadSource === "manual"
        ? "正在刷新…" : "刷新对局";
      nodes.liveRefresh.setAttribute("aria-busy", String(Boolean(state.liveLoading)));
    }
    if (state.liveError) {
      renderSessionSummary(null);
      nodes.liveContent.innerHTML = emptyState("实时对局读取失败", state.liveError, true);
      nodes.liveContent.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadLive(true, "manual"));
      return;
    }
    if (state.liveAwaitingGame) {
      renderSessionSummary(null);
      nodes.liveContent.innerHTML = `<div role="status">${emptyState("正在识别新对局…", "新对局数据尚未就绪，读取完成后会自动显示。也可点击刷新重试。", true)}</div>`;
      nodes.liveContent.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadLive(true, "manual"));
      return;
    }
    if (state.liveLoading && !state.live) {
      renderSessionSummary(null);
      nodes.liveContent.innerHTML = '<div class="gameplay-skeleton"><span></span><span></span><span></span></div>';
      return;
    }
    const data = state.live;
    renderSessionSummary(data || null);
    if (data?.unsupported) {
      nodes.liveContent.innerHTML = emptyState(data.unsupportedReason || "暂时不支持此模式，敬请期待", "当前模式不会进入阵容与推荐分析。", false);
      return;
    }
    if (!data?.available) {
      nodes.liveContent.innerHTML = renderRecommendationArea(data || {});
      bindLiveContent();
      return;
    }
    const markup = liveRecommendationMarkup(data);
    // Refresh feedback changes independently of the roster and recommendation DOM.
    if (nodes.liveContent._recommendationMarkup === markup && nodes.liveContent.querySelector("[data-live-body]")) {
      return;
    }
    // R129 P2: patch panels first; preserve visible icons unless the shell changes.
    if (updateLivePanels(markup)) {
      nodes.liveContent._recommendationMarkup = markup;
      return;
    }
    const fullHolder = document.createElement("div");
    fullHolder.innerHTML = markup;
    // Keep row signatures independent of attributes added by the image queue.
    stampLiveRows(fullHolder);
    const previousBody = nodes.liveContent.querySelector("[data-live-body]");
    const fullReasons = [];
    const chromePart = (root, selector) => [...(root?.querySelectorAll(selector) || [])].map(node => node.outerHTML).join("");
    if (chromePart(previousBody, ".recommendation-tabs") !== chromePart(fullHolder, ".recommendation-tabs")) fullReasons.push("tab-row");
    if (chromePart(previousBody, ".arena-my-squad-notice, .live-roster-notice, .recommendation-data-notices") !== chromePart(fullHolder, ".arena-my-squad-notice, .live-roster-notice, .recommendation-data-notices")) fullReasons.push("banner");
    if ((previousBody?.querySelectorAll(".recommendation-panel").length || 0) !== fullHolder.querySelectorAll(".recommendation-panel").length) fullReasons.push("panel-count");
    const laneShell = root => [...(root?.querySelectorAll("[data-lane-matchup-slot]") || [])].map(node => { const shell = node.cloneNode(false); return shell.outerHTML; }).join("");
    if (laneShell(previousBody) !== laneShell(fullHolder)) fullReasons.push("lane-slot");
    if (!fullReasons.length) fullReasons.push("other");
    // Compare structural labels only. Never export text, player names or DOM attributes.
    const shellLabels = root => {
      if (!root) return [];
      const shell = root.cloneNode(true);
      for (const panel of shell.querySelectorAll(".recommendation-panel")) panel.remove();
      for (const slot of shell.querySelectorAll("[data-lane-matchup-slot], [data-live-notice], .recommendation-tabs")) slot.innerHTML = "";
      return [...shell.querySelectorAll("*")].map(node => ({ html: node.cloneNode(false).outerHTML, label: `${node.tagName.toLowerCase()}.${String(node.className || "").trim().replace(/\s+/g, ".")}` }));
    };
    const beforeShell = shellLabels(previousBody), afterShell = shellLabels(fullHolder);
    const firstShellIndex = afterShell.findIndex((node, i) => node.html !== beforeShell[i]?.html);
    const shellNode = fullReasons.includes("other") ? String((afterShell[firstShellIndex] || beforeShell[afterShell.length])?.label || "root").slice(0, 120) : "";
    const stats = { imagesRecreated: 0, rowsReplaced: fullHolder.querySelectorAll('[data-live-player-row^="card:"]').length, fullReasons, shellNode };
    preserveLiveImages(nodes.liveContent, fullHolder, stats);
    recordLiveRenderRebuild(["full"], stats);
    nodes.liveContent._recommendationMarkup = markup;
    const body = document.createElement("div");
    body.setAttribute("data-live-body", "");
    body.replaceChildren(...fullHolder.childNodes);
    nodes.liveContent.replaceChildren(body);
    stampLiveRows(nodes.liveContent);
    bindLiveContent();
    applyRenderedMetricStyles(nodes.liveContent);
    prepareImages(nodes.liveContent);
  }

  // R129 P2 的增量替换：chrome＝把每个面板内容清空后剩下的外壳（提示条、页签行、
  // 面板自身的属性与顺序）。chrome 一致才允许只换面板内容，否则交回整块重建。
  function liveBodyChrome(body) {
    const clone = body.cloneNode(true);
    for (const panel of clone.querySelectorAll(".recommendation-panel")) panel.remove();
    for (const slot of clone.querySelectorAll("[data-lane-matchup-slot], [data-live-notice], .recommendation-tabs")) slot.innerHTML = "";
    return clone.innerHTML;
  }

  function captureLiveScroll() {
    return [document.getElementById("app-scroll"), document.querySelector(".app-main")]
      .filter(Boolean)
      .map((root) => [root, Number(root.scrollTop) || 0]);
  }

  function restoreLiveScroll(captured) {
    for (const [root, top] of captured) if (root?.isConnected) root.scrollTop = top;
  }

  function stampLiveRows(root) {
    for (const panel of root.querySelectorAll(".recommendation-panel")) if (panel._livePanelMarkup == null) panel._livePanelMarkup = panel.innerHTML;
    for (const row of root.querySelectorAll("[data-live-player-row], .specialist-game-row[data-rune-choice]")) if (!row._liveMarkup) row._liveMarkup = row.outerHTML;
    for (const img of root.querySelectorAll("img")) if (!img._liveAssetKey) img._liveAssetKey = img.getAttribute("data-queued-src") || img.getAttribute("src") || "";
  }

  function preserveLiveImages(current, next, stats) {
    const images = new Map();
    const originals = new Set(current.querySelectorAll("img"));
    for (const img of originals) {
      const key = img._liveAssetKey || img.getAttribute("data-queued-src") || img.getAttribute("src");
      if (key) { if (!images.has(key)) images.set(key, []); images.get(key).push(img); }
    }
    for (const img of [...next.querySelectorAll("img")]) {
      const key = img._liveAssetKey || img.getAttribute("data-queued-src") || img.getAttribute("src");
      const old = images.get(key)?.shift();
      if (!old || old === img) continue;
      for (const name of ["alt", "title", "width", "height"]) { if (img.hasAttribute(name)) old.setAttribute(name, img.getAttribute(name)); else old.removeAttribute(name); }
      img.replaceWith(old);
    }
    // A same-source replacement must be preserved above, so the recreated
    // count is zero. New sources represent changed content, not reload flicker.
    for (const img of next.querySelectorAll("img")) {
      const key = img._liveAssetKey || img.getAttribute("data-queued-src") || img.getAttribute("src");
      if (!originals.has(img) && [...originals].some(old => (old._liveAssetKey || old.getAttribute("data-queued-src") || old.getAttribute("src")) === key)) stats.imagesRecreated++;
    }
  }

  function patchLiveRosterPanel(current, next, stats) {
    const oldRows = new Map();
    for (const row of current.querySelectorAll("[data-live-player-row]")) if (!oldRows.has(row.dataset.livePlayerRow)) oldRows.set(row.dataset.livePlayerRow, row);
    if (!oldRows.size && !next.querySelector(".live-player-list")) return false;
    const seen = new Set();
    for (const row of [...next.querySelectorAll("[data-live-player-row]")]) {
      const key = row.dataset.livePlayerRow;
      if (seen.has(key)) { row.remove(); continue; }
      seen.add(key);
      const old = oldRows.get(key);
      if (old && (old._liveMarkup || old.outerHTML) === row.outerHTML) {
        // Existing keys move to their new position; stable order preserves nodes too.
        row.replaceWith(old);
      } else {
        row._liveMarkup = row.outerHTML;
        if (old) { preserveLiveImages(old, row, stats); if (key.startsWith("card:")) stats.rowsReplaced++; }
      }
    }
    current.replaceChildren(...next.childNodes);
    return true;
  }

  function updateLivePanels(markup) {
    const body = nodes.liveContent.querySelector("[data-live-body]");
    if (!body) return false;
    const holder = document.createElement("div");
    holder.innerHTML = markup;
    if (liveBodyChrome(body) !== liveBodyChrome(holder)) return false;
    const captured = captureLiveScroll();
    let replaced = false, tabsChanged = false;
    const syncAttributes = (current, next) => {
      for (const attr of [...current.attributes]) if (!next.hasAttribute(attr.name)) current.removeAttribute(attr.name);
      for (const attr of next.attributes) if (current.getAttribute(attr.name) !== attr.value) current.setAttribute(attr.name, attr.value);
    };
    for (const current of body.querySelectorAll("[data-live-notice]")) {
      const next = holder.querySelector(`[data-live-notice="${current.dataset.liveNotice}"]`);
      if (next && current.innerHTML !== next.innerHTML) { current.replaceChildren(...next.childNodes); recordLiveRenderRebuild(["banner"]); replaced = true; }
    }
    const currentTabs = body.querySelector(".recommendation-tabs"), nextTabs = holder.querySelector(".recommendation-tabs");
    if (currentTabs && nextTabs && currentTabs.innerHTML !== nextTabs.innerHTML) {
      const oldTabs = new Map([...currentTabs.children].map(tab => [tab.dataset.recommendationTab, tab]));
      const orderedTabs = [...nextTabs.children].map(tab => {
        const old = oldTabs.get(tab.dataset.recommendationTab);
        if (!old) return tab;
        syncAttributes(old, tab);
        if (old.innerHTML !== tab.innerHTML) old.innerHTML = tab.innerHTML;
        return old;
      });
      const focused = document.activeElement;
      for (const tab of [...currentTabs.children]) if (!orderedTabs.includes(tab)) tab.remove();
      orderedTabs.forEach((tab, index) => { if (currentTabs.children[index] !== tab) currentTabs.insertBefore(tab, currentTabs.children[index] || null); });
      if (focused?.isConnected && document.activeElement !== focused) focused.focus({ preventScroll: true });
      recordLiveRenderRebuild(["tabs"]); replaced = true; tabsChanged = true;
    }
    // Loading/selection attributes and panel membership change without remounting siblings.
    const area = body.querySelector(".recommendation-area");
    for (const panel of [...body.querySelectorAll(".recommendation-panel")]) if (!holder.querySelector(`#${panel.id}`)) panel.remove();
    for (const panel of holder.querySelectorAll(".recommendation-panel")) {
      const current = body.querySelector(`#${panel.id}`);
      if (current) syncAttributes(current, panel);
      else if (area) { const added = panel.cloneNode(true); area.append(added); stampLiveRows(added); bindLivePanelScope(added); applyRenderedMetricStyles(added); prepareImages(added); replaced = true; }
    }
    const currentSlot = body.querySelector("[data-lane-matchup-slot]");
    const nextSlot = holder.querySelector("[data-lane-matchup-slot]");
    if (currentSlot && nextSlot && currentSlot.innerHTML !== nextSlot.innerHTML) {
      stampLiveRows(nextSlot);
      preserveLiveImages(currentSlot, nextSlot, { imagesRecreated: 0, rowsReplaced: 0 });
      currentSlot.replaceChildren(...nextSlot.childNodes);
      stampLiveRows(currentSlot);
      prepareImages(currentSlot);
      replaced = true;
    }
    for (const key of ["runes", "build", "insight"]) {
      const current = body.querySelector(`#recommendation-panel-${key}`);
      const next = holder.querySelector(`#recommendation-panel-${key}`);
      if (!current || !next || (current._livePanelMarkup ?? current.innerHTML) === next.innerHTML) continue;
      current._livePanelMarkup = next.innerHTML;
      let stats = { imagesRecreated: 0, rowsReplaced: 0 };
      if (key === "insight" && patchLiveRosterPanel(current, next, stats)) {
        // The roster rows were patched or moved intact.
      } else {
        stampLiveRows(next);
        const oldRows = new Map([...current.querySelectorAll(".specialist-game-row[data-rune-choice]")].map(row => [row.dataset.runeChoice, row]));
        for (const row of next.querySelectorAll(".specialist-game-row[data-rune-choice]")) {
          const old = oldRows.get(row.dataset.runeChoice);
          if (old && (old._liveMarkup || old.outerHTML) === row.outerHTML) row.replaceWith(old);
        }
        preserveLiveImages(current, next, stats);
        current.replaceChildren(...next.childNodes);
      }
      stampLiveRows(current);
      bindLivePanelScope(current);
      applyRenderedMetricStyles(current);
      prepareImages(current);
      recordLiveRenderRebuild([key], stats);
      replaced = true;
    }
    if (tabsChanged) bindLiveContent();
    if (replaced) restoreLiveScroll(captured);
    return true;
  }

  function livePremadeRoster(players, player) {
	const group = String(player?.premadeGroup || "").trim();
	const expectedSize = Number(player?.premadeSize) || 0;
	if (!group || expectedSize < 2) return [];
	const members = (players || []).filter((candidate) => String(candidate?.premadeGroup || "").trim() === group);
	return members.length === expectedSize ? members : [];
  }

  function renderLivePremadeTag(player, players, currentChampionId = 0) {
	const members = livePremadeRoster(players, player);
	if (members.length < 2) return "";
	const groupNumber = Math.max(1, Number(player.premadeGroup) || 1);
	const colorIndex = (groupNumber - 1) % 12;
	const source = String(player?.premadeSource || "inferred").trim().toLowerCase();
	const roster = members.map((member) => {
	  const index = Math.max(0, players.indexOf(member));
	  return {
		name: maskedPlayerName(member, index),
		championIconURL: Number(member.championId) > 0 && !member.championPickPending ? proxyAsset(assetPath("champion", liveDisplayedChampionId(member, currentChampionId))) : "",
	  };
	});
	const direct = source === "session" || source === "both" || source === "lobby";
	const tooltip = `${direct ? "组队" : "预组队"} ${members.length} 人`;
	return `<span class="premade-team-tag is-color-${colorIndex}" tabindex="0" data-tooltip="${escapeHTML(tooltip)}" data-tooltip-roster="${escapeHTML(JSON.stringify(roster))}" data-tooltip-size="compact">${direct ? "组队" : "预组"} ×${members.length}</span>`;
  }

  function clusterPremadePlayers(players) {
	const ordered = [...(players || [])];
	const consumed = new Set();
	const clustered = [];
	for (const player of ordered) {
	  if (consumed.has(player)) continue;
	  const group = String(player?.premadeGroup || "").trim();
	  const members = livePremadeRoster(ordered, player);
	  if (!group || members.length < 2) {
		consumed.add(player);
		clustered.push(player);
		continue;
	  }
	  for (const member of members) {
		consumed.add(member);
		clustered.push(member);
	  }
	}
	return clustered;
  }

  function proBadgeAttributes(player) {
    const badge = player?.proPlayer;
    if (!badge?.playerName || !badge?.teamCode) return "";
    if (state.settings.maskNames) return 'data-pro-identified="true"';
    return `data-pro-player="${escapeHTML(badge.playerName)}" data-pro-team="${escapeHTML(badge.teamCode)}" data-pro-secondary="${badge.secondary === true}"`;
  }

  function proContextFromButton(button) {
    if (button?.dataset?.proIdentified === "true") return { group: "pro" };
    return button?.dataset?.proPlayer ? { group: "pro", proTeam: button.dataset.proTeam, proPlayer: button.dataset.proPlayer, proSecondary: button.dataset.proSecondary === "true" } : {};
  }

  function renderProIdentityBadge(player) {
    const badge = player?.proPlayer;
    if (state.settings.maskNames || !badge?.playerName || !badge?.teamCode) return "";
    return `<span class="pro-identity-chip${badge.secondary ? " is-secondary" : ""}">${escapeHTML(badge.teamCode)} ${escapeHTML(badge.playerName)}${badge.secondary ? " · 二队" : ""}</span>`;
  }

  function champSelectEnemyPlaceholder(player, phase, players) {
    const selfTeam = Number((players || []).find(p => p.isCurrent)?.teamId);
    return phase === "ChampSelect" && !player.isAlly && !player.isCurrent && selfTeam > 0 && Number(player.teamId) > 0 && Number(player.teamId) !== selfTeam;
  }

  function liveTagVisible(key) {
    try {
      const raw = readSetting("live-tags", "");
      if (!raw) return true;
      const selected = JSON.parse(raw);
      return !Array.isArray(selected) || selected.includes(key);
    } catch (_) { return true; }
  }

  function renderLivePlayer(player, index, arenaMode = false, currentChampionId = 0, premadePlayers = [], recentPositions = "", aramMode = false, hextechMode = false, phase = "", displayPosition = "", identityKind = "") {
    if (champSelectEnemyPlaceholder(player, phase, premadePlayers)) {
      return `<article class="live-player">${iconFigure("champion", liveDisplayedChampionId(player, currentChampionId), player.championName, "live")}<div class="live-player-copy live-player-placeholder">暂无玩家信息，进入游戏后显示</div></article>`;
    }
    const rank = player.rank;
    const games = Array.isArray(player.recentGames) ? player.recentGames.slice(0, 10) : [];
    const recordGames = games.length;
    const wins = games.filter(game => game.win === true).length;
    const totals = games.reduce((total, game) => ({kills: total.kills + Number(game.kills || 0), deaths: total.deaths + Number(game.deaths || 0), assists: total.assists + Number(game.assists || 0)}), {kills: 0, deaths: 0, assists: 0});
    const displayStats = {games: recordGames, wins, losses: recordGames - wins, winRate: recordGames ? wins * 100 / recordGames : 0, kda: (totals.kills + totals.assists) / Math.max(1, totals.deaths)};
    const displayName = maskedPlayerName(player, index);
    const championId = phase !== "ChampSelect" && Number(player.championId)>0 ? Number(player.championId) : liveDisplayedChampionId(player, currentChampionId);
    const portrait = arenaMode && liveBraveryPortrait(player, arenaMode, phase) ? `<span class="game-icon is-live" title="勇敢举动" data-tooltip="勇敢举动"><img src="/bravery.svg" alt="勇敢举动"></span>` : iconFigure("champion",championId,player.championName,"live");
	const historyState = liveHistoryStateOf(player);
	const historyPending = !liveHistorySettled(player);
    const rankCopy = rank?.tier ? rankTitle(rank) : "未定级";
    const contextCopy = player.hidden === true ? (player.position ? positionLabel(player.position) : "") : arenaMode || aramMode ? "" : `${positionLabel(displayPosition || player.position)} · ${rankCopy}`;
    const liveArena = identityKind ? globalThis.deepLegendsLiveArena : null;
    const rowTone = (player.isCurrent ? " is-self" : player.isAlly ? " is-ally" : "") + (player.hidden === true ? " is-hidden" : "");
    const hiddenChip = !liveTagVisible("hidden") ? "" : `${player.privateHistory === true ? '<span class="player-tab-hidden">隐藏战绩</span>' : ""}${player.hidden === true ? '<span class="player-tab-hidden">隐藏身份</span>' : player.identityUnresolved === true ? '<span class="player-tab-hidden">身份待公开</span>' : ""}`;
    const liveAutofillChip = liveTagVisible("autofill") && player.autofill === true ? '<span class="match-autofill-chip">补位</span>' : "";
    const premadeTag = !liveTagVisible("premade") ? "" : renderLivePremadeTag(player, premadePlayers, currentChampionId);
	const emptySummary = historyState === "unavailable" ? player.identityUnresolved === true ? "身份尚未公开" : "客户端未公开该玩家" : historyState === "failed" ? "读取失败" : "本模式暂无战绩";
	const historySummary = historyPending
	  ? '<dl class="is-history-pending" aria-label="战绩读取中"><div><span class="live-history-skeleton"></span></div><div><span class="live-history-skeleton"></span></div><div><span class="live-history-skeleton"></span></div></dl>'
	  : liveArena?.arenaSummary(player, identityKind, emptySummary) || `<dl><div><dt>${recordGames ? `近 ${number(recordGames)} 局` : "当前模式"}</dt><dd class="live-record-value">${displayStats.games ? `${number(displayStats.wins)}胜 ${number(displayStats.losses)}负` : emptySummary}</dd></div>${player.hidden === true && !displayStats.games ? "" : `<div><dt>胜率</dt><dd class="win-rate-value">${displayStats.games ? percent(displayStats.winRate) : "—"}</dd></div><div><dt>KDA</dt><dd class="live-kda-value">${displayStats.games ? `${kda(displayStats.kda)}:1` : "—"}</dd></div>`}</dl>`;
	return `<article class="live-player${rowTone}">${portrait}<div class="live-player-copy"><div class="live-player-identity"><button class="live-player-name" type="button" ${player.playerRef ? `data-player-ref="${escapeHTML(player.playerRef)}" ${proBadgeAttributes(player)}` : "disabled"} data-tooltip="${escapeHTML(displayName)}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(displayName)}</button>${liveTagVisible("self") && player.isCurrent ? '<span class="self-chip">自己</span>' : ""}${hiddenChip}${liveAutofillChip}${premadeTag}${liveTagVisible("squad") && player.mySquad && !(identityKind === "arena" && liveArena) ? '<span class="my-squad-chip">我的小队</span>' : ""}${liveTagVisible("pro") ? renderProIdentityBadge(player) : ""}</div>${liveArena ? liveArena.identityLine(player, identityKind, contextCopy) : contextCopy ? `<span>${phase!=='ChampSelect' || player.hidden || arenaMode || aramMode?escapeHTML(contextCopy):`${escapeHTML(positionLabel(displayPosition || player.position))} · <b class="live-rank-title">${escapeHTML(rankCopy)}</b>`}</span>` : ""}${liveTagVisible("positions") ? recentPositions : ""}</div>${historySummary}</article>`;
  }

	  function orderLivePlayers(players, groupByTeam = true) {
	    const ordered = [...players];
	    const currentFirst = (left, right) => Number(right?.isCurrent === true) - Number(left?.isCurrent === true);
	    const selfTeam = Number(players.find((player) => player?.isCurrent)?.teamId) || 100;
	    const teamOrder = (player) => Number(player?.teamId) === selfTeam ? 0 : 1;
	    if (state.settings.liveOrder === "team") {
	      return ordered.sort((left, right) => (groupByTeam ? teamOrder(left) - teamOrder(right) || Number(left.teamId) - Number(right.teamId) : 0) || currentFirst(left, right));
	    }
	    const positions = { top: 1, jungle: 2, middle: 3, bottom: 4, utility: 5, other: 6 };
	    return ordered.sort((left, right) => {
	      if (groupByTeam && left.teamId !== right.teamId) return teamOrder(left) - teamOrder(right) || Number(left.teamId) - Number(right.teamId);
	      if (state.settings.liveOrder === "position") return (positions[left.position] || 99) - (positions[right.position] || 99);
	      const currentOrder = currentFirst(left, right);
	      if (currentOrder) return currentOrder;
      const leftStats = left.modeStats || {};
      const rightStats = right.modeStats || {};
      const key = state.settings.liveOrder === "kda" ? "kda" : "winRate";
      return Number(rightStats[key] || 0) - Number(leftStats[key] || 0);
    });
  }

  // 统一的居中空状态卡（符文 / 详情 / 出装三个页签共用同一样式与位置）。
  function recommendationEmptyPanel(title, copy) {
    return `<div class="recommendation-empty is-panel"><span aria-hidden="true">⬡</span><strong>${escapeHTML(title)}</strong><p>${escapeHTML(copy)}</p></div>`;
  }

  function recommendationQueueHasTopPlayers(data) {
    const payload = liveRecommendationsFor(data);
    if (typeof payload?.hasTopPlayers === "boolean") return payload.hasTopPlayers;
    const mode = String(data?.gameMode || "").trim().toUpperCase();
    const mapId = Number(data?.mapId || 0);
	return mode === "CLASSIC" && (mapId === 0 || mapId === 11);
  }

  function recommendationCapabilities(data, payload) {
    payload = payload || {};
	const augmentSource = liveAugmentRecommendationSource(data);
	return {
	  hasRunes: augmentSource ? false : typeof payload.hasRunes === "boolean" ? payload.hasRunes : true,
	  hasAugments: Boolean(augmentSource) || payload.hasAugments === true,
	  hasCounters: payload.hasCounters !== false,
	  hasBanRate: payload.hasBanRate !== false,
	  hasTopPlayers: typeof payload.hasTopPlayers === "boolean" ? payload.hasTopPlayers : recommendationQueueHasTopPlayers(data),
	  hasItemDepths: payload.hasItemDepths === true,
    };
  }

  function recommendationTabSpecs(capabilities) {
    const tabs = [];
	if (capabilities.hasRunes) tabs.push(["runes", "符文"]);
	if (capabilities.hasAugments) tabs.push(["build", "海克斯与出装"], ["insight", "详情"]);
	else tabs.push(["insight", "详情"], ["build", "出装与技能"]);
    return tabs;
  }

  function recommendationActiveTab(tabKeys, capabilities) {
    if (!state.recommendationTabTouched && capabilities?.hasAugments && tabKeys.includes("build")) return "build";
    if (!state.recommendationTabTouched && tabKeys.length) return tabKeys[0];
    return tabKeys.includes(state.recommendationTab) ? state.recommendationTab : tabKeys[0];
  }

  function renderRecommendationDataNotices(payload) {
    const notices = [];
    if (payload?.isFallback && payload?.resolvedMode === "aram") notices.push("当前模式的出装与技能暂参考极地大乱斗数据；海克斯推荐仍按当前模式独立展示。");
    if (payload?.isFallback && payload?.resolvedMode === "ranked") notices.push("当前模式尚未识别，出装与技能暂参考经典排位数据。");
    if (payload?.isStale) notices.push(`该模式数据停留在 ${payload.dataVersion || "未知"} 版本，仅供参考。`);
    return notices.length ? `<div class="recommendation-data-notices">${notices.map((copy) => `<p role="note">${escapeHTML(copy)}</p>`).join("")}</div>` : "";
  }

  // 某个推荐页签是否处于「数据还在路上」的状态。没有选定英雄或数据已到齐时
  // 一律返回 false——遮罩只用于真正的等待期，不能盖住空状态和已有内容。
  function recommendationPanelBusy(key, target, payload) {
    if (!target) return false;
    const pending = liveRecommendationFlightActive(target.key);
    if (key === "insight") return false;
	    if (key === "runes") return pending && !payload?.runes;
    if (key === "build") return pending && ((!payload?.build) || (payload?.hasAugments && !payload?.augments));
    return false;
  }

  function laneMatchupEnemies(data, self) {
    // teamId distinguishes other allies even when isAlly only marks self.
    return (data?.players || []).filter(player => !player.isAlly && !player.isCurrent
      && Number(self?.teamId) > 0 && Number(player.teamId) > 0 && Number(player.teamId) !== Number(self.teamId)
      && player.championLocked === true && Number(player.championId) > 0);
  }

  function laneMatchupInference(data, self, position) {
    const enemies = laneMatchupEnemies(data, self);
    const tier = laneMatchupTier({ self });
    const unknown = enemies.filter(player => !livePositionValue(player.position));
    if (!unknown.length) return { reason: "lane-not-locked" };
    const candidates = [];
    for (const enemy of unknown) {
      const lanes = state.laneMatchupLanes?.get(`${Number(enemy.championId)}:${tier}`);
      // Missing competitor shares cannot be treated as zero evidence.
      if (lanes?.status === "failed" || lanes?.status === "succeeded" && !lanes.rows?.length) return { reason: "lanes-failed" };
      if (lanes?.status !== "succeeded") return { reason: "lanes-pending" };
      const rate = Number(lanes.rows.find(row => livePositionValue(row.position) === position)?.rate || 0);
      if (rate >= 0.25) candidates.push({ enemy, rate });
    }
    candidates.sort((left, right) => right.rate - left.rate);
    if (candidates.length === 1) return { enemy: candidates[0].enemy };
    if (candidates.length > 1 && candidates[0].rate >= 0.5 && candidates[0].rate >= 2 * candidates[1].rate) return { enemy: candidates[0].enemy };
    return { reason: candidates.length > 1 ? "lane-ambiguous" : "lane-not-locked" };
  }

  function laneMatchupContext(data) {
    if (data?.phase !== "ChampSelect") return null;
    const self = (data.players || []).find(player => player.isCurrent);
    const position = livePositionValue(self?.position);
    if (!["top", "jungle", "mid", "adc", "support"].includes(position)) return null;
    const known = laneMatchupEnemies(data, self).find(player => livePositionValue(player.position) === position);
    const enemy = known || laneMatchupInference(data, self, position).enemy;
    return enemy ? { self, enemy, position } : null;
  }

  async function ensureLaneMatchupPositions(data) {
    if (data?.phase !== "ChampSelect") return;
    const self = (data.players || []).find(player => player.isCurrent);
    const position = livePositionValue(self?.position);
    if (!["top", "jungle", "mid", "adc", "support"].includes(position)) return;
    const enemies = laneMatchupEnemies(data, self);
    if (enemies.some(player => livePositionValue(player.position) === position)) return;
    const tier = laneMatchupTier({ self });
    state.laneMatchupLanes ||= new Map();
    const missing = [...new Set(enemies.filter(player => !livePositionValue(player.position)).map(player => Number(player.championId)))]
      .filter(id => !state.laneMatchupLanes.has(`${id}:${tier}`)).slice(0, 5);
    if (!missing.length) return;
    const generation = Number(state.liveGameGeneration || 0);
    const key = `${missing.join(",")}:${tier}`;
    const entries = new Map(missing.map(id => [id, { rows: [], status: "pending", controllerKey: `lane-lanes:${key}` }]));
    for (const [id, entry] of entries) state.laneMatchupLanes.set(`${id}:${tier}`, entry);
    try {
      const response = await api(`/api/gameplay/champion-lanes?${new URLSearchParams({ champions: missing.join(","), tier })}`, {}, `lane-lanes:${key}`, 14000);
      for (const [id, entry] of entries) {
        if (state.laneMatchupLanes.get(`${id}:${tier}`) !== entry) continue;
        entry.rows = (Array.isArray(response?.[id]) ? response[id] : []).filter(row => ["top", "jungle", "mid", "adc", "support"].includes(livePositionValue(row.position)) && Number.isFinite(Number(row.rate)) && Number(row.rate) > 0 && Number(row.rate) <= 1);
        entry.status = "succeeded";
      }
    } catch (_) {
      for (const [id, entry] of entries) if (state.laneMatchupLanes.get(`${id}:${tier}`) === entry) entry.status = "failed";
    }
    if (generation === Number(state.liveGameGeneration || 0) && state.live?.phase === "ChampSelect" && state.section === "live") renderLive("recommendation");
  }

  function laneMatchupAvailability(data) {
    const self = (data?.players || []).find(player => player.isCurrent);
    const enemyLockedCount = laneMatchupEnemies(data, self).length;
    const players = data?.players || [];
    const known = player => ["top", "jungle", "mid", "adc", "support"].includes(livePositionValue(player.position));
    return { selfPosition: livePositionValue(self?.position), enemyLockedCount,
      enemyPositionKnownCount: players.filter(player => !player.isAlly && !player.isCurrent && Number(self?.teamId) > 0 && Number(player.teamId) > 0 && Number(player.teamId) !== Number(self.teamId) && known(player)).length,
      allyPositionKnownCount: players.filter(player => Number(self?.teamId) > 0 && Number(player.teamId) === Number(self.teamId) && known(player)).length };
  }

  function laneMatchupOwnChampionId(data, self) {
    if (Number(self?.championId) > 0) return Number(self.championId);
    if (self?.championPickPending) return 0;
    return Number(self?.championPickIntent) || Number(data?.currentChampionId) || 0;
  }

  function laneMatchupTier(_context) {
    return liveRecommendationTier();
  }

  function laneMatchupOwnLocked(self) {
    return self?.championLocked === true;
  }

  function laneMatchupCandidateKey(context) {
    return `${Number(context.enemy.championId)}:${context.position}:${laneMatchupTier(context)}`;
  }

  function laneMatchupPairKey(data, context) {
    return `${laneMatchupOwnChampionId(data, context.self)}:${Number(context.enemy.championId)}:${context.position}:${laneMatchupTier(context)}`;
  }

  async function ensureLaneMatchupPair(data, context) {
    const ownId = laneMatchupOwnChampionId(data, context.self);
    if (data?.phase !== "ChampSelect" || ownId <= 0) return;
    state.laneMatchupPairs ||= new Map();
    const key = laneMatchupPairKey(data, context);
    if (state.laneMatchupPairs.has(key)) return;
    const entry = { status: "pending", data: null };
    state.laneMatchupPairs.set(key, entry);
    const generation = Number(state.liveGameGeneration || 0);
    const query = new URLSearchParams({ champion: String(ownId), enemy: String(context.enemy.championId), position: context.position, tier: laneMatchupTier(context) });
    try {
      const pair = await api(`/api/gameplay/lane-matchup?${query}`, {}, `lane-pair:${key}`, 20000);
      if (generation !== Number(state.liveGameGeneration || 0) || state.live?.phase !== "ChampSelect" || state.laneMatchupPairs.get(key) !== entry) return;
      entry.data = Number(pair?.championId) === ownId && Number(pair?.enemyChampionId) === Number(context.enemy.championId)
        && Number.isFinite(Number(pair.winRate)) && Number(pair.winRate) >= 0 && Number(pair.winRate) <= 100 && Number(pair.games) > 0 ? pair : null;
      entry.status = "succeeded";
    } catch (_) {
      if (generation !== Number(state.liveGameGeneration || 0) || state.live?.phase !== "ChampSelect" || state.laneMatchupPairs.get(key) !== entry) return;
      entry.status = "failed";
    }
    if (state.section === "live") renderLive("recommendation");
  }

  function recordLaneMatchupCandidateSkip(reason, data, context) {
    const shape = laneMatchupAvailability(data);
    const key = `${Number(state.liveGameGeneration || 0)}:${reason}:${Number(data?.gameId || 0)}:${context ? laneMatchupCandidateKey(context) : ""}:${shape.selfPosition}:${shape.enemyLockedCount}:${shape.enemyPositionKnownCount}:${shape.allyPositionKnownCount}`;
    state.laneMatchupDiagnosticSkips ||= new Set();
    if (state.laneMatchupDiagnosticSkips.has(key)) return;
    if (reason === "context-unavailable" && Number(state.laneMatchupUnavailableCount || 0) >= 20) return;
    if (reason === "context-unavailable") state.laneMatchupUnavailableCount = Number(state.laneMatchupUnavailableCount || 0) + 1;
    state.laneMatchupDiagnosticSkips.add(key);
    recordLaneMatchupCandidateDiagnostic(reason, data, context);
  }

  function laneMatchupUnavailableReason(data) {
    const self = (data?.players || []).find(player => player.isCurrent);
    const position = livePositionValue(self?.position);
    if (data?.phase !== "ChampSelect" || !["top", "jungle", "mid", "adc", "support"].includes(position)) return "context-unavailable";
    return laneMatchupInference(data, self, position).reason || "context-unavailable";
  }

  function recordLaneMatchupCandidateDiagnostic(reason, data, context, extra) {
    recordItemSetClientDiagnostic("lane_matchup_candidate_fetch", reason, {
      enemyChampionId: Number(context?.enemy?.championId || 0), queueId: Number(data?.queueId || 0),
      gameId: Number(data?.gameId || 0), position: context?.position || "",
      tier: context ? laneMatchupTier(context) : "", ...laneMatchupAvailability(data), ...(extra || {}),
    });
  }

  async function ensureLaneMatchupCandidates(data) {
    let context = laneMatchupContext(data);
    if (!context) {
      const pending = ensureLaneMatchupPositions(data);
      recordLaneMatchupCandidateSkip(laneMatchupUnavailableReason(data), data, null);
      await pending;
      if (data !== state.live) return;
      context = laneMatchupContext(data);
    }
    if (!context) { recordLaneMatchupCandidateSkip(laneMatchupUnavailableReason(data), data, null); return; }
    const pairRequest = ensureLaneMatchupPair(data, context);
    if (laneMatchupOwnLocked(context.self)) {
      recordLaneMatchupCandidateSkip("own-champion-selected", data, context);
      await pairRequest;
      return;
    }
    state.laneMatchupCandidates ||= new Map();
    const key = laneMatchupCandidateKey(context);
    const cached = state.laneMatchupCandidates.get(key);
    if (cached) {
      const logFlag = cached.status === "pending" ? "inFlightLogged" : "cacheLogged";
      if (!cached[logFlag]) {
        cached[logFlag] = true;
        recordLaneMatchupCandidateDiagnostic(cached.status === "pending" ? "in-flight" : "cached", data, context, { rowCount: cached.rows?.length || 0 });
      }
      await pairRequest;
      return;
    }
    const entry = { rows: [], status: "pending" };
    state.laneMatchupCandidates.set(key, entry);
    const generation = Number(state.liveGameGeneration || 0);
    const query = new URLSearchParams({ mode: "ranked", champion: String(context.enemy.championId), position: context.position, tier: laneMatchupTier(context) });
    recordLaneMatchupCandidateDiagnostic("requested", data, context);
    try {
      const detail = await api(`/api/champions/detail?${query}`, {}, `lane-matchup:${key}`, 20000);
      if (generation !== Number(state.liveGameGeneration || 0) || state.live?.phase !== "ChampSelect" || state.laneMatchupCandidates.get(key) !== entry) return;
      // These losing matchups use the enemy perspective; candidate win rate is inverted.
      entry.rows = (detail?.counters?.weakAgainst || []).filter(row => Number(row.championId) > 0
        && Number.isFinite(Number(row.winRate)) && Number(row.winRate) > 0 && Number(row.winRate) < 50).slice(0, 5);
      entry.status = "succeeded";
      recordLaneMatchupCandidateDiagnostic("succeeded", data, context, { rowCount: entry.rows.length });
    } catch (error) {
      if (generation !== Number(state.liveGameGeneration || 0) || state.live?.phase !== "ChampSelect" || state.laneMatchupCandidates.get(key) !== entry) return;
      entry.status = "failed";
      recordLaneMatchupCandidateDiagnostic("failed", data, context, { httpStatus: Number(error?.status || 0), errorKind: String(error?.errorKind || error?.kind || (error?.status ? "http" : "network")) });
    }
    await pairRequest;
    if (laneMatchupContext(state.live) && state.section === "live") renderLive("recommendation");
  }

  function recordLaneMatchupCardDiagnostic(data, context, mode, shown, hiddenReason) {
    const fields = { mode, shown, placement: "tab-row", hiddenReason: shown ? "" : hiddenReason, ownLocked: laneMatchupOwnLocked(context.self),
      enemyChampionId: Number(context.enemy.championId), tier: laneMatchupTier(context) };
    const key = JSON.stringify(fields);
    state.laneMatchupCardDiagnostics ||= new Set();
    if (state.laneMatchupCardDiagnostics.has(key)) return;
    state.laneMatchupCardDiagnostics.add(key);
    recordItemSetClientDiagnostic("lane_matchup_card", "render", fields);
  }

  function renderLaneMatchupCard(data, _hero) {
    const context = laneMatchupContext(data);
    if (!context) return "";
    const enemyId = Number(context.enemy.championId);
    const ownId = laneMatchupOwnChampionId(data, context.self);
    const locked = laneMatchupOwnLocked(context.self);
    const mode = ownId > 0 ? locked ? "a" : "a+b" : "b";
    const pair = ownId > 0 ? state.laneMatchupPairs?.get(laneMatchupPairKey(data, context)) : null;
    const candidates = locked ? [] : state.laneMatchupCandidates?.get(laneMatchupCandidateKey(context))?.rows || [];
    let content = "";
    if (pair?.status === "succeeded" && pair.data) {
      const winRate = Number(pair.data.winRate);
      const tone = winRate > 50 ? "strong" : winRate < 50 ? "weak" : "even";
      const copy = `${winRate === 50 ? "五五开" : `偏${winRate > 50 ? "优势" : "劣势"}`} ${rate(winRate)}`;
      content += `<div class="lane-matchup-result is-${tone}"><strong>${copy}</strong></div>`;
    }
    if (candidates.length) content += `<div class="lane-matchup-options">${candidates.slice(0, 3).map(row => `<div class="lane-matchup-option" aria-label="${escapeHTML(row.name || "")}" data-tooltip="${escapeHTML(row.name || "")}">${iconFigure("champion", row.championId, row.name || "", "small")}<span>+${rate(50 - Number(row.winRate))}</span></div>`).join("")}</div>`;
    const hiddenReason = ownId > 0 ? !pair || pair.status === "pending" ? "pair-pending" : pair.status === "failed" ? "pair-failed" : "pair-no-data" : "candidates-empty";
    recordLaneMatchupCardDiagnostic(data, context, mode, Boolean(content), hiddenReason);
    if (!content) return "";
    return `<section class="lane-matchup-card" data-lane-matchup-card><div class="lane-matchup-foe">${iconFigure("champion", enemyId, context.enemy.championName || "敌方英雄", "small")}<span>对位 ${escapeHTML(context.enemy.championName || "敌方英雄")}</span></div>${content}</section>`;
  }

  function ensureLiveRecommendationForRender(data, target) {
    if (!target || !data.available || data.recommendations || state.liveRecommendations.has(target.key)) return;
    if (liveRecommendationFlightActive(target.key)) return;
    const failedAt = Number(state.liveRecommendationFailures.get(target.key) || 0);
    if (failedAt > 0 && Date.now() - failedAt < 60_000) return;
    void ensureLiveRecommendations(data);
  }

  function recordLiveRecommendationRender(data, target, payload) {
    const phase = String(data?.phase || "");
    const phaseChanged = phase !== state.liveRecommendationRenderPhase;
    state.liveRecommendationRenderPhase = phase;
    const empty = !payload || (!payload.build && !payload.augments?.length && !payload.runes);
    if (!target || (!phaseChanged && !empty)) return;
    const counts = state.liveRecommendationRenderDiagnostics ||= new Map();
    const key = `${target.scopeGameId}:${target.key}`;
    const count = counts.get(key) || 0;
    if (count >= 5) return;
    counts.set(key, count + 1);
    if (counts.size > 128) counts.delete(counts.keys().next().value);
    const cachedKeys = [...state.liveRecommendations.keys()].filter(key => key.split(":")[0] === String(target.championId)).slice(-5);
    const reset = state.liveScopeLastReset;
    void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ event: "live_recommendation_render", reason: empty ? "empty" : "phase",
        phase, gameId: Number(data?.gameId) || 0, championId: target.championId, targetKey: target.key,
        exactHit: state.liveRecommendations.has(target.key), fallbackHit: Boolean(payload && !data?.recommendations && !state.liveRecommendations.has(target.key)),
        cachedKeys, hasPayloadField: Boolean(data?.recommendations), augmentRows: payload?.augments?.length || 0,
        hasBuild: Boolean(payload?.build), lastResetReason: reset?.reason || "", msSinceReset: reset ? Math.max(0, Date.now() - reset.at) : 0 }),
    }).catch(() => {});
  }

  function renderRecommendationArea(data) {
    const customConfig = window.deepLegendsAutoConfigLive?.prepare(data);
    const self = (data.players || []).find((player) => player.isCurrent);
    const target = liveRecommendationTarget(data);
    const availablePayload = liveRecommendationsFor(data);
    const payload = availablePayload || {};
    recordLiveRecommendationRender(data, target, availablePayload);
    ensureLiveRecommendationForRender(data, target);
    const waiting = !data.available;
    const randomPending = Boolean(self?.championPickPending) || Number(self?.championPickIntent) < 0;
    const noChampion = !waiting && !target;
    const augmentSource = liveAugmentRecommendationSource(data);
    const capabilities = recommendationCapabilities(data, payload);
    if(customConfig)capabilities.hasRunes=true;
    const tabs = recommendationTabSpecs(capabilities);
    const tabKeys = tabs.map(([key]) => key);
    const activeTab = recommendationActiveTab(tabKeys, capabilities);
    if (activeTab !== state.recommendationTab) state.recommendationTab = activeTab;
    const selected = capabilities.hasRunes ? selectedRuneRecommendation(data) : null;
    const selectedComplete = Boolean(selected?.sourceLabel === "自配置" || selected && selected.selectedComplete !== false && (selected.selectedComplete !== true || selected.selectedPerkIds?.length === 9) && (selected.selectedPerkIds || selected.perkIds || []).length >= 6);
    const tab = (key, label) => `<button type="button" role="tab" id="recommendation-tab-${key}" aria-controls="recommendation-panel-${key}" aria-selected="${activeTab === key}" tabindex="${activeTab === key ? "0" : "-1"}" class="${activeTab === key ? "is-active" : ""}" data-recommendation-tab="${key}">${label}</button>`;
    const loadingCopy = {
      runes: ["正在读取符文推荐", "从 OP.GG 拉取当前英雄与位置的符文分布"],
      build: ["正在读取海克斯、出装与技能", "按当前模式整理海克斯、鞋子、核心装备与技能加点"],
      insight: ["正在读取双方玩家战绩", "逐个查询十名玩家的段位与最近对局"],
    };
    const panelLoading = (key) => {
      if (key === "runes" && state.runeSourceTab === "custom" || !recommendationPanelBusy(key, target, payload)) return "";
      const [title, detail] = loadingCopy[key] || ["正在读取数据", "稍候片刻"];
      return `<div class="panel-loading" role="status" aria-live="polite"><div class="panel-loading-content"><span class="panel-spinner" aria-hidden="true"></span><strong>${escapeHTML(title)}</strong></div></div>`;
    };
    const panel = (key, content) => `<div id="recommendation-panel-${key}" class="recommendation-panel${recommendationPanelBusy(key, target, payload) ? " is-loading" : ""}" role="tabpanel" aria-labelledby="recommendation-tab-${key}" ${activeTab !== key ? "hidden" : ""}>${content}${panelLoading(key)}</div>`;
    const content = {};
    if (waiting) {
      content.runes = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示对应模式的符文推荐。");
      content.insight = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示双方玩家与最新战绩。");
      content.build = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示召唤师技能、技能加点与装备分布。");
    } else {
      content.insight = renderLiveInsights(data, payload);
      if (noChampion) {
        const pendingTitle = randomPending ? "随机待定" : "请先选定英雄";
        const pendingCopy = randomPending ? "英雄尚未确定，锁定后自动加载。" : "点击预选或锁定英雄后自动读取推荐数据。";
        content.runes = recommendationEmptyPanel(pendingTitle, pendingCopy);
        content.build = recommendationEmptyPanel(pendingTitle, pendingCopy);
      } else {
        if (capabilities.hasRunes) {
          const runePanel = renderRuneRecommendations(data, true);
          const spellPair = selectedComplete ? recommendedRuneSpellIDs(data, selected) : [];
          const spellToggle = selected?.sourceLabel !== "自配置" && spellPair.length ? `<label class="rune-spell-toggle"><input type="checkbox" data-apply-rune-spells ${state.applyRuneSpells !== false ? "checked" : ""}>同时应用召唤师技能${state.summonerKeys?.flashSlot != null ? `（闪现放在 ${escapeHTML(state.summonerKeys.keys[state.summonerKeys.flashSlot])}）` : ""}</label>` : "";
          content.runes = `${renderChampionRecommendationHeader(payload.hero, self, data)}${runePanel}<footer class="recommendation-action"><div><strong>${selected ? `已选择：${escapeHTML(runeConfigurationTitle(selected))}` : "请选择一套完整符文"}</strong><span>${selectedComplete ? "将更新 [DL] 符文页并设为当前页" : "这套数据尚不完整，暂时不能应用"}</span>${spellToggle}</div><button class="button button-primary apply-runes" type="button" data-apply-runes="selected" ${!selectedComplete || data.phase !== "ChampSelect" ? "disabled" : ""}>${data.phase === "ChampSelect" ? "应用所选符文" : "仅英雄选择阶段可应用"}</button></footer>`;
        }
		const recommendationHeader = renderChampionRecommendationHeader(payload.hero, self, data);
		const augmentContent = capabilities.hasAugments && augmentSource ? renderLiveAugmentRecommendations(data, augmentSource) : "";
		content.build = `${recommendationHeader}${augmentContent}${renderBuildRecommendation(payload.build, self, data.championAbilities || [], data)}`;
      }
    }
 const rosterNotice = data.champSelectNotice ? `<p class="live-roster-notice" role="note"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="M12 11v5m0-9h.01"></path></svg><span>${escapeHTML(data.champSelectNotice)}</span></p>` : "";
	    return `<section class="recommendation-area"><div data-live-notice="data">${renderRecommendationDataNotices(payload)}</div><div class="recommendation-tab-row"><div class="recommendation-tabs" role="tablist" aria-label="推荐类型">${tabs.map(([key, label]) => tab(key, label)).join("")}</div><div data-lane-matchup-slot>${renderLaneMatchupCard(data, payload.hero)}</div><div data-live-notice="roster">${rosterNotice}</div></div>${tabs.map(([key]) => panel(key, content[key] || "")).join("")}</section>`;
  }

	function liveChampionAugmentRows(data, source) {
	  return liveRecommendationsFor(data)?.augments || [];
  }

  function normalizeAugmentRarity(value) {
    const key = String(value || "").trim();
    return ({
      kBronze: { key: "bronze", label: "青铜" }, bronze: { key: "bronze", label: "青铜" },
      kSilver: { key: "silver", label: "白银" }, silver: { key: "silver", label: "白银" },
      kGold: { key: "gold", label: "黄金" }, gold: { key: "gold", label: "黄金" },
      kPrismatic: { key: "prismatic", label: "棱彩" }, prismatic: { key: "prismatic", label: "棱彩" },
      kEventChoice: { key: "event", label: "活动" }, event: { key: "event", label: "活动" },
    })[key] || { key: "unknown", label: "海克斯" };
  }

  function augmentTooltipText(augment, fallbackID = 0) {
    const name = augment?.name || `海克斯 ${augment?.id || fallbackID || ""}`.trim();
    const description = plainText(augment?.description || augment?.tooltip || "");
    return [name, description].filter(Boolean).join("\n");
  }

  function wrapAugmentIcon(icon, augment, fallbackID = 0) {
    const rarity = normalizeAugmentRarity(augment?.rarity);
    const tooltip = augmentTooltipText(augment, fallbackID);
    return `<span class="arena-augment-icon is-${rarity.key}" tabindex="0" data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}">${icon}</span>`;
  }

  function renderLiveAugmentIcon(row, source) {
	const asset = row.assets?.[0] || {};
    const details = {
      ...asset, rarity: row.rarity || asset.rarity,
      description: asset.description || row.description || row.tooltip || asset.tooltip || "",
    };
	const imageSource = asset.source;
	const imagePath = asset.path;
	const fallbackPath = asset.fallbackPath;
	    if (imageSource && imagePath) {
	      const icon = fallbackPath
	        ? remoteStaticIcon(imageSource, imagePath, details.name || "海克斯", "large", false, fallbackPath)
	        : remoteStaticIcon(imageSource, imagePath, details.name || "海克斯", "large", false);
	      return wrapAugmentIcon(icon, details, asset.id || row.id);
    }
    return augmentIconFigure(asset.id || row.id, "large");
  }

  function renderLiveAugmentRecommendations(data, source) {
    const rows = liveChampionAugmentRows(data, source);
    if (!rows.length) {
      const target = liveRecommendationTarget(data);
	  const self = (data?.players || []).find((player) => player.isCurrent);
	  const randomPending = Boolean(self?.championPickPending) || Number(self?.championPickIntent) < 0;
      const loading = target ? liveRecommendationFlightActive(target.key) : false;
	  const failed = state.liveRecommendationFailures.has(target?.key);
      if (loading) return recommendationEmptyPanel("正在读取推荐海克斯", `正在按当前英雄整理${source === "arena" ? "斗魂竞技场" : "海克斯大乱斗"}样本。`);
      if (failed) return randomPending ? recommendationEmptyPanel("随机待定", "英雄尚未确定，锁定后自动加载。") : recommendationEmptyPanel("海克斯推荐暂不可用", "稍后刷新时会自动重试。");
      return recommendationEmptyPanel("暂无该英雄的海克斯样本", "当前数据源还没有足够的英雄关联样本。");
    }
    const renderRow = (row) => {
		  const asset = row.assets?.[0] || {};
      const rarity = normalizeAugmentRarity(row.rarity || asset.rarity);
      const tooltipDetails = {
        ...asset,
        rarity: row.rarity || asset.rarity,
        description: asset.description || row.description || row.tooltip || asset.tooltip || "",
      };
      const tooltip = augmentTooltipText(tooltipDetails, asset.id || row.id);
      const hasMetric = (value) => value !== null && value !== undefined && String(value).trim() !== "" && Number.isFinite(Number(value));
      const metrics = source === "arena"
        ? [["win", "胜率", row.winRate, percent], ["pick", "选用率", row.pickRate > 0 ? row.pickRate : null, percent], ["games", "场次", row.games, number]]
        : [["win", "胜率", row.winRate, percent], ["games", "场次", row.games, number]];
      const stats = metrics.filter(([, , value]) => hasMetric(value)).slice(0, 2).map(([key, label, value, formatter]) => {
        const suffix = key === "games" ? " 场" : "";
        return `<span class="live-augment-stat is-${key}"><span>${label}</span><b>${escapeHTML(formatter(value))}${suffix}</b></span>`;
      }).join("");
      const badge = gradeBadge(row.grade, "live-augment-grade");
      const statsMarkup = stats ? `<small class="live-augment-stats">${stats}</small>` : "";
      return `<article class="live-augment-option is-${rarity.key}" tabindex="0" data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}">${renderLiveAugmentIcon(row, source)}<span><span class="live-augment-title">${badge}<strong>${escapeHTML(asset.name || `海克斯 ${asset.id || row.id}`)}</strong></span>${statsMarkup}</span></article>`;
    };
    const definitions = [["silver", "白银"], ["gold", "黄金"], ["prismatic", "棱彩"]];
    const hasUnknown = rows.some((row) => !definitions.some(([key]) => normalizeAugmentRarity(row.rarity || row.assets?.[0]?.rarity).key === key));
    const columns = [...definitions, ...(hasUnknown ? [["unknown", "品质待确认"]] : [])].map(([key, label]) => {
      const matches = rows
        .filter((row) => normalizeAugmentRarity(row.rarity || row.assets?.[0]?.rarity).key === key)
        .sort((left, right) => gradeRank(left.grade) - gradeRank(right.grade)
          || (Number(right.score) || 0) - (Number(left.score) || 0)
          || (Number(right.games) || 0) - (Number(left.games) || 0)).slice(0, 3);
      if (!matches.length) return "";
      return `<section class="live-augment-column is-${key}"><header><span aria-hidden="true"></span><strong>${label}</strong><small>${matches.length} 个</small></header><div>${matches.map(renderRow).join("")}</div></section>`;
    }).filter(Boolean).join("");
			return `<section class="live-augment-recommendations${source === "hextech" ? " is-hextech" : ""}"><header><h3>海克斯推荐</h3></header><div class="live-augment-columns">${columns}</div></section>`;
  }

  // “详情”页签：双方队伍与每名玩家的最近战绩（英雄、KDA、评分），从新到旧。
  function insightScore(game) {
    const value = (Number(game.kills) + Number(game.assists)) / Math.max(1, Number(game.deaths));
    const tone = value >= 4 ? "is-gold" : value >= 3 ? "is-good" : value < 1.5 ? "is-poor" : "";
    return `<i class="insight-score ${tone}">${value.toFixed(1)}</i>`;
  }

  function renderInsightMatches(player) {
	if (player.hidden === true) return '<div class="insight-match-row is-hidden-player" aria-hidden="true"></div>';
	// R129 P1：与 renderLivePlayer 共用同一个判据，两处词表不会再各写一份。
	if (!liveHistorySettled(player)) return '<div class="insight-match-row is-history-pending" aria-label="战绩读取中"><span class="live-history-skeleton"></span><span class="live-history-skeleton"></span><span class="live-history-skeleton"></span></div>';
    const games = (player.recentGames || []).slice(0, 10);
	if (!games.length) {
	  const historyState = liveHistoryStateOf(player);
	  if (historyState === "unavailable") return `<div class="insight-match-row"><small class="insight-none">${player.identityUnresolved === true ? "身份尚未公开" : "客户端未公开该玩家"}</small></div>`;
	  if (historyState === "failed") return '<div class="insight-match-row"></div>';
	  return '<div class="insight-match-row"><small class="insight-none">该玩家当前模式暂无最近战绩</small></div>';
	}
    const cells = globalThis.deepLegendsLiveArena?.chips(games, player) ??
      games.map((game) => `<span class="insight-match is-${game.win ? "win" : "loss"}" data-tooltip="${escapeHTML(`${game.championName || "英雄"} · ${number(game.kills)}/${number(game.deaths)}/${number(game.assists)}${Number(game.cs) > 0 ? ` · CS ${number(game.cs)}` : ""} · ${game.win ? "胜利" : "失败"}`)}" data-tooltip-size="compact">${iconFigure("champion", game.championId, game.championName, "tiny")}<b>${number(game.kills)}/${number(game.deaths)}/${number(game.assists)}</b>${insightScore(game)}</span>`).join("");
    return `<div class="insight-match-row" aria-label="当前模式最近战绩，从左到右由新到旧">${cells}</div>`;
  }

  function renderLiveRecentPositions(player, queueID) {
    if (![420, 440].includes(Number(queueID))) return "";
    const labels = { top: "上路", jungle: "打野", middle: "中路", bottom: "下路", utility: "辅助" };
    const positions = [...new Set((Array.isArray(player.recentPositions) ? player.recentPositions : [])
      .filter((item) => item && Number(item.games) > 0 && Object.hasOwn(labels, item.position))
      .map((item) => labels[item.position]))];
    if (!positions.length) return "";
    return `<div class="live-recent-positions" aria-label="常用位置"><span class="live-position-label">常用位置：</span>${positions.map((label) => `<span class="live-position-sample">${label}</span>`).join("")}</div>`;
  }

	  function arenaLivePlayerGroups(data, players) {
	    if (!["GameStart", "InProgress", "Reconnect"].includes(data?.phase) || data?.arenaGrouped !== true || players.length < 6 || players.length > 18) return [];
	    const grouped = new Map();
	    for (const player of players) {
	      const key = String(player?.arenaGroup || "").trim();
	      if (!key) return [];
	      if (!grouped.has(key)) grouped.set(key, []);
	      grouped.get(key).push(player);
	    }
	    if (grouped.size < 2 || [...grouped.values()].some((group) => group.length > 3)) return [];
	    const keys = [...grouped.keys()].sort((left, right) => {
	      const leftNumber = Number(left);
	      const rightNumber = Number(right);
	      if (Number.isFinite(leftNumber) && Number.isFinite(rightNumber)) return leftNumber - rightNumber;
	      return left.localeCompare(right, "zh-CN", { numeric: true });
	    });
	    return keys.map((key, index) => {
	      const clientName = String(data?.arenaGroupNames?.[key] || "").trim();
	      const mascot = data?.arenaMascotMapping === true ? arenaTeamMeta({ subteamId: Number(key) }) : null;
	      return { key, label: clientName || mascot?.name || `小队 ${index + 1}`, players: grouped.get(key).sort((left, right) => Number(right?.isCurrent === true) - Number(left?.isCurrent === true)) };
	    }).sort((left, right) => Number(right.players.some(player => player.isCurrent)) - Number(left.players.some(player => player.isCurrent)));
	  }

  function insightTeamLayout(players, enabled) {
    const teams = new Map([100, 200].map((teamID) => [teamID, players.filter((player) => Number(player.teamId) === teamID)]));
    if (!enabled) return { teams, aligned: false, reason: "" };
    const positions = { top: 1, jungle: 2, middle: 3, bottom: 4, utility: 5, other: 6 };
    for (const teamID of [100, 200]) {
      const rows = teams.get(teamID) || [];
      // 同队可能出现重复或未知位置；只按已知位置排序，同位置保留原顺序。
      teams.set(teamID, [...rows].sort((left, right) =>
        (positions[String(left.position || "").toLowerCase()] || 6) - (positions[String(right.position || "").toLowerCase()] || 6)));
    }
    return { teams, aligned: true, reason: "" };
  }

  // ---------------------------------------------------------------------
  function liveRosterChampionName(players, championID) {
    const wanted = Number(championID) || 0;
    if (wanted <= 0) return "";
    const match = (players || []).find((player) => Number(player.championId) === wanted || Number(player.championPickIntent) === wanted);
    return String(match?.championName || "").trim();
  }

  // Convert upstream deltas to percentage points; direction determines the sign.
  function liveDeltaPoints(value) {
    return (Math.abs(Number(value) || 0) * 100).toFixed(1);
  }

  function liveConfidenceText(notice) {
    const low = Number(notice?.confidenceLow);
    const high = Number(notice?.confidenceHigh);
    if (!Number.isFinite(low) || !Number.isFinite(high) || (low <= 0 && high <= 0)) return "";
    return `上游置信区间 ${(low * 100).toFixed(1)}%–${(high * 100).toFixed(1)}%`;
  }

  function liveEvidenceSuffix(notice) {
    const evidence = String(notice?.evidence || "").trim();
    // Keep evidence and confidence bounds so non-significant deltas can degrade.
    if (!evidence) return "上游未标注证据等级";
    if (evidence !== "supported") return `上游证据等级：${evidence}（未达显著性）`;
    return "";
  }

  // modifier 传的是完整类名字面量（"is-weak" / "is-strong" / "is-synergy" /
  function liveNoticeBar(modifier, text, tooltip) {
    return `<span class="roster-notice ${modifier}"${tooltip ? ` data-tooltip="${escapeHTML(tooltip)}" tabindex="0"` : ""}>${text}</span>`;
  }

  function renderLiveRosterNoticeBars(payload, players) {
    const bars = [];
    for (const notice of (payload?.matchupNotices || []).slice(0, 5)) {
      const name = liveRosterChampionName(players, notice?.opponentChampionId);
      if (!name) continue;
      const weakened = liveEvidenceSuffix(notice);
      if (weakened) {
        bars.push(liveNoticeBar("is-unverified", `${escapeHTML(String(notice?.direction === "strong" ? "克制" : "被克制"))} ${escapeHTML(name)} · ${escapeHTML(weakened)}`, liveConfidenceText(notice)));
        continue;
      }
      const strong = String(notice?.direction) === "strong";
      const sign = strong ? "+" : "−";
      const verb = strong ? "克制" : "被";
      const tail = strong ? "" : " 克制";
      bars.push(liveNoticeBar(strong ? "is-strong" : "is-weak",
        `${escapeHTML(verb)} <b>${escapeHTML(name)}</b>${escapeHTML(tail)} · 胜率 ${sign}${liveDeltaPoints(notice?.counterDelta)} 个百分点`,
        liveConfidenceText(notice)));
    }
    for (const notice of (payload?.synergyNotices || []).slice(0, 5)) {
      const name = liveRosterChampionName(players, notice?.teammateChampionId);
      if (!name) continue;
      const weakened = liveEvidenceSuffix(notice);
      if (weakened) {
        bars.push(liveNoticeBar("is-unverified", `与 ${escapeHTML(name)} 协同 · ${escapeHTML(weakened)}`, liveConfidenceText(notice)));
        continue;
      }
      bars.push(liveNoticeBar("is-synergy",
        `与 <b>${escapeHTML(name)}</b> 协同 · 胜率 +${liveDeltaPoints(notice?.synergyDelta)} 个百分点`,
        liveConfidenceText(notice)));
    }
    if (!bars.length) return "";
    return `<div class="roster-notice-strip" role="note" aria-label="阵容克制与协同提示">${bars.join("")}</div>`;
  }

  function renderLiveTeamPortraitTags(payload) {
    const labels = (payload?.teamPortrait?.labels || []).filter((label) => String(label?.label || "").trim() !== "");
    if (!labels.length) return "";
    const resolved = Number(payload?.teamPortrait?.resolvedHeroes) || 0;
    const tooltip = resolved > 0 ? `本队 ${resolved} 位英雄进入统计` : "";
    // 缺口类型放在 data-portrait-gap 属性上而不是 is-<key> 类名上：
    return `<span class="team-portrait-tags"${tooltip ? ` data-tooltip="${escapeHTML(tooltip)}" tabindex="0"` : ""}>${labels.map((label) => `<b class="team-portrait-tag" data-portrait-gap="${escapeHTML(String(label.key || ""))}">${escapeHTML(String(label.label))}</b>`).join("")}</span>`;
  }

  // R116-D: payload is optional for legacy focused render callers.
  function renderLiveInsights(data, payload) {
    const seenPlayerKeys = new Set();
    let duplicatesDropped = 0;
    const players = (data.players || []).filter((player, index) => {
      const key = String(player.playerRef || `${player.teamId}:${player.cellId ?? index}`);
      if (seenPlayerKeys.has(key)) { duplicatesDropped++; return false; }
      seenPlayerKeys.add(key); return true;
    });
    if (duplicatesDropped) {
      const diagnosticKey = `${data.gameId || 0}:${data.phase || ""}:${duplicatesDropped}`;
      state.liveDuplicateDiagnostics ||= new Set();
      if (!state.liveDuplicateDiagnostics.has(diagnosticKey)) { state.liveDuplicateDiagnostics.add(diagnosticKey); window.reportFlowDiagnostic?.("live_roster_duplicate_dropped", "deduplicated", { gameId: Number(data.gameId || 0), queueId: Number(data.queueId || 0), count: duplicatesDropped }); }
    }
    if (!players.length) return '<div class="recommendation-empty"><strong>暂无玩家数据</strong><p>进入英雄选择或对局后，这里会展示自己、队友与对手的队伍信息与最近战绩。</p></div>';
		const arenaMode = liveAugmentRecommendationSource(data) === "arena";
		const basePlayers = orderLivePlayers(players, !arenaMode);
		const orderedPlayers = arenaMode ? [
          ...basePlayers.filter(player => player.isCurrent),
          ...basePlayers.filter(player => !player.isCurrent && player.mySquad),
          ...clusterPremadePlayers(basePlayers.filter(player => !player.isCurrent && !player.mySquad)),
        ] : basePlayers;
	    recordLiveRosterRendered(data, orderedPlayers.filter((player) => player.teamId === 100).length, orderedPlayers.filter((player) => player.teamId === 200).length);
    const selfPlayer = orderedPlayers.find((player) => player.isCurrent && [100, 200].includes(Number(player.teamId)));
    const selfTeam = Number(selfPlayer?.teamId) || 100;
    const foeTeam = selfTeam === 200 ? 100 : 200;
    const relativeTeamsKnown = Boolean(selfPlayer);
    const positionAligned = !arenaMode && isSummonersRiftMatch(data);
    const alignment = insightTeamLayout(orderedPlayers, positionAligned);
    if (!arenaMode && !positionAligned) {
      for (const teamID of [100, 200]) alignment.teams.set(teamID, clusterPremadePlayers(alignment.teams.get(teamID)));
    }
    const headerNotice = !relativeTeamsKnown ? "无法确定你所在阵营，按蓝方/红方展示" : alignment.reason;
    const bars = renderLiveRosterNoticeBars(payload, players);
    const portraitTags = renderLiveTeamPortraitTags(payload);
    const supplement = bars ? `<div class="live-roster-supplement">${bars}</div>` : "";
    const liveGameMode = String(data.gameMode || "").toUpperCase();
    const identityKind = arenaMode ? "arena" : isARAMRelatedMatch(data) ? "aram" : isSummonersRiftMatch(data) && ["", "CLASSIC", "PRACTICETOOL"].includes(liveGameMode) ? "rift" : "none";
    const playerRows = (source) => source.map((player, index) => {
        const key = escapeHTML(String(player.playerRef || `${player.teamId}:${player.cellId ?? index}`));
        const card = renderLivePlayer(player, index, arenaMode, data.currentChampionId, orderedPlayers, renderLiveRecentPositions(player, data.queueId), isARAMRelatedMatch(data), liveAugmentRecommendationSource(data) === "hextech", data.phase, liveGameMode === "PRACTICETOOL" ? practicePlayerPosition(player, data) : "", identityKind)
          .replace('<article ', `<article data-live-player-row="card:${key}"${data.phase==='ChampSelect'?' data-champselect-insight':''} `);
        const history = champSelectEnemyPlaceholder(player, data.phase, orderedPlayers) ? '<div class="insight-match-row"></div>' : renderInsightMatches(player);
        return card + history.replace('<div ', `<div data-live-player-row="history:${key}" `);
      }).join("");
    const team = (teamID, label, tone, source = orderedPlayers, extra = "") => {
      const rows = playerRows(source.filter((player) => teamID === null || player.teamId === teamID));
      return `<section class="live-team is-${tone}"><header><h3>${escapeHTML(label)}</h3><span>${escapeHTML(headerNotice || "当前模式最新战绩")}</span>${extra}</header><div class="live-player-list is-insight">${rows || '<p class="section-empty">客户端暂未公开这一队的玩家</p>'}</div></section>`;
    };
		if (arenaMode) {
		  const groups = arenaLivePlayerGroups(data, orderedPlayers);
		  if (groups.length >= 2) {
		    return `<div class="live-teams is-insight is-arena is-grouped">${groups.map((group) => team(null, group.label, "arena", group.players)).join("")}</div>`;
		  }
		  const title = data.phase === "ChampSelect" ? "己方小队" : "全部玩家";
		  return globalThis.deepLegendsLiveArena?.squadsMarkup(orderedPlayers, data.arenaSquadSize, playerRows) || `<div class="live-teams is-insight is-arena">${team(null, title, "arena")}</div>`;
		}
    if (!relativeTeamsKnown) return `${supplement}<div class="live-teams is-insight">${team(100, "蓝方", "blue", alignment.teams.get(100))}${team(200, "红方", "red", alignment.teams.get(200))}</div>`;
    return `${supplement}<div class="live-teams is-insight">${team(selfTeam, "我方", "blue", alignment.teams.get(selfTeam), portraitTags)}${team(foeTeam, "对方", "red", alignment.teams.get(foeTeam))}</div>`;
  }

  function renderChampionRecommendationHeader(hero, self, data = state.live) {
    const stats = hero || {};
    const championId = liveRecommendationChampionId(self) || Number(data?.currentChampionId) || 0;
    const target = liveRecommendationTarget(data);
    const payload = liveRecommendationsFor(data) || {};
	const positions = Array.isArray(payload.positions) ? payload.positions : [];
	const totalPositionPlay = positions.reduce((total, item) => total + Math.max(0, Number(item.play) || 0), 0);
	const orderedPositions = positions.map((item) => {
	  const explicitRate = Math.max(0, Number(item.roleRate) || 0);
	  const play = Math.max(0, Number(item.play) || 0);
	  return { ...item, displayRoleRate: explicitRate || (totalPositionPlay > 0 ? play / totalPositionPlay * 100 : 0) };
	}).sort((left, right) => right.displayRoleRate - left.displayRoleRate || (Number(right.play) || 0) - (Number(left.play) || 0));
    const resolvedPosition = livePositionValue(target?.positionOverride || payload.resolvedPosition || target?.position || self?.position);
    // 对抗数据来自 OP.GG 的对线样本。冷门英雄/冷门位置（例如下路雷克顿）上游本来就
    // 没有样本，此时给出明确说明，而不是一个让人以为是 bug 的“—”。
    const positionCopy = positionLabel(livePositionDisplay(resolvedPosition));
    const emptyMatchups = `<span class="muted matchups-empty" data-tooltip="${escapeHTML(`OP.GG 没有「${self?.championName || "该英雄"} · ${positionCopy}」的对线样本，通常是这个位置太冷门。换到常用位置就会有数据。`)}" data-tooltip-size="compact">该位置暂无对线样本</span>`;
    const matchups = (items, tone) => {
      const values = (items || []).slice(0, 3);
      if (!values.length) return emptyMatchups;
      return values.map((item) => {
      const name = item.championName || championDisplayName(item.championId || item.id);
      const hasWinRate = item.winRate != null && Number.isFinite(Number(item.winRate));
      const detail = [hasWinRate && `${percent(item.winRate)} 胜率`, Number(item.games) > 0 && `${number(item.games)} 场`].filter(Boolean).join(" · ");
      const detailHTML = [hasWinRate && `<b class="win-rate-value">${percent(item.winRate)}</b> 胜率`, Number(item.games) > 0 && `${number(item.games)} 场`].filter(Boolean).join(" · ");
      return `<span class="recommendation-matchup is-${tone}" data-tooltip="${escapeHTML(detail || name)}" data-tooltip-size="compact">${iconFigure("champion", item.championId || item.id, name, "small", false)}<span><b>${escapeHTML(name)}</b><small>${detailHTML || "暂无统计"}</small></span></span>`;
      }).join("") + Array.from({ length: 3 - values.length }, () => '<span class="recommendation-matchup is-placeholder" aria-hidden="true"></span>').join("");
    };
    const automatic = String(data?.gameMode || "").toUpperCase() !== "PRACTICETOOL" && !target?.positionOverride && payload.positionSource && payload.positionSource !== "requested";
    const sourceCopy = payload.positionSource === "opgg-primary"
	  ? `当前分路不在 OP.GG 常用位置中，已按${orderedPositions[0]?.displayRoleRate ? `${rate(orderedPositions[0].displayRoleRate)} 场次占比` : "最高场次占比"}选择${positionLabel(livePositionDisplay(resolvedPosition))}`
      : payload.positionSource === "fallback"
      ? `OP.GG 暂无常用位置，已使用${positionLabel(livePositionDisplay(resolvedPosition))}`
      : "";
	const positionSwitch = orderedPositions.length >= 1 ? `<div class="live-position-switch" role="group" aria-label="当前英雄推荐分路">${orderedPositions.map((item) => { const value = livePositionValue(item.position); const active = value === resolvedPosition; const display = livePositionDisplay(value); return `<button type="button" class="live-position-chip${active ? " is-active" : ""}" aria-pressed="${active}" data-live-position="${escapeHTML(value)}"${automatic && active ? ` data-tooltip="${escapeHTML(sourceCopy)}" data-tooltip-size="compact"` : ""}>${positionIcon(display)}<span>${escapeHTML(positionLabel(display))}</span>${item.displayRoleRate > 0 ? `<b class="live-position-rate">${rate(item.displayRoleRate)}</b>` : ""}</button>`; }).join("")}</div>` : "";
	// Requested lanes remain authoritative. Below 10%, disclose the niche sample
	// instead of silently making this view look equivalent to a primary lane.
	const resolvedPositionRate = orderedPositions.find((item) => livePositionValue(item.position) === resolvedPosition)?.displayRoleRate || 0;
	const positionContext = payload.positionSource === "requested" && resolvedPositionRate > 0 && resolvedPositionRate < 10
	  ? `<p class="live-position-context" role="note">当前展示的是${escapeHTML(positionCopy)}数据（该英雄此分路占比 ${rate(resolvedPositionRate)}）</p>`
	  : "";
	const positionControls = positionSwitch || positionContext ? `<div class="live-position-controls">${positionSwitch}${positionContext}</div>` : "";
	const hasBanRate = payload.hasBanRate !== false;
	const hasMetric = (value) => value !== null && value !== undefined && String(value).trim() !== "" && Number.isFinite(Number(value));
	const summaryMetrics = [
	  ["win", "胜率", stats.winRate, true],
	  ["pick", "选取率", stats.pickRate, payload.source !== "arena"],
	  ["ban", "禁用率", stats.banRate, hasBanRate],
	].filter(([, , value, enabled]) => enabled && hasMetric(value)).map(([tone, label, value]) => `<div class="is-${tone}"><dt>${label}</dt><dd>${rate(value)}</dd></div>`).join("");
	const statsContent = stats.emptyReason
	  ? `<p class="champion-stats-empty" role="status">${escapeHTML(stats.emptyReason)}</p>`
	  : summaryMetrics ? `<dl class="champion-summary-stats">${summaryMetrics}</dl>` : "";
	const hasCounters = payload.hasCounters !== false;
	const matchupHTML = hasCounters ? `<div class="champion-matchups"><div><span>优势对抗</span>${matchups(stats.strongAgainst, "strong")}</div><div><span>劣势对抗</span>${matchups(stats.weakAgainst, "weak")}</div></div>` : "";
	const showTier = Boolean(liveAugmentRecommendationSource(data));
	const tierContent = showTier && stats.grade ? `<div class="champion-summary-tier"><span>梯度</span><strong>${gradeBadge(stats.grade, "is-metric")}</strong></div>` : "";
	return `<section data-champion-id="${championId}" class="recommendation-champion-summary${hasCounters ? "" : " is-no-counters"}${tierContent ? " has-tier" : ""}"><div class="champion-summary-main">${championId ? iconFigure("champion", championId, self?.championName || "当前英雄", "live") : '<span class="game-icon is-live">?</span>'}<div><strong>${escapeHTML(self?.championName || (championId ? "当前英雄" : "尚未选择英雄"))}</strong><span>${escapeHTML(positionLabel(livePositionDisplay(resolvedPosition)))}</span></div>${statsContent}</div>${positionControls}${matchupHTML}${tierContent}</section>`;
  }

  function renderRuneRecommendations(data, championSelected) {
    const payload = liveRecommendationsFor(data) || {};
    const runes = payload.runes || {};
    const target = liveRecommendationTarget(data);
		const specialistTarget = specialistRequestTarget(data);
		const specialistFailed = Boolean(specialistTarget && specialistRuneFailure(state.specialistRuneFailures.get(specialistTarget.key)) && !state.specialistRunes.has(specialistTarget.key));
    const opggItems = Array.isArray(runes.opgg) ? runes.opgg : runes.opgg ? [runes.opgg] : [];
    const positions = Array.isArray(payload.positions) ? payload.positions : [];
    const opggFallback = opggItems.length ? opggItems : !positions.length && data.clientRecommendation ? [{ ...data.clientRecommendation, key: "opgg", title: "客户端内置（备用）" }] : [];
    const custom = window.deepLegendsAutoConfigLive?.selection(data);
    const sections = [custom ? {key:"custom",title:"自配置",items:[custom]} : { key: "opgg", title: "OPGG", items: opggFallback }];
	const hasTopPlayers = recommendationQueueHasTopPlayers(data);
	if (proRequestTarget(data)) sections.push({ key: "pro", title: "职业选手", items: proRunesFor(data), proStatus: state.proRunes?.get(proRequestTarget(data).key), loading: state.proRuneFlights?.has(proRequestTarget(data).key) });
	sections.push({ key: "specialist", title: "绝活哥", items: hasTopPlayers ? specialistRunesFor(data) : [], loading: Boolean(hasTopPlayers && specialistTarget && specialistRuneFlightActive(specialistTarget.key)), failed: specialistFailed, unsupported: !hasTopPlayers });
    const activeKey = sections.some((section) => section.key === state.runeSourceTab) ? state.runeSourceTab : sections[0]?.key || "opgg";
    if (state.runeSourceTab !== activeKey) state.runeSourceTab = activeKey;
    const sourceTabs = sections.map((section) => `<button type="button" class="rune-source-tab${section.key === activeKey ? " is-active" : ""}" data-rune-source="${section.key}" role="tab" aria-selected="${section.key === activeKey}">${escapeHTML(section.title)}</button>`).join("");
    const activeSection = sections.find((section) => section.key === activeKey) || sections[0];
    const sourceNote = activeSection?.note ? `<small class="rune-source-note">${escapeHTML(activeSection.note)}</small>` : "";
    return `<div class="rune-source-tabs"><div class="rune-source-tab-buttons" role="tablist" aria-label="符文数据来源">${sourceTabs}</div>${sourceNote}</div><div class="rune-source-stack">${activeSection ? renderRuneSourceSection(activeSection, championSelected) : ""}</div>`;
  }

  function recommendedRuneSpellIDs(data, recommendation) {
    if (!recommendation) return [];
    let ids = [];
    if (recommendation.sourceLabel === "OPGG") {
      const build = liveRecommendationsFor(data)?.build;
      ids = build?.spellOptions?.[0]?.ids || build?.spells || [];
    } else if (["绝活哥", "职业选手"].includes(recommendation.sourceLabel)) {
      ids = [recommendation.spell1Id, recommendation.spell2Id];
    }
    const pair = Array.isArray(ids) ? ids.map(Number) : [];
    return pair.length === 2 && pair.every((id) => Number.isInteger(id) && id > 0 && id <= 100000) && pair[0] !== pair[1] ? pair : [];
  }

  function renderRuneSpellPair(ids) {
    const pair = Array.isArray(ids) ? ids.map(Number) : [];
    if (pair.length !== 2 || pair.some((id) => !Number.isInteger(id) || id <= 0 || id > 100000) || pair[0] === pair[1]) return "";
    const items = pair.map((id) => {
      const spell = (state.summonerSpells?.spells || []).find((item) => Number(item.id) === id);
      return `<span class="rune-spell-item">${renderSummonerSpellIcon(id)}<small>${escapeHTML(spell?.name || `召唤师技能 ${id}`)}</small></span>`;
    }).join("");
    return `<div class="rune-spell-row"><span>召唤师技能</span><div class="rune-spell-items">${items}</div></div>`;
  }

  function renderRuneSourceSection(section, championSelected) {
    if(section.key==="custom")return renderCustomConfiguration(section.items[0]);
    if (section.key === "pro") {
      const status = section.proStatus || {};
      const unavailableReason = { pro: status.reason === "upstream-timeout" ? "职业赛事上游超时，请稍后重试。" : status.reason === "network-unavailable" ? "无法连接职业赛事数据源，请检查网络或代理后重试。" : status.reason === "no-sample" ? "近 30 天暂无该英雄在当前所选位置的职业选手符文。" : status.reason === "cache-write-failed" ? "职业数据缓存写入失败，请检查本地存储。" : "职业赛事数据暂不可用，请重试。" };
      const preparing = status.preparing || status.reason === "preparing" || section.loading;
      const totalGames = Math.max(0, Number(status.totalGames) || 0);
      const failedGames = Math.min(totalGames, Math.max(0, Number(status.failedGames) || 0));
      const failureRate = totalGames ? failedGames / totalGames : 0;
      // More than 10% is degraded coverage, not proof the entire source is offline.
      const degraded = failureRate > 0.1;
      const stale = status.stale || (status.readAt && Date.now() - Date.parse(status.readAt) > 300000);
      const note = section.items.length && !stale && status.outcomesLoading ? "正在核对近 30 天战绩，未确认胜负不计入统计。" : "";
      const content = section.items.length ? renderSpecialistPlayers(section.items, "pro") : `<div class="recommendation-empty"><strong>${!championSelected ? "请先选定英雄" : preparing ? "正在准备职业选手数据" : "暂无职业选手符文"}</strong><p>${preparing ? "正在后台索引近 30 天官方比赛，不影响其它推荐。" : unavailableReason.pro}</p></div>`;
      return `<section class="rune-source-section">${note ? `<p class="pro-rune-status" role="status">${escapeHTML(note)}</p>` : ""}${content}${!preparing && (stale || degraded || !section.items.length) ? `<button class="text-button" type="button" data-retry-pro-runes${stale && section.items.length ? ' data-tooltip="当前显示上次读取结果，点击重新获取"' : ''}>重试</button>` : ""}</section>`;
    }
    const target = specialistRequestTarget(state.live);
	const specialistKeyMissing = section.key === "specialist" && Boolean(target && specialistRuneFailure(state.specialistRuneFailures.get(target.key))?.reason === "riot-key-missing");
		const specialistFailureInfo = section.key === "specialist" ? specialistRuneFailure(state.specialistRuneFailures.get(target?.key)) : null;
		const specialistKeyInvalid = specialistFailureInfo?.reason === "riot-key-invalid";
		const specialistRelayUnavailable = specialistFailureInfo?.reason === "riot-relay-unavailable";
		const specialistKeyUnavailable = specialistKeyMissing || specialistKeyInvalid;
		const specialistNoPositionSample = specialistFailureInfo?.reason === "no-position-sample";
		const specialistTimeout = specialistFailureInfo?.reason === "upstream-timeout";
		const specialistThrottled = specialistFailureInfo?.reason === "upstream-throttled";
		const specialistUpstreamError = specialistFailureInfo?.reason === "upstream-error";
		const specialistRetryable = specialistRelayUnavailable || specialistTimeout || specialistThrottled || specialistUpstreamError || specialistFailureInfo?.reason === "request-failed";
		const unavailableReason = {
		  specialist: section.unsupported ? "绝活哥榜单仅支持单双排、灵活组排和召唤师峡谷自定义对局。" : specialistKeyMissing ? "未配置 Riot Key，暂时无法读取韩服绝活哥符文。" : specialistNoPositionSample ? `该绝活哥最近 10 局没有打过${positionLabel(livePositionDisplay(target?.position || ""))}。` : specialistTimeout ? "韩服接口响应超时，请稍后重试。" : specialistThrottled ? "请求过于频繁，约 1 分钟后可重试。" : specialistUpstreamError ? "上游数据异常，请稍后重试。" : "最近对局中没有找到完整且可核验的该英雄符文。",
      pro: "当前数据源不提供可核验的职业选手身份与完整符文，暂不展示。",
    };
		const emptyTitle = !championSelected ? "请先选定英雄" : section.unsupported ? "当前队列没有可用的绝活哥榜单" : section.loading ? "正在读取韩服绝活哥符文" : specialistRelayUnavailable ? "战绩服务暂时不可用" : specialistKeyInvalid ? "Riot Key 无效" : specialistKeyMissing ? "未配置 Riot Key" : specialistNoPositionSample ? "最近 10 局没有该位置样本" : specialistTimeout ? "韩服接口响应超时" : specialistThrottled ? "请求过于频繁" : specialistUpstreamError ? "上游数据异常" : section.failed ? "韩服绝活哥符文读取失败" : section.key === "opgg" ? "等待完整符文数据" : section.key === "specialist" ? "暂无可核验的韩服绝活哥符文" : "暂无可核验数据源";
    const content = section.items.length && section.key === "specialist"
      ? renderSpecialistPlayers(section.items)
      : section.items.length
      ? `<div class="rune-choice-list" role="radiogroup" aria-label="${escapeHTML(section.title)}符文">${section.items.map((config, index) => renderRuneChoice({ ...config, sourceKey: section.key, key: config.key || (section.key === "opgg" && index === 0 ? "opgg" : `${section.key}-${index}`) })).join("")}</div>`
	  : `<div class="recommendation-empty"><strong>${emptyTitle}</strong>${specialistKeyUnavailable || specialistRelayUnavailable ? "" : `<p>${section.loading ? "正在核对专家榜玩家最近对局中的完整符文，通常需要 5-15 秒。" : specialistNoPositionSample || specialistRetryable ? unavailableReason.specialist : section.failed ? "本次后台读取未完成，稍后刷新时会自动重试。" : unavailableReason[section.key] || "OPGG 返回完整主系、副系与属性碎片后即可选择。"}</p>`}${specialistKeyUnavailable ? '<button class="text-button" type="button" data-open-riot-settings>去设置</button>' : specialistRetryable ? '<button class="text-button" type="button" data-retry-specialist-runes>重试</button>' : ""}</div>`;
    const opggSpells = section.key === "opgg" && section.items.length ? renderRuneSpellPair(recommendedRuneSpellIDs(state.live, { sourceLabel: "OPGG" })) : "";
    return `<section class="rune-source-section">${opggSpells}${content}</section>`;
  }

  function renderSpecialistPlayers(items, source = "specialist") {
    const groups = [];
    const byPlayer = new Map();
    for (const [index, item] of (items || []).entries()) {
      const key = source === "pro" ? item.title || item.playerName || "职业选手"
        : item.playerName && item.tagLine ? `${item.playerName}#${item.tagLine}` : item.playerName || item.title || "绝活哥";
      if (!byPlayer.has(key)) {
        const games = [];
        byPlayer.set(key, games);
        groups.push([key, games]);
      }
      const group = byPlayer.get(key);
      group.push({ config: item, choiceKey: String(item.key || `specialist-${index}`) });
    }
    const limited = source === "pro" ? groups : groups.slice(0, 3);
    const target = source === "pro" ? proRequestTarget(state.live) : specialistRequestTarget(state.live);
    const tabKey = source === "pro" ? `pro:${target?.key}` : target?.key;
    let activeKey = state.specialistPlayerTabs.get(tabKey) || limited[0]?.[0] || "";
    if (target && activeKey && !limited.some(([key]) => key === activeKey)) {
      activeKey = limited[0]?.[0] || "";
      state.specialistPlayerTabs.set(tabKey, activeKey);
    }
    const tabs = limited.map(([key, games], playerIndex) => {
      const config = games[0]?.config;
      const active = key === activeKey;
      const name = String(config?.playerName || "").trim();
      const tagLine = String(config?.tagLine || "").trim();
      const visibleName=privacyName({gameName:name,tagLine},{context:"recommendation"});
      const canOpen = source === "specialist" && name && tagLine && config?.region === "kr";
      if(state.settings?.maskNames) {
        const choice = `${tabKey}:${playerIndex}`;
        state.specialistOverviewTargets ||= new Map();
        state.specialistOverviewTargets.set(choice,{gameName:name,tagLine});
        const nameButton = `<button type="button" role="tab" class="specialist-player-name" data-specialist-player="${escapeHTML(key)}" data-player-source="${source}" aria-selected="${active}">${escapeHTML(visibleName)}</button>`;
        return canOpen ? `<div class="specialist-player-tab specialist-player-split${active ? " is-active" : ""}">${nameButton}<button type="button" class="specialist-player-overview" data-specialist-overview data-specialist-choice="${escapeHTML(choice)}" aria-label="查看 ${escapeHTML(visibleName)} 的总览">总览</button></div>` : `<div class="specialist-player-tab${active ? " is-active" : ""}">${nameButton}</div>`;
      }
      if (!canOpen) return `<button type="button" role="tab" class="specialist-player-tab${source === "pro" ? " pro-player-tab" : ""}${active ? " is-active" : ""}" data-specialist-player="${escapeHTML(key)}" data-player-source="${source}" aria-selected="${active}">${source === "pro" ? `<span class="pro-player-name">${escapeHTML(config?.title || key)}</span>${proRuneRecordLabel(config)}` : escapeHTML(key)}</button>`;
      return `<div class="specialist-player-tab specialist-player-split${active ? " is-active" : ""}"><button type="button" role="tab" class="specialist-player-name" data-specialist-player="${escapeHTML(key)}" data-player-source="specialist" aria-selected="${active}" aria-label="查看 ${escapeHTML(name)} 的推荐符文">${escapeHTML(name)}</button><button type="button" class="specialist-player-overview" data-specialist-overview data-player-name="${escapeHTML(name)}" data-player-tag="${escapeHTML(tagLine)}" aria-label="查看 ${escapeHTML(name)}#${escapeHTML(tagLine)} 的总览">总览</button></div>`;
    }).join("");
    const panel = limited.find(([key]) => key === activeKey) || limited[0];
    const playerGames = panel?.[1] || [];
    const games = source === "pro" ? [...playerGames].sort((a, b) => Number(b.config.playedAt || 0) - Number(a.config.playedAt || 0)).slice(0, 5) : playerGames;
    const rows = games.map(({ config, choiceKey }) => {
      const selected = String(state.selectedRecommendation || "") === choiceKey;
      const opponentName = config.opponentChampionName || championDisplayName(config.opponentChampionId);
      const position = config.position ? source === "pro" ? config.position : positionLabel(livePositionDisplay(config.position)) : "";
      const tone = config.winKnown === false ? "is-unknown" : config.result === "win" ? "is-win" : config.result === "loss" ? "is-loss" : "is-remake";
      const championID = Number(config.championId || config.championID);
      const championIcon = championID > 0
        ? iconFigure("champion", championID, config.championName || "英雄", "small")
        : '<span class="rune-choice-champion-placeholder" aria-hidden="true"></span>';
      const title = runeConfigurationTitle(config);
      const opponentPlayer = config.opponentPlayerName
        ? privacyName({gameName:config.opponentPlayerName,tagLine:config.opponentTagLine},{championName:config.opponentChampionName,context:"match"})
        : "暂无召唤师信息";
	  const opponentRankTitle = config.opponentTier ? rankTitle({ tier: config.opponentTier, division: config.opponentDivision }) : "";
	  const opponentWinRate = config.opponentWinRate != null && Number.isFinite(Number(config.opponentWinRate))
	    ? `胜率 ${Math.round(Number(config.opponentWinRate))}%`
	    : "";
	  const opponentRankText = [opponentRankTitle, opponentWinRate].filter(Boolean).join(" · ");
	  const opponentRank = opponentRankText
	    ? `<span class="specialist-opponent-rank">${config.opponentTier ? rankCrestIcon(config.opponentTier) : ""}<small>${escapeHTML(opponentRankTitle)}${opponentRankTitle && opponentWinRate ? " · " : ""}${opponentWinRate ? `胜率 <b class="win-rate-value">${Math.round(Number(config.opponentWinRate))}%</b>` : ""}</small></span>`
	    : "";
      const skills = source === "pro" ? renderProSkillOrder(config.build) : "";
      const items = renderRuneEquipment(config, skills);
      const cornerSpells = renderRuneSpellIcons(config);
	      return `<div class="specialist-game-row ${tone}${selected ? " is-selected" : ""}" role="radio" aria-checked="${selected}" tabindex="0" data-rune-choice="${escapeHTML(choiceKey)}"><div class="specialist-game-head"><span class="specialist-game-choice"><span class="radio-mark" aria-hidden="true"></span>${championIcon}<span class="specialist-game-title"><strong>${escapeHTML(title)}</strong><span class="specialist-game-meta">${position ? `<span class="rune-position-tag">${escapeHTML(position)}</span>` : ""}${config.eventLabel ? `<span class="pro-event-label">${escapeHTML(config.eventLabel)}</span>` : ""}${config.winKnown === false ? '<span data-tooltip="该局胜负未获官方确认">胜负未确认</span>' : ""}</span></span></span><span class="specialist-opponent is-${tone.slice(3)}"><span class="specialist-result-dot" aria-hidden="true"></span>${config.opponentChampionId ? iconFigure("champion", config.opponentChampionId, opponentName, "small") : ""}<span class="specialist-opponent-copy"><b>${escapeHTML(opponentName)}</b><small>${escapeHTML(opponentPlayer)}</small></span>${opponentRank}</span></div><div class="specialist-game-content"><div class="specialist-game-runes${skills ? " has-skills" : ""}">${renderUnifiedRuneBoard(config, cornerSpells)}${items}${config.selectedComplete === false ? '<p class="pro-rune-status">属性碎片：上游未提供完整槽位；这套数据尚不完整，暂时不能应用。</p>' : ""}</div></div></div>`;
    }).join("");
    return `<div class="specialist-player-tabs" role="tablist" aria-label="${source === "pro" ? "职业选手" : "绝活哥玩家"}">${tabs}</div><div class="specialist-player-games" role="radiogroup" aria-label="${source === "pro" ? "职业比赛符文" : "绝活哥最近对局符文"}">${rows || '<p class="section-empty">暂无最近对局符文数据</p>'}</div>`;
  }

  function renderRuneSpellIcons(config) {
    const spellIDs = [config.spell1Id, config.spell2Id].map(Number);
    return spellIDs.every(id => Number.isInteger(id) && id > 0 && id <= 100000) && spellIDs[0] !== spellIDs[1]
      ? spellIDs.map(id => renderSummonerSpellIcon(id)).join("") : "";
  }

  function renderProSkillOrder(build) {
    const order = build?.skillOrder;
    if (!Array.isArray(order) || !order.length) return "";
    const priority = window.deepLegendsRuntime.skillPriority(order);
    const summary = priority.length >= 2
      ? `<strong class="skill-priority-summary">主<b class="is-${priority[0].toLowerCase()}">${priority[0]}</b>副<b class="is-${priority[1].toLowerCase()}">${priority[1]}</b></strong>` : "";
    const cells = order.map((key, index) => `<span class="is-${String(key).toLowerCase()}" data-tooltip="第 ${index + 1} 级：升级 ${escapeHTML(key)}" data-tooltip-size="compact"><b>${escapeHTML(key)}</b><small>${index + 1}</small></span>`).join("");
    return `<div class="sg-skills"><span class="sg-label">技能加点</span>${summary}<div class="skill-order" aria-label="技能升级顺序">${cells}</div></div>`;
  }

  function renderRuneEquipment(config, skills = "") {
    const itemIDs = (config.itemIds || config.itemIDs || []).map(Number).filter((id) => Number.isInteger(id) && id > 0).slice(0, 7);
    const icons = renderRuneSpellIcons(config);
    const spells = icons ? `<div class="sg-spells">${icons}</div>` : "";
    const starterIDs = (config.starterItemIds || []).map(Number).filter((id) => Number.isInteger(id) && id > 0).slice(0, 6);
    const starter = starterIDs.length ? `<div class="sg-items sg-starters"><span class="sg-label">出门装</span><div class="sg-icons">${starterIDs.map(id => renderItemIcon(id)).join("")}</div></div>` : "";
    const items = itemIDs.length ? `<div class="sg-items"><span class="sg-label">最终装备</span><div class="sg-icons">${itemIDs.map((id) => renderItemIcon(id)).join("")}</div></div>` : "";
    return starter || items || spells || skills ? `<div class="specialist-game-footer">${starter}${items}${spells}${skills}</div>` : "";
  }

  function proRuneRecordLabel(config) {
    const games = config?.recordGames, wins = config?.recordWins;
    if (!Number.isInteger(games) || !Number.isInteger(wins) || games <= 0 || wins < 0 || wins > games) return '<small class="pro-player-record">胜负待确认</small>';
    const losses = games - wins;
    return `<span class="pro-player-record" aria-label="${wins}胜${losses}负${config?.recordPartial ? '，仅已确认场次' : ''}"><b class="is-win">${wins}</b><span aria-hidden="true">-</span><b class="is-loss">${losses}</b></span>`;
  }

  function runeConfigurationTitle(config) {
    const selectedIDs = (config.perkIds || config.selectedPerkIds || []).map(Number);
    const selected = new Set(selectedIDs);
    const styles = state.perks?.styles || [];
    const primaryStyle = styles.find((style) => Number(style.id) === Number(config.primaryStyleId));
    const secondaryStyle = styles.find((style) => Number(style.id) === Number(config.subStyleId));
    const keystone = (primaryStyle?.slots?.[0]?.perks || []).find((perk) => selected.has(Number(perk.id)));
    if (keystone?.name && secondaryStyle?.name) return `${keystone.name} + ${secondaryStyle.name}`;

    const fallback = String(config.title || config.name || "推荐符文").trim();
    const [leftSide, rightSide] = fallback.split("+").map((part) => part.trim());
    const keystoneName = leftSide?.split("·")[0]?.trim();
    const secondaryName = rightSide || (fallback.includes("·") ? fallback.split("·").at(-1).trim() : "");
    return keystoneName && secondaryName ? `${keystoneName} + ${secondaryName}` : fallback;
  }

  function renderCustomConfiguration(...args){return window.deepLegendsAutoConfigLive?.renderCustomConfiguration(...args);}
  function updateLiveRuneSelection(...args){return window.deepLegendsAutoConfigLive?.updateLiveRuneSelection(...args);}
  function applyCustomConfiguration(...args){return window.deepLegendsAutoConfigLive?.applyCustomConfiguration(...args);}

  function renderRuneChoice(config) {
    const key = String(config.key || "");
    const selected = state.selectedRecommendation === key;
    const stats = config.stats || config;
    const games = config.championGames ?? stats.games ?? stats.play;
    const winRate = stats.winRate ?? stats.win_rate ?? (stats.win != null && games ? stats.win * 100 / games : null);
    const pickRate = stats.pickRate ?? stats.pick_rate;
    const specialistStats = `<dl class="rune-player-stats"><div><dt>胜率</dt><dd class="win-rate-value">${rate(winRate)}</dd></div><div><dt>场次</dt><dd>${compactNumber(games)}</dd></div><div><dt>选用率</dt><dd>${rate(pickRate)}</dd></div></dl>`;
    const riotID = privacyName({gameName:config.playerName,tagLine:config.tagLine},{context:"recommendation"});
    const specialistRank = config.tier ? rankTitle({ tier: config.tier, division: config.division }) : "";
    const played = relativeTime(config.playedAt);
    const result = config.result === "win" ? "胜利" : config.result === "loss" ? "失败" : config.result === "remake" ? "重开" : "";
    const sourceParts = config.sourceKey === "specialist" ? [riotID && `来自 ${riotID}`, specialistRank, played && `${played}的${config.championName || "该英雄"}`, config.region === "kr" ? "韩服" : "", result].filter(Boolean) : [];
    const title = runeConfigurationTitle(config);
    const championID = Number(config.championId || config.championID);
    const championIcon = championID > 0
      ? iconFigure("champion", championID, config.championName || "英雄", "small")
      : '<span class="rune-choice-champion-placeholder" aria-hidden="true"></span>';
    const choiceCopy = `<span class="rune-choice-copy"><strong>${escapeHTML(title)}</strong>${sourceParts.length ? `<small>${escapeHTML(sourceParts.join(" · "))}</small>` : ""}</span>`;
    return `<article class="rune-choice-card${selected ? " is-selected" : ""}" role="radio" aria-checked="${selected}" tabindex="0" data-rune-choice="${escapeHTML(key)}"><div class="rune-choice-selector"><span class="radio-mark" aria-hidden="true"></span>${championIcon}${choiceCopy}${specialistStats}</div><div class="rune-choice-detail">${renderUnifiedRuneBoard(config)}${renderRuneSpellPair([config.spell1Id, config.spell2Id])}</div></article>`;
  }

  function renderUnifiedRuneBoard(config, cornerSpells = "") {
    const selectedIDs = (config.perkIds || config.selectedPerkIds || []).map(Number);
    const selected = new Set(selectedIDs);
    const corner = cornerSpells ? `<div class="rune-corner-spells">${cornerSpells}</div>` : "";
    if (!state.perks) {
      const loading = `<div class="selected-rune-strip">${[...selected].map((id) => iconFigure("perk", id, `符文 ${id}`, "rune")).join("")}</div><p class="muted rune-loading-copy">正在读取完整符文树…</p>`;
      return corner ? `<div class="unified-rune-board"><div>${loading}</div><section class="unified-rune-column rune-shards" aria-label="属性碎片">${corner}</section></div>` : loading;
    }
    const styles = state.perks.styles || [];
    const renderStyle = (styleID, secondary) => {
      const style = styles.find((item) => Number(item.id) === Number(styleID));
      if (!style) return "";
      const slots = secondary ? (style.slots || []).slice(1) : (style.slots || []);
      return `<section class="unified-rune-column${secondary ? " is-secondary" : ""}" aria-label="${escapeHTML(style.name)}">${secondary ? '<div class="unified-rune-row is-spacer" aria-hidden="true"></div>' : ""}<header aria-hidden="true">${renderRuneStyleIcon(style)}</header>${slots.map((slot) => `<div class="unified-rune-row">${(slot.perks || []).map((perk) => renderRuneOption(perk, selected.has(Number(perk.id)))).join("")}</div>`).join("")}</section>`;
    };
    const fragments = state.perks.statModSlots || [];
    const fragmentSelections = (config.statModIds || config.stat_mod_ids || (config.selectedComplete === false ? [] : selectedIDs.length >= 9 ? selectedIDs.slice(-3) : [])).map(Number);
    // Only show unresolved shard types below the board; source-constrained
    // known slots are already highlighted in their actual rows.
    const knownShards = config.selectedComplete === false ? [...new Map(fragments.flatMap(slot => slot.perks || []).filter(perk => selected.has(Number(perk.id)) && !fragmentSelections.includes(Number(perk.id))).map(perk => [Number(perk.id), perk])).values()] : [];
    const incomplete = knownShards.length ? `<div class="pro-known-shards" aria-label="已取得的属性碎片，槽位未确认"><span>已取得的碎片（槽位未确认）</span>${knownShards.map(perk => renderRuneOption(perk, true)).join("")}</div>` : "";
    return `<div class="unified-rune-board">${renderStyle(config.primaryStyleId, false)}${renderStyle(config.subStyleId, true)}<section class="unified-rune-column rune-shards" aria-label="属性碎片">${corner}<header aria-hidden="true"></header><div class="unified-rune-row is-spacer" aria-hidden="true"></div>${fragments.map((slot, rowIndex) => `<div class="unified-rune-row">${(slot.perks || []).map((perk) => renderRuneOption(perk, fragmentSelections[rowIndex] === Number(perk.id))).join("")}</div>`).join("")}</section></div>${incomplete}`;
  }

  function renderRuneOption(perk, selected, fallbackID = 0) {
    const id = Number(perk?.id || fallbackID);
    const name = perk?.name || `符文 ${id}`;
    const explanation = runeShardDescription(id) || plainText(perk?.longDesc || perk?.shortDesc || name);
    const shardPath = dataDragonRuneShardPath(id);
    const icon = shardPath ? remoteStaticIcon("ddragon", shardPath, name, "rune", false) : perk?.iconPath ? assetIcon(perk.iconPath, name, "rune", false) : iconFigure("perk", id, name, "rune", false);
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
	    return `<span class="rune-option-button${selected ? " is-selected" : ""}" role="img" tabindex="-1" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-perk-id="${id}" data-tooltip="${escapeHTML(tooltip)}">${icon}</span>`;
  }

  function renderRuneStyleIcon(style) {
    const names = { 8000: "precision", 8100: "domination", 8200: "sorcery", 8300: "inspiration", 8400: "resolve" };
    const file = names[Number(style?.id)];
    if (!file) return assetIcon(style?.iconPath, style?.name || "符文系", "rune-style");
    const label = style?.name || "符文系";
    return `<span class="game-icon is-rune-style" data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact"><img src="/rune-styles/${file}.svg" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`;
  }

  function dataDragonRuneShardPath(id) {
    const name = ({ 5001: "StatModsHealthPlusIcon.png", 5005: "StatModsAttackSpeedIcon.png", 5007: "StatModsCDRScalingIcon.png", 5008: "StatModsAdaptiveForceIcon.png", 5010: "StatModsMovementSpeedIcon.png", 5011: "StatModsHealthScalingIcon.png", 5013: "StatModsTenacityIcon.png" })[Number(id)];
    return name ? `/cdn/img/perk-images/StatMods/${name}` : "";
  }

  function runeShardDescription(id) {
    return ({
      5001: "获得10至180额外生命值（基于等级）。",
      5005: "获得10%攻击速度。",
      5007: "获得8技能急速。",
      5008: "获得9适应之力（5.4攻击力或9法术强度）。",
      5010: "获得2.5%移动速度。",
      5011: "获得65生命值。",
      5013: "获得10%韧性和减速抗性。",
    })[Number(id)] || "";
  }

  function renderRecommendationStats(stats) {
    if (!stats || !Object.keys(stats).length) return "";
    return renderOptionStats(stats);
  }

  function renderArenaBuildOption(option, kind) {
    const ids = (option.ids || []).map(Number).filter((id) => Number.isInteger(id) && id > 0);
    const route = ids.map((id, index) => `${index ? '<span class="route-arrow" aria-hidden="true">›</span>' : ""}${renderItemIcon(id)}`).join("");
    const item = ids.length === 1 ? (state.items?.items || []).find((candidate) => Number(candidate.id) === ids[0]) : null;
    const name = ids.length === 1 ? item?.name || `装备 ${ids[0]}` : "";
    const badge = gradeBadge(option.grade, "live-build-grade");
    const stats = option.stats ? { ...option.stats, gamesUnavailable: option.gamesUnavailable ?? option.stats.gamesUnavailable } : option;
    const games = stats.games ?? stats.play;
    const derivedWinRate = stats.win != null && games ? stats.win * 100 / games : null;
    const arenaPercent = (value) => value === null || value === undefined || String(value).trim() === "" || !Number.isFinite(Number(value)) ? "—" : `${Number(value).toFixed(2)}%`;
    const averagePlacement = Number(stats.averagePlacement);
    const placement = Number.isFinite(averagePlacement) && averagePlacement > 0 ? averagePlacement.toFixed(2).replace(/0$/, "") : "—";
    const metrics = [
      ["胜率", arenaPercent(stats.winRate ?? derivedWinRate), "is-win"],
      ["平均名次", placement, "is-placement"],
      ["吃鸡率", arenaPercent(stats.firstPlaceRate), "is-first"],
    ];
    const cells = metrics.map(([label, value, tone]) => `<div class="${tone}"><dt>${label}</dt><dd>${value}</dd></div>`).join("");
    return `<article class="arena-option-card live-arena-build-option${kind === "core" ? " arena-core-option" : ""}" data-metric-count="3"><div class="arena-option-main"><div class="arena-option-icons">${route}</div>${badge}${name ? `<strong>${escapeHTML(name)}</strong>` : ""}</div><dl>${cells}</dl></article>`;
  }

  function renderBuildRecommendation(build, self, abilities, data) {
    if (!build) return recommendationEmptyPanel(liveRecommendationChampionId(self) || Number(data?.currentChampionId) ? "等待推荐出装与技能数据" : "请先选定英雄", "仅展示召唤师技能、技能加点与真实装备分布。");
    // The backend orders rows only after applying its minimum-games guard.
    // Keep that order here so tiny samples can never jump the queue locally.
    const inUpstreamOrder = (options, limit = options.length) => [...options].slice(0, limit);
    const spellOptions = inUpstreamOrder(build.spellOptions || (build.spells ? [{ ids: build.spells, stats: build.spellStats || {} }] : []), 2);
    const starterOptions = inUpstreamOrder(build.starterOptions || (build.starterItems ? [{ ids: build.starterItems, stats: build.starterStats || {} }] : []), 3);
    const bootOptions = inUpstreamOrder(build.bootOptions || (build.boots ? [{ ids: build.boots, stats: build.bootStats || {} }] : []), 3);
    const coreOptions = inUpstreamOrder(build.coreOptions || [], LIVE_CORE_OPTION_LIMIT);
    const prismOptions = inUpstreamOrder(build.prismOptions || [], 10);
    const fourthOptions = inUpstreamOrder(build.fourthOptions || [], 5);
    const fifthOptions = inUpstreamOrder(build.fifthOptions || [], 5);
    const sixthOptions = inUpstreamOrder(build.sixthOptions || [], 5);
    const upstreamSampleOrder = [...fourthOptions, ...fifthOptions, ...sixthOptions].some((option) => option?.gamesUnavailable === true)
      ? `<p class="build-order-note" role="note">上游未提供样本量，按上游推荐顺序展示</p>`
      : "";
    const optionList = (options, kind, emptyCopy, statsRenderer = null) => options.length
      ? `<div class="config-option-list">${options.map((option) => renderConfigOption(option, kind, statsRenderer)).join("")}</div>`
      : `<p class="build-group-empty">${escapeHTML(emptyCopy)}</p>`;
    const capabilities = recommendationCapabilities(data, liveRecommendationsFor(data) || {});
	    const hasOpeningConfiguration = spellOptions.length > 0 || starterOptions.length > 0;
	    const summaryLayout = capabilities.hasAugments ? (hasOpeningConfiguration ? "is-mayhem" : "is-arena") : "is-standard";
	    const buildLayout = capabilities.hasAugments ? (hasOpeningConfiguration ? "is-mayhem-build" : "is-arena-build") : "is-standard-build";
    const augmentSource = liveAugmentRecommendationSource(data);
    const isArenaBuild = augmentSource === "arena";
    const isMayhemBuild = augmentSource === "hextech";
	    const summary = [
	      starterOptions.length ? `<section class="build-summary-starter"><h3>出门装</h3>${optionList(starterOptions, "item", "该模式没有出门装样本")}</section>` : "",
	      spellOptions.length ? `<section class="build-summary-spells"><h3>召唤师技能</h3>${optionList(spellOptions, "spell", "该模式不使用召唤师技能")}</section>` : "",
	      `<section class="build-summary-boots"><h3>鞋子</h3>${optionList(bootOptions, "item", "暂无鞋子样本")}</section>`,
	      build.skillPriority?.length || build.skillOrder?.length ? `<section class="skill-plan build-summary-skill"><div class="skill-plan-head"><h3>技能加点</h3></div>${renderSkillPlan(build, abilities, renderRecommendationStats(build.skillStats))}</section>` : "",
	    ].filter(Boolean).join("");
    const itemSet = buildItemSetPayload(build, self, data);
	    const canApply = itemSet.blocks.length > 0 && data?.phase === "ChampSelect";
	// R116-B P0-3-2：这个 `capabilities.hasAugments ? "" : <footer>` 是有意设计，
	// 海斗/斗魂的海克斯局不支持写入客户端装备方案。
	    const action = capabilities.hasAugments ? "" : `<footer class="recommendation-action item-set-action"><div><strong>游戏装备方案</strong><span>${itemSet.blocks.length ? `${itemSet.blocks.length} 个推荐分组` : "当前没有可应用的装备推荐"}</span></div><button class="button button-primary apply-item-set" type="button" data-apply-item-set ${canApply ? "" : "disabled"}>${data?.phase === "ChampSelect" ? "应用装备方案" : "仅英雄选择阶段可应用"}</button></footer>`;
    const chainStatus = String(build.itemChainStatus || "");
    const hasChainStatus = chainStatus === "ready" || chainStatus === "unavailable";
    const depthGroups = capabilities.hasItemDepths ? [["第四件", fourthOptions, Number(build.fourthSample) || 0], ["第五件", fifthOptions, Number(build.fifthSample) || 0], ["第六件", sixthOptions, Number(build.sixthSample) || 0]]
      // 第六件没有样本就整列不展示（辅助位普遍撑不到第六件，op.gg 本来就不给）；
      // 第四/第五件保留空态，区分上游没有样本与读取失败。
      .filter(([label, options]) => (label === "第六件" ? options.length > 0 : hasChainStatus || options.length))
      .map(([label, options, sample]) => {
        const empty = chainStatus === "unavailable" ? `${label}推荐暂不可用，稍后重试` : "该阶段暂无可用样本";
        return `<section><h4><span>${label}</span></h4>${optionList(options, "item", empty, renderDepthStats)}</section>`;
      }) : [];
    const arenaOptionList = (options, kind, emptyCopy) => options.length
      ? `<div class="live-arena-build-grid">${options.map((option) => renderArenaBuildOption(option, kind)).join("")}</div>`
      : `<p class="build-group-empty">${escapeHTML(emptyCopy)}</p>`;
    const itemBuildLayout = isArenaBuild
      ? `<div class="item-build-layout"><section class="live-arena-build-section is-core"><h3>核心装</h3>${arenaOptionList(coreOptions, "core", "暂无核心装备样本")}</section></div>`
      : `<div class="item-build-layout"><div class="build-item-row" data-depth-count="${depthGroups.length}"><section class="item-core-column"><h3>核心装</h3>${optionList(coreOptions, "route", "暂无核心装备样本", renderCoreStats)}</section>${depthGroups.length ? `<div class="item-depth-columns">${depthGroups.join("")}</div>` : ""}</div></div>`;
    const lateBands = [
      prismOptions.length ? isArenaBuild
        ? `<section class="live-arena-build-section is-prismatic"><h3>棱彩装备</h3>${arenaOptionList(prismOptions, "prism", "暂无棱彩装备样本")}</section>`
        : `<section class="late-option-band is-prismatic"><h3>棱彩装备</h3>${optionList(prismOptions, "item", "暂无棱彩装备样本")}</section>` : "",
    ].filter(Boolean).join("");
    const itemRanking = liveRecommendationsFor(data)?.itemRanking || [];
	    const itemRankingContent = itemRanking.length ? renderLiveItemRanking(itemRanking) : "";
    const rankingLayoutClass = isMayhemBuild ? " is-mayhem-build-row" : "";
    return `<section class="build-recommendation ${buildLayout}"><header class="build-recommendation-heading"><h3>装备推荐</h3>${upstreamSampleOrder}</header><div class="build-summary-bar ${summaryLayout}">${summary}</div>${lateBands}<div class="build-core-ranking-row${rankingLayoutClass}">${itemBuildLayout}${itemRankingContent}</div></section>${action}`;
  }

  function renderLiveItemRanking(rows) {
    const sorted = [...(rows || [])].sort((left, right) => (Number(right.score) || 0) - (Number(left.score) || 0) || (Number(right.games) || 0) - (Number(left.games) || 0));
    const content = sorted.slice(0, 8).map((row, index) => {
      const asset = row.assets?.[0] || {};
      const itemID = Number(asset.id) || 0;
      const icon = itemID > 0 ? renderItemIcon(itemID) : '<span class="mayhem-item-name-only" aria-hidden="true"></span>';
      return `<article><b>${index + 1}</b><span class="live-ranking-icon">${icon}</span><strong>${escapeHTML(asset.name || "未知装备")}</strong><dl><div><dt>胜率</dt><dd class="metric-win">${rate(row.winRate)}</dd></div><div><dt>场次</dt><dd>${compactNumber(row.games)}</dd></div></dl></article>`;
    }).join("");
    return `<section class="recommendation-section mayhem-item-ranking live-item-ranking"><header><h3>装备排行</h3><span class="section-count">${sorted.length} 件</span></header><div class="mayhem-ranking-list">${content}</div></section>`;
  }

  function buildItemSetPayload(build, self, data) {
    const starterOptions = build?.starterOptions || (build?.starterItems ? [{ ids: build.starterItems }] : []);
    const bootOptions = build?.bootOptions || (build?.boots ? [{ ids: build.boots }] : []);
	const coreOptions = (build?.coreOptions || []).slice(0, 3);
    const depthOptions = [...(build?.fourthOptions || []), ...(build?.fifthOptions || []), ...(build?.sixthOptions || [])];
    const prismOptions = build?.prismOptions || [];
	    const items = (ids) => [...new Set((ids || []).map(Number).filter((id) => Number.isInteger(id) && id > 0))].slice(0, 20).map((id) => ({ id, count: 1 }));
	    const mergedItems = (options) => items((options || []).flatMap((option) => option.ids || []));
	    const blocks = [];
	    const starters = mergedItems(starterOptions);
	    if (starters.length) blocks.push({ type: "出门装", items: starters });
	    const boots = mergedItems(bootOptions);
	    if (boots.length) blocks.push({ type: "鞋子选择", items: boots });
	    const core = mergedItems(coreOptions);
	    if (core.length) blocks.push({ type: "核心装", items: core });
	    const coreIDs = new Set(core.map(({ id }) => id));
	    const depth = mergedItems(depthOptions).filter(({ id }) => !coreIDs.has(id));
	    if (depth.length) blocks.push({ type: "后续装备", items: depth });
	    const prism = mergedItems(prismOptions);
	    if (prism.length) blocks.push({ type: "棱彩装备", items: prism });
      const recommendations = liveRecommendationsFor(data) || {};
      const target = liveRecommendationTarget(data);
	    const recommendationPosition = String(recommendations.resolvedPosition || build?.position || target?.position || self?.position || "other").trim().toLowerCase();
	    const title = recommendationPosition && recommendationPosition !== "other" ? positionLabel(recommendationPosition) : "通用";
	    return {
        title, championId: liveRecommendationChampionId(self) || Number(data?.currentChampionId) || 0, mapId: Number(data?.mapId || 0),
        position: recommendationPosition || "other", selfPosition: self?.position || "other", blocks: blocks.slice(0, 20),
        traceId: recommendations.traceId || state.liveRecommendationTraces.get(target?.key) || "",
        recommendationKey: recommendations.recommendationKey || target?.key || "",
        requestedPosition: recommendations.requestedPosition || target?.position || "",
        resolvedPosition: recommendations.resolvedPosition || build?.position || "",
        positionSource: recommendations.positionSource || "", gameId: Number(data?.gameId || 0),
        queueId: Number(data?.queueId || 0), gameMode: String(data?.gameMode || ""), tier: target?.tier || "",
      };
  }

  function renderSkillPlan(build, abilities, statsHTML = "") {
    const bySlot = new Map((abilities || []).map((ability) => [String(ability.slot || "").toUpperCase(), ability]));
    const skillPriority = (build.skillPriority?.length ? build.skillPriority : window.deepLegendsRuntime.skillPriority(build.skillOrder || [])).map((slot) => String(slot).toUpperCase());
    const priority = ["Q", "W", "E", "R"].map((key) => {
      const ability = bySlot.get(key) || { slot: key, name: key, description: `${key} 技能` };
      const icon = ability.iconPath ? assetIcon(ability.iconPath, ability.name || key, "large", false) : `<span class="skill-letter">${escapeHTML(key)}</span>`;
      const rank = skillPriority.indexOf(key);
      const tooltip = abilityTooltip(key, ability);
      const emphasis = key === "R" ? " is-ultimate" : rank === 0 ? " is-priority" : rank === 1 ? " is-secondary" : "";
      return `<button class="skill-icon-button${emphasis}" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${icon}<span class="skill-copy"><b class="skill-slot">${key}</b><span class="skill-name">${escapeHTML(ability.name || `${key} 技能`)}</span></span></button>`;
    }).join("");
    const mainSkills = [...new Set(skillPriority.filter(key => ["Q", "W", "E"].includes(key)))];
    const summary = mainSkills.length >= 2 ? `<strong class="skill-priority-summary">主${mainSkills[0]}副${mainSkills[1]}</strong>` : "";
    const order = build.skillOrder || [];
    const orderGrid = order.length ? `<div class="skill-order" data-skill-count="${Math.max(1, order.length)}" aria-label="技能升级顺序">${order.map((key, index) => `<span class="is-${String(key).toLowerCase()}"><b>${escapeHTML(key)}</b><small>${index + 1}</small></span>`).join("")}</div>` : "";
    // 选取率/胜率/场次与技能图标同一行，垂直居中对齐（而不是贴在标题那一行）。
    return `<div class="skill-priority-row"><div class="skill-priority">${priority}${summary}</div>${statsHTML}</div>${orderGrid}`;
  }

  function abilityTooltip(key, ability) {
    const lines = [`${key} · ${ability.name || `${key} 技能`}`];
    const description = plainText(ability.description || ability.tooltip || "");
    if (description && description !== key) lines.push(description);
    const costs = abilityRankValues(ability.costs);
    const cooldowns = abilityRankValues(ability.cooldowns);
    const ranges = abilityRankValues(ability.ranges);
    if (costs) lines.push(`消耗：${costs}`);
    if (cooldowns) lines.push(`冷却：${cooldowns} 秒`);
    if (ranges) lines.push(`施法距离：${ranges}`);
    return lines.join("\n");
  }

  function abilityRankValues(values) {
    const normalized = (values || []).map(Number).filter((value) => Number.isFinite(value) && value > 0);
    while (normalized.length > 5 && normalized.at(-1) === normalized.at(-2)) normalized.pop();
    const limited = normalized.slice(0, 5);
    if (!limited.length) return "";
    return limited.every((value) => value === limited[0]) ? number(limited[0]) : limited.map((value) => number(value)).join(" / ");
  }

  function renderConfigOption(option, kind, statsRenderer = null) {
    const ids = option.ids || [];
    const icons = ids.map((id, index) => {
      const icon = kind === "spell" ? renderSummonerSpellIcon(id) : renderItemIcon(id);
      const item = `<span class="config-item">${icon}</span>`;
      return kind === "route" && ids.length > 1 ? `<span class="route-step">${index ? '<span class="route-arrow" aria-hidden="true">›</span>' : ""}${item}</span>` : item;
    }).join("");
    const stats = option.stats ? { ...option.stats, gamesUnavailable: option.gamesUnavailable ?? option.stats.gamesUnavailable } : option;
    const visualKind = kind === "route" && ids.length <= 1 ? "item" : kind;
    return `<div class="config-option is-${visualKind}"><div class="config-icons" data-icon-count="${ids.length}">${icons}</div>${statsRenderer ? statsRenderer(stats) : renderOptionStats(stats)}</div>`;
  }

  function renderOptionStats(stats) {
    const games = stats.games ?? stats.play;
    const derivedWinRate = stats.win != null && games ? stats.win * 100 / games : null;
    const pickRate = stats.pickRate ?? stats.pick_rate;
    const winRate = stats.winRate ?? derivedWinRate;
    const hasPick = Number.isFinite(Number(pickRate)) && Number(pickRate) > 0;
    const hasWin = Number.isFinite(Number(winRate)) && Number(winRate) > 0;
    if (!hasPick && !hasWin) return "";
    return `<dl class="option-stats"><div class="is-pick"><dt>选取率</dt><dd>${rate(pickRate)}</dd></div><div class="is-win"><dt>胜率</dt><dd class="win-rate-value">${rate(winRate)}</dd></div><div class="is-games"><dt>场次</dt><dd>${compactNumber(games)}</dd></div></dl>`;
  }

  // 核心装是三件套路线，一行要放下 3 个图标 + 2 个箭头。去掉选取率只留胜率与场次，
  // 把宽度让给图标，避免图标被压到最小值。
  function renderCoreStats(stats) {
    const games = stats?.games ?? stats?.play;
    const derivedWinRate = stats?.win != null && games ? stats.win * 100 / games : null;
    const pickRate = stats?.pickRate ?? stats?.pick_rate;
    const winRate = stats?.winRate ?? derivedWinRate;
    const hasPick = Number.isFinite(Number(pickRate)) && Number(pickRate) > 0;
    const hasWin = Number.isFinite(Number(winRate)) && Number(winRate) > 0;
    if (!hasPick && !hasWin) return "";
    return `<dl class="option-stats is-depth"><div class="is-win"><dt>胜率</dt><dd class="win-rate-value">${rate(winRate)}</dd></div><div class="is-games"><dt>场次</dt><dd>${compactNumber(games)}</dd></div></dl>`;
  }

  function renderDepthStats(stats) {
    const games = stats?.games ?? stats?.play;
    const gamesUnavailable = stats?.gamesUnavailable === true;
    const derivedWinRate = stats?.win != null && games ? stats.win * 100 / games : null;
    const winRate = stats?.winRate ?? derivedWinRate;
    const hasWin = Number.isFinite(Number(winRate)) && Number(winRate) > 0;
    if (!hasWin && !gamesUnavailable && !(Number(games) > 0)) return "";
    return `<dl class="option-stats is-depth"><div class="is-win"><dt>胜率</dt><dd class="win-rate-value">${rate(winRate)}</dd></div><div class="is-games"><dt>场次</dt><dd>${gamesUnavailable ? "未提供" : compactNumber(games)}</dd></div></dl>`;
  }

  function renderItemIcon(id) {
    const item = (state.items?.items || []).find((candidate) => Number(candidate.id) === Number(id));
    const name = item?.name || `装备 ${id}`;
    const explanation = plainText(item?.description || name);
    const icon = item?.iconPath ? assetIcon(item.iconPath, name, "large", false) : iconFigure("item", id, name, "large", false);
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
	    return `<span class="item-option-button" role="img" tabindex="-1" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-tooltip="${escapeHTML(tooltip)}">${icon}</span>`;
  }

  function renderSummonerSpellIcon(id) {
    const spell = (state.summonerSpells?.spells || []).find((candidate) => Number(candidate.id) === Number(id));
    const name = spell?.name || `召唤师技能 ${id}`;
    const explanation = plainText(spell?.description || name);
    const icon = spell?.iconPath ? assetIcon(spell.iconPath, name, "large", false) : iconFigure("spell", id, name, "large", false);
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
	    return `<span class="item-option-button" role="img" tabindex="-1" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-tooltip="${escapeHTML(tooltip)}">${icon}</span>`;
  }

  function selectedRuneRecommendation(data) {
    const custom = window.deepLegendsAutoConfigLive?.prepare(data);
    const runes = liveRecommendationsFor(data)?.runes || {};
    const opggItems = Array.isArray(runes.opgg) ? runes.opgg : runes.opgg ? [runes.opgg] : [];
    const opggFallback = opggItems.length
      ? opggItems.map((item) => ({ ...item, sourceLabel: "OPGG" }))
      : data?.clientRecommendation ? [{ ...data.clientRecommendation, title: "客户端内置（备用）", sourceLabel: "客户端内置" }] : [];
    const normalized = (custom ? [] : opggFallback).map((item, index) => ({ ...item, key: item.key || (index === 0 ? "opgg" : `opgg-${index}`) }));
    if (recommendationQueueHasTopPlayers(data)) {
      normalized.push(
        ...specialistRunesFor(data).map((item, index) => ({ ...item, sourceLabel: "绝活哥", key: item.key || `specialist-${index}` })),

      );
    }
    normalized.push(...proRunesFor(data).map((item, index) => ({ ...item, sourceLabel: "职业选手", key: item.key || `pro-${index}` })));
    if(custom)normalized.unshift(custom);
    const label={custom:"自配置",opgg:"OPGG",pro:"职业选手",specialist:"绝活哥"}[state.runeSourceTab];
    const visible=normalized.filter(item=>item.sourceLabel===label || state.runeSourceTab==="opgg"&&item.sourceLabel==="客户端内置");
    const choices=label?visible:normalized;
    const selected = choices.find((item) => String(item.key) === String(state.selectedRecommendation)) || choices[0] || null;
    if (selected && String(selected.key) !== String(state.selectedRecommendation)) state.selectedRecommendation = String(selected.key);
    return selected;
  }

  function plainText(value) {
    return String(value || "")
      .replace(/<br\s*\/?>|<\/(?:p|div|li|mainText|stats)>/gi, "\n")
      .replace(/<[^>]*>/g, " ")
      .replace(/&(nbsp|ensp|emsp);/gi, " ")
      .replace(/&amp;/gi, "&")
      .replace(/&lt;/gi, "<")
      .replace(/&gt;/gi, ">")
      .replace(/&quot;/gi, '"')
      .replace(/[\t ]+/g, " ")
      .replace(/ *\n */g, "\n")
      .replace(/\n{3,}/g, "\n\n")
      .trim();
  }

  function rate(value) {
    if (value === null || value === undefined || String(value).trim() === "") return "—";
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) return "—";
    return `${parsed.toFixed(1).replace(/\.0$/, "")}%`;
  }

  function rememberLiveRecommendationMarkup() {
    if (state.live?.available) nodes.liveContent._recommendationMarkup = liveRecommendationMarkup(state.live);
  }

  function activateLiveRecommendationTab(key, focus = true) {
    const button = nodes.liveContent.querySelector(`[data-recommendation-tab="${CSS.escape(key)}"]`);
    const panel = nodes.liveContent.querySelector(`#recommendation-panel-${CSS.escape(key)}`);
    if (!button || !panel) {
      renderLive("tabs");
      return;
    }
    for (const tab of nodes.liveContent.querySelectorAll("[data-recommendation-tab]")) {
      const active = tab === button;
      tab.classList.toggle("is-active", active);
      tab.setAttribute("aria-selected", String(active));
      tab.tabIndex = active ? 0 : -1;
    }
    for (const candidate of nodes.liveContent.querySelectorAll(".recommendation-panel")) candidate.hidden = candidate !== panel;
    window.reportFlowDiagnostic?.("live_ui_switch", "recommendation-tab", { tab: key, partial: true });
    rememberLiveRecommendationMarkup();
    if (focus) button.focus({ preventScroll: true });
  }

  function refreshLiveRuneWorkspace(focusSelector = "") {
    const panel = nodes.liveContent.querySelector("#recommendation-panel-runes");
    const currentTabs = panel?.querySelector(".rune-source-tabs");
    const currentStack = panel?.querySelector(".rune-source-stack");
    if (!panel || !currentTabs || !currentStack || !state.live) {
      renderLive("rune");
      return;
    }
    const next = document.createElement("div");
    next.innerHTML = renderRuneRecommendations(state.live, Boolean(liveRecommendationTarget(state.live)));
    const nextTabs = next.querySelector(".rune-source-tabs");
    const nextStack = next.querySelector(".rune-source-stack");
    if (!nextTabs || !nextStack) {
      renderLive("rune");
      return;
    }
    currentTabs.replaceWith(nextTabs);
    currentStack.replaceWith(nextStack);
    window.reportFlowDiagnostic?.("live_ui_switch", "rune-workspace", { source: state.runeSourceTab, partial: true });
    bindRuneWorkspaceControls(panel);
    bindRuneChoiceButtons(panel);
    updateLiveRuneSelection(panel);
    applyRenderedMetricStyles(panel);
    prepareImages(nextStack);
    rememberLiveRecommendationMarkup();
    if (focusSelector) panel.querySelector(focusSelector)?.focus({ preventScroll: true });
  }

  function bindLiveNode(node, event, role, listener) {
    if (!node) return;
    const bindings = node._liveBindings || (node._liveBindings = new Set());
    const key = `${event}:${role}`;
    if (bindings.has(key)) return;
    bindings.add(key);
    node.addEventListener(event, listener);
  }

  function bindRuneWorkspaceControls(root) {
    for (const button of root?.querySelectorAll("[data-rune-source]") || []) bindLiveNode(button, "click", "rune-source", () => {
      const source = button.dataset.runeSource;
      if (!["custom", "opgg", "specialist", "pro"].includes(source)) return;
      clearRuneStarterRetries();
      state.runeSourceTab = source;
      refreshLiveRuneWorkspace(`[data-rune-source="${CSS.escape(source)}"]`);
    });
    for (const button of root?.querySelectorAll("[data-specialist-overview]") || []) bindLiveNode(button, "click", "specialist-overview", () => {
      const target = state.specialistOverviewTargets?.get(button.dataset.specialistChoice);
      const gameName = target?.gameName || button.dataset.playerName;
      const tagLine = target?.tagLine || button.dataset.playerTag;
      if (!gameName || !tagLine) return;
      window.dispatchEvent(new CustomEvent("deep-legends:open-player", {
        // 绝活哥数据源固定选取韩服榜单。
        detail: { gameName, tagLine, region: "kr", source: "live-specialist" },
      }));
    });
    for (const button of root?.querySelectorAll("[data-specialist-player]") || []) bindLiveNode(button, "click", "specialist-player", () => {
      const pro = button.dataset.playerSource === "pro";
      const target = pro ? proRequestTarget(state.live) : specialistRequestTarget(state.live);
      if (!target) return;
      const player = button.dataset.specialistPlayer || "";
      state.specialistPlayerTabs.set(pro ? `pro:${target.key}` : target.key, player);
      refreshLiveRuneWorkspace(`[data-specialist-player="${CSS.escape(player)}"]`);
    });
    bindLiveNode(root?.querySelector("[data-retry-pro-runes]"), "click", "data-retry-pro-runes", () => { void ensureProRunes(state.live, true); });
	bindLiveNode(root?.querySelector("[data-open-riot-settings]"), "click", "data-open-riot-settings", () => {
	  window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: "settings", page: "privacy", focus: "riot-key" } }));
	});
	bindLiveNode(root?.querySelector("[data-retry-specialist-runes]"), "click", "data-retry-specialist-runes", () => {
	  const target = specialistRequestTarget(state.live);
	  if (!target) return;
	  state.specialistRuneFailures.delete(target.key);
	  state.specialistRunes.delete(target.key);
	  void ensureSpecialistRunes(state.live);
	});
    void ensureRuneStarterItems(state.live);
  }

  // R129 P2：面板内的交互绑定与 chrome（页签行、刷新按钮）分开。外科式替换单个
  // 面板时只重绑这一个面板，不会给没被替换的节点重复挂监听。
  function bindLivePanelScope(scope) {
    if (!scope) return;
    // 对局页里点击玩家名称：在当前页面上以覆盖层打开该玩家的总览，
    // 不再跳转到总览页；对局数据来自本机客户端，使用当前客户端的实际平台。
    for (const button of scope.querySelectorAll("[data-player-ref]")) button.onclick = () => {
      openPlayerOverlay({ playerRef: button.dataset.playerRef, region: clientRegion(), serverId: clientRegion() ? "" : state.status?.serverId || "", label: playerButtonLabel(button), ...proContextFromButton(button) });
    };
    bindRuneWorkspaceControls(scope);
    bindLiveNode(scope?.querySelector("[data-apply-rune-spells]"), "change", "data-apply-rune-spells", (event) => { state.applyRuneSpells = event.currentTarget.checked; });
    for (const button of scope.querySelectorAll("[data-live-position]")) bindLiveNode(button, "click", "live-position", () => selectLivePosition(livePositionValue(button.dataset.livePosition)));
    bindRuneChoiceButtons(scope);
    bindLiveNode(scope?.querySelector("[data-apply-runes]"), "click", "data-apply-runes", applyRunes);
    bindLiveNode(scope?.querySelector("[data-apply-item-set]"), "click", "data-apply-item-set", applyItemSet);
  }

  function bindLiveContent() {
    for (const button of nodes.liveContent.querySelectorAll("[data-recommendation-tab]")) bindLiveNode(button, "click", "recommendation-tab", () => {
      state.recommendationTab = button.dataset.recommendationTab;
      state.recommendationTabTouched = true;
      if (state.recommendationTab === "build") { ensureItems(); ensureSummonerSpells(); }
      activateLiveRecommendationTab(state.recommendationTab);
    });
    bindLiveNode(nodes.liveContent.querySelector(".recommendation-tabs"), "keydown", "recommendation-tabs", (event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const tabs = [...event.currentTarget.querySelectorAll('[role="tab"]')];
      const current = tabs.indexOf(document.activeElement);
      const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
      tabs[next]?.click();
    });
    bindLivePanelScope(nodes.liveContent);
  }

	function bindRuneChoiceButtons(root) {
	    for (const choice of root?.querySelectorAll("[data-rune-choice]") || []) {
	      if (choice._runeChoiceBound) continue;
	      choice._runeChoiceBound = true;
	      const select = () => {
	        state.selectedRecommendation = choice.dataset.runeChoice;
	        updateLiveRuneSelection(root);
	      };
	      choice.addEventListener("click", select);
	      choice.addEventListener("keydown", (event) => {
	        if (event.key === "Enter" || event.key === " ") {
	          event.preventDefault();
	          select();
	          return;
	        }
	        if (!["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
	        const group = choice.closest('[role="radiogroup"]');
	        const choices = [...(group?.querySelectorAll("[data-rune-choice]") || [])];
	        const current = choices.indexOf(choice);
	        if (current < 0 || !choices.length) return;
	        event.preventDefault();
	        const next = event.key === "Home" ? 0 : event.key === "End" ? choices.length - 1 : (current + (["ArrowRight", "ArrowDown"].includes(event.key) ? 1 : -1) + choices.length) % choices.length;
	        choices[next]?.focus();
	      });
	    }
	  }

  async function applyRunes(event) {
    const button = event.currentTarget;
    const recommendation = selectedRuneRecommendation(state.live);
    if (!recommendation) return;
    if(recommendation.sourceLabel==="自配置"){await applyCustomConfiguration(button,recommendation);return;}
    if (recommendation.selectedComplete === false || (recommendation.selectedComplete === true && recommendation.selectedPerkIds?.length !== 9)) { showToast("这套数据尚不完整，暂时不能应用"); return; }
    const self = (state.live?.players || []).find((player) => player.isCurrent);
    const championId = liveRecommendationChampionId(self);
    if (!String(self?.championName || "").trim() || !championId || !recommendation.sourceLabel) {
      showToast("暂时无法识别当前英雄或符文推荐来源");
      return;
    }
    button.disabled = true;
    button.textContent = "正在应用…";
    try {
      const spells = state.applyRuneSpells === false ? [] : recommendedRuneSpellIDs(state.live, recommendation);
      const payload = { championName: self.championName, source: recommendation.sourceLabel, championId: recommendation.championId || championId, primaryStyleId: recommendation.primaryStyleId, subStyleId: recommendation.subStyleId, selectedPerkIds: recommendation.selectedPerkIds || recommendation.perkIds || [] };
      if (spells.length) { payload.spell1Id = spells[0]; payload.spell2Id = spells[1]; }
      const result = await api("/api/gameplay/runes/apply", { method: "POST", body: JSON.stringify(payload) }, "apply-runes", 15000);
      showToast(spells.length ? result?.spellApplied === true ? "符文和召唤师技能已应用" : "符文已应用，召唤师技能未能同步" : "符文已应用并设为当前页");
      if(state.autoConfig?.championId===championId) {state.autoConfig.yielded ||= {};state.autoConfig.yielded.runes=true;}
      button.textContent = "再次应用";
    } catch (error) { showToast(error.message); button.textContent = "应用所选符文"; }
    finally { button.disabled = false; }
  }

  async function applyItemSet(event) {
    const button = event.currentTarget;
    const self = (state.live?.players || []).find((player) => player.isCurrent);
    const build = liveRecommendationsFor(state.live)?.build;
    if (!self || !build) {
      showToast("暂时无法识别当前英雄或装备推荐");
      recordItemSetClientDiagnostic("item_set_apply_request", "skipped", { reason: "missing-self-or-build" });
      return;
    }
    const payload = buildItemSetPayload(build, self, state.live);
    payload.storage = "recommended";
    if (!payload.championId || !payload.blocks.length) {
      const reason = !payload.championId ? "missing-champion-id" : "empty-blocks";
      showToast(!payload.championId ? "暂时无法识别当前英雄，不能应用装备方案" : "当前没有可应用的装备推荐");
      recordItemSetClientDiagnostic("item_set_apply_request", "skipped", { ...payload, blockCount: payload.blocks.length, reason });
      return;
    }
    const itemCount = payload.blocks.reduce((total, block) => total + block.items.length, 0);
    recordItemSetClientDiagnostic("item_set_apply_request", "submitted", { ...payload, blockCount: payload.blocks.length, itemCount });
    button.disabled = true;
    button.textContent = "正在应用…";
    try {
	  const result = await api("/api/gameplay/item-sets/apply", { method: "POST", body: JSON.stringify(payload) }, "apply-item-set", 20000);
		  const notice = result.notice ? `；${result.notice}` : "";
      recordItemSetClientDiagnostic("item_set_apply_request", "succeeded", { ...payload, traceId: result.traceId || payload.traceId, blockCount: payload.blocks.length, itemCount });
		  showToast(`${result.title || "装备方案"} 已写入客户端（${payload.blocks.length} 组 / ${itemCount} 件）${notice}`);
      button.textContent = "已写入";
      button.dataset.tooltip = "推荐文件已写入并校验；无法确认游戏是否已加载。游戏内商店首次打开可能把每组第一件裁掉，这是客户端渲染问题——客户端自建方案和其它工具的方案同样会出现，与写入内容无关，点一下任意装备即可归位。";
    } catch (error) {
      recordItemSetClientDiagnostic("item_set_apply_request", "failed", { ...payload, blockCount: payload.blocks.length, itemCount });
      showToast(error.message);
      button.textContent = "应用装备方案";
    }
    finally { button.disabled = false; }
  }

  function scheduleLiveRefresh() {
    clearTimeout(state.liveTimer);
    state.liveTimer = 0;
    if (state.destroyed || !state.settings.liveRefresh || !connected()) return;
    const phase = state.beacon.phase || state.live?.phase;
    if (!["ChampSelect", "GameStart", "InProgress", "Reconnect"].includes(phase)) return;
    syncLiveRetryBudget();
    const positionPending = ["InProgress", "Reconnect"].includes(phase) && state.live?.phase === phase && liveClientPositionsPending(state.live);
    if (positionPending) {
      if (Number(state.livePositionRetryAttempts || 0) >= 6) return;
      const gameId = String(state.live?.gameId || 0);
      const generation = Number(state.liveGameGeneration || 0);
      state.liveTimer = setTimeout(() => {
        state.liveTimer = 0;
        if (state.destroyed || !state.settings.liveRefresh || !connected() || generation !== Number(state.liveGameGeneration || 0)
          || String(state.live?.gameId || 0) !== gameId || !["InProgress", "Reconnect"].includes(state.beacon.phase || state.live?.phase)
          || !["InProgress", "Reconnect"].includes(state.live?.phase) || !liveClientPositionsPending(state.live)) return;
        state.livePositionRetryAttempts = Number(state.livePositionRetryAttempts || 0) + 1;
        void loadLive(false, "interval");
      }, 10000);
      return;
    }
    const background = document.hidden || state.section !== "live";
    if (background && (Number(state.liveRetryAttempts || 0) >= 8 || liveSnapshotComplete(state.live) && state.live?.phase === phase)) return;
    const inGame = phase === "GameStart" || phase === "InProgress" || phase === "Reconnect";
    if (inGame && (liveSnapshotComplete(state.live) && state.live?.phase === phase || Number(state.liveRetryAttempts || 0) >= 8)) return;
    const loadingProbe = inGame && liveAugmentRecommendationSource(state.live) === "arena" && !state.live?.arenaGrouped && Date.now() - Number(state.liveRetryStartedAt || 0) < 60_000;
    const loadingTenPlayerRoster = inGame && [400, 420, 430, 440, 450, 480, 490, 700, 720, 900, 930, 1900, 2300, 2400, 3220, 3270].includes(Number(state.live?.queueId)) && Array.isArray(state.live?.players) && state.live.players.length < 10;
    const delay = loadingProbe || loadingTenPlayerRoster ? 5_000 : inGame ? 20_000 : state.liveError ? 60_000 : liveRefreshDelayMs(phase, state.settings.liveInterval);
    state.liveTimer = setTimeout(() => {
      state.liveTimer = 0;
      if (background || inGame && !loadingProbe) state.liveRetryAttempts = Number(state.liveRetryAttempts || 0) + 1;
      void loadLive(false, "interval");
    }, delay);
  }

  function invalidatePrivacyRenderCaches() {
    for (const tab of new Set([...(state.tabs || []), ...(state.overlay || [])])) {
      tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;
      for (const view of tab.matchViews?.values() || []) { if (view.list) disposeMatchRowHeight(view.list); delete view.list; delete view.fragment; }
    }
    for (const container of [nodes.overviewContent,nodes.playerOverlayContent]) {
      if (!container) continue;
      for (const view of container._overviewViews?.values() || []) disposeMatchRowHeight(view.content);
      container._overviewViews?.clear();
      resetOverviewRenderCache(container);
    }
    for (const tab of nodes.playerTabs?.children || []) delete tab._tabMarkup;
    if (nodes.liveContent) delete nodes.liveContent._recommendationMarkup;
    if (nodes.careerDialogContent) delete nodes.careerDialogContent._careerData;
    const tab = state.careerDialogTab;
    if (nodes.careerDialog?.open && tab?.data) {
      const player=tab.data.player || {};
      const label=state.settings.maskNames ? privacyName(player,{context:"identity"}) : playerLabel(player);
      nodes.careerDialogContext.textContent=`${label} · ${tabServerLabel(tab)}`;
      renderCareerDialogContent(tab.data,tab,nodes.careerDialogContent.dataset.careerLayout || "single",true);
    }
  }
  window.addEventListener("deep-legends:privacy-changed", invalidatePrivacyRenderCaches);

  function bindSettings() {
    const autoLive = document.getElementById("setting-auto-live");
    if (autoLive) {
      autoLive.checked = readSetting("auto-live", "false") === "true";
      autoLive.addEventListener("change", () => writeSetting("auto-live", autoLive.checked));
    }
    const tagRoot = document.getElementById("setting-live-tags");
    const renderTags = () => {
      if (!tagRoot) return;
      tagRoot.innerHTML = [["self","自己"],["premade","组队 / 预组"],["pro","职业选手"],["hidden","隐藏战绩 / 身份"],["autofill","补位"],["squad","我的小队"],["positions","常用位置"]].map(([key,label]) => `<label><input type="checkbox" value="${key}" ${liveTagVisible(key) ? "checked" : ""}><span>${label}</span></label>`).join("");
    };
    renderTags();
    tagRoot?.addEventListener("change", () => { writeSetting("live-tags", JSON.stringify([...tagRoot.querySelectorAll("input:checked")].map(node => node.value))); renderLive("settings"); });
    document.getElementById("setting-live-tags-reset")?.addEventListener("click", () => { try { (globalThis.deepLegendsPreferenceStorage || localStorage).removeItem("lol-loot-live-tags"); } catch (_) {} renderTags(); renderLive("settings"); });
    window.addEventListener("deep-legends:settings-imported", () => { renderTags(); renderLive("settings"); });
    nodes.settingDefaultPage.value = state.settings.defaultPage;
    nodes.settingMatchCount.value = String(state.settings.matchCount);
    nodes.settingDefaultMatchFilter.value = state.settings.defaultMatchFilter;
    nodes.settingLiveRefresh.checked = state.settings.liveRefresh;
    nodes.settingLiveInterval.value = String(state.settings.liveInterval);
	    nodes.settingLiveOrder.value = state.settings.liveOrder;
    nodes.settingMaskNames.checked = state.settings.maskNames;
    nodes.settingConfirmReplay.checked = state.settings.confirmReplay;
    nodes.settingDefaultPage.addEventListener("change", () => { state.settings.defaultPage = nodes.settingDefaultPage.value; writeSetting("default-page", state.settings.defaultPage); });
    nodes.settingMatchCount.addEventListener("change", () => {
      state.settings.matchCount = normalizeMatchCount(nodes.settingMatchCount.value);
      writeSetting("match-count", state.settings.matchCount);
      for (const tab of state.tabs) tab.data = null;
      if (state.section === "overview") loadOverview(activeTab(overviewGroupForSection()), true);
    });
    nodes.settingDefaultMatchFilter.addEventListener("change", () => { state.settings.defaultMatchFilter = normalizeMatchFilter(nodes.settingDefaultMatchFilter.value); for (const tab of state.tabs) tab.matchFilter = state.settings.defaultMatchFilter; writeSetting("default-match-filter", state.settings.defaultMatchFilter); renderOverview(overviewGroupForSection()); });
    nodes.settingLiveRefresh.addEventListener("change", () => { state.settings.liveRefresh = nodes.settingLiveRefresh.checked; nodes.settingLiveInterval.disabled = !state.settings.liveRefresh; writeSetting("live-refresh", state.settings.liveRefresh); scheduleLiveRefresh(); renderLive("settings"); });
    nodes.settingLiveInterval.disabled = !state.settings.liveRefresh;
    nodes.settingLiveInterval.addEventListener("change", () => { state.settings.liveInterval = normalizeLiveInterval(nodes.settingLiveInterval.value); writeSetting("live-interval", state.settings.liveInterval); scheduleLiveRefresh(); renderLive("settings"); });
	    nodes.settingLiveOrder.addEventListener("change", () => { state.settings.liveOrder = normalizeLiveOrder(nodes.settingLiveOrder.value); writeSetting("live-order", state.settings.liveOrder); renderLive("settings"); });
    nodes.settingMaskNames.addEventListener("change", () => {
      state.settings.maskNames = nodes.settingMaskNames.checked; writeSetting("mask-names", state.settings.maskNames); window.dispatchEvent(new CustomEvent("deep-legends:privacy-changed")); renderPlayerTabs(); renderOverview(overviewGroupForSection()); renderLive("settings"); if (state.overlay.length) renderOverlay(); });
    nodes.settingConfirmReplay.addEventListener("change", () => { state.settings.confirmReplay = nodes.settingConfirmReplay.checked; writeSetting("confirm-replay", state.settings.confirmReplay); });
  }

  function renderCapabilitySettings() {
    const labels = { summoner: "召唤师资料", "ranked-stats": "排位数据", "match-history": "战绩列表", "seven-day-history": "7 天统计样本", "match-details": "对局详情", "champion-mastery": "英雄熟练度", gameflow: "游戏流程", "gameflow-session": "当前对局", "champ-select": "英雄选择", "live-player-analysis": "队伍分析", "client-rune-recommendation": "客户端推荐符文", "champion-abilities": "英雄技能" };
	const capabilities = (state.lastCapabilities || []).filter((item) => item?.state !== "canceled").filter(item => !clientRegion() || !String(item.path || "").startsWith("sgp:"));
	const list = capabilities.length ? `<div class="gameplay-capabilities">${capabilities.map((item) => `<div><span class="capability-dot is-${escapeHTML(item.state)}" aria-hidden="true"></span><span><strong>${escapeHTML(labels[item.name] || item.name)}</strong><small>${escapeHTML(item.detail || capabilityAttemptSummary(item) || (item.state === "available" ? `已读取 ${number(item.count)} 项` : "当前不可用"))}</small></span><b class="status-text${item.state === "available" ? "" : item.state === "unsupported" ? " is-warning" : " is-danger"}">${item.state === "available" ? "可用" : item.state === "unsupported" ? "不支持" : "读取失败"}</b></div>`).join("")}</div>` : '<p class="muted">打开总览或对局后显示能力状态。</p>';
    const available = capabilities.filter(item => item.state === "available").length;
    const degraded = capabilities.length - available;
    nodes.gameplaySettingsStatus.innerHTML = `<span class="status-text">${available} 项可用${degraded ? ` <span class="settings-capability-degraded status-text is-warning">· ${degraded} 项降级</span>` : ""}</span><button class="text-button" type="button">查看</button>`;
    nodes.gameplaySettingsStatus.querySelector("button")?.addEventListener("click", async () => {
      await window.deepLegendsSections.load("settings-preferences");
      const content = document.createElement("div"); content.innerHTML = list;
      window.deepLegendsSettingsModal("客户端接口", content);
    });
  }

	function capabilityAttemptSummary(capability) {
	  const sourceLabels = { lcu: "本机客户端", sgp: "腾讯 SGP", riot: "Riot API", opgg: "OP.GG" };
	  const outcomeLabels = { success: "成功", failed: "失败", disabled: "已关闭", "mode-unsupported": "模式不支持" };
	  const attempts = (Array.isArray(capability?.attempts) ? capability.attempts : []).filter(attempt => !clientRegion() || attempt.source !== "sgp");
	  return attempts.map((attempt) => {
		const source = sourceLabels[attempt?.source] || attempt?.source || "未知来源";
		const outcome = outcomeLabels[attempt?.outcome] || attempt?.outcome || "未知结果";
		return `${source}${outcome}${attempt?.message ? `：${attempt.message}` : ""}`;
	  }).join("；");
	}

  function emptyState(title, copy, retry, icon = "⬡") { return `<div class="gameplay-empty"><span aria-hidden="true">${escapeHTML(icon)}</span><strong>${escapeHTML(title)}</strong><p>${escapeHTML(copy)}</p>${retry ? '<button class="text-button" type="button" data-gameplay-retry>重试</button>' : ""}</div>`; }
  function summonerLabel(summoner) { return `${summoner.gameName || summoner.displayName || "当前召唤师"}${summoner.tagLine ? `#${summoner.tagLine}` : ""}`; }
  function playerLabel(player) { if (player.identityUnresolved === true) return "身份待公开"; if (player.hidden === true) return "隐藏玩家"; return `${player.gameName || player.displayName || "隐藏玩家"}${player.tagLine ? `#${player.tagLine}` : ""}`; }
  function privacyName(player, options = {}) { return window.deepLegendsPrivacy.name(player,{masked:state.settings?.maskNames===true,...options}); }
  function maskedPlayerName(player, index) { return state.settings.maskNames ? privacyName(player,{championName:player.championName,index,context:"live"}) : playerLabel(player); }
  function maskedListName(player, index) { if(state.settings.maskNames)return privacyName(player,{index,context:"recent"});if(player.identityUnresolved)return "身份待公开";if(player.hidden)return "隐藏玩家";return player.displayName || "隐藏玩家"; }
  function playerParticipantName(player, index) { return state.settings.maskNames ? privacyName(player,{championName:player.championName,index,context:"match"}) : playerLabel(player); }

  function rankTitle(rank) { const tier = String(rank.tier || "").toLowerCase(); const tierName = ({ iron: "黑铁", bronze: "青铜", silver: "白银", gold: "黄金", platinum: "铂金", emerald: "翡翠", diamond: "钻石", master: "大师", grandmaster: "宗师", challenger: "王者" })[tier] || rank.tier || "未定级"; return `${tierName}${rank.division && !["master", "grandmaster", "challenger"].includes(tier) ? ` ${rank.division}` : ""}`; }
  function rankTierMark(tier) { return ({ IRON: "I", BRONZE: "B", SILVER: "S", GOLD: "G", PLATINUM: "P", EMERALD: "E", DIAMOND: "D", MASTER: "M", GRANDMASTER: "GM", CHALLENGER: "C" })[String(tier).toUpperCase()] || "◇"; }
  function rankCrestIcon(tier) {
    const name = String(tier || "").toLowerCase();
    if (!["iron", "bronze", "silver", "gold", "platinum", "emerald", "diamond", "master", "grandmaster", "challenger"].includes(name)) return escapeHTML(rankTierMark(tier));
    return `<img class="rank-crest-icon" src="/rank-crests/${name}.png" alt="" decoding="async">`;
  }
  function positionLabel(value) { return ({ top: "上路", jungle: "打野", middle: "中路", bottom: "下路", utility: "辅助", other: "其他" })[value] || "位置未知"; }
  function positionIcon(value) {
    const name = ({ top: "top", jungle: "jungle", middle: "middle", bottom: "bottom", utility: "utility", other: "all" })[value] || "all";
    return `<img class="position-icon" src="/position-icons/${name}.svg" alt="" decoding="async">`;
  }
  function phaseLabel(value) { return ({ None: "大厅", Lobby: "大厅", Matchmaking: "匹配中", ReadyCheck: "等待接受", ChampSelect: "英雄选择", GameStart: "游戏启动", InProgress: "游戏进行中", Reconnect: "等待重连", EndOfGame: "游戏结束", PreEndOfGame: "结算中", WaitingForStats: "等待结算", Unavailable: "接口不可用" })[value] || value || "大厅"; }
  function matchSubject(match, playerRef) { return (match.participants || []).find((player) => Number(player.participantId) === Number(match.subjectParticipantId)) || (match.participants || []).find((player) => player.playerRef && player.playerRef === playerRef) || (match.participants || [])[0]; }
  function formatDuration(seconds) { const value = Number(seconds || 0); if (!value) return "时长未知"; return `${Math.floor(value / 60)}分${String(Math.floor(value % 60)).padStart(2, "0")}秒`; }
  // 时间戳解析：兼容毫秒数字与 ISO 字符串；无效时返回 null，界面直接不展示时间。
  function parseGameTime(value) {
    if (value == null || value === "" || value === 0) return null;
    const numeric = Number(value);
    const date = Number.isFinite(numeric) && numeric > 0 ? new Date(numeric) : new Date(String(value));
    return Number.isNaN(date.valueOf()) ? null : date;
  }
  function exactTime(value) {
    const date = parseGameTime(value);
    return date ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date) : "";
  }
  function relativeTime(value) { const date = parseGameTime(value); if (!date) return ""; const seconds = Math.max(0, Math.floor((Date.now() - date.valueOf()) / 1000)); if (seconds < 60) return "刚刚"; if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟前`; if (seconds < 86400) return `${Math.floor(seconds / 3600)}小时前`; if (seconds < 604800) return `${Math.floor(seconds / 86400)}天前`; return new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit" }).format(date); }

  function assetPath(kind, id) {
    if (kind === "champion" && Number(id) <= 0) return "/lol-game-data/assets/v1/champion-icons/-1.png";
    if (!id) return "";
    if (kind === "profile") return `/lol-game-data/assets/v1/profile-icons/${id}.jpg`;
    if (kind === "champion") return `/lol-game-data/assets/v1/champion-icons/${id}.png`;
    if (kind === "item") return `/lol-game-data/assets/v1/items/${id}.png`;
	if (kind === "spell") return `/lol-game-data/assets/v1/summoner-spells/${id}.png`;
	if (kind === "augment") {
	  const augment = augmentMetadata(id);
	  return augment?.iconPath || "";
	}
    if (kind === "perk") {
      const catalog = [...(state.perks?.perks || []), ...(state.perks?.styles || []).flatMap((style) => (style.slots || []).flatMap((slot) => slot.perks || []))];
      const perk = catalog.find((item) => Number(item.id) === Number(id));
      return perk?.iconPath || `/lol-game-data/assets/v1/perks/${id}.png`;
    }
    return "";
  }

  function proxyAsset(path) {
    if (!path) return "/image-unavailable.svg";
    // "ddragon:" 前缀表示后端目录来自 Data Dragon（未连接客户端时的兜底），
    // 通过联网资源代理加载而不是本机客户端。
    if (path.startsWith("ddragon:")) return `/api/champion-asset?source=ddragon&path=${encodeURIComponent(path.slice(8))}`;
    if (path.startsWith("hexdata:")) return `/api/champion-asset?source=hexdata&path=${encodeURIComponent(path.slice(8))}`;
    if (path.startsWith("builtin:")) return `/api/champion-asset?source=builtin&path=${encodeURIComponent(path.slice(8))}`;
    return `/api/image?path=${encodeURIComponent(path)}${/\/augments\/icons\//i.test(path) ? "&art=2" : ""}`;
  }
  function assetIcon(path, label, size = "", withTooltip = true, fallbackPath = "", extraClass = "") {
    const fallback = fallbackPath ? ` data-augment-fallback="${escapeHTML(proxyAsset(fallbackPath))}"` : "";
    return `<span class="game-icon${size ? ` is-${size}` : ""}${extraClass ? ` ${extraClass}` : ""}"${fallback}${withTooltip && String(label || "").trim() ? ` data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact"` : ""}><span aria-hidden="true">${escapeHTML(String(label || "?").slice(0, 1))}</span><img data-queued-src="${proxyAsset(path)}" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`;
  }
  function remoteStaticIcon(source, path, label, size = "", withTooltip = true, fallbackPath = "") {
    const fallback = fallbackPath ? ` data-augment-fallback="/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(fallbackPath)}"` : "";
    return `<span class="game-icon${size ? ` is-${size}` : ""}"${fallback}${withTooltip ? ` data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact"` : ""}><span aria-hidden="true">${escapeHTML(String(label || "?").slice(0, 1))}</span><img data-queued-src="/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(path)}${/\/augments\/icons\//i.test(path) ? "&art=2" : ""}" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`;
  }
  // Crop only the artwork vignette; keep the portrait box geometry identical to its placeholder.
  function iconFigure(kind, id, label, size = "", withTooltip = true) { return assetIcon(assetPath(kind, id), label || `${kind} ${id || ""}`, size, withTooltip, "", kind === "champion" ? "is-champion-art" : ""); }
  function maskedProfileIcon(size = "") { return `<span class="game-icon${size ? ` is-${size}` : ""}" data-tooltip="身份已遮罩" data-tooltip-size="compact"><span aria-hidden="true">◉</span></span>`; }
  // Use catalog icon paths; never guess URLs before catalog hydration.
  function pendingCatalogIcon(label, size = "", withTooltip = true) { return `<span class="game-icon is-pending${size ? ` is-${size}` : ""}"${withTooltip ? ` data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact"` : ""}><span aria-hidden="true"> </span></span>`; }
  // 装备/召唤师技能/符文图标一律使用目录里的 iconPath 与真实名称；
  // 目录未加载完成时先显示占位，加载完成后自动重绘。
  function itemIconFigure(id, size = "") {
    if (!state.items) return pendingCatalogIcon(`装备 ${id}`, size, false);
    const item = (state.items.items || []).find((candidate) => Number(candidate.id) === Number(id));
    if (!item) {
      state.itemsMissingIDs ||= new Set();
      if (Number(id) > 0 && state.itemsMissingIDs.size < 20 && !state.itemsMissingIDs.has(Number(id))) {
        state.itemsMissingIDs.add(Number(id));
        recordItemSetClientDiagnostic("item_id_not_in_catalog", "missing", { endpoint: "items", itemId: Number(id), itemCount: state.items.items.length, errorKind: "none" });
      }
      return pendingCatalogIcon(`装备 ${id}`, size, false);
    }
    const name = item.name || `装备 ${id}`;
    const description = plainText(item.description || "");
    const tooltip = [name, description].filter(Boolean).join("\n");
    const icon = item.iconPath ? assetIcon(item.iconPath, name, size, false) : pendingCatalogIcon(name, size, false);
    return `<span class="item-tooltip" tabindex="0" data-tooltip="${escapeHTML(tooltip)}" data-tooltip-size="item" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}">${icon}</span>`;
  }
  function spellIconFigure(id, size = "") {
    if (!Number(id)) return "";
    if (!state.summonerSpells) return pendingCatalogIcon(`召唤师技能 ${id}`, size);
    const spell = (state.summonerSpells.spells || []).find((candidate) => Number(candidate.id) === Number(id));
    const name = spell?.name || `召唤师技能 ${id}`;
    const description = plainText(spell?.description || "");
    const tooltip = description && description !== name ? `${name}\n${description}` : name;
    const icon = spell?.iconPath ? assetIcon(spell.iconPath, name, size, false) : iconFigure("spell", id, name, size, false);
    return `<span class="item-tooltip" tabindex="0" data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}">${icon}</span>`;
  }
  function perkIconFigure(id, size = "", withTooltip = true) {
    if (!state.perks) return pendingCatalogIcon(`符文 ${id}`, size, withTooltip);
    const catalog = [...(state.perks.perks || []), ...(state.perks.styles || []).flatMap((style) => (style.slots || []).flatMap((slot) => slot.perks || []))];
    const perk = catalog.find((item) => Number(item.id) === Number(id));
    return perk?.iconPath ? assetIcon(perk.iconPath, perk.name || `符文 ${id}`, size, withTooltip) : iconFigure("perk", id, `符文 ${id}`, size, withTooltip);
  }
  function perkStyleIconFigure(styleID, size = "") {
    const style = (state.perks?.styles || []).find((candidate) => Number(candidate.id) === Number(styleID));
    return style?.iconPath ? assetIcon(style.iconPath, `副系 · ${style.name}`, size) : "";
  }
  function augmentMetadata(id) {
    return (state.perks?.augments || []).find((candidate) => Number(candidate.id) === Number(id))
      || state.augmentCatalog?.get(Number(id));
  }
  function augmentIconFigure(id, size = "") {
    const augment = augmentMetadata(id);
    const description = state.augmentDescriptions?.get(Number(id));
    const details = { ...(augment || { id, name: `海克斯 ${id}`, rarity: "unknown" }), ...(description?.status === "ok" ? { description: description.description } : {}) };
    const icon = augment?.iconPath
      ? assetIcon(augment.iconPath, augment.name || `海克斯 ${id}`, size, false, augment.fallbackIconPath)
      : state.perks || state.augmentCatalog ? iconFigure("augment", id, details.name, size, false) : pendingCatalogIcon(details.name, size, false);
    return wrapAugmentIcon(icon, details, id);
  }
  function renderItemIcons(items, size = "") { const valid = items.filter((id) => Number(id) > 0); return valid.map((id) => itemIconFigure(id, size)).join(""); }
  function gameImageLoaded(image) {
    image.hidden = false;
    image.parentElement?.classList.add("has-loaded-image");image.parentElement?.classList.remove("has-image-error");
    if (image.closest(".augment-icon,.arena-augment-icon")) {
      if (window.deepLegendsAugmentArtwork) window.deepLegendsAugmentArtwork.prepare(image);
      else void window.deepLegendsSections?.load("augment-artwork").then(()=>window.deepLegendsAugmentArtwork?.prepare(image)).catch(()=>{});
    }
  }
  function gameImageFailed(image) {
    if (image.dataset.retryPending) return;
    const holder = image.parentElement;
    const fallback = holder?.dataset.augmentFallback;
    if (fallback && !image.dataset.augmentFallbackUsed) {
      image.dataset.augmentFallbackUsed = "1";
      image.hidden = false;
      image.setAttribute("data-queued-src", fallback);
      return;
    }
    // R100: no same-URL retry storm; alternate artwork above is still allowed.
    image.hidden = true;
    holder?.classList.remove("has-loaded-image");holder?.classList.add("has-image-error");
  }
  // load/error do not bubble, but document capture covers all locally replaced
  // cards too; no per-image listeners or repeated listener installation.
  document.addEventListener("load", event => {
    if (event.target.matches?.("[data-game-image]")) gameImageLoaded(event.target);
  }, true);
  document.addEventListener("error", event => {
    if (event.target.matches?.("[data-game-image]")) gameImageFailed(event.target);
  }, true);
  function disposeMatchRowHeight(root) {
    if (!root) return;
    const lists = root.matches?.(".match-list") ? [root] : root.querySelectorAll(".match-list");
    for (const list of lists) {list._rowHeightObserver?.disconnect();delete list._rowHeightObserver;delete list._measuredSummary;}
  }

  function observeMatchRowHeight(container) {
    if (typeof ResizeObserver !== "function") return;
    const list = container.matches?.(".match-list") ? container : container.querySelector(".match-list");
    if (!list) return;
    const summary = list.querySelector(".match-entry:not([hidden]) .match-summary");
    if (!summary || list._measuredSummary === summary) return;
    list._rowHeightObserver?.disconnect();
    list._measuredSummary = summary;
    list._rowHeightObserver = new ResizeObserver(entries => {
      if (!list.isConnected) return;
      const entry = entries.find(item => item.target === summary);
      const box = Array.isArray(entry?.borderBoxSize) ? entry.borderBoxSize[0] : entry?.borderBoxSize;
      const measured = Number(box?.blockSize) || summary.getBoundingClientRect().height;
      if (measured <= 0) return;
      const height = Math.max(118, Math.ceil(measured + 2));
      if (list._measuredRowHeight !== height) {list._measuredRowHeight = height;list.style.setProperty("--match-row-height",`${height}px`);}
    });
    list._rowHeightObserver.observe(summary);
  }

  function prepareImages(container) {
    observeMatchRowHeight(container);
    window.deepLegendsOverviewArt?.prepare(container);
    for (const image of container.querySelectorAll("[data-game-image]")) {
      // An image without src is complete too. It is still waiting for queue
      // admission; hiding it here prevents IntersectionObserver from admitting it.
      if (image.getAttribute("src") && image.complete) image.naturalWidth > 0 ? gameImageLoaded(image) : gameImageFailed(image);
    }
  }

	window.deepLegendsGameIcons = { iconFigure, prepareImages };

  // CSP: emit data attributes, then apply dynamic metrics through CSSOM.
  function applyRenderedMetricStyles(container) {
    if (!container) return;
    for (const node of container.querySelectorAll("[data-bar-width], [data-blue-share], [data-win-rate], [data-table-winrate], [data-skill-count]")) {
      const percent = value => `${Math.max(0, Math.min(100, Number(value) || 0))}%`;
      if (node.dataset.barWidth !== undefined) node.style.width = percent(node.dataset.barWidth);
      if (node.dataset.blueShare !== undefined) node.style.setProperty("--blue-share", percent(node.dataset.blueShare));
      if (node.dataset.winRate !== undefined) node.style.setProperty("--recent-win-rate", percent(node.dataset.winRate));
      if (node.dataset.tableWinrate !== undefined) node.style.setProperty("--table-winrate", percent(node.dataset.tableWinrate));
      if (node.dataset.skillCount !== undefined) node.style.setProperty("--skill-count", String(Math.max(1, Math.min(18, Number(node.dataset.skillCount) || 1))));
    }
  }

  function matchHasCompleteParticipantStats(match) {
    const participants = Array.isArray(match?.participants) ? match.participants : [];
    if (participants.length < 2) return false;
    const complete = participants.filter((participant) => Number(participant?.participantId) > 0 && Number(participant?.championId) > 0 && (
      Number(participant?.championLevel) > 0 || Number(participant?.kills) > 0 || Number(participant?.deaths) > 0 ||
      Number(participant?.assists) > 0 || Number(participant?.damage) > 0 || Number(participant?.damageTaken) > 0 ||
      Number(participant?.gold) > 0 || (participant?.itemIds || []).some((id) => Number(id) > 0) || (participant?.augmentIds || []).some((id) => Number(id) > 0)
    ));
    return complete.length >= 2;
  }

  function hydratedExternalMatch(original, hydrated, playerRef) {
    if (!hydrated || Number(hydrated.gameId) <= 0 || Number(hydrated.gameId) !== Number(original?.gameId) || !matchHasCompleteParticipantStats(hydrated)) return null;
    const originalSubject = matchSubject(original, playerRef);
    if (!originalSubject) return null;
    const participants = hydrated.participants || [];
    const normalized = (value) => String(value || "").trim().toLowerCase();
    const gameName = normalized(originalSubject.gameName);
    const tagLine = normalized(originalSubject.tagLine);
    let subject = gameName ? participants.find((participant) => normalized(participant.gameName) === gameName && normalized(participant.tagLine) === tagLine) : null;
    if (!subject) {
      const candidates = participants.filter((participant) => Number(participant.championId) === Number(originalSubject.championId) &&
        (!Number(originalSubject.placement) || Number(participant.placement) === Number(originalSubject.placement)) &&
        (!Number(originalSubject.subteamId) || Number(participant.subteamId) === Number(originalSubject.subteamId)));
      if (candidates.length === 1) subject = candidates[0];
    }
    if (!subject || Number(subject.participantId) <= 0) return null;
    return {
      ...original, ...hydrated,
      subjectParticipantId: Number(subject.participantId),
      result: Number(subject.placement) === 1 || subject.win ? "win" : "loss",
      averageTier: hydrated.averageTier ?? original?.averageTier,
    };
  }

  function mountExternalMatchCards(container, options = {}) {
    if (!container) return null;
    for (const mountedContainer of externalMatchViews.keys()) {
      if (!mountedContainer.isConnected) externalMatchViews.delete(mountedContainer);
    }
    const matches = Array.isArray(options.matches) ? options.matches : [];
    const playerRef = String(options.playerRef || "");
    const loadMatchDetails = typeof options.loadMatchDetails === "function" ? options.loadMatchDetails : null;
    const detailStates = new Map();
    const tab = {
      key: `external:${String(options.key || "matches")}`,
      region: String(options.region || ""),
      serverId: String(options.serverId || ""),
      overlay: true,
      data: { player: { playerRef, serverId: String(options.serverId || "") }, matches, capabilities: [] },
			matchCardOptions: {
        disableExpand: options.disableExpand === true,
        disableReplay: options.disableReplay === true,
        expandDisabledReason: String(options.expandDisabledReason || ""),
        replayDisabledReason: String(options.replayDisabledReason || ""),
				detailStates,
      },
      ...newTabView(),
    };
    const view = {
      tab,
      destroy() {
        if (externalMatchViews.get(container) === view) externalMatchViews.delete(container);
        tab.externalRender = null;
      },
      async toggleMatch(id) {
        id = String(id || "");
        if (!id || detailStates.get(id)?.status === "loading") return;
        if (tab.openMatches.has(id)) {
          tab.openMatches.delete(id);
          tab.matchDetailTabs.delete(id);
          resetBuildPlayerSelection(tab, id, "reset-collapse");
          view.render(id);
          return;
        }
        const index = matches.findIndex((match) => String(match.gameId) === id);
        const match = matches[index];
        if (!match) return;
        if (loadMatchDetails && !matchHasCompleteParticipantStats(match)) {
          detailStates.set(id, { status: "loading", message: "正在读取完整详情" });
          view.render(id, { full: true });
          try {
            const loaded = hydratedExternalMatch(match, await loadMatchDetails(match), playerRef);
            if (!loaded) throw new Error("Riot 返回的完整详情无法与当前样本匹配");
            if (externalMatchViews.get(container) !== view) return;
            matches[index] = loaded;
            tab.data.matches = matches;
            detailStates.delete(id);
            tab.openMatches.add(id);
          } catch (error) {
            if (externalMatchViews.get(container) !== view) return;
            const message = String(error?.message || "完整详情读取失败，请稍后重试").trim().split("\n")[0].slice(0, 180);
            detailStates.set(id, { status: "failed", message });
            tab.openMatches.delete(id);
          }
          view.render(id, { full: true });
          return;
        }
        tab.openMatches.add(id);
        tab.matchDetailModuleError=false;
        view.render(id);
      },
      bindRetryButtons(scope) {
        for (const button of scope.querySelectorAll("[data-retry-match-detail]")) {
          if (button._externalMatchRetryBound) continue;
          button._externalMatchRetryBound = true;
          button.addEventListener("click", () => view.toggleMatch(button.dataset.retryMatchDetail));
        }
      },
      render(id = "", options = {}) {
        if (!container.isConnected) { externalMatchViews.delete(container); return; }
        let scope = container;
        if (id) {
          const match = matches.find((item) => String(item.gameId) === id);
          const entry = [...container.querySelectorAll(".match-entry")].find((item) => item.querySelector("[data-toggle-match]")?.dataset.toggleMatch === id);
          if (!match || !entry) return;
          if (!options.full) {
            replaceMatchEntry(entry, tab, () => {});
            view.bindRetryButtons(entry);
            return;
          }
          // Hydration and loading/failure transitions also change the summary.
          // Replace this card only, then bind every control on its new nodes.
          const template = document.createElement("template");
          template.innerHTML = renderMatch(match, playerRef, tab).trim();
          scope = template.content.firstElementChild;
          if (!scope) return;
          entry.replaceWith(scope);
        } else container.innerHTML = matches.map((match) => renderMatch(match, playerRef, tab)).join("");
		for (const button of scope.querySelectorAll("[data-toggle-match]:not(:disabled)")) button.addEventListener("click", () => view.toggleMatch(button.dataset.toggleMatch));
        view.bindRetryButtons(scope);
        bindMatchDetailControls(scope, tab);
        for (const button of scope.querySelectorAll("[data-replay]:not(:disabled)")) button.addEventListener("click", () => replay(button));
        bindPlayerLinks(scope, tab);
        applyRenderedMetricStyles(scope);
        prepareImages(scope);
      },
    };
    externalMatchViews.get(container)?.destroy?.();
    tab.externalRender = (id = "") => view.render(id);
    externalMatchViews.set(container, view);
    if (options.openMatchId) tab.openMatches.add(options.openMatchId);
    view.render();
    ensurePerks();
    ensureItems();
    ensureSummonerSpells();
    return { destroy: () => view.destroy() };
  }

  async function currentArenaFirstPlaces(championID) {
    const tab = state.tabs[0];
    if (tabReady(tab) && !tab.data && !tab.loading) await loadOverview(tab);
    const playerRef = tab.data?.player?.playerRef || tab.playerRef || "";
    const matches = (tab.data?.matches || []).filter((match) => {
      const subject = matchSubject(match, playerRef);
      return match.modeGroup === "arena" && Number(subject?.championId) === Number(championID) && Number(subject?.placement) === 1;
    });
    return { matches, playerRef, region: tab.region || "", serverId: tabServerID(tab), loaded: Boolean(tab.data) };
  }

  function showToast(message, duration = 3600) {
    nodes.toast.textContent = message;
    nodes.toast.hidden = false;
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => { nodes.toast.hidden = true; }, duration);
  }

  function bindPlayerTabWorkspace(group) {
    const { tabs, prev, next, refresh } = overviewWorkspace(group);
    if (!tabs) return;
    tabs.addEventListener("click", (event) => {
      const close = event.target.closest("[data-close-player]");
      if (close) { closePlayerTab(close.dataset.closePlayer); return; }
      const tab = event.target.closest("[data-player-tab]");
      if (tab) selectPlayerTab(tab.dataset.playerTab);
    });
    tabs.addEventListener("keydown", (event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      const buttons = [...tabs.querySelectorAll("[data-player-tab]")];
      if (!buttons.length) return;
      event.preventDefault();
      const current = buttons.indexOf(document.activeElement);
      const focusedKey = document.activeElement?.dataset?.playerTab || "";
      if ((event.key === "ArrowLeft" || event.key === "ArrowRight") && state.tabs.some((tab) => tab.key === focusedKey && !tab.current)) {
        movePlayerTab(focusedKey, event.key === "ArrowRight" ? 1 : -1);
        return;
      }
      const targetIndex = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : Math.max(0, Math.min(buttons.length - 1, current + (event.key === "ArrowRight" ? 1 : -1)));
      buttons[targetIndex]?.focus();
    });
    tabs.addEventListener("dragstart", (event) => {
      const wrapper = event.target.closest("[data-player-tab-wrap]");
      const tab = state.tabs.find((item) => item.key === wrapper?.dataset.playerTabWrap);
      if (!wrapper || !tab || tab.current || tabGroup(tab) !== overviewGroupForSection()) { event.preventDefault(); return; }
      state.draggedPlayerTab = tab.key;
      wrapper.classList.add("is-dragging");
      event.dataTransfer?.setData("text/plain", tab.key);
      if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
    });
    tabs.addEventListener("dragover", (event) => {
      const target = event.target.closest("[data-player-tab-wrap]");
      if (!state.draggedPlayerTab || !target) return;
      event.preventDefault();
      for (const item of tabs.querySelectorAll(".is-drop-target")) item.classList.remove("is-drop-target");
      target.classList.add("is-drop-target");
      if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
    });
    tabs.addEventListener("drop", (event) => {
      const target = event.target.closest("[data-player-tab-wrap]");
      if (!target || !state.draggedPlayerTab) return;
      event.preventDefault();
      const rect = target.getBoundingClientRect();
      dropPlayerTab(state.draggedPlayerTab, target.dataset.playerTabWrap, event.clientX > rect.left + rect.width / 2);
      state.draggedPlayerTab = "";
    });
    tabs.addEventListener("dragend", () => {
      state.draggedPlayerTab = "";
      for (const item of tabs.querySelectorAll(".is-dragging, .is-drop-target")) item.classList.remove("is-dragging", "is-drop-target");
    });
    const scrollTabs = (direction) => {
      const distance = Math.max(180, Math.round(tabs.clientWidth * 0.68));
      tabs.scrollBy({ left: direction * distance, behavior: "smooth" });
    };
    prev?.addEventListener("click", () => scrollTabs(-1));
    next?.addEventListener("click", () => scrollTabs(1));
    let scrollFrame = 0;
    tabs.addEventListener("scroll", () => {
      if (scrollFrame) return;
      scrollFrame = requestAnimationFrame(() => {
        scrollFrame = 0;
        if (!state.destroyed) updatePlayerTabScrollControls(overviewGroupForSection());
      });
    }, { passive: true });
    if ("ResizeObserver" in window) new ResizeObserver(() => updatePlayerTabScrollControls(overviewGroupForSection())).observe(tabs);
    else window.addEventListener("resize", () => updatePlayerTabScrollControls(overviewGroupForSection()), { passive: true });
    refresh?.addEventListener("click", async () => {
      if (refresh.disabled) return;
      setOverviewRefreshLoading(overviewGroupForSection(), true);
      try {
        const tab = activeTab();
        if (tab) await loadOverview(tab, true, false, true);
      } finally {
        setOverviewRefreshLoading(overviewGroupForSection(), false);
      }
    });
  }
  bindPlayerTabWorkspace("players");
  nodes.playerGroups?.addEventListener("mouseenter", schedulePlayerGroupOpen);
  nodes.playerGroups?.addEventListener("mouseleave", schedulePlayerGroupClose);
  nodes.playerGroups?.addEventListener("click", event => {
    if (event.target.closest("#player-group-button")) {
      setPlayerGroupMenu(!playerGroupMenuOpen);
      return;
    }
    const item = event.target.closest("#player-group-menu [data-player-group]");
    if (item && item.getAttribute("aria-disabled") !== "true") choosePlayerGroup(item.dataset.playerGroup);
  });
  nodes.playerGroups?.addEventListener("keydown", event => {
    const button = event.target.closest("#player-group-button");
    const item = event.target.closest("#player-group-menu [data-player-group]");
    if (button) {
      if (["Enter", " "].includes(event.key)) {
        event.preventDefault();
        setPlayerGroupMenu(!playerGroupMenuOpen, !playerGroupMenuOpen ? "active" : "");
      } else if (["ArrowDown", "ArrowUp"].includes(event.key)) {
        event.preventDefault();
        setPlayerGroupMenu(true, event.key === "ArrowDown" ? "first" : "last");
      } else if (playerGroupMenuOpen && ["Home", "End"].includes(event.key)) {
        event.preventDefault();
        focusPlayerGroupItem(event.key === "Home" ? "first" : "last");
      } else if (!playerGroupMenuOpen && ["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) {
        event.preventDefault();
        selectAdjacentPlayerGroup(event.key);
      } else if (event.key === "Escape" && playerGroupMenuOpen) {
        event.preventDefault();
        setPlayerGroupMenu(false);
      }
      return;
    }
    if (!item) return;
    const items = enabledPlayerGroupItems();
    const index = Math.max(0, items.indexOf(item));
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.preventDefault();
      const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
      items[next]?.focus();
    } else if (["Enter", " "].includes(event.key)) {
      event.preventDefault();
      choosePlayerGroup(item.dataset.playerGroup);
    } else if (event.key === "Escape") {
      event.preventDefault();
      setPlayerGroupMenu(false);
      playerGroupButton()?.focus();
    }
  });
  nodes.playerGroups?.addEventListener("focusin", () => clearTimeout(playerGroupCloseTimer));
  nodes.playerGroups?.addEventListener("focusout", event => {
    if (!nodes.playerGroups.contains(event.relatedTarget)) schedulePlayerGroupClose();
  });
  function closePlayerGroupOnOutsidePointer(event) {
    if (playerGroupMenuOpen && !nodes.playerGroups?.contains(event.target)) setPlayerGroupMenu(false);
  }
  document.addEventListener("pointerdown", closePlayerGroupOnOutsidePointer);
  window.addEventListener("deep-legends:player-group", event => selectPlayerGroup(event.detail?.group));
  nodes.liveRefresh.addEventListener("click", () => loadLive(true, "manual"));
  window.addEventListener("deep-legends:riot-key-updated", () => {
	state.lastRiotKeyChangeAt=Date.now();
	state.riotKeyGeneration = Number(state.riotKeyGeneration || 0) + 1;
	for (const key of state.specialistRuneFlights.keys()) state.controllers?.get?.(`live-specialist-runes:${key}`)?.abort?.();
	state.specialistRuneFlights.clear();
    state.specialistRunes.clear(); state.specialistRuneFailures.clear();
    if (state.live) void ensureSpecialistRunes(state.live);
  });
  window.addEventListener("deep-legends:hard-refresh", (event) => {
    const detail = event.detail || {};
    const task = handleHardRefresh(detail);
    if (Array.isArray(detail.waitFor)) detail.waitFor.push(task);
  });
  window.addEventListener("deep-legends:status", (event) => updateStatus(event.detail || {}));
  window.addEventListener("deep-legends:client-view", (event) => {
    if (updateClientView(event.detail) && event.detail.state === "ready") applyOverviewPlayerIdentity(event.detail.summoner);
  });
	window.addEventListener("deep-legends:overview-player", (event) => applyOverviewPlayerIdentity(event.detail?.summoner || state.status?.summoner));
  window.addEventListener("deep-legends:section", (event) => activateSection(event.detail?.name || "overview"));
    window.addEventListener("deep-legends:history-service-recovered",event=>handleHistoryServiceRecovered(event.detail));
  window.addEventListener("deep-legends:lp-recorded",event=>handleLPRecorded(event.detail));
 window.addEventListener("deep-legends:overview-matches", event => handleOverviewMatches(event.detail));
	window.addEventListener("deep-legends:riot-relay-recovered", () => { void handleRelayRecovered(); });
	window.addEventListener("deep-legends:season-progress", (event) => { void handleSeasonProgress(event.detail || {}); });
	window.addEventListener("deep-legends:overview-incremental", (event) => { void handleOverviewIncremental(event.detail || {}); });

  window.addEventListener("deep-legends:live-disconnected", resetDisconnectedLive);
  window.addEventListener("deep-legends:open-player", (event) => {
    const playerRef = String(event.detail?.playerRef || "").trim();
    const gameName = String(event.detail?.gameName || "").trim();
    const tagLine = String(event.detail?.tagLine || "").trim();
    const region = String(event.detail?.region || "").trim();
    const serverId = String(event.detail?.serverId || "").trim().toUpperCase();
    const source = String(event.detail?.source || "champions");
    const badge = event.detail?.proPlayer;
    const isPro = region === "kr" && (source === "pro-players" || Boolean(badge?.playerName && badge?.teamCode));
    const tabContext = isPro ? { group: "pro", proTeam: badge?.teamCode || event.detail?.teamCode, proPlayer: badge?.playerName || event.detail?.playerName, proSecondary: badge?.secondary || event.detail?.secondary, expectedTier: event.detail?.expectedTier } : { group: "players" };
    const targetSection = "overview";
    const label = `${gameName || "好友"}${tagLine ? `#${tagLine}` : ""}`;
    if (playerRef) {
      if (source === "search" || isPro) {
        window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: targetSection } }));
        openPlayer(playerRef, label, region, serverId, tabContext);
      } else {
        openPlayerOverlay({ playerRef, region, serverId, label });
      }
      return;
    }
    if (!gameName) return;
    if (source === "search" || isPro) {
      // 同一总览内按来源切换国服、韩服、职业分组。
      window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: targetSection } }));
      openPlayerByRiotId(gameName, tagLine, region, serverId, tabContext);
      return;
    }
    // 英雄详情页等非总览入口：留在当前页面，用覆盖层展示总览。
    openPlayerOverlay({ riotId: { gameName, tagLine }, region, serverId, label: `${gameName}${tagLine ? `#${tagLine}` : ""}` });
  });
  nodes.playerOverlayBack?.addEventListener("click", overlayBack);
  nodes.playerOverlayAdd?.addEventListener("click", addOverlayPlayerToTabs);
  nodes.careerDialogClose?.addEventListener("click", closeCareerDialog);
  nodes.careerDialog?.addEventListener("click", (event) => {
    if (event.target === nodes.careerDialog) closeCareerDialog();
  });
  nodes.careerDialog?.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    event.preventDefault();
    event.stopPropagation();
    closeCareerDialog();
  });
  nodes.careerDialog?.addEventListener("close", () => {
    if (nodes.careerDialog.open) return;
    state.careerDialogObserver?.disconnect();
    state.careerDialogObserver = null;
    nodes.careerDialog.style.removeProperty("--career-dialog-width");
    nodes.careerDialogContent.replaceChildren();
    nodes.careerDialogContext.textContent = "";
    window.desktopTheme?.setModalOpen?.(false);
    const opener = state.careerDialogOpener;
    state.careerDialogOpener = null;
    requestAnimationFrame(() => {
      if (opener?.isConnected && opener.getClientRects().length) {
        opener.focus({ preventScroll: true });
        return;
      }
      const fallback = !nodes.playerOverlay?.hidden
        ? nodes.playerOverlayBack
        : overviewWorkspace(overviewGroupForSection()).tabs?.querySelector('[data-player-tab][aria-selected="true"]');
      fallback?.focus({ preventScroll: true });
    });
  });
  document.addEventListener("pointerdown", (event) => {
    if (event.target.closest(".app-select")) return;
    for (const root of document.querySelectorAll('.app-select [data-app-select-trigger][aria-expanded="true"]')) closeAppSelect(root.closest(".app-select"));
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && state.overlay.length && !document.querySelector("dialog[open]")) { event.preventDefault(); overlayBack(); }
  });
  document.addEventListener("visibilitychange", () => {
    if (state.destroyed) return;
    if (document.hidden) {
    clearTimeout(state.perksAugmentTimer);
    clearTimeout(state.currentGameTimer);
      // Hidden windows still prefetch an actual match, with the same bounded
      // retry budget. Pause idle-page work without losing a queued phase change.
      if (!liveGamePhase(state.beacon.phase)) {
        clearTimeout(state.liveEventTimer);
        state.liveEventTimer = 0;
      }
      scheduleLiveRefresh();
    } else if (state.resyncPending) {
      refreshAfterResync();
    } else if (state.liveRefreshQueued) {
      queueLiveEventRefresh();
    } else { scheduleBeaconPoll(0); }
  });

  /* ---------- 新对局提示灯：客户端进入英雄选择/对局时点亮“对局”页签 ---------- */
  const beaconPhases = new Set(["ChampSelect", "GameStart", "InProgress", "Reconnect"]);
  function updateBeacon(phase) {
    const normalizedPhase = String(phase || "");
    const active = beaconPhases.has(normalizedPhase);
    const phaseChanged = normalizedPhase !== state.beacon.phase;
    state.beacon.phase = normalizedPhase;
    if (active && (!state.beacon.active || phaseChanged)) {
      state.beacon.active = true;
      state.beacon.acked = state.section === "live";
    }
    if (!active) {
      state.beacon.active = false;
      state.beacon.acked = false;
    }
    renderBeacon();
  }
  function renderBeacon() {
    const tabButton = document.getElementById("section-live");
    if (!tabButton) return;
    const show = state.beacon.active && !state.beacon.acked && state.section !== "live";
    let dot = tabButton.querySelector(".live-beacon");
    if (show && !dot) {
      dot = document.createElement("span");
      dot.className = "live-beacon";
      dot.dataset.tooltip = "检测到新的对局，点击查看";
      dot.dataset.tooltipSize = "compact";
      dot.setAttribute("aria-hidden", "true");
      tabButton.append(dot);
    } else if (!show && dot) {
      dot.remove();
    }
  }
  // A fixed window plus one trailing refresh prevents event storms from
  // repeatedly aborting the full roster request before its body can arrive.
  function queueLiveEventRefresh(source = "event", phaseChanged = false) {
    if (state.destroyed || !connected()) return;
    phaseChanged = phaseChanged || Boolean(state.livePhaseRefreshQueued);
    if (state.section !== "live" && !phaseChanged && state.beacon.phase !== "ChampSelect") return;
    // In-game retries use the bounded timer; repeated same-phase SSE is not a retry clock.
    if (!phaseChanged && ["InProgress", "Reconnect"].includes(state.beacon.phase)) return;
    if (phaseChanged) state.livePhaseRefreshQueued = true;
    if (!state.liveRefreshQueued) recordLiveRefresh("queue", source, phaseChanged);
    state.liveRefreshQueued = true;
    if (document.hidden && !liveGamePhase(state.beacon.phase) || state.liveLoading || state.liveEventTimer) return;
    state.liveEventTimer = setTimeout(() => {
      state.liveEventTimer = 0;
      if (state.destroyed || !connected() || document.hidden && !liveGamePhase(state.beacon.phase) || state.liveLoading) return;
      void loadLive(Boolean(state.liveGameRefreshQueued), source);
    }, Math.max(180, 1000 - (Date.now() - Number(state.liveLoadStartedAt || 0))));
  }
  function handleSelfOverviewTimeout() {
    for (const tab of state.tabs || []) if (tab.current && !tab.overviewCardLoad?.selfReady) {
      tab.error = "总览读取超时，请重试";
      rerenderTab(tab,"handleSelfOverviewTimeout");
    }
  }
  window.addEventListener("deep-legends:self-overview-timeout", handleSelfOverviewTimeout);

  window.addEventListener("deep-legends:pro-runes", () => {
    const target = proRequestTarget(state.live);
    const entry = target && state.proRunes.get(target.key);
    if (entry) entry.receivedAt = 0;
    if (state.section === "live") void ensureProRunes(state.live);
  });
  window.addEventListener("deep-legends:dispose", () => clearTimeout(state.proRuneTimer));
  window.addEventListener("offline", () => {
    for (const entry of state.proRunes.values()) { entry.stale = true; entry.reason = "network-unavailable"; entry.receivedAt = 0; }
    if (state.section === "live") renderLive("sse");
  });
  window.addEventListener("online", () => { if (state.section === "live") void ensureProRunes(state.live, true); });
  function recordLiveRefresh(reason, source, phaseChanged = false) {
    window.reportFlowDiagnostic?.("live_refresh_client", reason, { source,
      hidden: document.hidden, section: state.section, phaseChanged, liveRefreshQueued: Boolean(state.liveRefreshQueued) });
  }

  // R129 P3: aggregate panel replacement causes into one diagnostic per minute.
  function liveRenderTriggerLabel(source) {
    const value = String(source || "").trim().toLowerCase();
    if (value === "interval") return "interval";
    if (value === "manual") return "manual";
    if (value === "sse" || value === "event") return "sse";
    if (value === "catalog") return "catalog";
    if (value === "progress") return "progress";
    if (["load", "timer", "navigation", "resync", "selection", "settings", "tabs", "recommendation", "rune", "image"].includes(value)) return value;
    return "direct";
  }

  function recordLiveRenderRebuild(parts, stats) {
    stats ||= {};
    const bucket = state.liveRenderRebuild || (state.liveRenderRebuild = { counts: {}, sources: {}, timer: 0, startedAt: 0 });
    bucket.imagesRecreated = Number(bucket.imagesRecreated || 0) + Number(stats.imagesRecreated || 0);
    bucket.rowsReplaced = Number(bucket.rowsReplaced || 0) + Number(stats.rowsReplaced || 0);
    bucket.fullReasons ||= {};
    if (stats.shellNode) bucket.shellNode = stats.shellNode;
    for (const reason of stats.fullReasons || []) bucket.fullReasons[reason] = Number(bucket.fullReasons[reason] || 0) + 1;
    if (!bucket.startedAt) bucket.startedAt = Date.now();
    for (const part of parts) bucket.counts[part] = Number(bucket.counts[part] || 0) + 1;
    const label = String(state.liveRenderSource || "direct");
    bucket.sources[label] = Number(bucket.sources[label] || 0) + 1;
    if (!bucket.timer) bucket.timer = setTimeout(flushLiveRenderRebuild, 60_000);
  }

  function flushLiveRenderRebuild() {
    const bucket = state.liveRenderRebuild;
    if (!bucket) return;
    clearTimeout(bucket.timer);
    state.liveRenderRebuild = { counts: {}, sources: {}, timer: 0, startedAt: 0 };
    const counts = bucket.counts || {};
    const total = Object.values(counts).reduce((sum, value) => sum + Number(value || 0), 0);
    if (!total) return;
    window.reportFlowDiagnostic?.("live_render_rebuild", "aggregated", {
      counts, shellNode: bucket.shellNode || "", sources: bucket.sources || {}, fullReasons: bucket.fullReasons || {}, total,
      windowMs: Math.max(0, Date.now() - Number(bucket.startedAt || 0)),
      phase: String(state.beacon?.phase || state.live?.phase || ""),
      imagesRecreated: Number(bucket.imagesRecreated || 0), rowsReplaced: Number(bucket.rowsReplaced || 0),
    });
  }

  function recordLiveProgressApply(result, requestId) {
    const bucket = state.liveProgressApply || (state.liveProgressApply = { applied: 0, ignored_same_game: 0, ignored_stale: 0, requests: new Set(), startedAt: Date.now(), timer: 0 });
    // One ignored background request corresponds to one complete roster read,
    // even though it emits a skeleton and several per-player snapshots.
    if (result === "ignored_same_game") {
      if (bucket.requests.has(requestId)) return;
      bucket.requests.add(requestId);
    }
    bucket[result]++;
    if (!bucket.timer) bucket.timer = setTimeout(flushLiveProgressApply, 60_000);
  }

  function flushLiveProgressApply() {
    const bucket = state.liveProgressApply;
    if (!bucket) return;
    clearTimeout(bucket.timer);
    state.liveProgressApply = null;
    if (!bucket.applied && !bucket.ignored_same_game && !bucket.ignored_stale) return;
    window.reportFlowDiagnostic?.("live_progress_apply", "aggregated", {
      applied: bucket.applied, ignored_same_game: bucket.ignored_same_game, ignored_stale: bucket.ignored_stale,
      windowMs: Math.max(0, Date.now() - bucket.startedAt),
    });
  }

  function handleGameplayPhase(phase, source = "sse", changed = false, gameId = 0) {
    if (state.destroyed) return;
    const phaseChanged = Boolean(phase) && phase !== state.beacon.phase;
    const nextId = normalizeLiveGameId(gameId);
    const previousId = normalizeLiveGameId(state.live?.gameId) || normalizeLiveGameId(state.liveExpectedGameId);
    const finishedQueueId = Number(state.live?.queueId || 0);
    const gameChanged = nextId > 0 && previousId > 0 && nextId !== previousId;
    if (phase || nextId) recordLiveObservation("received", phase, gameId, source);
    if (phaseChanged || gameChanged) {
      recordLiveRefresh("phase", source, phaseChanged);
      invalidateLiveForNewGame(state.beacon.phase || state.live?.phase, phase || state.beacon.phase, gameId, source);
    }
    if (nextId) state.liveExpectedGameId = nextId;
    else if (phaseChanged && (!liveGamePhase(phase) || phase === "ChampSelect")) state.liveExpectedGameId = 0;
    if (phase) {
      updateBeacon(phase);
      markOverviewAfterGame(phase, nextId || previousId, finishedQueueId);
    } else if (!nextId) scheduleBeaconPoll(0);
    if (!liveGamePhase(phase)) { state.autoLiveEntered = false; state.autoLiveLeft = false; }
    else {
      if (gameChanged) { state.autoLiveEntered = false; state.autoLiveLeft = false; }
      if ((phaseChanged || gameChanged) && ["ChampSelect", "GameStart", "InProgress"].includes(phase) && readSetting("auto-live", "false") === "true" && !state.autoLiveLeft) {
        state.autoLiveEntered = true;
        window.dispatchEvent(new CustomEvent("deep-legends:navigate", {detail:{section:"live"}}));
      }
    }
    if (phaseChanged || gameChanged) { state.liveRenderTrigger = "sse"; renderLive(); }
    if (phaseChanged || gameChanged || changed) queueLiveEventRefresh(source, phaseChanged || gameChanged);
  }
  window.addEventListener("deep-legends:post-game-reveal", () => {
    if (state.section === "live" && state.live?.gameId && !liveGamePhase(state.beacon.phase)) queueLiveEventRefresh("event");
  });
  window.deepLegendsGameAutoConfig = {state,renderLive,iconFigure,escapeHTML,renderItemIcon,renderRuneSpellIcons,renderUnifiedRuneBoard,runeConfigurationTitle,nodes,selectedRuneRecommendation,recommendedRuneSpellIDs,bindLiveNode,renderRecommendationArea,rememberLiveRecommendationMarkup,api,showToast};
  window.addEventListener("deep-legends:gameflow", (event) => {
    handleGameplayPhase(String(event.detail?.phase || ""), "sse", Boolean(event.detail?.changed), event.detail?.gameId);
  });
  // Identity polling detects a missed phase window without polling the full roster.
  const BEACON_FAST_POLL_MS = 1_000;
  const BEACON_DISCONNECTED_POLL_MS = 1_000;
  const BEACON_IDLE_POLL_MS = 12_000;
  let beaconPollTimer = 0;
  let lastLiveFrame = 0;
  window.addEventListener("deep-legends:live-frame", () => { lastLiveFrame = Date.now(); });
  window.addEventListener("deep-legends:live-disconnected", () => {
    lastLiveFrame = 0;
    scheduleBeaconPoll(0);
  });
  function beaconPollDelay() {
    return lastLiveFrame > 0 && Date.now() - lastLiveFrame < 45_000 ? BEACON_IDLE_POLL_MS : BEACON_FAST_POLL_MS;
  }
  function scheduleBeaconPoll(delay = BEACON_FAST_POLL_MS) {
    clearTimeout(beaconPollTimer);
    if (state.destroyed) return;
    beaconPollTimer = setTimeout(pollGameflowPhase, delay);
  }
  function pollGameflowPhase() {
    if (state.destroyed) return;
    if (!connected()) { scheduleBeaconPoll(BEACON_DISCONNECTED_POLL_MS); return; }
    const observedPhase = state.beacon.phase;
    const observedGeneration = state.liveGameGeneration;
    const observedId = state.liveExpectedGameId;
    api("/api/gameplay/phase", {}, "gameflow-phase", 8000)
      .then((payload) => {
        // An SSE signal or live response received during this probe is newer
        // evidence. Do not let the outstanding poll restore the old identity.
        if (observedPhase !== state.beacon.phase || observedGeneration !== state.liveGameGeneration || observedId !== state.liveExpectedGameId) {
          recordLiveObservation("stale-response", payload?.phase, payload?.gameId, "poll");
          return;
        }
        handleGameplayPhase(String(payload?.phase || ""), "poll", false, payload?.gameId);
      })
      .catch((error) => {
        window.reportFlowDiagnostic?.("gameflow_phase_client", "poll-failed", {
          phase: observedPhase, previousPhase: state.beacon.phase, source: "poll",
          gameId: observedId, gameIdComparison: "unavailable", phaseChanged: false,
          errorKind: error?.errorKind || (error?.name === "TimeoutError" ? "timeout" : "network"),
        });
      })
      .finally(() => scheduleBeaconPoll(document.hidden ? BEACON_IDLE_POLL_MS : beaconPollDelay()));
  }
  scheduleBeaconPoll(0);
  document.addEventListener("visibilitychange", () => { if (document.hidden) {
    for (const tab of state.tabs) if (tab.dirty) { clearTimeout(tab.dirtyTimer); tab.dirtyTimer = 0; tab.dirtyPausedAt = Date.now(); reportDirtyOverview(tab, "paused_hidden"); }
  } else {
    for (const tab of state.tabs) if (tab.dirty && (tab.current || tab === activeTab())) scheduleDirtyOverview(tab);
    scheduleLiveRefresh(); scheduleBeaconPoll(0); if (state.section === "overview" && activeTab()) scheduleOverviewCurrentGame(activeTab()); } });
  window.addEventListener("blur", () => { clearTimeout(state.currentGameTimer); });
  window.addEventListener("focus", () => { if (state.section === "overview" && activeTab()) scheduleOverviewCurrentGame(activeTab()); });
  const playerDurationTimer = setInterval(() => {
    if (!document.hidden) for (const node of document.querySelectorAll("[data-player-game-start]")) {
      node.textContent = friendGameDuration(Number(node.dataset.playerGameStart));
    }
    if (!document.hidden && state.section === "overview") for (const node of document.querySelectorAll("[data-current-game-start]")) {
      const seconds = Math.max(0, Math.floor((Date.now() - Date.parse(node.dataset.currentGameStart)) / 1000));
      if (Number.isFinite(seconds)) node.textContent = `已进行 ${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
    }
  }, 1_000);
  function computeOverviewStreak(matches, hasMore, playerRef) {
    const ordered = [...(matches || [])].sort((a,b) => Number(b.createdAt || 0)-Number(a.createdAt || 0));
    let result = "", count = 0, exhausted = true;
    for (const match of ordered) {
      if (match.result === "remake" || Number(match.queueId) === 0 || String(match.modeGroup || "").toLowerCase() === "custom" || /CUSTOM/i.test(`${match.gameType || ""} ${match.gameMode || ""}`)) continue;
      const subject = matchSubject(match, playerRef);
      const arena = match.modeGroup === "arena" || /ARENA|CHERRY/i.test(match.gameMode || "") || Number(subject?.subteamId) > 0;
      const next = arena ? (["win","loss"].includes(match.result) ? match.result : arenaPlacementResult(match, subject?.placement)) : ["win","loss"].includes(match.result) ? match.result : "";
      if (!["win","loss"].includes(next) || (result && next !== result)) { exhausted = false; break; }
      result = next; count++;
    }
    return count >= 3 ? {result,count,plus:Boolean(exhausted && hasMore)} : null;
  }

  function renderOverviewStreak(streak) {
    if (!streak) return "";
    const win = streak.result === "win";
    const icon = win ? '<svg viewBox="0 0 24 24" aria-hidden="true"><path class="streak-outer-flame" d="M12 2C15 7 19 8 19 14a7 7 0 1 1-14 0c0-3 2-5 4-7l1 5c3-2 3-5 2-10Z"/><path class="streak-inner-flame" d="M12 11c2 3 4 4 4 6a4 4 0 0 1-8 0c0-2 2-3 4-6Z"/></svg>' : '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 14a4 4 0 0 1-1-8 6 6 0 0 1 11-1 4 4 0 0 1 2 9Z"/><path class="streak-rain rain-1" d="m7 17-1 3"/><path class="streak-rain rain-2" d="m12 17-1 3"/><path class="streak-rain rain-3" d="m17 17-1 3"/></svg>';
    return `<span class="summoner-streak is-${win ? "win" : "loss"}${win && streak.count >= 5 ? " is-strong" : ""}">${icon}${streak.count}${streak.plus ? "+" : ""}连${win ? "胜" : "败"}</span>`;
  }

  async function copySummonerText(text) {
    let method="browser",errorName="";
    const report=ok=>window.reportFlowDiagnostic?.("summoner_copy",ok ? "success" : "failed",{ok,method,error_name:errorName});
    const toast=(message)=>{if(window.deepLegendsToast)window.deepLegendsToast(message,3000);else showToast(message,3000);};
    try {
      if(!navigator.clipboard?.writeText) throw new Error("ClipboardUnavailable");
      await navigator.clipboard.writeText(text);
    } catch(error) {
      errorName=error?.name || "Error";method="electron";
      try {
        if(!window.desktopClipboard?.copyText || await window.desktopClipboard.copyText(text)!==true) throw new Error("ClipboardUnavailable");
      } catch(error) {
        errorName=error?.name || "Error";method="execCommand";
        const previous=document.activeElement,input=document.createElement("textarea");
        input.value=text;input.setAttribute("readonly","");input.style.cssText="position:fixed;left:-9999px;top:0";document.body.appendChild(input);
        try {
          input.select();
          if(!document.execCommand("copy")) throw new Error("CopyFailed");
        } catch(error) { errorName=error?.name || "Error";report(false);toast("复制失败");return false; }
        finally {input.remove();previous?.focus?.({preventScroll:true});}
      }
    }
    report(true);toast("召唤师名称和编号已复制");return true;
  }

  function bindSummonerCopy(container) {
    for (const name of container.querySelectorAll("[data-copy-summoner]")) {
      if (name._summonerCopyBound) continue;
      name._summonerCopyBound = true;
      const copy = () => { void copySummonerText(name.dataset.copySummoner); };
      name.addEventListener("click", copy);
      name.addEventListener("keydown", event => { if (["Enter"," "].includes(event.key)) { event.preventDefault(); copy(); } });
    }
  }

  function reportDirtyOverview(tab, outcome) {
    if (typeof window === "undefined") return;
    window.reportFlowDiagnostic?.("overview_dirty_rescan", outcome, {
      attempt: Number(tab.dirtyAttempts || 0), filter: tab.matchFilter || "all",
      finished_game_id: String(tab.dirtyGameId || "0"), finished_queue_id: Number(tab.dirtyQueueId || 0),
      expected_present: Boolean(tab.expectedGamePresent), outcome,
      since_end_ms: Math.max(0, Date.now() - Number(tab.dirtyEndedAt || Date.now())),
    });
  }

  function markOverviewAfterGame(phase, gameId = 0, queueId = 0) {
    if (["ChampSelect", "InProgress"].includes(phase)) state.overviewEndHandled = false;
    if (!["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(phase) || state.overviewEndHandled) return;
    state.overviewEndHandled = true;
    const participants = new Set((state.live?.players || []).map(player => player.playerRef).filter(Boolean));
    for (const tab of state.tabs) {
      // A local match ending only invalidates accounts that played in this game.
      if (!tab.current && (riotTab(tab) || !participants.has(tab.playerRef || tab.data?.player?.playerRef))) continue;
      clearTimeout(tab.dirtyTimer);
      tab.dirtyTimer = 0;
      tab.dirty = true;
      tab.dirtyAttempts = 0;
      tab.dirtyPausedAt = 0;
      tab.dirtyEndedAt = Date.now();
      tab.dirtyDueAt = Date.now() + 7000;
      tab.dirtyGameId = String(gameId || state.liveExpectedGameId || state.live?.gameId || "0");
      tab.dirtyQueueId = Number(queueId || state.live?.queueId || 0);
      tab.dirtyBaseline = String(tab.latestAllGameId || ((tab.matchFilter || "all") === "all" ? tab.data?.matches?.[0]?.gameId || "" : ""));
      tab.expectedGamePresent = false;
      tab.matchViews = new Map();
      if (tab.current || (state.section === "overview" && tab === activeTab())) scheduleDirtyOverview(tab);
    }
  }

  function scheduleDirtyOverview(tab) {
    if (!tab?.dirty || state.destroyed || !connected() || tab.dirtyTimer) return;
    if (document.hidden) {
      if (!tab.dirtyPausedAt) { tab.dirtyPausedAt = Date.now(); reportDirtyOverview(tab, "paused_hidden"); }
      return;
    }
    if (Number(tab.dirtyAttempts || 0) >= 5) { tab.dirty = false; return; }
    if (tab.dirtyPausedAt) { tab.dirtyPausedAt = 0; tab.dirtyDueAt = Date.now(); }
    tab.dirtyTimer = setTimeout(async () => {
      tab.dirtyTimer = 0;
      if (state.destroyed || !state.tabs.includes(tab) || !connected() || !tab.dirty) return;
      if (document.hidden) { tab.dirtyPausedAt = Date.now(); reportDirtyOverview(tab, "paused_hidden"); return; }
      if (!tab.current && (state.section !== "overview" || tab !== activeTab())) return;
      if (tab.loading || tab.loadingMore) { tab.dirtyDueAt = Date.now() + 1000; scheduleDirtyOverview(tab); return; }
      tab.dirtyAttempts = Number(tab.dirtyAttempts || 0) + 1;
      const finishedGameId = tab.dirtyGameId, endedAt = tab.dirtyEndedAt;
      window.reportFlowDiagnostic?.("automatic_read_client", "request", { endpoint: "overview", source: "dirty_rescan" });
      const loaded = await loadOverview(tab, true);
      // A newer end-of-game event owns its own retry series.
      if (finishedGameId !== tab.dirtyGameId || endedAt !== tab.dirtyEndedAt || tab.closed) return;
      const resolved = loaded && (tab.dirtyGameId !== "0"
        ? tab.expectedGamePresent === true
        : Boolean(tab.dirtyBaseline && tab.latestAllGameId && tab.latestAllGameId !== tab.dirtyBaseline));
      if (resolved) { tab.dirty = false; reportDirtyOverview(tab, "resolved"); return; }
      if (tab.dirtyAttempts >= 5) { tab.dirty = false; reportDirtyOverview(tab, "gave_up"); return; }
      reportDirtyOverview(tab, "retry");
      tab.dirtyDueAt = Date.now() + [7000, 15000, 30000, 60000, 120000][tab.dirtyAttempts];
      scheduleDirtyOverview(tab);
    }, Math.max(0, Number(tab.dirtyDueAt || 0) - Date.now()));
  }

  function refreshAfterResync() {
    if (state.destroyed || document.hidden) return;
    state.resyncPending = false;
    if (state.section === "overview") {
      const tab = activeTab(overviewGroupForSection());
      if (tab) void loadOverview(tab, true);
    }
    if (state.section === "live") void loadLive(true, "resync");
    else if (state.liveRefreshQueued) queueLiveEventRefresh("resync");
    scheduleBeaconPoll(0);
  }
  window.addEventListener("deep-legends:resync", () => {
    if (state.destroyed) return;
    softResetGameplayState();
    for (const tab of state.tabs) tab.loadedAt = 0;
    state.resyncPending = true;
    refreshAfterResync();
  });
  function disposeGameplay() {
    globalThis.deepLegendsHistoryRecovery?.cancelAll();
    state.destroyed = true;
    for (const tab of [...state.tabs, ...state.overlay]) {
      if(globalThis.deepLegendsHistoryFilters)cancelAdvancedMatchSearch(tab);
      clearTimeout(tab.quotaRetry?.timer);
      clearTimeout(tab.matchTierRetryTimer);
      clearTimeout(tab.historyServerRetryTimer);
      clearTimeout(tab.historyWaitTimer);clearTimeout(tab.historyCountdownTimer);
    }
    for (const tab of state.tabs) clearTimeout(tab.dirtyTimer);
    clearInterval(playerDurationTimer);
    clearTimeout(beaconPollTimer);
    clearTimeout(state.liveTimer);
    flushLiveRenderRebuild();
    state.liveRenderRebuild = null;
    flushLiveProgressApply();
    state.liveProgressSnapshot = null;
    clearTimeout(state.perksAugmentTimer);
    clearTimeout(state.currentGameTimer);
    clearTimeout(state.overviewShareResetTimer);
    clearTimeout(state.liveEventTimer);
    clearTimeout(playerGroupOpenTimer);
    clearTimeout(playerGroupCloseTimer);
    document.removeEventListener("pointerdown", closePlayerGroupOnOutsidePointer);
    for (const tab of state.tabs) clearTimeout(tab.appendTimer);
    for (const entry of state.seasonProgressRefreshes.values()) clearTimeout(entry.timer);
    state.seasonProgressRefreshes.clear();
    for (const controller of state.controllers.values()) controller.abort();
    state.controllers.clear();
    for (const key of ["matchObserver", "proMatchObserver", "overlayObserver", "matchTierObserver", "proMatchTierObserver", "overlayTierObserver", "careerDialogObserver"]) state[key]?.disconnect();
  }
  window.addEventListener("deep-legends:dispose", disposeGameplay, { once: true });
  window.addEventListener("beforeunload", disposeGameplay, { once: true });


  window.deepLegendsMatchCards = Object.freeze({
    mount: mountExternalMatchCards,
    currentArenaFirstPlaces,
    live: { state, api, clientRegion, rankTitle, iconFigure, insightScore, render: renderLive },
  });

  window.deepLegendsPlayerTagContext = {
    sources: () => {
      const players=[...(state.live?.players || [])];
      for(const tab of [...state.tabs,...state.overlay,...[...externalMatchViews.values()].map(v=>v.tab)]) if(tab.data) {players.push(tab.data.player,...(tab.data.recentPlayers || []));for(const match of tab.data.matches || [])players.push(...(match.participants || []));}
      return players.filter(Boolean);
    },
    overviewPlayer: strip => {const container=strip.closest("#player-overlay-content");return container?state.overlay.at(-1)?.data?.player:activeTab()?.data?.player;},
    revealLive: () => {state.recommendationTab="insight";state.recommendationTabTouched=true;activateLiveRecommendationTab("insight");},
    live: () => state.live,
    name: player => privacyName(player,{context:"identity"}),
    avatar: player => state.settings.maskNames?maskedProfileIcon():iconFigure("profile",player.profileIconId,""),
    info: player => [player.position?positionLabel(player.position):"",player.rank?.tier?rankTitle(player.rank):"",player.championName?"本局 "+player.championName:""].filter(Boolean).join(" · "),
    open: player => openPlayer({playerRef:player.playerRef,label:privacyName(player),region:player.region,serverId:player.serverId}),
  };
  bindSettings();
  renderPlayerTabs();
  renderOverview("players");
  renderLive();
  renderCapabilitySettings();
  ensureAugments();
  restoreCommittedClientStatus();
  requestAnimationFrame(() => {
    const defaultSection = ["overview", "champions", "live", "favorites", "suite"].includes(state.settings.defaultPage) ? state.settings.defaultPage : "overview";
    if (defaultSection !== "overview") document.querySelector(`[data-section="${defaultSection}"]`)?.click();
  });
})();
