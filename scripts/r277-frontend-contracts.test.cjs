'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const web=process.env.R277_WEB || path.resolve(__dirname,'../backend/web');
const read=n=>fs.readFileSync(path.join(web,n),'utf8');
const {compile,escapeHTML}=require('../backend/web/r188-harness.cjs');
const {JSDOM}=require('../desktop/node_modules/jsdom');
const game=read('gameplay.js'),app=read('app.js'),css=read('gameplay.css');
test('P1 client view lifecycle diagnostics are deduplicated and reject backwards generations',()=>{
 const state={clientView:{generation:3,state:'ready'}},events=[];
 const api=compile(app,['acceptClientView','reportClientView'],{state,window:{reportFlowDiagnostic:(...e)=>events.push(e),dispatchEvent(){}},CustomEvent:class{},renderClientConnection(){},renderLaunchpad(){},retryCollectionAfterConnection(){},clearDisconnectedClientState(){},queueLiveUpdateSlices(){}});
 for(let i=0;i<20;i++)assert.equal(api.acceptClientView({type:'client-view',state:'ready',generation:2}),false);
 assert.equal(events.length,2);assert(events.some(e=>e[1]==='rejected' && e[2].reason==='generation-rollback'));
 for(const [generation,stateName] of [[4,'exiting'],[5,'no-client'],[6,'ready']])assert(api.acceptClientView({type:'client-view',state:stateName,generation}));
 assert.equal(state.clientView.state,'ready');
});
test('P2 user requests can abort background overview before foreground handoff',async()=>{
 for(const request of ['more','retry','scroll']){
  let aborted=0,rendered=0;const state={controllers:new Map([['overview:fixture',{abort(){aborted++;}}]]),destroyed:false},tab={key:'fixture',overviewBackground:true,loading:true,data:{matches:[],pagination:{hasMore:true}}};
  const api=compile(game,['loadOverview'],{state,connected:()=>true,riotTab:()=>true,globalThis:{deepLegendsHistoryRecovery:{reset(){}}},tabReady:()=>false,rerenderTab(){rendered++;}});
  assert.equal(await api.loadOverview(tab,request==='retry',request!=='retry',request!=='scroll',false,0,false,request==='scroll'),false);
  assert.equal(aborted,1,request);assert.equal(rendered,1,request);assert.equal(tab.overviewBackground,false,request);
 }
});
test('P3 all tag summaries use the same Chinese labels as match cards',()=>{
 const vm=require('node:vm'),ctx={module:{exports:{}},exports:{}};vm.runInNewContext(read('history-filters.js'),ctx);const af=ctx.module.exports;
 for(const [id,label] of Object.entries({damage:'输出',efficiency:'伤转率',control:'控制',heal:'治疗',shield:'护盾',tank:'承伤',kills:'击杀',participation:'参团',tower:'拆塔',gold:'经济',cs:'补兵',solo:'单杀',dive:'越塔',defense:'塔之子',advantage:'压刀',hook:'好钩'})){
 const html=af.renderConditions({advancedConditions:af.normalize({performance:{values:['tag:'+id]}})},()=>({rows:()=>[],subject:()=>null}));assert(html.includes(label),id+': '+html);
 }
});
test('P3 active quota ticker updates 103 seconds and tears down when inactive',()=>{
 let now=1000,active=true;const tasks=[],state={},tab={quotaRetry:{retryAt:104000}},events=[];
 const api=compile(game,['scheduleQuotaCountdown'],{Date:{now:()=>now},setTimeout:fn=>{tasks.push(fn);return tasks.length;},clearTimeout(){},historyRecoveryActive:()=>active,overviewContainer:()=>({querySelectorAll:()=>[{textContent:'',set textContent(v){events.push(v);}}]}),globalThis:{deepLegendsHistoryFilters:null},advancedFilterContext:()=>({}),appendOverviewMatches(){}});
 api.scheduleQuotaCountdown(tab);assert(events[0].includes('103'));
 for(let remaining=102;remaining>=0;remaining--){now+=1000;assert(tasks.length,'ticker must stay armed');tasks.shift()();assert.equal(events.at(-1),String(remaining));}
 assert.equal(tab.quotaCountdownTimer,0);assert.equal(tasks.length,0);
 tab.quotaRetry.retryAt=now+20000;api.scheduleQuotaCountdown(tab);active=false;tasks.shift()();assert.equal(tab.quotaCountdownTimer,0);
 now+=10000;active=true;api.scheduleQuotaCountdown(tab);assert.equal(events.at(-1),'10','resume reads real remaining time');
});
test('P4 skeleton uses final strip grid and filter spans cannot inherit 126px blocks',()=>{
 assert(!/\.summoner-strip\.is-pending-strip\{[^}]*height:116px/.test(css));assert.match(css,/\.match-filterbar\.gameplay-skeleton > span \{[^}]*min-height: 0/);
 assert.match(game,/if \(tab.loading && rawMatches.length/);
});
test('P4 retained filter controls lose skeleton state on empty failure and restore it on retry',()=>{
 const dom=new JSDOM('<div class="match-filterbar gameplay-skeleton" aria-label="正在读取筛选"><button data-af-open>筛选</button></div>'),bar=dom.window.document.querySelector('div'),button=bar.firstElementChild;
 const api=compile(game,['syncMatchFilterPending','matchesPending'],{}),tab={loading:true,data:{matches:[],pagination:{partial:true}},overviewCardLoad:{ready:new Set()}};
 api.syncMatchFilterPending(bar,tab);assert(bar.classList.contains('gameplay-skeleton'));
 tab.loading=false;tab.initialPageError='战绩服务暂时不可用';api.syncMatchFilterPending(bar,tab);
 assert(!bar.classList.contains('gameplay-skeleton'));assert(!bar.hasAttribute('aria-label'));assert.equal(bar.firstElementChild,button);
 tab.loading=true;tab.initialPageError='';api.syncMatchFilterPending(bar,tab);assert(bar.classList.contains('gameplay-skeleton'));assert.equal(bar.getAttribute('aria-label'),'正在读取筛选');assert.equal(bar.firstElementChild,button);
 dom.window.close();
});
test('P5 decoded hero DOM retained on every structural refresh',()=>{
 const dom=new JSDOM('<dialog><div class="af-options is-hero"><button data-af-option="103"><figure class="game-icon"><img src="/decoded"></figure></button></div></dialog>'),menu=dom.window.document.querySelector('dialog'),icon=menu.querySelector('figure'),image=menu.querySelector('img');
 const api=compile(read('history-filters.js'),['retainHeroIcons'],{});
 for(let i=0;i<3;i++){api.retainHeroIcons(menu,()=>{menu.innerHTML='<div class="af-options is-hero"><button data-af-option="103"><figure class="game-icon"><img src="/placeholder"></figure></button></div>';});assert.equal(menu.querySelector('figure'),icon);assert.equal(menu.querySelector('img'),image);}
 assert(/\.af-options strong \{[^}]*font-weight: 600/.test(read('history-filters.css')));dom.window.close();
});
test('P6 collection navigation unread survives restart and visit does not mark cards read',()=>{
 const vm=require('node:vm'),data=new Map(),storage={getItem:k=>data.get(k),setItem:(k,v)=>data.set(k,v)},ctx={module:{exports:{}},exports:{}};vm.runInNewContext(read('collection-new-items.js'),ctx);
 const status={connected:true,collectionAccount:'test',clientRegion:'TENCENT',serverId:'HN1'},make=()=>{const a=ctx.module.exports.create(storage);a.setAccount(status);return a;};let a=make();a.update('skins',[{id:1,owned:true}]);a.update('skins',[{id:1,owned:true},{id:2,owned:true}]);assert(a.hasUnread());a=make();assert(a.hasUnread());a.visit();assert(!a.hasUnread());assert(a.isNew('skins',2));assert(!make().hasUnread());
 assert(read('app.css').includes('outline: 3px solid var(--collection-new-blue)'));
});
test('P7 top buttons never wrap, search is first to shrink',()=>{assert.match(read('app.css'),/\.topbar button \{[^}]*white-space: nowrap/);assert.match(read('app.css'),/\.topbar-actions \{[^}]*flex-shrink: 0/);});
test('P8 absent winrate column omitted, history extra rows obey hidden',()=>{
 const api=compile(game,['renderHistoricalRanks'],{escapeHTML,percent:n=>n+'%',number:String,rankCrestIcon:()=>'',rankTitle:()=> '钻石'});
 const items=Array.from({length:8},(_,i)=>({season:String(i),tier:'diamond'})),html=api.renderHistoricalRanks(items);assert(!html.includes('未提供'));assert(!html.includes('胜率'));assert(html.includes('data-rank-history-extra hidden'));assert(api.renderHistoricalRanks([{...items[0],winRate:53}]).includes('53%'));
 assert.match(css,/\.rank-history-row\[hidden\] \{ display: none/);
});
test('P9 complete references resolve request-local full records and queues remain available',()=>{
 const api=compile(game,['resolveOverviewFrame'],{}),stream={matches:new Map(),overview:{}};const match={gameId:12,revision:'r1',participants:[{score:{value:9}}]};
 api.resolveOverviewFrame(stream,{player:{playerRef:'a'},matches:[match],rankedQueues:{solo:{games:3}}});
 const final=api.resolveOverviewFrame(stream,{player:{playerRef:'a'},matches:[{gameId:12,revision:'r1'}]});assert.equal(final.matches[0],match);assert.equal(final.rankedQueues.solo.games,3);
 assert.throws(()=>api.resolveOverviewFrame(stream,{matches:[{gameId:12,revision:'wrong'}]}),/引用/);
});
test('P9 distant 400-match rows bound rich DOM after a frame and remain recoverable',()=>{
 const dom=new JSDOM('<div class="match-list"></div>'),list=dom.window.document.querySelector('div'),state={},observers=[],frames=[];
 const tab={data:{matches:Array.from({length:400},(_,i)=>({gameId:i}))},openMatches:new Set()};
 for(let i=0;i<400;i++){const e=dom.window.document.createElement('article');e.className='match-entry';e.dataset.matchId=String(i);e.innerHTML='<div>'+ '<span>detail</span>'.repeat(200)+'</div>';e.getBoundingClientRect=()=>({top:i*120,bottom:i*120+120,height:120});list.append(e);}
 const api=compile(game,['boundMatchDOM'],{window:{IntersectionObserver:class{constructor(cb,options){this.cb=cb;this.options=options;observers.push(this);}observe(){}disconnect(){}}},document:dom.window.document,state,getComputedStyle:dom.window.getComputedStyle,requestAnimationFrame:fn=>{frames.push(fn);return frames.length},cancelAnimationFrame(){},renderMatch:m=>`<article class="match-entry" data-match-id="${m.gameId}"><div>${'<span>detail</span>'.repeat(200)}</div></article>`,bindOverviewContent(){},applyRenderedMetricStyles(){},prepareImages(){}});
 api.boundMatchDOM(list,tab);observers.find(o=>o.options.rootMargin==='3200px 0px').cb([...list.children].slice(30).map(target=>({target,isIntersecting:false})));frames.shift()();
 assert(list.querySelectorAll('*').length<25000);assert(list.querySelectorAll('[data-virtual-match]').length>300);const row=list.lastElementChild;observers.find(o=>o.options.rootMargin==='2400px 0px').cb([{target:row,isIntersecting:true}]);frames.shift()();assert(!row.hasAttribute('data-virtual-match'));assert.equal(row.querySelectorAll('span').length,200);dom.window.close();
});
test('P10 actual zoom multiplier and dimensions are diagnostic dedup key',()=>{
 const vm=require('node:vm'),{extract}=require('../backend/web/r188-harness.cjs'),events=[],style={setProperty(){}},state={},window={innerWidth:900,innerHeight:700,screen:{width:1920,height:1080},devicePixelRatio:1.5,reportFlowDiagnostic:(...e)=>events.push(e)};
 const ctx={state,window,document:{documentElement:{style,dataset:{}}},preferredUiScale:()=>1.25,autoUiScale:()=>1,renderUiScaleAutoLabel(){},appliedUiScale:0};vm.runInNewContext(extract(app,'applyUiScale'),ctx);
 ctx.applyUiScale();for(let i=0;i<3600;i++)ctx.applyUiScale();assert.equal(events.length,1);assert.equal(events[0][2].multiplier,1.25);assert.equal(events[0][2].screenWidth,1920);assert.equal(events[0][2].devicePixelRatio,1.5);
 window.innerWidth=1280;ctx.applyUiScale();assert.equal(events.length,2);assert.equal(events[1][2].windowWidth,1280);assert(Buffer.byteLength(JSON.stringify(events))<50000);
});
test('P3 quota automatically resumes once unless the user stopped',async()=>{
 const vm=require('node:vm'),ctx={module:{exports:{}}};vm.runInNewContext(read('history-recovery.js'),ctx);const recovery=ctx.module.exports;
 for(const stopped of [false,true]){let loads=0;const retry={append:true,requestToken:1,filter:'all',count:20},tab={overviewRequestToken:1,matchFilter:'all',advancedSearch:{stopReason:stopped?'user_stop':'quota'},quotaRetry:retry};await recovery.resumeQuota(tab,retry,{state:{settings:{matchCount:20}},active:()=>true,riot:()=>true,load:async()=>{loads++;return false;}});assert.equal(loads,stopped?0:1);assert.equal(tab.quotaRetry,null);}
});
test('P1 P10 diagnostic transport emits safe fields and hourly replay stays below 50KB',async()=>{
 const vm=require('node:vm'),sent=[],ctx={window:{},setTimeout:()=>1,clearTimeout(){},fetch:async(_url,init)=>{sent.push(JSON.parse(init.body));return {status:204};}};vm.runInNewContext(read('runtime.js'),ctx);
 for(let i=0;i<60;i++){
  ctx.window.reportFlowDiagnostic('ui_zoom_state','applied',{mode:'fixed',multiplier:i%2?1.25:1,windowWidth:900+i,windowHeight:700,screenWidth:1920,screenHeight:1080,devicePixelRatio:1.5,secret:'SECRET'});
  ctx.window.reportFlowDiagnostic('client_view_client','applied',{generation:i+1,state:i%2?'ready':'no-client',reason:'new-generation',summoner:'SECRET'});await new Promise(setImmediate);
 }
 assert.equal(sent.length,120);assert.equal(sent[0].multiplier,1);assert.equal(sent[1].generation,1);assert.equal(sent[1].action,'applied');assert.equal(sent[1].reason,'new-generation');assert(!JSON.stringify(sent).includes('SECRET'));assert(Buffer.byteLength(JSON.stringify(sent))<=50000);
});
test('P8 history binding expands and collapses eight real rows, including repeated bind',()=>{
 const dom=new JSDOM('<main></main>'),root=dom.window.document.querySelector('main');
 const api=compile(game,['renderHistoricalRanks','bindRankHistoryControls'],{escapeHTML,percent:n=>n+'%',number:String,rankCrestIcon:()=>'',rankTitle:()=> '钻石'});
 root.innerHTML=api.renderHistoricalRanks(Array.from({length:8},(_,i)=>({season:String(i),tier:'diamond'})));api.bindRankHistoryControls(root);api.bindRankHistoryControls(root);
 const extra=[...root.querySelectorAll('[data-rank-history-extra]')],button=root.querySelector('button');assert.equal(extra.length,3);assert(extra.every(r=>r.hidden));button.click();assert(extra.every(r=>!r.hidden));button.click();assert(extra.every(r=>r.hidden));dom.window.close();
});
test('P9 score breakdown is lazy, single-flight and cached after hover/focus',async()=>{
 const dom=new JSDOM('<article class="match-entry" data-match-id="1"><i class="match-score"></i></article>'),root=dom.window.document.querySelector('article'),requests=[],match={gameId:1,participants:[{participantId:2,score:{value:8}}]},tab={key:'fixture',region:'kr',data:{matches:[match],player:{playerRef:'opaque'}}};
 const api=compile(game,['bindScoreDetails','ensureScoreDetails'],{document:dom.window.document,window:{dispatchEvent(){}},CustomEvent:dom.window.CustomEvent,riotTab:()=>true,matchSubject:()=>match.participants[0],overviewContainer:()=>root,api:async(_url,options)=>{requests.push(JSON.parse(options.body));return {gameId:1,subjectParticipantId:2,participants:[{participantId:2,playerRef:'opaque',score:{value:8,parts:[{key:'damage',value:1,norm:.5}]}}]};}});
 api.bindScoreDetails(root,tab);assert.equal(requests.length,0);const node=root.querySelector('i');node.dispatchEvent(new dom.window.Event('pointerenter'));node.dispatchEvent(new dom.window.Event('focus'));await new Promise(setImmediate);
 assert.equal(requests.length,1);assert.equal(requests[0].refresh,'score-parts');assert.equal(match.participants[0].score.parts.length,1);node.dispatchEvent(new dom.window.Event('pointerenter'));await new Promise(setImmediate);assert.equal(requests.length,1);dom.window.close();
});
test('P4 pending champion statistics never flash zero or overall summary',()=>{
 const api=compile(game,['renderChampionStats'],{escapeHTML,number:String,percent:n=>n+'%',kda:n=>Number(n).toFixed(2),overviewDetailArrow:()=>'',iconFigure:()=>''});
 const html=api.renderChampionStats([],{games:0,kda:0},{collecting:true,complete:false,scanned:0},true);assert(html.includes('career-pending gameplay-skeleton'));assert(!/已统计 0 场|0\.00:1|全部英雄/.test(html));
});
