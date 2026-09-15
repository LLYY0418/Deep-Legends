"use strict";
// Fake Electron events; real main.cjs, HTTP, diagnostics download hook and disk.
// Invoked by Go tests hosting the actual authenticated diagnostic handlers.
const assert = require("node:assert/strict"), fs = require("node:fs"), path = require("node:path"), vm = require("node:vm");
const http = require("node:http"), { EventEmitter } = require("node:events");
const { collectRecord, parseLines } = require("../../scripts/r82-startup-ab-report.cjs");
const desktop = path.resolve(__dirname, "..");
const [baseUrl, token, directory, mode, sourcePath, buildFingerprint] = process.argv.slice(2);
assert.ok(buildFingerprint, "Go test must supply the running build fingerprint");
const source = fs.readFileSync(sourcePath || path.join(desktop, "main.cjs"), "utf8");
const pending = new Set(), responses = [], windows = [], timers = new Map(), logs = [];
let boot, child, now = 1000, timerID = 0;
const app = new EventEmitter(), ipcMain = new EventEmitter(), screen = new EventEmitter(), nativeTheme = new EventEmitter();
Object.assign(app, { isPackaged: true, setAppUserModelId() {}, requestSingleInstanceLock: () => true,
  whenReady: () => ({ then(fn) { boot = fn; } }), getPath: name => name === "downloads" ? directory : path.join(directory, "existing-user-data"), quit() {} });
ipcMain.handle = ipcMain.removeHandler = () => {};
Object.assign(screen, { getPrimaryDisplay: () => ({ workAreaSize: { width: 1440, height: 900 } }), getAllDisplays: () => [] });
class Window extends EventEmitter {
  constructor(options) {
    super(); this.options = options; this.visible = options.show !== false; this.destroyed = false;
    this.webContents = new EventEmitter(); this.webContents.session = new EventEmitter();
    Object.assign(this.webContents, { isDestroyed: () => this.destroyed, getURL: () => this.url,
      executeJavaScript: async () => {}, send() {}, setWindowOpenHandler() {} });
    windows.push(this);
  }
  loadURL(url) { this.url = url; return Promise.resolve(); }
  isDestroyed() { return this.destroyed; }
  show() { this.visible = true; }
  focus() {}
  close() { this.destroyed = true; this.visible = false; this.emit("closed"); }
  isMaximized() { return false; }
  getContentBounds() { return { width: 1100, height: 780 }; }
  getBounds() { return this.getContentBounds(); }
  setMinimumSize() {}
  setTitleBarOverlay() {}
}
const trackedHTTP = {
  Agent: http.Agent,
  request(url, options, callback) {
    let resolve;
    const completed = new Promise(done => { resolve = done; });
    pending.add(completed);
    const finish = () => { pending.delete(completed); resolve(); };
    const request = http.request(url, options, response => {
      if (url.includes("/api/diagnostics/startup")) responses.push({ url, status: response.statusCode });
      response.once("end", finish);
      callback(response);
    });
    request.once("error", finish);
    return request;
  },
};
const context = vm.createContext({ __dirname: desktop, URL, Buffer, console,
  Date: class extends Date { static now() { return now; } },
  process: { on() {}, platform: "win32", env: {}, resourcesPath: "/synthetic/resources", getCreationTime: () => 900 },
  setTimeout(fn, delay) { const id = ++timerID; timers.set(id, { fn, delay }); return id; },
  clearTimeout(id) { timers.delete(id); }, setImmediate,
  require(name) {
    if (name === "electron") return { app, BrowserWindow: Window, ipcMain, screen, nativeTheme, dialog: {}, shell: {},
      session: { defaultSession: { setPermissionCheckHandler() {}, setPermissionRequestHandler() {} } } };
    if (name === "node:http") return trackedHTTP;
    if (name === "node:fs") return {
      readFileSync: () => Buffer.from("synthetic logo"), existsSync: file => file.endsWith("loot-service.exe") || fs.existsSync(file),
      statSync: file => fs.statSync(file), mkdirSync() {}, appendFileSync: (_file, line) => logs.push(line),
    };
    if (name === "node:child_process") return { spawn() {
      child = new EventEmitter(); child.stdout = new EventEmitter(); child.stderr = new EventEmitter();
      child.stdout.setEncoding = child.stderr.setEncoding = () => {}; child.kill = () => {}; return child;
    } };
    if (name === "./share-export.cjs") return { createShareExportController: () => ({ clear() {}, getSaveDirectory: () => ({ directory }) }) };
    if (name === "./window-bounds-store.cjs") return { readWindowBounds: () => null, writeWindowBounds() {} };
    if (name === "./proxy-resolution.cjs") return { resolveSystemProxy: async () => "" };
    if (name.startsWith("./")) return require(path.join(desktop, name));
    return require(name);
  },
});
vm.runInContext(source + "\nglobalThis.closeTestConnections = () => startupStageAgent?.destroy();", context);

async function settle() {
  await Promise.resolve();
  while (pending.size) await Promise.all([...pending]);
  assert.ok(responses.every(response => response.status === 204), `diagnostic POST rejected: ${JSON.stringify(responses)}`);
}
async function exportedEvents() {
  const url = `${baseUrl}/api/diagnostics/log`;
  const body = await new Promise((resolve, reject) => {
    http.get(url, { headers: { "X-Local-Token": token } }, response => {
      assert.equal(response.statusCode, 200);
      assert.match(response.headers["content-type"], /application\/x-ndjson/);
      let data = ""; response.setEncoding("utf8"); response.on("data", chunk => { data += chunk; }); response.on("end", () => resolve(data));
    }).on("error", reject);
  });
  // Exercise the real hook attached by createMainWindow. Electron's file write
  // is simulated with the exact response bytes and the native Save dialog path.
  const main = windows[1], item = new EventEmitter();
  item.getURL = () => url; item.setSavePath = file => { item.savedPath = file; };
  item.setSaveDialogOptions = options => { item.dialogOptions = options; };
  item.getSavePath = () => item.savedPath;
  main.webContents.session.emit("will-download", {}, item, main.webContents);
  assert.equal(item.savedPath, undefined, "native Save dialog must choose the file");
  assert.equal(path.dirname(item.dialogOptions?.defaultPath || ""), directory, "diagnostic default must use downloads");
  item.savedPath = item.dialogOptions.defaultPath;
  fs.writeFileSync(item.savedPath, body); item.emit("done", {}, "completed");
  return parseLines(fs.readFileSync(item.savedPath, "utf8"));
}
async function checkSnapshot(expected) {
  await settle();
  const events = await exportedEvents(), stages = events.filter(event => event.event === "desktop_startup_stage");
  assert.deepEqual(stages.map(event => event.stage), expected, "export must contain stages before main window visibility");
  const appStart = events.find(event => event.event === "app_start");
  assert.ok(appStart);
  assert.ok(events.every(event => event.build_fingerprint === buildFingerprint), "export must retain the running build fingerprint on every event");
  assert.ok(stages.every(event => event.run_id === appStart.run_id && Number.isInteger(event.elapsed_ms) && event.elapsed_ms >= 0));
  for (let i = 1; i < stages.length; i++) assert.ok(stages[i].log_seq > stages[i - 1].log_seq, "stage sequence must retain event order");
  return events;
}
async function run() {
  now = 1100; boot(); const splash = windows[0];
  assert.equal(splash.options.show, false); assert.equal(splash.options.alwaysOnTop, true);
  now = 1150; splash.emit("ready-to-show");
  now = 1200; splash.webContents.emit("did-finish-load");
  now = 1400; child.stdout.emit("data", `LOOT_READY ${JSON.stringify({ baseUrl, bootstrapUrl: `${baseUrl}/?bootstrap=${token}`, token })}\n`);
  const main = windows[1]; assert.ok(main); assert.equal(main.options.show, false);
  const expected = ["backend_ready", "main_window_created"];
  let events = await checkSnapshot(expected);
  assert.equal(main.visible, false); assert.equal(splash.visible, true);
  assert.equal(events.some(event => event.event === "desktop_startup_phases_ms"), false, "summary must remain success-only");
  if (mode !== "stuck-created") {
    now = 1600; main.webContents.emit("did-finish-load");
    expected.push("main_window_did_finish_load"); events = await checkSnapshot(expected);
    assert.equal(main.visible, false); assert.equal(events.some(event => event.event === "desktop_startup_phases_ms"), false);
    if (mode === "normal") {
      now = 1800; main.emit("ready-to-show"); expected.push("main_window_ready_to_show");
      events = await checkSnapshot(expected);
      assert.equal(main.visible, true); assert.equal(splash.visible, false);
      const fixture = { records: [], group: "prewarm", fingerprint: buildFingerprint, pid: "321", since: 0, diagnostics: events,
        startupLog: "[pid=321] startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=55\n[pid=321] application handoff visible=true elapsed_ms=2100" };
      const record = collectRecord(fixture); assert.ok(record, "legacy collector lost the successful startup summary");
      assert.equal(record.phases_ms.spawn_to_ready, 200); assert.equal(record.phases_ms.total, 900);
      assert.deepEqual(Object.keys(record.phases_ms).sort(), ["process_to_js", "js_to_ready", "ready_to_splash", "splash_paint", "splash_window_shown", "spawn_to_ready", "ready_to_window", "total"].sort());
      main.emit("ready-to-show"); main.webContents.emit("did-finish-load"); await checkSnapshot(expected);
    }
  }
  console.log(JSON.stringify({ mode, stages: expected, exported: true }));
}
run().catch(error => { console.error(error.stack); process.exitCode = 1; }).finally(() => context.closeTestConnections());
