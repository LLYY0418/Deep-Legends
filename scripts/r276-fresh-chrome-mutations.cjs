'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const out=path.resolve(process.argv[2]||''),source=fs.readFileSync(path.join(__dirname,'r276-fresh-chrome-fixture.cjs'),'utf8');assert(process.argv[2]);fs.mkdirSync(out,{recursive:true});const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r276-context-mutants-')),rows=[];
function run(name,file,negative){
 const result=spawnSync(process.execPath,['--test','scripts/r276-fresh-chrome-fixture.test.cjs'],{cwd:path.resolve(__dirname,'..'),env:{...process.env,...(file?{R276_FRESH_CHROME_SOURCE:file}:{})},encoding:'utf8'}),log=result.stdout+result.stderr;
 fs.writeFileSync(path.join(out,name+'.log'),log);const ok=negative?result.status!==0&&/not ok|✖/.test(log)&&/ERR_ASSERTION|ETIMEDOUT|AssertionError/.test(log)&&!/SyntaxError|Cannot find module|ReferenceError/.test(log):result.status===0;
 rows.push({name,exitCode:result.status,assertionVerified:ok});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows.at(-1)));assert(ok,name);
}
try{
 run('baseline');
 for(const [name,needle,value] of [
  ['prior-target-not-closed',"await send('Target.closeTarget',{targetId});",''],
  ['reuse-prior-context',"{url:'about:blank',browserContextId}","{url:'about:blank'}"],
  ['fixture-initialization-omitted','await initialize({targetId:next.targetId,sessionId:attached.sessionId,browserContextId});','']
 ]){
  assert.equal(source.split(needle).length,2,name);const file=path.join(temp,name+'.cjs');fs.writeFileSync(file,source.replace(needle,value));run(name,file,true);
 }
 run('restored');
}finally{fs.rmSync(temp,{recursive:true,force:true})}
