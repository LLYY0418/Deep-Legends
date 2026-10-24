'use strict';
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),os=require('node:os');
const {launchChrome}=require('./chrome-test-launcher.cjs');
const {cleanupChromeTest}=require('./chrome-test-cleanup.cjs');
const {waitFor,PAGE_PROGRESS_EXPRESSION}=require('./lib/wait-progress.cjs');
const {serveStaticAPI}=require('./r276-browser-api-fixtures.cjs');
async function browser({out,backend,intercept='',inject='',sourceRoot,assetFixture,onEvent=()=>{},transform=(name,data)=>data}) {
 const web=path.resolve(__dirname,'../backend/web'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'r297-browser-'));
 fs.mkdirSync(out,{recursive:true});let chrome,ws,server,origin;const errors=[];
 const stop=async()=>await cleanupChromeTest({children:[chrome],sockets:[ws],servers:[server],directories:[temp]});
 try {
 server=http.createServer(async(req,res)=>{
  const name=new URL(req.url,'http://fixture').pathname;
  if(backend && (['/api/player-tags','/api/ui-preferences','/api/announcements','/api/gameplay/live','/api/gameplay/client-recommendation','/api/gameplay/champselect-latency','/api/events'].includes(name)||name.startsWith('/fixture/'))){
   const proxy=http.request(backend+req.url,{method:req.method,headers:{'Content-Type':'application/json'}},up=>{res.writeHead(up.statusCode,up.headers);up.pipe(res)});proxy.on('error',e=>{res.writeHead(500);res.end(e.message)});req.pipe(proxy);res.on('close',()=>proxy.destroy());return;
  }
  if(assetFixture&&assetFixture(req,res))return;
  if(serveStaticAPI(req,res))return;
  if(name.startsWith('/api/')){const asset=name.includes('image')||name.includes('asset');res.writeHead(200,{'Content-Type':asset?'image/svg+xml':'application/json'});res.end(asset?'<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#587c83"/></svg>':'{}');return}
  const relative=name==='/'?'index.html':name.slice(1),file=path.join(web,['index.html','license-ui.js'].includes(relative)?'default/'+relative:relative);
  if(relative.includes('..')||!fs.existsSync(file)){res.writeHead(404);res.end();return}
  let data=fs.readFileSync(sourceRoot && ['gameplay.js','app.js'].includes(relative)?path.join(sourceRoot,relative):file);
  if(relative==='demo-data.js')data=Buffer.from(data.toString().replace('  const fixtures = new Map([',inject+'\n  const fixtures = new Map([').replace('    const params = new URLSearchParams(url.split("?")[1] || "");','    const params = new URLSearchParams(url.split("?")[1] || "");'+intercept));
  if(relative==='gameplay.js')data=Buffer.from(data.toString().replace('  bindSettings();','  window.__r297={state,loadLive,renderLive,renderOverview,activeTab,applyLivePlayerProgress,ensureMatchDetailRenderer,rerenderMatch,loadSettings};\n  bindSettings();').replace(',loadSettings};','};'));
  data=transform(relative,data);
  res.writeHead(200,{'Content-Type':({'.js':'text/javascript','.css':'text/css','.html':'text/html','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store'});res.end(data);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));origin='http://127.0.0.1:'+server.address().port;
 const launched=await launchChrome({executable:process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',args:['--headless=new','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],directory:out});chrome=launched.child;
 ws=new WebSocket(launched.endpoint);await new Promise((r,j)=>{ws.addEventListener('open',r,{once:true});ws.addEventListener('error',j,{once:true})});let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method))},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}))});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.method)onEvent(m.method,m.params);if(pending.has(m.id)){const [r,j]=pending.get(m.id);pending.delete(m.id);m.error?j(Error(JSON.stringify(m.error))):r(m.result)}if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);if(m.method==='Fetch.requestPaused'){const ok=m.params.request.url.startsWith(origin+'/')||m.params.request.url.startsWith('data:');void send(ok?'Fetch.continueRequest':'Fetch.failRequest',{requestId:m.params.requestId,...(ok?{}:{errorReason:'BlockedByClient'})},m.sessionId).catch(()=>{})}});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
 await call('Page.enable');await call('Runtime.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
 const until=async(expression,timeout=20000)=>{const result=await waitFor(()=>evaluate(`Boolean(${expression})`),{label:expression,idleMs:timeout,hardMs:timeout,intervalMs:15,progress:()=>evaluate(PAGE_PROGRESS_EXPRESSION)});return result.ms};
 const shot=async name=>{const s=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,name+'.png'),Buffer.from(s.data,'base64'))};
 return {origin,call,evaluate,until,shot,errors,stop};
 }catch(e){await stop();throw e}
}
module.exports={browser};
