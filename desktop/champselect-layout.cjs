'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// CHAMPSELECT_LAYOUT=1 CURRENT_GAME_SHOTS=/private/tmp/champselect-1505 node desktop/current-game-layout.cjs
exports.verify = async ({call, evaluate, output}) => {
  const waitFor = expression => evaluate(`new Promise((resolve,reject)=>{const start=Date.now();const timer=setInterval(()=>{if(${expression}){clearInterval(timer);resolve();}else if(Date.now()-start>8000){clearInterval(timer);reject(Error('champselect UI did not settle'));}},40);})`);
  await evaluate(`document.getElementById('section-suite').click();document.querySelector('[data-suite-tab="champselect"]').click()`);
  await waitFor(`document.querySelector('[data-cs-rail-side="ban"]')`);
  const timeline = await evaluate(`document.querySelector('.cs-live-timeline').textContent`);
  assert.doesNotMatch(timeline, /英雄\s+-?\d+/, 'user-facing records must resolve catalog names, including legacy records');
  const timelineName = await evaluate(`fetch('/api/champions/catalog').then(r=>r.json()).then(c=>{const hero=c.champions.find(h=>Number(h.id)===103);return hero.nameZh||hero.nameEn;})`);
  assert.ok(timelineName && timeline.includes(timelineName), 'champion 103 must use the actual catalog name, not a hard-coded alias');
  await call('Emulation.setDeviceMetricsOverride', {width:1600, height:1100, deviceScaleFactor:1, mobile:false});
  await evaluate(`{const s=document.getElementById('setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));}`);
  await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
  await evaluate(`document.querySelector('[data-cs-rail-side="ban"]').scrollIntoView({block:'center',behavior:'instant'})`);
  await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
  const before = await evaluate(`[...document.querySelectorAll('[data-cs-rail-side="ban"]')].map(e=>e.dataset.csChampionId)`);
  const points = await evaluate(`(()=>{const rows=[...document.querySelectorAll('[data-cs-rail-side="ban"]')];return [rows[0],rows[3]].map(e=>{const r=e.querySelector('.cs-avatar').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};});})()`);
  await evaluate(`{window.railDragEvents=[];for(const name of ['mousedown','mousemove','mouseup','dragstart','dragover','drop','dragend'])document.addEventListener(name,e=>window.railDragEvents.push({name,buttons:e.buttons,tag:e.target.tagName,side:e.target.closest('[data-cs-rail-side]')?.dataset.csRailSide,index:e.target.closest('[data-cs-rail-index]')?.dataset.csRailIndex}),true);}`);
  await call('Input.dispatchMouseEvent', {type:'mouseMoved', ...points[0]});
  await call('Input.dispatchMouseEvent', {type:'mousePressed', ...points[0], button:'left', buttons:1, clickCount:1});
  for (let i=1;i<=12;i++) await call('Input.dispatchMouseEvent', {type:'mouseMoved', x:points[0].x+(points[1].x-points[0].x)*i/12,y:points[0].y,button:'left',buttons:1});
  await call('Input.dispatchMouseEvent', {type:'mouseReleased', ...points[1], button:'left',buttons:0,clickCount:1});
  try { await waitFor(`document.querySelector('[data-cs-rail-side="ban"]').dataset.csChampionId === ${JSON.stringify(before[3])}`); }
  catch (error) { console.log('drag evidence',points,await evaluate(`({events:window.railDragEvents,rows:[...document.querySelectorAll('[data-cs-rail-side="ban"]')].map(e=>({id:e.dataset.csChampionId,draggable:e.draggable,classes:e.className})),dialog:!!document.querySelector('.cs-dialog')})`));throw error; }
  const after = await evaluate(`[...document.querySelectorAll('[data-cs-rail-side="ban"]')].map(e=>e.dataset.csChampionId)`);
  const expected = [...before]; [expected[0],expected[3]]=[expected[3],expected[0]];
  assert.deepEqual(after,expected,'real Chromium mouse drag must swap and persist, not open the dialog');
  assert.equal(await evaluate(`!!document.querySelector('.cs-dialog')`),false);
  await evaluate(`{const input=document.querySelector('[data-cs-time="ban"]');input.value='2.25';input.dispatchEvent(new Event('change',{bubbles:true}));}`);
  await waitFor(`document.querySelector('[data-cs-time="ban"]').value === '2.25'`);
  await evaluate(`document.querySelector('[data-cs-time="ban"]').focus()`);
  const fieldShot = await call('Page.captureScreenshot',{format:'png'});
  fs.writeFileSync(path.join(output,'champselect-centered-input.png'),Buffer.from(fieldShot.data,'base64'));
  // A real click moves focus out of the protected time input before opening.
  await evaluate(`{const button=document.querySelector('[data-cs-open-dialog="ban"]');button.focus();button.click();}`);
  await waitFor(`document.querySelector('.cs-dialog')?.open`);
  const results = [];
  for (const width of [1440,820,620]) {
    await call('Emulation.setDeviceMetricsOverride', {width,height:900,deviceScaleFactor:1,mobile:false});
    await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
    const measure = await evaluate(`(()=>{const box=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,w:r.width,h:r.height,right:r.right,bottom:r.bottom}};const dialog=document.querySelector('.cs-dialog');const close=dialog.querySelector('header [data-cs-dialog-close]');const field=document.querySelector('.cs-time-field');const input=field.querySelector('input');const unit=field.querySelector('span');const style=getComputedStyle(input);const canvas=document.createElement('canvas');const ctx=canvas.getContext('2d');ctx.font=style.font;return {dialog:box(dialog),close:box(close),svg:box(close.querySelector('svg')),avatar:box(dialog.querySelector('.cs-champion-cell .cs-dialog-avatar')),chosen:box(dialog.querySelector('.cs-chosen-row .cs-dialog-avatar')),field:box(field),input:box(input),unitBox:box(unit),textWidth:ctx.measureText(input.value).width,paddingLeft:parseFloat(style.paddingLeft),paddingRight:parseFloat(style.paddingRight),align:style.textAlign,documentOverflow:document.documentElement.scrollWidth>innerWidth,dialogOverflow:dialog.scrollWidth>dialog.clientWidth+1,unit:unit.textContent,inputEditable:!input.readOnly};})()`);
    assert.equal(measure.documentOverflow,false,JSON.stringify(measure));
    assert.equal(measure.dialogOverflow,false,JSON.stringify(measure));
    assert.ok(measure.avatar.w>=47 && measure.chosen.w>=35,JSON.stringify(measure));
    assert.ok(measure.close.h>=28 && measure.close.w>=28 && measure.svg.h>=14,JSON.stringify(measure));
    assert.ok(Math.abs(measure.field.x+measure.field.w/2-measure.input.x-measure.input.w/2)<1,JSON.stringify(measure));
    assert.equal(measure.paddingLeft,measure.paddingRight,'symmetric padding keeps number centered');
    assert.ok(measure.unitBox.right<=measure.input.right && measure.unitBox.x>measure.input.x+measure.input.w/2+measure.textWidth/2,JSON.stringify(measure));
    assert.equal(measure.align,'center');assert.equal(measure.unit,'s');assert.equal(measure.inputEditable,true);
    assert.ok(measure.dialog.bottom<=901 && measure.dialog.y>=0,JSON.stringify(measure));
    const shot=await call('Page.captureScreenshot',{format:'png'});
    fs.writeFileSync(path.join(output,`champselect-${width}.png`),Buffer.from(shot.data,'base64'));
    results.push({width,...measure});
  }
  await evaluate(`document.querySelector('.cs-dialog header [data-cs-dialog-close]').click()`);
  await waitFor(`!document.querySelector('.cs-dialog')`);
  fs.writeFileSync(path.join(output,'champselect-layout.json'),JSON.stringify({before,after,results},null,2));
  console.log('1505 champselect PASS: named records, real mouse swap, centered editable seconds + fixed unit, close icon, 3 viewport layouts');
};
