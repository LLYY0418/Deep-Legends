'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs');
const read=n=>fs.readFileSync(path.join(__dirname,n),'utf8'),source=read('patch-notes.js');
const feed=JSON.parse(read('../patchnotes/patch-notes.json'));
test('R304 seven real bundled changes use the correct color and preserve official strings',()=>{
 const dom=new JSDOM('<main/>',{url:'http://fixture',runScripts:'outside-only'}),w=dom.window;
 try{w.deepLegendsRuntime={escapeHTML:String};w.deepLegendsSections={register(){}};w.fetch=()=>new Promise(()=>{});w.eval(source);
  for(const [version,mode,name,key,want]of [
   ['26.21','ranked','暴食胫甲','总费用','nerf'],['26.21','arena','幻境之刃','剩余冷却返还','nerf'],['26.21','arena','月华咒刃','每次攻击的冷却返还','nerf'],
   ['26.20','aramMayhem','斯维因','承受伤害','buff'],['26.17','arena','盖伦','非战斗状态计时器','buff'],['26.17','arena','塔姆','非战斗状态计时器','buff'],['26.21','arena','超强大脑','脱离战斗计时器','nerf']]){
   const sections=feed.versions.find(v=>v.version===version).modes[mode].sections,entry=sections.flatMap(s=>s.entries||[]).find(e=>e.name===name),line=entry?.groups.flatMap(g=>g.lines||[]).find(l=>l.k===key);assert(line,`${name}/${key}`);
   w.document.querySelector('main').innerHTML=w.deepLegendsPatchNotes.renderSections(sections);
   const article=[...w.document.querySelectorAll('.pn-entry')].find(n=>n.querySelector('.pn-entry-name').textContent===name),node=[...article.querySelectorAll('.pn-line')].find(n=>n.querySelector('.k')?.textContent===key+'：');
   assert(node.querySelector('.to').classList.contains('is-'+want),`${version}/${name}/${key}`);assert.equal(node.querySelector('.from').textContent,line.from);assert.equal(node.querySelector('.to').textContent,line.to);
   if(name==='月华咒刃'){assert.equal(article.querySelector('.pn-direction').textContent,'↓');assert(![...article.querySelectorAll('.pn-line')].find(n=>n.querySelector('.k')?.textContent==='强化攻击次数：').querySelector('.to').classList.contains('is-buff'));}
  }
 }finally{w.close();}
});
test('R304 cooldown return and reduction are higher-better; cooldown time and received damage are lower-better',()=>{
 const f=compile(source,['lineDirection'],{});
 for(const key of ['冷却返还','冷却缩减','每次攻击的冷却返还','基础技能 冷却时间返还'])assert.equal(f.lineDirection({k:key,from:'10',to:'20'}),'buff',key);
 for(const key of ['冷却时间','费用','承受伤害','受到的伤害','受到伤害','战斗状态计时器','脱离战斗计时器','脱战时间','种子充能'])assert.equal(f.lineDirection({k:key,from:'10',to:'20'}),'nerf',key);
 for(const key of ['强化攻击次数','减速','减速阈值','攻击速度收益'])assert.equal(f.lineDirection({k:key,from:'10',to:'20'}),'',key);
 assert.equal(f.lineDirection({k:'减速抗性',from:'10',to:'20'}),'buff');
});
test('R304 indeterminate changes allow known directions while mixed numerical sequences adjust',()=>{
 const f=compile(source,['lineDirection','entryDirection'],{}),entry=lines=>({groups:[{lines}]});
 assert.equal(f.entryDirection(entry([{k:'伤害',from:'1',to:'2'},{k:'强化攻击次数',from:'2',to:'3'}])), 'buff');
 assert.equal(f.entryDirection(entry([{k:'伤害',from:'1',to:'2'},{k:'生命值',from:'1/2',to:'2/1'}])), 'adjust');
 assert.equal(f.entryDirection(entry([{k:'伤害',from:'1',to:'2'},{text:'新增效果'}])), 'buff');
 assert.equal(f.entryDirection(entry([{k:'伤害',from:'1',to:'2'},{k:'护甲',from:'10',to:'10'}])), 'buff');
 const azir=feed.versions[0].modes.ranked.sections.flatMap(s=>s.entries||[]).find(e=>e.championId===268);assert.equal(f.entryDirection(azir),'buff');
});
test('R304 spell layout coalesces a frame and ignores unrelated live mutations and unchanged widths',()=>{
 const dom=new JSDOM('<main><article class="specialist-game-row"><header class="specialist-game-head"><strong>符文标题</strong><small role="status">状态</small></header><div class="rune-corner-spells"><span data-card-spells><img><img></span></div></article><div id="clock"></div></main>',{runScripts:'outside-only'}),w=dom.window,d=w.document;
 let mutation,resize,styles=0,next=0,disconnected=0,width=400;const frames=new Map(),targets=new Set();
 w.requestAnimationFrame=f=>{frames.set(++next,f);return next;};w.cancelAnimationFrame=id=>frames.delete(id);
 w.MutationObserver=class{constructor(f){mutation=f;}observe(){}disconnect(){disconnected++;}};
 w.ResizeObserver=class{constructor(f){resize=f;}observe(n){targets.add(n);}unobserve(n){targets.delete(n);}disconnect(){disconnected++;targets.clear();}};
 w.getComputedStyle=()=>{styles++;return {columnGap:'4px',paddingLeft:'0',paddingRight:'0'};};
 const row=d.querySelector('article'),head=d.querySelector('header'),pair=d.querySelector('[data-card-spells]');
 for(const n of [row,head])Object.defineProperty(n,'clientWidth',{get:()=>width});
 for(const n of [...head.children,pair])Object.defineProperty(n,'scrollWidth',{value:n===pair?80:100,configurable:true});
 w.deepLegendsGameAutoConfig={state:{},nodes:{liveContent:d.querySelector('main')}};w.deepLegendsSections={register(){}};
 const flush=()=>{const pending=[...frames.values()];frames.clear();pending.forEach(f=>f());};
 try{w.eval(read('auto-config-live.js'));assert.equal(frames.size,1);flush();assert.equal(styles,1);assert.equal(row.dataset.spellLayout,'head');assert.equal(pair.parentElement,head);
  resize([...targets].map(target=>({target,contentRect:{width:target===row?400:100}})));assert.equal(frames.size,0);
  for(let i=0;i<5;i++)mutation([{target:d.querySelector('#clock'),addedNodes:[d.createElement('span')],removedNodes:[]}]);resize([{target:row,contentRect:{width:400}}]);assert.equal(frames.size,0);assert.equal(styles,1);
  for(let i=0;i<5;i++)mutation([{target:pair,addedNodes:[d.createElement('img')],removedNodes:[]}]);assert.equal(frames.size,1);flush();assert.equal(styles,2);
  width=200;resize([{target:row,contentRect:{width:200}}]);resize([{target:row,contentRect:{width:200}}]);assert.equal(frames.size,1);flush();assert.equal(styles,3);assert.equal(row.dataset.spellLayout,'board');
  const replacement=pair.cloneNode(true);Object.defineProperty(replacement,'scrollWidth',{value:80});pair.replaceWith(replacement);mutation([{target:d.querySelector('.rune-corner-spells'),addedNodes:[replacement],removedNodes:[pair]}]);assert.equal(frames.size,1);flush();assert.equal(styles,4);assert.equal(row.querySelectorAll('[data-card-spells]').length,1);
  width=400;resize([{target:row,contentRect:{width:400}}]);flush();assert.equal(styles,5);assert.equal(row.dataset.spellLayout,'head');
  const title=head.querySelector('strong');mutation([{target:title,addedNodes:[d.createTextNode('same width')],removedNodes:[]}]);flush();assert.equal(styles,5,'same-width title/status changes must not recompute layout');
  Object.defineProperty(title,'scrollWidth',{value:500,configurable:true});mutation([{target:title,addedNodes:[d.createTextNode('longer title')],removedNodes:[]}]);assert.equal(frames.size,1);flush();assert.equal(styles,6);assert.equal(row.dataset.spellLayout,'board','intrinsic title width changed without resizing the card');
  row.remove();mutation([{target:d.querySelector('main'),addedNodes:[],removedNodes:[row]}]);assert.equal(targets.size,0);assert.equal(frames.size,0);
  w.dispatchEvent(new w.Event('deep-legends:dispose'));assert.equal(disconnected,2);assert.equal(frames.size,0);
 }finally{w.close();}
});
