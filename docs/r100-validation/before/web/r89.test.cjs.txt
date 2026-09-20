'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const source = fs.readFileSync(process.env.R89_GAMEPLAY_SOURCE || require('node:path').join(__dirname,'gameplay.js'),'utf8');
function body(name) {
  const m=new RegExp(`\\n  (?:async )?function ${name}\\(`).exec(source);
  assert.ok(m,name);
  const rest=source.slice(m.index+1);
  const end=/\n  (?:async )?function \w+\(/.exec(rest);
  return end?rest.slice(0,end.index):rest;
}
function compile(names,deps={}) { return Function(...Object.keys(deps),names.map(body).join('\n')+`\nreturn {${names.join(',')}};`)(...Object.values(deps)); }
test('R89 parallel supplements start before first overview resolves and survive ref adoption without duplicates',async()=>{
  const calls=[],pending=[];
  const state={settings:{matchCount:20},controllers:new Map()};
  const tab={key:'kr:fixture',region:'kr',riotId:{gameName:'Fixture',tagLine:'KR1'}};
  const noop=()=>{};
  const helpers=compile(['loadOverview','overviewSupplementTarget','syncOverviewSupplementRefs','loadOPGGSeasonSummary','loadOverviewCurrentGame'],{
    state,window:{},riotTab:t=>t?.region==='kr',tabReady:()=>true,tabGroup:()=> 'kr',
    rerenderTab:noop,renderCapabilitySettings:noop,rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture',
    updateCurrentGameCard:noop,ensurePerks:noop,ensureSummonerSpells:noop,
    normalizedPagination:(p,beg)=>({...p.pagination,nextBegIndex:beg+p.pagination.count}),
    AUTO_PAGE_DELAY_MS:1200,MAX_BROWSE_MATCHES:200,markProMismatch:noop,
    api:(url,options)=>new Promise(resolve=>{calls.push({url,body:JSON.parse(options.body),at:performance.now()});pending.push({url,resolve,onProgress:options.onProgress});})
  });
  const first=helpers.loadOverview(tab);
  assert.equal(calls.length,3,'all three requests must already be in flight');
  assert.ok(Math.max(...calls.map(c=>c.at))-Math.min(...calls.map(c=>c.at))<300);
  assert.equal(calls.find(c=>c.url.endsWith('/overview')).body.count,20);
  assert.deepEqual(calls.find(c=>c.url.endsWith('/season-summary')).body,{gameName:'Fixture',tagLine:'KR1',region:'kr',force:false});
  pending.find(c=>c.url.endsWith('/season-summary')).resolve({source:'OP.GG',queue:'RANKED',season:'S2026',champions:[],overall:{games:20}});
  pending.find(c=>c.url.endsWith('/current-game')).resolve({source:'OP.GG',status:'none'});
  await new Promise(r=>setImmediate(r));
  const payload=n=>({player:{playerRef:'opaque-ref',gameName:'Fixture',tagLine:'KR1',region:'kr'},matches:Array.from({length:n},(_,i)=>({gameId:i})),pagination:{count:n,hasMore:true},overall:{games:n}});
  pending.find(c=>c.url.endsWith('/overview')).onProgress(payload(5));
  assert.equal(tab.data.matches.length,5,'first screen remains visible during completion');
  // Supplement references are adopted when the complete snapshot arrives.
  assert.equal(calls.filter(c=>c.url.endsWith('/season-summary')).length,1);
  assert.equal(calls.filter(c=>c.url.endsWith('/current-game')).length,1);
  assert.equal(calls.at(-1).body.count,20);
  pending.find(c=>c.url.endsWith('/overview')).resolve(payload(20));await first;assert.equal(calls.filter(c=>c.url.endsWith('/overview')).length,1);assert.equal(tab.opggSeason.playerRef,'opaque-ref');assert.equal(tab.currentGame.ref,'opaque-ref');
  assert.equal(tab.data.matches.length,20);assert.equal(tab.data.overall.games,20);assert.equal(tab.initialPagePending,false);
});
test('R89 catalog failure retries once after backoff and missing IDs report once within a bound',async()=>{
  const state={},reports=[],timers=[];let attempts=0,now=100;
  const h=compile(['ensureItems','itemIconFigure'],{state,Date:{now:()=>now},setTimeout:(fn,ms)=>{timers.push({fn,ms});return timers.length;},clearTimeout:()=>{},
    api:async()=>{attempts++;if(attempts===1)throw Object.assign(new Error('timeout'),{errorKind:'timeout',status:504});return {items:[{id:3006,name:'boots',iconPath:'ddragon:/cdn/16.18.1/img/item/3006.png'}]};},
    rerenderCatalogViews:()=>{},recordItemSetClientDiagnostic:(...args)=>reports.push(args),pendingCatalogIcon:()=> 'pending',plainText:x=>x,escapeHTML:x=>x,assetIcon:()=> 'image'});
  await h.ensureItems();assert.equal(reports[0][0],'catalog_client');assert.equal(reports[0][2].errorKind,'timeout');assert.equal(reports[0][2].httpStatus,504);
  await h.ensureItems();assert.equal(attempts,1);assert.equal(timers[0].ms,30000);
  now+=30000;timers[0].fn();await new Promise(r=>setImmediate(r));assert.equal(attempts,2);assert.equal(state.items.items.length,1);
  for(let i=0;i<100;i++)h.itemIconFigure(9000+i%30);
  const missing=reports.filter(r=>r[0]==='item_id_not_in_catalog');assert.equal(missing.length,20);assert.equal(new Set(missing.map(r=>r[2].itemId)).size,20);
  assert.match(h.itemIconFigure(3006),/image/);
});
test('R89 catalog arrival invalidates retained match DOM in all open tabs',()=>{
 const first={matchViewRevision:2},second={},state={tabs:[first,second],overlay:[first],section:'overview'};
 let revisionAtRender;
 const {rerenderCatalogViews}=compile(['rerenderCatalogViews'],{state,externalMatchViews:new Map(),renderOverview:()=>{revisionAtRender=first.matchViewRevision},renderOverlay:()=>{},renderLive:()=>{}});
 rerenderCatalogViews();assert.equal(first.matchViewRevision,3);assert.equal(second.matchViewRevision,1);assert.equal(revisionAtRender,3);
});
test('R89 completion failure preserves first five and exposes a retry that completes the recent match list',async()=>{
 const tab={key:'kr:fixture',region:'kr',playerRef:'public-ref',initialPagePending:true,data:{player:{playerRef:'public-ref',region:'kr'},matches:Array.from({length:5},(_,gameId)=>({gameId})),pagination:{count:5,hasMore:true}}};
 const state={settings:{matchCount:20},controllers:new Map()};let fail=true;
 const noop=()=>{};
 const {loadOverview}=compile(['loadOverview'],{state,tabReady:()=>true,tabGroup:()=> 'kr',riotTab:()=>true,rerenderTab:noop,loadOPGGSeasonSummary:noop,loadOverviewCurrentGame:noop,syncOverviewSupplementRefs:noop,rememberTabPlayerRef:noop,playerLabel:()=> 'Fixture',renderCapabilitySettings:noop,AUTO_PAGE_DELAY_MS:1200,MAX_BROWSE_MATCHES:200,
  normalizedPagination:p=>({...p.pagination,nextBegIndex:p.pagination.count}),
  api:async()=>{if(fail)throw Object.assign(new Error('upstream timeout'),{status:504});return {player:{playerRef:'public-ref',region:'kr'},matches:Array.from({length:20},(_,gameId)=>({gameId})),pagination:{count:20,hasMore:true}};}});
 assert.equal(await loadOverview(tab,true,false,false,true),false);assert.equal(tab.data.matches.length,5);assert.equal(tab.initialPageError,'upstream timeout');assert.equal(tab.error,'');
 fail=false;assert.equal(await loadOverview(tab,true,false,true,true),true);assert.equal(tab.initialPageError,'');assert.equal(tab.data.matches.length,20);assert.equal(tab.initialPagePending,false);
});
test('R89 closing an overlay marks it closed and aborts its overview request',()=>{
 const old={key:'old',overlay:true},active={key:'active',overlay:true};let aborted=0;
 const state={overlay:[old,active],controllers:new Map([['overview:active',{abort:()=>aborted++}],['overview:old',{abort:()=>aborted++}]])};
 const h=compile(['overlayBack','closeOverlay'],{state,nodes:{playerOverlay:{}},renderOverlay:()=>{}});
 h.overlayBack();assert.equal(active.closed,true);assert.equal(aborted,1);
 h.closeOverlay();assert.equal(old.closed,true);assert.equal(aborted,2);
});
