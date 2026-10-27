'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {harness,available}=require('./update-harness.cjs');
test('R313 apply title changes synchronously and phase timeout posts within 300ms with dialog phase fallback',async()=>{
 let applyBody;const h=harness(undefined,(url,options)=>{
  if(url==='/api/gameplay/phase')return new Promise((_r,reject)=>options.signal?.addEventListener('abort',()=>reject(Object.assign(Error('timeout'),{name:'AbortError'}))));
  if(url==='/api/update/apply'){applyBody=JSON.parse(options.body);return Promise.resolve({ok:true,json:async()=>({...available,state:'applying'})});}
 });try{h.probe.updateUI.phase='Lobby';h.probe.renderUpdateStatus({...available,state:'ready'});h.get('update-dialog').showModal();const start=Date.now();h.get('update-apply').click();assert.equal(h.get('update-dialog-title').textContent,'正在启动升级程序…');await new Promise(r=>setTimeout(r,340));assert(applyBody?.applyClick>=start);assert(applyBody.applyClick<=Date.now());assert.equal(h.requests.filter(([url])=>url==='/api/update/apply').length,1);assert.equal(h.get('update-dialog-title').textContent,'正在启动升级程序…');}finally{h.close();}
});
