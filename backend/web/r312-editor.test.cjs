'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read}=require('./r188-harness.cjs');
const {editor}=new Function('require','__dirname',read('r300.test.cjs').split("test('R300 portraits")[0]+';return {editor};')(require,__dirname);
const config={version:1,champions:{1:{ranked:{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]},spells:{spell1Id:4,spell2Id:14},items:{core:[3000],starter:[],situational:[]}}}}};
const save=s=>'[data-act="save"][data-section="'+s+'"]';
test('R312 section save preserves other dirty sections and validates only its own section',async()=>{
 const h=await editor({legacySections:false,saved:config});try{const d=h.w.document;
 await h.click('[data-col="sub"][data-row="3"]');await h.click('[data-tab="kit"]');await h.click('[data-spell="6"]');
 assert.equal(d.querySelector(save('items')).textContent,'已保存');assert(d.querySelector(save('items')).disabled);
 assert.equal(d.querySelector(save('spells')).textContent,'保存召唤师技能');await h.click(save('spells'));
 assert.deepEqual(h.saved.champions[1].ranked.spells,{spell1Id:6,spell2Id:14});assert.deepEqual(h.saved.champions[1].ranked.runes,config.champions[1].ranked.runes);
 assert(d.querySelector('[data-tab="runes"] .is-unsaved'));assert(d.querySelector(save('spells')).disabled);await h.click('[data-tab="runes"]');assert(d.querySelector('[data-col="sub"][data-row="3"].is-picked'));
 }finally{h.close();}
});
test('R312 every section clear confirms, cancels unchanged and saves only that section immediately',async()=>{
 for(const section of ['runes','spells','items']){const h=await editor({legacySections:false,saved:config});try{const d=h.w.document;if(section!=='runes')await h.click('[data-tab="kit"]');
 await h.click('[data-act="clear"][data-section="'+section+'"]');assert(d.querySelector('dialog[open]'));assert.match(d.querySelector('dialog[open]').textContent,/将立即清空并保存/);assert.equal(h.writes.length,0);
 await h.click('[data-section-clear-cancel]');assert.deepEqual(h.saved,config);
 await h.click('[data-act="clear"][data-section="'+section+'"]');await h.click('[data-section-clear-confirm]');assert.equal(h.saved.champions[1].ranked[section],undefined);
 for(const other of ['runes','spells','items'].filter(s=>s!==section))assert.deepEqual(h.saved.champions[1].ranked[other],config.champions[1].ranked[other]);assert.equal(h.writes.length,1);
 }finally{h.close();}}
});
test('R312 section clear failure retains original content and blocks duplicate submissions',async()=>{
 let release;const h=await editor({legacySections:false,saved:config,saveFails:true,beforeSave:()=>new Promise(r=>release=r)});try{const d=h.w.document;await h.click('[data-tab="kit"]');await h.click('[data-act="clear"][data-section="spells"]');await h.click('[data-section-clear-confirm]');assert(d.querySelector('[data-section-clear-confirm]').disabled);d.querySelector('[data-section-clear-confirm]').click();assert.equal(h.calls.filter(c=>c.method==='PUT').length,1);release();await new Promise(setImmediate);await new Promise(setImmediate);assert(d.querySelector('[data-clear-spell="0"]'));assert.deepEqual(h.saved,config);assert.match(d.querySelector('.ac-feedback').textContent,/清空失败/);
 }finally{h.close();}
});
test('R312 import ignores other section dirtiness, saves only equipment and confirms own changes',async()=>{
 const saved=structuredClone(config);delete saved.champions[1].ranked.items;const h=await editor({legacySections:false,saved});try{const d=h.w.document;await h.click('[data-col="sub"][data-row="3"]');await h.click('[data-tab="kit"]');assert.equal(d.querySelector(save('items')).textContent,'保存装备');
 await h.click('[data-act="import"][data-section="items"]');assert.equal(d.querySelector('dialog[open]'),null);assert.deepEqual(h.saved.champions[1].ranked.items.core,[3000]);assert.deepEqual(h.saved.champions[1].ranked.runes,saved.champions[1].ranked.runes);assert(d.querySelector('[data-tab="runes"] .is-unsaved'));
 await h.click('[data-remove="core:0"]');await h.click('[data-act="import"][data-section="items"]');assert.equal(d.querySelector('dialog[open]').getAttribute('aria-label'),'装备有未保存的修改');assert.match(d.querySelector('dialog[open]').textContent,/符文和召唤师技能不受影响/);await h.click('[data-act="stay"]');assert.equal(d.querySelector('[data-build="core:0"] img'),null);
 await h.click('[data-act="import"][data-section="items"]');await h.click('[data-act="discard"]');assert(d.querySelector('[data-tab="runes"] .is-unsaved'));assert.deepEqual(h.saved.champions[1].ranked.items.core,[3000]);
 }finally{h.close();}
});

test('R312 section save completes after switching mode and rebases the retained other-section draft',async()=>{
 let release,started=0;const h=await editor({legacySections:false,saved:config,beforeSave:()=>++started===1?new Promise(r=>release=r):Promise.resolve()});try{const d=h.w.document;await h.click('[data-col="sub"][data-row="3"]');await h.click('[data-tab="kit"]');await h.click('[data-spell="6"]');await h.click(save('spells'));await h.click('[data-mode="normal"]');release();await new Promise(setImmediate);await new Promise(setImmediate);await h.click('[data-mode="ranked"]');
 assert.equal(h.saved.champions[1].ranked.spells.spell1Id,6);assert(d.querySelector('[data-tab="runes"] .is-unsaved'));assert(d.querySelector('[data-col="sub"][data-row="3"].is-picked'));assert.doesNotMatch(d.querySelector('.ac-feedback').textContent,/配置已更新/);await h.click('[data-act="save"]');assert.equal(h.saved.champions[1].ranked.runes.selectedPerkIds[5],14);
 }finally{h.close();}
});
test('R312 editing another section during a section save retains its draft and forbids repeated section actions',async()=>{
 let release;const h=await editor({legacySections:false,saved:config,beforeSave:()=>new Promise(r=>release=r)});try{const d=h.w.document;await h.click('[data-tab="kit"]');await h.click('[data-spell="6"]');await h.click(save('spells'));assert(d.querySelector('[data-act="import"][data-section="items"]').disabled);assert(d.querySelector(save('spells')).disabled);await h.click('[data-remove="core:0"]');release();await new Promise(setImmediate);await new Promise(setImmediate);
 assert.deepEqual(h.saved.champions[1].ranked.items.core,[3000]);assert.equal(d.querySelector('[data-build="core:0"] img'),null);assert.equal(d.querySelector('[data-save-state="items"]').textContent,'未保存');assert.equal(d.querySelector(save('spells')).textContent,'已保存');assert.equal(d.querySelector('[data-act="import"][data-section="items"]').disabled,false);assert.equal(h.writes.length,1);
 }finally{h.close();}
});

test('R312 whole-entry save queues behind a section save while later rune edits remain writable',async()=>{
 let release,started=0;const h=await editor({legacySections:false,saved:config,beforeSave:()=>++started===1?new Promise(r=>release=r):Promise.resolve()});try{const d=h.w.document;await h.click('[data-tab="kit"]');await h.click('[data-spell="6"]');await h.click(save('spells'));await h.click('[data-tab="runes"]');await h.click('[data-col="sub"][data-row="3"]');assert.equal(d.querySelector('.ac-mode-actions [data-act="save"]').disabled,false);await h.click('.ac-mode-actions [data-act="save"]');assert.equal(started,1);release();for(let i=0;i<8;i++)await new Promise(setImmediate);assert.equal(started,2);assert.equal(h.writes.length,2);assert.equal(h.saved.champions[1].ranked.spells.spell1Id,6);assert.equal(h.saved.champions[1].ranked.runes.selectedPerkIds[5],14);assert.doesNotMatch(d.querySelector('.ac-feedback').textContent,/配置已更新/);
 }finally{h.close();}
});
