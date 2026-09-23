(() => {
  "use strict";
  const panel = document.getElementById("pro-players-panel");
  if (!panel) return;
  const content = document.getElementById("pro-players-content");
  const status = document.getElementById("pro-players-status");
  const filters = document.getElementById("pro-team-filters");
  const search = document.getElementById("pro-players-search");
  const refresh = document.getElementById("pro-players-refresh");
  const home = document.getElementById("pro-players-home");
  const back = document.getElementById("pro-players-return");
  const primaryOnly = document.getElementById("pro-primary-only");
  const teams = ["BLG", "IG", "T1", "HLE", "GEN", "DK"];
  const positions = { top: "上单", jungle: "打野", middle: "中单", bottom: "下路", utility: "辅助" };
  const tiers = { IRON: "黑铁", BRONZE: "青铜", SILVER: "白银", GOLD: "黄金", PLATINUM: "铂金", EMERALD: "翡翠", DIAMOND: "钻石", MASTER: "超凡大师", GRANDMASTER: "傲世宗师", CHALLENGER: "最强王者" };
  const state = { data: null, team: "all", query: "", loading: false, error: "", loadedAt: 0, returnKey: "", primaryOnly: false, expanded: new Set(), mismatches: new Set() };
  let links = new Map();
	let visible = false, pollTimer = null;
  const escape = (value) => String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]);
  const identityKey = (value) => String(value || "").toLocaleLowerCase();
  const normalize = (value) => String(value || "").toLocaleLowerCase().replace(/\s+/g, "");
  const navigate = (section) => window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section } }));
  const integer = (value) => new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(Number(value));
  const stamp = (value) => {
    const date = new Date(value);
    if (!value || !Number.isFinite(date.getTime()) || date.getFullYear() < 2000) return "更新时间未知";
    return new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).format(date);
  };
  filters.innerHTML = [["all", "全部战队"], ...teams.map((team) => [team, team])].map(([value, label]) => `<button type="button" class="pro-team-filter" data-pro-team="${value}" aria-pressed="${value === state.team}">${label}</button>`).join("");

  function rank(account) {
    if (account.rankStatus !== "ranked" || !tiers[account.tier]) return `<span class="pro-muted">${account.rankStatus === "unranked" ? "未定级" : "段位暂不可用"}</span>`;
    const division = ["MASTER", "GRANDMASTER", "CHALLENGER"].includes(account.tier) ? "" : ` ${["", "I", "II", "III", "IV"][account.division] || ""}`;
    return `<span class="pro-rank"><img src="/rank-crests/${account.tier.toLowerCase()}.png" alt="" loading="lazy">${tiers[account.tier]}${division}</span>`;
  }
  function ladderRank(account) {
    const value = Number(account.ladderRank);
    if (account.rankStatus !== "ranked" || account.ladderRankKnown !== true || !Number.isSafeInteger(value) || value <= 0) return '<span class="pro-muted">—</span>';
    const formatted = integer(value);
    return `<span class="pro-ladder-rank" title="OP.GG 韩服天梯排名" aria-label="韩服天梯排名第 ${escape(formatted)} 名">#${escape(formatted)}</span>`;
  }
  function playerCell(player, count, shown) {
    const position = positions[player.position] || "位置未知";
    const icon = positions[player.position] ? `<img src="/position-icons/${player.position}.svg" alt="">` : "";
    const issue = player.status === "partial" ? `<span class="pro-pending">部分记录待核验</span>` : "";
    return `<th class="pro-player" scope="rowgroup" rowspan="${Math.max(1, shown)}"><div class="pro-player-name">${icon}<span>${escape(player.name)}</span></div><div class="pro-player-meta">${position}<span aria-hidden="true">·</span>${count ? `${count} 个账号` : "暂无账号"}</div>${issue}</th>`;
  }
  function render() {
    content.setAttribute("aria-busy", String(state.loading));
    refresh.disabled = state.loading;
    refresh.textContent = state.loading ? "正在读取…" : "↻ 刷新账号";
    for (const button of filters.querySelectorAll("[data-pro-team]")) button.setAttribute("aria-pressed", String(button.dataset.proTeam === state.team));
    const data = state.data;
    if (!data) {
      status.classList.toggle("is-warning", Boolean(state.error));
      status.textContent = state.error || "正在读取六支战队的公开账号…";
      content.innerHTML = state.error ? '<div class="pro-empty"><strong>暂时无法读取账号</strong><p>请检查网络后点击“刷新账号”重试。一队名单不会被空结果替换。</p></div>' : '<div class="pro-skeleton" aria-label="正在加载职业选手"><span></span><span></span><span></span><span></span></div>';
      return;
    }
    status.classList.toggle("is-warning", Boolean(state.error || data.stale || data.partial || data.unavailable));
    status.innerHTML = `<div class="pro-status-line"><span>${state.error ? escape(state.error) : state.loading ? "正在刷新…" : data.unavailable ? "来源暂不可用" : `目录读取 ${escape(stamp(data.fetchedAt))}${data.stale ? " · 缓存已过期" : ""}${data.updating ? " · 正在补充账号与排名" : data.partial ? " · 部分来源未更新" : ""}`}</span><span>名单核对 ${escape(data.rosterVerifiedAt || "未知")}${data.rosterStale ? " · 待更新" : ""}</span></div>`;
    links = new Map();
    const query = normalize(state.query);
    const html = [];
    for (const team of data.teams) {
      if (!teams.includes(team.code) || team.secondary === true || state.team !== "all" && state.team !== team.code) continue;
      const playerGroups = [];
      let shownAccounts = 0;
      for (const player of team.players || []) {
        const accounts = Array.isArray(player.accounts) ? player.accounts : [];
        const playerMatches = normalize(`${team.code} ${team.name} ${player.name} ${positions[player.position] || ""}`).includes(query);
        const visibleAccounts = playerMatches ? accounts : accounts.filter((account) => normalize(account.gameName + "#" + account.tagLine).includes(query));
        if (!playerMatches && !visibleAccounts.length) continue;
        shownAccounts += visibleAccounts.length;
        const collapsed = account => account.rankStatus !== "ranked";
        const history = visibleAccounts.filter(collapsed);
        const expanded = state.expanded.has(player.key);
        const shown = state.primaryOnly ? visibleAccounts.slice(0, 1) : visibleAccounts.filter(account => !collapsed(account));
        const rows = [];
        const accountRow = (account) => {
          const key = `${player.key}:${account.gameName}#${account.tagLine}`;
          links.set(key, { account, teamCode: team.code, playerName: player.name, secondary: team.secondary === true });
          const name = `${account.gameName}#${account.tagLine}`;
          const lp = account.rankStatus === "ranked" && account.lpKnown !== false && Number.isFinite(Number(account.lp)) ? `<span class="pro-lp-value">${escape(integer(account.lp))}</span>` : '<span class="pro-muted">—</span>';
          const updated = account.lastMatchAtKnown && account.lastMatchAt ? `最近对局 ${stamp(account.lastMatchAt)}` : (data.updating ? "正在读取最近对局…" : "最近对局时间暂不可用");
          return `<tr class="${accounts.indexOf(account) === 0 ? "pro-primary" : ""}" data-pro-row="${escape(key)}"><td class="pro-account"><button type="button" data-pro-account="${escape(key)}" aria-label="查看 ${escape(player.name)} 的账号 ${escape(name)} 总览"><span class="pro-account-line"><span class="pro-account-name" title="${escape(name)}">${escape(account.gameName)}<small>#${escape(account.tagLine)}</small></span>${account.stale ? '<small class="pro-cached">缓存</small>' : ""}${account.inactive ? '<small class="pro-inactive">不活跃</small>' : ""}${account.source === "TrackingThePros" ? '<small class="pro-ttp">TTP</small>' : ""}${state.mismatches.has(identityKey(name)) ? '<small class="pro-review">待核验</small>' : ""}</span><span class="pro-account-meta">${escape(updated)}</span></button></td><td class="pro-rank-cell">${rank(account)}<span class="pro-ladder-inline">${ladderRank(account)}</span></td><td class="pro-lp">${lp}</td><td class="pro-ladder">${ladderRank(account)}</td></tr>`;
        };
        rows.push(...shown.map(accountRow));
        if (!state.primaryOnly && history.length) {
          rows.push(`<tr class="pro-history"><td colspan="4"><button type="button" title="展开查看未定级和段位暂不可用的账号；读取失败不会被判为未定级" data-pro-history="${escape(player.key)}" aria-expanded="${expanded}">${expanded ? "▾" : "▸"} ${history.length} 个无段位或待确认账号</button></td></tr>`);
          if (expanded) rows.push(...history.map(accountRow));
        }
        if (!rows.length) {
          const message = state.primaryOnly && accounts.length ? "暂无匹配的最新账号" : data.updating ? "正在读取选手账号…" : data.unavailable ? "账号来源暂不可用，稍后重试" : player.status === "partial" ? "账号来源暂不可用或记录待核验，请刷新重试" : "公开来源暂未收录可核验的韩服账号";
          rows.push(`<tr><td colspan="4" class="pro-missing">${message}</td></tr>`);
        }
        rows[0] = rows[0].replace(/(<tr[^>]*>)/, "$1" + playerCell(player, accounts.length, rows.length));
        playerGroups.push(`<tbody>${rows.join("")}</tbody>`);
      }
      if (!playerGroups.length) continue;
      html.push(`<section class="pro-team" aria-labelledby="pro-team-${escape(team.code)}"><header class="pro-team-heading"><div><span class="pro-league">${escape(team.league)}</span><h2 id="pro-team-${escape(team.code)}">${escape(team.code)}</h2><span class="pro-team-name">${escape(team.name)}${team.secondary ? " · 二队 / 学院队" : ""}</span></div><span>${playerGroups.length} 位选手 · ${shownAccounts} 个账号</span></header><div class="pro-table-wrap"><table class="pro-table"><caption class="sr-only">${escape(team.name)} ${team.secondary ? "二队" : "一队"}选手账号及韩服单双排段位、胜点和天梯排名</caption><colgroup><col class="pro-col-player"><col class="pro-col-account"><col class="pro-col-rank"><col class="pro-col-lp"><col class="pro-col-ladder"></colgroup><thead><tr><th scope="col">选手</th><th scope="col">韩服账号</th><th scope="col">单双排段位</th><th scope="col" class="pro-lp">胜点</th><th scope="col" class="pro-ladder">天梯</th></tr></thead>${playerGroups.join("")}</table></div></section>`);
    }
    content.innerHTML = html.join("") || '<div class="pro-empty"><strong>没有匹配的选手或账号</strong><p>试试其他关键词，或切换到“全部战队”。</p></div>';
  }

  async function load(force = false, poll = false) {
    if (state.loading) return;
    if (!force && !poll && state.data && Date.now() - state.loadedAt < (state.data.unavailable || state.data.stale || state.data.partial ? 30000 : 300000) && !state.data.updating) return;
    clearTimeout(pollTimer);
    state.loading = true;
    state.error = "";
    let changed = !poll;
    if (!poll) render();
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 60000);
    try {
      const response = await fetch(`/api/pro-players${force ? "?refresh=1" : ""}`, { credentials: "same-origin", cache: "no-store", signal: controller.signal });
      if (!response.ok) throw new Error("source unavailable");
      const data = await response.json();
      if (!Array.isArray(data.teams) || !data.teams.every((team) => typeof team.code === "string" && team.code && Array.isArray(team.players))) throw new Error("invalid response");
      changed ||= JSON.stringify(state.data) !== JSON.stringify(data);
      state.data = data;
      state.loadedAt = Date.now();
    } catch (error) {
      changed = true;
      state.error = state.data ? "刷新失败，当前显示上次读取的数据，不代表最新账号、段位和排名。" : "账号与排名读取失败，请检查网络后重试。";
      window.reportFlowDiagnostic?.("local_request_client", "failed", { endpoint: "pro-players", httpStatus: Number(error?.status || 0), errorKind: error?.name === "AbortError" ? "timeout" : error?.errorKind || (error?.status ? "http" : "network") });
    } finally {
      clearTimeout(timeout);
      state.loading = false;
      if (changed) render();
      if (visible && state.data?.updating && !state.error) pollTimer = setTimeout(() => void load(false, true), 1500);
    }
  }
  filters.addEventListener("click", (event) => {
    const button = event.target.closest("[data-pro-team]");
    if (!button) return;
    state.team = button.dataset.proTeam;
    render();
  });
  search.addEventListener("input", () => { state.query = search.value; render(); });
  refresh.addEventListener("click", () => { void load(true); });
  primaryOnly.addEventListener("click", () => {
    state.primaryOnly = !state.primaryOnly;
    primaryOnly.setAttribute("aria-checked", String(state.primaryOnly));
    render();
  });
  window.addEventListener("deep-legends:pro-verification", event => {
    if (!event.detail?.mismatch) return;
    state.mismatches.add(identityKey(event.detail.gameName + "#" + event.detail.tagLine));
    render();
  });
  content.addEventListener("click", (event) => {
    const history = event.target.closest("[data-pro-history]");
    if (history) {
      const key = history.dataset.proHistory;
      state.expanded.has(key) ? state.expanded.delete(key) : state.expanded.add(key);
      render();
      [...content.querySelectorAll("[data-pro-history]")].find(button => button.dataset.proHistory === key)?.focus({ preventScroll: true });
      return;
    }
    const button = event.target.closest("[data-pro-account]") || event.target.closest("[data-pro-row]")?.querySelector("[data-pro-account]");
    const target = button && links.get(button.dataset.proAccount);
    if (!target) return;
    const { account, teamCode, playerName, secondary } = target;
    state.returnKey = button.dataset.proAccount;
    window.dispatchEvent(new CustomEvent("deep-legends:open-player", { detail: { gameName: account.gameName, tagLine: account.tagLine, region: "kr", serverId: "", source: "pro-players", expectedTier: account.tier || "", teamCode, playerName, secondary } }));
  });
  home.addEventListener("click", () => navigate("overview"));
  back.addEventListener("click", () => {
    const previous = [...content.querySelectorAll("[data-pro-account]")].find((button) => button.dataset.proAccount === state.returnKey);
    (previous || document.getElementById("pro-players-title"))?.focus({ preventScroll: true });
  });
  function handleLazySection(event) {
    visible = event.detail?.name === "pro-players";
    clearTimeout(pollTimer);
    if (visible) void load();
  }
  window.addEventListener("deep-legends:section", handleLazySection);
  window.deepLegendsSections?.register("pro-players", handleLazySection);
})();
