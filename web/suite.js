(() => {
  "use strict";

  const panel = document.getElementById("suite-panel");
  if (!panel) return;

  const tabs = [...panel.querySelectorAll("[data-suite-tab]")];
  const panels = [...panel.querySelectorAll("[data-suite-panel]")];
  const roots = {
    watch: document.getElementById("suite-watch-root"),
    rig: document.getElementById("suite-rig-root"),
    facade: document.getElementById("suite-facade-root"),
    sweep: document.getElementById("suite-sweep-root"),
  };
  const metrics = {
    watch: document.getElementById("suite-watch-metric"),
    rig: document.getElementById("suite-rig-metric"),
    claim: document.getElementById("suite-claim-metric"),
  };
  const offline = document.getElementById("suite-offline");
  const appScroll = document.getElementById("app-scroll");
  const subtitle = document.getElementById("topbar-subtitle");

  const tabCopy = {
    watch: "托管客户端流程",
    rig: "游戏端与客户端维护",
    facade: "生涯展示与聊天身份",
    sweep: "未领取奖励清扫",
  };
  const phaseOrder = ["Lobby", "Matchmaking", "ReadyCheck", "ChampSelect", "InProgress", "EndOfGame"];
  const phaseNames = { Lobby: "房间", Matchmaking: "匹配中", ReadyCheck: "确认对局", ChampSelect: "英雄选择", InProgress: "游戏中", Reconnect: "重连", EndOfGame: "结算" };
  const sourceNames = { grant: "奖励账本", mission: "任务", event: "事件中心" };
  const state = {
    tab: readPreference("suite-tab", "watch"),
    scroll: readJSONPreference("suite-scroll", {}),
    connected: null,
    active: false,
    loading: false,
    watch: null,
    rig: null,
    facade: null,
    facadeDraft: null,
    claims: null,
    phase: document.body.classList.contains("is-demo") ? "ReadyCheck" : "",
    watchEvents: new Map(),
    watchFired: 0,
    watchPriority: false,
    claimFilter: "all",
    selectedClaims: new Set(),
    claimChoices: new Map(),
    claimFailures: new Map(),
    claimProgress: { done: 0, total: 0, failed: 0 },
    claiming: false,
  };

  function readPreference(key, fallback) {
    try { return localStorage.getItem(`lol-loot-${key}`) || fallback; } catch (_) { return fallback; }
  }

  function writePreference(key, value) {
    try { localStorage.setItem(`lol-loot-${key}`, String(value)); } catch (_) {}
  }

  function readJSONPreference(key, fallback) {
    try { return JSON.parse(localStorage.getItem(`lol-loot-${key}`) || "null") || fallback; } catch (_) { return fallback; }
  }

  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>'"]/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[character]);
  }

  function toast(message) {
    if (typeof window.deepLegendsToast === "function") window.deepLegendsToast(message);
  }

  async function api(path, options = {}) {
    const response = await fetch(path, {
      ...options,
      headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    });
    const text = await response.text();
    let payload = null;
    try { payload = text ? JSON.parse(text) : {}; } catch (_) { payload = null; }
    if (!response.ok) throw new Error(payload?.message || text.trim() || `请求失败（${response.status}）`);
    return payload ?? {};
  }

  function imageURL(path) {
    const value = String(path || "");
    return value ? `/api/image?path=${encodeURIComponent(value)}` : "/image-unavailable.svg";
  }

  function setConnected(connected) {
    state.connected = Boolean(connected);
    offline.hidden = state.connected;
    for (const subpanel of panels) subpanel.hidden = !state.connected || subpanel.dataset.suitePanel !== state.tab;
    if (!state.connected) {
      metrics.rig.textContent = "未连接";
      metrics.claim.textContent = "—";
    }
  }

  function activateTab(name, focus = false) {
    if (!tabCopy[name]) name = "watch";
    if (state.tab !== name) {
      state.scroll[state.tab] = Number(appScroll?.scrollTop || 0);
      writePreference("suite-scroll", JSON.stringify(state.scroll));
    }
    state.tab = name;
    writePreference("suite-tab", name);
    for (const tab of tabs) {
      const active = tab.dataset.suiteTab === name;
      tab.classList.toggle("is-active", active);
      tab.setAttribute("aria-selected", String(active));
      tab.tabIndex = active ? 0 : -1;
      if (active && focus) tab.focus();
    }
    for (const subpanel of panels) subpanel.hidden = !state.connected || subpanel.dataset.suitePanel !== name;
    if (state.active && subtitle) subtitle.textContent = tabCopy[name];
    requestAnimationFrame(() => appScroll?.scrollTo({ top: Number(state.scroll[name] || 0), behavior: "instant" }));
  }

  function setupTabs() {
    for (const tab of tabs) {
      tab.addEventListener("click", () => activateTab(tab.dataset.suiteTab));
      tab.addEventListener("keydown", (event) => {
        if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const current = tabs.indexOf(tab);
        const forward = event.key === "ArrowRight" || event.key === "ArrowDown";
        const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (forward ? 1 : -1) + tabs.length) % tabs.length;
        activateTab(tabs[next].dataset.suiteTab, true);
      });
    }
    activateTab(state.tab);
  }

  async function loadAll(force = false) {
    if (state.loading && !force) return;
    state.loading = true;
    const tasks = [
      loadWatch(force),
      loadRig(force),
      loadFacade(force),
      loadClaims(force),
    ];
    await Promise.allSettled(tasks);
    state.loading = false;
  }

  const watchDefinitions = [
    { key: "autoAccept", action: "accept", group: "排队与房间", title: "自动接受对局", phase: "确认对局 · ReadyCheck", phaseKey: "ReadyCheck", description: "匹配到对局时替你点“接受”。延时设为 0 会在毫秒内响应，建议留 1 - 2 秒更接近手动节奏。", control: "delay", min: 0, max: 10000, step: 100, countdownVerb: "接受" },
    { key: "autoReconnect", action: "reconnect", group: "英雄选择与游戏中", title: "断线自动重连", phase: "掉线 · Reconnect", phaseKey: "Reconnect", description: "检测到掉线状态时自动重连正在进行的对局。只尝试一次，失败后交还给你手动处理。", control: "delay", min: 3000, max: 30000, step: 500, countdownVerb: "重连" },
    { key: "autoPlayAgain", action: "play-again", group: "结算", title: "快速下一把", phase: "结算 · EndOfGame", phaseKey: "EndOfGame", description: "结算后自动回到房间。若同时开启了自动点赞，会等点赞投票完成再返回。", control: "fixed-delay", displayDelayMs: 1575, countdownVerb: "返回" },
    { key: "autoHonor", action: "auto-honor", group: "结算", title: "结算自动点赞", phase: "结算 · 出现点赞票", phaseKey: "EndOfGame", description: "把点赞票投给同一房间的队友。找不到队友时按下方策略处理，任何情况下都不会投给敌方。", control: "honor", countdownVerb: "点赞" },
    { key: "skipCelebration", action: "skip-celebration", group: "结算", title: "跳过任务庆祝", phase: "结算前 · PreEndOfGame", phaseKey: "EndOfGame", description: "结算前的任务庆祝动画会拖慢回到房间的速度，开启后自动跳过这一段。", countdownVerb: "跳过" },
    { key: "positionBroadcast", action: "position-broadcast", group: "英雄选择与游戏中", title: "阵营位置播报", phase: "英雄选择 · 大乱斗类模式", phaseKey: "ChampSelect", description: "在极地大乱斗、海克斯大乱斗里告诉你当前是蓝色方还是红色方。", control: "visibility", countdownVerb: "播报" },
    { key: "promoteLeader", action: "promote-leader", group: "排队与房间", title: "房主自动转交", phase: "房间 · 自己是房主", phaseKey: "Lobby", description: "组队时把房主让给别人。优先转给已准备的成员，全员未准备时才随机挑一个。", countdownVerb: "转交" },
    { key: "invitations", action: "invitations", group: "排队与房间", title: "房间邀请处理", phase: "收到邀请", phaseKey: "Lobby", description: "按队列类型分别设定接受 / 拒绝 / 不处理。默认全部“不处理”，需要你逐个队列开。", control: "invitations", countdownVerb: "处理" },
    { key: "autoMatchmaking", action: "auto-matchmaking", group: "排队与房间", title: "自动开始匹配", phase: "房间 · 满足人数且可开始", phaseKey: "Lobby", description: "作为房主时自动开始匹配。不提供“排队超时自动重排”，避免形成无人值守的循环排队。", control: "matchmaking", countdownVerb: "匹配" },
  ];

  const queuePolicies = [
    ["420", "单双排"], ["440", "灵活组排"], ["450", "极地大乱斗"],
    ["400", "匹配征召"], ["430", "匹配自选"], ["490", "快速模式"],
    ["1700", "斗魂竞技场"], ["1090", "云顶普通"], ["default", "其他队列"],
  ];

  function watchRuleControl(definition, rule) {
    if (definition.control === "delay") {
      return `<div class="watch-delay-control"><span>延时</span><input class="suite-range" type="range" min="${definition.min}" max="${definition.max}" step="${definition.step}" value="${Number(rule.delayMs || 0)}" data-watch-delay="${definition.key}" aria-label="${escapeHTML(definition.title)}延时"><output class="suite-chip watch-delay-value" data-watch-output="${definition.key}">${formatDelay(rule.delayMs)}</output></div>`;
    }
    if (definition.control === "fixed-delay") {
      return `<div class="watch-delay-control" aria-label="${escapeHTML(definition.title)}固定等待 ${formatDelay(definition.displayDelayMs)}"><span>延时</span><span class="watch-delay-meter" data-watch-meter="22" aria-hidden="true"></span><span class="suite-chip watch-delay-value">${formatDelay(definition.displayDelayMs)}</span></div>`;
    }
    if (definition.control === "honor") {
      return watchChoiceButtons("autoHonor.strategy", rule.strategy, [["prefer-party", "优先房间队友"], ["party-only", "仅房间队友"], ["any-teammate", "任意队友"], ["abstain", "弃票"]], "点赞策略");
    }
    if (definition.control === "visibility") {
      return watchChoiceButtons("positionBroadcast.visibility", rule.visibility, [["self", "仅自己可见"], ["team", "发到队伍频道"]], "播报范围");
    }
    if (definition.control === "matchmaking") {
      return `<div class="watch-parameter-fields"><label><span>最少人数</span><select class="suite-select watch-compact-select" data-watch-number="autoMatchmaking.minPartySize">${[1,2,3,4,5].map((value) => `<option value="${value}"${selected(Number(rule.minPartySize), value)}>${value}</option>`).join("")}</select></label><label><span>延时</span><select class="suite-select watch-compact-select" data-watch-number="autoMatchmaking.delayMs">${[0,1000,3000,5000,10000,15000,30000].map((value) => `<option value="${value}"${selected(Number(rule.delayMs), value)}>${formatDelay(value)}</option>`).join("")}</select></label></div>`;
    }
    if (definition.control === "invitations") {
      const policies = rule.policies || {};
      const accepted = queuePolicies.filter(([key]) => invitationPolicy(policies, key) === "accept");
      const summary = accepted.concat(queuePolicies.filter(([key]) => invitationPolicy(policies, key) !== "accept").slice(0, Math.max(0, 3 - accepted.length)));
      const summaryKeys = new Set(summary.map(([key]) => key));
      const hidden = queuePolicies.filter(([key]) => !summaryKeys.has(key));
      const more = hidden.length ? `<details class="watch-invite-more"><summary>+ ${hidden.length} 项</summary><div>${hidden.map(([key, label]) => invitationPolicyButton(key, label, policies)).join("")}</div></details>` : "";
      return `<div class="watch-invite-summary">${summary.map(([key, label]) => invitationPolicyButton(key, label, policies)).join("")}${more}</div>`;
    }
    return "";
  }

  function watchChoiceButtons(path, current, options, label) {
    return `<div class="watch-pills" role="group" aria-label="${escapeHTML(label)}">${options.map(([value, copy]) => `<button class="watch-pill${current === value ? " is-active" : ""}" type="button" aria-pressed="${current === value}" data-watch-choice="${path}" data-watch-choice-value="${value}">${copy}</button>`).join("")}</div>`;
  }

  function invitationPolicy(policies, key) {
    return policies[key] || policies.default || "ignore";
  }

  function invitationPolicyButton(key, label, policies) {
    const policy = invitationPolicy(policies, key);
    const copy = { ignore: "不处理", accept: "接受", decline: "拒绝" }[policy] || "不处理";
    return `<button class="watch-pill${policy === "accept" ? " is-accept" : policy === "decline" ? " is-decline" : ""}" type="button" data-watch-policy-cycle="${escapeHTML(key)}" data-tooltip="点击切换：不处理 / 接受 / 拒绝">${escapeHTML(label)} · ${copy}</button>`;
  }

  function selected(value, expected) { return value === expected ? " selected" : ""; }
  function checked(value) { return value ? " checked" : ""; }
  function formatDelay(value) { return `${(Number(value || 0) / 1000).toFixed(Number(value || 0) % 1000 ? 1 : 0)} s`; }

  function renderWatch() {
    const settings = state.watch;
    if (!settings?.rules) return;
    const enabled = watchDefinitions.filter((definition) => settings.rules[definition.key]?.enabled).length;
    metrics.watch.textContent = `${enabled} / ${watchDefinitions.length}`;
    const groups = ["排队与房间", "英雄选择与游戏中", "结算"];
    roots.watch.className = "";
    roots.watch.innerHTML = `
      <section class="suite-card watch-hero">
        <div class="watch-hero-copy"><span class="watch-sigil" aria-hidden="true">◎</span><div><h2>${settings.masterEnabled ? "值守中" : "值守已暂停"}</h2><p>${settings.masterEnabled ? "客户端已连接，规则会在对应阶段自动触发。" : "规则配置仍会保存，但总开关关闭时不会写入客户端。"}</p></div></div>
        <div class="watch-summary"><div><small>本次启动已代劳</small><strong>${state.watchFired} 次</strong></div><div><small>最近一次</small><strong>${escapeHTML(latestWatchAction())}</strong></div><label class="suite-switch"><input type="checkbox" data-watch-master${checked(settings.masterEnabled)}><span>总开关</span></label></div>
      </section>
      <div class="phase-track" aria-label="客户端相位">${phaseOrder.map((phase, index) => {
        const current = phase === state.phase;
        const currentIndex = phaseOrder.indexOf(state.phase);
        return `<div class="phase-node${current ? " is-current" : ""}${currentIndex > index ? " is-complete" : ""}" data-watch-phase="${phase}"><i aria-hidden="true"></i><span>${phaseNames[phase]}</span></div>`;
      }).join("")}</div>
      ${state.watchPriority ? '<div class="suite-note is-warning watch-priority"><span aria-hidden="true">!</span><span><strong>点赞中 · 下一把已顺延</strong><br>点赞完成后才会执行快速下一把，两条规则不会互抢。</span></div>' : ""}
      <header class="watch-rules-head"><div><h2>规则</h2><p>每条规则挂在它会触发的阶段上；当前阶段的规则会高亮并显示倒计时</p></div><span class="suite-chip is-warning">${enabled} 条已启用</span></header>
      ${groups.map((group) => `<section class="watch-group"><header><h3>${group}</h3></header><div class="watch-rule-grid">${watchDefinitions.filter((definition) => definition.group === group).map((definition) => watchRuleCard(definition, settings.rules[definition.key] || {})).join("")}</div></section>`).join("")}`;
    bindWatchControls();
    applyWatchControlStyles();
    updateCountdowns();
  }

  function latestWatchAction() {
    let latest = null;
    for (const [action, event] of state.watchEvents) {
      if (event.kind !== "fired" || (latest && latest.event.start >= event.start)) continue;
      latest = { action, event };
    }
    if (!latest) return "暂无触发";
    const title = watchDefinitions.find((definition) => definition.action === latest.action)?.title || latest.action;
    return `${title} · 刚刚`;
  }

  function watchRuleCard(definition, rule) {
    const event = state.watchEvents.get(definition.action);
    const current = definition.phaseKey === state.phase || (definition.phaseKey === "Reconnect" && state.phase === "Reconnect");
    const statusClass = `${rule.enabled ? " is-enabled" : ""}${event?.kind === "failed" ? " is-failed" : current ? " is-current" : ""}`;
    const status = event?.kind === "armed" ? `<span>等待触发</span><span class="watch-countdown" data-watch-countdown="${definition.action}" data-watch-verb="${definition.countdownVerb || "触发"}"><b>准备触发</b></span>`
      : event?.kind === "failed" ? '<span class="suite-chip is-danger">上次触发失败</span>'
      : event?.kind === "fired" ? '<span>上次触发 · 刚刚</span>'
      : '<span>未触发过</span>';
    const control = watchRuleControl(definition, rule);
    return `<article class="suite-card watch-rule${statusClass}" data-watch-card="${definition.action}"><div class="watch-rule-top"><div class="watch-rule-title"><h3>${definition.title}</h3><small>触发于 <b>${definition.phase}</b></small></div><label class="suite-switch watch-rule-toggle"><input type="checkbox" aria-label="启用${definition.title}" data-watch-toggle="${definition.key}"${checked(rule.enabled)}><span class="sr-only">启用</span></label></div><p>${definition.description}</p>${control ? `<div class="watch-control-row">${control}</div>` : ""}<div class="watch-rule-foot">${status}</div></article>`;
  }

  function bindWatchControls() {
    roots.watch.querySelector("[data-watch-master]")?.addEventListener("change", async (event) => {
      state.watch.masterEnabled = event.target.checked;
      await saveWatch();
    });
    for (const input of roots.watch.querySelectorAll("[data-watch-toggle]")) input.addEventListener("change", async () => {
      state.watch.rules[input.dataset.watchToggle].enabled = input.checked;
      await saveWatch();
    });
    for (const input of roots.watch.querySelectorAll("[data-watch-delay]")) {
      input.addEventListener("input", () => {
        roots.watch.querySelector(`[data-watch-output="${input.dataset.watchDelay}"]`).textContent = formatDelay(input.value);
        setWatchRangeFill(input);
      });
      input.addEventListener("change", async () => { state.watch.rules[input.dataset.watchDelay].delayMs = Number(input.value); await saveWatch(); });
    }
    for (const button of roots.watch.querySelectorAll("[data-watch-choice]")) button.addEventListener("click", async () => {
      const [key, field] = button.dataset.watchChoice.split(".");
      state.watch.rules[key][field] = button.dataset.watchChoiceValue;
      await saveWatch();
    });
    for (const select of roots.watch.querySelectorAll("[data-watch-number]")) select.addEventListener("change", async () => {
      const [key, field] = select.dataset.watchNumber.split(".");
      state.watch.rules[key][field] = Number(select.value);
      await saveWatch();
    });
    for (const button of roots.watch.querySelectorAll("[data-watch-policy-cycle]")) button.addEventListener("click", async () => {
      state.watch.rules.invitations.policies ||= {};
      const key = button.dataset.watchPolicyCycle;
      const current = state.watch.rules.invitations.policies[key] || state.watch.rules.invitations.policies.default || "ignore";
      state.watch.rules.invitations.policies[key] = { ignore: "accept", accept: "decline", decline: "ignore" }[current] || "ignore";
      await saveWatch();
    });
  }

  function setWatchRangeFill(input) {
    const min = Number(input.min || 0);
    const max = Number(input.max || 100);
    const value = Math.max(min, Math.min(max, Number(input.value || 0)));
    const fill = max > min ? ((value - min) / (max - min)) * 100 : 0;
    input.style.setProperty("--range-fill", `${fill}%`);
  }

  function applyWatchControlStyles() {
    for (const meter of roots.watch.querySelectorAll("[data-watch-meter]")) meter.style.setProperty("--watch-fill", `${Math.max(0, Math.min(100, Number(meter.dataset.watchMeter || 0)))}%`);
    for (const input of roots.watch.querySelectorAll("[data-watch-delay]")) setWatchRangeFill(input);
  }

  async function loadWatch(force = false) {
    if (state.watch && !force) { renderWatch(); return; }
    try {
      state.watch = await api("/api/watch/rules");
      renderWatch();
    } catch (error) {
      roots.watch.innerHTML = errorCard("值守规则读取失败", error.message, "watch");
    }
  }

  async function saveWatch() {
    try {
      state.watch = await api("/api/watch/rules", { method: "POST", body: JSON.stringify(state.watch) });
      renderWatch();
      toast("值守规则已保存");
    } catch (error) {
      toast(error.message);
      await loadWatch(true);
    }
  }

  function handleWatchEvent(value) {
    const parts = String(value || "").split(":");
    if (parts[0] !== "watch") return;
    if (parts[1] === "priority") {
      state.watchPriority = parts[2] === "honoring";
      renderWatch();
      return;
    }
    const kind = parts[1];
    const action = parts[2];
    if (!action || !["armed", "fired", "failed"].includes(kind)) return;
    if (kind === "failed" && action.startsWith("facade-")) {
      toast(action === "facade-rank" ? "登录时重设展示段位失败" : "登录时重设个性签名失败");
      return;
    }
    const delay = Number(parts[3] || 0);
    state.watchEvents.set(action, { kind, start: Date.now(), end: Date.now() + delay, delay });
    if (kind === "fired") {
      state.watchFired += 1;
      if (action === "auto-honor") state.watchPriority = false;
    }
    if (kind === "failed" && action === "auto-honor") state.watchPriority = false;
    renderWatch();
  }

  let countdownFrame = 0;
  function updateCountdowns() {
    cancelAnimationFrame(countdownFrame);
    const tick = () => {
      let pending = false;
      for (const element of roots.watch.querySelectorAll("[data-watch-countdown]")) {
        const event = state.watchEvents.get(element.dataset.watchCountdown);
        if (!event || event.kind !== "armed") continue;
        const remaining = Math.max(0, event.end - Date.now());
        const ratio = event.delay > 0 ? remaining / event.delay : 0;
        element.style.setProperty("--watch-progress", `${Math.round(ratio * 100)}%`);
        const label = element.querySelector("b");
        if (label) label.textContent = remaining > 0 ? `${(remaining / 1000).toFixed(1)} s 后${element.dataset.watchVerb || "触发"}` : "等待客户端确认";
        pending ||= remaining > 0;
      }
      if (pending) countdownFrame = requestAnimationFrame(tick);
    };
    tick();
  }

  function renderRig() {
    const value = state.rig || {};
    metrics.rig.textContent = !value.connected ? "未连接" : !value.settingsKnown ? "无法定位" : value.settingsLocked ? "已锁定" : "未锁定";
    roots.rig.className = "";
    roots.rig.innerHTML = `<div class="rig-layout"><div class="rig-main">
      <section class="suite-card"><div class="suite-card-head"><div><h3>安装与连接</h3><p>只读核对当前连接安装目录，不猜测缺失路径。</p></div><span class="suite-chip ${value.connected ? "is-success" : "is-warning"}">${value.connected ? "已连接" : "未连接"}</span></div><dl class="rig-kv"><dt>发行渠道</dt><dd>${escapeHTML(value.region || "无法定位")}${value.platform ? ` / ${escapeHTML(value.platform)}` : ""}</dd><dt>安装根目录</dt><dd>${escapeHTML(value.installRoot || "无法定位")}</dd><dt>配置目录</dt><dd>${escapeHTML(value.configRoot || "无法定位")}</dd><dt>设置文件</dt><dd>${escapeHTML(value.settingsFile || "无法定位")}${value.settingsKnown ? " · 已定位" : ""}</dd><dt>本机连接</dt><dd>${value.connected ? "127.0.0.1 已连接（端口与令牌不展示、不落盘）" : escapeHTML(value.reason || "未连接")}</dd><dt>客户端界面</dt><dd>${escapeHTML(value.uxState || "状态未知")}</dd></dl><div class="rig-lock"><span class="rig-lock-icon" aria-hidden="true">${value.settingsLocked ? "▣" : "□"}</span><div><h2>${value.settingsLocked ? "游戏设置已锁定" : "游戏设置未锁定"}</h2><p>${value.settingsKnown ? (value.settingsLocked ? "设置文件为只读，游戏内改动不会写回文件。" : "设置文件可写，游戏内改动可以保存。") : escapeHTML(value.reason || "无法定位设置文件，不会猜路径或尝试修改。")}</p></div><div class="rig-lock-actions"><span class="suite-chip ${value.settingsLocked ? "is-success" : ""}">${value.settingsLocked ? "只读" : "可写入"}</span><button class="button ${value.settingsLocked ? "button-secondary" : "button-primary"}" type="button" data-rig-lock ${value.settingsKnown ? "" : "disabled"}>${value.settingsLocked ? "解除锁定" : "设为只读"}</button></div></div><div class="suite-note is-info rig-lock-note"><span aria-hidden="true">i</span><span>只影响当前连接的安装目录。写入前会再次校验路径位于安装目录内，且目标不是符号链接。</span></div></section>
    </div><aside class="suite-card"><div class="suite-card-head"><div><h3>客户端维护</h3><p>所有动作都由你手动发起并二次确认。</p></div></div><div class="rig-actions">${[
      ["restart-ux", "重启客户端界面", "结束并重新启动 LeagueClientUx。"],
      ["kill-ux", "结束界面进程", "只结束客户端界面，不结束游戏端。"],
      ["launch-ux", "启动界面进程", "请求客户端重新拉起界面。"],
      ["quit-client", "关闭客户端", "请求英雄联盟客户端正常退出。"],
      ["disconnect", "断开连接", "让助手停止当前连接，顶部刷新后可重连。"],
    ].map(([action, title, copy]) => `<div class="rig-action"><div><strong>${title}</strong><p>${copy}</p></div><button class="button ${action === "quit-client" ? "button-danger" : "button-secondary"}" type="button" data-rig-action="${action}">${action === "disconnect" ? "断开" : "执行"}</button></div>`).join("")}</div></aside></div>`;
    bindRigControls();
  }

  function bindRigControls() {
    roots.rig.querySelector("[data-rig-lock]")?.addEventListener("click", async () => {
      const locked = !state.rig.settingsLocked;
      if (!confirm(locked ? "将当前客户端设置文件设为只读？" : "解除设置文件只读状态？之后游戏内改动可以写回文件。")) return;
      await runRigRequest("/api/rig/settings-lock", { locked });
    });
    for (const button of roots.rig.querySelectorAll("[data-rig-action]")) button.addEventListener("click", async () => {
      const label = button.closest(".rig-action")?.querySelector("strong")?.textContent || "维护动作";
      if (!confirm(`确认执行“${label}”？此动作会立即影响当前客户端。`)) return;
      await runRigRequest("/api/rig/maintenance", { action: button.dataset.rigAction }, button.dataset.rigAction);
    });
  }

  async function runRigRequest(path, body, action = "") {
    try {
      await api(path, { method: "POST", body: JSON.stringify(body) });
      toast("客户端整备操作已执行");
      if (action === "disconnect") {
        setConnected(false);
        document.getElementById("refresh")?.click();
      } else {
        await loadRig(true);
      }
    } catch (error) { toast(error.message); }
  }

  async function loadRig(force = false) {
    if (state.rig && !force) { renderRig(); return; }
    try { state.rig = await api("/api/rig/status"); renderRig(); }
    catch (error) { roots.rig.innerHTML = errorCard("整备状态读取失败", error.message, "rig"); }
  }

  function hydrateFacadeDraft(force = false) {
    if (state.facadeDraft && !force) return;
    const value = state.facade || {};
    const chat = value.chat || {};
    const lol = chat.lol || {};
    const firstSkin = value.skins?.find((skin) => skin.id === value.profile?.backgroundSkinId) || value.skins?.find((skin) => skin.owned) || value.skins?.[0] || {};
    state.facadeDraft = {
      hero: String(firstSkin.championId || ""), skinId: Number(firstSkin.id || 0), ownedOnly: true,
      availability: chat.availability || "chat", statusMessage: chat.statusMessage || "",
      queue: lol.rankedLeagueQueue || "RANKED_SOLO_5X5", tier: lol.rankedLeagueTier || "DIAMOND", division: lol.rankedLeagueDivision || "II",
      resetStatus: Boolean(value.loginReset?.statusMessageEnabled), resetRank: Boolean(value.loginReset?.rankEnabled),
    };
  }

  function renderFacade() {
    const value = state.facade || {};
    hydrateFacadeDraft();
    const draft = state.facadeDraft;
    const summoner = value.summoner || {};
    const skins = Array.isArray(value.skins) ? value.skins : [];
    const champions = [...new Map(skins.map((skin) => [String(skin.championId), skin.championName || `英雄 ${skin.championId}`])).entries()].sort((left, right) => left[1].localeCompare(right[1], "zh-CN"));
    let visibleSkins = skins.filter((skin) => String(skin.championId) === draft.hero && (!draft.ownedOnly || skin.owned));
    if (!visibleSkins.length) visibleSkins = skins.filter((skin) => !draft.ownedOnly || skin.owned).slice(0, 16);
    if (!visibleSkins.some((skin) => Number(skin.id) === Number(draft.skinId)) && visibleSkins[0]) draft.skinId = Number(visibleSkins[0].id);
    const selectedSkin = skins.find((skin) => Number(skin.id) === Number(draft.skinId)) || visibleSkins[0] || {};
    const displayName = summoner.gameName || summoner.displayName || "当前召唤师";
    const tagLine = summoner.tagLine ? `#${summoner.tagLine}` : "";
    const availabilityLabels = { chat: "在线", away: "离开", dnd: "游戏中", offline: "离线" };
    const highTier = ["MASTER", "GRANDMASTER", "CHALLENGER", "UNRANKED"].includes(draft.tier);
    const backgroundApplied = Number(value.profile?.backgroundSkinId) === Number(draft.skinId);
    const identityDirty = facadeIdentityDirty();
    const resetDirty = facadeResetDirty();
    roots.facade.className = "";
    roots.facade.innerHTML = `<div class="facade-layout"><div class="facade-left"><section class="suite-card facade-preview"><div class="facade-preview-art">${selectedSkin.splashPath ? `<img src="${imageURL(selectedSkin.splashPath)}" alt="" data-suite-facade-art>` : ""}<span class="facade-art-label">生涯背景 · ${escapeHTML(selectedSkin.name || value.profile?.backgroundSkinName || "未设置")}</span></div><div class="facade-preview-body"><span class="facade-avatar">${summoner.profileIconId ? `<img src="${imageURL(`/lol-game-data/assets/v1/profile-icons/${summoner.profileIconId}.jpg`)}" alt="">` : escapeHTML(displayName.slice(0, 1))}<small class="facade-avatar-level">${Number(summoner.summonerLevel || 0)}</small></span><div class="facade-identity"><h2>${escapeHTML(displayName)}</h2><p>${escapeHTML(tagLine || "当前账号")}</p></div><div class="facade-tags"><span class="suite-chip is-gold">${escapeHTML(rankLabel(draft))}</span><span class="suite-chip">${escapeHTML(availabilityLabels[draft.availability] || draft.availability)}</span><span class="suite-chip">上赛季旗帜</span></div><div class="facade-signature">${escapeHTML(draft.statusMessage || "个性签名会显示在这里")}</div><div class="facade-slots"><span class="facade-slot">勋章 1</span><span class="facade-slot">勋章 2</span><span class="facade-slot">勋章 3</span></div><p>这是好友悬浮卡与生涯页的示意预览。右侧任何改动都会先反映在这里，确认后才写入客户端。</p></div></section><section class="suite-card facade-write-card"><div class="suite-card-head"><div><h3>本页会写入客户端的内容</h3></div></div><dl class="facade-write-list"><dt>生涯背景</dt><dd>当前召唤师的背景皮肤</dd><dt>聊天卡片</dt><dd>在线状态、个性签名、展示段位</dd><dt>展示位</dt><dd>头像框、挑战勋章、旗帜、表情轮盘</dd></dl><div class="suite-note facade-write-note"><span aria-hidden="true">⚑</span><span>以上全部<strong>只在你点击后执行</strong>，没有任何自动写入。只有“登录时重设”两项例外，它们默认关闭，开启后也只重放你保存过的值。</span></div></section></div>
      <div class="facade-controls"><section class="suite-card facade-background-card"><div class="suite-card-head"><div><h3>生涯背景</h3></div>${backgroundApplied ? '<span class="suite-chip">已应用</span>' : `<button class="button button-primary" type="button" data-facade-apply-background ${draft.skinId ? "" : "disabled"}>应用背景</button>`}</div><div class="facade-selections"><label><span class="sr-only">英雄</span><select class="suite-select" data-facade-hero>${champions.map(([id, name]) => `<option value="${escapeHTML(id)}"${selected(draft.hero, id)}>${escapeHTML(name)}</option>`).join("")}</select></label><label class="suite-switch"><input type="checkbox" data-facade-owned${checked(draft.ownedOnly)}><span>只显示已拥有</span></label></div><div class="facade-film" aria-label="皮肤胶片">${visibleSkins.map((skin) => `<button type="button" class="${Number(skin.id) === Number(draft.skinId) ? "is-selected" : ""}" data-facade-skin="${skin.id}" aria-label="${escapeHTML(skin.name)}${skin.owned ? "" : "，未拥有"}" aria-pressed="${Number(skin.id) === Number(draft.skinId)}" title="${escapeHTML(skin.name)}${skin.owned ? "" : " · 未拥有"}"><img src="${imageURL(skin.tilePath || skin.splashPath)}" alt=""><span class="sr-only">${escapeHTML(skin.name)}</span></button>`).join("") || '<span class="muted">没有符合筛选条件的皮肤</span>'}</div><p class="facade-background-note">客户端接口<strong>不校验皮肤是否拥有</strong>——关掉上面的开关就能设置未拥有的皮肤，但它可能在下次登录时被服务端还原。</p></section>
      <section class="suite-card facade-chat-card"><div class="suite-card-head"><div><h3>聊天身份</h3></div></div><div class="facade-row"><div><h4>在线状态</h4><p>部分状态只在特定情况下可用；设为离线后可开启“登录时重设”防止客户端自动切回</p></div><div class="suite-segment">${Object.entries(availabilityLabels).map(([key, label]) => `<button type="button" class="${draft.availability === key ? "is-active" : ""}" data-facade-availability="${key}">${label}</button>`).join("")}</div></div><div class="facade-row"><div><h4>个性签名</h4><p>留空即删除签名。开启“登录时重设”后每次客户端登录都会重新应用</p></div><div class="facade-row-control"><input class="suite-input facade-status-input" type="text" maxlength="200" value="${escapeHTML(draft.statusMessage)}" placeholder="输入签名…" data-facade-status><label class="suite-switch facade-reset-switch" data-tooltip="登录时重设个性签名"><input type="checkbox" aria-label="登录时重设个性签名" data-facade-reset-status${checked(draft.resetStatus)}><span class="sr-only">登录时重设</span></label></div></div><div class="facade-row"><div><h4>展示段位</h4><p>只改好友悬浮卡上的段位显示，不影响你的真实段位、战绩与匹配。大师及以上不需要选分段</p></div><div class="facade-row-control"><div class="facade-rank-fields"><select class="suite-select" data-facade-rank="queue"><option value="RANKED_SOLO_5X5"${selected(draft.queue, "RANKED_SOLO_5X5")}>单双排</option><option value="RANKED_FLEX_SR"${selected(draft.queue, "RANKED_FLEX_SR")}>灵活组排</option></select><select class="suite-select" data-facade-rank="tier">${rankOptions(draft.tier)}</select><select class="suite-select" data-facade-rank="division" ${highTier ? "disabled" : ""}>${["I","II","III","IV"].map((division) => `<option value="${division}"${selected(draft.division, division)}>${division}</option>`).join("")}</select></div><label class="suite-switch facade-reset-switch" data-tooltip="登录时重设展示段位"><input type="checkbox" aria-label="登录时重设展示段位" data-facade-reset-rank${checked(draft.resetRank)}><span class="sr-only">登录时重设</span></label></div></div><div class="facade-commit" data-facade-commit ${identityDirty || resetDirty ? "" : "hidden"}><span>改动只在左侧预览，确认后才写入客户端。</span><div><button class="button button-secondary" type="button" data-facade-save-reset ${resetDirty ? "" : "hidden"}>保存登录重设</button><button class="button button-primary" type="button" data-facade-apply-chat ${identityDirty ? "" : "hidden"}>确认并应用</button></div></div></section>
      <section class="suite-card"><div class="suite-card-head"><div><h3>展示清理</h3><p>以下均为不可撤销的一次性动作，点击后会二次确认。</p></div></div><div class="facade-actions">${[
        ["clear-border", "卸下头像框", "换成固定的典藏边框；需要召唤师等级 ≥ 526", Number(summoner.summonerLevel || 0) < 526, "卸下"],
        ["clear-challenges", "卸下全部勋章", "清空生涯页展示的挑战勋章", false, "卸下"],
        ["previous-banner", "切换上赛季旗帜", "换回上个赛季的旗帜样式，可能暂时不显示", false, "切换"],
        ["clear-emotes", "清空表情轮盘", "一次性清掉所有表情位，需要重新自行配置", false, "清空"],
      ].map(([action, title, copy, disabled, label]) => `<div class="facade-action"><div><strong>${title}</strong><p>${copy}</p></div>${disabled ? '<span class="suite-chip is-danger">等级不足</span>' : `<button class="button ${action === "clear-emotes" ? "button-danger" : "button-secondary"}" type="button" data-facade-clear="${action}">${label}</button>`}</div>`).join("")}</div></section></div></div>`;
    bindFacadeControls();
  }

  function facadeIdentityDirty() {
    const chat = state.facade?.chat || {};
    const lol = chat.lol || {};
    const draft = state.facadeDraft || {};
    return String(chat.availability || "chat") !== String(draft.availability || "chat")
      || String(chat.statusMessage || "") !== String(draft.statusMessage || "")
      || String(lol.rankedLeagueQueue || "RANKED_SOLO_5X5") !== String(draft.queue || "")
      || String(lol.rankedLeagueTier || "DIAMOND") !== String(draft.tier || "")
      || (!["MASTER", "GRANDMASTER", "CHALLENGER", "UNRANKED"].includes(draft.tier) && String(lol.rankedLeagueDivision || "II") !== String(draft.division || ""));
  }

  function facadeResetDirty() {
    const reset = state.facade?.loginReset || {};
    const rank = reset.rank || {};
    const draft = state.facadeDraft || {};
    return Boolean(reset.statusMessageEnabled) !== Boolean(draft.resetStatus)
      || ((reset.statusMessageEnabled || draft.resetStatus) && String(reset.statusMessage || "") !== String(draft.statusMessage || ""))
      || Boolean(reset.rankEnabled) !== Boolean(draft.resetRank)
      || ((reset.rankEnabled || draft.resetRank) && (String(rank.rankedLeagueQueue || "") !== String(draft.queue || "")
        || String(rank.rankedLeagueTier || "") !== String(draft.tier || "")
        || (!["MASTER", "GRANDMASTER", "CHALLENGER", "UNRANKED"].includes(draft.tier) && String(rank.rankedLeagueDivision || "") !== String(draft.division || ""))));
  }

  function syncFacadeCommitActions() {
    const identityDirty = facadeIdentityDirty();
    const resetDirty = facadeResetDirty();
    const commit = roots.facade.querySelector("[data-facade-commit]");
    const apply = roots.facade.querySelector("[data-facade-apply-chat]");
    const save = roots.facade.querySelector("[data-facade-save-reset]");
    if (commit) commit.hidden = !identityDirty && !resetDirty;
    if (apply) apply.hidden = !identityDirty;
    if (save) save.hidden = !resetDirty;
  }

  function rankOptions(current) {
    const values = [["UNRANKED","未定级"],["IRON","坚韧黑铁"],["BRONZE","英勇黄铜"],["SILVER","不屈白银"],["GOLD","荣耀黄金"],["PLATINUM","华贵铂金"],["EMERALD","流光翡翠"],["DIAMOND","璀璨钻石"],["MASTER","超凡大师"],["GRANDMASTER","傲世宗师"],["CHALLENGER","最强王者"]];
    return values.map(([value, label]) => `<option value="${value}"${selected(current, value)}>${label}</option>`).join("");
  }

  function rankLabel(draft) {
    const tier = { UNRANKED: "未定级", IRON: "黑铁", BRONZE: "黄铜", SILVER: "白银", GOLD: "黄金", PLATINUM: "铂金", EMERALD: "翡翠", DIAMOND: "钻石", MASTER: "大师", GRANDMASTER: "宗师", CHALLENGER: "王者" }[draft.tier] || draft.tier;
    const queue = draft.queue === "RANKED_FLEX_SR" ? "灵活组排" : "单双排";
    return `${tier}${["MASTER","GRANDMASTER","CHALLENGER","UNRANKED"].includes(draft.tier) ? "" : ` ${draft.division}`} · ${queue}`;
  }

  function bindFacadeControls() {
    roots.facade.querySelector("[data-facade-hero]")?.addEventListener("change", (event) => { state.facadeDraft.hero = event.target.value; state.facadeDraft.skinId = 0; renderFacade(); });
    roots.facade.querySelector("[data-facade-owned]")?.addEventListener("change", (event) => { state.facadeDraft.ownedOnly = event.target.checked; renderFacade(); });
    for (const button of roots.facade.querySelectorAll("[data-facade-skin]")) button.addEventListener("click", () => { state.facadeDraft.skinId = Number(button.dataset.facadeSkin); renderFacade(); });
    for (const button of roots.facade.querySelectorAll("[data-facade-availability]")) button.addEventListener("click", () => { state.facadeDraft.availability = button.dataset.facadeAvailability; renderFacade(); });
    roots.facade.querySelector("[data-facade-status]")?.addEventListener("input", (event) => { state.facadeDraft.statusMessage = event.target.value; roots.facade.querySelector(".facade-signature").textContent = event.target.value || "个性签名会显示在这里"; syncFacadeCommitActions(); });
    for (const select of roots.facade.querySelectorAll("[data-facade-rank]")) select.addEventListener("change", () => { state.facadeDraft[select.dataset.facadeRank] = select.value; renderFacade(); });
    roots.facade.querySelector("[data-facade-reset-status]")?.addEventListener("change", (event) => { state.facadeDraft.resetStatus = event.target.checked; syncFacadeCommitActions(); });
    roots.facade.querySelector("[data-facade-reset-rank]")?.addEventListener("change", (event) => { state.facadeDraft.resetRank = event.target.checked; syncFacadeCommitActions(); });
    roots.facade.querySelector("[data-facade-apply-background]")?.addEventListener("click", () => applyFacade({ action: "background", skinId: state.facadeDraft.skinId }));
    roots.facade.querySelector("[data-facade-apply-chat]")?.addEventListener("click", applyFacadeIdentity);
    roots.facade.querySelector("[data-facade-save-reset]")?.addEventListener("click", saveFacadeReset);
    for (const button of roots.facade.querySelectorAll("[data-facade-clear]")) button.addEventListener("click", () => {
      const label = button.closest(".facade-action")?.querySelector("strong")?.textContent || "展示清理";
      if (confirm(`确认“${label}”？此动作不可撤销。`)) applyFacade({ action: button.dataset.facadeClear });
    });
  }

  async function applyFacadeIdentity() {
    try {
      await api("/api/facade/apply", { method: "POST", body: JSON.stringify({ action: "chat", availability: state.facadeDraft.availability, statusMessage: state.facadeDraft.statusMessage }) });
      state.facade = await api("/api/facade/apply", { method: "POST", body: JSON.stringify({ action: "rank", queue: state.facadeDraft.queue, tier: state.facadeDraft.tier, division: state.facadeDraft.division }) });
      hydrateFacadeDraft(true);
      renderFacade();
      toast("聊天身份已应用");
    } catch (error) { toast(error.message); }
  }

  async function saveFacadeReset() {
    const rank = { rankedLeagueQueue: state.facadeDraft.queue, rankedLeagueTier: state.facadeDraft.tier };
    if (!["MASTER","GRANDMASTER","CHALLENGER","UNRANKED"].includes(state.facadeDraft.tier)) rank.rankedLeagueDivision = state.facadeDraft.division;
    await applyFacade({ action: "login-reset", loginReset: { statusMessageEnabled: state.facadeDraft.resetStatus, statusMessage: state.facadeDraft.statusMessage, rankEnabled: state.facadeDraft.resetRank, rank } }, "登录重设已保存；连接后延时 2 秒应用，手动操作会抢占本次重设");
  }

  async function applyFacade(body, success = "门面设置已应用") {
    try {
      state.facade = await api("/api/facade/apply", { method: "POST", body: JSON.stringify(body) });
      hydrateFacadeDraft(true);
      renderFacade();
      toast(success);
    } catch (error) { toast(error.message); }
  }

  async function loadFacade(force = false) {
    if (state.facade && !force) { renderFacade(); return; }
    try { state.facade = await api("/api/facade/state"); hydrateFacadeDraft(true); renderFacade(); }
    catch (error) { roots.facade.innerHTML = errorCard("门面读取失败", error.message, "facade"); }
  }

  function claimVisible(item) {
    if (state.claimFilter === "historical") return item.historical;
    if (state.claimFilter === "choice") return item.needsChoice;
    if (state.claimFilter === "chain") return Boolean(item.chainId || item.chainCount);
    if (state.claimFilter === "failed") return state.claimFailures.has(item.key);
    return true;
  }

  function choiceValid(item) {
    if (!item.needsChoice) return true;
    const count = state.claimChoices.get(item.key)?.size || 0;
    const minimum = Math.max(1, Number(item.minSelections || 1));
    const maximum = Math.min(Number(item.maxSelections || item.items?.length || 1), item.items?.length || 1);
    return count >= minimum && count <= maximum;
  }

  function renderClaims() {
    const response = state.claims || { items: [], sources: {} };
    const items = Array.isArray(response.items) ? response.items : [];
    metrics.claim.textContent = String(items.length);
    const counts = {
      all: items.length,
      historical: items.filter((item) => item.historical).length,
      choice: items.filter((item) => item.needsChoice).length,
      chain: items.filter((item) => item.chainId || item.chainCount).length,
      failed: items.filter((item) => state.claimFailures.has(item.key)).length,
    };
    const visible = items.filter(claimVisible);
    const scannedAt = response.scannedAt ? relativeTime(response.scannedAt) : "尚未扫描";
    roots.sweep.className = "";
    roots.sweep.innerHTML = `<div class="sweep-toolbar"><div class="sweep-sources">${["grant","mission","event"].map((source) => {
      const current = response.sources?.[source] || {};
      const className = current.state === "failed" ? " is-failed" : current.state !== "available" ? " is-unavailable" : "";
      return `<span class="sweep-source${className}" title="${escapeHTML(current.detail || current.state || "")}"><i aria-hidden="true"></i>${sourceNames[source]} <b>${Number(current.count || 0)}</b></span>`;
    }).join("")}<span class="sweep-source is-unavailable" title="普通战利品宝箱不是待领取账本，不在本页自动处理"><i aria-hidden="true"></i>战利品宝箱 <b>0</b></span></div><div class="sweep-toolbar-actions"><small>上次扫描 · ${escapeHTML(scannedAt)}</small><button class="button button-secondary" type="button" data-claim-scan>重新扫描</button></div></div>
      <div class="sweep-filters" role="tablist" aria-label="拾遗筛选">${[["all","全部"],["historical","历史活动"],["choice","需要选择"],["chain","任务链"],["failed","已失败"]].map(([key, label]) => `<button class="suite-filter${state.claimFilter === key ? " is-active" : ""}" type="button" role="tab" aria-selected="${state.claimFilter === key}" data-claim-filter="${key}">${label} ${counts[key]}</button>`).join("")}</div>
      ${visible.length ? `<div class="claim-list">${visible.map(claimRow).join("")}</div>` : `<div class="sweep-empty"><div><strong>${items.length ? "当前筛选下没有条目" : "没有发现待领取奖励"}</strong><p>${items.length ? "切换筛选查看其他来源。" : "奖励账本、任务与事件中心都已扫描。"}</p></div></div>`}
      <div class="sweep-sticky"><div class="sweep-progress-copy"><strong>已选 ${state.selectedClaims.size} 项</strong><span class="sweep-progress"><i data-claim-progress></i></span><span>已完成 ${state.claimProgress.done} / ${state.claimProgress.total || items.length} · 失败 ${state.claimProgress.failed}</span></div><div class="sweep-actions"><button class="button button-secondary" type="button" data-claim-select-all>全选可领取</button><button class="button button-primary" type="button" data-claim-run ${state.selectedClaims.size && !state.claiming ? "" : "disabled"}>${state.claiming ? "领取中…" : "开始领取"}</button></div></div>
      <div class="suite-note sweep-privacy"><span aria-hidden="true">!</span><span>领取过程<strong>逐项独立执行</strong>：某一项失败不会中断后面的项，客户端错误体会内联显示。<strong>待领取明细不会落盘</strong>，关闭页面即从内存中丢弃。</span></div>`;
    const progress = roots.sweep.querySelector("[data-claim-progress]");
    const denominator = Math.max(1, state.claimProgress.total || items.length);
    progress?.style.setProperty("--claim-progress", `${Math.min(100, Math.round(state.claimProgress.done / denominator * 100))}%`);
    bindClaimControls();
  }

  function claimRow(item) {
    const failure = state.claimFailures.get(item.key);
    const selectedRow = state.selectedClaims.has(item.key);
    const choices = state.claimChoices.get(item.key) || new Set();
    const maximum = Math.min(Number(item.maxSelections || item.items?.length || 1), item.items?.length || 1);
    const needsChoice = item.needsChoice && !choiceValid(item);
    return `<article class="claim-row${selectedRow ? " is-selected" : ""}${failure ? " is-failed" : ""}" data-claim-row="${escapeHTML(item.key)}"><input class="claim-check" type="checkbox" aria-label="选择 ${escapeHTML(item.title || "奖励")}" data-claim-check="${escapeHTML(item.key)}"${checked(selectedRow)} ${needsChoice || state.claiming ? "disabled" : ""}><div class="claim-main"><div class="claim-meta"><span class="claim-title">${escapeHTML(item.title || `未命名奖励组 (${item.items?.length || 0})`)}</span><span class="suite-chip is-accent">${escapeHTML(sourceNames[item.source] || item.source)}</span>${item.historical ? '<span class="suite-chip">历史活动</span>' : ""}${item.needsChoice ? `<span class="suite-chip is-warning">${Math.max(1, Number(item.minSelections || 1)) === maximum ? `${maximum} 选 ${maximum}` : `${Math.max(1, Number(item.minSelections || 1))} - ${maximum} 选`}</span>` : ""}${item.chainCount ? `<span class="suite-chip">任务链 ${Number(item.chainIndex || 1)}/${Number(item.chainCount)}</span>` : ""}${item.overlapWith ? `<span class="suite-chip is-warning">与“${escapeHTML(sourceNames[item.overlapWith] || item.overlapWith)}”重叠</span>` : ""}</div>${item.items?.length ? `<div class="claim-tiles">${item.items.map((reward, index) => rewardTile(item, reward, index, choices, maximum)).join("")}</div>` : ""}${needsChoice ? `<div class="claim-choice-hint"><span aria-hidden="true">!</span><span>需要你先选出 ${Math.max(1, Number(item.minSelections || 1))} 项；随行不会随机替你决定。</span></div>` : ""}${failure ? `<div class="claim-error"><span aria-hidden="true">×</span><span>${escapeHTML([failure.statusCode, failure.errorCode].filter(Boolean).join(" · ") || "领取失败")}：${escapeHTML(failure.message || "客户端未提供原因")} ${escapeHTML(failure.consequence || "本项失败不会中断其它条目。")}</span></div>` : ""}</div><div class="claim-status">${failure ? '<span class="suite-chip is-danger">领取失败</span>' : needsChoice ? "需要选择" : "待领取"}</div></article>`;
  }

  function rewardTile(item, reward, index, choices, maximum) {
    const id = String(reward.id || reward.itemId || index);
    const chosen = choices.has(id);
    const content = `${reward.iconUrl ? `<img src="${imageURL(reward.iconUrl)}" alt="">` : '<span class="suite-tile-icon" aria-hidden="true">◇</span>'}<span><strong>${escapeHTML(reward.title || reward.itemType || reward.itemId || "奖励")}</strong><small>×${Math.max(1, Number(reward.quantity || 1))}</small></span>`;
    if (!item.needsChoice) return `<span class="suite-tile">${content}</span>`;
    return `<button class="suite-tile${chosen ? " is-selected" : ""}" type="button" aria-pressed="${chosen}" data-claim-choice="${escapeHTML(item.key)}" data-reward-id="${escapeHTML(id)}" data-choice-max="${maximum}" ${state.claiming ? "disabled" : ""}>${content}</button>`;
  }

  function bindClaimControls() {
    roots.sweep.querySelector("[data-claim-scan]")?.addEventListener("click", () => loadClaims(true));
    for (const button of roots.sweep.querySelectorAll("[data-claim-filter]")) button.addEventListener("click", () => { state.claimFilter = button.dataset.claimFilter; renderClaims(); });
    for (const input of roots.sweep.querySelectorAll("[data-claim-check]")) input.addEventListener("change", () => { input.checked ? state.selectedClaims.add(input.dataset.claimCheck) : state.selectedClaims.delete(input.dataset.claimCheck); renderClaims(); });
    for (const button of roots.sweep.querySelectorAll("[data-claim-choice]")) button.addEventListener("click", () => {
      const key = button.dataset.claimChoice;
      const id = button.dataset.rewardId;
      const choices = state.claimChoices.get(key) || new Set();
      if (choices.has(id)) choices.delete(id);
      else {
        if (choices.size >= Number(button.dataset.choiceMax || 1)) choices.delete(choices.values().next().value);
        choices.add(id);
      }
      state.claimChoices.set(key, choices);
      if (!choiceValid((state.claims.items || []).find((item) => item.key === key) || {})) state.selectedClaims.delete(key);
      renderClaims();
    });
    roots.sweep.querySelector("[data-claim-select-all]")?.addEventListener("click", () => {
      for (const item of (state.claims?.items || []).filter(claimVisible)) if (choiceValid(item)) state.selectedClaims.add(item.key);
      renderClaims();
    });
    roots.sweep.querySelector("[data-claim-run]")?.addEventListener("click", executeClaims);
  }

  async function executeClaims() {
    const queue = [...state.selectedClaims];
    if (!queue.length || state.claiming) return;
    state.claiming = true;
    state.claimProgress = { done: 0, total: queue.length, failed: 0 };
    renderClaims();
    for (const key of queue) {
      const selections = [...(state.claimChoices.get(key) || [])];
      try {
        const response = await api("/api/claim/execute", { method: "POST", body: JSON.stringify({ items: [{ key, selections }] }) });
        const result = response.results?.[0];
        if (!result?.ok) {
          state.claimFailures.set(key, result || { message: "领取失败", consequence: "本项失败不会中断其它条目。" });
          state.claimProgress.failed += 1;
        } else {
          state.claimFailures.delete(key);
          state.selectedClaims.delete(key);
        }
        if (response.scan) state.claims = response.scan;
      } catch (error) {
        state.claimFailures.set(key, { message: error.message, consequence: "本项失败不会中断其它条目。" });
        state.claimProgress.failed += 1;
      }
      state.claimProgress.done += 1;
      renderClaims();
    }
    state.claiming = false;
    renderClaims();
    toast(`领取完成：成功 ${state.claimProgress.total - state.claimProgress.failed} 项，失败 ${state.claimProgress.failed} 项`);
  }

  async function loadClaims(force = false) {
    if (state.claims && !force) { renderClaims(); return; }
    try {
      state.claims = await api("/api/claim/scan");
      for (const item of state.claims.items || []) if (item.failure && !state.claimFailures.has(item.key)) state.claimFailures.set(item.key, item.failure);
      for (const key of [...state.selectedClaims]) if (!state.claims.items?.some((item) => item.key === key)) state.selectedClaims.delete(key);
      renderClaims();
    } catch (error) { roots.sweep.innerHTML = errorCard("未领取奖励扫描失败", error.message, "sweep"); }
  }

  function relativeTime(value) {
    const elapsed = Math.max(0, Date.now() - new Date(value).getTime());
    if (!Number.isFinite(elapsed)) return "时间未知";
    const seconds = Math.round(elapsed / 1000);
    if (seconds < 60) return `${seconds} 秒前`;
    const minutes = Math.round(seconds / 60);
    return minutes < 60 ? `${minutes} 分钟前` : `${Math.round(minutes / 60)} 小时前`;
  }

  function errorCard(title, message, target) {
    return `<section class="suite-card"><div class="suite-card-head"><div><h3>${escapeHTML(title)}</h3><p>${escapeHTML(message)}</p></div><button class="button button-secondary" type="button" data-suite-reload="${target}">重试</button></div></section>`;
  }

  panel.addEventListener("click", (event) => {
    const reload = event.target.closest("[data-suite-reload]");
    if (!reload) return;
    ({ watch: loadWatch, rig: loadRig, facade: loadFacade, sweep: loadClaims }[reload.dataset.suiteReload])?.(true);
  });

  document.addEventListener("click", (event) => {
    const navigate = event.target.closest("[data-navigate-suite]");
    if (!navigate) return;
    window.dispatchEvent(new CustomEvent("deep-legends:navigate", { detail: { section: "suite", tab: navigate.dataset.navigateSuite || "watch" } }));
  });

  panel.querySelector("[data-suite-retry]")?.addEventListener("click", () => document.getElementById("refresh")?.click());
  window.addEventListener("deep-legends:section", (event) => {
    state.active = event.detail?.name === "suite";
    if (!state.active) return;
    subtitle.textContent = tabCopy[state.tab];
    loadAll();
  });
  window.addEventListener("deep-legends:navigate", (event) => { if (event.detail?.section === "suite" && event.detail?.tab) activateTab(event.detail.tab); });
  window.addEventListener("deep-legends:status", (event) => {
    setConnected(Boolean(event.detail?.connected));
    if (state.connected && state.active) loadAll();
  });
  window.addEventListener("deep-legends:gameflow", (event) => { state.phase = event.detail?.phase || state.phase; if (state.watch) renderWatch(); });
  window.addEventListener("deep-legends:watch", (event) => handleWatchEvent(event.detail?.event || event.detail || ""));
  window.addEventListener("deep-legends:claim-changed", () => { if (state.active) loadClaims(true); else state.claims = null; });

  window.addEventListener("beforeunload", () => {
    state.scroll[state.tab] = Number(appScroll?.scrollTop || 0);
    writePreference("suite-scroll", JSON.stringify(state.scroll));
  });

  setupTabs();
})();
