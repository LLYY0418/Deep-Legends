// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/overview-banner-layout.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r74-banner-'));
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
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   const types={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'};
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');res.end(fs.readFileSync(file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve();}},50);setTimeout(()=>{clearInterval(timer);resolve();},5000);})`);

 await evaluate(`new Promise(resolve=>{const t=setInterval(()=>{if(document.querySelector('#overview-content .summoner-strip')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);resolve();},8000);})`);
 // R79: the production artwork now wraps an img; retain the intrinsic-size guard.
 // Force a loaded 1215x717 image: jsdom cannot reproduce intrinsic image sizing.
 await evaluate(`(()=>{const img=document.querySelector('#overview-content .summoner-strip-art img');if(!img)throw Error('missing production banner');img.removeAttribute('srcset');img.src='data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="1215" height="717"><rect width="1215" height="717" fill="#648cba"/></svg>');return img.decode();})()`);
 const measure=()=>evaluate(`(()=>{const strip=document.querySelector('#overview-content .summoner-strip');const image=strip.querySelector('.summoner-strip-art');const box=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,w:r.width,h:r.height};};return {strip:box(strip),copy:box(strip.querySelector('.summoner-strip-copy')),highlights:box(strip.querySelector('.summoner-strip-highlights')),imagePosition:getComputedStyle(image).position,images:[...strip.querySelectorAll(':scope > img')].map(e=>({class:e.className,position:getComputedStyle(e).position}))};})()`);
 // 这一组护栏在"设计单位"下比较绝对像素（列宽、溢出、变异后必须溢出）。
 // .app-frame 现在有全局 CSS zoom，窄视口会自动缩到 0.9 倍，模拟宽度就不等于逻辑宽度了。
 // 把缩放钉死在 100% 再量；缩放本身的行为由 desktop/ui-scale-layout.cjs 单独守。
 await evaluate(`(()=>{localStorage.setItem('lol-loot-ui-scale','1');const s=document.getElementById('setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));})()`);
 for(const width of [820,1440,1920,2164]) {
  await call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});
  await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
  const m=await measure();
  assert.ok(m.strip.h<190,`banner expanded at ${width}: ${JSON.stringify(m)}`);
  assert.equal(m.imagePosition,'absolute');
  assert.ok(m.copy.y>=m.strip.y&&m.copy.y+m.copy.h<=m.strip.y+m.strip.h+1,'name escaped banner');
  assert.ok(m.highlights.x>m.copy.x,'rank/mastery must remain to the right');
  console.log(width,m);
 }
 // Reproduce the exact prior regression: the backdrop enters the grid because
 // the production :not(.summoner-strip-art) rule overrides its positioning.
 await evaluate(`(()=>{const strip=document.querySelector('#overview-content .summoner-strip');const extra=strip.querySelector('.summoner-strip-art img').cloneNode();extra.className='summoner-strip-art-backdrop';strip.prepend(extra);})()`);
 assert.ok((await measure()).strip.h>190,'old backdrop regression was not detected');
 await evaluate(`document.querySelector('.summoner-strip-art-backdrop').remove()`);
 console.log('Production Chromium banner layout and exact regression mutant PASS');

}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
