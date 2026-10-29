'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,compile,escapeHTML}=require('./r188-harness.cjs');
function fixture(){return compile(read('gameplay.js'),['acceptRankRecords','renderRanks','renderRankWindowNote','refreshSeasonSummary'],{state:{},window:{reportFlowDiagnostic:()=>true},escapeHTML,number:String,percent:v=>v+'%',rankCrestIcon:()=>'',rankTitle:()=>'',renderRankMMRPopover:()=>'',renderRankMilestones:()=>'',riotTab:()=>false,overviewContainer:()=>null,updateCareerCards:()=>{}});}
const rank=(wins=11,losses=18,winRate=38)=>({queueType:'RANKED_SOLO_5x5',tier:'GOLD',officialWins:49,wins,losses,winRate,recordSource:'season',recordPending:false,recordSplitStart:1785351600000,recordSplitSource:'sgp'});
const data=r=>({player:{playerRef:'p',serverId:'HN1'},ranks:[r],seasonStatsProgress:{season:'S26',scanned:51,rankedComplete:true}});
test('R313 estimates render official wins, weak approximate sign and explanation only when estimated',()=>{
 const f=fixture();let html=f.renderRanks([rank()],[],[],null,{});assert.match(html,/11胜 18负/);assert.match(html,/胜率 <b[^>]*>38%/);assert.doesNotMatch(html,/≈|负场按|整个赛季|data-tooltip-accent/);
 html=f.renderRanks([rank(0,0,-1)],[],[],null,{});assert.match(html,/0胜 0负/);assert.match(html,/胜率 —/);assert.doesNotMatch(html,/胜率暂不可用|正在统计中|skeleton/);
});
test('R313 retained and summary records preserve backend flags and never recompute estimated losses',async()=>{
 const f=fixture(),tab={key:'p',playerRef:'p',serverId:'HN1',data:data(rank())};
 for(let i=0;i<11;i++){const response=data({...rank(),officialWins:49+i%2});tab.data=f.acceptRankRecords(tab,response);assert.deepEqual([tab.data.ranks[0].wins,tab.data.ranks[0].losses],[11,18]);assert.equal(tab.data.ranks[0].recordPending,false);}
 await f.refreshSeasonSummary(tab,{...data(rank(12,18,40)),seasonQueueStats:{420:{games:500,wins:300,losses:200,winRate:60}}});assert.deepEqual([tab.data.ranks[0].wins,tab.data.ranks[0].losses],[12,18]);
});
test('R313 estimate tooltip uses a themed text line and never interprets markup',()=>{
 const {JSDOM}=require('../../desktop/node_modules/jsdom'),dom=new JSDOM('<main></main>');try{const d=dom.window.document;d.querySelector('main').innerHTML=fixture().renderRankWindowNote({},[rank()]);const tip=d.querySelector('[data-tooltip]');assert.equal(tip.dataset.tooltip,'数据范围：2026 赛季第三赛段\n统计自 7月30日 起');assert.equal(tip.dataset.tooltip.split('\n').length,2);assert(!tip.hasAttribute('data-tooltip-accent'));
 d.querySelector('main').innerHTML=fixture().renderRankWindowNote({},[{...rank(),recordSplitStart:Date.UTC(2027,0,6,19),recordSplitSource:'sgp'}]);assert.equal(d.querySelector('[data-tooltip]').dataset.tooltip,'数据范围：2027 赛季第一赛段\n统计自 1月7日 起');
 d.querySelector('main').innerHTML=fixture().renderRankWindowNote({},[{...rank(),recordSplitStart:Date.UTC(2027,0,6,19),recordSplitSource:'season'}]);assert.equal(d.querySelector('[data-tooltip]').dataset.tooltip,'统计自 1月7日 起');}finally{dom.window.close();}
});
