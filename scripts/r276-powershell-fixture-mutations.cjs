'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const out=path.resolve(process.argv[2]||''),source=fs.readFileSync(path.join(__dirname,'r276-powershell-fixture.cjs'),'utf8');assert(process.argv[2]);fs.mkdirSync(out,{recursive:true});const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r276-pwsh-mutants-')),rows=[];
function run(name,file,negative){
 const result=spawnSync(process.execPath,['--test','scripts/r276-powershell-fixture.test.cjs'],{cwd:path.resolve(__dirname,'..'),env:{...process.env,...(file?{R276_PWSH_FIXTURE_SOURCE:file}:{})},encoding:'utf8'}),log=result.stdout+result.stderr;
 fs.writeFileSync(path.join(out,name+'.log'),log);const ok=negative?result.status!==0&&/not ok|✖/.test(log)&&/ERR_ASSERTION|ETIMEDOUT|AssertionError/.test(log)&&!/SyntaxError|Cannot find module|ReferenceError/.test(log):result.status===0;
 rows.push({name,exitCode:result.status,assertionVerified:ok});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows.at(-1)));assert(ok,name);
}
try{
 run('baseline');
 for(const [name,needle,value] of [
  ['offline-telemetry-removed',"POWERSHELL_TELEMETRY_OPTOUT:'1',POWERSHELL_UPDATECHECK:'Off'","POWERSHELL_TELEMETRY_OPTOUT:'0',POWERSHELL_UPDATECHECK:'Default'"],
  ['total-deadline-expanded','timeout:15000','timeout:30000'],
  ['timeout-swallowed','if(result.error){','if(false){'],
 ]){
  assert.equal(source.split(needle).length,2,name);const file=path.join(temp,name+'.cjs');fs.writeFileSync(file,source.replace(needle,value));run(name,file,true);
 }
 run('restored');
}finally{fs.rmSync(temp,{recursive:true,force:true})}
