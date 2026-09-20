// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/r98-browser.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r98-browser-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--disable-background-timer-throttling','--disable-renderer-backgrounding','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const url=await new Promise((resolve,reject)=>{let output=''; const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-1000)}`)));});
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},45000);pending.set(id,[value=>{clearTimeout(timer);resolve(value)},error=>{clearTimeout(timer);reject(error)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 console.log('CDP connected');
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.method==='Runtime.exceptionThrown'||m.method==='Runtime.consoleAPICalled'&&m.params.type==='error')console.log('browser error',JSON.stringify(m.params).slice(0,1800));});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 await call('Page.enable');
 await call('Runtime.enable');
 await call('Page.bringToFront');
 const output=process.env.R98_BROWSER_OUTPUT||path.join(root,'docs/r98-validation/browser');fs.mkdirSync(output,{recursive:true});
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if(pathname==='/api/image'||pathname==='/api/champion-asset'){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#364558"/><path d="M10 26L20 11L30 26Z" fill="#7790ac"/></svg>');return;}
   if(pathname==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'});res.write(': fixture stream\n\n');return;}
   if(pathname.startsWith('/api/')){res.statusCode=204;res.end();return;}
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream');
   const override=pathname==='/gameplay.css'?process.env.R95_CSS_SOURCE:pathname==='/gameplay.js'?process.env.R98_GAMEPLAY_SOURCE:pathname==='/suite.js'?process.env.R95_SUITE_SOURCE:null;
   let content=fs.readFileSync(override||file);
   if(pathname==='/gameplay.js')content=String(content).replace('window.deepLegendsMatchCards = Object.freeze','window.__r98={state,nodes,newTabView,rerenderTab,loadOverview,openPlayerByRiotId};\n window.deepLegendsMatchCards = Object.freeze');
   if(pathname==='/suite.js')content=String(content).replace('  setupTabs();','  window.__r91Suite={state,roots,hydrateFacadeDraft,rankLabel,renderFacade,renderChampSelect,loadChampSelect,loadFacade};\n  setupTabs();');
   res.end(content);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1050,deviceScaleFactor:1,mobile:false});
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 const until=async expression=>{const started=Date.now();while(Date.now()-started<15000){if(await evaluate('Boolean('+expression+')'))return;await new Promise(r=>setTimeout(r,50));}throw Error('condition timeout: '+expression);};
 await until('window.__r98 && document.querySelector(".demo-flag")');

 await evaluate(`(async()=>{window.r98Template=await (await fetch('/api/gameplay/overview?count=20')).json()})()`);
 await evaluate(`(()=>{
 const g=window.__r98,source=window.r98Template;
 clearTimeout(g.state.liveTimer);g.state.settings.liveRefresh=false;g.state.settings.maskNames=false;g.state.settings.matchCount=20;g.state.section='overview';
 const style=document.createElement('style');style.textContent='#r98-proof{position:fixed;inset:0;z-index:999999;background:var(--bg,#15191c);padding:20px;overflow:auto}';document.head.append(style);
 const area=document.createElement('main');area.id='r98-proof';document.body.append(area);area.append(g.nodes.overviewContent);
 const data=structuredClone(source);data.player.region='kr';data.player.gameName='Fixture';data.player.tagLine='KR1';delete data.player.proPlayer;
 data.matches=Array.from({length:20},(_,i)=>({...structuredClone(source.matches[i%source.matches.length]),gameId:i+1}));data.pagination={begIndex:0,count:20,hasMore:false};
 window.r98Base=data;window.r98Tab={...g.newTabView(),key:'r98-fixture',current:false,region:'kr',group:'kr',riotId:{gameName:'Fixture',tagLine:'KR1'},playerRef:data.player.playerRef,data:structuredClone(data),matchFilter:'all',label:'Fixture'};
 g.state.tabs=[window.r98Tab];g.state.activeGroup='kr';g.state.activeTabs.kr='r98-fixture';g.rerenderTab(window.r98Tab);
 window.r98Controllers=[];
 const original=window.fetch;window.fetch=async function(url,opts){if(String(url)==='/api/gameplay/overview'&&opts?.method==='POST')return new Response(new ReadableStream({start(c){window.r98Controllers.push(c)}}),{headers:{'Content-Type':'application/x-ndjson'}});return original(url,opts)};
 window.r98Send=(index,frame)=>window.r98Controllers[index].enqueue(new TextEncoder().encode(JSON.stringify(frame)+'\\n'));
 window.r98Finish=(index)=>window.r98Controllers[index].close();
 window.r98Rows=()=>Array.from(document.querySelectorAll('#r98-proof .match-list > .match-entry'),e=>({id:Number(e.dataset.matchId),top:e.getBoundingClientRect().top,height:e.getBoundingClientRect().height}));
 window.r98Samples=[];window.r98Sampling=true;const sample=()=>{window.r98Samples.push(window.r98Rows());if(window.r98Sampling)requestAnimationFrame(sample)};sample();
 new MutationObserver(()=>window.r98Samples.push(window.r98Rows())).observe(g.nodes.overviewContent,{childList:true,subtree:true});
 })()`);
 const expected=Array.from({length:20},(_,i)=>i+1);
 const snapshot=async name=>{const rows=await evaluate('r98Rows()');assert.deepEqual(rows.map(r=>r.id),expected,name);assert.ok(rows.every(r=>r.height>0),name+' visible rows');assert.ok(rows.every((r,i)=>!i||r.top>rows[i-1].top),name+' visual order');const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,name+'.png'),Buffer.from(shot.data,'base64'));};
 await snapshot('before');
 await evaluate('window.r98Pending=__r98.loadOverview(r98Tab,true);void 0');await until('r98Controllers.length===1');
 await evaluate(`(()=>{const partial=structuredClone(r98Base);partial.matches=[13,4,18,2,11].map(id=>({...partial.matches[id-1],r98Changed:true}));r98Send(0,{type:'progress',overview:partial})})()`);
 await new Promise(r=>setTimeout(r,180));await snapshot('sparse-preview');
 await evaluate('r98Send(0,{type:"complete",overview:r98Base});r98Finish(0);r98Pending');await snapshot('complete');
 const samples=await evaluate('window.r98Sampling=false;r98Samples');
 fs.writeFileSync(path.join(output,'order-samples.json'),JSON.stringify(samples,null,2));
 assert.ok(samples.length>4,'continuous real-render samples');for(const rows of samples)assert.deepEqual(rows.map(r=>r.id),expected,'no intermediate reorder');
 // A real error must undo even the in-place value updates, not just preserve IDs.
 await evaluate('window.r98Before=JSON.stringify(r98Tab.data.matches);window.r98Pending=__r98.loadOverview(r98Tab,true);void 0');await until('r98Controllers.length===2');
 await evaluate(`(()=>{const partial=structuredClone(r98Base);partial.matches=[13,4,18,2,11].map(id=>({...partial.matches[id-1],r98Changed:true}));r98Send(1,{type:'progress',overview:partial})})()`);
 await until('r98Tab.data.matches.some(m=>m.r98Changed)');
 await evaluate('r98Send(1,{type:"error",status:500,error:"fixture failure"});r98Finish(1);r98Pending');
 assert.equal(await evaluate('JSON.stringify(r98Tab.data.matches)===r98Before'),true,'error restores pre-refresh snapshot');await snapshot('error-rollback');
 // Abandoned old preview cannot become the baseline of a replacement request.
 await evaluate('window.r98Old=__r98.loadOverview(r98Tab,true);void 0');await until('r98Controllers.length===3');
 await evaluate(`(()=>{const partial=structuredClone(r98Base);partial.matches=[{...partial.matches[0],r98Changed:true}];r98Send(2,{type:'progress',overview:partial})})()`);await until('r98Tab.data.matches.some(m=>m.r98Changed)');
 await evaluate('window.r98New=__r98.loadOverview(r98Tab,true);void 0');await until('r98Controllers.length===4');
 assert.equal(await evaluate('JSON.stringify(r98Tab.data.matches)===r98Before'),true,'replacement baseline is committed data');
 await evaluate('r98Send(3,{type:"complete",overview:r98Base});r98Finish(3);r98New');
 await evaluate('r98Send(2,{type:"complete",overview:{...r98Base,matches:[]}});r98Finish(2);r98Old');await snapshot('superseded-response');
 // Budget recovery is visible, neutral, and preserves the existing rows.
 await evaluate('window.r98Pending=__r98.loadOverview(r98Tab,true);void 0');await until('r98Controllers.length===5');
 await evaluate('r98Send(4,{type:"error",status:429,kind:"rate-limited",retryAfter:30,error:"额度恢复"});r98Finish(4);r98Pending');
 assert.match(await evaluate('document.querySelector("[data-quota-recovery]")?.textContent||""'),/额度恢复中.*30.*自动重试/);
 await evaluate('clearTimeout(r98Tab.quotaRetry.timer);r98Tab.quotaRetry=null');
 // Search-origin overview promotion, not a directory click with pre-filled pro context.
 await evaluate('window.r98Pending=__r98.loadOverview(r98Tab,true);void 0');await until('r98Controllers.length===6');
 const proFixture=process.env.R98_CHOVY_FIXTURE||path.join(root,'docs/r98-validation/chovy-overview.json');
 const player=JSON.parse(fs.readFileSync(proFixture,'utf8')).player;
 await evaluate(`r98Send(5,{type:'complete',overview:{...r98Base,player:${JSON.stringify(player)}}});r98Finish(5);r98Pending`);
 assert.deepEqual(await evaluate('({group:r98Tab.group,active:__r98.state.activeGroup,team:r98Tab.proTeam,player:r98Tab.proPlayer})'),{group:'pro',active:'pro',team:'GEN',player:'Chovy'});
 assert.match(await evaluate('document.querySelector("#r98-proof .pro-identity-chip")?.textContent||document.querySelector("#r98-proof")?.textContent||""'),/GEN.*Chovy/);
 await snapshot('chovy-search-promotion');
 fs.writeFileSync(path.join(output,'result.json'),JSON.stringify({passed:true,samples:samples.length,errorRollback:true,supersededResponse:true,quotaVisible:true,chovySearchPromotion:true},null,2));
 console.log('R98 Chromium PASS: continuous order, value rollback, replacement, visible quota recovery, Chovy search promotion');
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
