'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
 const roster=await p.evaluate(`(()=>{
  const g=__r278Game,seen=new WeakSet(),identities=new Map(),leaks=[];
  const rename=value=>{if(!value||typeof value!=='object'||seen.has(value))return;seen.add(value);if(value.gameName||value.displayName&&value.playerRef){const key=value.playerRef||value.gameName;let id=identities.get(key);if(!id){id=identities.size+1;identities.set(key,id)}value.gameName='R288LeakName'+id;value.displayName=value.gameName;value.tagLine='R288LeakTag'+id;leaks.push(value.gameName,value.tagLine);}for(const item of Object.values(value))rename(item)};
  rename(g.activeTab().data);rename(__r278App.state.clientView);rename(__r280Overview);
  window.__r288Leaks=[...new Set(leaks)];g.state.settings.maskNames=false;g.activeTab().matchViewRevision++;g.renderPlayerTabs();g.renderOverview();__r278App.renderStatus();return __r288Leaks;
 })()`);
 assert(roster.length>=20,'fixture must include at least ten unique players');
 assert(await p.evaluate('__r288Leaks.some(value=>document.body.textContent.includes(value))'),'unique names must render before privacy is enabled');
 const check=async(name,extra='')=>{
  const outcome=await p.evaluate(`(()=>{const text=document.body.textContent+' '+[...document.querySelectorAll('[data-tooltip],[aria-label]')].map(n=>(n.dataset.tooltip||'')+' '+(n.getAttribute('aria-label')||'')).join(' ')+${JSON.stringify(extra)};return {name:'${name}',leaks:__r288Leaks.filter(value=>text.includes(value)),masked:document.body.textContent.includes('召唤师')}})()`);assert.deepEqual(outcome.leaks,[],JSON.stringify(outcome));results.push(outcome);
 };
 await p.evaluate("document.querySelector('.global-tooltip').textContent=__r288Leaks[0];document.getElementById('setting-mask-names').checked=true;document.getElementById('setting-mask-names').dispatchEvent(new Event('change'))");
 await check('overview-tabs-header-recent-history');
 const share=await p.evaluate("__r278Game.createOverviewShareMarkup(document.getElementById('overview-content'))");assert(!roster.some(value=>share.includes(value)));results.push({name:'share',leaks:[]});
 // The production detail renderer is lazy, with the same injected privacy function.
 await p.evaluate(`(async()=>{const g=__r278Game;await g.ensureMatchDetailRenderer();const tab=g.activeTab(),match=tab.data.matches?.[0]||tab.data.recentMatches?.[0];window.__r288Match=match;if(!match)throw Error('missing match');tab.openMatches.add(String(match.gameId));g.rerenderMatch(tab,String(match.gameId))})()`);
 await check('details-default');
 for(const tab of ['overview','team','build']){
  await p.evaluate(`(()=>{const g=__r278Game,view=g.activeTab();view.matchDetailTabs.set(String(__r288Match.gameId),'${tab}');g.rerenderMatch(view,String(__r288Match.gameId))})()`);
  assert(await p.evaluate("!!document.querySelector('.match-detail [role=tabpanel]')"),'detail panel must render');
  await check('details-'+tab);
 }
 await p.evaluate(`(async()=>{await deepLegendsSections.load('friends');const f=__r280Friends;f.state.connected=true;f.state.groups=[{id:1,name:'好友',isMetaGroup:true}];f.state.friends=Array.from({length:10},(_,i)=>({playerRef:'friend-'+i,gameName:__r288Leaks[i*2],tagLine:__r288Leaks[i*2+1],groupId:1,availability:'chat',gameStatus:'outOfGame'}));f.setOpen(true);f.render()})()`);
 await check('friends');
 await p.evaluate("__r280Friends.setOpen(false)");
 await p.evaluate(`(async()=>{const g=__r278Game;__r278App.activateSection('live');await deepLegendsSections.load('live-arena')})()`);
 await p.until('__r278Game.state.live?.players?.length>0');
 await p.evaluate(`(()=>{const g=__r278Game;g.state.live.players.forEach((player,i)=>{player.gameName=__r288Leaks[(i*2)%__r288Leaks.length];player.tagLine=__r288Leaks[(i*2+1)%__r288Leaks.length]});g.renderLive()})()`);
 await check('live');
 await p.evaluate(`(()=>{const g=__r278Game;g.state.live={...g.state.live,phase:'InProgress',queueId:1700,gameMode:'CHERRY',arenaGrouped:true,players:g.state.live.players.map((player,i)=>({...player,arenaGroup:String(Math.floor(i/3)+1),arenaSquadKey:i<3?'mine':'premade:'+Math.floor(i/3),mySquad:i<3}))};g.renderLive()})()`);
 await check('arena');
 await p.evaluate("__r278App.activateSection('overview');document.getElementById('setting-mask-names').checked=false;document.getElementById('setting-mask-names').dispatchEvent(new Event('change'))");
 assert(await p.evaluate('__r288Leaks.some(value=>document.body.textContent.includes(value))'),'mask off must restore identities');
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
