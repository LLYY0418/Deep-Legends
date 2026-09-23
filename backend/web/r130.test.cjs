'use strict';
// R130：皮肤列表过一会整屏「加载中」且不再恢复 + 头像/旗帜页面整理。
//
// P1 的根因是前端两层图片队列的配合：第一层 app.js 的卡片队列全局 8 个名额，
// 名额只在图片 onload、或 onerror 把全部候选地址试完时才释放；第二层
// image-queue.js 才真正给 <img> 设置 src。下面四条按工单 §3 逐条复现：
//
//  A —— 第二层超时放弃走的是 img.removeAttribute("src")，而移除 src 不会触发任何
//       load/error 事件（本文件先把这个前提钉死），第一层的 onerror 于是永远不
//       执行，名额永久泄漏。修复有两道防线：第二层彻底放弃时补发合成 error，
//       第一层加 25 秒看门狗。两条变异各关掉一道防线仍须 PASS，两道一起关掉
//       必须 FAIL。
//  B —— 两层各判一次可见区域：第一层按 620px 预取发名额，第二层按真实可见才
//       放行，屏幕外的卡片长期占着名额。修复是只在第一层判——拿到名额就把
//       loading 设成 eager，并且已排队未拿到名额的任务离开预取范围就撤回。
//       变异：去掉 eager 设置必须 FAIL。
//  C —— 悬停视频没有 pointerleave，鼠标移开后循环视频继续占着连接。
//  D —— 离开奖池页时不取消 el.poolSkinGrid 的卡片任务。
//
// P2–P6 是界面调整，按结构/样式/文案逐条钉住。
//
// 说明：这里跑的是真实的 image-queue.js 与从真实 app.js 原样抽出的队列函数，
// 只有「滚动容器 + 可见区域」是按工单 §1-B 的实测结论建模的：第一层的观察器
// 带 root=appScroll 与 620px 外扩，第二层的观察器 root 是视口、被 appScroll
// 裁掉，有效外扩按 0 算（工单原文：「实际上要真正可见」）。
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');

const read = (name) => fs.readFileSync(path.join(__dirname, name), 'utf8');
const flush = () => new Promise((resolve) => setImmediate(resolve));

const appSource = read('app.js');
const queueSource = read('image-queue.js');
const facadeSource = process.env.R138_FACADE_SOURCE ? fs.readFileSync(process.env.R138_FACADE_SOURCE, 'utf8') : read('favorites-facade.js');
const runtimeSource = read('runtime.js');
const html = read('index.html');
const css = read('app.css');
const featuresSource = fs.readFileSync(path.join(__dirname, '..', 'features.go'), 'utf8');

// ---------- 源码抽取（与 r123/r129 同一套花括号配对写法） ----------
function functionSource(script, name) {
  const marker = `function ${name}(`;
  const start = script.indexOf(marker);
  assert.notEqual(start, -1, `app.js 里找不到 ${name}`);
  const bodyStart = script.indexOf('{', script.indexOf(')', start));
  let depth = 0, quote = '', escaped = false;
  for (let index = bodyStart; index < script.length; index += 1) {
    const char = script[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === '\\') escaped = true;
      else if (char === quote) quote = '';
      continue;
    }
    if (char === '"' || char === "'" || char === '`') { quote = char; continue; }
    if (char === '{') depth += 1;
    if (char === '}' && --depth === 0) return script.slice(start, index + 1);
  }
  assert.fail(`花括号不配对：${name}`);
}

// 第一层卡片队列的全部函数，原样从 app.js 抽出，不做任何改写。
const CARD_QUEUE_FUNCTIONS = [
  'stopHoverVideo', 'prepareSkinVideo', 'loadSkinVideo', 'resetVideo',
  'clearCardImageWatchdog', 'reportCardImageStall', 'loadImageSources',
  'ensureCardImageObserver', 'withdrawCardImageJob', 'deferCardImageSources',
  'enqueueCardImageJob', 'pumpCardImageQueue', 'finishCardImageJob', 'cancelDeferredImages',
];
const cardQueueFunctions = (source = appSource) => CARD_QUEUE_FUNCTIONS.map((name) => functionSource(source, name)).join('\n');

// 第二层补发合成 error 的那一行；变异时整段抹掉。
const SYNTHETIC_ERROR = 'try { img.dispatchEvent(new Event("error")); } catch (_) {}';
// 第一层拿到名额时把懒加载关掉的那一行；变异时整行抹掉。
const EAGER_ADMISSION = 'job.image.loading = "eager";';
// 看门狗时长必须跟着生产常量走：harness 自己写死一个默认值，就会在一个和生产
// 不同的时长上通过——25s 那版能溜过测试正是因为这个漂移。
const PRODUCTION_STALL_MS = Number(String(appSource.match(/CARD_IMAGE_STALL_MS\s*=\s*([\d_]+)/)?.[1] || '').replace(/_/g, ''));
assert.ok(PRODUCTION_STALL_MS > 0, 'app.js 里必须能读出 CARD_IMAGE_STALL_MS');

// ---------- 复现用的窗口：假时钟 + 假 IntersectionObserver + 真实两层队列 ----------
function cardWindow({ queueSrc = queueSource, appSrc = appSource, stallMs = PRODUCTION_STALL_MS, viewportHeight = 300, cardHeight = 100, cardCount = 8 } = {}) {
  const dom = new JSDOM('<div id="app-scroll"><div id="card-host"></div><div id="pool-host"></div></div>', { url: 'http://fixture/', runScripts: 'outside-only' });
  const w = dom.window;
  const reports = [];
  w.reportFlowDiagnostic = (event, reason, fields) => reports.push({ event, reason, fields });

  // 假时钟：第二层超时 10s、重试冷却 10s、第一层看门狗 25s，都不能真等。
  const timers = new Map();
  let timerID = 0;
  let now = 1700000000000;
  w.Date.now = () => now;
  w.setTimeout = (fn, delay) => { const id = ++timerID; timers.set(id, { fn, at: now + (Number(delay) || 0) }); return id; };
  w.clearTimeout = (id) => { timers.delete(id); };
  w.requestAnimationFrame = (cb) => w.setTimeout(() => cb(now), 0);

  // jsdom 没有实现 HTMLImageElement.loading（属性读出来是 undefined），而第二层的
  // 懒加载闸门正是靠它判断。这里按浏览器语义补一个反映 content attribute 的实现，
  // 否则 P1-B 的前提在 jsdom 里根本不成立。
  Object.defineProperty(w.HTMLImageElement.prototype, 'loading', {
    configurable: true,
    get() { const value = this.getAttribute('loading'); return value === 'lazy' || value === 'eager' ? value : ''; },
    set(value) { this.setAttribute('loading', String(value)); },
  });
  // 只关心 src 的移除与恢复，不要 jsdom 的「未实现」噪音。
  w.HTMLMediaElement.prototype.load = function load() {};
  w.HTMLMediaElement.prototype.play = function play() { return Promise.resolve(); };
  w.HTMLMediaElement.prototype.pause = function pause() {};

  // 滚动容器与卡片的几何模型。layout 上的 top/height 直接挂在被观察的 <img> 上。
  const layout = { scrollTop: 0, height: viewportHeight, cardHeight, cardCount };
  const observers = [];
  const intersects = (observer, target) => {
    const top = Number(target.dataset.layoutTop);
    const bottom = top + Number(target.dataset.layoutHeight);
    return bottom > layout.scrollTop - observer.margin && top < layout.scrollTop + layout.height + observer.margin;
  };
  const deliver = (observer, candidates) => {
    const entries = [];
    for (const target of candidates) {
      if (!observer.targets.has(target)) continue;
      const isIntersecting = intersects(observer, target);
      if (observer.known.get(target) === isIntersecting) continue;
      observer.known.set(target, isIntersecting);
      entries.push({ target, isIntersecting, intersectionRatio: isIntersecting ? 1 : 0 });
    }
    if (entries.length) observer.callback(entries, observer);
  };
  class FakeIntersectionObserver {
    constructor(callback, options = {}) {
      this.callback = callback;
      this.options = options;
      // 第一层：root=appScroll + 620px 预取，按声明的 rootMargin 外扩。
      // 第二层：root 是视口，会被 appScroll 这个滚动容器裁掉，工单实测「实际上
      // 要真正可见」，所以有效外扩按 0 算——这正是两层判据不一致的来源。
      const declared = Number(/(-?[\d.]+)px/.exec(String(options.rootMargin || ''))?.[1] || 0);
      this.margin = options.root ? declared : 0;
      this.targets = new Set();
      this.known = new Map();
      observers.push(this);
    }
    observe(target) { this.targets.add(target); deliver(this, [target]); }
    unobserve(target) { this.targets.delete(target); this.known.delete(target); }
    disconnect() { this.targets.clear(); this.known.clear(); }
    takeRecords() { return []; }
  }
  w.IntersectionObserver = FakeIntersectionObserver;
  const refreshVisibility = () => { for (const observer of observers) deliver(observer, [...observer.targets]); };

  // 永不返回的图：第二层给它们设了 src 也不会收到 load/error，只能靠超时放弃。
  const shouldLoad = (url) => !String(url).includes('hung');

  w.eval(queueSrc);
  w.eval(`(() => {
    "use strict";
    const CARD_IMAGE_STALL_MS = ${stallMs};
    const CARD_IMAGE_STALL_REPORT_INTERVAL_MS = 10000;
    const state = {
      cardImageObserver: null,
      cardImageJobs: new WeakMap(),
      cardImageQueue: [],
      activeCardImages: 0,
      activePrestigeCardImages: 0,
      cardImageWatchdogs: new WeakMap(),
      lastCardImageStallReportAt: 0,
      hoverVideo: null,
    };
    const el = { appScroll: document.getElementById('app-scroll') };
    ${cardQueueFunctions(appSrc)}
    window.__cards = { state, el, deferCardImageSources, cancelDeferredImages, pumpCardImageQueue, loadImageSources, prepareSkinVideo, stopHoverVideo };
  })();`);

  const host = w.document.getElementById('card-host');
  const poolHost = w.document.getElementById('pool-host');
  const cards = [];
  const addCard = (sources, { index = cards.length, top = null, hung = false, into = host, withVideo = '' } = {}) => {
    const card = w.document.createElement('button');
    card.className = 'skin-card';
    const video = w.document.createElement('video');
    const image = w.document.createElement('img');
    // 生产模板上就是 loading="lazy"，第二层的懒加载闸门靠它生效。
    image.setAttribute('loading', 'lazy');
    const fallback = w.document.createElement('span');
    fallback.className = 'image-fallback';
    fallback.textContent = '加载中';
    card.append(video, image, fallback);
    into.append(card);
    image.dataset.layoutTop = String(top === null ? index * layout.cardHeight : top);
    image.dataset.layoutHeight = String(layout.cardHeight);
    const entry = { card, video, image, fallback, sources, hung, index };
    cards.push(entry);
    refreshVisibility();
    w.__cards.deferCardImageSources(image, fallback, sources, false);
    if (withVideo) w.__cards.prepareSkinVideo(video, image, fallback, withVideo, card);
    return entry;
  };

  // 一轮「本机毫秒级就能取到」的正常图：只要第二层放了 src 就立刻 load。
  const autoLoad = () => {
    for (const image of w.document.querySelectorAll('img[src]')) {
      if (image.dataset.imageReady === 'true') continue;
      if (!shouldLoad(image.getAttribute('src'))) continue;
      image.dispatchEvent(new w.Event('load'));
    }
  };

  const runTimer = (entry) => { now = entry.at; timers.delete([...timers.entries()].find(([, value]) => value === entry)[0]); entry.fn(); };
  // 一次「网络往返 + 两层队列交接」：第二层放行 src 靠 MutationObserver，第一层
  // 释放名额靠 load 事件，两者都是异步的，所以要多轮才能把一批图彻底跑完。
  const settle = async (rounds = 10) => {
    for (let round = 0; round < rounds; round += 1) { autoLoad(); await flush(); }
  };
  const advance = async (ms) => {
    const target = now + ms;
    for (;;) {
      await settle();
      const due = [...timers.entries()].filter(([, timer]) => timer.at <= target).sort((a, b) => a[1].at - b[1].at)[0];
      if (!due) break;
      runTimer(due[1]);
    }
    now = target;
    await settle();
  };
  const scrollTo = async (top) => { layout.scrollTop = top; refreshVisibility(); await settle(); };

  const loaded = () => cards.filter((entry) => entry.image.dataset.imageReady === 'true');
  const loadingPlaceholder = () => cards.filter((entry) => entry.fallback.textContent === '加载中' && !entry.fallback.hidden);
  const started = () => cards.filter((entry) => entry.image.hasAttribute('src') || entry.image.dataset.imageReady === 'true');
  const dispose = () => { w.dispatchEvent(new w.Event('deep-legends:dispose')); w.close(); };

  return {
    w, reports, cards, layout, host, poolHost, addCard, advance, scrollTo, autoLoad, refreshVisibility,
    state: () => w.__cards.state,
    api: () => w.__cards,
    activeCardImages: () => w.__cards.state.activeCardImages,
    queued: () => w.__cards.state.cardImageQueue.length,
    loaded, loadingPlaceholder, started, dispose,
    now: () => now,
  };
}

const skinSources = (index, kind = 'ok') => [`/api/image?path=%2F${kind}-${index}.png`];
const hungSources = (index) => [`/api/image?path=%2Fhung-${index}.png`];

// =====================================================================
// P1-A　第二层彻底放弃时必须通知第一层，否则名额永久泄漏
// =====================================================================

test('R130 P1 前提：移除 src 不触发任何 load/error 事件', async () => {
  const dom = new JSDOM('<body></body>', { url: 'http://fixture/', runScripts: 'outside-only' });
  const w = dom.window;
  try {
    const image = w.document.createElement('img');
    w.document.body.append(image);
    const events = [];
    image.addEventListener('load', () => events.push('load'));
    image.addEventListener('error', () => events.push('error'));
    image.setAttribute('src', '/api/image?path=%2Fhung.png');
    image.removeAttribute('src');
    await flush();
    await flush();
    // 工单 §1-A 的 Chromium 实测结论在这里同样成立：挂起的请求被 removeAttribute
    // 掐掉之后不会派发任何事件，所以第一层的 onerror 永远等不到。
    assert.deepEqual(events, []);
  } finally { w.close(); }
});

async function leakScenario({ queueSrc = queueSource, appSrc = appSource, stallMs = PRODUCTION_STALL_MS, seconds = 100 } = {}) {
  const h = cardWindow({ queueSrc, appSrc, stallMs, viewportHeight: 5000 });
  try {
    const hung = Array.from({ length: 8 }, (_, index) => h.addCard(hungSources(index), { hung: true }));
    await h.advance(1000);
    const atOneSecond = { active: h.activeCardImages(), queued: h.queued(), loaded: h.loaded().length };
    await h.advance(seconds * 1000);
    const settled = {
      active: h.activeCardImages(),
      queued: h.queued(),
      empty: hung.filter((entry) => entry.fallback.textContent === '暂无预览').length,
      stillLoading: hung.filter((entry) => entry.fallback.textContent === '加载中' && !entry.fallback.hidden).length,
    };
    // 名额归零以后，新加入的正常图必须能全部加载。
    const fresh = Array.from({ length: 6 }, (_, index) => h.addCard(skinSources(8 + index), { index: 8 + index }));
    await h.advance(5000);
    return {
      atOneSecond,
      settled,
      freshLoaded: fresh.filter((entry) => entry.image.dataset.imageReady === 'true').length,
      freshTotal: fresh.length,
      reports: h.reports.filter((report) => report.event === 'card_image_stalled'),
    };
  } finally { h.dispose(); }
}

test('R130 P1-A 8 张永不返回的图放弃后名额归零，后续正常图全部加载', async () => {
  const result = await leakScenario();
  assert.equal(result.atOneSecond.active, 8, '8 个卡片名额应当被挂起的图占满');
  assert.equal(result.atOneSecond.loaded, 0);
  assert.equal(result.settled.active, 0, '第二层彻底放弃后第一层名额必须归零');
  assert.equal(result.settled.empty, 8, '8 张图都必须落到「暂无预览」，不能停在「加载中」');
  assert.equal(result.settled.stillLoading, 0);
  assert.equal(result.freshLoaded, result.freshTotal, '名额释放后新加入的正常图必须全部加载');
});

test('R130 P1-A 挂起的第一个候选放弃后必须继续试下一个候选地址', async () => {
  const h = cardWindow({ viewportHeight: 5000 });
  try {
    const entries = Array.from({ length: 8 }, (_, index) => h.addCard([`/api/image?path=%2Fhung-${index}.png`, `/api/image?path=%2Fok-${index}.png`]));
    await h.advance(130000);
    // 没有这道修复时，第一个候选超时放弃后 onerror 不会执行，第二个候选
    // （本来能显示的图）永远不会去试，卡片一直停在「加载中」。
    assert.equal(h.activeCardImages(), 0, '候选全部试完后名额必须归零');
    // dataset.imageReady 是第二层写的：第一层 complete() 之后它照样会被置上，所以
    // 光看它测不出「候选被跳过」。必须看第一层自己认的 is-loaded 与占位状态。
    assert.equal(entries.filter((entry) => entry.image.classList.contains('is-loaded')).length, 8, '8 张卡都必须真的回退到能显示的候选地址');
    assert.equal(entries.filter((entry) => entry.fallback.hidden).length, 8, '出图之后占位必须隐藏');
    assert.equal(entries.filter((entry) => entry.fallback.textContent === '暂无预览').length, 0, '第二个候选明明能显示，不该落到「暂无预览」');
    assert.equal(entries.filter((entry) => entry.fallback.textContent === '加载中' && !entry.fallback.hidden).length, 0);
  } finally { h.dispose(); }
});

test('R130 P1-A 变异：两道防线都关掉时名额永久泄漏（场景必须能测到修复）', async () => {
  assert.ok(queueSource.includes(SYNTHETIC_ERROR), '生产源码里必须有第二层放弃时补发的合成 error');
  const mutatedQueue = queueSource.replace(SYNTHETIC_ERROR, '');
  const result = await leakScenario({ queueSrc: mutatedQueue, stallMs: Number.POSITIVE_INFINITY });
  assert.equal(result.settled.active, 8, '两道防线都关掉后名额仍然是 8，说明这个场景确实测得到泄漏');
  assert.equal(result.settled.empty, 0);
  assert.equal(result.settled.stillLoading, 8, '整屏永久停在「加载中」——这就是用户看到的截图');
  assert.equal(result.freshLoaded, 0, '名额被占满后新卡片永远排不到');
});

test('R130 P1-A 变异：只留看门狗（第二层不补发 error）仍然不泄漏', async () => {
  const mutatedQueue = queueSource.replace(SYNTHETIC_ERROR, '');
  assert.ok(mutatedQueue !== queueSource, '变异必须真的去掉了合成 error');
  const result = await leakScenario({ queueSrc: mutatedQueue });
  assert.equal(result.settled.active, 0, '看门狗单独也必须能兜住名额');
  assert.equal(result.settled.empty, 8);
  assert.equal(result.freshLoaded, result.freshTotal);
  assert.ok(result.reports.length >= 1, '看门狗判定超时必须上报 card_image_stalled');
});

test('R130 P1-A 变异：只留合成 error（关掉看门狗）仍然不泄漏', async () => {
  const result = await leakScenario({ stallMs: Number.POSITIVE_INFINITY });
  assert.equal(result.settled.active, 0, '第二层补发 error 单独也必须能释放名额');
  assert.equal(result.settled.empty, 8);
  assert.equal(result.freshLoaded, result.freshTotal);
  assert.equal(result.reports.length, 0, '看门狗关掉后不该有 stall 上报');
});

// =====================================================================
// P1-B　可见区域只在第一层判一次
// =====================================================================

async function jumpToBottomScenario({ appSrc = appSource } = {}) {
  const h = cardWindow({ appSrc, cardCount: 40, viewportHeight: 300, cardHeight: 100 });
  try {
    const entries = Array.from({ length: 40 }, (_, index) => h.addCard(skinSources(index), { index }));
    await h.advance(200);
    const atTop = { active: h.activeCardImages(), loaded: h.loaded().length };
    // 直接跳到底部（拖滚动条、恢复滚动位置、刷新后重建列表都会走到这里）。
    await h.scrollTo(40 * 100 - 300);
    const onScreen = entries.filter((entry) => entry.index >= 37);
    let elapsed = 0;
    while (elapsed < 4000) {
      await h.advance(100);
      elapsed += 100;
      if (onScreen.every((entry) => entry.image.hasAttribute('src') || entry.image.dataset.imageReady === 'true')) break;
    }
    return {
      atTop,
      elapsedMs: elapsed,
      onScreenStarted: onScreen.filter((entry) => entry.image.hasAttribute('src') || entry.image.dataset.imageReady === 'true').length,
      onScreenTotal: onScreen.length,
      onScreenLoaded: onScreen.filter((entry) => entry.image.dataset.imageReady === 'true').length,
      eagerOnAdmission: entries.filter((entry) => entry.image.getAttribute('loading') === 'eager').length,
    };
  } finally { h.dispose(); }
}

test('R130 P1-B 跳到列表底部后屏幕上的卡片必须在 2 秒内开始加载', async () => {
  const result = await jumpToBottomScenario();
  assert.ok(result.atTop.loaded >= 3, '停在顶部时屏幕上的卡片应当已经出图');
  assert.equal(result.onScreenStarted, result.onScreenTotal, '屏幕上的卡片必须全部开始加载');
  assert.equal(result.onScreenLoaded, result.onScreenTotal, '屏幕上的卡片必须全部出图');
  assert.ok(result.elapsedMs <= 2000, `屏幕上的卡片用了 ${result.elapsedMs}ms 才开始加载，超过 2 秒`);
  assert.ok(result.eagerOnAdmission > 0, '拿到名额的卡片必须被设成 eager，可见区域只在第一层判一次');
});

test('R130 P1-B 变异：去掉 eager 设置后屏幕上的卡片永远排不到', async () => {
  assert.ok(appSource.includes(EAGER_ADMISSION), '生产源码里必须有第一层的 eager 设置');
  const result = await jumpToBottomScenario({ appSrc: appSource.replace(EAGER_ADMISSION, '') });
  assert.ok(result.elapsedMs > 2000, '去掉 eager 后必须超过 2 秒，否则这个场景测不到修复');
  assert.equal(result.onScreenLoaded, 0, '去掉 eager 后屏幕上的卡片一张都加载不出来');
});

test('R130 P1-B 已排队但未拿到名额的任务离开预取范围会被撤回', async () => {
  const h = cardWindow({ cardCount: 40, viewportHeight: 300, cardHeight: 100 });
  try {
    const entries = Array.from({ length: 40 }, (_, index) => h.addCard(skinSources(index), { index }));
    // 停在顶部：620px 预取范围里是卡片 0..9；第一层 8 个名额被 0..7 占住，8、9 排队。
    assert.equal(h.activeCardImages(), 8, '第一层应当已经发满 8 个名额');
    const queuedJobs = [...h.state().cardImageQueue];
    assert.deepEqual(queuedJobs.map((job) => job.image.dataset.layoutTop), ['800', '900'], '排队里的应当是预取范围末尾那两张');
    await h.scrollTo(40 * 100 - 300);
    // 跳到底部后 8、9 离开预取范围：必须撤回并重新观察，不能继续排在队里白占位置。
    // 撤回不是取消——滚回来还要能继续加载。
    for (const job of queuedJobs) {
      assert.equal(job.cancelled, false, '撤回不是取消');
      assert.equal(h.state().cardImageQueue.includes(job), false, '撤回后不该再留在第一层队列里');
      assert.equal(job.image.dataset.cardImage, 'pending', '撤回后状态必须回到 pending');
    }
    assert.ok(h.state().cardImageQueue.every((job) => Number(job.image.dataset.layoutTop) >= 3000), '第一层队列里不该再留着屏幕外的任务');
    await h.advance(200);
    assert.ok(entries.some((entry) => entry.index >= 37 && entry.image.dataset.imageReady === 'true'), '屏幕上的卡片必须真的开始出图');
  } finally { h.dispose(); }
});

// =====================================================================
// P1-C　悬停视频必须释放连接，全局同时只播放一个
// =====================================================================

test('R130 P1-C pointerleave 之后视频 src 被移除，且全局只有 1 个视频在播放', async () => {
  const h = cardWindow({ cardCount: 6 });
  try {
    const entries = Array.from({ length: 3 }, (_, index) => h.addCard(skinSources(index), { index, withVideo: `/videos/${index}.mp4` }));
    const playing = () => entries.filter((entry) => entry.video.hasAttribute('src')).map((entry) => entry.video);
    // 先让原画走完两层队列：第二层放行时会执行 img.hidden = false，必须让它发生在
    // 悬停播放之前，否则测的是队列而不是悬停逻辑。
    await h.advance(100);
    for (const entry of entries) {
      entry.card.dispatchEvent(new h.w.Event('pointerenter'));
      entry.video.dispatchEvent(new h.w.Event('canplay'));
      await flush();
      // 连续划过 3 张卡：开始下一个之前必须先停掉上一个。
      assert.equal(playing().length, 1, `悬停 ${entry.index} 时同时播放的视频数必须是 1`);
      assert.equal(playing()[0], entry.video);
      assert.equal(entry.image.hidden, true, '播放时原画应当让位给视频');
    }
    const last = entries[2];
    last.card.dispatchEvent(new h.w.Event('pointerleave'));
    await flush();
    // 以前全文件没有 pointerleave：鼠标移开后循环视频继续占着连接，要等列表
    // 重建才会停。resetVideo 必须移除 src 并 load()，这才会真正中断传输。
    assert.equal(last.video.hasAttribute('src'), false, 'pointerleave 之后必须移除视频 src');
    assert.equal(last.video.hidden, true);
    assert.equal(playing().length, 0);
    assert.equal(last.image.hidden, false, '视频停掉后原画必须重新露出');
    // blur 同样要停。
    entries[0].card.dispatchEvent(new h.w.Event('pointerenter'));
    entries[0].video.dispatchEvent(new h.w.Event('canplay'));
    await flush();
    assert.equal(playing().length, 1);
    entries[0].card.dispatchEvent(new h.w.Event('blur'));
    await flush();
    assert.equal(playing().length, 0, 'blur 之后也必须停掉悬停视频');
  } finally { h.dispose(); }
});

test('R130 P1-C 生产源码绑定了 pointerleave/blur，且开始下一个前先停上一个', () => {
  const prepare = functionSource(appSource, 'prepareSkinVideo');
  assert.match(prepare, /addEventListener\("pointerleave", stop/, '必须绑定 pointerleave');
  assert.match(prepare, /addEventListener\("blur", stop/, '必须绑定 blur');
  assert.match(prepare, /stopHoverVideo\(\);[\s\S]*?loadSkinVideo/, '开始下一个悬停视频前必须先停掉上一个');
  assert.doesNotMatch(prepare, /\{ once: true/, '悬停视频不能只允许播放一次，停掉之后要能重新播放');
  assert.match(functionSource(appSource, 'stopHoverVideo'), /resetVideo\(current\.video\)/, '必须用 resetVideo 释放连接，单纯 pause 不够');
});

// =====================================================================
// P1-D　离开奖池页时必须取消奖池网格的卡片任务
// =====================================================================

test('R130 P1-D 取消奖池网格后 activeCardImages 只统计皮肤列表的任务', async () => {
  const h = cardWindow();
  try {
    // 皮肤列表里屏幕上的卡片：两层都放行，正常出图并释放名额。
    const skinCards = Array.from({ length: 3 }, (_, index) => h.addCard(skinSources(index), { index }));
    // 奖池面板隐藏后里面的卡片仍然占着全局名额：第一层按 620px 预取范围发了名额，
    // 图片却再也不会完成（面板不可见、请求挂住），名额要一直占到看门狗超时为止。
    // 这正是工单 §1-D 的场景——切页时必须主动取消，不能等它自己超时。
    const poolCards = Array.from({ length: 5 }, (_, index) => h.addCard([`/api/image?path=%2Fhung-pool-${index}.png`], { index, top: 400 + index * 100, into: h.poolHost }));
    await h.advance(300);
    assert.equal(skinCards.filter((entry) => entry.image.dataset.imageReady === 'true').length, 3, '屏幕上的皮肤卡片应当正常出图');
    assert.equal(h.activeCardImages(), 5, '不可见的奖池卡片应当占着 5 个名额不放');
    const before = h.activeCardImages();
    h.api().cancelDeferredImages(h.poolHost);
    await flush();
    const after = h.activeCardImages();
    assert.ok(after < before, `取消奖池网格必须释放名额：${before} -> ${after}`);
    assert.equal(after, 0, '奖池名额必须全部释放，只剩皮肤列表的计数');
    const stillTracked = h.state().cardImageJobs;
    for (const entry of poolCards) assert.equal(stillTracked.has(entry.image), false, '奖池任务必须已经从登记表里移除');
    for (const entry of skinCards) {
      if (entry.image.dataset.cardImage === undefined) continue;
      assert.ok(['pending', 'queued', 'loading'].includes(entry.image.dataset.cardImage), '皮肤列表的任务不受影响');
    }
    assert.equal(h.state().cardImageQueue.some((job) => job.cancelled), false, '队列里不该留着已取消的任务');
  } finally { h.dispose(); }
});

test('R130 P1-D 切换收藏子页与一级页签时都取消奖池网格', () => {
  const favorites = functionSource(appSource, 'activateFavoritesPage');
  assert.match(favorites, /cancelDeferredImages\(el\.grid\);\s*\n(?:\s*\/\/[^\n]*\n)*\s*cancelDeferredImages\(el\.poolSkinGrid\);/, '切换收藏子页必须同时取消奖池网格');
  const section = functionSource(appSource, 'activateSection');
  assert.match(section, /cancelDeferredImages\(el\.poolSkinGrid\);/, '离开收藏页必须同时取消奖池网格');
  // 工单 §2-5：deferCardImageSources 的每个调用点，容器被替换或隐藏时都要取消。
  const callSites = [...appSource.matchAll(/deferCardImageSources\(/g)].length;
  assert.equal(callSites, 4, '调用点数量变了就要重新核对取消逻辑（1 处定义 + 3 处调用）');
  assert.match(functionSource(appSource, 'renderItems'), /cancelDeferredImages\(el\.grid\);/, '皮肤列表重建前必须取消');
  assert.match(functionSource(appSource, 'renderPoolCatalog'), /cancelDeferredImages\(el\.poolSkinGrid\);/, '奖池列表重建前必须取消');
});

// =====================================================================
// P1-6　card_image_stalled 诊断
// =====================================================================

test('R130 P1-6 看门狗上报 card_image_stalled，每 10 秒最多一条且不带路径', async () => {
  // card_image_stalled 是看门狗的专属信号：第二层正常补发 error 时看门狗根本不会醒。
  // 所以要测上报，必须先把第二层那道防线关掉，让看门狗成为唯一的推进者。
  const h = cardWindow({ queueSrc: queueSource.replace(SYNTHETIC_ERROR, ''), viewportHeight: 5000 });
  try {
    Array.from({ length: 8 }, (_, index) => h.addCard(hungSources(index), { hung: true }));
    await h.advance(50000);
    const stalls = h.reports.filter((report) => report.event === 'card_image_stalled');
    assert.ok(stalls.length >= 1, '看门狗判定超时必须上报');
    // 8 张图在同一个 10 秒窗口里一起超时，只允许一条。
    assert.equal(stalls.length, 1, `10 秒窗口里上报了 ${stalls.length} 条`);
    const [first] = stalls;
    assert.equal(first.reason, 'watchdog');
    assert.equal(typeof first.fields.activeCardImages, 'number');
    assert.equal(typeof first.fields.queued, 'number');
    assert.equal(typeof first.fields.sourceIndex, 'number');
    assert.ok(['lcu', 'communitydragon', 'ddragon', 'gtimg'].includes(first.fields.imageSource), `imageSource 必须是来源类别枚举：${first.fields.imageSource}`);
    assert.equal(JSON.stringify(first.fields).includes('hung'), false, '上报里不得带任何资源路径');
    // 第一批已经全部收尾，不会再有看门狗触发；再放一批挂起的图，确认限速是
    // 「每 10 秒最多一条」而不是「永久只报一条」。
    Array.from({ length: 8 }, (_, index) => h.addCard(hungSources(50 + index), { index: 8 + index, hung: true }));
    await h.advance(55000);
    const later = h.reports.filter((report) => report.event === 'card_image_stalled');
    assert.ok(later.length >= 2, `超过 10 秒窗口后必须允许再报一条，实际 ${later.length} 条`);
  } finally { h.dispose(); }
});

test('R130 P1-6 card_image_stalled 打通前端白名单与后端白名单', () => {
  // 前端：runtime.js 的事件白名单不放行就会在浏览器里被直接丢掉，永远到不了后端。
  const allowList = /if \(!\[([^\]]*)\]\.includes\(event\)\) return;/.exec(runtimeSource);
  assert.ok(allowList, 'runtime.js 的事件白名单写法变了，请同步这条断言');
  assert.ok(allowList[1].includes('"card_image_stalled"'), 'runtime.js 必须放行 card_image_stalled');
  assert.match(runtimeSource, /if \(event === "card_image_stalled"\) \{[\s\S]*?activeCardImages[\s\S]*?imageSource/, 'runtime.js 必须透传 stall 字段');
  // 前端上报点与来源分类只有一份实现。
  assert.match(functionSource(appSource, 'reportCardImageStall'), /window\.deepLegendsImageSourceLabel\?\.\(url\)/, '来源分类必须复用 image-queue.js 的那一份');
  assert.match(queueSource, /window\.deepLegendsImageSourceLabel = imageSourceLabel;/, 'image-queue.js 必须导出来源分类');
  // 后端：不在 clientDiagnosticEvents 里就会被 400 拒收并记一条 client_diagnostic_rejected。
  assert.match(featuresSource, /"card_image_stalled":\s*\{"watchdog": true\},/, 'features.go 白名单必须收下 card_image_stalled/watchdog');
  assert.match(featuresSource, /if request\.Event == "card_image_stalled" \{[\s\S]*?active_card_images[\s\S]*?source_index/, 'features.go 必须把字段落到日志');
  for (const field of ['activeCardImages', 'queued', 'sourceIndex']) {
    assert.match(featuresSource, new RegExp(`json:"${field},omitempty"`), `clientDiagnosticRequest 必须声明 ${field}，否则 DisallowUnknownFields 会整条拒收`);
  }
});

// =====================================================================
// P2–P6　头像/旗帜页面整理
// =====================================================================

// favorites-facade.js 的渲染辅助函数：原样抽出来，配一份最小 DOM 与 state 跑真实实现。
const FACADE_FUNCTIONS = [
  'formatCount', 'facadeCollectionIconRows', 'facadeCollectionBannerRows',
  'facadeCollectionMetaParts', 'facadeCollectionMetaText', 'renderFacadeMeta',
  'ownershipUnavailable', 'syncSetOptions', 'syncControls',
  'sampleBannerRatio', 'resetBannerRatioSample', 'setDetailArtSize', 'applyDetailArtSize',
];

function facadeWindow(src = facadeSource) {
  const dom = new JSDOM('<body></body>', { url: 'http://fixture/', runScripts: 'outside-only' });
  const w = dom.window;
  const detailConstants = ['DETAIL_ART_PLACEHOLDER_PX', 'DETAIL_ART_ICON_SCALE', 'DETAIL_ART_ICON_MAX_PX', 'DETAIL_ART_BANNER_MAX_HEIGHT_PX'].map((name) => {
    const declaration = src.match(new RegExp(`const ${name} = \\d+;`))?.[0];
    assert.ok(declaration, `${name} 必须从生产源码读取，避免测试常量与实现漂移`);
    return declaration;
  }).join('\n');
  w.eval(`(() => {
    "use strict";
    const el = {
      grid: document.createElement('div'),
      meta: document.createElement('div'),
      dialog: document.createElement('dialog'),
      dialogImage: document.createElement('img'),
      setControl: document.createElement('div'),
      unownedControl: document.createElement('label'),
      showUnowned: (() => { const input = document.createElement('input'); input.type = 'checkbox'; return input; })(),
      groups: document.createElement('div'),
      sort: document.createElement('select'),
      direction: document.createElement('button'),
      set: document.createElement('select'),
      tabs: [],
    };
    const GROUPS = { icons: [['all', '全部头像'], ['recent', '近三年新增']], banners: [['all', '全部'], ['tencent', '国服专属']] };
    const SORTS = { icons: [['new', '最新在前'], ['old', '最早在前'], ['name', '按名称'], ['unowned', '未拥有优先']], banners: [['id', '按编号排序'], ['name', '按名称'], ['unowned', '未拥有优先']] };
    const state = {
      view: 'icons',
      icons: null,
      banners: null,
      filters: {
        icons: { query: '', group: 'all', set: '', sort: 'new', descending: false, showUnowned: false },
        banners: { query: '', group: 'all', sort: 'id', descending: false, showUnowned: true },
      },
    };
    let bannerRatioSampled = false;
    ${detailConstants}
    ${FACADE_FUNCTIONS.map((name) => functionSource(src, name)).join('\n')}
    window.__facade = {
      el, state, syncControls, sampleBannerRatio, resetBannerRatioSample, applyDetailArtSize, setDetailArtSize,
      renderFacadeMeta, facadeCollectionMetaText, facadeCollectionIconRows, ownershipUnavailable,
      get bannerRatioSampled() { return bannerRatioSampled; },
    };
  })();`);
  return { w, api: w.__facade, close: () => w.close() };
}

const natural = (w, width, height) => {
  const image = w.document.createElement('img');
  Object.defineProperty(image, 'naturalWidth', { value: width, configurable: true });
  Object.defineProperty(image, 'naturalHeight', { value: height, configurable: true });
  return image;
};

test('R130 P2 旗帜改成 150px 竖长卡，名称移到图片下方', () => {
  assert.match(css, /\.facade-grid\.is-banners \{[^}]*grid-template-columns: repeat\(auto-fill, 150px\)/, '旗帜格子必须固定 150px 宽，宽窗口下尺寸不变');
  assert.match(css, /\.facade-grid\.is-banners \{[^}]*justify-content: start/, '固定宽度的格子必须靠左排，不能被拉开');
  assert.match(css, /\.facade-grid\.is-banners \{[^}]*--facade-banner-ratio: 0\.3/, '取样前必须有 0.3 的默认比例兜底');
  assert.match(css, /\.facade-grid\.is-banners \.skin-art \{ aspect-ratio: var\(--facade-banner-ratio\);/, '格子高度必须跟着真实比例走，不能写死正方形');
  assert.match(css, /\.facade-grid\.is-banners \.skin-art img \{ object-fit: contain; \}/, '旗帜不得裁剪');
  assert.doesNotMatch(css, /\.facade-grid\.is-banners \{[^}]*--card-min/, '旗帜不再用 --card-min 的自适应格子');
  // 名称等文字移到图片下方（和 R125 头像格子一样），不再压在旗帜上。
  assert.match(css, /\.facade-grid \.skin-copy \{ position: static;[^}]*background: none; \}/, '文字必须脱离原画叠层');
  assert.match(css, /\.facade-grid \.skin-hero, \.facade-grid \.skin-meta \{ display: none; \}/, '旗帜格子只保留名称一行');
  assert.match(facadeSource, /if \(state\.view === "banners"\) card\.title = fields\.title;/, '完整名称放 title 供悬停查看');
  // R125 的头像格子不受影响。
  assert.match(css, /\.facade-grid\.is-icons \{[^}]*repeat\(auto-fill, 92px\)/);
});

test('R130 P2 旗帜比例取自第一张图片的真实尺寸，铺满格子后上下留白 ≤ 5%', () => {
  const h = facadeWindow();
  try {
    const { api, w } = h;
    api.state.view = 'banners';
    api.sampleBannerRatio(natural(w, 102, 400), true);
    const ratio = api.el.grid.style.getPropertyValue('--facade-banner-ratio');
    assert.equal(ratio, (102 / 400).toFixed(4), '必须写 naturalWidth / naturalHeight，不能写反');
    const cellWidth = 150;
    const cellHeight = cellWidth / Number(ratio);
    // object-fit: contain 下图片按同一比例铺满格子；格子比例 == 图片比例时留白为 0。
    const drawnHeight = Math.min(cellHeight, cellWidth / (102 / 400));
    const whitespace = Math.abs(1 - drawnHeight / cellHeight);
    assert.ok(whitespace <= 0.05, `上下留白 ${(whitespace * 100).toFixed(2)}% 超过 5%`);
    // 只用第一张取样：第二张不同比例的旗帜不得再改写变量，否则整个网格反复重排。
    api.sampleBannerRatio(natural(w, 200, 400), true);
    assert.equal(api.el.grid.style.getPropertyValue('--facade-banner-ratio'), ratio, '只取第一张旗帜的比例');
    assert.equal(api.bannerRatioSampled, true);
    // 目录被重新读取（重试按钮、断连重连）之后必须能重新取样，比例不能永久锁死。
    api.resetBannerRatioSample();
    assert.equal(api.bannerRatioSampled, false);
    assert.equal(api.el.grid.style.getPropertyValue('--facade-banner-ratio'), '', '重新取样前必须把网格上的旧值摘掉');
    api.sampleBannerRatio(natural(w, 200, 400), true);
    assert.equal(api.el.grid.style.getPropertyValue('--facade-banner-ratio'), (200 / 400).toFixed(4));
  } finally { h.close(); }
  // 头像视图不得写旗帜比例；图片还没解码出尺寸时也不得写，且要允许下一张继续取样。
  const icons = facadeWindow();
  try {
    icons.api.sampleBannerRatio(natural(icons.w, 102, 400), false);
    assert.equal(icons.api.el.grid.style.getPropertyValue('--facade-banner-ratio'), '', '头像视图不该写旗帜比例');
  } finally { icons.close(); }
  const blank = facadeWindow();
  try {
    blank.api.state.view = 'banners';
    blank.api.sampleBannerRatio(blank.w.document.createElement('img'));
    assert.equal(blank.api.el.grid.style.getPropertyValue('--facade-banner-ratio'), '', '尺寸未知时不得写比例');
    assert.equal(blank.api.bannerRatioSampled, false, '没取到样就必须允许下一张继续取样');
  } finally { blank.close(); }
});

test('R130 P3 头像/旗帜/炫彩去掉右上角状态标签，皮肤卡的奖池标签保留', () => {
  const createCard = functionSource(facadeSource, 'createCard');
  assert.doesNotMatch(createCard, /\.skin-state"\)\.textContent = fields\.state/, '头像/旗帜卡片不得再写状态标签');
  assert.match(createCard, /badge\.textContent = "";\s*\n\s*badge\.hidden = true;/, '状态标签必须清空并隐藏');
  assert.match(css, /\.facade-grid \.skin-state \{ display: none; \}/, '样式上也必须彻底不显示');
  assert.doesNotMatch(css, /\.facade-grid\.is-icons \.skin-state \{/, '头像格子的状态标签样式已经没用了');
  // 未拥有仍然靠置灰 + 锁图标表示。
  assert.match(createCard, /card\.classList\.toggle\("is-locked", fields\.locked\)/);
  assert.match(css, /\.facade-grid \.skin-lock \{ width: 30px; height: 30px; \}/, '旗帜格子也要能显示锁图标');
  // 炫彩卡片的「已拥有 / 未获取」标签同样去掉。
  const chroma = functionSource(appSource, 'makeChromaCard');
  assert.doesNotMatch(chroma, /status\.textContent = chroma\.owned \? "已拥有" : "未获取"/);
  assert.match(chroma, /status\.textContent = "";\s*\n\s*status\.hidden = true;/);
  assert.match(chroma, /card\.classList\.toggle\("is-locked", locked\)/, '炫彩未获取仍然置灰');
  // 皮肤卡在「全部皮肤」视图里的「三合一剩余」表示奖池状态，不是拥有状态，保留。
  const skin = functionSource(appSource, 'makeSkinCard');
  assert.match(skin, /status\.textContent = showPoolState \? "三合一剩余" : "";/, '奖池状态标签必须保留');
  assert.match(skin, /status\.hidden = !showPoolState;/);
  // 详情弹窗里的「拥有状态」一行保留（头像 + 旗帜各一处）。
  const detail = functionSource(facadeSource, 'openDetail');
  assert.equal((detail.match(/\["拥有状态", fields\.state\]/g) || []).length, 2, '头像与旗帜的详情弹窗都要保留「拥有状态」一行');
});

test('R130 P4 头像/旗帜页去掉三格统计条，只留摘要行', () => {
  for (const marker of ['facade-metrics', 'facade-total', 'facade-owned', 'facade-visible', 'metricsBar']) {
    assert.equal(html.includes(marker), false, `index.html 里还留着 ${marker}`);
    assert.equal(facadeSource.includes(marker), false, `favorites-facade.js 里还留着 ${marker}`);
  }
  for (const marker of ['facade-metrics']) {
    assert.equal(css.includes(marker), false, `app.css 里还留着 ${marker}`);
  }
  assert.doesNotMatch(html, /<dt>(目录总数|已拥有|当前显示)<\/dt>/, '三格统计条的标记必须删干净');
  // 摘要行还在，断连时的收起逻辑不引用任何已删除的元素。
  assert.match(html, /<div id="facade-list-meta" class="list-meta" role="status" aria-live="polite"><\/div>/);
  const disconnected = functionSource(facadeSource, 'renderDisconnected');
  assert.match(disconnected, /el\.toolbar\.hidden = true;/);
  assert.match(disconnected, /el\.listRow\.hidden = true;/);
  assert.doesNotMatch(disconnected, /metricsBar|el\.total|el\.owned|el\.visible/, '断连收起逻辑不得引用已删除的元素');
  assert.doesNotMatch(functionSource(facadeSource, 'render'), /metricsBar|el\.total\b|el\.owned\b|el\.visible\b/);
  // 别处的统计条不受影响。
  assert.match(html, /class="metrics/, '其它页面的统计条不该被一起删掉');
});

test('R130 P5 摘要行去掉「已显示」，数字走主题色且不经过 innerHTML', () => {
  const h = facadeWindow();
  try {
    const { api } = h;
    assert.equal(api.facadeCollectionMetaText('头像', 5099, 478, false), '共 5,099 款头像 · 已拥有 478');
    assert.equal(api.facadeCollectionMetaText('头像', 5099, 0, true), '共 5,099 款头像 · 拥有状态未读取');
    api.renderFacadeMeta('头像', 5099, 478, false);
    const numbers = [...api.el.meta.querySelectorAll('b.list-meta-number')];
    assert.deepEqual(numbers.map((node) => node.textContent), ['5,099', '478'], '摘要行里两个数字都要上主题色');
    assert.equal(api.el.meta.textContent, '共 5,099 款头像 · 已拥有 478');
    assert.equal(api.el.meta.getAttribute('aria-label'), '共 5,099 款头像 · 已拥有 478', '拆成多节点后要留一份完整文本给读屏软件');
    // 未转义文本绝不能进 innerHTML：kind 里塞一段 HTML 也必须原样变成文本节点。
    api.renderFacadeMeta('<img src=x onerror=alert(1)>', 1, 1, false);
    assert.equal(api.el.meta.querySelectorAll('img').length, 0, '摘要行不得解析成 DOM');
    assert.ok(api.el.meta.textContent.includes('<img src=x onerror=alert(1)>'), '尖括号必须原样保留为文本');
  } finally { h.close(); }
  assert.match(css, /\.list-meta-number \{ color: var\(--primary\); font-weight: 650; font-variant-numeric: tabular-nums; \}/, '主题色 + 等宽数字');
  // 皮肤摘要与炫彩摘要都必须走同一个 DOM 拼接函数。
  assert.match(appSource, /renderListMeta\(label, " ", visible\.length, " 款"/, '皮肤摘要行必须走 DOM 拼接');
  assert.match(appSource, /renderListMeta\("全部炫彩 ", visibleCount, " 款 · "/, '炫彩摘要行必须走 DOM 拼接');
  const renderListMeta = functionSource(appSource, 'renderListMeta');
  assert.match(renderListMeta, /replaceChildren/, '必须用 DOM 拼接');
  assert.doesNotMatch(renderListMeta, /innerHTML/, '不得用 innerHTML 拼摘要行');
  assert.match(renderListMeta, /className = "list-meta-number"/);
  // 只改摘要行，不改英雄分组的小标题。
  assert.doesNotMatch(functionSource(appSource, 'makeChromaStack'), /list-meta-number/);
  assert.doesNotMatch(functionSource(appSource, 'chromaSummaryAttributes'), /list-meta-number/);
});

test('R130 P5-4 拥有状态从不可用恢复后，头像开关回到用户原来的设置', () => {
  const icons = [
    { id: 1, title: '甲', year: 2026, owned: true, sets: [], searchTerms: [] },
    { id: 2, title: '乙', year: 2026, owned: false, sets: [], searchTerms: [] },
    { id: 3, title: '丙', year: 2026, owned: false, sets: [], searchTerms: [] },
  ];
  const recovery = (src) => {
    const h = facadeWindow(src);
    try {
      const { api } = h;
      api.state.view = 'icons';
      api.state.icons = { total: 3, icons, iconOwnershipUnavailable: true };
      api.syncControls();
      // 不可用期间：开关隐藏，但用户保存的值（默认关闭）不得被改写。
      assert.equal(api.el.unownedControl.hidden, true);
      assert.equal(api.el.showUnowned.checked, false, '开关必须仍然显示用户自己的设置');
      assert.equal(api.state.filters.icons.showUnowned, false, '不得改写用户保存的值');
      // R124：拥有状态不可用时网格不能被滤空。
      assert.equal(api.facadeCollectionIconRows(icons, api.state.filters.icons, 2026, api.ownershipUnavailable()).length, 3, 'R124 的降级行为必须继续成立');
      // 恢复可用。
      api.state.icons = { total: 3, icons, iconOwnershipUnavailable: false };
      api.syncControls();
      assert.equal(api.el.unownedControl.hidden, false);
      assert.equal(api.el.showUnowned.checked, false, '恢复后开关必须回到用户原来的设置（默认关闭）');
      assert.equal(api.state.filters.icons.showUnowned, false);
      // 开关这时才真的生效：只显示已拥有。
      assert.deepEqual(api.facadeCollectionIconRows(icons, api.state.filters.icons, 2026, api.ownershipUnavailable()).map((icon) => icon.id), [1]);
      // 旗帜的默认值不受影响。
      assert.equal(api.state.filters.banners.showUnowned, true);
    } finally { h.close(); }
  };
  recovery(facadeSource);
  // 对抗变异：把 R124 那条「不可用时复位记忆值」的写法塞回去，用户设置就会被
  // 永久改掉，上面两条断言必须失败。
  const mutated = facadeSource.replace(
    'el.showUnowned.checked = filters.showUnowned;',
    'if (unavailable) filters.showUnowned = true;\n    el.showUnowned.checked = filters.showUnowned;',
  );
  assert.ok(mutated !== facadeSource, '变异必须真的改到了 syncControls');
  assert.throws(() => recovery(mutated), /开关必须仍然显示用户自己的设置|恢复后开关必须回到用户原来的设置/, '把复位语句塞回去后必须测得出来');
});

test('R138 头像详情按原图 2 倍显示、最大 512px，旗帜详情保留 R130 尺寸', () => {
  const h = facadeWindow();
  try {
    const { api } = h;
    const image = api.el.dialogImage;
    const size = (width, height) => {
      Object.defineProperty(image, 'naturalWidth', { value: width, configurable: true });
      Object.defineProperty(image, 'naturalHeight', { value: height, configurable: true });
      api.applyDetailArtSize();
      return [api.el.dialog.style.getPropertyValue('--facade-art-width'), api.el.dialog.style.getPropertyValue('--facade-art-height')];
    };
    api.state.view = 'icons';
    // R138：用户明确接受放大后的模糊，覆盖 R130 的头像 1:1 设计。
    assert.deepEqual(size(128, 128), ['256px', '256px'], '128px 头像应放大为 256px');
    assert.deepEqual(size(64, 64), ['128px', '128px'], '更小的头像也按 2 倍放大');
    assert.deepEqual(size(300, 300), ['512px', '512px'], '300px 头像应封顶 512px，而不是 600px');
    assert.deepEqual(size(512, 512), ['512px', '512px'], '原图超过上限时仍封顶 512px');
    api.state.view = 'banners';
    assert.deepEqual(size(100, 400), ['80px', '320px'], '旗帜按自身比例，高度不超过上限');
    assert.deepEqual(size(100, 200), ['100px', '200px'], '高度没到上限时按原尺寸');
  } finally { h.close(); }
  // 还没解码出尺寸时不得改动占位大小，否则弹窗会跳。
  const fresh = facadeWindow();
  try {
    fresh.api.state.view = 'icons';
    fresh.api.setDetailArtSize(128, 128);
    fresh.api.applyDetailArtSize();
    const style = fresh.api.el.dialog.style;
    assert.deepEqual([style.getPropertyValue('--facade-art-width'), style.getPropertyValue('--facade-art-height')], ['128px', '128px']);
  } finally { fresh.close(); }
});

test('R130 P6 弹窗改紧凑布局、加载前 128×128 占位，皮肤/炫彩弹窗不动', () => {
  assert.match(css, /\.facade-detail-dialog \{ width: fit-content;/, '弹窗宽度必须跟着内容收缩，不留大片空白');
  assert.match(css, /\.facade-detail-dialog \.dialog-art \{ width: var\(--facade-art-width, 128px\); max-width: 100%; height: var\(--facade-art-height, 128px\); aspect-ratio: auto;/, '图片区域尺寸由 JS 写进变量，加载前按 128px 占位');
  assert.match(css, /\.facade-detail-dialog \.dialog-art-primary \{ object-fit: contain;/, '不得裁剪');
  assert.doesNotMatch(css, /\.facade-detail-dialog \{ width: min\(560px/, '560px 的固定宽度必须去掉');
  assert.doesNotMatch(css, /\.facade-detail-dialog \.dialog-art \{ aspect-ratio: 1\/1;/, '正方形写死的图片区域必须去掉');
  const openDetail = functionSource(facadeSource, 'openDetail');
  assert.match(openDetail, /setDetailArtSize\(DETAIL_ART_PLACEHOLDER_PX, DETAIL_ART_PLACEHOLDER_PX\);/, '加载前必须先按 128×128 占位');
  assert.match(openDetail, /onload = \(\) => \{[\s\S]*?applyDetailArtSize\(\);/, '图片到位后换成真实尺寸');
  // 关闭按钮挪出图片区域：128px 的头像放不下那个悬浮工具条，而 .dialog-art 带
  // contain: paint，留在里面会被裁掉。
  assert.match(html, /<span id="facade-detail-fallback">加载中<\/span><\/div><div class="dialog-copy"><div class="dialog-toolbar"><button id="facade-detail-close"/, '关闭按钮必须在图片区域之外');
  // 皮肤/炫彩详情弹窗不动。
  assert.match(css, /\.skin-dialog \{ zoom: var\(--ui-zoom, 1\); width: min\(900px,/, '皮肤详情弹窗宽度不变');
  assert.match(css, /\.dialog-art \{ position: relative; width: 100%; min-width: 0; min-height: 0; aspect-ratio: 16\/9;/, '皮肤详情图片区域不变');
  assert.doesNotMatch(css, /\.skin-dialog \.dialog-art \{[^}]*--facade-art/, '皮肤弹窗不得吃头像的尺寸变量');
  assert.match(css, /\.skin-dialog\.is-chroma-dialog \.dialog-art, \.skin-dialog\.is-chroma-dialog\.is-prestige-dialog \.dialog-art \{ height: auto; aspect-ratio: 16\/9; \}/, '炫彩弹窗不变');
});

test('R130 P1-A 取消/重建之后看门狗不得复活已取消的卡片', async () => {
  const h = cardWindow({ viewportHeight: 5000 });
  try {
    const entries = Array.from({ length: 4 }, (_, index) => h.addCard(hungSources(index), { hung: true }));
    await h.advance(1000);
    assert.equal(h.activeCardImages(), 4, '4 张挂起的图应当占满 4 个名额');
    // 生产里的顺序是：先 cancelDeferredImages，紧接着整个容器被 replaceChildren 换掉。
    h.api().cancelDeferredImages(h.host);
    h.host.replaceChildren();
    await flush();
    assert.equal(h.activeCardImages(), 0, '取消后名额必须立刻归零');
    // 看门狗本来会在 45s 后醒来推进候选地址；取消之后它必须已经被清掉，
    // 不能再给一批已经离开 DOM 的图重新写 data-queued-src。
    await h.advance(60000);
    assert.equal(entries.filter((entry) => entry.image.hasAttribute('src')).length, 0, '看门狗不得复活已取消的卡片');
    assert.equal(h.activeCardImages(), 0);
    assert.equal(h.reports.filter((report) => report.event === 'card_image_stalled').length, 0, '已取消的任务不该产生 stall 上报');
  } finally { h.dispose(); }
});

test('R130 P1-A 看门狗只统计卡片任务，详情弹窗不得污染 stall 上报', () => {
  // loadImageSources 同时服务卡片队列与皮肤/炫彩详情弹窗；弹窗图不占第一层名额，
  // 报成 card_image_stalled 会把「名额是否泄漏」这个唯一判据搞脏。
  assert.match(functionSource(appSource, 'loadImageSources'), /if \(state\.cardImageJobs\.has\(image\)\) reportCardImageStall/, '只有登记在册的卡片任务才允许上报 stall');
  // 弹窗关闭后不会再有新的 loadImageSources 来顶掉看门狗，两处拆除点都必须显式清理。
  assert.match(functionSource(appSource, 'resetDialogImage'), /clearCardImageWatchdog\(el\.skinDialogImage\)/, '弹窗重建前必须清掉上一张的看门狗');
  assert.match(functionSource(appSource, 'closeSkinDialog'), /clearCardImageWatchdog\(el\.skinDialogImage\)/, '关闭弹窗必须清掉看门狗');
  // 卡片路径的清理点一个都不能少，否则取消之后看门狗还会醒过来。
  for (const name of ['loadImageSources', 'deferCardImageSources', 'cancelDeferredImages']) {
    assert.match(functionSource(appSource, name), /clearCardImageWatchdog\(image\)/, `${name} 必须清理看门狗`);
  }
});

test('R130 P1-A 变异：看门狗先推进时，第二层迟到的合成 error 不得再推进一次', async () => {
  // 这条钉子专门逼出「第一层已经换到下一个候选、第二层才彻底放弃」的交错，也正是
  // 工单写的 25 秒必须改成 45 秒的原因：第二层从放行到彻底放弃是 10s 超时 + 10s
  // 冷却 + 再 10s 超时 = 30s，看门狗取 25s 就会抢在它前面推进。
  // 这里故意把看门狗调回 25s 复现交错，生产值是 45s。
  const run = async (queueSrc) => {
    const h = cardWindow({ queueSrc, stallMs: 25000, viewportHeight: 5000 });
    try {
      const entries = Array.from({ length: 4 }, (_, index) => h.addCard([`/api/image?path=%2Fhung-${index}.png`, `/api/image?path=%2Fok-${index}.png`]));
      await h.advance(90000);
      return {
        loaded: entries.filter((entry) => entry.image.classList.contains('is-loaded')).length,
        empty: entries.filter((entry) => entry.fallback.textContent === '暂无预览').length,
        active: h.activeCardImages(),
      };
    } finally { h.dispose(); }
  };
  const fixed = await run(queueSource);
  assert.deepEqual(fixed, { loaded: 4, empty: 0, active: 0 }, '看门狗先推进之后，第二个候选仍然必须被真的请求并出图');
  // 对抗变异：去掉「只为当前 url 补发」的闸门后，第二层在 t=30 的迟到 error 会通过
  // onerror 的陈旧闸门（此时 data-queued-src 已经等于 sources[index-1]），让第一层
  // 再推进一次直接跳到候选末尾——本来能显示的图被丢掉，卡片落到「暂无预览」，
  // 而第二层随后把那张图加载成功也只是白加载（onload 被 completed 挡掉，
  // 没有 is-loaded，CSS 里仍然是 opacity: 0）。
  const unguarded = queueSource.replace(' && img.getAttribute("data-queued-src") === url', '');
  assert.ok(unguarded !== queueSource, '变异必须真的去掉了闸门');
  const broken = await run(unguarded);
  assert.equal(broken.loaded, 0, `去掉闸门后必须能测出候选被跳过：${JSON.stringify(broken)}`);
  assert.equal(broken.empty, 4, `去掉闸门后 4 张卡都会落到「暂无预览」：${JSON.stringify(broken)}`);
});

test('R130 P1-A 看门狗必须晚于第二层的最坏放弃时间', () => {
  // 第二层从放行到彻底放弃：10s 超时 + 10s 重试冷却 + 再 10s 超时 = 30s。看门狗
  // 比它早就会抢在第二层前面推进——既有概率跳过本来能显示的候选地址，也会在每张
  // 慢图上误报一条 card_image_stalled，把「名额是否泄漏」这个判据搞脏。
  // 工单原文写的是 25 秒，按这个算式必须往上调，台账里记了这条偏差。
  const timeout = Number(queueSource.match(/setTimeout\(\(\) => \{ timedOut = true;[^}]*\}, (\d+)\)/)?.[1]);
  const retryDelay = Number(queueSource.match(/retryDelay = (\d+)/)?.[1]);
  const maxRetries = Number(queueSource.match(/maxRetries = (\d+)/)?.[1]);
  assert.ok(timeout > 0 && retryDelay >= 0 && maxRetries >= 0, '第二层的超时/冷却/重试次数必须能读出来');
  const worstCase = timeout * (maxRetries + 1) + retryDelay * maxRetries;
  assert.equal(worstCase, 30000, '第二层最坏路径应当是 30s；这个数变了就要重新核对看门狗时长');
  assert.ok(PRODUCTION_STALL_MS > worstCase, `看门狗 ${PRODUCTION_STALL_MS}ms 必须晚于第二层最坏放弃时间 ${worstCase}ms`);
});

test('R130 评审补漏：弹窗、悬停视频、来源分类与旗帜取样的边界', () => {
  // 打开详情弹窗必须停掉悬停视频：弹窗进入顶层之后卡片的 pointerleave 不可靠，
  // /api/media 会一直挂着连接，和弹窗自己的原画、背景抢浏览器那 6 条连接。
  assert.match(functionSource(appSource, 'openSkinDetails'), /stopHoverVideo\(\);/, '皮肤详情弹窗');
  assert.match(functionSource(appSource, 'openChromaDetails'), /stopHoverVideo\(\);/, '炫彩详情弹窗');
  // 关闭弹窗必须把图片回调与排队地址一起摘掉：弹窗节点一直留在文档里
  // （isConnected 恒为 true），第二层放弃时照样补发 error，否则一个已经关闭的
  // 弹窗会继续试剩下的候选地址、白占连接，还会把「暂无预览」写进下次的占位。
  const close = functionSource(appSource, 'closeSkinDialog');
  assert.match(close, /clearCardImageWatchdog\(el\.skinDialogImage\)/);
  assert.match(close, /el\.skinDialogImage\.onload = null;/);
  assert.match(close, /el\.skinDialogImage\.onerror = null;/);
  assert.match(close, /el\.skinDialogImage\.removeAttribute\("data-queued-src"\)/);
  // 来源分类不出来就不带这个字段，绝不兜底成 "lcu"——那是拿推断值代替证据。
  assert.doesNotMatch(functionSource(appSource, 'reportCardImageStall'), /\|\| "lcu"/);
  assert.match(functionSource(appSource, 'reportCardImageStall'), /imageSource: window\.deepLegendsImageSourceLabel\?\.\(url\),/);
  // 重复打开同一条目时 URL 没变，浏览器可能不派发 load；必须先摘 src，
  // 否则 applyDetailArtSize 不跑，弹窗会卡在 128×128 占位上而图片已经显示。
  assert.match(functionSource(facadeSource, 'openDetail'), /el\.dialogImage\.removeAttribute\("src"\);\s*\n\s*el\.dialogImage\.src = fields\.image;/);
  // 旗帜比例取样：视图必须在建卡时定死（onload 是异步的，等它回来视图可能已经
  // 切走，一张方形头像会把 1.0 写进整个旗帜网格），目录重读时必须能重新取样。
  assert.match(facadeSource, /sampleBannerRatio\(image, isBanner\)/);
  assert.match(functionSource(facadeSource, 'createCard'), /const isBanner = state\.view === "banners";/);
  assert.match(functionSource(facadeSource, 'sampleBannerRatio'), /if \(!isBanner \|\| bannerRatioSampled\) return;/);
  assert.doesNotMatch(functionSource(facadeSource, 'sampleBannerRatio'), /state\.view/, '取样函数不得再读实时视图');
  assert.match(functionSource(facadeSource, 'load'), /if \(key === "banners"\) resetBannerRatioSample\(\);/);
  // 断连/出错时 aria-label 必须一起清掉：它会盖过内容给读屏软件播报，
  // 否则断连后 live region 念的还是上一份目录的「共 N 款 · 已拥有 M」。
  assert.match(functionSource(facadeSource, 'renderDisconnected'), /el\.meta\.removeAttribute\("aria-label"\)/);
  assert.match(functionSource(facadeSource, 'renderError'), /el\.meta\.removeAttribute\("aria-label"\)/);
  // 行为面：视图已经切到旗帜，但卡片是在头像视图里建的，它迟到的 onload 不得写比例。
  const late = facadeWindow();
  try {
    late.api.state.view = 'banners';
    late.api.sampleBannerRatio(natural(late.w, 128, 128), false);
    assert.equal(late.api.el.grid.style.getPropertyValue('--facade-banner-ratio'), '', '建卡时不是旗帜，迟到的 onload 不得把 1.0 写进旗帜网格');
    assert.equal(late.api.bannerRatioSampled, false);
  } finally { late.close(); }
});
