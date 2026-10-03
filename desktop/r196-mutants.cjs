'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const {spawnSync}=require('node:child_process');const root=path.resolve(__dirname,'..'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'r196-mutants-'));
const app=fs.readFileSync(path.join(root,'backend/web/app.js'),'utf8'),download=fs.readFileSync(path.join(root,'backend/update_download.go'),'utf8');
const entries=[
 {name:'fixed-order-no-probe',go:'TestR196ProbeSelectsFastMirror',text:download.replace('probes := u.probeUpdateSources(ctx, asset, mirrors)','probes := []updateSourceProbe{}')},
 {name:'disable-slow-switch',go:'TestR196SlowSourceSwitchResumesAndHashes',text:download.replace(/canSwitch := [^\n]+/,'canSwitch := false')},
 {name:'eta-seconds-only',node:'R196 8 ',text:app.replace('updateETA(progress.etaSeconds)','`约剩 ${progress.etaSeconds} 秒`')},
 {name:'ready-redownload',node:'R196 11 ',text:app.replace('function openUpdateDialog() {','function openUpdateDialog() {\n    if (updateUI.status?.state === "ready") void updateAction("download");')},
 {name:'toast-skip-phase-guard',node:'R196 12 ',text:app.replace('if ((fromToast || !wasInGame) && ["InProgress", "ChampSelect"].includes(updateUI.phase))','if (false)')},
];
const results=[];
for(const entry of entries){assert.notEqual(entry.text,entry.go?download:app);const file=path.join(temp,entry.name+(entry.go?'.go':'.js'));fs.writeFileSync(file,entry.text);let run;
 if(entry.go){const overlay=path.join(temp,entry.name+'-overlay.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,'backend/update_download.go')]:file}}));run=spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^'+entry.go+'$','-count=1'],{cwd:root,encoding:'utf8'});}else{run=spawnSync(process.execPath,['--test','--test-name-pattern='+entry.node,'backend/web/r196.test.cjs'],{cwd:root,env:{...process.env,R196_APP_SOURCE:file},encoding:'utf8'});}
 const log=(run.stdout||'')+(run.stderr||'');fs.writeFileSync(path.join(temp,entry.name+'.log'),log);assert.notEqual(run.status,0,entry.name+' survived');assert.match(log,entry.go?new RegExp('--- FAIL: '+entry.go):/AssertionError/,entry.name+' must fail the specified assertion');assert.doesNotMatch(log,/SyntaxError|ReferenceError|build failed/,entry.name+' must execute');results.push({name:entry.name,test:entry.go||entry.node.trim(),killed:true});
}
const out=path.join(root,'docs/history/reports/r196');fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'mutations.json'),JSON.stringify({results,logs:temp},null,2)+'\n');console.log(JSON.stringify(results));
