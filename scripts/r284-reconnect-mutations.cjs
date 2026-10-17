'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2]),tmp=fs.mkdtempSync(path.join(os.tmpdir(),'r284-reconnect-mutants-')),results=[];
fs.mkdirSync(out,{recursive:true});
try{
 for(const [name,transform,pattern] of [
  ['p1-three-second-window',s=>s.replace('reconnectSettlementDelayMS = 15000','reconnectSettlementDelayMS = 3000').replace('effective := max(delay, reconnectSettlementDelayMS)','effective := 3000'),'TestR284ReconnectCrashAtFifteenSeconds'],
  ['p1-repeat-same-game',s=>s.replace('id > 0 && r.reconnectFired[watchReconnectKey{client, id}]','false').replace('once := r.reconnectFired[key]','once := false'),'TestR284ReconnectOncePerGameAndManualUnaffected'],
  ['p1-ignore-normal-exit',s=>s.replace('if s.ExitSignal == "normal" {','if false {'),'TestR284ReconnectNormalAndGameEnd'],
 ]){
  const source=fs.readFileSync(path.join(root,'backend/watch_reconnect.go'),'utf8'),mutant=transform(source);assert.notEqual(mutant,source);
  const file=path.join(tmp,name+'.go'),overlay=path.join(tmp,name+'.json');fs.writeFileSync(file,mutant);fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,'backend/watch_reconnect.go')]:file}}));
  const result=cp.spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^'+pattern+'$','-count=1'],{cwd:root,encoding:'utf8',timeout:180000,maxBuffer:4e6}),log=(result.stdout||'')+(result.stderr||'');
  fs.writeFileSync(path.join(out,name+'.log'),log);const killed=result.status!==0&&/--- FAIL:/.test(log)&&!/build failed|operation not permitted|no tests to run/.test(log);
  results.push({name,exitCode:result.status,killed});fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(results,null,2)+'\n');assert(killed,name+' survived or failed outside assertion');console.log(name+' KILLED');
 }
}finally{fs.rmSync(tmp,{recursive:true,force:true})}
