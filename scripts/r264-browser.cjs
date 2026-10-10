'use strict';
// Real public backend and its CSP/assets, with explicit synthetic API fixtures.
const {spawn}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=process.env.R264_BROWSER_OUT||path.join(os.tmpdir(),'deep-legends-r264-browser');
const closedSessions=new Set(), canceledRequests=new Set();let canceledInterceptions=0;
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r264-browser-')),errors=[],violations=[],results=[];let backend,chrome,ws;
const saved={
 'lol-loot-default-match-filter':'solo','lol-loot-mask-names':'true',
 'lol-loot-player-tab-order':'["cn:HN1:riot:搜索测试#","kr:riot:fixture#kr1"]',
 'deep-legends-history-presets-v1':JSON.stringify([{name:'上次常用',conditions:{result:{values:['win'],not:false}}}]),
};
const inject=`
 window.__r264Scenario=new URLSearchParams(location.search).get("r264Scenario")||"";window.__r264OverviewRequests=0;
 window.__r264Streams ||= [];
 window.__r264SetStatus=next=>{Object.assign(status,next);status.clientView={type:'client-view',state:status.clientDiscovery==='exiting'?'exiting':status.connected && status.identityReady && status.sgpReady?'ready':'no-client',summoner:status.summoner,region:status.clientRegion,serverId:status.serverId,sgpReady:status.sgpReady,generation:(status.clientView?.generation||0)+1};for(const stream of window.__r264Streams)stream.onmessage?.({data:JSON.stringify(status.clientView)});};
 window.__r264SetStatus({connected:true,identityReady:true,sgpReady:true});
 window.__r264EnsureCalls=0;window.__r264API=[];window.__r264Statuses=[];window.addEventListener("deep-legends:status",e=>window.__r264Statuses.push({snapshot:e.detail.snapshotReady,connected:e.detail.connected,syncing:e.detail.syncing}));
`;
const intercept=` const scenario=window.__r264Scenario;
 if(pathname==='/api/gameplay/season-summary' && scenario.startsWith('foreign-'))return Promise.resolve(new Response(JSON.stringify({available:false,playerRef:'player_foreign_fixture',champions:[],overall:{}}),{headers:{'Content-Type':'application/json'}}));
 if(pathname==='/api/gameplay/match-timeline' && scenario==='official-inline')return Promise.resolve(new Response(JSON.stringify({available:false,historyStatus:'official',detail:'战绩服务器暂时不可用，稍后自动重试'}),{headers:{'Content-Type':'application/json'}}));
 if(pathname==='/api/gameplay/overview' && scenario){
  let request={};try{request=JSON.parse(init?.body||'{}')}catch{}
  const foreign=scenario.startsWith('foreign-');
  if(!foreign || request.region==='kr' || request.playerRef==='player_foreign_fixture'){
   window.__r264OverviewRequests++;const data=structuredClone(overview);
   if(scenario==='official-large' || scenario==='official-auto'){data.matches=[];data.capabilities=data.capabilities.filter(c=>!c.name.startsWith('match'));data.capabilities.push({name:'match-history',state:'failed',historyStatus:'official',retryAfter:scenario==='official-auto'?2:60});if(scenario==='official-auto' && window.__r264OverviewRequests>1)return Promise.resolve(new Response(JSON.stringify(overview),{headers:{'Content-Type':'application/json'}}));}
   if(scenario==='official-compact'){data.matches=data.matches.slice(0,1);data.pagination.partial=true;data.capabilities=data.capabilities.filter(c=>!c.name.startsWith('match'));data.capabilities.push({name:'match-details',state:'failed',historyStatus:'official',retryAfter:60});}
   if(foreign){data.player.region='kr';data.player.playerRef='player_foreign_fixture';data.player.gameName='外服测试';data.player.tagLine='KR1';data.player.isCurrent=false;data.historicalRanks=[];data.matches=scenario==='foreign-partial'?data.matches.slice(0,1):[];data.pagination={begIndex:0,count:10,filter:'all',hasMore:true,partial:true};
    window.__r264OverviewStarted=performance.now();const encoder=new TextEncoder();
    const stream=new ReadableStream({start(controller){controller.enqueue(encoder.encode(JSON.stringify({type:'cards',overview:{player:data.player,matches:data.matches,ranks:data.ranks,masteries:data.masteries,capabilities:data.capabilities.filter(c=>!c.name.startsWith('match')),pagination:data.pagination}})+'\\n'));setTimeout(()=>{if(init?.signal?.aborted){try{controller.close()}catch{};return;}data.capabilities.push({name:'match-history',state:'failed',detail:'战绩服务暂时不可用',count:data.matches.length});controller.enqueue(encoder.encode(JSON.stringify({type:'error',status:503,error:'战绩服务暂时不可用',kind:'network'})+'\\n'));controller.close();window.__r264OverviewFinished=true;},7000)}});
    return Promise.resolve(new Response(stream,{headers:{'Content-Type':'application/x-ndjson'}}));
   }
   if(scenario==='partial-read-proposal'){data.matches=data.matches.slice(0,1);const m=data.matches[0],s=m.participants.find(p=>p.participantId===m.subjectParticipantId),win=m.result==='win';data.recentRanked={queueId:420,queueLabel:'单双排',games:1,wins:Number(win),losses:Number(!win),winRate:win?100:0,kills:s.kills,deaths:s.deaths,assists:s.assists,kda:s.kda,killParticipation:100*(s.kills+s.assists)/m.teams.find(t=>t.teamId===s.teamId).kills,killParticipationGames:1,positions:[{position:s.position,games:1,wins:Number(win),winRate:win?100:0}]};data.ability=null;data.positions=[{position:s.position,games:1,wins:Number(win),share:100}];delete data.rankedQueues;data.pagination={count:1,hasMore:false};}
   return Promise.resolve(new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}}));
  }
 }

 window.__r264API.push({path:pathname,snapshot:status.snapshotReady,at:performance.now()});
 // Wait for the consumer of this instant in-memory status fixture. With
 // gameplay.js deliberately delayed, c725ba27 also loses the first event.
 // The existing five-second preparation deadline still applies.
 if(pathname==='/api/status')return new Promise(resolve=>{const ready=()=>{if(window.deepLegendsMatchCards)resolve(new Response(JSON.stringify(status),{headers:{'Content-Type':'application/json'}}));else requestAnimationFrame(ready);};ready();});
 if(pathname==='/api/gameplay/summoner-spells')return Promise.resolve(new Response(JSON.stringify({spells:[{id:4,name:'闪现'}]}),{headers:{'Content-Type':'application/json'}}));
 if(pathname==='/api/collection/ensure'){window.__r264EnsureCalls++;if(window.__r264Conflict)return Promise.resolve(new Response('客户端连接中',{status:409}));status.snapshotReady=true;queueMicrotask(()=>{for(const stream of window.__r264Streams)stream.onmessage?.({data:"snapshot-updated"})});return Promise.resolve(new Response(null,{status:202}));}
`;
async function start(){
 fs.mkdirSync(out,{recursive:true});
 backend=spawn(process.env.R264_BACKEND||'/private/tmp/r264-baseline-public',['--desktop'],{cwd:root,env:{...process.env,LOL_LOOT_DATA_DIR:path.join(temp,'data')},stdio:['ignore','pipe','pipe']});
 const ready=await new Promise((resolve,reject)=>{let data='';const timer=setTimeout(()=>reject(Error('Backend startup timeout')),20000);backend.once('error',reject);backend.stdout.on('data',c=>{data+=c;for(const line of data.split('\n')){try{const value=JSON.parse(line.replace(/^LOOT_READY /,''));if(value.baseUrl){clearTimeout(timer);resolve(value);return}}catch{}}});backend.once('exit',code=>reject(Error('Backend exited '+code)));});
 const csp=(await fetch(ready.bootstrapUrl)).headers.get('content-security-policy');assert(csp?.includes("script-src 'self'") && !csp.includes('unsafe-inline'));
 chrome=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--no-sandbox','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${path.join(temp,'chrome')}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const endpoint=await new Promise((resolve,reject)=>{let data='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);chrome.once('error',reject);chrome.stderr.on('data',c=>{data+=c;const match=data.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(match){clearTimeout(timer);resolve(match[1]);}});});
 ws=new WebSocket(endpoint);await new Promise((resolve,reject)=>{ws.addEventListener('open',resolve,{once:true});ws.addEventListener('error',reject,{once:true});});
 let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [resolve,reject]=pending.get(m.id);pending.delete(m.id);m.error?reject(Error(JSON.stringify(m.error))):resolve(m.result);}
  if(m.method==='Network.loadingFailed' && m.params.canceled)canceledRequests.add(m.sessionId+':'+m.params.requestId);
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);
  if(m.method==='Log.entryAdded' && m.params.entry.level==='error')errors.push(m.params.entry);
  if(m.method==='Runtime.bindingCalled' && m.params.name==='r264CSP')violations.push(JSON.parse(m.params.payload));
  if(m.method==='Fetch.requestPaused')void (async()=>{const url=m.params.request.url,pathname=new URL(url).pathname,id=m.params.requestId;
   if(pathname==='/api/image'){await send('Fetch.fulfillRequest',{requestId:id,responseCode:200,responseHeaders:[{name:'Content-Type',value:'image/svg+xml'}],body:Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#23384a"/><circle cx="320" cy="180" r="80" fill="#cc9e45"/></svg>').toString('base64')},m.sessionId);}
   else if(pathname==='/demo-data.js'){const response=await fetch(ready.baseUrl+pathname),body=(await response.text()).replace('  const fixtures = new Map([',inject+'\n  const fixtures = new Map([').replace('    const params = new URLSearchParams(url.split("?")[1] || "");','    const params = new URLSearchParams(url.split("?")[1] || "");'+intercept);await send('Fetch.fulfillRequest',{requestId:id,responseCode:200,responseHeaders:[{name:'Content-Type',value:'text/javascript'}],body:Buffer.from(body).toString('base64')},m.sessionId);}
   else if(url.startsWith(ready.baseUrl+'/'))await send('Fetch.continueRequest',{requestId:id},m.sessionId);
   else await send('Fetch.failRequest',{requestId:id,errorReason:'BlockedByClient'},m.sessionId);
  })().catch(async e=>{if(closedSessions.has(m.sessionId))return;if(e.message.includes('Invalid InterceptionId')){await new Promise(resolve=>setTimeout(resolve,50));if(canceledRequests.has(m.sessionId+':'+m.params.networkId)){canceledInterceptions++;return;}}errors.push({message:e.message});});
 });
 for(const theme of ['dark','light'])for(const storage of ['empty','saved','malformed']){
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
  const until=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await new Promise(resolve=>setTimeout(resolve,50));}const snapshot=await evaluate("({api:window.__r264API?.slice(-20),statuses:window.__r264Statuses?.slice(-10),ensureCalls:window.__r264EnsureCalls,conflict:window.__r264Conflict,hidden:document.hidden,view:document.querySelector('#favorites-panel')?.textContent.slice(0,1200),grid:document.querySelector('#skin-grid')?.textContent.slice(0,500),launch:document.querySelector('#client-launchpad')?.hidden,filter:!!document.querySelector('[data-af-open]'),requests:window.__r264OverviewRequests,overview:document.querySelector('#overview-content')?._overviewViewTab && {loading:document.querySelector('#overview-content')._overviewViewTab.loading,error:document.querySelector('#overview-content')._overviewViewTab.initialPageError}})");throw Error('timeout '+expression+' '+JSON.stringify(snapshot));};
  const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
  const shot=async name=>{if(storage!=='empty')return;const r=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,`${theme}-${name}.png`),Buffer.from(r.data,'base64'));};
  await send('Target.activateTarget',{targetId});
  await call('Network.enable');await call('Page.enable');await call('Runtime.enable');await call('Log.enable');await call('Runtime.addBinding',{name:'r264CSP'});
  const values=storage==='saved'?saved:storage==='malformed'?{...saved,'deep-legends-history-presets-v1':'[{"name":"bad","conditions":42}]','lol-loot-player-tab-order':'{"bad":1}','lol-loot-default-match-filter':'wrong'}:{};
  await call('Page.addScriptToEvaluateOnNewDocument',{source:`window.__r264OverlayShown=0;new MutationObserver(events=>{for(const e of events)if(e.target.id==='startup-loading' && !e.target.hidden)window.__r264OverlayShown++;}).observe(document,{subtree:true,attributes:true,attributeFilter:['hidden']});window.__r264Streams=[];window.EventSource=class {constructor(){window.__r264Streams.push(this)}addEventListener(){}close(){}};try{localStorage.clear();for(const [k,v] of Object.entries(${JSON.stringify(values)}))localStorage.setItem(k,v);}catch{};document.addEventListener('securitypolicyviolation',e=>r264CSP(JSON.stringify({directive:e.effectiveDirective,blockedURI:e.blockedURI})));`});
  await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});
  await call('Network.setCookie',{name:'lol_loot_token',value:ready.token,url:ready.baseUrl,httpOnly:true});
  await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview'});
  await until("window.__r264SetStatus && document.querySelector('.match-list .match-entry')");await evaluate(`document.documentElement.dataset.theme='${theme}'`);
  if(!process.env.R264_CASE || process.env.R264_CASE==='P3'){
   await until("document.querySelector('.match-filterbar [data-history-filter-load],.match-filterbar [data-af-open]')");
   await click('.match-filterbar [data-history-filter-load],.match-filterbar [data-af-open]');
   await until("document.querySelector('[data-af-menu]')?.hidden===false");
   await click('[data-af-category="result"]');await click('[data-af-option="win"]');
   await click('[data-af-page="saved"]');
   if(storage!=='saved'){await evaluate("document.querySelector('[data-af-name]').value='常用测试'");await click('[data-af-save]');}
   await click('[data-af-rename="0"]');await evaluate("document.querySelector('[data-af-rename-input]').value='重命名完成'");await click('[data-af-rename-save="0"]');
   assert(await evaluate("JSON.parse(localStorage.getItem('deep-legends-history-presets-v1'))[0].name==='重命名完成'"));
   await shot('p3-filter');await click('[data-af-open]');await click('[data-match-more-select] [data-app-select-trigger]');
   assert(await evaluate("!document.querySelector('[data-match-more-select] [data-app-select-menu]').hidden"));await click('[data-match-more-select] [data-app-select-trigger]');
   results.push({theme,storage,case:'P3',firstClickOpens:true,conditionSelected:true,renameSaved:true,moreModesOpen:true});
  }
  if(!process.env.R264_CASE || process.env.R264_CASE==='P1'){
   await evaluate("window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{playerRef:'player_fixture_search',gameName:'搜索测试',region:'',serverId:'HN1',source:'search'}}))");
   await until("document.querySelectorAll('[data-player-tab-wrap]').length===2");
   await click('[data-match-filter="solo"]');
   await evaluate("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");
   await click('[data-history-filter-load],[data-af-open]');await until("document.querySelector('[data-af-menu]')?.hidden===false");
   await click('[data-af-category="result"]');await click('[data-af-option="win"]');await click('[data-af-open]');
   await evaluate("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");
   await evaluate("window.__r264SearchList=document.querySelector('.match-list');document.getElementById('app-scroll').scrollTop=400;window.__r264SearchScroll=document.getElementById('app-scroll').scrollTop;window.__r264BeforeMax=document.getElementById('app-scroll').scrollHeight-document.getElementById('app-scroll').clientHeight;window.__r264ClosedAt=performance.now()");
   await evaluate("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");
   await evaluate("window.__r264SearchScroll=document.getElementById('app-scroll').scrollTop;window.__r264ClosedAt=performance.now();window.__r264SetStatus({connected:false,clientDiscovery:'exiting'})");
   await until("!document.querySelector('[data-player-tab-wrap=\"current\"]')");
   assert(await evaluate("document.querySelector('.player-group-count').textContent==='0' && document.querySelectorAll('[data-player-tab-wrap]').length===0 && !document.querySelector('#client-launchpad').hidden && document.querySelector('#startup-loading').hidden && window.__r264OverlayShown===0 && !document.querySelector('#overview-content').children.length"));
   const closeMs=await evaluate("performance.now()-window.__r264ClosedAt");assert(closeMs<=300,'self tab close exceeded 300ms');
   await shot('p1-closed');
   await evaluate("window.__r264SetStatus({connected:true,identityReady:true,sgpReady:true,clientDiscovery:'connected'})");
   await until("document.querySelectorAll('[data-player-tab-wrap]').length===2");
   await evaluate("[...document.querySelectorAll('[data-player-tab]')].find(n=>n.dataset.playerTab!=='current').click()");
   await evaluate("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");
   const restored=await evaluate("({solo:document.querySelector('[data-match-filter=\"solo\"]').classList.contains('is-active'),condition:document.querySelector('[data-af-conditions]')?.textContent.includes('胜利'),scroll:document.getElementById('app-scroll').scrollTop,expected:window.__r264SearchScroll,max:document.getElementById('app-scroll').scrollHeight-document.getElementById('app-scroll').clientHeight,beforeMax:window.__r264BeforeMax})");
   assert(restored.solo && restored.condition && restored.scroll===restored.expected,JSON.stringify(restored));
   results.push({theme,storage,case:'P1',visibleCount:0,launchpadVisible:true,overlay:false,reconnectedSearch:true,closeMs});
  }
  if(!process.env.R264_CASE || process.env.R264_CASE==='P2'){
  await click('#section-favorites');
  await evaluate("window.__r264Conflict=true;window.__r264SetStatus({snapshotReady:false,syncing:false,lastError:''});for(const stream of window.__r264Streams)stream.onmessage?.({data:'refresh-started'})");
  await until("window.__r264EnsureCalls===1 && document.querySelector('#skin-grid')?.textContent.includes('客户端连接中')");
  const recoveryStarted=Date.now();await evaluate("window.__r264Conflict=false;window.__r264SetStatus({})");
  await until("window.__r264EnsureCalls===2 && !document.querySelector('#skin-grid')?.textContent.includes('客户端连接中') && document.querySelector('#skin-grid .skin-card')");
  assert(Date.now()-recoveryStarted<=1000,'collection recovery exceeded 1 second');await shot('p2-collection');
  results.push({theme,storage,case:'P2',ensureCalls:2,recoveryMs:Date.now()-recoveryStarted});
  }
  await click('#section-favorites');await click('#rarity-button');assert(await evaluate("!document.querySelector('#rarity-menu').hidden"));await click('#rarity-menu [data-rarity="epic"]');
  assert(await evaluate("document.querySelector('#rarity-menu [data-rarity=\"epic\"]').getAttribute('aria-checked')==='true'"));
  await click('#section-settings');await until("!document.querySelector('#settings-panel').hidden");
  await click('#setting-theme + .app-select [data-app-select-trigger]');assert(await evaluate("!document.querySelector('#setting-theme + .app-select [data-app-select-menu]').hidden"));
  if(storage==='empty' && (!process.env.R264_CASE || ['P5','P6'].includes(process.env.R264_CASE))){
   const navigateScenario=async scenario=>{await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview&r264Scenario='+scenario});await until("window.__r264Scenario=== "+JSON.stringify(scenario)+" && window.__r264SetStatus && document.querySelector('.summoner-strip')");await evaluate(`document.documentElement.dataset.theme='${theme}'`);};
   for(const scenario of ['official-large','official-compact','official-inline']){
    await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});await navigateScenario(scenario);
    if(scenario==='official-inline'){await until("document.querySelector('.match-entry [data-toggle-match]')");await click('.match-entry [data-toggle-match]');await click('.match-entry [data-match-detail="build"]');await until("document.querySelector('.service-outage-inline')");assert(await evaluate("document.querySelectorAll('.service-outage-inline').length===1 && !document.querySelector('.build-detail').textContent.match(/HTTP|SGP|LCU|网关/)"));}
    else{await until(scenario==='official-large'?"document.querySelector('.service-outage')":"document.querySelector('.service-outage-compact')");assert(await evaluate("!document.querySelector('.matches-column').textContent.match(/HTTP|SGP|LCU|网关/)"));if(scenario==='official-compact')assert(await evaluate("document.querySelectorAll('.match-list .match-entry').length===1"));}
    for(const width of [1500,780]){await call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});await evaluate("document.querySelector('.service-outage,.service-outage-compact,.service-outage-inline').scrollIntoView({block:'center'})");await evaluate("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");await shot('p5-'+scenario+'-'+width);const sizes=await evaluate("(()=>{const n=document.querySelector('.service-outage-disc'),i=document.querySelector('.service-outage-icon');return {disc:n?.getBoundingClientRect().width,icon:i.getBoundingClientRect().width}})()");if(scenario==='official-large')assert.deepEqual(sizes,{disc:width===780?72:88,icon:width===780?40:48});results.push({theme,case:'P5.1',scenario,width,...sizes});}
   }
   await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});await navigateScenario('official-auto');await until("document.querySelector('[data-history-countdown]') && document.querySelector('[data-history-countdown]').textContent.includes('2 秒')");await until("window.__r264OverviewRequests===2 && !document.querySelector('.service-outage') && document.querySelector('.match-entry')");results.push({theme,case:'P5.1-auto',retryCalls:2,recovered:true});
   for(const scenario of ['foreign-empty','foreign-partial']){
    await navigateScenario(scenario);await until("document.querySelector('.match-entry')");await evaluate("window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{gameName:'外服测试',tagLine:'KR1',region:'kr',source:'search'}}))");
    await until("window.__r264OverviewStarted && document.querySelector('.summoner-strip')?.textContent.includes('外服测试')");const cardsMs=await evaluate("performance.now()-window.__r264OverviewStarted");assert(cardsMs<=2000,'foreign known cards delayed');
    await until("performance.now()-window.__r264OverviewStarted>=3500");assert(await evaluate("!document.querySelector('.service-outage,.service-outage-compact')"),'R266 must wait for an actual foreign failure');
    await until(scenario==='foreign-empty'?"document.querySelector('.service-outage')":"document.querySelector('.service-outage-compact')");const switchMs=await evaluate("performance.now()-window.__r264OverviewStarted");assert(switchMs>=7000 && switchMs<8500,'foreign failure state delay '+switchMs);assert(await evaluate("!document.querySelector('#overview-content .gameplay-skeleton') && document.querySelectorAll('#overview-content .service-outage,#overview-content .service-outage-compact').length===1"),await evaluate("JSON.stringify({skeletons:[...document.querySelectorAll('.gameplay-skeleton')].map(n=>n.parentElement.id),outages:[...document.querySelectorAll('.service-outage,.service-outage-compact')].map(n=>n.parentElement.outerHTML.slice(0,500))})"));
    await shot('p6-'+scenario+'-after-3s');await until("window.__r264OverviewFinished");
    if(scenario==='foreign-partial'){const text=await evaluate("document.querySelector('.career-column').textContent");assert(!/近 0 场|未发现单双排/.test(text));assert.equal((text.match(/战绩未读全/g)||[]).length,5,await evaluate("JSON.stringify({text:document.querySelector('.career-column').textContent,data:document.querySelector('#overview-content')._overviewViewTab?.data && {matches:document.querySelector('#overview-content')._overviewViewTab.data.matches.length,requested:document.querySelector('#overview-content')._overviewViewTab.data.historyRequested,loaded:document.querySelector('#overview-content')._overviewViewTab.data.historyLoaded,pagination:document.querySelector('#overview-content')._overviewViewTab.data.pagination},requests:window.__r264OverviewRequests})"));assert(await evaluate("document.querySelectorAll('.match-entry').length===1"));await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1900,deviceScaleFactor:1,mobile:false});await evaluate("document.getElementById('app-scroll').scrollTop=0");await shot('p6-partial-weak-production');await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});}
    results.push({theme,case:'P6',scenario,cardsMs,switchMs,failedMatches:scenario==='foreign-partial'?9:10});
   }
   await navigateScenario('partial-read-proposal');await until("document.querySelector('.recent-ranked-section')?.textContent.includes('近 1 场')");await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1900,deviceScaleFactor:1,mobile:false});await shot('p6-partial-read-proposal');await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});results.push({theme,case:'P6-proposal-only',proposal:'one read match; no inferred radar baseline'});
  }

  closedSessions.add(sessionId);await send('Target.closeTarget',{targetId});
 }
 assert.equal(violations.length,0,JSON.stringify(violations));assert.equal(errors.length,0,JSON.stringify(errors));
 fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({host:{platform:process.platform,arch:process.arch,node:process.version},completedAt:new Date().toISOString(),scope:'real Chromium on recorded host; public production binary/CSP; synthetic API data; no Windows/real LCU claim',csp,results,errors,violations,canceledInterceptions},null,2));
 console.log(JSON.stringify({cases:results.length,errors:errors.length,violations:violations.length}));
}
start().catch(e=>{fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'failure.json'),JSON.stringify({error:e.message,results,errors,violations},null,2));console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();chrome?.kill();backend?.kill();setTimeout(()=>fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100}),250).unref();});
