// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/pro-players-layout.cjs
'use strict';
const {settleTopbar,waitViewport}=require('../scripts/lib/topbar-settle.cjs');
const {cleanupChromeTest}=require('../scripts/chrome-test-cleanup.cjs');
const {launchChrome}=require('../scripts/chrome-test-launcher.cjs');
const {serveStaticAPI}=require('../scripts/r276-browser-api-fixtures.cjs');
const {waitOutcome}=require('../scripts/lib/wait-progress.cjs');
const {navigateAppSection}=require('../scripts/lib/app-ready.cjs');
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'backend','web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r73-layout-'));
let proc,ws,server,captureFailure,mutationEvidence;const mutationHistory=[];
async function main(){
 const launched=await launchChrome({executable:chrome,args:['--headless=new','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],directory:process.env.R73_SCREENSHOT_DIR||path.join(os.tmpdir(),'pro-players-layout')});
 proc=launched.child;const url=launched.endpoint;
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 captureFailure=async error=>{
  const directory=process.env.R73_SCREENSHOT_DIR||path.join(os.tmpdir(),'pro-players-layout');
  fs.mkdirSync(directory,{recursive:true});
  let snapshot;try{snapshot=await evaluate(`(()=>{const bar=document.querySelector('.topbar'),search=document.querySelector('.player-search');return {innerWidth,sidebar:document.documentElement.dataset.sidebar,level:bar?.dataset.topbarCompact,settled:bar?.dataset.topbarSettled,loadedMs:performance.now(),font:getComputedStyle(document.body).fontFamily,userAgent:navigator.userAgent,dpr:devicePixelRatio,viewport:{width:visualViewport?.width,scale:visualViewport?.scale},mutation:{text:document.querySelector('#mutation')?.textContent,rules:[...(document.querySelector('#mutation')?.sheet?.cssRules||[])].map(r=>r.cssText)},search:{scrollWidth:search?.scrollWidth,clientWidth:search?.clientWidth},tables:[...document.querySelectorAll('.pro-table')].map(t=>({scrollWidth:t.scrollWidth,parentClientWidth:t.parentElement.clientWidth,offsetWidth:t.offsetWidth,rectWidth:t.getBoundingClientRect().width,minWidth:getComputedStyle(t).minWidth,width:getComputedStyle(t).width,zoom:getComputedStyle(t).zoom,display:getComputedStyle(t).display}))};})()`);}catch(diagnosticError){snapshot={diagnosticError:diagnosticError.message};}
  try{await call('DOM.enable');await call('CSS.enable');const {root}=await call('DOM.getDocument'),{nodeId}=await call('DOM.querySelector',{nodeId:root.nodeId,selector:'.pro-table'});fs.writeFileSync(path.join(directory,'matched-styles.json'),JSON.stringify(await call('CSS.getMatchedStylesForNode',{nodeId}),null,2)+'\n');}catch(error){snapshot.matchedStylesError=error.message;}
  fs.writeFileSync(path.join(directory,'failure.json'),JSON.stringify({file:'desktop/pro-players-layout.cjs',scenario:'pro-players-layout',error:error.stack||String(error),mutationEvidence,snapshot},null,2)+'\n');
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(directory,'failure.png'),Buffer.from(shot.data,'base64'));
 };
 const waitForDOM=async(label,expression)=>{
  let last;
  const outcome=await waitOutcome(async()=>{last=await evaluate(expression);return last.ready;},{idleMs:15000,hardMs:60000,intervalMs:50,progress:()=>last});
  if(!outcome.ok)throw Error(`${label}: ${outcome.reason}; waited ${outcome.ms} ms; last progress ${outcome.progress}`);
 };
 await call('Page.enable');
 // Use the production shell and every production stylesheet. Fixtures have no
 // personal IDs, no network dependency, long account names and missing rows.
 const data={teams:['BLG','IG','T1','HLE','GEN','DK'].map(code=>({code,name:code,league:'LCK',players:[{key:`${code}/fixture`,name:'Fixture',position:'middle',status:'partial',accounts:[{gameName:'A very long Korean account name '.repeat(3),tagLine:'KR1',tier:'CHALLENGER',rankStatus:'ranked',lp:1432,ladderRank:17,ladderRankKnown:true,updatedAt:'2026-09-08T10:00:00Z'},{gameName:'Short',tagLine:'KR2',tier:'MASTER',rankStatus:'ranked',lp:400,updatedAt:'2026-09-08T10:00:00Z'},{gameName:'Historical',tagLine:'KR3',dormant:true}]},{key:`${code}/missing`,name:'Missing',position:'top',status:'missing',accounts:[]}]})),warnings:['测试来源提示'],fetchedAt:'2026-09-08T11:00:00Z',rosterVerifiedAt:'2026-09-08'};
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if(pathname==='/api/pro-players'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data));return;}
   if(serveStaticAPI(req,res))return;
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   const types={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'};
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');res.end(fs.readFileSync(file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 // app.js publishes deepLegendsToast after registering its navigation listener.
 await navigateAppSection({evaluate,section:'pro-players'});
 await waitForDOM('pro-player six tables ready',`(()=>{const tables=document.querySelectorAll('.pro-table').length;return {ready:tables===6,tables,pending:document.querySelectorAll('.pro-pending').length,players:document.querySelectorAll('.pro-player-meta').length};})()`);
 await evaluate(`new Promise(r=>setTimeout(r,400))`);
 // R79: only one overview action remains in production. Add a second real
 // button clone to retain the original two-action alignment stress assertion.
 await evaluate(`(()=>{const nav=document.querySelector('.pro-page-nav');const button=nav.querySelector('button').cloneNode(true);button.removeAttribute('id');nav.append(button);})()`);
 // 这一组护栏在"设计单位"下比较绝对像素（列宽、溢出、变异后必须溢出）。
 // .app-frame 现在有全局 CSS zoom，窄视口会自动缩到 0.9 倍，模拟宽度就不等于逻辑宽度了。
 // 把缩放钉死在 100% 再量；缩放行为由界面缩放回归测试保护。
 await evaluate(`(()=>{localStorage.setItem('lol-loot-ui-scale','1');const s=document.getElementById('setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));})()`);
 for(const width of [820,1100,1440,1920]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});
  await waitViewport({evaluate,width});await settleTopbar({evaluate});
  const measurements=await evaluate(`(()=>{const tables=[...document.querySelectorAll('.pro-table')];return {overflow:tables.map(t=>t.scrollWidth-t.parentElement.clientWidth),heights:[...new Set(tables.flatMap(t=>[...t.querySelectorAll('td')].filter(e=>getComputedStyle(e).display!=='none').map(e=>Math.round(e.getBoundingClientRect().height))))],gap:[...document.querySelectorAll('.pro-account-line')].map(e=>{const range=document.createRange();range.selectNodeContents(e);return e.parentElement.parentElement.getBoundingClientRect().right-14-Math.min(e.getBoundingClientRect().right,range.getBoundingClientRect().right);}),indent:[getComputedStyle(document.querySelector('.pro-pending')).paddingLeft,getComputedStyle(document.querySelector('.pro-player-meta')).paddingLeft]};})()`);
  console.log(width,measurements);
  if(width===820)assert.ok(measurements.overflow.every(n=>n<=1),'820px horizontal overflow');
  if(width===1440 && !measurements.gap.every(n=>n<140)){console.warn('R73 density conflict: requested 46% column leaves >140px after short names.');if(process.env.R73_STRICT_DENSITY)assert.fail('1440px account dead space');}
  assert.ok(measurements.heights.length<=2,`inconsistent row heights at ${width}: ${measurements.heights}`);
  assert.equal(measurements.indent[0],measurements.indent[1]);
  const alignment=await evaluate(`(()=>{const headers=[...document.querySelectorAll('.pro-table thead tr:first-child th')].slice(2,5).map(e=>{const r=e.getBoundingClientRect();return {width:r.width,center:r.x+r.width/2};});return {headers,nav:[...document.querySelectorAll('.pro-page-nav button')].map(e=>e.getBoundingClientRect().top),warnings:!!document.querySelector('.pro-warnings'),marks:!!document.querySelector('.pro-team-filter i')};})()`);
  if(width>1000){assert.ok(Math.max(...alignment.headers.map(e=>e.width))-Math.min(...alignment.headers.map(e=>e.width))<=1,'last three columns must be equal width');assert.ok(Math.abs((alignment.headers[1].center-alignment.headers[0].center)-(alignment.headers[2].center-alignment.headers[1].center))<=1,'last three columns must have equal spacing');}
  assert.ok(Math.abs(alignment.nav[0]-alignment.nav[1])<=1,'overview actions must be on one row');
  assert.equal(alignment.warnings,false);assert.equal(alignment.marks,false);

  // R102: measure the actual collapsed view with all production styles loaded.
  const latest=await evaluate(`(()=>{const toggle=document.querySelector('#pro-primary-only');toggle.click();const rows=[...document.querySelectorAll('[data-pro-account]')];return {label:toggle.textContent.trim(),count:rows.length,badges:document.querySelectorAll('.pro-main-badge').length,first:rows.every(e=>e.textContent.includes('A very long Korean account name')&&e.closest('tr').classList.contains('pro-primary')),nameInsets:rows.map(e=>{const line=e.querySelector('.pro-account-line'),name=e.querySelector('.pro-account-name');return {inset:name.getBoundingClientRect().left-line.getBoundingClientRect().left,children:line.children.length};}),overflow:[...document.querySelectorAll('.pro-table')].map(t=>t.scrollWidth-t.parentElement.clientWidth)};})()`);
  assert.equal(latest.label,'只看最新账号');assert.equal(latest.count,6);assert.equal(latest.badges,0);assert.equal(latest.first,true);
  assert.ok(latest.nameInsets.every(m=>Math.abs(m.inset)<=1&&m.children===1),'removed badge must not reserve an empty slot');
  assert.ok(latest.overflow.every(n=>n<=1),'collapsed accounts must not overflow');
  console.log('R102 latest-only',width,JSON.stringify(latest));
  await evaluate(`document.querySelector('#pro-primary-only').click()`);

  if(process.env.R73_SCREENSHOT_DIR){fs.mkdirSync(process.env.R73_SCREENSHOT_DIR,{recursive:true});const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(process.env.R73_SCREENSHOT_DIR,`pro-players-${width}.png`),Buffer.from(shot.data,'base64'));}
 }
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'light'}]});
 await evaluate(`(()=>{const select=document.querySelector('#setting-theme');select.value='light';select.dispatchEvent(new Event('change',{bubbles:true}));})()`);
 assert.equal(await evaluate(`document.documentElement.dataset.theme`),'light');
 await evaluate(`new Promise(r=>setTimeout(r,250))`);
 const themeColors=await evaluate(`({lp:getComputedStyle(document.querySelector('.pro-lp-value')).color,ladder:getComputedStyle(document.querySelector('.pro-ladder-rank')).color})`);
 assert.notEqual(themeColors.lp,themeColors.ladder,'LP and ladder need distinct theme colors');
 if(process.env.R73_SCREENSHOT_DIR){const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(process.env.R73_SCREENSHOT_DIR,'pro-players-light.png'),Buffer.from(shot.data,'base64'));}
 // The same geometry assertions must observe the specified broken CSS.
 const mutate=async text=>{
  mutationEvidence=await evaluate(`new Promise(resolve=>{let s=document.querySelector('#mutation');if(!s){s=document.createElement('style');s.id='mutation';document.head.append(s);}s.textContent=${JSON.stringify(text)};const samples=[],sample=()=>{const tables=[...document.querySelectorAll('.pro-table')];samples.push({at:performance.now(),sheet:{disabled:s.sheet?.disabled,media:s.sheet?.media?.mediaText,connected:s.isConnected,rules:[...(s.sheet?.cssRules||[])].map(r=>r.cssText)},tables:tables.map(t=>({connected:t.isConnected,matches:t.matches('.pro-table'),inline:t.style.cssText,minWidth:getComputedStyle(t).minWidth,scrollWidth:t.scrollWidth,parentClientWidth:t.parentElement.clientWidth}))});if(samples.length===4)resolve(samples);else requestAnimationFrame(sample);};sample();})`);
  mutationHistory.push({css:text,samples:mutationEvidence});const directory=process.env.R73_SCREENSHOT_DIR||path.join(os.tmpdir(),'pro-players-layout');fs.mkdirSync(directory,{recursive:true});fs.writeFileSync(path.join(directory,'mutation-evidence.json'),JSON.stringify(mutationHistory,null,2)+'\n');
 };
 await call('Emulation.setDeviceMetricsOverride',{width:820,height:1100,deviceScaleFactor:1,mobile:false});
 await waitViewport({evaluate,width:820});await settleTopbar({evaluate});
 await mutate('.pro-table { min-width:770px !important; }');
 assert.ok(await evaluate(`[...document.querySelectorAll('.pro-table')].some(t=>t.scrollWidth-t.parentElement.clientWidth>1)`),'min-width mutant survived');
 await mutate('');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1100,deviceScaleFactor:1,mobile:false});
 await waitViewport({evaluate,width:1440});await settleTopbar({evaluate});
 await mutate('.pro-col-account { width:auto !important; }');
 console.log('width-deletion mutant text gaps',await evaluate(`[...document.querySelectorAll('.pro-account-line')].map(e=>{const r=document.createRange();r.selectNodeContents(e);return e.parentElement.parentElement.getBoundingClientRect().right-14-Math.min(e.getBoundingClientRect().right,r.getBoundingClientRect().right);})`));
 await mutate('.pro-account-name { white-space:normal; overflow-wrap:anywhere; }');
 const brokenHeights=await evaluate(`[...new Set([...document.querySelectorAll('.pro-table td')].filter(e=>getComputedStyle(e).display!=='none').map(e=>Math.round(e.getBoundingClientRect().height))) ]`);
 assert.ok(brokenHeights.length>2,'nowrap mutant survived');
 console.log('Chromium overflow/row-height/indent guards and their mutants PASS; strict short-name density remains a documented specification conflict.');
}
main().catch(async e=>{console.error(e);try{await captureFailure?.(e);}catch(diagnosticError){console.error(diagnosticError);}process.exitCode=1;}).finally(()=>cleanupChromeTest({children:[proc],sockets:[ws],servers:[server],directories:[temp]}));
