'use strict';
// CDP controls actual installed 080 then 081. No resource substitution is used.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict'),{execFileSync}=require('node:child_process');
const presetKey='deep-legends-history-presets-v1';
const keys=['lol-loot-default-page','lol-loot-default-match-filter','lol-loot-match-count','lol-loot-mask-names','lol-loot-ui-scale','lol-loot-search-region','lol-loot-search-server-id','lol-loot-search-region-manual'];
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
  fs.writeFileSync(path.join(out,`r266-cdp-${phase}.json`),JSON.stringify({phase,port,limitMs:30000,elapsedMs:now()-started,polls:polls+Number(!!target),ready:!!target,observations},null,2));
 }
}
async function probe({port,phase,out}){
 assert(Number.isInteger(port)&&port>0&&port<65536);assert(['write-settings','write-presets','read','restart','corrupt'].includes(phase));fs.mkdirSync(out,{recursive:true});
 const target=await waitForInstalledTarget({port,phase,out});
 if(process.platform==='win32'){
  const expectedPID=Number(process.env.R265_EXPECTED_ELECTRON_PID);assert(Number.isInteger(expectedPID)&&expectedPID>0,'installed Electron PID proof missing');
  const owners=JSON.parse(execFileSync('powershell.exe',['-NoProfile','-Command',`@(Get-NetTCPConnection -State Listen -LocalPort ${port} -ErrorAction Stop | Select-Object -ExpandProperty OwningProcess -Unique) | ConvertTo-Json -Compress`],{encoding:'utf8',timeout:5000}));
  const processIDs=Array.isArray(owners)?owners:[owners];fs.writeFileSync(path.join(out,`r266-cdp-owner-${phase}.json`),JSON.stringify({port,expectedPID,owners:processIDs},null,2));
  assert(processIDs.length>0&&processIDs.every(pid=>pid===expectedPID),'CDP port belongs to another process');
 }
 const ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{ws.addEventListener('open',resolve,{once:true});ws.addEventListener('error',reject,{once:true})});
 let sequence=0,intercept=null;const pending=new Map(),resourceProof=[];
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [ok,no]=pending.get(m.id);pending.delete(m.id);m.error?no(Error(JSON.stringify(m.error))):ok(m.result);}else if(m.method==='Fetch.requestPaused')void intercept?.(m.params).catch(e=>resourceProof.push({error:e.message}));});
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++sequence,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method))},15000);pending.set(id,[r=>{clearTimeout(timer);resolve(r)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params}));});
 const evaluate=async expression=>{const v=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(v.exceptionDetails)throw Error(JSON.stringify(v.exceptionDetails));return v.result.value;};
 const until=async expression=>{for(let n=0;n<200;n++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100));}throw Error('installed UI condition timeout: '+expression+' '+JSON.stringify(await evaluate("({page:document.querySelector('#overview-content')?.textContent.slice(0,800),live:!document.querySelector('#live-panel')?.hidden,section:document.querySelector('.sidebar-nav-button.is-active')?.id})")))};
 const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
 const choose=async(id,value)=>{await click(`#${id} + .app-select [data-app-select-trigger]`);await click(`#${id} + .app-select [data-native-select-value="${value}"]`);assert.equal(await evaluate(`document.getElementById(${JSON.stringify(id)}).value`),value);};
 const snapshot=()=>evaluate(`(()=>{const storage=Object.fromEntries(Object.keys(localStorage).sort().map(k=>[k,localStorage.getItem(k)]));return {preferences:Object.fromEntries(${JSON.stringify(keys)}.map(k=>[k,localStorage.getItem(k)])),presets:localStorage.getItem(${JSON.stringify(presetKey)}),storage,controls:{defaultPage:document.querySelector('#setting-default-page').value,defaultFilter:document.querySelector('#setting-default-match-filter').value,matchCount:document.querySelector('#setting-match-count').value,maskNames:document.querySelector('#setting-mask-names').checked,scale:document.querySelector('#setting-ui-scale').value,searchRegion:document.querySelector('#player-search-region').dataset.region,searchServer:document.querySelector('#player-search-region').dataset.serverId}}})()`);
 const save=(name,data)=>fs.writeFileSync(path.join(out,name),JSON.stringify(data,null,2));
 const shot=async name=>{const png=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(out,name),Buffer.from(png.data,'base64'));};
 const demo=async origin=>{const marker=Date.now();await call('Page.addScriptToEvaluateOnNewDocument',{source:`window.__r265ProbeLoad=${marker};window.EventSource=class{addEventListener(){}close(){}};`});await call('Page.navigate',{url:origin+'/?demo&section=overview'});await until(`window.__r265ProbeLoad===${marker} && !!document.querySelector('#setting-default-page + .app-select')`);await click('#section-overview');await until("document.querySelector('.match-list .match-entry')");};
 const openFilter=async()=>{
  await click('[data-history-filter-load],[data-af-open]');await until("!!document.querySelector('[data-af-open]')");
  if(!await evaluate("document.querySelector('[data-af-menu]')?.hidden===false"))await click('[data-af-open]');
  await until(phase.startsWith('write-') ? "document.querySelector('[data-af-menu]')?.hidden===false" : "document.querySelector('dialog[data-af-menu]:modal')?.hidden===false");
 };
 const openSaved=async()=>{await openFilter();await click('[data-af-page="saved"]');};
 try{
  await call('Page.enable');await call('Runtime.enable');await until("!!document.querySelector('#setting-default-page + .app-select')");
  const initial=await evaluate("({origin:location.origin,defaultPage:document.querySelector('#setting-default-page').value,liveVisible:!document.querySelector('#live-panel').hidden})");
  if(phase==='write-settings'){
   // No Fetch interception, no missing-script repair in this phase.
   await demo(initial.origin);await click('#section-settings');await until("!document.querySelector('#settings-panel').hidden");
   await choose('setting-default-page','live');await choose('setting-default-match-filter','solo');await choose('setting-match-count','20');await choose('setting-ui-scale','1.25');
   if(!await evaluate("document.querySelector('#setting-mask-names').checked"))await click('#setting-mask-names');
   await click('#player-search-region');await click('#player-search-cn-toggle');await click('[data-region-option="cn"][data-server-id="HN1"]');
   await until("localStorage.getItem('lol-loot-default-page')==='live'&&localStorage.getItem('lol-loot-default-match-filter')==='solo'&&localStorage.getItem('lol-loot-match-count')==='20'&&localStorage.getItem('lol-loot-mask-names')==='true'&&localStorage.getItem('lol-loot-ui-scale')==='1.25'&&localStorage.getItem('lol-loot-search-server-id')==='HN1'");
   const data=await snapshot();save('080-unmodified-settings-before.json',{phase,resourceFixtureApplied:false,scope:'actual published 080 UI-written settings; demo player data',initial,...data});await shot('080-unmodified-settings.png');
  }else if(phase==='write-presets'){
   const settings=JSON.parse(fs.readFileSync(path.join(out,'080-unmodified-settings-before.json'),'utf8'));
   assert.equal(await evaluate("fetch('/history-filters.js').then(r=>r.status)"),200,'published 080 filter resource must be available');await demo(initial.origin);
   assert.equal(await evaluate("localStorage.getItem('deep-legends-history-presets-v1')"),null,'prior independent corrupt-preset phase must leave no presets');
   await openFilter();await click('[data-af-category="result"]');await click('[data-af-option="win"]');await click('[data-af-page="saved"]');
   await evaluate("document.querySelector('[data-af-name]').value='R266 80 实际保存'");await click('[data-af-save]');await until("JSON.parse(localStorage.getItem('deep-legends-history-presets-v1')||'[]').some(p=>p.name==='R266 80 实际保存'&&p.conditions.result.values[0]==='win')");
   await click('[data-af-page="conditions"]');await click('[data-af-category="result"]');await click('[data-af-option="win"]');await click('[data-af-option="loss"]');await click('[data-af-page="saved"]');
   await evaluate("document.querySelector('[data-af-name]').value='R266 80 第二个条件'");await click('[data-af-save]');await until("JSON.parse(localStorage.getItem('deep-legends-history-presets-v1')||'[]').length===2");
   assert.deepEqual(await evaluate("JSON.parse(localStorage.getItem('deep-legends-history-presets-v1')).map(p=>({name:p.name,result:p.conditions.result.values}))"),[{name:'R266 80 实际保存',result:['win']},{name:'R266 80 第二个条件',result:['loss']}]);
   assert.equal(resourceProof.length,0);const data=await snapshot();assert.deepEqual(data.preferences,settings.preferences);assert.deepEqual(data.controls,settings.controls);
   save('080-format-presets-before.json',{phase,resourceFixtureApplied:false,scope:'actual published 080 UI-written preset names/content/order; no source interception',resourceProof,...data});await shot('080-format-preset.png');
  }else{
   const before=JSON.parse(fs.readFileSync(path.join(out,'080-unmodified-settings-before.json'),'utf8')),presets=JSON.parse(fs.readFileSync(path.join(out,'080-format-presets-before.json'),'utf8'));
   assert.equal(initial.defaultPage,'live');if(phase!=='corrupt')assert(initial.liveVisible,'candidate must start on the UI-written 080 default page');const data=await snapshot();assert.deepEqual(data.preferences,before.preferences);assert.deepEqual(data.controls,before.controls);
   if(phase==='read'){
    assert.equal(data.presets,presets.presets);assert.deepEqual(JSON.parse(data.presets),JSON.parse(presets.presets));save('080-to-081-preferences-comparison.json',{equal:true,resourceFixtureApplied:false,settingsBefore:before.preferences,settingsAfter:data.preferences,presetContentOrderNamesEqual:true,presetScope:presets.scope});await shot('081-actual-default-page.png');
    await demo(initial.origin);await openSaved();await until("document.querySelector('.af-saved-row,.af-preset')?.textContent.includes('R266 80 实际保存')");await click('[data-af-apply="0"]');await until("document.querySelector('[data-af-conditions]')?.textContent.includes('胜利')");
    await openSaved();await click('[data-af-rename="0"]');await evaluate("document.querySelector('[data-af-rename-input]').value='R266 81 重命名后保存'");await click('[data-af-rename-save="0"]');
    const renamed=await snapshot(),expected=JSON.parse(presets.presets);expected[0].name='R266 81 重命名后保存';assert.deepEqual(JSON.parse(renamed.presets),expected);assert.deepEqual(renamed.preferences,before.preferences);save('081-preset-renamed-before-restart.json',{resourceFixtureApplied:false,applied:true,renamedAndSaved:true,...renamed});await shot('081-applied-renamed-preset.png');
   }else if(phase==='restart'){
    const renamed=JSON.parse(fs.readFileSync(path.join(out,'081-preset-renamed-before-restart.json'),'utf8'));assert.equal(data.presets,renamed.presets);await demo(initial.origin);await openSaved();await until("document.querySelector('.af-saved-row,.af-preset')?.textContent.includes('R266 81 重命名后保存')");save('081-restart-preset-proof.json',{resourceFixtureApplied:false,restartRetained:true,...await snapshot()});await shot('081-restart-preset.png');
   }else{
    await evaluate("localStorage.setItem('deep-legends-history-presets-v1','{corrupt-r265')");await demo(initial.origin);await openSaved();await until("localStorage.getItem('deep-legends-history-presets-v1')===null");const after=await snapshot();assert.deepEqual(after.preferences,before.preferences);assert.deepEqual(after.controls,before.controls);assert(await evaluate("document.querySelector('dialog[data-af-menu]:modal .af-saved .af-empty')?.textContent.includes('暂无常用筛选')"));save('081-corrupt-preset-proof.json',{resourceFixtureApplied:false,corruptFixtureDropped:true,otherSettingsUnchanged:true,...after});await shot('081-corrupt-preset.png');
   }
  }
  console.log(JSON.stringify({phase,passed:true,resourceFixtureApplied:false}));
 }finally{ws.close()}
}
module.exports={probe,waitForInstalledTarget};
if(require.main===module){const [port,phase,out]=process.argv.slice(2);probe({port:Number(port),phase,out}).catch(e=>{console.error(e);process.exitCode=1});}
