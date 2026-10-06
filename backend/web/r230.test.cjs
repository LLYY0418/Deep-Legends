'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile,escapeHTML}=require('./r188-harness.cjs');
const {expand}=require('./r211-harness-support.cjs');
const source=read('gameplay.js'),champions=read('champions.js');
const functions=(names,deps)=>compile(source,expand(source,names,deps),deps);
test('R230 three queues, games ties use win rate/KDA, whole row and keyboard toggle, top three distinct metrics',()=>{
 const dom=new JSDOM('<main></main>'),root=dom.window.document.querySelector('main'),tab={championTableQueue:'420'};
 const row=(id,winRate,kda,score)=>({championId:id,championName:`英雄${id}`,games:10,wins:6,losses:4,winRate,kda,score,damagePerMinute:score*100,tankShare:score/10,controlWards:score,cs:score*10,gold:score*1000,doubleKills:score,tripleKills:0,quadraKills:null,pentaKills:0,opponents:id===1?[{championId:9,championName:'对位',games:5}]:[]});
 const data={queue:'420',overall:row(0,90,9,10),rows:[row(4,50,3,1),row(2,60,3,3),row(1,60,4,4),row(3,55,5,2),row(5,40,2,2)]};let f;tab.overviewSubpageState={data};
 const redraw=()=>{root.innerHTML=f.renderChampionTable(data,tab);f.bindChampionTable(root,tab);};
 f=functions(['renderChampionTable','bindChampionTable'],{document:dom.window.document,prepareImages:()=>{},escapeHTML,iconFigure:()=>'',renderOverviewSubpage:redraw,openOverviewSubpage:()=>{}});redraw();
 assert.deepEqual([...root.querySelectorAll('[data-table-queue]')].map(e=>e.dataset.tableQueue),['420','440','mayhem']);
 assert.deepEqual(f.championTableSortedRows(data,tab).map(e=>e.championId),[1,2,3,4,5]);
 assert.equal(root.querySelectorAll('thead th').length,15);assert.equal(root.querySelector('[data-table-expand]').tagName,'TR');
 assert.equal(root.querySelectorAll('.is-overall .is-top-metric').length,0);assert.equal(root.querySelectorAll('.is-opponent .is-top-metric').length,0);
 assert.equal(root.querySelectorAll('tbody tr:not(.is-overall):not(.is-opponent) td:nth-child(5).is-top-metric').length,4,'tie on third value highlighted together');
 assert.equal(root.querySelectorAll('td:nth-child(12).is-top-metric').length,0,'zeros excluded');
 let expandRow=root.querySelector('[data-table-expand]');assert.equal(expandRow.getAttribute('aria-expanded'),'true');expandRow.querySelector('td').click();assert.equal(root.querySelectorAll('.is-opponent').length,0);
 expandRow=root.querySelector('[data-table-expand]');expandRow.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:' ',bubbles:true,cancelable:true}));assert.equal(root.querySelectorAll('.is-opponent').length,1);
 const css=read('gameplay.css');assert.doesNotMatch(css,/is-hot b \{ color:#d44747|background:#bb4c5c|height:100%; background:#458ddd/);
 dom.window.close();
});
test('R230 copy uses browser, Electron and execCommand fallback and always emits private diagnostics + 3s feedback',async()=>{
 for(const target of ['browser','electron','execCommand','failure']){
  const dom=new JSDOM('<button>origin</button>'),calls=[],events=[],toasts=[],jobs=[];const w=dom.window;
  const el={toast:w.document.createElement('div')};let timer=0;
  const app=compile(read('app.js'),['showToast'],{el,clearTimeout:()=>{},setTimeout:(fn,ms)=>{jobs.push({fn,ms});return ++timer},toastTimer:0});
  w.document.execCommand=()=>{calls.push('execCommand');return target==='execCommand'};
  const navigator={clipboard:{writeText:async text=>{calls.push('browser');if(target!=='browser')throw new w.DOMException('denied','NotAllowedError')}}};
  const window={desktopClipboard:{copyText:async()=>{calls.push('electron');if(target!=='electron')throw Error('unavailable');return true}},reportFlowDiagnostic:(e,r,f)=>events.push({e,r,...f}),deepLegendsToast:(text,ms)=>{toasts.push({text,ms});app.showToast(text,ms)}};
  const f=functions(['copySummonerText','bindSummonerCopy'],{window,document:w.document,navigator});
  assert.equal(await f.copySummonerText('私密名称#1234'),target!=='failure');assert.equal(events[0].method,target==='failure'?'execCommand':target);assert.equal(JSON.stringify(events).includes('私密名称'),false);
  assert.equal(toasts[0].ms,3000);assert.equal(el.toast.hidden,false);jobs[0].fn();assert.equal(el.toast.hidden,true);
  if(target!=='failure')assert.equal(toasts[0].text,'召唤师名称和编号已复制');else assert.equal(toasts[0].text,'复制失败');
  assert.equal(w.document.querySelector('textarea'),null);dom.window.close();
 }
});
test('R230 completion event immediately refreshes even below 100 games and inside throttle window',async()=>{
 const tab={key:'self',data:{player:{playerRef:'account'},seasonStatsProgress:{season:'S26',scanned:49,complete:false}}},state={section:'overview',seasonProgressRefreshes:new Map([['account',{lastAt:1000}]] )};let calls=0;
 const f=functions(['handleSeasonProgress'],{state,overviewGroupForSection:()=> 'players',activeTab:()=>tab,document:{getElementById:()=>null},refreshSeasonSummary:async()=>{calls++},overviewSectionForGroup:()=> 'overview',requestAnimationFrame:fn=>fn(),clearTimeout:()=>{},markSeasonRefreshPending:()=>{}});
 assert.equal(await f.handleSeasonProgress({type:'season-progress',account:'account',season:'S26',scanned:90,complete:true},1500),true);assert.equal(calls,1);
});
test('R230 null atlas is skeleton; loaded directory filtered to zero is empty; render triggers one automatic request',()=>{
 const state={section:'champions',mode:'aram-mayhem',mayhemView:'atlas',augments:null,listScrollInner:0,mayhemAtlasLoading:false},root={querySelector:()=>null};let requests=0;
 const render=compile(champions,['render'],{state,root,window:{},renderWorkspace:()=>{},renderDetail:()=>{},prepareImages:()=>{},applyRenderedMetricStyles:()=>{},mountArenaMatchCards:()=>{},mountMayhemTierDialog:()=>{},mountArenaTierDialog:()=>{},requestAnimationFrame:()=>{},loadMayhemAtlas:()=>{requests++;state.mayhemAtlasLoading=true}});
 render.render();render.render();assert.equal(requests,1);
 const atlas=compile(champions,['renderMayhemAtlas'],{state,renderSkeleton:()=> 'skeleton',renderError:()=> 'error',objectRows:v=>v||[],escapeHTML,renderEmpty:title=>title,renderMayhemRarityPanel:()=>'',renderMayhemAtlasDetail:()=>''});
 assert.equal(atlas.renderMayhemAtlas([]),'skeleton');state.augments={rows:[{id:1}]};assert.match(atlas.renderMayhemAtlas([]),/没有匹配的海克斯/);
});
test('R230 card ready records each data card once with elapsed time and source',()=>{
 const dom=new JSDOM('<aside class="career-column"></aside>'),events=[],tab={overviewCardLoad:{startedAt:10,ready:new Set()},data:{ranks:[{}],masteries:[{}],seasonStatsProgress:{scanned:90}}};
 const f=functions(['reportOverviewCardReady'],{performance:{now:()=>150},window:{reportFlowDiagnostic:(e,r,f)=>events.push(f)},rankedQueueData:()=>({positions:[{}]})});
 f.reportOverviewCardReady(dom.window.document,tab);f.reportOverviewCardReady(dom.window.document,tab);assert.equal(events.length,4);assert.ok(events.every(e=>e.durationMs===140));assert.equal(events.find(e=>e.card==='champions').source,'snapshot');dom.window.close();
});

test('R230 initial queue uses season rank totals before recent samples; requests never use ranked',async()=>{
 for(const [ranks,want] of [ [[], '440'], [[{queueType:'RANKED_SOLO_5x5',wins:1,losses:1}], '420'], [[{queueType:'RANKED_SOLO_5x5',wins:0,losses:0}], '440'] ]){
  const tab={key:'one',data:{player:{playerRef:'ref'},ranks,rankedQueues:{'420':{recentRanked:{games:0}},'440':{recentRanked:{games:20}}}}},urls=[];
  const f=functions(['openOverviewSubpage'],{overviewContainer:()=>null,document:{getElementById:()=>null},rerenderTab:()=>{},api:async url=>{urls.push(url);return {available:true}}});
  await f.openOverviewSubpage(tab,'champion-table');assert.equal(tab.championTableQueue,want);assert.match(urls[0],new RegExp(`&queue=${want}$`));assert.doesNotMatch(urls[0],/ranked/);
 }
});
