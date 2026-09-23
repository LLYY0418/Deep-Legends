"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const source = fs.readFileSync(process.env.R90_GAMEPLAY_SOURCE || path.join(__dirname,"gameplay.js"),"utf8");
function functionSource(script, name) {
  const marker = `function ${name}(`;
  const markerStart = script.indexOf(marker);
  const asyncStart = script.lastIndexOf("async ", markerStart);
  const start = asyncStart >= 0 && asyncStart + 6 === markerStart ? asyncStart : markerStart;
  assert.notEqual(start, -1, `missing ${name}`);
  const bodyStart = script.indexOf("{", start);
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < script.length; index += 1) {
    const char = script[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return script.slice(start, index + 1);
  }
  assert.fail(`unbalanced ${name}`);
}

function compile(names,deps={}) {deps={updateLiveLoadingVisibility(){},...deps};return Function(...Object.keys(deps),names.map(n=>functionSource(source,n)).join("\n")+`\nreturn {${names.join(",")}}`)(...Object.values(deps))}
function harness(data,section="live") {
 let serial=0;const timers=new Map();const calls=[];
 const state={section,beacon:{phase:data.phase},live:data,settings:{liveRefresh:true,liveInterval:60}};
 const document={hidden:false};
 const deps={state,document,connected:()=>true,liveGamePhase:p=>["ChampSelect","GameStart","InProgress","Reconnect"].includes(p),liveAugmentRecommendationSource:d=>d?.gameMode==="CHERRY"?"arena":"",escapeHTML:String,
 setTimeout:(fn,delay)=>{timers.set(++serial,{fn,delay});return serial},clearTimeout:id=>timers.delete(id),loadLive:(...args)=>calls.push(args),recordLiveRefresh:()=>{}};
 const functions=compile(["liveSnapshotComplete","syncLiveRetryBudget","liveAutoRefreshStopped","renderLiveRefreshStatus","normalizeLiveInterval","liveRefreshDelayMs","scheduleLiveRefresh","queueLiveEventRefresh"],deps);
 return {state,document,timers,calls,...functions,fire(){const [id,timer]=timers.entries().next().value;timers.delete(id);timer.fn();return timer.delay}};
}
const full = (phase="InProgress",arena=false)=>({phase,gameId:90,available:true,gameMode:arena?"CHERRY":"CLASSIC",arenaGrouped:arena,players:[{historyState:"ok"}]});

test("R90 complete snapshots never schedule another in-game timer",()=>{
 for (const phase of ["InProgress","Reconnect"]) for(const arena of [false,true]) {
  const h=harness(full(phase,arena));h.scheduleLiveRefresh();assert.equal(h.timers.size,0,"complete snapshot must stop timers");
  h.state.live.players[0].historyState="pending";h.scheduleLiveRefresh();assert.equal(h.timers.size,1);assert.equal(h.fire(),20000);
  h.state.live.players[0].historyState="ok";h.scheduleLiveRefresh();assert.equal(h.timers.size,0);assert.equal(h.calls.length,1);
 }
});
test("R90 completeness requires availability, roster, settled history and arena verdict",()=>{
 const h=harness(full());
 for(const d of [null,{}, {...full(),available:false},{...full(),players:[]},{...full(),players:[{historyState:"pending"}]},{...full("InProgress",true),arenaGrouped:false}]) assert.equal(h.liveSnapshotComplete(d),false);
 assert.equal(h.liveSnapshotComplete({...full("InProgress",true),arenaGrouped:false,arenaGroupingUnavailable:true}),true);
 for(const historyState of ["empty","unavailable"]) assert.equal(h.liveSnapshotComplete({...full(),players:[{historyState}]}),true);
});
test("R90 incomplete snapshots stop after eight retries with a visible refresh exit",()=>{
 const h=harness({...full("InProgress",true),arenaGrouped:false});
 h.syncLiveRetryBudget();h.state.liveRetryStartedAt=Date.now()-60001;
 for(let i=0;i<8;i++){h.scheduleLiveRefresh();assert.equal(h.timers.size,1);assert.equal(h.fire(),20000)}
 h.scheduleLiveRefresh();assert.equal(h.calls.length,8);assert.equal(h.timers.size,0);
 assert.match(h.renderLiveRefreshStatus(h.state.live),/已停止自动重试/);assert.doesNotMatch(h.renderLiveRefreshStatus(h.state.live),/data-live-refresh/);
 h.state.live.gameId=91;h.scheduleLiveRefresh();assert.equal(h.state.liveRetryAttempts,0);assert.equal(h.timers.size,1);
 h.state.beacon.phase="ChampSelect";h.state.live.phase="ChampSelect";h.scheduleLiveRefresh();assert.equal(h.fire(),3000);
});
test("R90 inactive tabs and repeated in-game SSE do not reload the roster; phase transitions do",()=>{
 const h=harness(full(),"champions");h.scheduleLiveRefresh();h.queueLiveEventRefresh("sse",false);assert.equal(h.timers.size,0);
 h.queueLiveEventRefresh("sse",true);assert.equal(h.timers.size,1);h.fire();assert.equal(h.calls.length,1);
 h.state.section="live";h.state.livePhaseRefreshQueued=false;h.state.liveRefreshQueued=false;
 h.queueLiveEventRefresh("sse",false);assert.equal(h.timers.size,0);
 h.state.live.arenaGrouped=false;h.state.live.gameMode="CHERRY";h.queueLiveEventRefresh("sse",false);assert.equal(h.timers.size,0,"same-phase events must not bypass bounded retry budget");
 h.document.hidden=true;h.queueLiveEventRefresh("sse",true);assert.equal(h.timers.size,1);assert.equal(h.state.livePhaseRefreshQueued,true);
 h.document.hidden=false;h.queueLiveEventRefresh();assert.equal(h.timers.size,1);
});
test("R90 stopped UI has an operable manual refresh and foreground wake checks phase",()=>{
 const h=harness(full());assert.match(h.renderLiveRefreshStatus(h.state.live),/对局中数据不再变化，已停止自动刷新/);
 assert.match(source, /nodes\.liveRefresh\.addEventListener\("click", \(\) => loadLive\(true, "manual"\)\)/);
 const visibility=source.slice(source.indexOf('document.addEventListener("visibilitychange", () => {\n    if (state.destroyed)'),source.indexOf('/* ---------- 新对局提示灯'));
 assert.doesNotMatch(visibility,/loadLive\(true/);assert.match(visibility,/scheduleBeaconPoll\(0\)/);
 assert.match(source,/const BEACON_FAST_POLL_MS = 1_000/);assert.match(source,/const BEACON_IDLE_POLL_MS = 12_000/);
});
test("R90 actual loader bypasses cache for manual refresh; R91 supersedes the former manual queue",async()=>{
 const state={beacon:{phase:"InProgress"},live:full(),settings:{},controllers:new Map(),liveRetryAttempts:8};const requests=[];const noop=()=>{};
 const deps={state,connected:()=>true,recordLiveRefresh:noop,normalizeLiveGameId:v=>Number(v)||0,liveSnapshotBehindPhase:()=>false,recordLiveObservation:noop,invalidateLiveForNewGame:noop,liveGamePhase:()=>true,renderLive:noop,api:async(url)=>{requests.push(url);return full()},shouldResetLiveGameScopedState:()=>false,resetLiveGameScopedState:noop,resetLivePositionOverrides:noop,resetRecommendationTabsOnChampionChange:noop,updateBeacon:noop,renderCapabilitySettings:noop,liveRecommendationsFor:()=>null,ensureLiveRecommendations:noop,ensureSpecialistRunes:noop,ensureProRunes:noop,syncLiveRetryBudget:noop,scheduleLiveRefresh:noop,queueLiveEventRefresh:noop};
 // R131 §2.1-1：loadLive 开头会记一次触发来源，把纯函数 liveRenderTriggerLabel 一并按真实实现编译。
 const {loadLive}=compile(["loadLive","liveRenderTriggerLabel"],deps);await loadLive(true,"manual");assert.deepEqual(requests,["/api/gameplay/live?refresh=1"]);assert.equal(state.liveRetryAttempts,0);
 const releases=[];let aborted=0;state.controllers.set("live",{abort:()=>aborted++});
 deps.api=async(url)=>{requests.push(url);await new Promise(resolve=>releases.push(resolve));return full()};
 const loader=compile(["loadLive","liveRenderTriggerLabel"],deps).loadLive;const pending=loader(false);const forced=loader(true,"manual");
 assert.equal(aborted,1);assert.equal(releases.length,2);assert.equal(requests.at(-1),"/api/gameplay/live?refresh=1");
 releases[0]();await pending;assert.equal(state.liveLoading,true);releases[1]();await forced;assert.equal(state.liveLoading,false);
});
