"use strict";

const path = require("node:path");

// Narrow download hook: no renderer-supplied paths, URLs or arbitrary writes.
function attachDiagnosticsExport({ session, sender, getBaseURL, getDirectory = () => "", getDefaultDirectory = () => "", fileSystem, now = () => new Date(), onCompleted = () => {}, getDesktopLog }) {
  const pending = new Set();
  const listeners = new Map();
  let active = true;
  const onDownload = (_event, item, contents) => {
    if (contents !== sender) return;
    let expected;
    try { expected = new URL("/api/diagnostics/log", getBaseURL()).href; } catch (_) { return; }
    const chain = item.getURLChain?.() || [item.getURL()];
    if (!chain.length || chain.some((url) => url !== expected)) return;
    let reserved;
    // Subscribe before selecting a destination, including Electron's Save dialog.
    const done = (_event, state) => {
      listeners.delete(item);
      pending.delete(reserved);
      if (!active || state !== "completed") return;
      let destination;
      try { destination = item.getSavePath(); } catch (_) { return; }
      if (!destination || !path.isAbsolute(destination)) return;
      if (getDesktopLog) {
        let fd;
        try {
          const extra = getDesktopLog();
          if (extra) {
            const stat = fileSystem.lstatSync(destination);
            if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 12 * 1024 * 1024) throw Error("untrusted export file");
            fd = fileSystem.openSync(destination, fileSystem.constants.O_WRONLY | fileSystem.constants.O_APPEND | (fileSystem.constants.O_NOFOLLOW || 0));
            const opened = fileSystem.fstatSync(fd);
            if (!opened.isFile() || opened.ino !== stat.ino || opened.dev !== stat.dev) throw Error("export file changed");
            fileSystem.appendFileSync(fd, "\n" + extra, "utf8");
          }
        } catch (_) {
          // Backend evidence remains usable if shell evidence is unavailable.
        } finally { if (fd !== undefined) fileSystem.closeSync(fd); }
      }
      onCompleted(destination);
    };
    listeners.set(item, done);
    item.once("done", done);
    const date = now();
    const pad = (value) => String(value).padStart(2, "0");
    const stem = `lol-loot-diagnostics-${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(date.getHours())}${pad(date.getMinutes())}`;
    // Configure Electron's native Save dialog without pausing or canceling the download.
    try {
      const directory = getDefaultDirectory();
      item.setSaveDialogOptions?.({ title: "导出诊断日志", defaultPath: directory && path.isAbsolute(directory) ? path.join(directory, `${stem}.jsonl`) : `${stem}.jsonl` });
    } catch (_) { /* The normal Save dialog remains available. */ }
    let directory;
    try {
      directory = getDirectory();
      if (!directory || !path.isAbsolute(directory) || !fileSystem.statSync(directory).isDirectory()) return;
    } catch (_) { return; }
    for (let index = 1; index <= 10000; index++) {
      const candidate = path.join(directory, `${stem}${index === 1 ? "" : `-${index}`}.jsonl`);
      if (!pending.has(candidate) && !fileSystem.existsSync(candidate)) { reserved = candidate; break; }
    }
    if (!reserved) return;
    pending.add(reserved);
    try { item.setSavePath(reserved); } catch (_) { pending.delete(reserved); }
  };
  session.on("will-download", onDownload);
  return () => {
    active = false;
    session.removeListener("will-download", onDownload);
    for (const [item, done] of listeners) item.removeListener("done", done);
    listeners.clear();
    pending.clear();
  };
}

module.exports = { attachDiagnosticsExport };
