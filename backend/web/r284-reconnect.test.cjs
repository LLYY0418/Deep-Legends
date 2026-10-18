'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs');
test('R284 P1 reconnect delay offers 5, 10, 15, 20 and 30 seconds',()=>{
 const source=fs.readFileSync(process.env.R284_SUITE_SOURCE || __dirname+'/suite.js','utf8');
 const start=source.indexOf('  const watchDefinitions = ['),end=source.indexOf('\n  const queuePolicies',start);
 const definitions=Function(source.slice(start,end)+';return watchDefinitions')();
 const definition=definitions.find(d=>d.key==='autoReconnect');
 const f=compile(source,['watchRuleControl','formatDelay'],{state:{},escapeHTML:String,selected:(a,b)=>a===b?' selected':''});
 const dom=new JSDOM(f.watchRuleControl(definition,{delayMs:15000}));try{
  const select=dom.window.document.querySelector('select[data-watch-number="autoReconnect.delayMs"]');assert(select,'reconnect delay needs the five allowed choices');
  assert.deepEqual([...select.options].map(o=>Number(o.value)),[5000,10000,15000,20000,30000]);assert.equal(select.value,'15000');
 }finally{dom.window.close()}
});
