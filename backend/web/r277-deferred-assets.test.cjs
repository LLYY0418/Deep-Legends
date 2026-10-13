'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),zlib=require('node:zlib');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const web=process.env.R277_DEFERRED_WEB || __dirname;
const read=name=>fs.readFileSync(path.join(web,name),'utf8');
function initialAssets(){const dom=new JSDOM(read('index.html'));try{return [...dom.window.document.querySelectorAll('script[src],link[rel=stylesheet]')].map(n=>n.getAttribute('src') || n.getAttribute('href'));}finally{dom.window.close();}}
test('initial production assets remain below original 420KiB gzip level-one budget',()=>{
 const assets=initialAssets();assert(!assets.includes('/match-details.js'),'rich module cannot enter the initial request set');
 const total=assets.reduce((n,file)=>n+zlib.gzipSync(read(file),{level:1}).length,0);assert(total<420*1024,`initial gzip=${total}`);
});
async function fixture(){
 const {bootDemoApp}=require('../../desktop/overview-render-helpers.cjs'),{waitForOverview}=require('../../desktop/renderer-wait.cjs');
 const setup=`(()=>{const append=document.head.append.bind(document.head);window.__r277DeferredRequests=[];document.head.append=(node)=>{if(node.tagName==='SCRIPT'&&node.getAttribute('src')==='/match-details.js'){__r277DeferredRequests.push(node);return append(node);}return append(node);};})();\n`;
 const f=bootDemoApp({deferredMatchDetails:true,gameplaySourceTransform:()=>setup+read('section-loader.js')+'\n'+read('gameplay.js')});await waitForOverview(f.window,17);
 const w=f.window,d=w.document;
 const flush=async()=>{await new Promise(r=>setImmediate(r));await new Promise(r=>setImmediate(r));};
 const finish=async(success=true)=>{const node=w.__r277DeferredRequests.at(-1);assert(node);if(success){w.eval(read('match-details.js'));node.onload();}else node.onerror();await flush();};
 const expand=index=>{const entries=[...d.querySelectorAll('.match-list > .match-entry')];const e=entries[index],id=e.dataset.matchId;e.querySelector('[data-toggle-match]').click();return id;};
 return {...f,w,d,flush,finish,expand};
}
test('cold overview requests no rich asset; first expansions single-flight, cached reopens and tabs render locally',async()=>{
 const f=await fixture();try{
  assert.equal(f.w.__r277DeferredRequests.length,0);const list=f.d.querySelector('.match-list'),other=list.children[2];
  const first=f.expand(0),second=f.expand(1);await f.flush();assert.equal(f.w.__r277DeferredRequests.length,1);assert(f.d.querySelector(`#match-detail-${first}`).textContent.includes('正在读取'));
  await f.finish();assert(f.d.querySelector(`#match-detail-${first} .match-table`));assert(f.d.querySelector(`#match-detail-${second} .match-table`));assert.equal(f.d.querySelector('.match-list'),list);assert.equal(list.children[2],other);
  f.d.querySelector(`[data-game-id="${first}"][data-match-detail="team"]`).click();assert(f.d.querySelector(`#match-detail-${first} .team-analysis-metrics`));
  f.d.querySelector(`[data-game-id="${first}"][data-match-detail="build"]`).click();assert(f.d.querySelector(`#match-detail-${first} .build-detail`));
  f.d.querySelector(`[data-match-id="${first}"] [data-toggle-match]`).click();f.d.querySelector(`[data-match-id="${first}"] [data-toggle-match]`).click();await f.flush();assert(f.d.querySelector(`#match-detail-${first} .match-table`));assert.equal(f.w.__r277DeferredRequests.length,1);assert.deepEqual(f.errors,[]);
 }finally{f.w.close();}
});
test('failed deferred script settles without a retry loop; explicit collapse/reopen retries and retains the list',async()=>{
 const f=await fixture();try{
  const list=f.d.querySelector('.match-list'),id=f.expand(0);await f.flush();await f.finish(false);
  assert(f.d.querySelector(`#match-detail-${id}`).textContent.includes('详情资源加载失败'));assert.equal(f.w.__r277DeferredRequests.length,1);
  await f.flush();assert.equal(f.w.__r277DeferredRequests.length,1,'no automatic retry loop');
  assert.equal(f.w.__r277DeferredRequests[0].isConnected,false,'failed script is released');
  f.d.querySelector(`[data-match-id="${id}"] [data-toggle-match]`).click();f.d.querySelector(`[data-match-id="${id}"] [data-toggle-match]`).click();await f.flush();assert.equal(f.w.__r277DeferredRequests.length,2);
  await f.finish();assert(f.d.querySelector(`#match-detail-${id} .match-table`));assert.equal(f.d.querySelector('.match-list'),list);assert.deepEqual(f.errors,[]);
 }finally{f.w.close();}
});
test('late deferred completion cannot reopen a collapsed card',async()=>{
 const f=await fixture();try{const id=f.expand(0);await f.flush();f.d.querySelector(`[data-match-id="${id}"] [data-toggle-match]`).click();await f.finish();assert.equal(f.d.querySelector(`#match-detail-${id}`),null);assert.equal(f.w.__r277DeferredRequests.length,1);assert.deepEqual(f.errors,[]);}finally{f.w.close();}
});
