'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {runOffline}=require('./r276-offline.cjs');
async function main(){
 const out=path.resolve(process.argv[2]),filename=path.join(__dirname,'r266-browser.cjs');fs.mkdirSync(out,{recursive:true});
 const rows=[];
 for(const [name,route] of [['no-spells','/api/gameplay/summoner-spells'],['no-rankings','/api/champions/rankings'],['fixed',null]]){
  const folder=path.join(out,name);fs.mkdirSync(folder,{recursive:true});
  const code=`const fs=require('node:fs'),Module=require('node:module');const filename=${JSON.stringify(filename)};let source=fs.readFileSync(filename,'utf8');const needle='const fixture=browserAPI(new URL(url));';if(${JSON.stringify(route)}){if(source.split(needle).length!==2)throw Error('fixture mutation anchor changed');source=source.replace(needle,'const fixture=pathname==='+${JSON.stringify(JSON.stringify(route))}+'?undefined:browserAPI(new URL(url));');}const m=new Module(filename,module);m.filename=filename;m.paths=Module._nodeModulePaths(require('node:path').dirname(filename));m._compile(source,filename);`;
  const result=await runOffline(process.execPath,['-e',code],{env:{R266_CASE_ONE:'1',R266_EXTRAS:'1',R266_BROWSER_OUT:folder},output:path.join(out,name+'-offline.json')});
  assert.equal(result.code,route?1:0,name+' unexpected exit');
  if(route){const failure=JSON.parse(fs.readFileSync(path.join(folder,'failure.json')));assert(failure.errors.some(e=>new URL(e.url||'http://localhost/').pathname===route),name+' failed outside its missing API fixture');assert(failure.error.includes(route) && !/TypeError|ReferenceError|timeout/.test(failure.error),name+' must fail the original missing-API assertion: '+failure.error);}
  else assert.equal(JSON.parse(fs.readFileSync(path.join(folder,'result.json'))).errors.length,0);
  rows.push({name,result:route?'FAIL (expected)':'PASS',exitCode:result.code});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2));console.log(JSON.stringify(rows.at(-1)));
 }
}
main().catch(e=>{console.error(e);process.exitCode=1;});
