'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,compile}=require('./r188-harness.cjs');
const source=read('champions.js'),tick=()=>new Promise(setImmediate);
function atlasHarness(cached){const state={mode:'aram-mayhem',mayhemView:'atlas',mayhemAugmentRequestToken:0,mayhemAugmentID:2,augments:{rows:[{id:1},{id:2,key:'second'}]},mayhemAugmentDetail:null,mayhemAugmentDetailCache:new Map(cached?[[2,cached]]:[]),fitStage:'2',fitQuery:'保留',atlasTab:'items'},reports=[],requests=[];let complete;
 const f=compile(source,['loadMayhemAtlas','loadMayhemAugmentDetail'],{state,objectRows:v=>v||[],render(){},performance:{now:()=>10},window:{reportFlowDiagnostic:(...args)=>reports.push(args)},api:path=>{requests.push(path);return new Promise(r=>complete=r);}});
 return {state,reports,requests,...f,complete:v=>complete(v)};}
test('R309 atlas re-entry recovers cached counts and best card data without resetting selection or tab',async()=>{const detail={champions:[{id:1,games:300}],combinations:[{games:400}],items:[{games:500}]},h=atlasHarness(detail);await h.loadMayhemAtlas();assert.equal(h.state.mayhemAugmentDetail,detail);assert.equal(h.state.mayhemAugmentID,2);assert.equal(h.state.fitStage,'2');assert.equal(h.state.atlasTab,'items');assert.equal(h.requests.length,0);assert.equal(h.reports[0][2].source,'cache');});
test('R309 atlas re-entry during an old read discards stale response and refetches selected detail',async()=>{const h=atlasHarness();await h.loadMayhemAtlas();assert.equal(h.requests.length,1);// Leaving invalidates the request token, as resetTransientChampionState does.
 h.state.mayhemAugmentRequestToken++;h.state.mayhemAugmentLoading=false;h.state.mayhemAugmentDetail=null;
 h.complete({champions:['stale']});await tick();assert.equal(h.state.mayhemAugmentDetail,null);assert.equal(h.reports[0][2].result,'stale');
 await h.loadMayhemAtlas();assert.equal(h.requests.length,2);const detail={champions:[1,2],combinations:[3],items:[4]};h.complete(detail);await tick();await tick();assert.equal(h.state.mayhemAugmentDetail,detail);assert.equal(h.state.mayhemAugmentDetailCache.get(2),detail);assert.equal(h.reports.at(-1)[2].result,'ok');});
test('R309 ARAMKit redraw preserves outside DOM and scroll; loaded images bypass queue and placeholder',()=>{
 const {JSDOM}=require('../../desktop/node_modules/jsdom'),d=new JSDOM('<main><aside id="list"><button>英雄</button></aside><header id="banner"></header><nav id="tabs"></nav><div data-mayhem-detail-panels><div data-mayhem-detail-panel="build"></div></div></main>').window.document;
 const root=d.querySelector('main'),list=d.querySelector('#list'),button=list.firstChild,banner=d.querySelector('#banner'),tabs=d.querySelector('#tabs'),host=d.querySelector('[data-mayhem-detail-panel]'),panels=host.parentElement;panels.scrollTop=123;root.scrollTop=56;
 const state={detail:{items:1}};let prepared;
 const f=compile(source,['redrawARAMKitBuild','assetImage','prepareImages'],{root,state,mayhemBuildTabMarkup:()=>'<div class="equipment">图标</div>',window:{},applyRenderedMetricStyles(){},imageURL:()=>'/known.png',escapeHTML:String});
 f.redrawARAMKitBuild();assert(list.isSameNode(d.querySelector('#list')));assert(button.isSameNode(list.firstChild));assert(banner.isSameNode(d.querySelector('#banner')));assert(tabs.isSameNode(d.querySelector('#tabs')));assert.equal(panels.scrollTop,123);assert.equal(root.scrollTop,56);
 host.innerHTML=f.assetImage({name:'装备'});const image=host.querySelector('img');image.setAttribute('src','/known.png');f.prepareImages(host);image.dispatchEvent(new d.defaultView.Event('load'));host.innerHTML=f.assetImage({name:'装备'});assert.equal(host.querySelector('img').getAttribute('src'),'/known.png');assert.equal(host.querySelector('img').hasAttribute('data-queued-src'),false);assert(host.querySelector('.champion-asset').classList.contains('has-loaded-image'));d.defaultView.close();
});
