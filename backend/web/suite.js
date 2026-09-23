(() => {
  "use strict";

  const escapeHTML = window.deepLegendsRuntime.escapeHTML;

  const panel = document.getElementById("suite-panel");
  if (!panel) return;

  const tabs = [...panel.querySelectorAll("[data-suite-tab]")];
  const panels = [...panel.querySelectorAll("[data-suite-panel]")];
  const roots = {
    watch: document.getElementById("suite-watch-root"),
    rig: document.getElementById("suite-rig-root"),
    facade: document.getElementById("suite-facade-root"),
    sweep: document.getElementById("suite-sweep-root"),
    champselect: document.getElementById("suite-champselect-root"),
  };
  const metrics = {
    watch: document.getElementById("suite-watch-metric"),
    rig: document.getElementById("suite-rig-metric"),
    claim: document.getElementById("suite-claim-metric"),
  };
  const offline = document.getElementById("suite-offline");
  const appScroll = document.getElementById("app-scroll");
  const subtitle = document.getElementById("topbar-subtitle");
  const standardMetrics = panel.querySelector("[data-suite-standard-metrics]");
  const champSelectMetrics = panel.querySelector("[data-suite-champselect-metrics]");
  const champSelectConfiguredMetric = document.getElementById("suite-champselect-configured");
  const champSelectActiveMetric = document.getElementById("suite-champselect-active");

  const tabCopy = {
    watch: "自动接受、重连与点赞",
    rig: "安装目录与客户端进程",
    facade: "背景、签名与展示",
    sweep: "未领取奖励清扫",
    champselect: "禁用与选用的备选序列",
  };
  const phaseOrder = ["Lobby", "Matchmaking", "ReadyCheck", "ChampSelect", "InProgress", "EndOfGame"];
  const phaseNames = { Lobby: "房间", Matchmaking: "匹配中", ReadyCheck: "确认对局", ChampSelect: "英雄选择", InProgress: "游戏中", Reconnect: "重连", EndOfGame: "结算" };
  const sourceNames = { grant: "奖励账本", mission: "任务", event: "事件中心" };
  const state = {
    tab: readPreference("suite-tab", "watch"),
    scroll: readJSONPreference("suite-scroll", {}),
    connected: null,
    eventStream: null,
    active: false,
    loading: false,
    watch: null,
    watchPromise: null,
    watchRevision: 0,
    watchSaveQueue: null,
    watchPendingSaves: 0,
    rig: null,
    facade: null,
    facadeDraft: null,
	facadeLoadedAt: 0,
	facadeChallengeRetryUsed: false,
	facadeChallengeRetryTimer: 0,
	facadeRefreshTimer: 0,
	facadeRefreshDeferred: false,
	facadePointerActive: false,
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
    champSelectGroups: null,
    champSelectRuntime: null,
    champSelectCatalog: null,
    champSelectGroup: "ranked",
    champSelectLane: { ban: "middle", pick: "middle" },
    champSelectDialog: null,
    champSelectPosition: "all",
    champSelectPositionIDs: null,
    champSelectPositionCache: new Map(),
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


  function toast(message) {
    if (typeof window.deepLegendsToast === "function") window.deepLegendsToast(message);
  }

  let activeConfirmation = null;

  // 确认卡与 toast 共用右下角锚点，卡片打开期间把 toast 抬到卡片上方（见 app.css）。
  function syncConfirmationClearance(card) {
    const height = Math.ceil(card?.getBoundingClientRect().height || 0);
    document.documentElement.style.setProperty("--suite-confirm-clearance", `${height + 10}px`);
    document.body.dataset.suiteConfirmOpen = "true";
  }

  function clearConfirmationClearance() {
    document.documentElement.style.removeProperty("--suite-confirm-clearance");
    delete document.body.dataset.suiteConfirmOpen;
  }

  function confirmationFocusables(card) {
    return [...card.querySelectorAll("[data-suite-confirm-cancel], [data-suite-confirm-accept]")];
  }

  function closeConfirmation(confirmed, restoreFocus = true) {
    const current = activeConfirmation;
    if (!current) return;
    activeConfirmation = null;
    current.card.remove();
    clearConfirmationClearance();
    current.resolve(Boolean(confirmed));
    if (restoreFocus && current.trigger?.isConnected) current.trigger.focus();
  }

  function requestConfirmation(trigger, title, consequence) {
    if (activeConfirmation) closeConfirmation(false, false);
    return new Promise((resolve) => {
      const card = document.createElement("section");
      card.className = "suite-confirm-card";
      card.setAttribute("role", "dialog");
      card.setAttribute("aria-modal", "false");
      card.setAttribute("aria-labelledby", "suite-confirm-title");
      card.setAttribute("aria-describedby", "suite-confirm-copy");
      card.innerHTML = `<div><strong id="suite-confirm-title">${escapeHTML(title)}</strong><p id="suite-confirm-copy">${escapeHTML(consequence)}</p></div><div class="suite-confirm-actions"><button class="button button-secondary" type="button" data-suite-confirm-cancel>取消</button><button class="button button-danger" type="button" data-suite-confirm-accept>确认</button></div>`;
      document.body.append(card);
      activeConfirmation = { card, resolve, trigger };
      card.querySelector("[data-suite-confirm-cancel]").addEventListener("click", () => closeConfirmation(false));
      card.querySelector("[data-suite-confirm-accept]").addEventListener("click", () => closeConfirmation(true));
      requestAnimationFrame(() => {
        syncConfirmationClearance(card);
        card.querySelector("[data-suite-confirm-cancel]")?.focus();
      });
    });
  }

  document.addEventListener("keydown", (event) => {
    if (!activeConfirmation) return;
    const { card } = activeConfirmation;
    if (event.key === "Escape") {
      event.preventDefault();
      closeConfirmation(false);
      return;
    }
    // 焦点陷阱：卡片是不可逆操作（卸下全部勋章等）的最后一道确认，Tab 不能跑进背景，
    // 否则用户会在焦点已离开卡片后按 Enter 触发背景控件。
    // aria-modal 保持 "false"：鼠标仍能点背景，卡片对辅助技术不是真正的模态，
    // 声明成 true 会让读屏软件隐藏仍然可操作的内容。
    if (event.key === "Tab") {
      const focusables = confirmationFocusables(card);
      if (!focusables.length) return;
      event.preventDefault();
      const index = focusables.indexOf(document.activeElement);
      const step = event.shiftKey ? -1 : 1;
      const next = focusables[(index + step + focusables.length) % focusables.length];
      next?.focus();
      return;
    }
    if (event.key === "Enter" && card.contains(document.activeElement)) {
      event.preventDefault();
      closeConfirmation(true);
    }
  });

  async function api(path, options = {}, timeout = 15000, timeoutHint = "本地请求超时，请重试") {
    const controller = new AbortController();
    const abort = () => controller.abort();
    if (options.signal?.aborted) abort();
    options.signal?.addEventListener("abort", abort, { once: true });
    let timer;
    try {
      return await Promise.race([
        (async () => {
          const response = await fetch(path, { ...options, signal: controller.signal,
            headers: { "Content-Type": "application/json", ...(options.headers || {}) } });
          const text = await response.text();
          let payload = null;
          let parseError = null;
          try { payload = text ? JSON.parse(text) : {}; } catch (error) { parseError = error; }
          if (!response.ok) {
            const error = new Error(payload?.message || text.trim() || `请求失败（${response.status}）`);
            error.status = response.status; error.errorKind = response.status === 504 ? "timeout" : "http"; throw error;
          }
          if (parseError) {
            const error = new Error("响应格式异常，请重试");
            error.name = "ResponseFormatError"; error.errorKind = "invalid-response"; throw error;
          }
          return payload ?? {};
        })(),
        new Promise((_, reject) => { timer = setTimeout(() => {
          const error = new Error(timeoutHint);
          error.name = "TimeoutError"; error.errorKind = "timeout";
          reject(error); controller.abort();
        }, timeout); }),
      ]);
    } catch (error) {
      error.errorKind ||= error.name === "AbortError" ? "canceled" : "network";
      if (typeof window !== "undefined") window.reportFlowDiagnostic?.("local_request_client", "failed", { endpoint: "other", httpStatus: Number(error?.status || 0), errorKind: error.errorKind });
      throw error;
    } finally {
      clearTimeout(timer);
      options.signal?.removeEventListener("abort", abort);
    }
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
      clearTimeout(state.facadeRefreshTimer);
      clearTimeout(state.facadeChallengeRetryTimer);
      state.facadeRefreshTimer = 0;
      state.facadeChallengeRetryTimer = 0;
      state.facadeRefreshDeferred = false;
      state.facadeRequestToken = Number(state.facadeRequestToken || 0) + 1;
      state.facadeController?.abort();
      state.facade = null;
      state.facadeLoadedAt = 0;
      state.champSelectRuntime = null;
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
    if (standardMetrics) standardMetrics.hidden = name === "champselect";
    if (champSelectMetrics) champSelectMetrics.hidden = name !== "champselect";
    if (state.active && subtitle) subtitle.textContent = tabCopy[name];
    requestAnimationFrame(() => appScroll?.scrollTo({ top: Number(state.scroll[name] || 0), behavior: "instant" }));
	if (state.active && state.connected) void loadActiveTab(false);
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

  function loadActiveTab(force = false) {
    const loader = { watch: loadWatch, rig: loadRig, facade: loadFacade, sweep: loadClaims, champselect: loadChampSelect }[state.tab] || loadWatch;
    return loader(force);
  }

  async function loadAll(force = false) {
    if (state.loading && !force) return;
    state.loading = true;
    try {
      const tasks = [loadActiveTab(force)];
      if (state.tab !== "rig") tasks.push(loadRig(force));
      await Promise.allSettled(tasks);
    } finally { state.loading = false; }
  }

  const watchDefinitions = [
    { key: "autoAccept", action: "accept", group: "排队与房间", title: "自动接受对局", phase: "确认对局 · ReadyCheck", phaseKey: "ReadyCheck", description: "匹配到对局时替你点“接受”。延时设为 0 会在毫秒内响应，建议留 1 - 2 秒更接近手动节奏。", control: "delay", min: 0, max: 10000, step: 100, countdownVerb: "接受" },
    { key: "autoReconnect", action: "reconnect", group: "英雄选择与游戏中", title: "断线自动重连", phase: "掉线 · Reconnect", phaseKey: "Reconnect", description: "检测到掉线状态时自动重连正在进行的对局。只尝试一次，失败后交还给你手动处理。", control: "delay", min: 3000, max: 30000, step: 500, countdownVerb: "重连" },
    { key: "autoPlayAgain", action: "play-again", group: "结算", title: "快速下一把", phase: "对局结束后", phaseKey: "EndOfGame", description: "结算后自动回到房间。若同时开启了自动点赞，会等点赞投票完成再返回。", control: "fixed-delay", displayDelayMs: 1575, countdownVerb: "返回" },
    { key: "autoHonor", action: "auto-honor", group: "结算", title: "结算自动点赞", phase: "结算 · 出现点赞票", phaseKey: "EndOfGame", description: "把点赞票投给同一房间的队友。找不到队友时按下方策略处理，任何情况下都不会投给敌方。", control: "honor", countdownVerb: "点赞" },
    { key: "skipCelebration", action: "skip-celebration", group: "结算", title: "跳过任务庆祝", phase: "结算前 · PreEndOfGame", phaseKey: "EndOfGame", description: "结算前的任务庆祝动画会拖慢回到房间的速度，开启后自动跳过这一段。", countdownVerb: "跳过" },
    { key: "positionBroadcast", action: "position-broadcast", group: "英雄选择与游戏中", title: "阵营位置播报", phase: "英雄选择 · 大乱斗类模式", phaseKey: "ChampSelect", description: "在极地大乱斗、海克斯大乱斗的英雄选择聊天中发送。每局一次，多项合为一条；缺失项省略，发送失败不重发。", control: "visibility", countdownVerb: "播报" },
    { key: "promoteLeader", action: "promote-leader", group: "排队与房间", title: "房主自动转交", phase: "房间 · 自己是房主", phaseKey: "Lobby", description: "组队时把房主让给别人。优先转给已准备的成员，全员未准备时才随机挑一个。", countdownVerb: "转交" },
    { key: "invitations", action: "invitations", group: "排队与房间", title: "房间邀请处理", phase: "收到邀请", phaseKey: "Lobby", description: "按队列类型分别设定接受 / 拒绝 / 不处理。默认全部“不处理”，需要你逐个队列开。", control: "invitations", countdownVerb: "处理" },
    { key: "autoMatchmaking", action: "auto-matchmaking", group: "排队与房间", title: "自动开始匹配", phase: "房间 · 满足人数且可开始", phaseKey: "Lobby", description: "作为房主时自动开始匹配。不提供“排队超时自动重排”，避免形成无人干预的循环排队。", control: "matchmaking", countdownVerb: "匹配" },
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
      const wait = formatDelay(definition.displayDelayMs).replace(" s", " 秒");
      return `<div class="watch-delay-control is-fixed" aria-label="${escapeHTML(definition.title)}需等待 ${wait} 后返回房间"><span>需等待</span><span class="suite-chip watch-delay-value">${wait}</span><span class="watch-delay-fixed-note">后可开始下一把</span></div>`;
    }
    if (definition.control === "honor") {
      return watchChoiceButtons("autoHonor.strategy", rule.strategy, [["prefer-party", "优先房间队友"], ["party-only", "仅房间队友"], ["any-teammate", "任意队友"], ["abstain", "弃票"]], "点赞策略");
    }
    if (definition.control === "visibility") {
      const options = (state.watch?.broadcastOptions || []).map(option => {
        const fixed = option.key === "camp";
        const disabled = fixed || (option.teamOnly && rule.visibility !== "team");
        return `<label class="watch-broadcast-option"><input type="checkbox" data-watch-broadcast="${escapeHTML(option.key)}"${fixed || rule[option.key] ? " checked" : ""}${disabled ? " disabled" : ""}> ${escapeHTML(option.label)}<small>发送格式：${escapeHTML(option.template)}</small></label>`;
      }).join("");
      return watchChoiceButtons("positionBroadcast.visibility", rule.visibility, [["self", "仅自己可见"], ["team", "发到队伍频道"]], "播报范围") + `<div class="watch-broadcast-options">${options}</div>`;
    }
    if (definition.control === "matchmaking") {
      return `<div class="watch-parameter-fields"><label><span>最少人数</span><span class="select-wrap"><select class="suite-select watch-compact-select" data-watch-number="autoMatchmaking.minPartySize">${[1,2,3,4,5].map((value) => `<option value="${value}"${selected(Number(rule.minPartySize), value)}>${value}</option>`).join("")}</select></span></label><label><span>延时</span><span class="select-wrap"><select class="suite-select watch-compact-select" data-watch-number="autoMatchmaking.delayMs">${[0,1000,3000,5000,10000,15000,30000].map((value) => `<option value="${value}"${selected(Number(rule.delayMs), value)}>${formatDelay(value)}</option>`).join("")}</select></span></label></div>`;
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
    if (typeof watchDiagnostic === "function") watchDiagnostic("rendered");
    const customPaused = settings.customPaused === true;
    const enabled = watchDefinitions.filter((definition) => settings.rules[definition.key]?.enabled).length;
    metrics.watch.textContent = settings.masterEnabled ? `${enabled} / ${watchDefinitions.length}` : "已暂停";
    const groups = ["排队与房间", "英雄选择与游戏中", "结算"];
    roots.watch.className = "";
    roots.watch.innerHTML = `
      <section class="suite-card watch-hero">
        <div class="watch-hero-copy"><span class="watch-sigil" aria-hidden="true">◎</span><div><h2>${customPaused ? "自定义对局 · 自动规则已暂停" : settings.masterEnabled ? "自动规则运行中" : "自动规则已暂停"}</h2><p>${customPaused ? "自定义对局全程不触发自动操作；回到普通房间后恢复，保留你的开关设置。" : settings.masterEnabled ? "客户端已连接，规则会在对应阶段自动触发。" : "规则配置仍会保存，但总开关关闭时不会写入客户端。"}</p></div></div>
        <div class="watch-summary"><div><small>本次启动已代劳</small><strong>${state.watchFired} 次</strong></div><div><small>最近一次</small><strong>${escapeHTML(latestWatchAction())}</strong></div><label class="suite-switch watch-master-control"><input type="checkbox" data-watch-master${checked(settings.masterEnabled)}><span>总开关</span></label></div>
      </section>
      ${settings.masterEnabled ? "" : '<div class="suite-note is-warning watch-paused-note"><span aria-hidden="true">!</span><span><strong>自动规则已暂停</strong><br>下方开关仍可预先配置；重新打开总开关后才会触发。</span></div>'}
      <div class="phase-track" aria-label="客户端相位">${phaseOrder.map((phase, index) => {
        const current = phase === suiteDisplayPhase(state.phase);
        const currentIndex = phaseOrder.indexOf(suiteDisplayPhase(state.phase));
        return `<div class="phase-node${current ? " is-current" : ""}${currentIndex > index ? " is-complete" : ""}" data-watch-phase="${phase}"><i aria-hidden="true"></i><span>${phaseNames[phase]}</span></div>`;
      }).join("")}</div>
      ${state.watchPriority ? '<div class="suite-note is-warning watch-priority"><span aria-hidden="true">!</span><span><strong>点赞中 · 下一把已顺延</strong><br>点赞完成后才会执行快速下一把，两条规则不会互抢。</span></div>' : ""}
      <header class="watch-rules-head"><div><h2>规则</h2><p>总开关打开时，已启用规则保持高亮；自定义对局暂停执行，但保留配置</p></div><span class="suite-chip is-warning">${settings.masterEnabled ? `${enabled} 条已启用` : `${enabled} 条已配置 · 已暂停`}</span></header>
      ${groups.map((group) => `<section class="watch-group"><header><h3>${group}</h3></header><div class="watch-rule-grid">${watchDefinitions.filter((definition) => definition.group === group).map((definition) => watchRuleCard(definition, settings.rules[definition.key] || {}, settings.masterEnabled, settings.rules)).join("")}</div></section>`).join("")}`;
    bindWatchControls();
    applyWatchControlStyles();
    updateCountdowns();
  }

  const watchActionNames = {
    "champselect-ban": "自动禁用",
    "champselect-pick": "自动选用",
    "champselect-bench": "备战席换英雄",
    "champselect-trade": "英雄交换",
  };

  function watchActionTitle(action) {
    return watchDefinitions.find((definition) => definition.action === action)?.title
      || watchActionNames[action]
      || "自动规则";
  }

  function watchActionElapsed(start) {
    const elapsed = Math.max(0, Date.now() - Number(start ?? Date.now()));
    if (elapsed < 5000) return "刚刚";
    const seconds = Math.floor(elapsed / 1000);
    if (seconds < 60) return `${seconds}秒前`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}分钟前`;
    return `${Math.floor(minutes / 60)}小时前`;
  }

  function latestWatchAction() {
    let latest = null;
    for (const [action, event] of state.watchEvents) {
      if (event.kind !== "fired" || (latest && latest.event.start >= event.start)) continue;
      latest = { action, event };
    }
    if (!latest) return "暂无触发";
    return `${watchActionTitle(latest.action)} · ${watchActionElapsed(latest.event.start)}`;
  }

  function watchConflictNote(definition, rules) {
    if (definition.key !== "autoMatchmaking" || !rules?.promoteLeader?.enabled || !rules?.autoMatchmaking?.enabled) return "";
    return '<p class="watch-conflict-note">已开启“房主自动转交”，转交后你不再是房主，本规则会被跳过</p>';
  }

  function watchRuleCard(definition, rule, masterEnabled, rules) {
    rules ||= {};
    const customPaused = state.watch?.customPaused === true;
    const event = customPaused ? null : state.watchEvents.get(definition.action);
    const current = !customPaused && (definition.phaseKey === suiteDisplayPhase(state.phase) || (definition.phaseKey === "Reconnect" && state.phase === "Reconnect"));
    const statusClass = `${rule.enabled ? " is-enabled" : ""}${masterEnabled ? "" : " is-paused"}${event?.kind === "failed" ? " is-failed" : current ? " is-current" : ""}`;
    const eventStatus = customPaused ? '<span>自定义对局已暂停</span>' : event?.kind === "armed" ? `<span>等待触发</span><span class="watch-countdown" data-watch-countdown="${definition.action}" data-watch-verb="${definition.countdownVerb || "触发"}"><b>准备触发</b></span>`
      : event?.kind === "failed" ? '<span class="suite-chip is-danger">上次触发失败</span>'
      : event?.kind === "skipped" ? `<span>${escapeHTML(event.message)}</span>`
      : event?.kind === "fired" ? '<span>上次触发 · 刚刚</span>'
      : event?.kind === "canceled" ? '<span>已被更高优先级动作取消 · 准备下次触发</span>'
      : current ? `<span>${rule.enabled ? '正在评估当前阶段' : '已到触发阶段 · 规则尚未开启'}</span>` : '<span>尚未到触发阶段</span>';
    const status = masterEnabled ? eventStatus : '<span class="suite-chip is-warning">总开关已关闭 · 已暂停</span>';
    const control = watchRuleControl(definition, rule);
    return `<article class="suite-card watch-rule${statusClass}" data-watch-card="${definition.action}"><div class="watch-rule-top"><div class="watch-rule-title"><h3>${definition.title}</h3><small>触发于 <b>${definition.phase}</b></small></div><label class="suite-switch watch-rule-toggle"><input type="checkbox" aria-label="启用${definition.title}" data-watch-toggle="${definition.key}"${checked(rule.enabled)}><span class="sr-only">启用</span></label></div><p>${definition.description}</p>${watchConflictNote(definition, rules)}${control ? `<div class="watch-control-row">${control}</div>` : ""}<div class="watch-rule-foot">${status}</div></article>`;
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
    for (const input of roots.watch.querySelectorAll("[data-watch-broadcast]")) input.addEventListener("change", async () => {
      if (!["teamComposition", "assignedPosition"].includes(input.dataset.watchBroadcast)) return;
      state.watch.rules.positionBroadcast[input.dataset.watchBroadcast] = input.checked;
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
    for (const input of roots.watch.querySelectorAll("[data-watch-delay]")) setWatchRangeFill(input);
  }

  async function loadWatch(force = false) {
    if (state.watchPendingSaves > 0) { if (typeof watchDiagnostic === "function") watchDiagnostic("load-skipped"); return; }
    if (state.watch && !force) { renderWatch(); return; }
	if (state.watchPromise) return state.watchPromise;
	const revision = state.watchRevision || 0;
	if (typeof watchDiagnostic === "function") watchDiagnostic("load-started");
	state.watchPromise = (async () => {
	  try {
      const response = await api("/api/watch/rules", undefined, 15000, "自动规则读取超时，请重试");
		if ((state.watchRevision || 0) !== revision || state.watchPendingSaves > 0) { if (typeof watchDiagnostic === "function") watchDiagnostic("load-stale"); return; }
		state.watch = response;
		if (typeof watchDiagnostic === "function") watchDiagnostic("load-applied");
		renderWatch();
	  } catch (error) {
		if (typeof watchDiagnostic === "function") watchDiagnostic("load-failed");
		roots.watch.innerHTML = errorCard("自动规则读取失败", error.message, "watch");
	  } finally {
		state.watchPromise = null;
	  }
	})();
	return state.watchPromise;
  }

  async function saveWatch() {
    try {
      await persistWatchSettings();
      renderWatch();
      toast("自动规则已保存");
    } catch (error) {
      toast(error.message);
      await loadWatch(true);
    }
  }

  function watchSettingsPayload() {
    return { schemaVersion: state.watch.schemaVersion, masterEnabled: state.watch.masterEnabled, rules: state.watch.rules, facade: state.watch.facade, champSelect: state.watch.champSelect };
  }

  function watchDiagnostic(reason) {
    window.reportFlowDiagnostic?.("watch_settings_client", reason, { revision: state.watchRevision || 0, pendingSaves: state.watchPendingSaves || 0, masterEnabled: state.watch?.masterEnabled === true, customPaused: state.watch?.customPaused === true, champSelectEnabled: state.watch?.champSelect?.enabled === true, autoMatchmakingEnabled: state.watch?.rules?.autoMatchmaking?.enabled === true });
  }

  async function persistWatchSettings() {
    // Capture each user edit now; serialize POSTs and never let an earlier
    // response (or an in-flight GET) replace a newer local toggle.
    const body = JSON.stringify(watchSettingsPayload());
    const revision = (state.watchRevision || 0) + 1;
    state.watchRevision = revision;
    state.watchPendingSaves = (state.watchPendingSaves || 0) + 1;
    if (typeof watchDiagnostic === "function") watchDiagnostic("save-queued");
    const task = Promise.resolve(state.watchSaveQueue).catch(() => {}).then(() => api("/api/watch/rules", { method: "POST", body }, 15000, "自动规则保存超时，请重试"));
    state.watchSaveQueue = task;
    try {
      const response = await task;
      if (state.watchRevision === revision) state.watch = response;
      if (typeof watchDiagnostic === "function") watchDiagnostic("save-succeeded");
      return response;
    } catch (error) {
      if (typeof watchDiagnostic === "function") watchDiagnostic("save-failed");
      throw error;
    } finally {
      state.watchPendingSaves -= 1;
    }
  }

  function champSelectLaneName(value) {
    return ({ top: "上单", jungle: "打野", middle: "中单", bottom: "下路", utility: "辅助", default: "未分配位置" })[value] || value || "未分配位置";
  }

  function champSelectSideName(value) { return value === "ban" ? "禁用" : "选用"; }

  function champSelectChampionMeta(id, catalog) {
    return (catalog?.champions || []).find((champion) => Number(champion.id) === Number(id)) || null;
  }

  function champSelectChampionImage(champion, className = "cs-avatar") {
	if (!champion) return '<span class="cs-avatar cs-avatar-fallback" aria-hidden="true">?</span>';
	const helper = window.deepLegendsChampionAsset?.imageHTML;
	if (typeof helper === "function") return helper({ source: champion.imageSource, path: champion.imagePath, name: champion.nameZh || champion.nameEn || `英雄 ${champion.id}` }, className, false);
	return `<span class="${className} cs-avatar-fallback" aria-hidden="true">${escapeHTML((champion.nameZh || champion.nameEn || "?").slice(0, 1))}</span>`;
  }

  function champSelectSettings() {
    if (!state.watch) return null;
    state.watch.champSelect ||= { enabled: false, groups: {} };
    state.watch.champSelect.groups ||= {};
    return state.watch.champSelect;
  }

  function champSelectSelectedDefinition() {
    const groups = state.champSelectGroups || [];
    return groups.find((group) => group.groupId === state.champSelectGroup) || groups[0] || null;
  }

  function champSelectSelectedConfig() {
    const settings = champSelectSettings();
    const definition = champSelectSelectedDefinition();
    return definition ? settings?.groups?.[definition.groupId] || null : null;
  }

  function champSelectPoolLane(side, definition) {
    const lane = state.champSelectLane[side];
    return side === "pick" && definition?.positions?.includes(lane) ? lane : "default";
  }

  function champSelectPoolFor(side, definition = champSelectSelectedDefinition(), config = champSelectSelectedConfig()) {
    if (!definition || !config) return [];
    const lane = champSelectPoolLane(side, definition);
    return config[side]?.champions?.[lane] || [];
  }

  function champSelectConfiguredCount(config, side) {
    if (side === "ban") return config?.ban?.champions?.default?.length || 0;
    const pools = Object.values(config?.[side]?.champions || {});
    return pools.reduce((largest, pool) => Math.max(largest, Array.isArray(pool) ? pool.length : 0), 0);
  }

  function champSelectStatusCopy(status, side, active, pickIntent = false) {
    if (status === "manual-takeover") return "玩家主动切换";
    if (active) return side === "ban" ? "即将禁用" : pickIntent ? "提前预选中" : "即将选用";
    return ({ available: side === "ban" ? "可禁用" : "可选", "verify-hover": "待亮出验证", "own-intent": "自己准备选用", "list-empty": "等待禁用列表", "teammate-picked": "队友已经选择", intent: "队友预选中", gone: "已被禁用或拿走", unavailable: side === "ban" ? "不在客户端可禁用列表" : "不在客户端可选列表" })[status] || "备用";
  }

  function champSelectSlotsHTML(side, champions, limit, runtime, catalog) {
    const states = side === "ban" ? runtime?.banStates || {} : runtime?.pickStates || {};
    const activeID = Number(side === "ban" ? runtime?.activeBanId : runtime?.activePickId);
    const activeIndex = champions.findIndex((id) => Number(id) === activeID);
    const parts = [];
    for (let index = 0; index < limit; index += 1) {
      const championID = Number(champions[index] || 0);
      if (index > 0) parts.push(`<span class="cs-rail-arrow${activeIndex >= index ? " is-hot" : ""}" aria-hidden="true">›</span>`);
      if (!championID) {
        parts.push(`<button class="cs-rail-slot is-empty" type="button" data-cs-open-dialog="${side}" aria-label="添加第 ${index + 1} 个${champSelectSideName(side)}备选"><span class="cs-slot-order">${index + 1}</span><span class="cs-add-slot" aria-hidden="true">＋</span><span class="cs-champion-name">添加</span><span class="cs-champion-state">空槽</span></button>`);
        continue;
      }
      const champion = champSelectChampionMeta(championID, catalog);
      const brave = championID === -3;
      const status = brave ? "available" : states[String(championID)] || "";
      const active = championID === activeID;
      const statusClass = ({ available: " st-ok", "manual-takeover": " st-intent", "verify-hover": " st-intent", "own-intent": " st-intent", "list-empty": " st-intent", "teammate-picked": " st-gone", intent: " st-intent", gone: " st-gone", unavailable: " st-gone" })[status] || "";
      const name = brave ? "勇敢举动" : champion?.nameZh || champion?.nameEn || `英雄 ${championID}`;
      const artwork = brave ? '<span class="cs-avatar cs-brave-avatar" aria-hidden="true">⚔</span>' : champSelectChampionImage(champion);
      parts.push(`<div class="cs-rail-slot${statusClass}${active ? " is-live" : ""}" data-cs-champion-id="${championID}" data-cs-rail-side="${side}" data-cs-rail-index="${index}" draggable="true" title="拖动到同序列的另一位英雄上交换位置"><span class="cs-slot-order">${index + 1}</span><button class="cs-slot-open" type="button" data-cs-open-dialog="${side}" aria-label="编辑${champSelectSideName(side)}序列">${artwork}<span class="cs-champion-name">${escapeHTML(name)}</span><span class="cs-champion-state"${status === "manual-takeover" ? ' title="玩家主动切换，本轮已停止自动操作；下一局恢复"' : ""}>${escapeHTML(champSelectStatusCopy(status, side, active, runtime?.pickIntent))}</span></button><button class="cs-slot-remove" type="button" data-cs-remove="${side}" data-cs-remove-index="${index}" aria-label="移除${escapeHTML(name)}">×</button></div>`);
    }
    parts.push(`<span class="cs-rail-tail">${side === "ban" ? "全部不可用时不发送请求，并在本局记录说明原因" : "你主动换成别的英雄后，本轮停止自动选用和换人，下一局恢复"}</span>`);
    return parts.join("");
  }

  function champSelectStrategyHTML(side, current, blocked) {
    const options = [["show-only", "仅亮出"], ["show-then-lock", "亮出后锁定"], ["lock-now", "立即锁定"]];
    return `<div class="cs-segment" role="group" aria-label="${champSelectSideName(side)}策略">${options.map(([value, label]) => `<button class="${current === value ? "is-on" : ""}" type="button" data-cs-strategy="${side}" data-cs-strategy-value="${value}" aria-pressed="${current === value}"${blocked ? " disabled" : ""}>${label}</button>`).join("")}</div>`;
  }

  function champSelectLaneTabsHTML(side, definition, config) {
    if (side === "ban" || (definition?.positions || []).length <= 1) return "";
    const active = state.champSelectLane[side];
    return `<div class="cs-lane-tabs" role="tablist" aria-label="${champSelectSideName(side)}分路">${definition.positions.map((lane) => `<button class="cs-lane-tab${active === lane ? " is-on" : ""}" type="button" role="tab" aria-selected="${active === lane}" data-cs-lane-side="${side}" data-cs-lane="${lane}">${champSelectLaneName(lane)} <span>${config?.[side]?.champions?.[lane]?.length || 0}</span></button>`).join("")}</div>`;
  }

  function renderChampSelectSideCard(side, definition, config, runtime) {
    const isBan = side === "ban";
    if (isBan && !definition.hasBan) return "";
    const runtimeNoBan = isBan && definition.groupId === runtime?.groupId && runtime?.banCapabilityKnown && !runtime?.hasBanAction;
    const blocked = isBan && (!definition.hasBan || runtimeNoBan);
    const sideConfig = config?.[side] || {};
    const strategy = sideConfig.strategy || "show-then-lock";
    const lockWait = strategy === "show-then-lock";
    const delayKey = "lockDelayMs";
    const timingDisabled = blocked || !lockWait;
    const timingControls = `<span class="cs-control-label">锁定等待</span><div class="cs-stepper"><button type="button" data-cs-delay="${side}" data-cs-delay-key="${delayKey}" data-cs-delay-delta="-500"${timingDisabled ? " disabled" : ""}>−</button>${champSelectTimeInputHTML("lock", lockWait ? sideConfig[delayKey] ?? 10000 : 0, timingDisabled)}<button type="button" data-cs-delay="${side}" data-cs-delay-key="${delayKey}" data-cs-delay-delta="500"${timingDisabled ? " disabled" : ""}>＋</button></div>`;
    const yielded = runtime.active && runtime.groupId === definition.groupId && Object.values(isBan ? runtime.banStates || {} : runtime.pickStates || {}).includes("manual-takeover");
    const limit = isBan ? definition.banLimit : definition.pickLimit;
    const pool = champSelectPoolFor(side, definition, config);
    const blockedCopy = !definition.hasBan ? "本模式无禁用环节" : runtimeNoBan ? "客户端会话未发现禁用环节" : "";
    return `<section class="suite-card cs-seq-card is-${side}${sideConfig.enabled ? "" : " is-off"}${blocked ? " is-blocked" : ""}">
      <header class="cs-seq-head"><div class="cs-seq-title"><span class="cs-seq-mark">${isBan ? "禁" : "选"}</span><div><h3>${champSelectSideName(side)}序列</h3><p>${blocked ? blockedCopy : isBan ? definition.positions?.length > 1 ? "所有位置共用 5 个禁用备选，按顺序尝试" : "正式禁用阶段按顺序尝试；自定义兼容时先验证亮出" : "预选阶段自动提前亮出；轮到自己时按策略亮出或锁定"}</p></div></div>
      <div class="cs-seq-controls"><span class="cs-control-label">策略</span>${champSelectStrategyHTML(side, strategy, blocked)}${timingControls}<label class="suite-switch"><input type="checkbox" aria-label="启用自动${champSelectSideName(side)}" data-cs-side-enabled="${side}"${checked(sideConfig.enabled && !blocked)}${blocked ? " disabled" : ""}><span class="sr-only">启用</span></label></div></header>
      ${champSelectLaneTabsHTML(side, definition, config)}
      <p class="cs-execution-state">${yielded ? `玩家主动切换，本轮已停止自动${isBan ? "禁用" : "选用和换人"}，下一局恢复` : sideConfig.enabled && !blocked ? `自动${champSelectSideName(side)}已开启 · ${runtime.active && runtime.groupId === definition.groupId ? runtime.pickIntent ? isBan ? "等待正式禁用阶段" : "预选阶段，自动提前亮出序列英雄" : runtime.actionType === side ? "当前为自己的操作回合" : "等待自己的操作回合" : "等待英雄选择"}` : `自动${champSelectSideName(side)}未开启，配置的英雄不会自动提交`}</p>
      <div class="cs-rail">${champSelectSlotsHTML(side, pool, limit, runtime, state.champSelectCatalog)}</div>
    </section>`;
  }

  function champSelectTimeInputHTML(kind, milliseconds, disabled = false) {
    const minimum = kind === "hold" ? 1 : 0;
    const seconds = Math.max(minimum, Math.min(10, Number(milliseconds ?? minimum * 1000) / 1000));
    const label = kind === "hold" ? "备战席停留时间（秒）" : kind === "lock" ? "亮出后锁定等待时间（秒）" : `${champSelectSideName(kind)}延时（秒）`;
    return `<label class="cs-time-field"><input class="cs-time-input" type="number" inputmode="decimal" min="${minimum}" max="10" step="any" value="${seconds}" aria-label="${label}" data-cs-time="${kind}"${disabled ? " disabled" : ""}><span class="cs-time-unit" aria-hidden="true">s</span></label>`;
  }

  function renderChampSelectBenchCard(definition, config, runtime) {
    const applicable = Boolean(definition?.hasBench);
    const tradeApplicable = Boolean(definition?.hasTrade);
    if (!applicable && !tradeApplicable) return "";
    const bench = config?.bench || {};
    const liveUnavailable = runtime?.active && runtime.groupId === definition.groupId && !runtime.benchEnabled;
    return `<section class="suite-card cs-seq-card cs-bench-card">
      <header class="cs-seq-head"><div class="cs-seq-title"><span class="cs-seq-mark">换</span><div><h3>${applicable ? "备战席与交换" : "英雄交换"}</h3><p>${applicable ? liveUnavailable ? "当前会话没有备战席，配置将在支持时生效" : "自动争取序列内英雄，并按优先级处理队友交换" : "按选用序列处理队友英雄交换请求"}</p></div></div><span class="suite-chip">${applicable ? liveUnavailable ? "本局无备战席" : "可配置" : "支持英雄交换"}</span></header>
      <div class="cs-bench-grid">
        ${applicable ? `<div class="cs-bench-row"><span><strong>自动从备战席换英雄</strong><small>目标停留达到阈值后交换（1–10 秒）；你主动换成别的英雄后，本轮停止自动换人。</small></span><div class="cs-bench-control"><div class="cs-stepper"><button type="button" data-cs-hold-delta="-500" aria-label="备战席停留时间减少 0.5 秒">−</button>${champSelectTimeInputHTML("hold", bench.holdMs)}<button type="button" data-cs-hold-delta="500" aria-label="备战席停留时间增加 0.5 秒">＋</button></div><label class="suite-switch"><input type="checkbox" data-cs-bench-enabled${checked(bench.enabled)}><span class="sr-only">自动从备战席换英雄</span></label></div></div>` : ""}
        <div class="cs-bench-row"><span><strong>优先选用序列靠前的英雄</strong><small>${applicable ? "备战席换取和队友交换均遵循此优先级；" : "启用自动处理换英雄请求后生效；"}手上已有序列英雄时，只换取排序更靠前的目标。</small></span><label class="suite-switch"><input type="checkbox" data-cs-bench-prefer${checked(bench.preferFirst)}><span class="sr-only">优先靠前英雄</span></label></div>
        <div class="cs-bench-row"><span><strong>自动处理换英雄请求</strong><small>适用于队友发起的英雄交换请求，与备战席无关；按选用序列判断接受或拒绝。默认关闭。</small></span><label class="suite-switch"><input type="checkbox" data-cs-bench-trade${checked(bench.handleTrade)}${tradeApplicable ? "" : " disabled"}><span class="sr-only">自动处理换英雄请求</span></label></div>
      </div></section>`;
  }

  function champSelectRecordMessage(record, catalog) {
    // Old runtime/demo records lack championId. Only replace an explicit
    // champion token, never delay seconds, retry counts or other numbers.
    return String(record?.message || "").replace(/英雄\s+(-?\d+)(?!\d)/g, (token, value) => {
      const id = Number(value);
      if (record.championId != null && Number(record.championId) !== id) return token;
      if (id === -3) return "勇敢举动";
      const champion = champSelectChampionMeta(id, catalog);
      return champion?.nameZh || champion?.nameEn || "未知英雄（资料暂缺）";
    });
  }

  function renderChampSelectTimeline(runtime) {
    const records = runtime?.records || [];
    const rows = records.length ? records.slice().reverse().map((record) => {
      const at = record.at ? new Date(record.at) : null;
      const time = at && Number.isFinite(at.getTime()) ? at.toLocaleTimeString("zh-CN", { hour12: false }) : "—";
      const kind = ["ok", "warn", "fail"].includes(record.kind) ? record.kind : "";
      return `<div class="cs-live-row ${kind}"><span class="cs-live-time">${escapeHTML(time)}</span><span class="cs-live-bullet"><i></i></span><span>${escapeHTML(champSelectRecordMessage(record, state.champSelectCatalog))}</span></div>`;
    }).join("") : '<div class="cs-live-empty">进入英雄选择后，这里会记录模式识别、顺延、执行与让位原因。</div>';
    const phase = runtime?.active ? `${runtime.groupId === "unsupported" ? "未匹配模式" : "英雄选择"}${Number(runtime.remainingMs) >= 0 ? ` · 剩余 ${(Number(runtime.remainingMs) / 1000).toFixed(0)} 秒` : ""}` : "等待英雄选择";
    return `<section class="suite-card cs-live-card"><header><div class="cs-seq-title"><span class="cs-live-mark" aria-hidden="true">◷</span><div><h3>本局记录</h3><p>离开英雄选择后清空，不落盘</p></div></div><span class="suite-chip${runtime?.active ? " is-gold" : ""}">${escapeHTML(phase)}</span></header><div class="cs-live-timeline">${rows}</div></section>`;
  }

  function renderChampSelectDialog() {
    const dialog = state.champSelectDialog;
    if (!dialog) return "";
    const definition = state.champSelectGroups.find(group => group.groupId === dialog.group) || champSelectSelectedDefinition();
    const limit = dialog.side === "ban" ? definition.banLimit : definition.pickLimit;
    const selectedIDs = new Set(dialog.draft.map(Number));
    const rows = champSelectFilteredChampions(dialog, state.champSelectCatalog, state.champSelectPositionIDs);
    const brave = definition.groupId === "arena" && dialog.side === "pick";
    const grid = `${brave ? `<button class="cs-champion-cell cs-brave-cell${selectedIDs.has(-3) ? " is-picked" : ""}" type="button" data-cs-dialog-add="-3"><span class="cs-avatar cs-brave-avatar">⚔</span><span>勇敢举动</span></button>` : ""}${rows.map((champion) => {
      const selected = selectedIDs.has(Number(champion.id));
      const owned = state.champSelectRuntime?.owned?.[String(champion.id)];
      return `<button class="cs-champion-cell${selected ? " is-picked" : ""}${owned === false ? " is-dim" : ""}" type="button" data-cs-dialog-add="${champion.id}"${!selected && dialog.draft.length >= limit ? " disabled" : ""}>${champSelectChampionImage(champion, "cs-dialog-avatar")}<span>${escapeHTML(champion.nameZh || champion.nameEn || `英雄 ${champion.id}`)}</span></button>`;
    }).join("")}`;
    const chosen = dialog.draft.map((id, index) => {
      const champion = champSelectChampionMeta(id, state.champSelectCatalog);
      const name = Number(id) === -3 ? "勇敢举动" : champion?.nameZh || champion?.nameEn || `英雄 ${id}`;
      const image = Number(id) === -3 ? '<span class="cs-dialog-avatar cs-brave-avatar">⚔</span>' : champSelectChampionImage(champion, "cs-dialog-avatar");
      return `<div class="cs-chosen-row" draggable="true" data-cs-drag-index="${index}"><span class="cs-grip" aria-hidden="true">⠿</span>${image}<span>${escapeHTML(name)}</span><button type="button" data-cs-dialog-remove="${index}" aria-label="移除${escapeHTML(name)}">×</button></div>`;
    }).join("");
    const ghost = dialog.draft.length < limit ? `<div class="cs-chosen-ghost"><span>${dialog.draft.length + 1}</span><span>＋</span><span>还可再加 ${limit - dialog.draft.length} 个</span></div>` : "";
    return `<dialog class="cs-dialog" aria-labelledby="cs-dialog-title"><div class="cs-dialog-sheet"><header><div><h3 id="cs-dialog-title">编辑${champSelectSideName(dialog.side)}序列 · ${escapeHTML(definition.name)}${dialog.side === "pick" && definition.positions.length > 1 ? ` › ${champSelectLaneName(dialog.lane)}` : ""}</h3><p>点击左侧添加到末尾，拖动右侧手柄调整优先级</p></div><button class="icon-button control-icon-button" type="button" data-cs-dialog-close aria-label="关闭"><svg class="control-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button></header><div class="cs-dialog-body"><div class="cs-dialog-left"><label class="cs-search"><span aria-hidden="true">⌕</span><input type="search" value="${escapeHTML(dialog.query || "")}" placeholder="搜索中文、拼音、英文、缩写或外号" data-cs-dialog-search></label><div class="cs-filter-row">${[["all", "全部"], ["top", "上单"], ["jungle", "打野"], ["middle", "中单"], ["bottom", "下路"], ["utility", "辅助"]].map(([value, label]) => `<button class="suite-chip${state.champSelectPosition === value ? " is-gold" : ""}" type="button" data-cs-dialog-position="${value}">${label}</button>`).join("")}</div><div class="cs-champion-grid">${grid || '<p class="cs-dialog-empty">没有符合条件的英雄</p>'}</div></div><aside class="cs-dialog-right"><div class="cs-chosen-head"><strong>已选序列</strong><span>${dialog.draft.length} / ${limit}</span></div><div class="cs-chosen-list">${chosen}${ghost}</div><button class="text-button cs-dialog-clear" type="button" data-cs-dialog-clear>清空全部</button></aside></div><footer><span>${dialog.side === "ban" ? "灰显表示当前账号未拥有，仍可加入禁用序列" : "实际选用时会自动跳过不可用英雄"}</span><div><button class="button button-secondary" type="button" data-cs-dialog-close>取消</button><button class="button button-primary" type="button" data-cs-dialog-save>保存序列</button></div></footer></div></dialog>`;
  }

  function champSelectPanelRoot() {
    let body = roots.champselect.querySelector(":scope > [data-cs-panel-body]");
    if (!body) {
      body = document.createElement("div");
      body.dataset.csPanelBody = "";
      body.style.display = "contents";
      roots.champselect.replaceChildren(body);
    }
    return body;
  }

  function champSelectDialogDiagnostic(reason) {
    const revision = state.champSelectDialogRevision = (state.champSelectDialogRevision || 0) + 1;
    window.reportFlowDiagnostic?.("champselect_dialog_client", reason, { revision });
  }

  function syncChampSelectDialog() {
    const existing = roots.champselect.querySelector(".cs-dialog");
    const draft = state.champSelectDialog;
    if (!draft || state.destroyed) {
      if (existing) {
        existing.close();
        existing.remove();
        champSelectDialogDiagnostic("close");
      }
      return;
    }
    if (existing) {
      // Runtime events update the panel only. Preserve the open modal subtree.
      champSelectDialogDiagnostic("rerender-while-open");
      return;
    }
    roots.champselect.insertAdjacentHTML("beforeend", renderChampSelectDialog());
    const dialogElement = roots.champselect.querySelector(".cs-dialog");
    bindChampSelectDialog();
    champSelectDialogDiagnostic("open");
    requestAnimationFrame(() => {
      if (!state.destroyed && state.champSelectDialog === draft && dialogElement.isConnected && !dialogElement.open) dialogElement.showModal();
    });
  }

  function updateChampSelectDialog() {
    const existing = roots.champselect.querySelector(".cs-dialog");
    if (state.destroyed || !existing || !state.champSelectDialog) return;
    const template = document.createElement("template");
    template.innerHTML = renderChampSelectDialog();
    // Only explicit edits/filter completions update these fragments. The native
    // search input, selection, composition and modal scroll position stay mounted.
    for (const selector of [".cs-champion-grid", ".cs-chosen-head", ".cs-chosen-list", ".cs-filter-row"]) {
      const current = existing.querySelector(selector);
      const next = template.content.querySelector(selector);
      if (current.innerHTML !== next.innerHTML) current.innerHTML = next.innerHTML;
    }
  }

  function closeChampSelectDialog() {
    state.champSelectPositionController?.abort();
    state.champSelectPositionController = null;
    state.champSelectDialog = null;
    syncChampSelectDialog();
  }

  function champSelectFilteredChampions(dialog, catalog, positionIDs) {
    let rows = [...(catalog?.champions || [])];
    if (positionIDs instanceof Set) rows = rows.filter((champion) => positionIDs.has(Number(champion.id)));
    const query = String(dialog?.query || "").trim();
    if (query) {
      const scorer = window.deepLegendsChampionSearch?.scoreOption;
      rows = rows.map((champion, index) => ({ champion, index, score: typeof scorer === "function" ? scorer(query, champion.id, champion.nameZh || champion.nameEn) : String(champion.nameZh || champion.nameEn || "").includes(query) ? 1 : 0 })).filter((item) => item.score > 0).sort((a, b) => b.score - a.score || a.index - b.index).map((item) => item.champion);
    }
    return rows;
  }

  function champSelectPoolHelpHTML(definition, runtime) {
    let text = definition.positions.length > 1
      ? "禁用不分位置，共用 5 个备选，按从左到右的顺序尝试。选用仅使用本局被分配位置的序列；位置未知时使用「未分配位置」池。分路页签只用于编辑选用，不决定本局位置。"
      : definition.groupId === "practice"
        ? "自定义（包括征召自定义）与人机仅使用本组配置，不会借用普通征召或排位的英雄池。"
        : definition.groupId === "normal"
          ? "本组仅用于普通匹配 / 征召。自定义测试请配置「人机 / 自定义」分组。"
          : "按本组选用序列从左到右尝试，自动跳过不可用英雄。";
    if (runtime.active && !runtime.unsupported && runtime.groupId !== definition.groupId) {
      const active = state.champSelectGroups.find((group) => group.groupId === runtime.groupId);
      text = `本局使用「${active?.name || runtime.groupId}」，当前正在编辑「${definition.name}」。${text}`;
    }
    return `<p class="cs-pool-help">${escapeHTML(text)}</p>`;
  }

  function bindChampSelectTimeInputs(root) {
    for (const input of root.querySelectorAll("[data-cs-time]")) {
      input.addEventListener("keydown", (event) => {
        if (event.key === "Enter") { event.preventDefault(); input.blur(); }
      });
      input.addEventListener("change", async () => {
        if (!champSelectSettings()?.enabled || input.matches(":disabled")) return;
        const kind = input.dataset.csTime;
        const side = input.closest(".cs-seq-card.is-ban") ? "ban" : "pick";
        const config = kind === "hold" ? champSelectSelectedConfig().bench : champSelectSelectedConfig()[kind === "lock" ? side : kind];
        const key = kind === "hold" ? "holdMs" : kind === "lock" ? "lockDelayMs" : "delayMs";
        const seconds = Number(input.value);
        if (!input.value.trim() || !Number.isFinite(seconds)) {
          input.value = String((config[key] ?? (kind === "lock" ? 10000 : 0)) / 1000);
          toast("请输入有效秒数");
          return;
        }
        const milliseconds = Math.max(kind === "hold" ? 1000 : 0, Math.min(10000, Math.round(seconds * 1000)));
        input.value = String(milliseconds / 1000);
        if (config[key] === milliseconds) return;
        config[key] = milliseconds;
        await saveChampSelect();
      });
    }
  }

  function champSelectCapabilitiesHTML(definition, runtime) {
    runtime ||= {};
    if (runtime.unsupported) return '<span class="suite-chip is-warning">当前模式无对应分组</span>';
    const hasBan = definition.hasBan && !(runtime.active && runtime.groupId === definition.groupId && runtime.banCapabilityKnown && !runtime.hasBanAction);
    return [[hasBan, "有禁用环节"], [definition.positions.length > 1, "选用按分路"], [definition.hasBench, "有备战席"]]
      .filter(([present]) => present).map(([, label]) => `<span class="suite-chip is-success">${label}</span>`).join("");
  }

  function renderChampSelect() {
    if (!roots.champselect || !state.watch?.champSelect || !state.champSelectGroups) return;
    const settings = champSelectSettings();
    const definition = champSelectSelectedDefinition();
    const config = champSelectSelectedConfig();
    const runtime = state.champSelectRuntime || {};
    if (!definition || !config) return;
    // Keep native drag targets mounted across runtime polls/SSE. A settings,
    // group or lane change invalidates the drag instead of writing a stale pool.
    const drag = state.champSelectRailDrag;
    if (drag && settings.enabled && drag.group === definition.groupId && drag.lane === champSelectPoolLane(drag.side, definition) && drag.pool === champSelectPoolFor(drag.side, definition, config) && Date.now() < drag.expiresAt) return;
    state.champSelectRailDrag = null;
    // Runtime refreshes must not replace an input while the user is typing.
    const focusedTime = roots.champselect.querySelector("[data-cs-time]:focus");
    if (settings.enabled && focusedTime && focusedTime.closest("[data-cs-config-group]")?.dataset.csConfigGroup === definition.groupId) return;
    if (!settings.enabled) state.champSelectDialog = null;
    const configured = state.champSelectGroups.filter((group) => {
      const value = settings.groups?.[group.groupId];
      return value && (value.ban?.enabled || value.pick?.enabled || value.bench?.enabled || value.bench?.handleTrade);
    }).length;
    if (champSelectConfiguredMetric) champSelectConfiguredMetric.textContent = String(configured);
    if (champSelectActiveMetric) champSelectActiveMetric.textContent = runtime.active ? runtime.unsupported ? "未匹配" : runtime.groupId === definition.groupId ? "当前" : "其他" : "—";
    const liveLabel = runtime.active ? runtime.unsupported ? "当前模式无对应分组，本局不接管" : `${(state.champSelectGroups.find((group) => group.groupId === runtime.groupId)?.name || runtime.groupId)}${runtime.position && runtime.position !== "default" ? ` · ${champSelectLaneName(runtime.position)}` : ""}` : "等待英雄选择";
    const capabilities = champSelectCapabilitiesHTML(definition, runtime) + `<label class="suite-switch cs-avoid-global" title="同时用于本组禁用、提前预选和选用。开启时避让队友预选；关闭时允许选择队友仅预选的英雄。队友已经选择的英雄始终跳过。"><span>避让队友预选</span><input type="checkbox" data-cs-avoid${checked(config.ban?.avoidTeammateIntent !== false || config.pick?.avoidTeammateIntent !== false)}${settings.enabled ? "" : " disabled"}></label>`;
    roots.champselect.className = settings.enabled ? "" : "cs-master-off";
    champSelectPanelRoot().innerHTML = `<section class="suite-card cs-master"><div class="cs-master-id"><span class="cs-master-ring" aria-hidden="true">⌖</span><div><h2>征召托管</h2><p>仅在英雄选择阶段生效；总开关同步控制各模式的禁用与选用</p></div><label class="suite-switch"><input type="checkbox" data-cs-master${checked(settings.enabled)}><span>总开关</span></label></div><div class="cs-master-right"><span class="cs-live-pill"><i></i>${escapeHTML(liveLabel)}</span><button class="button button-secondary" type="button" data-cs-pause${runtime.active && settings.enabled ? "" : " disabled"}>${runtime.sessionPaused ? "恢复本局" : "本局暂停"}</button></div></section>
      <div class="cs-body"><nav class="cs-mode-list" aria-label="征召模式分组"><span class="cs-mode-title">模式分组</span>${state.champSelectGroups.map((group) => { const value = settings.groups?.[group.groupId] || {}; const banCount = champSelectConfiguredCount(value, "ban"); const pickCount = champSelectConfiguredCount(value, "pick"); return `<button class="cs-mode-item${definition.groupId === group.groupId ? " is-active" : ""}" type="button" data-cs-group="${group.groupId}"><span aria-hidden="true">${({ranked:"◈",normal:"◇",aram:"❄",arena:"⚔",event:"✧",practice:"▤"})[group.groupId] || "◇"}</span><span><strong>${escapeHTML(group.name)}</strong><small><i class="${banCount ? "on" : ""}">${group.hasBan ? `禁 ${banCount}` : "无禁用"}</i><i class="${pickCount ? "on" : ""}">选 ${pickCount}</i></small></span></button>`; }).join("")}</nav><div class="cs-stack"><div class="cs-capabilities"><h3>${escapeHTML(definition.name)}</h3>${capabilities}</div>${champSelectPoolHelpHTML(definition, runtime)}<fieldset class="cs-config-fields" data-cs-config-group="${definition.groupId}"${settings.enabled ? "" : " disabled"} aria-label="征召配置">${renderChampSelectSideCard("ban", definition, config, runtime)}${renderChampSelectSideCard("pick", definition, config, runtime)}${renderChampSelectBenchCard(definition, config, runtime)}</fieldset>${renderChampSelectTimeline(runtime)}</div></div>`;
    bindChampSelectControls();
    syncChampSelectDialog();
  }

  async function loadChampSelect(force = false) {
    if (state.destroyed || !roots.champselect) return;
    if (!force && state.watch && state.champSelectGroups && state.champSelectCatalog && state.champSelectRuntime) {
      renderChampSelect();
      return;
    }
    try {
      const tasks = [];
	  if (!state.watch || force) tasks.push(loadWatch(force));
      if (!state.champSelectGroups || force) tasks.push(api("/api/champselect/groups").then((value) => { state.champSelectGroups = value; }));
      if (!state.champSelectCatalog || force) tasks.push(api("/api/champions/catalog").then((value) => { state.champSelectCatalog = value; }));
      tasks.push(api("/api/champselect/state").then((value) => { state.champSelectRuntime = value; }));
      await Promise.all(tasks);
      if (state.destroyed) return;
      if (!state.champSelectDialog && state.champSelectRuntime?.active && !state.champSelectRuntime.unsupported && state.champSelectGroups.some((group) => group.groupId === state.champSelectRuntime.groupId)) {
        state.champSelectGroup = state.champSelectRuntime.groupId;
        if (state.champSelectRuntime.position) state.champSelectLane = { ban: state.champSelectRuntime.position, pick: state.champSelectRuntime.position };
      }
      renderChampSelect();
    } catch (error) {
      if (state.destroyed) return;
      champSelectPanelRoot().innerHTML = errorCard("征召设置读取失败", error.message, "champselect");
    }
  }

  async function saveChampSelect(message = "征召设置已保存") {
    try {
      await persistWatchSettings();
      renderChampSelect();
      toast(message);
    } catch (error) {
      toast(error.message);
      await loadChampSelect(true);
    }
  }

  function bindChampSelectControls() {
    const root = champSelectPanelRoot();
    root.querySelector("[data-cs-master]")?.addEventListener("change", async (event) => {
      const settings = champSelectSettings();
      settings.enabled = event.target.checked;
      for (const definition of state.champSelectGroups) {
        const config = settings.groups[definition.groupId];
        if (!config) continue;
        config.ban.enabled = settings.enabled && Boolean(definition.hasBan);
        config.pick.enabled = settings.enabled;
      }
      renderChampSelect();
      await saveChampSelect();
    });
    for (const button of root.querySelectorAll("[data-cs-group]")) button.addEventListener("click", () => {
      state.champSelectGroup = button.dataset.csGroup;
      const definition = champSelectSelectedDefinition();
      for (const side of ["ban", "pick"]) if (!definition.positions.includes(state.champSelectLane[side])) state.champSelectLane[side] = definition.positions[0] || "default";
      renderChampSelect();
    });
    for (const input of root.querySelectorAll("[data-cs-side-enabled]")) input.addEventListener("change", async () => {
      const config = champSelectSelectedConfig();
      config[input.dataset.csSideEnabled].enabled = input.checked;
      await saveChampSelect();
    });
    for (const button of root.querySelectorAll("[data-cs-strategy]")) button.addEventListener("click", async () => {
      champSelectSelectedConfig()[button.dataset.csStrategy].strategy = button.dataset.csStrategyValue;
      await saveChampSelect();
    });
    for (const button of root.querySelectorAll("[data-cs-delay]")) button.addEventListener("click", async () => {
      if (button.matches(":disabled")) return;
      const config = champSelectSelectedConfig()[button.dataset.csDelay];
      const key = button.dataset.csDelayKey === "lockDelayMs" ? "lockDelayMs" : "delayMs";
      config[key] = Math.max(0, Math.min(10000, Number(config[key] ?? (key === "lockDelayMs" ? 10000 : 0)) + Number(button.dataset.csDelayDelta || 0)));
      await saveChampSelect();
    });
    for (const input of root.querySelectorAll("[data-cs-avoid]")) input.addEventListener("change", async () => {
      const config = champSelectSelectedConfig();
      config.ban.avoidTeammateIntent = input.checked;
      config.pick.avoidTeammateIntent = input.checked;
      await saveChampSelect();
    });
    for (const button of root.querySelectorAll("[data-cs-lane]")) button.addEventListener("click", () => {
      state.champSelectLane[button.dataset.csLaneSide] = button.dataset.csLane;
      renderChampSelect();
    });
    for (const button of root.querySelectorAll("[data-cs-open-dialog]")) button.addEventListener("click", () => openChampSelectDialog(button.dataset.csOpenDialog));
    for (const button of root.querySelectorAll("[data-cs-remove]")) button.addEventListener("click", async () => {
      const pool = champSelectPoolFor(button.dataset.csRemove);
      pool.splice(Number(button.dataset.csRemoveIndex), 1);
      await saveChampSelect("已移除英雄");
    });
    for (const button of root.querySelectorAll("[data-cs-hold-delta]")) button.addEventListener("click", async () => {
      const bench = champSelectSelectedConfig().bench;
      bench.holdMs = Math.max(1000, Math.min(10000, Number(bench.holdMs || 1000) + Number(button.dataset.csHoldDelta || 0)));
      await saveChampSelect();
    });
    bindChampSelectTimeInputs(root);
    bindChampSelectRailDrag(root);
    root.querySelector("[data-cs-bench-enabled]")?.addEventListener("change", async (event) => { champSelectSelectedConfig().bench.enabled = event.target.checked; await saveChampSelect(); });
    root.querySelector("[data-cs-bench-prefer]")?.addEventListener("change", async (event) => { champSelectSelectedConfig().bench.preferFirst = event.target.checked; await saveChampSelect(); });
    root.querySelector("[data-cs-bench-trade]")?.addEventListener("change", async (event) => { champSelectSelectedConfig().bench.handleTrade = event.target.checked; await saveChampSelect(); });
    root.querySelector("[data-cs-pause]")?.addEventListener("click", async () => {
      try {
        state.champSelectRuntime = await api("/api/champselect/pause", { method: "POST", body: JSON.stringify({ paused: !state.champSelectRuntime?.sessionPaused }) });
        renderChampSelect();
        toast(state.champSelectRuntime.sessionPaused ? "本局征召托管已暂停" : "本局征召托管已恢复");
      } catch (error) { toast(error.message); }
    });
  }

  function bindChampSelectRailDrag(root) {
    const rows = [...root.querySelectorAll("[data-cs-rail-index]")];
    const clear = () => {
      state.champSelectRailDrag = null;
      for (const row of rows) row.classList.remove("is-dragging", "is-drop-target");
    };
    const canDrop = (row) => {
      const drag = state.champSelectRailDrag;
      if (!drag || !champSelectSettings()?.enabled || !row.isConnected || row.closest("fieldset")?.disabled || Date.now() >= drag.expiresAt) return false;
      const definition = champSelectSelectedDefinition();
      const pool = champSelectPoolFor(drag.side);
      return row.dataset.csRailSide === drag.side && definition.groupId === drag.group && champSelectPoolLane(drag.side, definition) === drag.lane &&
        pool === drag.pool && pool.length === drag.snapshot.length && pool.every((id, index) => id === drag.snapshot[index]);
    };
    for (const row of rows) {
      row.draggable = Boolean(champSelectSettings()?.enabled && !row.closest("fieldset")?.disabled);
      const open = row.querySelector(".cs-slot-open");
      if (open) open.draggable = row.draggable;
      for (const image of row.querySelectorAll("img")) image.draggable = false;
      row.addEventListener("dragstart", (event) => {
        if (!row.draggable || !champSelectSettings()?.enabled || event.target.closest?.("[data-cs-remove]")) { event.preventDefault(); return; }
        const side = row.dataset.csRailSide;
        const definition = champSelectSelectedDefinition();
        const pool = champSelectPoolFor(side);
        state.champSelectRailDrag = { side, group: definition.groupId, lane: champSelectPoolLane(side, definition), pool, snapshot: [...pool], index: Number(row.dataset.csRailIndex), expiresAt: Date.now() + 30000 };
        row.classList.add("is-dragging");
        event.dataTransfer?.setData("text/plain", "champselect-sequence");
        if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
      });
      row.addEventListener("dragover", (event) => {
        if (!canDrop(row)) return;
        event.preventDefault();
        for (const target of rows) target.classList.toggle("is-drop-target", target === row && Number(row.dataset.csRailIndex) !== state.champSelectRailDrag.index);
        if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
      });
      row.addEventListener("dragleave", (event) => {
        if (!row.contains(event.relatedTarget)) row.classList.remove("is-drop-target");
      });
      row.addEventListener("drop", async (event) => {
        event.preventDefault();
        const valid = canDrop(row);
        const drag = state.champSelectRailDrag;
        clear();
        if (!valid) { renderChampSelect(); return; }
        const from = drag.index, to = Number(row.dataset.csRailIndex);
        if (!Number.isInteger(from) || !Number.isInteger(to) || from < 0 || to < 0 || from >= drag.pool.length || to >= drag.pool.length || from === to) { renderChampSelect(); return; }
        [drag.pool[from], drag.pool[to]] = [drag.pool[to], drag.pool[from]];
        await saveChampSelect("英雄位置已交换");
      });
      row.addEventListener("dragend", () => { clear(); renderChampSelect(); });
    }
  }

  function openChampSelectDialog(side) {
    state.champSelectPositionController?.abort();
    const definition = champSelectSelectedDefinition();
    const lane = champSelectPoolLane(side, definition);
    state.champSelectDialog = { side, lane, group: definition.groupId, draft: [...champSelectPoolFor(side)], query: "", dragIndex: -1 };
    state.champSelectPosition = "all";
    state.champSelectPositionIDs = null;
    renderChampSelect();
  }

  function bindChampSelectDialog() {
    const dialogElement = roots.champselect.querySelector(".cs-dialog");
    const dialog = state.champSelectDialog;
    if (!dialogElement || !dialog) return;
    dialogElement.addEventListener("cancel", (event) => { event.preventDefault(); closeChampSelectDialog(); });
    dialogElement.querySelector("[data-cs-dialog-search]")?.addEventListener("input", (event) => {
      dialog.query = event.target.value;
      updateChampSelectDialog();
    });
    dialogElement.addEventListener("click", async (event) => {
      if (event.target === dialogElement) { closeChampSelectDialog(); return; }
      const button = event.target.closest("button");
      if (!button || button.disabled) return;
      const data = button.dataset;
      if ("csDialogClose" in data) { closeChampSelectDialog(); return; }
      if ("csDialogPosition" in data) { await loadChampSelectPositionFilter(data.csDialogPosition); return; }
      if ("csDialogSave" in data) {
        const config = champSelectSettings().groups?.[dialog.group]?.[dialog.side];
        if (!config) { toast("当前模式配置已变化，请重新打开编辑序列"); return; }
        config.champions[dialog.lane] = [...dialog.draft];
        closeChampSelectDialog();
        await saveChampSelect("英雄序列已保存");
        return;
      }
      if ("csDialogAdd" in data) {
        const id = Number(data.csDialogAdd);
        const definition = state.champSelectGroups.find(group => group.groupId === dialog.group);
        const limit = dialog.side === "ban" ? definition.banLimit : definition.pickLimit;
        if (!dialog.draft.includes(id) && dialog.draft.length < limit) dialog.draft.push(id);
      } else if ("csDialogRemove" in data) {
        dialog.draft.splice(Number(data.csDialogRemove), 1);
      } else if ("csDialogClear" in data) {
        dialog.draft = [];
      } else return;
      updateChampSelectDialog();
    });
    dialogElement.addEventListener("dragstart", (event) => {
      const row = event.target.closest("[data-cs-drag-index]");
      if (!row) return;
      dialog.dragIndex = Number(row.dataset.csDragIndex);
      event.dataTransfer?.setData("text/plain", row.dataset.csDragIndex);
      if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
    });
    dialogElement.addEventListener("dragover", (event) => {
      if (!event.target.closest("[data-cs-drag-index]")) return;
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
    });
    dialogElement.addEventListener("drop", (event) => {
      const row = event.target.closest("[data-cs-drag-index]");
      if (!row) return;
      event.preventDefault();
      const from = Number(event.dataTransfer?.getData("text/plain") || dialog.dragIndex);
      const to = Number(row.dataset.csDragIndex);
      if (!Number.isInteger(from) || !Number.isInteger(to) || from === to || from < 0 || from >= dialog.draft.length) return;
      const [value] = dialog.draft.splice(from, 1);
      dialog.draft.splice(to, 0, value);
      dialog.dragIndex = -1;
      updateChampSelectDialog();
    });
  }

  async function loadChampSelectPositionFilter(position) {
    const dialog = state.champSelectDialog;
    if (state.destroyed || !dialog) return;
    // Keep LCU pool names in the UI; rankings has a different API vocabulary.
    const apiPosition = { all: "all", top: "top", jungle: "jungle", middle: "mid", bottom: "adc", utility: "support" }[position];
    if (!apiPosition) return;
    const requestId = state.champSelectPositionRequest = (state.champSelectPositionRequest || 0) + 1;
    state.champSelectPositionController?.abort();
    const current = () => !state.destroyed && state.champSelectDialog === dialog && state.champSelectPositionRequest === requestId;
    const started = Date.now();
    const report = (reason, fields = {}) => {
      if (typeof window !== "undefined") window.reportFlowDiagnostic?.("champ_select_filter_client", reason, { requestId, requestedPosition: position, resolvedPosition: apiPosition, durationMs: Date.now() - started, ...fields });
    };
    state.champSelectPosition = position;
    if (position === "all") {
      state.champSelectPositionIDs = null;
      report("all");
      updateChampSelectDialog();
      return;
    }
    if (state.champSelectPositionCache.has(position)) {
      state.champSelectPositionIDs = state.champSelectPositionCache.get(position);
      report("cached", { itemCount: state.champSelectPositionIDs.size });
      updateChampSelectDialog();
      return;
    }
    const controller = new AbortController();
    state.champSelectPositionController = controller;
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, 15000);
    report("request");
    try {
      const response = await api(`/api/champions/rankings?mode=ranked&tier=emerald_plus&position=${apiPosition}`, { signal: controller.signal });
      if (!current()) { report("stale"); return; }
      if (!Array.isArray(response?.rows)) {
        const error = new Error("位置筛选响应格式无效");
        error.errorKind = "invalid-response";
        throw error;
      }
      const ids = new Set(response.rows.map((row) => Number(row.championId)).filter((id) => Number.isInteger(id) && id > 0));
      state.champSelectPositionCache.set(position, ids);
      state.champSelectPositionIDs = ids;
      report("received", { itemCount: ids.size });
    } catch (error) {
      if (!current()) { report("stale", { errorKind: "canceled" }); return; }
      report("failed", { errorKind: timedOut ? "timeout" : error.errorKind || "other", httpStatus: error.status || 0 });
      state.champSelectPosition = "all";
      state.champSelectPositionIDs = null;
      toast(`位置筛选读取失败：${timedOut ? "本地请求超时，请重试" : error.message}`);
    } finally {
      clearTimeout(timer);
      if (state.champSelectPositionController === controller) state.champSelectPositionController = null;
    }
    updateChampSelectDialog();
  }

  function handleWatchEvent(value) {
    const raw = String(value || "");
    const parts = raw.split(":");
    if (parts[0] !== "watch") return;
	    if (parts[1] === "session") {
	      if (String(parts[2] || "").startsWith("champselect-")) {
		state.champSelectRuntime = null;
		if (state.active && state.connected) void loadChampSelect(false);
		return;
	  }
      if (state.watch) state.watch.customPaused = parts[2] === "paused_custom";
      if (typeof watchDiagnostic === "function") watchDiagnostic("custom-event");
      if (parts[2] === "paused_custom") { state.watchEvents.clear(); state.watchPriority = false; }
      renderWatch(); return;
    }
    if (parts[1] === "priority") {
      state.watchPriority = parts[2] === "honoring";
      renderWatch();
      return;
    }
    const kind = parts[1];
    const action = parts[2];
    if (kind === "skipped" && ["auto-matchmaking", "position-broadcast"].includes(action)) {
      const reason = parts[3];
      const message = action === "position-broadcast" ? "客户端尚未提供阵营或选人聊天会话，本次播报已跳过" : { "not-leader": "需要你是房主才能自动开始匹配", custom: "自定义房间不自动开始匹配", "not-ready": "房间尚未就绪，需满足人数和可开始匹配条件" }[reason];
      if (!message) return;
      // Preconditions belong on the rule card; repeated lobby events must not
      // generate failure toasts or claim that a higher-priority action canceled it.
      state.watchEvents.set(action, { kind: "skipped", reason, start: Date.now(), message });
      renderWatch();
      return;
    }
    if (!action || !["armed", "fired", "failed", "canceled"].includes(kind)) return;
    if (kind === "failed" && action.startsWith("facade-")) {
      toast(action === "facade-rank" ? "登录时重设展示段位失败" : "登录时重设个性签名失败");
      return;
    }
    const statusCode = Number(parts[3] || 0);
    const errorCode = parts[4] || "";
    const message = parts.slice(5).join(":");
    const delay = Number(parts[3] || 0);
    state.watchEvents.set(action, { kind, start: Date.now(), end: Date.now() + delay, delay, statusCode, errorCode, message });
    if (kind === "fired") {
      state.watchFired += 1;
      if (action === "auto-honor") state.watchPriority = false;
    }
	if (action.startsWith("champselect-")) {
	  state.champSelectRuntime = null;
	  if (state.active && state.connected) void loadChampSelect(false);
	  renderWatch();
	  return;
	}
    if (kind === "failed" && action === "auto-honor") state.watchPriority = false;
    if (kind === "failed") {
      const title = watchActionTitle(action);
      const detail = message || errorCode || (statusCode ? `客户端返回 HTTP ${statusCode}` : "本机客户端请求失败");
      toast(`${title}失败：${detail}`);
    }
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
    const streamKnown = state.eventStream !== null;
    const eventHealthy = value.connected && state.eventStream === true;
    metrics.rig.textContent = !value.connected ? "未连接" : !value.settingsKnown ? "无法定位" : value.settingsLocked ? "已锁定" : "未锁定";
    roots.rig.className = "";
    roots.rig.innerHTML = `<div class="rig-layout"><div class="rig-main">
      <section class="suite-card"><div class="suite-card-head"><div><h3>安装与连接</h3><p>只读核对当前连接安装目录，不猜测缺失路径。</p></div><span class="suite-chip ${eventHealthy ? "is-success" : "is-warning"}">${!value.connected ? "未连接" : eventHealthy ? "连接正常" : "事件流异常"}</span></div><dl class="rig-kv"><dt>发行渠道</dt><dd>${escapeHTML(value.region || "无法定位")}${value.platform ? ` / ${escapeHTML(value.platform)}` : ""}</dd><dt>安装根目录</dt><dd>${escapeHTML(value.installRoot || "无法定位")}</dd><dt>配置目录</dt><dd>${escapeHTML(value.configRoot || "无法定位")}</dd><dt>设置文件</dt><dd>${escapeHTML(value.settingsFile || "无法定位")}${value.settingsKnown ? " · 已定位" : ""}</dd><dt>HTTP 连接</dt><dd>${value.connected ? "127.0.0.1 已连接（端口与令牌不展示、不落盘）" : escapeHTML(value.reason || "未连接")}</dd><dt>事件流</dt><dd>${!value.connected ? "未连接" : !streamKnown ? "状态读取中" : eventHealthy ? "已连接，自动规则可以接收客户端事件" : "已断开，自动规则暂时不会触发"}</dd><dt>客户端界面</dt><dd>${escapeHTML(value.uxState || "状态未知")}</dd></dl>${value.connected && streamKnown && !eventHealthy ? '<div class="suite-note is-danger rig-event-warning"><span aria-hidden="true">!</span><span><strong>事件流已断开</strong><br>HTTP 读取仍可用，但自动规则暂时不会触发；助手会在后台退避重连。</span></div>' : ""}<div class="rig-lock"><span class="rig-lock-icon" aria-hidden="true">${rigLockIcon(value.settingsLocked)}</span><div><h2>${value.settingsLocked ? "游戏设置已锁定" : "游戏设置未锁定"}</h2><p>${value.settingsKnown ? (value.settingsLocked ? "设置文件为只读，游戏内改动不会写回文件。" : "设置文件可写，游戏内改动可以保存。") : escapeHTML(value.reason || "无法定位设置文件，不会猜路径或尝试修改。")}</p></div><div class="rig-lock-actions"><span class="suite-chip ${value.settingsLocked ? "is-success" : ""}">${value.settingsLocked ? "只读" : "可写入"}</span><button class="button ${value.settingsLocked ? "button-secondary" : "button-primary"}" type="button" data-rig-lock ${value.settingsKnown ? "" : "disabled"}>${value.settingsLocked ? "解除锁定" : "设为只读"}</button></div></div><div class="suite-note is-info rig-lock-note"><span aria-hidden="true">i</span><span>只影响当前连接的安装目录。写入前会再次校验路径位于安装目录内，且目标不是符号链接。</span></div></section>
    </div><aside class="suite-card"><div class="suite-card-head"><div><h3>客户端维护</h3><p>普通维护动作会直接执行；关闭客户端前会再次确认。</p></div></div><div class="rig-actions">${[
      ["restart-ux", "重启客户端界面", "结束并重新启动 LeagueClientUx。"],
      ["kill-ux", "结束界面进程", "只结束客户端界面，不结束游戏端。"],
      ["launch-ux", "启动界面进程", "请求客户端重新拉起界面。"],
      ["quit-client", "关闭客户端", "请求英雄联盟客户端正常退出。"],
      ["disconnect", "断开连接", "让助手停止当前连接，顶部刷新后可重连。"],
    ].map(([action, title, copy]) => `<div class="rig-action"><div><strong>${title}</strong><p>${copy}</p></div><button class="button ${action === "quit-client" ? "button-danger" : "button-secondary"}" type="button" data-rig-action="${action}">${action === "disconnect" ? "断开" : "执行"}</button></div>`).join("")}</div></aside></div>`;
    bindRigControls();
  }

  function rigLockIcon(locked) {
    return locked
      ? '<svg viewBox="0 0 24 24"><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>'
      : '<svg viewBox="0 0 24 24"><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.5-2"/></svg>';
  }

  function bindRigControls() {
    roots.rig.querySelector("[data-rig-lock]")?.addEventListener("click", async () => {
      const locked = !state.rig.settingsLocked;
      await runRigRequest("/api/rig/settings-lock", { locked });
    });
    for (const button of roots.rig.querySelectorAll("[data-rig-action]")) button.addEventListener("click", async () => {
      const label = button.closest(".rig-action")?.querySelector("strong")?.textContent || "维护动作";
      const consequence = button.closest(".rig-action")?.querySelector("p")?.textContent || "此动作会立即影响当前客户端。";
      if (button.dataset.rigAction === "quit-client" && !(await requestConfirmation(button, label, consequence))) return;
      await runRigRequest("/api/rig/maintenance", { action: button.dataset.rigAction }, button.dataset.rigAction);
    });
  }

  async function runRigRequest(path, body, action = "") {
    try {
      await api(path, { method: "POST", body: JSON.stringify(body) });
      toast("客户端维护操作已执行");
      if (action === "disconnect") {
        setConnected(false);
        document.getElementById("refresh")?.click();
      } else {
        await loadRig(true);
      }
    } catch (error) { toast(error.message); }
  }

  async function loadRig(force = false) {
    const staleDisconnectedCache = state.rig?.connected === false && state.connected === true;
    if (state.rig && !force && !staleDisconnectedCache) { renderRig(); return; }
    try { state.rig = await api("/api/rig/status"); renderRig(); }
    catch (error) { roots.rig.innerHTML = errorCard("维护状态读取失败", error.message, "rig"); }
  }

  function facadeIconImage(icon) {
    return imageURL(`/lol-game-data/assets/v1/profile-icons/${Number(icon.id)}.jpg`);
  }



  async function runFacadeProbe(button) {
    if (state.facadeApplying) return;
    state.facadeApplying = true; button.disabled = true;
    state.facadeRequestToken = Number(state.facadeRequestToken || 0) + 1;
    state.facadeController?.abort();
    state.facadeController = null;
    try {
      const result = await api("/api/facade/probe", { method: "POST", body: "{}" });
      toast(result.mode === "read-only" ? "只读检测完成，未改动生涯设置；接口与拥有状态已写入诊断日志" : "检测完成，请导出诊断日志用于核对支持情况");
    } catch (error) { toast(error.message); }
    finally { state.facadeApplying = false; button.disabled = false; }
  }

  function hydrateFacadeDraft(force = false) {
    if (state.facadeDraft && !force) return;
    const value = state.facade || {};
	const skins = Array.isArray(value.skins) ? value.skins : [];

    const chat = value.chat || {};
    const lol = chat.lol || {};
	const actualID = Number(value.profile?.backgroundSkinId || 0);
	const firstSkin = skins.find((skin) => Number(skin.id) === actualID) || {};
	const explicitChampionID = Number(value.profile?.backgroundChampionId || 0);
	const inferredChampionID = actualID > 0 ? Math.floor(actualID / 1000) : 0;
    state.facadeDraft = {
      hero: String(Number(firstSkin.championId || 0) || explicitChampionID || inferredChampionID || ""), skinId: Number(firstSkin.id || value.profile?.backgroundSkinId || 0), ownedOnly: false,
      iconId: Number(value.summoner?.profileIconId || 0), bannerId: String(value.bannerId || ""), rankBanner: value.rankBanner || "", bannerAccent: value.bannerAccent || "",
      availability: chat.availability || "chat", statusMessage: chat.statusMessage || "",
      queue: lol.rankedLeagueQueue || "RANKED_SOLO_5X5", tier: lol.rankedLeagueTier || "UNRANKED", division: lol.rankedLeagueDivision || "I",
      resetStatus: Boolean(value.loginReset?.statusMessageEnabled), resetRank: Boolean(value.loginReset?.rankEnabled),
    };
  }

  function facadeTitleText(value) {
	const summaryTitle = value.challengeSummary?.title;
	if (typeof summaryTitle === "string" && summaryTitle.trim()) return summaryTitle.trim();
	if (summaryTitle && typeof summaryTitle === "object") {
	  const name = summaryTitle.name;
	  if (typeof name === "string" && name.trim()) return name.trim();
	}
	if (typeof summaryTitle === "number" && Number.isFinite(summaryTitle)) return String(summaryTitle);
	return "未设置头衔";
  }

  function facadeTitleFilled(value) {
	const summaryTitle = value.challengeSummary?.title;
	if (typeof summaryTitle === "string") return Boolean(summaryTitle.trim());
	if (summaryTitle && typeof summaryTitle === "object") return typeof summaryTitle.name === "string" && Boolean(summaryTitle.name.trim());
	return typeof summaryTitle === "number" && Number.isFinite(summaryTitle);
  }

  function facadeChallengeSlots(value) {
	// 后端已按“选中 ID + 挑战目录、topChallenges 兜底”解析为最多三个安全名称。
	const challenges = Array.isArray((value || state.facade || {}).challenges) ? (value || state.facade || {}).challenges.slice(0, 3) : [];
	return Array.from({ length: 3 }, (_, index) => {
	  const name = typeof challenges[index]?.name === "string" ? challenges[index].name.trim() : "";
	  return `<span class="facade-slot${name ? " is-filled" : ""}" data-facade-challenge-slot>${escapeHTML(name || "未设置")}</span>`;
	}).join("");
  }

  function facadeVisibleSkins(skins, draft) {
    const familyID = skin => Number(skin.parentSkinId || skin.id);
    const heroSkins = skins.filter(skin => String(skin.championId) === draft.hero);
    const dated = heroSkins.filter(skin => skin.releaseDate || skin.releaseSortDate)
      .map(skin => ({ id: familyID(skin), date: skin.releaseDate || skin.releaseSortDate }))
      .sort((a, b) => b.id - a.id);
    // Missing dates are not proof a skin is old. Anchor unknown families to the
    // nearest lower catalog ID; keep actual release fields untouched. New tail
    // entries use the latest known date, and base skins always remain last.
    const latestDate = dated.reduce((date, skin) => skin.date > date ? skin.date : date, "");
    const sortDate = skin => {
      if (skin.releaseDate || skin.releaseSortDate) return skin.releaseDate || skin.releaseSortDate;
      if (Number(skin.id) === Number(skin.championId) * 1000) return "";
      const id = familyID(skin);
      if (id > (dated[0]?.id || 0)) return latestDate;
      return dated.find(other => other.id <= id)?.date || "";
    };
    return heroSkins.filter(skin => !draft.ownedOnly || skin.owned).sort((a, b) =>
      sortDate(b).localeCompare(sortDate(a)) || familyID(b) - familyID(a)
      || Number(!!a.isVariant) - Number(!!b.isVariant) || Number(b.id) - Number(a.id));
  }

  function facadeFilmHTML(visibleSkins, draft) {
    const ownershipUnknown = state.facade?.skinOwnershipUnavailable === true;
    if (state.facade?.skinsUnavailable && !visibleSkins.length) return '<div class="facade-catalog-empty">皮肤目录暂时读取失败，不代表没有皮肤。<button type="button" class="text-button" data-facade-catalog-retry>重试读取</button></div>';

	return visibleSkins.map((skin) => `<button type="button" class="${Number(skin.id) === Number(draft.skinId) ? "is-selected" : ""}" data-facade-skin="${skin.id}" aria-label="${escapeHTML(skin.name)}${ownershipUnknown ? "，拥有状态未读取" : skin.owned ? "" : "，未拥有"}" aria-pressed="${Number(skin.id) === Number(draft.skinId)}" title="${escapeHTML(skin.name)}${ownershipUnknown ? " · 拥有状态未读取" : skin.owned ? "" : " · 未拥有"}"><img data-queued-src="${imageURL(skin.tilePath || skin.splashPath)}" alt="" loading="lazy" decoding="async"><span class="sr-only">${escapeHTML(skin.name)}</span></button>`).join("") || '<span class="muted facade-catalog-empty">没有符合筛选条件的皮肤</span>';
  }

  function renderFacade() {
    if (!state.facade) {
      roots.facade.innerHTML = `<div class="facade-layout" aria-busy="true"><section class="suite-card facade-preview"><div class="facade-preview-art"></div><p>正在同步当前生涯…</p></section><section class="suite-card"><h3>生涯背景与展示</h3><p>正在读取当前账号，资料就绪后即可编辑。</p></section></div>`;
      return;
    }
    const value = state.facade || {};
    hydrateFacadeDraft();
    const draft = state.facadeDraft;
    if (value.skinOwnershipUnavailable) draft.ownedOnly = false;
    const summoner = value.summoner || {};
    const skins = Array.isArray(value.skins) ? value.skins : [];
    const champions = [...new Map(skins.map((skin) => [String(skin.championId), skin.championName || `英雄 ${skin.championId}`])).entries()].sort((left, right) => left[1].localeCompare(right[1], "zh-CN"));
    if (draft.hero && !champions.some(([id]) => id === draft.hero)) champions.unshift([draft.hero, `英雄 ${draft.hero}`]);
    champions.unshift(["", "请选择英雄"]);
    const visibleSkins = facadeVisibleSkins(skins, draft);

    const selectedSkin = skins.find((skin) => Number(skin.id) === Number(draft.skinId)) || {};
    const currentSkin = skins.find(skin => Number(skin.id) === Number(value.profile?.backgroundSkinId)) || {};
    const actualBackgroundPath = value.profile?.backgroundPath || currentSkin.splashPath;
    const backgroundURL = actualBackgroundPath ? imageURL(actualBackgroundPath) : (Number(value.profile?.backgroundSkinId) > 0 ? `/api/skin-art?id=${Number(value.profile.backgroundSkinId)}` : "");
    const displayName = summoner.gameName || summoner.displayName || "当前召唤师";
    const tagLine = summoner.tagLine ? `#${summoner.tagLine}` : "";
	const availabilityLabels = { chat: "在线", away: "离开", dnd: "游戏中", spectating: "观战中", offline: "离线" };
    const highTier = ["MASTER", "GRANDMASTER", "CHALLENGER", "UNRANKED"].includes(draft.tier);
    const backgroundApplied = !value.profileUnavailable && Number(draft.skinId) > 0 && Number(value.profile?.backgroundSkinId) === Number(draft.skinId);
    const previewName = value.profile?.backgroundSkinName || currentSkin.name || (value.profileUnavailable ? "读取失败" : "客户端默认背景");
	const title = facadeTitleText(value);
	const titleFilled = facadeTitleFilled(value);
    const identityDirty = facadeIdentityDirty();
    const resetDirty = facadeResetDirty();
    roots.facade.className = "";
	// `.facade-signature` 保留既有样式类名，但这里承载的是生涯头衔而不是个性签名。
	const markup = `<div class="facade-layout"><div class="facade-left"><section class="suite-card facade-preview"><div class="facade-preview-art">${backgroundURL ? `<img data-queued-src="${backgroundURL}" alt="" data-suite-facade-art>` : ""}<span class="facade-art-label">当前背景 · ${escapeHTML(previewName)}</span></div><div class="facade-preview-body"><span class="facade-avatar">${summoner.profileIconId ? `<img data-queued-src="${imageURL(`/lol-game-data/assets/v1/profile-icons/${summoner.profileIconId}.jpg`)}" alt="">` : escapeHTML(displayName.slice(0, 1))}<small class="facade-avatar-level">${Number(summoner.summonerLevel || 0)}</small></span><div class="facade-identity"><h2>${escapeHTML(displayName)}</h2><p>${escapeHTML(tagLine || "当前账号")}</p></div><div class="facade-tags"><span class="suite-chip is-gold" data-facade-preview-rank>${escapeHTML(rankLabel(draft))}</span><span class="suite-chip" data-facade-preview-availability>${escapeHTML(availabilityLabels[draft.availability] || draft.availability)}</span><span class="suite-chip">上赛季旗帜</span></div><div class="facade-signature${titleFilled ? " is-filled" : ""}">${escapeHTML(title)}</div><div class="facade-slots">${facadeChallengeSlots(value)}</div></div></section><section class="suite-card facade-icon-card" ${value.connected ? "" : "hidden"}><div class="suite-card-head"><h3>头像</h3></div><div class="icon-current"><img class="icon-thumb" data-queued-src="${facadeIconImage({id:summoner.profileIconId})}" alt="当前头像"><div class="meta"><strong>生涯头像</strong><span>${Number(value.chat?.icon) > 0 && Number(value.chat.icon) !== Number(summoner.profileIconId) ? `聊天与好友栏另用头像 #${Number(value.chat.icon)}` : "当前客户端生涯头像"}</span></div><button class="text-button" type="button" data-facade-browse="icons">在收藏页浏览头像与旗帜 →</button></div></section>
      <section class="suite-card facade-banner-card" ${value.connected ? "" : "hidden"}><div class="suite-card-head"><h3>旗帜</h3></div><div class="facade-stack-row"><h4>生涯旗帜</h4><p>生涯旗帜只读浏览，目录与拥有状态在收藏页查看</p><div class="ctl"><button class="text-button" type="button" data-facade-browse="banners">在收藏页浏览头像与旗帜 →</button></div></div><div class="facade-stack-row"><h4>段位旗</h4><p>保留当前头像框偏好</p><div class="ctl"><div class="suite-segment">${[["lastSeasonHighestRank", "上赛季段位"], ["blank", "空白"]].map(([key, label]) => `<button type="button" data-facade-rank-banner="${key}" aria-pressed="${value.rankBanner === key}" class="${value.rankBanner === key ? "is-active" : ""}">${label}</button>`).join("")}</div></div></div></section><section class="suite-card facade-write-card"><div class="suite-card-head"><div><h3>这一页会改什么</h3></div></div><dl class="facade-write-list"><dt>生涯背景</dt><dd>你生涯页顶部的那张大图</dd><dt>好友悬浮卡</dt><dd>别人点你头像时看到的在线状态、签名和段位</dd><dt>生涯页展示</dt><dd>头像框、挑战勋章、赛季旗帜、表情轮盘</dd></dl><div class="suite-note facade-write-note"><span aria-hidden="true">⚑</span><span>以上全部<strong>只在你点击后执行</strong>，没有任何自动写入。只有“登录时重设”两项例外，它们默认关闭，开启后也只重放你保存过的值。</span></div></section></div>
      <div class="facade-controls"><section class="suite-card facade-background-card"><div class="suite-card-head"><div><h3>生涯背景</h3></div><span class="suite-chip" data-facade-background-applied ${backgroundApplied ? "" : "hidden"}>已应用</span><button class="button button-primary" type="button" data-facade-apply-background ${backgroundApplied ? "hidden" : ""} ${selectedSkin.id ? "" : "disabled"}>应用背景</button></div><div class="facade-selections"><label class="select-wrap"><span class="sr-only">英雄</span><select class="suite-select" data-facade-hero>${champions.map(([id, name]) => `<option value="${escapeHTML(id)}"${selected(draft.hero, id)}>${escapeHTML(name)}</option>`).join("")}</select></label><label class="suite-switch"><input type="checkbox" data-facade-owned${checked(draft.ownedOnly)}${value.skinOwnershipUnavailable ? ' disabled title="拥有状态尚未读取"' : ""}><span>只显示已拥有</span></label></div><div class="facade-film" aria-label="皮肤网格">${facadeFilmHTML(visibleSkins, draft)}</div><p class="facade-background-note">客户端接口<strong>不校验皮肤是否拥有</strong>——关掉上面的开关就能设置未拥有的皮肤，但它可能在下次登录时被服务端还原。</p></section>

	  <section class="suite-card facade-chat-card"><div class="suite-card-head"><div><h3>聊天身份</h3></div></div><div class="facade-row"><div><h4>在线状态</h4><p>部分状态只在特定情况下可用；客户端只会在实际进入对局或观战时保留对应状态</p></div><div class="suite-segment">${Object.entries(availabilityLabels).map(([key, label]) => `<button type="button" class="${draft.availability === key ? "is-active" : ""}" data-facade-availability="${key}">${label}</button>`).join("")}</div></div><div class="facade-row"><div><h4>个性签名</h4><p>留空即删除签名。开启“登录时重设”后每次客户端登录都会重新应用</p></div><div class="facade-row-control"><input class="suite-input facade-status-input" type="text" maxlength="200" value="${escapeHTML(draft.statusMessage)}" placeholder="输入签名…" data-facade-status><label class="suite-switch facade-reset-switch" data-tooltip="登录时重设个性签名"><input type="checkbox" aria-label="登录时重设个性签名" data-facade-reset-status${checked(draft.resetStatus)}><span class="sr-only">登录时重设</span></label></div></div><div class="facade-row"><div><h4>展示段位</h4><p>只改好友悬浮卡上的段位显示，不影响你的真实段位、战绩与匹配。大师及以上不需要选分段</p></div><div class="facade-row-control"><div class="facade-rank-fields"><span class="select-wrap"><select class="suite-select" aria-label="展示段位队列" data-facade-rank="queue"><option value="RANKED_SOLO_5X5"${selected(draft.queue, "RANKED_SOLO_5X5")}>单双排</option><option value="RANKED_FLEX_SR"${selected(draft.queue, "RANKED_FLEX_SR")}>灵活组排</option></select></span><span class="select-wrap"><select class="suite-select" aria-label="展示段位" data-facade-rank="tier">${rankOptions(draft.tier)}</select></span><span class="select-wrap"><select class="suite-select" aria-label="展示分段" data-facade-rank="division" ${highTier ? "disabled" : ""}>${["I","II","III","IV"].map((division) => `<option value="${division}"${selected(draft.division, division)}>${division}</option>`).join("")}</select></span></div><label class="suite-switch facade-reset-switch" data-tooltip="登录时重设展示段位"><input type="checkbox" aria-label="登录时重设展示段位" data-facade-reset-rank${checked(draft.resetRank)}><span class="sr-only">登录时重设</span></label></div></div><div class="facade-commit" data-facade-commit ${identityDirty || resetDirty ? "" : "hidden"}><span>改动只在左侧预览，确认后才写入客户端。</span><div><button class="button button-secondary" type="button" data-facade-save-reset ${resetDirty ? "" : "hidden"}>保存登录重设</button><button class="button button-primary" type="button" data-facade-apply-chat ${identityDirty ? "" : "hidden"}>确认并应用</button></div></div></section>
      <section class="suite-card"><div class="suite-card-head"><div><h3>展示清理</h3><p>任务数量提示点击即清空，其余操作执行前会二次确认。</p></div><button class="button button-secondary" type="button" data-facade-probe title="只读检查接口与拥有状态，不切换头像、旗帜或背景；结果写入诊断日志" ${value.connected ? "" : "disabled"}>检测客户端支持</button></div><div class="facade-actions">${[
		["clear-border", "卸下头像框", "换成固定的典藏边框；需要召唤师等级 ≥ 526", Number(summoner.summonerLevel || 0) < 526, "卸下"],
		["clear-title", "卸下头衔", "仅清空当前展示头衔，保留已选勋章和赛季旗帜", false, "卸下"],
		["clear-challenges", "卸下全部勋章", "清空挑战勋章并保留旗帜和当前头衔；如果客户端无法保留头衔，本次操作会中止并提示", false, "卸下"],
        ["clear-objectives", "清空右下角任务数量提示", "把当前未读任务与活动系列标记为已读，不领取奖励、不更改任务进度。", false, "清空"],
        ["clear-emotes", "清空表情轮盘", "一次性清掉所有表情位，需要重新自行配置", false, "清空"],
      ].map(([action, title, copy, disabled, label]) => `<div class="facade-action"><div><strong>${title}</strong><p>${copy}</p></div>${disabled ? `<span class="suite-chip">${action === "clear-objectives" ? "暂不支持" : "等级不足"}</span>` : `<button class="button ${action === "clear-emotes" ? "button-danger" : "button-secondary"}" type="button" data-facade-clear="${action}">${label}</button>`}</div>`).join("")}</div></section></div></div>`;
    if (roots.facade._facadeMarkup === markup && roots.facade.querySelector(".facade-controls")) return;
    const images = new Map();
    for (const image of roots.facade.querySelectorAll("img[data-queued-src]")) {
      const key = image.dataset.queuedSrc;
      if (!images.has(key)) images.set(key, []);
      images.get(key).push(image);
    }
    roots.facade.innerHTML = markup;
    for (const fresh of roots.facade.querySelectorAll("img[data-queued-src]")) {
      const image = images.get(fresh.dataset.queuedSrc)?.shift();
      if (image) { image.className = fresh.className; fresh.replaceWith(image); }
    }
    roots.facade._facadeMarkup = markup;
    bindFacadeControls();
  }

  function facadeIdentityDirty() {
    const chat = state.facade?.chat || {};
    const lol = chat.lol || {};
    const draft = state.facadeDraft || {};
    return String(chat.availability || "chat") !== String(draft.availability || "chat")
      || String(chat.statusMessage || "") !== String(draft.statusMessage || "")
      || String(lol.rankedLeagueQueue || "RANKED_SOLO_5X5") !== String(draft.queue || "")
      || String(lol.rankedLeagueTier || "UNRANKED") !== String(draft.tier || "")
      || (!["MASTER", "GRANDMASTER", "CHALLENGER", "UNRANKED"].includes(draft.tier) && String(lol.rankedLeagueDivision || "I") !== String(draft.division || ""));
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

  function facadeBackgroundDirty() {
    const value = state.facade || {};
    const draft = state.facadeDraft || {};
    const skins = Array.isArray(value.skins) ? value.skins : [];
    const actualID = Number(value.profile?.backgroundSkinId || 0);
    const firstSkin = skins.find((skin) => Number(skin.id) === actualID) || {};
    const actualHero = String(Number(firstSkin.championId || 0) || Number(value.profile?.backgroundChampionId || 0) || (actualID > 0 ? Math.floor(actualID / 1000) : 0) || "");
    return actualID !== Number(draft.skinId || 0) || actualHero !== String(draft.hero || "") || Boolean(draft.ownedOnly);
  }

  function facadeDraftDirty() {
    return facadeBackgroundDirty() || facadeIdentityDirty() || facadeResetDirty();
  }

  function syncFacadeCommitActions() {
    roots.facade._facadeMarkup = "";
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

  function rankLabel(draft = {}) {
    const tierKey = draft.tier || "UNRANKED";
    const tier = { UNRANKED: "未定级", IRON: "黑铁", BRONZE: "黄铜", SILVER: "白银", GOLD: "黄金", PLATINUM: "铂金", EMERALD: "翡翠", DIAMOND: "钻石", MASTER: "大师", GRANDMASTER: "宗师", CHALLENGER: "王者" }[tierKey] || tierKey;
    const queue = draft.queue === "RANKED_FLEX_SR" ? "灵活组排" : "单双排";
    return `${tier}${["MASTER","GRANDMASTER","CHALLENGER","UNRANKED"].includes(tierKey) || !draft.division ? "" : ` ${draft.division}`} · ${queue}`;
  }

  function facadeAvailabilityLabel(value) {
    return { chat: "在线", away: "离开", dnd: "游戏中", spectating: "观战中", offline: "离线" }[value] || value;
  }

  function updateFacadeBackgroundPreview() {
    roots.facade._facadeMarkup = "";
    const draft = state.facadeDraft || {};
    const skins = Array.isArray(state.facade?.skins) ? state.facade.skins : [];
    const selectedSkin = skins.find((skin) => Number(skin.id) === Number(draft.skinId));
    for (const button of roots.facade.querySelectorAll("[data-facade-skin]")) {
      const selected = Number(button.dataset.facadeSkin) === Number(draft.skinId);
      button.classList.toggle("is-selected", selected);
      button.setAttribute("aria-pressed", String(selected));
    }
    // The left card shows the committed client background. Choosing a hero
    // or skin only edits the selection until Apply succeeds and is read back.
    const sameID = Number(state.facade?.profile?.backgroundSkinId || 0) === Number(draft.skinId || 0);
    const applied = !state.facade?.profileUnavailable && Number(draft.skinId) > 0 && sameID;
    const appliedChip = roots.facade.querySelector("[data-facade-background-applied]");
    const applyButton = roots.facade.querySelector("[data-facade-apply-background]");
    if (appliedChip) appliedChip.hidden = !applied;
    if (applyButton) {
      applyButton.hidden = applied;
      applyButton.disabled = !selectedSkin;
    }
  }

  function bindFacadeSkinControls() {
    for (const button of roots.facade.querySelectorAll("[data-facade-skin]")) button.addEventListener("click", () => {
      state.facadeDraft.skinId = Number(button.dataset.facadeSkin);
      updateFacadeBackgroundPreview();
    });
  }

  function rebuildFacadeFilm() {
    const film = roots.facade.querySelector(".facade-film");
    if (!film) return;
    const skins = Array.isArray(state.facade?.skins) ? state.facade.skins : [];
    const visibleSkins = facadeVisibleSkins(skins, state.facadeDraft);
    film.innerHTML = facadeFilmHTML(visibleSkins, state.facadeDraft);
    bindFacadeSkinControls();
    updateFacadeBackgroundPreview();
  }

  function updateFacadeAvailability() {
    for (const button of roots.facade.querySelectorAll("[data-facade-availability]")) button.classList.toggle("is-active", button.dataset.facadeAvailability === state.facadeDraft.availability);
    const preview = roots.facade.querySelector("[data-facade-preview-availability]");
    if (preview) preview.textContent = facadeAvailabilityLabel(state.facadeDraft.availability);
    syncFacadeCommitActions();
  }

  function updateFacadeRank() {
    const division = roots.facade.querySelector('[data-facade-rank="division"]');
    if (division) {
      division.disabled = ["MASTER", "GRANDMASTER", "CHALLENGER", "UNRANKED"].includes(state.facadeDraft.tier);
      window.deepLegendsSelects?.sync(division);
    }
    const preview = roots.facade.querySelector("[data-facade-preview-rank]");
    if (preview) preview.textContent = rankLabel(state.facadeDraft);
    syncFacadeCommitActions();
  }

  function bindFacadeControls() {
    for (const button of roots.facade.querySelectorAll("[data-facade-browse]")) button.addEventListener("click", () => window.deepLegendsOpenFacadeCollection?.(button.dataset.facadeBrowse));
    roots.facade.querySelector("[data-facade-probe]")?.addEventListener("click", event => runFacadeProbe(event.currentTarget));
    for (const button of roots.facade.querySelectorAll("[data-facade-rank-banner]")) button.addEventListener("click", () => applyFacade({ action: "rank-banner", rankBanner: button.dataset.facadeRankBanner }));
    roots.facade.querySelector("[data-facade-catalog-retry]")?.addEventListener("click", () => loadFacade(true, true, "manual"));
    roots.facade.querySelector("[data-facade-hero]")?.addEventListener("change", (event) => { state.facadeDraft.hero = event.target.value; state.facadeDraft.skinId = 0; rebuildFacadeFilm(); });
    roots.facade.querySelector("[data-facade-owned]")?.addEventListener("change", (event) => { state.facadeDraft.ownedOnly = event.target.checked; rebuildFacadeFilm(); });
    bindFacadeSkinControls();
    for (const button of roots.facade.querySelectorAll("[data-facade-availability]")) button.addEventListener("click", () => { state.facadeDraft.availability = button.dataset.facadeAvailability; updateFacadeAvailability(); });
	roots.facade.querySelector("[data-facade-status]")?.addEventListener("input", (event) => { state.facadeDraft.statusMessage = event.target.value; syncFacadeCommitActions(); });
    for (const select of roots.facade.querySelectorAll("[data-facade-rank]")) select.addEventListener("change", () => { state.facadeDraft[select.dataset.facadeRank] = select.value; updateFacadeRank(); });
    roots.facade.querySelector("[data-facade-reset-status]")?.addEventListener("change", (event) => { state.facadeDraft.resetStatus = event.target.checked; syncFacadeCommitActions(); });
    roots.facade.querySelector("[data-facade-reset-rank]")?.addEventListener("change", (event) => { state.facadeDraft.resetRank = event.target.checked; syncFacadeCommitActions(); });
    roots.facade.querySelector("[data-facade-apply-background]")?.addEventListener("click", () => applyFacade({ action: "background", skinId: state.facadeDraft.skinId }));
    roots.facade.querySelector("[data-facade-apply-chat]")?.addEventListener("click", applyFacadeIdentity);
    roots.facade.querySelector("[data-facade-save-reset]")?.addEventListener("click", saveFacadeReset);
    for (const button of roots.facade.querySelectorAll("[data-facade-clear]")) button.addEventListener("click", async () => {
      const label = button.closest(".facade-action")?.querySelector("strong")?.textContent || "展示清理";
      const consequence = button.closest(".facade-action")?.querySelector("p")?.textContent || "此动作不可撤销。";
      if (button.dataset.facadeClear === "clear-objectives" || await requestConfirmation(button, label, consequence)) await applyFacade({ action: button.dataset.facadeClear }, button.dataset.facadeClear === "clear-objectives" ? "已复核任务与活动系列的已读状态" : "生涯设置已应用");
    });
  }

  async function applyFacadeIdentity() {
    if (state.facadeApplying) { toast("上一个生涯写入尚未完成，请稍候"); return false; }
    state.facadeApplying = true;
    state.facadeApplyError = "";
    state.facadeRequestToken = Number(state.facadeRequestToken || 0) + 1;
    state.facadeController?.abort();
    try {
      const requestedAvailability = state.facadeDraft.availability;
      await api("/api/facade/apply", { method: "POST", body: JSON.stringify({ action: "chat", availability: requestedAvailability, statusMessage: state.facadeDraft.statusMessage }) });
      state.facade = await api("/api/facade/apply", { method: "POST", body: JSON.stringify({ action: "rank", queue: state.facadeDraft.queue, tier: state.facadeDraft.tier, division: state.facadeDraft.division }) });
	  state.facadeLoadedAt = state.facade.skinsUnavailable ? 0 : Date.now();
	  const availabilityReverted = state.facade?.chat?.availability !== requestedAvailability;
      hydrateFacadeDraft(true);
      renderFacade();
	  const revertedMessage = requestedAvailability === "spectating" ? "客户端只在实际观战时才会保持“观战中”状态" : requestedAvailability === "dnd" ? "客户端只在实际进入对局时才会保持“游戏中”状态" : "客户端未保留所选在线状态，请在对应场景下重试";
	  toast(availabilityReverted ? revertedMessage : "聊天身份已应用");
    } catch (error) { toast(error.message); }
    finally { state.facadeApplying = false; }
  }

  async function saveFacadeReset() {
    const rank = { rankedLeagueQueue: state.facadeDraft.queue, rankedLeagueTier: state.facadeDraft.tier };
    if (!["MASTER","GRANDMASTER","CHALLENGER","UNRANKED"].includes(state.facadeDraft.tier)) rank.rankedLeagueDivision = state.facadeDraft.division;
    await applyFacade({ action: "login-reset", loginReset: { statusMessageEnabled: state.facadeDraft.resetStatus, statusMessage: state.facadeDraft.statusMessage, rankEnabled: state.facadeDraft.resetRank, rank } }, "登录重设已保存；连接后延时 2 秒应用，手动操作会抢占本次重设");
  }

  async function applyFacade(body, success = "生涯设置已应用") {
    if (state.facadeApplying) { toast("上一个生涯写入尚未完成，请稍候"); return false; }
    state.facadeApplying = true;
    state.facadeApplyError = "";
    state.facadeRequestToken = Number(state.facadeRequestToken || 0) + 1;
    state.facadeController?.abort();
    try {
      state.facade = await api("/api/facade/apply", { method: "POST", body: JSON.stringify(body) });
	  state.facadeLoadedAt = state.facade.skinsUnavailable ? 0 : Date.now();
      hydrateFacadeDraft(true);
      renderFacade();
      const backgroundConfirmed = body.action !== "background" || (!state.facade.profileUnavailable && Number(state.facade.profile?.backgroundSkinId) === Number(body.skinId));
      toast(backgroundConfirmed ? success : "背景请求已提交，但客户端尚未确认所选背景，请重新读取核对");
      return backgroundConfirmed;
    } catch (error) { state.facadeApplyError = error.message; toast(error.message); return false; }
    finally { state.facadeApplying = false; }
  }

  function scheduleFacadeChallengeRetry() {
	if ((!state.facade?.skinsUnavailable && state.facade?.challengesReady !== false) || Number(state.facadeChallengeRetryUsed) >= 3) return;
	state.facadeChallengeRetryUsed = Number(state.facadeChallengeRetryUsed || 0) + 1;
	clearTimeout(state.facadeChallengeRetryTimer);
	state.facadeChallengeRetryTimer = setTimeout(() => {
	  state.facadeChallengeRetryTimer = 0;
	  if (!state.active || state.tab !== "facade" || !state.connected) {
		state.facadeLoadedAt = 0;
		return;
	  }
	  const preserveDraft = facadeDraftDirty();
	  void loadFacade(true, preserveDraft, "poll");
	}, 3000);
  }

  function facadeRenderSignature(value = {}) {
	const summoner = value.summoner || {};
	const profile = value.profile || {};
	const chat = value.chat || {};
	const lol = chat.lol || {};
	const reset = value.loginReset || {};
	const resetRank = reset.rank || {};
	const rawTitle = value.challengeSummary?.title;
	let title = null;
	if (typeof rawTitle === "string") title = rawTitle.trim();
	else if (typeof rawTitle === "number" && Number.isFinite(rawTitle)) title = rawTitle;
	else if (rawTitle && typeof rawTitle === "object") title = { name: typeof rawTitle.name === "string" ? rawTitle.name.trim() : "" };
	return {
	  connected: value.connected === true,
      rankBanner: value.rankBanner || "", bannerAccent: value.bannerAccent || "",
      skinsUnavailable: value.skinsUnavailable === true,
      skinOwnershipUnavailable: value.skinOwnershipUnavailable === true,
      profileUnavailable: value.profileUnavailable === true,
	  reason: String(value.reason || ""),
	  summoner: {
		displayName: String(summoner.displayName || ""), gameName: String(summoner.gameName || ""), tagLine: String(summoner.tagLine || ""),
		profileIconId: Number(summoner.profileIconId || 0), summonerLevel: Number(summoner.summonerLevel || 0),
	  },
	  profile: { backgroundSkinId: Number(profile.backgroundSkinId || 0), backgroundChampionId: Number(profile.backgroundChampionId || 0), backgroundSkinName: String(profile.backgroundSkinName || ""), backgroundPath: String(profile.backgroundPath || ""), backgroundType: String(profile.backgroundType || "") },
	  chat: {
		icon: Number(chat.icon || 0), availability: String(chat.availability || ""), statusMessage: String(chat.statusMessage || ""),
		lol: {
		  rankedLeagueQueue: String(lol.rankedLeagueQueue || ""), rankedLeagueTier: String(lol.rankedLeagueTier || ""), rankedLeagueDivision: String(lol.rankedLeagueDivision || ""),
		},
	  },
	  challengeSummary: { title },
	  challenges: (Array.isArray(value.challenges) ? value.challenges : []).map((challenge) => ({
		id: String(challenge?.id || ""), name: String(challenge?.name || ""), iconPath: String(challenge?.iconPath || ""),
	  })),
	  challengesReady: value.challengesReady === true,
	  skins: (Array.isArray(value.skins) ? value.skins : []).map((skin) => ({
		id: Number(skin?.id || 0), name: String(skin?.name || ""), championId: Number(skin?.championId || 0), championName: String(skin?.championName || ""),
		splashPath: String(skin?.splashPath || ""), tilePath: String(skin?.tilePath || ""), owned: skin?.owned === true,
        releaseDate: skin?.releaseDate || "", releaseSortDate: skin?.releaseSortDate || "", parentSkinId: Number(skin?.parentSkinId || 0), isVariant: skin?.isVariant === true,
	  })),
	  loginReset: {
		statusMessageEnabled: reset.statusMessageEnabled === true, statusMessage: String(reset.statusMessage || ""), rankEnabled: reset.rankEnabled === true,
		rank: {
		  rankedLeagueQueue: String(resetRank.rankedLeagueQueue || ""), rankedLeagueTier: String(resetRank.rankedLeagueTier || ""), rankedLeagueDivision: String(resetRank.rankedLeagueDivision || ""),
		},
	  },
	};
  }

  async function loadFacade(force = false, preserveDraft = false, trigger = "poll") {
    if (state.connected === false || state.destroyed || state.facadeApplying) return;
    if (state.facadeController && !force) { if (state.facade) renderFacade(); return; }
    if (!state.facade) renderFacade();
	const fresh = state.facade && state.facadeLoadedAt > 0 && Date.now() - state.facadeLoadedAt < 30000;
	if (!force && fresh) { renderFacade(); return; }
    const token = state.facadeRequestToken = Number(state.facadeRequestToken || 0) + 1;
    state.facadeController?.abort();
    const controller = new AbortController();
    state.facadeController = controller;
    const timeout = setTimeout(() => controller.abort(), 15000);
	try {
	  const previousSkins = Array.isArray(state.facade?.skins) ? state.facade.skins : [];
	  const loadTrigger = ["manual", "sse", "poll"].includes(trigger) ? trigger : "poll";
	  const next = await api(`/api/facade/state?trigger=${encodeURIComponent(loadTrigger)}`, { signal: controller.signal });
      if (token !== state.facadeRequestToken || controller.signal.aborted) return;
	  if (next.connected === true && Array.isArray(next.skins) && next.skins.length === 0 && previousSkins.length) next.skins = previousSkins;
	  const unchanged = Boolean(state.facade) && JSON.stringify(facadeRenderSignature(next)) === JSON.stringify(facadeRenderSignature(state.facade));
	  state.facadeLoadedAt = next.skinsUnavailable ? 0 : Date.now();
	  if (unchanged && (preserveDraft || loadTrigger !== "manual")) {
		scheduleFacadeChallengeRetry();
		return;
	  }
	  const backgroundWasDirty = facadeBackgroundDirty();
	  state.facade = next;
	  if (!preserveDraft) hydrateFacadeDraft(true);
      else if (!backgroundWasDirty) {
        // Editing chat/rank must not freeze an otherwise clean background.
        const previousDraft = state.facadeDraft;
        hydrateFacadeDraft(true);
        state.facadeDraft = { ...previousDraft, hero: state.facadeDraft.hero, skinId: state.facadeDraft.skinId };
      }
	  renderFacade();
	  scheduleFacadeChallengeRetry();
	}
    catch (error) {
      if (token === state.facadeRequestToken && error.name !== "AbortError") {
        // A failed refresh must not tear down the last confirmed background,
        // loaded images, or an in-progress selection.
        if (!state.facade) roots.facade.innerHTML = errorCard("生涯读取失败", error.message, "facade");
        else if (trigger === "manual") toast(`生涯刷新失败，已保留当前内容：${error.message}`);
      }
    } finally {
      clearTimeout(timeout);
      if (token === state.facadeRequestToken) state.facadeController = null;
    }
  }

  function handleSuiteHardRefresh(event) {
    if (!state.active || state.tab !== "facade" || !state.connected || state.destroyed) return;
    const task = loadFacade(true, false, "manual");
    if (Array.isArray(event.detail?.waitFor)) event.detail.waitFor.push(task);
  }
  window.addEventListener("deep-legends:hard-refresh", handleSuiteHardRefresh);

  async function refreshFacadeFromEvent() {
    if (!state.connected || state.destroyed) return;
	if (!state.active || state.tab !== "facade") {
	  state.facadeLoadedAt = 0;
	  return;
	}
	if (facadeInteractionActive()) {
	  state.facadeRefreshDeferred = true;
	  return;
	}
	state.facadeRefreshDeferred = false;
	const preserveDraft = facadeDraftDirty();
	await loadFacade(true, preserveDraft, "sse");
  }

  function facadeInteractionActive() {
    const active = document.activeElement;
    const activeMenu = active?.closest?.("[data-app-select-menu]");
    const activeEditor = Boolean(active && active !== document.body && roots.facade.contains(active)
      && active.matches?.('input:not([type="checkbox"]), textarea, select')
      && !(activeMenu && activeMenu.hidden));
    return state.facadePointerActive
      || activeEditor
      || Boolean(roots.facade.querySelector('[data-app-select-trigger][aria-expanded="true"]'));
  }

  function queueFacadeRefresh(delay = 800) {
    if (state.destroyed) return;
    clearTimeout(state.facadeRefreshTimer);
    state.facadeRefreshTimer = 0;
    if (!state.active || state.tab !== "facade") {
      state.facadeLoadedAt = 0;
      state.facadeRefreshDeferred = false;
      return;
    }
    state.facadeRefreshTimer = setTimeout(() => {
      state.facadeRefreshTimer = 0;
      void refreshFacadeFromEvent();
    }, delay);
  }

  function flushDeferredFacadeRefresh() {
    if (!state.facadeRefreshDeferred) return;
    setTimeout(() => {
      if (state.facadeRefreshDeferred) queueFacadeRefresh(0);
    }, 0);
  }

  function claimSelectionKeys(item) {
    return Array.isArray(item?.claimKeys) && item.claimKeys.length ? item.claimKeys : [item?.key].filter(Boolean);
  }

  function claimHasFailure(item) {
    return claimSelectionKeys(item).some((key) => state.claimFailures.has(key));
  }

  function claimVisible(item, filter = state.claimFilter) {
    const failed = claimHasFailure(item);
    if (filter === "failed") return failed;
    if (failed || !claimActionable(item)) return false;
    if (filter === "historical") return item.historical;
    if (filter === "choice") return item.needsChoice;
    if (filter === "chain") return Boolean(item.chainId || item.chainCount);
    return true;
  }

  function claimPresentationItems(items) {
    // One selectable row per entitlement, never merge unrelated grants merely
    // because they share a category (or the same essence/item ID).
    return (Array.isArray(items) ? items : []).map((item) => ({ ...item, claimKeys: [item.key] }));
  }

  function choiceValid(item) {
    if (!claimActionable(item)) return false;
    if (!item.needsChoice) return true;
    const count = state.claimChoices.get(item.key)?.size || 0;
    const minimum = Math.max(1, Number(item.minSelections || 1));
    const maximum = Math.min(Number(item.maxSelections || item.items?.length || 1), item.items?.length || 1);
    return count >= minimum && count <= maximum;
  }

  function claimActionable(item) {
    return item?.actionable !== false;
  }

  function selectableClaimKeys(items) {
    return (Array.isArray(items) ? items : []).filter(claimVisible).filter((item) => claimActionable(item) && choiceValid(item)).flatMap(claimSelectionKeys);
  }

  function claimEventGroups(items) {
    const groups = new Map();
    for (const item of items) {
      const key = item.eventId ? `event:${item.eventId}` : "unknown";
      if (!groups.has(key)) groups.set(key, { id: item.eventId || "", name: item.eventId ? (item.eventName || "活动名称待确认") : "活动归属待确认", items: [] });
      groups.get(key).items.push(item);
    }
    return [...groups.values()].map(group => {
      // The card groups presentation only. Choices, failures and execution retain
      // their original entitlement keys; no currency/title-based merging.
      if (!group.id) return `<section class="claim-event-group"><h3>${escapeHTML(group.name)}</h3>${group.items.map(item => claimRow(item)).join("")}</section>`;
      const keys = group.items.filter(item => claimActionable(item) && choiceValid(item)).flatMap(claimSelectionKeys);
      const selected = keys.filter(key => state.selectedClaims.has(key)).length;
      return `<article class="claim-event-card"><header><input class="claim-check" type="checkbox" data-claim-event="${escapeHTML(group.id)}" data-partial="${selected > 0 && selected < keys.length}" aria-label="选择活动 ${escapeHTML(group.name)} 中可领取的奖励"${checked(keys.length > 0 && selected === keys.length)} ${!keys.length || state.claiming ? "disabled" : ""}><h3>${escapeHTML(group.name)}</h3><small>${group.items.length} 份奖励</small></header><div class="claim-event-rewards">${group.items.map(item => claimRow(item, true)).join("")}</div></article>`;
    }).join("");
  }

  function renderClaims() {
    const response = state.claims || { items: [], sources: {} };
    const items = Array.isArray(response.items) ? response.items : [];
    const itemsFor = (filter) => claimPresentationItems(items.filter((item) => claimVisible(item, filter)));
    const counts = {
      all: itemsFor("all").length,
      historical: itemsFor("historical").length,
      choice: itemsFor("choice").length,
      chain: itemsFor("chain").length,
      failed: itemsFor("failed").length,
    };
    metrics.claim.textContent = String(counts.all);
    const visible = itemsFor(state.claimFilter);
    const scannedAt = response.scannedAt ? relativeTime(response.scannedAt) : "尚未扫描";
    roots.sweep.className = "";
    roots.sweep.innerHTML = `<div class="sweep-toolbar"><div class="sweep-sources">${["grant","mission","event"].map((source) => {
      const current = response.sources?.[source] || {};
      const className = current.state === "failed" ? " is-failed" : current.state !== "available" ? " is-unavailable" : "";
      return `<span class="sweep-source${className}" title="${escapeHTML(current.detail || current.state || "")}"><i aria-hidden="true"></i>${sourceNames[source]} <b>${Number(current.count || 0)}</b></span>`;
    }).join("")}<span class="sweep-source is-unavailable" title="普通战利品宝箱不是待领取账本，不在本页自动处理"><i aria-hidden="true"></i>战利品宝箱 <b>0</b></span></div><div class="sweep-toolbar-actions"><small>上次扫描 · ${escapeHTML(scannedAt)}</small><button class="button button-secondary" type="button" data-claim-scan>重新扫描</button></div></div>
      <div class="sweep-filters" role="tablist" aria-label="领奖筛选">${[["all","全部"],["historical","历史活动"],["choice","需要选择"],["chain","任务链"],["failed","已失败"]].map(([key, label]) => `<button class="suite-filter${state.claimFilter === key ? " is-active" : ""}" type="button" role="tab" aria-selected="${state.claimFilter === key}" data-claim-filter="${key}">${label} ${counts[key]}</button>`).join("")}</div>
      ${visible.length ? `<div class="claim-list">${claimEventGroups(visible)}</div>` : `<div class="sweep-empty"><div><strong>${items.length ? "当前筛选下没有条目" : "没有发现待领取奖励"}</strong><p>${items.length ? "切换筛选查看其他来源。" : "奖励账本、任务与事件中心都已扫描。"}</p></div></div>`}
      <div class="sweep-sticky"><div class="sweep-progress-copy"><strong>已选 ${state.selectedClaims.size} 项</strong><span class="sweep-progress"><i data-claim-progress></i></span><span>已完成 ${state.claimProgress.done} / ${state.claimProgress.total || items.length} · 失败 ${state.claimProgress.failed}</span></div><div class="sweep-actions"><button class="button button-secondary" type="button" data-claim-select-all>全选可领取</button><button class="button button-primary" type="button" data-claim-run ${state.selectedClaims.size && !state.claiming ? "" : "disabled"}>${state.claiming ? "领取中…" : "开始领取"}</button></div></div>
      <div class="suite-note sweep-privacy"><span aria-hidden="true">!</span><span>领取过程<strong>逐项独立执行</strong>：某一项失败不会中断后面的项，客户端错误体会内联显示。<strong>待领取明细不会落盘</strong>，关闭页面即从内存中丢弃。</span></div>`;
    const progress = roots.sweep.querySelector("[data-claim-progress]");
    const denominator = Math.max(1, state.claimProgress.total || items.length);
    progress?.style.setProperty("--claim-progress", `${Math.min(100, Math.round(state.claimProgress.done / denominator * 100))}%`);
    bindClaimControls(visible);
  }

  function claimRow(item, compact = false) {
    const claimKeys = claimSelectionKeys(item);
    const failure = claimKeys.map((key) => state.claimFailures.get(key)).find(Boolean);
    const selectedRow = claimKeys.length > 0 && claimKeys.every((key) => state.selectedClaims.has(key));
    const choices = state.claimChoices.get(item.key) || new Set();
    const candidateCount = item.items?.length || 0;
    const maximum = Math.min(Number(item.maxSelections || candidateCount || 1), candidateCount || 1);
    const actionable = claimActionable(item);
    const needsChoice = actionable && item.needsChoice && !choiceValid(item);
    const choiceLabel = item.needsChoice && candidateCount > maximum ? `<span class="suite-chip is-warning">${candidateCount} 选 ${maximum}</span>` : "";
    const tag = compact ? "div" : "article";
    return `<${tag} class="claim-row${compact ? " is-compact" : ""}${selectedRow ? " is-selected" : ""}${failure ? " is-failed" : ""}" data-claim-row="${escapeHTML(item.key)}"><input class="claim-check" type="checkbox" aria-label="选择 ${escapeHTML(item.title || "奖励")}" data-claim-check="${escapeHTML(item.key)}"${checked(selectedRow)} ${!actionable || needsChoice || state.claiming ? "disabled" : ""}><div class="claim-main"><div class="claim-meta">${compact ? "" : `<span class="claim-title">${escapeHTML(item.title || "待领取奖励")}</span><span class="suite-chip is-accent">${escapeHTML(sourceNames[item.source] || item.source)}</span>`}${item.historical ? '<span class="suite-chip">历史活动</span>' : ""}${choiceLabel}${item.chainCount ? `<span class="suite-chip">任务链 ${Number(item.chainIndex || 1)}/${Number(item.chainCount)}</span>` : ""}${item.overlapWith ? `<span class="suite-chip is-warning">与“${escapeHTML(sourceNames[item.overlapWith] || item.overlapWith)}”重叠</span>` : ""}</div>${candidateCount ? `<div class="claim-tiles">${item.items.map((reward, index) => rewardTile(item, reward, index, choices, maximum)).join("")}</div>` : ""}${!actionable && item.detail ? `<div class="claim-choice-hint"><span aria-hidden="true">i</span><span>${escapeHTML(item.detail)}</span></div>` : ""}${needsChoice ? `<div class="claim-choice-hint"><span aria-hidden="true">!</span><span>需要你先选出 ${Math.max(1, Number(item.minSelections || 1))} 项；工具不会随机替你决定。</span></div>` : ""}${failure ? `<div class="claim-error"><span aria-hidden="true">×</span><span>${escapeHTML([failure.statusCode, failure.errorCode].filter(Boolean).join(" · ") || "领取失败")}：${escapeHTML(failure.message || "客户端未提供原因")} ${escapeHTML(failure.consequence || "本项失败不会中断其它条目。")}</span></div>` : ""}</div><div class="claim-status">${!actionable ? '<span class="suite-chip">需在客户端领取</span>' : failure ? '<span class="suite-chip is-danger">领取失败</span>' : needsChoice ? "需要选择" : "待领取"}</div></${tag}>`;
  }

  function rewardTile(item, reward, index, choices, maximum) {
    const id = String(reward.id || reward.itemId || index);
    const chosen = choices.has(id);
    const quantity = Number(reward.quantity);
    const quantityLabel = Number.isFinite(quantity) && quantity > 0 ? `×${quantity}` : "数量待客户端确认";
    const content = `${reward.iconUrl ? `<img data-queued-src="${imageURL(reward.iconUrl)}" alt="">` : '<span class="suite-tile-icon" aria-hidden="true">◇</span>'}<span><strong>${escapeHTML(reward.title || reward.itemType || reward.itemId || "奖励")}</strong><small>${quantityLabel}</small></span>`;
    if (!item.needsChoice) return `<span class="suite-tile">${content}</span>`;
    return `<button class="suite-tile${chosen ? " is-selected" : ""}" type="button" aria-pressed="${chosen}" data-claim-choice="${escapeHTML(item.key)}" data-reward-id="${escapeHTML(id)}" data-choice-max="${maximum}" ${state.claiming ? "disabled" : ""}>${content}</button>`;
  }

  function bindClaimControls(visibleItems = []) {
    const visibleByKey = new Map(visibleItems.map((item) => [item.key, item]));
    for (const input of roots.sweep.querySelectorAll("[data-claim-event]")) {
      input.indeterminate = input.dataset.partial === "true";
      input.addEventListener("change", () => {
        const keys = visibleItems.filter(item => item.eventId === input.dataset.claimEvent && claimActionable(item) && choiceValid(item)).flatMap(claimSelectionKeys);
        for (const key of keys) input.checked ? state.selectedClaims.add(key) : state.selectedClaims.delete(key);
        renderClaims();
      });
    }
    roots.sweep.querySelector("[data-claim-scan]")?.addEventListener("click", () => loadClaims(true));
    for (const button of roots.sweep.querySelectorAll("[data-claim-filter]")) button.addEventListener("click", () => { state.claimFilter = button.dataset.claimFilter; renderClaims(); });
    for (const input of roots.sweep.querySelectorAll("[data-claim-check]")) input.addEventListener("change", () => {
      const item = visibleByKey.get(input.dataset.claimCheck);
      for (const key of claimSelectionKeys(item)) input.checked ? state.selectedClaims.add(key) : state.selectedClaims.delete(key);
      renderClaims();
    });
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
      for (const key of selectableClaimKeys(state.claims?.items || [])) state.selectedClaims.add(key);
      renderClaims();
    });
    roots.sweep.querySelector("[data-claim-run]")?.addEventListener("click", executeClaims);
  }

  function recordClaimProgress(reason, started) {
    void fetch("/api/diagnostics/client", {method: "POST", headers: {"Content-Type":"application/json"},
      body: JSON.stringify({event:"claim_progress_client", reason, claiming: state.claiming,
        done: state.claimProgress?.done || 0, total: state.claimProgress?.total || 0, durationMs: Date.now() - started})}).catch(() => {});
  }

  async function executeClaims() {
    const queue = [...state.selectedClaims];
    if (!queue.length || state.claiming) return;
    state.claiming = true;
    state.claimProgress = { done: 0, total: queue.length, failed: 0 };
    renderClaims();
    const started = Date.now();
    recordClaimProgress("begin", started);
    const watchdog = setInterval(() => recordClaimProgress("heartbeat", started), 5000);
    try {
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
        if (error.errorKind === "timeout") recordClaimProgress("item-timeout", started);
        state.claimFailures.set(key, { message: error.message, consequence: "本项失败不会中断其它条目。" });
        state.claimProgress.failed += 1;
      }
      state.claimProgress.done += 1;
      renderClaims();
    }
    } finally {
      clearInterval(watchdog);
      state.claiming = false;
      recordClaimProgress("end", started);
      renderClaims();
    }
    void loadClaims(true);
    toast(`领取完成：成功 ${state.claimProgress.total - state.claimProgress.failed} 项，失败 ${state.claimProgress.failed} 项`);
  }

  async function loadClaims(force = false) {
    if (state.claims && !force) { renderClaims(); return; }
    try {
      state.claims = await api("/api/claim/scan", {}, 15000, "请求超时，可重新扫描后重试；本项失败不影响其它条目。");
      for (const item of state.claims.items || []) if (item.failure && !state.claimFailures.has(item.key)) state.claimFailures.set(item.key, item.failure);
      for (const key of [...state.selectedClaims]) if (!state.claims.items?.some((item) => item.key === key)) state.selectedClaims.delete(key);
      renderClaims();
    } catch (error) { roots.sweep.innerHTML = errorCard("未领取奖励扫描失败", error.message, "sweep"); }
  }

  function suiteDisplayPhase(phase) {
    return ["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(phase) ? "EndOfGame" : phase;
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
    ({ watch: loadWatch, rig: loadRig, facade: () => loadFacade(true, false, "manual"), sweep: loadClaims, champselect: loadChampSelect }[reload.dataset.suiteReload])?.(true);
  });



  panel.querySelector("[data-suite-retry]")?.addEventListener("click", () => document.getElementById("refresh")?.click());
  function handleLazySection(event) {
    state.active = event.detail?.name === "suite";
    if (!state.active) {
      clearTimeout(state.facadeChallengeRetryTimer);
      state.facadeChallengeRetryTimer = 0;
      return;
    }
    if (event.detail?.navigation?.section === "suite" && event.detail.navigation.tab) activateTab(event.detail.navigation.tab);
    subtitle.textContent = tabCopy[state.tab];
    loadAll();
  }
  window.addEventListener("deep-legends:section", handleLazySection);
  window.deepLegendsSections?.register("suite", handleLazySection);
  window.addEventListener("deep-legends:navigate", (event) => { if (event.detail?.section === "suite" && event.detail?.tab) activateTab(event.detail.tab); });
  (window.deepLegendsSections?.listen || window.addEventListener.bind(window))("deep-legends:status", (event) => {
    const wasConnected = state.connected;
    if (typeof event.detail?.eventStream === "boolean") state.eventStream = event.detail.eventStream;
    const identity = event.detail?.summoner;
    const account = identity ? `${identity.gameName || identity.displayName || ""}#${identity.tagLine || ""}` : "";
    if (account && state.facadeAccount && account !== state.facadeAccount) { setConnected(false); state.facadeDraft = null; }
    if (account) state.facadeAccount = account;
    setConnected(Boolean(event.detail?.connected));
	if (state.connected && !wasConnected) { state.facadeChallengeRetryUsed = false; void loadFacade(false, false, "poll"); }
    if (state.connected && state.active) loadAll(!wasConnected && state.tab !== "facade");
    else if (state.rig) renderRig();
  });
  window.addEventListener("deep-legends:live-disconnected", () => {
    state.eventStream = false;
    if (state.rig) renderRig();
  });
  (window.deepLegendsSections?.listen || window.addEventListener.bind(window))("deep-legends:gameflow", (event) => {
	state.phase = event.detail?.phase || state.phase;
	if (state.watch) renderWatch();
	if ((event.detail?.changed || event.detail?.phase) && state.active && state.connected) {
	  state.champSelectRuntime = null;
	  void loadChampSelect(false);
	}
  });
  window.addEventListener("deep-legends:watch", (event) => handleWatchEvent(event.detail?.event || event.detail || ""));
  window.addEventListener("deep-legends:claim-changed", () => { if (state.active) loadClaims(true); else state.claims = null; });
  window.addEventListener("deep-legends:facade-changed", () => queueFacadeRefresh());

  const beginFacadePointerInteraction = (event) => {
    if (roots.facade.contains(event.target)) state.facadePointerActive = true;
    else flushDeferredFacadeRefresh();
  };
  const endFacadePointerInteraction = () => {
    if (!state.facadePointerActive) return;
    state.facadePointerActive = false;
    flushDeferredFacadeRefresh();
  };
  document.addEventListener("pointerdown", beginFacadePointerInteraction);
  document.addEventListener("mousedown", beginFacadePointerInteraction);
  document.addEventListener("pointerup", endFacadePointerInteraction);
  document.addEventListener("mouseup", endFacadePointerInteraction);
  document.addEventListener("pointercancel", endFacadePointerInteraction);
  document.addEventListener("focusout", (event) => {
    if (roots.facade.contains(event.target)) flushDeferredFacadeRefresh();
  });

  function disposeSuite() {
    state.destroyed = true;
    closeChampSelectDialog();
    state.facadeRequestToken = Number(state.facadeRequestToken || 0) + 1;
    state.facadeController?.abort();
    clearTimeout(state.facadeRefreshTimer);
    clearTimeout(state.facadeChallengeRetryTimer);
    state.facadeRefreshDeferred = false;
  }
  window.addEventListener("deep-legends:dispose", disposeSuite, { once: true });
  window.addEventListener("beforeunload", () => {
    disposeSuite();
    state.scroll[state.tab] = Number(appScroll?.scrollTop || 0);
    writePreference("suite-scroll", JSON.stringify(state.scroll));
  });

  setupTabs();
})();
