'use strict';
// Real Chromium against a public binary and production CSP. API/image inputs
// are explicitly synthetic; this does not claim real Windows or LCU evidence.
const {spawn,execFileSync}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=process.env.R265_BROWSER_OUT || path.join(os.tmpdir(),'r265-browser'),tmp=fs.mkdtempSync(path.join(os.tmpdir(),'r265-browser-'));
const errors=[],violations=[],results=[],closed=new Set(),canceled=new Set();let backend,chrome,ws;
const fixture=`
 const r265LoadedStatistics=window.__r265LoadedStatistics;
 const r265Sample=(data,k)=>{
  const loaded=r265LoadedStatistics[String(k)],cards=r265LoadedStatistics[String(k)];
  data.matches=structuredClone(loaded.matches);
  for(const key of ['overall','recentRanked','ability','positions','rankedQueues','recentPlayers','activityHours'])data[key]=structuredClone(cards[key]??(key==='ability'?null:[]));
  window.__r265FixtureLoaded=loaded;
  return data;
 };
 if(pathname==='/api/gameplay/season-summary')return Promise.resolve(new Response(JSON.stringify({available:false,champions:[],overall:{}}),{headers:{'Content-Type':'application/json'}}));
 if(pathname==='/api/gameplay/overview' && new URLSearchParams(location.search).has('recovery')){
  let request={};try{request=JSON.parse(init?.body || '{}')}catch{}
  if(request.region==='kr' || request.playerRef==='player_r265_fixture'){
   const scenario=new URLSearchParams(location.search).get('recovery');
   (window.__r265RecoveryRequests ||= []).push({at:Date.now(),retryDetails:Boolean(request.retryDetails)});
   const round=window.__r265RecoveryRequests.length,data=structuredClone(overview),k=scenario==='rounds'?1:9;
   data.player={...data.player,region:'kr',playerRef:'player_r265_fixture',gameName:'外服测试',tagLine:'KR1',isCurrent:false};
   r265Sample(data,k);data.historyRequested=10;data.historyLoaded=k;data.pagination={count:10,hasMore:false,partial:true};
   data.capabilities=data.capabilities.filter(c=>!c.name.startsWith('match'));data.capabilities.push({name:'match-details',state:'failed',count:k,detail:'战绩服务暂时不可用'});
   if(scenario==='quota' && round===1){const frames=[{type:'cards',overview:data},{type:'error',status:429,error:'查询额度恢复中，请稍后重试',kind:'rate-limited',retryAfter:3}];return Promise.resolve(new Response(frames.map(f=>JSON.stringify(f)).join('\\n')+'\\n',{headers:{'Content-Type':'application/x-ndjson'}}));}
   if(round===1 || scenario==='rounds')return Promise.resolve(new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}}));
   const encoder=new TextEncoder();return Promise.resolve(new Response(new ReadableStream({start(controller){
    init?.signal?.addEventListener('abort',()=>{(window.__r265AbortedRequests ||= []).push({at:Date.now(),round});try{controller.error(new DOMException('request canceled','AbortError'))}catch{}},{once:true});
    for(let i=9;i<=10;i++)setTimeout(()=>{if(init?.signal?.aborted){try{controller.close()}catch{};return;}const d=r265Sample({...data,historyLoaded:i,pagination:{count:10,hasMore:false,partial:i<10},capabilities:[{name:'match-details',state:i<10?'failed':'available',count:i}]},i);controller.enqueue(encoder.encode(JSON.stringify({type:i===10?'complete':'cards',overview:d})+'\\n'));if(i===10)controller.close();},(i-8)*(scenario.endsWith('-running')?5000:150));
   }}),{headers:{'Content-Type':'application/x-ndjson'}}));
  }
 }
 if(pathname==='/api/gameplay/overview' && new URLSearchParams(location.search).has('k')){
  let request={};try{request=JSON.parse(init?.body || '{}')}catch{}
  if(request.region==='kr' || request.playerRef==='player_r265_fixture'){
   const k=Number(new URLSearchParams(location.search).get('k')),data=structuredClone(overview);
   data.player={...data.player,region:'kr',playerRef:'player_r265_fixture',gameName:'外服测试',tagLine:'KR1',isCurrent:false};
   r265Sample(data,k);data.historyRequested=10;data.historyLoaded=k;data.pagination={begIndex:0,count:10,filter:'all',hasMore:false,partial:k<10};
   data.capabilities=data.capabilities.filter(c=>!c.name.startsWith('match'));data.capabilities.push({name:'match-history',state:'available',count:10},{name:'match-details',state:k<10?'failed':'available',count:k,detail:k<10?'战绩服务暂时不可用':''});
   if(!k){const frames=[{type:'cards',overview:data},{type:'error',status:503,error:'战绩服务暂时不可用',kind:'network',retryAfter:120}];return Promise.resolve(new Response(frames.map(f=>JSON.stringify(f)).join('\\n')+'\\n',{headers:{'Content-Type':'application/x-ndjson'}}));}
   return Promise.resolve(new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}}));
  }
 }
`;
async function main(){
 fs.mkdirSync(out,{recursive:true});
 const statsPath=path.join(tmp,'loaded-stats.json');
 execFileSync('go',['test','-count=1','-run','^TestR265BrowserLoadedStatsFixture$','./backend'],{cwd:root,env:{...process.env,R265_STATS_FIXTURE_OUT:statsPath},stdio:'pipe'});
 const loadedStats=JSON.parse(fs.readFileSync(statsPath,'utf8'));
 fs.copyFileSync(statsPath,path.join(out,'loaded-stats-fixture.json'));
 backend=spawn(process.env.R265_BACKEND || path.join(os.tmpdir(),'r265-public'),['--desktop'],{cwd:root,env:{...process.env,LOL_LOOT_DATA_DIR:path.join(tmp,'data')},stdio:['ignore','pipe','pipe']});
 const ready=await new Promise((resolve,reject)=>{let data='';const timer=setTimeout(()=>reject(Error('Backend startup timeout')),20000);backend.on('error',reject);backend.stdout.on('data',c=>{data+=c;for(const line of data.split('\n'))try{const v=JSON.parse(line.replace(/^LOOT_READY /,''));if(v.baseUrl){clearTimeout(timer);resolve(v);}}catch{}});});
 const csp=(await fetch(ready.bootstrapUrl)).headers.get('content-security-policy');assert(csp.includes("script-src 'self'") && !csp.includes('unsafe-inline'));
 chrome=spawn(process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--no-sandbox','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--remote-debugging-port=0',`--user-data-dir=${path.join(tmp,'chrome')}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const endpoint=await new Promise((resolve,reject)=>{let data='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);chrome.on('error',reject);chrome.stderr.on('data',c=>{data+=c;const m=data.match(/DevTools listening on (ws:\/\/\S+)/);if(m){clearTimeout(timer);resolve(m[1]);}});});
 ws=new WebSocket(endpoint);await new Promise(resolve=>ws.addEventListener('open',resolve,{once:true}));
 let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [ok,no]=pending.get(m.id);pending.delete(m.id);m.error?no(Error(JSON.stringify(m.error))):ok(m.result);}
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);
  if(m.method==='Network.loadingFailed' && m.params.canceled)canceled.add(m.sessionId+':'+m.params.requestId);
  if(m.method==='Runtime.bindingCalled')violations.push(JSON.parse(m.params.payload));
  if(m.method==='Fetch.requestPaused')void(async()=>{
   const {requestId,request}=m.params,url=new URL(request.url),fulfill=(body,type)=>send('Fetch.fulfillRequest',{requestId,responseCode:200,responseHeaders:[{name:'Content-Type',value:type}],body:Buffer.from(body).toString('base64')},m.sessionId);
   if(['/api/image','/api/champion-asset'].includes(url.pathname))await fulfill('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="#345366"/></svg>','image/svg+xml');
   else if(url.pathname==='/demo-data.js'){let body=await(await fetch(request.url)).text();body=body.replace('    const params = new URLSearchParams(url.split("?")[1] || "");','    const params = new URLSearchParams(url.split("?")[1] || "");'+fixture);await fulfill(body,'text/javascript');}
   else if(request.url.startsWith(ready.baseUrl+'/'))await send('Fetch.continueRequest',{requestId},m.sessionId);
   else await send('Fetch.failRequest',{requestId,errorReason:'BlockedByClient'},m.sessionId);
  })().catch(async e=>{if(closed.has(m.sessionId))return;if(e.message.includes('Invalid InterceptionId')){await new Promise(r=>setTimeout(r,50));if(canceled.has(m.sessionId+':'+m.params.networkId))return;}errors.push({message:e.message});});
 });
 async function page(theme,k,width=1500,scenario){
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
  const until=async expr=>{for(let i=0;i<200;i++){if(await evaluate(expr))return;await new Promise(r=>setTimeout(r,50));}throw Error('timeout '+expr+' '+JSON.stringify(await evaluate("({url:location.href,ready:document.readyState,hidden:document.hidden,section:document.querySelector('[data-section].is-active')?.dataset.section,body:document.body.textContent.slice(0,300),view:document.querySelector('#overview-content')?.textContent.slice(0,1200),demo:document.body.classList.contains('is-demo'),gameplay:!!window.deepLegendsGameplay,requests:window.__r265RecoveryRequests,styles:[...document.querySelectorAll('link[rel=stylesheet]')].map(n=>n.href)})")));};
  const click=s=>evaluate(`document.querySelector(${JSON.stringify(s)}).click()`),frame=()=>evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const shot=async name=>{await frame();const v=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,name+'.png'),Buffer.from(v.data,'base64'));};
  await send('Target.activateTarget',{targetId});for(const m of ['Network.enable','Page.enable','Runtime.enable'])await call(m);await call('Runtime.addBinding',{name:'r265CSP'});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:"localStorage.clear();window.EventSource=class{addEventListener(){}close(){}};document.addEventListener('securitypolicyviolation',e=>r265CSP(JSON.stringify({directive:e.effectiveDirective,blockedURI:e.blockedURI})));window.__r265LoadedStatistics="+JSON.stringify(loadedStats)});
  await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width,height:scenario?1100:1900,deviceScaleFactor:1,mobile:false});
  await call('Network.setCookie',{name:'lol_loot_token',value:ready.token,url:ready.baseUrl,httpOnly:true});await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview'+(k==null?'':'&k='+k)+(scenario?'&recovery='+scenario:'')});
  await until("document.querySelector('.match-list .match-entry')");await evaluate(`document.documentElement.dataset.theme=${JSON.stringify(theme)}`);
  return {call,evaluate,until,click,frame,shot,async close(){closed.add(sessionId);await send('Target.closeTarget',{targetId});}};
 }
 for(const theme of ['dark','light']){
  if(!process.env.R265_CASE || process.env.R265_CASE==='filters'){
   const p=await page(theme);await p.click('[data-history-filter-load],[data-af-open]');await p.until("document.querySelector('[data-af-menu]')?.hidden===false");
   for(const hasCount of [false,true]){
    if(hasCount){await p.click('[data-af-category="result"]');await p.click('[data-af-option="win"]');}
    for(const selected of [false,true])for(const expanded of [true,false]){
     if(!expanded)await p.click('[data-af-open]');
     await p.evaluate(`document.querySelector('.af-select').classList.toggle('is-active',${selected})`);
     const geometry=await p.evaluate(`(()=>{const b=document.querySelector('[data-af-open]'),m=document.querySelector('[data-match-more-select] .app-select-trigger'),center=n=>{const r=n.getBoundingClientRect();return r.y+r.height/2},arrow=n=>{const s=getComputedStyle(n,'::after'),r=n.getBoundingClientRect(),border=parseFloat(getComputedStyle(n).borderTopWidth),height=parseFloat(s.height)+parseFloat(s.borderTopWidth)+parseFloat(s.borderBottomWidth);return r.y+border+parseFloat(s.top)+height/2+new DOMMatrix(s.transform).f},ys=[...b.children].map(center),moreYs=[...m.children].map(center),r=b.getBoundingClientRect();ys.push(arrow(b));moreYs.push(arrow(m));return {ys,moreYs,button:center(b),more:center(m),height:r.height,moreHeight:m.getBoundingClientRect().height}})()`);
     assert(Math.max(...geometry.ys)-Math.min(...geometry.ys)<=1,JSON.stringify(geometry));assert(Math.max(...geometry.moreYs)-Math.min(...geometry.moreYs)<=1,JSON.stringify(geometry));assert(Math.abs(geometry.button-geometry.more)<=1);assert.equal(geometry.height,geometry.moreHeight);
     results.push({case:'trigger',theme,hasCount,selected,expanded,...geometry});
     if(!expanded)await p.click('[data-af-open]');
    }
   }
   await p.click('[data-af-page="conditions"]');const conditions=await p.evaluate("(()=>{const r=document.querySelector('[data-af-menu]').getBoundingClientRect();return {height:r.height,top:r.top,max:getComputedStyle(document.querySelector('[data-af-menu]')).maxHeight}})()");assert.equal(conditions.height,640);await p.shot(theme+'-filter-conditions');
   const heights=[];
   for(const count of [0,1,12]){
    await p.evaluate(`localStorage.setItem('deep-legends-history-presets-v1',JSON.stringify(Array.from({length:${count}},(_,i)=>({name:'常用 '+(i+1),conditions:{result:{values:['win'],not:false}}}))))`);
    await p.click('[data-af-page="saved"]');
    const sizes=await p.evaluate("(()=>{const menu=document.querySelector('[data-af-menu]'),list=menu.querySelector('.af-saved-list'),footer=menu.querySelector('.af-savebar'),m=menu.getBoundingClientRect(),f=footer.getBoundingClientRect(),controls=[...footer.querySelectorAll('input,button')].map(n=>n.getBoundingClientRect());return {height:m.height,top:m.top,max:getComputedStyle(menu).maxHeight,scroll:list.scrollHeight>list.clientHeight,footerVisible:f.top>=m.top && f.bottom<=m.bottom && controls.every(r=>r.top>=m.top && r.bottom<=m.bottom)}})()");
    const frames=await p.evaluate("new Promise(resolve=>{const rows=[];const frame=()=>{const n=document.querySelector('[data-af-menu]'),r=n.getBoundingClientRect();rows.push({top:r.top,height:r.height,visible:!n.hidden && getComputedStyle(n).visibility!=='hidden' && Number(getComputedStyle(n).opacity)>0});if(rows.length<3)requestAnimationFrame(frame);else resolve(rows)};requestAnimationFrame(frame)})");
    assert(frames.every(f=>f.top===sizes.top && f.height===sizes.height && f.visible),JSON.stringify(frames));
    assert.equal(sizes.top+sizes.height/2,conditions.top+conditions.height/2);assert.equal(sizes.max,"640px");assert(sizes.footerVisible);heights.push(sizes.height);
    if(count<12)assert(sizes.height<640 && !sizes.scroll,JSON.stringify(sizes));else assert(sizes.height===640 && sizes.scroll,JSON.stringify(sizes));
    results.push({case:'saved',theme,count,...sizes});await p.shot(theme+'-filter-saved-'+count);
   }
   assert(heights[0]<heights[1] && heights[1]<heights[2],JSON.stringify(heights));await p.close();
  }
  if(!process.env.R265_CASE || process.env.R265_CASE==='samples')for(const k of [10,7,6,3,1,0])for(const width of [1500,780]){
   const p=await page(theme,k,width);await p.evaluate("window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{gameName:'外服测试',tagLine:'KR1',region:'kr',source:'search'}}))");
   await p.until("document.querySelector('.summoner-strip')?.textContent.includes('外服测试')");await p.until(k===0?"document.querySelector('.service-outage')":"document.querySelectorAll('.match-list .match-entry').length==="+k);
   const samples=await p.evaluate("({weak:(document.querySelector('.career-column').textContent.match(/战绩未读全/g)||[]).length,labels:[...document.querySelectorAll('.career-column .history-sample-label')].map(n=>n.textContent),radar:!!document.querySelector('.career-column .ability-radar')})");
   if(k<6){assert.equal(samples.weak,5);assert.equal(samples.labels.length,0);assert.equal(samples.radar,false);}else if(k<10){assert.equal(samples.weak,0);assert.deepEqual(samples.labels,Array(5).fill(`基于最近 ${k} 场`));}else assert.equal(samples.labels.length,0);
   if(!k)assert(await p.evaluate("document.querySelector('.service-outage p').textContent==='暂时没有读取到对局，恢复后会自动补齐。'"));
   await p.shot(`${theme}-k${k}-${width}`);
   if(width===780){await p.click('[data-open-career-dialog]');await p.until("document.querySelector('#career-dialog').open");await p.shot(`${theme}-k${k}-${width}-cards-top`);}
   const cards=await p.evaluate(`(()=>{
    const root=${width===780?"document.querySelector('#career-dialog')":"document.querySelector('.career-column')"},source=window.__r265FixtureLoaded,subject='player_r265_fixture',matches=source.matches;
    const keys=['recent-ranked','ability','positions','recent-players','activity'],section=key=>key==='recent-ranked'?root.querySelector('.recent-ranked-section'):key==='ability'?root.querySelector('.ability-section'):key==='activity'?root.querySelector('.activity-section'):[...root.querySelectorAll('.career-section')].find(n=>n.querySelector('h3')?.textContent===({positions:'位置偏好','recent-players':'最近一起玩'})[key]),recent=section('recent-ranked'),positions=section('positions'),players=section('recent-players');
    const expectedWins=matches.filter(m=>m.participants.find(p=>p.playerRef===subject).win).length,positionCounts={};for(const m of matches){const position=m.participants.find(p=>p.playerRef===subject).position;positionCounts[position]=(positionCounts[position]||0)+1;}
    const headers=keys.map(key=>{const header=section(key).querySelector(':scope > header'),r=header.getBoundingClientRect(),items=[...header.children].map(n=>{const b=n.getBoundingClientRect(),range=document.createRange();range.selectNodeContents(n);return {tag:n.tagName,text:n.textContent.trim(),left:b.left,right:b.right,center:b.top+b.height/2,lines:[...range.getClientRects()].map(v=>v.top)}});return {card:key,text:header.textContent.trim(),width:r.width,clientWidth:header.clientWidth,scrollWidth:header.scrollWidth,left:r.left,right:r.right,items};});
    return {headers,expectedWins,expectedLosses:matches.length-expectedWins,wins:recent.querySelector('.recent-ranked-record small b')?.textContent,losses:recent.querySelector('.recent-ranked-record small i')?.textContent,common:[...players.querySelectorAll('.recent-player small')].map(n=>Number(n.textContent.match(/共同对局 (\\d+) 场/)[1])),positionRows:[...positions.querySelectorAll('.position-row')].map(n=>({label:n.querySelector('strong').textContent,value:Number(n.querySelector('progress').value),text:n.querySelector('b').textContent})),expectedPositions:source.rankedQueues['420'].positions,positionCounts,activity:[...section('activity').querySelectorAll('.activity-cell')].map(n=>Number(n.dataset.tooltip.match(/· (\\d+) 场/)[1])),loaded:matches.length};
   })()`);
   assert.equal(cards.loaded,k);assert.equal(cards.headers.length,5);
   for(const header of cards.headers){assert(header.scrollWidth<=header.clientWidth,JSON.stringify(header));assert(Math.max(...header.items.map(n=>n.center))-Math.min(...header.items.map(n=>n.center))<=1,JSON.stringify(header));for(const item of header.items){assert(item.left>=header.left && item.right<=header.right,JSON.stringify(header));if(item.tag!=='DIV' && item.lines.length)assert(Math.max(...item.lines)-Math.min(...item.lines)<=1,JSON.stringify(header));}}
   const byKey=Object.fromEntries(cards.headers.map(h=>[h.card,h.text]));
   if(k<10){assert(byKey['recent-ranked'].startsWith('近期排位'));assert.doesNotMatch(byKey['recent-ranked'],/近 \d+ 场排位/);assert.doesNotMatch(byKey.positions.replace(/基于最近 \d+ 场/g,''),/近 \d+ 场/);assert.doesNotMatch(byKey['recent-players'],/最近 30 天/);}else{assert(byKey['recent-ranked'].startsWith('近 10 场排位'));assert.match(byKey.positions,/近 10 场/);assert.match(byKey['recent-players'],/最近 30 天/);}
   if(k>=6){assert.equal(Number(cards.wins.match(/\d+/)[0]),cards.expectedWins);assert.equal(Number(cards.losses.match(/\d+/)[0]),cards.expectedLosses);assert.equal(cards.expectedWins+cards.expectedLosses,k);assert(cards.common.length>0 && cards.common.every(n=>n<=k));for(const row of cards.positionRows){const expected=cards.expectedPositions.find(v=>v.label===row.label)||{position:{上单:'top',打野:'jungle',中单:'middle',下路:'bottom',辅助:'utility'}[row.label],share:0};assert(expected.position);assert.equal(row.value,expected.share);assert.equal(row.value,Math.round((cards.positionCounts[expected.position]||0)*100/k));}assert.equal(cards.activity.reduce((a,b)=>a+b,0),k);}
   if(width===780){await p.evaluate("document.querySelector('.career-dialog-grid').scrollTop=100000");await p.shot(`${theme}-k${k}-${width}-cards-bottom`);}
   results.push({case:'samples',theme,k,width,...samples,cards});await p.close();
  }
 }
 if(!process.env.R265_CASE || process.env.R265_CASE==='recovery'){
  for(const scenario of ['complete','close','switch','hidden','close-running','switch-running','hidden-running','quota','rounds']){
   const p=await page('dark',null,1500,scenario);await new Promise(r=>setTimeout(r,5500));await p.evaluate("window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{gameName:'外服测试',tagLine:'KR1',region:'kr',source:'search'}}))");
   await p.until("window.__r265RecoveryRequests?.length===1 && document.querySelectorAll('.match-list .match-entry').length==="+(scenario==='rounds'?1:9));
   if(scenario==='complete'){
    // Initial catalog hydration is independent of retry; measure the retained
    // body after that first render has settled, before the 5s retry starts.
    await new Promise(r=>setTimeout(r,1000));assert.equal(await p.evaluate('window.__r265RecoveryRequests.length'),1);
    await p.evaluate("window.__r265Writes=[];const prop=Object.getOwnPropertyDescriptor(Element.prototype,'innerHTML');Object.defineProperty(Element.prototype,'innerHTML',{...prop,set(v){if(this.id==='overview-content')window.__r265Writes.push({stack:new Error().stack,running:this._overviewViewTab?.detailRecovery?.running,round:this._overviewViewTab?.detailRecovery?.round,loading:this._overviewViewTab?.loading});prop.set.call(this,v)}});window.__r265Layout=document.querySelector('#overview-content .overview-layout');window.__r265List=document.querySelector('#overview-content .match-list');window.__r265Row=window.__r265List.firstElementChild;document.getElementById('app-scroll').scrollTop=200;window.__r265Scroll=document.getElementById('app-scroll').scrollTop;");
    await new Promise(r=>setTimeout(r,5600));await p.until("document.querySelectorAll('.match-list .match-entry').length===10");
    const proof=await p.evaluate("({requests:window.__r265RecoveryRequests,layout:window.__r265Layout===document.querySelector('#overview-content .overview-layout'),list:window.__r265List===document.querySelector('#overview-content .match-list'),row:window.__r265Row===window.__r265List.firstElementChild,scroll:document.getElementById('app-scroll').scrollTop,before:window.__r265Scroll,writes:window.__r265Writes,labels:document.querySelectorAll('.career-column .history-sample-label').length})");
    assert.equal(proof.requests.length,2);assert.deepEqual(proof.requests.map(r=>r.retryDetails),[false,true]);assert(proof.layout && proof.list && proof.row,JSON.stringify(proof));assert(proof.before>0);assert.equal(proof.scroll,proof.before);assert.equal(proof.labels,0);results.push({case:'recovery',scenario,...proof});await p.shot('recovery-keeps-body-scroll');
   }else if(scenario==='quota'){
    await new Promise(r=>setTimeout(r,2000));assert.equal(await p.evaluate('window.__r265RecoveryRequests.length'),1);await new Promise(r=>setTimeout(r,3600));await p.until("document.querySelectorAll('.match-list .match-entry').length===10");results.push({case:'recovery',scenario,requests:await p.evaluate('window.__r265RecoveryRequests'),blockedDuringQuota:true});
   }else if(scenario==='rounds'){
    await new Promise(r=>setTimeout(r,67000));assert.equal(await p.evaluate('window.__r265RecoveryRequests.length'),4);assert(await p.evaluate("!!document.querySelector('[data-gameplay-retry]')"));await new Promise(r=>setTimeout(r,1000));assert.equal(await p.evaluate('window.__r265RecoveryRequests.length'),4);results.push({case:'recovery',scenario,requests:await p.evaluate('window.__r265RecoveryRequests'),bounded:true});
   }else{
    const running=scenario.endsWith('-running');if(running)await p.until('window.__r265RecoveryRequests.length===2');
    if(scenario.startsWith('close'))await p.click('[data-close-player]');
    if(scenario.startsWith('switch'))await p.evaluate("window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{gameName:'切换目标',tagLine:'JP1',region:'jp1',source:'search'}}))");
    // Explicit hidden-window event fixture exercises the production listener.
    if(scenario.startsWith('hidden'))await p.evaluate("Object.defineProperty(document,'hidden',{configurable:true,get:()=>true});document.dispatchEvent(new Event('visibilitychange'));");
    if(running)await p.until('window.__r265AbortedRequests?.length===1');await new Promise(r=>setTimeout(r,5600));assert.equal(await p.evaluate('window.__r265RecoveryRequests.length'),running?2:1);results.push({case:'recovery',scenario,noFurtherRequests:true,canceledRunning:running,aborts:await p.evaluate('window.__r265AbortedRequests || []')});
   }
   await p.close();
  }
  const p=await page('dark');await p.click('#section-settings');await p.until("document.querySelector('[data-riot-cache-clear]')");await p.click('[data-riot-cache-clear]');await p.until("document.querySelector('[data-riot-cache-clear]').parentElement.textContent.includes('缓存已清除')");results.push({case:'cache-clear',authenticatedRealEndpoint:true});await p.close();
 }
 assert.equal(errors.length,0,JSON.stringify(errors));assert.equal(violations.length,0,JSON.stringify(violations));
 fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({host:{platform:process.platform,node:process.version},csp,scope:'real Chromium; public production binary/CSP; synthetic API/image fixtures',results,errors,violations},null,2));console.log(JSON.stringify({cases:results.length,screenshots:fs.readdirSync(out).filter(f=>f.endsWith('.png')).length,errors:errors.length}));
}
main().catch(e=>{fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'failure.json'),JSON.stringify({error:e.message,results,errors,violations},null,2));console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();chrome?.kill();backend?.kill();setTimeout(()=>fs.rmSync(tmp,{recursive:true,force:true,maxRetries:8,retryDelay:100}),250).unref();});
