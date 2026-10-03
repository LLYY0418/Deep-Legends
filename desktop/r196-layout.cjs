 'use strict';
const {spawn}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const {extract}=require('../backend/web/r188-harness.cjs');
const directory=process.env.R196_SOURCE_DIR||path.resolve(__dirname,'../backend/web'),before=process.env.R196_BEFORE==='1';
const read=name=>fs.readFileSync(path.join(directory,name),'utf8');
const output=path.resolve(__dirname,'../docs/history/reports/r196'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'r196-layout-'));
let proc,ws,server;
async function main(){
 const html=read('index.html'),source=read('app.js');
 const toolbar=html.slice(html.indexOf('<div class="topbar-actions">'),html.indexOf('</header>',html.indexOf('<div class="topbar-actions">')));
 const dialog=html.slice(html.indexOf('<dialog id="update-dialog"'),html.indexOf('</dialog>',html.indexOf('<dialog id="update-dialog"'))+9);
 const toast=before?'':html.slice(html.indexOf('<aside id="update-ready-toast"'),html.indexOf('</aside>',html.indexOf('<aside id="update-ready-toast"'))+8);
 const names=['updateBytes','renderUpdateStatus','openUpdateDialog','renderUpdateDialog','updateAction','setupUpdateEvents',...before?[]:['updateETA','renderUpdateReadyToast']];
 const runtime=names.map(name=>extract(source,name)).join('\n');
 server=require('node:http').createServer((req,res)=>{
  const scene=req.url.slice(1)||'downloading';const status={supported:true,current:'0.12.59',latest:'0.12.60',state:['ready','toast','ready-icon'].includes(scene)?'ready':'downloading',sizeBytes:111334912,publishedAt:'2026-10-03T06:00:00Z',progress:{receivedBytes:41193920,totalBytes:111334912,bytesPerSecond:350*1024,etaSeconds:192,selectingSource:scene==='probing'}};
  if(scene==='probing'){status.progress.receivedBytes=0;status.progress.bytesPerSecond=0;}
  res.setHeader('Content-Type','text/html');res.end(`<!doctype html><html data-theme="azure"><meta charset="utf-8"><style>${read('app.css')}\n${['build-item-row.css','gameplay.css','friends.css','metrics.css'].map(name=>fs.readFileSync(path.resolve(__dirname,'../backend/web',name),'utf8')).join('\n')}</style><style>body{margin:0;min-height:100vh;background:var(--bg)}.topbar-actions{margin:20px;justify-content:flex-end} .fixture-content{padding:20px;color:var(--muted)} .fixture-content div{height:100px;margin-bottom:16px;border:1px solid var(--line);border-radius:var(--radius-lg);background:var(--surface)}</style><body><header class="topbar" style="display:flex;justify-content:flex-end">${toolbar}</header><main class="fixture-content"><div></div><div></div></main>${dialog}${toast}<button id="settings-update-check" hidden></button><script>const camel=s=>s.replace(/-([a-z])/g,(_,c)=>c.toUpperCase());const el=Object.fromEntries([...document.querySelectorAll('[id]')].map(e=>[camel(e.id),e]));const updateUI={status:null,seen:'',announced:'',readyAnnounced:new Set(),phase:'None',pending:false,checkPending:false};const renderManualUpdateCheck=()=>{},finishManualUpdateCheck=()=>{},savePreference=()=>{},checkForUpdates=()=>{},renderUpdateNotes=()=>'';const api=async()=>({phase:'None'});${runtime}\nsetupUpdateEvents();renderUpdateStatus(${JSON.stringify(status)});if(${JSON.stringify(scene)}==='downloading'||${JSON.stringify(scene)}==='probing'||${JSON.stringify(scene)}==='ready')openUpdateDialog();</script></body></html>`);
 });await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
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
  await call('Page.enable'); await call('Emulation.setDeviceMetricsOverride', {width: 1280, height: 480, deviceScaleFactor: 1, mobile: false});
  await call('Page.navigate', {url: `http://127.0.0.1:${server.address().port}`});

 fs.mkdirSync(output,{recursive:true});const scenes=before?['busy-icon','ready-icon']:['probing','downloading','toast','ready','busy-icon','ready-icon'];const measures=[];
 for(const scene of scenes){
  await call('Emulation.setDeviceMetricsOverride',{width:900,height:650,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/${scene}`});
  await evaluate(`new Promise(resolve=>{const tick=()=>document.readyState==='complete'?resolve():setTimeout(tick,20);tick();})`);
  await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');await evaluate('Promise.all(document.getAnimations().map(a=>a.finished.catch(()=>{})))');
  const m=await evaluate(`(()=>{const box=e=>{const r=e.getBoundingClientRect();return{x:r.x,y:r.y,w:r.width,h:r.height}};const icon=document.querySelector('#update-button .control-icon'),tip=document.querySelector('#update-ready-toast'),dialog=document.querySelector('#update-dialog');return{icon:box(icon),controlSize:parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--control-icon-size')),fill:getComputedStyle(document.querySelector('.hex-track')).fill,stroke:getComputedStyle(document.querySelector('.hex-track')).strokeWidth,arrow:getComputedStyle(document.querySelector('#update-button .arrow')).strokeWidth,dialog:dialog.open?box(dialog):null,hint:document.querySelector('#update-progress-hint').textContent,buttons:[...dialog.querySelectorAll('.update-foot button')].filter(b=>!b.hidden).map(b=>b.textContent),toast:tip&&!tip.hidden?box(tip):null,overflow:document.documentElement.scrollWidth>innerWidth};})()`);
  assert.equal(m.overflow,false);
  if(!before){assert.equal(m.icon.w,m.controlSize);assert.equal(m.icon.h,m.controlSize);assert.equal(m.fill,'none');assert.equal(m.stroke,'1.6px');assert.equal(m.arrow,'1.8px');if(scene==='probing')assert.equal(m.hint,'正在选择最快的下载线路…');if(scene==='downloading'){assert.deepEqual(m.buttons,['取消下载','后台下载']);assert.ok(m.hint.endsWith('约剩 3 分 12 秒'));}if(scene==='toast'){assert.equal(m.toast.w,320);assert.equal(900-m.toast.x-m.toast.w,20);assert.equal(650-m.toast.y-m.toast.h,20);}}
  else assert.equal(m.icon.w,30);
  const b=scene.endsWith('-icon')?await evaluate(`(()=>{const r=document.querySelector('.topbar-actions').getBoundingClientRect();return{x:r.right-330,y:r.y,w:330,h:r.height};})()`):m.dialog||m.toast;
  const shot=await call('Page.captureScreenshot',{format:'png',clip:{x:Math.max(0,b.x-12),y:Math.max(0,b.y-12),width:b.w+24,height:b.h+24,scale:2}});fs.writeFileSync(path.join(output,`${before?'before':'after'}-${scene}.png`),Buffer.from(shot.data,'base64'));measures.push({scene,...m});
 }
 fs.writeFileSync(path.join(output,`${before?'before':'after'}-layout.json`),JSON.stringify(measures,null,2)+'\n');console.log(`R196 ${before?'before':'after'} Chromium scenes PASS`);
}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
