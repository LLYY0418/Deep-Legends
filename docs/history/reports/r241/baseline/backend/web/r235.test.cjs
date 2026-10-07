'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile,escapeHTML}=require('./r188-harness.cjs'),{expand}=require('./r211-harness-support.cjs');
const source=read('gameplay.js');
const functions=(names,deps)=>compile(source,expand(source,names,deps),deps);
function arena(queue,place,index=0,count=queue===1750 ? 6 : 8){return {queueId:queue,modeGroup:'arena',gameId:index+1,createdAt:100-index,subjectParticipantId:1,result:'unknown',participants:Array.from({length:count},(_,i)=>({participantId:i+1,subteamId:i+1,placement:i ? i+1 : place}))}}
test('R235 arena 3x6 streak and shared boundaries; fixed-four mutation is killed',()=>{
 const check=s=>{const f=compile(s,['arenaPlacementResult','computeOverviewStreak','renderOverviewStreak'],{matchSubject:m=>m.participants[0]});
 assert.equal(f.computeOverviewStreak([1,2,4,3,4,5].map((p,i)=>arena(1750,p,i)),false,''),null);
 const strong=f.computeOverviewStreak([1,2,3,3,1].map((p,i)=>arena(1750,p,i)),false,'');assert.equal(strong.count,5);assert.match(f.renderOverviewStreak(strong),/is-strong/);
 assert.equal(f.computeOverviewStreak([1,2,4,3,4].map((p,i)=>arena(1700,p,i)),false,'').count,5);
 for(const [q,p,result]of [[1750,3,'win'],[1750,4,'loss'],[1700,4,'win'],[1700,5,'loss']]){assert.equal(f.arenaPlacementResult(arena(q,p),p),result);assert.equal(f.arenaPlacementResult(arena(q,p,0,0),p),result)}
 assert.equal(f.arenaPlacementResult(arena(999,4,0,0),4),'unknown');};check(source);
 const mutant=source.replace('Number(placement)<=Math.ceil(count/2)','Number(placement)<=4');assert.notEqual(mutant,source);assert.throws(()=>check(mutant),assert.AssertionError);
});
test('R235 data labels preserve missing fields, globals, team maxima and single kills',()=>{
 const f=functions(['matchDataTags','renderMatchTags'],{state:{matchTimelines:new Map()},escapeHTML,scorePlacementChip:()=>'<b>MVP</b>',matchTimelineKey:()=>''});
 const participants=Array.from({length:10},(_,i)=>({participantId:i+1,teamId:i<5 ? 100:200,damage:i===0 ? 10000 : i===5 ? 9000 : 1000,gold:10000,kills:3,assists:2,deaths:1,cs:100,totalHeal:100,damageTaken:100,timeCCingOthers:10,totalDamageShieldedOnTeammates:100,damageDealtToTurrets:100}));
 const match={result:'win',gameId:1,mapId:11,duration:1800,participants};
 assert(f.matchDataTags(match,participants[0]).some(t=>t.label==='★伤害'));assert(f.matchDataTags(match,participants[5]).some(t=>t.label==='伤害'));assert(!f.matchDataTags(match,participants[0]).some(t=>t.label==='伤害'));
 participants[0].soloKills=1;assert(!f.matchDataTags(match,participants[0]).some(t=>t.label.startsWith('单杀')));participants[0].soloKills=3;assert(f.matchDataTags(match,participants[0]).some(t=>t.label==='单杀×3'));
 delete participants[0].soloKills;delete participants[0].totalHeal;assert(!f.matchDataTags(match,participants[0]).some(t=>t.label.includes('治疗')||t.label.includes('单杀')));
 const dom=new JSDOM(f.renderMatchTags(match,participants[0],{badge:"MVP"},{}));const collection=dom.window.document.querySelector('.match-tags-collection'),items=[...collection.querySelectorAll('[data-match-tag]')];
 Object.defineProperty(collection,'clientWidth',{value:150});items.forEach(item=>item.getBoundingClientRect=()=>({width:55}));
 const fit=compile(source,['fitMatchTags'],{});fit.fitMatchTags(dom.window.document);assert.equal(items.filter(i=>!i.hidden).length,2);assert.match(collection.querySelector('button').textContent,/^\+\d/);const tooltip=collection.querySelector('button').dataset.tooltip;assert(tooltip.includes('★伤害')&&tooltip.includes('MVP'));assert(items.length>4);dom.window.close();
});
test('R235 table clicked row stays anchored after earlier expansion collapses',()=>{
 const dom=new JSDOM('<div id="app-scroll"><main></main></div>'),document=dom.window.document,root=document.querySelector('main'),scroll=document.getElementById('app-scroll');
 const row=(id,opponents=[])=>({championId:id,championName:`英雄${id}`,games:40-id,wins:20,losses:10,winRate:60,opponents});
 const tab={championTableExpandedId:'2',overviewSubpageState:{data:{queue:'420',overall:row(0),rows:Array.from({length:18},(_,i)=>row(i+1,Array.from({length:10},(_,j)=>row(30+j))))}}};
 const f=functions(['renderChampionTable','bindChampionTable'],{document,escapeHTML,iconFigure:()=>'',prepareImages:()=>{},openOverviewSubpage:()=>{}});root.innerHTML=f.renderChampionTable(tab.overviewSubpageState.data,tab);f.bindChampionTable(root,tab);
 const anchor=root.querySelector('[data-table-expand="15"]');anchor.getBoundingClientRect=()=>({top:[...root.querySelector('tbody').children].indexOf(anchor)*30-scroll.scrollTop});const before=anchor.getBoundingClientRect().top;anchor.click();assert(Math.abs(anchor.getBoundingClientRect().top-before)<=1);dom.window.close();
});
test('R235 filter restore uses the identical DOM, expanded state, scroll and four-view LRU',()=>{
 const dom=new JSDOM('<div id="app-scroll"><main><div class="match-filterbar"></div><div class="match-list"><article id="original"></article></div></main></div>'),document=dom.window.document,container=document.querySelector('main');
 const tab={key:'one',matchFilter:'all',openMatches:new Set(['1']),matchDetailTabs:new Map([['1','build']]),buildPlayers:new Map(),matchViews:new Map(),data:{matches:[{gameId:1}],pagination:{hasMore:true}}};const view={matches:tab.data.matches,at:Date.now()};tab.matchViews.set('all',view);
 const original=container.querySelector('#original');const f=functions(['stashMatchFilterDOM','restoreMatchFilterDOM'],{document,overviewContainer:()=>container,renderMatchFilters:()=>'<div class="match-filterbar"></div>',bindMatchFilterControls:()=>{},requestAnimationFrame:fn=>fn(),fitMatchTags:()=>{},bindMatchSentinel:()=>{},restoreMatchScrollTop:()=>{document.getElementById('app-scroll').scrollTop=240}});
 f.stashMatchFilterDOM(tab,view);assert(!original.isConnected);tab.openMatches.clear();assert(f.restoreMatchFilterDOM(tab,view));assert.equal(container.querySelector('#original'),original);assert(tab.openMatches.has('1'));assert.equal(document.getElementById('app-scroll').scrollTop,240);
 for(let i=0;i<5;i++){const v={};tab.matchViews.set(String(i),v);f.stashMatchFilterDOM(tab,v)}assert.equal(tab.matchViews.size,4);dom.window.close();
});
test('R235 retry transitions from error to loading to ready without throttling',async()=>{
 const tab={key:'one',playerRef:'ref',mayhemRating:{status:'error',playerRef:'ref',at:Date.now()}},calls=[];
 let complete;const f=functions(['loadMayhemRating','mayhemRatingStatus','renderRankMMRPopover'],{state:{},escapeHTML,number:String,riotTab:()=>false,tabServerID:()=> 'HN1',updateMayhemRatingPopover:()=>{},rerenderTab:()=>{},api:(_url,_options,_key,timeout)=>{calls.push(timeout);return new Promise(r=>complete=r)}});
 assert.match(f.renderRankMMRPopover(tab),/data-mayhem-rating-retry/);const request=f.loadMayhemRating(tab);assert.equal(tab.mayhemRating.status,'loading');assert.match(f.renderRankMMRPopover(tab),/查询中/);complete({available:true,rating:2350});await request;assert.equal(tab.mayhemRating.status,'ready');assert.deepEqual(calls,[40000]);
});

test('R235 history head keeps pagination tail and rejects older generations',()=>{
 const f=compile(source,['mergeCurrentHistoryHead','handleOverviewMatches','syncOverviewStreak'],{state:{tabs:[],overlay:[]},renderFilteredMatchView:()=>{},computeOverviewStreak:()=>null,rerenderMatch:()=>{}});
 assert.deepEqual(f.mergeCurrentHistoryHead([{gameId:1},{gameId:2},{gameId:3}],[{gameId:4}]),[{gameId:4},{gameId:2},{gameId:3}]);
 const state={tabs:[{playerRef:'self',data:{historyGeneration:2,matches:[{gameId:1},{gameId:2},{gameId:3}],pagination:{hasMore:true}}}],overlay:[]};
 const api=compile(source,['mergeCurrentHistoryHead','handleOverviewMatches','syncOverviewStreak'],{state,renderFilteredMatchView:()=>{},computeOverviewStreak:()=>null,rerenderMatch:()=>{}});
 api.handleOverviewMatches({account:'self',replace:true,historyGeneration:1,matches:[{gameId:9}]});assert.equal(state.tabs[0].data.matches[0].gameId,1);
 api.handleOverviewMatches({account:'self',replace:true,historyGeneration:3,matches:[{gameId:4}],pagination:{hasMore:false}});assert.equal(state.tabs[0].data.matches.length,3);assert.equal(state.tabs[0].data.pagination.hasMore,true);
});

test('R235 restoring a 31-second filter schedules one head page and prepends only new cards',async()=>{
 const dom=new JSDOM('<main><div class="match-list"><article data-match-id="1"></article></div></main>'),document=dom.window.document,list=document.querySelector('.match-list'),original=list.firstChild;
 const all={at:Date.now()-31000,matches:[{gameId:1}],pagination:{hasMore:false},list};
 const tab={key:'self',matchFilter:'flex',matchViews:new Map([['all',all]]),data:{player:{playerRef:'self'},matches:[{gameId:3}],pagination:{hasMore:false}},openMatches:new Set(),matchDetailTabs:new Map()};
 const scheduled=[],requests=[],state={settings:{matchCount:20},controllers:new Map()};
 const f=functions(['updateMatchFilter','refreshMatchFilterHead'],{document,state,rememberMatchScrollTop:()=>{},stashMatchFilterDOM:()=>{},resetBuildPlayerSelection:()=>{},matchObserverKey:()=> 'observer',restoreMatchFilterDOM:()=>true,setTimeout:fn=>{scheduled.push(fn);return 1},clearTimeout:()=>{},api:async(url,options)=>{requests.push(JSON.parse(options.body));return {matches:[{gameId:2},{gameId:1}]}},renderMatch:m=>`<article data-match-id="${m.gameId}"></article>`,bindOverviewContent:()=>{},prepareImages:()=>{},overviewContainer:()=>document.querySelector('main'),fitMatchTags:()=>{}});
 await f.updateMatchFilter(tab,'all');assert.equal(scheduled.length,1);scheduled[0]();await new Promise(setImmediate);
 assert.equal(requests.length,1);assert.equal(requests[0].begIndex,0);assert.equal(requests[0].count,20);assert.deepEqual(all.matches.map(m=>m.gameId),[2,1]);assert.equal(list.lastChild,original);dom.window.close();
});

test('R235 late Arena loss updates the banner in place and removes the cached win streak',()=>{
 const dom=new JSDOM('<main><img id="avatar"><div class="summoner-name-meta"></div></main>'),document=dom.window.document,root=document.querySelector('main'),avatar=root.querySelector('img'),meta=root.querySelector('.summoner-name-meta');
 const tab={playerRef:'self',data:{player:{playerRef:'self'},matches:[1,2,3].map((p,i)=>arena(1750,p,i)),pagination:{hasMore:false}},overviewStreak:{result:'win',count:3}};
 const f=functions(['handleOverviewMatches'],{document,state:{tabs:[tab],overlay:[]},overviewContainer:()=>root,renderFilteredMatchView:()=>{},rerenderMatch:()=>{},matchSubject:m=>m.participants[0]});
 const render=compile(source,['renderOverviewStreak'],{});meta.innerHTML=render.renderOverviewStreak(tab.overviewStreak);assert(meta.querySelector('.summoner-streak'));
 f.handleOverviewMatches({account:'self',replace:true,historyGeneration:2,matches:[arena(1750,4)],pagination:{hasMore:false}});
 assert.equal(tab.overviewStreak,null);assert.equal(meta.querySelector('.summoner-streak'),null);assert.equal(root.querySelector('img'),avatar);assert.equal(root.querySelector('.summoner-name-meta'),meta);dom.window.close();
});
