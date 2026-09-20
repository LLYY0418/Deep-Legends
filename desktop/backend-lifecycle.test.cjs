"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { EventEmitter } = require("node:events");
const { JSDOM } = require("jsdom");
const mainSource = fs.readFileSync(process.env.R100_BACKEND_MAIN_SOURCE || path.join(__dirname, "main.cjs"), "utf8");
const preloadSource = fs.readFileSync(path.join(__dirname, "preload.cjs"), "utf8");
const appSource = fs.readFileSync(process.env.R100_BACKEND_APP_SOURCE || path.join(__dirname, "../web/app.js"), "utf8");
const flush = () => new Promise(setImmediate);
function extract(name) {
  let start = appSource.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (appSource.slice(start - 6, start) === "async ") start -= 6;
  return appSource.slice(start, appSource.indexOf("\n  }", start) + 4);
}
function shellHarness() {
  const app = new EventEmitter(), child = new EventEmitter(), ipcRenderer = new EventEmitter();
  const handlers = new Map(), messages = [], sent = [], bridges = {};
  let quits = 0, relaunches = 0, url = "http://127.0.0.1:8787/";
  Object.assign(app, { isPackaged: false, setAppUserModelId() {}, requestSingleInstanceLock: () => true,
    whenReady: () => ({ then() {} }), getPath: () => "/test", quit() { quits++; }, relaunch() { relaunches++; } });
  child.stdout = new EventEmitter(); child.stderr = new EventEmitter();
  child.stdout.setEncoding = child.stderr.setEncoding = () => {}; child.kill = () => {};
  const contents = { isDestroyed: () => false, getURL: () => url, send(channel, payload) {
    sent.push({ channel, payload }); ipcRenderer.emit(channel, {}, payload);
  } };
  const window = { isDestroyed: () => false, webContents: contents };
  const ipcMain = new EventEmitter();
  ipcMain.handle = (name, fn) => handlers.set(name, fn);
  ipcMain.removeHandler = name => handlers.delete(name);
  ipcRenderer.invoke = async (name, ...args) => handlers.get(name)({ sender: contents }, ...args);
  ipcRenderer.send = () => {};
  const electron = { app, ipcMain, ipcRenderer, BrowserWindow: class {}, nativeTheme: {}, session: {}, shell: {},
    dialog: { showMessageBox(value) { messages.push(value); return Promise.resolve(); } },
    contextBridge: { exposeInMainWorld(name, value) { bridges[name] = value; } } };
  const context = vm.createContext({ require(name) {
    if (name === "electron") return electron;
    if (name === "node:child_process") return { spawn: () => child };
    if (name === "node:fs") return { mkdirSync() {}, appendFileSync() {} };
    return require(name);
  }, __dirname, process: { on() {}, platform: "win32", env: {} }, URL, Buffer, console, setTimeout: () => 1, clearTimeout() {} });
  vm.runInContext(mainSource + '\nglobalThis.probe={startBackend,attach(window){backendReady={baseUrl:"http://127.0.0.1:8787",token:"secret"};mainWindow=window;setupBackendIPC();}};', context);
  context.probe.startBackend(); context.probe.attach(window);
  vm.runInNewContext(preloadSource, { require: () => electron });
  return { child, app, handlers, contents, bridges, messages, sent, ipcRenderer,
    navigate(value) { url = value; }, get quits() { return quits; }, get relaunches() { return relaunches; } };
}
function rendererHarness(bridge) {
  const dom = new JSDOM('<div id="connection"><span></span></div><div id="notice" hidden></div><div id="launchpad"></div><main>已有战绩</main>', { url: "http://127.0.0.1:8787", runScripts: "outside-only" });
  const w = dom.window;
  w.desktopBackend = bridge;
  w.state = { controllers: new Map(), status: { connected: true }, section: "live", statusRequestToken: 0 };
  w.el = { connection: w.document.querySelector("#connection"), notice: w.document.querySelector("#notice"), clientLaunchpad: w.document.querySelector("#launchpad") };
  w.STATUS_INTERVAL = 3600000;
  w.escapeHTML = text => String(text).replaceAll("<", "&lt;");
  w.updateWorkspaceAvailability = w.hideReadingOverlay = w.updateReadingOverlay = w.clearDisconnectedClientState = () => {};
  w.loadClientInstallations = async () => {};
  w.toasts = []; w.showToast = message => w.toasts.push(message);
  w.deepLegendsStatusRecovery = visible => { w.recovery = visible; };
  w.api = async () => { throw Error("HTTP timeout"); };
  w.eval(["scheduleStatus", "refreshStatus", "setupBackendLifecycle", "showFatal", "renderStatus"].map(extract).join("\n"));
  w.setupBackendLifecycle();
  return { w, close() { w.dispatchEvent(new w.Event("deep-legends:dispose")); dom.window.close(); } };
}

test("confirmed child close traverses main IPC + preload + renderer, offers a real app restart", async () => {
  const h = shellHarness(), r = rendererHarness(h.bridges.desktopBackend);
  try {
    await flush();
    assert.equal(await h.bridges.desktopBackend.restart(), false, "running process must not be restartable via this recovery action");
    for (let i = 0; i < 6; i++) await r.w.refreshStatus();
    assert.equal(r.w.document.body.classList.contains("is-fatal"), false);
    assert.equal(r.w.recovery, true);
    h.child.emit("close", 1, null);
    assert.equal(h.messages.length, 0, "runtime failure keeps recovery UI alive rather than startup dialog + quit");
    assert.equal(h.quits, 0);
    assert.equal(r.w.document.body.classList.contains("is-fatal"), true);
    assert.match(r.w.el.notice.textContent, /本地数据服务已退出.*请重启软件/);
    assert.equal(r.w.recovery, false);
    assert.equal(r.w.document.querySelector("main").textContent, "已有战绩");
    r.w.el.notice.querySelector("button").click(); await flush();
    assert.equal(h.relaunches, 1); assert.equal(h.quits, 1);
    assert.equal(await h.bridges.desktopBackend.restart(), false, "double click must not relaunch twice");
  } finally { r.close(); }
});

test("process exit before renderer subscription is recovered from retained snapshot", async () => {
  const h = shellHarness(); h.child.emit("close", null, "SIGKILL");
  const r = rendererHarness(h.bridges.desktopBackend);
  try { await flush(); assert.equal(r.w.state.backendExited, true); assert.match(r.w.el.notice.textContent, /重启软件/); }
  finally { r.close(); }
});

test("late successful status and old running snapshot cannot erase a confirmed exit", async () => {
  let listener, resolveSnapshot, resolveStatus;
  const r = rendererHarness({ onStateChanged(fn) { listener = fn; }, getState: () => new Promise(resolve => { resolveSnapshot = resolve; }) });
  try {
    await flush();
    r.w.api = () => new Promise(resolve => { resolveStatus = resolve; });
    const pending = r.w.refreshStatus();
    listener({ state: "exited" }); resolveSnapshot({ state: "running" }); resolveStatus({ connected: true });
    await pending; await flush();
    r.w.renderStatus();
    assert.equal(r.w.document.body.classList.contains("is-fatal"), true);
    assert.equal(r.w.recovery, false);
    r.w.api = () => { assert.fail("no more polls after confirmed exit"); };
    await r.w.refreshStatus();
  } finally { r.close(); }
});

test("unknown, absent or rejected bridge and repeated HTTP errors never prove death", async () => {
  for (const bridge of [undefined, { getState: async () => ({ state: "unknown" }) }, { getState: async () => { throw Error("IPC unavailable"); } }]) {
    const r = rendererHarness(bridge);
    try {
      await flush(); for (let i = 0; i < 6; i++) await r.w.refreshStatus();
      assert.equal(r.w.document.body.classList.contains("is-fatal"), false);
      assert.equal(r.w.recovery, true);
    } finally { r.close(); }
  }
});

test("lifecycle bridge cleans listeners and ignores async results after disposal", async () => {
  const h = shellHarness(), r = rendererHarness(h.bridges.desktopBackend);
  r.w.dispatchEvent(new r.w.Event("deep-legends:dispose"));
  assert.equal(h.ipcRenderer.listenerCount("desktop-backend-state"), 0);
  h.child.emit("close", 1, null); await flush();
  assert.equal(r.w.state.backendExited, undefined); r.close();
});

test("graceful update/user shutdown does not publish fatal; foreign renderers cannot query or restart", async () => {
  for (const reason of ["user", "update"]) {
    const h = shellHarness(); h.child.emit("exit", 0, null);
    h.child.stdout.emit("data", `LOOT_QUIT ${reason}\n`); h.child.emit("close", 0, null);
    assert.equal(h.sent.length, 0); assert.equal(h.messages.length, 0);
  }
  const h = shellHarness(); h.child.emit("close", 1, null);
  const foreign = { isDestroyed: () => false, getURL: () => "http://127.0.0.1:8787/" };
  assert.equal(h.handlers.get("desktop-backend-state")({ sender: foreign }), null);
  assert.equal(h.handlers.get("desktop-backend-restart")({ sender: foreign }), false);
  h.navigate("https://untrusted.example/");
  assert.equal(await h.bridges.desktopBackend.getState(), null);
  assert.equal(await h.bridges.desktopBackend.restart(), false);
  assert.equal(h.relaunches, 0);
});

test("renderer bootstrap actually installs lifecycle subscription", () => {
  assert.match(appSource, /setupBackendLifecycle\(\);\s*setupLiveUpdates\(\)/);
  assert.match(mainSource, /setupBackendIPC\(\);\s*ipcMain.removeHandler\("desktop-scale-get"\)/);
});


test("restart IPC failure tells the user to reopen manually instead of reloading a dead service", async () => {
  const h = shellHarness();
  const r = rendererHarness({ ...h.bridges.desktopBackend, restart: async () => { throw Error("shell unavailable"); } });
  try {
    h.child.emit("close", 1, null); r.w.el.notice.querySelector("button").click(); await flush();
    assert.match(r.w.toasts[0], /请关闭软件后重新打开/);
    assert.equal(r.w.document.body.classList.contains("is-fatal"), true);
  } finally { r.close(); }
});
