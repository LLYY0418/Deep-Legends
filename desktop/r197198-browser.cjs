"use strict";
// Real production app, backend-generated anonymous roster fixture and demo maintenance API.
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const web=path.resolve(__dirname,'../backend/web');
const output=path.resolve(__dirname,'../docs/history/reports/r197');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r197198-browser-'));
let proc,ws,server;const streams=new Set();
const roster=require('../backend/testdata/r197-post-game-reveal.json');
function installFixture(roster){
 window.__r197={errors:[],liveRequests:0,revealed:false,requests:[],overviewBodies:[],rigMode:localStorage.getItem('r198-camera')||'none'};
 addEventListener('error',e=>__r197.errors.push(String(e.error?.stack||e.message)));
 addEventListener('unhandledrejection',e=>__r197.errors.push(String(e.reason?.stack||e.reason)));
 const original=window.fetch;
 window.fetch=async(input,init)=>{
  const url=typeof input==='string'?input:input.url;
  __r197.requests.push(url);if(url==="/api/gameplay/overview"&&init?.body)__r197.overviewBodies.push(JSON.parse(init.body));
  if(url.startsWith('/api/gameplay/live')){
   const payload=structuredClone(__r197.revealed?roster.after:roster.before);
   payload.phase='EndOfGame';payload.queueLabel='灵活组排';payload.mapId=11;payload.players[0].isCurrent=true;
   __r197.liveRequests++;
   return new Response(JSON.stringify(payload),{headers:{'Content-Type':'application/json'}});
  }
  if(url.startsWith('/api/gameplay/phase'))return new Response(JSON.stringify({phase:'EndOfGame'}),{headers:{'Content-Type':'application/json'}});
  const response=await original(input,init);
  if(url==='/api/rig/camera-mode'){
   __r197.rigMode=JSON.parse(init.body).mode;localStorage.setItem('r198-camera',__r197.rigMode);
   return new Response(JSON.stringify({cameraMode:__r197.rigMode}),{headers:{'Content-Type':'application/json'}});
  }
  if(url==='/api/rig/status'){
   const payload=await response.json();payload.cameraMode=__r197.rigMode;payload.settingsLocked=true;
   return new Response(JSON.stringify(payload),{headers:{'Content-Type':'application/json'}});
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
  if(pathname==='/demo-data.js')res.end(fs.readFileSync(file,'utf8')+'\n('+installFixture.toString()+')('+JSON.stringify(roster)+');');else res.end(fs.readFileSync(file));
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
  let seq = 0; const pending = new Map();
  ws.addEventListener('message', event => {const msg = JSON.parse(event.data); if (pending.has(msg.id)) {const [resolve, reject] = pending.get(msg.id); pending.delete(msg.id); msg.error ? reject(Error(JSON.stringify(msg.error))) : resolve(msg.result);}});
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
 await wait('document.querySelector(".match-list .match-entry") && window.__r197');
 await click('[data-section="live"]');
 await wait('document.querySelector("[data-recommendation-tab=insight]")');
 await click('[data-recommendation-tab="insight"]');
 await wait('document.querySelector("#recommendation-panel-insight .live-player-name[disabled]")');
 for(const mode of ['before','after']){
  if(mode==='after'){
   await evaluate('__r197.revealed=true;fetch("/fixture-reveal")');
   await wait('[...document.querySelectorAll("#recommendation-panel-insight .live-player-name")].some(p=>p.textContent.startsWith("Fixture9")&&!p.disabled)');
  }
  await evaluate(`window.__r197.card=[...document.querySelectorAll('#recommendation-panel-insight .live-player')].find(p=>p.querySelector('.live-player-name')?.textContent===${JSON.stringify(mode==='before'?'隐藏玩家':'Fixture9#TEST')});__r197.card.scrollIntoView({block:'center'})`);
  await wait('__r197.card.querySelector("img")?.complete && __r197.card.querySelector("img")?.naturalWidth > 0');
  await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const result=await evaluate(`(()=>{const c=__r197.card,t=c.closest('.live-team'),r=t.getBoundingClientRect(),n=c.nextElementSibling;return {mode:${JSON.stringify(mode)},text:c.textContent+(n?.classList.contains('insight-match-row')?n.textContent:''),disabled:c.querySelector('.live-player-name').disabled,ref:c.querySelector('.live-player-name').dataset.playerRef,teamPlayers:t.querySelectorAll('.live-player').length,liveRequests:__r197.liveRequests,clip:{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1},errors:__r197.errors}})()`);
  assert.equal(result.teamPlayers,5);assert.deepEqual(result.errors,[]);
  if(mode==='before'){assert.equal(result.disabled,true);assert.equal(result.text.split('客户端未公开该玩家').length-1,1);assert.doesNotMatch(result.text,/未定级/)}
  else{assert.equal(result.disabled,false);assert.equal(result.ref,roster.after.players[9].playerRef);assert.match(result.text,/Fixture9/);assert.match(result.text,/黄金|GOLD/);assert.doesNotMatch(result.text,/隐藏玩家|客户端未公开该玩家/);assert.ok(result.liveRequests>results[0].liveRequests)}
  const shot=await call('Page.captureScreenshot',{format:'png',clip:result.clip,captureBeyondViewport:true});fs.writeFileSync(path.join(output,`${mode}-opponents.png`),Buffer.from(shot.data,'base64'));
  results.push(result);
 }
 await evaluate('__r197.card.querySelector(".live-player-name").click()');
 await wait('__r197.overviewBodies.some(body=>JSON.stringify(body).includes('+JSON.stringify(roster.after.players[9].playerRef)+'))');
 const overview=await evaluate('__r197.overviewBodies');
 await evaluate('new Promise(r=>setTimeout(r,500))');
 await click('[data-section="suite"]');await wait('__r197.requests.includes("/api/rig/status")');await click('[data-suite-tab="rig"]');
 await wait('document.querySelector("[data-rig-camera-mode]")');
 const initial=await evaluate('(()=>{const s=document.querySelector("[data-rig-camera-mode]");return {value:s.value,disabled:s.disabled,options:[...s.options].map(o=>[o.value,o.textContent]),maintenance:s.closest("aside").querySelector("h3").textContent,readOnly:document.querySelector("[data-rig-lock]").textContent}})()');
 assert.equal(initial.value,'none');assert.equal(initial.disabled,false);assert.equal(initial.options.length,4);assert.equal(initial.maintenance,'客户端维护');assert.equal(initial.readOnly,'解除锁定');
 await evaluate('(()=>{const s=document.querySelector("[data-rig-camera-mode]");s.value="free";s.dispatchEvent(new Event("change"))})()');
 await wait('!document.querySelector("[data-rig-camera-mode]").disabled && __r197.rigMode==="free"');
 await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/?demo&r198=refresh`});
 await wait('document.querySelector(".match-list .match-entry") && window.__r197');
 await evaluate('new Promise(r=>setTimeout(r,500))');
 await click('[data-section="suite"]');await wait('__r197.requests.includes("/api/rig/status")');await click('[data-suite-tab="rig"]');
 await wait('document.querySelector("[data-rig-camera-mode]")?.value==="free"');
 await wait('document.querySelector("[data-rig-camera-mode]").getBoundingClientRect().width>0');
 await evaluate('document.querySelector("[data-rig-camera-mode]").scrollIntoView({block:"center"})');
 await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 const maintenance=await evaluate('(()=>{const s=document.querySelector("[data-rig-camera-mode]"),r=s.closest(".suite-card").getBoundingClientRect();return {value:s.value,disabled:s.disabled,overflow:document.documentElement.scrollWidth>innerWidth,clip:{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1},errors:__r197.errors}})()');
 assert.equal(maintenance.disabled,false);assert.equal(maintenance.overflow,false);assert.deepEqual(maintenance.errors,[]);
 const shot=await call('Page.captureScreenshot',{format:'png',clip:maintenance.clip,captureBeyondViewport:true});const rigOutput=path.resolve(output,'../r198');fs.mkdirSync(rigOutput,{recursive:true});fs.writeFileSync(path.join(rigOutput,'maintenance.png'),Buffer.from(shot.data,'base64'));
 fs.writeFileSync(path.join(output,'browser.json'),JSON.stringify({route:'对局 → 详情 → 赛后事件刷新 → 点击名字打开总览',fixture:'Go 测试实际响应；匿名合成玩家；本地 SVG',results,overview},null,2)+'\n');
 fs.writeFileSync(path.join(rigOutput,'browser.json'),JSON.stringify({route:'工具 → 维护 → 选择自由镜头 → 刷新保持',fixture:'演示 API；设置只读',initial,maintenance},null,2)+'\n');
 console.log('R197/R198 actual Chromium event refresh, overview click, maintenance persistence PASS');

}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();for(const stream of streams)stream.end();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
