"use strict";
// Real production app and hero Arena navigation; only API responses are fixtures.
const {spawn}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const web=path.resolve(__dirname,'../backend/web');
const output=path.resolve(__dirname,'../docs/history/reports/r192');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r192-browser-'));
let proc,ws,server;
function installFixture(){
 window.__r192={attempts:0,errors:[]};
 addEventListener('error',event=>window.__r192.errors.push(String(event.error?.stack||event.message)));
 addEventListener('unhandledrejection',event=>window.__r192.errors.push(String(event.reason?.stack||event.reason)));
 const original=window.fetch;
 window.fetch=async(input,init)=>{
  const url=typeof input==='string'?input:input.url;
  if(url.startsWith('/api/champions/arena/match/KR_')){
   if(++window.__r192.attempts===1)return new Response('夹具：断网，完整详情读取失败',{status:503});
   const response=await original(input,init),payload=await response.json();
   for(const p of payload.participants)p.gameName=Number(p.participantId)===Number(payload.subjectParticipantId)?'补全主体':`补全玩家 ${p.participantId}`;
   return new Response(JSON.stringify(payload),{status:200,headers:{'Content-Type':'application/json'}});
  }
  const response=await original(input,init);
  if(url.startsWith('/api/champions/arena-first-places')){
   const payload=await response.json();
   payload.matches.push({...structuredClone(payload.matches[1]),gameId:98003});
   for(const match of payload.matches){
    for(const p of match.participants){p.gameName=Number(p.participantId)===Number(match.subjectParticipantId)?'轻量主体':`轻量玩家 ${p.participantId}`;delete p.damage;delete p.damageTaken;}
   }
   return new Response(JSON.stringify(payload),{status:200,headers:{'Content-Type':'application/json'}});
  }
  return response;
 };
}
async function main(){
 server=require('node:http').createServer((req,res)=>{
  const pathname=new URL(req.url,'http://fixture.local').pathname;
  if(pathname.startsWith('/api/')){res.statusCode=pathname.startsWith('/api/diagnostics/')?204:404;res.end();return;}
  const file=path.resolve(web,'.'+(pathname==='/'?'/index.html':pathname));
  if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
  const type={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png','.woff2':'font/woff2'}[path.extname(file)]||'application/octet-stream';res.setHeader('Content-Type',type);
  if(pathname==='/demo-data.js')res.end(fs.readFileSync(file,'utf8')+'\n('+installFixture.toString()+')();');else res.end(fs.readFileSync(file));
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
  const evaluate = async expression => {const r = await call('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true}); if (r.exceptionDetails) throw Error(JSON.stringify(r.exceptionDetails)); return r.result.value;};
  await call('Page.enable'); await call('Emulation.setDeviceMetricsOverride', {width: 1280, height: 1200, deviceScaleFactor: 1, mobile: false});
  await call('Page.navigate', {url: `http://127.0.0.1:${server.address().port}/?demo`});


 const wait=expression=>evaluate(`new Promise((resolve,reject)=>{const end=Date.now()+15000;const tick=()=>{if(${expression})return resolve(true);if(Date.now()>end)return reject(Error('fixture wait failed: '+${JSON.stringify(expression)}));setTimeout(tick,30)};tick()})`);
 const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
 await wait('document.querySelector(".match-list .match-entry") && window.__r192');
 await click('[data-section="champions"]');
 await wait('document.querySelector("[data-champion-mode=arena]")');
 await click('[data-champion-mode="arena"]');
 await wait('document.querySelector("[data-champion-row]")');
 await click('[data-champion-row="799"]');
 const list='.arena-match-list[data-arena-match-list="pros"]';
 await wait(`document.querySelectorAll('${list} .match-entry').length===3`);
 await evaluate(`window.__r192.cards=[...document.querySelectorAll('${list} .match-entry')];window.__r192.cards[0].scrollIntoView({block:'center'});`);
 assert.equal(await evaluate(`document.querySelector('${list} .match-stat-damage b').textContent`),'—');
 await click(`${list} .match-entry:first-child [data-toggle-match]`);
 await wait(`document.querySelector('${list} [data-retry-match-detail]')`);
 const failed=await evaluate(`({attempts:__r192.attempts,text:document.querySelector('${list} .match-entry').textContent,others:__r192.cards.slice(1).every((entry,i)=>entry===document.querySelectorAll('${list} .match-entry')[i+1])})`);
 assert.equal(failed.attempts,1);assert.match(failed.text,/完整详情未加载/);assert.equal(failed.others,true);
 fs.mkdirSync(output,{recursive:true});
 const screenshot=async name=>{await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);const result=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,name),Buffer.from(result.data,'base64'));};
 await screenshot('arena-details-failed.png');
 await click(`${list} [data-retry-match-detail]`);
 await wait(`document.querySelector('${list} .match-detail')`);
 const success=await evaluate(`(()=>{const card=document.querySelector('${list} .match-entry');return{attempts:__r192.attempts,name:card.querySelector('.is-current-player')?.textContent,damage:card.querySelector('.match-stat-damage b').textContent,taken:card.querySelector('.match-stat-taken b').textContent,retry:!!card.querySelector('[data-retry-match-detail]'),others:__r192.cards.slice(1).every((entry,i)=>entry===document.querySelectorAll('${list} .match-entry')[i+1]),errors:__r192.errors}})()`);
 assert.equal(success.attempts,2);assert.equal(success.name,'补全主体');assert.equal(success.damage,'98209');assert.equal(success.taken,'47140');assert.equal(success.retry,false);assert.equal(success.others,true);assert.deepEqual(success.errors,[]);
 await evaluate(`document.querySelector('${list} .match-entry .match-summary').scrollIntoView({block:'center'});`);
 await screenshot('arena-details-hydrated.png');
 fs.writeFileSync(path.join(output,'browser.json'),JSON.stringify({route:'英雄页 → 斗魂竞技场 → 高手第一名对局',fixture:true,failed,success},null,2));console.log('R192 actual Chromium hero Arena failure/retry/hydration PASS');
}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
