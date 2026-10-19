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
  assert.equal(releasePlan([file]).acceptance.rounds.length,1,file);
 }
 for(const file of ['installer/application_launch_test.go','scripts/preflight.cjs','scripts/r279-closeout-gaps.test.cjs','README.md'])
  assert.equal(releasePlan([file]).acceptance.rounds.length,1,file);
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
 assert.throws(()=>assert.equal(m1.releasePlan(['installer/main_windows.go']).classification,'product'),{name:'AssertionError'});
 const m2=mutant('release-plan.cjs','releaseInputs.has(file)','false');
 assert.throws(()=>assert.equal(m2.releasePlan(['go.mod']).classification,'product'),{name:'AssertionError'});
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


// Build a real index after staging removals, rather than retaining deleted files
// in a synthetic current-file list.
function deletedRepository(action, check) {
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r279-deleted-index-'));
 const git=(...args)=>cp.execFileSync('git',args,{cwd:dir,encoding:'utf8'});
 try {
  fs.mkdirSync(path.join(dir,'scripts'));fs.mkdirSync(path.join(dir,'backend'));
  const files={
   'scripts/old.test.cjs':'// removed direct test\n',
   'scripts/dependent.test.cjs':"require('./old.test.cjs');\n",
   'scripts/helper.cjs':"module.exports=require('./leaf.cjs');\n",
   'scripts/leaf.cjs':'module.exports=1;\n',
   'scripts/transitive.test.cjs':"require('./helper.cjs');\n",
   'backend/old.go':'package main\nfunc DeletedFeature() {}\n',
   'backend/survivor_test.go':'package main\nfunc TestSurvivor(t *testing.T) { DeletedFeature() }\n'
  };
  for(const [file,text] of Object.entries(files))fs.writeFileSync(path.join(dir,file),text);
  git('init','-q');git('add','.');git('-c','user.name=R279 fixture','-c','user.email=r279@example.invalid','commit','-qm','base');
  action({dir,git});
  const api=require('./preflight.cjs'),changes=api.changedFiles('HEAD',dir);
  const index=api.loadIndex('HEAD',dir,changes),selection=select(index,changes);
  check({dir,git,index,selection,changes});
 } finally {fs.rmSync(dir,{recursive:true,force:true});}
}
function selectedAndDeferred(selection,index,full) {
 const costs=Object.fromEntries(selection.nodeTests.map(row=>[row.file,1]));
 const plan=planExecution(selection,{costs,full,goNamesByFile:index.goTestNames});
 return [...plan.nodeTests,...plan.deferred.nodeTests].map(row=>row.file).sort();
}
test('staged and unstaged deletion retain surviving dependencies using the real Git index',()=>{
 for(const staged of [false,true])deletedRepository(({dir,git})=>{
  if(staged)git('rm','-q','scripts/old.test.cjs','scripts/helper.cjs','scripts/leaf.cjs','backend/old.go');
  else for(const f of ['scripts/old.test.cjs','scripts/helper.cjs','scripts/leaf.cjs','backend/old.go'])fs.rmSync(path.join(dir,f));
 },({index,selection})=>{
  assert.deepEqual(selection.nodeTests.map(row=>row.file),['scripts/dependent.test.cjs','scripts/transitive.test.cjs']);
  assert.deepEqual(selection.goTestNames,['TestSurvivor']);
  assert(!index.nodeTests.includes('scripts/old.test.cjs'),'historical tests are analysis-only');
  assert(!selection.relatedScripts.some(row=>row.file==='scripts/helper.cjs'),'historical helpers are not execution targets');
  for(const full of [false,true])assert.deepEqual(selectedAndDeferred(selection,index,full),['scripts/dependent.test.cjs','scripts/transitive.test.cjs']);
 });
});
test('staged Unicode rename selects the new test and surviving references to its old path',()=>{
 deletedRepository(({git})=>git('mv','scripts/old.test.cjs','scripts/新.test.cjs'),({index,selection,changes})=>{
  assert(changes.some(row=>row.file==='scripts/old.test.cjs'&&row.status==='D'));
  assert.deepEqual(selection.nodeTests.map(row=>row.file),['scripts/dependent.test.cjs','scripts/新.test.cjs']);
  for(const full of [false,true])assert.deepEqual(selectedAndDeferred(selection,index,full),['scripts/dependent.test.cjs','scripts/新.test.cjs']);
 });
});
test('historical helper content preserves transitive edges and unreadable history fails explicitly',()=>{
 const files={'scripts/live.test.cjs':"require('./removed.cjs')"};
 const historical={'scripts/removed.cjs':"require('./deep.cjs')",'scripts/deep.cjs':"require('./removed.cjs')"};
 const options={files:Object.keys(files),read:f=>files[f],historicalFiles:Object.keys(historical),readHistorical:f=>historical[f],guards:[]};
 const index=buildIndex(options),s=select(index,[{file:'scripts/deep.cjs',status:'D'}]);
 assert.deepEqual(s.nodeTests.map(row=>row.file),['scripts/live.test.cjs']);
 assert.throws(()=>buildIndex({...options,readHistorical:()=>{throw Error('historical dependency unavailable');}}),/historical dependency unavailable/);
 const m=mutant('preflight.cjs','historicalFiles, readHistorical','historicalFiles: [], readHistorical');
 deletedRepository(({git})=>git('rm','-q','scripts/old.test.cjs'),({dir,changes})=>{
  const broken=select(m.loadIndex('HEAD',dir,changes),changes);
  assert.throws(()=>assert(broken.nodeTests.some(row=>row.file==='scripts/dependent.test.cjs')),{name:'AssertionError'});
 });
});
test('historical acceptance references are checked and cannot disappear behind the current proof',()=>{
 const {checkIndex}=require('./r279-evidence-index.cjs'),dir=fs.mkdtempSync(path.join(os.tmpdir(),'r279-history-proof-'));
 try{
  fs.writeFileSync(path.join(dir,'current.json'),'{}');
  const index={schema:2,items:{},currentAcceptance:{formalProof:'current.json'},previousAcceptance:{formalProof:'missing.json'}};
  assert.equal(checkIndex(index,{reportRoot:dir,repositoryRoot:dir}).ok,false);
  for(const bad of ['../missing.json','/missing.json','C:\\missing.json','missing.json; claimed pass']){
   const unsafe=structuredClone(index);unsafe.previousAcceptance.formalProof=bad;
   assert.throws(()=>checkIndex(unsafe,{reportRoot:dir,repositoryRoot:dir}),{name:'AssertionError'});
  }
  const m=mutant('r279-evidence-index.cjs',"['currentAcceptance','previousAcceptance']","['currentAcceptance']");
  assert.throws(()=>assert.equal(m.checkIndex(index,{reportRoot:dir,repositoryRoot:dir}).ok,false),{name:'AssertionError'});
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
});
test('only the proven test-only installer fixture is excluded from product rounds',()=>{
 const fixture='installer/internal/p11callbacktest/fixture_windows.go';
 assert.equal(releasePlan([fixture]).acceptance.rounds.length,1);assert.equal(releasePlan([fixture]).classification,'test-infrastructure');
 for(const production of ['installer/internal/webviewhost/host_windows.go','installer/internal/p11callbacktest/new_runtime.go'])assert.equal(releasePlan([production]).acceptance.rounds.length,1);
 const files=cp.execFileSync('git',['ls-files','installer'],{cwd:root,encoding:'utf8'}).split('\n').filter(f=>f.endsWith('.go')&&f!==fixture);
 const callers=files.filter(f=>fs.readFileSync(path.join(root,f),'utf8').includes('/internal/p11callbacktest"'));
 assert.deepEqual(callers,['installer/r277_callback_windows_test.go','installer/uninstall/r277_callback_windows_test.go']);
 assert(callers.every(f=>f.endsWith('_test.go')),'test-only exception must never become reachable from production');
 const m=mutant('release-plan.cjs','testOnlyInputs.has(file)','false');
 assert.throws(()=>assert.equal(m.releasePlan([fixture]).classification,'test-infrastructure'),{name:'AssertionError'});
});

test('discovered renderer tests retain script fixture CI entry points with exact workflow scope',()=>{
 const files={'backend/web/unit.test.cjs':"runFixture('pwsh',['scripts/unit.test.ps1'])",'desktop/desktop.test.cjs':"runFixture('pwsh',['scripts/desktop.test.ps1'])",'scripts/unit.test.ps1':'','scripts/desktop.test.ps1':'','.github/workflows/full.yml':'run: node scripts/test-renderers.cjs all','.github/workflows/desktop.yml':'run: node scripts/test-renderers.cjs desktop'};
 const index=buildIndex({files:Object.keys(files),read:f=>files[f],guards:[]});
 const selected=select(index,['scripts/unit.test.ps1','scripts/desktop.test.ps1']);
 assert.deepEqual(selected.scriptTests.find(t=>t.file==='scripts/unit.test.ps1').workflows,['full.yml']);
 assert.deepEqual(selected.scriptTests.find(t=>t.file==='scripts/desktop.test.ps1').workflows,['desktop.yml','full.yml']);
 const bad={...files,'.github/workflows/full.yml':'run: node scripts/other-runner.cjs all'};
 const broken=select(buildIndex({files:Object.keys(bad),read:f=>bad[f],guards:[]}),['scripts/unit.test.ps1']);
 assert.deepEqual(broken.scriptTests[0].workflows,[],'an unrelated runner must never authorize deferred coverage');
});
