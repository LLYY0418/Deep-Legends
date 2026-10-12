'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {navigateInstalledDemo,waitInstalledStartup}=require(process.env.R276_DEMO_SOURCE||'./r276-installed-demo.cjs');
const gameplay=fs.readFileSync(path.join(__dirname,'../backend/web/gameplay.js'),'utf8');
const desktopMain=fs.readFileSync(path.join(__dirname,'../desktop/main.cjs'),'utf8');
const bootstrap=desktopMain.match(/mainWindow\.loadURL\(backendReady\.bootstrapUrl\)\.catch\([^\n]+/);assert(bootstrap,'exercise the real startup loadURL failure callback');
const startup=gameplay.match(/requestAnimationFrame\(\(\) => \{\s+const defaultSection = \["overview", "champions", "live", "favorites", "suite"\][\s\S]*?\n  \}\);/);assert(startup,'exercise the actual production default-section callback');
function fixture({navigationPending=false,legacyExport=false,statusReady=true,mainStartupPending=false}={}){
 const frames=[],state={settings:{defaultPage:'live'},section:'live',matches:false},window={__r265ProbeLoad:navigationPending?1:777,deepLegendsMatchCards:{live:{state:{status:{clientView:{state:statusReady?"ready":"no-client"}}}}}};if(legacyExport)delete window.deepLegendsMatchCards.live;let clicks=0,defaultRestores=0,navigated=false,startupTicks=0,bootstrapAborted=false;let finishBootstrap,rejectBootstrap;const bootstrapPromise=new Promise((ok,no)=>{finishBootstrap=ok;rejectBootstrap=no;});
 const document={readyState:mainStartupPending?'loading':'complete',querySelector(selector){
  if(selector==='#setting-default-page + .app-select')return {};
  if(selector==='.match-list .match-entry')return state.matches&&state.section==='overview'?{}:null;
  if(selector==='[data-section="live"]')return {click(){defaultRestores++;state.section='live';state.matches=false;}};
  throw Error('unexpected selector '+selector);
 }};
 const context=vm.createContext({window,document,state,Promise,mainWindow:{loadURL(){return bootstrapPromise;}},backendReady:{bootstrapUrl:'http://127.0.0.1:12345/'},failStartup(){bootstrapAborted=true;state.matches=false;},requestAnimationFrame(fn){frames.push(fn);}});
 vm.runInContext(bootstrap[0],context);if(!mainStartupPending)finishBootstrap();
 const completeNavigation=()=>{window.__r265ProbeLoad=777;vm.runInContext(startup[0],context);};
 const tick=async()=>{if(mainStartupPending&&!navigated&&++startupTicks>=3){document.readyState='complete';mainStartupPending=false;finishBootstrap();}const scheduled=frames.splice(0);for(const fn of scheduled)fn();await Promise.resolve();};
 const call=async(method,params)=>{if(method==='Page.addScriptToEvaluateOnNewDocument'){assert.match(params.source,/window\.__r265ProbeLoad=777/);return;}assert.equal(method,'Page.navigate');assert.equal(params.url,'http://127.0.0.1:12345/?demo&section=overview');if(mainStartupPending)rejectBootstrap(Error("ERR_ABORTED (-3)"));navigated=true;if(!navigationPending)completeNavigation();};
 const until=async expression=>{
  for(let n=0;n<12;n++){
   let done=false,value,error;const result=vm.runInContext(expression,context);Promise.resolve(result).then(v=>{value=v;done=true;},e=>{error=e;done=true;});for(let micro=0;!done&&micro<5;micro++)await Promise.resolve();
   for(let frame=0;!done&&frame<8;frame++){await tick();for(let micro=0;!done&&micro<5;micro++)await Promise.resolve();}if(error)throw error;assert(done,'controlled readiness promise stalled');if(value)return;
   if(navigationPending&&window.__r265ProbeLoad!==777)completeNavigation();await tick();
  }
  throw Error('installed UI condition timeout: '+expression+' '+JSON.stringify({page:'',live:state.section==='live'}));
 };
 const click=async selector=>{assert(navigated);assert.equal(selector,'#section-overview');clicks++;state.section='overview';state.matches=false;frames.push(()=>{if(state.section==='overview'&&!bootstrapAborted)state.matches=true;});};
 return {call,until,click,get bootstrapAborted(){return bootstrapAborted;},origin:'http://127.0.0.1:12345',marker:777,get proof(){return {clicks,defaultRestores,section:state.section,matches:state.matches};}};
}
for(const navigationPending of [false,true])for(const legacyExport of [false,true])test('R276 installed demo waits for actual default-section restoration '+navigationPending+' legacy='+legacyExport,async()=>{
 const f=fixture({navigationPending,legacyExport});await assert.doesNotReject(navigateInstalledDemo(f));assert.deepEqual(f.proof,{clicks:1,defaultRestores:1,section:'overview',matches:true});
});
test('R276 startup waits for the real Electron loadURL before changing documents',async()=>{const f=fixture({mainStartupPending:true});await assert.doesNotReject(async()=>{await waitInstalledStartup(f);await navigateInstalledDemo(f);});assert.equal(f.bootstrapAborted,false);assert.equal(f.proof.matches,true);});
test('R276 old controls-only initial gate aborts the real Electron loadURL',async()=>{const f=fixture({mainStartupPending:true});await f.until("!!document.querySelector('#setting-default-page + .app-select')");await assert.rejects(navigateInstalledDemo(f),/installed UI condition timeout/);assert.equal(f.bootstrapAborted,true);});
test('R276 demo readiness uses the real versioned client view, not removed legacy connected',async()=>{
 const f=fixture({statusReady:false});await assert.rejects(navigateInstalledDemo(f),/installed UI condition timeout/);assert.equal(f.proof.clicks,0);
 const normalized=gameplay.match(/function updateStatus\(status\) \{[\s\S]*?\n  \}/);assert(normalized);const state={status:{}};const c=vm.createContext({state,updateClientView(view){state.status.clientView=view;}});vm.runInContext(normalized[0]+`;updateStatus({connected:true,clientView:{state:'ready'}});`,c);assert.equal(state.status.connected,undefined);assert.equal(state.status.clientView.state,'ready');
});
test('R276 old controls-only probe deterministically loses its click to the real default-page callback',async()=>{
 const f=fixture();await f.call('Page.addScriptToEvaluateOnNewDocument',{source:'window.__r265ProbeLoad=777;'});await f.call('Page.navigate',{url:f.origin+'/?demo&section=overview'});
 await f.until("window.__r265ProbeLoad===777 && !!document.querySelector('#setting-default-page + .app-select')");await f.click('#section-overview');
 await assert.rejects(f.until("document.querySelector('.match-list .match-entry')"),/installed UI condition timeout:[\s\S]*"live":true/);assert.equal(f.proof.defaultRestores,1);
});
test('R276 all four installed probes use the same real-resource demo readiness',()=>{
 for(const n of [265,266,269,272]){const source=fs.readFileSync(path.join(__dirname,`r${n}-persisted-ui.cjs`),'utf8');assert(source.includes("require('./r276-installed-demo.cjs')"));assert(source.includes('navigateInstalledDemo({call,until,click,origin})'));assert(source.includes('for(let n=0;n<200;n++)'));assert(source.includes('waitInstalledStartup({until})'));}
 const source=fs.readFileSync(path.join(__dirname,'r276-installed-demo.cjs'),'utf8');assert(!source.includes('Fetch.fulfillRequest'));assert(!source.includes('localStorage.setItem'));
});
