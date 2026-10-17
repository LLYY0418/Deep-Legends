'use strict';
process.env.TZ='Asia/Shanghai';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile,escapeHTML}=require('./r188-harness.cjs'),{expand}=require('./r211-harness-support.cjs');
const source=process.env.R257_GAMEPLAY_SOURCE ? fs.readFileSync(process.env.R257_GAMEPLAY_SOURCE,'utf8') : read('gameplay.js');
const fixedNow=Date.parse('2026-10-08T08:00:00Z');
class FixedDate extends Date {constructor(...args){super(...(args.length?args:[fixedNow]));}}
const progress={season:'S26',complete:true,scanned:208,tableSupported:true,message:'已统计 208 场',upstreamCapped:true,rankedOldestAt:1775889886204,mayhemOldestAt:1778580000000};
const ranks=[{queueType:'RANKED_SOLO_5X5',wins:60,losses:40},{queueType:'RANKED_FLEX_SR',wins:150,losses:163}];
const noop=()=>{};
function api(dom,s=source,deps={}){
 const dependencies={Date:FixedDate,document:dom.window.document,escapeHTML,number:String,kda:String,percent:String,iconFigure:()=>'',prepareImages:noop,rerenderTab:noop,openOverviewSubpage:noop,renderMasteryDetails:()=>'',...deps};
 const names=['renderSeasonDataNote','bindSeasonDataNotes','renderChampionStats','renderOverviewSubpage','renderChampionTable'];
 return compile(s,expand(s,names,dependencies),dependencies);
}
function fixture(queue='440',p=progress,rs=ranks,s=source){
 const dom=new JSDOM('<main></main>',{pretendToBeVisual:true}),root=dom.window.document.querySelector('main');
 const data={available:true,queue,seasonStatsProgress:p,overall:{games:208,wins:97,losses:111,winRate:47},rows:[]};
 const tab={key:'synthetic',data:{ranks:rs},overviewSubpage:'champion-table',championTableQueue:queue,overviewSubpageState:{status:'ready',data}};
 const f=api(dom,s);f.renderOverviewSubpage(root,tab);return{dom,root,tab,f};
}
function lines(button){return JSON.parse(button.dataset.seasonDataNote);}
function check(s){
 for(const [queue,date,official] of [['420','4月11日',false],['440','4月11日',true],['mayhem','5月12日',false]]){
  const x=fixture(queue,progress,ranks,s);try{
   const button=x.root.querySelector('[data-season-data-note]');assert(button,queue);
   assert.equal(button.parentElement,x.root.querySelector('.champion-table-title'));
   assert.equal(button.previousElementSibling.tagName,'H2');assert.equal(button.nextElementSibling,null);
   assert(lines(button).some(line=>line.includes(date)),JSON.stringify(lines(button)));
   assert.equal(lines(button).some(line=>line.includes('官方总场次')),official);
   if(official)assert(lines(button).includes('已统计 208 场 · 官方总场次 313 场'));
   if(queue==='mayhem')assert(lines(button).includes('已统计 208 场'));
  }finally{x.dom.window.close();}
 }
 for(const p of [{...progress,upstreamCapped:false},{...progress,rankedOldestAt:0},{...progress,rankedOldestAt:'broken'},null,{...progress,foreign:true}]){
  const x=fixture('440',p,ranks,s);assert.equal(x.root.querySelector('.season-data-note'),null);assert.equal(x.root.querySelector('[data-season-data-note]'),null);x.dom.window.close();
 }
 for(const rs of [[],[{...ranks[1],wins:0,losses:1}],[{...ranks[1],wins:null}],[{...ranks[1],losses:-1}]]){
  const x=fixture('440',progress,rs,s);assert.equal(lines(x.root.querySelector('button.season-data-note')).length,3,'invalid total omits whole count line');x.dom.window.close();
 }
}
test('R257 three tabs use stream dates and valid official totals; uncapped/foreign leave no icon slot',()=>check(source));
test('R257 local date format includes a different year, independent per-stream visibility and zero official totals',()=>{
 const dom=new JSDOM(''),f=api(dom),note=(p,q,g,rs)=>new JSDOM(f.renderSeasonDataNote(p,q,g,rs)).window.document.querySelector('button');
 assert(lines(note({...progress,rankedOldestAt:Date.parse('2025-12-31T16:01:00Z')},'440',208,ranks))[1].includes('1月1日'));
 assert(lines(note({...progress,rankedOldestAt:Date.parse('2025-12-30T16:01:00Z')},'440',208,ranks))[1].includes('2025年12月31日'));
 assert.equal(f.renderSeasonDataNote({...progress,mayhemOldestAt:0},'mayhem',208,ranks),'');
 assert(f.renderSeasonDataNote({...progress,rankedOldestAt:0},'mayhem',208,ranks));
 assert(lines(note(progress,'440',0,[{...ranks[1],wins:0,losses:0}])).includes('已统计 0 场 · 官方总场次 0 场'));
 dom.window.close();
});
test('R257 updated scope leaves overview without a note and keeps the table arrow working',()=>{
 const dom=new JSDOM('<main></main>',{pretendToBeVisual:true}),root=dom.window.document.querySelector('main'),f=api(dom);
 for(const p of [progress,{...progress,upstreamCapped:false},null]){
  root.innerHTML=f.renderChampionStats([],{games:208},p);
  assert.equal(root.querySelector('.season-data-note'),null);assert.equal(root.querySelector('[data-season-data-note]'),null);
 }
 root.innerHTML=f.renderChampionStats([],{games:208},progress);
 const arrow=root.querySelector('[data-overview-subpage]');assert.match(arrow.previousSibling.textContent,/已统计 208 场/);
 let navigations=0;const deps={document:dom.window.document,state:{},requestAnimationFrame:noop,setTimeout:noop,matchTierScrollRoot:()=>null,bindSummonerCopy:noop,bindPlayerLinks:noop,bindOverviewShareControls:noop,bindMatchFilterControls:noop,bindRankHistoryControls:noop,bindRankedQueueControls:noop,bindMayhemRatingControls:noop,bindMatchEntryControls:noop,bindMatchDetailControls:noop,bindMatchSentinel:noop,openOverviewSubpage:()=>navigations++,rerenderTab:noop};
 compile(source,expand(source,['bindOverviewContent'],deps),deps).bindOverviewContent(root,{});
 arrow.click();assert.equal(navigations,1);assert.equal(dom.window.document.querySelector('.season-data-note-popover'),null);
 dom.window.close();
});
test('R257 focus/Escape/hover/touch and body portal placement/lifecycle',async()=>{
 const x=fixture(),{dom,root}=x,doc=dom.window.document,button=root.querySelector('.season-data-note'),panel=doc.querySelector('.season-data-note-popover');
 Object.defineProperty(dom.window,'innerWidth',{value:800});Object.defineProperty(dom.window,'innerHeight',{value:600});
 Object.defineProperty(panel,'offsetWidth',{value:288});Object.defineProperty(panel,'offsetHeight',{value:170});
 button.getBoundingClientRect=()=>({left:760,top:550,bottom:576,width:26});
 button.focus();assert.equal(panel.hidden,false);assert.equal(panel.parentElement,doc.body);assert.equal(panel.dataset.placement,'top');assert.equal(panel.style.left,'500px');assert(Number.parseFloat(panel.style.top)<550);
 assert.match(panel.textContent,/4月11日/);assert.equal(button.getAttribute('aria-describedby'),panel.id);
 doc.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert(panel.hidden);assert.equal(button.getAttribute('aria-expanded'),'false');
 button.blur();button.focus();assert(!panel.hidden);button.blur();assert(panel.hidden);
 button.dispatchEvent(new dom.window.MouseEvent('pointerover',{bubbles:true}));assert(!panel.hidden);button.dispatchEvent(new dom.window.MouseEvent('pointerout',{bubbles:true}));assert(panel.hidden);
 const touch=new dom.window.Event('pointerdown',{bubbles:true});Object.defineProperty(touch,'pointerType',{value:'touch'});button.dispatchEvent(touch);button.focus();button.click();assert(!panel.hidden);button.click();assert(panel.hidden);
 button.click();doc.dispatchEvent(new dom.window.Event('scroll'));assert(panel.hidden);
 button.click();button.remove();await new Promise(setImmediate);assert(panel.hidden);dom.window.close();
});
test('R257 assertions kill always-visible, wrong-stream and inverted official-total mutations',()=>{
 const mutations=[['cap','!progress?.upstreamCapped || ',''],['date','progress?.mayhemOldestAt : progress?.rankedOldestAt','progress?.rankedOldestAt : progress?.mayhemOldestAt'],['total','official>=games','official<games']];
 for(const [name,from,to] of mutations){const mutant=source.replace(from,to);assert.notEqual(mutant,source,name);assert.throws(()=>check(mutant),assert.AssertionError,name);}
});
test('R257 copy respects existing static wording guards and reuses R241 styling',()=>{
 const {extract}=require('./r188-harness.cjs'),body=extract(source,'renderSeasonDataNote');assert.doesNotMatch(body,/统计口径|上游公布的/);
 const css=read('gameplay.css');assert.match(source,/panel.className="match-tags-popover season-data-note-popover"/);assert.match(css,/season-data-note svg \{ width:16px; height:16px/);assert.match(css,/champion-table-title \{ display:flex; align-items:center; gap:8px/);assert.match(css,/season-data-note-popover \{[^}]*width:288px/);
});
