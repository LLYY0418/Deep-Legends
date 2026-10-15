#!/usr/bin/env node
"use strict";
// R279 P5: facts-only CI failure summary.
//
//   node scripts/ci-failure-triage.cjs --job <name> [--base <ref>] [--log f]... [--json f]... [--evidence dir]... [--out triage.json] [--summary file]
//
// For every failure it can find in the given logs / renderer timing JSON / failure.json files it records
//   * which test file (or Go test / guard) failed,
//   * whether that file is inside this change's diff (relative to --base; null when the base is unknown),
//   * the error category: wait-timeout | watchdog | assertion | crash | other,
//   * where the evidence is.
// It draws NO root-cause conclusion and does not consult CI-KNOWN-ISSUES. It never changes the job result.

const fs = require("node:fs"), path = require("node:path"), cp = require("node:child_process");
const root = path.resolve(__dirname, "..");

const CATEGORIES = [
  ["crash", /panic:|SIGSEGV|SIGKILL|SIGABRT|fatal error:|out of memory|Target closed|Session closed|browser (?:has )?disconnected|Chrome (?:exited|crashed)|ECONNRESET|socket hang up|unexpected exit/i],
  ["watchdog", /WAIT_HARD_LIMIT|hard limit|watchdog|File exceeded|exceeded .*budget|budget exceeded|suite exceeded/i],
  ["wait-timeout", /WAIT_STALLED|no observable progress|timed? ?out|timeout|did not mount|did not finish|did not (?:start|load|reach|appear)|condition timeout|deadline exceeded/i],
  ["assertion", /AssertionError|ERR_ASSERTION|\bassert(?:ion|s)?\b|\bexpected\b|\bactual\b|Expected values|not (?:deep)?equal/i],
];

function classify(message) {
  const text = String(message || "");
  for (const [name, pattern] of CATEGORIES) if (pattern.test(text)) return name;
  return "other";
}

function relFile(file) {
  if (!file) return "";
  const normalised = String(file).replaceAll("\\", "/");
  const marker = ["/backend/", "/desktop/", "/scripts/"].map((m) => normalised.lastIndexOf(m)).filter((i) => i >= 0);
  const cut = marker.length ? Math.max(...marker) + 1 : -1;
  return cut >= 0 ? normalised.slice(cut) : normalised.replace(/^\.\//, "");
}

// Parsers return [{kind, name, file, message, evidence}]
function parseRendererJson(data, evidence) {
  const rows = [];
  for (const failure of data.failures || []) rows.push({ kind: "node-test", name: failure.name, file: relFile(failure.file), message: failure.message, evidence });
  for (const file of data.files || []) if (file.duration_ms && data.file_budget_ms && file.duration_ms > data.file_budget_ms) rows.push({ kind: "node-test", name: file.file, file: relFile(file.file), message: `File exceeded ${data.file_budget_ms / 1000}s budget (${Math.round(file.duration_ms)} ms)`, evidence });
  if (data.success === false && !rows.length) rows.push({ kind: "node-test", name: "(suite)", file: "", message: "renderer suite reported success=false without a recorded failing case", evidence });
  return rows;
}

function parseFailureJson(data, evidence) {
  if(Array.isArray(data.failures)&&data.failures.length)return data.failures.flatMap(f=>parseFailureJson({...f,failures:undefined,scenario:f.name||f.scenario},evidence));
  const message = data.error || data.message || data.reason || "";
  if (!message && !data.scenario && !data.case) return [];
  const located=/(?:\(|\s)((?:[\w./\\:-]*?)(?:backend|desktop|scripts)[\w./\\-]*\.(?:cjs|js)):\d+/.exec(data.stack||message);
  return [{ kind: "guard", name: data.scenario || data.case || data.name || path.basename(path.dirname(evidence)), file: relFile(data.file || data.script || located?.[1] || ""), message: String(data.stack || message).slice(0, 600), evidence }];
}

function parseLog(text, evidence) {
  const rows = [];
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    let m;
    if ((m = /^FAIL (.+?): (.*)$/.exec(line))) {
      const detail = [m[2], ...lines.slice(i + 1, i + 6)].join("\n");
      const located = /(?:\(|\s|')((?:[\w./\\:-]*?)(?:backend|desktop|scripts)[\w./\\-]*\.(?:cjs|js)):\d+/.exec(detail);
      rows.push({ kind: "node-test", name: m[1], file: relFile(located ? located[1] : ""), message: detail.slice(0, 600), evidence });
    } else if ((m = /^\s*--- FAIL: (\w+)/.exec(line))) {
      rows.push({ kind: "go-test", name: m[1], file: "", message: lines.slice(i + 1, i + 6).join("\n").slice(0, 600), evidence });
    } else if ((m = /^not ok \d+ - (.+)$/.exec(line))) {
      const detail = lines.slice(i + 1, i + 14).join("\n");
      const located = /location: '([^']+?):\d+:\d+'/.exec(detail);
      rows.push({ kind: "node-test", name: m[1], file: relFile(located ? located[1] : ""), message: detail.slice(0, 600), evidence });
    } else if ((m = /^File exceeded (\d+)s: (.*)$/.exec(line))) {
      const file = /"file":"([^"]+)"/.exec(m[2]);
      rows.push({ kind: "node-test", name: file ? file[1] : "(file budget)", file: relFile(file ? file[1] : ""), message: line.slice(0, 600), evidence });
    } else if ((m = /^(?:##\[error\])?(AssertionError[^\n]{0,160}|(?:Type|Range|Reference)?Error: [^\n]{0,200}|Process completed with exit code \d+\.?)$/.exec(line))) {
      rows.push({ kind: "error-line", name: m[1].slice(0, 100), file: "", message: m[1], evidence });
    } else if (/^panic: /.test(line)) {
      rows.push({ kind: "go-test", name: "(panic)", file: "", message: lines.slice(i, i + 4).join("\n").slice(0, 600), evidence });
    }
  }
  return rows;
}

function readJson(file) { try { return JSON.parse(fs.readFileSync(file, "utf8")); } catch (_) { return null; } }

function collect(options, readers = {}) {
  const rows = [];
  const evidenceFiles = [],timingInputs=[];
  const walk = (dir) => {
    const out = [];
    if (!fs.existsSync(dir)) return out;
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) out.push(...walk(full));
      else out.push(full);
    }
    return out;
  };
  const jsonFiles = [...(options.json || [])];
  const logFiles = [...(options.log || [])];
  for (const dir of options.evidence || []) {
    for (const file of walk(dir)) {
      if (/failure\.json$/.test(file)) jsonFiles.push(file);
      else if (/\.log$/.test(file)) logFiles.push(file);
    }
  }
  for (const file of jsonFiles) {
    const data = (readers.json || readJson)(file);
    if (!data) continue;
    if(data.timingComparison)timingInputs.push(data);
    const parsed = path.basename(file) === "failure.json" ? parseFailureJson(data, file) : parseRendererJson(data, file);
    if (parsed.length) { rows.push(...parsed); evidenceFiles.push(file); }
  }
  for (const file of logFiles) {
    let text = "";
    try { text = (readers.text || ((f) => fs.readFileSync(f, "utf8")))(file); } catch (_) { continue; }
    timingInputs.push(...require("./lib/runner-diagnostics.cjs").timingReportsFromLog(text));
    const parsed = parseLog(text, file);
    if (parsed.length) { rows.push(...parsed); evidenceFiles.push(file); }
  }
  return { rows, evidenceFiles, timingInputs };
}

function resolveBase(base,exec=cp.execFileSync) {
  if(base!=='release-base')return base||null;
  try{return exec('git',['describe','--tags','--match','v*','--abbrev=0'],{cwd:root,encoding:'utf8',stdio:['ignore','pipe','ignore']}).trim()||null;}catch(_){return null;}
}

function changedSet(base) {
  if (!base) return null;
  try {
    const out = cp.execFileSync("git", ["diff", "--name-only", base], { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
    return new Set(out.split("\n").filter(Boolean));
  } catch (_) { return null; }
}

function triage(options, readers = {}) {
  const { rows, evidenceFiles, timingInputs } = collect(options, readers);
  const base=resolveBase(options.base,readers.exec);
  const changed = options.changed !== undefined ? options.changed : changedSet(base);
  const seen = new Set();
  const failures = [];
  // Structured rows (test cases, Go tests, guards) first; raw error lines only fill what is left.
  rows.sort((x, y) => (x.kind === "error-line") - (y.kind === "error-line"));
  for (const row of rows) {
    const key = `${row.kind}|${row.file}|${row.name}`;
    if (seen.has(key)) continue;
    seen.add(key);
    failures.push({
      kind: row.kind, name: row.name, file: row.file || null,
      inDiff: row.file && changed ? changed.has(row.file) : null,
      category: classify(row.message), message: String(row.message || "").slice(0, 400), evidence: row.evidence,
    });
  }
  return { runner: timingInputs[0]?.runner||null, timingComparisons:timingInputs.map(d=>d.timingComparison), job: options.job || "", base, failureCount: failures.length, failures, evidenceFiles, note: "仅事实汇总：不判断根因，不引用 CI-KNOWN-ISSUES，不改变作业结果。" };
}

function markdown(report) {
  const lines = [`### CI 失败分诊（${report.job || "job"}）`, "", `- 失败项：${report.failureCount}`, `- 对比基线：${report.base || "未提供"}`];
  for(const c of report.timingComparisons||[])lines.push(`- 耗时分布：${c.kind==='uniform-slowdown'?'均匀变慢':c.kind}；中位比${c.medianRatio}；异常文件${c.outliers?.map(f=>f.file).join(',')||'无'}（不推断原因）`);
  if(report.runner)lines.push(`- Runner：${JSON.stringify(report.runner)}`);
  if (!report.failureCount) lines.push("- 没有在给定的日志/证据里找到可解析的失败项（请看原始日志）。");
  for (const f of report.failures.slice(0, 40)) {
    const diff = f.inDiff === null ? "未知" : f.inDiff ? "是" : "否";
    lines.push(`- **${f.name}**  类别：${f.category}；文件：${f.file || "未知"}；在本次改动内：${diff}；证据：\`${f.evidence}\``);
  }
  if (report.failures.length > 40) lines.push(`- …另有 ${report.failures.length - 40} 项，见 triage.json`);
  lines.push("", report.note, "");
  return lines.join("\n");
}

// GitHub workflow-command lines (visible through the check-run annotations API, 10 per step at most).
function escapeData(value) { return String(value).replace(/%/g, "%25").replace(/\r/g, "%0D").replace(/\n/g, "%0A"); }
function escapeProperty(value) { return escapeData(value).replace(/:/g, "%3A").replace(/,/g, "%2C"); }
function annotations(report, limit = 10) {
  return report.failures.slice(0, limit).map((f) => {
    const diff = f.inDiff === null ? "unknown" : f.inDiff ? "yes" : "no";
    const detail = `category=${f.category}; file=${f.file || "unknown"}; in-diff=${diff}; ${String(f.message).replace(/\s+/g, " ").slice(0, 300)}`;
    return `::error title=${escapeProperty(`triage ${f.kind}: ${f.name}`.slice(0, 120))}::${escapeData(detail)}`;
  });
}

function parseArgs(argv) {
  const options = { job: "", base: "", log: [], json: [], evidence: [], out: "", summary: process.env.GITHUB_STEP_SUMMARY || "", annotate: false };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    const take = () => argv[++i] || "";
    if (arg === "--job") options.job = take();
    else if (arg === "--base") options.base = take();
    else if (arg === "--log") options.log.push(take());
    else if (arg === "--json") options.json.push(take());
    else if (arg === "--evidence") options.evidence.push(take());
    else if (arg === "--out") options.out = take();
    else if (arg === "--summary") options.summary = take();
    else if (arg === "--annotate") options.annotate = true;
    else throw new Error(`unknown option: ${arg}`);
  }
  return options;
}

if (require.main === module) {
  try {
    const options = parseArgs(process.argv.slice(2));
    const report = triage(options);
    if (options.out) { fs.mkdirSync(path.dirname(path.resolve(options.out)), { recursive: true }); fs.writeFileSync(options.out, JSON.stringify(report, null, 2) + "\n"); }
    const text = markdown(report);
    if (options.summary) fs.appendFileSync(options.summary, text);
    else process.stdout.write(text);
    if (options.annotate) for (const line of annotations(report)) console.log(line);
  } catch (error) {
    // Triage is advisory: it must never turn a red job into a different red or hide the original failure.
    console.error(`ci-failure-triage: ${error.message}`);
  }
  process.exitCode = 0;
}

module.exports = { resolveBase, annotations, classify, parseLog, parseRendererJson, parseFailureJson, triage, markdown, parseArgs, relFile };
