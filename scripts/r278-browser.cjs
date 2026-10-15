'use strict';
const {cleanupChromeTest}=require('./chrome-test-cleanup.cjs');
const {launchChrome}=require('./chrome-test-launcher.cjs');
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
// Production app / synthetic API fixtures. These screenshots do not claim Windows/LCU evidence.
const {spawn}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http'),assert=require('node:assert/strict');
const baseline=false,guardOnly=false;const root=path.resolve(__dirname,'..'),web=path.join(root,'backend/web'),out=process.env.R278_BROWSER_OUT||evidencePath('r278/chromium-synthetic');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r278-chromium-'));let chrome,ws,server,origin;const errors=[],results=[];let noticeRevision="1";
const inject=`
 const r258JSON=value=>Promise.resolve(new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}}));
 Object.assign(status,{connected:false,identityReady:false,sgpReady:false,clientDiscovery:'process-not-found',clientView:{type:'client-view',state:'no-client',generation:1}});
 overview.player.playerRef="player_r258_self";overview.player.serverId="HN1";status.summoner.playerRef=overview.player.playerRef;
 window.__r258SetStatus=next=>{Object.assign(status,next);const ready=status.connected && status.identityReady && status.sgpReady;status.clientView={type:'client-view',state:status.clientDiscovery==='exiting'?'exiting':ready?'ready':'no-client',summoner:status.summoner,region:status.clientRegion,serverId:status.serverId,sgpReady:status.sgpReady,generation:status.clientView.generation+1};for(const stream of window.__r258Streams||[])stream.onmessage?.({data:JSON.stringify(status.clientView)});};
`;
const intercept=`
 if(pathname==='/api/gameplay/history-head'){window.__r278HeadReads=(window.__r278HeadReads||0)+1;return r258JSON({matches:window.__r278HeadMatches || []});}
 if(pathname==='/api/gameplay/overview'){
  if(window.__r278QuotaTest){window.__r278QuotaReads=(window.__r278QuotaReads||0)+1;if(window.__r278QuotaReads===1)return Promise.resolve(new Response(JSON.stringify({error:'查询额度恢复中',retryAfter:44}),{status:429,headers:{'Content-Type':'application/json'}}));return r258JSON({...overview,player:{...overview.player,playerRef:'player_r278_foreign',region:'kr',isCurrent:false}});}
  if(window.__r278ReferenceTest){window.__r278ReferenceHeaders.push(init.headers.get('X-Overview-No-Reference') || '');if(window.__r278ReferenceHeaders.length===1){const matches=overview.matches.slice(0,20).map(m=>{const copy={...m};delete copy.revision;return copy});const frames=[{type:'progress',overview:{...overview,matches}},{type:'complete',overview:{...overview,matches:[{gameId:123456789,revision:''}]}}];return Promise.resolve(new Response(frames.map(f=>JSON.stringify(f)).join('\\n')+'\\n',{headers:{'Content-Type':'application/x-ndjson'}}))}window.__r278ReferenceTest=false;return r258JSON(overview);}
  if(window.__r278Gate){window.__r278Gate=false;return new Promise(resolve=>window.__r278Release=()=>resolve(new Response(JSON.stringify(overview),{headers:{'Content-Type':'application/json'}})));}
  window.__r258OverviewReads=(window.__r258OverviewReads||0)+1;
  if(!window.__r258Released)return new Promise(resolve=>window.__r258FinishOverview=()=>{window.__r258Released=true;resolve(new Response(JSON.stringify(overview),{headers:{'Content-Type':'application/json'}}));});
  return r258JSON(overview);
 }
`;
async function main(){
 fs.mkdirSync(out,{recursive:true});if(fs.readdirSync(out).length)throw Error('R258 screenshot output must be empty');
 const launched=await launchChrome({executable:process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',args:['--headless=new','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],directory:out});
 chrome=launched.child;const endpoint=launched.endpoint;
 ws=new WebSocket(endpoint);await new Promise((r,j)=>{ws.addEventListener('open',r,{once:true});ws.addEventListener('error',j,{once:true})});let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method))},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}))});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [r,j]=pending.get(m.id);pending.delete(m.id);m.error?j(Error(JSON.stringify(m.error))):r(m.result)}if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);if(m.method==='Fetch.requestPaused'){const url=m.params.request.url,ok=url.startsWith(origin+'/')||url.startsWith('data:');void send(ok?'Fetch.continueRequest':'Fetch.failRequest',{requestId:m.params.requestId,...(ok?{}:{errorReason:'BlockedByClient'})},m.sessionId).catch(()=>{})}});
 server=http.createServer((req,res)=>{const url=new URL(req.url,'http://fixture'),name=url.pathname;
  if(name==='/api/announcements'){res.writeHead(200,{'Content-Type':'application/json'});const notices=JSON.parse(fs.readFileSync(path.join(root,'backend/announcements/announcements.json')));notices[0].revision=noticeRevision;res.end(JSON.stringify({announcements:notices,source:'bundled'}));return}
  if(name==='/api/gameplay/phase'){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({phase:'None'}));return}
  if(name==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream'});res.write('event: heartbeat\ndata: {}\n\n');const timer=setInterval(()=>res.write('event: heartbeat\ndata: {}\n\n'),2000);req.on('close',()=>clearInterval(timer));return}
  if(name==='/api/license/status'){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({state:'ACTIVE',generation:1,message:''}));return}
  if(name.startsWith('/api/')){const asset=name.includes('image')||name.includes('asset');res.writeHead(200,{'Content-Type':asset?'image/svg+xml':'application/json'});res.end(asset?'<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#587c83"/><path d="M8 32 20 8 32 32Z" fill="#c8af67"/></svg>':'{}');return}
  const relative=name==='/'?'index.html':name.slice(1),file=path.join(web,['index.html','license-ui.js'].includes(relative)?'default/'+relative:relative);if(relative.includes('..')||!fs.existsSync(file)){res.writeHead(404);res.end();return}let data=baseline && ['gameplay.js','gameplay.css'].includes(relative) ? fs.readFileSync(path.join(out,'baseline/backend/web',relative)) : fs.readFileSync(file);
  if(relative==='demo-data.js')data=Buffer.from(data.toString().replace('  const fixtures = new Map([',inject+'\n  const fixtures = new Map([').replace('    const params = new URLSearchParams(url.split("?")[1] || "");','    const params = new URLSearchParams(url.split("?")[1] || "");'+intercept));
  if(relative==='gameplay.js')data=Buffer.from(data.toString().replace('  bindSettings();','  window.__r278Game={state,activeTab,applyClientViewStatus,renderOverview,probeOverviewHead,retryHistoryRegion,handleOverviewIncremental,bindMatchSentinel,loadOverview,openPlayerByRiotId};\n  bindSettings();'));
  if(relative==='app.js')data=Buffer.from(data.toString().replace(/\n\}\)\(\);\s*$/,'\n  window.__r278App={state,renderLaunchpad,applySearchRegion,activateSection};\n})();'));
  res.writeHead(200,{'Content-Type':({'.js':'text/javascript','.css':'text/css','.html':'text/html','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store'});res.end(data);
 });await new Promise(r=>server.listen(0,'127.0.0.1',r));origin='http://127.0.0.1:'+server.address().port;
 async function page(theme,ratingFail=false,foreign=false,arena=false,startup=false){const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  await call('Page.enable');await call('Page.addScriptToEvaluateOnNewDocument',{source:'window.__r258Streams=[];window.EventSource=class {constructor(){window.__r258Streams.push(this)}addEventListener(){}close(){}};try { localStorage.clear(); } catch (_) {} window.__r241LongTasks=[];new PerformanceObserver(list=>window.__r241LongTasks.push(...list.getEntries().map(e=>({start:e.startTime,duration:e.duration})))).observe({type:"longtask",buffered:true});'});await call('Runtime.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
  const until=async expression=>{let prior='',stalled=0;for(;;){if(await evaluate(expression))return;const progress=JSON.stringify(await evaluate("({reads:window.__r258OverviewReads,head:window.__r278HeadReads,reference:window.__r278ReferenceHeaders?.length,quota:window.__r278QuotaReads,scroll:window.__r278ScrollCalls,reports:window.__r278Sentinel?.reports?.length,inputMode:window.__r278Sentinel?.mode,rootScrollTop:window.__r278Sentinel?.root?.scrollTop,rootClientWidth:window.__r278Sentinel?.root?.clientWidth,rootOffsetWidth:window.__r278Sentinel?.root?.offsetWidth,ready:!!window.__r278Game,matches:document.querySelectorAll('.match-entry').length,notice:document.getElementById('announcements-dialog')?.open})"));stalled=progress===prior?stalled+1:0;prior=progress;if(stalled>=200){fs.writeFileSync(path.join(out,'stalled-state.json'),JSON.stringify({expression,progress,reason:'no progress for 200 polling intervals'},null,2));fs.writeFileSync(path.join(out,'failure.json'),JSON.stringify({error:'no-progress watchdog '+expression,progress:JSON.parse(progress)},null,2));await shot('stalled');throw Error('no-progress watchdog '+expression)}await new Promise(r=>setTimeout(r,100))}};
  const shot=async name=>{if(guardOnly)return;const s=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,theme+'-'+name+'.png'),Buffer.from(s.data,'base64'))};
  await call('Page.navigate',{url:origin+'/?demo&section=overview'});
  await until(`window.__r258SetStatus && window.__r278Game && window.__r278App`);
  await evaluate(`document.documentElement.dataset.theme='${theme}'`);
  return{theme,call,evaluate,until,shot};
 }

 for(const theme of ['dark','light']) {
  const p=await page(theme),ev=p.evaluate;
  await p.until("window.__r278Game && window.__r278App && window.deepLegendsAnnouncements");
  await p.until("document.querySelector('#announcements-dialog').open");
  await ev("document.querySelector('[data-announcements-close]').click()");
  assert(await ev("document.getElementById('announcements-button').classList.contains('has-announcement-unread')"));
  await ev("window.dispatchEvent(new CustomEvent('deep-legends:gameflow',{detail:{phase:'None'}}))");
  assert.equal(await ev("document.querySelector('#announcements-dialog').open"),false,'dismiss must not reopen in same run');
  await ev("document.getElementById('announcements-button').click();document.querySelector('[data-announcement-read]').click()");
  assert.equal(await ev("document.getElementById('announcements-button').classList.contains('has-announcement-unread')"),false);
  await ev("document.getElementById('announcements-button').click()");
  assert.equal(await ev("document.querySelector('[data-announcement-read]').textContent"),'关闭');
  await ev("document.querySelector('[data-announcements-close]').click()");
  // Actual launcher DOM in both groups, followed by controlled login state.
  await ev("Object.assign(__r278App.state,{overviewGroup:'kr',installationsLoaded:true,installations:[],installationLoadError:'',clientView:{state:'no-client'}});__r278App.renderLaunchpad()");
  assert.equal(await ev("document.querySelectorAll('#launcher-list .scan-launchers').length"),0);
  assert(await ev("document.querySelector('[data-client-id=riot]').disabled && document.querySelector('[data-client-id=riot]').textContent.includes('本机未安装 Riot 客户端')"));
  await ev("__r278App.state.installations=[{id:'riot',available:true,name:'Riot 客户端'}];__r278App.renderLaunchpad()");
  assert.equal(await ev("document.querySelector('[data-client-id=riot]').disabled"),false);
  await ev("Object.assign(__r278App.state,{overviewGroup:'players',installations:[]});__r278App.renderLaunchpad()");
  assert.equal(await ev("document.querySelectorAll('#launcher-list .scan-launchers').length"),1);
  await ev("__r278Game.state.activeGroup='kr';__r278Game.state.lastManualTabAt=Date.now();__r278App.applySearchRegion('kr');localStorage.setItem('deep-legends-search-region-manual','true');__r258SetStatus({connected:true,identityReady:true,sgpReady:true,clientDiscovery:'connected',clientRegion:'TENCENT',serverId:'HN10',serverName:'黑色玫瑰'})");
  await p.until("document.querySelector('.summoner-strip h2') && window.__r258FinishOverview");
  assert.equal(await ev("__r278Game.state.activeGroup"),'players');
  assert(await ev("document.querySelector('#player-search-region').dataset.region==='cn' && document.querySelector('#player-search-region').textContent.includes('黑色玫瑰')"));
  await ev("__r278App.applySearchRegion('jp1');__r278Game.state.activeGroup='kr';__r278Game.applyClientViewStatus(__r278Game.state.status)");
  assert.equal(await ev("__r278Game.state.activeGroup"),'kr');
  assert.equal(await ev("document.querySelector('#player-search-region').dataset.region"),'jp1');
  await ev("__r278Game.state.activeGroup='players';__r258FinishOverview()");
  await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
  // Two real rank rows; no fixed loaded-card height.
  const ranks=await ev("(()=>{const n=document.querySelector('[data-career-key=ranks]');return {min:getComputedStyle(n).minHeight,height:n.getBoundingClientRect().height,history:!!n.querySelector('.rank-history')}})()");
  assert.notEqual(ranks.min,'450px');assert.equal(ranks.history,false);
  // A 120-match tab survives stale section return and a new head match.
  await ev("(()=>{const t=__r278Game.activeTab(),one=t.data.matches[0];t.data.matches=Array.from({length:120},(_,i)=>({...one,gameId:900000+i}));t.nextBegIndex=120;t.data.pagination={...t.data.pagination,begIndex:100,count:20,hasMore:false};t.loadedAt=Date.now()-180001;t.quotaRetry=null;t.overviewScrollTop=650;__r278Game.renderOverview('players');window.__r278HeadMatches=[{...one,gameId:999999}];window.__r278Saved=t.nextBegIndex;window.__r278Reads=__r258OverviewReads;})()");
  await ev("new Promise(resolve=>requestAnimationFrame(()=>{document.getElementById('app-scroll').scrollTop=650;resolve()}))");
  await ev("__r278App.activateSection('favorites');__r278App.activateSection('overview')");
  await p.until("__r278HeadReads===1 && !__r278Game.activeTab().headProbeRunning");
  assert.equal(await ev("__r278Game.activeTab().data.matches.length"),121);
  assert.equal(await ev("__r278Game.activeTab().nextBegIndex"),120);
  assert.equal(await ev("__r258OverviewReads"),await ev("__r278Reads"));
  await ev("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");
  const scroll=await ev("document.getElementById('app-scroll').scrollTop");
  assert(scroll>0,'stored overview scroll must survive section return');
  for(const zoom of [90,100,125,150,200])await ev(`window.dispatchEvent(new CustomEvent('deep-legends:zoom',{detail:{zoom:${zoom}}}))`);
  assert.equal(await ev("__r258OverviewReads"),await ev("__r278Reads"));
  await ev("window.__r278ReferenceHeaders=[];window.__r278ReferenceTest=true;__r278Game.loadOverview(__r278Game.activeTab(),true,false,false,true)");
  await p.until("!__r278Game.activeTab().loading && __r278ReferenceHeaders.length===2");
  assert.deepEqual(await ev("__r278ReferenceHeaders"),['','1']);
  assert(await ev("__r278Game.activeTab().data.matches.length>=120 && __r278Game.activeTab().nextBegIndex===120 && !__r278Game.activeTab().initialPagePending"));
  assert.equal(await ev("document.body.textContent.includes('战绩引用不匹配')"),false);
  results.push({theme,kind:'silent-reference-fallback',requests:2,unscoredProgress:20,oldPagesRetained:true});
  // Controlled history failure keeps the header while the response gate is held.
  await ev("(()=>{const t=__r278Game.activeTab();t.data.capabilities=[...t.data.capabilities.filter(c=>c.name!=='match-history'),{name:'match-history',state:'failed',historyStatus:'official',retryAfter:1}];t.data.matches=[];__r278Game.renderOverview('players');window.__r278Header=document.querySelector('.summoner-strip');window.__r278Skeletons=document.querySelectorAll('.gameplay-skeleton').length;window.__r278Gate=true;__r278Game.retryHistoryRegion(t,'outage-recovery')})()");
  await p.until("window.__r278Release");
  assert(await ev("window.__r278Header===document.querySelector('.summoner-strip')"));
  assert(await ev("document.querySelectorAll('.gameplay-skeleton').length<=__r278Skeletons && !document.querySelector('.overview-strip-skeleton')"));
  assert.equal(await ev("document.querySelector('.match-list')?.textContent.includes('共 0 场')||false"),false);
  await ev("__r278Release()");await p.until("!__r278Game.activeTab().loading");
  // Actual saved-filter modal in six viewport/theme combinations.
  await ev("__r278Game.activeTab().advancedConditions={hero:{values:['103']}};__r278Game.renderOverview('players')");
  await p.until("document.querySelector('[data-af-open]')");
  await ev("document.querySelector('[data-af-open]').click()");await p.until("document.querySelector('[data-af-show-saved]')");
  await ev("document.querySelector('[data-af-show-saved]').click()");await p.until("document.querySelector('[data-af-name]') && !document.querySelector('[data-af-save]').disabled");
  await ev("document.querySelector('[data-af-save]').click()");
  assert(await ev("document.querySelector('[data-af-name]').getAttribute('aria-invalid')==='true' && document.activeElement===document.querySelector('[data-af-name]')"));
  assert(await ev("document.querySelector('.af-current-conditions header').textContent.includes('请先在底部输入框输入名称后保存')"));
  for(const width of [900,1280,1920]){
   await p.call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});
   await ev("Promise.all(document.querySelector('[data-af-name]').getAnimations().map(a=>a.finished.catch(()=>{})))");
   const geometry=await ev("(()=>{const n=document.querySelector('[data-af-name]'),r=n.getBoundingClientRect(),b=n.parentElement.getBoundingClientRect();return {left:r.left,right:r.right,containerLeft:b.left,containerRight:b.right,outline:getComputedStyle(n.parentElement).borderStyle}})()");
   assert(geometry.left>=geometry.containerLeft-1 && geometry.right<=geometry.containerRight+1);assert.notEqual(geometry.outline,'dashed');
   await p.shot('preset-empty-'+width);results.push({theme,width,kind:'preset-inline-error',geometry});
  }
  await p.call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});
  await ev("document.querySelector('[data-af-save]').click()");
  assert.equal(await ev("document.querySelector('[data-af-name]').getAnimations().length"),0);
  await p.call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'no-preference'}]});
  await ev("document.querySelector('[data-af-name]').value='测试';document.querySelector('[data-af-name]').dispatchEvent(new Event('input',{bubbles:true}))");
  assert.equal(await ev("document.querySelector('[data-af-name]').hasAttribute('aria-invalid')"),false);
  await ev("document.querySelector('[data-af-close]').click()");
  for(const zoom of [0.9,1,1.25,1.5,2]){
   await p.call('Emulation.setDeviceMetricsOverride',{width:900,height:1100,deviceScaleFactor:1,mobile:false});
   await ev(`document.documentElement.style.zoom=${zoom};document.getElementById('section-favorites').classList.add('has-collection-unread')`);
   const marker=await ev("(()=>{const n=document.getElementById('section-favorites'),r=n.getBoundingClientRect(),c=getComputedStyle(n,'::after');return {top:parseFloat(c.top),half:n.clientHeight/2,transform:c.transform,toolbar:[...document.querySelectorAll('.workspace-controls>.icon-button')].map(x=>Math.round(x.getBoundingClientRect().top))}})()");
   assert(Math.abs(marker.top-marker.half)<=2);assert.notEqual(marker.transform,'none');
   if(marker.toolbar.length)assert(Math.max(...marker.toolbar)-Math.min(...marker.toolbar)<=1,'toolbar must stay one row');
   await p.shot('marker-toolbar-'+Math.round(zoom*100));results.push({theme,zoom,kind:'marker-toolbar',marker});
  }
  await ev("document.documentElement.style.zoom=1");
  results.push({theme,kind:'behavior',loginFollows:true,manualRefreshPreserved:true,launcherGroups:true,historyHeadPreservesPages:true,zoomMakesNoOverviewRequest:true,outageKeepsHeader:true,announcementReadDismiss:true});
 }
 // A fresh document preserves read revisions, and delays an unread popup during a game.
 const p=await page('dark');await p.until("window.deepLegendsAnnouncements");
 await p.evaluate("document.querySelector('[data-announcements-close]')?.click();localStorage.removeItem('deep-legends-announcements-read-v1');window.dispatchEvent(new CustomEvent('deep-legends:gameflow',{detail:{phase:'ChampSelect'}}))");
 noticeRevision='2';await p.evaluate("window.dispatchEvent(new CustomEvent('deep-legends:announcements-changed'))");
 await p.until("document.getElementById('announcements-button').classList.contains('has-announcement-unread')");
 assert.equal(await p.evaluate("document.getElementById('announcements-dialog').open"),false);
 await p.evaluate("window.dispatchEvent(new CustomEvent('deep-legends:gameflow',{detail:{phase:'None'}}))");await p.until("document.getElementById('announcements-dialog').open");
 await p.evaluate("document.getElementById('announcements-dialog').dispatchEvent(new Event('cancel',{cancelable:true}))");
 assert(await p.evaluate("document.getElementById('announcements-button').classList.contains('has-announcement-unread')"));
 results.push({kind:'announcement-revision-game-phase',deferred:true,escUnread:true});
 await p.evaluate("window.__r278QuotaTest=true;window.__r278QuotaReads=0;__r278Game.state.settings.matchCount=10;__r278Game.openPlayerByRiotId('Known','KR','kr');window.__r278QuotaTab=__r278Game.activeTab()");
 await p.until("__r278QuotaTab.quotaRetry && !__r278QuotaTab.loading");
 assert.equal(await p.evaluate("__r278QuotaReads"),1);
 assert(await p.evaluate("document.querySelector('.summoner-strip').textContent.includes('Known') && document.body.textContent.includes('秒后自动开始') && !document.querySelector('.overview-strip-skeleton')"));
 assert.equal(await p.evaluate("document.querySelector('[data-quota-recovery]').textContent.includes('已加载的战绩仍可查看')"),false);
 await p.evaluate("clearTimeout(__r278QuotaTab.quotaRetry.timer);__r278Game.loadOverview(__r278QuotaTab,true,false,false,true)");
 await p.until("!__r278QuotaTab.loading && __r278QuotaTab.data?.matches?.length");
 assert(await p.evaluate("!__r278QuotaTab.initialPagePending && __r278QuotaTab.matchesReceived && __r278QuotaTab.recentStatsReceived && __r278QuotaTab.overviewCardLoad.ready.has('filterbar') && __r278QuotaTab.overviewCardLoad.ready.has('highlights') && !!document.querySelector('.match-filterbar:not(.gameplay-skeleton)')"));
 await p.evaluate("window.__r278QuotaTest=false");results.push({kind:'known-player-quota-recovery',headerImmediate:true,newTabCopyCorrect:true,allPendingFinished:true});
 // Sentinel logic from the production function, driven by real scroll input and IO.
 await p.evaluate("document.getElementById('announcements-dialog').close();(()=>{const root=document.createElement('section');root.className='player-overlay-scroll';root.tabIndex=0;root.style.cssText='position:fixed;top:80px;left:480px;width:300px;height:200px;overflow:scroll;scrollbar-width:auto;scrollbar-color:auto;z-index:999999;background:var(--bg)';root.innerHTML='<main style=\"height:1800px\"><div data-match-sentinel style=\"position:relative;top:1700px;height:30px\"></div></main>';document.body.append(root);const container=root.firstChild,tab={key:'r278-scroll',data:{pagination:{hasMore:true}},loadingMore:true},s={},reports=[];const bind=Function('state','matchObserverKey','overviewContainer','loadOverview','window','return ('+__r278Game.bindMatchSentinel.toString()+')')(s,()=> 'observer',()=>container,t=>{t.overviewRequestToken=Number(t.overviewRequestToken||0)+1;window.__r278ScrollCalls++;return Promise.resolve(true)},{IntersectionObserver,requestAnimationFrame:requestAnimationFrame.bind(window),reportFlowDiagnostic:(event,reason,fields)=>reports.push({event,reason,fields})});root.id='r278-scroll-input-fixture';const sheet=[...document.styleSheets].find(s=>s.href&&new URL(s.href).pathname==='/app.css'),scrollbarRules=['#r278-scroll-input-fixture::-webkit-scrollbar{width:16px;height:16px}','#r278-scroll-input-fixture::-webkit-scrollbar-button{display:none;width:0;height:0}','#r278-scroll-input-fixture::-webkit-scrollbar-thumb{background:#888;min-height:24px}'].map(rule=>sheet.insertRule(rule,sheet.cssRules.length));window.__r278ScrollCalls=0;window.__r278Sentinel={root,container,tab,s,reports,bind,sheet,scrollbarRules,mode:'loading-wheel'};bind(container,tab)})()");
 await p.call('Input.dispatchMouseEvent',{type:'mouseWheel',x:550,y:160,deltaY:2000,deltaX:0});
 await p.until("__r278Sentinel.reports.some(r=>r.fields.blocked_reason==='loading' && r.fields.near)");
 assert.equal(await p.evaluate("__r278ScrollCalls"),0);assert(await p.evaluate("!!__r278Sentinel.s.observer"));
 await p.evaluate("__r278Sentinel.tab.loadingMore=false;__r278Sentinel.s.observer.check()");await p.until("__r278ScrollCalls===1");assert.equal(await p.evaluate("__r278Sentinel.s.observer"),null);
 for(const mode of ['keyboard','scrollbar','touch']){
  await p.evaluate(`(()=>{const f=__r278Sentinel;f.mode=${JSON.stringify(mode)};f.root.scrollTop=0;f.tab.quotaRetry={retryAt:Date.now()+60000};f.tab.matchAutoloadRequested=false;f.reports.length=0;f.bind(f.container,f.tab);f.root.focus()})()`);
  if(mode==='keyboard'){
   await p.call('Input.dispatchKeyEvent',{type:'keyDown',key:'End',code:'End',windowsVirtualKeyCode:35});await p.call('Input.dispatchKeyEvent',{type:'keyUp',key:'End',code:'End',windowsVirtualKeyCode:35});
  }else if(mode==='scrollbar'){
   const rect=await p.evaluate("(()=>{const r=__r278Sentinel.root.getBoundingClientRect();return {x:r.right-6,y:r.top+12,bottom:r.bottom-12}})()");
   await p.call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,x:rect.x,y:rect.y});
   for(let i=1;i<=10;i++)await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',button:'left',buttons:1,x:rect.x,y:rect.y+(rect.bottom-rect.y)*i/10});
   await p.call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,x:rect.x,y:rect.bottom});
  }else await p.evaluate("__r278Sentinel.root.dispatchEvent(new Event('touchmove'));__r278Sentinel.root.scrollTop=1800");
  await p.until("__r278Sentinel.reports.some(r=>r.fields.blocked_reason==='quota' && r.fields.near)");
  assert(await p.evaluate("!!__r278Sentinel.s.observer"));const previous=await p.evaluate("__r278ScrollCalls");
  await p.evaluate("__r278Sentinel.tab.quotaRetry=null;__r278Sentinel.s.observer.check()");await p.until(`__r278ScrollCalls===${previous+1}`);
  results.push({kind:'sentinel-input',mode,blockedThenStarted:true,inputGeometry:await p.evaluate("(()=>{const r=__r278Sentinel.root;return {gutter:r.offsetWidth-r.clientWidth,scrollTop:r.scrollTop,clientHeight:r.clientHeight,scrollHeight:r.scrollHeight,width:getComputedStyle(r).scrollbarWidth,buttons:getComputedStyle(r,'::-webkit-scrollbar-button').display}})()")});
 }
 await p.evaluate("__r278Sentinel.root.remove();for(const index of __r278Sentinel.scrollbarRules.slice().reverse())__r278Sentinel.sheet.deleteRule(index)");
 const summary={scope:'Real Chromium production UI with explicit synthetic API/state fixtures; no real Windows or LCU claim.',results,errors};
 fs.writeFileSync(path.join(out,'chromium-synthetic.json'),JSON.stringify(summary,null,2));assert.equal(errors.length,0,JSON.stringify(errors));console.log(JSON.stringify({results:results.length,errors:errors.length}));
}
main().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>cleanupChromeTest({children:[chrome],sockets:[ws],servers:[server],directories:[temp]}));
