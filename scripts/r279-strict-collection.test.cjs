'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
function fixture(env={}){
 const source=fs.readFileSync(require('node:path').join(__dirname,'r277-frontend-browser-checks.cjs'),'utf8');const a=source.indexOf(' let sampledTopbar;'),b=source.indexOf('\n try {',a);let restores=0;
 const c={Map,process:{env},evaluate:async expression=>{if(expression.includes('window.dispatchEvent='))restores++;return {};}};
 const result=vm.runInNewContext('(()=>{'+source.slice(a,b)+';return {collectCase,failures};})()',c);return {...result,restores:()=>restores};
}
test('real strict collector records both independent failures and restores hooks',async()=>{const f=fixture();await f.collectCase({name:'one'},()=>{throw Error('first original assertion');});await f.collectCase({name:'two'},()=>{throw Error('second original assertion');});assert.equal(f.failures.length,2);assert.equal(f.restores(),2);assert.match(f.failures[1].error,/second original/);});
test('cumulative append pages skip after a failed dependency, independent matrix still runs',async()=>{const f=fixture();let ran=false;await f.collectCase({name:'append-page',page:1},()=>{throw Error('page one assertion');});await f.collectCase({name:'append-page',page:2},()=>{throw Error('must skip');});await f.collectCase({name:'independent'},()=>{ran=true;});assert.equal(f.failures[1].status,'skipped-dependency-failed');assert(ran);});

test('a deliberate mutant stops at its original assertion without hiding its failure context',async()=>{const f=fixture({R277_MUTATION_PROBE:'1'}),original=Error('exact target assertion');await assert.rejects(f.collectCase({name:'skeleton'},()=>{throw original;}),error=>error===original);assert.equal(f.failures.length,1);assert.equal(original.failures[0].name,'skeleton');assert.equal(original.failures[0].error,'exact target assertion');});
