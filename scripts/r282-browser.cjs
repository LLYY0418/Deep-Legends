'use strict';
const {withBrowser}=require('./r280-browser-fixture.cjs');
const assert=require('node:assert/strict');
async function run(){await withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 for(const width of [2189,1100,960,870])for(const zoom of [0.9,1,1.14,1.5,1.75,2]){
  await p.call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});
  await p.evaluate(`localStorage.setItem('lol-loot-ui-scale','${zoom}');document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}';__r278App.applySidebar();document.querySelector('.topbar').dispatchEvent(new Event('topbar-layout'));`);
  await p.until("document.querySelector('.topbar').dataset.topbarSettled==='true'");
  const geometry=await p.evaluate(`(()=>{const bar=document.querySelector('.topbar'),r=bar.getBoundingClientRect(),visible=n=>n.getClientRects().length>0;return {width,zoom,level:bar.dataset.topbarCompact,scrollWidth:bar.scrollWidth,clientWidth:bar.clientWidth,bar:{left:r.left,right:r.right,height:r.height},nativeInset:parseFloat(getComputedStyle(bar).paddingRight)*Number(document.documentElement.dataset.uiScale),title:document.querySelector('#current-section-title').getBoundingClientRect().toJSON(),subtitle:getComputedStyle(document.querySelector('.window-drag>span')).display,friends:getComputedStyle(document.querySelector('.friends-toggle-label')).display,buttons:[...bar.querySelectorAll('button')].filter(visible).map(n=>{const b=n.getBoundingClientRect();return {id:n.id,left:b.left,right:b.right,height:b.height}})}})()` .replace('width,zoom,',`width:${width},zoom:${zoom},`));
  results.push({kind:'topbar',...geometry});await p.shot(`topbar-${width}-${zoom}-level-${geometry.level}`);
  assert(geometry.scrollWidth<=geometry.clientWidth+1,JSON.stringify(geometry));
  for(const b of geometry.buttons)assert(b.left>=geometry.bar.left-1 && b.right<=geometry.bar.right+1,JSON.stringify({width,zoom,b,bar:geometry.bar}));
  assert(Math.abs(geometry.bar.height-56*zoom)<=1,JSON.stringify(geometry));assert(geometry.title.width>0);assert(Math.abs(geometry.nativeInset-146)<=2,JSON.stringify(geometry));for(const b of geometry.buttons)assert(b.right<=geometry.bar.right-geometry.nativeInset+1,JSON.stringify({b,geometry}));
  if(width===2189 && zoom>=1.5){assert.equal(await p.evaluate("document.documentElement.dataset.sidebar"),'collapsed');const saved=await p.evaluate("localStorage.getItem('lol-loot-sidebar')");await p.evaluate('__r278App.toggleSidebar()');assert.equal(await p.evaluate("localStorage.getItem('lol-loot-sidebar')"),saved);assert.equal(await p.evaluate("document.documentElement.dataset.sidebar"),'expanded');}
  if(width===2189 && [1,1.14].includes(zoom)){assert.notEqual(geometry.subtitle,'none');assert.notEqual(geometry.friends,'none');}
 }
 await p.call('Emulation.setDeviceMetricsOverride',{width:2189,height:1100,deviceScaleFactor:1,mobile:false});
 for(const saved of ['expanded','collapsed']){await p.evaluate(`localStorage.setItem('lol-loot-sidebar','${saved}');document.documentElement.dataset.uiScale='1.75';document.documentElement.style.setProperty('--ui-zoom','1.75');__r278App.applySidebar()`);assert.equal(await p.evaluate("document.documentElement.dataset.sidebar"),'collapsed');await p.evaluate('__r278App.toggleSidebar()');assert.equal(await p.evaluate("localStorage.getItem('lol-loot-sidebar')"),saved);await p.evaluate("document.documentElement.dataset.uiScale='1.14';document.documentElement.style.setProperty('--ui-zoom','1.14');__r278App.applySidebar()");assert.equal(await p.evaluate("document.documentElement.dataset.sidebar"),saved);results.push({kind:'sidebar-preference-restored',saved});}
 assert.equal(errors.length,0,JSON.stringify(errors));
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
