// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/r100-browser.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r100-browser-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--disable-background-timer-throttling','--disable-renderer-backgrounding','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const url=await new Promise((resolve,reject)=>{let output=''; const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-1000)}`)));});
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},45000);pending.set(id,[value=>{clearTimeout(timer);resolve(value)},error=>{clearTimeout(timer);reject(error)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 console.log('CDP connected');
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.method==='Runtime.exceptionThrown'||m.method==='Runtime.consoleAPICalled'&&m.params.type==='error')console.log('browser error',JSON.stringify(m.params).slice(0,1800));});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 await call('Page.enable');
 await call('Runtime.enable');
 await call('Page.bringToFront');

 const output=process.env.R100_BROWSER_OUTPUT||path.join(root,'docs/r100-validation/browser');fs.mkdirSync(output,{recursive:true});
 const appSource=fs.readFileSync(process.env.R100_APP_SOURCE||path.join(web,'app.js'),'utf8');
 const status=appSource.slice(appSource.indexOf('  async function refreshStatus('),appSource.indexOf('function clearDisconnectedClientState('));
 assert.ok(status.includes('scheduleStatus()'));
 const lifecycle=appSource.slice(appSource.indexOf('  function setupBackendLifecycle('),appSource.indexOf('  window.desktopDiagnostics?.onError'));
 assert.ok(lifecycle.includes('function showFatal('));
 let activeImages=0,peakImages=0,failStatus=false;
 server=require('node:http').createServer((req,res)=>{
  const pathname=new URL(req.url,'http://localhost').pathname;
  if(pathname==='/image-queue.js'){res.setHeader('Content-Type','text/javascript');res.end(fs.readFileSync(process.env.R100_QUEUE_SOURCE||path.join(web,'image-queue.js')));return;}
  if(pathname==='/app.css'){res.end(fs.readFileSync(path.join(web,'app.css')));return;}
  if(pathname==='/api/champion-asset'){
   activeImages++;peakImages=Math.max(peakImages,activeImages);
   const timer=setTimeout(()=>{res.writeHead(404);res.end();},12000);
   res.once('close',()=>{clearTimeout(timer);activeImages--});return;
  }
  if(pathname==='/api/status'){res.writeHead(failStatus?503:200,{'Content-Type':'application/json'});res.end(JSON.stringify({connected:false}));return;}
  res.setHeader('Content-Type','text/html');res.end('<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><script src="/image-queue.js" defer></script><main id="content">已加载的战绩</main>');
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/'});
 for(let i=0;i<100;i++){if(await evaluate('typeof deepLegendsStatusRecovery==="function"'))break;await new Promise(r=>setTimeout(r,25));}
 await evaluate(`window.state={section:'gameplay',statusRequestToken:0,controllers:new Map()};window.STATUS_INTERVAL=10000;
 window.scheduleStatus=()=>{};window.clearDisconnectedClientState=()=>{};window.updateReadingOverlay=()=>{};window.renderStatus=()=>{};window.loadClientInstallations=async()=>{};
 document.body.insertAdjacentHTML('afterbegin','<div id="connection"><span></span></div><div id="notice" hidden></div><div id="launchpad"></div>');
 window.el={connection:document.querySelector('#connection'),notice:document.querySelector('#notice'),clientLaunchpad:document.querySelector('#launchpad')};
 window.escapeHTML=text=>String(text).replaceAll('<','&lt;');window.updateWorkspaceAvailability=()=>{};window.hideReadingOverlay=()=>{};
 window.desktopBackend={getState:async()=>({state:'running'}),onStateChanged:fn=>{window.backendStateChanged=fn;return()=>{}},restart:async()=>{window.restartRequested=true;return true}};
 ${lifecycle}
 setupBackendLifecycle();
 window.api=async()=>{const r=await fetch('/api/status');if(!r.ok)throw Error('fixture HTTP '+r.status);return r.json()};
 ${status}
 window.refreshStatus=refreshStatus;
 for(let i=0;i<12;i++){const img=document.createElement('img');img.width=64;img.height=64;img.setAttribute('data-queued-src','/api/champion-asset?i='+i);document.body.append(img)}
 `);
 await new Promise(r=>setTimeout(r,250));
 const elapsed=await evaluate('(async()=>{const t=performance.now();await refreshStatus();return performance.now()-t})()');
 assert.ok(elapsed<8000,'status starved behind images: '+elapsed);
 assert.ok(peakImages<=2,'browser image connection cap exceeded: '+peakImages);
 failStatus=true;await evaluate('(async()=>{for(let i=0;i<3;i++)await refreshStatus()})()');
 assert.equal(await evaluate('document.body.classList.contains("is-fatal")'),false,'poll failure must not be a fatal page');
 assert.equal(await evaluate('document.querySelector("#local-status-recovery").hidden'),false);
 assert.equal(await evaluate('document.querySelector("#content").textContent'),'已加载的战绩');
 failStatus=false;await evaluate('refreshStatus()');
 assert.equal(await evaluate('document.querySelector("#local-status-recovery").hidden'),true);
 const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'status-with-hung-images.png'),Buffer.from(shot.data,'base64'));
 await evaluate('backendStateChanged({state:"exited",code:1})');
 assert.equal(await evaluate('document.body.classList.contains("is-fatal")'),true,'confirmed process death must show the full-page alert');
 assert.equal(await evaluate('document.querySelector("#local-status-recovery").hidden'),true);
 assert.match(await evaluate('document.querySelector("#notice").textContent'),/本地数据服务已退出.*重启软件/);
 await evaluate('document.querySelector("[data-fatal-reload]").click()');
 assert.equal(await evaluate('window.restartRequested'),true);
 const fatalShot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'confirmed-backend-exit.png'),Buffer.from(fatalShot.data,'base64'));
 fs.writeFileSync(path.join(output,'result.json'),JSON.stringify({passed:true,statusMilliseconds:elapsed,peakImages,recovered:true,confirmedExitFatal:true,restartRequested:true},null,2));
 console.log('R100 Chromium PASS',JSON.stringify({elapsed,peakImages}));
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
