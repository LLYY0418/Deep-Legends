"use strict";

// First executable statement in the shell. Everything before this mark is
// Windows loading the executable plus Electron/Chromium cold boot -- on a
// freshly installed, unsigned build that also includes the antivirus scan of
// the new binaries, and no splash window can possibly appear during it.
// Recording it here is the only way to tell that cost apart from our own.
const startupMarks = { jsEntry: Date.now() };

const { app, BrowserWindow, dialog, ipcMain, nativeTheme, session, shell } = require("electron");
const { spawn } = require("node:child_process");
const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");

function installDesktopCrashDiagnostics() {
  const record = (kind, details) => {
    const { desktopCrashEvent } = require("./desktop-log.cjs");
    appendDesktopLog("进程异常 " + JSON.stringify(desktopCrashEvent(kind, details)));
  };
  app.on("render-process-gone", (_event, _contents, details) => record("render-process-gone", details));
  app.on("child-process-gone", (_event, details) => record("child-process-gone", details));
  // Monitoring writes synchronously without swallowing the uncaught exception
  // or replacing Node/Electron's fatal-exception behavior.
  process.on("uncaughtExceptionMonitor", error => record("uncaughtException", { name: error?.name }));
}
installDesktopCrashDiagnostics();

// Deliberately NOT required at module scope: none of these are needed before
// the splash window exists, and every require() resolved out of the asar
// archive delays the first frame the user sees.
let attachDiagnosticsExport = null;
let resolveSystemProxy = null;
let createShareExportController = null;
let windowBoundsForWorkArea = null;
let readWindowBounds = null;
let writeWindowBounds = null;
let autoScaleFor = null;
let normalizeScale = null;

function loadDeferredModules() {
  if (attachDiagnosticsExport) return;
  ({ attachDiagnosticsExport } = require("./diagnostics-export.cjs"));
  ({ resolveSystemProxy } = require("./proxy-resolution.cjs"));
  ({ createShareExportController } = require("./share-export.cjs"));
  ({ windowBoundsForWorkArea } = require("./window-bounds.cjs"));
  ({ readWindowBounds, writeWindowBounds } = require("./window-bounds-store.cjs"));
  ({ autoScaleFor, normalizeScale } = require("./ui-scale.cjs"));
}

const APP_ID = "cn.hexcore.lootassistant";
const READY_PREFIX = "LOOT_READY ";
const READY_TIMEOUT_MS = 25_000;
const SHUTDOWN_TIMEOUT_MS = 1_500;

let mainWindow = null;
let splashWindow = null;
let backend = null;
let backendReady = null;
let readyTimer = null;
let stdoutBuffer = "";
let quitting = false;
let shutdownStarted = false;
let rendererTheme = null;
let modalOpen = false;
let shareExportController = null;
let backendStarted = false;
let backendStartTimer = null;
let uiScalePreference = "auto";
let currentUiScale = 1;
let startupStageAgent = null;
const reportedStartupStages = new Set();

app.setAppUserModelId(APP_ID);

const hasInstanceLock = app.requestSingleInstanceLock();
if (!hasInstanceLock) {
  app.quit();
} else {
  app.on("second-instance", () => {
    if (!mainWindow) {
      if (startupMarks.splashWindowShown) { splashWindow?.show(); splashWindow?.focus(); }
      return;
    }
    if (mainWindow.isMinimized()) mainWindow.restore();
    mainWindow.show();
    mainWindow.focus();
  });
}

function projectRoot() {
  return path.resolve(__dirname, "..");
}

function backendSpec() {
  if (app.isPackaged) {
    // 后端通过 asarUnpack 直接随包分发，免去以前每次启动
    // “读取 asar → SHA256 校验 → 释放到用户目录”的成本。
    const command = path.join(process.resourcesPath, "app.asar.unpacked", "backend", "loot-service.exe");
    if (!fs.existsSync(command)) {
      const error = new Error("内置数据服务文件缺失或不完整，请重新下载完整客户端。");
      error.diagnostics = [`missing backend executable: ${command}`];
      throw error;
    }
    return {
      command,
      args: ["--desktop", "--no-browser"],
      cwd: path.dirname(command),
    };
  }
  const configured = process.env.LOOT_BACKEND;
  if (configured) {
    return { command: configured, args: ["--desktop", "--no-browser"], cwd: projectRoot() };
  }
  return { command: "go", args: ["run", ".", "--desktop", "--no-browser"], cwd: projectRoot() };
}

function iconPath() {
  return path.join(__dirname, "assets", process.platform === "win32" ? "hexcore-icon.ico" : "hexcore-icon-1024.png");
}

function createSplashWindow() {
  let logo = "";
  try {
    // Deliberately NOT hexcore-icon.ico: that file carries seven frames up to
    // 256px (160KB), which became a ~225KB data: URL that Chromium had to parse
    // and decode before the splash could paint anything. Measured cold-start
    // splash paint was 975ms. splash-mark.png is the single 128px frame lifted
    // out of the same icon (31KB), which is still sharp at the 94px render size
    // on HiDPI displays.
    logo = `data:image/png;base64,${fs.readFileSync(path.join(__dirname, "assets", "splash-mark.png")).toString("base64")}`;
  } catch (error) {
    appendDesktopLog(`启动标识读取失败：${error.message}`);
  }
  splashWindow = new BrowserWindow({
    width: 460,
    height: 292,
    show: false,
    frame: false,
    resizable: false,
    movable: true,
    center: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    backgroundColor: "#07090b",
    icon: iconPath(),
    webPreferences: {
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: true,
      webSecurity: true,
      devTools: false,
    },
  });
  splashWindow.once("ready-to-show", () => {
    if (quitting || !splashWindow || splashWindow.isDestroyed()) return;
    splashWindow.show();
    startupMarks.splashWindowShown = Date.now();
  });
  // Preserve the R77 renderer-load cue for backend startup. This historical
  // splashPaint mark is separate from actual window visibility above.
  splashWindow.webContents.once("did-finish-load", () => {
    if (!startupMarks.splashPainted) startupMarks.splashPainted = Date.now();
    startBackendOnce();
  });
  splashWindow.on("closed", () => { splashWindow = null; });
  const markup = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><style>
    :root{color-scheme:dark}*{box-sizing:border-box}body{margin:0;height:100vh;overflow:hidden;background:#07090b;color:#f1ede7;font-family:"Segoe UI","Microsoft YaHei UI",sans-serif;-webkit-user-select:none}main{position:relative;display:grid;height:100%;place-items:center;overflow:hidden;border:1px solid #332d26;background:radial-gradient(circle at 50% 36%,#20201c 0,#0d1012 48%,#07090b 76%)}main:before,main:after{position:absolute;width:260px;height:260px;border:1px solid #9c672f;clip-path:polygon(25% 7%,75% 7%,100% 50%,75% 93%,25% 93%,0 50%);content:"";opacity:.16}main:before{top:-168px;right:-92px;transform:rotate(16deg)}main:after{bottom:-190px;left:-112px;transform:rotate(-14deg)}section{position:relative;z-index:1;display:grid;justify-items:center;padding:28px;text-align:center}.mark{display:grid;width:94px;height:94px;place-items:center;margin-bottom:16px}.mark img{width:94px;height:94px;object-fit:contain;filter:drop-shadow(0 12px 24px #0009)}h1{margin:0;font-size:27px;line-height:1.2;letter-spacing:.02em}p{margin:7px 0 0;color:#aaa39a;font-size:12px}.progress{position:relative;width:176px;height:2px;margin-top:26px;overflow:hidden;background:#282522;border-radius:99px}.progress:after{position:absolute;inset:0;width:42%;background:linear-gradient(90deg,transparent,#d27a28,#f3b36d,transparent);content:"";animation:loading 1.2s ease-in-out infinite}@keyframes loading{from{transform:translateX(-110%)}to{transform:translateX(290%)}}@media(prefers-reduced-motion:reduce){.progress:after{animation-duration:.01ms;animation-iteration-count:1;width:100%}}
  </style></head><body><main><section><div class="mark">${logo ? `<img src="${logo}" alt="">` : ""}</div><h1>Deep Legends</h1><p id="splash-status">正在启动本地数据服务</p><div class="progress" aria-hidden="true"></div></section></main></body></html>`;
  splashWindow.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(markup)}`).catch((error) => appendDesktopLog(`启动页加载失败：${error.message}`));
}

function setSplashStatus(text) {
  if (!splashWindow || splashWindow.isDestroyed()) return;
  const script = `(() => { const node = document.getElementById("splash-status"); if (node) node.textContent = ${JSON.stringify(String(text))}; })();`;
  splashWindow.webContents.executeJavaScript(script, true).catch(() => {});
}

function closeSplashWindow() {
  if (!splashWindow || splashWindow.isDestroyed()) return;
  splashWindow.close();
}

// Each stage is submitted when it happens, even if the main window never shows.
// One socket preserves event order without waiting on diagnostics to build or
// reveal a window. This is separate from the existing success-only summary.
function reportStartupStage(stage) {
  if (!backendReady || reportedStartupStages.has(stage)) return;
  reportedStartupStages.add(stage);
  const body = Buffer.from(JSON.stringify({ stage, elapsedMs: Math.max(0, Date.now() - startupMarks.jsEntry) }), "utf8");
  try {
    if (!startupStageAgent) startupStageAgent = new http.Agent({ keepAlive: true, maxSockets: 1 });
    const request = http.request(`${backendReady.baseUrl}/api/diagnostics/startup-stage`, {
      method: "POST", agent: startupStageAgent,
      headers: { "X-Local-Token": backendReady.token, "Content-Type": "application/json", "Content-Length": body.length },
      timeout: 2000,
    }, (response) => {
      if (response.statusCode !== 204) appendDesktopLog(`启动阶段记录失败：${stage} HTTP ${response.statusCode}`);
      response.resume();
    });
    request.once("error", () => appendDesktopLog(`启动阶段发送失败：${stage}`));
    request.once("timeout", () => request.destroy());
    request.end(body);
  } catch (_) { appendDesktopLog(`启动阶段发送失败：${stage}`); }
}

// Keep the historical timing summary and payload unchanged; it is emitted only
// after the main window is shown and remains the A/B collector's data source.
function reportStartupPhases() {
  const marks = startupMarks;
  if (marks.reported || !backendReady) return;
  marks.reported = true;
  // Electron exposes the real OS process creation time; falling back to the
  // first JS statement would silently hide the Electron boot cost, which is
  // the exact thing we are trying to measure.
  const created = typeof process.getCreationTime === "function" ? process.getCreationTime() : null;
  const origin = Number.isFinite(created) ? created : marks.jsEntry;
  const span = (from, to) => (Number.isFinite(from) && Number.isFinite(to) ? Math.max(0, Math.round(to - from)) : 0);
  const payload = {
    processToJs: span(origin, marks.jsEntry),
    jsToReady: span(marks.jsEntry, marks.appReady),
    readyToSplash: span(marks.appReady, marks.splashCreated),
    splashPaint: span(marks.splashCreated, marks.splashPainted),
    splashWindowShown: span(origin, marks.splashWindowShown),
    spawnToReady: span(marks.backendSpawn, marks.backendReady),
    readyToWindow: span(marks.backendReady, marks.mainShown),
    total: span(origin, marks.mainShown),
  };
  appendDesktopLog(`启动耗时 ${JSON.stringify(payload)}`);
  const body = Buffer.from(JSON.stringify(payload), "utf8");
  const request = http.request(`${backendReady.baseUrl}/api/diagnostics/startup`, {
    method: "POST",
    headers: { "X-Local-Token": backendReady.token, "Content-Type": "application/json", "Content-Length": body.length },
    timeout: 2000,
  }, (response) => response.resume());
  request.once("error", () => {});
  request.once("timeout", () => request.destroy());
  request.end(body);
}

function titleBarOverlay(theme, dimmed = modalOpen, scale = currentUiScale) {
  const dark = theme ? theme === "dark" : nativeTheme.shouldUseDarkColors;
  // CSS topbar is 56px; the native overlay takes DIP, so it includes zoom.
  const height = Math.round(56 * scale);
  if (dimmed) return { color: dark ? "#05070B" : "#8a8a8a", symbolColor: "#ffffff", height };
  return { color: dark ? "#0B0E14" : "#ffffff", symbolColor: dark ? "#E8EAF0" : "#24211f", height };
}

function syncTitleBar(scale = currentUiScale) {
  if (process.platform === "win32" && mainWindow && !mainWindow.isDestroyed()) mainWindow.setTitleBarOverlay(titleBarOverlay(rendererTheme, modalOpen, scale));
}

function uiScaleState(window = mainWindow) {
  const auto = autoScaleFor(window.getContentBounds());
  return { mode: uiScalePreference === "auto" ? "auto" : "fixed", value: uiScalePreference === "auto" ? auto : uiScalePreference, auto };
}

function applyUiScale(window, scale, force = false) {
  // The renderer performs the actual zoom (CSS `zoom` on .app-frame) so that the
  // browser preview scales as well; the shell must NOT call setZoomFactor here or
  // the two would multiply. Only the native chrome follows the step: the Windows
  // caption overlay is DIP while --topbar-height is CSS px, and the minimum size
  // has to grow or a 780px window would drop into the phone breakpoints.
  if (!window || window !== mainWindow || window.isDestroyed()) return;
  if (!force && currentUiScale === scale) return;
  currentUiScale = scale;
  syncTitleBar(scale);
  window.setMinimumSize(Math.round(780 * scale), Math.round(600 * scale));
}

function appendDesktopLog(message) {
  try { require("./desktop-log.cjs").appendDesktopLogFile(path.join(app.getPath("userData"), "logs"), message); } catch (_) {}
}

// Spawning the backend is NOT free on the main thread: libuv runs CreateProcessW
// inline inside uv_spawn, and on a freshly installed unsigned build Windows
// scans the new image while the process is being created. An earlier attempt to
// start it before app.whenReady() -- on the theory that the backend is the long
// pole -- backfired badly on real hardware: js_to_ready went 170ms -> 1557ms and
// the splash slid from 2630ms to 4038ms after process start, which is exactly
// the "nothing on screen" wait being complained about. Pixels first, then the
// expensive work. The timer is a floor, not a schedule: if the splash renderer
// never reports back we must still boot.
function startBackendOnce() {
  if (quitting) return;
  if (backendStarted) return;
  backendStarted = true;
  clearTimeout(backendStartTimer);
  backendStartTimer = null;
  startBackend();
  // Warm the deferred modules in the same window. The backend needs ~1.9s to
  // answer on a cold start and nothing else competes for the main thread until
  // it does, so paying the require cost here keeps it off the path between
  // "backend ready" and "main window visible".
  loadDeferredModules();
}

function startBackend() {
  let spec;
  try {
    spec = backendSpec();
  } catch (error) {
    if (Array.isArray(error.diagnostics)) appendDesktopLog(error.diagnostics.join(" | "));
    failStartup(`本地数据服务准备失败：${error.message}`);
    return;
  }
  const childEnvironment = { ...process.env };
  delete childEnvironment.ELECTRON_RUN_AS_NODE;
  startupMarks.backendSpawn = Date.now();
  backend = spawn(spec.command, spec.args, {
    cwd: spec.cwd,
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"],
    env: childEnvironment,
  });
  readyTimer = setTimeout(() => failStartup("本地数据服务启动超时，请重新打开客户端。"), READY_TIMEOUT_MS);
  backend.stdout.setEncoding("utf8");
  backend.stdout.on("data", onBackendStdout);
  backend.stderr.setEncoding("utf8");
  backend.stderr.on("data", appendDesktopLog);
  backend.on("error", (error) => failStartup(`本地数据服务无法启动：${error.message}`));
  // close follows stdout EOF; exit may precede the final LOOT_QUIT message.
  backend.on("close", (code, signal) => {
    clearTimeout(readyTimer);
    backend = null;
    if (!quitting && !shutdownStarted) failStartup(`本地数据服务已退出（${signal || code || "未知原因"}）。`);
  });
}

function onBackendStdout(chunk) {
  stdoutBuffer += chunk;
  const lines = stdoutBuffer.split(/\r?\n/);
  stdoutBuffer = lines.pop() || "";
  for (const line of lines) {
    if (backendReady && (line === "LOOT_QUIT update" || line === "LOOT_QUIT user")) {
      quitting = true;
      shutdownStarted = true;
      clearTimeout(readyTimer);
      app.quit();
      continue;
    }
    if (!line.startsWith(READY_PREFIX)) continue;
    try {
      const payload = JSON.parse(line.slice(READY_PREFIX.length));
      acceptReadyPayload(payload);
    } catch (_) {
      failStartup("本地数据服务返回了无效的启动信息。未写入任何客户端凭据。");
    }
  }
}

function acceptReadyPayload(payload) {
  if (backendReady) return;
  const base = new URL(payload.baseUrl);
  const bootstrap = new URL(payload.bootstrapUrl);
  if (base.protocol !== "http:" || bootstrap.origin !== base.origin || !isLoopback(base.hostname) || typeof payload.token !== "string" || payload.token.length < 32) {
    failStartup("本地数据服务的监听地址未通过安全校验。");
    return;
  }
  clearTimeout(readyTimer);
  backendReady = { baseUrl: base.origin, bootstrapUrl: bootstrap.toString(), token: payload.token };
  startupMarks.backendReady = Date.now();
  reportStartupStage("backend_ready");
  setSplashStatus("正在加载界面");
  createMainWindow();
  void pushSystemProxy();
}

// 系统代理解析不再阻塞后端启动：后端就绪后异步解析并下发，
// 英雄数据的联网请求在拿到结果前按“系统/环境代理”规则直连。
async function pushSystemProxy() {
  loadDeferredModules();
  let proxy = "";
  try {
    proxy = await resolveSystemProxy(session.defaultSession, "https://lol-api-champion.op.gg/");
  } catch (_) {}
  if (!backendReady) return;
  const body = JSON.stringify({ proxy });
  const request = http.request(`${backendReady.baseUrl}/api/system-proxy`, {
    method: "POST",
    headers: {
      "X-Local-Token": backendReady.token,
      "Content-Type": "application/json",
      "Content-Length": Buffer.byteLength(body),
    },
    timeout: 3000,
  }, (response) => response.resume());
  request.once("error", (error) => appendDesktopLog(`系统代理下发失败：${error.message}`));
  request.once("timeout", () => request.destroy());
  request.end(body);
}

function isLoopback(hostname) {
  return hostname === "127.0.0.1" || hostname === "::1" || hostname === "[::1]" || hostname === "localhost";
}

// Size the window to the current display so it opens at a comfortable
// proportion on any resolution (small laptops through 4K monitors).
function initialWindowBounds() {
  const { screen } = require("electron");
  const display = screen.getPrimaryDisplay();
  return windowBoundsForWorkArea(display.workAreaSize);
}

function createMainWindow() {
  loadDeferredModules();
  const { screen } = require("electron");
  const boundsPath = path.join(app.getPath("userData"), "window-bounds.json");
  const scalePath = path.join(app.getPath("userData"), "ui-scale.json");
  uiScalePreference = "auto";
  currentUiScale = 1;
  try {
    if (fs.statSync(scalePath).size <= 1024) {
      const stored = JSON.parse(fs.readFileSync(scalePath, "utf8"));
      uiScalePreference = stored.mode === "fixed" ? normalizeScale(stored.value) : "auto";
    }
  } catch (_) { /* Missing or invalid preferences use auto. */ }
  const storedBounds = readWindowBounds(boundsPath, screen.getAllDisplays());
  const bounds = storedBounds || initialWindowBounds();
  mainWindow = new BrowserWindow({
    title: "Deep Legends",
    width: bounds.width,
    height: bounds.height,
    minWidth: 780,
    minHeight: 600,
    ...(storedBounds ? { x: bounds.x, y: bounds.y } : { center: true }),
    show: false,
    autoHideMenuBar: true,
    backgroundColor: titleBarOverlay(rendererTheme).color,
    icon: iconPath(),
    titleBarStyle: "hidden",
    titleBarOverlay: process.platform === "win32" ? titleBarOverlay() : undefined,
    webPreferences: {
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: true,
      preload: path.join(__dirname, "preload.cjs"),
      webSecurity: true,
      devTools: !app.isPackaged,
      spellcheck: false,
    },
  });
  reportStartupStage("main_window_created");
  if (storedBounds?.maximized) mainWindow.maximize();
  let boundsWriteTimer = null;
  const persistBounds = () => {
    if (!mainWindow || mainWindow.isDestroyed()) return;
    const normal = mainWindow.isMaximized() ? mainWindow.getNormalBounds() : mainWindow.getBounds();
    writeWindowBounds(boundsPath, { ...normal, maximized: mainWindow.isMaximized() });
  };
  const scheduleBoundsWrite = () => {
    clearTimeout(boundsWriteTimer);
    boundsWriteTimer = setTimeout(() => {
      if (!mainWindow || mainWindow.isDestroyed()) return;
      applyUiScale(mainWindow, uiScaleState().value);
      publishUiScale();
      persistBounds();
    }, 300);
  };
  mainWindow.on("resize", scheduleBoundsWrite);
  mainWindow.on("move", scheduleBoundsWrite);
  mainWindow.on("close", () => {
    clearTimeout(boundsWriteTimer);
    persistBounds();
  });
  mainWindow.once("ready-to-show", () => {
    reportStartupStage("main_window_ready_to_show");
    mainWindow?.show();
    closeSplashWindow();
    startupMarks.mainShown = Date.now();
    reportStartupPhases();
  });
  let lastScaleState = "";
  const publishUiScale = (force = false) => {
    const state = uiScaleState();
    const serialized = JSON.stringify(state);
    if (!force && serialized === lastScaleState) return;
    lastScaleState = serialized;
    mainWindow.webContents.send("desktop-scale-changed", state);
  };
  mainWindow.webContents.on("did-finish-load", () => {
    reportStartupStage("main_window_did_finish_load");
    applyUiScale(mainWindow, uiScaleState().value, true);
    publishUiScale(true);
  });
  const onDisplayMetricsChanged = () => {
    applyUiScale(mainWindow, uiScaleState().value);
    publishUiScale();
  };
  screen.on("display-metrics-changed", onDisplayMetricsChanged);
  const onThemeUpdated = () => syncTitleBar();
  nativeTheme.on("updated", onThemeUpdated);
  ipcMain.removeHandler("desktop-scale-get");
  ipcMain.handle("desktop-scale-get", (event) => {
    if (!isTrustedRenderer(event.sender) || event.sender !== mainWindow?.webContents) return null;
    return uiScaleState();
  });
  ipcMain.removeAllListeners("desktop-scale-set");
  ipcMain.on("desktop-scale-set", (event, mode, value) => {
    if (!isTrustedRenderer(event.sender)) return;
    if (event.sender !== mainWindow?.webContents || !["auto", "fixed"].includes(mode)) return;
    const next = mode === "auto" ? "auto" : normalizeScale(value);
    if (mode === "fixed" && next === "auto") return;
    if (next !== uiScalePreference) {
      uiScalePreference = next;
      try {
        fs.mkdirSync(path.dirname(scalePath), { recursive: true });
        fs.writeFileSync(`${scalePath}.tmp`, JSON.stringify({ mode, value: next }), { mode: 0o600 });
        fs.renameSync(`${scalePath}.tmp`, scalePath);
      } catch (error) { appendDesktopLog(`界面缩放偏好保存失败：${error.message}`); }
    }
    applyUiScale(mainWindow, uiScaleState().value);
    publishUiScale();
  });
  ipcMain.removeAllListeners("desktop-scale-applied");
  ipcMain.on("desktop-scale-applied", (event, scale) => {
    if (!isTrustedRenderer(event.sender) || event.sender !== mainWindow?.webContents) return;
    const step = normalizeScale(scale);
    if (step === "auto") return;
    applyUiScale(mainWindow, step, true);
  });
  shareExportController?.clear();
  const windowShareExportController = createShareExportController({
    BrowserWindow,
    app,
    dialog,
    fileSystem: fs,
    isTrustedRenderer,
    getBaseURL: () => backendReady?.baseUrl || "",
    log: appendDesktopLog,
  });
  shareExportController = windowShareExportController;
  let lastDiagnosticsFile = "";
  const removeDiagnosticsExport = attachDiagnosticsExport({
    session: mainWindow.webContents.session,
    sender: mainWindow.webContents,
    getBaseURL: () => backendReady?.baseUrl || "",
    getDefaultDirectory: () => app.getPath("downloads"),
    fileSystem: fs,
    getDesktopLog: () => require("./desktop-log.cjs").desktopLogForExport(path.join(app.getPath("userData"), "logs")),
    onCompleted(file) {
      lastDiagnosticsFile = file;
      if (!mainWindow?.webContents.isDestroyed()) mainWindow.webContents.send("desktop-diagnostics-completed");
    },
  });
  ipcMain.removeHandler("desktop-diagnostics-open-folder");
  ipcMain.handle("desktop-diagnostics-open-folder", (event) => {
    if ((event.sender !== mainWindow?.webContents || !isTrustedRenderer(event.sender)) || !lastDiagnosticsFile) return false;
    shell.showItemInFolder(lastDiagnosticsFile);
    lastDiagnosticsFile = "";
    return true;
  });
  mainWindow.webContents.once("destroyed", removeDiagnosticsExport);
  ipcMain.removeHandler("desktop-share-prepare-save");
  ipcMain.handle("desktop-share-prepare-save", (event, suggestedName) => windowShareExportController.prepareSave(event, suggestedName));
  ipcMain.removeHandler("desktop-share-get-directory");
  ipcMain.handle("desktop-share-get-directory", (event) => windowShareExportController.getSaveDirectory(event));
  ipcMain.removeHandler("desktop-share-choose-directory");
  ipcMain.handle("desktop-share-choose-directory", (event) => windowShareExportController.chooseSaveDirectory(event));
  ipcMain.removeHandler("desktop-share-capture-and-save");
  ipcMain.handle("desktop-share-capture-and-save", (event, payload) => windowShareExportController.captureAndSave(event, payload));
  ipcMain.removeAllListeners("desktop-theme");
  ipcMain.on("desktop-theme", (_event, theme) => {
    if (process.platform === "win32" && mainWindow && !mainWindow.isDestroyed() && (theme === "dark" || theme === "light")) {
      rendererTheme = theme;
      syncTitleBar();
      mainWindow.setBackgroundColor(titleBarOverlay(rendererTheme).color);
    }
  });
  ipcMain.removeAllListeners("desktop-modal");
  ipcMain.on("desktop-modal", (_event, open) => {
    const nextModalOpen = Boolean(open);
    if (modalOpen === nextModalOpen) return;
    modalOpen = nextModalOpen;
    syncTitleBar();
  });
  let forcingWindowed = false;
  const forceWindowed = () => {
    if (forcingWindowed || !mainWindow || mainWindow.isDestroyed()) return;
    forcingWindowed = true;
    if (mainWindow.isFullScreen()) mainWindow.setFullScreen(false);
    else if (typeof mainWindow.isSimpleFullScreen === "function" && mainWindow.isSimpleFullScreen()) mainWindow.setSimpleFullScreen(false);
    setImmediate(() => { forcingWindowed = false; });
  };
  ipcMain.removeAllListeners("desktop-fullscreen-exit");
  ipcMain.on("desktop-fullscreen-exit", (event) => {
    if (!isTrustedRenderer(event.sender)) return;
    forceWindowed();
  });
  mainWindow.webContents.on("leave-html-full-screen", () => setImmediate(forceWindowed));
  mainWindow.webContents.once("did-finish-load", () => {
    const screenshotPath = !app.isPackaged ? process.env.LOOT_SCREENSHOT_PATH : "";
    if (!screenshotPath) return;
    setTimeout(async () => {
      try {
        const image = await mainWindow?.webContents.capturePage();
        if (image) fs.writeFileSync(path.resolve(screenshotPath), image.toPNG());
      } catch (error) {
        appendDesktopLog(`自动截图失败：${error.message}`);
      } finally {
        app.quit();
      }
    }, 1200);
  });
  mainWindow.on("closed", () => {
    clearTimeout(boundsWriteTimer);
    nativeTheme.removeListener("updated", onThemeUpdated);
    screen.removeListener("display-metrics-changed", onDisplayMetricsChanged);
    ipcMain.removeHandler("desktop-scale-get");
    ipcMain.removeAllListeners("desktop-scale-set");
    windowShareExportController.clear();
    if (shareExportController === windowShareExportController) shareExportController = null;
    mainWindow = null;
    if (!quitting) app.quit();
  });
  const allowedOrigin = new URL(backendReady.baseUrl).origin;
  mainWindow.webContents.on("will-navigate", (event, target) => {
    const parsed = safeURL(target);
    if (!parsed || parsed.origin !== allowedOrigin) event.preventDefault();
  });
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    const parsed = safeURL(url);
    if (parsed && parsed.protocol === "https:") void shell.openExternal(parsed.toString());
    return { action: "deny" };
  });
  mainWindow.loadURL(backendReady.bootstrapUrl).catch((error) => failStartup(`客户端界面加载失败：${error.message}`));
}

function safeURL(value) {
  try { return new URL(value); } catch (_) { return null; }
}

function isTrustedRenderer(webContents) {
  if (!backendReady || !webContents || webContents.isDestroyed()) return false;
  const current = safeURL(webContents.getURL());
  return Boolean(current && current.origin === new URL(backendReady.baseUrl).origin);
}

function failStartup(message) {
  clearTimeout(readyTimer);
  clearTimeout(backendStartTimer);
  appendDesktopLog(message);
  if (quitting) return;
  void dialog.showMessageBox({ type: "error", title: "Deep Legends", message, detail: "可在本地数据目录的 logs 文件夹查看脱敏日志。" }).finally(() => {
    closeSplashWindow();
    quitting = true;
    if (backend) backend.kill();
    app.quit();
  });
}

function requestBackendQuit() {
  return new Promise((resolve) => {
    if (!backendReady) { resolve(); return; }
    const request = http.request(`${backendReady.baseUrl}/api/quit`, {
      method: "POST",
      headers: { "X-Local-Token": backendReady.token, "Content-Length": "0" },
      timeout: 800,
    }, (response) => {
      response.resume();
      response.once("end", resolve);
    });
    request.once("error", resolve);
    request.once("timeout", () => { request.destroy(); resolve(); });
    request.end();
  });
}

async function shutdownBackend() {
  if (shutdownStarted) return;
  shutdownStarted = true;
  await Promise.race([requestBackendQuit(), new Promise((resolve) => setTimeout(resolve, SHUTDOWN_TIMEOUT_MS))]);
  if (backend) backend.kill();
}

app.whenReady().then(() => {
  if (!hasInstanceLock) return;
  startupMarks.appReady = Date.now();
  session.defaultSession.setPermissionCheckHandler((webContents, permission) => permission === "fullscreen" && isTrustedRenderer(webContents));
  session.defaultSession.setPermissionRequestHandler((webContents, permission, callback) => callback(permission === "fullscreen" && isTrustedRenderer(webContents)));
  createSplashWindow();
  startupMarks.splashCreated = Date.now();
  // Normally the splash's did-finish-load starts the backend a few frames from
  // now. This is the fallback for a splash that fails to render at all.
  backendStartTimer = setTimeout(startBackendOnce, 400);
});

app.on("activate", () => {
  if (mainWindow) mainWindow.show();
  else if (startupMarks.splashWindowShown) splashWindow?.show();
});

app.on("before-quit", (event) => {
  clearTimeout(backendStartTimer);
  if (quitting) return;
  event.preventDefault();
  quitting = true;
  void shutdownBackend().finally(() => app.quit());
});

app.on("window-all-closed", () => app.quit());
