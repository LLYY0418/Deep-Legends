'use strict';
const fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
const {stopChild}=require('../chrome-test-cleanup.cjs');
function resultFiles(directory,excluded=new Set()){
 const rows=[];
 function visit(folder){for(const entry of fs.readdirSync(folder,{withFileTypes:true})){
  const filename=path.join(folder,entry.name),relative=path.relative(directory,filename);
  if(excluded.has(relative))continue;
  if(entry.isDirectory())visit(filename);else if(entry.isFile()){const stat=fs.statSync(filename);rows.push({file:relative,bytes:stat.size,modified:stat.mtimeMs});}
 }}
 visit(directory);return rows.sort((a,b)=>a.file.localeCompare(b.file));
}
function mutantStates(directory,ids){
 let verified=[];try{verified=JSON.parse(fs.readFileSync(path.join(directory,'browser-mutations-result.json'),'utf8')).results||[];}catch{}
 return ids.map(id=>{
  const recorded=verified.find(row=>row.id===id);if(recorded)return {id,state:recorded.status};
  const result=path.join(directory,`browser-mutant-${id}-result.json`),log=path.join(directory,`browser-mutant-${id}.log`);
  if(fs.existsSync(result)){try{return {id,state:'failure-recorded-awaiting-target-verification',error:JSON.parse(fs.readFileSync(result,'utf8')).error};}catch{return {id,state:'result-writing'};}}
  return {id,state:fs.existsSync(log)?'log-recorded':'pending'};
 });
}
function runProgressChild({command,args=[],cwd,env,directory,logFile,ids=[],label='browser-mutants',idleMs=300000,totalMs=1800000,
 clock={now:Date.now,setTimeout,clearTimeout},spawnChild=spawn,watchDirectory=fs.watch,stop=stopChild,writeOutput=chunk=>process.stdout.write(chunk),writeError=chunk=>process.stderr.write(chunk)}){
 fs.mkdirSync(directory,{recursive:true});fs.writeFileSync(logFile,`COMMAND: ${command} ${args.join(' ')}\n`);
 const excluded=new Set([path.relative(directory,logFile),'browser-mutants-result.json','browser-mutants-progress.json']);
 const started=clock.now();let lastProgressAt=started,stdoutBytes=0,done=false,idleTimer,totalTimer,watcher,child;
 let files=resultFiles(directory,excluded),signature=JSON.stringify(files),lastProgress={kind:'started',elapsedMs:0};
 const snapshot=()=>({label,elapsedMs:clock.now()-started,idleMs:clock.now()-lastProgressAt,stdoutBytes,lastProgress,files,mutants:mutantStates(directory,ids)});
 return new Promise((resolve,reject)=>{
  const cleanup=()=>{clock.clearTimeout(idleTimer);clock.clearTimeout(totalTimer);watcher?.close();};
  const fail=async reason=>{
   if(done)return;done=true;const progress=snapshot();cleanup();
   const error=new Error(`${label}: ${reason}; last progress ${JSON.stringify(progress)}`);error.progressSnapshot=progress;
   fs.appendFileSync(logFile,`\nRUNNER ERROR: ${error.message}\n`);
   await stop(child);reject(error);
  };
  const armIdle=()=>{clock.clearTimeout(idleTimer);idleTimer=clock.setTimeout(()=>void fail(`no progress for ${idleMs}ms`),idleMs);};
  const progress=kind=>{lastProgressAt=clock.now();lastProgress={kind,elapsedMs:lastProgressAt-started};armIdle();};
  try{
   watcher=watchDirectory(directory,{recursive:true},()=>{
    if(done)return;
    try{const next=resultFiles(directory,excluded),key=JSON.stringify(next);if(key!==signature){files=next;signature=key;progress('result-files');}}
    catch(error){void fail('result directory unavailable: '+error.message);}
   });
   watcher.on?.('error',error=>void fail('result watcher failed: '+error.message));
   child=spawnChild(command,args,{cwd,env,stdio:['ignore','pipe','pipe']});
   child.stdout.on('data',chunk=>{if(done)return;stdoutBytes+=Buffer.byteLength(chunk);fs.appendFileSync(logFile,chunk);writeOutput(chunk);progress('stdout');});
   child.stderr.on('data',chunk=>{if(done)return;fs.appendFileSync(logFile,chunk);writeError(chunk);});
   child.once('error',error=>void fail('spawn failed: '+error.message));
   child.once('close',(exit,signal)=>{if(done)return;done=true;const final=snapshot();cleanup();resolve({exit,signal,progress:final});});
   armIdle();totalTimer=clock.setTimeout(()=>void fail(`total limit ${totalMs}ms reached`),totalMs);
  }catch(error){void fail(error.message);}
 });
}
module.exports={runProgressChild,resultFiles,mutantStates};
