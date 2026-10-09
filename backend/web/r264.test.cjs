'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(__dirname+'/gameplay.js','utf8');
const app=fs.readFileSync(__dirname+'/app.js','utf8');
test('R264 P1 count and active player exclude hidden Tencent tabs without losing their state',()=>{
 const self={key:'current',current:true,closed:true,region:''};
 const search={key:'cn-search',region:'',matchFilter:'solo',scrollTop:712,advancedConditions:{result:{values:['win']}}};
 const state={status:{clientView:{state:'exiting'}},tabs:[self,search],activeTabs:{players:search.key},activeGroup:'players'};
 const f=compile(source,['connected','riotTab','tabGroup','activeTab','playerGroupCount'],{state,overviewGroupForSection:()=>state.activeGroup});
 assert.equal(f.playerGroupCount('players'),0);assert.equal(f.activeTab('players'),null);
 state.status.clientView.state='ready';self.closed=false;
 assert.equal(f.playerGroupCount('players'),2);assert.equal(f.activeTab('players'),search);
 assert.equal(search.scrollTop,712);assert.equal(search.matchFilter,'solo');assert.deepEqual(search.advancedConditions.result.values,['win']);
});
test('R264 P3 lazy filter first click opens the panel after script loading',async()=>{
 let handler,loaded=false,renders=0;
 const tab={};const button={addEventListener:(_name,callback)=>handler=callback};
 const container={querySelector:selector=>selector==='[data-history-filter-load]'?button:null,querySelectorAll:()=>[]};
 const f=compile(source,['bindMatchFilterControls'],{state:{destroyed:false},globalThis:{},ensureAdvancedFilters:async()=>{loaded=true;renders++},renderFilteredMatchView:()=>renders++,bindAppSelect:()=>{},updateMatchFilter:()=>{}});
 f.bindMatchFilterControls(container,tab);await handler();
 assert(loaded);assert.equal(tab.advancedMenu?.open,true);assert.equal(renders,1);
});
test('R264 P3 malformed saved filters are discarded rather than reused',()=>{
 const af=require('./history-filters.js'),data=new Map([['deep-legends-history-presets-v1',JSON.stringify([{name:'broken',conditions:'bad'}])]]);
 const storage={getItem:k=>data.get(k),removeItem:k=>data.delete(k)};
 assert.deepEqual(af.readPresets(storage),[]);assert.equal(data.has('deep-legends-history-presets-v1'),false);
});
test('R264 P2 late 409 after a ready event retries once rather than getting stuck',async()=>{
 const state={status:{connected:true,snapshotReady:false},clientView:{state:'ready',generation:1},collectionEnsureInFlight:false};let finish,calls=0,renders=0;
 const f=compile(app,['ensureCollection','retryCollectionAfterConnection'],{state,api:()=>{calls++;return calls===1?new Promise((_,reject)=>finish=reject):Promise.resolve()},renderItems:()=>renders++});
 const pending=f.ensureCollection();state.clientView={state:'ready',generation:2};finish(Object.assign(new Error('fixture conflict'),{status:409}));await pending;await Promise.resolve();
 assert.equal(calls,2);assert.equal(state.collectionWaitingForConnection,false);assert.equal(renders,2);
 f.retryCollectionAfterConnection(state.clientView);assert.equal(calls,2);
});
test('R264 P6 relay backoff has one short notice while the already-read data remains available',()=>{
 const tab={region:'kr',initialPageError:'战绩服务暂时不可用',data:{matches:[{gameId:1}],ranks:[{tier:'DIAMOND'}],masteries:[{championId:103}],capabilities:[{detail:'战绩服务暂时不可用'},{detail:'战绩服务暂时不可用'}]}};
 const f=compile(source,['riotTab','relaySlowState','renderRelaySlowNotice'],{});
 const notice=f.renderRelaySlowNotice(tab);assert.equal((notice.match(/外服战绩服务较慢/g)||[]).length,1);assert.doesNotMatch(notice,/HTTP|SGP|LCU/);
 assert.equal(tab.data.matches.length,1);assert.equal(tab.data.ranks.length,1);assert.equal(tab.data.masteries.length,1);
});
test('R264 P5 automatic retry is single, quiet, and stops after recovery',()=>{
 const tab={data:{capabilities:[{name:'match-history',state:'failed',detail:'战绩服务器暂时不可用，稍后自动重试'}]}};
 const timers=new Map(),calls=[];let sequence=0;
 const f=compile(source,['scheduleHistoryServerRetry'],{state:{destroyed:false},setTimeout:(fn,delay)=>{assert.equal(delay,60000);timers.set(++sequence,fn);return sequence},clearTimeout:id=>timers.delete(id),overviewContainer:()=>({}),connected:()=>true,loadOverview:(...args)=>calls.push(args)});
 f.scheduleHistoryServerRetry(tab);f.scheduleHistoryServerRetry(tab);assert.equal(timers.size,1);timers.values().next().value();assert.deepEqual(calls,[[tab,true,false,false,true]]);
 tab.data.capabilities=[];f.scheduleHistoryServerRetry(tab);assert.equal(tab.historyServerRetryTimer,0);
});
test('R264 P6 fast cards and one match survive a late failure during the first page',async()=>{
 let progress,finish;const seen=[];
 const tab={key:'foreign',region:'kr',matchFilter:'all',data:{player:{playerRef:'fixture',region:'kr'},matches:[],pagination:{filter:'all'}},initialPagePending:true};
 const state={settings:{matchCount:10},controllers:new Map()};const noop=()=>{};
 const f=compile(source,['loadOverview'],{state,tabReady:()=>true,tabGroup:()=> 'kr',riotTab:()=>true,connected:()=>false,clearTimeout:noop,performance,rerenderTab:t=>seen.push({at:performance.now(),matches:t.data?.matches?.length,ranks:t.data?.ranks?.length,masteries:t.data?.masteries?.length}),api:(_url,options)=>{progress=options.onProgress;return new Promise(resolve=>finish=resolve)},rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture',normalizedPagination:p=>({...p.pagination}),MAX_BROWSE_MATCHES:200,AUTO_PAGE_DELAY_MS:1500,computeOverviewStreak:noop,syncOverviewSupplementRefs:noop,renderCapabilitySettings:noop,loadMayhemRating:noop,loadOPGGSeasonSummary:noop,loadOverviewCurrentGame:noop});
 const started=performance.now(),pending=f.loadOverview(tab,true);
 const player={playerRef:'fixture',region:'kr'},pagination={filter:'all',count:10,hasMore:true};
 progress({player,matches:[],ranks:[{tier:'GOLD'}],masteries:[{championId:103}],capabilities:[],pagination},'cards');
 progress({player,matches:[{gameId:264}],capabilities:[],pagination});
 assert(seen.some(row=>row.matches===1 && row.ranks===1 && row.masteries===1 && row.at-started<2000));
 finish({player,matches:[],ranks:[],masteries:[],pagination:{...pagination,partial:true},capabilities:[{name:'match-history',state:'failed',detail:'战绩服务暂时不可用'}]});await pending;
 assert.equal(tab.data.matches[0].gameId,264);assert.equal(tab.data.ranks.length,1);assert.equal(tab.data.masteries.length,1);
});
test('R264 P2 completed identical snapshot redraws cards that the connection placeholder removed',async()=>{
 const items=[{id:1,name:'Fixture',owned:true}],state={skinLoadGeneration:0,items,status:{connected:true,snapshotReady:true},view:'owned',skinsCache:new Map(),staleSnapshot:false,staleSnapshotAt:'',sort:'name',collectionRenderPending:true};let renders=0;
 const f=compile(app,['loadSkins','sameCollectionItems','applySkinsPayload'],{state,el:{retryList:{hidden:false}},window:{},api:async()=>({items,stale:false}),acquisitionTime:()=>null,configureSortControls:()=>{},renderItems:()=>renders++,ensureCollection:()=>{}});
 await f.loadSkins(true);assert.equal(renders,1);assert.equal(state.collectionRenderPending,false);assert.equal(state.loading,false);
});
