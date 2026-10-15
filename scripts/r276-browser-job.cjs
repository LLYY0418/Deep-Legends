'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawnSync}=require('node:child_process');
const {selectGuards}=require('./r276-browser-catalog.cjs'),{runOffline}=require('./r276-offline.cjs');
const census=require('./r276-api-census.cjs');
const {collectIndependent,requirePassed}=require('./lib/collect-independent.cjs');
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
 if(only?.includes('r269-geometry')&&!only.includes('r266-matrix'))only.push('r266-matrix');
 const selected=selectGuards(shard,only);
 const rows=[],apiRows=[];
 const tasks=selected.map(([name,file,key,extra={}])=>({name,dependsOn:name==='r269-geometry'?['r266-matrix']:[],run:async()=>{
  const folder=/^r\d+$/.test(name)?name+'-browser':name;
  const directory=path.join(out,folder),start=performance.now();fs.mkdirSync(directory,{recursive:true});
  const env={...extra,[key]:directory,R272_OVERVIEW_FIXTURE:fixture,R269_GEOMETRY_CURRENT:path.join(out,'r266-matrix')};
  const observe=name!=='r275-offline';
  env.R276_API_CENSUS_OUTPUT=observe?path.join(directory,'api-census-'+process.pid):'';
  env.R276_API_CENSUS_GUARD=name;
  env.NODE_OPTIONS=(process.env.NODE_OPTIONS||'')+(observe?' --require '+JSON.stringify(path.join(__dirname,'r276-api-census.cjs')):'');
  for(const n of [264,265,266,267,269])env['R'+n+'_BACKEND']=binary;
  const result=await runOffline(process.execPath,[file],{env,output:path.join(out,name+'-offline.json')});

  assert.equal(result.code,0,name+' guard failed');
  if(!observe)return;
  const report=census.summarize(env.R276_API_CENSUS_OUTPUT);apiRows.push({guard:name,...report});
  fs.writeFileSync(path.join(out,'api-census.json'),JSON.stringify({shard,guards:apiRows},null,2)+'\n');
  const classified=census.classify(report,name);apiRows[apiRows.length-1].assertedFaults=classified.assertedFaults;apiRows[apiRows.length-1].otherFailures=classified.otherFailures;apiRows[apiRows.length-1].canceled=classified.canceled;apiRows[apiRows.length-1].canceledCount=classified.canceledCount;
  fs.writeFileSync(path.join(out,'api-census.json'),JSON.stringify({shard,excluded:['r275-offline negative subprocesses'],guards:apiRows},null,2)+'\n');
  census.requireClean(report,name);
 }}));
 rows.push(...await collectIndependent(tasks,{onResult:(row,all)=>{
  if(row.status!=='passed'&&row.name!=='r275-offline'&&!apiRows.some(r=>r.guard===row.name))apiRows.push({guard:row.name,status:row.status,error:row.error,dependencies:row.dependencies});
  fs.writeFileSync(path.join(out,'browser-job.json'),JSON.stringify(all,null,2)+'\n');
  fs.writeFileSync(path.join(out,'api-census.json'),JSON.stringify({shard,excluded:['r275-offline negative subprocesses'],guards:apiRows},null,2)+'\n');
 }}));
 assert.equal(apiRows.length,selected.filter(row=>row[0]!=='r275-offline').length,'API census must cover every selected guard');
 requirePassed(rows);
}
main().catch(e=>{console.error(e);process.exitCode=1;});
