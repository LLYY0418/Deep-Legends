'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path'),cp=require('node:child_process'),Module=require('node:module');
const {releasePlan}=require('./release-plan.cjs');
const {buildIndex,select}=require('./lib/preflight-select.cjs');
const {planExecution}=require('./lib/preflight-cost.cjs');
const root=path.resolve(__dirname,'..');
function fixture(){
 const files={
  'scripts/unit.test.py':'import unittest',
  'scripts/unit.test.ps1':'# Windows fixture',
  'scripts/windows-proof.py':'# pwsh scripts/unit.test.ps1',
  'scripts/next.test.cjs':"readFile('old.test.cjs')",
  'scripts/old.test.cjs':'',
  'backend/old_test.go':'package main\nfunc TestOld(t *testing.T) {}',
  'backend/next_test.go':'package main\nfunc TestNext(t *testing.T) {}',
  '.github/workflows/ci.yml':'run: python scripts/windows-proof.py'
 };
 return buildIndex({files:Object.keys(files),read:f=>files[f],guards:[]});
}
test('release classification includes installer runtime, dependencies and actual release build inputs',()=>{
 for(const file of ['installer/main_windows.go','installer/ui/index.html','installer/go.mod','go.mod','go.sum','build-desktop-windows.ps1','build-desktop.sh','scripts/build-stage.cjs','scripts/normalize-source-line-endings.cjs','scripts/make-release.cjs']){
  assert.equal(releasePlan([file]).classification,'product',file);
  assert.equal(releasePlan([file]).acceptance.rounds.length,3,file);
 }
 for(const file of ['installer/application_launch_test.go','scripts/preflight.cjs','scripts/r279-closeout-gaps.test.cjs','README.md'])
  assert.equal(releasePlan([file]).acceptance.rounds.length,2,file);
});
test('direct Python and PowerShell tests are selected, with transitive Windows CI entry points',()=>{
 const s=select(fixture(),['scripts/unit.test.py','scripts/unit.test.ps1']);
 assert.deepEqual(s.scriptTests.map(x=>x.file),['scripts/unit.test.ps1','scripts/unit.test.py']);
 assert(s.scriptTests.find(x=>x.file.endsWith('.ps1')).workflows.includes('ci.yml'));
 assert.deepEqual(s.unmapped,[]);
 const p=planExecution(s,{costs:{}});
 assert.deepEqual(p.scriptTests,s.scriptTests,'cost deferral cannot drop directly edited script tests');
});
test('script tests really invoke their interpreter; failure and unavailable platform never become pass',()=>{
 const {runScriptTests}=require('./preflight.cjs'),calls=[];
 const tests=select(fixture(),['scripts/unit.test.py','scripts/unit.test.ps1']).scriptTests;
 const runCommand=(label,command,args)=>{calls.push({command,args});return {label,ok:false,status:1};};
 const mac=runScriptTests(tests,{platform:'darwin',runCommand});
 assert.deepEqual(calls,[{command:'python3',args:['scripts/unit.test.py']}]);
 assert.equal(mac.find(x=>x.file.endsWith('.py')).ok,false);
 const pending=mac.find(x=>x.file.endsWith('.ps1'));
 assert.equal(pending.status,'deferred-to-Windows-CI');assert.notEqual(pending.ok,true);assert.deepEqual(pending.workflows,['ci.yml']);
 calls.length=0;const windows=runScriptTests(tests,{platform:'win32',runCommand});
 assert(calls.some(x=>x.command==='pwsh'&&x.args.includes('scripts/unit.test.ps1')));
 assert(calls.some(x=>x.command==='python'&&x.args.includes('scripts/unit.test.py')));
 assert(windows.every(x=>x.ok===false));
});
test('a platform-only test without a known CI entry stays blocked, not covered by an unrelated reference',()=>{
 const s=select(fixture(),['scripts/unknown.test.ps1']);
 const {runScriptTests}=require('./preflight.cjs');
 const result=runScriptTests(s.scriptTests,{platform:'darwin',runCommand:()=>assert.fail('must not execute Windows fixture on Mac')});
 assert.equal(result[0].status,'blocked-unmapped-platform');assert.equal(result[0].ok,false);
});
test('deleted Node, Go and script tests are not executed in either cost mode; surviving dependents remain',()=>{
 const s=select(fixture(),[{file:'scripts/old.test.cjs',status:'D'},{file:'backend/old_test.go',status:'D'},{file:'scripts/unit.test.py',status:'D'},{file:'backend/next_test.go',status:'M'}]);
 assert.deepEqual(s.nodeTests.map(x=>x.file),['scripts/next.test.cjs']);
 assert.deepEqual(s.goTestNames,['TestNext']);
 assert.deepEqual(s.scriptTests,[]);
 assert.equal(s.removedTests.length,3);
 for(const full of [false,true]){
  const p=planExecution(s,{costs:{'scripts/next.test.cjs':1},goNamesByFile:fixture().goTestNames,full});
  assert(!p.nodeTests.some(x=>x.file==='scripts/old.test.cjs'));
  assert(!p.goTestNames.includes('TestOld'));
 }
});
test('real Git deletion and Unicode rename keep the old test out and select the new test',()=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r279-gap-git-'));
 const git=(...args)=>cp.execFileSync('git',args,{cwd:dir,encoding:'utf8'});
 try{
  git('init','-q');fs.mkdirSync(path.join(dir,'scripts'));fs.writeFileSync(path.join(dir,'scripts/旧.test.cjs'),'// old\n');
  git('add','.');git('-c','user.name=R279 fixture','-c','user.email=r279@example.invalid','commit','-qm','base');
  fs.renameSync(path.join(dir,'scripts/旧.test.cjs'),path.join(dir,'scripts/新.test.cjs'));git('add','-A');
  const changes=require('./preflight.cjs').changedFiles('HEAD',dir);
  assert(changes.some(x=>x.file==='scripts/旧.test.cjs'&&x.status==='D'));
  assert(changes.some(x=>x.file==='scripts/新.test.cjs'&&x.status==='R'));
  const index=buildIndex({files:['scripts/新.test.cjs'],read:()=>'',guards:[]});
  assert.deepEqual(select(index,changes).nodeTests.map(x=>x.file),['scripts/新.test.cjs']);
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
});

function mutant(file,from,to){
 const filename=path.join(__dirname,file),source=fs.readFileSync(filename,'utf8'),changed=source.replace(from,to);
 assert.notEqual(changed,source,'mutation anchor must match: '+file);
 const mod=new Module(filename,module);mod.filename=filename;mod.paths=module.paths;mod._compile(changed,filename);return mod.exports;
}
test('source mutants cannot drop installer/build inputs, direct tests, or resurrect deleted tests',()=>{
 const m1=mutant('release-plan.cjs','backend|desktop|installer|relay','backend|desktop|relay');
 assert.throws(()=>assert.equal(m1.releasePlan(['installer/main_windows.go']).acceptance.rounds.length,3),{name:'AssertionError'});
 const m2=mutant('release-plan.cjs','releaseInputs.has(file)','false');
 assert.throws(()=>assert.equal(m2.releasePlan(['go.mod']).acceptance.rounds.length,3),{name:'AssertionError'});
 const m3=mutant('lib/preflight-select.cjs','if (SCRIPT_TEST_RE.test(file)) {','if (false) {');
 assert.throws(()=>assert.equal(m3.select(fixture(),['scripts/unit.test.py']).scriptTests.length,1),{name:'AssertionError'});
 const m4=mutant('lib/preflight-cost.cjs','scriptTests:selection.scriptTests||[]','scriptTests:[]');
 const s=select(fixture(),['scripts/unit.test.py']);
 assert.throws(()=>assert.equal(m4.planExecution(s,{costs:{}}).scriptTests.length,1),{name:'AssertionError'});
 const m5=mutant('lib/preflight-select.cjs','if (deleted.has(file)) return;','');
 assert.throws(()=>assert(!m5.select(fixture(),[{file:'scripts/old.test.cjs',status:'D'}]).nodeTests.some(r=>r.file==='scripts/old.test.cjs')),{name:'AssertionError'});
 const m6=mutant('preflight.cjs','ok: mapped ? null : false','ok: true');
 const ps=select(fixture(),['scripts/unit.test.ps1']).scriptTests;
 assert.throws(()=>assert.notEqual(m6.runScriptTests(ps,{platform:'darwin'})[0].ok,true),{name:'AssertionError'});
});
test('evidence bundle rejects missing, unsafe or mixed paths and a source mutant cannot hide missing evidence',()=>{
 const {checkIndex}=require('./r279-evidence-index.cjs'),dir=fs.mkdtempSync(path.join(os.tmpdir(),'r279-gap-evidence-'));
 try{
  fs.writeFileSync(path.join(dir,'proof.json'),'{}');fs.writeFileSync(path.join(dir,'source.cjs'),'');
  const index={schema:2,items:{P2:{evidence:['proof.json'],sources:['source.cjs']}}};
  assert.equal(checkIndex(index,{reportRoot:dir,repositoryRoot:dir}).ok,true);
  fs.rmSync(path.join(dir,'proof.json'));
  assert.equal(checkIndex(index,{reportRoot:dir,repositoryRoot:dir}).ok,false);
  for(const bad of ['../proof.json','proof.json; claimed pass','/proof.json','C:\\proof.json']){
   const x=structuredClone(index);x.items.P2.evidence=[bad];
   assert.throws(()=>checkIndex(x,{reportRoot:dir,repositoryRoot:dir}),{name:'AssertionError'});
  }
  const api=mutant('r279-evidence-index.cjs','ok:missing.length===0','ok:true');
  assert.throws(()=>assert.equal(api.checkIndex(index,{reportRoot:dir,repositoryRoot:dir}).ok,false),{name:'AssertionError'});
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
});
test('actual Python failures return nonzero rather than being reported as selected-only success',()=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r279-gap-python-'));
 try{
  const file=path.join(dir,'bad.test.py');fs.writeFileSync(file,'raise AssertionError("direct Python target")\n');
  const {runScriptTests}=require('./preflight.cjs');
  const result=runScriptTests([{file,runner:'python',workflows:[]}],{runCommand:(label,command,args)=>{
   const x=cp.spawnSync(command,args,{encoding:'utf8'});assert.match(x.stderr,/direct Python target/);return {label,ok:x.status===0,status:x.status};
  }});
  assert.equal(result[0].ok,false);assert.notEqual(result[0].status,0);
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
});
