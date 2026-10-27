'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R309_RANK_GAMEPLAY_SOURCE||__dirname+'/gameplay.js','utf8');
const queues=[['RANKED_SOLO_5x5',162,168,49],['RANKED_FLEX_SR',12,21,36]];
const response=filled=>({player:{playerRef:'opaque-player',region:'cn',serverId:'HN1',gameName:'Fixture',isCurrent:false},ranks:queues.map(([queueType,wins,losses,winRate])=>({queueType,tier:'GOLD',wins,losses:filled?losses:0,winRate:filled?winRate:-1,...(filled?{recordSource:'season'}:{})})),seasonStatsProgress:{season:'S26',scanned:363,rankedComplete:true,complete:true},matches:[],pagination:{begIndex:0,count:20,hasMore:false}});
function fixture(payloads,overrides={}){
 const state={controllers:new Map(),settings:{matchCount:20},tabs:[],overlay:[]},reports=[];
 const noop=()=>{};
 const deps={state,window:{reportFlowDiagnostic:(event,reason,fields)=>{reports.push({event,reason,...fields});return true;}},MAX_BROWSE_MATCHES:1000,AUTO_PAGE_DELAY_MS:400,AUTO_PAGE_MAX_BACKOFF_MS:8000,
 connected:()=>true,tabReady:()=>true,riotTab:()=>false,tabGroup:()=> 'cn',rerenderTab:noop,appendOverviewMatches:noop,showLoadingMoreState:noop,showToast:noop,loadOPGGSeasonSummary:noop,loadOverviewCurrentGame:noop,loadMayhemRating:noop,syncOverviewSupplementRefs:noop,rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture',renderCapabilitySettings:noop,computeOverviewStreak:()=>null,reportDeferredMatchesCard:noop,finishOverviewLoad:noop,
 normalizedPagination:(p,beg)=>({...p.pagination,nextBegIndex:beg+20}),api:async()=>{assert(payloads.length,'unexpected request');return structuredClone(payloads.shift());},
 escapeHTML,number:String,percent:v=>v+'%',rankCrestIcon:()=>'',rankTitle:()=>'',renderRankMMRPopover:()=>'',renderRankMilestones:()=>'',overviewContainer:()=>null,updateCareerCards:noop};
 Object.assign(deps,overrides);
 const f=compile(source,['loadOverview','renderRanks','refreshSeasonSummary'],deps);
 return {...f,state,reports};
}
test('R309 P11 completed season rank record survives initial fill and six wins-only refreshes',async()=>{
 const f=fixture([response(true),...Array.from({length:6},()=>response(false))]),tab={key:'friend',playerRef:'opaque-player',region:'cn',serverId:'HN1'};f.state.tabs=[tab];
 for(let i=0;i<7;i++){
  assert.equal(await f.loadOverview(tab,true,false,true),true,'overview '+i);
  const html=f.renderRanks(tab.data.ranks,[],[],[],tab.data.seasonStatsProgress,tab);
  assert.match(html,/162胜 168负/,'solo refresh '+i);assert.match(html,/12胜 21负/,'flex refresh '+i);assert.doesNotMatch(html,/负场未提供/);
 }
});

test('R309 P11 retained records are isolated by player/season; changed wins await a fresh aggregate',()=>{
 const f=fixture([]),tab={key:'one',playerRef:'opaque-player',serverId:'HN1',data:response(true)};
 tab.data=f.acceptRankRecords(tab,tab.data,'season-fill');
 const same=f.acceptRankRecords(tab,response(false),'refresh');assert.equal(same.ranks[0].losses,168);assert.equal(f.reports[0].event,'rank_record_downgrade_blocked');
 const reopened={key:'reopened',playerRef:'opaque-player',serverId:'HN1'};assert.equal(f.acceptRankRecords(reopened,response(false),'cache').ranks[0].losses,168);
 const changed=response(false);changed.ranks[0].wins++;const next=f.acceptRankRecords(tab,changed,'refresh');assert.equal(next.ranks[0].winRate,-1);assert.equal(next.ranks[0].recordPending,true);
 const season=response(false);season.seasonStatsProgress.season='S27';assert.equal(f.acceptRankRecords(tab,season).ranks[1].winRate,-1);
 const player=response(false);player.player.playerRef='another';assert.equal(f.acceptRankRecords(tab,player).ranks[1].winRate,-1);
 const server=response(false);server.player.serverId='HN2';assert.equal(f.acceptRankRecords(tab,server).ranks[1].winRate,-1,'another server cannot inherit the previous response');
 const upstream=response(true);upstream.ranks.forEach(r=>r.recordSource='upstream');tab.data=f.acceptRankRecords(tab,upstream,'refresh');assert.equal(tab.rankRecordCause,'refresh');assert.equal(f.acceptRankRecords({serverId:'HN1'},response(false)).ranks[1].winRate,-1);
});
test('R309 P11 season summary ignores an in-flight response after switching server',async()=>{
 let release;const f=fixture([],{api:()=>new Promise(resolve=>{release=resolve})}),tab={key:'friend',playerRef:'opaque-player',serverId:'HN1',data:response(false)};
 const pending=f.refreshSeasonSummary(tab);tab.data.player.serverId='HN2';
 release({seasonStatsProgress:{season:'S26',rankedComplete:true},seasonQueueStats:{420:{games:330,wins:162,losses:168,winRate:49}}});
 assert.equal(await pending,false);assert.equal(tab.data.ranks[0].winRate,-1);
});
test('R309 P11 summary fallback deduplicates each player/season/scanned and retries failures only twice at 3 seconds',async()=>{
 let calls=0,seq=0;const timers=new Map(),diagnostics=[];
 const f=fixture([],{api:async()=>{calls++;throw Error('offline')},setTimeout:(fn,ms)=>{timers.set(++seq,{fn,ms});return seq},clearTimeout:id=>timers.delete(id),window:{reportFlowDiagnostic:(event,reason,fields)=>{diagnostics.push({event,reason,...fields});return true}}});
 const tab={key:'one',playerRef:'opaque-player',serverId:'HN1',data:response(false)};
 const flush=()=>new Promise(setImmediate);
 for(let i=0;i<6;i++)f.ensureRankRecordFill(tab);await flush();assert.equal(calls,1);assert.equal(timers.size,1);
 for(let i=0;i<2;i++){const [id,job]=timers.entries().next().value;assert.equal(job.ms,3000);timers.delete(id);job.fn();await flush();}
 assert.equal(calls,3);assert.equal(timers.size,0);f.ensureRankRecordFill(tab);await flush();assert.equal(calls,3);assert.deepEqual(diagnostics.filter(d=>d.event==='rank_record_fill_retry').map(d=>[d.attempt,d.result]),[[1,'error'],[2,'error'],[3,'error']]);
 tab.data.seasonStatsProgress.scanned++;f.ensureRankRecordFill(tab);await flush();assert.equal(calls,4);
 tab.closed=true;const job=timers.values().next().value;timers.clear();job.fn();await flush();assert.equal(calls,4,'closed tabs do not retry');
});
test('R309 P11 summary fallback fills once without needing a completion transition',async()=>{
 let calls=0;const f=fixture([],{api:async()=>{calls++;return {seasonStatsProgress:{season:'S26',scanned:363,rankedComplete:true},seasonQueueStats:{420:{games:330,wins:162,losses:168,winRate:49},440:{games:33,wins:12,losses:21,winRate:36}}}}});
 const tab={key:'one',playerRef:'opaque-player',serverId:'HN1',data:response(false)};f.ensureRankRecordFill(tab);await new Promise(setImmediate);for(let i=0;i<6;i++)f.ensureRankRecordFill(tab);assert.equal(calls,1);assert.deepEqual(tab.data.ranks.map(r=>[r.wins,r.losses,r.recordSource]),[[162,168,'season'],[12,21,'season']]);
});
test('R309 P11 pending record skeleton leaves tier and LP visible, expires at eight seconds, and genuine empty/current records stay explicit',t=>{
 t.mock.timers.enable({apis:['Date']});let jobs=0;
 const f=fixture([],{setTimeout:()=>++jobs,clearTimeout:()=>{}}),tab={key:'one',playerRef:'opaque-player',data:response(false)};tab.data.ranks.forEach(r=>{r.recordPending=true;r.leaguePoints=44});
 const render=()=>f.renderRanks(tab.data.ranks,[],[],[],tab.data.seasonStatsProgress,tab);
 const first=render();assert.match(first,/rank-record gameplay-skeleton/);assert.match(first,/44 LP/);assert.doesNotMatch(first,/负场未提供|胜率暂不可用/);assert.equal(jobs,1);
 t.mock.timers.tick(7999);assert.match(render(),/rank-record gameplay-skeleton/);t.mock.timers.tick(1);const expired=render();assert.match(expired,/162胜 · 正在统计中/);assert.doesNotMatch(expired,/负场未提供/);
 tab.data.ranks.forEach(r=>r.recordPending=false);assert.match(render(),/负场未提供/);
 tab.current=true;tab.data=response(true);tab.data.player.isCurrent=true;tab.data.ranks.forEach(r=>r.recordSource='upstream');const own=render();assert.match(own,/162胜 168负/);assert.doesNotMatch(own,/skeleton|正在统计中/);
});
test('R309 P11 failed diagnostic delivery does not suppress the final rank transition',()=>{
 let accepted=false;const reports=[];const f=fixture([],{window:{reportFlowDiagnostic:(event,reason,fields)=>{reports.push({event,reason,...fields});return accepted}}}),tab={key:'one',data:response(true)};
 const render=()=>f.renderRanks(tab.data.ranks,[],[],[],tab.data.seasonStatsProgress,tab);render();render();assert.equal(reports.length,4);accepted=true;render();render();assert.equal(reports.length,6);assert(reports.every(r=>r.state==='known'&&r.source==='season'&&r.ranked_complete&&r.foreign));assert(reports.every(r=>Number.isInteger(r.render_seq)));
});
test('R309 P11 season summary cannot replace verified upstream records or cross a season response',async()=>{
 const f=fixture([]),tab={key:'own',playerRef:'opaque-player',data:response(true)};tab.data.ranks.forEach(r=>r.recordSource='upstream');tab.seasonRanksFilled=new Set(['420','440']);
 await f.refreshSeasonSummary(tab,{seasonStatsProgress:{season:'S26',rankedComplete:true},seasonQueueStats:{420:{games:1,wins:1,losses:0,winRate:100}}});assert.equal(tab.data.ranks[0].wins,162);assert.equal(tab.data.ranks[0].losses,168);assert.equal(tab.data.ranks[0].recordSource,'upstream');
 assert.equal(await f.refreshSeasonSummary(tab,{seasonStatsProgress:{season:'S25',rankedComplete:true},seasonQueueStats:{420:{games:1,wins:1,losses:0,winRate:100}}}),false);assert.equal(tab.data.seasonStatsProgress.season,'S26');
});

test('R312 truncated season keeps aggregate 40/38 across eleven official 49-win loads and summary fills',async()=>{
 const truncated=(filled,official=49,wins=40)=>{const data=response(filled);data.ranks[0]={...data.ranks[0],wins:filled?wins:official,officialWins:official,wins:filled?official:official,losses:filled?(wins===40?47:46):0,winRate:filled?51:-1,recordEstimated:filled,recordSampleWins:wins,recordSampleLosses:38,recordSampleGames:wins+38,recordPending:!filled};return data;};
 const f=fixture([]),tab={key:'truncated',playerRef:'opaque-player',serverId:'HN1'};
 tab.data=f.acceptRankRecords(tab,truncated(true),'initial');
 for(let i=0;i<11;i++){
  tab.data=f.acceptRankRecords(tab,truncated(false),'refresh');
  assert.equal(tab.data.ranks[0].recordPending,false,'load '+i);
  assert.match(f.renderRanks(tab.data.ranks,[],[],[],tab.data.seasonStatsProgress,tab),/49胜 <span class="rank-record-estimate">≈<\/span>47负/,'load '+i);
 }
 const reopened={key:'reopened',playerRef:'opaque-player',serverId:'HN1'};
 assert.equal(f.acceptRankRecords(reopened,truncated(false),'cache').ranks[0].losses,47);
 const pending=truncated(false,50);tab.data=f.acceptRankRecords(tab,pending,'refresh');assert.equal(tab.data.ranks[0].recordPending,true);
 await f.refreshSeasonSummary(tab,{seasonStatsProgress:pending.seasonStatsProgress,ranks:truncated(true,50,41).ranks,seasonQueueStats:{420:{games:79,wins:41,losses:38,winRate:52}}});
 assert.match(f.renderRanks(tab.data.ranks,[],[],[],tab.data.seasonStatsProgress,tab),/50胜 <span class="rank-record-estimate">≈<\/span>46负/);
 assert.equal(tab.data.ranks[0].officialWins,50);assert.equal(tab.data.ranks[0].recordPending,false);
 tab.data=f.acceptRankRecords(tab,truncated(true,50,41),'refresh');assert.equal(tab.data.ranks[0].officialWins,50);
});

test('R312 collecting rank record has an empty secondary line',()=>{
 const f=fixture([]),tab={key:'collecting',data:response(false),rankPendingAt:Date.now()-9000};tab.data.ranks.forEach(r=>r.recordPending=true);
 const html=f.renderRanks(tab.data.ranks,[],[],[],tab.data.seasonStatsProgress,tab);
 assert.equal((html.match(/正在统计中/g)||[]).length,2,'one label per queue, no duplicate secondary/tooltip');
 assert.match(html,/<b>162胜 · 正在统计中<\/b><span><\/span>/);
});

test('R312 stale summary cannot downgrade a known aggregate or fill changed official wins from an old snapshot',async()=>{
 const f=fixture([]),tab={key:'stale',playerRef:'opaque-player',serverId:'HN1'},known=response(true);known.ranks[0]={...known.ranks[0],officialWins:50,wins:50,losses:46,winRate:52,recordEstimated:true,recordSampleWins:41,recordSampleLosses:38,recordSampleGames:79};tab.data=f.acceptRankRecords(tab,known);
 const summary=wins=>({seasonStatsProgress:known.seasonStatsProgress,seasonQueueStats:{420:{games:wins+38,wins,losses:38,winRate:51}}});await f.refreshSeasonSummary(tab,summary(40));assert.equal(tab.data.ranks[0].wins,50);assert.equal(tab.data.ranks[0].losses,46);
 const changed=response(false);changed.ranks[0].wins=51;changed.ranks[0].officialWins=51;tab.data=f.acceptRankRecords(tab,changed,'refresh');await f.refreshSeasonSummary(tab,summary(41));assert.equal(tab.data.ranks[0].recordPending,true);assert.equal(tab.data.ranks[0].winRate,-1);
 await f.refreshSeasonSummary(tab,{...summary(42),ranks:[{...known.ranks[0],officialWins:51,wins:51,losses:46,winRate:53,recordSampleWins:42,recordSampleGames:80,recordPending:false}]});assert.equal(tab.data.ranks[0].wins,51);assert.equal(tab.data.ranks[0].losses,46);assert.equal(tab.data.ranks[0].officialWins,51);assert.equal(tab.data.ranks[0].recordPending,false);
});
