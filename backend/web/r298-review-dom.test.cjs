'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require(require.resolve('jsdom',{paths:[path.join(__dirname,'../../desktop')]}));
const fixture=require('../testdata/r298-review-catalog.json');
const clone=value=>JSON.parse(JSON.stringify(value));
const tick=()=>new Promise(setImmediate);
async function editor(keys=['N','M'],eventGameMode='') {
 const dom=new JSDOM('<div id="suite-config-root"></div>',{url:'http://127.0.0.1',runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window;
 let saved={version:1,enabled:false,runes:true,spells:true,items:true,champions:{'1':Object.fromEntries(Object.keys(fixture.expectedSpellPools).map(mode=>[mode,{}]))}};
 const writes=[];
 w.deepLegendsRuntime={escapeHTML:v=>String(v??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;')};w.deepLegendsSections={register(){}};
 w.fetch=async(url,options={})=>{
  let value={};
  if(url==='/api/auto-config/settings')value=saved;
  else if(url==='/api/auto-config/catalog')value={...fixture.catalog,eventGameMode};
  else if(url==='/api/champions/catalog')value={champions:[{id:1,key:'Annie',nameZh:'安妮',titleZh:'黑暗之女'}]};
  else if(url==='/api/settings/summoner-keys')value={keys,flashSlot:1};
  else if(url.startsWith('/api/auto-config/champions/1/')){const mode=url.split('/').at(-1),body=JSON.parse(options.body);writes.push({mode,body});saved.champions['1'][mode]=body;saved.version++;value=saved;}
  return {ok:true,json:async()=>clone(value),text:async()=>''};
 };
 w.eval(fs.readFileSync(path.join(__dirname,'auto-config.js'),'utf8'));await w.deepLegendsAutoConfig.load();
 const click=async selector=>{const node=w.document.querySelector(selector);assert(node,selector);node.click();await tick();};
 return {w,dom,writes,click,get saved(){return clone(saved)},close:()=>w.close()};
}
for(const keys of [['D','F'],['N','M']])test(`R298 real DOM spell picks save and reload in ${keys.join('/')}`,async()=>{
 const h=await editor(keys);
 try {
  await h.click('[data-tab="kit"]');
  await h.click('[data-spell="4"]');
  assert.equal(h.w.document.querySelector('.ac-slot.is-active').dataset.slot,'1','first pick advances to the right slot');
  await h.click('[data-spell="14"]');
  assert.equal(h.w.document.querySelector('.ac-slot.is-active').dataset.slot,'0','second pick advances to the left slot');
  await h.click('[data-act="save"]');
  assert.deepEqual(h.saved.champions['1'].ranked.spells,{spell1Id:4,spell2Id:14},'flash preference must not reorder editor configuration');
  await h.click('.ac-slot[data-slot="1"]');await h.click('[data-spell="6"]');
  await h.click('[data-act="swap"]');await h.click('[data-act="save"]');
  assert.deepEqual(h.saved.champions['1'].ranked.spells,{spell1Id:6,spell2Id:4});
  // Pick an already-used candidate, then replace the selected slot again.
  await h.click('.ac-slot[data-slot="0"]');await h.click('[data-spell="4"]');
  await h.click('[data-spell="14"]');await h.click('[data-act="save"]');
  assert.deepEqual(h.saved.champions['1'].ranked.spells,{spell1Id:4,spell2Id:14});
  await h.w.deepLegendsAutoConfig.load(true);
  assert.deepEqual([...h.w.document.querySelectorAll('.ac-slot kbd')].map(n=>n.textContent),keys);
  assert(h.w.document.querySelector('.ac-slot.is-active'),'slot index must remain 0 or 1');
  assert(!h.w.document.querySelector('#suite-config-root').textContent.includes('NaN'));
  const icons=[...h.w.document.querySelectorAll('.ac-slot img')].map(n=>new URL(n.src).searchParams.get('path'));
  assert.deepEqual(icons,['/fixture-icon/4','/fixture-icon/14']);
  assert(h.writes.every(({body})=>Number.isFinite(body.spells.spell1Id)&&Number.isFinite(body.spells.spell2Id)));
 } finally {h.close()}
});
test('R298 active ULTBOOK session keeps all six editor spell pools isolated',async t=>{
 for(const [mode,expected] of Object.entries(fixture.expectedSpellPools))await t.test(mode,async()=>{
  const h=await editor(['N','M'],'ULTBOOK');try{await h.click(`[data-mode="${mode}"]`);await h.click('[data-tab="kit"]');assert.deepEqual([...h.w.document.querySelectorAll('[data-spell]')].map(n=>Number(n.dataset.spell)),expected);}finally{h.close()}
 });
});
