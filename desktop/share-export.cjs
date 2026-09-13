"use strict";

const crypto = require("node:crypto");
const path = require("node:path");

const SHARE_EXPORT_CONTENT_WIDTH = 1440;
const SHARE_EXPORT_SURFACE_WIDTH = 1488;
const SHARE_EXPORT_WINDOW_HEIGHT = 900;
const SHARE_EXPORT_MAX_MARKUP = 8 * 1024 * 1024;
const SHARE_EXPORT_MAX_EDGE = 32760;
const SHARE_EXPORT_MAX_PIXELS = 64_000_000;
const SHARE_EXPORT_SCALE = 2;
const SHARE_SAVE_TTL_MS = 10 * 60 * 1000;
const SHARE_SETTINGS_FILE = "share-export.json";

const ALLOWED_THEMES = new Set(["", "light", "dark", "azure", "emerald", "violet", "crimson", "aurora", "oled"]);
const ALLOWED_DENSITIES = new Set(["", "compact"]);

function sanitizeSuggestedName(value, date = new Date()) {
  const fallback = `deep-legends-overview-${date.toISOString().slice(0, 10)}.png`;
  const cleaned = String(value || "")
    .replace(/[<>:"/\\|?*\x00-\x1f]/g, "-")
    .replace(/[. ]+$/g, "")
    .trim()
    .slice(0, 120);
  if (!cleaned) return fallback;
  return /\.png$/i.test(cleaned) ? cleaned : `${cleaned}.png`;
}

function ensurePngPath(filePath) {
  const parsed = path.parse(path.resolve(String(filePath || "")));
  if (!parsed.base) throw new Error("未选择有效的保存位置。");
  if (parsed.ext.toLowerCase() === ".png") return path.format(parsed);
  return path.join(parsed.dir, `${parsed.name || "deep-legends-overview"}.png`);
}

function normalizeSaveDirectory(value) {
  const directory = String(value || "").trim();
  if (!directory || directory.length > 2048 || !path.isAbsolute(directory)) return "";
  return path.normalize(directory);
}

function uniquePngPath(fileSystem, directory, suggestedName) {
  const fileName = sanitizeSuggestedName(suggestedName);
  const parsed = path.parse(fileName);
  for (let index = 1; index <= 999; index++) {
    const suffix = index === 1 ? "" : `-${index}`;
    const candidate = path.join(directory, `${parsed.name}${suffix}.png`);
    if (!fileSystem.existsSync(candidate)) return candidate;
  }
  return path.join(directory, `${parsed.name}-${Date.now()}.png`);
}

function normalizeCapturePayload(payload) {
  const token = typeof payload?.token === "string" ? payload.token.trim() : "";
  const markup = typeof payload?.markup === "string" ? payload.markup : "";
  const theme = typeof payload?.theme === "string" ? payload.theme : "";
  const rawDensity = typeof payload?.density === "string" ? payload.density : "";
  const density = rawDensity === "comfortable" ? "" : rawDensity;
  if (!/^[A-Za-z0-9_-]{24,128}$/.test(token)) throw new Error("保存请求已失效，请重新选择位置。");
  if (!markup.startsWith('<div class="overview-share-surface"') || markup.length > SHARE_EXPORT_MAX_MARKUP) {
    throw new Error("分享图内容无效或过大。");
  }
  if (!ALLOWED_THEMES.has(theme)) throw new Error("分享图主题无效。");
  if (!ALLOWED_DENSITIES.has(density)) throw new Error("分享图界面密度无效。");
  return { token, markup, theme, density };
}

function screenshotScale(width, height) {
  const safeWidth = Number(width);
  const safeHeight = Number(height);
  if (!Number.isFinite(safeWidth) || !Number.isFinite(safeHeight) || safeWidth < 1000 || safeWidth > 2000 || safeHeight < 300 || safeHeight > 100_000) {
    throw new Error("分享图尺寸无效。");
  }
  const edgeScale = SHARE_EXPORT_MAX_EDGE / Math.max(safeWidth, safeHeight);
  const pixelScale = Math.sqrt(SHARE_EXPORT_MAX_PIXELS / (safeWidth * safeHeight));
  const scale = Math.min(SHARE_EXPORT_SCALE, edgeScale, pixelScale);
  if (!Number.isFinite(scale) || scale < 0.25) throw new Error("总览内容过长，无法生成单张分享图。");
  return Math.max(0.25, Math.floor(scale * 100) / 100);
}

function exportDocumentScript(payload) {
  const serialized = JSON.stringify(payload).replace(/</g, "\\u003c");
  return `(async () => {
    const payload = ${serialized};
    const rootElement = document.documentElement;
    if (payload.theme) rootElement.dataset.theme = payload.theme;
    else delete rootElement.dataset.theme;
    if (payload.density) rootElement.dataset.density = payload.density;
    else delete rootElement.dataset.density;
    document.body.className = "overview-share-document";
    document.body.replaceChildren();
    document.body.insertAdjacentHTML("afterbegin", payload.markup);
    const surface = document.querySelector(".overview-share-surface");
    if (!surface) throw new Error("分享图页面构建失败。");

    for (const fill of surface.querySelectorAll("[data-bar-width]")) {
      fill.style.width = Math.max(0, Math.min(100, Number(fill.dataset.barWidth) || 0)) + "%";
    }
    for (const fill of surface.querySelectorAll("[data-blue-share]")) {
      fill.style.setProperty("--blue-share", Math.max(0, Math.min(100, Number(fill.dataset.blueShare) || 0)) + "%");
    }
    for (const ring of surface.querySelectorAll("[data-win-rate]")) {
      ring.style.setProperty("--recent-win-rate", Math.max(0, Math.min(100, Number(ring.dataset.winRate) || 0)) + "%");
    }
    for (const grid of surface.querySelectorAll("[data-skill-count]")) {
      grid.style.setProperty("--skill-count", String(Math.max(1, Math.min(18, Number(grid.dataset.skillCount) || 1))));
    }

    const images = [...surface.querySelectorAll("img")];
    await Promise.all(images.map((image) => new Promise((resolve) => {
      if (image.complete) { resolve(); return; }
      const finish = () => resolve();
      image.addEventListener("load", finish, { once: true });
      image.addEventListener("error", finish, { once: true });
      setTimeout(finish, 6000);
    })));
    for (const image of images) {
      if (image.naturalWidth > 0) image.parentElement?.classList.add("has-loaded-image");
      else if (image.matches("[data-game-image]")) {
        image.hidden = true;
        image.parentElement?.classList.remove("has-loaded-image");
      }
    }
    await document.fonts?.ready;
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const rect = surface.getBoundingClientRect();
    return {
      width: Math.ceil(Math.max(rect.width, surface.scrollWidth)),
      height: Math.ceil(Math.max(rect.height, surface.scrollHeight)),
    };
  })()`;
}

async function renderOverviewPng(BrowserWindow, baseURL, payload) {
  let exportWindow = null;
  let debuggerAttached = false;
  try {
    exportWindow = new BrowserWindow({
      width: SHARE_EXPORT_SURFACE_WIDTH,
      height: SHARE_EXPORT_WINDOW_HEIGHT,
      useContentSize: true,
      show: false,
      backgroundColor: "#0B0E14",
      webPreferences: {
        nodeIntegration: false,
        contextIsolation: true,
        sandbox: true,
        webSecurity: true,
        devTools: false,
        backgroundThrottling: false,
        offscreen: true,
        // Electron shares page zoom per origin within a session, even across
        // BrowserWindows. This in-memory session keeps exports at 100%; the
        // normal top-level navigation obtains its own local session cookie.
        partition: "deep-legends-share-export",
      },
    });
    exportWindow.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
    await exportWindow.loadURL(`${String(baseURL).replace(/\/$/, "")}/?share-export=1`);
    const dimensions = await exportWindow.webContents.executeJavaScript(exportDocumentScript(payload), true);
    const width = Math.ceil(Number(dimensions?.width));
    const height = Math.ceil(Number(dimensions?.height));
    const scale = screenshotScale(width, height);
    exportWindow.webContents.debugger.attach("1.3");
    debuggerAttached = true;
    await exportWindow.webContents.debugger.sendCommand("Page.enable");
    const screenshot = await exportWindow.webContents.debugger.sendCommand("Page.captureScreenshot", {
      format: "png",
      fromSurface: true,
      captureBeyondViewport: true,
      optimizeForSpeed: false,
      clip: { x: 0, y: 0, width, height, scale },
    });
    const png = Buffer.from(String(screenshot?.data || ""), "base64");
    if (png.length < 8 || png.subarray(1, 4).toString("ascii") !== "PNG") throw new Error("截图引擎未返回有效的 PNG 图片。");
    return { png, width, height, scale };
  } finally {
    if (debuggerAttached && exportWindow && !exportWindow.isDestroyed()) {
      try { exportWindow.webContents.debugger.detach(); } catch (_) {}
    }
    if (exportWindow && !exportWindow.isDestroyed()) exportWindow.destroy();
  }
}

function createShareExportController({ BrowserWindow, app, dialog, fileSystem, isTrustedRenderer, getBaseURL, log, now = Date.now, randomToken }) {
  const pending = new Map();
  const makeToken = randomToken || (() => crypto.randomBytes(24).toString("base64url"));
  const settingsPath = path.join(app.getPath("userData"), SHARE_SETTINGS_FILE);
  let saveDirectory = "";

  try {
    const stored = JSON.parse(fileSystem.readFileSync(settingsPath, "utf8"));
    saveDirectory = normalizeSaveDirectory(stored?.saveDirectory);
  } catch (_) {}

  function prune() {
    const current = now();
    for (const [senderID, entry] of pending) if (entry.expiresAt <= current) pending.delete(senderID);
  }

  function usableDirectory(directory) {
    try {
      return Boolean(directory && fileSystem.statSync(directory).isDirectory());
    } catch (_) {
      return false;
    }
  }

  function persistDirectory(directory) {
    const normalized = normalizeSaveDirectory(directory);
    if (!usableDirectory(normalized)) throw new Error("所选导出位置不可用。");
    fileSystem.mkdirSync(path.dirname(settingsPath), { recursive: true });
    fileSystem.writeFileSync(settingsPath, `${JSON.stringify({ saveDirectory: normalized }, null, 2)}\n`, "utf8");
    saveDirectory = normalized;
    return normalized;
  }

  async function requestDirectory(event, title) {
    const options = {
      title,
      buttonLabel: "选择文件夹",
      defaultPath: usableDirectory(saveDirectory) ? saveDirectory : app.getPath("pictures"),
      properties: ["openDirectory", "createDirectory"],
    };
    const owner = BrowserWindow.fromWebContents?.(event.sender) || null;
    const result = owner ? await dialog.showOpenDialog(owner, options) : await dialog.showOpenDialog(options);
    const directory = normalizeSaveDirectory(result?.filePaths?.[0]);
    if (result?.canceled || !directory) return { canceled: true };
    return { canceled: false, directory: persistDirectory(directory) };
  }

  function getSaveDirectory(event) {
    if (!isTrustedRenderer(event?.sender)) throw new Error("不受信任的页面不能读取分享图设置。");
    if (!usableDirectory(saveDirectory)) saveDirectory = "";
    return { directory: saveDirectory };
  }

  async function chooseSaveDirectory(event) {
    if (!isTrustedRenderer(event?.sender)) throw new Error("不受信任的页面不能修改分享图设置。");
    const result = await requestDirectory(event, "选择导出位置");
    if (!result.canceled) pending.delete(event.sender.id);
    return result;
  }

  async function prepareSave(event, suggestedName) {
    if (!isTrustedRenderer(event?.sender)) throw new Error("不受信任的页面不能保存分享图。");
    prune();
    let prompted = false;
    if (!usableDirectory(saveDirectory)) {
      saveDirectory = "";
      const selected = await requestDirectory(event, "首次生成分享图：选择保存位置");
      if (selected.canceled) return selected;
      prompted = true;
    }
    const token = makeToken();
    pending.set(event.sender.id, { token, filePath: uniquePngPath(fileSystem, saveDirectory, suggestedName), expiresAt: now() + SHARE_SAVE_TTL_MS });
    return { canceled: false, token, directory: saveDirectory, prompted };
  }

  async function captureAndSave(event, rawPayload) {
    if (!isTrustedRenderer(event?.sender)) throw new Error("不受信任的页面不能生成分享图。");
    prune();
    const payload = normalizeCapturePayload(rawPayload);
    const entry = pending.get(event.sender.id);
    if (!entry || entry.token !== payload.token) throw new Error("保存请求已失效，请重新选择位置。");
    pending.delete(event.sender.id);
    try {
      const result = await renderOverviewPng(BrowserWindow, getBaseURL(), payload);
      fileSystem.writeFileSync(entry.filePath, result.png);
      return {
        ok: true,
        fileName: path.basename(entry.filePath),
        pixelWidth: Math.round(result.width * result.scale),
        pixelHeight: Math.round(result.height * result.scale),
      };
    } catch (error) {
      log?.(`分享图导出失败：${error?.message || error}`);
      throw new Error("分享图生成失败，请重试。");
    }
  }

  function clear(senderID) {
    if (senderID === undefined) pending.clear();
    else pending.delete(senderID);
  }

  return { prepareSave, getSaveDirectory, chooseSaveDirectory, captureAndSave, clear };
}

module.exports = {
  SHARE_EXPORT_CONTENT_WIDTH,
  SHARE_EXPORT_SURFACE_WIDTH,
  SHARE_EXPORT_MAX_MARKUP,
  sanitizeSuggestedName,
  ensurePngPath,
  normalizeSaveDirectory,
  uniquePngPath,
  normalizeCapturePayload,
  screenshotScale,
  exportDocumentScript,
  renderOverviewPng,
  createShareExportController,
};
