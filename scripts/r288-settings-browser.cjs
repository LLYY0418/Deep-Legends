'use strict';
const fs=require('node:fs'),assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']){
  const p=await page(theme);await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
  await p.call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});
  await p.evaluate(`(()=>{
   localStorage.setItem('lol-loot-ui-scale','1');__r278App.applyUiScale();
   const original=window.fetch;let native={contentProtection:false,closeBehavior:'exit',disableHardwareAcceleration:false,hardwareAccelerationDisabledAtStartup:false};
   window.desktopShare={getSaveDirectory:async()=>({directory:''}),setSaveDirectory:async()=>({directory:''}),chooseSaveDirectory:async()=>({canceled:true})};
   window.desktopPreferences={get:async()=>({...native}),update:async patch=>(Object.assign(native,patch),{...native}),openGithub:async()=>true,openDirectory:async()=>true};
   const fixtures={'/api/update/settings':{mirrors:[],preferredSource:'',automatic:true},'/api/client-autostart':{block:true},'/api/watch/rules':{schemaVersion:3,masterEnabled:false,rules:{autoAccept:{enabled:false,delayMs:0},autoReconnect:{enabled:false,delayMs:15000}},champSelect:{enabled:false,groups:{}}},'/api/champions/network':{mode:'auto',url:'',active:'${theme}'==='dark'?'直连更快，已直连':''},'/api/release-notes':{version:'0.12.91',notes:'## 0.12.91 · fixture'}};
   window.fetch=(url,options)=>{const name=new URL(url,location.origin).pathname;if(Object.hasOwn(fixtures,name)){if(options?.method==='POST')Object.assign(fixtures[name],JSON.parse(options.body||'{}'));return Promise.resolve(new Response(JSON.stringify(fixtures[name]),{headers:{'Content-Type':'application/json'}}));}return original(url,options);};
   __r278App.renderShareDirectorySetting('');
   __r278App.state.status.version='0.12.91';__r278App.renderUpdateStatus({supported:true,state:'idle'});__r278App.renderStatus();__r278App.renderUpdateStatus({supported:true,state:'idle'});
   __r278App.activateSection('settings');
  })()`);
  await p.until('window.deepLegendsSettingsBackup?.createBackup && !!document.querySelector(\'link[href="/settings-preferences.css"]\')');
  await p.evaluate("document.fonts.ready");
  if(!process.env.R288_SETTINGS_DESIGN)throw Error('Set R288_SETTINGS_DESIGN to the approved mock.html');
  const designHTML=fs.readFileSync(process.env.R288_SETTINGS_DESIGN,'utf8');
  results.push(await p.evaluate(`(async()=>{const iframe=document.createElement('iframe');iframe.style.cssText='position:fixed;left:0;top:0;width:1440px;height:900px;visibility:hidden;border:0';const loaded=new Promise(resolve=>iframe.onload=resolve);iframe.srcdoc=${JSON.stringify(designHTML)};document.body.append(iframe);await loaded;const result={design:true,theme:'${theme}',frames:{}};for(const name of ['f1','f2','f3','f4','f5','f6']){const frame=iframe.contentDocument.querySelector('#'+name+' .frame'),base=frame.getBoundingClientRect(),rect=node=>{const r=node.getBoundingClientRect();return {x:r.x-base.x,y:r.y-base.y,width:r.width,height:r.height}};result.frames[name]={head:rect(frame.querySelector('.shead')),tabs:rect(frame.querySelector('.nav')),content:rect(frame.querySelector('.col')),firstRow:rect(frame.querySelector('.row')),rows:[...frame.querySelectorAll('.row')].map(node=>({title:node.querySelector('h3')?.textContent,rect:rect(node),control:node.querySelector('.r')?rect(node.querySelector('.r')):null})),github:rect(frame.querySelector('.gh'))};}iframe.remove();return result})()`));
  for(const section of theme==='dark'?['appearance','gameplay','privacy','app']:['privacy']){
   await p.evaluate(`document.getElementById('settings-tab-${section}').click()`);
   await p.shot('settings-'+section);
   results.push(await p.evaluate(`(()=>{const rect=n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}};return {theme:'${theme}',section:'${section}',head:rect(document.querySelector('.settings-head')),tabs:rect(document.querySelector('.settings-tabs')),content:rect(document.querySelector('#settings-${section}-panel')),firstRow:rect(document.querySelector('#settings-${section}-panel .setting-card')),github:rect(document.getElementById('settings-github')),rows:[...document.querySelectorAll('#settings-${section}-panel .setting-card')].map(node=>({title:node.querySelector('h3')?.textContent,rect:rect(node),control:(()=>{let target=node.querySelector('.settings-facts,.settings-capability-summary,.select-wrap,.toggle-control,.setting-actions,.setting-network-controls,.setting-share-controls,.text-button');if(!target)return null;const toggle=target.matches('.toggle-control')?target:target.querySelector('.toggle-control');if(toggle&&!target.querySelector('select,button')){const input=toggle.querySelector('input').getBoundingClientRect(),label=toggle.querySelector('span').getBoundingClientRect();return {x:input.x,y:Math.min(input.y,label.y),width:label.right-input.x,height:Math.max(input.bottom,label.bottom)-Math.min(input.y,label.y)}}return rect(target)})()})),version:document.getElementById('settings-build-identity').textContent}})()`));
  }
  assert(!results.at(-1).version.includes('构建'));
  const auto=await p.evaluate(`(()=>{const g=__r278Game;localStorage.setItem('lol-loot-auto-live','true');g.handleGameplayPhase('Lobby');g.handleGameplayPhase('ChampSelect');const entered=__r278App.state.section;__r278App.activateSection('settings');g.handleGameplayPhase('GameStart');g.handleGameplayPhase('InProgress');const stayed=__r278App.state.section;g.handleGameplayPhase('EndOfGame');g.handleGameplayPhase('ChampSelect');const next=__r278App.state.section;__r278App.activateSection('settings');localStorage.setItem('lol-loot-auto-live','false');return {entered,stayed,next}})()`);
  assert.deepEqual(auto,{entered:'live',stayed:'settings',next:'live'});results.push({theme,auto});
  const roundtrip=await p.evaluate(`(async()=>{const first=await deepLegendsSettingsBackup.createBackup();const count=await deepLegendsSettingsBackup.applyBackup({...first,riotKey:'private-key',unknown:1});const second=await deepLegendsSettingsBackup.createBackup();return {first,second,count}})()`);
  assert.deepEqual(roundtrip.second,roundtrip.first);assert(!JSON.stringify(roundtrip.first).includes('private-key'));results.push({theme,backupCount:roundtrip.count});
  const rollback=await p.evaluate(`(async()=>{const before=await deepLegendsSettingsBackup.createBackup(),original=window.fetch;let failed=false;window.fetch=(url,options)=>{if(!failed&&new URL(url,location.origin).pathname==='/api/champions/network'&&options?.method==='POST'){failed=true;return Promise.resolve(new Response('{}',{status:503}))}return original(url,options)};let rejected=false;try{await deepLegendsSettingsBackup.applyBackup({...before,watch:{...before.watch,masterEnabled:!before.watch.masterEnabled}})}catch{rejected=true}finally{window.fetch=original}return {before,after:await deepLegendsSettingsBackup.createBackup(),rejected}})()`);
  assert(rollback.rejected);assert.deepEqual(rollback.after,rollback.before);results.push({theme,failedImportRestored:true});

  await p.evaluate(`document.getElementById('setting-content-protection').click();document.getElementById('setting-close-behavior').value='tray';document.getElementById('setting-close-behavior').dispatchEvent(new Event('change'));document.getElementById('setting-disable-hardware').click()`);
  await p.until("!document.getElementById('setting-hardware-restart').hidden");
  if(theme==='dark'){
   await p.call('Emulation.setDeviceMetricsOverride',{width:900,height:760,deviceScaleFactor:1,mobile:false});
   await p.evaluate("document.getElementById('settings-tab-privacy').click();__r278App.applySidebar()");
   await p.shot('settings-narrow');
   const narrow=await p.evaluate("(()=>{const rect=node=>{const r=node.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}};return {narrow:true,head:rect(document.querySelector('.settings-head')),tabs:rect(document.querySelector('.settings-tabs')),firstRow:rect(document.querySelector('#settings-privacy-panel .setting-card')),github:rect(document.getElementById('settings-github')),direction:getComputedStyle(document.querySelector('.settings-tabs')).flexDirection}})()");assert.equal(narrow.direction,'row');results.push(narrow);
  }
 }
 const design=results.find(row=>row.design).frames;
 const mapping={appearance:'f1',gameplay:'f2',privacy:'f3',app:'f4'};
 for(const row of results.filter(row=>row.section||row.narrow)){
  const expected=design[row.narrow?'f6':row.theme==='light'?'f5':mapping[row.section]];
  const differences={};
  for(const key of ['head','tabs','firstRow','github'])for(const dimension of ['x','y','width','height']){
   const delta=Math.abs(row[key][dimension]-expected[key][dimension]);differences[key+'.'+dimension]=delta;
   assert(delta<=2,JSON.stringify({section:row.section,narrow:row.narrow,key,dimension,actual:row[key],expected:expected[key]}));
  }
  if(row.rows){
   const expectedRows=new Map(expected.rows.map(item=>[item.title,item]));
   row.rowDifferences=[];
   for(const actual of row.rows){const match=expectedRows.get(actual.title);if(!match)continue;
    const delta={title:actual.title};
    for(const dimension of ['x','y','width','height']){delta[dimension]=Math.abs(actual.rect[dimension]-match.rect[dimension]);assert(delta[dimension]<=2,JSON.stringify({actual,expected:match}));}
    if(actual.control&&match.control){delta.control={};
     const dynamic=['数据说明','客户端接口'].includes(actual.title);
     const dimensions=dynamic?['y','height']:['x','y','width','height'];
     for(const dimension of dimensions){delta.control[dimension]=Math.abs(actual.control[dimension]-match.control[dimension]);assert(delta.control[dimension]<=2,JSON.stringify({title:actual.title,actual:actual.control,expected:match.control}));}
     if(dynamic){delta.control.right=Math.abs(actual.control.x+actual.control.width-match.control.x-match.control.width);assert(delta.control.right<=2);}
    }
    row.rowDifferences.push(delta);
   }
  }
  row.geometryDifferences=differences;
 }
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
