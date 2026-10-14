"use strict";
// R279 P5: the facts-only triage summary. Every fixture below is a failure shape that CI really produced.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const { annotations, classify, parseLog, triage, markdown, parseArgs, relFile } = require("./ci-failure-triage.cjs");

test("categories: wait timeout, watchdog, assertion, crash and other are told apart", () => {
  assert.equal(classify("full app did not mount: no observable progress for 15000 ms; last progress {\"nodes\":3}"), "wait-timeout");
  assert.equal(classify("condition timeout: document.title"), "wait-timeout");
  assert.equal(classify("hard limit 120000 ms reached"), "watchdog");
  assert.equal(classify("File exceeded 90s: {\"file\":\"x\"}"), "watchdog");
  assert.equal(classify("AssertionError [ERR_ASSERTION]: Expected values to be strictly equal"), "assertion");
  assert.equal(classify("panic: runtime error: invalid memory address"), "crash");
  assert.equal(classify("Target closed"), "crash");
  assert.equal(classify("something unexpected happened"), "other");
  assert.equal(classify(""), "other");
});

test("renderer timing JSON: failing case, file and category; in-diff is computed against the change set", () => {
  const data = { success: false, failures: [{ file: "backend/web/r192.test.cjs", name: "R192 hydrate", message: "Error: full app did not mount" }, { file: "desktop/pro-players.test.cjs", name: "tabs", message: "AssertionError: 1 !== 2" }], files: [] };
  const report = triage({ job: "quality", base: "x", json: ["t.json"], changed: new Set(["desktop/pro-players.test.cjs"]) }, { json: () => data });
  assert.equal(report.failureCount, 2);
  const byFile = Object.fromEntries(report.failures.map((f) => [f.file, f]));
  assert.deepEqual([byFile["backend/web/r192.test.cjs"].category, byFile["backend/web/r192.test.cjs"].inDiff], ["wait-timeout", false]);
  assert.deepEqual([byFile["desktop/pro-players.test.cjs"].category, byFile["desktop/pro-players.test.cjs"].inDiff], ["assertion", true]);
  assert.equal(byFile["backend/web/r192.test.cjs"].evidence, "t.json");
});

test("unknown base: in-diff is reported as unknown (null), never guessed", () => {
  const data = { failures: [{ file: "scripts/a.test.cjs", name: "a", message: "x" }] };
  const report = triage({ job: "j", json: ["t.json"], changed: null }, { json: () => data });
  assert.equal(report.failures[0].inDiff, null);
  assert.match(markdown(report), /在本次改动内：未知/);
});

test("log shapes: renderer FAIL lines, TAP not ok blocks, Go --- FAIL and panics", () => {
  const log = [
    "FAIL R264 countdown: AssertionError: expected 2 秒",
    "    at file:///work/scripts/r264-browser.cjs:155:9",
    "--- FAIL: TestSomething (0.01s)",
    "    thing_test.go:12: boom",
    "not ok 3 - overview tab",
    "  ---",
    "  location: '/work/desktop/overview.test.cjs:44:1'",
    "  error: timeout waiting",
    "panic: runtime error: index out of range",
  ].join("\n");
  const rows = parseLog(log, "job.log");
  assert.deepEqual(rows.map((r) => r.kind), ["node-test", "go-test", "node-test", "go-test"]);
  assert.equal(rows[0].file, "scripts/r264-browser.cjs");
  assert.equal(rows[2].file, "desktop/overview.test.cjs");
  assert.equal(classify(rows[3].message + rows[3].name), "crash");
});

test("failure.json from a guard is summarised with its scenario and error", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "triage-"));
  try {
    fs.mkdirSync(path.join(dir, "strict"));
    fs.writeFileSync(path.join(dir, "strict", "failure.json"), JSON.stringify({ scenario: "p7-1280-1.75", error: "{\"width\":1280,\"zoom\":1.75} overlap assertion" }));
    fs.writeFileSync(path.join(dir, "step.log"), "all good\n");
    const report = triage({ job: "browser", evidence: [dir], changed: new Set() });
    assert.equal(report.failureCount, 1);
    assert.equal(report.failures[0].name, "p7-1280-1.75");
    assert.equal(report.failures[0].category, "assertion");
    assert.ok(report.failures[0].evidence.endsWith("failure.json"));
  } finally { fs.rmSync(dir, { recursive: true, force: true }); }
});

test("it makes no root-cause claim and never mentions CI-KNOWN-ISSUES; duplicates collapse", () => {
  const data = { failures: [{ file: "a.test.cjs", name: "n", message: "m" }, { file: "a.test.cjs", name: "n", message: "m" }] };
  const report = triage({ job: "j", json: ["t.json"], changed: new Set() }, { json: () => data });
  assert.equal(report.failureCount, 1);
  assert.doesNotMatch(JSON.stringify(report) + markdown(report), /根因是|已知问题条目|CI-KNOWN-ISSUES 匹配/);
  assert.match(report.note, /不判断根因/);
});

test("an unreadable or empty input produces an empty, harmless report", () => {
  const report = triage({ job: "j", json: ["missing.json"], log: ["missing.log"], changed: new Set() }, { json: () => null, text: () => { throw new Error("ENOENT"); } });
  assert.equal(report.failureCount, 0);
  assert.match(markdown(report), /没有在给定的日志/);
});

test("end to end: the CLI exits 0 even for a red job and writes triage.json", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "triage-cli-"));
  try {
    fs.writeFileSync(path.join(dir, "r.json"), JSON.stringify({ success: false, failures: [{ file: "scripts/x.test.cjs", name: "x", message: "AssertionError" }] }));
    const out = path.join(dir, "triage.json"), summary = path.join(dir, "summary.md");
    const result = require("node:child_process").spawnSync(process.execPath, [path.join(__dirname, "ci-failure-triage.cjs"), "--job", "t", "--json", path.join(dir, "r.json"), "--out", out, "--summary", summary], { encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(JSON.parse(fs.readFileSync(out, "utf8")).failures[0].category, "assertion");
    assert.match(fs.readFileSync(summary, "utf8"), /CI 失败分诊（t）/);
    const bad = require("node:child_process").spawnSync(process.execPath, [path.join(__dirname, "ci-failure-triage.cjs"), "--nope"], { encoding: "utf8" });
    assert.equal(bad.status, 0, "advisory tool must never fail a job");
  } finally { fs.rmSync(dir, { recursive: true, force: true }); }
});

test("argument parsing and path normalisation", () => {
  assert.deepEqual(parseArgs(["--job", "q", "--log", "a", "--log", "b", "--evidence", "d", "--base", "origin/main", "--out", "o", "--summary", "s", "--annotate"]), { job: "q", base: "origin/main", log: ["a", "b"], json: [], evidence: ["d"], out: "o", summary: "s", annotate: true });
  assert.equal(relFile("C:\\w\\desktop\\a.test.cjs"), "desktop/a.test.cjs");
  assert.equal(relFile("/runner/work/repo/scripts/x.cjs"), "scripts/x.cjs");
});

test("annotations: one ::error line per failure, escaped, capped at 10, no newlines inside", () => {
  const failures = Array.from({ length: 12 }, (_, i) => ({ kind: "node-test", name: `case ${i}: a,b`, file: "scripts/x.test.cjs", inDiff: i % 2 === 0, category: "assertion", message: `line1\nline2 100%` }));
  const lines = annotations({ failures });
  assert.equal(lines.length, 10);
  for (const line of lines) {
    assert.match(line, /^::error title=triage node-test%3A case \d+%3A a%2Cb::category=assertion; file=scripts\/x\.test\.cjs; in-diff=(yes|no); line1 line2 100%25$/);
    assert.ok(!/\n/.test(line));
  }
  assert.match(annotations({ failures: [{ kind: "go-test", name: "T", file: null, inDiff: null, category: "other", message: "m" }] })[0], /file=unknown; in-diff=unknown/);
});

test("log shapes: file budget lines and bare error lines are kept, structured failures sort first", () => {
  const log = ["Error: browser guard exploded", "File exceeded 90s: {\"file\":\"desktop/pro-players.test.cjs\",\"duration_ms\":120000}", "FAIL a case: AssertionError: x"].join("\n");
  const report = triage({ job: "j", log: ["l"], changed: new Set(["desktop/pro-players.test.cjs"]) }, { text: () => log });
  assert.deepEqual(report.failures.map((f) => f.kind), ["node-test", "node-test", "error-line"]);
  const budget = report.failures.find((f) => f.file === "desktop/pro-players.test.cjs");
  assert.equal(budget.category, "watchdog");
  assert.equal(budget.inDiff, true);
});

test("GitHub log decoration: ##[error] lines are read as error lines", () => {
  const rows = parseLog("##[error]AssertionError [ERR_ASSERTION]: boom\n##[error]Process completed with exit code 1.\n", "l");
  assert.deepEqual(rows.map((r) => r.name), ["AssertionError [ERR_ASSERTION]: boom", "Process completed with exit code 1."]);
});
