'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs'),{clock}=require('./r301-harness.cjs');
const read=(name)=>fs.readFileSync(process.env['R301_'+name.replaceAll(/[^a-z]/gi,'_').toUpperCase()]||path.join(__dirname,name),'utf8');
const app=read('app.js'),game=read('gameplay.js');
function refreshFixture({api=async()=>{},wait=false,syncing=false}={}){
 const dom=new JSDOM('<button><i class="refresh-icon"></i><span>重新读取</span></button>'),w=dom.window,c=clock(),state={status:{connected:true,syncing},clientView:{state:'ready'}},button=w.document.querySelector('button'),reports=[];
 const el={refresh:button};for(const key of ['ownedCount','chromaCount','poolCount','remainingCount','poolSource'])el[key]=w.document.createElement('span');
 w.reportFlowDiagnostic=(...args)=>reports.push(args);if(wait)w.addEventListener('deep-legends:hard-refresh',e=>e.detail.waitFor.push(new Promise(()=>{})));
 const deps={state,el,window:w,document:w.document,CustomEvent:w.CustomEvent,performance:{now:c.now},setTimeout:c.setTimeout,clearTimeout:c.clearTimeout,api,showToast(){},softResetShellState(){},refreshStatus:async()=>{state.status.syncing=syncing;},acceptClientView(){},renderClientConnection(){},formatNumber:String,safeHTTPURL:()=>false,renderNotice(){},renderLaunchpad(){},updateWorkspaceAvailability(){},renderUpdateStatus(){}};
 const f=compile(app,['runManualRefresh','renderStatus','setRefreshBusy'],deps);return {w,c,state,button,reports,...f,close:()=>w.close()};
}
test('R301 manual refresh bounds API and unresolved hard refresh and records stage',async()=>{
 for(const [options,stage]of [[{api:()=>new Promise(()=>{})},'api'],[{wait:true},'hard-refresh-wait']]){const h=refreshFixture(options);try{const task=h.runManualRefresh();await h.c.advance(20000);await task;assert.equal(h.state.manualRefreshing,false);assert.equal(h.button.disabled,false);assert.equal(h.button.textContent,'重新读取');assert(h.reports.some(r=>r[1]==='timeout'&&r[2].stage===stage&&r[2].busy_ms===20000));}finally{h.close();}}
});
test('R302 manual refresh dispatches only after delayed identity API settles and also dispatches on ordinary error',async()=>{
 for(const fail of [false,true]){let returned=false;const seen=[];const h=refreshFixture({api:async()=>{await new Promise(resolve=>h.c.setTimeout(resolve,75));returned=true;if(fail)throw Error('ordinary API failure');}});
 try{h.w.addEventListener('deep-legends:hard-refresh',()=>seen.push(returned));const task=h.runManualRefresh();await h.c.advance(74);assert.equal(returned,false);assert.deepEqual(seen,[]);await h.c.advance(1);await task;assert.equal(returned,true);assert.deepEqual(seen,[true]);assert.equal(h.button.disabled,false);}finally{h.close();}}
});
test('R301 no-client and deadline release immediately; old finally cannot release a new token',async()=>{
 const h=refreshFixture({wait:true});try{const old=h.runManualRefresh();await h.c.flush();h.state.clientView.state='no-client';h.renderStatus();assert.equal(h.button.disabled,false);assert(h.reports.some(r=>r[1]==='released-no-client'));
 h.state.clientView.state='ready';h.state.status.connected=true;const next=h.runManualRefresh();await h.c.flush();await old;assert.equal(h.state.manualRefreshing,true);assert.equal(h.button.disabled,true);
 h.state.manualRefreshDeadline=0;h.renderStatus();await next;assert.equal(h.button.disabled,false);assert(h.reports.some(r=>r[1]==='released-deadline'));}finally{h.close();}
});
test('R301 gameplay hard refresh bounds unending request/controller and exits on disconnect',async()=>{
 for(const disconnect of [false,true]){const c=clock(),state={section:'overview',controllers:new Map(),destroyed:false},reports=[];let connected=true,aborted=0;state.controllers.set('overview:one',{abort(){aborted++;}});
 const f=compile(game,['handleHardRefresh'],{state,performance:{now:c.now},setTimeout:c.setTimeout,window:{reportFlowDiagnostic:(...a)=>reports.push(a)},softResetGameplayState(){},overviewGroupForSection:()=> 'self',activeTab:()=>({}),tabReady:()=>true,loadOverview:()=>new Promise(()=>{}),loadLive:()=>new Promise(()=>{}),renderOverview(){},renderLive(){},connected:()=>connected,scheduleBeaconPoll(){}});
 let settled=false;const task=f.handleHardRefresh().then(()=>settled=true);if(disconnect){connected=false;await c.advance(20);}else await c.advance(8000);await c.flush();assert(settled,'hard refresh exceeded 8s');await task;assert.equal(aborted,disconnect?0:1);assert.equal(reports.length,disconnect?0:1);if(!disconnect)assert.equal(reports[0][2].stage,'hard-refresh-wait');}
});
test('R301 desktop unconditionally allows unload, preserves tray close, and pages cannot veto unloading',()=>{
 const main=fs.readFileSync(process.env.R301_MAIN_SOURCE||path.join(__dirname,'../../desktop/main.cjs'),'utf8');let handler;const match=main.match(/mainWindow.webContents.on\("will-prevent-unload",[^\n]+/);assert(match,'unload fallback registered');vm.runInNewContext(match[0],{mainWindow:{webContents:{on:(event,fn)=>{assert.equal(event,'will-prevent-unload');handler=fn;}}}});let prevented=0;handler({preventDefault:()=>prevented++});assert.equal(prevented,1);
 for(const file of fs.readdirSync(__dirname).filter(n=>n.endsWith('.js'))){const source=file==='auto-config.js'?read(file):fs.readFileSync(path.join(__dirname,file),'utf8');for(const hit of source.matchAll(/(?:beforeunload|onbeforeunload)[\s\S]{0,160}/g))assert(!/preventDefault|returnValue/.test(hit[0]),file+' must never veto unload');}
 const close=main.match(/mainWindow.on\("close", event => \{([\s\S]*?)\n  \}\);/)[1];let hidden=false;new Function('clearTimeout','boundsWriteTimer','persistBounds','quitting','desktopPreferences','event','mainWindow',close)(()=>{},null,()=>{},false,{closeBehavior:'tray'},{preventDefault(){}},{hide:()=>hidden=true});assert(hidden);
 assert(main.includes('window.deepLegendsAutoConfig?.flushDraft(); window.deepLegendsPreferenceStorage?.flush()'),'quit flushes draft before stopping backend');
});
test('R301 draft validation rejects an entire corrupt category and keeps valid incomplete selections',()=>{
 const {validEditorDraft}=require('./auto-config.js'),catalog=structuredClone(require('../testdata/r298-review-catalog.json').catalog);
 const good={runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]}};assert(validEditorDraft(good,catalog,'ranked'));
 for(const mutate of [d=>d.runes.selectedPerkIds[0]=999999,d=>d.spells={spell1Id:9999,spell2Id:4},d=>d.items={core:[9999]},d=>d.runes.selectedPerkIds.pop(),d=>d.runes.subStyleId=8000]){const bad=structuredClone(good);mutate(bad);assert.equal(validEditorDraft(bad,catalog,'ranked'),false);}
 const incomplete=structuredClone(good);incomplete.runes.selectedPerkIds[0]=0;assert(validEditorDraft(incomplete,catalog,'ranked'));
});

async function draftEditor({disk={},saved,local={},offline=false}={}){
 const dom=new JSDOM('<main id="app-frame"><div id="suite-config-root"></div></main>',{url:'http://127.0.0.1:47391',runScripts:'outside-only'}),w=dom.window,c=clock(),catalog=structuredClone(require('../testdata/r298-review-catalog.json').catalog),writes=[];
 catalog.styles[0].slots[0].perks.push({...catalog.styles[0].slots[0].perks[0],id:101,name:'替代基石'});Object.assign(catalog,{clientConnected:true,itemsStatus:'ready',spellsStatus:'ready'});saved||={version:1,champions:{1:{ranked:{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]}}}}};
 w.setTimeout=c.setTimeout;w.clearTimeout=c.clearTimeout;w.AbortSignal=AbortSignal;for(const [key,value]of Object.entries(local))w.localStorage.setItem(key,value);
 w.HTMLDialogElement.prototype.showModal=function(){this.open=true};w.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new w.Event('close'))};
 w.deepLegendsRuntime={escapeHTML:String};w.deepLegendsSections={register(){}};w.fetch=async(url,init={})=>{let value={};if(url==='/api/ui-preferences'){if(init.method==='POST'){if(offline)throw Error('offline');const b=JSON.parse(init.body);for(const [key,v]of Object.entries(b.values))if(b.mode!=='migrate'||!Object.hasOwn(disk,key)){if(v===null)delete disk[key];else disk[key]=v;}}value={schema:1,values:disk};}
 else if(url==='/api/auto-config/settings')value=saved;else if(url==='/api/auto-config/catalog')value=catalog;else if(url==='/api/champions/catalog')value={champions:Array.from({length:52},(_,i)=>({id:i+1,nameZh:String(i+1).padStart(2,'0')}))};else if(url==='/api/settings/summoner-keys')value={keys:['D','F']};else if(url.startsWith('/api/auto-config/champions/')){const body=JSON.parse(init.body);writes.push(body);saved.champions[1].ranked=body;saved.version++;value=saved;}return {ok:true,status:200,json:async()=>structuredClone(value),text:async()=>''};};
 w.eval(read('ui-preferences.js'));await w.deepLegendsPreferenceStorage.ready;w.eval(read('auto-config.js'));await w.deepLegendsAutoConfig.load();
 const click=async selector=>{const n=w.document.querySelector(selector);assert(n,selector);n.click();await c.flush();};
 return {w,c,disk,saved,writes,click,local:()=>Object.fromEntries(Array.from({length:w.localStorage.length},(_,i)=>{const k=w.localStorage.key(i);return [k,w.localStorage.getItem(k)]})),close:()=>w.close()};
}
test('R301 pagehide synchronously retains final draft, reload restores unsaved and explicit save alone publishes',async()=>{
 const h=await draftEditor();let next;try{
 const original=structuredClone(h.saved);await h.click('[data-rune="101"]');assert.equal(h.w.document.querySelector('[data-save-state]').textContent,'未保存');
 h.w.dispatchEvent(new h.w.Event('pagehide'));const raw=h.w.deepLegendsPreferenceStorage.getItem('lol-loot-auto-config-draft-1-ranked');assert(raw,'pagehide must flush before the debounce');assert.equal(JSON.parse(raw).value.runes.selectedPerkIds[0],101);assert.deepEqual(h.saved,original);assert.equal(h.writes.length,0,'draft never enters automatic application source');
 await h.w.deepLegendsPreferenceStorage.flush();next=await draftEditor({disk:h.disk,saved:h.saved,local:h.local()});assert.equal(next.w.document.querySelector('[data-save-state]').textContent,'未保存');assert(!next.w.document.querySelector('[data-act="save"]').disabled);await next.click('[data-act="save"]');assert.equal(next.writes.length,1);assert.equal(next.saved.champions[1].ranked.runes.selectedPerkIds[0],101);assert.equal(next.w.deepLegendsPreferenceStorage.getItem('lol-loot-auto-config-draft-1-ranked'),null);
 }finally{h.close();next?.close();}
});
test('R301 slot/region/tab/same rune do not create draft; corrupt entry drops wholly; revert clears persisted draft',async()=>{
 const h=await draftEditor();try{await h.click('[data-tab="kit"]');await h.click('[data-act="start-spells"]');await h.click('[data-slot="1"]');await h.click('[data-act="start-items"]');await h.click('[data-item-group="core"]');await h.click('[data-tab="runes"]');await h.click('[data-rune="1"]');await h.c.advance(1000);assert.equal(h.w.deepLegendsPreferenceStorage.getItem('lol-loot-auto-config-draft-1-ranked'),null);
 await h.click('[data-rune="101"]');h.w.dispatchEvent(new h.w.Event('pagehide'));await h.click('[data-act="revert"]');assert.equal(h.w.deepLegendsPreferenceStorage.getItem('lol-loot-auto-config-draft-1-ranked'),null);assert(!h.w.document.querySelector('[data-save-state]').classList.contains('ac-dirty'));
 const key='lol-loot-auto-config-draft-1-ranked',row={championId:1,modeKey:'ranked',revision:1,at:1,value:{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[99999,2,3,4,12,13,5008,5008,5001]}}};h.w.deepLegendsPreferenceStorage.setItem(key,JSON.stringify(row));await h.w.deepLegendsAutoConfig.load(true);assert.equal(h.w.deepLegendsPreferenceStorage.getItem(key),null);assert.equal(h.w.document.querySelector('[data-rune="1"]').getAttribute('aria-pressed'),'true');
 }finally{h.close();}
});
test('R301 draft cap evicts oldest; stale revision uses existing conflict; offline journal survives close',async()=>{
 const prefix='lol-loot-auto-config-draft-',disk={},rows=[];for(let id=2;id<=51;id++){const key=prefix+id+'-ranked';disk[key]='{}';rows.push({key,at:id});}disk[prefix+'index']=JSON.stringify(rows);
 const h=await draftEditor({disk,offline:true});let next;try{await h.click('[data-rune="101"]');h.w.dispatchEvent(new h.w.Event('pagehide'));assert.equal(JSON.parse(h.w.deepLegendsPreferenceStorage.getItem(prefix+'index')).length,50);assert.equal(h.w.deepLegendsPreferenceStorage.getItem(prefix+'2-ranked'),null);
 await h.w.deepLegendsPreferenceStorage.flush();next=await draftEditor({disk,saved:h.saved,local:h.local()});assert.equal(next.w.document.querySelector('[data-save-state]').textContent,'未保存');assert.equal(next.writes.length,0);next.saved.version++;next.saved.champions[1].ranked.runes.selectedPerkIds[0]=0;await next.click('[data-act="save"]');assert(next.w.document.querySelector('.ac-feedback').textContent.includes('配置已更新'));assert.equal(next.writes.length,0);
 }finally{h.close();next?.close();}
});
test('R301 dialog refreshes only after ten-minute successful check and changed updates closed buttons',async()=>{
 const feed=JSON.parse(fs.readFileSync(path.join(__dirname,'../patchnotes/patch-notes.json'))),tick=()=>new Promise(setImmediate);
 for(const old of [false,true]){const dom=new JSDOM('<main id="root"></main>',{url:'http://fixture',runScripts:'outside-only'}),w=dom.window,calls=[];let latest='26.20';w.deepLegendsRuntime={escapeHTML:String};w.deepLegendsSections={register(){}};w.fetch=async url=>{calls.push(url);return {ok:true,json:async()=>({...structuredClone(feed.versions.find(v=>v.version===latest)),successfulAt:new Date(Date.now()-(old?660000:60000)).toISOString()})}};
 try{w.eval(read('patch-notes.js'));await tick();w.document.querySelector('main').innerHTML=w.deepLegendsPatchNotes.button('ranked');w.document.querySelector('button').click();await tick();assert.equal(calls.filter(u=>u.includes('refresh=1')).length,old?1:0);w.document.querySelector('[data-pn-close]').click();latest='26.21';w.dispatchEvent(new w.Event('deep-legends:patch-notes-changed'));await tick();assert.equal(w.document.querySelector('[data-patch-notes] b').textContent,'26.21');assert.equal(w.document.querySelector('.pn-dialog'),null);}finally{w.close();}}
});
test('R302 fallback notes say updated without claiming an official check',async()=>{
 const feed=JSON.parse(fs.readFileSync(path.join(__dirname,'../patchnotes/patch-notes.json'))),dom=new JSDOM('<main></main>',{url:'http://fixture',runScripts:'outside-only'}),w=dom.window;
 w.deepLegendsRuntime={escapeHTML:String};w.deepLegendsSections={register(){}};w.fetch=async()=>({ok:true,json:async()=>({...structuredClone(feed.versions[0]),updateStatus:'fallback',successfulAt:new Date().toISOString()})});
 try{w.eval(read('patch-notes.js'));await new Promise(setImmediate);w.document.querySelector('main').innerHTML=w.deepLegendsPatchNotes.button('ranked');w.document.querySelector('button').click();await new Promise(setImmediate);const text=w.document.querySelector('.pn-foot').textContent;assert(text.includes('已更新公告'));assert(!text.includes('已检查官方公告'));}finally{w.close();}
});
test('R301 repeated refresh diagnostics retain identical timeout durations without dedup loss',async()=>{
 const dom=new JSDOM('',{url:'http://fixture',runScripts:'outside-only'}),w=dom.window,calls=[];w.fetch=async(url,init)=>{calls.push(JSON.parse(init.body));return {status:204}};
 try{w.eval(read('runtime.js'));for(let i=0;i<2;i++){w.reportFlowDiagnostic('manual_refresh_client','released-no-client',{stage:'api',busy_ms:1});await new Promise(setImmediate);}assert.equal(calls.length,2);assert(calls.every(x=>x.busy_ms===1&&x.stage==='api'));}finally{w.close();}
});
test('R301 preference flush serializes pending pagehide writes and confirms only the latest value',async()=>{
 const dom=new JSDOM('',{url:'http://fixture',runScripts:'outside-only'}),w=dom.window,requests=[],disk={};w.AbortSignal=AbortSignal;
 w.fetch=async(url,init={})=>{if(init.method!=='POST')return {ok:true,json:async()=>({schema:1,values:disk})};const batch=JSON.parse(init.body);await new Promise(resolve=>requests.push({batch,resolve}));Object.assign(disk,batch.values);return {ok:true,json:async()=>({schema:1,values:disk})};};
 try{w.eval(read('ui-preferences.js'));const storage=w.deepLegendsPreferenceStorage;await storage.ready;storage.setItem('lol-loot-theme','dark');const first=storage.flush();storage.setItem('lol-loot-theme','light');const final=storage.flush(true);assert.equal(requests.length,1);requests[0].resolve();await new Promise(setImmediate);assert.equal(requests.length,2);requests[1].resolve();await Promise.all([first,final]);assert.equal(disk['lol-loot-theme'],'light');assert.equal(w.localStorage.getItem('deep-legends-ui-pending-v1'),null);}finally{w.close();}
});
