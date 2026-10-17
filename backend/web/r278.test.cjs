'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R278_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8'),app=fs.readFileSync(process.env.R278_APP_SOURCE || __dirname+'/app.js','utf8');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const af=require(process.env.R278_FILTER_MODULE || './history-filters.js');
test('R278 P1 login overrides recent manual group choice; ordinary refresh preserves choice',()=>{
 const current={key:'self',current:true,region:'',serverId:'HN10',identity:{playerRef:'self'},data:{}},state={tabs:[current],overlay:[],activeTabs:{players:'self'},activeGroup:'kr',section:'settings',status:{clientView:{state:'no-client'}},lastManualTabAt:Date.now()};
 const noop=()=>{},f=compile(source,['applyClientViewStatus'],{state,connected:()=>state.status.clientView.state==='ready',selfTabReady:(s=state.status)=>s.clientView.state==='ready',clientRegion:()=>'',summonerLabel:()=> '本人',localStorage:{setItem:noop},renderPlayerTabs:noop,scheduleBeaconPoll:noop,resetPlayerHistoryConditions:noop,activeTab:()=>current,window:{reportFlowDiagnostic:noop,dispatchEvent:noop},CustomEvent:class{}});
 f.applyClientViewStatus({clientView:{state:'ready'},clientRegion:'TENCENT',serverId:'HN10',summoner:{playerRef:'self'}});assert.equal(state.activeGroup,'players');state.activeGroup='kr';f.applyClientViewStatus(state.status);assert.equal(state.activeGroup,'kr');
});
test('R278 P2 official outage has no countdown, terminal zero or frontend polling',()=>{
 const tab={data:{matches:[],capabilities:[{name:'match-history',state:'failed',historyStatus:'official',retryAfter:1}]}};
 const f=compile(source,['historyServiceState','renderHistoryServiceStatus','paginationCopyFor','scheduleHistoryServerRetry'],{riotTab:()=>false,serviceOutageIcon:()=>'',matchesPending:()=>false,number:String,escapeHTML,state:{},clearTimeout:()=>{},setTimeout:()=>assert.fail('official outage must await backend recovery')});
 assert.doesNotMatch(f.renderHistoryServiceStatus(tab),/秒后|data-history-countdown/);assert.equal(f.paginationCopyFor(tab),'<span>官方战绩服务暂时中断，部分玩家战绩无法读取，恢复后点「刷新对局」</span>');assert.doesNotMatch(f.paginationCopyFor(tab),/<button/);f.scheduleHistoryServerRetry(tab);
});
test('R278 P3 empty revision round trips and malformed reference carries silent fallback reason',()=>{
 const f=compile(source,['resolveOverviewFrame'],{}),stream={matches:new Map(),overview:{}};
 f.resolveOverviewFrame(stream,{matches:[{gameId:1,participants:[]}]});assert.equal(f.resolveOverviewFrame(stream,{matches:[{gameId:1,revision:''}]}).matches[0].gameId,1);
 assert.throws(()=>f.resolveOverviewFrame(stream,{matches:[{gameId:2,revision:'x'}]}),e=>e.referenceFallback==='missing');assert.doesNotMatch(source,/已保留前 \$\{number\(rawMatches.length\)\} 场/);
});
test('R278 P5 pending ranks never invent unranked and CN uses milestones',()=>{
 const f=compile(source,['renderRanks'],{renderRankMMRPopover:()=>'',riotTab:()=>false,escapeHTML,renderRankHistory:()=>'<div class="rank-history">foreign</div>',renderRankMilestones:()=>'<div>历史最高</div>',renderCareerPendingMarkup:markup=>markup});
 const pending=f.renderRanks([],[],[],null,null,{loading:true});assert.doesNotMatch(pending,/未定级|Unranked/);
 const html=f.renderRanks([],[{name:'ranked-stats',state:'available'}],[],{},null,{loading:false});assert.match(html,/未定级/);assert.doesNotMatch(html,/rank-history|尚未定级/);
 const css=fs.readFileSync(process.env.R278_CSS_SOURCE || __dirname+'/gameplay.css','utf8');assert.doesNotMatch(css,/\[data-career-key="ranks"\]\s*\{\s*min-height:\s*450px/);
});
test('R278 P6 completion clears all pending flags and marks filter/highlights ready',()=>{
 const tab={loading:true,initialPagePending:true,initialPageError:'old',matchesReceived:false,recentStatsReceived:false,overviewCardLoad:{ready:new Set()}};
 const f=compile(source,['finishOverviewLoad'],{});f.finishOverviewLoad(tab);assert.equal(tab.initialPagePending,false);assert.equal(tab.matchesReceived,true);assert.equal(tab.recentStatsReceived,true);assert(tab.overviewCardLoad.ready.has('filterbar'));assert(tab.overviewCardLoad.ready.has('highlights'));
});
test('R278 P7 observer survives a blocked append and retries when loading finishes',async()=>{
 const dom=new JSDOM('<div id="app-scroll"><main><div data-match-sentinel></div></main></div>');try{
 const root=dom.window.document.querySelector('#app-scroll'),container=root.firstChild,tab={data:{pagination:{hasMore:true}},loadingMore:true},state={},timers=[],events=[];let cb,disconnected=0,calls=0;
 class IO{constructor(fn){cb=fn}observe(){}disconnect(){disconnected++}}
 const f=compile(source,['bindMatchSentinel'],{state,document:dom.window.document,window:{IntersectionObserver:IO,requestAnimationFrame:fn=>fn(),reportFlowDiagnostic:(...v)=>events.push(v)},IntersectionObserver:IO,matchObserverKey:()=> 'observer',overviewContainer:()=>container,loadOverview:async()=>{calls++;if(tab.loadingMore)return false;tab.overviewRequestToken=Number(tab.overviewRequestToken||0)+1;return true},setTimeout:fn=>{timers.push(fn);return timers.length},clearTimeout:()=>{}});
 f.bindMatchSentinel(container,tab);assert(cb,'must bind while loading');cb([{isIntersecting:true}]);root.dispatchEvent(new dom.window.WheelEvent('wheel',{deltaY:10}));if(timers.length)await timers.shift()();assert.equal(disconnected,0);assert.equal(calls,0);tab.loadingMore=false;state.observer.check();if(timers.length)await timers.shift()();assert.equal(calls,1);assert.equal(disconnected,1);assert.equal(state.observer,null);assert(events.some(v=>v[1]==='started' && v[2].started));
 }finally{dom.window.close()}
});
test('R278 P8 positive time boundary remains search-local; NOT continues older and stop remains usable',async()=>{
 const now=Date.now(),rows=Array.from({length:200},(_,i)=>({gameId:i+1,createdAt:now-i*86400000/2,participants:[{championId:i===90?999:103}]})),tab={advancedConditions:{time:{values:['d30']},hero:{values:['999']}},data:{matches:rows.slice(0,20),pagination:{hasMore:true}}};let cursor=20;
 const ctx={rows:()=>tab.data.matches,version:()=>tab.data.matches,cursor:()=>cursor,now:()=>now,subject:m=>m.participants[0],tags:()=>[],isActive:()=>true,render(){},load:async n=>{tab.data.matches=tab.data.matches.concat(rows.slice(cursor,cursor+n));cursor+=n;return true;}};
 await af.find(tab,ctx);assert.equal(tab.data.pagination.hasMore,true);assert.equal(tab.advancedSearch.stopReason,'time_boundary');tab.advancedConditions.time.not=true;ctx.progress=()=>{if(cursor>=120 && tab.advancedSearch.running)af.cancel(tab)};await af.find(tab,ctx);assert(cursor>=120);assert.equal(tab.advancedSearch.stopReason,'user_stop');
});
test('R278 P9 chromas keep blue marker without sidebar marker; skin SSE needs no status refresh',()=>{
 const storage={getItem:()=>null,setItem(){}},f=require(process.env.R278_COLLECTION_MODULE || './collection-new-items.js').create(storage),status={connected:true,collectionAccount:'safe-account',clientRegion:'TENCENT',serverId:'HN10',snapshotReady:true,ownedCount:1,chromaOwnedCount:1};
 f.setAccount(status);f.setAccount({...status,chromaOwnedCount:2});assert.equal(f.hasUnread(),false);f.update('chromas',[{id:1,owned:true}]);f.update('chromas',[{id:1,owned:true},{id:2,owned:true}]);assert(f.isNew('chromas',2));assert.equal(f.hasUnread(),false);f.newSkins({count:1});assert.equal(f.hasUnread(),true);
});
test('R278 P10 cached data cannot suppress an unmounted or loading collection',()=>{
 const dom=new JSDOM('<main><div class="loading">加载中</div></main>');try{const f=compile(app,['collectionIsRendered'],{el:{grid:dom.window.document.querySelector('main')},state:{items:[{id:1}],renderedSignature:'old'}});assert.equal(f.collectionIsRendered('old'),false);}finally{dom.window.close()}
 assert.doesNotMatch(source,/else if \(shouldReloadOverview\(tab\)\) loadOverview\(tab, true\)/);
});
test('R278 P11 empty preset name only marks placeholder; duplicate names do not overwrite',()=>{
 let raw='[]';const storage={getItem:()=>raw,setItem:(k,v)=>raw=v};assert(af.savePreset(storage,'one',{hero:{values:['103']}}));assert.equal(af.savePreset(storage,'one',{hero:{values:['86']}}),false);
 const dom=new JSDOM('<footer><input data-af-name><p data-af-save-error></p></footer>');try{const input=dom.window.document.querySelector('input'),f=af.saveCurrentPreset;assert.equal(typeof f,'function');f(dom.window.document.querySelector('footer'),{}, {storage,error:()=>assert.fail('must not toast')});assert.equal(input.getAttribute('aria-invalid'),'true');assert.equal(dom.window.document.activeElement,input);assert.equal(dom.window.document.querySelector('[data-af-save-error]').textContent,'');}finally{dom.window.close()}
});

test('R278 P4 foreign launcher omits scan action and CN retains it only when both entries missing',()=>{
 const dom=new JSDOM('<div id="pad"></div><div id="list"></div><div id="eye"></div><div id="title"></div><div id="desc"></div><div id="status"></div>');
 try {const d=dom.window.document,el={clientLaunchpad:d.querySelector('#pad'),launcherList:d.querySelector('#list'),launchpadEyebrow:d.querySelector('#eye'),launchpadTitle:d.querySelector('#title'),launchpadDescription:d.querySelector('#desc'),officialLoginStatus:d.querySelector('#status')},state={section:'overview',overviewGroup:'kr',clientView:{state:'no-client'},installationsLoaded:true,installations:[]};
 const f=compile(app,['renderLaunchpad'],{state,el,escapeHTML,loadClientInstallations(){},launchDetectedClient(){},launchOfficialLogin(){}});f.renderLaunchpad();assert(!el.launcherList.querySelector('.scan-launchers'));assert.equal(el.launcherList.querySelector('[data-client-id=riot]').disabled,true);assert.match(el.launcherList.textContent,/本机未安装 Riot 客户端/);assert.doesNotMatch(el.officialLoginStatus.textContent,/未找到/);
 state.overviewGroup='players';f.renderLaunchpad();assert(el.launcherList.querySelector('.scan-launchers'));state.installations=[{id:'tcls',available:true}];f.renderLaunchpad();assert(!el.launcherList.querySelector('.scan-launchers'));
 }finally{dom.window.close()}
});
