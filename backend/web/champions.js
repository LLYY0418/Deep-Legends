(() => {
  "use strict";

  const escapeHTML = window.deepLegendsRuntime.escapeHTML;

  // Shoes are rendered separately and do not count toward these route limits.
  const ADC_ITEM_ROUTE_LIMIT = 7;
  const DEFAULT_ITEM_ROUTE_LIMIT = 6;
  const CORE_RECOMMENDATION_LIMIT = 5;

  const root = document.getElementById("champions-root");
  const panel = document.getElementById("champions-panel");
  const appScroll = document.getElementById("app-scroll");
  const settingPosition = document.getElementById("setting-champion-position");
  if (!root || !panel) return;

  const positionOptions = [
    { value: "all", label: "全部" }, { value: "top", label: "上单" }, { value: "jungle", label: "打野" },
    { value: "mid", label: "中单" }, { value: "adc", label: "下路" }, { value: "support", label: "辅助" },
  ];
  const fallbackTiers = [
    ["all", "全部段位"], ["challenger", "最强王者"], ["grandmaster", "傲世宗师"], ["master_plus", "超凡大师以上"],
    ["master", "超凡大师"], ["diamond_plus", "钻石以上"], ["diamond", "钻石"], ["emerald_plus", "翡翠以上"],
    ["emerald", "翡翠"], ["platinum_plus", "铂金以上"], ["platinum", "铂金"], ["gold_plus", "黄金以上"],
    ["gold", "黄金"], ["silver", "白银"], ["bronze", "青铜"], ["iron", "黑铁"],
  ].map(([value, label]) => ({ value, label }));

  const initialMode = normalizeMode(readSetting("champion-mode", "ranked"));
  migrateChampionModePreferences(initialMode);

  const state = {
    section: "overview",
	    mode: initialMode,
    tier: normalizeTier(readSetting("champion-tier", "emerald_plus")),
    position: normalizePosition(readSetting("champion-position", "all")),
	    query: normalizeStoredSearch(readSetting(`champion-query-${initialMode}`, "")),
	    augmentQuery: normalizeStoredSearch(readSetting("champion-augment-query", "")),
    augmentRarity: "all",
	    mayhemView: normalizeMayhemView(readSetting("champion-mayhem-view", "champions")),
    mayhemDetailTab: normalizeMayhemDetailTab(readSetting("champion-mayhem-detail-tab", "overview")),
    // 阶段筛选（P0-6）是会话内的视图状态，不持久化：换英雄后上游缺该阶段时
    // 一个记住的阶段号会让整片推荐凭空消失，默认回到英雄级汇总更安全。
    mayhemStage: 0,
    mayhemAugmentID: 0,
    mayhemAugmentDetail: null,
    mayhemAugmentDetailCache: window.deepLegendsRuntime?.createCache({ max: 96, ttl: 300000 }) || new Map(),
    mayhemAugmentLoading: false,
    mayhemAugmentError: "",
    mayhemAtlasLoading: false,
    mayhemAtlasError: "",
    mayhemRarityData: null,
    mayhemRarityLoading: false,
    mayhemRarityError: "",
    // R116-E P2-6：本人海克斯大乱斗「选了某个海克斯之后通常出什么」静态查询。
    // 数据全部来自本地赛季缓存（season-stats），后端零额外网络请求。
    mayhemPersonalBuilds: null,
    mayhemPersonalBuildsKey: 0,
    mayhemPersonalBuildsLoading: false,
    mayhemPersonalBuildsError: "",
    mayhemDetailLoading: false,
    mayhemDetailError: "",
    mayhemDetailKey: 0,
    mayhemRequestToken: 0,
    mayhemAugmentRequestToken: 0,
    mayhemDialogOpen: false,
    mayhemDialogReturnFocus: null,
    catalog: null,
    rankings: null,
    augments: null,
	    detail: null,
	    detailPosition: null,
	    detailCache: window.deepLegendsRuntime?.createCache({ max: 64, ttl: 300000 }) || new Map(),
	    selected: null,
    loading: false,
    error: "",
    requests: new Map(),
    listScroll: 0,
    listScrollInner: 0,
    listScrollRestorePending: false,
    preload: null,
    preloaded: Object.create(null),
    arenaSort: "winRate",
    arenaRarity: "all",
    arenaExpanded: { augments: false, prism: false, core: false },
	    arenaFirstTab: normalizeArenaFirstTab(readSetting("champion-arena-first-tab", "pros")),
    arenaFirstPlaces: null,
    arenaFirstError: "",
    arenaLocalMatches: null,
    arenaDetailLoading: false,
    arenaDetailError: "",
    arenaDetailKey: 0,
    arenaRequestToken: 0,
    arenaDialogOpen: false,
    arenaDialogReturnFocus: null,
    arenaFirstLoading: false,
    arenaLocalLoading: false,
    workspaceRequestToken: 0,
    detailRequestToken: 0,
	    runePage: normalizeRunePage(readSetting("champion-rune-page", "0")),
	    detailChampionID: normalizeChampionDetailID(readSetting(`champion-detail-id-${initialMode}`, "")),
    playerDetour: false,
  };

  let searchComposing = false;

  function readSetting(key, fallback) { try { return localStorage.getItem(`lol-loot-${key}`) ?? fallback; } catch (_) { return fallback; } }
  function writeSetting(key, value) { try { localStorage.setItem(`lol-loot-${key}`, String(value)); } catch (_) {} }
  // The legacy selection belongs only to the mode saved alongside it, never all three.
  function migrateChampionModePreferences(mode) {
    if (readSetting("champion-mode-preferences-v2", "") === "1") return;
    for (const key of ["champion-detail-id", "champion-query"]) {
      const scoped = `${key}-${mode}`;
      if (readSetting(scoped, null) === null) writeSetting(scoped, readSetting(key, ""));
    }
    writeSetting("champion-mode-preferences-v2", "1");
  }

  function switchChampionMode(value) {
    const mode = normalizeMode(value);
    if (mode === state.mode) return;
    flushChampionSearch();
    resetTransientChampionState({ restorePosition: true });
    state.mode = mode;
    writeSetting("champion-mode", mode);
    state.detailChampionID = normalizeChampionDetailID(readSetting(`champion-detail-id-${mode}`, ""));
    state.query = normalizeStoredSearch(readSetting(`champion-query-${mode}`, ""));
    state.rankings = null;
    state.augments = null;
    loadWorkspace(true);
  }

  function normalizeTier(value) { return fallbackTiers.some((item) => item.value === value) ? value : "emerald_plus"; }
  function normalizePosition(value) { return positionOptions.some((item) => item.value === value) ? value : "all"; }
  function firstPositionOf(row) {
    const candidate = Array.isArray(row?.positions) ? row.positions[0] : row?.position;
    return positionOptions.some((item) => item.value === candidate && item.value !== "all") ? candidate : "";
  }
  function normalizeMode(value) { return ["ranked", "aram-mayhem", "arena"].includes(value) ? value : "ranked"; }
	function normalizeStoredSearch(value) { return String(value || "").slice(0, 120); }
	function normalizeMayhemView(value) { return value === "atlas" ? "atlas" : "champions"; }
  // R116-B：英雄详情页三 tab（概览/构筑/表现）。「表现」只在 postmatch 面板真的
  // 下发时出现，所以这里只做取值归一，有没有内容由 mayhemDetailTabSpecs 判断。
  // 取值列表写成字面量而不是模块级 const：这个函数在 state 初始化时（第 40 行）
  // 就会被调用，const 还没到声明位置会踩 TDZ，整个英雄页直接白屏。
  function normalizeMayhemDetailTab(value) { return ["overview", "build", "performance"].includes(value) ? value : "overview"; }
  // 阶段筛选：0 = 英雄级汇总（默认视图，工单 P0-6-2），1..4 = 上游 stages[] 的阶段号。
  const MAYHEM_STAGE_OPTIONS = [[0, "汇总"], [1, "阶段 1"], [2, "阶段 2"], [3, "阶段 3"], [4, "阶段 4"]];
  function normalizeMayhemStage(value) { const parsed = Number(value); return Number.isInteger(parsed) && parsed >= 0 && parsed <= 4 ? parsed : 0; }
	function normalizeArenaFirstTab(value) { return value === "mine" ? "mine" : "pros"; }
	function normalizeRunePage(value) { const parsed = Number(value); return Number.isInteger(parsed) && parsed >= 0 && parsed <= 20 ? parsed : 0; }
	function normalizeChampionDetailID(value) { const parsed = Number(value); return Number.isInteger(parsed) && parsed > 0 ? parsed : 0; }
  function usesHexdata(source) { return String(source || "").toLowerCase().includes("hexdata"); }
  function augmentRarityKey(value) {
    const key = String(value || "").replace(/^k/i, "").toLowerCase();
    return ["silver", "gold", "prismatic"].includes(key) ? key : "unknown";
  }
  function normalizeSearch(value) { return String(value || "").normalize("NFKC").toLocaleLowerCase("zh-CN").replace(/[\s\p{P}\p{S}]+/gu, ""); }
  const numberFormatters = new Map();
  const integerFormatter = new Intl.NumberFormat("zh-CN");
  function numberFormatter(digits) {
    const precision = Math.max(0, Math.min(20, Number(digits) || 0));
    if (!numberFormatters.has(precision)) numberFormatters.set(precision, new Intl.NumberFormat("zh-CN", { maximumFractionDigits: precision }));
    return numberFormatters.get(precision);
  }
  function number(value, digits = 1) { const parsed = Number(value); return Number.isFinite(parsed) ? numberFormatter(digits).format(parsed) : "—"; }
  function percent(value) { if (value === null || value === undefined || String(value).trim() === "") return "—"; const parsed = Number(value); return Number.isFinite(parsed) ? `${parsed.toFixed(2)}%` : "—"; }
  function compactNumber(value) { const parsed = Number(value); if (!Number.isFinite(parsed) || parsed <= 0) return "—"; return parsed >= 10000 ? `${(parsed / 10000).toFixed(parsed >= 100000 ? 0 : 1)}万` : integerFormatter.format(parsed); }
  function positionLabel(value) { return ({ top: "上单", jungle: "打野", mid: "中单", adc: "下路", support: "辅助", all: "全部" })[value] || value || "位置未知"; }
  function positionIcon(value) {
    const name = ({ all: "all", top: "top", jungle: "jungle", mid: "middle", adc: "bottom", support: "utility" })[value] || "all";
    return `<span class="position-icon" aria-hidden="true"><img src="/position-icons/${name}.svg" alt="" decoding="async"></span>`;
  }
  function tierDisplay(value) {
    if (value === null || value === undefined || String(value).trim() === "") return "—";
    const parsed = Number(value);
    return Number.isFinite(parsed) && parsed >= 0 ? (parsed === 0 ? "OP" : String(parsed)) : "—";
  }
  function tierBadge(value, extra = "", sourceGrade = "") {
    const grade = String(sourceGrade).trim().toUpperCase();
    if (["S", "A", "B", "C", "D", "F"].includes(grade)) return `<img class="tier-badge${extra ? ` ${extra}` : ""}" src="/tier-icons/yourgg-${grade.toLowerCase()}.svg" alt="梯度 ${grade}" decoding="async">`;
    const present = value !== null && value !== undefined && String(value).trim() !== "";
    const parsed = Number(value);
    const key = present && Number.isFinite(parsed) && parsed >= 0 && parsed <= 5 ? (parsed === 0 ? "op" : String(parsed)) : "";
    const label = tierDisplay(value);
    return key
      ? `<img class="tier-badge${extra ? ` ${extra}` : ""}" src="/tier-icons/${key}.svg" alt="梯度 ${label}" decoding="async">`
      : `<span class="tier-badge-fallback${extra ? ` ${extra}` : ""}">${label}</span>`;
  }
  function objectRows(value) { return Array.isArray(value) ? value.filter((item) => item && typeof item === "object") : []; }
  function currentModePatch() { return state.rankings?.patch || state.detail?.citation?.patch || "当前版本"; }
  function augmentGrade(value, score = 0, scores = []) {
    const grade = String(value || "").trim().toUpperCase();
    if (["OP", "S", "A", "B", "C", "D", "F"].includes(grade)) return grade;
    const numericScore = Number(score);
    const numericScores = scores.map(Number).filter((item) => Number.isFinite(item) && item > 0);
    const percentile = Number.isFinite(numericScore) && numericScore > 0 && numericScores.length
      ? numericScores.filter((item) => item <= numericScore).length / numericScores.length
      : 0;
    return percentile >= 0.9 ? "S" : percentile >= 0.7 ? "A" : "B";
  }
  function catalogTiers() { return objectRows(state.catalog?.tiers); }
  function championIndex() {
    const source = state.catalog?.champions;
    if (state.championIndex && state.championIndex.source === source) return state.championIndex;
    const rows = objectRows(source), ids = new Map(), keys = new Map();
    for (const item of rows) {
      const id = Number(item.id);
      if (!Number.isNaN(id) && !ids.has(id)) ids.set(id, item);
      for (const key of [String(item.slug || item.key || "").toLowerCase(), String(item.key || "").toLowerCase()]) {
        if (!keys.has(key)) keys.set(key, item);
      }
    }
    return state.championIndex = { source, rows, ids, keys, terms: new WeakMap() };
  }
  function catalogChampions() { return championIndex().rows; }
  function rankingRows() { return objectRows(state.rankings?.rows); }
  function tierLabel(value) { return catalogTiers().find((item) => item.value === value)?.label || fallbackTiers.find((item) => item.value === value)?.label || value; }
  function championMeta(id) { return championIndex().ids.get(Number(id)) || null; }
  function championMetaByKey(key) { return championIndex().keys.get(String(key || "").toLowerCase()) || null; }
  function imageURL(source, path) { return source && path ? `/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(path)}${/\/augments\/icons\//i.test(path) ? "&art=2" : ""}` : "/image-unavailable.svg"; }
  function heroArtworkURL(meta, fallbackSource, fallbackPath) {
    if (meta?.artworkSource && meta?.artworkPath) return imageURL(meta.artworkSource, meta.artworkPath);
    const key = String(meta?.key || "").replace(/[^A-Za-z0-9]/g, "");
    if (key) return imageURL("ddragon", `/cdn/img/champion/splash/${key}_0.jpg`);
    const id = Number(meta?.id) || 0;
    return id > 0 ? imageURL("gtimg", `/images/lol/act/img/skin/big${id * 1000}.jpg`) : imageURL(fallbackSource, fallbackPath);
  }
  function heroArtworkFallbackURL(meta) {
    const key = String(meta?.key || "").replace(/[^A-Za-z0-9]/g, "");
    return key ? imageURL("ddragon", `/cdn/img/champion/splash/${key}_0.jpg`) : "";
  }
  function runeStyleIcon(style, className = "rune-style-icon") {
    const names = { 8000: "precision", 8100: "domination", 8200: "sorcery", 8300: "inspiration", 8400: "resolve" };
    const file = names[Number(style?.id)];
    const label = style?.name || "符文系";
    if (!file) return style?.path ? assetImage(style, className, false) : "";
    return `<span class="champion-asset ${className}" data-tooltip="${escapeHTML(label)}"><img src="/rune-styles/${file}.svg" alt="${escapeHTML(label)}" loading="lazy" decoding="async"></span>`;
  }
  function assetImage(asset, className = "recommend-icon", withTooltip = false) {
    const name = asset?.name || "图标";
    const explanation = String(asset?.description || "").trim();
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
    const image = imageURL(asset?.source, asset?.path);
    const isAugment = String(className).split(/\s+/).includes("augment-icon");
    const path = String(asset?.path || "");
    const isLargeAugment = isAugment && /_large\.png(?:[?#]|$)/i.test(path);
    const fallbackPath = asset?.fallbackPath || asset?.imageFallbackPath || "";
    const fallbackImage = isLargeAugment && asset?.source && fallbackPath ? imageURL(asset.source, fallbackPath) : "";
    const fallbackData = fallbackImage ? ` data-augment-fallback="${escapeHTML(fallbackImage)}"` : "";
    return `<span class="champion-asset ${className}"${fallbackData}${withTooltip ? ` tabindex="0" data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}"` : ""}><img data-queued-src="${image}" alt="${escapeHTML(name)}" loading="lazy" decoding="async" data-champion-image><span aria-hidden="true">${escapeHTML(name.slice(0, 1))}</span></span>`;
  }

  async function api(path, key = path) {
    state.requests.get(key)?.abort();
    const controller = new AbortController();
    state.requests.set(key, controller);
    let timedOut = false;
    let httpStatus = 0;
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, 35000);
    try {
      const response = await fetch(path, { headers: { Accept: "application/json" }, signal: controller.signal });
      httpStatus = response.status;
      if (response.status === 401) throw new Error("页面会话已过期，刷新页面即可重新连接");
      if (!response.ok) throw new Error((await response.text()).trim() || `本地服务返回 HTTP ${response.status}`);
      let payload;
      try { payload = await response.json(); } catch (error) { error.errorKind = "decode"; throw error; }
      if (controller.signal.aborted || state.requests.get(key) !== controller) {
        const cancelled = new Error("请求已取消"); cancelled.name = "RequestCancelled"; cancelled.errorKind = "canceled"; throw cancelled;
      }
      return payload;
    } catch (error) {
      const errorKind = timedOut ? "timeout" : error.errorKind || (error.name === "AbortError" || error.name === "RequestCancelled" ? "canceled" : httpStatus >= 400 ? "http" : httpStatus ? "decode" : "network");
      window.reportFlowDiagnostic?.("local_request_client", "failed", { endpoint: "champions", httpStatus, errorKind });
      if (error.name === "RequestCancelled") throw error;
      if (timedOut) { const timeoutError = new Error("本地请求超时，请重试"); timeoutError.name = "TimeoutError"; throw timeoutError; }
      if (error.name === "AbortError") { const cancelled = new Error("请求已取消"); cancelled.name = "RequestCancelled"; throw cancelled; }
      throw error;
    } finally {
      clearTimeout(timer);
      if (state.requests.get(key) === controller) state.requests.delete(key);
    }
  }

  function beginStartupPreload() {
    if (state.preload) return;
    const tier = state.tier;
    const position = state.position;
    const quiet = (promise) => promise.catch(() => null);
    const remember = (key, promise) => quiet(promise).then((value) => { if (value) state.preloaded[key] = value; return value; });
    state.preload = {
      tier,
      position,
      catalog: remember("catalog", api("/api/champions/catalog", "preload-catalog")),
      ranked: remember("ranked", api(`/api/champions/rankings?mode=ranked&tier=${encodeURIComponent(tier)}&position=${encodeURIComponent(position)}`, "preload-ranked")),
	  arena: remember("arena", api("/api/champions/rankings?mode=arena", "preload-arena")),
    };
  }

  function rankingsCacheKey() { return `${state.mode}|${state.tier}|${state.position}`; }
  function stampRankings() { state.rankingsKey = rankingsCacheKey(); state.rankingsAt = Date.now(); }
  function rankingsFresh() {
	return Boolean(state.rankings && state.rankingsKey === rankingsCacheKey() && Date.now() - (state.rankingsAt || 0) < 5 * 60 * 1000);
  }

  function adoptCatalog(promise) {
    void Promise.resolve(promise).then((catalog) => {
      if (!catalog || !Array.isArray(catalog.tiers) || !Array.isArray(catalog.champions)) return;
      state.catalog = catalog;
      if (state.mode === "arena" && state.selected) {
        state.selected = arenaSelectedRow(state.selected) || state.selected;
      }
      if (state.section === "champions" && state.rankings) render();
    }).catch(() => null);
  }

  function primeArenaDetail(selected, resetControls = false) {
    if (!selected) return null;
    if (resetControls) resetArenaControls();
    state.selected = selected;
    state.detail = null;
    state.arenaDetailLoading = true;
    state.arenaDetailError = "";
    state.arenaDetailKey = Number(selected.championId);
    state.arenaFirstLoading = true;
    state.arenaLocalLoading = true;
    return selected;
  }

  function prepareArenaSelection() {
    if (state.mode !== "arena") return null;
    const rows = rankingRows();
    if (!rows.length) {
      state.selected = null;
      state.detail = null;
      state.arenaDetailKey = 0;
      return null;
    }
    const row = rows.find((item) => Number(item.championId) === Number(state.selected?.championId))
      || rows.find((item) => Number(item.championId) === Number(state.detailChampionID))
      || rows[0];
    const selected = arenaSelectedRow({ ...(state.selected || {}), ...row });
    if (!selected) {
      state.selected = null;
      return null;
    }
    const changed = Number(state.selected?.championId) !== Number(selected.championId);
    state.selected = selected;
    if (changed || state.arenaDetailKey !== Number(selected.championId)) return primeArenaDetail(selected, changed);
    return null;
  }

  function renderLoadedWorkspace() {
    const restored = restorePersistedChampionSelection();
    if (restored && state.mode === "ranked") {
      void openDetail(restored);
      return;
    }
    if (restored) state.selected = restored;
    const pendingArenaDetail = prepareArenaSelection();
    const pendingMayhemDetail = prepareMayhemSelection();
    render();
    if (pendingArenaDetail) loadArenaDetail(pendingArenaDetail);
    if (pendingMayhemDetail) loadMayhemDetail(pendingMayhemDetail);
  }

  async function enterChampionSection() {
    const preload = state.preload;
    let hydrated = false;
    if (preload) {
      const token = state.workspaceRequestToken + 1;
      state.workspaceRequestToken = token;
      const mode = state.mode;
      state.loading = true;
      const restored = restorePersistedChampionSelection();
      if (restored) state.selected = restored;
      render();
      adoptCatalog(preload.catalog);
      if (mode === "ranked" && preload.tier === state.tier && preload.position === state.position) {
        const rankings = await preload.ranked;
        if (Array.isArray(rankings?.rows)) {
          state.rankings = rankings;
          state.augments = null;
          hydrated = true;
          delete state.preloaded.ranked;
        }
	  } else if (mode === "arena") {
        const rankings = await preload.arena;
        if (Array.isArray(rankings?.rows)) {
          state.rankings = rankings;
          state.augments = null;
          hydrated = true;
          delete state.preloaded.arena;
        }
      }
      if (token !== state.workspaceRequestToken || mode !== state.mode) return;
      state.preload = null;
      state.loading = false;
      if (state.section !== "champions") return;
      if (hydrated) {
        state.error = "";
        stampRankings();
        renderLoadedWorkspace();
        return;
      }
    }
    if (state.section !== "champions") return;
    if (rankingsFresh()) { renderLoadedWorkspace(); return; }
    loadWorkspace(true);
  }

  async function loadWorkspace(force = false) {
    if (!force && rankingsFresh()) { renderLoadedWorkspace(); return; }
    const token = state.workspaceRequestToken + 1;
    state.workspaceRequestToken = token;
    const mode = state.mode;
    const tier = state.tier;
    const position = state.position;
    const current = () => token === state.workspaceRequestToken && mode === state.mode && tier === state.tier && position === state.position;
    state.loading = true;
    state.error = "";
    const restored = restorePersistedChampionSelection();
    if (restored) state.selected = restored;
    render();
    const catalogPromise = state.catalog && !force
      ? Promise.resolve(state.catalog)
      : state.preloaded.catalog
        ? Promise.resolve(state.preloaded.catalog)
        : api("/api/champions/catalog", "catalog");
    adoptCatalog(catalogPromise);
    try {
      let rankings;
	if (mode === "ranked") {
        rankings = state.preloaded.ranked || await api(`/api/champions/rankings?mode=ranked&tier=${encodeURIComponent(tier)}&position=${encodeURIComponent(position)}`, "rankings");
        delete state.preloaded.ranked;
	} else if (mode === "aram-mayhem") {
	  rankings = await api("/api/champions/rankings?mode=aram-mayhem", "rankings");
      } else {
        rankings = state.preloaded.arena || await api("/api/champions/rankings?mode=arena", "rankings");
        delete state.preloaded.arena;
      }
      if (!current()) return;
      if (!rankings || !Array.isArray(rankings.rows) || !rankings.rows.length) throw new Error("英雄梯度数据暂时为空，已保留上次结果");
	state.rankings = rankings;
      stampRankings();
    } catch (error) {
      if (current()) state.error = error.message || "数据读取失败";
    } finally {
      if (!current()) return;
      state.loading = false;
      renderLoadedWorkspace();
    }
  }

  async function loadRankings(preserveSearch = false) {
    if (state.mode !== "ranked") return;
    const token = ++state.workspaceRequestToken;
    const tier = state.tier, position = state.position;
    const current = () => state.mode === "ranked" && state.workspaceRequestToken === token && state.tier === tier && state.position === position;
    state.loading = true;
    state.error = "";
    if (!preserveSearch) { state.rankings = null; render(); }
    try {
      if (!state.catalog) state.catalog = await api("/api/champions/catalog", "catalog");
      if (!current()) return;
      const rankings = await api(`/api/champions/rankings?mode=ranked&tier=${encodeURIComponent(tier)}&position=${encodeURIComponent(position)}`, "rankings");
      if (!current()) return;
      state.rankings = rankings;
      stampRankings();
    } catch (error) {
      if (current()) state.error = error.message || "梯度读取失败";
    } finally {
      if (!current()) return;
      state.loading = false;
      if (preserveSearch) renderChampionSearchResults(); else render();
    }
  }

  async function openDetail(row) {
    const championID = normalizeChampionDetailID(row?.championId);
    if (!championID) return;
    state.detailChampionID = championID;
    writeSetting(`champion-detail-id-${state.mode}`, championID);
    if (state.mode === "arena") {
      selectArenaChampion(row);
      return;
    }
    if (state.mode === "aram-mayhem") {
      selectMayhemChampion(row);
      return;
    }
	    const meta = championMeta(row.championId);
	    const champion = String(row.key || meta?.slug || "").toLowerCase();
	    if (!champion) {
	      state.selected = null;
	      state.detail = null;
	      state.detailChampionID = 0;
	      state.loading = false;
	      state.error = "";
	      writeSetting(`champion-detail-id-${state.mode}`, "");
	      render();
	      return;
	    }
    const championChanged = Number(state.selected?.championId) !== Number(row.championId);
    if (!state.selected) state.listScroll = appScroll?.scrollTop || 0;
    if (championChanged) state.detailPosition = null;
    state.selected = { ...row, meta, champion };
    state.detail = null;
    state.error = "";
	    state.runePage = normalizeRunePage(readSetting("champion-rune-page", "0"));
    // A clicked ranking row is an explicit route selection. It must win over
    // the previous detail route even when the champion itself did not change.
    const rowPosition = firstPositionOf({ position: row.position });
    const candidatePosition = rowPosition || state.detailPosition || (state.position !== "all" ? state.position : "") || firstPositionOf(row);
    const position = firstPositionOf({ position: candidatePosition });
    state.detailPosition = position || null;
    if (!position) {
      state.detailRequestToken += 1;
      state.loading = false;
      state.error = "请先选择分路";
      render();
      appScroll?.scrollTo({ top: 0, behavior: "instant" });
      return;
    }
    state.loading = true;
    const token = state.detailRequestToken + 1;
    state.detailRequestToken = token;
    const current = () => token === state.detailRequestToken && Number(state.selected?.championId) === championID && state.mode === "ranked";
    render();
    appScroll?.scrollTo({ top: 0, behavior: "instant" });
    const query = `mode=ranked&champion=${encodeURIComponent(champion)}&position=${encodeURIComponent(position)}&tier=${encodeURIComponent(state.tier)}`;
    const cacheKey = `${championID}:${position}:${state.tier}`;
    const cached = state.detailCache.get(cacheKey);
    if (cached) {
      state.detail = cached;
      state.loading = false;
      render();
      appScroll?.scrollTo({ top: 0, behavior: "instant" });
      root.querySelector("[data-champion-back]")?.focus({ preventScroll: true });
      return;
    }
    try {
      const detail = await api(`/api/champions/detail?${query}`, "detail");
      if (!current()) return;
      state.detail = detail;
      state.detailCache.set(cacheKey, detail);
      state.error = "";
    } catch (error) {
      if (current()) state.error = error.message || "英雄详情读取失败";
    } finally {
      if (!current()) return;
      state.loading = false;
      render();
      root.querySelector("[data-champion-back]")?.focus({ preventScroll: true });
    }
  }

  function arenaSelectedRow(row) {
    if (!row || Number(row.championId) <= 0) return null;
    const meta = championMeta(row.championId);
    const champion = String(row.key || meta?.slug || row.champion || row.championId || "").trim().toLowerCase();
    return champion ? { ...row, meta, champion } : null;
  }

  function mayhemSelectedRow(row) {
    if (!row || Number(row.championId) <= 0) return null;
    const meta = championMeta(row.championId);
    const champion = String(row.key || meta?.slug || row.champion || row.championId || "").trim().toLowerCase();
    return champion ? { ...row, meta, champion } : null;
  }

  function primeMayhemDetail(selected) {
    if (!selected) return null;
    const preserveDetail = Number(state.selected?.championId) === Number(selected.championId) && state.detail;
    state.selected = selected;
    if (!preserveDetail) state.detail = null;
    state.mayhemDetailLoading = true;
    state.mayhemDetailError = "";
    state.mayhemDetailKey = Number(selected.championId);
    // 评审整改 A1：换英雄必须把阶段筛选放回「汇总」。state 初始化处（第 41-43 行）
    // 的注释早就写明了这个意图，但重置只发生在 resetTransientChampionState（换模式
    // / section 重置），换英雄走的是本函数与 selectMayhemChampion，两者都不重置——
    // 于是上一个英雄记住的「阶段 3」会带到下一个英雄，而下一个英雄可能一条阶段行
    // 都没有（hero-json 熔断走 OP.GG RSC 兜底时必然如此）。renderRecommendedAugments
    // 里另有一道兜底钳制，这里是第一道：从源头上不让阶段号跨英雄存活。
    state.mayhemStage = 0;
    return selected;
  }

  function prepareMayhemSelection() {
    if (state.mode !== "aram-mayhem" || state.mayhemView !== "champions") return null;
    const rows = rankingRows();
    if (!rows.length) {
      state.selected = null;
      state.detail = null;
      state.mayhemDetailKey = 0;
      return null;
    }
    const row = rows.find((item) => Number(item.championId) === Number(state.selected?.championId))
      || rows.find((item) => Number(item.championId) === Number(state.detailChampionID))
      || rows[0];
    const selected = mayhemSelectedRow({ ...(state.selected || {}), ...row });
    if (!selected) return null;
    const changed = Number(state.selected?.championId) !== Number(selected.championId);
    state.selected = selected;
    if (changed || state.mayhemDetailKey !== Number(selected.championId)) return primeMayhemDetail(selected);
    return null;
  }

  function selectMayhemChampion(row) {
    const selected = mayhemSelectedRow(row);
    if (!selected) return;
	const sameChampion = Number(state.selected?.championId) === Number(selected.championId);
	if (sameChampion && (state.mayhemDetailLoading || state.mayhemDetailKey === Number(selected.championId) && state.detail)) {
	  closeMayhemTierDialog(false);
	  return;
	}
    primeMayhemDetail(selected);
    closeMayhemTierDialog(false);
    render();
    loadMayhemDetail(selected);
  }

  function loadMayhemDetail(selected) {
    const token = state.mayhemRequestToken + 1;
    state.mayhemRequestToken = token;
    const championID = Number(selected.championId);
    const current = () => token === state.mayhemRequestToken && state.mode === "aram-mayhem" && state.mayhemView === "champions" && Number(state.selected?.championId) === championID;
    void api(`/api/champions/detail?mode=aram-mayhem&champion=${encodeURIComponent(selected.champion)}`, "mayhem-detail").then((detail) => {
      if (!current()) return;
      state.detail = detail;
      state.mayhemDetailError = "";
    }).catch((error) => {
      if (!current()) return;
      state.mayhemDetailError = error?.message || "英雄详情读取失败";
    }).finally(() => {
      if (!current()) return;
      state.mayhemDetailLoading = false;
      render();
    });
  }

  async function loadMayhemAtlas(force = false) {
    if (state.mayhemAtlasLoading || state.augments && !force) return;
    state.mayhemAtlasLoading = true;
    state.mayhemAtlasError = "";
    render();
    try {
      const augments = await api("/api/champions/augments", "mayhem-atlas");
      if (!augments || !Array.isArray(augments.rows) || !augments.rows.length) throw new Error("海克斯图鉴数据暂时为空，已保留上次结果");
      state.augments = augments;
      const first = filteredAugments()[0];
      // 保留首次加载时的自动选中语义；刷新后若旧选项已经消失，下面的
      // fallback 分支仍会把当前列表第一项选中。
      if (!state.mayhemAugmentID && first) {
        state.mayhemAugmentID = Number(first.id);
      }
      const selected = filteredAugments().find((row) => Number(row.id) === Number(state.mayhemAugmentID));
      if (!selected && first) {
        state.mayhemAugmentID = Number(first.id);
        state.mayhemAugmentDetail = state.mayhemAugmentDetailCache.get(Number(first.id)) || null;
        if (usesHexdata(state.augments?.source)) {
          loadMayhemAugmentDetail(first);
        } else {
          state.mayhemAugmentDetail = { source: state.augments?.source || "OP.GG", champions: [] };
        }
      } else if (selected && !state.mayhemAugmentDetail) {
        state.mayhemAugmentDetail = state.mayhemAugmentDetailCache.get(Number(selected.id)) || null;
        if (usesHexdata(state.augments?.source)) loadMayhemAugmentDetail(selected);
      }
    } catch (error) {
      state.mayhemAtlasError = error?.message || "海克斯图鉴读取失败";
    } finally {
      state.mayhemAtlasLoading = false;
      render();
    }
  }

  function loadMayhemAugmentDetail(item) {
    if (!item || Number(item.id) <= 0) return;
    const token = state.mayhemAugmentRequestToken + 1;
    state.mayhemAugmentRequestToken = token;
    const augmentID = Number(item.id);
    state.mayhemAugmentID = augmentID;
    state.mayhemAugmentDetail = state.mayhemAugmentDetailCache.get(augmentID) || null;
    state.mayhemAugmentLoading = true;
    state.mayhemAugmentError = "";
    if (!usesHexdata(state.augments?.source)) {
      state.mayhemAugmentDetail = { source: state.augments?.source || "OP.GG", champions: [] };
      state.mayhemAugmentLoading = false;
      render();
      return;
    }
    render();
    const current = () => token === state.mayhemAugmentRequestToken && state.mode === "aram-mayhem" && state.mayhemView === "atlas" && state.mayhemAugmentID === augmentID;
    void api(`/api/champions/augment-detail?id=${augmentID}&slug=${encodeURIComponent(item.key || "")}`, "mayhem-augment-detail").then((detail) => {
      if (!current()) return;
      state.mayhemAugmentDetail = detail;
      state.mayhemAugmentDetailCache.set(augmentID, detail);
    }).catch((error) => {
      if (!current()) return;
      state.mayhemAugmentError = error?.message || "海克斯详情读取失败";
    }).finally(() => {
      if (token !== state.mayhemAugmentRequestToken) return;
      state.mayhemAugmentLoading = false;
      if (!current()) return;
      render();
    });
  }

  async function loadMayhemRarity() {
    if (state.mayhemRarityLoading || state.mayhemRarityData) return;
    state.mayhemRarityLoading = true;
    state.mayhemRarityError = "";
    render();
    try {
      state.mayhemRarityData = await api("/api/champions/augment-rarity", "mayhem-rarity");
    } catch (error) {
      state.mayhemRarityError = error?.message || "品质分布读取失败";
    } finally {
      state.mayhemRarityLoading = false;
      if (state.section === "champions" && state.mode === "aram-mayhem") render();
    }
  }

  // ---------------------------------------------------------------------
  // R116-E P2-6：本人海克斯大乱斗「选了某个海克斯之后通常出什么」静态查询
  //
  // 这是对**本机已保存的本人赛季样本**的静态聚合，不是局内实时联动。
  // 局内形态的三个输入（已选海克斯 / 当前阶段 / 系统候选）在仓库现状下
  // 全部没有通道，已被 R116-探测与评审第 3 节证伪（Anti-scope 第 1 条）。
  // 后端 /api/gameplay/season-mayhem-builds 只读本地 season-stats 缓存，
  // 零额外网络请求、不碰 Riot 配额。
  // ---------------------------------------------------------------------

  // 详情页响应里已经带了本英雄的海克斯行与装备行，每行 assets[0] 就是
  // {id, name}。用它把个人样本里的纯 ID 翻成中文名，不必为这一块再拉一次目录。
  // 翻不出来的 ID 一律回落到「海克斯 <id>」/「装备 <id>」，绝不编名字
  // （数据准确性红线：证据不足明确降级，不用推断值代替）。
  function mayhemPersonalAssetNames(rows) {
    const names = new Map();
    for (const row of objectRows(rows)) {
      for (const asset of objectRows(row.assets)) {
        const id = Number(asset.id);
        const name = String(asset.name || "").trim();
        if (Number.isFinite(id) && id > 0 && name && !names.has(id)) names.set(id, name);
      }
    }
    return names;
  }

  async function loadMayhemPersonalBuilds(championID) {
    const key = Number(championID);
    if (!Number.isFinite(key) || key <= 0) return;
    // 同一个英雄只拉一次；失败也记住 key，避免每次重渲染都重打一次。
    if (state.mayhemPersonalBuildsLoading || state.mayhemPersonalBuildsKey === key) return;
    state.mayhemPersonalBuildsLoading = true;
    state.mayhemPersonalBuildsError = "";
    try {
      state.mayhemPersonalBuilds = await api(`/api/gameplay/season-mayhem-builds?championId=${encodeURIComponent(key)}`, "mayhem-personal-builds");
    } catch (error) {
      state.mayhemPersonalBuilds = null;
      state.mayhemPersonalBuildsError = error?.message || "个人海斗出装样本读取失败";
    } finally {
      state.mayhemPersonalBuildsKey = key;
      state.mayhemPersonalBuildsLoading = false;
      if (state.section === "champions" && state.mode === "aram-mayhem") render();
    }
  }

  // 只渲染后端明确标了 available 的结果。样本不足、没连客户端、本赛季没有
  // 海斗场次——一律返回空串，整块不进 DOM（评审 6.1「取不到就整块隐藏」），
  // 不在构筑 tab 里留一个空壳，也不用一句猜测把空缺填上。
  //
  // 结构刻意复用「装备排行」那套 .mayhem-item-ranking / .mayhem-ranking-list
  // 网格（b 序号 / 图标位 / strong 名称 / dl 指标），因此本块零新增 CSS：
  // R117 的样式棘轮把 border-radius / padding / gap 三个预算都卡在实测上限，
  // 任何新的间距取值都会让棘轮变差。
  function renderMayhemPersonalBuilds(detail) {
    const championID = Number(state.selected?.championId || 0);
    const payload = state.mayhemPersonalBuildsKey === championID ? state.mayhemPersonalBuilds : null;
    if (!payload?.available) return "";
    const groups = objectRows(payload.groups);
    if (!groups.length) return "";
    const augmentNames = mayhemPersonalAssetNames(detail?.recommendedAugments);
    const catalogItemNames = mayhemPersonalAssetNames(detail?.itemRanking);
    let index = 0;
    const rows = [];
    for (const group of groups) {
      const augmentID = Number(group.augmentId);
      const augmentName = augmentNames.get(augmentID) || `海克斯 ${Number.isFinite(augmentID) ? augmentID : "?"}`;
      for (const combo of objectRows(group.combos)) {
        index += 1;
        const ids = (Array.isArray(combo.itemIds) ? combo.itemIds : []).map(Number).filter((id) => Number.isFinite(id) && id > 0);
        // 后端给的 itemNames 优先（它覆盖全目录）；缺名字时回落到详情页
        // 自带的装备行；再缺就显示 ID。三种来源都不猜。
        const given = Array.isArray(combo.itemNames) ? combo.itemNames.map((name) => String(name || "").trim()) : [];
        const label = (given.length === ids.length && given.every(Boolean) ? given : ids.map((id) => catalogItemNames.get(id) || `装备 ${id}`)).join(" · ");
        rows.push(`<article><b>${index}</b><span class="mayhem-item-name-only" aria-hidden="true"></span><strong>${escapeHTML(`${augmentName} → ${label}`)}</strong><dl><div><dt>场次</dt><dd>${compactNumber(combo.games)}</dd></div><div><dt>胜率</dt><dd class="metric-win">${percent(combo.winRate)}</dd></div></dl></article>`);
      }
    }
    if (!rows.length) return "";
    return `<section class="recommendation-section mayhem-item-ranking" data-mayhem-personal-builds><header><h3><span class="arena-section-icon" aria-hidden="true">◈</span>我的海斗出装</h3><span class="section-count">本人 ${compactNumber(payload.sampleGames)} 场</span></header><div class="mayhem-ranking-list">${rows.join("")}</div></section>`;
  }

  function resetArenaControls() {
    state.arenaSort = "winRate";
    state.arenaRarity = "all";
    state.arenaExpanded = { augments: false, prism: false, core: false };
	    state.arenaFirstTab = normalizeArenaFirstTab(readSetting("champion-arena-first-tab", "pros"));
    state.arenaFirstPlaces = null;
    state.arenaFirstError = "";
    state.arenaLocalMatches = null;
    state.arenaDetailError = "";
    state.arenaFirstLoading = false;
    state.arenaLocalLoading = false;
  }

  function selectArenaChampion(row) {
    const selected = arenaSelectedRow(row);
    if (!selected) return;
    const changed = Number(state.selected?.championId) !== Number(selected.championId);
    primeArenaDetail(selected, changed);
    closeArenaTierDialog(false);
    render();
    loadArenaDetail(selected);
  }

  function loadArenaDetail(selected) {
    const token = state.arenaRequestToken + 1;
    state.arenaRequestToken = token;
    const championID = Number(selected.championId);
    const query = `mode=arena&champion=${encodeURIComponent(selected.champion)}`;
    const matchCards = window.deepLegendsMatchCards;
    const current = () => token === state.arenaRequestToken && state.mode === "arena" && Number(state.selected?.championId) === championID;

    void api(`/api/champions/detail?${query}`, "detail").then((detail) => {
      if (!current()) return;
      state.detail = detail;
      state.arenaDetailError = "";
    }).catch((error) => {
      if (!current()) return;
      state.detail = null;
      state.arenaDetailError = error?.message || "英雄详情读取失败";
    }).finally(() => {
      if (!current()) return;
      state.arenaDetailLoading = false;
      render();
    });

    void api(`/api/champions/arena-first-places?championId=${championID}&limit=10`, "arena-first-places").then((result) => {
      if (!current()) return;
      state.arenaFirstPlaces = result;
      state.arenaFirstError = "";
      if (!(result?.matches || []).length) state.arenaFirstTab = "mine";
    }).catch((error) => {
      if (!current()) return;
      state.arenaFirstPlaces = { matches: [] };
      state.arenaFirstError = error?.message || "高手对局暂时不可用";
      state.arenaFirstTab = "mine";
    }).finally(() => {
      if (!current()) return;
      state.arenaFirstLoading = false;
      render();
    });

    const localPromise = matchCards?.currentArenaFirstPlaces
      ? Promise.resolve().then(() => matchCards.currentArenaFirstPlaces(championID))
      : Promise.resolve({ matches: [], loaded: false });
    void localPromise.then((result) => {
      if (!current()) return;
      state.arenaLocalMatches = result;
    }).catch(() => {
      if (!current()) return;
      state.arenaLocalMatches = { matches: [], loaded: false };
    }).finally(() => {
      if (!current()) return;
      state.arenaLocalLoading = false;
      render();
    });
  }

  function closeDetail() {
    state.detailRequestToken += 1;
    state.detail = null;
    state.detailPosition = null;
    state.selected = null;
    state.detailChampionID = 0;
    writeSetting(`champion-detail-id-${state.mode}`, "");
    state.error = "";
    render();
    requestAnimationFrame(() => appScroll?.scrollTo({ top: state.listScroll, behavior: "instant" }));
  }

  function restorePersistedChampionSelection() {
    if (!state.detailChampionID || state.mode === "aram-mayhem" && state.mayhemView !== "champions") return null;
    if (state.selected && Number(state.selected.championId) === Number(state.detailChampionID)) return state.selected;
    if (!Array.isArray(state.rankings?.rows)) return null;
    const row = rankingRows().find((item) => Number(item.championId) === Number(state.detailChampionID));
    if (row) return row;
    state.detailChampionID = 0;
    writeSetting(`champion-detail-id-${state.mode}`, "");
    return null;
  }

  async function switchDetailPosition(position) {
    if (state.mode !== "ranked" || !state.selected || !positionOptions.some((item) => item.value === position && item.value !== "all")) return;
    const champion = state.selected.champion;
    const championID = Number(state.selected.championId);
    const cacheKey = `${championID}:${position}:${state.tier}`;
    state.detailPosition = position;
    state.error = "";
    state.loading = true;
    const token = state.detailRequestToken + 1;
    state.detailRequestToken = token;
    const current = () => token === state.detailRequestToken && Number(state.selected?.championId) === championID && state.mode === "ranked";
    const content = root.querySelector(".champion-detail-content");
    if (content) content.innerHTML = renderDetailSkeleton();
    for (const button of root.querySelectorAll("[data-detail-position]")) {
      const active = button.dataset.detailPosition === position;
      button.classList.toggle("is-active", active);
      button.setAttribute("aria-pressed", String(active));
    }
    const cached = state.detailCache.get(cacheKey);
    if (cached) {
      if (!current()) return;
      state.detail = cached;
      state.loading = false;
      render();
      return;
    }
    const query = `mode=ranked&champion=${encodeURIComponent(champion)}&position=${encodeURIComponent(position)}&tier=${encodeURIComponent(state.tier)}`;
    try {
      const detail = await api(`/api/champions/detail?${query}`, "detail");
      if (!current()) return;
      state.detail = detail;
      state.detailCache.set(cacheKey, detail);
      state.error = "";
    } catch (error) {
      if (current()) state.error = error.message || "英雄详情读取失败";
    } finally {
      if (!current()) return;
      state.loading = false;
      render();
    }
  }

	async function refreshRankingsForDetailTier(tier) {
	  const position = state.position;
	  const current = () => state.mode === "ranked" && state.tier === tier && state.position === position;
	  try {
		const rankings = await api(`/api/champions/rankings?mode=ranked&tier=${encodeURIComponent(tier)}&position=${encodeURIComponent(position)}`, "rankings");
		if (!current() || !rankings || !Array.isArray(rankings.rows) || !rankings.rows.length) return;
		state.rankings = rankings;
		stampRankings();
	  } catch (_) {
		// The detail request is authoritative while this screen is open. If the
		// background list refresh fails, returning to the list will retry it.
	  }
	}

	function switchDetailTier(tier) {
	  if (state.mode !== "ranked" || !state.selected) return;
	  const nextTier = normalizeTier(tier);
	  if (nextTier === state.tier) return;
	  state.tier = nextTier;
	  writeSetting("champion-tier", nextTier);
	  refreshRankingsForDetailTier(nextTier);
	  const position = firstPositionOf({ position: state.detailPosition || state.detail?.position || state.selected?.position });
	  if (!position) {
		openDetail(state.selected);
		return;
	  }
	  state.detail = null;
	  state.loading = true;
	  state.error = "";
	  render();
	  switchDetailPosition(position);
	}

  function openCounterDetail(key) {
    const meta = championMetaByKey(key);
    if (!meta) return;
    const ranked = rankingRows().find((item) => Number(item.championId) === Number(meta.id)) || {};
    const position = firstPositionOf(ranked) || firstPositionOf({ position: state.selected?.position }) || "mid";
    openDetail({ ...ranked, championId: meta.id, key: meta.slug, name: meta.nameZh, position, meta });
  }

  function render() {
    const list = root.querySelector(".champion-table-scroll");
    // 重新进入英雄页时会先渲染一版加载态：那一版的英雄表要么不存在、要么
    // 只有骨架行，scrollTop 必然是 0。如果照常把它记下来，真正的位置就被
    // 0 覆盖了，数据到位后也再没得还原。所以还原未完成前只读不写。
    if (list && !state.listScrollRestorePending) state.listScrollInner = list.scrollTop;
    if (state.mode === "arena" || state.mode === "aram-mayhem") renderWorkspace();
    else if (state.selected) renderDetail();
    else renderWorkspace();
	window.deepLegendsSelects?.enhance(root);
    prepareImages();
    if (state.section === "champions" && state.mode === "aram-mayhem" && root.querySelector(".mayhem-rarity-panel") && !state.mayhemRarityData && !state.mayhemRarityLoading && !state.mayhemRarityError) void loadMayhemRarity();
    // R116-E P2-6：构筑 tab 真的渲染出来了才去拉个人海斗出装样本。
    // 与上面 mayhem-rarity 的懒加载同一套口径：不在渲染函数里发请求，
    // 也不为「用户从没点开构筑 tab」的情况白花一次本地读盘。
    if (state.section === "champions" && state.mode === "aram-mayhem" && root.querySelector("[data-mayhem-personal-builds-host]") && !state.mayhemPersonalBuildsLoading) void loadMayhemPersonalBuilds(state.selected?.championId);
    applyRenderedMetricStyles();
    mountArenaMatchCards();
    mountMayhemTierDialog();
    mountArenaTierDialog();
    requestAnimationFrame(() => {
      const nextList = root.querySelector(".champion-table-scroll");
      if (!nextList) return;
      const target = state.listScrollInner || 0;
      nextList.scrollTop = target;
      if (state.listScrollRestorePending && Math.abs(nextList.scrollTop - target) <= 1) state.listScrollRestorePending = false;
    });
  }

  function renderWorkspace() {
    const patchNote = state.rankings?.patch ? `版本 ${state.rankings.patch} · ` : "";
    const dataNote = state.mode === "ranked"
      ? `${patchNote}${tierLabel(state.tier)}`
      : state.mode === "arena"
        ? `${patchNote}海克斯、棱彩装备、核心物品与吃鸡战绩`
        : `${patchNote}英雄、海克斯与构筑`;
    root.innerHTML = `
      <header class="champion-page-head">
        <div><h2>英雄梯度与构建</h2><p>${escapeHTML(dataNote)}</p></div>
        <div class="champion-head-actions">
          ${state.mode === "ranked" ? `<label class="champion-tier-select select-wrap"><span>段位</span><select data-champion-tier>${renderTierOptions()}</select></label>` : ""}
          <button class="text-button" type="button" data-champion-refresh ${state.loading ? "disabled" : ""}>${state.loading ? "正在联网…" : "刷新数据"}</button>
        </div>
      </header>
      <nav class="champion-mode-tabs" role="tablist" aria-label="游戏模式">
        ${modeTab("ranked", "单/双排", "韩服排位梯度与出装")}
		${modeTab("aram-mayhem", "海克斯大乱斗", "随机英雄 · 全员海克斯")}
        ${modeTab("arena", "斗魂竞技场", "一命三人，六强争胜")}
      </nav>
      ${state.error && !state.rankings ? renderError(state.error) : state.loading && !state.rankings ? renderSkeleton() : state.mode === "ranked" ? renderRanked() : state.mode === "arena" ? renderArena() : renderARAM()}`;
  }

  function renderTierOptions() {
    const tiers = catalogTiers().length ? catalogTiers() : fallbackTiers;
    return tiers.map((item) => `<option value="${escapeHTML(item.value)}" ${item.value === state.tier ? "selected" : ""}>${escapeHTML(item.label)}</option>`).join("");
  }

  function modeTab(mode, label, hint) {
    const active = state.mode === mode;
    return `<button type="button" role="tab" aria-selected="${active}" class="champion-mode-tab${active ? " is-active" : ""}" data-champion-mode="${mode}"><span>${label}</span><small>${hint}</small></button>`;
  }

  function renderRanked() {
    const rows = filteredChampionRows();
    return `<section class="champion-list-card">
      <div class="champion-filter-bar">
        <label class="champion-search"><span aria-hidden="true">⌕</span><span class="sr-only">搜索英雄</span><input type="search" value="${escapeHTML(state.query)}" placeholder="搜索中文、拼音、英文、缩写或外号" autocomplete="off" data-champion-search></label>
        <div class="champion-position-tabs" role="tablist" aria-label="英雄位置">${positionOptions.map((item) => `<button type="button" role="tab" aria-label="${item.label}" aria-selected="${item.value === state.position}" class="${item.value === state.position ? "is-active" : ""}" data-champion-position="${item.value}">${positionIcon(item.value)}<span>${item.label}</span></button>`).join("")}</div>
        <span class="champion-result-count" role="status">${rows.length} 位英雄</span>
      </div>
      <div data-champion-results>${renderChampionResults(rows)}</div>
    </section>`;
  }

  // 设计稿：梯度前三名使用横版原画大卡展示，表格从第四名继续。
  function renderTopThree(rows) {
    return `<div class="champion-top3">${rows.map((row, index) => {
      const meta = championMeta(row.championId);
      const name = row.name || meta?.nameZh || meta?.titleZh || `英雄 ${row.championId}`;
      const subname = meta?.titleZh && meta.titleZh !== name ? meta.titleZh : meta?.nameEn || "";
      const source = row.imageSource || meta?.imageSource;
      const path = row.imagePath || meta?.imagePath;
      const position = state.position === "all" && row.position ? ` · ${positionLabel(row.position)}` : "";
      return `<article class="champion-topcard${index === 0 ? " is-first" : ""}" role="button" tabindex="0" data-champion-row="${Number(row.championId)}" aria-label="查看${escapeHTML(name)}详情">
        <img class="topcard-art" data-queued-src="${heroArtworkURL(meta, source, path)}" alt="" loading="lazy" decoding="async" data-champion-image>
        <div class="topcard-shade" aria-hidden="true"></div>
        ${tierBadge(row.tier, "topcard-tier")}
        <div class="topcard-copy">
          <h3>${escapeHTML(name)}</h3>
          <p>${escapeHTML(subname)}${position}</p>
          <div class="topcard-stats"><span>胜率 <b class="metric-win">${percent(row.winRate)}</b></span><span>选用 <b class="metric-pick">${percent(row.pickRate)}</b></span><span>禁用 <b>${percent(row.banRate)}</b></span></div>
        </div>
      </article>`;
    }).join("")}</div>`;
  }

  function renderARAM() {
    const rows = filteredChampionRows();
    const viewTabs = [["champions", "英雄榜"], ["atlas", "海克斯图鉴"]].map(([value, label]) => {
      const active = state.mayhemView === value;
      return `<button type="button" role="tab" aria-selected="${active}" class="${active ? "is-active" : ""}" data-mayhem-view="${value}">${label}</button>`;
    }).join("");
    return `<div class="mayhem-shell">
      <div class="mayhem-view-tabs" role="tablist" aria-label="海克斯大乱斗资料">${viewTabs}</div>
      ${state.mayhemView === "atlas" ? renderMayhemAtlas(filteredAugments()) : renderMayhemChampionBoard(rows)}
    </div>`;
  }

  function renderMayhemChampionBoard(rows) {
    const selectedRow = rows.find((row) => Number(row.championId) === Number(state.selected?.championId)) || rankingRows().find((row) => Number(row.championId) === Number(state.selected?.championId)) || null;
    const selected = selectedRow ? mayhemSelectedRow({ ...(state.selected || {}), ...selectedRow }) : null;
    const list = `<section class="champion-list-card mayhem-champions">
      <div class="aram-champion-head"><div><h3>英雄梯度</h3><p>胜率与海克斯 · ${escapeHTML(currentModePatch())}</p></div><span class="champion-result-count">${rows.length} 位</span></div>
      <label class="champion-search compact"><span aria-hidden="true">⌕</span><span class="sr-only">搜索英雄</span><input type="search" value="${escapeHTML(state.query)}" placeholder="搜索英雄" autocomplete="off" data-champion-search></label>
      <div data-champion-results>${renderChampionResults(rows)}</div>
    </section>`;
    return `<div class="mayhem-workspace">
      ${list}<span data-mayhem-list-marker hidden></span>
      <div class="mayhem-detail-pane">
        <div class="mayhem-tier-entry"><button class="button button-primary tier-trigger" type="button" aria-haspopup="dialog" aria-controls="mayhem-tier-dialog" data-open-mayhem-tiers>英雄梯度</button></div>
        ${selected ? renderMayhemDetailPane(selected) : '<div class="arena-pane-empty"><strong>选择一位英雄</strong><span>查看推荐海克斯、装备排行与完整构筑</span></div>'}
      </div>
      <dialog class="mayhem-tier-dialog" id="mayhem-tier-dialog" aria-labelledby="mayhem-tier-dialog-title"><div class="mayhem-tier-dialog-sheet"><header><div><h2 id="mayhem-tier-dialog-title">英雄梯度</h2><p>胜率与海克斯 · ${escapeHTML(currentModePatch())}</p></div><button class="icon-button control-icon-button tier-dialog-close" type="button" aria-label="关闭英雄梯度" data-close-mayhem-tiers><svg class="control-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button></header><div class="mayhem-tier-dialog-content"></div></div></dialog>
    </div>`;
  }

  function renderMayhemDetailPane(row = state.selected) {
    if (!row) return "";
    const meta = row.meta || championMeta(row.championId);
    const detail = state.detail;
    const title = row.name || meta?.nameZh || meta?.titleZh || `英雄 ${row.championId}`;
    const subtitle = meta?.titleZh && meta.titleZh !== title ? meta.titleZh : meta?.nameEn || "";
    const source = row.imageSource || meta?.imageSource;
    const path = row.imagePath || meta?.imagePath;
    // R116-B P0-5-2：梯度徽章只认官方档位。详情已到而 stats.tier 为空说明官方
    // 档位不可用（后端在 insights 熔断时刻意留 nil），整块隐藏，不拿榜单行的
    // 本地分档冒充官方档位。
    const heroTier = mayhemHeroTier(row, detail);
    const localNote = heroTier?.locallyCalculated ? '<small class="mayhem-tier-local">本地估算</small>' : "";
    const overview = `<section class="mayhem-overview-strip">
      <img class="mayhem-overview-art" data-queued-src="${heroArtworkURL(meta, source, path)}" data-artwork-fallback="${escapeHTML(heroArtworkFallbackURL(meta))}" alt="" aria-hidden="true" decoding="async" data-champion-image>
      <span class="mayhem-overview-shade" aria-hidden="true"></span>
      <div class="mayhem-overview-identity"><span class="champion-detail-portrait"><img data-queued-src="${imageURL(source, path)}" alt="${escapeHTML(title)}" decoding="async" data-champion-image><span>${escapeHTML(title.slice(0, 1))}</span></span><div><h2>${escapeHTML(title)} ${heroTier ? tierBadge(heroTier.tier, "arena-title-tier") : ""}</h2><small>${escapeHTML(subtitle)} · 总榜第 ${Number(row.rank) || "—"} 位${localNote}</small></div></div>
      <div class="mayhem-overview-metrics">${metric("胜率", percent(detail?.stats?.winRate ?? row.winRate))}${metric("样本", compactNumber(row.play))}${heroTier ? metric("梯度", `T${heroTier.tier}`) : ""}</div>
    </section>`;
    if (state.mayhemDetailLoading && !detail) return overview + renderDetailSkeleton();
    if (state.mayhemDetailError && !detail) return overview + renderError(state.mayhemDetailError, true);
    if (!detail) return overview + '<div class="arena-pane-empty"><strong>详情暂时不可用</strong><span>左侧英雄梯度仍可继续浏览</span></div>';
    const staleNotice = state.mayhemDetailError
      ? `<p class="mayhem-detail-stale" role="status">${escapeHTML(state.mayhemDetailError)}，已保留上次适配数据。</p>`
      : "";
    return overview + staleNotice + `<div class="mayhem-detail-content">${mayhemDetailWorkspace(detail)}</div>`;
  }

  // R116-B P0-5-2：详情页梯度徽章的取值来源。
  // - 详情已到：只认 response.stats.tier（官方 hextech-insights 档位）；为 nil
  //   就返回 null → 徽章与「梯度」指标整块隐藏，绝不显示一个编出来的档位。
  // - 详情还在路上：先用榜单行的档位顶上，并按 tierLocallyCalculated 标注
  //   「本地估算」，避免加载期间徽章闪一下再消失。
  function mayhemHeroTier(row, detail) {
    if (detail) {
      const official = Number(detail?.stats?.tier);
      return Number.isFinite(official) && official > 0 ? { tier: official, locallyCalculated: false } : null;
    }
    const fallback = Number(row?.tier);
    return Number.isFinite(fallback) && fallback > 0 ? { tier: fallback, locallyCalculated: row?.tierLocallyCalculated === true } : null;
  }

  // 英雄详情使用「概览/构筑/表现」三页局部切换，只渲染当前页内容。
  function mayhemDetailWorkspace(detail) {
    return `${mayhemDetailToolbar(detail)}<div class="mayhem-detail-panels" data-mayhem-detail-panels>${mayhemDetailPanelMarkup(detail)}</div>`;
  }

  // 表现 tab 只在后端真的下发了指标时出现：某组指标全缺时后端整组不下发，
  // 前端也不留一个点进去全是空的页签（评审 6.1）。
  function mayhemDetailTabSpecs(detail) {
    const tabs = [["overview", "概览"], ["build", "构筑"]];
    if (objectRows(detail?.performance?.metrics).length) tabs.push(["performance", "表现"]);
    return tabs;
  }

  function mayhemDetailActiveTab(detail) {
    const keys = mayhemDetailTabSpecs(detail).map(([key]) => key);
    return keys.includes(state.mayhemDetailTab) ? state.mayhemDetailTab : keys[0];
  }

  function mayhemDetailToolbar(detail) {
    const active = mayhemDetailActiveTab(detail);
    const specs = mayhemDetailTabSpecs(detail);
    const tabs = specs.map(([key, label]) => `<button type="button" role="tab" id="mayhem-detail-tab-${key}" aria-controls="mayhem-detail-panel-${key}" aria-selected="${active === key}" tabindex="${active === key ? "0" : "-1"}" class="${active === key ? "is-active" : ""}" data-mayhem-detail-tab="${key}">${label}</button>`).join("");
    // 海克斯图鉴与稀有度分布是跨英雄的全局维度，保留独立入口并复用既有视图状态机。
    return `<div class="mayhem-detail-toolbar"><div class="mayhem-detail-tabs" role="tablist" aria-label="英雄详情分区" data-mayhem-detail-tabs style="--tab-count:${specs.length}">${tabs}</div><button type="button" class="mayhem-atlas-entry" data-mayhem-view="atlas" aria-label="离开当前英雄，查看全局海克斯图鉴与稀有度分布"><span aria-hidden="true">◈</span>海克斯图鉴</button></div>`;
  }

  // 只渲染当前 tab 的内容：隐藏 tab 一个节点都不进 DOM，因此不可能占布局高度
  // （R33「收藏页 UI」那类隐藏元素占位问题）。
  function mayhemDetailPanelMarkup(detail) {
    const active = mayhemDetailActiveTab(detail);
    const content = active === "build" ? mayhemBuildTabMarkup(detail) : active === "performance" ? renderMayhemPerformancePanel(detail.performance) : mayhemOverviewTabMarkup(detail);
    return `<div class="mayhem-detail-panel" id="mayhem-detail-panel-${active}" role="tabpanel" aria-labelledby="mayhem-detail-tab-${active}" data-mayhem-detail-panel="${active}">${content}</div>`;
  }

  // 概览：海克斯推荐 → 开局配置 → 该英雄专属品质概率。
  function mayhemOverviewTabMarkup(detail) {
    return `<div data-mayhem-augments>${renderRecommendedAugments(detail.recommendedAugments || [], detail.citation)}</div>${renderMayhemOpeningConfiguration(detail.build || {}, detail.buildCitation)}${renderMayhemHeroRarityPanel(detail.heroStageRarity)}`;
  }

  // 构筑：装备排行（P0-1/P0-2）+ 装备路线 + 技能加点。成装三件套
  // （terminalItemTrios）后端解析层已有但详情页响应还没下发，本轮未消费。
  function mayhemBuildTabMarkup(detail) {
    // R116-E P2-6：末尾追加「我的海斗出装」个人静态查询块。既有三段
    // （装备排行 / 装备路线 / 技能加点）的渲染一字未动，只是多拼一段；
    // 拿不到样本时 renderMayhemPersonalBuilds 返回空串，输出与改动前完全一致。
    return `${renderMayhemItemRanking(detail.itemRanking || [], detail.citation)}${renderMayhemItemRoutes(detail.build || {}, detail.buildCitation)}${renderMayhemSkillPlan(detail.build || {})}<div data-mayhem-personal-builds-host>${renderMayhemPersonalBuilds(detail)}</div>`;
  }


  // 切 tab 只替换面板内容与页签状态，避免整棵详情重渲染。
  function switchMayhemDetailTab(value) {
    const tab = normalizeMayhemDetailTab(value);
    if (tab === state.mayhemDetailTab) return;
    state.mayhemDetailTab = tab;
    writeSetting("champion-mayhem-detail-tab", tab);
    if (!updateMayhemDetailTabs()) render();
  }

  function updateMayhemDetailTabs() {
    const detail = state.detail;
    if (!detail) return false;
    const panels = root.querySelector("[data-mayhem-detail-panels]");
    const bar = root.querySelector("[data-mayhem-detail-tabs]");
    if (!panels || !bar) return false;
    const active = mayhemDetailActiveTab(detail);
    panels.innerHTML = mayhemDetailPanelMarkup(detail);
    for (const button of bar.querySelectorAll("[data-mayhem-detail-tab]")) {
      const isActive = button.dataset.mayhemDetailTab === active;
      button.classList.toggle("is-active", isActive);
      button.setAttribute("aria-selected", String(isActive));
      button.tabIndex = isActive ? 0 : -1;
    }
    prepareImages(panels);
    applyRenderedMetricStyles();
    return true;
  }

  function renderMayhemItemRanking(rows, citation) {
    // R116-B P0-2 排序护栏：这里以前按 score 降序、再按 games 降序重排（R63
    // 「前端偷偷重排」的老毛病），一条「10 场胜率 100%」的行只要 score 高就能
    // 插到「5000 场胜率 55%」前面。后端直出的顺序就是官方口径
    // meta.recommendationPolicy.ranking = sample_tier_then_wilson_lower_bound_v1
    // （rankingSignals: sample_tier → wilson_lower_bound → pick_rate → games），
    // 所以前端一律不重排，原样渲染。
    const rankingRows = objectRows(rows).slice(0, 8);
    const content = rankingRows.map((row, index) => {
      const asset = row.assets?.[0] || {};
      const icon = asset.source && asset.path ? assetImage(asset, "recommend-icon", true) : '<span class="mayhem-item-name-only" aria-hidden="true"></span>';
      return `<article${mayhemSampleTierLabel(row) ? ' class="is-low-confidence"' : ""}><b>${index + 1}</b>${icon}<strong>${escapeHTML(asset.name || "未知装备")}</strong><dl><div><dt>胜率</dt><dd class="metric-win"${mayhemWilsonTooltip(row)}>${percent(row.winRate)}</dd></div><div><dt>场次</dt><dd>${compactNumber(row.games)}</dd></div>${mayhemDeltaCell(row.deltaWinRate)}${mayhemWithoutItemCell(row)}${mayhemSampleCell(row)}</dl></article>`;
    }).join("");
    return `<section class="recommendation-section mayhem-item-ranking"><header><h3><span class="arena-section-icon" aria-hidden="true">◈</span>装备排行</h3><span class="section-count">${objectRows(rows).length} 件</span></header><div class="mayhem-ranking-list">${content || '<p class="mayhem-inline-empty">暂无装备排行样本。</p>'}</div></section>`;
  }

  // ---- R116-B P0-1 / P0-2 的行内标签（海克斯卡与装备排行共用）----
  // deltaWinRate 是上游 0..1 原值（后端不换算，与 winRate 的百分数不同），
  // 展示时 ×100。为 0 或字段缺失时返回空串：「较基准 +0.0%」没有信息量，
  // 还会被读成「实测过、确实没有收益」（评审 6.1：取不到就整块隐藏）。
  //
  // 评审整改 A2：0 的判据必须放在 toFixed(1) 之后。原来只挡精确 0，挡不住被一位
  // 小数抹成 0 的小值——任何 |deltaWinRate| < 0.00005 的行都会渲染出「较基准
  // +0.0%」，正是工单 P0-1 判据明文禁止的那个字符串；而触发面不小（单英雄 118 条
  // 装备行 + 499 条阶段行都走这个函数，「某阶段收益率实测接近 0」是完全正常的
  // 上游取值）。格式化之后再判 "0.0" 就把整个舍入区间一起挡住了。
  function mayhemDeltaLabel(deltaWinRate) {
    const value = Number(deltaWinRate);
    if (!Number.isFinite(value)) return "";
    const points = value * 100;
    const text = Math.abs(points).toFixed(1);
    if (text === "0.0") return "";
    return `${points > 0 ? "+" : "-"}${text}%`;
  }

  // 正数用既有的胜率高亮色（--success），负数用既有的警示色（--warning），
  // 不新造配色（工单 P0-1-2）。
  function mayhemDeltaTone(deltaWinRate) {
    const value = Number(deltaWinRate);
    if (!Number.isFinite(value) || value === 0) return "";
    return value > 0 ? "is-delta-up" : "is-delta-down";
  }

  // 评审整改 B6：给「较基准」一个定义，别让它是个没有定义的词。装备行是英雄级
  // 口径，实测 deltaWinRate = 这件装备的胜率 − 该英雄整体胜率（4 个英雄共 479 条
  // 装备行零误差），tooltip 就把这句话原样给用户；不带任何算出来的数字，因为英雄
  // 级基准值本身没有逐行下发。tabindex 与 data-tooltip-size 沿用 mayhemWilsonTooltip
  // 的既有写法（键盘可达 + 紧凑气泡）。海克斯卡那边因为卡片外壳 overflow:hidden
  // 装不下 tooltip，改用可见的「阶段基准」一格，见 mayhemAugmentConfidenceMetrics。
  function mayhemDeltaCell(deltaWinRate) {
    const label = mayhemDeltaLabel(deltaWinRate);
    return label ? `<div><dt>较基准</dt><dd class="mayhem-delta ${mayhemDeltaTone(deltaWinRate)}" tabindex="0" data-tooltip="较基准 = 这件装备的胜率 − 该英雄整体胜率" data-tooltip-size="compact">${label}</dd></div>` : "";
  }

  // P0-1-3：「出 vs 不出」对照。withoutItemWinRate 后端已换算成百分数；
  // 缺失或为 0 时整个单元格不渲染（0 会被读成「不出这件必败」）。
  function mayhemWithoutItemCell(row) {
    const value = Number(row?.withoutItemWinRate);
    if (!Number.isFinite(value) || value <= 0) return "";
    return `<div><dt>不出时</dt><dd class="mayhem-without-item" data-tooltip="不出这件装备时的胜率 ${percent(value)}">${percent(value)}</dd></div>`;
  }

  // P0-2-2：低样本徽记复用既有的 is-low-confidence 样式类，不新造一套。
  // sampleTier 由后端按 meta.samplePolicy 算好直出；空串（meta 不可用）时
  // 整块不渲染，前端不自己按 games 猜阈值。
  function mayhemSampleTierLabel(row) {
    return String(row?.sampleTier || "") === "low" ? "样本极少" : "";
  }

  function mayhemSampleCell(row) {
    const label = mayhemSampleTierLabel(row);
    return label ? `<div><dt>置信</dt><dd class="is-low-confidence">${label}</dd></div>` : "";
  }

  // P0-2-3：Wilson 下界只在高样本行给（低样本行的下界没有参考意义，那一行已经
  // 有「样本极少」徽记）。wilsonLowerWinRate 是上游 0..1 原值，展示 ×100。
  function mayhemWilsonLabel(row) {
    if (String(row?.sampleTier || "") !== "high") return "";
    const value = Number(row?.wilsonLowerWinRate);
    if (!Number.isFinite(value) || value <= 0) return "";
    return percent(value * 100);
  }

  function mayhemWilsonTooltip(row) {
    const label = mayhemWilsonLabel(row);
    return label ? ` tabindex="0" data-tooltip="95% 置信区间下界 ${label}" data-tooltip-size="compact"` : "";
  }

  function renderMayhemOpeningConfiguration(build, citation) {
    const starters = objectRows(build.starterItems).slice(0, 2);
    const boots = objectRows(build.boots).slice(0, 2);
    const spells = objectRows(build.summonerSpells).slice(0, 2);
    const group = (title, rows, kind) => `<section><h4>${title}</h4><div class="config-option-list">${rows.map((row) => renderConfigOption(row, kind, false)).join("") || '<p class="muted">暂无样本</p>'}</div></section>`;
    // R116-B P0-4：只有召唤师技能这一组显示胜率。它的数据源已经换成同时带
    // winRate 与 pickRate 的那一份；出门装与鞋子仍然只有选用率，保持 false 不变
    // ——它们的数据里没有胜率，跟着打开就是显示不存在的数据（工单 P0-4-2）。
    // renderConfigOption 的三态开关本身一字未改，这里只用它的第四个参数
    // （statsRenderer，与 renderDepthStats 同一套用法）。
    //
    // 为什么不直接传 null（＝既有的 renderOptionStats）：那个函数只用
    // hasPick/hasWin 决定「整块要不要渲染」，之后两格是无条件输出的，所以一旦
    // 上游波动、后端回退到只有选用率的那一份数据（winRate 为 0），它会照渲染
    // 「胜率 0.00%」——那是显示不存在的数据。这里改成逐格判断：大于 0 才渲染，
    // 两格都取不到就整块返回空串（工单 P0-4 判据 2 的「回退成 pickRate-only」）。
    const spellStats = (row) => {
      const hasPick = Number(row?.pickRate) > 0;
      const hasWin = Number(row?.winRate) > 0;
      if (!hasPick && !hasWin) return "";
      return `<dl class="option-stats">${hasPick ? `<div class="is-pick"><dt>选用率</dt><dd>${percent(row.pickRate)}</dd></div>` : ""}${hasWin ? `<div class="is-win"><dt>胜率</dt><dd>${percent(row.winRate)}</dd></div>` : ""}</dl>`;
    };
    const spellGroup = (title, rows) => `<section><h4>${title}</h4><div class="config-option-list">${rows.map((row) => renderConfigOption(row, "spell", null, spellStats)).join("") || '<p class="muted">暂无样本</p>'}</div></section>`;
    return `<section class="recommendation-section champion-build-board mayhem-opening-config"><header><div><h3><span class="arena-section-icon" aria-hidden="true">✦</span>开局配置</h3><p>当前版本推荐出门装、鞋子与召唤师技能</p></div></header><div class="mayhem-opening-grid">${group("出门装", starters, "item")}${group("鞋子", boots, "item")}${spellGroup("召唤师技能", spells)}</div></section>`;
  }

  // 技能加点从「开局配置」搬到「构筑」tab（工单三-tab 表格：构筑 = 装备排行 +
  // 成装三件套 + 装备路线 + 技能加点）。取不到技能行时整块隐藏。
  function renderMayhemSkillPlan(build) {
    const skills = objectRows(build?.skills)[0];
    if (!skills) return "";
    return `<section class="recommendation-section champion-build-board mayhem-skill-plan"><header><div><h3><span class="arena-section-icon" aria-hidden="true">✧</span>技能加点</h3><p>升级优先级与加点顺序</p></div></header><div class="mayhem-build-grid"><section class="skill-plan">${renderChampionSkillPlan(skills, false)}</section></div></section>`;
  }

  function renderMayhemItemRoutes(build, citation) {
    const routes = mayhemRouteRows(objectRows(build.coreItems).filter((row) => objectRows(row.assets).length && objectRows(row.assets).every((asset) => asset.kind === "item"))).slice(0, CORE_RECOMMENDATION_LIMIT);
    return `<section class="recommendation-section champion-build-board mayhem-item-routes"><header><div><h3><span class="arena-section-icon" aria-hidden="true">◇</span>装备路线</h3><p>核心装备路线与后续成装顺序</p></div></header><div class="mayhem-build-grid"><section><div class="config-option-list">${routes.map((row) => renderConfigOption(row, "route", false)).join("") || '<p class="muted">暂无样本</p>'}</div></section></div></section>`;
  }

  // R116-B P0-2 排序护栏：这里以前调 sortedGradeRows（按 grade → score → games
  // 重排），同样忽略样本分档，低样本行能靠一个高 score 插队。海斗路径改成保持
  // 后端直出顺序，与对局推荐页 renderBuildRecommendation 的 inUpstreamOrder 一致。
  // sortedGradeRows 函数体一字未改：它同时服务 renderArenaAugmentSection 的
  // 斗魂路径（那边读的是 YOUR.GG/用户显式排序，属另一套口径，不许动）。
  function mayhemRouteRows(rows) {
    return objectRows(rows);
  }

  function renderMayhemAtlas(items) {
    if (state.mayhemAtlasLoading && !state.augments) return renderSkeleton();
    if (state.mayhemAtlasError && !state.augments) return renderError(state.mayhemAtlasError);
    const selected = items.find((item) => Number(item.id) === Number(state.mayhemAugmentID)) || null;
    const filters = [["all", "全部"], ["silver", "白银"], ["gold", "黄金"], ["prismatic", "棱彩"]].map(([value, label]) => {
      const active = state.augmentRarity === value;
      return `<button type="button" role="tab" aria-selected="${active}" class="${active ? "is-active" : ""}" data-augment-rarity="${value}"><i class="is-${value}" aria-hidden="true"></i>${label}</button>`;
    }).join("");
    return `<div class="mayhem-atlas">
      <section class="augment-directory mayhem-atlas-browser">
        <div class="mayhem-panel-head">
          <div><h3>海克斯图鉴</h3><p>${objectRows(state.augments?.rows).length} 个海克斯，按梯度与品质排列</p></div>
          <label class="champion-search compact"><span aria-hidden="true">⌕</span><span class="sr-only">搜索海克斯</span><input type="search" value="${escapeHTML(state.augmentQuery)}" placeholder="搜索名称或效果" autocomplete="off" data-augment-search></label>
        </div>
        <div class="augment-rarity-tabs mayhem-rarity-tabs" role="tablist" aria-label="海克斯品质">${filters}</div>
        <div class="mayhem-atlas-list" role="listbox" aria-label="海克斯列表">${items.length ? items.map((item) => renderMayhemAtlasRow(item, selected)).join("") : renderEmpty("没有匹配的海克斯", "更换品质或清除搜索词后重试。")}</div>
      </section>
      <section class="augment-directory mayhem-atlas-detail">${selected ? renderMayhemAtlasDetail(selected) : renderEmpty("选择一个海克斯", "点击左侧海克斯后按需读取 12 位适配英雄。")}</section>
      ${renderMayhemRarityPanel()}
    </div>`;
  }

  function renderMayhemAtlasRow(item, selected) {
    const active = Number(item.id) === Number(selected?.id);
    return `<button type="button" class="mayhem-atlas-row is-${escapeHTML(item.rarity || "unknown")}${active ? " is-active" : ""}" role="option" aria-selected="${active}" data-mayhem-augment="${Number(item.id)}">
      ${assetImage({ source: item.imageSource, path: item.imagePath, fallbackPath: item.imageFallbackPath, name: item.name, description: item.tooltip || item.description }, "augment-icon")}
      <span><strong>${escapeHTML(item.name)}</strong><small>${rarityLabel(item.rarity)} · ${augmentTierLabel(item.tier)} 级</small></span>
      <b class="augment-grade is-${augmentGrade(augmentTierLabel(item.tier))}">${augmentGrade(augmentTierLabel(item.tier))}</b>
    </button>`;
  }

  function renderMayhemAtlasDetail(item) {
    const detail = state.mayhemAugmentDetail;
    // 回退目录中的英雄适配信息也可直接展示，详情请求失败时不清空列表。
    const champions = objectRows(detail?.champions?.length ? detail.champions : item?.champions).slice(0, 12);
    const performance = Number(item.performance);
    const metrics = [
      Number.isFinite(performance) && performance > 0 ? `<span>综合评分 <b>${number(performance, 1)}</b></span>` : "",
      Number(item.winRate) > 0 ? `<span>胜率 <b class="win-rate-value">${percent(item.winRate)}</b></span>` : "",
    ].filter(Boolean).join("");
    const body = state.mayhemAugmentLoading && !detail
      ? renderDetailSkeleton()
      : state.mayhemAugmentError && !detail
        ? renderError(state.mayhemAugmentError, true)
        : `${state.mayhemAugmentError ? `<p class="mayhem-detail-stale" role="status">${escapeHTML(state.mayhemAugmentError)}，已保留上次适配数据。</p>` : ""}<div class="mayhem-fit-head"><div><h4>适配英雄</h4><p>按综合评分、胜率与样本展示</p></div><span>${champions.length} 位</span></div>
          <div class="mayhem-fit-list">${champions.length ? champions.map((champion, index) => {
            const meta = championMeta(champion.id);
            const score = champion.score ?? champion.performance;
            const winRate = champion.winRate ?? champion.win_rate;
            const games = champion.games ?? champion.play;
            return `<div class="mayhem-fit-row"><b>${index + 1}</b>${assetImage({ source: champion.imageSource || meta?.imageSource, path: champion.imagePath || meta?.imagePath, name: champion.name || meta?.nameZh || meta?.titleZh }, "augment-champion-icon")}<strong>${escapeHTML(champion.name || meta?.nameZh || meta?.titleZh || `英雄 ${champion.id}`)}</strong><span class="mayhem-fit-metrics">${score != null ? `综合评分 ${number(score, 1)} · ` : ""}${winRate != null ? `胜率 <b class="win-rate-value">${percent(winRate)}</b> · ` : ""}${games != null ? `${compactNumber(games)} 场` : "暂无样本"}</span></div>`;
          }).join("") : '<p class="mayhem-inline-empty">当前没有可展示的适配英雄样本。</p>'}</div>`;
    return `<header class="mayhem-atlas-detail-head is-${escapeHTML(item.rarity || "unknown")}">
        ${assetImage({ source: item.imageSource, path: item.imagePath, fallbackPath: item.imageFallbackPath, name: item.name, description: item.tooltip || item.description }, "augment-icon")}
        <div><span class="rarity-label is-${escapeHTML(item.rarity || "unknown")}">${rarityLabel(item.rarity)}</span><h3>${escapeHTML(item.name)}</h3><p>${augmentTierLabel(item.tier)} 级海克斯${metrics ? ` · ${metrics}` : ""}</p></div>
      </header>
      <div class="mayhem-atlas-description">${escapeHTML(mayhemAtlasDescription(item))}</div>
      <div class="mayhem-atlas-detail-body">${body}</div>`;
  }

  // 说明只认海克斯本身的效果文案。详情接口以前回的是页面 SEO 摘要
  // （"…海克斯大乱斗胜率 53.2%，选取率 0.4%…"），那是数据口径不是说明。
  function mayhemAtlasDescription(item) {
    const detail = state.mayhemAugmentDetail;
    const fromDetail = String(detail?.description || "").trim();
    if (fromDetail) return fromDetail;
    const fromCatalog = [item.description, item.tooltip].map((value) => String(value || "").trim())
      .find((value) => value && value !== AUGMENT_DESCRIPTION_PENDING);
    if (fromCatalog) return fromCatalog;
    if (state.mayhemAugmentLoading) return "正在读取说明…";
    if (detail) return "上游未提供这个海克斯的说明。";
    return AUGMENT_DESCRIPTION_PENDING;
  }

  // 上游对海斗海克斯没有说明字段，后端在这种情况下回这句占位；它不是说明，
  // 渲染时要当成"还没有说明"处理。与 champions.go 的 augmentOfflineDescription 一致。
  const AUGMENT_DESCRIPTION_PENDING = "海克斯图鉴中可读取说明";

  function renderMayhemRarityPanel() {
    const stages = objectRows(state.mayhemRarityData?.stages);
    // 用户要求：拿不到就别展示。读取失败时整块隐藏，不留一个报错的空壳。
    if (state.mayhemRarityError && !stages.length) return "";
    let body = '<p class="mayhem-inline-empty">正在读取品质分布…</p>';
    if (state.mayhemRarityLoading) body = '<p class="mayhem-inline-empty">正在读取品质分布…</p>';
    else if (stages.length) body = `<div class="mayhem-rarity-stages">${stages.map((row) => `<article><b>第 ${Number(row.stage)} 阶段</b><span class="is-silver">白银 ${percent(row.silver)}</span><span class="is-gold">黄金 ${percent(row.gold)}</span><span class="is-prismatic">棱彩 ${percent(row.prismatic)}</span><small>${compactNumber(row.games)} 次选择</small></article>`).join("")}</div>`;
    return `<section class="augment-directory mayhem-rarity-panel"><header><div><h3>海克斯品质分布</h3></div></header>${body}</section>`;
  }

  // ---- R116-F P1：该英雄专属的「阶段 × 稀有度」概率（英雄口径）----
  // 与上面那份全服口径并存、互不替换：全服口径（/api/champions/augment-rarity，
  // 渲染在海克斯图鉴里）回答「整体分布」，这一份回答「这个英雄」。两者并存是工单
  // 要求，但实测差异并不大：patch 16.18 全服阶段 1 是白银 8.4% / 黄金 46.6% /
  // 棱彩 45.0%，英雄 157 同阶段是 8.12 / 44.90 / 46.98（差 ≤2 个百分点），阶段 2-4
  // 差 ≤0.4 个百分点（四个英雄 157/223/17/875 实测，见执行账本 §3.4）。所以文案
  // 只说「该英雄样本里」，不宣称英雄口径与全服口径有量级差别——那是没有证据的话。
  // 数值由后端从 hero-json 的 augments[].stages[].pickRate 本地聚合好直出（百分数，
  // 与全服口径同单位，前端直接 percent() 渲染），前端不重算、也不为它多发一条请求。
  // 取不到就整块隐藏，不留一个空壳（评审 6.1）。
  function renderMayhemHeroRarityPanel(rows) {
    const stages = objectRows(rows).filter((row) => Number(row?.stage) > 0 && mayhemHeroRarityTotal(row) > 0);
    if (!stages.length) return "";
    return `<section class="recommendation-section mayhem-hero-rarity"><header><div><h3><span class="arena-section-icon" aria-hidden="true">◈</span>该英雄专属品质概率</h3><p>各阶段白银 / 黄金 / 棱彩选取概率</p></div><span class="section-count">${stages.length} 个阶段</span></header><div class="mayhem-rarity-stages">${stages.map(mayhemHeroRarityStage).join("")}</div></section>`;
  }

  // 三组之和。后端已经保证只有 Total > 0 的阶段才下发，这里再算一次是给前端一道
  // 自己的护栏：任何一组缺失都会让合计明显小于 100%，用户能直接看出来。
  function mayhemHeroRarityTotal(row) {
    return [row?.silver, row?.gold, row?.prismatic].reduce((total, value) => total + (Number(value) || 0), 0);
  }

  // 单阶段一列，形态与全服口径那一份一致（复用同一套 .mayhem-rarity-stages 样式），
  // 两个视图并排看时不需要重新学习。小字披露这一阶段真实参与聚合的海克斯条数与三组
  // 之和：实测阶段 1 只有 121/126 条有 stage 行，缺失的行是跳过的、不是按 0 计入的，
  // 把条数写出来用户才知道分母是多少。
  function mayhemHeroRarityStage(row) {
    const cell = (label, value, tone) => `<span class="is-${tone}">${label} ${percent(value)}</span>`;
    const count = Number(row?.augments) > 0 ? `${compactNumber(row.augments)} 项海克斯` : "";
    const total = mayhemHeroRarityTotal(row);
    const sum = total > 0 ? `三项合计 ${percent(total)}` : "";
    const note = [count, sum].filter(Boolean).join(" · ");
    return `<article><b>第 ${Number(row.stage)} 阶段</b>${cell("白银", row.silver, "silver")}${cell("黄金", row.gold, "gold")}${cell("棱彩", row.prismatic, "prismatic")}${note ? `<small>${note}</small>` : ""}</article>`;
  }

  function renderArena() {
    const rows = filteredChampionRows();
    const selectedRow = rows.find((row) => Number(row.championId) === Number(state.selected?.championId)) || null;
    const selected = selectedRow && state.selected ? arenaSelectedRow({ ...state.selected, ...selectedRow }) : null;
    const list = `<section class="champion-list-card aram-champions arena-champions">
      <div class="aram-champion-head"><div><h3>英雄梯度</h3><p>胜率与海克斯 · ${escapeHTML(currentModePatch())}</p></div><span class="champion-result-count">${rows.length} 位</span></div>
      <label class="champion-search compact"><span aria-hidden="true">⌕</span><span class="sr-only">搜索英雄</span><input type="search" value="${escapeHTML(state.query)}" placeholder="搜索英雄" autocomplete="off" data-champion-search></label>
      <div data-champion-results>${renderChampionResults(rows)}</div>
    </section>`;
    return `<div class="arena-redesign-workspace">
      ${list}<span data-arena-list-marker hidden></span>
      <div class="arena-detail-pane">
        <div class="arena-tier-entry"><button class="button button-primary tier-trigger" type="button" aria-haspopup="dialog" aria-controls="arena-tier-dialog" data-open-arena-tiers>英雄梯度</button></div>
        ${selected ? renderArenaDetailPane(selected) : '<div class="arena-pane-empty"><strong>选择一位英雄</strong><span>查看海克斯、装备与吃鸡战绩</span></div>'}
      </div>
      <dialog class="mayhem-tier-dialog arena-tier-dialog" id="arena-tier-dialog" aria-labelledby="arena-tier-dialog-title"><div class="mayhem-tier-dialog-sheet"><header><div><h2 id="arena-tier-dialog-title">英雄梯度</h2><p>胜率与海克斯 · ${escapeHTML(currentModePatch())}</p></div><button class="icon-button control-icon-button tier-dialog-close" type="button" aria-label="关闭英雄梯度" data-close-arena-tiers><svg class="control-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button></header><div class="arena-tier-dialog-content"></div></div></dialog>
    </div>`;
  }

  function renderArenaDetailPane(row = state.selected) {
    if (!row) return '<div class="arena-pane-empty"><strong>选择一位英雄</strong><span>查看海克斯、装备与吃鸡战绩</span></div>';
    const meta = row.meta || championMeta(row.championId);
    const detail = state.detail;
    const stats = detail?.arenaStats || {};
    const title = row.name || meta?.nameZh || meta?.titleZh || `英雄 ${row.championId}`;
    const subtitle = meta?.titleZh && meta.titleZh !== title ? meta.titleZh : meta?.nameEn || "";
    const source = row.imageSource || meta?.imageSource;
    const path = row.imagePath || meta?.imagePath;
    const tier = stats.tier ?? row.tier;
    const rank = Number(stats.rank || row.rank) || 0;
    const hasMetric = (value) => value !== null && value !== undefined && String(value).trim() !== "" && Number.isFinite(Number(value));
    const secondaryMetrics = [
      ["选用率", "metric-pick", stats.pickRate ?? row.pickRate, percent, ""],
      ["禁用率", "metric-ban", stats.banRate ?? row.banRate, percent, ""],
      ["样本", "", stats.games ?? row.play, compactNumber, " 场"],
    ].filter(([, , value]) => hasMetric(value)).map(([label, className, value, formatter, suffix]) => `<span>${label} <b${className ? ` class="${className}"` : ""}>${formatter(value)}</b>${suffix}</span>`).join("");
    const overview = `<section class="arena-overview-strip">
      <img class="arena-overview-art" data-queued-src="${heroArtworkURL(meta, source, path)}" data-artwork-fallback="${escapeHTML(heroArtworkFallbackURL(meta))}" alt="" aria-hidden="true" decoding="async" data-champion-image>
      <span class="arena-overview-shade" aria-hidden="true"></span>
      <div class="arena-overview-identity"><span class="champion-detail-portrait"><img data-queued-src="${imageURL(source, path)}" alt="${escapeHTML(title)}" decoding="async" data-champion-image><span>${escapeHTML(title.slice(0, 1))}</span></span><div><h2>${escapeHTML(title)} ${tierBadge(tier, "arena-title-tier")}</h2><small>${escapeHTML(subtitle)}${rank ? ` · 总榜第 ${rank} 位` : ""}</small></div></div>
      <div class="arena-overview-metrics">
        ${arenaOverviewMetric("梯度", tierBadge(tier, "is-metric"), "tier", tierDisplay(tier) === "—" ? "暂无梯度" : `${tierDisplay(tier)} 档`)}
        ${arenaOverviewMetric("胜率", percent(stats.winRate || row.winRate), "win", rank ? `总榜第 ${rank} 名` : "全球样本")}
        ${arenaOverviewMetric("平均名次", number(stats.averagePlacement || row.averagePlacement, 2), "placement", "名次越低越好")}
        ${arenaOverviewMetric("吃鸡率", percent(stats.firstPlaceRate || row.firstPlaceRate), "first", `${compactNumber(stats.games || row.play)} 场样本`)}
      </div>
      <div class="arena-overview-secondary">${secondaryMetrics}${renderArenaSortBar()}</div>
    </section>`;
    if (state.arenaDetailLoading) return overview + renderDetailSkeleton();
    if (state.arenaDetailError) return overview + `<div class="arena-detail-content">${renderError(state.arenaDetailError, true)}${renderArenaFirstPlaces({})}</div>`;
    if (!detail) return overview + `<div class="arena-detail-content"><div class="arena-pane-empty"><strong>详情暂时不可用</strong><span>左侧英雄梯度仍可继续浏览</span></div>${renderArenaFirstPlaces({})}</div>`;
    return overview + renderArenaDetailContent(detail);
  }

  function arenaOverviewMetric(label, value, tone, note) {
    return `<div class="is-${tone}"><span>${label}</span><strong>${value}</strong><small>${escapeHTML(note)}</small></div>`;
  }

  function renderArenaDetailContent(detail) {
    const sections = [
      renderArenaAugmentSection(detail),
      renderArenaItemSection("棱彩装备", "单件", detail.build?.prismItems || [], "prism"),
      renderArenaCoreSection(detail.build || {}),
    ];
    if (detail.teamCompositions?.length) sections.push(renderArenaSynergySection(detail.teamCompositions));
    sections.push(renderArenaFirstPlaces(detail));
    return `<div class="arena-detail-content">${sections.filter(Boolean).join("")}</div>`;
  }

  function renderArenaSortBar() {
    const options = [["winRate", "按胜率"], ["firstPlaceRate", "按吃鸡率"], ["averagePlacement", "按平均名次"], ["games", "按场次"]];
    return `<div class="arena-chips arena-sort-chips" role="group" aria-label="竞技场数据排序">${options.map(([value, label]) => `<button type="button" class="${state.arenaSort === value ? "is-active" : ""}" data-arena-sort="${value}">${label}</button>`).join("")}</div>`;
  }

  function sortedArenaRows(rows) {
    const metric = state.arenaSort;
    return [...(rows || [])].sort((left, right) => {
      const leftValue = Number(left?.[metric]) || 0;
      const rightValue = Number(right?.[metric]) || 0;
      const primary = metric === "averagePlacement" ? leftValue - rightValue : rightValue - leftValue;
      return primary || (Number(right?.games) || 0) - (Number(left?.games) || 0);
    });
  }
  function gradeRank(value) { return ({ OP: 0, S: 1, A: 2, B: 3, C: 4, D: 5, F: 6 })[String(value || "B").toUpperCase()] ?? 7; }
  function sortedGradeRows(rows) {
    const list = objectRows(rows);
    const scores = list.map((row) => row.score).filter((value) => Number(value) > 0);
    return [...list].sort((left, right) => {
      const grade = gradeRank(augmentGrade(left.tier || left.grade, left.score, scores)) - gradeRank(augmentGrade(right.tier || right.grade, right.score, scores));
      return grade || (Number(right.score) || 0) - (Number(left.score) || 0) || (Number(right.games) || 0) - (Number(left.games) || 0);
    });
  }

  function arenaRarityKey(value) {
    const key = String(value ?? "").toLowerCase();
    return ({ 0: "silver", 1: "silver", 2: "prismatic", 4: "gold", 8: "prismatic", silver: "silver", gold: "gold", prismatic: "prismatic" })[key] || "unknown";
  }

  function renderArenaAugmentSection(detail) {
    const groups = detail.arenaAugmentGroups?.length
      ? detail.arenaAugmentGroups
      : [{ rarity: 0, rows: detail.arenaAugments || [] }];
    const allRows = groups.flatMap((group) => (group.rows || []).map((row) => ({ ...row, rarity: arenaRarityKey(row.rarity) !== "unknown" ? arenaRarityKey(row.rarity) : arenaRarityKey(group.rarity) })));
    const filtered = state.arenaRarity === "all" ? allRows : allRows.filter((row) => arenaRarityKey(row.rarity) === state.arenaRarity);
    const sorted = sortedGradeRows(filtered);
    const visible = state.arenaExpanded.augments ? sorted : sorted.slice(0, 9);
    const scores = sorted.map((row) => row.score);
    const filters = [["all", "全部"], ["silver", "银色"], ["gold", "黄金"], ["prismatic", "棱彩"]];
    return `<section class="arena-data-section arena-augment-section"><header><div><span class="arena-section-icon" aria-hidden="true">✦</span><span><h3>海克斯推荐</h3><small>按综合评分、胜率与样本展示前九项 · ${allRows.length} 条</small></span></div><div class="arena-chips arena-rarity-chips" role="tablist" aria-label="海克斯品质">${filters.map(([value, label]) => `<button type="button" role="tab" aria-selected="${state.arenaRarity === value}" class="${state.arenaRarity === value ? "is-active" : ""}" data-arena-rarity="${value}"><i class="is-${value}" aria-hidden="true"></i>${label}</button>`).join("")}</div></header>${visible.length ? `<div class="arena-option-grid">${visible.map((row, index) => renderArenaOptionCard(row, "augment", index, scores)).join("")}</div>${renderArenaExpand("augments", sorted.length, 9)}` : '<div class="arena-section-empty">该品质暂无海克斯样本</div>'}</section>`;
  }

  // YOUR.GG's default equipment order is tier, then sample count (not score).
  // Keep this separate from augment/mayhem scoring and retain every item.
  function sortedArenaItemRows(rows) {
    const tier = (value) => ({ OP: 0, S: 1, A: 2, B: 3, C: 4, D: 5, F: 6 })[String(value || "").toUpperCase()] ?? 99;
    return [...objectRows(rows)].sort((left, right) =>
      tier(left.tier || left.grade) - tier(right.tier || right.grade)
        || (Number(right.games) || 0) - (Number(left.games) || 0));
  }

	function renderArenaItemSection(title, copy, rows, kind) {
    const sorted = sortedArenaItemRows(rows);
		const visible = state.arenaExpanded[kind] ? sorted : sorted.slice(0, 6);
		const scores = sorted.map((row) => row.score);
		if (!sorted.length) return "";
		return `<section class="arena-data-section arena-${kind}-section"><header><div><span class="arena-section-icon" aria-hidden="true">◈</span><span><h3>${title}</h3><small>${copy} · ${sorted.length} 条</small></span></div></header><div class="arena-option-grid">${visible.map((row, index) => renderArenaOptionCard(row, kind, index, scores)).join("")}</div>${renderArenaExpand(kind, sorted.length)}</section>`;
  }

  function renderArenaCoreSection(build) {
    const sorted = sortedArenaItemRows(build.coreItems || []);
    const visible = state.arenaExpanded.core ? sorted : sorted.slice(0, 6);
    const scores = sorted.map((row) => row.score);
    if (!sorted.length && !(build.boots || []).length) return "";
    return `<section class="arena-data-section arena-core-section"><header><div><span class="arena-section-icon" aria-hidden="true">◇</span><span><h3>核心物品</h3><small>核心装备统计 · ${sorted.length} 条</small></span></div></header>${visible.length ? `<div class="arena-option-grid">${visible.map((row, index) => renderArenaOptionCard(row, "core", index, scores)).join("")}</div>${renderArenaExpand("core", sorted.length)}` : ""}<div class="arena-slim-rows">${renderArenaSlimRow(sortedArenaRows(build.boots || []).slice(0, 3))}</div></section>`;
  }

  // 斗魂与海斗共用同一张海克斯卡片：DOM 与类名一致，样式就不会再走偏。
  // 两个模式的指标口径不同（海斗是大乱斗，没有名次/吃鸡率），所以指标列可
  // 由调用方给定，其余（图标尺寸、徽章、名称字号、品质芯片）全部共享。
  function renderArenaOptionCard(row, kind, index, scores = [], options) {
    const assets = row.assets || [];
    const route = assets.map((asset, assetIndex) => `${assetIndex ? '<span class="route-arrow" aria-hidden="true">›</span>' : ""}${renderAssetButton(asset)}`).join("");
    const name = assets.length === 1 ? assets[0]?.name || "未知选项" : "";
    const rarityKey = kind === "augment" ? arenaRarityKey(row.rarity) : "";
    const rarity = rarityKey ? ` is-${rarityKey}` : "";
    const quality = ({ silver: "银色", gold: "黄金", prismatic: "棱彩" })[rarityKey] || "";
    const grade = augmentGrade(row.tier || row.grade, row.score, scores);
    // options.hideGradeBadge（R116-B 评审整改 B4）：只有海斗的阶段视图会用——阶段行
    // 没有自己的官方字母档位时整块隐藏徽章，而不是拿英雄级的字母去配阶段级的
    // 「官方档位」。不传这个选项时行为与从前逐字一致（斗魂/棱彩/核心装备照旧）。
    const gradeBadge = options?.hideGradeBadge ? "" : `<b class="augment-grade is-${grade}">${grade}</b>`;
    const badge = kind === "augment" || kind === "prism" || kind === "core"
      ? gradeBadge
      : `<b class="hex-rank ${index < 3 ? `is-${index + 1}` : "is-rest"}">${index + 1}</b>`;
    const primary = kind === "augment" || kind === "prism" || kind === "core"
      ? `<div class="arena-option-icons">${route}</div>${badge}`
      : `${badge}<div class="arena-option-icons">${route}</div>`;
    const metrics = options?.metrics || [
      ["胜率", percent(row.winRate), "is-win"],
      ["平均名次", number(row.averagePlacement, 2), "is-placement"],
      ["吃鸡率", percent(row.firstPlaceRate), "is-first"],
      ["样本", compactNumber(row.games), ""],
      ["综合评分", number(row.score, 2), "is-score"],
    ];
    const cells = metrics.map(([label, value, tone]) => `<div${tone ? ` class="${tone}"` : ""}><dt>${escapeHTML(label)}</dt><dd>${value}</dd></div>`).join("");
    const extra = options?.className ? ` ${options.className}` : "";
    return `<article class="arena-option-card${kind === "core" ? " arena-core-option" : ""}${rarity}${extra}" data-metric-count="${metrics.length}"><div class="arena-option-main">${primary}${name ? `<strong>${escapeHTML(name)}</strong>` : ""}${quality ? `<small class="arena-option-rarity is-${rarityKey}">${quality}</small>` : ""}</div><dl>${cells}</dl></article>`;
  }

  function renderArenaExpand(kind, count, previewLimit = 6) {
    if (count <= previewLimit) return "";
    const expanded = state.arenaExpanded[kind];
    return `<button class="arena-expand-button" type="button" data-arena-expand="${kind}">${expanded ? "收起" : `展开全部 ${count} 条`}</button>`;
  }

  function renderArenaSlimRow(rows) {
    if (!rows.length) return "";
    return `<div class="arena-slim-row"><div class="arena-slim-grid">${rows.map((row, index) => `<article><b>#${index + 1}</b><span class="arena-slim-icons">${(row.assets || []).map(renderAssetButton).join("")}</span><span class="is-win">${percent(row.winRate)}</span><span class="is-placement">${number(row.averagePlacement, 2)} 名</span><small>吃鸡 ${percent(row.firstPlaceRate)} · ${compactNumber(row.games)} 次选择</small></article>`).join("")}</div></div>`;
  }

  function renderArenaSynergySection(teams) {
    const rows = teams.slice(0, 6);
    return `<section class="arena-data-section arena-synergy-section"><header><div><span class="arena-section-icon" aria-hidden="true">+</span><span><h3>搭档协同</h3><small>与当前英雄搭配的高样本组合</small></span></div><span class="section-count">${teams.length} 组</span></header><div class="arena-synergy-grid">${rows.map((team, index) => `<article><b>#${index + 1}</b>${renderArenaTeamFaces(team.champions)}${renderArenaTeamNames(team)}<span class="is-win">${percent(team.winRate)}</span><span class="is-placement">${number(team.averagePlacement, 2)} 名</span><small>吃鸡 ${percent(team.firstPlaceRate)} · ${compactNumber(team.games)} 场</small></article>`).join("")}</div></section>`;
  }

  function arenaFirstFailureCopy(message) {
    const value = String(message || "").toLowerCase();
    if (value.includes("403") || value.includes("拒绝")) return "高手对局数据源拒绝了本次请求（HTTP 403），请稍后重试。";
    if (value.includes("404") || value.includes("不存在")) return "暂未提供该英雄的高手对局（HTTP 404）。";
    if (value.includes("超时") || value.includes("timeout") || value.includes("deadline")) return "高手对局读取超时，已保留本地战绩。";
    if (value.includes("空响应")) return "高手对局返回了空响应，已保留本地战绩。";
    return "高手对局读取失败，已保留本地战绩。";
  }

  function arenaFirstUnavailableCopy(reason) {
    return ({
      empty: "当前没有返回该英雄的高手对局。",
      "champion-metadata-mismatch": "高手对局中的英雄版本尚未同步，数据暂时无法安全匹配。",
      "champion-mismatch": "高手对局返回的英雄与当前选择不一致，已忽略这批数据。",
      "no-first-place": "高手样本中暂时没有第一名对局。",
      "invalid-upstream-data": "高手对局的数据格式暂时无法识别。",
    })[reason] || "";
  }

  function renderArenaFirstPlaces(detail = {}) {
    const pros = state.arenaFirstPlaces?.matches || [];
    const mine = state.arenaLocalMatches?.matches || [];
    const prosAvailable = pros.length > 0 || state.arenaFirstLoading;
    const active = state.arenaFirstTab === "pros" && prosAvailable ? "pros" : "mine";
    const name = state.selected?.name || state.selected?.meta?.nameZh || "该英雄";
    const firstRate = detail?.arenaStats?.firstPlaceRate || state.selected?.firstPlaceRate;
    const tabs = `<div class="arena-first-tabs" role="tablist" aria-label="吃鸡战绩来源"><button type="button" role="tab" aria-selected="${active === "pros"}" class="${active === "pros" ? "is-active" : ""}" data-arena-first-tab="pros" data-tooltip="韩服高手样本" ${prosAvailable ? "" : "disabled"}>高手对局</button><button type="button" role="tab" aria-selected="${active === "mine"}" class="${active === "mine" ? "is-active" : ""}" data-arena-first-tab="mine">我的吃鸡</button></div>`;
    let body;
    if (active === "pros" && state.arenaFirstLoading && !pros.length) {
      body = '<div class="arena-match-empty is-loading"><strong>正在读取高手对局</strong><span>详情与本地战绩会独立更新。</span></div>';
    } else if (active === "pros") {
      body = '<div class="match-list arena-match-list" data-arena-match-list="pros"></div>';
    } else if (mine.length) {
      body = `<div class="match-list arena-match-list" data-arena-match-list="mine"></div>`;
    } else if (state.arenaLocalLoading) {
      body = '<div class="arena-match-empty is-loading"><strong>正在读取我的吃鸡</strong><span>高手对局与英雄详情无需等待本地战绩。</span></div>';
    } else {
      const copy = state.arenaLocalMatches?.loaded === false ? "总览战绩尚未加载，连接客户端后可读取本地对局。" : `${name} 的全球吃鸡率为 ${percent(firstRate)}，当前已读取战绩里还没有第一名。`;
      body = `<div class="arena-match-empty"><strong>还没有用${escapeHTML(name)}拿过第一名</strong><span>${escapeHTML(copy)}</span></div>`;
    }
    const unavailable = arenaFirstUnavailableCopy(state.arenaFirstPlaces?.unavailableReason);
    const degradedCopy = state.arenaFirstError ? arenaFirstFailureCopy(state.arenaFirstError) : unavailable;
    const degraded = degradedCopy && !pros.length ? `<p class="arena-first-degraded" role="status">${escapeHTML(degradedCopy)}</p>` : "";
    return `<section class="arena-data-section arena-first-section"><header><div><span class="arena-section-icon" aria-hidden="true">1</span><span><h3>吃鸡战绩</h3><small>该英雄最近拿到第一名的对局</small></span></div>${tabs}</header>${degraded}${body}</section>`;
  }

  function mountArenaMatchCards() {
    const matchCards = window.deepLegendsMatchCards;
    if (!matchCards?.mount) return;
    for (const container of root.querySelectorAll("[data-arena-match-list]")) {
      const kind = container.dataset.arenaMatchList;
      if (kind === "pros") {
        matchCards.mount(container, {
          key: `arena-pros-${state.selected?.championId}`,
          matches: state.arenaFirstPlaces?.matches || [], region: "kr",
          disableReplay: true,
          replayDisabledReason: "外部样本不支持回放",
          loadMatchDetails: (match) => {
            const gameID = Number(match?.gameId);
            if (!Number.isSafeInteger(gameID) || gameID <= 0) throw new Error("对局编号无效");
            return api(`/api/champions/arena/match/KR_${gameID}`, `arena-match-${gameID}`);
          },
        });
      } else {
        const local = state.arenaLocalMatches || {};
        matchCards.mount(container, { key: `arena-mine-${state.selected?.championId}`, matches: local.matches || [], playerRef: local.playerRef || "", region: local.region || "", serverId: local.serverId || "" });
      }
    }
  }

  function mountMayhemTierDialog() {
    if (state.mode !== "aram-mayhem" || state.mayhemView !== "champions") return;
    const dialog = root.querySelector("#mayhem-tier-dialog");
    const list = root.querySelector(".mayhem-champions");
    const content = dialog?.querySelector(".mayhem-tier-dialog-content");
    const marker = root.querySelector("[data-mayhem-list-marker]");
    if (!dialog || !list || !content || !marker) return;
    dialog.addEventListener("close", () => {
      state.mayhemDialogOpen = false;
      if (marker.isConnected && list.isConnected) marker.before(list);
      const returnFocus = state.mayhemDialogReturnFocus;
      state.mayhemDialogReturnFocus = null;
      const focusTarget = returnFocus?.isConnected ? returnFocus : returnFocus ? root.querySelector("[data-open-mayhem-tiers]") : null;
      focusTarget?.focus?.({ preventScroll: true });
    }, { once: true });
    if (state.mayhemDialogOpen) {
      content.append(list);
      if (!dialog.open) dialog.showModal();
    }
  }

  function openMayhemTierDialog(trigger) {
    const dialog = root.querySelector("#mayhem-tier-dialog");
    const list = root.querySelector(".mayhem-champions");
    const content = dialog?.querySelector(".mayhem-tier-dialog-content");
    if (!dialog || !list || !content) return;
    state.mayhemDialogOpen = true;
    state.mayhemDialogReturnFocus = trigger || document.activeElement;
    content.append(list);
    if (!dialog.open) dialog.showModal();
    list.querySelector("[data-champion-search]")?.focus({ preventScroll: true });
  }

  function closeMayhemTierDialog(restoreFocus = true) {
    state.mayhemDialogOpen = false;
    const dialog = root.querySelector("#mayhem-tier-dialog");
    const list = root.querySelector(".mayhem-champions");
    const marker = root.querySelector("[data-mayhem-list-marker]");
    if (!restoreFocus) state.mayhemDialogReturnFocus = null;
    if (marker && list) marker.before(list);
    if (dialog?.open) dialog.close();
  }

  function mountArenaTierDialog() {
    if (state.mode !== "arena") return;
    const dialog = root.querySelector("#arena-tier-dialog");
    const list = root.querySelector(".arena-champions");
    const content = dialog?.querySelector(".arena-tier-dialog-content");
    const marker = root.querySelector("[data-arena-list-marker]");
    if (!dialog || !list || !content || !marker) return;
    dialog.addEventListener("close", () => {
      state.arenaDialogOpen = false;
      if (marker.isConnected && list.isConnected) marker.before(list);
      const returnFocus = state.arenaDialogReturnFocus;
      state.arenaDialogReturnFocus = null;
      const focusTarget = returnFocus?.isConnected ? returnFocus : returnFocus ? root.querySelector("[data-open-arena-tiers]") : null;
      focusTarget?.focus?.({ preventScroll: true });
    }, { once: true });
    if (state.arenaDialogOpen) {
      content.append(list);
      if (!dialog.open) dialog.showModal();
    }
  }

  function openArenaTierDialog(trigger) {
    const dialog = root.querySelector("#arena-tier-dialog");
    const list = root.querySelector(".arena-champions");
    const content = dialog?.querySelector(".arena-tier-dialog-content");
    if (!dialog || !list || !content) return;
    state.arenaDialogOpen = true;
    state.arenaDialogReturnFocus = trigger || document.activeElement;
    content.append(list);
    if (!dialog.open) dialog.showModal();
    list.querySelector("[data-champion-search]")?.focus({ preventScroll: true });
  }

  function closeArenaTierDialog(restoreFocus = true) {
    state.arenaDialogOpen = false;
    const dialog = root.querySelector("#arena-tier-dialog");
    const list = root.querySelector(".arena-champions");
    const marker = root.querySelector("[data-arena-list-marker]");
    if (!restoreFocus) state.arenaDialogReturnFocus = null;
    if (marker && list) marker.before(list);
    if (dialog?.open) dialog.close();
  }

  function renderArenaTeamFaces(champions, large = false) {
    return `<div class="arena-team-faces${large ? " is-large" : ""}">${(champions || []).slice(0, 3).map((champion) => `<span class="champion-portrait"><img data-queued-src="${imageURL(champion.imageSource, champion.imagePath)}" alt="${escapeHTML(champion.name || "英雄")}" loading="lazy" decoding="async" data-champion-image><span>${escapeHTML((champion.name || "?").slice(0, 1))}</span></span>`).join("")}</div>`;
  }

  function renderArenaTeamNames(team) {
    const names = (team?.champions || []).map((champion) => String(champion?.name || "").trim()).filter(Boolean).slice(0, 2);
    const label = names.join(" + ") || "未知英雄";
    return `<span class="arena-team-name" data-tooltip="${escapeHTML(label)}" data-tooltip-size="compact">${escapeHTML(label)}</span>`;
  }

  function teamName(team) {
    const name = (team?.champions || []).map((champion) => champion.name).filter(Boolean).join(" + ") || "未知队伍";
    return escapeHTML(name);
  }

  function renderAugmentGroups(items) {
    const groups = new Map();
    for (const item of items) {
      const tier = Number(item.tier) || 0;
      if (!groups.has(tier)) groups.set(tier, []);
      groups.get(tier).push(item);
    }
    return `<div class="augment-tier-groups">${[...groups.entries()].sort((a, b) => a[0] - b[0]).map(([tier, rows]) => `<section class="augment-tier-group"><header><span class="augment-tier-letter is-${Math.min(5, tier)}">${augmentTierLabel(tier)}</span><div><h4>${augmentTierLabel(tier)} 级海克斯</h4><p>${rows.length} 个，优先展示更高品质</p></div></header><div class="augment-grid">${rows.map(renderAugmentCard).join("")}</div></section>`).join("")}</div>`;
  }

  function renderChampionTable(rows, metrics, rankOffset = 0) {
    const arena = metrics === "arena";
    const mayhem = metrics === "mayhem";
    const rankedMetrics = metrics === true;
    const showPosition = rankedMetrics && state.position === "all";
    const metricHeaders = mayhem
      ? '<th class="metric-win">胜率</th>'
      : arena
      ? '<th class="metric-win">胜率</th><th class="metric-placement">均名次</th>'
      : rankedMetrics ? '<th class="metric-win">胜率</th><th class="metric-pick">选用率</th><th class="metric-ban">禁用率</th><th class="metric-games">场次</th>' : "";
    return `<div class="champion-table-scroll"><table class="champion-table${showPosition ? " has-position" : ""}${arena ? " is-arena-table" : ""}${mayhem ? " is-mayhem-table" : ""}"><thead><tr><th>排名</th><th>英雄</th><th>梯度</th>${showPosition ? "<th>位置</th>" : ""}${metricHeaders}</tr></thead><tbody>${rows.map((row, index) => renderChampionRow(row, metrics, showPosition, index + rankOffset)).join("")}</tbody></table></div>`;
  }

  function renderChampionRow(row, metrics, showPosition, index) {
    const arena = metrics === "arena";
    const mayhem = metrics === "mayhem";
    const rankedMetrics = metrics === true;
    const meta = championMeta(row.championId);
    const name = row.name || meta?.nameZh || meta?.titleZh || `英雄 ${row.championId}`;
    const subname = meta?.titleZh && meta.titleZh !== name ? meta.titleZh : meta?.nameEn || "";
    const source = row.imageSource || meta?.imageSource;
    const path = row.imagePath || meta?.imagePath;
    const rank = Number(row.rank) > 0 ? row.rank : index + 1;
    const selected = Number(state.selected?.championId) === Number(row.championId);
    const rowClass = selected ? "is-selected" : "";
    const artwork = arena ? `<img class="champion-row-art" data-queued-src="${heroArtworkURL(meta, source, path)}" alt="" aria-hidden="true" loading="lazy" decoding="async" data-champion-image>` : "";
    return `<tr class="champion-row${rowClass ? ` ${rowClass}` : ""}" tabindex="0" role="button" data-champion-row="${Number(row.championId)}" aria-label="查看${escapeHTML(name)}详情"${selected ? ' aria-current="true"' : ""}>
      <td class="champion-rank">${rank}</td>
      <td class="champion-name-cell">${artwork}<span class="champion-identity"><span class="champion-portrait"><img data-queued-src="${imageURL(source, path)}" alt="" loading="lazy" decoding="async" data-champion-image><span>${escapeHTML(name.slice(0, 1))}</span></span><span><strong>${escapeHTML(name)}</strong>${subname ? `<small>${escapeHTML(subname)}</small>` : ""}</span></span></td>
      <td>${tierBadge(row.tier, "", arena ? row.grade : "")}${mayhem && row.tierLocallyCalculated === true ? '<small class="mayhem-tier-local">本地估算</small>' : ""}</td>
      ${showPosition ? `<td><span class="position-pill">${positionIcon(row.position)}${positionLabel(row.position)}</span></td>` : ""}${mayhem ? `<td class="metric-win" data-tooltip="样本 ${escapeHTML(compactNumber(row.play))}">${percent(row.winRate)}</td>` : arena ? `<td class="metric-win${Number(row.winRate) < 49.5 ? " is-low" : ""}">${percent(row.winRate)}</td><td class="metric-placement">${number(row.averagePlacement, 2)}</td>` : rankedMetrics ? `<td class="metric-win${Number(row.winRate) < 49.5 ? " is-low" : ""}">${percent(row.winRate)}</td><td class="metric-pick">${percent(row.pickRate)}</td><td class="metric-ban">${percent(row.banRate)}</td><td class="metric-games">${Number(row.play) > 0 ? compactNumber(row.play) : "—"}</td>` : ""}
    </tr>`;
  }

  function renderAugmentCard(item) {
    const champions = objectRows(item.champions).slice(0, 5);
    const rarity = augmentRarityKey(item.rarity);
    return `<article class="augment-card is-${rarity}">
      <header>${assetImage({ source: item.imageSource, path: item.imagePath, fallbackPath: item.imageFallbackPath, name: item.name, description: item.tooltip || item.description }, "augment-icon")}<span><strong>${escapeHTML(item.name)}</strong><small class="rarity-label is-${rarity}">${rarityLabel(rarity)}</small></span></header>
      <p>${escapeHTML(item.description || item.tooltip || "暂无描述")}</p>
      <footer><span>适配英雄</span><div>${champions.map((champion) => {
        const meta = championMeta(champion.id);
        return assetImage({ source: champion.imageSource || meta?.imageSource, path: champion.imagePath || meta?.imagePath, name: champion.name || meta?.nameZh || meta?.titleZh }, "augment-champion-icon");
      }).join("") || "<small>暂无样本</small>"}</div></footer>
    </article>`;
  }

  function renderDetail() {
    const row = state.selected;
    const meta = row.meta || championMeta(row.championId);
    const title = row.name || meta?.nameZh || meta?.titleZh || `英雄 ${row.championId}`;
    const name = meta?.titleZh || meta?.nameEn || "";
    const source = row.imageSource || meta?.imageSource;
    const path = row.imagePath || meta?.imagePath;
    const detail = state.detail;
    const detailPositions = Array.isArray(detail?.positions) ? detail.positions : [];
    const rowPositions = Array.isArray(row.positions) ? row.positions : [];
    // Keep the position switcher visible while a detail response is loading or
    // when the response omits positions. The ranking row already contains the
    // same lane choices and is a better fallback than silently dropping them.
    const positions = detailPositions.length ? detailPositions : rowPositions;
    const hasPositions = state.mode === "ranked" && positions.length > 1;
    const activePosition = detail?.position || state.detailPosition || row.position || firstPositionOf(row);
    const positionStats = detailPositions.find((item) => item.position === activePosition) || null;
    const tier = positionStats?.tier ?? row.tier;
    const winRate = positionStats?.winRate ?? row.winRate;
    const pickRate = positionStats?.pickRate ?? row.pickRate;
    const banRate = positionStats?.banRate ?? row.banRate;
    const positionsMarkup = hasPositions ? `<div class="champion-detail-positions" data-count="${positions.length}" role="group" aria-label="${escapeHTML(title)}可用分路">${positions.map((item) => `<button type="button" class="${item.position === activePosition ? "is-active" : ""}" aria-pressed="${item.position === activePosition}" data-detail-position="${escapeHTML(item.position)}">${positionIcon(item.position)}<span><strong>${escapeHTML(positionLabel(item.position))}</strong><small><span class="metric-win">${percent(item.winRate)}</span><span aria-hidden="true">·</span><span class="metric-pick">占${percent(item.roleRate)}</span></small></span></button>`).join("")}</div>` : "";
    const detailBody = state.loading ? `<div class="champion-detail-content">${renderDetailSkeleton()}</div>` : state.error ? renderError(state.error, true) : detail ? renderDetailContent(detail) : renderError("详情暂时不可用", true);
    const modeLabel = state.mode === "ranked" ? "梯度榜" : state.mode === "arena" ? "斗魂竞技场" : "海克斯大乱斗";
    const arenaStats = detail?.arenaStats || {};
	const detailTierSelect = state.mode === "ranked" ? `<label class="champion-tier-select select-wrap champion-detail-tier-select"><span>段位</span><select data-champion-tier aria-label="切换英雄详情段位">${renderTierOptions()}</select></label>` : "";
    root.innerHTML = `<div class="champion-detail-toolbar"><button class="champion-back" type="button" data-champion-back><span aria-hidden="true">←</span> 返回${modeLabel}</button>${detailTierSelect}</div>
      <header class="champion-detail-hero${hasPositions ? " has-positions" : ""}">
        <img class="champion-detail-art" data-queued-src="${heroArtworkURL(meta, source, path)}" alt="" aria-hidden="true" decoding="async" data-champion-image>
        <div class="champion-detail-art-shade" aria-hidden="true"></div>
        <span class="champion-detail-portrait"><img data-queued-src="${imageURL(source, path)}" alt="${escapeHTML(title)}" decoding="async" data-champion-image><span>${escapeHTML(title.slice(0, 1))}</span></span>
        <div class="champion-detail-title"><p>${state.mode === "ranked" ? `韩服 · ${tierLabel(state.tier)} · ${positionLabel(activePosition)}` : state.mode === "arena" ? "斗魂竞技场" : "海克斯大乱斗"}</p><h2>${escapeHTML(title)}</h2><span>${escapeHTML(name)}${detail?.patch ? ` · 版本 ${escapeHTML(detail.patch)}` : ""}</span></div>
        <div class="champion-detail-side"><div class="champion-detail-metrics${state.mode === "aram-mayhem" ? " is-compact" : state.mode === "arena" ? " is-arena" : ""}">${state.mode === "ranked" ? metric("梯度", tierBadge(tier, "is-metric")) + metric("胜率", percent(winRate)) + metric("选用率", percent(pickRate)) + metric("禁用率", percent(banRate)) : state.mode === "arena" ? metric("平均名次", number(arenaStats.averagePlacement, 2)) + metric("第一名", percent(arenaStats.firstPlaceRate)) + metric("胜率", percent(arenaStats.winRate || row.winRate)) + metric("选用率", percent(arenaStats.pickRate || row.pickRate)) + metric("禁用率", percent(arenaStats.banRate)) : metric("排名", `#${row.rank || "—"}`) + metric("梯度", tierBadge(row.tier, "is-metric"))}</div>${positionsMarkup}</div>
      </header>
      ${detailBody}`;
  }

  function renderDetailContent(detail) {
    const sections = [];
    if (state.mode === "aram-mayhem" && detail.recommendedAugments?.length) sections.push(renderRecommendedAugments(detail.recommendedAugments));
    if (state.mode === "arena" && detail.teamCompositions?.length) sections.push(renderArenaDetailTeams(detail.teamCompositions.slice(0, 3)));
    if (state.mode === "arena" && detail.arenaAugments?.length) sections.push(renderArenaAugments(detail.arenaAugments));
    const rankedRuneLayout = state.mode === "ranked" && detail.runes?.length;
    if (rankedRuneLayout) {
      sections.push(renderRuneWorkspace(detail.runes, detail.build || {}));
      sections.push(renderRankedBuild(detail.build || {}));
    } else {
      sections.push(renderBuildBoard(detail.build || {}));
    }
	if (state.mode === "ranked") sections.push(renderCounters(detail.counters, detail.topPlayers, detail.countersTier || detail.sampleTier));
    return `<div class="champion-detail-content">${sections.filter(Boolean).join("")}</div>`;
  }

  function renderRecommendedAugments(items, citation) {
    // R116-B P0-6：阶段筛选。stage=0 是英雄级汇总（默认视图，展示 augments[]
    // 自己的 winRate），1..4 用上游 stages[] 的对应行替换数值。缺这个阶段的
    // augment 在该阶段视图下整条不渲染——绝不拿 0 顶上（0 会显示成「胜率 0%」，
    // 是显示不存在的数据）。斗魂路径没有 stages，stage 恒为 0。
    //
    // 评审整改 A1：还要加一道兜底钳制。记住的阶段号在新英雄上一条阶段行都匹配不
    // 到时，过滤结果是空的 → 卡片区变成「0 个 + 这个阶段没有可展示的海克斯样本」，
    // 而 renderMayhemStageChips 因为 available.length <= 1 整块不渲染 → 用户没有
    // 任何控件可以点回「汇总」，只能换模式或重载，视图被锁死在无出口的空态里。
    // 最现实的触发路径不需要上游缺阶段：主数据源熔断时后端仍然会拿 RSC 那份
    // augments 兜底，而那些行一律没有 stages。所以这里退回英雄级汇总渲染
    // （评审 6.1：取不到就整块隐藏，不留空态），并把状态写回，让 chips 的
    // aria-pressed 与实际渲染的视图一致，不留一个「没有选中项」的工具条。
    const wanted = state.mode === "aram-mayhem" ? normalizeMayhemStage(state.mayhemStage) : 0;
    const stage = wanted && !items.some((item) => mayhemAugmentStageRow(item, wanted)) ? 0 : wanted;
    if (stage !== wanted) state.mayhemStage = stage;
    const rows = stage === 0 ? items : items.filter((item) => mayhemAugmentStageRow(item, stage));
    const counts = new Map();
    const visible = rows.filter(item => {
      const rarity = augmentMetaForAsset(item.assets?.[0])?.rarity || augmentRarityKey(item.rarity || item.assets?.[0]?.rarity);
      const count = counts.get(rarity) || 0;
      counts.set(rarity, count + 1);
      return count < 3;
    });
    const scores = visible.map((item) => item.score);
    const entries = visible.map((item) => {
      const catalogMeta = augmentMetaForAsset(item.assets?.[0]);
      const asset = item.assets?.[0] || {};
      const meta = catalogMeta || { ...asset, rarity: augmentRarityKey(item.rarity), imageSource: asset.source, imagePath: asset.path };
      return { item, meta, grade: augmentGrade(item.grade, item.score, scores) };
    });
    // chips 用的是未过滤的 items：只要上游给过任何一个阶段，工具条就在，用户随时
    // 能点回「汇总」（评审整改 A1 之前，一条都不剩时工具条会整块消失）。
    const chips = renderMayhemStageChips(items);
    // 兜底钳制之后，阶段视图已经不可能一条都不剩，所以这句阶段文案只在理论上出现；
    // 汇总视图（含斗魂路径，它根本没有阶段维度）另给一句不带「阶段」字样的文案，
    // 免得对着一片空说「这个阶段」。
    const body = entries.length
      ? `<div class="arena-option-grid mayhem-recommend-grid">${entries.map(renderMayhemRecommendedAugment).join("")}</div>`
      : `<p class="mayhem-inline-empty">${stage ? "这个阶段没有可展示的海克斯样本。" : "暂无可展示的海克斯推荐样本。"}</p>`;
    return `<section class="recommendation-section mayhem-augment-ranking"><header><div><h3><span class="arena-section-icon" aria-hidden="true">✦</span>海克斯推荐</h3><p>按综合评分、胜率与样本展示各品质前三项</p></div>${chips}</header>${body}</section>`;
  }

  // 当前生效的阶段号。斗魂/单双排路径恒为 0（英雄级汇总），它们的行里没有
  // stages，套一个阶段号只会把整片推荐过滤空。
  function mayhemActiveStage() {
    return state.mode === "aram-mayhem" ? normalizeMayhemStage(state.mayhemStage) : 0;
  }

  // 取某条 augment 在指定阶段的行。stage=0（汇总）或该阶段缺失时返回 null，
  // 调用方据此决定「用汇总值」还是「整条不渲染」，两种情况都不会出现 0 值。
  function mayhemAugmentStageRow(item, stage) {
    const wanted = normalizeMayhemStage(stage);
    if (!wanted) return null;
    return objectRows(item?.stages).find((row) => Number(row?.stage) === wanted) || null;
  }

  // 阶段视图下把父行的展示字段换成该阶段的值。汇总视图原样返回父行。
  //
  // 评审整改 A3：不能无条件展开。后端 championMetricStageRow 的展示字段全带
  // omitempty，Go 的零值不进 JSON，于是「这个阶段没有这个值」在前端表现为「键不
  // 存在」；`{ ...item, ...staged }` 会静默保留父行（英雄级汇总）的值，却把它显示
  // 在「阶段 N」的 chip 下——口径混用且不披露（最容易触发的是 deltaWinRate：阶段
  // 行缺这个键，卡片就会在阶段 3 下渲染出英雄级的「较基准 +11.2%」）。所以阶段
  // 维度拥有的那几个字段一律以阶段行为准：键不在就置 undefined，让
  // mayhemDeltaLabel / percent / mayhemSampleTierLabel 各自走隐藏分支（评审 6.1）。
  // score / rarity / assets / description 这些阶段维度根本没有的字段继续沿用父行
  // （「综合评分」的口径由 renderMayhemRecommendedAugment 显式标注成英雄级）。
  //
  // 键名列表写在函数体内而不是模块级 const：本函数被 champions.test.cjs 与
  // r116b.test.cjs 的 compileFunctions 单独编译（依赖列表是固定的），模块级 const
  // 会变成自由变量抛 ReferenceError（与 normalizeMayhemDetailTab 的 TDZ 教训同源）。
  function mayhemAugmentStageItem(item, stage) {
    const staged = mayhemAugmentStageRow(item, stage);
    if (!staged) return item;
    const merged = { ...item, ...staged };
    const stageOwnedKeys = ["winRate", "pickRate", "deltaWinRate", "wilsonLowerWinRate", "stageBaselineWinRate", "games", "hexLabel", "grade", "sampleTier"];
    for (const key of stageOwnedKeys) if (!Object.hasOwn(staged, key)) merged[key] = undefined;
    return merged;
  }

  // P0-6 的阶段 chips：交互形态复用 .arena-chips（renderArenaSortBar 那一套）。
  // 上游没有任何阶段数据时整条不渲染——不给用户一个点了没反应的控件。
  function renderMayhemStageChips(items) {
    if (state.mode !== "aram-mayhem") return "";
    const rows = objectRows(items);
    if (!rows.length) return "";
    const available = MAYHEM_STAGE_OPTIONS.filter(([stage]) => stage === 0 || rows.some((row) => mayhemAugmentStageRow(row, stage)));
    if (available.length <= 1) return "";
    const active = normalizeMayhemStage(state.mayhemStage);
    return `<div class="arena-chips mayhem-stage-chips" role="group" aria-label="海克斯阶段筛选">${available.map(([stage, label]) => `<button type="button" class="${active === stage ? "is-active" : ""}" aria-pressed="${active === stage}" data-mayhem-stage="${stage}">${label}</button>`).join("")}</div>`;
  }

  // 切阶段是纯本地重渲染：四个阶段的数值已随详情页一次下发，这里只替换海克斯
  // 推荐那一段，不发任何请求（工单 P0-6 判据 2）。
  function switchMayhemStage(value) {
    const stage = normalizeMayhemStage(value);
    if (stage === state.mayhemStage) return;
    state.mayhemStage = stage;
    if (!updateMayhemAugments()) render();
  }

  function updateMayhemAugments() {
    const host = root.querySelector("[data-mayhem-augments]");
    const detail = state.detail;
    if (!host || !detail) return false;
    host.innerHTML = renderRecommendedAugments(detail.recommendedAugments || [], detail.citation);
    prepareImages(host);
    applyRenderedMetricStyles();
    return true;
  }

  function renderMayhemRecommendedAugment(entry, index) {
    const { meta } = entry;
    // 阶段视图下用该阶段的数值替换英雄级汇总（缺阶段的行已在上一步过滤掉）。
    const stage = mayhemActiveStage();
    const item = mayhemAugmentStageItem(entry.item, stage);
    const base = item.assets?.[0] || item;
    const asset = {
      kind: "augment",
      id: Number(base.id || meta?.id) || 0,
      source: meta?.imageSource || meta?.source || base.source,
      path: meta?.imagePath || meta?.path || base.path,
      fallbackPath: meta?.imageFallbackPath || meta?.fallbackPath || base.fallbackPath,
      name: meta?.name || base.name || "推荐海克斯",
      description: base.description || meta?.description || meta?.tooltip || "",
    };
    // 评审整改 B4：一张卡上不许混两种口径。阶段视图里徽章的字母必须来自阶段行自己
    // 的官方档位（后端用同一个官方档位映射算好直出，前端不复制 hexTier/hexLabel →
    // 字母的对照表），否则同一张卡会出现「徽章 S（父行 hang）」配「官方档位 顶级
    // （阶段 top）」这种自相矛盾——实测英雄 157 的 499 条阶段行里有 115 条 hexTier
    // 与父行不同。阶段行没有官方档位时（上游给 insufficient，实测 4/499）字母徽章
    // 整块隐藏：拿英雄级的 S 去配阶段级的「样本过少」正是评审点名的口径混用
    // （评审 6.1：取不到就整块隐藏，不拿别的口径顶）。
    const stageGrade = stage ? String(item.grade || "").trim().toUpperCase() : "";
    const row = {
      assets: [asset],
      rarity: meta?.rarity || item.rarity,
      grade: stage ? stageGrade : entry.grade,
      score: item.score,
      winRate: item.winRate,
      games: item.games,
    };
    return renderArenaOptionCard(row, "augment", index, [], {
      className: "is-mayhem",
      hideGradeBadge: Boolean(stage) && !stageGrade,
      metrics: [
        ["胜率", percent(item.winRate), "is-win"],
        // 工单 P0-6「实现要求」第 1 条：点击阶段 chip 后要重新渲染该阶段的
        // winRate / deltaWinRate / pickRate 三个值。选取率只在阶段视图出现——汇总
        // 视图的布局保持原样（P0-6 第 2 条只要求汇总展示英雄级 winRate，多选一格
        // 属超范围视觉改动）。阶段行没带 pickRate 时整格不渲染，绝不留 0.0% 或
        // 「—」占位（评审 6.1：取不到就整块隐藏）；父行的英雄级 pickRate 不参与，
        // 因为 mayhemAugmentStageItem 只覆盖阶段行真带着的键（评审整改 A3）。
        ...(stage && Number(item.pickRate) > 0 ? [["选取率", percent(item.pickRate), ""]] : []),
        ["样本", compactNumber(item.games), ""],
        // 综合评分是英雄级的 hexScore：后端刻意没下发阶段级 hexScore（体积取舍，
        // 见台账第 5 节），所以阶段视图里必须写明口径，不能让一个英雄级数字冒充
        // 阶段数值（评审整改 B4 的第二半）。
        [stage ? "综合评分（英雄级）" : "综合评分", number(item.score, 1), "is-score"],
        ...mayhemAugmentConfidenceMetrics(item),
      ],
    });
  }

  // P0-1 / P0-2 / P0-5 / B6 的追加指标格，每一格都「有数据才出现」：字段缺失时
  // 卡片保持原来的三格，不会冒出 undefined / NaN% / 0.0%（评审 6.1）。
  function mayhemAugmentConfidenceMetrics(item) {
    const metrics = [];
    const delta = mayhemDeltaLabel(item?.deltaWinRate);
    if (delta) metrics.push(["较基准", delta, `mayhem-delta ${mayhemDeltaTone(item.deltaWinRate)}`]);
    // 评审整改 B6：「较基准」的基准必须可见，否则它是个没有定义的词（相对英雄？
    // 相对阶段？相对全英雄？）。阶段行带 stageBaselineWinRate，实测口径精确成立
    // （deltaWinRate = 阶段胜率 − 阶段基准胜率，英雄 157 的 499/499 条零误差），
    // 所以在阶段视图里把基准值单列一格，用户能自己验算 61.66% − 57.05% = +4.6%。
    // 卡片外壳是 overflow:hidden，逐格 data-tooltip 会被裁掉（P0-2 的 Wilson 下界
    // 因此也是单独一格而不是 tooltip），所以这里用可见的一格而不是悬浮提示。
    // 英雄级汇总行没有这个字段（上游只在 stages[] 里给），拿不到就整格不渲染。
    const baseline = Number(item?.stageBaselineWinRate);
    if (delta && Number.isFinite(baseline) && baseline > 0) metrics.push(["阶段基准", percent(baseline), "is-baseline"]);
    // P0-5：官方档位文案用 hexLabel（中文「夯」「顶级」），绝不用 hexTier
    // （那是内部枚举名 "hang"/"top"，给用户看等于泄露实现细节）。
    const label = String(item?.hexLabel || "").trim();
    if (label) metrics.push(["官方档位", escapeHTML(label), "is-hex-label"]);
    const lowSample = mayhemSampleTierLabel(item);
    if (lowSample) metrics.push(["置信", lowSample, "is-low-confidence"]);
    else {
      const wilson = mayhemWilsonLabel(item);
      if (wilson) metrics.push(["95%下界", wilson, "is-wilson"]);
    }
    return metrics;
  }


  // ---- R116-B P1-6：赛后表现指标面板（「表现」tab）----
  // 数值、「较全英雄平均」与量纲处置全部在后端算好，前端只负责分组与展示。
  function renderMayhemPerformancePanel(panel) {
    const metrics = objectRows(panel?.metrics).filter((metric) => mayhemPerformanceValue(metric) !== "");
    if (!metrics.length) return "";
    const groups = mayhemPerformanceGroups(panel, metrics);
    if (!groups.length) return "";
    const heroCount = Number(panel?.heroCount);
    const scope = Number.isFinite(heroCount) && heroCount > 0 ? `<span class="section-count">对比 ${compactNumber(heroCount)} 位英雄</span>` : "";
    const cumulativeNote = metrics.some((metric) => metric?.cumulative === true)
      ? '<p class="mayhem-performance-note">双杀/三杀/四杀/五杀为累计次数，受出场场次影响，不可跨英雄直接比较。</p>'
      : "";
    return `<section class="recommendation-section mayhem-performance"><header><div><h3><span class="arena-section-icon" aria-hidden="true">◔</span>表现指标</h3></div>${scope}</header><div class="mayhem-performance-groups">${groups.map((group) => mayhemPerformanceGroup(group, metrics)).join("")}</div>${cumulativeNote}</section>`;
  }

  // Groups 决定分组与顺序；某一组指标全缺时后端整组不下发，前端也不留空态。
  function mayhemPerformanceGroups(panel, metrics) {
    const declared = (Array.isArray(panel?.groups) ? panel.groups : []).map((name) => String(name || "").trim()).filter(Boolean);
    const extra = [];
    for (const metric of metrics) {
      const name = String(metric?.group || "").trim();
      if (name && !declared.includes(name) && !extra.includes(name)) extra.push(name);
    }
    return [...declared, ...extra].filter((name) => metrics.some((metric) => String(metric?.group || "").trim() === name));
  }

  function mayhemPerformanceGroup(group, metrics) {
    const rows = metrics.filter((metric) => String(metric?.group || "").trim() === group);
    const cells = rows.filter((metric) => metric?.cumulative !== true).map((metric) => {
      const delta = mayhemPerformanceDelta(metric);
      return `<div><dt>${escapeHTML(metric?.label)}</dt><dd><b>${mayhemPerformanceValue(metric)}</b>${delta ? `<span class="mayhem-delta ${mayhemPerformanceDeltaTone(metric)}">较平均 ${delta}</span>` : ""}</dd></div>`;
    });
    const cumulative = rows.filter((metric) => metric?.cumulative === true);
    if (cumulative.length) {
      const values = cumulative.map((metric) => {
        const label = String(metric?.label || "").replace(/累计次数$/, "").trim() || "多杀";
        return `<span><small>${escapeHTML(label)}</small><b>${mayhemPerformanceValue(metric)}</b></span>`;
      }).join("");
      cells.push(`<div class="is-cumulative"><dt>多杀累计</dt><dd class="mayhem-multikill-values">${values}</dd></div>`);
    }
    if (!cells.length) return "";
    return `<section class="mayhem-performance-group"><h4>${escapeHTML(group)}</h4><dl>${cells.join("")}</dl></section>`;
  }

  // format 的取值只有后端 hexdataPerformanceFields 里那五种（decimal2 /
  // decimal3 / int / count / percent）。percent 的 value 是上游 0..1 原值
  // （实测 killParticipation 0.76618、damageShare 0.19214），要 ×100 再格式化；
  // 其余三种已经是最终量纲。
  function mayhemPerformanceValue(metric) {
    const value = Number(metric?.value);
    if (!Number.isFinite(value)) return "";
    switch (String(metric?.format || "")) {
      case "percent": return percent(value * 100);
      case "int":
      case "count": return number(value, 0);
      case "decimal2": return number(value, 2);
      case "decimal3": return number(value, 3);
      default: return "";
    }
  }

  // hasDelta=false 时 DeltaPercent=0 有「真的等于平均」和「无法计算」两种含义，
  // 所以只看 hasDelta、不看数值（后端注释里的硬要求）。累计项永不带 ±%。
  // 评审整改 A2：0 的判据必须放在 toFixed(1) 之后。|deltaPercent| < 0.05 会被
  // 一位小数抹成 "0.0"，先判数值再格式化就会渲染出「较平均 +0.0%」——那正是
  // 工单 P1-6 判据（与 P0-1 同一条红线）禁止的没有信息量的标签。
  function mayhemPerformanceDelta(metric) {
    if (metric?.cumulative === true || metric?.hasDelta !== true) return "";
    const value = Number(metric?.deltaPercent);
    if (!Number.isFinite(value)) return "";
    const text = Math.abs(value).toFixed(1);
    return text === "0.0" ? "" : `${value > 0 ? "+" : "-"}${text}%`;
  }

  function mayhemPerformanceDeltaTone(metric) {
    return Number(metric?.deltaPercent) > 0 ? "is-delta-up" : "is-delta-down";
  }

  function augmentMetaForAsset(asset) {
    const rows = objectRows(state.augments?.rows);
    const id = Number(asset?.id);
    const path = String(asset?.path || "");
    const name = normalizeSearch(asset?.name);
    const match = rows.find((item) => id > 0 && Number(item.id) === id)
      || rows.find((item) => path && String(item.imagePath || "") === path)
      || rows.find((item) => name && normalizeSearch(item.name) === name)
      || null;
    return match ? { ...match, rarity: augmentRarityKey(match.rarity) } : null;
  }

  function renderArenaDetailTeams(teams) {
    return `<section class="recommendation-section arena-detail-teams"><header><div><h3>推荐三人队伍</h3><p>当前英雄与两名搭档组成完整队伍</p></div><span class="section-count">${teams.length} 组</span></header><div class="arena-detail-team-grid">${teams.map((team, index) => `<article><b>${index + 1}</b>${renderArenaTeamFaces(team.champions, true)}<strong>${teamName(team)}</strong><dl><div><dt>平均名次</dt><dd>${number(team.averagePlacement, 2)}</dd></div><div><dt>第一名</dt><dd>${percent(team.firstPlaceRate)}</dd></div><div><dt>选用率</dt><dd class="metric-pick">${percent(team.pickRate)}</dd></div><div><dt>胜率</dt><dd class="metric-win">${percent(team.winRate)}</dd></div></dl></article>`).join("")}</div></section>`;
  }

  function renderArenaAugments(rows) {
    return renderRecommendedAugments(rows);
  }

  function activeRunePage(pages) {
    return Math.min(Math.max(0, Number(state.runePage) || 0), Math.max(0, pages.length - 1));
  }

  // 设计稿布局：左侧符文与技能加点，右侧按召唤师技能、出门装、鞋子排序。
  function renderRuneWorkspace(pages, build) {
    const active = activeRunePage(pages);
    const tabs = pages.map((page, index) => {
      const keys = `${runeStyleIcon(page.primaryStyle, "rune-tab-style")}${runeStyleIcon(page.subStyle, "rune-tab-substyle")}`;
      return `<button type="button" class="rune-page-tab${index === active ? " is-active" : ""}" data-rune-page="${index}" role="tab" aria-selected="${index === active}" aria-label="第 ${index + 1} 套符文方案">
        <span class="rune-tab-keys">${keys}</span>
        <span class="rune-tab-stats"><b class="metric-win">${percent(page.winRate)}</b><small>${compactNumber(page.games)} 场 · 选用 ${percent(page.pickRate)}</small></span>
      </button>`;
    }).join("");
    const boards = pages.length ? `<div class="rune-board-panel" data-rune-page-panel="${active}">${renderRuneTree(pages[active])}</div>` : "";
    const allocated = allocateLoadoutSideRows(build);
    const metricSideCard = (title, rows) => {
      const hasMetric = (value) => value !== null && value !== undefined && String(value).trim() !== "" && Number.isFinite(Number(value));
      const options = (rows || []).map((row) => {
        const metrics = [["选用率", "metric-pick", row.pickRate], ["胜率", "metric-win", row.winRate]]
          .filter(([, , value]) => hasMetric(value))
          .map(([label, className, value]) => `<div><dt>${label}</dt><dd class="${className}">${percent(value)}</dd></div>`).join("");
        const stats = metrics ? `<dl class="side-stats">${metrics}</dl>` : "";
        return `<div class="build-side-row"><div class="config-icons">${(row.assets || []).map((asset) => renderAssetButton(asset)).join("")}</div>${stats}</div>`;
      }).join("");
      return `<section class="recommendation-section side-card"><header><h3>${title}</h3></header><div class="side-build-options">${options || '<p class="muted side-empty">暂无样本</p>'}</div></section>`;
    };
    return `<div class="champion-loadout-row">
      <div class="loadout-main"><section class="recommendation-section rune-workspace"><header><h3>推荐符文</h3><span class="section-count">${pages.length} 套</span></header><div class="rune-page-tabs" role="tablist" aria-label="符文方案">${tabs}</div>${boards}</section>${renderSkillsCard(build)}</div>
      <div class="loadout-side">${renderSpellsCard({ ...build, summonerSpells: allocated.summonerSpells })}${metricSideCard("出门装", allocated.starterItems)}${metricSideCard("鞋子", allocated.boots)}</div>
    </div>`;
  }

  function allocateLoadoutSideRows(build) {
    const available = {
      summonerSpells: (build?.summonerSpells || []).slice(0, 3),
      starterItems: (build?.starterItems || []).slice(0, 3),
      boots: (build?.boots || []).slice(0, 3),
    };
    const allocated = {
      summonerSpells: available.summonerSpells.slice(0, 2),
      starterItems: available.starterItems.slice(0, 2),
      boots: available.boots.slice(0, 2),
    };
    // Keep the right rail at six rows when one category has fewer than two
    // real recommendations. A third real row may fill the vacant slot.
    const keys = ["summonerSpells", "starterItems", "boots"];
    for (const key of keys) {
      if (keys.reduce((total, candidate) => total + allocated[candidate].length, 0) >= 6) break;
      if (available[key].length > allocated[key].length) allocated[key].push(available[key][allocated[key].length]);
    }
    return allocated;
  }

  function renderSpellsCard(build) {
    const rows = (build.summonerSpells || []).slice(0, 2);
    const options = rows.map((row, index) => {
      const assets = (row.assets || []).slice(0, 2);
      const hasMetric = (value) => value !== null && value !== undefined && String(value).trim() !== "" && Number.isFinite(Number(value));
      const metrics = [
        ["场次", "", row.games, compactNumber],
        ["选用率", "metric-pick", row.pickRate, percent],
        ["胜率", "metric-win", row.winRate, percent],
      ].filter(([, , value]) => hasMetric(value)).map(([label, className, value, formatter]) => `<div><dt>${label}</dt><dd${className ? ` class="${className}"` : ""}>${formatter(value)}</dd></div>`).join("");
      const stats = metrics ? `<dl class="spell-option-stats">${metrics}</dl>` : "";
      return `<div class="spell-option${index === 0 ? " is-primary" : ""}">
        <span class="spell-option-icons">${assets.map((asset) => assetImage(asset, "game-icon spell-option-icon", true)).join("")}</span>
        ${stats}
      </div>`;
    }).join("");
    return `<section class="recommendation-section side-card"><header><h3>召唤师技能</h3></header><div class="spell-options${rows.length === 1 ? " is-single" : ""}">${options || '<p class="muted side-empty">该模式暂无独立召唤师技能样本</p>'}</div></section>`;
  }

  function renderSkillsCard(build) {
    const skills = (build.skills || [])[0];
    const hasMetric = (value) => value !== null && value !== undefined && String(value).trim() !== "" && Number.isFinite(Number(value));
    const metrics = [["选用率", "metric-pick", skills?.pickRate], ["胜率", "metric-win", skills?.winRate]]
      .filter(([, , value]) => hasMetric(value))
      .map(([label, className, value]) => `<span>${label} <b class="${className}">${percent(value)}</b></span>`).join("");
    const stats = metrics ? `<span class="skill-head-stats">${metrics}</span>` : "";
    return `<section class="recommendation-section side-card"><header><h3>技能加点</h3>${stats}</header><div class="skill-plan side-skill-plan">${renderChampionSkillPlan(skills, false)}</div></section>`;
  }

  function renderRuneTree(page) {
    return `<div class="champion-rune-board">${renderRuneColumn(page.primaryStyle, page.primarySlots, "primary")}${renderRuneColumn(page.subStyle, page.subSlots, "secondary")}${renderRuneColumn({ name: "属性碎片" }, page.shardSlots, "shards")}</div>`;
  }

  function renderRuneColumn(style, slots, kind) {
    const rows = slots || [];
    // 副系与属性碎片首行与主系的第二行（小符文首行）水平对齐。
    const spacer = kind !== "primary" ? '<div class="champion-rune-row is-spacer" aria-hidden="true"></div>' : "";
    return `<section class="champion-rune-column is-${kind}" aria-label="${escapeHTML(style?.name || "属性碎片")}">${spacer}${rows.map((row) => `<div class="champion-rune-row">${row.map(renderRuneOption).join("")}</div>`).join("")}</section>`;
  }

  function renderRuneOption(item) {
    const name = item?.name || "符文";
    const explanation = item?.description || name;
    const tooltip = explanation !== name ? `${name}\n${explanation}` : name;
    return `<button class="rune-option-button${item?.active ? " is-selected" : ""}" type="button" aria-label="${escapeHTML(`${name}：${explanation}`)}" data-tooltip="${escapeHTML(tooltip)}">${assetImage(item, "game-icon rune-icon", false)}</button>`;
  }

  function renderBuildBoard(build) {
    const spells = (build.summonerSpells || []).slice(0, 2);
    const skills = (build.skills || [])[0];
    const starters = (build.starterItems || []).slice(0, 3);
    const boots = (build.boots || []).slice(0, 2);
    const prismItems = (build.prismItems || []).slice(0, 5);
    const routes = buildItemRoutes(build);
    const depthGroups = renderBuildDepthGroups(build);
    if (!spells.length && !skills && !routes.length && !prismItems.length && !depthGroups) return "";
    const spellContent = spells.length ? spells.map((row) => renderConfigOption(row, "spell")).join("") : '<p class="muted">该模式暂无独立召唤师技能样本</p>';
    const itemSections = [];
    if (state.mode !== "arena") itemSections.push(`<section><h3>出门装</h3><div class="config-option-list">${starters.map((row) => renderConfigOption(row, "item")).join("")}</div></section>`);
    itemSections.push(`<section><h3>鞋子</h3><div class="config-option-list">${boots.map((row) => renderConfigOption(row, "item")).join("")}</div></section>`);
    if (state.mode === "arena" && prismItems.length) itemSections.push(`<section><h3>棱彩装备</h3><div class="config-option-list">${prismItems.map((row) => renderConfigOption(row, "item")).join("")}</div></section>`);
    itemSections.push(`<section class="route-options"><h3>出装路线</h3><div class="config-option-list">${routes.map((row) => renderConfigOption(row, "route", null, renderDepthStats)).join("")}</div>${depthGroups}</section>`);
    return `<section class="build-recommendation champion-build-board"><div class="build-essentials"><section><h3>召唤师技能</h3><div class="config-option-list">${spellContent}</div></section><section class="skill-plan"><h3>技能加点</h3>${renderChampionSkillPlan(skills)}</section></div><div class="item-option-groups">${itemSections.join("")}</div></section>`;
  }

  // 设计稿布局：条件核心链与第四/第五件在同一横向列网格中展示。
  function renderRankedBuild(build) {
    const routes = buildItemRoutes(build);
    const depthGroups = renderBuildDepthGroups(build);
    // 列数直接数实际渲染出来的列，避免网格列数与真实列数漂移。
    const depthCount = (depthGroups.match(/class="build-depth-column"/g) || []).length;
    if (!routes.length && !depthCount) return "";
    const routeRows = routes.map((row) => renderConfigOption(row, "route", null, renderDepthStats)).join("");
    return `<section class="recommendation-section champion-build-board build-workspace"><header><h3>出装</h3><span class="build-primary-source">OP.GG · 当前版本</span></header><div class="build-split"><div class="build-routes"><div class="build-item-row" data-depth-count="${depthCount}"><section class="build-core-column"><h4>核心装</h4><div class="config-option-list">${routeRows}</div>${routeRows ? "" : '<p class="muted">暂无出装路线样本</p>'}</section>${depthGroups}</div></div></div></section>`;
  }

  function renderChampionSkillPlan(row, showStats = true) {
    if (!row) return '<p class="muted">暂无技能加点样本</p>';
    const priority = row.skillPriority || [];
    const icons = row.assets || [];
    let priorityHTML = priority.map((key, index) => {
      const asset = icons[index];
      const tooltip = championAbilityTooltip(key, asset);
      return `<button class="skill-icon-button" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${asset ? assetImage(asset, "game-icon recommend-icon", false) : `<span class="skill-letter">${escapeHTML(key)}</span>`}<span>${escapeHTML(key)}</span></button>`;
    }).join("");
    if (row.ultimate) {
      const tooltip = championAbilityTooltip("R", row.ultimate);
      priorityHTML += `<button class="skill-icon-button is-ultimate" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${assetImage(row.ultimate, "game-icon recommend-icon", false)}<span>R</span></button>`;
    }
    const mainSkills = [...new Set(priority.map(key => String(key).toUpperCase()).filter(key => ["Q", "W", "E"].includes(key)))];
    if (mainSkills.length >= 2) priorityHTML += `<strong class="skill-priority-summary">主${mainSkills[0]}副${mainSkills[1]}</strong>`;
    const order = row.skillOrder || [];
    const skillWinRate = !showStats ? `<span class="skill-win-rate"><small>胜率</small><b>${percent(row.winRate)}</b></span>` : "";
    return `<div class="skill-plan-summary"><div class="skill-priority">${priorityHTML}</div>${skillWinRate}</div><div class="skill-order" data-skill-count="${Math.max(1, order.length)}" aria-label="技能升级顺序">${order.map((key, index) => `<span class="is-${String(key).toLowerCase()}"><b>${escapeHTML(key)}</b><small>${index + 1}</small></span>`).join("")}</div>${showStats ? renderOptionStats(row) : ""}`;
  }

  function championAbilityTooltip(key, ability) {
    const lines = [`${key} · ${ability?.name || `${key} 技能`}`];
    const description = String(ability?.description || "").trim();
    if (description && description !== key) lines.push(description);
    appendAssetNumbers(lines, ability);
    return lines.join("\n");
  }

  function appendAssetNumbers(lines, asset) {
    for (const [label, values, suffix, showFree] of [[asset?.costType || "消耗", asset?.costs, "", true], ["冷却", asset?.cooldowns, " 秒", false], ["施法距离", asset?.ranges, "", false]]) {
      const normalized = (values || []).map(Number).filter(Number.isFinite).slice(0, 6);
      if (!normalized.length) continue;
      const positive = normalized.filter((value) => value > 0);
      if (!positive.length) { if (showFree) lines.push(`${label}：无`); continue; }
      const display = positive.every((value) => value === positive[0]) ? number(positive[0]) : positive.map((value) => number(value)).join(" / ");
      lines.push(`${label}：${display}${suffix}`);
    }
  }

  function assetTooltip(asset, fallback = "装备") {
    const name = asset?.name || fallback;
    const lines = [name];
    const description = String(asset?.description || "").trim();
    if (description && description !== name) lines.push(description);
    appendAssetNumbers(lines, asset);
    return lines.join("\n");
  }

  function buildItemRoutes(build) {
    const cores = (build.coreItems || []).slice(0, CORE_RECOMMENDATION_LIMIT);
    const detailPosition = String(state.detail?.position || state.detailPosition || state.selected?.position || state.position).toLowerCase();
    const routeLimit = ["adc", "bottom"].includes(detailPosition) ? ADC_ITEM_ROUTE_LIMIT : DEFAULT_ITEM_ROUTE_LIMIT;
    return cores.map((core) => {
      const seen = new Set();
      const assets = [];
      const addAsset = (asset) => {
        if (!asset?.path || seen.has(asset.path) || assets.length >= routeLimit) return false;
        seen.add(asset.path);
        assets.push(asset);
        return true;
      };
      for (const asset of core.assets || []) addAsset(asset);
      return { ...core, assets };
    }).filter((row) => row.assets.length);
  }

  function championItemAttemptSummary(build) {
    const sourceLabels = { qq101: "QQ101", opgg: "OP.GG" };
    return (build?.itemAttempts || []).filter((attempt) => attempt?.outcome && attempt.outcome !== "success").map((attempt) => {
      const source = sourceLabels[String(attempt.source || "").toLowerCase()] || String(attempt.source || "未知来源").toUpperCase();
      const rawMessage = String(attempt.message || "").trim();
      let message = rawMessage || "暂不可用";
      if (/empty response/i.test(rawMessage)) message = "返回为空";
      else if (/timeout|deadline|加载预算/i.test(rawMessage)) message = "请求超时";
      else if (/disabled by feature gate|远程开关关闭/i.test(rawMessage)) message = "已停用";
      else if (/unsupported/i.test(rawMessage)) message = "当前段位口径不兼容";
      return `${source}：${message}`;
    }).join("；");
  }

  function renderBuildDepthGroups(build) {
    const chainStatus = String(build?.itemChainStatus || "");
    const hasChainStatus = chainStatus === "ready" || chainStatus === "unavailable";
    const attemptSummary = championItemAttemptSummary(build);
    const groups = [
      ["第四件", build?.fourthItems || [], Number(build?.fourthSample) || 0],
      ["第五件", build?.fifthItems || [], Number(build?.fifthSample) || 0],
      ["第六件", build?.sixthItems || [], Number(build?.sixthSample) || 0],
    ].map(([label, rows, sample]) => [label, rows.filter((row) => (row?.assets || []).length), sample])
      // 第六件是逐英雄/逐分路独立缺失的（辅助位常年没有），上游没有这一层
      // 就整列不展示，而不是留一列空态文案占着位置。第四/第五件仍然保留
      // 空态，按读取状态区分上游没有样本与解析/网络失败。
      .filter(([label, rows]) => (label === "第六件" ? rows.length > 0 : hasChainStatus || rows.length));
    if (!groups.length) return "";
    return `<div class="item-depth-columns champion-item-depth-columns">${groups.map(([label, rows, sample]) => {
      const empty = chainStatus === "unavailable" ? `${label}推荐暂不可用${attemptSummary ? `：${escapeHTML(attemptSummary)}` : "，稍后重试"}` : "该阶段暂无可用样本";
      return `<section class="build-depth-column"><h4><span>${label}</span></h4><div class="config-option-list">${rows.map((row) => renderConfigOption(row, "item", null, renderDepthStats)).join("")}</div>${rows.length ? "" : `<p class="muted build-depth-empty">${empty}</p>`}</section>`;
    }).join("")}</div>`;
  }

  function renderConfigOption(row, kind, showWinRate = null, statsRenderer = null) {
    const icons = (row.assets || []).map((asset, index) => {
      const normalized = kind === "spell" ? summonerSpellAsset(asset) : asset;
      return kind === "route" ? `<span class="route-step">${index ? '<span class="route-arrow" aria-hidden="true">›</span>' : ""}${renderAssetButton(normalized)}</span>` : renderAssetButton(normalized);
    }).join("");
    const stats = statsRenderer ? statsRenderer(row) : showWinRate === true
      ? `<span class="${kind === "route" ? "route-win-rate" : "option-win-rate"}"><small>胜率</small><b>${percent(row.winRate)}</b></span>`
      : showWinRate === false ? renderOptionPickStats(row) : renderOptionStats(row);
    const sampleGames = Number(row?.games ?? row?.stats?.games ?? 0);
    return `<article class="config-option" data-sample-games="${sampleGames}"><div class="config-icons" data-icon-count="${(row.assets || []).length}">${icons}</div>${stats}</article>`;
  }

  function summonerSpellAsset(asset) {
    const id = Number(asset?.id || asset?.name);
    const keys = { 1: "SummonerBoost", 3: "SummonerExhaust", 4: "SummonerFlash", 6: "SummonerHaste", 7: "SummonerHeal", 11: "SummonerSmite", 12: "SummonerTeleport", 14: "SummonerDot", 21: "SummonerBarrier", 32: "SummonerSnowball" };
    const labels = { 1: "净化", 3: "虚弱", 4: "闪现", 6: "疾跑", 7: "治疗术", 11: "惩戒", 12: "传送", 14: "引燃", 21: "屏障", 32: "标记" };
    const names = { 净化: "SummonerBoost", 虚弱: "SummonerExhaust", 闪现: "SummonerFlash", 疾跑: "SummonerHaste", 治疗术: "SummonerHeal", 惩戒: "SummonerSmite", 传送: "SummonerTeleport", 引燃: "SummonerDot", 屏障: "SummonerBarrier", 标记: "SummonerSnowball" };
    const assetName = String(asset?.name || "").trim();
    const key = keys[id] || names[assetName] || (/^Summoner[A-Za-z0-9]+$/.test(assetName) ? assetName : "");
    const invalidPath = !asset?.source || !asset?.path || /\/img\/spell\/\d+\.png(?:$|\?)/.test(String(asset.path));
    if (!key || !invalidPath) return asset;
    const patch = state.detail?.currentPatch || state.detail?.patch || state.rankings?.patch || "latest";
    return { ...asset, name: labels[id] || asset?.name || "召唤师技能", source: "ddragon", path: `/cdn/${patch}/img/spell/${key}.png` };
  }

  function renderAssetButton(asset) {
    const name = asset?.name || "装备";
    const tooltip = assetTooltip(asset, name);
    // Arena augments use the same button shell as items, but keep the
    // augment-icon class so neutral upstream glyphs receive metadata rarity color.
    const iconClass = /augment/i.test(String(asset?.kind || ""))
      ? "game-icon recommend-icon augment-icon"
      : "game-icon recommend-icon";
    return `<button class="item-option-button" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${assetImage(asset, iconClass, false)}</button>`;
  }

  function renderOptionStats(row) {
    const hasPick = Number.isFinite(Number(row?.pickRate)) && Number(row.pickRate) > 0;
    const hasWin = Number.isFinite(Number(row?.winRate)) && Number(row.winRate) > 0;
    if (!hasPick && !hasWin) return "";
    return `<dl class="option-stats"><div class="is-pick"><dt>选用率</dt><dd>${percent(row?.pickRate)}</dd></div><div class="is-win"><dt>胜率</dt><dd>${percent(row?.winRate)}</dd></div></dl>`;
  }

  function renderDepthStats(row) {
	// R60 cleanup marker: GamesUnavailable only exists for the retained QQ101
	// fallback; OP.GG RSC rows always include a parsed game count.
    const gamesUnavailable = row?.gamesUnavailable === true;
    // QQ101 validates both rates before constructing a row, but Go omits a
    // numeric zero from JSON. Under this row-level marker, an absent rate is 0.
    const winRate = gamesUnavailable && row?.winRate == null ? 0 : Number(row?.winRate);
    const pickRate = gamesUnavailable && row?.pickRate == null ? 0 : Number(row?.pickRate);
    const hasWin = Number.isFinite(winRate) && (gamesUnavailable ? winRate >= 0 : winRate > 0);
    const games = Number(row?.games);
    if (!hasWin && !gamesUnavailable && !(games > 0)) return "";
    const isExtreme = (value) => Number.isFinite(value) && (value === 0 || value === 100);
    const lowConfidence = gamesUnavailable && (isExtreme(winRate) || isExtreme(pickRate));
    if (gamesUnavailable) {
      const note = lowConfidence ? "腾讯官方数据不提供样本量；0% 或 100% 极值通常表示样本极少" : "腾讯官方数据不提供样本量";
      const label = lowConfidence ? "样本极少" : "未提供";
      return `<dl class="option-stats is-depth${lowConfidence ? " is-low-confidence" : ""}"><div class="is-win"><dt>胜率</dt><dd>${percent(winRate)}</dd></div><div class="is-games"><dt>样本量</dt><dd><span class="sample-volume-note${lowConfidence ? " is-low-confidence" : ""}" tabindex="0" aria-label="${note}" data-tooltip="${note}" data-tooltip-size="compact">${label}</span></dd></div></dl>`;
    }
    return `<dl class="option-stats is-depth"><div class="is-win"><dt>胜率</dt><dd>${percent(row?.winRate)}</dd></div><div class="is-games"><dt>场次</dt><dd>${compactNumber(games)}</dd></div></dl>`;
  }

  function renderOptionPickStats(row) {
    const hasPick = Number.isFinite(Number(row?.pickRate)) && Number(row.pickRate) > 0;
    return hasPick ? `<dl class="option-stats"><div class="is-pick"><dt>选用率</dt><dd>${percent(row?.pickRate)}</dd></div></dl>` : "";
  }

  // 英雄详情沿用正文三卡布局：优势、劣势和高场次玩家并列展示。
  function renderCounters(counters, players, countersTier) {
    const weak = counters?.weakAgainst || [];
    const strong = counters?.strongAgainst || [];
    const ranked = players || [];
    const sampleNote = countersTier ? `${tierLabel(countersTier)}样本` : "对线样本";
    return `<div class="counter-row">${renderCounterGroup("优势对抗", "面对这些英雄更占优势", strong, "strong", sampleNote)}${renderCounterGroup("劣势对抗", "这些英雄更难应对", weak, "weak", sampleNote)}${renderTopPlayers(ranked)}</div>`;
  }

  function renderCounterGroup(title, copy, rows, kind, sampleNote = "对线样本") {
    if (!rows.length) {
      return `<section class="recommendation-section counter-group is-${kind}"><header><div><h4>${title}</h4><p>${copy}</p></div><span class="section-count">${escapeHTML(sampleNote)}</span></header><p class="counter-empty">该段位暂无足够对线样本</p></section>`;
    }
    return `<section class="recommendation-section counter-group is-${kind}"><header><div><h4>${title}</h4><p>${copy}</p></div><span class="section-count">${escapeHTML(sampleNote)}</span></header><div>${rows.slice(0, 5).map((row) => {
      const width = Math.min(100, Math.max(4, Number(row.winRate) || 0));
      return `<button type="button" class="counter-champion" data-counter-champion="${escapeHTML(row.key)}"><span class="champion-portrait"><img data-queued-src="${imageURL(row.imageSource, row.imagePath)}" alt="" loading="lazy" decoding="async" data-champion-image><span>${escapeHTML((row.name || "?").slice(0, 1))}</span></span><span><strong>${escapeHTML(row.name || row.key)}</strong><small>${compactNumber(row.games)} 场</small></span><span class="counter-meter" aria-hidden="true"><i data-bar-width="${width}"></i></span><b>${percent(row.winRate)}</b><i aria-hidden="true">›</i></button>`;
    }).join("")}</div></section>`;
  }

  function playerTierLabel(tier) {
    const parts = String(tier || "").trim().split(/\s+/);
    const names = { iron: "黑铁", bronze: "青铜", silver: "白银", gold: "黄金", platinum: "铂金", emerald: "翡翠", diamond: "钻石", master: "大师", grandmaster: "宗师", challenger: "王者" };
    const base = names[parts[0]] || "";
    return base ? `${base}${parts[1] ? ` ${parts[1]}` : ""}` : "";
  }

  function renderTopPlayers(players) {
    if (!players.length) {
      return `<section class="recommendation-section counter-group is-players"><header><div><h4>场次最多的玩家</h4><p>韩服 · 钻二以上样本</p></div><span class="section-count">暂无样本</span></header><p class="counter-empty">暂时没有该英雄的高场次玩家样本</p></section>`;
    }
    const visible = players.slice(0, 5);
    const rows = visible.map((player, index) => {
      const crest = { iron: 1, bronze: 1, silver: 1, gold: 1, platinum: 1, emerald: 1, diamond: 1, master: 1, grandmaster: 1, challenger: 1 }[String(player.tier || "").split(/\s+/)[0]]
        ? `<img class="player-tier-crest" src="/rank-crests/${String(player.tier).split(/\s+/)[0]}.png" alt="" decoding="async">` : "";
      const tierText = [playerTierLabel(player.tier), player.lp ? `${player.lp} LP` : ""].filter(Boolean).join(" · ");
      return `<button type="button" class="player-row${index === 0 ? " is-top" : ""}" data-player-name="${escapeHTML(player.name)}" data-player-tag="${escapeHTML(player.tagline || "")}" aria-label="查看 ${escapeHTML(player.name)} 的战绩">
        <b class="player-rank">${Number(player.rank) || index + 1}</b>
        <span class="player-avatar">${player.iconPath ? `<img data-queued-src="${imageURL(player.iconSource, player.iconPath)}" alt="" loading="lazy" decoding="async">` : ""}</span>
        <span class="player-copy"><strong>${escapeHTML(player.name)}${player.tagline ? ` <small>#${escapeHTML(player.tagline)}</small>` : ""}</strong><small>${crest}${escapeHTML(tierText || "段位未知")}</small></span>
        <span class="player-games"><b class="metric-win">${percent(player.winRate)}</b><small>${escapeHTML(player.games || "—")} 场</small></span>
      </button>`;
    }).join("");
    return `<section class="recommendation-section counter-group is-players"><header><div><h4>场次最多的玩家</h4><p>韩服 · 钻二以上样本</p></div><span class="section-count">前 ${visible.length} 名</span></header><div>${rows}</div></section>`;
  }

  function metric(label, value) { const className = label === "胜率" ? "metric-win" : label === "选用率" ? "metric-pick" : label === "禁用率" ? "metric-ban" : ""; return `<div class="${className}"><span>${label}</span><strong>${value}</strong></div>`; }
  function augmentTierLabel(value) { return ({ 0: "S", 1: "A", 2: "B", 3: "C", 4: "D", 5: "E" })[Number(value)] || "—"; }
  function rarityLabel(value) { return ({ silver: "白银", gold: "黄金", prismatic: "棱彩" })[augmentRarityKey(value)] || "未分类"; }

  function filteredChampionRows() {
    const rows = rankingRows();
    const query = normalizeSearch(state.query);
    if (!query) return rows;
    const scored = rows.map((row, order) => {
      const meta = championMeta(row.championId);
      const cache = championIndex().terms;
      let cached = cache.get(row);
      if (!cached || cached.name !== row.name || cached.key !== row.key) {
        cached = { name: row.name, key: row.key, values: [row.name, row.key, meta?.nameZh, meta?.titleZh, meta?.nameEn, meta?.titleEn, ...(meta?.searchTerms || [])].map(normalizeSearch).filter(Boolean) };
        cache.set(row, cached);
      }
      const values = cached.values;
      const score = Math.max(0, ...values.map((value) => searchScore(query, value)));
      return { row, order, score };
    });
    const bestScore = Math.max(0, ...scored.map((item) => item.score));
    return scored.filter((item) => item.score > 0 && (bestScore === 100 ? item.score === 100 : true)).sort((a, b) => b.score - a.score || a.order - b.order).map((item) => item.row);
  }

  function searchScore(query, value) {
    if (value === query) return 100;
    if (value.startsWith(query)) return 80 - Math.min(20, value.length - query.length);
    const index = value.indexOf(query);
    if (index >= 0) return 60 - Math.min(20, index);
    return fuzzySubsequence(query, value) ? 10 : 0;
  }

  function scoreChampionSelectOption(query, value, label) {
	const normalizedQuery = normalizeSearch(query);
	if (!normalizedQuery) return 0;
	const meta = championMeta(value);
	const values = [label, value, meta?.key, meta?.slug, meta?.nameZh, meta?.titleZh, meta?.nameEn, meta?.titleEn, ...(meta?.searchTerms || [])].map(normalizeSearch).filter(Boolean);
	return Math.max(0, ...values.map((candidate) => searchScore(normalizedQuery, candidate)));
  }

  function filteredAugments() {
    const query = normalizeSearch(state.augmentQuery);
    const rarityOrder = { prismatic: 0, gold: 1, silver: 2, unknown: 3 };
    return objectRows(state.augments?.rows)
      .map((item) => ({ ...item, rarity: augmentRarityKey(item.rarity) }))
      .filter((item) => (state.augmentRarity === "all" || item.rarity === state.augmentRarity) && (!query || normalizeSearch(`${item.name}${item.description}${item.tooltip}${item.key}`).includes(query)))
      .sort((a, b) => Number(a.tier) - Number(b.tier) || (rarityOrder[a.rarity] ?? 3) - (rarityOrder[b.rarity] ?? 3) || String(a.name).localeCompare(String(b.name), "zh-CN"));
  }

  function fuzzySubsequence(query, value) {
    if (query.length < 3 || value.length > Math.max(48, query.length * 8)) return false;
    let index = 0;
    for (const character of value) if (character === query[index]) index += 1;
    return index === query.length;
  }

  function renderSkeleton() { return `<div class="champions-skeleton" aria-label="正在联网读取英雄数据"><span></span><span></span><span></span><span></span><span></span><span></span></div>`; }
  function renderDetailSkeleton() { return `<div class="champion-detail-skeleton" aria-label="正在读取英雄详情"><span></span><span></span><span></span></div>`; }
  function renderError(message, detail = false) { return `<div class="champion-state is-error"><span aria-hidden="true">!</span><strong>${detail ? "详情读取失败" : "数据读取失败"}</strong><p>${escapeHTML(message)}</p><button class="text-button" type="button" data-champion-retry>${detail ? "重新读取详情" : "重试"}</button></div>`; }
  function renderEmpty(title, copy) { return `<div class="champion-state"><span aria-hidden="true">◇</span><strong>${title}</strong><p>${copy}</p><button class="text-button" type="button" data-champion-clear>清除筛选</button></div>`; }
  function prepareImages(container = root) {
    for (const image of container.querySelectorAll("[data-champion-image]")) {
      const loaded = () => { image.hidden = false; image.parentElement?.classList.add("has-loaded-image"); window.deepLegendsAugmentArtwork?.prepare(image); };
      const failed = () => {
        const holder = image.parentElement;
        const artworkFallback = holder?.dataset.artworkFallback || image.dataset.artworkFallback;
        if (artworkFallback && !image.dataset.artworkFallbackUsed) {
          image.dataset.artworkFallbackUsed = "1";
          image.hidden = false;
          image.addEventListener("load", loaded, { once: true });
          image.addEventListener("error", failed, { once: true });
          image.setAttribute("data-queued-src", artworkFallback);
          return;
        }
        const fallback = holder?.dataset.augmentFallback;
        if (fallback && !image.dataset.augmentFallbackUsed) {
          image.dataset.augmentFallbackUsed = "1";
          image.hidden = false;
          image.addEventListener("load", loaded, { once: true });
          image.addEventListener("error", failed, { once: true });
          image.setAttribute("data-queued-src", fallback);
          return;
        }
        image.hidden = true;
        holder?.classList.remove("has-loaded-image");
      };
      if (image.getAttribute("src") && image.complete) image.naturalWidth > 0 ? loaded() : failed();
      else {
        image.addEventListener("load", loaded, { once: true });
        image.addEventListener("error", failed, { once: true });
      }
    }
  }

  function applyRenderedMetricStyles() {
    for (const fill of root.querySelectorAll("[data-bar-width]")) {
      const width = Math.max(0, Math.min(100, Number(fill.dataset.barWidth) || 0));
      fill.style.width = `${width}%`;
    }
    for (const grid of root.querySelectorAll("[data-skill-count]")) {
      const count = Math.max(1, Math.min(18, Number(grid.dataset.skillCount) || 1));
      grid.style.setProperty("--skill-count", String(count));
    }
  }

  function resetTransientChampionState({ restorePosition = false } = {}) {
    closeMayhemTierDialog(false);
    closeArenaTierDialog(false);
	    state.augmentRarity = "all";
    // 阶段筛选是会话内的视图状态：换模式/重置时回到英雄级汇总，避免一个记住的
    // 阶段号在新英雄身上把整片推荐过滤空。
    state.mayhemStage = 0;
    state.mayhemAugmentID = 0;
    state.mayhemAugmentDetail = null;
    state.mayhemAugmentLoading = false;
    state.mayhemAugmentError = "";
    state.mayhemAtlasLoading = false;
    state.mayhemAtlasError = "";
    state.mayhemRarityData = null;
    state.mayhemRarityLoading = false;
    state.mayhemRarityError = "";
    state.mayhemPersonalBuilds = null;
    state.mayhemPersonalBuildsKey = 0;
    state.mayhemPersonalBuildsLoading = false;
    state.mayhemPersonalBuildsError = "";
    state.mayhemDetailLoading = false;
    state.mayhemDetailError = "";
    state.mayhemDetailKey = 0;
    state.mayhemRequestToken += 1;
    state.mayhemAugmentRequestToken += 1;
    state.mayhemDialogOpen = false;
    state.mayhemDialogReturnFocus = null;
    state.arenaDialogOpen = false;
    state.arenaDialogReturnFocus = null;
    state.detail = null;
    state.selected = null;
    state.error = "";
    state.listScroll = 0;
	    state.runePage = normalizeRunePage(readSetting("champion-rune-page", "0"));
    state.workspaceRequestToken += 1;
    state.detailRequestToken += 1;
    state.arenaRequestToken += 1;
    state.arenaDetailLoading = false;
    state.arenaDetailError = "";
    state.arenaDetailKey = 0;
    resetArenaControls();
    state.detailPosition = null;
    if (restorePosition) state.position = normalizePosition(readSetting("champion-position", "all"));
  }

  root.addEventListener("click", (event) => {
    const playerRow = event.target.closest("[data-player-name]");
    if (playerRow) {
      // 玩家榜数据来自韩服，点击后在当前页面以覆盖层展示韩服总览。
      state.playerDetour = true;
      window.dispatchEvent(new CustomEvent("deep-legends:open-player", { detail: { gameName: playerRow.dataset.playerName, tagLine: playerRow.dataset.playerTag || "", region: "kr", source: "champions" } }));
      return;
    }
    const runeTab = event.target.closest("[data-rune-page]");
    if (runeTab) {
      const index = Number(runeTab.dataset.runePage) || 0;
      state.runePage = index;
	      writeSetting("champion-rune-page", state.runePage);
      render();
      return;
    }
    const mode = event.target.closest("[data-champion-mode]");
    if (mode) {
      switchChampionMode(mode.dataset.championMode);
      return;
    }
    const position = event.target.closest("[data-champion-position]");
    if (position) {
      state.position = normalizePosition(position.dataset.championPosition);
      writeSetting("champion-position", state.position);
      if (settingPosition) settingPosition.value = state.position;
      loadRankings();
      return;
    }
    const rarity = event.target.closest("[data-augment-rarity]");
    if (rarity) { state.augmentRarity = rarity.dataset.augmentRarity; render(); return; }
    // R116-B：详情页三 tab 与阶段 chips 都走「局部替换」而不是整棵重渲染——
    // 只换面板内容与页签状态，切阶段也不许发请求（R128 起详情页已无口径页脚）。
    const mayhemDetailTab = event.target.closest("[data-mayhem-detail-tab]");
    if (mayhemDetailTab) { switchMayhemDetailTab(mayhemDetailTab.dataset.mayhemDetailTab); return; }
    const mayhemStage = event.target.closest("[data-mayhem-stage]");
    if (mayhemStage) { switchMayhemStage(mayhemStage.dataset.mayhemStage); return; }
    const mayhemView = event.target.closest("[data-mayhem-view]");
    if (mayhemView) {
	      state.mayhemView = normalizeMayhemView(mayhemView.dataset.mayhemView);
	      writeSetting("champion-mayhem-view", state.mayhemView);
      if (state.mayhemView === "atlas") {
        render();
        loadMayhemAtlas();
      } else {
        const pending = prepareMayhemSelection();
        render();
        if (pending) loadMayhemDetail(pending);
      }
      return;
    }
    const mayhemAugment = event.target.closest("[data-mayhem-augment]");
    if (mayhemAugment) {
      const item = objectRows(state.augments?.rows).find((row) => Number(row.id) === Number(mayhemAugment.dataset.mayhemAugment));
      if (item) loadMayhemAugmentDetail(item);
      return;
    }
    if (event.target.closest("[data-load-mayhem-rarity]")) { loadMayhemRarity(); return; }
    const mayhemTierTrigger = event.target.closest("[data-open-mayhem-tiers]");
    if (mayhemTierTrigger) { openMayhemTierDialog(mayhemTierTrigger); return; }
    if (event.target.closest("[data-close-mayhem-tiers]")) { closeMayhemTierDialog(); return; }
    const arenaTierTrigger = event.target.closest("[data-open-arena-tiers]");
    if (arenaTierTrigger) { openArenaTierDialog(arenaTierTrigger); return; }
    if (event.target.closest("[data-close-arena-tiers]")) { closeArenaTierDialog(); return; }
    const arenaSort = event.target.closest("[data-arena-sort]");
    if (arenaSort) { state.arenaSort = arenaSort.dataset.arenaSort; render(); return; }
    const arenaRarity = event.target.closest("[data-arena-rarity]");
    if (arenaRarity) {
      state.arenaRarity = arenaRarity.dataset.arenaRarity;
      state.arenaExpanded.augments = false;
      render();
      return;
    }
    const arenaExpand = event.target.closest("[data-arena-expand]");
    if (arenaExpand) {
      const key = arenaExpand.dataset.arenaExpand;
      if (Object.prototype.hasOwnProperty.call(state.arenaExpanded, key)) state.arenaExpanded[key] = !state.arenaExpanded[key];
      render();
      return;
    }
    const arenaFirstTab = event.target.closest("[data-arena-first-tab]");
	    if (arenaFirstTab && !arenaFirstTab.disabled) { state.arenaFirstTab = normalizeArenaFirstTab(arenaFirstTab.dataset.arenaFirstTab); writeSetting("champion-arena-first-tab", state.arenaFirstTab); render(); return; }
    const counter = event.target.closest("[data-counter-champion]");
    if (counter) { openCounterDetail(counter.dataset.counterChampion); return; }
    const detailPosition = event.target.closest("[data-detail-position]");
    if (detailPosition) { void switchDetailPosition(normalizePosition(detailPosition.dataset.detailPosition)); return; }
    const rowElement = event.target.closest("[data-champion-row]");
    if (rowElement) {
      const row = rankingRows().find((item) => Number(item.championId) === Number(rowElement.dataset.championRow));
      if (row) openDetail(row);
      return;
    }
    if (event.target.closest("[data-champion-back]")) { closeDetail(); return; }
    if (event.target.closest("[data-champion-refresh]")) {
      if (state.mode === "arena") { state.selected = null; state.detail = null; state.arenaDetailKey = 0; resetArenaControls(); }
      if (state.mode === "aram-mayhem") state.mayhemDetailKey = 0;
      loadWorkspace(true);
      return;
    }
    if (event.target.closest("[data-champion-retry]")) {
      if (state.mode === "aram-mayhem" && state.mayhemView === "atlas") {
        const item = objectRows(state.augments?.rows).find((row) => Number(row.id) === Number(state.mayhemAugmentID));
        if (state.mayhemAugmentError && item) loadMayhemAugmentDetail(item);
        else loadMayhemAtlas(true);
      } else if (state.selected) openDetail(state.selected);
      else loadWorkspace(true);
      return;
    }
    if (event.target.closest("[data-champion-clear]")) {
      if (state.mode === "aram-mayhem" && state.mayhemView === "atlas") {
        state.augmentQuery = "";
        state.augmentRarity = "all";
        writeSetting("champion-augment-query", "");
      } else {
        state.query = "";
        writeSetting(`champion-query-${state.mode}`, "");
      }
      render();
    }
  });

  root.addEventListener("change", (event) => {
    if (event.target.matches("[data-champion-tier]")) {
	  const tier = normalizeTier(event.target.value);
	  writeSetting("champion-tier", tier);
	  if (state.mode === "ranked" && state.selected) switchDetailTier(tier);
	  else {
		state.tier = tier;
		loadRankings();
	  }
    }
  });

  root.addEventListener("input", (event) => {
    if (searchComposing || event.isComposing) return;
    if (event.target.matches("[data-champion-search]")) updateChampionSearch(event.target);
	    if (event.target.matches("[data-augment-search]")) preserveSearchInput(event.target, "[data-augment-search]", () => { state.augmentQuery = normalizeStoredSearch(event.target.value); writeSetting("champion-augment-query", state.augmentQuery); });
  });

  root.addEventListener("compositionstart", () => { searchComposing = true; clearTimeout(state.searchTimer); });
  root.addEventListener("compositionend", (event) => {
    searchComposing = false;
    if (event.target.matches("[data-champion-search]")) updateChampionSearch(event.target);
	    if (event.target.matches("[data-augment-search]")) preserveSearchInput(event.target, "[data-augment-search]", () => { state.augmentQuery = normalizeStoredSearch(event.target.value); writeSetting("champion-augment-query", state.augmentQuery); });
  });

  function preserveSearchInput(input, selector, update) {
    const start = input.selectionStart ?? input.value.length;
    const end = input.selectionEnd ?? start;
    update();
    render();
    const next = root.querySelector(selector);
    next?.focus({ preventScroll: true });
    next?.setSelectionRange(Math.min(start, next.value.length), Math.min(end, next.value.length));
  }

  function renderChampionResults(rows) {
    if (!rows.length) return renderEmpty("没有匹配的英雄", "清除搜索词后查看全部梯度。");
    if (state.mode === "arena") return renderChampionTable(rows, "arena");
    if (state.mode === "aram-mayhem") return renderChampionTable(rows, "mayhem");
    return state.query.trim() || rows.length <= 3 ? renderChampionTable(rows, true)
      : renderTopThree(rows.slice(0, 3)) + renderChampionTable(rows.slice(3), true, 3);
  }

  function renderChampionSearchResults() {
    const results = root.querySelector("[data-champion-results]");
    if (!results) return;
    const rows = filteredChampionRows();
    results.innerHTML = (state.error ? renderError(state.error) : "") + renderChampionResults(rows);
    const count = root.querySelector(".champion-result-count");
    if (count) count.textContent = `${rows.length} 位英雄`;
    for (const button of root.querySelectorAll("[data-champion-position]")) {
      const active = button.dataset.championPosition === state.position;
      button.classList.toggle("is-active", active);
      button.setAttribute("aria-selected", String(active));
    }
    prepareImages(results);
  }

  function flushChampionSearch() {
    clearTimeout(state.searchTimer);
    const pending = state.pendingSearch;
    state.pendingSearch = null;
    if (pending && pending.mode === state.mode && pending.query === state.query) writeSetting(`champion-query-${pending.mode}`, pending.query);
  }

  function updateChampionSearch(input) {
    clearTimeout(state.searchTimer);
    const mode = state.mode, query = normalizeStoredSearch(input.value);
    state.query = query;
    state.pendingSearch = { mode, query };
    state.searchTimer = setTimeout(() => {
      if (state.mode !== mode || !input.isConnected || searchComposing) return;
      flushChampionSearch();
      if (mode === "ranked" && normalizeSearch(query) && state.position !== "all") {
        state.position = "all";
        void loadRankings(true);
      } else renderChampionSearchResults();
    }, 150);
  }

  root.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && state.arenaDialogOpen) {
      event.preventDefault();
      event.stopPropagation();
      closeArenaTierDialog();
      return;
    }
    if (event.key === "Escape" && state.mayhemDialogOpen) {
      event.preventDefault();
      event.stopPropagation();
      closeMayhemTierDialog();
      return;
    }
    const row = event.target.closest("[data-champion-row]");
    if (row && (event.key === "Enter" || event.key === " ")) { event.preventDefault(); row.click(); }
    if (event.key === "Escape" && state.selected && state.mode === "ranked") { event.preventDefault(); closeDetail(); }
  });

  function handleLazySection(event) {
    const previous = state.section;
    state.section = event.detail?.name || "overview";
    if (previous === "champions" && state.section !== "champions") {
      // 查看玩家详情的临时跳转保留英雄详情现场，返回时原样恢复。
      if (state.playerDetour) return;
      state.requests.get("detail")?.abort();
	      // 只清理详情与加载态；模式、搜索和页签属于用户偏好，返回时继续使用。
	      resetTransientChampionState({ restorePosition: true });
      return;
    }
    if (state.section === "champions" && previous !== "champions") {
      if (state.playerDetour) {
        state.playerDetour = false;
        render();
        return;
      }
      // 数据还在路上时会先渲染一版矮得多的加载态，滚动位置必须等到真正
      // 还原成功才允许被重新采样，否则会被加载态的 0 覆盖掉。
      state.listScrollRestorePending = (state.listScrollInner || 0) > 0;
      enterChampionSection();
    }
  }
  window.addEventListener("deep-legends:section", handleLazySection);
  window.deepLegendsSections?.register("champions", handleLazySection);

  if (settingPosition) {
    settingPosition.value = state.position;
    settingPosition.addEventListener("change", () => {
      state.position = normalizePosition(settingPosition.value);
      writeSetting("champion-position", state.position);
      if (state.section === "champions" && state.mode === "ranked" && !state.selected) loadRankings();
    });
  }

  window.addEventListener("deep-legends:open-champion", event => {
    const detail = event.detail || {};
    const championId = normalizeChampionDetailID(detail.championId);
    if (!championId || !/^[a-z0-9-]+$/.test(String(detail.key || ""))) return;
    state.mode = "ranked";
    state.playerDetour = false;
    void openDetail({ championId, key: detail.key, position: normalizePosition(detail.position) });
  });

  window.addEventListener("pagehide", flushChampionSearch);
  window.deepLegendsChampionSearch = Object.freeze({ scoreOption: scoreChampionSelectOption });
  window.deepLegendsChampionAsset = Object.freeze({ imageHTML: assetImage, imageURL });
  render();
  beginStartupPreload();
  adoptCatalog(state.preload?.catalog);
})();
