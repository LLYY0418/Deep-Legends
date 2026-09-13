"use strict";

const path = require("node:path");

// Narrow download hook: no renderer-supplied paths, URLs or arbitrary writes.
function attachDiagnosticsExport({ session, sender, getBaseURL, getDirectory, fileSystem, now = () => new Date(), onCompleted = () => {}, getDesktopLog }) {
  const pending = new Set();
  const onDownload = (_event, item, contents) => {
    if (contents !== sender) return;
    let expected;
    try { expected = new URL("/api/diagnostics/log", getBaseURL()).href; } catch (_) { return; }
    const chain = item.getURLChain?.() || [item.getURL()];
    if (!chain.length || chain.some((url) => url !== expected)) return;
    let directory;
    try { directory = getDirectory(); } catch (_) { return; }
    if (!directory || !path.isAbsolute(directory)) return;
    try { if (!fileSystem.statSync(directory).isDirectory()) return; } catch (_) { return; }
    const date = now();
    const pad = (value) => String(value).padStart(2, "0");
    const stem = `lol-loot-diagnostics-${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(date.getHours())}${pad(date.getMinutes())}`;
    let destination;
    for (let index = 1; index <= 10000; index++) {
      const candidate = path.join(directory, `${stem}${index === 1 ? "" : `-${index}`}.jsonl`);
      if (!pending.has(candidate) && !fileSystem.existsSync(candidate)) { destination = candidate; break; }
    }
    if (!destination) return; // Fall back to Electron's normal Save dialog.
    pending.add(destination);
    item.once("done", (_event, state) => {
      pending.delete(destination);
      if (state === "completed") {
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
      }
    });
    try { item.setSavePath(destination); } catch (_) { pending.delete(destination); }
  };
  session.on("will-download", onDownload);
  return () => { session.removeListener("will-download", onDownload); pending.clear(); };
}

module.exports = { attachDiagnosticsExport };
