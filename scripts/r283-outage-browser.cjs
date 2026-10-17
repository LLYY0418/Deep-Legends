'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
async function run(){await withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']){
 const p=await page(theme);await p.until("__r278Game.activeTab()?.data && !__r278Game.activeTab().loading");
 await p.evaluate("__r278App.activateSection('live')");await p.until("window.deepLegendsLiveArena && !__r278Game.state.liveLoading");
 for(const queue of [420,1750])for(const zoom of [1,1.5]){
 const r=await p.evaluate(`(()=>{const g=__r278Game;document.documentElement.style.setProperty('--ui-zoom','${zoom}');g.state.settings.liveRefresh=false;g.state.liveRetryAttempts=8;g.state.beacon.phase='InProgress';g.state.liveLoadingVisible=false;g.state.live={phase:'InProgress',gameId:283,queueId:${queue},queueLabel:'${queue===1750?'斗魂竞技场':'单排/双排'}',gameMode:'${queue===1750?'CHERRY':'CLASSIC'}',mapId:${queue===1750?30:11},available:true,arenaGrouped:${queue===1750},historyServiceDown:true,players:Array.from({length:${queue===1750?18:10}},(_,i)=>({playerRef:'fixture-'+i,gameName:'玩家'+i,isCurrent:i===0,isAlly:i<${queue===1750?3:5},teamId:i<${queue===1750?3:5}?100:200,arenaGroup:String(Math.floor(i/3)+1),historyState:'failed',championId:1,recentGames:[]}))};g.state.recommendationTab='insight';g.state.recommendationTabTouched=true;g.renderLive('r283');const bar=document.querySelector('.live-toolbar'),status=bar.querySelector('[data-live-status]'),height=bar.getBoundingClientRect().height;const down={text:status.textContent,failed:document.querySelectorAll('.insight-none').length,rows:document.querySelectorAll('[data-live-player-row]').length,repeat:bar.textContent.split('数据尚未完整，已停止自动重试，可手动刷新').length-1,retries:document.querySelectorAll('[data-live-history-retry],.insight-retry').length};g.state.live.historyServiceDown=false;g.renderLive('r283');const restored={text:status.textContent,height:bar.getBoundingClientRect().height};g.state.live.historyServiceDown=true;g.renderLive('r283');return {height,down,restored}})()`);
 assert(r.down.failed>=10,JSON.stringify(r));assert.equal(r.down.retries,0);assert.equal(r.down.repeat,1);assert.equal(r.restored.text,'');assert.equal(r.height,r.restored.height);results.push({theme,queue,zoom,...r});await p.shot('r283-outage-'+queue+'-'+zoom);
 }
 }
 assert.deepEqual(errors,[]);
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
