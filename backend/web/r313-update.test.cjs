'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {harness,available}=require('./update-harness.cjs');
test('R313 apply title changes synchronously and phase timeout posts within 300ms with dialog phase fallback',{timeout:2000},async()=>{
 let applyBody,resolveApply;const applied=new Promise(resolve=>{resolveApply=resolve});const h=harness(undefined,(url,options)=>{
  if(url==='/api/gameplay/phase')return new Promise((_r,reject)=>options.signal?.addEventListener('abort',()=>reject(Object.assign(Error('timeout'),{name:'AbortError'}))));
  if(url==='/api/update/apply'){applyBody=JSON.parse(options.body);resolveApply();return Promise.resolve({ok:true,json:async()=>({...available,state:'applying'})});}
 });const original=h.w.setTimeout,phaseTimers=[];h.w.setTimeout=function(fn,ms,...args){if(ms===300){phaseTimers.push({fn,ms,args});return 0}return original.call(this,fn,ms,...args)};try{h.probe.updateUI.phase='Lobby';h.probe.renderUpdateStatus({...available,state:'ready'});h.get('update-dialog').showModal();const start=Date.now();h.get('update-apply').click();assert.equal(h.get('update-dialog-title').textContent,'正在启动升级程序…');assert.equal(phaseTimers.length,1);assert.equal(phaseTimers[0].ms,300);assert.equal(applyBody,undefined);phaseTimers[0].fn(...phaseTimers[0].args);await applied;assert(applyBody?.applyClick>=start);assert(applyBody.applyClick<=Date.now());assert.equal(h.requests.filter(([url])=>url==='/api/update/apply').length,1);assert.equal(h.get('update-dialog-title').textContent,'正在启动升级程序…');}finally{h.w.setTimeout=original;h.close();}
});
