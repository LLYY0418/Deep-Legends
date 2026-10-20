'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {withBrowser}=require('./r280-browser-fixture.cjs');
const response=JSON.parse(fs.readFileSync(process.env.R293_PRO_RESPONSE,'utf8'));
const widths=(process.env.R293_WIDTHS||'1100,1000,999,860,640').split(',').map(Number);
const themes=(process.env.R293_THEMES||'dark,light').split(',');
// Geometry fixture: actual public games, with the supplied 18-step order and
// seven final inventory slots on the first row to exercise the widest case.
const rows=structuredClone(response.pros.filter(row=>row.title==='KC Kyeahoo'));
assert.equal(rows.length,2);
rows[0].build=structuredClone(response.pros.find(row=>row.build?.skillOrder?.length===18).build);
assert(rows[0].itemIds.length>=7);
// The demo catalog omits Sorcery; use the captured Resolve page for a complete
// three-column geometry fixture, matching the design's primary/sub/shards grid.
for(const row of rows)for(const key of ['primaryStyleId','subStyleId','selectedPerkIds','statModIds'])row[key]=structuredClone(response.pros[0][key]);

withBrowser(async({page,out,results,errors})=>{
 for(const theme of themes){
  const p=await page(theme);
  await p.evaluate(`(()=>{const base=window.fetch;window.fetch=async(input,init)=>{const url=new URL(typeof input==='string'?input:input.url,location.href);if(url.pathname==='/api/gameplay/pro-runes')return new Response(JSON.stringify({...${JSON.stringify(response)},pros:${JSON.stringify(rows)}}),{headers:{'Content-Type':'application/json'}});const reply=await base(input,init);if(url.pathname!=='/api/gameplay/live')return reply;const data=await reply.json(),self=data.players.find(p=>p.isCurrent);self.championId=103;self.championName='阿狸';self.position='mid';data.recommendations.resolvedPosition='mid';return new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}});};__r278App.activateSection('live');})()`);
  await p.until("!__r278Game.state.liveLoading && __r278Game.state.live?.players?.some(p=>p.isCurrent&&p.championId===103) && document.querySelector('[data-rune-source=pro]')");
  await p.evaluate("document.querySelector('[data-rune-source=pro]').click()");
  await p.until("[...document.querySelectorAll('[data-specialist-player]')].some(n=>n.textContent.includes('Kyeahoo'))");
  await p.evaluate("[...document.querySelectorAll('[data-specialist-player]')].find(n=>n.textContent.includes('Kyeahoo')).click()");
  await p.until("document.querySelectorAll('#live-content .specialist-game-row').length===2 && document.querySelector('#live-content .sg-skills')");
  // Keep the actual renderer DOM and production CSS, but isolate the two rows
  // from the app shell so each requested content width is measured exactly.
  await p.evaluate(`(()=>{const canvas=document.createElement('main');canvas.id='r293-canvas';canvas.style.cssText='margin:20px auto;background:var(--surface);border-radius:10px';canvas.append(document.querySelector('#live-content .rune-source-stack'));document.body.append(canvas);const style=document.createElement('style');style.textContent='body{display:block;overflow:visible;height:auto;margin:0}body>:not(#r293-canvas):not(#global-tooltip){display:none!important}#r293-canvas .specialist-player-games{padding:0}';document.head.append(style)})()`);
  for(const width of widths){
   const viewport=width>=999?Math.max(1180,width+40):width+40;
   await p.call('Emulation.setDeviceMetricsOverride',{width:viewport,height:2600,deviceScaleFactor:1,mobile:false});
   await p.evaluate(`document.getElementById('r293-canvas').style.width='${width+4}px'`);
   await p.until(`Math.abs(document.querySelector('#r293-canvas .specialist-game-runes').getBoundingClientRect().width-${width})<0.1`);
   const geometry=await p.evaluate(`(()=>{const box=n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,right:r.right,bottom:r.bottom,cx:r.x+r.width/2,cy:r.y+r.height/2}},visible=n=>getComputedStyle(n).display!=='none'&&n.getBoundingClientRect().width>0;return [...document.querySelectorAll('#r293-canvas .specialist-game-row')].map(row=>{const runes=row.querySelector('.specialist-game-runes'),footer=row.querySelector('.specialist-game-footer'),items=footer.querySelector('.sg-items'),skills=footer.querySelector('.sg-skills'),corner=row.querySelector('.rune-corner-spells'),spells=footer.querySelector('.sg-spells'),shown=[corner,spells].filter(visible),cells=[...row.querySelectorAll('.skill-order>span')],r=box(runes);return {row:r,hasSkills:runes.classList.contains('has-skills'),container:getComputedStyle(runes).containerName,locations:shown.map(n=>n.className),visibleIcons:shown.reduce((n,el)=>n+el.children.length,0),spellBoxes:shown.flatMap(n=>[...n.children].map(box)),equipment:box(items.querySelector('.sg-icons')),footer:box(footer),skills:skills?box(skills):null,summary:skills?.querySelector('.skill-priority-summary')?.textContent,items:items.querySelectorAll('.item-option-button').length,steps:cells.length,sequenceRows:new Set(cells.map(n=>Math.round(n.getBoundingClientRect().top))).size,overflow:runes.scrollWidth>runes.clientWidth+1||footer.scrollWidth>footer.clientWidth+1,outside:cells.some(n=>box(n).right>r.right+1||box(n).x<r.x-1),abilityButtons:row.querySelectorAll('.skill-icon-button,h3').length}})})()`);
   const headers=await p.evaluate(`(()=>{const box=n=>{const r=n.getBoundingClientRect();return {top:r.top,right:r.right,bottom:r.bottom,left:r.left}};return [...document.querySelectorAll('.specialist-game-head')].map(head=>({head:box(head),opponent:box(head.querySelector('.specialist-opponent')),font:getComputedStyle(head.querySelector('.specialist-game-meta')).fontSize,overflow:head.scrollWidth>head.clientWidth+1,text:head.textContent}))})()`);
   for(const h of headers){assert.equal(h.font,'12px');assert(!h.overflow);assert(h.opponent.right<=h.head.right+1&&h.opponent.bottom<=h.head.bottom+1);assert(!/英雄 \d+/.test(h.text));}
   results.push({theme,width,geometry,headers});
   for(const [i,g] of geometry.entries()){
    assert.deepEqual(g.locations,i===0&&width>=1000?['rune-corner-spells']:['sg-spells']);
    assert.equal(g.container,'sgrow');assert.equal(g.hasSkills,i===0);assert.equal(g.visibleIcons,2);assert.equal(g.spellBoxes.length,2);assert(Math.abs(g.spellBoxes[0].cy-g.spellBoxes[1].cy)<=1,'spell pair must be horizontal');assert.equal(g.spellBoxes[0].width,width<=700?34:38);assert.equal(g.items,7);assert(!g.overflow&&!g.outside,JSON.stringify(g));assert(g.spellBoxes.every(b=>b.x>=g.row.x-1&&b.right<=g.row.right+1),'spell icons stay inside their row');assert.equal(g.abilityButtons,0);
    if(i===0){assert.equal(g.summary,'主Q副W');assert.equal(g.steps,18);assert.equal(g.sequenceRows,1);if(width>=1000)assert(Math.abs(g.equipment.cy-g.skills.cy)<=1,'items and skills share a footer row');else assert(g.skills.y>g.equipment.bottom,'skills use the second footer row');}
    else assert.equal(g.skills,null);
    if(width<1000||i===1)assert(Math.abs(g.spellBoxes[0].cy-g.equipment.cy)<=1,'spells are centered on equipment');
   }
   const bounds=await p.evaluate("({width:innerWidth,overflow:document.documentElement.scrollWidth>innerWidth+1,height:document.getElementById('r293-canvas').getBoundingClientRect().bottom+20})");assert(!bounds.overflow);
   const screenshot=await p.call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width:bounds.width,height:Math.ceil(bounds.height),scale:1}});fs.writeFileSync(path.join(out,`${theme}-${width}.png`),Buffer.from(screenshot.data,'base64'));
   const tooltips=[];
   for(const selector of ['.rune-option-button[data-tooltip]','.rune-shards .rune-option-button[data-tooltip]','.sg-items [data-tooltip]',width>=1000?'.rune-corner-spells [data-tooltip]':'.sg-spells [data-tooltip]','.skill-order [data-tooltip]']){
    const anchor=await p.evaluate(`(()=>{const n=document.querySelector('#r293-canvas .specialist-game-row ${selector}');n.scrollIntoView({block:'center'});const r=n.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2,text:n.dataset.tooltip}})()`);
    await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:anchor.x,y:anchor.y});
    await p.until("document.getElementById('global-tooltip').dataset.shown==='true'");
    const tip=await p.evaluate("(()=>{const t=document.getElementById('global-tooltip'),r=t.getBoundingClientRect();return {host:t.parentElement.tagName,position:getComputedStyle(t).position,text:t.textContent,x:r.x,y:r.y,right:r.right,bottom:r.bottom,width:r.width,height:r.height,viewport:[innerWidth,innerHeight],popover:t.matches(':popover-open')}})()");
    assert.equal(tip.host,'BODY');assert.equal(tip.position,'fixed');assert(tip.width>0&&tip.height>0);assert(tip.x>=0&&tip.y>=0&&tip.right<=tip.viewport[0]+1&&tip.bottom<=tip.viewport[1]+1);assert(tip.bottom<=anchor.y||tip.y>=anchor.y||tip.right<=anchor.x||tip.x>=anchor.x,'tooltip stays beside its anchor');assert(tip.text.includes(anchor.text.split('\n')[0]));tooltips.push({selector,...tip});
    await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:1,y:1});await p.until("document.getElementById('global-tooltip').hidden");
   }
   results[results.length-1].tooltips=tooltips;
  }
 }
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
