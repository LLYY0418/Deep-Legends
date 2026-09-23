// R123：收藏页「头像与旗帜」只读浏览子页。
// 数据源为现成的只读目录接口 /api/facade/icons 与 /api/facade/banners；
// 视觉与交互复用皮肤收藏的组件族（view-tabs / filters / skin-grid / skin-card-template / list-meta），
// 不提供任何写入或切换按钮。
(() => {
  "use strict";

  const escapeHTML = window.deepLegendsRuntime?.escapeHTML || ((value) => String(value ?? "").replace(/[&<>'"]/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", "\"": "&quot;" }[character])));
  function formatCount(value) {
    return Number(value || 0).toLocaleString("zh-CN");
  }

  const el = {
    panel: document.getElementById("favorites-facade-panel"),
    tabs: [...document.querySelectorAll("#favorites-facade-panel [data-view]")],
    groups: document.getElementById("facade-quick-groups"),
    search: document.getElementById("facade-search"),
    setControl: document.getElementById("facade-set-control"),
    set: document.getElementById("facade-set"),
    unownedControl: document.getElementById("facade-unowned-control"),
    showUnowned: document.getElementById("facade-show-unowned"),
    sort: document.getElementById("facade-sort"),
    direction: document.getElementById("facade-sort-direction"),
    meta: document.getElementById("facade-list-meta"),
    grid: document.getElementById("facade-grid"),
    toolbar: document.querySelector("#favorites-facade-panel .collection-toolbar"),
    listRow: document.querySelector("#favorites-facade-panel .list-row"),
    template: document.getElementById("skin-card-template"),
    dialog: document.getElementById("facade-detail-dialog"),
    dialogImage: document.getElementById("facade-detail-image"),
    dialogFallback: document.getElementById("facade-detail-fallback"),
    dialogTitle: document.getElementById("facade-detail-title"),
    dialogData: document.getElementById("facade-detail-data"),
    dialogClose: document.getElementById("facade-detail-close"),
  };

  const GROUPS = {
    icons: [["all", "全部头像"], ["recent", "近三年新增"]],
    banners: [["all", "全部"], ["tencent", "国服专属"]],
  };
  const SORTS = {
    icons: [["new", "最新在前"], ["old", "最早在前"], ["name", "按名称"], ["unowned", "未拥有优先"]],
    banners: [["id", "按编号排序"], ["name", "按名称"], ["unowned", "未拥有优先"]],
  };

  const state = {
    view: "icons",
    connected: false,
    icons: null,
    banners: null,
    flights: { icons: null, banners: null },
    filters: {
      // R130 P5：头像默认只显示已拥有；旗帜仍默认显示全部。
      icons: { query: "", group: "all", set: "", sort: "new", descending: false, showUnowned: false },
      banners: { query: "", group: "all", sort: "id", descending: false, showUnowned: true },
    },
    renderGeneration: 0,
    searchTimer: 0,
    composing: false,
  };

  function imageURL(path) {
    const value = String(path || "");
    return value ? `/api/image?path=${encodeURIComponent(value)}` : "";
  }
  function iconImageURL(icon) {
    return imageURL(`/lol-game-data/assets/v1/profile-icons/${Number(icon.id)}.jpg`);
  }

  // ---------- 纯过滤/排序/字段映射（测试直接抽取这些函数） ----------
  function facadeCollectionIconRows(icons, filters, currentYear, ownershipUnavailable) {
    const query = String(filters.query || "").trim().toLowerCase();
    const score = (icon) => Number(window.deepLegendsChampionSearch?.scoreOption?.(query, "", [icon.title, ...(icon.searchTerms || [])].join(" "))) || 0;
    const rows = (Array.isArray(icons) ? icons : []).filter((icon) => {
      if (filters.group === "recent" && Number(icon.year) < currentYear - 2) return false;
      if (filters.set && !(icon.sets || []).includes(filters.set)) return false;
      if (!ownershipUnavailable && !filters.showUnowned && !icon.owned) return false;
      if (query && score(icon) <= 0 && ![icon.title, ...(icon.searchTerms || [])].join(" ").toLowerCase().includes(query)) return false;
      return true;
    });
    const base = {
      new: (a, b) => Number(b.year) - Number(a.year) || Number(b.id) - Number(a.id),
      old: (a, b) => Number(a.year) - Number(b.year) || Number(a.id) - Number(b.id),
      name: (a, b) => String(a.title).localeCompare(String(b.title), "zh-CN") || Number(a.id) - Number(b.id),
      unowned: (a, b) => Number(Boolean(a.owned)) - Number(Boolean(b.owned)) || Number(a.id) - Number(b.id),
    }[filters.sort] || ((a, b) => Number(a.id) - Number(b.id));
    rows.sort((a, b) => (filters.descending ? -base(a, b) : base(a, b)));
    return rows;
  }

  function facadeCollectionBannerRows(banners, filters, ownershipUnavailable) {
    const query = String(filters.query || "").trim().toLowerCase();
    const score = (banner) => Number(window.deepLegendsChampionSearch?.scoreOption?.(query, "", [banner.localizedName, ...(banner.searchTerms || [])].join(" "))) || 0;
    const rows = (Array.isArray(banners) ? banners : []).filter((banner) => {
      if (filters.group === "tencent" && !banner.isTencentOnly) return false;
      if (!ownershipUnavailable && !filters.showUnowned && !banner.owned) return false;
      if (query && score(banner) <= 0 && ![banner.localizedName, banner.id, banner.idSecondary, ...(banner.searchTerms || [])].join(" ").toLowerCase().includes(query)) return false;
      return true;
    });
    const base = {
      id: (a, b) => Number(a.id) - Number(b.id) || String(a.id).localeCompare(String(b.id)),
      name: (a, b) => String(a.localizedName).localeCompare(String(b.localizedName), "zh-CN") || Number(a.id) - Number(b.id),
      unowned: (a, b) => Number(Boolean(a.owned)) - Number(Boolean(b.owned)) || Number(a.id) - Number(b.id),
    }[filters.sort] || ((a, b) => Number(a.id) - Number(b.id));
    rows.sort((a, b) => (filters.descending ? -base(a, b) : base(a, b)));
    return rows;
  }

  function facadeCollectionIconFields(icon, ownershipUnavailable) {
    const sets = Array.isArray(icon.sets) && icon.sets.length ? icon.sets.join(" · ") : "未分类";
    return {
      title: String(icon.title || `头像 ${icon.id}`),
      hero: `${sets} · ${Number(icon.year) || "年份未知"}`,
      meta: `ID ${icon.id}`,
      image: iconImageURL(icon),
      locked: !ownershipUnavailable && !icon.owned,
      state: ownershipUnavailable ? "拥有状态未知" : icon.owned ? "已拥有" : "未拥有",
      owned: Boolean(icon.owned),
    };
  }

  function facadeCollectionBannerFields(banner, ownershipUnavailable) {
    return {
      title: String(banner.localizedName || `旗帜 ${banner.id}`),
      hero: banner.isTencentOnly ? "国服专属" : "通用",
      meta: banner.idSecondary ? `ID ${banner.id} · ${banner.idSecondary}` : `ID ${banner.id}`,
      image: imageURL(banner.imagePath),
      locked: !ownershipUnavailable && !banner.owned,
      state: ownershipUnavailable ? "拥有状态未知" : banner.owned ? "已拥有" : "未拥有",
      owned: Boolean(banner.owned),
    };
  }

  // R130 P5：摘要行去掉「· 已显示 N」，只留目录总数与已拥有数。这是同一份
  // parts 的纯文本投影：摘要行被拆成多个节点后，读屏软件会一段一段念，给
  // role="status" 的容器挂一份完整文本作为 aria-label，播报才连得起来。
  function facadeCollectionMetaText(kind, total, ownedCount, ownershipUnavailable) {
    return facadeCollectionMetaParts(kind, total, ownedCount, ownershipUnavailable).map(([text]) => text).join("");
  }
  // R130 P5：摘要行里的数字用主题色。返回 [文本, 是否数字]，由 renderFacadeMeta
  // 用 DOM 拼接——绝不把未转义文本塞进 innerHTML。
  function facadeCollectionMetaParts(kind, total, ownedCount, ownershipUnavailable) {
    const parts = [["共 ", false], [formatCount(total), true], [` 款${kind} · `, false]];
    if (ownershipUnavailable) {
      parts.push(["拥有状态未读取", false]);
      return parts;
    }
    parts.push(["已拥有 ", false], [formatCount(ownedCount), true]);
    return parts;
  }
  function renderFacadeMeta(kind, total, ownedCount, ownershipUnavailable) {
    el.meta.setAttribute("aria-label", facadeCollectionMetaText(kind, total, ownedCount, ownershipUnavailable));
    el.meta.replaceChildren(...facadeCollectionMetaParts(kind, total, ownedCount, ownershipUnavailable).map(([text, isNumber]) => {
      if (!isNumber) return document.createTextNode(text);
      const value = document.createElement("b");
      value.className = "list-meta-number";
      value.textContent = text;
      return value;
    }));
  }

  // ---------- 渲染 ----------
  function panelVisible() {
    return Boolean(el.panel) && !el.panel.hidden;
  }

  function renderDisconnected() {
    state.renderGeneration += 1;
    // 断连时与收藏页其它子页一致：只保留空状态提示，筛选/统计/列表行全部收起。
    el.toolbar.hidden = true;
    el.listRow.hidden = true;
    el.meta.textContent = "";
    // R130 P5：aria-label 会盖过内容给读屏软件播报，断连/出错时必须一起清掉，
    // 否则会留着上一份目录摘要。
    el.meta.removeAttribute("aria-label");
    el.grid.setAttribute("aria-busy", "false");
    el.grid.innerHTML = '<div class="gameplay-empty"><span aria-hidden="true">❖</span><strong>等待英雄联盟客户端</strong><p>登录国服客户端并进入大厅后，这里会自动展示头像与旗帜目录。</p></div>';
  }

  function renderError(message) {
    state.renderGeneration += 1;
    el.grid.setAttribute("aria-busy", "false");
    el.grid.innerHTML = "";
    const empty = document.createElement("div");
    empty.className = "gameplay-empty";
    empty.innerHTML = `<span aria-hidden="true">!</span><strong>目录读取失败</strong><p>${escapeHTML(message || "请稍后重试。")}</p>`;
    const retry = document.createElement("button");
    retry.type = "button";
    retry.className = "text-button";
    retry.textContent = "重试";
    retry.addEventListener("click", () => load(state.view, true));
    empty.append(retry);
    el.grid.append(empty);
    el.meta.textContent = message || "目录读取失败";
    el.meta.removeAttribute("aria-label");
  }

  function syncControls() {
    const filters = state.filters[state.view];
    const unavailable = ownershipUnavailable();
    el.setControl.hidden = state.view !== "icons";
    el.unownedControl.hidden = unavailable;
    // R124：拥有状态不可用时开关被隐藏，此时不能把网格滤空。
    // R130 P5-4：不再改写用户保存的值——以前这里会把记忆值无条件复位成打开，
    // 拥有状态恢复后开关仍然是开着的，用户设置被永久改掉。实际生效值由
    // facadeCollection*Rows 里的「ownershipUnavailable || filters.showUnowned」决定。
    el.showUnowned.checked = filters.showUnowned;
    el.groups.replaceChildren(...GROUPS[state.view].map(([key, label]) => {
      const button = document.createElement("button");
      button.type = "button";
      button.dataset.facadeGroup = key;
      button.className = filters.group === key ? "is-active" : "";
      button.setAttribute("aria-pressed", String(filters.group === key));
      button.textContent = label;
      button.addEventListener("click", () => { filters.group = key; syncControls(); render(); });
      return button;
    }));
    const options = SORTS[state.view];
    if (!options.some(([value]) => value === filters.sort)) filters.sort = options[0][0];
    el.sort.replaceChildren(...options.map(([value, label]) => new Option(label, value)));
    el.sort.value = filters.sort;
    el.direction.textContent = filters.descending ? "降序" : "升序";
    el.direction.setAttribute("aria-pressed", String(filters.descending));
    el.grid.classList.toggle("is-icons", state.view === "icons");
    el.grid.classList.toggle("is-banners", state.view === "banners");
    for (const tab of el.tabs) {
      const active = tab.dataset.view === state.view;
      tab.classList.toggle("is-active", active);
      tab.setAttribute("aria-selected", String(active));
      tab.tabIndex = active ? 0 : -1;
    }
    if (state.view === "icons") syncSetOptions();
  }

  function syncSetOptions() {
    const icons = state.icons?.icons || [];
    const sets = [...new Set(icons.flatMap((icon) => icon.sets || []))].sort((a, b) => a.localeCompare(b, "zh-CN"));
    const current = state.filters.icons.set;
    el.set.replaceChildren(new Option("全部系列", ""), ...sets.map((set) => new Option(set, set)));
    el.set.value = sets.includes(current) ? current : "";
    state.filters.icons.set = el.set.value;
  }

  function ownershipUnavailable() {
    if (state.view === "icons") return Boolean(state.icons?.iconOwnershipUnavailable);
    return Boolean(state.banners?.bannerOwnershipUnavailable);
  }

  function currentRows() {
    const filters = state.filters[state.view];
    if (state.view === "icons") return facadeCollectionIconRows(state.icons?.icons || [], filters, new Date().getFullYear(), ownershipUnavailable());
    return facadeCollectionBannerRows(state.banners?.banners || [], filters, ownershipUnavailable());
  }

  // R130 P2：旗帜是细长竖图，格子高度必须跟着图片真实比例走。只用第一张加载
  // 成功的旗帜取样写进网格变量，避免每张卡各写一次导致整个网格反复重排；取样
  // 之前 CSS 里已有 0.3 的默认值兜底。
  let bannerRatioSampled = false;
  // R130 P2：目录被重新读取（重试按钮、断连重连）时要能重新取样，第一张旗帜的
  // 比例不能永久锁死在网格上。
  function resetBannerRatioSample() {
    bannerRatioSampled = false;
    el.grid.style.removeProperty("--facade-banner-ratio");
  }
  // isBanner 由 createCard 在建卡时定死：卡片图片的 onload 是异步的，等它回来时
  // state.view 可能已经切走，读实时值会让一张方形头像把 1.0 写进整个旗帜网格。
  function sampleBannerRatio(image, isBanner) {
    if (!isBanner || bannerRatioSampled) return;
    const width = Number(image.naturalWidth);
    const height = Number(image.naturalHeight);
    if (!width || !height) return;
    bannerRatioSampled = true;
    el.grid.style.setProperty("--facade-banner-ratio", (width / height).toFixed(4));
  }

  // R130 P6：头像/旗帜详情按图片自身尺寸显示。以前弹窗固定 560px、图片区域
  // 560×560，128px 的头像被放大 4 倍多——详情图发糊就是这么来的。
  const DETAIL_ART_PLACEHOLDER_PX = 128;
  const DETAIL_ART_ICON_MAX_PX = 256;
  const DETAIL_ART_BANNER_MAX_HEIGHT_PX = 320;
  function setDetailArtSize(width, height) {
    el.dialog.style.setProperty("--facade-art-width", `${Math.max(1, Math.round(width))}px`);
    el.dialog.style.setProperty("--facade-art-height", `${Math.max(1, Math.round(height))}px`);
  }
  function applyDetailArtSize() {
    const width = Number(el.dialogImage.naturalWidth);
    const height = Number(el.dialogImage.naturalHeight);
    if (!width || !height) return;
    if (state.view === "icons") {
      // 正方形区域，边长取图片较长边并封顶 256px；object-fit: contain 保证小于
      // 256px 的头像按 CSS 像素 1:1 显示，绝不放大。
      const size = Math.min(DETAIL_ART_ICON_MAX_PX, Math.max(width, height));
      setDetailArtSize(size, size);
      return;
    }
    // 旗帜按自身比例缩放，高度不超过弹窗可视高度；contain 保证不裁剪。
    const scaled = Math.min(DETAIL_ART_BANNER_MAX_HEIGHT_PX, height);
    setDetailArtSize(scaled * (width / height), scaled);
  }

  function createCard(fields, item) {
    const card = el.template.content.firstElementChild.cloneNode(true);
    card.querySelector("strong").textContent = fields.title;
    card.querySelector(".skin-hero").textContent = fields.hero;
    card.querySelector(".skin-meta").textContent = fields.meta;
    // R130 P3：头像与旗帜卡片不再显示右上角的「已拥有 / 未拥有」标签。未拥有靠
    // 置灰 + 锁图标表示；拥有状态在详情弹窗里仍然保留一行。拥有状态未知（R124
    // 降级）时同样不显示标签——那时已经有摘要行「拥有状态未读取」，而且不上锁。
    const badge = card.querySelector(".skin-state");
    badge.textContent = "";
    badge.hidden = true;
    card.classList.toggle("is-locked", fields.locked);
    card.setAttribute("aria-label", fields.title);
    // R125：头像格子只展示名称一行，完整名称放 title 供悬停查看。
    if (state.view === "icons") card.title = fields.title;
    // R130 P2：旗帜格子同样只保留名称一行，完整名称放 title。
    if (state.view === "banners") card.title = fields.title;
    const isBanner = state.view === "banners";
    const image = card.querySelector("img");
    const fallback = card.querySelector(".image-fallback");
    if (fields.image) {
      image.onload = () => { image.classList.add("is-loaded"); fallback.hidden = true; sampleBannerRatio(image, isBanner); };
      image.onerror = () => { fallback.textContent = "无图"; };
      image.setAttribute("data-queued-src", fields.image);
    } else {
      image.remove();
      fallback.textContent = "";
    }
    card.addEventListener("click", () => openDetail(item, fields));
    return card;
  }

  function render() {
    if (!panelVisible()) return;
    if (!state.connected) { renderDisconnected(); return; }
    const payload = state.view === "icons" ? state.icons : state.banners;
    el.toolbar.hidden = false;
    el.listRow.hidden = false;
    if (!payload) { load(state.view, false); return; }
    const generation = ++state.renderGeneration;
    const unavailable = ownershipUnavailable();
    const items = state.view === "icons" ? payload.icons || [] : payload.banners || [];
    const rows = currentRows();
    const ownedCount = items.filter((item) => item.owned).length;
    const total = state.view === "icons" ? (Number(payload.total) || items.length) : items.length;
    renderFacadeMeta(state.view === "icons" ? "头像" : "旗帜", total, ownedCount, unavailable);
    el.grid.setAttribute("aria-busy", "false");
    el.grid.replaceChildren();
    if (!rows.length) {
      el.grid.innerHTML = '<div class="gameplay-empty"><span aria-hidden="true">⌕</span><strong>没有符合条件的条目</strong><p>调整搜索、系列或快捷分类后再试。</p></div>';
      return;
    }
    let index = 0;
    const chunk = () => {
      if (generation !== state.renderGeneration || !panelVisible()) return;
      const started = performance.now();
      const fragment = document.createDocumentFragment();
      // 与生涯页选择器同样的分帧预算：5099 条目录不能一次性同步渲染。
      while (index < rows.length && performance.now() - started < 7) {
        const item = rows[index++];
        const fields = state.view === "icons" ? facadeCollectionIconFields(item, unavailable) : facadeCollectionBannerFields(item, unavailable);
        fragment.append(createCard(fields, item));
      }
      el.grid.append(fragment);
      if (index < rows.length) requestAnimationFrame(chunk);
    };
    chunk();
  }

  async function load(view, force = false) {
    if (!state.connected) { renderDisconnected(); return; }
    const key = view === "icons" ? "icons" : "banners";
    if (!force && state[key]) { render(); return; }
    if (state.flights[key]) return state.flights[key];
    el.grid.setAttribute("aria-busy", "true");
    const flight = (async () => {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), 10000);
      try {
        const response = await fetch(`/api/facade/${key}`, { signal: controller.signal, headers: { Accept: "application/json" } });
        if (response.status === 409) {
          state.connected = false;
          state.icons = state.banners = null;
          renderDisconnected();
          return;
        }
        if (!response.ok) throw new Error(`目录读取失败（${response.status}）`);
        state[key] = await response.json();
        if (key === "banners") resetBannerRatioSample();
        if (state.view === view) { syncControls(); render(); }
      } catch (error) {
        if (state.view === view) renderError(error?.message || "目录读取失败");
      } finally {
        clearTimeout(timer);
        state.flights[key] = null;
      }
    })();
    state.flights[key] = flight;
    return flight;
  }

  // ---------- 详情弹窗（只读，无任何写入按钮） ----------
  function openDetail(item, fields) {
    el.dialogTitle.textContent = fields.title;
    el.dialogImage.classList.remove("is-loaded");
    el.dialogFallback.hidden = false;
    el.dialogFallback.textContent = "加载中";
    // R130 P6：加载前先按 128×128 占位，图片到位再换成真实尺寸，弹窗尺寸不跳动。
    setDetailArtSize(DETAIL_ART_PLACEHOLDER_PX, DETAIL_ART_PLACEHOLDER_PX);
    if (fields.image) {
      el.dialogImage.onload = () => { el.dialogImage.classList.add("is-loaded"); el.dialogFallback.hidden = true; applyDetailArtSize(); };
      el.dialogImage.onerror = () => { el.dialogFallback.textContent = "无图"; };
      // R130 P6：同一个条目再打开一次时 URL 没变，浏览器可能直接复用当前请求、
      // 不再派发 load，applyDetailArtSize 就永远不会跑，弹窗会卡在 128×128 的
      // 占位上、图片却已经显示出来了。先摘掉 src 强制重新走一次请求。
      el.dialogImage.removeAttribute("src");
      el.dialogImage.src = fields.image;
    } else {
      el.dialogImage.removeAttribute("src");
      el.dialogFallback.textContent = "无图";
    }
    const rows = state.view === "icons"
      ? [["名称", fields.title], ["系列", fields.hero], ["拥有状态", fields.state], ["ID", String(item.id)]]
      : [["名称", fields.title], ["国服专属", item.isTencentOnly ? "是" : "否"], ["拥有状态", fields.state], ["ID", item.idSecondary ? `${item.id} · ${item.idSecondary}` : String(item.id)]];
    el.dialogData.replaceChildren(...rows.flatMap(([label, value]) => {
      const dt = document.createElement("dt");
      dt.textContent = label;
      const dd = document.createElement("dd");
      dd.textContent = value;
      return [dt, dd];
    }));
    if (!el.dialog.open) {
      el.dialog.showModal();
      window.desktopTheme?.setModalOpen?.(true);
    }
  }

  function setView(view) {
    const next = view === "banners" ? "banners" : "icons";
    const changed = state.view !== next;
    state.view = next;
    syncControls();
    if (changed && !state[next]) load(next, false);
    else render();
  }

  function activate() {
    if (!state.connected) { renderDisconnected(); return; }
    syncControls();
    if (!state[state.view]) load(state.view, false);
    else render();
  }

  // ---------- 事件绑定 ----------
  for (const tab of el.tabs) tab.addEventListener("click", () => setView(tab.dataset.view));
  el.search.addEventListener("compositionstart", () => { state.composing = true; clearTimeout(state.searchTimer); });
  el.search.addEventListener("compositionend", () => { state.composing = false; queueSearch(); });
  el.search.addEventListener("input", () => { if (!state.composing) queueSearch(); });
  function queueSearch() {
    clearTimeout(state.searchTimer);
    state.searchTimer = setTimeout(() => { state.filters[state.view].query = el.search.value; render(); }, 150);
  }
  el.set.addEventListener("change", () => { state.filters.icons.set = el.set.value; render(); });
  el.showUnowned.addEventListener("change", () => { state.filters[state.view].showUnowned = el.showUnowned.checked; render(); });
  el.sort.addEventListener("change", () => { state.filters[state.view].sort = el.sort.value; render(); });
  el.direction.addEventListener("click", () => {
    const filters = state.filters[state.view];
    filters.descending = !filters.descending;
    el.direction.textContent = filters.descending ? "降序" : "升序";
    el.direction.setAttribute("aria-pressed", String(filters.descending));
    render();
  });
  el.dialogClose.addEventListener("click", () => el.dialog.close());
  el.dialog.addEventListener("click", (event) => { if (event.target === el.dialog) el.dialog.close(); });
  el.dialog.addEventListener("cancel", (event) => { event.preventDefault(); el.dialog.close(); });
  el.dialog.addEventListener("close", () => window.desktopTheme?.setModalOpen?.(false));

  window.addEventListener("deep-legends:status", (event) => {
    const connected = Boolean(event.detail?.connected);
    if (connected === state.connected) return;
    state.connected = connected;
    state.icons = state.banners = null;
    if (!connected) el.dialog.close?.();
    if (panelVisible()) activate();
  });

  window.deepLegendsFavoritesFacade = Object.freeze({ activate, setView });
})();
