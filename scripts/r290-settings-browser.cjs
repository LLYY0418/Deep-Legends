'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 for(const theme of ["dark","light"])for(const failure of [false,true]){
  const p=await page(theme);
  await p.call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});
  await p.evaluate(`(()=>{window.__r290StarsCalls=0;const original=window.fetch;const values={'/api/update/settings':{mirrors:[''],automatic:true,autoDownload:true},'/api/client-autostart':{block:true},'/api/watch/rules':{schemaVersion:1,masterEnabled:false},'/api/champions/network':{mode:'direct',url:'',active:'直连'}};window.fetch=(url,options)=>{const name=new URL(url,location.origin).pathname;if(name==='/api/github/stars'){__r290StarsCalls++;return ${failure?"Promise.reject(new Error('offline'))":"Promise.resolve(new Response(JSON.stringify({count:1234})))"};}if(Object.hasOwn(values,name)){if(options?.method==='POST')values[name]=JSON.parse(options.body);return Promise.resolve(new Response(JSON.stringify(values[name])));}return original(url,options)};})()`);
  assert.equal(await p.evaluate('__r290StarsCalls'),0);
  await p.evaluate("__r278App.activateSection('settings')");
  await p.until("window.deepLegendsSettingsBackup?.createBackup && __r290StarsCalls===1");
  await p.until(`document.querySelector('#settings-github-count').${failure?'hidden':"textContent==='1,234'"}`);
  const read=()=>p.evaluate(`(()=>{const b=document.getElementById('settings-github'),r=b.getBoundingClientRect();return {text:b.innerText,count:document.getElementById('settings-github-count').textContent,height:r.height,right:r.right,calls:__r290StarsCalls,label:getComputedStyle(b.querySelector('.settings-github-label')).display,tooltip:b.dataset.tooltip}})()`);
  const normal=await read();assert.equal(normal.height,40);assert(normal.text.includes('GitHub'));assert.equal(normal.count,failure?'':'1,234');assert.equal(normal.calls,1);
  await p.call('DOM.enable');await p.call('CSS.enable');const {root}=await p.call('DOM.getDocument');const {nodeId}=await p.call('DOM.querySelector',{nodeId:root.nodeId,selector:'#settings-github'});
  for(const pseudo of ['hover','focus-visible']){
   await p.call('CSS.forcePseudoState',{nodeId,forcedPseudoClasses:[pseudo]});const hovered=await read();assert(hovered.text.includes('觉得顺手？赏颗星'));assert.equal(hovered.right,normal.right);assert.equal(hovered.height,40);results.push({kind:'github',failure,state:pseudo,...hovered});
  }
  await p.shot(failure?'github-offline':'github-hover');
  await p.call('CSS.forcePseudoState',{nodeId,forcedPseudoClasses:[]});assert((await read()).text.includes('GitHub'));
  await p.call('Emulation.setDeviceMetricsOverride',{width:900,height:900,deviceScaleFactor:1,mobile:false});
  const narrow=await read();assert.equal(narrow.label,'none');assert.equal(narrow.tooltip,'觉得顺手？赏颗星');assert.equal(narrow.height,40);
  results.push({kind:'github',theme,failure,state:'normal',...normal},{kind:'github',theme,failure,state:'narrow',...narrow});
  if(!failure){
   await p.call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});
   await p.evaluate("document.getElementById('settings-tab-privacy').click()");
   const measure=async(id,tone)=>{
    const value=await p.evaluate(`(()=>{const node=document.getElementById('${id}'),s=getComputedStyle(node),canvas=document.createElement('canvas'),c=canvas.getContext('2d');const rgba=color=>{c.clearRect(0,0,1,1);c.fillStyle=color;c.fillRect(0,0,1,1);return [...c.getImageData(0,0,1,1).data]},composite=(a,b)=>a.slice(0,3).map((v,i)=>(v*a[3]/255+b[i]*(1-a[3]/255)));let bg=[255,255,255];const chain=[];for(let n=node;n;n=n.parentElement)chain.unshift(n);for(const n of chain)bg=composite(rgba(getComputedStyle(n).backgroundColor),bg);const color=rgba(s.color),expected=rgba(s.getPropertyValue('--${tone}')),fg=composite(color,bg),lum=v=>v.map(x=>x/255).map(x=>x<=.04045?x/12.92:((x+.055)/1.055)**2.4).reduce((a,v,i)=>a+v*[.2126,.7152,.0722][i],0),a=lum(fg),b=lum(bg);return {id:'${id}',tone:'${tone}',text:node.textContent,color:color.slice(0,3),expected:expected.slice(0,3),weight:s.fontWeight,contrast:(Math.max(a,b)+.05)/(Math.min(a,b)+.05)}})()`);
    assert.deepEqual(value.color,value.expected,'computed status theme color');assert.equal(value.weight,'600');assert(value.contrast>=4.5,JSON.stringify(value));results.push({kind:'status',theme,...value});
   };
   await p.evaluate("__r278App.renderChampionNetwork({mode:'direct',active:'直连'});__r278App.renderRiotKeySettings({source:'embedded',status:'configured'})");
   await measure('setting-proxy-state','primary-strong');await measure('setting-riot-key-state','primary-strong');
   await p.evaluate("__r278App.renderRiotKeySettings({source:'embedded',status:'cooldown'})");await measure('setting-riot-key-state','warning');
   await p.evaluate("__r278App.renderRiotKeySettings({source:'user',status:'invalid'});document.getElementById('setting-proxy-mode').value='manual';document.getElementById('setting-proxy-url').value='';__r278App.saveChampionNetwork()");await measure('setting-riot-key-state','danger');await measure('setting-proxy-state','danger');
   await p.evaluate("__r278App.renderRiotKeySettings({source:'embedded',status:'configured'});__r278App.renderChampionNetwork({mode:'direct',active:'直连'})");
   await p.shot('status-privacy');
   await p.evaluate("document.getElementById('settings-tab-app').click();document.getElementById('setting-hardware-restart').hidden=false;__r278App.renderShareDirectorySetting('/fixture/exports');__r278App.showUpdateCheckFeedback('已是最新版本')");
   await measure('setting-hardware-restart','primary-strong');await measure('setting-share-directory','primary-strong');await measure('settings-update-feedback','primary-strong');
   await p.evaluate("__r278App.showUpdateCheckFeedback('检查失败',true)");await measure('settings-update-feedback','danger');
   await p.shot('status-app');
   await p.evaluate("__r278Game.state.lastCapabilities=[{name:'summoner',state:'available'},{name:'ranked-stats',state:'unsupported'}];__r278Game.renderCapabilitySettings();document.getElementById('settings-tab-privacy').click();document.querySelector('#gameplay-settings-status>span').id='r290-capability-current';document.querySelector('.settings-capability-degraded').id='r290-capability-warning';document.querySelector('.settings-fact').id='r290-privacy-fact'");
   await measure('r290-capability-current','primary-strong');await measure('r290-capability-warning','warning');await measure('r290-privacy-fact','primary-strong');
   await p.evaluate("__r278App.activateSection('suite')");await p.until('window.__r282Suite');
   await p.evaluate("__r282Suite.state.watch={masterEnabled:true,rules:{}};__r282Suite.renderWatch();document.querySelector('.watch-rules-head .suite-chip').id='r290-watch-current'");await measure('r290-watch-current','primary-strong');
   await p.evaluate("__r282Suite.state.watch.masterEnabled=false;__r282Suite.renderWatch();document.querySelector('.watch-rules-head .suite-chip').id='r290-watch-warning'");await measure('r290-watch-warning','warning');
   const description=await p.evaluate("(()=>{__r278Game.state.settings.liveRefresh=true;__r278Game.renderSessionSummary({available:true,phase:'Lobby',queueLabel:'测试队列',mapId:11});const copy=document.querySelector('#live-session-summary .live-session-copy'),line=copy.querySelector('span'),c=document.createElement('canvas').getContext('2d'),rgb=x=>{c.clearRect(0,0,1,1);c.fillStyle=x;c.fillRect(0,0,1,1);return [...c.getImageData(0,0,1,1).data]};return {text:line.textContent,hasStatus:!!copy.querySelector('.status-text'),color:rgb(getComputedStyle(line).color),muted:rgb(getComputedStyle(line).getPropertyValue('--muted'))}})()");
   assert(description.text.includes('自动刷新已开启'));assert.equal(description.hasStatus,false);assert.deepEqual(description.color,description.muted,'live refresh explanation stays muted');results.push({kind:'description-exemption',theme,...description});


  }
 }
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
