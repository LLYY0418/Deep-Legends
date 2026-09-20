'use strict';
// Real Chromium, offline demo fixtures; this is not a Windows/LCU validation.
const {spawn}=require('node:child_process'), fs=require('node:fs'), path=require('node:path'), os=require('node:os'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),web=path.join(root, "backend", "web"),out=path.join(root,'docs/r115-validation');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r115-browser-'));let proc,ws,server;
async function main(){
 const chrome=process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
 proc=spawn(chrome,['--headless=new','--disable-background-networking','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const endpoint=await new Promise((resolve,reject)=>{let output='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-500)}`)));});
 ws=new WebSocket(endpoint);await new Promise((r,j)=>{ws.addEventListener('open',r,{once:true});ws.addEventListener('error',j,{once:true});});
 let seq=0;const pending=new Map(),errors=[],blocked=[];let currentCase='',pageLoaded=null;
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},20000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [r,j]=pending.get(msg.id);pending.delete(msg.id);msg.error?j(Error(JSON.stringify(msg.error))):r(msg.result);}
  if(msg.method==='Page.loadEventFired')pageLoaded?.();
  if(msg.method==='Runtime.exceptionThrown'){errors.push({case:currentCase,details:msg.params.exceptionDetails});console.error(currentCase,JSON.stringify(msg.params.exceptionDetails));}
  if(msg.method==='Fetch.requestPaused'){
   const url=msg.params.request.url,ok=url.startsWith(origin+'/')||url.startsWith('data:');if(!ok)blocked.push(url);
   void send(ok?'Fetch.continueRequest':'Fetch.failRequest',{requestId:msg.params.requestId,...(ok?{}:{errorReason:'BlockedByClient'})},msg.sessionId).catch(error=>{if(!String(error).includes('Invalid InterceptionId'))console.error(error);});
  }
 });
 let mode='after',failModule=false;
 const before=process.env.R115_BEFORE_DIR;
 const bytes={};
 server=require('node:http').createServer((req,res)=>{
  const name=new URL(req.url,'http://fixture').pathname;
  if(name==='/api/events'){res.writeHead(204);res.end();return;}
  if(name.startsWith('/api/')){if(name.includes('image')||name.includes('asset')||name.includes('art')){res.writeHead(200,{'Content-Type':'image/svg+xml'});res.end('<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"/>');}else{res.writeHead(200,{'Content-Type':'application/json'});res.end('{}');}return;}
  if(failModule&&name==='/pro-players.js'){res.writeHead(503);res.end('fixture failure');return;}
  const relative=name==='/'?'index.html':name.slice(1);
  if(relative.includes('..')){res.writeHead(400);res.end();return;}
  const override=before&&path.join(before,'web',relative),file=mode==='before'&&override&&fs.existsSync(override)?override:path.join(web,relative);
  if(!fs.existsSync(file)){res.writeHead(404);res.end();return;}
  const data=fs.readFileSync(file);if(/\.(js|css)$/.test(file)){(bytes[currentCase]||={})[relative]=data.length;}
  res.writeHead(200,{'Content-Type':({'.js':'text/javascript','.css':'text/css','.html':'text/html','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store'});res.end(data);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const origin='http://127.0.0.1:'+server.address().port;
 const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);await call('Page.enable');await call('Runtime.enable');await call('Network.enable');await call('Network.setCacheDisabled',{cacheDisabled:true});await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 const until=async(expression)=>{for(let i=0;i<160;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,50));}console.error(await evaluate('JSON.stringify({notice:document.querySelector("[data-section-loading]")?.outerHTML,scripts:[...document.scripts].map(s=>s.src)})'));throw Error('timeout: '+expression);};
 async function navigate(section,label){currentCase=label;console.log('case',label);const loaded=new Promise(resolve=>{pageLoaded=resolve});await call('Page.navigate',{url:origin+'/?demo&section='+section});await new Promise((resolve,reject)=>{const timer=setTimeout(async()=>{console.error('LOAD SNAPSHOT',await evaluate('JSON.stringify({url:location.href,ready:document.readyState,notice:document.querySelector("[data-section-loading]")?.outerHTML,scripts:[...document.scripts].map(s=>s.src)})'));reject(Error('page load timeout '+label));},15000);loaded.then(()=>{clearTimeout(timer);resolve();});});await until('document.body?.classList.contains("is-demo") && document.querySelector("#app-frame")?.inert === false');await until('!document.querySelector("[data-section-loading]")');}
 if(before){mode='before';await navigate('overview','before');await new Promise(r=>setTimeout(r,500));}
 mode='after';await navigate('overview','after');await new Promise(r=>setTimeout(r,500));
 for(const file of ['champions.js','suite.js','pro-players.js'])assert.equal(bytes.after[file],undefined,`${file} loaded on overview`);
 const checklist=[];
 for(const section of (process.env.R115_SECTION?[process.env.R115_SECTION]:['overview','champions','live','favorites','pro-players','suite','settings','friends'])){
  await navigate(section,'link-'+section);
  if(section==='friends')await until('document.getElementById("friends-dock").classList.contains("is-open")');
  else assert.equal(await evaluate(`document.getElementById(${JSON.stringify(section+'-panel')}).hidden`),false,section);
  if(section==='suite')await until('document.querySelector("[data-watch-card]")');
  const initial=await evaluate('document.querySelectorAll("script[data-section-module]").length');
  if(section!=='friends')for(let i=0;i<3;i++)await evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:${JSON.stringify(section)}}}))`);
  assert.equal(await evaluate('document.querySelectorAll("script[data-section-module]").length'),initial);
  checklist.push({section,directRefresh:true,duplicateNodes:false});
 }
 await navigate('favorites','collection-to-tools');await evaluate(`document.querySelector('[data-favorites-page="account"]').click()`);await until(`document.querySelector('[data-navigate-suite="sweep"]')`);await evaluate(`document.querySelector('[data-navigate-suite="sweep"]').click()`);await until('document.querySelector("#suite-panel").hidden === false && !document.querySelector("[data-section-loading]")');
 assert.equal(await evaluate(`document.querySelector('[data-suite-tab="sweep"]').getAttribute('aria-selected')`),'true');
 currentCase='failure';failModule=true;await call('Page.navigate',{url:origin+'/?demo&section=pro-players'});await until('document.querySelector("[data-section-loading][role=alert]")');failModule=false;await evaluate('document.querySelector("[data-section-loading] button").click()');await until('!document.querySelector("[data-section-loading]")');
 const resources=label=>Object.entries(bytes[label]||{}).filter(([name])=>name!=='demo-data.js').reduce((sum,[,size])=>sum+size,0);
 const result={platform:process.platform,engine:'Headless Chrome',fixture:'local demo, external requests blocked; not Windows or LCU',before:before?resources('before'):null,after:resources('after'),resources:bytes,checklist,collectionToTools:true,failureRetry:true,errors,blocked};
 fs.writeFileSync(path.join(out,'browser.json'),JSON.stringify(result,null,2));
 assert.deepEqual(errors,[],'page exceptions');console.log(JSON.stringify({before:result.before,after:result.after,checklist,errors:errors.length}));
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
