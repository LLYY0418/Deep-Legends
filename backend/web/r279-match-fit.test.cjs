"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
function run(source,sameWidth=false,positionOnly=false){
 const observed=[],attrs=new Set(),frames=[];let cramped=false,insideObserver=false,frameId=0;
 const requestAnimationFrame=callback=>{frames.push(callback);return ++frameId;};
 const flush=()=>{while(frames.length)frames.shift()();};
 const icons={getBoundingClientRect:()=>({left:0,right:54,width:54,top:0,bottom:22,height:22})},kda={getBoundingClientRect:()=>({left:cramped?45:60,right:140,width:80,top:0,bottom:22,height:22})};
 const champion={clientWidth:124};
 const main={clientWidth:408,parentElement:{clientWidth:795},querySelector:s=>s==='.match-loadout-mini'?icons:kda,querySelectorAll:s=>[...(s.includes('.match-champion')?[champion]:[]),icons,kda],setAttribute:n=>{assert.equal(insideObserver,false,'icon correction must run outside ResizeObserver delivery');attrs.add(n)},removeAttribute:n=>attrs.delete(n)};
 const more={hidden:false,setAttribute(){}},items=[{hidden:false,getBoundingClientRect:()=>({width:20}),hasAttribute:()=>true}];
 const collection={clientWidth:168,checkVisibility:()=>true,closest:()=>({querySelector:()=>main}),querySelectorAll:()=>items,querySelector:()=>more};
 class ResizeObserver{constructor(callback){this.callback=()=>{insideObserver=true;try{callback()}finally{insideObserver=false}};observed.push(this)}observe(n){(this.targets ||= []).push(n)}}
 const f=compile(source,['fitMatchTags'],{ResizeObserver,requestAnimationFrame,bindMatchTagsPopover:()=>({anchor:null})}),container={querySelectorAll:()=>[collection]};
 f.fitMatchTags(container);assert.equal(attrs.has('data-icon-overlap'),false);
 cramped=true;if(positionOnly)champion.clientWidth=107;if(!sameWidth)main.clientWidth=367;
 const changed=positionOnly?champion:sameWidth?kda:main;
 if(observed[0].targets.includes(changed)){observed[0].callback();observed[0].callback();assert.equal(frames.length,1,'observer deliveries must coalesce into one correction frame');flush();}
 assert.equal(attrs.has('data-icon-overlap'),true,'internal grid changes must invalidate icon fit even when row and tag widths stay unchanged');
 assert(observed[0].targets.includes(main),'icon grid must be observed independently of tag collection');
 observed[0].callback();flush();assert.equal(attrs.has('data-icon-overlap'),true,'observer reentry must retain the correction');
 cramped=false;main.clientWidth=450;observed[0].callback();flush();assert.equal(attrs.has('data-icon-overlap'),false,'recovered grid must clear overlap adjustment');
}
test('match icon fitting follows internal grid width independently of tag width',()=>run(source));
test('font or text geometry changes at the same main width are still measured',()=>run(source,true));
test('mutation: retaining the tag-width early return must fail icon overlap target',()=>{
 const broken=source.replace('const main=collection.closest?', 'if(collection._tagFitWidth===collection.clientWidth)return;const main=collection.closest?');
 assert.notEqual(broken,source);assert.throws(()=>run(broken),/internal grid changes must invalidate icon fit/);
});

test('mutation: omitting child geometry observations must fail same-width overlap target',()=>{
 const broken=source.replace("for(const target of main.querySelectorAll('.match-champion,.match-loadout-mini,.match-kda'))collection._tagObserver.observe(target);",'');
 assert.notEqual(broken,source);assert.throws(()=>run(broken,true),/internal grid changes must invalidate icon fit/);
});

test('container track changes are measured when tag/main/KDA/loadout dimensions stay unchanged',()=>run(source,true,true));
test('mutation: omitting champion track observation must fail position-only overlap target',()=>{
 const broken=source.replace("main.querySelectorAll('.match-champion,.match-loadout-mini,.match-kda')","main.querySelectorAll('.match-loadout-mini,.match-kda')");
 assert.notEqual(broken,source);assert.throws(()=>run(broken,true,true),/internal grid changes must invalidate icon fit/);
});

test('mutation: synchronous observer writes must fail the error-free correction target',()=>{
 const anchor=`new ResizeObserver(()=>{
          if(collection._tagFitFrame)return;
          collection._tagFitFrame=requestAnimationFrame(()=>{collection._tagFitFrame=0;fit();});
        })`;
 const broken=source.replace(anchor,'new ResizeObserver(fit)');
 assert.notEqual(broken,source);assert.throws(()=>run(broken,true,true),/icon correction must run outside ResizeObserver delivery/);
});

function detachedObserverScenario(source) {
 const observers=[],frames=[];
 const collection={isConnected:true,clientWidth:100,checkVisibility(){return this.isConnected;},closest:()=>null,querySelectorAll:()=>[],querySelector:()=>null};
 class ResizeObserver{constructor(callback){this.callback=callback;this.disconnected=false;observers.push(this)}observe(){}disconnect(){this.disconnected=true}}
 const f=compile(source,['fitMatchTags'],{ResizeObserver,requestAnimationFrame:fn=>{frames.push(fn);return frames.length},bindMatchTagsPopover:()=>({anchor:null})});
 const root={querySelectorAll:()=>[collection]};f.fitMatchTags(root);
 assert.equal(observers.length,1);collection.isConnected=false;observers[0].callback();while(frames.length)frames.shift()();
 assert.equal(observers[0].disconnected,true,'detached tag observer must disconnect instead of retaining virtualized card nodes');
 assert.equal(collection._tagObserver,undefined,'detached observer reference must be released');
 collection.isConnected=true;f.fitMatchTags(root,true);assert.equal(observers.length,2,'restored cached card must acquire a fresh geometry observer');
}
test('virtualized or removed tag collections release their observer and cached cards can rebind',()=>detachedObserverScenario(source));
test('mutation: retaining observations on detached cards must fail the lifecycle target',()=>{
 const broken=source.replace('if(collection.isConnected===false) {','if(false) {');
 assert.notEqual(broken,source);assert.throws(()=>detachedObserverScenario(broken),/detached tag observer must disconnect/);
});

function virtualizationCleanupScenario(source) {
 const frames=[],observers=[],canceled=[];let disconnected=false,removedAfterCleanup=false;
 const tag={_tagObserver:{disconnect(){disconnected=true}},_tagFitFrame:123};
 const entry={isConnected:true,currentCSSZoom:1,dataset:{matchId:'one'},style:{},getBoundingClientRect:()=>({height:120}),hasAttribute:()=>false,querySelectorAll:()=>[tag],set innerHTML(value){assert(disconnected,'virtualization must release tag observations before replacing card children');removedAfterCleanup=true;}};
 const list={dataset:{},closest:()=>({}),querySelectorAll:()=>[entry]};
 class IntersectionObserver{constructor(callback,options){this.callback=callback;this.options=options;observers.push(this)}observe(){}disconnect(){}}
 const f=compile(source,['boundMatchDOM'],{window:{IntersectionObserver},document:{},requestAnimationFrame:fn=>{frames.push(fn);return frames.length},cancelAnimationFrame:id=>canceled.push(id),Date:{now:()=>0}});
 f.boundMatchDOM(list,{openMatches:new Set()});observers.find(o=>o.options.rootMargin==='3200px 0px').callback([{target:entry,isIntersecting:false}]);while(frames.length)frames.shift()();
 assert(removedAfterCleanup);assert.equal(tag._tagObserver,undefined);assert.equal(tag._tagFitFrame,0);assert.deepEqual(canceled,[123]);
}
test('virtualization releases hidden-card tag observations even without a ResizeObserver delivery',()=>virtualizationCleanupScenario(source));
test('mutation: retaining hidden-card observations must fail before virtualization replaces DOM',()=>{
 const cleanup='          releaseMatchTagObservers(entry);\n';
 const broken=source.replace(cleanup,'');assert.notEqual(broken,source);
 assert.throws(()=>virtualizationCleanupScenario(broken),/virtualization must release tag observations/);
});

function disposalFixture(source,mode) {
 const {JSDOM}=require('../../desktop/node_modules/jsdom');
 const dom=new JSDOM('<main><div class="match-filterbar"></div><div class="match-list"><article class="match-entry" data-match-id="1"><span class="match-tags-collection"></span></article><article class="match-entry" data-match-id="2"></article></div></main>');
 const d=dom.window.document,root=d.querySelector('main'),list=root.querySelector('.match-list'),old=list.firstChild,kept=list.lastChild,tag=old.firstChild,canceled=[];
 let released=false,changed=false;tag._tagObserver={disconnect(){released=true}};tag._tagFitFrame=19;
 const rows=[{gameId:1,revision:'old'},{gameId:2,revision:'kept'}];list._matchData=new Map(rows.map(m=>[String(m.gameId),m]));
 const tab={data:{matches:mode==='replace'?[{gameId:1,revision:'new'},rows[1]]:[rows[1]]},openMatches:new Set(),matchViews:new Map()};
 const deps={document:d,window:dom.window,globalThis:{deepLegendsHistoryFilters:null},cancelAnimationFrame:id=>canceled.push(id),filteredMatches:rows=>rows,bindOverviewContent(){},applyRenderedMetricStyles(){},prepareImages(){},overviewContainer:()=>root,renderMatch:m=>`<article class="match-entry" data-match-id="${m.gameId}"></article>`};
 let name='reconcileFilteredMatchList';
 if(mode==='stash'){name='stashMatchFilterDOM';const native=list.replaceWith.bind(list);list.replaceWith=(...args)=>{assert(released,'cache stash must release tag observers before detaching its list');changed=true;native(...args)};}
 else if(mode==='dispose')name='disposeMatchRowHeight';
 else {const method=mode==='replace'?'replaceWith':'remove',native=old[method].bind(old);old[method]=(...args)=>{assert(released,'ordinary filtering must release tag observers before removing a card');changed=true;native(...args)};}
 try {
  const f=compile(source,[name],deps);
  if(mode==='stash'){const view={};f.stashMatchFilterDOM(tab,view);assert.equal(view.list,list);assert.equal(view.list.firstChild,old,'cached card DOM identity must remain unchanged');}
  else if(mode==='dispose')f.disposeMatchRowHeight(root);
  else {f.reconcileFilteredMatchList(list,tab);assert.equal(list.lastChild,kept,'unaffected card must retain DOM identity');}
  assert(released,'full view disposal must release tag observers even without delivery');assert.equal(tag._tagObserver,undefined);assert.equal(tag._tagFitFrame,0);assert.deepEqual(canceled,[19]);if(mode!=='dispose')assert(changed);
 }finally{dom.window.close();}
}
for(const mode of ['remove','replace'])test('ordinary filtering releases observers before '+mode+' without notification',()=>disposalFixture(source,mode));
test('cache stash releases observations while preserving the retained card nodes',()=>disposalFixture(source,'stash'));
test('full view disposal releases tag observations without a ResizeObserver delivery',()=>disposalFixture(source,'dispose'));
test('mutation: removing the filter disposal hook must fail before a card leaves the list',()=>{
 const broken=source.replaceAll('releaseMatchTagObservers(entry);','');assert.notEqual(broken,source);assert.throws(()=>disposalFixture(broken,'remove'),/ordinary filtering must release/);
});
test('mutation: removing the stash disposal hook must fail before the list is detached',()=>{
 const broken=source.replace('releaseMatchTagObservers(list);','');assert.notEqual(broken,source);assert.throws(()=>disposalFixture(broken,'stash'),/cache stash must release/);
});
test('mutation: removing full-view disposal must fail the observation release target',()=>{
 const broken=source.replace('releaseMatchTagObservers(root);','');assert.notEqual(broken,source);assert.throws(()=>disposalFixture(broken,'dispose'),/full view disposal must release/);
});
function initiallyDetached(source){let created=0;const collection={isConnected:false,querySelectorAll:()=>[]};class ResizeObserver{constructor(){created++}observe(){}}const f=compile(source,['fitMatchTags'],{ResizeObserver,bindMatchTagsPopover:()=>({anchor:null})});f.fitMatchTags({querySelectorAll:()=>[collection]});assert.equal(created,0,'first fit on detached collections must not create a new observation');}
test('first fit on detached collections does not allocate new observations',()=>initiallyDetached(source));
test('mutation: observing a detached cached list must fail the allocation target',()=>{const broken=source.replace('collection.isConnected!==false && ','');assert.notEqual(broken,source);assert.throws(()=>initiallyDetached(broken),/first fit on detached collections/);});

function virtualizerDisposal(source){
 const {JSDOM}=require('../../desktop/node_modules/jsdom'),dom=new JSDOM('<div class="match-list"><article class="match-entry" data-match-id="1"><span class="match-tags-collection"></span></article></div>');
 const list=dom.window.document.querySelector('.match-list'),entry=list.firstChild,observers=[],frames=new Map();let serial=0;entry.getBoundingClientRect=()=>({height:120});
 class IntersectionObserver{constructor(callback,options){this.callback=callback;this.options=options;observers.push(this)}observe(){}disconnect(){this.disconnected=true}}
 try{const f=compile(source,['boundMatchDOM','disposeMatchRowHeight'],{window:{IntersectionObserver},document:dom.window.document,requestAnimationFrame:fn=>{frames.set(++serial,fn);return serial},cancelAnimationFrame:id=>frames.delete(id),Date:{now:()=>0}});f.boundMatchDOM(list,{openMatches:new Set()});observers.find(o=>o.options.rootMargin==='3200px 0px').callback([{target:entry,isIntersecting:false}]);assert.equal(frames.size,1);f.disposeMatchRowHeight(list);assert(observers.every(o=>o.disconnected),'view disposal must disconnect both virtualizer observations');assert.equal(frames.size,0,'view disposal must cancel the pending virtualizer frame');assert.equal(list._virtualObserver,undefined);}finally{dom.window.close();}
}
test('view disposal cancels pending virtualization and disconnects both observers',()=>virtualizerDisposal(source));
test('mutation: retaining the virtualizer must fail its disposal target',()=>{const broken=source.replace('list._virtualObserver?.disconnect();delete list._virtualObserver;','');assert.notEqual(broken,source);assert.throws(()=>virtualizerDisposal(broken),/view disposal must disconnect/);});
function evictedFilterView(source){
 const {JSDOM}=require('../../desktop/node_modules/jsdom'),dom=new JSDOM('<main><div class="match-list"></div></main>'),root=dom.window.document.querySelector('main'),evicted=dom.window.document.createElement('div');evicted.className='match-list';let disconnected=false;evicted._virtualObserver={disconnect(){disconnected=true}};
 const tab={matchViews:new Map([['oldest',{list:evicted}],['b',{}],['c',{}],['d',{}],['current',{}]])};
 try{const f=compile(source,['stashMatchFilterDOM'],{document:dom.window.document,overviewContainer:()=>root});f.stashMatchFilterDOM(tab,tab.matchViews.get('current'));assert.equal(tab.matchViews.size,4);assert(!tab.matchViews.has('oldest'));assert(disconnected,'evicted filter view must dispose its virtualizer');assert.equal(evicted._virtualObserver,undefined);}finally{dom.window.close();}
}
test('filter view LRU eviction disposes its layout observers',()=>evictedFilterView(source));
test('mutation: deleting a retained view without disposal must fail the LRU target',()=>{const broken=source.replace('disposeMatchRowHeight(tab.matchViews.get(oldest)?.list);','');assert.notEqual(broken,source);assert.throws(()=>evictedFilterView(broken),/evicted filter view must dispose/);});
