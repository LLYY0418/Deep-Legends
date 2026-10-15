#!/usr/bin/env node
"use strict";
// R279 P2: change-aware LOCAL quick preflight.
//
//   node scripts/preflight.cjs [--base <ref>] [--list] [--json <file>] [--files a,b,c] [--browsers]
//
// It answers one question quickly: "did my change obviously break something it touches?".
// It is NOT release evidence. Formal verification is the CI run on a probe/release branch.
// Anything it cannot map is printed as 未覆盖, with the advice to push a probe branch.

const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const cp = require("node:child_process");
const { buildIndex, select, R277_STRICT } = require("./lib/preflight-select.cjs");
const root = path.resolve(__dirname, "..");

function git(args, options = {}) {
  return cp.execFileSync("git", args, { cwd: root, encoding: "utf8", maxBuffer: 64 * 1024 * 1024, ...options });
}
function tryGit(args) {
  try { return git(args, { stdio: ["ignore", "pipe", "ignore"] }).trim(); } catch (_) { return ""; }
}

function parseArgs(argv) {
  const options = { base: "", list: false, json: "", files: [], browsers: false, help: false };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--list") options.list = true;
    else if (arg === "--browsers") options.browsers = true;
    else if (arg === "--help" || arg === "-h") options.help = true;
    else if (arg === "--base") options.base = argv[++i] || "";
    else if (arg === "--json") options.json = argv[++i] || "";
    else if (arg === "--files") options.files = (argv[++i] || "").split(",").map((s) => s.trim()).filter(Boolean);
    else throw new Error(`未知参数：${arg}`);
  }
  return options;
}

// Base preference: explicit --base, then the branch's upstream (what is already pushed), then origin/main.
function resolveBase(explicit) {
  if (explicit) return explicit;
  const upstream = tryGit(["rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"]);
  if (upstream) return tryGit(["merge-base", "HEAD", upstream]) || upstream;
  const main = ["origin/main", "main"].find((ref) => tryGit(["rev-parse", "--verify", ref]));
  return main ? tryGit(["merge-base", "HEAD", main]) : "HEAD";
}

function changedFiles(base) {
  const changes = new Map();
  const status = git(["diff", "--name-status", "-M", base]).split("\n").filter(Boolean);
  for (const line of status) {
    const parts = line.split("\t");
    const code = parts[0][0];
    if (code === "R") { changes.set(parts[2], "R"); changes.set(parts[1], "D"); }
    else changes.set(parts[1], code);
  }
  for (const file of git(["ls-files", "--others", "--exclude-standard"]).split("\n").filter(Boolean)) changes.set(file, "A");
  return [...changes].map(([file, statusCode]) => ({ file, status: statusCode }));
}

function loadIndex() {
  const files = git(["ls-files", "-co", "--exclude-standard"]).split("\n").filter(Boolean);
  const { guards } = require("./r276-browser-catalog.cjs");
  return buildIndex({ files, read: (file) => fs.readFileSync(path.join(root, file), "utf8"), guards: guards.map((row) => row[0]) });
}

function run(label, command, args, options = {}) {
  const started = Date.now();
  console.log(`\n▶ ${label}\n  $ ${[command, ...args].join(" ").slice(0, 300)}`);
  const result = cp.spawnSync(command, args, { cwd: root, stdio: "inherit", env: { ...process.env, ...options.env } });
  const ms = Date.now() - started;
  const ok = result.status === 0;
  console.log(`${ok ? "✔" : "✘"} ${label}（${(ms / 1000).toFixed(1)}s）`);
  return { label, ok, ms, status: result.status };
}

function staticGates(selection, base) {
  const gates = [];
  const jsFiles = selection.changed.map((c) => c.file).filter((f) => /\.(?:cjs|js|mjs)$/.test(f) && fs.existsSync(path.join(root, f)));
  const goFiles = selection.changed.map((c) => c.file).filter((f) => /\.go$/.test(f) && fs.existsSync(path.join(root, f)));
  gates.push({label:"显式暂存隔离",command:process.execPath,args:["scripts/r279-staged-gates.cjs"]});
  gates.push({ label: "git diff --check", command: "git", args: ["diff", "--check", base] });
  gates.push({ label: "本地笔记与助手指令未被跟踪", command: process.execPath, args: ["scripts/verify-no-docs-tracked.cjs"] });
  for (const file of jsFiles) gates.push({ label: `语法检查 ${file}`, command: process.execPath, args: ["--check", file] });
  if (goFiles.length) gates.push({ label: "gofmt", command: process.execPath, args: ["scripts/r279-static-go.cjs",...goFiles] });
  if (selection.staticGates.includes("go-vet-backend")) gates.push({ label: "go vet ./backend", command: "go", args: ["vet", "./backend"] });
  const testsOrWorkflowsTouched = selection.changed.some((c) => /\.test\.[cm]js$|_test\.go$|^\.github\/workflows\//.test(c.file));
  if (testsOrWorkflowsTouched) gates.push({ label: "CI 测试过滤器校验", command: process.execPath, args: ["scripts/verify-ci-test-filters.cjs"] });
  if(selection.staticGates.includes("pinned-contracts"))gates.push({label:"pinned-contracts",command:process.execPath,args:["scripts/pinned-contracts-check.cjs"]});
  if(selection.webLayerTouched)gates.push({label:"首屏产物预算",command:process.execPath,args:["--test","--test-name-pattern=initial production assets","backend/web/r277-deferred-assets.test.cjs"]});
  return gates;
}

function printSummary(base, selection, total) {
  console.log(`基线：${base}`);
  for(const warning of selection.scopeWarnings||[])console.log("⚠ "+warning);
  if(selection.webLayerTouched)console.log("完整浏览器命令：node scripts/r276-browser-job.cjs main <out>；node scripts/r276-browser-job.cjs r265 <out>；r277-ci-orchestrate build-browser / browser <scratch backend>");
  console.log(`改动文件 ${selection.changed.length} 个；Node 测试 ${selection.nodeTests.length}/${total.node}（${Math.round(selection.ratio * 100)}%）；Go 测试文件 ${selection.goTestFiles.length}/${total.go}（${selection.goTestNames.length} 个用例）；浏览器守卫 ${selection.browserGuards.length} 个`);
  if (selection.nodeTests.length) {
    console.log("\n选中的 Node 测试：");
    for (const row of selection.nodeTests.slice(0, 60)) console.log(`  ${row.file}\n      ← ${row.reasons[0]}${row.reasons.length > 1 ? `（另有 ${row.reasons.length - 1} 条）` : ""}`);
    if (selection.nodeTests.length > 60) console.log(`  …另有 ${selection.nodeTests.length - 60} 个`);
  }
  if (selection.goTestFiles.length) {
    console.log("\n选中的 Go 测试文件：");
    for (const row of selection.goTestFiles) console.log(`  ${row.file}  ← ${row.reason}`);
  }
  if (selection.browserGuards.length) {
    console.log(`\n浏览器守卫：${selection.browserGuards.map((g) => g.name).join(", ")}`);
    console.log(`  原因：${selection.browserGuards[0].reason}`);
  }
  if (selection.relatedScripts.length) {
    console.log("\n相关脚本（在 CI 的 quality / proof 作业里运行，这里只提示，不自动执行）：");
    for (const row of selection.relatedScripts.slice(0, 25)) console.log(`  ${row.file}  ← ${row.because.join(", ")}`);
    if (selection.relatedScripts.length > 25) console.log(`  …另有 ${selection.relatedScripts.length - 25} 个`);
  }
  if (selection.ciOnly.length) {
    console.log("\n只由 CI 工作流运行、本地不重放的改动文件：");
    for (const row of selection.ciOnly) console.log(`  ${row.file}  ← ${row.workflows.join(", ")}`);
  }
  for (const note of selection.windowsOnly) console.log(`\n⚠ 只能在 CI 验证：${note}`);
  if (selection.unmapped.length) console.log(`\n⚠ 未覆盖（没有任何测试关联这些改动文件）：\n  ${selection.unmapped.join("\n  ")}`);
  if (selection.broad) console.log(`\n⚠ 选中比例 ${Math.round(selection.ratio * 100)}% 超过 60%，本地逐个跑意义不大。`);
  if (selection.recommendProbe) console.log("\n→ 建议：本地只跑静态门禁，然后直接推 probe 分支，用 CI 做完整验证。");
}

function runBrowsers(selection) {
  const out = fs.mkdtempSync(path.join(os.tmpdir(), "preflight-browsers-"));
  const results = [];
  const names = selection.browserGuards.map((g) => g.name).filter((n) => n !== R277_STRICT);
  if (names.length) {
    results.push(run(`浏览器守卫 ${names.join(",")}`, process.execPath, ["scripts/r276-browser-job.cjs", "all", path.join(out, "guards")], { env: { R276_BROWSER_ONLY: names.join(",") } }));
  }
  if (selection.browserGuards.some((g) => g.name === R277_STRICT)) {
    const backend = path.join(out, process.platform === "win32" ? "r277.exe" : "r277-browser-public");
    const target = path.join(out, "r277");
    results.push(run("R277 构建浏览器后端", process.execPath, ["scripts/r277-ci-orchestrate.cjs", "build-browser", "--backend", backend, "--out", target]));
    if (results.at(-1).ok) results.push(run("R277 严格几何矩阵", process.execPath, ["scripts/r277-ci-orchestrate.cjs", "browser", "--backend", backend, "--out", target]));
  }
  return results;
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    console.log("用法：node scripts/preflight.cjs [--base <ref>] [--list] [--json <file>] [--files a,b,c] [--browsers]\n  --list      只显示选择结果，不执行\n  --files     手动指定改动文件（逗号分隔），用于回放历史改动\n  --browsers  同时运行选中的真浏览器守卫（需要本机 Chrome 与 Go）");
    return 0;
  }
  const base = options.files.length ? "(手动指定文件)" : resolveBase(options.base);
  const changes = options.files.length ? options.files.map((file) => ({ file, status: "M" })) : changedFiles(base);
  const index = loadIndex();
  const selection = select(index, changes);
  const total = { node: index.nodeTests.length, go: index.goTests.length };
  if (!changes.length) { console.log("没有改动，无需预检。"); return 0; }
  printSummary(base, selection, total);

  const report = { base, generatedAt: new Date().toISOString(), totals: total, selection, results: [] };
  let exitCode = 0;
  if (!options.list) {
    const parallel = Math.max(1, Math.min(os.availableParallelism?.() ?? os.cpus().length, 6));
    for (const gate of staticGates(selection, base)) {
      const result = run(gate.label, gate.command, gate.args);
      report.results.push(result);
      if (!result.ok) exitCode = 1;
    }
    if(selection.broad)report.results.push({label:"关联测试范围过宽",status:"deferred-to-CI",note:"仅本地静态门禁，关联测试由完整CI执行；不是测试通过证据"});
    if (exitCode===0 && !selection.broad && selection.nodeTests.length) {
      const result = run(`Node 测试（${selection.nodeTests.length} 个文件，并发 ${parallel}）`, process.execPath, ["--test", `--test-concurrency=${parallel}`, ...selection.nodeTests.map((t) => t.file)]);
      report.results.push(result);
      if (!result.ok) exitCode = 1;
    }
    if (exitCode===0 && !selection.broad && selection.goTestNames.length) {
      const pattern = `^(${selection.goTestNames.join("|")})$`;
      const result = run(`Go 测试（${selection.goTestNames.length} 个用例，-race）`, "go", ["test", "-race", "-count=1", "-run", pattern, "./backend"]);
      report.results.push(result);
      if (!result.ok) exitCode = 1;
    }
    if (exitCode===0 && options.browsers && selection.browserGuards.length) {
      for (const result of runBrowsers(selection)) { report.results.push(result); if (!result.ok) exitCode = 1; }
    }
    console.log(`\n${exitCode === 0 ? (selection.broad?"静态预检通过，范围过宽的关联测试延后到CI":"快速预检通过") : "快速预检有失败"}。注意：这不是发版证据；完整验证以 probe / release 分支上的 CI 为准。`);
    if (selection.browserGuards.length && !options.browsers) console.log("浏览器守卫未在本地运行（加 --browsers 可运行）；它们在 CI 的 browser-main / browser-r265 / R277 matrix 里一定会跑。");
  }
  if (options.json) { fs.mkdirSync(path.dirname(path.resolve(options.json)), { recursive: true }); fs.writeFileSync(options.json, JSON.stringify(report, null, 2) + "\n");fs.writeFileSync(path.join(path.dirname(path.resolve(options.json)),"preflight-static-gates.json"),JSON.stringify({automaticStaging:false,gates:staticGates(selection,base)},null,2)+"\n"); }
  return exitCode;
}

if (require.main === module) {
  main().then((code) => { process.exitCode = code; }, (error) => { console.error(error.stack || error); process.exitCode = 2; });
}

module.exports = { parseArgs, resolveBase, changedFiles, staticGates };
