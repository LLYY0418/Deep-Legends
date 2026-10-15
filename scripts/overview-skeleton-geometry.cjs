'use strict';
const {withBrowser}=require('./r280-browser-fixture.cjs');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const geometry=`(()=>{const rect=n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}},root=document.getElementById('overview-content');return {scrollTop:document.getElementById('app-scroll').scrollTop,root:rect(root),ancestors:[root,root.parentElement,root.parentElement.parentElement].map(n=>({class:n.className,rect:rect(n),padding:getComputedStyle(n).padding,gap:getComputedStyle(n).gap})),strips:root.querySelectorAll('.summoner-strip').length,strip:rect(root.querySelector('.summoner-strip')),cards:[...root.querySelectorAll('.career-column>[data-career-key]')].map(n=>({key:n.dataset.careerKey,title:n.querySelector('h3')?.textContent,...rect(n)})),filter:rect(root.querySelector('.match-filterbar')),rows:[...root.querySelectorAll('.match-list>.match-entry')].slice(0,5).map(n=>({box:rect(n),cells:[...n.querySelectorAll('.match-result-meta,.match-champion,.match-build,.match-kda,.match-stats,.match-players,.match-actions')].map(rect)})),badText:/战绩未读全|正在读取本赛季英雄统计/.test(root.innerText),iconText:[...root.querySelectorAll('.game-icon')].some(n=>n.innerText.trim())}})()`;
async function run(){await withBrowser(async({page,out,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 await p.call('Emulation.setDeviceMetricsOverride',{width:1583,height:951,deviceScaleFactor:1.75,mobile:false});
 await p.evaluate('window.__r280Tab=__r278Game.activeTab()');
 for(const entry of ['foreign','self','cn-other'])for(const zoom of [1,1.4,2]){
  const measure=async()=>{
  await p.evaluate(`(()=>{const g=__r278Game,t=__r280Tab;t.loading=true;t.initialPagePending=true;t.matchesReceived=false;t.recentStatsReceived=false;t.overviewCardLoad=null;t.error='';t.quotaRetry=null;t.current=${entry==='self'};t.region='${entry==='foreign'?'kr':'TENCENT'}';t.riotId={gameName:'은여하',tagLine:'1103'};t.label='은여하#1103';t.group='${entry==='foreign'?'pro':'players'}';g.state.activeGroup=t.group;g.state.activeTabs[t.group]=t.key;t.identity=${entry==='self'?'__r280Overview.player':'null'};window.__r280SavedData={...__r280Overview,player:{...__r280Overview.player,gameName:'은여하',tagLine:'1103',region:t.region,isCurrent:t.current},matches:Array.from({length:5},(_,i)=>({...__r280Overview.matches.find(m=>m.queueId===420),gameId:98000+i})),pagination:{hasMore:false,count:5}};t.data=null;g.renderPlayerTabWorkspace(t.group);const c=document.getElementById('overview-content');c.replaceChildren();delete c._overviewViewTab;document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.documentElement.dataset.uiScale='${zoom}';g.renderOverviewBodyContent(c,t);return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()`);
  const before=await p.evaluate(geometry);await p.shot(`skeleton-${entry}-${zoom}`);
  await p.evaluate("(()=>{const t=__r280Tab;t.data={...__r280SavedData,matches:__r280SavedData.matches.slice(0,2)};__r278Game.patchOverviewProgress(t);return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()");
  const intermediate=await p.evaluate(geometry);assert.equal(intermediate.badText,false);assert.equal(intermediate.iconText,false);
  await p.evaluate(`(()=>{const g=__r278Game,t=__r280Tab;t.data=__r280SavedData;t.loading=false;t.initialPagePending=false;t.matchesReceived=true;t.recentStatsReceived=true;g.renderOverviewBodyContent(document.getElementById('overview-content'),t);return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()`);
  const after=await p.evaluate(geometry);await p.shot(`loaded-${entry}-${zoom}`);
  results.push({entry,zoom,before,intermediate,after});
  assert.deepEqual(before.cards.map(c=>c.title),intermediate.cards.map(c=>c.title));assert.deepEqual(before.cards.map(c=>c.title),after.cards.map(c=>c.title));
  assert.equal(before.strips,1);assert.equal(before.badText,false);assert.equal(before.iconText,false);
  for(const key of ['strip','filter'])for(const field of ['y','height'])assert(Math.abs(before[key][field]-after[key][field])<=2,entry+' '+zoom+' '+key+' '+field);
  assert.equal(before.cards.length,after.cards.length);for(let i=0;i<before.cards.length;i++)for(const field of ['y','height'])assert(Math.abs(before.cards[i][field]-after.cards[i][field])<=2,entry+' '+zoom+' '+before.cards[i].key+' '+field);
  assert.equal(before.rows.length,5);assert.equal(after.rows.length,5);
  for(let i=0;i<5;i++){for(const field of ['y','height'])assert(Math.abs(before.rows[i].box[field]-after.rows[i].box[field])<=2,'row '+i+' '+field);assert.equal(before.rows[i].cells.length,7);assert.equal(after.rows[i].cells.length,7);for(let j=0;j<7;j++)for(const field of ['x','width'])assert(Math.abs(before.rows[i].cells[j][field]-after.rows[i].cells[j][field])<=2,entry+' '+zoom+' row '+i+' cell '+j+' '+field);}
  };
  if(entry==='foreign' && zoom===1)await p.record('foreign-first-load',measure);else await measure();
 }
 fs.writeFileSync(path.join(out,'skeleton-geometry.json'),JSON.stringify(results,null,2));
 assert.equal(errors.length,0,JSON.stringify(errors));
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
