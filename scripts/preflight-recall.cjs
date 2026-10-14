#!/usr/bin/env node
"use strict";
// R279 P2: recall check for the change-aware selector against REAL historical CI/renderer failures.
// Every recorded failure must be selected (or flagged) for the diff that caused it.
//   node scripts/preflight-recall.cjs [--json out.json] [--verify-git]
const fs = require("node:fs"), path = require("node:path"), cp = require("node:child_process");
const { buildIndex, select, WINDOWS_ONLY_NOTE } = require("./lib/preflight-select.cjs");
const root = path.resolve(__dirname, "..");
const dataset = require("./preflight-recall-cases.json");

function loadIndex() {
  const files = cp.execFileSync("git", ["ls-files", "-co", "--exclude-standard"], { cwd: root, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 }).split("\n").filter(Boolean);
  const { guards } = require("./r276-browser-catalog.cjs");
  return buildIndex({ files, read: (file) => fs.readFileSync(path.join(root, file), "utf8"), guards: guards.map((row) => row[0]) });
}

function covers(selection, expression) {
  if (expression === "windows-only") return selection.windowsOnly.includes(WINDOWS_ONLY_NOTE);
  const [kind, ...rest] = expression.split(":");
  const value = rest.join(":");
  if (kind === "guard") return selection.browserGuards.some((g) => g.name === value);
  if (kind === "node") return selection.nodeTests.some((t) => t.file === value);
  if (kind === "script") return selection.relatedScripts.some((s) => s.file === value) || selection.nodeTests.some((t) => t.file === value);
  throw new Error(`unknown expression ${expression}`);
}

function evaluate(index = loadIndex()) {
  const bySet = new Map();
  for (const [id, set] of Object.entries(dataset.changeSets)) bySet.set(id, select(index, set.changed));
  const rows = dataset.cases.map((entry) => {
    const selection = bySet.get(entry.changeSet);
    const missing = entry.mustSelect.filter((expression) => !covers(selection, expression));
    return { id: entry.id, changeSet: entry.changeSet, failure: entry.failure, mustSelect: entry.mustSelect, missing, ok: missing.length === 0 };
  });
  const sets = [...bySet].map(([id, s]) => ({
    id, changedFiles: s.changed.length, nodeSelected: s.nodeTests.length, nodeTotal: index.nodeTests.length,
    nodeRatio: s.ratio, goTestFiles: s.goTestFiles.length, goTotal: index.goTests.length, guards: s.browserGuards.length,
    relatedScripts: s.relatedScripts.length, unmapped: s.unmapped, recommendProbe: s.recommendProbe,
  }));
  return { rows, sets, recalled: rows.filter((r) => r.ok).length, total: rows.length, notSelectable: dataset.notSelectable };
}

function verifyGit() {
  let bad = 0;
  for (const [id, set] of Object.entries(dataset.changeSets)) {
    if (!set.from || !set.to) continue;
    let actual;
    try { actual = cp.execFileSync("git", ["diff", "--name-only", set.from, set.to], { cwd: root, encoding: "utf8" }).split("\n").filter(Boolean).sort(); }
    catch (error) { console.log(`跳过 ${id}：本地没有该历史（${error.message.split("\n")[0]}）`); continue; }
    const recorded = [...set.changed].sort();
    if (JSON.stringify(actual) !== JSON.stringify(recorded)) { bad++; console.log(`✘ ${id} 记录的文件列表与 git 历史不一致`); } else console.log(`✔ ${id} 与 git 历史一致（${actual.length} 个文件）`);
  }
  return bad;
}

if (require.main === module) {
  const args = process.argv.slice(2);
  if (args.includes("--verify-git")) process.exit(verifyGit() ? 1 : 0);
  const result = evaluate();
  for (const row of result.rows) console.log(`${row.ok ? "✔" : "✘"} ${row.id}  ← ${row.failure}${row.ok ? "" : `\n    漏选：${row.missing.join(", ")}`}`);
  console.log(`\n召回 ${result.recalled}/${result.total}；不可选择类 ${result.notSelectable.length} 项：${result.notSelectable.map((n) => n.id).join(", ")}`);
  console.log("\n各改动集的选择规模：");
  for (const s of result.sets) console.log(`  ${s.id}: 改动 ${s.changedFiles} 个文件 → Node ${s.nodeSelected}/${s.nodeTotal}（${Math.round(s.nodeRatio * 100)}%），Go 文件 ${s.goTestFiles}/${s.goTotal}，浏览器守卫 ${s.guards}，相关脚本 ${s.relatedScripts}，未覆盖 ${s.unmapped.length}，建议直接推 probe：${s.recommendProbe}`);
  const out = args[args.indexOf("--json") + 1];
  if (args.includes("--json") && out) fs.writeFileSync(out, JSON.stringify(result, null, 2) + "\n");
  process.exit(result.recalled === result.total ? 0 : 1);
}
module.exports = { evaluate, covers, loadIndex };
