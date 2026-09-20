'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('jsdom');
const read=f=>fs.readFileSync(path.join(__dirname,'../backend/web',f),'utf8');
test('offline national group remains reachable from a pro tab',()=>{
 const src=read('gameplay.js');
 const functions=src.slice(src.indexOf('  function visiblePlayerGroups('),src.indexOf('  function playerGroupButton('));
 let selected='';const current={key:'current',current:true};
 new Function('PLAYER_GROUPS','playerGroupCount','connected','state','activeTab','selectPlayerTab','assert',`${functions};assert.deepEqual(visiblePlayerGroups(),['players','kr','pro']);selectPlayerGroup('players');`)({players:'国服',kr:'韩服',pro:'职业'},group=>group==='pro'?1:0,()=>false,{tabs:[current]},()=>null,key=>selected=key,assert);
 assert.equal(selected,'current');
 assert.match(src,/const disabled = false/);
});
test('ordinary and centered artwork use different face coordinates, including load failure',()=>{
 const dom=new JSDOM('',{runScripts:'outside-only'}),w=dom.window;
 try {
  w.eval(read('overview-art.js'));
  for(const [id,y] of [[103086,.16],[101000,.20]]) {
   w.document.body.innerHTML=w.deepLegendsOverviewArt.render({backgroundSkinId:id,backgroundSource:'gtimg',backgroundPath:'/ordinary.jpg'});
   const img=w.document.querySelector('img'),holder=img.parentNode;
   Object.defineProperties(holder,{clientWidth:{value:2000},clientHeight:{value:140}});
   Object.defineProperties(img,{naturalWidth:{value:1200},naturalHeight:{value:700},complete:{value:true}});
   w.deepLegendsOverviewArt.prepare(w.document);
   assert.ok(Math.abs(parseFloat(img.style.top)+parseFloat(img.style.height)*y-70)<.01);
  }
  w.document.body.innerHTML=w.deepLegendsOverviewArt.render({backgroundSkinId:103086,backgroundPosterPath:'/ahri_centered_86.jpg',backgroundSource:'gtimg',backgroundPath:'/ordinary.jpg'});
  w.deepLegendsOverviewArt.prepare(w.document);
  const img=w.document.querySelector('img');img.dispatchEvent(new w.Event('error'));
  assert.equal(img.parentNode.dataset.composition,'ordinary');assert.equal(img.parentNode.dataset.focusSkin,'103086');
 } finally {w.close();}
});
test('unknown reward quantities are not manufactured as one',()=>{
 const src=read('suite.js');assert.match(src,/数量待客户端确认/);assert.doesNotMatch(src,/Math.max\(1, Number\(reward.quantity/);
});
