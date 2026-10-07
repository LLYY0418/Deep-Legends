"use strict";
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// Real Chromium / actual client HTML and JS / synthetic local Go-state API.
// This does not claim a deployed license server or Windows acceptance.
const { spawn } = require("node:child_process"), fs = require("node:fs"), path = require("node:path"), os = require("node:os"), http = require("node:http"), assert = require("node:assert/strict");
const root = path.resolve(__dirname, ".."), web = process.env.R240_WEB_ROOT || path.join(root, "backend/web"), out = process.env.R240_BROWSER_OUTPUT || path.join(root, "docs/history/reports/r240/browser");
const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r240-chromium-"));
let chrome, ws, server, origin, state = "LOCKED", generation = 1, business = 0, privacyRequests = 0, privacyFailure = false, activationCode = "", holdInitialStatus = true, releaseStatus;
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
    if (name === "/api/license/status") { if (holdInitialStatus) releaseStatus = () => json(snapshot()); else json(snapshot()); return; }
    if (name === "/api/license/activate") {
      let body = ""; request.on("data", chunk => { body += chunk; });
      request.on("end", () => { activationCode = JSON.parse(body).code; setTimeout(() => { state = "ACTIVE"; generation++; json(snapshot()); }, 350); }); return;
    }
    if (name === "/api/privacy") { privacyRequests++; if (privacyFailure) { response.writeHead(503); response.end(); } else json({ licenseDisclosure: "测试授权数据隐私说明（合成夹具）。\n".repeat(80) }); return; }
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
  const click = async selector => {
    const p = await evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`);
    await call("Input.dispatchMouseEvent", { type: "mousePressed", ...p, button: "left", clickCount: 1 });
    await call("Input.dispatchMouseEvent", { type: "mouseReleased", ...p, button: "left", clickCount: 1 });
  };
  const key = async (name, code) => { for (const type of ["keyDown", "keyUp"]) await call("Input.dispatchKeyEvent", { type, key: name, code: name, windowsVirtualKeyCode: code, ...(name === "Enter" && type === "keyDown" ? { text: "\r", unmodifiedText: "\r" } : {}) }); };
  const inputHit = "(()=>{const n=document.querySelector('#license-code'),r=n.getBoundingClientRect();return document.elementFromPoint(r.x+r.width/2,r.y+r.height/2)===n})()";
  const noScroll = "[document.documentElement,document.body,document.querySelector('#license-overlay')].every(n=>n.scrollHeight<=n.clientHeight && n.scrollWidth<=n.clientWidth)";
  const closed = "getComputedStyle(document.querySelector('#license-privacy-dialog')).display";
  await call("Emulation.setDeviceMetricsOverride", { width: 860, height: 580, deviceScaleFactor: 1, mobile: false }); await call("Page.navigate", { url: origin });
  await until("window.deepLegendsLicense && document.documentElement.dataset.license==='pending'");
  assert.equal(await evaluate("document.querySelector('#license-overlay').hidden && document.querySelector('#app-frame').hidden && getComputedStyle(document.querySelector('#license-overlay')).display==='none' && getComputedStyle(document.querySelector('#app-frame')).display==='none'"), true, "P2 default pending surfaces must both be invisible");
  checks.push("real rendered pending: overlay and frame both invisible");
  for (let n = 0; !releaseStatus && n < 200; n++) await new Promise(resolve => setTimeout(resolve, 10));
  assert.ok(releaseStatus); holdInitialStatus = false; releaseStatus();
  await until("document.documentElement.dataset.license==='locked'");
  assert.deepEqual(await evaluate("(()=>{const q=s=>document.querySelector(s),h=s=>q(s).getBoundingClientRect().height;return {card:q('#license-form').getBoundingClientRect().width,icon:h('#license-form img'),title:getComputedStyle(q('#license-title')).fontSize,input:h('#license-code'),button:h('#license-submit'),hint:getComputedStyle(q('#license-message')).fontSize,link:getComputedStyle(q('#license-privacy')).fontSize}})()"), {card:480,icon:64,title:'24px',input:46,button:44,hint:'13px',link:'13px'});
  assert.equal(await evaluate(closed), "none", "P1 closed privacy dialog must be invisible");
  assert.equal(privacyRequests, 0, "privacy is fetched only after the link is clicked");
  assert.equal(await evaluate(inputHit), true, "input center must be unobstructed");
  assert.equal(await evaluate("document.activeElement.id"), "license-code");
  assert.equal(business, 0); checks.push("860x580 first frame: closed privacy, input hit/focus, no business requests");
  for (const theme of ["dark", "light"]) for (const zoom of [1, 2.5]) {
    await evaluate(`document.documentElement.dataset.theme='${theme}';document.documentElement.style.setProperty('--ui-zoom','${zoom}')`);
    assert.equal(await evaluate("(()=>{const r=document.querySelector('#license-form').getBoundingClientRect();return r.left>=0 && r.top>=0 && r.right<=innerWidth && r.bottom<=innerHeight})()"), true);
    assert.equal(await evaluate(noScroll), true, "860x580 page must not scroll");
    assert.equal(await evaluate("getComputedStyle(document.querySelector('#license-form')).zoom"), "1");
    assert.equal(await evaluate("(()=>{const a=[...document.querySelectorAll('.license-links a')].map(n=>n.getBoundingClientRect());return a.length===2 && a[0].top===a[1].top})()"), true);
    const screenshot = await call("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(path.join(out, `${theme}-${zoom}-locked.png`), Buffer.from(screenshot.data, "base64"));
  }
  await evaluate("document.querySelector('.license-links a:last-child').focus()"); await key("Tab", 9); assert.equal(await evaluate("document.activeElement.id"), "license-code");
  // Closed-state audit includes all real static dialogs and the two dynamically
  // generated dialog class families; hidden remains protected by !important.
  assert.equal(await evaluate("(()=>{for(const cls of ['mayhem-tier-dialog','cs-dialog']){const n=document.createElement('dialog');n.className=cls;document.body.append(n)}return [...document.querySelectorAll('dialog:not([open]),[hidden]')].every(n=>getComputedStyle(n).display==='none')})()"), true);
  for (const closeBy of ["button", "Escape"]) {
    await click("#license-privacy"); await until("document.querySelector('#license-privacy-dialog').open");
    assert.equal(await evaluate("document.querySelector('#license-privacy-text').textContent.length>0"), true);
    assert.equal(await evaluate("(()=>{const r=document.querySelector('#license-privacy-dialog').getBoundingClientRect();return r.left>=0 && r.top>=56 && r.right<=860 && r.bottom<=580 && r.height<=580*.8})()"), true);
    assert.equal(await evaluate("(()=>{const n=document.querySelector('#license-privacy-text');return n.scrollHeight>n.clientHeight})()"), true, "long disclosure scrolls inside the body");
    if (closeBy === "button") await click("#license-privacy-dialog button"); else await key("Escape", 27);
    await until("!document.querySelector('#license-privacy-dialog').open");
    assert.equal(await evaluate(closed), "none"); assert.equal(await evaluate(inputHit), true); assert.equal(await evaluate(noScroll), true);
  }
  checks.push("real privacy link/Close/Esc, nonempty scrollable body, bounds and caption area");
  const formHeight = await evaluate("document.querySelector('#license-form').getBoundingClientRect().height");
  privacyFailure = true; await click("#license-privacy"); await until("document.querySelector('#license-message').textContent==='隐私说明读取失败，请重试'");
  assert.equal(await evaluate(closed), "none"); assert.equal(await evaluate("document.querySelector('#license-form').getBoundingClientRect().height"), formHeight); assert.equal(await evaluate(noScroll), true);
  privacyFailure = false; checks.push("privacy read failure keeps original message and stable one-line layout");
  await click("#license-code"); await call("Input.insertText", { text: "TEST-ONLY-R240" }); await key("Enter", 13);
  await until("document.querySelector('#license-code').disabled && document.querySelector('#license-submit').disabled");
  await until("window.deepLegendsLicense.isActive()"); assert.equal(activationCode, "TEST-ONLY-R240");
  assert.equal(await evaluate("document.querySelector('#license-code').value"), ""); checks.push("real pointer/text/Enter activation, busy disabled, input cleared");
  state = "REPLACED"; generation++; await evaluate("window.deepLegendsLicense.poll()"); await until("document.querySelector('#license-message').textContent==='注册码已在其他设备使用或已被重置'");
  assert.equal(await evaluate("document.querySelector('#license-message').dataset.tone"), "error");
  assert.equal(await evaluate(inputHit), true); assert.equal(await evaluate(noScroll), true);
  checks.push("replacement warning remains one line without scrolling");
  assert.equal(errors.length, 0, JSON.stringify(errors));
  const result = { started, finished: new Date().toISOString(), scope: "Real Chromium client with synthetic local-state API; Windows/deployed-server acceptance pending", checks, errors }; fs.writeFileSync(path.join(out, "chromium.json"), JSON.stringify(result, null, 2)); console.log(JSON.stringify(result));
}
main().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => { ws?.close(); chrome?.kill(); server?.closeAllConnections(); server?.close(); fs.rmSync(temp, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 }); });
