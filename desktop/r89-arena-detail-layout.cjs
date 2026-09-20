// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// R89_LAYOUT_OUTPUT=/tmp/r89-before node desktop/r89-arena-detail-layout.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'backend','web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r89-arena-detail-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const url=await new Promise((resolve,reject)=>{let output=''; const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-1000)}`)));});
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 await call('Page.enable');
 await call('Page.addScriptToEvaluateOnNewDocument',{source:`{const NativeDate=Date;globalThis.Date=class extends NativeDate{constructor(...args){super(...(args.length?args:[1790000000000]));}static now(){return 1790000000000;}}}`});
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   const types={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'};
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');res.end(fs.readFileSync(file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve();}},50);setTimeout(()=>{clearInterval(timer);resolve();},5000);})`);

 await evaluate(`new Promise(resolve=>{const t=setInterval(()=>{if(document.querySelector('#overview-content .summoner-strip')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);resolve();},8000);})`);
 const output=process.env.R89_LAYOUT_OUTPUT || '/private/tmp/r89-arena-detail';fs.mkdirSync(output,{recursive:true});
 const waitFor=expression=>evaluate(`new Promise((resolve,reject)=>{const start=performance.now();const timer=setInterval(()=>{if(${expression}){clearInterval(timer);resolve();}else if(performance.now()-start>8000){clearInterval(timer);reject(Error('Arena detail did not settle'));}},30);})`);
 const frames=()=>evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
 await waitFor(`document.querySelector('.match-entry:has(.match-summary.is-arena) [data-toggle-match]:not([disabled])')`);
 await evaluate(`{const style=document.createElement('style');style.textContent='*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}';document.head.append(style);const s=document.getElementById('setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));}`);
 await evaluate(`document.fonts.ready`);
 const id=await evaluate(`document.querySelector('.match-entry:has(.match-summary.is-arena)').dataset.matchId`);
 const selector=`[data-match-id="${id}"] [data-toggle-match="${id}"]`;
 const click=async()=>{
  await evaluate(`document.querySelector(${JSON.stringify(selector)}).scrollIntoView({block:'center',behavior:'instant'})`);
  await frames();
  const point=await evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};})()`);
  await call('Input.dispatchMouseEvent',{type:'mousePressed',...point,button:'left',buttons:1,clickCount:1});
  await call('Input.dispatchMouseEvent',{type:'mouseReleased',...point,button:'left',buttons:0,clickCount:1});
 };
 const results=[];
 for(const width of [1920,1280,820,620]) {
  await call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});await frames();
  await evaluate(`window.r89OtherCards=[...document.querySelectorAll('.match-entry')].filter(e=>e.dataset.matchId!==${JSON.stringify(id)})`);
  await click();
  await waitFor(`document.querySelector('[data-match-id="${id}"] .arena-match-detail')`);
  await evaluate(`document.querySelector('[data-match-id="${id}"] .arena-match-detail').scrollIntoView({block:'start',behavior:'instant'})`);await frames();
  const measured=await evaluate(`(()=>{const detail=document.querySelector('[data-match-id="${id}"] .arena-match-detail');const card=detail.closest('.match-entry');const button=card.querySelector('[data-toggle-match]');const ancestors=[];for(let e=detail.parentElement;e;e=e.parentElement){const s=getComputedStyle(e);if(s.containerName!=='none')ancestors.push({name:s.containerName,width:e.clientWidth});}const rect=e=>{const r=e.getBoundingClientRect();return {width:r.width,height:r.height};};const r=detail.getBoundingClientRect();detail.scrollLeft=100;const canScroll=detail.scrollLeft>0;detail.scrollLeft=0;return {detail:rect(detail),columns:rect(detail.querySelector('.arena-detail-columns')),teams:[...detail.querySelectorAll('.arena-detail-team')].map(rect),players:detail.querySelectorAll('.arena-detail-player').length,ancestors,overflowX:getComputedStyle(detail).overflowX,canScroll,expanded:button.getAttribute('aria-expanded'),controls:button.getAttribute('aria-controls'),detailId:detail.closest('.match-detail').id,otherCardsStable:window.r89OtherCards.every(e=>e.isConnected&&document.querySelector('[data-match-id="'+e.dataset.matchId+'"]')===e),clip:{x:Math.max(0,r.x),y:Math.max(0,r.y),width:Math.min(r.width,innerWidth-Math.max(0,r.x)),height:Math.min(r.height,innerHeight-Math.max(0,r.y)),scale:1}};})()`);
  assert.equal(measured.expanded,'true');assert.equal(measured.controls,measured.detailId);assert.equal(measured.otherCardsStable,true);assert.equal(measured.players,21);assert.equal(measured.teams.length,7);
  assert.ok(measured.ancestors.some(a=>a.name==='matches-column'));
  assert.ok(measured.ancestors.every(a=>!a.name.split(' ').includes('arena-first')),'dead rule unexpectedly has an ancestor');
  if(measured.ancestors.find(a=>a.name==='matches-column').width<=720){assert.equal(measured.overflowX,'auto');assert.equal(measured.canScroll,true);}
  const shot=await call('Page.captureScreenshot',{format:'png',clip:measured.clip});
  fs.writeFileSync(path.join(output,`arena-detail-${width}.png`),Buffer.from(shot.data,'base64'));
  delete measured.clip;results.push({width,...measured});
  await click();await waitFor(`!document.querySelector('[data-match-id="${id}"] .arena-match-detail')`);
  assert.equal(await evaluate(`document.querySelector(${JSON.stringify(selector)}).getAttribute('aria-expanded')`),'false');
 }
 fs.writeFileSync(path.join(output,'measurements.json'),JSON.stringify(results,null,2)+'\n');
 console.log('R89 Chromium Arena detail: expand/collapse, 21 players, 7 teams, horizontal scroll, stable sibling cards PASS');
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
