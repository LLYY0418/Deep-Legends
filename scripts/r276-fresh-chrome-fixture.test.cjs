'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {freshChromeFixture}=require(process.env.R276_FRESH_CHROME_SOURCE||'./r276-fresh-chrome-fixture.cjs');
test('R276 fixture restart closes prior scenarios and gives R266 a distinct initialized browser context',async()=>{
 let closed=false,disposed=false,initialized=false;const retiredHeapOwner='old-context',commands=[],closedSessions=new Set();
 const send=async(method,params)=>{
  commands.push(method);
  if(method==='Target.closeTarget'){assert.equal(params.targetId,'old');assert(closedSessions.has('old-session'));closed=true;return {success:true};}
  if(method==='Target.disposeBrowserContext'){assert(closed);assert.equal(params.browserContextId,'old-context');disposed=true;return {};}
  if(method==='Target.createBrowserContext'){assert(disposed,'old fixture context must be released');assert(closed,'preceding guard must finish before isolation');return {browserContextId:'fresh-context'};}
  if(method==='Target.createTarget'){assert.equal(params.browserContextId,'fresh-context','navigation/shared context retains the prior heap owner');assert.notEqual(params.browserContextId,retiredHeapOwner);return {targetId:'fresh-target'};}
  if(method==='Target.attachToTarget'){assert.equal(params.targetId,'fresh-target');assert.equal(params.flatten,true);return {sessionId:'fresh-session'};}
  throw Error(method);
 };
 const result=await freshChromeFixture({send,targetId:'old',sessionId:'old-session',browserContextId:'old-context',closedSessions,initialize:async value=>{assert.deepEqual(value,{targetId:'fresh-target',sessionId:'fresh-session',browserContextId:'fresh-context'});initialized=true;}});
 assert(initialized,'all original network, fixture and page initialization must execute');assert.deepEqual(result,{targetId:'fresh-target',sessionId:'fresh-session',browserContextId:'fresh-context'});
 assert.deepEqual(commands,['Target.closeTarget','Target.disposeBrowserContext','Target.createBrowserContext','Target.createTarget','Target.attachToTarget']);
});
test('R276 fixture startup errors remain fatal',async()=>{
 await assert.rejects(freshChromeFixture({send:async()=>{throw Error('Chrome context failed');},targetId:'old',sessionId:'old-session',closedSessions:new Set(),initialize(){}}),/Chrome context failed/);
});
test('R276 original 1000-row click gate retains every long task, including GC, at 100ms',()=>{
 const source=fs.readFileSync(path.join(__dirname,'r266-browser.cjs'),'utf8'),checks=fs.readFileSync(path.join(__dirname,'r269-browser-checks.cjs'),'utf8');
 assert(source.includes('process.env.R272_SCENE_ONLY?20:1000'));
 assert(source.includes("__r266Tasks.filter(t=>t.start>=__r266ClickStart)"));
 assert(source.includes("assert(clickTasks<=100,'click long tasks '+clickTasks)"));
 assert(!/collectGarbage|HeapProfiler\.collect|window\.gc\(/.test(source));
 assert(checks.includes('await restartFixture()'));
 assert(source.includes('freshChromeFixture({send,targetId,sessionId,browserContextId,closedSessions,initialize:async next=>'));
});
