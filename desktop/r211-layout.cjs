'use strict';
const { evidencePath, requireEvidence } = require('../scripts/local-evidence.cjs');
// R211 production renderers/CSS in Chromium; fixture portraits, real 108px mastery crest.
const {spawn}=require('node:child_process');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),assert=require('node:assert/strict');
const {fixture}=require('../backend/web/r190-harness.cjs');
const {read,extract,compile,escapeHTML}=require('../backend/web/r188-harness.cjs');
const output=evidencePath('r211');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r211-layout-'));
let proc,ws,server;
async function main(){
 requireEvidence('r211/topking-opgg-masteries-parsed.json', 'r211/topking-masteries.json', 'r211/mastery-crest-10.png');
 const source=read('gameplay.js');
 const icon=(_kind,id,name='')=>`<span class="game-icon"><img src="/fixture/${id}.svg" alt="${escapeHTML(name)}"></span>`;
 const route=compile(source,['renderItemRoute','renderSkillOrder','skillPrioritySummary'],{escapeHTML,number:String,itemIconFigure:id=>icon('item',id),SKILL_SLOT_LETTERS:{1:'Q',2:'W',3:'E',4:'R'}});
 const f=fixture(source,route);
 const extra=compile(source,['masteryDetailsPortrait','masteryMarks','masteryTooltipContent','renderMasteryDetails','renderOverviewStreak'],{number:value=>Number(value).toLocaleString('en-US'),escapeHTML,iconFigure:icon});
 const players=Array.from({length:16},(_,i)=>({...f.subject,participantId:i+1,teamId:i<5?100:200,subteamId:Math.floor(i/2)+1,placement:Math.floor(i/2)+1,position:['top','jungle','middle','bottom','utility'][i%5],gameName:`测试玩家 ${i+1}`,tagLine:'演示',championId:i+1,championName:'测试英雄',championLevel:18,playerRef:`fixture${i+1}`}));
 const scenes={};
 for(const scene of ['ranked','mayhem','arena']){
  const ids=scene==='ranked'?[]:scene==='mayhem'?[1225,1116]:[120,1225,1116,1004];
  ids.forEach(id=>f.state.augmentDescriptions.set(id,{status:'ok',description:'布局验证夹具：额外攻击速度与攻击效果。'}));
  const match={gameId:211,subjectParticipantId:4,gameMode:scene==='arena'?'CHERRY':'',participants:players.slice(0,scene==='arena'?16:10).map(p=>({...p,augmentIds:ids,subteamId:scene==='arena'?p.subteamId:0}))};
  f.state.matchTimelines.set('fixture',{available:true,participants:match.participants.map(p=>({participantId:p.participantId,itemGroups:[{minute:1,events:[{itemId:1055}]},{minute:7,events:[{itemId:3006}]},{minute:12,events:[{itemId:3031}]}],skillOrder:Array.from({length:18},(_,i)=>({slot:i%6===5?4:i%3+1,level:i+1}))}))});
  const tab={region:'kr',buildPlayers:new Map()};
  scenes[scene]=`<h2>${scene==='ranked'?'单双排':scene==='mayhem'?'海克斯大乱斗':'斗魂竞技场'} · 构建</h2><div class="match-detail">${f.renderBuild(match,match.participants[3],tab)}</div>`;
 }
 const names=new Map(JSON.parse(fs.readFileSync(path.join(output,'topking-opgg-masteries-parsed.json'),'utf8')).map(p=>[p.champion_id,p.champion_name]));
 const items=JSON.parse(fs.readFileSync(path.join(output,'topking-masteries.json'),'utf8')).map(p=>({...p,championName:names.get(p.championId)||String(p.championId)}));
 scenes.masteries=`<section class="overview-detail-page"><header><button class="text-button">‹ 返回总览</button><h2>英雄熟练度</h2></header>${extra.renderMasteryDetails({items,totalScore:1130,totalPoints:items.reduce((s,p)=>s+p.championPoints,0),totalChampions:173})}</section>`;
 scenes.banner=`<div class="summoner-strip-copy"><div><h2 class="summoner-copy-name" data-copy-summoner="测试玩家#KR1">测试玩家</h2><span class="summoner-name-meta">#KR1 ${extra.renderOverviewStreak({result:'win',count:5})}</span></div></div><br><div class="summoner-strip-copy"><div><h2>隐藏玩家</h2><span class="summoner-name-meta">${extra.renderOverviewStreak({result:'loss',count:3})}</span></div></div>`;
 server=require('node:http').createServer((req,res)=>{
  if(req.url.startsWith('/rune-styles/')){res.setHeader('Content-Type','image/svg+xml');res.end(fs.readFileSync(path.resolve(__dirname,'../backend/web'+req.url)));return;}
  if(req.url.startsWith('/api/champion-asset')){res.setHeader('Content-Type','image/png');res.end(fs.readFileSync(path.join(output,'mastery-crest-10.png')));return;}
  if(req.url.startsWith('/fixture/')){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="72" height="72"><rect width="72" height="72" rx="8" fill="#39465b"/><path d="M15 40L36 12 57 40 36 63Z" fill="#b78c45"/></svg>');return;}
  const scene=req.url.slice(1)||'ranked';
  res.setHeader('Content-Type','text/html');
  const binding=`const number=value=>Number(value).toLocaleString('en-US');const escapeHTML=${escapeHTML.toString()};const iconFigure=${icon.toString()};${['masteryDetailsPortrait','masteryMarks','masteryTooltipContent','bindMasteryDetails','bindSummonerCopy'].map(name=>extract(source,name)).join('\n')}\nconst prepareImages=root=>{};if(${JSON.stringify(scene)}==='masteries')bindMasteryDetails(document,${JSON.stringify({items})});bindSummonerCopy(document);`;
  res.end(`<!doctype html><html data-theme="dark"><meta charset="utf-8"><style>${read('app.css')}\n${read('gameplay.css')}</style><style>body{margin:0;padding:32px;overflow:auto}main{max-width:1200px;margin:auto}.match-detail{margin:0}@media(max-width:800px){.match-detail{max-width:560px;margin:auto}}h2{margin:0 0 24px}.summoner-strip-copy>div{display:flex;gap:8px}</style><body><main>${scenes[scene]}</main><script>${read("image-queue.js")}</script><script>${binding}</script></body></html>`);
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
 for(const scene of ['ranked','mayhem','arena','masteries','banner'])for(const width of [1280,780]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:scene==='ranked'?1250:900,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/${scene}`});
  await evaluate(`new Promise(resolve=>{const tick=()=>document.readyState==='complete'?resolve():setTimeout(tick,20);tick();})`);await frame();
  if(scene==='masteries')await evaluate(`new Promise((resolve,reject)=>{const deadline=Date.now()+5000;const tick=()=>{const image=document.querySelector('.mastery-details-crest');if(image?.dataset.imageReady)return resolve(true);if(Date.now()>deadline)return reject(Error('crest not visible'));setTimeout(tick,20)};tick()})`);
  const m=await evaluate(`(()=>{const selector=document.querySelector('.build-player-selector'),rows=[...document.querySelectorAll('.build-player-group')],grid=document.querySelector('.mastery-details-grid'),level=document.querySelector('.mastery-details-level'),crest=document.querySelector('.mastery-details-crest');return{overflow:document.documentElement.scrollWidth>innerWidth,players:document.querySelectorAll('[data-build-player]').length,selectorWidth:selector?.getBoundingClientRect().width,groupRows:[...new Set(rows.map(e=>Math.round(e.getBoundingClientRect().top)))].length,masteryColumns:grid?getComputedStyle(grid).gridTemplateColumns.split(' ').length:null,levelWidth:level?.getBoundingClientRect().width,crestWidth:crest?.getBoundingClientRect().width,crestNaturalWidth:crest?.naturalWidth,crestVisible:crest?getComputedStyle(crest).visibility:null,crestZ:crest?getComputedStyle(crest).zIndex:null,masteryCount:document.querySelectorAll('[data-mastery-detail]').length,copyCursor:getComputedStyle(document.querySelector('.summoner-copy-name')||document.body).cursor};})()`);
  assert.equal(m.overflow,false,JSON.stringify({scene,width,...m}));
  if(['ranked','mayhem'].includes(scene)){assert.equal(m.players,10);assert.equal(m.groupRows,1)}
  if(scene==='arena'){assert.equal(m.players,16);assert.equal(m.groupRows,width===1280?1:2)}
  if(scene==='masteries'){assert.equal(m.masteryCount,167);assert.ok(Math.abs(m.crestWidth-56*42/52)<.02);assert.equal(m.crestNaturalWidth,108);assert.equal(m.crestVisible,"visible");assert.equal(m.crestZ,"1");assert.ok(m.levelWidth>=30);if(width===780)assert.equal(m.masteryColumns,5)}
  if(scene==='banner')assert.equal(m.copyCursor,'pointer');
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,`${scene}-${width===1280?'wide':'narrow'}.png`),Buffer.from(shot.data,'base64'));measurements.push({scene,width,...m});
  if(scene==='masteries' && width===780){
   await evaluate(`document.querySelector('[data-mastery-detail]').focus()`);await frame();
   const popup=await evaluate(`(()=>{const p=document.querySelector('.mastery-details-popover'),r=p.getBoundingClientRect(),icon=p.querySelector('.game-icon');return{left:r.left,right:r.right,top:r.top,bottom:r.bottom,portraitWidth:icon.getBoundingClientRect().width,portraitRadius:getComputedStyle(icon).borderRadius,crestCount:p.querySelectorAll('.mastery-details-crest').length,text:p.textContent};})()`);
   assert.ok(popup.left>=8 && popup.right<=772 && popup.top>=8 && popup.bottom<=892);assert.equal(popup.portraitWidth,40);assert.equal(popup.portraitRadius,'50%');assert.equal(popup.crestCount,1);assert.match(popup.text,/3,246 \/ 11,000 点/);
   const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'mastery-popover-narrow.png'),Buffer.from(shot.data,'base64'));measurements.push({scene:'mastery-popover',width,...popup});
  }
 }
 await evaluate(`document.documentElement.dataset.theme='light'`);await frame();const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'banner-light.png'),Buffer.from(shot.data,'base64'));
 fs.writeFileSync(path.join(output,'layout.json'),JSON.stringify({fixturePortraits:true,crest:'real 108x108 rendered at the banner 42/52 ratio',championTable:'not implemented: score gate failed',keywordPalette:'not implemented: score gate failed',measurements},null,2)+'\n');
 console.log('R211 Chromium 5 independent scenes × 2 widths PASS');
}
main().catch(error=>{console.error(error);process.exitCode=1}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
