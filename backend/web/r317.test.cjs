'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
test('R317 InProgress recommendation starts within 200ms while roster/history is blocked for 3 seconds',async()=>{
 let release;const history=new Promise(resolve=>release=resolve),calls=[],noop=()=>{};
 const state={live:{phase:'ChampSelect',queueId:1750,gameMode:'CHERRY',mapId:30,players:[{isCurrent:true,championId:154,championLocked:true}]},beacon:{phase:'InProgress'},controllers:new Map()};
 const f=compile(source,['loadLive'],{state,recordLiveRefresh:noop,connected:()=>true,invalidateLiveForNewGame:noop,ensureLiveRecommendations:data=>calls.push({at:performance.now(),data}),liveRenderTriggerLabel:v=>v,updateLiveLoadingVisibility:noop,renderLive:noop,api:()=>history});
 const started=performance.now();const pending=f.loadLive();assert.equal(calls.length,1);assert(calls[0].at-started<200);assert.equal(calls[0].data.phase,'InProgress');
 // The blocked request remains independent; let teardown ignore its obsolete result.
 state.liveRequestToken++;release({});await pending;
});

test('R317 CHERRY recommendation key ignores summoner spell changes across entry',()=>{
 const state={livePositionOverride:new Map(),liveRecommendationSpells:new Map()};
 const f=compile(source,['liveRecommendationTarget','liveRecommendationChampionId'],{state,USE_CHAMPION_PICK_INTENT_FOR_RECOMMENDATIONS:true,liveAugmentRecommendationSource:()=> 'arena',liveRecommendationTier:()=>'',});
 const data={phase:'ChampSelect',queueId:1750,gameMode:'CHERRY',mapId:30,players:[{isCurrent:true,championId:154,championLocked:true,position:'other',spell1Id:4,spell2Id:14}]};
 const a=f.liveRecommendationTarget(data);data.phase='InProgress';data.players[0].spell1Id=0;data.players[0].spell2Id=0;const b=f.liveRecommendationTarget(data);assert.equal(a.key,b.key);assert.match(a.key,/:other:CHERRY:30:.*:none$/);
});
