'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {releasePlan}=require('./release-plan.cjs');
const policy=require('./release-validation-policy.json');
test('versioned plan retains product three rounds and test infrastructure two on one SHA',()=>{
 assert.equal(releasePlan(['backend/web/app.js']).acceptance.rounds.length,3);
 const plan=releasePlan(['scripts/preflight.cjs','backend/r196_test.go']);assert.equal(plan.acceptance.rounds.length,2);assert(plan.acceptance.sameSha);
 for(const row of plan.acceptance.rounds.slice(1))assert.deepEqual(row,{event:'workflow_dispatch',inputs:{measure_windows_renderers:true}});
 assert.equal(plan.acceptance.actualWindowsGoAndRendererRequired,true);assert.equal(plan.diagnostic.releaseEvidence,false);assert.equal(plan.diagnostic.sweepRequired,false);
});
test('bad policy shortcut cannot call diagnostic output release evidence or omit Windows measurements',()=>{
 function verify(p){assert.equal(p.formal.productRounds,3);assert.equal(p.formal.testInfrastructureRounds,2);assert.equal(p.formal.measure_windows_renderers,true);assert.equal(p.diagnostic.releaseEvidence,false);}
 verify(policy);for(const mutate of [p=>p.formal.productRounds=1,p=>p.formal.measure_windows_renderers=false,p=>p.diagnostic.releaseEvidence=true]){const p=structuredClone(policy);mutate(p);assert.throws(()=>verify(p),{name:'AssertionError'});}
});

test('an empty upstream edit diff cannot authorize zero-round release validation',()=>{const p=releasePlan([]);assert.equal(p.classification,'no-change');assert.equal(p.acceptance.canAuthorizeRelease,false);});

const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),cp=require('node:child_process');
const planner=path.join(__dirname,'release-plan.cjs');
function withRepository(fn){
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r279-plan-'));
 const git=(...args)=>cp.execFileSync('git',args,{cwd:root,encoding:'utf8',stdio:'pipe'});
 const write=(file,text)=>{fs.mkdirSync(path.dirname(path.join(root,file)),{recursive:true});fs.writeFileSync(path.join(root,file),text);};
 try{
  git('init','-q');write('scripts/fixture.cjs','// baseline\n');write('.gitignore','backend/ignored.go\n');git('add','.');
  git('-c','user.name=R279 fixture','-c','user.email=r279@example.invalid','commit','-qm','baseline');
  fn({root,git,write,plan:()=>JSON.parse(cp.execFileSync(process.execPath,[planner,'--base','HEAD'],{cwd:root,encoding:'utf8'}))});
 }finally{fs.rmSync(root,{recursive:true,force:true});}
}
test('CLI counts untracked product additions alone and alongside tracked infrastructure edits',()=>{
 withRepository(({write,plan,git})=>{
  write('backend/new_product.go','package main\n');
  for(const mixed of [false,true]){
   if(mixed)write('scripts/fixture.cjs','// changed\n');
   const actual=plan();assert.equal(actual.classification,'product');assert.equal(actual.acceptance.rounds.length,3);
   assert.deepEqual(actual.productFiles,['backend/new_product.go']);
  }
  git('add','backend/new_product.go');
  assert.equal(plan().acceptance.rounds.length,3,'staging must not change classification');
 });
});
test('CLI preserves unusual paths and ignored files do not create product changes',()=>{
 withRepository(({write,plan,git})=>{
  write('backend/ignored.go','package main\n');
  write('scripts/new.test.cjs','// direct test\n');
  assert.equal(plan().classification,'test-infrastructure');
  const name=process.platform==='win32'?'backend/web/中文, new file.js':'backend/web/中文, new\nfile.js';write(name,'// product\n');
  assert.deepEqual(plan().productFiles,[name]);
  git('add','backend/web');assert.deepEqual(plan().productFiles,[name]);
 });
});
test('bad discovery that drops untracked files must fail the product-round oracle',()=>{
 withRepository(({root,write})=>{
  write('backend/new_product.go','package main\n');
  const Module=require('node:module'),source=fs.readFileSync(planner,'utf8');
  const mutant=source.replace("git(['ls-files', '--others', '--exclude-standard', '-z'])","[]");
  assert.notEqual(mutant,source,'untracked collection mutation must match');
  const m=new Module(planner,module);m.filename=planner;m.paths=module.paths;m._compile(mutant,planner);
  assert.throws(()=>assert.equal(m.exports.releasePlan(m.exports.candidateFiles('HEAD',root)).acceptance.rounds.length,3),{name:'AssertionError'});
 });
});
