'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R273_APP_SOURCE || __dirname+'/app.js','utf8');
const facadeSource=fs.readFileSync(process.env.R273_FACADE_SOURCE || __dirname+'/favorites-facade.js','utf8');
function harness(view='owned',rarity=false,prestige=false){
 const dom=new JSDOM('<main id="grid"></main><main id="account"><section class="account-section"></section></main>'),doc=dom.window.document,grid=doc.querySelector('#grid'),frames=[];let clock=0;
 const state={section:'favorites',favoritesPage:'collection',view,sort:'name',descending:false,query:'',qualitySelections:new Set(rarity?['epic']:[]),loading:false,status:{connected:true,snapshotReady:true,collectionAccount:'anonymous'},renderGeneration:0,accountRequestSeq:1};
 const card=(item,kind='skin')=>{clock+=2;const n=doc.createElement('article');n.dataset.collectionId=String(item.id);n._collectionSkin=item;n.className=kind;if(kind==='mini')n.innerHTML='<span class="chroma-mini-art"></span>';return n;};
 const deps={document:doc,Event:dom.window.Event,window:{deepLegendsCollectionNewItems:{decorate(){clock+=2;}}},state,el:{grid,accountContent:doc.querySelector('#account'),listMeta:doc.createElement('div')},performance:{now:()=>clock},requestAnimationFrame:fn=>frames.push(fn),queueRenderFrame:fn=>frames.push(fn),queuePoolRenderFrame:fn=>frames.push(fn),cancelRenderFrames:()=>frames.splice(0),cancelDeferredImages:()=>{},stopHoverVideo:()=>{},visibleItems:()=>state.items,renderListMeta:()=>{},configureSparseGrid:()=>{},applyChromaVisibility:()=>{},formatNumber:String,escapeHTML,readableRarity:()=> 'epic',makeSkinCard:item=>card(item),makeChromaMiniCard:item=>card(item,'mini'),makeChromaCard:item=>card(item,'prestige'),navigationIcon:()=>'',enableHorizontalNavigation:()=>{},lootCategorySlug:()=> 'wards',lootCategoryIcon:()=>'',lootCard:item=>{clock+=2;return `<article data-loot-collection-id="${item.lootId}"></article>`;},loadNextLootImage:()=>{}};
 const names=['renderItems','reconcileCollectionCards','reconcileGroupedSkinCards','renderRarityGroups','renderChampionGroups','renderChromaCatalog','reconcileChromaCatalog','runRenderTasks','makeChromaStack','applyChromaGroupClasses','chromaVisibilityCounts','chromaSummaryAttributes','updateChromaSummary','reconcileAccountLoot'];
 const api=compile(source,names,deps);
 const items=(count,start=1)=>Array.from({length:count},(_,i)=>({id:start+i,name:'item '+i,championName:'hero',championId:101,parentSkinId:Math.floor(i/8)+1001,parentSkinName:'parent',owned:true,isPrestige:prestige}));
 const drain=()=>{let n=0;while(frames.length){assert(++n<500,'queued frames must finish');const before=clock;frames.shift()();assert(clock-before<=12,'later frame obeys time budget plus one card cost');}};
 return {dom,doc,grid,state,api,items,frames,drain,get clock(){return clock;}};
}
for(const [view,rarity,prestige] of [['owned',false,false],['all',false,false],['owned',true,false],['chromas',false,false],['chromas',false,true]])for(const count of [17,80]){
 test(`slow clock ${view}${rarity?'-rarity':''}${prestige?'-prestige':''} ${count}: empty and existing grid synchronously show first 40`,()=>{
  const h=harness(view,rarity,prestige);try{
   h.state.items=h.items(count);h.api.renderItems();assert.equal(h.grid.querySelectorAll('[data-collection-id]').length,Math.min(40,count*(prestige?2:1)),'empty grid first chunk');
   if(count>40){assert(h.frames.length,'rest must yield');const before=h.grid.querySelectorAll('[data-collection-id]').length;h.frames.shift()();assert(h.grid.querySelectorAll('[data-collection-id]').length-before<40,'later chunk keeps time budget');}
   h.drain();assert.equal(h.grid.querySelectorAll('[data-collection-id]').length,count*(prestige?2:1));
   h.state.items=h.items(count,10000);h.api.renderItems();
   assert.equal(h.grid.querySelectorAll('[data-collection-id]').length,Math.min(40,count*(prestige?2:1)),'existing grid first chunk');
   h.drain();assert.equal(h.grid.querySelectorAll('[data-collection-id]').length,count*(prestige?2:1));
   const retained=h.grid.querySelector('[data-collection-id="10000"]');h.state.items=h.state.items.map(i=>({...i,name:i.name+' updated'}));h.api.renderItems();h.drain();assert.equal(h.grid.querySelector('[data-collection-id="10000"]'),retained,'unchanged node retained');
  }finally{h.dom.window.close();}
 });
}
test('slow clock account loot first chunk is forty, later chunks yield and unchanged node survives',()=>{
 const h=harness();try{const items=Array.from({length:80},(_,i)=>({lootId:'ward-'+i,category:'wards'})),run=rows=>h.api.reconcileAccountLoot(rows,['wards'],'','',0,0,0,0,1);
 run(items);assert.equal(h.doc.querySelectorAll('[data-loot-collection-id]').length,40);h.drain();const retained=h.doc.querySelector('[data-loot-collection-id="ward-0"]');run([items[0],...items.slice(1).map(i=>({...i,lootId:i.lootId+'-new'}))]);assert.equal(h.doc.querySelectorAll('[data-loot-collection-id]').length,40);assert.equal(h.doc.querySelector('[data-loot-collection-id="ward-0"]'),retained);h.drain();assert.equal(h.doc.querySelectorAll('[data-loot-collection-id]').length,80);
 }finally{h.dom.window.close();}
});
for(const view of ['icons','banners'])test(`slow clock ${view} first chunk is forty for new and existing facade cards`,()=>{
 const dom=new JSDOM('<main></main>'),grid=dom.window.document.querySelector('main'),frames=[];let now=0;
 const state={connected:true,view,renderGeneration:0,[view]:{[view]:Array.from({length:80},(_,id)=>({id,owned:true}))}},deps={state,el:{grid,toolbar:{},listRow:{}},panelVisible:()=>true,ownershipUnavailable:()=>false,currentRows:()=>state[view][view],renderFacadeMeta:()=>{},facadeCollectionIconFields:()=>({}),facadeCollectionBannerFields:()=>({}),performance:{now:()=>now},requestAnimationFrame:fn=>frames.push(fn),window:{deepLegendsCollectionNewItems:{decorate(){now+=2;}}},refreshCard(){now+=2;},createCard(_fields,item){now+=2;const n=dom.window.document.createElement('article');n.className='skin-card';n.dataset.facadeKey=view+':'+item.id;return n;}};
 const {render}=compile(facadeSource,['render'],deps);const drain=()=>{while(frames.length){const before=now;frames.shift()();assert(now-before<=10,'later facade frame obeys time budget plus one card cost');}};
 try{render();assert.equal(grid.children.length,40);drain();const retained=grid.firstChild;state[view][view]=[state[view][view][0],...Array.from({length:79},(_,i)=>({id:i+1000,owned:true}))];render();assert.equal(grid.children.length,40);assert.equal(grid.firstChild,retained);drain();assert.equal(grid.children.length,80);}finally{dom.window.close();}
});
