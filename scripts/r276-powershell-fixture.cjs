'use strict';
const assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
// The existing 15-second total deadline includes engine startup and cleanup.
// Offline fixtures need neither PowerShell telemetry nor update notifications.
function runFixture(engine,args,{cwd,execute=spawnSync}={}){
 const env={...process.env,POWERSHELL_TELEMETRY_OPTOUT:'1',POWERSHELL_UPDATECHECK:'Off'};
 const result=execute(engine,args,{cwd,encoding:'utf8',timeout:15000,env});
 if(result.error){
  const stages=(result.stdout||'').split(/\r?\n/).filter(line=>line.startsWith('R271_STAGE '));
  assert.fail(`${result.error.code||'spawn error'}: ${result.error.message}\nPowerShell fixture status=${result.status??'null'} signal=${result.signal||'none'}\nPowerShell fixture stages:\n${stages.join('\n')||'[script did not report a stage]'}\nstdout:\n${result.stdout||''}\nstderr:\n${result.stderr||''}`);
 }
 return result;
}
module.exports={runFixture};
