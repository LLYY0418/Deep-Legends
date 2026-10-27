'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,compile,escapeHTML}=require('./r188-harness.cjs');
const source=read('gameplay.js');
function fixture(){const state={};return compile(source,['acceptRankRecords','renderRanks','renderRankWindowNote','refreshSeasonSummary'],{state,window:{reportFlowDiagnostic:()=>true},escapeHTML,number:String,percent:v=>v+'%',rankCrestIcon:()=>'',rankTitle:()=>'',renderRankMMRPopover:()=>'',renderRankMilestones:()=>'',riotTab:()=>false,overviewContainer:()=>null,updateCareerCards:()=>{}});}
const rank=(wins=49,losses=76,winRate=39,unknown=false)=>({queueType:'RANKED_SOLO_5x5',tier:'GOLD',officialWins:wins,wins,losses,winRate,recordSource:'season',recordPending:false,recordEstimated:true,recordLossUnknown:unknown,recordSampleGames:51,recordSampleWins:20,recordSampleLosses:31});
const data=r=>({player:{playerRef:'p',serverId:'HN1'},ranks:[r],seasonStatsProgress:{season:'S26',scanned:51,rankedComplete:true}});
test('R313 estimates render official wins, weak approximate sign and explanation only when estimated',()=>{
 const f=fixture(),r=rank();let html=f.renderRanks([r],[],[],null,{});assert.match(html,/49胜 <span class="rank-record-estimate">≈<\/span>76负/);assert.match(html,/胜率 <b[^>]*>39%/);assert.match(html,/负场与胜率按已读到的对局比例推算/);
 html=f.renderRanks([{...r,recordEstimated:false}],[],[],null,{});assert.doesNotMatch(html,/≈|负场与胜率按已读到/);
 html=f.renderRanks([rank(49,0,-1,true)],[],[],null,{});assert.match(html,/49胜 —负/);assert.match(html,/胜率 —/);assert.doesNotMatch(html,/胜率暂不可用|正在统计中|skeleton/);
});
test('R313 retained and summary records preserve backend flags and never recompute estimated losses',async()=>{
 const f=fixture(),tab={key:'p',playerRef:'p',serverId:'HN1'};tab.data=f.acceptRankRecords(tab,data(rank()));
 for(let i=0;i<10;i++){tab.data=f.acceptRankRecords(tab,data({...rank(),wins:49,losses:0,winRate:-1,recordEstimated:false,recordLossUnknown:false,recordSource:'none',recordPending:true}));assert.equal(tab.data.ranks[0].recordEstimated,true);assert.equal(tab.data.ranks[0].losses,76);}
 tab.data=f.acceptRankRecords(tab,data({...rank(50,0,-1),recordEstimated:false,recordPending:true,recordSource:"none"}));
 await f.refreshSeasonSummary(tab,{...data(rank(50,76,40)),seasonQueueStats:{420:{games:51,wins:20,losses:31,winRate:39}}});assert.deepEqual([tab.data.ranks[0].wins,tab.data.ranks[0].losses,tab.data.ranks[0].recordEstimated],[50,76,true]);
 await f.refreshSeasonSummary(tab,{...data({...rank(50,74,40),recordSampleGames:52,recordSampleWins:21}),seasonQueueStats:{420:{games:52,wins:21,losses:31,winRate:40}}});assert.equal(tab.data.ranks[0].losses,74);
});

test('R313 estimate tooltip uses a themed text line and never interprets markup',()=>{
 const {JSDOM}=require('../../desktop/node_modules/jsdom'),{extract}=require('./r188-harness.cjs');
 const dom=new JSDOM('<main></main>',{url:'http://localhost',pretendToBeVisual:true,runScripts:'outside-only'}),w=dom.window;w.matchMedia=()=>({matches:false});try{w.eval(extract(read('app.js'),'setupFloatingTooltips')+';setupFloatingTooltips();');w.document.querySelector('main').innerHTML=fixture().renderRankWindowNote({},[rank()]);const button=w.document.querySelector('[data-tooltip]');button.dispatchEvent(new w.Event('pointerover',{bubbles:true}));const line=w.document.querySelector('.tooltip-accent');assert.equal(line.textContent,'负场与胜率按已读到的对局比例推算');assert.equal(line.children.length,0);}finally{w.close();}
});
