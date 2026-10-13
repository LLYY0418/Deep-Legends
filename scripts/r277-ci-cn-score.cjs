'use strict';
// Independent CN proof; no changes to calibrated renderer shards or assertions.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),Module=require('node:module'),assert=require('node:assert/strict'),{spawnSync,spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),{chrome}=require('./r277-ci-orchestrate.cjs');
function mutationCommand(){
 const file=path.join(__dirname,'r277-frontend-cn-score-mutations.cjs');
 const source=fs.readFileSync(file,'utf8');
 assert(source.includes('process.env.R277_REPORT_DIR')&&source.includes('GOCACHE:process.env.GOCACHE'),'native CN output/cache support required');
 return [file];
}
async function main(args){
 const mode=args.shift();assert(['mutants','browser'].includes(mode));const options={};
 while(args.length){const key=args.shift();assert(['--out','--backend'].includes(key)&&args.length&&!options[key],'invalid/duplicate option');options[key]=path.resolve(args.shift());}
 assert(options['--out'],'--out required');if(mode==='browser')assert(options['--backend']&&fs.existsSync(options['--backend']),'real public fixture required');
 const out=options['--out'];fs.mkdirSync(out,{recursive:true});
 const scratch=process.env.PI_SCRATCH_DIR||process.env.RUNNER_TEMP||os.tmpdir(),temp=fs.mkdtempSync(path.join(scratch,'r277-ci-cn-'));
 const env={...process.env,PI_SCRATCH_DIR:scratch,GOCACHE:path.join(temp,'gocache'),R277_REPORT_DIR:out,R277_MEASURE_ONLY:'0'};
 delete env.R266_MATRIX_BASELINE;delete env.R266_WEB_OVERRIDE;
 const report={mode,host:process.platform,status:'RUNNING',runs:[]};let child,exitPromise;
 const persist=()=>fs.writeFileSync(path.join(out,'ci-'+mode+'-result.json'),JSON.stringify(report,null,2)+'\n');
 function run(label,command,argv,timeout){
  const r=spawnSync(command,argv,{cwd:root,env,encoding:'utf8',timeout,maxBuffer:32*1024*1024});
  fs.writeFileSync(path.join(out,label+'.log'),`COMMAND: ${command} ${argv.join(' ')}\n`+(r.stdout||'')+(r.stderr||'')+(r.error?'\n'+r.error.message:''));
  report.runs.push({label,exit:r.status,error:r.error?.message});persist();assert(!r.error&&r.status===0,label+' failed; original log retained');
 }
 try{
  if(mode==='mutants'){
   run('fixed',process.execPath,['--test','backend/web/r277-cn-score.test.cjs'],90000);
   run('mutants',process.execPath,mutationCommand(),300000);
   const result=JSON.parse(fs.readFileSync(path.join(out,'mutations-result.json'),'utf8'));assert.equal(result.results.length,4);assert(result.results.every(r=>r.status==='killed'));
   run('restored',process.execPath,['--test','backend/web/r277-cn-score.test.cjs'],90000);
  }else{
   const binary=path.join(temp,'cn-score-test'+(process.platform==='win32'?'.exe':'')),ready=path.join(temp,'ready.json');
   run('fixture-build','go',['test','-c','-o',binary,'./backend'],180000);
   env.R277_CN_SCORE_SERVER=ready;env.R266_BACKEND=options['--backend'];env.CHROME_BIN=chrome(env);env.R266_BROWSER_OUT=path.join(out,'browser');report.chromium=env.CHROME_BIN;
   const fd=fs.openSync(path.join(out,'actual-http-fixture.log'),'w');
   child=spawn(binary,['-test.run=^TestR277CNScoreBrowserContract$','-test.v','-test.timeout=100s'],{cwd:path.join(root,'backend'),env,stdio:['ignore',fd,fd]});fs.closeSync(fd);
   let ended=false;
   exitPromise=new Promise(resolve=>{child.once('error',error=>{ended=true;resolve({error:error.message});});child.once('exit',(code,signal)=>{ended=true;resolve({code,signal});});});
   const deadline=Date.now()+20000;
   while(!fs.existsSync(ready)){assert(!ended,'HTTP fixture exited before ready');assert(Date.now()<deadline,'HTTP fixture readiness timeout');await new Promise(r=>setTimeout(r,50));}
   run('browser',process.execPath,['scripts/r277-frontend-cn-score-browser.cjs'],100000);
   assert.equal(fs.readFileSync(ready+'.done','utf8'),'browser-contract-pass','browser must mark real contract completion');
   const exit=await Promise.race([exitPromise,new Promise(resolve=>{const timer=setTimeout(()=>resolve({error:'HTTP fixture exit timeout'}),10000);timer.unref();})]);
   report.fixture_exit=exit;assert.equal(exit.code,0,'actual Go HTTP contract must finish PASS, not merely browser exit');
   const result=JSON.parse(fs.readFileSync(path.join(env.R266_BROWSER_OUT,'result.json'),'utf8'));
   assert.equal(result.errors.length,0);assert.equal(result.violations.length,0);
   const proofs=result.results.filter(r=>r.P===9&&['self','cn'].includes(r.kind));assert.equal(proofs.length,2,'nonempty two scoped real browser proofs required');report.proofs=proofs;
  }
  report.status='PASS';
 }catch(error){report.status='FAIL';report.error=error.message;throw error;}
 finally{
  if(child){const stopped=await require('./chrome-test-cleanup.cjs').stopChild(child);assert(stopped,'HTTP fixture did not exit; temporary directory must be retained');}
  try{run('cache-clean','go',['clean','-cache','-testcache'],60000);}finally{fs.rmSync(temp,{recursive:true,force:true});persist();}
 }
 console.log(JSON.stringify({mode,status:report.status}));
}
module.exports={mutationCommand};
if(require.main===module)main(process.argv.slice(2)).catch(e=>{console.error(e.stack);process.exitCode=1;});
