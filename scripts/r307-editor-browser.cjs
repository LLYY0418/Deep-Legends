'use strict';
const {browser}=require('./r297-browser-harness.cjs'),fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const out=process.env.R307_REPORT_DIR||'/private/tmp/r307-editor-browser',fixture=require('../backend/testdata/r298-review-catalog.json'),catalog={...structuredClone(fixture.catalog),clientConnected:true,spellsStatus:'ready',itemsStatus:'ready'};
const setup=`window.deepLegendsRuntime={escapeHTML:v=>String(v??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;')};window.deepLegendsSections={register(){}};window.saved={enabled:true,runes:true,spells:true,items:true,version:1,champions:{1:{ranked:{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]}}}}};window.fetch=async(url,opts={})=>({ok:true,json:async()=>structuredClone(url==='/api/auto-config/settings'?saved:url==='/api/auto-config/catalog'?${JSON.stringify(catalog)}:url==='/api/champions/catalog'?{champions:Array.from({length:52},(_,i)=>({id:i+1,nameZh:String(i+1).padStart(2,'0')}))}:url==='/api/settings/summoner-keys'?{keys:['D','F']}:{})});`;
(async()=>{const b=await browser({out,transform:(name,data)=>name==='index.html'?Buffer.from(`<!doctype html><html data-theme="dark"><head><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/suite.css"><link rel="stylesheet" href="/auto-config.css"></head><body><main class="suite-panel" style="container-type:inline-size;container-name:suite-page"><div id="suite-config-root"></div></main><script>${setup}</script><script src="/auto-config.js"></script><script>deepLegendsAutoConfig.load()</script></body></html>`):data});
const frames=[];try{
 await b.call('Emulation.setDeviceMetricsOverride',{width:1920,height:1000,deviceScaleFactor:1,mobile:false});
 await b.call('Page.navigate',{url:b.origin+'/'});await b.until('document.querySelector(".ac-tree")');
 await b.evaluate(`document.querySelector('[data-champ="52"]').click();deepLegendsAutoConfig.load()`);await b.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 const location=await b.evaluate(`(()=>{const a=document.querySelector('.ac-champ.is-active').getBoundingClientRect(),r=document.querySelector('.ac-champ-list').getBoundingClientRect();return{visible:a.bottom>r.top&&a.top<r.bottom&&a.right>r.left&&a.left<r.right,scroll:document.querySelector('.ac-champ-list').scrollTop}})()`);assert(location.visible,JSON.stringify(location));
 await b.evaluate(`document.querySelector('[data-champ="1"]').click();`);
 await b.evaluate(`(async()=>{await document.fonts.ready;await Promise.all([...document.images].map(i=>i.decode().catch(()=>{})));await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()`);
 await b.evaluate(`window.__tree=document.querySelector('.ac-tree');window.__tasks=[];window.__observer=new PerformanceObserver(l=>__tasks.push(...l.getEntries().map(e=>({duration:e.duration,at:e.startTime}))));__observer.observe({type:'longtask'});window.__start=performance.now();`);
 for(let width=1920;width>=858;width=Math.max(858,width-20)){
 await b.call('Emulation.setDeviceMetricsOverride',{width,height:1000,deviceScaleFactor:1,mobile:false});
 frames.push(await b.evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>{const tree=document.querySelector('.ac-tree'),e=document.querySelector('.ac-editor');r({width:innerWidth,treeSame:tree===__tree,right:tree.getBoundingClientRect().right,editorRight:e.getBoundingClientRect().right,overflow:document.documentElement.scrollWidth>innerWidth})})))`));
 if([1920,1380,858].includes(width))await b.shot('resize-'+width);
 if(width===858)break;
 }
 const tasks=await b.evaluate(`__observer.disconnect();__tasks`);assert(frames.every(f=>f.treeSame&&!f.overflow&&f.right<=f.editorRight+1),JSON.stringify(frames.filter(f=>!f.treeSame||f.overflow||f.right>f.editorRight+1)));assert(tasks.every(t=>t.duration<=50),JSON.stringify(tasks));assert.deepEqual(b.errors,[]);
 fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({location,frames,tasks,errors:b.errors},null,2));
 console.log(JSON.stringify({frames:frames.length,longtasks:tasks.length,max:Math.max(0,...tasks.map(t=>t.duration))}));
}finally{await b.stop();}})().catch(e=>{console.error(e);process.exitCode=1});
