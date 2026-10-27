'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,compile,escapeHTML}=require('./r188-harness.cjs');
const source=read('gameplay.js');
const markup=title=>`<section><header><h3>${title}</h3></header></section>`;
function fixture(){return compile(source,['careerSectionEntries','renderRecentRanked'],{state:{},escapeHTML,number:String,percent:v=>v+'%',kda:()=>'1',riotRegion:()=>false,historyServiceState:()=>null,historyStatsPending:()=>false,matchesPending:()=>false,casualOverview:()=>null,renderRanks:()=>'',renderChampionStats:()=>'',renderMasteries:()=>'',renderOverviewShareButton:()=>'',rankedQueueData:(_d,_t,scope)=>({queueId:420,recentRanked:{games:20},ability:{},abilitySampleGames:17,positions:[{games:8},{games:7}],queueGames:20}),rankedQueueSwitcher:()=>'',renderAbility:()=>markup('能力表现'),renderPositionStats:()=>markup('位置偏好'),renderRecentPlayers:()=>markup('最近一起玩'),renderActivity:()=>markup('游戏时间分布')});}
test('R313 recent count is a direct sibling immediately after heading even for zero games',()=>{
 const f=fixture();for(const games of [0,20]){const html=f.renderRecentRanked({games},420,'<button>单双排</button>');assert.match(html,/<h3>近期排位<\/h3><small class="history-sample-label">近 (0|20) 场<\/small><div class="career-section-tools"><button>/);}
});
test('R313 complete refreshes never add sample labels and partial cards use individual sample counts',()=>{
 const f=fixture(),data={player:{region:'cn'},matches:[{},{}],historyLoaded:999,historyRequested:20,pagination:{partial:false},recentPlayersSampleGames:12,activityHours:[3,6]},tab={data};const cards=()=>new Map(f.careerSectionEntries(data,tab));
 for(let i=0;i<10;i++){tab.loading=false;for(const html of cards().values())assert.doesNotMatch(html,/基于最近/);tab.loading=true;tab.initialPagePending=true;for(const html of cards().values())assert.doesNotMatch(html,/基于最近/);}
 tab.loading=false;data.pagination.partial=true;const counts={ability:17,positions:15,'recent-players':12,activity:9};for(const [key,n]of Object.entries(counts))assert.match(cards().get(key),new RegExp('基于最近 '+n+' 场'));
 tab.loading=true;data.recentPlayersSampleGames=100;assert.match(cards().get('recent-players'),/基于最近 12 场/);tab.loading=false;assert.match(cards().get('recent-players'),/基于最近 100 场/);
});
