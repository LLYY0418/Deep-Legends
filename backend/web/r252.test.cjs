"use strict";
const test=require("node:test"),assert=require("node:assert/strict");
const {JSDOM}=require("../../desktop/node_modules/jsdom");
const {read,compile,escapeHTML}=require("./r188-harness.cjs");
test("R252 visible tag hydration indexes the tree once without losing or admitting games",async()=>{
 const matches=Array.from({length:200},(_,i)=>({gameId:252000+i,queueId:420,result:"win",participants:[{participantId:1}]}));
 matches[0].tagsAvailable=true;matches[1].result="remake";matches[2].result="unknown";matches[3].queueId=1700;matches[4].queueId=1710;
 const dom=new JSDOM('<main>'+matches.map(m=>`<article data-match-id="${m.gameId}"></article>`).join('')+'</main>'),container=dom.window.document.querySelector('main');
 matches.push({gameId:252999,queueId:420,result:"win"});
 let treeReads=0;for(const method of ['querySelector','querySelectorAll']){const original=container[method].bind(container);container[method]=(...args)=>{treeReads++;return original(...args)}}
 const source=process.env.R252_GAMEPLAY_SOURCE ? require('node:fs').readFileSync(process.env.R252_GAMEPLAY_SOURCE,'utf8') : read('gameplay.js');
 const seen=[];const {hydrateVisibleMatchTags}=compile(source,['hydrateVisibleMatchTags'],{
  state:{controllers:new Map()},matchTagsBackgroundAllowed:()=>true,matchTierScrollRoot:()=>null,
  matchTierNodeIsVisible:node=>!!node,matchSubject:match=>match.participants?.[0],matchTimelineKey:match=>String(match.gameId),ensureMatchTimeline:async match=>{seen.push(match.gameId)},
 });
 const tab={data:{matches,player:{playerRef:'synthetic'}}};hydrateVisibleMatchTags(container,tab);
 await new Promise(setImmediate);
 assert.deepEqual(seen,matches.slice(5,200).map(m=>m.gameId));assert.equal(tab.matchTagsHydrating,false);
 assert.ok(treeReads<=1,`full-tree reads=${treeReads} for 200 cards`);dom.window.close();
});

test("R252 arena detail and unknown-result fallback preserve partial and larger team counts",()=>{
 const source=process.env.R252_GAMEPLAY_SOURCE ? require('node:fs').readFileSync(process.env.R252_GAMEPLAY_SOURCE,'utf8') : read('gameplay.js');
 const {arenaPlacementResult}=compile(source,['arenaPlacementResult']);
 for(const queueId of [1700,1710,1701,1704,1720,1731,1732,1740])assert.equal(arenaPlacementResult({queueId,participants:[{subteamId:1}]},4),'win',`known queue ${queueId}`);
 for(const [queueId,teams,placement,want] of [[1750,3,3,'win'],[1750,3,4,'loss'],[1700,3,4,'win'],[1710,10,5,'win'],[1750,10,6,'loss'],[0,3,2,'win'],[0,0,1,'unknown'],[1750,3,0,'unknown']]) {
  const match={queueId,participants:Array.from({length:teams},(_,i)=>({subteamId:i+1}))};
  assert.equal(arenaPlacementResult(match,placement),want,`${queueId}/${teams}/${placement}`);
 }
});
test("R252 specialist upstream timeout keeps OPGG, pro and client backup selectable",()=>{
 const data={clientRecommendation:{title:'synthetic-client'}};
 const target={key:'synthetic-target',position:'mid'},payload={positions:['mid'],runes:{opgg:[{title:'synthetic-opgg'}]}};
 const state={live:data,runeSourceTab:'specialist',specialistRuneFailures:new Map([[target.key,{reason:'upstream-timeout',at:1}]]),specialistRunes:new Map(),proRunes:new Map(),proRuneFlights:new Map()};
 const {renderRuneRecommendations}=compile(read('gameplay.js'),['specialistRuneFailure','renderRuneRecommendations','renderRuneSourceSection'],{
  state,escapeHTML,liveRecommendationsFor:()=>payload,liveRecommendationTarget:()=>target,specialistRequestTarget:()=>target,
  recommendationQueueHasTopPlayers:()=>true,proRequestTarget:()=>target,proRunesFor:()=>[{title:'synthetic-pro'}],specialistRunesFor:()=>[],specialistRuneFlightActive:()=>false,
  positionLabel:v=>v,livePositionDisplay:v=>v,renderSpecialistPlayers:items=>items.map(v=>v.title).join(','),renderRuneChoice:v=>v.title,
  recommendedRuneSpellIDs:()=>[],renderRuneSpellPair:()=>'',
 });
 const timeout=renderRuneRecommendations(data,true);
 assert.match(timeout,/韩服接口响应超时/);assert.match(timeout,/data-retry-specialist-runes/);
 for(const key of ['opgg','pro','specialist'])assert.match(timeout,new RegExp(`data-rune-source="${key}"`));
 for(const [key,title] of [['opgg','synthetic-opgg'],['pro','synthetic-pro']]) {
  state.runeSourceTab=key;const html=renderRuneRecommendations(data,true);assert.match(html,new RegExp(title));assert.doesNotMatch(html,/韩服接口响应超时|data-retry-specialist-runes/);
 }
 state.runeSourceTab='opgg';payload.runes={};
 assert.doesNotMatch(renderRuneRecommendations(data,true),/客户端内置（备用）/);
 payload.positions=[];assert.match(renderRuneRecommendations(data,true),/客户端内置（备用）/);
});
