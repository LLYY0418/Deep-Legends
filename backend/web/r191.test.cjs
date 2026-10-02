'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile}=require('./r188-harness.cjs');
const {fixture}=require('./r190-harness.cjs');
const source=process.env.R191_SOURCE_FILE?fs.readFileSync(process.env.R191_SOURCE_FILE,'utf8'):read('gameplay.js');
function views(both=false){
 const f=fixture(source),d=new JSDOM('<main id="one"></main><main id="two"></main>');
 const document=d.window.document,externalMatchViews=new Map(),nodes={playerOverlayContent:document.createElement('main')};
 const state=f.state;state.tabs=[];state.overlay=[];
 const workspaces=new Map();
 const renderMatch=(match,_ref,tab)=>`<article class="match-entry" data-match-id="${match.gameId}"><div class="match-summary"><button data-toggle-match="${match.gameId}"></button><img src="icon.svg"></div><div class="match-detail">${tab.openMatches.has(String(match.gameId))&&tab.matchDetailTabs.get(String(match.gameId))==='build'?f.renderBuildAugments(match.participants[0].augmentIds):'概览'}</div></article>`;
 for(let i=0;i<2;i++){
  const tab={key:`tab${i}`,data:{player:{playerRef:'self'},matches:Array.from({length:10},(_,j)=>({gameId:i*10+j+1,subjectParticipantId:4,participants:[{participantId:4,augmentIds:[j===1?1225:120]}]}))},openMatches:new Set(i===0||both?[String(i*10+2)]:[]),matchDetailTabs:new Map([[String(i*10+2),'build']]),matchViewRevision:7};
  state.tabs.push(tab);const content=document.querySelector(i?'#two':'#one');content.innerHTML=tab.data.matches.map(m=>renderMatch(m,'',tab)).join('');workspaces.set(tab,{content});
 }
 // Second tab's existing DOM is parked in the real retained-view fragment form.
 const cached=document.createDocumentFragment();while(document.querySelector('#two').firstChild)cached.appendChild(document.querySelector('#two').firstChild);
 workspaces.get(state.tabs[1]).content._overviewViews=new Map([[state.tabs[1],{content:cached}]]);
 const deps={...f.deps,state,externalMatchViews,nodes,document,renderMatch,
  api:async()=>({items:[{id:1225,status:'ok',description:'真实正文'}]}),
  tabGroup:t=>t,overviewWorkspace:t=>workspaces.get(t),overviewContainer:t=>t.overlay?nodes.playerOverlayContent:t===state.tabs[0]?workspaces.get(t).content:null,
  rerenderTab:()=>assert.fail('whole tab redraw'),bindMatchDetailControls:()=>{},bindPlayerLinks:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{},
  rerenderCatalogViews:()=>{for(const tab of state.tabs)tab.matchViewRevision++;document.querySelector('#one').innerHTML='global redraw';},
 };
 const fn=compile(source,['ensureAugmentDescriptions','rerenderAugmentDescriptionViews','rerenderMatch','replaceMatchEntry'],deps);
 const entries=[...document.querySelectorAll('.match-entry'),...cached.querySelectorAll('.match-entry')];
 return{...fn,state,document,cached,entries,externalMatchViews,nodes,renderMatch};
}
test('R191 09 only intersecting open build detail changes among twenty retained cards',async()=>{
 const f=views(),cards=[...f.entries],details=cards.map(c=>c.querySelector('.match-detail')),summaries=cards.map(c=>c.querySelector('.match-summary'));
 await f.ensureAugmentDescriptions([1225]);
 assert.equal(f.state.tabs[0].matchViewRevision,7);assert.equal(f.state.tabs[1].matchViewRevision,7);
 const current=[...f.document.querySelectorAll(".match-entry"),...f.cached.querySelectorAll(".match-entry")];
 for(let i=0;i<20;i++) {assert.equal(cards[i],current[i]);assert.equal(cards[i].querySelector('.match-summary'),summaries[i]);if(i===1){assert.notEqual(cards[i].querySelector('.match-detail'),details[i]);assert.match(cards[i].textContent,/真实正文/);}else assert.equal(cards[i].querySelector('.match-detail'),details[i]);}
 assert.equal(f.document.querySelectorAll('.match-entry').length,10);
});
test('R191 10 shared IDs update active and cached tabs plus overlay and external views',async()=>{
 const f=views(true),before=f.entries.map(c=>c.querySelector('.match-detail'));
 // A closed build and a non-build open detail must remain untouched.
 f.state.tabs[0].openMatches.add('3');f.state.tabs[0].matchDetailTabs.set('3','overview');f.state.tabs[0].data.matches[2].participants[0].augmentIds=[1225];
 const overlay={overlay:true,data:{matches:[{gameId:21,subjectParticipantId:4,participants:[{participantId:4,augmentIds:[1225]}]}]},openMatches:new Set(['21']),matchDetailTabs:new Map([['21','build']]),matchViewRevision:3};
 f.state.overlay.push(overlay);f.document.body.append(f.nodes.playerOverlayContent);f.nodes.playerOverlayContent.innerHTML=f.renderMatch(overlay.data.matches[0],'',overlay);const overlayCard=f.nodes.playerOverlayContent.firstElementChild,overlayDetail=overlayCard.querySelector('.match-detail');
 const external=f.document.createElement('section');f.document.body.append(external);const calls=[];
 f.externalMatchViews.set(external,{tab:{data:{matches:[{gameId:22,subjectParticipantId:4,participants:[{participantId:4,augmentIds:[1225]}]}]},openMatches:new Set(['22']),matchDetailTabs:new Map([['22','build']]),externalRender:id=>calls.push(id)}});
 await f.ensureAugmentDescriptions([1225]);
 for(const i of [1,11]){assert.notEqual(f.entries[i].querySelector('.match-detail'),before[i]);assert.match(f.entries[i].textContent,/真实正文/)}
 assert.equal(f.entries[2].querySelector('.match-detail'),before[2]);assert.equal(overlayCard,f.nodes.playerOverlayContent.firstElementChild);assert.notEqual(overlayDetail,overlayCard.querySelector('.match-detail'));assert.deepEqual(calls,['22']);assert.deepEqual([...f.state.tabs.map(t=>t.matchViewRevision),overlay.matchViewRevision],[7,7,3]);
});
const css=read('gameplay.css');
test('R191 14 effect icons and value gaps use corrected dimensions, wide tree is centered',()=>{
 assert.match(css,/\.eff \.game-icon\s*\{[^}]*width:\s*28px;\s*height:\s*28px/);
 assert.match(css,/\.eff\.is-keystone \.game-icon\s*\{[^}]*width:\s*44px;\s*height:\s*44px/);
 assert.match(css,/\.rune-effects \.stats\s*\{[^}]*gap:\s*12px/);
 assert.match(css,/@container \(min-width: 1100px\)\s*\{\s*\.rune-split\s*\{[^}]*\}\s*\.rune-split-tree\s*\{[^}]*display:\s*grid;\s*align-content:\s*center/);
});
test('R191 15–16 R195 removes effect shards while preserving the shared tree icons',()=>{
 const {extract}=require('./r188-harness.cjs');
 const f=fixture(source);const d=new JSDOM(f.renderRuneEffects(f.subject)).window.document;
 assert.equal(d.querySelectorAll('.shards-line,.shard-chip').length,0);
 assert.doesNotMatch(css,/shards-line|shard-chip/);
 assert.match(extract(source,'renderRuneOption'),/dataDragonRuneShardPath\(id\)/);
});
