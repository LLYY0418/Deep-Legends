'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R285_GAMEPLAY_SOURCE||__dirname+'/gameplay.js','utf8');
function fixture(group,stableLoad=false){
 const dom=new JSDOM('<div id="app-scroll"><main><header></header><div class="match-list"></div><div data-match-sentinel></div></main></div>'),w=dom.window,root=w.document.getElementById('app-scroll'),container=root.firstChild,tab={group,data:{matches:Array.from({length:20},(_,i)=>i),pagination:{hasMore:true}},overviewRequestToken:0},state={},frames=[],timers=[],reports=[];let cb,calls=0,liveContainer=container;
 class IO{constructor(fn){cb=fn}observe(){}disconnect(){}}
 const key=group==='pro'?'proMatchObserver':'matchObserver';
 const f=compile(source,['bindMatchSentinel'],{state,document:w.document,window:{IntersectionObserver:IO,requestAnimationFrame:fn=>frames.push(fn),reportFlowDiagnostic:(e,r,f)=>reports.push({reason:r,...f})},IntersectionObserver:IO,matchObserverKey:()=>key,overviewContainer:()=>liveContainer,loadOverview:async()=>{calls++;if(!stableLoad)tab.overviewRequestToken++;tab.data.matches.push(...Array.from({length:20},(_,i)=>calls*20+i));if(!stableLoad)f.bindMatchSentinel(container,tab);return true},clearTimeout:()=>{},setTimeout:fn=>(timers.push(fn),timers.length)});
 return {dom,w,root,container,tab,state,frames,reports,key,f,get calls(){return calls},visible:v=>cb([{isIntersecting:v}]),setContainer:v=>liveContainer=v,async down(){root.dispatchEvent(new w.WheelEvent('wheel',{deltaY:100}));while(frames.length)frames.shift()();while(timers.length)await timers.shift()();await Promise.resolve()}};
}
for(const group of ['pro','kr'])test('R285 P5 '+group+' five pages survive navigation and partial header/card bindings',async()=>{
 const x=fixture(group);try{
  x.f.bindMatchSentinel(x.container,x.tab);x.visible(true);await x.down();assert.equal(x.calls,1);
  x.state[x.key].disconnect();x.f.bindMatchSentinel(x.container,x.tab);x.visible(true);await x.down();assert.equal(x.calls,2);
  const observer=x.state[x.key];x.f.bindMatchSentinel(x.container.querySelector('header'),x.tab);assert.equal(x.state[x.key],observer,'header must not disconnect list observer');
  x.f.bindMatchSentinel(x.container.querySelector('.match-list'),x.tab);assert.equal(x.state[x.key],observer);
  x.visible(true);await x.down();x.visible(true);await x.down();assert.equal(x.calls,4);assert.equal(x.tab.data.matches.length,100);
 }finally{x.dom.window.close()}
});
test('R285 P5 guard diagnosis is bounded to one per binding and identifies each condition',async()=>{
 for(const guard of ['near','frame-pending','container']){
  const x=fixture('kr');try{x.f.bindMatchSentinel(x.container,x.tab);x.visible(guard!=='near');if(guard==='frame-pending')x.tab.appendFramePending=true;if(guard==='container')x.setContainer(null);await x.down();await x.down();const logs=x.reports.filter(r=>r.reason==='guard');assert.equal(logs.length,1);assert.equal(logs[0].blocked_reason,guard);assert.equal(x.calls,0)}finally{x.dom.window.close()}
 }
});

test('R285 P5 requested guard is reported after a completed request clears intent',async()=>{
 const x=fixture('kr',true);try{x.f.bindMatchSentinel(x.container,x.tab);x.visible(true);await x.down();x.visible(true);x.visible(true);const logs=x.reports.filter(r=>r.reason==='guard');assert.equal(logs.length,1);assert.equal(logs[0].blocked_reason,'requested')}finally{x.dom.window.close()}
});
