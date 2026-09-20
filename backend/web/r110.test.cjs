'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const source=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
function extract(name){let start=source.indexOf(`function ${name}(`);assert.ok(start>=0,name);return source.slice(start,source.indexOf('\n  }',start)+4)}
function compile(names,deps){return Function(...Object.keys(deps),names.map(extract).join('\n')+`\nreturn {${names.join(',')}}`)(...Object.values(deps))}
const noop=()=>{};
test('R110 tab selection, progress and unrelated scroll preserve all existing avatar nodes',()=>{
 const dom=new JSDOM('<nav></nav>');try{
  const tabs=dom.window.document.querySelector('nav'),state={tabs:[{key:'a',icon:1,label:'Alpha'},{key:'b',icon:2,label:'Beta'},{key:'c',icon:3,label:'Gamma'}],activeTabs:{kr:'a'},settings:{}},scrolls=[];
  const {renderPlayerTabWorkspace:render}=compile(['renderPlayerTabWorkspace'],{state,document:dom.window.document,overviewWorkspace:()=>({tabs}),tabGroup:()=> 'kr',connected:()=>true,riotTab:()=>true,escapeHTML:String,assetPath:(_k,id)=>`/${id}`,assetIcon:url=>`<img data-queued-src="${url}">`,prepareImages:noop,requestAnimationFrame:f=>f(),updatePlayerTabScrollControls:(_g,selected)=>scrolls.push(selected)});
  render('kr');const images=[...tabs.querySelectorAll('img')],wraps=[...tabs.children];
  for(let n=0;n<10;n++)render('kr');
  state.activeTabs.kr='b';render('kr');
  assert.ok([...tabs.children].every((node,i)=>node===wraps[i]));
  state.tabs[2].loading=true;render('kr');
  assert.ok([...tabs.querySelectorAll('img')].every((img,i)=>img===images[i]));
  assert.deepEqual(scrolls,[true,...Array(10).fill(false),true,false]);
  assert.equal(tabs.querySelector('[data-player-tab="b"]').getAttribute('aria-selected'),'true');
 }finally{dom.window.close()}
});
test('R110 a newly streamed root card expands on the first click without replacing summary images',()=>{
 const dom=new JSDOM('<main></main>');try{
  const main=dom.window.document.querySelector('main'),tab={data:{player:{},matches:[{gameId:1}]},openMatches:new Set(),matchDetailTabs:new Map()};
  const renderMatch=()=>`<article class="match-entry" data-match-id="1"><div class="match-summary"><img data-queued-src="/hero"><button data-toggle-match="1" aria-expanded="${tab.openMatches.has('1')}" aria-label="toggle" data-tooltip="toggle"></button></div>${tab.openMatches.has('1')?'<div class="match-details">details</div>':''}</article>`;
  const deps={document:dom.window.document,renderMatch,prepareImages:noop,applyRenderedMetricStyles:noop,bindPlayerLinks:noop,bindMatchDetailControls:noop,bindOverviewShareControls:noop,bindMatchFilterControls:noop,bindRankHistoryControls:noop,bindRankedQueueControls:noop,bindMatchSentinel:noop,rerenderTab:()=>assert.fail('whole overview was rendered')};
  const ui=compile(['replaceMatchEntry','bindMatchEntryControls','bindOverviewContent'],deps);
  main.innerHTML=renderMatch();const card=main.firstChild,summary=card.firstChild,img=card.querySelector('img'),button=card.querySelector('button');
  ui.bindOverviewContent(card,tab);ui.bindOverviewContent(card,tab);
  for(let n=0;n<4;n++){
   button.click();assert.equal(Boolean(card.querySelector('.match-details')),n%2===0);
   assert.equal(main.firstChild,card);assert.equal(card.firstChild,summary);assert.equal(card.querySelector('img'),img);assert.equal(card.querySelector('button'),button);
  }
 }finally{dom.window.close()}
});
