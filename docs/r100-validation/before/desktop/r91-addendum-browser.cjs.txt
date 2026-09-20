// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/current-game-layout.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r91-addendum-browser-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const url=await new Promise((resolve,reject)=>{let output=''; const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-1000)}`)));});
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},45000);pending.set(id,[value=>{clearTimeout(timer);resolve(value)},error=>{clearTimeout(timer);reject(error)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 console.log('CDP connected');
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 await call('Page.enable');
 await call('Runtime.enable');
 const output=process.env.R91_BROWSER_OUTPUT||path.join(root,'docs/r91-addendum/browser');fs.mkdirSync(output,{recursive:true});
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if(pathname==='/api/image'||pathname==='/api/champion-asset'){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#364558"/><path d="M10 26L20 11L30 26Z" fill="#7790ac"/></svg>');return;}
   if(pathname.startsWith('/api/')){res.statusCode=204;res.end();return;}
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream');
   const override=pathname==='/gameplay.css'?process.env.R91_ADDENDUM_CSS_SOURCE:pathname==='/gameplay.js'?process.env.R91_ADDENDUM_GAMEPLAY_SOURCE:pathname==='/suite.js'?process.env.R91_ADDENDUM_SUITE_SOURCE:null;
   let content=fs.readFileSync(override||file);
   if(pathname==='/gameplay.js')content=String(content).replace('window.deepLegendsMatchCards = Object.freeze','window.__r91Gameplay={state,nodes,renderLive,renderLivePlayer,renderInsightMatches};\n window.deepLegendsMatchCards = Object.freeze');
   if(pathname==='/suite.js')content=String(content).replace('  setupTabs();','  window.__r91Suite={state,roots,hydrateFacadeDraft,rankLabel,renderFacade,renderChampSelect,loadChampSelect,loadFacade};\n  setupTabs();');
   res.end(content);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1050,deviceScaleFactor:1,mobile:false});
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 const until=async expression=>{const started=Date.now();while(Date.now()-started<15000){if(await evaluate('Boolean('+expression+')'))return;await new Promise(r=>setTimeout(r,50));}throw Error('condition timeout: '+expression);};
 await until(`window.__r91Suite && document.querySelector('.match-entry:has(.match-summary.is-arena)') && document.querySelector('.match-entry:has(.match-summary:not(.is-arena))')`);
 await evaluate(`{const style=document.createElement('style');style.textContent='*,*::before,*::after{animation:none!important;transition:none!important}#r91-proof{position:fixed;inset:0;z-index:999999;background:var(--bg,#15191c);padding:30px;overflow:auto}#r91-pair{display:grid;grid-template-columns:1fr 1fr;gap:24px;align-items:start}';document.head.append(style);const area=document.createElement('main');area.id='r91-proof';area.innerHTML='<h1>R91 · 战绩数值与首行对齐</h1><p>普通战绩与斗魂战绩，使用同一组生产组件和样式。</p><div id="r91-pair"></div>';document.body.append(area);for(const selector of ['.match-entry:has(.match-summary:not(.is-arena))','.match-entry:has(.match-summary.is-arena)']){const column=document.createElement('div');column.className='matches-column';column.append(document.querySelector(selector).cloneNode(true));document.querySelector('#r91-pair').append(column);}}`);
 const cards=[];
 for(const width of [2400,1500,1280]){
   await call('Emulation.setDeviceMetricsOverride',{width,height:750,deviceScaleFactor:1,mobile:false});
   await evaluate('document.fonts.ready');
   const row=await evaluate(`(()=>{const cards=[...document.querySelectorAll('#r91-pair .match-entry')];return cards.map(e=>{const stats=e.querySelector('.match-stats'),first=stats.firstElementChild;return {arena:!!e.querySelector('.is-arena'),top:first.getBoundingClientRect().top,text:stats.textContent,damage:e.querySelector('.match-stat-damage b')?getComputedStyle(e.querySelector('.match-stat-damage b')).color:null,taken:e.querySelector('.match-stat-taken b')?getComputedStyle(e.querySelector('.match-stat-taken b')).color:null,rows:getComputedStyle(stats).gridTemplateRows};});})()`);
   assert.equal(row[0].top,row[1].top,JSON.stringify(row));assert.doesNotMatch(row[1].text,/万/);assert.notEqual(row[1].damage,row[1].taken);cards.push({width,rows:row});
   const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,`cards-${width}.png`),Buffer.from(shot.data,'base64'));
 }
 const facade=await evaluate(`(async()=>{const s=window.__r91Suite;await s.loadFacade(true);s.state.facade.chat={lol:{}};s.hydrateFacadeDraft(true);s.renderFacade();document.querySelector('#r91-proof').innerHTML='<h1>R91 · 缺省段位</h1>'+s.roots.facade.innerHTML;return {label:s.rankLabel({}),tier:s.roots.facade.querySelector('[data-facade-rank="tier"]').value,division:s.roots.facade.querySelector('[data-facade-rank="division"]').value,commitHidden:s.roots.facade.querySelector('[data-facade-commit]').hidden};})()`);
 assert.equal(facade.label,'未定级 · 单双排');assert.equal(facade.tier,'UNRANKED');assert.equal(facade.division,'I');assert.equal(facade.commitHidden,true);
 let shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'facade-default.png'),Buffer.from(shot.data,'base64'));
 const arena=await evaluate(`(async()=>{const s=window.__r91Suite;await s.loadChampSelect(true);s.state.champSelectGroup='arena';s.renderChampSelect();const warning=[...s.roots.champselect.querySelectorAll('.suite-note.is-warning')].map(e=>e.textContent);document.querySelector('#r91-proof').innerHTML='<h1>R91 · 斗魂征召</h1>'+s.roots.champselect.innerHTML;return {warning,text:s.roots.champselect.textContent};})()`);
 assert.deepEqual(arena.warning,[]);assert.doesNotMatch(arena.text,/斗魂禁用环节需真机确认/);
 shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'arena-config.png'),Buffer.from(shot.data,'base64'));
 const live=await evaluate(`(()=>{const g=window.__r91Gameplay,p={playerRef:'fixture',gameName:'测试召唤师',championId:1,championName:'安妮',rank:{tier:'SILVER',division:'I'},historyState:'ok',modeStats:{games:3,wins:2,kda:3},recentGames:[{championId:1,kills:3,deaths:1,assists:4,win:true}]};g.state.liveLoading=false;const a=g.renderLivePlayer(p,0,true)+g.renderInsightMatches(p);g.state.liveLoading=true;const b=g.renderLivePlayer(p,0,true)+g.renderInsightMatches(p);const ordinary=g.renderLivePlayer(p,0,false);document.querySelector('#r91-proof').innerHTML='<h1>R91 · 刷新时保留斗魂战绩</h1><div style="max-width:800px">'+b+'</div>';return {stable:a===b,text:document.querySelector('#r91-proof').textContent,ordinary};})()`);
 assert.equal(live.stable,true);assert.doesNotMatch(live.text,/白银|未定级/);assert.match(live.ordinary,/白银/);
 shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'arena-history.png'),Buffer.from(shot.data,'base64'));
 fs.writeFileSync(path.join(output,'measurements.json'),JSON.stringify({cards,facade,arenaWarningCount:arena.warning.length,live:{stable:live.stable,arenaRankHidden:true,ordinaryRankVisible:true}},null,2)+'\n');
 console.log('R91 Chromium PASS: cards, integer damage, colors, facade defaults, arena warning removed, stable history, ranks');
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
