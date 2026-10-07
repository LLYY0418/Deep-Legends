'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const gameplay=fs.readFileSync(process.env.R244_GAMEPLAY_SOURCE||path.join(__dirname,'gameplay.js'),'utf8');
const app=fs.readFileSync(process.env.R244_APP_SOURCE||path.join(__dirname,'app.js'),'utf8');

test('R244 live header counts and KDA come from displayed rows even if stale stats disagree',()=>{
 const f=compile(gameplay,['renderLivePlayer'],{state:{settings:{}},champSelectEnemyPlaceholder:()=>false,maskedPlayerName:()=> '玩家',liveDisplayedChampionId:()=>61,liveHistoryStateOf:p=>p.historyState,liveHistorySettled:()=>true,rankTitle:()=>'',positionLabel:()=>'',renderLivePremadeTag:()=>'',iconFigure:()=>'',proBadgeAttributes:()=>'',renderProIdentityBadge:()=>'',number:String,percent:x=>`${x}%`,kda:x=>x.toFixed(2),escapeHTML});
 for(const n of [0,3,10]){
  const games=Array.from({length:n},(_,i)=>({win:i%2===0,kills:2,deaths:1,assists:3,createdAt:Date.now()-60*86400000}));
  const html=f.renderLivePlayer({recentGames:games,modeStats:{games:10,wins:5,losses:5,kda:2.27},recentRankedRecord:{games:10,wins:5,losses:5},historyState:n?'ok':'empty'},0);
  if(n){assert.match(html,new RegExp(`近 ${n} 局`));assert.match(html,/5\.00:1/);assert.match(html,new RegExp(`${Math.ceil(n/2)}胜 ${Math.floor(n/2)}负`))}
  else{assert.match(html,/本模式暂无战绩/);assert.doesNotMatch(html,/近 10 局|5胜/)}
 }
});
function overlay(){
 const dom=new JSDOM('<div id="overlay" hidden></div><main></main>');
 const state={},timers=new Map(),events=[];let seq=0;
 const el={startupLoading:dom.window.document.getElementById('overlay'),appFrame:dom.window.document.querySelector('main'),startupLoadingTitle:{},startupLoadingCopy:{}};
 const window={deepLegendsLicense:{isActive:()=>true},dispatchEvent:e=>events.push({type:e.type}),reportFlowDiagnostic:(event,reason,fields)=>events.push({event,reason,...fields})};
 const f=compile(app,['updateReadingOverlay','showReadingOverlay','hideReadingOverlay','snapshotRetryText'],{state,el,window,CustomEvent:dom.window.CustomEvent,STATUS_INTERVAL:3600000,setTimeout:(fn,delay)=>{timers.set(++seq,{fn,delay});return seq},clearTimeout:id=>timers.delete(id),renderNotice:()=>{}});
 return {state,timers,events,el,...f,close:()=>dom.window.close()};
}
test('R244 first self card hides immediately and 15-second no-card fallback remains retryable',()=>{
 const h=overlay();try{
  h.state.status={connected:false,clientDiscovery:'probe-failed'};h.updateReadingOverlay();
  h.state.status={connected:true,identityReady:true};h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,false);
  const timer=[...h.timers.values()].find(t=>t.delay===15000);assert(timer,'missing 15s fallback');timer.fn();
  assert.equal(h.el.startupLoading.hidden,true);assert(h.events.some(e=>e.type==='deep-legends:self-overview-timeout'));
  h.state.selfOverviewReady=true;h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,true);assert.equal(h.state.overlaySuppressed,false);
 }finally{h.close()}
 const ready=overlay();try{ready.state.status={connected:true,identityReady:true};ready.updateReadingOverlay();const start=performance.now();ready.state.selfOverviewReady=true;ready.updateReadingOverlay();assert.equal(ready.el.startupLoading.hidden,true);assert(performance.now()-start<=300);assert.equal(ready.events.find(e=>e.reason==='hide').hide_reason,'self-tab-ready');assert(![...ready.timers.values()].some(t=>t.delay===15000));}finally{ready.close()}
});
test('R244 connection-state starts status request within 100ms',()=>{
 let eventSource,calledAt;
 class EventSource {constructor(){eventSource=this}addEventListener(){}close(){}}
 const state={},window={EventSource,dispatchEvent:()=>{}};
 const f=compile(app,['setupLiveUpdates'],{state,window,EventSource,CustomEvent:class{},clearTimeout:()=>{},setTimeout:()=>1,refreshStatus:()=>{calledAt=performance.now()},queueLiveUpdateSlices:()=>assert.fail('connection-state debounced'),LIVE_UPDATE_STATE_SLICES:{'connection-state':['status']},resyncLiveState:()=>{},renderUpdateStatus:()=>{},updateUI:{}});
 f.setupLiveUpdates();const start=performance.now();eventSource.onmessage({data:'connection-state'});assert(calledAt!==undefined);assert(calledAt-start<=100);
});
test('R244 matches card emits readiness from the rendered list and only once for self',()=>{
 const dom=new JSDOM('<main><div class="match-list"></div></main>');try{
 const events=[],diagnostics=[],tab={current:true,data:{matches:[{gameId:1}]},overviewCardLoad:{startedAt:performance.now(),ready:new Set()}};
 const f=compile(gameplay,['reportOverviewCardReady'],{rankedQueueData:()=>({}),window:{dispatchEvent:e=>events.push(e.type),reportFlowDiagnostic:(_e,_r,v)=>diagnostics.push(v)},CustomEvent:dom.window.CustomEvent});
 f.reportOverviewCardReady(dom.window.document.querySelector('main'),tab);f.reportOverviewCardReady(dom.window.document.querySelector('main'),tab);
 assert.deepEqual(events,['deep-legends:self-tab-ready']);assert.equal(diagnostics[0].card,'matches');assert.equal(diagnostics.length,1);
 }finally{dom.window.close()}
});
test('R244 timeout produces the existing retry error flow for self',()=>{
 const tab={current:true},other={current:false},rendered=[];
 const f=compile(gameplay,['handleSelfOverviewTimeout'],{state:{tabs:[tab,other]},rerenderTab:t=>rendered.push(t)});
 f.handleSelfOverviewTimeout();assert.match(tab.error,/超时.*重试/);assert.equal(other.error,undefined);assert.deepEqual(rendered,[tab]);
});
