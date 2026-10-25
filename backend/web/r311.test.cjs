'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {extract} = require('./r188-harness.cjs');
const read = name => fs.readFileSync(path.join(__dirname,name),'utf8');
function runtime() {const ctx={window:{}};vm.runInNewContext(read('runtime.js'),ctx);return ctx.window.deepLegendsRuntime;}
const dataRows = () => Array.from({length:18},(_,i)=>({id:i+1,rarity:['silver','gold','prismatic'][i%3],grade:i<3?'OP':'C',score:100-i,games:100+i,winRate:52+i/10,pickRate:12,assets:[{id:i+1,name:`推荐${i+1}`,kind:'arena-augment'}]}));

test('R311 shared ordering uses games/score/grade, retains low samples, and preserves tier order and input',()=>{
 const {orderArenaRows}=runtime();
 const rows=[{id:1,grade:'S',score:90,games:1},{id:2,grade:'OP',score:1,games:1},{id:3,grade:'S',score:90,games:10},{id:4,grade:'',score:100,games:100},{id:5,grade:'A',score:90,games:10}];
 const ids=rows=>Array.from(rows,x=>x.id);
 assert.deepEqual(ids(orderArenaRows(rows)),[4,3,5,1,2]);
 assert.deepEqual(ids(orderArenaRows(rows,'tier')),[2,3,1,5,4]);
 assert.deepEqual(ids(orderArenaRows(rows.map(x=>({...x,games:undefined,stats:{games:x.games}})))),[4,3,5,1,2]);
 assert.deepEqual(ids(rows),[1,2,3,4,5]);
 assert.equal(orderArenaRows(rows).length,5,'no invented minimum-games filter');
});

test('R311 real hero DOM switches both lists, retains expansion, normalizes old values, and resets on hero change',async()=>{
 const dom=new JSDOM(read('index.html'),{url:'http://localhost',runScripts:'outside-only',pretendToBeVisual:true});
 const {window}=dom;
 const row=id=>({championId:id,champion:id===32?'amumu':'galio',key:id===32?'amumu':'galio',name:id===32?'阿木木':'加里奥',grade:'A',winRate:54,play:8000});
 const rows=dataRows();const detail={mode:'arena',patch:'16.21',arenaAugmentGroups:['silver','gold','prismatic'].map(rarity=>({rarity,rows:rows.filter(r=>r.rarity===rarity)})),build:{coreItems:rows.map(r=>({...r,assets:r.assets.map(a=>({...a,kind:'item'}))})),prismItems:[],boots:[]}};
 window.fetch=async()=>({ok:true,status:200,json:async()=>detail});
 window.eval(read('runtime.js'));
 window.eval(read('champions.js').replace(/\n\}\)\(\);\s*$/,'\nwindow.__r311={state,render,openDetail,resetArenaControls,arenaOrderRows:typeof arenaOrderRows === "function" ? arenaOrderRows : null};\n})();'));
 try {
  const c=window.__r311;assert.equal(c.state.arenaSort,'games');
  Object.assign(c.state,{section:'champions',mode:'arena',selected:row(32),rankings:{patch:'16.21',rows:[row(32),row(3)]},catalog:{champions:[]},detail});c.render();
  const doc=window.document;
  const names=()=>[...doc.querySelectorAll('.arena-augment-section .arena-option-main > strong')].map(e=>e.textContent);
  const coreNames=()=>[...doc.querySelectorAll('.arena-core-section .arena-option-main > strong')].map(e=>e.textContent);
  assert.deepEqual([...doc.querySelectorAll('[data-arena-sort]')].map(e=>e.textContent),['按场次','按梯度']);
  assert.equal(doc.querySelector('[data-arena-sort="games"]').getAttribute('aria-pressed'),'true');
  assert.deepEqual(names(),[18,17,16,15,14,13,12,11,10].map(i=>`推荐${i}`));
  assert.deepEqual(coreNames(),[18,17,16,15,14,13].map(i=>`推荐${i}`));
  doc.querySelector('[data-arena-expand="augments"]').click();assert.equal(names().length,18);
  doc.querySelector('[data-arena-sort="tier"]').click();assert.equal(c.state.arenaExpanded.augments,true);
  assert.deepEqual(names().slice(0,3),['推荐1','推荐2','推荐3']);
  assert.deepEqual(coreNames(),[1,2,3,4,5,6].map(i=>`推荐${i}`));
  assert.match(doc.querySelector('.arena-augment-section header').textContent,/按梯度展示前九项/);
  c.state.arenaSort='winRate';assert.deepEqual(Array.from(c.arenaOrderRows(rows)),[...rows].reverse());
  await c.openDetail(row(3));assert.equal(c.state.arenaSort,'games');assert.equal(c.state.arenaExpanded.augments,false);
 } finally {window.__r311.state.arenaRequestToken += 1;window.dispatchEvent(new window.Event("deep-legends:dispose"));window.close();}
});

test('R311 live Arena shows four per rarity ordered by games and always win plus games; hextech remains three by tier',()=>{
 const rt=runtime(),rows=dataRows(),comboSources=[];
 const deps={window:{deepLegendsRuntime:rt},gradeRank:rt.gradeRank,gradeBadge:rt.gradeBadge,state:{liveRecommendationFailures:new Map()},renderLiveARAMKitCombos:(_data,source)=>{comboSources.push(source);return source==='hextech'?'<section data-r311-combos>海克斯组合</section>':'';},escapeHTML:String,liveChampionAugmentRows:()=>rows,normalizeAugmentRarity:v=>({key:v}),augmentTooltipText:a=>a.name,renderLiveAugmentIcon:r=>`<i data-id="${r.id}"></i>`,percent:n=>`${n}%`,number:String};
 const render=Function(...Object.keys(deps),extract(read('gameplay.js'),'renderLiveAugmentRecommendations')+';return renderLiveAugmentRecommendations;')(...Object.values(deps));
 for(const source of ['arena','hextech']){
  const dom=new JSDOM(render({},source));const doc=dom.window.document;
  assert.equal(comboSources.at(-1),source);assert.equal(!!doc.querySelector('[data-r311-combos]'),source==='hextech','Arena omits combos; Hextech keeps them');
  for(const [j,key] of ['silver','gold','prismatic'].entries()){
   const ids=[...doc.querySelectorAll(`.is-${key}.live-augment-column [data-id]`)].map(e=>+e.dataset.id);
   assert.deepEqual(ids,source==='arena'?[16+j,13+j,10+j,7+j]:[1+j,4+j,7+j]);
  }
  assert.equal(doc.querySelectorAll('.live-augment-option').length,source==='arena'?12:9);
  for(const card of doc.querySelectorAll('.live-augment-option')){
   assert.ok(card.querySelector('.live-augment-stat.is-win'));assert.ok(card.querySelector('.live-augment-stat.is-games'));assert.equal(card.querySelector('.live-augment-stat.is-pick'),null);
   assert.ok(card.querySelector('.live-augment-grade'));
  }
  dom.window.close();
 }
 rows.splice(2);const sparse=new JSDOM(render({},'arena'));assert.equal(sparse.window.document.querySelectorAll('.live-augment-option').length,2);sparse.window.close();
});

test('R311 live Arena equipment sorts complete candidates before each display limit; other modes keep upstream order',()=>{
 const rows=Array.from({length:24},(_,i)=>({ids:[i+1],grade:'S',score:100-i,stats:{games:i+1}}));
 const rt=runtime();let source='arena';
 const deps={window:{deepLegendsRuntime:rt},LIVE_CORE_OPTION_LIMIT:5,liveAugmentRecommendationSource:()=>source,liveRecommendationsFor:d=>d.recommendations||{},recommendationCapabilities:()=>({hasAugments:true}),buildItemSetPayload:()=>({blocks:[]}),escapeHTML:String,renderConfigOption:o=>`<i data-id="${o.ids[0]}"></i>`,renderArenaBuildOption:(o,kind)=>`<i data-kind="${kind}" data-id="${o.ids[0]}"></i>`,renderLiveItemRanking:()=>'',renderCoreStats:()=>''};
 const render=Function(...Object.keys(deps),extract(read('gameplay.js'),'renderBuildRecommendation')+';return renderBuildRecommendation;')(...Object.values(deps));
 const build={coreOptions:rows,prismOptions:rows,bootOptions:rows,starterOptions:rows};
 const ids=(doc,selector)=>[...doc.querySelectorAll(selector+' [data-id]')].map(e=>+e.dataset.id);
 let dom=new JSDOM(render(build,{},[],{}));let doc=dom.window.document;
 assert.deepEqual(ids(doc,'.is-core'),[24,23,22,21,20]);
 assert.deepEqual(ids(doc,'.is-prismatic'),[24,23,22,21,20,19,18,17,16,15]);
 for(const group of ['boots','starter'])assert.deepEqual(ids(doc,'.build-summary-'+group),[24,23,22]);
 dom.window.close();source='hextech';dom=new JSDOM(render(build,{},[],{}));doc=dom.window.document;
 assert.deepEqual(ids(doc,'.item-core-column'),[1,2,3,4,5]);dom.window.close();
 const rankingDeps={window:{deepLegendsRuntime:rt},escapeHTML:String,renderItemIcon:()=>'',rate:String,compactNumber:String};
 const ranking=Function(...Object.keys(rankingDeps),extract(read('gameplay.js'),'renderLiveItemRanking')+';return renderLiveItemRanking;')(...Object.values(rankingDeps));
 const flat=dataRows().map((row,i)=>({...row,pickRate:100-Math.floor(i/2)}));dom=new JSDOM(ranking(flat,true));assert.deepEqual([...dom.window.document.querySelectorAll('article strong')].map(e=>e.textContent),[18,17,16,15,14,13,12,11,10,9].map(i=>'推荐'+i));dom.window.close();
 dom=new JSDOM(ranking(flat,false));assert.deepEqual([...dom.window.document.querySelectorAll('article strong')].map(e=>e.textContent),[2,1,4,3,6,5,8,7,10,9].map(i=>'推荐'+i));dom.window.close();
});
