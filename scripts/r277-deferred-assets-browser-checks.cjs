'use strict';
const assert=require('node:assert/strict');
module.exports=async({evaluate,until,results,ready})=>{
 const fault=process.env.R277_DEFERRED_FAULT==='503',settle=()=>evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 const response=await fetch(ready.baseUrl+'/match-details.js');assert.equal(response.status,200,'parent must embed match-details.js; initial source cannot stand in for it');assert((await response.text()).includes('deepLegendsMatchDetails'));
 const count=()=>evaluate("performance.getEntriesByType('resource').filter(e=>new URL(e.name).pathname==='/match-details.js').length");
 assert.equal(await count(),0,'cold overview cannot fetch rich details');
 const ids=await evaluate("(()=>{const entries=[...document.querySelectorAll('#overview-content .match-entry')];window.__r277DeferredList=entries[0].parentNode;window.__r277DeferredOther=entries[2];return entries.slice(0,2).map(e=>e.dataset.matchId)})()");
 await evaluate(`document.querySelector('[data-match-id="${ids[0]}"] [data-toggle-match]').click();true`);
 if(fault){
  await until(`document.querySelector('#match-detail-${ids[0]}')?.textContent.includes('详情资源加载失败')`);await settle();assert.equal(await count(),1,'failure must settle without infinite retry');
  assert(await evaluate("!document.querySelector('script[data-section-module=match-details]')"));
  await evaluate(`document.querySelector('[data-match-id="${ids[0]}"] [data-toggle-match]').click();document.querySelector('[data-match-id="${ids[0]}"] [data-toggle-match]').click();true`);
 }else await evaluate(`document.querySelector('[data-match-id="${ids[1]}"] [data-toggle-match]').click();true`);
 await until(`document.querySelector('#match-detail-${ids[0]} .match-table')`);
 if(!fault)await until(`document.querySelector('#match-detail-${ids[1]} .match-table')`);
 assert.equal(await count(),fault?2:1);
 for(const page of ['team','build','overview']){
  await evaluate(`document.querySelector('#match-detail-${ids[0]} [data-match-detail="${page}"]').click();true`);
  const selector={team:'.team-analysis-metrics',build:'.build-detail',overview:'.match-table'}[page];await until(`document.querySelector('#match-detail-${ids[0]} ${selector}')`);
 }
 await evaluate(`document.querySelector('[data-match-id="${ids[0]}"] [data-toggle-match]').click();document.querySelector('[data-match-id="${ids[0]}"] [data-toggle-match]').click();true`);await settle();await until(`document.querySelector('#match-detail-${ids[0]} .match-table')`);
 assert.equal(await count(),fault?2:1,'cached reopen must issue no module GET');
 assert(await evaluate("__r277DeferredList===document.querySelector('#overview-content .match-list') && __r277DeferredOther===document.querySelector('#overview-content .match-list').children[2]"));
 const module=await evaluate("(()=>{const n=document.querySelector('script[data-section-module=match-details]');return {src:n.getAttribute('src'),origin:new URL(n.src).origin,inline:n.textContent,hasFactory:typeof deepLegendsMatchDetails.create==='function'}})()");assert.equal(module.src,'/match-details.js');assert.equal(module.origin,new URL(ready.baseUrl).origin);assert.equal(module.inline,'');assert(module.hasFactory);
 // A Riot external card uses the exact same loaded factory and local controls.
 await evaluate("(()=>{const t=document.querySelector('#overview-content')._overviewViewTab,r=document.createElement('main');r.id='r277-deferred-external';document.body.append(r);const m=structuredClone(t.data.matches[0]);m.gameId=277123;window.__r277External=deepLegendsMatchCards.mount(r,{matches:[m],playerRef:t.data.player.playerRef,region:'kr',key:'r277-deferred-riot'});r.querySelector('[data-toggle-match]').click();return true})()");
 await until("document.querySelector('#r277-deferred-external .match-table')");
 for(const page of ['team','build','overview']){await evaluate(`document.querySelector('#r277-deferred-external [data-match-detail="${page}"]').click();true`);await until(`document.querySelector('#r277-deferred-external ${{team:'.team-analysis-metrics',build:'.build-detail',overview:'.match-table'}[page]}')`);}
 assert.equal(await count(),fault?2:1,'Riot card must reuse the same rich module');await evaluate("__r277External.destroy();document.querySelector('#r277-deferred-external').remove();true");
 results.push({P:9,case:fault?'explicit-503-recovery':'deferred-rich-details',moduleRequests:await count(),retainedList:true,cachedReopen:true,allThreeTabs:true,riotExternalReuse:true,module});
};
