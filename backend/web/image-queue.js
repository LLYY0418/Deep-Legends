(() => {
  "use strict";
  // src is absent until admission: waiting images cannot occupy an HTTP/1 socket.
  const IMAGE_QUEUE_LIMIT = 5;
  // R127 P1-a：远程来源单独限额。海克斯大乱斗的强化符文图标本机客户端没有
  // （LCU 返回 400），后端转去 raw.communitydragon.org，国内经常整张等满超时。
  // 这类图以前会占满全部 5 个名额，让本机毫秒级就能取到的英雄头像、召唤师技能、
  // 装备图标陪着排队（截图里的单字占位就是这么来的）。远程道最多 2 张，本机道
  // 因此始终至少还有 3 个名额；总名额仍是 5，加上 SSE 正好不超过浏览器对同一源
  // 的 6 条连接。
  const REMOTE_LANE_LIMIT = 2;
  // 占着名额超过这个时长即算慢图，失败上报时带上数量，用于判断名额被谁占满。
  const SLOW_ACTIVE_MS = 3000;
  const REPORTABLE_IMAGE_SOURCES = ["lcu", "communitydragon", "ddragon", "gtimg"];
  const limit = IMAGE_QUEUE_LIMIT, pending = new Map(), active = new Map(), failed = new Map();
  const seen = new WeakMap(), retries = new WeakMap(), cooldowns = new Map();
  const loadedURLs = new Map();
  const queuedAt = new WeakMap(), admittedAt = new WeakMap(), admittedLane = new WeakMap();
  const retryDelay = 10000, maxRetries = 1;
  let disposed = false;
  const visible = new WeakSet(), prefetch = new WeakSet(), succeeded = new WeakMap();
  const connected = img => img.isConnected || prefetch.has(img);
  // 分道判据：/api/image?path=… 没有 source 参数，走本机客户端；带
  // source=communitydragon|ddragon|gtimg|opgg 的是联网资源。强化符文图标虽然
  // 也没有 source 参数（后端要先吃一次 LCU 400 才回退到 CommunityDragon），
  // 但前端能用 augments/icons 路径与 art=2 标记提前识别，直接归入远程道。
  // /api/skin-art 与 /api/prestige-image 同样没有 source= 参数，后端却是去腾讯
  // 官方图片域名取原画，也必须算远程，否则皮肤列表回退时照样能占满全部名额。
  const remoteAssetPattern = /augments%2Ficons|augments\/icons|[?&]art=2|\/api\/(?:skin-art|prestige-image)\?/i;
  function imageSourceOf(url) {
    const match = /[?&]source=([^&#]*)/.exec(String(url || ""));
    if (!match) return "";
    try { return decodeURIComponent(match[1]).toLowerCase(); } catch (_) { return String(match[1]).toLowerCase(); }
  }
  function imageSourceLabel(url) {
    const value = String(url || "");
    const source = imageSourceOf(value);
    if (source) return REPORTABLE_IMAGE_SOURCES.includes(source) ? source : "";
    // 没有 source= 参数的不一定来自本机客户端：skin-art / prestige-image 是后端
    // 去腾讯官方图片域名取的原画，报成 lcu 会把慢源归错类。
    if (/\/api\/(?:skin-art|prestige-image)\?/i.test(value)) return "gtimg";
    return "lcu";
  }
  function laneOf(url) {
    const source = imageSourceOf(url);
    if (source && source !== "lcu") return "remote";
    return remoteAssetPattern.test(String(url || "")) ? "remote" : "local";
  }
  function activeLaneCount(lane) {
    let count = 0;
    for (const img of active.keys()) if (admittedLane.get(img) === lane) count++;
    return count;
  }
  function slowActiveCount() {
    const now = Date.now();
    let count = 0;
    for (const img of active.keys()) {
      const at = admittedAt.get(img);
      if (at && now - at > SLOW_ACTIVE_MS) count++;
    }
    return count;
  }
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
    // A successful URL is still admitted through the same queue. The cache only
    // records that it succeeded recently; it is not evidence that the browser
    // cache can satisfy a new element without opening a connection.
    if (loadedURLs.has(url) && Date.now() - loadedURLs.get(url) >= 600000) loadedURLs.delete(url);
    if (!pending.has(img)) queuedAt.set(img, Date.now());
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
      const lane = laneOf(url);
      // R127 P1-a：远程道满员时只跳过这一张，后面的本机图继续放行。这里绝不能
      // break，否则一张等满超时的远程图又会堵住英雄头像、技能与装备图标。
      if (lane === "remote" && activeLaneCount("remote") >= REMOTE_LANE_LIMIT) continue;
      pending.delete(img);
      seen.set(img, url);
      const admittedAtMs = Date.now();
      const queueWaitMs = Math.max(0, admittedAtMs - (queuedAt.get(img) || admittedAtMs));
      queuedAt.delete(img);
      admittedAt.set(img, admittedAtMs);
      admittedLane.set(img, lane);
      let timer, timedOut = false, exhausted = false, wasConnected = true;
      const finish = (error, cancelled = false) => {
        if (!active.has(img)) return;
        clearTimeout(timer); img.removeEventListener("load", loaded); img.removeEventListener("error", errored);
        active.delete(img);
        const loadMs = Math.max(0, Date.now() - admittedAtMs);
        const activeSlowCount = slowActiveCount();
        admittedAt.delete(img); admittedLane.delete(img);
        if (cancelled) {
          seen.delete(img);
        } else if (error) {
          delete img.dataset.imageReady;
          // R127 P0-2：失败上报补齐排队时长、加载时长、当时占着名额的慢图数量与
          // 图片来源类别（只报来源，不报具体路径）。以前这些失败被后端白名单全部
          // 拒收，图片排队卡死在日志里没有任何痕迹。
          window.reportFlowDiagnostic?.("local_request_client", "failed", {
            endpoint: "image", httpStatus: 0, errorKind: timedOut ? "timeout" : "network",
            queueWaitMs, loadMs, activeSlowCount, imageSource: imageSourceLabel(url),
          });
          succeeded.delete(img);
          failed.set(url, Date.now() + retryDelay);
          if (failed.size > 512) failed.delete(failed.keys().next().value);
          const retry = retries.get(img);
          const count = retry?.url === url ? retry.count : 0;
          if (count < maxRetries) {
            retries.set(img, {url, count: count + 1});
            seen.delete(img);
            if (connected(img)) { queuedAt.set(img, Date.now()); pending.set(img, url); }
            coolDown(url);
          } else {
            // R130 P1-A：prefetch.delete 会让 connected(img) 立刻变成 false（prefetch 是
            // deepLegendsQueueImage 给脱离文档的图片保活用的），连接状态必须在删除之前
            // 取好，否则补发的合成 error 会被自己的清理动作吞掉。
            wasConnected = connected(img);
            prefetch.delete(img);
            exhausted = true;
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
        // R130 P1-A：超时放弃走的是 img.removeAttribute("src")，而移除 src 不会
        // 触发任何 load/error 事件（Chromium 实测挂起请求移除 src 后 1.5s 内收到
        // 事件 []）。调用方挂在 onerror 上的「换下一个候选地址 + 释放名额」因此
        // 永远不会执行，收藏页 8 个卡片名额被占满后整屏永久停在「加载中」。
        // 彻底放弃（不再重试）时补发一个合成 error，让调用方按失败正常收尾。
        // 取消（cancelled）走的是 finish(false, true)，绝不能补发，否则会把已经
        // 取消的卡片任务重新激活。自身监听已在上面移除，不会自我重入。
        // data-queued-src 已经不是这个 url，说明调用方（第一层看门狗）早就推进到
        // 下一个候选了，这时再补发会让调用方多推进一次、把本来能显示的那个地址直接
        // 跳过；下面那段「URL 变了就重新入队」的逻辑会接手新地址，不需要这个 error。
        if (exhausted && !cancelled && wasConnected && img.getAttribute("data-queued-src") === url) {
          try { img.dispatchEvent(new Event("error")); } catch (_) {}
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
      timer = setTimeout(() => { timedOut = true; img.removeAttribute("src"); finish(true); }, 10000);
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
  window.deepLegendsImageQueueLimit = IMAGE_QUEUE_LIMIT;
  // R130 P1-6：卡片图看门狗上报 card_image_stalled 时要带来源类别（不带路径）。
  // 分类逻辑只有这一份，避免 app.js 各写一套把慢源归错类。
  window.deepLegendsImageSourceLabel = imageSourceLabel;
  window.deepLegendsImageQueueRemoteLimit = REMOTE_LANE_LIMIT;
  window.deepLegendsQueueImage = (img, url) => { prefetch.add(img); img.setAttribute("data-queued-src", url); enqueue(img); };
  window.deepLegendsStatusRecovery = unavailable => {
    let bar = document.getElementById("local-status-recovery");
    if (!bar && unavailable) { bar = document.createElement("div"); bar.id = "local-status-recovery"; bar.className = "local-status-recovery"; bar.setAttribute("role","status"); bar.textContent = "本地助手暂时响应较慢，正在自动重试；已加载内容仍可使用。"; document.body.prepend(bar); }
    if (bar) bar.hidden = !unavailable;
  };
})();
