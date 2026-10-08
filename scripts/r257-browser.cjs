'use strict';
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
// Production app / synthetic API fixtures. These screenshots do not claim Windows/LCU evidence.
const {spawn}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http'),assert=require('node:assert/strict');
const baseline=false,guardOnly=false;const root=path.resolve(__dirname,'..'),web=path.join(root,'backend/web'),out=process.env.R257_BROWSER_OUT||evidencePath('r257/chromium-synthetic');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r257-chromium-'));let chrome,ws,server,origin;const errors=[],results=[];
const inject=`
 overview.player.serverId='HN1';overview.player.playerRef='r257-demo-player';
 overview.ranks=[{queueType:'RANKED_SOLO_5X5',queueLabel:'单双排',wins:0,losses:0,tier:'GOLD',division:'II'},{queueType:'RANKED_FLEX_SR',queueLabel:'灵活组排',wins:150,losses:163,tier:'PLATINUM',division:'IV'}];
 overview.seasonChampionStats=[{...overview.championStats[0],games:208,wins:97,losses:111,winRate:46.6}];overview.seasonOverall={...overview.overall,games:208,wins:97,losses:111,winRate:46.6};
 overview.seasonStatsProgress={season:'S26',complete:true,scanned:208,tableSupported:true,message:'已统计 208 场',upstreamCapped:true,rankedOldestAt:1775889886204,mayhemOldestAt:1778580000000};
 const r257Row=(id,games=208)=>({championId:id,championName:id?'阿狸':'所有英雄',games,wins:Math.round(games*97/208),losses:games-Math.round(games*97/208),winRate:46.6,kda:3.5,kills:7,deaths:3,assists:8,score:7.2,rank:2,kp:.6,damagePerMinute:700,damageShare:.28,tankShare:.2,controlWards:2,cs:180,csPerMinute:7,gold:15000,goldPerMinute:450,doubleKills:8,tripleKills:4,quadraKills:2,pentaKills:1,opponents:[]});
 const r257JSON=value=>Promise.resolve(new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}}));
`;
const intercept=`
 if(pathname==='/api/gameplay/overview'){window.__r257OverviewReads=(window.__r257OverviewReads||0)+1;return r257JSON(overview);}
 if(pathname==='/api/gameplay/champion-table'){const queue=params.get('queue')||'440',games=queue==='mayhem'?70:208;return r257JSON({available:true,queue,season:'S26',complete:true,seasonStatsProgress:overview.seasonStatsProgress,rows:[r257Row(103,games)],overall:r257Row(0,games)});}
 if(pathname==='/api/gameplay/season-summary')return r257JSON({seasonChampionStats:overview.seasonChampionStats,seasonOverall:overview.seasonOverall,seasonStatsProgress:overview.seasonStatsProgress,rankedQueues:{}});
`;
async function main(){
 fs.mkdirSync(out,{recursive:true});if(fs.readdirSync(out).length)throw Error('R257 screenshot output must be empty');
 chrome=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const endpoint=await new Promise((resolve,reject)=>{let output='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);chrome.once('error',reject);chrome.stderr.on('data',c=>{output+=c;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1])}})});
 ws=new WebSocket(endpoint);await new Promise((r,j)=>{ws.addEventListener('open',r,{once:true});ws.addEventListener('error',j,{once:true})});let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method))},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}))});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [r,j]=pending.get(m.id);pending.delete(m.id);m.error?j(Error(JSON.stringify(m.error))):r(m.result)}if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);if(m.method==='Fetch.requestPaused'){const url=m.params.request.url,ok=url.startsWith(origin+'/')||url.startsWith('data:');void send(ok?'Fetch.continueRequest':'Fetch.failRequest',{requestId:m.params.requestId,...(ok?{}:{errorReason:'BlockedByClient'})},m.sessionId).catch(()=>{})}});
 server=http.createServer((req,res)=>{const url=new URL(req.url,'http://fixture'),name=url.pathname;
  if(name==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream'});res.write('event: heartbeat\ndata: {}\n\n');const timer=setInterval(()=>res.write('event: heartbeat\ndata: {}\n\n'),2000);req.on('close',()=>clearInterval(timer));return}
  if(name==='/api/license/status'){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({state:'ACTIVE',generation:1,message:''}));return}
  if(name.startsWith('/api/')){const asset=name.includes('image')||name.includes('asset');res.writeHead(200,{'Content-Type':asset?'image/svg+xml':'application/json'});res.end(asset?'<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#587c83"/><path d="M8 32 20 8 32 32Z" fill="#c8af67"/></svg>':'{}');return}
  const relative=name==='/'?'index.html':name.slice(1),file=path.join(web,['index.html','license-ui.js'].includes(relative)?'default/'+relative:relative);if(relative.includes('..')||!fs.existsSync(file)){res.writeHead(404);res.end();return}let data=baseline && ['gameplay.js','gameplay.css'].includes(relative) ? fs.readFileSync(path.join(out,'baseline/backend/web',relative)) : fs.readFileSync(file);
  if(relative==='demo-data.js')data=Buffer.from(data.toString().replace('  const fixtures = new Map([',inject+'\n  const fixtures = new Map([').replace('    const params = new URLSearchParams(url.split("?")[1] || "");','    const params = new URLSearchParams(url.split("?")[1] || "");'+intercept));
  res.writeHead(200,{'Content-Type':({'.js':'text/javascript','.css':'text/css','.html':'text/html','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store'});res.end(data);
 });await new Promise(r=>server.listen(0,'127.0.0.1',r));origin='http://127.0.0.1:'+server.address().port;
 async function page(theme,ratingFail=false,foreign=false,arena=false,startup=false){const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Page.addScriptToEvaluateOnNewDocument',{source:'try { localStorage.clear(); } catch (_) {} window.__r241LongTasks=[];new PerformanceObserver(list=>window.__r241LongTasks.push(...list.getEntries().map(e=>({start:e.startTime,duration:e.duration})))).observe({type:"longtask",buffered:true});'});await call('Runtime.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
  const until=async expression=>{for(let i=0;i<200;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('timeout '+expression)};
  const shot=async name=>{if(guardOnly)return;const s=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,theme+'-'+name+'.png'),Buffer.from(s.data,'base64'))};
  await call('Page.navigate',{url:origin+'/?demo&section=overview'+(ratingFail?'&ratingFail=1':'')+(foreign?'&foreign=1':'')+(arena?'&arena235=1':'')+(startup?'&startup235=1':'')});await until(startup ? `!document.querySelector('#startup-loading').hidden` : `document.querySelector('[data-overview-subpage="champion-table"]') && !document.querySelector('[data-section-loading]')`).catch(async error=>{console.log(JSON.stringify({errors,body:await evaluate('document.body.innerText'),scripts:await evaluate('[...document.scripts].map(s=>s.src)')}));await shot('failure');throw error});await evaluate(`document.documentElement.dataset.theme='${theme}';window.__r257RootNodes=['.career-column','.match-list','.summoner-strip'].map(s=>document.querySelector(s));window.__r257Reads=window.__r257OverviewReads;`);return{theme,call,evaluate,until,shot};
 }

 for(const theme of ['dark','light']) {
  const p=await page(theme);
  await p.call('Emulation.setTimezoneOverride',{timezoneId:'Asia/Shanghai'});
  const hover=async selector=>{
   await p.evaluate(`document.querySelector(${JSON.stringify(selector)}).scrollIntoView({block:'center',behavior:'instant'})`);
   await p.evaluate('new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))');
   const rect=await p.evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return {x:r.left+r.width/2,y:r.top+r.height/2}})()`);
   await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:0,y:0});
   await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:rect.x,y:rect.y});
   await p.until(`document.querySelector('.season-data-note-popover:not([hidden])')`).catch(async e=>{await p.shot('failure-synthetic');throw e;});
  };
  assert(await p.evaluate("!document.querySelector('.champion-performance .season-data-note')"));
  await p.shot('overview-no-note-synthetic');
  await p.evaluate("document.querySelector('.champion-performance [data-overview-subpage]').click()");
  await p.until("document.querySelector('.champion-data-table') && document.querySelector('[data-table-queue=\"440\"].is-active')");
  await hover('.overview-detail-page .season-data-note');
  const alignment=await p.evaluate("(()=>{const title=document.querySelector('.champion-table-title h2').getBoundingClientRect(),icon=document.querySelector('.champion-table-title .season-data-note').getBoundingClientRect();return {gap:icon.left-title.right,centerDifference:Math.abs((icon.top+icon.bottom-title.top-title.bottom)/2)}})()");assert.equal(alignment.gap,8);assert(alignment.centerDifference<1);
  const flex=await p.evaluate("document.querySelector('.season-data-note-popover').textContent");assert.match(flex,/4月11日/);assert.match(flex,/已统计 208 场 · 官方总场次 313 场/);
  await p.shot('flex-note-synthetic');
  await p.evaluate("document.querySelector('[data-table-queue=\"mayhem\"]').click()");await p.until("document.querySelector('[data-table-queue=\"mayhem\"].is-active')");
  await hover('.overview-detail-page .season-data-note');
  const mayhem=await p.evaluate("document.querySelector('.season-data-note-popover').textContent");assert.match(mayhem,/5月12日/);assert.match(mayhem,/已统计 70 场/);assert.doesNotMatch(mayhem,/官方总场次/);
  await p.shot('mayhem-note-synthetic');
  await p.call('Emulation.setDeviceMetricsOverride',{width:480,height:850,deviceScaleFactor:1,mobile:false});
  await hover('.overview-detail-page .season-data-note');
  const placement=await p.evaluate("(()=>{const b=document.querySelector('.overview-detail-page .season-data-note').getBoundingClientRect(),p=document.querySelector('.season-data-note-popover').getBoundingClientRect();return {button:{left:b.left,right:b.right},panel:{left:p.left,right:p.right},width:innerWidth}})()");
  assert(placement.panel.left<placement.button.left);assert(placement.panel.right<=placement.width-11);await p.shot('right-edge-left-synthetic');
  await p.call('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});assert(await p.evaluate("document.querySelector('.season-data-note-popover').hidden"));
  await p.evaluate("document.querySelector('[data-overview-return]').focus()");
  await p.call('Input.dispatchKeyEvent',{type:'keyDown',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});
  assert(await p.evaluate("document.activeElement.matches('.season-data-note') && !document.querySelector('.season-data-note-popover').hidden"));
  results.push({theme,alignment,flex,mayhem,placement,hover:true,escape:true,focus:true});
 }
 const summary={scope:'macOS real Chromium; production default HTML/JS with synthetic API data. 未在 Windows 实跑. No real LCU or upstream claims.',results,errors};
 fs.writeFileSync(path.join(out,'chromium-synthetic.json'),JSON.stringify(summary,null,2));assert.equal(errors.length,0,JSON.stringify(errors));console.log(JSON.stringify({results:results.length,errors:errors.length}));
}
main().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{ws?.close();chrome?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
