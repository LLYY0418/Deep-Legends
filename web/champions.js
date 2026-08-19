(() => {
  "use strict";

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

  const state = {
    section: "overview",
    // 模式与段位不做持久化：离开英雄页后恢复默认，重新进入时从头开始。
    mode: "ranked",
    tier: "emerald_plus",
    position: normalizePosition(readSetting("champion-position", "all")),
    query: "",
    augmentQuery: "",
    augmentRarity: "all",
    catalog: null,
    rankings: null,
    augments: null,
    detail: null,
    selected: null,
    loading: false,
    error: "",
    requests: new Map(),
    listScroll: 0,
    preload: null,
    preloaded: Object.create(null),
  };

  let searchComposing = false;

  function readSetting(key, fallback) { try { return localStorage.getItem(`lol-loot-${key}`) ?? fallback; } catch (_) { return fallback; } }
  function writeSetting(key, value) { try { localStorage.setItem(`lol-loot-${key}`, String(value)); } catch (_) {} }
  function normalizeTier(value) { return fallbackTiers.some((item) => item.value === value) ? value : "emerald_plus"; }
  function normalizePosition(value) { return positionOptions.some((item) => item.value === value) ? value : "all"; }
  function normalizeMode(value) { return ["ranked", "aram-mayhem", "arena"].includes(value) ? value : "ranked"; }
  function escapeHTML(value) { const span = document.createElement("span"); span.textContent = String(value ?? ""); return span.innerHTML; }
  function normalizeSearch(value) { return String(value || "").normalize("NFKC").toLocaleLowerCase("zh-CN").replace(/[\s\p{P}\p{S}]+/gu, ""); }
  function number(value, digits = 1) { const parsed = Number(value); return Number.isFinite(parsed) ? new Intl.NumberFormat("zh-CN", { maximumFractionDigits: digits }).format(parsed) : "—"; }
  function percent(value) { const parsed = Number(value); return Number.isFinite(parsed) && parsed !== 0 ? `${parsed.toFixed(2)}%` : "—"; }
  function compactNumber(value) { const parsed = Number(value); if (!Number.isFinite(parsed) || parsed <= 0) return "—"; return parsed >= 10000 ? `${(parsed / 10000).toFixed(parsed >= 100000 ? 0 : 1)}万` : new Intl.NumberFormat("zh-CN").format(parsed); }
  function positionLabel(value) { return ({ top: "上单", jungle: "打野", mid: "中单", adc: "下路", support: "辅助", all: "全部" })[value] || value || "位置未知"; }
  function positionIcon(value) {
    const name = ({ all: "all", top: "top", jungle: "jungle", mid: "middle", adc: "bottom", support: "utility" })[value] || "all";
    return `<span class="position-icon" aria-hidden="true"><img src="/position-icons/${name}.svg" alt="" decoding="async"></span>`;
  }
  function tierDisplay(value) { const parsed = Number(value); return Number.isFinite(parsed) ? (parsed === 0 ? "OP" : String(parsed)) : "—"; }
  function tierBadge(value, extra = "") {
    const parsed = Number(value);
    const key = Number.isFinite(parsed) && parsed >= 0 && parsed <= 5 ? (parsed === 0 ? "op" : String(parsed)) : "";
    const label = tierDisplay(value);
    return key
      ? `<img class="tier-badge${extra ? ` ${extra}` : ""}" src="/tier-icons/${key}.svg" alt="梯度 ${label}" decoding="async">`
      : `<span class="tier-badge-fallback${extra ? ` ${extra}` : ""}">${label}</span>`;
  }
  function tierLabel(value) { return state.catalog?.tiers?.find((item) => item.value === value)?.label || fallbackTiers.find((item) => item.value === value)?.label || value; }
  function championMeta(id) { return state.catalog?.champions?.find((item) => Number(item.id) === Number(id)) || null; }
  function championMetaByKey(key) { const value = String(key || "").toLowerCase(); return state.catalog?.champions?.find((item) => String(item.slug || item.key || "").toLowerCase() === value || String(item.key || "").toLowerCase() === value) || null; }
  function imageURL(source, path) { return source && path ? `/api/champion-asset?source=${encodeURIComponent(source)}&path=${encodeURIComponent(path)}` : "/image-unavailable.svg"; }
  function heroArtworkURL(meta, fallbackSource, fallbackPath) {
    const id = Number(meta?.id) || 0;
    if (id > 0) return imageURL("gtimg", `/images/lol/act/img/skin/big${id * 1000}.jpg`);
    const key = String(meta?.key || "").replace(/[^A-Za-z0-9]/g, "");
    return key ? imageURL("ddragon", `/cdn/img/champion/splash/${key}_0.jpg`) : imageURL(fallbackSource, fallbackPath);
  }
  function runeStyleIcon(style, className = "rune-style-icon") {
    const names = { 8000: "precision", 8100: "domination", 8200: "sorcery", 8300: "inspiration", 8400: "resolve" };
    const file = names[Number(style?.id)];
    const label = style?.name || "符文系";
    if (!file) return style?.path ? assetImage(style, className, false) : "";
    return `<span class="champion-asset ${className}" title="${escapeHTML(label)}"><img src="/rune-styles/${file}.svg" alt="${escapeHTML(label)}" loading="lazy" decoding="async"></span>`;
  }
  function assetImage(asset, className = "recommend-icon", withTooltip = true) {
    const name = asset?.name || "图标";
    const explanation = String(asset?.description || "").trim();
    const tooltip = explanation && explanation !== name ? `${name}\n${explanation}` : name;
    return `<span class="champion-asset ${className}"${withTooltip ? ` tabindex="0" data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}"` : ""}><img src="${imageURL(asset?.source, asset?.path)}" alt="${escapeHTML(name)}" loading="lazy" decoding="async" data-champion-image><span aria-hidden="true">${escapeHTML(name.slice(0, 1))}</span></span>`;
  }

  async function api(path, key = path) {
    state.requests.get(key)?.abort();
    const controller = new AbortController();
    state.requests.set(key, controller);
    const timer = setTimeout(() => controller.abort(), 35000);
    try {
      const response = await fetch(path, { headers: { Accept: "application/json" }, signal: controller.signal });
      if (response.status === 401) throw new Error("页面会话已过期，刷新页面即可重新连接");
      if (!response.ok) throw new Error((await response.text()).trim() || `本地服务返回 HTTP ${response.status}`);
      return response.json();
    } catch (error) {
      if (error.name === "AbortError") throw new Error("联网读取超时，请重试");
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
      aram: remember("aram", api("/api/champions/rankings?mode=aram-mayhem", "preload-aram")),
      arena: remember("arena", api("/api/champions/rankings?mode=arena", "preload-arena")),
      augments: remember("augments", api("/api/champions/augments", "preload-augments")),
    };
  }

  function rankingsCacheKey() { return `${state.mode}|${state.tier}|${state.position}`; }
  function stampRankings() { state.rankingsKey = rankingsCacheKey(); state.rankingsAt = Date.now(); }
  function rankingsFresh() {
    return Boolean(state.rankings && state.rankingsKey === rankingsCacheKey() && Date.now() - (state.rankingsAt || 0) < 5 * 60 * 1000 && (state.mode !== "aram-mayhem" || state.augments));
  }

  async function enterChampionSection() {
    const preload = state.preload;
    let hydrated = false;
    if (preload) {
      state.loading = true;
      render();
      if (state.mode === "ranked" && preload.tier === state.tier && preload.position === state.position) {
        const [catalog, rankings] = await Promise.all([preload.catalog, preload.ranked]);
        if (catalog && rankings) {
          state.catalog = catalog;
          state.rankings = rankings;
          state.augments = null;
          hydrated = true;
          delete state.preloaded.ranked;
        }
      } else if (state.mode === "aram-mayhem") {
        const [catalog, rankings, augments] = await Promise.all([preload.catalog, preload.aram, preload.augments]);
        if (catalog && rankings && augments) {
          state.catalog = catalog;
          state.rankings = rankings;
          state.augments = augments;
          hydrated = true;
          delete state.preloaded.aram;
          delete state.preloaded.augments;
        }
      } else if (state.mode === "arena") {
        const [catalog, rankings] = await Promise.all([preload.catalog, preload.arena]);
        if (catalog && rankings) {
          state.catalog = catalog;
          state.rankings = rankings;
          state.augments = null;
          hydrated = true;
          delete state.preloaded.arena;
        }
      }
      state.preload = null;
      state.loading = false;
      if (state.section !== "champions") return;
      if (hydrated) {
        stampRankings();
        render();
        return;
      }
    }
    if (state.section !== "champions") return;
    if (rankingsFresh()) { render(); return; }
    loadWorkspace(true);
  }

  async function loadWorkspace(force = false) {
    if (state.loading) return;
    if (force && state.mode === "arena" && state.preloaded.arena) {
      state.catalog ||= state.preloaded.catalog;
      state.rankings = state.preloaded.arena;
      state.augments = null;
      delete state.preloaded.arena;
      stampRankings();
      render();
      return;
    }
    if (force && state.mode === "aram-mayhem" && state.preloaded.aram && state.preloaded.augments) {
      state.catalog ||= state.preloaded.catalog;
      state.rankings = state.preloaded.aram;
      state.augments = state.preloaded.augments;
      delete state.preloaded.aram;
      delete state.preloaded.augments;
      stampRankings();
      render();
      return;
    }
    if (!force && state.rankings && (state.mode !== "aram-mayhem" || state.augments)) { render(); return; }
    state.loading = true;
    state.error = "";
    state.detail = null;
    render();
    try {
      const catalogPromise = state.catalog && !force ? Promise.resolve(state.catalog) : api("/api/champions/catalog", "catalog");
      if (state.mode === "ranked") {
        const rankingsPromise = api(`/api/champions/rankings?mode=ranked&tier=${encodeURIComponent(state.tier)}&position=${encodeURIComponent(state.position)}`, "rankings");
        [state.catalog, state.rankings] = await Promise.all([catalogPromise, rankingsPromise]);
        state.augments = null;
      } else if (state.mode === "aram-mayhem") {
        const rankingsPromise = api("/api/champions/rankings?mode=aram-mayhem", "rankings");
        const augmentsPromise = api("/api/champions/augments", "augments");
        [state.catalog, state.rankings, state.augments] = await Promise.all([catalogPromise, rankingsPromise, augmentsPromise]);
      } else if (state.mode === "arena") {
        const rankingsPromise = api("/api/champions/rankings?mode=arena", "rankings");
        [state.catalog, state.rankings] = await Promise.all([catalogPromise, rankingsPromise]);
        state.augments = null;
      }
      stampRankings();
    } catch (error) {
      state.error = error.message || "数据读取失败";
    } finally {
      state.loading = false;
      render();
    }
  }

  async function loadRankings() {
    if (state.loading) return;
    state.loading = true;
    state.error = "";
    state.rankings = null;
    render();
    try {
      if (!state.catalog) state.catalog = await api("/api/champions/catalog", "catalog");
      state.rankings = await api(`/api/champions/rankings?mode=ranked&tier=${encodeURIComponent(state.tier)}&position=${encodeURIComponent(state.position)}`, "rankings");
      stampRankings();
    } catch (error) {
      state.error = error.message || "梯度读取失败";
    } finally {
      state.loading = false;
      render();
    }
  }

  async function openDetail(row) {
    const meta = championMeta(row.championId);
    const champion = String(row.key || meta?.slug || "").toLowerCase();
    if (!champion) return;
    if (!state.selected) state.listScroll = appScroll?.scrollTop || 0;
    state.selected = { ...row, meta, champion };
    state.detail = null;
    state.error = "";
    state.runePage = 0;
    state.loading = true;
    render();
    appScroll?.scrollTo({ top: 0, behavior: "instant" });
    const position = row.position || state.position;
    const query = state.mode === "ranked"
      ? `mode=ranked&champion=${encodeURIComponent(champion)}&position=${encodeURIComponent(position)}&tier=${encodeURIComponent(state.tier)}`
      : `mode=${encodeURIComponent(state.mode)}&champion=${encodeURIComponent(champion)}`;
    try {
      state.detail = await api(`/api/champions/detail?${query}`, "detail");
    } catch (error) {
      state.error = error.message || "英雄详情读取失败";
    } finally {
      state.loading = false;
      render();
      root.querySelector("[data-champion-back]")?.focus({ preventScroll: true });
    }
  }

  function closeDetail() {
    state.detail = null;
    state.selected = null;
    state.error = "";
    render();
    requestAnimationFrame(() => appScroll?.scrollTo({ top: state.listScroll, behavior: "instant" }));
  }

  function openCounterDetail(key) {
    const meta = championMetaByKey(key);
    if (!meta) return;
    const ranked = (state.rankings?.rows || []).find((item) => Number(item.championId) === Number(meta.id)) || {};
    openDetail({ ...ranked, championId: meta.id, key: meta.slug, name: meta.nameZh, position: state.selected?.position || state.position, meta });
  }

  function render() {
    if (state.selected) renderDetail();
    else renderWorkspace();
    prepareImages();
  }

  function renderWorkspace() {
    const patchNote = state.rankings?.patch ? `版本 ${state.rankings.patch} · ` : "";
    const dataNote = state.mode === "ranked"
      ? `${patchNote}${tierLabel(state.tier)}`
      : state.mode === "arena"
        ? `${patchNote}三人队伍协同与竞技场构建`
        : `${patchNote}随机英雄与海克斯强化，不按段位拆分`;
    root.innerHTML = `
      <header class="champion-page-head">
        <div><h2>英雄梯度与构建</h2><p>${escapeHTML(dataNote)}</p></div>
        <div class="champion-head-actions">
          ${state.mode === "ranked" ? `<label class="champion-tier-select"><span>段位</span><select data-champion-tier>${renderTierOptions()}</select></label>` : ""}
          <button class="text-button" type="button" data-champion-refresh ${state.loading ? "disabled" : ""}>${state.loading ? "正在联网…" : "刷新数据"}</button>
        </div>
      </header>
      <nav class="champion-mode-tabs" role="tablist" aria-label="游戏模式">
        ${modeTab("ranked", "单/双排", "韩服排位梯度与出装")}
        ${modeTab("aram-mayhem", "海克斯大乱斗", "英雄与全部海克斯")}
        ${modeTab("arena", "斗魂竞技场", "三人队伍与竞技场构建")}
      </nav>
      ${state.error && !state.rankings ? renderError(state.error) : state.loading && !state.rankings ? renderSkeleton() : state.mode === "ranked" ? renderRanked() : state.mode === "arena" ? renderArena() : renderARAM()}`;
  }

  function renderTierOptions() {
    const tiers = state.catalog?.tiers?.length ? state.catalog.tiers : fallbackTiers;
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
        <div class="champion-position-tabs" role="tablist" aria-label="英雄位置">${positionOptions.map((item) => `<button type="button" role="tab" aria-label="${item.label}" title="${item.label}" aria-selected="${item.value === state.position}" class="${item.value === state.position ? "is-active" : ""}" data-champion-position="${item.value}">${positionIcon(item.value)}<span>${item.label}</span></button>`).join("")}</div>
        <span class="champion-result-count" role="status">${rows.length} 位英雄</span>
      </div>
      ${rows.length ? (state.query.trim() || rows.length <= 3 ? renderChampionTable(rows, true) : renderTopThree(rows.slice(0, 3)) + renderChampionTable(rows.slice(3), true, 3)) : renderEmpty("没有匹配的英雄", "试试中文名、英文名、拼音首字母或常用外号。")}
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
        <img class="topcard-art" src="${heroArtworkURL(meta, source, path)}" alt="" loading="lazy" decoding="async" data-champion-image>
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
    const augments = filteredAugments();
    return `<div class="aram-workspace">
      <section class="champion-list-card aram-champions">
        <div class="aram-champion-head"><div><h3>英雄梯度</h3><p>点击英雄查看海克斯与完整构建</p></div><span class="champion-result-count">${rows.length} 位</span></div>
        <label class="champion-search compact"><span aria-hidden="true">⌕</span><span class="sr-only">搜索英雄</span><input type="search" value="${escapeHTML(state.query)}" placeholder="搜索英雄" autocomplete="off" data-champion-search></label>
        ${rows.length ? renderChampionTable(rows, false) : renderEmpty("没有匹配的英雄", "清除搜索词后查看全部梯度。")}
      </section>
      <section class="augment-directory">
        <div class="augment-toolbar"><div><h3>全部海克斯</h3><p>按梯度排序，同梯度内依次为棱彩、黄金、白银</p></div><label class="champion-search compact"><span aria-hidden="true">⌕</span><span class="sr-only">搜索海克斯</span><input type="search" value="${escapeHTML(state.augmentQuery)}" placeholder="搜索海克斯名称或效果" autocomplete="off" data-augment-search></label></div>
        <div class="augment-rarity-tabs" role="tablist" aria-label="海克斯品质">${[["all", "全部"], ["prismatic", "棱彩"], ["gold", "黄金"], ["silver", "白银"]].map(([value, label]) => `<button type="button" role="tab" aria-selected="${state.augmentRarity === value}" class="${state.augmentRarity === value ? "is-active" : ""}" data-augment-rarity="${value}">${label}</button>`).join("")}</div>
        ${augments.length ? renderAugmentGroups(augments) : renderEmpty("没有匹配的海克斯", "更换品质或清除搜索词后重试。")}
      </section>
    </div>`;
  }

  function renderArena() {
    const rows = filteredChampionRows();
    const teams = state.rankings?.teamCompositions || [];
    return `<div class="aram-workspace arena-workspace">
      <section class="champion-list-card aram-champions">
        <div class="aram-champion-head"><div><h3>英雄梯度</h3><p>点击英雄查看三人搭配与完整构建</p></div><span class="champion-result-count">${rows.length} 位</span></div>
        <label class="champion-search compact"><span aria-hidden="true">⌕</span><span class="sr-only">搜索英雄</span><input type="search" value="${escapeHTML(state.query)}" placeholder="搜索英雄" autocomplete="off" data-champion-search></label>
        ${rows.length ? renderChampionTable(rows, false) : renderEmpty("没有匹配的英雄", "清除搜索词后查看全部梯度。")}
      </section>
      <section class="arena-synergy-panel">
        <header class="arena-synergy-head"><div><h3>三人队伍协同</h3><p>前三组优先展示，完整比较平均名次、第一名与胜率</p></div><span class="section-count">${teams.length} 组</span></header>
        ${teams.length ? renderArenaTeamPodium(teams.slice(0, 3)) + renderArenaTeamList(teams.slice(3)) : renderEmpty("暂无队伍样本", "OP.GG 暂时没有返回竞技场队伍组合。")}
      </section>
    </div>`;
  }

  function renderArenaTeamPodium(teams) {
    return `<div class="arena-team-podium">${teams.map((team, index) => `<article class="arena-team-place is-${index + 1}"><b aria-label="第 ${index + 1} 名">${index + 1}</b>${renderArenaTeamFaces(team.champions, true)}<strong>${teamName(team)}</strong><dl><div><dt>平均名次</dt><dd>${number(team.averagePlacement, 2)}</dd></div><div><dt>胜率</dt><dd class="metric-win">${percent(team.winRate)}</dd></div></dl></article>`).join("")}</div>`;
  }

  function renderArenaTeamList(teams) {
    if (!teams.length) return "";
    return `<div class="arena-team-list" role="table" aria-label="更多竞技场队伍组合"><div class="arena-team-list-head" role="row"><span>队伍</span><span>平均名次</span><span>第一名</span><span>选用率</span><span>胜率</span></div>${teams.map((team) => `<article class="arena-team-row" role="row"><div class="arena-team-identity" role="cell">${renderArenaTeamFaces(team.champions)}<strong>${teamName(team)}</strong></div><span data-label="平均名次">${number(team.averagePlacement, 2)}</span><span data-label="第一名" class="metric-first">${percent(team.firstPlaceRate)}</span><span data-label="选用率" class="metric-pick"><b>${percent(team.pickRate)}</b><small>${compactNumber(team.games)} 场</small></span><span data-label="胜率" class="metric-win">${percent(team.winRate)}</span></article>`).join("")}</div>`;
  }

  function renderArenaTeamFaces(champions, large = false) {
    return `<div class="arena-team-faces${large ? " is-large" : ""}">${(champions || []).slice(0, 3).map((champion) => `<span class="champion-portrait"><img src="${imageURL(champion.imageSource, champion.imagePath)}" alt="${escapeHTML(champion.name || "英雄")}" loading="lazy" decoding="async" data-champion-image><span>${escapeHTML((champion.name || "?").slice(0, 1))}</span></span>`).join("")}</div>`;
  }

  function teamName(team) { return (team?.champions || []).map((champion) => champion.name).filter(Boolean).join(" + ") || "未知队伍"; }

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
    const showPosition = metrics && state.position === "all";
    return `<div class="champion-table-scroll"><table class="champion-table${showPosition ? " has-position" : ""}"><thead><tr><th>排名</th><th>英雄</th><th>梯度</th>${showPosition ? "<th>位置</th>" : ""}${metrics ? '<th class="metric-win">胜率</th><th class="metric-pick">选用率</th><th class="metric-ban">禁用率</th><th class="metric-games">场次</th>' : ""}</tr></thead><tbody>${rows.map((row, index) => renderChampionRow(row, metrics, showPosition, index + rankOffset)).join("")}</tbody></table></div>`;
  }

  function renderChampionRow(row, metrics, showPosition, index) {
    const meta = championMeta(row.championId);
    const name = row.name || meta?.nameZh || meta?.titleZh || `英雄 ${row.championId}`;
    const subname = meta?.titleZh && meta.titleZh !== name ? meta.titleZh : meta?.nameEn || "";
    const source = row.imageSource || meta?.imageSource;
    const path = row.imagePath || meta?.imagePath;
    const rank = Number(row.rank) > 0 ? row.rank : index + 1;
    return `<tr tabindex="0" role="button" data-champion-row="${Number(row.championId)}" aria-label="查看${escapeHTML(name)}详情">
      <td class="champion-rank">${rank}</td>
      <td><span class="champion-identity"><span class="champion-portrait"><img src="${imageURL(source, path)}" alt="" loading="lazy" decoding="async" data-champion-image><span>${escapeHTML(name.slice(0, 1))}</span></span><span><strong>${escapeHTML(name)}</strong>${subname ? `<small>${escapeHTML(subname)}</small>` : ""}</span></span></td>
      <td>${tierBadge(row.tier)}</td>
      ${showPosition ? `<td><span class="position-pill">${positionIcon(row.position)}${positionLabel(row.position)}</span></td>` : ""}${metrics ? `<td class="metric-win${Number(row.winRate) < 49.5 ? " is-low" : ""}">${percent(row.winRate)}</td><td class="metric-pick">${percent(row.pickRate)}</td><td class="metric-ban">${percent(row.banRate)}</td><td class="metric-games">${Number(row.play) > 0 ? compactNumber(row.play) : "—"}</td>` : ""}
    </tr>`;
  }

  function renderAugmentCard(item) {
    const champions = (item.champions || []).slice(0, 5);
    return `<article class="augment-card is-${escapeHTML(item.rarity || "unknown")}">
      <header>${assetImage({ source: item.imageSource, path: item.imagePath, name: item.name, description: item.tooltip || item.description }, "augment-icon")}<span><strong>${escapeHTML(item.name)}</strong><small class="rarity-label is-${escapeHTML(item.rarity || "unknown")}">${rarityLabel(item.rarity)}</small></span></header>
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
    const modeLabel = state.mode === "ranked" ? "梯度榜" : state.mode === "arena" ? "斗魂竞技场" : "海克斯大乱斗";
    const arenaStats = detail?.arenaStats || {};
    root.innerHTML = `<button class="champion-back" type="button" data-champion-back><span aria-hidden="true">←</span> 返回${modeLabel}</button>
      <header class="champion-detail-hero">
        <img class="champion-detail-art" src="${heroArtworkURL(meta, source, path)}" alt="" aria-hidden="true" decoding="async" data-champion-image>
        <div class="champion-detail-art-shade" aria-hidden="true"></div>
        <span class="champion-detail-portrait"><img src="${imageURL(source, path)}" alt="${escapeHTML(title)}" decoding="async" data-champion-image><span>${escapeHTML(title.slice(0, 1))}</span></span>
        <div class="champion-detail-title"><p>${state.mode === "ranked" ? `韩服 · ${tierLabel(state.tier)} · ${positionLabel(row.position)}${detail?.sampleTier ? `（该段位样本不足，展示${tierLabel(detail.sampleTier)}数据）` : ""}` : state.mode === "arena" ? "斗魂竞技场 · 全球样本" : "海克斯大乱斗 · 当前样本"}</p><h2>${escapeHTML(title)}</h2><span>${escapeHTML(name)}${detail?.patch ? ` · 版本 ${escapeHTML(detail.patch)}` : ""}</span></div>
        <div class="champion-detail-metrics${state.mode === "aram-mayhem" ? " is-compact" : state.mode === "arena" ? " is-arena" : ""}">${state.mode === "ranked" ? metric("梯度", tierBadge(row.tier, "is-metric")) + metric("胜率", percent(row.winRate)) + metric("选用率", percent(row.pickRate)) + metric("禁用率", percent(row.banRate)) : state.mode === "arena" ? metric("平均名次", number(arenaStats.averagePlacement, 2)) + metric("第一名", percent(arenaStats.firstPlaceRate)) + metric("胜率", percent(arenaStats.winRate || row.winRate)) + metric("选用率", percent(arenaStats.pickRate || row.pickRate)) + metric("禁用率", percent(arenaStats.banRate)) : metric("排名", `#${row.rank || "—"}`) + metric("梯度", tierBadge(row.tier, "is-metric"))}</div>
      </header>
      ${state.loading ? renderDetailSkeleton() : state.error ? renderError(state.error, true) : detail ? renderDetailContent(detail) : renderError("详情暂时不可用", true)}`;
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

  function renderRecommendedAugments(items) {
    const top = items.slice(0, 3);
    const rest = items.slice(3);
    return `<section class="recommendation-section augment-ranking"><header><div><h3>最适合的海克斯</h3><p>前三名优先选择，其余按 OP.GG 当前推荐顺序</p></div><span class="section-count">${items.length} 个</span></header><div class="augment-podium">${top.map((item, index) => `<article class="podium-place is-${index + 1}"><b>${index + 1}</b>${assetImage(item, "augment-icon")}<strong>${escapeHTML(item.name)}</strong><span>${index === 0 ? "首选" : index === 1 ? "次选" : "备选"}</span></article>`).join("")}</div>${rest.length ? `<div class="recommended-augment-list">${rest.map((item, index) => `<div><b>${index + 4}</b>${assetImage(item, "augment-icon")}<strong>${escapeHTML(item.name)}</strong><span>推荐</span></div>`).join("")}</div>` : ""}</section>`;
  }

  function renderArenaDetailTeams(teams) {
    return `<section class="recommendation-section arena-detail-teams"><header><div><h3>推荐三人队伍</h3><p>当前英雄与两名搭档组成完整队伍</p></div><span class="section-count">${teams.length} 组</span></header><div class="arena-detail-team-grid">${teams.map((team, index) => `<article><b>${index + 1}</b>${renderArenaTeamFaces(team.champions, true)}<strong>${teamName(team)}</strong><dl><div><dt>平均名次</dt><dd>${number(team.averagePlacement, 2)}</dd></div><div><dt>第一名</dt><dd>${percent(team.firstPlaceRate)}</dd></div><div><dt>选用率</dt><dd class="metric-pick">${percent(team.pickRate)}</dd></div><div><dt>胜率</dt><dd class="metric-win">${percent(team.winRate)}</dd></div></dl></article>`).join("")}</div></section>`;
  }

  function renderArenaAugments(rows) {
    const top = rows.slice(0, 3);
    const rest = rows.slice(3, 10);
    const podium = top.map((row, index) => {
      const asset = row.assets?.[0] || {};
      return `<article class="podium-place is-${index + 1}"><b>${index + 1}</b>${assetImage(asset, "augment-icon")}<strong>${escapeHTML(asset.name || "海克斯")}</strong><dl><div><dt>选用率</dt><dd class="metric-pick">${percent(row.pickRate)}</dd></div><div><dt>胜率</dt><dd class="metric-win">${percent(row.winRate)}</dd></div></dl></article>`;
    }).join("");
    const list = rest.length ? `<div class="arena-augment-list">${rest.map((row, index) => { const asset = row.assets?.[0] || {}; return `<article><b>${index + 4}</b>${assetImage(asset, "augment-icon")}<strong>${escapeHTML(asset.name || "海克斯")}</strong><span class="metric-pick">${percent(row.pickRate)}</span><span class="metric-win">${percent(row.winRate)}</span></article>`; }).join("")}</div>` : "";
    return `<section class="recommendation-section arena-augment-ranking"><header><div><h3>推荐海克斯</h3><p>优先查看前三项，再按当前选用率浏览其余推荐</p></div><span class="section-count">${rows.length} 个</span></header><div class="augment-podium arena-augment-podium">${podium}</div>${list}</section>`;
  }

  function activeRunePage(pages) {
    return Math.min(Math.max(0, Number(state.runePage) || 0), Math.max(0, pages.length - 1));
  }

  // 设计稿布局：符文方案页签（主系+副系系别图标与胜率场次）+ 单块符文板，
  // 右侧独立呈现召唤师技能与技能加点两张半高卡片。
  function renderRuneWorkspace(pages, build) {
    const active = activeRunePage(pages);
    const tabs = pages.map((page, index) => {
      const keys = `${runeStyleIcon(page.primaryStyle, "rune-tab-style")}${runeStyleIcon(page.subStyle, "rune-tab-substyle")}`;
      return `<button type="button" class="rune-page-tab${index === active ? " is-active" : ""}" data-rune-page="${index}" role="tab" aria-selected="${index === active}" aria-label="第 ${index + 1} 套符文方案">
        <span class="rune-tab-keys">${keys}</span>
        <span class="rune-tab-stats"><b class="metric-win">${percent(page.winRate)}</b><small>${compactNumber(page.games)} 场 · 选用 ${percent(page.pickRate)}</small></span>
      </button>`;
    }).join("");
    const boards = pages.map((page, index) => `<div class="rune-board-panel" data-rune-page-panel="${index}"${index === active ? "" : " hidden"}>${renderRuneTree(page)}</div>`).join("");
    return `<div class="champion-loadout-row">
      <section class="recommendation-section rune-workspace"><header><h3>推荐符文</h3><span class="section-count">${pages.length} 套</span></header><div class="rune-page-tabs" role="tablist" aria-label="符文方案">${tabs}</div>${boards}</section>
      <div class="loadout-side">${renderSpellsCard(build)}${renderSkillsCard(build)}</div>
    </div>`;
  }

  function renderSpellsCard(build) {
    const rows = (build.summonerSpells || []).slice(0, 2);
    const options = rows.map((row, index) => {
      const assets = (row.assets || []).slice(0, 2);
      return `<div class="spell-option${index === 0 ? " is-primary" : ""}">
        <span class="spell-option-icons">${assets.map((asset) => assetImage(asset, "game-icon spell-option-icon", true)).join("")}</span>
        <dl class="spell-option-stats">
          <div><dt>场次</dt><dd>${compactNumber(row.games)}</dd></div>
          <div><dt>选用率</dt><dd class="metric-pick">${percent(row.pickRate)}</dd></div>
          <div><dt>胜率</dt><dd class="metric-win">${percent(row.winRate)}</dd></div>
        </dl>
      </div>`;
    }).join("");
    return `<section class="recommendation-section side-card"><header><h3>召唤师技能</h3></header><div class="spell-options">${options || '<p class="muted side-empty">该模式暂无独立召唤师技能样本</p>'}</div></section>`;
  }

  function renderSkillsCard(build) {
    const skills = (build.skills || [])[0];
    const hasStats = skills && (Number(skills.pickRate) > 0 || Number(skills.winRate) > 0);
    const stats = hasStats ? `<span class="skill-head-stats"><span>选用率 <b class="metric-pick">${percent(skills.pickRate)}</b></span><span>胜率 <b class="metric-win">${percent(skills.winRate)}</b></span></span>` : "";
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
    const starters = (build.starterItems || []).slice(0, 2);
    const boots = (build.boots || []).slice(0, 2);
    const prismItems = (build.prismItems || []).slice(0, 5);
    const routes = buildItemRoutes(build);
    if (!spells.length && !skills && !routes.length && !prismItems.length) return "";
    const spellContent = spells.length ? spells.map((row) => renderConfigOption(row, "spell")).join("") : '<p class="muted">该模式暂无独立召唤师技能样本</p>';
    const itemSections = [];
    if (state.mode !== "arena") itemSections.push(`<section><h3>出门装</h3><div class="config-option-list">${starters.map((row) => renderConfigOption(row, "item")).join("")}</div></section>`);
    itemSections.push(`<section><h3>鞋子</h3><div class="config-option-list">${boots.map((row) => renderConfigOption(row, "item")).join("")}</div></section>`);
    if (state.mode === "arena" && prismItems.length) itemSections.push(`<section><h3>棱彩装备</h3><div class="config-option-list">${prismItems.map((row) => renderConfigOption(row, "item")).join("")}</div></section>`);
    itemSections.push(`<section class="route-options"><h3>出装路线</h3><div class="config-option-list">${routes.map((row) => renderConfigOption(row, "route")).join("")}</div></section>`);
    return `<section class="build-recommendation champion-build-board"><div class="build-essentials"><section><h3>召唤师技能</h3><div class="config-option-list">${spellContent}</div></section><section class="skill-plan"><h3>技能加点</h3>${renderChampionSkillPlan(skills)}</section></div><div class="item-option-groups">${itemSections.join("")}</div></section>`;
  }

  // 设计稿布局：左侧窄栏放出门装与鞋子（图标与数据同一行），右侧每条出装路线独占一行。
  function renderRankedBuild(build) {
    const starters = (build.starterItems || []).slice(0, 2);
    const boots = (build.boots || []).slice(0, 2);
    const routes = buildItemRoutes(build);
    if (!starters.length && !boots.length && !routes.length) return "";
    const sideStats = (row) => `<dl class="side-stats"><div><dt>选用率</dt><dd class="metric-pick">${percent(row.pickRate)}</dd></div><div><dt>胜率</dt><dd class="metric-win">${percent(row.winRate)}</dd></div></dl>`;
    const sideRow = (row) => `<div class="build-side-row"><div class="config-icons">${(row.assets || []).map((asset) => renderAssetButton(asset)).join("")}</div>${sideStats(row)}</div>`;
    const group = (title, rows) => `<div class="build-side-group"><h4>${title}</h4>${rows.map(sideRow).join("") || '<p class="muted">暂无样本</p>'}</div>`;
    const routeRows = routes.map((row) => `<div class="route-row"><div class="config-icons">${(row.assets || []).map((asset, iconIndex) => `<span class="route-step">${iconIndex ? '<span class="route-arrow" aria-hidden="true">›</span>' : ""}${renderAssetButton(asset)}</span>`).join("")}</div>${renderOptionStats(row)}</div>`).join("");
    return `<section class="recommendation-section champion-build-board build-workspace"><header><h3>出装</h3></header><div class="build-split"><div class="build-side">${group("出门装", starters)}${group("鞋子", boots)}</div><div class="build-routes">${routeRows || '<p class="muted">暂无出装路线样本</p>'}</div></div></section>`;
  }

  function renderChampionSkillPlan(row, showStats = true) {
    if (!row) return '<p class="muted">暂无技能加点样本</p>';
    const priority = row.skillPriority || [];
    const icons = row.assets || [];
    const labels = { 0: "主升", 1: "副升", 2: "最后" };
    let priorityHTML = priority.map((key, index) => {
      const asset = icons[index];
      const tooltip = championAbilityTooltip(key, asset);
      return `<button class="skill-icon-button" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${asset ? assetImage(asset, "game-icon recommend-icon", false) : `<span class="skill-letter">${escapeHTML(key)}</span>`}<span>${labels[index] || "技能"}</span></button>`;
    }).join("");
    if (row.ultimate) {
      const tooltip = championAbilityTooltip("R", row.ultimate);
      priorityHTML += `<button class="skill-icon-button is-ultimate" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${assetImage(row.ultimate, "game-icon recommend-icon", false)}<span>大招</span></button>`;
    }
    const order = row.skillOrder || [];
    return `<div class="skill-priority">${priorityHTML}</div><div class="skill-order" style="--skill-count:${Math.max(1, order.length)}" aria-label="技能升级顺序">${order.map((key, index) => `<span class="is-${String(key).toLowerCase()}"><b>${escapeHTML(key)}</b><small>${index + 1}</small></span>`).join("")}</div>${showStats ? renderOptionStats(row) : ""}`;
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
    const cores = (build.coreItems || []).slice(0, 3);
    const late = [build.fourthItems || [], build.fifthItems || [], build.sixthItems || []];
    const routeLimit = String(state.selected?.position || state.position).toLowerCase() === "adc" ? 7 : 6;
    return cores.map((core, index) => {
      const seen = new Set();
      const assets = [];
      const addAsset = (asset) => {
        if (!asset?.path || seen.has(asset.path) || assets.length >= routeLimit) return false;
        seen.add(asset.path);
        assets.push(asset);
        return true;
      };
      for (const asset of core.assets || []) addAsset(asset);
      for (const choices of late) {
        for (let offset = 0; offset < choices.length; offset += 1) {
          const row = choices[(index + offset) % choices.length];
          if ((row?.assets || []).some(addAsset)) break;
        }
      }
      for (const choices of late) for (const row of choices) for (const asset of row.assets || []) addAsset(asset);
      return { ...core, assets };
    }).filter((row) => row.assets.length);
  }

  function renderConfigOption(row, kind) {
    const icons = (row.assets || []).map((asset, index) => kind === "route"
      ? `<span class="route-step">${index ? '<span class="route-arrow" aria-hidden="true">›</span>' : ""}${renderAssetButton(asset)}</span>`
      : renderAssetButton(asset)).join("");
    return `<article class="config-option"><div class="config-icons">${icons}</div>${renderOptionStats(row)}</article>`;
  }

  function renderAssetButton(asset) {
    const name = asset?.name || "装备";
    const tooltip = assetTooltip(asset, name);
    return `<button class="item-option-button" type="button" aria-label="${escapeHTML(tooltip.replace(/\n/g, "，"))}" data-tooltip="${escapeHTML(tooltip)}">${assetImage(asset, "game-icon recommend-icon", false)}</button>`;
  }

  function renderOptionStats(row) {
    const hasPick = Number.isFinite(Number(row?.pickRate)) && Number(row.pickRate) > 0;
    const hasWin = Number.isFinite(Number(row?.winRate)) && Number(row.winRate) > 0;
    if (!hasPick && !hasWin) return "";
    return `<dl class="option-stats"><div class="is-pick"><dt>选用率</dt><dd>${percent(row?.pickRate)}</dd></div><div class="is-win"><dt>胜率</dt><dd>${percent(row?.winRate)}</dd></div></dl>`;
  }

  // 设计稿布局：优势对抗、劣势对抗、场次最多的玩家三张卡片放在一行。
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
    return `<section class="recommendation-section counter-group is-${kind}"><header><div><h4>${title}</h4><p>${copy}</p></div><span class="section-count">${escapeHTML(sampleNote)}</span></header><div>${rows.map((row) => {
      const width = Math.min(100, Math.max(4, Number(row.winRate) || 0));
      return `<button type="button" class="counter-champion" data-counter-champion="${escapeHTML(row.key)}"><span class="champion-portrait"><img src="${imageURL(row.imageSource, row.imagePath)}" alt="" loading="lazy" decoding="async" data-champion-image><span>${escapeHTML((row.name || "?").slice(0, 1))}</span></span><span><strong>${escapeHTML(row.name || row.key)}</strong><small>${compactNumber(row.games)} 场</small></span><span class="counter-meter" aria-hidden="true"><i style="width:${width}%"></i></span><b>${percent(row.winRate)}</b><i aria-hidden="true">›</i></button>`;
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
        <span class="player-avatar">${player.iconPath ? `<img src="${imageURL(player.iconSource, player.iconPath)}" alt="" loading="lazy" decoding="async">` : ""}</span>
        <span class="player-copy"><strong>${escapeHTML(player.name)}${player.tagline ? ` <small>#${escapeHTML(player.tagline)}</small>` : ""}</strong><small>${crest}${escapeHTML(tierText || "段位未知")}</small></span>
        <span class="player-games"><b class="metric-win">${percent(player.winRate)}</b><small>${escapeHTML(player.games || "—")} 场</small></span>
      </button>`;
    }).join("");
    return `<section class="recommendation-section counter-group is-players"><header><div><h4>场次最多的玩家</h4><p>韩服 · 钻二以上样本</p></div><span class="section-count">前 ${visible.length} 名</span></header><div>${rows}</div></section>`;
  }

  function metric(label, value) { const className = label === "胜率" ? "metric-win" : label === "选用率" ? "metric-pick" : label === "禁用率" ? "metric-ban" : ""; return `<div class="${className}"><span>${label}</span><strong>${value}</strong></div>`; }
  function augmentTierLabel(value) { return ({ 0: "S", 1: "A", 2: "B", 3: "C", 4: "D", 5: "E" })[Number(value)] || "—"; }
  function rarityLabel(value) { return ({ silver: "白银", gold: "黄金", prismatic: "棱彩" })[value] || "未分类"; }

  function filteredChampionRows() {
    const rows = state.rankings?.rows || [];
    const query = normalizeSearch(state.query);
    if (!query) return rows;
    const scored = rows.map((row, order) => {
      const meta = championMeta(row.championId);
      const values = [row.name, row.key, meta?.nameZh, meta?.titleZh, meta?.nameEn, meta?.titleEn, ...(meta?.searchTerms || [])].map(normalizeSearch).filter(Boolean);
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

  function filteredAugments() {
    const query = normalizeSearch(state.augmentQuery);
    const rarityOrder = { prismatic: 0, gold: 1, silver: 2, unknown: 3 };
    return (state.augments?.rows || []).filter((item) => (state.augmentRarity === "all" || item.rarity === state.augmentRarity) && (!query || normalizeSearch(`${item.name}${item.description}${item.tooltip}${item.key}`).includes(query))).sort((a, b) => Number(a.tier) - Number(b.tier) || (rarityOrder[a.rarity] ?? 3) - (rarityOrder[b.rarity] ?? 3) || String(a.name).localeCompare(String(b.name), "zh-CN"));
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
  function prepareImages() {
    for (const image of root.querySelectorAll("[data-champion-image]")) {
      const loaded = () => image.parentElement?.classList.add("has-loaded-image");
      const failed = () => { image.hidden = true; image.parentElement?.classList.remove("has-loaded-image"); };
      if (image.complete) image.naturalWidth > 0 ? loaded() : failed();
      else {
        image.addEventListener("load", loaded, { once: true });
        image.addEventListener("error", failed, { once: true });
      }
    }
  }

  function resetTransientChampionState({ restorePosition = false } = {}) {
    state.query = "";
    state.augmentQuery = "";
    state.augmentRarity = "all";
    state.detail = null;
    state.selected = null;
    state.error = "";
    state.listScroll = 0;
    state.runePage = 0;
    if (restorePosition) state.position = normalizePosition(readSetting("champion-position", "all"));
  }

  root.addEventListener("click", (event) => {
    const playerRow = event.target.closest("[data-player-name]");
    if (playerRow) {
      // 玩家榜数据来自 OP.GG 韩服，点击后在当前页面以覆盖层展示韩服总览。
      window.dispatchEvent(new CustomEvent("deep-legends:open-player", { detail: { gameName: playerRow.dataset.playerName, tagLine: playerRow.dataset.playerTag || "", region: "kr", source: "champions" } }));
      return;
    }
    const runeTab = event.target.closest("[data-rune-page]");
    if (runeTab) {
      const index = Number(runeTab.dataset.runePage) || 0;
      state.runePage = index;
      for (const tab of root.querySelectorAll("[data-rune-page]")) {
        const active = Number(tab.dataset.runePage) === index;
        tab.classList.toggle("is-active", active);
        tab.setAttribute("aria-selected", String(active));
      }
      for (const panel of root.querySelectorAll("[data-rune-page-panel]")) panel.hidden = Number(panel.dataset.runePagePanel) !== index;
      return;
    }
    const mode = event.target.closest("[data-champion-mode]");
    if (mode) {
      state.mode = normalizeMode(mode.dataset.championMode);
      resetTransientChampionState({ restorePosition: true });
      state.rankings = null; state.augments = null;
      loadWorkspace(true);
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
    const counter = event.target.closest("[data-counter-champion]");
    if (counter) { openCounterDetail(counter.dataset.counterChampion); return; }
    const rowElement = event.target.closest("[data-champion-row]");
    if (rowElement) {
      const row = (state.rankings?.rows || []).find((item) => Number(item.championId) === Number(rowElement.dataset.championRow));
      if (row) openDetail(row);
      return;
    }
    if (event.target.closest("[data-champion-back]")) { closeDetail(); return; }
    if (event.target.closest("[data-champion-refresh]")) { state.rankings = null; state.augments = null; loadWorkspace(true); return; }
    if (event.target.closest("[data-champion-retry]")) { state.selected ? openDetail(state.selected) : loadWorkspace(true); return; }
    if (event.target.closest("[data-champion-clear]")) { state.query = ""; state.augmentQuery = ""; state.augmentRarity = "all"; render(); }
  });

  root.addEventListener("change", (event) => {
    if (event.target.matches("[data-champion-tier]")) {
      state.tier = normalizeTier(event.target.value);
      loadRankings();
    }
  });

  root.addEventListener("input", (event) => {
    if (searchComposing || event.isComposing) return;
    if (event.target.matches("[data-champion-search]")) updateChampionSearch(event.target);
    if (event.target.matches("[data-augment-search]")) preserveSearchInput(event.target, "[data-augment-search]", () => { state.augmentQuery = event.target.value; });
  });

  root.addEventListener("compositionstart", () => { searchComposing = true; });
  root.addEventListener("compositionend", (event) => {
    searchComposing = false;
    if (event.target.matches("[data-champion-search]")) updateChampionSearch(event.target);
    if (event.target.matches("[data-augment-search]")) preserveSearchInput(event.target, "[data-augment-search]", () => { state.augmentQuery = event.target.value; });
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

  function updateChampionSearch(input) {
    const start = input.selectionStart ?? input.value.length;
    const end = input.selectionEnd ?? start;
    state.query = input.value;
    if (state.mode === "ranked" && normalizeSearch(state.query) && state.position !== "all") {
      state.position = "all";
      Promise.resolve(loadRankings()).finally(() => {
        const next = root.querySelector("[data-champion-search]");
        next?.focus({ preventScroll: true });
        next?.setSelectionRange(Math.min(start, next.value.length), Math.min(end, next.value.length));
      });
      return;
    }
    preserveSearchInput(input, "[data-champion-search]", () => {});
  }

  root.addEventListener("keydown", (event) => {
    const row = event.target.closest("[data-champion-row]");
    if (row && (event.key === "Enter" || event.key === " ")) { event.preventDefault(); row.click(); }
    if (event.key === "Escape" && state.selected) { event.preventDefault(); closeDetail(); }
  });

  window.addEventListener("deep-legends:section", (event) => {
    const previous = state.section;
    state.section = event.detail?.name || "overview";
    if (previous === "champions" && state.section !== "champions") {
      // 查看玩家详情的临时跳转保留英雄详情现场，返回时原样恢复。
      if (state.playerDetour) return;
      state.requests.get("detail")?.abort();
      // 离开英雄页即还原全部筛选项：模式、段位、位置、搜索词与详情现场。
      resetTransientChampionState({ restorePosition: true });
      state.mode = "ranked";
      state.tier = "emerald_plus";
      return;
    }
    if (state.section === "champions" && previous !== "champions") {
      if (state.playerDetour) {
        state.playerDetour = false;
        render();
        return;
      }
      enterChampionSection();
    }
  });

  if (settingPosition) {
    settingPosition.value = state.position;
    settingPosition.addEventListener("change", () => {
      state.position = normalizePosition(settingPosition.value);
      writeSetting("champion-position", state.position);
      if (state.section === "champions" && state.mode === "ranked" && !state.selected) loadRankings();
    });
  }

  render();
  beginStartupPreload();
})();
