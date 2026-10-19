'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']){
  const p=await page(theme);await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
  for(const zoom of [1,1.5]){
   const value=await p.evaluate(`(()=>{let stage=document.getElementById('r288-rank-stage');if(!stage){stage=document.createElement('div');stage.id='r288-rank-stage';stage.style.cssText='position:absolute;top:30px;left:30px;width:360px;z-index:9999;background:var(--bg)';document.body.append(stage)}stage.style.zoom=${zoom};stage.innerHTML=__r278Game.renderRanks([],[],[],[],{upstreamCapped:true,rankedOldestAt:new Date('2026-04-26T12:00:00+08:00').getTime()});const label=stage.querySelector('.rank-header-tools>span').getBoundingClientRect(),icon=stage.querySelector('.season-data-note'),rect=icon.getBoundingClientRect();return {theme:'${theme}',zoom:${zoom},centerDifference:Math.abs((label.top+label.bottom-rect.top-rect.bottom)/2),tooltip:icon.dataset.tooltip,label:stage.querySelector('.rank-header-tools>span').textContent}})()`);
   assert(value.centerDifference<=1,JSON.stringify(value));assert.equal(value.label,'当前整个赛季');assert.equal(value.tooltip,'官方接口仅提供 4月26日 之后的对局');results.push(value);
   await p.evaluate("document.getElementById('r288-rank-stage').innerHTML=__r278Game.renderRanks([],[],[],[],{upstreamCapped:false})");assert.equal(await p.evaluate("document.querySelectorAll('#r288-rank-stage .season-data-note').length"),0);
  }
 }
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
