'use strict';
// Demo screenshots use the actual loot renderer/CSS; emote artwork/name are fixtures.
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const {read, extract, compile, escapeHTML} = require('../backend/web/r188-harness.cjs');
const directory = process.env.R189_SOURCE_DIR;
const before = process.env.R189_BEFORE === '1';
const output = process.env.R189_SHOTS || path.resolve(__dirname, '../docs/history/reports/r189');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'r189-layout-'));
let proc, ws, server;
const requestedImages = [];
async function main() {
  const source = read('app.js', directory);
  const f = compile(source, ['lootToken', 'lootName', 'lootNamePending', 'lootTypeLabel', 'lootImagePaths', 'lootCategorySlug', 'lootCategoryIcon', 'lootCard'], {escapeHTML, formatNumber: String});
  const item = {lootId: 'EMOTE_1468', lootName: 'EMOTE_1468', displayName: before ? 'EMOTE_1468' : '演示表情（测试夹具）', category: '表情', type: 'EMOTE', itemStatus: 'OWNED', ownedKnown: true, owned: true, dataPending: false, count: 1,
    asset: before ? '' : '/lol-game-data/assets/ASSETS/Loadouts/SummonerEmotes/fixture.png'};
  server = require('node:http').createServer((req, res) => {
    if (req.url.startsWith('/api/image?')) {
      const imagePath = new URL(req.url, 'http://localhost').searchParams.get('path');
      requestedImages.push(imagePath);
      if (imagePath !== item.asset) {res.statusCode = 404; res.end(); return;}
      res.setHeader('Content-Type', 'image/svg+xml');
      res.end('<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128"><circle cx="64" cy="64" r="43" fill="#f1c452" stroke="#7e571d" stroke-width="4"/><ellipse cx="48" cy="54" rx="5" ry="8" fill="#594122"/><ellipse cx="80" cy="54" rx="5" ry="8" fill="#594122"/><path d="M43 76Q64 99 85 76" fill="none" stroke="#594122" stroke-width="5" stroke-linecap="round"/></svg>'); return;
    }
    res.setHeader('Content-Type', 'text/html');
    res.end(`<!doctype html><html data-theme="light"><meta charset="utf-8"><style>${read('app.css', directory)}</style><style>body{margin:0;padding:24px;overflow:auto}.loot-grid{display:grid;grid-template-columns:minmax(0,1fr)}</style><body><section class="loot-grid">${f.lootCard(item)}</section><script>const LOOT_ICON_BOX=70,LOOT_ICON_TARGET=60,LOOT_ICON_MAX_SIDE=78;${['lootIconFit','normalizeLootIcon','loadNextLootImage'].map(n=>extract(source,n)).join('\n')}</script></body></html>`);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const chrome = process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
  proc = spawn(chrome, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', `--user-data-dir=${temp}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
  const url = await new Promise((resolve, reject) => {
    let text = ''; const timer = setTimeout(() => reject(Error('Chrome startup timeout')), 20000);
    proc.once('error', reject); proc.stderr.on('data', chunk => {text += chunk; const m = text.match(/DevTools listening on (ws:\/\/[^\s]+)/); if (m) {clearTimeout(timer); resolve(m[1]);}});
    proc.once('exit', code => reject(Error(`Chrome exited ${code}: ${text.slice(-800)}`)));
  });
  ws = new WebSocket(url); await new Promise((resolve, reject) => {ws.addEventListener('open', resolve, {once: true}); ws.addEventListener('error', reject, {once: true});});
  let seq = 0; const pending = new Map();
  ws.addEventListener('message', event => {const msg = JSON.parse(event.data); if (pending.has(msg.id)) {const [resolve, reject] = pending.get(msg.id); pending.delete(msg.id); msg.error ? reject(Error(JSON.stringify(msg.error))) : resolve(msg.result);}});
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {const id = ++seq; pending.set(id, [resolve, reject]); ws.send(JSON.stringify({id, method, params, sessionId}));});
  const {targetId} = await send('Target.createTarget', {url: 'about:blank'});
  const {sessionId} = await send('Target.attachToTarget', {targetId, flatten: true});
  const call = (method, params = {}) => send(method, params, sessionId);
  const evaluate = async expression => {const r = await call('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true}); if (r.exceptionDetails) throw Error(JSON.stringify(r.exceptionDetails)); return r.result.value;};
  await call('Page.enable'); await call('Emulation.setDeviceMetricsOverride', {width: 560, height: 160, deviceScaleFactor: 1, mobile: false});
  await call('Page.navigate', {url: `http://127.0.0.1:${server.address().port}`});
  await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{if(document.querySelector('.loot-card')&&typeof loadNextLootImage==='function'){clearInterval(timer);resolve();}},20);})`);
  await evaluate(`Promise.all([...document.querySelectorAll('.loot-art img')].map(async img=>{loadNextLootImage(img,true);img.src=img.dataset.queuedSrc;await img.decode();img.dataset.imageReady='true';}))`);
  await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const m = await evaluate(`(()=>{const root=document.querySelector('.loot-card');return {title:root.querySelector('strong').textContent,hint:root.querySelector('.loot-name-pending')?.textContent||'',ownership:root.querySelector('.loot-ownership')?.textContent||'',imageLoaded:!!root.querySelector('.loot-art.has-image'),overflow:root.scrollWidth>root.clientWidth+1};})()`);
  assert.equal(m.overflow, false);
  if (before) {assert.equal(m.title, 'EMOTE_1468'); assert.ok(m.hint); assert.equal(m.ownership, '');}
  else {assert.equal(m.title, item.displayName); assert.equal(m.hint, ''); assert.equal(m.ownership, '已拥有'); assert.equal(m.imageLoaded, true);}
  assert.deepEqual(requestedImages, before ? [] : [item.asset]);
  m.requestedImages = requestedImages;
  fs.mkdirSync(output, {recursive: true});
  const shot = await call('Page.captureScreenshot', {format: 'png'});
  fs.writeFileSync(path.join(output, `${before ? 'before' : 'after'}-emote.png`), Buffer.from(shot.data, 'base64'));
  fs.writeFileSync(path.join(output, `${before ? 'before' : 'after'}-emote.json`), JSON.stringify(m, null, 2));
  console.log(`R189 ${before ? 'before' : 'after'} emote screenshot PASS`);
}
main().catch(error => {console.error(error); process.exitCode = 1;}).finally(() => {ws?.close(); proc?.kill(); server?.close(); fs.rmSync(temp, {recursive: true, force: true, maxRetries: 8, retryDelay: 100});});
