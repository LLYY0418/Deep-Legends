'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile,escapeHTML}=require('./r188-harness.cjs');const source=read('gameplay.js');
const {expand}=require('./r211-harness-support.cjs');
function functions(names,deps){return compile(source,expand(source,names,deps),deps)}
test('R216 tags show corrected words, four ordered tags, disabled keyword and unknown suppression',()=>{
 const state={matchTimelines:new Map()};const f=functions(['renderMatchTags'],{state,escapeHTML,matchTimelineKey:()=> 'one',scoreBadgeChip:r=>r?.badge?`<b>${r.badge}</b>`:''});
 const match={gameId:1,result:'win'},p={participantId:1,multiKill:5,keyword:'resilience',deaths:0};let html=f.renderMatchTags(match,p,{badge:'MVP'},{});assert.ok(html.indexOf('MVP')<html.indexOf('五杀'));assert.ok(html.indexOf('五杀')<html.indexOf('坚韧'));assert.ok(html.indexOf('坚韧')<html.indexOf('零阵亡'));assert.equal((html.match(/class="(?:multikill-tag|match-keyword-tag|match-zero-deaths)/g)||[]).length,3);
 p.keywordDisabled=true;assert.doesNotMatch(f.renderMatchTags(match,p,{badge:'MVP'},{}),/坚韧/);match.result='unknown';assert.equal(f.renderMatchTags(match,p,{},{}),'');
 for(const [key,label]of [['innocent','竭尽全力'],['slowstarter','慢热'],['unyielding','不屈之志'],['struggling','挣扎'],['dedication','奉献'],['rollercoaster','过山车']])assert.equal(f.matchKeywordMetadata(key)[0],label);
 for(let n=2;n<=5;n++){html=f.renderMultiKillTag(n,1);assert.match(html,/<svg/);assert.match(html,new RegExp(`is-mk-${n}`));assert.match(html,/--mk-/);}assert.equal(f.renderMultiKillTag(1,1),'');
});
test('R216 background timelines pause for KR, hidden, champ select, live and inactive tabs',()=>{
 const tab={},state={section:'overview',beacon:{phase:''}},document={hidden:false};let kr=false;let active=tab;
 const f=compile(source,['matchTagsBackgroundAllowed'],{state,document,riotTab:()=>kr,connected:()=>true,tabGroup:()=> 'players',activeTab:()=>active});
 assert.equal(f.matchTagsBackgroundAllowed(tab),true);kr=true;assert.equal(f.matchTagsBackgroundAllowed(tab),false);kr=false;document.hidden=true;assert.equal(f.matchTagsBackgroundAllowed(tab),false);document.hidden=false;
 for(const phase of ['ChampSelect','InProgress']){state.beacon.phase=phase;assert.equal(f.matchTagsBackgroundAllowed(tab),false);}state.beacon.phase='';active={};assert.equal(f.matchTagsBackgroundAllowed(tab),false);
});
function tableHarness(queue='ranked'){
 const dom=new JSDOM('<main></main>'),root=dom.window.document.querySelector('main');const row=(id,games)=>({championId:id,championName:`英雄${id}`,games,wins:games/2,losses:games/2,winRate:50,kda:3,opponents:Array.from({length:21},(_,i)=>({championId:100+i,championName:`对位${i}`,games:21-i,wins:2,losses:1,winRate:66}))});
 const data={queue,overall:{championName:'所有英雄',games:30},rows:[row(2,10),row(1,20)]};const tab={overviewSubpageState:{data}};let f;
 const redraw=()=>{root.innerHTML=f.renderChampionTable(data,tab);f.bindChampionTable(root,tab)};
 f=functions(['renderChampionTable','bindChampionTable'],{document:dom.window.document,prepareImages:()=>{},escapeHTML,iconFigure:()=>'',renderOverviewSubpage:redraw,openOverviewSubpage:async()=>{}});redraw();return{dom,root,tab,data,f};
}
test('R216 table sorts, expands first champion, increments opponents by ten and preserves overall first',()=>{
 const h=tableHarness();try{let rows=h.root.querySelectorAll('tbody tr');assert.match(rows[0].textContent,/所有英雄/);assert.match(rows[1].textContent,/英雄1/);assert.equal(h.root.querySelectorAll('tr.is-opponent').length,6);
 h.root.querySelector('[data-table-more]').click();assert.equal(h.root.querySelectorAll('tr.is-opponent').length,16);
 h.root.querySelector('[data-table-sort="games"]').click();assert.equal(h.tab.championTableDirection,1);assert.match(h.root.querySelectorAll('tbody tr')[1].textContent,/英雄2/);
 h.root.querySelector('[data-table-sort="games"]').click();assert.equal(h.tab.championTableDirection,-1);h.root.querySelector('[data-table-expand="1"]').click();assert.equal(h.root.querySelectorAll('tr.is-opponent').length,0);
 assert.equal(h.f.championTableNumber(null),'-');
 }finally{h.dom.window.close()}
});
test('R216 mayhem removes wards, CS and opponent expansion; fourteen keyword colors remain distinct',()=>{
 const h=tableHarness('mayhem');try{assert.equal(h.root.querySelectorAll('[data-table-expand]').length,0);assert.doesNotMatch(h.root.querySelector('thead').textContent,/守卫|补刀/);assert.equal(h.root.querySelectorAll('tr.is-opponent').length,0)}finally{h.dom.window.close()}
 const css=read('gameplay.css'),colors=[...css.matchAll(/--tag-(?!zero)([a-z]+):(#\w+);/g)].map(m=>m[2]);assert.equal(colors.length,14);assert.equal(new Set(colors).size,14);assert.match(css,/prefers-reduced-motion/);assert.match(css,/\.champion-table-hero[^}]*sticky/s);
});

test('R216 champion-table arrow uses full season count and supported data source',()=>{
 const f=compile(source,['renderChampionStats'],{escapeHTML,iconFigure:()=>'',number:String,kda:String,percent:String});
 const items=[{games:19}],overall={games:20};assert.match(f.renderChampionStats(items,overall,{tableSupported:true}),/data-overview-subpage="champion-table"/);
 overall.games=19;assert.doesNotMatch(f.renderChampionStats(items,overall,{tableSupported:true}),/data-overview-subpage=/);overall.games=200;assert.doesNotMatch(f.renderChampionStats(items,overall,{tableSupported:false}),/data-overview-subpage=/);
});
test('R216 successful keyword computation suppresses background refetch even when equipment timeline is empty',async()=>{
 const state={matchTimelines:new Map(),matchTimelineFlights:new Set()},match={gameId:1},subject={participantId:1};let calls=0;
 const f=compile(source,['ensureMatchTimeline'],{state,riotTab:()=>false,connected:()=>true,matchTimelineKey:()=> 'one',tabServerID:()=> 'HN1',api:async()=>{calls++;return {available:false,tags:[{participantId:1,keyword:'average',checkpoints:[6,6,6]}]}},recordTimelineClient:()=>{},rerenderMatch:()=>{}});
 await f.ensureMatchTimeline(match,subject,{key:'one'});assert.equal(calls,1);assert.equal(match.tagsAvailable,true);assert.match(source,/filter\(match=>!match\.tagsAvailable/);
});

test('R217 holdout eight closed keywords never render from participant or stale timeline cache',()=>{
 const state={matchTimelines:new Map()},f=functions(['renderMatchTags'],{state,escapeHTML,matchTimelineKey:()=> 'one',scoreBadgeChip:()=>''}),match={gameId:1,result:'win'},subject={participantId:1,deaths:1};
 for(const key of ['leader','victorious','dedication','average','rollercoaster','decline','innocent','slowstarter']){
  subject.keyword=key;assert.equal(f.renderMatchTags(match,subject,{},{}),'');state.matchTimelines.set('one',{tags:[{participantId:1,keyword:key}]});assert.equal(f.renderMatchTags(match,subject,{},{}),'');state.matchTimelines.clear();
 }
 for(const key of ['unstoppable','latebloomer','resilience','unlucky','unyielding','struggling']){subject.keyword=key;assert.match(f.renderMatchTags(match,subject,{},{}),new RegExp(`is-tag-${key}`));}
});

test('R217 newly opened keywords use existing site words and palette for participant and cached tags',()=>{
 const state={matchTimelines:new Map()},f=functions(['renderMatchTags'],{state,escapeHTML,matchTimelineKey:()=> 'one',scoreBadgeChip:()=>''}),match={gameId:1,result:'win'},subject={participantId:1,deaths:1},css=read('gameplay.css');
 for(const [key,label,color] of [['latebloomer','大器晚成','#6CC04A'],['unyielding','不屈之志','#E0C341']]){
  subject.keyword=key;let html=f.renderMatchTags(match,subject,{},{});assert.match(html,new RegExp(`is-tag-${key}`));assert.match(html,new RegExp(label));
  state.matchTimelines.set('one',{tags:[{participantId:1,keyword:key}]});subject.keyword='leader';html=f.renderMatchTags(match,subject,{},{});assert.match(html,new RegExp(label));
  state.matchTimelines.get('one').tags[0].disabled=true;assert.equal(f.renderMatchTags(match,subject,{},{}),'');state.matchTimelines.clear();
  assert.ok(css.includes(`--tag-${key}:${color};`));assert.match(css,new RegExp(`\\.is-tag-${key}\\s*\\{\\s*--tag-color:var\\(--tag-${key}\\);\\s*\\}`));assert.match(css,new RegExp(`\\[data-theme="light"\\] \\.is-tag-${key}\\s*\\{`));
 }
});
