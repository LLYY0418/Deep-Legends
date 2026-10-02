"use strict";
// Real production app; hidden player is an anonymous demo API fixture. Before uses R192 committed gameplay.js.
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const web=path.resolve(__dirname,'../backend/web');
const output=path.resolve(__dirname,'../docs/history/reports/r193');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r193-browser-'));
let proc,ws,server;
let scene = 'before';
const beforeCSS = execFileSync('git', ['show', 'a175718b:backend/web/gameplay.css'], {cwd:web,encoding:'utf8'});
const before = execFileSync('git', ['show', 'a175718b:backend/web/gameplay.js'], {cwd:web,encoding:'utf8'});
function installFixture(){
 window.__r193={errors:[],liveRequests:0};
 addEventListener('error',e=>__r193.errors.push(String(e.error?.stack||e.message)));
 addEventListener('unhandledrejection',e=>__r193.errors.push(String(e.reason?.stack||e.reason)));
 const original=window.fetch;
 window.fetch=async(input,init)=>{
  const url=typeof input==='string'?input:input.url;
  const response=await original(input,init);
  if(url.startsWith('/api/gameplay/live')){
   const payload=await response.json();
   payload.phase='InProgress';payload.queueId=440;payload.queueLabel='灵活组排';payload.gameId=193;
   const p=payload.players.find(p=>p.teamId===200&&p.position==='jungle');
   Object.assign(p,{hidden:true,identityUnresolved:false,privateHistory:false,playerRef:'',gameName:'',tagLine:'',displayName:'隐藏玩家',rank:null,modeStats:{},recentGames:[],recentRankedRecord:null,recentPositions:[],historyState:'unavailable',championId:64,championName:'李青'});
   __r193.liveRequests++;
   return new Response(JSON.stringify(payload),{headers:{'Content-Type':'application/json'}});
  }
  if(url.startsWith('/api/gameplay/phase'))return new Response(JSON.stringify({phase:'InProgress'}),{headers:{'Content-Type':'application/json'}});
  return response;
 };
}
async function main(){
 server=require('node:http').createServer((req,res)=>{
  const pathname=new URL(req.url,'http://fixture.local').pathname;
  // Local demo avatar fixture; no real Riot/Windows image fetch.
  if(['/api/image','/api/champion-asset'].includes(pathname)){
   res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><rect width="48" height="48" fill="#27374b"/><circle cx="24" cy="17" r="9" fill="#b99b72"/><path d="M7 48v-9q2-14 17-14t17 14v9" fill="#576b83"/><path d="M14 14h20v7H14z" fill="#803d40"/></svg>');return;
  }
  if(pathname.startsWith('/api/')){res.statusCode=pathname.startsWith('/api/diagnostics/')?204:404;res.end();return;}
  const file=path.resolve(web,'.'+(pathname==='/'?'/index.html':pathname));
  if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
  const type={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png','.woff2':'font/woff2'}[path.extname(file)]||'application/octet-stream';res.setHeader('Content-Type',type);
  if(pathname==='/gameplay.css'&&scene==='before')res.end(beforeCSS);else if(pathname==='/gameplay.js'&&scene==='before')res.end(before);else if(pathname==='/demo-data.js')res.end(fs.readFileSync(file,'utf8')+'\n('+installFixture.toString()+')();');else res.end(fs.readFileSync(file));
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
 fs.mkdirSync(output,{recursive:true});
 const results=[];
 for(const mode of ['before','after']){
  scene=mode;
  if(mode==='after')await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/?demo&r193=after`});
  await wait('document.querySelector(".match-list .match-entry") && window.__r193');
  await click('[data-section="live"]');
  await wait('document.querySelector("[data-recommendation-tab=insight]")');
  await click('[data-recommendation-tab="insight"]');
  await wait('document.querySelector("#recommendation-panel-insight .live-player-name[disabled]")');
  await evaluate(`window.__r193.card=[...document.querySelectorAll('#recommendation-panel-insight .live-player')].find(p=>p.querySelector('.live-player-name')?.textContent==='隐藏玩家');__r193.card.scrollIntoView({block:'center'})`);
  await wait('__r193.card.querySelector("img")?.complete && __r193.card.querySelector("img")?.naturalWidth > 0');
  await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const result=await evaluate(`(()=>{const c=__r193.card,next=c.nextElementSibling,history=next?.classList.contains('insight-match-row')?next:null;const rect=c.getBoundingClientRect();return {mode:${JSON.stringify(mode)},text:c.textContent+(history?.textContent||''),subtitle:c.querySelector('.live-player-copy > span')?.textContent,columns:c.querySelectorAll('dl>div').length,history:!!history,disabled:c.querySelector('.live-player-name').disabled,avatar:c.querySelector('img').complete&&c.querySelector('img').naturalWidth>0,nextGap:next?next.getBoundingClientRect().top-rect.bottom:null,statWidth:c.querySelector('dl > div').getBoundingClientRect().width,clip:{x:rect.x+scrollX,y:rect.y+scrollY,width:rect.width,height:(history?history.getBoundingClientRect().bottom:rect.bottom)-rect.top,scale:1},rows:[...document.querySelectorAll('#recommendation-panel-insight .live-team')].map(t=>[...t.querySelectorAll('.live-player')].map(p=>({name:p.querySelector('.live-player-name')?.textContent,y:p.getBoundingClientRect().y,height:p.getBoundingClientRect().height}))),errors:__r193.errors}})()`);
  assert.equal(result.disabled,true);assert.equal(result.avatar,true);assert.deepEqual(result.errors,[]);
  if(mode==='before'){assert.match(result.subtitle,/打野 · 未定级/);assert.equal(result.text.split('客户端未公开该玩家').length-1,2);assert.equal(result.columns,3)}
  else{assert.equal(result.subtitle,'打野');assert.equal(result.text.split('客户端未公开该玩家').length-1,1);assert.equal(result.columns,1);assert.equal(result.history,false);assert.doesNotMatch(result.text,/未定级|—/);assert.equal(result.nextGap,0);assert.ok(result.statWidth>180);assert.ok(result.rows.flat().every(row=>row.height<90))}
  const shot=await call('Page.captureScreenshot',{format:'png',clip:result.clip,captureBeyondViewport:true});fs.writeFileSync(path.join(output,`hidden-card-${mode}.png`),Buffer.from(shot.data,'base64'));
  results.push(result);
 }
 await call('Emulation.setDeviceMetricsOverride',{width:760,height:1200,deviceScaleFactor:1,mobile:false});
 await evaluate('__r193.card.scrollIntoView({block:"center"})');
 await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 const narrow=await evaluate(`(()=>{const c=__r193.card,r=c.getBoundingClientRect(),n=c.nextElementSibling;return {width:760,subtitle:c.querySelector('.live-player-copy > span').textContent,history:n?.classList.contains('insight-match-row'),nextGap:n.getBoundingClientRect().top-r.bottom,columns:c.querySelectorAll('dl>div').length,clip:{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1}}})()`);
 assert.equal(narrow.subtitle,'打野');assert.equal(narrow.history,false);assert.equal(narrow.nextGap,0);assert.equal(narrow.columns,1);
 const shot=await call('Page.captureScreenshot',{format:'png',clip:narrow.clip,captureBeyondViewport:true});fs.writeFileSync(path.join(output,'hidden-card-after-narrow.png'),Buffer.from(shot.data,'base64'));
 fs.writeFileSync(path.join(output,'browser.json'),JSON.stringify({route:'对局页 → 详情',fixture:true,avatar:'本地 SVG 演示夹具',results,narrow},null,2)+'\n');console.log('R193 actual Chromium hidden card before/after PASS');

}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
