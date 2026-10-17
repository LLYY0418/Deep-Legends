'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,extract}=require('./r188-harness.cjs');
const game=fs.readFileSync(process.env.R282_GAMEPLAY_SOURCE||__dirname+'/gameplay.js','utf8'),app=fs.readFileSync(process.env.R282_APP_SOURCE||__dirname+'/app.js','utf8');
test('R282 P1 baseline filled navigation, animated streaks and gradient multikills',()=>{
 const html=fs.readFileSync(process.env.R282_HTML_SOURCE||__dirname+'/index.html','utf8'),css=fs.readFileSync(process.env.R282_APP_CSS_SOURCE||__dirname+'/app.css','utf8');
 const dom=new JSDOM(html);assert(dom.window.document.querySelector('#section-overview rect'));assert(!/\.function-icon\{[^}]*!important/.test(css));assert(!css.includes(':has(>.function-icon)'));
 const multi=Function('let multikillGradientSeq=0;'+extract(game,'renderMultiKillTag')+';return renderMultiKillTag')();
 for(const level of [2,3,4,5]){const d=new JSDOM(multi(level,1));assert(d.window.document.querySelector('linearGradient'));assert(d.window.document.querySelector('.mk-plate'));d.window.close();}
 const streak=extract(game,'renderOverviewStreak');assert(streak.includes('streak-outer-flame'));assert(streak.includes('streak-rain rain-1'));assert(fs.readFileSync(process.env.R282_GAMEPLAY_CSS_SOURCE||__dirname+'/gameplay.css','utf8').includes('prefers-reduced-motion'));dom.window.close();
});
test('R282 P2 forced sidebar expansion is temporary within the same scale',()=>{
 const dom=new JSDOM('<div class="app-frame"></div><div class="topbar"></div><button id="a"></button><button id="b"></button>'),document=dom.window.document;
 const preferences=new Map([['sidebar','expanded']]);Object.defineProperty(document.querySelector('.app-frame'),'clientWidth',{value:1500});
 const deps={document,window:dom.window,el:{sidebarToggle:document.querySelector('#a'),settingsSidebarToggle:document.querySelector('#b')},preference:(k,d)=>preferences.get(k)||d,savePreference:(k,v)=>preferences.set(k,v)};
 const f=Function(...Object.keys(deps),'let sidebarOverride=null;'+['sidebarConstraint','applySidebar','toggleSidebar'].map(n=>extract(app,n)).join('\n')+';return {applySidebar,toggleSidebar}')( ...Object.values(deps));
 document.documentElement.dataset.uiScale='1.5';f.applySidebar();assert.equal(document.documentElement.dataset.sidebar,'collapsed');f.toggleSidebar();f.applySidebar();assert.equal(document.documentElement.dataset.sidebar,'expanded');assert.equal(preferences.get('sidebar'),'expanded');document.documentElement.dataset.uiScale='1.14';f.applySidebar();assert.equal(document.documentElement.dataset.sidebar,'expanded');
 preferences.set('sidebar','collapsed');document.documentElement.dataset.uiScale='2';f.applySidebar();f.toggleSidebar();assert.equal(preferences.get('sidebar'),'collapsed');document.documentElement.dataset.uiScale='1';f.applySidebar();assert.equal(document.documentElement.dataset.sidebar,'collapsed');dom.window.close();
});
test('R282 P8 five disconnect generations clear every career aggregate before complete',()=>{
 const reset=compile(game,['resetCareerState']).resetCareerState,tab={careerGeneration:0};
 for(let i=1;i<=5;i++){
  Object.assign(tab,{data:{rankedQueues:{420:{recentRanked:{games:0}}}},recentStatsReceived:true,overviewView:{content:'stale'},confirmedStats:{games:99},pendingHistoricalRanks:{},opggSeason:{},seasonRanksFilled:new Set(['420'])});
  reset(tab);assert.equal(tab.careerGeneration,i);assert.equal(tab.data,null);assert.equal(tab.recentStatsReceived,false);for(const key of ['overviewView','confirmedStats','pendingHistoricalRanks','opggSeason','seasonRanksFilled'])assert.equal(tab[key],undefined);
  tab.data={rankedQueues:{420:{recentRanked:{games:20},abilitySampleGames:20}},overall:{games:20}};tab.careerStatsSource='complete';
  const queue=compile(game,['rankedQueueData'],{MAYHEM_QUEUE_TAB_KEY:'mayhem',isMayhemQueueId:()=>false,rankedQueueLabel:()=>'',rankedQueueKey:()=> '420'}).rankedQueueData(tab.data,tab,'recent');assert.equal(queue.recentRanked.games,20);
 }
 assert(extract(game,'loadOverview').includes('...payload,\n          ...header'));
});
test('R282 P9 returning to already loaded pools must repaint interrupted catalog',()=>{
 const body=extract(app,'activateFavoritesPage');assert.match(body,/state\.poolRenderInterrupted \|\| state\.poolRenderPending\) renderPoolCatalog\(\)/);
 assert(extract(app,'activateSection').includes('state.poolRenderInterrupted=true'));assert(body.includes('state.poolRenderInterrupted=true'));
});
test('R282 P10 independent all-history cursor outlives the 43-game queue window',async()=>{
 const af=require('./history-filters.js'),now=Date.now(),rows=Array.from({length:85},(_,i)=>({gameId:i+1,createdAt:now-(i>=55?40:1)*86400000,queueId:420,participants:[{championId:1}]}));
 const tab={key:'self',matchFilter:'solo',data:{matches:rows.slice(0,43),pagination:{hasMore:false}},advancedConditions:{time:{not:true,values:['d30']}}};let requests=0;
 const deps={riotTab:()=>false,api:async(_url,{body})=>{requests++;const q=JSON.parse(body);assert.equal(q.matchFilter,'all');return {matches:rows.slice(q.begIndex,q.begIndex+q.count),pagination:{begIndex:q.begIndex,count:q.count,hasMore:q.begIndex+q.count<85}};},normalizedPagination:(p,b)=>({...p.pagination,nextBegIndex:b+p.pagination.count}),loadOverview:()=>{throw Error('queue cursor reused')},TextEncoder};
 const f=compile(game,['advancedHistoryState','advancedHistoryHasMore','loadAdvancedHistory'],deps);const ctx={rows:()=>tab.data.matches,subject:m=>m.participants[0],tags:()=>[],now:()=>now,isActive:()=>true,render(){},cursor:()=>tab.advancedHistory?.next||0,hasMore:()=>f.advancedHistoryHasMore(tab),load:n=>f.loadAdvancedHistory(tab,n),pageScanned:()=>tab.advancedHistory.lastScanned,scannedCount:()=>tab.advancedHistory.scanned.size};
 while(f.advancedHistoryHasMore(tab))await af.find(tab,ctx);
 assert.equal(af.filter(tab.data.matches,tab,ctx).length,30);assert.equal(tab.advancedHistory.scanned.size,85);assert.equal(requests,5);assert.equal(tab.data.pagination.hasMore,false);
 const range=af.serverTimeRange(tab,ctx);assert.equal(range.startTime,1);assert(range.endTime<now/1000-29*86400);
});
test('R282 P5 P6 placeholder identity structure and content driven ranks',()=>{
 const body=extract(game,'renderSummonerStrip'),css=fs.readFileSync(process.env.R282_GAMEPLAY_CSS_SOURCE||__dirname+'/gameplay.css','utf8');assert(body.includes('class="summoner-tag'));assert(body.includes('is-identity-placeholder'));assert(body.includes('is-summoner-avatar'));assert(css.includes('--strip-name-height:32px'));assert(!/data-career-key="ranks"\]\{height:(245|444)px/.test(css));
 const history=compile(game,['renderHistoricalRanks'],{escapeHTML:String,rankCrestIcon:()=>'',rankTitle:()=>'',percent:String,number:String}).renderHistoricalRanks;assert.equal(history([]),'');assert(!history([]).includes('rank-history-head'));
});
test('R282 P7 shortest column distribution preserves reading sequence and reopens at top',()=>{
 const dom=new JSDOM('<div id="root" data-career-layout="masonry"><div class="career-dialog-stack"></div><div class="career-dialog-stack"></div></div>'),root=dom.window.document.querySelector('#root');
 [250,80,170,120,60,180,95,90].forEach((height,index)=>{const n=dom.window.document.createElement('div');n.className='career-dialog-item';n.dataset.careerOrder=index;n.getBoundingClientRect=()=>({height});root.firstElementChild.append(n);});
 const f=compile(game,['distributeCareerDialog'],{nodes:{careerDialogContent:root},document:dom.window.document});f.distributeCareerDialog();const columns=[...root.children].map(c=>[...c.children].map(n=>Number(n.dataset.careerOrder)));assert.deepEqual(columns,[[0,3,6,7],[1,2,4,5]]);assert(extract(game,'openCareerDialog').includes('nodes.careerDialogContent.scrollTop = 0'));dom.window.close();
});
test('R282 P11 P12 P13 light labels, selected runes and busy copy',()=>{
 const css=fs.readFileSync(process.env.R282_APP_CSS_SOURCE||__dirname+'/app.css','utf8'),gamecss=fs.readFileSync(process.env.R282_GAMEPLAY_CSS_SOURCE||__dirname+'/gameplay.css','utf8');assert(css.includes('[data-theme="light"] .facade-grid .skin-copy strong'));assert(gamecss.includes('[data-theme="light"] .arena-augment-icon'));assert(gamecss.includes('.specialist-game-row.is-selected {outline:none;border:2px'));
 assert(extract(game,'renderSpecialistPlayers').includes('rune-position-tag'));
 const dom=new JSDOM('<button><span class="refresh-icon"></span><span>重新读取</span></button>'),button=dom.window.document.querySelector('button'),f=compile(app,['setRefreshBusy'],{el:{refresh:button}});
 for(const busy of [true,false,true,false]){f.setRefreshBusy(busy);const text=busy?'正在获取':'重新读取';assert.equal(button.lastElementChild.textContent,text);assert.equal(button.getAttribute('aria-label'),text);assert.equal(button.dataset.tooltip,text);}dom.window.close();
});

test('R282 P16 Arena bravery survives transition until real champions arrive',()=>{
 const f=compile(game,['retainArenaBravery','liveBraveryPortrait']);
 const players=[{playerRef:'self',isCurrent:true,championId:-3,bravery:true},{playerRef:'ally',isAlly:true,championPickIntent:-3,bravery:true}];
 for(const p of players)assert(f.liveBraveryPortrait(p,true,'ChampSelect'));
 assert(!f.liveBraveryPortrait(players[0],false,'ChampSelect'));assert(!f.liveBraveryPortrait({championId:-3},true,'ChampSelect'));
 const previous={queueId:1750,gameId:77,phase:'ChampSelect',players},next={queueId:1750,gameId:77,phase:'GameStart',players:players.map(p=>({...p,championId:0,championPickIntent:0,bravery:false}))};
 f.retainArenaBravery(previous,next);for(const p of next.players)assert(f.liveBraveryPortrait(p,true,'GameStart'));
 const actual={...next,phase:'InProgress',players:next.players.map((p,i)=>({...p,championId:1+i,bravery:false}))};f.retainArenaBravery(next,actual);for(const p of actual.players)assert(!f.liveBraveryPortrait(p,true,'InProgress'));
 const another={...next,gameId:78,players:next.players.map(p=>({...p,bravery:false}))};f.retainArenaBravery(previous,another);assert(another.players.every(p=>!p.bravery));
 const suite=fs.readFileSync(process.env.R282_SUITE_SOURCE||__dirname+'/suite.js','utf8');assert.equal((suite.match(/src="\/bravery.svg"/g)||[]).length,3);assert(!suite.includes('>⚔</span>'));
 const dom=new JSDOM(fs.readFileSync(process.env.R282_HTML_SOURCE||__dirname+'/index.html','utf8'));const buttons=['skin-dialog-fullscreen','skin-dialog-close'].map(id=>dom.window.document.getElementById(id));assert.equal(buttons[0].className,buttons[1].className);dom.window.close();
});

test('R282 P17 collection tracks and skeletons do not depend on card count',()=>{
 const css=fs.readFileSync(process.env.R282_APP_CSS_SOURCE||__dirname+'/app.css','utf8');
 for(const selector of ['skin-grid','champion-skins','chroma-prestige-grid','loot-card-grid','reward-grid'])assert.match(css,new RegExp('\\.'+selector+' \\{[^}]*repeat\\(auto-fill'));
 const skeleton=compile(app,['renderCollectionSkeleton'],{state:{view:'chromas',qualitySelections:new Set(),sort:'name'}}).renderCollectionSkeleton();
 for(const cls of ['chroma-prestige-grid','chroma-stack-cards','chroma-mini-card'])assert(skeleton.includes(cls));
 assert(!extract(app,'configureSparseGrid').includes('count * 640'));
});
test('R282 P18 ARAM and Rift preferred ranks have crests; Arena does not show ranked tiers',()=>{
 const arena=require('./live-arena.js');globalThis.deepLegendsMatchCards={live:{rankTitle:r=>r.tier+' '+(r.division||'')}};
 try{const solo={tier:'DIAMOND',division:'II'},flex={tier:'MASTER',leaguePoints:132,queueType:'RANKED_FLEX_SR'};
 for(const kind of ['aram','rift']){
  assert.match(arena.identityLine({soloRank:solo,flexRank:flex},kind,'中路 · 未定级'),/rank-crests\/diamond.png/);
  const html=arena.identityLine({soloRank:{tier:'UNRANKED'},flexRank:flex},kind,'中路');assert.match(html,/灵活 MASTER  132 LP/);assert.match(html,/rank-crests\/master.png/);
  assert(!arena.identityLine({},kind,'中路').includes('rank-crests/'));assert.equal(arena.identityLine({hidden:true},kind,''),'');assert.match(arena.identityLine({hidden:true,soloRank:solo},kind,''),/diamond.png/);
 }
 assert(!arena.identityLine({soloRank:solo},'arena','').includes('rank-crests/'));
 }finally{delete globalThis.deepLegendsMatchCards}
});

test('R282 P19 confirmed zero ranked only, real casual samples and shared mode selection',()=>{
 const dom=new JSDOM('',{runScripts:'outside-only'}),w=dom.window;
 w.eval(fs.readFileSync(process.env.R282_CASUAL_SOURCE||__dirname+'/overview-casual.js','utf8'));
 const select=w.deepLegendsOverviewCasual.select;
 const match=(id,queueId=2300)=>({gameId:id,queueId,subjectParticipantId:2,result:id%2?'win':'loss',duration:1200,participants:[{participantId:1,championId:99,kills:99},{participantId:2,championId:id%2+1,championName:'英雄'+(id%2+1),kills:4,deaths:2,assists:8,cs:100}]});
 const data={player:{},ranks:[],capabilities:[{name:'ranked-stats',state:'available'}],matches:Array.from({length:20},(_,i)=>match(i+1))},tab={};
 const result=select(data,tab);assert(result);assert.equal(result.modes.length,1);for(const region of ['HN1','JP','KR','EUW'])assert(select({...data,player:{region}},{}));assert.equal(result.stats.games,20);assert.equal(result.stats.kills,4);assert.equal(result.stats.kda,6);assert.equal(result.champions.length,2);assert.equal(tab.casualMode,'2300');assert.equal(select({...data,matches:[]},tab).stats.games,20);const delayed={};assert.equal(select({...data,matches:[match(30,440)]},delayed),null);assert.equal(select(data,delayed),null);
 for(const changed of [{matches:[...data.matches,match(21,440)]},{ranks:[{queueType:'RANKED_FLEX_SR',wins:0,losses:1,winRate:0}]},{capabilities:[{name:'ranked-stats',state:'failed'}]},{rankedQueues:{420:{seasonGames:1}}},{ranks:[{queueType:'RANKED_SOLO_5x5',wins:0,losses:0,winRate:-1}]}])assert.equal(select({...data,...changed},{}),null);
 assert.equal(select(data,{initialPageError:'failed'}),null);assert.equal(select({...data,ranks:undefined},{}),null);assert.equal(select(data,{loading:true}),null);assert.equal(select(data,{initialPagePending:true}),null);
 const multi={...data,matches:[...data.matches,match(30,450)]};assert.equal(select(multi,{}).selected.key,'2300');assert.equal(select(multi,{casualMode:'450'}).overall.games,1);
 assert(extract(game,'careerSectionEntries').includes("!['ranks','ability','positions'].includes(key)"));assert(extract(game,'careerSectionEntries').includes("replaceAll('近期海斗','近期对局')"));assert(extract(game,'updateCareerCards').includes('child.remove()'));
 dom.window.close();
});

test('R282 P20 content video ignores reduced motion and reports failures once with HTTP status',async()=>{
 for(const reduced of [false,true]){
  const dom=new JSDOM('<main></main>',{url:'http://localhost',runScripts:'outside-only'}),w=dom.window,reports=[];
  try{
   Object.defineProperty(w.document,'hidden',{value:false});w.matchMedia=()=>({matches:reduced});w.reportFlowDiagnostic=(event,reason,fields)=>reports.push({event,reason,fields});w.fetch=async(_url,options)=>{assert.equal(options.method,'HEAD');return {status:404}};w.AbortSignal=AbortSignal;
   w.HTMLMediaElement.prototype.play=function(){return Promise.resolve()};w.HTMLMediaElement.prototype.pause=function(){};
   w.eval(fs.readFileSync(process.env.R282_ART_SOURCE||__dirname+'/overview-art.js','utf8'));
   const main=w.document.querySelector('main'),player={backgroundPosterPath:'/poster',backgroundVideoPath:'/test.webm'};
   main.innerHTML='<section class="summoner-strip">'+w.deepLegendsOverviewArt.render(player,'tab')+'</section>';w.deepLegendsOverviewArt.prepare(main);
   const video=main.querySelector('video');assert(video,'content video must render under reduce');assert(video.getAttribute('src'));Object.defineProperty(video,'error',{value:{code:4}});video.dispatchEvent(new w.Event('error'));await new Promise(r=>setImmediate(r));
   w.dispatchEvent(new w.Event('pagehide'));assert.equal(reports.length,1);assert.equal(reports[0].event,'overview_art_client');assert.equal(reports[0].fields.reduced_motion,reduced);assert.equal(reports[0].fields.media_http_status,404);assert.equal(reports[0].fields.error_code,4);assert.equal(video.classList.contains('is-playing'),false);assert(main.querySelector('[data-overview-poster]'));
  }finally{w.close()}
 }
 assert(extract(game,'renderSummonerStrip').includes("'summoner-avatar',false"));assert(extract(game,'renderOverviewBodyContent').includes('player.backgroundPosterPath,player.backgroundVideoPath'));
});

test('R282 P20 identical per-tab summaries are both delivered, with no identity fields',async()=>{
 const dom=new JSDOM('',{url:'http://localhost',runScripts:'outside-only'}),w=dom.window,bodies=[];
 try{
  Object.assign(w,{Response,Headers,Request,AbortController,fetch:async(_url,options)=>{bodies.push(JSON.parse(options.body));return new Response('{}',{status:200})}});
  w.eval(fs.readFileSync(__dirname+'/runtime.js','utf8'));
  const fields={video_path_present:true,reduced_motion:true,video_rendered:true,src_set:true,playing:true,error_code:0,media_http_status:200,rerender_count:1,playerName:'private'};
  w.reportFlowDiagnostic('overview_art_client','summary',fields);await new Promise(r=>setImmediate(r));w.reportFlowDiagnostic('overview_art_client','summary',fields);await new Promise(r=>setImmediate(r));
  assert.equal(bodies.length,2);assert.equal(Object.keys(bodies[0].r278).length,8);assert(!JSON.stringify(bodies).includes('private'));
 }finally{w.close()}
});

test('R282 P20 incomplete skeleton does not finalize a no-video diagnostic',()=>{
 const dom=new JSDOM('',{runScripts:'outside-only'}),w=dom.window,timers=[],reports=[];
 try{
  w.setTimeout=fn=>{timers.push(fn);return timers.length};w.clearTimeout=()=>{};w.reportFlowDiagnostic=(event,reason,fields)=>reports.push(fields);
  w.eval(fs.readFileSync(__dirname+'/overview-art.js','utf8'));
  w.deepLegendsOverviewArt.render({},'slow-tab',false);assert.equal(timers.length,0);
  w.deepLegendsOverviewArt.render({backgroundPosterPath:'/poster'},'slow-tab',true);assert.equal(timers.length,1);
  w.deepLegendsOverviewArt.flush();w.deepLegendsOverviewArt.flush();assert.equal(reports.length,1);assert.equal(reports[0].video_path_present,false);assert.equal(reports[0].rerender_count,1);
 }finally{w.close()}
});
