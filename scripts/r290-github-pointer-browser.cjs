'use strict';
const assert=require('node:assert/strict');
const {withBrowser}=require('./r280-browser-fixture.cjs');
// Exercise real mouse movement, rather than only checking computed cursor CSS.
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light'])for(const zoom of [0.9,1,1.4]){
  const p=await page(theme);
  await p.call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});
  await p.evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');__r278App.activateSection('settings')`);
  await p.until('window.deepLegendsSettingsBackup?.createBackup');
  await p.evaluate(`window.__githubPointerEvents=[];window.__githubOpens=[];window.open=(...args)=>__githubOpens.push(args);const b=document.getElementById('settings-github');for(const type of ['pointerover','pointerout','click'])b.addEventListener(type,e=>__githubPointerEvents.push({type,target:e.target.id||e.target.tagName}));`);
  const box=()=>p.evaluate(`(()=>{const r=document.getElementById('settings-github').getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}})()`);
  let r=await box();
  await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:r.x+r.width/2,y:r.y+r.height/2});
  r=await box();
  const samples=[];
  for(const fraction of [.02,.15,.3,.5,.7,.85,.98])for(const vertical of [.2,.5,.8]){
   const point={x:r.x+r.width*fraction,y:r.y+r.height*vertical};
   await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',...point});
   const frames=await p.evaluate(`new Promise(resolve=>{const frames=[];function sample(){const b=document.getElementById('settings-github'),n=document.elementFromPoint(${point.x},${point.y}),r=b.getBoundingClientRect();frames.push({target:n?.id||n?.tagName,cursor:n&&getComputedStyle(n).cursor,hover:b.matches(':hover'),x:r.x,width:r.width});if(frames.length<3)requestAnimationFrame(sample);else resolve(frames)}requestAnimationFrame(sample)})`);
   samples.push(...frames);
  }
  results.push({theme,zoom,samples,events:await p.evaluate('__githubPointerEvents')});
  assert(samples.every(s=>s.target==='settings-github'),'icons, text, divider and padding must share the anchor hit target');
  assert(samples.every(s=>s.cursor==='pointer'&&s.hover),'moving inside the capsule keeps pointer and hover');
  assert(samples.every(s=>Math.abs(s.x-r.x)<.1&&Math.abs(s.width-r.width)<.1),'hover layout stays stable while crossing children');
  assert.equal((await p.evaluate("__githubPointerEvents.filter(e=>e.type==='pointerout').length")),0,'internal movement must not emit pointerout');
  // Click the icon area, main copy, divider and stars through the same link.
  for(const fraction of [.05,.4,.75,.95]){
   const point={x:r.x+r.width*fraction,y:r.y+r.height/2};
   await p.call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...point});
   await p.call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...point});
  }
  const clicks=await p.evaluate('__githubOpens');assert.equal(clicks.length,4);assert(clicks.every(args=>args[0]==='https://github.com/LLYY0418/Deep-Legends'));
  await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:10,y:200});
  assert.equal(await p.evaluate("document.getElementById('settings-github').matches(':hover')"),false);
 }
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
