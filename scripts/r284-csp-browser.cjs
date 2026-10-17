'use strict';
process.env.R284_STRICT_CSP='1';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results})=>{for(const theme of ['dark','light']){
 const p=await page(theme);await p.evaluate("__r278App.activateSection('suite')");await p.until('window.__r282Suite');await p.evaluate("document.querySelector('[data-suite-tab=facade]').click()");await p.until("document.querySelector('.facade-film button.is-selected')");
 await p.evaluate("document.querySelector('.facade-film').scrollIntoView();new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))");
 const suite=await p.evaluate('__r284CSP');results.push({theme,page:'suite-facade',violations:suite});
 if(process.env.R284_CSP_EXPECT_BLOCK==='1'){assert(suite.some(e=>['data','data:'].includes(e.blocked)),'baseline must reproduce data image CSP');continue}assert.deepEqual(suite,[]);await p.shot('p8-suite-facade');
 await p.evaluate("__r278App.activateSection('favorites');__r278App.activateFavoritesPage('facade-collection')");await p.until('window.__r283Facade');
 for(const group of ['icons','banners']){await p.evaluate(`document.querySelector('#favorites-facade-panel [data-view="${group}"]').click();new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);await p.until(`__r283Facade.state.view==='${group}' && document.querySelector('#facade-grid .skin-card')`);const v=await p.evaluate('__r284CSP');assert.deepEqual(v,[]);results.push({theme,page:'favorites-'+group,violations:v});await p.shot('p8-favorites-'+group);}
}}).catch(e=>{console.error(e);process.exitCode=1});
