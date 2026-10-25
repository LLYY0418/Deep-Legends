'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const read=n=>fs.readFileSync(path.join(__dirname,n),'utf8'),tick=()=>new Promise(setImmediate);
let source=read('r300.test.cjs').split("test('R300 portraits")[0];
source=source.replace("if(url==='/api/auto-config/settings')value=saved;","if(url==='/api/auto-config/settings'){if(opts.method==='PUT'){saved=JSON.parse(opts.body);saved.version++;}value=saved;}");
source=source.replace('get saved(){return clone(saved);}', 'get saved(){return clone(saved);},update(fn){fn(saved);saved.version++;}');
const {editor}=new Function('require','__dirname',source+';return {editor};')(require,__dirname);
test('R305 switches do not invalidate current draft and recommendation auto save',async()=>{
 const h=await editor();try{const d=h.w.document;for(const key of ['enabled','runes','spells','items']){const n=d.querySelector(`[data-enable="${key}"]`);n.checked=!n.checked;n.dispatchEvent(new h.w.Event('change',{bubbles:true}));await tick();await tick();}
 await h.click('[data-tab="kit"]');await h.click('[data-spell="4"]');await h.click('[data-slot="1"]');await h.click('[data-spell="14"]');await h.click('[data-act="save"]');assert.equal(h.writes.length,1);assert.equal(d.querySelector('dialog'),null);await h.click('[data-act="import"]');assert.equal(h.writes.length,2);assert.match(d.querySelector('.ac-feedback').textContent,/推荐已导入并保存/);assert.equal(d.querySelector('[data-act="save"]').disabled,true);await h.click('[data-act="save"]');assert.equal(h.writes.length,2);
 }finally{h.close();}
});
test('R305 only the same champion and mode conflicts; reload discards and errors never open a modal',async()=>{
 const h=await editor();try{const d=h.w.document;await h.click('[data-tab="kit"]');await h.click('[data-spell="4"]');await h.click('[data-slot="1"]');await h.click('[data-spell="14"]');h.update(s=>s.champions[2]={ranked:{spells:{spell1Id:4,spell2Id:14}}});await h.click('[data-act="save"]');assert.equal(h.writes.length,1);
 await h.click('[data-act="swap"]');h.update(s=>s.champions[1].ranked={spells:{spell1Id:4,spell2Id:6}});await h.click('[data-tab="kit"]');await h.click('[data-act="clear"][data-section="spells"]');await h.click('[data-act="save"]');assert.equal(h.writes.length,1);assert.match(d.querySelector('.ac-feedback').textContent,/配置已更新/);assert(d.querySelector('[data-act="reload"]'));assert.equal(d.querySelector('dialog'),null);await h.click('[data-act="reload"]');assert.equal(d.querySelector('[data-save-state]').textContent.startsWith('已保存'),true);
 }finally{h.close();}
});
test('R305 incomplete runes and single spell explain disabled save',async()=>{
 const h=await editor();try{const d=h.w.document;await h.click('[data-tab="kit"]');await h.click('[data-spell="4"]');assert(d.querySelector('[data-act="save"]').disabled);assert.match(d.querySelector('.ac-feedback').textContent,/召唤师技能只选了一个/);await h.click('[data-clear-spell="0"]');await h.click('[data-tab="runes"]');await h.click('[data-pick-path="primary"]');assert(d.querySelector('[data-act="save"]').disabled);assert.match(d.querySelector('.ac-feedback').textContent,/符文还差 \d+ 个没选/);assert.equal(d.querySelector('dialog'),null);
 }finally{h.close();}
});
module.exports={editor};

test('R305 copying another mode keeps dirty content and saves with persisted content base',async()=>{
 const h=await editor();try{await h.click('[data-tab="kit"]');await h.click('[data-spell="4"]');await h.click('[data-slot="1"]');await h.click('[data-spell="14"]');h.w.deepLegendsAutoConfig.flushDraft();const disk=JSON.parse(h.w.localStorage.getItem('lol-loot-auto-config-draft-1-ranked'));assert.equal(typeof disk.baseContent,'string');assert.equal(disk.revision,undefined);await h.click('[data-act="copy"]');h.w.document.querySelector('[data-copy-target="normal"]').click();await h.click('dialog [data-act="copy-save"]');assert.equal(h.w.document.querySelector('[data-save-state]').textContent,'未保存');await h.click('[data-act="save"]');assert.equal(h.writes.length,1);assert.deepEqual(h.saved.champions[1].ranked.spells,{spell1Id:4,spell2Id:14});}finally{h.close();}
});
