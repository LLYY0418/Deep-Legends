'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{browser}=require('./r297-browser-harness.cjs');
const out=process.env.R317_ARENA_OUT||'/tmp/r317-arena',rows=[];
(async()=>{const b=await browser({out});try{
 await b.call('Page.navigate',{url:b.origin+'/?demo=arena-squads&section=live'});
 await b.until('window.__r297?.state.live?.players?.length===18 && window.deepLegendsLiveArena');
 await b.evaluate(`document.querySelector('[data-announcements-close]')?.click();window.r317Roster=structuredClone(__r297.state.live.players)`);
 for(const theme of ['dark','light'])for(const phase of ['ChampSelect','InProgress']){
  await b.evaluate(`(()=>{document.documentElement.dataset.theme='${theme}';const s=__r297.state;s.recommendationTab='insight';s.recommendationTabTouched=true;s.live.phase='${phase}';s.beacon.phase='${phase}';s.live.players=structuredClone(r317Roster${phase==='ChampSelect'?'.slice(0,3)':''});__r297.renderLive();document.querySelector('.app-scroll').scrollTop=0})()`);
  await b.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
  const m=await b.evaluate(`(()=>{const players=[...document.querySelectorAll('.live-player')];return{count:players.length,ranks:players.map(p=>p.querySelector('.live-player-rank')?.textContent||''),crests:players.map(p=>p.querySelectorAll('.live-rank-crest').length),self:players.filter(p=>p.querySelector('.self-chip')).length}})()`);
  assert.equal(m.count,phase==='ChampSelect'?3:18);assert(m.ranks.every(r=>r.includes('单双')&&r.includes('组排')));assert(m.crests.every(n=>n===2));assert.equal(m.self,1);
  rows.push({theme,phase,...m});await b.shot(`arena-${theme}-${phase}`);
 }
 assert.deepEqual(b.errors,[]);
}finally{fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2));await b.stop()}})().catch(error=>{console.error(error);process.exitCode=1});
