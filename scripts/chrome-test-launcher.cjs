'use strict';
// Test-only DevTools startup. Page waits and assertions belong to each caller.
const {spawn}=require('node:child_process');
const fs=require('node:fs'),path=require('node:path');
const realClock={now:()=>performance.now(),setTimeout,clearTimeout};
async function launchChrome({executable,args,directory,clock=realClock,onStderr=()=>{}}){
 if(process.env.R276_OFFLINE_PROXY)args=[...args,'--proxy-server='+process.env.R276_OFFLINE_PROXY,'--proxy-bypass-list=localhost;127.0.0.1;[::1]'];
 const attempts=[];
 const save=()=>{fs.mkdirSync(directory,{recursive:true});fs.writeFileSync(path.join(directory,'chrome-startup.json'),JSON.stringify({attempts},null,2)+'\n');fs.writeFileSync(path.join(directory,'chrome-startup-stderr.log'),attempts.map(a=>`attempt ${a.attempt} (${a.result})\n${a.stderr}`).join('\n'));};
 const failure=()=>Error(attempts.map(a=>`Chrome ${a.result==='timeout'?'startup timeout':a.result} attempt ${a.attempt} (pid=${a.pid}, exit=${a.exitCode}, signal=${a.signalCode}): ${a.stderr}`).join('\n'));
 for(let attempt=1;attempt<=2;attempt++){
  const started=clock.now(),child=spawn(executable,args,{stdio:['ignore','ignore','pipe']});
  let log='';const closed=new Promise(resolve=>child.once('close',resolve));
  const result=await new Promise(resolve=>{
   let finished=false;
   const finish=(result,endpoint)=>{if(finished)return;finished=true;clock.clearTimeout(timer);child.off('error',error);child.off('exit',exit);child.stderr?.off('data',data);resolve({result,endpoint});};
   const error=e=>{log+='\n'+e.message;finish('spawn-error');};
   const exit=()=>finish('exit');
   const data=c=>{log+=c;const match=log.match(/DevTools listening on (ws:\/\/\S+)/);if(match)finish('connected',match[1]);onStderr({attempt,child,text:String(c)});};
   const timer=clock.setTimeout(()=>finish('timeout'),20000);
   child.once('error',error);child.once('exit',exit);child.stderr?.on('data',data);
  });
  attempts.push({attempt,result:result.result,elapsedMs:clock.now()-started,pid:child.pid??null,exitCode:child.exitCode,signalCode:child.signalCode,stderr:log.slice(-2000)});save();
  if(result.result==='connected')return {child,endpoint:result.endpoint};
  // A replacement must not overlap the timed-out process. SIGKILL bounds
  // cleanup for a child that ignores SIGTERM; neither timer affects page waits.
  if(child.exitCode===null&&child.signalCode===null)child.kill('SIGTERM');
  const killTimer=setTimeout(()=>child.kill('SIGKILL'),5000);
  try{await closed;}finally{clearTimeout(killTimer);}
  if(result.result!=='timeout'||attempt===2)throw failure();
 }
}
module.exports={launchChrome};
