"use strict";
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// Real Chromium / actual client HTML and JS / synthetic local Go-state API.
// This does not claim a deployed license server or Windows acceptance.
const { spawn } = require("node:child_process"), fs = require("node:fs"), path = require("node:path"), os = require("node:os"), http = require("node:http"), assert = require("node:assert/strict");
const root = path.resolve(__dirname, ".."), web = path.join(root, "backend/web"), out = process.env.R232_BROWSER_OUTPUT || path.join(root, "docs/history/reports/r232");
const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r232-chromium-"));
let chrome, ws, server, origin, state = "LOCKED", generation = 1, business = 0;
const errors = [], checks = [], started = new Date().toISOString();
async function main() {
  fs.mkdirSync(out, { recursive: true });
  chrome = spawn(process.env.CHROME_BIN || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", ["--headless=new", "--disable-background-networking", "--disable-renderer-backgrounding", "--disable-background-timer-throttling", "--no-first-run", "--no-default-browser-check", "--remote-debugging-port=0", `--user-data-dir=${temp}`, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
  const endpoint = await new Promise((resolve, reject) => { let output = ""; const timer = setTimeout(() => reject(Error("Chrome startup timeout")), 20000); chrome.once("error", reject); chrome.stderr.on("data", chunk => { output += chunk; const match = output.match(/DevTools listening on (ws:\/\/[^\s]+)/); if (match) { clearTimeout(timer); resolve(match[1]); } }); });
  ws = new WebSocket(endpoint); await new Promise((resolve, reject) => { ws.addEventListener("open", resolve, { once: true }); ws.addEventListener("error", reject, { once: true }); });
  let sequence = 0; const pending = new Map();
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => { const id = ++sequence, timer = setTimeout(() => { pending.delete(id); reject(Error("CDP timeout " + method)); }, 30000); pending.set(id, [value => { clearTimeout(timer); resolve(value); }, error => { clearTimeout(timer); reject(error); }]); ws.send(JSON.stringify({ id, method, params, sessionId })); });
  ws.addEventListener("message", event => { const message = JSON.parse(event.data); if (pending.has(message.id)) { const [resolve, reject] = pending.get(message.id); pending.delete(message.id); message.error ? reject(Error(JSON.stringify(message.error))) : resolve(message.result); } if (message.method === "Runtime.exceptionThrown") errors.push(message.params.exceptionDetails); if (message.method === "Fetch.requestPaused") { const allowed = message.params.request.url.startsWith(origin + "/") || message.params.request.url.startsWith("data:"); void send(allowed ? "Fetch.continueRequest" : "Fetch.failRequest", { requestId: message.params.requestId, ...(allowed ? {} : { errorReason: "BlockedByClient" }) }, message.sessionId).catch(() => {}); } });
  server = http.createServer((request, response) => {
    const name = new URL(request.url, "http://fixture").pathname;
    const json = value => { response.setHeader("Content-Type", "application/json"); response.end(JSON.stringify(value)); };
    const snapshot = () => ({ state, generation, message: state === "REPLACED" ? "注册码已在其他设备使用或已被重置" : "请输入注册码激活软件" });
    if (name === "/api/license/status") { json(snapshot()); return; }
    if (name === "/api/license/activate") { state = "ACTIVE"; generation++; json(snapshot()); return; }
    if (name === "/api/privacy") { json({ licenseDisclosure: "测试授权数据隐私说明（合成夹具）" }); return; }
    if (name.startsWith("/api/diagnostics/") || name.startsWith("/api/update/")) { json({}); return; }
    if (name.startsWith("/api/")) {
      business++;
      if (state !== "ACTIVE") { response.statusCode = 403; response.end("软件尚未激活"); return; }
      if (name === "/api/events") { response.writeHead(200, { "Content-Type": "text/event-stream" }); response.write(": fixture\n\n"); return; }
      if (name === "/api/status") { json({ connected: false, installations: [] }); return; }
      if (name === "/api/champions/catalog") { json({ tiers: [], champions: [] }); return; }
      json({}); return;
    }
    const file = path.resolve(web, name === "/" ? "index.html" : "." + name);
    if (!file.startsWith(web + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) { response.statusCode = 404; response.end(); return; }
    response.setHeader("Content-Type", ({ ".js": "text/javascript", ".css": "text/css", ".html": "text/html", ".png": "image/png" })[path.extname(file)] || "application/octet-stream"); response.end(require(path.join(root,"desktop/license-render-fixture.cjs")).licenseFixtureHTML(fs.readFileSync(file)));
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve)); origin = "http://127.0.0.1:" + server.address().port;
  const { targetId } = await send("Target.createTarget", { url: "about:blank" }), { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
  const call = (method, params) => send(method, params, sessionId);
  const evaluate = async expression => { const value = await call("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true }); if (value.exceptionDetails) throw Error(JSON.stringify(value.exceptionDetails)); return value.result.value; };
  const until = async expression => { for (let n = 0; n < 200; n++) { if (await evaluate(expression)) return; await new Promise(resolve => setTimeout(resolve, 50)); } throw Error("timeout: " + expression); };
  await call("Page.enable"); await call("Runtime.enable"); await call("Fetch.enable", { patterns: [{ urlPattern: "*" }] });
  await call("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }); await call("Page.navigate", { url: origin });
  await until("window.deepLegendsLicense && document.documentElement.dataset.license==='locked'");
  await new Promise(resolve => setTimeout(resolve, 16000));
  assert.equal(await evaluate("!document.querySelector('#license-overlay').hidden && document.querySelector('#app-frame').hidden && document.querySelector('#app-frame').hasAttribute('inert')"), true); assert.equal(business, 0); checks.push("first frame and >15s remain locked; zero business requests");
  for (const theme of ["dark", "light"]) for (const zoom of [1, 1.5]) {
    await evaluate(`document.documentElement.dataset.theme='${theme}';document.documentElement.style.setProperty('--ui-zoom','${zoom}')`);
    assert.equal(await evaluate("(()=>{const r=document.querySelector('#license-form').getBoundingClientRect();return r.left>=0 && r.top>=0 && r.right<=innerWidth && r.bottom<=innerHeight})()"), true);
    const screenshot = await call("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(path.join(out, `${theme}-${zoom}-locked.png`), Buffer.from(screenshot.data, "base64"));
  }
  await evaluate("document.querySelector('#license-form a:last-child').focus()"); await call("Input.dispatchKeyEvent", { type: "keyDown", key: "Tab", code: "Tab", windowsVirtualKeyCode: 9 }); await call("Input.dispatchKeyEvent", { type: "keyUp", key: "Tab", code: "Tab", windowsVirtualKeyCode: 9 }); assert.equal(await evaluate("document.activeElement.id"), "license-code");
  await call("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 }); assert.equal(await evaluate("document.querySelector('#license-overlay').hidden"), false); checks.push("themes/zoom/Tab/Esc");
  await evaluate("document.querySelector('#license-privacy').click()"); await until("document.querySelector('#license-privacy-dialog').open"); await evaluate("document.querySelector('#license-privacy-dialog').close()");
  assert.equal(await evaluate("(async()=>{const layer=document.querySelector('#license-overlay');layer.remove();try{await fetch('/api/gameplay/live');return false}catch(e){return e.name==='RequestCancelled'}finally{document.body.append(layer)}})()"), true); assert.equal(business, 0); checks.push("privacy accessible; removing DOM does not admit business fetch");
  await evaluate("document.querySelector('#license-code').value='TEST-ONLY';document.querySelector('#license-form').requestSubmit()"); await until("window.deepLegendsLicense.isActive()"); assert.equal(await evaluate("document.querySelector('#license-code').value"), ""); await until("!document.querySelector('#app-frame').hidden"); assert(business > 0); checks.push("manual activation starts business and clears input");
  state = "REPLACED"; generation++; await evaluate("window.deepLegendsLicense.poll()"); await until("document.querySelector('#license-message').textContent==='注册码已在其他设备使用或已被重置'"); assert.equal(await evaluate("document.querySelector('#app-frame').hidden && document.querySelector('#app-frame').hasAttribute('inert')"), true); checks.push("replacement reloads to locked shell without displayed business");
  assert.equal(errors.length, 0, JSON.stringify(errors));
  const result = { started, finished: new Date().toISOString(), scope: "Real Chromium client with synthetic local-state API; Windows/deployed-server acceptance pending", checks, errors }; fs.writeFileSync(path.join(out, "chromium.json"), JSON.stringify(result, null, 2)); console.log(JSON.stringify(result));
}
main().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => { ws?.close(); chrome?.kill(); server?.closeAllConnections(); server?.close(); fs.rmSync(temp, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 }); });
