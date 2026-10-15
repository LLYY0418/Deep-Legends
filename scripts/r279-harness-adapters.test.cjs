'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),Module=require('node:module');
const root=path.resolve(__dirname,'..');
// Execute each real source adapter, but parse its final harness instead of
// launching Chromium. The nested deferred adapter executes both real layers.
function compileAdapter(file,env={},mutate=s=>s){
 let compiled=0;
 const filename=path.join(root,'scripts',file),read=file=>{const source=fs.readFileSync(file,'utf8');return file.endsWith(path.sep+'r266-browser.cjs')?mutate(source):source;};
 const localFS={...fs,readFileSync:(file,encoding)=>encoding==='utf8'?read(String(file)):fs.readFileSync(file,encoding)};
 class CapturedModule{constructor(name){this.filename=name;this.paths=[];this.exports={};}_compile(source,name){
  if(source.includes("let source=fs.readFileSync(path.join(__dirname,'r266-browser.cjs')"))execute(source,name,this);
  else {new vm.Script(source,{filename:name});compiled++;}
 }}
 function execute(source,name,module={exports:{},paths:[]}){
  const req=id=>id==='node:module'?CapturedModule:id==='node:fs'?localFS:Module.createRequire(name)(id);
  const run=vm.runInNewContext(Module.wrap(source),{process:{env},console:{log(){}}},{filename:name});
  run(module.exports,req,module,name,path.dirname(name));
 }
 execute(read(filename),filename);assert.equal(compiled,1,'exactly one final harness must compile');
}
for(const [file,env]of [['r277-frontend-browser.cjs',{}],['r277-deferred-assets-browser.cjs',{}],['r277-deferred-assets-browser.cjs',{R277_DEFERRED_FAULT:'503'}],['r277-perf-browser.cjs',{}]])
 test('real adapter anchors and transformed syntax '+file+' '+JSON.stringify(env),()=>compileAdapter(file,env));
test('mutation: breaking the legacy declaration must fail the actual deferred 503 adapter',()=>{
 assert.throws(()=>compileAdapter('r277-deferred-assets-browser.cjs',{R277_DEFERRED_FAULT:'503'},s=>s.replace('let currentCase,clickEvidence;','let currentCase,clickEvidence,extra;')),/Unexpected browser harness anchor: let currentCase,clickEvidence;/);
});
