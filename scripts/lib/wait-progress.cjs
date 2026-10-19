"use strict";
// R279 P3: one shared "wait until, but only fail when nothing is happening" helper.
//
// A fixed window ("5 seconds") fails whenever the machine is merely slow: five jsdom workers on two
// CPUs, a cold Windows runner, a busy Chrome. This helper keeps waiting as long as the observable
// state keeps CHANGING (progress), and fails only when
//   * nothing observable changed for `idleMs` (stalled), or
//   * `hardMs` elapsed in total (hard ceiling, so a livelock still fails).
// Both limits are never SHORTER than the fixed window they replace, so a migrated wait can only get
// more tolerant of slowness; a condition that can never become true still fails, with the last
// observed progress in the message for triage.

const DEFAULTS = { idleMs: 15000, hardMs: 120000, intervalMs: 10 };

function describe(value) {
  try { return JSON.stringify(value) ?? "undefined"; } catch (_) { return String(value); }
}

async function snapshot(progress) {
  if (typeof progress !== "function") return "";
  try { return describe(await progress()); } catch (error) { return `progress-error:${error && error.message}`; }
}

// Returns {ok:true, polls, ms} when `check` became truthy, or {ok:false, reason, progress, polls, ms}.
// Never throws because of the wait itself; errors thrown by `check` propagate.
async function waitOnce(check, options = {}) {
  const idleMs = options.idleMs ?? DEFAULTS.idleMs;
  const hardMs = Math.max(options.hardMs ?? DEFAULTS.hardMs, idleMs);
  const intervalMs = options.intervalMs ?? DEFAULTS.intervalMs;
  const now = options.now || Date.now;
  const sleep = options.sleep || ((ms) => new Promise((resolve) => setTimeout(resolve, ms)));
  const started = now();
  let lastChange = started;
  let last = await snapshot(options.progress);
  let polls = 0;
  for (;;) {
    if (await check()) return { ok: true, polls, ms: now() - started };
    polls++;
    const current = now();
    const next = await snapshot(options.progress);
    if (next !== last) { last = next; lastChange = current; }
    if (current - lastChange >= idleMs) return { ok: false, reason: "stalled", progress: last, polls, ms: current - started };
    if (current - started >= hardMs) return { ok: false, reason: "hard-limit", progress: last, polls, ms: current - started };
    await sleep(intervalMs);
  }
}

// Each failed temporal observation gets one fresh window with the identical limits.
async function waitOutcome(check, options = {}) {
  const {timingRetry}=require('./timing-retry.cjs');
  let last;
  const measurement=async()=>{last=await waitOnce(check,options);return last;};
  try {
    const result=await timingRetry({label:options.label||'wait for progress',measure:measurement,within:value=>value.ok,record:options.recordTiming});
    return {...result.value,attempts:result.attempts.map(a=>a.ms),measurements:result.attempts};
  } catch(error) {
    if(error.code!=='TIMING_LIMIT'||error.measurement!==measurement)throw error;
    return {...last,attempts:error.attempts.map(a=>a.ms),measurements:error.attempts};
  }
}

// Throwing flavour for plain callers.
async function waitFor(check, options = {}) {
  const outcome = await waitOutcome(check, options);
  if (outcome.ok) return outcome;
  const label = options.label || "condition";
  const detail = outcome.reason === "stalled" ? `no observable progress for ${options.idleMs ?? DEFAULTS.idleMs} ms` : `hard limit ${Math.max(options.hardMs ?? DEFAULTS.hardMs, options.idleMs ?? DEFAULTS.idleMs)} ms reached`;
  const error = new Error(`${label}: ${detail}; last progress ${outcome.progress}`);
  error.code = outcome.reason === "stalled" ? "WAIT_STALLED" : "WAIT_HARD_LIMIT";
  error.progress = outcome.progress;
  error.attempts = outcome.attempts;
  throw error;
}

// A generic progress probe for page-driving scripts (run inside the page via evaluate()).
const PAGE_PROGRESS_EXPRESSION = "(()=>({e:document.getElementsByTagName('*').length,x:(document.body&&document.body.textContent||'').length,r:document.readyState,res:performance.getEntriesByType('resource').length,u:location.pathname+location.search}))()";

// For CDP scripts: waitForPage(evaluate, expression, {idleMs, hardMs}) -> true when satisfied, false on stall/hard limit.
async function waitForPage(evaluate, expression, options = {}) {
  const outcome = await waitOutcome(() => evaluate(expression), {
    intervalMs: 50,
    ...options,
    progress: () => evaluate(PAGE_PROGRESS_EXPRESSION),
  });
  return outcome.ok;
}

module.exports = { waitFor, waitOutcome, waitForPage, PAGE_PROGRESS_EXPRESSION, DEFAULTS };
