// Real Chromium layout guard. No jsdom measurements or external npm dependency.
// CHROME_BIN=/path/to/chrome node desktop/r117-browser.cjs
'use strict';
const {spawn} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=path.resolve(__dirname,'..'), web=path.join(root,'backend','web');
const chrome=process.env.CHROME_BIN || (process.platform==='darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'chromium');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r117-browser-'));
let proc,ws,server;
async function main(){
 proc=spawn(chrome,['--headless=new','--disable-background-timer-throttling','--disable-renderer-backgrounding','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${temp}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const url=await new Promise((resolve,reject)=>{let output=''; const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);proc.once('error',reject);proc.stderr.on('data',chunk=>{output+=chunk;const m=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(m){clearTimeout(timer);resolve(m[1]);}});proc.once('exit',code=>reject(Error(`Chrome exited ${code}: ${output.slice(-1000)}`)));});
 ws=new WebSocket(url); await new Promise((res,rej)=>{ws.addEventListener('open',res,{once:true});ws.addEventListener('error',rej,{once:true});});
 let seq=0;const pending=new Map();ws.addEventListener('message',e=>{const msg=JSON.parse(e.data);if(pending.has(msg.id)){const [resolve,reject]=pending.get(msg.id);pending.delete(msg.id);msg.error?reject(Error(JSON.stringify(msg.error))):resolve(msg.result);}});
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq;const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},45000);pending.set(id,[value=>{clearTimeout(timer);resolve(value)},error=>{clearTimeout(timer);reject(error)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 console.log('CDP connected');
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.method==='Runtime.exceptionThrown'||m.method==='Runtime.consoleAPICalled'&&m.params.type==='error')console.log('browser error',JSON.stringify(m.params).slice(0,1800));});
 const {targetId}=await send('Target.createTarget',{url:'about:blank'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 const call=(m,p)=>send(m,p,sessionId);
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
 await call('Page.enable');
 await call('Runtime.enable');
 await call('Page.bringToFront');
 const output=process.env.R117_BROWSER_OUTPUT||path.join(root,'docs/r117-validation/browser');fs.mkdirSync(output,{recursive:true});
 server=require('node:http').createServer((req,res)=>{
   const pathname=new URL(req.url,'http://localhost').pathname;
   if(pathname==='/api/image'||pathname==='/api/champion-asset'){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#364558"/></svg>');return;}
   if(pathname==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'});res.write(': fixture stream\n\n');return;}
   if(pathname.startsWith('/api/')){res.statusCode=204;res.end();return;}
   const file=path.resolve(web,pathname==='/'?'index.html':'.'+pathname);
   if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.statusCode=404;res.end();return;}
   const content=fs.readFileSync(file);
   const send=()=>{res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png'})[path.extname(file)]||'application/octet-stream');res.end(content);};
   // 把 champions.css 拖慢：只有样式响应明显晚于 <link> 创建，才能区分「真的 await 了
   // onload」和「只是同步创建了 <link> 就走」。DOM 存在性检查测不出这种时序回归。
   if(pathname==='/champions.css')setTimeout(send,350);else send();
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 await call('Emulation.setDeviceMetricsOverride',{width:1500,height:1050,deviceScaleFactor:1,mobile:false});
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 await call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?demo'});
 const until=async expression=>{const started=Date.now();while(Date.now()-started<20000){if(await evaluate('Boolean('+expression+')'))return;await new Promise(r=>setTimeout(r,50));}throw Error('condition timeout: '+expression);};
 await until('document.querySelector(".demo-flag")');
 await new Promise(r=>setTimeout(r,600));

 // ---- 阶段 A：首屏（只有 5 张首屏样式表，任何懒页 CSS 都还没注入）----
 const initialLinks = await evaluate(`JSON.stringify([...document.querySelectorAll('link[rel="stylesheet"]')].map(l=>l.getAttribute("href")))`);
 assert.deepEqual(JSON.parse(initialLinks), ['/app.css','/build-item-row.css','/gameplay.css','/friends.css','/metrics.css'], 'first-screen stylesheet set drifted');
 assert.equal(await evaluate(`Boolean(document.querySelector('link[data-section-style]'))`), false, 'a lazy section sheet was already injected at first paint');
 // 反证：champions.css 此刻确实未生效，否则下面几条共享片断言证明不了任何事。
 assert.notEqual(await evaluate(`getComputedStyle(document.querySelector('#champions-panel .champions-skeleton')).display`), 'grid', 'champions.css must not be active before the section switch');
 // 共享片必须在首屏生效：对局页装备排行用的 .mayhem-ranking-list 基础几何原本只存在于 champions.css。
 // 先切到对局页（该 section 没有懒加载模块，不会注入任何懒 CSS），在可见布局里量真实几何。
 await evaluate(`document.getElementById('section-live').click()`);
 await new Promise(r=>setTimeout(r,300));
 const geometry = JSON.parse(await evaluate(`(()=>{
   const probe=document.createElement('div');probe.className='mayhem-ranking-list';probe.id='r117-shared-probe';
   const row='<article><b>1</b><span class="recommend-icon"></span><strong>probe</strong><dl><div><dt>K</dt><dd>1</dd></div></dl></article>';
   probe.innerHTML=row+row;
   document.getElementById('live-panel').append(probe);
   const list=getComputedStyle(probe),item=getComputedStyle(probe.querySelector('article'));
   const cells=[...probe.querySelectorAll('article')].map(a=>({top:a.offsetTop,left:a.offsetLeft}));
   return JSON.stringify({display:list.display,tracks:list.gridTemplateColumns,columns:cells.length,
     sideBySide:cells[0].top===cells[1].top&&cells[1].left>cells[0].left,minHeight:item.minHeight});
 })()`));
 assert.equal(geometry.display, 'grid', 'shared .mayhem-ranking-list must already be a grid before any lazy CSS loads');
 assert.equal(geometry.sideBySide, true, `shared .mayhem-ranking-list must keep its two-column ranking geometry, got ${geometry.tracks}`);
 assert.equal(geometry.minHeight, '70px', 'shared ranking row height must come from the first-screen sheet');
 // index.html 有完整 suite 静态外壳，容器查询锚点必须首屏就位。
 assert.equal(await evaluate(`getComputedStyle(document.getElementById('suite-panel')).containerType`), 'inline-size', 'suite container query anchor is not in the first-screen sheet');
 await evaluate(`document.getElementById('r117-shared-probe')?.remove()`);

 // ---- 逐帧采样器：抓「切换瞬间是否存在无样式帧」----
 // rAF 单独用会漏掉极短的 loading 窗口（suite 只补一个模块时可能一帧内走完），
 // 所以再挂一个 MutationObserver：notice 一插入就同步补一帧，采样不再依赖帧率。
 const startSampler = (target) => evaluate(`(()=>{
   window.__r117={frames:[],on:true};
   const record=(source)=>{
     const loading=document.querySelector('[data-section-loading]');
     const section=document.querySelector('.section-tab.is-active')?.dataset.section||'';
     const skeleton=document.querySelector('#champions-panel .champions-skeleton');
     const table=document.querySelector('#champions-panel .champion-table');
     const panel=document.getElementById('suite-panel');
     window.__r117.frames.push({source,section,loading:loading?loading.dataset.sectionLoading:'',
       skeletonDisplay:skeleton?getComputedStyle(skeleton).display:'',
       tableLayout:table?getComputedStyle(table).tableLayout:'',
       containerType:panel?getComputedStyle(panel).containerType:''});
   };
   window.__r117.record=record;
   const sample=()=>{record('frame');if(window.__r117.on)requestAnimationFrame(sample);};
   requestAnimationFrame(sample);
   window.__r117.observer=new MutationObserver(()=>record('mutation'));
   window.__r117.observer.observe(document.body,{childList:true,subtree:true});
   record('install');
   // 采样器安装与点击必须在同一个任务里完成，否则切换前的帧会被算进本次切换。
   document.getElementById('${target}').click();
   return true;
 })()`.replace('${target}', target));
 const stopSampler = async () => {
   await evaluate('window.__r117.on=false;window.__r117.observer?.disconnect()');
   return JSON.parse(await evaluate('JSON.stringify(window.__r117.frames)'));
 };

 // ---- 阶段 B：切到英雄页，懒 CSS 注入时机 + 无样式帧 ----
 await startSampler('section-champions');
 await until(`!document.querySelector('[data-section-loading]') && document.querySelector('script[data-section-module="champions"]')`);
 await new Promise(r=>setTimeout(r,500));
 const championsFrames = (await stopSampler()).filter(f=>f.section==='champions');
 // 样式必须由 section-loader 在模块执行前注入；只等模块会让「不 await 样式」的退化变成超时而不是明确断言。
 assert.equal(await evaluate(`document.querySelector('link[data-section-style="champions"]')?.getAttribute('href')||''`), '/champions.css', 'section loader never injected the champions stylesheet before running its module');
 // 时序护栏：champions.js 的请求必须不早于 champions.css 加载结束。
 // 去掉 section-loader 的 `await Promise.all(...loadStyle)` 后 <link> 仍然会被创建，
 // 所以「元素存在」这类断言测不出来——独立验收连跑 8 次全 PASS 就是这个原因。
 const timing = JSON.parse(await evaluate(`(()=>{
   const entry=(name)=>performance.getEntriesByName(location.origin+name)[0]||{};
   return JSON.stringify({cssStart:entry('/champions.css').startTime||0,cssEnd:entry('/champions.css').responseEnd||0,jsStart:entry('/champions.js').startTime||0});
 })()`));
 assert.ok(timing.cssEnd > 0 && timing.jsStart > 0, `resource timing missing for the lazy champions assets: ${JSON.stringify(timing)}`);
 assert.ok(timing.cssEnd - timing.cssStart >= 200, `champions.css was not actually delayed, the ordering assertion would be vacuous: ${JSON.stringify(timing)}`);
 assert.ok(timing.jsStart >= timing.cssEnd, `champions.js started before champions.css finished loading (cssEnd=${timing.cssEnd.toFixed(1)} jsStart=${timing.jsStart.toFixed(1)}) — the section loader no longer awaits page styles`);
 assert.ok(championsFrames.length > 1, 'sampler recorded no frames during the champions switch');
 assert.ok(championsFrames.some(f=>f.skeletonDisplay==='grid'), 'never observed champions.css taking effect');
 // 无样式帧：加载态之外，只要骨架屏/表格存在，它的关键计算属性就必须已经生效。
 // 加载态期间允许未上样式——section-loader 是「先插 notice，再 await 样式与模块」，
 // 那段时间面板显示的本来就是「正在加载页面…」，静态骨架屏还没有内容可闪。
 const unstyledSkeleton = championsFrames.filter(f=>!f.loading && f.skeletonDisplay && f.skeletonDisplay!=='grid').length;
 const unstyledTable = championsFrames.filter(f=>!f.loading && f.tableLayout && f.tableLayout!=='fixed').length;
 assert.equal(unstyledSkeleton + unstyledTable, 0, `unstyled frames outside the loading state: skeleton=${unstyledSkeleton} table=${unstyledTable}`);
 assert.ok(championsFrames.some(f=>f.loading==='champions'), 'never observed the champions section in its loading state');
 // 切换完成后 champions.css 必须真的生效：优先量 .champion-table，页面没有表格数据时
 // 用一个临时 .champions-skeleton 探针复量（阶段 A 已证明同一个类在切换前不是 grid）。
 const championsAfter = JSON.parse(await evaluate(`(()=>{
   const panel=document.getElementById('champions-panel');
   const t=panel.querySelector('.champion-table'),s=panel.querySelector('.champions-skeleton');
   const probe=document.createElement('div');probe.className='champions-skeleton';panel.append(probe);
   const probeDisplay=getComputedStyle(probe).display;probe.remove();
   return JSON.stringify({table:t?getComputedStyle(t).tableLayout:'',skeleton:s?getComputedStyle(s).display:'',probe:probeDisplay});
 })()`));
 assert.ok(championsAfter.table==='fixed' || championsAfter.probe==='grid', `champions.css key computed style never landed after the switch: ${JSON.stringify(championsAfter)}`);

 // ---- 阶段 C：切到工具页，[data-section-loading] 期间容器锚点必须已生效 ----
 await startSampler('section-suite');
 await until(`!document.querySelector('[data-section-loading]') && document.querySelector('script[data-section-module="suite"]')`);
 await new Promise(r=>setTimeout(r,400));
 const suiteFrames = (await stopSampler()).filter(f=>f.section==='suite');
 assert.equal(await evaluate(`document.querySelector('link[data-section-style="suite"]')?.getAttribute('href')||''`), '/suite.css', 'section loader never injected the suite stylesheet before running its module');
 assert.ok(suiteFrames.some(f=>f.loading==='suite'), 'never observed the suite section in its loading state');
 const lostAnchor = suiteFrames.filter(f=>f.containerType!=='inline-size').length;
 assert.equal(lostAnchor, 0, `suite-panel lost its container anchor in ${lostAnchor} frames (shared piece must be first-screen, not suite.css)`);
 assert.equal(await evaluate(`getComputedStyle(document.getElementById('suite-panel')).containerType`), 'inline-size', 'suite container anchor missing after load');

 fs.writeFileSync(path.join(output,'result.json'),JSON.stringify({passed:true,firstScreenStylesheets:JSON.parse(initialLinks),sharedGeometry:geometry,championsFrames:championsFrames.length,suiteFrames:suiteFrames.length,suiteLoadingFramesObserved:suiteFrames.filter(f=>f.loading==='suite').length,unstyledFrames:unstyledSkeleton+unstyledTable},null,2));
 console.log('R117 Chromium PASS',JSON.stringify({frames:{champions:championsFrames.length,suite:suiteFrames.length},unstyledFrames:unstyledSkeleton+unstyledTable,styleBeforeModule:{cssEnd:Math.round(timing.cssEnd),jsStart:Math.round(timing.jsStart)},sharedGeometry:geometry}));
}
// 收尾清理必须尽力而为：proc.kill() 之后 Chrome 子进程还在写 profile 目录，立刻
// rmSync 会偶发 ENOTEMPTY。它从 .finally 里抛出去就是未处理拒绝、退出码 1——护栏
// 实际通过也会把 CI 变红，人就会习惯性加 || true，正是护栏最该防的事。所以这里先
// 等进程真的退出（带超时）再删目录，删除失败只打一行清理警告；主流程失败的退出码
// 仍由上面的 catch 决定，绝不被清理路径掩盖。
async function cleanup(){
 try{ws?.close();}catch(error){}
 if(proc&&proc.exitCode===null&&proc.signalCode===null){
  const exited=new Promise(resolve=>{const timer=setTimeout(resolve,5000);const done=()=>{clearTimeout(timer);resolve();};proc.once('exit',done);proc.once('error',done);});
  try{proc.kill();}catch(error){}
  await exited;
 }
 try{server?.closeAllConnections();}catch(error){}
 await new Promise(resolve=>{const timer=setTimeout(resolve,2000);if(!server){clearTimeout(timer);resolve();return;}server.close(()=>{clearTimeout(timer);resolve();});});
 for(let attempt=1;attempt<=8;attempt++){
  try{fs.rmSync(temp,{recursive:true,force:true});return;}
  catch(error){if(attempt===8){console.error(`cleanup warning: ${temp} left behind (${error.message})`);return;}await new Promise(resolve=>setTimeout(resolve,150));}
 }
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{cleanup().catch(error=>console.error('cleanup warning:',error&&error.message));});
