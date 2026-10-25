'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const read=n=>fs.readFileSync(path.join(__dirname,n),'utf8');
const {editor}=new Function('require','__dirname',read('r300.test.cjs').split("test('R300 portraits")[0]+';return {editor};')(require,__dirname);
test('R307 dirty champion mode and position switches retain drafts and markers without dialogs',async()=>{
 const h=await editor();try{const d=h.w.document;
 await h.click('[data-col="sub"][data-row="3"]');const selected=d.querySelector('[data-col="sub"][data-row="3"].is-picked').dataset.rune;
 assert(d.querySelector('[data-champ="1"] .ac-state-dot.is-unsaved'));assert.equal(d.querySelector('[data-mode="ranked"] .ac-state-dot'),null);assert(d.querySelector('[data-tab="runes"] .ac-state-dot.is-unsaved'));assert(d.querySelector('[data-pos=""] .ac-state-dot.is-unsaved'));
 for(const [away,back]of [['[data-champ="2"]','[data-champ="1"]'],['[data-mode="aram"]','[data-mode="ranked"]'],['[data-pos="top"]','[data-pos=""]']]){
 await h.click(away);assert.equal(d.querySelector('dialog[open]'),null);await h.click(back);assert.equal(d.querySelector('[data-save-state]').textContent,'未保存');assert.equal(d.querySelector(`[data-rune="${selected}"]`).getAttribute('aria-pressed'),'true');}
 assert.equal(d.querySelector('.ac-pos-label'),null);await h.click('[data-act="save"]');assert.equal(d.querySelectorAll('.ac-state-dot.is-unsaved').length,0);assert.equal(h.writes.length,1);
 await h.click('[data-col="sub"][data-row="1"]');assert(d.querySelector('.ac-state-dot.is-unsaved'));await h.click('[data-act="revert"]');assert.equal(d.querySelectorAll('.ac-state-dot.is-unsaved').length,0);
 }finally{h.close();}
});
test('R307 missing selections pulse only on a disabled save attempt and can pulse again',async()=>{
 const h=await editor();try{const d=h.w.document;await h.click('[data-act="clear"]');assert.equal(d.querySelector('.ac-validation-pulse'),null);await h.click('[data-act="start-runes"]');assert.match(d.querySelector('.ac-feedback').textContent,/符文还差 9/);assert.equal(d.querySelector('.ac-validation-pulse'),null);
 await h.click('[data-col="primary"][data-row="0"]');assert.equal(d.querySelector('.ac-validation-pulse'),null);assert(d.querySelector('[data-act="save"]').disabled);
 for(let i=0;i<2;i++){await h.click('[data-save-attempt]');const nodes=[...d.querySelectorAll('.ac-validation-pulse')];assert(nodes.length>0);for(const n of nodes)n.dispatchEvent(new h.w.Event('animationend'));assert.equal(d.querySelector('.ac-validation-pulse'),null);}assert.equal(h.writes.length,0);
 }finally{h.close();}
});
test('R307 custom selection filters every enabled-category combination and falls back when disabled',()=>{
 const {compile}=require('./r188-harness.cjs'),entry={runes:{selectedPerkIds:[1,2,3,4,5,6,7,8,9]},spells:{spell1Id:4,spell2Id:14},items:{starter:[{id:2003,count:2}],core:[3000]}},state={autoConfigSettings:{enabled:true,champions:{1:{ranked:entry}}}},data={gameId:1,queueId:420,players:[{isCurrent:true,championId:1}]};
 const f=compile(read('auto-config-live.js'),['mode','resolveModeKey','selection','prepare','renderCustomConfiguration'],{state,labels:{ranked:'排位'},positions:{},escapeHTML:String,renderUnifiedRuneBoard:()=>'<board/>',renderRuneSpellIcons:()=>'<spell/>',renderItemIcon:id=>'<item>'+id+'</item>',runeConfigurationTitle:()=>''});
 for(let mask=0;mask<8;mask++){
 Object.assign(state.autoConfigSettings,{runes:!!(mask&1),spells:!!(mask&2),items:!!(mask&4)});const c=f.prepare(data);assert.equal(!!c,mask!==0);if(!c)continue;assert.equal(!!c.entry.runes,!!(mask&1));assert.equal(!!c.entry.spells,!!(mask&2));assert.equal(!!c.entry.items,!!(mask&4));const html=f.renderCustomConfiguration(c);assert.equal(html.includes('<board'),!!(mask&1));if(!(mask&1))assert.equal(html.includes('<spell'),!!(mask&2));assert.equal(html.includes('最终装备'),!!(mask&4));}
 state.selectedRecommendation='custom';state.autoConfigSettings.enabled=false;assert.equal(f.prepare(data),null);assert.equal(state.selectedRecommendation,'opgg');assert.equal(state.runeSourceTab,'opgg');assert.equal(f.renderCustomConfiguration({entry}),'');
 state.autoConfigSettings={enabled:true,runes:true,spells:false,items:false,champions:{1:{ranked:{spells:entry.spells}}}};assert.equal(f.prepare(data),null);
});
test('R307 settings events remove custom immediately without a live payload',()=>{
 const {JSDOM}=require('../../desktop/node_modules/jsdom'),dom=new JSDOM('',{url:'http://fixture',runScripts:'outside-only'}),w=dom.window,state={section:'live',selectedRecommendation:'custom',runeSourceTab:'custom'};let renders=0;
 try{w.deepLegendsGameAutoConfig={state,renderLive:()=>renders++};w.deepLegendsSections={register(){}};w.eval(read('auto-config-live.js'));w.dispatchEvent(new w.CustomEvent('deep-legends:auto-config-settings',{detail:{enabled:false,champions:{}}}));assert.equal(renders,1);assert.equal(state.selectedRecommendation,'opgg');assert.equal(state.runeSourceTab,'opgg');}finally{w.close();}
});
