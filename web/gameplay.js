(() => {
  "use strict";

  const nodes = Object.fromEntries([
    "player-tabs", "overview-content", "overview-refresh", "live-content", "live-refresh", "live-session-summary", "toast",
    "player-overlay", "player-overlay-back", "player-overlay-title", "player-overlay-content",
    "setting-default-page", "setting-match-count", "setting-default-match-filter", "setting-live-refresh", "setting-live-interval", "setting-live-order", "setting-mask-names",
    "setting-confirm-replay", "setting-auto-accept", "setting-auto-play-again", "setting-auto-reconnect", "gameplay-settings-status",
  ].map((id) => [camel(id), document.getElementById(id)]));

  function newTabView() {
    return { matchFilter: normalizeMatchFilter(readSetting("default-match-filter", "all")), openMatches: new Set(), matchDetailTabs: new Map() };
  }

  const state = {
    status: null,
    section: "overview",
    tabs: [{ key: "current", playerRef: "", label: "当前召唤师", current: true, loading: false, data: null, error: "", ...newTabView() }],
    activeTab: "current",
    // 页签切换历史：关闭页签时返回进入它之前查看的页签。
    tabHistory: [],
    // 覆盖层栈：在非总览页点击玩家名称时，于当前页面之上展示该玩家的总览。
    overlay: [],
    controllers: new Map(),
    perks: null,
    perksLoading: false,
    items: null,
    itemsLoading: false,
    summonerSpells: null,
    summonerSpellsLoading: false,
    live: null,
    liveLoading: false,
    liveError: "",
    liveTimer: 0,
    liveRecommendations: new Map(),
    liveRecommendationFlights: new Set(),
    liveRecommendationFailures: new Map(),
    specialistRunes: new Map(),
    specialistRuneFlights: new Set(),
    specialistRuneFailures: new Map(),
    recommendationTab: "runes",
    selectedRecommendation: "opgg",
    matchObserver: null,
    overlayObserver: null,
    // 平均段位缓存按区域、页签、玩家与 gameId 隔离；null 表示查过但不可用。
    matchTiers: new Map(),
    matchTierFlights: new Set(),
    // 对局时间线缓存（装备路线 + 技能加点）：key = region:gameId:participantId。
    matchTimelines: new Map(),
    matchTimelineFlights: new Set(),
    beacon: { active: false, acked: false },
    lastCapabilities: [],
    settings: {
      defaultPage: readSetting("default-page", "overview"),
      matchCount: normalizeMatchCount(readSetting("match-count", "20")),
      defaultMatchFilter: normalizeMatchFilter(readSetting("default-match-filter", "all")),
      liveRefresh: readSetting("live-refresh", "true") !== "false",
      liveInterval: normalizeLiveInterval(readSetting("live-interval", "15")),
      liveOrder: normalizeLiveOrder(readSetting("live-order", "team")),
      maskNames: readSetting("mask-names", "false") === "true",
      // 回放默认直接在客户端内播放，不再弹确认框（可在设置中重新开启）。
      confirmReplay: readSetting("confirm-replay", "false") === "true",
    },
  };

  function camel(value) { return value.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase()); }
  function readSetting(key, fallback) { try { return localStorage.getItem(`lol-loot-${key}`) ?? fallback; } catch (_) { return fallback; } }
  function writeSetting(key, value) { try { localStorage.setItem(`lol-loot-${key}`, String(value)); } catch (_) {} }
  function normalizeMatchCount(value) { const parsed = Number(value); return [10, 20, 30, 40, 50].includes(parsed) ? parsed : 20; }
  function normalizeMatchFilter(value) { return ["all", "solo", "flex"].includes(value) ? value : "all"; }
  function normalizeLiveInterval(value) { const parsed = Number(value); return [10, 15, 30, 60].includes(parsed) ? parsed : 15; }
  function normalizeLiveOrder(value) { return ["team", "position", "kda", "win-rate"].includes(value) ? value : "team"; }
  function escapeHTML(value) { const span = document.createElement("span"); span.textContent = String(value ?? ""); return span.innerHTML; }
  function number(value) { const parsed = Number(value); return Number.isFinite(parsed) ? new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 }).format(parsed) : "—"; }
  function percent(value) { const parsed = Number(value); return Number.isFinite(parsed) ? `${Math.round(parsed)}%` : "—"; }
  function compactNumber(value) { const parsed = Number(value); if (!Number.isFinite(parsed)) return "—"; if (parsed >= 10000) return `${(parsed / 10000).toFixed(parsed >= 100000 ? 0 : 1)}万`; return number(parsed); }
  function kda(value) { const parsed = Number(value); return Number.isFinite(parsed) ? parsed.toFixed(2) : "—"; }

  const CN_SERVER_LABELS = Object.freeze({
    HN1: "艾欧尼亚", HN10: "黑色玫瑰", NJ100: "联盟一区", GZ100: "联盟二区", CQ100: "联盟三区",
    TJ100: "联盟四区", TJ101: "联盟五区", BGP2: "峡谷之巅", PBE: "体验服",
  });

  async function api(path, options = {}, key = path, timeout = 30000) {
    state.controllers.get(key)?.abort();
    const controller = new AbortController();
    state.controllers.set(key, controller);
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, timeout);
    try {
      const headers = new Headers(options.headers || {});
      headers.set("Accept", "application/json");
      if (options.body) headers.set("Content-Type", "application/json");
      const response = await fetch(path, { ...options, headers, signal: controller.signal });
      if (response.status === 401) throw new Error("页面会话已过期，刷新页面即可重新连接");
      if (!response.ok) throw new Error((await response.text()).trim() || `本地服务返回 HTTP ${response.status}`);
      return response.status === 204 ? null : response.json();
    } catch (error) {
      if (error.name === "AbortError") {
        if (timedOut) throw new Error("客户端数据读取超时，请重试");
        const aborted = new Error("请求已取消");
        aborted.name = "RequestCancelled";
        throw aborted;
      }
      throw error;
    } finally {
      clearTimeout(timer);
      if (state.controllers.get(key) === controller) state.controllers.delete(key);
    }
  }

  function connected() { return Boolean(state.status?.connected); }
  function activeTab() { return state.tabs.find((tab) => tab.key === state.activeTab) || state.tabs[0]; }
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

  function matchTierScope(tab) {
    const region = riotTab(tab) ? "kr" : `cn:${tabServerID(tab) || "current"}`;
    const playerRef = tab?.data?.player?.playerRef || tab?.playerRef || `${tab?.riotId?.gameName || ""}#${tab?.riotId?.tagLine || ""}` || "unknown";
    return `${region}:${tab?.key || "unknown"}:${playerRef}`;
  }

  function matchTierCacheKey(tab, gameID) {
    return `${matchTierScope(tab)}:${String(gameID || "")}`;
  }

  function activateSection(name) {
    const previous = state.section;
    state.section = name;
    clearTimeout(state.liveTimer);
    // 切换主页面时关闭覆盖层、收起已展开的战绩详情，并把游戏类型筛选还原为默认。
    if (previous !== name) {
      closeOverlay();
      for (const tab of state.tabs) {
        tab.openMatches?.clear();
        tab.matchDetailTabs?.clear();
        tab.matchFilter = state.settings.defaultMatchFilter;
      }
    }
    if (name === "live") { state.beacon.acked = true; renderBeacon(); }
    if (name === "overview") {
      const tab = activeTab();
      if (tabReady(tab) && !tab.data && !tab.loading) loadOverview(tab);
      else renderOverview();
    }
    if (name === "live") {
      if (connected()) loadLive();
      else renderLive();
    }
    renderBeacon();
  }

  function updateStatus(status) {
    const wasConnected = connected();
    state.status = status;
    if (!status.connected) {
      // 客户端断开只清空依赖客户端的数据；韩服页签的数据来自 Riot API，保留。
      for (const tab of state.tabs) { if (!riotTab(tab)) { tab.data = null; tab.error = ""; tab.loading = false; } }
      state.overlay = state.overlay.filter((entry) => riotTab(entry));
      state.live = null;
      state.liveError = "";
      // 图标目录改用 Data Dragon 兜底重新加载。
      state.perks = null;
      state.items = null;
      state.summonerSpells = null;
      updateBeacon("None");
      renderPlayerTabs();
      renderOverview();
      renderLive();
      renderOverlay();
      return;
    }
    if (!wasConnected && status.connected) {
      const current = state.tabs[0];
      current.label = summonerLabel(status.summoner || {});
      // 重新连接后改用客户端目录（名称与图标以客户端为准）。
      state.perks = null;
      state.items = null;
      state.summonerSpells = null;
      renderPlayerTabs();
      if (state.section === "overview") loadOverview(current);
      if (state.section === "live") loadLive();
    }
  }

  function rerenderTab(tab) {
    if (tab.overlay) { renderOverlay(); return; }
    renderPlayerTabs();
    if (tab.key === state.activeTab) renderOverview();
  }

  async function loadOverview(tab, force = false, append = false) {
    if (!tabReady(tab)) { rerenderTab(tab); return; }
    if (append) {
      if (tab.loading || tab.loadingMore || !tab.data?.pagination?.hasMore) return;
      tab.loadingMore = true;
    } else {
      if (tab.loading) return;
      if (tab.data && !force) { rerenderTab(tab); return; }
      tab.loading = true;
      // 强制刷新会取消同页签正在进行的“加载更多”。
      tab.loadingMore = false;
    }
    const requestToken = Number(tab.overviewRequestToken || 0) + 1;
    tab.overviewRequestToken = requestToken;
    tab.error = "";
    rerenderTab(tab);
    try {
      const begIndex = append ? Number(tab.data?.pagination?.begIndex || 0) + Number(tab.data?.pagination?.count || (tab.data?.matches || []).length) : 0;
      const body = tab.current ? null : JSON.stringify(tab.riotId && !tab.playerRef
        ? { gameName: tab.riotId.gameName, tagLine: tab.riotId.tagLine, region: tab.region || "", serverId: tab.serverId || "", count: state.settings.matchCount, begIndex }
        : { playerRef: tab.playerRef, serverId: tab.serverId || "", count: state.settings.matchCount, begIndex });
      const payload = tab.current
        ? await api(`/api/gameplay/overview?count=${state.settings.matchCount}&begIndex=${begIndex}`, {}, `overview:${tab.key}`)
        : await api("/api/gameplay/overview", { method: "POST", body }, `overview:${tab.key}`);
      if (append) {
        const seen = new Set((tab.data.matches || []).map((match) => String(match.gameId)));
        const additions = (payload.matches || []).filter((match) => {
          const gameID = String(match.gameId);
          if (seen.has(gameID)) return false;
          seen.add(gameID);
          return true;
        });
        tab.data = { ...tab.data, matches: [...(tab.data.matches || []), ...additions], pagination: payload.pagination || { begIndex, count: additions.length, hasMore: additions.length === state.settings.matchCount } };
      } else {
        tab.data = payload;
      }
      tab.playerRef = payload.player?.playerRef || tab.playerRef;
      tab.region = payload.player?.region || tab.region || "";
      tab.serverId = payload.player?.serverId || tab.serverId || "";
      tab.serverName = payload.player?.serverName || tab.serverName || "";
      tab.label = playerLabel(payload.player || {});
      tab.icon = payload.player?.profileIconId || 0;
      tab.error = "";
      if (!append) {
        state.lastCapabilities = payload.capabilities || [];
        renderCapabilitySettings();
      }
    } catch (error) {
      if (error.name !== "RequestCancelled") {
        // 玩家引用极少数情况下会失效（例如切换登录账号）；搜索打开的页签
        // 还留有 Riot ID，直接改用 Riot ID 重新查询一次。
        if (!append && tab.playerRef && tab.riotId && /引用/.test(error.message)) {
          tab.playerRef = "";
          tab.loading = false;
          tab.loadingMore = false;
          loadOverview(tab, true);
          return;
        }
        if (append) showToast(`更多战绩加载失败：${error.message}`);
        else tab.error = error.message;
      }
    } finally {
      // 被新刷新取代的旧请求不能清除新请求的 loading 状态。
      if (tab.overviewRequestToken === requestToken) {
        tab.loading = false;
        tab.loadingMore = false;
        rerenderTab(tab);
      }
    }
  }

  function renderPlayerTabs() {
    nodes.playerTabs.innerHTML = state.tabs.map((tab, index) => {
      const active = tab.key === state.activeTab;
      const masked = state.settings.maskNames && !tab.current;
      const label = masked ? (tab.data?.player?.hidden ? "隐藏玩家" : `玩家 ${String(index).padStart(2, "0")}`) : tab.label;
      const icon = !masked && tab.icon ? iconFigure("profile", tab.icon, "") : '<span class="player-tab-placeholder" aria-hidden="true">◉</span>';
      const serverLabel = tabServerLabel(tab);
      const regionBadge = riotTab(tab) || tabServerID(tab) ? `<span class="player-tab-region${riotTab(tab) ? " is-kr" : ""}" title="${escapeHTML(tabServerTitle(tab))}">${escapeHTML(serverLabel)}</span>` : "";
      const hiddenBadge = tab.data?.player?.hidden || tab.data?.player?.privateHistory ? '<small class="player-tab-hidden">隐藏战绩</small>' : "";
      const selfBadge = tab.current ? '<span class="player-tab-self" title="当前登录的召唤师" aria-hidden="true">★</span>' : "";
      return `<span class="player-tab-wrap${active ? " is-active" : ""}${tab.current ? " is-self" : ""}"><button class="player-tab" type="button" role="tab" aria-selected="${active}" tabindex="${active ? 0 : -1}" data-player-tab="${escapeHTML(tab.key)}">${selfBadge}${icon}<span class="player-tab-copy"><span class="player-tab-name" title="${escapeHTML(label)}">${escapeHTML(label)}</span>${hiddenBadge}</span>${regionBadge}${tab.loading ? '<span class="mini-loading" aria-label="正在读取"></span>' : ""}</button>${tab.current ? "" : `<button class="player-tab-close" type="button" aria-label="关闭 ${escapeHTML(label)}" data-close-player="${escapeHTML(tab.key)}">×</button>`}</span>`;
    }).join("");
    prepareImages(nodes.playerTabs);
  }

  function selectPlayerTab(key) {
    const tab = state.tabs.find((item) => item.key === key);
    if (!tab) return;
    // 记录页签切换历史：关闭页签时返回上一个查看的页签。
    if (state.activeTab !== key) {
      state.tabHistory = (state.tabHistory || []).filter((item) => item !== state.activeTab);
      state.tabHistory.push(state.activeTab);
      if (state.tabHistory.length > 20) state.tabHistory.shift();
    }
    state.activeTab = key;
    // 每个页签的筛选、展开状态与详情页签独立保留：来回切换玩家页签
    // 不还原（离开总览页再回来时才统一还原，见 activateSection）。
    renderPlayerTabs();
    if (!tab.data && !tab.loading) loadOverview(tab);
    else renderOverview();
  }

  // 两个服务器的玩家互不相通：新页签/覆盖层继承来源页面的服务器标签，
  // 韩服总览里点到的玩家一定按韩服查询，国服页面同理。
  function rememberActiveTab() {
    state.tabHistory = (state.tabHistory || []).filter((item) => item !== state.activeTab);
    state.tabHistory.push(state.activeTab);
    if (state.tabHistory.length > 20) state.tabHistory.shift();
  }

  function openPlayer(playerRef, label, region, serverId = "") {
    if (!playerRef) return;
    const existing = state.tabs.find((tab) => tab.playerRef === playerRef && (tab.serverId || "") === (serverId || ""));
    if (existing) { selectPlayerTab(existing.key); return; }
    const tab = { key: `player-${state.tabs.length}-${Date.now()}`, playerRef, region: region || "", serverId: serverId || "", label: label || "隐藏玩家", current: false, loading: false, data: null, error: "", ...newTabView() };
    rememberActiveTab();
    state.tabs.push(tab);
    state.activeTab = tab.key;
    renderPlayerTabs();
    renderOverview();
    loadOverview(tab);
  }

  // 按 Riot ID 打开玩家（顶部搜索）。
  function openPlayerByRiotId(gameName, tagLine, region, serverId = "") {
    const label = `${gameName}${tagLine ? `#${tagLine}` : ""}`;
    const existing = state.tabs.find((tab) => tab.riotId && tab.riotId.gameName === gameName && tab.riotId.tagLine === tagLine && (tab.region || "") === (region || "") && (tab.serverId || "") === (serverId || ""));
    if (existing) { selectPlayerTab(existing.key); return; }
    const tab = { key: `player-${state.tabs.length}-${Date.now()}`, playerRef: "", riotId: { gameName, tagLine }, region: region || "", serverId: serverId || "", label, current: false, loading: false, data: null, error: "", ...newTabView() };
    rememberActiveTab();
    state.tabs.push(tab);
    state.activeTab = tab.key;
    renderPlayerTabs();
    renderOverview();
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
      nodes.playerOverlayContent.innerHTML = "";
      return;
    }
    nodes.playerOverlay.hidden = false;
    const regionChip = `<span class="region-chip${riotTab(entry) ? " is-kr" : ""}" title="${escapeHTML(tabServerTitle(entry))}">${escapeHTML(tabServerLabel(entry))}</span>`;
    const hiddenBadge = entry.data?.player?.hidden || entry.data?.player?.privateHistory ? '<span class="player-tab-hidden">隐藏战绩</span>' : "";
    nodes.playerOverlayTitle.innerHTML = `<strong title="${escapeHTML(entry.label)}">${escapeHTML(entry.label)}</strong>${regionChip}${hiddenBadge}${state.overlay.length > 1 ? `<small>第 ${state.overlay.length} 层</small>` : ""}`;
    renderOverviewBody(nodes.playerOverlayContent, entry);
    const scroller = nodes.playerOverlay.querySelector(".player-overlay-scroll");
    if (scroller) scroller.scrollTop = 0;
  }

  function opggSummonerURL(tab) {
    const region = tab.region || "kr";
    const slug = `${tab.riotId.gameName}-${tab.riotId.tagLine || ""}`.replace(/-$/, "");
    return `https://op.gg/zh-cn/lol/summoners/${encodeURIComponent(region)}/${encodeURIComponent(slug)}`;
  }

  function closePlayerTab(key) {
    const index = state.tabs.findIndex((tab) => tab.key === key && !tab.current);
    if (index < 0) return;
    state.controllers.get(`overview:${key}`)?.abort();
    state.tabs.splice(index, 1);
    state.tabHistory = (state.tabHistory || []).filter((item) => item !== key);
    if (state.activeTab === key) {
      // 返回进入这个页签之前查看的页签；历史里已关闭的项跳过。
      let previous = "";
      while (state.tabHistory.length && !previous) {
        const candidate = state.tabHistory.pop();
        if (state.tabs.some((tab) => tab.key === candidate)) previous = candidate;
      }
      state.activeTab = previous || "current";
    }
    renderPlayerTabs();
    renderOverview();
  }

  function renderOverview() {
    state.matchObserver?.disconnect();
    state.matchObserver = null;
    const tab = activeTab();
    // 通知外层：启动入口卡只在“当前召唤师”页签展示。
    window.dispatchEvent(new CustomEvent("deep-legends:overview-tab", { detail: { current: Boolean(tab?.current) } }));
    // 只要存在韩服页签，未连接客户端时总览页仍保持可用。
    const panelUsable = connected() || state.tabs.some((item) => riotTab(item));
    nodes.overviewContent.closest(".gameplay-panel")?.classList.toggle("is-disconnected", !panelUsable);
    if (!panelUsable) {
      nodes.overviewContent.innerHTML = "";
      return;
    }
    if (!tabReady(tab)) {
      nodes.overviewContent.innerHTML = emptyState("等待英雄联盟客户端", "国服召唤师数据需要本机客户端连接后读取；已打开的韩服页签不受影响。", false);
      return;
    }
    renderOverviewBody(nodes.overviewContent, tab);
  }

  // renderOverviewBody 把某个玩家页签（总览页签或覆盖层条目）的总览
  // 渲染到指定容器；总览页与覆盖层共用同一套展示与交互。
  function renderOverviewBody(container, tab) {
    const tierScope = matchTierScope(tab);
    // 主总览和覆盖层容器都会被后续页签复用。异步结果只允许写回
    // 当前仍属于同一页签和玩家的容器。
    container.dataset.matchTierScope = tierScope;
    if (tab.loading || !tab.data && !tab.error) {
      container.innerHTML = '<div class="gameplay-skeleton"><span></span><span></span><span></span><span></span></div>';
      return;
    }
    if (tab.error) {
      const opggEscape = tab.riotId && riotTab(tab) ? '<div class="opgg-escape"><button class="text-button" type="button" data-open-opgg>在 OP.GG 中查看该玩家 ↗</button></div>' : "";
      container.innerHTML = emptyState("战绩读取失败", tab.error, true) + opggEscape;
      container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadOverview(tab, true));
      container.querySelector("[data-open-opgg]")?.addEventListener("click", () => window.open(opggSummonerURL(tab), "_blank", "noopener"));
      return;
    }
    const data = tab.data;
    const player = data.player || {};
    const matches = filteredMatches(data.matches || [], tab);
    const playerIndex = Math.max(1, state.tabs.findIndex((item) => item.key === tab.key));
    const maskProfile = state.settings.maskNames && !player.isCurrent;
    const profileName = maskProfile ? (player.hidden ? "隐藏玩家" : `玩家 ${String(playerIndex).padStart(2, "0")}`) : (player.gameName || player.displayName || "隐藏玩家");
    const profileIcon = maskProfile ? maskedProfileIcon("summoner-avatar") : iconFigure("profile", player.profileIconId, playerLabel(player), "summoner-avatar");
    const historyCapability = (data.capabilities || []).find((item) => item.name === "match-history");
    const pagination = data.pagination || { hasMore: false };
    const emptyMatches = historyCapability && historyCapability.state !== "available"
      ? emptyState("战绩暂时无法读取", historyCapability.detail || "客户端暂未返回该玩家的战绩，请稍后重试。", true)
      : emptyState("没有符合条件的对局", "可以切换上方游戏类型，或刷新读取最新战绩。", false);
    const regionChip = `<span class="region-chip${riotTab(tab) ? " is-kr" : ""}" title="${escapeHTML(tabServerTitle(tab))}">${escapeHTML(tabServerLabel(tab))}</span>`;
    const hiddenChip = player.hidden || player.privateHistory ? '<span class="player-tab-hidden" title="该玩家在客户端里开启了隐藏战绩">隐藏战绩</span>' : "";
    container.innerHTML = `
      <section class="summoner-strip">
        ${profileIcon}
        <div class="summoner-strip-copy"><div><h2>${escapeHTML(profileName)}</h2>${!maskProfile && player.tagLine ? `<span>#${escapeHTML(player.tagLine)}</span>` : ""}${regionChip}${hiddenChip}</div><p>召唤师等级 ${number(player.summonerLevel)}${player.hidden ? " · 身份已隐藏，战绩正常展示" : ""}</p></div>
      </section>
      <div class="overview-layout">
        <aside class="career-column" aria-label="生涯统计">
          ${renderRanks(data.ranks || [], data.capabilities || [])}
          ${renderChampionStats(data.championStats || [], data.overall || {})}
          ${renderPositionStats(data.positions || [])}
          ${renderMasteries(data.masteries || [])}
          ${renderSevenDay(data.sevenDayRank || {})}
          ${renderRecentPlayers(data.recentPlayers || [])}
          ${renderActivity(data.activityHours || [])}
        </aside>
        <section class="matches-column" aria-label="最近对局">
          ${renderMatchFilters(data.matches || [], tab)}
          <div class="match-list">${matches.length ? matches.map((match) => renderMatch(match, player.playerRef, tab)).join("") : emptyMatches}</div>
          ${(data.matches || []).length ? `<div class="match-pagination" data-match-sentinel aria-live="polite">${tab.loadingMore ? '<span class="mini-loading" aria-hidden="true"></span><span>正在加载下一批战绩…</span>' : pagination.hasMore ? '<span>继续向下滚动，自动加载更多</span><button class="text-button" type="button" data-load-more>加载更多</button>' : `<span>已展示全部 ${number((data.matches || []).length)} 场</span>`}</div>` : ""}
        </section>
      </div>`;
    bindOverviewContent(container, tab);
    prepareImages(container);
    if ((data.matches || []).length) { ensurePerks(); ensureItems(); ensureSummonerSpells(); hydrateMatchTiers(container, tab, tierScope); }
  }

  function renderRanks(ranks, capabilities) {
    const rankedCapability = capabilities.find((item) => item.name === "ranked-stats");
    if (rankedCapability?.state === "unsupported") {
      return `<section class="career-section"><header><h3>排位</h3><span>能力边界</span></header><div class="rank-unavailable"><strong>${escapeHTML(rankedCapability.detail || "跨服暂不支持排位")}</strong><small>当前客户端的排位接口只能读取登录服务器。</small></div></section>`;
    }
    const expected = ["RANKED_SOLO_5x5", "RANKED_FLEX_SR"];
    const rows = expected.map((queue) => {
      const rank = ranks.find((item) => item.queueType === queue);
      const label = queue === "RANKED_SOLO_5x5" ? "单排/双排" : "灵活组排";
      if (!rank || !rank.tier) return `<div class="rank-row"><span class="rank-crest is-unranked" aria-hidden="true">◇</span><div><strong>${label}</strong><small>尚未定级或客户端未提供</small></div><b>Unranked</b></div>`;
      return `<div class="rank-row"><span class="rank-crest" aria-hidden="true">${rankCrestIcon(rank.tier)}</span><div><strong>${label}</strong><small>${escapeHTML(rankTitle(rank))} · ${number(rank.leaguePoints)} LP</small></div><div class="rank-record"><b>${number(rank.wins)}胜 ${number(rank.losses)}负</b><span>胜率 ${percent(rank.winRate)}</span></div></div>`;
    }).join("");
    return `<section class="career-section"><header><h3>排位</h3><span>当前赛季</span></header><div class="rank-list">${rows}</div></section>`;
  }

  function renderChampionStats(items, overall) {
    const overallRow = `<div class="champion-stat-row is-overall"><span class="overall-champion-mark" aria-hidden="true">全</span><div><strong>全部英雄</strong><small>CS ${number(overall.cs)} (${number(overall.csPerMinute)})</small></div><div><b>${kda(overall.kda)}:1 KDA</b><small>${number(overall.kills)} / ${number(overall.deaths)} / ${number(overall.assists)}</small></div><div><b>${percent(overall.winRate)}</b><small>${number(overall.games)} 场</small></div></div>`;
    const rows = items.slice(0, 8).map((item) => `<div class="champion-stat-row">${iconFigure("champion", item.championId, item.championName)}<div><strong>${escapeHTML(item.championName)}</strong><small>CS ${number(item.cs)} (${number(item.csPerMinute)})</small></div><div><b>${kda(item.kda)}:1 KDA</b><small>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}</small></div><div><b>${percent(item.winRate)}</b><small>${number(item.games)} 场</small></div></div>`).join("");
    return `<section class="career-section champion-performance"><header><h3>英雄胜率</h3><span>最近 ${number(overall.games)} 场</span></header><div>${overallRow}${rows || '<p class="section-empty">暂无英雄统计</p>'}</div></section>`;
  }

  function renderPositionStats(items) {
    const rows = items.map((item) => `<div class="position-row"><span>${positionIcon(item.position)}</span><div><strong>${escapeHTML(item.label)}</strong><progress max="100" value="${Math.max(0, Math.min(100, Number(item.share) || 0))}">${percent(item.share)}</progress></div><b>${percent(item.share)}</b></div>`).join("");
    return `<section class="career-section"><header><h3>位置偏好</h3><span>按最近对局</span></header><div class="position-list">${rows}</div></section>`;
  }

  function renderMasteries(items) {
    const rows = items.map((item) => `<div class="mastery-row">${iconFigure("champion", item.championId, item.championName)}<div><strong>${escapeHTML(item.championName)}</strong><small>${compactNumber(item.championPoints)} 熟练度</small></div><b>Lv.${number(item.championLevel)}</b></div>`).join("");
    return `<section class="career-section"><header><h3>英雄熟练度</h3><span>最高分</span></header><div class="mastery-list">${rows || '<p class="section-empty">客户端未提供熟练度</p>'}</div></section>`;
  }

  function renderSevenDay(stats) {
    return `<section class="career-section seven-day"><header><h3>过去 30 天排位</h3><span>${stats.sampled ? "最近 100 场样本" : "单排与灵活"}</span></header><div class="seven-day-content"><strong>${stats.games ? percent(stats.winRate) : "—"}</strong><div><b>${number(stats.wins)} 胜 ${number(stats.losses)} 负</b><small>${stats.games ? `${kda(stats.kda)}:1 KDA · ${number(stats.games)} 场${stats.sampled ? "以上" : ""}` : "暂无排位记录"}</small></div></div></section>`;
  }

  function renderRecentPlayers(items) {
    const rows = items.map((item, index) => {
      const label = maskedListName(item, index);
      const icon = state.settings.maskNames ? maskedProfileIcon() : iconFigure("profile", item.profileIconId, "");
      return `<button class="recent-player" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"} title="${escapeHTML(label)}">${icon}<span><strong>${escapeHTML(label)}</strong><small>共同对局 ${number(item.games)} 场</small></span><span aria-hidden="true">›</span></button>`;
    }).join("");
    return `<section class="career-section"><header><h3>最近一起玩</h3><span>最近 30 天</span></header><div class="recent-player-list">${rows || '<p class="section-empty">暂无重复同场玩家</p>'}</div></section>`;
  }

  function renderActivity(hours) {
    const max = Math.max(1, ...hours.map(Number));
    const cells = Array.from({ length: 24 }, (_, hour) => {
      const value = Number(hours[hour] || 0);
      const intensity = value ? Math.max(1, Math.ceil(value / max * 4)) : 0;
      return `<span class="activity-cell level-${intensity}" title="${hour}:00 · ${value} 场"><b>${hour}</b></span>`;
    }).join("");
    return `<section class="career-section"><header><h3>游戏时间分布</h3><span>本地时间</span></header><div class="activity-legend"><span>上午</span><span>下午</span></div><div class="activity-grid">${cells}</div></section>`;
  }

  // “更多模式”固定清单（对照 OP.GG 的模式下拉）：每项对应一组 queueId；
  // “特殊模式”兜底承接其余所有未归类队列。
  const MORE_MODE_OPTIONS = [
    ["match", "匹配模式", [400, 430, 490]],
    ["hextech-classic", "海克斯大乱斗 经典模式版", [2400]],
    ["bots", "人机对战", [820, 830, 840, 850, 860, 870, 880, 890]],
    ["urf", "无限火力", [900, 1900]],
    ["clash", "冠军杯赛", [700, 720]],
    ["nexus-blitz", "极限闪击", [1300]],
    ["doombots", "末日人工智能", [950, 960]],
    ["special", "特殊模式", null],
  ];

  function renderMatchFilters(matches, tab) {
    const direct = [["all", "全部"], ["solo", "单排/双排"], ["flex", "灵活组排"], ["hextech-aram", "海克斯大乱斗"], ["arena", "斗魂竞技场"], ["aram", "极地大乱斗"]];
    const moreActive = String(tab.matchFilter).startsWith("more:");
    return `<div class="match-filterbar"><div class="match-filter-tabs" role="group" aria-label="游戏类型">${direct.map(([value, label]) => `<button class="match-filter${tab.matchFilter === value ? " is-active" : ""}" type="button" aria-pressed="${tab.matchFilter === value}" data-match-filter="${value}">${label}</button>`).join("")}</div><label class="select-wrap match-more-filter${moreActive ? " is-active" : ""}"><span class="sr-only">更多游戏类型</span><select data-match-more><option value="">更多模式</option>${MORE_MODE_OPTIONS.map(([key, label]) => `<option value="more:${key}"${tab.matchFilter === `more:${key}` ? " selected" : ""}>${escapeHTML(label)}</option>`).join("")}</select></label></div>`;
  }

  function filteredMatches(matches, tab) {
    const filter = String(tab.matchFilter || "all");
    if (filter === "all") return matches;
    if (filter.startsWith("more:")) {
      const key = filter.slice(5);
      const option = MORE_MODE_OPTIONS.find(([value]) => value === key);
      if (!option) return matches;
      if (option[2]) return matches.filter((match) => option[2].includes(Number(match.queueId)));
      // 特殊模式：其余全部未归类队列。
      const known = new Set(MORE_MODE_OPTIONS.flatMap(([, , ids]) => ids || []));
      return matches.filter((match) => match.modeGroup === "other" && !known.has(Number(match.queueId)));
    }
    // 兼容旧版 queue:ID 设置值。
    if (filter.startsWith("queue:")) return matches.filter((match) => String(match.queueId) === filter.slice(6));
    return matches.filter((match) => match.modeGroup === filter);
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
    const rows = participants.map((item) => ({
      item,
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
      scores.set(Number(row.item.participantId), { score: Math.round((2 + raw * 8) * 10) / 10, badge: "" });
    }
    // MVP 给胜方最高分，SVP 给败方最高分；结果不明的对局不发徽章。
    const hasWin = rows.some((row) => row.item.win);
    const hasLoss = rows.some((row) => !row.item.win);
    if (hasWin && hasLoss) {
      for (const winSide of [true, false]) {
        let best = null;
        for (const row of rows) {
          if (Boolean(row.item.win) !== winSide) continue;
          if (!best || scores.get(Number(row.item.participantId)).score > scores.get(best).score) best = Number(row.item.participantId);
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
    const tone = record.score >= 8 ? " is-gold" : record.score >= 6.5 ? " is-good" : record.score < 4 ? " is-poor" : "";
    return `<i class="match-score${tone}" title="单场相对评分（KDA、参团、伤害、经济、补刀、视野相对全场最高值加权）">${record.score.toFixed(1)}</i>`;
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
    if (rank === 1) return "1st";
    if (rank === 2) return "2nd";
    if (rank === 3) return "3rd";
    return `${rank}th`;
  }

  // 竞技场客户端使用固定的中立生物队名。playerSubteamId 与客户端队伍
  // 槽位一一对应；名称和徽标均取自 lol-game-data，而不是展示英文枚举。
  const ARENA_TEAM_MASCOTS = [
    null,
    { name: "魄罗", file: "teamporos.png" },
    { name: "小兵", file: "teamminions.png" },
    { name: "河道蟹", file: "teamscuttles.png" },
    { name: "石甲虫", file: "teamkrugs.png" },
    { name: "锋喙鸟", file: "teamraptors.png" },
    { name: "暗影狼", file: "teamwolves.png" },
    { name: "魔沼蛙", file: "teamgromp.png" },
    { name: "哨兵", file: "teamsentinel.png" },
  ];

  function arenaTeamMeta(group) {
    const id = Number(group?.subteamId) || 0;
    const mascot = ARENA_TEAM_MASCOTS[id] || null;
    return mascot
      ? { ...mascot, iconPath: `/lol-game-data/assets/UX/Cherry/TeamIcons/${mascot.file}` }
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
      return { arena: true, groups: [...groups.values()].sort((left, right) => (left.placement || 99) - (right.placement || 99) || left.subteamId - right.subteamId) };
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
  // 斗魂竞技场按名次显示前四队，每队三名玩家一行；其余队伍在展开详情中展示。
  function renderMatchPlayers(match) {
    const grouping = matchPlayerGroups(match);
    const playerButton = (item, index, tiny) => `<button type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"} title="${escapeHTML(playerParticipantName(item, index))}">${iconFigure("champion", item.championId, item.championName, "tiny")}${tiny ? "" : `<span>${escapeHTML(playerParticipantName(item, index))}</span>`}</button>`;
    if (grouping.arena) {
      const rows = grouping.groups.slice(0, 4).map((group) => {
        const team = arenaTeamMeta(group);
        const emblem = team.iconPath ? assetIcon(team.iconPath, team.name, "tiny") : "";
        return `<div class="arena-team-row-compact${group.placement === 1 ? " is-first" : ""}" title="${escapeHTML(arenaGroupLabel(group))}"><b>#${number(group.placement || "—")}</b><span class="arena-team-name">${emblem}<span>${escapeHTML(team.name)}</span></span><span class="arena-team-roster">${group.players.map((item, index) => playerButton(item, index, true)).join("")}</span></div>`;
      }).join("");
      return `<div class="match-players is-arena">${rows}</div>`;
    }
    const lists = grouping.groups.map((group) => `<div class="match-team-list">${group.players.map((item, index) => playerButton(item, index, false)).join("")}</div>`).join("");
    return `<div class="match-players">${lists}</div>`;
  }

  // 本场评分排名 chip：前三名有专属配色（金 / 银 / 铜）。
  function scoreRankChip(rank, total) {
    if (!rank) return "";
    const tone = rank <= 3 ? ` is-rank-${rank}` : "";
    return `<b class="match-rank-chip${tone}" title="本场评分排名：全场 ${number(total)} 人中的第 ${number(rank)} 名">${ordinalLabel(rank)}</b>`;
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

  function renderMatch(match, playerRef, tab) {
    const subject = matchSubject(match, playerRef);
    if (!subject) return "";
    const win = match.result === "win";
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
    // 全场评分排名（并列时取最好名次）。
    let scoreRank = 0;
    if (subjectScore) scoreRank = 1 + [...scores.values()].filter((record) => record.score > subjectScore.score).length;
    const badges = [];
    if (subject.multiKill >= 5) badges.push("五杀"); else if (subject.multiKill === 4) badges.push("四杀"); else if (subject.multiKill === 3) badges.push("三杀"); else if (subject.multiKill === 2) badges.push("双杀");
    if (subject.deaths === 0) badges.push("零阵亡");
    const open = tab.openMatches.has(String(match.gameId));
    // 左侧：模式 + 时间（未知时整行不展示）+ 结果 + 胜点变化（仅两种排位且有记录时）+ 时长。
    const timeLabel = relativeTime(match.createdAt);
    const timeRow = timeLabel ? `<time title="${escapeHTML(exactTime(match.createdAt))}">${escapeHTML(timeLabel)}</time>` : "";
    const ranked = Number(match.queueId) === 420 || Number(match.queueId) === 440;
    const lpValue = Number(match.lpDelta);
    const lpChip = ranked && Number.isFinite(lpValue) && match.lpDelta != null
      ? `<b class="lp-delta ${lpValue >= 0 ? "is-gain" : "is-drop"}" title="这场排位的胜点变化（由助手在对局结算时记录）">${lpValue >= 0 ? "+" : "−"}${Math.abs(lpValue)} LP</b>`
      : "";
    // 斗魂竞技场展示小队名次代替胜负字样。
    const arenaPlacement = Number(subject.placement) > 0 && (match.modeGroup === "arena" || Number(subject.subteamId) > 0) ? Number(subject.placement) : 0;
    const resultWord = arenaPlacement ? `第 ${arenaPlacement} 名` : win ? "胜利" : match.result === "loss" ? "失败" : "结果未知";
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
    const standardStatRows = `
      <span class="match-stat" title="${escapeHTML(participationTitle)}"><em>击杀参与率</em><b>${participation === null ? "—" : percent(participation)}</b></span>
      <span class="match-stat" title="${escapeHTML(csTitle)}"><em>CS</em><b>${number(subject.cs)} (${number(subject.csPerMinute)})</b></span>
      <span class="match-stat match-stat-tier" data-match-tier data-game-id="${match.gameId}"${tierPending} title="${escapeHTML(matchTierTitle(tierValue))}"><em>平均段位</em><span class="match-tier-value">${tierValue === undefined ? '<b class="match-tier-unknown">…</b>' : matchTierContent(tierValue)}</span></span>`;
    const arenaStatRows = `
      <span class="match-stat is-arena-place" title="这支小队在本场斗魂竞技场的最终名次"><em>本场名次</em><b>${arenaPlacement ? `第 ${arenaPlacement} 名` : "—"}</b></span>
      <span class="match-stat" title="对英雄造成的总伤害"><em>英雄伤害</em><b>${compactNumber(subject.damage)}</b></span>
      <span class="match-stat" title="本场承受的总伤害"><em>承受伤害</em><b>${compactNumber(subject.damageTaken)}</b></span>`;
    const statRows = arenaPlacement ? arenaStatRows : standardStatRows;
    const loadout = arenaPlacement
      ? (subject.augmentIds || []).map((id) => augmentIconFigure(id, "small")).join("")
      : `${spellIconFigure(subject.spell1Id, "small")}${spellIconFigure(subject.spell2Id, "small")}${subject.perkIds?.[0] ? perkIconFigure(subject.perkIds[0], "small") : ""}${subject.subStyleId ? perkStyleIconFigure(subject.subStyleId, "small") : ""}`;
    // 评分排名放在成就徽章前；MVP 已代表全场第一，避免与 1st 重复展示。
    const rankChip = subjectScore?.badge === "MVP" && scoreRank === 1 ? "" : scoreRankChip(scoreRank, scores.size);
    return `<article class="match-entry is-${win ? "win" : match.result === "loss" ? "loss" : "unknown"}" data-match-id="${match.gameId}">
      <div class="match-summary">
        <div class="match-result-meta"><strong>${escapeHTML(match.queueLabel)}</strong>${timeRow}<span class="result-word">${resultWord}${lpChip}</span><small>${formatDuration(match.duration)}</small></div>
        <div class="match-main">
          <div class="match-champion">${iconFigure("champion", subject.championId, subject.championName, "large")}<span class="champion-level">${number(subject.championLevel)}</span><div class="match-loadout-mini${arenaPlacement ? " is-augments" : ""}">${loadout}</div></div>
          <div class="match-kda"><strong><span>${number(subject.kills)}</span> / <em>${number(subject.deaths)}</em> / <span>${number(subject.assists)}</span></strong><small>${kda(subject.kda)}:1 KDA</small>${subjectScore ? `<span class="match-score-line"><small>评分</small>${scoreChip(subjectScore)}</span>` : ""}</div>
          <div class="match-stats">${statRows}</div>
          <div class="match-build"><div class="match-items">${renderItemSlots(subject.itemIds)}</div><div class="match-badges">${rankChip}${scoreBadgeChip(subjectScore)}${badges.map((badge) => `<span>${badge}</span>`).join("")}</div></div>
        </div>
        ${renderMatchPlayers(match)}
        <div class="match-actions">
          <button class="match-expand" type="button" aria-expanded="${open}" aria-controls="match-detail-${match.gameId}" data-toggle-match="${match.gameId}" aria-label="${open ? "收起" : "展开"}这场对局" title="${open ? "收起详情" : "展开详情"}"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg></button>
          <button class="match-replay" type="button" data-replay="${match.gameId}" aria-label="观看这场对局的回放" title="观看回放"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5.5v13l11-6.5z"/></svg></button>
        </div>
      </div>
      ${open ? renderMatchDetail(match, subject, tab) : ""}
    </article>`;
  }

  /* ---------- 平均段位懒加载 ---------- */

  function applyMatchTierValue(container, scope, gameID, value) {
    if (!container || container.dataset.matchTierScope !== scope) return;
    for (const node of container.querySelectorAll("[data-match-tier]")) {
      if (String(node.dataset.gameId || "") !== String(gameID)) continue;
      node.removeAttribute("data-match-tier-pending");
      node.title = matchTierTitle(value);
      const slot = node.querySelector(".match-tier-value");
      if (slot) slot.innerHTML = matchTierContent(value);
    }
  }

  async function hydrateMatchTiers(container, tab, scope = matchTierScope(tab)) {
    // 国服依赖本机客户端；韩服由后端一次读取 OP.GG 对局页，不需要连接客户端。
    if (!riotTab(tab) && !connected()) return;
    const pendingNodes = [...container.querySelectorAll("[data-match-tier-pending]")];
    const pendingMatches = new Map();
    for (const node of pendingNodes) {
      const gameID = String(node.dataset.gameId || "");
      if (!gameID) continue;
      const cacheKey = matchTierCacheKey(tab, gameID);
      if (state.matchTiers.has(cacheKey)) { applyMatchTierValue(container, scope, gameID, state.matchTiers.get(cacheKey)); continue; }
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
          applyMatchTierValue(container, scope, gameID, value);
        }
      } catch (_) {
        // 当前容器明确降级为不可用，但不写缓存；重新进入页签时仍可重试。
        for (const [gameID] of entries) applyMatchTierValue(container, scope, gameID, null);
      } finally {
        for (const [, entry] of entries) state.matchTierFlights.delete(entry.cacheKey);
      }
      return;
    }

    // 国服保留原有单场契约：逐人从本机客户端读取排位后计算平均值。
    for (const [gameID, entry] of pendingMatches) {
      const match = entry.match;
      const refs = [...new Set((match?.participants || []).map((item) => item.playerRef).filter(Boolean))];
      if (!refs.length) { state.matchTiers.set(entry.cacheKey, null); applyMatchTierValue(container, scope, gameID, null); continue; }
      state.matchTierFlights.add(entry.cacheKey);
      try {
        const result = await api("/api/gameplay/match-tiers", { method: "POST", body: JSON.stringify({ playerRefs: refs, serverId: tabServerID(tab) }) }, `match-tiers:${entry.cacheKey}`, 20000);
        const value = result && Number(result.samples) > 0 ? result : null;
        state.matchTiers.set(entry.cacheKey, value);
        applyMatchTierValue(container, scope, gameID, value);
      } catch (_) {
        // 失败保持占位，下次渲染时重试。
      } finally {
        state.matchTierFlights.delete(entry.cacheKey);
      }
    }
  }

  function renderMatchDetail(match, subject, tab) {
    if (matchPlayerGroups(match).arena) {
      return `<div id="match-detail-${match.gameId}" class="match-detail is-arena-detail">${renderArenaMatchOverview(match)}</div>`;
    }
    const active = tab.matchDetailTabs.get(String(match.gameId)) || "overview";
    const panelID = `match-detail-body-${match.gameId}`;
    const detailTab = (value, label) => `<button id="match-detail-tab-${value}-${match.gameId}" class="${active === value ? "is-active" : ""}" type="button" role="tab" aria-selected="${active === value}" aria-controls="${panelID}" tabindex="${active === value ? 0 : -1}" data-match-detail="${value}" data-game-id="${match.gameId}">${label}</button>`;
    return `<div id="match-detail-${match.gameId}" class="match-detail"><div class="match-detail-head"><div class="match-detail-tabs" role="tablist" aria-label="对局详情">${detailTab("overview", "概览")}${detailTab("team", "队伍分析")}${detailTab("build", "构建")}</div></div><div id="${panelID}" role="tabpanel" aria-labelledby="match-detail-tab-${active}-${match.gameId}">${active === "overview" ? renderMatchOverview(match) : active === "team" ? renderTeamAnalysis(match) : renderBuild(match, subject, tab)}</div></div>`;
  }

  function matchTableRows(players, scores) {
    return players.map((item, index) => {
      const record = scores.get(Number(item.participantId));
      return `<tr><td><button class="participant-link" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"}>${iconFigure("champion", item.championId, item.championName, "small")}<span>${escapeHTML(playerParticipantName(item, index))}</span></button></td><td><span class="match-score-cell">${scoreBadgeChip(record)}${scoreChip(record)}</span></td><td>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}<small>${kda(item.kda)}:1</small></td><td>${number(item.damage)}<small>承伤 ${number(item.damageTaken)}</small></td><td>${number(item.wardsPlaced)} / ${number(item.wardsKilled)}</td><td>${number(item.cs)}<small>${number(item.csPerMinute)}/分钟</small></td><td><div class="table-items">${renderItemIcons(item.itemIds || [], "small")}</div></td></tr>`;
    }).join("");
  }

  function matchTableShell(rows) {
    return `<div class="table-scroll"><table class="match-table"><thead><tr><th>玩家</th><th>评分</th><th>KDA</th><th>伤害</th><th>守卫</th><th>CS</th><th>装备</th></tr></thead><tbody>${rows}</tbody></table></div>`;
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
      const emblem = team.iconPath ? assetIcon(team.iconPath, team.name, "small") : "";
      const players = group.players.map((item, index) => {
        const record = scores.get(Number(item.participantId));
        return `<div class="arena-detail-player"><button class="participant-link" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}"` : "disabled"}>${iconFigure("champion", item.championId, item.championName, "small")}<span>${escapeHTML(playerParticipantName(item, index))}</span></button><span class="arena-detail-kda"><b>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}</b><small>${kda(item.kda)}:1 KDA</small></span><span class="arena-detail-damage"><b>${compactNumber(item.damage)}</b><small>伤害 · 承伤 ${compactNumber(item.damageTaken)}</small></span><span class="arena-detail-augments">${(item.augmentIds || []).map((id) => augmentIconFigure(id, "small")).join("")}</span><span class="arena-detail-items">${renderItemIcons(item.itemIds || [], "small")}</span><span class="match-score-cell">${scoreBadgeChip(record)}${scoreChip(record)}</span></div>`;
      }).join("");
      return `<section class="arena-detail-team${tone}"><header><b>#${number(group.placement || "—")}</b>${emblem}<strong>${escapeHTML(team.name)}</strong><span>${number(kills)} 击杀 · ${compactNumber(damage)} 伤害</span></header><div class="arena-detail-player-list">${players}</div></section>`;
    }).join("");
    return `<div class="arena-match-detail"><header><strong>最终排名</strong><span>按小队名次展示全部玩家、海克斯与装备</span></header>${sections}</div>`;
  }

  function renderTeamAnalysis(match) {
    const participants = match.participants || [];
    const maxDamage = Math.max(1, ...participants.map((item) => Number(item.damage) || 0));
    const maxTaken = Math.max(1, ...participants.map((item) => Number(item.damageTaken) || 0));
    // 每名玩家两行：上行“伤害”（按队伍色），下行“承伤”（灰色），
    // 数值直接标在各自条的末端，一眼分清哪个数字属于哪条。
    const damageRow = (item, index, tone) => {
      const damageShare = Math.round((Number(item.damage) || 0) * 100 / maxDamage);
      const takenShare = Math.round((Number(item.damageTaken) || 0) * 100 / maxTaken);
      return `<div class="damage-row">${iconFigure("champion", item.championId, item.championName, "tiny")}<span class="damage-name" title="${escapeHTML(playerParticipantName(item, index))}">${escapeHTML(playerParticipantName(item, index))}</span><span class="damage-bar-lines"><span class="damage-bar-line"><em>伤害</em><span class="damage-bar is-${tone}"><i style="width:${Math.max(2, damageShare)}%"></i></span><b>${number(item.damage)}</b></span><span class="damage-bar-line"><em>承伤</em><span class="damage-bar is-taken"><i style="width:${Math.max(2, takenShare)}%"></i></span><b>${number(item.damageTaken)}</b></span></span></div>`;
    };
    const grouping = matchPlayerGroups(match);
    // 斗魂竞技场：红蓝双方总量对比无意义，改为按名次逐小队展示伤害与承伤。
    if (grouping.arena) {
      const sections = grouping.groups.map((group) => `<section class="damage-team is-arena"><h4>${escapeHTML(arenaGroupLabel(group))}</h4>${group.players.map((item, index) => damageRow(item, index, group.placement === 1 ? "blue" : "red")).join("")}</section>`).join("");
      return `<div class="damage-compare is-arena">${sections}</div>`;
    }
    const metrics = [["kills", "英雄击杀"], ["gold", "金币"], ["damage", "伤害"], ["visionScore", "视野分数"], ["damageTaken", "承伤"], ["cs", "小兵分数"]];
    const blue = (match.teams || []).find((team) => team.teamId === 100) || {};
    const red = (match.teams || []).find((team) => team.teamId === 200) || {};
    // 红蓝双色对峙条：左段蓝方、右段红方，长度即各自占比。
    const totals = `<div class="team-analysis-grid">${metrics.map(([key, label]) => {
      const left = Number(blue[key] || 0); const right = Number(red[key] || 0); const total = Math.max(1, left + right); const share = Math.round(left * 100 / total);
      return `<section class="team-metric"><div class="team-metric-head"><b class="team-metric-value is-blue">${number(left)}</b><h4>${label}</h4><b class="team-metric-value is-red">${number(right)}</b></div><div class="team-metric-track" role="img" aria-label="${label}：蓝方 ${share}%，红方 ${100 - share}%" title="蓝方 ${share}% · 红方 ${100 - share}%"><i style="--blue-share:${share}%"></i></div></section>`;
    }).join("")}</div>`;
    // 逐人伤害与承伤对比：直观看出各队输出核心与前排承压。
    const contribution = `<div class="damage-compare"><section class="damage-team is-blue"><h4>蓝方伤害 / 承伤</h4>${participants.filter((item) => item.teamId === 100).map((item, index) => damageRow(item, index, "blue")).join("")}</section><section class="damage-team is-red"><h4>红方伤害 / 承伤</h4>${participants.filter((item) => item.teamId === 200).map((item, index) => damageRow(item, index, "red")).join("")}</section></div>`;
    return totals + contribution;
  }

  /* ---------- 对局时间线（装备路线 + 技能加点） ---------- */

  function matchTimelineKey(match, subject, tab) {
    return `${riotTab(tab) ? "kr" : `cn:${tabServerID(tab) || "current"}`}:${match.gameId}:${Number(subject?.participantId) || 0}`;
  }

  async function ensureMatchTimeline(match, subject, tab) {
    const participantId = Number(subject?.participantId);
    if (!match || !participantId) return;
    // 国服时间线需要本机客户端（或 SGP 令牌）；未连接时不请求。
    if (!riotTab(tab) && !connected()) return;
    const key = matchTimelineKey(match, subject, tab);
    if (state.matchTimelines.has(key) || state.matchTimelineFlights.has(key)) return;
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
      settled = true;
    } catch (error) {
      if (error.name !== "RequestCancelled") {
        state.matchTimelines.set(key, { available: false, detail: error.message });
        settled = true;
      }
    } finally {
      state.matchTimelineFlights.delete(key);
      if (settled) rerenderTab(tab);
    }
  }

  // 装备路线：按分钟分组的购买/出售记录，组间用箭头衔接（同 OP.GG）。
  function renderItemRoute(timeline) {
    const groups = timeline?.itemGroups || [];
    if (!groups.length) return "";
    return `<div class="timeline-route">${groups.map((group) => `<div class="timeline-group"><div class="timeline-group-items">${(group.events || []).map((event) => `<span class="timeline-item${event.sold ? " is-sold" : ""}"${event.sold ? ' title="已出售"' : ""}>${itemIconFigure(event.itemId, "slot")}</span>`).join("")}</div><small>${number(group.minute)}分</small></div>`).join('<span class="timeline-arrow" aria-hidden="true">›</span>')}</div>`;
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
    const cells = order.map((up) => `<span class="match-skill-cell is-slot-${up.slot}" title="第 ${up.level} 级：升级 ${SKILL_SLOT_LETTERS[up.slot] || "?"}"><b>${SKILL_SLOT_LETTERS[up.slot] || "?"}</b><small>${up.level}</small></span>`).join("");
    return `<div class="match-skill-seq">${cells}</div>`;
  }

  function renderBuild(match, subject, tab) {
    const runeContent = subject.perkIds?.length ? renderRuneBoard(subject) : '<div class="detail-empty"><strong>这场对局没有符文数据</strong><p>部分娱乐模式会关闭符文系统。</p></div>';
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
      <section><header><h4>装备路线</h4><span>按购买时间分组，含出售记录</span></header>${routeContent}</section>
      <section><header><h4>技能加点</h4>${skillSummary || "<span>按对局中的真实加点顺序</span>"}</header>${skillContent}</section>
      <section class="rune-detail"><header><h4>符文</h4><span>已选图标完整展示，未选符文降低亮度</span></header>${runeContent}</section>
    </div>`;
  }

  // 符文悬停说明：名称 + 完整效果描述（与英雄详情页一致的浮层提示）。
  function runeTooltip(perk, id) {
    const name = perk?.name || `符文 ${id}`;
    const explanation = runeShardDescription(id) || plainText(perk?.longDesc || perk?.shortDesc || "");
    return explanation && explanation !== name ? `${name}\n${explanation}` : name;
  }

  function renderRuneBoard(config) {
    const selected = new Set((config.perkIds || config.selectedPerkIds || []).map(Number));
    if (!state.perks) return `<div class="selected-rune-strip">${[...selected].map((id) => perkIconFigure(id, "rune")).join("")}</div><p class="muted rune-loading-copy">正在读取完整符文树…</p>`;
    const styles = state.perks.styles || [];
    const renderStyle = (styleID, secondary = false) => {
      const style = styles.find((item) => Number(item.id) === Number(styleID));
      if (!style) return "";
      const slots = (style.slots || []).map((slot) => `<div class="rune-slot">${(slot.perks || []).map((perk) => {
        const record = perkRecord(perk.id);
        const tooltip = runeTooltip({ ...perk, ...record }, perk.id);
        return `<span class="rune-option${selected.has(Number(perk.id)) ? " is-selected" : ""}" data-tooltip="${escapeHTML(tooltip)}" title="${escapeHTML(perk.name)}">${assetIcon(perk.iconPath, perk.name, "rune")}</span>`;
      }).join("")}</div>`).join("");
      return `<section class="rune-tree${secondary ? " is-secondary" : ""}"><header>${assetIcon(style.iconPath, style.name, "small")}<strong>${escapeHTML(style.name)}</strong></header>${slots}</section>`;
    };
    const known = new Set();
    for (const style of styles) for (const slot of style.slots || []) for (const perk of slot.perks || []) known.add(Number(perk.id));
    // 属性碎片：图标用 Data Dragon 的 StatMods 图，悬停显示具体加成。
    const fragments = [...selected].filter((id) => !known.has(id));
    const fragmentIcons = fragments.map((id) => {
      const record = perkRecord(id);
      const name = record?.name || `属性碎片 ${id}`;
      const shardPath = dataDragonRuneShardPath(id);
      const icon = shardPath ? remoteStaticIcon("ddragon", shardPath, name, "rune") : perkIconFigure(id, "rune");
      return `<span class="rune-option is-selected" data-tooltip="${escapeHTML(runeTooltip(record, id))}" title="${escapeHTML(name)}">${icon}</span>`;
    }).join("");
    return `<div class="rune-board">${renderStyle(config.primaryStyleId, false)}${renderStyle(config.subStyleId, true)}${fragments.length ? `<section class="rune-fragments"><strong>属性碎片</strong><div>${fragmentIcons}</div></section>` : ""}</div>`;
  }

  function rerenderCatalogViews() {
    if (state.section === "overview") renderOverview();
    if (state.section === "live") renderLive();
    if (state.overlay.length) renderOverlay();
  }

  // 图标目录（符文/装备/召唤师技能）：客户端连接时来自本机客户端；
  // 未连接时后端会回退到 Data Dragon，因此不再限制连接状态。
  async function ensurePerks() {
    if (state.perks || state.perksLoading) return;
    state.perksLoading = true;
    try { state.perks = await api("/api/gameplay/perks", {}, "perks", 15000); }
    catch (_) { state.perks = null; }
    finally {
      state.perksLoading = false;
      if (state.perks) rerenderCatalogViews();
    }
  }

  async function ensureItems() {
    if (state.items || state.itemsLoading) return;
    state.itemsLoading = true;
    try { state.items = await api("/api/gameplay/items", {}, "items", 15000); }
    catch (_) { state.items = null; }
    finally {
      state.itemsLoading = false;
      if (state.items) rerenderCatalogViews();
    }
  }

  async function ensureSummonerSpells() {
    if (state.summonerSpells || state.summonerSpellsLoading) return;
    state.summonerSpellsLoading = true;
    try { state.summonerSpells = await api("/api/gameplay/summoner-spells", {}, "summoner-spells", 15000); }
    catch (_) { state.summonerSpells = null; }
    finally {
      state.summonerSpellsLoading = false;
      if (state.summonerSpells) rerenderCatalogViews();
    }
  }

  // 从玩家按钮中提取干净的显示名：图标占位符（aria-hidden 的首字母）
  // 不能混入，否则新页签的名称会莫名多出一个字。
  function playerButtonLabel(button) {
    const named = button.querySelector("span:not([aria-hidden]):not(.game-icon)");
    const label = (button.title || named?.textContent || "").trim();
    if (label) return label;
    const clone = button.cloneNode(true);
    for (const hidden of clone.querySelectorAll('[aria-hidden="true"], .game-icon')) hidden.remove();
    return clone.textContent.trim();
  }

  function bindOverviewContent(container, tab) {
    const rerender = () => rerenderTab(tab);
    // 点击页面内的玩家名称时沿用当前页面所属服务器（两服玩家互不相通）；
    // 在覆盖层或非总览页内点击时，继续以覆盖层方式打开。
    const sourceRegion = tab.region || "";
    const sourceServerID = tabServerID(tab);
    for (const button of container.querySelectorAll("[data-player-ref]")) button.addEventListener("click", () => {
      const label = playerButtonLabel(button);
      if (tab.overlay) openPlayerOverlay({ playerRef: button.dataset.playerRef, region: sourceRegion, serverId: sourceServerID, label });
      else openPlayer(button.dataset.playerRef, label, sourceRegion, sourceServerID);
    });
    container.querySelector("[data-gameplay-retry]")?.addEventListener("click", () => loadOverview(tab, true));
    for (const button of container.querySelectorAll("[data-match-filter]")) button.addEventListener("click", () => { tab.matchFilter = button.dataset.matchFilter; tab.openMatches.clear(); tab.matchDetailTabs.clear(); rerender(); });
    container.querySelector("[data-match-more]")?.addEventListener("change", (event) => { if (event.target.value) { tab.matchFilter = event.target.value; tab.openMatches.clear(); tab.matchDetailTabs.clear(); rerender(); } });
    container.querySelector("[data-load-more]")?.addEventListener("click", () => loadOverview(tab, false, true));
    const sentinel = container.querySelector("[data-match-sentinel]");
    if (sentinel && tab.data?.pagination?.hasMore && !tab.loadingMore && "IntersectionObserver" in window) {
      const scrollRoot = container.closest(".player-overlay-scroll") || document.getElementById("app-scroll");
      const observer = new IntersectionObserver((entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadOverview(tab, false, true);
      }, { root: scrollRoot, rootMargin: "0px 0px 180px 0px" });
      observer.observe(sentinel);
      if (tab.overlay) state.overlayObserver = observer;
      else state.matchObserver = observer;
    }
    for (const button of container.querySelectorAll("[data-toggle-match]")) button.addEventListener("click", () => {
      const id = button.dataset.toggleMatch;
      if (tab.openMatches.has(id)) {
        tab.openMatches.delete(id);
        // 收起后清除页签记忆，重新展开时回到“概览”。
        tab.matchDetailTabs.delete(id);
      } else {
        tab.openMatches.add(id);
      }
      rerender();
    });
    for (const button of container.querySelectorAll("[data-match-detail]")) button.addEventListener("click", () => {
      tab.matchDetailTabs.set(button.dataset.gameId, button.dataset.matchDetail);
      if (button.dataset.matchDetail === "build") {
        ensurePerks(); ensureItems();
        const match = (tab.data?.matches || []).find((item) => String(item.gameId) === String(button.dataset.gameId));
        if (match) ensureMatchTimeline(match, matchSubject(match, tab.data?.player?.playerRef), tab);
      }
      rerender();
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
    for (const button of container.querySelectorAll("[data-replay]")) button.addEventListener("click", () => replay(button));
    // 已展开且停在“构建”页签的对局：确保时间线已请求（幂等，命中缓存
    // 或已在途时直接返回），避免请求被取消后一直停留在加载文案。
    for (const gameId of tab.openMatches) {
      if ((tab.matchDetailTabs.get(String(gameId)) || "overview") !== "build") continue;
      const match = (tab.data?.matches || []).find((item) => String(item.gameId) === String(gameId));
      if (match) ensureMatchTimeline(match, matchSubject(match, tab.data?.player?.playerRef), tab);
    }
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
    if (!connected()) { renderLive(); return; }
    if (state.liveLoading && !force) return;
    state.liveLoading = true;
    state.liveError = "";
    renderLive();
    try {
      state.live = await api("/api/gameplay/live", {}, "live", 45000);
      state.lastCapabilities = state.live.capabilities || state.lastCapabilities;
      renderCapabilitySettings();
      const recommendations = liveRecommendationsFor(state.live);
      if (state.live.clientRecommendation || recommendations?.runes) ensurePerks();
      if (recommendations?.build) { ensureItems(); ensureSummonerSpells(); }
      ensureLiveRecommendations(state.live);
      ensureSpecialistRunes(state.live);
    } catch (error) {
      if (error.name !== "RequestCancelled") state.liveError = error.message;
    } finally {
      state.liveLoading = false;
      renderLive();
      scheduleLiveRefresh();
    }
  }

  function liveRecommendationTarget(data) {
    const self = (data?.players || []).find((player) => player.isCurrent);
    if (!self?.championId) return null;
    const position = self.position || "other";
    return { self, position, key: `${Number(self.championId)}:${position}` };
  }

  function liveRecommendationsFor(data) {
    if (data?.recommendations) return data.recommendations;
    const target = liveRecommendationTarget(data);
    return target ? state.liveRecommendations.get(target.key) || null : null;
  }

  function specialistRunesFor(data) {
    const target = liveRecommendationTarget(data);
    if (target && state.specialistRunes.has(target.key)) return state.specialistRunes.get(target.key);
    const embedded = liveRecommendationsFor(data)?.runes?.specialists;
    return Array.isArray(embedded) ? embedded : [];
  }

  async function ensureLiveRecommendations(data) {
    const target = liveRecommendationTarget(data);
    if (!target || data?.recommendations || state.liveRecommendations.has(target.key) || state.liveRecommendationFlights.has(target.key)) return;
    if (Date.now() - Number(state.liveRecommendationFailures.get(target.key) || 0) < 60_000) return;
    state.liveRecommendationFlights.add(target.key);
    const query = new URLSearchParams({ championId: String(target.self.championId), position: target.position });
    try {
      const response = await api(`/api/gameplay/recommendations?${query}`, {}, `live-recommendations:${target.key}`, 20000);
      if (!response?.recommendations) throw new Error("推荐数据不完整");
      state.liveRecommendations.set(target.key, response.recommendations);
      state.liveRecommendationFailures.delete(target.key);
      ensurePerks();
      ensureItems();
      ensureSummonerSpells();
      if (liveRecommendationTarget(state.live)?.key === target.key) renderLive();
    } catch (error) {
      if (error.name !== "RequestCancelled") state.liveRecommendationFailures.set(target.key, Date.now());
    } finally {
      state.liveRecommendationFlights.delete(target.key);
    }
  }

  async function ensureSpecialistRunes(data) {
    const target = liveRecommendationTarget(data);
    const embedded = liveRecommendationsFor(data)?.runes?.specialists;
    if (!target || (Array.isArray(embedded) && embedded.length) || state.specialistRunes.has(target.key) || state.specialistRuneFlights.has(target.key)) return;
    if (Date.now() - Number(state.specialistRuneFailures.get(target.key) || 0) < 60_000) return;
    state.specialistRuneFlights.add(target.key);
    if (liveRecommendationTarget(state.live)?.key === target.key) renderLive();
    const query = new URLSearchParams({ championId: String(target.self.championId), position: target.position });
    try {
      const response = await api(`/api/gameplay/specialist-runes?${query}`, {}, `live-specialist-runes:${target.key}`, 45000);
      if (!Array.isArray(response)) throw new Error("绝活哥符文数据不完整");
      state.specialistRunes.set(target.key, response);
      state.specialistRuneFailures.delete(target.key);
      if (response.length) ensurePerks();
      if (liveRecommendationTarget(state.live)?.key === target.key) renderLive();
    } catch (error) {
      if (error.name !== "RequestCancelled") state.specialistRuneFailures.set(target.key, Date.now());
    } finally {
      state.specialistRuneFlights.delete(target.key);
      if (liveRecommendationTarget(state.live)?.key === target.key) renderLive();
    }
  }

  function renderSessionSummary(data) {
    if (!nodes.liveSessionSummary) return;
    if (!connected() || !data) {
      nodes.liveSessionSummary.hidden = true;
      nodes.liveSessionSummary.innerHTML = "";
      return;
    }
    const phase = phaseLabel(data.phase);
    const note = state.liveLoading ? "正在刷新" : state.settings.liveRefresh ? `每 ${state.settings.liveInterval} 秒自动刷新` : "手动刷新";
    nodes.liveSessionSummary.hidden = false;
    nodes.liveSessionSummary.innerHTML = data.available
      ? `<span class="state-chip success">${escapeHTML(phase)}</span><div class="live-session-copy"><strong>${escapeHTML(data.queueLabel || "当前对局")}</strong><span>${escapeHTML(data.gameMode || "游戏模式未知")} · 地图 ${number(data.mapId)}${data.gameId ? ` · 对局 ${number(data.gameId)}` : ""} · ${escapeHTML(note)}</span></div>`
      : `<span class="state-chip">${escapeHTML(phase)}</span><div class="live-session-copy"><strong>${phase === "大厅" ? "等待进入对局" : escapeHTML(phase)}</strong><span>进入英雄选择或游戏后自动读取 · ${escapeHTML(note)}</span></div>`;
  }

  function renderLive() {
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
    if (!data?.available) {
      nodes.liveContent.innerHTML = renderRecommendationArea(data || {});
      bindLiveContent();
      return;
    }
    nodes.liveContent.innerHTML = renderRecommendationArea(data);
    bindLiveContent();
    prepareImages(nodes.liveContent);
  }

  function renderLivePlayer(player, index) {
    const rank = player.rank;
    const stats = player.modeStats || {};
    const displayName = maskedPlayerName(player, index);
    return `<article class="live-player${player.isCurrent ? " is-self" : ""}">${iconFigure("champion", player.championId, player.championName, "live")}<div class="live-player-copy"><button type="button" ${player.playerRef ? `data-player-ref="${escapeHTML(player.playerRef)}"` : "disabled"} title="${escapeHTML(displayName)}">${escapeHTML(displayName)}</button><span>${escapeHTML(positionLabel(player.position))} · ${rank?.tier ? escapeHTML(rankTitle(rank)) : "未定级"}</span></div><dl><div><dt>当前模式</dt><dd>${stats.games ? `${number(stats.wins)}胜 ${number(stats.losses)}负` : "暂无样本"}</dd></div><div><dt>胜率</dt><dd>${stats.games ? percent(stats.winRate) : "—"}</dd></div><div><dt>KDA</dt><dd>${stats.games ? `${kda(stats.kda)}:1` : "—"}</dd></div></dl>${player.isCurrent ? '<span class="self-chip">自己</span>' : ""}</article>`;
  }

  function orderLivePlayers(players) {
    const ordered = [...players];
    if (state.settings.liveOrder === "team") return ordered;
    const positions = { top: 1, jungle: 2, middle: 3, bottom: 4, utility: 5, other: 6 };
    return ordered.sort((left, right) => {
      if (left.teamId !== right.teamId) return Number(left.teamId) - Number(right.teamId);
      if (state.settings.liveOrder === "position") return (positions[left.position] || 99) - (positions[right.position] || 99);
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

  function renderRecommendationArea(data) {
    const self = (data.players || []).find((player) => player.isCurrent);
    const payload = liveRecommendationsFor(data) || {};
    const waiting = !data.available;
    const noChampion = !waiting && !self?.championId;
    const selected = selectedRuneRecommendation(data);
    const selectedComplete = Boolean(selected && (selected.selectedPerkIds || selected.perkIds || []).length >= 6);
    const tab = (key, label) => `<button type="button" role="tab" id="recommendation-tab-${key}" aria-controls="recommendation-panel-${key}" aria-selected="${state.recommendationTab === key}" tabindex="${state.recommendationTab === key ? "0" : "-1"}" class="${state.recommendationTab === key ? "is-active" : ""}" data-recommendation-tab="${key}">${label}</button>`;
    const panel = (key, content) => `<div id="recommendation-panel-${key}" class="recommendation-panel" role="tabpanel" aria-labelledby="recommendation-tab-${key}" ${state.recommendationTab !== key ? "hidden" : ""}>${content}</div>`;
    let runeContent;
    let insightContent;
    let buildContent;
    if (waiting) {
      runeContent = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示 OPGG、绝活哥与职业选手的符文推荐。");
      insightContent = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示双方玩家与最近战绩。");
      buildContent = recommendationEmptyPanel("等待进入对局", "进入英雄选择或游戏后，这里会展示召唤师技能、技能加点与出装路线。");
    } else {
      insightContent = renderLiveInsights(data);
      if (noChampion) {
        runeContent = recommendationEmptyPanel("请先选定英雄", "锁定或预选英雄后自动读取符文推荐。");
        buildContent = recommendationEmptyPanel("请先选定英雄", "锁定或预选英雄后自动读取出装与技能数据。");
      } else {
        const runePanel = renderRuneRecommendations(data, true);
        runeContent = `${renderChampionRecommendationHeader(payload.hero, self)}${runePanel}<footer class="recommendation-action"><div><strong>${selected ? `已选择：${escapeHTML(selected.title || selected.name || "符文配置")}` : "请选择一套完整符文"}</strong><span>${selectedComplete ? "将新建符文页并设为当前页" : "这套数据尚不完整，暂时不能应用"}</span></div><button class="button button-primary apply-runes" type="button" data-apply-runes="selected" ${!selectedComplete || data.phase !== "ChampSelect" ? "disabled" : ""}>${data.phase === "ChampSelect" ? "应用所选符文" : "仅英雄选择阶段可应用"}</button></footer>`;
        buildContent = renderBuildRecommendation(payload.build, self, data.championAbilities || [], data);
      }
    }
    return `<section class="recommendation-area"><div class="recommendation-tabs" role="tablist" aria-label="推荐类型">${tab("runes", "符文")}${tab("insight", "详情")}${tab("build", "出装与技能")}</div>${panel("runes", runeContent)}${panel("insight", insightContent)}${panel("build", buildContent)}</section>`;
  }

  // “详情”页签：双方队伍与每名玩家的最近战绩（英雄、KDA、评分），从新到旧。
  function insightScore(game) {
    const value = (Number(game.kills) + Number(game.assists)) / Math.max(1, Number(game.deaths));
    const tone = value >= 4 ? "is-gold" : value >= 3 ? "is-good" : value < 1.5 ? "is-poor" : "";
    return `<i class="insight-score ${tone}">${value.toFixed(1)}</i>`;
  }

  function renderInsightMatches(player) {
    const games = (player.recentGames || []).slice(0, 8);
    if (!games.length) return '<div class="insight-match-row"><small class="insight-none">该玩家当前模式暂无最近战绩</small></div>';
    const cells = games.map((game) => `<span class="insight-match is-${game.win ? "win" : "loss"}" title="${escapeHTML(`${game.championName || "英雄"} · ${number(game.kills)}/${number(game.deaths)}/${number(game.assists)}${Number(game.cs) > 0 ? ` · CS ${number(game.cs)}` : ""} · ${game.win ? "胜利" : "失败"}`)}">${iconFigure("champion", game.championId, game.championName, "tiny")}<b>${number(game.kills)}/${number(game.deaths)}/${number(game.assists)}</b>${insightScore(game)}</span>`).join("");
    return `<div class="insight-match-row" aria-label="当前模式最近战绩，从左到右由新到旧">${cells}</div>`;
  }

  function renderLiveInsights(data) {
    const players = data.players || [];
    if (!players.length) return '<div class="recommendation-empty"><strong>暂无玩家数据</strong><p>进入英雄选择或对局后，这里会展示自己、队友与对手的队伍信息与最近战绩。</p></div>';
    const orderedPlayers = orderLivePlayers(players);
    const team = (teamID, label, tone) => {
      const rows = orderedPlayers.filter((player) => player.teamId === teamID).map((player, index) => `${renderLivePlayer(player, index)}${renderInsightMatches(player)}`).join("");
      return `<section class="live-team is-${tone}"><header><h3>${label}</h3><span>${players.filter((player) => player.teamId === teamID).length} 名玩家 · 当前模式最近战绩，点击名字查看总览</span></header><div class="live-player-list is-insight">${rows || '<p class="section-empty">客户端暂未公开这一队的玩家</p>'}</div></section>`;
    };
    return `<div class="live-teams is-insight">${team(100, "我方", "blue")}${team(200, "对方", "red")}</div>`;
  }

  function renderChampionRecommendationHeader(hero, self) {
    const stats = hero || {};
    const matchups = (items) => (items || []).slice(0, 5).map((item) => iconFigure("champion", item.championId || item.id, item.championName || `英雄 ${item.championId || item.id}`, "small")).join("") || '<span class="muted">—</span>';
    return `<section class="recommendation-champion-summary"><div class="champion-summary-main">${self?.championId ? iconFigure("champion", self.championId, self.championName, "live") : '<span class="game-icon is-live">?</span>'}<div><strong>${escapeHTML(self?.championName || "尚未选择英雄")}</strong><span>${escapeHTML(positionLabel(self?.position))}</span></div><dl><div><dt>胜率</dt><dd>${rate(stats.winRate)}</dd></div><div><dt>选取率</dt><dd>${rate(stats.pickRate)}</dd></div><div><dt>禁用率</dt><dd>${rate(stats.banRate)}</dd></div></dl></div><div class="champion-matchups"><div><span>优势对抗</span>${matchups(stats.strongAgainst)}</div><div><span>劣势对抗</span>${matchups(stats.weakAgainst)}</div></div></section>`;
  }

  function renderRuneRecommendations(data, championSelected) {
    const runes = liveRecommendationsFor(data)?.runes || {};
    const target = liveRecommendationTarget(data);
    const specialistFailed = Boolean(target && state.specialistRuneFailures.has(target.key) && !state.specialistRunes.has(target.key));
    const opggItems = Array.isArray(runes.opgg) ? runes.opgg : runes.opgg ? [runes.opgg] : [];
    const opggFallback = opggItems.length ? opggItems : data.clientRecommendation ? [{ ...data.clientRecommendation, key: "opgg", title: "客户端内置（备用）" }] : [];
    const sections = [
      { key: "opgg", title: "OPGG", items: opggFallback },
      { key: "specialist", title: "绝活哥", items: specialistRunesFor(data), loading: Boolean(target && state.specialistRuneFlights.has(target.key)), failed: specialistFailed },
      { key: "pro", title: "职业选手", items: runes.pros || [] },
    ];
    return `<div class="rune-source-stack">${sections.map((section) => renderRuneSourceSection(section, championSelected)).join("")}</div>`;
  }

  function renderRuneSourceSection(section, championSelected) {
    const unavailableReason = {
      specialist: "最近对局中没有找到完整且可核验的该英雄符文。",
      pro: "当前数据源不提供可核验的职业选手身份与完整符文，暂不展示。",
    };
    const emptyTitle = !championSelected ? "请先选定英雄" : section.loading ? "正在读取韩服绝活哥符文" : section.failed ? "韩服绝活哥符文读取失败" : section.key === "opgg" ? "等待完整符文数据" : section.key === "specialist" ? "暂无可核验的韩服绝活哥符文" : "暂无可核验数据源";
    const content = section.items.length
      ? `<div class="rune-choice-list" role="radiogroup" aria-label="${escapeHTML(section.title)}符文">${section.items.map((config, index) => renderRuneChoice({ ...config, sourceKey: section.key, key: config.key || (section.key === "opgg" && index === 0 ? "opgg" : `${section.key}-${index}`) })).join("")}</div>`
      : `<div class="recommendation-empty"><strong>${emptyTitle}</strong><p>${section.loading ? "正在核对专家榜玩家最近对局中的完整符文。" : section.failed ? "本次后台读取未完成，稍后刷新时会自动重试。" : unavailableReason[section.key] || "OPGG 返回完整主系、副系与属性碎片后即可选择。"}</p></div>`;
    return `<section class="rune-source-section"><header><h3>${escapeHTML(section.title)}</h3></header>${content}</section>`;
  }

  function renderRuneChoice(config) {
    const key = String(config.key || "");
    const selected = state.selectedRecommendation === key;
    const stats = config.stats || config;
    const games = config.championGames ?? stats.games ?? stats.play;
    const winRate = stats.winRate ?? stats.win_rate ?? (stats.win && games ? stats.win / games : NaN);
    const specialistStats = config.sourceKey === "specialist" ? `<dl class="rune-player-stats"><div><dt>场次</dt><dd>${compactNumber(games)}</dd></div><div><dt>胜率</dt><dd>${rate(winRate)}</dd></div></dl>` : "";
    const riotID = `${config.playerName || ""}${config.tagLine ? `#${config.tagLine}` : ""}`;
    const specialistRank = config.tier ? rankTitle({ tier: config.tier, division: config.division }) : "";
    const played = relativeTime(config.playedAt);
    const result = config.result === "win" ? "胜利" : config.result === "loss" ? "失败" : "";
    const sourceParts = config.sourceKey === "specialist" ? [riotID && `来自 ${riotID}`, specialistRank, played && `${played}的${config.championName || "该英雄"}`, config.region === "kr" ? "韩服" : "", result].filter(Boolean) : [];
    const title = config.sourceKey === "specialist" ? `${config.championName || "该英雄"} · 绝活哥` : config.title || config.name || config.playerName || "推荐符文";
    const choiceCopy = `<span class="rune-choice-copy"><strong>${escapeHTML(title)}</strong>${sourceParts.length ? `<small>${escapeHTML(sourceParts.join(" · "))}</small>` : ""}</span>`;
    return `<article class="rune-choice-card${selected ? " is-selected" : ""}"><button class="rune-choice-selector" type="button" role="radio" aria-checked="${selected}" data-rune-choice="${escapeHTML(key)}"><span class="radio-mark" aria-hidden="true"></span>${choiceCopy}${specialistStats}</button>${renderUnifiedRuneBoard(config)}</article>`;
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
      return `<section class="unified-rune-column${secondary ? " is-secondary" : ""}" aria-label="${escapeHTML(style.name)}"><header aria-hidden="true">${renderRuneStyleIcon(style)}</header>${secondary ? '<div class="unified-rune-row is-spacer" aria-hidden="true"></div>' : ""}${slots.map((slot) => `<div class="unified-rune-row">${(slot.perks || []).map((perk) => renderRuneOption(perk, selected.has(Number(perk.id)))).join("")}</div>`).join("")}</section>`;
    };
    const fragments = [
      [5005, 5008, 5007],
      [5008, 5010, 5001],
      [5011, 5013, 5001],
    ];
    const fragmentSelections = (config.statModIds || config.stat_mod_ids || (selectedIDs.length >= 9 ? selectedIDs.slice(-3) : [])).map(Number);
    return `<div class="unified-rune-board">${renderStyle(config.primaryStyleId, false)}${renderStyle(config.subStyleId, true)}<section class="unified-rune-column rune-shards" aria-label="属性碎片"><header aria-hidden="true"></header><div class="unified-rune-row is-spacer" aria-hidden="true"></div>${fragments.map((row, rowIndex) => `<div class="unified-rune-row">${row.map((id) => renderRuneOption(perkRecord(id), fragmentSelections[rowIndex] === id, id)).join("")}</div>`).join("")}</section></div>`;
  }

  function renderRuneOption(perk, selected, fallbackID = 0) {
    const id = Number(perk?.id || fallbackID);
    const name = perk?.name || `符文 ${id}`;
    const explanation = runeShardDescription(id) || plainText(perk?.longDesc || perk?.shortDesc || name);
    const shardPath = dataDragonRuneShardPath(id);
    const icon = shardPath ? remoteStaticIcon("ddragon", shardPath, name, "rune") : perk?.iconPath ? assetIcon(perk.iconPath, name, "rune") : iconFigure("perk", id, name, "rune");
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
    return `<button class="rune-option-button${selected ? " is-selected" : ""}" type="button" tabindex="0" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-tooltip="${escapeHTML(tooltip)}">${icon}</button>`;
  }

  function renderRuneStyleIcon(style) {
    const names = { 8000: "precision", 8100: "domination", 8200: "sorcery", 8300: "inspiration", 8400: "resolve" };
    const file = names[Number(style?.id)];
    if (!file) return assetIcon(style?.iconPath, style?.name || "符文系", "rune-style");
    const label = style?.name || "符文系";
    return `<span class="game-icon is-rune-style" title="${escapeHTML(label)}"><img src="/rune-styles/${file}.svg" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`;
  }

  function dataDragonRuneShardPath(id) {
    const name = ({ 5001: "StatModsHealthScalingIcon.png", 5005: "StatModsAttackSpeedIcon.png", 5007: "StatModsCDRScalingIcon.png", 5008: "StatModsAdaptiveForceIcon.png", 5010: "StatModsMovementSpeedIcon.png", 5011: "StatModsHealthPlusIcon.png", 5013: "StatModsTenacityIcon.png" })[Number(id)];
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
    return { id, name: `属性碎片 ${id}`, iconPath: assetPath("perk", id) };
  }

  function renderRecommendationStats(stats) {
    if (!stats || !Object.keys(stats).length) return "";
    return `<dl class="recommendation-stats"><div><dt>选取率</dt><dd>${rate(stats.pickRate)}</dd></div><div><dt>场次</dt><dd>${compactNumber(stats.games || stats.play)}</dd></div><div><dt>胜率</dt><dd>${rate(stats.winRate ?? (stats.win && stats.play ? stats.win / stats.play : NaN))}</dd></div></dl>`;
  }

  function renderBuildRecommendation(build, self, abilities, data) {
    if (!build) return recommendationEmptyPanel(self?.championId ? "等待 OPGG 出装与技能数据" : "请先选定英雄", "仅展示召唤师技能、技能加点、出门装与装备路线。");
    const spellOptions = build.spellOptions || (build.spells ? [{ ids: build.spells, stats: build.spellStats || {} }] : []);
    const starterOptions = build.starterOptions || (build.starterItems ? [{ ids: build.starterItems, stats: build.starterStats || {} }] : []);
    const bootOptions = build.bootOptions || (build.boots ? [{ ids: build.boots, stats: build.bootStats || {} }] : []);
    const itemRoutes = build.itemRoutes || (build.itemRoute ? [{ ids: build.itemRoute, stats: build.routeStats || {} }] : []);
    const itemSet = buildItemSetPayload(build, self, data);
    const canApply = itemSet.blocks.length > 0 && data?.phase === "ChampSelect";
    const action = `<footer class="recommendation-action item-set-action"><div><strong>客户端装备方案</strong><span>${itemSet.blocks.length ? `${itemSet.blocks.length} 个推荐分组` : "当前没有可应用的装备推荐"}</span></div><button class="button button-primary apply-item-set" type="button" data-apply-item-set ${canApply ? "" : "disabled"}>${data?.phase === "ChampSelect" ? "应用装备方案" : "仅英雄选择阶段可应用"}</button></footer>`;
    return `<section class="build-recommendation"><div class="build-essentials"><section><h3>召唤师技能</h3><div class="config-option-list">${spellOptions.map((option) => renderConfigOption(option, "spell")).join("")}</div></section><section class="skill-plan"><div class="skill-plan-head"><h3>技能加点</h3>${renderRecommendationStats(build.skillStats)}</div>${renderSkillPlan(build, abilities)}</section></div><div class="item-option-groups"><section><h3>出门装</h3><div class="config-option-list">${starterOptions.map((option) => renderConfigOption(option, "item")).join("")}</div></section><section><h3>鞋子</h3><div class="config-option-list">${bootOptions.map((option) => renderConfigOption(option, "item")).join("")}</div></section><section class="route-options"><h3>出装路线</h3><div class="config-option-list">${itemRoutes.map((option) => renderConfigOption(option, "route")).join("")}</div></section></div>${action}</section>`;
  }

  function buildItemSetPayload(build, self, data) {
    const starterOptions = build?.starterOptions || (build?.starterItems ? [{ ids: build.starterItems }] : []);
    const bootOptions = build?.bootOptions || (build?.boots ? [{ ids: build.boots }] : []);
    const itemRoutes = build?.itemRoutes || (build?.itemRoute ? [{ ids: build.itemRoute }] : []);
    const items = (ids) => [...new Set((ids || []).map(Number).filter((id) => Number.isInteger(id) && id > 0))].map((id) => ({ id, count: 1 }));
    const blocks = starterOptions.map((option, index) => ({ type: starterOptions.length > 1 ? `出门装 ${index + 1}` : "出门装", items: items(option.ids) })).filter((block) => block.items.length);
    const boots = items(bootOptions.flatMap((option) => option.ids || []));
    if (boots.length) blocks.push({ type: "鞋子选择", items: boots });
    blocks.push(...itemRoutes.map((option, index) => ({ type: itemRoutes.length > 1 ? `出装路线 ${index + 1}` : "出装路线", items: items(option.ids) })).filter((block) => block.items.length));
    const championName = self?.championName || "当前英雄";
    const lane = self?.position && self.position !== "other" ? ` · ${positionLabel(self.position)}` : "";
    return { title: `${championName}${lane} OPGG 推荐`, championId: Number(self?.championId || 0), mapId: Number(data?.mapId || 0), position: self?.position || "other", blocks: blocks.slice(0, 20) };
  }

  function renderSkillPlan(build, abilities) {
    const bySlot = new Map((abilities || []).map((ability) => [String(ability.slot || "").toUpperCase(), ability]));
    const skillPriority = (build.skillPriority || []).map((slot) => String(slot).toUpperCase());
    const priority = ["Q", "W", "E", "R"].map((key) => {
      const ability = bySlot.get(key) || { slot: key, name: key, description: `${key} 技能` };
      const icon = ability.iconPath ? assetIcon(ability.iconPath, ability.name || key, "large") : `<span class="skill-letter">${escapeHTML(key)}</span>`;
      const rank = skillPriority.indexOf(key);
      const label = key === "R" ? "终极技能" : rank === 0 ? "主升" : rank === 1 ? "副升" : "最后";
      const tooltip = abilityTooltip(key, ability);
      return `<button class="skill-icon-button" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${icon}<span>${label}</span></button>`;
    }).join("");
    const order = build.skillOrder || [];
    const orderGrid = order.length ? `<div class="skill-order" style="--skill-count:${Math.max(1, order.length)}" aria-label="技能升级顺序">${order.map((key, index) => `<span class="is-${String(key).toLowerCase()}"><b>${escapeHTML(key)}</b><small>${index + 1}</small></span>`).join("")}</div>` : "";
    return `<div class="skill-priority">${priority}</div>${orderGrid}`;
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

  function renderConfigOption(option, kind) {
    const ids = option.ids || [];
    const icons = ids.map((id, index) => {
      const icon = kind === "spell" ? renderSummonerSpellIcon(id) : renderItemIcon(id);
      return kind === "route" ? `<span class="route-step">${index ? '<span class="route-arrow" aria-hidden="true">›</span>' : ""}${icon}</span>` : icon;
    }).join("");
    return `<div class="config-option"><div class="config-icons">${icons}</div>${renderOptionStats(option.stats || option)}</div>`;
  }

  function renderOptionStats(stats) {
    return `<dl class="option-stats"><div class="is-pick"><dt>选取率</dt><dd>${rate(stats.pickRate ?? stats.pick_rate)}</dd></div><div class="is-win"><dt>胜率</dt><dd>${rate(stats.winRate ?? (stats.win && (stats.play || stats.games) ? stats.win / (stats.play || stats.games) : NaN))}</dd></div></dl>`;
  }

  function renderItemIcon(id) {
    const item = (state.items?.items || []).find((candidate) => Number(candidate.id) === Number(id));
    const name = item?.name || `装备 ${id}`;
    const explanation = plainText(item?.description || name);
    const icon = item?.iconPath ? assetIcon(item.iconPath, name, "large") : iconFigure("item", id, name, "large");
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
    return `<button class="item-option-button" type="button" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-tooltip="${escapeHTML(tooltip)}">${icon}</button>`;
  }

  function renderSummonerSpellIcon(id) {
    const spell = (state.summonerSpells?.spells || []).find((candidate) => Number(candidate.id) === Number(id));
    const name = spell?.name || `召唤师技能 ${id}`;
    const explanation = plainText(spell?.description || name);
    const icon = spell?.iconPath ? assetIcon(spell.iconPath, name, "large") : iconFigure("spell", id, name, "large");
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
    return `<button class="item-option-button" type="button" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-tooltip="${escapeHTML(tooltip)}">${icon}</button>`;
  }

  function selectedRuneRecommendation(data) {
    const runes = liveRecommendationsFor(data)?.runes || {};
    const opggItems = Array.isArray(runes.opgg) ? runes.opgg : runes.opgg ? [runes.opgg] : [];
    const opggFallback = opggItems.length
      ? opggItems.map((item) => ({ ...item, sourceLabel: "OPGG" }))
      : data?.clientRecommendation ? [{ ...data.clientRecommendation, title: "客户端内置（备用）", sourceLabel: "客户端内置" }] : [];
    const normalized = [
      ...opggFallback.map((item, index) => ({ ...item, key: item.key || (index === 0 ? "opgg" : `opgg-${index}`) })),
      ...specialistRunesFor(data).map((item, index) => ({ ...item, sourceLabel: "绝活哥", key: item.key || `specialist-${index}` })),
      ...(runes.pros || []).map((item, index) => ({ ...item, sourceLabel: "职业选手", key: item.key || `pro-${index}` })),
    ];
    return normalized.find((item) => String(item.key) === String(state.selectedRecommendation)) || normalized[0] || null;
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
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) return "—";
    return `${(parsed <= 1 ? parsed * 100 : parsed).toFixed(1).replace(/\.0$/, "")}%`;
  }

  function bindLiveContent() {
    // 对局页里点击玩家名称：在当前页面上以覆盖层打开该玩家的总览，
    // 不再跳转到总览页；对局数据来自本机客户端，必然是国服玩家。
    for (const button of nodes.liveContent.querySelectorAll("[data-player-ref]")) button.addEventListener("click", () => {
      openPlayerOverlay({ playerRef: button.dataset.playerRef, region: "", label: playerButtonLabel(button) });
    });
    for (const button of nodes.liveContent.querySelectorAll("[data-recommendation-tab]")) button.addEventListener("click", () => {
      state.recommendationTab = button.dataset.recommendationTab;
      if (state.recommendationTab === "build") { ensureItems(); ensureSummonerSpells(); }
      renderLive();
      nodes.liveContent.querySelector(`[data-recommendation-tab="${state.recommendationTab}"]`)?.focus();
    });
    nodes.liveContent.querySelector(".recommendation-tabs")?.addEventListener("keydown", (event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const tabs = [...event.currentTarget.querySelectorAll('[role="tab"]')];
      const current = tabs.indexOf(document.activeElement);
      const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
      tabs[next]?.click();
    });
    for (const button of nodes.liveContent.querySelectorAll("[data-rune-choice]")) button.addEventListener("click", () => {
      state.selectedRecommendation = button.dataset.runeChoice;
      renderLive();
    });
    nodes.liveContent.querySelector("[data-apply-runes]")?.addEventListener("click", applyRunes);
    nodes.liveContent.querySelector("[data-apply-item-set]")?.addEventListener("click", applyItemSet);
  }

  async function applyRunes(event) {
    const button = event.currentTarget;
    const recommendation = selectedRuneRecommendation(state.live);
    if (!recommendation) return;
    const self = (state.live?.players || []).find((player) => player.isCurrent);
    if (!String(self?.championName || "").trim() || !recommendation.sourceLabel) {
      showToast("暂时无法识别当前英雄或符文推荐来源");
      return;
    }
    button.disabled = true;
    button.textContent = "正在应用…";
    try {
      await api("/api/gameplay/runes/apply", { method: "POST", body: JSON.stringify({ championName: self.championName, source: recommendation.sourceLabel, championId: recommendation.championId || self.championId, primaryStyleId: recommendation.primaryStyleId, subStyleId: recommendation.subStyleId, selectedPerkIds: recommendation.selectedPerkIds || recommendation.perkIds || [] }) }, "apply-runes", 15000);
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
    if (!payload.championId || !payload.blocks.length) return;
    button.disabled = true;
    button.textContent = "正在应用…";
    try {
      const result = await api("/api/gameplay/item-sets/apply", { method: "POST", body: JSON.stringify(payload) }, "apply-item-set", 20000);
      showToast(`${result.title || "装备方案"} 已写入客户端`);
      button.textContent = "已应用";
    } catch (error) { showToast(error.message); button.textContent = "应用装备方案"; }
    finally { button.disabled = false; }
  }

  function scheduleLiveRefresh() {
    clearTimeout(state.liveTimer);
    if (!state.settings.liveRefresh || state.section !== "live" || document.hidden) return;
    state.liveTimer = setTimeout(() => loadLive(true), state.settings.liveInterval * 1000);
  }

  function bindSettings() {
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
      if (state.section === "overview") loadOverview(activeTab(), true);
    });
    nodes.settingDefaultMatchFilter.addEventListener("change", () => { state.settings.defaultMatchFilter = normalizeMatchFilter(nodes.settingDefaultMatchFilter.value); for (const tab of state.tabs) tab.matchFilter = state.settings.defaultMatchFilter; writeSetting("default-match-filter", state.settings.defaultMatchFilter); renderOverview(); });
    nodes.settingLiveRefresh.addEventListener("change", () => { state.settings.liveRefresh = nodes.settingLiveRefresh.checked; nodes.settingLiveInterval.disabled = !state.settings.liveRefresh; writeSetting("live-refresh", state.settings.liveRefresh); scheduleLiveRefresh(); renderLive(); });
    nodes.settingLiveInterval.disabled = !state.settings.liveRefresh;
    nodes.settingLiveInterval.addEventListener("change", () => { state.settings.liveInterval = normalizeLiveInterval(nodes.settingLiveInterval.value); writeSetting("live-interval", state.settings.liveInterval); scheduleLiveRefresh(); renderLive(); });
    nodes.settingLiveOrder.addEventListener("change", () => { state.settings.liveOrder = normalizeLiveOrder(nodes.settingLiveOrder.value); writeSetting("live-order", state.settings.liveOrder); renderLive(); });
    nodes.settingMaskNames.addEventListener("change", () => { state.settings.maskNames = nodes.settingMaskNames.checked; writeSetting("mask-names", state.settings.maskNames); renderPlayerTabs(); renderOverview(); renderLive(); if (state.overlay.length) renderOverlay(); });
    nodes.settingConfirmReplay.addEventListener("change", () => { state.settings.confirmReplay = nodes.settingConfirmReplay.checked; writeSetting("confirm-replay", state.settings.confirmReplay); });
    bindConvenienceSettings();
  }

  // 便捷设置保存在本机数据目录，由本地服务监听客户端阶段后自动执行；
  // 前端只负责读取/写入开关，不直接调用客户端。
  const convenienceToggles = [nodes.settingAutoAccept, nodes.settingAutoPlayAgain, nodes.settingAutoReconnect];

  function bindConvenienceSettings() {
    if (convenienceToggles.some((node) => !node)) return;
    for (const node of convenienceToggles) node.disabled = true;
    for (const node of convenienceToggles) node.addEventListener("change", saveConvenienceSettings);
    loadConvenienceSettings();
  }

  function conveniencePayload() {
    return {
      autoAccept: Boolean(nodes.settingAutoAccept?.checked),
      autoPlayAgain: Boolean(nodes.settingAutoPlayAgain?.checked),
      autoReconnect: Boolean(nodes.settingAutoReconnect?.checked),
    };
  }

  function applyConveniencePayload(payload) {
    if (nodes.settingAutoAccept) nodes.settingAutoAccept.checked = Boolean(payload?.autoAccept);
    if (nodes.settingAutoPlayAgain) nodes.settingAutoPlayAgain.checked = Boolean(payload?.autoPlayAgain);
    if (nodes.settingAutoReconnect) nodes.settingAutoReconnect.checked = Boolean(payload?.autoReconnect);
  }

  async function loadConvenienceSettings() {
    try {
      applyConveniencePayload(await api("/api/gameplay/convenience", {}, "convenience", 8000));
    } catch (_) {
      applyConveniencePayload({ autoAccept: false, autoPlayAgain: false, autoReconnect: false });
    } finally {
      for (const node of convenienceToggles) if (node) node.disabled = false;
    }
  }

  async function saveConvenienceSettings() {
    const payload = conveniencePayload();
    for (const node of convenienceToggles) if (node) node.disabled = true;
    try {
      applyConveniencePayload(await api("/api/gameplay/convenience", { method: "POST", body: JSON.stringify(payload) }, "convenience-save", 8000));
    } catch (error) {
      showToast(error.message);
      await loadConvenienceSettings();
    } finally {
      for (const node of convenienceToggles) if (node) node.disabled = false;
    }
  }

  function renderCapabilitySettings() {
    const labels = { summoner: "召唤师资料", "ranked-stats": "排位数据", "match-history": "战绩列表", "seven-day-history": "7 天统计样本", "match-details": "对局详情", "champion-mastery": "英雄熟练度", gameflow: "游戏流程", "gameflow-session": "当前对局", "champ-select": "英雄选择", "live-player-analysis": "队伍分析", "client-rune-recommendation": "客户端推荐符文", "champion-abilities": "英雄技能" };
    const capabilities = state.lastCapabilities || [];
    nodes.gameplaySettingsStatus.innerHTML = capabilities.length ? `<div class="gameplay-capabilities">${capabilities.map((item) => `<div><span class="capability-dot is-${escapeHTML(item.state)}" aria-hidden="true"></span><span><strong>${escapeHTML(labels[item.name] || item.name)}</strong><small>${escapeHTML(item.detail || (item.state === "available" ? `已读取 ${number(item.count)} 项` : "当前不可用"))}</small></span><b>${item.state === "available" ? "可用" : item.state === "unsupported" ? "不支持" : "读取失败"}</b></div>`).join("")}</div>` : '<p class="muted">打开总览或对局后显示能力状态。</p>';
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
    if (!id) return "";
    if (kind === "profile") return `/lol-game-data/assets/v1/profile-icons/${id}.jpg`;
    if (kind === "champion") return `/lol-game-data/assets/v1/champion-icons/${id}.png`;
    if (kind === "item") return `/lol-game-data/assets/v1/items/${id}.png`;
    if (kind === "spell") return `/lol-game-data/assets/v1/summoner-spells/${id}.png`;
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
    return `/api/image?path=${encodeURIComponent(path)}`;
  }
  function assetIcon(path, label, size = "") { return `<span class="game-icon${size ? ` is-${size}` : ""}" title="${escapeHTML(label)}"><span aria-hidden="true">${escapeHTML(String(label || "?").slice(0, 1))}</span><img src="${proxyAsset(path)}" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`; }
  function remoteStaticIcon(source, path, label, size = "") { return `<span class="game-icon${size ? ` is-${size}` : ""}" title="${escapeHTML(label)}"><span aria-hidden="true">${escapeHTML(String(label || "?").slice(0, 1))}</span><img src="/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(path)}" alt="${escapeHTML(label)}" loading="lazy" decoding="async" data-game-image></span>`; }
  function iconFigure(kind, id, label, size = "") { return assetIcon(assetPath(kind, id), label || `${kind} ${id || ""}`, size); }
  function maskedProfileIcon(size = "") { return `<span class="game-icon${size ? ` is-${size}` : ""}" title="身份已遮罩"><span aria-hidden="true">◉</span></span>`; }
  // 目录尚未加载时的占位图标：不发起注定 404 的猜测 URL 请求（客户端
  // 的装备/技能/符文图标路径必须来自目录的 iconPath），目录加载完成后
  // rerenderCatalogViews 会自动重绘补上真实图标，避免图标时有时无。
  function pendingCatalogIcon(label, size = "") { return `<span class="game-icon is-pending${size ? ` is-${size}` : ""}" title="${escapeHTML(label)}"><span aria-hidden="true"> </span></span>`; }
  // 装备/召唤师技能/符文图标一律使用目录里的 iconPath 与真实名称；
  // 目录未加载完成时先显示占位，加载完成后自动重绘。
  function itemIconFigure(id, size = "") {
    if (!state.items) return pendingCatalogIcon(`装备 ${id}`, size);
    const item = (state.items.items || []).find((candidate) => Number(candidate.id) === Number(id));
    return item?.iconPath ? assetIcon(item.iconPath, item.name || `装备 ${id}`, size) : iconFigure("item", id, `装备 ${id}`, size);
  }
  function spellIconFigure(id, size = "") {
    if (!Number(id)) return "";
    if (!state.summonerSpells) return pendingCatalogIcon(`召唤师技能 ${id}`, size);
    const spell = (state.summonerSpells.spells || []).find((candidate) => Number(candidate.id) === Number(id));
    return spell?.iconPath ? assetIcon(spell.iconPath, spell.name || `召唤师技能 ${id}`, size) : iconFigure("spell", id, `召唤师技能 ${id}`, size);
  }
  function perkIconFigure(id, size = "") {
    if (!state.perks) return pendingCatalogIcon(`符文 ${id}`, size);
    const catalog = [...(state.perks.perks || []), ...(state.perks.styles || []).flatMap((style) => (style.slots || []).flatMap((slot) => slot.perks || []))];
    const perk = catalog.find((item) => Number(item.id) === Number(id));
    return perk?.iconPath ? assetIcon(perk.iconPath, perk.name || `符文 ${id}`, size) : iconFigure("perk", id, `符文 ${id}`, size);
  }
  function perkStyleIconFigure(styleID, size = "") {
    const style = (state.perks?.styles || []).find((candidate) => Number(candidate.id) === Number(styleID));
    return style?.iconPath ? assetIcon(style.iconPath, `副系 · ${style.name}`, size) : "";
  }
  function augmentIconFigure(id, size = "") {
    const augment = (state.perks?.augments || []).find((candidate) => Number(candidate.id) === Number(id));
    if (!augment) return state.perks ? iconFigure("augment", id, `海克斯 ${id}`, size) : pendingCatalogIcon(`海克斯 ${id}`, size);
    const rarity = ({ kSilver: "白银", kGold: "黄金", kPrismatic: "棱彩" })[augment.rarity] || augment.rarity || "";
    const description = plainText(augment.description || "");
    const tooltip = [augment.name || `海克斯 ${id}`, rarity ? `${rarity}级` : "", description].filter(Boolean).join("\n");
    const icon = augment.iconPath ? assetIcon(augment.iconPath, augment.name || `海克斯 ${id}`, size) : iconFigure("augment", id, augment.name || `海克斯 ${id}`, size);
    return `<span class="arena-augment-icon" tabindex="0" data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}">${icon}</span>`;
  }
  function renderItemIcons(items, size = "") { const valid = items.filter((id) => Number(id) > 0); return valid.map((id) => itemIconFigure(id, size)).join(""); }
  function prepareImages(container) {
    for (const image of container.querySelectorAll("[data-game-image]")) {
      const loaded = () => image.parentElement?.classList.add("has-loaded-image");
      // 失败后延迟重试一次：客户端在对局中偶发拒绝资源请求，直接放弃
      // 会让图标“时有时无”；两次都失败才回落到字母占位。
      const failed = () => {
        if (!image.dataset.retried) {
          image.dataset.retried = "1";
          const source = image.src;
          setTimeout(() => {
            if (!image.isConnected) return;
            image.addEventListener("load", loaded, { once: true });
            image.addEventListener("error", () => { image.hidden = true; image.parentElement?.classList.remove("has-loaded-image"); }, { once: true });
            image.src = "";
            image.src = source;
          }, 1200);
          return;
        }
        image.hidden = true;
        image.parentElement?.classList.remove("has-loaded-image");
      };
      if (image.complete) image.naturalWidth > 0 ? loaded() : failed();
      else {
        image.addEventListener("load", loaded, { once: true });
        image.addEventListener("error", failed, { once: true });
      }
    }
  }

  function showToast(message) {
    nodes.toast.textContent = message;
    nodes.toast.hidden = false;
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => { nodes.toast.hidden = true; }, 3600);
  }

  nodes.playerTabs.addEventListener("click", (event) => {
    const close = event.target.closest("[data-close-player]");
    if (close) { closePlayerTab(close.dataset.closePlayer); return; }
    const tab = event.target.closest("[data-player-tab]");
    if (tab) selectPlayerTab(tab.dataset.playerTab);
  });
  nodes.playerTabs.addEventListener("keydown", (event) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const buttons = [...nodes.playerTabs.querySelectorAll("[data-player-tab]")];
    if (!buttons.length) return;
    event.preventDefault();
    const current = buttons.indexOf(document.activeElement);
    const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : (current + (event.key === "ArrowRight" ? 1 : -1) + buttons.length) % buttons.length;
    buttons[next].focus(); buttons[next].click();
  });
  nodes.overviewRefresh.addEventListener("click", () => loadOverview(activeTab(), true));
  nodes.liveRefresh.addEventListener("click", () => loadLive(true));
  window.addEventListener("deep-legends:status", (event) => updateStatus(event.detail || {}));
  window.addEventListener("deep-legends:section", (event) => activateSection(event.detail?.name || "overview"));
  window.addEventListener("deep-legends:open-player", (event) => {
    const gameName = String(event.detail?.gameName || "").trim();
    if (!gameName) return;
    const tagLine = String(event.detail?.tagLine || "").trim();
    const region = String(event.detail?.region || "").trim();
    const serverId = String(event.detail?.serverId || "").trim().toUpperCase();
    const source = String(event.detail?.source || "champions");
    if (source === "search") {
      // 顶部搜索：在总览页新开一个玩家页签。
      window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: "overview" } }));
      openPlayerByRiotId(gameName, tagLine, region, serverId);
      return;
    }
    // 英雄详情页等非总览入口：留在当前页面，用覆盖层展示总览。
    openPlayerOverlay({ riotId: { gameName, tagLine }, region, serverId, label: `${gameName}${tagLine ? `#${tagLine}` : ""}` });
  });
  nodes.playerOverlayBack?.addEventListener("click", overlayBack);
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && state.overlay.length && !document.querySelector("dialog[open]")) { event.preventDefault(); overlayBack(); }
  });
  document.addEventListener("visibilitychange", () => { if (document.hidden) clearTimeout(state.liveTimer); else if (state.section === "live") loadLive(true); });

  /* ---------- 新对局提示灯：客户端进入英雄选择/对局时点亮“对局”页签 ---------- */
  const beaconPhases = new Set(["ChampSelect", "GameStart", "InProgress", "Reconnect"]);
  function updateBeacon(phase) {
    const active = beaconPhases.has(phase);
    if (active && !state.beacon.active) {
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
      dot.title = "检测到新的对局，点击查看";
      dot.setAttribute("aria-hidden", "true");
      tabButton.append(dot);
    } else if (!show && dot) {
      dot.remove();
    }
  }
  window.addEventListener("deep-legends:gameflow", (event) => updateBeacon(String(event.detail?.phase || "")));
  // 阶段轮询兜底：SSE 偶发漏事件会让呼吸灯不亮，因此不再只在事件流
  // 断开时轮询——只要页面可见且客户端已连接，就每 15 秒核对一次阶段。
  setInterval(() => {
    if (document.hidden || !connected()) return;
    api("/api/gameplay/phase", {}, "gameflow-phase", 8000).then((payload) => updateBeacon(String(payload?.phase || ""))).catch(() => {});
  }, 15000);

  bindSettings();
  renderPlayerTabs();
  renderOverview();
  renderLive();
  renderCapabilitySettings();
  requestAnimationFrame(() => {
    const defaultSection = ["overview", "champions", "live", "favorites"].includes(state.settings.defaultPage) ? state.settings.defaultPage : "overview";
    if (defaultSection !== "overview") document.querySelector(`[data-section="${defaultSection}"]`)?.click();
  });
})();
