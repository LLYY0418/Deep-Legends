#!/usr/bin/env node
"use strict";
// R279 P3.3: writes scripts/test-hygiene-baseline.json from the current scan.
//   node scripts/test-hygiene-baseline.cjs --write   (lower or remove entries after cleaning up debt)
// Existing reasons are preserved. A NEW file or category can only be added with --allow-new "<reason>", which a reviewer
// has to approve: the baseline is a ratchet that only goes down.
const fs = require("node:fs"), path = require("node:path");
const { scanRepo, CATEGORIES } = require("./lib/test-hygiene-scan.cjs");
const root = path.resolve(__dirname, "..");
const file = path.join(root, "scripts", "test-hygiene-baseline.json");

function build(previous, current, allowNew) {
  const files = {};
  for (const [name, counts] of Object.entries(current)) {
    const before = previous.files?.[name];
    const entry = {};
    for (const category of CATEGORIES) {
      const allowed = before?.[category] ?? 0;
      const actual = counts[category];
      if (actual > allowed && !allowNew) throw new Error(`${name}: ${category} 从 ${allowed} 增加到 ${actual}；新增固定等待必须改用 scripts/lib/wait-progress.cjs，或经评审后用 --allow-new "<原因>" 登记`);
      if (actual > 0) entry[category] = actual;
    }
    entry.reason = before?.reason && !(allowNew && CATEGORIES.some((c) => counts[c] > (before?.[c] ?? 0))) ? before.reason : allowNew;
    if (!entry.reason) throw new Error(`${name}: 缺少原因`);
    files[name] = entry;
  }
  return { version: 1, note: "存量计时债务登记表，只减不增。新增 sleep/轮询/墙钟断言必须使用 scripts/lib/wait-progress.cjs。", files };
}

if (require.main === module) {
  const args = process.argv.slice(2);
  if (!args.includes("--write")) { console.log("用法：node scripts/test-hygiene-baseline.cjs --write [--allow-new \"原因\"]"); process.exit(0); }
  const i = args.indexOf("--allow-new");
  const allowNew = i >= 0 ? args[i + 1] : "";
  const previous = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : { files: {} };
  const next = build(previous, scanRepo(root), allowNew);
  fs.writeFileSync(file, JSON.stringify(next, null, 2) + "\n");
  console.log(`已写入 ${path.relative(root, file)}：${Object.keys(next.files).length} 个文件`);
}
module.exports = { build };
