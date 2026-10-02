"use strict";
const {execFile}=require("node:child_process");
const TYPES=["Browser","Renderer","GPU","Utility"];
const bounded=n=>Number.isFinite(n)?Math.max(0,Math.min(1e9,n)):undefined;
function normalizeProcessMetrics(raw={}) {
  const processes=[];
  for(const row of (Array.isArray(raw.processes)?raw.processes:[]).slice(0,4)) {
    if(!TYPES.includes(row.type))continue;
    const cpu=bounded(row.cpu_percent),memory=bounded(row.working_set_mb);
    if(cpu===undefined||memory===undefined)continue;
    processes.push({type:row.type,cpu_percent:cpu,working_set_mb:memory,count:Math.max(1,Math.min(1000,Math.floor(Number(row.count)||1)))});
  }
  const event={event:"desktop_process_metrics",window_ms:60000,processes};
  const backend=bounded(raw.backend_working_set_mb);if(backend!==undefined)event.backend_working_set_mb=backend;
  return event;
}
function backendMemory(pid) {
  if(!Number.isInteger(pid)||pid<=0)return Promise.resolve(undefined);
  const windows=process.platform==="win32";
  const cmd=windows?"powershell.exe":"ps";
  const args=windows?["-NoProfile","-NonInteractive","-Command",`$p=Get-Process -Id ${pid} -ErrorAction Stop; [Console]::Write($p.WorkingSet64)`]:["-o","rss=","-p",String(pid)];
  return new Promise(resolve=>execFile(cmd,args,{timeout:1500,windowsHide:true,maxBuffer:1024},(err,out)=>{const n=Number(String(out).trim());resolve(!err&&n>0?n/(windows?1048576:1024):undefined);}));
}
function startProcessMetrics({app,getBackendPid,report,readBackend=backendMemory,interval=setInterval,clear=clearInterval}) {
  let disposed=false,pending=false;
  const sample=async()=>{
    if(disposed||pending)return;pending=true;
    try {
      const groups=new Map();
      for(const row of app.getAppMetrics()) {
        if(!TYPES.includes(row.type))continue;
        const cpu=bounded(row.cpu?.percentCPUUsage),memory=bounded(row.memory?.workingSetSize);
        if(cpu===undefined||memory===undefined)continue;
        const g=groups.get(row.type)||{type:row.type,cpu_percent:0,working_set_mb:0,count:0};
        g.cpu_percent+=cpu;g.working_set_mb+=memory/1024;g.count++;groups.set(row.type,g);
      }
      const backend=await readBackend(getBackendPid());
      if(!disposed)report(normalizeProcessMetrics({processes:[...groups.values()],backend_working_set_mb:backend}));
    } catch {} finally {pending=false;}
  };
  const timer=interval(sample,60000);timer?.unref?.();
  return {sample,dispose(){disposed=true;clear(timer);}};
}
module.exports={normalizeProcessMetrics,startProcessMetrics};
