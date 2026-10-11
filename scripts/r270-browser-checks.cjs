'use strict';
const assert=require('node:assert/strict');
module.exports=async({evaluate,until,click,call,shot,ready,results,releaseFilters})=>{
 const key='deep-legends-history-presets-v1',modal='dialog[data-af-menu]:modal';
 const snapshot=()=>evaluate("(()=>{const controls=['setting-default-page','setting-default-match-filter','setting-match-count','setting-mask-names','setting-ui-scale'];return {preferences:Object.fromEntries(Object.keys(localStorage).filter(k=>k.startsWith('lol-loot-')).sort().map(k=>[k,localStorage.getItem(k)])),controls:controls.map(id=>{const n=document.getElementById(id);return [id,n.value,n.checked]})}})()");
 const open=async()=>{await click('#overview-content [data-history-filter-load],#overview-content [data-af-open]');await until(`document.querySelector('${modal}')?.hidden===false`);};
 assert.equal(await evaluate('!!window.deepLegendsHistoryFilters'),false,'cold script must be held, not preloaded before first click');
 await click('#overview-content [data-history-filter-load]');
 assert(await evaluate("document.querySelector('#overview-content')._overviewViewTab.advancedMenu.open && !document.querySelector('#overview-content')._afBoundTab"));
 releaseFilters();await until(`document.querySelector('${modal}') && document.querySelector('#overview-content [data-af-open]')`);
 await click(modal+' [data-af-category="result"]');await click(modal+' [data-af-option="win"]');await click(modal+' [data-af-page="saved"]');
 await evaluate(`document.querySelector('${modal} [data-af-name]').value='R270 restart retained'`);await click(modal+' [data-af-save]');
 await until(`JSON.parse(localStorage.getItem('${key}')||'[]').some(p=>p.name==='R270 restart retained')`);
 const before=await snapshot();await evaluate("localStorage.setItem('r270-preserve','1')");
 // A new document resets the module/dialog/top layer, retaining real storage.
 // Actual process restarts are additionally required by the Windows upgrade chain.
 await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview'});await until("document.querySelector('.match-list .match-entry') && !document.querySelector('#overview-content')._overviewViewTab.loading");
 await open();await click(modal+' [data-af-page="saved"]');await until(`document.querySelector('${modal} .af-saved-row')?.textContent.includes('R270 restart retained')`);
 assert.deepEqual(await snapshot(),before);
 await evaluate(`localStorage.setItem('${key}','{corrupt-r265')`);
 await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview'});await until("document.querySelector('.match-list .match-entry') && !document.querySelector('#overview-content')._overviewViewTab.loading");
 await open();await click(modal+' [data-af-page="saved"]');await until(`localStorage.getItem('${key}')===null`);
 await evaluate("(()=>{const decoy=document.createElement('p');decoy.className='af-empty';decoy.id='r270-decoy';decoy.textContent='没有符合的对局';document.body.prepend(decoy)})()");
 assert.equal(await evaluate("document.querySelector('.af-empty').textContent"),'没有符合的对局');
 assert(await evaluate(`document.querySelector('${modal} .af-saved .af-empty')?.textContent.includes('暂无常用筛选')`));assert.deepEqual(await snapshot(),before);
 await shot('r270-corrupt-preset-modal');results.push({r270:'cold-first-click-document-restart-corrupt',scriptHeldBeforeClick:true,firstClickOpenedModal:true,presetRetainedAfterDocumentRestart:true,corruptKeyDeleted:true,preferencesAndControlsUnchanged:true,currentModalEmptyVisible:true,unscopedSelectorDecoyProved:true});
 await evaluate("document.querySelector('#r270-decoy').remove();localStorage.removeItem('r270-preserve')");
 await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview'});await until("document.querySelector('.match-list .match-entry') && !document.querySelector('#overview-content')._overviewViewTab.loading");
};
