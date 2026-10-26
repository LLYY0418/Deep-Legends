'use strict';
// Diagnostic only: untouched published 0.13.0 application, real update service.
const fs=require('node:fs'),path=require('node:path'),net=require('node:net'),assert=require('node:assert/strict'),{spawn,spawnSync}=require('node:child_process');
const [exe,folder,expected]=process.argv.slice(2),out=path.resolve(folder);
assert.match(expected,/^0\.13\.[01]$/);fs.mkdirSync(out,{recursive:true});
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
 const data=path.join(out,'backend-data');fs.mkdirSync(data,{recursive:true});
 fs.writeFileSync(path.join(data,'update-settings.json'),JSON.stringify({automatic:false,autoDownload:false,mirrors:[''],preferredSource:''}));
 const port=await new Promise(r=>{const s=net.createServer();s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>r(p));});});
 const child=spawn(exe,['--remote-debugging-port='+port,'--user-data-dir='+path.join(out,'user-data')],{env:{...process.env,LOL_LOOT_DATA_DIR:data},stdio:['ignore','pipe','pipe']});
 const logs=[];child.stdout.on('data',x=>logs.push(String(x)));child.stderr.on('data',x=>logs.push(String(x)));let ws;
 try{
  let target;for(let i=0;i<600;i++){try{target=(await fetch('http://127.0.0.1:'+port+'/json/list').then(r=>r.json())).find(t=>t.type==='page'&&t.url.startsWith('http://127.0.0.1'));if(target)break;}catch{}await sleep(100);}assert(target,'original packaged app renderer');assert(!target.url.includes('demo'));
  ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j;});let seq=0;const pending=new Map();
  ws.onmessage=e=>{const m=JSON.parse(e.data),p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p[1](Error(JSON.stringify(m.error))):p[0](m.result);}};
  const call=(method,params={})=>new Promise((r,j)=>{const id=++seq;pending.set(id,[r,j]);ws.send(JSON.stringify({id,method,params}));});
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
  const request=(route,options={})=>evaluate(`fetch(${JSON.stringify(route)},${JSON.stringify(options)}).then(async r=>{if(!r.ok)throw Error('HTTP '+r.status+': '+await r.text());return r.json()})`);
  await call('Runtime.enable');
  await request('/api/update/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({automatic:false,autoDownload:false,mirrors:[''],preferredSource:''})});
  const before=await request('/api/update/status');assert.equal(before.current,'0.13.0');assert(before.supported);
  const started=await request('/api/update/check',{method:'POST'}),samples=[started];let status=started;
  for(let i=0;i<900;i++){await sleep(200);status=await request('/api/update/status');samples.push(status);if(!status.checking&&status.latest===expected)break;}
  fs.writeFileSync(path.join(out,'update-status.json'),JSON.stringify({expected,originalExecutable:path.basename(exe),target:target.url,before,samples,final:status},null,2));
  assert.equal(status.current,'0.13.0');assert.equal(status.latest,expected);assert.equal(status.error,'');assert.equal(status.checking,false);
  assert.equal(status.state,expected==='0.13.1'?'available':'idle');assert.equal(status.manualOnly,false);
  const image=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(out,'old-client.png'),Buffer.from(image.data,'base64'));
  console.log(JSON.stringify({current:status.current,latest:status.latest,state:status.state,supported:status.supported,error:status.error,autoDownload:false,originalExecutable:path.basename(exe)}));
 }finally{ws?.close();if(child.exitCode===null)spawnSync('taskkill',['/PID',String(child.pid),'/T','/F'],{stdio:'inherit'});fs.writeFileSync(path.join(out,'app.log'),logs.join(''));}
})().catch(e=>{fs.writeFileSync(path.join(out,'failure.txt'),e.stack);console.error(e);process.exitCode=1;});
