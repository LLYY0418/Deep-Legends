'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const sourcePath=path.join(os.tmpdir(),'r311-gameplay-'+process.pid+'.js');
const gameplay=fs.readFileSync(path.join(__dirname,'../backend/web/gameplay.js'),'utf8');
fs.writeFileSync(sourcePath,gameplay.replace('  bindSettings();','  window.__r311Live={renderLiveAugmentRecommendations,renderBuildRecommendation,state};\n  bindSettings();'));
process.env.R285_GAMEPLAY_SOURCE=sourcePath;
const {withBrowser}=require('./r280-browser-fixture.cjs');
const syntheticRows=Array.from({length:18},(_,i)=>({rarity:['silver','gold','prismatic'][i%3],grade:i<3?'OP':'C',score:100-i,games:100+i,winRate:55,pickRate:25,assets:[{id:i+1,kind:'arena-augment',name:`推荐${i+1}`,source:'communitydragon',path:'/latest/game/arena.png'}]}));
withBrowser(async({page,results,errors,out})=>{
 for(const theme of ['light','dark']){
  const p=await page(theme);
  await p.until('window.__r311Live && window.__r278Game.activeTab()?.data');
  await p.evaluate(`(async()=>{for(const name of ['champions.css','gameplay.css']){const link=document.createElement('link');link.rel='stylesheet';link.href='/'+name;await new Promise((r,j)=>{link.onload=r;link.onerror=j;document.head.append(link)})}const source=await(await fetch('/champions.js')).text();window.eval(source.replace(/\\n\\}\\)\\(\\);\\s*$/,'\\nwindow.__r311Hero={state,render,resetArenaControls,renderArenaDetailPane};\\n})();'));window.__r311Rows=${JSON.stringify(syntheticRows)};const h=__r311Hero;const row={championId:32,champion:'amumu',key:'amumu',name:'阿木木',grade:'S',rank:3,winRate:54.6,averagePlacement:3.3,firstPlaceRate:20.9,play:12000};Object.assign(h.state,{mode:'arena',section:'champions',selected:row,rankings:{mode:'arena',patch:'16.21',source:'YOUR.GG',rows:[row]},catalog:{champions:[]},detail:{mode:'arena',patch:'16.21',arenaAugmentGroups:['silver','gold','prismatic'].map(rarity=>({rarity,rows:__r311Rows.filter(r=>r.rarity===rarity)})),build:{coreItems:__r311Rows.map(r=>({...r,assets:r.assets.map(a=>({...a,kind:'item'}))})),prismItems:[],boots:[]}}});document.querySelectorAll('.section-panel').forEach(e=>e.hidden=e.id!=='champions-panel');document.getElementById('champions-panel').hidden=false;const root=document.getElementById('champions-root');root.style.cssText='position:absolute;top:20px;left:20px;width:1260px;z-index:9999;background:var(--bg);padding:12px';h.render();})()`);
  await p.until('document.querySelectorAll("[data-arena-sort]").length===2');
  const check=await p.evaluate(`(()=>{const d=document,s=[...d.querySelectorAll('[data-arena-sort]')],names=()=>[...d.querySelectorAll('.arena-augment-section .arena-option-main > strong')].map(e=>e.textContent);const first=names();d.querySelector('[data-arena-expand="augments"]').click();d.querySelector('[data-arena-sort="tier"]').click();const expanded=__r311Hero.state.arenaExpanded.augments,tiers=names();d.querySelector('[data-arena-sort="games"]').click();d.querySelector('[data-arena-expand="augments"]').click();const rgb=color=>{const c=d.createElement('canvas');c.width=c.height=1;const x=c.getContext('2d');x.fillStyle=color;x.fillRect(0,0,1,1);return [...x.getImageData(0,0,1,1).data].slice(0,3)};const lum=cs=>cs.map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((sum,v,i)=>sum+v*[.2126,.7152,.0722][i],0);const samples=[...d.querySelectorAll('.arena-sort-chips button.is-active,.arena-rarity-chips button.is-active,.arena-first-tabs button.is-active')].map(b=>{const st=getComputedStyle(b),a=lum(rgb(st.color)),z=lum(rgb(st.backgroundColor));return {className:b.parentElement.className,color:st.color,background:st.backgroundColor,contrast:(Math.max(a,z)+.05)/(Math.min(a,z)+.05),text:b.textContent,outline:st.outlineWidth,shadow:st.boxShadow}});d.querySelector('[data-arena-sort="games"]').focus();return {labels:s.map(b=>b.textContent),first,tiers,expanded,samples}})()`);
  assert.deepEqual(check.labels,['按场次','按梯度']);assert.deepEqual(check.first,[18,17,16,15,14,13,12,11,10].map(i=>'推荐'+i));assert.equal(check.expanded,true);assert.deepEqual(check.tiers.slice(0,3),['推荐1','推荐2','推荐3']);
  for(const sample of check.samples){if(!sample.className.includes('first-tabs'))assert.ok(sample.contrast>=4.5,JSON.stringify({theme,sample}));}
  for(const selector of ['.arena-sort-chips','.arena-rarity-chips']){
   const before=check.samples.find(s=>s.className.includes(selector.slice(1)));
   const point=await p.evaluate(`(()=>{const r=document.querySelector('${selector} button.is-active').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`);
   await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',...point});
   const hovered=await p.evaluate(`(()=>{const s=getComputedStyle(document.querySelector('${selector} button.is-active'));return {color:s.color,background:s.backgroundColor,shadow:s.boxShadow}})()`);
   for(const key of ['color','background','shadow'])assert.equal(hovered[key],before[key],theme+' selected hover '+selector);
  }
  const focus=await p.evaluate(`(()=>{const button=document.querySelector('[data-arena-sort="games"]');button.focus();const s=getComputedStyle(button);const first=document.querySelector('.arena-first-tabs button.is-active');const reference=document.createElement('button');reference.style.cssText='color:var(--ink);background:var(--surface-strong);box-shadow:0 0 0 1px var(--line);font-weight:750';document.body.append(reference);const neutral=getComputedStyle(reference),actual=getComputedStyle(first);const result={active:document.activeElement===button,visible:button.matches(':focus-visible'),outline:s.outlineStyle,width:parseFloat(s.outlineWidth),firstUnchanged:['color','backgroundColor','boxShadow','fontWeight'].every(k=>actual[k]===neutral[k])};reference.remove();return result})()`);
  assert.equal(focus.active,true);assert.equal(focus.visible,true);assert.ok(focus.width>=2);assert.notEqual(focus.outline,'none');assert.equal(focus.firstUnchanged,true);
  results.push({theme,hero:check,focus});await p.shot('r311-hero-games');
  for(const width of [760,560]){
   await p.call('Emulation.setDeviceMetricsOverride',{width:width+40,height:1100,deviceScaleFactor:1.75,mobile:false});
   const geometry=await p.evaluate(`(async()=>{document.getElementById('champions-root').style.width='${width}px';await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));const pane=document.querySelector('.arena-detail-pane'),grid=document.querySelector('.arena-augment-section .arena-option-grid');return {pane:pane.clientWidth,columns:getComputedStyle(grid).gridTemplateColumns.split(' ').length,chips:[...document.querySelectorAll('.arena-sort-chips,.arena-rarity-chips')].map(el=>({width:el.clientWidth,scroll:el.scrollWidth,buttons:[...el.children].map(b=>({width:b.clientWidth,scroll:b.scrollWidth}))}))}})()`);
   assert.equal(geometry.columns,geometry.pane<=540?1:geometry.pane<=820?2:3);for(const g of geometry.chips){assert.ok(g.scroll<=g.width+1,JSON.stringify({theme,width,geometry}));for(const b of g.buttons)assert.ok(b.scroll<=b.width+1)}results.push({theme,width,heroGeometry:geometry});
  }
  await p.call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1.75,mobile:false});
  await p.evaluate(`document.getElementById('champions-root').hidden=true;const s=document.createElement('main');s.id='r311-stage';s.style.cssText='position:absolute;top:20px;left:20px;z-index:99999;padding:0;background:var(--bg);container:recommendation-area / inline-size';document.body.append(s)`);
  for(const width of [1200,760,560]){
   const measurement=await p.evaluate(`(async()=>{const s=document.getElementById('r311-stage');s.style.width='${width}px';s.innerHTML=__r311Live.renderLiveAugmentRecommendations({recommendations:{augments:__r311Rows}},'arena');await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return [...s.querySelectorAll('.live-augment-column')].map(el=>({width:el.getBoundingClientRect().width,cards:[...el.querySelectorAll('.live-augment-option')].map(c=>({name:c.querySelector('strong').textContent,top:c.getBoundingClientRect().top,left:c.getBoundingClientRect().left,height:c.getBoundingClientRect().height,overflow:c.scrollWidth>c.clientWidth+1,stats:c.querySelector('small').textContent})),columns:getComputedStyle(el.querySelector(':scope > div')).gridTemplateColumns}))})()`);
   assert.equal(measurement.length,3);
   for(const [i,col]of measurement.entries()){
    assert.equal(col.cards.length,4);assert.deepEqual(col.cards.map(c=>c.name),[16+i,13+i,10+i,7+i].map(j=>'推荐'+j));
    assert.equal(new Set(col.cards.map(c=>Math.round(c.top))).size,width>900?1:width>620?2:4);
    for(const c of col.cards){assert.ok(!c.overflow);assert.match(c.stats,/胜率.*场次/s);assert.doesNotMatch(c.stats,/选用率/);assert.ok(c.height>0);}
   }
   results.push({theme,width,live:measurement});if(theme==='dark'){
    const clip=await p.evaluate(`(()=>{const r=document.getElementById('r311-stage').getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,scale:1}})()`);
    const shot=await p.call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip});
    fs.writeFileSync(path.join(out,theme+'-r311-live-'+width+'.png'),Buffer.from(shot.data,'base64'));
   }
  }
 }
 assert.deepEqual(errors,[]);
}).catch(e=>{console.error(e);process.exitCode=1}).finally(()=>fs.rmSync(sourcePath,{force:true}));
