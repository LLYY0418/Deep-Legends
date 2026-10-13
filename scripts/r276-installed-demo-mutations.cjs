'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const out=path.resolve(process.argv[2]||''),source=fs.readFileSync(path.join(__dirname,'r276-installed-demo.cjs'),'utf8');assert(process.argv[2]);fs.mkdirSync(out,{recursive:true});const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r276-demo-mutants-')),rows=[];
function run(name,file,negative){
 const result=spawnSync(process.execPath,['--test','scripts/r276-installed-demo.test.cjs'],{cwd:path.resolve(__dirname,'..'),env:{...process.env,...(file?{R276_DEMO_SOURCE:file}:{})},encoding:'utf8'}),log=result.stdout+result.stderr;
 fs.writeFileSync(path.join(out,name+'.log'),log);const ok=negative?result.status!==0&&/not ok|✖/.test(log)&&/ERR_ASSERTION|ETIMEDOUT|AssertionError/.test(log)&&!/SyntaxError|Cannot find module|ReferenceError/.test(log):result.status===0;
 rows.push({name,exitCode:result.status,assertionVerified:ok});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows.at(-1)));assert(ok,name);
}
try{
 run('baseline');
 for(const [name,needle,value] of [
  ['status-before-gameplay-export','while (!window.deepLegendsMatchCards)', 'while (false)'],
  ['initial-load-not-awaited',"if(!(document.readyState==='complete' && window.deepLegendsMatchCards && !!document.querySelector('#setting-default-page + .app-select')))return false;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return document.readyState==='complete' && !!document.querySelector('#setting-default-page + .app-select');","return !!document.querySelector('#setting-default-page + .app-select');"],
  ['removed-legacy-connected-field',"window.deepLegendsMatchCards.live.state.status?.clientView?.state === 'ready'",'window.deepLegendsMatchCards.live.state.status?.connected'],
  ['default-frame-not-drained','await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));',''],
  ['stale-document-marker-accepted','window.__r265ProbeLoad===${marker}', 'true'],
  ['business-match-gate-removed',`await until("document.querySelector('.match-list .match-entry')");`,'']
 ]){
  assert(source.includes(needle),name);const file=path.join(temp,name+'.cjs');fs.writeFileSync(file,source.replaceAll(needle,value));run(name,file,true);
 }
 run('restored');
}finally{fs.rmSync(temp,{recursive:true,force:true})}
