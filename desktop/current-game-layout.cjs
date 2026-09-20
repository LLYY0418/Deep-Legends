// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/current-game-layout.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'backend','web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'current-game-layout-'));
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
 await call('Runtime.enable');
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.method==='Runtime.exceptionThrown'||m.method==='Runtime.consoleAPICalled'&&m.params.type==='error')console.log('browser error',JSON.stringify(m.params).slice(0,1800));});
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if (process.env.DIAGNOSTICS_1555_LAYOUT === '1' && pathname === '/api/events') { res.writeHead(200, {'Content-Type':'text/event-stream','Cache-Control':'no-cache'}); res.write('data: ready\n\n'); return; }
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   const types={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'};
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');
   if(pathname==='/demo-data.js') {
     const game={status:'active',source:'OP.GG',checkedAt:new Date().toISOString(),startedAt:new Date(Date.now()-600000).toISOString(),gameId:'fixture',queue:'单排/双排',map:'召唤师峡谷',teams:['blue','red'].map((side,j)=>({side,averageRank:{tier:'CHALLENGER'},averageLP:1920,players:Array.from({length:5},(_,i)=>({playerRef:`fixture-${j}-${i}`,gameName:'玩家 '+(j*5+i+1),tagLine:'KR1',championId:64,championName:'李青',summonerLevel:701,spells:[4,11],runes:[8005,8100],rank:{tier:i%2?'MASTER':'CHALLENGER',leaguePoints:i%2?447:1920},preferredPosition:i===4?'':'jungle',streak:i===0?(j?'loss':'win'):'',recent:Array.from({length:[6,8,10,8,10][i]},(_,n)=>({championId:64,championName:'李青',win:n%2===0,spells:[4,11]}))}))}))};
     res.end(fs.readFileSync(file,'utf8')+`
{const identities=new Map();window.addEventListener("deep-legends:open-player",event=>{if(event.detail?.playerRef)identities.set(event.detail.playerRef,event.detail);});const original=window.fetch;window.fetch=async(input,init)=>{
       const url=String(input);
       if(url==="/api/gameplay/current-game") return new Response(JSON.stringify(${JSON.stringify(game)}));
       if(url==="/api/gameplay/summoner-spells") return new Response(JSON.stringify({spells:[{id:4,name:"闪现"},{id:11,name:"惩戒"}]}));
       const response=await original(input,init);
       if(url.startsWith("/api/gameplay/overview")) {
         const data=await response.json(); const requestIdentity=init?.method==="POST"?JSON.parse(init.body):null;const identity=requestIdentity?{...identities.get(requestIdentity.playerRef),...requestIdentity}:null;
         data.player={...data.player,...identity,playerRef:identity?.playerRef||identity?.puuid||"fixture-current",...(identity?{displayName:identity.gameName,isCurrent:false}:{})};
         if(identity) data.region=identity.region||"cn";
         return new Response(JSON.stringify(data));
       }
       return response;
     }; }`);return;
   }
   res.end(fs.readFileSync(file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve();}},50);setTimeout(()=>{clearInterval(timer);resolve();},5000);})`);

 await evaluate(`new Promise(resolve=>{const t=setInterval(()=>{if(document.querySelector('#overview-content .summoner-strip')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);resolve();},8000);})`);
 await evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:{gameName:'Fixture',tagLine:'KR1',region:'kr',source:'search'}}))`);
 await evaluate(`new Promise((resolve,reject)=>{const t=setInterval(()=>{if(document.querySelector('.current-game-card')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);reject(Error('live card missing: '+document.querySelector('#overview-content').textContent.slice(0,700)));},8000);})`);
 const output=process.env.CURRENT_GAME_SHOTS || '/private/tmp/current-game-layout';fs.mkdirSync(output,{recursive:true});
 if(process.env.SUITE_TABS_LAYOUT==='1') { await require('./suite-tabs-layout.cjs').verify({call,evaluate,output}); return; }
 if(process.env.CHAMPSELECT_LAYOUT==='1') { await require('./champselect-layout.cjs').verify({call,evaluate,output}); return; }
 if(process.env.DIAGNOSTICS_1555_LAYOUT==='1') { await require('./diagnostics-1555-layout.cjs').verify({call,evaluate,output}); return; }
 if(process.env.R74_LAYOUT_SUITE==='groups') { await require('./player-group-layout.cjs').verify({call,evaluate,output}); return; }
 // 这一组护栏全是"设计单位"下的绝对像素契约（例如段位列固定 104px）。
 // .app-frame 现在有全局 CSS zoom，getBoundingClientRect 返回的是缩放后的视觉像素，
 // 2164x1100 会自动选到 1.1 档，104 就变成 114.39。把缩放钉死在 100% 再量，
 // 缩放本身的行为由 desktop/ui-scale-layout.cjs 单独守。
 await evaluate(`(()=>{localStorage.setItem('lol-loot-ui-scale','1');const s=document.getElementById('setting-ui-scale');s.value='1';s.dispatchEvent(new Event('change',{bubbles:true}));})()`);
 for(const theme of ['dark','light','violet']) for(const width of [820,1100,1440,1920,2164]) {
  await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:theme}]});
  await evaluate(`(()=>{const select=document.getElementById('setting-theme');select.value=${JSON.stringify(theme)};select.dispatchEvent(new Event('change',{bubbles:true}));})()`);
  await call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});
  await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
  const m=await evaluate(`(()=>{const card=document.querySelector('.current-game-card');const box=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,w:r.width,h:r.height}};return {theme:document.documentElement.dataset.theme,background:getComputedStyle(card).backgroundColor,card:box(card),teams:[...card.querySelectorAll('.current-game-team')].map(box),recentOverflow:[...card.querySelectorAll('.current-game-recent')].some(e=>e.scrollWidth>e.clientWidth+2),players:card.querySelectorAll('.current-game-player').length,overflow:[...card.querySelectorAll('.current-game-player-line')].some(e=>e.scrollWidth>e.clientWidth+2),banner:box(document.querySelector('.summoner-strip')),groups:document.querySelector('#player-groups').textContent,documentOverflow:document.documentElement.scrollWidth>innerWidth,uiZoom:getComputedStyle(document.documentElement).getPropertyValue('--ui-zoom').trim()};})()`);
  assert.equal(m.uiZoom,'1','these pixel contracts are measured at 100% zoom');
  assert.equal(m.theme,theme);assert.equal(m.players,10);assert.equal(m.recentOverflow,false,JSON.stringify(m));assert.equal(m.overflow,false,JSON.stringify(m));assert.equal(m.documentOverflow,false);assert.ok(m.banner.h<190);assert.ok(m.card.h<1800);
  if(width===2164)assert.ok(Math.abs(m.teams[0].y-m.teams[1].y)<2,'wide teams should be side by side');
  await require('./r74-current-game-checks.cjs').verify({evaluate,width,theme});
  const action=await evaluate(`(()=>{const e=document.querySelector('[data-current-game-spectate]');const h=e.closest('header');const b=e.getBoundingClientRect(),r=h.getBoundingClientRect();return {text:e.textContent,region:e.dataset.currentGameSpectate,fits:b.left>=r.left&&b.right<=r.right,headerFits:h.scrollWidth<=h.clientWidth+1};})()`);
  assert.equal(action.text,'前往OPGG观战');
  assert.equal(action.region,'kr');
  assert.ok(action.fits && action.headerFits,'spectate actions must fit the header');
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,theme+'-'+width+'.png'),Buffer.from(shot.data,'base64'));
  console.log(theme,width,m);
 }
 console.log('Production current game layout PASS');

}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
