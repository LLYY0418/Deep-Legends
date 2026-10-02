 'use strict';
// R191 fixture scenes through production renderers and actual Chromium/CSS.
const {spawn}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const {fixture}=require('../backend/web/r190-harness.cjs');
const {read,extract}=require('../backend/web/r188-harness.cjs');
const directory=undefined;
const output=path.resolve(__dirname,'../docs/history/reports/r191');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r191-layout-'));
let proc,ws,server;
async function main(){
 const sharedPath=require('../backend/web/r188-harness.cjs').compile(read('gameplay.js'),['dataDragonRuneShardPath']).dataDragonRuneShardPath;
 const f=fixture(undefined,{dataDragonRuneShardPath:sharedPath,remoteStaticIcon:(_provider,url,name,size)=>`<span class="game-icon is-${size}"><img src="/fixture/${encodeURIComponent(url)}.svg" alt="${name}"></span>`});
 const players=Array.from({length:16},(_,i)=>({...f.subject,participantId:i+1,teamId:i<5?100:200,subteamId:Math.floor(i/2)+1,placement:Math.floor(i/2)+1,gameName:`测试玩家 ${i+1}`,tagLine:'演示',championId:i+1,championName:'测试英雄',playerRef:`fixture${i+1}`}));
 const scenes={};
 for(const scene of ['ranked']){
  const ids=scene==='ranked'?[]:scene==='mayhem'?[1225,1116,1004,120]:[120,1225,1116,1004,1005,1006];
  f.state.augmentDescriptions.clear();
  ids.forEach((id,i)=>{if(i<2||i>=4)f.state.augmentDescriptions.set(id,{status:'ok',description:'测试效果：获得额外攻击速度，并在攻击时触发额外伤害。此句为布局夹具。'});if(i===3)f.state.augmentDescriptions.set(id,{status:'unavailable'});});
  const subject={...f.subject,augmentIds:ids};
  const match={gameId:190,subjectParticipantId:4,gameMode:scene==='arena'?'CHERRY':'',participants:players.slice(0,scene==='arena'?16:10).map(p=>({...p,subteamId:scene==='arena'?p.subteamId:0}))};
  scenes[scene]=`<div class="fixture-roster">${f.renderMatchPlayers(match)}</div><div class="match-detail">${f.renderBuild(match,subject,{region:'kr'})}</div>`;
 }
 server=require('node:http').createServer((req,res)=>{
  if(req.url.startsWith('/rune-styles/')){res.setHeader('Content-Type','image/svg+xml');res.end(fs.readFileSync(path.resolve(__dirname,'../backend/web'+req.url)));return;}
  if(req.url.startsWith('/fixture/')) {res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><circle cx="32" cy="32" r="30" fill="#b78c45"/><path d="M16 32L32 12 48 32 32 52Z" fill="#ffe3a0"/></svg>');return;}
  const scene=req.url.slice(1)||'ranked';
  res.setHeader('Content-Type','text/html');
  res.end(`<!doctype html><html data-theme="azure"><meta charset="utf-8"><style>${read('app.css')}\n${read('gameplay.css')}</style><style>body{margin:0;padding:24px;overflow:auto}.fixture-roster{display:flex;justify-content:flex-end;margin-bottom:12px}.fixture-roster .match-players{width:390px}.match-detail{margin:0}</style><body>${scenes[scene]}<script>${extract(f.source,'bindRuneEffectLinks')}\nbindRuneEffectLinks(document.body);</script></body></html>`);
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
 for(const scene of ['ranked'])for(const width of [1280,820]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:scene==='ranked'?1400:580,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/${scene}`});
  await evaluate(`new Promise(resolve=>{const tick=()=>document.readyState==='complete'?resolve():setTimeout(tick,20);tick();})`);await frame();
  const m=await evaluate(`(()=>{const box=e=>{const r=e.getBoundingClientRect();return{x:r.x,y:r.y,w:r.width,h:r.height}};return{overflow:document.documentElement.scrollWidth>innerWidth,build:box(document.querySelector('.build-detail')),split:document.querySelector('.rune-split')&&getComputedStyle(document.querySelector('.rune-split')).gridTemplateColumns,tree:document.querySelector('.rune-split-tree')&&box(document.querySelector('.rune-split-tree')),effects:document.querySelector('.rune-effects')&&box(document.querySelector('.rune-effects')),board:document.querySelector('.unified-rune-board')&&box(document.querySelector('.unified-rune-board')),shards:document.querySelector('.shards-line')&&{display:getComputedStyle(document.querySelector('.shards-line')).display,chips:[...document.querySelectorAll('.shard-chip')].map(box),icons:[...document.querySelectorAll('.shard-chip .game-icon')].map(box)},cards:[...document.querySelectorAll('.aug')].map(box),highlight:[...document.querySelectorAll('.match-player-name.is-current-player')].map(e=>({text:e.textContent,color:getComputedStyle(e).color,weight:getComputedStyle(e).fontWeight})),skeletal:document.querySelectorAll('.skel').length,bare:document.querySelectorAll('.aug.is-bare').length};})()`);
  assert.equal(m.overflow,false,JSON.stringify({scene,width,...m}));assert.equal(m.highlight.length,1);assert.equal(m.highlight[0].weight,'700');
  if(scene==='ranked'){assert.equal(m.shards.icons.length,3);assert.ok(m.shards.icons.every(i=>i.w===22&&i.h===22),JSON.stringify(m.shards));if(width===1280){assert.equal(m.shards.display,'grid');assert.ok(Math.abs((m.board.y+m.board.h/2)-(m.tree.y+m.tree.h/2))<1);}else{assert.equal(m.shards.display,'flex');assert.ok(m.shards.chips.every(c=>Math.abs(c.y-m.shards.chips[0].y)<1));}if(width===1280)assert.ok(m.effects.x>m.tree.x+m.tree.w-1);else assert.ok(m.effects.y>=m.tree.y+m.tree.h-1);await evaluate(`document.querySelector('.eff[data-perk-id="9111"]').dispatchEvent(new Event('pointerover',{bubbles:true}))`);assert.equal(await evaluate(`document.querySelectorAll('.rune-option-button.is-linked').length`),1);}
  else{assert.equal(m.cards.length,scene==='mayhem'?4:6);assert.equal(m.skeletal,2);assert.equal(m.bare,1);}
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,`${scene}-${width===1280?'wide':'narrow'}.png`),Buffer.from(shot.data,'base64'));measures.push({scene,width,...m});
 }
 fs.writeFileSync(path.join(output,'layout.json'),JSON.stringify(measures,null,2));console.log('R191 17 ranked wide/narrow Chromium scenes PASS');
}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
