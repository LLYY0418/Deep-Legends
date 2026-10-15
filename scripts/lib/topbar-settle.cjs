'use strict';
const assert=require('node:assert/strict');
// A repeated state, including a repeating cycle, is not new progress.
async function waitForPage({evaluate,sample,label='page condition',noProgressMs=20000,totalMs=120000,now=Date.now}){
 let value;const seen=new Set();let lastProgress='initial';
 const outcome=await require('./wait-progress.cjs').waitOutcome(async()=>{
  value=await evaluate(sample);const key=JSON.stringify(value.progress);
  if(!seen.has(key)){seen.add(key);lastProgress=key;}
  return value.done;
 },{label,idleMs:noProgressMs,hardMs:totalMs,now,sleep:()=>Promise.resolve(),progress:()=>lastProgress});
 if(outcome.ok)return value;
 const error=new Error(`${label}: ${outcome.reason==='stalled'?'no new progress':'total limit'} for ${outcome.ms}ms ${JSON.stringify(value?.snapshot??value?.progress)}`);
 error.pageWaitSnapshot=value?.snapshot;error.code=outcome.reason==='stalled'?'WAIT_STALLED':'WAIT_HARD_LIMIT';throw error;
}
function readTopbar(){
 const bar=document.querySelector('.topbar'),search=document.querySelector('.player-search');
 return {level:bar?.dataset.topbarCompact,settled:bar?.dataset.topbarSettled,sidebar:document.documentElement.dataset.sidebar,scrollWidth:search?.scrollWidth,clientWidth:search?.clientWidth,innerWidth:window.innerWidth,loadedMs:performance.now(),sampledAt:new Date().toISOString()};
}
async function waitViewport({evaluate,width,...options}){
 return waitForPage({evaluate,...options,label:'viewport width '+width,sample:`(async()=>{await new Promise(r=>requestAnimationFrame(r));const snapshot=(${readTopbar})();return {done:window.innerWidth===${JSON.stringify(width)},progress:window.innerWidth,snapshot};})()`});
}
async function settleTopbar({evaluate,sample,...options}){
 await evaluate('document.fonts.ready');
 try{
  const value=await waitForPage({evaluate,...options,label:'topbar stable across three frames',sample:`(async()=>{const read=${readTopbar};let previous;for(let frame=0;frame<3;frame++){await new Promise(r=>requestAnimationFrame(r));const snapshot=read(),progress=[snapshot.level,snapshot.sidebar,snapshot.settled],key=JSON.stringify(progress);if(snapshot.settled!=='true'||previous!==undefined&&key!==previous)return {done:false,progress,snapshot};previous=key;if(frame===2)return {done:true,progress,snapshot,sample:${sample ? `(${sample})` : 'null'}};}})()`});
  const geometry=value.snapshot;
  try{assert(geometry.scrollWidth<=geometry.clientWidth+1,'player-search must not overflow internally '+JSON.stringify(geometry));}catch(error){error.topbarSample=geometry;throw error;}
  return sample ? {...geometry,sample:value.sample} : geometry;
 }catch(error){error.topbarSample ||= error.pageWaitSnapshot;throw error;}
}
module.exports={settleTopbar,waitViewport,readTopbar,waitForPage};
