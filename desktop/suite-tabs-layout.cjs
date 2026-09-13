'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// SUITE_TABS_LAYOUT=1 CURRENT_GAME_SHOTS=/private/tmp/suite-tabs-2153 node desktop/current-game-layout.cjs
// Uses only the demo backend; never connects to the League client.
exports.verify = async ({call, evaluate, output}) => {
  await evaluate(`document.getElementById('section-suite').click();
    {const s=document.getElementById('setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));}`);
  const order = ['watch', 'rig', 'champselect', 'facade', 'sweep'];
  const results = [];
  for (const width of [1600, 1440, 820, 620, 420]) {
    await call('Emulation.setDeviceMetricsOverride', {width,height:900,deviceScaleFactor:1,mobile:false});
    await evaluate(`document.querySelector('[data-suite-tab="watch"]').click();
      new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
    const measure = await evaluate(`(()=>{
      const rect=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,w:r.width,h:r.height,right:r.right,bottom:r.bottom};};
      return {documentOverflow:document.documentElement.scrollWidth>innerWidth,
        tabs:[...document.querySelectorAll('.suite-tab')].map(tab=>{
          const svg=tab.querySelector('.suite-tab-icon svg'), bounds=svg.getBBox();
          const label=tab.querySelector('strong'), css=getComputedStyle(tab);
          return {key:tab.dataset.suiteTab,box:rect(tab),icon:rect(svg),art:{w:bounds.width,h:bounds.height},label:rect(label),color:getComputedStyle(label).color,expected:css.getPropertyValue(tab.classList.contains('is-active')?'--primary':'--ink').trim()};
        })};
    })()`);
    assert.equal(measure.documentOverflow,false,JSON.stringify({width,...measure}));
    assert.deepEqual(measure.tabs.map(t=>t.key),order);
    for (const tab of measure.tabs) {
      assert.ok(Math.abs(tab.icon.w-22)<0.1 && Math.abs(tab.icon.h-22)<0.1,JSON.stringify({width,tab}));
      const expected = await evaluate(`(()=>{const e=document.createElement('span');e.style.color=${JSON.stringify(tab.expected)};document.body.appendChild(e);const color=getComputedStyle(e).color;e.remove();return color;})()`);
      assert.equal(tab.color,expected,'selected title uses theme color; other titles use normal text color');
      assert.equal(tab.art.w,20,'all symbols must occupy the same drawing width');
      assert.equal(tab.art.h,20,'all symbols must occupy the same drawing height');
      assert.ok(tab.icon.right<=tab.label.x,'icon must not overlap the label');
      assert.ok(tab.label.h<24 && tab.label.right<=tab.box.right,'labels must stay on one line inside the tab; narrow layouts scroll instead of squeezing labels');
      assert.ok(Math.abs(tab.icon.y+tab.icon.h/2-tab.box.y-tab.box.h/2)<1,'icon vertically centered');
    }
    // Real keyboard events validate DOM order, focus, selected state and the visible panel.
    await evaluate(`document.querySelector('[data-suite-tab="rig"]').focus()`);
    await call('Input.dispatchKeyEvent',{type:'keyDown',key:'ArrowRight',code:'ArrowRight',windowsVirtualKeyCode:39});
    await call('Input.dispatchKeyEvent',{type:'keyUp',key:'ArrowRight',code:'ArrowRight',windowsVirtualKeyCode:39});
    const active = await evaluate(`({key:document.activeElement.dataset.suiteTab,
      selected:[...document.querySelectorAll('.suite-tab[aria-selected="true"]')].map(t=>t.dataset.suiteTab),
      panelHidden:document.getElementById('suite-champselect-panel').hidden})`);
    assert.equal(active.key,'champselect');
    assert.deepEqual(active.selected,['champselect']);
    assert.equal(active.panelHidden,false);
    await evaluate(`document.querySelector('[data-suite-tab="watch"]').click();
      document.activeElement.blur();
      document.querySelector('.suite-tabs').scrollLeft=0;
      new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
    const clip = await evaluate(`(()=>{const r=document.querySelector('.suite-tabs').getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,scale:1};})()`);
    const shot=await call('Page.captureScreenshot',{format:'png',clip});
    fs.writeFileSync(path.join(output,`suite-tabs-${width}.png`),Buffer.from(shot.data,'base64'));
    results.push({width,...measure,active});
  }
  fs.writeFileSync(path.join(output,'suite-tabs-layout.json'),JSON.stringify(results,null,2));
  console.log('Suite tabs PASS: order, equal 22px icons / 20-unit drawings, normal and theme title colors, no overlap, keyboard + panel activation at 5 widths');
};
