'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{extract}=require('./r188-harness.cjs');
const root=process.env.R315_SOURCE||__dirname,read=n=>fs.readFileSync(path.join(root,n),'utf8');
for(const kind of ['global','auto-config','season-note','match-tags','mastery','mmr'])for(const scenario of ['remove','hidden','section','blur','pointermove','outside','escape','window-exit','visibility'])test(`R315 ${kind} closes within a frame on ${scenario}`,async()=>{
 const dom=new JSDOM('<main id="panel"><div id="suite-config-root"></div></main><aside id="blank"></aside>',{url:'http://localhost',pretendToBeVisual:true,runScripts:'outside-only'}),w=dom.window,d=w.document;w.matchMedia=()=>({matches:false});w.deepLegendsRuntime={escapeHTML:v=>String(v)};w.deepLegendsSections={register(){}};
 let frames=new Map(),seq=0;w.requestAnimationFrame=fn=>{frames.set(++seq,fn);return seq};w.cancelAnimationFrame=id=>frames.delete(id);
 const frame=async()=>{await Promise.resolve();const jobs=[...frames.values()];frames.clear();jobs.forEach(fn=>fn());await Promise.resolve();};
 try{
  const app=read('app.js');if(app.includes('function createFloatingLayerLifecycle'))w.eval(extract(app,'createFloatingLayerLifecycle')+';window.deepLegendsFloatingLayers=createFloatingLayerLifecycle();');
  let anchor,panelSelector;
  if(kind==='global'){w.eval(extract(app,'setupFloatingTooltips')+';setupFloatingTooltips();');d.querySelector('#panel').insertAdjacentHTML('beforeend','<button data-tooltip="Title\nBody">anchor</button>');anchor=d.querySelector('[data-tooltip]');panelSelector='#global-tooltip';}
  if(kind==='auto-config'){d.querySelector('#suite-config-root').innerHTML='<button data-ac-tooltip="Rune\nDetail">anchor</button>';w.eval(read('auto-config.js'));anchor=d.querySelector('[data-ac-tooltip]');panelSelector='#auto-config-tooltip';}
  if(kind==='season-note'){d.querySelector('#panel').insertAdjacentHTML('beforeend','<button data-season-data-note=\'["Title","Detail"]\'>anchor</button>');w.eval(extract(read('gameplay.js'),'bindSeasonDataNotes')+';bindSeasonDataNotes(document.querySelector("main"));');anchor=d.querySelector('[data-season-data-note]');panelSelector='#season-data-note-popover';}
  if(kind==='match-tags'){d.querySelector('#panel').insertAdjacentHTML('beforeend','<div><span data-match-tag hidden>Tag</span><button data-match-tags-more>+1</button></div>');w.eval(extract(read('gameplay.js'),'bindMatchTagsPopover')+';bindMatchTagsPopover(document.querySelector("main"));');anchor=d.querySelector('[data-match-tags-more]');panelSelector='#match-tags-popover';}
  if(kind==='mastery'){d.querySelector('#panel').insertAdjacentHTML('beforeend','<button data-mastery-detail="1">anchor</button>');w.masteryTooltipContent=()=>'<strong>Mastery</strong>';w.prepareImages=()=>{};w.eval(extract(read('gameplay.js'),'bindMasteryDetails')+';bindMasteryDetails(document.querySelector("main"),{items:[{championId:1}]});');anchor=d.querySelector('[data-mastery-detail]');panelSelector='.mastery-details-popover';}
  if(kind==='mmr'){d.querySelector('#panel').insertAdjacentHTML('beforeend','<div class="rank-mmr-control"><button class="rank-mmr-trigger">MMR</button><div class="rank-mmr-popover" role="tooltip">Detail</div></div>');anchor=d.querySelector('.rank-mmr-trigger');panelSelector='.rank-mmr-popover';}
  anchor.dispatchEvent(new w.MouseEvent(kind==='auto-config'?'mouseover':kind==='mastery'?'mouseenter':'pointerover',{bubbles:true,relatedTarget:d.querySelector('#blank')}));await frame();
  const layer=d.querySelector(panelSelector);assert(layer&&!layer.hidden,`opened ${kind}`);if(kind==='mmr')assert(anchor.closest('.rank-mmr-control').classList.contains('is-open'));
  if(scenario==='remove')anchor.remove();if(scenario==='hidden')d.querySelector('#panel').hidden=true;
  if(scenario==='section')w.dispatchEvent(new w.CustomEvent('deep-legends:section',{detail:{name:'champions'}}));if(scenario==='blur')w.dispatchEvent(new w.Event('blur'));
  if(scenario==='pointermove')d.querySelector('#blank').dispatchEvent(new w.MouseEvent('pointermove',{bubbles:true,clientX:900,clientY:900}));
  if(scenario==='outside')d.querySelector('#blank').dispatchEvent(new w.MouseEvent('pointerdown',{bubbles:true}));
  if(scenario==='escape')d.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));
  if(scenario==='window-exit')d.dispatchEvent(new w.MouseEvent('pointerout',{bubbles:true,relatedTarget:null}));
  if(scenario==='visibility'){Object.defineProperty(d,'visibilityState',{value:'hidden',configurable:true});d.dispatchEvent(new w.Event('visibilitychange'));}
  await frame();assert(!anchor.hasAttribute('aria-describedby'),'ARIA cleaned');assert(!layer.isConnected||layer.hidden||kind==='mmr'&&!d.querySelector('.rank-mmr-control').classList.contains('is-open'),'layer closed');
 }finally{w.deepLegendsFloatingLayers?.closeAll();d._seasonDataNotes?.close();d._matchTagsPopover?.close();d._masteryDetails?.close();dom.window.close()}
});

test('R315 shared lifecycle keeps an interactive layer under the pointer and a keyboard layer until focus leaves',()=>{
 const dom=new JSDOM('<button id="a">Anchor</button><div id="p"><button id="inside">Action</button></div><button id="outside">Elsewhere</button>',{url:'http://localhost',pretendToBeVisual:true,runScripts:'outside-only'}),w=dom.window,d=w.document;
 let jobs=new Map(),seq=0;w.requestAnimationFrame=fn=>{jobs.set(++seq,fn);return seq};w.cancelAnimationFrame=id=>jobs.delete(id);const frame=()=>{const run=[...jobs.values()];jobs.clear();run.forEach(fn=>fn())};
 try{w.eval(extract(read('app.js'),'createFloatingLayerLifecycle')+';window.layers=createFloatingLayerLifecycle();');let closed=0;const a=d.querySelector('#a'),p=d.querySelector('#p');
 const stop=w.layers.watch(a,p,()=>closed++);d.querySelector('#inside').dispatchEvent(new w.MouseEvent('pointermove',{bubbles:true}));frame();assert.equal(closed,0);stop();
 a.focus();assert(a.matches(':focus-visible'));w.layers.watch(a,p,()=>closed++);d.querySelector('#outside').dispatchEvent(new w.MouseEvent('pointermove',{bubbles:true}));frame();assert.equal(closed,0);d.querySelector('#outside').focus();frame();assert.equal(closed,1);
 }finally{w.layers?.closeAll();w.close();}
});
test('R315 mastery rebinds share one document controller and never retain an earlier card layer',()=>{
 const dom=new JSDOM('<main><button data-mastery-detail="1">A</button><button data-mastery-detail="2">B</button></main>',{url:'http://localhost',pretendToBeVisual:true,runScripts:'outside-only'}),w=dom.window,d=w.document;
 try{w.masteryTooltipContent=item=>`<strong>${item.championId}</strong>`;w.prepareImages=()=>{};w.eval(extract(read('gameplay.js'),'bindMasteryDetails')+';window.bind=bindMasteryDetails;');w.bind(d.querySelector('main'),{items:[{championId:1},{championId:2}]});const controller=d._masteryDetails;w.bind(d.querySelector('main'),{items:[{championId:1},{championId:2}]});assert.equal(d._masteryDetails,controller);
 const [a,b]=d.querySelectorAll('button');a.dispatchEvent(new w.Event('mouseenter'));b.dispatchEvent(new w.Event('mouseenter'));assert.equal(d.querySelectorAll('.mastery-details-popover').length,1);assert.equal(d.querySelector('.mastery-details-popover').textContent,'2');assert(!a.hasAttribute('aria-describedby'));a.dispatchEvent(new w.Event('mouseleave'));assert.equal(d.querySelectorAll('.mastery-details-popover').length,1);b.dispatchEvent(new w.Event('mouseleave'));assert.equal(d.querySelector('.mastery-details-popover'),null);assert(!b.hasAttribute('aria-describedby'));
 }finally{d._masteryDetails?.close();w.close();}
});
