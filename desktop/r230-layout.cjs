'use strict';
const { evidencePath, requireEvidence } = require('../scripts/local-evidence.cjs');
// Actual app entry, production scripts/CSS, UI navigation in Chromium. Data and
// portraits are explicit local fixtures; these screenshots are not Windows/LCU evidence.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');const {prefix}=require('./r230-fixture.cjs');
requireEvidence('r211/mastery-crest-10.png');
const web=path.resolve(__dirname,'../backend/web'),output=evidencePath('r230'),profile=fs.mkdtempSync(path.join(os.tmpdir(),'r230-chrome-'));
let chrome,socket,server;
async function main(){
 fs.mkdirSync(output,{recursive:true});
 const source=fs.readFileSync(path.join(web,'gameplay.js'),'utf8');
 const instrument=`window.__r230Perf=[];for(const name of ['renderOverviewBodyContent','renderCareerSections','renderMatch','bindOverviewContent','prepareImages','applyRenderedMetricStyles','observeMatchTierVisibility']){const original=eval(name);eval(name+' = function(...args){const start=performance.now();try{return original(...args)}finally{window.__r230Perf.push({name,duration:performance.now()-start})}}');}`;
 server=require('node:http').createServer((req,res)=>{
  const p=new URL(req.url,'http://localhost').pathname;
  if(p==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream'});res.end();return}
  if(p.startsWith('/api/')){
   if(p==='/api/champion-asset' && req.url.includes('crest-and-banner-mastery')){res.setHeader('Content-Type','image/png');res.end(fs.readFileSync(evidencePath('r211/mastery-crest-10.png')));return}
   if(/asset|icon|image/.test(p)){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="72" height="72"><rect width="72" height="72" fill="#39465b"/><path d="M15 40L36 12 57 40 36 63Z" fill="#b78c45"/></svg>');return}
   res.setHeader('Content-Type','application/json');res.end('{}');return;
  }
  const file=path.resolve(web,p==='/'?'index.html':'.'+p);
  if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.writeHead(404);res.end();return}
  res.setHeader('Content-Type',{'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.woff2':'font/woff2'}[path.extname(file)]||'application/octet-stream');
  let data=fs.readFileSync(file);
  if(p==='/demo-data.js')data=String(data)+'\n'+prefix;
  if(p==='/gameplay.js')data=source.replace(/\}\)\(\);\s*$/,instrument+'\n})();');res.end(data);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 chrome=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${profile}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const address=await new Promise((resolve,reject)=>{let log='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);chrome.once('error',reject);chrome.stderr.on('data',data=>{log+=data;const m=log.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1])}});chrome.once('exit',code=>reject(Error(`Chrome exited ${code}: ${log.slice(-500)}`)))});
 socket=new WebSocket(address);await new Promise((r,j)=>{socket.addEventListener('open',r,{once:true});socket.addEventListener('error',j,{once:true})});
 let sequence=0;const pending=new Map();socket.addEventListener('message',event=>{const m=JSON.parse(event.data),c=pending.get(m.id);if(c){pending.delete(m.id);m.error?c.reject(Error(JSON.stringify(m.error))):c.resolve(m.result)}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++sequence;pending.set(id,{resolve,reject});socket.send(JSON.stringify({id,method,params,sessionId}))});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});const call=(m,p={})=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
 const until=predicate=>evaluate(`new Promise((resolve,reject)=>{const deadline=Date.now()+15000;const tick=()=>{if(${predicate})return resolve(true);if(Date.now()>deadline)return reject(Error('UI timeout: '+${JSON.stringify(predicate)}));setTimeout(tick,30)};tick()})`);
 const frame=()=>evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
 const click=async selector=>{await until(`document.querySelector(${JSON.stringify(selector)})`);await evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);await frame()};
 const screenshot=async name=>{await frame();const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,name+'.png'),Buffer.from(shot.data,'base64'))};
 await call('Page.enable');await call('Emulation.setDeviceMetricsOverride',{width:1280,height:1000,deviceScaleFactor:1,mobile:false});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:`localStorage.setItem('lol-loot-ui-scale','1');window.__r230Longtasks=[];window.__r230Errors=[];window.addEventListener('error',e=>window.__r230Errors.push(e.message));new PerformanceObserver(l=>window.__r230Longtasks.push(...l.getEntries().map(e=>({start:e.startTime,duration:e.duration})))).observe({type:'longtask',buffered:true});`});
 await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/?demo`});await until(`document.querySelector('#overview-content [data-overview-subpage="champion-table"]')`);
 const count=await evaluate(`document.querySelector('#overview-content .career-column').children.length`),measurements=[];
 for(const theme of ['dark','light']){
  await evaluate(`document.documentElement.dataset.theme=${JSON.stringify(theme)};document.getElementById('app-scroll').scrollTop=0`);
  for(const page of ['champion-table','masteries']){
   await click(`#overview-content [data-overview-subpage="${page}"]`);await until(`document.querySelector(${JSON.stringify(page==='masteries'?'[data-mastery-detail]':'.champion-data-table')})`);
   await screenshot(`${theme}-${page}-1280`);
   if(page==='masteries'){
    await evaluate(`document.querySelector('[data-mastery-detail]').focus()`);await frame();await screenshot(`${theme}-mastery-tooltip-1280`);
    const geometry=await evaluate(`(()=>{const p=document.querySelector('.mastery-details-portrait'),c=p.querySelector('.mastery-details-crest').getBoundingClientRect(),l=p.querySelector('.mastery-details-level').getBoundingClientRect(),portrait=p.querySelector('.game-icon').getBoundingClientRect();return{portrait:portrait.width,crestBottom:c.bottom,levelTop:l.top}})()`);assert.equal(geometry.portrait,72);assert.ok(geometry.levelTop>=geometry.crestBottom);measurements.push({theme,mastery:geometry});
   }
   await click('[data-overview-return]');const returned=await evaluate(`document.querySelector('#overview-content .career-column').children.length`);assert.equal(returned,count);
   await screenshot(`${theme}-overview-after-${page}`);
  }
  // Header detail is captured within its actual left column.
  await evaluate(`document.querySelector('.champion-performance').scrollIntoView({block:'start'})`);await screenshot(`${theme}-overview-card-headers`);
  for(const width of [1280,1000,780]){
   await call('Emulation.setDeviceMetricsOverride',{width,height:1000,deviceScaleFactor:1,mobile:false});
   await evaluate(`document.getElementById('app-scroll').scrollTop=0;document.querySelectorAll('.match-entry').forEach(e=>e.hidden=!['90001','90006','90004'].includes(e.dataset.matchId));`);
   await screenshot(`${theme}-match-cards-${width}`);
   const m=await evaluate(`[...document.querySelectorAll('.match-entry')].filter(e=>!e.hidden).map(e=>{const items=e.querySelector('.match-items'),badges=e.querySelector('.match-badges'),i=items?.getBoundingClientRect(),b=badges?.getBoundingClientRect();return{id:e.dataset.matchId,itemWidth:i?.width,badgeWidth:b?.width,sameRow:i&&b&&Math.abs(i.top-b.top)<5}})`);measurements.push({theme,width,cards:m});
  }
  await call('Emulation.setDeviceMetricsOverride',{width:1280,height:1000,deviceScaleFactor:1,mobile:false});
  await evaluate(`document.querySelectorAll('.match-entry').forEach(e=>e.hidden=false)`);
  await evaluate(`if(!document.querySelector('.match-entry[data-match-id="90001"] [data-match-detail="build"]'))document.querySelector('.match-entry[data-match-id="90001"] [data-toggle-match]').click()`);await click('.match-entry[data-match-id="90001"] [data-match-detail="build"]');
  await until(`document.querySelector('.build-player-selector')`);await evaluate(`document.querySelector('.build-player-selector').scrollIntoView({block:'center'})`);await screenshot(`${theme}-build-selector`);
  await evaluate(`if(!document.querySelector('.match-entry[data-match-id="90004"] [data-match-detail="build"]'))document.querySelector('.match-entry[data-match-id="90004"] [data-toggle-match]').click()`);await click('.match-entry[data-match-id="90004"] [data-match-detail="build"]');
  await evaluate(`document.querySelector('.match-entry[data-match-id="90004"] .build-player-selector').scrollIntoView({block:'center'})`);await screenshot(`${theme}-build-arena16`);
  await click('[data-section="champions"]');await click('[data-champion-mode="aram-mayhem"]');await click('[data-mayhem-view="atlas"]');await until(`document.querySelector('.mayhem-atlas-row')`);
  await click('[data-champion-mode="ranked"]');await click('[data-champion-mode="aram-mayhem"]');await until(`document.querySelector('.mayhem-atlas-row')`);await screenshot(`${theme}-atlas-return`);
  await click('[data-section="overview"]');await until(`document.querySelector('#overview-content .career-column')`);
 }
 const perf=await evaluate(`({longtasks:window.__r230Longtasks,functions:window.__r230Perf,errors:window.__r230Errors})`);
 assert.deepEqual(perf.errors,[]);
 const trace=[];let traceDone;const tracingComplete=new Promise(r=>traceDone=r);socket.addEventListener('message',event=>{const m=JSON.parse(event.data);if(m.method==='Tracing.dataCollected')trace.push(...m.params.value);if(m.method==='Tracing.tracingComplete')traceDone()});
 await call('Tracing.start',{categories:'devtools.timeline,v8.execute,disabled-by-default-devtools.timeline',transferMode:'ReportEvents'});
 await call('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/?demo&r230Stress=1`});await until(`document.querySelectorAll('.match-entry').length===200`);await frame();
 const stress=await evaluate(`({longtasks:window.__r230Longtasks,functions:window.__r230Perf,errors:window.__r230Errors,matchCount:document.querySelectorAll('.match-entry').length})`);
 await call('Tracing.end');await tracingComplete;fs.writeFileSync(path.join(output,'performance-trace.json'),JSON.stringify({traceEvents:trace}));
 fs.writeFileSync(path.join(output,'performance-summary.json'),JSON.stringify({normal:perf,stress},null,2));
 assert.deepEqual(stress.errors,[]);assert.ok(Math.max(0,...stress.longtasks.map(e=>e.duration))<=200,`stress longtask exceeded 200ms: ${JSON.stringify(stress.longtasks)}`);
 fs.writeFileSync(path.join(output,'layout.json'),JSON.stringify({fixture:true,environment:'macOS Chromium, actual app entry',measurements,perf,stress},null,2)+'\n');
 console.log('R230 production app Chromium: deep/light screenshots, round trips, mastery geometry PASS');console.log('longtask max:',Math.max(0,...perf.longtasks.map(e=>e.duration)));
}
main().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{socket?.close();chrome?.kill();server?.close();fs.rmSync(profile,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
