'use strict';
// Fixed candidate only; baseline failures never count as a clean sweep.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const tests=["TestARAMKitSameHeroTenConcurrentLoadsUseOneRequestAndRestartUsesNoImmutableRequests", "TestARAMKitColdRankingBudgetAndDiskRestart"];
function runStress(out,run=spawnSync){
 const root=path.resolve(__dirname,'..');fs.mkdirSync(out,{recursive:true});
 const start=performance.now(),invocations=[],events=[],logs=[];
 // Each test retains Go's default ten-minute fuse. Do not put 6000 distinct
 // repetitions under one package fuse, or extend that fuse to hide a hang.
 const groups=tests.map(test=>[test]);
 for(const [index,group] of groups.entries()){
  const args=['test','-race','-json','-count=200','-cpu=1,2,4','-run','^('+group.join('|')+')$','./backend'];
  const begun=performance.now(),result=run('go',args,{cwd:root,env:{...process.env,CGO_ENABLED:'1'},encoding:'utf8',maxBuffer:64*1024*1024});
  const log=(result.stdout||'')+(result.stderr||''),file=`invocation-${index}.jsonl`;logs.push(log);fs.writeFileSync(path.join(out,file),log);
  let parsed=[];let parseError=null;try{parsed=(result.stdout||'').split('\n').filter(Boolean).map(line=>JSON.parse(line));}catch(error){parseError=error.message;}
  events.push(...parsed);
  const packageComplete=parsed.filter(e=>e.Action==='pass'&&!e.Test&&e.Package==='lol-loot-assistant/backend').length===1;
  invocations.push({test:group.length===1?group[0]:null,command:['go',...args],exitCode:result.status,error:result.error?.message||parseError||(!packageComplete?'missing expected backend package pass':null),duration_ms:performance.now()-begun,raw:file});
 }
 const log=logs.join('');fs.writeFileSync(path.join(out,'stress.jsonl'),log);
 const rows=tests.map(test=>({test,passed:events.filter(e=>e.Action==='pass'&&e.Test===test).length,failed:events.filter(e=>e.Action==='fail'&&e.Test===test).length}));
 for(const row of rows){row.samples=row.passed+row.failed;row.failureRate=row.samples?row.failed/row.samples:null;}
 const exitCode=invocations.every(r=>r.exitCode===0&&!r.error)?0:1;
 const summary={platform:process.platform,sha:process.env.GITHUB_SHA||null,count:200,cpu:[1,2,4],strategy:'per-test-default-timeout',invocations,exitCode,duration_ms:performance.now()-start,rows};
 fs.writeFileSync(path.join(out,'stress-summary.json'),JSON.stringify(summary,null,2)+'\n');
 assert.equal(summary.exitCode,0,'fixed ARAMKit restart stress failed');
 assert(!log.includes('WARNING: DATA RACE'),'restart stress contains a race');
 for(const row of rows){assert.equal(row.samples,600,row.test+' must run 200 times at each CPU count');assert.equal(row.failed,0,row.test+' failed');}
 assert.equal(events.filter(e=>e.Action==='fail').length,0,'unknown restart stress failure');
 return summary;
}
function main(){
 assert(process.argv[2],'usage: node scripts/r307-aramkit-stress.cjs <out>');
 console.log(JSON.stringify(runStress(path.resolve(process.argv[2]))));
}
if(require.main===module)main();
module.exports={tests,runStress};
