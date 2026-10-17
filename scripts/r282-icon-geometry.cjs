'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
function geometry(){
 const visible=n=>n.getBoundingClientRect().width>0&&getComputedStyle(n).visibility!=='hidden';const rows=[];
 for(const control of document.querySelectorAll('.app-select-trigger,.player-search-region>button,.match-more-button,[aria-haspopup="listbox"]')){
  if(!visible(control))continue;const text=[...control.querySelectorAll('span,strong')].find(n=>visible(n)&&n.textContent.trim()&&!n.closest('svg')) || control;const tr=text.getBoundingClientRect(),style=getComputedStyle(text),line=parseFloat(style.lineHeight)||parseFloat(style.fontSize)*1.2,scale=Number(document.documentElement.dataset.uiScale)||1,top=tr.top+(tr.height-line*scale)/2,bottom=top+line*scale;
  const pseudo=getComputedStyle(control,'::after');let arrow=pseudo.content!=='none'&&pseudo.display!=='none'?null:control.querySelector('svg path,svg polyline,svg'),temporary;
  if(!arrow){const c=getComputedStyle(control,'::after');if(c.content==='none'||c.display==='none')continue;temporary=document.createElement('span');for(const key of c)temporary.style.setProperty(key,c.getPropertyValue(key));temporary.textContent=c.content.replace(/^"|"$/g,'');control.append(temporary);arrow=temporary;}
  const ar=arrow.getBoundingClientRect();rows.push({kind:'arrow',selector:control.id||control.closest('.select-wrap')?.querySelector('select')?.id||control.className,top:ar.top,lineTop:top,lineBottom:bottom,okay:ar.top>=top-1&&ar.top<=bottom+1});temporary?.remove();
 }
 if(document.documentElement.dataset.sidebar!=='collapsed')for(const n of document.querySelectorAll('.section-tab')){if(!visible(n))continue;const icon=n.querySelector('svg'),text=n.querySelector('span:not(.live-beacon)');if(!icon||!text||!visible(text))continue;const a=icon.getBoundingClientRect(),b=text.getBoundingClientRect();rows.push({kind:'sidebar',selector:n.id,okay:a.left<b.left&&Math.abs((a.top+a.bottom-b.top-b.bottom)/2)<=1});}
 for(const n of document.querySelectorAll('.player-tab-close')){if(!visible(n))continue;const a=n.getBoundingClientRect(),range=document.createRange();range.selectNodeContents(n);const b=range.getBoundingClientRect();rows.push({kind:'tab-close',okay:Math.abs((a.left+a.right-b.left-b.right)/2)<=1&&Math.abs((a.top+a.bottom-b.top-b.bottom)/2)<=1,button:a.toJSON(),glyph:b.toJSON()});}
 return rows;
}
async function run(){await withBrowser(async({page,results,errors})=>{const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 for(const zoom of [.9,1,1.14,1.5,2]){
  await p.call('Emulation.setDeviceMetricsOverride',{width:2189,height:1100,deviceScaleFactor:1,mobile:false});await p.evaluate(`localStorage.setItem('lol-loot-ui-scale','${zoom}');document.documentElement.dataset.uiScale='${zoom}';document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.sidebar='expanded'`);
  for(const section of ['overview','settings','champions','suite','favorites']){await p.evaluate(`__r278App.activateSection('${section}')`);await p.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
   if(section==='settings')for(const tab of ['appearance','gameplay','privacy']){await p.evaluate(`document.querySelector('[data-settings-page="${tab}"]')?.click()`);const rows=await p.evaluate('('+geometry.toString()+')()');results.push({kind:'icon-geometry',section,tab,zoom,rows});for(const row of rows)assert(row.okay,JSON.stringify({section,tab,zoom,row}));}
   const rows=await p.evaluate('('+geometry.toString()+')()');results.push({kind:'icon-geometry',section,zoom,rows});for(const row of rows)assert(row.okay,JSON.stringify({section,zoom,row}));
  }
 }
 await p.evaluate("document.documentElement.dataset.uiScale='1';document.documentElement.style.setProperty('--ui-zoom','1');__r278App.activateSection('overview');document.querySelector('[data-af-open]').click()");await p.until("document.querySelector('[data-af-menu]')?.open");const rows=await p.evaluate('('+geometry.toString()+')()');for(const row of rows)assert(row.okay,JSON.stringify(row));results.push({kind:'icon-geometry',section:'filter-dialog',rows});
 await p.evaluate("document.querySelector('[data-af-close]').click()");
 await p.call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});
 const animations=await p.evaluate("(()=>{const n=document.createElement('div');n.id='r282-motion';n.innerHTML=__r278Game.renderOverviewStreak({result:'win',count:3})+__r278Game.renderOverviewStreak({result:'loss',count:3});document.body.append(n);return [...n.querySelectorAll('svg *')].map(e=>getComputedStyle(e).animationName)})()");assert(animations.length>=6);assert(animations.every(n=>n==='none'));results.push({kind:'reduced-motion',animations});assert.deepEqual(errors,[]);
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={geometry,run};
