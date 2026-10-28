'use strict';
const fs=require('node:fs'),path=require('node:path'),net=require('node:net'),{spawn}=require('node:child_process'),assert=require('node:assert/strict');
const executable=path.resolve(process.argv[2]),out=path.resolve(process.argv[3]),label=process.argv[4];fs.mkdirSync(out,{recursive:true});
const wait=ms=>new Promise(r=>setTimeout(r,ms)),run=(cmd,args)=>new Promise((r,j)=>{const p=spawn(cmd,args,{stdio:'inherit'});p.once('error',j);p.once('exit',c=>c===0?r():j(Error(cmd+' exit '+c)));});
const connect=async url=>{const ws=new WebSocket(url);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});let seq=0;const pending=new Map();ws.onmessage=e=>{const m=JSON.parse(e.data),p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p[1](Error(JSON.stringify(m.error))):p[0](m.result)}};return{ws,call:(method,params={})=>new Promise((r,j)=>{const id=++seq;pending.set(id,[r,j]);ws.send(JSON.stringify({id,method,params}))})};};
(async()=>{
 const port=await new Promise(r=>{const s=net.createServer();s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>r(p))})}),mainPort=port+1;
 const child=spawn(executable,['--inspect='+mainPort,'--remote-debugging-port='+port,'--user-data-dir='+path.join(out,'user-data')],{env:{...process.env,LOL_LOOT_DATA_DIR:path.join(out,'data')},stdio:['ignore','pipe','pipe']});const logs=[];child.stdout.on('data',c=>logs.push(String(c)));child.stderr.on('data',c=>logs.push(String(c)));let renderer,main;
 try{
  let target,mainTarget;for(let i=0;i<600;i++){try{target=(await fetch(`http://127.0.0.1:${port}/json/list`).then(r=>r.json())).find(t=>t.type==='page'&&t.url.startsWith('http://127.0.0.1'));mainTarget=(await fetch(`http://127.0.0.1:${mainPort}/json/list`).then(r=>r.json()))[0];if(target)break}catch{}await wait(100)}assert(target,'real packaged renderer inspector');
  renderer=await connect(target.webSocketDebuggerUrl);
  const evaluate=async(conn,expression)=>{const r=await conn.call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
  const catalog=require('../backend/testdata/r308/editor-catalog.json').catalog,primary=catalog.styles.find(s=>s.id===8100),sub=catalog.styles.find(s=>s.id===8400),runes={primaryStyleId:8100,subStyleId:8400,selectedPerkIds:[...primary.slots.slice(0,4).map(s=>s.perks[0].id),sub.slots[1].perks[0].id,sub.slots[3].perks[0].id,...catalog.statModSlots.map(s=>s.perks[0].id)]};
  const value={runes},config={version:1,enabled:true,runes:true,spells:true,items:true,champions:{164:{ranked:value}}},champs={champions:[{id:164,key:'Camille',nameZh:'卡蜜尔',titleZh:'青钢影'}]};
  const setup=`localStorage.setItem('lol-loot-demo','1');(()=>{const routes=${JSON.stringify({'/api/auto-config/settings':config,'/api/auto-config/catalog':{...catalog,clientConnected:true,spellsStatus:'ready',itemsStatus:'ready'},'/api/champions/catalog':champs})};const wrap=delegate=>function(...args){const u=new URL(typeof args[0]==='string'?args[0]:args[0].url,location.href);return routes[u.pathname]?Promise.resolve(new Response(JSON.stringify(routes[u.pathname]))):Reflect.apply(delegate,window,args);};let current=wrap(window.fetch);Object.defineProperty(window,'fetch',{configurable:true,get:()=>current,set:d=>{current=wrap(d)}})})();`;
  await renderer.call('Page.addScriptToEvaluateOnNewDocument',{source:setup});
  const origin=new URL(target.url).origin,results=[];
  for(const page of ['overview','runes']){
   await renderer.call('Page.navigate',{url:origin+'/?demo'});
   for(let i=0;i<300;i++){try{if(await evaluate(renderer,'Boolean(window.deepLegendsSections&&document.querySelector("#suite-config-root"))'))break}catch{}await wait(100)}
   if(page==='runes')await evaluate(renderer,`(async()=>{await deepLegendsSections.load('suite');document.querySelector('[data-section="suite"]')?.click();document.querySelector('[data-suite-tab="config"]').click();await deepLegendsAutoConfig.load()})()`);
   if(page==='runes'){let ready=false;for(let i=0;i<300;i++){if(await evaluate(renderer,'Boolean(document.querySelector(".ac-tree"))')){ready=true;break}await wait(100)}assert(ready,'rune page opened');}
   for(const [width,height]of [[914,911],[1561,913]]){

    const file=path.join(out,`${page}-${width}x${height}.json`);await run('powershell.exe',['-NoProfile','-File',path.join(__dirname,'r314-edge-scan.ps1'),'-ProcessID',String(child.pid),'-Width',String(width),'-Height',String(height),'-Output',file,'-Label',`${label}/${page}/${width}x${height}`]);results.push(JSON.parse(fs.readFileSync(file)));
   }
  }
  fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({label,rows:results},null,2));
 }finally{renderer?.ws.close();main?.ws.close();if(child.exitCode===null)await run('taskkill',['/PID',String(child.pid),'/T','/F']).catch(()=>{});fs.writeFileSync(path.join(out,'app.log'),logs.join(''));}
})().catch(e=>{console.error(e);process.exitCode=1});
