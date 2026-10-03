"use strict";
// Actual production app with the Go cold-start response, synthetic local images.
const {spawn}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const web=path.resolve(__dirname,'../backend/web');
const output=path.resolve(__dirname,'../docs/history/reports/maintenance-overview-1003');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'maintenance-overview-browser-'));
const player=require('../backend/testdata/maintenance-overview-player.json');
let proc,ws,server;const streams=new Set();
function installFixture(player){
 window.__r197={instance:crypto.randomUUID(),errors:[],requests:[],rigMode:localStorage.getItem('camera-mode')||'none'};
 addEventListener('error',e=>__r197.errors.push(String(e.error?.stack||e.message)));
 addEventListener('unhandledrejection',e=>__r197.errors.push(String(e.reason?.stack||e.reason)));
 const original=window.fetch;
 window.fetch=async(input,init)=>{
  const url=typeof input==='string'?input:input.url;
  __r197.requests.push(url);
  const response=await original(input,init);
  if(url.split('?')[0]==='/api/gameplay/overview'){
   const payload=await response.json();payload.player={...payload.player,...player,backgroundVideoPath:''};
   return new Response(JSON.stringify(payload),{headers:{'Content-Type':'application/json'}});
  }
  if(url==='/api/status'){
   const payload=await response.json();payload.version='0.12.62';payload.snapshotReady=false;payload.identityReady=true;payload.ownedCount=0;payload.remaining=0;
   payload.summoner={...payload.summoner,...player,backgroundVideoPath:''};
   return new Response(JSON.stringify(payload),{headers:{'Content-Type':'application/json'}});
  }
  if(url==='/api/rig/status'){
   const payload=await response.json();payload.cameraMode=__r197.rigMode;
   return new Response(JSON.stringify(payload),{headers:{'Content-Type':'application/json'}});
  }
  if(url==='/api/rig/camera-mode'){
   __r197.rigMode=JSON.parse(init.body).mode;localStorage.setItem('camera-mode',__r197.rigMode);
  }
  return response;
 };
}
async function main(){
 server=require('node:http').createServer((req,res)=>{
  const pathname=new URL(req.url,'http://fixture.local').pathname;
  if(pathname==='/api/events'){res.setHeader('Content-Type','text/event-stream');res.setHeader('Cache-Control','no-cache');res.write(': connected\n\n');streams.add(res);req.on('close',()=>streams.delete(res));return;}
  if(pathname==='/fixture-reveal'){for(const stream of streams)stream.write('data: live-post-game-reveal\n\n');res.end('ok');return;}
  // Local demo avatar fixture; no real Riot/Windows image fetch.
  if(['/api/image','/api/champion-asset'].includes(pathname)){
   res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><rect width="48" height="48" fill="#27374b"/><circle cx="24" cy="17" r="9" fill="#b99b72"/><path d="M7 48v-9q2-14 17-14t17 14v9" fill="#576b83"/><path d="M14 14h20v7H14z" fill="#803d40"/></svg>');return;
  }
  if(pathname.startsWith('/api/')){res.statusCode=pathname.startsWith('/api/diagnostics/')?204:404;res.end();return;}
  const file=path.resolve(web,'.'+(pathname==='/'?'/index.html':pathname));
  if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
  const type={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png','.woff2':'font/woff2'}[path.extname(file)]||'application/octet-stream';res.setHeader('Content-Type',type);
  if(pathname==='/demo-data.js')res.end(fs.readFileSync(file,'utf8')+'\n('+installFixture.toString()+')('+JSON.stringify(player)+');');else res.end(fs.readFileSync(file));
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  const chrome = process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
  proc = spawn(chrome, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', `--user-data-dir=${temp}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
  const url = await new Promise((resolve, reject) => {
    let output = ''; const timer = setTimeout(() => reject(Error('Chrome startup timeout')), 20000);
    proc.once('error', reject); proc.stderr.on('data', chunk => {output += chunk; const m = output.match(/DevTools listening on (ws:\/\/[^\s]+)/); if (m) {clearTimeout(timer); resolve(m[1]);}});
    proc.once('exit', code => reject(Error(`Chrome exited ${code}: ${output.slice(-800)}`)));
  });
  ws = new WebSocket(url); await new Promise((resolve, reject) => {ws.addEventListener('open', resolve, {once: true}); ws.addEventListener('error', reject, {once: true});});
  let seq = 0; const pending = new Map(); let loaded = null;
  ws.addEventListener('message', event => {const msg = JSON.parse(event.data); if(msg.method==='Page.loadEventFired')loaded?.(); if (pending.has(msg.id)) {const [resolve, reject] = pending.get(msg.id); pending.delete(msg.id); msg.error ? reject(Error(JSON.stringify(msg.error))) : resolve(msg.result);}});
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {const id = ++seq; pending.set(id, [resolve, reject]); ws.send(JSON.stringify({id, method, params, sessionId}));});
  const {targetId} = await send('Target.createTarget', {url: 'about:blank'});
  const {sessionId} = await send('Target.attachToTarget', {targetId, flatten: true});
  const call = (method, params = {}) => send(method, params, sessionId);
  const evaluate = async expression => {const r = await call('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true}); if (r.exceptionDetails) {const debug=await call('Runtime.evaluate',{expression:'JSON.stringify({fixture:window.__r197,names:[...document.querySelectorAll(".live-player-name")].map(p=>({name:p.textContent,disabled:p.disabled})),live:document.querySelector("#live-panel")?.textContent,body:document.body.textContent.slice(-200)})',returnByValue:true});console.error(debug.result.value);throw Error(JSON.stringify(r.exceptionDetails));} return r.result.value;};
  await call('Page.enable'); await call('Emulation.setDeviceMetricsOverride', {width: 1280, height: 1200, deviceScaleFactor: 1, mobile: false});
  await call('Page.navigate', {url: `http://127.0.0.1:${server.address().port}/?demo`});


 const wait=expression=>evaluate(`new Promise((resolve,reject)=>{const end=Date.now()+15000;const tick=()=>{if(${expression})return resolve(true);if(Date.now()>end)return reject(Error('fixture wait failed: '+${JSON.stringify(expression)}));setTimeout(tick,30)};tick()})`);
 const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
 fs.mkdirSync(output,{recursive:true});
 const results=[];
 await wait('document.querySelector(".summoner-strip [data-overview-poster]")?.naturalWidth > 0 && decodeURIComponent(document.querySelector(".summoner-strip [data-overview-poster]").getAttribute("src")||"").includes("ahri_splash_centered_86") && window.__r197');
 const art=await evaluate('(()=>{const img=document.querySelector(".summoner-strip [data-overview-poster]"),r=img.closest(".summoner-strip").getBoundingClientRect();return {src:img.getAttribute("src"),width:img.naturalWidth,loaded:img.closest(".summoner-strip").classList.contains("has-loaded-image"),requests:__r197.requests,errors:__r197.errors,clip:{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1}}})()');
 assert.ok(art.loaded);assert.match(decodeURIComponent(art.src),/ahri_splash_centered_86/);
 assert.equal(art.requests.some(url=>url==='/api/refresh'||url==='/api/collection/refresh'),false);assert.deepEqual(art.errors,[]);
 let shot=await call('Page.captureScreenshot',{format:'png',clip:art.clip,captureBeyondViewport:true});fs.writeFileSync(path.join(output,'cold-overview.png'),Buffer.from(shot.data,'base64'));
 await click('[data-section="suite"]');await wait('window.__r197?.requests.includes("/api/rig/status")');await click('[data-suite-tab="rig"]');
 await wait('document.querySelector("[data-rig-camera-mode]")');
 for(const width of [1280,960,720]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:1200,deviceScaleFactor:1,mobile:false});
  await evaluate('document.querySelector("[data-rig-camera-mode]").closest("aside").scrollIntoView({block:"center"})');
  await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const rig=await evaluate('(()=>{const s=document.querySelector("[data-rig-camera-mode]"),r=s.closest("aside").getBoundingClientRect(),left=document.querySelector(".rig-main").getBoundingClientRect();return {width:innerWidth,overflow:document.documentElement.scrollWidth>innerWidth,right:r.x>left.x,actions:s.closest(".rig-actions")!==null,toggle:document.querySelector("[data-rig-settings-sync]")!==null,text:document.querySelector("#suite-panel-rig")?.textContent||document.querySelector(".rig-layout").textContent,value:s.value,errors:__r197.errors,clip:{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1}}})()');
  assert.equal(rig.actions,true);assert.equal(rig.toggle,false);assert.doesNotMatch(rig.text,/保留对局内设置改动/);assert.equal(rig.overflow,false);assert.deepEqual(rig.errors,[]);if(width===1280)assert.equal(rig.right,true);
  shot=await call('Page.captureScreenshot',{format:'png',clip:rig.clip,captureBeyondViewport:true});fs.writeFileSync(path.join(output,`maintenance-${width}.png`),Buffer.from(shot.data,'base64'));results.push(rig);
 }
 await evaluate('(()=>{const s=document.querySelector("[data-rig-camera-mode]");s.value="dynamic";s.dispatchEvent(new Event("change"))})()');
 await wait('!document.querySelector("[data-rig-camera-mode]").disabled && __r197.rigMode==="dynamic"');
 const previous=await evaluate('__r197.instance');await new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(Error('reload timeout')),15000);loaded=()=>{clearTimeout(timer);loaded=null;resolve()};call('Page.reload').catch(reject)});await wait('window.__r197?.instance !== '+JSON.stringify(previous)+' && document.readyState==="complete" && document.querySelector(".summoner-strip [data-overview-poster]")?.naturalWidth > 0');
 await click('[data-section="suite"]');await wait('window.__r197?.requests.includes("/api/rig/status")');await click('[data-suite-tab="rig"]');await wait('document.querySelector("[data-rig-camera-mode]")?.value==="dynamic"');
 fs.writeFileSync(path.join(output,'browser.json'),JSON.stringify({fixture:'Go first overview response; full public catalog; synthetic local SVG; demo maintenance API; no Windows client',art,results,persisted:'dynamic'},null,2)+'\n');
 console.log('Cold overview without Collection, maintenance layout 1280/960/720, camera persistence PASS');
}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();for(const stream of streams)stream.end();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
