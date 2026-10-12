'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2]||'');assert(process.argv[2]);fs.mkdirSync(out,{recursive:true});
const file=path.join(__dirname,'r276-hexdata-stress.cjs'),source=fs.readFileSync(file,'utf8'),rows=[];
function run(name,modulePath,negative){
 const start=performance.now(),r=spawnSync(process.execPath,['--test','--test-reporter=tap','scripts/r276-hexdata-stress.test.cjs'],{cwd:root,env:{...process.env,...(modulePath?{R276_STRESS_MODULE:modulePath}:{})},encoding:'utf8'});const log=r.stdout+r.stderr;fs.writeFileSync(path.join(out,name+'.log'),log);
 const ok=negative?r.status!==0&&/not ok/.test(log)&&!/SyntaxError|MODULE_NOT_FOUND/.test(log):r.status===0;
 rows.push({name,exitCode:r.status,assertionVerified:ok,duration_ms:performance.now()-start});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows.at(-1)));assert(ok,name);
}
run('baseline',null,false);
const directory=fs.mkdtempSync(path.join(os.tmpdir(),'r276-stress-mut-'));
try{
 for(const [name,needle,replacement,signature] of [
  ['grouped-ten-tests','const groups=tests.map(test=>[test]);','const groups=[tests];','default package fuse must not cover ten unrelated workloads'],
  ['reduced-count',"'-count=200'","'-count=100'",'must run 200 times at each CPU count'],
  ['hidden-invocation-failure','const exitCode=invocations.every(r=>r.exitCode===0&&!r.error)?0:1;','const exitCode=0;','Missing expected exception']
 ]){
  assert.equal(source.split(needle).length,2);const changed=path.join(directory,name+'.cjs');fs.writeFileSync(changed,source.replace(needle,replacement));run(name,changed,true);assert(fs.readFileSync(path.join(out,name+'.log'),'utf8').includes(signature),name+' requires actual target assertion failure');
 }
}finally{fs.rmSync(directory,{recursive:true,force:true});}
assert.equal(fs.readFileSync(file,'utf8'),source);run('restored',null,false);
