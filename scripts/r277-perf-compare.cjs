'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const BASE='867ca1d7cb627a23af79e6de489a0d4b3c7389ea',root=path.resolve(__dirname,'..');
function main(argv){const opts={};for(let i=0;i<argv.length;i+=2){assert(['--out','--backend','--chrome'].includes(argv[i])&&argv[i+1],'--out DIR --backend EXE [--chrome CHROME]');opts[argv[i]]=path.resolve(argv[i+1]);}assert(opts['--out']&&opts['--backend'],'out/backend required');const out=opts['--out'];fs.mkdirSync(out,{recursive:true});const scratch=process.env.PI_SCRATCH_DIR||process.env.RUNNER_TEMP||os.tmpdir(),temp=fs.mkdtempSync(path.join(scratch,'r277-perf-'));const report={host:process.platform,arch:process.arch,node:process.version,base_sha:BASE,record_only:true,status:'RUNNING',runs:[]};const persist=()=>fs.writeFileSync(path.join(out,'comparison.json'),JSON.stringify(report,null,2)+'\n');persist();
 try{
  const original=spawnSync('git',['show',BASE+':backend/web/gameplay.js'],{cwd:root,encoding:'utf8',maxBuffer:8*1024*1024});assert.equal(original.status,0,original.stderr);fs.writeFileSync(path.join(temp,'gameplay.js'),original.stdout);
  for(const phase of ['before','after']){
   const directory=path.join(out,phase),env={...process.env,R277_PERF_PHASE:phase,R266_BACKEND:opts['--backend'],R266_BROWSER_OUT:directory,R266_WEB_OVERRIDE:phase==='before'?temp:path.join(root,'backend/web'),R277_MEASURE_ONLY:'0'};env.CHROME_BIN=opts['--chrome']||require('./r277-ci-orchestrate.cjs').chrome(env);
   delete env.R266_MATRIX_BASELINE;delete env.R266_MATRIX;delete env.R266_CARD_SERVER;delete env.R269_CHECKS;
   const run=spawnSync(process.execPath,[path.join(__dirname,'r277-perf-browser.cjs')],{cwd:root,env,encoding:'utf8',timeout:300000,maxBuffer:8*1024*1024});fs.writeFileSync(path.join(out,phase+'.log'),(run.stdout||'')+(run.stderr||'')+(run.error?'\n'+run.error.message:''));assert(!run.error&&run.status===0,phase+' actual browser run failed; log preserved');
   const result=JSON.parse(fs.readFileSync(path.join(directory,'result.json'),'utf8'));assert.equal(result.errors.length,0);assert.equal(result.violations.length,0);const measurement=result.results.find(r=>r.measurement==='400-match-before-after-record-only');assert(measurement&&measurement.match_count===400&&measurement.pages.length===19,'nonempty actual measurement required');report.runs.push(measurement);persist();
  }
  assert.equal(report.runs[0].fixture_sha256,report.runs[1].fixture_sha256,'before/after must use identical 400-match fixture');report.long_task_total_ms={before:report.runs[0].long_task_total_ms,after:report.runs[1].long_task_total_ms};report.long_task_delta_ms=report.runs[1].long_task_total_ms-report.runs[0].long_task_total_ms;report.status='PASS';
 }catch(e){report.status='FAIL';report.error=e.message;throw e;}finally{fs.rmSync(temp,{recursive:true,force:true});persist();}
 console.log(JSON.stringify(report.long_task_total_ms));
}
if(require.main===module){try{main(process.argv.slice(2));}catch(e){console.error(e.stack);process.exitCode=1;}}
