'use strict';
const {spawn}=require('node:child_process');const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const {read,compile,escapeHTML}=require('../backend/web/r188-harness.cjs');const output=path.resolve(__dirname,'../docs/history/reports/r216');const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r216-layout-'));let proc,ws,server;
async function main(){
 const source=read('gameplay.js');const keys=['unstoppable','resilience','unlucky','struggling'];
 const state={matchTimelines:new Map()};const icon=(_kind,id,name='')=>`<span class="game-icon is-small"><span>${id}</span></span>`;
 const fn=compile(source,['renderMatchTags','renderMultiKillTag','matchKeywordMetadata','scorePlacementChip','scoreBadgeChip','scoreRankChip','renderChampionTable','championTableNumber','championTableSortedRows'],{state,escapeHTML,matchTimelineKey:()=> 'one',iconFigure:icon});
 const cards=keys.map((key,i)=>`<article class="match-entry"><div class="match-summary"><div class="match-result-meta"><strong>单排 / 双排</strong><span>胜利 · 28 分钟</span></div><div class="match-main"><div>测试英雄 · 12 / 0 / 8</div><div class="match-build"><div class="match-items">${Array.from({length:7},(_,j)=>`<span class="item-slot is-empty ${j===6?'is-trinket':''}"></span>`).join('')}</div><div class="match-badges">${fn.renderMatchTags({gameId:216+i,result:'win'},{participantId:1,multiKill:2+i%4,keyword:key,deaths:0},{badge:i%2?'SVP':'MVP',rank:1,total:10},{})}</div></div></div><div class="match-roster">双方玩家</div><div class="match-actions">⌄</div></div></article>`).join('');
 const row=(id,games)=>({championId:id,championName:`测试英雄 ${id}`,games,wins:Math.round(games*.65),losses:games-Math.round(games*.65),winRate:65,kills:5.2,deaths:3.1,assists:8.2,kda:4.32,kp:.62,score:6.4,rank:3.6,damagePerMinute:685.3,damageShare:.24,tankShare:.31,controlWards:1.2,wardsPlaced:9.8,wardsKilled:3.1,cs:182.4,csPerMinute:6.6,gold:12512,goldPerMinute:453,doubleKills:12,tripleKills:3,quadraKills:1,pentaKills:0});
 const rows=[row(1,60),row(2,30),row(3,12)];rows[0].opponents=Array.from({length:17},(_,i)=>row(100+i,17-i));const table=fn.renderChampionTable({queue:'ranked',overall:{...row(0,102),championName:'所有英雄'},rows},{});
 server=require('node:http').createServer((req,res)=>{res.setHeader('Content-Type','text/html');res.end(`<!doctype html><html data-theme="dark"><meta charset="utf-8"><style>${read('app.css')}\n${read('gameplay.css')}</style><style>body{margin:0;padding:20px}main{max-width:1200px;margin:auto}h2{font-size:18px;margin:8px 0}.tags-scene .match-entry{margin:7px 0}.tags-scene .game-icon{background:var(--surface-strong)}</style><body><main>${req.url.includes('table')?`<section class="overview-detail-page"><h2>英雄数据表</h2>${table}</section>`:`<h2>战绩标签 · 布局夹具</h2><div class="tags-scene">${cards}</div>`}</main></body></html>`)});await new Promise(r=>server.listen(0,'127.0.0.1',r));
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



 const measurements=[];
 for(const scene of ['tags','table'])for(const width of [1280,780])for(const theme of ['light','dark']) {
  await call('Emulation.setDeviceMetricsOverride',{width,height:scene==='tags'?1600:950,deviceScaleFactor:1,mobile:false});await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/${scene}`});
  await evaluate(`new Promise(resolve=>{const tick=()=>document.readyState==='complete'?resolve():setTimeout(tick,20);tick()})`);
  await evaluate(`document.documentElement.dataset.theme='${theme}';new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
  await evaluate(`Promise.all(document.getAnimations().filter(a=>Number.isFinite(a.effect?.getComputedTiming().endTime)).map(a=>a.finished.catch(()=>{})))`);
  const m=await evaluate(`(()=>{const tags=[...document.querySelectorAll('.match-badges')];const wrap=document.querySelector('.champion-table-wrap');const sticky=document.querySelector('.champion-table-hero');const canvas=document.createElement('canvas');canvas.width=canvas.height=1;const ctx=canvas.getContext('2d');const rgb=color=>{ctx.clearRect(0,0,1,1);ctx.fillStyle=color;ctx.fillRect(0,0,1,1);return [...ctx.getImageData(0,0,1,1).data].slice(0,3).map(v=>v/255)};const luminance=c=>rgb(c).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((s,v,i)=>s+v*[.2126,.7152,.0722][i],0);return{overflow:document.documentElement.scrollWidth>innerWidth,tagHeights:tags.map(e=>e.getBoundingClientRect().height),equipmentWidths:[...document.querySelectorAll('.match-items')].map(e=>e.getBoundingClientRect().width),lightContrast:[...document.querySelectorAll('.match-keyword-tag')].map(e=>{const s=getComputedStyle(e),a=luminance(s.color),b=luminance(s.backgroundColor);return (Math.max(a,b)+.05)/(Math.min(a,b)+.05)}),moreButtonContrast:[...document.querySelectorAll('[data-table-more]')].map(e=>{const s=getComputedStyle(e),a=luminance(s.color),b=luminance(s.backgroundColor);return {text:s.color,background:s.backgroundColor,ratio:(Math.max(a,b)+.05)/(Math.min(a,b)+.05)}}),tableWidth:wrap?.clientWidth,tableScrollWidth:wrap?.scrollWidth,heroPosition:sticky?getComputedStyle(sticky).position:null,combinedVisible:!!wrap&&getComputedStyle(document.querySelector('.table-multi-combined')).display!=='none'};})()`);
  assert.equal(m.overflow,false,JSON.stringify({scene,width,theme,...m}));if(scene==='tags'){assert.equal(m.tagHeights.length,4);assert.ok(m.tagHeights.every(h=>h<=60));assert.ok(m.equipmentWidths.every(w=>w>=210));if(theme==='light')assert.ok(m.lightContrast.every(v=>v>=4.5),JSON.stringify(m.lightContrast))}else {assert.equal(m.moreButtonContrast.length,1);assert.ok(m.moreButtonContrast.every(v=>v.ratio>=4.5),JSON.stringify(m.moreButtonContrast));assert.equal(m.heroPosition,'sticky');assert.equal(m.combinedVisible,width<900);if(width===780)assert.ok(m.tableScrollWidth>m.tableWidth)}
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,`${scene}-${theme}-${width}.png`),Buffer.from(shot.data,'base64'));measurements.push({scene,width,theme,...m});
 }
 fs.writeFileSync(path.join(output,'layout.json'),JSON.stringify({productionRenderers:true,fixtureData:true,measurements},null,2)+'\n');console.log('R216 tags/table × light/dark × wide/780 PASS');
}
main().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
