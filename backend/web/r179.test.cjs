'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const src=fs.readFileSync(process.env.R179_GAMEPLAY_SOURCE||path.join(__dirname,'gameplay.js'),'utf8');
function extract(name,source=src){let start=source.indexOf(`function ${name}(`);assert.ok(start>=0,name);if(source.slice(start-6,start)==='async ')start-=6;const body=source.indexOf(') {',start)+2;let depth=0,quote='',escaped=false;for(let i=body;i<source.length;i++){const c=source[i];if(quote){if(escaped)escaped=false;else if(c==='\\')escaped=true;else if(c===quote)quote='';continue;}if(c==='"'||c==="'"||c==='`'){quote=c;continue;}if(c==='{')depth++;if(c==='}'&&--depth===0)return source.slice(start,i+1);}throw Error(name);}

function compile(names,deps={},source=src){return Function(...Object.keys(deps),names.map(n=>extract(n,source)).join('\n')+`\nreturn {${names.join(',')}};`)(...Object.values(deps));}
const flush=()=>new Promise(setImmediate);
function cardRenderer(source=src){const state={live:{phase:'ChampSelect'}};const deps={state,maskedPlayerName:p=>p.hidden?'隐藏玩家':'Player',liveDisplayedChampionId:p=>p.championId||0,rankTitle:()=> '未定级',positionLabel:()=> '位置未知',renderLivePremadeTag:()=>'',proBadgeAttributes:()=>'',renderProIdentityBadge:()=>'',number:String,percent:String,kda:String,escapeHTML:String,iconFigure:(_,id)=>`<img src="champion-${id}">`};const f=compile(['renderLivePlayer','champSelectEnemyPlaceholder','liveHistoryStateOf','liveHistorySettled'],deps,source);return {state,renderLivePlayer:(...args)=>{args[8]=state.live.phase;return f.renderLivePlayer(...args)}};}
test('R179 enemy placeholder is champ-select only and keeps R193 live hidden identity display',()=>{
 const h=cardRenderer(),players=[{teamId:100,isCurrent:true},{teamId:200,hidden:true,historyState:'unavailable',championId:61}];
 const html=h.renderLivePlayer(players[1],1,false,0,players);const dom=new JSDOM(html);
 const style=dom.window.document.createElement('style');style.textContent=fs.readFileSync(path.join(__dirname,'gameplay.css'),'utf8');dom.window.document.head.appendChild(style);
 const hint=dom.window.document.querySelector('.live-player-placeholder');assert.equal(hint.textContent,'暂无玩家信息，进入游戏后显示');
 const computed=dom.window.getComputedStyle(hint);assert.equal(computed.whiteSpace,'normal');assert.equal(computed.gridColumn,'2 / -1');assert.notEqual(computed.textOverflow,'ellipsis');assert.equal(dom.window.document.querySelector('img').getAttribute('src'),'champion-61');assert.doesNotMatch(html,/player-tab-hidden|客户端未公开该玩家|位置未知/);dom.window.close();
 h.state.live.phase='InProgress';const live=h.renderLivePlayer(players[1],1,false,0,players);
 const baseline=fs.readFileSync(path.join(__dirname,'../testdata/r179-live-hidden.html'),'utf8');
 assert.equal(live,baseline);assert.match(live,/隐藏玩家/);assert.match(live,/隐藏身份/);assert.doesNotMatch(live,/暂无玩家信息，进入游戏后显示/);
 h.state.live.phase='ChampSelect';assert.match(h.renderLivePlayer({...players[1],teamId:100},1,false,0,players),/隐藏身份/);
});
function runeHarness(){
 const state={section:'live',live:{phase:'ChampSelect',spell1Id:4,spell2Id:12},runeSourceTab:'specialist',liveGameGeneration:1,specialistRunes:new Map(),specialistRuneFlights:new Map(),specialistRuneFailures:new Map(),runeStarterRequests:new Map()};
 let calls=0,reply=[{key:'row',playedAt:1000,itemIds:[1001],starterItemIds:[1055]}],release;
 const deps={state,liveRecommendationTarget:d=>({championId:61,position:'mid',clientPosition:'mid',gameMode:'CLASSIC',mapId:11,tier:'all',spellKey:d.phase==='ChampSelect'?`${d.spell1Id}-${d.spell2Id}`:'none'}),liveRecommendationsFor:()=>null,recommendationQueueHasTopPlayers:()=>true,recordSpecialistRuneClientSkip:()=>{},specialistRuneFailure:()=>null,renderLive:()=>{},ensurePerks:()=>{},api:async()=>{calls++;if(release)await new Promise(resolve=>release=resolve);return structuredClone(reply);}};
 const functions=compile(['specialistPosition','specialistRequestTarget','ensureSpecialistRunes','specialistRuneFlightActive','retainRuneStarterItems','renderRuneEquipment'],{renderSummonerSpellIcon:()=>'',renderItemIcon:id=>`<img src="item-${id}">`,...deps});
 return {state,...functions,calls:()=>calls,setReply:r=>reply=r,deps};
}
test('R179 specialist key survives four skill changes and phase; concurrent entries send one request',async()=>{
 const h=runeHarness();let resolve;h.deps.api=()=>{return new Promise(r=>resolve=r)};
 await h.ensureSpecialistRunes(h.state.live);assert.equal(h.calls(),1);
 const original=h.state.specialistRunes.get(h.specialistRequestTarget(h.state.live).key);
 for(const [a,b] of [[4,6],[4,12],[4,21],[3,4]]) {h.state.live.spell1Id=a;h.state.live.spell2Id=b;await h.ensureSpecialistRunes(h.state.live);assert.equal(h.state.specialistRunes.get('61:mid'),original)}
 h.state.live.phase='InProgress';await h.ensureSpecialistRunes(h.state.live);assert.equal(h.calls(),1);
 // Two real callers enter while the first API promise is unresolved.
 const state=h.state;state.specialistRunes.clear();let count=0,done;
 const f=compile(['specialistPosition','specialistRequestTarget','ensureSpecialistRunes','specialistRuneFlightActive','retainRuneStarterItems'],{...h.deps,api:()=>{count++;return new Promise(r=>done=r)}});
 const first=f.ensureSpecialistRunes(state.live),second=f.ensureSpecialistRunes(state.live);assert.equal(count,1);done(original);await Promise.all([first,second]);
});
test('R179 specialist refresh retains starter items and cached response renders equipment immediately',async()=>{
 const h=runeHarness();await h.ensureSpecialistRunes(h.state.live);const row=h.state.specialistRunes.get('61:mid')[0];
 h.state.specialistRunes.delete('61:mid');h.setReply([{key:'row',playedAt:1000,itemIds:[1001]}]);
 // A cache update arriving while a refresh is in flight must be retained by the real writer.
 let done;const f=compile(['specialistPosition','specialistRequestTarget','ensureSpecialistRunes','specialistRuneFlightActive','retainRuneStarterItems'],{...h.deps,api:()=>new Promise(r=>done=r)});
 const pending=f.ensureSpecialistRunes(h.state.live);h.state.specialistRunes.set('61:mid',[row]);done([{key:'row',playedAt:1000,itemIds:[1001]}]);await pending;
 assert.deepEqual(h.state.specialistRunes.get('61:mid')[0].starterItemIds,[1055]);assert.match(h.renderRuneEquipment(row),/装备.*item-1055.*route-divider.*item-1001/);
 const dom=new JSDOM('<div class="specialist-game-row" data-rune-choice="row"></div>');let requests=0;
 const g=compile(['ensureRuneStarterItems','clearRuneStarterRetries','runeStarterTargetActive'],{state:h.state,document:dom.window.document,nodes:{liveContent:dom.window.document.body},specialistRequestTarget:f.specialistRequestTarget,specialistRunesFor:()=>h.state.specialistRunes.get('61:mid'),proRequestTarget:()=>null,proRunesFor:()=>[],clearTimeout:()=>{},api:async()=>{requests++;return {starters:[]}},patchRuneStarterEquipment:()=>{},setTimeout:()=>0});
 await g.ensureRuneStarterItems(h.state.live);assert.equal(requests,0);delete h.state.specialistRunes.get('61:mid')[0].starterItemIds;await g.ensureRuneStarterItems(h.state.live);assert.equal(requests,1);dom.window.close();
});
function rosterMarkup(changed=-1,iteration=0){return `<section id="recommendation-panel-insight" class="recommendation-panel"><div class="live-teams"><section class="live-team"><header>team</header><div class="live-player-list">${Array.from({length:10},(_,i)=>`<article data-live-player-row="card:p${i}"><img src="hero-${i}"><span>${i===changed?'changed'+iteration:'same'}</span></article><div data-live-player-row="history:p${i}"><img src="history-${i}"></div>`).join('')}</div></section></div></section><section id="recommendation-panel-runes" class="recommendation-panel"><div class="specialist-game-row" data-rune-choice="r"><div class="specialist-game-items"><img src="starter"></div></div></section>`;}
test('R179 three minute roster polling preserves nine rows/images and skill changes retain rune DOM',()=>{
 const dom=new JSDOM(`<main><div data-live-body>${rosterMarkup()}</div></main>`),root=dom.window.document.querySelector('main'),reports=[];
 const f=compile(['liveBodyChrome','updateLivePanels','stampLiveRows','preserveLiveImages','patchLiveRosterPanel'],{document:dom.window.document,nodes:{liveContent:root},captureLiveScroll:()=>[],restoreLiveScroll:()=>{},bindLivePanelScope:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{},recordLiveRenderRebuild:(parts,stats)=>reports.push(stats)});f.stampLiveRows(root);
 let current=rosterMarkup();const starter=root.querySelector('[src="starter"]');
 for(let tick=0;tick<61;tick++) {
  const rows=[...root.querySelectorAll('[data-live-player-row^="card:"]')],images=rows.map(r=>r.querySelector('img'));
  if(tick%6===0 && tick<60) current=rosterMarkup(3,tick);
  assert.equal(f.updateLivePanels(current),true);
  const next=[...root.querySelectorAll('[data-live-player-row^="card:"]')];for(let i=0;i<10;i++) {if(i!==3||tick%6!==0||tick>=60)assert.equal(next[i],rows[i]);assert.equal(next[i].querySelector('img'),images[i])}
  assert.equal(root.querySelector('[src="starter"]'),starter);
 }
 assert.ok(reports.every(r=>r.imagesRecreated===0));assert.equal(reports.reduce((n,r)=>n+r.rowsReplaced,0),10);dom.window.close();
});
test('R179 refresh button changes only for manual loads',()=>{
 const dom=new JSDOM('<div class="live-toolbar"><button></button><span data-live-status></span></div><main data-main><div data-live-body></div></main>');const button=dom.window.document.querySelector('button'),root=dom.window.document.querySelector('main'),state={section:'live',liveLoading:true,live:{available:true},beacon:{}};
 root._recommendationMarkup='same';const {renderLive}=compile(['renderLive'],{state,nodes:{liveRefresh:button,liveContent:root},document:dom.window.document,connected:()=>true,renderLiveRefreshStatus:()=>'',renderSessionSummary:()=>{},liveRenderTriggerLabel:s=>s,liveRecommendationMarkup:()=> 'same'});
 for(const source of ['interval','sse','event','direct']){state.liveLoadSource=source;renderLive();assert.equal(button.textContent,'刷新对局');assert.equal(button.getAttribute('aria-busy'),'true')}
 state.liveLoadSource='manual';renderLive();assert.equal(button.textContent,'正在刷新…');dom.window.close();
});
test('R179 runtime transport admits bounded rebuild counts and lane fields, dropping private fields',async()=>{
 const dom=new JSDOM('<body></body>',{url:'http://localhost'}),bodies=[];
 const context={window:dom.window,document:dom.window.document,fetch:async(_,options)=>{bodies.push(JSON.parse(options.body));return {ok:true,status:204}},setTimeout:()=>1,clearTimeout:()=>{},setInterval:()=>1,clearInterval:()=>{},Date,URL,URLSearchParams,AbortController,console};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 dom.window.reportFlowDiagnostic('live_render_rebuild','aggregated',{counts:{insight:10,puuid:99},sources:{interval:60,secret:99},total:10,windowMs:180000,phase:'ChampSelect',rowsReplaced:10,imagesRecreated:0,playerRef:'secret'});
 dom.window.reportFlowDiagnostic('lane_matchup_candidate_fetch','requested',{selfPosition:'mid',enemyLockedCount:4,enemyPositionKnownCount:2,allyPositionKnownCount:5,enemyChampionId:101,rowCount:5,position:'mid',tier:'all'});await flush();
 assert.equal(bodies.length,2);assert.deepEqual(bodies[0].counts,{insight:10});assert.equal(bodies[0].rowsReplaced,10);assert.equal(bodies[1].enemyLockedCount,4);assert.doesNotMatch(JSON.stringify(bodies),/secret|puuid|playerRef/);
 if(process.env.R179_WRITE_DIAGNOSTICS)fs.writeFileSync(process.env.R179_WRITE_DIAGNOSTICS,JSON.stringify(bodies));dom.window.close();
});
test('R179 lane diagnostic direct sender preserves nonzero availability fields',async()=>{
 const sent=[];compile(['recordItemSetClientDiagnostic'],{fetch:async(_,o)=>{sent.push(JSON.parse(o.body));return {ok:true}}}).recordItemSetClientDiagnostic('lane_matchup_candidate_fetch','requested',{selfPosition:'mid',enemyLockedCount:4,enemyPositionKnownCount:2,allyPositionKnownCount:5,enemyChampionId:101,position:'mid',tier:'all'});await flush();assert.equal(sent[0].selfPosition,'mid');assert.equal(sent[0].enemyLockedCount,4);if(process.env.R179_WRITE_LANE_DIAGNOSTIC)fs.writeFileSync(process.env.R179_WRITE_LANE_DIAGNOSTIC,JSON.stringify(sent));
});

test('R179 repeated partial panel bindings dispatch each action exactly once',()=>{
 const dom=new JSDOM('<section><button data-rune-source="specialist"></button><button data-specialist-player="first"></button><button data-player-ref="p"></button><button data-live-position="mid"></button><button data-apply-runes></button><button data-apply-item-set></button><button data-live-history-retry></button><input data-apply-rune-spells type="checkbox"><div data-rune-choice="r"></div></section>'),root=dom.window.document.querySelector('section');
 const counts={};const count=k=>()=>counts[k]=(counts[k]||0)+1;const state={live:{},specialistPlayerTabs:new Map()};
 const f=compile(['bindLiveNode','bindRuneWorkspaceControls','bindLivePanelScope','bindRuneChoiceButtons'],{state,window:dom.window,CSS:{escape:String},clearRuneStarterRetries:()=>{},refreshLiveRuneWorkspace:count('workspace'),specialistRequestTarget:()=>({key:'61:mid'}),proRequestTarget:()=>null,ensureRuneStarterItems:()=>{},openPlayerOverlay:count('overlay'),playerButtonLabel:()=>'',proContextFromButton:()=>({}),selectLivePosition:count('position'),livePositionValue:String,applyRunes:count('runes'),applyItemSet:count('items'),loadLive:count('history'),renderLive:count('choice')});
 for(let i=0;i<30;i++) f.bindLivePanelScope(root);
 for(const button of root.querySelectorAll('button'))button.click();root.querySelector('[data-rune-choice]').click();
 assert.deepEqual(counts,{workspace:2,overlay:1,position:1,runes:1,items:1,history:1,choice:1});dom.window.close();
});

test('R179 changing roster structure replaces rows and reports same-source recreated images honestly',()=>{
 const dom=new JSDOM(`<main><div data-live-body>${rosterMarkup()}</div></main>`),root=dom.window.document.querySelector('main'),reports=[];
 const f=compile(['liveBodyChrome','updateLivePanels','stampLiveRows','preserveLiveImages','patchLiveRosterPanel'],{document:dom.window.document,nodes:{liveContent:root},captureLiveScroll:()=>[],restoreLiveScroll:()=>{},bindLivePanelScope:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{},recordLiveRenderRebuild:(p,s)=>reports.push(s)});f.stampLiveRows(root);
 const before=root.querySelector('[data-live-player-row="card:p0"]');
 assert.equal(f.updateLivePanels(rosterMarkup().replace('card:p0','card:new')),true);assert.equal(before.isConnected,false);
 assert.ok(reports.every(r=>r.imagesRecreated===0),'same-source images are retained even across structural panel replacement');dom.window.close();
});

test('R179 actual specialist equipment DOM stays identical through skills and entering the game',async()=>{
 const h=runeHarness();await h.ensureSpecialistRunes(h.state.live);
 const markup=()=>`<section id="recommendation-panel-runes" class="recommendation-panel"><p>${h.state.live.phase}:${h.state.live.spell1Id}:${h.state.live.spell2Id}</p><div class="specialist-game-row" data-rune-choice="row">${h.renderRuneEquipment(h.state.specialistRunes.get(h.specialistRequestTarget(h.state.live).key)[0])}</div></section>`;
 const dom=new JSDOM(`<main><div data-live-body>${markup()}</div></main>`),root=dom.window.document.querySelector('main');
 const f=compile(['liveBodyChrome','updateLivePanels','stampLiveRows','preserveLiveImages','patchLiveRosterPanel'],{document:dom.window.document,nodes:{liveContent:root},captureLiveScroll:()=>[],restoreLiveScroll:()=>{},bindLivePanelScope:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{},recordLiveRenderRebuild:()=>{}});f.stampLiveRows(root);
 const row=root.querySelector('.specialist-game-row'),starter=root.querySelector('[src="item-1055"]');
 for(const [a,b] of [[4,6],[4,12],[4,21],[3,4]]){h.state.live.spell1Id=a;h.state.live.spell2Id=b;await h.ensureSpecialistRunes(h.state.live);f.updateLivePanels(markup());assert.equal(root.querySelector('.specialist-game-row'),row);assert.equal(root.querySelector('[src="item-1055"]'),starter);assert.equal(h.calls(),1)}
 h.state.live.phase='InProgress';await h.ensureSpecialistRunes(h.state.live);f.updateLivePanels(markup());assert.equal(root.querySelector('[src="item-1055"]'),starter);assert.equal(h.calls(),1);dom.window.close();
});
