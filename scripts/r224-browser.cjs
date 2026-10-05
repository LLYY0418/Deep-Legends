'use strict';
// R224 offline Chromium regression: synthetic demo fixtures, never Windows/LCU evidence.
const {spawn}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),http=require('node:http');
const root=path.resolve(__dirname,'..'),web=path.join(root,'backend/web'),out=path.join(root,'docs/history/reports/r224',process.env.R224_SCREENSHOTS_ONLY==='1'?'visual':'');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r224-browser-'));let chrome,ws,server;let origin='';const modeResults=[],errors=[];
const injectDemo=`
 const r224Mode=query.get('r224') || 'rift';
 function r224Live() {
  const arena=r224Mode==='arena',data=structuredClone(arena ? arenaFullLive : r224Mode==='hextech' ? hextechLive : live);
  data.phase='InProgress';data.gameId=22401;data.champSelectNotice='';data.arenaMySquadNotice='';
  if(r224Mode==='aram') {data.queueId=450;data.gameMode='ARAM';data.mapId=12;}
  if(r224Mode==='practice') {data.queueId=0;data.gameMode='PRACTICETOOL';data.mapId=11;data.players=data.players.slice(0,1);}
  if(r224Mode==='urf') {data.queueId=900;data.gameMode='URF';}
  data.players.forEach((p,i)=>{p.playerRef='r224-fixture-'+i;p.championLocked=true;p.historyState=p.historyState || 'ok';if(arena){p.teamId=100;p.mySquad=[0,4,11].includes(i);}});
  data.arenaGrouped=false;data.arenaMascotMapping=false;
  if(!arena){data.players[0].premadeGroup='1';data.players[0].premadeSize=2;data.players[0].premadeSource='session';data.players[0].proPlayer={playerId:'fixture-pro',playerName:'测试职业',teamName:'测试队',teamCode:'TEST'};if(data.players[1]){data.players[1].premadeGroup='1';data.players[1].premadeSize=2;data.players[1].premadeSource='session';data.players[1].privateHistory=true;data.players[1].autofill=true;}}
  if(window.__r224Step===1) data.arenaMySquadNotice='小队数据尚未完整，正在自动重试';
  if(arena && window.__r224Step===2) { data.players.reverse();data.players.push({...data.players.find(p=>p.isCurrent),championId:999}); }
  return data;
 }
`;
async function main(){
 fs.mkdirSync(out,{recursive:true});
 chrome=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const endpoint=await new Promise((resolve,reject)=>{let output='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);chrome.once('error',reject);chrome.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});});
 ws=new WebSocket(endpoint);await new Promise((resolve,reject)=>{ws.addEventListener('open',resolve,{once:true});ws.addEventListener('error',reject,{once:true});});let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method))},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [r,j]=pending.get(msg.id);pending.delete(msg.id);msg.error?j(Error(JSON.stringify(msg.error))):r(msg.result)}if(msg.method==='Runtime.exceptionThrown')errors.push(msg.params.exceptionDetails);if(msg.method==='Fetch.requestPaused'){const url=msg.params.request.url,ok=url.startsWith(origin+'/')||url.startsWith('data:');void send(ok?'Fetch.continueRequest':'Fetch.failRequest',{requestId:msg.params.requestId,...(ok?{}:{errorReason:'BlockedByClient'})},msg.sessionId).catch(()=>{});}});
 server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://fixture'),name=url.pathname;
  if(name==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache','Connection':'keep-alive'});res.write('event: heartbeat\ndata: {}\n\n');const timer=setInterval(()=>res.write('event: heartbeat\ndata: {}\n\n'),2000);req.on('close',()=>clearInterval(timer));return;}
  if(name.startsWith('/api/')){res.writeHead(200,{'Content-Type':name.includes('image')||name.includes('asset')?'image/svg+xml':'application/json'});res.end(name.includes('image')||name.includes('asset')?'<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#567478"/></svg>':'{}');return;}
  const relative=name==='/'?'index.html':name.slice(1),file=path.join(web,relative);if(relative.includes('..')||!fs.existsSync(file)){res.writeHead(404);res.end();return;}
  let data=fs.readFileSync(file);
  if(relative==='demo-data.js')data=Buffer.from(data.toString().replace('  const fixtures = new Map([',injectDemo+'\n  const fixtures = new Map([').replace('["/api/gameplay/phase", () => ({ phase: "None" })]','["/api/gameplay/phase", () => ({ phase: "InProgress", gameId:22401 })]').replace('["/api/gameplay/live", () => hextechLiveDemo ? hextechLive : arenaFullDemo ? arenaFullLive : arenaLiveDemo ? arenaLive : live]','["/api/gameplay/live", r224Live]'));
  if(relative==='runtime.js')data=Buffer.from(data.toString()+`\n{const original=window.reportFlowDiagnostic;window.__r224Events=[];window.reportFlowDiagnostic=(event,reason,fields)=>{window.__r224Events.push({event,reason,...fields,at:new Date().toISOString()});return original(event,reason,fields);};window.__r224Perf=window.deepLegendsPerformance.createRendererPerformance({report:(...args)=>window.reportFlowDiagnostic(...args),snapshot:()=>({domNodes:document.getElementsByTagName('*').length,imgCount:document.images.length}),observe:callback=>{const observer=new PerformanceObserver(list=>callback(list.getEntries()));observer.observe({type:'longtask',buffered:true});return observer;}}); }\n`);
  res.writeHead(200,{'Content-Type':({'.js':'text/javascript','.css':'text/css','.html':'text/html','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store'});res.end(data);
 });await new Promise(r=>server.listen(0,'127.0.0.1',r));origin='http://127.0.0.1:'+server.address().port;
 async function page(mode,section='live'){
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Runtime.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width:1600,height:1100,deviceScaleFactor:1,mobile:false});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
  const until=async expression=>{for(let i=0;i<150;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100));}console.error(JSON.stringify(await evaluate(`({text:document.querySelector('#live-content')?.textContent,events:window.__r224Events.slice(-12),errors:window.__r224Errors,scripts:[...document.scripts].map(s=>s.src)})`)));throw Error('timeout '+mode+': '+expression);};
  await call('Page.navigate',{url:`${origin}/?demo=arena-full&r224=${mode}&section=${section}`});await until(`document.body.classList.contains('is-demo') && !document.querySelector('[data-section-loading]') && document.querySelector('#app-frame')?.inert===false`);
  if(section==='live'){await evaluate(`document.querySelector('#live-refresh').click()`);await until(`document.querySelector('[data-live-player-row]')`);await evaluate(`document.querySelector('[data-recommendation-tab="insight"]').click()`);await until(`document.querySelector('#recommendation-panel-insight')?.hidden===false`);}
  const screenshot=async name=>{const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,name+'.png'),Buffer.from(shot.data,'base64'));};
  return {targetId,mode,call,evaluate,until,screenshot};
 }
 const pages=[];
 for(const mode of ['rift','hextech','aram','arena','practice','urf']){
  const p=await page(mode);pages.push(p);
  await p.evaluate(`window.__r224InitialBody=document.querySelector('[data-live-body]');window.__r224Images=[...document.querySelectorAll('#live-content img')];window.__r224Rows=[...document.querySelectorAll('[data-live-player-row]')];window.__r224InitialKeys=window.__r224Rows.map(r=>r.dataset.livePlayerRow);window.__r224InitialFull=window.__r224Events.filter(e=>e.event==='live_render_rebuild').reduce((n,e)=>n+(e.counts?.full||0),0);`);
  if(mode==='rift'||mode==='arena')await p.screenshot(mode+'-chips');
  for(const step of [1,0]){await p.evaluate(`window.__r224Step=${step};document.querySelector('#live-refresh').click()`);await p.until(step?`document.querySelector('.arena-my-squad-notice')`:`!document.querySelector('.arena-my-squad-notice')`);}
  assert.equal(await p.evaluate(`document.querySelector('[data-live-body]')===window.__r224InitialBody`),true,mode+' notice remount');
  assert.equal(await p.evaluate(`window.__r224Images.every(img=>img.isConnected)`),true,mode+' notice lost images');
  if(mode==='arena'){await p.evaluate(`window.__r224Step=2;document.querySelector('#live-refresh').click()`);await p.until(`document.querySelectorAll('[data-live-player-row^="card:"]').length===18 && window.__r224Events.some(e=>e.event==='live_roster_duplicate_dropped')`);const squad=await p.evaluate(`[...document.querySelectorAll('[data-live-player-row^="card:"]')].slice(0,3).map(row=>({self:!!row.querySelector('.self-chip'),squad:!!row.querySelector('.my-squad-chip')}))`);assert.equal(squad[0].self,true);assert(squad.every(r=>r.squad));await p.screenshot('arena-squad-first');}
  await p.evaluate(`window.__r224StableRows=[...document.querySelectorAll('[data-live-player-row]')];window.__r224StableImages=[...document.querySelectorAll('#live-content img')];window.__r224IdleStart=Date.now();`);
 }
 // Real elapsed three-minute idle observation; all six synthetic modes run concurrently.
 const duration=Number(process.env.R224_BROWSER_IDLE_MS||185000);const untilTime=Date.now()+duration;
 while(Date.now()<untilTime){await new Promise(r=>setTimeout(r,Math.min(10000,untilTime-Date.now())));for(const p of pages){assert.equal(await p.evaluate(`window.__r224StableRows.every(row=>row.isConnected)&&window.__r224StableImages.every(img=>img.isConnected)`),true,p.mode+' idle remount');}}
 for(const p of pages){await p.evaluate(`window.deepLegendsPerformance?.requestMetricsInstalled;`);const result=await p.evaluate(`({elapsedMs:Date.now()-window.__r224IdleStart,rows:document.querySelectorAll('[data-live-player-row^="card:"]').length,events:window.__r224Events,bodyPreserved:document.querySelector('[data-live-body]')===window.__r224InitialBody,imagesPreserved:window.__r224StableImages.every(img=>img.isConnected),rowsPreserved:window.__r224StableRows.every(row=>row.isConnected)})`);modeResults.push({mode:p.mode,...result});fs.writeFileSync(path.join(out,p.mode+'-chromium.json'),JSON.stringify(result,null,2));}
 for(const p of pages.slice(1))await send('Target.closeTarget',{targetId:p.targetId});
 const overview=await page('rift','overview');await overview.until(`document.querySelector('.summoner-name-meta')`);await overview.screenshot('overview-chips');
 const livePage=pages[0];await livePage.evaluate(`document.querySelector('.live-player-name').click()`);await livePage.until(`document.querySelector('#player-overlay')?.hidden===false`);await livePage.screenshot('player-overlay-chips');
 assert.deepEqual(errors,[],'page exceptions');const summary={platform:process.platform,engine:'Headless Chromium',evidence:'synthetic offline demo, not Windows/LCU, no real-game recording',modes:modeResults.map(({mode,elapsedMs,rows,bodyPreserved,imagesPreserved,rowsPreserved,events})=>({mode,elapsedMs,rows,bodyPreserved,imagesPreserved,rowsPreserved,rebuilds:events.filter(e=>e.event==='live_render_rebuild')})),errors};fs.writeFileSync(path.join(out,'chromium.json'),JSON.stringify(summary,null,2));console.log(JSON.stringify(summary));
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();chrome?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
