(() => {
  "use strict";
  const nativeFetch = (window.deepLegendsDemoNativeFetch || window.fetch).bind(window);
  const businessFetch = window.fetch.bind(window);
  const layer = document.getElementById("license-overlay"), form = document.getElementById("license-form");
  const input = document.getElementById("license-code"), message = document.getElementById("license-message");
  const button = document.getElementById("license-submit"), frame = document.getElementById("app-frame");
  function disableLicense() {
    layer.hidden = true; frame.hidden = false; frame.removeAttribute("inert");
    form.hidden = true;
    const expiry = document.getElementById("setting-license-expiry");
    if (expiry) expiry.hidden = true;
    document.documentElement.dataset.license = "disabled";
    let started = false;
    window.deepLegendsLicense = Object.freeze({ isActive: () => true, poll() {
      if (started) return; started = true;
      window.dispatchEvent(new CustomEvent("deep-legends:license", { detail: { active: true, generation: 0 } }));
    } });
  }
  if (document.documentElement.dataset.license === "disabled" || window.desktopBackend?.licenseEnabled === false) {
    disableLicense(); return;
  }
  let active = false, observedState = "PENDING", generation = 0, requestSequence = 0, busy = false, disposed = false, timer = 0;
  const nativeWindow = typeof window.desktopBackend?.onLicenseApply === "function";
  const requests = new Set();
  const publicAPI = (path, method) => [
    "GET /api/license/status", "POST /api/license/activate", "GET /api/privacy", "GET /api/diagnostics/log",
    "POST /api/diagnostics/client", "POST /api/diagnostics/startup", "POST /api/diagnostics/startup-stage", "POST /api/quit",
    "GET /api/update/status", "POST /api/update/check", "POST /api/update/download", "POST /api/update/cancel", "POST /api/update/apply",
    "GET /api/update/settings", "POST /api/update/settings",
  ].includes(`${method} ${path}`);
  const cancelled = () => Object.assign(new Error("请求已取消"), { name: "RequestCancelled" });
  function setMessage(text, error = false) {
    message.textContent = text; message.dataset.tone = error ? "error" : "normal";
  }
  function updateExpiry(value) {
    const row = document.getElementById("setting-license-expiry");
    if (!row || value?.state !== "ACTIVE") return;
    row.hidden = false;
    const expires = value.license_expires_at;
    if (expires === undefined) { row.textContent = "授权到期：永久"; return; }
    const date = new Date(expires * 1000);
    if (!Number.isSafeInteger(expires) || expires <= 0 || !Number.isFinite(date.getTime())) return;
    const day = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
    // Local calendar date; round a partial remaining day upwards.
    const remaining = Math.max(0, Math.ceil((date.getTime() - Date.now()) / 86400000));
    row.textContent = `授权到期：${day}（剩 ${remaining} 天）`;
  }
  function updateAdmission(value) {
    const next = value?.state === "ACTIVE", changed = active !== next;
    active = next;
    if (changed) generation += 1;
    if (!active) {
      frame.setAttribute("inert", "");
      for (const controller of requests) controller.abort();
      requests.clear();
    }
    if (changed) {
      window.dispatchEvent(new CustomEvent("deep-legends:license", { detail: { active, generation } }));
    }
  }
  function render(value) {
    updateAdmission(value);
    updateExpiry(value);
    layer.hidden = active; frame.hidden = !active;
    document.documentElement.dataset.license = active ? "active" : "locked";
    if (!active) {
      if (!busy) setMessage(value?.message || "请输入注册码激活软件", value?.state !== "LOCKED" && value?.message !== "正在连接激活服务…");
      if (!busy && document.activeElement === document.body) input.focus();
    } else {
      input.value = "";
      if (document.getElementById("startup-loading").hidden) frame.removeAttribute("inert");
    }
  }
  function apply(value) {
    if (value?.state === "DISABLED") { disposed = true; clearTimeout(timer); disableLicense(); window.deepLegendsLicense.poll(); return; }
    if (!nativeWindow) { render(value); return; }
    updateAdmission(value);
    updateExpiry(value);
    if (observedState === value.state) {
      if (!active && !busy) setMessage(value?.message || "请输入注册码激活软件", value?.state !== "LOCKED" && value?.message !== "正在连接激活服务…");
      return;
    }
    observedState = value.state;
    // Keep both surfaces absent until the native controller has made geometry
    // invisible and sends a fresh, authenticated snapshot back for rendering.
    layer.hidden = true; frame.hidden = true;
    document.documentElement.dataset.license = "pending";
    window.desktopBackend.licenseObserved();
  }
  window.desktopBackend?.onLicenseApply?.(value => {
    if (disposed || !Number.isSafeInteger(value?.renderId)) return;
    ++requestSequence; observedState = value.state;
    render(value);
    requestAnimationFrame(() => requestAnimationFrame(() => {
      if (!disposed) window.desktopBackend.licenseRendered(value.renderId);
    }));
  });
  // License requests use native transport, so demo fixtures cannot authorize a
  // distributed client. All independent page modules share this admission gate.
  window.fetch = async (target, options = {}) => {
    const url = new URL(typeof target === "string" ? target : target.url, location.href);
    const method = String(options.method || target?.method || "GET").toUpperCase();
    const protectedRequest = url.origin === location.origin && url.pathname.startsWith("/api/") && !publicAPI(url.pathname, method);
    if (!protectedRequest) return businessFetch(target, options);
    if (!active || disposed) throw cancelled();
    const admitted = generation, controller = new AbortController(); requests.add(controller);
    const signal = options.signal || target?.signal, abort = () => controller.abort();
    if (signal?.aborted) abort();
    signal?.addEventListener("abort", abort, { once: true });
    try {
      const response = await businessFetch(target, { ...options, signal: controller.signal });
      if (!active || admitted !== generation || controller.signal.aborted) throw cancelled();
      return response;
    } finally { requests.delete(controller); signal?.removeEventListener("abort", abort); }
  };
  async function request(path, options = {}) {
    const controller = new AbortController(), timeout = setTimeout(() => controller.abort(), path === "/api/license/activate" ? 36000 : 10000);
    try {
      const response = await nativeFetch(path, { ...options, credentials: "same-origin", signal: controller.signal, headers: { "Content-Type": "application/json", "Accept": "application/json" } });
      if (!response.ok) throw new Error((await response.text()).trim() || "无法连接激活服务，请联网后重试");
      return await response.json();
    } finally { clearTimeout(timeout); }
  }
  async function poll() {
    if (disposed || busy) return;
    const sequence = ++requestSequence;
    try { const value = await request("/api/license/status"); if (!disposed && sequence === requestSequence) apply(value); }
    catch (_) { if (!disposed && sequence === requestSequence) apply({ state: "NETWORK_LOCKED", message: "无法连接激活服务，请联网后重试" }); }
    finally { if (!disposed) { clearTimeout(timer); timer = setTimeout(poll, 2000); } }
  }
  form.addEventListener("submit", async event => {
    event.preventDefault(); if (busy || disposed) return;
    busy = true; ++requestSequence; button.disabled = true; button.textContent = "正在激活…"; input.disabled = true; setMessage("正在激活…");
    try { apply(await request("/api/license/activate", { method: "POST", body: JSON.stringify({ code: input.value }) })); }
    catch (error) { setMessage(error.name === "AbortError" ? "无法连接激活服务，请联网后重试" : error.message, true); }
    finally { busy = false; button.disabled = false; button.textContent = "激活"; input.disabled = false; clearTimeout(timer); timer = setTimeout(poll, 2000); }
  });
  const privacy = document.getElementById("license-privacy-dialog");
  document.getElementById("license-privacy")?.addEventListener("click", async event => {
    event.preventDefault();
    try {
      const value = await request("/api/privacy");
      document.getElementById("license-privacy-text").textContent = value.licenseDisclosure;
      privacy.showModal();
    } catch (_) { setMessage("隐私说明读取失败，请重试", true); }
  });
  window.addEventListener("keydown", event => {
    if (active || layer.hidden || privacy?.open) return;
    if (event.key === "Escape") { event.preventDefault(); return; }
    if (event.key !== "Tab") return;
    const focusable = [...form.querySelectorAll("input,button,a[href]")].filter(node => !node.disabled);
    const first = focusable[0], last = focusable.at(-1);
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  });
  window.addEventListener("focusin", event => {
    if (!active && !layer.hidden && !privacy?.open && !layer.contains(event.target)) input.focus();
  });
  window.deepLegendsLicense = Object.freeze({ isActive: () => active, poll });
  window.addEventListener("deep-legends:dispose", () => { disposed = true; ++requestSequence; clearTimeout(timer); for (const controller of requests) controller.abort(); });
  window.addEventListener("focus", () => { void poll(); });
  window.addEventListener("online", () => { void poll(); });
  layer.hidden = true; frame.hidden = true;
  document.documentElement.dataset.license = "pending";
})();
