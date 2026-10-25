'use strict';
const assert=require('node:assert/strict');
module.exports=async({evaluate,until,click,call,shot,ready,results})=>{
 let sampledTopbar;const failures=[],caseStatus=new Map();
 const collectCase=async(context,check)=>{
  const key=context.name+':'+(context.page??''),dependency=context.page>1?context.name+':'+(context.page-1):null;
  if(dependency&&caseStatus.get(dependency)!=='passed'){caseStatus.set(key,'skipped-dependency-failed');failures.push({...context,status:'skipped-dependency-failed',error:'prior append page failed'});return;}
  try{await check();caseStatus.set(key,'passed');}
  catch(error){
   let snapshot=error.topbarSample||error.pageWaitSnapshot||sampledTopbar;
   try{snapshot={...snapshot,current:await evaluate("(()=>{const b=document.querySelector('.topbar');return {level:b?.dataset.topbarCompact,settled:b?.dataset.topbarSettled,sidebar:document.documentElement.dataset.sidebar,loadedMs:performance.now(),viewport:innerWidth,font:b&&getComputedStyle(b).fontFamily}})()")};}catch{}
   caseStatus.set(key,'failed');failures.push({...context,status:'failed',error:error.message,stack:error.stack,snapshot});
   // Deliberate mutants need their first real assertion, while the baseline collects every independent failure.
   if(process.env.R277_MUTATION_PROBE==='1'){error.failures=[...failures];throw error;}
   try{await evaluate("(()=>{if(window.__r278DispatchSearch)window.dispatchEvent=__r278DispatchSearch;if(window.__r277NativeIdle)window.requestIdleCallback=__r277NativeIdle;return true})()");}catch{}
  }
 };
 try {
 const {settleTopbar,waitViewport}=require('./lib/topbar-settle.cjs');
 const pageCall=call;
 call=async(method,params)=>{const result=await pageCall(method,params);if(method==='Emulation.setDeviceMetricsOverride')await waitViewport({evaluate,width:params.width});return result;};
 const settle=async()=>{sampledTopbar=await settleTopbar({evaluate});};
 await evaluate("window.__r277Tab=document.querySelector('#overview-content')._overviewViewTab;window.__r277Data=structuredClone(__r277Tab.data);window.__r277Historical=structuredClone(__r277Data.historicalRanks||[]);__r277Data.historicalRanks=[];delete __r277Data.rankMilestones;for(const key of ['championStats','seasonChampionStats'])if(__r277Data[key]?.length)__r277Data[key]=Array.from({length:8},(_,i)=>({...__r277Data[key][0],championId:103+i,championName:'默认英雄 '+i}));true");
 // P4 exact rectangles: initial page shell vs final fixture for three player contexts.
 const rects="(()=>{const c=document.querySelector('#overview-content'),r=n=>{if(!n)return null;const b=n.getBoundingClientRect();return {top:b.top,height:b.height,width:b.width}};return {strip:r(c.querySelector('.summoner-strip')),filter:r(c.querySelector('.match-filterbar')),match:r(c.querySelector('.match-entry,.match-skeleton')),cards:[...c.querySelectorAll('.career-column > section')].map(n=>({key:n.dataset.careerKey,...r(n),skeletonRows:n.querySelectorAll('.champion-pending-rows>.champion-skeleton-row').length,gap:n.getBoundingClientRect().bottom-n.lastElementChild.getBoundingClientRect().bottom,padding:parseFloat(getComputedStyle(n).paddingBottom)+parseFloat(getComputedStyle(n).borderBottomWidth)})),text:c.innerText}})()";
 for(const player of ['cn','kr','pro'])for(const theme of ['dark','light'])for(const width of [900,1280,1920]){
  await collectCase({name:'skeleton',player,theme,width},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.dataset.theme='${theme}';document.documentElement.style.setProperty('--ui-zoom','1');true`);
  await evaluate(`(()=>{const t=__r277Tab;t.group='${player==='pro'?'pro':'cn'}';t.region='${player==='cn'?'':'kr'}';t.data=structuredClone(__r277Data);t.data.player.region=t.region;t.data.player.serverId='${player==='cn'?'HN1':''}';t.loading=false;t.matchesReceived=true;t.recentStatsReceived=true;__r266Game.renderOverviewBody(document.querySelector('#overview-content'),t);return true})()`);await settle();
  const final=await evaluate(rects);await shot(`r277-p4-${player}-${theme}-${width}-final`);
  await evaluate("(()=>{const t=__r277Tab;window.__r277FinalData=t.data;t.identity=t.data.player;t.data=null;t.loading=true;t.initialPagePending=true;t.matchesReceived=false;t.recentStatsReceived=false;__r266Game.renderOverviewBody(document.querySelector('#overview-content'),t);return true})()");await settle();const pending=await evaluate(rects);await shot(`r277-p4-${player}-${theme}-${width}-pending`);
  results.push({P:4,player,theme,width,pending,final});
  if(process.env.R277_MEASURE_ONLY!=='1'){
   for(const key of ['strip','filter','match'])for(const metric of ['top','height'])assert(Math.abs(pending[key][metric]-final[key][metric])<=4,JSON.stringify({player,theme,width,key,metric,pending:pending[key],final:final[key]}));
   assert(!/已统计 0 场|0\.00:1|全部英雄/.test(pending.text));
   // Card row counts arrive with their statistics; compare the same structure while masked.
   // The initial unknown-data shell above still protects the strip and match geometry.
   await evaluate("__r277Tab.data=structuredClone(__r277FinalData);__r266Game.renderOverviewBody(document.querySelector('#overview-content'),__r277Tab);true");await settle();
   const structuredPending=await evaluate(rects);results.at(-1).structuredPending=structuredPending;
   for(const card of structuredPending.cards){const f=final.cards.find(c=>c.key===card.key);assert(f,card.key);if(card.key==='ranks'){for(const sample of [card,f])assert(Math.abs(sample.gap-sample.padding)<=1,JSON.stringify({player,width,sample}));}else if(card.key==='champions'){
    assert.equal(card.skeletonRows,5,JSON.stringify({player,width,card}));
    assert.equal(f.skeletonRows,0,JSON.stringify({player,width,f}));
    for(const sample of [card,f])assert(Math.abs(sample.gap-sample.padding)<=1,JSON.stringify({player,width,sample}));
   }else assert(Math.abs(f.height-card.height)<=4,JSON.stringify({player,width,key:card.key,pending:card.height,final:f.height}));}
  }
  await evaluate("__r277Tab.data=__r277FinalData;__r277Tab.loading=false;__r277Tab.initialPagePending=false;__r277Tab.matchesReceived=true;__r277Tab.recentStatsReceived=true;__r266Game.renderOverviewBody(document.querySelector('#overview-content'),__r277Tab);true");await settle();
   });
 }
 await evaluate("__r277Tab.region='';__r277Tab.group='cn';__r277Tab.data=structuredClone(__r277Data);__r266Game.renderOverviewBody(document.querySelector('#overview-content'),__r277Tab);true");
 // History content must arrive while the reserved outer rank card stays stable; the original equal-height matrix above remains unchanged.
 for(const region of ['','kr']){
  await collectCase({name:'rank-arrival',region},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width:1920,height:900,deviceScaleFactor:1,mobile:false});
  await evaluate(`(()=>{const t=__r277Tab;t.region='${region}';t.data=structuredClone(__r277Data);t.data.player.region=t.region;t.loading=false;__r266Game.renderOverviewBody(document.querySelector('#overview-content'),t);window.__r277RankCard=document.querySelector('[data-career-key=ranks]');window.__r277RankBefore=__r277RankCard.getBoundingClientRect().height;window.__r277HistoryBefore=__r277RankCard.querySelector('.rank-history,.rank-milestones')?.getBoundingClientRect().height||0;window.__r277MatchBefore=document.querySelector('.match-list');window.__r277MatchTop=__r277MatchBefore.getBoundingClientRect().top;return true})()`);
  await settle();
  await evaluate(`(()=>{const t=__r277Tab;if('${region}')t.data.historicalRanks=structuredClone(__r277Historical);else t.data.rankMilestones={peakTier:'diamond',peakDivision:'I',previousSeason:[{queueType:'RANKED_SOLO_5x5',tier:'emerald',division:'II'}]};__r266Game.renderOverviewBody(document.querySelector('#overview-content'),t);return true})()`);await settle();
  const arrival=await evaluate("({before:__r277HistoryBefore,after:document.querySelector('[data-career-key=ranks] .rank-history,[data-career-key=ranks] .rank-milestones')?.getBoundingClientRect().height||0,cardBefore:__r277RankBefore,cardAfter:document.querySelector('[data-career-key=ranks]').getBoundingClientRect().height,gap:document.querySelector('[data-career-key=ranks]').getBoundingClientRect().bottom-document.querySelector('[data-career-key=ranks]').lastElementChild.getBoundingClientRect().bottom,padding:parseFloat(getComputedStyle(document.querySelector('[data-career-key=ranks]')).paddingBottom)+parseFloat(getComputedStyle(document.querySelector('[data-career-key=ranks]')).borderBottomWidth),rows:document.querySelectorAll('[data-career-key=ranks] .rank-list>.rank-row').length,skeletons:document.querySelectorAll('[data-career-key=ranks] .gameplay-skeleton').length,sameList:__r277MatchBefore===document.querySelector('.match-list'),beforeTop:__r277MatchTop,afterTop:document.querySelector('.match-list').getBoundingClientRect().top})");
  assert(arrival.cardAfter>arrival.cardBefore,JSON.stringify({region,...arrival}));assert(Math.abs(arrival.gap-arrival.padding)<=1,JSON.stringify({region,...arrival}));  assert(arrival.after>arrival.before,JSON.stringify({region,...arrival}));assert.equal(arrival.rows,2);assert.equal(arrival.skeletons,0);assert(arrival.sameList);assert(Math.abs(arrival.beforeTop-arrival.afterTop)<=4);results.push({P:4,rankDataArrival:true,region,...arrival});
   });
 }
 await evaluate("__r277Tab.region='';__r277Tab.group='cn';__r277Tab.data=structuredClone(__r277Data);__r266Game.renderOverviewBody(document.querySelector('#overview-content'),__r277Tab);true");
 // P5 preserve decoded images across close/category/reopen (not merely HTTP cache).
 await click('[data-af-open],[data-history-filter-load]');await until("document.querySelector('dialog[data-af-menu]:modal')");await click('[data-af-category="hero"]');
 await until("[...document.querySelectorAll('.af-options.is-hero img')].filter(i=>i.getBoundingClientRect().top<innerHeight).every(i=>i.complete&&i.naturalWidth>0)");
 await evaluate("window.__r277Images=[...document.querySelectorAll('.af-options.is-hero img')];window.__r277Requests=performance.getEntriesByType('resource').filter(e=>/api\\/(image|champion-asset)/.test(e.name)).length;true");
 await click('[data-af-close]');await click('[data-af-open]');await click('[data-af-category="hero"]');await settle();
 const reuse=await evaluate("({same:__r277Images.every((img,i)=>img===document.querySelectorAll('.af-options.is-hero img')[i]),newRequests:performance.getEntriesByType('resource').filter(e=>/api\\/(image|champion-asset)/.test(e.name)).length-__r277Requests,placeholders:[...document.querySelectorAll('.af-options.is-hero img')].filter(i=>i.getBoundingClientRect().top<innerHeight).filter(i=>!i.complete||!i.naturalWidth).length})");assert(reuse.same);assert.equal(reuse.newRequests,0);assert.equal(reuse.placeholders,0);results.push({P:5,...reuse});
 for(const width of [900,1280,1920])for(const zoom of [1,1.25,1.5]){
  await collectCase({name:'filter-geometry',width,zoom},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');true`);await settle();
  const geometry=await evaluate("(()=>{const menu=document.querySelector('[data-af-menu]'),p=menu.querySelector('.af-pane').getBoundingClientRect(),c=menu.querySelector('.af-categories').getBoundingClientRect(),o=menu.querySelector('[data-af-option]');return {pane:p.toJSON(),categories:c.toJSON(),weight:getComputedStyle(o.querySelector('strong')).fontWeight,overflow:menu.scrollWidth-menu.clientWidth}})()");assert.equal(geometry.overflow,0);assert(geometry.categories.right<=geometry.pane.left+1);assert(Number(geometry.weight)>=600);results.push({P:5,width,zoom,...geometry});await shot(`r277-p5-${width}-${zoom}`);
   });
 }
 await click('[data-af-close]');
 // P7 real width × zoom matrix, no overlap and all visible text is one line.
 const savedSidebar=await evaluate("localStorage.getItem('lol-loot-sidebar')");
 await call('Emulation.setDeviceMetricsOverride',{width:700,height:900,deviceScaleFactor:1,mobile:false});await evaluate("document.documentElement.style.setProperty('--ui-zoom','2');true");await settle();
 assert.equal(await evaluate("document.documentElement.dataset.sidebar"),'collapsed','automatic sidebar collapse');
 assert.equal(await evaluate("getComputedStyle(document.querySelector('#refresh .refresh-icon + span')).display"),'none','refresh label must hide in narrow topbar');
 await call('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});await evaluate("document.documentElement.style.setProperty('--ui-zoom','1');true");await settle();
 for(const width of [900,1100,1280,1440,1600,1920])for(const zoom of [.9,1,1.1,1.25,1.5,1.75,2]){
  await collectCase({name:'topbar-empty',width,zoom},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');true`);await settle();
  const buttons=await evaluate("[...document.querySelectorAll('.topbar button')].filter(n=>n.getBoundingClientRect().width>0).map(n=>({id:n.id,rect:n.getBoundingClientRect().toJSON(),nowrap:getComputedStyle(n).whiteSpace}))");for(const b of buttons)assert.equal(b.nowrap,'nowrap',JSON.stringify({width,zoom,b}));for(let i=0;i<buttons.length;i++)for(let j=i+1;j<buttons.length;j++){const a=buttons[i].rect,b=buttons[j].rect;assert(!(Math.min(a.right,b.right)-Math.max(a.left,b.left)>.5 && Math.min(a.bottom,b.bottom)-Math.max(a.top,b.top)>.5),JSON.stringify({width,zoom,a:buttons[i],b:buttons[j]}));}results.push({P:7,width,zoom,buttons});
   });
 }
 await call('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});await evaluate("document.documentElement.style.setProperty('--ui-zoom','1');true");
 // P7 approved R278 regression: retain original 42 combinations and archive both extreme viewports in both themes.
 const previousTheme=await evaluate("document.documentElement.dataset.theme");
 for(const theme of ['dark','light'])for(const [width,zoom] of [[700,2],[900,2],[1920,.9]]){
  await collectCase({name:'topbar-viewport',theme,width,zoom},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.dataset.theme='${theme}';document.documentElement.style.setProperty('--ui-zoom','${zoom}');true`);await settle();
  const bounds=await evaluate("[...document.querySelectorAll('.topbar button')].map(n=>({id:n.id,...n.getBoundingClientRect().toJSON()})).filter(n=>n.width>0)");
  assert(bounds.every(n=>n.left>=0 && n.right<=width+1),JSON.stringify({theme,width,zoom,bounds}));await shot(`r278-p7-${theme}-${width}-${zoom}`);results.push({P:7,theme,width,zoom,viewportBounds:bounds});
   });
 }
 await call('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.dataset.theme='${previousTheme}';document.documentElement.style.setProperty('--ui-zoom','1');true`);await settle();
 // P7 additional query state: preserve the original empty-query matrix above.
 await evaluate("(()=>{const n=document.querySelector('#player-search-name');n.value='fixture';n.dispatchEvent(new Event('input',{bubbles:true}));return true})()");
 for(const width of [900,1100,1280,1440,1600,1920])for(const zoom of [.9,1,1.1,1.25,1.5,1.75,2]){
  await collectCase({name:'topbar-query',width,zoom},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.style.setProperty('--ui-zoom','${zoom}');true`);await settle();
  assert(await evaluate("!document.querySelector('#player-search-clear').hidden && document.querySelector('#player-search-clear').getBoundingClientRect().width>0"),'query state must show clear button');
  const buttons=await evaluate("[...document.querySelectorAll('.topbar button')].filter(n=>n.getBoundingClientRect().width>0).map(n=>({id:n.id,rect:n.getBoundingClientRect().toJSON(),nowrap:getComputedStyle(n).whiteSpace}))");for(const b of buttons)assert.equal(b.nowrap,'nowrap',JSON.stringify({state:'query',width,zoom,b}));for(let i=0;i<buttons.length;i++)for(let j=i+1;j<buttons.length;j++){const a=buttons[i].rect,b=buttons[j].rect;assert(!(Math.min(a.right,b.right)-Math.max(a.left,b.left)>.5 && Math.min(a.bottom,b.bottom)-Math.max(a.top,b.top)>.5),JSON.stringify({state:'query',width,zoom,a:buttons[i],b:buttons[j]}));}results.push({P:7,state:'query',width,zoom,buttons});
   });
 }
 const pointerClick=async selector=>{
  const before=await evaluate("(()=>{const b=document.querySelector('.topbar');return {level:b.dataset.topbarCompact,settled:b.dataset.topbarSettled,sidebar:document.documentElement.dataset.sidebar,loadedMs:performance.now()}})()");
  await settle();
  const r=await evaluate(`document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect().toJSON()`);assert(r.width>0&&r.height>0,JSON.stringify({selector,r}));
  const x=(r.left+r.right)/2,y=(r.top+r.bottom)/2;
  await evaluate(`(()=>{const b=document.querySelector('.topbar'),hit=document.elementFromPoint(${x},${y}),status=__r266App.state.status;return (window.__r278ClickTrace ||= []).push({selector:${JSON.stringify(selector)},before:${JSON.stringify(before)},after:{level:b.dataset.topbarCompact,settled:b.dataset.topbarSettled,sidebar:document.documentElement.dataset.sidebar,loadedMs:performance.now()},hit:hit?.closest('button,input')?.id,client:{connected:status?.connected,region:status?.clientRegion,server:status?.serverId},query:{region:document.querySelector('#player-search-region').dataset.region,server:document.querySelector('#player-search-region').dataset.serverId},modal:document.querySelector('dialog:modal')?.id});})()`);
  await call('Input.dispatchMouseEvent',{type:'mousePressed',x,y,button:'left',clickCount:1});await call('Input.dispatchMouseEvent',{type:'mouseReleased',x,y,button:'left',clickCount:1});
 };
 for(const theme of ['dark','light'])for(const [width,zoom] of [[700,2],[900,2],[1920,.9]]){
  await collectCase({name:'query-input-click',theme,width,zoom},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.dataset.theme='${theme}';document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.querySelector('#player-search-name').value='';true`);await settle();
  await pointerClick('#player-search-name');await call('Input.insertText',{text:'输入测试'});await settle();
  const responsive=await evaluate("({sidebar:document.documentElement.dataset.sidebar,saved:localStorage.getItem('lol-loot-sidebar'),frame:document.querySelector('.app-frame').clientWidth,topbar:document.querySelector('.topbar').getBoundingClientRect().width,label:getComputedStyle(document.querySelector('#refresh .refresh-icon + span')).display,refresh:document.querySelector('#refresh').getBoundingClientRect().toJSON()})");
  if(width===700&&zoom===2)assert.equal(responsive.sidebar,'collapsed','automatic sidebar collapse');
  assert.equal(responsive.saved,savedSidebar,'automatic layout must preserve saved sidebar preference');
  if(width===1920&&zoom===.9)assert.equal(responsive.sidebar,savedSidebar==='collapsed'?'collapsed':'expanded','wide layout must restore saved sidebar preference');
  if(responsive.topbar<=600)assert.equal(responsive.label,'none','refresh label must hide in narrow topbar');
  const input=await evaluate("(()=>{const n=document.querySelector('#player-search-name'),s=getComputedStyle(n);return {focused:document.activeElement===n,value:n.value,selection:n.selectionStart,rect:n.getBoundingClientRect().toJSON(),caret:s.caretColor,fields:document.querySelector('.player-search-fields').getBoundingClientRect().toJSON(),clear:document.querySelector('#player-search-clear').getBoundingClientRect().toJSON(),submit:document.querySelector('#player-search-go').getBoundingClientRect().toJSON(),icons:[...document.querySelectorAll('#player-search-clear svg,#player-search-go svg')].map(n=>n.getBoundingClientRect().toJSON())}})()");
  assert(input.focused);assert.equal(input.value,'输入测试');assert.equal(input.selection,4);assert(input.rect.width>=2*zoom,JSON.stringify(input));assert.notEqual(input.caret,'transparent');assert.notEqual(input.caret,'rgba(0, 0, 0, 0)');
  assert(!await evaluate("document.querySelector('#player-search-clear').hidden"));
  const bounds=await evaluate("[...document.querySelectorAll('.topbar button')].map(n=>({id:n.id,...n.getBoundingClientRect().toJSON()})).filter(n=>n.width>0)");assert(bounds.every(n=>n.left>=0&&n.right<=width+1),JSON.stringify({state:'query',theme,width,zoom,bounds}));
  await shot(`r278-p7-query-${theme}-${width}-${zoom}`);
  await pointerClick('#player-search-clear');assert(await evaluate("!document.querySelector('#player-search-name').value && !document.querySelector('#player-search-tag').value && document.querySelector('#player-search-clear').hidden"));
  // Observe the real submit handler while isolating downstream player lookup from this geometry fixture.
  await evaluate("(()=>{window.__r278SearchSubmit=null;window.__r278DispatchSearch=window.dispatchEvent;window.dispatchEvent=function(e){if(e.type==='deep-legends:open-player'&&e.detail?.source==='search'){__r278SearchSubmit=e.detail;return true;}return __r278DispatchSearch.call(this,e);};const n=document.querySelector('#player-search-name');n.value='fixture';document.querySelector('#player-search-tag').value='CN1';n.dispatchEvent(new Event('input',{bubbles:true}));return true})()");
  await pointerClick('#player-search-go');const submission=await evaluate("__r278SearchSubmit");assert.equal(submission?.gameName,'fixture');assert.equal(submission?.tagLine,'CN1');assert.equal(submission?.source,'search');
  await evaluate("window.dispatchEvent=__r278DispatchSearch;true");results.push({P:7,state:'query-input-click',theme,width,zoom,responsive,...input,submission,clickTrace:await evaluate('window.__r278ClickTrace')});
   });
 }
 await call('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.dataset.theme='${previousTheme}';document.documentElement.style.setProperty('--ui-zoom','1');true`);await settle();
 // P7 compact region labels preserve full query identity and show two complete characters without ellipsis.
 for(const [region,server,full,short] of [['cn','HN1','艾欧尼亚','艾欧'],['cn','HN10','黑色玫瑰','黑色'],['kr','','韩服','韩服'],['jp1','','日服','日服']])for(const width of [700,900]){
  await collectCase({name:'compact-region',region,server,full,short,width},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`document.documentElement.style.setProperty('--ui-zoom','2');__r266App.applySearchRegion('${region}','${server}',false);true`);await settle();
  const labelFrame=await settleTopbar({evaluate,sample:"(()=>{const full=document.querySelector('#player-search-region-label'),short=document.querySelector('#player-search-region-short'),n=getComputedStyle(short).display!=='none'?short:full,b=n.parentElement;return {full:full.textContent,visible:n.textContent,compact:n===short,overflow:getComputedStyle(n).textOverflow,scroll:n.scrollWidth,client:n.clientWidth,title:b.title,aria:b.getAttribute('aria-label'),region:b.dataset.region,server:b.dataset.serverId}})()"});sampledTopbar=labelFrame;const label=labelFrame.sample;
  assert.equal(label.full,full);if(width===700)assert(label.compact,'700px must retain compact region labels');if(label.compact){assert.equal(label.visible,short,'compact region must show two characters');assert.equal(label.overflow,'clip','compact region must not use ellipsis');}else assert.equal(label.visible,full,'available room must retain the full region label');assert(label.scroll<=label.client+1,'compact short name must not be clipped');assert.equal(label.title,full);assert.equal(label.aria,full);assert.equal(label.region,region);assert.equal(label.server,server);results.push({P:7,state:'compact-region',...label});
  await call('Emulation.setDeviceMetricsOverride',{width:1920,height:900,deviceScaleFactor:1,mobile:false});await evaluate("document.documentElement.style.setProperty('--ui-zoom','.9');true");await settle();
  assert(await evaluate("(()=>{const n=document.querySelector('#player-search-region-label'),r=document.createRange();r.selectNodeContents(n);return r.getBoundingClientRect().width<=n.getBoundingClientRect().width+1})()"),'wide region must show full name');
   });
 }
 await call('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});await evaluate("document.documentElement.style.setProperty('--ui-zoom','1');__r266App.applySearchRegion('cn','',false);true");await settle();
 // P8 expansion real bindings and hidden cascade.
 await evaluate("(()=>{const n=document.createElement('section');n.id='r277-ranks';n.innerHTML=__r266Game.renderHistoricalRanks(Array.from({length:8},(_,i)=>({season:'S'+i,tier:'diamond',leaguePoints:3})));document.querySelector('#overview-content').append(n);__r266Game.bindRankHistoryControls(n);return true})()");
 assert.equal(await evaluate("[...document.querySelectorAll('#r277-ranks [data-rank-history-extra]')].filter(n=>n.getBoundingClientRect().height>0).length"),0);await click('#r277-ranks [data-rank-history-toggle]');assert.equal(await evaluate("[...document.querySelectorAll('#r277-ranks [data-rank-history-extra]')].filter(n=>n.getBoundingClientRect().height>0).length"),3);await click('#r277-ranks [data-rank-history-toggle]');assert.equal(await evaluate("[...document.querySelectorAll('#r277-ranks [data-rank-history-extra]')].filter(n=>n.getBoundingClientRect().height>0).length"),0);assert(await evaluate("!document.querySelector('#r277-ranks').textContent.includes('未提供') && !document.querySelector('#r277-ranks').textContent.includes('胜率')"));results.push({P:8,expanded:3,collapsed:0});
 // P9 every append retains the table and existing rows; 400 placeholders keep scroll reachability.
 await evaluate("(()=>{const t=__r277Tab;t.current=true;t.region='';t.group='players';t.matchFilter='all';t.advancedConditions={};t.filteredVisibleCount=20;t.data=structuredClone(__r277Data);t.data.matches=t.data.matches.slice(0,20);t.data.pagination={count:20,hasMore:true};__r266Game.renderOverviewBody(document.querySelector('#overview-content'),t);window.__r277List=document.querySelector('#overview-content .match-list');window.__r277First=__r277List.firstElementChild;window.__r277FullRender=0;new MutationObserver(rs=>{for(const r of rs)for(const n of r.removedNodes)if(n===__r277List)__r277FullRender++;}).observe(__r277List.parentNode,{childList:true});return true})()");
 for(let page=1;page<20;page++){
  await collectCase({name:'append-page',page},async()=>{
  await evaluate(`(()=>{const t=__r277Tab,added=Array.from({length:20},(_,i)=>({...structuredClone(__r277Data.matches[i%__r277Data.matches.length]),gameId:900000+${page}*20+i}));t.data.matches=[...t.data.matches,...added];t.data.pagination.count=t.data.matches.length;__r266Game.loadNextMatchWindow(t);return true})()`);await settle();
  const sample=await evaluate("({sameList:__r277List===document.querySelector('#overview-content .match-list'),sameFirst:__r277First===__r277List.firstElementChild,fullRenders:__r277FullRender,entries:__r277List.querySelectorAll(':scope>.match-entry').length,nodes:document.querySelectorAll('*').length,rich:__r277List.querySelectorAll(':scope>.match-entry:not([data-virtual-match])').length})");
  assert(sample.sameList && sample.sameFirst);assert.equal(sample.fullRenders,0);assert.equal(sample.entries,(page+1)*20);assert(sample.nodes<=25000,JSON.stringify(sample));assert(sample.rich<=60,JSON.stringify(sample));results.push({P:9,page,...sample});
   });
 }
 await evaluate("document.querySelector('#app-scroll').scrollTop=100000;true");await settle();await until("(()=>{const e=document.querySelector('#overview-content .match-list').lastElementChild;return e.children.length>0})()");results.push({P:9,tailRecovered:true});await shot('r277-p9-400-tail');
 // P1 committed client-view frames remove/rebuild self and start the overview load.
 await evaluate("window.__r277Ready=structuredClone(__r266App.state.clientView);window.__r277Generation=__r277Ready.generation+100;__r266App.acceptClientView({...__r277Ready,state:'exiting',generation:__r277Generation});true");
 await until("!document.querySelector('[data-player-tab=\"current\"]')");
 await evaluate("__r266App.acceptClientView({...__r277Ready,state:'no-client',generation:__r277Generation+1});__r266App.acceptClientView({...__r277Ready,state:'ready',generation:__r277Generation+2});true");
 // A ready connection starts a new collection read; its cleared snapshot must
 // be restored by a real status response, as in the production scan flow.
 await evaluate("__r266App.refreshStatus().then(()=>{if(!__r266App.state.status.snapshotReady)throw Error('reconnected collection snapshot missing');return true})");
 await until("__r266Game.state.tabs.some(t=>t.current && t.data?.matches?.length>0 && !t.loading)");
 const rejection=await evaluate("(()=>{const before=__r277FlowEvents.length;for(let i=0;i<20;i++)__r266App.acceptClientView({...__r277Ready,generation:__r277Generation});return __r277FlowEvents.slice(before).filter(e=>e[0]==='client_view_client')})()");assert.equal(rejection.length,2);assert(rejection.some(e=>e[1]==='rejected'&&e[2].reason==='generation-rollback'));results.push({P:1,selfReappeared:true,staleFrameDiagnostics:rejection});
 // P10 real settings handler, not a CSS-only test write.
 await evaluate("window.__r277ZoomBefore=__r277FlowEvents.length;(()=>{const s=document.querySelector('#setting-ui-scale');s.value='1.25';s.dispatchEvent(new Event('change',{bubbles:true}));return true})()");await settle();
 const zoom=await evaluate("__r277FlowEvents.slice(__r277ZoomBefore).filter(e=>e[0]==='ui_zoom_state')");assert.equal(zoom.length,1);assert.equal(zoom[0][2].multiplier,1.25);assert.equal(zoom[0][2].windowWidth,1280);assert.equal(zoom[0][2].windowHeight,900);assert(zoom[0][2].screenWidth>0);assert(zoom[0][2].devicePixelRatio>0);
 await evaluate("(()=>{const s=document.querySelector('#setting-ui-scale');s.dispatchEvent(new Event('change',{bubbles:true}));return true})()");await settle();assert.equal(await evaluate("__r277FlowEvents.slice(__r277ZoomBefore).filter(e=>e[0]==='ui_zoom_state').length"),1);results.push({P:10,startup:await evaluate("__r277FlowEvents.filter(e=>e[0]==='ui_zoom_state')[0]"),change:zoom[0],deduplicated:true});
 await evaluate("(()=>{const s=document.querySelector('#setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));return true})()");
 // P6 nav persistence comes from account-local state; visiting does not read individual cards.
 await evaluate("(()=>{const n=deepLegendsCollectionNewItems;window.__r277CollectionStatus={connected:true,snapshotReady:true,collectionAccount:'r277-browser',clientRegion:'TENCENT',serverId:'HN1',ownedCount:0,chromaOwnedCount:0};n.setAccount(__r277CollectionStatus);n.setAccount({...__r277CollectionStatus,ownedCount:1});return true})()");assert(await evaluate("document.querySelector('#section-favorites').classList.contains('has-collection-unread')"));assert.equal(await evaluate("getComputedStyle(document.querySelector('#section-favorites'),'::after').animationName"),'none');
 await click('#section-favorites');await click('[data-favorites-page="collection"]');await until("window.__r266App.state.items.length>0");assert(await evaluate("!document.querySelector('#section-favorites').classList.contains('has-collection-unread')"));
 await evaluate("(()=>{const n=deepLegendsCollectionNewItems;n.setAccount(__r277CollectionStatus);n.update('skins',[]);const card=document.querySelector('#skin-grid [data-collection-id]');n.update('skins',[{id:card.dataset.collectionId,owned:true}]);n.decorate(card,'skins',card.dataset.collectionId);return true})()");
 for(const theme of ['dark','light']){
  await collectCase({name:'skeleton7',theme},async()=>{await evaluate(`document.documentElement.dataset.theme='${theme}';true`);const style=await evaluate("(()=>{const c=document.querySelector('#skin-grid .is-new-collection-item'),s=getComputedStyle(c);return {outline:s.outlineWidth,blue:s.outlineColor,glow:s.boxShadow}})()");assert.equal(style.outline,'3px');assert.equal(style.blue,'rgb(52, 141, 255)');assert.notEqual(style.glow,'none');results.push({P:6,theme,...style});await shot(`r277-p6-${theme}`);  });
 }
 await evaluate("document.querySelector('#skin-grid .is-new-collection-item').dispatchEvent(new MouseEvent('mouseenter'));true");assert(await evaluate("!document.querySelector('#skin-grid .is-new-collection-item')"));
 // P9 collection first 40 are synchronous, all later chunks enter browser idle callbacks.
 await evaluate("window.__r277Idle=0;window.__r277NativeIdle=requestIdleCallback;window.requestIdleCallback=(fn,opts)=>{__r277Idle++;return __r277NativeIdle(fn,opts);};true");
 const first=await evaluate("(()=>{const a=__r266App;a.state.items=Array.from({length:1200},(_,i)=>({...a.state.items[0],id:700000+i,name:'idle fixture '+i,owned:true}));a.state.view='owned';a.state.loading=false;a.state.status.snapshotReady=true;a.renderItems();return document.querySelector('#skin-grid').children.length})()");assert.equal(first,40);await until("document.querySelector('#skin-grid').children.length===1200");assert(await evaluate('__r277Idle>0'));results.push({P:9,collectionFirst:first,collectionTotal:1200,idleCallbacks:await evaluate('__r277Idle')});await evaluate("window.requestIdleCallback=__r277NativeIdle;true");
 // P7 real pointer travel and keyboard editing in the compact query viewport.
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'no-preference'}]});await settle();
 for(const theme of ['dark','light'])for(const width of [700,900]){
  await collectCase({name:'query-pan',theme,width},async()=>{
  await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await evaluate(`(()=>{document.documentElement.dataset.theme='${theme}';document.documentElement.style.setProperty('--ui-zoom','2');const n=document.querySelector('#player-search-name');n.value='召唤师名称';document.querySelector('#player-search-tag').value='12345';n.dispatchEvent(new Event('input',{bubbles:true}));return true})()`);await settle();
  await evaluate("document.querySelector('#player-search-name').focus();true");await until("document.querySelector('.player-search-query').scrollLeft<1");await pointerClick('#player-search-name');await evaluate("document.activeElement.blur();true");await until("document.querySelector('.player-search-query').scrollLeft<1");
  await evaluate("(()=>{const v=document.querySelector('.player-search-query');window.__r278PanTrace=[];const add=(type,e)=>{__r278PanTrace.push({type,x:e?.clientX,buttons:e?.buttons,left:v.scrollLeft,max:v.scrollWidth-v.clientWidth,width:v.clientWidth,level:document.querySelector('.topbar').dataset.topbarCompact,focus:document.activeElement.id});if(__r278PanTrace.length>60)__r278PanTrace.shift();};v.addEventListener('mousemove',e=>add('mouse',e));v.addEventListener('scroll',()=>add('scroll'));new ResizeObserver(()=>add('resize')).observe(v);return true})()");
  const pan=await evaluate("(()=>{const v=document.querySelector('.player-search-query');return {rect:v.getBoundingClientRect().toJSON(),fields:document.querySelector('.player-search-fields').getBoundingClientRect().toJSON(),name:document.querySelector('#player-search-name').getBoundingClientRect().toJSON(),tag:document.querySelector('#player-search-tag').getBoundingClientRect().toJSON(),max:v.scrollWidth-v.clientWidth,behavior:getComputedStyle(v).scrollBehavior}})()");if(width===700)assert(pan.max>0);if(pan.max>0){assert.equal(pan.behavior,'smooth');assert(pan.name.left>=pan.rect.left-1&&pan.name.right<=pan.rect.right+1,JSON.stringify(pan));assert(pan.tag.left>=pan.rect.right-1,JSON.stringify(pan));await shot(`r278-p7-pan-${theme}-${width}-name`);
  const y=(pan.rect.top+pan.rect.bottom)/2;
  await call('Input.dispatchMouseEvent',{type:'mouseMoved',x:Math.round(pan.rect.left+pan.rect.width*.55),y});await until(`Math.abs(document.querySelector('.player-search-query').scrollLeft-${pan.max/2})<1`);await shot(`r278-p7-pan-${theme}-${width}-middle`);
  await call('Input.dispatchMouseEvent',{type:'mouseMoved',x:Math.round(pan.rect.left+pan.rect.width*.95),y});await until("(()=>{const v=document.querySelector('.player-search-query');return Math.abs(v.scrollLeft-(v.scrollWidth-v.clientWidth))<1})()");assert(await evaluate("document.querySelector('#player-search-tag').getBoundingClientRect().right<=document.querySelector('.player-search-query').getBoundingClientRect().right+1"));await shot(`r278-p7-pan-${theme}-${width}-tag`);
  await call('Input.dispatchMouseEvent',{type:'mouseMoved',x:pan.rect.left,y:pan.rect.bottom+20});await until("document.querySelector('.player-search-query').scrollLeft<1");
  }else{assert.equal(pan.max,0);assert(pan.name.left>=pan.fields.left-1&&pan.tag.right<=pan.fields.right+1,JSON.stringify(pan));await shot(`r278-p7-pan-${theme}-${width}-fits`);}
  await pointerClick('#player-search-name');for(const type of ['keyDown','keyUp'])await call('Input.dispatchKeyEvent',{type,key:'Tab',code:'Tab',windowsVirtualKeyCode:9});await until("document.activeElement.id==='player-search-tag' && Math.abs(document.querySelector('.player-search-query').scrollLeft-(document.querySelector('.player-search-query').scrollWidth-document.querySelector('.player-search-query').clientWidth))<1");
  for(const type of ['keyDown','keyUp'])await call('Input.dispatchKeyEvent',{type,key:'End',code:'End',windowsVirtualKeyCode:35});await call('Input.insertText',{text:'6'});assert.equal(await evaluate("document.querySelector('#player-search-tag').value"),'123456');
  // Complete-ID paste uses the actual paste handler, then the actual Enter handler.
  await evaluate("(()=>{const n=document.querySelector('#player-search-name'),data=new DataTransfer();data.setData('text','粘贴玩家#76543');n.dispatchEvent(new ClipboardEvent('paste',{clipboardData:data,bubbles:true,cancelable:true}));window.__r278SearchSubmit=null;window.__r278DispatchSearch=window.dispatchEvent;window.dispatchEvent=function(e){if(e.type==='deep-legends:open-player'&&e.detail?.source==='search'){__r278SearchSubmit=e.detail;return true;}return __r278DispatchSearch.call(this,e);};return true})()");
  assert.equal(await evaluate("document.querySelector('#player-search-name').value"),'粘贴玩家');assert.equal(await evaluate("document.querySelector('#player-search-tag').value"),'76543');
  for(const type of ['keyDown','keyUp'])await call('Input.dispatchKeyEvent',{type,key:'Enter',code:'Enter',windowsVirtualKeyCode:13});assert.equal(await evaluate("__r278SearchSubmit?.gameName"),'粘贴玩家');assert.equal(await evaluate("__r278SearchSubmit?.tagLine"),'76543');await evaluate("window.dispatchEvent=__r278DispatchSearch;true");await until("document.querySelector('.player-search-query').scrollLeft<1");results.push({P:7,state:'query-pan',theme,...pan,keyboardTag:'123456',pasteAndEnter:true});
   });
 }
 await call('Emulation.setDeviceMetricsOverride',{width:1920,height:900,deviceScaleFactor:1,mobile:false});await evaluate("document.documentElement.style.setProperty('--ui-zoom','.9');true");await settle();assert.equal(await evaluate("getComputedStyle(document.querySelector('.player-search-query')).display"),'contents');
 await call('Emulation.setDeviceMetricsOverride',{width:700,height:900,deviceScaleFactor:1,mobile:false});await evaluate("document.documentElement.style.setProperty('--ui-zoom','2');true");await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});await settle();assert.equal(await evaluate("getComputedStyle(document.querySelector('.player-search-query')).scrollBehavior"),'auto');await call('Emulation.setEmulatedMedia',{features:[]});

 if(failures.length){const error=new Error(failures.map(f=>f.name+': '+f.error).join('\n'));error.failures=failures;throw error;}

 } catch(error) {
  error.failures ||= [...failures,{name:'strict setup/interaction',error:error.message,stack:error.stack}];
  try {
   const failureSample=error.topbarSample || error.pageWaitSnapshot || sampledTopbar;
   error.topbarGeometry=await evaluate("(()=>{const bar=document.querySelector('.topbar'),rect=n=>n.getBoundingClientRect().toJSON(),item=n=>{const c=getComputedStyle(n);return {id:n.id,class:n.className,rect:rect(n),clientWidth:n.clientWidth,scrollWidth:n.scrollWidth,flex:c.flex,minWidth:c.minWidth,font:c.font,fontFamily:c.fontFamily}};return {width:bar.clientWidth,rect:rect(bar),level:bar.dataset.topbarCompact,settled:bar.dataset.topbarSettled,font:getComputedStyle(bar).fontFamily,children:[...bar.children].map(item),search:[...bar.querySelector('.player-search').children].map(item),buttons:[...bar.querySelectorAll('button')].filter(n=>n.getBoundingClientRect().width).map(item),query:item(bar.querySelector('.player-search-query')),queryLeft:bar.querySelector('.player-search-query').scrollLeft,panTrace:window.__r278PanTrace,clickTrace:window.__r278ClickTrace}})()");
   error.topbarGeometry.failureSample=failureSample;
   await call('DOM.enable');await call('CSS.enable');const root=await call('DOM.getDocument');const nodes=await call('DOM.querySelectorAll',{nodeId:root.root.nodeId,selector:'#player-search-region-short,#player-search-region-label,#current-section-title,#refresh span'});error.topbarGeometry.platformFonts=[];for(const nodeId of nodes.nodeIds)error.topbarGeometry.platformFonts.push(...(await call('CSS.getPlatformFontsForNode',{nodeId})).fonts);
  } catch(diagnosticError) { if(error.topbarGeometry)error.topbarGeometry.diagnosticError=diagnosticError.message; }
  throw error;
 }
};
