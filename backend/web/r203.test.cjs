'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R203_APP_SOURCE||path.join(__dirname,'app.js'),'utf8');
function fixture(items=[]) {
 const dom=new JSDOM('<main id="frame"><section id="notice" hidden></section><div id="grid"></div></main><div id="overlay" hidden></div><p id="title"></p><p id="copy"></p><p id="meta"></p><button id="retry"></button><button id="list-retry"></button><p id="list-meta"></p>',{pretendToBeVisual:true});
 const doc=dom.window.document;let now=0,seq=0;const timers=new Map(),requests=[],events=[];
 const state={status:{connected:true,identityReady:true,snapshotReady:true,calculationOK:true,lastSync:'0'},section:'favorites',favoritesPage:'collection',view:'owned',items,loading:false,skinLoadGeneration:0,renderGeneration:0,qualitySelections:new Set(),sort:'name',showUnownedChromas:true,showPrestigeChromas:true,statusRequestToken:0,skinsCache:new Map(),controllers:new Map()};
 const el={appFrame:doc.querySelector('#frame'),notice:doc.querySelector('#notice'),grid:doc.querySelector('#grid'),startupLoading:doc.querySelector('#overlay'),startupLoadingTitle:doc.querySelector('#title'),startupLoadingCopy:doc.querySelector('#copy'),startupLoadingMeta:doc.querySelector('#meta'),startupLoadingRetry:doc.querySelector('#retry'),retryList:doc.querySelector('#list-retry'),listMeta:doc.querySelector('#list-meta'),refresh:{click(){throw Error('full refresh forbidden')}}};
 for(const item of items){const card=doc.createElement('article');card.className='skin-card';card.textContent=String(item.id);el.grid.append(card);}
 const deps={state,el,document:doc,window:{reportFlowDiagnostic:(...row)=>events.push(row)},Date:{now:()=>now},setTimeout:(fn,delay)=>{timers.set(++seq,{fn,at:now+delay});return seq},clearTimeout:id=>timers.delete(id),api:async(url)=>{requests.push(url);if(url==='/api/status')return {...state.status};if(url.startsWith('/api/skins'))return {items};return null},escapeHTML,formatNumber:String,formatDateTime:String,acquisitionTime:()=>null,configureSortControls(){},cancelRenderFrames(){},cancelDeferredImages(){},stopHoverVideo(){},showToast(){},renderStatus(){},loadClientInstallations(){},clearDisconnectedClientState(){},scheduleStatus(){},loadAccount(){},loadPools(){},STATUS_INTERVAL:10000};
 const h=compile(source,['updateReadingOverlay','showReadingOverlay','hideReadingOverlay','retrySummonerIdentity','renderNotice','loadSkins','applySkinsPayload','sameCollectionItems','snapshotRetryText','ensureCollection','triggerCollectionRescanIfDirty','refreshStatus','renderItems'],deps);
 async function advance(ms){const target=now+ms;for(;;){const entry=[...timers].filter(([,row])=>row.at<=target).sort((a,b)=>a[1].at-b[1].at)[0];if(!entry)break;now=entry[1].at;timers.delete(entry[0]);await entry[1].fn();}now=target;}
 return {dom,doc,state,el,requests,events,timers,advance,...h};
}
test('R203 1183 cards survive lastSync changes every eight seconds without global blocking',async()=>{
 const f=fixture(Array.from({length:1183},(_,id)=>({id})));try{const first=f.el.grid.firstChild;for(let i=0;i<8;i++){f.state.status={...f.state.status,lastSync:String(i+1)};await f.refreshStatus(true);f.state.skinsCache.clear();await f.loadSkins(false);assert.equal(f.el.startupLoading.hidden,true);assert.equal(f.el.appFrame.hasAttribute('inert'),false);assert.equal(f.el.grid.children.length,1183);assert.equal(f.el.grid.firstChild,first);await f.advance(8000);}assert(!f.events.some(e=>e[0]==='blocking_state_client'&&e[1]==='show'));}finally{f.dom.window.close();}
});
test('R203 first empty collection shows only a grid placeholder and no indefinite loading flag',async()=>{
 const f=fixture();try{f.state.status.snapshotReady=false;await f.loadSkins();f.updateReadingOverlay(true);assert.match(f.el.grid.textContent,/收藏信息还在准备中/);assert.equal(f.el.startupLoading.hidden,true);assert.equal(f.el.appFrame.hasAttribute('inert'),false);assert.equal(f.state.loading,false);}finally{f.dom.window.close();}
});
test('R203 missing identity escapes at fifteen seconds across repeated shows and retries',async()=>{
 const f=fixture();try{f.state.status.identityReady=false;f.updateReadingOverlay();assert(f.el.appFrame.hasAttribute('inert'));for(let i=0;i<3;i++){await f.advance(3000);f.updateReadingOverlay();if(i<2)await f.retrySummonerIdentity();}await f.advance(5999);assert.equal(f.el.startupLoading.hidden,false);await f.advance(1);assert.equal(f.el.startupLoading.hidden,true);assert.equal(f.el.appFrame.hasAttribute('inert'),false);assert.match(f.el.notice.textContent,/召唤师信息读取失败/);assert(f.el.notice.querySelector('.identity-retry'));for(let i=0;i<4;i++){f.updateReadingOverlay();await f.advance(1000);assert.equal(f.el.startupLoading.hidden,true);}}finally{f.dom.window.close();}
});
test('R203 overlay retry is identity-only, single-flight and one-second debounced',async()=>{
 const f=fixture();try{await Promise.all([f.retrySummonerIdentity(),f.retrySummonerIdentity()]);await f.retrySummonerIdentity();assert.deepEqual(f.requests,['/api/identity/refresh?source=overlay_retry']);await f.advance(1000);await f.retrySummonerIdentity();assert.equal(f.requests.length,2);assert(!f.requests.some(x=>x.startsWith('/api/refresh')));assert.match(source,/startupLoadingRetry\?\.addEventListener\("click", retrySummonerIdentity\)/);}finally{f.dom.window.close();}
});
test('R203 sustained dirty statuses yield one automatic scan per sixty seconds, foreground favorites only',async()=>{
 const f=fixture();try{f.state.status.collectionDirty=true;assert(f.triggerCollectionRescanIfDirty());for(let i=0;i<59;i++){f.state.collectionRescanInFlight=false;await f.advance(1000);assert.equal(f.triggerCollectionRescanIfDirty(),false);}assert.equal(f.requests.length,1);await f.advance(1000);f.state.collectionRescanInFlight=false;assert(f.triggerCollectionRescanIfDirty());await f.advance(60000);f.state.collectionRescanInFlight=false;f.state.section='overview';assert.equal(f.triggerCollectionRescanIfDirty(),false);f.state.section='favorites';Object.defineProperty(f.doc,'hidden',{value:true,configurable:true});assert.equal(f.triggerCollectionRescanIfDirty(),false);assert.equal(f.requests.length,2);}finally{f.dom.window.close();}
});
test('R203 native dialogs log show and user close with category, never raw IDs',()=>{
 const doc=new JSDOM('<dialog id="skin-dialog-secret"></dialog>').window.document;
 const events=[],proto={showModal(){this.open=true;},close(){this.open=false;}};const win={HTMLDialogElement:{prototype:proto},reportFlowDiagnostic:(...v)=>events.push(v)};
 const runtime=fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8');const h=compile(runtime,['installBlockingDiagnostics'],{});h.installBlockingDiagnostics(win,doc);const dialog=doc.querySelector('dialog');proto.showModal.call(dialog);dialog.dispatchEvent(new doc.defaultView.Event('close'));assert.deepEqual(events.map(e=>e.slice(0,2)),[['blocking_state_client','show'],['blocking_state_client','hide']]);assert.equal(events[0][2].source,'skin');assert(!JSON.stringify(events).includes('secret'));dialog.open=false;proto.showModal.call(dialog);dialog.remove();proto.close.call(dialog);assert.equal(events.length,4,'detached dialog close must log once');doc.defaultView.close();
});

test('R203 collection render telemetry preserves visibility and card count',()=>{
 const vm=require('node:vm'),bodies=[];const context={window:{},AbortController,setTimeout,clearTimeout,fetch:async(_url,options)=>{bodies.push(JSON.parse(options.body));return {status:204};},URL,TextEncoder,location:{origin:'http://localhost'}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);context.window.reportFlowDiagnostic('collection_render_client','unchanged-suppressed',{view:'owned',itemCount:1183,force:true,keptVisible:true});assert.equal(bodies.length,1);assert.equal(bodies[0].itemCount,1183);assert.equal(bodies[0].view,'owned');assert.equal(bodies[0].force,true);assert.equal(bodies[0].keptVisible,true);
});

test('R203 native confirmation preserves cancellation and logs no message text',()=>{
 const events=[],win={confirm:()=>false,reportFlowDiagnostic:(...e)=>events.push(e)};
 compile(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),['installBlockingDiagnostics'],{}).installBlockingDiagnostics(win,{});
 assert.equal(win.confirm('fixture-secret'),false);assert.deepEqual(events.map(e=>e.slice(0,2)),[['blocking_state_client','show'],['blocking_state_client','hide']]);assert(!JSON.stringify(events).includes('fixture-secret'));
});
