(() => {
  "use strict";

  // Production never downloads demo fixtures. Gate demo API calls until the
  // dynamically loaded interceptor is installed; do not fall through to live
  // endpoints if loading the explicit demo fails.
  if (typeof document !== "undefined" && typeof location !== "undefined" && typeof window.fetch === "function") {
    const demo = new URLSearchParams(location.search).has("demo") || location.hash.includes("demo") || (() => {
      try { return localStorage.getItem("lol-loot-demo") === "1"; } catch (_) { return false; }
    })();
    if (demo) {
      window.deepLegendsDemoNativeFetch = window.fetch.bind(window);
      let finish, fail;
      const ready = new Promise((resolve, reject) => { finish = resolve; fail = reject; });
      // A failed demo may have no API callers yet.
      ready.catch(() => {});
      window.deepLegendsDemoReady = finish;
      window.fetch = (...args) => ready.then(() => window.fetch(...args));
      const script = document.createElement("script");
      script.src = "/demo-data.js";
      script.onerror = () => fail(new Error("演示数据加载失败，请刷新重试"));
      document.head.appendChild(script);
    }
  }

  // Bounded, access-ordered response caches. In-flight requests deliberately use
  // ordinary Maps: evicting a flight would permit duplicate network work.
  class ResponseCache extends Map {
    constructor({ max = 128, ttl = 300_000, now = Date.now, dispose } = {}) {
      super();
      this.limit = Math.max(1, max);
      this.ttl = ttl;
      this.now = now;
      this.dispose = dispose;
      this.timestamps = new Map();
    }
    prune() {
      const now = this.now();
      for (const [key, at] of this.timestamps) {
        if (now - at >= this.ttl) this.delete(key);
      }
    }
    get size() { this.prune(); return super.size; }
    has(key) {
      if (!super.has(key)) return false;
      if (this.now() - this.timestamps.get(key) >= this.ttl) { this.delete(key); return false; }
      return true;
    }
    get(key) {
      if (!this.has(key)) return undefined;
      const value = super.get(key);
      super.delete(key);
      super.set(key, value);
      return value;
    }
    set(key, value) {
      this.prune();
      if (super.has(key)) {
        if (super.get(key) !== value) this.delete(key);
        else super.delete(key);
      }
      super.set(key, value);
      this.timestamps.set(key, this.now());
      while (super.size > this.limit) this.delete(super.keys().next().value);
      return this;
    }
    delete(key) {
      if (!super.has(key)) return false;
      const value = super.get(key);
      this.timestamps.delete(key);
      super.delete(key);
      this.dispose?.(value, key);
      return true;
    }
    clear() { for (const key of super.keys()) this.delete(key); }
  }
  const escapeHTML = (value) => String(value ?? "").replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[character]));
  const GRADE_ORDER = Object.freeze({ OP: 0, S: 1, A: 2, B: 3, C: 4, D: 5, F: 6 });
  const gradeRank = (value) => GRADE_ORDER[String(value || "").trim().toUpperCase()] ?? 7;
  const gradeBadge = (value, extra = "") => {
    const grade = String(value || "").trim().toUpperCase();
    if (!Object.hasOwn(GRADE_ORDER, grade)) return "";
    const icon = grade === "OP" ? "op" : `yourgg-${grade.toLowerCase()}`;
    return `<img class="tier-badge${extra ? ` ${extra}` : ""}" src="/tier-icons/${icon}.svg" alt="梯度 ${grade}" decoding="async">`;
  };
  // Shared by the hero directory and history selector; metadata supplies the
  // server's Chinese/pinyin/initial/alias terms, never inferred from match names.
  function scoreChampionSearchOption(query, value, label, meta) {
    const normalize=value=>String(value || "").normalize("NFKC").toLocaleLowerCase("zh-CN").replace(/[\s\p{P}\p{S}]+/gu, "");
    query=normalize(query);if(!query)return 0;
    const score=value=>{if(value===query)return 100;if(value.startsWith(query))return 80-Math.min(20,value.length-query.length);const index=value.indexOf(query);if(index>=0)return 60-Math.min(20,index);if(query.length<3 || value.length>Math.max(48,query.length*8))return 0;let i=0;for(const c of value)if(c===query[i])i++;return i===query.length?10:0;};
    return Math.max(0,...[label,value,meta?.key,meta?.slug,meta?.nameZh,meta?.titleZh,meta?.nameEn,meta?.titleEn,...(meta?.searchTerms || [])].map(normalize).filter(Boolean).map(score));
  }
  window.deepLegendsRuntime = Object.freeze({ createCache: (options) => new ResponseCache(options), escapeHTML, gradeRank, gradeBadge, scoreChampionSearchOption });
  const flowSamples = new Map();
  const flowPending = new Set();
  const sampledPending = new Set();
  const sampledAt = new Map();
  const flowDelivery = { transportFailed: 0, transportDropped: 0, transportSuppressed: 0, transportHTTPStatus: 0, transportErrorKind: "none" };
  const increment = (key) => { flowDelivery[key] = Math.min(1000000, flowDelivery[key] + 1); };
  // Bounded retry of diagnostics only. Never retry a game action or log raw errors.
  const sendFlowDiagnostic = async (body, encoded, attempt = 0, timeoutMs = 5000) => {
    let timer, status = 0, timedOut = false;
    const controller = typeof AbortController === "function" ? new AbortController() : null;
    try {
      if (controller && typeof setTimeout === "function") {
        timer = setTimeout(() => { timedOut = true; controller.abort(); }, timeoutMs);
        timer?.unref?.();
      }
      const response = await fetch("/api/diagnostics/client", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, signal: controller?.signal, body: JSON.stringify({ ...body, ...flowDelivery }) });
      status = response.status;
      if (status < 200 || status >= 300) throw new Error("diagnostic-http");
      flowSamples.set(encoded, Date.now());
      if (flowSamples.size > 128) flowSamples.delete(flowSamples.keys().next().value);
      flowPending.delete(encoded);
    } catch (_) {
      increment("transportFailed");
      flowDelivery.transportHTTPStatus = status;
      flowDelivery.transportErrorKind = timedOut ? "timeout" : status ? "http" : "network";
      // A rejected schema or expired page session will not improve by retrying.
      if (attempt === 0 && (status === 0 || status >= 500 || status === 429) && typeof setTimeout === "function") {
        const retry = setTimeout(() => { void sendFlowDiagnostic(body, encoded, 1); }, 750);
        retry?.unref?.();
      } else {
        increment("transportDropped");
        flowPending.delete(encoded);
      }
    } finally {
      if (timer !== undefined) clearTimeout(timer);
      sampledPending.delete(body.event);
    }
  };
  // Keep individual phase observations (including identical polls), but send
  // bounded batches through one slot so telemetry cannot crowd out live reads.
  const gameflowQueue = [];
  let gameflowTimer = null, gameflowSending = false;
  const flushGameflowDiagnostics = async () => {
    if (gameflowSending || !gameflowQueue.length) return;
    if (gameflowTimer !== null) { clearTimeout(gameflowTimer); gameflowTimer = null; }
    gameflowSending = true;
    const body = { event: "gameflow_phase_client", reason: "batch", observations: gameflowQueue.splice(0, 8) };
    const encoded = JSON.stringify(body);
    flowPending.add(encoded);
    try { await sendFlowDiagnostic(body, encoded, 1); }
    finally {
      gameflowSending = false;
      if (gameflowQueue.length) scheduleGameflowDiagnostics();
    }
  };
  const scheduleGameflowDiagnostics = () => {
    if (gameflowTimer !== null || gameflowSending) return;
    gameflowTimer = setTimeout(() => { gameflowTimer = null; void flushGameflowDiagnostics(); }, 1000);
    gameflowTimer?.unref?.();
  };
  const queueGameflowDiagnostic = (reason, fields) => {
    if (!["received", "invalidate", "stale-response", "poll-failed"].includes(reason)) return;
    if (!["direct", "event", "sse", "poll", "resync", "interval", "manual"].includes(fields.source)) return;
    const rawPhase = value => typeof value === "string" && (value === "" || /^[A-Za-z][A-Za-z0-9_-]{0,63}$/.test(value)) ? value : "";
    const id = value => Number.isSafeInteger(value) && value > 0 ? value : 0;
    const observation = {
      reason, phase: rawPhase(fields.phase), previousPhase: rawPhase(fields.previousPhase), source: fields.source,
      gameId: id(fields.gameId), cachedGameId: id(fields.cachedGameId),
      gameIdComparison: ["same", "different", "first", "unavailable"].includes(fields.gameIdComparison) ? fields.gameIdComparison : "unavailable",
      phaseChanged: fields.phaseChanged === true, invalidated: fields.invalidated === true,
      receivedAt: Number.isFinite(fields.receivedAt) ? Math.max(0, Math.min(1e13, Math.floor(fields.receivedAt))) : Date.now(),
    };
    if (gameflowQueue.length >= 64) {
      // Prefer retaining new identity transitions over repeated unchanged polls.
      const unchanged = gameflowQueue.findIndex(row => !row.invalidated && row.gameIdComparison !== "different");
      gameflowQueue.splice(unchanged < 0 ? 0 : unchanged, 1);
      increment("transportDropped");
    }
    gameflowQueue.push(observation);
    scheduleGameflowDiagnostics();
  };
  let exportSequence = 0;
  window.flushFlowDiagnostics = async () => {
    // Export must not race a just-clicked filter's telemetry. Wait briefly,
    // then record remaining pending work rather than claiming a complete drain.
    void flushGameflowDiagnostics();
    const deadline = Date.now() + 1000;
    while ((flowPending.size || gameflowQueue.length) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 25));
    const body = { event: "diagnostic_delivery_client", reason: "export", requestId: ++exportSequence, transportPending: flowPending.size + gameflowQueue.length };
    await sendFlowDiagnostic(body, JSON.stringify(body), 1, 1500);
  };
  window.reportFlowDiagnostic = (event, reason, fields = {}) => {
    if (event === "gameflow_phase_client") { queueGameflowDiagnostic(reason, fields); return; }
    if (!["current_game_client", "watch_settings_client", "champ_select_filter_client", "champselect_dialog_client", "live_refresh_client", "local_request_client", "image_queue_slow", "card_image_stalled", "collection_card_image_state", "card_image_slot_reconciled", "card_image_observer_fallback", "arena_header_source", "live_render_rebuild", "live_roster_duplicate_dropped", "stale_team_two_dropped", "live_progress_apply", "status_render_failed", "lane_matchup_candidate_fetch", "lane_matchup_card", "renderer_perf", "blocking_state_client", "automatic_read_client", "collection_render_client", "overview_dirty_rescan", "summoner_copy", "overview_card_ready", "self_tab_client", "browser_cold_requests_client", "champselect_request_client"].includes(event)) return;
    if(event==='browser_cold_requests_client') {
      // Cumulative frames remain unsampled so a late body can amend the count.
      const body={event,reason,startedAt:Math.max(0,Math.min(1e13,Math.floor(fields.startedAt || 0))),count:Math.max(-1,Math.min(1000000,Math.floor(fields.count ?? -1))),resourceCount:Math.max(0,Math.min(1000000,Math.floor(fields.resourceCount || 0))),windowElapsed:fields.windowElapsed===true,timingAvailable:fields.timingAvailable===true};
      const encoded=JSON.stringify(body);if(flowPending.has(encoded) || flowSamples.has(encoded))return;
      if(flowPending.size>=32){increment("transportDropped");return;}flowPending.add(encoded);void sendFlowDiagnostic(body,encoded,0);return;
    }
    // Sample local requests by fixed endpoint category so status polling cannot
    // hide page timings. Delivery stays bounded and sampled events never retry.
    const sampled = event === "live_refresh_client" || event === "local_request_client";
    const sampleKey = event === "local_request_client" ? event + ":" + fields.endpoint : event;
    const now = Date.now();
    if (sampled && (sampledPending.size && event !== "local_request_client" || now - (sampledAt.get(sampleKey) ?? -Infinity) < (event === "local_request_client" ? 10000 : 1000))) { increment("transportSuppressed"); return; }
    const body = { event, reason };
    if(event === "summoner_copy") {
      body.ok=Boolean(fields.ok);
      if(["browser","electron","execCommand"].includes(fields.method))body.method=fields.method;
      if(["NotAllowedError","SecurityError","NotFoundError","AbortError","TypeError","Error"].includes(fields.error_name))body.error_name=fields.error_name;
    }
    if(event === "overview_card_ready") {
      body.isSelf=Boolean(fields.isSelf);
      if(["matches","ranks","champions","masteries","positions"].includes(fields.card))body.card=fields.card;
      if(["snapshot","network","opgg"].includes(fields.source))body.source=fields.source;
      body.durationMs=Math.max(0,Math.min(3600000,Math.floor(Number(fields.durationMs)||0)));
    }
    if (event === "overview_dirty_rescan") {
      body.attempt = Math.max(0, Math.min(5, Math.floor(Number(fields.attempt) || 0)));
      body.filter = fields.filter;
      body.finished_game_id = String(fields.finished_game_id || "0");
      body.finished_queue_id = Math.max(0, Math.min(100000, Number(fields.finished_queue_id) || 0));
      body.expected_present = Boolean(fields.expected_present);
      body.outcome = fields.outcome;
      body.since_end_ms = Math.max(0, Math.min(86400000, Math.floor(Number(fields.since_end_ms) || 0)));
    }
    if (event === "collection_render_client") {
      if (["owned", "remaining", "all", "chromas"].includes(fields.view)) body.view = fields.view;
      body.force = Boolean(fields.force); body.keptVisible = Boolean(fields.keptVisible);
    }
    if (event === "blocking_state_client" || event === "automatic_read_client") {
      if (["startup", "skin", "chroma", "champions", "career", "facade", "champselect", "update", "confirmation", "artwork_fullscreen", "other", "poll", "event", "direct", "dirty_rescan", "workspace", "manual"].includes(fields.source)) body.source = fields.source;
      if (["friends", "pro-players", "champions", "overview", "facade"].includes(fields.endpoint)) body.endpoint = fields.endpoint;
      if (event === "blocking_state_client" && reason === "skip" && fields.skip_reason === "client-exiting") body.skip_reason = "client-exiting";
      if (event === "blocking_state_client" && reason === "hide" && ["self-tab-ready", "connect-failed", "hard-timeout", "timeout", "no-client-process", "suppressed"].includes(fields.hide_reason)) body.hide_reason = fields.hide_reason;
    }
    if(["self_tab_client","overview_card_ready"].includes(event) && Number.isFinite(fields.startedAt))body.startedAt=Math.max(0,Math.min(1e13,Math.floor(fields.startedAt)));
    if (event === "arena_header_source") {
      if (Number.isInteger(fields.championId) && fields.championId > 0) body.championId = Math.min(1000000, fields.championId);
      if (Number.isInteger(fields.rank) && fields.rank >= 0) body.rank = Math.min(1000000, fields.rank);
      if (Number.isInteger(fields.listGames) && fields.listGames >= 0) body.listGames = Math.min(100000000, fields.listGames);
      if (["OP", "S", "A", "B", "C", "D", "F"].includes(fields.grade)) body.grade = fields.grade;
      if (typeof fields.hasListRow === "boolean") body.hasListRow = fields.hasListRow;
    }
    for (const key of ["revision", "durationMs", "pendingSaves", "playersReceived", "rendered100", "rendered200", "teamsReceived", "cacheAgeMs", "httpStatus", "requestId", "itemCount"]) {
      if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1000000, Math.floor(fields[key])));
    }
    for (const key of ["masterEnabled", "customPaused", "champSelectEnabled", "autoMatchmakingEnabled", "forceRefresh", "hidden", "phaseChanged", "liveRefreshQueued"]) if (typeof fields[key] === "boolean") body[key] = fields[key];
    if (fields.source === "OP.GG") body.source = fields.source;
    if (event === "live_refresh_client") {
      if (["direct", "event", "sse", "poll", "resync", "interval"].includes(fields.source)) body.source = fields.source;
      if (["overview", "live", "champions", "favorites", "suite", "collection", "tools"].includes(fields.section)) body.section = fields.section;
    }
    if (event === "local_request_client" || event === "image_queue_slow" || event === "champselect_request_client") {
      if (["status", "gameplay", "champions", "collection", "pro-players", "friends", "image", "section-loader", "other", "overview", "live", "facade", "watch", "rig", "claim", "champselect"].includes(fields.endpoint)) body.endpoint = fields.endpoint;
      for (const key of ["startedAt", "completedAt"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1e13, Math.floor(fields[key])));
      // R127 P0-2：图片队列的排队/加载计时与来源类别（不含具体路径）。
      for (const key of ["queueWaitMs", "loadMs", "activeSlowCount"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1000000, Math.floor(fields[key])));
      if (["lcu", "communitydragon", "ddragon", "gtimg", "builtin"].includes(fields.imageSource)) body.imageSource = fields.imageSource;
    }
    // R209：只放行计数与布尔值；等待状态的三十秒限频由 app.js 负责。
    if (["collection_card_image_state", "card_image_slot_reconciled", "card_image_observer_fallback"].includes(event)) {
      for (const key of ["activeCount", "activeJobs", "queued", "pendingObserved", "visiblePending", "oldestActiveAgeMs", "beforeCount", "afterCount", "beforeRemoteCount", "afterRemoteCount", "count"]) {
        if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1000000, Math.floor(fields[key])));
      }
      if (typeof fields.observerRootOk === "boolean") body.observerRootOk = fields.observerRootOk;
    }
    // R130 P1-6：卡片图看门狗超时上报。只放行队列计数、候选序号与来源类别，
    // 不放行任何资源路径。限速由 app.js 的 reportCardImageStall 负责（每 10 秒
    // 最多一条）；这里不进 sampled 通道——那条通道的 in-flight 集合是所有 sampled
    // 事件共用的，local_request_client 在途时会把 stall 一起压掉。
    if (event === "card_image_stalled") {
      for (const key of ["activeCardImages", "queued", "sourceIndex"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1000000, Math.floor(fields[key])));
      if (["lcu", "communitydragon", "ddragon", "gtimg"].includes(fields.imageSource)) body.imageSource = fields.imageSource;
    }
    if (["no-reference", "destroyed", "external-render", "private-kr", "reference-changed"].includes(fields.gate)) body.gate = fields.gate;
    if (/^cg-[0-9]{13}-[0-9]{1,6}$/.test(fields.traceId || "")) body.traceId = fields.traceId;
    if (["active", "none", "unsupported", "error"].includes(fields.phase)) body.phase = fields.phase;
    if (["none", "http", "timeout", "network", "decode", "read", "canceled", "invalid-response", "other"].includes(fields.errorKind)) body.errorKind = fields.errorKind;
    for (const key of ["requestedPosition", "resolvedPosition"]) {
      if (["all", "top", "jungle", "middle", "bottom", "utility", "mid", "adc", "support"].includes(fields[key])) body[key] = fields[key];
    }
    if (["live_roster_duplicate_dropped", "stale_team_two_dropped"].includes(event)) {
      for (const key of ["count", "queueId", "gameId"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(key === "gameId" ? 1e13 : 10000, Math.floor(fields[key])));
    }
    if (event === "live_render_rebuild") {
      if (/^[a-zA-Z0-9_.-]{1,120}$/.test(fields.shellNode || "")) body.shellNode = fields.shellNode;
      for (const [field,allowed] of [["counts",["full","status","runes","build","insight","banner","tabs"]],["sources",["direct","manual","interval","sse","event","poll","resync","recommendation","rune","catalog","progress","unknown","load","timer","navigation","selection","settings","tabs","image"]],["fullReasons",["tab-row","banner","panel-count","lane-slot","other"]]]) {
        body[field] = {};
        for (const key of allowed) if (Number.isFinite(fields[field]?.[key])) body[field][key] = Math.max(0,Math.min(1000000,Math.floor(fields[field][key])));
      }
      for (const key of ["total","windowMs","imagesRecreated","rowsReplaced"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0,Math.min(1000000,Math.floor(fields[key])));
      if (["ChampSelect","GameStart","InProgress","Reconnect","EndOfGame","Lobby","None"].includes(fields.phase)) body.phase = fields.phase;
    }
    if (event === "live_progress_apply") {
      for (const key of ["applied","ignored_same_game","ignored_stale","windowMs"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0,Math.min(1000000,Math.floor(fields[key])));
    }
    if (event === "lane_matchup_candidate_fetch") {
      for (const key of ["enemyLockedCount","enemyPositionKnownCount","allyPositionKnownCount","enemyChampionId","rowCount","queueId","gameId"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0,Math.min(1e13,Math.floor(fields[key])));
      for (const key of ["selfPosition","position"]) if (["top","jungle","mid","adc","support"].includes(fields[key])) body[key] = fields[key];
      if (["all","iron","bronze","silver","gold","gold_plus","platinum","platinum_plus","emerald","emerald_plus","diamond","diamond_plus","master","master_plus","grandmaster","challenger"].includes(fields.tier)) body.tier = fields.tier;
    }
    if (event === "lane_matchup_card") {
      if (fields.placement === "tab-row") body.placement = "tab-row";
      if (["a","b","a+b"].includes(fields.mode)) body.mode = fields.mode;
      for (const key of ["shown","ownLocked"]) if (typeof fields[key] === "boolean") body[key] = fields[key];
      if (["pair-no-data","pair-pending","pair-failed","candidates-empty"].includes(fields.hiddenReason)) body.hiddenReason = fields.hiddenReason;
      if (Number.isInteger(fields.enemyChampionId)) body.enemyChampionId = Math.max(0,Math.min(1000000,fields.enemyChampionId));
      if (["all","iron","bronze","silver","gold","gold_plus","platinum","platinum_plus","emerald","emerald_plus","diamond","diamond_plus","master","master_plus","grandmaster","challenger"].includes(fields.tier)) body.tier = fields.tier;
    }
    if (["local_request_client","champselect_request_client"].includes(event) && Number.isFinite(fields.responseBytes)) body.responseBytes = Math.max(0, Math.min(2147483648, Math.floor(fields.responseBytes)));
    if (event === "renderer_perf") {
      for (const key of ["windowMs","longtaskCount","longtaskTotalMs","longtaskMaxMs","timerLagCount","timerLagMaxMs","heapUsedMb","heapLimitMb","domNodes","imgCount"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1e9, key === "windowMs" ? Math.floor(fields[key]) : fields[key]));
      body.groups = (Array.isArray(fields.groups) ? fields.groups : []).slice(0,32).filter(row => ["overview","live","champions","favorites","suite","settings","pro-players"].includes(row.section) && ["main","watch","rig","facade","sweep","champselect","collection","account","facade-collection","items","pools","icons","banners","runes","build","specialist","pro","opgg"].includes(row.tab)).map(row => ({section:row.section,tab:row.tab,count:Math.max(0,Math.min(1e6,Number(row.count)||0)),totalMs:Math.max(0,Math.min(1e9,Number(row.totalMs)||0)),maxMs:Math.max(0,Math.min(1e9,Number(row.maxMs)||0))}));
    }
    if (event === "status_render_failed") {
      for (const key of Object.keys(body)) if (!["event", "reason"].includes(key)) delete body[key];
      body.errorType = ["Error", "TypeError", "ReferenceError", "RangeError", "SyntaxError", "URIError", "EvalError", "AggregateError"].includes(fields.errorType) ? fields.errorType : "Error";
      body.functionName = ["refreshStatus", "renderStatus", "renderLaunchpad", "acceptClientView", "renderClientConnection", "updateReadingOverlay", "renderNotice", "updateWorkspaceAvailability", "renderUpdateStatus", "loadClientInstallations", "loadSkins", "loadAccount", "loadPools"].includes(fields.functionName) ? fields.functionName : "other";
    }
    const encoded = JSON.stringify(body);
    if (flowPending.has(encoded) || flowSamples.has(encoded) && now - flowSamples.get(encoded) < 30000) { increment("transportSuppressed"); return; }
    if (flowPending.size >= 32) { increment("transportDropped"); return; }
    flowPending.add(encoded);
    if (sampled) { sampledAt.set(sampleKey, now); sampledPending.add(event); }
    void sendFlowDiagnostic(body, encoded, sampled ? 1 : 0);
  };
})();

// R185: application-wide observers. No response content or dynamic tab names
// enter diagnostics; request measurement consumes the same body, never a clone.
(() => {
  "use strict";
  const sections = ["overview","live","champions","favorites","suite","settings","pro-players"];
  let exactChampselectSequence = 0;
  const localOrigin = typeof location === "undefined" ? "" : location.origin;
  const tabs = ["main","watch","rig","facade","sweep","champselect","collection","account","facade-collection","items","pools","icons","banners","runes","build","specialist","pro","opgg"];
  const page = () => {
    const section = document.querySelector('.section-tab.is-active')?.dataset?.section || "overview";
    const root = document.getElementById(`${section}-panel`);
    const active = root?.querySelector('[aria-selected="true"][data-suite-tab], [aria-selected="true"][data-favorites-page], [aria-selected="true"][data-recommendation-tab]');
    const raw = active?.dataset?.suiteTab || active?.dataset?.favoritesPage || active?.dataset?.recommendationTab || "main";
    return {section:sections.includes(section)?section:"overview",tab:tabs.includes(raw)?raw:"main"};
  };
  function createRendererPerformance({now=()=>performance.now(), getPage=page, snapshot=()=>({}), report, observe, interval=setInterval, clear=clearInterval}={}) {
    let disposed=false,lastTick=now(), lastFlush=lastTick, lastReport=lastTick, groups=new Map(), count=0,total=0,max=0,lagCount=0,lagMax=0;
    const timeline=[{at:lastTick,...getPage()}];
    const markPage=()=>{if(disposed)return;const p=getPage();timeline.push({at:now(),...p});while(timeline.length>64)timeline.shift();};
    const longtasks=entries=>{for(const e of entries){const duration=Number(e.duration);if(!Number.isFinite(duration)||duration<50)continue;
      let p=timeline[0];for(const item of timeline){if(item.at<=Number(e.startTime??now())+duration)p=item;else break;}
      const key=p.section+":"+p.tab;const row=groups.get(key)||{section:p.section,tab:p.tab,count:0,totalMs:0,maxMs:0};row.count++;row.totalMs+=duration;row.maxMs=Math.max(row.maxMs,duration);groups.set(key,row);count++;total+=duration;max=Math.max(max,duration);
    }};
    const tick=()=>{const at=now(),elapsed=at-lastTick;lastTick=at;if(elapsed>1200){lagCount++;lagMax=Math.max(lagMax,elapsed-1000);}};
    const flush=()=>{const at=now();if(count || at-lastReport>=300000){report("renderer_perf","aggregated",{windowMs:at-lastFlush,longtaskCount:count,longtaskTotalMs:total,longtaskMaxMs:max,groups:[...groups.values()],timerLagCount:lagCount,timerLagMaxMs:lagMax,...snapshot()});lastReport=at;lastFlush=at;groups=new Map();count=total=max=lagCount=lagMax=0;}};
    const observer=observe?.(longtasks),lagTimer=interval(tick,1000),flushTimer=interval(flush,60000);
    return {longtasks,tick,flush,markPage,dispose(){disposed=true;observer?.disconnect?.();clear(lagTimer);clear(flushTimer);}};
  }
  function localEndpoint(input) {
    const raw=typeof input==="string"?input:input?.url;
    if(typeof raw!=="string")return null;
    let url;try{url=new URL(raw,localOrigin);}catch{return null;}
    if(url.origin!==localOrigin || !url.pathname.startsWith("/api/") || url.pathname.startsWith("/api/diagnostics/") || url.pathname==="/api/events" || url.pathname.startsWith("/api/image"))return null;
    const p=url.pathname;
    for(const [prefix,kind] of [["/api/gameplay/overview","overview"],["/api/gameplay/live","live"],["/api/facade/","facade"],["/api/watch/","watch"],["/api/rig/","rig"],["/api/claim/","claim"],["/api/champselect/","champselect"],["/api/gameplay/","gameplay"],["/api/champions/","champions"],["/api/pro-players","pro-players"],["/api/social/","friends"],["/api/status","status"]])if(p.startsWith(prefix))return kind;
    return /^\/api\/(account|skins|collection|pool|snapshots)/.test(p)?"collection":"other";
  }
  function measuredFetch(native, report, input, init) {
    const endpoint=localEndpoint(input);if(!endpoint)return native(input,init);
    const startedAt=Date.now();let finished=false,bytes, status=0;
    const exactChampselect = new URL(typeof input==="string"?input:input.url,localOrigin).pathname === "/api/champselect/state";
    const finish=(errorKind="none")=>{if(finished)return;finished=true;const completedAt=Date.now();try {report("local_request_client",errorKind==="none"?"complete":"failed",{endpoint,startedAt,completedAt,durationMs:completedAt-startedAt,httpStatus:status,errorKind,...(bytes===undefined?{}:{responseBytes:bytes})});if(exactChampselect)report("champselect_request_client",errorKind==="none"?"complete":"failed",{requestId:++exactChampselectSequence,endpoint:"champselect",startedAt,completedAt,durationMs:completedAt-startedAt,httpStatus:status,errorKind,...(bytes===undefined?{}:{responseBytes:bytes})});} catch {}};
    return Promise.resolve(native(input,init)).then(response=>{
      status=response.status;const length=response.headers?.get?.("Content-Length");if(length!==null&&length!==undefined&&/^\d+$/.test(length))bytes=Number(length);
      if(!response.ok){finish("http");return response;}if(status===204||status===202){finish();return response;}
      const text=response.text?.bind(response),json=response.json?.bind(response);
      for(const method of ["text","json"]) {
        const consume=method==="json"?json:text;if(!consume)continue;
        response[method]=async()=>{try{
          let value;
          if (method==="text" || Object.prototype.toString.call(response)==="[object Response]") {
            const raw=await text();if(typeof TextEncoder!=="undefined")bytes=new TextEncoder().encode(raw).length;value=method==="json"?JSON.parse(raw):raw;
          } else value=await consume();
          finish();return value;
        }catch(e){finish(e.name==="SyntaxError"?"decode":e.name==="AbortError"?"canceled":"read");throw e;}};
      }
      if(response.body?.getReader){const get=response.body.getReader.bind(response.body);response.body.getReader=(...args)=>{const reader=get(...args);const read=reader.read.bind(reader);let readBytes=0;reader.read=async(...a)=>{try{const chunk=await read(...a);readBytes+=chunk.value?.byteLength||0;bytes=readBytes;if(chunk.done)finish();return chunk;}catch(e){finish(e.name==="AbortError"?"canceled":"read");throw e;}};const cancel=reader.cancel.bind(reader);reader.cancel=(...a)=>{if(!finished)finish("canceled");return cancel(...a);};return reader;};}
      return response;
    },error=>{finish(error.name==="AbortError"?"canceled":"network");throw error;});
  }
  window.deepLegendsPerformance={createRendererPerformance,measuredFetch,localEndpoint};
  if(window.deepLegendsDemoNativeFetch || typeof document==="undefined" || typeof window.fetch!=="function" || typeof performance==="undefined")return;
  const original=window.fetch,native=original.bind(window);const measured=(input,init)=>measuredFetch(native,(...args)=>window.reportFlowDiagnostic?.(...args),input,init);window.fetch=measured;window.deepLegendsPerformance.requestMetricsInstalled=true;
  const monitor=createRendererPerformance({report:(...args)=>window.reportFlowDiagnostic?.(...args),snapshot:()=>{
    const memory=performance.memory;return {domNodes:document.getElementsByTagName("*").length,imgCount:document.getElementsByTagName("img").length,...(memory?{heapUsedMb:memory.usedJSHeapSize/1048576,heapLimitMb:memory.jsHeapSizeLimit/1048576}:{})};
  },observe:callback=>{try{const observer=new PerformanceObserver(list=>callback(list.getEntries()));observer.observe({type:"longtask",buffered:true});return observer;}catch{return null;}}});
  const mark=()=>queueMicrotask(()=>{try{monitor.markPage();}catch{}});window.addEventListener("deep-legends:section",mark);document.addEventListener("click",mark,true);
  window.addEventListener("deep-legends:dispose",()=>{monitor.dispose();window.removeEventListener("deep-legends:section",mark);document.removeEventListener("click",mark,true);window.deepLegendsPerformance.requestMetricsInstalled=false;if(window.fetch===measured)window.fetch=original;},{once:true});
})();

// Native user dialogs are reading/editing surfaces. Keep them open until the
// user's close/Escape; log the successful show and native close, not raw IDs.
function installBlockingDiagnostics(win, doc) {
  if (typeof win.confirm === "function" && !win.confirm.deepLegendsTracked) {
    const nativeConfirm = win.confirm.bind(win);
    const trackedConfirm = message => {
      const started = Date.now();
      win.reportFlowDiagnostic?.("blocking_state_client", "show", { source: "confirmation" });
      try { return nativeConfirm(message); }
      finally { win.reportFlowDiagnostic?.("blocking_state_client", "hide", { source: "confirmation", durationMs: Date.now() - started }); }
    };
    trackedConfirm.deepLegendsTracked = true;
    win.confirm = trackedConfirm;
  }
  const proto = win.HTMLDialogElement?.prototype;
  if (!proto?.showModal || proto.showModal.deepLegendsTracked) return;
  const shown = new WeakMap();
  const category = dialog => {
    if (dialog.matches?.(".cs-dialog")) return "champselect";
    if (dialog.matches?.(".mayhem-tier-dialog")) return "champions";
    const id = dialog.id || "";
    for (const key of ["champselect", "chroma", "skin", "career", "facade", "update", "champion"]) if (id.includes(key)) return key === "champion" ? "champions" : key;
    return "other";
  };
  const nativeShow = proto.showModal;
  function showModal() {
    const wasOpen = this.open;
    nativeShow.call(this);
    if (!wasOpen && this.open) {
      shown.set(this, Date.now());
      win.reportFlowDiagnostic?.("blocking_state_client", "show", { source: category(this) });
    }
  }
  showModal.deepLegendsTracked = true;
  proto.showModal = showModal;
  const finish = dialog => {
    const started = shown.get(dialog);
    if (started === undefined) return;
    shown.delete(dialog);
    win.reportFlowDiagnostic?.("blocking_state_client", "hide", { source: category(dialog), durationMs: Date.now() - started });
  };
  const nativeClose = proto.close;
  if (nativeClose) proto.close = function close(...args) { nativeClose.apply(this, args); if (!this.open) finish(this); };
  doc.addEventListener("close", event => finish(event.target), true);
}
if (typeof window !== "undefined" && typeof document !== "undefined") installBlockingDiagnostics(window, document);

// Observe the actual startup element, including regressions that make it visible.
function installStartupVisibilityDiagnostics(win, doc) {
  const overlay = doc.getElementById?.("startup-loading");
  if (!overlay || !win.MutationObserver) return;
  let visible = false;
  const observe = () => {
    const next = !overlay.hidden;
    if (next && !visible) win.reportFlowDiagnostic?.("self_tab_client", "overlay-shown");
    visible = next;
  };
  observe();
  const observer = new win.MutationObserver(observe);
  observer.observe(overlay, {attributes:true,attributeFilter:["hidden"]});
  win.addEventListener("deep-legends:dispose", () => observer.disconnect(), {once:true});
}
if (typeof window !== "undefined" && typeof document !== "undefined") installStartupVisibilityDiagnostics(window, document);

// R258: count completed local ResourceTiming entries beginning in the first
// three seconds after process detection. Exclude DNS/connect time from the
// pre-send wait. Late completions amend the same epoch; unsupported timing is
// sent as unavailable. No URL, account, payload or image path leaves this code.
(() => {
  'use strict';
  let windowStart=0,timer=0,publishTimer=0,elapsed=false,available=false,windowAvailable=false,droppedAt=0;
  let windowCounts={queued:0,resources:0};
  const records=[];
  const report=()=>{
    if(!windowStart)return;
    const selected=windowCounts;
    window.reportFlowDiagnostic?.('browser_cold_requests_client','sample',{startedAt:windowStart,count:windowAvailable?selected.queued:-1,resourceCount:selected.resources,windowElapsed:elapsed,timingAvailable:windowAvailable});
  };
  const scheduleReport=()=>{if(!publishTimer)publishTimer=setTimeout(()=>{publishTimer=0;report();},250);};
  const consume=entries=>{
    let changed=false;
    for(const entry of entries) {
      let url;try{url=new URL(entry.name,location.origin);}catch(_){continue;}
      if(url.origin!==location.origin || url.pathname.startsWith('/api/diagnostics/') || url.pathname==='/api/events' || !Number.isFinite(entry.requestStart) || entry.requestStart<=0)continue;
      const dns=Math.max(0,entry.domainLookupEnd-entry.domainLookupStart),connect=Math.max(0,entry.connectEnd-entry.connectStart);
      const row={at:performance.timeOrigin+entry.fetchStart,wait:Math.max(0,entry.requestStart-entry.fetchStart-dns-connect)};
      records.push(row);if(records.length>4096)droppedAt=records.shift().at;
      if(windowStart && row.at>=windowStart && row.at<windowStart+3000){windowCounts.resources++;if(row.wait>100)windowCounts.queued++;changed=true;}
    }
    if(changed)scheduleReport();
  };
  try {
    if(typeof PerformanceObserver==='function' && PerformanceObserver.supportedEntryTypes?.includes('resource')) {
      const observer=new PerformanceObserver(list=>consume(list.getEntries()));observer.observe({type:'resource',buffered:true});available=true;
    }
  }catch(_){}
  const startColdRequestWindow=at=>{
    if(!Number.isFinite(at) || at<=0 || at<=windowStart)return;
    clearTimeout(timer);windowStart=at;elapsed=Date.now()>=at+3000;windowAvailable=available && (!droppedAt || at>droppedAt);
    const selected=records.filter(row=>row.at>=at && row.at<at+3000);windowCounts={queued:selected.filter(row=>row.wait>100).length,resources:selected.length};report();
    if(!elapsed)timer=setTimeout(()=>{elapsed=true;report();},Math.max(0,at+3000-Date.now()));
  };
  if(window.deepLegendsPerformance)window.deepLegendsPerformance.startColdRequestWindow=startColdRequestWindow;
  window.addEventListener?.('deep-legends:dispose',()=>{clearTimeout(timer);clearTimeout(publishTimer);},{once:true});
})();
