'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 const p=await page('dark');
 await p.evaluate(`localStorage.setItem('lol-loot-ui-scale','1');__r278App.applyUiScale();window.__r290Loading={frames:[],active:true};(()=>{const sample=()=>{const panel=document.getElementById('settings-panel'),link=document.querySelector('link[href="/settings-preferences.css"]'),icon=document.querySelector('#settings-github svg'),button=document.getElementById('settings-github'),r=icon.getBoundingClientRect();__r290Loading.frames.push({visible:!panel.hidden,styled:!!link?.sheet,rendered:icon.getClientRects().length>0,width:r.width,height:r.height,buttonHeight:button.getBoundingClientRect().height});if(__r290Loading.active)requestAnimationFrame(sample)};requestAnimationFrame(sample)})();__r278App.activateSection('settings')`);
 await p.until(`document.querySelector('link[href="/settings-preferences.css"]')&&!document.querySelector('link[href="/settings-preferences.css"]').sheet`);
 await p.shot('settings-style-pending');
 await p.evaluate("fetch('/__r290/release-settings-css')");
 await p.until('window.deepLegendsSettingsBackup?.createBackup');
 await p.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>{__r290Loading.active=false;r()})))');
 const frames=await p.evaluate('__r290Loading.frames');
 results.push({kind:'cold-settings-load',frames});
 assert(frames.some(x=>x.visible&&!x.styled),'delayed CSS must be exercised');
 const bad=frames.filter(x=>x.visible&&x.rendered&&(Math.abs(x.width-20)>.02||Math.abs(x.height-20)>.02||Math.abs(x.buttonHeight-40)>.02));
 assert.deepEqual(bad,[],'settings must never paint the unstyled GitHub icon');
 for(let i=0;i<3;i++){
  const value=await p.evaluate(`__r278App.activateSection('overview');__r278App.activateSection('settings');(()=>{const r=document.querySelector('#settings-github svg').getBoundingClientRect();return {width:r.width,height:r.height,pending:document.getElementById('settings-panel').hasAttribute('data-settings-style-pending')}})()`);
  assert.equal(value.pending,false);assert.equal(value.width,20);assert.equal(value.height,20);results.push({kind:'warm-return',...value});
 }
 await p.shot('settings-loaded');
 const retryPage=await page('light');
 await retryPage.call('Network.enable');await retryPage.call('Network.setBlockedURLs',{urls:['*settings-preferences.css*']});
 await retryPage.evaluate("__r278App.activateSection('settings')");
 await retryPage.until("document.querySelector('[data-section-loading][role=alert]')");
 const failed=await retryPage.evaluate("({pending:document.getElementById('settings-panel').hasAttribute('data-settings-style-pending'),iconRendered:document.querySelector('#settings-github svg').getClientRects().length>0,retryVisible:document.querySelector('[data-section-loading] button').getClientRects().length>0})");
 assert(failed.pending&&!failed.iconRendered&&failed.retryVisible);results.push({kind:'css-failure',...failed});
 await retryPage.call('Network.setBlockedURLs',{urls:[]});
 await retryPage.evaluate("document.querySelector('[data-section-loading] button').click()");
 await retryPage.until('window.deepLegendsSettingsBackup?.createBackup');
 assert.equal(await retryPage.evaluate("document.getElementById('settings-panel').hasAttribute('data-settings-style-pending')"),false);
 results.push({kind:'css-retry',recovered:true});
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
