'use strict';
// R214: production banner/detail/popover structures and CSS, real client crests.
const {spawn}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const {fixture}=require('../backend/web/r190-harness.cjs');
const {read,extract,compile,escapeHTML}=require('../backend/web/r188-harness.cjs');
const output=path.resolve(__dirname,'../docs/history/reports/r214');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r214-layout-'));
let proc,ws,server;
async function main(){
 const source=read('gameplay.js');
 const icon=(_kind,id,name='',size='')=>`<span class="game-icon is-${size}"><img data-game-image data-queued-src="/fixture/${id}.svg" alt="${escapeHTML(name)}"></span>`;
 const fn=compile(source,['masteryDetailsPortrait','masteryMarks','masteryTooltipContent','renderMasteryDetails','renderSummonerHighlights'],{number:String,compactNumber:String,escapeHTML,iconFigure:icon,highestCurrentRank:()=>null});
 const items=[4,7,10,31,72].map((championLevel,i)=>({championId:i+1,championName:`等级 ${championLevel}`,championLevel,championPoints:500-i,markRequiredForNextLevel:2,tokensEarned:1,championPointsSinceLastLevel:200,championPointsUntilNextLevel:800,championSeasonMilestone:1,milestoneGrades:['S'],lastPlayTime:1790075888000}));
 const column=(title,html)=>`<section class="comparison"><h2>${title}</h2><div class="comparison-grid">${html}</div></section>`;
 const details=items.map(item=>`<div data-kind="detail" data-level="${item.championLevel}">${fn.masteryDetailsPortrait(item)}<b>Lv.${item.championLevel}</b></div>`).join('');
 const banners=items.map(item=>`<div data-kind="banner" data-level="${item.championLevel}">${fn.renderSummonerHighlights([],[item])}</div>`).join('');
 const popups=items.map(item=>`<div data-kind="popup" data-level="${item.championLevel}" class="mastery-details-popover">${fn.masteryTooltipContent(item)}</div>`).join('');
 const content=column('总览横幅 · 原结构',banners)+column('熟练度详情 · 等比放大',details)+column('弹窗图标 · 等比缩小',popups);
 server=require('node:http').createServer((req,res)=>{
  if(req.url.startsWith('/api/champion-asset')){
   const asset=new URL(req.url,'http://local').searchParams.get('path');const level=asset.match(/mastery-(\d+)\.png/)[1];res.setHeader('Content-Type','image/png');res.end(fs.readFileSync(path.join(output,`mastery-crest-${level}.png`)));return;
  }
  if(req.url.startsWith('/fixture/')){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="72" height="72"><rect width="72" height="72" fill="#39465b"/><circle cx="36" cy="32" r="23" fill="#b78c45"/><path d="M18 32h8m20 0h8M28 46h16" stroke="#39465b" stroke-width="4"/></svg>');return;}
  res.setHeader('Content-Type','text/html');
  res.end(`<!doctype html><html data-theme="dark"><meta charset="utf-8"><style>${read('app.css')}\n${read('gameplay.css')}</style><style>body{margin:0;padding:24px}main{max-width:1200px;margin:auto}h2{font-size:18px;margin:12px 0}.comparison{margin-bottom:24px}.comparison-grid{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:12px}.comparison-grid>div{display:flex;align-items:center;flex-direction:column;gap:12px;min-width:0}.summoner-highlight{margin:0}.mastery-details-popover{position:static;width:auto;max-width:none;font-size:11px;padding:8px}.mastery-details-popover header{display:flex;width:100%;flex-direction:column;gap:6px}.mastery-details-popover header .mastery-details-portrait{flex:0 0 auto}.mastery-details-popover p{margin:6px 0}</style><body><main>${content}<p>头像为布局夹具；徽章为真实客户端资源。</p></main><script>${read('image-queue.js')}</script></body></html>`);
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
 const measurements=[];
 for(const width of [1280,780]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:1020,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/`});
  await evaluate(`new Promise((resolve,reject)=>{const until=Date.now()+5000;const tick=()=>{const imgs=[...document.querySelectorAll('img')];if(imgs.length===30 && imgs.every(img=>img.dataset.imageReady))return resolve();if(Date.now()>until)return reject(Error('images did not load'));setTimeout(tick,20)};tick()})`);await frame();
  const rows=await evaluate(`(()=>[...document.querySelectorAll('[data-kind]')].map(root=>{const icon=root.querySelector('.game-icon'),crest=root.querySelector('.summoner-mastery-crest'),level=root.querySelector('.mastery-details-level'),a=icon.getBoundingClientRect(),b=crest.getBoundingClientRect(),c=level?.getBoundingClientRect();return{kind:root.dataset.kind,level:Number(root.dataset.level),avatarSize:a.width,crestSize:b.width,crestTop:b.top-a.top,avatarZ:getComputedStyle(icon).zIndex,crestZ:getComputedStyle(crest).zIndex,levelTop:c?.top,crestBottom:b.bottom,levelHeight:c?.height,asset:crest.dataset.queuedSrc,visible:getComputedStyle(crest).visibility};}))()`);
  for(const row of rows){assert.equal(row.avatarZ,'2');assert.equal(row.crestZ,'1');assert.equal(row.visible,'visible');assert.ok(Math.abs(row.crestSize-row.avatarSize*42/52)<.03);assert.ok(Math.abs(row.crestTop-row.avatarSize*36/52)<.03);if(row.kind!=='banner')assert.ok(Math.abs(row.levelTop+row.levelHeight/2-row.crestBottom)<.03);assert.match(row.asset,new RegExp(`mastery-${Math.min(row.level,10)}\\.png`));}
  assert.equal(await evaluate('document.documentElement.scrollWidth>innerWidth'),false);
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,`mastery-comparison-${width===1280?'wide':'narrow'}.png`),Buffer.from(shot.data,'base64'));measurements.push({width,rows});
 }
 fs.writeFileSync(path.join(output,'layout.json'),JSON.stringify({fixturePortraits:true,actualClientCrests:[4,7,10],levels:[4,7,10,31,72],ratios:{crestSize:'42/52',crestTop:'36/52'},measurements},null,2)+'\n');
 console.log('R214 mastery banner/detail/popover comparison × five levels × two widths PASS');

}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
