'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read}=require('./r188-harness.cjs');
const {editor}=new Function('require','__dirname',read('r300.test.cjs').split("test('R300 portraits")[0]+';return {editor};')(require,__dirname);
const config={version:1,champions:{1:{ranked:{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]},spells:{spell1Id:4,spell2Id:14},items:{core:[3000],starter:[],situational:[]}}}}};
const tick=()=>new Promise(setImmediate);
test('R309 section dots and shared tooltips exclude mode dots; section reset preserves other drafts',async()=>{
 const h=await editor({legacySections:false,saved:config});try{const d=h.w.document;
 assert.equal(d.querySelectorAll('.ac-mode .ac-state-dot').length,0);
 for(const sel of ['[data-pos=""]','[data-champ="1"]','[data-tab="runes"]','[data-tab="kit"]'])assert(d.querySelector(sel+' .is-saved'),sel);
 assert.equal(d.querySelectorAll('.ac-editor [title],.ac-dots [title]').length,0);
 assert.equal(d.querySelector('[data-pos=""] .ac-state-dot').dataset.tooltip,'通用 · 已保存');
 await h.click('[data-col="sub"][data-row="3"]');await h.click('[data-tab="kit"]');await h.click('[data-section="spells"][data-act="clear"]');assert(d.querySelector('dialog[open]'));await h.click('[data-section-clear-confirm]');
 assert.equal(d.querySelector('.ac-spells'),null);assert(d.querySelector('[data-act="start-spells"]'));assert.equal(d.querySelector('[data-save-state="spells"]').textContent,'未配置');assert(d.querySelector('[data-tab="runes"] .is-unsaved'));
 await h.click('[data-section="items"][data-act="clear"]');await h.click('[data-section-clear-confirm]');assert.equal(d.querySelector('.ac-items'),null);assert(d.querySelector('[data-act="start-items"]'));
 await h.click('[data-act="start-spells"]');await h.click('[data-spell="6"]');await h.click('[data-section="spells"][data-act="revert"]');assert(d.querySelector('[data-act="start-spells"]'));assert.equal(d.querySelector('.ac-items'),null);
 await h.click('[data-act="save"]');assert.equal(h.saved.champions[1].ranked.spells,undefined);assert.equal(h.saved.champions[1].ranked.items,undefined);assert.equal(h.saved.champions[1].ranked.runes.selectedPerkIds[5],14);
 await h.click('[data-act="start-items"]');assert(d.querySelector('.ac-items'));await h.click('[data-act="start-spells"]');assert(d.querySelector('.ac-spells'));
 }finally{h.close();}
});
test('R309 all-clear cancels unchanged, sends one immediate DELETE, clears drafts and saved markers',async()=>{
 let release;const h=await editor({legacySections:false,saved:config,beforeSave:()=>new Promise(r=>release=r)});try{const d=h.w.document;
 await h.click('[data-col="sub"][data-row="3"]');h.w.deepLegendsAutoConfig.flushDraft();
 const before=h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked');
 await h.click('[data-act="clear-all"]');assert.match(d.querySelector('dialog').textContent,/清空后立刻生效/);assert.equal(h.calls.filter(c=>c.method==='DELETE').length,0);
 await h.click('[data-clear-cancel]');assert.equal(h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked'),before);
 await h.click('[data-act="clear-all"]');await h.click('[data-clear-confirm]');assert(d.querySelector('[data-clear-confirm]').disabled);d.querySelector('[data-clear-confirm]').click();
 assert.equal(h.calls.filter(c=>c.method==='DELETE').length,1);assert(d.querySelector('.ac-tree'));
 release();await tick();await tick();assert.equal(h.saved.champions[1].ranked,undefined);assert.equal(h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked'),null);
 assert.equal(d.querySelectorAll('.ac-state-dot').length,0);assert.equal(d.querySelector('.ac-tree'),null);assert(d.querySelector('[data-act="start-runes"]'));assert(d.querySelector('[data-act="clear-all"]').disabled);
 await h.click('[data-tab="kit"]');assert(d.querySelector('[data-act="start-spells"]'));assert(d.querySelector('[data-act="start-items"]'));assert.equal(d.querySelectorAll('[data-act="revert"]:not(:disabled)').length,0);
 assert.match(d.querySelector('.ac-feedback').textContent,/已清空/);
 }finally{h.close();}
});
test('R309 delete failure preserves displayed data and draft markers',async()=>{
 const h=await editor({legacySections:false,saved:config,saveFails:true});try{const d=h.w.document;await h.click('[data-col="sub"][data-row="3"]');h.w.deepLegendsAutoConfig.flushDraft();const before=h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked');await h.click('[data-act="clear-all"]');await h.click('[data-clear-confirm]');assert(d.querySelector('.ac-tree'));assert(d.querySelector('[data-pos=""] .is-unsaved'));assert.equal(h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked'),before);assert.match(d.querySelector('.ac-feedback').textContent,/清空失败/);assert.equal(d.querySelector('dialog[open]'),null);assert.deepEqual(h.saved,config);
 }finally{h.close();}
});
test('R309 imports save automatically and keep recoverable drafts on save failure',async()=>{
 for(const saveFails of [false,true]){const h=await editor({legacySections:false,saveFails,runeIDs:[1,2,3,4,12,14,5008,5008,5001]});try{const d=h.w.document;await h.click('[data-act="import"]');assert.equal(h.calls.filter(c=>c.method==='PUT'&&c.url.startsWith('/api/auto-config/champions/')).length,1);
 if(saveFails){assert.match(d.querySelector('.ac-feedback').textContent,/推荐已导入，但保存失败/);assert(h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked'));assert.equal(d.querySelector('[data-act="save"]').disabled,false);}else{assert.match(d.querySelector('.ac-feedback').textContent,/推荐已导入并保存/);assert.equal(h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked'),null);assert.equal(d.querySelector('[data-act="save"]').disabled,true);}
 }finally{h.close();}}
});
