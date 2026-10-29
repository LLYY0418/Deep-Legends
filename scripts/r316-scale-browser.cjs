'use strict';
process.env.R282_SCREEN_CATALOG='1';
const assert=require('node:assert/strict'),fs=require('node:fs'),{withBrowser}=require('./r280-browser-fixture.cjs');
(async()=>withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 await p.call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});
 await p.call('Emulation.setDeviceMetricsOverride',{width:1646,height:981,deviceScaleFactor:1.75,mobile:false});
 const frame=()=>p.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 for(const zoom of [1.75,2]){
  await p.evaluate(`localStorage.setItem('lol-loot-ui-scale','${zoom}');window.deepLegendsPreferenceStorage?.setItem('lol-loot-ui-scale','${zoom}');__r278App.applyUiScale();window.dispatchEvent(new Event('resize'))`);await frame();
  const capture=async(name)=>{
   await frame();await p.until(`(async()=>{const s=document.querySelector('.app-scroll');s.style.scrollBehavior='auto';s.scrollTo({top:s.scrollHeight,behavior:'instant'});await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return s.scrollTop+s.clientHeight>=s.scrollHeight-1})()`);
   const g=await p.evaluate(`(()=>{const s=document.querySelector('.app-scroll'),top=document.querySelector('.topbar'),r=s.getBoundingClientRect();const horizontal=[...s.querySelectorAll('*')].filter(n=>n.getBoundingClientRect().width>0&&/auto|scroll/.test(getComputedStyle(n).overflowX)&&n.scrollWidth>n.clientWidth+1).map(n=>({className:n.className,extra:n.scrollWidth-n.clientWidth}));return {zoom:parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--ui-zoom')),top:s.scrollTop,height:s.clientHeight,total:s.scrollHeight,overflow:s.scrollWidth-s.clientWidth,horizontal,bottom:r.bottom,topbar:top?.getBoundingClientRect().height}})()`);
   results.push({name,zoom,...g});await p.shot(`${name}-${zoom}`);assert.equal(g.zoom,zoom);assert(g.top+g.height>=g.total-1,JSON.stringify(g));assert(g.overflow<=1,`${name} ${JSON.stringify(g)}`);assert.deepEqual(g.horizontal,[],`${name} inner horizontal scrollbar`);assert(g.bottom<=982);assert(Math.abs(g.topbar-56*zoom)<=1,JSON.stringify(g));
  };
  for(const section of ['overview','live','champions','suite','settings']){
   await p.evaluate(`__r278App.activateSection('${section}')`);await frame();await capture(section);
   if(section==='champions')for(const mode of ['ranked','aram-mayhem','arena']){await p.evaluate(`document.querySelector('[data-champion-mode="${mode}"]').click()`);await p.until(`document.querySelector('[data-champion-mode="${mode}"].is-active') && document.querySelector('[data-champion-refresh]')?.disabled===false`);await capture('champions-'+mode);}
   if(section==='suite')for(const sub of ['watch','champselect','config','facade','sweep','rig']){await p.evaluate(`document.querySelector('[data-suite-tab="${sub}"]').click()`);await capture('tool-'+sub);}
   if(section==='settings')for(const sub of ['appearance','gameplay','privacy']){await p.evaluate(`document.querySelector('[data-settings-page="${sub}"]')?.click()`);await capture('settings-'+sub);}
  }
 }
 assert.deepEqual(errors,[]);
}))().catch(e=>{console.error(e);process.exitCode=1});
