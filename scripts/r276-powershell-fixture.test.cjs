'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {runFixture}=require(process.env.R276_PWSH_FIXTURE_SOURCE||'./r276-powershell-fixture.cjs');
test('R276 offline PowerShell fixture keeps its total 15-second deadline and exact diagnostics',()=>{
 const result={status:0,stdout:'PASS R271 actual Run-Setup',stderr:''};
 assert.equal(runFixture('pwsh',['-NoProfile','-NonInteractive','-File','fixture.ps1'],{cwd:'/fixture',execute:(engine,args,options)=>{
  assert.equal(engine,'pwsh');assert.deepEqual(args,['-NoProfile','-NonInteractive','-File','fixture.ps1']);
  assert.equal(options.timeout,15000);assert.equal(options.env.POWERSHELL_TELEMETRY_OPTOUT,'1');assert.equal(options.env.POWERSHELL_UPDATECHECK,'Off');
  return result;
 }}),result);
});
test('R276 PowerShell timeout retains the last completed stage and both output streams',()=>{
 assert.throws(()=>runFixture('pwsh',[],{execute:()=>({error:Object.assign(Error('spawnSync pwsh ETIMEDOUT'),{code:'ETIMEDOUT'}),stdout:'R271_STAGE host-saved 10ms\nR271_STAGE launch-0.12.65 11ms\n',stderr:'fixture failure detail'})}),/ETIMEDOUT[\s\S]*host-saved[\s\S]*launch-0\.12\.65[\s\S]*fixture failure detail/);
 assert.throws(()=>runFixture('pwsh',[],{execute:()=>({error:Error('startup failure')})}),/script did not report a stage/);
});
test('R276 nonzero script completion is returned to the original business assertion',()=>{
 const result={status:2,stdout:'R271_STAGE helper-loaded 1ms',stderr:'ASSERTION FAILED: original contract'};
 assert.equal(runFixture('pwsh',[],{execute:()=>result}),result);
});
