'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {fixture} = require('./r190-harness.cjs');
const {read, extract, fixture: premadeFixture} = require('./r188-harness.cjs');
const source = process.env.R195_GAMEPLAY_SOURCE ? fs.readFileSync(process.env.R195_GAMEPLAY_SOURCE,'utf8') : read('gameplay.js');
const appSource = process.env.R195_APP_SOURCE ? fs.readFileSync(process.env.R195_APP_SOURCE,'utf8') : read('app.js');
const lethal = {id:8008,name:'致命节奏',shortDesc:'攻击一个敌方英雄会提供攻击速度',eogDescs:['最大攻速运转时间：@eogvar1@:@eogvar2@<br>已造成的伤害：@eogvar2@']};
function effects() {
 const f=fixture(source);f.perks.push(lethal);
 const subject={...f.subject,perkIds:[8008],perkStats:[{perkId:8008,vars:[932,972,0]}]};
 return {...f,subject};
}
test('R195 1 effect list removes shards and preserves shared board',()=>{
 const f=effects(),d=new JSDOM(f.renderRuneEffects({...f.subject,statModIds:[5005,5008,5001]})).window.document;
 assert.doesNotMatch(d.body.textContent,/属性碎片|攻击速度 \+10%|生命值 \+10/);
 assert.equal(d.querySelectorAll('.shards-line,.shard-chip').length,0);
 assert.doesNotMatch(read('gameplay.css'),/shards-line|shard-chip/);
 assert.match(f.renderUnifiedRuneBoard(f.subject),/data-perk-id="5005"/);
});
test('R195 2 lethal tempo keeps confirmed var2 damage and suppresses malformed time',()=>{
 const f=effects();
 assert.deepEqual(f.perkEffectLines(lethal,[932,972,0]),[{label:'已造成的伤害',value:972,kind:'damage'}]);
 assert.doesNotMatch(f.renderRuneEffects(f.subject),/最大攻速运转时间|932:972/);
 for(const vars of [[520,520,0],[775,775,0]]) assert.equal(f.perkEffectLines(lethal,vars)[0].value,vars[1]);
});
test('R195 3 magical footwear uses the client label and minute/tens/units template',()=>{
 const f=effects();
 assert.deepEqual(f.perkEffectLines({id:8304,eogDescs:['X：@eogvar1@:@eogvar2@@eogvar3@']},[7,3,0]),[{label:'X',value:'7:30',kind:'other'}]);
 assert.deepEqual(f.perkEffectLines({id:8304,eogDescs:['鞋子到达时间：@eogvar1@:@eogvar2@@eogvar3@']},[0,0,5]),[{label:'鞋子到达时间',value:'0:05',kind:'other'}]);
});
test('R195 4 invalid footwear digits and minutes do not render',()=>{
 const f=effects(),perk={id:8304,eogDescs:['X：@eogvar1@:@eogvar2@@eogvar3@']};
 for(const vars of [[7,12,0],[7,3,10],[-1,3,0],[7,-1,0],[7,3,-1],[7,3.1,0],[7,3,null],[1.5,3,0]]) assert.deepEqual(f.perkEffectLines(perk,vars),[]);
});
test('R195 5 keystone with values shows only tree subtitle; missing values retain description',()=>{
 const f=effects();
 const withValue=new JSDOM(f.renderRuneEffects(f.subject)).window.document;
 assert.equal(withValue.querySelector('.is-keystone .name small').textContent,'精密');
 assert.doesNotMatch(withValue.querySelector('.name').textContent,/攻击一个敌方英雄/);
 const missing=new JSDOM(f.renderRuneEffects({...f.subject,perkStats:undefined})).window.document;
 assert.match(missing.querySelector('.is-keystone .name small').textContent,/精密 · 攻击一个敌方英雄/);
 const css=read('gameplay.css');assert.match(css,/\.eff\.is-keystone \.game-icon\s*\{[^}]*width:\s*44px;\s*height:\s*44px/);
 assert.match(css,/\.eff\.is-keystone \.stat b\s*\{[^}]*var\(--primary-strong\);\s*font-size:\s*18px/);
});
test('R195 6 header damage includes the confirmed lethal tempo value',()=>{
 const f=effects();assert.deepEqual(f.runeEffectTotals(f.selectedRuneEffects(f.subject)),{damage:972,heal:0,gold:0});
 const d=new JSDOM(f.renderRuneYield(f.subject)).window.document;assert.equal(d.querySelector('.is-damage b').textContent,'972');
});
function rosterTooltip(players, mask=false) {
 const f=premadeFixture({gameplaySource:source});f.state.settings.maskNames=mask;
 const d=new JSDOM('<main></main>',{url:'http://localhost',pretendToBeVisual:true,runScripts:'outside-only'});
 d.window.matchMedia=()=>({matches:false});
 d.window.eval(extract(appSource,'setupFloatingTooltips')+'\nsetupFloatingTooltips();');
 d.window.document.querySelector('main').innerHTML=f.renderLivePremadeTag(players[0],players,103);
 const anchor=d.window.document.querySelector('[data-tooltip-roster]');
 anchor.dispatchEvent(new d.window.Event('pointerover',{bubbles:true}));
 return {d,anchor,tooltip:d.window.document.querySelector('#global-tooltip')};
}
function members(){return [103,69].map((championId,i)=>({gameName:`测试玩家${i+1}`,tagLine:'195',championId,championName:i?'卡西奥佩娅':'阿狸',premadeGroup:'1',premadeSize:2,premadeSource:'session'}));}
test('R195 7 picked premade heroes render two queued icons before names without hero text',()=>{
 const {d,anchor,tooltip}=rosterTooltip(members());try{
 assert.equal(tooltip.querySelector('.tooltip-title').textContent,'组队 2 人');
 const images=[...tooltip.querySelectorAll('img')];assert.equal(images.length,2);
 assert.ok(images.every(img=>img.dataset.queuedSrc.startsWith('/api/image?path=')&&!img.hasAttribute('src')));
 for(const row of tooltip.querySelectorAll('.tooltip-roster-player')){assert.equal(row.firstElementChild.tagName,'IMG');assert.equal(row.lastElementChild.className,'tooltip-roster-name');}
 assert.doesNotMatch(tooltip.textContent,/阿狸|卡西奥佩娅/);
 const rows=JSON.parse(anchor.dataset.tooltipRoster);assert.deepEqual(Object.keys(rows[0]).sort(),['championIconURL','name']);
 // The same tooltip renderer rejects remote or active-content image URLs.
 for(const url of ['https://example.org/hero.png','javascript:alert(1)','//example.org/hero.png']){
  rows[0].championIconURL=url;anchor.dataset.tooltipRoster=JSON.stringify(rows);
  anchor.dispatchEvent(new d.window.Event('focusin',{bubbles:true}));
  assert.equal(tooltip.querySelectorAll('img').length,1);assert.ok(tooltip.querySelector('span.tooltip-roster-icon'));
 }
 }finally{d.window.close();}
});
test('R195 8 no champion or pending pick retains empty icon slot and aligned name',()=>{
 for(const patch of [{championId:0,championPickIntent:69},{championPickPending:true}]){
  const players=members();Object.assign(players[1],patch);
  const {d,tooltip}=rosterTooltip(players);try{
   const row=tooltip.querySelectorAll('.tooltip-roster-player')[1];assert.equal(row.querySelector('img'),null);assert.ok(row.querySelector('span.tooltip-roster-icon'));assert.equal(row.lastElementChild.textContent,'测试玩家2#195');
  }finally{d.window.close();}
 }
});
test('R195 9 premade icon rows preserve name masking',()=>{
 const {d,tooltip}=rosterTooltip(members(),true);try{assert.deepEqual([...tooltip.querySelectorAll('.tooltip-roster-name')].map(n=>n.textContent),['玩家 01','玩家 02']);assert.doesNotMatch(tooltip.textContent,/测试玩家|#195/);}finally{d.window.close();}
});
