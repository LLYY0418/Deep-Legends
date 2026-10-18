'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R285_GAMEPLAY_SOURCE||__dirname+'/gameplay.js','utf8');
const spells=require('../testdata/r285-summoner-spells.json');
test('R285 P4 ordinary and Arena spell tooltips preserve catalog description and empty fallback',()=>{
 const f=compile(source,['spellIconFigure','plainText','assetIcon','iconFigure','proxyAsset'],{state:{summonerSpells:{spells:[...spells,{id:999,name:'无说明',iconPath:'ddragon:empty.png'}]}},escapeHTML,assetPath:(_kind,id)=>'ddragon:'+id,pendingCatalogIcon:()=>''});
 for(const id of [4,2201,2202,999]){
  const s=spells.find(s=>s.id===id)||{name:'无说明'},html=f.spellIconFigure(id,'small'),dom=new JSDOM(html);
  try{const tips=[...dom.window.document.querySelectorAll('[data-tooltip]')];assert.equal(tips.length,1);assert.equal(tips[0].getAttribute('data-tooltip'),s.description?s.name+'\n'+s.description:s.name);assert.equal(dom.window.document.querySelector('img').alt,s.name)}finally{dom.window.close()}
 }
});
test('R285 P6 hidden player emits one empty, inaccessible history slot',()=>{
 const f=compile(source,['renderInsightMatches']);
 const dom=new JSDOM(f.renderInsightMatches({hidden:true}));try{
  const row=dom.window.document.querySelector('.insight-match-row.is-hidden-player');assert(row);assert.equal(row.getAttribute('aria-hidden'),'true');assert.equal(row.textContent,'');assert.equal(row.children.length,0);
 }finally{dom.window.close()}
 const css=fs.readFileSync(process.env.R285_GAMEPLAY_CSS_SOURCE||__dirname+'/gameplay.css','utf8');assert(!css.includes(':has(.live-player.is-hidden)'));assert(css.includes('grid-template-rows: subgrid'));
});
