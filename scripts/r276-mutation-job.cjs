'use strict';
const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process');
const {runOffline}=require('./r276-offline.cjs');
async function main(){
 const [shard,folder]=process.argv.slice(2),out=path.resolve(folder);fs.mkdirSync(out,{recursive:true});
 const groups={'1':[269,270,271],'2':[272],'3':[273,274]};
 if(!groups[shard])throw Error('unknown mutation shard');
 const env={};
 if(shard==='1'){
  const binary=path.join(out,'r264-public');require('./r276-public-fixture.cjs').build(binary);
  env.R269_BACKEND=binary;env.R266_BACKEND=binary;
  const chrome=spawnSync('bash',['-c','command -v google-chrome-stable || command -v google-chrome || command -v chromium-browser || command -v chromium'],{encoding:'utf8'});
  if(!process.env.CHROME_BIN&&chrome.status!==0)throw Error('real Chromium required');
  env.CHROME_BIN=process.env.CHROME_BIN||chrome.stdout.trim();
 }
 for(const n of groups[shard]){
  const args=['scripts/r'+n+'-mutations.py','--out',path.join(out,'r'+n+'-mutations')];
  if(n===271)args.push('--browser');
  const result=await runOffline(process.platform==='win32'?'python':'python3',args,{env,output:path.join(out,'r'+n+'-offline.json')});
  if(result.code!==0)throw Error('R'+n+' mutation shard failed');
 }
 if(shard==='1'){
  const result=await runOffline(process.execPath,['scripts/r276-browser-fixture-mutations.cjs',path.join(out,'r276-browser-mutations')],{env,output:path.join(out,'r276-browser-mutations-offline.json')});
  if(result.code!==0)throw Error('R276 missing-browser-fixture mutations failed');
 }
}
main().catch(e=>{console.error(e);process.exitCode=1;});
