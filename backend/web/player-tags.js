(() => {
  "use strict";
  const esc = (value) => window.deepLegendsRuntime.escapeHTML(value),
    ctx = () => window.deepLegendsPlayerTagContext;
  const bookmark =
    '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4.5 2.5h7v11L8 11 4.5 13.5z"/></svg>';
  const eye =
    '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5Z"/><circle cx="8" cy="8" r="2"/></svg>';
  const records = new Map(),
    players = new Map(),
    queried = new Set(),
    pending = new Map(),
    edits = new Map(), cancelled = new Set();
  let catalog = [],
    version = 0,
    request = null,
    queryRetryAt = 0,
    queryFailures = 0,
    timer,
    observer,
    scheduled = false,
    modal = null,
    opener = null,
    openerRef = "",
    openerLive = false,
    sequence = [],
    current = "",
    saveTail = Promise.resolve(),
    saveError = "",
    settingsTab = "tags",
    page = 0,
    search = "",
    filter = "",
    sort = "met",
    initialized = false;
  const eligible = (p) => p?.playerRef && !p.isCurrent && p.taggable === true;
  const shown = () =>
    (window.deepLegendsPreferenceStorage || localStorage).getItem(
      "lol-loot-mytags",
    ) !== "false";
  const toast = (text) => window.deepLegendsToast?.(text);
  async function api(body) {
    const r = await fetch("/api/player-tags", {
      method: body ? "POST" : "GET",
      headers: { "Content-Type": "application/json" },
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    if (!r.ok)
      throw Object.assign(Error((await r.text()).slice(0, 180) || "玩家标签保存失败"), {status:r.status});
    return r.json();
  }
  const empty = () => ({ tags: [], note: "" });
  const normalize = (r) => ({
    ...empty(),
    ...r,
    tags: Array.isArray(r?.tags) ? r.tags : [],
  });
  const tags = (record) =>
    (record?.tags || [])
      .map((id) => catalog.find((t) => t.id === id))
      .filter(Boolean);
  const tone = (record) =>
    tags(record).some((t) => t.group === "neg")
      ? "neg"
      : tags(record).some((t) => t.group === "mine")
        ? "mine"
        : "pos";
  const marked = (ref) =>
    !!(records.get(ref)?.tags?.length || records.get(ref)?.note);
  const date = (at) => (at ? new Date(at).toLocaleDateString("zh-CN") : "");
  const ago = (at) => {
    const days = Math.floor((Date.now() - Number(at)) / 86400000);
    return !at
      ? "第一次遇到"
      : days < 1
        ? "刚刚"
        : days < 7
          ? `${days}天前`
          : date(at);
  };
  const queue = (id) =>
    ({
      420: "单/双排",
      440: "灵活组排",
      450: "极地大乱斗",
      2400: "海克斯大乱斗",
      1700: "斗魂竞技场",
    })[id] || "其他模式";
  function tooltip(ref) {
    const r = records.get(ref),
      p = players.get(ref);
    if (!r) return "";
    return [
      ctx().name(p),
      tags(r)
        .map((t) => t.name)
        .join(" · "),
      r.note,
      `标记于 ${date(r.createdAt)}`,
      r.lastMet?.at
        ? `上次遇到：${ago(r.lastMet.at)} · ${queue(r.lastMet.queueId)} · ${r.lastMet.championName || ""}`
        : "第一次遇到",
    ]
      .filter(Boolean)
      .join("\n");
  }
  function chips(ref) {
    if (!shown() || !marked(ref)) return "";
    const ts = tags(records.get(ref));
    return `<span class="ptag-row" data-ptag-chips="${esc(ref)}" data-tooltip="${esc(tooltip(ref))}">${ts
      .slice(0, 2)
      .map(
        (t, i) =>
          `<span class="ptag is-${t.group}">${!i ? bookmark : ""}${esc(t.name)}</span>`,
      )
      .join(
        "",
      )}${ts.length > 2 ? `<span class="ptag is-more">+${ts.length - 2}</span>` : ""}${!ts.length ? '<span class="ptag is-mine">' + bookmark + "备注</span>" : ""}</span>`;
  }
  function button(ref, span = false) {
    return `<${span ? 'span role="button" tabindex="0"' : 'button type="button"'} class="${span ? (marked(ref) && shown() ? ({neg:"pmark is-neg",mine:"pmark is-mine",pos:"pmark is-pos"})[tone(records.get(ref))] : "pmark-add") : "ptag-btn" + (marked(ref) ? " is-tagged" : "")}" data-ptag-open="${esc(ref)}" aria-label="标记玩家"${marked(ref) && shown() ? ` data-tooltip="${esc(tooltip(ref))}"` : ""}>${bookmark}</${span ? "span" : "button"}>`;
  }
  function collect() {
    const blocked = new Set(
      (ctx()?.live()?.players || [])
        .filter((p) => p.taggable !== true || p.isCurrent)
        .map((p) => p.playerRef),
    );
    for (const p of ctx()?.sources() || []) if(p.isCurrent)blocked.add(p.playerRef);
    for (const ref of blocked) players.delete(ref);
    for (const p of ctx()?.sources() || []) {
      if (eligible(p) && !blocked.has(p.playerRef)) {
        players.set(p.playerRef, p);
      }
    }
    while (players.size > 10000) {
      const id = players.keys().next().value;
      if (id === current) break;
      players.delete(id);
      records.delete(id);
      queried.delete(id);
    }
  }
  function schedule() {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(refresh);
  }
  async function query() {
    if (request) return request;
    if (queryFailures >= 5 || Date.now() < queryRetryAt) return;
    const refs = [...players.keys()]
      .filter((ref) => !queried.has(ref))
      .slice(0, 200);
    if (!refs.length) return;
    refs.forEach((ref) => queried.add(ref));
    request = api({ op: "query", playerRefs: refs })
      .then((data) => {
        queryFailures = 0;
        catalog = data.catalog || catalog;
        version = data.version || 0;
        for (const ref of refs)
          if (!pending.has(ref))
            records.set(ref, normalize(data.records?.[ref]));
      })
      .catch(() => {
        refs.forEach((ref) => queried.delete(ref));
        queryFailures++;
        const delay = Math.min(60000, 2000 * 2 ** (queryFailures - 1));
        queryRetryAt = Date.now() + delay;
        if (queryFailures < 5) setTimeout(schedule, delay);
      })
      .finally(() => {
        request = null;
        schedule();
      });
    await request;
  }
  function decorate() {
    for (const target of document.querySelectorAll("[data-player-ref]")) {
      const ref = target.dataset.playerRef,
        p = players.get(ref);
      if (target.closest(".ptag-modal,.ptag-manage-card")) continue;
      if (!eligible(p)) {
        target.querySelectorAll(".pmark,.pmark-add,.ptag-btn").forEach((n) => n.remove());
        target.classList.remove("is-marked");
        delete target._ptagSignature;
        const card = target.closest(".live-player");
        if (card)
          card
            .querySelectorAll("[data-ptag-decoration]")
            .forEach((n) => n.remove());
        else
          (target.closest(".participant-identity") || target.parentElement)
            ?.querySelectorAll(":scope > [data-ptag-decoration]")
            .forEach((n) => n.remove());
        continue;
      }
      const signature = JSON.stringify([
        records.get(ref),
        shown(),
        catalog,
        ctx().name(p),
      ]);
      if (
        (target._ptagSignature === signature &&
          (target.querySelector("[data-ptag-open]") || target.closest(".match-team-list,.arena-team-roster"))) ||
        (target._ptagSignature === signature &&
          target.closest(".participant-identity")?.querySelector(
            "[data-ptag-decoration]",
          ))
      )
        continue;
      target._ptagSignature = signature;
      const roster = target.closest(".match-team-list,.arena-team-roster");
      if (roster) {
        target.querySelectorAll(".pmark,.pmark-add,.ptag-btn").forEach((n) => n.remove());
        target.classList.toggle("is-marked", shown() && marked(ref));
        const icon = target.querySelector(".game-icon");
        if (marked(ref) && shown() && icon) {
          let wrap = icon.closest(".pmark-wrap");
          if (!wrap) {
            wrap = document.createElement("span");
            wrap.className = "pmark-wrap";
            icon.replaceWith(wrap);
            wrap.append(icon);
          }
          wrap.insertAdjacentHTML("beforeend", button(ref, true));
        }
        continue;
      }
      const live = target.closest(".live-player"),
        detail = target.closest(".participant-link");
      if (live) {
        live
          .querySelectorAll("[data-ptag-decoration]")
          .forEach((n) => n.remove());
        live.insertAdjacentHTML(
          "beforeend",
          `<span data-ptag-decoration>${button(ref)}</span>`,
        );
        live
          .querySelector(".live-player-identity")
          ?.insertAdjacentHTML(
            "beforeend",
            `<span data-ptag-decoration>${chips(ref)}</span>`,
          );
      } else if (detail) {
        let cell=target.closest(".participant-identity");
        if(!cell){
          cell=document.createElement("div");cell.className="participant-identity";
          target.replaceWith(cell);cell.append(target);
          const line=document.createElement("div");line.className="ptag-name-line";target.replaceWith(line);line.append(target);
        }
        if(!cell.querySelector(".ptag-name-line")){const line=document.createElement("div");line.className="ptag-name-line";target.replaceWith(line);line.append(target);}
        cell.querySelectorAll("[data-ptag-decoration]").forEach(n=>n.remove());
        cell.querySelector(".ptag-name-line").insertAdjacentHTML("beforeend",`<span class="ptag-detail is-${tone(records.get(ref))}" data-ptag-decoration>${button(ref)}</span>`);
        const equipment=cell.querySelector(".participant-subline")||target.closest(".arena-detail-player")?.querySelector(".participant-subline");
        equipment?.querySelectorAll("[data-ptag-decoration]").forEach(n=>n.remove());
        (equipment||cell).insertAdjacentHTML("beforeend",`<span class="ptag-detail-tags" data-ptag-decoration>${chips(ref)}</span>`);
      }
    }
    for (const strip of document.querySelectorAll(".summoner-strip")) {
      const p = (ctx()?.sources() || []).find(
        (p) =>
          eligible(p) &&
          strip
            .closest("#player-overlay-content")
            ?.querySelector(`[data-player-ref="${CSS.escape(p.playerRef)}"]`),
      );
      const main = ctx()?.overviewPlayer?.(strip) || p;
      strip.querySelectorAll(".ptag-inline").forEach((n) => n.remove());
      if (!eligible(main)) continue;
      strip
        .querySelector(".summoner-level-row > span")
        ?.insertAdjacentHTML(
          "afterend",
          `<span class="ptag-inline">${chips(main.playerRef)}${button(main.playerRef)}</span>`,
        );
    }
    const live = ctx()?.live(),
      visible = (live?.players || []).filter(
        (p) => eligible(p) && marked(p.playerRef),
      );
    document.querySelectorAll(".ptag-notice").forEach((n) => n.remove());
    if (shown() && visible.length) {
      const notice = document.createElement("p");
      notice.className = "ptag-notice";
      notice.innerHTML =
        bookmark +
        `<span>本局有 <b>${visible.length}</b> 位你标记过的玩家：${visible.map((p) => `<button type="button" data-ptag-scroll="${esc(p.playerRef)}">${esc(ctx().name(p))}（${esc(tags(records.get(p.playerRef))[0]?.name || "备注")}）</button>`).join("、")}</span>`;
      document.querySelector("#live-content .live-teams")?.before(notice);
    }
  }
  function focusRef(ref) {
    [...document.querySelectorAll(`[data-ptag-open="${CSS.escape(ref)}"]`)]
      .find((n) => n.getClientRects().length)
      ?.focus();
  }
  async function refresh() {
    scheduled = false;
    if (!ctx()) return;
    collect();
    const focused = document.activeElement?.dataset.ptagOpen;
    observer?.disconnect();
    decorate();
    updateModalInfo();
    if (focused && !document.activeElement?.dataset.ptagOpen) focusRef(focused);
    observer?.observe(
      document.getElementById("main-content") || document.body,
      { childList: true, subtree: true },
    );
    if ([...players.keys()].some((ref) => !queried.has(ref))) void query();
  }
  function changed(ref) {
    version++;
    schedule();
    window.dispatchEvent(
      new CustomEvent("deep-legends:player-tags-changed", {
        detail: { playerRef: ref, version },
      }),
    );
  }
  function set(ref, value) {
    cancelled.delete(ref);
    edits.set(ref, {
      ...records.get(ref),
      ...value,
      createdAt: records.get(ref)?.createdAt || Date.now(),
      updatedAt: Date.now(),
    });
    pending.set(ref, { tags: [...(value.tags || [])], note: value.note || "" });
    saveError = "";
    clearTimeout(timer);
    timer = setTimeout(() => void flush().catch(() => {}), 300);
    status("正在保存…");
  }
  function status(copy) {
    modal
      ?.querySelector(".saved")
      ?.replaceChildren(document.createTextNode(copy));
  }
  async function flush() {
    clearTimeout(timer);
    const batch=[...pending];pending.clear();
    saveTail=saveTail.catch(()=>{}).then(async()=>{
      let failure;
      for(const [ref,value] of batch){
        if(cancelled.has(ref))continue;
        try{
          await api({op:"update",playerRef:ref,...value});
          records.set(ref,{...records.get(ref),...value});
          if(!pending.has(ref))edits.delete(ref);
          saveError="";changed(ref);
        }catch(e){
          if(!cancelled.has(ref)&&![400,404].includes(e.status)){if(!pending.has(ref))pending.set(ref,value);}
          else {pending.delete(ref);edits.delete(ref);}
          saveError=e.message;failure=e;
          if(modal&&current===ref){renderModal();status(saveError);}
        }
      }
      if(failure)throw failure;
    });
    try{await saveTail;status("✓ 已保存");}catch(e){status(e.message);throw e;}
  }
  function close() {
    clearTimeout(timer);
    for(const ref of edits.keys()){cancelled.add(ref);pending.delete(ref);}
    edits.clear();schedule();
    modal?.parentElement.remove();modal=null;
    if(openerLive&&!opener?.getClientRects().length)ctx().revealLive();
    opener?.isConnected&&opener.getClientRects().length?opener.focus():focusRef(openerRef);
  }
  function updateModalInfo() {
    if (!modal) return;
    const p = players.get(current);
    if (!p) return;
    modal.querySelector(".who strong").innerHTML = "标记 <span class=\"ptag-player-name\">" + esc(ctx().name(p)) + "</span>";
    modal.querySelector(".who small").textContent = [
      ctx().info(p),
      records.get(current)?.lastMet?.at
        ? "上次遇到 " + ago(records.get(current).lastMet.at)
        : "第一次遇到",
    ]
      .filter(Boolean)
      .join(" · ");
    modal.querySelector(".avatar").innerHTML = ctx().avatar(p);
  }
  function renderModal() {
    const record = edits.get(current) || records.get(current) || empty();
    modal.innerHTML = `<header><span class="avatar"></span><div class="who"><strong></strong><small></small></div><div class="tools">${sequence.length > 1 ? `<button type="button" data-ptag-prev aria-label="上一位">‹</button><i>${sequence.indexOf(current) + 1} / ${sequence.length}</i><button type="button" data-ptag-next aria-label="下一位">›</button>` : ""}<button type="button" class="dialog-close-button control-icon-button" data-ptag-close aria-label="关闭"><svg class="control-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg></button></div></header>${[
      ["neg", "负面（来自客户端举报项）"],
      ["pos", "正面"],
      ["mine", "我的标签"],
    ]
      .map(
        ([g, title]) =>
          `<section class="ptag-group"><span>${title}</span><div class="ptag-options">${catalog
            .filter((t) => t.group === g && !t.hidden)
            .map(
              (t) =>
                `<button type="button" class="ptag-option is-${g}${record.tags.includes(t.id) ? " is-on" : ""}" data-ptag-toggle="${esc(t.id)}" aria-pressed="${record.tags.includes(t.id)}">${esc(t.name)}</button>`,
            )
            .join(
              "",
            )}${g === "mine" ? '<button type="button" class="ptag-option is-add" data-ptag-new>＋ 新标签</button>' : ""}</div></section>`,
      )
      .join(
        "",
      )}<label class="ptag-note"><span>备注</span><input aria-label="备注" maxlength="80" placeholder="可选，最多40字" value="${esc(record.note)}"></label><footer><span class="saved" role="status">${pending.size ? "正在保存…" : saveError ? esc(saveError) : "✓ 已保存"}</span><div><button type="button" class="text-button clear" data-ptag-clear>清除标记</button><button type="button" class="done" data-ptag-close>完成</button></div></footer>`;
    updateModalInfo();
    modal.querySelector("input").oninput = (e) => {
      const value = [...e.target.value].slice(0, 40).join("");
      e.target.value = value;
      set(current, { ...(edits.get(current) || records.get(current)), note: value });
    };
    modal.querySelector("[data-ptag-toggle],input,[data-ptag-close]")?.focus();
  }
  async function open(ref, trigger) {
    collect();
    if (!eligible(players.get(ref))) return;
    try {
      queryFailures = 0;
      queryRetryAt = 0;
      await query();
      if (!records.has(ref)) {
        const data = await api({ op: "query", playerRefs: [ref] });
        catalog = data.catalog;
        records.set(ref, normalize(data.records?.[ref]));
        queried.add(ref);
      }
      if (modal) await flush();
    } catch (e) {
      toast(e.message);
      return;
    }
    opener = trigger;
    openerRef = ref;
    openerLive = !!trigger?.closest(".live-player");
    const live = trigger?.closest(".live-player"),
      detail = trigger?.closest(".match-detail");
    sequence = live
      ? (ctx().live()?.players || []).filter(eligible).map((p) => p.playerRef)
      : detail
        ? [
            ...new Set(
              [...detail.querySelectorAll("[data-player-ref]")]
                .map((n) => n.dataset.playerRef)
                .filter((r) => eligible(players.get(r))),
            ),
          ]
        : [];
    current = ref;
    if (!modal) {
      const scrim = document.createElement("div");
      scrim.className = "ptag-scrim";
      modal = document.createElement("section");
      modal.className = "ptag-modal";
      modal.setAttribute("role", "dialog");
      modal.setAttribute("aria-modal", "true");
      modal.setAttribute("aria-label", "标记玩家");
      scrim.append(modal);
      document.body.append(scrim);
      scrim.onmousedown = (e) => {
        if (e.target === scrim) void close();
      };
      scrim.addEventListener("keydown", key);
    }
    renderModal();
  }
  async function move(delta) {
    if (sequence.length < 2) return;
    try {
      await flush();
      current =
        sequence[
          (sequence.indexOf(current) + delta + sequence.length) %
            sequence.length
        ];
      renderModal();
    } catch {}
  }
  function key(e) {
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      void close();
      return;
    }
    if (e.key === "Tab") {
      const list = [...modal.querySelectorAll("button:not(:disabled),input")];
      if (e.shiftKey && document.activeElement === list[0]) {
        e.preventDefault();
        list.at(-1).focus();
      } else if (!e.shiftKey && document.activeElement === list.at(-1)) {
        e.preventDefault();
        list[0].focus();
      }
      return;
    }
    if (e.target.matches("input,textarea,select")) return;
    if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
      e.preventDefault();
      void move(e.key === "ArrowLeft" ? -1 : 1);
    } else if (/^[1-9]$/.test(e.key)) {
      e.preventDefault();
      modal.querySelectorAll("[data-ptag-toggle]")[Number(e.key) - 1]?.click();
    }
  }
  function undo(ref, old) {
    document.querySelector(".ptag-undo")?.remove();
    const n = document.createElement("div");
    n.className = "ptag-undo";
    n.innerHTML = '<span>已清除标记</span><button type="button">撤销</button>';
    document.body.append(n);
    const timeout = setTimeout(() => n.remove(), 5000);
    n.querySelector("button").onclick = () => {
      clearTimeout(timeout);
      set(ref, old);
      if (modal && current === ref) renderModal();
      n.remove();
    };
  }
  function newTag(select) {
    const overlay = document.createElement("div");
    overlay.className = "ptag-create";
    overlay.innerHTML =
      '<form><strong>新标签</strong><input name="name" aria-label="标签名称" placeholder="最多6个汉字或12个字符" required><select name="group" aria-label="颜色组"><option value="mine">自定义</option><option value="neg">负面</option><option value="pos">正面</option></select><div><button type="button" class="text-button" data-cancel>取消</button><button type="submit" class="primary">新建</button></div><small role="alert"></small></form>';
    document.body.append(overlay);
    const form = overlay.querySelector("form");
    form.elements.name.focus();
    overlay.querySelector("[data-cancel]").onclick = () => {
      overlay.remove();
      modal?.querySelector("[data-ptag-new]")?.focus();
    };
    overlay.onkeydown = (e) => {
      e.stopPropagation();
      if (e.key === "Escape") overlay.querySelector("[data-cancel]").click();
    };
    form.onsubmit = async (e) => {
      e.preventDefault();
      const t = {
        id: "custom-" + crypto.randomUUID(),
        name: form.elements.name.value.trim(),
        group: form.elements.group.value,
        preset: false,
        hidden: false,
      };
      try {
        await api({ op: "catalog", catalog: [...catalog, t] });
        catalog.push(t);
        if (select)
          set(current, {
            ...(edits.get(current) || records.get(current)),
            tags: [...((edits.get(current) || records.get(current))?.tags || []), t.id],
          });
        overlay.remove();
        if (modal) renderModal();
        renderSettings();
        changed(current);
      } catch (e) {
        form.querySelector("[role=alert]").textContent = e.message;
      }
    };
  }
  async function loadAll() {
    const data = await api();
    catalog = data.catalog || [];
    version = data.version;
    records.clear();
    queried.clear();
    for (const row of data.players || []) {
      const p = { ...row.nameSnapshot, playerRef: row.playerRef };
      players.set(p.playerRef, p);
      records.set(p.playerRef, normalize(row));
      queried.add(p.playerRef);
    }
    return data;
  }
  function renderSettings() {
    const root = document.getElementById("player-tags-settings");
    if (!root) return;
    const rows = [...records].filter(([r]) => marked(r)&&eligible(players.get(r)));
    const usages = (id) => rows.filter(([, r]) => r.tags.includes(id)).length;
    root.innerHTML = `<nav class="ptag-subtabs" role="tablist" aria-label="玩家标签"><button type="button" role="tab" aria-selected="${settingsTab === "tags"}" data-ptag-tab="tags">标签管理</button><button type="button" role="tab" aria-selected="${settingsTab === "players"}" data-ptag-tab="players">已标记玩家</button></nav>`;
    if (settingsTab === "tags")
      root.insertAdjacentHTML(
        "beforeend",
        `<section class="setting-card ptag-manage-card"><header><div><h3>快捷标签</h3><p>对局中点一下就能加的标签。隐藏只影响标记窗口，已打上的标签不受影响。</p></div><button type="button" class="text-button" data-ptag-reset>恢复默认</button></header><div class="ptag-manage-groups">${[
          ["neg", "负面 · 来自客户端举报项"],
          ["pos", "正面"],
          ["mine", "我的标签 · 最多12个"],
        ]
          .map(
            ([g, title]) =>
              `<section><h4>${title}</h4><div class="ptag-options">${catalog
                .filter((t) => t.group === g)
                .map(
                  (t) =>
                    `<span class="ptag-chip-edit is-${g}${t.hidden ? " is-off" : ""}"><i></i>${esc(t.name)}<small>${usages(t.id) || ""}</small><button type="button" data-ptag-hide="${esc(t.id)}" aria-label="${t.hidden ? "显示" : "隐藏"} ${esc(t.name)}" aria-pressed="${!t.hidden}">${eye}</button>${!t.preset ? `<button type="button" data-ptag-delete="${esc(t.id)}" aria-label="删除 ${esc(t.name)}">×</button>` : ""}</span>`,
                )
                .join(
                  "",
                )}${g === "mine" ? '<button type="button" class="ptag-option is-add" data-ptag-new>＋ 新标签</button>' : ""}</div></section>`,
          )
          .join(
            "",
          )}</div></section><section class="setting-card ptag-manage-card"><h3>显示我的标签</h3><p>关闭后卡片、战绩名单与本局提示不显示标签，标记按钮仍可用。</p><label class="toggle-control"><input type="checkbox" data-ptag-visible ${shown() ? "checked" : ""}><span>显示我的标签</span></label></section>`,
      );
    else {
      const list = rows
        .filter(([ref, r]) => {
          const p = players.get(ref);
          return (
            (!search ||
              (ctx().name(p) + " " + r.note)
                .toLowerCase()
                .includes(search.toLowerCase())) &&
            (!filter || r.tags.includes(filter))
          );
        })
        .sort(
          (a, b) =>
            Number(sort === "met" ? b[1].lastMet?.at : b[1].updatedAt) -
            Number(sort === "met" ? a[1].lastMet?.at : a[1].updatedAt),
        );
      page = Math.min(page, Math.max(0, Math.ceil(list.length / 40) - 1));
      root.insertAdjacentHTML(
        "beforeend",
        `<section class="setting-card ptag-manage-card"><header><div><h3>已标记的玩家 <small>共 ${rows.length} 位</small></h3><p>只保存在这台电脑上。隐私遮罩同样生效。</p></div><div class="setting-actions"><button type="button" class="text-button" data-ptag-export>导出</button><button type="button" class="text-button" data-ptag-import>导入</button></div></header><div class="ptag-list-tools"><label class="ptag-search"><input aria-label="搜索名字或备注" data-ptag-search value="${esc(search)}" placeholder="搜索名字或备注"></label><select class="ptag-select" aria-label="按标签筛选" data-ptag-filter><option value="">全部标签</option>${catalog.map((t) => `<option value="${esc(t.id)}" ${filter === t.id ? "selected" : ""}>${esc(t.name)}</option>`).join("")}</select><select class="ptag-select" aria-label="排序" data-ptag-sort><option value="met" ${sort === "met" ? "selected" : ""}>最近遇到</option><option value="mark" ${sort === "mark" ? "selected" : ""}>最近标记</option></select></div><div class="ptag-list">${
          list
            .slice(page * 40, page * 40 + 40)
            .map(
              ([ref, r]) =>
                `<article class="ptag-item"><span class="avatar">${ctx().avatar(players.get(ref))}</span><div class="who"><button type="button" data-ptag-overview="${esc(ref)}">${esc(ctx().name(players.get(ref)))}</button><small>标记于 ${date(r.createdAt)}</small></div><div class="tags">${tags(
                  r,
                )
                  .map(
                    (t) =>
                      `<span class="ptag is-${t.group}">${esc(t.name)}</span>`,
                  )
                  .join(
                    "",
                  )}<em>${esc(r.note)}</em></div><div class="met">上次遇到 ${ago(r.lastMet?.at)}<br>${r.lastMet?.at ? esc(queue(r.lastMet.queueId) + " · " + r.lastMet.championName) : ""}</div><div class="acts"><button type="button" data-ptag-open="${esc(ref)}" aria-label="编辑标记">✎</button><button type="button" data-ptag-remove="${esc(ref)}" aria-label="删除玩家标记">×</button></div></article>`,
            )
            .join("") || '<p class="section-empty">暂无标记</p>'
        }</div><div class="ptag-pager"><button type="button" class="text-button" data-ptag-page="-1" ${page ? "" : "disabled"}>上一页</button><span>${page + 1} / ${Math.max(1, Math.ceil(list.length / 40))}</span><button type="button" class="text-button" data-ptag-page="1" ${(page + 1) * 40 >= list.length ? "disabled" : ""}>下一页</button></div></section>`,
      );
      root.querySelector("[data-ptag-search]").oninput = (e) => {
        search = e.target.value;
        page = 0;
        const at = e.target.selectionStart;
        renderSettings();
        const input = root.querySelector("[data-ptag-search]");
        input.focus();
        input.setSelectionRange(at, at);
      };
      for (const [selector, update] of [
        ["[data-ptag-filter]", (v) => (filter = v)],
        ["[data-ptag-sort]", (v) => (sort = v)],
      ])
        root.querySelector(selector).onchange = (e) => {
          update(e.target.value);
          page = 0;
          renderSettings();
        };
    }
    root
      .querySelector("[data-ptag-visible]")
      ?.addEventListener("change", (e) => {
        (window.deepLegendsPreferenceStorage || localStorage).setItem(
          "lol-loot-mytags",
          String(e.target.checked),
        );
        changed("");
      });
  }
  async function updateCatalog(next) {
    await flush();
    await api({ op: "catalog", catalog: next });
    catalog = next;
    const ids = new Set(next.map((t) => t.id));
    for (const [ref, r] of records) {
      records.set(ref, { ...r, tags: r.tags.filter((id) => ids.has(id)) });
    }
    renderSettings();
    changed("");
  }
  async function importFile() {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".json,application/json";
    input.onchange = async () => {
      try {
        const file = input.files[0];
        if (!file || file.size > 8 * 1024 * 1024) throw Error("文件过大");
        const data = JSON.parse(await file.text()),
          preview = await api({ op: "import", import: data, commit: false });
        if (
          !(await confirmAction(
            `将新增 ${preview.added} 位、更新 ${preview.updated} 位玩家标记，是否导入？`,
          ))
        )
          return;
        await api({ op: "import", import: data, commit: true });
        await loadAll();
        renderSettings();
        changed("");
        toast("已导入玩家标签");
      } catch (e) {
        toast(e.message);
      }
    };
    input.click();
  }
  function confirmAction(text) {
    return new Promise((resolve) => {
      const overlay = document.createElement("div");
      overlay.className = "ptag-create";
      overlay.innerHTML = `<form><p>${esc(text)}</p><div><button type="button" class="text-button" data-cancel>取消</button><button type="submit" class="primary">确认</button></div></form>`;
      document.body.append(overlay);
      const done = (value) => {
        overlay.remove();
        resolve(value);
      };
      overlay.querySelector("[data-cancel]").onclick = () => done(false);
      overlay.onkeydown = (e) => {
        e.stopPropagation();
        if (e.key === "Escape") done(false);
      };
      overlay.querySelector("form").onsubmit = (e) => {
        e.preventDefault();
        done(true);
      };
      overlay.querySelector("[data-cancel]").focus();
    });
  }
  document.addEventListener(
    "click",
    async (e) => {
      const b = e.target.closest(
        "[data-ptag-open],[data-ptag-toggle],[data-ptag-close],[data-ptag-prev],[data-ptag-next],[data-ptag-clear],[data-ptag-new],[data-ptag-tab],[data-ptag-hide],[data-ptag-delete],[data-ptag-reset],[data-ptag-remove],[data-ptag-overview],[data-ptag-page],[data-ptag-export],[data-ptag-import],[data-ptag-scroll]",
      );
      if (!b) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      try {
        if (b.hasAttribute("data-ptag-open")) {
          await open(b.dataset.ptagOpen, b);
        } else if (b.hasAttribute("data-ptag-toggle")) {
          const r = edits.get(current) || records.get(current) || empty(),
            id = b.dataset.ptagToggle;
          set(current, {
            ...r,
            tags: r.tags.includes(id)
              ? r.tags.filter((t) => t !== id)
              : [...r.tags, id],
          });
          b.classList.toggle("is-on");
          b.setAttribute("aria-pressed", b.classList.contains("is-on"));
        } else if (b.hasAttribute("data-ptag-close")) await close();
        else if (b.hasAttribute("data-ptag-prev")) await move(-1);
        else if (b.hasAttribute("data-ptag-next")) await move(1);
        else if (b.hasAttribute("data-ptag-clear")) {
          const old = records.get(current);
          set(current, empty());
          renderModal();
          undo(current, old);
        } else if (b.hasAttribute("data-ptag-new"))
          newTag(!!b.closest(".ptag-modal"));
        else if (b.hasAttribute("data-ptag-tab")) {
          settingsTab = b.dataset.ptagTab;
          page = 0;
          await loadAll();
          renderSettings();
        } else if (b.hasAttribute("data-ptag-hide"))
          await updateCatalog(
            catalog.map((t) =>
              t.id === b.dataset.ptagHide ? { ...t, hidden: !t.hidden } : t,
            ),
          );
        else if (b.hasAttribute("data-ptag-delete")) {
          if (await confirmAction("删除此标签，并从所有玩家标记中移除？"))
            await updateCatalog(
              catalog.filter((t) => t.id !== b.dataset.ptagDelete),
            );
        } else if (b.hasAttribute("data-ptag-reset"))
          await updateCatalog(
            catalog.map((t) => (t.preset ? { ...t, hidden: false } : t)),
          );
        else if (b.hasAttribute("data-ptag-remove")) {
          const ref = b.dataset.ptagRemove,
            old = records.get(ref);
          set(ref, empty());
          await flush();
          renderSettings();
          undo(ref, old);
        } else if (b.hasAttribute("data-ptag-overview"))
          ctx().open(players.get(b.dataset.ptagOverview));
        else if (b.hasAttribute("data-ptag-page")) {
          page += Number(b.dataset.ptagPage);
          renderSettings();
        } else if (b.hasAttribute("data-ptag-export")) {
          await flush();
          const a = document.createElement("a");
          a.href = "/api/player-tags?export=1";
          a.download = "player-tags.json";
          a.click();
        } else if (b.hasAttribute("data-ptag-import")) await importFile();
        else if (b.hasAttribute("data-ptag-scroll"))
          document
            .querySelector(
              `.live-player [data-player-ref="${CSS.escape(b.dataset.ptagScroll)}"]`,
            )
            ?.closest(".live-player")
            ?.scrollIntoView({
              block: "center",
              behavior: matchMedia("(prefers-reduced-motion:reduce)").matches
                ? "instant"
                : "smooth",
            });
      } catch (error) {
        toast(error.message);
      }
    },
    true,
  );
  document.addEventListener(
    "keydown",
    (e) => {
      const target = e.target.closest("span[data-ptag-open]");
      if (target && ["Enter", " "].includes(e.key)) {
        e.preventDefault();
        e.stopImmediatePropagation();
        target.click();
      }
    },
    true,
  );
  window.addEventListener("deep-legends:privacy-changed", () => {
    schedule();
    renderSettings();
  });
  window.addEventListener("pagehide", () => {
    for (const [ref, value] of pending)
      void fetch("/api/player-tags", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ op: "update", playerRef: ref, ...value }),
        keepalive: true,
      }).catch(() => {});
  });
  window.addEventListener("deep-legends:dispose",()=>{observer?.disconnect();clearTimeout(timer);},{once:true});
  window.deepLegendsPlayerTags = Object.freeze({
    signature: (match) =>
      (match.participants || [])
        .map((p) => JSON.stringify(records.get(p.playerRef) || null))
        .join("|"),
    refresh: schedule,
    flush,
  });
  function initialize() {
    if (!initialized) {
      initialized = true;
      observer = new MutationObserver(schedule);
      schedule();
      const panel = document.getElementById("settings-player-tags-panel");
      if (panel)
        new MutationObserver(() => {
          if (!panel.hidden) {
            settingsTab = "tags";
            void loadAll()
              .then(renderSettings)
              .catch((e) => toast(e.message));
          }
        }).observe(panel, { attributes: true, attributeFilter: ["hidden"] });
      if (panel && !panel.hidden)
        void loadAll()
          .then(renderSettings)
          .catch((e) => toast(e.message));
    }
  }
  window.deepLegendsSections.register("player-tags", initialize);
  initialize();
})();
