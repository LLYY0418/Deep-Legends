'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const read = name => fs.readFileSync(path.join(__dirname,name),'utf8');
const app = fs.readFileSync(process.env.R209_APP_SOURCE || path.join(__dirname,'app.js'),'utf8');
const flush = () => new Promise(resolve=>setImmediate(resolve));
function harness({silent=false}={}) {
 const dom = new JSDOM(read('index.html'),{url:'http://fixture/',runScripts:'outside-only',pretendToBeVisual:true});
 const w=dom.window,timers=new Map(),bodies=[],requests=[],observers=[];
 let now=1700000000000,serial=0,ready=false,complete=false;
 w.Date.now=()=>now;w.setTimeout=(fn,delay)=>{const id=++serial;timers.set(id,{fn,at:now+Number(delay||0)});return id};w.clearTimeout=id=>timers.delete(id);
 w.requestAnimationFrame=fn=>w.setTimeout(()=>fn(now),0);w.cancelAnimationFrame=w.clearTimeout;
 w.matchMedia=()=>({matches:false,addEventListener(){},removeEventListener(){}});w.ResizeObserver=class{observe(){}disconnect(){}};
 w.HTMLElement.prototype.scrollTo=function({top=0,left=0}={}){this.scrollTop=top;this.scrollLeft=left};
 w.HTMLDialogElement.prototype.showModal=function(){this.open=true};w.HTMLDialogElement.prototype.close=function(){this.open=false};
 w.HTMLMediaElement.prototype.pause=function(){};w.HTMLMediaElement.prototype.load=function(){};w.HTMLMediaElement.prototype.play=function(){return Promise.resolve()};
 Object.defineProperty(w.HTMLImageElement.prototype,'loading',{get(){return this.getAttribute('loading')||''},set(value){this.setAttribute('loading',value)},configurable:true});
 const rect={top:20,left:20,right:220,bottom:120,width:200,height:100};
 w.HTMLElement.prototype.getBoundingClientRect=function(){if(this.dataset.offscreen)return {...rect,top:10000,bottom:10100};return ['app-scroll','outer-scroll'].includes(this.id)?{top:0,left:0,bottom:600,right:1000,height:600,width:1000}:rect};
 w.HTMLElement.prototype.getClientRects=function(){return this.isConnected&&!this.closest('[hidden]')?[this.getBoundingClientRect()]:[]};
 class Observer {
  constructor(callback,options={}){this.callback=callback;this.options=options;this.targets=new Set();observers.push(this)}
  observe(image){this.targets.add(image);if(!silent)w.queueMicrotask(()=>{if(this.targets.has(image)&&image.isConnected)this.callback([{target:image,isIntersecting:!image.dataset.offscreen}],this)})}
  unobserve(image){this.targets.delete(image)}disconnect(){this.targets.clear()}
 }
 w.IntersectionObserver=Observer;
 w.EventSource=class{static CLOSED=2;addEventListener(){}close(){}};w.Headers=Headers;
 const items=Array.from({length:12},(_,i)=>({id:1000+i,name:`皮肤 ${i}`,championId:1,championName:'安妮',owned:true,splashPath:`/fixture/${i}.png`,rarity:'rare'}));
 const chromas=Array.from({length:3},(_,i)=>({id:2000+i,name:`炫彩 ${i}`,parentSkinId:1000,parentSkinName:'皮肤 0',championId:1,championName:'安妮',owned:true,tilePath:`/fixture/chroma-${i}.png`}));
 const response=payload=>Promise.resolve({ok:true,status:200,json:async()=>payload,text:async()=>JSON.stringify(payload)});
 w.fetch=(url,opts={})=>{
  requests.push(String(url));
  if(url==='/api/diagnostics/client'){bodies.push(JSON.parse(opts.body));return Promise.resolve({ok:true,status:204})}
  if(!ready)return new Promise(()=>{});
  if(String(url).startsWith('/api/skins'))return response({items});
  if(url==='/api/chromas')return response({items:chromas,capability:{ordinary:{available:true},prestige:{available:true}}});
  if(url==='/api/status')return response(w.r209.state.status);
  return new Promise(()=>{});
 };
 w.eval(read('runtime.js'));w.eval(read('image-queue.js'));
 const names='state,el,renderItems,loadSkins,activateSection,activateFavoritesPage,resetCollectionControls,openChromaDetails,closeSkinDialog,pumpCardImageQueue,deferCardImageSources,enqueueCardImageJob,cancelDeferredImages,checkCardImageHealth';
 w.eval(app.replace(/\}\)\(\);\s*$/,`window.r209={${names}};})();`));
 const api=w.r209;ready=true;
 Object.assign(api.state,{status:{connected:true,snapshotReady:true,identityReady:true,calculationOK:true,lastSync:'1'},loading:false,items:[],sort:'name'});
 function invariant(){const jobs=[...api.state.cardImageJobRegistry].filter(job=>job.active&&!job.done&&job.image.isConnected);assert.equal(api.state.activeCardImages,jobs.length);assert.equal(api.state.activePrestigeCardImages,jobs.filter(job=>job.remote).length);assert(api.state.activeCardImages<=8);assert(api.state.activePrestigeCardImages<=2);}
 async function settle(rounds=12){for(let i=0;i<rounds;i++){if(complete)for(const img of w.document.querySelectorAll('img[src]'))if(img.dataset.imageReady!=='true')img.dispatchEvent(new w.Event('load'));await flush();}}
 async function advance(ms){const target=now+ms;let guard=0;for(;;){await settle(2);const due=[...timers].filter(([,timer])=>timer.at<=target).sort((a,b)=>a[1].at-b[1].at)[0];if(!due)break;assert(++guard<10000,'timer runaway');timers.delete(due[0]);now=due[1].at;due[1].fn();}now=target;await settle();}
 async function enter(){api.activateSection('favorites');await advance(0);await settle();invariant();}
 return {w,dom,api,bodies,requests,observers,items,chromas,timers,advance,settle,enter,invariant,set complete(value){complete=value},close(){w.close();timers.clear()}};
}
test('R209 removed eight active DOM cards reconcile slots once and admit new screen',async()=>{
 const h=harness();try{
  await h.enter();assert.equal(h.api.state.activeCardImages,8);
  const old=[...h.api.state.cardImageJobRegistry];h.api.el.grid.replaceChildren();
  h.api.renderItems();await h.settle();
  h.invariant();assert.equal(h.api.state.activeCardImages,8);assert(old.every(job=>job.done&&job.cancelled));
  const events=h.bodies.filter(d=>d.event==='card_image_slot_reconciled');assert.equal(events.length,1);assert.equal(events[0].beforeCount,8);assert.equal(events[0].afterCount,0);
  assert(h.api.el.grid.querySelector('img[data-queued-src]'),'new screen began loading');
 }finally{h.close()}
});
test('R209 collection lifecycle preserves unchanged cards then releases every rebuilt or hidden job',async()=>{
 const h=harness();try{
  await h.enter();h.invariant();assert.equal(h.api.el.grid.querySelectorAll(".skin-card").length,12);const first=h.api.el.grid.firstChild;
  await h.api.loadSkins(true);await h.settle();assert.equal(h.api.el.grid.firstChild,first);assert(h.bodies.some(d=>d.event==='collection_render_client'&&d.reason==='unchanged-suppressed'));h.invariant();
  for(const view of ['all','owned','chromas']){h.api.resetCollectionControls(view);await h.api.loadSkins(true);await h.advance(0);h.invariant();}
  h.api.openChromaDetails(h.chromas[0],h.chromas);h.invariant();h.api.closeSkinDialog();h.invariant();
  h.api.activateSection('overview');await h.advance(0);h.invariant();assert.equal(h.api.state.activeCardImages,0);
  await h.enter();h.invariant();h.complete=true;await h.settle(40);h.invariant();assert.equal(h.api.state.activeCardImages,0);
  assert([...h.api.el.grid.querySelectorAll('.image-fallback')].every(node=>node.hidden||node.textContent==='暂无预览'));
  assert.equal(h.bodies.filter(d=>d.event==='card_image_slot_reconciled').length,0);
 }finally{h.close()}
});
test('R209 silent observer falls back for visible pending cards exactly after two seconds',async()=>{
 const h=harness({silent:true});try{
  await h.enter();assert.equal(h.api.state.activeCardImages,0);
  await h.advance(1999);assert.equal(h.api.state.activeCardImages,0);await h.advance(1);
  h.invariant();assert.equal(h.api.state.activeCardImages,8);
  const events=h.bodies.filter(d=>d.event==='card_image_observer_fallback');assert.equal(events.length,1);assert.equal(events[0].count,12);
  h.complete=true;await h.settle(40);assert.equal(h.api.state.activeCardImages,0);
 }finally{h.close()}
});
test('R209 duplicate enqueue cannot reserve the same queued task twice',async()=>{
 const h=harness();try{
  await h.enter();const waiting=[...h.api.state.cardImageJobRegistry].find(job=>job.queued);assert(waiting);
  for(let i=0;i<5;i++)h.api.enqueueCardImageJob(waiting);assert.equal(h.api.state.cardImageQueue.filter(job=>job===waiting).length,1);
  h.complete=true;await h.settle(40);h.invariant();assert.equal(h.api.state.activeCardImages,0);assert.equal(h.bodies.filter(d=>d.event==='card_image_slot_reconciled').length,0);
 }finally{h.close()}
});
test('R209 stalled waiting diagnostics carry complete counts and are limited to one per thirty seconds',async()=>{
 const h=harness();try{
  await h.enter();await h.advance(6000);let rows=h.bodies.filter(d=>d.event==='collection_card_image_state');assert.equal(rows.length,1);
  for(const key of ['activeCount','activeJobs','queued','pendingObserved','visiblePending','observerRootOk','oldestActiveAgeMs'])assert(key in rows[0],key);
  assert.equal(rows[0].activeCount,8);assert.equal(rows[0].activeJobs,8);assert.equal(rows[0].queued,4);assert.equal(rows[0].observerRootOk,true);assert.equal(rows[0].oldestActiveAgeMs,6000);
  await h.advance(28000);assert.equal(h.bodies.filter(d=>d.event==='collection_card_image_state').length,1);await h.advance(2000);assert.equal(h.bodies.filter(d=>d.event==='collection_card_image_state').length,2);
  h.api.state.section='overview';await h.advance(4000);assert.equal(h.bodies.filter(d=>d.event==='collection_card_image_state').length,2);
 }finally{h.close()}
});
test('R209 pending external cards use actual scroll root and hidden or offscreen cards never fall back',async()=>{
 const h=harness({silent:true});try{
  await h.enter();h.api.cancelDeferredImages(h.api.el.grid);h.api.el.grid.replaceChildren();
  const outer=h.w.document.createElement('div');outer.id='outer-scroll';outer.style.overflowY='auto';h.w.document.body.append(outer);
  const make=({hidden=false,offscreen=false,detached=false}={})=>{const card=h.w.document.createElement('div'),image=h.w.document.createElement('img'),fallback=h.w.document.createElement('span');card.append(image,fallback);card.hidden=hidden;if(offscreen)image.dataset.offscreen='true';h.api.deferCardImageSources(image,fallback,['/api/image?path=fixture']);if(!detached)outer.append(card);return image;};
  const visible=make(),hidden=make({hidden:true}),offscreen=make({offscreen:true}),detached=make({detached:true});await h.advance(2000);
  assert(h.api.state.cardImageJobs.get(visible).active);assert.equal(h.api.state.cardImageJobs.get(visible).observer.options.root,outer);
  assert.equal(hidden.dataset.cardImage,'pending');assert.equal(offscreen.dataset.cardImage,'pending');assert.equal(h.api.state.cardImageJobs.has(detached),false);
  h.invariant();const events=h.bodies.filter(d=>d.event==='card_image_observer_fallback');assert.equal(events.at(-1).count,1);
  await h.advance(4000);const state=h.bodies.find(d=>d.event==='collection_card_image_state');assert.equal(state.observerRootOk,false);
 }finally{h.close()}
});
test('R209 runtime transport bounds fields and excludes resource paths',()=>{
 const bodies=[],context={window:{},AbortController,setTimeout,clearTimeout,fetch:async(_url,opts)=>{bodies.push(JSON.parse(opts.body));return {status:204}},URL,TextEncoder,location:{origin:'http://fixture'}};
 vm.runInNewContext(read('runtime.js'),context);
 for(const [event,reason,fields] of [['collection_card_image_state','waiting',{activeCount:8,activeJobs:8,queued:9,pendingObserved:1,visiblePending:1,observerRootOk:false,oldestActiveAgeMs:6000}],['card_image_slot_reconciled','reconciled',{beforeCount:8,afterCount:0,beforeRemoteCount:2,afterRemoteCount:0}],['card_image_observer_fallback','visible-pending',{count:10000000}]])context.window.reportFlowDiagnostic(event,reason,{...fields,url:'secret-path',puuid:'secret-player'});
 assert.equal(bodies.length,3);assert.equal(bodies[0].observerRootOk,false);assert.equal(bodies[2].count,1000000);assert(!JSON.stringify(bodies).includes('secret'));
});
