(() => {
  "use strict";

  const panel = document.getElementById("ladder-panel");
  if (!panel) return;
  const byId = (id) => document.getElementById(id);
  const tbody = byId("ladder-rows");
  const tableScroll = byId("ladder-table-scroll");
  const scrollRoot = byId("app-scroll");
  const footer = byId("ladder-footer");
  const pagination = byId("ladder-pages");
  const refresh = byId("ladder-refresh");
  const proOnly = byId("ladder-pro-only");
  const search = byId("ladder-find");
  const input = search.querySelector("input");
  const searchStatus = search.querySelector("[role='status']");
  const skeletons = [...tbody.children].map((row) => row.cloneNode(true));
  const tierNames = { challenger: "王者", grandmaster: "宗师", master: "大师", diamond: "钻石", emerald: "翡翠", platinum: "铂金", gold: "黄金", silver: "白银", bronze: "青铜", iron: "黑铁" };
  const cache = new Map();
  const inflight = new Map();
  const rendered = new Map();
  const freshFor = 10 * 60_000;
  const maxPage = 100;
  let page = 1, tier = "", onlyPro = false, visible = false;
  let currentData, updateTimer, prefetchFlight, idleWork, idleTimer, hover, foundPage;
  let highlighted = "", returnKey = "", loadedKey = "", displayedPagination = "";
  let revision = 0, searchRevision = 0, globalTotal = null, destroyed = false;

  const escape = (value) => String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]);
  const number = (value) => new Intl.NumberFormat("zh-CN").format(Number(value) || 0);
  const key = (page, tier) => `${page}:${tier}`;
  const rowKey = (row) => `${row.gameName}#${row.tagLine}`.toLowerCase();
  const age = (record) => Date.now() - Date.parse(record?.fetchedAt);
  const isMasked = () => window.deepLegendsPrivacy?.masked() === true;
  const rowButton = (identity) => rendered.get(identity)?.element.querySelector("button");
  const maxPages = (count) => Math.min(maxPage, Math.max(1, Math.ceil(count / 100)));

  async function request(url, options) {
    const response = await fetch(url, { credentials: "same-origin", cache: "no-store", ...options });
    if (!response.ok) throw new Error(String(response.status));
    return response.status === 204 ? null : response.json();
  }

  function requestPage(targetPage, targetTier, force = false, prefetch = false) {
    const id = key(targetPage, targetTier);
    if (inflight.has(id)) return inflight.get(id);
    const params = new URLSearchParams({ page: String(targetPage), tier: targetTier });
    if (force) params.set("refresh", "1");
    if (prefetch) params.set("prefetch", "1");
    const flight = request(`/api/ladder?${params}`).then((data) => {
      if (prefetch && !data) return null;
      if (!data || !Array.isArray(data.rows)) throw new Error("ladder unavailable");
      cache.delete(id);
      cache.set(id, data);
      if (cache.size > 24) cache.delete(cache.keys().next().value);
      if (!targetTier) globalTotal = data.totalSummoners;
      return data;
    }).finally(() => inflight.delete(id));
    inflight.set(id, flight);
    return flight;
  }

  function prefetch(targetPage, targetTier) {
    const data = cache.get(key(targetPage, targetTier));
    if (destroyed || prefetchFlight || inflight.has(key(targetPage, targetTier)) || age(data) < freshFor) return;
    prefetchFlight = requestPage(targetPage, targetTier, false, true).catch(() => {}).finally(() => { prefetchFlight = null; });
  }

  function cancelIdle() {
    clearTimeout(idleTimer);
    if (idleWork != null) window.cancelIdleCallback?.(idleWork);
    idleWork = idleTimer = null;
  }

  function scheduleNext(data) {
    cancelIdle();
    if (page >= maxPages(data.totalSummoners)) return;
    const next = page + 1, currentTier = tier, currentRevision = revision;
    const run = () => {
      idleWork = idleTimer = null;
      if (visible && currentRevision === revision) prefetch(next, currentTier);
    };
    if (window.requestIdleCallback) idleWork = window.requestIdleCallback(run, { timeout: 2000 });
    else idleTimer = setTimeout(run, 2000);
  }

  function renderTime() {
    const time = byId("ladder-time");
    byId("ladder-update").hidden = !currentData?.fetchedAt;
    if (!currentData?.fetchedAt) return;
    time.dateTime = currentData.fetchedAt;
    const stale = currentData.stale === true;
    time.classList.toggle("is-stale", stale);
    time.textContent = `${Math.max(0, Math.floor(age(currentData) / 60_000))} 分钟前${stale ? " · 暂时无法更新" : ""}`;
  }

  function renderCards(data) {
    for (const [id, lp, count] of [
      ["ladder-challenger", data?.challengerCutoffLP, data?.challengerCount],
      ["ladder-grandmaster", data?.grandmasterCutoffLP, data?.grandmasterCount],
    ]) {
      byId(id).innerHTML = count ? `${number(lp)} LP <small>· ${number(count)} 名</small>` : "—";
    }
    byId("ladder-total").textContent = globalTotal == null ? "—" : number(globalTotal);
  }

  function image(kind, id, label = "") {
    const filename = kind === "profile" ? `profile-icons/${Number(id)}.jpg` : `champion-icons/${Number(id)}.png`;
    const src = `/api/image?path=${encodeURIComponent(`/lol-game-data/assets/v1/${filename}`)}`;
    return `<img ${kind === "profile" ? 'class="ladder-avatar"' : ''} data-queued-src="${src}" alt="${escape(label)}" loading="lazy" decoding="async"${label ? ` data-tooltip="${escape(label)}"` : ""}>`;
  }

  function makeRow(row, masked) {
    const element = document.createElement("tr");
    const isPro = row.proMark === true && !masked;
    const identity = rowKey(row);
    const name = masked ? window.deepLegendsPrivacy.name(row, { index: Math.max(0, row.rank - 1), context: "ladder" }) : row.gameName;
    const mark = isPro ? '<span class="pro-mark" role="img" aria-label="职业选手" data-tooltip="职业选手">P</span>' : "";
    const team = isPro && row.proPlayer ? `<span class="pro-tag"><b>${escape(row.proPlayer.team)}</b>${escape(row.proPlayer.name)}</span>` : "";
    const tag = masked ? "" : `<span class="ladder-tagline">#${escape(row.tagLine)}</span>`;
    const avatar = masked ? '<span class="ladder-avatar" aria-hidden="true"></span>' : image("profile", row.profileIconId);
    const tierLabel = tierNames[row.tier];
    const division = row.division ? ` ${number(row.division)}` : "";
    const tierIcon = tierLabel ? `<img src="/rank-crests/${row.tier}.png" alt="" loading="lazy">` : "";
    const record = Number(row.wins) + Number(row.losses);
    const share = record > 0 ? Math.round(100 * Number(row.wins) / record) : 0;
    element.dataset.ladderRow = "";
    element.innerHTML = `
      <td class="ladder-rank${row.rank <= 3 ? " is-top" : ""}">${number(row.rank)}</td>
      <td><button class="ladder-account" type="button">${avatar}${mark}<span class="${isPro ? "pro-name" : "ladder-account-name"}" data-tooltip="${escape(name)}" data-tooltip-overflow="self">${escape(name)}</span>${tag}${team}</button></td>
      <td><div class="ladder-tier">${tierIcon}${tierLabel || "—"}${division}</div></td>
      <td class="ladder-lp"><strong>${number(row.lp)}</strong></td>
      <td><div class="ladder-champions">${(row.champions || []).map((champion) => image("champion", champion.championId, champion.name)).join("")}</div></td>
      <td>${number(row.level)}</td>
      <td><div class="ladder-record"><span class="ladder-bar"><progress max="100" value="${share}" aria-hidden="true"></progress><span>${number(row.wins)}胜</span><span>${number(row.losses)}负</span></span><strong class="ladder-winrate${row.winRate >= 55 ? " is-high" : ""}">${number(row.winRate)}%</strong></div></td>`;
    element.ladderIdentity = identity;
    return element;
  }

  function renderRows(data) {
    const rows = onlyPro ? data.rows.filter((row) => row.proMark === true) : data.rows;
    const masked = isMasked();
    const keep = new Set(rows.map(rowKey));
    const focusKey = document.activeElement?.closest("[data-ladder-row]")?.ladderIdentity;
    for (const [identity, old] of rendered) {
      if (!keep.has(identity)) {
        old.element.remove();
        rendered.delete(identity);
      }
    }
    tbody.querySelectorAll(".ladder-skeleton, .ladder-empty-row").forEach((node) => node.remove());
    let cursor = tbody.firstElementChild;
    for (const row of rows) {
      const identity = rowKey(row);
      const fingerprint = JSON.stringify(row);
      let item = rendered.get(identity);
      if (!item || item.fingerprint !== fingerprint || item.masked !== masked) {
        const element = makeRow(row, masked);
        if (item?.element === cursor) cursor = element;
        item?.element.replaceWith(element);
        item = { row, element, fingerprint, masked };
        rendered.set(identity, item);
      }
      const element = item.element;
      if (element !== cursor || !element.parentNode) tbody.insertBefore(element, cursor?.parentNode ? cursor : null);
      cursor = element.nextElementSibling;
      element.classList.toggle("is-highlighted", identity === highlighted);
    }
    if (!rows.length) {
      tbody.innerHTML = `<tr class="ladder-empty-row"><td colspan="7">${onlyPro ? "本页暂无职业选手" : "暂无排名"}</td></tr>`;
    }
    if (focusKey && rowButton(focusKey) !== document.activeElement) rowButton(focusKey)?.focus({ preventScroll: true });
  }

  function renderFooter(data) {
    footer.hidden = false;
    const first = data.rows[0]?.rank;
    const last = data.rows.at(-1)?.rank;
    byId("ladder-range").textContent = first ? `#${number(first)} ~ #${number(last)}` : "—";
    const count = maxPages(data.totalSummoners);
    const signature = `${page}:${count}`;
    if (displayedPagination === signature) return;
    displayedPagination = signature;
    if (page > maxPage) {
      pagination.innerHTML = `<button type="button" data-ladder-page="${maxPage}">返回第 ${maxPage} 页</button>`;
      return;
    }
    const start = Math.floor((page - 1) / 10) * 10 + 1;
    const buttons = Array.from({ length: Math.max(0, Math.min(10, count - start + 1)) }, (_, index) => start + index);
    pagination.innerHTML = `<button type="button" data-ladder-page="${page - 1}" ${page <= 1 ? "disabled" : ""}>上一页</button>` +
      buttons.map((n) => `<button type="button" data-ladder-page="${n}" ${n === page ? 'aria-current="page"' : ""}>${n}</button>`).join("") +
      `<button type="button" data-ladder-page="${page + 1}" ${page >= count ? "disabled" : ""}>下一页</button>`;
  }

  function render(data) {
    currentData = data;
    tbody.setAttribute("aria-busy", "false");
    tableScroll.hidden = false;
    byId("ladder-failed").hidden = true;
    renderCards(data);
    renderTime();
    renderRows(data);
    renderFooter(data);
  }

  function pending() {
    tbody.setAttribute("aria-busy", "true");
    if (!currentData) {
      rendered.clear();
      tbody.replaceChildren(...skeletons.map((row) => row.cloneNode(true)));
      footer.hidden = true;
      byId("ladder-failed").hidden = true;
      tableScroll.hidden = false;
      renderTime();
    }
  }

  async function showPage({ force = false, revalidate = false } = {}) {
    if (!visible) return;
    if (foundPage) {
      if (force || age(foundPage.data) >= freshFor) return findRank(foundPage.riotId);
      render(foundPage.data);
      return;
    }
    const targetPage = page, targetTier = tier, id = key(page, tier);
    const token = ++revision;
    const cached = cache.get(id);
    if (loadedKey !== id) currentData = undefined;
    loadedKey = id;
    if (cached && age(cached) < 24 * 60 * 60_000) render(cached);
    refresh.disabled = false;
    if (cached && age(cached) < freshFor && !force && !revalidate) {
      scheduleNext(cached);
      return;
    }
    pending();
    refresh.disabled = true;
    try {
      // An update may arrive while the stale response is still in flight.
      if ((revalidate || force) && inflight.has(id)) await inflight.get(id).catch(() => {});
      if (token !== revision || !visible) return;
      const data = await requestPage(targetPage, targetTier, force) ?? await requestPage(targetPage, targetTier);
      if (token === revision && visible) {
        render(data);
        scheduleNext(data);
      }
    } catch (_) {
      if (token === revision && visible && !currentData) {
        tableScroll.hidden = true;
        byId("ladder-failed").hidden = false;
      }
    } finally {
      if (token === revision) {
        tbody.setAttribute("aria-busy", "false");
        refresh.disabled = false;
      }
    }
  }

  function applyFilters() {
    for (const button of panel.querySelectorAll("[data-ladder-tier]")) {
      button.setAttribute("aria-pressed", String(button.dataset.ladderTier === tier));
    }
    proOnly.setAttribute("aria-checked", String(onlyPro));
  }

  function changePage(newPage, newTier = tier) {
    cancelHover();
    cancelIdle();
    foundPage = null;
    page = newPage;
    tier = newTier;
    applyFilters();
    if (scrollRoot) scrollRoot.scrollTop = 0;
    return showPage();
  }

  function cancelHover() {
    if (!hover) return;
    clearTimeout(hover.timer);
    hover.controller?.abort();
    hover = null;
  }

  function startHover(identity) {
    if (!identity || hover?.key === identity) return;
    cancelHover();
    const row = rendered.get(identity)?.row;
    if (!row) return;
    const task = { key: identity };
    hover = task;
    task.timer = setTimeout(async () => {
      task.controller = new AbortController();
      try {
        const result = await request(`/api/ladder/prewarm?riotId=${encodeURIComponent(`${row.gameName}#${row.tagLine}`)}`, { signal: task.controller.signal });
        if (hover === task) task.ref = result?.playerRef || "";
      } catch (_) {}
    }, 200);
  }

  function openRow(identity) {
    const row = rendered.get(identity)?.row;
    if (!row) return;
    returnKey = identity;
    window.dispatchEvent(new CustomEvent("deep-legends:open-player", { detail: {
      source: "ladder", region: "kr", gameName: row.gameName, tagLine: row.tagLine,
      playerRef: hover?.key === identity ? hover.ref : undefined,
      proMark: row.proMark === true, proPlayer: row.proPlayer,
    } }));
  }

  function enter(event) {
    const active = event.detail?.name === "ladder";
    if (active === visible || destroyed) return;
    visible = active;
    if (!visible) {
      ++revision;
      ++searchRevision;
      cancelHover();
      cancelIdle();
      clearInterval(updateTimer);
      return;
    }
    void showPage();
    updateTimer = setInterval(renderTime, 30_000);
    if (returnKey) rowButton(returnKey)?.focus({ preventScroll: true });
  }

  panel.addEventListener("click", (event) => {
    const target = event.target;
    const row = target.closest("[data-ladder-row]");
    const tierButton = target.closest("[data-ladder-tier]");
    const pageButton = target.closest("[data-ladder-page]");
    if (row) openRow(row.ladderIdentity);
    if (tierButton && tierButton.dataset.ladderTier !== tier) {
      ++searchRevision;
      highlighted = "";
      void changePage(1, tierButton.dataset.ladderTier);
    }
    if (pageButton && !pageButton.disabled && Number(pageButton.dataset.ladderPage) !== page) {
      ++searchRevision;
      highlighted = "";
      void changePage(Number(pageButton.dataset.ladderPage));
    }
    if (target.closest("[data-ladder-retry]")) void showPage({ force: true });
  });
  tbody.addEventListener("pointerover", (event) => startHover(event.target.closest("[data-ladder-row]")?.ladderIdentity));
  tbody.addEventListener("pointerout", (event) => {
    const row = event.target.closest("[data-ladder-row]");
    if (row && !row.contains(event.relatedTarget)) cancelHover();
  });
  refresh.addEventListener("click", () => void showPage({ force: true }));
  proOnly.addEventListener("click", () => {
    cancelHover();
    onlyPro = !onlyPro;
    applyFilters();
    if (currentData) renderRows(currentData);
  });

  function showSearchStatus(message) {
    searchStatus.textContent = message;
    searchStatus.hidden = !message;
  }
  input.addEventListener("input", () => { ++searchRevision; showSearchStatus(""); });
  search.addEventListener("submit", async (event) => {
    event.preventDefault();
    const riotId = input.value.trim();
    const parts = riotId.split("#").map((part) => part.trim());
    if (parts.length !== 2 || parts.some((part) => !part)) {
      showSearchStatus("请输入名字#编号");
      return;
    }
    await findRank(parts.join("#"));
  });

  async function findRank(riotId) {
    const token = ++searchRevision;
    showSearchStatus("");
    try {
      const match = await request(`/api/ladder/find?riotId=${encodeURIComponent(riotId)}`);
      if (token !== searchRevision || !visible) return;
      highlighted = riotId.toLowerCase();
      if (match.page > maxPage) {
        if (!match.data) {
          showSearchStatus(`第 ${number(match.rank)} 名`);
          return;
        }
        cancelHover();
        cancelIdle();
        ++revision;
        foundPage = { riotId, data: match.data };
        page = match.page;
        tier = "";
        applyFilters();
        refresh.disabled = false;
        render(match.data);
      } else {
        await changePage(match.page, "");
      }
      if (token !== searchRevision || !visible) return;
      if (!rowButton(highlighted) && onlyPro) {
        onlyPro = false;
        applyFilters();
        if (currentData) renderRows(currentData);
      }
      const result = rowButton(highlighted);
      result?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
      result?.focus({ preventScroll: true });
      showSearchStatus(result ? "" : `第 ${number(match.rank)} 名`);
    } catch (error) {
      if (token === searchRevision && visible) showSearchStatus(error.message === "404" ? "未找到该召唤师" : "查找失败");
    }
  }

  window.addEventListener("deep-legends:section", enter);
  window.addEventListener("deep-legends:ladder-prefetch", () => prefetch(1, ""));
  window.addEventListener("deep-legends:ladder-updated", (event) => {
    const { page: updatedPage, tier: updatedTier } = event.detail || {};
    cache.delete(key(updatedPage, updatedTier));
    if (visible && page === updatedPage && tier === updatedTier) void showPage({ revalidate: true });
  });
  window.addEventListener("deep-legends:resync", () => { if (visible) void showPage({ revalidate: true }); });
  window.addEventListener("deep-legends:privacy-changed", () => { if (visible && currentData) renderRows(currentData); });
  function dispose() {
    destroyed = true;
    visible = false;
    ++revision;
    ++searchRevision;
    cancelHover();
    cancelIdle();
    clearInterval(updateTimer);
  }
  window.addEventListener("deep-legends:dispose", dispose, { once: true });
  window.addEventListener("pagehide", dispose, { once: true });
  window.deepLegendsSections?.register("ladder", enter);
})();
