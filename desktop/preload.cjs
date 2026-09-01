"use strict";

const { contextBridge, ipcRenderer } = require("electron");

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
