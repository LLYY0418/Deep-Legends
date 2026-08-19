"use strict";

const { app, BrowserWindow, dialog, ipcMain, nativeTheme, session, shell } = require("electron");
const { spawn } = require("node:child_process");
const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");
const { resolveSystemProxy } = require("./proxy-resolution.cjs");

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

app.setAppUserModelId(APP_ID);

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", () => {
    if (!mainWindow) {
      splashWindow?.show();
      splashWindow?.focus();
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
    logo = `data:image/x-icon;base64,${fs.readFileSync(path.join(__dirname, "assets", "hexcore-icon.ico")).toString("base64")}`;
  } catch (error) {
    appendDesktopLog(`启动标识读取失败：${error.message}`);
  }
  splashWindow = new BrowserWindow({
    width: 460,
    height: 292,
    show: true,
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
    if (splashWindow && !splashWindow.isDestroyed() && !splashWindow.isVisible()) splashWindow.show();
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

function titleBarOverlay(theme, dimmed = modalOpen) {
  const dark = theme ? theme === "dark" : nativeTheme.shouldUseDarkColors;
  // 高度需与前端 app.css 的 --topbar-height 保持一致，否则系统窗口按钮与顶栏错位。
  if (dimmed) return { color: dark ? "#05070B" : "#8a8a8a", symbolColor: "#ffffff", height: 56 };
  return { color: dark ? "#0B0E14" : "#ffffff", symbolColor: dark ? "#E8EAF0" : "#24211f", height: 56 };
}

function appendDesktopLog(message) {
  const safe = String(message || "")
    .replace(/bootstrap=[^\s&]+/gi, "bootstrap=[redacted]")
    .replace(/(?:token|authorization)["'\s:=]+[^\s,"']+/gi, "$1=[redacted]")
    .trim();
  if (!safe) return;
  try {
    const directory = path.join(app.getPath("userData"), "logs");
    fs.mkdirSync(directory, { recursive: true });
    fs.appendFileSync(path.join(directory, "desktop.log"), `${new Date().toISOString()} ${safe.slice(0, 2000)}\n`, "utf8");
  } catch (_) {}
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
  backend.on("exit", (code, signal) => {
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
  setSplashStatus("正在加载界面");
  createMainWindow();
  void pushSystemProxy();
}

// 系统代理解析不再阻塞后端启动：后端就绪后异步解析并下发，
// 英雄数据的联网请求在拿到结果前按“系统/环境代理”规则直连。
async function pushSystemProxy() {
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
  const area = screen.getPrimaryDisplay().workAreaSize;
  const width = Math.min(1680, Math.max(1080, Math.round(area.width * 0.84)));
  const height = Math.min(1050, Math.max(680, Math.round(area.height * 0.88)));
  return { width, height };
}

function createMainWindow() {
  const bounds = initialWindowBounds();
  mainWindow = new BrowserWindow({
    title: "Deep Legends",
    width: bounds.width,
    height: bounds.height,
    minWidth: 780,
    minHeight: 600,
    center: true,
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
  mainWindow.once("ready-to-show", () => {
    mainWindow?.show();
    closeSplashWindow();
  });
  const syncTitleBar = () => {
    if (process.platform === "win32" && mainWindow && !mainWindow.isDestroyed()) mainWindow.setTitleBarOverlay(titleBarOverlay(rendererTheme));
  };
  nativeTheme.on("updated", syncTitleBar);
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
    nativeTheme.removeListener("updated", syncTitleBar);
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
  session.defaultSession.setPermissionCheckHandler((webContents, permission) => permission === "fullscreen" && isTrustedRenderer(webContents));
  session.defaultSession.setPermissionRequestHandler((webContents, permission, callback) => callback(permission === "fullscreen" && isTrustedRenderer(webContents)));
  createSplashWindow();
  setImmediate(startBackend);
});

app.on("activate", () => {
  if (mainWindow) mainWindow.show();
  else splashWindow?.show();
});

app.on("before-quit", (event) => {
  if (quitting) return;
  event.preventDefault();
  quitting = true;
  void shutdownBackend().finally(() => app.quit());
});

app.on("window-all-closed", () => app.quit());
