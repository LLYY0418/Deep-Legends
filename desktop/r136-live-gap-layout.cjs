// Chromium layout guard for R136 P4/P5. Run: node desktop/r136-live-gap-layout.cjs
"use strict";
const { spawn } = require("node:child_process");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const os = require("node:os");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const web = path.join(root, "backend", "web");
const output = process.env.R136_LAYOUT_OUTPUT || path.join(root, "docs", "r136-validation", "live-gap");
const chrome = process.env.CHROME_BIN || (process.platform === "darwin" ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" : "chromium");
const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r136-layout-"));
let processChrome, socket, server;

const tabRow = '<section class="recommendation-area"><div class="recommendation-tab-row"><div class="recommendation-tabs"><button class="is-active">海克斯与出装</button><button>详情</button></div></div></section>';
const variants = {
  classic: `<div data-live-body>${tabRow}</div>`,
  aram: `<div data-live-body>${tabRow}</div>`,
  mayhem: `<div data-live-body>${tabRow}</div>`,
  arena: `<div data-live-body><p class="arena-my-squad-notice">斗魂提示</p>${tabRow}</div>`,
  waiting: tabRow,
  unsupported: '<div class="gameplay-empty"><strong>暂不支持</strong></div>',
  failed: '<div class="gameplay-empty"><strong>实时对局读取失败</strong></div>',
  awaiting: '<div role="status"><div class="gameplay-empty"><strong>正在识别新对局</strong></div></div>',
};

function page() {
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/gameplay.css"><link rel="stylesheet" href="/champions.css"><style>html,body{margin:0}#fixture{margin-left:226px;padding:24px;min-width:0}#live-panel{display:block;min-height:0}.mayhem-detail-pane{margin-top:24px}.mayhem-performance{background:var(--surface);padding:14px}</style></head><body><main id="fixture"><section id="live-panel" class="panel gameplay-panel"><div class="live-toolbar"><div id="live-session-summary" class="live-session-summary"><span class="state-chip">英雄选择</span><div class="live-session-copy"><strong>海克斯大乱斗</strong><span>对局数据已就绪</span></div></div><div data-live-status aria-live="polite"></div><button id="live-refresh" class="text-button">刷新对局</button></div><div id="live-content" class="gameplay-content"></div></section><div class="mayhem-detail-pane"><section class="recommendation-section mayhem-performance"><header><h3>表现指标</h3><span class="section-count">对比 123 位英雄</span></header><div class="mayhem-performance-groups"><section class="mayhem-performance-group"><h4>战斗表现</h4><dl><div><dt>伤害</dt><dd><b>12345</b><span class="mayhem-delta">较平均 +12%</span></dd></div><div class="is-cumulative"><dt>多杀累计</dt><dd class="mayhem-multikill-values"><span><small>双杀</small><b>1,234,567</b></span><span><small>三杀</small><b>1,234,567</b></span><span><small>四杀</small><b>1,234,567</b></span><span><small>五杀</small><b>1,234,567</b></span></dd></div></dl></section><section class="mayhem-performance-group"><h4>参与表现</h4><dl><div><dt>参团率</dt><dd><b>64%</b></dd></div></dl></section></div><p class="mayhem-performance-note">双杀/三杀/四杀/五杀为累计次数，受出场场次影响，不可跨英雄直接比较。</p></section></div></main></body></html>`;
}

async function main() {
  fs.mkdirSync(output, { recursive: true });
  server = http.createServer((request, response) => {
    const name = new URL(request.url, "http://localhost").pathname;
    if (name === "/") { response.setHeader("Content-Type", "text/html"); response.end(page()); return; }
    const file = name === "/gameplay.css" && process.env.R136_GAMEPLAY_CSS
      ? path.resolve(process.env.R136_GAMEPLAY_CSS) : path.resolve(web, `.${name}`);
    if (!(file.startsWith(`${web}${path.sep}`) || name === "/gameplay.css" && process.env.R136_GAMEPLAY_CSS) || !fs.existsSync(file)) { response.statusCode = 404; response.end(); return; }
    response.setHeader("Content-Type", "text/css"); response.end(fs.readFileSync(file));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  processChrome = spawn(chrome, ["--headless=new", "--no-first-run", "--no-default-browser-check", "--remote-debugging-port=0", `--user-data-dir=${temp}`, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
  const url = await new Promise((resolve, reject) => {
    let stderr = "";
    const timer = setTimeout(() => reject(new Error("Chrome startup timeout")), 20000);
    processChrome.once("error", reject);
    processChrome.stderr.on("data", (bytes) => { stderr += bytes; const found = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/); if (found) { clearTimeout(timer); resolve(found[1]); } });
    processChrome.once("exit", (code) => reject(new Error(`Chrome exited ${code}: ${stderr.slice(-500)}`)));
  });
  socket = new WebSocket(url);
  await new Promise((resolve, reject) => { socket.addEventListener("open", resolve, { once: true }); socket.addEventListener("error", reject, { once: true }); });
  let sequence = 0;
  const pending = new Map();
  socket.addEventListener("message", (event) => { const message = JSON.parse(event.data); if (!pending.has(message.id)) return; const [resolve, reject] = pending.get(message.id); pending.delete(message.id); message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result); });
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => { const id = ++sequence; pending.set(id, [resolve, reject]); socket.send(JSON.stringify({ id, method, params, sessionId })); });
  const { targetId } = await send("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
  const call = (method, params = {}) => send(method, params, sessionId);
  const evaluate = async (expression) => { const result = await call("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true }); if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails)); return result.result.value; };
  await call("Page.enable"); await call("Runtime.enable");
  await call("Page.navigate", { url: `http://127.0.0.1:${server.address().port}/` });
  await evaluate("document.fonts.ready.then(() => true)");
  for (const width of [1200, 960]) {
    await call("Emulation.setDeviceMetricsOverride", { width, height: 900, deviceScaleFactor: 1, mobile: false });
    for (const [mode, markup] of Object.entries(variants)) {
      const positions = [];
      for (const withStatus of [false, true]) {
        const status = withStatus ? '<div class="live-refresh-status" role="status"><span>数据尚未完整，已停止自动重试，可手动刷新</span></div>' : "";
        const content = process.env.R136_STATUS_IN_CONTENT === "1" ? `${status}${markup}` : markup;
        const toolbarStatus = process.env.R136_STATUS_IN_CONTENT === "1" ? "" : status;
        await evaluate(`(() => {document.querySelector('#live-content').innerHTML=${JSON.stringify(content)};document.querySelector('[data-live-status]').innerHTML=${JSON.stringify(toolbarStatus)};})()`);
        const result = await evaluate(`(() => {const bar=document.querySelector('.live-toolbar').getBoundingClientRect();const root=document.querySelector('#live-content');const first=root.querySelector('[data-live-body] > :first-child')||root.firstElementChild;const rect=first.getBoundingClientRect();const tabs=root.querySelector('.recommendation-tab-row');return {gap:rect.top-bar.bottom,y:tabs?.getBoundingClientRect().top??null,toolbarHeight:bar.height};})()`);
        assert.ok(result.gap >= 9.5 && result.gap <= 12.5, `${width} ${mode} status=${withStatus}: ${JSON.stringify(result)}`);
        positions.push(result);
        const screenshot = await call("Page.captureScreenshot", { format: "png" });
        fs.writeFileSync(path.join(output, `${mode}-${width}-${withStatus ? "status" : "plain"}.png`), Buffer.from(screenshot.data, "base64"));
      }
      if (positions[0].y !== null) assert.ok(Math.abs(positions[0].y - positions[1].y) < 0.5, `${width} ${mode} status shifted tabs: ${JSON.stringify(positions)}`);
      console.log(width, mode, positions.map((item) => item.gap));
    }
    const fonts = await evaluate(`(() => {const panel=document.querySelector('.mayhem-performance');const values=[...panel.querySelectorAll('.mayhem-multikill-values b')];return {clipped:values.map(el=>el.scrollWidth>el.clientWidth+0.5),fonts:[...panel.querySelectorAll('h4,dt,dd,.mayhem-delta,.mayhem-multikill-values small,.mayhem-multikill-values b,.mayhem-performance-note')].map(el=>getComputedStyle(el).fontSize)};})()`);
    assert.ok(fonts.clipped.every((value) => !value), `${width} multikill clipped: ${JSON.stringify(fonts)}`);
    const screenshot = await call("Page.captureScreenshot", { format: "png" });
    fs.writeFileSync(path.join(output, `mayhem-font-${width}.png`), Buffer.from(screenshot.data, "base64"));
    console.log(width, "mayhem-font", fonts);
  }
}

main().catch((error) => { console.error(error); process.exitCode = 1; }).finally(() => { socket?.close(); processChrome?.kill(); server?.close(); fs.rmSync(temp, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 }); });
