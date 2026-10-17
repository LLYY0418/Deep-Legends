'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {bootDemoApp}=require('../../desktop/overview-render-helpers.cjs'),{waitForRender}=require('../../desktop/renderer-wait.cjs');
async function fixture(){
 const f=bootDemoApp({appSourceTransform:source=>process.env.R283_APP_SOURCE?fs.readFileSync(process.env.R283_APP_SOURCE,'utf8'):source,runtimeSourceTransform:source=>process.env.R283_RUNTIME_SOURCE?fs.readFileSync(process.env.R283_RUNTIME_SOURCE,'utf8'):source,gameplaySourceTransform:source=>(process.env.R283_GAMEPLAY_SOURCE?fs.readFileSync(process.env.R283_GAMEPLAY_SOURCE,'utf8'):source).replace(/\}\)\(\);\s*$/,'window.__r283Career={state,nodes,activeTab,renderCareerDialogContent,closeCareerDialog,overviewContainer,renderCareerSections,renderOverview,renderLiveInsights};\n})();')});
 const w=f.window,g=w.__r283Career;await waitForRender(w,()=>g?.activeTab()?.data&&!g.activeTab().loading&&w.document.querySelector('.career-column'),'career fixture ready',10000);
 const tab=g.activeTab();tab.data={...tab.data,capabilities:[...tab.data.capabilities.filter(c=>c.name!=='champion-mastery'),{name:'champion-mastery',state:'available'}],seasonStatsProgress:{complete:true,tableSupported:true},seasonChampionStats:tab.data.championStats||[],seasonOverall:{...tab.data.overall,games:30}};const dialog=g.nodes.careerDialog,root=g.nodes.careerDialogContent,requests=[],diagnostics=[],listeners=new WeakMap();
 dialog.close=function(){this.removeAttribute('open');this.dispatchEvent(new w.Event('close'))};
 dialog.showModal=function(){this.setAttribute('open','')};
 const fetch=w.fetch;w.fetch=(url,options)=>{if(String(url).startsWith('/api/gameplay/champion-table')||String(url).startsWith('/api/gameplay/masteries')){requests.push({url:String(url),open:dialog.open});return Promise.resolve(new w.Response(JSON.stringify({available:true,rows:[],items:[],overall:{},queue:'420'})))}if(String(url).startsWith('/api/diagnostics/client'))diagnostics.push(JSON.parse(options.body));return fetch(url,options)};
 const add=w.EventTarget.prototype.addEventListener;w.EventTarget.prototype.addEventListener=function(type,...args){if(!listeners.has(this))listeners.set(this,new Set());listeners.get(this).add(type);return add.call(this,type,...args)};
 function render(layout='masonry'){root.dataset.careerLayout='';dialog.setAttribute('open','');g.renderCareerDialogContent(tab.data,tab,layout);return root}
 return {...f,w,g,tab,dialog,root,requests,diagnostics,listeners,render};
}
test('R283 P18 dialog arrows close first, open both subpages and return with scroll',async()=>{
 const f=await fixture();try{
  for(const region of ['cn','kr'])for(const layout of ['single','masonry'])for(const page of ['champion-table','masteries']){
   f.tab.region=region;f.tab.current=region==='cn';f.tab.data.player={...f.tab.data.player,region,playerRef:'p18-fixture-'+region};f.g.state.activeGroup=region==='kr'?'kr':'players';f.g.state.activeTabs[f.g.state.activeGroup]=f.tab.key;f.g.renderOverview(f.g.state.activeGroup);f.tab.overviewSubpage='';f.w.document.getElementById('app-scroll').scrollTop=315;f.render(layout);
   const arrow=f.root.querySelector('[data-overview-subpage="'+page+'"]');assert(arrow,'production arrow '+page);const count=f.requests.length;arrow.click();
   await waitForRender(f.w,()=>f.requests.length>count&&f.tab.overviewSubpageState?.status==='ready','dialog arrow did not open subpage',2000);
   assert.equal(f.dialog.open,false,'dialog must close before navigation');assert.equal(f.requests.at(-1).open,false,'request started behind modal');assert.equal(f.tab.overviewSubpage,page);assert(f.requests.at(-1).url.includes('playerRef='+encodeURIComponent(f.tab.data.player.playerRef)),'navigation must retain the selected account reference');
   assert(f.diagnostics.some(d=>d.event==='overview_subpage_open'&&d.page===page&&d.from==='dialog'&&d.ok===true),'click diagnostic missing');
   f.g.overviewContainer(f.tab).querySelector('[data-overview-return]').click();assert.equal(f.tab.overviewSubpage,'');assert.equal(f.dialog.open,false);assert.equal(f.w.document.getElementById('app-scroll').scrollTop,315);
  }
  assert.deepEqual(f.errors,[]);
 }finally{f.w.close()}
});
test('R283 P18 telemetry sends subpage actions and existing R283 metrics through runtime',async()=>{
 const f=await fixture();try{
  f.w.reportFlowDiagnostic('overview_subpage_open','open',{page:'masteries',from:'page',ok:false,privateName:'must-not-send'});
  f.w.reportFlowDiagnostic('manual_refresh_client','complete',{overview_requests:1,canceled_requests:0,busy_ms:800,label_changes:2});
  f.w.reportFlowDiagnostic('match_filter_view','render',{filter:'solo',source:'view-cache',count:3,all_list_count:3,missing_count:0});
  for(const event of ['overview_subpage_open','manual_refresh_client','match_filter_view'])assert(f.diagnostics.some(d=>d.event===event),'runtime dropped '+event);
  const sub=f.diagnostics.find(d=>d.event==='overview_subpage_open');assert.equal(sub.from,'page');assert.equal(sub.ok,false);assert(!('privateName' in sub));
 }finally{f.w.close()}
});
test('R284 P6 dialog return restores both scroll positions only while its opener is visible',async()=>{
 const f=await fixture();try{
  let visible=true;Object.defineProperty(f.w.HTMLElement.prototype,'offsetParent',{configurable:true,get(){return this.matches('[data-open-career-dialog]')&&visible?f.w.document.body:null}});
  for(const region of ['cn','kr'])for(const layout of ['single','masonry'])for(const from of ['dialog','page','widened','leave']){
   f.tab.region=region;f.tab.current=region==='cn';f.g.state.activeGroup=region==='kr'?'kr':'players';f.g.state.activeTabs[f.g.state.activeGroup]=f.tab.key;f.tab.data.player={...f.tab.data.player,region,playerRef:'p6-'+region};f.tab.overviewSubpage='';f.g.renderOverview(f.g.state.activeGroup);
   const container=f.g.overviewContainer(f.tab);Object.defineProperty(container,'clientWidth',{configurable:true,value:layout==='single'?500:900});visible=true;f.w.document.getElementById('app-scroll').scrollTop=315;
   if(from!=='page')f.render(layout);const root=from==='page'?container:f.root;root.scrollTop=187;root.querySelector('[data-overview-subpage="masteries"]').click();
   await waitForRender(f.w,()=>f.tab.overviewSubpageState?.status==='ready','subpage ready',2000);
   if(from==='widened')visible=false;if(from==='leave')f.w.dispatchEvent(new f.w.CustomEvent('deep-legends:before-section'));
   container.querySelector('[data-overview-return]').click();await new Promise(r=>f.w.requestAnimationFrame(()=>f.w.requestAnimationFrame(r)));
   assert.equal(f.dialog.open,from==='dialog',region+'/'+layout+'/'+from);assert.equal(f.w.document.getElementById('app-scroll').scrollTop,315);
   if(from==='dialog'){assert.equal(f.root.scrollTop,187);assert.equal(f.root.dataset.careerLayout,layout);f.g.closeCareerDialog();}
   assert.equal(f.tab.overviewDialogReturn,undefined);
  }
  assert.deepEqual(f.errors,[]);
 }finally{f.w.close()}
});
test('R283 P18 same-data casual mode click repaints the open dialog',async()=>{
 const f=await fixture();try{
  f.w.eval(fs.readFileSync(__dirname+'/overview-casual.js','utf8'));delete f.tab.rankedHistorySeen;delete f.tab.overviewMatchSnapshot;f.tab.casualHistory=[];
  f.tab.data={...f.tab.data,ranks:[],rankedQueues:{},seasonQueueStats:{},pagination:{},capabilities:[{name:'ranked-stats',state:'available'},{name:'champion-mastery',state:'available'}],matches:[450,2300].map((queueId,i)=>({gameId:900+i,queueId,result:'win',subjectParticipantId:1,participants:[{participantId:1,championId:i+1,championName:'英雄'+i}]}))};
  for(const layout of ['single','masonry']){f.render(layout);const button=[...f.root.querySelectorAll('[data-casual-mode]')].find(n=>n.dataset.casualMode!==f.tab.casualMode);assert(button);const mode=button.dataset.casualMode,old=f.root.innerHTML;button.click();assert.equal(f.tab.casualMode,mode);assert.notEqual(f.root.innerHTML,old,'same data/layout skipped repaint');assert.equal(f.root.querySelector('[data-casual-mode="'+mode+'"]').getAttribute('aria-pressed'),'true');const champions=f.root.querySelector('[data-career-key="champions"]');assert(champions.textContent.includes(mode==='450'?'英雄0':'英雄1'));assert(!champions.textContent.includes(mode==='450'?'英雄1':'英雄0'),'champion statistics must use the selected mode')}
  assert.deepEqual(f.errors,[]);
 }finally{f.w.close()}
});
function assertControls(f){
 const presentation=/^(tooltip(?:-.+)?|career-.+|metric-.+|win-rate|ranked-queue-scope|rank-history-extra|rating-tab|queued-src|game-image|image-.+|mastery-level)$/;
 const controls=[...f.root.querySelectorAll('*')].filter(n=>n.matches('button,[role="button"]')||[...n.attributes].some(a=>a.name.startsWith('data-')&&!presentation.test(a.name.slice(5))));
 assert(controls.length>=10,'fixture must contain real card controls');
 for(const node of controls){const actions=[...node.attributes].filter(a=>a.name.startsWith('data-')&&!presentation.test(a.name.slice(5)));const tooltipOnly=actions.length===0&&node.hasAttribute('data-tooltip');assert(tooltipOnly||f.listeners.get(node)?.has('click'),'unbound career control '+node.outerHTML.slice(0,240))}
 return controls;
}
test('R283 P18 every rendered dialog action is bound, tooltip-only controls are delegated',async()=>{
 const f=await fixture();try{
  f.render();const controls=assertControls(f);assert(controls.some(n=>n.hasAttribute('data-overview-subpage')));assert(controls.some(n=>n.hasAttribute('data-generate-overview-share')));
  const unbound=f.w.document.createElement('button');unbound.dataset.futureAction='x';f.root.append(unbound);assert.throws(()=>assertControls(f),/unbound career control/);unbound.remove();
  const tip=f.root.querySelector('[data-tooltip]:not([data-tooltip-overflow])');assert(tip);tip.dispatchEvent(new f.w.MouseEvent('pointerover',{bubbles:true}));await new Promise(r=>f.w.requestAnimationFrame(()=>f.w.requestAnimationFrame(r)));const popup=f.w.document.getElementById('global-tooltip');assert.equal(popup.hidden,false);assert.equal(popup.parentElement,f.dialog,'modal tooltip must belong to the modal subtree');assert.equal(popup.style.zoom,'1','modal zoom must not be applied twice');
  tip.dispatchEvent(new f.w.MouseEvent('pointerout',{bubbles:true}));assert(popup.hidden);assert.equal(popup.parentElement,f.w.document.body);assert.equal(popup.style.zoom,'');
  assert.equal(f.root.querySelector('[data-note-popover]'),null,'current production career cards have no note-popover controls');
  tip.dispatchEvent(new f.w.MouseEvent('pointerover',{bubbles:true}));f.g.closeCareerDialog();assert(popup.hidden);assert.equal(popup.parentElement,f.w.document.body);
  assert.deepEqual(f.errors,[]);
 }finally{f.w.close()}
});

test('R283 P17 failed live cards keep only the current-mode failure summary',async()=>{
 const f=await fixture();try{
  for(const queueId of [420,450,2300,1750]){
   const data={phase:'InProgress',queueId,gameMode:queueId===1750?'CHERRY':queueId===2300?'ARAM':'CLASSIC',mapId:queueId===420?11:12,available:true,players:Array.from({length:6},(_,i)=>({playerRef:'test-'+i,gameName:'玩家'+i,championId:1,historyState:'failed',recentGames:[],teamId:100,isAlly:true,isCurrent:i===0}))};
   const doc=new f.w.DOMParser().parseFromString(f.g.renderLiveInsights(data),'text/html'),cards=[...doc.querySelectorAll('[data-live-player-row^="card:"]')];assert.equal(cards.length,6);
   for(const card of cards){assert.equal((card.textContent.match(/读取失败/g)||[]).length,1);assert(card.textContent.includes('当前模式'))}
   assert.equal((doc.body.textContent.match(/读取失败/g)||[]).length,cards.length,'duplicate failed placeholder below player cards');assert.equal(doc.querySelectorAll('.insight-none').length,0);
  }
 }finally{f.w.close()}
});
