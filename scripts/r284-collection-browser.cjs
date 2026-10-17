'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results})=>{for(const theme of ['dark','light']){
 const p=await page(theme);await p.until('__r278Game.activeTab()?.data');
 await p.evaluate(`(()=>{const a=__r278App;window.__r284StatusReads=0;const fetch=window.fetch;window.fetch=(url,...rest)=>String(url)==='/api/status'?(++__r284StatusReads,new Promise(()=>{})):fetch(url,...rest);Object.assign(a.state.status,{snapshotReady:true,snapshotFallback:true,calculationOK:true,ownedCount:1201,chromaOwnedCount:1309,remainingCount:209,poolTotal:554});a.renderStatus();a.acceptClientView({type:'client-view',state:'no-client',generation:a.state.clientView.generation+1});})()`);
 for(const subpage of ['collection','pools']){
  const r=await p.evaluate(`(()=>{const a=__r278App;a.activateSection('favorites');a.activateFavoritesPage('${subpage}');const selectors=['.collection-toolbar','.list-row','.pool-primary-tabs','.pool-catalog-toolbar','.pool-filters','.pool-footer'];return {controls:selectors.map(s=>({s,hidden:document.querySelector(s).hidden})),counts:['owned-count','chroma-count','pool-count','remaining-count'].map(id=>document.getElementById(id).textContent),status:a.state.status}})()`);
  assert(r.controls.every(n=>n.hidden));assert.deepEqual(r.counts,["—","—","554","—"]);assert.equal(r.status.ownedCount,null);assert.equal(r.status.remainingCount,null);assert.equal(r.status.poolTotal,554);results.push({theme,subpage,...r});await p.shot('p2-disconnected-'+subpage);
 }
}}).catch(e=>{console.error(e);process.exitCode=1});
