'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile}=require('./r188-harness.cjs');
const root=__dirname;
const css=name=>fs.readFileSync(path.join(root,name.endsWith('.css')?name:name+'.css'),'utf8');
const gameplay=read('gameplay.js');
function selectors(text){const out=[];for(const m of text.replace(/\/\*[\s\S]*?\*\//g,'').matchAll(/([^{}]+)\{/g)){for(const s of m[1].split(','))out.push(s.trim())}return out}
function defines(files,needle){return files.some(f=>selectors(css(f)).some(s=>s===needle||s.startsWith(needle+' ')||s.includes(needle)))}
const initial=[...fs.readFileSync(path.join(root,'index.html'),'utf8').matchAll(/<link rel="stylesheet" href="\/([^"]+)"/g)].map(m=>m[1]);
const pageStyles={
  overview:[...initial,'match-detail'],
  details:[...initial,'match-detail'],
  champions:[...initial,'patch-notes','shared-cards','champions'],
  suite:[...initial,'patch-notes','shared-cards','champions','auto-config','suite'],
  live:[...initial,'shared-cards','match-detail','live-arena','auto-config','player-tags'],
  settings:[...initial,'patch-notes','champions','settings-preferences'],
};
test('R319 overview match detail styles are on demand and loaded before HTML',()=>{
  assert.ok(initial.includes('gameplay.css')&&!initial.includes('match-detail.css')&&!initial.includes('live-arena.css'));
  for(const selector of ['.arena-detail-columns','.arena-detail-team','.arena-detail-player','.arena-detail-kda','.arena-detail-damage','.arena-detail-augments','.arena-detail-items','.match-table .match-score-cell','.build-core-ranking-row','.item-build-layout','.late-option-band']){
    assert.ok(defines(pageStyles.details,selector),`details loads ${selector}`);
    assert.ok(!defines(['live-arena.css'],selector)||selector==='.build-core-ranking-row',`${selector} not only in live-arena`);
  }
  assert.match(fs.readFileSync(path.join(root,'section-loader.js'),'utf8'),/"match-details": \["match-detail"\]/);
  assert.match(gameplay,/function ensureMatchDetailRenderer\(\)[\s\S]{0,240}load\('match-details'\)/);
  const loader=fs.readFileSync(path.join(root,'section-loader.js'),'utf8');
  assert.match(loader,/"live-arena": \["shared-cards", "match-detail", "live-arena"\]/);
});
test('R319 every moved selector is reachable from each page that renders it',()=>{
  const rules=[
    ['overview',['.arena-detail-columns','.arena-detail-player','.arena-detail-augments','.arena-detail-items']],
    ['details',['.arena-detail-columns','.late-option-band','.build-core-ranking-row','.match-table .match-score-cell']],
    ['live',['.build-summary-bar.is-arena-no-skill','.live-augment-option','.live-aramkit-combos','.insight-match','.live-rank-label']],
    ['champions',['.arena-option-card','.mayhem-ranking-list','.position-icon','.recommendation-section']],
    ['suite',['.arena-option-card','.mayhem-ranking-list']],
  ];
  for(const [page,list] of rules)for(const selector of list)assert.ok(defines(pageStyles[page],selector),`${page} must load ${selector}`);
  assert.ok(!defines(pageStyles.overview,'.live-augment-option'),'overview must not require live-only styles');
});
test('R319 each moved rule exists exactly once',()=>{
  const files=['gameplay.css','champions.css','shared-cards.css','match-detail.css','live-arena.css'];
  const exact=(text,selector)=>new RegExp('(?:^|\\n)'+selector.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')+' \\{').test(text);
  for(const selector of ['.arena-detail-columns','.build-core-ranking-row','.arena-option-card','.late-option-band','.recommendation-section','.live-augment-recommendations']){
    const owners=files.filter(f=>exact(css(f),selector));
    assert.equal(owners.length,1,`${selector} defined in ${owners.join(',')}`);
  }
});
test('R319 match detail grid has five tracks and a single 玩家 / 装备 header',()=>{
  const block=css('match-detail.css').match(/\.arena-detail-columns \{([^}]*)\}/)[1];
  const value=block.match(/grid-template-columns:([^;]+)/)[1];
  const tracks=[];let depth=0,current='';
  for(const c of value){if(c==='(')depth++;else if(c===')')depth--;if(/\s/.test(c)&&depth===0){if(current){tracks.push(current);current=''}continue}current+=c}
  if(current)tracks.push(current);
  assert.equal(tracks.length,5,tracks.join('|'));
  assert.equal((tracks[0].match(/minmax/g)||[]).length,1);
  assert.match(css('match-detail.css'),/@container matches-column \(max-width:720px\)[\s\S]*grid-template-columns:minmax\(176px,1fr\)/);
  const source=fs.readFileSync(path.join(root,'match-details.js'),'utf8');
  assert.match(source,/class="arena-column-player">玩家 \/ 装备</);
  assert.doesNotMatch(source,/class="arena-column-items">装备</);
});
test('R319 cold overview computed style resolves the arena detail grid',()=>{
  const markup=`<section class="arena-match-detail" data-augment-count="6"><div class="arena-detail-columns"><span>玩家 / 装备</span><span>海克斯</span><span>评分</span><span>KDA</span><span>伤害 / 承伤</span></div></section>`;
  const styles=[...pageStyles.overview].map(f=>css(f)).join('\n');
  const dom=new JSDOM(`<style>${styles}</style>${markup}`,{pretendToBeVisual:true});
  try{
    const el=dom.window.document.querySelector('.arena-detail-columns');
    assert.equal(dom.window.getComputedStyle(el).display,'grid');
    const template=dom.window.getComputedStyle(el).gridTemplateColumns;
    assert.ok(template.split(/\s+/).length>=5||template.split(',').length===5,template);
    assert.equal(dom.window.document.querySelectorAll('.arena-detail-columns > *').length,5);
  }finally{dom.window.close()}
});
test('R319 augment icons keep perks fallback and resolve catalog-missing IDs',()=>{
  const start=gameplay.indexOf('function augmentMetadata(id)'),end=gameplay.indexOf('function refreshAugmentIconNodes',start);
  const slice=gameplay.slice(gameplay.indexOf('function augmentMetadata(id)'),gameplay.indexOf('function ensureAugmentResolved',start));
  const state={perks:{augments:[{id:3055,name:'终极循环往复',rarity:'gold',iconPath:'builtin:/augments/3055.png'}]},augmentCatalog:new Map(),augmentCatalogLoaded:true,augmentDescriptions:new Map()};
  let resolved=[];
  const f=Function('state','proxyAsset','escapeHTML','wrapAugmentIcon','ensureAugmentResolved',`${slice};return {augmentMetadata,augmentIconFigure}`)(state,p=>p,String,(icon)=>icon,(id)=>resolved.push(id));
  const meta=f.augmentMetadata(3055);
  assert.equal(meta.name,'终极循环往复');
  assert.deepEqual(resolved,[3055],'perks-known but catalog-missing IDs must be resolved');
  const html=f.augmentIconFigure(3055,'small');
  assert.match(html,/data-augment-candidates/);
  assert.match(html,/builtin:\/augments\/3055\.png/);
  assert.doesNotMatch(html,/has-image-error/);
  state.augmentCatalog=new Map([[999,{id:999,unknown:true}]]);
  const missing=f.augmentIconFigure(999,'small');
  assert.match(missing,/has-image-error/);
  assert.match(missing,/augment-placeholder/);
  assert.doesNotMatch(missing,/海克斯 999/);
});
test('R319 augment catalog failure backs off instead of giving up',async()=>{
  const start=gameplay.indexOf('async function ensureAugments()'),end=gameplay.indexOf('async function ensurePerks',start);
  const code=gameplay.slice(start,end);
  const state={destroyed:false,augmentCatalog:null,augmentCatalogLoading:false};
  const delays=[],jobs=[];
  const fakeSetTimeout=(fn,ms)=>{delays.push(ms);jobs.push(fn);return jobs.length},fakeClear=()=>{};
  const f=Function('state','api','scheduleCatalogViews','refreshAugmentIconNodes','document','setTimeout','clearTimeout',`${code};return ensureAugments`)(state,async()=>{throw new Error('503')},()=>{},()=>{},{querySelectorAll:()=>[]},fakeSetTimeout,fakeClear);
  await f();
  for(let i=0;i<4;i++)await jobs.at(-1)();
  assert.deepEqual(delays,[3000,10000,30000,300000,300000]);
  assert.equal(state.augmentCatalogLoading,false);
});
