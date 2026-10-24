(() => {
  "use strict";
  const escape = window.deepLegendsRuntime.escapeHTML;
  const modes = {
    ranked: ["ranked", "单/双排 · 召唤师峡谷"],
    "aram-mayhem": ["aramMayhem", "海克斯大乱斗"],
    arena: ["arena", "斗魂竞技场"],
  };
  const documentIcon =
    '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4 2h7l2 2v10H3V2h1Zm2 3h4M6 8h4M6 11h3"/></svg>';
  let data = null,
    error = false,
    flight = null,
    dialog = null,
    opener = null,
    mode = "ranked",
    reloadQueued = false,
    refreshQueued = false,
    pollTimer = null,
    refreshPolls = 0;
  function validSource(value) {
    try {
      const u = new URL(value);
      return (
        u.protocol === "https:" &&
        u.host === "lol.qq.com" &&
        !u.username &&
        !u.password &&
        /^\/gicp\/news\/410\/\d+\.html$/.test(u.pathname) &&
        !u.search &&
        !u.hash
      );
    } catch {
      return false;
    }
  }
  function validate(value) {
    if (
      !value ||
      value.schema !== 1 ||
      !/^\d{1,3}(?:\.\d{1,3}){1,3}$/.test(value.version) ||
      !validSource(value.sourceUrl)
    )
      throw Error("公告格式无效");
    let count = 0;
    const visit = (x) => {
      if (++count > 12000) throw Error("公告条目过多");
      if (typeof x === "string" && x.length > 8192) throw Error("公告文字过长");
      if (Array.isArray(x)) {
        if (x.length > 1000) throw Error("公告条目过多");
        x.forEach(visit);
      } else if (x && typeof x === "object") Object.values(x).forEach(visit);
    };
    visit(value);
    for (const [key] of Object.values(modes))
      if (!Array.isArray(value.modes?.[key]?.sections))
        throw Error("公告模式缺失");
    return value;
  }
  function button(currentMode) {
    return `<button type="button" class="text-button" data-patch-notes="${escape(currentMode)}">${documentIcon}<span>版本更新</span>${data ? `<b>${escape(data.version)}</b>` : ""}</button>`;
  }
  function updateBadges() {
    for (const node of document.querySelectorAll("[data-patch-notes]")) {
      node.querySelector("b")?.remove();
      if (data) {
        const badge = document.createElement("b");
        badge.textContent = data.version;
        node.append(badge);
      }
    }
  }
  async function load(refresh = false) {
    if (flight) {if(refresh)refreshQueued=true;return flight;}
    clearTimeout(pollTimer);
    if (refresh) refreshPolls = 0;
    error = false;
    flight = (async () => {
      const controller = new AbortController(),
        timeout = setTimeout(() => controller.abort(), 5000);
      try {
        const response = await fetch(`/api/patch-notes${refresh ? "?refresh=1" : ""}`, {
          signal: controller.signal,
        });
        if (!response.ok) throw Error("公告暂不可用");
        data = validate(await response.json());
      } catch {
        error = true;
      } finally {
        clearTimeout(timeout);
        flight = null;
        updateBadges();
        renderDialog();
        if(refreshQueued){refreshQueued=false;reloadQueued=false;void load(true);}
        else if (reloadQueued) { reloadQueued = false; void load(); }
        else if (data?.updateStatus === "checking" && refreshPolls++ < 20) pollTimer = setTimeout(() => void load(), 1500);
      }
    })();
    renderDialog();
    return flight;
  }
  function icon(entry) {
    let url = "";
    if (entry.championId > 0)
      url =
        "/api/image?path=" +
        encodeURIComponent(
          `/lol-game-data/assets/v1/champion-icons/${Number(entry.championId)}.png`,
        );
    else if (entry.itemId > 0)
      url =
        "/api/image?path=" +
        encodeURIComponent(
          `/lol-game-data/assets/v1/items/${Number(entry.itemId)}.png`,
        );
    else if (mode === "aram-mayhem" && !entry.augmentId &&
      data?.augmentIcons?.[entry.name]?.source === "opgg" &&
      data.augmentIcons[entry.name].path?.startsWith("/meta/images/"))
      url = "/api/champion-asset?source=opgg&path=" +
        encodeURIComponent(data.augmentIcons[entry.name].path);
    else if (entry.augmentId > 0)
      url =
        "/api/champion-asset?source=builtin&path=" +
        encodeURIComponent(`/augments/${Number(entry.augmentId)}.png`);
    return url
      ? `<span class="ic"><img data-queued-src="${escape(url)}" alt="" loading="lazy" decoding="async"></span>`
      : "";
  }
  function listItem(item) {
    const parts = /^([^：:]{1,80}[：:])([\s\S]*)$/.exec(item);
    return parts
      ? `<li><span class="pn-list-name">${escape(parts[1])}</span><span class="pn-list-description">${escape(parts[2])}</span></li>`
      : `<li>${escape(item)}</li>`;
  }
  function renderSections(sections) {
    return sections
      .map((section) => {
        const entries = section.entries || [],
          items = section.items || [],
          count = entries.length + items.length;
        const unit =
          section.id === "champions"
            ? "位"
            : section.id === "items"
              ? "件"
              : "项";
        const equipment = entries.filter(entry => entry.itemId).length;
        const caption = section.id === "items" && equipment !== count
          ? [equipment ? `${equipment} 件` : "", count - equipment ? `${count - equipment} 项` : ""].filter(Boolean).join(" · ")
          : `${count} ${unit}`;
        return `<section class="pn-section"><h4>${escape(section.title)}${count ? ` <small>${caption}</small>` : ""}</h4>${section.intro ? `<p class="pn-intro">${escape(section.intro)}</p>` : ""}${entries
          .map((entry) => {
            const image = icon(entry);
            const augment = entry.augmentId > 0 || /(?:^|-)augments$/.test(section.id);
            return `<article class="pn-entry${image ? "" : " is-text-only"}${augment ? " is-augment" : ""}">${augment ? "" : image}<strong>${entry.isNew ? '<span class="pn-new">新增</span>' : ""}${augment ? image : ""}<span class="pn-entry-name">${escape(entry.name)}</span></strong><div>${(entry.groups || []).map((group, index) => `<div class="pn-group">${group.label ? `<span>${escape(group.label)}</span>` : ""}${(group.lines || []).map((line) => (line.text ? `<div class="pn-line${section.id === "champions" && index === 0 && !group.label ? " is-explanation" : ""}">${escape(line.text)}</div>` : `<div class="pn-line"><span class="k">${escape(line.k)}：</span><span class="from">${escape(line.from)}</span><span class="arrow">⇒</span><strong class="to">${escape(line.to)}</strong></div>`)).join("")}</div>`).join("")}</div></article>`;
          })
          .join(
            "",
          )}${items.length ? `<ul class="pn-list">${items.map(listItem).join("")}</ul>` : ""}</section>`;
      })
      .join("");
  }
  function renderDialog() {
    if (!dialog) return;
    const [key, label] = modes[mode] || modes.ranked,
      sections = data?.modes?.[key]?.sections;
    const body = dialog.querySelector(".pn-body"),
      scroll = body?.scrollTop || 0,
      focusedRetry = document.activeElement?.matches("[data-pn-retry]"),
      focusedInside = dialog.contains(document.activeElement),
      focusedSource = document.activeElement?.matches(".pn-foot a");
    const status = error || data?.updateStatus === "failed" ? "检查更新失败，已保留保存的公告" : data?.updateStatus === "checking" ? "正在检查官方更新…" : data?.updateStatus === "checked" ? "已检查官方公告" : "当前为已保存公告";
    dialog.innerHTML = `<header class="pn-head"><h3><b>${escape(data?.version || "")}</b> 版本更新</h3><span class="pn-mode">${label}</span><span class="pn-date">${data ? escape(data.publishedAt) + " 发布" : ""}</span><button type="button" class="pn-close" data-pn-close aria-label="关闭版本更新">×</button></header><div class="pn-body">${!data && flight ? '<div class="pn-loading" aria-label="正在读取版本公告"><i></i><i></i><i></i></div>' : !data && error ? '<div class="pn-empty"><span>暂时无法获取版本公告</span><button type="button" class="text-button" data-pn-retry>重试</button></div>' : !sections?.length ? '<div class="pn-empty">本版本没有该模式的改动</div>' : renderSections(sections)}</div><footer class="pn-foot"><span>${data ? `${escape(status)} <button type="button" class="text-button" data-pn-retry${flight ? " disabled" : ""}>检查更新</button>` : ""}</span>${data ? `<a href="${escape(data.sourceUrl)}" target="_blank" rel="noopener noreferrer">查看官方公告原文</a>` : ""}</footer>`;
    dialog.querySelectorAll(".is-augment .ic img").forEach((img) => {
      img.addEventListener("error", () => img.parentElement?.remove(), { once: true });
    });
    dialog.querySelector(".pn-body").scrollTop = scroll;
    if (focusedInside)
      dialog
        .querySelector(
          focusedRetry
            ? "[data-pn-retry],.pn-close"
            : focusedSource
              ? ".pn-foot a,.pn-close"
              : ".pn-close",
        )
        ?.focus();
  }
  function close() {
    dialog?.parentElement.remove();
    dialog = null;
    if (opener?.isConnected) opener.focus();
    else
      [...document.querySelectorAll("[data-patch-notes]")]
        .find((n) => n.getClientRects().length)
        ?.focus();
  }
  function open(currentMode, trigger) {
    if (dialog) close();
    mode = currentMode;
    opener = trigger;
    const scrim = document.createElement("div");
    scrim.className = "pn-scrim";
    dialog = document.createElement("section");
    dialog.className = "pn-dialog";
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    dialog.setAttribute("aria-label", "版本更新");
    scrim.append(dialog);
    document.body.append(scrim);
    scrim.addEventListener("mousedown", (event) => {
      if (event.target === scrim) close();
    });
    scrim.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        close();
      }
      if (event.key === "Tab") {
        const nodes = [...dialog.querySelectorAll("button,a[href]")];
        if (event.shiftKey && document.activeElement === nodes[0]) {
          event.preventDefault();
          nodes.at(-1).focus();
        } else if (!event.shiftKey && document.activeElement === nodes.at(-1)) {
          event.preventDefault();
          nodes[0].focus();
        }
      }
    });
    renderDialog();
    dialog.querySelector(".pn-close").focus();
    const checked=Date.parse(data?.successfulAt||data?.checkedAt||"");
    if(!Number.isFinite(checked)||Date.now()-checked>600000)void load(true);
  }
  document.addEventListener("click", (event) => {
    const trigger = event.target.closest("[data-patch-notes]");
    if (trigger) {
      open(trigger.dataset.patchNotes, trigger);
      return;
    }
    if (event.target.closest("[data-pn-close]")) close();
    if (event.target.closest("[data-pn-retry]")) void load(true);
  });
  window.addEventListener(
    "deep-legends:patch-notes-changed",
    () => { if (flight) reloadQueued = true; else void load(); },
  );
  window.deepLegendsPatchNotes = Object.freeze({
    button,
    validate,
    renderSections,
    open,
  });
  window.deepLegendsSections.register("patch-notes", () => {});
  void load();
})();
