'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
test('R264 P3 browser errors use independent telemetry delivery and retain burst counters',async()=>{
 const timers=new Map(),listeners=new Map(),sent=[],window={addEventListener:(name,fn)=>listeners.set(name,fn)};
 vm.runInNewContext(fs.readFileSync(__dirname+'/runtime.js','utf8'),{window,fetch:async(_url,options)=>{sent.push(JSON.parse(options.body));return {status:204}},AbortController,setTimeout:(fn,delay)=>{const id={unref(){}};timers.set(id,{fn,delay});return id},clearTimeout:id=>timers.delete(id),URL,location:{origin:'http://fixture'}});
 for(let i=0;i<60;i++)window.reportFlowDiagnostic('local_request_client','failed',{endpoint:'status'});
 for(let i=0;i<40;i++)listeners.get('error')({error:{name:'TypeError'},filename:'http://fixture/gameplay.js',lineno:51,colno:3});
 listeners.get('unhandledrejection')({reason:{name:'ReferenceError'}});listeners.get('securitypolicyviolation')({effectiveDirective:'style-src-attr'});
 for(let i=0;i<8;i++){await new Promise(setImmediate);const entry=[...timers].find(([,timer])=>timer.delay===2000);if(entry){timers.delete(entry[0]);entry[1].fn();}}
 const errors=sent.filter(x=>x.event==='browser_error_client');assert(errors.length>=3);
 assert(errors.some(x=>x.counts.error===40));assert(errors.some(x=>x.reason==='unhandledrejection'));assert(errors.some(x=>x.reason==='csp'));
 assert(errors.some(x=>x.scriptName==='gameplay.js' && x.line===51));assert(!JSON.stringify(errors).includes('http://fixture'));
});
