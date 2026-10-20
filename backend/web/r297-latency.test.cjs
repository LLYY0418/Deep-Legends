'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(__dirname+'/gameplay.js','utf8');
const deferred=()=>{let resolve,reject;const promise=new Promise((r,j)=>{resolve=r;reject=j});return {promise,resolve,reject}};
function fixture(api){
 let now=1000;
 const state={liveGameGeneration:1,live:{phase:'ChampSelect',championId:1}},renders=[];
 const target=data=>({key:String(data.championId),championId:data.championId,clientPosition:'mid',mapId:11});
 const f=compile(source,['ensureFastClientRecommendation'],{state,api,liveRecommendationTarget:target,Date:{now:()=>now},ensurePerks(){},renderLive:kind=>renders.push(kind),noteChampSelectLatency(){}});
 return {...f,state,renders,advance:ms=>{now+=ms}};
}
test('R297 fast client requests single-flight, cache results and cool down failed targets',async()=>{
 const pending=deferred();let reads=0,fail=true;
 const h=fixture(async()=>{reads++;if(reads===1)return pending.promise;if(fail)throw Error('409');return {championId:2,clientRecommendation:{championId:2}}});
 const first=h.ensureFastClientRecommendation(h.state.live);
 await h.ensureFastClientRecommendation(h.state.live);assert.equal(reads,1);
 pending.resolve({championId:1,clientRecommendation:{championId:1}});await first;
 await h.ensureFastClientRecommendation(h.state.live);assert.equal(reads,1);assert.equal(h.state.live.clientRecommendation.championId,1);
 h.state.live={phase:'ChampSelect',championId:2};await h.ensureFastClientRecommendation(h.state.live);
 await h.ensureFastClientRecommendation(h.state.live);assert.equal(reads,2);assert(h.state.clientRuneFailures.has('2'));
 h.advance(60001);fail=false;await h.ensureFastClientRecommendation(h.state.live);
 assert.equal(reads,3);assert(!h.state.clientRuneFailures.has('2'));assert.equal(h.state.live.clientRecommendation.championId,2);
});
test('R297 an old client response cannot render or remove the new generation flight',async()=>{
 const old=deferred(),fresh=deferred();let reads=0;
 const h=fixture(()=>++reads===1?old.promise:fresh.promise);
 const first=h.ensureFastClientRecommendation(h.state.live);
 h.state.liveGameGeneration++;h.state.clientRuneFlights.clear();h.state.clientRuneCache.clear();
 const second=h.ensureFastClientRecommendation(h.state.live);
 old.resolve({championId:1,clientRecommendation:{id:'old'}});await first;
 assert.equal(h.state.clientRuneFlights.get('1'),2);assert.equal(h.state.clientRuneCache.size,0);assert.equal(h.renders.length,0);
 fresh.resolve({championId:1,clientRecommendation:{id:'new'}});await second;
 assert.equal(h.state.live.clientRecommendation.id,'new');assert.equal(h.state.clientRuneFlights.size,0);
});
test('R297 empty client results cool down and non-selection snapshots issue no request',async()=>{
 let reads=0;const h=fixture(async()=>{reads++;return {championId:1}});
 await h.ensureFastClientRecommendation({phase:'InProgress'});assert.equal(reads,0);
 await h.ensureFastClientRecommendation(h.state.live);await h.ensureFastClientRecommendation(h.state.live);
 assert.equal(reads,1);assert(h.state.clientRuneFailures.has('1'));
});
test('R297 ready diagnostics count each source once per selection and reject stale targets',async()=>{
 const h=fixture(async()=>({championId:1,clientRecommendation:{championId:1}}));
 const samples=[];h.state.champSelectLite={version:1};h.state.champSelectRecommendationStarted=10;
 const f=compile(source,['ensureFastClientRecommendation','noteChampSelectReady'],{state:h.state,api:async()=>({championId:1,clientRecommendation:{championId:1}}),liveRecommendationTarget:d=>({key:String(d.championId),championId:d.championId,clientPosition:'mid',mapId:11}),ensurePerks(){},renderLive(){},noteChampSelectLatency:(stage,started)=>samples.push({stage,started})});
 for(let i=0;i<26;i++)await f.ensureFastClientRecommendation(h.state.live);
 assert.deepEqual(samples,[{stage:'client_ready',started:10}]);
 h.state.champSelectLite.version=2;h.state.champSelectRecommendationStarted=20;
 await f.ensureFastClientRecommendation(h.state.live);assert.equal(samples.length,2);assert.equal(samples[1].started,20);
 h.state.live={phase:'ChampSelect',championId:2};h.state.champSelectLite.version=3;
 await f.ensureFastClientRecommendation({phase:'ChampSelect',championId:1});
 f.noteChampSelectReady('build_ready',false);assert.equal(samples.length,2,'old hero responses must not pollute current timings');
 f.noteChampSelectReady('build_ready',true);f.noteChampSelectReady('build_ready',true);assert.equal(samples.length,3);
 h.state.live.phase='InProgress';h.state.champSelectLite.version=4;f.noteChampSelectReady('build_ready',true);assert.equal(samples.length,3);
});
