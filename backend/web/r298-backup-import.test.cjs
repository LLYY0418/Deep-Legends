'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const clone=value=>JSON.parse(JSON.stringify(value));
const tick=()=>new Promise(setImmediate);
function largeBackup() {
 const champions={};
 for(let id=1;id<=180;id++)champions[id]=Object.fromEntries(['ranked','normal','aram','urf','event','ranked-top'].map(mode=>[mode,{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]},spells:{spell1Id:4,spell2Id:14},items:{core:[3000],situational:[]},updatedAt:'2026-10-20'}]));
 return {autoConfig:{version:1,enabled:true,runes:true,spells:true,items:true,champions},summonerKeys:{flashSlot:1},watch:{schemaVersion:1,masterEnabled:true,rules:{autoAccept:{enabled:true,delayMs:250}}},network:{mode:'manual',url:'http://127.0.0.1:7890'},native:{contentProtection:true,closeBehavior:'exit',disableHardwareAcceleration:true},automatic:false,autoDownload:false,blockAutostart:false,exportDirectory:'/tmp/r298-imported',renderer:{theme:'dark','mask-names':'true'}};
}
async function backupHarness({failOnce=false}={}) {
 const values={
  '/api/auto-config/settings':{version:7,enabled:false,runes:true,spells:true,items:true,champions:{'1':{normal:{spells:{spell1Id:14,spell2Id:4}}}}},
  '/api/settings/summoner-keys':{keys:['N','M'],flashSlot:null},
  '/api/update/settings':{automatic:true,autoDownload:true,mirrors:['existing-mirror']},
  '/api/client-autostart':{block:true},'/api/champions/network':{mode:'direct',url:''},
  '/api/watch/rules':{schemaVersion:1,masterEnabled:false,rules:{autoAccept:{enabled:false,delayMs:100}}},
 };
 let native={contentProtection:false,closeBehavior:'tray',disableHardwareAcceleration:false,hardwareAccelerationDisabledAtStartup:false},directory='/tmp/r298-original',shouldFail=failOnce;
 const writes=[];
 const h=require('./update-harness.cjs').harness(undefined,(url,options={})=>{
  if(!Object.hasOwn(values,url))return;
  if(['POST','PUT'].includes(options.method)){
   const body=JSON.parse(options.body);writes.push({url,body});
   if(url==='/api/client-autostart'&&shouldFail){shouldFail=false;return Promise.resolve({ok:false,text:async()=>'injected import failure'});}
   if(url==='/api/auto-config/settings'){
    assert.equal(body.version,values[url].version,'every import/rollback uses the current CAS version');
    values[url]={...body,version:body.version+1};
   } else values[url]={...values[url],...body};
  }
  return Promise.resolve({ok:true,json:async()=>clone(values[url])});
 });
 h.w.desktopPreferences={get:async()=>clone(native),update:async patch=>{native={...native,...patch};return clone(native)},openGithub(){},openDirectory(){}};
 h.w.desktopShare={getSaveDirectory:async()=>({directory}),setSaveDirectory:async next=>{directory=next;}};
 let initialize;h.w.deepLegendsSections={register:(_name,fn)=>{initialize=fn}};
 h.w.eval(fs.readFileSync(__dirname+'/privacy.js','utf8'));h.w.eval(fs.readFileSync(__dirname+'/settings-preferences.js','utf8'));initialize();await tick();
 h.w.deepLegendsPreferenceStorage.setItem('lol-loot-theme','light');h.get('setting-theme').value='light';
 h.w.deepLegendsPreferenceStorage.setItem('lol-loot-mask-names','false');h.get('setting-mask-names').checked=false;
 const snapshot=()=>({values:clone(values),native:clone(native),directory,renderer:{theme:h.w.deepLegendsPreferenceStorage.getItem('lol-loot-theme'),mask:h.w.deepLegendsPreferenceStorage.getItem('lol-loot-mask-names')}});
 return {...h,values,writes,snapshot};
}
test('R298 actual file import accepts a complete backup larger than old 128KiB',async()=>{
 const h=await backupHarness();try{
  const backup=largeBackup(),text=JSON.stringify(backup),file=new h.w.File([text],'large-settings.json',{type:'application/json'});
  assert(file.size>128*1024&&file.size<2*1024*1024);
  // jsdom File lacks Blob.text; preserve its real byte size and file-input event path.
  Object.defineProperty(file,'text',{value:async()=>text});
  Object.defineProperty(h.get('setting-backup-file'),'files',{value:[file],configurable:true});
  h.get('setting-backup-file').dispatchEvent(new h.w.Event('change',{bubbles:true}));
  await tick();await tick();
  assert.match(h.get('toast').textContent,/已导入/);
  assert.equal(Object.keys(h.values['/api/auto-config/settings'].champions).length,180);
  assert.deepEqual(h.values['/api/auto-config/settings'].champions,backup.autoConfig.champions);
  assert.equal(h.values['/api/settings/summoner-keys'].flashSlot,1);
  assert.equal(h.values['/api/watch/rules'].masterEnabled,true);
  assert.equal(h.values['/api/client-autostart'].block,false);
  assert.equal(h.w.deepLegendsSettingsBackup.sanitize(await h.w.deepLegendsSettingsBackup.createBackup()).renderer.theme,'dark');
  assert.equal(h.get('setting-backup-file').value,'');
 }finally{h.close()}
});
test('R298 actual multi-endpoint import fully rolls back after a late endpoint failure',async()=>{
 const h=await backupHarness({failOnce:true});try{
  const before=h.snapshot();
  await assert.rejects(h.w.deepLegendsSettingsBackup.applyBackup(largeBackup()),/设置保存失败/);
  const after=h.snapshot();
  assert(after.values['/api/auto-config/settings'].version>before.values['/api/auto-config/settings'].version,'CAS revision advances during compensating rollback');
  after.values['/api/auto-config/settings'].version=before.values['/api/auto-config/settings'].version;
  assert.deepEqual(after,before,'all configuration, keys, watch, proxy, native, update, directory and renderer values must be restored');
  for(const url of ['/api/auto-config/settings','/api/settings/summoner-keys','/api/watch/rules','/api/champions/network','/api/update/settings'])assert.equal(h.writes.filter(w=>w.url===url).length,2,`${url} must be written then rolled back`);
  assert.equal(h.writes.filter(w=>w.url==='/api/client-autostart').length,2);
 }finally{h.close()}
});
test('R298 import restores all state after renderer preferences have already changed',async()=>{
 const h=await backupHarness();try{
  const before=h.snapshot(),storage=h.w.deepLegendsPreferenceStorage,setItem=storage.setItem.bind(storage),themes=[];
  let failOnce=true;
  storage.setItem=(key,value)=>{
   if(key==='lol-loot-mask-names'&&value==='true'&&failOnce){
    failOnce=false;
    assert.equal(storage.getItem('lol-loot-theme'),'dark','failure must occur after the imported theme was persisted');
    assert.equal(h.get('setting-theme').value,'dark','the renderer control must also have changed');
    throw Error('injected renderer failure');
   }
   setItem(key,value);
   if(key==='lol-loot-theme')themes.push(value);
  };
  await assert.rejects(h.w.deepLegendsSettingsBackup.applyBackup(largeBackup()),/injected renderer failure/);
  const after=h.snapshot();
  assert(after.values['/api/auto-config/settings'].version>before.values['/api/auto-config/settings'].version);
  after.values['/api/auto-config/settings'].version=before.values['/api/auto-config/settings'].version;
  assert.deepEqual(after,before,'backend, native, directory and persisted renderer preferences must all roll back');
  assert.deepEqual(themes,['dark','dark','light','light'],'import and the real change handler both persist each theme');
  assert.equal(h.get('setting-theme').value,'light');
  assert.equal(h.get('setting-mask-names').checked,false);
 }finally{h.close()}
});
