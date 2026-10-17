'use strict';
const assert=require('node:assert/strict'),{waitFor}=require('./lib/wait-progress.cjs'),{withBrowser}=require('./r280-browser-fixture.cjs');
async function run(){await withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
 await p.evaluate("(()=>{window.__r282CardEvents=[];const report=window.reportFlowDiagnostic;window.reportFlowDiagnostic=(name,reason,fields)=>{if(name==='career_cards_render')__r282CardEvents.push(fields);report?.(name,reason,fields)};const b=structuredClone(__r280Overview);for(const key of ['420','440']){b.rankedQueues||={};b.rankedQueues[key]={recentRanked:{...b.recentRanked,games:20,wins:12,losses:8,queueId:Number(key)},ability:{...b.ability,sampleGames:20},abilitySampleGames:20};}b.overall={...b.overall,games:20};b.seasonOverall={...b.seasonOverall,games:20};window.__r280Payload=b;window.__r280Gate=true;window.__r282Ready=structuredClone(__r278Game.state.status);})()");
 for(let cycle=1;cycle<=5;cycle++){
  await p.evaluate("(()=>{const s=__r282Ready;__r278Game.applyClientViewStatus({...s,connected:false,identityReady:false,clientView:{...s.clientView,state:'exiting'}});window.__r282ReconnectAt=Date.now()+30000;})()");
  await waitFor(()=>p.evaluate('Date.now()>=__r282ReconnectAt'),{idleMs:35000,hardMs:45000,label:'required 30-second disconnected interval'});
  await p.evaluate(`(()=>{const s=structuredClone(__r282Ready);s.clientView.generation=${cycle+1};__r278Game.applyClientViewStatus(s);})()`);await p.until("typeof __r280Release==='function' && __r278Game.activeTab().loading");
  await p.evaluate("__r280Release();window.__r280Release=null");await p.until("!__r278Game.activeTab().loading && __r278Game.activeTab().data?.rankedQueues?.['420']?.recentRanked?.games===20");
  const cards=await p.evaluate("(()=>{const t=__r278Game.activeTab();__r278Game.careerSectionEntries(t.data,t);return {generation:t.careerGeneration,source:t.careerStatsSource,events:__r282CardEvents.filter(e=>e.generation===t.careerGeneration&&e.source==='complete').slice(-3),texts:[...document.querySelectorAll('[data-career-key]')].map(n=>({key:n.dataset.careerKey,text:n.textContent}))}})()");
  assert.equal(cards.events.length,3,JSON.stringify(cards));for(const event of cards.events)assert.equal(event.games,20,JSON.stringify(cards));results.push({kind:'career-reconnect',cycle,...cards});await p.shot('reconnect-'+cycle);
 }
 assert.deepEqual(errors,[]);
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
