'use strict';
// Real Chromium acceptance for R99 picker and R101 ownership/left-column layout.
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const assert = require('node:assert/strict');
const web = path.resolve(__dirname, '../web');
const output = process.env.R99_BROWSER_OUTPUT || path.resolve(__dirname, '../docs/r99-validation/browser');
const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'r99-picker-'));
let chrome, socket, server;

async function main() {
  const binary = process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
  chrome = spawn(binary, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], { stdio: ['ignore', 'ignore', 'pipe'] });
  const address = await new Promise((resolve, reject) => {
    let log = '';
    const timeout = setTimeout(() => reject(Error('Chrome startup timeout')), 20000);
    chrome.once('error', reject);
    chrome.stderr.on('data', data => {
      log += data;
      const match = log.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (match) { clearTimeout(timeout); resolve(match[1]); }
    });
  });
  socket = new WebSocket(address);
  await new Promise((resolve, reject) => { socket.addEventListener('open', resolve, { once: true }); socket.addEventListener('error', reject, { once: true }); });
  let sequence = 0;
  const pending = new Map();
  socket.addEventListener('message', event => {
    const message = JSON.parse(event.data), callback = pending.get(message.id);
    if (!callback) return;
    pending.delete(message.id);
    message.error ? callback.reject(Error(JSON.stringify(message.error))) : callback.resolve(message.result);
  });
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const id = ++sequence;
    pending.set(id, { resolve, reject });
    socket.send(JSON.stringify({ id, method, params, sessionId }));
  });
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
  const call = (method, params) => send(method, params, sessionId);
  const evaluate = async expression => {
    const response = await call('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (response.exceptionDetails) throw Error(JSON.stringify(response.exceptionDetails));
    return response.result.value;
  };
  server = require('node:http').createServer((req, res) => {
    const pathname = new URL(req.url, 'http://localhost').pathname;
    const file = path.resolve(web, pathname === '/' ? 'index.html' : '.' + pathname);
    if (!file.startsWith(web + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) { res.writeHead(404); res.end(); return; }
    res.setHeader('Content-Type', { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' }[path.extname(file)] || 'application/octet-stream');
    let content = fs.readFileSync(file);
    if (process.env.R99_SUITE_SOURCE && pathname === '/suite.js') content = fs.readFileSync(process.env.R99_SUITE_SOURCE);
    if (process.env.R99_CSS_SOURCE && pathname === '/suite.css') content = fs.readFileSync(process.env.R99_CSS_SOURCE);
    res.end(content);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  await call('Page.enable');

  await call('Page.addScriptToEvaluateOnNewDocument', { source: "localStorage.setItem('lol-loot-ui-scale','1')" });
  await call('Page.navigate', { url: `http://127.0.0.1:${server.address().port}/?demo` });
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve()}},30);setTimeout(()=>{clearInterval(timer);reject(Error('app missing'))},8000)})`);
  await evaluate(`document.querySelector('[data-section="suite"]').click();document.querySelector('[data-suite-tab="facade"]').click()`);
  const samples = []; fs.mkdirSync(output, { recursive: true });
  for (const width of [1500, 1180, 980]) for (const zoom of [1, 1.25]) {
    await call('Emulation.setDeviceMetricsOverride', { width, height: 900, deviceScaleFactor: 1, mobile: false });
    await evaluate(`document.documentElement.style.setProperty('--ui-zoom', '${zoom}')`);
    await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const b=document.querySelector('[data-facade-icons]');if(b){clearInterval(timer);b.click();resolve()}},30);setTimeout(()=>{clearInterval(timer);reject(Error('facade missing'))},8000)})`);
    const ownership = await evaluate(`new Promise((resolve,reject)=>{const start=Date.now();const poll=()=>{const toggle=document.querySelector('[data-picker-owned]');if(toggle){const checked=toggle.checked;toggle.checked=false;toggle.dispatchEvent(new Event('change'));return resolve(checked)};if(Date.now()-start>8000)return reject(Error('ownership missing'));setTimeout(poll,30)};poll()})`);
    assert.equal(ownership, true, 'owned-only must default on');
    await evaluate(`new Promise((resolve,reject)=>{const start=Date.now();const poll=()=>{if(document.querySelectorAll('[data-picker-icon]').length===5099)return resolve();if(Date.now()-start>15000)return reject(Error('grid incomplete'));setTimeout(poll,40)};poll()})`);
    const result = await evaluate(`(()=>{
      const d=document.querySelector('.facade-picker'),r=d.getBoundingClientRect();
      const side=d.querySelector('.facade-picker-side'),grid=d.querySelector('.facade-picker-scroll'),first=d.querySelector('.facade-icon-picture'),img=first.querySelector('img');
      side.scrollTop=100;grid.scrollTop=100;
      return {parent:d.parentElement.tagName, x:r.x,y:r.y,width:r.width,height:r.height,dialogScroll:d.scrollHeight-d.clientHeight,sideScroll:side.scrollTop,gridScroll:grid.scrollTop,cell:first.clientWidth,pictureHeight:first.clientHeight,aspect:getComputedStyle(first).aspectRatio,display:getComputedStyle(img).display,unownedDisabled:[...d.querySelectorAll('.is-unowned')].every(e=>e.disabled),unownedCount:d.querySelectorAll('.is-unowned').length,styles:d.querySelectorAll('[style]').length,footer:[...d.querySelectorAll('.facade-picker-selection strong,[data-picker-apply]')].map(e=>({clipped:e.scrollWidth-e.clientWidth>1&&getComputedStyle(e).overflowX==='hidden'}))};
    })()`);
    samples.push({ viewport: width, zoom, ...result });
    assert.equal(result.parent, 'BODY');
    assert.ok(result.unownedCount > 0 && result.unownedDisabled, 'unowned icons must be disabled');
    assert.ok(result.x >= 0 && result.y >= 0 && result.x + result.width <= width + 1 && result.y + result.height <= 901, 'dialog outside viewport');
    assert.ok(Math.abs(result.width - Math.min(1120 * zoom, width - 32)) < 2, 'double zoom');
    assert.ok(result.cell >= 40 && result.pictureHeight >= 40, 'collapsed icon');
    assert.notEqual(result.aspect, 'auto', 'lost square aspect ratio');
    assert.equal(result.display, 'block', 'lost block image');
    assert.ok(result.sideScroll > 0 && result.gridScroll > 0 && result.dialogScroll <= 1, 'scroll ownership');
    assert.ok(result.footer.every(item => !item.clipped), 'footer clipped');
    assert.equal(result.styles, 0, 'inline styles violate CSP');
    fs.mkdirSync(output, { recursive: true });
    const screenshot = await call('Page.captureScreenshot', { format: 'png' });
    fs.writeFileSync(path.join(output, `icons-${width}-${zoom}.png`), Buffer.from(screenshot.data, 'base64'));
    await evaluate(`document.querySelector('[data-picker-close]').click()`);
    await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
    const facade = await evaluate(`(()=>{
      const left=document.querySelector('.facade-left').getBoundingClientRect();
      const cards=[...document.querySelectorAll('.facade-icon-card,.facade-banner-card')].map(e=>{const r=e.getBoundingClientRect();return {left:r.left,right:r.right,parent:e.parentElement.className}});
      const segment=document.querySelector('.facade-banner-card .suite-segment');
      // A narrow pressure fixture makes the nowrap contract observable even when
      // the normal column happens to leave enough room for both short labels.
      const pressure=segment.cloneNode(true);pressure.style.width='110px';pressure.style.gridTemplateColumns='70px 34px';segment.parentElement.append(pressure);
      const pressureHeight=pressure.offsetHeight;pressure.remove();
      return {left:{left:left.left,right:left.right},cards,scroll:segment.scrollWidth,client:segment.clientWidth,height:segment.offsetHeight,pressureHeight,overflow:document.documentElement.scrollWidth-innerWidth};
    })()`);
    assert.ok(facade.cards.length===2 && facade.cards.every(c=>c.parent==='facade-left' && c.left>=facade.left.left-1 && c.right<=facade.left.right+1),'cards must be inside left column');
    assert.ok(facade.scroll<=facade.client && facade.height<=34 && facade.pressureHeight<=34,'rank segment must not overflow or wrap '+JSON.stringify({width,zoom,...facade}));
    assert.ok(facade.overflow<=1,'horizontal page overflow');
    assert.equal(await evaluate(`document.activeElement?.hasAttribute('data-facade-icons')`), true, 'focus restore');

    samples.at(-1).facade=facade;
    const pageShot=await call('Page.captureScreenshot',{format:'png'});
    fs.writeFileSync(path.join(output,`facade-${width}-${zoom}.png`),Buffer.from(pageShot.data,'base64'));
    await evaluate(`document.querySelector('[data-facade-banners]').click()`);
    await evaluate(`new Promise((resolve,reject)=>{const start=Date.now();const poll=()=>{if(document.querySelectorAll('[data-banner-id]').length===35)return resolve();if(Date.now()-start>8000)return reject(Error('banner catalog missing'));setTimeout(poll,30)};poll()})`);
    assert.equal(await evaluate(`document.querySelectorAll('[data-banner-id]:disabled').length`),31);
    await evaluate(`document.querySelector('[data-banner-close]').click()`);

  }
  fs.writeFileSync(path.join(output, 'result.json'), JSON.stringify({ samples, banner: '35 fixture entries; ownership read-only; write pending real W6' }, null, 2));
  process.stdout.write('R101 ownership and left-column Chromium layout PASS; banner write pending W6\n');
}
main().catch(error => { process.stderr.write(String(error.stack) + '\n'); process.exitCode = 1; }).finally(() => {
  socket?.close(); chrome?.kill(); server?.close();
  fs.rmSync(profile, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 });
});
