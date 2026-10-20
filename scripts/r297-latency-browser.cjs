'use strict';
const {browser}=require('./r297-browser-harness.cjs');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const baseline=process.env.R297_BASELINE==='1',out=process.env.R297_BROWSER_OUT||path.join(process.env.R297_REPORT_DIR||path.join(require('node:os').tmpdir(),'r297-browser-proof'),'p1-browser');
const backend=process.argv[2];
(async()=>{
 const b=await browser({out,backend,sourceRoot:baseline?process.env.R297_BASELINE_SOURCE:undefined,
 inject:`window.__r297RecommendationReads=[];`,
 intercept:`if(pathname==='/api/status')return new Promise(resolve=>{const ready=()=>window.__r297?resolve(new Response(JSON.stringify(status),{headers:{'Content-Type':'application/json'}})):requestAnimationFrame(ready);ready();});
 if(['/api/gameplay/live','/api/gameplay/client-recommendation','/api/gameplay/champselect-latency'].includes(pathname))return nativeFetch(input,init);
 if(pathname==='/api/gameplay/phase')return Promise.resolve(new Response(JSON.stringify({phase:'ChampSelect'})));
 if(pathname==='/api/gameplay/recommendations'){const id=Number(params.get('championId'));__r297RecommendationReads.push(id);const delay=id===1?1800:id===2?1200:60;return new Promise(resolve=>setTimeout(()=>resolve(new Response(JSON.stringify({recommendations:{...live.recommendations,championId:id,resolvedPosition:params.get("position")}}))),delay));}
 if(pathname==='/api/gameplay/pro-runes')return Promise.resolve(new Response(JSON.stringify({pros:[],preparing:false})));
 if(pathname==='/api/gameplay/specialist-runes')return Promise.resolve(new Response(JSON.stringify({runes:[]})));`
 });
 const rows=[];
 try {
 await b.call('Page.navigate',{url:b.origin+'/?demo'});
 await b.until("window.__r297?.state.status?.clientView?.state==='ready'");
 await b.until(`fetch('/fixture/ready').then(r=>r.json()).then(v=>v.ready)`);
 await b.evaluate(`document.querySelector('[data-announcements-close]')?.click();document.querySelector('[data-section="live"]').click()`);
 await b.evaluate(`fetch('/fixture/start',{method:'POST'})`);
 await b.until('__r297.state.liveLoading && __r297.state.live?.players?.length===10');
 const initial=await b.evaluate('__r297.state.live.players[0].championId');assert.equal(initial,13);
 for(const id of [1,2,3]){
  const start=Date.now();await b.evaluate(`fetch('/fixture/select?id=${id}${(id===1||id===3) && !baseline?'&hover=1':''}',{method:'POST'})`);
  let ms;
  if(baseline){ms=await b.until(`Number(__r297.state.live?.players?.[0]?.championId)===${id}`,15000).catch(()=>-1)}
  else ms=await b.until(`Number(__r297.state.live?.players?.[0]?.championId || __r297.state.live?.players?.[0]?.championPickIntent)===${id} && Number(__r297.state.live?.players?.[9]?.championId)===${id+9} && document.querySelector('.live-player img[alt="${({1:"黑暗之女",2:"狂战士",3:"正义巨像"})[id]}"]') && document.querySelector('.recommendation-champion-summary[data-champion-id="${id}"] .champion-summary-main strong')?.textContent === ${JSON.stringify(({1:'黑暗之女',2:'狂战士',3:'正义巨像'})[id])}`,1000);
  const elapsed=Date.now()-start;
  const loading=await b.evaluate('__r297.state.liveLoading');
  rows.push({id,scenario:({1:'single-change-leading-hover-13-to-1',2:'rapid-change-locked-1-to-2',3:'rapid-change-hover-2-to-3'})[id],card_and_loading_ms:ms<0?null:elapsed,histories_loading:loading});
  if(!baseline){assert(elapsed<=(id===1?100:300),`champion ${id} ${elapsed}ms`);assert(loading,'full history completed before pick proof')}
  if(baseline)break;
 }
 if(!baseline){
  const clientMs=await b.until('__r297.state.live.clientRecommendation?.championId===3',1500);assert(clientMs<=1000);
  rows.push({client_local_ready_ms:clientMs+rows.at(-1).card_and_loading_ms});
 await b.until(`[...__r297.state.proRunes.keys()].some(k=>k.startsWith('3:')) && [...__r297.state.specialistRunes.keys()].some(k=>k.startsWith('3:'))`,1000);
 rows.push({pros_specialists_ready_before_full_history:await b.evaluate('__r297.state.liveLoading')});
 const cachedStarted=Date.now();await b.evaluate(`fetch('/fixture/select?id=3',{method:'POST'})`);const cachedMs=await b.until('__r297.state.live.players[0].championLocked && __r297.state.live.clientRecommendation?.championId===3');const cachedElapsed=Date.now()-cachedStarted;rows.push({locked_cached_recommendations_ms:cachedElapsed});assert(cachedElapsed<=1000);
 await b.evaluate(`window.__r297ChampionFlashes=[];window.__r297Observer=new MutationObserver(()=>{for(const n of document.querySelectorAll('.recommendation-champion-summary'))__r297ChampionFlashes.push(n.dataset.championId)});__r297Observer.observe(document.getElementById('live-content'),{subtree:true,childList:true})`);
  await b.until('!__r297.state.liveLoading',15000);
  assert.equal(await b.evaluate('__r297.state.live.players[0].championId'),3,'full/progress overwrote fast fields');
  assert.equal(await b.evaluate('__r297.state.live.players[9].championId'),12,'enemy card regressed');
  assert(await b.evaluate('__r297ChampionFlashes.every(id=>id==="3")'),'late recommendation flashed a superseded champion');
  rows.push({final_champion:await b.evaluate('__r297.state.live.players[0].championId'),recommendation_requests:await b.evaluate('__r297RecommendationReads')});
 }
 await b.shot('champselect');assert.deepEqual(b.errors,[]);
 for(const name of ['failure.json','failure.png'])fs.rmSync(path.join(out,name),{force:true});
 fs.writeFileSync(path.join(out,'metrics.json'),JSON.stringify({baseline,rows,errors:b.errors},null,2)+'\n');console.log(JSON.stringify({baseline,rows}));
 }catch(e){await b.shot('failure').catch(()=>{});fs.writeFileSync(path.join(out,'failure.json'),JSON.stringify({message:e.stack,errors:b.errors,state:await b.evaluate('({phase:window.__r297?.state.beacon,live:window.__r297?.state.live,loading:window.__r297?.state.liveLoading})').catch(()=>null)},null,2));throw e}
 finally{await b.stop()}
})().catch(e=>{console.error(e);process.exitCode=1});
