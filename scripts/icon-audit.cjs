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
  const prior=groups.get(key);if(prior && (Math.abs(prior.width-r.width)>.5 || Math.abs(prior.height-r.height)>.5))issues.push('unequal '+key+' '+n.outerHTML.slice(0,180));groups.set(key,r);
  return {module:module(n),size,width:r.width,height:r.height,stroke:style.strokeWidth,vector:u?.vectorEffect};});
 for(const button of document.querySelectorAll('button[aria-label]'))if(visible(button) && /^[^\p{L}\p{N}\s]$/u.test(button.textContent.trim()) && !button.querySelector('svg'))issues.push('text icon '+button.outerHTML);
 for(const svg of document.querySelectorAll('svg'))if(visible(svg) && !svg.classList.contains('function-icon') && !svg.matches('.ability-radar,.tier-badge,.multikill-badge,.score-sparkline') && !svg.closest('.mk-tag,.score-sparkline,.tier-badge'))issues.push('uncatalogued '+svg.outerHTML.slice(0,150));
 return {count:icons.length,rows,issues};
}
async function run(){await withBrowser(async({page,out,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
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
 }
 // Both mutations must be detected in the actual browser.
 await p.evaluate("document.querySelector('.topbar .function-icon').style.setProperty('stroke-width','2.2','important')");
 assert((await p.evaluate('('+audit.toString()+')()')).issues.some(s=>s.startsWith('stroke')));
 await p.evaluate("document.querySelector('.topbar .function-icon').style.removeProperty('stroke-width');const n=document.createElement('button');n.id='r280-icon-mutation';n.setAttribute('aria-label','关闭');n.textContent='×';document.querySelector('.topbar').append(n)");
 assert((await p.evaluate('('+audit.toString()+')()')).issues.some(s=>s.startsWith('text icon')));
 await p.evaluate("document.getElementById('r280-icon-mutation').remove()");
 fs.writeFileSync(path.join(out,'icon-audit.json'),JSON.stringify(results,null,2));assert.equal(errors.length,0,JSON.stringify(errors));
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={audit,run};
