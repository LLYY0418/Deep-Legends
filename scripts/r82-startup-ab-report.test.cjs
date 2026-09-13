"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path");
const { spawnSync } = require("node:child_process");
const { summarize, collectRecord, readDiagnostics } = require("./r82-startup-ab-report.cjs");

// Synthetic fixtures verify the collector/math only. These are not Windows measurements.
function sample(i, total = 3600) {
  return { group: i < 3 ? "control" : "prewarm", fingerprint: i.toString(16).padStart(12, "0"), run_id: `run-${i}`,
    phases_ms: { process_to_js: 1600, spawn_to_ready: 1500, total, splash_window_shown: 1800 },
    prewarm_ms: i < 3 ? 0 : 700, handoff_ms: 2100, prewarm_failures: 0, prewarm_timed_out: false };
}
test("R82 synthetic A/B medians enforce the 300ms gate and include prewarm cost", () => {
  const rows = [3900, 3500, 3600, 3300, 3100, 4000].map((total, i) => sample(i, total));
  const report = summarize(rows);
  assert.equal(report.medians.control.total, 3600);
  assert.equal(report.medians.prewarm.total, 3300);
  assert.equal(report.total_gain_ms, 300);
  assert.match(report.decision, /gate passed/);
  assert.equal(report.medians.prewarm.installed_to_visible_ms, 2800);
  rows[3].phases_ms.total = 3301;
  assert.match(summarize(rows).decision, /^REMOVE/);
});
test("R82 incomplete, repeated and malformed samples cannot pass acceptance", () => {
  assert.equal(summarize([]).status, "pending");
  assert.equal(summarize([sample(0)]).status, "pending");
  assert.throws(() => summarize([sample(0), sample(0)]), /Repeated/);
  assert.throws(() => summarize([{ ...sample(0), phases_ms: {} }]), /phase/);
  assert.throws(() => summarize([{ ...sample(0), prewarm_ms: -1 }]), /prewarm_ms/);
  assert.throws(() => summarize({}), /array/);
  assert.throws(() => summarize([{ ...sample(3), prewarm_ms: 9000 }]), /budget/);
  assert.throws(() => summarize([{ ...sample(3), handoff_ms: 21000 }]), /budget/);
  assert.throws(() => summarize([{ ...sample(3), prewarm_timed_out: true }]), /timed out/);
  assert.throws(() => summarize([{ ...sample(3), prewarm_failures: 1 }]), /failed/);
});

function captureFixture() {
  const time = "2026-09-12T04:00:00.000Z", row = sample(3);
  return { records: [], group: "prewarm", fingerprint: row.fingerprint, pid: "321", since: Date.parse(time),
    diagnostics: [
      { event: "app_start", build_fingerprint: row.fingerprint, run_id: row.run_id, time },
      { event: "desktop_startup_phases_ms", run_id: row.run_id, log_seq: 2, time, phases_ms: row.phases_ms },
      { event: "desktop_startup_phases_ms", run_id: "unrelated", time, phases_ms: { total: 1 } },
    ],
    startupLog: "[pid=999] startup prewarm enabled=false elapsed_ms=0\n[pid=321] startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=700\n[pid=321] application handoff visible=true elapsed_ms=2100" };
}
test("R82 collector joins phases to fingerprint by run_id and installer log by PID", () => {
  const fixture = captureFixture(), result = collectRecord(fixture);
  assert.equal(result.fingerprint, sample(3).fingerprint);
  assert.equal(result.run_id, sample(3).run_id);
  assert.equal(result.prewarm_ms, 700);
  assert.equal(result.handoff_ms, 2100);
  assert.equal(collectRecord({ ...fixture, pid: "111" }), null);
  assert.equal(collectRecord({ ...fixture, fingerprint: "111111111111" }), null);
  assert.throws(() => collectRecord({ ...fixture, group: "control" }), /mode/);
  assert.throws(() => collectRecord({ ...fixture, startupLog: fixture.startupLog.replace("visible=true", "visible=false") }), /timed out/);
  assert.throws(() => collectRecord({ ...fixture, records: [result] }), /already measured/);
  fixture.diagnostics.push({ ...fixture.diagnostics[0], time: "2026-09-11T00:00:00Z", run_id: "old" });
  assert.throws(() => collectRecord(fixture), /already launched/);
});
test("R82 collector rejects a second launch but deduplicates log rotation copies", () => {
  const fixture = captureFixture(); fixture.diagnostics.push(fixture.diagnostics[1]);
  assert.ok(collectRecord(fixture));
  fixture.diagnostics.push({ ...fixture.diagnostics[1], log_seq: 3 });
  assert.throws(() => collectRecord(fixture), /Multiple startup/);
});
test("R82 malformed inputs and failed prewarm cannot silently enter the A/B comparison", () => {
  const fixture = captureFixture();
  assert.throws(() => collectRecord({ ...fixture, group: "typo" }), /group/);
  assert.throws(() => collectRecord({ ...fixture, records: {} }), /array/);
  assert.throws(() => collectRecord({ ...fixture, startupLog: fixture.startupLog.replace("files=2", "files=1") }), /both/);
  assert.throws(() => collectRecord({ ...fixture, startupLog: fixture.startupLog.replace("failures=0", "failures=1") }), /failed/);
  fixture.diagnostics[1].time = "bad timestamp";
  assert.throws(() => collectRecord(fixture), /timestamp/);
  fixture.diagnostics[0].time = "bad timestamp";
  assert.throws(() => collectRecord(fixture), /app_start timestamp/);
});
test("R82 reads the actual diagnostics.N.jsonl archive naming", t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "r82-log-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.writeFileSync(path.join(root, "diagnostics.1.jsonl"), '{"event":"app_start"}\n');
  fs.writeFileSync(path.join(root, "diagnostics.jsonl"), 'partial\n{"event":"desktop_startup_phases_ms"}\n');
  assert.deepEqual(readDiagnostics(path.join(root, "diagnostics.jsonl")).map(r => r.event), ["app_start", "desktop_startup_phases_ms"]);
});

function collectCLI(t, file, withoutAt) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "r84-collector-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const fixture = captureFixture();
  const diagnostics = path.join(root, "diagnostics.jsonl"), startup = path.join(root, "startup.log"), results = path.join(root, "results.json");
  fs.writeFileSync(diagnostics, fixture.diagnostics.map(JSON.stringify).join("\n"));
  fs.writeFileSync(startup, [
    "[pid=321] startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=100",
    "[pid=321] application handoff visible=true elapsed_ms=1000",
    fixture.startupLog,
    "[pid=999] startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=800",
    "[pid=999] application handoff visible=true elapsed_ms=9999",
  ].join("\n"));
  const preload = path.join(root, "without-at.cjs");
  fs.writeFileSync(preload, "delete Array.prototype.at;\n");
  const args = [...(withoutAt ? ["--require", preload] : []), file, "--collect", fixture.group, fixture.fingerprint,
    fixture.pid, new Date(fixture.since).toISOString(), diagnostics, startup, results];
  const run = spawnSync(process.execPath, args, { encoding: "utf8", timeout: 5000 });
  assert.ifError(run.error);
  assert.equal(run.status, 0, run.stderr);
  const saved = JSON.parse(fs.readFileSync(results, "utf8"));
  assert.equal(saved.length, 1);
  assert.equal(saved[0].prewarm_ms, 700, "must use the last prewarm for this PID");
  assert.equal(saved[0].handoff_ms, 2100, "must use the last handoff for this PID");
  assert.deepEqual(JSON.parse(run.stdout).added, saved[0]);
  return saved;
}

test("R84 collector CLI works without Array.prototype.at and keeps the last matching PID records", t => {
  const file = path.join(__dirname, "r82-startup-ab-report.cjs");
  assert.deepEqual(collectCLI(t, file, true), collectCLI(t, file, false));
});

test("R84 collector production source cannot reintroduce .at calls", () => {
  assert.doesNotMatch(fs.readFileSync(path.join(__dirname, "r82-startup-ab-report.cjs"), "utf8"), /\.at\s*\(/);
});

test("R84 first-match mutation fails the end-to-end collector assertions", t => {
  const original = fs.readFileSync(path.join(__dirname, "r82-startup-ab-report.cjs"), "utf8");
  const before = "values[values.length - 1]";
  assert.equal(original.split(before).length, 2);
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "r84-first-match-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const file = path.join(root, "collector.cjs");
  fs.writeFileSync(file, original.replace(before, "values[0]"));
  assert.throws(() => collectCLI(t, file, true), /must use the last prewarm/);
});
