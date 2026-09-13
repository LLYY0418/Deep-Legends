"use strict";

const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("desktopScale", {
  get() { return ipcRenderer.invoke("desktop-scale-get"); },
  set(mode, value) { ipcRenderer.send("desktop-scale-set", mode, value); },
  // The renderer owns the actual zoom (CSS `zoom` on .app-frame) so the browser
  // preview scales too; the shell only mirrors the effective step so the native
  // Windows caption overlay and the minimum window size stay in step with it.
  applied(scale) { if (typeof scale === "number" && Number.isFinite(scale)) ipcRenderer.send("desktop-scale-applied", scale); },
  onChanged(callback) {
    if (typeof callback !== "function") return;
    const listener = (_event, payload) => callback(payload);
    ipcRenderer.on("desktop-scale-changed", listener);
    return () => ipcRenderer.removeListener("desktop-scale-changed", listener);
  },
});

contextBridge.exposeInMainWorld("desktopTheme", {
  setTheme(theme) {
    if (theme === "dark" || theme === "light") ipcRenderer.send("desktop-theme", theme);
  },
  setModalOpen(open) {
    ipcRenderer.send("desktop-modal", Boolean(open));
  },
  exitFullscreen() {
    ipcRenderer.send("desktop-fullscreen-exit");
  },
});

contextBridge.exposeInMainWorld("desktopShare", {
  preparePngSave(suggestedName) {
    const name = typeof suggestedName === "string" ? suggestedName.slice(0, 160) : "";
    return ipcRenderer.invoke("desktop-share-prepare-save", name);
  },
  getSaveDirectory() {
    return ipcRenderer.invoke("desktop-share-get-directory");
  },
  chooseSaveDirectory() {
    return ipcRenderer.invoke("desktop-share-choose-directory");
  },
  captureAndSavePng(payload) {
    const token = typeof payload?.token === "string" ? payload.token : "";
    const markup = typeof payload?.markup === "string" ? payload.markup : "";
    const theme = typeof payload?.theme === "string" ? payload.theme : "";
    const density = typeof payload?.density === "string" ? payload.density : "";
    if (!/^[A-Za-z0-9_-]{24,128}$/.test(token) || !markup.startsWith('<div class="overview-share-surface"') || markup.length > 8 * 1024 * 1024) {
      return Promise.reject(new Error("分享图内容无效。"));
    }
    return ipcRenderer.invoke("desktop-share-capture-and-save", { token, markup, theme, density });
  },
});


contextBridge.exposeInMainWorld("desktopDiagnostics", {
  onCompleted(callback) {
    if (typeof callback !== "function") return;
    ipcRenderer.on("desktop-diagnostics-completed", () => callback());
  },
  openFolder() { return ipcRenderer.invoke("desktop-diagnostics-open-folder"); },
});
