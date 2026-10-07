'use strict';
// Production renderer with synthetic local API data; no real Windows/LCU claims.
const {spawn}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),http=require('node:http'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),web=path.join(root,'backend/web'),out=path.join(root,'docs/history/reports/r244');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r244-chromium-'));let chrome,ws,server,origin;const errors=[],results=[];
const inject=`
 const makeRows=n=>Array.from({length:n},(_,i)=>({championId:61,championName:'奥莉安娜',win:i%2===0,kills:2,deaths:1,assists:3,createdAt:Date.now()-60*86400000-i*60000}));
 for(const [i,p] of live.players.entries()){p.historyState='ok';p.recentGames=makeRows(i===0?10:i===2?3:i===3?0:8);p.modeStats={games:10,wins:5,losses:5,winRate:50,kda:2.27};p.recentRankedRecord={games:10,wins:5,losses:5};if(i===3)p.historyState='empty';}
 live.players[0].gameName='自己十局';live.players[2].gameName='六十天前玩家';live.players[3].gameName='本模式空战绩';
`;
(async()=>{
 fs.mkdirSync(out,{recursive:true});
 chrome=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const endpoint=await new Promise((resolve,reject)=>{let output='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);chrome.once('error',reject);chrome.stderr.on('data',c=>{output+=c;const match=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(match){clearTimeout(timer);resolve(match[1])}})});
 ws=new WebSocket(endpoint);await new Promise((r,j)=>{ws.addEventListener('open',r,{once:true});ws.addEventListener('error',j,{once:true})});let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method))},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}))});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [r,j]=pending.get(m.id);pending.delete(m.id);m.error?j(Error(JSON.stringify(m.error))):r(m.result)}if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);if(m.method==='Fetch.requestPaused'){const allowed=m.params.request.url.startsWith(origin+'/')||m.params.request.url.startsWith('data:');void send(allowed?'Fetch.continueRequest':'Fetch.failRequest',{requestId:m.params.requestId,...(allowed?{}:{errorReason:'BlockedByClient'})},m.sessionId).catch(()=>{})}});
 server=http.createServer((req,res)=>{
  const name=new URL(req.url,'http://fixture').pathname;
  if(name==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream'});res.write('event: heartbeat\ndata: {}\n\n');const timer=setInterval(()=>res.write('event: heartbeat\ndata: {}\n\n'),2000);req.on('close',()=>clearInterval(timer));return}
  if(name==='/api/license/status'){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({state:'ACTIVE',generation:1,message:''}));return}
  if(name.startsWith('/api/')){const asset=name.includes('image')||name.includes('asset');res.writeHead(200,{'Content-Type':asset?'image/svg+xml':'application/json'});res.end(asset?'<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#587c83"/></svg>':'{}');return}
  const relative=name==='/'?'index.html':name.slice(1),file=path.join(web,relative);if(relative.includes('..')||!fs.existsSync(file)){res.writeHead(404);res.end();return}
  let data=fs.readFileSync(file);if(relative==='demo-data.js')data=Buffer.from(data.toString().replace('  const fixtures = new Map([',inject+'\n  const fixtures = new Map(['));
  res.writeHead(200,{'Content-Type':({'.js':'text/javascript','.css':'text/css','.html':'text/html','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store'});res.end(data);
 });await new Promise(r=>server.listen(0,'127.0.0.1',r));origin='http://127.0.0.1:'+server.address().port;
 const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
 await call('Page.enable');await call('Runtime.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1100,deviceScaleFactor:1,mobile:false});
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
 const until=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,100))}throw Error('timeout '+expression)};
 await call('Page.navigate',{url:origin+'/?demo&section=overview'});await until(`document.querySelector('#startup-loading').hidden`);await evaluate(`document.querySelector('[data-section="live"]').click()`);await until(`document.querySelector('[data-recommendation-tab="insight"]')`);
 await evaluate(`document.querySelector('[data-recommendation-tab="insight"]').click()`);
 await until(`document.querySelector('.live-player-list')?.textContent.includes('自己十局')`);
 const summaries=await evaluate(`Array.from(document.querySelectorAll('.live-player')).map(n=>({text:n.textContent,rows:n.nextElementSibling?.querySelectorAll('.insight-match').length}))`);
 const self=summaries.find(p=>p.text.includes('自己十局')),old=summaries.find(p=>p.text.includes('六十天前玩家')),empty=summaries.find(p=>p.text.includes('本模式空战绩'));
 assert(self&&/近 10 局/.test(self.text)&&self.rows===10);assert(old&&/近 3 局/.test(old.text)&&old.rows===3);assert(empty&&/本模式暂无战绩/.test(empty.text)&&!empty.text.includes('近 10 局'));
 for(const theme of ['dark','light']){await evaluate(`document.documentElement.dataset.theme='${theme}'`);await new Promise(r=>setTimeout(r,100));const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});fs.writeFileSync(path.join(out,'champselect-history-'+theme+'.png'),Buffer.from(shot.data,'base64'))}
 results.push({summaries});assert.equal(errors.length,0,JSON.stringify(errors));fs.writeFileSync(path.join(out,'chromium.json'),JSON.stringify({scope:'Production Chromium / synthetic local fixtures; real Windows cold-launch pending.',results,errors},null,2));console.log(JSON.stringify({success:true,rows:summaries.length,errors}));
})().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{ws?.close();chrome?.kill();server?.closeAllConnections();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100})});
