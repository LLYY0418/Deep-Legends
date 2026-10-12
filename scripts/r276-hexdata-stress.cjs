'use strict';
// Fixed candidate only; baseline failures never count as a clean sweep.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const tests=[
 'TestHexdataSameHeroTenConcurrentLoadsUseOneRequestAndRestartUsesNone',
 'TestHexdataColdRankingBudgetAndDiskRestart',
 'TestR149MayhemCatalogsWarmConcurrentlyAndRestartFromDisk',
 'TestR276HexdataRestartWaitsForStateAndUsesDisk',
 'TestAdoptingNewBuildIDPrunesStaleHexdataFilesAsynchronously',
 'TestPruneStaleHexdataBuildsRemovesExpiredBuildsOnly',
 'TestPruneStaleHexdataBuildsMakesNoRequests',
 'TestHexdataCircuitStatePersistsAcrossRestart',
 'TestHexdataRestartClearsLongLivedPersistedCircuit',
 'TestHexdataHTTP429OpensPersistentCircuitBeforeNextRequest'
];
function main(){
 assert(process.argv[2],'usage: node scripts/r276-hexdata-stress.cjs <out>');
 const out=path.resolve(process.argv[2]),root=path.resolve(__dirname,'..');fs.mkdirSync(out,{recursive:true});
 const args=['test','-race','-json','-count=200','-cpu=1,2,4','-run','^('+tests.join('|')+')$','./backend'];
 const start=performance.now(),result=spawnSync('go',args,{cwd:root,env:{...process.env,CGO_ENABLED:'1'},encoding:'utf8',maxBuffer:64*1024*1024});
 const log=(result.stdout||'')+(result.stderr||'');fs.writeFileSync(path.join(out,'stress.jsonl'),log);
 const events=(result.stdout||'').split('\n').filter(Boolean).map(line=>JSON.parse(line));
 const rows=tests.map(test=>({test,passed:events.filter(e=>e.Action==='pass'&&e.Test===test).length,failed:events.filter(e=>e.Action==='fail'&&e.Test===test).length}));
 for(const row of rows){row.samples=row.passed+row.failed;row.failureRate=row.samples?row.failed/row.samples:null;}
 const summary={platform:process.platform,sha:process.env.GITHUB_SHA||null,count:200,cpu:[1,2,4],command:['go',...args],exitCode:result.status,duration_ms:performance.now()-start,rows};
 fs.writeFileSync(path.join(out,'stress-summary.json'),JSON.stringify(summary,null,2)+'\n');console.log(JSON.stringify(summary));
 assert(!result.error,result.error?.message);assert.equal(result.status,0,'fixed Hexdata restart stress failed');
 assert(!log.includes('WARNING: DATA RACE'),'restart stress contains a race');
 for(const row of rows){assert.equal(row.samples,600,row.test+' must run 200 times at each CPU count');assert.equal(row.failed,0,row.test+' failed');}
 assert.equal(events.filter(e=>e.Action==='fail').length,0,'unknown restart stress failure');
}
if(require.main===module)main();
module.exports={tests};
