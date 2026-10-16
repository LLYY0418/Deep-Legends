'use strict';
const assert = require('node:assert/strict');

module.exports = async function checkProgressAndShare(page, results) {
  await page.evaluate("document.documentElement.style.setProperty('--ui-zoom','1');document.documentElement.dataset.uiScale='1';window.__r280Shares=[];window.desktopShare={preparePngSave:async()=>({token:'fixture'}),captureAndSavePng:async payload=>{__r280Shares.push(payload.markup);return {ok:true}}}");
  for (const phase of ['initial','career-update','stream-update']) {
    await page.evaluate(`(()=>{const g=__r278Game,t=g.activeTab();g.state.overviewShareStatus='idle';if('${phase}'!=='initial'){t.data={...t.data,activityHours:Array.from({length:24},(_,i)=>i+${phase==='career-update'?10:20})};g.patchOverviewProgress(t);}document.querySelector('[data-generate-overview-share]').click();})()`);
    const count = ['initial','career-update','stream-update'].indexOf(phase)+1;
    await page.until(`__r280Shares.length===${count}`);
    const markup = await page.evaluate(`(()=>{const doc=new DOMParser().parseFromString(__r280Shares.at(-1),'text/html');return {strip:doc.querySelectorAll('.summoner-strip').length,layout:doc.querySelectorAll('.overview-layout').length,rows:doc.querySelectorAll('.match-entry').length}})()`);
    assert.equal(markup.strip,1);assert.equal(markup.layout,1);assert(markup.rows>0);
    results.push({kind:'share-owner',phase,...markup});
  }

  await page.evaluate("__r278Game.openPlayerOverlay({playerRef:'r280-parent',label:'Parent A',region:'TENCENT',serverId:'HN1'})");
  await page.until("__r278Game.state.overlay.length===1 && !__r278Game.state.overlay[0].loading");
  await page.evaluate("(()=>{const g=__r278Game,a=g.state.overlay.at(-1);window.__r280Parent=a;a.data={...a.data,player:{...a.data.player,gameName:'Parent A',tagLine:'A'}};g.patchOverviewProgress(a);g.scheduleOverviewProgress(a);g.openPlayerOverlay({playerRef:'r280-child',label:'Child B',region:'TENCENT',serverId:'HN1'});})()");
  await page.until("__r278Game.state.overlay.length===2 && !__r278Game.state.overlay.at(-1).loading");
  await page.evaluate("(()=>{const g=__r278Game,b=g.state.overlay.at(-1);b.data={...b.data,player:{...b.data.player,gameName:'Child B',tagLine:'B'}};g.patchOverviewProgress(b);const root=document.getElementById('player-overlay-content');window.__r280ChildBefore={name:root.querySelector('.summoner-strip h2').textContent,rows:[...root.querySelectorAll('.match-entry')].map(n=>n.dataset.matchId),owner:root._overviewViewTab.key};__r280Parent.data={...__r280Parent.data,player:{...__r280Parent.data.player,gameName:'Parent A latest'}};g.scheduleOverviewProgress(__r280Parent);return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()");
  const child = await page.evaluate("(()=>{const root=document.getElementById('player-overlay-content');return {before:__r280ChildBefore,after:{name:root.querySelector('.summoner-strip h2').textContent,rows:[...root.querySelectorAll('.match-entry')].map(n=>n.dataset.matchId),owner:root._overviewViewTab.key},parentFrame:__r280Parent.progressFrame}})()");
  assert.deepEqual(child.before,child.after);assert.match(child.after.name,/Child B/);assert.equal(child.parentFrame,0);
  results.push({kind:'overlay-progress-owner',...child});

  const stale = await page.evaluate("(()=>{const g=__r278Game,b=g.state.overlay.at(-1),root=document.getElementById('player-overlay-content'),before=root.querySelector('.summoner-strip h2').textContent;g.scheduleOverviewProgress(b);b.overviewRequestToken++;b.data={...b.data,player:{...b.data.player,gameName:'Stale mutation'}};return new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve({before,after:root.querySelector('.summoner-strip h2').textContent}))));})()");
  assert.equal(stale.before,stale.after);results.push({kind:'stale-request-frame',...stale});
  await page.evaluate("__r278Game.overlayBack()");
  const returned = await page.evaluate("document.querySelector('#player-overlay-content .summoner-strip h2').textContent");
  assert.match(returned,/Parent A latest/);results.push({kind:'overlay-return-latest',name:returned});
  await page.evaluate("(()=>{const g=__r278Game;g.scheduleOverviewProgress(g.state.overlay.at(-1));g.closeOverlay();return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()");
  assert.equal(await page.evaluate("document.getElementById('player-overlay-content').children.length"),0);
};
