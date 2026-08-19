// 好友面板：右侧浮窗展示本机客户端的好友列表。
// 分组、顺序、折叠初始态与备注完全来自客户端（/api/social/friends），只读。
(() => {
  "use strict";

  const el = {
    toggle: document.getElementById("friends-toggle"),
    count: document.getElementById("friends-count"),
    dock: document.getElementById("friends-dock"),
    close: document.getElementById("friends-close"),
    search: document.getElementById("friends-search-input"),
    list: document.getElementById("friends-list"),
    summary: document.getElementById("friends-summary"),
  };
  if (!el.toggle || !el.dock) return;

  const OFFLINE_KEY = "offline";
  const state = {
    open: false,
    connected: false,
    loading: false,
    error: "",
    groups: [],
    friends: [],
    filter: "",
    collapsed: loadCollapsed(),
    stale: true,
    refreshTimer: 0,
    tickTimer: 0,
  };

  /* ---------- 小工具 ---------- */
  function loadCollapsed() {
    try { return JSON.parse(localStorage.getItem("lol-loot-friends-collapsed") || "{}") || {}; } catch (_) { return {}; }
  }
  function saveCollapsed() {
    try { localStorage.setItem("lol-loot-friends-collapsed", JSON.stringify(state.collapsed)); } catch (_) {}
  }
  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char]));
  }
  function profileIcon(icon) {
    if (!icon) return "/image-unavailable.svg";
    return `/api/image?path=${encodeURIComponent(`/lol-game-data/assets/v1/profile-icons/${icon}.jpg`)}`;
  }
  function championIcon(championId) {
    return `/api/image?path=${encodeURIComponent(`/lol-game-data/assets/v1/champion-icons/${championId}.png`)}`;
  }
  function formatDuration(ms) {
    const total = Math.max(0, Math.floor(ms / 1000));
    const minutes = Math.floor(total / 60);
    const seconds = total % 60;
    if (minutes >= 60) return `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
    return `${minutes}:${String(seconds).padStart(2, "0")}`;
  }
  function relativeTime(iso) {
    const timestamp = Date.parse(iso || "");
    if (!Number.isFinite(timestamp)) return "";
    const elapsed = Date.now() - timestamp;
    if (elapsed < 60_000) return "刚刚";
    const minutes = Math.floor(elapsed / 60_000);
    if (minutes < 60) return `${minutes} 分钟前`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours} 小时前`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days} 天前`;
    return "一个月前";
  }

  /* ---------- 状态语义 ---------- */
  function presenceKind(friend) {
    const availability = friend.availability || "";
    if (!availability || availability === "offline" || availability === "none") return "offline";
    if (availability === "mobile") return "mobile";
    if (availability === "away") return "away";
    if (availability === "dnd" || availability === "spectating") return "ingame";
    return "online";
  }
  const PRESENCE_WEIGHT = { ingame: 4, online: 3, away: 2, mobile: 1, offline: 0 };

  function statusHTML(friend, kind) {
    if (kind === "offline") {
      const seen = relativeTime(friend.lastSeenAt);
      return escapeHTML(seen ? `${seen}在线` : "离线");
    }
    if (kind === "mobile") return "手机在线";
    if (kind === "away") return "离开";
    if (kind === "ingame") {
      if (friend.availability === "spectating") return "观战中";
      const product = (friend.product || "").toLowerCase();
      if (product && product !== "league_of_legends") {
        const label = friend.productName || "其他产品";
        return escapeHTML(friend.gameStatus === "inGame" ? `${label} · 对局中` : label);
      }
      switch (friend.gameStatus) {
        case "inGame": {
          const parts = [friend.queueLabel || "对局中"];
          if (friend.championName) parts.push(friend.championName);
          const started = Number(friend.gameStartedAt) || 0;
          const duration = started ? ` · <b data-started="${started}">${formatDuration(Date.now() - started)}</b>` : "";
          return escapeHTML(parts.join(" · ")) + duration;
        }
        case "championSelect": return "英雄选择中";
        case "inQueue": return "队列中";
        case "spectating": return "观战中";
        default:
          if (/^hosting_/i.test(friend.gameStatus || "")) return "组队大厅";
          return "游戏中";
      }
    }
    return escapeHTML(friend.statusMessage ? `在线 · ${friend.statusMessage}` : "在线");
  }

  /* ---------- 数据 ---------- */
  async function loadFriends() {
    if (state.loading || !state.connected) return;
    state.loading = true;
    state.error = "";
    if (state.open && !state.friends.length) renderSkeleton();
    try {
      const response = await fetch("/api/social/friends", { headers: { Accept: "application/json" } });
      if (!response.ok) throw new Error((await response.text()).trim() || `本地服务返回 HTTP ${response.status}`);
      const data = await response.json();
      state.groups = Array.isArray(data.groups) ? data.groups : [];
      state.friends = Array.isArray(data.friends) ? data.friends : [];
      state.stale = false;
    } catch (error) {
      state.error = error?.message || "读取好友列表失败";
    } finally {
      state.loading = false;
      updateBadge();
      if (state.open) render();
    }
  }

  function matchesFilter(friend) {
    if (!state.filter) return true;
    const query = state.filter;
    return [friend.gameName, friend.tagLine, friend.note, friend.statusMessage]
      .some((field) => String(field || "").toLowerCase().includes(query));
  }

  function buildSections() {
    const known = new Map(state.groups.map((group) => [group.id, true]));
    const buckets = new Map(state.groups.map((group) => [group.id, []]));
    const offlineCounts = new Map();
    const fallback = state.groups.find((group) => group.isMetaGroup) || state.groups[0];
    const offline = [];
    const homeGroup = (friend) => {
      if (known.has(friend.displayGroupId)) return friend.displayGroupId;
      if (known.has(friend.groupId)) return friend.groupId;
      return fallback ? fallback.id : null;
    };
    for (const friend of state.friends) {
      const groupId = homeGroup(friend);
      if (groupId === null) continue;
      if (presenceKind(friend) === "offline") {
        // 离线好友统一归入底部“离线”组（与客户端一致），但仍计入原分组总数。
        offline.push(friend);
        offlineCounts.set(groupId, (offlineCounts.get(groupId) || 0) + 1);
        continue;
      }
      buckets.get(groupId).push(friend);
    }
    const sections = state.groups.map((group) => {
      const members = buckets.get(group.id) || [];
      members.sort((a, b) => {
        const weight = PRESENCE_WEIGHT[presenceKind(b)] - PRESENCE_WEIGHT[presenceKind(a)];
        if (weight) return weight;
        return String(a.gameName).localeCompare(String(b.gameName), "zh-Hans-CN");
      });
      const total = members.length + (offlineCounts.get(group.id) || 0);
      return { key: `g${group.id}`, name: group.name, members, total, online: members.length, defaultCollapsed: Boolean(group.collapsed), offline: false };
    });
    offline.sort((a, b) => (Date.parse(b.lastSeenAt || "") || 0) - (Date.parse(a.lastSeenAt || "") || 0));
    sections.push({ key: OFFLINE_KEY, name: "离线", members: offline, total: offline.length, online: 0, defaultCollapsed: true, offline: true });
    return sections;
  }

  function isCollapsed(section) {
    if (state.filter) return false;
    const preference = state.collapsed[section.key];
    return typeof preference === "boolean" ? preference : section.defaultCollapsed;
  }

  /* ---------- 渲染 ---------- */
  function renderSkeleton() {
    el.list.setAttribute("aria-busy", "true");
    el.list.innerHTML = '<div class="friends-skeleton" aria-hidden="true"><span></span><span></span><span></span></div>';
  }

  function renderEmpty(icon, text, retry) {
    const icons = {
      disconnected: '<path d="M8.5 9.5a3.2 3.2 0 1 1 3.2 3.2M2.8 20.4c.7-3.1 3.1-4.9 6-4.9 1.2 0 2.3.3 3.2.9"/><path d="m15.5 15.5 5 5m0-5-5 5"/>',
      search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
      empty: '<circle cx="9" cy="8" r="3.4"/><path d="M2.8 20c.7-3.2 3.2-5 6.2-5s5.5 1.8 6.2 5"/><circle cx="17" cy="9" r="2.6"/><path d="M16.4 15.2c2.5.2 4.3 1.8 4.8 4.3"/>',
    };
    el.list.setAttribute("aria-busy", "false");
    el.list.innerHTML = `<div class="friends-empty"><svg viewBox="0 0 24 24" aria-hidden="true">${icons[icon] || icons.empty}</svg><span>${text}</span>${retry ? '<button id="friends-retry" class="text-button" type="button">重试</button>' : ""}</div>`;
  }

  function renderFriendRow(friend, offlineSection) {
    const kind = presenceKind(friend);
    const nameTitle = `${friend.gameName}${friend.tagLine ? ` #${friend.tagLine}` : ""}`;
    const note = friend.note ? `<span class="friend-note">· ${escapeHTML(friend.note)}</span>` : "";
    const tag = friend.tagLine ? `<span class="friend-tag">#${escapeHTML(friend.tagLine)}</span>` : "";
    const champion = !offlineSection && kind === "ingame" && friend.championId && (friend.product || "league_of_legends") === "league_of_legends"
      ? `<img class="friend-champion" src="${championIcon(friend.championId)}" alt="" loading="lazy" decoding="async">`
      : "";
    return `<button class="friend-row${offlineSection ? " is-offline" : ""}" type="button" role="listitem" data-game-name="${escapeHTML(friend.gameName)}" data-tag-line="${escapeHTML(friend.tagLine || "")}" title="查看 ${escapeHTML(nameTitle)} 的战绩">
      <span class="friend-avatar"><img src="${profileIcon(friend.icon)}" alt="" loading="lazy" decoding="async"><span class="friend-presence ${kind}"></span></span>
      <span class="friend-copy">
        <span class="friend-name"><span>${escapeHTML(friend.gameName)}</span>${tag}${note}</span>
        <span class="friend-status ${kind}">${statusHTML(friend, kind)}</span>
      </span>
      ${champion}<span class="friend-open-hint" aria-hidden="true">战绩 ›</span>
    </button>`;
  }

  function render() {
    if (!state.connected) { renderEmpty("disconnected", "未连接英雄联盟客户端<br>登录并进入大厅后自动读取好友"); updateSummary(); return; }
    if (state.error && !state.friends.length) { renderEmpty("empty", escapeHTML(state.error), true); updateSummary(); return; }
    if (state.loading && !state.friends.length) { renderSkeleton(); return; }
    const sections = buildSections();
    const parts = [];
    let visibleTotal = 0;
    for (const section of sections) {
      const members = section.members.filter(matchesFilter);
      if (state.filter && !members.length) continue;
      if (!section.total && section.offline) continue;
      visibleTotal += members.length;
      const collapsed = isCollapsed(section);
      const count = section.offline
        ? `${section.total}`
        : `<b>${section.online}</b> / ${section.total}`;
      parts.push(`<button class="friends-group-head${section.offline ? " is-offline-group" : ""}" type="button" data-group-key="${section.key}" aria-expanded="${String(!collapsed)}">
        <span class="friends-group-arrow" aria-hidden="true">${collapsed ? "▸" : "▾"}</span>
        <span>${escapeHTML(section.name)}</span>
        <span class="friends-group-count">${count}</span>
      </button>`);
      if (!collapsed) parts.push(members.map((friend) => renderFriendRow(friend, section.offline)).join(""));
    }
    el.list.setAttribute("aria-busy", "false");
    if (!parts.length) {
      if (state.filter) renderEmpty("search", `没有匹配“${escapeHTML(state.filter)}”的好友<br>支持名称、Riot ID 与备注搜索`);
      else renderEmpty("empty", "客户端好友列表为空");
    } else {
      el.list.innerHTML = parts.join("");
    }
    updateSummary();
  }

  function updateSummary() {
    if (!state.connected || !state.friends.length) { el.summary.textContent = ""; return; }
    const online = state.friends.filter((friend) => presenceKind(friend) !== "offline").length;
    el.summary.innerHTML = `在线 <b>${online}</b> / ${state.friends.length}`;
  }

  function updateBadge() {
    const online = state.friends.filter((friend) => presenceKind(friend) !== "offline").length;
    el.count.textContent = String(online);
    el.count.hidden = !state.connected || !state.friends.length;
    el.toggle.title = state.connected ? `好友（在线 ${online} / ${state.friends.length}）` : "好友（未连接客户端）";
  }

  /* ---------- 对局时长每 30 秒原位刷新，避免整列表重绘 ---------- */
  function updateDurations() {
    for (const node of el.list.querySelectorAll("[data-started]")) {
      node.textContent = formatDuration(Date.now() - Number(node.dataset.started));
    }
  }
  function startTick() { stopTick(); state.tickTimer = setInterval(updateDurations, 30_000); }
  function stopTick() { clearInterval(state.tickTimer); state.tickTimer = 0; }

  /* ---------- 打开 / 收起 ---------- */
  function setOpen(open) {
    if (state.open === open) return;
    state.open = open;
    el.toggle.setAttribute("aria-expanded", String(open));
    if (open) {
      el.dock.classList.add("is-open");
      render();
      if (state.stale || state.error) void loadFriends();
      startTick();
      el.search.focus({ preventScroll: true });
    } else {
      el.dock.classList.remove("is-open");
      stopTick();
      if (el.dock.contains(document.activeElement)) el.toggle.focus();
    }
  }

  /* ---------- 事件 ---------- */
  el.toggle.addEventListener("click", () => setOpen(!state.open));
  el.close.addEventListener("click", () => setOpen(false));

  el.search.addEventListener("input", () => {
    state.filter = el.search.value.trim().toLowerCase();
    render();
  });

  el.list.addEventListener("click", (event) => {
    const retry = event.target.closest("#friends-retry");
    if (retry) { void loadFriends(); return; }
    const groupHead = event.target.closest(".friends-group-head");
    if (groupHead) {
      if (state.filter) return;
      const key = groupHead.dataset.groupKey;
      const sections = buildSections();
      const section = sections.find((item) => item.key === key);
      state.collapsed[key] = section ? !isCollapsed(section) : true;
      saveCollapsed();
      render();
      return;
    }
    const row = event.target.closest(".friend-row");
    if (!row) return;
    window.dispatchEvent(new CustomEvent("deep-legends:open-player", {
      detail: { gameName: row.dataset.gameName, tagLine: row.dataset.tagLine, region: "cn", source: "search" },
    }));
    setOpen(false);
  });

  // 头像 / 英雄小图加载失败时退回占位图（error 不冒泡，用捕获阶段委托）。
  el.list.addEventListener("error", (event) => {
    const image = event.target;
    if (!(image instanceof HTMLImageElement) || image.dataset.fallback) return;
    image.dataset.fallback = "1";
    image.src = "/image-unavailable.svg";
  }, true);

  document.addEventListener("pointerdown", (event) => {
    if (!state.open) return;
    if (el.dock.contains(event.target) || el.toggle.contains(event.target)) return;
    setOpen(false);
  });

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape" || !state.open) return;
    if (document.querySelector("dialog[open]")) return;
    event.stopPropagation();
    setOpen(false);
  }, true);

  window.addEventListener("deep-legends:status", (event) => {
    const connected = Boolean(event.detail?.connected);
    if (connected === state.connected) return;
    state.connected = connected;
    el.toggle.disabled = !connected;
    if (connected) {
      state.stale = true;
      void loadFriends();
    } else {
      state.groups = [];
      state.friends = [];
      state.error = "";
      updateBadge();
      if (state.open) render();
    }
  });

  window.addEventListener("deep-legends:friends-updated", () => {
    if (!state.connected) return;
    clearTimeout(state.refreshTimer);
    state.refreshTimer = setTimeout(() => { state.stale = true; void loadFriends(); }, 300);
  });

  el.toggle.disabled = true;
  updateBadge();
})();
