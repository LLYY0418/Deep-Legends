'use strict';
const {browser}=require('./r297-browser-harness.cjs'),fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const fixture=require('../backend/testdata/r298-review-catalog.json');
const out=process.env.R298_REVIEW_REPORT_DIR||'/tmp/r298-review-browser';
const initial={version:1,enabled:true,runes:true,spells:true,items:true,champions:{'1':Object.fromEntries(Object.keys(fixture.expectedSpellPools).map(mode=>[mode,{}]))}};
const setup=`window.deepLegendsRuntime={escapeHTML:v=>String(v??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;')};window.deepLegendsSections={register(){}};window.catalog=${JSON.stringify(fixture.catalog)};window.saved=${JSON.stringify(initial)};window.writes=[];window.keys=new URLSearchParams(location.search).get('keys')==='df'?['D','F']:['N','M'];window.fetch=async(url,options={})=>{let value={};if(url==='/api/auto-config/settings')value=window.saved;else if(url==='/api/auto-config/catalog')value=window.catalog;else if(url==='/api/champions/catalog')value={champions:[{id:1,key:'Annie',nameZh:'安妮',titleZh:'黑暗之女',imagePath:'/fixture-icon/1'}]};else if(url==='/api/settings/summoner-keys')value={keys:window.keys,flashSlot:1};else if(url.startsWith('/api/auto-config/champions/1/')){const mode=url.split('/').at(-1),body=JSON.parse(options.body);window.writes.push({mode,body});window.saved.champions['1'][mode]={...body,updatedAt:new Date().toISOString()};window.saved.version++;value=window.saved;}return {ok:true,json:async()=>JSON.parse(JSON.stringify(value)),text:async()=>''}};`;
(async()=>{
 const b=await browser({out,transform:(name,data)=>name==='index.html'?Buffer.from(`<!doctype html><html data-theme="dark"><head><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/suite.css"><link rel="stylesheet" href="/auto-config.css"></head><body><main class="suite-panel" style="padding:20px"><div id="suite-config-root"></div></main><script>${setup}</script><script src="/auto-config.js"></script><script>deepLegendsAutoConfig.load()</script></body></html>`):data});
 const results={keyBindings:[],spellPools:{},errors:b.errors};
 const click=selector=>b.evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
 try {
  for(const name of ['df','nm']){
   await b.call('Page.navigate',{url:b.origin+'/?keys='+name});await b.until(`document.querySelector('[data-tab="kit"]')`);await click('[data-tab="kit"]');await b.evaluate(`document.querySelector('[data-act="start-spells"]')?.click()`);
   await click('[data-spell="4"]');assert.equal(await b.evaluate(`document.querySelector('.ac-slot.is-active').dataset.slot`),'0');
   await click('[data-slot="1"]');await click('[data-spell="14"]');assert.equal(await b.evaluate(`document.querySelector('.ac-slot.is-active').dataset.slot`),'1');
   await click('[data-act="save"][data-section="spells"]');await b.until('window.writes.length===1');
   assert.deepEqual(await b.evaluate(`window.saved.champions['1'].ranked.spells`),{spell1Id:4,spell2Id:14});
   await click('.ac-slot[data-slot="1"]');await click('[data-spell="6"]');await click('[data-act="swap"]');await click('[data-act="save"][data-section="spells"]');await b.until('window.writes.length===2');
   assert.deepEqual(await b.evaluate(`window.saved.champions['1'].ranked.spells`),{spell1Id:6,spell2Id:4});
   await click('.ac-slot[data-slot="0"]');await click('[data-spell="4"]');await click('[data-slot="1"]');await click('[data-spell="14"]');await click('[data-act="save"][data-section="spells"]');await b.until('window.writes.length===3');
   await b.evaluate('deepLegendsAutoConfig.load(true)');
   const result=await b.evaluate(`({keys:[...document.querySelectorAll('.ac-slot kbd')].map(n=>n.textContent),pair:window.saved.champions['1'].ranked.spells,active:document.querySelector('.ac-slot.is-active')?.dataset.slot,badges:[...document.querySelectorAll('.ac-spell.is-used')].map(n=>n.dataset.spellKey),invalidIndex:document.querySelector('#suite-config-root').textContent.includes('NaN'),writes:window.writes})`);
   assert.deepEqual(result.keys,name==='df'?['D','F']:['N','M']);assert.deepEqual(result.pair,{spell1Id:4,spell2Id:14});assert(['0','1'].includes(result.active));assert(!result.invalidIndex);assert.deepEqual(result.badges,result.keys);
   results.keyBindings.push(result);await b.shot('spells-'+name);
  }
  for(const [mode,expected] of Object.entries(fixture.expectedSpellPools)){
   if(mode==='hextech'){assert.equal(await b.evaluate(`document.querySelector('[data-mode="hextech"]')`),null);assert.deepEqual(fixture.catalog.spells.filter(s=>require('../backend/web/auto-config.js').spellAllowed(s,mode,'ULTBOOK')).map(s=>s.id),expected);results.removedEditorModes=['hextech'];}else{await click(`[data-mode="${mode}"]`);await click('[data-tab="kit"]');await b.evaluate(`document.querySelector('[data-act="start-spells"]')?.click()`);const actual=await b.evaluate(`[...document.querySelectorAll('[data-spell]')].map(n=>Number(n.dataset.spell))`);assert.deepEqual(actual,expected,mode);results.spellPools[mode]=actual;}
  }
  await b.call('Emulation.setDeviceMetricsOverride',{width:960,height:1100,deviceScaleFactor:1,mobile:false});assert(!await b.evaluate(`document.documentElement.scrollWidth>innerWidth`));await b.shot('narrow');
  assert.deepEqual(b.errors,[]);fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(results,null,2));console.log(JSON.stringify(results));
 }catch(error){await b.shot('failure');throw error;}finally{await b.stop()}
})().catch(e=>{console.error(e);process.exitCode=1});
