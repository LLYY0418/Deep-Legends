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
  window.deepLegendsRuntime = Object.freeze({ createCache: (options) => new ResponseCache(options), escapeHTML });
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
    if (!["current_game_client", "watch_settings_client", "champ_select_filter_client", "champselect_dialog_client", "live_refresh_client", "local_request_client", "card_image_stalled"].includes(event)) return;
    // Background observations share one in-flight slot and never retry. A slow
    // diagnostics endpoint must not occupy the connections needed by the UI.
    const sampled = event === "live_refresh_client" || event === "local_request_client";
    const now = Date.now();
    if (sampled && (sampledPending.size || now - (sampledAt.get(event) ?? -Infinity) < (event === "local_request_client" ? 10000 : 1000))) { increment("transportSuppressed"); return; }
    const body = { event, reason };
    for (const key of ["revision", "durationMs", "pendingSaves", "playersReceived", "rendered100", "rendered200", "teamsReceived", "cacheAgeMs", "httpStatus", "requestId", "itemCount"]) {
      if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1000000, Math.floor(fields[key])));
    }
    for (const key of ["masterEnabled", "customPaused", "champSelectEnabled", "autoMatchmakingEnabled", "forceRefresh", "hidden", "phaseChanged", "liveRefreshQueued"]) if (typeof fields[key] === "boolean") body[key] = fields[key];
    if (fields.source === "OP.GG") body.source = fields.source;
    if (event === "live_refresh_client") {
      if (["direct", "event", "sse", "poll", "resync", "interval"].includes(fields.source)) body.source = fields.source;
      if (["overview", "live", "champions", "favorites", "suite", "collection", "tools"].includes(fields.section)) body.section = fields.section;
    }
    if (event === "local_request_client") {
      if (["status", "gameplay", "champions", "collection", "pro-players", "friends", "image", "section-loader", "other"].includes(fields.endpoint)) body.endpoint = fields.endpoint;
      for (const key of ["startedAt", "completedAt"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1e13, Math.floor(fields[key])));
      // R127 P0-2：图片队列的排队/加载计时与来源类别（不含具体路径）。
      for (const key of ["queueWaitMs", "loadMs", "activeSlowCount"]) if (Number.isFinite(fields[key])) body[key] = Math.max(0, Math.min(1000000, Math.floor(fields[key])));
      if (["lcu", "communitydragon", "ddragon", "gtimg"].includes(fields.imageSource)) body.imageSource = fields.imageSource;
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
    const encoded = JSON.stringify(body);
    if (flowPending.has(encoded) || flowSamples.has(encoded) && now - flowSamples.get(encoded) < 30000) { increment("transportSuppressed"); return; }
    if (flowPending.size >= 32) { increment("transportDropped"); return; }
    flowPending.add(encoded);
    if (sampled) { sampledAt.set(event, now); sampledPending.add(event); }
    void sendFlowDiagnostic(body, encoded, sampled ? 1 : 0);
  };
})();
