'use strict';
const assert=require('node:assert/strict');
const {withBrowser}=require('./r280-browser-fixture.cjs');

// Real Chromium modal/focus/paint checks with synthetic API data, not live LCU evidence.
withBrowser(async({page,results})=>{
 for(const theme of ['dark','light']){
  const p=await page(theme);
  await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
  await p.evaluate(`(()=>{const g=__r278Game,t=g.activeTab();t.data={...t.data,capabilities:[...t.data.capabilities.filter(c=>c.name!=='champion-mastery'),{name:'champion-mastery',state:'available'}],seasonStatsProgress:{complete:true,tableSupported:true},seasonOverall:{...t.data.overall,games:30}};window.__r283SubpageRequests=[];const fetch=window.fetch;window.fetch=(url,options)=>{if(['/api/gameplay/champion-table?','/api/gameplay/masteries?'].some(prefix=>String(url).startsWith(prefix))){__r283SubpageRequests.push({url:String(url),open:document.getElementById('career-dialog').open});return Promise.resolve(new Response(JSON.stringify({available:true,rows:[],items:[],overall:{},queue:'420'})))}return fetch(url,options)}})()`);
  for(const layout of ['single','masonry'])for(const region of ['cn','kr'])for(const subpage of ['champion-table','masteries']){
   const ready=await p.evaluate(`(()=>{const g=__r278Game,t=g.activeTab(),d=document.getElementById('career-dialog');t.region=${JSON.stringify(region)};t.current=${region==='cn'};t.data.player={...t.data.player,region:${JSON.stringify(region)},isCurrent:${region==='cn'},playerRef:${JSON.stringify('p18-fixture-'+region)}};g.state.activeGroup=${JSON.stringify(region==='kr'?'kr':'players')};g.state.activeTabs[g.state.activeGroup]=t.key;t.overviewSubpage='';g.renderOverview(g.state.activeGroup);document.getElementById('app-scroll').scrollTop=315;g.renderCareerDialogContent(t.data,t,${JSON.stringify(layout)},true);d.style.width=${JSON.stringify(layout==='single'?'500px':'960px')};d.showModal();return !!d.querySelector('[data-overview-subpage="${subpage}"]')})()`);
   assert(ready,`${theme}/${layout}/${region}/${subpage} arrow missing`);
   if(region==='cn'&&subpage==='champion-table'){
    await p.evaluate(`Promise.all(document.getElementById('career-dialog').getAnimations().map(a=>a.finished)).then(()=>document.dispatchEvent(new CustomEvent('deep-legends:tooltip-hide')))`);
    await p.shot('p18-dialog-'+layout);
   }
   await p.evaluate(`document.querySelector('#career-dialog [data-overview-subpage="${subpage}"]').click()`);
   await p.until('__r278Game.activeTab().overviewSubpageState?.status==="ready"');
   const opened=await p.evaluate(`(()=>{const t=__r278Game.activeTab(),r=__r283SubpageRequests.at(-1);return {page:t.overviewSubpage,open:document.getElementById('career-dialog').open,requestOpen:r.open,url:r.url,ref:t.data.player.playerRef}})()`);
   assert.equal(opened.page,subpage);assert.equal(opened.open,false);assert.equal(opened.requestOpen,false);assert.equal(opened.ref,'p18-fixture-'+region);assert(opened.url.includes('playerRef='+encodeURIComponent(opened.ref)));
   const returned=await p.evaluate(`(()=>{document.querySelector('[data-overview-return]').click();return {page:__r278Game.activeTab().overviewSubpage,open:document.getElementById('career-dialog').open,scroll:document.getElementById('app-scroll').scrollTop}})()`);
   assert.equal(returned.page,'');assert.equal(returned.open,false);assert.equal(returned.scroll,315);
   results.push({theme,layout,region,subpage,opened,returned});
  }
  for(const zoom of [1,1.25,1.5,2]){
  await p.evaluate(`(()=>{document.documentElement.style.setProperty('--ui-zoom','${zoom}');const g=__r278Game,t=g.activeTab(),d=document.getElementById('career-dialog');g.renderCareerDialogContent(t.data,t,'masonry',true);d.showModal();const anchor=d.querySelector('#career-dialog-content [data-overview-subpage="masteries"]');anchor.scrollIntoView({block:'center'});anchor.dispatchEvent(new MouseEvent('pointerover',{bubbles:true}))})()`);
  await p.until('document.getElementById("global-tooltip").dataset.shown==="true"');
  const tooltip=await p.evaluate(`(async()=>{const t=document.getElementById('global-tooltip');await Promise.all(t.getAnimations().map(a=>a.finished));await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));t.style.pointerEvents='auto';const box=t.getBoundingClientRect(),hit=document.elementFromPoint(box.x+box.width/2,box.y+box.height/2);return {aboveModal:hit===t||t.contains(hit),parent:t.parentElement.id,popover:t.matches(':popover-open'),hit:hit?.outerHTML,box:box.toJSON()}})()`);
  assert.equal(tooltip.parent,'career-dialog');assert(tooltip.aboveModal,JSON.stringify(tooltip));
  if(zoom===1)results.push({theme,zoom,tooltip});else{const base=results.find(r=>r.theme===theme&&r.tooltip&&r.zoom===1);assert(Math.abs(tooltip.box.width/base.tooltip.box.width-zoom)<0.06,'tooltip must scale once');results.push({theme,zoom,tooltip});}
  if(zoom===1)await p.shot('p18-dialog-tooltip');
  await p.evaluate('__r278Game.closeCareerDialog()');
  await p.until('document.getElementById("global-tooltip").hidden&&document.getElementById("global-tooltip").parentElement===document.body');
  }
 }
}).catch(error=>{console.error(error);process.exitCode=1});
