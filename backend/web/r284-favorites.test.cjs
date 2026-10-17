'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{bootDemoApp}=require('../../desktop/overview-render-helpers.cjs'),{waitForRender}=require('../../desktop/renderer-wait.cjs');
test('R284 P5 returning to favorites resets its page and owned filters, tools retain their tab',async()=>{
 const f=bootDemoApp({appSourceTransform:s=>(process.env.R284_APP_SOURCE?fs.readFileSync(process.env.R284_APP_SOURCE,'utf8'):s).replace(/\}\)\(\);\s*$/,'window.__r284App={state,activateSection,activateFavoritesPage};})();')});
 try{const w=f.window;await waitForRender(w,()=>w.__r284App?.state.status,'app ready',10000);const a=w.__r284App,scroll=w.document.getElementById('app-scroll');scroll.scrollTo=({top})=>{scroll.scrollTop=top};
  for(const previous of ['pools','collection']){a.activateSection('favorites');a.activateFavoritesPage(previous);a.state.view='chromas';a.state.query='安妮';w.document.getElementById('search').value='安妮';scroll.scrollTop=500;
   a.activateSection('overview');a.activateSection('favorites');await waitForRender(w,()=>scroll.scrollTop===0,'favorites scroll restored',2000);assert.equal(a.state.favoritesPage,'collection');assert.equal(a.state.view,'owned');assert.equal(a.state.query,'');assert.equal(scroll.scrollTop,0);assert(w.document.querySelector('[data-pool-page="catalog"]').classList.contains('is-active'));
  }
  a.activateSection('suite');await waitForRender(w,()=>w.document.querySelector('[data-suite-tab="facade"]'),'suite ready',10000);w.document.querySelector('[data-suite-tab="facade"]').click();a.activateSection('overview');a.activateSection('suite');assert(w.document.querySelector('[data-suite-tab="facade"]').classList.contains('is-active'));
  assert.deepEqual(f.errors,[]);
 }finally{f.window.close()}
});
