'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const ps=fs.readFileSync(path.join(__dirname,'r206-real-upgrade-windows.ps1'),'utf8');
const action=/Invoke-R279UpgradeStage -Name 'corrupt'[^\r\n]+-Action \{\r?\n([\s\S]*?)\r?\n    \}/.exec(ps)?.[1];
function verify(source){assert(source,'actual corrupt stage action exists');assert(!source.includes('Start-R265DebugApp'),'corrupt must retain the original already-running app lifecycle');assert(source.includes('node scripts/r265-persisted-ui.cjs $debugPort corrupt'));assert(source.includes('if ($LASTEXITCODE -ne 0)'));}
test('real corrupt stage retains app lifecycle and its original failure assertion',()=>verify(action));
test('mutation: redundant restart before corrupt must fail the lifecycle invariant',()=>assert.throws(()=>verify('$debugPort=Start-R265DebugApp\n'+action),/already-running app lifecycle/));
test('shared-profile chains are explicitly dependent on corrupt; never run on contaminated input',()=>{const start=ps.indexOf("-Name 'later-shared-profile-chains'");assert(start>0);assert(ps.slice(start,start+100).includes("-DependsOn @('corrupt')"));assert(ps.indexOf('Assert-R279UpgradeStages',start)<ps.indexOf("$r266Evidence=Join-Path $evidence '080-to-081'"));});
