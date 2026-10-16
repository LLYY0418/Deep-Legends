"use strict";
// R279 P3: the shared wait helper, driven by a controllable clock (no real sleeping, no wall-clock thresholds).
const test = require("node:test");
const assert = require("node:assert/strict");
const { waitFor, waitOutcome, waitForPage, PAGE_PROGRESS_EXPRESSION } = require("./lib/wait-progress.cjs");

function clock() {
  let time = 0;
  return { now: () => time, sleep: async (ms) => { time += ms; }, advance: (ms) => { time += ms; }, get time() { return time; } };
}

test("returns immediately when the condition is already true", async () => {
  const c = clock();
  const outcome = await waitOutcome(() => true, { now: c.now, sleep: c.sleep });
  assert.deepEqual({ ok: outcome.ok, polls: outcome.polls, ms: outcome.ms }, { ok: true, polls: 0, ms: 0 });
});

test("keeps waiting past the idle window while progress keeps changing, then succeeds", async () => {
  const c = clock();
  let polls = 0;
  const outcome = await waitOutcome(() => polls > 40, {
    now: c.now, sleep: async (ms) => { polls++; await c.sleep(ms); }, idleMs: 100, hardMs: 100000, intervalMs: 10,
    progress: () => polls, // changes every poll
  });
  assert.equal(outcome.ok, true);
  assert.ok(c.time > 100, "total wait exceeded the idle window because progress never stopped");
});

test("mutation: a condition that never becomes true and never progresses fails at the idle window (watchdog fires)", async () => {
  const c = clock();
  const outcome = await waitOutcome(() => false, { now: c.now, sleep: c.sleep, idleMs: 250, hardMs: 100000, intervalMs: 10, progress: () => "frozen" });
  assert.equal(outcome.ok, false);
  assert.equal(outcome.reason, "stalled");
  assert.ok(outcome.ms >= 250 && outcome.ms < 300, `stalled at ${outcome.ms}`);
  assert.equal(outcome.progress, '"frozen"');
});

test("the hard limit still ends a livelock whose progress keeps changing", async () => {
  const c = clock();
  let ticks = 0;
  const outcome = await waitOutcome(() => false, { now: c.now, sleep: async (ms) => { ticks++; await c.sleep(ms); }, idleMs: 100, hardMs: 1000, intervalMs: 10, progress: () => ticks });
  assert.equal(outcome.ok, false);
  assert.equal(outcome.reason, "hard-limit");
  assert.ok(outcome.ms >= 1000);
});

test("without a progress probe the idle window behaves exactly like the fixed window it replaces", async () => {
  const c = clock();
  const outcome = await waitOutcome(() => false, { now: c.now, sleep: c.sleep, idleMs: 5000, intervalMs: 10 });
  assert.equal(outcome.ok, false);
  assert.equal(outcome.reason, "stalled");
  assert.ok(outcome.ms >= 5000 && outcome.ms < 5100);
});

test("hardMs is never allowed to be shorter than idleMs", async () => {
  const c = clock();
  const outcome = await waitOutcome(() => false, { now: c.now, sleep: c.sleep, idleMs: 800, hardMs: 100, intervalMs: 10, progress: () => "x" });
  assert.equal(outcome.reason, "stalled");
  assert.ok(outcome.ms >= 800);
});

test("waitFor throws a labelled error carrying the last observed progress", async () => {
  const c = clock();
  await assert.rejects(waitFor(() => false, { label: "full app did not mount", now: c.now, sleep: c.sleep, idleMs: 100, progress: () => ({ nodes: 7 }) }), (error) => {
    assert.equal(error.code, "WAIT_STALLED");
    assert.match(error.message, /full app did not mount: no observable progress for 100 ms; last progress \{"nodes":7\}/);
    assert.equal(error.progress, '{"nodes":7}');
    return true;
  });
});

test("errors thrown by the condition propagate; errors in the progress probe do not break the wait", async () => {
  const c = clock();
  await assert.rejects(waitOutcome(() => { throw new Error("check exploded"); }, { now: c.now, sleep: c.sleep }), /check exploded/);
  let n = 0;
  const outcome = await waitOutcome(() => ++n > 3, { now: c.now, sleep: c.sleep, progress: () => { throw new Error("probe exploded"); } });
  assert.equal(outcome.ok, true);
});

test("waitForPage reports true/false instead of throwing and probes the page through evaluate()", async () => {
  const c = clock();
  const seen = [];
  let calls = 0;
  const evaluate = async (expression) => { seen.push(expression); if (expression === PAGE_PROGRESS_EXPRESSION) return { e: calls }; calls++; return calls > 5; };
  assert.equal(await waitForPage(evaluate, "window.ready", { now: c.now, sleep: c.sleep }), true);
  assert.ok(seen.includes("window.ready") && seen.includes(PAGE_PROGRESS_EXPRESSION));
  const frozen = async (expression) => (expression === PAGE_PROGRESS_EXPRESSION ? { e: 1 } : false);
  assert.equal(await waitForPage(frozen, "window.never", { now: c.now, sleep: c.sleep, idleMs: 200 }), false);
});

function playerReadiness(mode) {
  const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
  const source=fs.readFileSync(path.join(__dirname,'../desktop/pro-players.test.cjs'),'utf8');
  const start=source.indexOf('  const ready = async (name) => {'),end=source.indexOf('\n  open("First")',start);
  assert(start>=0&&end>start,'actual player readiness fixture must be exercised');
  const c=clock(),observers=[],targets=[];let loaded=false,frames=0;
  const d={getElementById:id=>({id}),getElementsByTagName(){throw Error('readiness must not scan the whole DOM');},querySelector(selector){if(mode==='check-error')throw Error('original readiness check failed');return selector.includes('player-tabs')?{textContent:'First',dataset:{playerTab:'first-key'},querySelector:()=>loaded?null:{}}:loaded?{}:null;}};
  const w={MutationObserver:class{constructor(callback){this.callback=callback;this.disconnected=false;observers.push(this);}observe(target,options){targets.push({target,options});}disconnect(){this.disconnected=true;}},requestAnimationFrame(callback){frames++;callback();}};
  const instrumented=async(check,options)=>{
    assert.equal(observers.length,1,'readiness progress must observe changes rather than rescan the whole DOM');
    assert.equal(options.idleMs,20000);assert.equal(options.hardMs,120000);assert.equal(options.intervalMs,20);
    assert.equal(options.progress(),0);
    return waitOutcome(check,{...options,now:c.now,sleep:async ms=>{await c.sleep(mode==='success'?ms:20000);if(mode==='success'||mode==='hard-limit'){observers[0].callback([]);assert(options.progress()>0);}if(mode==='success')loaded=true;}});
  };
  const ready=vm.runInNewContext('(()=>{'+source.slice(start,end)+';return ready;})()',{w,d,waitOutcome:instrumented,assert});
  return {ready,observers,targets,frames:()=>frames};
}

test('actual player readiness observes progress without repeated full-page scans and keeps both restoration frames',async()=>{
  const f=playerReadiness('success');assert.equal(await f.ready('First'),'first-key');assert.equal(f.frames(),2);assert.deepEqual(f.targets.map(t=>t.target.id),['player-tabs','overview-content']);assert(f.targets.every(t=>t.options.childList&&t.options.subtree&&t.options.characterData));assert(f.observers[0].disconnected);
});
for(const reason of ['stalled','hard-limit'])test('actual player readiness retains '+reason+' failure and disconnects progress observation',async()=>{
  const f=playerReadiness(reason);await assert.rejects(f.ready('First'),new RegExp('player First did not finish loading.*'+reason));assert(f.observers[0].disconnected);assert.equal(f.frames(),0);
});
test('actual player readiness preserves a thrown check and disconnects its observer',async()=>{
  const f=playerReadiness('check-error');await assert.rejects(f.ready('First'),/original readiness check failed/);assert(f.observers[0].disconnected);
});
