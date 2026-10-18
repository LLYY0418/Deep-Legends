'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{compile,escapeHTML}=require('./r188-harness.cjs');
const src=()=>fs.readFileSync(process.env.R286_GAME_SOURCE||__dirname+'/gameplay.js','utf8');
const rank={queueType:'RANKED_SOLO_5x5',tier:'GOLD',wins:7,losses:0,winRate:-1};
test('R286 paused rank is explicit, then ranked completion fills without mayhem and state logs only change',async()=>{
 const reports=[],tab={key:'test',playerRef:'ref',data:{player:{playerRef:'ref'},ranks:[{...rank}],seasonStatsProgress:{season:'S26',complete:false,paused:true,collecting:true}}};
 const f=compile(src(),['renderRanks','refreshSeasonSummary'],{window:{reportFlowDiagnostic:(e,r,v)=>reports.push({e,r,...v})},escapeHTML,number:String,percent:v=>v+'%',rankCrestIcon:()=>'',rankTitle:()=>'',renderRankMMRPopover:()=>'',riotTab:()=>false,renderRankMilestones:()=>'',overviewContainer:()=>null,updateCareerCards(){}});
 const render=()=>f.renderRanks(tab.data.ranks,[],[],[],tab.data.seasonStatsProgress,tab);
 for(let i=0;i<3;i++){const html=render();assert(html.includes('7胜 · 统计已暂停'));assert(!html.includes('正在统计中'));assert(html.includes('胜率暂不可用'));assert(html.includes('官方战绩服务暂时中断，恢复后自动继续统计'));}
 assert.equal(reports.length,1);assert.equal(reports[0].queue,'solo');assert.equal(reports[0].state,'paused');
 await f.refreshSeasonSummary(tab,{seasonStatsProgress:{season:'S26',complete:false,rankedComplete:true,collecting:true},seasonQueueStats:{420:{games:10,wins:7,losses:3,winRate:70}}});
 assert(render().includes('7胜 3负'));assert.equal(reports.length,2);assert.equal(reports[1].state,'known');
});
test('R286 recovery refreshes every paused Tencent tab including hidden tabs without changing history cursor',()=>{
 const tabs=[true,false].map((active,i)=>({key:String(i),nextBegIndex:40,data:{seasonStatsProgress:{paused:true}},active})),calls=[];
 const other={key:'other',data:{seasonStatsProgress:{paused:true}},server:'HN2'};
 const f=compile(src(),['handleHistoryServiceRecovered'],{state:{section:'overview',tabs:[...tabs,other],overlay:[]},riotTab:()=>false,tabServerID:t=>t.server||'HN1',historyRecoveryActive:t=>t.active,loadOverview:(...args)=>{calls.push(args);return Promise.resolve()},historyServiceState:()=>null});
 f.handleHistoryServiceRecovered({serverId:'HN1'});assert.equal(calls.length,2);assert(calls.every(a=>a[1]===true&&a[4]===true));assert.equal(calls[1][7],true);assert(tabs.every(t=>t.nextBegIndex===40));
});
test('R286 paused and ranked-complete zero-count SSE reaches inactive tabs',async()=>{
 const tab={key:'bg',data:{player:{playerRef:'ref'},seasonStatsProgress:{complete:false,paused:false}}},calls=[];
 const f=compile(src(),['markSeasonRefreshPending'],{state:{section:'live',tabs:[tab],overlay:[]},refreshSeasonSummary:async(t,s)=>calls.push([t,s])});
 const snapshot={seasonStatsProgress:{paused:true,rankedComplete:true}};f.markSeasonRefreshPending({account:'ref',complete:false,rankedComplete:true,paused:true,snapshot});assert.equal(calls.length,1);assert.equal(calls[0][1],snapshot);
});
