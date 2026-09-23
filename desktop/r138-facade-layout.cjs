// R138: real Chromium comparison of icon spacing and the detail image size.
// Run: node desktop/r138-facade-layout.cjs
"use strict";
const { spawn } = require("node:child_process");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const os = require("node:os");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const web = path.join(root, "backend", "web");
const output = process.env.R138_LAYOUT_OUTPUT || path.join(root, "docs", "r138-validation");
const chrome = process.env.CHROME_BIN || (process.platform === "darwin" ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" : "chromium");
const source = fs.readFileSync(path.join(web, "favorites-facade.js"), "utf8");
const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r138-facade-"));
let browser, socket, server;

function functionSource(name) {
  const start = source.indexOf("function " + name + "(");
  assert.ok(start >= 0, name + " missing");
  const bodyStart = source.indexOf("{", start);
  let depth = 0, quote = "", escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const character = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === quote) quote = "";
      continue;
    }
    if (character === "'" || character === '"' || character.charCodeAt(0) === 96) {
      quote = character;
      continue;
    }
    if (character === "{") depth += 1;
    if (character === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(name + " braces unbalanced");
}

const constants = ["DETAIL_ART_ICON_SCALE", "DETAIL_ART_ICON_MAX_PX"].map((name) => {
  const declaration = source.match(new RegExp("const " + name + " = \\d+;"))?.[0];
  assert.ok(declaration, name + " missing");
  return declaration;
}).join("\n");
const sizingScript = "(function() { const el = { dialog: document.getElementById('facade-detail-dialog'), dialogImage: document.getElementById('facade-detail-image') }; const state = {view:'icons'}; " +
  constants + "\n" + functionSource("setDetailArtSize") + "\n" + functionSource("applyDetailArtSize") +
  "\nwindow.r138ApplyDetailArtSize = applyDetailArtSize; })();";

const svg = "data:image/svg+xml," + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128"><rect width="128" height="128" rx="18" fill="#28465f"/><circle cx="64" cy="47" r="25" fill="#cfae7b"/><path d="M25 119c5-30 25-44 39-44s34 14 39 44" fill="#99bed3"/><path d="M27 25h74M30 97h68" stroke="#e4c581" stroke-width="6"/></svg>');
const cards = Array.from({ length: 16 }, (_, index) => '<button class="skin-card" type="button"><span class="skin-art"><img class="is-loaded" src="' + svg + '" alt=""></span><span class="skin-copy"><strong>头像 ' + (index + 1) + '</strong></span></button>').join("");
const page = '<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/gameplay.css"><style>' +
  'body{margin:0;padding:32px;background:var(--bg)}#fixture{margin-left:210px;max-width:1050px}.facade-grid.is-icons{margin-top:20px}' +
  'body.is-baseline .facade-grid.is-icons{gap:12px 10px}.facade-detail-dialog .dialog-copy{padding:20px}' +
  '</style></head><body><main id="fixture"><h1>头像与旗帜</h1><h2>头像</h2><div id="facade-grid" class="skin-grid facade-grid is-icons">' +
  cards + '</div></main><dialog id="facade-detail-dialog" class="skin-dialog facade-detail-dialog"><div class="dialog-art">' +
  '<img id="facade-detail-image" class="dialog-art-primary" src="' + svg + '" alt=""></div><div class="dialog-copy"><h2>鹰啸！图标</h2><p>头像详情</p></div></dialog></body></html>';

async function main() {
  fs.mkdirSync(output, { recursive: true });
  server = http.createServer((request, response) => {
    if (request.url === "/") { response.setHeader("Content-Type", "text/html"); response.end(page); return; }
    if (request.url === "/app.css" || request.url === "/gameplay.css") {
      response.setHeader("Content-Type", "text/css");
      response.end(fs.readFileSync(path.join(web, request.url.slice(1))));
      return;
    }
    response.statusCode = 404; response.end();
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  browser = spawn(chrome, ["--headless=new", "--no-first-run", "--no-default-browser-check", "--remote-debugging-port=0", "--user-data-dir=" + temp, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
  const url = await new Promise((resolve, reject) => {
    let stderr = "";
    const timer = setTimeout(() => reject(new Error("Chrome startup timeout")), 20000);
    browser.once("error", reject);
    browser.stderr.on("data", (bytes) => {
      stderr += bytes;
      const found = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (found) { clearTimeout(timer); resolve(found[1]); }
    });
    browser.once("exit", (code) => reject(new Error("Chrome exited " + code + ": " + stderr.slice(-500))));
  });
  socket = new WebSocket(url);
  await new Promise((resolve, reject) => { socket.addEventListener("open", resolve, { once: true }); socket.addEventListener("error", reject, { once: true }); });
  let sequence = 0;
  const pending = new Map();
  socket.addEventListener("message", (event) => {
    const message = JSON.parse(event.data);
    if (!pending.has(message.id)) return;
    const [resolve, reject] = pending.get(message.id);
    pending.delete(message.id);
    message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result);
  });
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const id = ++sequence;
    pending.set(id, [resolve, reject]);
    socket.send(JSON.stringify({ id, method, params, sessionId }));
  });
  const { targetId } = await send("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
  const call = (method, params = {}) => send(method, params, sessionId);
  const evaluate = async (expression) => {
    const result = await call("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
    if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
    return result.result.value;
  };
  const screenshot = async (name) => {
    const result = await call("Page.captureScreenshot", { format: "png" });
    fs.writeFileSync(path.join(output, name), Buffer.from(result.data, "base64"));
  };
  await call("Page.enable"); await call("Runtime.enable");
  await call("Page.navigate", { url: "http://127.0.0.1:" + server.address().port + "/" });
  await evaluate("document.fonts.ready.then(() => true)");
  await evaluate(sizingScript);
  await evaluate("document.getElementById('facade-detail-image').decode().then(() => true)");
  for (const width of [1200, 960]) {
    await call("Emulation.setDeviceMetricsOverride", { width, height: 900, deviceScaleFactor: 1, mobile: false });
    const measure = () => evaluate("(() => {const cards=[...document.querySelectorAll('#facade-grid .skin-card')];const a=cards[0].getBoundingClientRect(),b=cards[1].getBoundingClientRect();const nextRow=cards.map(card=>card.getBoundingClientRect()).find(rect=>rect.top>a.top+1);const art=document.querySelector('.facade-detail-dialog .dialog-art');return {columnGap:b.left-a.right,rowGap:nextRow.top-a.bottom,tileWidth:a.width,artWidth:art.getBoundingClientRect().width,naturalWidth:document.getElementById('facade-detail-image').naturalWidth};})()");
    await evaluate("document.body.classList.add('is-baseline');document.getElementById('facade-detail-dialog').style.setProperty('--facade-art-width','128px');document.getElementById('facade-detail-dialog').style.setProperty('--facade-art-height','128px');");
    const before = await measure();
    await screenshot("before-grid-" + width + ".png");
    await evaluate("document.getElementById('facade-detail-dialog').showModal()");
    const beforeDialog = await measure();
    await screenshot("before-detail-" + width + ".png");
    await evaluate("document.getElementById('facade-detail-dialog').close();document.body.classList.remove('is-baseline');window.r138ApplyDetailArtSize()");
    const after = await measure();
    await screenshot("after-grid-" + width + ".png");
    await evaluate("document.getElementById('facade-detail-dialog').showModal()");
    const afterDialog = await measure();
    await screenshot("after-detail-" + width + ".png");
    await evaluate("document.getElementById('facade-detail-dialog').close()");
    assert.equal(before.columnGap, 10, "baseline column gap");
    assert.equal(before.rowGap, 12, "baseline row gap");
    assert.ok(after.columnGap >= 14 && after.columnGap > before.columnGap, width + " icon column gap " + JSON.stringify({ before, after }));
    assert.ok(after.rowGap >= 18 && after.rowGap > before.rowGap, width + " icon row gap " + JSON.stringify({ before, after }));
    assert.equal(after.tileWidth, 92, "icon tile width must stay 92px");
    assert.equal(after.naturalWidth, 128, "fixture image must decode at 128px");
    assert.ok(afterDialog.artWidth >= 255 && afterDialog.artWidth <= 257, width + " detail art width " + afterDialog.artWidth);
    assert.ok(afterDialog.artWidth > beforeDialog.artWidth + 100, width + " detail art did not grow");
    console.log("R138 Chromium", width, JSON.stringify({ before, after, beforeDialog, afterDialog }));
  }
  const playerStyle = await evaluate("(() => {const section=document.createElement('section');section.className='match-detail';section.innerHTML='<button class=\"participant-link\"><span class=\"participant-name\">队友</span></button><button class=\"participant-link\"><span class=\"participant-name is-current-player\">当前玩家</span></button>';document.body.append(section);const names=section.querySelectorAll('.participant-name');const other=getComputedStyle(names[0]),self=getComputedStyle(names[1]);return {otherColor:other.color,selfColor:self.color,selfWeight:self.fontWeight};})()");
  assert.notEqual(playerStyle.selfColor, playerStyle.otherColor, "current player must use the theme color");
  assert.ok(Number(playerStyle.selfWeight) >= 700, "current player name must be bold");
  console.log("R138 current player Chromium", JSON.stringify(playerStyle));
  console.log("R138 Chromium PASS");
}

async function cleanup() {
  try { socket?.close(); } catch (_) {}
  if (browser && browser.exitCode === null && browser.signalCode === null) {
    const exited = new Promise((resolve) => {
      const timer = setTimeout(resolve, 5000);
      browser.once("exit", () => { clearTimeout(timer); resolve(); });
    });
    browser.kill();
    await exited;
  }
  try { server?.closeAllConnections(); } catch (_) {}
  await new Promise((resolve) => {
    if (!server) return resolve();
    const timer = setTimeout(resolve, 2000);
    server.close(() => { clearTimeout(timer); resolve(); });
  });
  fs.rmSync(temp, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 });
}

main().catch((error) => { console.error(error); process.exitCode = 1; }).finally(() => cleanup().catch((error) => console.error("cleanup warning:", error)));
