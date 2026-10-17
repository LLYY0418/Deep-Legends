'use strict';
process.env.R282_SCREEN_CATALOG='1';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
async function run(){await withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']){
  const p=await page(theme);await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
  const manual=await p.evaluate(`(async()=>{const a=__r278App,fetchBase=window.fetch.bind(window),button=document.getElementById('refresh'),labels=[button.textContent.trim()],metrics=[];let overview=0,canceled=0,finishAt=performance.now()+1000;
   const beforeStreams=__r258Streams.length,started=performance.now(),reports=window.reportFlowDiagnostic;
   window.reportFlowDiagnostic=(e,r,f)=>{if(e==='manual_refresh_client')metrics.push(f);reports(e,r,f)};
   window.fetch=(url,options={})=>{const path=new URL(url,location.href).pathname;
    if(path==='/api/refresh')return new Promise(resolve=>setTimeout(()=>{finishAt=performance.now()+650;resolve(new Response('{}',{status:202,headers:{'Content-Type':'application/json'}}))},80));
    if(path==='/api/status')return Promise.resolve(new Response(JSON.stringify({...__r280Status,syncing:performance.now()<finishAt}),{headers:{'Content-Type':'application/json'}}));
    if(path==='/api/gameplay/overview'){overview++;options.signal?.addEventListener('abort',()=>canceled++);}
    return fetchBase(url,options);
   };
   const observer=new MutationObserver(()=>{const label=button.querySelector('.refresh-icon+span').textContent;if(labels.at(-1)!==label)labels.push(label)});observer.observe(button,{childList:true,subtree:true});
   const task=a.runManualRefresh();button.click();await task;await Promise.resolve();observer.disconnect();window.fetch=fetchBase;
   const elapsed=performance.now()-started,busyUntil=finishAt-started;
   let resync=0;window.addEventListener('deep-legends:resync',()=>resync++);a.state.liveEventsReady=true;__r258Streams.at(-1).onmessage({data:'ready'});
   return {overview,canceled,elapsed,busyUntil,labels,metrics,streamsBefore:beforeStreams,streamsAfter:__r258Streams.length,resync};})()`);
  assert.equal(manual.overview,1,JSON.stringify(manual));assert.equal(manual.canceled,0);assert.deepEqual(manual.labels,['重新读取','正在获取','重新读取']);assert(manual.elapsed>=manual.busyUntil);assert.equal(manual.streamsBefore,manual.streamsAfter);assert.equal(manual.resync,1);assert.equal(manual.metrics[0].label_changes,2);results.push({kind:'P2-manual',theme,...manual});
  await p.evaluate("__r278App.activateSection('favorites');__r278App.activateFavoritesPage('facade-collection')");await p.until("window.__r283Facade");
  for(const type of ['icons','banners']){
   await p.evaluate(`__r283Facade.state.view='${type}';__r283Facade.openDetail({id:1},{title:'${type==='icons'?'头像':'旗帜'}',hero:'安妮',state:'已拥有',image:'/api/image?path=fixture'})`);await p.until("document.getElementById('facade-detail-dialog').open");
   const art=await p.evaluate("(()=>{const d=document.getElementById('facade-detail-dialog'),a=d.querySelector('.dialog-art'),x=d.getBoundingClientRect(),y=a.getBoundingClientRect();return {top:y.top-x.top,left:y.left-x.left,right:x.right-y.right,imageWidth:d.querySelector('img').getBoundingClientRect().width}})()");assert(art.top<=1.1&&art.left<=1.1&&art.right<=1.1,JSON.stringify(art));assert(art.imageWidth<=128);results.push({kind:'P9-facade',theme,type,...art});await p.shot('r283-'+type+'-dialog');await p.evaluate("document.getElementById('facade-detail-dialog').close()");
  }
  await p.evaluate("__r278App.activateSection('suite')");await p.until("window.__r283Champions && window.__r282Suite");
  await p.evaluate("document.querySelector('[data-suite-tab=facade]').click()");await p.until("document.querySelector('.facade-identity')");
  const career=await p.evaluate("(()=>{const commit=document.querySelector('.facade-commit button'),status=document.querySelector('.suite-segment button'),selected=document.querySelector('.facade-film button.is-selected img'),other=document.querySelector('.facade-film button:not(.is-selected) img');return {commitFont:getComputedStyle(commit).fontSize,statusFont:getComputedStyle(status).fontSize,selectedOpacity:selected?getComputedStyle(selected).opacity:null,otherOpacity:other?getComputedStyle(other).opacity:null}})()");assert.equal(career.commitFont,career.statusFont);if(career.selectedOpacity)assert.equal(career.selectedOpacity,'1');if(career.otherOpacity)assert.equal(career.otherOpacity,'0.72');results.push({kind:'P13-fonts-selection',theme,...career});await p.shot('r283-career-before');
  await p.evaluate("document.querySelector('.facade-film button:not(.is-selected)')?.click()");await p.shot('r283-career-after');
  await p.evaluate("__r278App.activateSection('live')");await p.until("window.deepLegendsLiveArena");
  await p.evaluate("const stage=document.createElement('main');stage.id='r283-detail-stage';stage.style.cssText='position:fixed;inset:72px 18px 18px 100px;z-index:999;overflow:auto;background:var(--bg);padding:12px';document.body.append(stage)");
  const ranks=await p.evaluate(`(()=>{const g=__r278Game,s=document.getElementById('r283-detail-stage'),solo={tier:'PLATINUM',division:'I'},flex={tier:'DIAMOND',division:'IV'},players=[{soloRank:solo,flexRank:flex},{soloRank:solo},{},{soloRank:solo},{soloRank:solo}];s.innerHTML=players.map((p,i)=>'<article class="live-player"><div class="live-player-copy">'+deepLegendsLiveArena.identityLine({...p,position:'top'},i===3?'aram':i===4?'arena':'rift','上单 · 旧段位')+'</div></article>').join('');return [...s.querySelectorAll('.live-player-copy')].map(n=>n.textContent)})()`);assert(ranks[0].includes('单双 铂金 I')&&ranks[0].includes('组排 钻石 IV'));assert(ranks[1].includes('组排 未定级'));assert.equal(ranks[4],'');results.push({kind:'P11-rank-fixtures',theme,ranks});await p.shot('r283-rank-fixtures');
  await p.evaluate("document.getElementById('r283-detail-stage').innerHTML=[40,64,180].map(size=>'<img src=\"/bravery.svg\" width=\"'+size+'\" height=\"'+size+'\" style=\"margin:16px;background:white\">').join('')");await p.until("[...document.querySelectorAll('#r283-detail-stage img')].every(n=>n.complete)");await p.shot('r283-bravery-sizes');
  await p.evaluate("document.getElementById('r283-detail-stage').innerHTML=['英雄榜','海斗推荐','海斗图鉴列表','海斗图鉴详情','斗魂推荐','斗魂海克斯','实时对局','详情构建','战绩行','分享图'].map(name=>'<section style=\"display:inline-grid;gap:8px;margin:14px\"><b>'+name+'</b>'+__r283Champions.assetImage({source:'lcu',path:'/lol-game-data/assets/fixture.png',name:'海克斯'},'augment-icon')+'</section>').join('')");
  const bases=await p.evaluate("[...document.querySelectorAll('#r283-detail-stage .augment-icon')].map(n=>({background:getComputedStyle(n).backgroundColor,radius:getComputedStyle(n).borderRadius}))");if(theme==='light')for(const b of bases)assert.equal(b.background,'rgb(29, 37, 51)');results.push({kind:'P8-inventory-base',theme,bases});await p.shot('r283-augment-inventory');
 }
 assert.deepEqual(errors,[]);
});}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
