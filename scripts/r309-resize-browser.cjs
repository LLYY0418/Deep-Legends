'use strict';
const {browser}=require('./r297-browser-harness.cjs'),fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const out=process.env.R309_RESIZE_REPORT_DIR||'/private/tmp/r309-resize-browser';
(async()=>{
 const report={};
 for(const mode of ['before','after']){
  const baseline=mode==='before'?fs.readFileSync(process.env.R309_BASELINE_APP,'utf8'):null;
  const b=await browser({out:path.join(out,mode),transform:(name,data)=>name==='app.js'&&baseline?Buffer.from(baseline):data});
  try{
   await b.call('Emulation.setDeviceMetricsOverride',{width:1900,height:1400,deviceScaleFactor:1,mobile:false});await b.call('Page.navigate',{url:b.origin+'/?demo'});await b.until('window.__r297?.activeTab()?.data?.matches?.length&&!__r297.activeTab().loading');
   await b.until("document.documentElement.dataset.uiScale==='1' && document.documentElement.dataset.sidebar==='expanded'");
   await b.evaluate(`window.__trace={zoomChanges:0,sidebarChanges:0,longtaskCount:0,longtaskMaxMs:0};window.__resizeObserver=new MutationObserver(rows=>{for(const row of rows){if(row.attributeName==='data-ui-scale')__trace.zoomChanges++;if(row.attributeName==='data-sidebar')__trace.sidebarChanges++;}});__resizeObserver.observe(document.documentElement,{attributes:true,attributeFilter:['data-ui-scale','data-sidebar']});window.__longtasks=new PerformanceObserver(list=>{for(const e of list.getEntries()){__trace.longtaskCount++;__trace.longtaskMaxMs=Math.max(__trace.longtaskMaxMs,e.duration)}});__longtasks.observe({type:'longtask'});`);
   for(let i=1;i<=30;i++){await b.call('Emulation.setDeviceMetricsOverride',{width:Math.round(1900+500*i/30),height:1400,deviceScaleFactor:1,mobile:false});await b.evaluate('new Promise(r=>requestAnimationFrame(r))');}
   const during=await b.evaluate('({...__trace})');await b.until("document.documentElement.dataset.uiScale==='1.25'");const settled=await b.evaluate('({...__trace,scale:document.documentElement.dataset.uiScale})');report[mode]={during,settled};
   if(mode==='after'){assert.equal(during.zoomChanges,0,'automatic zoom must stay frozen during all thirty frames');assert.equal(settled.zoomChanges,1,'release applies final zoom once');assert.equal(settled.scale,'1.25');}
   await b.evaluate('__resizeObserver.disconnect();__longtasks.disconnect()');
   if(mode==='after')for(const [width,sidebar]of [[800,'collapsed'],[1200,'expanded']]){await b.call('Emulation.setDeviceMetricsOverride',{width,height:1000,deviceScaleFactor:1,mobile:false});await b.until(`document.documentElement.dataset.sidebar===${JSON.stringify(sidebar)}`);assert.equal(await b.evaluate('document.documentElement.dataset.sidebar'),sidebar);}
   assert.deepEqual(b.errors,[]);
  }finally{await b.stop();}
 }
 assert(report.before.during.zoomChanges>report.after.during.zoomChanges);fs.writeFileSync(path.join(out,'resize-result.json'),JSON.stringify(report,null,2));console.log(JSON.stringify(report));
})().catch(error=>{console.error(error);process.exitCode=1});
