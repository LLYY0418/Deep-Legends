"use strict";
// R279 P3: static scan for the three timing anti-patterns RELEASE-PROCESS section C forbids in NEW tests:
//   sleep   - a fixed setTimeout(..., >=200ms) used as a pause/sleep
//   poll    - a fixed-iteration or fixed-deadline polling loop (for i<N / while Date.now()<deadline)
//   wall    - an assertion comparing a measured elapsed wall-clock time with a fixed threshold
// Existing uses are recorded in scripts/test-hygiene-baseline.json and may only decrease.

const fs = require("node:fs"), path = require("node:path");

const CATEGORIES = ["sleep", "poll", "wall", "silent-timeout", "early-ready"];

function stripComments(source) {
  return source.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, " ")).replace(/(^|[^:'"`\\])\/\/[^\n]*/g, (m, p) => p + " ".repeat(m.length - p.length));
}

// Read a balanced call, including multiline calls and CDP expression strings.
// This is deliberately a source scanner, not a claim of full JS control-flow proof.
function callEnd(text,start) {
  let depth=0,quote='',escaped=false;
  for(let i=start;i<text.length;i++){
    const c=text[i];
    if(quote){if(escaped)escaped=false;else if(c==='\\')escaped=true;else if(c===quote)quote='';continue;}
    if(c==='"'||c==="'"||c==='`'){quote=c;continue;}
    if(c==='(')depth++;
    if(c===')'&&--depth===0)return i+1;
  }
  return text.length;
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
  let silentTimeout=0;
  for(const promise of text.matchAll(/new\s+Promise\s*\(\s*\(?\s*(\w+)/g)){
    const start=text.indexOf('(',promise.index),body=text.slice(start,callEnd(text,start)),resolve=promise[1];
    // A lone timer pause belongs to sleep. A readiness check plus a resolving
    // timeout is the silent escape that can turn failed readiness into success.
    if(!/\bsetInterval\s*\(|\b(?:if|while)\s*\(/.test(body))continue;
    for(const timer of body.matchAll(/\bsetTimeout\s*\(/g)){
      const timerStart=body.indexOf('(',timer.index),callback=body.slice(timerStart+1,callEnd(body,timerStart)-1);
      const resolves=new RegExp('(?:^\\s*'+resolve+'\\s*,|\\b'+resolve+'\\s*\\(\\s*\\))').test(callback);
      if(resolves&&!/\b(?:reject|throw)\b/.test(callback))silentTimeout++;
    }
  }
  let earlyReady=0;
  const calls=[...text.matchAll(/\bawait\s+(waitForDOM|waitForPage|until|waitAppReady|navigateAppSection)\s*\(/g)].map(match=>({start:match.index,name:match[1],source:text.slice(match.index,callEnd(text,text.indexOf('(',match.index)))}));
  for(const call of calls){
    if(!['waitForDOM','waitForPage','until'].includes(call.name)||!/(?:hasAttribute\(\s*['"]inert|\.hidden|querySelector)/.test(call.source))continue;
    const tail=text.slice(call.start+call.source.length),action=/deep-legends:navigate|getBoundingClientRect\s*\(/.exec(tail);
    if(!action)continue;
    const actionAt=call.start+call.source.length+action.index,lastNavigation=text.lastIndexOf('Page.navigate',actionAt);
    if(call.start<lastNavigation)continue;
    const strong=calls.some(other=>other.start>lastNavigation&&other.start<=actionAt&&(['waitAppReady','navigateAppSection'].includes(other.name)||/readyState\s*===\s*['"]complete['"]/.test(other.source)&&/deepLegendsToast|\bappReady\b/.test(other.source)));
    if(!strong)earlyReady++;
  }
  return { sleep, poll, wall, "silent-timeout":silentTimeout, "early-ready":earlyReady };
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

module.exports = { scanSource, scanRepo, compare, listFiles, stripComments, CATEGORIES, EXEMPT, callEnd };
