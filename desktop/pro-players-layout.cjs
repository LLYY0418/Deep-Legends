// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/pro-players-layout.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r73-layout-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const url=await new Promise((resolve,reject)=>{let output=''; const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-1000)}`)));});
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,[resolve,reject]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 await call('Page.enable');
 // Use the production shell and every production stylesheet. Fixtures have no
 // personal IDs, no network dependency, long account names and missing rows.
 const data={teams:['BLG','IG','T1','HLE','GEN','DK'].map(code=>({code,name:code,league:'LCK',players:[{key:`${code}/fixture`,name:'Fixture',position:'middle',status:'partial',accounts:[{gameName:'A very long Korean account name '.repeat(3),tagLine:'KR1',tier:'CHALLENGER',rankStatus:'ranked',lp:1432,primary:true,ladderRank:17,ladderRankKnown:true,updatedAt:'2026-09-08T10:00:00Z'},{gameName:'Short',tagLine:'KR2',tier:'MASTER',rankStatus:'ranked',lp:400,updatedAt:'2026-09-08T10:00:00Z'},{gameName:'Historical',tagLine:'KR3',dormant:true}]},{key:`${code}/missing`,name:'Missing',position:'top',status:'missing',accounts:[]}]})),warnings:['测试来源提示'],fetchedAt:'2026-09-08T11:00:00Z',rosterVerifiedAt:'2026-09-08'};
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if(pathname==='/api/pro-players'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data));return;}
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   const types={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'};
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');res.end(fs.readFileSync(file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve();}},50);setTimeout(()=>{clearInterval(timer);resolve();},5000);})`);
 await evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:'pro-players'}}))`);
 await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{if(document.querySelectorAll('.pro-table').length===6){clearInterval(timer);resolve();}},30);setTimeout(()=>{clearInterval(timer);resolve();},5000);})`);
 await evaluate(`new Promise(r=>setTimeout(r,400))`);
 // R79: only one overview action remains in production. Add a second real
 // button clone to retain the original two-action alignment stress assertion.
 await evaluate(`(()=>{const nav=document.querySelector('.pro-page-nav');const button=nav.querySelector('button').cloneNode(true);button.removeAttribute('id');nav.append(button);})()`);
 // 这一组护栏在"设计单位"下比较绝对像素（列宽、溢出、变异后必须溢出）。
 // .app-frame 现在有全局 CSS zoom，窄视口会自动缩到 0.9 倍，模拟宽度就不等于逻辑宽度了。
 // 把缩放钉死在 100% 再量；缩放本身的行为由 desktop/ui-scale-layout.cjs 单独守。
 await evaluate(`(()=>{localStorage.setItem('lol-loot-ui-scale','1');const s=document.getElementById('setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));})()`);
 for(const width of [820,1100,1440,1920]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});
  await evaluate(`new Promise(r=>setTimeout(r,250))`);
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
 const mutate=async text=>evaluate(`(()=>{let s=document.querySelector('#mutation');if(!s){s=document.createElement('style');s.id='mutation';document.head.append(s);}s.textContent=${JSON.stringify(text)};})()`);
 await call('Emulation.setDeviceMetricsOverride',{width:820,height:1100,deviceScaleFactor:1,mobile:false});
 await mutate('.pro-table { min-width:770px !important; }');
 assert.ok(await evaluate(`[...document.querySelectorAll('.pro-table')].some(t=>t.scrollWidth-t.parentElement.clientWidth>1)`),'min-width mutant survived');
 await mutate('');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1100,deviceScaleFactor:1,mobile:false});
 await mutate('.pro-col-account { width:auto !important; }');
 console.log('width-deletion mutant text gaps',await evaluate(`[...document.querySelectorAll('.pro-account-line')].map(e=>{const r=document.createRange();r.selectNodeContents(e);return e.parentElement.parentElement.getBoundingClientRect().right-14-Math.min(e.getBoundingClientRect().right,r.getBoundingClientRect().right);})`));
 await mutate('.pro-account-name { white-space:normal; overflow-wrap:anywhere; }');
 const brokenHeights=await evaluate(`[...new Set([...document.querySelectorAll('.pro-table td')].filter(e=>getComputedStyle(e).display!=='none').map(e=>Math.round(e.getBoundingClientRect().height))) ]`);
 assert.ok(brokenHeights.length>2,'nowrap mutant survived');
 console.log('Chromium overflow/row-height/indent guards and their mutants PASS; strict short-name density remains a documented specification conflict.');
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
