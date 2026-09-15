"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const source = fs.readFileSync(path.join(__dirname, "suite.js"), "utf8");
const start = source.indexOf("  async function loadChampSelectPositionFilter(");
const filterSource = source.slice(start, source.indexOf("  function handleWatchEvent(", start));
function fixture(api, clock = {setTimeout, clearTimeout}) {
  const state = { champSelectDialog: {}, champSelectPositionCache: new Map() };
  const reports = [], messages = [];
  let renders = 0;
  const load = Function("state", "api", "updateChampSelectDialog", "toast", "window", "setTimeout", "clearTimeout", `${filterSource};return loadChampSelectPositionFilter;`)(state, api, () => renders++, msg => messages.push(msg), {reportFlowDiagnostic: (...args) => reports.push(args)}, clock.setTimeout, clock.clearTimeout);
  return {state, load, reports, messages, renders: () => renders};
}
function deferred() { let resolve, reject; const promise = new Promise((a,b) => {resolve=a; reject=b;}); return {promise,resolve,reject}; }
const row = championId => ({ rows: [{championId}] });

test("filter deadline aborts fetch and records a retryable timeout",async()=>{
 let timeout;
 const f=fixture((_url,{signal})=>new Promise((_,reject)=>signal.addEventListener("abort",()=>{const e=Error("aborted");e.name="AbortError";reject(e);})),{setTimeout:(fn,ms)=>{assert.equal(ms,15000);timeout=fn;return 1;},clearTimeout:()=>{}});
 const pending=f.load("middle");timeout();await pending;
 assert.equal(f.reports.at(-1)[2].errorKind,"timeout");assert.equal(f.state.champSelectPosition,"all");assert.equal(f.state.champSelectPositionCache.size,0);
});

test("all five modal lane values match the real backend ranking whitelist", async () => {
  const backend = fs.readFileSync(path.join(__dirname,"../champions.go"),"utf8");
  const allowed = new Set([...backend.match(/championPositionNames = map\[string\]string\{([^\n]+)\}/)[1].matchAll(/"([^"]+)":/g)].map(m=>m[1]));
  const calls=[];
  const f=fixture(async url=>{const p=new URL(url,"http://localhost").searchParams.get("position");assert.ok(allowed.has(p));calls.push(p);return row(1);});
  for(const p of ["top","jungle","middle","bottom","utility","all"]) await f.load(p);
  assert.deepEqual(calls,["top","jungle","mid","adc","support"]);
  assert.equal(f.state.champSelectPositionIDs,null);
  await f.load("middle"); assert.equal(calls.length,5);
  assert.equal(f.reports.at(-1)[1],"cached");
});
test("stale success cannot overwrite all, newer lane, or a reopened dialog", async () => {
  for(const mode of ["all","newer","reopened","cached"]) {
    const old=deferred();const f=fixture(url=>url.includes("position=top")?old.promise:Promise.resolve(row(2)));
    f.state.champSelectPositionCache.set("utility",new Set([3]));
    const pending=f.load("top");
    if(mode==="reopened") {f.state.champSelectDialog={};f.state.champSelectPosition="all";f.state.champSelectPositionIDs=null;}
    else await f.load(mode==="newer"?"bottom":mode==="cached"?"utility":"all");
    const before=f.state.champSelectPositionIDs, renders=f.renders();
    old.resolve(row(1));await pending;
    assert.equal(f.state.champSelectPositionIDs,before,mode);assert.equal(f.renders(),renders);
    assert.equal(f.reports.at(-1)[1],"stale");
  }
});
test("stale failure is quiet, current failure resets and is retryable",async()=>{
  const old=deferred();let attempts=0;
  const f=fixture(()=>++attempts===1?old.promise:Promise.resolve(row(4)));
  const pending=f.load("middle");await f.load("all");old.reject(Error("old failure"));await pending;
  assert.equal(f.messages.length,0);await f.load("middle");assert.deepEqual([...f.state.champSelectPositionIDs],[4]);
  const g=fixture(async()=>{const e=Error("invalid");e.status=400;e.errorKind="http";throw e;});
  await g.load("bottom");assert.equal(g.state.champSelectPosition,"all");assert.equal(g.state.champSelectPositionCache.size,0);
  assert.equal(g.reports.at(-1)[2].httpStatus,400);assert.equal(g.reports.at(-1)[2].errorKind,"http");
});
test("malformed rows are not cached as a successful empty list",async()=>{
 const f=fixture(async()=>({}));await f.load("utility");
 assert.equal(f.state.champSelectPositionCache.size,0);assert.equal(f.reports.at(-1)[2].errorKind,"invalid-response");
});
test("closing the dialog prevents old response application",async()=>{
 const d=deferred();const f=fixture(()=>d.promise);const pending=f.load("top");
 f.state.champSelectDialog=null;f.state.champSelectPositionController.abort();d.resolve(row(1));await pending;
 assert.equal(f.renders(),0);assert.equal(f.messages.length,0);
});

test("R87 P9 disposal aborts filtering and late success cannot update the dialog", async () => {
 const d=deferred();const f=fixture(()=>d.promise);const pending=f.load("top");
 const signal=f.state.champSelectPositionController.signal;
 let closes=0;
 const closeStart=source.indexOf("  function closeChampSelectDialog(");
 const closeSource=source.slice(closeStart,source.indexOf("  function champSelectFilteredChampions(",closeStart));
 const disposeStart=source.indexOf("  function disposeSuite(");
 const disposeSource=source.slice(disposeStart,source.indexOf('  window.addEventListener("deep-legends:dispose"',disposeStart));
 const dispose=Function("state","syncChampSelectDialog","clearTimeout",`${closeSource}\n${disposeSource};return disposeSuite;`)(f.state,()=>closes++,clearTimeout);
 dispose();assert.equal(signal.aborted,true);assert.equal(f.state.champSelectDialog,null);assert.equal(closes,1);
 d.resolve(row(1));await pending;
 assert.equal(f.renders(),0);assert.equal(f.messages.length,0);assert.equal(f.state.champSelectPositionCache.size,0);
 await f.load("bottom");assert.equal(f.state.champSelectPositionController,null);
});
