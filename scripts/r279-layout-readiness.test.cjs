'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm');
const {settleTopbar}=require('./lib/topbar-settle.cjs');
test('transient overflow fails old sample but late settled legal geometry passes',async()=>{
 let frame=0;const bar={dataset:{topbarCompact:'0',topbarSettled:'false'}},search={scrollWidth:500,clientWidth:475};
 const context={document:{fonts:{ready:Promise.resolve()},documentElement:{dataset:{sidebar:'expanded'}},querySelector:s=>s==='.topbar'?bar:search},window:{innerWidth:900},performance:{now:()=>frame*100},Date,requestAnimationFrame:fn=>{if(++frame>=6){bar.dataset.topbarSettled='true';search.scrollWidth=475;}fn();}};
 assert.throws(()=>assert(search.scrollWidth<=search.clientWidth+1),'old transient sample must fail');
 const value=await settleTopbar({evaluate:async s=>vm.runInNewContext(s,context),now:()=>frame*100,noProgressMs:2000});assert.equal(value.settled,'true');assert.equal(value.scrollWidth,475);assert(frame>=8);
});
