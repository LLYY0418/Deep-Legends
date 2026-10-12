'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../desktop/node_modules/jsdom');
const {install:installDialog}=require('../backend/web/filter-dialog-fixture.cjs');
const filters=require('../backend/web/history-filters.js');
const {settleFilterGeometry}=require(process.env.R276_GEOMETRY_MODULE||'./r276-filter-geometry-fixture.cjs');
function fixture(){
 const rows=Array.from({length:14},()=>'<article class="match-entry"><div class="match-main"></div><div class="match-tags-collection"><span data-match-tag>2nd</span><button data-match-tags-more>more</button></div></article>').join('');
 const dom=new JSDOM('<main><div id="filters"></div><div class="match-list">'+rows+'</div></main>',{url:'http://fixture',runScripts:'outside-only'});installDialog(dom);
 const document=dom.window.document,root=document.querySelector('main'),observers=[],frames=[];
 dom.window.requestAnimationFrame=callback=>{frames.push(callback);return frames.length;};Object.defineProperty(document,'fonts',{value:{ready:Promise.resolve()}});
 const source=fs.readFileSync(path.join(__dirname,'../backend/web/gameplay.js'),'utf8');const begin=source.indexOf('  function fitMatchTags('),end=source.indexOf('\n  function matchTagsBackgroundAllowed',begin);assert(begin>=0&&end>begin);
 const ResizeObserver=class{constructor(callback){this.callback=callback;observers.push(this);}observe(node){this.node=node;}};
 const fit=new Function('bindMatchTagsPopover','ResizeObserver',source.slice(begin,end)+'\nreturn fitMatchTags;')(()=>({anchor:null,close(){}}),ResizeObserver);
 for(const node of document.querySelectorAll('.match-tags-collection'))Object.defineProperty(node,'clientWidth',{get:()=>document.body.style.overflow==='hidden'?136:120});
 for(const tag of document.querySelectorAll('[data-match-tag]'))tag.getBoundingClientRect=()=>({width:30});
 fit(root); // Real production fit/ResizeObserver callback, without source edits.
 const tab={advancedConditions:{},data:{matches:Array.from({length:1000},(_,gameId)=>({gameId,result:'win',participants:[{championId:103,championName:'阿狸'}]})),pagination:{hasMore:false}}};let changes=0;
 const ctx={rows:()=>tab.data.matches,version:()=>tab.data.matches,subject:m=>m.participants[0],tags:()=>[],cursor:()=>1000,storage:dom.window.localStorage,container:()=>root,icon:()=>'',change(){changes++;document.querySelector('.match-list').replaceChildren(document.createElement('article'));},prepare(){},error(){}};
 root.querySelector('#filters').innerHTML=filters.render(tab,()=>ctx);filters.bind(root,tab,()=>ctx);
 root.querySelector('[data-af-open]').click();document.querySelector('[data-af-category="hero"]').click();assert.equal(document.body.style.overflow,'hidden');
 let mutations=0,details=[];
 const arm=()=>new dom.window.MutationObserver(records=>{mutations+=records.length;details.push(...records.map(r=>r.target.getAttribute('data-match-tags-more')!==null?'match-tags-more':r.target.className));}).observe(root.querySelector('.match-list'),{childList:true,subtree:true});
 const frame=async()=>{const callbacks=frames.splice(0);callbacks.forEach(fn=>fn());for(const observer of observers)observer.callback([]);await Promise.resolve();};
 const evaluate=expression=>dom.window.eval(expression);
 return {dom,root,arm,frame,evaluate,get mutations(){return mutations},get details(){return details},get changes(){return changes},get pendingFrame(){return frames.length>0}};
}
test('old observer includes fourteen real tag-fit writes from modal scroll-lock',async t=>{
 const h=fixture();t.after(()=>h.dom.window.close());h.arm();await h.frame();h.dom.window.document.querySelector('[data-af-option="103"]').click();await Promise.resolve();assert.equal(h.mutations,14);assert(h.details.every(n=>n==='match-tags-more'));assert.equal(h.changes,0);
});
test('settled geometry keeps the original zero draft mutations and applies only on close',async t=>{
 const h=fixture();t.after(()=>h.dom.window.close());let done=false;
 const pending=settleFilterGeometry(h.evaluate).then(()=>{done=true;});
 for(let step=0;step<6&&!done;step++){for(let turn=0;turn<10;turn++)await Promise.resolve();if(!done&&h.pendingFrame)await h.frame();}
 await pending;h.arm();await h.frame();h.dom.window.document.querySelector('[data-af-option="103"]').click();await h.frame();assert.equal(h.mutations,0,'draft click must not inherit pending tag-fit mutations');assert.equal(h.changes,0);
 h.dom.window.document.querySelector('[data-af-close]').click();await Promise.resolve();assert.equal(h.changes,1);assert.equal(h.mutations,1);
});
test('real browser measures only after setup, retaining both original assertions',()=>{
 const source=fs.readFileSync(path.join(__dirname,'r266-browser.cjs'),'utf8');const settled=source.indexOf('await settleFilterGeometry(evaluate);');assert(settled>=0&&settled<source.indexOf('window.__r266ListMutations=0;'));assert.match(source,/assert\.equal\(await evaluate\('__r266ListMutations'\),0\)/);assert.match(source,/assert\(clickTasks<=100,/);
});
