'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm');
const {settleTopbar}=require('./lib/topbar-settle.cjs');
test('transient overflow fails old sample but late settled legal geometry passes',async()=>{
 let frame=0;const bar={dataset:{topbarCompact:'0',topbarSettled:'false'}},search={scrollWidth:500,clientWidth:475};
 const context={document:{fonts:{ready:Promise.resolve()},documentElement:{dataset:{sidebar:'expanded'}},querySelector:s=>s==='.topbar'?bar:search},window:{innerWidth:900},performance:{now:()=>frame*100},Date,requestAnimationFrame:fn=>{if(++frame>=6){bar.dataset.topbarSettled='true';search.scrollWidth=475;}fn();}};
 assert.throws(()=>assert(search.scrollWidth<=search.clientWidth+1),'old transient sample must fail');
 const value=await settleTopbar({evaluate:async s=>vm.runInNewContext(s,context),now:()=>frame*100,noProgressMs:2000});assert.equal(value.settled,'true');assert.equal(value.scrollWidth,475);assert(frame>=8);
});

const fs=require('node:fs'),path=require('node:path');
function verifyMutantSampling(source){
 for(const width of [820,1440]){
  const marker=" await call('Emulation.setDeviceMetricsOverride',{width:"+width+",height:1100,deviceScaleFactor:1,mobile:false});";
  const start=source.indexOf(marker);assert(start>=0);
  const beforeMutation=source.slice(start,source.indexOf(' await mutate(',start));
  assert(beforeMutation.includes('await waitViewport({evaluate,width:'+width+'})'),'mutant must wait for actual viewport '+width);
  assert(beforeMutation.includes('await settleTopbar({evaluate})'),'mutant must wait for settled topbar '+width);
 }
}
test('actual layout mutant sampling waits for viewport and stable geometry',()=>verifyMutantSampling(fs.readFileSync(path.join(__dirname,'../desktop/pro-players-layout.cjs'),'utf8')));
test('mutation: sampling before a viewport change settles must fail',()=>{
 const source=fs.readFileSync(path.join(__dirname,'../desktop/pro-players-layout.cjs'),'utf8');
 assert.throws(()=>verifyMutantSampling(source.replace('await waitViewport({evaluate,width:820});await settleTopbar({evaluate});','')),/mutant must wait for actual viewport 820/);
});
test('layout failure retains sidebar, viewport and every table geometry',()=>{
 const source=fs.readFileSync(path.join(__dirname,'../desktop/pro-players-layout.cjs'),'utf8');
 for(const field of ['captureFailure','failure.json','innerWidth','sidebar:','level:','settled:','loadedMs:','font:','scrollWidth:t.scrollWidth','parentClientWidth:t.parentElement.clientWidth'])assert(source.includes(field),field);
});
