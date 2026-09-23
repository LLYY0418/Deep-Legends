'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// R141_LAYOUT=1 R141_BANNER_SAMPLE=/private/tmp/r140-lny-banner.png
// CURRENT_GAME_SHOTS=docs/r141-validation node desktop/current-game-layout.cjs
// R140_LAYOUT=1 also checks the current banner detail dialog at multiple viewport heights.
exports.verify = async ({call,evaluate,output}) => {
  assert.ok(process.env.R141_BANNER_SAMPLE||process.env.R140_BANNER_SAMPLE,'banner sample must point to a real game image');
  await evaluate(`(() => {
    const original=window.fetch;
    window.fetch=async(...args)=>{
      const response=await original(...args);
      if(String(args[0]).split('?')[0]!=='/api/facade/banners')return response;
      const data=await response.json();
      data.banners=data.banners.map((item)=>({...item,imagePath:'/lol-game-data/assets/ASSETS/Regalia/BannerSkins/lny2023.png'}));
      return new Response(JSON.stringify(data),{status:response.status,headers:{'Content-Type':'application/json'}});
    };
    const scale=document.getElementById('setting-ui-scale');scale.value='1';scale.dispatchEvent(new Event('change',{bubbles:true}));
    document.getElementById('section-favorites').click();
    document.getElementById('favorites-tab-facade').click();
    document.getElementById('facade-view-banners').click();
  })()`);
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const grid=document.getElementById('facade-grid');const image=grid?.querySelector('.skin-card img');if(grid?.classList.contains('is-banners')&&image?.naturalWidth){clearInterval(timer);resolve(true)}},30);setTimeout(()=>{clearInterval(timer);reject(Error('real banner did not load'))},12000)})`);
  // .skin-art img 有 200ms opacity 过渡，等稳定后再断言未拥有态。
  await evaluate(`new Promise(resolve=>setTimeout(resolve,250))`);
  const gridMeasure=()=>evaluate(`(() => {
    const grid=document.getElementById('facade-grid'), cards=[...grid.querySelectorAll('.skin-card')];
    const rects=cards.map(card=>card.getBoundingClientRect()),first=rects[0];
    const firstRow=rects.filter(rect=>Math.abs(rect.top-first.top)<1);
    const img=cards[0].querySelector('img'),locked=cards.find(card=>card.classList.contains('is-locked'));
    const lock=locked?.querySelector('.skin-lock');
    const css=getComputedStyle(grid), cardCSS=getComputedStyle(cards[0]);
    return {gridWidth:grid.getBoundingClientRect().width,columns:firstRow.length,cardWidth:first.width,cardHeight:first.height,
      gap:css.gap,intrinsic:cardCSS.containIntrinsicSize,ratio:css.getPropertyValue('--facade-banner-ratio').trim(),
      naturalWidth:img.naturalWidth,naturalHeight:img.naturalHeight,objectFit:getComputedStyle(img).objectFit,
      artWidth:cards[0].querySelector('.skin-art').getBoundingClientRect().width,
      artHeight:cards[0].querySelector('.skin-art').getBoundingClientRect().height,
      lockedOpacity:getComputedStyle(locked.querySelector('img')).opacity,
      lockedFilter:getComputedStyle(locked.querySelector('img')).filter,
      lockDisplay:getComputedStyle(lock).display,lockWidth:lock.getBoundingClientRect().width};
  })()`);
  const shot=async name=>{
    const data=await call('Page.captureScreenshot',{format:'png'});
    fs.writeFileSync(path.join(output,name),Buffer.from(data.data,'base64'));
  };
  const dialogMeasure=()=>evaluate(`(() => {
    const d=document.getElementById('facade-detail-dialog'),art=d.querySelector('.dialog-art'),copy=d.querySelector('.dialog-copy'),note=document.getElementById('facade-detail-note');
    return {viewportHeight:innerHeight,dialogHeight:d.getBoundingClientRect().height,clientHeight:d.clientHeight,scrollHeight:d.scrollHeight,
      overflowY:getComputedStyle(d).overflowY,artHeight:art.getBoundingClientRect().height,copyHeight:copy.getBoundingClientRect().height,
      noteBottom:note.getBoundingClientRect().bottom,dialogBottom:d.getBoundingClientRect().bottom};
  })()`);
  for(const width of [1200,960]) {
    await call('Emulation.setDeviceMetricsOverride',{width,height:950,deviceScaleFactor:1,mobile:false});
    await evaluate(`document.getElementById('facade-grid').scrollIntoView({block:'start',behavior:'instant'});new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
    if(process.env.R141_PROBE==='1'||process.env.R140_PROBE==='1') {
      const samples=[];
      for(const size of [152,184,190,192,196,200,204,208,212,216]) {
        await evaluate(`{const g=document.getElementById('facade-grid');g.style.gridTemplateColumns='repeat(auto-fill,${size}px)';g.style.gap='20px 16px'}`);
        samples.push({size,...await gridMeasure()});
      }
      console.log('banner probe',width,JSON.stringify(samples));
      continue;
    }
    await evaluate(`{const g=document.getElementById('facade-grid');g.style.gridTemplateColumns='repeat(auto-fill,150px)';g.style.gap='12px 10px'}`);
    const r138=await gridMeasure();
    await shot('r138-grid-'+width+'.png');
    await evaluate(`{const g=document.getElementById('facade-grid');g.style.gridTemplateColumns='repeat(auto-fill,152px)';g.style.gap='20px 16px'}`);
    const r140=await gridMeasure();
    await shot('r140-grid-'+width+'.png');
    await evaluate(`{const g=document.getElementById('facade-grid');g.style.removeProperty('grid-template-columns');g.style.removeProperty('gap')}`);
    const after=await gridMeasure();
    await shot('r141-grid-'+width+'.png');
    console.log('R141 grid',width,JSON.stringify({r138,r140,after}));
    assert.ok(after.columns>=3&&after.columns<=4,`${width}: expected 3–4 columns, got ${after.columns}`);
    assert.ok(after.columns<r140.columns,`${width}: reverting to the R140 width must change the column count`);
    assert.equal(r138.gap,'12px 10px');assert.equal(r140.gap,'20px 16px');assert.equal(after.gap,'20px 16px');
    assert.equal(r140.cardWidth,152);
    assert.ok(after.cardWidth>=190,`${width}: banner tile must be visibly wider`);
    assert.equal(after.cardWidth,200,`${width}: measured 200px width must remain stable`);
    assert.equal(after.intrinsic,'auto 531px');
    assert.ok(after.naturalWidth>0&&after.naturalHeight>0&&after.naturalWidth<after.naturalHeight,'fixture is a decoded vertical banner');
    assert.equal(after.objectFit,'contain');
    assert.ok(Math.abs(after.artWidth/after.artHeight-after.naturalWidth/after.naturalHeight)<.002,'banner art ratio must match source');
    assert.equal(after.lockDisplay,'grid');assert.equal(after.lockWidth,30);
    assert.equal(after.lockedOpacity,'0.38');assert.match(after.lockedFilter,/grayscale\(1\)/);
    assert.ok(after.cardHeight>r140.cardHeight*1.2,`${width}: banner card did not visibly grow`);
    if(process.env.R141_LAYOUT==='1') continue;
    const card=await evaluate(`(() => {const card=document.querySelector('#facade-grid .skin-card');card.click();return true})()`);
    assert.ok(card);
    await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const d=document.getElementById('facade-detail-dialog');if(d.open&&d.style.getPropertyValue('--facade-art-height')==='640px'){clearInterval(timer);resolve(true)}},30);setTimeout(()=>{clearInterval(timer);reject(Error('banner detail did not reach 640px'))},8000)})`);
    const detail=await evaluate(`(() => {const d=document.getElementById('facade-detail-dialog'),img=document.getElementById('facade-detail-image');return {naturalWidth:img.naturalWidth,naturalHeight:img.naturalHeight,artWidth:d.style.getPropertyValue('--facade-art-width'),artHeight:d.style.getPropertyValue('--facade-art-height'),fit:getComputedStyle(img).objectFit}})()`);
    assert.equal(detail.artHeight,'640px');assert.equal(detail.fit,'contain');
    await evaluate(`{const d=document.getElementById('facade-detail-dialog');d.style.setProperty('--facade-art-width','125px');d.style.setProperty('--facade-art-height','320px')}`);
    await shot('before-detail-'+width+'.png');
    await evaluate(`{const d=document.getElementById('facade-detail-dialog');d.style.setProperty('--facade-art-width','${detail.artWidth}');d.style.setProperty('--facade-art-height','${detail.artHeight}')}`);
    await shot('after-detail-'+width+'.png');
    for(const height of [950,700,550]) {
      await call('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile:false});
      await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
      const measured=await dialogMeasure();
      console.log('R140 dialog',width,height,JSON.stringify(measured));
      assert.equal(measured.overflowY,'hidden',`${width}×${height}: detail dialog must not show a scrollbar`);
      assert.ok(measured.scrollHeight<=measured.clientHeight+1,`${width}×${height}: dialog content is clipped`);
      assert.ok(measured.noteBottom<=measured.dialogBottom+1,`${width}×${height}: detail note is clipped`);
      assert.ok(measured.artHeight>0,`${width}×${height}: banner art collapsed`);
      if(height===950) assert.equal(measured.artHeight,640,`${width}×${height}: full 2× banner height should fit`);
      if(height<950) await shot(`after-detail-${width}-h${height}.png`);
    }
    if(width===960) for(const zoom of [1.25,1.5]) {
      await call('Emulation.setDeviceMetricsOverride',{width,height:700,deviceScaleFactor:1,mobile:false});
      await evaluate(`{const select=document.getElementById('setting-ui-scale');select.value='${zoom}';select.dispatchEvent(new Event('change',{bubbles:true}))}`);
      await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
      const measured=await dialogMeasure();
      console.log('R140 dialog zoom',width,700,zoom,JSON.stringify(measured));
      assert.equal(measured.overflowY,'hidden');
      assert.ok(measured.scrollHeight<=measured.clientHeight+1,`960×700 at ${zoom}×: dialog content is clipped`);
      assert.ok(measured.noteBottom<=measured.dialogBottom+1,`960×700 at ${zoom}×: detail note is clipped`);
      assert.ok(measured.artHeight>0,`960×700 at ${zoom}×: banner art collapsed`);
    }
    await evaluate(`document.getElementById('facade-detail-dialog').close()`);
    console.log('R140 detail',width,JSON.stringify(detail));
  }
  if(process.env.R141_PROBE==='1'||process.env.R140_PROBE==='1')return;
  console.log(process.env.R141_LAYOUT==='1'?'R141 Chromium PASS':'banner Chromium PASS');
};
