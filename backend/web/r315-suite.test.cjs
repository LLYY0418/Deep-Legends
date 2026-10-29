'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
test('R315 tools start on automatic, ignore old preferences, retain runtime tabs/scroll, and honor explicit navigation',()=>{
 const dom=new JSDOM(fs.readFileSync(__dirname+'/default/index.html','utf8'),{url:'http://localhost',runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window;
 try{
  w.deepLegendsRuntime={escapeHTML:String};w.deepLegendsSections={register(){},listen(){}};
  w.localStorage.setItem('lol-loot-suite-tab','champselect');w.localStorage.setItem('lol-loot-suite-scroll','{"watch":500}');
  w.requestAnimationFrame=fn=>{fn();return 0};w.document.querySelector('#app-scroll').scrollTo=function({top}){this.scrollTop=top};
  w.eval(fs.readFileSync(__dirname+'/suite.js','utf8').replace('  setupTabs();','  window.__suite={state,activateTab,handleLazySection};setupTabs();'));
  const {state,activateTab,handleLazySection}=w.__suite;assert.equal(state.tab,'watch');assert.equal(w.localStorage.getItem('lol-loot-suite-tab'),null);assert.equal(w.localStorage.getItem('lol-loot-suite-scroll'),null);
  activateTab('champselect');const scroll=w.document.querySelector('#app-scroll');scroll.scrollTop=75;activateTab('watch');activateTab('champselect');assert.equal(scroll.scrollTop,75);
  handleLazySection({detail:{name:'overview'}});assert.equal(state.tab,'champselect');handleLazySection({detail:{name:'suite'}});assert.equal(state.tab,'champselect');
  w.dispatchEvent(new w.CustomEvent('deep-legends:navigate',{detail:{section:'suite',tab:'watch'}}));assert.equal(state.tab,'watch');
  w.dispatchEvent(new w.CustomEvent('deep-legends:navigate',{detail:{section:'suite',tab:'config'}}));assert.equal(state.tab,'config');
  w.dispatchEvent(new w.Event('beforeunload'));assert.equal(w.localStorage.getItem('lol-loot-suite-scroll'),null);
 }finally{w.close();}
});
