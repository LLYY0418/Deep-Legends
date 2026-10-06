'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R211_GAMEPLAY_SOURCE || path.join(__dirname,'gameplay.js'),'utf8');
function dirtyHarness({hidden=false,present=false,gameId='9014822625'}={}) {
 let now=100000,serial=0;const jobs=new Map(),events=[],requests=[];
 const tab={current:true,key:'self',matchFilter:'flex',latestAllGameId:'100',data:{matches:[{gameId:100}],pagination:{filter:'flex'}},matchViews:new Map([['all',{}]])};
 const state={tabs:[tab],section:'overview',live:{gameId:9014822625,queueId:2400,players:[]},liveExpectedGameId:9014822625};
 const document={hidden},window={reportFlowDiagnostic:(event,reason,fields)=>events.push({event,reason,...fields})};
 const f=compile(source,['markOverviewAfterGame','scheduleDirtyOverview','reportDirtyOverview','shouldReloadOverview'],{state,document,window,Date:{now:()=>now},riotTab:()=>false,connected:()=>true,activeTab:()=>tab,
  clearTimeout:id=>jobs.delete(id),setTimeout:(fn,delay)=>{jobs.set(++serial,{fn,delay});return serial},loadOverview:async(t,force)=>{requests.push({force,gameId:t.dirtyGameId,filter:t.matchFilter});t.expectedGamePresent=present;t.latestAllGameId=present?'9014822625':'100';t.loadedAt=now;return true;}});
 return {f,tab,state,document,jobs,events,requests,mark:()=>f.markOverviewAfterGame('WaitingForStats',gameId,2400),next:async()=>{const [id,job]=jobs.entries().next().value;jobs.delete(id);now+=job.delay;await job.fn();return job.delay},advance:ms=>now+=ms,now:()=>now};
}
test('R211 flex after mayhem resolves from expected ID, clears all cached views and does not retry',async()=>{
 const h=dirtyHarness({present:true});h.mark();assert.equal(h.tab.matchViews.size,0);assert.equal(await h.next(),7000);
 assert.equal(h.tab.dirty,false);assert.equal(h.jobs.size,0);assert.equal(h.requests.length,1);
 assert.deepEqual(h.requests[0],{force:true,gameId:'9014822625',filter:'flex'});
 assert.equal(h.events.at(-1).outcome,'resolved');assert.equal(h.events.at(-1).finished_queue_id,2400);
});
test('R211 five delays give up and restore ordinary 120-second reload',async()=>{
 const h=dirtyHarness();h.mark();const delays=[];while(h.jobs.size)delays.push(await h.next());
 assert.deepEqual(delays,[7000,15000,30000,60000,120000]);assert.equal(h.tab.dirty,false);assert.equal(h.tab.dirtyAttempts,5);
 assert.equal(h.events.at(-1).outcome,'gave_up');h.advance(120001);assert.equal(h.f.shouldReloadOverview(h.tab,h.now()),true);
});
test('R211 background pauses; foreground resumes immediately with all-history fallback',async()=>{
 const h=dirtyHarness({hidden:true,present:true,gameId:'0'});h.state.live=null;h.state.liveExpectedGameId=0;h.mark();
 assert.equal(h.jobs.size,0);assert.equal(h.events.at(-1).outcome,'paused_hidden');h.advance(60000);h.document.hidden=false;h.f.scheduleDirtyOverview(h.tab);
 assert.equal(await h.next(),0);assert.equal(h.tab.dirty,false);assert.equal(h.events.at(-1).outcome,'resolved');
});
test('R211 load request and identity cache are explicitly wired to the expected game',()=>{
 assert.match(source,/expectGameId: String\(tab.dirtyGameId/);assert.match(source,/&expectGameId=/);
 assert.match(source,/tab.expectedGamePresent = payload.expectedGamePresent === true/);
 const runtime=fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8');assert.match(runtime,/"overview_dirty_rescan"/);assert.match(runtime,/body\.attempt/);assert.match(runtime,/body\.finished_game_id/);
});
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {escapeHTML}=require('./r188-harness.cjs');
test('R211 build selection switches equipment skills rune effects and augments, defaults to self',()=>{
 const state={matchTimelines:new Map()},tab={key:'one'},match={gameId:9,participants:[{participantId:1,teamId:100,championId:1,championName:'一',championLevel:18,gameName:'Self',perkIds:[11]},{participantId:2,teamId:200,championId:2,championName:'二',championLevel:17,hidden:true,perkIds:[22]}]};
 state.matchTimelines.set('9',{available:true,participants:match.participants.map(p=>({participantId:p.participantId,itemGroups:[{minute:1,events:[{itemId:100+p.participantId}]}],skillOrder:[{slot:p.participantId,level:1}]}))});
 const f=compile(source,['buildSubject','renderBuildPlayers','matchPlayerGroups','renderBuild'],{state,escapeHTML,number:String,ensureAugmentDescriptions:()=>{},matchTimelineKey:()=> '9',riotTab:()=>false,connected:()=>true,matchAugmentIDs:p=>p.augmentIds||[],iconFigure:(_kind,id)=>`<img src="/${id}">`,renderBuildAugments:ids=>`hex:${ids.join(',')}`,renderUnifiedRuneBoard:p=>`runes:${p.perkIds}`,renderRuneEffects:p=>`effects:${p.perkIds}`,renderRuneYield:p=>`yield:${p.perkIds}`,renderItemRoute:t=>`item:${t.itemGroups[0].events[0].itemId}`,renderSkillOrder:t=>`skill:${t.skillOrder[0].slot}`,skillPrioritySummary:()=>''});
 let html=f.renderBuild(match,match.participants[0],tab);assert.match(html,/item:101/);assert.match(html,/skill:1/);assert.match(html,/runes:11/);assert.match(html,/effects:11/);assert.match(html,/yield:11/);
 tab.buildPlayers=new Map([['9',2]]);html=f.renderBuild(match,match.participants[0],tab);assert.match(html,/item:102/);assert.match(html,/skill:2/);assert.match(html,/runes:22/);assert.match(html,/effects:22/);assert.match(html,/yield:22/);assert.match(html,/隐藏玩家/);
 match.participants[1].augmentIds=[33];assert.match(f.renderBuild(match,match.participants[0],tab),/hex:33/);
 const interaction=new JSDOM('<section></section>'),container=interaction.window.document.querySelector('section');
 tab.openMatches=new Set();tab.buildPlayers.delete('9');let rerenders=0;
 const controls=compile(source,['bindMatchDetailControls'],{bindRuneEffectLinks:()=>{},overviewContainer:()=>container,rerenderMatch:()=>{rerenders++;container.innerHTML=f.renderBuild(match,match.participants[0],tab);controls.bindMatchDetailControls(container,tab);}});
 container.innerHTML=f.renderBuild(match,match.participants[0],tab);controls.bindMatchDetailControls(container,tab);
 container.querySelector('[data-build-player="1"]').focus();
 interaction.window.document.activeElement.dispatchEvent(new interaction.window.KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true,cancelable:true}));
 assert.equal(interaction.window.document.activeElement.dataset.buildPlayer,'2');
 interaction.window.document.activeElement.click();
 assert.equal(rerenders,1);assert.equal(tab.buildPlayers.get('9'),2);assert.equal(interaction.window.document.activeElement.dataset.buildPlayer,'2');
 assert.match(container.textContent,/item:102/);assert.match(container.textContent,/skill:2/);assert.match(container.textContent,/hex:33/);
 interaction.window.close();
 const arena={gameId:10,modeGroup:'arena',participants:Array.from({length:16},(_,i)=>({participantId:i+1,subteamId:Math.floor(i/2)+1,placement:Math.floor(i/2)+1,championId:i+1,championLevel:18}))};
 const dom=new JSDOM(f.renderBuildPlayers(arena,arena.participants[0],arena.participants[0]));assert.equal(dom.window.document.querySelectorAll('[data-build-player]').length,16);assert.equal(dom.window.document.querySelectorAll('.is-arena-group').length,8);dom.window.close();
});
test('R211 streak skips remakes/custom, stops unknown and uses arena placement',()=>{
 const f=compile(source,['computeOverviewStreak','renderOverviewStreak'],{escapeHTML,matchSubject:m=>m.subject||{}});
 const match=(result,id)=>({result,queueId:420,createdAt:100-id});
 assert.equal(f.computeOverviewStreak([match('win',0),match('win',1),match('win',2),match('loss',3)],true,'').count,3);
 assert.equal(f.computeOverviewStreak([match('loss',0),match('loss',1),match('win',2)],false,''),null);
 assert.equal(f.computeOverviewStreak([match('win',0),match('remake',1),match('win',2),{...match('loss',3),gameType:'CUSTOM_GAME'},match('win',4)],false,'').count,3);
 const all=f.computeOverviewStreak(Array.from({length:20},(_,i)=>match('win',i)),true,'');assert.match(f.renderOverviewStreak(all),/20\+连胜/);
 assert.equal(f.computeOverviewStreak([match('win',0),match('unknown',1),match('win',2)],false,''),null);
 const arena=Array.from({length:3},(_,i)=>({...match('loss',i),modeGroup:'arena',subject:{placement:4}}));assert.equal(f.computeOverviewStreak(arena,false,'').result,'win');
 assert.match(f.renderOverviewStreak({result:'loss',count:3}),/streak-rain/);assert.match(f.renderOverviewStreak({result:'win',count:3}),/streak-outer-flame/);
});
test('R230 name copy shows feedback; hidden name cannot bind',async()=>{
 const dom=new JSDOM('<h2 data-copy-summoner="名称#tag" data-tooltip="名称">名称</h2><h2>隐藏玩家</h2>'),writes=[];
 const f=compile(source,['bindSummonerCopy','copySummonerText'],{window:{deepLegendsToast:()=>{}},document:dom.window.document,navigator:{clipboard:{writeText:async text=>writes.push(text)}},showToast:()=>assert.fail('fallback toast')});
 f.bindSummonerCopy(dom.window.document);dom.window.document.querySelector('h2').click();dom.window.document.querySelectorAll('h2')[1].click();await Promise.resolve();assert.deepEqual(writes,['名称#tag']);assert.equal(dom.window.document.querySelector('[data-copy-summoner]').dataset.tooltip,'名称');dom.window.close();
 const css=fs.readFileSync(path.join(__dirname,'gameplay.css'),'utf8');assert.match(css,/summoner-copy-name \{ cursor:pointer/);assert.match(css,/prefers-reduced-motion:reduce/);
});

test('R211 mastery entrance, real milestone mapping, sorted grid and popup fields',()=>{
 const rows=JSON.parse(fs.readFileSync(path.join(__dirname,'../testdata/r211/masteries-topking-five.json'),'utf8'));
 const names={126:'杰斯',897:'奎桑提',39:'艾瑞莉娅',799:'安蓓萨',68:'兰博'};
 const f=compile(source,['masteryDetailsPortrait','masteryMarkProgress','masteryGradeBoxes','masteryMarks','masteryTooltipContent','renderMasteryDetails','renderMasteries'],{number:String,escapeHTML,compactNumber:String,iconFigure:(_kind,id)=>`<span class="game-icon"><img src="/${id}"></span>`});
 assert.doesNotMatch(f.renderMasteries([],false),/data-overview-subpage/);
 assert.match(f.renderMasteries([],true),/data-overview-subpage="masteries"/);
 assert.match(f.masteryMarks(rows[0]),/0 \/ 2 标记/); // Bank 65 is not 65 completed milestone grades.
 assert.match(f.masteryMarks(rows[1]),/1 \/ 2 标记/);
 assert.match(f.masteryMarks(rows[2]),/0 \/ 1 标记/);
 const items=rows.map(row=>({...row,championName:names[row.championId]})).reverse();
 const dom=new JSDOM(f.renderMasteryDetails({items,totalScore:264,totalPoints:123456,totalChampions:173}));
 assert.equal(dom.window.document.querySelectorAll('[data-mastery-detail]').length,5);
 assert.equal(dom.window.document.querySelector('[data-mastery-detail]').dataset.masteryDetail,'126');
 assert.match(dom.window.document.querySelector('.overview-detail-summary').textContent,/264.*123456.*5 \/ 173/);
 assert.equal(dom.window.document.querySelector('.mastery-details-level').textContent,'72');
 const popup=f.masteryTooltipContent({...rows[0],championName:'杰斯'});
 assert.match(popup,/summoner-mastery-portrait mastery-details-portrait/);assert.match(popup,/summoner-mastery-crest mastery-details-crest/);
 assert.match(popup,/mastery-grade-box/);assert.match(popup,/里程碑 17/);assert.doesNotMatch(popup,/最高评价/);assert.match(popup,/3246 \/ 11000 点/);assert.match(popup,/最近游玩：\d{4}-\d{2}-\d{2}/);
 dom.window.close();
 const css=fs.readFileSync(path.join(__dirname,'gameplay.css'),'utf8');assert.match(css,/mastery-details-level[^}]+min-width:30px/);assert.match(css,/mastery-details-crest[^}]+object-fit:contain/);
});

test('R211 mastery portal flips inward and failed images become neutral placeholders',()=>{
 const dom=new JSDOM('<section><article data-mastery-detail="126"><span><img class="mastery-details-crest"></span></article></section>');
 const document=dom.window.document,card=document.querySelector('article');card.getBoundingClientRect=()=>({left:770,top:570,bottom:620});
 const proto=dom.window.HTMLElement.prototype;proto.getBoundingClientRect=function(){return this.classList.contains('mastery-details-popover')?{width:290,height:200}:{left:0,top:0,bottom:0};};card.getBoundingClientRect=()=>({left:770,top:570,bottom:620});
 const f=compile(source,['bindMasteryDetails'],{document,window:{innerWidth:780,innerHeight:650},masteryTooltipContent:()=>'<b>杰斯</b>',prepareImages:()=>{}});
 f.bindMasteryDetails(document.querySelector('section'),{items:[{championId:126}]});card.dispatchEvent(new dom.window.Event('mouseenter'));
 const tip=document.querySelector('.mastery-details-popover');assert.equal(tip.parentElement,document.body);assert.equal(tip.style.left,'482px');assert.equal(tip.style.top,'362px');
 card.dispatchEvent(new dom.window.Event('mouseleave'));assert.equal(document.querySelector('.mastery-details-popover'),null);
 const image=document.querySelector('img');image.dispatchEvent(new dom.window.Event('error'));assert.equal(image.hidden,true);assert.equal(image.parentElement.classList.contains('is-crest-missing'),true);dom.window.close();
});
