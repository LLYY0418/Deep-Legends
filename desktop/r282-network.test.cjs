'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),{EventEmitter}=require('node:events');
const {desktopCrashEvent,isNetworkService,recoverNetworkService}=require(process.env.R282_DESKTOP_LOG_SOURCE||'./desktop-log.cjs');
test('R282 P14 utility service identity survives privacy filtering',()=>{
 const e=desktopCrashEvent('child-process-gone',{type:'Utility',reason:'crashed',name:'Network Service',serviceName:'network.mojom.NetworkService',token:'private',url:'private'});
 assert.equal(e.name,'Network Service');assert.equal(e.serviceName,'network.mojom.NetworkService');assert(!JSON.stringify(e).includes('private'));assert(isNetworkService(e));
});
test('R282 P14 network service recovers requests within five seconds with reload fallback',async()=>{
 for(const fails of [0,1,2]){
  const contents=new EventEmitter();let requests=0,reloads=0;contents.isDestroyed=()=>false;contents.executeJavaScript=async script=>{assert(script.includes('/api/status'));return ++requests>fails;};contents.reload=()=>{reloads++;queueMicrotask(()=>contents.emit('did-finish-load'));};const records=[];
  assert(await recoverNetworkService({isDestroyed:()=>false,webContents:contents},e=>records.push(e)));assert.equal(reloads,fails===2?1:0);assert.equal(records[0].event,'network_service_recovered');assert(records[0].duration_ms<5000);
 }
});
