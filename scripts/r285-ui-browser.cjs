'use strict';
const assert=require('node:assert/strict'),spells=require('../backend/testdata/r285-summoner-spells.json'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']){
  const p=await page(theme);await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
  await p.evaluate("deepLegendsSections.load('live-arena')");
  await p.call('Emulation.setDeviceMetricsOverride',{width:2200,height:1800,deviceScaleFactor:1,mobile:false});
  await p.evaluate(`window.__r285Spells=${JSON.stringify(spells)};__r278Game.state.summonerSpells={spells:__r285Spells};const stage=document.createElement('main');stage.id='r285-stage';stage.style.cssText='position:absolute;top:20px;left:20px;width:1250px;z-index:9999;background:var(--bg);padding:16px;container-type:inline-size;container-name:recommendation-area';document.body.append(stage);`);
  for(const zoom of [1,1.5])for(const queueId of [420,450,2400])for(const side of [100,200])for(const hidden of [1,2]){
   const r=await p.evaluate(`(()=>{const g=__r278Game,stage=document.getElementById('r285-stage');stage.style.zoom=${zoom};stage.style.width='1250px';const players=Array.from({length:10},(_,i)=>({playerRef:'fixture-'+i,teamId:i<5?100:200,position:['top','jungle','middle','bottom','utility'][i%5],isCurrent:i===0,championId:103,championName:'阿狸',gameName:'玩家'+i,hidden:(i<5?100:200)===${side} && i%5<${hidden},historyState:'ok',modeStats:{games:10,wins:5,losses:5,winRate:50,kda:3},recentGames:Array.from({length:i%2?10:3},()=>({championId:103,championName:'阿狸',kills:10,deaths:2,assists:12,win:true}))}));stage.innerHTML=g.renderLiveInsights({phase:'InProgress',queueId:${queueId},gameId:285,players});const teams=[...stage.querySelectorAll('.live-team')];return {rows:teams.map(t=>[...t.querySelectorAll('.live-player,.insight-match-row')].map(n=>({top:n.getBoundingClientRect().top,bottom:n.getBoundingClientRect().bottom,hidden:n.classList.contains('is-hidden-player'),text:n.textContent}))),grid:getComputedStyle(teams[0]).gridTemplateRows}})()`);
   assert.equal(r.rows.length,2);assert.equal(r.rows[0].length,10);assert.equal(r.rows[1].length,10);
   let max=0;for(let i=0;i<10;i++)max=Math.max(max,Math.abs(r.rows[0][i].top-r.rows[1][i].top),Math.abs(r.rows[0][i].bottom-r.rows[1][i].bottom));assert(max<=1,JSON.stringify({theme,zoom,queueId,side,hidden,max,r}));
   for(const row of r.rows.flat().filter(r=>r.hidden))assert.equal(row.text,'');
   results.push({kind:'P6',theme,zoom,queueId,side,hidden,max});
   if(queueId===420 && side===100 && hidden===2)await p.shot('hidden-'+zoom);
  }
  const narrow=await p.evaluate(`(async()=>{const stage=document.getElementById('r285-stage');stage.style.width='600px';await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return {grid:getComputedStyle(stage.querySelector('.live-team')).gridTemplateRows,display:getComputedStyle(stage.querySelector('.live-team')).display,width:getComputedStyle(stage).width,container:getComputedStyle(stage).container,css:stage.style.cssText}})()`);assert.equal(narrow.display,'block',JSON.stringify(narrow));results.push({kind:'P6-narrow',theme,grid:narrow});
  await p.evaluate(`(()=>{const g=__r278Game,stage=document.getElementById('r285-stage');stage.style.width='1250px';stage.style.zoom=1;stage.innerHTML=g.renderLiveInsights({phase:'InProgress',queueId:1750,gameMode:'CHERRY',mapId:30,gameId:285,arenaGrouped:true,players:Array.from({length:6},(_,i)=>({playerRef:'arena-'+i,teamId:100,arenaGroup:String(Math.floor(i/3)+1),isCurrent:i===0,championId:103,hidden:i===1,historyState:'ok',recentGames:[]}))})})()`);
  const arena=await p.evaluate("({teams:document.querySelectorAll('#r285-stage .live-team').length,arena:!!document.querySelector('#r285-stage .live-teams.is-arena')})");assert(arena.arena);assert.equal(arena.teams,2);results.push({kind:'P6-arena',theme,...arena});
  await p.evaluate(`__r278Game.state.summonerSpells={spells:__r285Spells};document.getElementById('r285-stage').innerHTML=__r285Spells.map(s=>'<div style="display:inline-block;margin:30px">'+__r278Game.spellIconFigure(s.id,'small')+'</div>').join('');`);
  const tip=await p.evaluate("[...document.querySelectorAll('#r285-stage [data-tooltip]')].map(n=>({text:n.dataset.tooltip,rect:n.getBoundingClientRect().toJSON()}))");for(const s of spells)assert(tip.some(t=>t.text===s.name+'\n'+s.description));results.push({kind:'P4',theme,tips:tip});
  await p.call('Input.dispatchMouseEvent',{type:'mouseMoved',x:tip[1].rect.x+8,y:tip[1].rect.y+8});await p.until("document.querySelector('.global-tooltip')?.textContent.includes('爆发性移动速度')");await p.shot('spell-tooltip');
 }
 assert.deepEqual(errors,[]);
}).catch(e=>{console.error(e);process.exitCode=1});
