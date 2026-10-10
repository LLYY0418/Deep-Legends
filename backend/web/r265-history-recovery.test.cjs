'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(__dirname+'/gameplay.js','utf8');
function clock(){
 let now=0,next=0;const tasks=new Map(),box={module:{exports:{}},setTimeout:(fn,delay)=>{tasks.set(++next,{fn,at:now+delay,delay});return next;},clearTimeout:id=>tasks.delete(id),Date:{now:()=>now}};
 vm.runInNewContext(fs.readFileSync(__dirname+'/history-recovery.js','utf8'),box);
 return {api:box.module.exports,tasks,async advance(ms){now+=ms;for(const [id,t] of [...tasks])if(t.at<=now){tasks.delete(id);await t.fn();}},setNow:n=>now=n};
}
test('R265 detail retries use 5/15/45 seconds, one timer, three rounds and keep failed details manual',async()=>{
 const h=clock(),tab={};let calls=0,pending=true;
 const ctx={pending:()=>pending,active:()=>true,retry:async()=>calls++};
 h.api.schedule(tab,ctx);h.api.schedule(tab,ctx);assert.equal(h.tasks.size,1);assert.equal([...h.tasks.values()][0].delay,5000);
 await h.advance(4999);assert.equal(calls,0);await h.advance(1);assert.equal(calls,1);
 assert.equal([...h.tasks.values()][0].delay,15000);await h.advance(15000);assert.equal(calls,2);
 assert.equal([...h.tasks.values()][0].delay,45000);await h.advance(45000);assert.equal(calls,3);
 assert.equal(h.tasks.size,0);h.api.schedule(tab,ctx);await h.advance(600000);assert.equal(calls,3);
 h.api.reset(tab);pending=false;h.api.schedule(tab,ctx);assert.equal(h.tasks.size,0);
});
test('R265 closing/switching/hidden cancels both scheduled and running requests; quota spends no round',async()=>{
 const h=clock(),tab={};let active=true,quota=20000,calls=0,finish,aborts=0;
 const ctx={pending:()=>true,active:()=>active,notBefore:()=>quota,retry:()=>{calls++;return new Promise(r=>finish=r)},abort:()=>aborts++};
 h.api.schedule(tab,ctx);await h.advance(5000);assert.equal(calls,0);assert.equal(tab.detailRecovery.round,0);
 quota=40000;const blocked=h.advance(15000);await Promise.resolve();assert.equal(calls,0);await blocked;assert.equal(tab.detailRecovery.round,0);assert.equal(h.tasks.size,1);
 const running=h.advance(20000);await Promise.resolve();assert.equal(calls,1);h.api.cancel(tab);assert.equal(aborts,1);finish();await running;assert.equal(h.tasks.size,0);
 h.api.reset(tab);h.api.schedule(tab,ctx);active=false;await h.advance(5000);assert.equal(calls,1);assert.equal(h.tasks.size,0);
});
test('R265 a 429 returned by a detail attempt preserves the full three-round retry budget',async()=>{
 const h=clock(),tab={};let quota=false,calls=0;
 const ctx={pending:()=>!quota,active:()=>true,quotaBlocked:()=>quota,retry:async()=>{calls++;if(calls===1)quota=true;}};
 h.api.schedule(tab,ctx);await h.advance(5000);
 assert.equal(calls,1);assert.equal(tab.detailRecovery.round,0);assert.equal(h.tasks.size,0);
 quota=false;h.api.schedule(tab,ctx);await h.advance(5000);assert.equal(calls,2);assert.equal(tab.detailRecovery.round,1);
 await h.advance(15000);await h.advance(45000);assert.equal(calls,4);assert.equal(tab.detailRecovery.round,3);assert.equal(h.tasks.size,0);
});
test('R265 quota cancellation retains its page cursor and resumes only while the player is visible',async()=>{
 let now=0,next=0,active=true;const jobs=new Map(),requests=[];
 const state={settings:{matchCount:20}},tab={region:'kr',matchFilter:'solo',overviewRequestToken:8};
 const f=compile(source,['scheduleQuotaRetry','cancelHistoryRecovery'],{globalThis:{deepLegendsHistoryRecovery:require('./history-recovery.js')},state,riotTab:()=>true,historyRecoveryActive:()=>active,Date:{now:()=>now},setTimeout:(fn,ms)=>{jobs.set(++next,{fn,ms});return next;},clearTimeout:id=>jobs.delete(id),loadOverview:async(...args)=>{requests.push(args);return false;}});
 tab.quotaRetry={append:true,retryAt:60000,scheduleDelay:60000,requestToken:8,filter:'solo',count:20,timer:0};
 f.scheduleQuotaRetry(tab);assert.equal([...jobs.values()][0].ms,60000);
 active=false;f.cancelHistoryRecovery(tab);assert.equal(jobs.size,0);assert.equal(tab.quotaRetry.append,true);
 f.scheduleQuotaRetry(tab);assert.equal(jobs.size,0);
 active=true;now=61000;f.scheduleQuotaRetry(tab);const job=[...jobs.values()][0];assert.equal(job.ms,0);job.fn();await Promise.resolve();
 assert.equal(requests.length,1);assert.equal(requests[0][1],false);assert.equal(requests[0][2],true);assert.equal(tab.quotaRetry,null);
});
test('R265 a lazy recovery module cannot schedule a retry over an active first stream',async()=>{
 let finish,calls=0;const globals={deepLegendsSections:{load:()=>new Promise(r=>finish=r)}};
 const tab={loading:false},f=compile(source,['scheduleHistoryServerRetry'],{globalThis:globals,document:{hidden:false},riotTab:()=>true,historyRecoveryActive:()=>true,state:{},window:{}});
 f.scheduleHistoryServerRetry(tab);assert.equal(tab.detailRecoveryLoading,true);
 tab.loading=true;globals.deepLegendsHistoryRecovery={schedule:()=>calls++};finish();await Promise.resolve();await Promise.resolve();
 assert.equal(calls,0);
 tab.loading=false;f.scheduleHistoryServerRetry(tab);assert.equal(calls,1);
});
test('R265 a filter click survives a same-player toolbar redraw while its CSS loads',async()=>{
 const {JSDOM}=require('../../desktop/node_modules/jsdom'),af=require('./history-filters.js');
 for(const switched of [false,true]){
  const dom=new JSDOM('<main><button data-af-open>筛选</button></main>');
  try{
   require('./filter-dialog-fixture.cjs').install(dom);const container=dom.window.document.querySelector('main'),tab={advancedConditions:{}};let finish,ready=false,renders=0;
   const ctx={stylesReady:()=>ready,ensureStyles:()=>new Promise(r=>finish=r),render:()=>renders++,container:()=>container};
   af.bind(container,tab,()=>ctx);container.querySelector('button').click();assert.equal(renders,0);
   container.innerHTML='<button data-af-open>筛选</button>';
   if(switched)af.bind(container,{advancedConditions:{}},()=>ctx);
   ready=true;finish();await Promise.resolve();
   assert.equal(renders,switched?0:1);assert.equal(Boolean(tab.advancedMenu?.open),!switched);
  }finally{dom.window.close();}
 }
});
test('R265 recovery runs only in the visible top overview, including player overlays',()=>{
 const state={section:'overview',overlay:[]},document={hidden:false},nodes={playerOverlay:{hidden:false}};
 const f=compile(source,['historyRecoveryActive'],{state,document,nodes,overviewContainer:()=>({})});
 const tab={},lower={overlay:true},top={overlay:true};
 assert.equal(f.historyRecoveryActive(tab),true);state.overlay=[lower,top];assert.equal(f.historyRecoveryActive(tab),false);
 state.section='pro-players';assert.equal(f.historyRecoveryActive(lower),false);assert.equal(f.historyRecoveryActive(top),true);
 document.hidden=true;assert.equal(f.historyRecoveryActive(top),false);document.hidden=false;
 nodes.playerOverlay.hidden=true;assert.equal(f.historyRecoveryActive(top),false);nodes.playerOverlay.hidden=false;
 top.closed=true;assert.equal(f.historyRecoveryActive(top),false);delete top.closed;
 state.destroyed=true;assert.equal(f.historyRecoveryActive(top),false);
});
test('R265 partial sample boundaries and all five dependent cards; complete samples remove labels',()=>{
 const f=compile(source,['historyStatsPending','careerSectionEntries','renderRecentPlayers','renderActivity'],{escapeHTML,number:String,riotRegion:()=>false,renderRanks:()=>'',renderChampionStats:()=>'',renderMasteries:()=>'',renderOverviewShareButton:()=>'',rankedQueueData:()=>({queueId:420,recentRanked:{games:6},ability:{},positions:[],queueGames:6}),rankedQueueSwitcher:()=>'',renderRecentRanked:s=>`<section><h3>近期排位</h3>${s.historyPending?'战绩未读全':'6'}</section>`,renderAbility:a=>`<section><h3>能力表现</h3>${a.historyPending?'战绩未读全':'样本'}</section>`,renderPositionStats:(_a,_b,_c,_d,n)=>`<section><h3>位置偏好</h3>${n<0?'战绩未读全':'位置'}</section>`,state:{settings:{maskNames:true}}});
 for(const [n,k,partial,weak] of [[10,1,true,true],[10,3,true,true],[10,5,true,true],[10,6,true,false],[10,7,true,false],[20,6,true,true],[20,12,true,false],[5,3,true,true],[5,5,false,false],[3,1,true,true],[3,3,false,false],[10,10,false,false]]){
  const data={matches:Array.from({length:k},(_,gameId)=>({gameId})),historyRequested:n,historyLoaded:k,pagination:{count:n,partial},activityHours:Array(24).fill(0)},tab={data};
  assert.equal(f.historyStatsPending(data,tab),weak,`N=${n},k=${k}`);
  const cards=new Map(f.careerSectionEntries(data,tab));
  if(partial){data.pagination.partial=false;data.capabilities=[{name:'match-history',state:'failed'}];assert.deepEqual(new Map(f.careerSectionEntries(data,tab)),cards,'capability-only failure keeps partial titles and labels');}
  for(const key of ['recent-ranked','ability','positions','recent-players','activity']){
   const html=cards.get(key);
   if(weak){assert.match(html,/战绩未读全/);assert.doesNotMatch(html,/暂无重复同场玩家|activity-cell|基于最近/);}
   else if(partial)assert.match(html,new RegExp(`基于最近 ${k} 场`));
   else assert.doesNotMatch(html,/基于最近/);
  }
 }
});
test('R265 empty foreign outage describes no matches while compact stays one line',()=>{
 const f=compile(source,['renderHistoryServiceStatus'],{historyServiceState:()=>({kind:'relay',seconds:0}),serviceOutageIcon:()=>'<svg/>'});
 assert.match(f.renderHistoryServiceStatus({}),/暂时没有读取到对局，恢复后会自动补齐。/);
 assert.doesNotMatch(f.renderHistoryServiceStatus({}),/已显示能读取到的部分/);
 assert.match(f.renderHistoryServiceStatus({},true),/<span>外服战绩连接较慢<\/span>/);
});
test('R265 partial card headers have one sample description and restore complete titles',()=>{
 const f=compile(source,['renderRecentRanked','renderPositionStats','renderRecentPlayers'],{escapeHTML,number:String,percent:n=>`${n||0}%`,kda:String,isMayhemQueueId:()=>false,rankedQueueNoun:()=> '排位',rankedQueueLabel:()=> '单双排',positionLabel:()=> '上单',positionIcon:()=> '',state:{settings:{maskNames:true}},maskedListName:()=> '同场测试',maskedProfileIcon:()=> '',iconFigure:()=> '',proBadgeAttributes:()=> ''});
 const stats={games:7,wins:5,losses:2,winRate:71,positions:[]};
 for(const partial of [true,false]){
  const recent=f.renderRecentRanked(stats,420,'',partial),positions=f.renderPositionStats([{position:'top',games:7,share:100}],420,'','单双排',7,partial),players=f.renderRecentPlayers([{games:7}],{},false,partial);
  if(partial){assert.match(recent,/<h3>近期排位<\/h3>/);assert.doesNotMatch(recent,/近 7 场排位/);assert.doesNotMatch(positions,/近 7 场/);assert.doesNotMatch(players,/最近 30 天/);}
  else{assert.match(recent,/<h3>近 7 场排位<\/h3>/);assert.match(positions,/近 7 场/);assert.match(players,/最近 30 天/);}
 }
});
test('R265 final incremental frame keeps the running marker until completion render finishes',async()=>{
 const h=clock(),tab={};let pending=true,finish;
 const ctx={pending:()=>pending,active:()=>true,retry:()=>new Promise(r=>finish=r)};
 h.api.schedule(tab,ctx);const running=h.advance(5000);await Promise.resolve();assert.equal(tab.detailRecovery.running,true);
 pending=false;h.api.schedule(tab,ctx);assert.equal(tab.detailRecovery.running,true);assert.equal(h.tasks.size,0);
 finish();await running;assert.equal(tab.detailRecovery.running,false);assert.equal(h.tasks.size,0);
});
