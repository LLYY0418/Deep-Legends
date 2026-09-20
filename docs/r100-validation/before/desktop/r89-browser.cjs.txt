// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/current-game-layout.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r89-browser-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const url=await new Promise((resolve,reject)=>{let output=''; const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-1000)}`)));});
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},45000);pending.set(id,[value=>{clearTimeout(timer);resolve(value)},error=>{clearTimeout(timer);reject(error)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 console.log('CDP connected');
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 await call('Page.enable');
 await call('Runtime.enable');

 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.method==='Runtime.exceptionThrown'||m.method==='Runtime.consoleAPICalled'&&m.params.type==='error')console.log('browser error',JSON.stringify(m.params).slice(0,1800));});

 const diagnostics=[];
 const browserFixture = `
 {
 const original=window.fetch;
 window.r89Requests=[];window.r89Diagnostics=[];
 const wait=ms=>new Promise(r=>setTimeout(r,ms));
 const respond=x=>new Response(JSON.stringify(x),{headers:{'Content-Type':'application/json'}});
 const identities=new Map();let itemsCalls=0,retryFailed=false;
 window.fetch=async(input,init={})=>{
   const url=String(input);let body={};try{body=JSON.parse(init.body||'{}');}catch{}
   if(url==='/api/diagnostics/client'){window.r89Diagnostics.push(body);return fetchNative(input,init);}
   if(url==='/api/gameplay/items') { itemsCalls++;if(itemsCalls===1)return new Response('fixture transient failure',{status:504});return respond({items:[{id:3006,name:'轻灵之靴',iconPath:'ddragon:/cdn/16.18.1/img/item/3006.png'}]}); }
   if(['/api/gameplay/current-game','/api/gameplay/season-summary'].includes(url)){
     window.r89Requests.push({url,body,at:performance.now()});await wait(100);
     return respond(url.endsWith('current-game')?{status:'none',source:'OP.GG'}:{source:'OP.GG',queue:'RANKED',season:'S2026',overall:{games:20},champions:[]});
   }
   if(url==='/api/gameplay/overview' && init.method==='POST'){
     const info=body.gameName?{gameName:body.gameName,tagLine:body.tagLine||'KR1'}:identities.get(body.playerRef);
     if(info){
       const ref='r89-'+info.gameName;identities.set(ref,info);
       const record={url,body,at:performance.now()};window.r89Requests.push(record);
       if(info.gameName==='FixtureRetry' && body.count===20 && !retryFailed){retryFailed=true;await wait(100);record.completed=performance.now();return new Response('fixture completion failed',{status:504});}
       const response=await original(input,init);const data=await response.json();
       const templates=data.matches||[];data.player={...data.player,...info,playerRef:ref,region:'kr'};
       data.matches=Array.from({length:info.gameName==='FixtureEmpty'?0:body.count},(_,i)=>{const m=structuredClone(templates[i%templates.length]);m.gameId=9000+i;m.queueId=420;m.modeGroup='ranked';m.subjectParticipantId=m.participants[0].participantId;m.participants[0]={...m.participants[0],playerRef:ref,championId:64,championName:'李青',position:'jungle',itemIds:[3006,99999,0,0,0,0,0]};return m;});
       data.pagination={begIndex:0,count:data.matches.length,hasMore:info.gameName!=='FixtureEmpty'};data.overall={...data.overall,games:body.count};
       await wait(1200);record.completed=performance.now();return respond(data);
     }
   }
   return original(input,init);
 };
 }
 `;
 server=require('node:http').createServer(async(req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if(pathname==='/api/diagnostics/client'){
     let raw='';for await(const chunk of req)raw+=chunk;diagnostics.push(JSON.parse(raw));
     if(process.env.R89_BACKEND && process.env.R89_DATA_DIR){
       const token=fs.readFileSync(path.join(process.env.R89_DATA_DIR,'session-token'),'utf8').trim();
       const result=await fetch(process.env.R89_BACKEND+pathname,{method:'POST',headers:{'Content-Type':'application/json','X-Local-Token':token},body:raw});
       res.statusCode=result.status;res.end(await result.text());return;
     }
     res.statusCode=204;res.end();return;
   }
   if(pathname.startsWith('/api/') && (pathname==='/api/image'||pathname==='/api/champion-asset')){res.setHeader('Content-Type','image/png');res.end(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6txAAAAAASUVORK5CYII=','base64'));return;}
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream');
   if(pathname==='/demo-data.js'){res.end('const fetchNative=window.deepLegendsDemoNativeFetch || window.fetch.bind(window);\n'+fs.readFileSync(file,'utf8')+browserFixture);return;}
   res.end(fs.readFileSync(pathname==='/gameplay.js' && process.env.R89_GAMEPLAY_SOURCE ? process.env.R89_GAMEPLAY_SOURCE : file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 console.log('fixture server ready');
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 const until=async expression=>{const started=Date.now();while(Date.now()-started<12000){if(await evaluate('Boolean('+expression+')'))return;await new Promise(r=>setTimeout(r,50));}throw Error('condition timeout: '+expression);};
 console.log('page navigated');
 console.log('document state',await evaluate('document.readyState'));
 await until(`document.querySelector('#overview-content .summoner-strip')`);
 console.log('production UI ready');
 const records=[];
 for(const source of ['search','pro-players','champions']){
   await evaluate(`window.r89Requests=[];window.dispatchEvent(new CustomEvent('deep-legends:open-player',${JSON.stringify({detail:{gameName:'Fixture'+source,tagLine:'KR1',region:'kr',source}})}));`);
   await until(`window.r89Requests.filter(r=>r.url.endsWith('/overview')).length===2 && window.r89Requests.filter(r=>r.url.endsWith('/overview')).every(r=>r.completed)`);
   const rows=await evaluate('window.r89Requests');
   const first=rows.filter(r=>r.url.endsWith('/overview'))[0];
   const supplements=rows.filter(r=>!r.url.endsWith('/overview'));
   assert.equal(supplements.length,2,source+' duplicate/missing supplement');
   assert.ok(Math.max(first.at,...supplements.map(r=>r.at))-Math.min(first.at,...supplements.map(r=>r.at))<300,source+' waterfall');
   assert.ok(supplements.every(r=>r.at<first.completed),source+' waited for overview');
   assert.deepEqual(rows.filter(r=>r.url.endsWith('/overview')).map(r=>r.body.count),[5,20]);
   records.push({source,requests:rows.map(({url,at,completed,body})=>({url,at,completed,count:body.count}))});
   await evaluate(`document.getElementById('player-overlay-close')?.click()`);
 }
 // Real 30s backoff; keep the browser alive until the catalog retries and the
 // production renderer reports the deliberately absent ID exactly once.
 await evaluate(`new Promise(r=>setTimeout(r,31000))`);
 const evidence=await evaluate(`({diagnostics:window.r89Diagnostics,images:[...document.querySelectorAll('img')].filter(i=>i.src.includes('3006')).map(i=>({loaded:i.complete&&i.naturalWidth>0}))})`);
 fs.writeFileSync('/tmp/deep-legends-r89/browser-observed.json',JSON.stringify({records,evidence},null,2));
 assert.ok(evidence.diagnostics.some(d=>d.event==='catalog_client'&&d.reason==='failed'&&d.httpStatus===504));
 assert.ok(evidence.diagnostics.some(d=>d.event==='catalog_client'&&d.reason==='loaded'));
 assert.equal(evidence.diagnostics.filter(d=>d.event==='item_id_not_in_catalog'&&d.itemId===99999).length,1);
 assert.ok(evidence.images.some(i=>i.loaded),'catalog retry did not render a real image');
 // Verify the recovery UI against the actual DOM, not just state helpers.
 await evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{gameName:'FixtureRetry',tagLine:'KR1',region:'kr',source:'champions'}}))`);
 await until(`document.querySelector('#player-overlay-content [data-complete-overview]')`);
 assert.equal(await evaluate(`document.querySelectorAll('#player-overlay-content .match-entry').length`),5);
 await evaluate(`document.querySelector('#player-overlay-content [data-complete-overview]').click()`);
 await until(`document.querySelectorAll('#player-overlay-content .match-entry').length===20`);
 await evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{gameName:'FixtureEmpty',tagLine:'KR1',region:'kr',source:'champions'}}))`);
 await until(`document.querySelector('#player-overlay-content .summoner-strip')?.textContent.includes('FixtureEmpty')`);
 evidence.completionRetry=true;evidence.emptyMatches=true;
 const output=process.env.R89_OUTPUT||'/tmp/deep-legends-r89/browser';fs.mkdirSync(output,{recursive:true});
 const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'overview.png'),Buffer.from(shot.data,'base64'));
 fs.writeFileSync(path.join(output,'results.json'),JSON.stringify({records,evidence,transportDiagnostics:diagnostics},null,2));
 console.log('R89 Chromium PASS',JSON.stringify({entries:records.length,diagnostics:diagnostics.length}));
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
