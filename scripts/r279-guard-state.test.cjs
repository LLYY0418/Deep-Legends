"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require('../desktop/node_modules/jsdom');
const {compile}=require('../backend/web/r188-harness.cjs');
const current=fs.readFileSync(path.join(__dirname,'r272-browser-checks.cjs'),'utf8');
function sampling(source){const end=source.indexOf("results.push({r272:'score-placement-native-narrow'");const start=source.lastIndexOf('  await evaluate(',source.indexOf('  const score=await evaluate(',0));const atomic=source.indexOf('  const score=await evaluate(');return source.slice(start>source.indexOf("for(const rank of")&&source.slice(start,atomic).includes("row.style.width='35px'")?start:atomic,end);}
async function scenario(source,{breakFit=false}={}){
 const dom=new JSDOM('<main></main>',{runScripts:'outside-only'}),w=dom.window,list=w.document.querySelector('main');
 const markup='<article class="match-entry"><div class="match-tags-collection"><span data-match-tag data-match-tag-pinned>1st<i data-tooltip="评分：全场第1名"></i></span><span data-match-tag>五杀</span><button data-match-tags-more></button></div></article>';
 Object.defineProperty(w.HTMLElement.prototype,'clientWidth',{get(){return this.style.width==='35px'?35:168;}});w.HTMLElement.prototype.getBoundingClientRect=()=>({width:40});const reset=()=>{list.innerHTML=markup;list.querySelector('.match-tags-collection').checkVisibility=()=>true;};reset();
 const fit=compile(fs.readFileSync(path.join(__dirname,'../backend/web/gameplay.js'),'utf8'),['fitMatchTags'],{bindMatchTagsPopover:()=>({anchor:null})}).fitMatchTags;
 w.__r272List=list;w.__r266Game={fitMatchTags:breakFit?()=>{}:fit};
 const evaluate=async expression=>{const value=w.eval(expression);reset();return value;};
 try{await vm.runInNewContext('(async()=>{'+sampling(source)+'})()',{evaluate,rank:1,assert,JSON});}finally{w.close();}
}
test('R272 narrow mutation and score assertion sample the same node across async reconciliation',()=>scenario(current));
test('mutation: old separate CDP writes and reads fail when reconciliation replaces the row',async()=>{
 const start=current.indexOf('  const score=await evaluate('),first=current.indexOf('const first=',start);
 const setter="const row=__r272List.querySelector('.match-tags-collection');row.style.width='35px';row._tagFitWidth=undefined;__r266Game.fitMatchTags(__r272List);return true";
 const tail=current.slice(first).replace('window.__r279NarrowFailure=sample;row._tagObserver?.disconnect();host.remove();return sample','return sample');
 const previous=current.slice(0,start)+'  await evaluate('+JSON.stringify('(()=>{'+setter+'})()')+');\n  const score=await evaluate("(()=>{const row=__r272List.querySelector(\'.match-tags-collection\');'+tail;
 await assert.rejects(scenario(previous),/narrow fixture must be 35 logical pixels/);
});
test('mutation: stable narrow fixture with broken product fitting still fails original visible-tag assertion',()=>assert.rejects(scenario(current,{breakFit:true}),/2 !== 1/));


const r265=fs.readFileSync(path.join(__dirname,'r265-browser.cjs'),'utf8');
const savedWait=JSON.parse(r265.match(/await p\.until\(("new Promise[^\n]+")\);/)[1]);
const savedMaxAssertion=r265.match(/assert\.equal\(sizes\.max,"640px"\)/)[0];
async function savedSample({wait=true,brokenCSS=false}={}){
 let frames=0;const menu={className:'af-menu',isConnected:true,_afMenuState:{page:'conditions'}};
 const context={document:{querySelector:()=>menu},Promise,JSON,requestAnimationFrame:fn=>{if(++frames===2){menu.className='af-menu is-saved';menu._afMenuState.page='saved';}queueMicrotask(fn);}};
 if(wait)assert.equal(await vm.runInNewContext(savedWait,context),true);
 const sizes={max:menu._afMenuState.page==='saved'&&!brokenCSS?'640px':'1634px'};
 vm.runInNewContext(savedMaxAssertion,{assert,sizes});return frames;
}
test('saved page settles semantically before the original max-height assertion',async()=>assert((await savedSample())>=4));
test('mutation: sampling before saved page settles fails the original max-height assertion',()=>assert.rejects(savedSample({wait:false}),/1634px/));
test('mutation: wrong CSS after a settled saved page still fails the original max-height assertion',()=>assert.rejects(savedSample({brokenCSS:true}),/1634px/));

const r266=fs.readFileSync(path.join(__dirname,'r266-browser.cjs'),'utf8');
const matrixReady=r266.match(/await until\("document\.querySelectorAll\('\.match-summary'\)\.length===3"\);await settleTopbar\(\{evaluate\}\);/)[0];
const threeRows=r266.match(/assert\.equal\(rects\.length,3,'matrix requires all three modes'\);/)[0];
async function matrixRows({source=matrixReady,stuck=false}={}){
 let count=2,now=0;const {waitForPage}=require('./lib/wait-progress.cjs');
 const until=async expression=>{const ok=await waitForPage(async e=>e===expression?count===3:{rows:count},expression,{idleMs:100,hardMs:200,now:()=>now,sleep:async ms=>{now+=ms;if(!stuck)count=3;}});if(!ok)throw Error('matrix three rows ready: stalled');};
 await vm.runInNewContext('(async()=>{'+source+'const rects=Array.from({length:count()});'+threeRows+'})()',{until,settleTopbar:async()=>{},evaluate:()=>{},count:()=>count,assert,Array});
}
test('R266 geometry waits for the third actual card after virtual hydration',()=>matrixRows());
test('mutation: removing actual-row readiness must fail the original three-mode assertion',()=>assert.rejects(matrixRows({source:''}),/matrix requires all three modes/));
test('mutation: a permanently virtual third row must fail readiness explicitly',()=>assert.rejects(matrixRows({stuck:true}),/matrix three rows ready: stalled/));
