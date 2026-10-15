'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {waitFor}=require('./lib/wait-progress.cjs');
const source=fs.readFileSync(path.join(__dirname,'../backend/web/r192-harness.cjs'),'utf8');
// 19f883b3 wait-migration.test.cjs has no r192 row. Keep that file out of
// R278; exercise only the imported r192 function and its progress wiring here.
function untilWith(wait){
 const statement=source.slice(source.indexOf('async function until('),source.indexOf('async function boot('));
 return vm.runInNewContext(`${statement};until`,{waitFor:wait,Math,activity:{w:{document:{getElementsByTagName:()=>[1,2]}},fetches:3}});
}
test('R192 imported until keeps the mount label, original window floor and progress',async()=>{
 let options;
 const until=untilWith(async(check,o)=>{options=o;assert.equal(check(),true);});
 await until(()=>true,'full app did not mount',5000);
 assert.equal(options.label,'full app did not mount');
 assert.equal(options.idleMs,15000);assert.equal(options.hardMs,120000);
 assert.deepEqual(JSON.parse(JSON.stringify(options.progress())),{nodes:2,fetches:3});
 await until(()=>true,'long fixture',20000);assert.equal(options.idleMs,20000);
 assert.match(source,/activity\.w=w;activity\.fetches=0/);
 assert.match(source,/fetch:\(input,init\)=>\{activity\.fetches\+\+;/);
});
test('R192 slow progress succeeds, frozen mount and endless progress still fail',async()=>{
 let clock=0,ticks=0;
 const until=untilWith((check,o)=>waitFor(check,{...o,now:()=>clock,sleep:async(ms)=>{clock+=ms;ticks++;}}));
 await until(()=>ticks>1600,'slow mount',5000,()=>ticks);
 assert.ok(clock>15000);
 clock=0;ticks=0;
 await assert.rejects(until(()=>false,'full app did not mount',5000,()=>0),error=>error.code==='WAIT_STALLED'&&/full app did not mount/.test(error.message));
 assert.equal(clock,15000);
 clock=0;ticks=0;
 await assert.rejects(until(()=>false,'endless mount',5000,()=>ticks),error=>error.code==='WAIT_HARD_LIMIT');
 assert.equal(clock,120000);
});
