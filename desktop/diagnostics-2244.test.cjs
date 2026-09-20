'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('jsdom');
const read=f=>fs.readFileSync(path.join(__dirname,'../web',f),'utf8');
test('2244 skill summary uses real main/sub priorities after R, without repeated labels',()=>{
 const dom=new JSDOM('',{runScripts:'outside-only'}),w=dom.window;
 try {
  const source=read('gameplay.js');
  const fn=source.slice(source.indexOf('  function renderSkillPlan('),source.indexOf('  function abilityTooltip('));
  w.eval(`const escapeHTML=x=>String(x);const assetIcon=()=>'';const abilityTooltip=()=>'';${fn};window.renderSkillPlan=renderSkillPlan;`);
  w.document.body.innerHTML=w.renderSkillPlan({skillPriority:['Q','E','W'],skillOrder:['Q','E','W']},[]);
  assert.equal(w.document.querySelector('.skill-priority-summary').textContent,'主Q副E');
  assert.equal(w.document.querySelector('.skill-priority').lastElementChild.className,'skill-priority-summary');
  assert.equal(w.document.querySelectorAll('.skill-copy small').length,0);
  assert.equal(w.document.querySelectorAll('.skill-icon-button').length,4);
  assert.equal(w.renderSkillPlan({skillPriority:[]},[]).includes('主Q'),false);
 }finally{w.close();}
});
test('2244 banner exact animation, no video for unknown skin or reduced motion',()=>{
 for(const reduced of [false,true]){
  const dom=new JSDOM('',{runScripts:'outside-only'}),w=dom.window;
  try{
   w.matchMedia=()=>({matches:reduced,addEventListener(){}});w.eval(read('overview-art.js'));
   const player={backgroundSkinId:103086,backgroundPosterPath:'/lol-game-data/assets/ahri_centered_86.jpg',backgroundSource:'gtimg',backgroundPath:'/fallback.jpg',backgroundVideoPath:'/lol-game-data/assets/centered.webm'};
   w.document.body.innerHTML=w.deepLegendsOverviewArt.render(player);
   assert.equal(w.document.querySelectorAll('video').length,reduced?0:1);
   assert.equal(w.document.querySelector('.overview-art').dataset.focusSkin,'103086');
   assert.equal(w.document.querySelector('[data-overview-poster]').getAttribute('data-queued-src'),'/api/image?path=%2Flol-game-data%2Fassets%2Fahri_centered_86.jpg');
   assert.equal(w.deepLegendsOverviewArt.render({backgroundSkinId:123456}), '');
  }finally{w.close();}
 }
});
test('event cards group presentation but retain all entitlement rows; unknown events stay separate',()=>{
 const dom=new JSDOM('',{runScripts:'outside-only'}),w=dom.window;
 try{
  const source=read('suite.js'),fn=source.slice(source.indexOf('  function claimEventGroups('),source.indexOf('  function renderClaims('));
  w.eval(`const escapeHTML=x=>String(x),checked=()=>'',state={selectedClaims:new Set()},claimActionable=()=>true,choiceValid=()=>true,claimSelectionKeys=x=>[x.key];const claimRow=(x,compact)=>compact?'<div data-entitlement>'+x.key+'</div>':'<article>'+x.key+'</article>';${fn};window.groups=claimEventGroups;`);
  w.document.body.innerHTML=w.groups([{eventId:'a',eventName:'活动A',key:'grant:1'},{eventId:'a',eventName:'活动A',key:'grant:2'},{eventId:'b',eventName:'活动B',key:'grant:3'},{key:'grant:4'}]);
  assert.equal(w.document.querySelectorAll('.claim-event-card').length,2);
  assert.equal(w.document.querySelectorAll('article').length,3);
  assert.equal(w.document.querySelector('.claim-event-card').querySelectorAll('[data-entitlement]').length,2);
  assert.deepEqual([...w.document.querySelectorAll('h3')].map(x=>x.textContent),['活动A','活动B','活动归属待确认']);
 }finally{w.close();}
});
test('2244 installer uses the actual STM_SETIMAGE constant and distribution is automatic',()=>{
 const nsis=fs.readFileSync(path.join(__dirname,'nsis/installer.nsh'),'utf8');
 assert.match(nsis,/SendMessage \$DLIcon \$\{STM_SETIMAGE\} 1 \$DLIconHandle/);
 assert.doesNotMatch(nsis,/SendMessage \$DLIcon 0x0170/);
 assert.doesNotMatch(read('champions.js'),/查看四阶段品质分布/);
 assert.match(read('champions.js'),/root\.querySelector\("\.mayhem-rarity-panel"\)/);
 assert.doesNotMatch(read('pro-players.js'),/\$\{source\} · \$\{escape\(updated\)\}/);
});
