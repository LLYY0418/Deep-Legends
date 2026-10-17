'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results})=>{const p=await page('dark');await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
for(const bottom of [false,true]){
 await p.evaluate(`(()=>{const g=__r278Game,t=g.activeTab();t.data={...__r280Overview,matches:Array.from({length:20},(_,i)=>({...__r280Overview.matches[i%__r280Overview.matches.length],gameId:90000+i})),capabilities:[{name:'match-history',state:'failed',historyStatus:'official'}],pagination:{begIndex:0,count:20,hasMore:true,historyServiceDown:true,localHasMore:false}};t.nextBegIndex=20;t.loading=t.loadingMore=false;t.matchFilter='all';g.renderOverview('players');const root=document.getElementById('app-scroll');root.scrollTop=${bottom?'root.scrollHeight':'0'};window.__r284Reads=[];window.__r284Fetch ||= window.fetch;window.fetch=(url,options)=>{if(String(url).includes('/api/gameplay/overview')){const input=options?.body?JSON.parse(options.body):Object.fromEntries(new URL(url,location.href).searchParams);__r284Reads.push(input);return Promise.resolve(new Response(JSON.stringify({...__r280Overview,matches:[{...__r280Overview.matches[0],gameId:89999}],pagination:{begIndex:20,count:1,hasMore:false},capabilities:[]})))}return __r284Fetch(url,options)};})()`);
 const before=await p.evaluate(`(()=>{const s=document.querySelector('[data-match-sentinel]');return {buttons:s.querySelectorAll('button').length,text:s.textContent}})()`);assert.equal(before.buttons,0);assert(before.text.includes('官方战绩服务暂时中断'));
 await p.evaluate("__r278Game.retryHistoryRegion(__r278Game.activeTab(),'outage-recovery')");
 const r=await p.evaluate('({reads:__r284Reads,cursor:__r278Game.activeTab().nextBegIndex})');assert.equal(r.reads.length,bottom?1:0);if(bottom)assert.equal(Number(r.reads[0].begIndex),20);results.push({bottom,before,...r});
}
}).catch(e=>{console.error(e);process.exitCode=1});
