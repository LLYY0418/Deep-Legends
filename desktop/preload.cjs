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
