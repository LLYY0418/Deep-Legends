(() => {
  "use strict";

  const modules = { champions: ["champions"], "pro-players": ["pro-players"], suite: ["champions", "suite"], settings: ["champions"] };
  const flights = new Map(), ready = new Set(), callbacks = new Map(), snapshots = new Map();
  let current = "overview", navigation = {}, generation = 0;
  for (const type of ["deep-legends:status", "deep-legends:gameflow"]) {
    window.addEventListener(type, event => snapshots.set(type, event.detail));
  }
  window.addEventListener("deep-legends:navigate", event => { navigation = event.detail || {}; });

  function load(name) {
    if (ready.has(name)) return Promise.resolve();
    if (flights.has(name)) return flights.get(name);
    const task = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = `/${name}.js`;
      script.dataset.sectionModule = name;
      script.onload = () => {
        if (!callbacks.has(name)) { script.onerror(); return; }
        ready.add(name);
        script.onload = script.onerror = null;
        resolve();
      };
      script.onerror = () => {
        script.remove();
        flights.delete(name);
        reject(new Error("页面资源加载失败，请重试"));
      };
      document.head.append(script);
    });
    flights.set(name, task);
    return task;
  }

  async function activate(name) {
    current = name;
    const revision = ++generation;
    const needed = modules[name] || [];
    const fresh = needed.filter(module => !ready.has(module));
    const panel = document.getElementById(`${name}-panel`);
    document.querySelectorAll("[data-section-loading]").forEach(node => node.remove());
    if (!fresh.length) return;
    const notice = document.createElement("div");
    notice.className = "notice";
    notice.dataset.sectionLoading = name;
    notice.setAttribute("role", "status");
    notice.textContent = "正在加载页面…";
    panel?.prepend(notice);
    try {
      // Suite uses the champion search and image helpers. Load those first.
      for (const module of needed) await load(module);
      if (revision !== generation) return;
      notice.remove();
      for (const module of fresh) callbacks.get(module)?.({ detail: { name: current, navigation } });
    } catch (_) {
      if (revision !== generation) return;
      notice.setAttribute("role", "alert");
      notice.textContent = "页面资源加载失败。";
      const retry = document.createElement("button");
      retry.type = "button";
      retry.className = "text-button";
      retry.textContent = "重新加载";
      retry.addEventListener("click", () => { void activate(name); });
      notice.append(retry);
    }
  }

  window.deepLegendsSections = Object.freeze({
    activate,
    register: (name, callback) => callbacks.set(name, callback),
    listen: (type, callback) => {
      window.addEventListener(type, callback);
      if (snapshots.has(type)) callback({ detail: snapshots.get(type) });
    },
  });
})();
