'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const {sanitize}=require('./settings-preferences.js');
test('R288 settings backup round trip retains preferences and strips unknown fields and keys',()=>{
 const input={renderer:{theme:'light','mask-names':'true','live-tags':['pro'],riotKey:'private'},native:{contentProtection:true,closeBehavior:'tray',disableHardwareAcceleration:true},automatic:false,blockAutostart:true,
 watch:{schemaVersion:3,masterEnabled:true,rules:{autoReconnect:{enabled:true,delayMs:15000},invitations:{enabled:true,policies:{'420':'accept',default:'ignore',riotKey:'private'}}},champSelect:{enabled:true,groups:{ranked:{pick:{enabled:true,strategy:'show-then-lock',champions:{top:[103,86]}}}}}},riotKey:'private',cache:'private',logs:'private'};
 const clean=sanitize(input);assert.deepEqual(sanitize(JSON.parse(JSON.stringify(clean))),clean);assert(!JSON.stringify(clean).includes('private'));assert.equal(clean.watch.rules.invitations.policies['420'],'accept');
 for(const invalid of [{native:{contentProtection:'yes'}},{renderer:{theme:'unknown'}},{watch:{rules:{autoReconnect:{delayMs:-1}}}}])assert.throws(()=>sanitize(invalid));
});
test('R288 desktop preferences apply protection immediately, hardware before ready, tray restores and exits',()=>{
 const source=fs.readFileSync(__dirname+'/../../desktop/main.cjs','utf8');const begin=source.indexOf('const desktopPreferencePath'),end=source.indexOf('\nconst hasInstanceLock',begin);
 const events=[],disk=new Map([['/data/application-preferences.json',JSON.stringify({disableHardwareAcceleration:true})]]),window={setContentProtection:v=>events.push(['protection',v]),isDestroyed:()=>false,isMinimized:()=>true,restore:()=>events.push('restore'),show:()=>events.push('show'),focus:()=>events.push('focus')};
 class Tray{constructor(){events.push('tray')}setToolTip(){}setContextMenu(m){this.menu=m;globalThis.menu=m}on(name,fn){this.click=fn}destroy(){events.push('destroy')}}
 const context={path:require('node:path'),process:{platform:'win32',env:{LOCALAPPDATA:'/cache'}},fs:{statSync:p=>({size:disk.get(p)?.length||0}),readFileSync:p=>disk.get(p),mkdirSync(){},writeFileSync:(p,v)=>disk.set(p,v),renameSync:(a,b)=>disk.set(b,disk.get(a))},app:{getPath:()=>'/data',disableHardwareAcceleration:()=>events.push('hardware-before-ready'),quit:()=>events.push('quit')},mainWindow:window,quitting:false,licenseEnabled:false,iconPath:()=>'/icon',require:n=>n==='electron'?{Tray,Menu:{buildFromTemplate:v=>v}}:'Deep Legends'};
 vm.createContext(context);vm.runInContext(source.slice(begin,end)+'\nglobalThis.testPrefs={updateDesktopPreferences,showMainWindow,get:()=>desktopPreferences,tray:()=>applicationTray};',context);
 assert(events.includes('hardware-before-ready'));context.testPrefs.updateDesktopPreferences({contentProtection:true,closeBehavior:'tray',unknown:'ignored'});assert.deepEqual(events.find(Array.isArray),['protection',true]);assert.equal(context.testPrefs.get().unknown,undefined);
 const tray=context.testPrefs.tray();tray.click();assert(events.includes('restore')&&events.includes('show'));tray.menu[1].click();assert(events.includes('quit'));context.testPrefs.updateDesktopPreferences({closeBehavior:'exit'});assert(events.includes('destroy'));
 const close=source.match(/mainWindow.on\("close", event => \{([\s\S]*?)\n  \}\);/)[1];let prevented=false,hidden=false;
 new Function('clearTimeout','boundsWriteTimer','persistBounds','quitting','desktopPreferences','event','mainWindow',close)(()=>{},null,()=>{},false,{closeBehavior:'tray'},{preventDefault:()=>{prevented=true}},{hide:()=>{hidden=true}});assert(prevented&&hidden);
});
