'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawnSync}=require('node:child_process');
const {selectGuards}=require('./r276-browser-catalog.cjs'),{runOffline}=require('./r276-offline.cjs');
async function main(){
 const [shard='all',output=process.env.RUNNER_TEMP||'/tmp/r276-browser']=process.argv.slice(2);
 const root=path.resolve(__dirname,'..'),out=path.resolve(output);fs.mkdirSync(out,{recursive:true});
 const binary=process.env.R276_PUBLIC_BACKEND||path.join(out,'r264-public');
 if(!fs.existsSync(binary))require('./r276-public-fixture.cjs').build(binary);
 const fixture=path.join(out,'r272-overview.json');
 fs.mkdirSync(path.join(out,'r272-shape'),{recursive:true});
 const shape=spawnSync('go',['test','./backend','-run','^TestR272SyntheticOverviewWireBudget$','-count=1'],{cwd:root,env:{...process.env,R272_SHAPE_OUTPUT:path.join(out,'r272-shape/result.json'),R272_OVERVIEW_FIXTURE:fixture},encoding:'utf8'});
 fs.writeFileSync(path.join(out,'shape.log'),(shape.stdout||'')+(shape.stderr||''));assert.equal(shape.status,0,'synthetic wire fixture failed');
 const only=process.env.R276_BROWSER_ONLY?.split(',');
 if(only&&process.env.CI)throw Error('CI cannot focus or omit browser guards');
 const selected=selectGuards(shard,only);
 const rows=[];
 for(const [name,file,key,extra={}] of selected){
  const folder=/^r\d+$/.test(name)?name+'-browser':name;
  const directory=path.join(out,folder),start=performance.now();fs.mkdirSync(directory,{recursive:true});
  const env={...extra,[key]:directory,R272_OVERVIEW_FIXTURE:fixture,R269_GEOMETRY_CURRENT:path.join(out,'r266-matrix')};
  for(const n of [264,265,266,267,269])env['R'+n+'_BACKEND']=binary;
  const result=await runOffline(process.execPath,[file],{env,output:path.join(out,name+'-offline.json')});
  rows.push({name,exitCode:result.code,duration_ms:performance.now()-start});
  fs.writeFileSync(path.join(out,'browser-job.json'),JSON.stringify(rows,null,2)+'\n');
  assert.equal(result.code,0,name+' guard failed');
 }
}
main().catch(e=>{console.error(e);process.exitCode=1;});
