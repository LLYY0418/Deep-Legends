'use strict';
// Compare the protected 0.12.80 tracks against the candidate in one Chromium/font environment.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{execFileSync,spawn}=require('node:child_process');
function chromeClosed(chrome){return new Promise(resolve=>chrome.once('close',resolve));}
async function stopChrome(chrome,closed){
 if(chrome.exitCode===null && chrome.signalCode===null)chrome.kill('SIGTERM');
 let timer;try{await Promise.race([closed,new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('comparison Chrome did not exit before cleanup')),5000);})]);}finally{clearTimeout(timer);}
}
async function capture(file,png,profile){
 const chrome=spawn(process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--no-sandbox','--disable-background-networking','--no-first-run','--remote-debugging-port=0',`--user-data-dir=${profile}`,'about:blank'],{stdio:['ignore','ignore','pipe']});let ws;
 const closed=chromeClosed(chrome);
 try{const url=await new Promise((resolve,reject)=>{let log='';const timer=setTimeout(()=>reject(Error('comparison Chrome startup timeout')),20000);chrome.once('error',reject);chrome.stderr.on('data',data=>{log+=data;const match=log.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(match){clearTimeout(timer);resolve(match[1])}})});ws=new WebSocket(url);await new Promise((resolve,reject)=>{ws.addEventListener('open',resolve,{once:true});ws.addEventListener('error',reject,{once:true})});let sequence=0;const pending=new Map();ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [ok,no]=pending.get(m.id);pending.delete(m.id);m.error?no(Error(JSON.stringify(m.error))):ok(m.result)}});const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++sequence,timer=setTimeout(()=>reject(Error('comparison CDP timeout '+method)),15000);pending.set(id,[r=>{clearTimeout(timer);resolve(r)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}))});const {targetId}=await send('Target.createTarget',{url:'file://'+file}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});await send('Emulation.setDeviceMetricsOverride',{width:2880,height:960,deviceScaleFactor:1,mobile:false},sessionId);let ready=false;for(let i=0;i<100;i++){const r=await send('Runtime.evaluate',{expression:"document.readyState==='complete' && document.images.length===2 && [...document.images].every(n=>n.complete && n.naturalWidth)",returnByValue:true},sessionId);if(r.result.value){ready=true;break};await new Promise(r=>setTimeout(r,50))};assert(ready,'comparison images failed to load');const image=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:false},sessionId);fs.writeFileSync(png,Buffer.from(image.data,'base64'));await send('Browser.close');
 }finally{ws?.close();await stopChrome(chrome,closed);}
}
async function main(){
const root=path.resolve(__dirname,'..'),out=process.env.R269_GEOMETRY_OUT || path.join(os.tmpdir(),'r269-geometry'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'r269-geometry-'));
fs.mkdirSync(out,{recursive:true});const baseline=path.join(out,'baseline'),current=process.env.R269_GEOMETRY_CURRENT || path.join(out,'current');
try{
 const css=execFileSync('git',['show','c725ba27:backend/web/gameplay.css'],{cwd:root,encoding:'utf8'});fs.writeFileSync(path.join(temp,'gameplay.css'),css);
 const run=(folder,extra={})=>execFileSync(process.execPath,[path.join(root,'scripts/r266-browser.cjs')],{cwd:root,env:{...process.env,R266_BACKEND:process.env.R269_BACKEND || process.env.R266_BACKEND,R266_MATRIX:'1',R266_BROWSER_OUT:folder,...extra},stdio:'inherit',timeout:180000});
 run(baseline,{R266_WEB_OVERRIDE:temp,R266_MATRIX_BASELINE:'1'});if(!fs.existsSync(path.join(current,'result.json')))run(current);
 const a=JSON.parse(fs.readFileSync(path.join(baseline,'result.json'))).results,b=JSON.parse(fs.readFileSync(path.join(current,'result.json'))).results,key=r=>JSON.stringify([r.theme,r.w,r.zoom]),old=new Map(a.map(r=>[key(r),r]));assert.equal(a.length,60);assert.equal(b.length,60);let repaired=0,maxDifference=0;const differences=[];
 for(const row of b){const before=old.get(key(row));assert(before);assert.equal(row.overlaps.length,0,JSON.stringify(row));if(before.overlaps.length){repaired++;continue;}
  for(let i=0;i<row.rects.length;i++)for(let j=0;j<row.rects[i].blocks.length;j++){const x=before.rects[i].blocks[j],y=row.rects[i].blocks[j];assert.equal(x.visible,y.visible);if(!x.visible)continue;for(const field of ['left','top','width','height']){const difference=Math.abs(x[field]-y[field]);maxDifference=Math.max(maxDifference,difference);if(difference>1)differences.push({case:key(row),selector:x.selector,field,difference});}}
 }
 const report={cases:60,originalOverlapCases:repaired,maximumNormalDifference:maxDifference,differences,baseline:'c725ba27 protected CSS, same candidate DOM/fixture and Chromium environment; full original binary comparison remains a separate recorded local run'};fs.writeFileSync(path.join(out,'comparison.json'),JSON.stringify(report,null,2));assert.deepEqual(differences,[]);
 const png=folder=>fs.readFileSync(path.join(folder,'dark-p5-1440-auto.png')).toString('base64'),html=`<!doctype html><meta charset="utf-8"><title>0.12.80 / 0.12.82</title><style>body{margin:0;background:#111;color:#eee;font:24px sans-serif}header{display:flex}header span{width:1440px;padding:12px;box-sizing:border-box}main{display:flex}img{width:1440px;height:900px}</style><header><span>0.12.80 — 1440 × 自动</span><span>0.12.82 — 1440 × 自动</span></header><main><img src="data:image/png;base64,${png(baseline)}"><img src="data:image/png;base64,${png(current)}"></main>`;
 const file=path.join(out,'1440-auto-comparison.html');fs.writeFileSync(file,html);
 await capture(file,path.join(out,'1440-auto-comparison.png'),path.join(temp,'chrome'));
 console.log(JSON.stringify(report));
}finally{fs.rmSync(temp,{recursive:true,force:true});}

}
module.exports={chromeClosed,stopChrome};
if(require.main===module)main().catch(e=>{console.error(e);process.exitCode=1});
