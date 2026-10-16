'use strict';
const {withBrowser}=require('./r280-browser-fixture.cjs');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
// Called in Chromium against visible production controls, including lazy modules.
function audit(){
 const visible=n=>{const r=n.getBoundingClientRect(),s=getComputedStyle(n);return r.width>0 && r.height>0 && s.visibility!=='hidden' && s.display!=='none' && !n.closest('[hidden]')};
 const icons=[...document.querySelectorAll('svg.function-icon')].filter(visible),issues=[],groups=new Map();
 const module=n=>n.closest('.topbar')?'topbar':n.closest('.sidebar')?'sidebar':n.closest('dialog[open],.af-dialog')?'dialog':n.closest('.career-column')?'career':n.closest('.match-entry')?'match':n.closest('#champions-panel')?'champions':n.closest('#live-panel')?'live':n.closest('#suite-panel')?'tools':n.closest('#settings-panel')?'settings':n.closest('#favorites-workspace,#collection-panel,#pool-panel')?'collection':'page';
 const rows=icons.map(n=>{const style=getComputedStyle(n),r=n.getBoundingClientRect(),use=n.querySelector('use'),u=use && getComputedStyle(use),rawSize=parseFloat(style.width),size=[14,18,22,40].find(v=>Math.abs(v-rawSize)<.05) || rawSize,key=module(n)+':'+size;
  if(n.getAttribute('viewBox')!=='0 0 24 24')issues.push('viewBox '+n.outerHTML);
  if(![14,18,22,40].includes(size))issues.push('size '+size);
  if(style.strokeWidth!=='1.75px' || u?.vectorEffect!=='non-scaling-stroke')issues.push('stroke '+style.strokeWidth+' '+u?.vectorEffect);
  if(!use?.getAttribute('href')?.startsWith('/icons.svg#dl-icon-'))issues.push('source '+n.outerHTML);
  const reference=use?.getAttribute('href')?.split('#')[1],drawing=n.getBBox();
  if(!window.__r280SpriteIDs?.has(reference))issues.push('reference '+reference);
  if(!(drawing.width>0 && drawing.height>0))issues.push('empty drawing '+reference);
  const prior=groups.get(key);if(prior && (Math.abs(prior.width-r.width)>.5 || Math.abs(prior.height-r.height)>.5))issues.push('unequal '+key+' '+n.outerHTML.slice(0,180));groups.set(key,r);
  return {module:module(n),size,width:r.width,height:r.height,stroke:style.strokeWidth,vector:u?.vectorEffect,reference,drawing:{width:drawing.width,height:drawing.height}};});
 for(const button of document.querySelectorAll('button[aria-label]'))if(visible(button) && /^[^\p{L}\p{N}\s]$/u.test(button.textContent.trim()) && !button.querySelector('svg'))issues.push('text icon '+button.outerHTML);
 for(const svg of document.querySelectorAll('svg'))if(visible(svg) && !svg.classList.contains('function-icon') && !svg.matches('.ability-radar,.tier-badge,.multikill-badge,.score-sparkline') && !svg.closest('.mk-tag,.score-sparkline,.tier-badge'))issues.push('uncatalogued '+svg.outerHTML.slice(0,150));
 return {count:icons.length,rows,issues};
}
async function run(){await withBrowser(async({page,out,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 await p.evaluate("fetch('/icons.svg').then(r=>r.text()).then(xml=>{__r280SpriteIDs=new Set([...new DOMParser().parseFromString(xml,'image/svg+xml').querySelectorAll('symbol[id]')].map(n=>n.id));})");
 for(const zoom of [1,1.4,2]){
  await p.evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}'`);
  for(const section of ['overview','champions','live','favorites','suite','settings']){
   await p.evaluate(`__r278App.activateSection('${section}')`);
   await p.evaluate("new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
   const report=await p.evaluate('('+audit.toString()+')()');results.push({zoom,section,...report});
   if(zoom===2)await p.shot('icons-'+section);
   assert.deepEqual(report.issues,[],section+' '+JSON.stringify(report.issues));
  }
  await p.call('Emulation.setDeviceMetricsOverride',{width:1100,height:1100,deviceScaleFactor:1.75,mobile:false});
  await p.evaluate("__r278App.activateSection('overview')");
  for(const [name,open,close] of [
   ['career-dialog',"document.querySelector('[data-open-career-dialog]').click()","document.getElementById('career-dialog-close').click()"],
   ['announcement-dialog',"document.getElementById('announcements-button').click()","document.querySelector('[data-announcements-close]').click()"],
   ['filter-dialog',"document.querySelector('[data-af-open]').click()","document.querySelector('[data-af-close]').click()"]
  ]) {
   if(name==='filter-dialog'){await p.evaluate("window.deepLegendsLoadHistoryFilters(true)");await p.until("document.querySelector('[data-af-open]')");}
   await p.evaluate(open);await p.evaluate("new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
   const report=await p.evaluate('('+audit.toString()+')()');results.push({zoom,section:name,...report});
   if(zoom===2)await p.shot('icons-'+name);assert.deepEqual(report.issues,[],name+' '+JSON.stringify(report.issues));await p.evaluate(close);
  }
  await p.call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1.75,mobile:false});
  await p.evaluate("(()=>{const a=__r278App;window.__r280Account={summoner:__r280Overview.player,account:{loot:['材料','宝箱','英雄','皮肤','小小英雄','永恒星碑','表情','守卫','图标'].map((category,i)=>({category,lootId:'review-'+i,displayName:category,count:1})),rewards:[],capabilities:[]}};a.activateSection('favorites');a.activateFavoritesPage('account');return a.loadAccount();})()");
  await p.until("document.querySelectorAll('.loot-category-icon').length===9");
  await p.evaluate("new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
  const loot=await p.evaluate('('+audit.toString()+')()');results.push({zoom,section:'collection-account',...loot});assert.deepEqual(loot.issues,[],'collection-account '+JSON.stringify(loot.issues));
  if(zoom===2)await p.shot('icons-collection-account');
  await p.evaluate("window.__r280FriendsPayload={groups:[],friends:[]};__r280Friends.setOpen(true)");
  await p.until("!__r280Friends.state.loading");
  for(const empty of ['disconnected','search','empty']){
   await p.evaluate(`(()=>{const f=__r280Friends;f.state.connected=${empty!=='disconnected'};f.state.filter='${empty==='search'?'no-match':''}';f.state.friends=[];f.state.groups=[];f.state.loading=false;f.state.error='';f.state.stale=false;f.render();return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()`);
   const report=await p.evaluate('('+audit.toString()+')()');results.push({zoom,section:'friends-'+empty,...report});assert.deepEqual(report.issues,[],empty+' '+JSON.stringify(report.issues));
   if(zoom===2)await p.shot('icons-friends-'+empty);
  }
  await p.evaluate("__r280Friends.state.connected=true;__r280Friends.state.filter='';window.__r280FriendsPayload=null;__r280Friends.loadFriends('audit')");
  await p.until("!__r280Friends.state.loading && document.querySelector('.friends-group-head')");
  await p.evaluate("new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
  const populated=await p.evaluate('('+audit.toString()+')()');results.push({zoom,section:'friends-populated',...populated});assert.deepEqual(populated.issues,[],'friends-populated '+JSON.stringify(populated.issues));
  await p.evaluate("__r280Friends.setOpen(false);__r278App.activateFavoritesPage('collection')");
 }
 // A missing sprite must still fail after waiting for its rendering frame.
 await p.evaluate("window.__r280Glyph=document.querySelector('.topbar .function-icon use');window.__r280GlyphHref=__r280Glyph.getAttribute('href');__r280Glyph.setAttribute('href','/icons.svg#dl-icon-missing');new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
 assert((await p.evaluate('('+audit.toString()+')()')).issues.some(s=>s.startsWith('empty drawing')));
 await p.evaluate("__r280Glyph.setAttribute('href',__r280GlyphHref);new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
 // Both mutations must be detected in the actual browser.
 await p.evaluate("document.querySelector('.topbar .function-icon').style.setProperty('stroke-width','2.2','important')");
 assert((await p.evaluate('('+audit.toString()+')()')).issues.some(s=>s.startsWith('stroke')));
 await p.evaluate("document.querySelector('.topbar .function-icon').style.removeProperty('stroke-width');const n=document.createElement('button');n.id='r280-icon-mutation';n.setAttribute('aria-label','关闭');n.textContent='×';document.querySelector('.topbar').append(n)");
 assert((await p.evaluate('('+audit.toString()+')()')).issues.some(s=>s.startsWith('text icon')));
 await p.evaluate("document.getElementById('r280-icon-mutation').remove()");
 fs.writeFileSync(path.join(out,'icon-audit.json'),JSON.stringify(results,null,2));assert.equal(errors.length,0,JSON.stringify(errors));
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={audit,run};
