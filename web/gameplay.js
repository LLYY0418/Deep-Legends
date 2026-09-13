(() => {
  "use strict";

  const escapeHTML = window.deepLegendsRuntime.escapeHTML;

  const nodes = Object.fromEntries([
    "player-tabs", "player-tabs-prev", "player-tabs-next", "overview-content", "overview-refresh",
    "player-groups",
    "live-content", "live-refresh", "live-session-summary", "toast",
    "player-overlay", "player-overlay-back", "player-overlay-title", "player-overlay-add", "player-overlay-content",
    "career-dialog", "career-dialog-close", "career-dialog-context", "career-dialog-content",
    "setting-default-page", "setting-match-count", "setting-default-match-filter", "setting-live-refresh", "setting-live-interval", "setting-live-order", "setting-detail-position-align", "setting-mask-names",
    "setting-confirm-replay", "gameplay-settings-status",
  ].map((id) => [camel(id), document.getElementById(id)]));

  function newTabView() {
    return {
      matchFilter: normalizeMatchFilter(readSetting("default-match-filter", "all")),
	  rankedQueueRecent: "420", rankedQueueAbility: "420", rankedQueuePosition: "420",
	  matchScrollTops: new Map(),
	  openMatches: new Set(), matchDetailTabs: new Map(), damageSorts: new Map(), teamAnalysisMetrics: new Map(),
	  matchViewRevision: 0,
      playerRefs: new Set(),
      nextBegIndex: 0, paginationStalls: 0,
    };
  }

  const state = {
    status: null,
    section: "overview",
    tabs: [{ key: "current", playerRef: "", label: "当前召唤师", current: true, loading: false, data: null, error: "", ...newTabView() }],
    activeGroup: "players",
    activeTabs: { players: "current", kr: "", pro: "" },
    // 两套总览分别保存页签切换历史，关闭页签时只回到同一类账号。
    tabHistories: { players: [], kr: [], pro: [] },
    // 覆盖层栈：在非总览页点击玩家名称时，于当前页面之上展示该玩家的总览。
    overlay: [],
    playerTabOrder: readPlayerTabOrder(),
    draggedPlayerTab: "",
    controllers: new Map(),
    perks: null,
    perksLoading: false,
    perksRequestToken: 0,
    items: null,
    itemsLoading: false,
    summonerSpells: null,
    summonerSpellsLoading: false,
    live: null,
    liveLoading: false,
    liveError: "",
    liveTimer: 0,
    liveRecommendations: new Map(),
    liveRecommendationTraces: new Map(),
    liveRecommendationFlights: new Map(),
    liveRecommendationFailures: new Map(),
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
    // 对局时间线缓存（装备路线 + 技能加点）：key = region:gameId:participantId。
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
      detailPositionAlign: readSetting("detail-position-align", "false") === "true",
      maskNames: readSetting("mask-names", "false") === "true",
      // 回放默认直接在客户端内播放，不再弹确认框（可在设置中重新开启）。
      confirmReplay: readSetting("confirm-replay", "false") === "true",
    },
  };
  const externalMatchViews = new Map();

  function camel(value) { return value.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase()); }
  function readSetting(key, fallback) { try { return localStorage.getItem(`lol-loot-${key}`) ?? fallback; } catch (_) { return fallback; } }
  function writeSetting(key, value) { try { localStorage.setItem(`lol-loot-${key}`, String(value)); } catch (_) {} }
  function normalizeMatchCount(value) { const parsed = Number(value); return [10, 20, 30, 40, 50].includes(parsed) ? parsed : 20; }
  function normalizeMatchFilter(value) { return ["all", "solo", "flex"].includes(value) ? value : "all"; }
  function normalizeLiveInterval(value) { const parsed = Number(value); return [3, 5, 10, 15, 30, 60].includes(parsed) ? parsed : 3; }
  function liveRefreshDelayMs(phase, configuredSeconds) {
    const normalized = String(phase || "").trim();
    if (normalized === "ChampSelect") return 3_000;
    if (normalized === "InProgress" || normalized === "Reconnect") return 20_000;
    return normalizeLiveInterval(configuredSeconds) * 1_000;
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
    state.controllers.get(key)?.abort();
    const controller = new AbortController();
    state.controllers.set(key, controller);
    let timedOut = false;
    let errorStage = "network";
    let responseStatus = 0;
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, timeout);
    try {
      const headers = new Headers(options.headers || {});
      headers.set("Accept", "application/json");
      if (options.body) headers.set("Content-Type", "application/json");
      const response = await fetch(path, { ...options, headers, signal: controller.signal });
      responseStatus = response.status;
      errorStage = "read";
      if (response.status === 401) {
        const error = new Error("页面会话已过期，刷新页面即可重新连接");
        error.status = 401;
        error.errorKind = "http";
        throw error;
      }
      if (!response.ok) {
        const error = new Error((await response.text()).trim() || `本地服务返回 HTTP ${response.status}`);
        error.status = response.status;
        error.errorKind = "http";
        throw error;
      }
      errorStage = "decode";
      const payload = response.status === 204 ? null : await response.json();
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
        const timeoutError = new Error("客户端数据读取超时，请重试");
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
      if (state.controllers.get(key) === controller) state.controllers.delete(key);
    }
  }

  function connected() { return Boolean(state.status?.connected); }

  const PLAYER_GROUPS = { players: "国服", kr: "韩服", pro: "职业" };
  const PLAYER_GROUP_EMPTY_TIPS = { kr: "在顶栏搜索里选韩服并搜索玩家", pro: "从职业选手目录打开账号" };
  let playerGroupMenuOpen = false;
  let playerGroupOpenTimer = 0;
  let playerGroupCloseTimer = 0;
  function overviewGroupForSection() { return state.activeGroup || "players"; }
  function overviewSectionForGroup() { return "overview"; }
  function overviewWorkspace() {
    return { tabs: nodes.playerTabs, prev: nodes.playerTabsPrev, next: nodes.playerTabsNext, content: nodes.overviewContent, refresh: nodes.overviewRefresh };
  }
  function activeTab(group = overviewGroupForSection()) {
    return state.tabs.find(tab => tab.key === state.activeTabs[group] && tabGroup(tab) === group)
      || state.tabs.find(tab => tabGroup(tab) === group) || null;
  }
  function playerGroupCount(group) {
    return state.tabs.filter(tab => tabGroup(tab) === group && (!tab.current || connected())).length;
  }
  function visiblePlayerGroups() {
    return Object.keys(PLAYER_GROUPS).filter(group => group === "players" || playerGroupCount(group) > 0);
  }
  function selectPlayerGroup(group) {
    if (!visiblePlayerGroups().includes(group)) return;
    const tab = group === "players" && !connected() ? state.tabs.find(tab => tab.current) : activeTab(group);
    if (tab) selectPlayerTab(tab.key);
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
  function riotTab(tab) { return (tab?.region || "") === "kr"; }
  function tabReady(tab) { return connected() || riotTab(tab); }
  function tabServerID(tab) {
    if (riotTab(tab)) return "";
    const serverID = String(tab?.data?.player?.serverId || tab?.serverId || "").toUpperCase();
    return CN_SERVER_LABELS[serverID] ? serverID : "";
  }
  function tabServerLabel(tab) {
    if (riotTab(tab)) return "韩服";
    return CN_SERVER_LABELS[tabServerID(tab)] || tab?.data?.player?.serverName || "国服";
  }
  function tabServerTitle(tab) {
    if (riotTab(tab)) return "韩服";
    const serverID = tabServerID(tab);
    return serverID ? `国服 · ${tabServerLabel(tab)} (${serverID})` : "国服";
  }

  function shouldReloadOverview(tab, now = Date.now()) {
    return !tab?.data || now - Number(tab.loadedAt || 0) >= 120_000;
  }

  function matchTierScope(tab) {
    const region = riotTab(tab) ? "kr" : "cn";
    const serverID = riotTab(tab) ? "kr" : (tabServerID(tab) || "current");
    const playerRef = tab?.data?.player?.playerRef || tab?.playerRef || `${tab?.riotId?.gameName || ""}#${tab?.riotId?.tagLine || ""}` || "unknown";
    return `${region}:${serverID}:${playerRef}`;
  }

  function matchTierCacheKey(tab, gameID) {
    return `${matchTierScope(tab)}:${String(gameID || "")}`;
  }

  function activateSection(name) {
    const previous = state.section;
    state.section = name;
    clearTimeout(state.liveTimer);
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
      if (!tab && group === "pro") {
        renderOverview(group);
      } else if (tabReady(tab)) {
        if (shouldReloadOverview(tab) && tab.loadingMore) {
          tab.reloadAfterAppend = true;
          renderOverview(group);
        } else if (shouldReloadOverview(tab)) loadOverview(tab, true);
        else renderOverview(group);
      } else renderOverview(group);
    }
    if (name === "live") {
      if (connected()) { renderLive(); loadLive(); }
      else renderLive();
    }
    renderBeacon();
  }

  function updateStatus(status) {
    const wasConnected = connected();
    state.status = status;
    if (Array.isArray(status.queueGroups) && status.queueGroups.length) state.queueGroups = status.queueGroups;
    if (!status.connected) {
      // 客户端断开只清空依赖客户端的数据；韩服页签的数据来自 Riot API，保留。
      resetTencentTabsAfterDisconnect();
      state.overlay = state.overlay.filter((entry) => riotTab(entry));
      state.liveRequestToken = Number(state.liveRequestToken || 0) + 1;
      state.controllers.get("live")?.abort();
      state.liveLoading = false;
      state.live = null;
      state.liveError = "";
      resetLiveGameScopedState();
      // 只在连接状态真正切换时更换目录。周期性的未连接状态事件不能
      // 清空已加载的 Data Dragon 兜底，否则下一次卡片重绘会只剩空槽。
      if (wasConnected) {
        state.perks = null;
        state.items = null;
        state.summonerSpells = null;
        ensurePerks(true);
        ensureItems();
        ensureSummonerSpells();
      }
      updateBeacon("None");
      renderPlayerTabs();
      renderOverview();
      renderLive();
      renderOverlay();
      return;
    }
	const current = state.tabs.find((tab) => tab.current);
	const nextLabel = summonerLabel(status.summoner || {});
	const nextIcon = Number(status.summoner?.profileIconId) || 0;
	const identityChanged = Boolean(current && (current.label !== nextLabel || Number(current.icon || 0) !== nextIcon));
	if (identityChanged) {
	  current.label = nextLabel;
	  current.icon = nextIcon;
	  renderPlayerTabs();
	}
    if (!wasConnected && status.connected) {
      // 重新连接后改用客户端目录（名称与图标以客户端为准）。
      state.perks = null;
      state.items = null;
      state.summonerSpells = null;
      ensurePerks(true);
      ensureItems();
      ensureSummonerSpells();
	  if (!identityChanged) renderPlayerTabs();
	  if (state.section === "overview" && current) loadOverview(current);
      if (state.section === "live") loadLive();
      scheduleBeaconPoll(0);
    }
  }

  function resetTencentTabsAfterDisconnect() {
    const current = state.tabs.find((tab) => tab.current);
    const koreanTabs = state.tabs.filter((tab) => !tab.current && riotTab(tab));
    const removedTabs = state.tabs.filter((tab) => !tab.current && !riotTab(tab));
    for (const tab of removedTabs) state.controllers?.get(`overview:${tab.key}`)?.abort();
    if (current) {
      current.playerRef = "";
      current.playerRefs = new Set();
      current.label = "当前召唤师";
      current.icon = 0;
      current.data = null;
      current.error = "";
      current.loading = false;
      current.loadingMore = false;
      current.loadedAt = 0;
    }
    state.tabs = current ? [current, ...koreanTabs] : koreanTabs;
    for (const group of ["players", "kr", "pro"]) {
      const retainedKeys = new Set(state.tabs.filter((tab) => tabGroup(tab) === group).map((tab) => tab.key));
      state.tabHistories[group] = (state.tabHistories[group] || []).filter((key) => retainedKeys.has(key));
      if (!retainedKeys.has(state.activeTabs[group])) state.activeTabs[group] = state.tabs.find(tab => tabGroup(tab) === group)?.key || "";
    }
  }

  function rerenderTab(tab) {
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
	if (backgroundFields.some((field) => Object.prototype.hasOwnProperty.call(summoner, field))) {
	  Object.assign(identity, {
		backgroundSkinId: Number(summoner.backgroundSkinId) || 0,
		backgroundSkinName: String(summoner.backgroundSkinName || ""),
		backgroundSource: String(summoner.backgroundSource || ""),
		backgroundPath: String(summoner.backgroundPath || ""),
		backgroundPosterPath: String(summoner.backgroundPosterPath || ""),
		backgroundVideoPath: String(summoner.backgroundVideoPath || ""),
	  });
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
	rerenderTab(tab);
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
      hasMore: upstreamHasMore && !reachedLimit && !stalled,
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
	const data = tab.data || {};
	const pagination = data.pagination || { hasMore: false };
	if (tab.filterPaging) return `<span class="mini-loading" aria-hidden="true"></span><span>正在查找更早的${escapeHTML(matchFilterDisplayLabel(tab))}对局…第 ${number(Math.max(2, Number(tab.filterPagingPage) || 2))} 页</span>`;
	if (tab.loadingMore) return '<span class="mini-loading" aria-hidden="true"></span><span>正在加载下一批战绩…</span>';
    if (pagination.hasMore) {
      if (pagination.budgetExceeded) return `<span>本次只读到 ${number((data.matches || []).length)} 条，点这里继续</span><button class="text-button" type="button" data-load-more>继续加载</button>`;
      if (pagination.partial) return `<span>上游中断，已加载 ${number((data.matches || []).length)} 条，点击继续</span><button class="text-button" type="button" data-load-more>继续加载</button>`;
      return pagination.moreError
        ? `<span>自动加载已暂停：${escapeHTML(pagination.moreError)}</span><button class="text-button" type="button" data-load-more>重试加载</button>`
        : pagination.autoPaused
          ? `<span>${escapeHTML(pagination.pauseReason || "自动加载已暂停")}</span><button class="text-button" type="button" data-load-more>继续查找</button>`
        : '<span>继续向下滚动，自动加载更多</span><button class="text-button" type="button" data-load-more>加载更多</button>';
    }
    return `<span>${escapeHTML(pagination.exhaustedReason || "已展示全部可查询战绩")} · 共 ${number((data.matches || []).length)} 场</span>`;
  }

  function matchSentinelShouldHide(tab, hasVisibleMatch) {
    return Boolean(tab?.filterPaging && !hasVisibleMatch);
  }

  function matchListEmptyContent(tab, historyCapability, specialModeEmpty) {
	if (historyCapability && historyCapability.state !== "available") {
	  return emptyState("战绩暂时无法读取", historyCapability.detail || "客户端暂未返回该玩家的战绩，请稍后重试。", true);
	}
	if (tab.filterPaging) {
	  return `<div class="match-filter-paging" role="status">${paginationCopyFor(tab)}</div>`;
	}
	if (tab.data?.pagination?.hasMore && !tab.data.pagination.autoPaused) {
	  return '<div class="match-filter-paging" role="status"><span class="mini-loading" aria-hidden="true"></span><span>继续查找更早的对局…</span></div>';
	}
	return emptyState("没有符合条件的对局", specialModeEmpty, false);
  }

  function bindMatchSentinel(container, tab) {
    const observerKey = matchObserverKey(tab);
    state[observerKey]?.disconnect();
    state[observerKey] = null;
    const sentinel = container?.querySelector("[data-match-sentinel]");
	if (!sentinel || !tab.data?.pagination?.hasMore || tab.data.pagination.autoPaused || tab.loadingMore || tab.filterPaging || !("IntersectionObserver" in window)) return;
    const scrollRoot = container.closest(".player-overlay-scroll") || document.getElementById("app-scroll");
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting) || tab.appendFramePending) return;
      observer.disconnect();
      state[observerKey] = null;
      tab.appendFramePending = true;
      const schedule = window.requestAnimationFrame || ((callback) => setTimeout(callback, 0));
      schedule(() => {
        const delay = Math.max(0, Number(tab.nextAutoAppendAt || 0) - Date.now());
        clearTimeout(tab.appendTimer);
        tab.appendTimer = setTimeout(() => {
          tab.appendFramePending = false;
          tab.appendTimer = 0;
          loadOverview(tab, false, true);
        }, delay);
      });
    }, { root: scrollRoot, rootMargin: "0px 0px 48px 0px" });
    observer.observe(sentinel);
    state[observerKey] = observer;
  }

  function appendOverviewMatches(tab, additions) {
    const container = overviewContainer(tab);
    if (!container) return;
    const list = container.querySelector(".match-list");
    if (list && additions.length) {
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
    bindMatchSentinel(container, tab);
    if (list) list._matchData = new Map((tab.data?.matches || []).map(match => [String(match.gameId), match]));
    container._matchListMatches = tab.data?.matches;
    container._matchListFilter = tab.matchFilter;
    container._matchListViewRevision = Number(tab.matchViewRevision || 0);
  }

  async function loadOverview(tab, force = false, append = false, manual = false, quiet = false) {
	if (!tabReady(tab)) { rerenderTab(tab); return false; }
	if (force) {
      // Successful immutable timelines remain cached; failed attempts must be
      // retried even when the build detail stayed open during a refresh.
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
	  if (tab.data && !force) {
        rerenderTab(tab);
        void loadOPGGSeasonSummary(tab);
        void loadOverviewCurrentGame(tab);
        return false;
      }
      tab.loading = true;
	}
    const requestToken = Number(tab.overviewRequestToken || 0) + 1;
    tab.overviewRequestToken = requestToken;
    tab.error = "";
	let appendAdditions = [];
	let loaded = false;
    if (append) showLoadingMoreState(tab);
    else if (!quiet) rerenderTab(tab);
    try {
      const begIndex = append ? Math.max(0, Number(tab.nextBegIndex ?? (Number(tab.data?.pagination?.begIndex || 0) + Number(tab.data?.pagination?.count || 0)))) : 0;
      const verification = tabGroup(tab) === "pro" && !append ? { expectedTier: tab.expectedTier || "" } : {};
      const body = tab.current ? null : JSON.stringify(tab.riotId && !tab.playerRef
		? { ...verification, gameName: tab.riotId.gameName, tagLine: tab.riotId.tagLine, region: tab.region || "", serverId: tab.serverId || "", count: state.settings.matchCount, begIndex, force, matchFilter: tab.matchFilter }
		: { ...verification, playerRef: tab.playerRef, serverId: tab.serverId || "", count: state.settings.matchCount, begIndex, force, matchFilter: tab.matchFilter });
      const timeout = 25_000;
      const requestKey = `${append ? "overview-more" : "overview"}:${tab.key}`;
      const payload = tab.current
		? await api(`/api/gameplay/overview?count=${state.settings.matchCount}&begIndex=${begIndex}&force=${force ? 1 : 0}&matchFilter=${encodeURIComponent(tab.matchFilter || "all")}`, {}, requestKey, timeout)
        : await api("/api/gameplay/overview", { method: "POST", body }, requestKey, timeout);
      if (tab.overviewRequestToken !== requestToken) return false;
      if (payload.proMismatch) markProMismatch(tab);
      if (append) {
        const seen = new Set((tab.data.matches || []).map((match) => String(match.gameId)));
        const additions = (payload.matches || []).filter((match) => {
          const gameID = String(match.gameId);
          if (seen.has(gameID)) return false;
          seen.add(gameID);
          return true;
        });
        const pagination = normalizedPagination(payload, begIndex);
		const upstreamAdditions = additions.length;
		tab.paginationStalls = upstreamAdditions > 0 ? 0 : Number(tab.paginationStalls || 0) + 1;
		if (pagination.hasMore && tab.paginationStalls >= 2) {
		  pagination.autoPaused = true;
		  pagination.pauseReason = "上游连续无响应，已暂停自动查找";
        }
        tab.nextBegIndex = pagination.nextBegIndex;
        delete pagination.nextBegIndex;
        tab.data = { ...tab.data, matches: [...(tab.data.matches || []), ...additions], pagination: { ...pagination, moreError: "" } };
        appendAdditions = additions;
        tab.paginationBackoffMs = 0;
        tab.nextAutoAppendAt = Date.now() + AUTO_PAGE_DELAY_MS;
      } else {
        const pagination = normalizedPagination(payload, 0);
        const previousData = tab.data;
        const previousMatches = previousData?.matches || [];
        const freshMatches = payload.matches || [];
        const preserveLoadedPages = force && previousMatches.length > freshMatches.length;
        let mergedMatches = freshMatches;
        let mergedPagination = pagination;
        if (preserveLoadedPages) {
          const freshIDs = new Set(freshMatches.map((match) => String(match.gameId)));
          mergedMatches = [...freshMatches, ...previousMatches.filter((match) => !freshIDs.has(String(match.gameId)))].slice(0, MAX_BROWSE_MATCHES);
          mergedPagination = { ...(previousData.pagination || {}), moreError: "" };
        }
        tab.nextBegIndex = preserveLoadedPages ? Number(tab.nextBegIndex || Number(mergedPagination.begIndex || 0) + Number(mergedPagination.count || 0)) : pagination.nextBegIndex;
        tab.paginationStalls = 0;
        delete mergedPagination.nextBegIndex;
        tab.data = { ...(previousData || {}), ...payload, matches: mergedMatches, pagination: mergedPagination };
        tab.loadedAt = Date.now();
        tab.paginationBackoffMs = 0;
		tab.nextAutoAppendAt = Date.now() + AUTO_PAGE_DELAY_MS;
	  }
	  loaded = true;
      const resolvedPlayerRef = payload.player?.playerRef || "";
      rememberTabPlayerRef(tab, resolvedPlayerRef);
      tab.playerRef = resolvedPlayerRef || tab.playerRef;
      tab.region = payload.player?.region || tab.region || "";
      tab.serverId = payload.player?.serverId || tab.serverId || "";
      tab.serverName = payload.player?.serverName || tab.serverName || "";
      tab.label = playerLabel(payload.player || {});
      tab.icon = payload.player?.profileIconId || 0;
      tab.error = "";
		if (!append) {
		  state.lastCapabilities = Array.isArray(payload.capabilities) ? payload.capabilities : [];
		  renderCapabilitySettings();
		}
    } catch (error) {
      if (tab.overviewRequestToken !== requestToken) return false;
      if (error.name !== "RequestCancelled") {
        // 玩家引用极少数情况下会失效（例如切换登录账号）；搜索打开的页签
        // 还留有 Riot ID，直接改用 Riot ID 重新查询一次。
        if (!append && tab.playerRef && tab.riotId && /引用/.test(error.message)) {
          tab.playerRefs?.delete(tab.playerRef);
          tab.playerRef = "";
          tab.loading = false;
          tab.loadingMore = false;
          loadOverview(tab, true);
          return;
        }
        if (!append && error.status === 404 && !/引用/.test(error.message)) markProMismatch(tab);
        if (append) {
          if (error.status === 503) {
            tab.paginationBackoffMs = Math.min(AUTO_PAGE_MAX_BACKOFF_MS, Math.max(AUTO_PAGE_DELAY_MS * 2, Number(tab.paginationBackoffMs || 0) * 2));
            tab.nextAutoAppendAt = Date.now() + tab.paginationBackoffMs;
          }
          const retryCopy = error.status === 503
            ? `${error.message}；已退避 ${Math.max(1, Math.ceil(tab.paginationBackoffMs / 1000))} 秒`
            : error.message;
          tab.data.pagination = { ...(tab.data.pagination || {}), hasMore: true, autoPaused: true, pauseReason: "", moreError: retryCopy };
          showToast(`更多战绩加载失败：${error.message}`);
        }
        else if (!quiet) tab.error = error.message;
      }
	} finally {
	  const reloadAfterAppend = tab.overviewRequestToken === requestToken && append && tab.reloadAfterAppend;
      // 被新刷新取代的旧请求不能清除新请求的 loading 状态。
      if (tab.overviewRequestToken === requestToken) {
        tab.loading = false;
        tab.loadingMore = false;
        if (append) appendOverviewMatches(tab, appendAdditions);
        else rerenderTab(tab);
	  }
	  if (reloadAfterAppend) {
		tab.reloadAfterAppend = false;
		void loadOverview(tab, true, false, false, true);
	  }
	}
	if (loaded && !append) {
      void loadOPGGSeasonSummary(tab, force && !quiet);
      void loadOverviewCurrentGame(tab, force && !quiet);
    }
	return loaded;
  }

  // One aggregated OP.GG query, independent of Riot history and pagination.
  // Keep its result on this tab; never refresh the entire overview on completion.
  async function loadOPGGSeasonSummary(tab, force = false) {
    const player = tab?.data?.player;
    const ref = String(player?.playerRef || "");
    if (String(player?.region || "").toLowerCase() !== "kr" || player?.privateHistory || !ref || state.destroyed) return false;
    if (tab.opggSeasonPending === ref) return false;
    const same = tab.opggSeason?.playerRef === ref;
    const age = Date.now() - Number(tab.opggSeasonAttemptAt || 0);
    if (!force && tab.opggSeasonAttemptRef === ref && age < (same ? 600_000 : 30_000)) return false;
    tab.opggSeasonPending = ref;
    tab.opggSeasonAttemptRef = ref;
    tab.opggSeasonAttemptAt = Date.now();
    try {
      const summary = await api("/api/gameplay/season-summary", { method: "POST", body: JSON.stringify({ playerRef: ref, force }) }, `opgg-season:${tab.key}`, 17_000);
      if (state.destroyed || tab.data?.player?.playerRef !== ref) return false;
      if (summary?.source !== "OP.GG" || summary?.queue !== "RANKED" || !summary?.season || !Array.isArray(summary.champions) || !Number.isFinite(summary.overall?.games)) return false;
      tab.opggSeason = { playerRef: ref, data: summary };
      tab.opggSeasonStale = false;
      return true;
    } catch (error) {
      if (error.name !== "RequestCancelled" && tab.data?.player?.playerRef === ref) {
        tab.opggSeasonStale = same;
        // Retain an available season aggregate, otherwise show unavailable;
        // never replace season statistics with a recent-match sample.
      }
      return false;
    } finally {
      if (tab.opggSeasonPending === ref) tab.opggSeasonPending = "";
      if (!state.destroyed && tab.data?.player?.playerRef === ref) rerenderTab(tab);
    }
  }

	async function handleSeasonProgress(detail, now = Date.now()) {
	  if (state.section !== "overview" || !detail || detail.type !== "season-progress") return false;
	  const group = overviewGroupForSection();
	  const tab = activeTab(group);
	  const progress = tab?.data?.seasonStatsProgress;
	  if (!tab?.data || !progress?.season || String(detail.season || "") !== String(progress.season)) return false;
	  const account = String(detail.account || "");
	  const tabAccount = String(tab.data?.player?.playerRef || tab.playerRef || "");
	  if (account && tabAccount && account !== tabAccount) return false;
	  const completed = detail.complete === true && progress.complete === false;
	  const advanced = Number(detail.scanned || 0) - Number(progress.scanned || 0) >= 100;
	  if (!completed && !advanced) return false;
	  const refreshKey = account || tabAccount || tab.key;
	  const previous = state.seasonProgressRefreshes.get(refreshKey) || { lastAt: 0, timer: 0 };
	  const remaining = 10_000 - (now - previous.lastAt);
	  if (remaining > 0) {
		if (!previous.timer) {
		  previous.timer = setTimeout(() => {
			previous.timer = 0;
			void handleSeasonProgress(detail);
		  }, remaining);
		}
		state.seasonProgressRefreshes.set(refreshKey, previous);
		return false;
	  }
	  previous.lastAt = now;
	  state.seasonProgressRefreshes.set(refreshKey, previous);
	  const tabKey = tab.key;
	  const scrollRoot = document.getElementById("app-scroll");
	  const scrollTop = Number(scrollRoot?.scrollTop || 0);
	  await loadOverview(tab, true, false, false, true);
	  if (state.section !== overviewSectionForGroup(group) || activeTab(group)?.key !== tabKey) return false;
	  requestAnimationFrame(() => {
		if (scrollRoot?.isConnected) scrollRoot.scrollTop = scrollTop;
	  });
	  return true;
	}

	async function handleOverviewIncremental(detail) {
	  if (state.section !== "overview" || !detail || detail.type !== "historical-ranks") return false;
	  const group = overviewGroupForSection();
	  const tab = activeTab(group);
	  if (!tab?.data) return false;
	  const account = String(detail.account || "");
	  const tabAccount = String(tab.data?.player?.playerRef || tab.playerRef || "");
	  if (account && tabAccount && account !== tabAccount) return false;
	  const tabKey = tab.key;
	  const scrollRoot = document.getElementById("app-scroll");
	  const scrollTop = Number(scrollRoot?.scrollTop || 0);
	  await loadOverview(tab, true, false, false, true);
	  if (state.section !== overviewSectionForGroup(group) || activeTab(group)?.key !== tabKey) return false;
	  requestAnimationFrame(() => {
		if (scrollRoot?.isConnected) scrollRoot.scrollTop = scrollTop;
	  });
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
        button.dataset.playerGroup = group;
        button.setAttribute("aria-label", `当前玩家分组：${PLAYER_GROUPS[group]}，${count} 个玩家。展开切换分组`);
        button.setAttribute("aria-expanded", String(playerGroupMenuOpen));
        button.innerHTML = `<span class="player-group-dot is-${group}" aria-hidden="true"></span><span class="player-group-label">${PLAYER_GROUPS[group]}</span><span class="player-group-count">${count}</span>`;
      }
      if (menu) {
        const focusedGroup = menu.contains(document.activeElement) ? document.activeElement.dataset.playerGroup : "";
        menu.innerHTML = Object.keys(PLAYER_GROUPS).map(key => {
          const itemCount = playerGroupCount(key);
          const disabled = key !== "players" && itemCount === 0;
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
    let visibleTabs = state.tabs.filter((tab) => tabGroup(tab) === group);
    if (group === "players" && !connected()) visibleTabs = visibleTabs.filter((tab) => riotTab(tab));
    const indexedTabs = visibleTabs.map((tab) => ({ tab, index: state.tabs.indexOf(tab) }));
    workspace.tabs.innerHTML = indexedTabs.length ? indexedTabs.map(({ tab, index }) => {
      const selected = tab.key === state.activeTabs[group];
      const masked = state.settings.maskNames && !tab.current;
      const label = masked ? (tab.data?.player?.hidden ? "隐藏玩家" : `玩家 ${String(index).padStart(2, "0")}`) : tab.label;
      const icon = !masked && tab.icon ? assetIcon(assetPath("profile", tab.icon), "", "", false) : '<span class="player-tab-placeholder" aria-hidden="true">◉</span>';
      const selfBadge = tab.current ? '<span class="player-tab-self" data-tooltip="当前登录的召唤师" data-tooltip-size="compact" aria-hidden="true">★</span>' : "";
      return `<span class="player-tab-wrap${selected ? " is-active" : ""}${tab.current ? " is-self" : ""}" data-player-tab-wrap="${escapeHTML(tab.key)}" draggable="${!tab.current}"><button class="player-tab" type="button" role="tab" aria-selected="${selected}" tabindex="${selected ? 0 : -1}" data-player-tab="${escapeHTML(tab.key)}">${selfBadge}${icon}<span class="player-tab-copy"><span class="player-tab-name" data-tooltip="${escapeHTML(label)}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(label)}</span></span>${tab.loading ? '<span class="mini-loading" aria-label="正在读取"></span>' : ""}</button>${tab.current ? "" : `<button class="player-tab-close" type="button" aria-label="关闭 ${escapeHTML(label)}" data-close-player="${escapeHTML(tab.key)}">×</button>`}</span>`;
    }).join("") : `<span class="player-tab-skeleton">${group === "pro" ? "请选择职业选手账号…" : "暂无可用玩家页签"}</span>`;
    prepareImages(workspace.tabs);
    requestAnimationFrame(() => updatePlayerTabScrollControls(group, true));
  }

  function tabGroup(tab) {
    return tab?.group === "pro" ? "pro" : riotTab(tab) ? "kr" : "players";
  }

  function normalizePlayerTabContext(context = {}) {
    const group = context?.group === "pro" ? "pro" : context?.region === "kr" || context?.group === "kr" ? "kr" : "players";
    if (group !== "pro") return { group };
    return {
      group,
      proTeam: String(context.proTeam || "").trim().slice(0, 16),
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
    if (leftRegion === "kr" || rightRegion === "kr") return leftRegion === rightRegion;
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
    if (tabGroup(tab) !== "pro") return "";
    const label = [tab.proTeam, tab.proPlayer].filter(Boolean).join(" ");
    return label ? `<span class="pro-identity-chip">${escapeHTML(label)}</span>${tab.proMismatch ? '<span class="pro-verification-warning">该账号与公开来源记录不一致</span>' : ""}` : "";
  }

  function summonerRegionChip(tab) {
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
      return Array.isArray(parsed) ? parsed.filter((item) => typeof item === "string" && item.length <= 240).slice(0, 64) : [];
    } catch (_) {
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
    const scope = region === "kr" ? "kr" : `cn:${tabServerID(tab) || "current"}`;
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
    const normalizedGroup = group === "pro" ? "pro" : region === "kr" ? "kr" : "players";
    return state.tabs.find((tab) => tabGroup(tab) === normalizedGroup && sameTabServerScope(tab, region, serverId) && (
      (normalizedRef && (tab.playerRefs?.has(normalizedRef) || tab.playerRef === normalizedRef || tab.data?.player?.playerRef === normalizedRef))
      || (normalizedName && sameRiotID(tab, normalizedName, tagLine))
    )) || null;
  }

  function savePlayerScroll() {
    if (state.section !== "overview") return;
    const tab = activeTab();
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
  window.addEventListener("deep-legends:before-section", savePlayerScroll);

  function selectPlayerTab(key) {
    const tab = state.tabs.find((item) => item.key === key);
    if (!tab) return;
    const group = tabGroup(tab);
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
    if (!tab.data && !tab.loading) loadOverview(tab);
    else renderOverview(group);
  }

  // 两个服务器的玩家互不相通：新页签/覆盖层继承来源页面的服务器标签，
  // 韩服总览里点到的玩家一定按韩服查询，国服页面同理。
  function rememberActiveTab(group) {
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
    renderOverview(tabContext.group);
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
    renderOverview(tabContext.group);
    loadOverview(tab);
  }

  /* ---------- 覆盖层：非总览页内查看玩家总览，左上角返回上一层 ---------- */

  function openPlayerOverlay(init) {
    const top = state.overlay[state.overlay.length - 1];
    if (top && init.playerRef && top.playerRef === init.playerRef && (top.serverId || "") === (init.serverId || "")) return;
    if (top && init.riotId && top.riotId && top.riotId.gameName === init.riotId.gameName && top.riotId.tagLine === init.riotId.tagLine && (top.region || "") === (init.region || "") && (top.serverId || "") === (init.serverId || "")) return;
    const entry = {
      key: `overlay-${Date.now()}-${state.overlay.length}`, overlay: true, current: false,
      playerRef: "", region: "", serverId: "", label: "隐藏玩家", loading: false, data: null, error: "",
      ...newTabView(), ...init,
    };
    state.overlay.push(entry);
    renderOverlay();
    loadOverview(entry);
  }

  function overlayBack() {
    const closed = state.overlay.pop();
    if (closed) state.controllers.get(`overview:${closed.key}`)?.abort();
    renderOverlay();
  }

  function closeOverlay() {
    while (state.overlay.length) {
      const closed = state.overlay.pop();
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
      nodes.playerOverlayContent.innerHTML = "";
      return;
    }
    const entryChanged = nodes.playerOverlay.dataset.entryKey !== entry.key;
    nodes.playerOverlay.dataset.entryKey = entry.key;
    nodes.playerOverlay.hidden = false;
    const regionChip = `<span class="region-chip${riotTab(entry) ? " is-kr" : ""}" data-tooltip="${escapeHTML(tabServerTitle(entry))}" data-tooltip-size="compact">${escapeHTML(tabServerLabel(entry))}</span>`;
    const hiddenBadge = entry.data?.player?.hidden || entry.data?.player?.privateHistory ? '<span class="player-tab-hidden">隐藏战绩</span>' : "";
    nodes.playerOverlayTitle.innerHTML = `<strong data-tooltip="${escapeHTML(entry.label)}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(entry.label)}</strong>${regionChip}${hiddenBadge}${state.overlay.length > 1 ? `<small>第 ${state.overlay.length} 层</small>` : ""}`;
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
    const region = tab.region || "kr";
    const slug = `${tab.riotId.gameName}-${tab.riotId.tagLine || ""}`.replace(/-$/, "");
    return `https://op.gg/zh-cn/lol/summoners/${encodeURIComponent(region)}/${encodeURIComponent(slug)}`;
  }

  function closePlayerTab(key) {
    savePlayerScroll();
    const index = state.tabs.findIndex((tab) => tab.key === key && !tab.current);
    if (index < 0) return;
    state.controllers.get(`overview:${key}`)?.abort();
    state.controllers.get(`overview-more:${key}`)?.abort();
    state.controllers.get(`opgg-season:${key}`)?.abort();
    state.controllers.get(`current-game:${key}`)?.abort();
    const [closedTab] = state.tabs.splice(index, 1);
    const group = tabGroup(closedTab);
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
    renderOverview();
  }

  function renderOverview(group) {
    if (state.destroyed || state.section !== "overview") return;
    group = group || overviewGroupForSection();
    if (state.destroyed || state.section !== "overview" || group !== overviewGroupForSection()) return;
    const workspace = overviewWorkspace(group);
    for (const key of ["matchObserver", "proMatchObserver", "matchTierObserver", "proMatchTierObserver"]) { state[key]?.disconnect(); state[key] = null; }
    const tab = activeTab(group);
    // 通知外层：启动入口卡只在“当前召唤师”页签展示。
    window.dispatchEvent(new CustomEvent("deep-legends:overview-tab", { detail: { current: group === "players" && Boolean(tab?.current) } }));
    if (group !== "players" && !tab) {
      workspace.content.closest(".gameplay-panel")?.classList.remove("is-disconnected");
      workspace.content.innerHTML = emptyState("尚未打开职业选手账号", "请返回职业选手目录选择账号。", false);
      return;
    }
    const groupTabs = state.tabs.filter((item) => tabGroup(item) === group);
    // 职业账号与已打开的韩服玩家不依赖本机客户端。
    const panelUsable = group === "pro" ? Boolean(tab) : connected() || groupTabs.some((item) => riotTab(item));
    workspace.content.closest(".gameplay-panel")?.classList.toggle("is-disconnected", !panelUsable);
    if (!panelUsable) {
      // 客户端退出后不能留下空白页：给出明确的等待提示，并保留重试入口。
      workspace.content.innerHTML = emptyState("等待英雄联盟客户端", "国服召唤师数据需要本机客户端连接后读取。启动并登录客户端后会自动恢复；也可以在顶部搜索框切换到韩服直接查询玩家。", true);
      workspace.content.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => { const target = activeTab(group); if (target) loadOverview(target, true); });
      return;
    }
    if (!tabReady(tab)) {
      workspace.content.innerHTML = emptyState("等待英雄联盟客户端", "国服召唤师数据需要本机客户端连接后读取；已打开的韩服页签不受影响。", false);
      return;
    }
    renderOverviewBody(workspace.content, tab);
    if (tab.restoreScrollPending) restorePlayerScroll(tab);
  }

  // renderOverviewBody 把某个玩家页签（总览页签或覆盖层条目）的总览
  // 渲染到指定容器；总览页与覆盖层共用同一套展示与交互。

  function rankedQueueData(data, tab, scope = "recent") {
    const stateKey = { recent: "rankedQueueRecent", ability: "rankedQueueAbility", position: "rankedQueuePosition" }[scope] || "rankedQueueRecent";
    const key = String(tab?.[stateKey] || "420");
    const queue = data?.rankedQueues?.[key];
    if (!data?.rankedQueues) {
      // Older cached/demo payloads only contain the original combined fields.
      // Keep those fields on the default solo tab, but never reuse them for
      // flex: an empty flex tab is more honest than mixing queue statistics.
	  if (key === "440") return { recentRanked: { queueId: 440, queueLabel: "灵活组排" }, ability: null, abilitySampleGames: 0, queueGames: 0, positions: [], queueId: 440, queueLabel: "灵活组排", positionQueueId: 440, positionQueueLabel: "灵活组排" };
      const recentRanked = data?.recentRanked || {};
	  return { recentRanked, ability: data?.ability || null, abilitySampleGames: Number(data?.ability?.sampleGames || recentRanked.games || 0), queueGames: Number(recentRanked.games || 0), positions: data?.positions || [], queueId: Number(recentRanked.queueId || 420), queueLabel: recentRanked.queueLabel || "单双排", positionQueueId: Number(recentRanked.queueId || 420), positionQueueLabel: recentRanked.queueLabel || "单双排" };
    }
	if (!queue) return { recentRanked: { queueId: Number(key), queueLabel: Number(key) === 440 ? "灵活组排" : "单双排" }, ability: null, abilitySampleGames: 0, queueGames: 0, positions: [], queueId: Number(key), queueLabel: Number(key) === 440 ? "灵活组排" : "单双排", positionQueueId: Number(key), positionQueueLabel: Number(key) === 440 ? "灵活组排" : "单双排" };
	return { recentRanked: queue.recentRanked || {}, ability: queue.ability || null, abilitySampleGames: Number(queue.abilitySampleGames || queue.ability?.sampleGames || 0), queueGames: Number(queue.recentRanked?.games || 0), positions: queue.positions || [], queueId: Number(key), queueLabel: queue.recentRanked?.queueLabel || (Number(key) === 440 ? "灵活组排" : "单双排"), positionQueueId: Number(queue.positionQueueId || key), positionQueueLabel: queue.positionQueueLabel || rankedQueueLabel(Number(queue.positionQueueId || key)) };
  }

  function rankedQueueSwitcher(tab, activeQueue, scope = "recent") {
    const stateKey = { recent: "rankedQueueRecent", ability: "rankedQueueAbility", position: "rankedQueuePosition" }[scope] || "rankedQueueRecent";
    const selected = String(activeQueue || tab?.[stateKey] || "420");
    return `<div class="ranked-queue-switcher" role="group" aria-label="排位模式" data-ranked-queue-scope="${scope}"><button type="button" class="ranked-queue-button${selected === "420" ? " is-active" : ""}" data-ranked-queue="420" aria-pressed="${selected === "420"}">单双排</button><button type="button" class="ranked-queue-button${selected === "440" ? " is-active" : ""}" data-ranked-queue="440" aria-pressed="${selected === "440"}">灵活组排</button></div>`;
  }

  function careerSectionEntries(data, tab) {
    const recentHistoryCapability = (data.capabilities || []).find((item) => item.name === "seven-day-history");
    const recentQueue = rankedQueueData(data, tab, "recent");
    const abilityQueue = rankedQueueData(data, tab, "ability");
    const positionQueue = rankedQueueData(data, tab, "position");
    const hasSeason = data.seasonStatsProgress && !data.seasonStatsProgress.unavailable;
    const opggSeason = String(data.player?.region || "").toLowerCase() === "kr" && !data.player?.privateHistory && tab?.opggSeason?.playerRef === data.player?.playerRef ? tab.opggSeason.data : null;
    const missingKRSeason = String(data.player?.region || "").toLowerCase() === "kr" && !opggSeason;
    const championRows = opggSeason?.champions || (missingKRSeason ? [] : hasSeason ? (data.seasonChampionStats || []) : (data.championStats || []));
    const championOverall = opggSeason?.overall || (missingKRSeason ? {} : hasSeason ? (data.seasonOverall || {}) : (data.overall || {}));
    const championProgress = opggSeason ? { season: opggSeason.season, complete: true, message: `${tab.opggSeasonStale ? "缓存 · " : ""}${number(opggSeason.overall.games)} 场排位` } : missingKRSeason ? { seasonOnly: true, unavailable: true, message: data.player?.privateHistory ? "该玩家战绩不可公开查询" : tab?.opggSeasonPending || !tab?.opggSeasonAttemptRef ? "正在读取本赛季英雄统计…" : "本赛季英雄统计暂不可用，请刷新重试" } : data.seasonStatsProgress;
    return [
	  ["ranks", renderRanks(data.ranks || [], data.capabilities || [], data.historicalRanks || [], data.rankMilestones, data.seasonStatsProgress)],
	  ["recent-ranked", renderRecentRanked(recentQueue.recentRanked, recentQueue.queueId, rankedQueueSwitcher(tab, recentQueue.queueId, "recent"))],
	  ["ability", renderAbility(abilityQueue.ability, abilityQueue.queueLabel, rankedQueueSwitcher(tab, abilityQueue.queueId, "ability"), abilityQueue.abilitySampleGames, abilityQueue.queueGames)],
      ["champions", renderChampionStats(championRows, championOverall, championProgress)],
	  ["positions", renderPositionStats(positionQueue.positions, positionQueue.positionQueueId || positionQueue.queueId, rankedQueueSwitcher(tab, positionQueue.queueId, "position"), positionQueue.positionQueueLabel, positionQueue.queueGames)],
      ["masteries", renderMasteries(data.masteries || [])],
      ["recent-players", renderRecentPlayers(data.recentPlayers || [], recentHistoryCapability)],
      ["activity", `${renderActivity(data.activityHours || [])}${renderOverviewShareButton()}`],
    ];
  }

  function renderCareerSections(data, tab) {
    return careerSectionEntries(data, tab).map(([, markup]) => markup).join("");
  }

  function renderCareerDialogSections(data, tab) {
    const sections = new Map(careerSectionEntries(data, tab));
    const renderItem = (key) => `<div class="career-dialog-item is-${key}">${sections.get(key) || ""}</div>`;
    const primaryKeys = ["ranks", "champions", "activity"];
    const secondaryKeys = ["recent-ranked", "ability", "positions", "masteries", "recent-players"];
    return `<div class="career-dialog-stack is-primary">${primaryKeys.map(renderItem).join("")}</div>
      <div class="career-dialog-stack is-secondary">${secondaryKeys.map(renderItem).join("")}</div>`;
  }

  // 渲染期一旦抛异常（例如上游多出一个没预料到的字段形状），容器会永远停在
  // 骨架屏上，用户只能看到“一直在加载”。这里兜住异常并切到可重试的错误态，
  // 保证任何单点渲染故障都不会把整页锁死。
  function renderOverviewBody(container, tab) {
    try {
      renderOverviewBodyContent(container, tab);
    } catch (error) {
      console.error("总览渲染失败", error);
      container.innerHTML = emptyState("总览渲染失败", `页面渲染时出现异常：${error?.message || error}`, true);
      container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadOverview(tab, true));
    }
  }

  function renderOverviewBodyContent(container, tab) {
    const tierScope = matchTierScope(tab);
    // 主总览和覆盖层容器都会被后续页签复用。异步结果只允许写回
    // 当前仍属于同一页签和玩家的容器。
    container.dataset.matchTierScope = tierScope;
    if (!tab.data && !tab.error) {
      container.innerHTML = '<div class="gameplay-skeleton"><span></span><span></span><span></span><span></span></div>';
      return;
    }
    if (tab.error) {
      const opggEscape = tab.riotId && riotTab(tab) ? '<div class="opgg-escape"><button class="text-button" type="button" data-open-opgg>在 OP.GG 中查看该玩家 ↗</button></div>' : "";
      container.innerHTML = (tab.proMismatch ? summonerProChip(tab) : "") + emptyState("战绩读取失败", tab.error, true) + opggEscape;
      container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadOverview(tab, true));
      container.querySelector("[data-open-opgg]")?.addEventListener("click", () => window.open(opggSummonerURL(tab), "_blank", "noopener"));
      return;
    }
    const data = tab.data;
    const player = data.player || {};
    const rawMatches = data.matches || [];
    const matches = filteredMatches(rawMatches, tab);
    const retainedMatchList = container.querySelector(".match-list");
    const preserveMatchList = Boolean(retainedMatchList
      && ((container._matchListMatches === rawMatches && container._matchListFilter === tab.matchFilter) || tab.filterPaging)
      && container._matchListViewRevision === Number(tab.matchViewRevision || 0));
    if (preserveMatchList) retainedMatchList.remove();
    const playerIndex = Math.max(1, state.tabs.findIndex((item) => item.key === tab.key));
    const maskProfile = state.settings.maskNames && !player.isCurrent;
    const profileName = maskProfile ? (player.hidden ? "隐藏玩家" : `玩家 ${String(playerIndex).padStart(2, "0")}`) : (player.gameName || player.displayName || "隐藏玩家");
    const profileIcon = maskProfile ? maskedProfileIcon("summoner-avatar") : iconFigure("profile", player.profileIconId, playerLabel(player), "summoner-avatar");
    const historyCapability = (data.capabilities || []).find((item) => item.name === "match-history");
    const matchDetailsCapability = (data.capabilities || []).find((item) => item.name === "match-details");
    const matchDetailsWarning = !riotTab(tab) && ["failed", "unsupported"].includes(matchDetailsCapability?.state)
      ? `<div class="notice is-warning match-details-warning" role="alert"><div class="notice-symbol" aria-hidden="true">!</div><div><strong>完整对局参与者读取不完整，部分场次只能显示接口实际返回的玩家。</strong><p>${escapeHTML(matchDetailsCapability.detail || "SGP 完整对局数据暂时不可用。")}</p></div></div>`
      : "";
    const pagination = data.pagination || { hasMore: false };
    const specialModeEmpty = tab.matchFilter === "more:special"
      ? "自定义对局不会展示；这里只保留客户端实际返回的其他特殊模式。"
      : "可以切换上方游戏类型，或刷新读取最新战绩。";
    const emptyMatches = matchListEmptyContent(tab, historyCapability, specialModeEmpty);
    const sentinelHidden = matchSentinelShouldHide(tab, matches.length > 0) ? " hidden" : "";
    const proChip = !maskProfile ? summonerProChip(tab) : "";
    const regionChip = summonerRegionChip(tab);
    const playerTag = !maskProfile && player.tagLine ? `<span>#${escapeHTML(player.tagLine)}</span>` : "";
    const nameMeta = playerTag || proChip ? `<div class="summoner-name-meta">${playerTag}${proChip}</div>` : "";
    const hiddenChip = player.hidden || player.privateHistory ? '<span class="player-tab-hidden" data-tooltip="该玩家在客户端里开启了隐藏战绩" data-tooltip-size="compact">隐藏战绩</span>' : "";
    const backgroundArt = window.deepLegendsOverviewArt?.render(player) || (player.backgroundSource && player.backgroundPath
      ? `<img class="summoner-strip-art" src="/api/champion-asset?source=${encodeURIComponent(player.backgroundSource)}&path=${encodeURIComponent(player.backgroundPath)}" alt="" aria-hidden="true" decoding="async" data-game-image>`
      : "");
    const highlights = renderSummonerHighlights(data.ranks || [], data.masteries || []);
    const paginationCopy = paginationCopyFor(tab);
    const careerSections = renderCareerSections(data, tab);
    container.innerHTML = `
      <section class="summoner-strip">
        ${backgroundArt}
        ${profileIcon}
        <div class="summoner-strip-copy"><div><h2 data-tooltip="${escapeHTML(profileName)}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(profileName)}</h2>${nameMeta}</div><p class="summoner-level-row"><span>召唤师等级 ${number(player.summonerLevel)}${player.hidden ? " · 身份已隐藏" : ""}</span>${regionChip}${hiddenChip}</p></div>
        ${highlights}
      </section>
      <div class="overview-layout">
        <aside class="career-column" aria-label="生涯统计">
          ${careerSections}
        </aside>
        <section class="matches-column" aria-label="最近对局">
          <div class="overview-career-entry"><button class="career-dialog-trigger" type="button" aria-haspopup="dialog" aria-controls="career-dialog" data-open-career-dialog><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9m5 10V5m5 14v-7m5 7V8"/></svg><span>生涯统计</span></button></div>
          ${renderMatchFilters(tab)}
          ${matchDetailsWarning}
          ${riotTab(tab) ? '<div data-current-game hidden></div>' : ""}
          <div class="match-list">${preserveMatchList ? "" : (matches.length ? matches.map((match) => renderMatch(match, player.playerRef, tab)).join("") : emptyMatches)}</div>
          <div class="match-pagination${tab.loadingMore ? " is-loading" : ""}" data-match-sentinel aria-live="polite"${sentinelHidden}>${paginationCopy}</div>
        </section>
      </div>`;
    if (tab.loading) {
      container.insertAdjacentHTML("beforeend", '<div class="overview-refreshing" role="status"><span class="mini-loading" aria-hidden="true"></span><span>正在刷新最新战绩，当前内容仍可查看</span></div>');
    }
    bindOverviewContent(container, tab);
    applyRenderedMetricStyles(container);
    prepareImages(container);
    if (preserveMatchList) {
      container.querySelector(".match-list")?.replaceWith(retainedMatchList);
      reconcileFilteredMatchList(retainedMatchList, tab);
    }
    const renderedList = container.querySelector(".match-list");
    if (renderedList) renderedList._matchData = new Map(rawMatches.map(match => [String(match.gameId), match]));
    container._matchListMatches = rawMatches;
    container._matchListFilter = tab.matchFilter;
    container._matchListViewRevision = Number(tab.matchViewRevision || 0);
    scheduleOverviewCurrentGame(tab);
    if ((data.matches || []).length || tab.currentGame?.data?.status === "active") { ensurePerks(); ensureItems(); ensureSummonerSpells(); observeMatchTierVisibility(container, tab, tierScope); }
  }

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
      const name = mask ? `玩家 ${index + 1}` : player.gameName || player.displayName || "隐藏玩家";
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
      const identity = player.playerRef && !mask ? `<button type="button" data-current-game-player="${escapeHTML(player.playerRef)}" data-label="${escapeHTML(name + tag)}">${escapeHTML(name)}</button>` : `<strong>${escapeHTML(name)}</strong>`;
      return `<article class="current-game-player"><div class="current-game-player-line">${iconFigure("champion", player.championId, player.championName, "current-game-champion", false)}<div class="current-game-spells">${(player.spells || []).map(id => spellIconFigure(id, "small")).join("")}</div><div class="current-game-runes">${(player.runes || []).map((id, i) => i === 1 ? perkStyleIconFigure(id, "small") : perkIconFigure(id, "small")).join("")}</div><div class="current-game-identity">${identity}<small>${player.summonerLevel > 0 ? `Lv.${number(player.summonerLevel)}` : "等级未提供"}</small></div></div>${recent ? `<div class="current-game-recent" aria-label="近期对局">${recent}</div>` : ""}<div class="current-game-badges">${positionIcon}${streak}</div><div class="current-game-rank">${rank}</div></article>`;
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
    if (!riotTab(tab)) return;
    let traceID = tab?.currentGameTrace;
    const report = (reason, fields = {}) => { if (typeof window !== "undefined") window.reportFlowDiagnostic?.("current_game_client", reason, { traceId: traceID, forceRefresh: force, ...fields }); };
    const ref = tab?.data?.player?.playerRef;
    if (!ref || tab.data.player.privateHistory && riotTab(tab) || tab.externalRender || state.destroyed) { report("gated", { gate: !ref ? "no-reference" : state.destroyed ? "destroyed" : tab.externalRender ? "external-render" : "private-kr" }); return; }
    if (tab.currentGamePending === ref) { report("in-flight"); return; }
    if (!force && tab.currentGame?.ref === ref && Date.now() - tab.currentGame.at < 30_000) { report("cached", { cacheAgeMs: Date.now() - tab.currentGame.at, phase: tab.currentGame.error ? "error" : tab.currentGame.data?.status }); return; }
    tab.currentGamePending = ref;
    state.currentGameTraceSequence = ((state.currentGameTraceSequence || 0) + 1) % 1000000;
    tab.currentGameTrace = `cg-${Date.now()}-${state.currentGameTraceSequence}`;
    traceID = tab.currentGameTrace;
    const started = Date.now();
    report("request", { forceRefresh: force });
    try {
      const data = await api("/api/gameplay/current-game", { method: "POST", body: JSON.stringify({ playerRef: ref, traceId: tab.currentGameTrace, forceRefresh: force }) }, `current-game:${tab.key}`, 14_000);
      if (!["active", "none", "unsupported"].includes(data?.status) || data.status === "active" && !Array.isArray(data.teams)) {
        report("invalid-response");
        const error = new Error("当前对局响应无效");
        error.errorKind = "invalid-response";
        throw error;
      }
      if (tab.data?.player?.playerRef !== ref || state.destroyed) { report("stale", { gate: state.destroyed ? "destroyed" : "reference-changed" }); return; }
      tab.currentGame = { ref, at: Date.now(), data, traceId: traceID, forceRefresh: force };
      report("received", { phase: data.status, source: data.source, teamsReceived: (data.teams || []).length, durationMs: Date.now()-started, playersReceived: (data.teams || []).reduce((sum, team) => sum + (team.players || []).length, 0) });
      if (data.status === "active") { ensurePerks(); ensureSummonerSpells(); }
    } catch (error) {
      report(error.name === "RequestCancelled" ? "canceled" : "failed", { durationMs: Date.now()-started, httpStatus: error.status || 0, errorKind: error.errorKind || "other" });
      if (error.name !== "RequestCancelled" && tab.data?.player?.playerRef === ref && !state.destroyed) tab.currentGame = { ref, at: Date.now(), error: true, traceId: traceID, forceRefresh: force };
    } finally {
      if (tab.currentGamePending === ref) tab.currentGamePending = "";
      updateCurrentGameCard(tab);
    }
  }
  function scheduleOverviewCurrentGame(tab) {
    if (!riotTab(tab)) { clearTimeout(state.currentGameTimer); return; }
    updateCurrentGameCard(tab);
    if (tab.overlay || tab.externalRender || state.section !== "overview" || activeTab() !== tab) return;
    clearTimeout(state.currentGameTimer);
    void loadOverviewCurrentGame(tab);
    void loadOPGGSeasonSummary(tab);
    state.currentGameTimer = setTimeout(() => {
      if (!state.destroyed && state.section === "overview" && activeTab() === tab && !document.hidden) scheduleOverviewCurrentGame(tab);
    }, 30_000);
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

  function renderSummonerHighlights(ranks, masteries) {
    const highestRank = highestCurrentRank(ranks);
    const mastery = (masteries || []).reduce((best, item) => Number(item?.championPoints || 0) > Number(best?.championPoints || 0) ? item : best, null);
    // 客户端只导出了 crest-and-banner-mastery-0 ~ -10 这 11 张徽章图
    // （已核对 CommunityDragon 的 rcp-fe-lol-collections/.../item-element/ 目录），
    // 所以 11 级及以上复用 10 级外观，真实等级由徽章下方的 Lv 牌显示。
    const masteryLevel = Math.max(0, Math.min(10, Math.floor(Number(mastery?.championLevel) || 0)));
    const masteryCrestPath = `/latest/plugins/rcp-fe-lol-collections/global/default/images/item-element/crest-and-banner-mastery-${masteryLevel}.png`;
    const masteryTooltip = mastery ? `${mastery.championName || "英雄"} · 熟练度 ${number(mastery.championLevel)} 级\n${number(mastery.championPoints)} 熟练度点数` : "";
    // 两列都不再顶一行"最高熟练度 / 最高段位·单排/双排"的小标签：召唤师条高度
    // 是稀缺资源（用户反复反馈过太高），而这两块的图形本身已经说明了身份——
    // 英雄头像+熟练度徽章、段位徽章都是一眼可辨的。段位列改成把队列名当主标题、
    // 段位与胜点合成一行，等于用原来标签那一行的高度换来了信息密度。
    const masteryMarkup = mastery ? `<figure class="summoner-highlight is-mastery" data-tooltip="${escapeHTML(masteryTooltip)}" data-tooltip-size="compact">
      <span class="summoner-mastery-portrait" data-mastery-level="${masteryLevel}">${iconFigure("champion", mastery.championId, mastery.championName, "summoner-mastery-icon", false)}<img class="summoner-mastery-crest" src="/api/champion-asset?source=communitydragon&path=${encodeURIComponent(masteryCrestPath)}" alt="" aria-hidden="true" loading="lazy" decoding="async" data-game-image></span>
      <figcaption><strong>${escapeHTML(mastery.championName || "英雄")}</strong><small>Lv.${number(mastery.championLevel)} · ${compactNumber(mastery.championPoints)} 点</small></figcaption>
    </figure>` : "";
    const queueLabel = highestRank?.queueLabel || (highestRank?.queueType === "RANKED_FLEX_SR" ? "灵活组排" : "单排/双排");
    const rankTooltip = highestRank ? `最高段位 · ${queueLabel}\n${rankTitle(highestRank)} · ${number(highestRank.leaguePoints)} LP` : "";
    const rankMarkup = highestRank ? `<figure class="summoner-highlight is-rank" data-tooltip="${escapeHTML(rankTooltip)}" data-tooltip-size="compact">
      <span class="summoner-highlight-rank" aria-hidden="true">${rankCrestIcon(highestRank.tier)}</span>
      <figcaption><strong>${escapeHTML(queueLabel)}</strong><small>${escapeHTML(rankTitle(highestRank))} ${number(highestRank.leaguePoints)} LP</small></figcaption>
    </figure>` : "";
    if (!masteryMarkup && !rankMarkup) return "";
    return `<div class="summoner-strip-highlights">${masteryMarkup}${rankMarkup}</div>`;
  }

  function renderHistoricalRanks(items) {
    const rows = (items || []).filter((item) => item?.season && item?.tier).map((item, index) => {
      const winRateKnown = item.winRate !== null && item.winRate !== undefined && Number(item.winRate) >= 0;
      const winRate = winRateKnown ? percent(item.winRate) : `<span data-tooltip="OP.GG 的历史赛段记录未提供胜负场，无法核验胜率" data-tooltip-size="compact">未提供</span>`;
      const leaguePoints = item.leaguePoints === null || item.leaguePoints === undefined ? "—" : number(item.leaguePoints);
      return `<div class="rank-history-row"${index >= 5 ? " data-rank-history-extra hidden" : ""}><span class="rank-season">${escapeHTML(item.season)}</span><span class="rank-history-tier"><span class="rank-history-crest" aria-hidden="true">${rankCrestIcon(item.tier)}</span><b>${escapeHTML(rankTitle(item))}</b><span>${leaguePoints} LP</span></span><b class="rank-history-winrate">${winRate}</b></div>`;
    }).join("");
    if (!rows) return "";
    const expandable = items.filter((item) => item?.season && item?.tier).length > 5;
    const toggle = expandable ? `<button class="rank-history-toggle" type="button" aria-expanded="false" data-rank-history-toggle><span>查看更多赛段段位</span><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m7 10 5 5 5-5"/></svg></button>` : "";
    return `<div class="rank-history"><div class="rank-history-head"><span>赛段</span><span>段位与 LP</span><span>胜率</span></div><div class="rank-history-list">${rows}</div>${toggle}</div>`;
  }

  function renderRankMilestones(milestones) {
    const items = [];
    if (milestones?.peakTier) {
      const peak = { tier: milestones.peakTier, division: milestones.peakDivision };
      items.push(`<div class="rank-milestone"><span class="rank-milestone-crest" aria-hidden="true">${rankCrestIcon(peak.tier)}</span><div><small>历史最高</small><strong>${escapeHTML(rankTitle(peak))}</strong></div></div>`);
    }
    const previous = (milestones?.previousSeason || []).find((item) => item?.queueType === "RANKED_SOLO_5x5" && item?.tier);
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
    const value = Number(metric?.[key]);
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
	  const title = Number(sampleGames) > 0
		  ? `近期对局中${queueLabel}样本不足（需 ≥3 场，当前 ${Number(sampleGames)} 场）`
		  : playedQueue ? `暂无足够的${queueLabel}同位置样本` : `当前最近对局未发现${queueLabel}样本`;
	  const detail = "基于首屏最近对局，至少需要 3 场包含完整参与者数据的排位对局。";
      return `<section class="career-section ability-section"><header><h3>能力表现</h3><div class="career-section-tools">${tools}</div></header><div class="ability-unavailable"><strong>${escapeHTML(title)}</strong><small>${escapeHTML(detail)}</small></div></section>`;
    }
    const rings = [25, 50, 75, 100].map((score) => `<polygon points="${radarPoints(metrics, () => score)}"/>`).join("");
    const axes = metrics.map((_, index) => {
      const [x, y] = radarPoint(index, 100, metrics.length);
      return `<line x1="160" y1="144" x2="${x.toFixed(1)}" y2="${y.toFixed(1)}"/>`;
    }).join("");
    const markers = metrics.map((metric, index) => {
      const [x, y] = radarPoint(index, metric.playerScore, metrics.length);
      return `<circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="4"/>`;
    }).join("");
    const baselineLabel = ability.baselineLabel || "近期同位置对手样本";
    const controls = metrics.map((metric, index) => {
      const tooltip = `${metric.label}\n当前玩家 ${abilityMetricValue(metric, "player")} · ${baselineLabel} ${abilityMetricValue(metric, "baseline")}\n${metric.description || ""}`;
      return `<button class="ability-radar-metric is-metric-${index}" type="button" aria-label="${escapeHTML(tooltip.replaceAll("\n", "，"))}" data-tooltip="${escapeHTML(tooltip)}" data-tooltip-size="compact"><b>${escapeHTML(metric.grade)}</b><span>${escapeHTML(metric.label)}</span></button>`;
    }).join("");
    const summary = `${number(ability.sampleGames)} 场当前玩家 · ${number(ability.baselineGames)} 场同位置对手`;
    const sourceDetail = `${ability.sourceLabel || "七项指标参考 OP.GG"}\n${summary}\n对手基准来自这名玩家排位中的同位置对手样本聚合，不是全服或段位平均`;
    return `<section class="career-section ability-section"><header><h3>能力表现</h3><div class="career-section-tools">${tools}</div></header><div class="ability-meta"><div class="ability-legend"><span><i class="is-player"></i>当前玩家</span><span><i class="is-baseline"></i>${escapeHTML(baselineLabel)}</span></div><span data-tooltip="${escapeHTML(sourceDetail)}" data-tooltip-size="compact">${escapeHTML(`${ability.positionLabel ? `${ability.positionLabel} · ` : ""}${number(ability.sampleGames)} 场样本`)}</span></div><div class="ability-radar-wrap"><svg class="ability-radar" viewBox="0 0 320 275" aria-hidden="true" focusable="false"><g class="ability-radar-grid">${rings}${axes}</g><polygon class="ability-radar-baseline" points="${radarPoints(metrics, () => 62)}"/><polygon class="ability-radar-player" points="${radarPoints(metrics, (metric) => metric.playerScore)}"/><g class="ability-radar-points">${markers}</g></svg><div class="ability-radar-controls" role="group" aria-label="当前玩家与近期同位置对手样本的七维能力数据">${controls}</div></div></section>`;
  }

	function renderRecentRanked(stats, queueId = 0, queueSwitcher = "") {
	  const games = Number(stats?.games || 0);
	  const heading = `近 ${number(Math.min(20, games))} 场排位`;
	const tools = queueSwitcher;
    if (!games) {
      const queueLabel = stats?.queueLabel || (queueId === 440 ? "灵活组排" : "单双排");
	  const title = `当前样本未发现${queueLabel}对局`;
	  const detail = "基于首屏已加载的最近对局，最多统计 20 场。";
	  return `<section class="career-section recent-ranked-section"><header><h3>${heading}</h3><div class="career-section-tools">${tools}</div></header><div class="ability-unavailable"><strong>${escapeHTML(title)}</strong><small>${escapeHTML(detail)}</small></div></section>`;
    }
    const participationKnown = Number(stats.killParticipationGames || 0) > 0;
    const participation = participationKnown ? percent(stats.killParticipation) : "—";
    const participationTooltip = participationKnown
      ? `基于 ${number(stats.killParticipationGames)} 场含完整队伍击杀的数据计算`
      : "现有对局没有返回完整队伍击杀，暂不估算参团率";
    const positions = (stats.positions || []).slice(0, 2).map((item) => `<span class="recent-ranked-position is-${escapeHTML(item.position || "other")}" data-tooltip="${escapeHTML(`${number(item.games)} 场 · ${number(item.wins)} 胜`)}" data-tooltip-size="compact"><b>${escapeHTML(item.label || positionLabel(item.position))}</b><strong>${percent(item.winRate)}</strong></span>`).join("");
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
      <div class="recent-ranked-positions"><span>位置胜率</span><div>${positions || "<small>位置数据不足</small>"}</div></div>
    </section>`;
  }

  function renderRanks(ranks, capabilities, historicalRanks, rankMilestones, seasonProgress = null) {
    const rankedCapability = capabilities.find((item) => item.name === "ranked-stats");
    if (rankedCapability?.state === "unsupported") {
      return `<section class="career-section"><header><h3>排位</h3><span>能力边界</span></header><div class="rank-unavailable"><strong>${escapeHTML(rankedCapability.detail || "跨服暂不支持排位")}</strong><small>当前客户端的排位接口只能读取登录服务器。</small></div></section>`;
    }
    const expected = ["RANKED_SOLO_5x5", "RANKED_FLEX_SR"];
    const rows = expected.map((queue) => {
      const rank = ranks.find((item) => item.queueType === queue);
      const label = queue === "RANKED_SOLO_5x5" ? "单排/双排" : "灵活组排";
      if (!rank || !rank.tier) return `<div class="rank-row"><span class="rank-crest is-unranked" aria-hidden="true">◇</span><div><strong>${label}</strong><small>尚未定级或客户端未提供</small></div><b>Unranked</b></div>`;
      const recordKnown = Number(rank.winRate) >= 0;
      const collecting = !recordKnown && seasonProgress?.collecting === true;
      const record = recordKnown ? `${number(rank.wins)}胜 ${number(rank.losses)}负` : collecting ? `${number(rank.wins)}胜 · 正在统计中` : `${number(rank.wins)}胜 · 负场未提供`;
	  const unavailableDetail = collecting ? "正在后台按当前队列统计本赛季战绩，完成后会自动更新胜率" : (rankedCapability?.detail || "上游未提供负场，无法计算胜率");
	  return `<div class="rank-row"><span class="rank-crest" aria-hidden="true">${rankCrestIcon(rank.tier)}</span><div><strong>${label}</strong><small>${escapeHTML(rankTitle(rank))} · ${number(rank.leaguePoints)} LP</small></div><div class="rank-record"><b>${record}</b><span${recordKnown ? "" : ` data-tooltip="${escapeHTML(unavailableDetail)}" data-tooltip-size="compact"`}>${recordKnown ? `胜率 <b class="win-rate-value">${percent(rank.winRate)}</b>` : collecting ? "正在统计中" : "胜率暂不可用"}</span></div></div>`;
    }).join("");
    return `<section class="career-section"><header><h3>排位</h3><span>当前赛季</span></header><div class="rank-list">${rows}</div>${renderRankHistory(historicalRanks, rankMilestones)}</section>`;
  }

  function renderChampionStats(items, overall, progress) {
    if (progress?.seasonOnly && progress.unavailable) return `<section class="career-section champion-performance"><header><h3>英雄胜率</h3><span>本赛季</span></header><p class="section-empty">${escapeHTML(progress.message)}<br>不会用最近 20 场代替赛季数据。</p></section>`;
    const rankedGames = (items || []).reduce((total, item) => total + Number(item.games || 0), 0);
    const overallRow = `<div class="champion-stat-row is-overall"><span class="overall-champion-mark" aria-hidden="true">全</span><div><strong>全部英雄</strong><small>CS ${number(overall.cs)} (${number(overall.csPerMinute)})</small></div><div><b>${kda(overall.kda)}:1 KDA</b><small>${number(overall.kills)} / ${number(overall.deaths)} / ${number(overall.assists)}</small></div><div><b class="win-rate-value">${percent(overall.winRate)}</b><small>${number(overall.games)} 场</small></div></div>`;
    const rows = items.slice(0, 8).map((item) => `<div class="champion-stat-row">${iconFigure("champion", item.championId, item.championName)}<div><strong>${escapeHTML(item.championName)}</strong><small>CS ${number(item.cs)} (${number(item.csPerMinute)})</small></div><div><b>${kda(item.kda)}:1 KDA</b><small>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}</small></div><div><b class="win-rate-value">${percent(item.winRate)}</b><small>${number(item.games)} 场</small></div></div>`).join("");
    const label = progress?.message ? `${progress.season ? `${escapeHTML(progress.season)} · ` : ""}${escapeHTML(progress.message)}` : (progress?.complete ? `本赛季 · ${number(rankedGames)} 场排位` : `已统计 ${number(rankedGames)} 场`);
    return `<section class="career-section champion-performance"><header><h3>英雄胜率</h3><span>${label}</span></header><div>${overallRow}${rows || '<p class="section-empty">暂无英雄统计</p>'}</div></section>`;
  }

  function renderPositionStats(items, queueId = 0, queueSwitcher = "", sourceLabel = "", queueGames = 0) {
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
	const sampleLabel = Number(queueGames) > 0 ? `<span class="career-sample-label">近 ${number(Math.min(20, queueGames))} 场</span>` : "";
	const tools = `${sampleLabel}${queueSwitcher}`;
    const hasSamples = [...values.values()].some((item) => Number(item?.games || 0) > 0 || Number(item?.share || 0) > 0);
    if (!hasSamples) {
      const queueLabel = sourceLabel || (Number(queueId) === 440 ? "灵活组排" : "单双排");
	  const title = `暂无可统计的${queueLabel}位置样本`;
	  const detail = "位置偏好基于首屏已加载的最近排位对局。";
	  return `<section class="career-section"><header><h3>位置偏好</h3><div class="career-section-tools">${tools}</div></header><div class="ability-unavailable"><strong>${escapeHTML(title)}</strong><small>${escapeHTML(detail)}</small></div></section>`;
    }
    const rows = positions.map(([position, label]) => {
      const share = Math.max(0, Math.min(100, Number(values.get(position)?.share) || 0));
      return `<div class="position-row"><span>${positionIcon(position)}</span><div><strong>${label}</strong><progress max="100" value="${share}">${percent(share)}</progress></div><b>${percent(share)}</b></div>`;
    }).join("");
	return `<section class="career-section"><header><h3>位置偏好</h3><div class="career-section-tools">${tools}</div></header><div class="position-list">${rows}</div></section>`;
  }

  function renderMasteries(items) {
    const rows = items.map((item) => `<div class="mastery-row">${iconFigure("champion", item.championId, item.championName, "", false)}<div><strong>${escapeHTML(item.championName)}</strong><small>${compactNumber(item.championPoints)} 熟练度</small></div><b>Lv.${number(item.championLevel)}</b></div>`).join("");
    return `<section class="career-section"><header><h3>英雄熟练度</h3><span>最高分</span></header><div class="mastery-list">${rows || '<p class="section-empty">客户端未提供熟练度</p>'}</div></section>`;
  }

  function renderRecentPlayers(items, capability) {
    const rows = items.map((item, index) => {
      const label = maskedListName(item, index);
      const icon = state.settings.maskNames ? maskedProfileIcon() : iconFigure("profile", item.profileIconId, "");
      return `<button class="recent-player" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"} data-tooltip="${escapeHTML(label)}" data-tooltip-overflow=".recent-player-name" data-tooltip-size="compact">${icon}<span><strong class="recent-player-name">${escapeHTML(label)}</strong><small>共同对局 ${number(item.games)} 场</small></span><span aria-hidden="true">›</span></button>`;
    }).join("");
    const emptyCopy = capability?.state === "failed"
      ? escapeHTML(capability.detail || "最近 30 天的参与者数据不完整，暂时无法生成可靠结果。")
      : "暂无重复同场玩家";
    return `<section class="career-section"><header><h3>最近一起玩</h3><span>最近 30 天</span></header><div class="recent-player-list">${rows || `<p class="section-empty">${emptyCopy}</p>`}</div></section>`;
  }

  function renderActivity(hours) {
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
    const playerName = String(player.gameName || player.displayName || "overview")
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
      button.addEventListener("click", () => generateOverviewShare(container, tab));
    }
  }

  function closeCareerDialog() {
    if (nodes.careerDialog?.open) nodes.careerDialog.close();
  }

  function renderCareerDialogContent(data, tab, layout) {
    if (!nodes.careerDialogContent || nodes.careerDialogContent.dataset.careerLayout === layout) return;
    const scrollTop = nodes.careerDialogContent.scrollTop;
    nodes.careerDialogContent.innerHTML = layout === "single" ? renderCareerSections(data, tab) : renderCareerDialogSections(data, tab);
    nodes.careerDialogContent.dataset.careerLayout = layout;
    bindPlayerLinks(nodes.careerDialogContent, tab, closeCareerDialog);
    bindOverviewShareControls(nodes.careerDialogContent, overviewContainer(tab), tab);
    bindRankHistoryControls(nodes.careerDialogContent);
    bindRankedQueueControls(nodes.careerDialogContent, tab);
    applyRenderedMetricStyles(nodes.careerDialogContent);
    prepareImages(nodes.careerDialogContent);
    nodes.careerDialogContent.scrollTop = scrollTop;
  }

  function openCareerDialog(container, tab, opener) {
    if (!nodes.careerDialog || !nodes.careerDialogContent || !tab.data || nodes.careerDialog.open) return;
    const data = tab.data;
    const player = data.player || {};
    const playerIndex = Math.max(1, state.tabs.findIndex((item) => item.key === tab.key));
    const maskProfile = state.settings.maskNames && !player.isCurrent;
    const profileName = maskProfile ? (player.hidden ? "隐藏玩家" : `玩家 ${String(playerIndex).padStart(2, "0")}`) : (player.gameName || player.displayName || "隐藏玩家");
    const riotID = !maskProfile && player.tagLine ? `${profileName}#${player.tagLine}` : profileName;

    state.careerDialogOpener = opener;
    nodes.careerDialogContext.textContent = `${riotID} · ${tabServerLabel(tab)}`;
    nodes.careerDialogContent.dataset.careerLayout = "";

    const syncWidth = () => {
      const width = container.querySelector(".overview-layout")?.getBoundingClientRect().width || container.getBoundingClientRect().width;
      if (width <= 0) return;
      if (nodes.careerDialog.open && width > 1020) {
        closeCareerDialog();
        return;
      }
      const roundedWidth = Math.round(width);
      nodes.careerDialog.style.setProperty("--career-dialog-width", `${roundedWidth}px`);
      const dialogWidth = Math.min(roundedWidth, Math.max(0, window.innerWidth - 24));
      renderCareerDialogContent(data, tab, dialogWidth <= 640 ? "single" : "masonry");
    };
    state.careerDialogObserver?.disconnect();
    state.careerDialogObserver = "ResizeObserver" in window ? new ResizeObserver(syncWidth) : null;
    state.careerDialogObserver?.observe(container);
    syncWidth();
    nodes.careerDialog.showModal();
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
	return `<div class="match-filterbar"><div class="match-filter-tabs" role="group" aria-label="游戏类型">${direct.map(([value, label]) => `<button class="match-filter${tab.matchFilter === value ? " is-active" : ""}" type="button" aria-pressed="${tab.matchFilter === value}" data-match-filter="${value}">${label}</button>`).join("")}</div><div class="app-select match-more-filter${moreActive ? " is-active" : ""}" data-match-more-select><button class="app-select-trigger" type="button" aria-haspopup="menu" aria-expanded="false" data-app-select-trigger><span>${escapeHTML(triggerLabel)}</span></button><div class="app-select-menu" role="menu" aria-label="更多游戏类型" data-app-select-menu hidden>${options}</div></div></div>`;
  }

  function matchFilterDisplayLabel(tab) {
	const direct = { all: "全部", solo: "单排/双排", flex: "灵活组排", "hextech-aram": "海克斯大乱斗", arena: "斗魂竞技场" };
	const filter = String(tab?.matchFilter || "all");
	if (direct[filter]) return direct[filter];
	if (filter.startsWith("more:")) return moreModeOptions(tab).find(([key]) => filter === `more:${key}`)?.[1] || "所选模式";
	return "所选模式";
  }

  function filteredMatches(matches, tab) {
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
    if (filter === "all") return matches;
    if (filter.startsWith("more:")) {
      const key = filter.slice(5);
      const option = moreModeOptions(tab).find(([value]) => value === key);
      if (!option) return [];
      if (key === "aram") return matches.filter((match) => matchModeKind(match) === "aram");
      if (key === "hextech-qualifier") return matches.filter(isHextechQualifier);
      if (key === "hextech-classic") return matches.filter(isHextechClassic);
      if (option[2]) return matches.filter((match) => option[2].includes(Number(match.queueId)) || queueModeGroup(match) === key);
      if (key !== "special") return matches.filter((match) => queueModeGroup(match) === key);
      // 特殊模式只承接真正未识别的模式，不能再收纳带经典海克斯等
      // 明确语义、但后端旧缓存仍标成 other 的对局。
      return matches.filter((match) => !isRecognizedModeMatch(match));
    }
    // 兼容旧版 queue:ID 设置值。
    if (filter.startsWith("queue:")) return matches.filter((match) => String(match.queueId) === filter.slice(6));
    if (filter === "hextech-aram") return matches.filter(isOrdinaryHextechMatch);
    if (filter === "arena") return matches.filter((match) => matchModeKind(match) === "arena");
    return matches.filter((match) => match.modeGroup === filter);
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

  // 单场相对评分：KDA、参团率、伤害、金币、补刀、视野分别相对全场最高值归一后加权，映射到 2.0-10.0。
  // 数据缺失的维度（例如部分模式没有金币或视野）自动剔除并重新归一权重。
  function computeMatchScores(match) {
    const participants = match.participants || [];
    const scores = new Map();
    if (participants.length < 2) return scores;
    const minutes = Math.max(1, Number(match.duration || 0) / 60);
    const teamKills = new Map();
    for (const item of participants) teamKills.set(participantGroupKey(item), (teamKills.get(participantGroupKey(item)) || 0) + (Number(item.kills) || 0));
    const rows = participants.map((item, index) => ({
      item,
      index,
      values: {
        kda: ((Number(item.kills) || 0) + (Number(item.assists) || 0) * 0.8) / Math.max(1, Number(item.deaths) || 0),
        kp: ((Number(item.kills) || 0) + (Number(item.assists) || 0)) / Math.max(1, teamKills.get(participantGroupKey(item)) || 0),
        damage: Number(item.damage) || 0,
        gold: Number(item.gold) || 0,
        cs: Number(item.csPerMinute) || (Number(item.cs) || 0) / minutes,
        vision: Number(item.visionScore) || (Number(item.wardsPlaced) || 0) + (Number(item.wardsKilled) || 0),
      },
    }));
    const weights = { kda: 0.3, kp: 0.2, damage: 0.22, gold: 0.08, cs: 0.12, vision: 0.08 };
    const peaks = {};
    for (const key of Object.keys(weights)) peaks[key] = Math.max(...rows.map((row) => row.values[key]));
    const available = Object.keys(weights).filter((key) => peaks[key] > 0);
    const totalWeight = available.reduce((sum, key) => sum + weights[key], 0) || 1;
    for (const row of rows) {
      const raw = available.reduce((sum, key) => sum + weights[key] * (row.values[key] / peaks[key]), 0) / totalWeight;
      const rawScore = 2 + raw * 8;
      scores.set(Number(row.item.participantId), { score: Math.round(rawScore * 10) / 10, rawScore, rank: 0, total: 0, badge: "" });
    }
    // 与 OP.GG 一致：一位小数仅用于展示，名次按未舍入分数排序。
    // 极少数原始分也完全相同时按参与者原始顺序稳定落位，保持 1-N 唯一名次。
    const rankedRows = [...rows].sort((left, right) => {
      const scoreDelta = scores.get(Number(right.item.participantId)).rawScore - scores.get(Number(left.item.participantId)).rawScore;
      return scoreDelta || left.index - right.index;
    });
    for (const [index, row] of rankedRows.entries()) {
      const record = scores.get(Number(row.item.participantId));
      record.rank = index + 1;
      record.total = scores.size;
    }
    // MVP 给胜方最高分，SVP 给败方最高分；结果不明的对局不发徽章。
    const hasWin = rows.some((row) => row.item.win);
    const hasLoss = rows.some((row) => !row.item.win);
    if (hasWin && hasLoss) {
      for (const winSide of [true, false]) {
        let best = null;
        for (const row of rows) {
          if (Boolean(row.item.win) !== winSide) continue;
          if (!best || scores.get(Number(row.item.participantId)).rawScore > scores.get(best).rawScore) best = Number(row.item.participantId);
        }
        if (best != null) scores.get(best).badge = winSide ? "MVP" : "SVP";
      }
    }
    return scores;
  }

  function scoreBadgeChip(record) {
    if (!record?.badge) return "";
    return `<b class="match-badge-chip is-${record.badge.toLowerCase()}">${record.badge}</b>`;
  }

  function scoreChip(record) {
    if (!record) return "";
    const tone = record.score >= 8 ? " is-gold" : record.score >= 6.5 ? " is-good" : record.score < 5 ? " is-poor" : "";
    return `<i class="match-score${tone}" data-tooltip="单场相对评分\nKDA、参团、伤害、经济、补刀和视野按同场表现加权" data-tooltip-size="compact">${record.score.toFixed(1)}</i>`;
  }

  function scorePlacementChip(record) {
    const badge = scoreBadgeChip(record);
    if (badge) return badge;
    return Number(record?.rank) === 1 ? "" : scoreRankChip(record?.rank, record?.total);
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
      const size = participants.length % 3 === 0 ? 3 : 2;
      const groups = [];
      for (let index = 0; index < participants.length; index += size) groups.push({ subteamId: groups.length + 1, placement: 0, players: participants.slice(index, index + size) });
      return { arena: true, groups };
    }
    return { arena: false, groups: [100, 200].map((teamId) => ({ teamId, players: participants.filter((item) => item.teamId === teamId) })) };
  }

  function arenaGroupLabel(group) {
    const team = arenaTeamMeta(group);
    return group.placement > 0 ? `第 ${group.placement} 名 · ${team.name}` : team.name;
  }

  // 卡片右侧的玩家名单：普通模式两列（红蓝方，图标 + 名称）；
  // 斗魂竞技场按名次预览前四支队伍，完整队伍在展开详情中展示。
  function renderMatchPlayers(match) {
    const grouping = matchPlayerGroups(match);
    const playerButton = (item, index) => `<button type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"} data-tooltip="${escapeHTML(playerParticipantName(item, index))}" data-tooltip-overflow=".match-player-name" data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "tiny")}<span class="match-player-name">${escapeHTML(playerParticipantName(item, index))}</span></button>`;
    if (grouping.arena) {
      const rows = grouping.groups.slice(0, 4).map((group) => {
        const placement = Number(group.placement) || 0;
        const rankTone = placement >= 1 && placement <= 3 ? ` is-rank-${placement}` : "";
        const rank = `<b class="arena-rank-chip${rankTone}" aria-label="${placement ? `第 ${placement} 名` : "名次未知"}">${placement || "—"}</b>`;
        return `<div class="arena-team-row-compact${placement === 1 ? " is-first" : ""}" data-tooltip="${escapeHTML(arenaGroupLabel(group))}" data-tooltip-size="compact">${rank}<span class="arena-team-roster">${group.players.map((item, index) => playerButton(item, index)).join("")}</span></div>`;
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
    return `<b class="match-rank-chip${tone}" data-tooltip="本场评分排名：全场 ${number(total)} 人中的第 ${number(rank)} 名" data-tooltip-size="compact">${ordinalLabel(rank)}</b>`;
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
    const augmentIDs = matchAugmentIDs(subject, modeKind === "mayhem" ? 2 : 4);
    const augmentSlots = Array.from({ length: modeKind === "mayhem" ? 2 : 4 }, (_, index) => augmentIDs[index]
      ? augmentIconFigure(augmentIDs[index], "small")
      : '<span class="arena-augment-slot is-empty" aria-hidden="true"></span>');
    const usesAugments = modeKind === "arena" || modeKind === "mayhem";
    const loadout = modeKind === "arena"
      ? augmentSlots.join("")
      : modeKind === "mayhem"
        ? [...spells, ...augmentSlots].join("")
        : [...spells, ...runes].join("");
    return { augmentIDs, usesAugments, layout: modeKind === "mayhem" ? "mayhem" : usesAugments ? "augments" : "standard", loadout };
  }

  // 平均段位：优先用数据里带的 averageTier（演示数据），否则读缓存；
  // 均未命中时输出占位符，由 hydrateMatchTiers 懒加载后回填。
  function matchTierContent(value) {
    if (value?.unsupported) return '<b class="match-tier-unknown">不支持</b>';
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
    const base = "这场对局玩家的平均段位（按可查询到的玩家统计）";
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
    const scores = computeMatchScores(match);
    const subjectScore = scores.get(Number(subject.participantId));
    const badges = [];
    if (subject.multiKill >= 5) badges.push("五杀"); else if (subject.multiKill === 4) badges.push("四杀"); else if (subject.multiKill === 3) badges.push("三杀"); else if (subject.multiKill === 2) badges.push("双杀");
    if (subject.deaths === 0) badges.push("零阵亡");
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
    const lpValue = Number(match.lpDelta);
    const lpChip = ranked && resultKind !== "remake" && Number.isFinite(lpValue) && match.lpDelta != null
      ? `<b class="lp-delta ${lpValue >= 0 ? "is-gain" : "is-drop"}" data-tooltip="这场排位的胜点变化\n由助手在对局结算时记录" data-tooltip-size="compact">${lpValue >= 0 ? "+" : "−"}${Math.abs(lpValue)} LP</b>`
      : "";
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
    const damageRow = `<span class="match-stat match-stat-damage" data-tooltip="对英雄造成的总伤害" data-tooltip-size="compact"><em>伤害</em><b>${compactNumber(subject.damage)}</b></span>`;
    const takenRow = `<span class="match-stat match-stat-damage" data-tooltip="承受的总伤害" data-tooltip-size="compact"><em>承伤</em><b>${compactNumber(subject.damageTaken)}</b></span>`;
    // 斗魂竞技场用伤害/承伤替代召唤师峡谷的参团率、CS、平均段位；
    // 数据缺失时保留明确占位，避免卡片列高度随模式变化而漂移。
    const statRows = modeKind === "arena" ? `${damageRow}${takenRow}` : `${participationRow}${csRow}${tierRow}`;
    const { usesAugments, layout, loadout } = renderMatchLoadout(subject, modeKind);
    const detailFailure = renderMatchDetailFailure(match.gameId, detailState);
    return `<article class="match-entry is-${resultKind}" data-match-id="${match.gameId}">
      <div class="match-summary${arenaPlacement ? " is-arena" : ""}">
        <div class="match-result-meta"><strong>${escapeHTML(match.queueLabel)}</strong>${timeRow}<span class="result-word">${resultWord}${lpChip}</span><small>${formatDuration(match.duration)}</small></div>
        <div class="match-main${arenaPlacement ? " is-arena" : ""}">
          <div class="match-champion">${iconFigure("champion", subject.championId, subject.championName, "large")}<span class="champion-level">${number(subject.championLevel)}</span><div class="match-loadout-mini${usesAugments ? " is-augments" : ""}${layout === "mayhem" ? " is-mayhem" : ""}">${loadout}</div></div>
          <div class="match-kda"><strong><span>${number(subject.kills)}</span> / <em>${number(subject.deaths)}</em> / <span>${number(subject.assists)}</span></strong><small>${kda(subject.kda)}:1 KDA</small></div>
	          ${statRows ? `<div class="match-stats${modeKind === "arena" ? " is-arena" : ""}">${statRows}</div>` : ""}
          <div class="match-build"><div class="match-items">${renderItemSlots(subject.itemIds)}</div>${subjectScore ? scorePlacementChip(subjectScore) : ""}<div class="match-badges">${badges.map((badge) => `<span>${badge}</span>`).join("")}</div></div>
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

  function applyMatchTierValue(container, scope, gameID, value) {
    if (!container || container.dataset.matchTierScope !== scope) return;
    for (const node of container.querySelectorAll("[data-match-tier]")) {
      if (String(node.dataset.gameId || "") !== String(gameID)) continue;
      node.removeAttribute("data-match-tier-pending");
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
	}, { root, rootMargin: "64px 0px" });
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
      if (failure && Number(failure.nextRetryAt || 0) > Date.now()) {
        applyMatchTierValue(container, scope, gameID, null);
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
      const entries = [...pendingMatches.entries()];
      if (!entries.length) return;
      for (const [, entry] of entries) state.matchTierFlights.add(entry.cacheKey);
      try {
        const matches = entries.map(([, entry]) => ({
          gameId: entry.match.gameId,
          createdAt: entry.match.createdAt,
          duration: entry.match.duration,
        }));
        const result = await api("/api/gameplay/match-tiers", {
          method: "POST",
          body: JSON.stringify({
            region: "kr",
            playerRef,
            gameName: player.gameName || tab.riotId?.gameName || "",
            tagLine: player.tagLine || tab.riotId?.tagLine || "",
            matches,
          }),
        }, `match-tiers:${scope}:${entries.map(([gameID]) => gameID).join(",")}`, 20000);
        for (const [gameID, entry] of entries) {
          const candidate = result?.[gameID];
          const value = candidate?.tier ? candidate : null;
          state.matchTiers.set(entry.cacheKey, value);
          state.matchTierFailures?.delete(entry.cacheKey);
          applyMatchTierValue(container, scope, gameID, value);
        }
      } catch (_) {
        for (const [gameID, entry] of entries) {
          noteMatchTierFailure(entry.cacheKey);
          applyMatchTierValue(container, scope, gameID, null);
        }
      } finally {
        for (const [, entry] of entries) state.matchTierFlights.delete(entry.cacheKey);
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
    try {
      for (let offset = 0; offset < uniqueRefs.length; offset += MATCH_TIERS_MAX_REFS) {
        const refs = uniqueRefs.slice(offset, offset + MATCH_TIERS_MAX_REFS);
        const result = await api("/api/gameplay/match-tiers", {
          method: "POST",
          body: JSON.stringify({ playerRefs: refs, serverId: tabServerID(tab) }),
        }, `match-tiers:${scope}:${offset}:${refs.join(",")}`, 20000);
        cacheHits += Math.max(0, Number(result?.cacheHits) || 0);
        for (const [ref, value] of Object.entries(result?.players || {})) {
          if (Number.isFinite(Number(value?.score))) playerScores.set(ref, Number(value.score));
        }
      }
      for (const [gameID, entry] of entries) {
        const scores = (refsByGame.get(gameID) || []).map((ref) => playerScores.get(ref)).filter((score) => Number.isFinite(score));
        const value = matchTierFromScores(scores);
        state.matchTiers.set(entry.cacheKey, value);
        state.matchTierFailures?.delete(entry.cacheKey);
        applyMatchTierValue(container, scope, gameID, value);
      }
      recordMatchTierOverviewBatch(totalRefs, uniqueRefs.length, cacheHits);
    } catch (_) {
      for (const [gameID, entry] of entries) {
        noteMatchTierFailure(entry.cacheKey);
        applyMatchTierValue(container, scope, gameID, null);
      }
    } finally {
      for (const [, entry] of entries) state.matchTierFlights.delete(entry.cacheKey);
    }
  }

  function recordMatchTierOverviewBatch(totalRefs, uniqueRefs, cacheHits) {
    void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({
        event: "match_tiers_overview_batch", reason: "complete",
        totalRefs: Math.max(0, Number(totalRefs) || 0),
        uniqueRefs: Math.max(0, Number(uniqueRefs) || 0),
        cacheHits: Math.max(0, Number(cacheHits) || 0),
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

  function renderMatchDetail(match, subject, tab) {
    if (matchPlayerGroups(match).arena) {
      return `<div id="match-detail-${match.gameId}" class="match-detail is-arena-detail">${renderArenaMatchOverview(match)}</div>`;
    }
    const active = tab.matchDetailTabs.get(String(match.gameId)) || "overview";
    const panelID = `match-detail-body-${match.gameId}`;
    const detailTab = (value, label) => `<button id="match-detail-tab-${value}-${match.gameId}" class="${active === value ? "is-active" : ""}" type="button" role="tab" aria-selected="${active === value}" aria-controls="${panelID}" tabindex="${active === value ? 0 : -1}" data-match-detail="${value}" data-game-id="${match.gameId}">${label}</button>`;
    return `<div id="match-detail-${match.gameId}" class="match-detail"><div class="match-detail-head"><div class="match-detail-tabs" role="tablist" aria-label="对局详情">${detailTab("overview", "概览")}${detailTab("team", "队伍分析")}${detailTab("build", "构建")}</div></div><div id="${panelID}" role="tabpanel" aria-labelledby="match-detail-tab-${active}-${match.gameId}">${active === "overview" ? renderMatchOverview(match) : active === "team" ? renderTeamAnalysis(match, tab) : renderBuild(match, subject, tab)}</div></div>`;
  }

  function matchTableRows(players, scores) {
    return players.map((item, index) => {
      const record = scores.get(Number(item.participantId));
      const name = playerParticipantName(item, index);
      return `<tr><td><button class="participant-link" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"} data-tooltip="${escapeHTML(name)}" data-tooltip-overflow=".participant-name" data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "small")}<span class="participant-name">${escapeHTML(name)}</span></button></td><td>${renderMatchScoreCell(record)}</td><td>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}<small>${kda(item.kda)}:1</small></td><td>${number(item.damage)}<small>承伤 ${number(item.damageTaken)}</small></td><td>${number(item.wardsPlaced)} / ${number(item.wardsKilled)}</td><td>${number(item.cs)}<small>${number(item.csPerMinute)}/分钟</small></td><td><div class="table-items">${renderItemIcons(item.itemIds || [], "small")}</div></td></tr>`;
    }).join("");
  }

  function matchTableShell(rows) {
    return `<div class="table-scroll"><table class="match-table"><colgroup><col class="match-col-player"><col class="match-col-score"><col class="match-col-kda"><col class="match-col-damage"><col class="match-col-wards"><col class="match-col-cs"><col class="match-col-items"></colgroup><thead><tr><th>玩家</th><th>评分</th><th>KDA</th><th>伤害</th><th>守卫</th><th>CS</th><th>装备</th></tr></thead><tbody>${rows}</tbody></table></div>`;
  }

  function renderMatchOverview(match) {
    const scores = computeMatchScores(match);
    const grouping = matchPlayerGroups(match);
    // 斗魂竞技场：一队一个分组，按名次排序完整展示全部玩家。
    if (grouping.arena) {
      return renderArenaMatchOverview(match);
    }
    const teams = grouping.groups.map((group) => {
      const teamID = group.teamId;
      const team = (match.teams || []).find((item) => item.teamId === teamID) || {};
      return `<section class="team-overview is-${teamID === 100 ? "blue" : "red"}"><header><strong>${teamID === 100 ? "蓝方" : "红方"}${team.win ? " · 胜利" : ""}</strong><span>${number(team.kills)} 击杀 · ${number(team.gold)} 金币</span></header>${matchTableShell(matchTableRows(group.players, scores))}</section>`;
    }).join("");
    return `<div class="match-overview-teams">${teams}</div>`;
  }

	function renderArenaMatchOverview(match) {
    const scores = computeMatchScores(match);
    const grouping = matchPlayerGroups(match);
    const sections = grouping.groups.map((group) => {
      const team = arenaTeamMeta(group);
      const kills = group.players.reduce((sum, item) => sum + (Number(item.kills) || 0), 0);
      const damage = group.players.reduce((sum, item) => sum + (Number(item.damage) || 0), 0);
      const tone = group.placement === 1 ? " is-first" : group.placement > 0 && group.placement <= 4 ? " is-top" : "";
      const emblem = team.iconPath ? `<span class="game-icon is-small"><img src="${escapeHTML(team.iconPath)}" alt="" loading="lazy" decoding="async" data-game-image></span>` : "";
      const players = group.players.map((item, index) => {
        const record = scores.get(Number(item.participantId));
        const name = playerParticipantName(item, index);
		return `<div class="arena-detail-player"><button class="participant-link" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"} data-tooltip="${escapeHTML(name)}" data-tooltip-overflow=".participant-name" data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "small")}<span class="participant-name">${escapeHTML(name)}</span></button><span class="arena-detail-augments">${(item.augmentIds || []).slice(0, 6).map((id) => augmentIconFigure(id, "small")).join("")}</span>${renderMatchScoreCell(record)}<span class="arena-detail-kda"><b>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}</b><small>${kda(item.kda)}:1 KDA</small></span><span class="arena-detail-damage" aria-label="${escapeHTML(`伤害 ${plainInteger(item.damage)}，承伤 ${plainInteger(item.damageTaken)}`)}" data-tooltip="${escapeHTML(`伤害 ${number(item.damage)} · 承伤 ${number(item.damageTaken)}`)}" data-tooltip-size="compact"><b class="match-damage-value">${plainInteger(item.damage)}</b><i aria-hidden="true">/</i><b class="match-taken-value">${plainInteger(item.damageTaken)}</b></span><span class="arena-detail-items">${renderItemIcons(item.itemIds || [], "small")}</span></div>`;
      }).join("");
      return `<section class="arena-detail-team${tone}"><header><b>#${number(group.placement || "—")}</b>${emblem}<strong>${escapeHTML(team.name)}</strong><span>${number(kills)} 击杀 · ${compactNumber(damage)} 伤害</span></header><div class="arena-detail-player-list">${players}</div></section>`;
    }).join("");
	  return `<div class="arena-match-detail"><header><strong>最终排名</strong><span>按小队名次展示全部玩家、海克斯与装备</span></header><div class="arena-detail-columns" role="row"><span>玩家</span><span class="arena-column-augments">海克斯</span><span class="arena-column-score">评分</span><span>KDA</span><span class="arena-column-damage">伤害 / 承伤</span><span class="arena-column-items">装备</span></div>${sections}</div>`;
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

	function isSummonersRiftMatch(match) {
	  const mapID = Number(match?.mapId);
	  if (mapID > 0) return mapID === 11;
	  const queueID = Number(match?.queueId);
	  const modeGroup = String(match?.modeGroup || "").toLowerCase();
	  return SUMMONERS_RIFT_QUEUE_IDS.includes(queueID) || modeGroup === "solo" || modeGroup === "flex";
	}

	function isARAMRelatedMatch(match) {
	  return ["aram", "mayhem", "mayhem-classic"].includes(matchModeKind(match));
	}

	function teamAnalysisMetricsFor(match) {
	  const summonersRift = isSummonersRiftMatch(match);
	  const aram = isARAMRelatedMatch(match);
	  return TEAM_ANALYSIS_METRICS.filter((metric) => !(metric.summonersRiftOnly && !summonersRift) && !(metric.unavailableInARAM && aram));
	}

	function teamAnalysisMembers(players) {
	  return [...players].sort((left, right) => {
		const leftPosition = TEAM_ANALYSIS_POSITIONS.indexOf(String(left.position || "").toLowerCase());
		const rightPosition = TEAM_ANALYSIS_POSITIONS.indexOf(String(right.position || "").toLowerCase());
		const leftOrder = leftPosition < 0 ? TEAM_ANALYSIS_POSITIONS.length : leftPosition;
		const rightOrder = rightPosition < 0 ? TEAM_ANALYSIS_POSITIONS.length : rightPosition;
		return leftOrder - rightOrder || Number(left.participantId) - Number(right.participantId);
	  });
	}

	function teamAnalysisMetricValue(item, metric, teamKills) {
	  if (!item) return null;
	  if (metric.key === "killParticipation") {
		if (!(teamKills > 0)) return null;
		return Math.min(100, Math.round(((Number(item.kills) || 0) + (Number(item.assists) || 0)) * 1000 / teamKills) / 10);
	  }
	  if (metric.nullable && (item[metric.key] === null || item[metric.key] === undefined)) return null;
	  const value = Number(item[metric.key]);
	  return Number.isFinite(value) ? Math.max(0, value) : null;
	}

	function teamAnalysisMetricLabel(value, metric) {
	  if (value === null) return "—";
	  return metric.percent ? `${number(value)}%` : number(value);
	}

	function renderTeamAnalysis(match, tab) {
    const participants = match.participants || [];
    const sortKey = (team = "") => `${String(match.gameId)}:${String(team)}`;
    const metricFor = (team = "") => tab?.damageSorts?.get(sortKey(team)) === "damageTaken" ? "damageTaken" : "damage";
    const sortButton = (team = "", currentMetric = metricFor(team)) => {
      const label = currentMetric === "damageTaken" ? "承伤" : "伤害";
      const next = currentMetric === "damageTaken" ? "伤害" : "承伤";
      return `<button class="damage-sort-toggle" type="button" data-damage-sort="${currentMetric}" data-game-id="${match.gameId}" data-team="${escapeHTML(team)}" aria-label="当前按${label}排序，切换为按${next}排序" data-tooltip="当前按${label}排序\n点击切换为按${next}排序"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 6h11M8 12h8M8 18h5M4 4v16m0 0-2.5-2.5M4 20l2.5-2.5"/></svg><span>${label}</span></button>`;
    };
    const damageRow = (item, index, tone, rowMetric, peak) => {
      const value = Math.max(0, Number(item[rowMetric]) || 0);
      const share = Math.round(value * 1000 / peak) / 10;
      const rowLabel = rowMetric === "damageTaken" ? "承伤" : "伤害";
      return `<div class="damage-row">${iconFigure("champion", item.championId, item.championName, "tiny")}<span class="damage-name" data-tooltip="${escapeHTML(playerParticipantName(item, index))}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(playerParticipantName(item, index))}</span><span class="damage-bar-line"><span class="damage-bar is-${tone}" role="img" aria-label="${escapeHTML(`${rowLabel} ${number(value)}，相对本组最高值 ${share}%`)}"><i data-bar-width="${value > 0 ? Math.max(1, share) : 0}"></i></span><b>${number(value)}</b></span></div>`;
    };
    const grouping = matchPlayerGroups(match);
    // 斗魂竞技场：红蓝双方总量对比无意义，改为按名次逐小队展示伤害与承伤。
    if (grouping.arena) {
      const sections = grouping.groups.map((group) => {
        const team = String(group.subteamId || group.placement || "arena");
        const groupMetric = metricFor(team);
        const groupLabel = groupMetric === "damageTaken" ? "承伤" : "伤害";
        const groupPeak = Math.max(1, ...group.players.map((item) => Number(item[groupMetric]) || 0));
        const tone = group.placement === 1 ? "blue" : "red";
        return `<section class="damage-team is-arena"><div class="damage-team-head"><h4>${escapeHTML(arenaGroupLabel(group))} · ${groupLabel}</h4>${sortButton(team, groupMetric)}</div>${[...group.players].sort((left, right) => (Number(right[groupMetric]) || 0) - (Number(left[groupMetric]) || 0) || Number(left.participantId) - Number(right.participantId)).map((item, index) => damageRow(item, index, tone, groupMetric, groupPeak)).join("")}</section>`;
      }).join("");
      return `<div class="damage-compare is-arena">${sections}</div>`;
    }
    const metrics = [["kills", "英雄击杀"], ["gold", "金币"], ["damage", "伤害"], ["visionScore", "视野分数"], ["damageTaken", "承伤"], ["cs", "小兵分数"]];
    const blue = (match.teams || []).find((team) => team.teamId === 100) || {};
    const red = (match.teams || []).find((team) => team.teamId === 200) || {};
    // 红蓝双色对峙条：左段蓝方、右段红方，长度即各自占比。
    const totals = `<div class="team-analysis-grid">${metrics.map(([key, label]) => {
      const left = Number(blue[key] || 0); const right = Number(red[key] || 0); const total = Math.max(1, left + right); const share = Math.round(left * 100 / total);
      return `<section class="team-metric"><div class="team-metric-head"><b class="team-metric-value is-blue">${number(left)}</b><h4>${label}</h4><b class="team-metric-value is-red">${number(right)}</b></div><div class="team-metric-track" role="img" aria-label="${label}：蓝方 ${share}%，红方 ${100 - share}%" data-tooltip="蓝方 ${share}% · 红方 ${100 - share}%" data-tooltip-size="compact"><i data-blue-share="${share}"></i></div></section>`;
    }).join("")}</div>`;
	  const gameID = String(match.gameId);
	  const availableMetrics = teamAnalysisMetricsFor(match);
	  const selectedKey = tab?.teamAnalysisMetrics?.get(gameID) || availableMetrics[0].key;
	  const selectedMetric = availableMetrics.find((metric) => metric.key === selectedKey) || availableMetrics[0];
	  const blueMembers = teamAnalysisMembers(participants.filter((item) => Number(item.teamId) === 100));
	  const redMembers = teamAnalysisMembers(participants.filter((item) => Number(item.teamId) === 200));
	  const teamKills = new Map([100, 200].map((teamID) => [teamID, participants.filter((item) => Number(item.teamId) === teamID).reduce((sum, item) => sum + (Number(item.kills) || 0), 0)]));
	  const values = participants.map((item) => teamAnalysisMetricValue(item, selectedMetric, teamKills.get(Number(item.teamId))));
	  const peak = Math.max(1, ...values.filter((value) => value !== null));
	  const metricTabs = `<div class="team-analysis-metrics" role="tablist" aria-label="队伍分析指标">${availableMetrics.map((metric) => {
		const active = metric.key === selectedMetric.key;
		return `<button id="team-analysis-metric-${metric.key}-${gameID}" type="button" role="tab" aria-selected="${active}" aria-controls="team-analysis-chart-${gameID}" tabindex="${active ? 0 : -1}" data-team-analysis-metric="${metric.key}" data-game-id="${gameID}">${metric.label}</button>`;
	  }).join("")}</div>`;
	  const playerCell = (item, index, tone) => item
		? `<span class="team-duel-player is-${tone}" data-tooltip="${escapeHTML(`${playerParticipantName(item, index)} · ${item.championName || "未知英雄"} · ${positionLabel(item.position)}`)}" data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "small")}</span>`
		: '<span class="team-duel-player is-empty" aria-hidden="true"></span>';
	  const valueCell = (item, tone) => {
		const value = teamAnalysisMetricValue(item, selectedMetric, teamKills.get(Number(item?.teamId)));
		return `<b class="team-duel-value is-${tone}">${teamAnalysisMetricLabel(value, selectedMetric)}</b>`;
	  };
	  const bar = (item, tone) => {
		const value = teamAnalysisMetricValue(item, selectedMetric, teamKills.get(Number(item?.teamId)));
		const share = value === null ? 0 : Math.round(value * 1000 / peak) / 10;
		const label = item ? `${item.championName || "未知英雄"}${selectedMetric.label} ${teamAnalysisMetricLabel(value, selectedMetric)}` : `${tone === "blue" ? "蓝方" : "红方"}该位置无数据`;
		return `<span class="team-duel-half is-${tone}" role="img" aria-label="${escapeHTML(label)}"><i data-bar-width="${value > 0 ? Math.max(1, share) : 0}"></i></span>`;
	  };
	  const rowCount = Math.max(blueMembers.length, redMembers.length);
	  const rows = Array.from({ length: rowCount }, (_, index) => {
		const bluePlayer = blueMembers[index]; const redPlayer = redMembers[index];
		return `<div class="team-duel-row">${playerCell(bluePlayer, index, "blue")}${valueCell(bluePlayer, "blue")}<span class="team-duel-bars">${bar(bluePlayer, "blue")}<i aria-hidden="true"></i>${bar(redPlayer, "red")}</span>${valueCell(redPlayer, "red")}${playerCell(redPlayer, index, "red")}</div>`;
	  }).join("");
	  const contribution = `<section class="team-contribution">${metricTabs}<div id="team-analysis-chart-${gameID}" class="team-duel-chart" role="tabpanel" aria-labelledby="team-analysis-metric-${selectedMetric.key}-${gameID}"><header class="team-duel-head"><b class="is-blue">蓝方</b><strong>${selectedMetric.label}</strong><b class="is-red">红方</b></header>${rows}</div></section>`;
	  return totals + contribution;
	}

  /* ---------- 对局时间线（装备路线 + 技能加点） ---------- */

  function matchTimelineKey(match, subject, tab) {
    return `${riotTab(tab) ? "kr" : `cn:${tabServerID(tab) || "current"}`}:${match.gameId}:${Number(subject?.participantId) || 0}`;
  }

  function recordTimelineClient(reason) {
    void fetch("/api/diagnostics/client", {
      method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({event: "match_timeline_client", reason}),
    }).catch(() => {});
  }

  async function ensureMatchTimeline(match, subject, tab) {
    const participantId = Number(subject?.participantId);
    if (!match || !participantId) { recordTimelineClient("missing-participant"); return; }
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
          gameId: Number(match.gameId), participantId, region: riotTab(tab) ? "kr" : "",
          serverId: tabServerID(tab), playerRef: tab.data?.player?.playerRef || tab.playerRef || "",
        }),
      }, `match-timeline:${key}`, 25000);
      state.matchTimelines.set(key, result && typeof result === "object" ? result : { available: false });
      if (!result?.available) recordTimelineClient("unavailable");
      settled = true;
    } catch (error) {
      if (error.name !== "RequestCancelled") {
        recordTimelineClient("request-failed");
        state.matchTimelines.set(key, { available: false, detail: error.message });
        settled = true;
      }
    } finally {
      state.matchTimelineFlights.delete(key);
      if (settled) {
        tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;
        rerenderTab(tab);
      }
    }
  }

  // 装备路线：按分钟分组的购买/出售记录，组间用箭头衔接（同 OP.GG）。
  function renderItemRoute(timeline) {
    const groups = timeline?.itemGroups || [];
    if (!groups.length) return "";
    return `<div class="timeline-route">${groups.map((group) => `<div class="timeline-group"><div class="timeline-group-items">${(group.events || []).map((event) => `<span class="timeline-item${event.sold ? " is-sold" : ""}"${event.sold ? ' data-tooltip="已出售" data-tooltip-size="compact"' : ""}>${itemIconFigure(event.itemId, "slot")}</span>`).join("")}</div><small>${number(group.minute)}分</small></div>`).join('<span class="timeline-arrow" aria-hidden="true">›</span>')}</div>`;
  }

  const SKILL_SLOT_LETTERS = { 1: "Q", 2: "W", 3: "E", 4: "R" };

  // 主升/副升摘要：R 之外，按“先点满”的顺序排列。
  function skillPrioritySummary(order) {
    const counts = { 1: 0, 2: 0, 3: 0 };
    const maxedAt = {};
    order.forEach((up, index) => {
      if (up.slot < 1 || up.slot > 3) return;
      counts[up.slot] += 1;
      if (counts[up.slot] === 5 && maxedAt[up.slot] === undefined) maxedAt[up.slot] = index;
    });
    const ranked = [1, 2, 3].filter((slot) => counts[slot] > 0)
      .sort((left, right) => (maxedAt[left] ?? 99) - (maxedAt[right] ?? 99) || counts[right] - counts[left]);
    if (ranked.length < 2) return "";
    return `<span class="match-skill-priority">主升 <b class="is-slot-${ranked[0]}">${SKILL_SLOT_LETTERS[ranked[0]]}</b> · 副升 <b class="is-slot-${ranked[1]}">${SKILL_SLOT_LETTERS[ranked[1]]}</b></span>`;
  }

  // 技能加点：一格一次加点，格内是技能字母（按技能着色）+ 第几级，
  // 与 OP.GG 的顺序条一致，直接从左往右读。
  function renderSkillOrder(timeline) {
    const order = timeline?.skillOrder || [];
    if (!order.length) return "";
    const cells = order.map((up) => `<span class="match-skill-cell is-slot-${up.slot}" data-tooltip="第 ${up.level} 级：升级 ${SKILL_SLOT_LETTERS[up.slot] || "?"}" data-tooltip-size="compact"><b>${SKILL_SLOT_LETTERS[up.slot] || "?"}</b><small>${up.level}</small></span>`).join("");
    return `<div class="match-skill-seq">${cells}</div>`;
  }

  function renderBuild(match, subject, tab) {
    const augmentIDs = matchAugmentIDs(subject, 6);
    const hasAugments = augmentIDs.length > 0;
    const runeContent = hasAugments
      ? `<div class="build-augment-grid">${augmentIDs.map((id) => augmentIconFigure(id, "large")).join("")}</div>`
      : subject.perkIds?.length
        ? renderUnifiedRuneBoard(subject)
        : '<div class="detail-empty"><strong>这场对局没有符文或海克斯数据</strong><p>部分娱乐模式会关闭符文系统。</p></div>';
    const timeline = state.matchTimelines.get(matchTimelineKey(match, subject, tab));
    const timelineLoading = timeline === undefined && (riotTab(tab) || connected());
    const unavailableCopy = (fallback) => `<div class="detail-empty compact"><p>${escapeHTML(timeline?.detail || fallback)}</p></div>`;
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
    const skillSummary = timeline?.available && timeline.skillOrder?.length ? skillPrioritySummary(timeline.skillOrder) : "";
    return `<div class="build-detail">
      <section><header><h4>装备路线</h4>${timeline && !timeline.available ? `<button type="button" class="text-button" data-timeline-retry="${escapeHTML(String(match.gameId))}">重试加载</button>` : ""}</header>${routeContent}</section>
      <section><header><h4>技能加点</h4>${skillSummary || "<span>按对局中的真实加点顺序</span>"}</header>${skillContent}</section>
      <section class="rune-detail"><header><h4>${hasAugments ? "海克斯" : "符文"}</h4></header>${runeContent}</section>
    </div>`;
  }

  function rerenderCatalogViews() {
    if (state.section === "overview") renderOverview();
    if (state.section === "live") renderLive();
    if (state.overlay.length) renderOverlay();
    for (const [container, view] of externalMatchViews) {
      if (!container.isConnected) externalMatchViews.delete(container);
      else view.render();
    }
  }

  // 图标目录（符文/装备/召唤师技能）：客户端连接时来自本机客户端；
  // 未连接时后端会回退到 Data Dragon，因此不再限制连接状态。
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
        if (state.perks) rerenderCatalogViews();
      }
    }
  }

  async function ensureItems() {
    if (state.items || state.itemsLoading) return;
    state.itemsLoading = true;
    try {
      const result = await api("/api/gameplay/items", {}, "items", 15000);
      state.items = Array.isArray(result?.items) && result.items.length ? result : null;
    }
    catch (_) { state.items = null; }
    finally {
      state.itemsLoading = false;
      if (state.items) rerenderCatalogViews();
    }
  }

  async function ensureSummonerSpells() {
    if (state.summonerSpells || state.summonerSpellsLoading || Date.now() < Number(state.summonerSpellsRetryAt || 0)) return;
    state.summonerSpellsLoading = true;
    try {
      const result = await api("/api/gameplay/summoner-spells", {}, "summoner-spells", 15000);
      state.summonerSpells = Array.isArray(result?.spells) && result.spells.length ? result : null;
    }
    catch (_) { state.summonerSpells = null; }
    finally {
      state.summonerSpellsLoading = false;
      if (state.summonerSpells) {
        state.summonerSpellsRetryAt = 0;
        rerenderCatalogViews();
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
      if (tab.overlay) openPlayerOverlay({ playerRef: button.dataset.playerRef, region: sourceRegion, serverId: sourceServerID, label });
      else openPlayer(button.dataset.playerRef, label, sourceRegion, sourceServerID);
    });
  }

  function bindMatchFilterControls(container, tab) {
    for (const button of container.querySelectorAll("[data-match-filter]")) button.addEventListener("click", () => updateMatchFilter(tab, button.dataset.matchFilter));
    bindAppSelect(container.querySelector("[data-match-more-select]"), (value) => {
      if (value) updateMatchFilter(tab, value);
    });
  }

	async function updateMatchFilter(tab, value) {
    if (!value || value === tab.matchFilter) return;
    rememberMatchScrollTop(tab, tab.matchFilter);
    tab.matchFilter = value;
    tab.openMatches.clear();
    tab.matchDetailTabs.clear();
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
  }

  async function autoLoadMatchFilter(tab, filter = tab.matchFilter) {
	if (tab.data?.pagination?.serverFiltered) return;
	const token = Number(tab.filterPagingToken || 0) + 1;
	tab.filterPagingToken = token;
	tab.filterPaging = true;
	try {
	  while (tab.filterPagingToken === token && tab.matchFilter === filter) {
		const pagination = tab.data?.pagination;
		const visible = filteredMatches(tab.data?.matches || [], tab).length;
		if (!pagination?.hasMore || pagination.autoPaused || visible >= state.settings.matchCount) break;
		if (tab.loading || tab.loadingMore) {
		  await new Promise((resolve) => setTimeout(resolve, 50));
		  continue;
		}
		tab.filterPagingPage = Math.floor(Math.max(0, Number(tab.nextBegIndex || 0)) / Math.max(1, state.settings.matchCount)) + 1;
		renderFilteredMatchView(tab);
		const loaded = await loadOverview(tab, false, true);
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
    const filterbar = container.querySelector(".match-filterbar");
    if (filterbar) filterbar.outerHTML = renderMatchFilters(tab);
    bindMatchFilterControls(container, tab);
    const list = container.querySelector(".match-list");
    if (list) {
      reconcileFilteredMatchList(list, tab);
    }
    appendOverviewMatches(tab, []);
    if (restoreScroll) restoreMatchScrollTop(tab);
  }

  function reconcileFilteredMatchList(list, tab) {
    const raw = tab.data?.matches || [];
    const previous = list._matchData || new Map(raw.map(match => [String(match.gameId), match]));
    const current = new Map(raw.map(match => [String(match.gameId), match]));
    const visible = filteredMatches(raw, tab);
    const visibleIDs = new Set(visible.map(match => String(match.gameId)));
    const entries = new Map([...list.querySelectorAll(":scope > .match-entry")].map(entry => [entry.dataset.matchId, entry]));
    for (const [id, entry] of entries) {
      if (!current.has(id)) { entry.remove(); entries.delete(id); continue; }
      entry.hidden = !visibleIDs.has(id);
    }
    const playerRef = tab.data?.player?.playerRef || tab.playerRef || "";
    for (const match of visible) {
      const id = String(match.gameId), entry = entries.get(id), old = previous.get(id);
      const changed = old !== match && JSON.stringify(old) !== JSON.stringify(match);
      const wasOpen = entry?.querySelector('[data-toggle-match][aria-expanded="true"]');
      if (entry && !changed && (!wasOpen || tab.openMatches.has(id))) continue;
      const template = document.createElement("template");
      template.innerHTML = renderMatch(match, playerRef, tab);
      const replacement = template.content.firstElementChild;
      bindOverviewContent(replacement, tab);
      applyRenderedMetricStyles(replacement);
      prepareImages(replacement);
      if (entry) entry.replaceWith(replacement); else list.appendChild(replacement);
      entries.set(id, replacement);
    }
    // Reordering existing nodes preserves identity, handlers and loaded images.
    for (const match of raw) { const entry = entries.get(String(match.gameId)); if (entry) list.appendChild(entry); }
    for (const child of [...list.children]) if (!child.matches('.match-entry')) child.remove();
    if (!visible.length) {
      const empty = document.createElement('div');
      const capability = (tab.data?.capabilities || []).find(item => item.name === 'match-history');
      empty.innerHTML = matchListEmptyContent(tab, capability, tab.matchFilter === 'more:special'
        ? '自定义对局不会展示；这里只保留客户端实际返回的其他特殊模式。' : '可以切换上方游戏类型，或刷新读取最新战绩。');
      list.appendChild(empty);
    }
    list._matchData = current;
  }

	function bindMatchDetailControls(container, tab, rerender) {
    for (const button of container.querySelectorAll("[data-timeline-retry]")) button.addEventListener("click", () => {
      const match = (tab.data?.matches || []).find((item) => String(item.gameId) === button.dataset.timelineRetry);
      if (!match) return;
      const subject = matchSubject(match, tab.data?.player?.playerRef);
      state.matchTimelines.delete(matchTimelineKey(match, subject, tab));
      void ensureMatchTimeline(match, subject, tab);
      rerender();
    });

    for (const button of container.querySelectorAll("[data-match-detail]")) button.addEventListener("click", () => {
      tab.matchDetailTabs.set(button.dataset.gameId, button.dataset.matchDetail);
      if (button.dataset.matchDetail === "build") {
        ensurePerks(); ensureItems();
        const match = (tab.data?.matches || []).find((item) => String(item.gameId) === String(button.dataset.gameId));
        if (match) ensureMatchTimeline(match, matchSubject(match, tab.data?.player?.playerRef), tab);
      }
      tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;
      rerender();
    });
	  for (const button of container.querySelectorAll("[data-damage-sort]")) button.addEventListener("click", () => {
      const gameID = String(button.dataset.gameId || "");
      if (!gameID) return;
      const team = String(button.dataset.team || "");
      tab.damageSorts.set(`${gameID}:${team}`, button.dataset.damageSort === "damageTaken" ? "damage" : "damageTaken");
		tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;
		rerender();
	  });
	  for (const button of container.querySelectorAll("[data-team-analysis-metric]")) button.addEventListener("click", () => {
		const gameID = String(button.dataset.gameId || "");
		const metric = String(button.dataset.teamAnalysisMetric || "");
		if (!gameID || !TEAM_ANALYSIS_METRICS.some((item) => item.key === metric)) return;
		tab.teamAnalysisMetrics.set(gameID, metric);
		tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;
		rerender();
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
		tabs[next].click();
		container.querySelector(`[data-game-id="${gameID}"][data-team-analysis-metric="${metric}"]`)?.focus();
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
      tabs[next].click();
      container.querySelector(`[data-game-id="${gameID}"][data-match-detail="${detail}"]`)?.focus();
    });
    // 已展开且停在“构建”页签的对局：确保时间线已请求（幂等，命中缓存
    // 或已在途时直接返回），避免请求被取消后一直停留在加载文案。
    for (const gameId of tab.openMatches) {
      if ((tab.matchDetailTabs.get(String(gameId)) || "overview") !== "build") continue;
      const match = (tab.data?.matches || []).find((item) => String(item.gameId) === String(gameId));
      if (match) ensureMatchTimeline(match, matchSubject(match, tab.data?.player?.playerRef), tab);
    }
  }

  function bindMatchEntryControls(entry, tab, rerender) {
    if (!entry) return;
    for (const button of entry.querySelectorAll("[data-toggle-match]")) button.addEventListener("click", () => {
      const id = String(button.dataset.toggleMatch || "");
      if (tab.openMatches.has(id)) {
        tab.openMatches.delete(id);
        // 收起后清除页签记忆，重新展开时回到“概览”。
        tab.matchDetailTabs.delete(id);
      } else {
        tab.openMatches.add(id);
      }
      const match = (tab.data?.matches || []).find((item) => String(item.gameId) === id);
      if (!match || !entry.isConnected) {
        rerender();
        return;
      }
      // 展开/收起只替换目标卡片，不改变 matchViewRevision；该 revision
      // 只描述筛选、排序等会改变整张列表形态的状态。
      const template = document.createElement("template");
      template.innerHTML = renderMatch(match, tab.data?.player?.playerRef || "", tab).trim();
      const replacement = template.content.firstElementChild;
      if (!replacement) {
        rerender();
        return;
      }
      entry.replaceWith(replacement);
      bindMatchEntryControls(replacement, tab, rerender);
      bindMatchDetailControls(replacement, tab, rerender);
      for (const replayButton of replacement.querySelectorAll("[data-replay]")) replayButton.addEventListener("click", () => replay(replayButton));
      applyRenderedMetricStyles(replacement);
      prepareImages(replacement);
    });
  }

  function bindOverviewContent(container, tab) {
    const rerender = () => rerenderTab(tab);
    bindPlayerLinks(container, tab);
    bindOverviewShareControls(container, container, tab);
    container.querySelector("[data-open-career-dialog]")?.addEventListener("click", (event) => openCareerDialog(container, tab, event.currentTarget));
    container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadOverview(tab, true));
    bindMatchFilterControls(container, tab);
    container.querySelector("[data-load-more]")?.addEventListener("click", () => loadOverview(tab, false, true, true));
    bindRankHistoryControls(container);
    bindRankedQueueControls(container, tab);
    bindMatchSentinel(container, tab);
    for (const entry of container.querySelectorAll(".match-entry")) bindMatchEntryControls(entry, tab, rerender);
    bindMatchDetailControls(container, tab, rerender);
    for (const button of container.querySelectorAll("[data-replay]")) button.addEventListener("click", () => replay(button));
  }

  function bindRankedQueueControls(container, tab) {
    for (const button of container.querySelectorAll("[data-ranked-queue]")) button.addEventListener("click", () => {
      const queue = button.dataset.rankedQueue;
      const scope = button.closest("[data-ranked-queue-scope]")?.dataset.rankedQueueScope || "recent";
      const stateKey = { recent: "rankedQueueRecent", ability: "rankedQueueAbility", position: "rankedQueuePosition" }[scope] || "rankedQueueRecent";
      if (queue !== "420" && queue !== "440" || tab[stateKey] === queue) return;
      tab[stateKey] = queue;
      rerenderTab(tab);
      if (nodes.careerDialog?.open && tab.data && nodes.careerDialogContent) {
        nodes.careerDialogContent.dataset.careerLayout = "";
        const width = nodes.careerDialogContent.getBoundingClientRect().width || nodes.careerDialog.getBoundingClientRect().width || window.innerWidth;
        renderCareerDialogContent(tab.data, tab, width <= 640 ? "single" : "masonry");
      }
    });
  }

  function bindRankHistoryControls(container) {
    for (const button of container.querySelectorAll("[data-rank-history-toggle]")) button.addEventListener("click", () => {
      const expanded = button.getAttribute("aria-expanded") !== "true";
      button.setAttribute("aria-expanded", String(expanded));
      for (const row of button.closest(".rank-history")?.querySelectorAll("[data-rank-history-extra]") || []) row.hidden = !expanded;
      button.querySelector("span").textContent = expanded ? "收起" : "查看更多赛段段位";
    });
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

  async function loadLive(force = false) {
    if (state.destroyed) return;
    if (!connected()) { renderLive(); return; }
    if (state.liveLoading && !force) return;
    const requestToken = state.liveRequestToken = Number(state.liveRequestToken || 0) + 1;
    state.liveLoading = true;
    state.liveError = "";
    renderLive();
    try {
      const previousLive = state.live;
      const nextLive = await api("/api/gameplay/live", {}, "live", 45000);
      if (state.liveRequestToken !== requestToken || !connected()) return;
      if (shouldResetLiveGameScopedState(previousLive, nextLive)) resetLiveGameScopedState();
      resetLivePositionOverrides(previousLive, nextLive);
      resetRecommendationTabsOnChampionChange(previousLive, nextLive);
      state.live = nextLive;
      updateBeacon(String(state.live?.phase || ""));
      state.lastCapabilities = state.live.capabilities || state.lastCapabilities;
      renderCapabilitySettings();
      const recommendations = liveRecommendationsFor(state.live);
      if (state.live.clientRecommendation || recommendations?.runes) ensurePerks();
      if (recommendations?.build) { ensureItems(); ensureSummonerSpells(); }
      ensureLiveRecommendations(state.live);
      ensureSpecialistRunes(state.live);
      void ensureProRunes(state.live);
    } catch (error) {
      if (state.liveRequestToken === requestToken && error.name !== "RequestCancelled") state.liveError = error.message;
    } finally {
      if (state.liveRequestToken !== requestToken) return;
      state.liveLoading = false;
      renderLive();
      scheduleLiveRefresh();
      if (state.liveRefreshQueued) queueLiveEventRefresh();
    }
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
    const gameChanged = String(previous.gameId || "") !== String(next.gameId || "");
    const enteringChampionSelect = String(previous.phase || "") !== "ChampSelect" && String(next.phase || "") === "ChampSelect";
    return gameChanged || enteringChampionSelect;
  }

  function resetLiveGameScopedState(preserveTabs = false) {
    const recommendationTab = state.recommendationTab;
    const recommendationTabTouched = state.recommendationTabTouched;
    const runeSourceTab = state.runeSourceTab;
    state.liveGameGeneration = Number(state.liveGameGeneration || 0) + 1;
    for (const key of state.liveRecommendationFlights?.keys?.() || []) state.controllers?.get?.(`live-recommendations:${key}`)?.abort?.();
    for (const key of state.specialistRuneFlights?.keys?.() || []) state.controllers?.get?.(`live-specialist-runes:${key}`)?.abort?.();
    clearTimeout(state.proRuneTimer);
    for (const key of state.proRuneFlights?.keys?.() || []) state.controllers?.get?.(`live-pro-runes:${key}`)?.abort?.();
    for (const collection of [state.proRunes, state.proRuneFlights, state.liveRecommendations, state.liveRecommendationTraces, state.specialistRunes, state.liveRecommendationFailures, state.liveRecommendationFlights, state.specialistRuneFailures, state.specialistRuneFlights, state.livePositionOverride, state.liveRecommendationSkipDiagnostics, state.specialistRuneSkipDiagnostics, state.liveRosterRenderDiagnostics, state.specialistPlayerTabs]) collection?.clear?.();
    state.recommendationTab = "runes";
    state.recommendationTabTouched = false;
    state.runeSourceTab = "opgg";
    state.selectedRecommendation = "opgg";
    if (preserveTabs && recommendationTabTouched) {
      state.recommendationTab = recommendationTab;
      state.recommendationTabTouched = true;
      state.runeSourceTab = runeSourceTab;
    }
  }

  function softResetGameplayState() {
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
    state.liveError = "";
    resetLiveGameScopedState(true);
  }

  function handleHardRefresh() {
    softResetGameplayState();
    const group = overviewGroupForSection();
    const tab = activeTab(group);
    const tasks = [];
    if (state.section === "overview" && tab && tabReady(tab)) tasks.push(loadOverview(tab, true));
    else if (state.section === "overview") renderOverview(group);
    if (connected()) tasks.push(loadLive(true));
    else renderLive();
    scheduleBeaconPoll(0);
    return Promise.allSettled(tasks);
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
    const spellKey = [Number(self?.spell1Id) || 0, Number(self?.spell2Id) || 0].filter((value) => value > 0).sort((left, right) => left - right).join("-") || "none";
	return { self, championId, position, clientPosition, positionOverride, augmentSource, queueId, gameMode, mapId, tier, gameId: Number(data?.gameId) || 0, baseKey, spellKey, key: `${championId}:${position}:${gameMode}:${mapId}:${tier}:${spellKey}` };
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
    renderLive();
  }

  function liveRecommendationsFor(data) {
    if (data?.recommendations) return data.recommendations;
    const target = liveRecommendationTarget(data);
    return target ? state.liveRecommendations.get(target.key) || null : null;
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
    return { ...target, position, key: `${target.championId}:${position}:${target.gameMode}:${target.mapId}:${target.tier}:${target.spellKey}` };
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
    };
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
    if (data?.recommendations) { recordLiveRecommendationSkip("has-payload", target, data); return; }
    if (state.liveRecommendations.has(targetKey)) { recordLiveRecommendationSkip("cached", target, data); return; }
    if (liveRecommendationFlightActive(targetKey)) { recordLiveRecommendationSkip("in-flight", target, data); return; }
    const failedAt = Number(state.liveRecommendationFailures.get(targetKey) || 0);
    if (failedAt > 0 && Date.now() - failedAt < 60_000) { recordLiveRecommendationSkip("backoff", target, data); return; }
    const gameGeneration = Number(state.liveGameGeneration || 0);
    const traceId = newLiveRecommendationTraceId();
    state.liveRecommendationTraces.set(targetKey, traceId);
    state.liveRecommendationFlights.set(targetKey, Date.now());
    // 立刻重绘一次，好让各个页签在等待期间显示加载遮罩，而不是先闪一屏空状态。
    if (liveRecommendationTarget(state.live)?.key === targetKey) renderLive();
		const query = new URLSearchParams({
		  championId: String(target.championId), position: target.position,
		  queueId: String(target.queueId), gameMode: target.gameMode, mapId: String(target.mapId), tier: target.tier,
      traceId, recommendationKey: targetKey,
		});
		if (target.gameId > 0) query.set("gameId", String(target.gameId));
		if (Number(target.self?.spell1Id) > 0) query.set("spell1Id", String(target.self.spell1Id));
		if (Number(target.self?.spell2Id) > 0) query.set("spell2Id", String(target.self.spell2Id));
	if (target.augmentSource === "hextech") query.set("source", "mayhem");
    try {
      const response = await api(`/api/gameplay/recommendations?${query}`, {}, `live-recommendations:${targetKey}`, 20000);
      if (gameGeneration !== Number(state.liveGameGeneration || 0)) return;
      if (!hasUsableLiveRecommendations(response?.recommendations)) throw new Error("推荐数据不完整");
      const recommendations = response.recommendations;
      recommendations.traceId = recommendations.traceId || traceId;
      recommendations.recommendationKey = recommendations.recommendationKey || targetKey;
      state.liveRecommendations.set(targetKey, recommendations);
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
	  // hasTopPlayers arrives, retry immediately instead of waiting for another
	  // LCU live snapshot that may never change during champion select.
	  ensureSpecialistRunes(state.live);
      if (liveRecommendationTarget(state.live)?.key === targetKey) {
        renderLive();
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
      if (liveRecommendationTarget(state.live)?.key === targetKey) renderLive();
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
    if (!force && cached && Date.now() - cached.receivedAt < (cached.preparing || cached.outcomesLoading ? 3000 : cached.reason ? 60000 : 300000)) return;
    const generation = Number(state.liveGameGeneration || 0);
    state.proRuneFlights.set(target.key, true);
    try {
      const query = new URLSearchParams({ championId: String(target.championId), position: target.position || "", ...(force ? { refresh: "1" } : {}) });
      const response = await api(`/api/gameplay/pro-runes?${query}`, {}, `live-pro-runes:${target.key}`, 28000);
      if (generation !== Number(state.liveGameGeneration || 0)) return;
      if (!Array.isArray(response?.pros)) throw new Error("职业符文数据不完整");
      state.proRunes.set(target.key, { ...response, receivedAt: Date.now() });
      if (response.pros.length) { ensurePerks(); ensureItems(); }
    } catch (error) {
      if (generation !== Number(state.liveGameGeneration || 0) || error.name === "RequestCancelled") return;
      state.proRunes.set(target.key, { ...cached, pros: cached?.pros || [], stale: true, reason: "network-unavailable", receivedAt: Date.now() });
    } finally {
      if (generation === Number(state.liveGameGeneration || 0)) {
        state.proRuneFlights.delete(target.key);
        if (proRequestTarget(state.live)?.key === target.key) {
          renderLive();
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
	if (Array.isArray(embedded) && embedded.length) { recordSpecialistRuneClientSkip("embedded", target, data); return; }
	if (state.specialistRunes.has(target.key)) {
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
	state.specialistRuneFlights.set(target.key, now);
	    if (specialistRequestTarget(state.live)?.key === target.key) renderLive();
    const query = new URLSearchParams({ championId: String(target.championId), position: target.position });
    try {
	  const response = await api(`/api/gameplay/specialist-runes?${query}`, {}, `live-specialist-runes:${target.key}`, 30000);
	  if (gameGeneration !== Number(state.liveGameGeneration || 0)) return;
	  const runes = Array.isArray(response) ? response : Array.isArray(response?.runes) ? response.runes : null;
      if (!runes) throw new Error("绝活哥符文数据不完整");
	  if (response?.reason === "riot-key-missing") {
		state.specialistRuneFailures.set(target.key, { reason: "riot-key-missing", at: Date.now() });
		state.specialistRunes.delete(target.key);
	  } else if (response?.reason === "no-position-sample" && !runes.length) {
		state.specialistRuneFailures.set(target.key, { reason: "no-position-sample", at: Date.now() });
		state.specialistRunes.set(target.key, []);
	  } else if (String(response?.reason || "").startsWith("upstream-")) {
		state.specialistRuneFailures.set(target.key, { reason: String(response.reason), at: Date.now() });
		state.specialistRunes.delete(target.key);
	  } else {
		state.specialistRunes.set(target.key, runes);
		state.specialistRuneFailures.delete(target.key);
	  }
      if (runes.length) ensurePerks();
	      if (specialistRequestTarget(state.live)?.key === target.key) renderLive();
    } catch (error) {
	  if (gameGeneration === Number(state.liveGameGeneration || 0) && error.name !== "RequestCancelled") state.specialistRuneFailures.set(target.key, { reason: "request-failed", at: Date.now() });
    } finally {
      if (gameGeneration === Number(state.liveGameGeneration || 0)) state.specialistRuneFlights.delete(target.key);
	      if (specialistRequestTarget(state.live)?.key === target.key) renderLive();
    }
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
	  const key = `${gameId}:${phase}:${playersReceived}:${rendered100}:${rendered200}`;
	  if (state.liveRosterRenderDiagnostics?.has(key)) return;
	  state.liveRosterRenderDiagnostics ||= new Set();
	  state.liveRosterRenderDiagnostics.add(key);
	  void fetch("/api/diagnostics/client", {
	    method: "POST", credentials: "same-origin", headers: { "Accept": "application/json", "Content-Type": "application/json" },
	    body: JSON.stringify({ event: "live_roster_rendered", reason: "render", phase, gameId, queueId: Number(data?.queueId || 0), playersReceived, rendered100, rendered200 }),
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

	function liveCurrentPositionChip(data) {
	  const recommendation = liveRecommendationsFor(data) || {};
	  const positions = Array.isArray(recommendation.positions) ? recommendation.positions : [];
	  if (!positions.length) return "";
	  const self = (data?.players || []).find((player) => player.isCurrent);
	  const target = liveRecommendationTarget(data);
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
	    const note = state.settings.liveRefresh ? "自动刷新已开启" : "手动刷新";
    nodes.liveSessionSummary.hidden = false;
	    nodes.liveSessionSummary.innerHTML = data.available
	      ? `<span class="state-chip success">${escapeHTML(phase)}</span><div class="live-session-copy"><strong>${escapeHTML(data.queueLabel || "当前对局")}</strong><span>${escapeHTML(liveModeLabel(data))} · ${escapeHTML(liveMapLabel(data.mapId))} · ${escapeHTML(note)}</span></div>${liveCurrentPositionChip(data)}`
      : `<span class="state-chip">${escapeHTML(phase)}</span><div class="live-session-copy"><strong>${phase === "大厅" ? "等待进入对局" : escapeHTML(phase)}</strong><span>进入英雄选择或游戏后自动读取 · ${escapeHTML(note)}</span></div>`;
  }

  function renderLive() {
    if (state.destroyed || (state.section && state.section !== "live")) return;
    const toolbar = nodes.liveRefresh?.closest(".live-toolbar");
    if (!connected()) {
      // 未连接时收起工具栏（刷新按钮无意义），只保留居中的提示卡。
      if (toolbar) toolbar.hidden = true;
      renderSessionSummary(null);
      nodes.liveContent.innerHTML = emptyState("等待英雄联盟客户端", "登录国服客户端并进入英雄选择或对局后，这里会自动展示队伍信息与推荐配置。", false);
      return;
    }
    if (toolbar) toolbar.hidden = false;
    if (state.liveLoading && !state.live) {
      nodes.liveContent.innerHTML = '<div class="gameplay-skeleton"><span></span><span></span><span></span></div>';
      return;
    }
    if (state.liveError) {
      renderSessionSummary(null);
      nodes.liveContent.innerHTML = emptyState("实时对局读取失败", state.liveError, true);
      nodes.liveContent.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadLive(true));
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
    nodes.liveContent.innerHTML = renderRecommendationArea(data);
    bindLiveContent();
    applyRenderedMetricStyles(nodes.liveContent);
    prepareImages(nodes.liveContent);
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
	  const championId = liveDisplayedChampionId(member, currentChampionId);
	  return {
		name: maskedPlayerName(member, index),
		champion: member.championName || "英雄待确认",
		profileURL: Number(member.profileIconId) > 0 ? proxyAsset(assetPath("profile", member.profileIconId)) : "",
		championURL: proxyAsset(assetPath("champion", championId)),
	  };
	});
	const direct = source === "session" || source === "both";
	const tooltip = direct
	  ? `客户端直接给出的组队信息${source === "both" ? "\n最近战绩也支持这一判断" : ""}`
	  : `预组队推测\n最近战绩中共同出现至少 ${LIVE_PREMADE_MIN_SHARED_GAMES} 场`;
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

  function renderLivePlayer(player, index, arenaMode = false, currentChampionId = 0, premadePlayers = [], recentPositions = "") {
    const rank = player.rank;
    const stats = player.modeStats || {};
    const rankedRecord = player.recentRankedRecord;
    const recordGames = Number(rankedRecord?.games) || 0;
    const displayStats = recordGames ? {
      games: recordGames, wins: Number(rankedRecord.wins) || 0, losses: Number(rankedRecord.losses) || 0,
      winRate: (Number(rankedRecord.wins) || 0) * 100 / recordGames, kda: stats.kda,
    } : stats;
    const displayName = maskedPlayerName(player, index);
    const championId = liveDisplayedChampionId(player, currentChampionId);
	const historyState = String(player?.historyState || "empty").trim().toLowerCase();
	const historyPending = state.liveLoading === true;
    const rankCopy = rank?.tier ? rankTitle(rank) : "未定级";
    const contextCopy = arenaMode ? rankCopy : `${positionLabel(player.position)} · ${rankCopy}`;
    const rowTone = player.isCurrent ? " is-self" : player.isAlly ? " is-ally" : "";
    const premadeTag = renderLivePremadeTag(player, premadePlayers, currentChampionId);
	const emptySummary = historyState === "unavailable" ? "客户端未公开该玩家" : historyState === "failed" ? "读取失败" : "暂无样本";
	const historySummary = historyPending
	  ? '<dl class="is-history-pending" aria-label="战绩读取中"><div><span class="live-history-skeleton"></span></div><div><span class="live-history-skeleton"></span></div><div><span class="live-history-skeleton"></span></div></dl>'
	  : `<dl><div><dt>${recordGames ? `近 ${number(recordGames)} 局` : "当前模式"}</dt><dd>${displayStats.games ? `${number(displayStats.wins)}胜 ${number(displayStats.losses)}负` : emptySummary}</dd></div><div><dt>胜率</dt><dd class="win-rate-value">${displayStats.games ? percent(displayStats.winRate) : "—"}</dd></div><div><dt>KDA</dt><dd>${displayStats.games ? `${kda(displayStats.kda)}:1` : "—"}</dd></div></dl>`;
	return `<article class="live-player${rowTone}">${iconFigure("champion", championId, player.championName, "live")}<div class="live-player-copy"><div class="live-player-identity"><button class="live-player-name" type="button" ${player.playerRef ? `data-player-ref="${escapeHTML(player.playerRef)}"` : "disabled"} data-tooltip="${escapeHTML(displayName)}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(displayName)}</button>${player.isCurrent ? '<span class="self-chip">自己</span>' : ""}${premadeTag}</div><span>${escapeHTML(contextCopy)}</span>${recentPositions}</div>${historySummary}</article>`;
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

  function renderRecommendationArea(data) {
    const self = (data.players || []).find((player) => player.isCurrent);
    const target = liveRecommendationTarget(data);
    const payload = liveRecommendationsFor(data) || {};
    const waiting = !data.available;
    const randomPending = Boolean(self?.championPickPending) || Number(self?.championPickIntent) < 0;
    const noChampion = !waiting && !target;
    const augmentSource = liveAugmentRecommendationSource(data);
    const capabilities = recommendationCapabilities(data, payload);
    const tabs = recommendationTabSpecs(capabilities);
    const tabKeys = tabs.map(([key]) => key);
    const activeTab = recommendationActiveTab(tabKeys, capabilities);
    if (activeTab !== state.recommendationTab) state.recommendationTab = activeTab;
    const selected = capabilities.hasRunes ? selectedRuneRecommendation(data) : null;
    const selectedComplete = Boolean(selected && selected.selectedComplete !== false && (selected.selectedComplete !== true || selected.selectedPerkIds?.length === 9) && (selected.selectedPerkIds || selected.perkIds || []).length >= 6);
    const tab = (key, label) => `<button type="button" role="tab" id="recommendation-tab-${key}" aria-controls="recommendation-panel-${key}" aria-selected="${activeTab === key}" tabindex="${activeTab === key ? "0" : "-1"}" class="${activeTab === key ? "is-active" : ""}" data-recommendation-tab="${key}">${label}</button>`;
    // 页签内容还在拉取时，只在 tab 栏「下方」的这块面板上盖一层加载遮罩，
    // tab 栏本身保持可点击可切换。提示文案按页签内容区分。
    const loadingCopy = {
      runes: ["正在读取符文推荐", "从 OP.GG 拉取当前英雄与位置的符文分布"],
      build: ["正在读取海克斯、出装与技能", "按当前模式整理海克斯、鞋子、核心装备与技能加点"],
      insight: ["正在读取双方玩家战绩", "逐个查询十名玩家的段位与最近对局"],
    };
    const panelLoading = (key) => {
      if (!recommendationPanelBusy(key, target, payload)) return "";
      const [title, detail] = loadingCopy[key] || ["正在读取数据", "稍候片刻"];
      return `<div class="panel-loading" role="status" aria-live="polite"><div class="panel-loading-content"><span class="panel-spinner" aria-hidden="true"></span><strong>${escapeHTML(title)}</strong><small>${escapeHTML(detail)}</small></div></div>`;
    };
    const panel = (key, content) => `<div id="recommendation-panel-${key}" class="recommendation-panel${recommendationPanelBusy(key, target, payload) ? " is-loading" : ""}" role="tabpanel" aria-labelledby="recommendation-tab-${key}" ${activeTab !== key ? "hidden" : ""}>${content}${panelLoading(key)}</div>`;
    const content = {};
    if (waiting) {
      content.runes = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示对应模式的符文推荐。");
      content.insight = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示双方玩家与最新战绩。");
      content.build = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示召唤师技能、技能加点与装备分布。");
    } else {
      content.insight = renderLiveInsights(data);
      if (noChampion) {
        const pendingTitle = randomPending ? "随机待定" : "请先选定英雄";
        const pendingCopy = randomPending ? "英雄尚未确定，锁定后自动加载。" : "点击预选或锁定英雄后自动读取推荐数据。";
        content.runes = recommendationEmptyPanel(pendingTitle, pendingCopy);
        content.build = recommendationEmptyPanel(pendingTitle, pendingCopy);
      } else {
        if (capabilities.hasRunes) {
          const runePanel = renderRuneRecommendations(data, true);
          content.runes = `${renderChampionRecommendationHeader(payload.hero, self, data)}${runePanel}<footer class="recommendation-action"><div><strong>${selected ? `已选择：${escapeHTML(runeConfigurationTitle(selected))}` : "请选择一套完整符文"}</strong><span>${selectedComplete ? "将新建符文页并设为当前页" : "这套数据尚不完整，暂时不能应用"}</span></div><button class="button button-primary apply-runes" type="button" data-apply-runes="selected" ${!selectedComplete || data.phase !== "ChampSelect" ? "disabled" : ""}>${data.phase === "ChampSelect" ? "应用所选符文" : "仅英雄选择阶段可应用"}</button></footer>`;
        }
		const recommendationHeader = renderChampionRecommendationHeader(payload.hero, self, data);
		const augmentContent = capabilities.hasAugments && augmentSource ? renderLiveAugmentRecommendations(data, augmentSource) : "";
		content.build = `${recommendationHeader}${augmentContent}${renderBuildRecommendation(payload.build, self, data.championAbilities || [], data)}`;
      }
    }
	    const rosterNotice = data.champSelectNotice ? `<p class="live-roster-notice" role="note"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="M12 11v5m0-9h.01"></path></svg><span>${escapeHTML(data.champSelectNotice)}</span></p>` : "";
	    return `<section class="recommendation-area">${renderRecommendationDataNotices(payload)}<div class="recommendation-tab-row"><div class="recommendation-tabs" role="tablist" aria-label="推荐类型">${tabs.map(([key, label]) => tab(key, label)).join("")}</div>${rosterNotice}</div>${tabs.map(([key]) => panel(key, content[key] || "")).join("")}</section>`;
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
    const rows = liveChampionAugmentRows(data, source).slice(0, 9);
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
        ? [["win", "胜率", row.winRate, percent], ["pick", "选用率", row.pickRate, percent]]
        : [["win", "胜率", row.winRate, percent], ["games", "场次", row.games, number]];
      const stats = metrics.filter(([, , value]) => hasMetric(value)).map(([key, label, value, formatter]) => {
        const suffix = key === "games" ? " 场" : "";
        return `<span class="live-augment-stat is-${key}"><span>${label}</span><b>${escapeHTML(formatter(value))}${suffix}</b></span>`;
      }).join("");
      const gradeValue = String(row.grade || row.tier || "").trim().toUpperCase();
      const grade = ["OP", "S", "A", "B", "C", "D", "F"].includes(gradeValue) ? gradeValue : "";
      const gradeBadge = grade ? `<b class="augment-grade is-${grade}" aria-label="${grade}档">${grade}</b>` : "";
      const statsMarkup = stats ? `<small class="live-augment-stats">${stats}</small>` : "";
      return `<article class="live-augment-option is-${rarity.key}" tabindex="0" data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}">${renderLiveAugmentIcon(row, source)}<span><span class="live-augment-title">${gradeBadge}<strong>${escapeHTML(asset.name || `海克斯 ${asset.id || row.id}`)}</strong></span>${statsMarkup}</span></article>`;
    };
    const definitions = [["silver", "白银"], ["gold", "黄金"], ["prismatic", "棱彩"]];
    const hasUnknown = rows.some((row) => !definitions.some(([key]) => normalizeAugmentRarity(row.rarity || row.assets?.[0]?.rarity).key === key));
    const columns = [...definitions, ...(hasUnknown ? [["unknown", "品质待确认"]] : [])].map(([key, label]) => {
      const gradeRank = (row) => ({ OP: 0, S: 1, A: 2, B: 3, C: 4, D: 5, F: 6 }[String(row?.grade || row?.tier || "").trim().toUpperCase()] ?? 7);
      const matches = rows
        .filter((row) => normalizeAugmentRarity(row.rarity || row.assets?.[0]?.rarity).key === key)
        .sort((left, right) => gradeRank(left) - gradeRank(right)
          || (Number(right.score) || 0) - (Number(left.score) || 0)
          || (Number(right.winRate) || 0) - (Number(left.winRate) || 0)
          || (Number(right.games) || 0) - (Number(left.games) || 0));
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
	if (state.liveLoading === true) return '<div class="insight-match-row is-history-pending" aria-label="战绩读取中"><span class="live-history-skeleton"></span><span class="live-history-skeleton"></span><span class="live-history-skeleton"></span></div>';
    const games = (player.recentGames || []).slice(0, 8);
	if (!games.length) {
	  const historyState = String(player?.historyState || "empty").trim().toLowerCase();
	  if (historyState === "unavailable") return '<div class="insight-match-row"><small class="insight-none">客户端未公开该玩家</small></div>';
	  if (historyState === "failed") return '<div class="insight-match-row"><small class="insight-none">读取失败</small><button class="text-button insight-retry" type="button" data-live-history-retry>重试</button></div>';
	  return '<div class="insight-match-row"><small class="insight-none">该玩家当前模式暂无最近战绩</small></div>';
	}
    const cells = games.map((game) => `<span class="insight-match is-${game.win ? "win" : "loss"}" data-tooltip="${escapeHTML(`${game.championName || "英雄"} · ${number(game.kills)}/${number(game.deaths)}/${number(game.assists)}${Number(game.cs) > 0 ? ` · CS ${number(game.cs)}` : ""} · ${game.win ? "胜利" : "失败"}`)}" data-tooltip-size="compact">${iconFigure("champion", game.championId, game.championName, "tiny")}<b>${number(game.kills)}/${number(game.deaths)}/${number(game.assists)}</b>${insightScore(game)}</span>`).join("");
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
	    if (!["InProgress", "Reconnect"].includes(data?.phase) || data?.arenaGrouped !== true || players.length < 6 || players.length > 18) return [];
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
	      return { key, label: clientName || mascot?.name || `小队 ${index + 1}`, players: grouped.get(key) };
	    });
	  }

	  function insightTeamLayout(players, enabled) {
	    const teams = new Map([100, 200].map((teamID) => [teamID, players.filter((player) => Number(player.teamId) === teamID)]));
	    if (!enabled) return { teams, aligned: false, reason: "" };
	    const positions = { top: 1, jungle: 2, middle: 3, bottom: 4, utility: 5, other: 6 };
	    for (const teamID of [100, 200]) {
	      const rows = teams.get(teamID) || [];
	      const values = rows.map((player) => String(player.position || "").toLowerCase());
	      if (rows.length !== 5 || values.some((position) => !positions[position]) || new Set(values).size !== values.length) {
	        return { teams: new Map([100, 200].map((id) => [id, players.filter((player) => Number(player.teamId) === id)])), aligned: false, reason: "位置数据重复或缺失，已保留客户端顺序" };
	      }
	      teams.set(teamID, [...rows].sort((left, right) => positions[left.position] - positions[right.position]));
	    }
	    return { teams, aligned: true, reason: "" };
	  }

	  function renderLiveInsights(data) {
	    const players = data.players || [];
	    if (!players.length) return '<div class="recommendation-empty"><strong>暂无玩家数据</strong><p>进入英雄选择或对局后，这里会展示自己、队友与对手的队伍信息与最近战绩。</p></div>';
		const arenaMode = liveAugmentRecommendationSource(data) === "arena";
		const basePlayers = orderLivePlayers(players, !arenaMode);
		const orderedPlayers = arenaMode || !state.settings.detailPositionAlign ? clusterPremadePlayers(basePlayers) : basePlayers;
	    recordLiveRosterRendered(data, orderedPlayers.filter((player) => player.teamId === 100).length, orderedPlayers.filter((player) => player.teamId === 200).length);
	    const selfPlayer = orderedPlayers.find((player) => player.isCurrent && [100, 200].includes(Number(player.teamId)));
	    const selfTeam = Number(selfPlayer?.teamId) || 100;
	    const foeTeam = selfTeam === 200 ? 100 : 200;
	    const relativeTeamsKnown = Boolean(selfPlayer);
	    const alignment = insightTeamLayout(orderedPlayers, !arenaMode && state.settings.detailPositionAlign === true);
	    const headerNotice = !relativeTeamsKnown ? "无法确定你所在阵营，按蓝方/红方展示" : alignment.reason;
	    const team = (teamID, label, tone, source = orderedPlayers) => {
	      const rows = source.filter((player) => teamID === null || player.teamId === teamID).map((player, index) => `${renderLivePlayer(player, index, arenaMode, data.currentChampionId, orderedPlayers, renderLiveRecentPositions(player, data.queueId))}${renderInsightMatches(player)}`).join("");
	      return `<section class="live-team is-${tone}"><header><h3>${escapeHTML(label)}</h3><span>${escapeHTML(headerNotice || "当前模式最新战绩")}</span></header><div class="live-player-list is-insight">${rows || '<p class="section-empty">客户端暂未公开这一队的玩家</p>'}</div></section>`;
	    };
		if (arenaMode) {
		  const groups = arenaLivePlayerGroups(data, orderedPlayers);
		  if (groups.length >= 2) {
		    return `<div class="live-teams is-insight is-arena is-grouped">${groups.map((group) => team(null, group.label, "arena", group.players)).join("")}</div>`;
		  }
		  const title = data.phase === "ChampSelect" ? "己方小队" : "全部玩家";
		  return `<div class="live-teams is-insight is-arena">${team(null, title, "arena")}</div>`;
		}
	    if (!relativeTeamsKnown) return `<div class="live-teams is-insight">${team(100, "蓝方", "blue", alignment.teams.get(100))}${team(200, "红方", "red", alignment.teams.get(200))}</div>`;
	    return `<div class="live-teams is-insight">${team(selfTeam, "我方", "blue", alignment.teams.get(selfTeam))}${team(foeTeam, "对方", "red", alignment.teams.get(foeTeam))}</div>`;
	  }

  function liveChampionTierBadge(value) {
    const present = value !== null && value !== undefined && String(value).trim() !== "";
    const parsed = Number(value);
    const key = present && Number.isFinite(parsed) && parsed >= 0 && parsed <= 5 ? (parsed === 0 ? "op" : String(parsed)) : "";
    const label = key ? (parsed === 0 ? "OP" : String(parsed)) : "—";
    return key
      ? `<img class="tier-badge is-metric" src="/tier-icons/${key}.svg" alt="梯度 ${label}" decoding="async">`
      : `<span class="tier-badge-fallback is-metric">${label}</span>`;
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
      const name = item.championName || `英雄 ${item.championId || item.id}`;
      const hasWinRate = item.winRate != null && Number.isFinite(Number(item.winRate));
      const detail = [hasWinRate && `${percent(item.winRate)} 胜率`, Number(item.games) > 0 && `${number(item.games)} 场`].filter(Boolean).join(" · ");
      const detailHTML = [hasWinRate && `<b class="win-rate-value">${percent(item.winRate)}</b> 胜率`, Number(item.games) > 0 && `${number(item.games)} 场`].filter(Boolean).join(" · ");
      return `<span class="recommendation-matchup is-${tone}" data-tooltip="${escapeHTML(detail || name)}" data-tooltip-size="compact">${iconFigure("champion", item.championId || item.id, name, "small")}<span><b>${escapeHTML(name)}</b><small>${detailHTML || "暂无统计"}</small></span></span>`;
      }).join("") + Array.from({ length: 3 - values.length }, () => '<span class="recommendation-matchup is-placeholder" aria-hidden="true"></span>').join("");
    };
    const automatic = !target?.positionOverride && payload.positionSource && payload.positionSource !== "requested";
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
	  ["pick", "选取率", stats.pickRate, true],
	  ["ban", "禁用率", stats.banRate, hasBanRate],
	].filter(([, , value, enabled]) => enabled && hasMetric(value)).map(([tone, label, value]) => `<div class="is-${tone}"><dt>${label}</dt><dd>${rate(value)}</dd></div>`).join("");
	const statsContent = stats.emptyReason
	  ? `<p class="champion-stats-empty" role="status">${escapeHTML(stats.emptyReason)}</p>`
	  : summaryMetrics ? `<dl class="champion-summary-stats">${summaryMetrics}</dl>` : "";
	const hasCounters = payload.hasCounters !== false;
	const matchupHTML = hasCounters ? `<div class="champion-matchups"><div><span>优势对抗</span>${matchups(stats.strongAgainst, "strong")}</div><div><span>劣势对抗</span>${matchups(stats.weakAgainst, "weak")}</div></div>` : "";
	const showTier = Boolean(liveAugmentRecommendationSource(data));
	const tierContent = showTier ? `<div class="champion-summary-tier"><span>梯度</span><strong>${liveChampionTierBadge(stats.tier)}</strong></div>` : "";
	return `<section class="recommendation-champion-summary${hasCounters ? "" : " is-no-counters"}${showTier ? " has-tier" : ""}"><div class="champion-summary-main">${championId ? iconFigure("champion", championId, self?.championName || "当前英雄", "live") : '<span class="game-icon is-live">?</span>'}<div><strong>${escapeHTML(self?.championName || (championId ? "当前英雄" : "尚未选择英雄"))}</strong><span>${escapeHTML(positionLabel(livePositionDisplay(resolvedPosition)))}</span></div>${statsContent}</div>${positionControls}${matchupHTML}${tierContent}</section>`;
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
    const sections = [{ key: "opgg", title: "OPGG", items: opggFallback }];
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

  function renderRuneSourceSection(section, championSelected) {
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
		const specialistNoPositionSample = specialistFailureInfo?.reason === "no-position-sample";
		const specialistTimeout = specialistFailureInfo?.reason === "upstream-timeout";
		const specialistThrottled = specialistFailureInfo?.reason === "upstream-throttled";
		const specialistUpstreamError = specialistFailureInfo?.reason === "upstream-error";
		const specialistRetryable = specialistTimeout || specialistThrottled || specialistUpstreamError || specialistFailureInfo?.reason === "request-failed";
		const unavailableReason = {
		  specialist: section.unsupported ? "绝活哥榜单仅支持单双排、灵活组排和召唤师峡谷自定义对局。" : specialistKeyMissing ? "未配置 Riot Key，暂时无法读取韩服绝活哥符文。" : specialistNoPositionSample ? `该绝活哥最近 10 局没有打过${positionLabel(livePositionDisplay(target?.position || ""))}。` : specialistTimeout ? "韩服接口响应超时，请稍后重试。" : specialistThrottled ? "请求过于频繁，约 1 分钟后可重试。" : specialistUpstreamError ? "上游数据异常，请稍后重试。" : "最近对局中没有找到完整且可核验的该英雄符文。",
      pro: "当前数据源不提供可核验的职业选手身份与完整符文，暂不展示。",
    };
		const emptyTitle = !championSelected ? "请先选定英雄" : section.unsupported ? "当前队列没有可用的绝活哥榜单" : section.loading ? "正在读取韩服绝活哥符文" : specialistKeyMissing ? "未配置 Riot Key" : specialistNoPositionSample ? "最近 10 局没有该位置样本" : specialistTimeout ? "韩服接口响应超时" : specialistThrottled ? "请求过于频繁" : specialistUpstreamError ? "上游数据异常" : section.failed ? "韩服绝活哥符文读取失败" : section.key === "opgg" ? "等待完整符文数据" : section.key === "specialist" ? "暂无可核验的韩服绝活哥符文" : "暂无可核验数据源";
    const content = section.items.length && section.key === "specialist"
      ? renderSpecialistPlayers(section.items)
      : section.items.length
      ? `<div class="rune-choice-list" role="radiogroup" aria-label="${escapeHTML(section.title)}符文">${section.items.map((config, index) => renderRuneChoice({ ...config, sourceKey: section.key, key: config.key || (section.key === "opgg" && index === 0 ? "opgg" : `${section.key}-${index}`) })).join("")}</div>`
	  : `<div class="recommendation-empty"><strong>${emptyTitle}</strong><p>${section.loading ? "正在核对专家榜玩家最近对局中的完整符文，通常需要 5-15 秒。" : specialistNoPositionSample || specialistRetryable ? unavailableReason.specialist : section.failed ? "本次后台读取未完成，稍后刷新时会自动重试。" : unavailableReason[section.key] || "OPGG 返回完整主系、副系与属性碎片后即可选择。"}</p>${specialistKeyMissing ? '<button class="text-button" type="button" data-open-riot-settings>去设置</button>' : specialistRetryable ? '<button class="text-button" type="button" data-retry-specialist-runes>重试</button>' : ""}</div>`;
    return `<section class="rune-source-section">${content}</section>`;
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
    const tabs = limited.map(([key, games]) => {
      const config = games[0]?.config;
      const active = key === activeKey;
      const name = String(config?.playerName || "").trim();
      const tagLine = String(config?.tagLine || "").trim();
      // Keep rune selection separate from navigation; tournament display names
      // are not Riot IDs and must never be used to look up a KR account.
      const canOpen = source === "specialist" && name && tagLine && config?.region === "kr";
      if (!canOpen) return `<button type="button" role="tab" class="specialist-player-tab${source === "pro" ? " pro-player-tab" : ""}${active ? " is-active" : ""}" data-specialist-player="${escapeHTML(key)}" data-player-source="${source}" aria-selected="${active}">${source === "pro" ? `<span class="pro-player-name">${escapeHTML(config?.title || key)}</span>${proRuneRecordLabel(config)}` : escapeHTML(key)}</button>`;
      return `<div class="specialist-player-tab specialist-player-split${active ? " is-active" : ""}"><button type="button" role="tab" class="specialist-player-name" data-specialist-player="${escapeHTML(key)}" data-player-source="specialist" aria-selected="${active}" aria-label="查看 ${escapeHTML(name)} 的推荐符文">${escapeHTML(name)}</button><button type="button" class="specialist-player-overview" data-specialist-overview data-player-name="${escapeHTML(name)}" data-player-tag="${escapeHTML(tagLine)}" aria-label="查看 ${escapeHTML(name)}#${escapeHTML(tagLine)} 的总览">总览</button></div>`;
    }).join("");
    const panel = limited.find(([key]) => key === activeKey) || limited[0];
    const playerGames = panel?.[1] || [];
    const games = source === "pro" ? [...playerGames].sort((a, b) => Number(b.config.playedAt || 0) - Number(a.config.playedAt || 0)).slice(0, 5) : playerGames;
    const rows = games.map(({ config, choiceKey }) => {
      const selected = String(state.selectedRecommendation || "") === choiceKey;
      const opponentName = config.opponentChampionName || (config.opponentChampionId ? `英雄 ${config.opponentChampionId}` : "暂无对位数据");
      const position = config.position ? source === "pro" ? config.position : positionLabel(livePositionDisplay(config.position)) : "";
      const tone = config.winKnown === false ? "is-unknown" : config.result === "win" ? "is-win" : config.result === "loss" ? "is-loss" : "is-remake";
      const championID = Number(config.championId || config.championID);
      const championIcon = championID > 0
        ? iconFigure("champion", championID, config.championName || "英雄", "small")
        : '<span class="rune-choice-champion-placeholder" aria-hidden="true"></span>';
      const title = runeConfigurationTitle(config);
      const opponentPlayer = config.opponentPlayerName
        ? `${config.opponentPlayerName}${config.opponentTagLine ? `#${config.opponentTagLine}` : ""}`
        : "暂无召唤师信息";
	  const opponentRankTitle = config.opponentTier ? rankTitle({ tier: config.opponentTier, division: config.opponentDivision }) : "";
	  const opponentWinRate = config.opponentWinRate != null && Number.isFinite(Number(config.opponentWinRate))
	    ? `胜率 ${Math.round(Number(config.opponentWinRate))}%`
	    : "";
	  const opponentRankText = [opponentRankTitle, opponentWinRate].filter(Boolean).join(" · ");
	  const opponentRank = opponentRankText
	    ? `<span class="specialist-opponent-rank">${config.opponentTier ? rankCrestIcon(config.opponentTier) : ""}<small>${escapeHTML(opponentRankTitle)}${opponentRankTitle && opponentWinRate ? " · " : ""}${opponentWinRate ? `胜率 <b class="win-rate-value">${Math.round(Number(config.opponentWinRate))}%</b>` : ""}</small></span>`
	    : "";
      const itemIDs = (config.itemIds || config.itemIDs || []).map(Number).filter((id) => Number.isInteger(id) && id > 0).slice(0, 7);
      const items = itemIDs.length
        ? `<div class="specialist-game-items"><span>最终装备</span><div>${itemIDs.map((id) => renderItemIcon(id)).join("")}</div></div>`
        : "";
	      return `<div class="specialist-game-row ${tone}${selected ? " is-selected" : ""}" role="radio" aria-checked="${selected}" tabindex="0" data-rune-choice="${escapeHTML(choiceKey)}"><div class="specialist-game-head"><span class="specialist-game-choice"><span class="radio-mark" aria-hidden="true"></span>${championIcon}<span class="specialist-game-title"><strong>${escapeHTML(title)}</strong><span class="specialist-game-meta">${position ? `<span>${escapeHTML(position)}</span>` : ""}${config.eventLabel ? `<span class="pro-event-label">${escapeHTML(config.eventLabel)}</span>` : ""}${config.winKnown === false ? '<span data-tooltip="该局胜负未获官方确认">胜负未确认</span>' : ""}</span></span></span><span class="specialist-opponent is-${tone.slice(3)}"><span class="specialist-result-dot" aria-hidden="true"></span>${config.opponentChampionId ? iconFigure("champion", config.opponentChampionId, opponentName, "small") : ""}<span class="specialist-opponent-copy"><b>${escapeHTML(opponentName)}</b><small>${escapeHTML(opponentPlayer)}</small></span>${opponentRank}</span></div><div class="specialist-game-content"><div class="specialist-game-runes">${renderUnifiedRuneBoard(config)}${items}${config.selectedComplete === false ? '<p class="pro-rune-status">属性碎片：上游未提供完整槽位；这套数据尚不完整，暂时不能应用。</p>' : ""}</div></div></div>`;
    }).join("");
    return `<div class="specialist-player-tabs" role="tablist" aria-label="${source === "pro" ? "职业选手" : "绝活哥玩家"}">${tabs}</div><div class="specialist-player-games" role="radiogroup" aria-label="${source === "pro" ? "职业比赛符文" : "绝活哥最近对局符文"}">${rows || '<p class="section-empty">暂无最近对局符文数据</p>'}</div>`;
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

  function renderRuneChoice(config) {
    const key = String(config.key || "");
    const selected = state.selectedRecommendation === key;
    const stats = config.stats || config;
    const games = config.championGames ?? stats.games ?? stats.play;
    const winRate = stats.winRate ?? stats.win_rate ?? (stats.win != null && games ? stats.win * 100 / games : null);
    const pickRate = stats.pickRate ?? stats.pick_rate;
    const specialistStats = `<dl class="rune-player-stats"><div><dt>胜率</dt><dd class="win-rate-value">${rate(winRate)}</dd></div><div><dt>场次</dt><dd>${compactNumber(games)}</dd></div><div><dt>选用率</dt><dd>${rate(pickRate)}</dd></div></dl>`;
    const riotID = `${config.playerName || ""}${config.tagLine ? `#${config.tagLine}` : ""}`;
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
    return `<article class="rune-choice-card${selected ? " is-selected" : ""}" role="radio" aria-checked="${selected}" tabindex="0" data-rune-choice="${escapeHTML(key)}"><div class="rune-choice-selector"><span class="radio-mark" aria-hidden="true"></span>${championIcon}${choiceCopy}${specialistStats}</div><div class="rune-choice-detail">${renderUnifiedRuneBoard(config)}</div></article>`;
  }

  function renderUnifiedRuneBoard(config) {
    const selectedIDs = (config.perkIds || config.selectedPerkIds || []).map(Number);
    const selected = new Set(selectedIDs);
    if (!state.perks) return `<div class="selected-rune-strip">${[...selected].map((id) => iconFigure("perk", id, `符文 ${id}`, "rune")).join("")}</div><p class="muted rune-loading-copy">正在读取完整符文树…</p>`;
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
    return `<div class="unified-rune-board">${renderStyle(config.primaryStyleId, false)}${renderStyle(config.subStyleId, true)}<section class="unified-rune-column rune-shards" aria-label="属性碎片"><header aria-hidden="true"></header><div class="unified-rune-row is-spacer" aria-hidden="true"></div>${fragments.map((slot, rowIndex) => `<div class="unified-rune-row">${(slot.perks || []).map((perk) => renderRuneOption(perk, fragmentSelections[rowIndex] === Number(perk.id))).join("")}</div>`).join("")}</section></div>${incomplete}`;
  }

  function renderRuneOption(perk, selected, fallbackID = 0) {
    const id = Number(perk?.id || fallbackID);
    const name = perk?.name || `符文 ${id}`;
    const explanation = runeShardDescription(id) || plainText(perk?.longDesc || perk?.shortDesc || name);
    const shardPath = dataDragonRuneShardPath(id);
    const icon = shardPath ? remoteStaticIcon("ddragon", shardPath, name, "rune", false) : perk?.iconPath ? assetIcon(perk.iconPath, name, "rune", false) : iconFigure("perk", id, name, "rune", false);
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
	    return `<span class="rune-option-button${selected ? " is-selected" : ""}" role="img" tabindex="-1" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-tooltip="${escapeHTML(tooltip)}">${icon}</span>`;
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

  function perkRecord(id) {
    const direct = (state.perks?.perks || []).find((item) => Number(item.id) === Number(id));
    if (direct) return direct;
    for (const style of state.perks?.styles || []) for (const slot of style.slots || []) {
      const found = (slot.perks || []).find((item) => Number(item.id) === Number(id));
      if (found) return found;
    }
    for (const slot of state.perks?.statModSlots || []) {
      const found = (slot.perks || []).find((item) => Number(item.id) === Number(id));
      if (found) return found;
    }
    return { id, name: `属性碎片 ${id}`, iconPath: assetPath("perk", id) };
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
    const gradeValue = String(option.grade || "B").trim().toUpperCase();
    const grade = ["OP", "S", "A", "B", "C", "D", "F"].includes(gradeValue) ? gradeValue : "B";
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
    return `<article class="arena-option-card live-arena-build-option${kind === "core" ? " arena-core-option" : ""}" data-metric-count="3"><div class="arena-option-main"><div class="arena-option-icons">${route}</div><b class="augment-grade is-${grade}">${grade}</b>${name ? `<strong>${escapeHTML(name)}</strong>` : ""}</div><dl>${cells}</dl></article>`;
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
    const skillPriority = (build.skillPriority || []).map((slot) => String(slot).toUpperCase());
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
    // 不再给低样本行加任何视觉标记：R30 的做法是整块压暗到 opacity .55、
    // 只有 hover 才恢复，用户看到的就是「装备图标全是灰的」。
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
    const runes = liveRecommendationsFor(data)?.runes || {};
    const opggItems = Array.isArray(runes.opgg) ? runes.opgg : runes.opgg ? [runes.opgg] : [];
    const opggFallback = opggItems.length
      ? opggItems.map((item) => ({ ...item, sourceLabel: "OPGG" }))
      : data?.clientRecommendation ? [{ ...data.clientRecommendation, title: "客户端内置（备用）", sourceLabel: "客户端内置" }] : [];
    const normalized = opggFallback.map((item, index) => ({ ...item, key: item.key || (index === 0 ? "opgg" : `opgg-${index}`) }));
    if (recommendationQueueHasTopPlayers(data)) {
      normalized.push(
        ...specialistRunesFor(data).map((item, index) => ({ ...item, sourceLabel: "绝活哥", key: item.key || `specialist-${index}` })),

      );
    }
    normalized.push(...proRunesFor(data).map((item, index) => ({ ...item, sourceLabel: "职业选手", key: item.key || `pro-${index}` })));
    const selected = normalized.find((item) => String(item.key) === String(state.selectedRecommendation)) || normalized[0] || null;
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

  function bindLiveContent() {
	nodes.liveContent.querySelector("[data-live-history-retry]")?.addEventListener("click", () => loadLive(true));
    // 对局页里点击玩家名称：在当前页面上以覆盖层打开该玩家的总览，
    // 不再跳转到总览页；对局数据来自本机客户端，必然是国服玩家。
    for (const button of nodes.liveContent.querySelectorAll("[data-player-ref]")) button.addEventListener("click", () => {
      openPlayerOverlay({ playerRef: button.dataset.playerRef, region: "", label: playerButtonLabel(button) });
    });
    for (const button of nodes.liveContent.querySelectorAll("[data-recommendation-tab]")) button.addEventListener("click", () => {
      state.recommendationTab = button.dataset.recommendationTab;
      state.recommendationTabTouched = true;
      if (state.recommendationTab === "build") { ensureItems(); ensureSummonerSpells(); }
      renderLive();
      nodes.liveContent.querySelector(`[data-recommendation-tab="${state.recommendationTab}"]`)?.focus();
    });
    for (const button of nodes.liveContent.querySelectorAll("[data-rune-source]")) button.addEventListener("click", () => {
      const source = button.dataset.runeSource;
      if (!["opgg", "specialist", "pro"].includes(source)) return;
      state.runeSourceTab = source;
      renderLive();
      nodes.liveContent.querySelector(`[data-rune-source="${source}"]`)?.focus();
    });
    for (const button of nodes.liveContent.querySelectorAll("[data-specialist-overview]")) button.addEventListener("click", () => {
      const gameName = button.dataset.playerName;
      const tagLine = button.dataset.playerTag;
      if (!gameName || !tagLine) return;
      window.dispatchEvent(new CustomEvent("deep-legends:open-player", {
        detail: { gameName, tagLine, region: "kr", source: "live-specialist" },
      }));
    });
    for (const button of nodes.liveContent.querySelectorAll("[data-specialist-player]")) button.addEventListener("click", () => {
      const pro = button.dataset.playerSource === "pro";
      const target = pro ? proRequestTarget(state.live) : specialistRequestTarget(state.live);
      if (!target) return;
      state.specialistPlayerTabs.set(pro ? `pro:${target.key}` : target.key, button.dataset.specialistPlayer || "");
      renderLive();
      nodes.liveContent.querySelector(`[data-specialist-player="${CSS.escape(button.dataset.specialistPlayer || "")}"]`)?.focus();
    });
    nodes.liveContent.querySelector("[data-retry-pro-runes]")?.addEventListener("click", () => { void ensureProRunes(state.live, true); });
    for (const button of nodes.liveContent.querySelectorAll("[data-live-position]")) button.addEventListener("click", () => selectLivePosition(livePositionValue(button.dataset.livePosition)));
		nodes.liveContent.querySelector("[data-open-riot-settings]")?.addEventListener("click", () => {
	  window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: "settings", page: "privacy" } }));
	});
	nodes.liveContent.querySelector("[data-retry-specialist-runes]")?.addEventListener("click", () => {
	  const target = specialistRequestTarget(state.live);
	  if (!target) return;
	  state.specialistRuneFailures.delete(target.key);
	  state.specialistRunes.delete(target.key);
	  void ensureSpecialistRunes(state.live);
	});
    nodes.liveContent.querySelector(".recommendation-tabs")?.addEventListener("keydown", (event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const tabs = [...event.currentTarget.querySelectorAll('[role="tab"]')];
      const current = tabs.indexOf(document.activeElement);
      const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
      tabs[next]?.click();
    });
    bindRuneChoiceButtons(nodes.liveContent);
    nodes.liveContent.querySelector("[data-apply-runes]")?.addEventListener("click", applyRunes);
    nodes.liveContent.querySelector("[data-apply-item-set]")?.addEventListener("click", applyItemSet);
  }

	function bindRuneChoiceButtons(root) {
	    for (const choice of root?.querySelectorAll("[data-rune-choice]") || []) {
	      const select = () => {
	        state.selectedRecommendation = choice.dataset.runeChoice;
	        renderLive();
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
      await api("/api/gameplay/runes/apply", { method: "POST", body: JSON.stringify({ championName: self.championName, source: recommendation.sourceLabel, championId: recommendation.championId || championId, primaryStyleId: recommendation.primaryStyleId, subStyleId: recommendation.subStyleId, selectedPerkIds: recommendation.selectedPerkIds || recommendation.perkIds || [] }) }, "apply-runes", 15000);
      showToast("符文已新建并设为当前页");
      button.textContent = "再次新增";
    } catch (error) { showToast(error.message); button.textContent = "应用所选符文"; }
    finally { button.disabled = false; }
  }

  async function applyItemSet(event) {
    const button = event.currentTarget;
    const self = (state.live?.players || []).find((player) => player.isCurrent);
    const build = liveRecommendationsFor(state.live)?.build;
    if (!self || !build) return;
    const payload = buildItemSetPayload(build, self, state.live);
    payload.storage = "recommended";
    if (!payload.championId || !payload.blocks.length) return;
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
    clearTimeout(state.currentGameTimer);
    if (!state.settings.liveRefresh || state.section !== "live" || document.hidden) return;
    state.liveTimer = setTimeout(() => loadLive(true), liveRefreshDelayMs(state.live?.phase, state.settings.liveInterval));
  }

  function bindSettings() {
    nodes.settingDefaultPage.value = state.settings.defaultPage;
    nodes.settingMatchCount.value = String(state.settings.matchCount);
    nodes.settingDefaultMatchFilter.value = state.settings.defaultMatchFilter;
    nodes.settingLiveRefresh.checked = state.settings.liveRefresh;
    nodes.settingLiveInterval.value = String(state.settings.liveInterval);
	    nodes.settingLiveOrder.value = state.settings.liveOrder;
	    nodes.settingDetailPositionAlign.checked = state.settings.detailPositionAlign;
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
    nodes.settingLiveRefresh.addEventListener("change", () => { state.settings.liveRefresh = nodes.settingLiveRefresh.checked; nodes.settingLiveInterval.disabled = !state.settings.liveRefresh; writeSetting("live-refresh", state.settings.liveRefresh); scheduleLiveRefresh(); renderLive(); });
    nodes.settingLiveInterval.disabled = !state.settings.liveRefresh;
    nodes.settingLiveInterval.addEventListener("change", () => { state.settings.liveInterval = normalizeLiveInterval(nodes.settingLiveInterval.value); writeSetting("live-interval", state.settings.liveInterval); scheduleLiveRefresh(); renderLive(); });
	    nodes.settingLiveOrder.addEventListener("change", () => { state.settings.liveOrder = normalizeLiveOrder(nodes.settingLiveOrder.value); writeSetting("live-order", state.settings.liveOrder); renderLive(); });
	    nodes.settingDetailPositionAlign.addEventListener("change", () => { state.settings.detailPositionAlign = nodes.settingDetailPositionAlign.checked; writeSetting("detail-position-align", state.settings.detailPositionAlign); renderLive(); });
    nodes.settingMaskNames.addEventListener("change", () => { state.settings.maskNames = nodes.settingMaskNames.checked; writeSetting("mask-names", state.settings.maskNames); renderPlayerTabs(); renderOverview(overviewGroupForSection()); renderLive(); if (state.overlay.length) renderOverlay(); });
    nodes.settingConfirmReplay.addEventListener("change", () => { state.settings.confirmReplay = nodes.settingConfirmReplay.checked; writeSetting("confirm-replay", state.settings.confirmReplay); });
  }

  function renderCapabilitySettings() {
    const labels = { summoner: "召唤师资料", "ranked-stats": "排位数据", "match-history": "战绩列表", "seven-day-history": "7 天统计样本", "match-details": "对局详情", "champion-mastery": "英雄熟练度", gameflow: "游戏流程", "gameflow-session": "当前对局", "champ-select": "英雄选择", "live-player-analysis": "队伍分析", "client-rune-recommendation": "客户端推荐符文", "champion-abilities": "英雄技能" };
	const capabilities = (state.lastCapabilities || []).filter((item) => item?.state !== "canceled");
	nodes.gameplaySettingsStatus.innerHTML = capabilities.length ? `<div class="gameplay-capabilities">${capabilities.map((item) => `<div><span class="capability-dot is-${escapeHTML(item.state)}" aria-hidden="true"></span><span><strong>${escapeHTML(labels[item.name] || item.name)}</strong><small>${escapeHTML(item.detail || capabilityAttemptSummary(item) || (item.state === "available" ? `已读取 ${number(item.count)} 项` : "当前不可用"))}</small></span><b>${item.state === "available" ? "可用" : item.state === "unsupported" ? "不支持" : "读取失败"}</b></div>`).join("")}</div>` : '<p class="muted">打开总览或对局后显示能力状态。</p>';
  }

	function capabilityAttemptSummary(capability) {
	  const sourceLabels = { lcu: "本机客户端", sgp: "腾讯 SGP", riot: "Riot API", opgg: "OP.GG" };
	  const outcomeLabels = { success: "成功", failed: "失败", disabled: "已关闭", "mode-unsupported": "模式不支持" };
	  const attempts = Array.isArray(capability?.attempts) ? capability.attempts : [];
	  return attempts.map((attempt) => {
		const source = sourceLabels[attempt?.source] || attempt?.source || "未知来源";
		const outcome = outcomeLabels[attempt?.outcome] || attempt?.outcome || "未知结果";
		return `${source}${outcome}${attempt?.message ? `：${attempt.message}` : ""}`;
	  }).join("；");
	}

  function emptyState(title, copy, retry) { return `<div class="gameplay-empty"><span aria-hidden="true">⬡</span><strong>${escapeHTML(title)}</strong><p>${escapeHTML(copy)}</p>${retry ? '<button class="text-button" type="button" data-gameplay-retry>重试</button>' : ""}</div>`; }
  function summonerLabel(summoner) { return `${summoner.gameName || summoner.displayName || "当前召唤师"}${summoner.tagLine ? `#${summoner.tagLine}` : ""}`; }
  function playerLabel(player) { return `${player.gameName || player.displayName || "隐藏玩家"}${player.tagLine ? `#${player.tagLine}` : ""}`; }
  function maskedPlayerName(player, index) { if (!state.settings.maskNames || player.isCurrent) return playerLabel(player); return player.hidden ? "隐藏玩家" : `玩家 ${String(index + 1).padStart(2, "0")}`; }
  function maskedListName(player, index) { if (!state.settings.maskNames) return player.displayName || "隐藏玩家"; return player.hidden ? "隐藏玩家" : `玩家 ${String(index + 1).padStart(2, "0")}`; }
  function playerParticipantName(player, index) { if (!state.settings.maskNames) return `${player.gameName || player.displayName || "隐藏玩家"}${player.tagLine ? `#${player.tagLine}` : ""}`; return player.hidden ? "隐藏玩家" : `玩家 ${String(index + 1).padStart(2, "0")}`; }
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
	  const augment = (state.perks?.augments || []).find((item) => Number(item.id) === Number(id));
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
    return `/api/image?path=${encodeURIComponent(path)}${/\/augments\/icons\//i.test(path) ? "&art=2" : ""}`;
  }
  function assetIcon(path, label, size = "", withTooltip = true, fallbackPath = "", extraClass = "") {
    const fallback = fallbackPath ? ` data-augment-fallback="${escapeHTML(proxyAsset(fallbackPath))}"` : "";
    return `<span class="game-icon${size ? ` is-${size}` : ""}${extraClass ? ` ${extraClass}` : ""}"${fallback}${withTooltip && String(label || "").trim() ? ` data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact"` : ""}><span aria-hidden="true">${escapeHTML(String(label || "?").slice(0, 1))}</span><img src="${proxyAsset(path)}" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`;
  }
  function remoteStaticIcon(source, path, label, size = "", withTooltip = true, fallbackPath = "") {
    const fallback = fallbackPath ? ` data-augment-fallback="/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(fallbackPath)}"` : "";
    return `<span class="game-icon${size ? ` is-${size}` : ""}"${fallback}${withTooltip ? ` data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact"` : ""}><span aria-hidden="true">${escapeHTML(String(label || "?").slice(0, 1))}</span><img src="/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(path)}${/\/augments\/icons\//i.test(path) ? "&art=2" : ""}" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`;
  }
  // 英雄方形头像源图自带一圈渐暗的装饰性暗角（CommunityDragon 的
  // v1/champion-icons/{id}.png；DDragon 方形图、game assets 的 _square/_circle
  // 三份实测是同一张底图，换数据源没用）。实测径向亮度曲线：r≤0.42w 亮度稳定在
  // ~102，r=0.46w 掉到 69，r=0.49w 掉到 42——暗角从 0.42w 才开始。所以圆形裁切
  // 只要落在 0.42w 以内就完全避开暗角，对应放大倍数 1/(0.42*2) ≈ 1.19，取 1.18。
  // ⚠️ 之前用过 1.4，那是拍脑袋定的（只验证了「黑边没了」没验证「脸还完整」），
  // 裁掉 29% 画面，圆形裁切连下巴一起吃掉被用户否掉；当时我把整个机制删了，
  // 但错的是数值不是机制，删完黑边就又回来了。1.18 只裁 15%，六个英雄
  // （卡蜜儿/阿狸/李青/金克丝/巨魔/阿卡丽）逐个渲染核对：暗角全消失且头像完整。
  // 改这个数字前先重跑一遍裁切对比图，别再凭感觉调。
  function iconFigure(kind, id, label, size = "", withTooltip = true) { return assetIcon(assetPath(kind, id), label || `${kind} ${id || ""}`, size, withTooltip, "", kind === "champion" ? "is-champion-art" : ""); }
  function maskedProfileIcon(size = "") { return `<span class="game-icon${size ? ` is-${size}` : ""}" data-tooltip="身份已遮罩" data-tooltip-size="compact"><span aria-hidden="true">◉</span></span>`; }
  // 目录尚未加载时的占位图标：不发起注定 404 的猜测 URL 请求（客户端
  // 的装备/技能/符文图标路径必须来自目录的 iconPath），目录加载完成后
  // rerenderCatalogViews 会自动重绘补上真实图标，避免图标时有时无。
  function pendingCatalogIcon(label, size = "", withTooltip = true) { return `<span class="game-icon is-pending${size ? ` is-${size}` : ""}"${withTooltip ? ` data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact"` : ""}><span aria-hidden="true"> </span></span>`; }
  // 装备/召唤师技能/符文图标一律使用目录里的 iconPath 与真实名称；
  // 目录未加载完成时先显示占位，加载完成后自动重绘。
  function itemIconFigure(id, size = "") {
    if (!state.items) return pendingCatalogIcon(`装备 ${id}`, size, false);
    const item = (state.items.items || []).find((candidate) => Number(candidate.id) === Number(id));
    if (!item) return pendingCatalogIcon(`装备 ${id}`, size, false);
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
    return spell?.iconPath ? assetIcon(spell.iconPath, spell.name || `召唤师技能 ${id}`, size) : iconFigure("spell", id, `召唤师技能 ${id}`, size);
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
  function augmentIconFigure(id, size = "") {
    const augment = (state.perks?.augments || []).find((candidate) => Number(candidate.id) === Number(id));
    const details = augment || { id, name: `海克斯 ${id}`, rarity: "unknown" };
    const icon = augment?.iconPath
      ? assetIcon(augment.iconPath, augment.name || `海克斯 ${id}`, size, false, augment.fallbackIconPath)
      : state.perks ? iconFigure("augment", id, details.name, size, false) : pendingCatalogIcon(details.name, size, false);
    return wrapAugmentIcon(icon, details, id);
  }
  function renderItemIcons(items, size = "") { const valid = items.filter((id) => Number(id) > 0); return valid.map((id) => itemIconFigure(id, size)).join(""); }
  function gameImageLoaded(image) {
    image.parentElement?.classList.add("has-loaded-image");
    window.deepLegendsAugmentArtwork?.prepare(image);
  }
  function gameImageFailed(image) {
    if (image.dataset.retryPending) return;
    const holder = image.parentElement;
    const fallback = holder?.dataset.augmentFallback;
    if (fallback && !image.dataset.augmentFallbackUsed) {
      image.dataset.augmentFallbackUsed = "1";
      image.hidden = false;
      image.src = fallback;
      return;
    }
    if (!image.dataset.retried) {
      image.dataset.retried = "1";
      image.dataset.retryPending = "1";
      const source = image.src;
      setTimeout(() => {
        delete image.dataset.retryPending;
        if (!image.isConnected) return;
        image.src = source;
      }, 1200);
      return;
    }
    image.hidden = true;
    holder?.classList.remove("has-loaded-image");
  }
  // load/error do not bubble, but document capture covers all locally replaced
  // cards too; no per-image listeners or repeated listener installation.
  document.addEventListener("load", event => {
    if (event.target.matches?.("[data-game-image]")) gameImageLoaded(event.target);
  }, true);
  document.addEventListener("error", event => {
    if (event.target.matches?.("[data-game-image]")) gameImageFailed(event.target);
  }, true);
  function prepareImages(container) {
    window.deepLegendsOverviewArt?.prepare(container);
    for (const image of container.querySelectorAll("[data-game-image]")) {
      if (image.complete) image.naturalWidth > 0 ? gameImageLoaded(image) : gameImageFailed(image);
    }
  }

	window.deepLegendsGameIcons = { iconFigure, prepareImages };

  // 页面 CSP 禁止 HTML 内联 style。百分比条与动态网格先以 data 属性
  // 输出，再通过 CSSOM 写入；否则浏览器会忽略 style="width:..."，所有
  // 伤害条都会退化成同样的满宽，蓝红对比也会固定成 50/50。
  function applyRenderedMetricStyles(container) {
    if (!container) return;
    for (const node of container.querySelectorAll("[data-bar-width], [data-blue-share], [data-win-rate], [data-skill-count]")) {
      const percent = value => `${Math.max(0, Math.min(100, Number(value) || 0))}%`;
      if (node.dataset.barWidth !== undefined) node.style.width = percent(node.dataset.barWidth);
      if (node.dataset.blueShare !== undefined) node.style.setProperty("--blue-share", percent(node.dataset.blueShare));
      if (node.dataset.winRate !== undefined) node.style.setProperty("--recent-win-rate", percent(node.dataset.winRate));
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
          view.render(id);
          return;
        }
        const index = matches.findIndex((match) => String(match.gameId) === id);
        const match = matches[index];
        if (!match) return;
        if (loadMatchDetails && !matchHasCompleteParticipantStats(match)) {
          detailStates.set(id, { status: "loading", message: "正在读取完整详情" });
          view.render(id);
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
          view.render(id);
          return;
        }
        tab.openMatches.add(id);
        view.render(id);
      },
      render(id = "") {
        if (!container.isConnected) { externalMatchViews.delete(container); return; }
        let scope = container;
        if (id) {
          const match = matches.find((item) => String(item.gameId) === id);
          const entry = [...container.querySelectorAll(".match-entry")].find((item) => item.querySelector("[data-toggle-match]")?.dataset.toggleMatch === id);
          if (!match || !entry) return;
          const template = document.createElement("template");
          template.innerHTML = renderMatch(match, playerRef, tab).trim();
          scope = template.content.firstElementChild;
          if (!scope) return;
          entry.replaceWith(scope);
        } else container.innerHTML = matches.map((match) => renderMatch(match, playerRef, tab)).join("");
		for (const button of scope.querySelectorAll("[data-toggle-match]:not(:disabled), [data-retry-match-detail]")) button.addEventListener("click", () => view.toggleMatch(button.dataset.toggleMatch || button.dataset.retryMatchDetail));
        bindMatchDetailControls(scope, tab, () => view.render(id));
        for (const button of scope.querySelectorAll("[data-replay]:not(:disabled)")) button.addEventListener("click", () => replay(button));
        bindPlayerLinks(scope, tab);
        applyRenderedMetricStyles(scope);
        prepareImages(scope);
      },
    };
    externalMatchViews.get(container)?.destroy?.();
    tab.externalRender = () => view.render();
    externalMatchViews.set(container, view);
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

  function showToast(message) {
    nodes.toast.textContent = message;
    nodes.toast.hidden = false;
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => { nodes.toast.hidden = true; }, 3600);
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
        if (tab) await loadOverview(tab, true);
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
  nodes.liveRefresh.addEventListener("click", () => loadLive(true));
  window.addEventListener("deep-legends:hard-refresh", (event) => {
    const detail = event.detail || {};
    const task = handleHardRefresh(detail);
    if (Array.isArray(detail.waitFor)) detail.waitFor.push(task);
  });
  window.addEventListener("deep-legends:status", (event) => updateStatus(event.detail || {}));
	window.addEventListener("deep-legends:overview-player", (event) => applyOverviewPlayerIdentity(event.detail?.summoner || state.status?.summoner));
  window.addEventListener("deep-legends:section", (event) => activateSection(event.detail?.name || "overview"));
	window.addEventListener("deep-legends:season-progress", (event) => { void handleSeasonProgress(event.detail || {}); });
	window.addEventListener("deep-legends:overview-incremental", (event) => { void handleOverviewIncremental(event.detail || {}); });

	window.addEventListener("deep-legends:live-disconnected", () => {
	  resetLiveGameScopedState();
	  state.live = null;
	  state.liveError = "";
	  if (state.section === "live") renderLive();
	});
  window.addEventListener("deep-legends:open-player", (event) => {
    const playerRef = String(event.detail?.playerRef || "").trim();
    const gameName = String(event.detail?.gameName || "").trim();
    const tagLine = String(event.detail?.tagLine || "").trim();
    const region = String(event.detail?.region || "").trim();
    const serverId = String(event.detail?.serverId || "").trim().toUpperCase();
    const source = String(event.detail?.source || "champions");
    const isPro = source === "pro-players";
    const tabContext = isPro ? { group: "pro", proTeam: event.detail?.teamCode, proPlayer: event.detail?.playerName, expectedTier: event.detail?.expectedTier } : { group: "players" };
    const targetSection = "overview";
    const label = `${gameName || "好友"}${tagLine ? `#${tagLine}` : ""}`;
    if (playerRef) {
      if (source === "search" || source === "pro-players") {
        window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: targetSection } }));
        openPlayer(playerRef, label, region, serverId, tabContext);
      } else {
        openPlayerOverlay({ playerRef, region, serverId, label });
      }
      return;
    }
    if (!gameName) return;
    if (source === "search" || source === "pro-players") {
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
      clearTimeout(state.liveTimer);
    clearTimeout(state.currentGameTimer);
      clearTimeout(state.liveEventTimer);
      state.liveEventTimer = 0;
    } else if (state.resyncPending) {
      refreshAfterResync();
    } else if (state.liveRefreshQueued) {
      queueLiveEventRefresh();
    } else if (state.section === "live") loadLive(true);
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
  function queueLiveEventRefresh() {
    if (state.destroyed || !connected()) return;
    state.liveRefreshQueued = true;
    if (document.hidden || state.liveLoading || state.liveEventTimer) return;
    state.liveEventTimer = setTimeout(() => {
      state.liveEventTimer = 0;
      if (state.destroyed || !connected() || document.hidden || state.liveLoading) return;
      state.liveRefreshQueued = false;
      void loadLive();
    }, 180);
  }
  window.addEventListener("deep-legends:pro-runes", () => {
    const target = proRequestTarget(state.live);
    const entry = target && state.proRunes.get(target.key);
    if (entry) entry.receivedAt = 0;
    if (state.section === "live") void ensureProRunes(state.live);
  });
  window.addEventListener("deep-legends:dispose", () => clearTimeout(state.proRuneTimer));
  window.addEventListener("offline", () => {
    for (const entry of state.proRunes.values()) { entry.stale = true; entry.reason = "network-unavailable"; entry.receivedAt = 0; }
    if (state.section === "live") renderLive();
  });
  window.addEventListener("online", () => { if (state.section === "live") void ensureProRunes(state.live, true); });
  window.addEventListener("deep-legends:gameflow", (event) => {
    if (state.destroyed) return;
    const phase = String(event.detail?.phase || "");
    const phaseChanged = Boolean(phase) && phase !== state.beacon.phase;
    if (phase) updateBeacon(phase);
    else scheduleBeaconPoll(0);
    if (phaseChanged && !document.hidden && connected()) {
      clearTimeout(state.liveEventTimer);
      state.liveEventTimer = 0;
      state.liveRefreshQueued = false;
      void loadLive(true);
    } else if (event.detail?.changed || phase) queueLiveEventRefresh();
  });
  // SSE 负责即时通知；1 秒轮询只兜底读取本机轻量阶段接口，不读取完整对局。
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
    if (document.hidden) { scheduleBeaconPoll(BEACON_IDLE_POLL_MS); return; }
    if (!connected()) { scheduleBeaconPoll(BEACON_DISCONNECTED_POLL_MS); return; }
    api("/api/gameplay/phase", {}, "gameflow-phase", 8000)
      .then((payload) => updateBeacon(String(payload?.phase || "")))
      .catch(() => {})
      .finally(() => scheduleBeaconPoll(beaconPollDelay()));
  }
  scheduleBeaconPoll(0);
  document.addEventListener("visibilitychange", () => { if (!document.hidden) { scheduleBeaconPoll(0); if (state.section === "overview" && activeTab()) scheduleOverviewCurrentGame(activeTab()); } });
  const playerDurationTimer = setInterval(() => {
    if (!document.hidden && state.section === "overview") for (const node of document.querySelectorAll("[data-current-game-start]")) {
      const seconds = Math.max(0, Math.floor((Date.now() - Date.parse(node.dataset.currentGameStart)) / 1000));
      if (Number.isFinite(seconds)) node.textContent = `已进行 ${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
    }
  }, 1_000);
  function refreshAfterResync() {
    if (state.destroyed || document.hidden) return;
    state.resyncPending = false;
    state.liveRefreshQueued = false;
    if (state.section === "overview") {
      const tab = activeTab(overviewGroupForSection());
      if (tab) void loadOverview(tab, true);
    }
    if (state.section === "live") void loadLive(true);
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
    state.destroyed = true;
    clearInterval(playerDurationTimer);
    clearTimeout(beaconPollTimer);
    clearTimeout(state.liveTimer);
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
  });

  bindSettings();
  renderPlayerTabs();
  renderOverview("players");
  renderLive();
  renderCapabilitySettings();
  ensurePerks();
  requestAnimationFrame(() => {
    const defaultSection = ["overview", "champions", "live", "favorites", "suite"].includes(state.settings.defaultPage) ? state.settings.defaultPage : "overview";
    if (defaultSection !== "overview") document.querySelector(`[data-section="${defaultSection}"]`)?.click();
  });
})();
