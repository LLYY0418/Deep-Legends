// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/ui-scale-layout.cjs
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
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');let content=fs.readFileSync(file);if(pathname==='/gameplay.css'&&process.env.R79_ROSTER_MUTANT){
    content=content.toString();const mutant=process.env.R79_ROSTER_MUTANT;
    if(mutant==='420px'||mutant==='300px'||mutant==='24cqi')content=content.replace('--match-roster-width: 370px', '--match-roster-width: '+(mutant==='24cqi'?'24cqi':mutant));
    else if(mutant==='players-1fr'||mutant==='content-columns')content=content.replace(/(\.match-players \{[^}]*grid-template-columns:) repeat\(2,minmax\(0,1fr\)\)/,'$1 repeat(2,minmax(0,max-content)); justify-content: space-between');
    else if(mutant==='content-width')content=content.replace('108px minmax(0,1fr) var(--match-roster-width,clamp(150px,18cqi,190px)) 40px','108px minmax(0,1fr) minmax(0,max-content) 40px').replace('.match-players { display: grid; width: 100%;','.match-players { display: grid; width: auto; max-width: var(--match-roster-width,clamp(150px,18cqi,190px));');
    else throw Error('unknown mutant');
   }res.end(content);
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:`localStorage.setItem('lol-loot-ui-scale','1')`});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 await evaluate(`new Promise(resolve=>{const timer=setInterval(()=>{const app=document.querySelector('#app-frame');if(app&&!app.hasAttribute('inert')){clearInterval(timer);resolve();}},50);setTimeout(()=>{clearInterval(timer);resolve();},5000);})`);

 await evaluate(`new Promise(resolve=>{const t=setInterval(()=>{if(document.querySelector('#overview-content .summoner-strip')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);resolve();},8000);})`);
 const output=path.join(root,'docs','r88','roster');fs.mkdirSync(output,{recursive:true});
 await call('Emulation.setDeviceMetricsOverride',{width:1920,height:1080,deviceScaleFactor:1,mobile:false});
 await evaluate(`new Promise((resolve,reject)=>{const t=setInterval(()=>{if(document.querySelector('.match-summary:not(.is-arena) .match-player-name')){clearInterval(t);resolve();}},30);setTimeout(()=>{clearInterval(t);reject(Error('match roster missing'));},8000);})`);
 const fixture=JSON.parse(fs.readFileSync(path.join(root,'testdata/r88/roster-names.json')));
 await evaluate(`(async()=>{window.r88Overview=await (await fetch('/api/gameplay/overview')).json();})()`);
 const results=[], failures=[];
 for(const viewport of [1920,2560,3840]) {
  await call('Emulation.setDeviceMetricsOverride',{width:viewport,height:1080,deviceScaleFactor:1,mobile:false});
  for(const scenario of ['lengths','mixed']) {
   await evaluate(`(()=>{
    const fixture=${JSON.stringify(fixture)}, scenario=${JSON.stringify(scenario)};
    const list=document.querySelector('.matches-column .match-list');
    const normal=r88Overview.matches.find(m=>m.modeGroup!=='arena'), arena=r88Overview.matches.find(m=>m.modeGroup==='arena');
    const lengths=[4,6,8,12,16,20,6,16];
    const matches=lengths.map((length,i)=>{
      const isArena=scenario==='mixed'&&i%2===1, match=structuredClone(isArena?arena:normal);
      match.gameId=880000+i;
      for(const [j,p] of match.participants.entries()){
        const label=isArena?fixture.arena[((i-1)*6+j)%fixture.arena.length]:scenario==='mixed'?fixture.normal[(i+j)%fixture.normal.length]:'春江花月夜山河星辰流光云海清风明天地日月'.slice(0,length)+'#12345';
        [p.gameName,p.tagLine]=label.split('#');p.displayName=p.gameName;p.playerRef='r88-'+i+'-'+j;
      }
      return match;
    });
    window.r88View?.destroy();window.r88View=deepLegendsMatchCards.mount(list,{matches,disableExpand:true,disableReplay:true});
    [...list.querySelectorAll('.match-entry')].forEach((entry,i)=>{entry.dataset.nameLength=lengths[i]});
   })()`);
   await evaluate(`document.fonts.ready`);
   await evaluate(`new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
   const measurements=await evaluate(`(()=>{return [...document.querySelectorAll('[data-name-length]')].map(entry=>{
    const card=entry.querySelector('.match-summary'),players=card.querySelector('.match-players'),main=card.querySelector('.match-main');
    const arena=card.classList.contains('is-arena'), lists=[...players.querySelectorAll('.match-team-list')], names=[...players.querySelectorAll('.match-player-name')];
    const box=players.getBoundingClientRect(), outer=card.getBoundingClientRect();
    return {length:Number(entry.dataset.nameLength),arena,playersWidth:box.width,left:box.left,right:box.right,
      height:outer.height,columnGap:arena?parseFloat(getComputedStyle(players.querySelector('.arena-team-roster')).columnGap):lists[1].getBoundingClientRect().left-lists[0].getBoundingClientRect().right,
      names:names.map(e=>({name:e.textContent,tooltip:e.parentElement.dataset.tooltip,fontSize:getComputedStyle(e.parentElement).fontSize,truncated:e.scrollWidth-e.clientWidth>1,clippedByParent:e.getBoundingClientRect().right>e.parentElement.getBoundingClientRect().right+1})),
      overflow:box.right>outer.right||box.left<main.getBoundingClientRect().right};
   })})()`);
   results.push({viewport,scenario,measurements});assert.equal(measurements.length,8);
   const verify=(name,fn)=>{try{fn();}catch(error){failures.push({viewport,scenario,name,message:error.message});}};
   for(const m of measurements){
    verify('equal-width',()=>assert.equal(m.playersWidth,measurements[0].playersWidth));
    verify('equal-edges',()=>{assert.equal(m.left,measurements[0].left);assert.equal(m.right,measurements[0].right)});
    verify('370-cap',()=>assert.equal(m.playersWidth,370));
    if(!m.arena)verify('content-columns',()=>assert.equal(m.columnGap,14));
    verify('height',()=>assert.equal(m.height,m.arena?118:116));
    for(const n of m.names){
      if(m.arena||scenario==='lengths'&&m.length<=8||scenario==='mixed'&&!n.name.startsWith('FLAGGED'))verify('names:'+n.name,()=>assert.equal(n.truncated||n.clippedByParent,false));
      if(m.arena)verify('arena-tooltip',()=>{assert.ok(!n.name.includes('#'));assert.ok(n.tooltip.startsWith(n.name+'#'))});
    }
    verify('overflow',()=>assert.equal(m.overflow,false));
   }
   if(viewport===1920){
    verify('reference-edges',()=>{assert.equal(measurements[0].left,1424);assert.equal(measurements[0].right,1794)});
    const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,(process.env.R79_ROSTER_MUTANT||'baseline')+'-'+scenario+'.png'),Buffer.from(shot.data,'base64'));
   }
  }
 }
 const seen=new Set(results.filter(r=>r.scenario==='mixed').flatMap(r=>r.measurements.filter(m=>m.arena).flatMap(m=>m.names.map(n=>n.name))));
 assert.equal(seen.size,23,'all user-supplied visible names must be measured');
 const tag=(process.env.R79_ROSTER_MUTANT||'baseline')+(process.env.R79_CLIPPING_POLICY==='existing'?'-existing-clipping':'');
 fs.writeFileSync(path.join(output,tag+'.json'),JSON.stringify({clippingPolicy:process.env.R79_CLIPPING_POLICY||'strict',results,failures},null,2));
 const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,tag+'.png'),Buffer.from(shot.data,'base64'));
 assert.deepEqual(failures,[],'roster acceptance');
 console.log('R88 production roster Chromium PASS: 23 arena names, 8 equal-width cards, 3 viewport sizes');
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();server?.close();fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
