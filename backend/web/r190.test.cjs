'use strict';
const test=require('node:test'), assert=require('node:assert/strict'), fs=require('node:fs'), path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {fixture}=require('./r190-harness.cjs');
const dom=html=>new JSDOM(html).window.document;
function players(n){return Array.from({length:n},(_,i)=>({participantId:i+1,teamId:i<5?100:200,gameName:`召唤师 ${i+1}`,tagLine:'测试',championId:i+1,playerRef:`player${i+1}`}));}
test('R190 1–4 roster marks only subject, including Arena and masking',()=>{
 const f=fixture();
 for(const n of [10,16]) {const match={participants:players(n).map((p,i)=>({...p,...(n===16?{subteamId:Math.floor(i/2)+1,placement:8-Math.floor(i/2)}:{})})),subjectParticipantId:4,gameMode:n===16?'CHERRY':''};const d=dom(f.renderMatchPlayers(match));assert.equal(d.querySelectorAll('.match-player-name.is-current-player').length,1);assert.equal(d.querySelector('.is-current-player').textContent,n===16?'召唤师 4':'召唤师 4#测试');}
 for(const subjectParticipantId of [undefined,0]) assert.equal(dom(f.renderMatchPlayers({participants:players(10),subjectParticipantId})).querySelectorAll('.is-current-player').length,0);
 f.state.settings.maskNames=true;const d=dom(f.renderMatchPlayers({participants:players(10),subjectParticipantId:4}));assert.equal(d.querySelector('.is-current-player').textContent,'玩家 04');
});
test('R190 5–9 effect parser handles zero, labels, time, invalid templates and overrides',()=>{
 const f=fixture();assert.deepEqual(f.perkEffectLines(f.perks[1],[804,300,0]),[{label:'回复生命值',value:804,kind:'heal'},{label:'额外金币',value:300,kind:'gold'}]);assert.equal(f.perkEffectLines(f.perks[2],[19,5,0])[0].value,'19:05');
 assert.equal(f.perkEffectLines({eogDescs:['护盾的总和：@eogvar1@']},[0,0,0])[0].value,0);
 for(const eogDescs of [['--'],[''],['时间：@eogvar1@:@eogvar2@@eogvar3@']]) assert.deepEqual(f.perkEffectLines({eogDescs},[1,2,3]),[]);
 for(const id of [8008,8304]) assert.deepEqual(f.perkEffectLines({id,eogDescs:['最大攻速运转时间：@eogvar1@:@eogvar2@<br>已造成的伤害：@eogvar2@']},[3,1028,0]),[]);
 assert.equal(f.perkEffectLines({eogDescs:['回复生命总和：@eogvar1@<BR/>金币总计：@eogvar2@']},[2,3,0]).length,2);
});
test('R190 10–11 slot order, max-per-perk totals and missing stats use descriptions',()=>{
 const f=fixture();assert.deepEqual(f.selectedRuneEffects(f.subject).map(r=>r.perk.id),[8005,9111,9103,8017,8321,8347]);assert.deepEqual(f.runeEffectTotals(f.selectedRuneEffects(f.subject)),{damage:2300,heal:804,gold:680});
 const d=dom(f.renderRuneEffects(f.subject));assert.equal(d.querySelectorAll('.eff').length,6);assert.equal(d.querySelectorAll('.shard-chip').length,3);assert.doesNotMatch(d.body.textContent,/--|@eogvar/);assert.match(d.body.textContent,/固定效果 8347/);
 const missing={...f.subject,perkStats:undefined};const no=dom(f.renderRuneEffects(missing));assert.equal(no.querySelectorAll('.eff .name small').length,6);assert.equal(no.querySelectorAll('.stat').length,0);assert.equal(f.renderRuneYield(missing),'');f.state.perks=null;assert.equal(f.renderRuneEffects(f.subject),'');
});
test('R190 12 shared board matches pre-R190 snapshot after removing data attribute',()=>{
 const f=fixture();assert.equal(f.renderUnifiedRuneBoard(f.subject).replace(/ data-perk-id="\d+"/g,''),fs.readFileSync(path.join(__dirname,'../testdata/r190-rune-board.html'),'utf8'));
});
test('R190 13 real delegated pointer/focus links selected icons only without rerender',()=>{
 const f=fixture();const d=dom(`<div class="rune-split"><div>${f.renderUnifiedRuneBoard(f.subject)}</div>${f.renderRuneEffects(f.subject)}</div>`);f.bindRuneEffectLinks(d.body);const row=d.querySelector('.eff[data-perk-id="9111"]'),tree=d.querySelector('.rune-option-button[data-perk-id="9111"]'),html=d.body.innerHTML;
 row.dispatchEvent(new d.defaultView.Event('pointerover',{bubbles:true}));assert.ok(tree.classList.contains('is-linked'));assert.equal(d.querySelectorAll('.is-linked').length,1);row.dispatchEvent(new d.defaultView.Event('pointerout',{bubbles:true}));assert.equal(d.querySelectorAll('.is-linked').length,0);
 tree.dispatchEvent(new d.defaultView.Event('focusin',{bubbles:true}));assert.ok(row.classList.contains('is-hover'));tree.dispatchEvent(new d.defaultView.Event('focusout',{bubbles:true}));assert.equal(d.querySelectorAll('.is-hover').length,0);assert.equal(d.body.innerHTML,html);
 const off=d.querySelector('.rune-option-button[data-perk-id="8008"]');off.dispatchEvent(new d.defaultView.Event('pointerover',{bubbles:true}));assert.equal(d.querySelectorAll('.is-hover').length,0);
});
test('R190 20–21 augment order, rarity, skeleton, bare and escaped full tooltip',()=>{
 const f=fixture();f.state.augmentDescriptions.set(120,{status:'ok',description:'效果 <b>30%</b>'});f.state.augmentDescriptions.set(1004,{status:'unavailable'});const d=dom(f.renderBuildAugments([120,1225,1116,1004]));assert.deepEqual([...d.querySelectorAll('.aug-order')].map(n=>n.textContent),['1','2','3','4']);assert.deepEqual([...d.querySelectorAll('.aug')].map(n=>n.className),['aug is-silver','aug is-prismatic','aug is-gold','aug is-gold is-bare']);assert.equal(d.querySelectorAll('.skel').length,4);assert.doesNotMatch(d.querySelector('.is-bare').textContent,/暂无|说明/);assert.match(d.querySelector('.aug').dataset.tooltip,/效果 30%/);assert.equal(d.querySelectorAll('p b').length,0);
});
test('R190 22 description IDs reused, failures settled, no repeated request',async()=>{
 let calls=0,rerenders=0;const f=fixture(undefined,{api:async()=>{calls++;return {items:[{id:120,status:'ok',description:'真实效果'}]};},rerenderCatalogViews:()=>rerenders++});await Promise.all([f.ensureAugmentDescriptions([120,1225]),f.ensureAugmentDescriptions([120])]);await f.ensureAugmentDescriptions([120,1225]);assert.equal(calls,1);assert.equal(rerenders,1);assert.equal(f.state.augmentDescriptions.get(1225).status,'unavailable');assert.match(f.augmentIconFigure(120),/真实效果/);
 const fail=fixture(undefined,{api:async()=>{throw Error('offline')},rerenderCatalogViews:()=>{}});await fail.ensureAugmentDescriptions([120]);await fail.ensureAugmentDescriptions([120]);assert.equal(fail.state.augmentDescriptions.get(120).status,'unavailable');
});
test('R190 old match requests exactly one refresh across views, preserves participant metadata',async()=>{
 let calls=0;const f=fixture(undefined,{api:async()=>{calls++;return {participants:[{participantId:4,perkStats:[{perkId:9111,vars:[804,300,0]}]}]};},rerenderMatch:()=>{}});const match={gameId:1,subjectParticipantId:4,perkStatsStale:true,participants:[{...f.subject,perkStats:undefined,championName:'测试英雄'}]};await Promise.all([f.ensureBuildData(match,{}),f.ensureBuildData(match,{})]);assert.equal(calls,1);assert.equal(match.perkStatsStale,false);assert.equal(match.participants[0].championName,'测试英雄');assert.equal(match.participants[0].perkStats[0].vars[0],804);
 const second={...match,perkStatsStale:true,participants:[{...f.subject,perkStats:undefined}]};await f.ensureBuildData(second,{});assert.equal(calls,1);assert.equal(second.perkStatsStale,false);
 let fails=0;const failed=fixture(undefined,{api:async()=>{fails++;throw Error('quota')},rerenderMatch:()=>{}});const stale={...match,perkStatsStale:true};await failed.ensureBuildData(stale,{});await failed.ensureBuildData(stale,{});assert.equal(fails,1);assert.equal(stale.perkStatsStale,true);
});

test('R190 build controls request augment copy only after build opens',async()=>{
 const {compile,read}=require('./r188-harness.cjs');let calls=0;
 const f=fixture(undefined,{api:async()=>{calls++;return{items:[{id:120,status:'ok',description:'效果'}]}},rerenderCatalogViews:()=>{}});
 const match={gameId:190,subjectParticipantId:4,participants:[{...f.subject,augmentIds:[120]}]};
 const tab={openMatches:new Set(),matchDetailTabs:new Map(),data:{matches:[match]}};
 const d=dom('<button data-match-detail="overview" data-game-id="190">概览</button><button data-match-detail="build" data-game-id="190">构建</button>');
 const {bindMatchDetailControls}=compile(read('gameplay.js'),['bindMatchDetailControls'],{...f.deps,...f,ensurePerks:()=>{},ensureItems:()=>{},ensureMatchTimeline:()=>{},rerenderMatch:()=>{},TEAM_ANALYSIS_METRICS:[]});
 bindMatchDetailControls(d.body,tab);assert.equal(calls,0);d.querySelector('[data-match-detail="overview"]').click();assert.equal(calls,0);d.querySelector('[data-match-detail="build"]').click();await new Promise(r=>setImmediate(r));assert.equal(calls,1);d.querySelector('[data-match-detail="build"]').click();await new Promise(r=>setImmediate(r));assert.equal(calls,1);
 const zero={...f.subject,perkStats:f.subject.perkStats.map(p=>({...p,vars:[0,0,0]}))};assert.equal(f.renderRuneYield(zero),'');
});
