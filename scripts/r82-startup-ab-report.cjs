"use strict";
// Only reads diagnostics and small startup logs; never opens/hashes installed executables.
const fs = require("fs");
const path = require("path");
const METRICS = ["process_to_js", "spawn_to_ready", "total", "splash_window_shown"];
const median = values => [...values].sort((a, b) => a - b)[Math.floor(values.length / 2)];
const last = values => values[values.length - 1];

function validateIdentity(group, fingerprint) {
  if (!["control", "prewarm"].includes(group) || !/^[a-f0-9]{12}$/.test(fingerprint)) {
    throw new Error("Invalid group or fingerprint");
  }
}

function validateRecord(record) {
  validateIdentity(record.group, record.fingerprint);
  if (!record.run_id) throw new Error("Missing run_id");
  for (const key of METRICS) {
    if (!Number.isInteger(record.phases_ms?.[key]) || record.phases_ms[key] <= 0 || record.phases_ms[key] > 600000) {
      throw new Error(`Missing/invalid measured phase: ${key}`);
    }
  }
  for (const key of ["prewarm_ms", "handoff_ms"]) {
    if (!Number.isInteger(record[key]) || record[key] < 0) throw new Error(`Invalid ${key}`);
  }
  if (record.prewarm_timed_out !== false || record.prewarm_failures !== 0) throw new Error("Prewarm failed/timed out; preserve logs and use a new fingerprint for a replacement sample");
  // A little scheduler latency is normal; a substantial overrun must be investigated.
  if (record.prewarm_ms > 8500 || record.handoff_ms > 20500) throw new Error("Stage exceeded its 8s/20s budget (500ms scheduling tolerance)");
  if (record.group === "control" && record.prewarm_ms !== 0) throw new Error("Control unexpectedly spent time prewarming");
}

function summarize(records) {
  if (!Array.isArray(records)) throw new Error("Results JSON must be an array");
  records.forEach(validateRecord);
  if (new Set(records.map(r => r.fingerprint)).size !== records.length || new Set(records.map(r => r.run_id)).size !== records.length) {
    throw new Error("Repeated fingerprint/run_id: warm reruns are not fresh A/B samples");
  }
  const groups = Object.fromEntries(["control", "prewarm"].map(group => [group, records.filter(r => r.group === group)]));
  if (Object.values(groups).some(rows => rows.length > 3)) throw new Error("Use exactly 3 fresh samples per group");
  if (Object.values(groups).some(rows => rows.length < 3)) {
    return { status: "pending", counts: { control: groups.control.length, prewarm: groups.prewarm.length }, decision: "Keep prewarm disabled; six fresh Windows measurements are required." };
  }
  const medians = Object.fromEntries(Object.entries(groups).map(([group, rows]) => [group, {
    ...Object.fromEntries(METRICS.map(key => [key, median(rows.map(r => r.phases_ms[key]))])),
    prewarm_ms: median(rows.map(r => r.prewarm_ms)),
    // Includes prewarm cost; handoff ends at the first visible splash/main window.
    installed_to_visible_ms: median(rows.map(r => r.prewarm_ms + r.handoff_ms)),
  }]));
  const gain = medians.control.total - medians.prewarm.total;
  return { status: "measured", medians, total_gain_ms: gain,
    decision: gain < 300 ? "REMOVE prewarm: total improvement is below 300ms."
      : "300ms gate passed; review installed_to_visible_ms and read failures before deciding to enable prewarm.",
    prewarm_failures: records.filter(r => r.group === "prewarm").map(r => ({ fingerprint: r.fingerprint, failures: r.prewarm_failures, timed_out: r.prewarm_timed_out })),
  };
}

function parseLines(text) {
  return text.split(/\r?\n/).flatMap(line => { try { return [JSON.parse(line)]; } catch { return []; } });
}

// Shared with R83's real StartupLog coupling test. Keep accepting historical
// second-resolution logs; R83 writes microseconds after the same PID prefix.
function parseInstallerStartupLog(startupLog, pid) {
  if (!/^\d+$/.test(String(pid))) throw new Error("Invalid installer PID");
  const lines = startupLog.split(/\r?\n/).filter(line => line.startsWith(`[pid=${pid}] `)).join("\n");
  const prewarm = last([...lines.matchAll(/startup prewarm enabled=(true|false)(?: files=(\d+) failures=(\d+) timed_out=(true|false))? elapsed_ms=(\d+)/g)]);
  const handoff = last([...lines.matchAll(/application handoff visible=(true|false) elapsed_ms=(\d+)/g)]);
  const launches = [...lines.matchAll(/application launch pid=(\d+) error=([^\r\n]*?) elapsed_ms=(\d+)$/gm)]
    .map(match => ({ pid: Number(match[1]), error: match[2], elapsed_ms: Number(match[3]) }));
  return { prewarm, handoff, launches };
}

function collectRecord({ records, diagnostics, startupLog, group, fingerprint, pid, since }) {
  validateIdentity(group, fingerprint);
  if (!Array.isArray(records)) throw new Error("Results JSON must be an array");
  if (records.some(r => r.fingerprint === fingerprint)) throw new Error("Fingerprint already measured; use a genuinely new build");
  const starts = diagnostics.filter(r => r.event === "app_start" && r.build_fingerprint === fingerprint);
  if (starts.some(r => !Number.isFinite(Date.parse(r.time)))) throw new Error("Invalid app_start timestamp");
  if (starts.some(r => Date.parse(r.time) < since)) throw new Error("This fingerprint was already launched before the test; reject warm sample");
  const runIDs = new Set(starts.filter(r => Date.parse(r.time) >= since).map(r => r.run_id));
  const candidates = new Map();
  for (const event of diagnostics) {
    if (event.event === "desktop_startup_phases_ms" && event.run_id && runIDs.has(event.run_id)) {
      if (!Number.isFinite(Date.parse(event.time))) throw new Error("Invalid startup phase timestamp");
      if (Date.parse(event.time) >= since) candidates.set(`${event.run_id}:${event.log_seq}`, event);
    }
  }
  if (candidates.size > 1) throw new Error("Multiple startup runs found; do not reopen the application during collection");
  const event = [...candidates.values()][0];
  const { prewarm, handoff } = parseInstallerStartupLog(startupLog, pid);
  if (!event || !prewarm || !handoff) return null;
  if ((prewarm[1] === "true") !== (group === "prewarm")) throw new Error("Installer prewarm mode does not match the requested A/B group");
  if (group === "prewarm" && prewarm[2] !== "2") throw new Error("Prewarm did not finish both executable reads");
  if (handoff[1] !== "true") throw new Error("Application window handoff timed out; record as a failure, not a latency sample");
  const record = { group, fingerprint, run_id: event.run_id, captured_at: event.time, phases_ms: event.phases_ms,
    prewarm_ms: Number(prewarm[5]), handoff_ms: Number(handoff[2]), prewarm_failures: Number(prewarm[3] || 0), prewarm_timed_out: prewarm[4] === "true" };
  validateRecord(record);
  return record;
}

function readDiagnostics(file) {
  return [4, 3, 2, 1, 0].flatMap(n => {
    const candidate = n ? path.join(path.dirname(file), `diagnostics.${n}.jsonl`) : file;
    return fs.existsSync(candidate) ? parseLines(fs.readFileSync(candidate, "utf8")) : [];
  });
}

async function main(args) {
  if (args[0] !== "--collect") {
    if (args.length !== 1) throw new Error("Usage: node scripts/r82-startup-ab-report.cjs <results.json>");
    console.log(JSON.stringify(summarize(JSON.parse(fs.readFileSync(args[0], "utf8"))), null, 2));
    return;
  }
  const [, group, fingerprint, pid, sinceText, diagnosticsPath, startupPath, resultsPath] = args;
  validateIdentity(group, fingerprint);
  const since = Date.parse(sinceText);
  if (args.length !== 8 || !Number.isFinite(since) || !/^\d+$/.test(pid)) throw new Error("Invalid collector arguments");
  const records = fs.existsSync(resultsPath) ? JSON.parse(fs.readFileSync(resultsPath, "utf8")) : [];
  summarize(records);
  for (let attempt = 0; attempt < 60; attempt++) {
    const record = collectRecord({ records, diagnostics: readDiagnostics(diagnosticsPath),
      startupLog: fs.existsSync(startupPath) ? fs.readFileSync(startupPath, "utf8") : "", group, fingerprint, pid, since });
    if (record) {
      const combined = [...records, record];
      const report = summarize(combined); // Validate before changing the saved results.
      fs.mkdirSync(path.dirname(path.resolve(resultsPath)), { recursive: true });
      fs.writeFileSync(resultsPath, JSON.stringify(combined, null, 2) + "\n");
      console.log(JSON.stringify({ added: record, report }, null, 2));
      return;
    }
    await new Promise(resolve => setTimeout(resolve, 1000));
  }
  throw new Error("No matching startup/handoff logs within 60s. Check log paths and preserve the failed run for investigation.");
}

module.exports = { summarize, collectRecord, parseLines, readDiagnostics, parseInstallerStartupLog };
if (require.main === module) main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exitCode = 1; });
