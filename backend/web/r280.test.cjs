'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,escapeHTML}=require('./r188-harness.cjs');
const game=fs.readFileSync(__dirname+'/gameplay.js','utf8'),app=fs.readFileSync(__dirname+'/app.js','utf8');
test('R280 P4 unchanged owned catalog repaints after subpage and section navigation',async()=>{
 const extract=require('./r188-harness.cjs').extract;
 for(const route of ['collection','overview']){
  const dom=new JSDOM('<main><div id="grid"></div><div id="pool"></div><p id="meta"></p></main>'),d=dom.window.document,node=()=>d.createElement('div'),tab=(key,value)=>{const b=d.createElement('button');b.dataset[key]=value;b.setAttribute('aria-controls',value+'-panel');return b};
  const el={grid:d.querySelector('#grid'),poolSkinGrid:d.querySelector('#pool'),poolListMeta:d.querySelector('#meta'),appScroll:{scrollTop:0},currentSectionTitle:node(),topbarSubtitle:node(),pageIntro:node(),sectionTabs:['overview','favorites'].map(v=>tab('section',v)),sectionPanels:[],viewTabs:[tab('view','owned')],favoritesTabs:['collection','pools'].map(v=>tab('favoritesPage',v)),favoritesCollectionPanel:node(),favoritesAccountPanel:node(),favoritesPoolsPanel:node(),favoritesFacadePanel:node()};
  const items=[{id:1,name:'一',owned:true},{id:2,name:'二',owned:true},{id:3,name:'三',owned:false}],state={section:'favorites',favoritesPage:'pools',sectionScroll:{},status:{connected:true,calculationOK:true},poolsLoaded:true,poolItems:[],poolCatalogLoaded:false,poolQuery:'',poolView:'owned',poolQuality:'all',poolSort:'name',poolRenderGeneration:0,renderGeneration:0};
  const noop=()=>{},deps={state,el,document:d,window:dom.window,CustomEvent:dom.window.CustomEvent,escapeHTML,formatNumber:String,qualityLabels:{epic:'史诗'},poolRarityGroup:()=> 'epic',localeCompare:(a,b)=>a.localeCompare(b),comparePoolRarity:()=>0,api:async()=>({items}),activateTab:(selected,tabs,fn)=>fn(selected),runRenderTasks:tasks=>tasks.forEach(fn=>fn()),makeSkinCard:skin=>{const n=d.createElement('article');n.className='skin-card';n.dataset.id=skin.id;return n}};
  for(const name of ['cancelRenderFrames','cancelPoolRenderFrames','cancelDeferredImages','stopHoverVideo','resetDiagnosticsExport','restoreSectionScroll','resetPoolControls','resetCollectionControls','triggerCollectionRescanIfDirty','loadSkins','loadPools','loadAccount','renderNotice','renderLaunchpad'])deps[name]=noop;
  const names=['activateSection','activateFavoritesPage','loadPoolCatalog','renderPoolCatalog','visiblePoolItems'];
  const f=Function(...Object.keys(deps),names.map(n=>extract(app,n)).join('\n')+'\nreturn {'+names.join(',')+'}')( ...Object.values(deps));
  await f.loadPoolCatalog();assert.equal(el.poolSkinGrid.querySelectorAll('.skin-card').length,2);
  if(route==='collection')f.activateFavoritesPage('collection');else f.activateSection('overview');
  assert.equal(el.poolSkinGrid.children.length,0);assert.equal(state.poolCatalogSignature,'');assert.equal(state.poolRenderPending,true);
  if(route==='overview')f.activateSection('favorites');f.activateFavoritesPage('pools');await f.loadPoolCatalog();
  assert.equal(el.poolListMeta.textContent,'已拥有 2 款');assert.equal(el.poolSkinGrid.querySelectorAll('.skin-card').length,2);dom.window.close();
 }
});
test('R280 P5 hover bridge, delayed leave, pin and outside close keep retry accessible',async()=>{
 const dom=new JSDOM('<div class="rank-mmr-control"><button data-rank-mmr-trigger aria-expanded="false"></button><div class="rank-mmr-popover"><button data-mayhem-rating-retry>重试</button></div></div>');const doc=dom.window.document,c=doc.querySelector('div'),trigger=doc.querySelector('button'),tab={key:'fixture'};let calls=0,next=0;const timers=new Map();
 const f=compile(game,['bindMayhemRatingControls'],{document:doc,mayhemRatingStatus:()=>({status:'error'}),loadMayhemRating:async()=>calls++,setTimeout:fn=>{timers.set(++next,fn);return next},clearTimeout:id=>timers.delete(id)});
 f.bindMayhemRatingControls(doc,tab);c.dispatchEvent(new dom.window.Event('mouseenter'));c.dispatchEvent(new dom.window.Event('mouseleave'));assert.equal(trigger.getAttribute('aria-expanded'),'true');c.dispatchEvent(new dom.window.Event('mouseenter'));assert.equal(timers.size,0);doc.querySelector('[data-mayhem-rating-retry]').click();assert(calls>=2);trigger.click();c.dispatchEvent(new dom.window.Event('mouseleave'));assert.equal(timers.size,0);doc.body.dispatchEvent(new dom.window.Event('pointerdown',{bubbles:true}));assert.equal(trigger.getAttribute('aria-expanded'),'false');assert.match(fs.readFileSync(__dirname+'/gameplay.css','utf8'),/\.rank-mmr-popover::before/);dom.window.close();
});
test('R280 P5 network failure retries next hover, not-found remains cached',async()=>{
 for(const missing of [false,true]){
  const tab={key:'fixture',region:'cn',mayhemRating:{playerRef:'local-ref',status:missing?'ready':'error',data:missing?{unavailableReason:'未收录'}:null,at:Date.now()}};let requests=0;
  const f=compile(game,['loadMayhemRating'],{state:{},riotTab:()=>false,tabServerID:()=> 'HN1',mayhemRatingStatus:t=>t.mayhemRating,updateMayhemRatingPopover(){},api:async()=>{requests++;return {available:false,unavailableReason:'未收录'}}});
  await f.loadMayhemRating(tab);assert.equal(requests,missing?0:1);
 }
});
test('R280 P6 manual retry gives immediate feedback and respects minimum duration',async()=>{
 const dom=new JSDOM('<main><button data-gameplay-retry>立即重试</button></main>'),root=dom.window.document.querySelector('main'),button=root.querySelector('button'),tab={data:{matches:[]}},jobs=[];let now=0,release,requests=0;const events=[];
 const f=compile(game,['retryHistoryRegion'],{overviewContainer:()=>root,historyServiceState:()=>({kind:'official'}),loadOverview:async(...args)=>{requests++;assert.equal(args[3],true);return new Promise(r=>release=r)},window:{reportFlowDiagnostic:(...e)=>events.push(e)},Date:{now:()=>now},setTimeout:(fn,delay)=>{jobs.push({fn,delay});return jobs.length},clearTimeout(){}});
 const promise=f.retryHistoryRegion(tab);assert(button.disabled);assert.match(button.textContent,/正在重试/);assert.equal(requests,1);now=100;release(false);await new Promise(setImmediate);assert.equal(jobs[0].delay,500);now=600;jobs[0].fn();await promise;assert(!button.disabled);assert.equal(button.textContent,'仍未恢复');assert.equal(jobs[1].delay,2500);jobs[1].fn();assert.equal(button.textContent,'立即重试');assert(events.some(e=>e[0]==='history_outage_retry' && e[1]==='still_down'));dom.window.close();
});
test('R280 P7 title appears once, summary only in toolbar hint and read action styled',()=>{
 const api=require('./announcements.js');for(const heading of [true,false]){
 const dom=new JSDOM('<button id="announcements-button"></button><dialog id="announcements-dialog"><button data-announcements-close><svg/></button><div data-announcements-content></div></dialog>',{url:'http://fixture'}),w=dom.window;Object.defineProperty(w.document,'hidden',{value:false});w.setInterval=()=>1;const dialog=w.document.querySelector('dialog');dialog.showModal=function(){this.open=true};dialog.close=function(){this.open=false};
 api.mount(w,{visible:true,phase:'None',announcements:[{id:'one',revision:'1',popup:false,title:'单一标题',summary:'只在悬停',content:(heading?'## 单一标题\n':'')+'正文'}]}).open();assert.equal([...dialog.querySelectorAll('h2,h3,h4')].filter(n=>n.textContent==='单一标题').length,1);assert(!dialog.textContent.includes('只在悬停'));assert(dialog.querySelector('[data-announcement-read]').classList.contains('button-primary'));w.close();
 }
});
test('R280 P12 homogeneous tabs omit region text, mixed foreign tabs retain it and tooltip always includes region',()=>{
 for(const group of ['players','pro','kr'])for(const mixed of [false,true]){
 const dom=new JSDOM('<div></div>'),tabs=[{key:'a',label:'A',region:'kr'},{key:'b',label:'B',region:mixed?'jp1':'kr'}],state={tabs,settings:{},activeTabs:{[group]:'a'}};
 const f=compile(game,['renderPlayerTabWorkspace'],{state,overviewWorkspace:()=>({tabs:dom.window.document.querySelector('div')}),visiblePlayerTabs:()=>tabs,document:dom.window.document,tabServerLabel:t=>t.region==='kr'?'韩服':'日服',riotTab:()=>true,escapeHTML,requestAnimationFrame:fn=>fn(),prepareImages(){},updatePlayerTabScrollControls(){}});
 f.renderPlayerTabWorkspace(group);assert.equal(dom.window.document.querySelectorAll('.player-tab-region').length,group==='kr' && mixed?2:0);assert.match(dom.window.document.querySelector('.player-tab-name').dataset.tooltip,/韩服/);dom.window.close();
 }
});
test('R280 P2 three key states update wording, placeholder and clear availability',()=>{
 const dom=new JSDOM('<span></span><input><button></button>'),el={settingRiotKeyState:dom.window.document.querySelector('span'),settingRiotKeyInput:dom.window.document.querySelector('input'),settingRiotKeyClear:dom.window.document.querySelector('button')},state={};const f=compile(app,['renderRiotKeySettings'],{state,el});
 for(const [source,status,text,disabled] of [['embedded','configured','使用内置服务',true],['user','configured','已配置',false],['user','unverified','已保存，暂未验证',false]]){f.renderRiotKeySettings({source,status});assert.equal(el.settingRiotKeyState.textContent,text);assert.equal(el.settingRiotKeyClear.disabled,disabled);if(!disabled)assert.equal(el.settingRiotKeyInput.placeholder,'已保存，粘贴新的 Key 可替换');}dom.window.close();
});
