(() => {
  "use strict";
  // src is absent until admission: waiting images cannot occupy an HTTP/1 socket.
  const limit = 6, pending = new Map(), active = new Map(), failed = new Map();
  const seen = new WeakMap(), retries = new WeakMap(), cooldowns = new Map();
  const loadedURLs = new Map();
  const retryDelay = 10000, maxRetries = 1;
  let disposed = false;
  const visible = new WeakSet(), prefetch = new WeakSet(), succeeded = new WeakMap();
  const connected = img => img.isConnected || prefetch.has(img);
  const observer = typeof IntersectionObserver === "function" ? new IntersectionObserver(entries => {
    for (const entry of entries) {
      if (entry.isIntersecting) { visible.add(entry.target); enqueue(entry.target); }
      else visible.delete(entry.target);
    }
  }, {rootMargin: "160px"}) : null;
  function enqueue(img) {
    if (disposed) return;
    const url = img.getAttribute("data-queued-src");
    if (!url || !connected(img) || active.has(img) || (seen.get(img) === url && !(succeeded.get(img) === url && !img.getAttribute("src")))) return;
    if (img.loading === "lazy" && observer && !visible.has(img)) { observer.observe(img); return; }
    // Reopened pickers reuse the browser's fresh cache immediately, without
    // waiting behind cold downloads. Only successful URLs qualify, for 10 min.
    if (loadedURLs.has(url) && Date.now() - loadedURLs.get(url) < 600000) {
      seen.set(img, url);
      img.hidden = false;
      const loaded = () => {
        img.removeEventListener("error", errored);
        img.dataset.imageReady = "true";
        succeeded.set(img, url); retries.delete(img); prefetch.delete(img);
      };
      const errored = () => {
        img.removeEventListener("load", loaded);
        delete img.dataset.imageReady;
        loadedURLs.delete(url); seen.delete(img); enqueue(img);
      };
      img.addEventListener("load", loaded, {once: true});
      img.addEventListener("error", errored, {once: true});
      img.loading = "eager";
      img.src = url;
      return;
    }
    pending.set(img, url); pump();
  }
  function coolDown(url) {
    if (cooldowns.has(url) || disposed) return;
    const until = failed.get(url) || 0;
    cooldowns.set(url, setTimeout(() => {
      cooldowns.delete(url);
      if ((failed.get(url) || 0) <= Date.now()) failed.delete(url);
      pump();
    }, Math.max(1, until - Date.now())));
  }
  function pump() {
    if (disposed) return;
    for (const [img, url] of pending) {
      if (active.size >= limit) break;
      if (!connected(img)) { pending.delete(img); continue; }
      if (img.getAttribute("data-queued-src") !== url) { pending.delete(img); enqueue(img); continue; }
      if (img.loading === "lazy" && observer && !visible.has(img)) { pending.delete(img); continue; }
      if ((failed.get(url) || 0) > Date.now()) { coolDown(url); continue; }
      pending.delete(img);
      seen.set(img, url);
      let timer;
      const finish = (error, cancelled = false) => {
        if (!active.has(img)) return;
        clearTimeout(timer); img.removeEventListener("load", loaded); img.removeEventListener("error", errored);
        active.delete(img);
        if (cancelled) {
          seen.delete(img);
        } else if (error) {
          delete img.dataset.imageReady;
          succeeded.delete(img);
          failed.set(url, Date.now() + retryDelay);
          if (failed.size > 512) failed.delete(failed.keys().next().value);
          const retry = retries.get(img);
          const count = retry?.url === url ? retry.count : 0;
          if (count < maxRetries) {
            retries.set(img, {url, count: count + 1});
            seen.delete(img);
            if (connected(img)) pending.set(img, url);
            coolDown(url);
          } else {
            prefetch.delete(img);
          }
        } else {
          img.dataset.imageReady = "true";
          succeeded.set(img, url);
          loadedURLs.delete(url);
          loadedURLs.set(url, Date.now());
          if (loadedURLs.size > 2048) loadedURLs.delete(loadedURLs.keys().next().value);
          retries.delete(img);
          prefetch.delete(img);
        }
        // A fallback or new selection may change the URL while this request
        // was active. Never leave that new URL waiting for an unrelated mutation.
        if (img.getAttribute("data-queued-src") !== url) {
          pending.delete(img); seen.delete(img); enqueue(img);
        }
        pump();
      };
      const loaded = () => finish(false), errored = () => finish(true);
      active.set(img, () => { img.removeAttribute("src"); finish(false, true); });
      img.addEventListener("load", loaded); img.addEventListener("error", errored);
      timer = setTimeout(() => { img.removeAttribute("src"); finish(true); }, 10000);
      delete img.dataset.imageReady;
      img.hidden = false;
      // Visibility has already been checked by our observer. Native lazy
      // loading must not defer an admitted request while it occupies a slot.
      img.loading = "eager";
      img.src = url;
    }
  }
  function scan(root = document) {
    if (disposed || !window.document) return;
    if (root.matches?.("img[data-queued-src]")) enqueue(root);
    for (const img of root.querySelectorAll?.("img[data-queued-src]") || []) enqueue(img);
  }
  function prune() {
    for (const [img, cancel] of active) if (!connected(img)) cancel();
    for (const img of pending.keys()) if (!connected(img)) pending.delete(img);
  }
  const mutations = new MutationObserver(records => {
    prune();
    for (const record of records) {
      if (record.type === "attributes") enqueue(record.target);
      else for (const node of record.addedNodes) scan(node);
    }
  });
  mutations.observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:["data-queued-src"]});
  const dispose = () => { disposed = true; mutations.disconnect(); observer?.disconnect(); for (const cancel of active.values()) cancel(); for (const timer of cooldowns.values()) clearTimeout(timer); cooldowns.clear(); pending.clear(); };
  window.addEventListener("deep-legends:dispose", dispose, {once:true});
  window.addEventListener("beforeunload", dispose, {once:true});
  scan();
  window.deepLegendsQueueImage = (img, url) => { prefetch.add(img); img.setAttribute("data-queued-src", url); enqueue(img); };
  window.deepLegendsStatusRecovery = unavailable => {
    let bar = document.getElementById("local-status-recovery");
    if (!bar && unavailable) { bar = document.createElement("div"); bar.id = "local-status-recovery"; bar.className = "local-status-recovery"; bar.setAttribute("role","status"); bar.textContent = "本地助手暂时响应较慢，正在自动重试；已加载内容仍可使用。"; document.body.prepend(bar); }
    if (bar) bar.hidden = !unavailable;
  };
})();
