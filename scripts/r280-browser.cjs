'use strict';
const {withBrowser}=require('./r280-browser-fixture.cjs');
const assert=require('node:assert/strict');
async function run(){await withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 for(const width of [1280,1583])for(const zoom of [1,1.4,2]){
  await p.call('Emulation.setDeviceMetricsOverride',{width,height:951,deviceScaleFactor:1.75,mobile:false});
  await p.evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}';new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
  const geometry=await p.evaluate(`(()=>{const rect=n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,right:r.right}},bar=document.querySelector('.topbar');return {bar:rect(bar),controls:[...bar.querySelectorAll('#player-search-region,.player-search-fields,#refresh,#friends-toggle,#announcements-button,#update-button')].filter(n=>n.getClientRects().length).map(n=>({id:n.id || n.className,...rect(n)})),icons:[...bar.querySelectorAll('.function-icon')].filter(n=>n.getClientRects().length).map(rect),subtitle:getComputedStyle(document.querySelector('.window-drag>span')).display,friendsLabel:getComputedStyle(document.querySelector('.friends-toggle-label')).display,count:document.querySelector('#friends-toggle').innerText,gap:innerWidth-138-Math.max(...[...bar.querySelectorAll('.topbar-actions>*')].filter(n=>n.getClientRects().length).map(n=>n.getBoundingClientRect().right))}})()`);
  for(const c of geometry.controls)assert(Math.abs(c.height-40)<=1,c.id+' height');for(const i of geometry.icons){assert(Math.abs(i.width-18)<=1);assert(Math.abs(i.height-18)<=1);}assert(Math.abs(geometry.gap-8)<=1);assert.equal(geometry.bar.height,56);if(zoom>1){assert.equal(geometry.subtitle,'none');assert.equal(geometry.friendsLabel,'none');assert(/\d/.test(geometry.count));}
  results.push({kind:'topbar',width,zoom,geometry});await p.shot(`topbar-${width}-${zoom}`);
 }
 // Forty real collapsed/expanded cards, with the true scrolling container.
 await p.call('Emulation.setDeviceMetricsOverride',{width:1583,height:951,deviceScaleFactor:1.75,mobile:false});
 for(const zoom of process.env.R280_MUTANT==='zoom-height'?[2]:[1,1.4,2]){
  await p.evaluate(`(()=>{const t=__r278Game.activeTab(),sample=__r280Overview.matches.find(m=>m.queueId===420);t.data={...__r280Overview,matches:Array.from({length:40},(_,i)=>({...sample,gameId:99000+i})),pagination:{hasMore:false}};t.loading=false;t.initialPagePending=false;t.openMatches=new Set();document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}';__r278Game.renderOverview('players');document.getElementById('app-scroll').scrollTop=0;return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>{window.__r280OriginalHeights=[...document.querySelectorAll('.match-entry')].map(n=>n.getBoundingClientRect().height);r()})));})()`);
  const action=async()=>{for(let i=1;i<=12;i++){await p.evaluate(`document.getElementById('app-scroll').scrollTop=document.getElementById('app-scroll').scrollHeight*${i}/12`);await new Promise(r=>setTimeout(r,70));}await new Promise(r=>setTimeout(r,350));};
  if(zoom===2)await p.record('zoom-2-scroll',action);else await action();
  const samples=await p.evaluate("new Promise(resolve=>{const samples=[],timer=setInterval(()=>{samples.push(document.getElementById('app-scroll').scrollTop);if(samples.length===60){clearInterval(timer);resolve(samples)}},50)})");assert.equal(new Set(samples).size,1,'scroll jitter zoom='+zoom);
  const heights=await p.evaluate("[...document.querySelectorAll('.match-entry')].slice(0,5).map((n,i)=>({before:__r280OriginalHeights[i],after:n.getBoundingClientRect().height,collapsed:n.hasAttribute('data-virtual-match')}))");for(const h of heights){assert(h.collapsed);assert(Math.abs(h.before-h.after)<=1,'placeholder height '+JSON.stringify(h));}
  results.push({kind:'virtualization',zoom,samples,heights});
 }
 // Match the real wire contract: stable revisions, a pending page, then ten deltas.
 await p.evaluate("(()=>{const t=__r278Game.activeTab(),c=document.getElementById('overview-content');document.documentElement.style.setProperty('--ui-zoom','1');document.documentElement.dataset.uiScale='1';document.getElementById('app-scroll').scrollTop=0;c.querySelector('.match-list')?._virtualObserver?.disconnect();c.replaceChildren();delete c.dataset.matchTierScope;t.current=false;t.region='kr';__r278Game.state.activeGroup='kr';__r278Game.state.activeTabs.kr=t.key;__r278Game.renderPlayerTabWorkspace('kr');t.loading=true;t.initialPagePending=true;t.matchesReceived=false;t.recentStatsReceived=false;t.data={...__r280Overview,player:{...__r280Overview.player,region:'kr'},matches:[],historyRequested:0,pagination:{count:10,partial:true}};__r278Game.renderOverviewBodyContent(c,t);return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()");
 let perf;
 await p.record('foreign-stream',async()=>{
  await p.evaluate("window.__r280Shell=document.querySelector('.overview-layout');window.__r280RetainedRows=new Map();window.__r280Tasks=[];window.__r280Observer=new PerformanceObserver(list=>__r280Tasks.push(...list.getEntries().map(e=>e.duration)));__r280Observer.observe({type:'longtask'});");
  for(let i=1;i<=10;i++){
   const merged=await p.evaluate(`(()=>{const t=__r278Game.activeTab(),m=__r280Overview.matches.find(m=>m.queueId===420);t.matchesReceived=true;t.data={...t.data,matches:Array.from({length:${i}},(_,j)=>({...m,gameId:99500+j,revision:'r280-stream-'+j}))};__r278Game.scheduleOverviewProgress(t);const frame=t.progressFrame;__r278Game.scheduleOverviewProgress(t);return frame===t.progressFrame;})()`);assert(merged);
   await new Promise(r=>setTimeout(r,70));
   const retained=await p.evaluate("new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>{let retained=true;for(const n of document.querySelectorAll('.match-list>.match-entry:not(.match-pending)')){const old=__r280RetainedRows.get(n.dataset.matchId);if(old && old!==n)retained=false;__r280RetainedRows.set(n.dataset.matchId,n);}r(retained)})))");assert(retained,'unchanged stream rows must retain their DOM');
  }
  await new Promise(r=>setTimeout(r,100));
  perf=await p.evaluate("__r280Observer.disconnect();({tasks:__r280Tasks,sameShell:__r280Shell===document.querySelector('.overview-layout'),matches:document.querySelectorAll('.match-list>.match-entry:not(.match-pending)').length,retainedRows:__r280RetainedRows.size})");
 });
 results.push({kind:'stream-performance',platform:process.platform,...perf});assert(perf.sameShell);assert.equal(perf.matches,10);assert.equal(perf.retainedRows,10);assert(perf.tasks.length<=3);assert(Math.max(0,...perf.tasks)<=100);
 await p.evaluate("__r278Game.activeTab().loading=false;__r278Game.renderOverview('kr')");
 await p.call('Emulation.setDeviceMetricsOverride',{width:1100,height:951,deviceScaleFactor:1.75,mobile:false});
 for(const theme of ['dark','light'])for(const zoom of [1,2]){
  await p.evaluate(`document.documentElement.dataset.theme='${theme}';document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}';document.querySelector('[data-open-career-dialog]').click()`);await p.until("document.querySelector('#career-dialog').open");await p.shot('career-header-'+theme+'-'+zoom);await p.evaluate("document.getElementById('career-dialog-close').click();document.getElementById('announcements-button').click()");await p.until("document.querySelector('#announcements-dialog').open");await p.shot('announcement-'+theme+'-'+zoom);await p.evaluate("document.querySelector('[data-announcements-close]').click()");
 }
 assert.equal(errors.length,0,JSON.stringify(errors));
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
