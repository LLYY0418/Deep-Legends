"use strict";

const path = require("node:path");

// Narrow download hook: no renderer-supplied paths, URLs or arbitrary writes.
function attachDiagnosticsExport({ session, sender, getBaseURL, getDirectory, fileSystem, now = () => new Date(), onCompleted = () => {} }) {
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
      if (state === "completed") onCompleted(destination);
    });
    try { item.setSavePath(destination); } catch (_) { pending.delete(destination); }
  };
  session.on("will-download", onDownload);
  return () => { session.removeListener("will-download", onDownload); pending.clear(); };
}

module.exports = { attachDiagnosticsExport };
