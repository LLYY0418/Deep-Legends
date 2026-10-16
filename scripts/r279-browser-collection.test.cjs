'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),vm=require('node:vm');
const {collectIndependent,requirePassed}=require('./lib/collect-independent.cjs');
async function runFixture(codes){
 const out=fs.mkdtempSync(path.join(os.tmpdir(),'r279-guards-')),binary=path.join(out,'backend');fs.writeFileSync(binary,'');const ran=[];
 let source=fs.readFileSync(path.join(__dirname,'r276-browser-job.cjs'),'utf8').replace('main().catch(e=>{console.error(e);process.exitCode=1;});','module.exports=main;');
 const module={exports:{}};const census={summarize:()=>({connections:1,observations:[]}),classify:()=>({}),requireClean:()=>{}};
 const localRequire=name=>name==='./r276-browser-catalog.cjs'?{selectGuards:()=>[['r266-matrix','one.cjs','OUT'],['r269-geometry','two.cjs','OUT'],['last','last.cjs','OUT']]}:name==='./r276-offline.cjs'?{runOffline:async(_,args)=>{ran.push(args[0]);return {code:codes[args[0]]||0};}}:name==='./r276-api-census.cjs'?census:name==='node:child_process'?{spawnSync:()=>({status:0,stdout:'',stderr:''})}:name==='./lib/collect-independent.cjs'?{collectIndependent,requirePassed}:require(name);
 vm.runInNewContext(source,{require:localRequire,module,__dirname,process:{argv:['node','fixture','all',out],env:{R276_PUBLIC_BACKEND:binary},execPath:process.execPath,pid:process.pid},console,performance});
 try{await assert.rejects(module.exports(),/失败清单/);return {ran,rows:JSON.parse(fs.readFileSync(path.join(out,'browser-job.json'))),census:JSON.parse(fs.readFileSync(path.join(out,'api-census.json')))};}finally{fs.rmSync(out,{recursive:true,force:true});}
}
test('actual browser runner collects independent failures and accounts for dependency/census',async()=>{const result=await runFixture({'one.cjs':1,'last.cjs':1});assert.deepEqual(result.ran,['one.cjs','last.cjs']);assert.equal(result.rows.length,3);assert.equal(result.rows[1].status,'skipped-dependency-failed');assert.equal(result.rows[2].status,'failed');assert.equal(result.census.guards.length,3);});
test('actual browser runner remains red when the last guard passes',async()=>{const result=await runFixture({'one.cjs':1});assert.equal(result.rows[2].status,'passed');assert.equal(result.rows[0].status,'failed');});

test('Windows default fixture binary retains the executable suffix',()=>{
 const source=fs.readFileSync(path.join(__dirname,'r276-browser-job.cjs'),'utf8');
 const expression=/const binary=([^;]+);/.exec(source)[1];
 const choose=platform=>vm.runInNewContext(expression,{process:{env:{},platform},path,out:'/runner/fixture'});
 assert.equal(path.basename(choose('win32')),'r264-public.exe');
 assert.equal(path.basename(choose('darwin')),'r264-public');
 const mutant=expression.replace("process.platform==='win32'?'r264-public.exe':'r264-public'","'r264-public'");
 assert.throws(()=>assert.equal(path.basename(vm.runInNewContext(mutant,{process:{env:{},platform:'win32'},path,out:'/runner/fixture'})),'r264-public.exe'));
});
