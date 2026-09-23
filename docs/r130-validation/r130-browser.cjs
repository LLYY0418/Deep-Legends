// R130 真机 Chromium 验证：P1-A 的名额泄漏与两道防线，以及 P2–P6 的界面截图。
//
// 用法：CHROME_BIN="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
//        node docs/r130-validation/r130-browser.cjs
// 输出：docs/r130-validation/browser/*.png 与 result.json
//
// 这里跑的是**真实的** image-queue.js、favorites-facade.js、app.css、index.html 的
// 收藏页片段，以及从真实 app.js 原样抽出的卡片队列函数；只有目录 JSON 与图片是
// 合成的（SVG，带精确的 naturalWidth/naturalHeight）。不连英雄联盟客户端。
'use strict';
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');

const root = path.resolve(__dirname, '..', '..');
const web = path.join(root, 'backend', 'web');
const output = process.env.R130_BROWSER_OUTPUT || path.join(__dirname, 'browser');
const chrome = process.env.CHROME_BIN || (process.platform === 'darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'r130-browser-'));
fs.mkdirSync(output, { recursive: true });

const read = (name) => fs.readFileSync(path.join(web, name), 'utf8');
const appSource = read('app.js');
const queueSource = read('image-queue.js');
const facadeSource = read('favorites-facade.js');
const indexHTML = read('index.html');

// 从真实 app.js 原样抽出第一层卡片队列（与 backend/web/r130.test.cjs 同一套写法）。
function functionSource(script, name) {
  const start = script.indexOf(`function ${name}(`);
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
const CARD_QUEUE_FUNCTIONS = [
  'clearCardImageWatchdog', 'reportCardImageStall', 'loadImageSources',
  'ensureCardImageObserver', 'withdrawCardImageJob', 'deferCardImageSources',
  'enqueueCardImageJob', 'pumpCardImageQueue', 'finishCardImageJob', 'cancelDeferredImages',
];
// 看门狗时长直接读生产常量，脚本里不写死——写死就会在一个和生产不同的时长上通过。
const PRODUCTION_STALL_MS = Number(String(appSource.match(/CARD_IMAGE_STALL_MS\s*=\s*([\d_]+)/)?.[1] || '').replace(/_/g, ''));
assert.ok(PRODUCTION_STALL_MS > 0, 'app.js 里必须能读出 CARD_IMAGE_STALL_MS');
const cardQueueScript = (stallMs) => `(() => {
  "use strict";
  const CARD_IMAGE_STALL_MS = ${stallMs};
  const CARD_IMAGE_STALL_REPORT_INTERVAL_MS = 10000;
  window.state = {
    cardImageObserver: null, cardImageJobs: new WeakMap(), cardImageQueue: [],
    activeCardImages: 0, activePrestigeCardImages: 0,
    cardImageWatchdogs: new WeakMap(), lastCardImageStallReportAt: 0, hoverVideo: null,
  };
  window.el = { appScroll: document.getElementById('app-scroll') };
  ${CARD_QUEUE_FUNCTIONS.map((name) => functionSource(appSource, name)).join('\n')}
  window.deferCardImageSources = deferCardImageSources;
  window.cardTemplate = document.getElementById('skin-card-template');
})();`;

// 从真实 index.html 里切出需要的三块（片段本身就是生产标记，不重写）。
const slice = (marker, endMarker) => {
  const start = indexHTML.indexOf(marker);
  assert.notEqual(start, -1, `index.html 里找不到 ${marker}`);
  const end = indexHTML.indexOf(endMarker, start);
  assert.notEqual(end, -1, `index.html 里找不到 ${marker} 的结束标记`);
  return indexHTML.slice(start, end + endMarker.length);
};
const facadePanel = slice('<section id="favorites-facade-panel"', '</section>').replace(' role="tabpanel"', ' role="tabpanel"').replace(/ hidden>/, '>');
const cardTemplate = slice('<template id="skin-card-template">', '</template>');
const facadeDialog = slice('<dialog id="facade-detail-dialog"', '</dialog>');

// 变异体：抹掉第二层放弃时补发的合成 error（与 backend/web/r130.test.cjs 同一处）。
const SYNTHETIC_ERROR = 'try { img.dispatchEvent(new Event("error")); } catch (_) {}';
const mutatedQueue = queueSource.replace(SYNTHETIC_ERROR, '');
assert.ok(mutatedQueue !== queueSource, '变异必须真的去掉了合成 error');
const page = (mode) => `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<link rel="stylesheet" href="/app.css">
<script src="/image-queue.js${mode ? `?mode=${mode}` : ''}" defer><\/script>
<script src="/favorites-facade.js" defer><\/script>
<script src="/card-queue.js${mode ? `?mode=${mode}` : ''}" defer><\/script>
</head><body>
<div id="app-scroll" style="height:100vh;overflow:auto">
<main style="width:1200px;margin:0 auto;padding:18px">
${facadePanel}
<div id="card-host" class="skin-grid" style="margin-top:24px"></div>
</main>
</div>
${cardTemplate}
${facadeDialog}
</body></html>`;

// 合成图片：SVG 带显式 width/height，naturalWidth/naturalHeight 就是精确值。
const svg = (width, height, label, fill) => `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}"><rect width="${width}" height="${height}" fill="${fill}"/><text x="50%" y="50%" fill="#0b0d10" font-family="sans-serif" font-size="${Math.max(10, Math.round(width / 8))}" text-anchor="middle" dominant-baseline="middle">${label}</text></svg>`;
const PNG_1PX = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');

const ICON_IDS = [4379, 4380, 4381, 4382, 4383, 4384, 4385, 4386, 4387, 4388, 4389, 4390];
const iconsPayload = {
  total: 5099,
  iconOwnershipUnavailable: false,
  icons: ICON_IDS.map((id, index) => ({
    id, title: `头像 ${id}`, year: 2020 + (index % 6), owned: index % 3 !== 0,
    sets: index % 2 ? ['2024 赛事'] : [], searchTerms: [],
  })),
};
const BANNER_IDS = ['31', '32', '33', '34', '35', '36', '37', '38'];
const bannersPayload = {
  bannerOwnershipUnavailable: false,
  banners: BANNER_IDS.map((id, index) => ({
    id, idSecondary: index % 2 ? 'GOLD' : '', localizedName: `旗帜 ${id}`,
    owned: index % 4 !== 0, isTencentOnly: index % 3 === 0,
    // 真实旗帜是细长竖图，这里按 102×400 的比例合成。
    imagePath: `/banner-${id}-102x400.svg`, searchTerms: [],
  })),
};

let hungRequests = 0;
let proc, ws, server;
const results = { chrome: null, stall: {}, mutation: {}, facade: {} };

async function main() {
  proc = spawn(chrome, ['--headless=new', '--disable-background-timer-throttling', '--disable-renderer-backgrounding', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', `--user-data-dir=${temp}`, 'about:blank'], { stdio: ['ignore', 'ignore', 'pipe'] });
  const url = await new Promise((resolve, reject) => {
    let out = '';
    const timer = setTimeout(() => reject(Error('Chrome startup timeout')), 25000);
    proc.once('error', reject);
    proc.stderr.on('data', (chunk) => { out += chunk; const m = out.match(/DevTools listening on (ws:\/\/[^\s]+)/); if (m) { clearTimeout(timer); resolve(m[1]); } });
    proc.once('exit', (code) => reject(Error(`Chrome exited ${code}: ${out.slice(-800)}`)));
  });
  ws = new WebSocket(url);
  await new Promise((res, rej) => { ws.addEventListener('open', res, { once: true }); ws.addEventListener('error', rej, { once: true }); });
  let seq = 0;
  const pending = new Map();
  ws.addEventListener('message', (e) => { const msg = JSON.parse(e.data); if (pending.has(msg.id)) { const [resolve, reject] = pending.get(msg.id); pending.delete(msg.id); msg.error ? reject(Error(JSON.stringify(msg.error))) : resolve(msg.result); } });
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const id = ++seq;
    const timer = setTimeout(() => { pending.delete(id); reject(Error('CDP timeout ' + method)); }, 60000);
    pending.set(id, [(value) => { clearTimeout(timer); resolve(value); }, (error) => { clearTimeout(timer); reject(error); }]);
    ws.send(JSON.stringify({ id, method, params, sessionId }));
  });
  console.log('CDP connected');
  const browserErrors = [];
  ws.addEventListener('message', (e) => {
    const m = JSON.parse(e.data);
    if (m.method === 'Runtime.exceptionThrown') browserErrors.push(JSON.stringify(m.params.exceptionDetails).slice(0, 500));
  });
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
  const call = (m, p) => send(m, p, sessionId);
  const evaluate = async (expression) => {
    const r = await call('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (r.exceptionDetails) throw Error(JSON.stringify(r.exceptionDetails).slice(0, 1200));
    return r.result.value;
  };
  await call('Page.enable');
  await call('Runtime.enable');
  await call('Emulation.setDeviceMetricsOverride', { width: 1400, height: 1000, deviceScaleFactor: 1, mobile: false });
  await call('Page.bringToFront');
  results.chrome = await evaluate('navigator.userAgent');

  server = require('node:http').createServer((req, res) => {
    const parsed = new URL(req.url, 'http://localhost');
    const pathname = parsed.pathname;
    if (pathname === '/image-queue.js') { const mode = parsed.searchParams.get('mode'); res.setHeader('Content-Type', 'text/javascript'); res.end(mode === 'mutated' || mode === 'nosynthetic' ? mutatedQueue : queueSource); return; }
    if (pathname === '/favorites-facade.js') { res.setHeader('Content-Type', 'text/javascript'); res.end(facadeSource); return; }
    if (pathname === '/app.css') { res.setHeader('Content-Type', 'text/css'); res.end(read('app.css')); return; }
    // 变异体把看门狗关掉。真机 setTimeout 会把 Infinity 转成 0（WebIDL long），
    // 所以这里用 int32 上限附近的大数，等价于「本次运行内永不触发」。
    if (pathname === '/card-queue.js') { res.setHeader('Content-Type', 'text/javascript'); res.end(cardQueueScript(parsed.searchParams.get('mode') === 'mutated' ? 2000000000 : PRODUCTION_STALL_MS)); return; }
    if (pathname === '/api/facade/icons') { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(iconsPayload)); return; }
    if (pathname === '/api/facade/banners') { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(bannersPayload)); return; }
    if (pathname === '/api/image') {
      const asset = parsed.searchParams.get('path') || '';
      // 永不返回的图：挂着连接什么都不写，正是工单 §1-A 复现用的那种请求。
      if (asset.includes('hung')) {
        hungRequests += 1;
        res.writeHead(200, { 'Content-Type': 'image/png' });
        res.flushHeaders();
        const keep = setTimeout(() => res.end(PNG_1PX), 300000);
        res.once('close', () => clearTimeout(keep));
        return;
      }
      const banner = /-(\d+)x(\d+)\.svg$/.exec(asset);
      if (banner) {
        res.setHeader('Content-Type', 'image/svg+xml');
        res.end(svg(Number(banner[1]), Number(banner[2]), asset.slice(1, 12), '#7fa8d8'));
        return;
      }
      if (asset.includes('profile-icons')) { res.setHeader('Content-Type', 'image/svg+xml'); res.end(svg(128, 128, '128', '#d8b47f')); return; }
      res.setHeader('Content-Type', 'image/png');
      res.end(PNG_1PX);
      return;
    }
    res.setHeader('Content-Type', 'text/html');
    res.end(page(parsed.searchParams.get('mode') || ''));
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const base = `http://127.0.0.1:${server.address().port}`;
  const navigate = async (query = '') => {
    await call('Page.navigate', { url: `${base}/${query}` });
    for (let i = 0; i < 200; i++) { if (await evaluate('typeof deepLegendsQueueImage === "function"')) break; await new Promise((r) => setTimeout(r, 25)); }
  };
  const screenshot = async (name) => {
    const shot = await call('Page.captureScreenshot', { format: 'png' });
    fs.writeFileSync(path.join(output, name), Buffer.from(shot.data, 'base64'));
    console.log('screenshot', name);
  };
  const wait = (ms) => new Promise((r) => setTimeout(r, ms));

  // ---------- P1-A：8 张永不返回的图，真机 Chromium ----------
  const stallProbe = `(() => {
    const cards = [...document.querySelectorAll('#card-host .skin-card')];
    return {
      activeCardImages: window.state.activeCardImages,
      queued: window.state.cardImageQueue.length,
      cards: cards.length,
      ready: cards.filter((card) => card.querySelector('img').dataset.imageReady === 'true').length,
      loading: cards.filter((card) => { const f = card.querySelector('.image-fallback'); return f.textContent === '加载中' && !f.hidden; }).length,
      empty: cards.filter((card) => card.querySelector('.image-fallback').textContent === '暂无预览').length,
      stalls: (window.__stalls || []).length,
    };
  })()`;
  const addCards = (count, hung, offset = 0) => evaluate(`(() => {
    window.__stalls = window.__stalls || [];
    window.reportFlowDiagnostic = (event, reason, fields) => { if (event === 'card_image_stalled') window.__stalls.push({ reason, fields }); };
    const host = document.getElementById('card-host');
    for (let i = 0; i < ${count}; i++) {
      const card = window.cardTemplate.content.firstElementChild.cloneNode(true);
      host.append(card);
      const image = card.querySelector('img');
      const fallback = card.querySelector('.image-fallback');
      window.deferCardImageSources(image, fallback, ['/api/image?path=%2F${hung ? 'hung' : 'ok'}-\${i + offset}.png']);
    }
    return host.querySelectorAll('.skin-card').length;
  })()`);

  console.log('--- P1-A 修复后（真实 image-queue.js + 真实 app.js 队列函数）');
  await navigate('?stall=fixed');
  await addCards(8, true);
  await wait(1500);
  results.stall.atOneSecond = await evaluate(stallProbe);
  console.log('1.5s:', JSON.stringify(results.stall.atOneSecond));
  // 第二层从放行到彻底放弃：10s 超时 → 10s 冷却 → 再 10s 超时 = 30s；而且第二层
  // 只有 5 个名额，8 张图要分两批，全部收尾在 40s 左右。第一层看门狗是 45s 兜底，
  // 正常情况下轮不到它。
  await wait(58000);
  results.stall.afterGiveUp = await evaluate(stallProbe);
  console.log('60s:', JSON.stringify(results.stall.afterGiveUp));
  await addCards(6, false, 100);
  await wait(6000);
  results.stall.afterFresh = await evaluate(stallProbe);
  console.log('加入 6 张正常图 +6s:', JSON.stringify(results.stall.afterFresh));
  await screenshot('p1-stall-fixed.png');

  // 只关掉第二层那道防线（不补发合成 error）：看门狗必须独自兜住名额，而且这时
  // 才该出现 card_image_stalled——它是「第二层没能通知第一层」的专属信号，
  // 单纯慢的图不该被误报成名额泄漏。
  console.log('--- P1-A 只留看门狗（第二层不补发 error）');
  await navigate('?mode=nosynthetic');
  for (let i = 0; i < 200; i++) { if (await evaluate('typeof window.deferCardImageSources === "function"')) break; await wait(25); }
  await addCards(8, true);
  await wait(PRODUCTION_STALL_MS + 15000);
  results.stall.watchdogOnly = await evaluate(stallProbe);
  console.log(`只留看门狗 ${Math.round((PRODUCTION_STALL_MS + 15000) / 1000)}s:`, JSON.stringify(results.stall.watchdogOnly));
  await screenshot('p1-stall-watchdog-only.png');

  // 变异体走 mode=mutated：服务端换掉 image-queue.js 的源码并把看门狗设成 Infinity，
  // 页面重新加载，避免同一个页面里跑两份第二层队列。
  await navigate('?mode=mutated');
  for (let i = 0; i < 200; i++) { if (await evaluate('typeof window.deferCardImageSources === "function"')) break; await wait(25); }
  await addCards(8, true);
  await wait(45000);
  results.mutation.afterGiveUp = await evaluate(stallProbe);
  console.log('变异 45s:', JSON.stringify(results.mutation.afterGiveUp));
  await screenshot('p1-stall-mutated.png');

  // ---------- P2–P6：真实收藏页片段 + 真实 favorites-facade.js ----------
  console.log('--- P2–P6 头像与旗帜页面');
  await navigate('?facade=1');
  await evaluate(`(() => {
    const panel = document.getElementById('favorites-facade-panel');
    panel.hidden = false;
    window.addEventListener('deep-legends:status', () => {}, { once: true });
    window.dispatchEvent(new CustomEvent('deep-legends:status', { detail: { connected: true } }));
    window.deepLegendsFavoritesFacade.activate();
  })()`);
  for (let i = 0; i < 200; i++) { if (await evaluate(`document.querySelectorAll('#facade-grid .skin-card').length >= ${ICON_IDS.length}`)) break; await wait(25); }
  await wait(2500);
  results.facade.icons = await evaluate(`(() => {
    const grid = document.getElementById('facade-grid');
    const cards = [...grid.querySelectorAll('.skin-card')];
    const art = cards[0]?.querySelector('.skin-art');
    const image = cards[0]?.querySelector('img');
    const meta = document.getElementById('facade-list-meta');
    return {
      cards: cards.length,
      metricsBar: !!document.querySelector('.facade-metrics'),
      totalNode: !!document.getElementById('facade-total'),
      stateBadges: cards.filter((card) => { const badge = card.querySelector('.skin-state'); return badge && badge.textContent.trim() !== ''; }).length,
      stateVisible: cards.filter((card) => { const badge = card.querySelector('.skin-state'); return badge && badge.offsetParent !== null; }).length,
      locks: cards.filter((card) => card.classList.contains('is-locked')).length,
      unownedToggleChecked: document.getElementById('facade-show-unowned').checked,
      unownedToggleHidden: document.getElementById('facade-unowned-control').hidden,
      metaText: meta.textContent,
      metaNumbers: [...meta.querySelectorAll('b.list-meta-number')].map((node) => ({ text: node.textContent, color: getComputedStyle(node).color, weight: getComputedStyle(node).fontWeight, numeric: getComputedStyle(node).fontVariantNumeric })),
      gridColumns: getComputedStyle(grid).gridTemplateColumns,
      artBox: art ? { width: art.getBoundingClientRect().width, height: art.getBoundingClientRect().height } : null,
      natural: image ? { width: image.naturalWidth, height: image.naturalHeight } : null,
      copyBelowArt: (() => { const copy = cards[0]?.querySelector('.skin-copy'); if (!copy || !art) return null; return { position: getComputedStyle(copy).position, top: copy.getBoundingClientRect().top, artBottom: art.getBoundingClientRect().bottom }; })(),
      heroHidden: cards[0] ? getComputedStyle(cards[0].querySelector('.skin-hero')).display : null,
      cardTitle: cards[0]?.title || null,
    };
  })()`);
  console.log('icons:', JSON.stringify(results.facade.icons, null, 1));
  await screenshot('p3-p5-icons.png');
  // 打开「显示未拥有」：未拥有的头像必须靠置灰 + 锁图标表示，而不是右上角标签。
  results.facade.iconsWithUnowned = await evaluate(`(async () => {
    const toggle = document.getElementById('facade-show-unowned');
    toggle.checked = true;
    toggle.dispatchEvent(new Event('change'));
    await new Promise((resolve) => setTimeout(resolve, 1500));
    const cards = [...document.querySelectorAll('#facade-grid .skin-card')];
    return {
      cards: cards.length,
      locks: cards.filter((card) => card.classList.contains('is-locked')).length,
      stateBadges: cards.filter((card) => { const badge = card.querySelector('.skin-state'); return badge && badge.textContent.trim() !== ''; }).length,
      stateVisible: cards.filter((card) => { const badge = card.querySelector('.skin-state'); return badge && badge.offsetParent !== null; }).length,
      metaText: document.getElementById('facade-list-meta').textContent,
    };
  })()`);
  console.log('iconsWithUnowned:', JSON.stringify(results.facade.iconsWithUnowned));
  await screenshot('p3-icons-with-unowned.png');
  // 关回去，后面的详情弹窗与旗帜视图都从默认状态开始。
  await evaluate(`(() => { const toggle = document.getElementById('facade-show-unowned'); toggle.checked = false; toggle.dispatchEvent(new Event('change')); })()`);
  await wait(1200);

  // 头像详情弹窗：P6。
  await evaluate(`document.querySelectorAll('#facade-grid .skin-card')[0].click()`);
  await wait(1800);
  results.facade.iconDetail = await evaluate(`(() => {
    const dialog = document.getElementById('facade-detail-dialog');
    const art = dialog.querySelector('.dialog-art');
    const image = document.getElementById('facade-detail-image');
    const box = art.getBoundingClientRect();
    const close = document.getElementById('facade-detail-close').getBoundingClientRect();
    return {
      open: dialog.open,
      dialogWidth: dialog.getBoundingClientRect().width,
      artBox: { width: box.width, height: box.height },
      natural: { width: image.naturalWidth, height: image.naturalHeight },
      objectFit: getComputedStyle(image).objectFit,
      vars: { width: dialog.style.getPropertyValue('--facade-art-width'), height: dialog.style.getPropertyValue('--facade-art-height') },
      ownershipRow: [...dialog.querySelectorAll('dt')].map((node) => node.textContent),
      closeInsideArt: close.top >= box.top && close.bottom <= box.bottom && close.right <= box.right + 1,
    };
  })()`);
  console.log('iconDetail:', JSON.stringify(results.facade.iconDetail, null, 1));
  await screenshot('p6-icon-detail.png');
  await evaluate(`document.getElementById('facade-detail-close').click()`);
  await wait(400);

  // 旗帜视图：P2。
  await evaluate(`document.getElementById('facade-view-banners').click()`);
  for (let i = 0; i < 200; i++) { if (await evaluate(`document.querySelectorAll('#facade-grid .skin-card').length >= ${BANNER_IDS.length}`)) break; await wait(25); }
  await wait(2500);
  results.facade.banners = await evaluate(`(() => {
    const grid = document.getElementById('facade-grid');
    const cards = [...grid.querySelectorAll('.skin-card')];
    const art = cards[0]?.querySelector('.skin-art');
    const image = cards[0]?.querySelector('img');
    const box = art?.getBoundingClientRect();
    // object-fit: contain 下图片按自身比例铺满格子；contain 的高度 = min(格子高, 格子宽 / 图片比例)。
    const drawn = image ? Math.min(box.height, box.width / (image.naturalWidth / image.naturalHeight)) : 0;
    return {
      cards: cards.length,
      bannerRatioVar: grid.style.getPropertyValue('--facade-banner-ratio'),
      gridColumns: getComputedStyle(grid).gridTemplateColumns,
      artBox: box ? { width: box.width, height: box.height } : null,
      natural: image ? { width: image.naturalWidth, height: image.naturalHeight } : null,
      objectFit: image ? getComputedStyle(image).objectFit : null,
      verticalWhitespace: box && drawn ? Math.abs(1 - drawn / box.height) : null,
      cropped: image && box ? (image.naturalWidth / image.naturalHeight) < (box.width / box.height) - 0.01 : null,
      stateBadges: cards.filter((card) => { const badge = card.querySelector('.skin-state'); return badge && badge.textContent.trim() !== ''; }).length,
      copyBelowArt: (() => { const copy = cards[0]?.querySelector('.skin-copy'); if (!copy || !art) return null; return { position: getComputedStyle(copy).position, top: copy.getBoundingClientRect().top, artBottom: art.getBoundingClientRect().bottom }; })(),
      metaText: document.getElementById('facade-list-meta').textContent,
      unownedToggleChecked: document.getElementById('facade-show-unowned').checked,
    };
  })()`);
  console.log('banners:', JSON.stringify(results.facade.banners, null, 1));
  await screenshot('p2-banners.png');

  // 旗帜详情弹窗：P6。
  await evaluate(`document.querySelectorAll('#facade-grid .skin-card')[1].click()`);
  await wait(1800);
  results.facade.bannerDetail = await evaluate(`(() => {
    const dialog = document.getElementById('facade-detail-dialog');
    const art = dialog.querySelector('.dialog-art');
    const image = document.getElementById('facade-detail-image');
    const box = art.getBoundingClientRect();
    return {
      artBox: { width: box.width, height: box.height },
      natural: { width: image.naturalWidth, height: image.naturalHeight },
      ratio: box.width / box.height,
      naturalRatio: image.naturalWidth / image.naturalHeight,
      objectFit: getComputedStyle(image).objectFit,
      dialogWidth: dialog.getBoundingClientRect().width,
    };
  })()`);
  console.log('bannerDetail:', JSON.stringify(results.facade.bannerDetail, null, 1));
  await screenshot('p6-banner-detail.png');

  // 拥有状态不可用（R124 降级）：不显示标签、不上锁、摘要行说明、开关隐藏但记忆值不变。
  await evaluate(`document.getElementById('facade-detail-close').click()`);
  await wait(300);
  results.facade.degraded = await evaluate(`(async () => {
    const snapshot = () => {
      const cards = [...document.querySelectorAll('#facade-grid .skin-card')];
      return {
        cards: cards.length,
        locks: cards.filter((card) => card.classList.contains('is-locked')).length,
        stateBadges: cards.filter((card) => { const badge = card.querySelector('.skin-state'); return badge && badge.textContent.trim() !== ''; }).length,
        metaText: document.getElementById('facade-list-meta').textContent,
        unownedToggleHidden: document.getElementById('facade-unowned-control').hidden,
        unownedToggleChecked: document.getElementById('facade-show-unowned').checked,
      };
    };
    const reconnect = async () => {
      window.dispatchEvent(new CustomEvent('deep-legends:status', { detail: { connected: false } }));
      await new Promise((resolve) => setTimeout(resolve, 150));
      window.dispatchEvent(new CustomEvent('deep-legends:status', { detail: { connected: true } }));
      await new Promise((resolve) => setTimeout(resolve, 2000));
    };
    document.getElementById('facade-view-icons').click();
    await new Promise((resolve) => setTimeout(resolve, 1500));
    // 拥有状态不可用（R124 降级）：后端把 owned 一律填 false。
    const original = window.fetch;
    window.fetch = async (input, init) => {
      if (String(input).includes('/api/facade/icons')) {
        const payload = ${JSON.stringify(iconsPayload)};
        payload.iconOwnershipUnavailable = true;
        payload.icons = payload.icons.map((icon) => ({ ...icon, owned: false }));
        return new Response(JSON.stringify(payload), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return original(input, init);
    };
    await reconnect();
    const unavailable = snapshot();
    // 恢复可用：开关必须回到用户原来的设置（默认关闭），网格重新按已拥有过滤。
    window.fetch = original;
    await reconnect();
    return { unavailable, recovered: snapshot() };
  })()`);
  console.log('degraded:', JSON.stringify(results.facade.degraded, null, 1));
  await screenshot('p3-p5-degraded.png');

  results.browserErrors = browserErrors;
  results.hungRequests = hungRequests;
  fs.writeFileSync(path.join(output, 'result.json'), JSON.stringify(results, null, 2));
  console.log('\n==== 断言 ====');

  // P1-A：名额必须归零，卡片必须落到「暂无预览」，之后加入的正常图必须全部加载。
  assert.equal(results.stall.atOneSecond.activeCardImages, 8, '一开始 8 个名额应当被挂起的图占满');
  assert.equal(results.stall.atOneSecond.empty, 0);
  assert.equal(results.stall.afterGiveUp.activeCardImages, 0, '真机 Chromium 上第二层放弃后名额必须归零');
  assert.equal(results.stall.afterGiveUp.empty, 8, '8 张挂起的图必须落到「暂无预览」，不能停在「加载中」');
  assert.equal(results.stall.afterGiveUp.loading, 0);
  // 第二层只有 5 个名额、第一层有 8 个，排在第二层队尾的卡片可能等满 45s 才轮到，
  // 这时看门狗会报一条。它表达的是「一个卡片名额被占了 45s 还没有任何 load/error」：
  // 无论根因是第二层没通知还是第二层严重积压，用户看到的都是 45 秒的「加载中」，
  // 都该报，所以这里不断言 0，只钉限速——8 张卡落在同一个 10 秒窗口里最多一条。
  assert.ok(results.stall.afterGiveUp.stalls <= 1, `同一个 10 秒窗口最多一条，实际 ${results.stall.afterGiveUp.stalls} 条`);
  assert.equal(results.stall.afterFresh.ready, 6, '名额释放后新加入的 6 张正常图必须全部加载');
  assert.equal(results.stall.afterFresh.loading, 0);
  assert.equal(results.stall.watchdogOnly.activeCardImages, 0, '只留看门狗也必须把名额兜回来');
  assert.equal(results.stall.watchdogOnly.empty, 8, '只留看门狗时 8 张图同样要落到「暂无预览」');
  assert.ok(results.stall.watchdogOnly.stalls >= 1, '看门狗必须在真机 Chromium 上也上报 card_image_stalled');
  // 变异：两道防线都关掉时必须复现「整屏永久加载中」。
  assert.equal(results.mutation.afterGiveUp.activeCardImages, 8, '两道防线都关掉后名额必须仍然是 8（否则这个场景测不到修复）');
  assert.equal(results.mutation.afterGiveUp.loading, 8, '变异体必须整屏停在「加载中」');
  assert.equal(results.mutation.afterGiveUp.empty, 0);
  assert.equal(results.mutation.afterGiveUp.stalls, 0, '两道防线都关掉时不会有任何上报，名额也就永久泄漏了');

  // P4：三格统计条彻底不在。
  assert.equal(results.facade.icons.metricsBar, false, '.facade-metrics 必须删除');
  assert.equal(results.facade.icons.totalNode, false, '#facade-total 必须删除');
  // P3：任何卡片都不得显示右上角状态标签；未拥有仍然置灰上锁。
  assert.equal(results.facade.icons.stateBadges, 0, '头像卡片不得再写状态标签');
  assert.equal(results.facade.icons.stateVisible, 0, '状态标签必须真的不可见');
  assert.equal(results.facade.icons.locks, 0, '默认只显示已拥有，不该出现锁图标');
  assert.equal(results.facade.iconsWithUnowned.cards, ICON_IDS.length, '打开「显示未拥有」后必须显示全部头像');
  assert.ok(results.facade.iconsWithUnowned.locks > 0, '未拥有的头像必须靠锁图标表示');
  assert.equal(results.facade.iconsWithUnowned.stateBadges, 0, '打开「显示未拥有」后也不得出现右上角状态标签');
  assert.equal(results.facade.iconsWithUnowned.stateVisible, 0, '状态标签必须真的不可见');
  assert.equal(results.facade.iconsWithUnowned.metaText, results.facade.icons.metaText, '摘要行是目录口径，不随筛选变化');
  assert.equal(results.facade.banners.stateBadges, 0, '旗帜卡片不得再写状态标签');
  assert.equal(results.facade.degraded.unavailable.stateBadges, 0, '拥有状态未知时也不显示标签');
  assert.equal(results.facade.degraded.unavailable.locks, 0, 'R124：拥有状态未知时不上锁');
  // P5：摘要行没有「已显示」，数字上主题色，头像默认只显示已拥有。
  assert.equal(results.facade.icons.metaText, `共 5,099 款头像 · 已拥有 ${iconsPayload.icons.filter((icon) => icon.owned).length}`);
  assert.equal(results.facade.icons.metaText.includes('已显示'), false);
  assert.deepEqual(results.facade.icons.metaNumbers.map((node) => node.text), ['5,099', String(iconsPayload.icons.filter((icon) => icon.owned).length)]);
  assert.ok(results.facade.icons.metaNumbers.every((node) => node.color !== 'rgb(0, 0, 0)' && node.weight === '650' && node.numeric.includes('tabular-nums')), `摘要行数字样式：${JSON.stringify(results.facade.icons.metaNumbers)}`);
  assert.equal(results.facade.icons.unownedToggleChecked, false, '头像默认关闭「显示未拥有」');
  assert.equal(results.facade.icons.unownedToggleHidden, false);
  assert.equal(results.facade.icons.cards, iconsPayload.icons.filter((icon) => icon.owned).length, '默认只显示已拥有的头像');
  assert.equal(results.facade.banners.unownedToggleChecked, true, '旗帜默认显示全部');
  assert.equal(results.facade.banners.cards, BANNER_IDS.length);
  assert.match(results.facade.degraded.unavailable.metaText, /拥有状态未读取/);
  assert.equal(results.facade.degraded.unavailable.unownedToggleHidden, true, 'R124：拥有状态不可用时开关隐藏');
  assert.equal(results.facade.degraded.unavailable.unownedToggleChecked, false, 'R130 P5-4：隐藏期间不得改写用户保存的值');
  assert.equal(results.facade.degraded.unavailable.cards, ICON_IDS.length, 'R124：拥有状态不可用时网格不得被滤空');
  assert.equal(results.facade.degraded.recovered.unownedToggleHidden, false);
  assert.equal(results.facade.degraded.recovered.unownedToggleChecked, false, 'R130 P5-4：恢复后开关必须回到用户原来的设置（默认关闭）');
  assert.equal(results.facade.degraded.recovered.cards, iconsPayload.icons.filter((icon) => icon.owned).length, '恢复后重新按已拥有过滤');
  // P2：旗帜是 150px 竖长卡，比例取自图片真实尺寸，不裁剪，上下留白 ≤ 5%，文字在图片下方。
  assert.equal(results.facade.banners.bannerRatioVar, (102 / 400).toFixed(4), '网格变量必须是 naturalWidth/naturalHeight');
  const bannerColumns = results.facade.banners.gridColumns.split(' ').filter(Boolean);
  assert.ok(bannerColumns.length > 1 && bannerColumns.every((column) => column === '150px'), `旗帜格子必须固定 150px、宽窗口下不拉伸：${results.facade.banners.gridColumns}`);
  // 格子宽 150px，卡片左右各 1px 边框，所以图片区域是 148px；高度必须等于宽度 / 真实比例。
  assert.ok(Math.abs(results.facade.banners.artBox.width - 148) <= 1, `图片区域宽度应当是 148px，实际 ${results.facade.banners.artBox.width}`);
  assert.ok(Math.abs(results.facade.banners.artBox.height - results.facade.banners.artBox.width / (102 / 400)) <= 1.5, `旗帜图片区域高度应当是 ${results.facade.banners.artBox.width / (102 / 400)}，实际 ${results.facade.banners.artBox.height}`);
  assert.equal(results.facade.banners.objectFit, 'contain');
  assert.equal(results.facade.banners.cropped, false, '旗帜不得裁剪');
  assert.ok(results.facade.banners.verticalWhitespace <= 0.05, `上下留白 ${(results.facade.banners.verticalWhitespace * 100).toFixed(2)}% 超过 5%`);
  assert.ok(results.facade.banners.copyBelowArt.top >= results.facade.banners.copyBelowArt.artBottom - 1, '旗帜名称必须在图片下方，不能压在原画上');
  assert.equal(results.facade.banners.copyBelowArt.position, 'static');
  assert.ok(results.facade.icons.copyBelowArt.top >= results.facade.icons.copyBelowArt.artBottom - 1, '头像名称同样在图片下方');
  assert.match(results.facade.icons.cardTitle, /^头像 \d+$/, '完整名称放 title 供悬停查看');
  // P6：头像详情按 1:1 原尺寸显示，弹窗收窄；旗帜按自身比例。
  assert.deepEqual(results.facade.iconDetail.natural, { width: 128, height: 128 });
  assert.deepEqual(results.facade.iconDetail.artBox, { width: 128, height: 128 }, '头像图片区域必须等于 naturalWidth×naturalHeight');
  assert.deepEqual(results.facade.iconDetail.vars, { width: '128px', height: '128px' });
  assert.equal(results.facade.iconDetail.objectFit, 'contain');
  assert.ok(results.facade.iconDetail.dialogWidth < 400, `弹窗必须跟着缩小，实际 ${results.facade.iconDetail.dialogWidth}px`);
  assert.ok(results.facade.iconDetail.ownershipRow.includes('拥有状态'), '详情弹窗保留「拥有状态」一行');
  assert.equal(results.facade.iconDetail.closeInsideArt, false, '关闭按钮必须挪出图片区域，否则会被 contain: paint 裁掉');
  assert.ok(Math.abs(results.facade.bannerDetail.ratio - results.facade.bannerDetail.naturalRatio) < 0.02, `旗帜详情必须按自身比例：${results.facade.bannerDetail.ratio} vs ${results.facade.bannerDetail.naturalRatio}`);
  assert.ok(results.facade.bannerDetail.artBox.height <= 320.5, '旗帜详情高度不得超过弹窗上限');
  assert.equal(results.facade.bannerDetail.objectFit, 'contain');

  assert.deepEqual(browserErrors, [], `真机 Chromium 上报了异常：${JSON.stringify(browserErrors).slice(0, 1200)}`);
  console.log('R130 Chromium PASS', JSON.stringify({
    stallFixed: results.stall.afterGiveUp, stallWatchdogOnly: results.stall.watchdogOnly, stallMutated: results.mutation.afterGiveUp,
    hungRequests: results.hungRequests,
    iconDetail: results.facade.iconDetail.artBox, bannerCell: results.facade.banners.artBox,
    bannerWhitespace: Number((results.facade.banners.verticalWhitespace * 100).toFixed(2)) + '%',
    dialogWidth: results.facade.iconDetail.dialogWidth,
  }));
}

main().then(() => { cleanup(); process.exit(0); }, (error) => { console.error('R130 Chromium FAIL', error.message); try { fs.writeFileSync(path.join(output, 'result.json'), JSON.stringify(results, null, 2)); } catch (_) {} cleanup(); process.exit(1); });
function cleanup() {
  try { ws?.close(); } catch (_) {}
  try { proc?.kill('SIGKILL'); } catch (_) {}
  try { server?.close(); } catch (_) {}
  try { fs.rmSync(temp, { recursive: true, force: true }); } catch (_) {}
}
