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
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r79-roster-'));
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
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');let content=fs.readFileSync(file);if(pathname==='/gameplay.css'&&process.env.R79_ROSTER_MUTANT){content=content.toString();if(process.env.R79_ROSTER_MUTANT==='24cqi')content=content.replace('36cqi','24cqi');else if(process.env.R79_ROSTER_MUTANT==='300px')content=content.replace('36cqi,420px','36cqi,300px');else if(process.env.R79_ROSTER_MUTANT==='players-1fr')content=content.replace('.match-players { display: grid; width: auto; max-width: var(--match-roster-width,clamp(150px,18cqi,190px)); min-width: 0; grid-template-columns: repeat(2,minmax(0,max-content)); justify-content: end; justify-self: end; gap: 2px 14px;','.match-players { display: grid; width: var(--match-roster-width,clamp(150px,18cqi,190px)); min-width: 0; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 2px 8px;');else throw Error('unknown mutant');}res.end(content);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve();}},50);setTimeout(()=>{clearInterval(timer);resolve();},5000);})`);

 await evaluate(`new Promise(resolve=>{const t=setInterval(()=>{if(document.querySelector('#overview-content .summoner-strip')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);resolve();},8000);})`);
 const output=path.join(root,'output','playwright','r79');fs.mkdirSync(output,{recursive:true});
 await call('Emulation.setDeviceMetricsOverride',{width:1920,height:1080,deviceScaleFactor:1,mobile:false});
 await evaluate(`new Promise((resolve,reject)=>{const t=setInterval(()=>{if(document.querySelector('.match-summary:not(.is-arena) .match-player-name')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);reject(Error('match roster missing'));},8000);})`);
 // 名单宽度必须跟着名字长度走：这四条带子就是"加宽的是名字不是间隙"的判据。
 // 把 .match-players 改回 width:var(--match-roster-width) + repeat(2,1fr) 会让四档全部变成 420，
 // 6/8/12 三档立刻超出上带而变红。
 const WIDTH_BAND={6:[160,205],8:[205,250],12:[290,335],16:[375,420]};
 const results=[];
 const failures=[];
 for(const length of [6,8,12,16]) {
  await evaluate(`(()=>{for(const name of document.querySelectorAll('.match-summary:not(.is-arena) .match-player-name'))name.textContent='春江花月夜山河星辰流光云海清风明'.slice(0,${length});})()`);
  await evaluate(`document.fonts.ready`);
  await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
  const measurements=await evaluate(`(()=>{const cards=[...document.querySelectorAll('.match-summary:not(.is-arena)')];return cards.map(card=>{
   const main=card.querySelector('.match-main'), players=card.querySelector('.match-players');
   const right=Math.max(...['.match-champion','.match-kda','.match-stats'].map(s=>main.querySelector(s).getBoundingClientRect().right));
   const names=[...players.querySelectorAll('.match-player-name')];
   const lists=[...players.querySelectorAll('.match-team-list')];
   const columnGap=lists.length>1?lists[1].getBoundingClientRect().left-lists[0].getBoundingClientRect().right:null;
   return {width:players.getBoundingClientRect().width,height:card.getBoundingClientRect().height,gap:players.getBoundingClientRect().left-main.getBoundingClientRect().right,columnGap,spare:main.getBoundingClientRect().right-right,names:names.map(e=>({width:e.clientWidth,scroll:e.scrollWidth})),truncated:names.filter(e=>e.scrollWidth-e.clientWidth>1).length,overflow:[...card.querySelectorAll('*')].filter(e=>!e.closest('.match-team-list')&&!e.matches('.match-player-name')&&e.scrollWidth-e.clientWidth>1&&getComputedStyle(e).overflowX==='hidden').map(e=>({className:e.className,tag:e.tagName,parent:e.parentElement.className,text:e.textContent.slice(0,40),scroll:e.scrollWidth,width:e.clientWidth}))};
  });})()`);
  console.log(JSON.stringify({length,measurements}));results.push({length,measurements});
  assert.ok(measurements.length>0);
  for(const m of measurements){
   const verify=(name,fn)=>{try{fn();}catch(error){failures.push({length,name,message:error.message});}};
   // 名单块按内容收缩：名字短的时候不许把宽度撑满、在列尾留一片空转（用户原话
   // "只是把玩家名称宽度增大，不是增大间隙"），名字长的时候才吃满 420 的额度。
   verify('roster-width',()=>assert.ok(m.width<=420,`roster ${m.width} exceeds the 420 cap`));
   verify('roster-hugs-content',()=>assert.ok(m.width>=WIDTH_BAND[length][0]&&m.width<=WIDTH_BAND[length][1],`roster ${m.width} outside ${JSON.stringify(WIDTH_BAND[length])} for ${length}-char names`));
   verify('names',()=>assert.equal(m.truncated,0));
   verify('height',()=>assert.equal(m.height,116));
   verify('overflow',()=>assert.deepEqual(m.overflow.filter(e=>process.env.R79_CLIPPING_POLICY!=='existing'||!(e.className==='game-icon is-large is-champion-art'||e.tag==='STRONG'&&e.parent==='match-result-meta')),[]));
   // 两队之间只留一条正常的列间距，不是被 1fr 等分撑出来的大缝。
   verify('column-gap',()=>assert.equal(m.columnGap,14));
   verify('gap',()=>assert.ok(m.gap>=0,`roster overlaps the stats block by ${-m.gap}px`));
  }
 }
 const tag=(process.env.R79_ROSTER_MUTANT||'baseline')+(process.env.R79_CLIPPING_POLICY==='existing'?'-existing-clipping':'');
 fs.writeFileSync(path.join(output,tag+'.json'),JSON.stringify({clippingPolicy:process.env.R79_CLIPPING_POLICY||'strict',results,failures},null,2));
 const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,tag+'.png'),Buffer.from(shot.data,'base64'));
 assert.deepEqual(failures,[],'roster acceptance');
 console.log('R79 production roster Chromium PASS');
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
