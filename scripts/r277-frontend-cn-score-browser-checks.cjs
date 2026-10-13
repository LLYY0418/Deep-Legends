'use strict';
const fs=require('node:fs'),assert=require('node:assert/strict');
module.exports=async({evaluate,until,results})=>{
 const file=process.env.R277_CN_SCORE_SERVER,ready=JSON.parse(fs.readFileSync(file,'utf8'));
 const response=await fetch(ready.url+'/fixture');assert.equal(response.status,200);const fixtures=await response.json();
 await evaluate("window.__r277CNReads=[];window.__r277CNOriginalFetch=window.fetch;window.fetch=(input,init)=>{if(String(input)==='/api/gameplay/match')__r277CNReads.push(JSON.parse(init.body));return __r277CNOriginalFetch(input,init)};true");
 for(const kind of ['self','cn']){
  const f=fixtures[kind];assert(!f.matches.some(m=>m.participants.some(p=>p.score?.parts)),'actual overview wire must omit parts');
  const gameID=f.matches[0].gameId;
  await evaluate(`(()=>{const t=document.querySelector('#overview-content')._overviewViewTab;t.current=${kind==='self'};t.region='';t.group='players';t.matchFilter='all';t.matchViewRevision=Number(t.matchViewRevision||0)+1;t.serverId=${JSON.stringify(f.serverId)};t.data={...t.data,player:{...t.data.player,isCurrent:${kind==='self'},playerRef:${JSON.stringify(f.playerRef)},serverId:${JSON.stringify(f.serverId)},region:''},matches:${JSON.stringify(f.matches)},pagination:{count:1,hasMore:false}};t.openMatches.clear();delete t.filteredNodeCache;__r266Game.renderOverviewBody(document.querySelector('#overview-content'),t);return true})()`);
  await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  assert(await evaluate("Boolean(document.querySelector('#overview-content [data-score-participant]'))"),await evaluate("document.querySelector('#overview-content').textContent"));
  const before=await evaluate('__r277CNReads.length');assert.equal(before,kind==='self'?0:1);
  // Hover and expansion independently exercise both user entry points.
  if(kind==='self')await evaluate("document.querySelector('#overview-content [data-score-participant]').dispatchEvent(new PointerEvent('pointerenter'));true");
  else await evaluate("document.querySelector('#overview-content [data-toggle-match]').click();true");
  await until("document.querySelector('#overview-content')._overviewViewTab.data.matches[0].scorePartsLoaded===true");
  const proof=await evaluate("(()=>{const c=document.querySelector('#overview-content'),t=c._overviewViewTab;return {reads:__r277CNReads.length,request:__r277CNReads.at(-1),parts:t.data.matches[0].participants[0].score.parts.length,tooltipParts:JSON.parse(c.querySelector('[data-score-participant]').dataset.tooltipScore).parts.length}})()");
  assert.equal(proof.reads,before+1);assert.equal(proof.request.gameId,gameID);assert.equal(proof.request.serverId,f.serverId);assert.equal(proof.request.playerRef,f.playerRef);assert.equal(proof.request.refresh,'score-parts');assert(proof.parts>0 && proof.tooltipParts>0);
  await evaluate("document.querySelector('#overview-content [data-score-participant]').dispatchEvent(new PointerEvent('pointerenter'));document.querySelector('#overview-content [data-score-participant]').focus();true");await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');assert.equal(await evaluate('__r277CNReads.length'),before+1);
  results.push({P:9,kind,scope:'real Chromium frontend POST forwarded unchanged to actual Go handleGameplayMatch; synthetic immutable upstream games',...proof});
 }
 fs.writeFileSync(file+'.done','browser-contract-pass');
};
