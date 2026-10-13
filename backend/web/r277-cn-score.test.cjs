'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs');
const game=fs.readFileSync(process.env.R277_CN_GAMEPLAY_SOURCE||__dirname+'/gameplay.js','utf8');
function fixture({region='',serverId='HN10',current=false,playerRef='opaque-cn',respond,group='players',badge=false}={}){
 const dom=new JSDOM('<article class="match-entry" data-match-id="277"><i class="match-score" data-score-participant="1"></i><button data-toggle-match="277"></button></article>'),root=dom.window.document.querySelector('article'),requests=[],match={gameId:277,subjectParticipantId:1,participants:[{participantId:1,playerRef,score:{value:8}}]},tab={key:'fixture',group,current,region,serverId,data:{matches:[match],player:{playerRef,serverId,region}},openMatches:new Set(),matchDetailTabs:new Map()};
 if(badge)root.querySelector('i').outerHTML='<span data-match-tag-pinned><b class="match-badge-chip" data-tooltip="评分排名">MVP</b></span>';
 const api=compile(game,['bindScoreDetails','ensureScoreDetails','bindMatchEntryControls'],{document:dom.window.document,window:{dispatchEvent(){}},CustomEvent:dom.window.CustomEvent,riotTab:t=>Boolean(t.region),matchSubject:m=>m.participants[0],overviewContainer:()=>root,api:async(url,options)=>{requests.push({url,...JSON.parse(options.body)});return respond?respond():{gameId:277,subjectParticipantId:1,participants:[{participantId:1,playerRef,score:{value:8,parts:[{key:'damage',value:1,norm:.5}]}}]};},resetBuildPlayerSelection(){},ensureMatchTimeline(){},replaceMatchEntry(){}});
 return {dom,root,requests,match,tab,api,close:()=>dom.window.close()};
}
for(const [kind,options] of [['cn',{}],['self',{current:true}],['riot',{region:'kr',serverId:'',playerRef:'opaque-riot'}],['pro',{region:'kr',serverId:'',playerRef:'opaque-pro',group:'pro'}]])for(const trigger of ['hover','expand','badge'])test(`${kind} ${trigger} requests scoped lazy parts once and retains decoded score`,async()=>{
 const h=fixture({...options,badge:trigger==='badge'});try{
  h.api.bindScoreDetails(h.root,h.tab);h.api.bindMatchEntryControls(h.root,h.tab,()=>{});assert.equal(h.requests.length,0);
  const score=h.root.querySelector('[data-score-participant]');assert(score,'visible score badge must bind lazy details');
  if(trigger!=='expand')score.dispatchEvent(new h.dom.window.Event('pointerenter'));else h.root.querySelector('button').click();
  await new Promise(setImmediate);assert.equal(h.requests.length,1);assert.equal(h.requests[0].refresh,'score-parts');assert.equal(h.requests[0].serverId,options.serverId??'HN10');assert.equal(h.requests[0].playerRef,options.playerRef??'opaque-cn');assert.equal(h.requests[0].participantId,1);assert.equal(h.match.participants[0].score.parts.length,1);
  score.dispatchEvent(new h.dom.window.Event('focus'));await new Promise(setImmediate);assert.equal(h.requests.length,1);assert(JSON.parse(score.dataset.tooltipScore).parts.length);
 }finally{h.close();}
});
for(const mismatch of ['game','subject','player','server-changed','account-changed'])test(`lazy score rejects ${mismatch} response or stale tab identity`,async()=>{
 let finish;const h=fixture({respond:()=>new Promise(r=>finish=r)});try{const promise=h.api.ensureScoreDetails(h.match,h.tab);await new Promise(setImmediate);assert.equal(h.requests.length,1);const response={gameId:277,subjectParticipantId:1,participants:[{participantId:1,playerRef:'opaque-cn',score:{value:9,parts:[{key:'damage',value:1,norm:.5}]}}]};if(mismatch==='game')response.gameId=278;if(mismatch==='subject')response.subjectParticipantId=2;if(mismatch==='player')response.participants[0].playerRef='other';if(mismatch==='server-changed')h.tab.serverId=h.tab.data.player.serverId='HN1';if(mismatch==='account-changed')h.tab.data.player.playerRef='other';finish(response);await promise;assert.equal(h.match.participants[0].score.parts,undefined);assert(!h.match.scorePartsLoaded);
 }finally{h.close();}
});
