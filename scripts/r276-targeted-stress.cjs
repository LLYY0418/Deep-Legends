'use strict';
// Evidence run for the already identified R276 sweep failures, not a new audit.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const {tests:hexdataTests,runStress}=require('./r276-hexdata-stress.cjs');
const tests=['TestR196ProbeSelectsFastMirror','TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds'];
function runTargeted(out,run=spawnSync,hexRun=runStress){
 const root=path.resolve(__dirname,'..');fs.mkdirSync(out,{recursive:true});
 const started=performance.now();let hexdata=null,hexdataError=null;
 try{hexdata=hexRun(path.join(out,'hexdata'));}catch(error){hexdataError=error.message;const p=path.join(out,'hexdata','stress-summary.json');if(fs.existsSync(p))hexdata=JSON.parse(fs.readFileSync(p,'utf8'));}
 const invocations=[];
 // Three separate CPU invocations retain 200 repetitions at each CPU and the
 // default package fuse. R196 deliberately measures real paced transfers.
 for(const test of tests)for(const cpu of [1,2,4]){
  const args=['test','-race','-json','-count=200',`-cpu=${cpu}`,'-run',`^(${test})$`,'./backend'];
  const begun=performance.now(),r=run('go',args,{cwd:root,env:{...process.env,CGO_ENABLED:'1'},encoding:'utf8',maxBuffer:64*1024*1024});
  const raw=`${test}-cpu${cpu}.jsonl`,log=(r.stdout||'')+(r.stderr||'');fs.writeFileSync(path.join(out,raw),log);
  let events=[],error=r.error?.message||null;try{events=(r.stdout||'').split('\n').filter(Boolean).map(JSON.parse);}catch(e){error=e.message;}
  const passed=events.filter(e=>e.Test===test&&e.Action==='pass').length,failed=events.filter(e=>e.Action==='fail').length;
  const testFailed=events.filter(e=>e.Test===test&&e.Action==='fail').length;
  const complete=events.filter(e=>!e.Test&&e.Package==='lol-loot-assistant/backend'&&e.Action==='pass').length===1;
  invocations.push({test,cpu,command:['go',...args],exitCode:r.status,error,passed,failed,testFailed,packageComplete:complete,race:log.includes('WARNING: DATA RACE'),durationMs:performance.now()-begun,raw});
 }
 const success=!hexdataError&&hexdata?.exitCode===0&&hexdata.rows.length===hexdataTests.length&&hexdata.rows.every(r=>r.passed===600&&r.failed===0)&&invocations.length===6&&invocations.every(r=>r.exitCode===0&&!r.error&&r.passed===200&&r.failed===0&&r.packageComplete&&!r.race);
 const rows=[...(hexdata?.rows||[]).map(r=>({...r,expectedSamples:600,complete:!hexdataError&&r.passed===600&&r.failed===0})),...tests.map(test=>{const rs=invocations.filter(r=>r.test===test),passed=rs.reduce((n,r)=>n+r.passed,0),failed=rs.reduce((n,r)=>n+r.testFailed,0),samples=passed+failed;return {test,passed,failed,samples,failureRate:samples?failed/samples:null,expectedSamples:600,complete:rs.length===3&&rs.every(r=>r.exitCode===0&&!r.error&&r.passed===200&&r.failed===0&&r.packageComplete&&!r.race)};})];
 const summary={sha:process.env.GITHUB_SHA||null,platform:process.platform,count:200,cpu:[1,2,4],success,durationMs:performance.now()-started,hexdataError,hexdata,invocations,rows};
 fs.writeFileSync(path.join(out,'targeted-summary.json'),JSON.stringify(summary,null,2)+'\n');assert(success,'R276 targeted stress is incomplete or failed');return summary;
}
if(require.main===module){assert(process.argv[2],'usage: r276-targeted-stress.cjs <out>');console.log(JSON.stringify(runTargeted(path.resolve(process.argv[2]))));}
module.exports={tests,runTargeted};
