'use strict';
// CDP controls the actual installed program. Only the separately recorded 079
// preset-format fixture may supply the exact missing resource from its tag.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict'),{execFileSync}=require('node:child_process');
const presetKey='deep-legends-history-presets-v1';
const keys=['lol-loot-default-page','lol-loot-default-match-filter','lol-loot-match-count','lol-loot-mask-names','lol-loot-ui-scale','lol-loot-search-region','lol-loot-search-server-id','lol-loot-search-region-manual'];
const scriptSHA='74699f9576ccd8686fce0e95c38b718b96718baa46bd68e9068cf947f7ea38f3';
const tagSHA='049b30cad1ccd13fd8cbe785b25c5a56112f46a5';
async function probe({port,phase,out}){
 assert(Number.isInteger(port)&&port>0&&port<65536);assert(['write-settings','write-presets','read','restart','corrupt'].includes(phase));fs.mkdirSync(out,{recursive:true});
 let target;for(let n=0;n<150;n++){try{target=(await(await fetch(`http://127.0.0.1:${port}/json/list`)).json()).find(t=>t.type==='page'&&/^http:\/\/(127\.0\.0\.1|localhost):/.test(t.url));if(target)break;}catch{}await new Promise(r=>setTimeout(r,200));}assert(target,'installed Electron page did not become ready');
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
  await until("document.querySelector('[data-af-menu]')?.hidden===false");
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
   assert.equal(initial.defaultPage,'live');if(phase!=='corrupt')assert(initial.liveVisible,'candidate must start on the UI-written 079 default page');const data=await snapshot();assert.deepEqual(data.preferences,before.preferences);assert.deepEqual(data.controls,before.controls);
   if(phase==='read'){
    assert.equal(data.presets,presets.presets);assert.deepEqual(JSON.parse(data.presets),JSON.parse(presets.presets));save('079-to-080-preferences-comparison.json',{equal:true,resourceFixtureApplied:false,settingsBefore:before.preferences,settingsAfter:data.preferences,presetContentOrderNamesEqual:true,presetScope:presets.scope});await shot('080-actual-default-page.png');
    await demo(initial.origin);await openSaved();await until("document.querySelector('[data-af-apply]')?.textContent.includes('R265 79 格式兼容夹具')");await click('[data-af-apply="0"]');await until("document.querySelector('[data-af-conditions]')?.textContent.includes('胜利')");
    await openSaved();await click('[data-af-rename="0"]');await evaluate("document.querySelector('[data-af-rename-input]').value='R265 80 重命名后保存'");await click('[data-af-rename-save="0"]');
    const renamed=await snapshot(),expected=JSON.parse(presets.presets);expected[0].name='R265 80 重命名后保存';assert.deepEqual(JSON.parse(renamed.presets),expected);assert.deepEqual(renamed.preferences,before.preferences);save('080-preset-renamed-before-restart.json',{resourceFixtureApplied:false,applied:true,renamedAndSaved:true,...renamed});await shot('080-applied-renamed-preset.png');
   }else if(phase==='restart'){
    const renamed=JSON.parse(fs.readFileSync(path.join(out,'080-preset-renamed-before-restart.json'),'utf8'));assert.equal(data.presets,renamed.presets);await demo(initial.origin);await openSaved();await until("document.querySelector('[data-af-apply]')?.textContent.includes('R265 80 重命名后保存')");save('080-restart-preset-proof.json',{resourceFixtureApplied:false,restartRetained:true,...await snapshot()});await shot('080-restart-preset.png');
   }else{
    await evaluate("localStorage.setItem('deep-legends-history-presets-v1','{corrupt-r265')");await demo(initial.origin);await openSaved();await until("localStorage.getItem('deep-legends-history-presets-v1')===null");const after=await snapshot();assert.deepEqual(after.preferences,before.preferences);assert.deepEqual(after.controls,before.controls);assert(await evaluate("document.querySelector('.af-empty')?.textContent.includes('暂无常用筛选')"));save('080-corrupt-preset-proof.json',{resourceFixtureApplied:false,corruptFixtureDropped:true,otherSettingsUnchanged:true,...after});await shot('080-corrupt-preset.png');
   }
  }
  console.log(JSON.stringify({phase,passed:true,resourceFixtureApplied:phase==='write-presets'}));
 }finally{ws.close()}
}
module.exports={probe,scriptSHA,tagSHA};
if(require.main===module){const [port,phase,out]=process.argv.slice(2);probe({port:Number(port),phase,out}).catch(e=>{console.error(e);process.exitCode=1});}
