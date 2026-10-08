'use strict';
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
// Production app / synthetic API fixtures. These screenshots do not claim Windows/LCU evidence.
const {spawn}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http'),assert=require('node:assert/strict');
const baseline=false,guardOnly=false;const root=path.resolve(__dirname,'..'),web=path.join(root,'backend/web'),out=process.env.R258_BROWSER_OUT||evidencePath('r258/chromium-batch1-synthetic');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r258-chromium-'));let chrome,ws,server,origin;const errors=[],results=[];
const inject=`
 const r258JSON=value=>Promise.resolve(new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}}));
 Object.assign(status,{connected:false,identityReady:false,sgpReady:false,clientDiscovery:'process-not-found',clientView:{type:'client-view',state:'no-client',generation:1}});
 overview.player.playerRef="player_r258_self";overview.player.serverId="HN1";status.summoner.playerRef=overview.player.playerRef;
 window.__r258SetStatus=next=>{Object.assign(status,next);const ready=status.connected && status.identityReady && status.sgpReady;status.clientView={type:'client-view',state:status.clientDiscovery==='exiting'?'exiting':ready?'ready':'no-client',summoner:status.summoner,region:status.clientRegion,serverId:status.serverId,sgpReady:status.sgpReady,generation:status.clientView.generation+1};for(const stream of window.__r258Streams||[])stream.onmessage?.({data:JSON.stringify(status.clientView)});};
`;
const intercept=`
 if(pathname==='/api/gameplay/overview'){
  window.__r258OverviewReads=(window.__r258OverviewReads||0)+1;
  if(!window.__r258Released)return new Promise(resolve=>window.__r258FinishOverview=()=>{window.__r258Released=true;resolve(new Response(JSON.stringify(overview),{headers:{'Content-Type':'application/json'}}));});
  return r258JSON(overview);
 }
`;
async function main(){
 fs.mkdirSync(out,{recursive:true});if(fs.readdirSync(out).length)throw Error('R258 screenshot output must be empty');
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
  await call('Page.enable');await call('Page.addScriptToEvaluateOnNewDocument',{source:'window.__r258Streams=[];window.EventSource=class {constructor(){window.__r258Streams.push(this)}addEventListener(){}close(){}};try { localStorage.clear(); } catch (_) {} window.__r241LongTasks=[];new PerformanceObserver(list=>window.__r241LongTasks.push(...list.getEntries().map(e=>({start:e.startTime,duration:e.duration})))).observe({type:"longtask",buffered:true});'});await call('Runtime.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
  const until=async expression=>{for(let i=0;i<200;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('timeout '+expression)};
  const shot=async name=>{if(guardOnly)return;const s=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,theme+'-'+name+'.png'),Buffer.from(s.data,'base64'))};
  await call('Page.navigate',{url:origin+'/?demo&section=overview'});
  await until(`window.__r258SetStatus && document.querySelector('#client-launchpad:not([hidden])')`);
  await evaluate(`document.documentElement.dataset.theme='${theme}'`);
  return{theme,call,evaluate,until,shot};
 }

 for(const theme of ['dark','light']) {
  const p=await page(theme);
  const unblocked=async()=>assert(await p.evaluate("document.querySelector('#startup-loading').hidden && !document.querySelector('#app-frame').hasAttribute('inert')"));
  await unblocked();
  for(const clientDiscovery of ['credentials-unreadable','probe-failed']) {
   await p.evaluate(`window.__r258SetStatus({connected:false,clientDiscovery:${JSON.stringify(clientDiscovery)}})`);
   await new Promise(resolve=>setTimeout(resolve,100));await unblocked();
  }
  await p.evaluate("window.__r258SetStatus({connected:true,identityReady:true,sgpReady:false})");
  await new Promise(resolve=>setTimeout(resolve,100));await unblocked();assert(await p.evaluate("!document.querySelector('[data-player-tab-wrap=\"current\"]')"));
  await p.evaluate("window.__r258SetStatus({connected:true,identityReady:true,sgpReady:true,clientDiscovery:'connected'})");
  await p.until("document.querySelector('.summoner-strip h2') && window.__r258FinishOverview");await unblocked();
  assert(await p.evaluate("document.querySelector('[data-player-tab-wrap=\"current\"]') && document.querySelector('.gameplay-skeleton')"));
  await p.shot('header-before-network-synthetic');
  await p.evaluate("window.__r258FinishOverview()");await p.until("document.querySelector('.match-list .match-entry')");
  await p.evaluate("window.__r258List=document.querySelector('.match-list');window.__r258Reads=window.__r258OverviewReads");
  await p.shot('first-card-synthetic');

  if(process.env.R258_FILTER_SCREENSHOTS==='1') {
   await p.until("document.querySelector('[data-af-open]')");
   await p.evaluate(`document.querySelector('[data-af-open]').click();document.querySelector('[data-af-category="result"]').click();document.querySelector('[data-af-option="win"]').click();`);
   await p.until("document.querySelector('[data-af-remove=\"result\"]')");
   await p.evaluate(`document.querySelector('[data-af-category="result"][data-af-remove]')?.click();document.querySelector('[data-af-add]').click();document.querySelector('[data-af-category="hero"]').click();document.querySelector('[data-af-option="164"]').click();`);
   await p.until("document.querySelector('[data-af-remove=\"hero\"]')");
   await p.shot('hero-menu-synthetic');
   await p.evaluate(`document.querySelector('[data-af-add]').click();document.querySelector('[data-af-category="multikill"]').click();document.querySelector('[data-af-option="3"]').click();const checkbox=document.querySelector('[data-af-atleast]');checkbox.checked=true;checkbox.dispatchEvent(new Event('change',{bubbles:true}));document.querySelector('[data-af-open]').click();`);
   await p.until("document.querySelector('[data-af-remove=\"multikill\"]')");
   assert(await p.evaluate("document.querySelectorAll('.match-entry:not([hidden])').length>0 && document.querySelector('.af-hit')"));
   await p.shot('advanced-active-synthetic');
   const menuMetrics=await p.evaluate(`(()=>{document.querySelector('[data-af-open]').click();document.querySelector('[data-af-back]').click();const r=document.querySelector('[data-af-menu]').getBoundingClientRect();document.querySelector('[data-af-open]').click();return {width:r.width,height:r.height}})()`);
   assert.equal(menuMetrics.width,320);assert.equal(menuMetrics.height,440);
   if(theme==='dark') {
    await p.call('Emulation.setDeviceMetricsOverride',{width:900,height:1100,deviceScaleFactor:1,mobile:false});
    await p.evaluate("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");
    const narrow=await p.evaluate(`(()=>{const bar=document.querySelector('.match-filterbar'),caps=document.querySelector('.af-conditionbar'),tabs=document.querySelector('.match-filter-tabs'),tools=document.querySelector('.match-filter-tools');return {viewport:innerWidth,pageWidth:document.documentElement.scrollWidth,barWidth:bar.getBoundingClientRect().width,capsWidth:caps.getBoundingClientRect().width,tabsTop:tabs.getBoundingClientRect().top,toolsTop:tools.getBoundingClientRect().top}})()`);
    assert(narrow.pageWidth<=narrow.viewport);assert(narrow.toolsTop>narrow.tabsTop);assert(narrow.capsWidth<=narrow.barWidth);
    await p.shot('advanced-narrow-synthetic');results.push({theme,kind:'advanced-filter-narrow',...narrow});
    await p.call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});
   }
   results.push({theme,kind:'advanced-filters',categoryCount:3,menuMetrics,matchingLabelsOutlined:true});
  }

  if(process.env.R258_LAYOUT_MEASURE==='1') {
   await p.call('Performance.enable');
   const measured=await p.evaluate(`(()=>{const row=document.querySelector('.match-list .match-entry');return {height:row.getBoundingClientRect().height,width:row.getBoundingClientRect().width}})()`);
   const trials=[];
   const metrics=async()=>Object.fromEntries((await p.call('Performance.getMetrics')).metrics.map(x=>[x.name,x.value]));
   for(let trial=0;trial<4;trial++) for(const mode of trial%2?['auto','visible']:['visible','auto']) {
    await p.evaluate(`(()=>{document.getElementById('r258-layout-fixture')?.remove();void document.body.offsetHeight;const source=document.querySelector('.match-list .match-entry'),fixture=document.createElement('section'),list=document.createElement('div');fixture.id='r258-layout-fixture';fixture.className='matches-column';fixture.style.cssText='position:fixed;top:0;left:0;z-index:100000;width:${measured.width}px;height:700px;overflow:auto;background:var(--bg);contain:layout paint;';list.className='match-list';fixture.append(list);window.__r258Skipped=new Map();for(let i=0;i<200;i++){const row=source.cloneNode(true);row.style.contentVisibility='${mode}';row.style.containIntrinsicSize='${mode==='auto'?'auto '+measured.height+'px':'none'}';row.addEventListener('contentvisibilityautostatechange',e=>window.__r258Skipped.set(i,e.skipped));for(const img of row.querySelectorAll('img')){img.removeAttribute('data-queued-src');img.src='data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="40" height="40"/%3E'}list.append(row)}window.__r258LayoutFixture=fixture})()`);
    const before=await metrics();
    await p.evaluate(`new Promise(resolve=>{const fixture=window.__r258LayoutFixture;document.body.append(fixture);void fixture.offsetHeight;requestAnimationFrame(()=>requestAnimationFrame(resolve))})`);
    const after=await metrics(),counts=await p.evaluate(`(()=>{const root=document.getElementById('r258-layout-fixture'),rows=[...root.querySelectorAll('.match-entry')],skipped=window.__r258Skipped;return {rows:rows.length,totalNodes:root.querySelectorAll('*').length,observedSkippedRows:[...skipped.values()].filter(Boolean).length,observedStateRows:skipped.size,participatingRowNodes:rows.filter((_,i)=>!skipped.get(i)).reduce((n,row)=>n+1+row.querySelectorAll('*').length,0)}})()`);
    assert.equal(counts.rows,200);trials.push({mode,trial,layoutMs:1000*(after.LayoutDuration-before.LayoutDuration),layoutCount:after.LayoutCount-before.LayoutCount,...counts});
    await p.evaluate("document.getElementById('r258-layout-fixture').remove()");
   }
   const median=mode=>{const v=trials.filter(x=>x.mode===mode).map(x=>x.layoutMs).sort((a,b)=>a-b);return (v[1]+v[2])/2};
   results.push({theme,kind:'200-row-layout',measuredRow:measured,trials,medianVisibleMs:median('visible'),medianAutoMs:median('auto'),scope:'Synthetic Chromium CSS comparison; total DOM unchanged, participating nodes determined by actual auto-state events; no Windows claim.'});
  }

  await p.evaluate("window.__r258SetStatus({connected:false,clientDiscovery:'exiting'})");
  await p.until("!document.querySelector('[data-player-tab-wrap=\"current\"]')");await unblocked();await p.shot('closed-synthetic');
  await p.evaluate("window.__r258SetStatus({connected:true,identityReady:true,sgpReady:true,clientDiscovery:'connected'})");
  await p.until("document.querySelector('[data-player-tab-wrap=\"current\"]')");
  assert(await p.evaluate("window.__r258List===document.querySelector('.match-list') && window.__r258Reads===window.__r258OverviewReads"));await unblocked();
  await p.evaluate("window.__r258SetStatus({connected:false,clientDiscovery:'exiting'})");await p.until("!document.querySelector('[data-player-tab-wrap=\"current\"]')");
  for(const clientDiscovery of ['credentials-unreadable','process-not-found']) {await p.evaluate(`window.__r258SetStatus({connected:false,clientDiscovery:${JSON.stringify(clientDiscovery)}})`);await new Promise(resolve=>setTimeout(resolve,100));await unblocked();}
  results.push({theme,startupUnblocked:true,headerBeforeNetwork:true,exitUnblocked:true,restoredSameNodesWithoutHistoryReload:true});
 }
 const summary={scope:'macOS real Chromium; production default HTML/JS with synthetic API data. 未在 Windows 实跑. No real LCU or upstream claims.',results,errors};
 fs.writeFileSync(path.join(out,'chromium-synthetic.json'),JSON.stringify(summary,null,2));assert.equal(errors.length,0,JSON.stringify(errors));console.log(JSON.stringify({results:results.length,errors:errors.length}));
}
main().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{ws?.close();chrome?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
