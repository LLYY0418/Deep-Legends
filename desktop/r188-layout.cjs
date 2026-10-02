'use strict';
// Real Chromium layout evidence, using production renderers/tooltip/CSS.
// R188_SOURCE_DIR can point to the pre-change snapshot for before screenshots.
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const {read, extract, fixture} = require('../backend/web/r188-harness.cjs');
const directory = process.env.R188_SOURCE_DIR;
const before = process.env.R188_BEFORE === '1';
const output = process.env.R188_SHOTS || path.resolve(__dirname, '../docs/history/reports/r188');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'r188-layout-'));
let proc, ws, server;
async function main() {
  const f = fixture({directory, active: before ? 'insight' : 'runes', notice: '我的小队'});
  const members = [
    {gameName: '测试玩家一', tagLine: '演示', championId: 103, championName: '阿狸', profileIconId: 11},
    {gameName: '测试玩家二', tagLine: '演示', championId: 0, championName: '未知英雄', profileIconId: 12},
  ].map(p => ({...p, premadeGroup: '1', premadeSize: 2, premadeSource: 'session'}));
  const markup = f.renderRecommendationArea(f.data);
  const tag = f.renderLivePremadeTag(members[0], members);
  server = require('node:http').createServer((req, res) => {
    if (req.url.startsWith('/fixture-icon/') || req.url.startsWith('/api/image?')) {
      res.setHeader('Content-Type', 'image/svg+xml');
      res.end('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" rx="8" fill="#b88755"/><circle cx="16" cy="13" r="7" fill="#f1d3ad"/><path d="M4 32Q16 10 28 32" fill="#5c667e"/></svg>'); return;
    }
    res.setHeader('Content-Type', 'text/html');
    res.end(`<!doctype html><html data-theme="light"><meta charset="utf-8"><style>${read('app.css', directory)}\n${read('gameplay.css', directory)}</style><style>body{margin:0;padding:24px;overflow:auto} .fixture-tag{margin-top:180px;text-align:center} .recommendation-panel{min-height:0}body[data-shot="tooltip"] .recommendation-area{display:none}body[data-shot="lane"] .fixture-tag{display:none}</style><body data-shot="lane"><main>${markup}</main><div class="fixture-tag">${tag}</div><script>${extract(read('app.js', directory), 'setupFloatingTooltips')}\nsetupFloatingTooltips();</script></body></html>`);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const chrome = process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
  proc = spawn(chrome, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', `--user-data-dir=${temp}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
  const url = await new Promise((resolve, reject) => {
    let output = ''; const timer = setTimeout(() => reject(Error('Chrome startup timeout')), 20000);
    proc.once('error', reject); proc.stderr.on('data', chunk => {output += chunk; const m = output.match(/DevTools listening on (ws:\/\/[^\s]+)/); if (m) {clearTimeout(timer); resolve(m[1]);}});
    proc.once('exit', code => reject(Error(`Chrome exited ${code}: ${output.slice(-800)}`)));
  });
  ws = new WebSocket(url); await new Promise((resolve, reject) => {ws.addEventListener('open', resolve, {once: true}); ws.addEventListener('error', reject, {once: true});});
  let seq = 0; const pending = new Map();
  ws.addEventListener('message', event => {const msg = JSON.parse(event.data); if (pending.has(msg.id)) {const [resolve, reject] = pending.get(msg.id); pending.delete(msg.id); msg.error ? reject(Error(JSON.stringify(msg.error))) : resolve(msg.result);}});
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {const id = ++seq; pending.set(id, [resolve, reject]); ws.send(JSON.stringify({id, method, params, sessionId}));});
  const {targetId} = await send('Target.createTarget', {url: 'about:blank'});
  const {sessionId} = await send('Target.attachToTarget', {targetId, flatten: true});
  const call = (method, params = {}) => send(method, params, sessionId);
  const evaluate = async expression => {const r = await call('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true}); if (r.exceptionDetails) throw Error(JSON.stringify(r.exceptionDetails)); return r.result.value;};
  await call('Page.enable'); await call('Emulation.setDeviceMetricsOverride', {width: 1280, height: 480, deviceScaleFactor: 1, mobile: false});
  await call('Page.navigate', {url: `http://127.0.0.1:${server.address().port}`});
  await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{if(document.querySelector('#global-tooltip')){clearInterval(timer);resolve();}},20);})`);
  const frame = () => evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const shot = async name => {await frame(); const result = await call('Page.captureScreenshot', {format: 'png'}); fs.writeFileSync(path.join(output, name), Buffer.from(result.data, 'base64'));};
  fs.mkdirSync(output, {recursive: true});
  const measures = [];
  for (const width of [1280, 960, 820, 620]) {
    await call('Emulation.setDeviceMetricsOverride', {width, height: before ? 420 : 220, deviceScaleFactor: 1, mobile: false}); await frame();
    const m = await evaluate(`(()=>{const box=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,w:r.width,h:r.height,right:r.right}};const tabs=document.querySelector('.recommendation-tabs'),card=document.querySelector('[data-lane-matchup-card]'),slot=document.querySelector('[data-lane-matchup-slot]');return {tabs:box(tabs),card:box(card),slot:slot&&box(slot),notice:box(document.querySelector('.live-roster-notice')),candidateCount:[...document.querySelectorAll('.lane-matchup-option')].filter(e=>getComputedStyle(e).display!=='none').length,overflow:document.documentElement.scrollWidth>innerWidth,cardOverflow:card.scrollWidth>card.clientWidth+1,iconWidths:[...card.querySelectorAll('.game-icon')].map(e=>e.getBoundingClientRect().width)};})()`);
    if (!before) {
      assert.equal(m.overflow, false, JSON.stringify({width, ...m})); assert.equal(m.cardOverflow, false, JSON.stringify({width, ...m}));
      assert.equal(m.card.h, m.tabs.h, 'bar and tabs have equal height');
      assert.ok(m.iconWidths.filter(w => w > 0).every(w => w === 22));
      assert.equal(m.candidateCount, width - 48 <= 700 ? 2 : 3);
      if (width - 48 <= 700) {assert.ok(m.slot.y >= m.tabs.y + m.tabs.h); assert.equal(m.slot.w, width - 48);}
      else {assert.equal(m.slot.y, m.tabs.y, JSON.stringify({width, ...m})); assert.ok(m.slot.x > m.tabs.x);}
      assert.ok(m.notice.y >= m.slot.y);
    }
    if (width === 1280 || width === 620) await shot(`${before ? 'before' : 'after'}-lane-${width === 1280 ? 'wide' : 'narrow'}.png`);
    measures.push({width, ...m});
  }
  await call('Emulation.setDeviceMetricsOverride', {width: 640, height: 300, deviceScaleFactor: 1, mobile: false});
  await evaluate(`document.body.dataset.shot='tooltip';document.querySelector('[data-tooltip-roster]').dispatchEvent(new Event('pointerover',{bubbles:true}));`);
  await evaluate(`Promise.all([...document.querySelectorAll('#global-tooltip img[data-queued-src]')].map(async img=>{img.src=img.dataset.queuedSrc;await img.decode();img.dataset.imageReady='true';}))`);
  await frame();
  const tooltip = await evaluate(`(()=>{const root=document.querySelector('#global-tooltip');return {text:root.textContent,images:root.querySelectorAll('img').length,rows:root.querySelectorAll('.tooltip-roster-player').length,heroNames:[...root.querySelectorAll('.tooltip-roster-champion')].map(e=>e.textContent)};})()`);
  if (!before) {assert.equal(tooltip.images, 0); assert.equal(tooltip.rows, 2); assert.equal(tooltip.heroNames[1], ''); assert.doesNotMatch(tooltip.text, /未知英雄|·/);}
  await shot(`${before ? 'before' : 'after'}-premade-tooltip.png`);
  fs.writeFileSync(path.join(output, `${before ? 'before' : 'after'}-layout.json`), JSON.stringify({measures, tooltip}, null, 2));
  console.log(`R188 ${before ? 'before screenshots' : 'Chromium layout assertions'} PASS`);
}
main().catch(error => {console.error(error); process.exitCode = 1;}).finally(() => {ws?.close(); proc?.kill(); server?.close(); fs.rmSync(temp, {recursive: true, force: true, maxRetries: 8, retryDelay: 100});});
