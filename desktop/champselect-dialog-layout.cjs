'use strict';
// Real Chromium input/scroll acceptance for R87 P9. Optional R87_DIALOG_MUTANT=1.
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const assert = require('node:assert/strict');
const web = path.resolve(__dirname, '../backend/web');
const output = path.resolve(__dirname, '../docs/r87/addendum');
const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'r87-dialog-'));
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
    if (pathname === '/suite.js' && process.env.R87_DIALOG_MUTANT) {
      const before = content.toString();
      content = before.replace('if (existing) {\n      // Runtime events', 'if (existing) { existing.remove(); return syncChampSelectDialog();\n      // Runtime events');
      assert.notEqual(content, before, 'mutation must apply');
    }
    res.end(content);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  await call('Page.enable');
  await call('Emulation.setDeviceMetricsOverride', { width: 480, height: 700, deviceScaleFactor: 1, mobile: false });
  await call('Page.addScriptToEvaluateOnNewDocument', { source: "localStorage.setItem('lol-loot-ui-scale','1')" });
  await call('Page.navigate', { url: `http://127.0.0.1:${server.address().port}/?demo` });
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve()}},30);setTimeout(()=>{clearInterval(timer);reject(Error('app missing'))},8000)})`);
  await evaluate(`document.querySelector('[data-section="suite"]').click();document.querySelector('[data-suite-tab="champselect"]').click()`);
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const button=document.querySelector('[data-cs-open-dialog="pick"]');if(button){clearInterval(timer);button.click();resolve()}},30);setTimeout(()=>{clearInterval(timer);reject(Error('champselect missing'))},8000)})`);
  await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const before = await evaluate(`(()=>{const grid=document.querySelector('.cs-champion-grid');grid.scrollTop=200;return {scroll:grid.scrollTop,contentHeight:grid.scrollHeight,height:grid.clientHeight}})()`);
  assert.ok(before.scroll > 0, 'fixture must have real scroll range');
  const external = async () => {
    await evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:gameflow',{detail:{phase:'ChampSelect',changed:true}}))`);
    await evaluate('new Promise(r=>setTimeout(r,120))');
  };
  await external();
  const after = await evaluate("document.querySelector('.cs-champion-grid').scrollTop");
  await evaluate("document.querySelector('[data-cs-dialog-search]').focus()");
  for (let index = 0; index < 6; index++) {
    if (index < 4) await call('Input.insertText', { text: 'ahri'[index] });
    await external();
  }
  const typed = await evaluate(`({value:document.querySelector('[data-cs-dialog-search]').value,focused:document.activeElement===document.querySelector('[data-cs-dialog-search]'),open:document.querySelector('.cs-dialog').open})`);
  const tag = process.env.R87_DIALOG_MUTANT ? 'dialog-mutant' : 'dialog-baseline';
  fs.mkdirSync(output, { recursive: true });
  fs.writeFileSync(path.join(output, tag + '.json'), JSON.stringify({ viewport: [480, 700], before, after, typed }, null, 2));
  const screenshot = await call('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(path.join(output, tag + '.png'), Buffer.from(screenshot.data, 'base64'));
  assert.equal(after, before.scroll, 'external event reset real scroll');
  assert.equal(typed.value, 'ahri', 'external events lost typed text');
  assert.equal(typed.focused, true, 'external events lost search focus');
  assert.equal(typed.open, true);
  process.stdout.write('R87 P9 Chromium scroll/input PASS\n');
}
main().catch(error => { process.stderr.write(String(error.stack) + '\n'); process.exitCode = 1; }).finally(() => {
  socket?.close(); chrome?.kill(); server?.close();
  fs.rmSync(profile, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 });
});
