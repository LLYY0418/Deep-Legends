'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const src=fs.readFileSync(process.env.R181_GAMEPLAY_SOURCE||path.join(__dirname,'gameplay.js'),'utf8');
function extract(name,source=src){let start=source.indexOf(`function ${name}(`);assert.ok(start>=0,name);if(source.slice(start-6,start)==='async ')start-=6;const body=source.indexOf(') {',start)+2;let depth=0,quote='',escaped=false;for(let i=body;i<source.length;i++){const c=source[i];if(quote){if(escaped)escaped=false;else if(c==='\\')escaped=true;else if(c===quote)quote='';continue;}if(c==='"'||c==="'"||c==='`'){quote=c;continue;}if(c==='{')depth++;if(c==='}'&&--depth===0)return source.slice(start,i+1);}throw Error(name);}
function compile(names,deps={}){return Function(...Object.keys(deps),names.map(n=>extract(n)).join('\n')+`\nreturn {${names.join(',')}};`)(...Object.values(deps));}
const flush=()=>new Promise(setImmediate);
function harness({locked=true,own=799,enemyPosition='top',pairMissing=false,pairFailed=false,shares}={}) {
 const data={phase:'ChampSelect',available:true,gameId:181,queueId:440,players:[{isCurrent:true,teamId:100,position:'top',championId:locked?own:0,championPickIntent:own,championLocked:locked,rank:{tier:'GOLD'}},{teamId:200,position:enemyPosition,championId:266,championLocked:true,championName:'敌方'}]};
 const state={live:data,section:'live',liveGameGeneration:1,laneMatchupPairs:new Map(),laneMatchupCandidates:new Map(),laneMatchupLanes:new Map()},calls=[],events=[];
 let tier='emerald_plus',gate;
 const names=['laneMatchupEnemies','laneMatchupInference','laneMatchupContext','ensureLaneMatchupPositions','laneMatchupAvailability','laneMatchupOwnChampionId','laneMatchupTier','laneMatchupOwnLocked','laneMatchupCandidateKey','laneMatchupPairKey','ensureLaneMatchupPair','recordLaneMatchupCandidateSkip','laneMatchupUnavailableReason','recordLaneMatchupCandidateDiagnostic','ensureLaneMatchupCandidates','recordLaneMatchupCardDiagnostic','renderLaneMatchupCard'];
 const f=compile(names,{state,URLSearchParams,livePositionValue:v=>({middle:'mid',bottom:'adc',utility:'support'})[v]||v||'',liveRecommendationTier:()=>tier,renderLive:()=>{},rate:v=>`${Number(v).toFixed(1)}%`,escapeHTML:String,iconFigure:(_,id)=>`<img data-champion="${id}">`,recordItemSetClientDiagnostic:(event,reason,fields)=>events.push({event,reason,...fields}),api:async url=>{
 calls.push(url);const q=new URL(url,'http://local').searchParams;
 if(url.includes('champion-lanes')) {if(gate)await gate;if(shares instanceof Error)throw shares;return shares||{266:[{position:'top',rate:0.8}]};}
 if(url.includes('lane-matchup')) {if(pairFailed)throw Error('failed');return pairMissing?{}:{championId:Number(q.get('champion')),enemyChampionId:Number(q.get('enemy')),winRate:53,games:1000};}
 return {counters:{weakAgainst:[{championId:61,name:'候选',winRate:43,games:100}]}};
 }});
 return {data,state,calls,events,...f,setTier:v=>tier=v,hold:()=>{let resolve;gate=new Promise(r=>resolve=r);return resolve}};
}
test('R181 full pair shows locked A when enemy is absent from both top-five lists',async()=>{
 const h=harness({enemyPosition:''});await h.ensureLaneMatchupCandidates(h.data);
 const html=h.renderLaneMatchupCard(h.data,{weakAgainst:[{championId:1,winRate:40}],strongAgainst:[{championId:2,winRate:60}]});
 assert.match(html,/偏优势 53\.0%/);assert.doesNotMatch(html,/候选/);
 assert.ok(h.events.some(e=>e.event==='lane_matchup_card'&&e.mode==='a'&&e.shown===true&&e.ownLocked===true&&e.tier==='emerald_plus'));
 for(let i=0;i<10;i++)h.renderLaneMatchupCard(h.data,{});assert.equal(h.events.filter(e=>e.event==='lane_matchup_card'&&e.shown).length,1);
 assert.ok(h.calls.every(url=>new URL(url,'http://local').searchParams.get('tier')==='emerald_plus'));
});
test('R181 missing full pair hides A and diagnoses pair-no-data; failure/pending are distinct',async()=>{
 for(const [opts,reason] of [[{pairMissing:true},'pair-no-data'],[{pairFailed:true},'pair-failed']]) {
 const h=harness(opts);assert.equal(h.renderLaneMatchupCard(h.data,{}),'');assert.equal(h.events.at(-1).hiddenReason,'pair-pending');await h.ensureLaneMatchupCandidates(h.data);assert.equal(h.renderLaneMatchupCard(h.data,{}),'');assert.equal(h.events.at(-1).hiddenReason,reason);
 }
 const h=harness();await h.ensureLaneMatchupCandidates(h.data);const pair=[...h.state.laneMatchupPairs.values()][0];pair.data.winRate=50;assert.match(h.renderLaneMatchupCard(h.data,{}),/五五开 50\.0%/);pair.data.winRate=0;assert.match(h.renderLaneMatchupCard(h.data,{}),/偏劣势 0\.0%/);
 const b=harness({own:0,locked:false});assert.equal(b.renderLaneMatchupCard(b.data,{}),'');assert.equal(b.events.at(-1).hiddenReason,'candidates-empty');
});
test('R181 intent and hover show A+B under one enemy heading, locking keeps A and hides B',async()=>{
 const h=harness({locked:false});await h.ensureLaneMatchupCandidates(h.data);let html=h.renderLaneMatchupCard(h.data,{});assert.match(html,/53\.0%/);assert.match(html,/候选/);assert.equal((html.match(/data-champion="266"/g)||[]).length,1);assert.equal((html.match(/data-lane-matchup-card/g)||[]).length,1);assert.equal(h.events.at(-1).mode,'a+b');
 h.data.players[0].championId=799;h.data.players[0].championPickIntent=0;await h.ensureLaneMatchupCandidates(h.data);assert.match(h.renderLaneMatchupCard(h.data,{}),/候选/);
 h.data.players[0].championLocked=true;await h.ensureLaneMatchupCandidates(h.data);html=h.renderLaneMatchupCard(h.data,{});assert.match(html,/53\.0%/);assert.doesNotMatch(html,/候选/);assert.equal(h.events.at(-1).mode,'a');
});
test('R181 sixty-one polls send one pair request and new own/enemy/tier keys each send once',async()=>{
 const h=harness();await Promise.all(Array.from({length:61},()=>h.ensureLaneMatchupCandidates(h.data)));assert.equal(h.calls.filter(u=>u.includes('lane-matchup')).length,1);
 h.data.players[0].championId=800;await h.ensureLaneMatchupCandidates(h.data);await h.ensureLaneMatchupCandidates(h.data);assert.equal(h.calls.filter(u=>u.includes('lane-matchup')).length,2);
 h.data.players[1].championId=267;await h.ensureLaneMatchupCandidates(h.data);assert.equal(h.calls.length,3);
 h.setTier('diamond_plus');await h.ensureLaneMatchupCandidates(h.data);assert.equal(h.calls.length,4);
 h.data.phase='InProgress';await h.ensureLaneMatchupCandidates(h.data);assert.equal(h.calls.length,4);assert.equal(h.renderLaneMatchupCard(h.data,{}),'');
});
test('R181 lane pending, failed, below-threshold and ambiguous diagnoses remain distinguishable',async()=>{
 const pending=harness({enemyPosition:''});const release=pending.hold();const work=pending.ensureLaneMatchupCandidates(pending.data);assert.equal(pending.events.at(-1).reason,'lanes-pending');release();await work;
 for(const [shares,reason]of [[Error('failed'),'lanes-failed'],[{266:[{position:'top',rate:0.1}]},'lane-not-locked']]) {const h=harness({enemyPosition:'',shares});await h.ensureLaneMatchupCandidates(h.data);assert.ok(h.events.some(e=>e.reason===reason));assert.equal(h.calls.filter(u=>u.includes('lane-matchup')).length,0);}
 const h=harness({enemyPosition:'',shares:{266:[{position:'top',rate:0.6}],267:[{position:'top',rate:0.4}]}});h.data.players.push({teamId:200,championId:267,championLocked:true});await h.ensureLaneMatchupCandidates(h.data);assert.ok(h.events.some(e=>e.reason==='lane-ambiguous'));
});
test('R181 direct sender and actual runtime carry card booleans/tier and omit identity',async()=>{
 const sent=[];const fields={mode:'a+b',shown:false,hiddenReason:'pair-no-data',ownLocked:false,enemyChampionId:266,tier:'emerald_plus',playerRef:'private-player',puuid:'private-puuid'};
 compile(['recordItemSetClientDiagnostic'],{fetch:async(_,o)=>{sent.push(JSON.parse(o.body));return {ok:true}}}).recordItemSetClientDiagnostic('lane_matchup_card','render',fields);await flush();
 const dom=new JSDOM('<body></body>',{url:'http://localhost'});vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),{window:dom.window,document:dom.window.document,fetch:async(_,o)=>{sent.push(JSON.parse(o.body));return {ok:true,status:204}},setTimeout:()=>1,clearTimeout:()=>{},setInterval:()=>1,clearInterval:()=>{},Date,URL,URLSearchParams,AbortController,console});
 dom.window.reportFlowDiagnostic('lane_matchup_card','render',fields);dom.window.reportFlowDiagnostic('lane_matchup_candidate_fetch','lanes-pending',{position:'top',tier:'emerald_plus',selfPosition:'top',enemyLockedCount:4});await flush();
 assert.equal(sent.length,3);for(const body of sent.slice(0,2)){assert.equal(body.mode,'a+b');assert.equal(body.shown,false);assert.equal(body.ownLocked,false);assert.equal(body.hiddenReason,'pair-no-data');assert.equal(body.enemyChampionId,266);assert.equal(body.tier,'emerald_plus');}assert.doesNotMatch(JSON.stringify(sent),/private|puuid|playerRef/);
 if(process.env.R181_WRITE_DIAGNOSTICS)fs.writeFileSync(process.env.R181_WRITE_DIAGNOSTICS,JSON.stringify(sent));dom.window.close();
});
