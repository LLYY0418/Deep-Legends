"use strict";
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..'),out=path.join(root,'docs/history/reports/r245/mutations'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'r245-mutants-')),rows=[];fs.mkdirSync(out,{recursive:true});
function killed(name,command,args,pattern){const started=new Date(),r=spawnSync(command,args,{cwd:root,encoding:'utf8',timeout:90000}),log=(r.stdout||'')+(r.stderr||'');fs.writeFileSync(path.join(out,name+'.log'),log);assert.equal(r.error,undefined);assert.equal(r.status,1);assert.match(log,pattern);assert.doesNotMatch(log,/SyntaxError|build failed|undefined:/);rows.push({name,started:started.toISOString(),finished:new Date().toISOString(),exit_code:r.status,assertion_killed:true,last5:log.trimEnd().split('\n').slice(-5)})}
try{
 const desktop=path.join(temp,'desktop');fs.mkdirSync(desktop);for(const name of ['license-window.cjs','license-window.license.cjs','window-bounds-store.cjs'])fs.copyFileSync(path.join(root,'desktop',name),path.join(desktop,name));
 const file=path.join(desktop,'license-window.cjs'),source=fs.readFileSync(file,'utf8');assert.ok(source.includes('Math.round(bounds[key])'));fs.writeFileSync(file,source.replace('Math.round(bounds[key])','bounds[key]'));
 killed('a-remove-rounding-150percent',process.execPath,['--test','--test-name-pattern=R245 1.5 native calls',path.join(desktop,'license-window.license.cjs')],/R245 (native geometry must be integer|must restore saved rectangle)/);
 const original=path.join(root,'backend/license.go'),mutant=path.join(temp,'license.go'),go=fs.readFileSync(original,'utf8');assert.ok(go.includes('licenseTimeout = 15 * time.Second'));fs.writeFileSync(mutant,go.replace('licenseTimeout = 15 * time.Second','licenseTimeout = 8 * time.Second'));const overlay=path.join(temp,'overlay.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[original]:mutant}}));
 killed('b-return-to-8second-timeout','go',['test','-tags=license','-count=1','-overlay='+overlay,'-run','^TestR245SlowNetwork$/^renew_headers10s$','./backend'],/R245 slow renew headers10s must succeed/);
 fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows));
}finally{fs.rmSync(temp,{recursive:true,force:true,maxRetries:5,retryDelay:100})}
