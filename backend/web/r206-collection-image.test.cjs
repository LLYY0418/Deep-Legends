'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile:compileSource,escapeHTML}=require('./r188-harness.cjs');
const compile=(source,names,deps)=>compileSource(source.replace(/^[ \t]*\/\/.*$/gm,''),names,deps);
const app=fs.readFileSync(path.join(__dirname,'app.js'),'utf8'),queue=fs.readFileSync(path.join(__dirname,'image-queue.js'),'utf8');
test('R206 first collection scan skeleton, failure after fifteen seconds and collection-only retry',async()=>{
 for(const status of [{syncing:true},{collectionRefreshElapsedMs:15000},{lastError:'fixture failure'},{snapshotRetryCount:1}]){
  const dom=new JSDOM('<div id="grid"></div><p id="meta"></p>');const doc=dom.window.document,requests=[];
  const state={status:{connected:true,snapshotReady:false,...status},section:'favorites',favoritesPage:'collection',qualitySelections:new Set(),renderGeneration:0};
  const el={grid:doc.querySelector('#grid'),listMeta:doc.querySelector('#meta'),refresh:{click(){throw Error('full refresh forbidden')}}};
  const f=compile(app,['renderItems','syncCollectionSession','collectionReadFailed','retryCollection'],{state,el,escapeHTML,cancelRenderFrames(){},cancelDeferredImages(){},stopHoverVideo(){},api:async(url)=>requests.push(url),showToast(){}});
  f.renderItems();assert.equal(el.listMeta.textContent,'正在读取收藏');
  if(status.syncing){assert.equal(el.grid.querySelectorAll('.skeleton').length,8);assert.equal(el.grid.querySelector('button'),null);assert(!/重试/.test(el.grid.textContent));}
  else{assert.match(el.grid.textContent,/收藏信息读取失败/);el.grid.querySelector('button').click();await Promise.resolve();assert.deepEqual(requests,['/api/refresh?source=collection_retry']);}
  dom.window.close();
 }
});
function fixture(source=queue){const dom=new JSDOM('<body></body>',{url:'http://fixture/',runScripts:'outside-only'});dom.window.eval(source);return dom;}
test('R206 twenty local images start within 100ms while five remote candidates are bounded to two',async()=>{
 const dom=fixture(),w=dom.window,local=[],remote=[];const started=Date.now();let peak=0;
 const add=url=>{const img=w.document.createElement('img');img.setAttribute('data-queued-src',url);w.document.body.append(img);return img;};
 for(let i=0;i<5;i++)remote.push(add(`/api/image?path=remote${i}&source=communitydragon`));
 for(let i=0;i<20;i++)local.push(add(`/api/image?path=local${i}`));
 for(let round=0;round<30;round++){await new Promise(setImmediate);peak=Math.max(peak,remote.filter(i=>i.hasAttribute('src')).length);for(const img of local)if(img.hasAttribute('src')&&!img.dataset.imageReady)img.dispatchEvent(new w.Event('load'));if(local.every(i=>i.dataset.imageReady))break;}
 assert(local.every(i=>i.dataset.imageReady));assert(Date.now()-started<100);assert(peak<=2);w.dispatchEvent(new w.Event('deep-legends:dispose'));w.close();
});
test('R206 local timeout is 4s and first error immediately queues a token-free remote candidate',async()=>{
 const dom=fixture(),w=dom.window,timers=new Map();let seq=0;
 w.setTimeout=(fn,ms)=>{timers.set(++seq,{fn,ms});return seq};w.clearTimeout=id=>timers.delete(id);
 const img=w.document.createElement('img');let exhausted=0;img.onerror=()=>exhausted++;w.document.body.append(img);
 w.deepLegendsQueueImage(img,'/api/image?path=%2Flol-game-data%2Fassets%2Fv1%2Fprofile-icons%2F1.jpg');
 assert.equal([...timers.values()][0].ms,4000);img.dispatchEvent(new w.Event('error'));await new Promise(setImmediate);
 assert.match(img.getAttribute('src'),/&source=communitydragon$/);assert.equal(exhausted,0);assert([...timers.values()].some(t=>t.ms===10000));
 w.dispatchEvent(new w.Event('deep-legends:dispose'));w.close();
});
test('R206 all single-letter tier icons share font size 13',()=>{
 for(const name of fs.readdirSync(path.join(__dirname,'tier-icons')).filter(n=>n.endsWith('.svg'))){const svg=fs.readFileSync(path.join(__dirname,'tier-icons',name),'utf8');if(/>[SABCD]<\/text>/.test(svg))assert.match(svg,/font-size="13"/,name);}
});
