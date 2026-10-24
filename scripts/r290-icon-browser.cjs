'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {withBrowser}=require('./r280-browser-fixture.cjs');
// Sample the rendered stroke, including dashing/caps/vector effects, rather than
// asserting only that the JS wrote 58 to the SVG attribute. SVG 2 types.html.
function borderGeometry(){
 const button=document.querySelector('#update-button'),progress=button.querySelector('.update-border-progress');
 const svg=progress.ownerSVGElement,b=button.getBoundingClientRect(),s=svg.getBoundingClientRect(),css=getComputedStyle(button),p=getComputedStyle(progress),box=progress.getBBox();
 const perimeter=progress.getTotalLength(),samples=4000;
 let painted=0;for(let i=0;i<samples;i++)painted+=progress.isPointInStroke(progress.getPointAtLength(perimeter*(i+.5)/samples))?1:0;
 return {paintedFraction:painted/samples,perimeter,offset:Number(progress.getAttribute('stroke-dashoffset')),length:progress.getAttribute('pathLength'),radius:parseFloat(css.borderTopLeftRadius),rx:parseFloat(p.rx),stroke:parseFloat(p.strokeWidth),border:parseFloat(getComputedStyle(button.querySelector('.update-border-track')).strokeWidth),sameOutline:['x','y','width','height'].every(k=>box[k]===button.querySelector('.update-border-track').getBBox()[k]),box:{x:box.x,y:box.y,width:box.width,height:box.height},svg:{x:s.x-b.x,y:s.y-b.y,width:s.width,height:s.height},button:{width:b.width,height:b.height},scale:b.width/button.offsetWidth};
}
function badgeGeometry(){
 const read=id=>{const button=document.querySelector(id),icon=button.querySelector('.control-icon').getBoundingClientRect(),b=button.getBoundingClientRect(),pseudo=id==='#announcements-button',dot=getComputedStyle(pseudo?button:button.querySelector('.dot'),pseudo?'::after':null),scale=b.width/button.offsetWidth;
  const width=parseFloat(dot.width)*scale,height=parseFloat(dot.height)*scale,x=b.right-parseFloat(getComputedStyle(button).borderRightWidth)*scale-parseFloat(dot.right)*scale-width,y=b.top+parseFloat(getComputedStyle(button).borderTopWidth)*scale+parseFloat(dot.top)*scale;
  return {visible:dot.display!=='none'&&(pseudo||!button.querySelector('.dot').hidden),width:width/scale,height:height/scale,x,y,right:x+width,bottom:y+height,icon:{x:icon.x,y:icon.y,right:icon.right,bottom:icon.bottom},button:{x:b.x,y:b.y,right:b.right,bottom:b.bottom},scale,color:getComputedStyle(button).color,background:getComputedStyle(button).backgroundColor,hover:button.matches(':hover'),focus:button.matches(':focus')};};
 return {bell:read('#announcements-button'),update:read('#update-button')};
}
async function staticState(p,state,percent=42){
 await p.evaluate(`__r278App.renderUpdateStatus({supported:true,current:'0.12.93',latest:'0.12.94',state:'${state}',progress:{receivedBytes:${percent},totalBytes:100}});document.activeElement?.blur();`);
 await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:0,y:899});
 await p.evaluate(`(async()=>{for(const id of ['announcements-button','update-button']){const b=document.getElementById(id);getComputedStyle(b).color;for(const a of b.getAnimations({subtree:true}))a.finish();}await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()`);
}
withBrowser(async({page,results,errors,out})=>{
 for(const theme of ['dark','light']){
  const p=await page(theme);
  for(const width of [1440,900,760,640])for(const zoom of [1,1.4,2]){
   await p.call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1.75,mobile:false});
   await p.evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}';__r278App.applySidebar();__r278App.renderUpdateStatus({supported:true,current:'0.12.93',latest:'0.12.94',state:'available'});document.querySelector('.topbar').dispatchEvent(new Event('topbar-layout'))`);
   await p.until("document.querySelector('.topbar').dataset.topbarSettled==='true'");
   const geometry=await p.evaluate(`(()=>{const b=document.querySelector('#update-button'),s=b.querySelector('svg'),bell=document.querySelector('#announcements-button svg'),r=n=>{const x=n.getBoundingClientRect();return {width:x.width,height:x.height}},a=s.querySelector('path').getBBox();return {compact:document.querySelector('.topbar').dataset.topbarCompact,computed:getComputedStyle(s).width,variable:getComputedStyle(s).getPropertyValue('--topbar-icon-size'),update:r(s),bell:r(bell),button:r(b),ratio:a.height/bell.querySelector('path').getBBox().height,stroke:getComputedStyle(s.querySelector('path')).strokeWidth,bellStroke:getComputedStyle(bell.querySelector('path')).strokeWidth,vector:getComputedStyle(s.querySelector('path')).vectorEffect}})()`);
   for(const dimension of ['width','height'])assert(Math.abs(geometry.update[dimension]-geometry.bell[dimension])<=.02,'update and bell rendering size');
   assert(Math.abs(parseFloat(geometry.computed)-parseFloat(geometry.variable))<=.02,'shared icon size');
   assert(geometry.ratio>=.55);assert.equal(geometry.stroke,geometry.bellStroke);assert.equal(geometry.stroke,'1.8px');assert.equal(geometry.vector,'non-scaling-stroke');
   assert(geometry.update.width<geometry.button.width&&geometry.update.height<geometry.button.height);
   await staticState(p,'downloading');
   const border=await p.evaluate(`(${borderGeometry})()`);
   results.push({theme,width,zoom,geometry,border});
   assert(Math.abs(border.paintedFraction-.42)<=.02,`42% painted perimeter ±2%: ${JSON.stringify(results.at(-1))}`);
   assert.equal(border.length,'100');assert(Math.abs(border.offset-58)<1e-8);
   assert(Math.abs(border.rx-(border.radius-border.border/2))<.02,'border and progress share radius');
   assert.equal(border.stroke,border.border,'progress follows border thickness');assert(border.sameOutline,'progress and actual button outline share geometry');
   for(const axis of ['x','y'])assert(Math.abs(border.svg[axis])<.05,'SVG fits button border box');
   for(const dimension of ['width','height'])assert(Math.abs(border.svg[dimension]-border.button[dimension])<.05,'SVG fits button border box');
   for(const value of ['x','y'])assert(Math.abs(border.box[value]-border.border/2)<.02,'stroke stays inside outer border');
   assert(Math.abs((border.box.width+border.stroke)*border.scale-border.button.width)<.1);
   assert(Math.abs((border.box.height+border.stroke)*border.scale-border.button.height)<.1);
   const edges={};for(const percent of [0,100]){
    await staticState(p,'downloading',percent);
    const edge=await p.evaluate(`(${borderGeometry})()`);
    edges[percent]=edge.paintedFraction;assert(Math.abs(edge.paintedFraction-percent/100)<=.02,percent+'% painted perimeter');
   }
   const states={};
   for(const state of ['available','downloading','ready']){
    await staticState(p,state);
    const badges=await p.evaluate(`(${badgeGeometry})()`);states[state]=badges;
    for(const badge of Object.values(badges)){
     assert(!badge.hover&&!badge.focus,'neutral screenshot');
     if(!badge.visible)continue;
     assert(badge.width<=6.02&&badge.width>=3,'scaled capped badge '+JSON.stringify(badge));
     assert(badge.bottom<=badge.icon.y+.05||badge.x>=badge.icon.right-.05,'badge does not enter icon box');
     assert(Math.abs(badge.right-badge.button.right)<=badge.scale*.6&&Math.abs(badge.y-badge.button.y)<=badge.scale*.6,'badge sits at outer top-right');
    }
    assert.equal(badges.bell.background,badges.update.background,'same static button background');
    const clip=await p.evaluate(`(()=>{const a=document.querySelector('#announcements-button').getBoundingClientRect(),b=document.querySelector('#update-button').getBoundingClientRect(),previous=document.querySelector('#refresh').getBoundingClientRect(),gap=a.left-previous.right;return {x:a.left-Math.min(4,gap/2),y:Math.max(0,a.y-6),width:b.right-a.left+Math.min(4,gap/2)+4,height:Math.max(a.height,b.height)+12,scale:1}})()`);
    const shot=await p.call('Page.captureScreenshot',{format:'png',clip});fs.writeFileSync(path.join(out,`${theme}-${width}-${zoom}-${state}.png`),Buffer.from(shot.data,'base64'));
   }
   const primary=theme==='light'?states.available.update.color:await p.evaluate("getComputedStyle(document.querySelector('.update-border-progress')).stroke");
   assert.equal(states.ready.update.color,primary,'ready glyph has the intended theme color after transition');
   results.at(-1).states=states;results.at(-1).edges=edges;
  }
 }
 assert(results.some(x=>x.geometry.compact==='2'));assert(results.some(x=>x.geometry.compact==='3'));assert.deepEqual(errors,[]);
}).catch(e=>{console.error(e);process.exitCode=1});
