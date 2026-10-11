'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{create}=require('./collection-new-items.js'),{compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R272_APP_SOURCE || __dirname+'/app.js','utf8');
test('new item baseline, hover read and restart markers are account local',()=>{
 const dom=new JSDOM('<article></article>',{url:'http://localhost'}),storage=dom.window.localStorage,account={connected:true,collectionAccount:'salted-account',clientRegion:'KR'};
 let tracker=create(storage);tracker.setAccount(account);tracker.update('skins',[{id:1,owned:true}]);assert.equal(tracker.isNew('skins',1),false);
 tracker.update('skins',[{id:1,owned:true},{id:2,owned:true}]);assert.equal(tracker.isNew('skins',2),true);
 const card=dom.window.document.querySelector('article');tracker.decorate(card,'skins',2);assert.equal(card.classList.contains('is-new-collection-item'),true);card.dispatchEvent(new dom.window.Event('mouseenter'));
 tracker=create(storage);tracker.setAccount(account);tracker.update('skins',[{id:2,owned:true,newItem:true}]);assert.equal(tracker.isNew('skins',2),false);
 tracker.setAccount({...account,collectionAccount:'other-account'});tracker.update('skins',[{id:2,owned:true}]);assert.equal(tracker.isNew('skins',2),false);dom.window.close();
});
test('skin and chroma card constructors use the correct new-item ID',()=>{
 const dom=new JSDOM('<template><article><video></video><img><span class="image-fallback"></span><span class="skin-state"></span><strong></strong><span class="skin-hero"></span><span class="skin-meta"></span></article></template>');
 const calls=[],state={view:'owned',items:[]},deps={el:{template:dom.window.document.querySelector('template')},state,window:{deepLegendsCollectionNewItems:{decorate:(card,kind,id)=>calls.push({kind,id})}},deferCardImageSources:()=>{},skinImageSources:()=>[],prepareSkinVideo:()=>{},skinVideoPath:()=>'',readableRarity:()=>'',openSkinDetails:()=>{},chromaImageSources:()=>[],openChromaDetails:()=>{},bindChromaCardHover:()=>{},escapeHTML:String,formatNumber:String};
 const fn=compile(source,['makeSkinCard','makeChromaCard'],deps);
 const skin=fn.makeSkinCard({id:1001,name:'synthetic'});assert.equal(skin.dataset.collectionId,'1001');assert.equal(skin._collectionSkin.id,1001);
 const chroma=fn.makeChromaCard({id:2001,name:'synthetic'},[]);assert.equal(chroma.dataset.collectionId,'2001');assert.deepEqual(calls,[{kind:'skins',id:1001},{kind:'chromas',id:2001}]);dom.window.close();
});
test('chroma ownership refresh retains mounted cards and updates click data and unread markers',()=>{
 const dom=new JSDOM('<main></main>',{url:'http://localhost'}),grid=dom.window.document.querySelector('main'),tracker=create(dom.window.localStorage);tracker.setAccount({connected:true,collectionAccount:'anonymous'});
 const first=[{id:1,championId:103,parentSkinId:103001,name:'ordinary',championName:'hero',owned:true},{id:2,championId:103,parentSkinId:103001,name:'new',championName:'hero',owned:false}];tracker.update('chromas',first);
 const deps={Event:dom.window.Event,document:dom.window.document,window:{deepLegendsCollectionNewItems:tracker},el:{grid},state:{renderGeneration:1},runRenderTasks:tasks=>tasks.forEach(task=>task()),formatNumber:String,escapeHTML:String,enableHorizontalNavigation:()=>{},deferCardImageSources:()=>{},chromaImageSources:()=>[],openChromaDetails:()=>{},navigationIcon:()=>''};
 const names=['renderChromaCatalog','reconcileChromaCatalog','makeChromaStack','makeChromaMiniCard','chromaDisplayName','updateChromaSummary','chromaSummaryAttributes','chromaVisibilityCounts','applyChromaGroupClasses'];
 const fn=compile(source,names,deps);fn.renderChromaCatalog(first,1);const retained=grid.querySelector('[data-collection-id="1"]'),newCard=grid.querySelector('[data-collection-id="2"]');
 const next=first.map(item=>({...item,owned:true}));tracker.update('chromas',next);fn.reconcileChromaCatalog(next,1);assert.equal(grid.querySelector('[data-collection-id="1"]'),retained);assert.equal(grid.querySelector('[data-collection-id="2"]'),newCard);assert.equal(newCard.classList.contains('is-locked'),false);assert.equal(newCard.classList.contains('is-new-collection-item'),true);assert.equal(newCard._collectionSkin,next[1]);newCard.dispatchEvent(new dom.window.Event('mouseenter'));assert.equal(tracker.isNew('chromas',2),false);dom.window.close();
});
test('a reused card hover acknowledges its current account only',()=>{
 const dom=new JSDOM('<article></article>',{url:'http://localhost'}),tracker=create(dom.window.localStorage),card=dom.window.document.querySelector('article');
 for(const account of ['a','b']){tracker.setAccount({connected:true,collectionAccount:account});tracker.update('wards',[{id:1,owned:true}]);tracker.update('wards',[{id:2,owned:true}]);tracker.decorate(card,'wards',2);}
 card.dispatchEvent(new dom.window.Event('mouseenter'));assert.equal(tracker.isNew('wards',2),false);tracker.setAccount({connected:true,collectionAccount:'a'});assert.equal(tracker.isNew('wards',2),true);dom.window.close();
});

test('account inventory refresh retains unchanged cards and marks newly owned ward',()=>{
 const dom=new JSDOM('<main><section class="account-section"><div class="empty-state"></div></section></main>',{url:'http://localhost'}),content=dom.window.document.querySelector('main'),tracker=create(dom.window.localStorage);tracker.setAccount({connected:true,collectionAccount:'anonymous'});
 const initial=[{lootId:'ward-1',category:'守卫',owned:true}];tracker.update('loot-wards',initial.map(i=>({id:i.lootId,owned:i.owned})));
 const state={accountRequestSeq:1},deps={document:dom.window.document,window:{deepLegendsCollectionNewItems:tracker},performance:{now:()=>0},requestAnimationFrame:fn=>fn(),state,el:{accountContent:content},formatNumber:String,escapeHTML:String,lootCategorySlug:()=> 'wards',lootCategoryIcon:()=>'',lootCard:item=>`<article data-loot-collection-id="${item.lootId}">${item.lootId}</article>`,loadNextLootImage:()=>{}};
 const {reconcileAccountLoot}=compile(source,['reconcileAccountLoot'],deps),render=items=>reconcileAccountLoot(items,['守卫'],'','',0,0,0,0,1);render(initial);const retained=content.querySelector('[data-loot-collection-id="ward-1"]');
 const next=[...initial,{lootId:'ward-2',category:'守卫',owned:true}];tracker.update('loot-wards',next.map(i=>({id:i.lootId,owned:i.owned})));render(next);assert.equal(content.querySelector('[data-loot-collection-id="ward-1"]'),retained);const fresh=content.querySelector('[data-loot-collection-id="ward-2"]');assert(fresh.classList.contains('is-new-collection-item'));fresh.dispatchEvent(new dom.window.Event('mouseenter'));assert.equal(tracker.isNew('loot-wards','ward-2'),false);dom.window.close();
});

test('same-sync account switch invalidates pending collection and account generations',()=>{
 const state={collectionAccount:'a',collectionSessionEpoch:7,skinLoadGeneration:3,accountRequestSeq:4,renderGeneration:5,items:[{id:1}],skinsCache:new Map([['owned',{}]])},el={grid:{_collectionRenderKey:'a'},accountContent:{_accountMarkup:'a'}};
 compile(source,['syncCollectionSession'],{state,el}).syncCollectionSession({collectionAccount:'b',clientSessionEpoch:7});assert.equal(state.skinLoadGeneration,4);assert.equal(state.accountRequestSeq,5);assert.equal(state.items.length,0);assert.equal(state.skinsCache.size,0);assert.equal(el.grid._collectionRenderKey,undefined);
});

test('same account changing server invalidates generations and updates tracker before hover',()=>{
 const calls=[],state={collectionAccount:'a',collectionScope:'KR||a',collectionSessionEpoch:7,skinLoadGeneration:3,accountRequestSeq:4,renderGeneration:5,items:[{id:1}],skinsCache:new Map([['owned',{}]])},el={grid:{_collectionRenderKey:'a'},accountContent:{_accountMarkup:'a'}};
 compile(source,['syncCollectionSession'],{state,el,window:{deepLegendsCollectionNewItems:{setAccount:status=>calls.push(status.serverId)}}}).syncCollectionSession({connected:true,collectionAccount:'a',clientRegion:'KR',serverId:'KR2',clientSessionEpoch:7});
 assert.equal(state.skinLoadGeneration,4);assert.equal(state.accountRequestSeq,5);assert.equal(state.items.length,0);assert.deepEqual(calls,['KR2']);
});
test('old server skin HTTP response cannot enter current collection even before status invalidation',async()=>{
 let resolve,applied=0;const state={status:{connected:true,snapshotReady:true,collectionAccount:'a',clientRegion:'KR',serverId:'KR1'},skinLoadGeneration:3,items:[{id:1}],view:'owned',skinsCache:new Map()},el={retryList:{hidden:true}};
 const deps={state,el,api:()=>new Promise(r=>resolve=r),renderItems:()=>{},ensureCollection:()=>{},applySkinsPayload:()=>applied++,sameCollectionItems:()=>false,window:{}};
 const flight=compile(source,['loadSkins'],deps).loadSkins(true);state.status={...state.status,serverId:'KR2'};resolve({items:[{id:2}]});await flight;
 assert.equal(applied,0);assert.equal(state.skinsCache.size,0);assert.equal(state.items[0].id,1);
});
