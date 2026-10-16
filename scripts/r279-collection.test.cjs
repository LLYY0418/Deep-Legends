'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {collectIndependent,requirePassed}=require('./lib/collect-independent.cjs');
const bad=name=>({name,run:()=>{throw Error(name+' target failure');}});
test('two failures are both executed, persisted and remain red',async()=>{
 const saved=[];const rows=await collectIndependent([bad('one'),bad('two')],{onResult:(_,all)=>saved.push(structuredClone(all))});
 assert.equal(rows.length,2);assert.equal(saved.at(-1).length,2);assert(rows.every(r=>r.status==='failed'));assert.throws(()=>requirePassed(rows),/one.*target failure[\s\S]*two.*target failure/);
});
test('a later pass cannot erase a failure',async()=>{const rows=await collectIndependent([bad('one'),{name:'two',run:()=>{}}]);assert.equal(rows[1].status,'passed');assert.throws(()=>requirePassed(rows),/失败清单/);});
test('r266 dependency failure skips r269, independent guard still runs',async()=>{let ran=false;const rows=await collectIndependent([bad('r266-matrix'),{name:'r269-geometry',dependsOn:['r266-matrix'],run:()=>{throw Error('must not run');}},{name:'last',run:()=>{ran=true;}}]);assert.equal(rows[1].status,'skipped-dependency-failed');assert(ran);assert.throws(()=>requirePassed(rows));});
test('mutation: fail-fast and swallowing failures violate the collecting oracle',async()=>{
 const oracle=rows=>{assert.equal(rows.length,2);requirePassed(rows);};
 assert.throws(()=>oracle([{name:'one',status:'failed'}]),/1 !== 2/);
 assert.throws(()=>oracle([{name:'one',status:'failed'},{name:'two',status:'passed'}]),/失败清单/);
});
