'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{runOffline}=require('./r276-offline.cjs'),census=require('./r276-api-census.cjs');
async function main(){
 const out=path.resolve(process.argv[2]),filename=path.join(__dirname,'r264-browser.cjs'),rows=[];fs.mkdirSync(out,{recursive:true});
 for(const name of ['no-r264-rankings','fixed']){
  const directory=path.join(out,name),metadata=path.join(directory,'api-census');fs.mkdirSync(directory,{recursive:true});
  const probe="assert.equal(await evaluate("+JSON.stringify("fetch('/api/champions/rankings?mode=ranked&tier=emerald_plus&position=all').then(r=>r.status)")+"),200,'R276 synthetic rankings fixture required');";
  const code=`const fs=require('node:fs'),Module=require('node:module'),filename=${JSON.stringify(filename)};let source=fs.readFileSync(filename,'utf8');const needle='const fixture=r264API(new URL(url));';if(source.split(needle).length!==2)throw Error('R264 fixture anchor changed');if(${JSON.stringify(name)}==='no-r264-rankings')source=source.replace(needle,'const fixture=undefined;');const close='closedSessions.add(sessionId);';if(source.split(close).length!==2)throw Error('R264 close anchor changed');source=source.replace(close,${JSON.stringify(probe)}+close);const m=new Module(filename,module);m.filename=filename;m.paths=Module._nodeModulePaths(require('node:path').dirname(filename));m._compile(source,filename);`;
  const result=await runOffline(process.execPath,['-e',code],{env:{R287_MUTATION_PATH:name==='fixed'?'0':'1',R264_BACKEND:process.env.R264_BACKEND||process.env.R266_BACKEND,R264_CASE:'P3',R264_BROWSER_OUT:directory,R276_API_CENSUS_OUTPUT:metadata,NODE_OPTIONS:(process.env.NODE_OPTIONS||'')+' --require '+JSON.stringify(path.join(__dirname,'r276-api-census.cjs'))},output:path.join(out,name+'-offline.json')});
  const report=census.summarize(metadata);fs.writeFileSync(path.join(directory,'api-census-summary.json'),JSON.stringify(report,null,2)+'\n');
  assert.equal(result.code,name==='fixed'?0:1,name+' unexpected exit');
  if(name==='fixed')census.requireClean(report);
  else {const failure=JSON.parse(fs.readFileSync(path.join(directory,'failure.json')));assert(failure.error.includes('R276 synthetic rankings fixture required'),'mutant failed outside its missing fixture');assert(report.observations.some(r=>r.path==='/api/champions/rankings'&&r.status===502),'missing endpoint did not reach rejecting backend');}
  rows.push({name,exitCode:result.code,result:name==='fixed'?'PASS':'FAIL (expected)',apiFailures:census.failures(report).length});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows.at(-1)));
 }
}
main().catch(e=>{console.error(e);process.exitCode=1;});
