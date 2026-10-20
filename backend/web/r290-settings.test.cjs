'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
test('R290 star count format includes zero and comma grouped values',()=>{
 const {formatStars}=require('./settings-preferences.js');
 for(const [count,text] of [[0,'0'],[128,'128'],[1234,'1,234'],[null,''],[-1,''],['4',''],[1.1,'']])assert.equal(formatStars(count),text);
});
test('R290 actual status renderers clear failure tone and distinguish warning from failure',async()=>{
 const source=fs.readFileSync(__dirname+'/app.js','utf8').replace(/\}\)\(\);\s*$/,'window.r290={renderRiotKeySettings,renderChampionNetwork,saveChampionNetwork};})();');
 const h=require('./update-harness.cjs').harness(source);
 try{
  const riot=h.get('setting-riot-key-state'),proxy=h.get('setting-proxy-state');
  h.w.r290.renderRiotKeySettings({source:'embedded',status:'cooldown'});assert(riot.classList.contains('is-warning'));assert(!riot.classList.contains('is-danger'));
  h.w.r290.renderRiotKeySettings({source:'user',status:'invalid'});assert(riot.classList.contains('is-danger'));assert(!riot.classList.contains('is-warning'));
  h.w.r290.renderRiotKeySettings({source:'embedded',status:'configured'});assert(!riot.classList.contains('is-danger'));assert.equal(riot.textContent,'使用内置服务');
  h.get('setting-proxy-mode').value='manual';h.get('setting-proxy-url').value='';await h.w.r290.saveChampionNetwork();assert(proxy.classList.contains('is-danger'));assert.equal(proxy.hidden,false);
  h.w.r290.renderChampionNetwork({mode:'direct',active:'直连'});assert(!proxy.classList.contains('is-danger'));assert.equal(proxy.textContent,'直连');
  h.probe.showUpdateCheckFeedback('检查失败',true);assert(h.get('settings-update-feedback').classList.contains('is-danger'));
  h.probe.showUpdateCheckFeedback('已是最新版本');assert(!h.get('settings-update-feedback').classList.contains('is-danger'));
 }finally{h.close()}
});
test('R290 settings stars load once on opening, failures fall back quietly to just the star',async()=>{
 for(const mode of ['success','http-failure','offline']){
  let calls=0;
  const h=require('./update-harness.cjs').harness(undefined,(url)=>{
   if(url==='/api/github/stars'){
    calls++;
    if(mode==='offline')return Promise.reject(new Error('offline'));
    return Promise.resolve({ok:mode==='success',json:async()=>({count:1234})});
   }
   return Promise.resolve({ok:true,json:async()=>({})});
  });
  try{
   let initialize;h.w.deepLegendsSections={register:(_name,fn)=>{initialize=fn}};
   h.w.eval(fs.readFileSync(__dirname+'/settings-preferences.js','utf8'));
   assert.equal(calls,0,'script/startup must not request stars');
   initialize();await new Promise(setImmediate);initialize();await new Promise(setImmediate);
   assert.equal(calls,1,'no retry or repeated initialization');
   assert.equal(h.get('settings-github-count').textContent,mode==='success'?'1,234':'');
   assert.equal(h.get('settings-github-count').hidden,mode!=='success');
   assert(h.get('settings-github').querySelector('svg.github-star path'),'failure retains the star icon');
   assert.equal(h.get('settings-github').querySelector('svg.github-star').getAttribute('width'),'18');
   assert.equal(h.get('settings-github').hasAttribute('data-tooltip'),false);
   assert.equal(h.get('settings-github').hasAttribute('title'),false);
   assert.equal(h.get('settings-github').getAttribute('href'),'https://github.com/LLYY0418/Deep-Legends');
   assert.equal(h.get('toast').hidden,true,'no failure toast');
  }finally{h.close()}
 }
});
test('R290 unavailable persistent storage keeps settings initialization and backup usable in the current session',async()=>{
 const app=fs.readFileSync(__dirname+'/app.js','utf8');
 const script=`Object.defineProperty(window,'localStorage',{get(){throw new DOMException('blocked','SecurityError')}});\n`+app;
 const values={'/api/auto-config/settings':{version:1,enabled:false,runes:true,spells:true,items:true,champions:{}},'/api/settings/summoner-keys':{keys:['D','F'],flashSlot:null},'/api/update/settings':{automatic:true,mirrors:['']},'/api/client-autostart':{block:true},'/api/champions/network':{mode:'auto',url:''},'/api/watch/rules':{schemaVersion:1,masterEnabled:false}};
 const h=require('./update-harness.cjs').harness(script,(url,options={})=>{
  if(!Object.hasOwn(values,url))return;
  if(options.method==='POST'||options.method==='PUT')values[url]=JSON.parse(options.body);
  return Promise.resolve({ok:true,json:async()=>values[url]});
 });
 try {
  let initialize;h.w.deepLegendsSections={register:(_name,callback)=>{initialize=callback}};
  h.w.eval(fs.readFileSync(__dirname+'/privacy.js','utf8'));
  h.w.eval(fs.readFileSync(__dirname+'/settings-preferences.js','utf8'));
  assert.doesNotThrow(()=>initialize());
  h.get('settings-tab-app').click();assert.equal(h.get('settings-app-panel').hidden,false);
  await h.w.deepLegendsSettingsBackup.applyBackup({renderer:{theme:'dark','mask-names':'true'}});
  const result=await h.w.deepLegendsSettingsBackup.createBackup();
  assert.equal(result.renderer.theme,'dark');assert.equal(result.renderer['mask-names'],'true');assert.equal(h.w.deepLegendsPrivacy.masked(),true);
  h.get('setting-theme').value='light';h.get('setting-theme').dispatchEvent(new h.w.Event('change'));
  assert.equal((await h.w.deepLegendsSettingsBackup.createBackup()).renderer.theme,'light');
  await h.w.deepLegendsSettingsBackup.applyBackup({automatic:false,autoDownload:false});
  assert.equal(h.get('setting-auto-download').disabled,true);assert.equal(h.get('setting-auto-download').checked,false);
  let backed=await h.w.deepLegendsSettingsBackup.createBackup();assert.equal(backed.autoDownload,false);assert.equal(backed.automatic,false);
  h.get('setting-auto-update').click();await new Promise(setImmediate);await new Promise(setImmediate);
  assert.equal(h.get('setting-auto-download').disabled,false);assert.equal(h.get('setting-auto-download').checked,false);
  h.get('setting-auto-download').click();await new Promise(setImmediate);await new Promise(setImmediate);
  backed=await h.w.deepLegendsSettingsBackup.createBackup();assert.equal(backed.autoDownload,true);
  values['/api/champions/network']={mode:'manual',url:'http://name:secret@127.0.0.1:7890'};
  const clean=await h.w.deepLegendsSettingsBackup.createBackup();
  assert.equal(clean.network.url,'http://127.0.0.1:7890/');
  assert.ok(!JSON.stringify(clean).includes('secret'));
  const before=JSON.stringify(values);
  await assert.rejects(h.w.deepLegendsSettingsBackup.applyBackup({network:{mode:'manual',url:'http://name:secret@127.0.0.1:7890'}}),/用户名或密码/);
  assert.equal(JSON.stringify(values),before,'userinfo import must fail before any write');
 } finally {h.close();}
});
