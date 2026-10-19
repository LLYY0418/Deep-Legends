'use strict';
process.env.R287_MUTATION_PATH='1'; // Negative cases only; child baselines use their own explicit mode.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2]),tmp=fs.mkdtempSync(path.join(os.tmpdir(),'r285-game-log-mutants-')),results=[];fs.mkdirSync(out,{recursive:true});
try{for(const [name,file,from,to,pattern] of [
 ['truncation-blocked','watch_reconnect.go','case "game-truncated", "crash-handler":','case "crash-handler":','TestR285RealGameLogReconnectDecisions'],
 ['unknown-allowed','watch_reconnect.go','return "canceled-unknown"','return ""','TestR285UnknownAndRunningReconnect'],
 ['quit-confirmed-is-voluntary','watch_reconnect_game_logs.go','early, completed, clean := false, false, false','if strings.Contains(r3d,"Quit Game Confirmed") { return "game-exit-early" }; early, completed, clean := false, false, false','TestR285RealGameLogReconnectDecisions']
]){
 const original=fs.readFileSync(path.join(root,'backend',file),'utf8');assert(original.includes(from));const target=path.join(tmp,name+'.go');fs.writeFileSync(target,original.replace(from,to));const overlay=path.join(tmp,name+'.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,'backend',file)]:target}}));
 const result=cp.spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^'+pattern+'$','-count=1'],{cwd:root,encoding:'utf8',timeout:180000,maxBuffer:4e6}),log=(result.stdout||'')+(result.stderr||'');fs.writeFileSync(path.join(out,name+'.log'),log);
 const killed=result.status!==0&&/--- FAIL:/.test(log)&&!/build failed|operation not permitted|no tests to run/.test(log);results.push({name,killed,status:result.status});assert(killed,name+' survived or failed outside assertion');console.log(name+' KILLED');
}}finally{fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(results,null,2)+'\n');fs.rmSync(tmp,{recursive:true,force:true})}
