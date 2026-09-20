"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require(require.resolve("jsdom", { paths: [path.join(__dirname, "..", "desktop")] }));
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const source = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
const ids = ["update-button", "update-dialog", "update-dialog-title", "update-notes", "update-meta", "update-progress", "update-progress-fill", "update-progress-percent", "update-progress-hint", "update-alert", "update-start", "update-later", "update-cancel", "update-apply", "update-release-link", "update-dialog-close", "settings-update-check", "settings-update-feedback"];
const available = { supported: true, current: "0.11.2", latest: "0.12.0", state: "available", portable: false, notes: "### 新增\n- **更新**和`代码`\n- [日志](https://example.com/log)", sizeBytes: 100 * 1024 * 1024, publishedAt: "2026-09-11T12:00:00Z", progress: {} };
function harness(script = source, respond) {
  const dom = new JSDOM(html, { url: "http://127.0.0.1:8787/", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window, requests = [], streams = [];
  w.matchMedia = () => ({ matches: false, addEventListener(){}, removeEventListener(){} });
  w.ResizeObserver = w.IntersectionObserver = class { observe(){} disconnect(){} unobserve(){} };
  w.HTMLElement.prototype.scrollTo = function(){};
  w.HTMLDialogElement.prototype.showModal = function(){this.open=true;};
  w.HTMLDialogElement.prototype.close = function(){this.open=false;};
  w.fetch = (url, options) => { requests.push([url,options]);if(url === "/api/diagnostics/client")return Promise.resolve({ok:true,status:204});return respond?.(url, options) || new Promise(()=>{}); };
  w.Headers = Headers;
  w.EventSource = class {
    static CLOSED = 2;
    constructor(url){this.url=url;this.listeners=new Map();streams.push(this);}
    addEventListener(name,fn){this.listeners.set(name,fn);}
    close(){}
    emit(name,data){this.listeners.get(name)?.({data:JSON.stringify(data)});}
  };
  try {
    w.eval(fs.readFileSync(path.join(__dirname, "runtime.js"), "utf8"));
    w.eval(script.replace(/\}\)\(\);\s*$/, 'window.updateProbe = { renderUpdateStatus, renderUpdateNotes, updateUI, renderUpdateDialog, showUpdateCheckFeedback };})();'));
  } catch(error) {dom.window.close();throw error;}
  return { w, dom, requests, streams, probe:w.updateProbe, get:id=>w.document.getElementById(id), close:()=>w.close() };
}
function renderScenarios(h) {
  for (const state of ["available", "downloading", "verifying", "ready", "failed", "applying"]) h.probe.renderUpdateStatus({ ...available, state, error:state==="failed"?"最后线路超时":"",progress:{receivedBytes:42,totalBytes:100,bytesPerSecond:10,etaSeconds:6} });
}
test("real app bootstrap registers every updater element and renders every state", () => {
 const h=harness();try {
  assert.equal(h.get("refresh").nextElementSibling.id,"update-button");
  assert.equal(h.get("update-button").nextElementSibling.id,"quit");
  assert.equal(h.get("update-button").hidden,true);
  renderScenarios(h);
  h.probe.renderUpdateStatus(available);
  assert.equal(h.get("update-button").hidden,false);
  assert.equal(h.get("update-dialog").open,false,"checks never open a dialog");
  assert.equal(h.get("update-notes").querySelector("h3").textContent,"新增");
  assert.equal(h.get("update-meta").textContent.includes("100.0 MB"),true);
  assert.equal(h.get("update-button").classList.contains("fresh"),true);
  h.get("update-button").click();
  assert.equal(h.get("update-dialog").open,true);
  assert.equal(h.get("update-button").querySelector(".dot").hidden,true);
  h.get("update-later").click();assert.equal(h.get("update-dialog").open,false);
 }finally{h.close();}
});
test("existing SSE drives progress and persists it across close/reopen without polling",()=>{
 const h=harness();try {
  assert.equal(h.streams.length,1);assert.equal(h.streams[0].url,"/api/events");
  h.streams[0].emit("update:status",{...available,state:"downloading"});
  h.streams[0].emit("update:progress",{receivedBytes:42,totalBytes:100,bytesPerSecond:10,etaSeconds:6});
  assert.equal(h.get("update-progress-percent").textContent,"42%");
  assert.ok(Math.abs(Number(h.get("update-button").querySelector(".hex-progress").getAttribute("stroke-dashoffset"))-59*.58)<1e-9);
  h.get("update-button").click();h.get("update-dialog-close").click();h.get("update-button").click();
  assert.equal(h.get("update-progress-percent").textContent,"42%");
  assert.equal(h.requests.some(([url])=>url.startsWith("/api/update/")),false,"opening/closing must not poll or cancel");
  h.get("update-cancel").click();assert.equal(h.requests.filter(([url])=>url==="/api/update/cancel").length,1);
  h.streams[0].emit("update:status",{...available,state:"ready"});
  assert.equal(h.get("update-button").classList.contains("ready"),true);
  assert.match(h.get("update-meta").textContent,/SHA-256/);
 }finally{h.close();}
});
test("portable, minimum supported, offline, game warning and no-update behavior",()=>{
 const h=harness();try {
  for(const extra of [{portable:true},{manualOnly:true}]){
   h.probe.renderUpdateStatus({...available,...extra});
   assert.equal(h.get("update-start").hidden,true);assert.equal(h.get("update-apply").hidden,true);
   assert.equal(h.get("update-release-link").hidden,false);assert.match(h.get("update-notes").textContent,/发布页/);
  }
  h.probe.renderUpdateStatus({...available,state:"ready"});h.get("update-button").click();
  h.w.dispatchEvent(new h.w.CustomEvent("deep-legends:gameflow",{detail:{phase:"ChampSelect"}}));
  assert.match(h.get("update-alert").textContent,/你正在对局中，建议打完再升级/);
  assert.equal(h.get("update-apply").disabled,false);
  h.probe.renderUpdateStatus({...available,state:"failed",error:"最后线路：HTTP 503"});
  assert.match(h.get("update-alert").textContent,/最后线路：HTTP 503/);
  assert.equal(h.get("update-notes").parentElement.hidden,true);
  assert.equal(h.get("update-meta").hidden,true);
  assert.equal(h.get("update-release-link").href,"https://github.com/LLYY0418/Deep-Legends/releases");
  for(const status of [{...available,supported:false},{...available,state:"idle"},{...available,state:"idle",error:"断网"}]){
   h.probe.renderUpdateStatus(status);assert.equal(h.get("update-button").hidden,true);
  }
 }finally{h.close();}
});
function assertSafeNotes(h){
 const node=h.get("update-notes");
 node.innerHTML=h.probe.renderUpdateNotes('### 标题\n- <img src=x onerror=alert(1)>\n- [危险](javascript:alert)\n- [普通](http://example.com)\n- [安全](https://example.com/?a=1&b=2)\n- [属性](https://example.com/"onmouseover="alert)\n- `**原样**`\n- **粗体**');
 assert.equal(node.querySelector("img"),null);assert.match(node.textContent,/<img src=x onerror=alert\(1\)>/);
 const links=[...node.querySelectorAll("a")];assert.equal(links.length,2);
 for(const link of links){assert.equal(new URL(link.href).protocol,"https:");assert.equal(link.target,"_blank");assert.equal(link.rel,"noopener noreferrer");assert.equal(link.getAttribute("onmouseover"),null);}
 assert.equal(node.querySelector("code").textContent,"**原样**");
}
test("notes allow only escaped whitelist Markdown",()=>{const h=harness();try{assertSafeNotes(h);}finally{h.close();}});
test("all 18 missing registry ids are killed by the real renderer",()=>{
 for(const id of ids){
  const mutated=source.replace(`"${id}",`,"");assert.notEqual(mutated,source,id);
  let h;
  assert.throws(()=>{h=harness(mutated);renderScenarios(h);h.probe.showUpdateCheckFeedback("");},undefined,id);
  h?.close();
 }
});
test("escaping, https whitelist and unsupported-button mutants fail the same checks",()=>{
 const mutants=[
  [source.replace('escapeHTML(String(notes || ""))','String(notes || "")'),assertSafeNotes],
  [source.replace('link && /^https:\\/\\//i.test(link[2])','link'),assertSafeNotes],
  [source.replace('current.supported && ["available"','true && ["available"'),h=>{h.probe.renderUpdateStatus({...available,supported:false});assert.equal(h.get("update-button").hidden,true);}]
 ];
 for(const [script,check] of mutants){assert.notEqual(script,source);const h=harness(script);try{assert.throws(()=>check(h));}finally{h.close();}}
});

const idle = { supported: true, current: "0.12.1", state: "idle", checking: false };
const response = (value, status = 200) => Promise.resolve({ ok: status >= 200 && status < 300, status, json: async () => value, text: async () => String(value) });
const flush = () => new Promise(resolve => setImmediate(resolve));
const checks = h => h.requests.filter(([url]) => url === "/api/update/check");
const forceClick = h => h.get("settings-update-check").dispatchEvent(new h.w.Event("click"));

async function assertManualAvailable(script = source) {
  const h = harness(script, url => url === "/api/update/check" ? response(available) : null);
  const automatic = harness();
  try {
    h.probe.renderUpdateStatus(idle);
    const button = h.get("settings-update-check");
    assert.equal(button.closest("#settings-panel")?.id, "settings-panel");
    assert.equal(h.get("settings-build-identity").nextElementSibling, button);
    assert.equal(button.hidden, false);
    assert.equal(button.disabled, false);
    button.click();
    assert.equal(button.disabled, true);
    assert.equal(button.textContent, "正在检查…");
    await flush();
    assert.equal(checks(h).length, 1);
    assert.equal(checks(h)[0][1].method, "POST");
    assert.equal(checks(h)[0][1].headers.get("Accept"), "application/json", "reuse api() headers");
    assert.equal(h.get("update-dialog").open, true);
    assert.equal(h.w.document.querySelectorAll("#update-dialog").length, 1);
    automatic.probe.renderUpdateStatus(available);
    automatic.get("update-button").click();
    assert.equal(h.get("update-dialog").innerHTML, automatic.get("update-dialog").innerHTML, "manual and notification entry share the renderer");
    assert.equal(h.get("settings-update-feedback").textContent, "");
    assert.equal(button.disabled, false);
  } finally { h.close(); automatic.close(); }
}

test("manual update check posts through api and opens the existing notification dialog", async () => {
  await assertManualAvailable();
});

test("manual check waits beyond the HTTP acknowledgement for SSE completion", async () => {
  const h = harness(source, url => url === "/api/update/check" ? response({ ...idle, state: "checking", checking: true }) : null);
  try {
    h.probe.renderUpdateStatus(idle);
    h.get("settings-update-check").click();
    await flush();
    assert.equal(h.get("settings-update-check").disabled, true);
    assert.equal(h.get("settings-update-feedback").textContent, "");
    forceClick(h);
    assert.equal(checks(h).length, 1);
    h.streams[0].emit("update:status", { ...available, checking: false });
    assert.equal(h.get("update-dialog").open, true);
    assert.equal(h.get("settings-update-check").disabled, false);
  } finally { h.close(); }
});

test("late checking acknowledgement cannot overwrite an earlier SSE result", async () => {
  let reply;
  const h = harness(source, url => url === "/api/update/check" ? new Promise(resolve => { reply = resolve; }) : null);
  try {
    h.probe.renderUpdateStatus(idle);
    h.get("settings-update-check").click();
    h.streams[0].emit("update:status", { ...available, checking: false });
    forceClick(h);
    assert.equal(checks(h).length, 1, "keep HTTP request locked until it returns");
    reply(await response({ ...idle, state: "checking", checking: true }));
    await flush();
    assert.equal(h.probe.updateUI.status.state, "available");
    assert.equal(h.get("update-dialog").open, true);
    assert.equal(h.get("settings-update-check").disabled, false);
  } finally { h.close(); }
});

test("no-update feedback expires and a new check clears the previous timer", async () => {
  const h = harness(source, url => url === "/api/update/check" ? response(idle) : null);
  try {
    const timers = new Map();
    const originalSet = h.w.setTimeout.bind(h.w), originalClear = h.w.clearTimeout.bind(h.w);
    let timerID = 100000;
    h.w.setTimeout = (fn, ms, ...args) => ms === 5000 ? (timers.set(++timerID, fn), timerID) : originalSet(fn, ms, ...args);
    h.w.clearTimeout = id => { timers.delete(id); originalClear(id); };
    h.probe.renderUpdateStatus(idle);
    h.get("settings-update-check").click();
    await flush();
    assert.equal(h.get("settings-update-feedback").textContent, "已是最新版本");
    assert.equal(h.get("update-dialog").open, false);
    assert.equal(timers.size, 1);
    h.get("settings-update-check").click();
    assert.equal(h.get("settings-update-feedback").textContent, "");
    assert.equal(timers.size, 0);
    await flush();
    assert.equal(timers.size, 1);
    [...timers.values()][0]();
    assert.equal(h.get("settings-update-feedback").textContent, "");
    assert.equal(h.get("settings-update-check").disabled, false);
  } finally { h.close(); }
});

test("HTTP, network and asynchronous check failures show errors instead of latest-version success", async () => {
  for (const failure of [() => response("当前构建不支持更新", 400), () => Promise.reject(new Error("网络不可用")), () => response({ ...idle, error: "无法检查更新：HTTP 503" })]) {
    const h = harness(source, url => url === "/api/update/check" ? failure() : null);
    try {
      h.probe.renderUpdateStatus(idle);
      h.get("settings-update-check").click();
      await flush();
      assert.match(h.get("settings-update-feedback").textContent, /当前构建不支持更新|网络不可用|HTTP 503/);
      assert.doesNotMatch(h.get("settings-update-feedback").textContent, /已是最新/);
      assert.equal(h.get("settings-update-check").disabled, false);
    } finally { h.close(); }
  }
  const h = harness(source, url => url === "/api/update/check" ? response({ ...idle, state: "checking", checking: true }) : null);
  try {
    h.probe.renderUpdateStatus(idle);
    h.get("settings-update-check").click();
    await flush();
    h.streams[0].emit("update:status", { ...idle, error: "无法检查更新：所有线路超时" });
    assert.match(h.get("settings-update-feedback").textContent, /所有线路超时/);
    assert.equal(h.get("settings-update-check").disabled, false);
  } finally { h.close(); }
});

async function assertManualBlocked(script = source) {
  const h = harness(script);
  try {
    for (const status of [{ ...idle, supported: false }, ...["checking", "downloading", "verifying", "applying"].map(state => ({ ...available, state }))]) {
      h.probe.renderUpdateStatus(status);
      assert.equal(h.get("settings-update-check").disabled, true, status.state);
      forceClick(h);
      assert.equal(checks(h).length, 0, status.state);
    }
    h.probe.renderUpdateStatus(idle);
    h.probe.updateUI.pending = true;
    h.probe.renderUpdateStatus(idle);
    assert.equal(h.get("settings-update-check").disabled, true);
    forceClick(h);
    assert.equal(checks(h).length, 0);
    h.probe.updateUI.pending = false;
    h.probe.renderUpdateStatus(idle);
    h.get("settings-update-check").click();
    forceClick(h); forceClick(h);
    assert.equal(checks(h).length, 1, "one unresolved manual request");
    assert.equal(h.get("settings-update-check").disabled, true);
  } finally { h.close(); }
}

test("unsupported, updater busy and manual in-flight checks cannot issue duplicate requests", async () => {
  await assertManualBlocked();
});

test("manual-check request, shared-dialog, support and busy mutations are all detected", async () => {
  const mutants = [
    [source.replace('api("/api/update/check", { method: "POST" }', 'api("/api/update/status", { method: "GET" }'), assertManualAvailable],
    [source.replace('].includes(current.state)) openUpdateDialog();', '].includes(current.state)) { const duplicate = el.updateDialog.cloneNode(true); document.body.append(duplicate); duplicate.showModal(); }'), assertManualAvailable],
    [source.replace('return current.supported !== true || updateUI.checkPending', 'return false || updateUI.checkPending'), assertManualBlocked],
    [source.replace('if (manualUpdateCheckBlocked()) return;', 'if (false) return;'), assertManualBlocked],
    [source.replace('if (result && updateUI.statusEventRevision === revision)', 'if (result)'), async script => {
      let reply;
      const h = harness(script, url => url === "/api/update/check" ? new Promise(resolve => { reply = resolve; }) : null);
      try {
        h.probe.renderUpdateStatus(idle); h.get("settings-update-check").click();
        h.streams[0].emit("update:status", available);
        reply(await response({ ...idle, state: "checking", checking: true })); await flush();
        assert.equal(h.get("update-dialog").open, true);
      } finally { h.close(); }
    }],
  ];
  for (const [script, check] of mutants) {
    assert.notEqual(script, source, "mutation must change production source");
    await assert.rejects(() => check(script));
  }
});
