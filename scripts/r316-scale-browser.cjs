'use strict';
process.env.R282_SCREEN_CATALOG='1';
const assert=require('node:assert/strict'),fs=require('node:fs'),{withBrowser}=require('./r280-browser-fixture.cjs');
(async()=>withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 await p.call('Emulation.setDeviceMetricsOverride',{width:1646,height:981,deviceScaleFactor:1.75,mobile:false});
 const frame=()=>p.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 for(const zoom of [1.75,2]){
  await p.evaluate(`localStorage.setItem('lol-loot-ui-scale','${zoom}');window.deepLegendsPreferenceStorage?.setItem('lol-loot-ui-scale','${zoom}');__r278App.applyUiScale();window.dispatchEvent(new Event('resize'))`);await frame();
  const capture=async(name)=>{
   await frame();for(let i=0;i<3;i++){await p.evaluate(`document.querySelector('.app-scroll').scrollTop=document.querySelector('.app-scroll').scrollHeight`);await frame();}
   const g=await p.evaluate(`(()=>{const s=document.querySelector('.app-scroll'),top=document.querySelector('.topbar'),r=s.getBoundingClientRect();return {zoom:parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--ui-zoom')),top:s.scrollTop,height:s.clientHeight,total:s.scrollHeight,overflow:s.scrollWidth-s.clientWidth,bottom:r.bottom,topbar:top?.getBoundingClientRect().height}})()`);
   results.push({name,zoom,...g});assert.equal(g.zoom,zoom);assert(g.top+g.height>=g.total-1,JSON.stringify(g));assert(g.overflow<=1,`${name} ${JSON.stringify(g)}`);assert(g.bottom<=982);assert(Math.abs(g.topbar-56*zoom)<=1,JSON.stringify(g));await p.shot(`${name}-${zoom}`);
  };
  for(const section of ['overview','live','champions','suite','settings']){
   await p.evaluate(`__r278App.activateSection('${section}')`);await frame();await capture(section);
   if(section==='champions')for(const mode of ['ranked','aram-mayhem','arena']){await p.evaluate(`document.querySelector('[data-champion-mode="${mode}"]')?.click()`);await p.until("document.querySelector('[data-champion-row]') || document.querySelector('.arena-pane-empty')");await capture('champions-'+mode);}
   if(section==='suite')for(const sub of ['watch','champselect','config','facade','sweep','rig']){await p.evaluate(`document.querySelector('[data-suite-tab="${sub}"]').click()`);await capture('tool-'+sub);}
   if(section==='settings')for(const sub of ['appearance','gameplay','privacy']){await p.evaluate(`document.querySelector('[data-settings-page="${sub}"]')?.click()`);await capture('settings-'+sub);}
  }
 }
 assert.deepEqual(errors,[]);
}))().catch(e=>{console.error(e);process.exitCode=1});
