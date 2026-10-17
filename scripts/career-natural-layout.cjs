'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
const rectangles=`(()=>{const root=document.querySelector('#overview-content .career-column');return [...root.children].map(n=>{const r=n.getBoundingClientRect();return {key:n.dataset.careerKey,x:r.x,y:r.y,height:r.height}})})()`;
const gaps=root=>`(()=>{const root=${root};return [...root.querySelectorAll('[data-career-key]')].map(n=>{const r=n.getBoundingClientRect(),last=n.lastElementChild.getBoundingClientRect(),s=getComputedStyle(n);return {key:n.dataset.careerKey,height:r.height,gap:r.bottom-last.bottom,padding:parseFloat(s.paddingBottom)+parseFloat(s.borderBottomWidth),overflow:s.overflowY,scroll:n.scrollHeight-n.clientHeight}})})()`;
function requireNatural(rows){assert(rows.length>=5);for(const r of rows){assert(r.gap<=r.padding+1,JSON.stringify(r));assert.notEqual(r.overflow,'auto',JSON.stringify(r));assert(r.scroll<=1,JSON.stringify(r));}}
async function run(){await withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry')&&!__r278Game.activeTab().loading");
 await p.evaluate('window.__naturalOriginal=structuredClone(__r278Game.activeTab().data)');
 for(const theme of ['dark','light'])for(const mode of ['solo-positions','solo-no-positions','mayhem'])for(const zoom of [1,1.14,1.5]){
  await p.call('Emulation.setDeviceMetricsOverride',{width:1920,height:1100,deviceScaleFactor:1,mobile:false});
  await p.evaluate(`(()=>{const g=__r278Game,t=g.activeTab(),d=structuredClone(__naturalOriginal),queue='${mode}'==='mayhem'?'2300':'420',positions='${mode}'==='solo-positions'?[{position:'middle',label:'中单',games:20,wins:12,winRate:60,share:100}]:[],stats={games:20,wins:12,losses:8,winRate:60,kills:5,deaths:3,assists:8,kda:4.3,positions};d.historicalRanks=[];d.rankMilestones=null;d.rankedQueues={420:{recentRanked:stats,positions,ability:d.ability},2300:{recentRanked:stats,positions:[],ability:d.ability}};d.recentRanked=stats;d.positions=positions;t.rankedQueueRecent=queue;t.rankedQueuePosition='420';t.data=d;t.loading=true;t.initialPagePending=true;t.matchesReceived=false;t.recentStatsReceived=false;t.overviewCardLoad=null;document.documentElement.dataset.theme='${theme}';document.documentElement.dataset.uiScale='${zoom}';document.documentElement.style.setProperty('--ui-zoom','${zoom}');g.renderOverview();window.__naturalFrames=[];window.__naturalTimer=setInterval(()=>__naturalFrames.push({at:performance.now(),rows:${rectangles}}),50)})()`);
  await new Promise(r=>setTimeout(r,180));
  const pending=await p.evaluate(rectangles);
  const pendingPosition=await p.evaluate("!!document.querySelector('[data-career-key=recent-ranked] .recent-ranked-positions')");assert.equal(pendingPosition,mode!=='mayhem');
  assert(!/近 0 场|已统计 0 场|暂无英雄统计/.test(await p.evaluate("document.querySelector('#overview-content .career-column').innerText")));
  await p.evaluate("(()=>{const t=__r278Game.activeTab();t.loading=false;t.initialPagePending=false;t.matchesReceived=true;t.recentStatsReceived=true;__r278Game.renderOverview()})()");
  await new Promise(r=>setTimeout(r,180));
  const final=await p.evaluate(rectangles),frames=await p.evaluate('clearInterval(__naturalTimer);__naturalFrames');
  assert.deepEqual(pending.map(r=>r.key),final.map(r=>r.key));
  for(const frame of frames)for(const before of pending){const after=frame.rows.find(r=>r.key===before.key);assert(after);for(const metric of ['x','y','height'])assert(Math.abs(before[metric]-after[metric])<=1,JSON.stringify({theme,mode,zoom,key:before.key,metric,before,after}));}
  const column=await p.evaluate(gaps("document.querySelector('#overview-content .career-column')"));requireNatural(column);
  await p.call('Emulation.setDeviceMetricsOverride',{width:780,height:1100,deviceScaleFactor:1,mobile:false});await p.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  await p.evaluate("__r278Game.openCareerDialog(document.querySelector('#overview-content'),__r278Game.activeTab(),document.querySelector('[data-open-career-dialog]'))");await p.until("document.querySelector('#career-dialog').open");
  const dialog=await p.evaluate(gaps("document.querySelector('#career-dialog')"));requireNatural(dialog);await p.evaluate('__r278Game.closeCareerDialog()');
  await p.call('Emulation.setDeviceMetricsOverride',{width:1920,height:1100,deviceScaleFactor:1,mobile:false});await p.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  await p.evaluate(`(()=>{const n=document.createElement('div');n.style.cssText='position:fixed;left:-10000px;top:0;width:1440px';n.innerHTML=__r278Game.createOverviewShareMarkup(document.querySelector('#overview-content'));document.body.append(n);window.__naturalShare=n;return true})()`);
  const exported=await p.evaluate(gaps('window.__naturalShare'));requireNatural(exported);await p.evaluate('__naturalShare.remove()');
  results.push({theme,mode,zoom,pending,final,frames,column,dialog,share:exported});await p.shot(`natural-${theme}-${mode}-${zoom}`);
 }
 assert.deepEqual(errors,[]);console.log(JSON.stringify({cases:results.length,errors:errors.length}));
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
