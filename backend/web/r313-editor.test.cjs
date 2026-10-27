'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,extract}=require('./r188-harness.cjs');
const {editor}=new Function('require','__dirname',read('r300.test.cjs').split("test('R300 portraits")[0]+';return {editor};')(require,__dirname);
const config={version:1,champions:{1:{ranked:{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]},spells:{spell1Id:4,spell2Id:6},items:{core:[3000],starter:[],situational:[]}}}}};
function toast(h){const d=h.w.document,n=d.createElement('div');n.className='toast';n.hidden=true;d.body.append(n);let seq=0;const jobs=new Map(),calls=[];h.w.deepLegendsToast=Function('el','setTimeout','clearTimeout','let toastTimer=0;'+extract(read('app.js'),'showToast')+';return showToast;')({toast:n},(fn,ms)=>{jobs.set(++seq,{fn,ms});calls.push([n.textContent,ms]);return seq;},id=>jobs.delete(id));return{n,jobs,calls};}
const errors=d=>[...d.querySelectorAll('[data-feedback-section]')].filter(n=>!n.hidden).map(n=>[n.dataset.feedbackSection,n.textContent]);
test('R313 equipment clear/import use the latest 1500ms toast outside closed dialogs; headers stay silent',async()=>{
 const h=await editor({legacySections:false,saved:config});try{const d=h.w.document,t=toast(h);await h.click('[data-tab="kit"]');await h.click('[data-act="clear"][data-section="items"]');await h.click('[data-section-clear-confirm]');assert.deepEqual(t.calls,[['已清空并保存装备',1500]]);assert(t.n.isConnected);assert(!t.n.closest('dialog'));assert.deepEqual(errors(d),[]);await h.click('[data-act="import"][data-section="items"]');assert.deepEqual(t.calls.at(-1),['推荐已导入并保存装备',1500]);assert.equal(t.jobs.size,1);assert.equal(t.n.hidden,false);[...t.jobs.values()][0].fn();assert.equal(t.n.hidden,true);await h.click('[data-act="save"][data-section="items"]');assert.equal(t.calls.length,2);
 }finally{h.close();}
});
test('R313 spells import changes only spells and confirms only its own dirty draft',async()=>{
 const h=await editor({legacySections:false,saved:config});try{const d=h.w.document,t=toast(h);await h.click('[data-tab="kit"]');await h.click('[data-act="import"][data-section="spells"]');assert.deepEqual(h.saved.champions[1].ranked.spells,{spell1Id:4,spell2Id:14});for(const s of ['runes','items'])assert.deepEqual(h.saved.champions[1].ranked[s],config.champions[1].ranked[s]);assert.deepEqual(t.calls,[['推荐已导入并保存召唤师技能',1500]]);await h.click('[data-spell="6"]');await h.click('[data-act="import"][data-section="spells"]');assert.equal(d.querySelector('dialog[open]').getAttribute('aria-label'),'召唤师技能有未保存的修改');assert.match(d.querySelector('dialog[open]').textContent,/符文和装备不受影响/);
 }finally{h.close();}
});
test('R313 section validation and failed saves remain in their own section',async()=>{
 const h=await editor({legacySections:false,saved:config,saveFails:true});try{const d=h.w.document;await h.click('[data-pick-path="primary"][data-style="8100"]');assert.deepEqual(errors(d).map(e=>e[0]),['runes']);await h.click('[data-tab="kit"]');assert.deepEqual(errors(d),[]);await h.click('[data-spell="14"]');await h.click('[data-act="save"][data-section="spells"]');assert.deepEqual(errors(d).map(e=>e[0]),['spells']);assert.match(errors(d)[0][1],/保存失败/);assert(!d.querySelector('[data-page-feedback]').textContent);
 }finally{h.close();}
});
test('R313 missing spell recommendation and inactive selection hint stay local',async()=>{
 const empty=structuredClone(config);delete empty.champions[1].ranked.spells;const h=await editor({legacySections:false,saved:empty,detail:async()=>({ok:true,json:async()=>({})})});try{const d=h.w.document;await h.click('[data-tab="kit"]');assert.doesNotMatch(d.querySelector('.ac-section-head').textContent,/点一个技能/);await h.click('[data-act="import"][data-section="spells"]');assert.deepEqual(errors(d),[['spells','当前没有可导入的召唤师技能推荐']]);await h.click('[data-act="start-spells"]');assert.match(d.querySelector('.ac-section-head').textContent,/点一个技能/);
 }finally{h.close();}
});
test('R313 configuration conflict stays below mode controls with reload action',async()=>{
 const h=await editor({legacySections:false,saved:config});try{const d=h.w.document;await h.click('[data-col="sub"][data-row="3"]');const next=structuredClone(config);next.champions[1].ranked.runes.selectedPerkIds[5]=14;h.w.dispatchEvent(new h.w.CustomEvent('deep-legends:auto-config-settings',{detail:next}));const page=d.querySelector('[data-page-feedback]');assert.match(page.textContent,/配置已更新/);assert(page.querySelector('[data-act="reload"]'));assert.equal(page.previousElementSibling.className,'ac-modes');assert.equal(d.querySelector('[data-feedback-section="runes"]').textContent,'');
 }finally{h.close();}
});
