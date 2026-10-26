'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {compile,read,escapeHTML}=require('./r188-harness.cjs');
const source=read('champions.js');
test('R312 atlas description removes current-game counters and escapes highlighted values',()=>{
 const state={mayhemAugmentDetail:{description:'你的攻击造成额外 (75%法术强度) 物理伤害。\n已造成的伤害：当前对局数值\n冷却 6 秒。<img src=x onerror=bad()>'}},f=compile(source,['mayhemAtlasDescription','renderMayhemAtlasDescription'],{state,escapeHTML,AUGMENT_DESCRIPTION_PENDING:'等待'});
 const text=f.mayhemAtlasDescription({});assert.doesNotMatch(text,/当前对局数值|已造成的伤害/);assert.match(text,/冷却 6 秒/);const html=f.renderMayhemAtlasDescription({});assert.match(html,/<b class="mayhem-description-value">\(75%法术强度\)<\/b>/);assert.match(html,/<b class="mayhem-description-value">6<\/b>/);assert.doesNotMatch(html,/<img/);assert.match(html,/&lt;img/);
 state.mayhemAugmentDetail=null;assert.doesNotMatch(f.mayhemAtlasDescription({description:'效果 40 点\n已治疗：当前对局数值'}),/当前对局数值/);
});
test('R312 mayhem stage chips select summary by default and follow stage two',()=>{
 const state={mode:'aram-mayhem',mayhemStage:0},f=compile(source,['renderMayhemStageChips','mayhemAugmentStageRow'],{state,objectRows:v=>Array.isArray(v)?v:[],MAYHEM_STAGE_OPTIONS:[[0,'汇总'],[1,'阶段 1'],[2,'阶段 2']],normalizeMayhemStage:Number});const detail=[{stages:[{stage:1,games:300},{stage:2,games:300}]}];
 let html=f.renderMayhemStageChips(detail);assert.match(html,/class="is-active" aria-pressed="true" data-mayhem-stage="0"/);state.mayhemStage=2;html=f.renderMayhemStageChips(detail);assert.match(html,/class="is-active" aria-pressed="true" data-mayhem-stage="2"/);
 assert.match(read('champions.css'),/\.mayhem-stage-chips button\.is-active[^}]+var\(--primary/s);
});
test('R312 equipment fixes the original scope and keeps linked badges, sorting and pagination',()=>{
 const build=source.slice(source.indexOf('function renderARAMKitBuild('),source.indexOf('function filteredAugments('));assert.match(build,/scope=scopes\.unfiltered/);assert.doesNotMatch(source,/itemScope|aramkit-scope/);assert.match(build,/renderARAMKitItems\(rows,detail.stats\?\.winRate,filtered\)/);
 const state={itemSort:'pickRate',itemLimit:10},f=compile(source,['renderARAMKitItems'],{state,objectRows:v=>v||[],assetImage:()=>'',escapeHTML,percent:String,number:String});const html=f.renderARAMKitItems([{assets:[{id:1,name:'装备'}],winRate:55,pickRate:10}],50,[]);assert.match(html,/aramkit-linked">联动/);
});
test('R312 search buttons and atlas geometry use the required cursor, opacity and sizing contracts',()=>{
 const css=read('history-filters.css'),atlas=read('champions.css');assert.match(css,/\.af-conditionbar button\s*\{\s*cursor:pointer/);assert.match(css,/\.af-conditionbar button:disabled\s*\{\s*cursor:default/);
 assert.match(atlas,/\.mayhem-fit-columns\s*\{[^}]*z-index:5[^}]*background:var\(--surface\)/);assert.match(atlas,/width:fit-content; max-width:min\(520px,58%\)/);assert.match(atlas,/padding:8px 12px; background:var\(--surface-strong\)/);assert.match(atlas,/font-size:12.5px; line-height:1.6/);assert.match(atlas,/flex-basis:100%; width:100%; max-width:none/);
});
