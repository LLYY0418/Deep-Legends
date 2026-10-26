'use strict';
// Real packaged application + Win32 SetWindowPos + desktop CopyFromScreen.
const fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
const executable=path.resolve(process.argv[2]),out=path.resolve(process.argv[3]);fs.mkdirSync(out,{recursive:true});
const detail=require('../backend/testdata/r308/detail-response.json'),directory=require('../backend/testdata/r307/champion-directory.json');
const catalog={champions:Object.entries(directory).map(([id,m])=>({...m,id:Number(id),slug:m.key.toLowerCase(),imageSource:'gtimg',imagePath:'/images/lol/act/img/champion/'+m.key+'.png'}))};
const ranking={rows:Array.from({length:120},(_,i)=>({championId:Number(Object.keys(directory)[i%Object.keys(directory).length]),rank:i+1,grade:'B',winRate:52,pickRate:5,play:3000}))};
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const run=(cmd,args)=>new Promise((resolve,reject)=>{const p=spawn(cmd,args,{stdio:'inherit'});p.once('error',reject);p.once('exit',c=>c===0?resolve():reject(Error(cmd+' exit '+c)));});
(async()=>{
 const results=[];
 for(const variant of ['baseline','a','b','c','ab']){
  const dir=path.join(out,variant);fs.mkdirSync(dir,{recursive:true});const port=19312;
  const args=['--remote-debugging-port='+port,...(variant==='c'?['--disable-gpu']:[])];
  const child=spawn(executable,args,{stdio:['ignore','pipe','pipe']});const logs=[];child.stdout.on('data',c=>logs.push(String(c)));child.stderr.on('data',c=>logs.push(String(c)));
  let ws;
  try{
   let target;for(let i=0;i<600;i++){try{const targets=await fetch('http://127.0.0.1:'+port+'/json/list').then(r=>r.json());target=targets.find(t=>t.type==='page'&&t.url.startsWith('http://127.0.0.1'));if(target)break;}catch{}await sleep(100);}assert(target,'packaged renderer CDP target');
   ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j;});let seq=0;const pending=new Map();
   ws.onmessage=e=>{const m=JSON.parse(e.data);const p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p[1](Error(JSON.stringify(m.error))):p[0](m.result);}};
   const call=(method,params={})=>new Promise((r,j)=>{const id=++seq;pending.set(id,[r,j]);ws.send(JSON.stringify({id,method,params}));});
   const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
   await call('Page.enable');await call('Runtime.enable');
   const resizeStrategy="(()=>{const style=document.createElement('style');style.textContent='html.is-resizing .app-main {contain:layout paint} html.is-resizing [hidden] {content-visibility:hidden}';addEventListener('DOMContentLoaded',()=>document.head.append(style),{once:true});window.__r312PendingObservers=new Map();const RO=window.ResizeObserver;window.ResizeObserver=class extends RO{constructor(cb){super((entries,o)=>document.documentElement.classList.contains('is-resizing')?__r312PendingObservers.set(o,()=>cb(entries,o)):cb(entries,o))}};let timer;addEventListener('resize',()=>{document.documentElement.classList.add('is-resizing');clearTimeout(timer);timer=setTimeout(()=>requestAnimationFrame(()=>{document.documentElement.classList.remove('is-resizing');for(const cb of __r312PendingObservers.values())cb();__r312PendingObservers.clear();}),180);});})()";
   const setup=`localStorage.setItem('lol-loot-champion-mode','aram-mayhem');localStorage.setItem('lol-loot-theme','dark');localStorage.setItem('lol-loot-demo','1');window.__r312Long=[];new PerformanceObserver(l=>__r312Long.push(...l.getEntries().map(e=>({at:e.startTime,duration:e.duration})))).observe({type:'longtask'});window.__r312Variant=${JSON.stringify(variant)};`;
   await call('Page.addScriptToEvaluateOnNewDocument',{source:setup+(["a","ab"].includes(variant)?resizeStrategy:"")});
   const origin=new URL(target.url).origin;await call('Page.navigate',{url:origin+'/?demo&section=champions'});
   for(let i=0;i<300;i++){if(await evaluate(`Boolean(document.querySelector('[data-champion-mode="aram-mayhem"]'))`))break;await sleep(100);}
   // Let demo-data install its fetch fixture, then substitute only public champion fixture data.
   await evaluate(`window.__r312OriginalFetch=window.fetch;window.fetch=(url,init)=>{const p=new URL(url,location.href).pathname;const v=p==='/api/champions/catalog'?${JSON.stringify(catalog)}:p==='/api/champions/rankings'?${JSON.stringify(ranking)}:p==='/api/champions/detail'?${JSON.stringify(detail)}:null;return v?Promise.resolve(new Response(JSON.stringify(v))):__r312OriginalFetch(url,init)};window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:'overview'}}));window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:'champions'}}));`);
   for(let i=0;i<300;i++){const found=await evaluate(`(()=>{const n=document.querySelector('[data-champion-row="120"]');if(n){n.click();return true;}return false;})()`);if(found)break;await sleep(100);}
   for(let i=0;i<300;i++){if(await evaluate(`Boolean(document.querySelector('.mayhem-detail-content .aramkit-combo'))`))break;await sleep(100);}
   assert(await evaluate(`Boolean(document.querySelector('.mayhem-detail-content .aramkit-combo'))`),'real mayhem hero page');

   if(['b','ab'].includes(variant))await evaluate(`document.documentElement.style.background='#0B0E14';window.desktopTheme.setTheme('dark')`);
   await sleep(2000);await evaluate('__r312Long=[]');
   // CDP Browser window id is not a HWND. Enumerate native windows in PowerShell by PID tree/title.
   const pidFile=path.join(dir,'pid.txt');await run('powershell.exe',['-NoProfile','-Command',`Add-Type -AssemblyName System.Windows.Forms; Get-Process | Where-Object {$_.MainWindowTitle -like '*Deep Legends*'} | Select-Object -First 1 -ExpandProperty MainWindowHandle | Set-Content '${pidFile.replaceAll("'","''")}'`]);
   const hwnd=fs.readFileSync(pidFile,'utf8').replace(/^\uFEFF/,'').trim();assert(BigInt(hwnd)>0n,'real HWND');
   await run('powershell.exe',['-NoProfile','-File',path.join(__dirname,'r312-windows-capture.ps1'),'-Window',hwnd,'-Output',dir]);
   await sleep(500);const tasks=await evaluate('__r312Long'),capture=JSON.parse(fs.readFileSync(path.join(dir,'capture.json'),'utf8').replace(/^\uFEFF/,''));
   const result={variant,...capture,longtaskCount:tasks.length,longtaskMaxMs:Math.max(0,...tasks.map(t=>t.duration)),longtasks:tasks,page:await evaluate(`({url:location.href,title:document.title,hero:document.querySelector('.mayhem-detail-content')?.textContent.slice(0,160),background:getComputedStyle(document.documentElement).backgroundColor})`)};
   results.push(result);fs.writeFileSync(path.join(dir,'result.json'),JSON.stringify(result,null,2));
   await call('Browser.close');
  }finally{ws?.close();if(child.exitCode===null)await run('taskkill',['/PID',String(child.pid),'/T','/F']).catch(()=>{});fs.writeFileSync(path.join(dir,'app.log'),logs.join(''));}
 }
 fs.writeFileSync(path.join(out,'comparison.json'),JSON.stringify(results,null,2));console.log(JSON.stringify(results));
})().catch(e=>{console.error(e);process.exitCode=1});
