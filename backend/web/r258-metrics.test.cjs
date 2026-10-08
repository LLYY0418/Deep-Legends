'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const runtime=fs.readFileSync(__dirname+'/runtime.js','utf8');
function resourceHarness(supported=true) {
 const base=1700000000000,reports=[],timers=[];let callback,now=base;
 class Observer {static supportedEntryTypes=['resource'];constructor(fn){callback=fn;}observe(options){assert.equal(options.type,'resource');}}
 const window={deepLegendsPerformance:{},reportFlowDiagnostic:(...args)=>reports.push(args),addEventListener(){}};
 const context={window,URL,location:{origin:'http://local'},performance:{timeOrigin:base},Date:{now:()=>now},setTimeout:(fn,ms)=>{timers.push({fn,ms});return timers.length},clearTimeout(){},...(supported?{PerformanceObserver:Observer}:{})};
 vm.runInNewContext(runtime.slice(runtime.indexOf('// R258: count completed local ResourceTiming')),context);
 return {base,reports,timers,start:window.deepLegendsPerformance.startColdRequestWindow,feed:entries=>callback({getEntries:()=>entries}),setNow:at=>now=at};
}
const entry=(start,wait,name='http://local/api/image?player=PRIVATE',extra={})=>({name,fetchStart:start,requestStart:start+wait,domainLookupStart:0,domainLookupEnd:0,connectStart:0,connectEnd:0,...extra});
test('R258 P8 browser first-three-second count excludes DNS/connect and allows late completed resources',()=>{
 const h=resourceHarness();h.feed([entry(20,101),entry(40,450,undefined,{domainLookupStart:40,domainLookupEnd:290,connectStart:290,connectEnd:490}),entry(3100,500),entry(60,200,'https://foreign/image'),entry(60,200,'http://local/api/diagnostics/client')]);
 h.start(h.base);assert.equal(h.reports.at(-1)[2].count,1);assert.equal(h.reports.at(-1)[2].resourceCount,2);assert.equal(h.reports.at(-1)[2].windowElapsed,false);
 h.setNow(h.base+3000);h.timers.find(t=>t.ms===3000).fn();assert.equal(h.reports.at(-1)[2].windowElapsed,true);
 h.feed([entry(100,100),entry(120,150)]);h.timers.find(t=>t.ms===250).fn();assert.equal(h.reports.at(-1)[2].count,2);assert.equal(h.reports.at(-1)[2].resourceCount,4);
 const n=h.timers.length;h.start(h.base);h.start(h.base-1);assert.equal(h.timers.length,n);assert(!JSON.stringify(h.reports).includes('PRIVATE'));
});
test('R258 P8 unsupported resource timing remains unavailable, never a fake zero',()=>{
 const h=resourceHarness(false);h.start(h.base);assert.equal(h.reports.at(-1)[2].count,-1);assert.equal(h.reports.at(-1)[2].timingAvailable,false);
});
test('R258 P8 exact champselect state observations bypass category sampling and strip URL/body identity',async()=>{
 const delivered=[],context={window:{},URL,location:{origin:'http://local'},TextEncoder,fetch:async(_url,init)=>{delivered.push(JSON.parse(init.body));return {status:204};}};
 vm.runInNewContext(runtime,context);const p=context.window.deepLegendsPerformance;
 for(let i=0;i<2;i++){const response=await p.measuredFetch(async()=>new Response('{"value":"中"}'),context.window.reportFlowDiagnostic,'/api/champselect/state?player=PRIVATE');await response.json();}
 await new Promise(setImmediate);
 const exact=delivered.filter(row=>row.event==='champselect_request_client');assert.equal(exact.length,2);assert.equal(exact[1].requestId,exact[0].requestId+1);assert.equal(exact[0].responseBytes,Buffer.byteLength('{"value":"中"}'));assert(!JSON.stringify(exact).includes('PRIVATE'));assert(!JSON.stringify(exact).includes('value'));
});
