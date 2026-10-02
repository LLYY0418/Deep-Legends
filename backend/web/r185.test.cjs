"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),vm=require("node:vm"),path=require("node:path");
const root=process.env.DEEP_LEGENDS_WEB_ROOT||__dirname;
const suite=fs.readFileSync(path.join(root,"suite.js"),"utf8"),runtime=fs.readFileSync(path.join(root,"runtime.js"),"utf8");
function fn(source,name){let start=source.indexOf(`function ${name}(`);assert(start>=0);if(source.slice(start-6,start)==="async ")start-=6;let params=source.indexOf("(",start),pd=0,body;for(let i=params;i<source.length;i++){if(source[i]==="(")pd++;if(source[i]===")"&&--pd===0){body=source.indexOf("{",i+1);break;}}let depth=0,quote="",escape=false;for(let i=body;i<source.length;i++){let c=source[i];if(quote){if(escape)escape=false;else if(c==="\\")escape=true;else if(c===quote)quote="";continue;}if(['"',"'","`"].includes(c)){quote=c;continue;}if(c==="{")depth++;if(c==="}"&&--depth===0)return source.slice(start,i+1);}throw Error(name);}
function helpers(names,deps){return Function(...Object.keys(deps),names.map(n=>fn(suite,n)).join("\n")+`\nreturn {${names.join(',')}}`)(...Object.values(deps));}
function perf(){const context={window:{},URL,location:{origin:"http://localhost"},TextEncoder};vm.runInNewContext(runtime,context);return context.window.deepLegendsPerformance;}
test("R185 SSE omits skins, preserves 2125 entries and does not redraw or renew catalog age",async()=>{
 const skins=Array.from({length:2125},(_,id)=>({id,name:`skin-${id}`}));const state={connected:true,facade:{connected:true,skins},facadeLoadedAt:12};const requested=[];let renders=0;
 const h=helpers(["facadeSkinSignature","facadeRenderSignature","loadFacade"],{state,api:async url=>{requested.push(url);return {connected:true};},renderFacade:()=>renders++,scheduleFacadeChallengeRetry(){},facadeBackgroundDirty(){return false;},hydrateFacadeDraft(){},roots:{facade:{}},errorCard(){throw Error("unexpected")}});
 await h.loadFacade(true,false,"sse");assert.deepEqual(requested,["/api/facade/state?trigger=sse&skins=0"]);assert.equal(state.facade.skins,skins);assert.equal(state.facade.skins.length,2125);assert.equal(renders,0);assert.equal(state.facadeLoadedAt,12);assert.equal(state.facadeSkinSignatures,undefined,"SSE must skip skin signatures entirely");
 const empty=helpers(["facadeSkinSignature","facadeRenderSignature","loadFacade"],{state,api:async()=>({connected:true,skins:[]}),renderFacade:()=>renders++,scheduleFacadeChallengeRetry(){},facadeBackgroundDirty(){return false;},hydrateFacadeDraft(){},roots:{facade:{}},errorCard(){throw Error("unexpected")}});
 await empty.loadFacade(true,false,"manual");assert.equal(state.facade.skins.length,0,"explicit empty skins must replace the previous catalog");assert.equal(renders,1);assert(state.facadeLoadedAt>12);
});
test("R185 skin signature is cached by list identity and explicit empty lists replace old lists",()=>{
 const state={},h=helpers(["facadeSkinSignature","facadeRenderSignature"],{state});const skins=[{id:1,name:"one",owned:true}];let traversals=0;const iterator=skins[Symbol.iterator].bind(skins);skins[Symbol.iterator]=()=>{traversals++;return iterator();};skins.map=()=>{throw Error("skins serialized by map")};
 const first=h.facadeRenderSignature({skins});for(let i=0;i<10;i++)assert.deepEqual(h.facadeRenderSignature({skins}),first);assert.equal(traversals,1);
 assert.notDeepEqual(h.facadeRenderSignature({skins:[{id:1,name:"two",owned:true}]}),first);assert.notDeepEqual(h.facadeRenderSignature({skins:[]}),first);
});
test("R185 longtasks are grouped by current section and tab with minute aggregation",()=>{
 let now=0,page={section:"overview",tab:"main"};const reports=[],timers=[];let observe;
 const m=perf().createRendererPerformance({now:()=>now,getPage:()=>page,snapshot:()=>({domNodes:99,imgCount:9}),report:(...x)=>reports.push(x),observe:fn=>{observe=fn;return {disconnect(){}};},interval:(fn,ms)=>{timers.push(ms);return fn;},clear(){}});
 observe([{startTime:0,duration:55},{startTime:20,duration:49}]);now=100;page={section:"suite",tab:"facade"};m.markPage();observe([{startTime:120,duration:80},{startTime:200,duration:100}]);now=60000;m.flush();const r=reports[0][2];assert.equal(r.longtaskCount,3);assert.equal(r.longtaskTotalMs,235);assert.equal(r.longtaskMaxMs,100);assert.equal(r.groups.length,2);assert.equal(r.groups[1].tab,"facade");assert.deepEqual(timers,[1000,60000]);m.dispose();
});
test("R185 idle heartbeat is once per five minutes and retains a 1.5 second timer lag",()=>{
 let now=0;const rows=[],cleared=[];const m=perf().createRendererPerformance({now:()=>now,getPage:()=>({section:"live",tab:"runes"}),report:(...x)=>rows.push(x),interval:(_fn,ms)=>ms,clear:id=>cleared.push(id)});
 now=1500;m.tick();for(now=60000;now<300000;now+=60000)m.flush();assert.equal(rows.length,0);m.flush();assert.equal(rows.length,1);assert.equal(rows[0][2].timerLagCount,1);assert.equal(rows[0][2].timerLagMaxMs,500);assert.equal(rows[0][2].windowMs,300000);m.dispose();assert.deepEqual(cleared,[1000,60000]);
});
test("R185 measured requests cover all main page categories and count real UTF8 and stream bytes",async()=>{
 const p=perf(),rows=[];for(const url of ["/api/gameplay/overview?player=private","/api/gameplay/live","/api/account","/api/facade/state?skins=0","/api/watch/rules","/api/rig/status","/api/claim/scan","/api/champselect/state","/api/champions/detail","/api/pro-players","/api/social/friends"]){
  const response=await p.measuredFetch(async()=>new Response('{"a":"中"}',{headers:{'Content-Type':'application/json'}}),(...x)=>rows.push(x),url);assert.equal((await response.json()).a,"中");
 }
 assert.deepEqual(rows.map(x=>x[2].endpoint),["overview","live","collection","facade","watch","rig","claim","champselect","champions","pro-players","friends"]);assert.equal(rows[0][2].responseBytes,Buffer.byteLength('{"a":"中"}'));assert(!JSON.stringify(rows).includes("private"));
 const data=new TextEncoder().encode('{"type":"complete"}\n');const streamed=await p.measuredFetch(async()=>new Response(data),(...x)=>rows.push(x),"/api/gameplay/overview");const reader=streamed.body.getReader();await reader.read();await reader.read();await reader.cancel();assert.equal(rows.at(-1)[2].responseBytes,data.length);
});
test("R185 metrics whitelist survives renderer transport and strips identity fields",async()=>{
 const rows=[],context={window:{},fetch:async(_url,init)=>{rows.push(JSON.parse(init.body));return {status:204};}};vm.runInNewContext(runtime,context);
 context.window.reportFlowDiagnostic("renderer_perf","aggregated",{longtaskCount:2,heapUsedMb:8,groups:[{section:"suite",tab:"facade",count:2,totalMs:100,maxMs:60,account:"private"},{section:"private",tab:"private"}],token:"private"});await new Promise(setImmediate);
 assert.equal(rows[0].longtaskCount,2);assert.equal(rows[0].groups.length,1);assert(!JSON.stringify(rows).includes("private"));
});
