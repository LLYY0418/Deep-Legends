'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {fixture}=require('./r190-harness.cjs');
test('P21 build spells follow selected participant independently of timeline and mode',()=>{
 for(const mode of ['CLASSIC','ARAM','ARAM_MAYHEM','CHERRY'])for(const available of [true,false]){
  const f=fixture(undefined,{renderSkillOrder:()=>'<div class="match-skill-seq"><span class="match-skill-cell">Q</span></div>',skillPrioritySummary:()=>'<span class="match-skill-priority">主升 Q · 副升 W</span>'});
  const a={...f.subject,spell1Id:4,spell2Id:14},b={...f.subject,participantId:5,spell1Id:6,spell2Id:7};
  const match={gameId:21,gameMode:mode,participants:[a,b]},tab={buildPlayers:new Map([['21',5]])};
  f.state.matchTimelines.set('fixture',{available,skillOrder:available?[{slot:1,level:1},{slot:2,level:2}]:[]});
  const {JSDOM}=require('../../desktop/node_modules/jsdom'),d=new JSDOM(f.renderBuild(match,a,tab)).window.document;
  assert.deepEqual([...d.querySelectorAll('.build-spells [data-spell-id]')].map(n=>Number(n.dataset.spellId)),[6,7]);assert.equal(d.querySelectorAll('.build-skill-body').length,1);
 }
 const f=fixture();const a={...f.subject,spell1Id:0,spell2Id:0};assert(!f.renderBuild({gameId:22,gameMode:'CHERRY',participants:[a]},a,{}).includes('data-spell-id'));
 assert.equal((f.renderBuild({gameId:22,gameMode:'CLASSIC',participants:[a]},a,{}).match(/match-loadout-slot is-empty/g)||[]).length,0);
});
test('P22 retries 3/10/30 seconds, never poisons temporary failure and updates only augment cards',async()=>{
 let calls=0;const timers=[],reports=[];
 const f=fixture(undefined,{api:async()=>{calls++;return{items:[{id:120,status:calls<5?'retry':'ok',description:calls<5?'':'恢复后的效果',source:'cache',error_kind:'backoff_skipped'}]};},setTimeout:(fn,ms)=>{const t={fn,ms};timers.push(t);return t;},window:{reportFlowDiagnostic:(...r)=>reports.push(r)}});
 const d=f.deps.document;d.body.innerHTML='<div class="build-detail"><section id="skills"><input value="retained"></section><div class="aug-grid">'+f.renderBuildAugments([120]).match(/<article[\s\S]*<\/article>/)[0]+'</div></div>';const skills=d.querySelector('#skills');
 await f.ensureAugmentDescriptions([120]);assert.equal(calls,1);await f.ensureAugmentDescriptions([120]);assert.equal(calls,1);
 for(let i=0;i<3;i++){const timer=timers[i];assert.equal(timer.ms,[3000,10000,30000][i]);timer.fn();await new Promise(r=>setImmediate(r));}
 assert.equal(timers.length,3);assert.equal(calls,4);assert(reports.at(-1)[2].gave_up);assert.equal(f.state.augmentDescriptions.get(120).status,'retry');
 await f.ensureAugmentDescriptions([120],true);assert.equal(calls,5);assert.equal(f.state.augmentDescriptions.get(120).status,'ok');assert(d.body.textContent.includes('恢复后的效果'));assert.equal(d.querySelector('#skills'),skills);
 assert(!d.body.textContent.includes('重试'));assert.equal(timers.length,3);
});
test('P22 healthy unavailable is terminal, HTTP/no-response failures are retryable',async()=>{
 let calls=0;const f=fixture(undefined,{api:async()=>{calls++;return{items:[{id:120,status:'unavailable'}]};}});await f.ensureAugmentDescriptions([120]);await f.ensureAugmentDescriptions([120],true);assert.equal(calls,1);assert.equal(f.state.augmentDescriptions.get(120).status,'unavailable');
});
test('P21 spell shapes are anonymous and emitted once per mode',()=>{
 const reports=[],f=fixture(undefined,{matchModeKind:m=>m.mode,window:{reportFlowDiagnostic:(...r)=>reports.push(r)}});
 for(const mode of ['arena','mayhem','standard','aram'])for(let i=0;i<2;i++)f.recordBuildSpellShape({mode,participants:[{spell1Id:4,spell2Id:0},{spell1Id:0},{spell2Id:7}]},{});
 assert.equal(reports.length,4);for(const [event,reason,fields] of reports){assert.equal(event,'build_spells_client');assert.deepEqual(Object.keys(fields).sort(),['mode','spell1Id_missing','spell1Id_value','spell1Id_zero','spell2Id_missing','spell2Id_value','spell2Id_zero'].sort());for(const k of Object.keys(fields).filter(k=>k!=='mode'))assert.equal(fields[k],1);}
});
test('P22 newly seen augment retains its own retry budget beside an exhausted ID',async()=>{
 const calls=[],timers=[];const f=fixture(undefined,{api:async url=>{const query=new URL(url,'http://fixture').searchParams;calls.push({ids:query.get('ids'),retry:Number(query.get('retry'))});return{items:query.get('ids').split(',').map(id=>({id:Number(id),status:'retry',error_kind:'no_response'}))};},setTimeout:(fn,ms)=>{const t={fn,ms};timers.push(t);return t;}});
 f.state.augmentDescriptions.set(120,{status:'retry',retryCount:3});await f.ensureAugmentDescriptions([120,1225],true);assert.deepEqual(calls,[{ids:'120',retry:4},{ids:'1225',retry:0}]);assert.equal(timers.length,1);assert.equal(timers[0].ms,3000);
});
test('P22 retained build rebinding is not a retry; reopening retries the selected player',async()=>{
 const calls=[],f=fixture(undefined,{api:async url=>{calls.push(url);return{items:[{id:1225,status:'retry',error_kind:'no_response'}]};}});
 const a={...f.subject,augmentIds:[120]},b={...f.subject,participantId:5,augmentIds:[1225]},match={gameId:21,subjectParticipantId:4,participants:[a,b]},tab={buildPlayers:new Map([['21',5]])};
 f.state.augmentDescriptions.set(120,{status:'ok',description:'self'});f.state.augmentDescriptions.set(1225,{status:'retry',retryCount:3});
 await f.ensureBuildData(match,tab);await new Promise(r=>setImmediate(r));assert.equal(calls.length,0);
 await f.ensureBuildData(match,tab,true);await new Promise(r=>setImmediate(r));assert.equal(calls.length,1);assert(calls[0].includes('ids=1225'));assert(calls[0].includes('retry=4'));
});
