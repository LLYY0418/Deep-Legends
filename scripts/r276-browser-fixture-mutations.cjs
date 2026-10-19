'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {runOffline}=require('./r276-offline.cjs');
const {runWithPerformanceRetry,isMissingApiTarget}=require('./lib/mutation-performance-retry.cjs');
async function main(){
 const out=path.resolve(process.argv[2]),filename=path.join(__dirname,'r266-browser.cjs');fs.mkdirSync(out,{recursive:true});
 const rows=[];
 for(const [name,route] of [['no-spells','/api/gameplay/summoner-spells'],['no-rankings','/api/champions/rankings'],['fixed',null]]){
  const folder=path.join(out,name);fs.mkdirSync(folder,{recursive:true});
  const code=`const fs=require('node:fs'),Module=require('node:module');const filename=${JSON.stringify(filename)};let source=fs.readFileSync(filename,'utf8');const needle='const fixture=browserAPI(new URL(url));';if(${JSON.stringify(route)}){if(source.split(needle).length!==2)throw Error('fixture mutation anchor changed');source=source.replace(needle,'const fixture=pathname==='+${JSON.stringify(JSON.stringify(route))}+'?undefined:browserAPI(new URL(url));');}const m=new Module(filename,module);m.filename=filename;m.paths=Module._nodeModulePaths(require('node:path').dirname(filename));m._compile(source,filename);`;
  let result,attempts=[];
  const execute=async number=>{
   const attemptFolder=path.join(folder,'attempt-'+number);fs.mkdirSync(attemptFolder,{recursive:true});
   const logfile=path.join(attemptFolder,'browser.log'),fd=fs.openSync(logfile,'w'),at=Date.now();
   try{result=await runOffline(process.execPath,['-e',code],{env:{R287_MUTATION_PATH:route?'1':'0',R266_CASE_ONE:'1',R266_EXTRAS:'1',R266_BROWSER_OUT:attemptFolder},stdio:['ignore',fd,fd],output:path.join(attemptFolder,'offline.json')});}finally{fs.closeSync(fd);}
   const log=fs.readFileSync(logfile,'utf8');process.stdout.write(log);
   for(const file of ['failure.json','result.json'])if(fs.existsSync(path.join(attemptFolder,file)))fs.copyFileSync(path.join(attemptFolder,file),path.join(folder,file));
   const failure=fs.existsSync(path.join(attemptFolder,'failure.json'))?JSON.parse(fs.readFileSync(path.join(attemptFolder,'failure.json'))):{};
   return {code:result.code,error:failure.error||'',log,directory:attemptFolder,elapsedMs:Date.now()-at,targetVerified:isMissingApiTarget({route,failure,log})};
  };
  if(route){const proof=await runWithPerformanceRetry(execute,rows=>{attempts=rows;fs.writeFileSync(path.join(folder,'attempts.json'),JSON.stringify(rows,null,2));});assert(proof.verified,name+' did not fail its designated target; see attempts.json');}
  else attempts=[await execute(1)];
  assert.equal(result.code,route?1:0,name+' unexpected exit');
  if(route){const failure=JSON.parse(fs.readFileSync(path.join(folder,'failure.json')));assert(failure.errors.some(e=>new URL(e.url||'http://localhost/').pathname===route),name+' failed outside its missing API fixture');assert(failure.error.includes(route) && !/TypeError|ReferenceError|timeout/.test(failure.error),name+' must fail the original missing-API assertion: '+failure.error);}
  else assert.equal(JSON.parse(fs.readFileSync(path.join(folder,'result.json'))).errors.length,0);
  rows.push({name,result:route?'FAIL (expected)':'PASS',exitCode:result.code,attempts});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2));console.log(JSON.stringify(rows.at(-1)));
 }
}
main().catch(e=>{console.error(e);process.exitCode=1;});
