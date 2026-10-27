'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {waitForInstalledTarget}=require('./r265-persisted-ui.cjs');
const {waitFor,waitForPage}=require('./lib/wait-progress.cjs');
async function main(){
 const [port,out]=process.argv.slice(2);fs.mkdirSync(out,{recursive:true});
 const target=await waitForInstalledTarget({port,phase:'r313-published-check',out});
 require('./r276-windows-port-owner.cjs').verifyInstalledPortOwner({port:Number(port),expectedPID:Number(process.env.R265_EXPECTED_ELECTRON_PID),evidence:path.join(out,'port-owner.json')});
 const ws=new WebSocket(target.webSocketDebuggerUrl),pending=new Map();let sequence=0;
 await new Promise((yes,no)=>{ws.addEventListener('open',yes,{once:true});ws.addEventListener('error',no,{once:true})});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data),row=pending.get(m.id);if(row){pending.delete(m.id);m.error?row[1](Error(JSON.stringify(m.error))):row[0](m.result)}});
 function call(method,params={}){return new Promise((yes,no)=>{const id=++sequence,timer=setTimeout(()=>{pending.delete(id);no(Error('CDP timeout'))},60000);pending.set(id,[r=>{clearTimeout(timer);yes(r)},e=>{clearTimeout(timer);no(e)}]);ws.send(JSON.stringify({id,method,params}))})}
 async function evaluate(expression){const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value}
 try{
  assert(await waitForPage(evaluate,`document.readyState==='complete'&&typeof window.deepLegendsToast==='function'`,{idleMs:30000,hardMs:60000}),'installed application readiness');
  await waitFor(()=>evaluate(`fetch('/api/update/status').then(r=>r.json()).then(s=>!s.checking)`),{label:'startup update check completes',idleMs:45000,hardMs:60000});
  const checked=await evaluate(`(async()=>{const response=await fetch('/api/update/check',{method:'POST',headers:{Accept:'application/json'},signal:AbortSignal.timeout(45000)});return {status:response.status,body:await response.json()}})()`);
  assert.equal(checked.status,200);
  let finalStatus;await waitFor(async()=>{finalStatus=await evaluate(`fetch('/api/update/status').then(r=>r.json())`);return !finalStatus.checking},{label:'manual published update check completes',idleMs:45000,hardMs:60000});
  const evidence={manualResponse:checked,finalStatus};fs.writeFileSync(path.join(out,'old-client-check.json'),JSON.stringify(evidence,null,2));assert.equal(finalStatus.current,'0.13.1');assert.equal(finalStatus.latest,'0.13.2');assert(['available','ready','downloading','verifying'].includes(finalStatus.state));assert.equal(finalStatus.error,'');console.log(JSON.stringify(evidence));
  const screenshot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(out,'old-client-after-check.png'),Buffer.from(screenshot.data,'base64'));
 }finally{ws.close()}
}
main().catch(e=>{console.error(e);process.exitCode=1});
