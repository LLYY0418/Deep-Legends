'use strict';
// CDP controls the actual installed program. Only the separately recorded 079
// preset-format fixture may supply the exact missing resource from its tag.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict'),{execFileSync}=require('node:child_process');
const presetKey='deep-legends-history-presets-v1';
const keys=['lol-loot-default-page','lol-loot-default-match-filter','lol-loot-match-count','lol-loot-mask-names','lol-loot-ui-scale','lol-loot-search-region','lol-loot-search-server-id','lol-loot-search-region-manual'];
const scriptSHA='74699f9576ccd8686fce0e95c38b718b96718baa46bd68e9068cf947f7ea38f3';
const tagSHA='049b30cad1ccd13fd8cbe785b25c5a56112f46a5';
async function waitForInstalledTarget({port,phase,out,now=Date.now,sleep=ms=>new Promise(r=>setTimeout(r,ms)),fetchTargets=async remaining=>{
 const response=await fetch(`http://127.0.0.1:${port}/json/list`,{signal:AbortSignal.timeout(Math.max(1,Math.min(1000,remaining)))});
 assert.equal(response.status,200,'CDP target list HTTP status');return response.json();
}}){
 const started=now(),deadline=started+30000,observations=[];let target,previous='',polls=0;
 const redactURL=value=>{try{const url=new URL(value);for(const key of url.searchParams.keys())if(/token|secret|password|credential/i.test(key))url.searchParams.set(key,'[redacted]');return url.toString();}catch{return value;}};
 try{
  for(;polls<150&&now()<deadline;polls++){
   let observation;try{
    const targets=await fetchTargets(deadline-now());assert(Array.isArray(targets),'CDP target list must be an array');
    observation={targets:targets.map(t=>({...t,url:redactURL(t.url)}))};
    target=targets.find(t=>t.type==='page'&&/^http:\/\/(127\.0\.0\.1|localhost):/.test(t.url));
   }catch(error){observation={error:error.message,cause:error.cause?.code};}
   const key=JSON.stringify(observation);if(key!==previous){observations.push({elapsedMs:now()-started,...observation});previous=key;}
   if(target)break;await sleep(Math.min(200,Math.max(0,deadline-now())));
  }
  assert(target,'installed Electron page did not become ready');return target;
 }finally{
  fs.writeFileSync(path.join(out,`r265-cdp-${phase}.json`),JSON.stringify({phase,port,limitMs:30000,elapsedMs:now()-started,polls:polls+Number(!!target),ready:!!target,observations},null,2));
 }
}
function assertCorruptControls(actual,before,follow) {
 const other=controls=>Object.fromEntries(Object.entries(controls).filter(([key])=>!['searchRegion','searchServer'].includes(key)));
 assert.deepEqual(other(actual),other(before));
 assert.equal(actual.searchRegion,'cn','P1 corrupt demo login must follow the CN client');
 assert.equal(follow.label,'艾欧尼亚','P1 corrupt demo login must show the client server name');
 assert(follow.events.some(e=>e.searchUI&&e.event==='client_follow_client'&&e.trigger==='login'&&e.fields?.to_group==='players'&&e.fields?.search_to==='HN1'&&e.fields?.applied),'P1 corrupt demo must record search client_follow_client login');
}
function assertFollowControls(actual,before,follow) {
 const other=controls=>Object.fromEntries(Object.entries(controls).filter(([key])=>!['searchRegion','searchServer'].includes(key)));
 assert.deepEqual(other(actual),other(before));
 assert.equal(actual.searchRegion,'cn','P1 startup follows the ready CN client');
 assert.equal(actual.searchServer,'','P1 startup follows the client without a manual server override');
 assert.equal(follow.label,'艾欧尼亚','P1 startup shows the ready client server name');
}
function assertReadControls(actual,before,follow) {
 assertFollowControls(actual,before,follow);
 assert(follow.events.some(e=>e.searchUI&&e.event==='client_follow_client'&&e.trigger==='login'&&e.fields?.to_group==='players'&&e.fields?.search_to==='HN1'&&e.fields?.applied),'P1 read must record search client_follow_client login');
}
function followCapturePrelude() {
 return `(()=>{window.__r265FollowEvents=[];window.__r265FollowCaptureStartedMs=performance.now();const wrap=fn=>function(...args){if(args[0]==='client_follow_client')window.__r265FollowEvents.push({event:args[0],trigger:args[1],fields:args[2],searchUI:new Error().stack.includes('updateSearchRegionStatus'),loadedMs:performance.now()});return fn?.apply(this,args);};let report=wrap(window.reportFlowDiagnostic);Object.defineProperty(window,'reportFlowDiagnostic',{configurable:true,get(){return report},set(fn){report=wrap(fn)}});return true})();window.EventSource=class{addEventListener(){}close(){}};(()=>{
 // Same installed-demo transport synchronization as the other upgrade phases.
 // Preserve the real fetch, Response, bytes and cancellation; suppress live SSE
 // only in this synthetic demo fixture, which already supplies client status.
 if(typeof window.fetch!=='function')return;
 const wrap=delegate=>function(...args){
  const result=Reflect.apply(delegate,window,args),input=args[0],url=new URL(typeof input==='string'?input:input?.url||input,location.href);
  if(url.origin!==location.origin||url.pathname!=='/api/status')return result;
  return Promise.resolve(result).then(async response=>{
   const signal=args[1]?.signal||input?.signal;
   while(!window.deepLegendsMatchCards){if(signal?.aborted)throw signal.reason;await new Promise(requestAnimationFrame);}
   if(signal?.aborted)throw signal.reason;return response;
  });
 };
 let current=wrap(window.fetch);Object.defineProperty(window,'fetch',{configurable:true,enumerable:true,get:()=>current,set:delegate=>{current=wrap(delegate)}});
})()`;
}
function clientFrameSnapshot() {
 const status=window.deepLegendsMatchCards?.live?.state?.status,client=status?.clientView,search=document.querySelector('#player-search-region');
 return {ready:client?.state==='ready'&&Number.isFinite(window.__r265FollowCaptureStartedMs),region:status?.clientRegion??client?.region??null,server:status?.serverId??client?.serverId??null,searchRegion:search?.dataset.region,searchServer:search?.dataset.serverId,label:document.querySelector('#player-search-region-label')?.textContent,loadedMs:performance.now()};
}
function navigationPending(error) {
 let protocol;try{protocol=JSON.parse(error.message);}catch{return false;}
 return protocol.code===-32000&&['Inspected target navigated or closed','Not attached to an active page','Execution context was destroyed.','Cannot find context with specified id','Cannot find default execution context'].includes(protocol.message);
}
async function waitForFollowedClient({evaluate,phase,now=Date.now,noProgressMs=20000,totalMs=120000}) {
 const started=now();
 return require('./lib/topbar-settle.cjs').waitForPage({now,noProgressMs,label:phase+' ready client stable across three frames',evaluate:async expression=>{
  if(now()-started>=totalMs)throw Error(phase+' client readiness total limit reached');
  try{return await evaluate(expression);}
  catch(error){
   if(!navigationPending(error))throw error;
   const protocol=JSON.parse(error.message);await new Promise(setImmediate);
   return {done:false,progress:['navigation-pending',protocol.message],snapshot:{state:'navigation-pending',message:protocol.message,method:error.cdpMethod||'Runtime.evaluate'}};
  }
 },sample:`(async()=>{const read=${clientFrameSnapshot};let previous;for(let frame=0;frame<3;frame++){await new Promise(r=>requestAnimationFrame(r));const snapshot=read(),progress=[snapshot.ready,snapshot.region,snapshot.server,snapshot.searchRegion,snapshot.searchServer,snapshot.label],key=JSON.stringify(progress);if(!snapshot.ready||previous!==undefined&&key!==previous)return {done:false,progress,snapshot};previous=key;if(frame===2)return {done:true,progress,snapshot};}})()`});
}
function assertRestartControls(actual,before,follow) {
 assertFollowControls(actual,before,follow);
 assert.deepEqual(actual,{...before,searchServer:''},'P1 restart startup must follow the ready client');
 assert.equal(follow.label,'艾欧尼亚','P1 restart login must show the client server name');
 assert(follow.events.some(e=>e.searchUI&&e.event==='client_follow_client'&&e.trigger==='login'&&e.fields?.to_group==='players'&&e.fields?.search_to==='HN1'&&e.fields?.applied),'P1 restart must record search client_follow_client login');
}
async function prepareFollowedStartup({call,evaluate,until,phase,save,shot,evidencePrefix='080'}) {
   // Complete the real loadURL before reloading this same installed document.
   // Capture startup diagnostics before app initialization, including early login.
   await require('./r276-installed-demo.cjs').waitInstalledStartup({until});
   await call('Page.addScriptToEvaluateOnNewDocument',{source:followCapturePrelude()});
   await evaluate('delete window.__r265FollowCaptureStartedMs;true');
   await call('Page.reload',{ignoreCache:false});
   try{const stable=await waitForFollowedClient({evaluate,phase});save(`${evidencePrefix}-${phase}-client-ready.json`,{phase,...stable.snapshot});}
   catch(error){
    const record={phase,error:error.message,method:error.cdpMethod,snapshot:error.pageWaitSnapshot};
    save(`${evidencePrefix}-${phase}-client-stalled.json`,record);
    try{record.clientStatus=await evaluate("(async()=>{const response=await fetch('/api/status'),raw=await response.json(),rendered=window.deepLegendsMatchCards?.live?.state?.status;const client=s=>({connected:s?.connected??null,region:s?.clientRegion??null,server:s?.serverId??null,state:s?.clientView?.state??null});return {httpStatus:response.status,demoEnabled:localStorage.getItem('lol-loot-demo')==='1',captureStartedMs:window.__r265FollowCaptureStartedMs,raw:client(raw),rendered:client(rendered),events:window.__r265FollowEvents||[]};})()");}catch(diagnosticError){record.diagnosticError={message:diagnosticError.message,method:diagnosticError.cdpMethod};}
    save(`${evidencePrefix}-${phase}-client-stalled.json`,record);
    try{await shot(`${evidencePrefix}-${phase}-client-stalled.png`);}catch(screenshotError){save(`${evidencePrefix}-${phase}-client-stalled-screenshot-error.json`,{phase,error:screenshotError.message,method:screenshotError.cdpMethod});}
    throw error;
   }
}
async function probe({port,phase,out}){
 assert(Number.isInteger(port)&&port>0&&port<65536);assert(['write-settings','write-presets','read','restart','corrupt'].includes(phase));fs.mkdirSync(out,{recursive:true});
 const target=await waitForInstalledTarget({port,phase,out});
 if(process.platform==='win32'){
  const expectedPID=Number(process.env.R265_EXPECTED_ELECTRON_PID);assert(Number.isInteger(expectedPID)&&expectedPID>0,'installed Electron PID proof missing');
  const ownerProof=require('./r276-windows-port-owner.cjs').verifyInstalledPortOwner({port,expectedPID,evidence:path.join(out,`r265-cdp-owner-${phase}.json`)});
  const processIDs=ownerProof.owners;
  assert(processIDs.length>0&&processIDs.every(pid=>pid===expectedPID),'CDP port belongs to another process');
 }
 const ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{ws.addEventListener('open',resolve,{once:true});ws.addEventListener('error',reject,{once:true})});
 let sequence=0,intercept=null;const pending=new Map(),resourceProof=[];
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [ok,no]=pending.get(m.id);pending.delete(m.id);m.error?no(Error(JSON.stringify(m.error))):ok(m.result);}else if(m.method==='Fetch.requestPaused')void intercept?.(m.params).catch(e=>resourceProof.push({error:e.message}));});
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++sequence,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method))},15000);pending.set(id,[r=>{clearTimeout(timer);resolve(r)},e=>{clearTimeout(timer);e.cdpMethod=method;reject(e)}]);ws.send(JSON.stringify({id,method,params}));});
 const evaluate=async expression=>{const v=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(v.exceptionDetails)throw Error(JSON.stringify(v.exceptionDetails));return v.result.value;};
 const until=async expression=>{for(let n=0;n<200;n++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100));}throw Error('installed UI condition timeout: '+expression+' '+JSON.stringify(await evaluate("({page:document.querySelector('#overview-content')?.textContent.slice(0,800),live:!document.querySelector('#live-panel')?.hidden,section:document.querySelector('.sidebar-nav-button.is-active')?.id})")))};
 const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
 const choose=async(id,value)=>{await click(`#${id} + .app-select [data-app-select-trigger]`);await click(`#${id} + .app-select [data-native-select-value="${value}"]`);assert.equal(await evaluate(`document.getElementById(${JSON.stringify(id)}).value`),value);};
 const snapshot=()=>evaluate(`(()=>{const storage=Object.fromEntries(Object.keys(localStorage).sort().map(k=>[k,localStorage.getItem(k)]));return {searchFollow:{label:document.querySelector('#player-search-region-label').textContent,events:window.__r265FollowEvents||[]},preferences:Object.fromEntries(${JSON.stringify(keys)}.map(k=>[k,localStorage.getItem(k)])),presets:localStorage.getItem(${JSON.stringify(presetKey)}),storage,controls:{defaultPage:document.querySelector('#setting-default-page').value,defaultFilter:document.querySelector('#setting-default-match-filter').value,matchCount:document.querySelector('#setting-match-count').value,maskNames:document.querySelector('#setting-mask-names').checked,scale:document.querySelector('#setting-ui-scale').value,searchRegion:document.querySelector('#player-search-region').dataset.region,searchServer:document.querySelector('#player-search-region').dataset.serverId}}})()`);
 const save=(name,data)=>fs.writeFileSync(path.join(out,name),JSON.stringify(data,null,2));
 const shot=async name=>{const png=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(out,name),Buffer.from(png.data,'base64'));};
 const demo=async origin=>require('./r276-installed-demo.cjs').navigateInstalledDemo({call,until,click,origin});
 const openFilter=async()=>{
  const visible=phase.startsWith('write-') ? "document.querySelector('[data-af-menu]')?.hidden===false" : "document.querySelector('dialog[data-af-menu]:modal')?.hidden===false";
  if(!await evaluate(visible))await click('[data-history-filter-load],[data-af-open]');
  await until("!!document.querySelector('[data-af-open]')");await until(visible);
 };
 const openSaved=async()=>{await openFilter();await click('[data-af-page="saved"]');};
 try{
  await call('Page.enable');await call('Runtime.enable');
  if(phase==='read'||phase==='restart'){
   await prepareFollowedStartup({call,evaluate,until,phase,save,shot});
  }
  await require('./r276-installed-demo.cjs').waitInstalledStartup({until});
  const initial=await evaluate("({origin:location.origin,defaultPage:document.querySelector('#setting-default-page').value,liveVisible:!document.querySelector('#live-panel').hidden})");
  if(phase==='write-settings'){
   // No Fetch interception, no missing-script repair in this phase.
   await demo(initial.origin);await click('#section-settings');await until("!document.querySelector('#settings-panel').hidden");
   await choose('setting-default-page','live');await choose('setting-default-match-filter','solo');await choose('setting-match-count','20');await choose('setting-ui-scale','1.25');
   if(!await evaluate("document.querySelector('#setting-mask-names').checked"))await click('#setting-mask-names');
   await click('#player-search-region');await click('#player-search-cn-toggle');await click('[data-region-option="cn"][data-server-id="HN1"]');
   await until("localStorage.getItem('lol-loot-default-page')==='live'&&localStorage.getItem('lol-loot-default-match-filter')==='solo'&&localStorage.getItem('lol-loot-match-count')==='20'&&localStorage.getItem('lol-loot-mask-names')==='true'&&localStorage.getItem('lol-loot-ui-scale')==='1.25'&&localStorage.getItem('lol-loot-search-server-id')==='HN1'");
   const data=await snapshot();save('079-unmodified-settings-before.json',{phase,resourceFixtureApplied:false,scope:'actual published 079 UI-written settings; demo player data',initial,...data});await shot('079-unmodified-settings.png');
  }else if(phase==='write-presets'){
   const settings=JSON.parse(fs.readFileSync(path.join(out,'079-unmodified-settings-before.json'),'utf8'));
   assert.equal(execFileSync('git',['rev-parse','v0.12.79^{commit}'],{encoding:'utf8'}).trim(),tagSHA);
   const body=execFileSync('git',['show','v0.12.79:backend/web/history-filters.js'],{maxBuffer:4*1024*1024});assert.equal(crypto.createHash('sha256').update(body).digest('hex'),scriptSHA);
   const originalStatus=await evaluate("fetch('/history-filters.js').then(r=>r.status)");assert.equal(originalStatus,404,'published 079 must expose the original missing resource');
   // The exact bytes stay in harness memory, outside candidate resources/build.
   intercept=async({requestId,request})=>{assert.equal(new URL(request.url).pathname,'/history-filters.js');const servedSHA=crypto.createHash('sha256').update(body).digest('hex');assert.equal(servedSHA,scriptSHA);resourceProof.push({originalStatus,tag:'v0.12.79',tagSHA,sourceSHA256:scriptSHA,servedSHA256:servedSHA,bytes:body.length});await call('Fetch.fulfillRequest',{requestId,responseCode:200,responseHeaders:[{name:'Content-Type',value:'text/javascript'}],body:body.toString('base64')});};
   await call('Fetch.enable',{patterns:[{urlPattern:'*/history-filters.js',requestStage:'Request'}]});await demo(initial.origin);
   await openFilter();await click('[data-af-category="result"]');await click('[data-af-option="win"]');await click('[data-af-page="saved"]');
   await evaluate("document.querySelector('[data-af-name]').value='R265 79 格式兼容夹具'");await click('[data-af-save]');await until("JSON.parse(localStorage.getItem('deep-legends-history-presets-v1')||'[]').some(p=>p.name==='R265 79 格式兼容夹具'&&p.conditions.result.values[0]==='win')");
   await call('Fetch.disable');intercept=null;assert.equal(resourceProof.length,1);assert(!resourceProof.some(p=>p.error));const data=await snapshot();assert.deepEqual(data.preferences,settings.preferences);assert.deepEqual(data.controls,settings.controls);
   save('079-format-presets-before.json',{phase,resourceFixtureApplied:true,scope:'preset data format compatibility; published 079 cannot save presets without its missing same-tag script',resourceProof,...data});await shot('079-format-preset.png');
  }else{
   const before=JSON.parse(fs.readFileSync(path.join(out,'079-unmodified-settings-before.json'),'utf8')),presets=JSON.parse(fs.readFileSync(path.join(out,'079-format-presets-before.json'),'utf8'));
   assert.equal(initial.defaultPage,'live');if(phase!=='corrupt')assert(initial.liveVisible,'candidate must start on the UI-written 079 default page');const data=await snapshot();
   if(phase==='read'||phase==='restart'){
    const name=`080-${phase}-before-controls.json`,record={phase,sampledAt:new Date().toISOString(),...data};save(name,record);
    try{record.clientStatus=await evaluate("(async()=>{const r=await fetch('/api/status'),s=await r.json();return {captureStartedMs:window.__r265FollowCaptureStartedMs,httpStatus:r.status,connected:s.connected??null,clientRegion:s.clientRegion??null,platform:s.platform??null,serverId:s.serverId??null,clientViewState:s.clientView?.state??null,clientViewRegion:s.clientView?.region??null,clientViewServerId:s.clientView?.serverId??null,loadedMs:performance.now()};})()");}catch(error){record.clientStatus={diagnosticError:error.message};}save(name,record);
   }
   assert.deepEqual(data.preferences,before.preferences);if(phase==='corrupt')assertCorruptControls(data.controls,before.controls,data.searchFollow);else if(phase==='restart')assertRestartControls(data.controls,before.controls,data.searchFollow);else assertReadControls(data.controls,before.controls,data.searchFollow);
   if(phase==='read'){
    assert.equal(data.presets,presets.presets);assert.deepEqual(JSON.parse(data.presets),JSON.parse(presets.presets));save('079-to-080-preferences-comparison.json',{equal:true,resourceFixtureApplied:false,settingsBefore:before.preferences,settingsAfter:data.preferences,presetContentOrderNamesEqual:true,presetScope:presets.scope});await shot('080-actual-default-page.png');
    await demo(initial.origin);await openSaved();await until("document.querySelector('.af-saved-row,.af-preset')?.textContent.includes('R265 79 格式兼容夹具')");await click('[data-af-apply="0"]');await until("document.querySelector('[data-af-conditions]')?.textContent.includes('胜利')");
    await openSaved();await click('[data-af-rename="0"]');await evaluate("document.querySelector('[data-af-rename-input]').value='R265 80 重命名后保存'");await click('[data-af-rename-save="0"]');
    const renamed=await snapshot(),expected=JSON.parse(presets.presets);expected[0].name='R265 80 重命名后保存';assert.deepEqual(JSON.parse(renamed.presets),expected);assert.deepEqual(renamed.preferences,before.preferences);save('080-preset-renamed-before-restart.json',{resourceFixtureApplied:false,applied:true,renamedAndSaved:true,...renamed});await shot('080-applied-renamed-preset.png');
   }else if(phase==='restart'){
    const renamed=JSON.parse(fs.readFileSync(path.join(out,'080-preset-renamed-before-restart.json'),'utf8'));assert.equal(data.presets,renamed.presets);await demo(initial.origin);await openSaved();await until("document.querySelector('.af-saved-row,.af-preset')?.textContent.includes('R265 80 重命名后保存')");save('080-restart-preset-proof.json',{resourceFixtureApplied:false,restartRetained:true,...await snapshot()});await shot('080-restart-preset.png');
   }else{
    await evaluate("localStorage.setItem('deep-legends-history-presets-v1','{corrupt-r265')");await demo(initial.origin);await openSaved();await until("localStorage.getItem('deep-legends-history-presets-v1')===null");const after=await snapshot();assert.deepEqual(after.preferences,before.preferences);assertCorruptControls(after.controls,before.controls,after.searchFollow);assert(await evaluate("document.querySelector('dialog[data-af-menu]:modal .af-saved .af-empty')?.textContent.includes('暂无常用筛选')"));save('080-corrupt-preset-proof.json',{resourceFixtureApplied:false,corruptFixtureDropped:true,otherSettingsUnchanged:true,...after});await shot('080-corrupt-preset.png');
   }
  }
  console.log(JSON.stringify({phase,passed:true,resourceFixtureApplied:phase==='write-presets'}));
 }finally{ws.close()}
}
module.exports={probe,scriptSHA,tagSHA,waitForInstalledTarget,assertCorruptControls,assertRestartControls,assertReadControls,assertFollowControls,followCapturePrelude,waitForFollowedClient,prepareFollowedStartup};
if(require.main===module){const [port,phase,out]=process.argv.slice(2);probe({port:Number(port),phase,out}).catch(e=>{console.error(e);process.exitCode=1});}
