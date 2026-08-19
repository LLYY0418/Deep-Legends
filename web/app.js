(() => {
  "use strict";

  const STATUS_INTERVAL = 60 * 60 * 1000;
  const CN_SERVER_MERGE_NOTE = [
    "联盟一区：祖安、皮尔特沃夫、巨神峰、教育网、男爵领域、均衡教派、影流、守望之海",
    "联盟二区：卡拉曼达、暗影岛、征服之海、诺克萨斯、战争学院、雷瑟守备",
    "联盟三区：班德尔城、裁决之地、水晶之痕、钢铁烈阳、皮城警备",
    "联盟四区：比尔吉沃特、弗雷尔卓德、扭曲丛林",
    "联盟五区：德玛西亚、无畏先锋、恕瑞玛、巨龙之巢",
    "艾欧尼亚、黑色玫瑰、峡谷之巅独立运营，未参与合并。",
  ].join("\n");

  const state = {
    status: null,
    section: "overview",
    favoritesPage: "collection",
    view: "owned",
    items: [],
    query: "",
    qualitySelections: new Set(),
    sort: "acquired",
    descending: true,
    loading: true,
    listError: "",
    controllers: new Map(),
    skinLoadGeneration: 0,
    renderGeneration: 0,
    poolRenderGeneration: 0,
    statusDelay: STATUS_INTERVAL,
    statusTimer: 0,
    diagnostics: null,
    account: null,
    accountLoaded: false,
    history: [],
    historyLoaded: false,
    pools: [],
    poolsLoaded: false,
    poolItems: [],
    poolView: "owned",
    poolQuery: "",
    poolQuality: "all",
    poolSort: "rarity",
    poolLoading: false,
    poolError: "",
    selectedSkin: null,
    detailItems: [],
    detailKind: "skin",
    destroyed: false,
    eventSource: null,
    liveUpdateTimer: 0,
    detailGeneration: 0,
    detailMediaTimer: 0,
    skinDetailCache: new Map(),
    skinDetailPromises: new Map(),
    artworkPrefetchCache: new Map(),
    acquisitionAvailable: null,
    acquisitionFallback: false,
    installations: [],
    installationsLoaded: false,
    themeTimer: 0,
    showUnownedChromas: true,
    showPrestigeChromas: true,
    chromaCapability: null,
    chromaRenderedItems: [],
    startupFallbackTimer: 0,
    overlayForced: false,
    overlayBaselineAttempt: "",
    overlayTimer: 0,
    overlaySuppressed: false,
    renderFrames: new Set(),
    poolRenderFrames: new Set(),
    cardImageObserver: null,
    cardImageJobs: new WeakMap(),
    cardImageQueue: [],
    activeCardImages: 0,
    activePrestigeCardImages: 0,
    fullscreenExitInProgress: false,
    detailArtworkMode: "ordinary",
    settingsPage: "appearance",
  };

  const el = Object.fromEntries([
    "connection", "connection-avatar", "refresh", "quit", "owned-count", "chroma-count", "pool-count", "remaining-count", "notice",
    "search", "rarity-button", "rarity-menu", "sort", "sort-button", "sort-label", "sort-menu", "sort-direction", "list-meta", "retry-list", "skin-grid", "skin-card-template",
    "pool-source", "setting-theme", "density-toggle", "account-content", "account-live-state", "diagnostics-content", "copy-diagnostics", "history-content",
    "player-search-region", "player-search-region-label", "player-search-region-menu", "player-search-cn-toggle", "player-search-cn-info", "player-search-cn-options",
    "player-search-follow-client", "player-search-follow-status", "player-search-name", "player-search-tag", "player-search-go", "player-search-clear",
    "refresh-history", "pools-content", "pool-import", "pool-name", "pool-version", "pool-source-input", "pool-file", "pool-import-status",
    "privacy-content", "skin-dialog", "skin-dialog-close", "skin-dialog-image", "skin-dialog-fallback", "skin-dialog-status",
    "skin-dialog-title", "skin-dialog-hero", "skin-dialog-data", "skin-dialog-video", "copy-skin-id", "toast", "client-launchpad", "launcher-list",
    "sidebar-toggle", "settings-sidebar-toggle", "current-section-title", "topbar-subtitle", "page-intro",
    "chroma-unowned-control", "show-unowned-chromas", "chroma-prestige-control", "show-prestige-chromas",
    "startup-loading", "startup-loading-title", "startup-loading-copy", "app-frame",
    "pool-catalog-panel", "pool-upload-panel", "pool-history-panel", "pool-picker", "pool-search", "pool-quality", "pool-sort", "pool-list-meta", "pool-skin-grid",
    "favorites-collection-panel", "favorites-account-panel", "favorites-pools-panel",
    "skin-dialog-art", "skin-dialog-backdrop", "skin-dialog-artwork", "skin-dialog-fullscreen", "skin-dialog-previous", "skin-dialog-next", "app-main", "app-scroll", "back-to-top",
    "setting-proxy-mode", "setting-proxy-url", "setting-proxy-url-wrap", "setting-proxy-save", "setting-proxy-state",
  ].map((id) => [camel(id), document.getElementById(id)]));

  el.grid = el.skinGrid;
  el.template = el.skinCardTemplate;
  el.showUnownedChromas.checked = state.showUnownedChromas;
  el.showPrestigeChromas.checked = state.showPrestigeChromas;

  el.sectionTabs = [...document.querySelectorAll("[data-section]")];
  el.sectionPanels = [...document.querySelectorAll("main > [role='tabpanel']")];
  el.viewTabs = [...document.querySelectorAll("[data-view]")];
  el.favoritesTabs = [...document.querySelectorAll("[data-favorites-page]")];
  el.poolPageTabs = [...document.querySelectorAll("[data-pool-page]")];
  el.poolViewTabs = [...document.querySelectorAll("[data-pool-view]")];
  el.settingsTabs = [...document.querySelectorAll("[data-settings-page]")];
  el.settingsPanels = [...document.querySelectorAll(".settings-subpanel")];

  function camel(value) { return value.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase()); }
  function preference(key, fallback) { try { return localStorage.getItem(`lol-loot-${key}`) ?? fallback; } catch (_) { return fallback; } }
  function savePreference(key, value) { try { localStorage.setItem(`lol-loot-${key}`, String(value)); } catch (_) {} }

  async function api(path, options = {}, requestKey = path, timeout = 10000) {
    const previous = state.controllers.get(requestKey);
    if (previous) previous.abort();
    const controller = new AbortController();
    state.controllers.set(requestKey, controller);
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, timeout);
    try {
      const headers = new Headers(options.headers || {});
      headers.set("Accept", "application/json");
      if (options.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
      const response = await fetch(path, { ...options, headers, signal: controller.signal });
      if (!response.ok) {
        if (response.status === 401) throw new Error("页面会话已过期，刷新页面即可重新连接");
        const message = (await response.text()).trim();
        throw new Error(message || `本地服务返回 HTTP ${response.status}`);
      }
      if (response.status === 202 || response.status === 204) return null;
      return response.json();
    } catch (error) {
      if (error.name === "AbortError") {
        if (timedOut) throw new Error("本地请求超时，请重试");
        const cancelled = new Error("请求已取消");
        cancelled.name = "RequestCancelled";
        throw cancelled;
      }
      throw error;
    } finally {
      clearTimeout(timer);
      if (state.controllers.get(requestKey) === controller) state.controllers.delete(requestKey);
    }
  }

  function scheduleStatus() {
    clearTimeout(state.statusTimer);
    if (document.hidden || state.destroyed) return;
    state.statusTimer = setTimeout(() => refreshStatus(false), state.statusDelay);
  }

  async function refreshStatus(loadItems = false) {
    try {
      const previous = state.status;
      state.status = await api("/api/status", {}, "status", 8000);
      state.statusDelay = STATUS_INTERVAL;
      if (!previous?.connected && state.status.connected) state.overlaySuppressed = false;
      const changed = !previous || previous.lastSync !== state.status.lastSync || previous.lastAttempt !== state.status.lastAttempt || previous.calculationOK !== state.status.calculationOK || previous.poolId !== state.status.poolId || previous.connected !== state.status.connected || previous.snapshotReady !== state.status.snapshotReady;
      if (changed) {
        if (!(state.section === "favorites" && state.favoritesPage === "account")) state.accountLoaded = false;
        if (!(state.section === "favorites" && state.favoritesPage === "pools")) {
          state.historyLoaded = false;
          state.poolsLoaded = false;
        }
      }
      updateReadingOverlay(loadItems || changed);
      renderStatus();
      if (!state.status.connected) await loadClientInstallations();
      if (loadItems || changed) { state.skinsCache?.clear(); await loadSkins(true); }
      if (state.section === "favorites" && state.favoritesPage === "account" && (loadItems || changed)) await loadAccount();
      if (state.section === "favorites" && state.favoritesPage === "pools" && (loadItems || changed)) await loadPools();
      updateReadingOverlay();
    } catch (error) {
      if (error.name === "RequestCancelled" || state.destroyed) return;
      state.statusDelay = STATUS_INTERVAL;
      showFatal(error.message);
    } finally {
      scheduleStatus();
    }
  }

  function updateReadingOverlay(willLoadItems = false) {
    if (!state.status) return;
    const data = state.status;
    const attemptChanged = String(data.lastAttempt || "") !== state.overlayBaselineAttempt;
    if (data.connected && data.snapshotReady && !state.loading && !willLoadItems && (!state.overlayForced || attemptChanged)) {
      state.overlaySuppressed = false;
      hideReadingOverlay();
      return;
    }
    if (state.overlayForced && !attemptChanged) {
      state.statusDelay = 450;
      return;
    }
    // 快照已就绪后的后台皮肤重载（lastSync 变化触发）只在收藏页需要
    // 遮罩；停留在总览/对局等页面时静默完成，避免周期性闪烁。
    const preparing = data.syncing && !data.snapshotReady
      || (data.connected && !data.snapshotReady)
      || (data.connected && data.snapshotReady && state.section === "favorites" && (state.loading || willLoadItems));
    if (!preparing) {
      state.overlaySuppressed = false;
      hideReadingOverlay();
      return;
    }
    if (state.overlaySuppressed && !state.overlayForced) {
      hideReadingOverlay();
      state.statusDelay = 900;
      return;
    }
    showReadingOverlay(data.connected ? "正在读取收藏信息" : "正在连接英雄联盟客户端", data.connected ? "正在整理皮肤、炫彩与账户物品；失败时会自动重试…" : "检测到客户端正在启动，请稍候。", state.overlayForced);
    state.statusDelay = 900;
  }

  function showReadingOverlay(title, copy, forced = false) {
    clearTimeout(state.overlayTimer);
    if (forced) state.overlaySuppressed = false;
    if (forced && !state.overlayForced) state.overlayBaselineAttempt = String(state.status?.lastAttempt || "");
    state.overlayForced ||= forced;
    el.startupLoadingTitle.textContent = title;
    el.startupLoadingCopy.textContent = copy;
    el.startupLoading.hidden = false;
    el.startupLoading.classList.remove("is-leaving");
    el.appFrame.setAttribute("inert", "");
    if (!state.startupFallbackTimer) {
      state.startupFallbackTimer = setTimeout(() => {
        state.startupFallbackTimer = 0;
        state.overlaySuppressed = true;
        state.overlayForced = false;
        hideReadingOverlay();
      }, 25000);
    }
  }

  function hideReadingOverlay() {
    state.overlayForced = false;
    state.overlayBaselineAttempt = "";
    clearTimeout(state.startupFallbackTimer);
    state.startupFallbackTimer = 0;
    if (el.startupLoading.hidden) {
      el.appFrame.removeAttribute("inert");
      return;
    }
    el.appFrame.removeAttribute("inert");
    el.startupLoading.classList.add("is-leaving");
    clearTimeout(state.overlayTimer);
    state.overlayTimer = setTimeout(() => { el.startupLoading.hidden = true; }, 180);
  }

  function applySkinsPayload(items, capability) {
    state.items = items;
    state.chromaCapability = state.view === "chromas" ? capability || null : state.chromaCapability;
    state.acquisitionAvailable = state.items.some((skin) => acquisitionTime(skin) !== null);
    state.acquisitionFallback = state.sort === "acquired" && !state.acquisitionAvailable;
    if (state.acquisitionFallback) state.sort = state.view === "all" ? "mastery" : "name";
    configureSortControls();
  }

  async function loadSkins(force = false) {
    const generation = ++state.skinLoadGeneration;
    state.listError = "";
    el.retryList.hidden = true;
    if (!state.status?.connected || !state.status?.snapshotReady) {
      state.items = [];
      state.loading = false;
      state.listError = state.status?.connected && !state.status?.snapshotReady ? "客户端已连接，正在自动重试读取收藏" : "";
      renderItems();
      return;
    }
    state.skinsCache ||= new Map();
    const cached = force ? null : state.skinsCache.get(state.view);
    if (cached && Date.now() - cached.at < 10 * 60 * 1000) {
      applySkinsPayload(cached.items, cached.capability);
      state.loading = false;
      renderItems();
      return;
    }
    state.loading = true;
    renderItems();
    try {
      const endpoint = state.view === "chromas" ? "/api/chromas" : `/api/skins?view=${encodeURIComponent(state.view)}`;
      const payload = await api(endpoint, {}, "skins", 15000);
      if (generation !== state.skinLoadGeneration || state.destroyed) return;
      const items = Array.isArray(payload.items) ? payload.items : [];
      applySkinsPayload(items, payload.capability || null);
      state.skinsCache.set(state.view, { items, capability: payload.capability || null, at: Date.now() });
    } catch (error) {
      if (generation !== state.skinLoadGeneration || error.name === "RequestCancelled" || state.destroyed) return;
      state.items = [];
      state.listError = error.message;
      el.retryList.hidden = false;
    } finally {
      if (generation !== state.skinLoadGeneration || state.destroyed) return;
      state.loading = false;
      renderItems();
    }
  }

  function renderStatus() {
    const data = state.status;
    document.body.classList.remove("is-fatal");
    el.refresh.disabled = data.syncing;
    el.refresh.classList.toggle("is-loading", data.syncing);
    el.connection.className = "connection";
    const summoner = data.summoner || {};
    const name = summoner.gameName || summoner.displayName || "当前召唤师";
    const tag = summoner.tagLine ? `#${summoner.tagLine}` : "";
    if (data.connected && summoner.profileIconId) {
      el.connectionAvatar.hidden = false;
      el.connectionAvatar.src = `/api/image?path=${encodeURIComponent(`/lol-game-data/assets/v1/profile-icons/${summoner.profileIconId}.jpg`)}`;
      el.connectionAvatar.onerror = () => { el.connectionAvatar.hidden = true; };
    } else {
      el.connectionAvatar.hidden = true;
      el.connectionAvatar.removeAttribute("src");
    }
    if (data.connected && data.snapshotReady) {
      el.connection.classList.add("is-connected");
      el.connection.lastElementChild.textContent = `${name}${tag}`;
      el.connection.title = `${name}${tag} · 等级 ${summoner.summonerLevel || "—"}${data.syncing ? " · 正在同步" : data.eventStream ? " · 实时连接" : ""}`;
    } else if (data.connected) {
      el.connection.classList.add("is-connecting");
      el.connection.lastElementChild.textContent = `${name}${tag}`;
      el.connection.title = `${name}${tag} · 正在读取客户端数据`;
    } else {
      el.connection.classList.add(data.connectionState === "connecting" ? "is-connecting" : "is-error");
      el.connection.lastElementChild.textContent = data.syncing || data.connectionState === "connecting" ? "正在检查客户端" : "尚未连接";
      el.connection.title = el.connection.lastElementChild.textContent;
    }
    el.ownedCount.textContent = data.snapshotReady ? formatNumber(data.ownedCount) : "—";
    el.chromaCount.textContent = data.snapshotReady ? formatNumber(data.chromaOwnedCount || 0) : "—";
    el.poolCount.textContent = data.poolTotal ? formatNumber(data.poolTotal) : "—";
    el.remainingCount.textContent = data.calculationOK ? formatNumber(data.remainingCount) : "—";
    if (safeHTTPURL(data.poolSource)) el.poolSource.href = data.poolSource; else el.poolSource.removeAttribute("href");
    el.poolSource.textContent = `${data.poolVersion || "当前"} 奖池清单`;
    renderNotice(data);
    renderLaunchpad(data);
    updateWorkspaceAvailability(data);
    window.dispatchEvent(new CustomEvent("deep-legends:status", { detail: data }));
  }

  // 未连接客户端时，收藏页与奖池页隐藏全部筛选控件，只保留居中的提示卡。
  const workspaceControls = {
    collectionToolbar: document.querySelector(".collection-toolbar"),
    listRow: document.querySelector(".list-row"),
    accountHeading: document.querySelector("#favorites-account-panel .page-heading"),
    poolHeading: document.querySelector("#favorites-pools-panel > .page-heading"),
    poolTabs: document.querySelector(".pool-primary-tabs"),
    poolToolbar: document.querySelector(".pool-catalog-toolbar"),
    poolFilters: document.querySelector(".pool-filters"),
    poolFooter: document.querySelector(".pool-footer"),
  };

  function updateWorkspaceAvailability(data) {
    const waiting = !data.connected;
    for (const element of Object.values(workspaceControls)) if (element) element.hidden = waiting;
    if (el.poolListMeta) el.poolListMeta.hidden = waiting;
    if (waiting) {
      // 断开时收起清单上传 / 本地历史，只保留奖池目录里的提示卡。
      const catalogTab = el.poolPageTabs.find((tab) => tab.dataset.poolPage === "catalog");
      if (catalogTab && !catalogTab.classList.contains("is-active")) catalogTab.click();
    }
  }

  function renderNotice(data) {
    if (state.section !== "favorites" || state.favoritesPage !== "collection") {
      el.notice.hidden = true;
      return;
    }
    el.notice.hidden = false;
    el.notice.className = "notice";
    if (!data.connected) {
      el.notice.hidden = true;
      return;
    }
    if (!data.calculationOK) {
      el.notice.classList.add("is-warning");
      const issues = data.poolIssues || [];
      const issueRows = issues.slice(0, 100).map((item) => `<li>${escapeHTML(item.name)}：${escapeHTML(item.reason)}</li>`).join("");
	  const detail = !data.snapshotReady ? "正在读取收藏信息；如果客户端刚启动，Deep Legends 会自动重试。" : `奖池共 ${formatNumber(data.poolTotal)} 款，已经确认 ${formatNumber(data.poolMatched)} 款；数据完整后会自动显示结果。`;
	  el.notice.innerHTML = `<div class="notice-symbol" aria-hidden="true">!</div><div><strong>${data.connected ? "部分收藏信息暂时不可用" : "奖池结果暂不可用"}</strong><p>${escapeHTML(detail)}</p>${issues.length ? `<details><summary>查看未识别条目</summary><ul class="issue-list">${issueRows}</ul></details>` : ""}</div>`;
      return;
    }
    el.notice.classList.add("is-success");
    el.notice.innerHTML = '<div class="notice-symbol" aria-hidden="true">✓</div><div><strong>收藏与奖池已更新</strong></div>';
  }

  async function loadClientInstallations() {
    try {
      const payload = await api("/api/client-installations", {}, "client-installations", 8000);
      state.installations = Array.isArray(payload.items) ? payload.items : [];
      state.installationsLoaded = true;
      renderLaunchpad(state.status || {});
    } catch (error) {
      if (error.name === "RequestCancelled") return;
      state.installations = [];
      state.installationsLoaded = true;
      renderLaunchpad(state.status || {});
    }
  }

  function renderLaunchpad(data) {
    // 启动入口卡只在总览页“当前召唤师”页签展示；搜索出的玩家页签（无论
    // 是否查到结果）都不展示，避免干扰查看他人战绩。
    const visible = !data.connected && state.section === "overview" && state.overviewTabIsCurrent !== false;
    el.clientLaunchpad.hidden = !visible;
    if (!visible) return;
    if (!state.installationsLoaded) {
      el.launcherList.innerHTML = '<span class="muted">正在检查 TCLS、WeGame 与 Riot 客户端…</span>';
      return;
    }
    if (!state.installations.length) {
      el.launcherList.innerHTML = '<div class="empty-state compact"><strong>没有检测到可启动入口</strong><p>请先从桌面启动 WeGame 或英雄联盟。助手会继续在后台等待连接。</p><button class="text-button scan-launchers" type="button">重新检查安装位置</button></div>';
      el.launcherList.querySelector(".scan-launchers")?.addEventListener("click", loadClientInstallations);
      return;
    }
    el.launcherList.innerHTML = state.installations.map((item) => `<button class="launcher-card" type="button" data-client-id="${escapeHTML(item.id)}"><span class="launcher-kind">${escapeHTML(item.kind === "riot" ? "R" : item.kind === "tcls" ? "L" : "W")}</span><span class="launcher-card-copy"><strong>${escapeHTML(item.name)}</strong><small title="${escapeHTML(item.location || item.description)}">${escapeHTML(item.location || item.description)}</small></span><span class="launcher-arrow" aria-hidden="true">›</span></button>`).join("");
    for (const button of el.launcherList.querySelectorAll("[data-client-id]")) button.addEventListener("click", () => launchDetectedClient(button));
  }

  async function launchDetectedClient(button) {
    for (const candidate of el.launcherList.querySelectorAll("button")) candidate.disabled = true;
    try {
      await api("/api/client-launch", { method: "POST", body: JSON.stringify({ id: button.dataset.clientId }) }, "client-launch", 8000);
      showToast("客户端已启动，登录并进入大厅后会自动连接");
      setTimeout(() => refreshStatus(false), 3000);
    } catch (error) {
      showToast(error.message);
    } finally {
      for (const candidate of el.launcherList.querySelectorAll("button")) candidate.disabled = false;
    }
  }

  function visibleItems() {
    const query = state.query.trim().toLocaleLowerCase("zh-CN");
    const items = state.items.filter((skin) => {
      const haystack = `${skin.name} ${skin.championName || ""} ${skin.poolName || ""} ${skin.id}`.toLocaleLowerCase("zh-CN");
      return (!query || haystack.includes(query)) && qualitySelectionMatches(skin);
    });
    if (state.view === "chromas") return sortChromaItems(items);
    const qualityGrouping = state.qualitySelections.size > 0 || state.sort === "rarity";
    if (state.view === "all" && !qualityGrouping) return sortCatalogItems(items);
    const direction = state.descending ? -1 : 1;
    items.sort((left, right) => {
      if (state.view === "all" && qualityGrouping) {
        const quality = compareRarity(left, right);
        if (quality) return quality * (state.sort === "rarity" ? (state.descending ? 1 : -1) : 1);
        if (Boolean(left.owned) !== Boolean(right.owned)) return left.owned ? -1 : 1;
        if (left.owned) {
          const acquired = compareAcquisition(left, right, true);
          if (acquired) return acquired;
        }
        const release = releaseTime(left) - releaseTime(right);
        return (state.sort === "rarity" ? release * (state.descending ? 1 : -1) : -release) || localeCompare(left.name, right.name) || Number(left.id) - Number(right.id);
      }
      let result = 0;
      if (state.sort === "acquired") return compareAcquisition(left, right, state.descending);
      else if (state.sort === "name") result = localeCompare(left.name, right.name);
      else if (state.sort === "rarity") result = compareRarity(left, right) || (releaseTime(left) - releaseTime(right)) || Number(left.id) - Number(right.id) || localeCompare(left.name, right.name);
      else if (state.sort === "id") result = Number(left.id) - Number(right.id);
      else result = localeCompare(left.championName || "", right.championName || "") || localeCompare(left.name, right.name);
      return result * (state.sort === "rarity" ? (state.descending ? 1 : -1) : direction);
    });
    return items;
  }

  function sortCatalogItems(items) {
    const groups = new Map();
    for (const skin of items) {
      const key = String(skin.championId || skin.championName || "unknown");
      if (!groups.has(key)) groups.set(key, { name: skin.championName || `英雄 ID ${skin.championId || "—"}`, points: Number(skin.championMasteryPoints || 0), level: Number(skin.championMasteryLevel || 0), newest: 0, skins: [] });
      const group = groups.get(key);
      group.skins.push(skin);
      const acquired = acquisitionTime(skin);
      if (acquired !== null) group.newest = Math.max(group.newest, acquired);
    }
    const ordered = [...groups.values()];
    ordered.sort((left, right) => {
      if (state.sort === "champion") return localeCompare(left.name, right.name);
      if (state.sort === "skinCount") return right.skins.length - left.skins.length || localeCompare(left.name, right.name);
      if (state.sort === "acquired") {
        if (!left.newest && right.newest) return 1;
        if (left.newest && !right.newest) return -1;
        const chronological = state.descending ? right.newest - left.newest : left.newest - right.newest;
        return chronological || localeCompare(left.name, right.name);
      }
      return right.points - left.points || localeCompare(left.name, right.name);
    });
    const output = [];
    for (const group of ordered) {
      group.skins.sort((left, right) => {
        if (Boolean(left.owned) !== Boolean(right.owned)) return left.owned ? -1 : 1;
        if (state.sort === "acquired" && left.owned) {
          return compareAcquisition(left, right, state.descending);
        }
		const quality = compareRarity(left, right);
		if (quality) return quality;
		if (left.owned) {
			const acquired = compareAcquisition(left, right, true);
			if (acquired) return acquired;
		}
		return localeCompare(left.name, right.name) || Number(left.id) - Number(right.id);
      });
      output.push(...group.skins);
    }
    return output;
  }

  function sortChromaItems(items) {
    const siblingCounts = new Map();
    for (const chroma of items) {
      const parentID = Number(chroma.parentSkinId || chroma.id);
      siblingCounts.set(parentID, (siblingCounts.get(parentID) || 0) + 1);
    }
    return items.sort((left, right) => {
      if (Boolean(left.isPrestige) !== Boolean(right.isPrestige)) return left.isPrestige ? -1 : 1;
      if (Boolean(left.owned) !== Boolean(right.owned)) return left.owned ? -1 : 1;
      let result = 0;
      if (state.sort === "acquired") result = compareAcquisition(left, right, state.descending);
      else if (state.sort === "rarity") result = compareRarity(left, right);
      else if (state.sort === "chromaCount") result = (siblingCounts.get(Number(right.parentSkinId || right.id)) || 0) - (siblingCounts.get(Number(left.parentSkinId || left.id)) || 0);
      else result = Number(right.championMasteryPoints || 0) - Number(left.championMasteryPoints || 0);
      if (result) return state.sort === "acquired" ? result : result * (state.descending ? 1 : -1);
      return localeCompare(left.championName || "", right.championName || "") || Number(left.parentSkinId || 0) - Number(right.parentSkinId || 0) || Number(left.id) - Number(right.id);
    });
  }

  function acquisitionTime(skin) {
    const value = Date.parse(skin?.acquiredAt || "");
    return Number.isFinite(value) ? value : null;
  }

  function compareAcquisition(left, right, descending) {
    const leftTime = acquisitionTime(left);
    const rightTime = acquisitionTime(right);
    if (leftTime === null && rightTime === null) return localeCompare(left.name, right.name) || Number(left.id) - Number(right.id);
    if (leftTime === null) return 1;
    if (rightTime === null) return -1;
    const chronological = descending ? rightTime - leftTime : leftTime - rightTime;
    return chronological || localeCompare(left.name, right.name) || Number(left.id) - Number(right.id);
  }

  function renderItems() {
    cancelRenderFrames();
    cancelDeferredImages(el.grid);
    const generation = ++state.renderGeneration;
    el.grid.classList.remove("is-sparse");
    el.grid.style.removeProperty("--sparse-columns");
    el.grid.style.removeProperty("--sparse-max-width");
    el.grid.setAttribute("aria-busy", String(state.loading));
    el.grid.classList.toggle("hide-unowned", state.view === "chromas" && !state.showUnownedChromas);
    el.grid.classList.toggle("hide-prestige", state.view === "chromas" && !state.showPrestigeChromas);
    const qualityGrouping = state.qualitySelections.size > 0 || state.sort === "rarity";
    el.grid.classList.toggle("is-grouped", (state.view === "all" || state.view === "chromas" || qualityGrouping) && !state.loading && !state.listError);
    if (state.loading) {
      el.grid.innerHTML = Array.from({ length: 8 }, () => '<div class="skeleton"></div>').join("");
      el.listMeta.textContent = "正在整理皮肤…";
      return;
    }
	if (state.status?.connected && !state.status?.snapshotReady) {
	  el.listMeta.textContent = "正在读取收藏";
	  el.grid.innerHTML = '<div class="empty-state"><strong>客户端已经连接</strong><p>收藏信息还在准备中，读取失败时会自动重试。</p><div class="empty-actions"><button class="text-button refresh-inline" type="button">立即重新读取</button></div></div>';
	  el.grid.querySelector(".refresh-inline")?.addEventListener("click", () => el.refresh.click());
	  return;
	}
    if (state.listError) {
      el.listMeta.textContent = "读取列表失败";
      el.grid.innerHTML = `<div class="empty-state"><strong>列表读取失败</strong><p>${escapeHTML(state.listError)}</p><button class="text-button retry-inline" type="button">重新读取列表</button></div>`;
      el.grid.querySelector(".retry-inline")?.addEventListener("click", () => loadSkins(true));
      return;
    }
    const visible = visibleItems();
    state.chromaRenderedItems = state.view === "chromas" ? visible : [];
    const label = { owned: "已拥有", all: "全部皮肤", chromas: "全部炫彩" }[state.view];
    const heroCount = ["all", "chromas"].includes(state.view) ? new Set(visible.map((skin) => skin.championId || skin.championName)).size : 0;
    const acquisitionHint = state.acquisitionFallback ? " · 客户端未提供获取时间，已改按名称排列" : state.view === "all" && state.sort === "acquired" ? " · 英雄按最近获得的皮肤排列" : "";
    el.listMeta.textContent = `${label} ${formatNumber(visible.length)} 款${heroCount ? ` · ${formatNumber(heroCount)} 位英雄` : ""}${visible.length !== state.items.length ? ` · 共 ${formatNumber(state.items.length)} 款` : ""}${acquisitionHint}`;
    el.grid.replaceChildren();
    if (!visible.length) {
      if (!state.status?.connected) {
        el.listMeta.textContent = "";
        el.grid.innerHTML = '<div class="gameplay-empty"><span aria-hidden="true">◆</span><strong>等待英雄联盟客户端</strong><p>登录国服客户端并进入大厅后，这里会自动展示皮肤与炫彩收藏。</p></div>';
        return;
      }
      const message = state.view === "chromas" && !state.showUnownedChromas ? "当前没有已拥有的炫彩；可打开“显示未获取”查看完整目录。" : state.view === "chromas" && !state.showPrestigeChromas ? "当前没有符合条件的普通炫彩；可打开“显示臻彩”查看臻彩目录。" : "没有符合当前筛选条件的皮肤。";
      el.grid.innerHTML = `<div class="empty-state"><strong>暂时没有结果</strong><p>${escapeHTML(message)}</p></div>`;
      return;
    }
    if (state.view === "chromas") {
      renderChromaCatalog(visible, generation);
      applyChromaVisibility();
      return;
    }
    if (qualityGrouping) {
      renderRarityGroups(visible, generation);
      return;
    }
    if (state.view === "all") {
      renderChampionGroups(visible, generation);
      return;
    }
    configureSparseGrid(el.grid, visible.length);
    let index = 0;
    const appendChunk = () => {
      if (generation !== state.renderGeneration) return;
      const fragment = document.createDocumentFragment();
      for (const end = Math.min(index + 40, visible.length); index < end; index += 1) fragment.append(makeSkinCard(visible[index], { detailItems: visible }));
      el.grid.append(fragment);
      if (index < visible.length) queueRenderFrame(appendChunk);
    };
    appendChunk();
  }

  function renderRarityGroups(items, generation) {
    const groups = [];
    for (const skin of items) {
      const tier = readableRarity(skin);
      const subtier = skin.raritySubtier || "";
      const key = `${tier}/${subtier}`;
      let group = groups.find((item) => item.key === key);
      if (!group) {
        group = { key, name: subtier ? `${tier} · ${subtier}` : tier, hint: "", skins: [] };
        groups.push(group);
      }
      group.skins.push(skin);
    }
    const tasks = [];
    const displayOrder = groups.flatMap((group) => group.skins);
    for (const group of groups) {
      tasks.push(() => {
        const section = document.createElement("section");
        section.className = "champion-group rarity-group";
        section.innerHTML = `<header class="champion-heading"><div><h2>${escapeHTML(group.name)}</h2><span>${formatNumber(group.skins.length)} 款${group.hint ? ` · ${escapeHTML(group.hint)}` : ""}</span></div></header><div class="champion-skins"></div>`;
        group.grid = section.querySelector(".champion-skins");
        configureSparseGrid(group.grid, group.skins.length);
        el.grid.append(section);
      });
      for (const skin of group.skins) tasks.push(() => group.grid?.append(makeSkinCard(skin, { detailItems: displayOrder })));
    }
    runRenderTasks(tasks, generation, "renderGeneration");
  }

  function renderChampionGroups(items, generation) {
    const groups = [];
    for (const skin of items) {
      const previous = groups.at(-1);
      const key = String(skin.championId || skin.championName || "unknown");
      if (!previous || previous.key !== key) groups.push({ key, name: skin.championName || `英雄 ID ${skin.championId || "—"}`, points: Number(skin.championMasteryPoints || 0), level: Number(skin.championMasteryLevel || 0), skins: [] });
      groups.at(-1).skins.push(skin);
    }
    const tasks = [];
    for (const group of groups) {
      tasks.push(() => {
        const section = document.createElement("section");
        section.className = "champion-group";
        const ownedCount = group.skins.filter((skin) => skin.owned).length;
        section.innerHTML = `<header class="champion-heading"><div><h2>${escapeHTML(group.name)}</h2><span>${formatNumber(group.skins.length)} 款 · 已拥有 ${formatNumber(ownedCount)}</span></div><span class="mastery-summary">${group.points ? `熟练度 ${formatNumber(group.points)}${group.level ? ` · 等级 ${formatNumber(group.level)}` : ""}` : "尚无熟练度记录"}</span></header><div class="champion-skins"></div>`;
        group.grid = section.querySelector(".champion-skins");
        configureSparseGrid(group.grid, group.skins.length);
        el.grid.append(section);
      });
      for (const skin of group.skins) tasks.push(() => group.grid?.append(makeSkinCard(skin, { detailItems: group.skins })));
    }
    runRenderTasks(tasks, generation, "renderGeneration");
  }

  function renderChromaCatalog(items, generation) {
    const prestige = items.filter((item) => item.isPrestige);
    const fragment = document.createDocumentFragment();
    let prestigeGrid = null;
    if (prestige.length) {
      const section = document.createElement("section");
      section.className = "chroma-section prestige-section";
      if (prestige.some((item) => item.owned)) section.classList.add("has-owned");
      section.innerHTML = `<header class="champion-heading"><div><h2>臻彩</h2><span>${formatNumber(prestige.length)} 款 · 独立原画</span></div></header><div class="chroma-prestige-grid"></div>`;
      prestigeGrid = section.querySelector(".chroma-prestige-grid");
      fragment.append(section);
    } else {
      const section = document.createElement("section");
      section.className = "chroma-prestige-status";
      section.innerHTML = `<strong>臻彩</strong><span>当前客户端没有返回同时具备炫彩关联与独立原画的可验证记录。</span>`;
      fragment.append(section);
    }
    const champions = new Map();
    for (const chroma of items) {
      const key = String(chroma.championId || chroma.championName || "unknown");
      if (!champions.has(key)) champions.set(key, { name: chroma.championName || `英雄 ID ${chroma.championId || "—"}`, points: Number(chroma.championMasteryPoints || 0), items: [], parents: new Map(), grid: null });
      const champion = champions.get(key);
      champion.items.push(chroma);
      const parentKey = String(chroma.parentSkinId || chroma.id);
      if (!champion.parents.has(parentKey)) champion.parents.set(parentKey, { name: chroma.parentSkinName || chroma.name, items: [] });
      champion.parents.get(parentKey).items.push(chroma);
    }
    el.grid.append(fragment);
    const championList = [...champions.values()];
    const prestigeTasks = [];
    if (prestigeGrid) {
      for (let start = 0; start < prestige.length; start += 8) {
        prestigeTasks.push(() => {
          const chunk = document.createDocumentFragment();
          for (let index = start; index < Math.min(start + 8, prestige.length); index += 1) chunk.append(makeChromaCard(prestige[index], prestige));
          prestigeGrid.append(chunk);
        });
      }
    }
    const tasks = [];
    let prestigeTaskIndex = 0;
    for (let championIndex = 0; championIndex < championList.length; championIndex += 1) {
      const champion = championList[championIndex];
      if (prestigeTaskIndex < prestigeTasks.length && championIndex % 2 === 0) tasks.push(prestigeTasks[prestigeTaskIndex++]);
      tasks.push(() => {
        const section = document.createElement("section");
        section.className = "chroma-section champion-group";
        applyChromaGroupClasses(section, champion.items);
        section.innerHTML = `<header class="champion-heading"><div><h2>${escapeHTML(champion.name)}</h2><span class="chroma-section-summary" ${chromaSummaryAttributes(champion.items, "款炫彩")}></span></div><span class="mastery-summary">${champion.points ? `熟练度 ${formatNumber(champion.points)}` : "尚无熟练度记录"}</span></header><div class="chroma-stack-grid"></div>`;
        champion.grid = section.querySelector(".chroma-stack-grid");
        el.grid.append(section);
      });
      for (const parent of champion.parents.values()) tasks.push(() => champion.grid?.append(makeChromaStack(parent)));
    }
    while (prestigeTaskIndex < prestigeTasks.length) tasks.push(prestigeTasks[prestigeTaskIndex++]);
    runRenderTasks(tasks, generation, "renderGeneration");
  }

  function cancelRenderFrames() {
    for (const frame of state.renderFrames) cancelAnimationFrame(frame);
    state.renderFrames.clear();
  }

  function queueRenderFrame(callback) {
    const frame = requestAnimationFrame(() => {
      state.renderFrames.delete(frame);
      callback();
    });
    state.renderFrames.add(frame);
  }

  function cancelPoolRenderFrames() {
    for (const frame of state.poolRenderFrames) cancelAnimationFrame(frame);
    state.poolRenderFrames.clear();
  }

  function queuePoolRenderFrame(callback) {
    const frame = requestAnimationFrame(() => {
      state.poolRenderFrames.delete(frame);
      callback();
    });
    state.poolRenderFrames.add(frame);
  }

  function runRenderTasks(tasks, generation, generationKey) {
    let index = 0;
    const run = () => {
      if (generation !== state[generationKey]) return;
      const started = performance.now();
      let completed = 0;
      while (index < tasks.length && (completed < 1 || performance.now() - started < 7)) {
        tasks[index++]();
        completed += 1;
      }
      if (index < tasks.length) (generationKey === "poolRenderGeneration" ? queuePoolRenderFrame : queueRenderFrame)(run);
    };
    run();
  }

  function chromaVisibilityCounts(items) {
    let owned = 0;
    let ordinary = 0;
    let ownedOrdinary = 0;
    for (const item of items) {
      if (item.owned) owned += 1;
      if (!item.isPrestige) {
        ordinary += 1;
        if (item.owned) ownedOrdinary += 1;
      }
    }
    return { all: items.length, owned, ordinary, ownedOrdinary };
  }

  function applyChromaGroupClasses(node, items) {
    const counts = chromaVisibilityCounts(items);
    node.classList.toggle("has-owned", counts.owned > 0);
    node.classList.toggle("has-ordinary", counts.ordinary > 0);
    node.classList.toggle("has-owned-ordinary", counts.ownedOrdinary > 0);
    return counts;
  }

  function chromaSummaryAttributes(items, noun = "款") {
    const counts = chromaVisibilityCounts(items);
    const label = (count, owned) => `${formatNumber(count)} ${noun} · 已拥有 ${formatNumber(owned)}`;
    return `data-all="${label(counts.all, counts.owned)}" data-owned="${label(counts.owned, counts.owned)}" data-ordinary="${label(counts.ordinary, counts.ownedOrdinary)}" data-owned-ordinary="${label(counts.ownedOrdinary, counts.ownedOrdinary)}"`;
  }

  function makeChromaCard(chroma, candidates) {
    const card = el.template.content.firstElementChild.cloneNode(true);
    const image = card.querySelector("img");
    const video = card.querySelector("video");
    const fallback = card.querySelector(".image-fallback");
    video.remove();
    image.alt = `${chroma.name}臻彩原画`;
    image.loading = "lazy";
    deferCardImageSources(image, fallback, chromaImageSources(chroma, "prestige"), true);
    const locked = !chroma.owned;
    card.classList.toggle("is-locked", locked);
    const status = card.querySelector(".skin-state");
    status.textContent = chroma.owned ? "已拥有" : "未获取";
    status.hidden = false;
    card.querySelector("strong").textContent = chroma.name;
    card.querySelector(".skin-hero").textContent = chroma.championName || `英雄 ID ${chroma.championId || "—"}`;
    card.querySelector(".skin-meta").textContent = `臻彩 · ID ${chroma.id}`;
    card.setAttribute("aria-label", `${locked ? "未获取，" : ""}查看 ${chroma.name} 炫彩详情`);
    card.addEventListener("click", () => openChromaDetails(chroma, candidates, { artworkMode: "prestige" }));
    return card;
  }

  function makeChromaStack(parent) {
    const section = document.createElement("section");
    section.className = "chroma-stack";
    applyChromaGroupClasses(section, parent.items);
    section.innerHTML = `<header><div><strong>${escapeHTML(parent.name)}</strong><span class="chroma-stack-summary" ${chromaSummaryAttributes(parent.items)}></span></div><div class="chroma-row-controls" aria-label="横向浏览${escapeHTML(parent.name)}炫彩"><button class="control-icon-button" type="button" data-direction="previous" aria-label="向左查看更多炫彩">${navigationIcon("previous")}</button><button class="control-icon-button" type="button" data-direction="next" aria-label="向右查看更多炫彩">${navigationIcon("next")}</button></div></header><div class="chroma-stack-cards" role="group" aria-label="${escapeHTML(parent.name)}的炫彩列表"></div>`;
    const cards = section.querySelector(".chroma-stack-cards");
    for (const chroma of parent.items) {
      const button = document.createElement("button");
      button.className = `chroma-mini-card${chroma.owned ? " is-owned" : " is-locked"}${chroma.isPrestige ? " is-prestige-chroma" : ""}`;
      button.type = "button";
      button.setAttribute("aria-label", `${chroma.owned ? "已拥有" : "未获取"}，查看 ${chroma.name}`);
      button.innerHTML = `<span class="chroma-mini-art"><img alt="" loading="lazy" decoding="async">${chroma.isPrestige ? '<span class="chroma-prestige-badge">臻彩</span>' : ""}${chroma.owned ? "" : '<span class="chroma-mini-lock" aria-hidden="true">🔒</span>'}</span><span class="chroma-mini-name">${escapeHTML(chromaDisplayName(chroma))}</span>`;
      const image = button.querySelector("img");
      const fallback = document.createElement("span");
      fallback.className = "image-fallback";
      fallback.textContent = "加载中";
      fallback.hidden = true;
      button.querySelector(".chroma-mini-art").append(fallback);
      deferCardImageSources(image, fallback, chromaImageSources(chroma, "ordinary"));
      button.addEventListener("click", () => openChromaDetails(chroma, parent.items, { artworkMode: "ordinary" }));
      cards.append(button);
    }
    enableHorizontalNavigation(cards, section.querySelector('[data-direction="previous"]'), section.querySelector('[data-direction="next"]'), parent.items.length);
    return section;
  }

  function chromaDisplayName(chroma) {
    const name = String(chroma?.name || "").trim();
    const parent = String(chroma?.parentSkinName || "").trim();
    if (!name) return `炫彩 ${chroma?.id || "—"}`;
    if (!parent) return name;
    const compactName = name.replace(/\s+/g, " ");
    const compactParent = parent.replace(/\s+/g, " ");
    if (!compactName.startsWith(compactParent)) return name;
    const remainder = compactName.slice(compactParent.length);
    if (remainder && !/^[\s·:：\-—–_/（(【\[]/u.test(remainder)) return name;
    const suffix = remainder
      .replace(/^[\s·:：\-—–_/（(【\[]+/u, "")
      .replace(/[）)】\]]+$/u, "")
      .trim();
    return suffix || name;
  }

  function enableHorizontalNavigation(scroller, previousButton, nextButton, itemCount = 2) {
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const controls = previousButton.parentElement;
    let updateFrame = 0;
    const updateControls = () => {
      updateFrame = 0;
      const maximum = Math.max(0, scroller.scrollWidth - scroller.clientWidth);
      controls.hidden = maximum <= 1;
      previousButton.disabled = scroller.scrollLeft <= 1;
      nextButton.disabled = scroller.scrollLeft >= maximum - 1;
    };
    const scheduleUpdate = () => {
      if (!updateFrame) updateFrame = requestAnimationFrame(updateControls);
    };
    const move = (direction) => {
      scroller.scrollBy({ left: direction * Math.max(320, scroller.clientWidth * 0.72), behavior: reducedMotion ? "auto" : "smooth" });
    };
    previousButton.addEventListener("click", () => move(-1));
    nextButton.addEventListener("click", () => move(1));
    scroller.addEventListener("scroll", scheduleUpdate, { passive: true });
    scroller.addEventListener("pointerenter", scheduleUpdate, { once: true, passive: true });
    scroller.addEventListener("focusin", scheduleUpdate, { once: true, passive: true });
    scroller.addEventListener("wheel", (event) => {
      if (!event.shiftKey || scroller.scrollWidth <= scroller.clientWidth) return;
      event.preventDefault();
      scroller.scrollLeft += Math.abs(event.deltaY) >= Math.abs(event.deltaX) ? event.deltaY : event.deltaX;
    }, { passive: false });
    controls.hidden = itemCount < 2;
    previousButton.disabled = true;
    nextButton.disabled = itemCount < 2;
  }

  function localImageSources(paths) { return paths.map((path) => `/api/image?path=${encodeURIComponent(path)}`); }
  function navigationIcon(direction) {
    const path = direction === "previous" ? "m15 5-7 7 7 7" : "m9 5 7 7-7 7";
    return `<svg class="control-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="${path}"/></svg>`;
  }
  function ordinaryChromaImageSources(chroma) {
    return [...new Set(localImageSources([chroma.tilePath, chroma.chromaPath, chroma.splashPath].filter(Boolean)))];
  }

  function chromaImageSources(chroma, mode = "ordinary") {
    if (mode === "prestige" && chroma.prestigeImageId) return [`/api/prestige-image?id=${encodeURIComponent(chroma.id)}`];
    return ordinaryChromaImageSources(chroma);
  }

  function updateChromaArtworkControl(chroma) {
    const canToggle = Boolean(chroma?.isPrestige && chroma?.prestigeImageId && ordinaryChromaImageSources(chroma).length);
    el.skinDialogArtwork.hidden = !canToggle;
    const showingPrestige = canToggle && state.detailArtworkMode === "prestige";
    const label = showingPrestige ? "切换到普通炫彩图" : "切换到臻彩原画";
    el.skinDialogArtwork.setAttribute("aria-label", label);
    el.skinDialogArtwork.title = label;
    el.skinDialogArtwork.classList.toggle("is-active", showingPrestige);
    el.skinDialog.classList.toggle("is-prestige-dialog", showingPrestige);
  }

  function loadSelectedChromaArtwork(chroma, generation) {
    requestAnimationFrame(() => {
      if (generation !== state.detailGeneration || !el.skinDialog.open) return;
      el.skinDialogImage.fetchPriority = "high";
      loadImageSources(el.skinDialogImage, el.skinDialogFallback, chromaImageSources(chroma, state.detailArtworkMode), () => {
        if (generation !== state.detailGeneration || !el.skinDialog.open) return;
        state.detailMediaTimer = setTimeout(() => {
          if (generation === state.detailGeneration && el.skinDialog.open) loadDialogBackdrop(state.detailArtworkMode === "prestige" ? "" : chroma.parentSplashPath);
        }, 180);
        prefetchAdjacentChromaArtwork(generation);
      });
    });
  }

  function openChromaDetails(chroma, candidates = null, options = {}) {
    const generation = ++state.detailGeneration;
    clearTimeout(state.detailMediaTimer);
    state.selectedSkin = chroma;
    state.detailKind = "chroma";
    state.detailArtworkMode = chroma.isPrestige && options.artworkMode === "prestige" ? "prestige" : "ordinary";
    state.detailItems = candidates || state.items.filter((item) => Number(item.parentSkinId) === Number(chroma.parentSkinId));
    el.skinDialog.classList.add("is-chroma-dialog");
    updateChromaArtworkControl(chroma);
    el.skinDialogTitle.textContent = chroma.name;
    el.skinDialogHero.textContent = `${chroma.championName || `英雄 ID ${chroma.championId || "—"}`} · ${chroma.parentSkinName || "独立臻彩"}`;
    el.skinDialogStatus.textContent = chroma.owned ? "当前账号已拥有" : "当前账号未获取";
    const rows = [
      ["炫彩 ID", chroma.id],
      ["所属皮肤", chroma.parentSkinName || "独立臻彩"],
      ["分类", chroma.isPrestige ? "臻彩" : "普通炫彩"],
      ["品质", chroma.isPrestige ? "臻彩" : `${readableRarity(chroma)}${chroma.raritySubtier ? ` · ${chroma.raritySubtier}` : ""}`],
    ];
    if (chroma.owned) rows.push(["获取时间", chroma.acquiredAt ? formatDateTime(chroma.acquiredAt) : "客户端未提供"]);
    el.skinDialogData.innerHTML = rows.map(([label, value]) => `<dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd>`).join("");
    el.skinDialogImage.alt = `${chroma.name}炫彩预览`;
    resetDialogImage("正在读取预览…");
    resetVideo(el.skinDialogVideo);
    el.copySkinId.textContent = "复制炫彩 ID";
    updateDetailNavigation();
    if (!el.skinDialog.open) {
      el.skinDialog.showModal();
      window.desktopTheme?.setModalOpen?.(true);
    }
    loadSelectedChromaArtwork(chroma, generation);
  }

  function loadDialogBackdrop(path) {
    const image = el.skinDialogBackdrop;
    image.onload = null;
    image.onerror = null;
    image.hidden = true;
    image.removeAttribute("src");
    if (!path) return;
    image.onload = () => { image.hidden = false; };
    image.onerror = () => { image.hidden = true; image.removeAttribute("src"); };
    image.src = `/api/image?path=${encodeURIComponent(path)}`;
  }

  function prefetchAdjacentChromaArtwork(generation) {
    if (state.detailKind !== "chroma" || state.detailItems.length < 2) return;
    const index = state.detailItems.findIndex((item) => Number(item.id) === Number(state.selectedSkin?.id));
    const chroma = state.detailItems[(index + 1) % state.detailItems.length];
    const source = chromaImageSources(chroma, state.detailArtworkMode)[0];
    if (!source || state.artworkPrefetchCache.has(source)) return;
    scheduleIdleTask(() => {
      if (generation !== state.detailGeneration || state.artworkPrefetchCache.has(source)) return;
      const image = new Image();
      state.artworkPrefetchCache.set(source, image);
      image.decoding = "async";
      image.onload = image.onerror = () => trimArtworkPrefetchCache();
      image.src = source;
    }, 1200);
  }

  function trimArtworkPrefetchCache() {
    while (state.artworkPrefetchCache.size > 36) state.artworkPrefetchCache.delete(state.artworkPrefetchCache.keys().next().value);
  }

  function scheduleIdleTask(callback, timeout = 900) {
    if ("requestIdleCallback" in window) return window.requestIdleCallback(callback, { timeout });
    return setTimeout(callback, Math.min(timeout, 300));
  }

  function configureSparseGrid(grid, count) {
    if (count < 1 || count > 3) return;
    grid.classList.add("is-sparse");
    grid.style.setProperty("--sparse-columns", String(count));
    grid.style.setProperty("--sparse-max-width", `${count * 640}px`);
  }

  function makeSkinCard(skin, options = {}) {
    const card = el.template.content.firstElementChild.cloneNode(true);
    const video = card.querySelector("video");
    const image = card.querySelector("img");
    const fallback = card.querySelector(".image-fallback");
    image.alt = `${skin.championName ? `${skin.championName}的` : ""}${skin.name}皮肤原画`;
    fallback.setAttribute("aria-label", `${skin.name}没有可用预览`);
    deferCardImageSources(image, fallback, skinImageSources(skin));
    prepareSkinVideo(video, image, fallback, skinVideoPath(skin), card);
    const locked = options.locked ?? (state.view === "all" && !skin.owned);
    card.classList.toggle("is-locked", locked);
    const status = card.querySelector(".skin-state");
    const showPoolState = options.poolState || (state.view === "all" && !skin.owned && Boolean(skin.poolName));
    status.textContent = showPoolState ? "三合一剩余" : "";
    status.hidden = !showPoolState;
    card.querySelector("strong").textContent = skin.name;
    card.querySelector(".skin-hero").textContent = skin.championName || `英雄 ID ${skin.championId || "—"}`;
    card.querySelector(".skin-meta").textContent = `${readableRarity(skin)}${skin.raritySubtier ? ` · ${skin.raritySubtier}` : ""} · ID ${skin.id}`;
    card.setAttribute("aria-label", `${locked ? "未拥有，" : ""}查看 ${skin.name} 详情`);
    card.addEventListener("click", () => openSkinDetails(skin, options.detailItems));
    return card;
  }

  function imagePaths(skin) { return [...new Set([skin.splashPath, skin.tilePath, skin.loadScreenPath].filter(Boolean))]; }
  function skinImageSources(skin) {
    const sources = localImageSources(imagePaths(skin));
    const skinId = Number(skin?.id);
    if (Number.isInteger(skinId) && skinId >= 1000) sources.push(`/api/skin-art?id=${skinId}`);
    return sources;
  }
  function skinVideoPath(skin) { return skin.collectionVideoPath || skin.splashVideoPath || skin.cardHoverVideoPath; }
  function prepareSkinVideo(video, image, fallback, path, card) {
    video.hidden = true;
    video.removeAttribute("src");
    if (!path) return;
    const start = () => loadSkinVideo(video, image, fallback, path);
    card.addEventListener("pointerenter", start, { once: true, passive: true });
    card.addEventListener("focus", start, { once: true, passive: true });
  }
  function loadSkinVideo(video, image, fallback, path) {
    resetVideo(video);
    if (!path) return;
    video.src = `/api/media?path=${encodeURIComponent(path)}`;
    video.oncanplay = () => {
      video.hidden = false;
      image.hidden = true;
      fallback.hidden = true;
      video.play().catch(() => {});
    };
    video.onerror = () => resetVideo(video);
  }
  function resetVideo(video) {
    try { video.pause(); } catch (_) {}
    video.oncanplay = null;
    video.onerror = null;
    video.hidden = true;
    video.removeAttribute("src");
    try { video.load(); } catch (_) {}
  }
  function loadImageAlternatives(image, fallback, paths, onComplete = null) {
    loadImageSources(image, fallback, localImageSources(paths), onComplete);
  }
  function loadImageSources(image, fallback, sources, onComplete = null) {
    let index = 0;
    let completed = false;
    const complete = () => {
      if (completed) return;
      completed = true;
      onComplete?.();
    };
    image.onload = null;
    image.onerror = null;
    image.removeAttribute("src");
    image.classList.remove("is-loaded");
    image.hidden = false;
    fallback.hidden = false;
    const next = () => {
      if (index >= sources.length) { image.hidden = true; fallback.textContent = fallback.dataset.emptyText || "暂无预览"; fallback.hidden = false; complete(); return; }
      image.src = sources[index++];
    };
    image.onload = () => { image.classList.add("is-loaded"); fallback.hidden = true; complete(); };
    image.onerror = next;
    next();
  }

  function ensureCardImageObserver() {
    if (state.cardImageObserver || !("IntersectionObserver" in window)) return state.cardImageObserver;
    state.cardImageObserver = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const job = state.cardImageJobs.get(entry.target);
        state.cardImageObserver.unobserve(entry.target);
        if (job && !job.cancelled) enqueueCardImageJob(job);
      }
    }, { root: el.appScroll, rootMargin: "620px 0px" });
    return state.cardImageObserver;
  }

  function deferCardImageSources(image, fallback, sources, remote = false) {
    image.onload = null;
    image.onerror = null;
    image.removeAttribute("src");
    image.classList.remove("is-loaded");
    image.hidden = false;
    image.fetchPriority = remote ? "low" : "auto";
    fallback.textContent = "加载中";
    fallback.dataset.emptyText = "暂无预览";
    fallback.hidden = false;
    const job = { image, fallback, sources, remote, active: false, done: false, cancelled: false };
    state.cardImageJobs.set(image, job);
    image.dataset.cardImage = "pending";
    const observer = ensureCardImageObserver();
    if (observer) observer.observe(image);
    else enqueueCardImageJob(job);
  }

  function enqueueCardImageJob(job) {
    if (job.done || job.cancelled || job.active) return;
    if (!job.image.isConnected) {
      requestAnimationFrame(() => {
        if (!job.done && !job.cancelled) enqueueCardImageJob(job);
      });
      return;
    }
    job.image.dataset.cardImage = "queued";
    state.cardImageQueue.push(job);
    pumpCardImageQueue();
  }

  function pumpCardImageQueue() {
    while (state.activeCardImages < 8) {
      let index = state.cardImageQueue.findIndex((job) => !job.cancelled && !job.done && !job.remote);
      if (index < 0 && state.activePrestigeCardImages < 2) index = state.cardImageQueue.findIndex((job) => !job.cancelled && !job.done);
      if (index < 0) return;
      const [job] = state.cardImageQueue.splice(index, 1);
      if (job.cancelled || job.done) continue;
      job.active = true;
      state.activeCardImages += 1;
      if (job.remote) state.activePrestigeCardImages += 1;
      job.image.dataset.cardImage = "loading";
      loadImageSources(job.image, job.fallback, job.sources, () => finishCardImageJob(job));
    }
  }

  function finishCardImageJob(job) {
    if (job.done) return;
    job.done = true;
    if (job.active) {
      state.activeCardImages = Math.max(0, state.activeCardImages - 1);
      if (job.remote) state.activePrestigeCardImages = Math.max(0, state.activePrestigeCardImages - 1);
    }
    job.active = false;
    delete job.image.dataset.cardImage;
    state.cardImageJobs.delete(job.image);
    pumpCardImageQueue();
  }

  function cancelDeferredImages(container) {
    if (!container) return;
    for (const image of container.querySelectorAll("img[data-card-image]")) {
      const job = state.cardImageJobs.get(image);
      if (!job || job.done) continue;
      job.cancelled = true;
      state.cardImageObserver?.unobserve(image);
      image.onload = null;
      image.onerror = null;
      image.removeAttribute("src");
      finishCardImageJob(job);
    }
    state.cardImageQueue = state.cardImageQueue.filter((job) => !job.cancelled && !job.done);
  }

  function resetDialogImage(message) {
    loadDialogBackdrop("");
    el.skinDialogImage.onload = null;
    el.skinDialogImage.onerror = null;
    el.skinDialogImage.removeAttribute("src");
    el.skinDialogImage.classList.remove("is-loaded");
    el.skinDialogImage.hidden = true;
    el.skinDialogFallback.textContent = message;
    el.skinDialogFallback.dataset.emptyText = "暂无预览";
    el.skinDialogFallback.hidden = false;
  }

  async function openSkinDetails(skin, candidates = null) {
    const generation = ++state.detailGeneration;
    clearTimeout(state.detailMediaTimer);
    state.selectedSkin = skin;
    state.detailKind = "skin";
    state.detailItems = candidates || state.items;
    el.skinDialog.classList.remove("is-chroma-dialog");
    el.skinDialog.classList.remove("is-prestige-dialog");
    el.skinDialogArtwork.hidden = true;
    el.copySkinId.textContent = "复制皮肤 ID";
    el.skinDialogTitle.textContent = skin.name;
    el.skinDialogHero.textContent = skin.championName || `英雄 ID ${skin.championId || "—"}`;
    el.skinDialogStatus.textContent = skin.owned ? "当前账号已拥有" : skin.poolName ? "当前三合一仍可获得" : "当前账号未拥有";
    updateDetailNavigation();
    renderSkinDetailData(skin, null);
    el.skinDialogImage.alt = `${skin.name}皮肤原画`;
    resetDialogImage("正在读取原画…");
    resetVideo(el.skinDialogVideo);
    if (!el.skinDialog.open) {
      el.skinDialog.showModal();
      window.desktopTheme?.setModalOpen?.(true);
    }
    requestAnimationFrame(() => {
      if (generation !== state.detailGeneration || !el.skinDialog.open) return;
      el.skinDialogImage.fetchPriority = "high";
      loadImageSources(el.skinDialogImage, el.skinDialogFallback, skinImageSources(skin), () => {
        if (generation !== state.detailGeneration || !el.skinDialog.open) return;
        state.detailMediaTimer = setTimeout(() => {
          if (generation === state.detailGeneration && el.skinDialog.open) loadSkinVideo(el.skinDialogVideo, el.skinDialogImage, el.skinDialogFallback, skinVideoPath(skin));
        }, 650);
      });
    });
    const cached = state.skinDetailCache.get(Number(skin.id));
    if (cached) {
      renderSkinDetailData(cached.skin || skin, cached.details || {});
      prefetchAdjacentSkinDetails();
      return;
    }
    try {
      const payload = await getSkinDetails(skin.id);
      if (generation !== state.detailGeneration || state.selectedSkin?.id !== skin.id) return;
      state.skinDetailCache.set(Number(skin.id), payload);
      renderSkinDetailData(payload.skin || skin, payload.details || {});
      prefetchAdjacentSkinDetails();
    } catch (error) {
      if (generation !== state.detailGeneration || error.name === "RequestCancelled") return;
      renderSkinDetailData(skin, {});
    }
  }

  function updateDetailNavigation() {
    const items = state.detailItems || [];
    const index = items.findIndex((item) => Number(item.id) === Number(state.selectedSkin?.id));
    const disabled = items.length < 2 || index < 0;
    el.skinDialogPrevious.disabled = disabled;
    el.skinDialogNext.disabled = disabled;
    el.skinDialogPrevious.hidden = disabled;
    el.skinDialogNext.hidden = disabled;
    if (!disabled) {
      el.skinDialogPrevious.setAttribute("aria-label", `查看上一款，共 ${items.length} 款`);
      el.skinDialogNext.setAttribute("aria-label", `查看下一款，共 ${items.length} 款`);
    }
  }

  function navigateDetail(direction) {
    const items = state.detailItems || [];
    if (items.length < 2) return;
    const current = items.findIndex((item) => Number(item.id) === Number(state.selectedSkin?.id));
    const next = items[(current + direction + items.length) % items.length];
    if (state.detailKind === "chroma") openChromaDetails(next, items, { artworkMode: state.detailArtworkMode });
    else openSkinDetails(next, items);
  }

  function prefetchAdjacentSkinDetails() {
    if (state.detailKind !== "skin" || state.detailItems.length < 2) return;
    const index = state.detailItems.findIndex((item) => Number(item.id) === Number(state.selectedSkin?.id));
    const skin = state.detailItems[(index + 1) % state.detailItems.length];
    if (!skin || state.skinDetailCache.has(Number(skin.id))) return;
    const generation = state.detailGeneration;
    scheduleIdleTask(() => {
      if (generation !== state.detailGeneration) return;
      getSkinDetails(skin.id).catch(() => {});
    }, 1000);
  }

  function getSkinDetails(id) {
    const key = Number(id);
    const cached = state.skinDetailCache.get(key);
    if (cached) return Promise.resolve(cached);
    const pending = state.skinDetailPromises.get(key);
    if (pending) return pending;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 12000);
    const request = fetch(`/api/skin-details?id=${encodeURIComponent(key)}`, { headers: { Accept: "application/json" }, signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error((await response.text()).trim() || `本地服务返回 HTTP ${response.status}`);
        return response.json();
      })
      .then((payload) => {
        state.skinDetailCache.set(key, payload);
        while (state.skinDetailCache.size > 320) state.skinDetailCache.delete(state.skinDetailCache.keys().next().value);
        return payload;
      })
      .finally(() => {
        clearTimeout(timer);
        state.skinDetailPromises.delete(key);
      });
    state.skinDetailPromises.set(key, request);
    return request;
  }

  function renderSkinDetailData(skin, details) {
    const rows = [
      ["皮肤 ID", skin.id],
      ["英雄 ID", skin.championId || "—"],
      ["品质", `${readableRarity(skin)}${skin.raritySubtier ? ` · ${skin.raritySubtier}` : ""}`],
      ["价格", skin.rarityTier === "圣堂" ? "圣堂花火 80 抽" : details === null ? "正在读取…" : details.priceKnown ? `${formatNumber(details.priceRp)} RP` : "客户端未提供 RP 价格"],
    ];
    if (skin.isVariant && skin.parentSkinId) rows.push(["形态", `包含于主皮肤（ID ${skin.parentSkinId}）`]);
    if (skin.owned) rows.push(["获取时间", skin.acquiredAt ? formatDateTime(skin.acquiredAt) : "客户端未提供"]);
    if (details?.hasBorder) rows.push(["加载界面边框", details.borderOwnershipKnown ? (details.ownsBorder ? "已拥有" : "未拥有") : "皮肤支持边框，但客户端未提供拥有状态"]);
    el.skinDialogData.innerHTML = rows.map(([label, value]) => `<dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd>`).join("");
  }

  async function loadAccount() {
    if (!state.status?.connected) {
      state.accountLoaded = false;
      if (el.accountLiveState) {
        el.accountLiveState.textContent = "等待客户端";
        el.accountLiveState.className = "state-chip";
      }
      el.accountContent.innerHTML = '<div class="gameplay-empty"><span aria-hidden="true">◉</span><strong>等待英雄联盟客户端</strong><p>登录国服客户端并进入大厅后，这里会按类别展示战利品、皮肤物品和待领取奖励。</p></div>';
      return;
    }
    el.accountContent.innerHTML = '<div class="account-loading"><div class="skeleton-line"></div><div class="skeleton-line"></div><div class="skeleton-line"></div></div>';
    try {
      const payload = await api("/api/account", {}, "account", 15000);
      state.account = payload;
      state.accountLoaded = true;
      const summoner = payload.summoner || {};
      const account = payload.account || {};
      const loot = Array.isArray(account.loot) ? account.loot : [];
      const displayLoot = loot.filter((item) => lootName(item) !== "未识别材料");
      if (account.sanctumSparksKnown) displayLoot.push({
        lootId: "CURRENCY_ANCIENT_SPARK",
        displayName: "圣堂花火",
        category: "材料",
        kind: "圣堂货币",
        count: Number(account.sanctumSparks || 0),
        asset: "/loot-icons/sanctum-spark.svg",
      });
      const rewards = Array.isArray(account.rewards) ? account.rewards : [];
      const capabilities = Array.isArray(account.capabilities) ? account.capabilities : [];
      const categoryOrder = ["材料", "英雄", "皮肤", "小小英雄", "永恒星碑", "表情", "守卫", "图标"];
      const skinLoot = displayLoot.filter((item) => item.category === "皮肤");
      const skinLootQuantity = skinLoot.reduce((total, item) => total + Number(item.count || 0), 0);
      const rewardQuantity = rewards.reduce((total, grant) => total + (grant.items || []).reduce((subtotal, item) => subtotal + Number(item.quantity || 1), 0), 0);
      const categorySections = categoryOrder.map((category) => {
        const items = displayLoot.filter((item) => item.category === category);
        if (!items.length) return "";
        const slug = lootCategorySlug(category);
        return `<section class="account-section loot-section loot-category category-${slug}"><div class="loot-category-heading"><span class="loot-category-icon" aria-hidden="true">${lootCategoryIcon(category)}</span><h3>${escapeHTML(category)}</h3></div><div class="loot-card-grid">${items.map(lootCard).join("")}</div></section>`;
      }).join("");
      const capabilityRows = capabilities.map((item) => `<div class="capability-row"><div><strong>${escapeHTML(capabilityName(item.name))}</strong><small>${escapeHTML(item.detail || "本机客户端已返回数据")}</small></div><span class="state-chip ${escapeHTML(item.state)}">${escapeHTML(sourceStateLabel(item.state))}</span><b>${formatNumber(item.count)}</b></div>`).join("");
      const rewardCards = rewards.map((grant) => {
        const items = (grant.items || []).map((item) => `${escapeHTML(item.title || item.itemType || item.itemId || "奖励")} × ${formatNumber(item.quantity || 1)}`).join("、") || "客户端未提供明细";
        return `<article class="reward-card"><div><span class="state-chip">${escapeHTML(rewardStatusLabel(grant.status))}</span><strong>${escapeHTML(grant.title || "待领取奖励")}</strong></div><p>${items}</p>${grant.description ? `<small>${escapeHTML(grant.description)}</small>` : ""}<time>${grant.dateCreated ? escapeHTML(formatDateTime(grant.dateCreated)) : "创建时间未知"}</time></article>`;
      }).join("");
      el.accountLiveState.textContent = state.status?.eventStream ? "自动更新" : "已连接";
      el.accountLiveState.className = `state-chip ${state.status?.eventStream ? "success" : ""}`;
      const backgroundName = account.profile?.backgroundSkinName || (account.profile?.backgroundSkinId ? "客户端未返回对应名称" : "未设置");
      el.accountContent.innerHTML = `<section class="account-hero"><span class="account-hex" aria-hidden="true">⬡</span><div class="account-identity"><p class="eyebrow">当前召唤师</p><h3>${escapeHTML(playerName(summoner))}</h3><p>等级 ${formatNumber(summoner.summonerLevel)} · 本机只读连接</p></div><dl class="account-facts"><div><dt>主页背景</dt><dd>${escapeHTML(backgroundName)}</dd></div><div><dt>皮肤物品</dt><dd>${formatNumber(skinLoot.length)} 种 · ${formatNumber(skinLootQuantity)} 件</dd></div><div><dt>待领取</dt><dd>${formatNumber(rewards.length)} 组 · ${formatNumber(rewardQuantity)} 件</dd></div></dl></section>${categorySections || '<div class="empty-state"><strong>仓库当前没有可展示物品</strong></div>'}<section class="account-section"><div class="section-copy"><h3>待领取奖励</h3><span class="section-count">${formatNumber(rewards.length)} 组</span></div>${rewardCards ? `<div class="reward-grid">${rewardCards}</div>` : '<div class="empty-state compact"><strong>没有待领取奖励</strong><p>领取操作仍需回到英雄联盟客户端完成。</p></div>'}</section><details class="capability-details"><summary>数据来源状态</summary><p>只读接口单独降级，不影响皮肤库存的双来源一致性核验。</p><div class="capability-list">${capabilityRows || '<p class="muted">尚无能力状态</p>'}</div></details>`;
      for (const image of el.accountContent.querySelectorAll(".loot-art img")) loadNextLootImage(image, true);
    } catch (error) {
      if (error.name === "RequestCancelled" || state.destroyed) return;
      state.accountLoaded = false;
      el.accountLiveState.textContent = "读取失败";
      el.accountLiveState.className = "state-chip failed";
      renderPanelError(el.accountContent, "账户与物品读取失败", error.message, loadAccount);
    }
  }

  async function loadDiagnostics() {
    el.diagnosticsContent.innerHTML = '<p class="muted">正在读取核对详情…</p>';
    try {
      state.diagnostics = await api("/api/diagnostics", {}, "diagnostics");
      const data = state.diagnostics;
	  const discovery = data.discovery || {};
      const rows = (data.ownershipSources || []).map((source) => {
        const evidenceCount = Number(source.evidenceCount || 0);
        const unknownCount = Number(source.unknownCount || 0);
        const count = `${formatNumber(source.count)} / ${formatNumber(source.rawOwnedCount || source.count)} / ${evidenceCount > 0 ? formatNumber(evidenceCount) : "—"} / ${formatNumber(source.variantCount)} / ${formatNumber(source.baseCount)} / ${formatNumber(unknownCount)} / ${formatNumber(source.rentalCount)} / ${formatNumber(source.freeToPlayCount)}`;
        return `<tr><td>${escapeHTML(source.path)}</td><td><span class="state-chip ${escapeHTML(source.state)}">${escapeHTML(sourceStateLabel(source.state))}</span></td><td>${count}</td><td>${escapeHTML(source.detail || "—")}</td></tr>`;
      }).join("");
      const capabilityRows = (data.capabilities || []).map((item) => `<tr><td>${escapeHTML(capabilityName(item.name))}</td><td><span class="state-chip ${escapeHTML(item.state)}">${escapeHTML(sourceStateLabel(item.state))}</span></td><td>${formatNumber(item.count)}</td><td>${escapeHTML(item.detail || "—")}</td></tr>`).join("");
	  el.diagnosticsContent.innerHTML = `<div class="audit-tables"><section><h3>客户端发现</h3><div class="stat-grid"><div class="stat"><span>探测结果</span><strong>${escapeHTML(discoveryResultLabel(discovery.result))}</strong></div><div class="stat"><span>探测方式</span><strong>${escapeHTML(discovery.method || "尚未探测")}</strong></div><div class="stat"><span>客户端进程</span><strong>${formatNumber(discovery.processCount)}</strong></div><div class="stat"><span>可读命令行</span><strong>${formatNumber(discovery.commandLineCount)}</strong></div><div class="stat"><span>凭据候选</span><strong>${formatNumber(discovery.credentialCandidates)}</strong></div><div class="stat"><span>接口探测失败</span><strong>${formatNumber(discovery.probeFailures)}</strong></div></div><p class="muted">${escapeHTML(discovery.detail || "尚未运行客户端探测")}${discovery.attemptAt ? ` · ${escapeHTML(formatDateTime(discovery.attemptAt))}` : ""}</p></section><section><h3>核对概览</h3><div class="stat-grid"><div class="stat"><span>快照状态</span><strong>${data.snapshotReady ? "完整" : "未通过"}</strong></div><div class="stat"><span>目录指纹</span><strong>${escapeHTML(shortHash(data.catalog?.fingerprint))}</strong></div><div class="stat"><span>普通 / 基础 / 英雄</span><strong>${formatNumber(Number(data.catalog?.skinCount || 0) - Number(data.catalog?.baseSkinCount || 0))} / ${formatNumber(data.catalog?.baseSkinCount)} / ${formatNumber(data.catalog?.championCount)}</strong></div><div class="stat"><span>奖池映射</span><strong>${formatNumber(data.poolMatched)} / ${formatNumber(data.poolTotal)}</strong></div><div class="stat"><span>读取耗时</span><strong>${formatNumber(data.lastDurationMs)} ms</strong></div><div class="stat"><span>LCU 凭据来源</span><strong>${escapeHTML(data.lcuSource || "未连接")}</strong></div><div class="stat"><span>更新方式</span><strong>${data.eventStream ? "自动更新" : "自动 / 手动更新"}</strong></div><div class="stat"><span>诊断日志</span><strong>${data.diagnosticLogReady ? "可导出" : escapeHTML(data.diagnosticLogError || "不可用")}</strong></div></div></section><section><h3>皮肤库存证据</h3><p class="muted">导出的本地日志会附带目录外、独立形态、租用与免费轮换 ID；只包含公开物品编号，不包含客户端令牌。</p><div class="table-scroll"><table class="data-table"><thead><tr><th>库存来源</th><th>状态</th><th>目录命中 / 原始拥有 / 核验 / 独立形态 / 基础 / 目录外 / 租用 / 免费</th><th>说明</th></tr></thead><tbody>${rows || '<tr><td colspan="4">尚无库存证据</td></tr>'}</tbody></table></div></section><section><h3>附加只读能力</h3><div class="table-scroll"><table class="data-table"><thead><tr><th>能力</th><th>状态</th><th>条数</th><th>说明</th></tr></thead><tbody>${capabilityRows || '<tr><td colspan="4">尚无能力状态</td></tr>'}</tbody></table></div></section></div>`;
    } catch (error) { renderPanelError(el.diagnosticsContent, "诊断信息读取失败", error.message, loadDiagnostics); }
  }

  async function loadHistory() {
    el.historyContent.innerHTML = '<p class="muted">正在读取本地历史…</p>';
    try {
      const payload = await api("/api/snapshots", {}, "history");
      state.history = payload.items || [];
      state.historyLoaded = true;
      if (!state.history.length) { el.historyContent.innerHTML = '<div class="empty-state"><strong>还没有历史快照</strong><p>完整核对成功后会自动保留最近 30 次结果。</p></div>'; return; }
      const rows = state.history.map((item) => {
        const snapshotID = encodeURIComponent(item.id);
        const exportOptions = ["json", "csv", "html"].map((format) => `<a href="/api/snapshots/${snapshotID}/export?format=${format}" download>${format.toUpperCase()}</a>`).join("");
        const capturedAt = formatDateTime(item.capturedAt);
        return `<tr><td>${escapeHTML(capturedAt)}</td><td>${escapeHTML(item.accountHash)}</td><td>${escapeHTML(item.poolName)} · ${escapeHTML(item.poolVersion)}</td><td>${formatNumber(item.ownedCount)}</td><td>${formatNumber(item.remainingCount)}</td><td class="table-actions-cell"><nav class="history-export-options" aria-label="导出 ${escapeHTML(capturedAt)} 的历史记录">${exportOptions}</nav></td></tr>`;
      }).join("");
      const options = state.history.map((item) => `<option value="${escapeHTML(item.id)}">${escapeHTML(formatDateTime(item.capturedAt))} · ${escapeHTML(item.accountHash)}</option>`).join("");
      el.historyContent.innerHTML = `<div class="history-actions"><label class="select-wrap"><span class="sr-only">起始快照</span><select id="diff-from">${options}</select></label><span class="muted">对比</span><label class="select-wrap"><span class="sr-only">目标快照</span><select id="diff-to">${options}</select></label><button id="run-diff" class="text-button" type="button">比较变化</button></div><div class="table-scroll"><table class="data-table"><thead><tr><th>时间</th><th>脱敏账号</th><th>奖池</th><th>已拥有</th><th>剩余</th><th>操作</th></tr></thead><tbody>${rows}</tbody></table></div><div id="diff-result"></div>`;
      const from = el.historyContent.querySelector("#diff-from");
      const to = el.historyContent.querySelector("#diff-to");
      if (state.history.length > 1) to.selectedIndex = 1;
      el.historyContent.querySelector("#run-diff").addEventListener("click", () => compareHistory(from.value, to.value));
    } catch (error) { state.historyLoaded = false; renderPanelError(el.historyContent, "历史读取失败", error.message, loadHistory); }
  }

  async function compareHistory(fromID, toID) {
    const target = el.historyContent.querySelector("#diff-result");
    if (!target || fromID === toID) { showToast("请选择两个不同的快照"); return; }
    target.innerHTML = '<p class="muted">正在比较…</p>';
    try {
      const diff = await api(`/api/snapshots/${encodeURIComponent(fromID)}/diff?against=${encodeURIComponent(toID)}`, {}, "history-diff");
      const groups = [["新增拥有", diff.addedOwned], ["不再拥有", diff.removedOwned], ["新增剩余", diff.newRemaining], ["不再剩余", diff.noLongerRemaining]];
      target.innerHTML = `<div class="diff-block"><h3>变化结果</h3><div class="diff-columns">${groups.map(([label, items]) => `<section><h3>${label} · ${formatNumber(items?.length || 0)}</h3><ul>${(items || []).map((skin) => `<li>${escapeHTML(skin.name)}${skin.championName ? ` · ${escapeHTML(skin.championName)}` : ""}</li>`).join("") || "<li>没有变化</li>"}</ul></section>`).join("")}</div></div>`;
    } catch (error) { target.innerHTML = `<div class="notice is-error"><div><strong>无法比较</strong><p>${escapeHTML(error.message)}</p></div></div>`; }
  }

  async function loadPools() {
    el.poolsContent.innerHTML = '<p class="muted">正在读取奖池清单…</p>';
    try {
      const payload = await api("/api/pools", {}, "pools");
      state.pools = payload.items || [];
      state.poolsLoaded = true;
      el.poolPicker.replaceChildren(...state.pools.map((pool) => {
        const option = new Option(`${pool.name} · ${formatNumber(pool.entryCount)} 款`, pool.id);
        option.selected = Boolean(pool.selected);
        return option;
      }));
      const pickerWrap = el.poolPicker.closest(".pool-picker-wrap");
      if (pickerWrap) pickerWrap.hidden = state.pools.length <= 1;
      const rows = state.pools.map((pool) => `<tr><td>${escapeHTML(pool.name)}${pool.builtIn ? ' <span class="state-chip">内置</span>' : ""}</td><td>${escapeHTML(pool.version || "—")}</td><td>${formatNumber(pool.entryCount)}</td><td class="table-actions-cell">${pool.selected ? '<span class="state-chip success">当前使用</span>' : `<button class="text-button select-pool" type="button" data-pool-id="${escapeHTML(pool.id)}">使用这份清单</button>`}</td></tr>`).join("");
      el.poolsContent.innerHTML = `<div class="table-scroll"><table class="data-table"><thead><tr><th>清单</th><th>版本</th><th>皮肤</th><th>操作</th></tr></thead><tbody>${rows}</tbody></table></div>`;
      for (const button of el.poolsContent.querySelectorAll(".select-pool")) button.addEventListener("click", () => selectPool(button.dataset.poolId));
      const activePage = el.poolPageTabs.find((tab) => tab.classList.contains("is-active"))?.dataset.poolPage;
      if (activePage === "history") {
        if (!state.historyLoaded) await loadHistory();
      } else if (activePage === "catalog" || !activePage) {
        await loadPoolCatalog();
      }
    } catch (error) { state.poolsLoaded = false; renderPanelError(el.poolsContent, "奖池读取失败", error.message, loadPools); }
  }

  async function loadPoolCatalog() {
    state.poolError = "";
    if (!state.status?.connected || !state.status?.calculationOK) {
      state.poolItems = [];
      state.poolLoading = false;
      renderPoolCatalog();
      return;
    }
    state.poolLoading = true;
    renderPoolCatalog();
    try {
      const payload = await api("/api/pool-skins", {}, "pool-skins", 15000);
      state.poolItems = Array.isArray(payload.items) ? payload.items : [];
    } catch (error) {
      if (error.name === "RequestCancelled") return;
      state.poolItems = [];
      state.poolError = error.message;
    } finally {
      state.poolLoading = false;
      renderPoolCatalog();
    }
  }

  function visiblePoolItems() {
    const query = state.poolQuery.trim().toLocaleLowerCase("zh-CN");
    const items = state.poolItems.filter((skin) => {
      if (state.poolView === "owned" ? !skin.owned : skin.owned) return false;
      if (query && !`${skin.name} ${skin.championName || ""} ${skin.id}`.toLocaleLowerCase("zh-CN").includes(query)) return false;
      return state.poolQuality === "all" || poolRarityGroup(skin) === state.poolQuality;
    });
    items.sort((left, right) => {
      if (state.poolSort === "name") return localeCompare(left.name, right.name);
      if (state.poolSort === "champion") return localeCompare(left.championName, right.championName) || localeCompare(left.name, right.name);
      return comparePoolRarity(left, right);
    });
    return items;
  }

  function renderPoolCatalog() {
    cancelPoolRenderFrames();
    cancelDeferredImages(el.poolSkinGrid);
    const generation = ++state.poolRenderGeneration;
    el.poolSkinGrid.setAttribute("aria-busy", String(state.poolLoading));
    el.poolSkinGrid.classList.remove("is-grouped");
    if (state.poolLoading) {
      el.poolListMeta.textContent = "正在整理奖池皮肤…";
      el.poolSkinGrid.innerHTML = Array.from({ length: 6 }, () => '<div class="skeleton"></div>').join("");
      return;
    }
    if (!state.status?.connected) {
      el.poolListMeta.textContent = "";
      el.poolSkinGrid.innerHTML = '<div class="gameplay-empty"><span aria-hidden="true">⬡</span><strong>等待英雄联盟客户端</strong><p>登录并进入大厅后，三合一奖池目录、清单上传与本地历史会自动开放。</p></div>';
      return;
    }
    if (!state.status?.calculationOK) {
      el.poolListMeta.textContent = "奖池正在准备";
      el.poolSkinGrid.innerHTML = '<div class="empty-state"><strong>奖池暂时不可用</strong><p>完成本次读取后会自动显示。</p></div>';
      return;
    }
    if (state.poolError) {
      el.poolListMeta.textContent = "奖池读取失败";
      el.poolSkinGrid.innerHTML = `<div class="empty-state"><strong>无法读取奖池目录</strong><p>${escapeHTML(state.poolError)}</p><button class="text-button pool-retry" type="button">重试</button></div>`;
      el.poolSkinGrid.querySelector(".pool-retry")?.addEventListener("click", loadPoolCatalog);
      return;
    }
    const visible = visiblePoolItems();
    const total = state.poolItems.filter((skin) => state.poolView === "owned" ? skin.owned : !skin.owned).length;
    el.poolListMeta.textContent = `${state.poolView === "owned" ? "已拥有" : "未拥有"} ${formatNumber(visible.length)} 款${visible.length !== total ? ` · 共 ${formatNumber(total)} 款` : ""}`;
    el.poolSkinGrid.replaceChildren();
    if (!visible.length) {
      el.poolSkinGrid.innerHTML = '<div class="empty-state"><strong>没有符合条件的皮肤</strong><p>可以调整搜索或品质筛选。</p></div>';
      return;
    }
    el.poolSkinGrid.classList.add("is-grouped");
    const groups = new Map();
    for (const skin of visible) {
      const name = qualityLabels[poolRarityGroup(skin)] || "未分级";
      if (!groups.has(name)) groups.set(name, []);
      groups.get(name).push(skin);
    }
    const displayOrder = Array.from(groups.values()).flat();
    const tasks = [];
    for (const [name, skins] of groups) {
      const section = document.createElement("section");
      section.className = "champion-group rarity-group";
      section.innerHTML = `<header class="champion-heading"><div><h2>${escapeHTML(name)}</h2><span>${formatNumber(skins.length)} 款</span></div></header><div class="champion-skins"></div>`;
      const grid = section.querySelector(".champion-skins");
      el.poolSkinGrid.append(section);
      for (const skin of skins) tasks.push(() => grid.append(makeSkinCard(skin, { locked: !skin.owned, poolState: !skin.owned, detailItems: displayOrder })));
    }
    runRenderTasks(tasks, generation, "poolRenderGeneration");
  }

  async function selectPool(id) {
    try {
      showReadingOverlay("正在切换奖池", "正在重新整理奖池与收藏，请稍候。", true);
      await api("/api/pools/select", { method: "POST", body: JSON.stringify({ id }) }, "pool-select");
      showToast("奖池已切换，正在重新读取");
      await refreshStatus(true);
      await loadPools();
    } catch (error) { showToast(error.message); }
  }

  async function loadPrivacy() {
    el.privacyContent.innerHTML = '<p class="muted">正在读取隐私说明…</p>';
    try {
      const data = await api("/api/privacy", {}, "privacy");
      const group = (title, values) => `<section><h3>${title}</h3><ul>${values.map((value) => `<li>${escapeHTML(value)}</li>`).join("")}</ul></section>`;
      el.privacyContent.innerHTML = `<div class="stat-grid"><div class="stat"><span>账号数据处理</span><strong>${data.localOnly ? "仅限本机" : "包含网络服务"}</strong></div><div class="stat"><span>账号密码</span><strong>${data.requiresPassword ? "需要" : "不需要"}</strong></div><div class="stat"><span>收藏数据上传</span><strong>${data.uploadsData ? "会上传" : "不会上传"}</strong></div></div><div class="privacy-list">${group("默认读取", data.reads || [])}${group("明确点击后写入客户端", data.explicitWrites || [])}${group("设置开启后自动写入客户端", data.automaticWrites || [])}${group("外部读取", data.externalReads || [])}${group("本地保存", data.stores || [])}${group("绝不保存", data.neverStores || [])}</div>`;
    } catch (error) { renderPanelError(el.privacyContent, "隐私说明读取失败", error.message, loadPrivacy); }
  }

  function renderPanelError(container, title, message, retry) {
    container.innerHTML = `<div class="empty-state"><strong>${escapeHTML(title)}</strong><p>${escapeHTML(message)}</p><button class="text-button panel-retry" type="button">重试</button></div>`;
    container.querySelector(".panel-retry")?.addEventListener("click", retry);
  }

  function activateTab(tab, tabs, activate) {
    for (const item of tabs) {
      const active = item === tab;
      item.classList.toggle("is-active", active);
      item.setAttribute("aria-selected", String(active));
      item.tabIndex = active ? 0 : -1;
    }
    activate(tab);
  }

  function setupTabKeyboard(tabs) {
    for (const tab of tabs) tab.addEventListener("keydown", (event) => {
      if (!["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const current = tabs.indexOf(tab);
      const forward = event.key === "ArrowRight" || event.key === "ArrowDown";
      const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (forward ? 1 : -1) + tabs.length) % tabs.length;
      tabs[next].focus();
      tabs[next].click();
    });
  }

  function activateSection(name) {
    const tab = el.sectionTabs.find((item) => item.dataset.section === name);
    if (!tab) return;
    const previousSection = state.section;
    if (name !== "favorites" && state.section === "favorites" && state.favoritesPage === "collection") cancelDeferredImages(el.grid);
    state.section = name;
    const sectionTitles = { overview: ["总览", "召唤师生涯与最近对局"], champions: ["英雄", "韩服梯度、符文与构建推荐"], live: ["对局", "实时队伍与赛前配置"], favorites: ["收藏", "皮肤、物品与三合一奖池"], settings: ["设置", "显示、对局行为与隐私"] };
    const title = sectionTitles[name] || ["Deep Legends", "战绩 · 英雄 · 对局 · 收藏"];
    el.currentSectionTitle.textContent = title[0];
    el.topbarSubtitle.textContent = title[1];
    el.pageIntro.hidden = name !== "favorites";
    const applyPanels = () => activateTab(tab, el.sectionTabs, (selected) => {
      for (const panel of el.sectionPanels) panel.hidden = panel.id !== selected.getAttribute("aria-controls");
    });
    if (previousSection !== name && typeof document.startViewTransition === "function" && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      document.startViewTransition(applyPanels);
    } else {
      applyPanels();
    }
    // 离开后重新进入的页面一律还原到默认页签与筛选项。
    if (name === "favorites") {
      const fresh = previousSection !== "favorites";
      if (fresh) resetPoolControls();
      activateFavoritesPage(fresh ? "collection" : state.favoritesPage, fresh);
    }
    if (name === "settings") {
      if (previousSection !== "settings") activateSettingsPage("appearance");
      loadPrivacy();
    }
    if (state.status) renderNotice(state.status);
    if (state.status) renderLaunchpad(state.status);
    window.dispatchEvent(new CustomEvent("deep-legends:section", { detail: { name } }));
  }

  window.addEventListener("deep-legends:navigate", (event) => {
    const name = event.detail?.section;
    if (name) activateSection(name);
  });

  // 总览页切换玩家页签时同步启动入口卡的可见性（只对当前召唤师展示）。
  window.addEventListener("deep-legends:overview-tab", (event) => {
    state.overviewTabIsCurrent = Boolean(event.detail?.current);
    if (state.status) renderLaunchpad(state.status);
  });

  // 离开收藏页后还原奖池子页的全部页签与筛选项（不触发数据加载）。
  function resetPoolControls() {
    state.poolView = "owned";
    state.poolQuery = "";
    state.poolQuality = "all";
    state.poolSort = "rarity";
    el.poolSearch.value = "";
    el.poolQuality.value = "all";
    el.poolSort.value = "rarity";
    const ownedTab = el.poolViewTabs.find((item) => item.dataset.poolView === "owned");
    if (ownedTab) activateTab(ownedTab, el.poolViewTabs, () => {});
    const catalogTab = el.poolPageTabs.find((item) => item.dataset.poolPage === "catalog");
    if (catalogTab) activateTab(catalogTab, el.poolPageTabs, () => {
      el.poolCatalogPanel.hidden = false;
      el.poolUploadPanel.hidden = true;
      el.poolHistoryPanel.hidden = true;
    });
  }

  function activateFavoritesPage(name, returning = false) {
    const tab = el.favoritesTabs.find((item) => item.dataset.favoritesPage === name);
    if (!tab) return;
    const previous = state.favoritesPage;
    state.favoritesPage = name;
    activateTab(tab, el.favoritesTabs, (selected) => {
      for (const panel of [el.favoritesCollectionPanel, el.favoritesAccountPanel, el.favoritesPoolsPanel]) panel.hidden = panel.id !== selected.getAttribute("aria-controls");
    });
    const subtitles = { collection: "皮肤与炫彩收藏", account: "账户、战利品与待领取奖励", pools: "奖池目录、清单上传与本地历史" };
    if (state.section === "favorites") el.topbarSubtitle.textContent = subtitles[name] || "收藏工具";
    el.pageIntro.hidden = state.section !== "favorites";
    if (name === "collection" && (returning || previous !== "collection")) {
      const ownedTab = el.viewTabs.find((item) => item.dataset.view === "owned");
      resetCollectionControls("owned");
      if (ownedTab) activateTab(ownedTab, el.viewTabs, () => {});
      loadSkins();
    }
    if (name === "account" && !state.accountLoaded) loadAccount();
    if (name === "pools" && !state.poolsLoaded) loadPools();
    if (state.status) renderNotice(state.status);
  }

  function resetCollectionControls(view) {
    state.view = view;
    state.query = "";
    state.qualitySelections.clear();
    state.sort = ["all", "chromas"].includes(view) ? "mastery" : "acquired";
    state.descending = defaultDescending(state.sort);
    state.acquisitionAvailable = null;
    state.acquisitionFallback = false;
    state.showUnownedChromas = true;
    state.showPrestigeChromas = true;
    el.search.value = "";
    el.showUnownedChromas.checked = true;
    el.showPrestigeChromas.checked = true;
    el.grid.classList.remove("hide-unowned");
    el.rarityMenu.hidden = true;
    el.rarityButton.setAttribute("aria-expanded", "false");
    updateQualityMenu();
    configureSortControls();
  }

  function activateSettingsPage(name) {
    const tab = el.settingsTabs.find((item) => item.dataset.settingsPage === name);
    if (!tab) return;
    state.settingsPage = name;
    activateTab(tab, el.settingsTabs, (selected) => {
      for (const panel of el.settingsPanels) panel.hidden = panel.id !== selected.getAttribute("aria-controls");
    });
  }

  function configureSortControls() {
    const allView = state.view === "all";
    const chromaView = state.view === "chromas";
    const options = chromaView
      ? [["mastery", "按英雄熟练度排序"], ["rarity", "按品质排序"], ["chromaCount", "按炫彩数量排序"], ["acquired", "按获取时间排序"]]
      : allView
      ? [["mastery", "按熟练度排序"], ["skinCount", "按皮肤数量排序"], ["champion", "按英雄名排序"], ["rarity", "按品质排序"], ["acquired", "按获取时间排序"]]
      : [["acquired", "按获取时间排序"], ["champion", "按英雄排序"], ["name", "按皮肤名排序"], ["rarity", "按品质排序"], ["id", "按皮肤 ID 排序"]];
    if (!options.some(([value]) => value === state.sort)) {
      state.sort = allView || chromaView ? "mastery" : "acquired";
    }
    el.sort.replaceChildren(...options.map(([value, label]) => {
      const option = new Option(value === "acquired" && state.acquisitionAvailable === false ? `${label}（客户端未提供）` : label, value);
      option.disabled = value === "acquired" && state.acquisitionAvailable === false;
      return option;
    }));
    el.sort.value = state.sort;
    el.sortMenu.replaceChildren(...options.map(([value, label]) => {
      const button = document.createElement("button");
      button.type = "button";
      button.role = "menuitemradio";
      button.dataset.sort = value;
      button.disabled = value === "acquired" && state.acquisitionAvailable === false;
      button.setAttribute("aria-checked", String(value === state.sort));
      button.textContent = button.disabled ? `${label}（客户端未提供）` : label;
      return button;
    }));
    el.sortLabel.textContent = options.find(([value]) => value === state.sort)?.[1] || "选择排序方式";
    el.sortMenu.hidden = true;
    el.sortButton.setAttribute("aria-expanded", "false");
    el.sortDirection.hidden = allView && !["acquired", "rarity"].includes(state.sort);
    el.chromaUnownedControl.hidden = !chromaView;
    el.chromaPrestigeControl.hidden = !chromaView;
    el.sortDirection.textContent = state.descending ? "降序" : "升序";
    el.sortDirection.setAttribute("aria-pressed", String(state.descending));
  }

  function defaultDescending(sort) {
    return ["acquired", "rarity", "mastery", "skinCount", "chromaCount"].includes(sort);
  }

  // 可选主题：除自动与浅色外都是深色系配色，颜色全部由 CSS 变量控制。
  // 背景保持中性深色，主题色只体现在主色与选中态上。
  const THEME_OPTIONS = ["auto", "light", "dark", "azure", "emerald", "violet", "crimson", "aurora", "oled"];
  const THEME_BACKGROUNDS = { light: "#ffffff", dark: "#0B0E14", azure: "#0A0F16", emerald: "#0B0E12", violet: "#0C0D14", crimson: "#0E0C0D", aurora: "#0A0F12", oled: "#000000" };

  function applyAppearance() {
    const stored = preference("theme", "dark");
    const theme = THEME_OPTIONS.includes(stored) ? stored : "dark";
    const density = preference("density", "comfortable");
    const now = new Date();
    const effectiveTheme = resolvedTheme(theme, now);
    document.documentElement.dataset.theme = effectiveTheme;
    document.querySelector('meta[name="theme-color"]')?.setAttribute("content", THEME_BACKGROUNDS[effectiveTheme] || "#0B0E14");
    // 桌面外壳只区分明暗两种标题栏。
    window.desktopTheme?.setTheme?.(effectiveTheme === "light" ? "light" : "dark");
    clearTimeout(state.themeTimer);
    if (theme === "auto") {
      const nextBoundary = new Date(now);
      if (now.getHours() < 6) nextBoundary.setHours(6, 0, 0, 0);
      else if (now.getHours() < 18) nextBoundary.setHours(18, 0, 0, 0);
      else { nextBoundary.setDate(nextBoundary.getDate() + 1); nextBoundary.setHours(6, 0, 0, 0); }
      state.themeTimer = setTimeout(() => applyAppearance(), Math.max(1000, nextBoundary.getTime() - now.getTime() + 250));
    }
    document.documentElement.dataset.density = density;
    if (el.settingTheme) el.settingTheme.value = theme;
    el.densityToggle.textContent = density === "compact" ? "切换为舒适" : "切换为紧凑";
    el.densityToggle.setAttribute("aria-pressed", String(density === "compact"));
  }

  function resolvedTheme(theme, date = new Date()) {
    if (theme !== "auto") return theme;
    const hour = date.getHours();
    return hour >= 6 && hour < 18 ? "light" : "dark";
  }

  function applySidebar() {
    const collapsed = preference("sidebar", "expanded") === "collapsed";
    document.documentElement.dataset.sidebar = collapsed ? "collapsed" : "expanded";
    el.sidebarToggle.setAttribute("aria-expanded", String(!collapsed));
    el.sidebarToggle.setAttribute("aria-label", collapsed ? "展开侧边栏" : "收起侧边栏");
    el.settingsSidebarToggle.textContent = collapsed ? "展开侧边栏" : "收起侧边栏";
  }

  function toggleSidebar() {
    savePreference("sidebar", preference("sidebar", "expanded") === "collapsed" ? "expanded" : "collapsed");
    applySidebar();
  }

  function toggleDensity() {
    savePreference("density", preference("density", "comfortable") === "compact" ? "comfortable" : "compact");
    applyAppearance();
  }

  function renderChampionNetwork(status) {
    if (!status) return;
    el.settingProxyMode.value = status.mode || "auto";
    el.settingProxyUrl.value = status.url || "";
    el.settingProxyUrlWrap.hidden = el.settingProxyMode.value !== "manual";
    el.settingProxyState.textContent = `当前：${status.active || "未检测到代理，将直接连接"}`;
  }

  async function setupChampionNetwork() {
    el.settingProxyMode.addEventListener("change", () => {
      el.settingProxyUrlWrap.hidden = el.settingProxyMode.value !== "manual";
      if (el.settingProxyMode.value !== "manual") void saveChampionNetwork();
      else el.settingProxyUrl.focus();
    });
    el.settingProxySave.addEventListener("click", () => void saveChampionNetwork());
    el.settingProxyUrl.addEventListener("keydown", (event) => {
      if (event.key === "Enter") { event.preventDefault(); void saveChampionNetwork(); }
    });
    try {
      renderChampionNetwork(await api("/api/champions/network", {}, "champion-network", 5000));
    } catch (error) {
      el.settingProxyState.textContent = `读取失败：${error.message}`;
    }
  }

  async function saveChampionNetwork() {
    const mode = el.settingProxyMode.value;
    const url = el.settingProxyUrl.value.trim();
    if (mode === "manual" && !url) {
      el.settingProxyState.textContent = "请填写代理地址";
      el.settingProxyUrl.focus();
      return;
    }
    el.settingProxySave.disabled = true;
    el.settingProxyState.textContent = "正在应用…";
    try {
      const status = await api("/api/champions/network", { method: "POST", body: JSON.stringify({ mode, url }) }, "champion-network-save", 7000);
      renderChampionNetwork(status);
      showToast("英雄数据网络设置已保存");
    } catch (error) {
      el.settingProxyState.textContent = `保存失败：${error.message}`;
    } finally {
      el.settingProxySave.disabled = false;
    }
  }

  // 本地服务不可达（或会话失效）时的全局兜底：隐藏页面主体与筛选控件，
  // 只保留一张与各页面空状态同风格的居中提示卡；恢复由 renderStatus 完成。
  function showFatal(message) {
    el.connection.className = "connection is-error";
    el.connection.lastElementChild.textContent = "本地服务异常";
    const expired = /会话已过期/.test(String(message || ""));
    el.notice.className = "notice is-fatal";
    el.notice.hidden = false;
    el.notice.innerHTML = `<div class="notice-symbol" aria-hidden="true">!</div><div><strong>${expired ? "需要重新连接本地助手" : "无法连接本地助手"}</strong><p>${escapeHTML(expired ? "本地服务重启后页面授权已失效，重新连接即可继续使用，收藏与设置都不会丢失。" : message)}</p></div><button class="text-button" type="button" data-fatal-reload>重新连接</button>`;
    el.notice.querySelector("[data-fatal-reload]")?.addEventListener("click", () => location.reload());
    el.clientLaunchpad.hidden = true;
    updateWorkspaceAvailability({ connected: false });
    document.body.classList.add("is-fatal");
    hideReadingOverlay();
  }

  let toastTimer = 0;
  function showToast(message) { clearTimeout(toastTimer); el.toast.textContent = message; el.toast.hidden = false; toastTimer = setTimeout(() => { el.toast.hidden = true; }, 3200); }
  function localeCompare(left, right) { return String(left || "").localeCompare(String(right || ""), "zh-CN", { numeric: true, sensitivity: "base" }); }
  const rarityKeys = { "卓越": "transcendent", "圣堂": "exalted", "神话": "mythic", "终极": "ultimate", "传说": "legendary", "限定": "limited", "史诗": "epic", "王者": "royal", "勇士": "brave", "典藏": "archive", "未分级": "unranked" };
  const rarityRanks = { transcendent: 0, exalted: 1, mythic: 2, ultimate: 3, legendary: 4, limited: 5, epic: 6, royal: 7, brave: 8, archive: 9, unranked: 10 };
  const mythicSubRanks = { "神话幻想": 0, "殿堂系列": 1, "总决赛FMVP系列": 2, "战队系列": 3, "至臻": 4, "海克斯系列": 5, "水晶系列": 6, "灰烬系列": 7, "周年系列": 8, "强行神话": 9 };
  const qualityLabels = { transcendent: "卓越", exalted: "圣堂", mythic: "神话", ultimate: "终极", legendary: "传说", limited: "限定", epic: "史诗", royal: "王者", brave: "勇士", archive: "典藏", unranked: "未分级" };
  function rarityGroup(skin = {}) { if (skin.rarityTier && rarityKeys[skin.rarityTier]) return rarityKeys[skin.rarityTier]; const text = String(skin.rarity || skin).toLocaleLowerCase("en-US"); if (text.includes("exalted")) return "exalted"; if (text.includes("transcendent")) return "transcendent"; if (text.includes("mythic")) return "mythic"; if (text.includes("ultimate")) return "ultimate"; if (text.includes("legend")) return "legendary"; if (text.includes("epic")) return "epic"; return "unranked"; }
  function readableRarity(skin) { return skin?.rarityTier || qualityLabels[rarityGroup(skin)] || "未分级"; }
  function compareRarity(left, right) { return (rarityRanks[rarityGroup(left)] ?? 99) - (rarityRanks[rarityGroup(right)] ?? 99) || (mythicSubRanks[left.raritySubtier] ?? 99) - (mythicSubRanks[right.raritySubtier] ?? 99) || localeCompare(left.raritySubtier || "", right.raritySubtier || ""); }
  function poolRarityGroup(skin) {
    const id = Number(skin?.id);
    if (id === 13001 || id === 1010) return "limited";
    return rarityGroup(skin);
  }
  function comparePoolRarity(left, right) {
    const ranks = { limited: 0, legendary: 1, epic: 2, royal: 3, brave: 4, archive: 5, unranked: 6 };
    return (ranks[poolRarityGroup(left)] ?? 99) - (ranks[poolRarityGroup(right)] ?? 99) || (releaseTime(left) - releaseTime(right)) || Number(left.id) - Number(right.id) || localeCompare(left.name, right.name);
  }
  function qualityToken(rarity, subtier = "") { return subtier ? `${rarity}:${subtier}` : rarity; }
  function qualitySelectionMatches(skin) {
    if (!state.qualitySelections.size) return true;
    const rarity = rarityGroup(skin);
    return state.qualitySelections.has(rarity) || (rarity === "mythic" && state.qualitySelections.has(qualityToken(rarity, skin.raritySubtier || "")));
  }
  function qualitySelectionLabel(token) {
    const separator = token.indexOf(":");
    return separator < 0 ? qualityLabels[token] || token : `神话 · ${token.slice(separator + 1)}`;
  }
  function updateQualityMenu() {
    for (const option of el.rarityMenu.querySelectorAll("[data-rarity]")) {
      const rarity = option.dataset.rarity;
      const token = qualityToken(rarity, option.dataset.subtier || "");
      const checked = rarity === "all" ? state.qualitySelections.size === 0 : state.qualitySelections.has(token);
      option.setAttribute("aria-checked", String(checked));
    }
    const selected = [...state.qualitySelections];
    el.rarityButton.textContent = selected.length === 0 ? "全部品质" : selected.length === 1 ? qualitySelectionLabel(selected[0]) : `已选 ${selected.length} 项`;
  }
  function toggleQualitySelection(option) {
    const rarity = option.dataset.rarity;
    const subtier = option.dataset.subtier || "";
    if (rarity === "all") {
      state.qualitySelections.clear();
    } else {
      const token = qualityToken(rarity, subtier);
      if (rarity === "mythic" && !subtier) {
        for (const selected of [...state.qualitySelections]) if (selected.startsWith("mythic:")) state.qualitySelections.delete(selected);
      } else if (rarity === "mythic" && subtier) {
        state.qualitySelections.delete("mythic");
      }
      if (state.qualitySelections.has(token)) state.qualitySelections.delete(token); else state.qualitySelections.add(token);
    }
    updateQualityMenu();
    renderItems();
  }
  function releaseTime(skin) { const value = Date.parse(skin?.releaseDate || ""); return Number.isFinite(value) ? value : 0; }
  function sourceStateLabel(value) { return ({ success: "已验证", available: "可用", warning: "口径差异", conflict: "证据冲突", unsupported: "不支持", failed: "失败", invalid: "无效" })[value] || value; }
  function discoveryResultLabel(value) { return ({ connected: "已连接", searching: "正在探测", "process-not-found": "未发现进程", "credentials-unreadable": "凭据不可读", "probe-failed": "接口未就绪", "process-query-failed": "进程查询失败", unsupported: "系统不支持" })[value] || value || "尚未探测"; }
  function capabilityName(value) { return ({ "summoner-profile": "玩家主页资料", "player-loot": "客户端战利品", "sanctum-sparks": "圣堂花火", "pending-rewards": "待领取奖励", "owned-champions": "已拥有英雄", "owned-chromas": "已拥有炫彩", "skin-acquisition-time": "皮肤获取时间", "champion-mastery": "英雄熟练度" })[value] || value || "未知能力"; }
  function lootToken(item) { return String(item?.lootId || item?.lootName || "").trim().replace(/[\s-]+/g, "_").toUpperCase(); }
  function lootName(item) { return item.displayName || item.localizedName || item.lootName || item.lootId || "未识别材料"; }
  function lootTypeLabel(item) {
    const type = String(item?.type || item?.displayCategories || "").toUpperCase();
    if (item?.kind) return item.kind;
    return ({ SKIN_RENTAL: "皮肤碎片", CHAMPION_SKIN: "完整皮肤", SKIN_SHARD: "皮肤碎片", CHAMPION: "英雄碎片", CURRENCY: "精粹", MATERIAL: "材料", CHEST: "宝箱", STATSTONE_SHARD: "永恒星碑碎片" })[type] || item?.category || "战利品";
  }
  function lootImagePaths(item) {
    const token = lootToken(item);
    const fixed = {
      CURRENCY_CHAMPION: "/loot-icons/blue-essence.png",
      CURRENCY_COSMETIC: "/loot-icons/orange-essence.png",
      MATERIAL_KEY: "/loot-icons/hextech-key.png",
      MATERIAL_KEY_FRAGMENT: "/loot-icons/key-fragment.png",
      CHEST_CHAMPION_MASTERY: "/loot-icons/hextech-chest-transparent.png",
      CURRENCY_ANCIENT_SPARK: "/loot-icons/sanctum-spark.svg",
    };
    return [...new Set([fixed[token], item.tilePath, item.asset, item.splashPath].filter(Boolean))];
  }
  function loadNextLootImage(image, first = false) {
    if (first) {
      image.dataset.index = "0";
      image.addEventListener("error", () => loadNextLootImage(image));
      image.addEventListener("load", () => image.closest(".loot-art")?.classList.add("has-image"));
    }
    const paths = decodeURIComponent(image.dataset.paths || "").split("\n").filter(Boolean);
    const index = Number(image.dataset.index || 0);
    if (index >= paths.length) { image.hidden = true; image.closest(".loot-art")?.classList.remove("has-image"); return; }
    image.hidden = false;
    image.dataset.index = String(index + 1);
    image.src = paths[index].startsWith("/loot-icons/") ? paths[index] : `/api/image?path=${encodeURIComponent(paths[index])}`;
  }
  function lootCard(item) {
    const category = item.category || "材料";
    const slug = lootCategorySlug(category);
    const tokenClass = lootToken(item).toLocaleLowerCase("en-US").replace(/[^a-z0-9]+/g, "-");
    const paths = lootImagePaths(item);
    const art = paths.length ? `<img data-paths="${encodeURIComponent(paths.join("\n"))}" alt="" loading="lazy" decoding="async">` : "";
	const ownership = category === "皮肤" && item.skinOwnedKnown
	  ? `<small class="loot-ownership ${item.skinOwned ? "is-owned" : "is-unowned"}">${item.skinOwned ? "已拥有" : "可升级"}</small>`
      : "";
    const description = category === "皮肤" ? "" : (item.localizedDescription || "");
	const essenceIcon = '<img src="/loot-icons/orange-essence.png" alt="橙色精粹">';
	const canUpgrade = !item.skinOwnedKnown || !item.skinOwned;
	const essence = category === "皮肤" && (Number(item.disenchantValue) > 0 || (canUpgrade && Number(item.upgradeEssenceValue) > 0))
	  ? `<div class="loot-essence-values">${Number(item.disenchantValue) > 0 ? `<span>分解 <b>+ ${formatNumber(item.disenchantValue)}</b>${essenceIcon}</span>` : ""}${canUpgrade && Number(item.upgradeEssenceValue) > 0 ? `<span>升级 <b>− ${formatNumber(item.upgradeEssenceValue)}</b>${essenceIcon}</span>` : ""}</div>`
	  : "";
    return `<article class="loot-card category-${slug}${tokenClass ? ` loot-${tokenClass}` : ""}"><span class="loot-art" aria-hidden="true"><span>${lootCategoryIcon(category)}</span>${art}</span><div class="loot-card-copy"><span>${escapeHTML(lootTypeLabel(item))}</span><strong>${escapeHTML(lootName(item))}</strong>${ownership}${essence}${description ? `<small>${escapeHTML(description)}</small>` : ""}</div><div class="loot-card-end"><b>× ${formatNumber(item.count)}</b></div></article>`;
  }
  function lootCategorySlug(category) { return ({ "材料": "material", "英雄": "champion", "皮肤": "skin", "小小英雄": "companion", "永恒星碑": "eternal", "表情": "emote", "守卫": "ward", "图标": "icon" })[category] || "material"; }
  function lootCategoryIcon(category) {
    const paths = {
      "材料": '<path d="M6 3h12l3 5-3 13H6L3 8l3-5Zm2 3L6 9l2 9h8l2-9-2-3H8Zm1 4h6v2H9v-2Z"/>',
      "英雄": '<path d="m12 2 7 4v6c0 4.7-3 8-7 10-4-2-7-5.3-7-10V6l7-4Zm0 3L8 7.3v4.5c0 3 1.6 5.3 4 6.9 2.4-1.6 4-3.9 4-6.9V7.3L12 5Zm-3 4h6l-1 6h-4L9 9Z"/>',
      "皮肤": '<path d="M4 5.5 8.5 3 12 5l3.5-2L20 5.5V13c0 4.4-3.7 7.3-8 9-4.3-1.7-8-4.6-8-9V5.5Zm3 2V13c0 2.6 2 4.5 5 5.9 3-1.4 5-3.3 5-5.9V7.5l-1.5-.8L12 9 8.5 6.7 7 7.5Zm1.5 3.5 2 1 1.5-1.5 1.5 1.5 2-1v3.5l-3.5 2-3.5-2V11Z"/>',
      "小小英雄": '<path d="M12 2c4.4 0 8 3.1 8 7 0 2.3-1.2 4.2-3.1 5.5L18 20l-4-2-2 4-2-4-4 2 1.1-5.5A6.7 6.7 0 0 1 4 9c0-3.9 3.6-7 8-7Zm-3 6a1.4 1.4 0 1 0 0 2.8A1.4 1.4 0 0 0 9 8Zm6 0a1.4 1.4 0 1 0 0 2.8A1.4 1.4 0 0 0 15 8Zm-6 5c.8 1.2 1.8 1.8 3 1.8s2.2-.6 3-1.8H9Z"/>',
      "永恒星碑": '<path d="M13 2c.3 3-1.4 4.6-2.9 6.1C8.5 9.7 7 11.2 7 14a5 5 0 0 0 10 0c0-2.1-.9-4.2-2.7-6.4.2 2.6-.6 4.1-2.3 5.4.4-3.6-1.2-6.1 1-11Zm-1 18a3 3 0 0 1-3-3c0-1.3.7-2.4 1.7-3.5.2 1.7.8 2.8 1.8 3.5.6-1 .9-2 .7-3.4 1.1 1.2 1.8 2.3 1.8 3.4a3 3 0 0 1-3 3Z"/>',
      "表情": '<path d="M12 2a10 10 0 1 1 0 20 10 10 0 0 1 0-20ZM8.5 8A1.5 1.5 0 1 0 8.5 11a1.5 1.5 0 0 0 0-3Zm7 0a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Zm-8 6c.9 2 2.4 3 4.5 3s3.6-1 4.5-3h-9Z"/>',
      "守卫": '<path d="M12 2 7 7v5l-3 4 5 1 3 5 3-5 5-1-3-4V7l-5-5Zm0 4 2 2v5l1 1.5-2 .5-1 2-1-2-2-.5 1-1.5V8l2-2Z"/>',
      "图标": '<path d="M12 2a7 7 0 0 1 4.7 12.2A9.5 9.5 0 0 1 21 22H3a9.5 9.5 0 0 1 4.3-7.8A7 7 0 0 1 12 2Zm0 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8Zm0 11c-2.4 0-4.5 1.1-5.8 3h11.6A7 7 0 0 0 12 16Z"/>',
    };
    return `<svg viewBox="0 0 24 24" focusable="false">${paths[category] || paths["材料"]}</svg>`;
  }
  function playerName(summoner) { const name = summoner.gameName || summoner.displayName || "当前召唤师"; return `${name}${summoner.tagLine ? `#${summoner.tagLine}` : ""}`; }
  function rewardStatusLabel(value) { return ({ PENDING_SELECTION: "待选择", PENDING: "待领取", CREATED: "待处理" })[String(value || "").toUpperCase()] || value || "待处理"; }
  function formatNumber(value) { const number = Number(value); return Number.isFinite(number) ? new Intl.NumberFormat("zh-CN").format(number) : "—"; }
  function formatDateTime(value) { const date = new Date(value); return Number.isNaN(date.valueOf()) ? "时间未知" : new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(date); }
  function shortHash(value = "") { return value ? `${value.slice(0, 12)}…` : "—"; }
  function escapeHTML(value) { const node = document.createElement("span"); node.textContent = String(value ?? ""); return node.innerHTML; }
  function safeHTTPURL(value) { try { const parsed = new URL(value); return parsed.protocol === "https:" || parsed.protocol === "http:"; } catch (_) { return false; } }

  el.refresh.addEventListener("click", async () => {
    el.refresh.disabled = true;
    el.refresh.classList.add("is-loading");
    showReadingOverlay("正在重新读取", "正在更新皮肤、炫彩与账户物品…", true);
    try { await api("/api/refresh", { method: "POST" }, "refresh"); showToast("正在重新读取客户端"); setTimeout(() => refreshStatus(true), 450); }
    catch (error) { showFatal(error.message); }
  });
  el.quit.addEventListener("click", async () => { if (!confirm("退出 Deep Legends？")) return; state.destroyed = true; clearTimeout(state.statusTimer); clearTimeout(state.liveUpdateTimer); state.eventSource?.close(); for (const controller of state.controllers.values()) controller.abort(); state.controllers.clear(); try { await fetch("/api/quit", { method: "POST" }); } catch (_) {} document.body.innerHTML = '<main class="shell"><section class="empty-state"><strong>Deep Legends 已退出</strong><p>现在可以关闭这个页面。</p></section></main>'; });
  el.settingTheme.addEventListener("change", () => { savePreference("theme", el.settingTheme.value); applyAppearance(); });
  el.densityToggle.addEventListener("click", toggleDensity);

  /* ---------- 顶部召唤师搜索：名称 + # 编号，支持整段粘贴自动拆分 ---------- */
  function splitRiotID(raw) {
    const value = String(raw || "").trim();
    const hashIndex = value.indexOf("#");
    if (hashIndex < 0) return null;
    return { name: value.slice(0, hashIndex).trim(), tag: value.slice(hashIndex + 1).replace(/#/g, "").trim() };
  }

  function absorbPastedRiotID(input) {
    const parts = splitRiotID(input.value);
    if (!parts) return false;
    el.playerSearchName.value = parts.name;
    if (parts.tag || input === el.playerSearchTag) el.playerSearchTag.value = parts.tag;
    const target = parts.tag ? el.playerSearchGo : el.playerSearchTag;
    el.playerSearchTag.focus();
    if (target === el.playerSearchGo) el.playerSearchTag.select();
    return true;
  }

  function searchRegion() { return el.playerSearchRegion.dataset.region === "kr" ? "kr" : ""; }
  function searchServerID() { return searchRegion() === "kr" ? "" : (el.playerSearchRegion.dataset.serverId || ""); }

  function submitPlayerSearch() {
    absorbPastedRiotID(el.playerSearchName);
    const region = searchRegion();
    const serverId = searchServerID();
    const gameName = el.playerSearchName.value.trim();
    const tagLine = el.playerSearchTag.value.replace(/#/g, "").trim();
    if (!gameName) { showToast("请填写玩家名称"); el.playerSearchName.focus(); return; }
    // 韩服允许只填名称：后端会用 OP.GG 自动补全出当前编号再查询。
    if (!tagLine && region !== "kr") { showToast("请填写 # 后的编号，例如 12345"); el.playerSearchTag.focus(); return; }
    if (region !== "kr" && !serverId && !state.status?.connected) {
      showToast("“跟随客户端”需要英雄联盟客户端正在运行");
      el.playerSearchRegion.focus();
      return;
    }
    if (region !== "kr" && !state.status?.connected) {
      showToast("国服查询需要本机英雄联盟客户端在运行");
      return;
    }
    if (region !== "kr" && !serverId && !state.status?.serverId) {
      showToast("无法识别当前客户端服务器，请手动选择国服服务器");
      el.playerSearchRegion.focus();
      return;
    }
    window.dispatchEvent(new CustomEvent("deep-legends:open-player", { detail: { gameName, tagLine, region, serverId, source: "search" } }));
  }

  /* 自定义服务器下拉：一级选择区域，二级选择国服服务器。 */
  function setCNRegionExpanded(expanded) {
    el.playerSearchCnToggle.setAttribute("aria-expanded", String(expanded));
    el.playerSearchCnOptions.hidden = !expanded;
  }

  function updateSearchRegionLabel(data = state.status) {
    if (el.playerSearchRegion.dataset.region === "kr") {
      el.playerSearchRegionLabel.textContent = "韩服";
      return;
    }
    const serverId = el.playerSearchRegion.dataset.serverId || "";
    if (serverId) {
      const selected = [...el.playerSearchRegionMenu.querySelectorAll('[data-region-option="cn"]')]
        .find((option) => option.dataset.serverId === serverId);
      el.playerSearchRegionLabel.textContent = selected?.querySelector("b")?.textContent?.trim() || "国服";
      return;
    }
    const currentServerName = data?.connected ? String(data.serverName || "").trim() : "";
    el.playerSearchRegionLabel.textContent = currentServerName ? `国服 · ${currentServerName}` : "国服";
  }

  function updateSearchRegionStatus(data) {
    const serverId = data?.connected ? String(data.serverId || "").trim().toUpperCase() : "";
    const serverName = data?.connected ? String(data.serverName || "").trim() : "";
    const available = Boolean(serverId && serverName);
    el.playerSearchFollowClient.disabled = !available;
    el.playerSearchFollowClient.setAttribute("aria-disabled", String(!available));
    el.playerSearchFollowStatus.textContent = available ? `（${serverName}）` : data?.connected ? "未识别当前服务器" : "需要客户端在运行";
    if (el.playerSearchRegion.dataset.region === "cn" && !el.playerSearchRegion.dataset.serverId) updateSearchRegionLabel(data);
  }

  function applySearchRegion(value, requestedServerID = "") {
    const region = value === "kr" ? "kr" : "cn";
    const serverId = region === "kr" ? "" : String(requestedServerID ?? "").trim().toUpperCase();
    const selected = [...el.playerSearchRegionMenu.querySelectorAll("[data-region-option]")].find((option) => (
      option.dataset.regionOption === region && (region === "kr" || option.dataset.serverId === serverId)
    )) || el.playerSearchRegionMenu.querySelector('[data-region-option="cn"][data-server-id=""]');
    el.playerSearchRegion.dataset.region = region;
    el.playerSearchRegion.dataset.serverId = region === "kr" ? "" : (selected?.dataset.serverId ?? "");
    for (const option of el.playerSearchRegionMenu.querySelectorAll("[data-region-option]")) {
      option.setAttribute("aria-checked", String(option === selected));
    }
    if (region === "kr") setCNRegionExpanded(false);
    updateSearchRegionLabel();
    savePreference("search-region", region);
    if (region === "cn") savePreference("search-server-id", el.playerSearchRegion.dataset.serverId);
  }

  function visibleRegionMenuEntries() {
    return [...el.playerSearchRegionMenu.querySelectorAll("[data-region-menu-entry]")]
      .filter((entry) => !entry.disabled && !entry.closest("[hidden]"));
  }

  function closeRegionMenu() { el.playerSearchRegionMenu.hidden = true; el.playerSearchRegion.setAttribute("aria-expanded", "false"); }
  el.playerSearchRegion.addEventListener("click", () => {
    const opening = el.playerSearchRegionMenu.hidden;
    if (opening) setCNRegionExpanded(el.playerSearchRegion.dataset.region === "cn");
    el.playerSearchRegionMenu.hidden = !opening;
    el.playerSearchRegion.setAttribute("aria-expanded", String(opening));
    if (opening) {
      const selected = visibleRegionMenuEntries().find((entry) => entry.getAttribute("aria-checked") === "true");
      (selected || (el.playerSearchRegion.dataset.region === "cn" ? el.playerSearchCnToggle : visibleRegionMenuEntries()[0]))?.focus();
    }
  });
  el.playerSearchCnToggle.addEventListener("click", () => setCNRegionExpanded(el.playerSearchCnToggle.getAttribute("aria-expanded") !== "true"));
  el.playerSearchRegionMenu.addEventListener("click", (event) => {
    const option = event.target.closest("[data-region-option]");
    if (!option || option.disabled) return;
    applySearchRegion(option.dataset.regionOption, option.dataset.serverId ?? "");
    closeRegionMenu();
    el.playerSearchRegion.focus();
  });
  el.playerSearchRegionMenu.addEventListener("keydown", (event) => {
    if (event.key === "Escape") { closeRegionMenu(); el.playerSearchRegion.focus(); return; }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const options = visibleRegionMenuEntries();
    if (!options.length) return;
    const current = options.indexOf(document.activeElement);
    const direction = event.key === "ArrowDown" ? 1 : -1;
    const origin = current >= 0 ? current : direction > 0 ? -1 : 0;
    const next = event.key === "Home" ? 0 : event.key === "End" ? options.length - 1 : (origin + direction + options.length) % options.length;
    options[next]?.focus();
  });
  document.addEventListener("click", (event) => { if (!event.target.closest(".player-search-region")) closeRegionMenu(); });
  el.playerSearchCnInfo.dataset.tooltip = CN_SERVER_MERGE_NOTE;
  applySearchRegion(preference("search-region", "cn"), preference("search-server-id", ""));
  updateSearchRegionStatus(state.status || {});
  window.addEventListener("deep-legends:status", (event) => updateSearchRegionStatus(event.detail || {}));

  function updateSearchClear() {
    el.playerSearchClear.hidden = !el.playerSearchName.value.trim() && !el.playerSearchTag.value.trim();
  }
  el.playerSearchClear.addEventListener("click", () => {
    el.playerSearchName.value = "";
    el.playerSearchTag.value = "";
    updateSearchClear();
    el.playerSearchName.focus();
  });
  // 粘贴时直接覆盖旧内容：无论输入框里是否还留着上一次查询的名称。
  el.playerSearchName.addEventListener("paste", (event) => {
    const text = (event.clipboardData || window.clipboardData)?.getData("text");
    if (!text) return;
    event.preventDefault();
    el.playerSearchName.value = text.trim();
    if (!absorbPastedRiotID(el.playerSearchName)) el.playerSearchName.select();
    updateSearchClear();
  });
  el.playerSearchTag.addEventListener("paste", (event) => {
    const text = (event.clipboardData || window.clipboardData)?.getData("text");
    if (!text) return;
    event.preventDefault();
    if (text.includes("#")) {
      el.playerSearchTag.value = text.trim();
      absorbPastedRiotID(el.playerSearchTag);
    } else {
      el.playerSearchTag.value = text.replace(/#/g, "").trim();
    }
    updateSearchClear();
  });
  el.playerSearchName.addEventListener("input", () => { absorbPastedRiotID(el.playerSearchName); updateSearchClear(); });
  el.playerSearchTag.addEventListener("input", () => {
    if (el.playerSearchTag.value.includes("#")) absorbPastedRiotID(el.playerSearchTag);
    updateSearchClear();
  });
  for (const input of [el.playerSearchName, el.playerSearchTag]) input.addEventListener("keydown", (event) => {
    if (event.key === "Enter") { event.preventDefault(); submitPlayerSearch(); }
  });
  el.playerSearchGo.addEventListener("click", submitPlayerSearch);
  el.sidebarToggle.addEventListener("click", toggleSidebar);
  el.settingsSidebarToggle.addEventListener("click", toggleSidebar);
  el.retryList.addEventListener("click", () => loadSkins(true));
  el.search.addEventListener("input", debounce(() => { state.query = el.search.value; renderItems(); }, 180));
  const closeRarityMenu = () => { el.rarityMenu.hidden = true; el.rarityButton.setAttribute("aria-expanded", "false"); };
  const closeSortMenu = () => { el.sortMenu.hidden = true; el.sortButton.setAttribute("aria-expanded", "false"); };
  el.rarityButton.addEventListener("click", () => { const opening = el.rarityMenu.hidden; el.rarityMenu.hidden = !opening; el.rarityButton.setAttribute("aria-expanded", String(opening)); if (opening) el.rarityMenu.querySelector("button")?.focus(); });
  for (const option of el.rarityMenu.querySelectorAll("[data-rarity]")) option.addEventListener("click", () => toggleQualitySelection(option));
  el.rarityMenu.addEventListener("keydown", (event) => { if (event.key === "Escape") { closeRarityMenu(); el.rarityButton.focus(); } });
  el.sortButton.addEventListener("click", () => {
    const opening = el.sortMenu.hidden;
    el.sortMenu.hidden = !opening;
    el.sortButton.setAttribute("aria-expanded", String(opening));
    if (opening) el.sortMenu.querySelector('[aria-checked="true"]')?.focus();
  });
  el.sortMenu.addEventListener("click", (event) => {
    const option = event.target.closest("[data-sort]");
    if (!option || option.disabled) return;
    el.sort.value = option.dataset.sort;
    el.sort.dispatchEvent(new Event("change"));
    closeSortMenu();
    el.sortButton.focus();
  });
  el.sortMenu.addEventListener("keydown", (event) => { if (event.key === "Escape") { closeSortMenu(); el.sortButton.focus(); } });
  document.addEventListener("click", (event) => {
    if (!event.target.closest(".quality-menu")) closeRarityMenu();
    if (!event.target.closest(".sort-menu-control")) closeSortMenu();
  });
  updateQualityMenu();
  configureSortControls();
  el.sort.addEventListener("change", () => {
    state.sort = el.sort.value;
    state.descending = defaultDescending(state.sort);
    state.acquisitionFallback = false;
    configureSortControls();
    renderItems();
  });
  el.sortDirection.addEventListener("click", () => { state.descending = !state.descending; el.sortDirection.textContent = state.descending ? "降序" : "升序"; el.sortDirection.setAttribute("aria-pressed", String(state.descending)); renderItems(); });

  for (const tab of el.sectionTabs) tab.addEventListener("click", () => activateSection(tab.dataset.section));
  for (const tab of el.favoritesTabs) tab.addEventListener("click", () => activateFavoritesPage(tab.dataset.favoritesPage));
  for (const tab of el.settingsTabs) tab.addEventListener("click", () => activateSettingsPage(tab.dataset.settingsPage));
  for (const tab of el.viewTabs) tab.addEventListener("click", () => activateTab(tab, el.viewTabs, (selected) => {
    resetCollectionControls(selected.dataset.view);
    loadSkins();
  }));
  setupTabKeyboard(el.sectionTabs);
  setupTabKeyboard(el.favoritesTabs);
  setupTabKeyboard(el.settingsTabs);
  setupTabKeyboard(el.viewTabs);

  for (const tab of el.poolPageTabs) tab.addEventListener("click", () => activateTab(tab, el.poolPageTabs, (selected) => {
    const page = selected.dataset.poolPage;
    el.poolCatalogPanel.hidden = page !== "catalog";
    el.poolUploadPanel.hidden = page !== "upload";
    el.poolHistoryPanel.hidden = page !== "history";
    if (page === "catalog") loadPoolCatalog();
    if (page === "history" && !state.historyLoaded) loadHistory();
  }));
  for (const tab of el.poolViewTabs) tab.addEventListener("click", () => activateTab(tab, el.poolViewTabs, (selected) => {
    state.poolView = selected.dataset.poolView;
    state.poolQuality = "all";
    el.poolQuality.value = "all";
    renderPoolCatalog();
  }));
  setupTabKeyboard(el.poolPageTabs);
  setupTabKeyboard(el.poolViewTabs);
  el.poolPicker.addEventListener("change", () => selectPool(el.poolPicker.value));
  el.poolSearch.addEventListener("input", debounce(() => { state.poolQuery = el.poolSearch.value; renderPoolCatalog(); }, 150));
  el.poolQuality.addEventListener("change", () => { state.poolQuality = el.poolQuality.value; renderPoolCatalog(); });
  el.poolSort.addEventListener("change", () => { state.poolSort = el.poolSort.value; renderPoolCatalog(); });

  el.showUnownedChromas.addEventListener("change", () => {
    state.showUnownedChromas = el.showUnownedChromas.checked;
    applyChromaVisibility();
  });

  el.showPrestigeChromas.addEventListener("change", () => {
    state.showPrestigeChromas = el.showPrestigeChromas.checked;
    applyChromaVisibility();
  });

  function applyChromaVisibility() {
    if (state.view !== "chromas") return;
    el.grid.classList.toggle("hide-unowned", !state.showUnownedChromas);
    el.grid.classList.toggle("hide-prestige", !state.showPrestigeChromas);
    let visibleCount = 0;
    const heroes = new Set();
    const renderedItems = state.chromaRenderedItems;
    for (const item of renderedItems) {
      if ((!state.showUnownedChromas && !item.owned) || (!state.showPrestigeChromas && item.isPrestige)) continue;
      visibleCount += 1;
      heroes.add(item.championId || item.championName);
    }
    el.listMeta.textContent = `全部炫彩 ${formatNumber(visibleCount)} 款 · ${formatNumber(heroes.size)} 位英雄${visibleCount !== state.items.length ? ` · 共 ${formatNumber(state.items.length)} 款` : ""}`;
  }

  el.refreshHistory.addEventListener("click", loadHistory);
  el.copyDiagnostics.addEventListener("click", async () => { if (!state.diagnostics) await loadDiagnostics(); if (!state.diagnostics) { showToast("诊断信息尚不可用"); return; } try { await navigator.clipboard.writeText(JSON.stringify(state.diagnostics, null, 2)); showToast("诊断摘要已复制，不包含客户端令牌"); } catch (_) { showToast("浏览器未允许复制，请手动选择内容"); } });

  el.poolImport.addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = el.poolFile.files[0];
    if (!file) return;
    if (file.size > 1024 * 1024) { el.poolImportStatus.textContent = "文件超过 1 MiB"; return; }
    el.poolImportStatus.textContent = "正在校验…";
    try {
      const content = await file.text();
      let names = [];
      if (file.name.toLocaleLowerCase("en-US").endsWith(".json")) { const parsed = JSON.parse(content); names = Array.isArray(parsed) ? parsed : parsed.names; if (!Array.isArray(names)) throw new Error("JSON 必须是字符串数组或包含 names 数组"); }
      await api("/api/pools/import", { method: "POST", body: JSON.stringify({ name: el.poolName.value, version: el.poolVersion.value, source: el.poolSourceInput.value, content: names.length ? "" : content, names }) }, "pool-import", 15000);
      el.poolImportStatus.textContent = "导入成功";
      el.poolImport.reset();
      await loadPools();
    } catch (error) { el.poolImportStatus.textContent = error.message; }
  });

  async function exitArtworkFullscreen() {
    if (state.fullscreenExitInProgress) return;
    state.fullscreenExitInProgress = true;
    try {
      if (document.fullscreenElement) await document.exitFullscreen().catch(() => {});
      window.desktopTheme?.exitFullscreen?.();
    } finally {
      requestAnimationFrame(() => { state.fullscreenExitInProgress = false; });
    }
  }

  async function closeSkinDialog() {
    ++state.detailGeneration;
    clearTimeout(state.detailMediaTimer);
    resetVideo(el.skinDialogVideo);
    await exitArtworkFullscreen();
    if (el.skinDialog.open) el.skinDialog.close();
  }

  el.skinDialogClose.addEventListener("click", closeSkinDialog);
  el.skinDialog.addEventListener("close", () => {
    window.desktopTheme?.setModalOpen?.(false);
  });
  el.skinDialog.addEventListener("cancel", (event) => {
    if (!document.fullscreenElement) return;
    event.preventDefault();
    void exitArtworkFullscreen();
  });
  el.skinDialogPrevious.addEventListener("click", () => navigateDetail(-1));
  el.skinDialogNext.addEventListener("click", () => navigateDetail(1));
  el.skinDialogArtwork.addEventListener("click", () => {
    const chroma = state.detailKind === "chroma" ? state.selectedSkin : null;
    if (!chroma?.isPrestige || !chroma.prestigeImageId || !ordinaryChromaImageSources(chroma).length) return;
    const generation = ++state.detailGeneration;
    clearTimeout(state.detailMediaTimer);
    state.detailArtworkMode = state.detailArtworkMode === "prestige" ? "ordinary" : "prestige";
    updateChromaArtworkControl(chroma);
    resetDialogImage(state.detailArtworkMode === "prestige" ? "正在读取臻彩原画…" : "正在读取普通炫彩图…");
    loadSelectedChromaArtwork(chroma, generation);
  });
  el.skinDialogFullscreen.addEventListener("click", async () => {
    try {
      if (document.fullscreenElement) await exitArtworkFullscreen();
      else await el.skinDialogArt.requestFullscreen();
    } catch (_) { showToast("当前窗口不支持全屏查看"); }
  });
  document.addEventListener("fullscreenchange", () => {
    const active = document.fullscreenElement === el.skinDialogArt;
    el.skinDialogFullscreen.classList.toggle("is-active", active);
    el.skinDialogFullscreen.setAttribute("aria-label", active ? "退出全屏" : "全屏查看原画");
    el.skinDialogFullscreen.title = active ? "退出全屏" : "全屏查看原画";
    if (!active && !state.fullscreenExitInProgress) window.desktopTheme?.exitFullscreen?.();
  });
  document.addEventListener("keydown", (event) => {
    if (!el.skinDialog.open || event.altKey || event.ctrlKey || event.metaKey) return;
    if (event.key === "ArrowLeft") { event.preventDefault(); navigateDetail(-1); }
    if (event.key === "ArrowRight") { event.preventDefault(); navigateDetail(1); }
  });
  el.skinDialog.addEventListener("click", (event) => { if (event.target === el.skinDialog) void closeSkinDialog(); });
  for (const eventName of ["dblclick", "selectstart"]) el.skinDialogArt.addEventListener(eventName, (event) => event.preventDefault());
  el.copySkinId.addEventListener("click", async () => { if (!state.selectedSkin) return; const label = state.detailKind === "chroma" ? "炫彩 ID" : "皮肤 ID"; try { await navigator.clipboard.writeText(String(state.selectedSkin.id)); showToast(`${label} 已复制`); } catch (_) { showToast(`${label}：${state.selectedSkin.id}`); } });
  document.addEventListener("visibilitychange", () => { if (document.hidden) { clearTimeout(state.statusTimer); state.controllers.get("status")?.abort(); } else refreshStatus(false); });

  function debounce(callback, delay) { let timer = 0; return (...args) => { clearTimeout(timer); timer = setTimeout(() => callback(...args), delay); }; }

  function setupLiveUpdates() {
    if (!("EventSource" in window) || state.destroyed) return;
    state.eventSource?.close();
    const source = new EventSource("/api/events");
    state.eventSource = source;
    source.onmessage = (event) => {
      if (event.data === "ready" || state.destroyed) return;
      // 对局阶段事件只转发给对局模块（“对局”页签的新对局提示灯），不触发全量刷新。
      if (typeof event.data === "string" && event.data.startsWith("gameflow:")) {
        window.dispatchEvent(new CustomEvent("deep-legends:gameflow", { detail: { phase: event.data.slice(9) } }));
        return;
      }
      if (typeof event.data === "string" && event.data.startsWith("convenience:")) {
        const messages = { "convenience:accept": "已自动接受对局", "convenience:play-again": "已再来一局", "convenience:reconnect": "已重新连接对局" };
        if (messages[event.data]) showToast(messages[event.data]);
        return;
      }
      // 好友状态事件只转发给好友面板模块，不触发全量状态刷新。
      if (event.data === "friends-updated") {
        window.dispatchEvent(new CustomEvent("deep-legends:friends-updated"));
        return;
      }
      if (event.data === "account-updated") state.accountLoaded = false;
      // 只在首次连接（快照尚未就绪）时展示全屏读取遮罩；客户端事件触发的
      // 后台刷新静默进行，避免总览等页面每隔几秒被遮罩闪一下。
      if (event.data === "refresh-started" && state.status?.connected && !state.status?.snapshotReady) {
        state.overlaySuppressed = false;
        showReadingOverlay("正在读取收藏信息", "正在整理皮肤、炫彩与账户物品…");
      }
      clearTimeout(state.liveUpdateTimer);
      state.liveUpdateTimer = setTimeout(async () => {
        await refreshStatus(false);
        if (event.data === "account-updated" && state.section === "favorites" && state.favoritesPage === "account") await loadAccount();
      }, 180);
    };
  }

  function setupScrollControls() {
    let frame = 0;
    const update = () => {
      frame = 0;
      el.backToTop.hidden = el.appScroll.scrollTop < Math.max(360, el.appScroll.clientHeight * 0.65);
    };
    el.appScroll.addEventListener("scroll", () => {
      if (!frame) frame = requestAnimationFrame(update);
    }, { passive: true });
    el.backToTop.addEventListener("click", () => {
      const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      el.appScroll.scrollTo({ top: 0, behavior: reducedMotion ? "auto" : "smooth" });
    });
    update();
  }

  function setupFloatingTooltips() {
    const tooltip = document.createElement("div");
    tooltip.id = "global-tooltip";
    tooltip.className = "global-tooltip";
    tooltip.setAttribute("role", "tooltip");
    tooltip.hidden = true;
    document.body.append(tooltip);
    let anchor = null;
    let frame = 0;

    const targetOf = (node) => node instanceof Element ? node.closest("[data-tooltip]") : null;
    const hide = () => {
      if (frame) cancelAnimationFrame(frame);
      frame = 0;
      if (anchor?.getAttribute("aria-describedby") === tooltip.id) anchor.removeAttribute("aria-describedby");
      anchor = null;
      tooltip.hidden = true;
    };
    const position = () => {
      frame = 0;
      if (!anchor?.isConnected || tooltip.hidden) { hide(); return; }
      const rect = anchor.getBoundingClientRect();
      const viewportPadding = 12;
      const gap = 8;
      const width = tooltip.offsetWidth;
      const height = tooltip.offsetHeight;
      if (anchor.dataset.tooltipSide === "menu") {
        const boundary = anchor.closest(".region-menu")?.getBoundingClientRect() || rect;
        const fitsRight = window.innerWidth - boundary.right >= width + gap + viewportPadding;
        const fitsLeft = boundary.left >= width + gap + viewportPadding;
        if (fitsRight || fitsLeft) {
          const left = fitsRight ? boundary.right + gap : boundary.left - width - gap;
          const top = Math.min(window.innerHeight - height - viewportPadding, Math.max(viewportPadding, rect.top + rect.height / 2 - height / 2));
          tooltip.dataset.placement = fitsRight ? "right" : "left";
          tooltip.style.left = `${Math.round(left)}px`;
          tooltip.style.top = `${Math.round(top)}px`;
          return;
        }
      }
      const left = Math.min(window.innerWidth - width - viewportPadding, Math.max(viewportPadding, rect.left + rect.width / 2 - width / 2));
      const fitsAbove = rect.top >= height + gap + viewportPadding;
      const top = fitsAbove ? rect.top - height - gap : Math.min(window.innerHeight - height - viewportPadding, rect.bottom + gap);
      tooltip.dataset.placement = fitsAbove ? "top" : "bottom";
      tooltip.style.left = `${Math.round(left)}px`;
      tooltip.style.top = `${Math.round(Math.max(viewportPadding, top))}px`;
    };
    const show = (next) => {
      const content = String(next?.dataset.tooltip || "").trim();
      if (!content) { hide(); return; }
      if (anchor && anchor !== next && anchor.getAttribute("aria-describedby") === tooltip.id) anchor.removeAttribute("aria-describedby");
      anchor = next;
      anchor.setAttribute("aria-describedby", tooltip.id);
      tooltip.textContent = content;
      tooltip.dataset.size = next.dataset.tooltipSize || "";
      tooltip.hidden = false;
      tooltip.style.left = "0px";
      tooltip.style.top = "0px";
      if (frame) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(position);
    };

    document.addEventListener("pointerover", (event) => {
      const next = targetOf(event.target);
      if (next && next !== anchor) show(next);
    });
    document.addEventListener("pointerout", (event) => {
      const current = targetOf(event.target);
      if (!current || current !== anchor || (event.relatedTarget instanceof Node && current.contains(event.relatedTarget)) || document.activeElement === current) return;
      hide();
    });
    document.addEventListener("focusin", (event) => { const next = targetOf(event.target); if (next) show(next); });
    document.addEventListener("focusout", (event) => { if (targetOf(event.target) === anchor) hide(); });
    document.addEventListener("keydown", (event) => { if (event.key === "Escape") hide(); });
    window.addEventListener("resize", () => { if (anchor && !frame) frame = requestAnimationFrame(position); }, { passive: true });
    document.addEventListener("scroll", () => { if (anchor && !frame) frame = requestAnimationFrame(position); }, { passive: true, capture: true });
  }

  // 顶栏右侧为 Windows 桌面壳的系统窗口按钮（最小化/最大化/关闭）预留空间：
  // 仅当窗口控件覆盖层真实存在时按其几何信息计算占位，浏览器与 macOS 壳
  // 中保持 CSS 默认的 10px；全屏时覆盖层隐藏，占位自动归零。
  function setupTitlebarInset() {
    const overlay = navigator.windowControlsOverlay;
    if (!overlay) return;
    const update = () => {
      if (!overlay.visible) {
        document.documentElement.style.removeProperty("--titlebar-inset");
        return;
      }
      const rect = overlay.getTitlebarAreaRect();
      const inset = Math.max(0, Math.round(window.innerWidth - rect.x - rect.width));
      if (inset > 0) document.documentElement.style.setProperty("--titlebar-inset", `${inset + 8}px`);
      else document.documentElement.style.removeProperty("--titlebar-inset");
    };
    overlay.addEventListener("geometrychange", update);
    window.addEventListener("resize", update, { passive: true });
    update();
  }

  // 暗夜金库改版的一次性迁移：老用户档案里存的浅色/自动主题
  // 首次升级后统一切到与新设计一致的深色，之后的手动选择照常生效。
  if (!preference("theme-migrated-dark-vault", "")) {
    savePreference("theme", "dark");
    savePreference("theme-migrated-dark-vault", "1");
  }
  applyAppearance();
  applySidebar();
  setupLiveUpdates();
  setupScrollControls();
  setupFloatingTooltips();
  setupTitlebarInset();
  void setupChampionNetwork();
  refreshStatus(true);
})();
