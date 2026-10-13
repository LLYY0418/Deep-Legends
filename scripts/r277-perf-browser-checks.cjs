'use strict';
const assert=require('node:assert/strict'),crypto=require('node:crypto');
module.exports=async({evaluate,results})=>{
 const phase=process.env.R277_PERF_PHASE;assert(['before','after'].includes(phase));
 console.log("R277-perf setup",phase);
 assert.equal(await evaluate("__r266Game.appendOverviewMatches.toString().includes('boundMatchDOM')"),phase==='after','exact old/new renderer asset required');
 const sizes=await evaluate("(()=>{const t=document.querySelector('#overview-content')._overviewViewTab;return {matches:t.data.matches.length,first_bytes:JSON.stringify(t.data.matches[0]).length,modes:t.data.matches.map(m=>m.modeGroup)}})()");console.log("R277-perf seed",JSON.stringify(sizes));
 const fixture=await evaluate("(()=>{const t=document.querySelector('#overview-content')._overviewViewTab;window.__r277PerfTab=t;window.__r277PerfData=structuredClone(t.data);window.__r277PerfMatches=Array.from({length:400},(_,i)=>({...structuredClone(t.data.matches[i%t.data.matches.length]),gameId:810000+i}));t.current=false;t.group='kr';t.region='kr';__r266Game.state.activeGroup='kr';__r266Game.state.activeTabs.kr=t.key;t.matchFilter='all';t.advancedConditions={};t.data=structuredClone(__r277PerfData);t.data.matches=__r277PerfMatches.slice(0,20);t.data.pagination={count:20,hasMore:true};const raw=JSON.stringify(__r277PerfMatches),bytes=new TextEncoder().encode(raw);return crypto.subtle.digest('SHA-256',bytes).then(hash=>({sha256:[...new Uint8Array(hash)].map(b=>b.toString(16).padStart(2,'0')).join(''),bytes:bytes.length}))})()");
 console.log("R277-perf fixture_bytes",fixture.bytes);
 console.log("R277-perf initial_render_start",phase);
 await evaluate("__r266Game.renderOverviewBody(document.querySelector('#overview-content'),__r277PerfTab);true");
 console.log("R277-perf initial_render_done",phase);
 const hash=fixture.sha256;
 await evaluate("new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
 await evaluate("window.__r277PerfTasks=[];window.__r277PerfObserver=new PerformanceObserver(l=>__r277PerfTasks.push(...l.getEntries().map(e=>({start:e.startTime,duration:e.duration}))));__r277PerfObserver.observe({type:'longtask'});window.__r277PerfStart=performance.now();true");
 const pages=[];
 for(let page=1;page<20;page++){
  console.log("R277-perf page_start",phase,page);
  const sample=await evaluate(`(()=>{const t=__r277PerfTab,added=__r277PerfMatches.slice(${page}*20,(${page}+1)*20),list=document.querySelector('#overview-content .match-list'),started=performance.now();t.data.matches=__r277PerfMatches.slice(0,(${page}+1)*20);t.data.pagination.count=t.data.matches.length;__r266Game.appendOverviewMatches(t,added);return {page:${page},matches:t.data.matches.length,synchronous_ms:performance.now()-started,list_replaced:list!==document.querySelector('#overview-content .match-list'),nodes:document.querySelectorAll('*').length,entries:document.querySelectorAll('#overview-content .match-list > .match-entry').length}})()`);
  await evaluate("new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>setTimeout(r,30))))");
  assert.equal(sample.matches,(page+1)*20);assert.equal(sample.entries,sample.matches,'actual rendered rows, not data-only append');assert.equal(sample.list_replaced,false);if(phase==='after')assert(sample.nodes<=25000,'after DOM cap');pages.push(sample);results.push({measurement:"page-progress",phase,...sample});console.log("R277-perf page_done",phase,page,sample.nodes,sample.synchronous_ms);
 }
 const timing=await evaluate("(()=>{__r277PerfObserver.takeRecords().forEach(e=>__r277PerfTasks.push({start:e.startTime,duration:e.duration}));__r277PerfObserver.disconnect();const tasks=__r277PerfTasks.filter(e=>e.start>=__r277PerfStart);return {elapsed_ms:performance.now()-__r277PerfStart,long_task_count:tasks.length,long_task_total_ms:tasks.reduce((s,e)=>s+e.duration,0),long_task_max_ms:Math.max(0,...tasks.map(e=>e.duration)),long_tasks:tasks}})()");
 results.push({P:9,measurement:'400-match-before-after-record-only',phase,fixture_sha256:hash,width:1440,height:900,theme:'dark',match_count:400,pages,...timing});
};
