"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path"), vm = require("node:vm");
const { EventEmitter } = require("node:events");
const source = fs.readFileSync(path.join(__dirname, "main.cjs"), "utf8");

function harness(text = source) {
  const app = new EventEmitter(), windows = [], timers = new Map(), posts = [];
  let boot, now = 1000, spawns = 0, timerID = 0;
  Object.assign(app, { isPackaged: true, setAppUserModelId() {}, requestSingleInstanceLock: () => true,
    whenReady: () => ({ then(fn) { boot = fn; } }), getPath: () => "/test", quit() {} });
  class Window extends EventEmitter {
    constructor(options) {
      super(); this.options = options; this.visible = options.show !== false; this.destroyed = false; this.shows = 0;
      this.webContents = new EventEmitter(); windows.push(this);
    }
    loadURL() { return Promise.resolve(); }
    show() { assert.equal(this.destroyed, false); this.visible = true; this.shows++; }
    focus() {}
    isVisible() { return this.visible; }
    isDestroyed() { return this.destroyed; }
    close() { this.destroyed = true; this.visible = false; this.emit("closed"); }
  }
  const electron = { app, BrowserWindow: Window, ipcMain: new EventEmitter(), nativeTheme: new EventEmitter(),
    dialog: { showMessageBox: () => Promise.resolve() }, shell: {},
    session: { defaultSession: { setPermissionCheckHandler() {}, setPermissionRequestHandler() {} } } };
  electron.ipcMain.handle = () => {};
  const context = vm.createContext({ __dirname, URL, Buffer, console,
    Date: class extends Date { static now() { return now; } },
    process: { on() {}, platform: "win32", env: {}, resourcesPath: "/test/resources", getCreationTime: () => 900 },
    setTimeout(fn, delay) { const id = ++timerID; timers.set(id, { fn, delay }); return id; },
    clearTimeout(id) { timers.delete(id); },
    require(name) {
      if (name === "electron") return electron;
      if (name === "node:fs") return { readFileSync: () => Buffer.from("logo"), existsSync: () => true, mkdirSync() {}, appendFileSync() {} };
      if (name === "node:child_process") return { spawn() {
        spawns++; const child = new EventEmitter(); child.stdout = new EventEmitter(); child.stderr = new EventEmitter();
        child.stdout.setEncoding = child.stderr.setEncoding = () => {}; child.kill = () => {}; return child;
      } };
      if (name === "node:http") return { request() { const request = new EventEmitter(); request.end = body => posts.push(JSON.parse(body)); request.destroy = () => {}; return request; } };
      if (name.startsWith("./")) return { attachDiagnosticsExport() {} };
      return require(name);
    },
  });
  vm.runInContext(text + `\nglobalThis.probe = {
    marks: startupMarks, closeSplashWindow, startBackendOnce,
    report() { backendReady = { baseUrl: "http://127.0.0.1:8787", token: "test" }; startupMarks.mainShown = Date.now(); reportStartupPhases(); }
  };`, context);
  return { app, windows, timers, posts, probe: context.probe, boot: () => boot(), setTime: value => { now = value; }, get spawns() { return spawns; } };
}

function checkStartup(text = source) {
  const h = harness(text); h.boot(); const splash = h.windows[0];
  assert.equal(splash.options.show, false, "splash must be created hidden");
  assert.equal(h.spawns, 0, "backend must wait for the renderer or fallback");
  const fallback = [...h.timers.values()].find(timer => timer.delay === 400);
  assert.ok(fallback, "400ms fallback is required");
  h.app.emit("second-instance"); h.app.emit("activate");
  assert.equal(splash.visible, false, "reactivation must not reveal an unpainted splash");
  h.setTime(1200); splash.emit("ready-to-show");
  assert.equal(splash.shows, 1, "ready-to-show must reveal splash exactly once");
  assert.equal(h.probe.marks.splashWindowShown, 1200, "record the actual show event");
  assert.equal(h.spawns, 0, "ready-to-show must not replace did-finish-load as spawn trigger");
  h.setTime(1210); splash.webContents.emit("did-finish-load");
  assert.equal(h.spawns, 1, "did-finish-load must start the backend");
  assert.equal([...h.timers.values()].some(timer => timer.delay === 400), false);
  fallback.fn(); assert.equal(h.spawns, 1, "late fallback must not spawn twice");
  h.setTime(1500); h.probe.report();
  assert.equal(h.posts[0].splashWindowShown, 300, "show time is relative to OS process creation");
  return h;
}

test("R82 splash appears only when painted and reports its actual show time", () => checkStartup());
test("R82 closed splash cannot be resurrected by late readiness", () => {
  const h = harness(); h.boot(); const splash = h.windows[0]; h.probe.closeSplashWindow();
  splash.emit("ready-to-show"); assert.equal(splash.shows, 0); assert.equal(h.probe.marks.splashWindowShown, undefined);
});
test("R82 fallback boots a stalled splash; quitting cancels all startup triggers", () => {
  const h = harness(); h.boot(); [...h.timers.values()].find(t => t.delay === 400).fn(); assert.equal(h.spawns, 1);
  const quit = harness(); quit.boot(); const late = [...quit.timers.values()].find(t => t.delay === 400).fn;
  quit.app.emit("before-quit", { preventDefault() {} });
  assert.equal([...quit.timers.values()].some(t => t.delay === 400), false);
  late(); quit.windows[0].webContents.emit("did-finish-load"); quit.windows[0].emit("ready-to-show");
  assert.equal(quit.spawns, 0); assert.equal(quit.windows[0].shows, 0);
});
test("R82 all four requested splash mutations fail real event-driven checks", () => {
  for (const [name, broken] of [
    ["visible at creation", source.replace("    show: false,", "    show: true,")],
    ["never show", source.replace("    splashWindow.show();", "    // show removed")],
    ["no painted trigger", source.replace("    startBackendOnce();", "    // trigger removed")],
    ["no fallback", source.replace("backendStartTimer = setTimeout(startBackendOnce, 400)", "void 0")],
  ]) { assert.notEqual(broken, source, name); assert.throws(() => checkStartup(broken), undefined, name); }
});
