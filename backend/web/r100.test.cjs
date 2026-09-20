'use strict';
const test=require('node:test'), assert=require('node:assert/strict'), fs=require('node:fs'), path=require('node:path'), vm=require('node:vm');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const gameplay=fs.readFileSync(process.env.R100_GAMEPLAY_SOURCE||path.join(__dirname,'gameplay.js'),'utf8');
const suite=fs.readFileSync(process.env.R95_SUITE_SOURCE||path.join(__dirname,'suite.js'),'utf8');
const flush=()=>new Promise(setImmediate);
function extract(src,name){let start=src.indexOf(`function ${name}(`);assert.ok(start>=0,name);if(src.slice(start-6,start)==='async ')start-=6;return src.slice(start,src.indexOf('\n  }',start)+4);}
// Reuse the existing real api/loadOverview and loadLive/renderLive harnesses;
// only their declarations, never their tests, are evaluated here.
function fixture(file,names){const text=fs.readFileSync(path.join(__dirname,file),'utf8');const end=file==='r91-addendum.test.cjs'?text.indexOf('for(const append of'):text.indexOf('\ntest(');const context={require,__dirname,process:{env:{...process.env,R91_GAMEPLAY_SOURCE:process.env.R100_GAMEPLAY_SOURCE,R91_ADDENDUM_GAMEPLAY_SOURCE:process.env.R100_GAMEPLAY_SOURCE}},module:{exports:{}},Headers,Response,ReadableStream,TextEncoder,TextDecoder,Uint8Array,AbortController,setImmediate,console};vm.runInNewContext(text.slice(0,end)+`\nmodule.exports={${names}};`,context);return context.module.exports;}
const streaming=fixture('r91-addendum.test.cjs','harness,payload,response');
const live=fixture('r91.test.cjs','harness,snapshot');
test('R100 progress updates ready profile fields but null and empty never wipe career',async()=>{
 let controller;const stream=new ReadableStream({start(c){controller=c;}});
 const h=streaming.harness([new Response(stream,{headers:{'Content-Type':'application/x-ndjson'}})]);
 try{
  const old={...streaming.payload(20),player:{...streaming.payload().player,summonerLevel:99,profileIconId:7},ranks:[{tier:'MASTER'}],masteries:[{championId:1}]};
  const tab={key:'fixture',region:'kr',data:old};const promise=h.loadOverview(tab,true);await flush();
  const send=async extra=>{controller.enqueue(new TextEncoder().encode(JSON.stringify({type:'progress',overview:{...streaming.payload(5),...extra}})+'\n'));await flush()};
  await send({player:{...old.player,summonerLevel:300},ranks:[{tier:'CHALLENGER'}],masteries:[{championId:2}]});
  assert.equal(tab.data.player.summonerLevel,300);assert.equal(tab.data.ranks[0].tier,'CHALLENGER');assert.equal(tab.data.masteries[0].championId,2);
  for(const empty of [null,undefined]) {await send({player:{...old.player,summonerLevel:empty,profileIconId:empty},ranks:empty,masteries:empty});assert.equal(tab.data.player.summonerLevel,300);assert.equal(tab.data.player.profileIconId,7);assert.equal(tab.data.ranks[0].tier,'CHALLENGER');assert.equal(tab.data.masteries[0].championId,2)}
  await send({profilePending:true,player:{...old.player,summonerLevel:0},ranks:[],masteries:[]});assert.equal(tab.data.player.summonerLevel,300);assert.equal(tab.data.ranks.length,1);assert.equal(tab.data.masteries.length,1);
  controller.enqueue(new TextEncoder().encode(JSON.stringify({type:'complete',overview:{...old,ranks:[],masteries:[]}})+'\n'));controller.close();await promise;
  assert.equal(tab.data.ranks.length,0);assert.equal(tab.data.masteries.length,0);
 }finally{h.close()}
});
test('R100 historical enrichment defers while foreground overview is in flight',async()=>{
 const start=gameplay.indexOf('async function handleOverviewIncremental('),end=gameplay.indexOf('  function renderPlayerTabs()',start);
 const tab={key:'a',loading:true,data:{player:{playerRef:'account'}}},calls=[];
 const context={state:{section:'overview'},overviewGroupForSection:()=> 'kr',activeTab:()=>tab,document:{getElementById:()=>null},loadOverview:async()=>assert.fail('history queried again'),rerenderTab:(...args)=>calls.push(args),overviewSectionForGroup:()=> 'overview',requestAnimationFrame:fn=>fn()};
 vm.runInNewContext(gameplay.slice(start,end),context);
 const detail={type:'historical-ranks',account:'account',historicalRanks:[{season:'2025',tier:'MASTER'}]};
 assert.equal(await context.handleOverviewIncremental(detail),false);assert.equal(calls.length,0);assert.equal(tab.pendingHistoricalRanks,detail);
 tab.loading=false;assert.equal(await context.handleOverviewIncremental(detail),true);assert.equal(calls.length,1);
 assert.match(extract(gameplay,'loadOverview'),/pendingHistoricalRanks[\s\S]*setTimeout\([\s\S]*handleOverviewIncremental/);
});

test('R100 image admission covers lazy DOM images and detached prefetch without same-URL retries',async()=>{
 const dom=new JSDOM('<main></main>',{url:'http://localhost',runScripts:'outside-only'}),w=dom.window;
 try{
  w.eval(fs.readFileSync(path.join(__dirname,'image-queue.js'),'utf8'));
  const imgs=Array.from({length:7},(_,i)=>{const img=w.document.createElement('img');img.setAttribute('data-queued-src','/api/image?'+i);w.document.body.append(img);return img});
  await flush();assert.equal(imgs.filter(i=>i.hasAttribute('src')).length,6);
  const prefetch=new w.Image();w.deepLegendsQueueImage(prefetch,'/api/image?prefetch');assert.equal(prefetch.hasAttribute('src'),false);
  imgs[0].dispatchEvent(new w.Event('error'));await flush();assert.equal(imgs[6].getAttribute('src'),'/api/image?6');
  imgs[1].dispatchEvent(new w.Event('load'));await flush();assert.equal(prefetch.getAttribute('src'),'/api/image?prefetch');
  prefetch.dispatchEvent(new w.Event('load'));for(const img of imgs.slice(2))img.dispatchEvent(new w.Event('load'));
  imgs[0].removeAttribute('src');imgs[0].setAttribute('data-queued-src','/api/image?0');await flush();assert.equal(imgs[0].hasAttribute('src'),false);
  imgs[1].removeAttribute('src');imgs[1].setAttribute('data-queued-src','/api/image?1');await flush();assert.equal(imgs[1].getAttribute('src'),'/api/image?1');imgs[1].dispatchEvent(new w.Event('load'));
  assert.doesNotMatch(suite,/data-data-queued-src/);
 }finally{w.close()}
});
