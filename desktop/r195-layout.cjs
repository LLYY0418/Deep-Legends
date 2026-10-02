 'use strict';
// R195 demo data through the actual rune/tooltip renderers, CSS and image queue.
const {spawn}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const {effectFixture}=require('../backend/web/r195-harness.cjs');
const {read,extract,fixture}=require('../backend/web/r188-harness.cjs');
const directory=process.env.R195_SOURCE_DIR;
const before=process.env.R195_BEFORE==='1';
const output=path.resolve(__dirname,'../docs/history/reports/r195');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r195-layout-'));
let proc,ws,server;
async function main(){
 const source=read('gameplay.js',directory),f=effectFixture(source),p=fixture({gameplaySource:source});
 f.subject.perkStats.find(row=>row.perkId===8017).vars=[0,0,0];
 const members=[103,69,0].map((championId,i)=>({gameName:`演示玩家${i+1}`,tagLine:'示例',championId,championName:['阿狸','卡西奥佩娅','未知英雄'][i],premadeGroup:'1',premadeSize:3,premadeSource:'session'}));
 const scenes={runes:`<div class="build-detail"><section class="rune-detail"><header><b>符文</b>${f.renderRuneYield(f.subject)}</header><div class="rune-split"><div class="rune-split-tree">${f.renderUnifiedRuneBoard(f.subject)}</div>${f.renderRuneEffects(f.subject)}</div></section></div>`,premade:`<main class="fixture-premade">${p.renderLivePremadeTag(members[0],members)}</main>`};
 const catalog=JSON.parse(read('../data/champion_names_16.19.1_zh_cn.json'));
 const iconData=fs.readFileSync(path.resolve(__dirname,'../backend/data/champion_icons_16.19.1.bin'));
 server=require('node:http').createServer((req,res)=>{
  if(req.url.startsWith('/rune-styles/')){res.setHeader('Content-Type','image/svg+xml');res.end(fs.readFileSync(path.resolve(__dirname,'../backend/web'+req.url)));return;}
  if(req.url.startsWith('/api/image?')){
   const id=Number(new URL(req.url,'http://localhost').searchParams.get('path').split('/').at(-1));
   const span=catalog.icons.find(icon=>icon.id===id);assert.ok(span);res.setHeader('Content-Type','image/png');res.end(iconData.subarray(span.offset,span.offset+span.length));return;
  }
  if(req.url.startsWith('/fixture/')){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><circle cx="32" cy="32" r="30" fill="#b78c45"/><path d="M16 32L32 12 48 32 32 52Z" fill="#ffe3a0"/></svg>');return;}
  const scene=req.url.slice(1)||'runes';res.setHeader('Content-Type','text/html');
  res.end(`<!doctype html><html data-theme="azure"><meta charset="utf-8"><style>${read('app.css',directory)}\n${read('gameplay.css',directory)}</style><style>body{margin:0;padding:24px;background:var(--bg);overflow:auto}.fixture-premade{margin-top:190px;text-align:center}.rune-detail{margin:0}.rune-detail>header{padding:12px 18px;display:flex;justify-content:space-between}</style><body>${scenes[scene]}<script>${read('image-queue.js')}\n${extract(read('app.js',directory),'setupFloatingTooltips')}\nsetupFloatingTooltips();</script></body></html>`);
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
  await call('Page.enable'); await call('Emulation.setDeviceMetricsOverride', {width: 1280, height: 480, deviceScaleFactor: 1, mobile: false});
  await call('Page.navigate', {url: `http://127.0.0.1:${server.address().port}`});

 const frame=()=>evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 fs.mkdirSync(output,{recursive:true});const measures=[];
 for(const scene of ['runes','premade'])for(const width of scene==='runes'?[1280,820]:[640]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:scene==='runes'?950:330,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/${scene}`});
  await evaluate(`new Promise(resolve=>{const tick=()=>document.readyState==='complete'?resolve():setTimeout(tick,20);tick();})`);await frame();
  if(scene==='premade'){
   await evaluate(`document.querySelector('[data-tooltip-roster]').dispatchEvent(new Event('pointerover',{bubbles:true}));`);await frame();
   await evaluate(`new Promise((resolve,reject)=>{const deadline=Date.now()+10000;const tick=()=>{const imgs=[...document.querySelectorAll('#global-tooltip img')];if(imgs.every(i=>i.complete&&i.naturalWidth>0))resolve();else if(Date.now()>deadline)reject(Error('queued tooltip icons did not load'));else setTimeout(tick,20);};tick();})`);
  }
  await evaluate('Promise.all(document.getAnimations().map(a=>a.finished.catch(()=>{})))');
  const m=await evaluate(`(()=>{const box=e=>{const r=e.getBoundingClientRect();return{x:r.x,y:r.y,w:r.width,h:r.height}};const tip=document.querySelector('#global-tooltip');return{overflow:document.documentElement.scrollWidth>innerWidth,shards:document.querySelectorAll('.shards-line').length,keystone:document.querySelector('.is-keystone .game-icon')&&box(document.querySelector('.is-keystone .game-icon')),subtitle:document.querySelector('.is-keystone .name small')?.textContent,damage:document.querySelector('.rune-yield .is-damage b')?.textContent,shoe:document.querySelector('.eff[data-perk-id="8304"] .stat b')?.textContent,tooltip:tip&&!tip.hidden?{box:box(tip),text:tip.textContent,images:[...tip.querySelectorAll('img')].map(i=>({width:box(i).w,height:box(i).h,loaded:i.naturalWidth>0,url:i.dataset.queuedSrc})),placeholders:tip.querySelectorAll('span.tooltip-roster-icon').length,names:[...tip.querySelectorAll('.tooltip-roster-name')].map(e=>({text:e.textContent,x:box(e).x}))}:null};})()`);
  assert.equal(m.overflow,false);
  if(!before&&scene==='runes'){assert.equal(m.shards,0);assert.equal(m.keystone.w,44);assert.equal(m.keystone.h,44);assert.equal(m.subtitle,'精密');assert.equal(m.damage,'972');assert.equal(m.shoe,'7:30');}
  if(before&&scene==='runes'){assert.equal(m.shards,1);assert.equal(m.shoe,undefined);assert.notEqual(m.subtitle,'精密');}
  if(!before&&scene==='premade'){assert.equal(m.tooltip.images.length,2);assert.equal(m.tooltip.placeholders,1);assert.ok(m.tooltip.images.every(i=>i.width===20&&i.height===20&&i.loaded&&i.url.startsWith('/api/image?path=')),JSON.stringify(m.tooltip));assert.ok(m.tooltip.names.every(n=>n.x===m.tooltip.names[0].x));assert.doesNotMatch(m.tooltip.text,/阿狸|卡西奥佩娅|未知英雄/);}
  const params={format:'png'};
  if(scene==='premade'){const b=m.tooltip.box;params.clip={x:b.x-12,y:b.y-12,width:b.w+24,height:b.h+24,scale:2};}
  const shot=await call('Page.captureScreenshot',params);fs.writeFileSync(path.join(output,`${before?'before':'after'}-${scene}${scene==='runes'?'-'+(width===1280?'wide':'narrow'):''}.png`),Buffer.from(shot.data,'base64'));measures.push({scene,width,...m});
 }
 fs.writeFileSync(path.join(output,`${before?'before':'after'}-layout.json`),JSON.stringify(measures,null,2)+'\n');console.log(`R195 ${before?'before':'after'} Chromium scenes PASS`);
}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
