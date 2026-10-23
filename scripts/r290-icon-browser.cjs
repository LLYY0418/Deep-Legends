'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']) {
 const p=await page(theme);
 for(const width of [1440,900,760,640])for(const zoom of [1,1.4,2]){
  await p.call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1.75,mobile:false});
  await p.evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}';__r278App.applySidebar();__r278App.renderUpdateStatus({supported:true,current:'0.12.91',latest:'0.12.92',state:'available'});document.querySelector('.topbar').dispatchEvent(new Event('topbar-layout'))`);
  await p.until("document.querySelector('.topbar').dataset.topbarSettled==='true'");
  const geometry=await p.evaluate(`(()=>{const b=document.querySelector('#update-button'),s=b.querySelector('svg'),bell=document.querySelector('#announcements-button svg'),track=bell.querySelector('path'),arrows=[...s.querySelectorAll('path')],r=n=>{const x=n.getBoundingClientRect();return {width:x.width,height:x.height}},box=n=>{const x=n.getBBox();return {y:x.y,height:x.height}},a=arrows.map(box),lo=Math.min(...a.map(x=>x.y)),hi=Math.max(...a.map(x=>x.y+x.height));return {compact:document.querySelector('.topbar').dataset.topbarCompact,variable:getComputedStyle(s).getPropertyValue('--topbar-icon-size'),computed:getComputedStyle(s).width,update:r(s),bell:r(bell),button:r(b),ratio:(hi-lo)/track.getBBox().height,stroke:getComputedStyle(track).strokeWidth,bellStroke:getComputedStyle(bell.querySelector('path')).strokeWidth,vector:getComputedStyle(s.querySelector('path')).vectorEffect,fresh:b.classList.contains('fresh')}})()`);
  for(const dimension of ['width','height'])assert(Math.abs(geometry.update[dimension]-geometry.bell[dimension])<=.02,'update and bell rendering size');
  assert(Math.abs(parseFloat(geometry.computed)-parseFloat(geometry.variable))<=.02,'computed size follows shared variable within Chrome zoom rounding');
  assert(geometry.ratio>=.55,'arrow height ratio');assert.equal(geometry.stroke,geometry.bellStroke);assert.equal(geometry.stroke,'1.8px');assert.equal(geometry.vector,'non-scaling-stroke');
  assert(geometry.update.width<geometry.button.width&&geometry.update.height<geometry.button.height);
  const states=await p.evaluate(`(()=>{const b=document.querySelector('#update-button');__r278App.renderUpdateStatus({supported:true,current:'0.12.91',latest:'0.12.92',state:'downloading',progress:{receivedBytes:42,totalBytes:100}});const progress=b.querySelector('.update-border-progress');const downloading={busy:b.classList.contains('busy'),offset:Number(progress.getAttribute('stroke-dashoffset')),length:progress.getAttribute('pathLength')};__r278App.renderUpdateStatus({supported:true,current:'0.12.91',latest:'0.12.92',state:'ready'});return {downloading,ready:b.classList.contains('ready'),dot:!b.querySelector('.dot').hidden}})()`);
  assert(states.downloading.busy);assert(Math.abs(states.downloading.offset-100*.58)<1e-8);assert.equal(states.downloading.length,'100');assert(states.ready&&states.dot);
  for(const state of ['available','downloading','ready']){
   await p.evaluate(`__r278App.renderUpdateStatus({supported:true,current:'0.12.93',latest:'0.12.94',state:'${state}',progress:{receivedBytes:42,totalBytes:100}})`);
   const clip=await p.evaluate(`(()=>{const a=document.querySelector('#announcements-button').getBoundingClientRect(),b=document.querySelector('#update-button').getBoundingClientRect();return {x:Math.max(0,a.x-6),y:Math.max(0,a.y-6),width:b.right-a.x+12,height:Math.max(a.height,b.height)+12,scale:1}})()`);
   const shot=await p.call('Page.captureScreenshot',{format:'png',clip});require('node:fs').writeFileSync(require('node:path').join(process.env.R280_BROWSER_OUT,`${theme}-${width}-${zoom}-${state}.png`),Buffer.from(shot.data,'base64'));
  }
  results.push({theme,width,zoom,geometry,states});
 }
 }
 assert(results.some(x=>x.geometry.compact==='2'),'compact 2 covered');assert(results.some(x=>x.geometry.compact==='3'),'compact 3 covered');
 assert.deepEqual(errors,[]);
}).catch(e=>{console.error(e);process.exitCode=1});
