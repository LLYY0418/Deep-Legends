'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// Runs inside the existing production-page Chromium harness. No mocked CSS or
// jsdom geometry: real scroll ancestors, container queries and page navigation.
exports.verify = async ({call, evaluate, output}) => {
  await evaluate(`{
    const original = window.fetch;
    window.fetch = async (input, init) => {
      const response = await original(input, init);
      const url = String(input);
      if (!url.startsWith('/api/gameplay/overview') && url !== '/api/gameplay/live') return response;
      const data = await response.json();
      if (url.startsWith('/api/gameplay/overview')) data.player.privateHistory = true;
      else if (data.recommendations?.build) {
        data.recommendations.build.fifthOptions = [];
        data.recommendations.build.sixthOptions = [];
        data.recommendations.build.itemChainStatus = 'ready';
      }
      return new Response(JSON.stringify(data));
    };
    document.getElementById('overview-refresh').click();
  }`);
  const waitFor = expression => evaluate(`new Promise((resolve,reject)=>{const start=Date.now();const timer=setInterval(()=>{if(${expression}){clearInterval(timer);resolve();}else if(Date.now()-start>10000){clearInterval(timer);reject(Error('missing expected UI '+JSON.stringify({liveHidden:document.querySelector('#live-panel')?.hidden,buildHidden:document.querySelector('#recommendation-panel-build')?.hidden,buildClass:document.querySelector('#recommendation-panel-build')?.className,body:document.querySelector('#live-content')?.textContent.slice(-700)})));}},40);})`);
  await waitFor(`document.querySelector('.summoner-level-row .player-tab-hidden')`);
  const overview = await evaluate(`({badgeInTabs:document.querySelectorAll('.player-tab .player-tab-hidden').length,badgeInBanner:document.querySelectorAll('.summoner-level-row .player-tab-hidden').length,players:document.querySelectorAll('.current-game-player').length})`);
  assert.equal(overview.badgeInTabs, 0); assert.equal(overview.badgeInBanner, 1); assert.equal(overview.players, 10);
  await evaluate(`document.getElementById('section-live').click()`);
  await waitFor(`document.querySelector('[data-recommendation-tab="build"]')`);
  await evaluate(`document.querySelector('[data-recommendation-tab="build"]').click()`);
  await waitFor(`document.querySelector('#recommendation-panel-build:not([hidden]):not(.is-loading) .item-set-action')?.getBoundingClientRect().height > 0`);
  const summaries = [];
  for (const theme of ['dark', 'light']) for (const width of [1440, 820, 620]) {
    await call('Emulation.setDeviceMetricsOverride', {width, height:900, deviceScaleFactor:1, mobile:false});
    await evaluate(`{const s=document.getElementById('setting-theme');s.value=${JSON.stringify(theme)};s.dispatchEvent(new Event('change',{bubbles:true}));}`);
    await evaluate(`new Promise(r=>setTimeout(r,500))`);
    await evaluate(`document.querySelector('[data-recommendation-tab="build"]').click()`);
    await waitFor(`document.querySelector('#recommendation-panel-build:not([hidden]):not(.is-loading) .item-set-action')?.getBoundingClientRect().height > 0`);
    await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
    const positions = [];
    for (const offset of ["start", 0, 170, 340, "end"]) {
      await evaluate(`{const panel=document.querySelector('#recommendation-panel-build');const scroll=document.querySelector('.app-scroll');if (${JSON.stringify(offset)} === "start") scroll.scrollTop=0; else if (${JSON.stringify(offset)} === "end") scroll.scrollTop=scroll.scrollHeight; else scroll.scrollTop+=panel.getBoundingClientRect().top-scroll.getBoundingClientRect().top+${Number(offset) || 0};}`);
      await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
      await waitFor(`document.querySelector('#recommendation-panel-build:not([hidden]):not(.is-loading) .item-set-action')?.getBoundingClientRect().height > 0`);
      const box = await evaluate(`(()=>{const action=document.querySelector('#recommendation-panel-build .item-set-action');const build=document.querySelector('#recommendation-panel-build .build-recommendation');const b=action.getBoundingClientRect(),s=document.querySelector('.app-scroll').getBoundingClientRect();return {top:b.top,bottom:b.bottom,scrollBottom:s.bottom,width:b.width,visible:b.top>=s.top&&b.bottom<=s.bottom+1,scrollRemaining:document.querySelector('.app-scroll').scrollHeight-document.querySelector('.app-scroll').scrollTop-document.querySelector('.app-scroll').clientHeight,sticky:getComputedStyle(action).position,insideClippedBuild:build.contains(action),fourth:build.querySelector('.item-depth-columns section:first-child .config-option-list')?.children.length,fifth:build.querySelector('.item-depth-columns section:nth-child(2)')?.textContent,overflow:document.documentElement.scrollWidth>innerWidth};})()`);
      assert.equal(box.insideClippedBuild,false);
      assert.equal(box.sticky,'sticky');assert.equal(box.visible,true,JSON.stringify({width,offset,box}));
      assert.equal(box.overflow,false);assert.equal(box.fourth,5);assert.match(box.fifth,/暂无可用样本/);
      assert.ok(Math.abs(box.bottom-box.scrollBottom)<2,JSON.stringify({width,offset,box}));
      positions.push(box);
    }
    assert.ok(positions[0].scrollRemaining>100,'button must be visible before the scroll ends');
    assert.ok(Math.abs(positions[0].bottom-positions[1].bottom)<1,'scroll must not move the action');
    const shot=await call('Page.captureScreenshot',{format:'png'});
    fs.writeFileSync(path.join(output,`build-${theme}-${width}.png`),Buffer.from(shot.data,'base64'));
    summaries.push({theme,width,positions});
  }
  fs.writeFileSync(path.join(output,'1555-layout.json'),JSON.stringify({overview,summaries},null,2));
  console.log('1555 production layout PASS: hidden-history roster, single badge, fixed actions in 6 theme/size combinations');
};
