// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/r95-browser.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r95-browser-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--disable-background-timer-throttling','--disable-renderer-backgrounding','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
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
 await call('Page.bringToFront');
 const output=process.env.R95_BROWSER_OUTPUT||path.join(root,'docs/r95-validation/browser');fs.mkdirSync(output,{recursive:true});
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if(pathname==='/api/image'||pathname==='/api/champion-asset'){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#364558"/><path d="M10 26L20 11L30 26Z" fill="#7790ac"/></svg>');return;}
   if(pathname==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'});res.write(': fixture stream\n\n');return;}
   if(pathname.startsWith('/api/')){res.statusCode=204;res.end();return;}
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream');
   const override=pathname==='/gameplay.css'?process.env.R95_CSS_SOURCE:pathname==='/gameplay.js'?process.env.R95_GAMEPLAY_SOURCE:pathname==='/suite.js'?process.env.R95_SUITE_SOURCE:null;
   let content=fs.readFileSync(override||file);
   if(pathname==='/gameplay.js')content=String(content).replace('window.deepLegendsMatchCards = Object.freeze','window.__r91Gameplay={state,nodes,renderLive,renderLivePlayer,renderInsightMatches,loadLive};\n window.deepLegendsMatchCards = Object.freeze');
   if(pathname==='/suite.js')content=String(content).replace('  setupTabs();','  window.__r91Suite={state,roots,hydrateFacadeDraft,rankLabel,renderFacade,renderChampSelect,loadChampSelect,loadFacade};\n  setupTabs();');
   res.end(content);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1050,deviceScaleFactor:1,mobile:false});
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 const until=async expression=>{const started=Date.now();while(Date.now()-started<15000){if(await evaluate('Boolean('+expression+')'))return;await new Promise(r=>setTimeout(r,50));}throw Error('condition timeout: '+expression);};
 await until('window.__r91Gameplay && window.__r91Suite && document.querySelector(".match-entry")');
 await evaluate(`(async()=>{
 const g=window.__r91Gameplay;
 const style=document.createElement('style');style.textContent='*,*::before,*::after{animation:none!important;transition:none!important}#r95-proof{position:fixed;inset:0;z-index:999999;background:var(--bg,#15191c);padding:20px;overflow:auto}';document.head.append(style);
 const area=document.createElement('main');area.id='r95-proof';document.body.append(area);area.append(g.nodes.liveContent);
 clearTimeout(g.state.liveTimer);g.state.settings.liveRefresh=false;g.state.settings.maskNames=false;g.state.status={connected:true};g.state.section='live';g.state.recommendationTab='insight';g.state.recommendationTabTouched=true;g.state.beacon.phase='ChampSelect';g.state.liveLoading=false;g.state.liveError='';g.state.liveAwaitingGame=false;g.state.livePhaseRefreshQueued=false;g.state.liveGameRefreshQueued=false;
 window.r95Fixture={available:true,phase:'ChampSelect',gameId:95001,queueId:1750,gameMode:'CHERRY',arenaGroupingUnavailable:true,currentChampionId:1,players:Array.from({length:3},(_,i)=>({playerRef:'fixture-'+i,gameName:i?'队友'+i:'HLE Gumayusi',tagLine:'0298',championId:i+1,championName:'安妮',teamId:100,isAlly:true,isCurrent:i===0,mySquad:true,historyState:'ready',modeStats:{games:10,wins:6,losses:4,winRate:60,kda:3},premadeGroup:'1',premadeSize:3,premadeSource:'history',...(i===0?{proPlayer:{playerName:'Gumayusi',teamCode:'HLE',teamName:'Hanwha Life Esports',secondary:false}}:{})}))};
 const original=window.fetch;window.fetch=async function(url,opts){if(String(url).startsWith('/api/gameplay/phase'))return new Response(JSON.stringify({phase:'ChampSelect',gameId:95001}),{headers:{'Content-Type':'application/json'}});if(String(url).startsWith('/api/gameplay/live')){await new Promise(r=>setTimeout(r,900));return new Response(JSON.stringify(window.r95Fixture),{headers:{'Content-Type':'application/json'}});}return original(url,opts)};
 g.state.live=structuredClone(window.r95Fixture);g.state.liveExpectedGameId=95001;g.renderLive();await document.fonts.ready;
 })()`);
 console.log(await evaluate('JSON.stringify({section:window.__r91Gameplay.state.section,live:!!window.__r91Gameplay.state.live,html:window.__r91Gameplay.nodes.liveContent.innerHTML.slice(0,300)})'));
 const layout=await evaluate(`(async()=>{const g=window.__r91Gameplay,values=[],visibility=[];let done=false;const started=performance.now();const sample=()=>{const body=document.querySelector('[data-live-body]');if(!body)throw Error(JSON.stringify({html:g.nodes.liveContent.innerHTML.slice(0,500),error:g.state.liveError,awaiting:g.state.liveAwaitingGame,phase:g.state.beacon.phase,live:g.state.live}));values.push(body.getBoundingClientRect().top);visibility.push(!!document.querySelector('.live-refresh-status'));if(!done)setTimeout(sample,16)};sample();await new Promise(r=>setTimeout(r,100));await g.loadLive(true,'manual');await new Promise(r=>setTimeout(r,Math.max(1000,3100-(performance.now()-started))));done=true;sample();return {values,visibility,span:Math.max(...values)-Math.min(...values)}})()`);
 fs.writeFileSync(path.join(output,'layout.json'),JSON.stringify(layout,null,2));
 assert.ok(layout.values.length>30,'real continuous layout samples');assert.ok(layout.visibility.some(Boolean),'manual slow response displayed status');assert.equal(layout.span,0,'R95 body top changed during 3-second cycle');
 const widths=[];
 for(const width of [1500,1280,900,700,620,619]){
 await call('Emulation.setDeviceMetricsOverride',{width,height:1050,deviceScaleFactor:1,mobile:false});await evaluate('document.fonts.ready');
 const measures=await evaluate(`(()=>{const row=document.querySelector('#r95-proof .live-player.is-self'),identity=row.querySelector('.live-player-identity');const rect=e=>{const r=e.getBoundingClientRect();return {left:r.left,right:r.right,top:r.top,bottom:r.bottom,width:r.width}};const name=identity.querySelector('.live-player-name'),pro=identity.querySelector('.pro-identity-chip'),nameWithBadge=name.clientWidth,nameScroll=name.scrollWidth;pro.remove();const nameWithoutBadge=name.clientWidth;identity.append(pro);return {nameWithBadge,nameWithoutBadge,nameScroll,text:identity.textContent,row:rect(row),identity:rect(identity),badges:[...identity.querySelectorAll('.self-chip,.premade-team-tag,.pro-identity-chip')].map(e=>({text:e.textContent,...rect(e)})),scroll:row.scrollWidth,client:row.clientWidth}})()`);
 assert.equal(measures.nameWithBadge,measures.nameWithoutBadge,'professional badge must not further squeeze the account name');assert.ok(measures.row.width>100&&measures.row.bottom>measures.row.top,'professional row must be visibly laid out');assert.match(measures.text,/自己/);assert.match(measures.text,/预组 ×3/);assert.match(measures.text,/HLE Gumayusi/);assert.ok(measures.scroll<=measures.client+1,JSON.stringify(measures));
 for(const b of measures.badges)assert.ok(b.left>=measures.row.left&&b.right<=measures.row.right,JSON.stringify({width,b,measures}));widths.push({width,...measures});
 await evaluate('document.querySelector("#r95-proof .live-player.is-self").scrollIntoView({block:"center"})');
 const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,`pro-live-${width}.png`),Buffer.from(shot.data,'base64'));
 }
 fs.writeFileSync(path.join(output,'badge-measurements.json'),JSON.stringify(widths,null,2));
 console.log('R95 Chromium PASS: real loadLive, 3.1s continuous body top invariant; professional row at six breakpoints');
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
