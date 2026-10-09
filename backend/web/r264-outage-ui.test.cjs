'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile,escapeHTML}=require('./r188-harness.cjs'),source=fs.readFileSync(__dirname+'/gameplay.js','utf8');
function helpers(){return compile(source,['historyServiceState','renderHistoryServiceStatus','historyStatsPending'],{riotTab:t=>t.region==='kr',escapeHTML,Date});}
test('R264 P5.1 A B C are distinct, official content is safe, and cached matches use compact status',()=>{
 const f=helpers(),a={data:{matches:[],capabilities:[{name:'match-history',historyStatus:'official',retryAfter:60}]}};
 assert.equal(f.historyServiceState(a).kind,'official');const full=f.renderHistoryServiceStatus(a);assert.match(full,/service-outage-icon/);assert.match(full,/role="status"/);assert.match(full,/战绩服务暂时中断/);assert.match(full,/将在 60 秒后自动重试/);assert.doesNotMatch(full,/HTTP|SGP|LCU|网关/);
 a.data.matches=[{gameId:1}];assert.match(f.renderHistoryServiceStatus(a,true),/以下为已保存的对局/);assert.doesNotMatch(f.renderHistoryServiceStatus(a,true),/英雄联盟官方/);
 const b={region:'kr',loading:true,historyWaitStarted:Date.now()-3100,data:{matches:[]}};assert.equal(f.historyServiceState(b).kind,'relay');assert.match(f.renderHistoryServiceStatus(b),/外服战绩连接较慢/);
 const c={data:{capabilities:[{name:'match-history',detail:'单局解析失败',state:'failed'}]}};assert.equal(f.historyServiceState(c),null);assert.equal(f.renderHistoryServiceStatus(c),'');
});
test('R264 P6 one of ten matches keeps all three incomplete statistics cards away from zero and absence',()=>{
 const f=helpers(),tab={data:{matches:[{gameId:1,queueId:420}],pagination:{partial:true,count:10}},initialPagePending:false};assert(f.historyStatsPending(tab.data,tab));
 const h=compile(source,['careerSectionEntries','rankedQueueData','rankedQueueSwitcher','rankedQueueLabel','rankedQueueNoun','isMayhemQueueId','renderAbility','renderRecentRanked','renderPositionStats'],{escapeHTML,number:String,riotRegion:()=>false,renderRanks:()=>'',renderChampionStats:()=>'',renderMasteries:()=>'',renderRecentPlayers:()=>'',renderActivity:()=>'',renderOverviewShareButton:()=>''});
 const sections=new Map(h.careerSectionEntries(tab.data,tab));for(const key of ['recent-ranked','ability','positions']){assert.match(sections.get(key),/战绩未读全/);assert.doesNotMatch(sections.get(key),/近 0 场|未发现|暂无可统计|0%/);}
 tab.data.pagination.partial=false;assert.equal(f.historyStatsPending(tab.data,tab),false);
});
test('R264 P6 ready avatar survives sparse cards and a late history error',async()=>{
 let progress,reject;const tab={key:'profile',region:'kr',playerRef:'fixture',matchFilter:'all'},state={settings:{matchCount:10},controllers:new Map()},noop=()=>{};
 const f=compile(source,['loadOverview'],{state,tabReady:()=>true,tabGroup:()=> 'kr',riotTab:()=>true,connected:()=>false,clearTimeout:noop,performance,rerenderTab:noop,api:(_url,options)=>{progress=options.onProgress;return new Promise((_,fail)=>reject=fail)},rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture'});
 const pending=f.loadOverview(tab,true),player={playerRef:'fixture',region:'kr',profileIconId:71,summonerLevel:30};
 progress({player,matches:[],profilePending:false,pagination:{count:10,hasMore:true}},'cards');
 progress({player:{...player,profileIconId:0,summonerLevel:0},matches:[],profilePending:true,ranks:[{tier:'GOLD'}],masteries:[{championId:103}],pagination:{count:10,hasMore:true}},'cards');
 assert.equal(tab.data.player.profileIconId,71);assert.equal(tab.data.player.summonerLevel,30);reject(new Error('战绩服务暂时不可用'));await pending;
 assert.equal(tab.data.player.profileIconId,71);assert.equal(tab.data.player.summonerLevel,30);assert.equal(tab.data.ranks.length,1);assert.equal(tab.data.masteries.length,1);assert(helpers().historyStatsPending(tab.data,tab));
});
test('R264 early profile header does not turn unread summoner level into zero',()=>{
 const f=compile(source,['renderSelfIdentityHeader'],{assetIcon:()=>'',assetPath:()=>'',number:String,escapeHTML,tabServerLabel:()=>''});
 for(const level of [undefined,0])assert.doesNotMatch(f.renderSelfIdentityHeader({current:true,identity:{summonerLevel:level},label:'Fixture'}),/召唤师等级 (?:0|undefined)/);
 assert.match(f.renderSelfIdentityHeader({current:true,identity:{summonerLevel:30},label:'Fixture'}),/召唤师等级 30/);
});
