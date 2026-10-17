"use strict";
const fs = require("node:fs"), path = require("node:path");
const MAX_BYTES = 2 * 1024 * 1024;
function regular(file) {
  try { const info=fs.lstatSync(file); if(!info.isFile() || info.isSymbolicLink()) throw Error("untrusted desktop log"); return info; }
  catch(error){if(error.code==='ENOENT')return null;throw error}
}
function trustedDirectory(directory) {
  fs.mkdirSync(directory,{recursive:true,mode:0o700});
  const info=fs.lstatSync(directory);if(!info.isDirectory()||info.isSymbolicLink())throw Error('untrusted log directory');
}
function appendDesktopLogFile(directory, message) {
  const safe=String(message||'').replace(/bootstrap=[^\s&]+/gi,'bootstrap=[redacted]')
    .replace(/(token|authorization)["'\s:=]+[^\s,"']+/gi,'$1=[redacted]').trim();
  if(!safe)return;
  trustedDirectory(directory);
  const names=['desktop.log',...Array.from({length:4},(_,i)=>`desktop.${i+1}.log`)].map(n=>path.join(directory,n));
  const active=regular(names[0]);
  const data=`${new Date().toISOString()} ${safe.slice(0,2000)}\n`;
  if(active && active.size+Buffer.byteLength(data)>MAX_BYTES){
    names.slice(1).forEach(regular); // Validate all generations before changing any evidence.
    if(regular(names[4]))fs.unlinkSync(names[4]);
    for(let i=3;i>=0;i--)if(regular(names[i]))fs.renameSync(names[i],names[i+1]);
  }
  fs.appendFileSync(names[0],data,{encoding:'utf8',mode:0o600,flag:fs.constants.O_WRONLY|fs.constants.O_APPEND|fs.constants.O_CREAT|(fs.constants.O_NOFOLLOW||0)});
}
// Export only structured startup timings and known event labels. stderr and
// exception text are not a privacy-reviewed schema and must never leave as raw
// text (redacting a few token spellings is not a sufficient privacy boundary).
function desktopCrashEvent(kind, details = {}) {
  if (!["render-process-gone", "child-process-gone", "uncaughtException"].includes(kind)) return null;
  const event = { event: "desktop_process_failure", kind };
  if (["clean-exit", "abnormal-exit", "killed", "crashed", "oom", "launch-failed", "integrity-failure"].includes(details.reason)) event.reason = details.reason;
  if (Number.isInteger(details.exitCode)) event.exitCode = Math.max(-2147483648, Math.min(2147483647, details.exitCode));
  if (["GPU", "Utility", "Zygote", "Sandbox helper", "Renderer", "Browser"].includes(details.type)) event.processType = details.type;
  if (["Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"].includes(details.name)) event.errorType = details.name;
  for (const key of ['name','serviceName']) {
    const value = details[key];
    if (typeof value === 'string' && (['Network Service','Audio Service','Storage Service','Video Capture','Print Compositor'].includes(value) || /^[a-z][a-z0-9_]*(?:\.[a-zA-Z][a-zA-Z0-9_]*){1,5}Service$/.test(value)) && value.length<=128) event[key]=value;
  }
  return event;
}
function isNetworkService(details={}) {
  return [details.name,details.serviceName].some(value=>value==='Network Service' || value==='network.mojom.NetworkService');
}
async function recoverNetworkService(window, record) {
  if (!window || window.isDestroyed() || window.webContents.isDestroyed()) return false;
  const started=Date.now(),contents=window.webContents;
  const bounded=async promise=>{
    let timer;
    try{return await Promise.race([promise,new Promise(resolve=>{timer=setTimeout(()=>resolve(false),1000);})]);}
    catch{return false;}finally{clearTimeout(timer);}
  };
  const probe=()=>bounded(contents.executeJavaScript('fetch("/api/status",{cache:"no-store",signal:AbortSignal.timeout(900)}).then(r=>r.ok).catch(()=>false)'));
  for(let i=0;i<2;i++) {
    if(await probe()){record({event:'network_service_recovered',duration_ms:Date.now()-started,reloaded:false});return true;}
    if(window.isDestroyed() || contents.isDestroyed())return false;
  }
  const loaded=new Promise(resolve=>contents.once('did-finish-load',()=>resolve(true)));
  contents.reload();
  if(await bounded(loaded) && await probe()) {record({event:'network_service_recovered',duration_ms:Date.now()-started,reloaded:true});return true;}
  record({event:'network_service_recovery_failed',duration_ms:Date.now()-started});return false;
}
function desktopLogForExport(directory, now = Date.now()) {
  let info;try{info=fs.lstatSync(directory)}catch(e){if(e.code==='ENOENT')return '';throw e}
  if(!info.isDirectory()||info.isSymbolicLink())throw Error('untrusted log directory');
  const rows=[];let omitted=0, expired=0;
  const timingKeys=['processToJs','jsToReady','readyToSplash','splashPaint','splashWindowShown','spawnToReady','readyToWindow','total'];
  const labels=['启动标识读取失败','启动页加载失败','启动阶段记录失败','启动阶段发送失败','系统代理下发失败','界面缩放偏好保存失败','自动截图失败'];
  for(const name of ['desktop.4.log','desktop.3.log','desktop.2.log','desktop.1.log','desktop.log']){
    const file=path.join(directory,name),stat=regular(file);if(!stat)continue;
    if(stat.size>MAX_BYTES+8192)throw Error('desktop log too large');
    for(const line of fs.readFileSync(file,'utf8').split('\n')){
      if(!line)continue;
      const match=line.match(/^(\d{4}-\d{2}-\d{2}T[\d:.]+Z) (.*)$/);if(!match){omitted++;continue}
      const [,time,message]=match;
      const at=Date.parse(time);
      if (!Number.isFinite(at)) { omitted++; continue; }
      if (at < now - 7*24*60*60*1000 || at > now) { expired++; continue; }
      if(message.startsWith("进程指标 ")) {
        try {const {normalizeProcessMetrics}=require("./process-metrics.cjs");rows.push(JSON.stringify({...normalizeProcessMetrics(JSON.parse(message.slice(5))),time}));continue;}catch{}
      }
      if(message.startsWith("后端证据 ")) {
        try {
          const { normalizeBackendEvidence } = require("./backend-evidence.cjs");
          const event = normalizeBackendEvidence(JSON.parse(message.slice(5)));
          if(event){rows.push(JSON.stringify({...event,time}));continue;}
        } catch {}
      }
      if(message.startsWith('网络服务 ')) {
        try {const e=JSON.parse(message.slice(5));if(['network_service_recovered','network_service_recovery_failed'].includes(e.event) && Number.isFinite(e.duration_ms)){rows.push(JSON.stringify({event:e.event,duration_ms:Math.min(5000,Math.max(0,e.duration_ms)),reloaded:e.reloaded===true,time}));continue;}}catch{}
      }
      if(message.startsWith('进程异常 ')) {
        try { const raw=JSON.parse(message.slice(5));const event=desktopCrashEvent(raw.kind, {reason:raw.reason,exitCode:raw.exitCode,type:raw.processType,name:raw.name || raw.errorType,serviceName:raw.serviceName});
          if(event){rows.push(JSON.stringify({...event,time}));continue;}
        } catch {}
      }
      if(message.startsWith('启动耗时 ')){
        try{const raw=JSON.parse(message.slice(5)),timings={};for(const key of timingKeys)if(Number.isFinite(raw[key]))timings[key]=Math.max(0,Math.min(3600000,Math.round(raw[key])));
          rows.push(JSON.stringify({event:'desktop_startup',time,message:'启动耗时',...timings}));continue;
        }catch{}
      }
      const label=labels.find(label=>message.startsWith(label+'：'));
      if(label)rows.push(JSON.stringify({event:'desktop_error',time,message:label}));else omitted++;
    }
  }
  if(omitted)rows.push(JSON.stringify({event:'desktop_export_redacted',omittedLines:omitted}));
  if(expired)rows.push(JSON.stringify({event:'desktop_export_window',time:new Date(now).toISOString(),windowDays:7,expiredLines:expired}));
  return rows.length ? rows.join('\n')+'\n' : '';
}
module.exports={appendDesktopLogFile,desktopLogForExport,desktopCrashEvent,isNetworkService,recoverNetworkService,MAX_BYTES};
