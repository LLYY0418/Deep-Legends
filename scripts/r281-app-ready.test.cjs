"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),{createRequire}=require('node:module');
const {JSDOM}=require('../desktop/node_modules/jsdom');
const filename=path.join(__dirname,'lib/app-ready.cjs'),original=fs.readFileSync(filename,'utf8');
function load(source){const module={exports:{}};vm.runInNewContext(source,{module,exports:module.exports,require:createRequire(filename)},{filename});return module.exports;}
async function scenario(source=original,{completeAt=4,listenerAt=2,activate=true,inertUntil=0,appAt=0,multiple=false,evaluationError=false}={}){
 const helper=load(source),dom=new JSDOM('<div id="app-frame"></div><main><section id="overview-panel" role="tabpanel"></section><section id="pro-players-panel" role="tabpanel" hidden></section></main>',{runScripts:'outside-only'}),w=dom.window;
 let now=0,dispatchedAt,activeAt;Object.defineProperty(w.document,'readyState',{get:()=>now>=completeAt*50?'complete':'loading',configurable:true});
 const advance=()=>{const app=w.document.getElementById('app-frame');if(now<appAt*50){if(app)app.remove();}else if(!app){const frame=w.document.createElement('div');frame.id='app-frame';w.document.body.prepend(frame);}w.document.getElementById('app-frame')?.toggleAttribute('inert',now<inertUntil*50);if(now>=listenerAt*50)w.deepLegendsToast=()=>{};if(activate&&dispatchedAt!==undefined&&now>=dispatchedAt+100){w.document.getElementById('overview-panel').hidden=!multiple;w.document.getElementById('pro-players-panel').hidden=false;activeAt=now;}};
 w.addEventListener('deep-legends:navigate',()=>{dispatchedAt=now;});
 const evaluate=async expression=>{if(evaluationError)throw Error("CDP fixture error");advance();return vm.runInContext(expression,dom.getInternalVMContext());};
 try{await helper.navigateAppSection({evaluate,section:'pro-players',idleMs:1000,hardMs:5000,intervalMs:50,now:()=>now,sleep:async ms=>{now+=ms;advance();}});return {dispatchedAt,activeAt,now};}finally{w.close();}
}
test('app readiness waits for complete DOM, registered app listener, then the actual active panel',async()=>{
 const result=await scenario();assert.equal(result.dispatchedAt,200);assert.equal(result.activeAt,300);assert.equal(result.now,300);
 const lateListener=await scenario(original,{completeAt:1,listenerAt:4});assert.equal(lateListener.dispatchedAt,200);
});
test('an unhandled section event fails at the named active-section wait instead of silently resolving',async()=>{
 await assert.rejects(scenario(original,{activate:false}),error=>error.code==='WAIT_STALLED'&&/pro-players active section ready/.test(error.message));
});
test('bad readiness mutants fail the same dispatch timing assertions',async()=>{
 const noComplete=original.replace("document.readyState==='complete'&&",'');assert.notEqual(noComplete,original);
 const early=await scenario(noComplete);assert.throws(()=>assert.equal(early.dispatchedAt,200),{code:'ERR_ASSERTION'});
 const noListener=original.replace('&&appReady&&','&&');assert.notEqual(noListener,original);
 const missingListener=await scenario(noListener,{completeAt:1,listenerAt:4});assert.throws(()=>assert.equal(missingListener.dispatchedAt,200),{code:'ERR_ASSERTION'});
});

test('missing frame and inert frame both postpone dispatch; two active panels never count as ready',async()=>{
 assert.equal((await scenario(original,{appAt:6})).dispatchedAt,300);
 assert.equal((await scenario(original,{inertUntil:7})).dispatchedAt,350);
 await assert.rejects(scenario(original,{multiple:true}),error=>error.code==='WAIT_STALLED'&&error.snapshot.activeSection.length===2);
});
test('CDP evaluation errors propagate instead of being converted into ready',async()=>{
 await assert.rejects(scenario(original,{evaluationError:true}),/CDP fixture error/);
});
