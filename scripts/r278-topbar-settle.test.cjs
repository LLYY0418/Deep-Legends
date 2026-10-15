'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm');
const {settleTopbar,waitViewport}=require(process.env.R278_SETTLE_SOURCE||'./lib/topbar-settle.cjs');
function page(mode){
 let frames=0;const bar={dataset:{topbarCompact:'0',topbarSettled:'true'}},search={scrollWidth:475,clientWidth:475},root={dataset:{sidebar:'expanded'}},window={innerWidth:1280};
 const context={document:{fonts:{ready:Promise.resolve()},documentElement:root,querySelector:s=>s==='.topbar'?bar:search},window,performance:{now:()=>frames*1000},Date,requestAnimationFrame:fn=>{
  frames++;if(mode==='unsettled')bar.dataset.topbarSettled='false';if(mode==='oscillating')bar.dataset.topbarCompact=String(frames%2);if(mode==='overflow')search.scrollWidth=499;if(mode==='resize'&&frames>=3)window.innerWidth=900;fn();
 }};
 return {evaluate:async expression=>vm.runInNewContext(expression,context),now:()=>frames*1000,noProgressMs:3000,frames:()=>frames};
}
test('R278 settle waits for target viewport and three unchanged ready frames',async()=>{
 const p=page('resize');await waitViewport({...p,width:900});assert.equal(p.frames(),3);const before=p.frames(),value=await settleTopbar(p);assert.equal(p.frames()-before,3);assert.equal(value.innerWidth,900);assert.equal(value.settled,'true');
});
for(const mode of ['unsettled','oscillating'])test('R278 settle bad state must FAIL: '+mode,async()=>{
 const p=page(mode);await assert.rejects(settleTopbar(p),error=>{assert.match(error.message,/topbar stable across three frames: no new progress/);assert(error.topbarSample.loadedMs>0);assert.equal(error.topbarSample.sidebar,'expanded');console.log(JSON.stringify({mutation:mode,result:'FAIL (expected)',message:error.message,sample:error.topbarSample}));return true;});
});
test('R278 stable internal overflow must FAIL at its original invariant with sampled geometry',async()=>{
 const p=page('overflow');await assert.rejects(settleTopbar(p),error=>{assert.match(error.message,/player-search must not overflow internally/);assert.deepEqual([error.topbarSample.scrollWidth,error.topbarSample.clientWidth],[499,475]);assert.equal(error.topbarSample.settled,'true');assert.equal(error.topbarSample.level,'0');assert(error.topbarSample.loadedMs>0);console.log(JSON.stringify({mutation:'stable-overflow',result:'FAIL (expected)',message:error.message,sample:error.topbarSample}));return true;});
});

test('R278 compact label is sampled in the same third stable frame without another CDP gap',async()=>{
 const p=page('resize');await waitViewport({...p,width:900});const value=await settleTopbar({...p,sample:"({level:document.querySelector('.topbar').dataset.topbarCompact,settled:document.querySelector('.topbar').dataset.topbarSettled,frame:performance.now()})"});assert.equal(value.sample.settled,'true');assert.equal(value.sample.level,value.level);assert.equal(value.sample.frame,value.loadedMs);
});
