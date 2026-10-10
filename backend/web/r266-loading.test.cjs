'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R266_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8'),app=fs.readFileSync(process.env.R266_APP_SOURCE || __dirname+'/app.js','utf8');
test('R266 P3 pending streamed matches show five skeletons; arrived zero shows empty',async()=>{
 const tab={loading:true,data:{player:{},matches:[]},overviewCardLoad:{ready:new Set()}};
 const f=compile(source,['matchesPending','renderPendingMatches','matchListEmptyContent','matchSentinelShouldHide'],{historyServiceState:()=>null,emptyState:(title,copy)=>title+copy,globalThis:{},advancedFilterContext:()=>{}});
 for(const at of [0,500,1000,1400]){if(at)await new Promise(r=>setTimeout(r,at===500?500:at===1000?500:400));const html=f.matchListEmptyContent(tab,null,'');assert.equal((html.match(/class="match-skeleton gameplay-skeleton"/g)||[]).length,5);assert(!/没有符合|共 0 场|战绩未读全/.test(html));assert(f.matchSentinelShouldHide(tab,false));}
 tab.matchesReceived=true;tab.loading=false;assert.match(f.matchListEmptyContent(tab,null,''),/没有符合条件的对局/);
});
test('R266 P4 new session resets previous collection clock; failure reason belongs to current session',()=>{
 const state={collectionSessionEpoch:1,collectionRequestAt:Date.now()-300000,collectionRequestAttempt:'old',collectionFailureSignature:'old'},events=[];
 const f=compile(app,['syncCollectionSession','collectionReadFailed'],{state,window:{reportFlowDiagnostic:(...args)=>events.push(args)}});
 assert.equal(f.collectionReadFailed({clientSessionEpoch:2,connected:true,snapshotReady:false}),false);assert.equal(state.collectionRequestAt,0);assert.equal(state.collectionRequestAttempt,'');assert.equal(events.length,0);
 assert.equal(f.collectionReadFailed({clientSessionEpoch:2,lastError:'sensitive original error'}),true);assert.equal(events[0][1],'last_error');assert.equal(events[0][2].sessionEpoch,2);assert(!JSON.stringify(events).includes('sensitive'));
});
test('R266 P9 suite DOM and keyboard ordering use the same array',()=>{
 const {JSDOM}=require('../../desktop/node_modules/jsdom');for(const file of ['index.html','default/index.html']){const doc=new JSDOM(fs.readFileSync(__dirname+'/'+file,'utf8')).window.document;assert.deepEqual([...doc.querySelectorAll('[data-suite-tab]')].map(n=>n.dataset.suiteTab),['watch','champselect','facade','sweep','rig']);}
 const doc=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8')).window.document,tabs=[...doc.querySelectorAll('[data-suite-tab]')],calls=[];const f=compile(fs.readFileSync(__dirname+'/suite.js','utf8'),['setupTabs'],{tabs,state:{tab:'watch'},activateTab:(name)=>calls.push(name)});f.setupTabs();tabs[0].dispatchEvent(new doc.defaultView.KeyboardEvent('keydown',{key:'ArrowRight'}));assert.equal(calls.at(-1),'champselect');tabs[0].dispatchEvent(new doc.defaultView.KeyboardEvent('keydown',{key:'ArrowLeft'}));assert.equal(calls.at(-1),'rig');
});
test('R266 P7 repeated foreground retry waits for the original request',async()=>{
 let calls=0;const finishes=[],noop=()=>{},tab={key:'fixture',region:'kr',matchFilter:'all'},state={settings:{matchCount:10},controllers:new Map()};
 const f=compile(source,['loadOverview'],{state,tabReady:()=>true,tabGroup:()=> 'kr',riotTab:()=>true,connected:()=>false,clearTimeout:noop,performance,rerenderTab:noop,api:()=>{calls++;return new Promise(resolve=>finishes.push(resolve))},rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture'});
 const first=f.loadOverview(tab,true),second=f.loadOverview(tab,true),whileLoading=calls;for(const finish of finishes)finish({player:{playerRef:'fixture',region:'kr'},matches:[],pagination:{count:10,hasMore:false}});await Promise.all([first,second]);assert.equal(whileLoading,1,'retry must not replace the running request');
});
test('R266 P7 loading retry buttons wait; failed partial next page retains a visible retry result',()=>{
 const f=compile(source,['renderHistoryServiceStatus','paginationCopyFor'],{historyServiceState:()=>({kind:'relay',seconds:0}),serviceOutageIcon:()=>'',matchesPending:()=>false,number:String,escapeHTML});
 assert.match(f.renderHistoryServiceStatus({loading:true}),/data-gameplay-retry disabled aria-busy="true">读取中/);assert.match(f.renderHistoryServiceStatus({loadingMore:true},true),/data-gameplay-retry disabled aria-busy="true">读取中/);
 assert.match(f.paginationCopyFor({data:{matches:[{gameId:1}],pagination:{hasMore:true,partial:true,moreError:'synthetic timeout'}}}),/这一页读取失败.*data-load-more>重试/);
});
