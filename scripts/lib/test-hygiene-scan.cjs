"use strict";
// R279 P3: static scan for the three timing anti-patterns RELEASE-PROCESS section C forbids in NEW tests:
//   sleep   - a fixed setTimeout(..., >=200ms) used as a pause/sleep
//   poll    - a fixed-iteration or fixed-deadline polling loop (for i<N / while Date.now()<deadline)
//   wall    - an assertion comparing a measured elapsed wall-clock time with a fixed threshold
// Existing uses are recorded in scripts/test-hygiene-baseline.json and may only decrease.

const fs = require("node:fs"), path = require("node:path");

const CATEGORIES = ["sleep", "poll", "wall"];

function stripComments(source) {
  return source.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, " ")).replace(/(^|[^:'"`\\])\/\/[^\n]*/g, (m, p) => p + " ".repeat(m.length - p.length));
}

function scanSource(source) {
  const text = stripComments(source);
  let sleep = 0;
  for (const m of text.matchAll(/setTimeout\(\s*[^,()]*(?:\([^()]*\))?[^,()]*,\s*(\d{3,})\s*\)/g)) if (Number(m[1]) >= 200) sleep++;
  let poll = 0;
  poll += (text.match(/for\s*\(\s*let\s+\w+\s*=\s*0\s*;\s*\w+\s*<\s*\d+\s*;\s*\w+\+\+\s*\)\s*\{[^}]{0,240}setTimeout/g) || []).length;
  poll += (text.match(/while\s*\(\s*Date\.now\(\)\s*(?:-\s*\w+\s*)?<\s*(?:\w+|\d+)\s*\)/g) || []).length;
  let wall = 0;
  wall += (text.match(/assert[\w.]*\([^;]{0,100}(?:Date\.now\(\)|performance\.now\(\))\s*-\s*[\w.]+\s*[<>]=?\s*\d+/g) || []).length;
  return { sleep, poll, wall };
}

function listFiles(root) {
  const out = [];
  for (const dir of ["backend/web", "desktop", "scripts"]) {
    const full = path.join(root, dir);
    if (!fs.existsSync(full)) continue;
    for (const name of fs.readdirSync(full)) if (name.endsWith(".cjs")) out.push(`${dir}/${name}`);
  }
  return out.sort();
}

// Performance-budget tooling measures time on purpose (RELEASE-PROCESS C exception); desktop/main.cjs and
// desktop/share-export.cjs are product code that merely lives next to the tests.
const EXEMPT = new Set(["scripts/renderer-budget-gate.cjs", "scripts/renderer-budgets.cjs", "scripts/runner-calibration.cjs", "scripts/test-renderers.cjs", "scripts/r277-perf-compare.cjs", "scripts/test-hygiene-baseline.cjs", "desktop/main.cjs", "desktop/share-export.cjs", "scripts/test-hygiene.test.cjs"]);

function scanRepo(root) {
  const result = {};
  for (const file of listFiles(root)) {
    if (EXEMPT.has(file) || file.endsWith("test-hygiene-scan.cjs")) continue;
    const counts = scanSource(fs.readFileSync(path.join(root, file), "utf8"));
    if (CATEGORIES.some((c) => counts[c] > 0)) result[file] = counts;
  }
  return result;
}

// Returns {increases:[...], stale:[...]}: increases = new debt, stale = baseline higher than reality (must be lowered).
function compare(baseline, current) {
  const increases = [], stale = [];
  const files = new Set([...Object.keys(baseline), ...Object.keys(current)]);
  for (const file of [...files].sort()) {
    for (const category of CATEGORIES) {
      const allowed = baseline[file]?.[category] ?? 0, actual = current[file]?.[category] ?? 0;
      if (actual > allowed) increases.push({ file, category, allowed, actual });
      else if (actual < allowed) stale.push({ file, category, allowed, actual });
    }
  }
  return { increases, stale };
}

module.exports = { scanSource, scanRepo, compare, listFiles, stripComments, CATEGORIES, EXEMPT };
