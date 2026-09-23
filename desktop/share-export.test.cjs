"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const {
  SHARE_EXPORT_SURFACE_WIDTH,
  sanitizeSuggestedName,
  normalizeCapturePayload,
  screenshotScale,
  exportDocumentScript,
  createShareExportController,
} = require("./share-export.cjs");

const validToken = "A".repeat(32);
const validMarkup = '<div class="overview-share-surface"><main>overview</main></div>';
const temporaryDirectory = path.resolve("/tmp");
const userDataDirectory = path.resolve("/user-data");

test("分享图文件名会清理路径字符并固定为 PNG", () => {
  assert.equal(sanitizeSuggestedName('Deep:Legends/玩家?.jpg'), "Deep-Legends-玩家-.jpg.png");
  assert.equal(sanitizeSuggestedName("overview.PNG"), "overview.PNG");
  assert.equal(sanitizeSuggestedName("", new Date("2026-08-26T00:00:00Z")), "deep-legends-overview-2026-08-26.png");
});

test("分享图载荷限制令牌、根节点、大小和主题", () => {
  assert.deepEqual(normalizeCapturePayload({ token: validToken, markup: validMarkup, theme: "dark", density: "compact" }), {
    token: validToken,
    markup: validMarkup,
    theme: "dark",
    density: "compact",
  });
  assert.throws(() => normalizeCapturePayload({ token: "short", markup: validMarkup }), /保存请求/);
  assert.throws(() => normalizeCapturePayload({ token: validToken, markup: "<main></main>" }), /内容无效/);
  assert.throws(() => normalizeCapturePayload({ token: validToken, markup: validMarkup, theme: "unknown" }), /主题无效/);
  assert.equal(normalizeCapturePayload({ token: validToken, markup: validMarkup, density: "comfortable" }).density, "");
  assert.throws(() => normalizeCapturePayload({ token: validToken, markup: validMarkup, density: "wide" }), /密度无效/);
});

test("导出样式固定标准桌面双列并强制恢复生涯栏和水印", () => {
  const css = fs.readFileSync(path.join(__dirname, "..", "backend", "web", "gameplay.css"), "utf8");
  assert.match(css, /\.overview-share-surface\s*\{[^}]*width:\s*1488px/s);
  assert.match(css, /\.overview-share-surface \.overview-share-content\s*\{[^}]*width:\s*1440px[^}]*container:\s*gameplay-page\s*\/\s*inline-size/s);
  assert.match(css, /\.overview-share-surface \.overview-layout\s*\{[^}]*grid-template-columns:\s*minmax\(300px,340px\)\s+minmax\(0,1fr\)\s*!important/s);
  assert.match(css, /\.overview-share-surface \.overview-layout > \.career-column\s*\{[^}]*display:\s*grid\s*!important/s);
  assert.match(css, /\.overview-share-watermark\s*\{[^}]*bottom:\s*19px[^}]*left:\s*24px/s);
  assert.match(exportDocumentScript({ markup: validMarkup, theme: "dark", density: "" }), /\[data-skill-count\]/, "展开对局的技能网格变量也应在导出页重建");
});

test("常规总览保持 2x，超长总览按像素预算安全降采样", () => {
  assert.equal(screenshotScale(SHARE_EXPORT_SURFACE_WIDTH, 4200), 2);
  const longScale = screenshotScale(SHARE_EXPORT_SURFACE_WIDTH, 20_000);
  assert.ok(longScale >= 1 && longScale < 2, `超长总览缩放异常：${longScale}`);
  assert.throws(() => screenshotScale(200, 300), /尺寸无效/);
});

function createHarness({ canceled = false, trusted = true, writeError = null, storedDirectory = "", selectedDirectory = "/tmp", existingFiles = [], directoryController } = {}) {
  const events = [];
  const writes = [];
  const files = new Map();
  const chosenDirectory = path.resolve(selectedDirectory);
  const directories = new Set([temporaryDirectory, userDataDirectory, chosenDirectory]);
  for (const file of existingFiles) files.set(path.resolve(file), Buffer.from("existing"));
  if (storedDirectory) {
    directories.add(path.resolve(storedDirectory));
    files.set(path.join(userDataDirectory, "share-export.json"), JSON.stringify({ saveDirectory: path.resolve(storedDirectory) }));
  }
  let destroyed = false;
  class FakeBrowserWindow {
    static fromWebContents(sender) { return { sender }; }
    constructor(options) {
      events.push(["window", options]);
      this.webContents = {
        setWindowOpenHandler(handler) { events.push(["window-open-handler", handler({}).action]); },
        async executeJavaScript(script) {
          events.push(["execute", script]);
          return { width: SHARE_EXPORT_SURFACE_WIDTH, height: 3600 };
        },
        debugger: {
          attach(version) { events.push(["debugger-attach", version]); },
          detach() { events.push(["debugger-detach"]); },
          async sendCommand(command, payload) {
            events.push([command, payload]);
            if (command === "Page.captureScreenshot") return { data: Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]).toString("base64") };
            return {};
          },
        },
      };
    }
    async loadURL(url) { events.push(["load", url]); }
    isDestroyed() { return destroyed; }
    destroy() { destroyed = true; events.push(["destroy"]); }
  }
  const dialog = {
    async showOpenDialog(_owner, options) {
      events.push(["dialog", options]);
      return canceled ? { canceled: true, filePaths: [] } : { canceled: false, filePaths: [chosenDirectory] };
    },
  };
  const fileSystem = {
    existsSync(filePath) { return files.has(path.resolve(filePath)) || directories.has(path.resolve(filePath)); },
    statSync(filePath) {
      if (!directories.has(path.resolve(filePath))) throw new Error("missing directory");
      return { isDirectory: () => true };
    },
    mkdirSync(directory) { directories.add(path.resolve(directory)); },
    readFileSync(filePath) {
      const value = files.get(path.resolve(filePath));
      if (value === undefined) throw new Error("missing file");
      return String(value);
    },
    writeFileSync(filePath, bytes) {
      events.push(["write", filePath]);
      if (path.basename(filePath) === "share-export.json") {
        files.set(path.resolve(filePath), String(bytes));
        return;
      }
      if (writeError) throw writeError;
      files.set(path.resolve(filePath), Buffer.from(bytes));
      writes.push({ filePath, bytes: Buffer.from(bytes) });
    },
  };
  const logs = [];
  const controller = createShareExportController({
    BrowserWindow: FakeBrowserWindow,
    directoryController,
    app: { getPath(name) { return name === "userData" ? userDataDirectory : temporaryDirectory; } },
    dialog,
    fileSystem,
    isTrustedRenderer: () => trusted,
    getBaseURL: () => "http://127.0.0.1:8787",
    log(message) { logs.push(message); },
    now: () => 1000,
    randomToken: () => validToken,
  });
  const sender = { id: 7 };
  return { controller, event: { sender }, events, writes, logs };
}

test("首次选择目录后持久化，后续自动保存且不覆盖同名 PNG", async () => {
  const harness = createHarness();
  const destination = await harness.controller.prepareSave(harness.event, "Deep Legends.png");
  assert.deepEqual(destination, { canceled: false, token: validToken, directory: temporaryDirectory, prompted: true });
  assert.equal(harness.events[0][0], "dialog");
  assert.equal(harness.events.some(([name]) => name === "window"), false, "选择路径前不应创建截图窗口");

  const result = await harness.controller.captureAndSave(harness.event, { token: validToken, markup: validMarkup, theme: "dark", density: "" });
  assert.equal(result.ok, true);
  assert.equal(result.fileName, "Deep Legends.png");
  assert.ok(result.pixelWidth > SHARE_EXPORT_SURFACE_WIDTH);
  assert.equal(harness.events.findIndex(([name]) => name === "dialog") < harness.events.findIndex(([name]) => name === "window"), true);
  assert.equal(harness.writes.length, 1);
  assert.equal(harness.writes[0].filePath, path.resolve("/tmp/Deep Legends.png"));
  assert.equal(harness.writes[0].bytes.subarray(1, 4).toString("ascii"), "PNG");
  const capture = harness.events.find(([name]) => name === "Page.captureScreenshot");
  assert.equal(capture[1].captureBeyondViewport, true);
  assert.equal(capture[1].clip.scale, 2);

  const next = await harness.controller.prepareSave(harness.event, "Deep Legends.png");
  assert.equal(next.prompted, false);
  await harness.controller.captureAndSave(harness.event, { token: validToken, markup: validMarkup, theme: "dark", density: "comfortable" });
  assert.equal(harness.events.filter(([name]) => name === "dialog").length, 1);
  assert.equal(harness.writes[1].filePath, path.resolve("/tmp/Deep Legends-2.png"));
});

test("取消保存不会创建截图窗口或写文件", async () => {
  const harness = createHarness({ canceled: true });
  assert.deepEqual(await harness.controller.prepareSave(harness.event, "share.png"), { canceled: true });
  assert.equal(harness.events.some(([name]) => name === "window"), false);
  assert.equal(harness.writes.length, 0);
});

test("保存令牌只能使用一次，写入失败会记录脱敏错误", async () => {
  const harness = createHarness({ writeError: new Error("disk full") });
  await harness.controller.prepareSave(harness.event, "share.png");
  await assert.rejects(() => harness.controller.captureAndSave(harness.event, { token: validToken, markup: validMarkup, theme: "", density: "" }), /生成失败/);
  assert.match(harness.logs.join("\n"), /disk full/);
  await assert.rejects(() => harness.controller.captureAndSave(harness.event, { token: validToken, markup: validMarkup, theme: "", density: "" }), /已失效/);
});

test("不受信任的 renderer 不能选择路径或提交截图", async () => {
  const harness = createHarness({ trusted: false });
  await assert.rejects(() => harness.controller.prepareSave(harness.event, "share.png"), /不受信任/);
  assert.throws(() => harness.controller.getSaveDirectory(harness.event), /不受信任/);
  await assert.rejects(() => harness.controller.chooseSaveDirectory(harness.event), /不受信任/);
  await assert.rejects(() => harness.controller.captureAndSave(harness.event, { token: validToken, markup: validMarkup }), /不受信任/);
});

test("设置接口读取并更改主进程保存目录", async () => {
  const harness = createHarness({ storedDirectory: "/tmp", selectedDirectory: "/exports" });
  assert.deepEqual(harness.controller.getSaveDirectory(harness.event), { directory: temporaryDirectory });
  assert.deepEqual(await harness.controller.chooseSaveDirectory(harness.event), { canceled: false, directory: path.resolve("/exports") });
  assert.deepEqual(harness.controller.getSaveDirectory(harness.event), { directory: path.resolve("/exports") });
});

test("preload 只暴露受校验的分享接口，不暴露 ipcRenderer", async () => {
  const preload = fs.readFileSync(path.join(__dirname, "preload.cjs"), "utf8");
  const exposed = new Map();
  const invokes = [];
  const sandbox = {
    require(name) {
      assert.equal(name, "electron");
      return {
        contextBridge: { exposeInMainWorld(name, value) { exposed.set(name, value); } },
        ipcRenderer: {
          send() {},
          invoke(channel, payload) { invokes.push([channel, payload]); return Promise.resolve({ ok: true }); },
        },
      };
    },
    Promise,
    Error,
  };
  vm.runInNewContext(preload, sandbox, { filename: "preload.cjs" });
  const share = exposed.get("desktopShare");
  assert.ok(share);
  assert.equal(Object.hasOwn(share, "ipcRenderer"), false);
  await share.preparePngSave("overview.png");
  await share.getSaveDirectory();
  await share.chooseSaveDirectory();
  await share.captureAndSavePng({ token: validToken, markup: validMarkup, theme: "dark", density: "" });
  assert.deepEqual(invokes.map(([channel]) => channel), ["desktop-share-prepare-save", "desktop-share-get-directory", "desktop-share-choose-directory", "desktop-share-capture-and-save"]);
  await assert.rejects(() => share.captureAndSavePng({ token: "bad", markup: validMarkup }), /内容无效/);
});


test("分享图独立窗口不携带主窗口 zoom，且使用独立内存会话", async () => {
  const harness = createHarness();
  await harness.controller.prepareSave(harness.event, "share.png");
  await harness.controller.captureAndSave(harness.event, { token: validToken, markup: validMarkup, theme: "dark", density: "" });
  const options = harness.events.find(([name]) => name === "window")[1];
  assert.equal(Object.hasOwn(options, "zoomFactor"), false);
  assert.equal(Object.hasOwn(options.webPreferences, "zoomFactor"), false);
  assert.equal(options.webPreferences.partition, "deep-legends-share-export");
  assert.equal(options.width, SHARE_EXPORT_SURFACE_WIDTH);
  const source = fs.readFileSync(path.join(__dirname, "share-export.cjs"), "utf8");
  assert.doesNotMatch(source, /applyUiScale|mainWindow\.webContents\.capturePage/);
});


test("R110 share capture writes through the common validated staging controller", async () => {
  const calls=[];
  const directoryController={
    getSaveDirectory(){return {directory:temporaryDirectory};},
    prepareFile(destination){calls.push(["prepare",destination]);return path.join(userDataDirectory,"stage.png");},
    async finalizeFile(file){calls.push(["finalize",file]);return path.join(temporaryDirectory,"shared-final.png");},
    discardFile(file){calls.push(["discard",file]);},
  };
  const harness=createHarness({directoryController});
  const prepared=await harness.controller.prepareSave(harness.event,"share");
  const result=await harness.controller.captureAndSave(harness.event,{token:prepared.token,markup:validMarkup});
  assert.equal(harness.writes[0].filePath,path.join(userDataDirectory,"stage.png"));
  assert.equal(result.fileName,"shared-final.png");
  assert.deepEqual(calls.map(row=>row[0]),["prepare","finalize","discard"]);
  await harness.controller.prepareSave(harness.event,"canceled");
  harness.controller.clear();assert.equal(calls.at(-1)[0],"discard");
});
