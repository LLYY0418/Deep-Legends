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
let preflightGocache=process.env.GOCACHE,ownedGocache=false;
const {planExecution}=require("./lib/preflight-cost.cjs");

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
    else if (arg === "--full") options.full = true;
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

function changedFiles(base, cwd = root) {
  const changes = new Map();
  const tokens = git(["diff", "--name-status", "-M", "-z", base, "--"], {cwd}).split("\0").filter(Boolean);
  for (let i = 0; i < tokens.length;) {
    const code = tokens[i++][0], file = tokens[i++];
    if (code === "R") { changes.set(tokens[i++], "R"); changes.set(file, "D"); }
    else changes.set(file, code);
  }
  for (const file of git(["ls-files", "--others", "--exclude-standard", "-z"], {cwd}).split("\0").filter(Boolean)) changes.set(file, "A");
  return [...changes].map(([file, statusCode]) => ({ file, status: statusCode }));
}

function loadIndex(base = "HEAD", cwd = root, changes = changedFiles(base, cwd)) {
  const files = git(["ls-files", "-co", "--exclude-standard", "-z"], {cwd}).split("\0").filter(file => file && fs.existsSync(path.join(cwd, file)));
  const historicalFiles = changes.filter(change => change.status === "D").map(change => change.file);
  const readHistorical = file => git(["show", `${base}:${file}`], {cwd, stdio:["ignore","pipe","pipe"]});
  const { guards } = require("./r276-browser-catalog.cjs");
  return buildIndex({ files, historicalFiles, readHistorical, read: file => fs.readFileSync(path.join(cwd, file), "utf8"), guards: guards.map(row => row[0]) });
}

function behaviorChecks(base,changes,cwd=root) {
  const source=require('./lib/source-inventory.cjs').inventory(cwd);
  const deltas=changes.map(row=>{
    const diff=git(['diff','--no-ext-diff','--unified=0',base,'--',row.file],{cwd});
    const delta=diff.split('\n').filter(line=>/^[+-]/.test(line)&&!/^---|^\+\+\+/.test(line)).join('\n');
    const conservative=!delta&&source.files.includes(row.file);
    return {...row,delta:conservative?source.read(row.file):delta,basis:conservative?'whole-file-conservative':'changed-lines'};
  });
  return require('./lib/behavior-checks.cjs').analyzeChanges(source,deltas);
}

function run(label, command, args, options = {}) {
  const started = Date.now();
  console.log(`\n▶ ${label}\n  $ ${[command, ...args].join(" ").slice(0, 300)}`);
  const result = cp.spawnSync(command, args, { cwd: root, stdio: "inherit", env: { ...process.env, ...(preflightGocache?{GOCACHE:preflightGocache}:{}), ...options.env } });
  const ms = Date.now() - started;
  const ok = result.status === 0;
  console.log(`${ok ? "✔" : "✘"} ${label}（${(ms / 1000).toFixed(1)}s）`);
  return { label, ok, ms, status: result.status };
}

// Direct script tests never disappear into the associated-cost deferral.
function runScriptTests(tests, {platform = process.platform, runCommand = run} = {}) {
  return tests.map(test => {
    const command = test.runner === "python" ? (platform === "win32" ? "python" : "python3") : "pwsh";
    const args = test.runner === "python" ? [test.file] : ["-NoProfile", "-File", test.file];
    if (test.runner === "powershell" && platform !== "win32") {
      const mapped = test.workflows.length > 0;
      return {label: "直接测试 " + test.file, file: test.file, command, args, workflows: test.workflows,
        status: mapped ? "deferred-to-Windows-CI" : "blocked-unmapped-platform",
        ok: mapped ? null : false,
        note: mapped ? "本机不运行Windows夹具；等待列出的CI入口，不算通过" : "缺少Windows验证入口，必须补齐后才能声称直接测试覆盖"};
    }
    return {...runCommand("直接测试 " + test.file, command, args), file: test.file};
  });
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
  gates.push({label:"go vet ./...",command:"go",args:["vet","./..."]});
  gates.push({label:"Windows amd64 交叉编译",command:process.execPath,args:["scripts/preflight-build.cjs"]});
  gates.push({label:"发布清单",command:process.execPath,args:["scripts/make-release.cjs","--check-notes"]});
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
  if (selection.scriptTests?.length) {
    console.log("\n直接改动的Python/PowerShell测试：");
    for (const row of selection.scriptTests) console.log("  " + row.file + " ← " + row.runner + (row.workflows.length ? "；CI入口：" + row.workflows.join(", ") : "；无已知CI入口"));
  }
  if (selection.removedTests?.length) console.log("\n已删除测试不执行，关联验证仍保留：" + selection.removedTests.join(", "));
  if (selection.relatedScripts.length) {
    console.log("\n相关脚本（引用关系提示，不代表已执行或已验证；本地不自动执行）：");
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

function runBrowsers(selection, {runCommand=run, cleanCommand=(env)=>cp.spawnSync("go",["clean","-cache","-testcache"],{cwd:root,env,encoding:"utf8"})} = {}) {
  const out = fs.mkdtempSync(path.join(os.tmpdir(), "preflight-browsers-"));
  const results = [];
  const names = selection.browserGuards.map((g) => g.name).filter((n) => n !== R277_STRICT);
  try {
  if (names.length) {
    results.push(runCommand(`浏览器守卫 ${names.join(",")}`, process.execPath, ["scripts/r276-browser-job.cjs", "all", path.join(out, "guards")], { env: { R276_BROWSER_ONLY: names.join(",") } }));
  }
  if (selection.browserGuards.some((g) => g.name === R277_STRICT)) {
    const backend = path.join(out, process.platform === "win32" ? "r277.exe" : "r277-browser-public");
    const target = path.join(out, "r277");
    results.push(runCommand("R277 构建浏览器后端", process.execPath, ["scripts/r277-ci-orchestrate.cjs", "build-browser", "--backend", backend, "--out", target], {env:{PI_SCRATCH_DIR:out}}));
    if (results.at(-1).ok) results.push(runCommand("R277 严格几何矩阵", process.execPath, ["scripts/r277-ci-orchestrate.cjs", "browser", "--backend", backend, "--out", target], {env:{PI_SCRATCH_DIR:out}}));
  }
  return results;
  } finally {
    try {
      const cache=path.join(out,'r277-browser-gocache');
      if(fs.existsSync(cache)){const result=cleanCommand({...process.env,GOCACHE:cache});if(result.status!==0)throw Error('Browser GOCACHE cleanup failed: '+(result.stderr||result.error));}
    } finally { fs.rmSync(out,{recursive:true,force:true}); }
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    console.log("用法：node scripts/preflight.cjs [--base <ref>] [--list] [--json <file>] [--files a,b,c] [--browsers]\n  --list      只显示选择结果，不执行\n  --files     手动指定改动文件（逗号分隔），用于回放历史改动\n  --browsers  按成本决策运行浏览器守卫，过重时明确延后CI\n  --full      显式执行全部已选关联验证（可与--browsers一起使用）");
    return 0;
  }
  const gateBase = resolveBase(options.base);
  const base = options.files.length ? "(手动指定文件)" : gateBase;
  const changes = options.files.length ? options.files.map((file) => ({ file, status: "M" })) : changedFiles(base);
  const index = loadIndex(gateBase, root, changes);
  const selection = select(index, changes);
  const total = { node: index.nodeTests.length, go: index.goTests.length };
  if (!changes.length) { console.log("没有改动，无需预检。"); return 0; }
  printSummary(base, selection, total);
  const behavior=behaviorChecks(gateBase,changes);
  if(behavior.conservativeFiles.length)console.log('契约分析为整文件保守回放（不是实际变更行）：'+behavior.conservativeFiles.join(', '));
  if(behavior.contractImpact.identifiers.length){
    console.log('\n可能需要同步的契约：'+behavior.contractImpact.identifiers.join(', '));
    for(const row of behavior.contractImpact.files)console.log(`  ${row.file}: 引用行 ${row.references.map(r=>r.line).join(',')}；断言行 ${row.assertions.map(r=>r.line).join(',')}`);
  }
  if(behavior.fixtureAlignment){
    console.log(`启动夹具对齐：${behavior.fixtureAlignment.startupEndpoints.length}个静态端点候选、${behavior.fixtureAlignment.automaticUI.length}处自动界面线索`);
    for(const issue of behavior.fixtureAlignment.issues)console.log(`  ✘ ${issue.guard} ${issue.file}:${issue.line} ${issue.reason}`);
  }

  const parallel = Math.max(1, Math.min(os.availableParallelism?.() ?? os.cpus().length, 6));
  const execution=planExecution(selection,{workers:parallel,goNamesByFile:index.goTestNames,full:Boolean(options.full),browsers:options.browsers});
  console.log(`成本估算：Node worker约${Math.round(execution.estimate.nodeWorkerMs/1000)}s，已测文件成本占比${Math.round(execution.estimate.nodeFileCostShare*100)}%；未知Node成本${execution.estimate.unknownNodeFiles.length}文件；Go ${execution.estimate.goCases}用例（无耗时档案）；浏览器串行约${execution.estimate.browserSerialMs===null?"未知":Math.round(execution.estimate.browserSerialMs/1000)+"s"}。估算不是预算或通过证据。`);
  if(execution.deferAssociated)console.log(`关联验证延后CI：${execution.reasons.join("; ")}；直接改动测试仍回归。`);
  if(execution.deferBrowser)console.log("浏览器工作量超出本地反馈目标，保留完整清单交Windows诊断/CI；显式 --full --browsers 可本地完整执行。");
  const staleValues=require('./lib/stale-values.cjs').scanRepository(root,gateBase);
  for(const hit of staleValues)console.log(`仍断言旧值：${hit.file}:${hit.line} ${JSON.stringify(hit.value)}（来自 ${hit.from}）`);
  const report = { staleValues, base, gateBase, generatedAt: new Date().toISOString(), totals: total, selection, execution, behavior, results: [] };
  let exitCode = staleValues.length?1:0;
  if (!options.list) {
    if(!preflightGocache){preflightGocache=fs.mkdtempSync(path.join(os.tmpdir(),'r287-preflight-gocache-'));ownedGocache=true;}
    if(staleValues.length){exitCode=1;report.results.push({label:'旧值扫描',ok:false,hits:staleValues});}
    if(behavior.fixtureAlignment?.issues.length){exitCode=1;report.results.push({label:'启动夹具对齐',ok:false,issues:behavior.fixtureAlignment.issues});}
    for (const gate of staticGates(selection, gateBase)) {
      const result = run(gate.label, gate.command, gate.args);
      report.results.push(result);
      if (!result.ok) exitCode = 1;
    }
    if(exitCode!==0){
      for(const [label,selected] of [["脚本测试",execution.scriptTests.map(row=>row.file)],["Node回归",execution.nodeTests.map(row=>row.file)],["Go回归",execution.goTestNames],["浏览器",options.browsers&&!execution.deferBrowser?selection.browserGuards.map(row=>row.name):[]]])if(selected.length)report.results.push({label,status:"skipped-static-gate-failed",selected,note:"前置门禁失败，未执行且不算通过；修复前置后必须重新执行"});
    }
    if(execution.deferAssociated||execution.deferBrowser)report.results.push({label:"成本感知关联验证",status:"deferred-to-CI",note:"完整选择清单保留；直接修改测试回归，其余关联验证由CI执行，未执行不算通过",deferred:execution.deferred});
    if (exitCode===0 && execution.nodeTests.length) {
      const result = run(`Node 测试（${execution.nodeTests.length} 个文件，并发 ${parallel}）`, process.execPath, ["--test", `--test-concurrency=${parallel}`, ...execution.nodeTests.map((t) => t.file)]);
      report.results.push(result);
      if (!result.ok) exitCode = 1;
    }
    if (exitCode===0 && execution.scriptTests.length) {
      for (const result of runScriptTests(execution.scriptTests)) {
        report.results.push(result);
        if (result.ok === false) exitCode = 1;
        if (result.ok === null) console.log("待Windows CI验证：" + result.file + " → " + result.workflows.join(", ") + "（未通过）");
      }
    }
    if (exitCode===0 && execution.goTestNames.length) {
      const pattern = `^(${execution.goTestNames.join("|")})$`;
      const result = run(`Go 测试（${execution.goTestNames.length} 个用例${options.full?"，-race":""}）`, "go", ["test", ...(options.full?["-race"]:[]), "-count=1", "-run", pattern, "./backend"]);
      report.results.push(result);
      if (!result.ok) exitCode = 1;
    }
    if (exitCode===0 && options.browsers && !execution.deferBrowser && selection.browserGuards.length) {
      for (const result of runBrowsers(selection)) { report.results.push(result); if (!result.ok) exitCode = 1; }
    }
    const platformPending=report.results.some(r=>r.status==="deferred-to-Windows-CI");
    const summary=exitCode!==0 ? "快速预检有失败" : platformPending ? "本地已执行预检通过，直接改动Windows测试仍待CI" : execution.deferAssociated||execution.deferBrowser ? "本地已执行预检通过，其余关联验证明确延后CI" : "快速预检通过";
    console.log("\n"+summary+"。注意：这不是发版证据；完整验证以 probe / release 分支上的 CI 为准。");
    if (report.results.some(r=>r.status==="deferred-to-Windows-CI")) console.log("直接改动的Windows测试尚未通过；必须以列出的Windows CI实际结果补齐，不能把本地预检称为全部测试通过。");
    if (selection.browserGuards.length && (!options.browsers||execution.deferBrowser)) console.log("浏览器守卫未在本地运行（加 --browsers 查看成本决策，--full --browsers 显式完整运行）；它们在 CI 的 browser-main / browser-r265 / R277 matrix 里一定会跑。");
  }
  if (options.json) { fs.mkdirSync(path.dirname(path.resolve(options.json)), { recursive: true }); fs.writeFileSync(options.json, JSON.stringify(report, null, 2) + "\n");fs.writeFileSync(path.join(path.dirname(path.resolve(options.json)),"preflight-static-gates.json"),JSON.stringify({automaticStaging:false,gates:staticGates(selection,gateBase)},null,2)+"\n"); }
  return exitCode;
}

function cleanupCaches(){
  if(!preflightGocache)return;
  const env={...process.env};delete env.GOCACHE;
  for(const [label,cacheEnv]of [['default',env],['dedicated',{...env,GOCACHE:preflightGocache}]]){
    const result=cp.spawnSync('go',['clean','-cache','-testcache'],{cwd:root,env:cacheEnv,encoding:'utf8'});
    console.log(JSON.stringify({cleanup:'go clean -cache -testcache',cache:label,ok:result.status===0}));
    if(result.status!==0){console.error(result.stderr||result.error);process.exitCode=1;}
  }
  if(ownedGocache)fs.rmSync(preflightGocache,{recursive:true,force:true});
}

if (require.main === module) {
  main().then((code) => { process.exitCode = code; }, (error) => { console.error(error.stack || error); process.exitCode = 2; }).finally(cleanupCaches);
}

module.exports = { parseArgs, resolveBase, changedFiles, loadIndex, staticGates, runScriptTests, behaviorChecks, cleanupCaches, runBrowsers };
