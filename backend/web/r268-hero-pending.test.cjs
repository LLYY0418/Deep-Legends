'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R268_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8');
const functions=compile(source,['historyStatsPending','championStatsPending','renderChampionStats'],{
 historyServiceState:tab=>tab.service || null,matchesPending:tab=>Boolean(tab.loading && !tab.matchesReceived),
 escapeHTML,number:value=>String(value ?? '—'),percent:value=>String(value ?? '—'),kda:value=>String(value ?? '—'),
 overviewDetailArrow:()=>'<button>详情</button>',iconFigure:()=>'<i></i>'
});
const waiting={season:'2026',collecting:true,complete:false,scanned:0};
function render(items,overall,progress,tab={loading:true}) {
 return functions.renderChampionStats(items,overall,progress,functions.championStatsPending(items,overall,progress,tab));
}
test('R268 empty collecting season and missing recent history show hero skeleton without zero summary',()=>{
 assert.equal(functions.championStatsPending([],{},waiting,{loading:false,matchesReceived:true}),true,'season collection can remain pending after recent history arrives');
 for(const progress of [waiting,undefined,{unavailable:true}]){
  const html=render([],{},progress);
  assert.equal((html.match(/<span><\/span>/g)||[]).length,3);
  assert.match(html,/career-pending gameplay-skeleton/);
  assert.doesNotMatch(html,/已统计 0 场|暂无英雄统计|—:1 KDA|全部英雄/);
 }
});
test('R268 completed empty season and arrived empty recent history show an actual empty result',()=>{
 assert.equal(functions.championStatsPending([],{games:0},{...waiting,complete:true,collecting:false},{loading:true}),false,'completed season is independent of recent-history loading');
 for(const progress of [{...waiting,complete:true,collecting:false},undefined]){
  const html=render([],{games:0},progress,{loading:false,matchesReceived:true});
  assert.match(html,/暂无英雄统计/);assert.doesNotMatch(html,/career-pending/);
 }
});
test('R268 real or cached season rows remain visible while history or season backfill is pending',()=>{
 const items=[{championName:'阿狸',games:20,kda:3.2}];
 for(const progress of [{...waiting,scanned:20},undefined]){
  const html=render(items,{games:20,kda:3.2},progress);
  assert.match(html,/阿狸/);assert.match(html,/3.2:1 KDA/);assert.doesNotMatch(html,/career-pending|暂无英雄统计/);
 }
 assert.equal(functions.championStatsPending([],{}, {...waiting,scanned:1},{loading:true}),false);
});
test('R268 foreign season unavailable messages and official/relay interruption states retain their behavior',()=>{
 const foreign={seasonOnly:true,foreign:true,unavailable:true,message:'本赛季英雄统计暂不可用，请刷新重试'};
 assert.equal(functions.championStatsPending([],{},foreign,{loading:true}),false);
 assert.match(render([],{},foreign),/本赛季英雄统计暂不可用/);assert.doesNotMatch(render([],{},foreign),/career-pending|暂无英雄统计/);
 for(const service of [{kind:'official'},{kind:'relay'}])assert.equal(functions.championStatsPending([],{},waiting,{loading:true,service}),false);
});
test('R268 raw recent matches arriving before aggregates keep all dependent statistics pending',()=>{
 const data={matches:Array.from({length:20},()=>({})),historyRequested:0,overall:{games:0},championStats:[]};
 const tab={loading:true,matchesReceived:true,data};
 assert.equal(functions.championStatsPending([],data.overall,undefined,tab),true);
 assert.equal(functions.historyStatsPending(data,tab),true);
 assert.match(render([],data.overall,undefined,tab),/career-pending gameplay-skeleton/);
 assert.equal(functions.championStatsPending([],{games:0},undefined,{...tab,data:{...data,historyRequested:20}}),false,'an arrived zero aggregate is confirmed even before loading finishes');
 assert.equal(functions.historyStatsPending({...data,historyRequested:20,historyLoaded:20},tab),false);
});
test('R268 confirmed recent aggregates, including zero games, remain visible during refresh',()=>{
 const tab={loading:true,matchesReceived:true,recentStatsReceived:true,data:{historyRequested:0,matches:[]}};
 assert.equal(functions.championStatsPending([],{games:0},undefined,tab),false);
 assert.match(render([],{games:0},undefined,tab),/暂无英雄统计/);
 assert.doesNotMatch(render([],{games:0},undefined,tab),/career-pending/);
 const data={historyRequested:0,matches:Array.from({length:20},()=>({}))};
 assert.equal(functions.historyStatsPending(data,tab),false,'confirmed aggregates retain the original sufficient-sample behavior');
});
