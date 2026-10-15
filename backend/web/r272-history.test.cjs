'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile}=require('./r188-harness.cjs'),af=require(process.env.R272_FILTER_SOURCE || './history-filters.js');
const source=fs.readFileSync(process.env.R272_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8');
test('intermediate cards stay pending until a complete empty page confirms zero',()=>{
 const {matchesPending}=compile(source,['matchesPending'],{});
 const tab={loading:true,matchesReceived:false,data:{matches:[],pagination:{partial:true}}};assert.equal(matchesPending(tab),true);
 tab.data.pagination={partial:false};assert.equal(matchesPending(tab),true,'an intermediate missing flag must never establish final zero');
 tab.matchesReceived=true;tab.loading=false;assert.equal(matchesPending(tab),false);
});
test('time search consumes new unique pages from loaded cursor, reaches 40/60/80 and exhausts',async()=>{
 for(const foreign of [false,true]){
  const now=Date.parse('2026-10-11T12:00:00Z'),rows=Array.from({length:80},(_,i)=>({gameId:i+1,result:'win',createdAt:now-i*60000,participants:[{participantId:1,championId:103}]}));
  const tab={advancedConditions:{time:{values:['d30']},hero:{values:['999']}},data:{matches:rows.slice(0,20),pagination:{hasMore:true}}},calls=[],diagnostics=[];let cursor=20;
  const ctx={rows:()=>tab.data.matches,version:()=>tab.data.matches,cursor:()=>cursor,now:()=>now,subject:m=>m.participants[0],tags:()=>[],foreign:()=>foreign,isActive:()=>true,render(){},progress(){},diagnostic:(reason,fields)=>diagnostics.push({reason,...fields}),load:async n=>{calls.push({beg:cursor,count:n});const page=rows.slice(cursor,cursor+n);tab.data.matches=tab.data.matches.concat(page);cursor+=page.length;tab.data.pagination.hasMore=cursor<80;return true;}};
  await af.find(tab,ctx);assert.equal(tab.data.matches.length,80,'one search continues across all pages to the actual time boundary');
  assert.deepEqual(calls.map(c=>c.beg),[20,40,60]);assert.equal(tab.advancedSearch.stopReason,'no_more');assert.match(af.empty(tab,ctx),/近\s*30\s*天的\s*80\s*场|已查完可查询的\s*80\s*场/);assert.doesNotMatch(af.empty(tab,ctx),/data-af-continue/);
  assert(diagnostics.some(d=>d.exhausted===true));
 }
});
test('four presets migrate obsolete season without losing hero/result and empty saved state is read only',()=>{
 const presets=[{name:'s',conditions:{time:{values:['season']},hero:{values:['103']}}},{name:'s+d7',conditions:{time:{values:['season','d7']},result:{values:['win']}}},{name:'days',conditions:{time:{values:['d3']}}},{name:'hero',conditions:{hero:{values:['86']}}}];
 const storage={getItem:()=>JSON.stringify(presets)};const migrated=af.readPresets(storage);assert.equal(migrated.length,4);assert.equal(migrated[0].conditions.time,undefined);assert.deepEqual(migrated[0].conditions.hero.values,['103']);assert.deepEqual(migrated[1].conditions.time.values,['d7']);assert.deepEqual(migrated[1].conditions.result.values,['win']);
});
test('actual advanced context cursor never falls back to obsolete time cursor',()=>{
 const noop=()=>{},tab={nextBegIndex:20,historyTimeCursor:0,data:{pagination:{begIndex:0,count:20}},advancedConditions:{time:{values:['d30']}}};
 const deps={globalThis:{deepLegendsHistoryFilters:af},state:{settings:{}},matchDataTags:noop,ensureHistoryFilterStyles:noop,positionIcon:noop,renderMultiKillTag:noop,prepareImages:noop,showToast:noop};
 const {advancedFilterContext}=compile(source,['advancedFilterContext'],deps);assert.equal(advancedFilterContext(tab).cursor(),20);tab.nextBegIndex=40;assert.equal(advancedFilterContext(tab).cursor(),40);
});

test('arrived matches keep aggregate cards pending until their own real statistics arrive',()=>{
 const plain=()=>'<section><h3>统计</h3><strong>战绩未读全</strong></section>',data={player:{region:'cn'},matches:[{gameId:1}],historyRequested:0,pagination:{partial:true},ranks:[{}],masteries:[{}],capabilities:[]},tab={loading:true,matchesReceived:true,initialPagePending:true,recentStatsReceived:false,data};
 const deps={historyStatsPending:()=>true,rankedQueueData:()=>({queueId:420,positions:[]}),riotRegion:()=>false,rankedQueueSwitcher:()=>'',renderRanks:plain,renderRecentRanked:plain,renderAbility:plain,renderChampionStats:()=>'<section class="career-pending"></section>',championStatsPending:()=>true,renderMasteries:plain,renderPositionStats:plain,renderRecentPlayers:plain,renderActivity:plain,renderOverviewShareButton:()=>'',number:String};
 const f=compile(source,['careerSectionEntries','matchesPending'],deps),keys=['recent-ranked','ability','positions','recent-players','activity'];
 for(const [key,html] of f.careerSectionEntries(data,tab))if(keys.includes(key)){assert.match(html,/career-pending/,key);assert.doesNotMatch(html,/战绩未读全/,key);}
 tab.recentStatsReceived=true;data.historyRequested=20;deps.historyStatsPending=()=>false;const ready=compile(source,['careerSectionEntries','matchesPending'],deps);for(const [key,html] of ready.careerSectionEntries(data,tab))if(keys.includes(key))assert.doesNotMatch(html,/career-pending/,key);
});
