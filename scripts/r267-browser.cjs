'use strict';
// Real public backend and its CSP/assets, with explicit synthetic API fixtures.
const {spawn}=require('node:child_process'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const {decodePNG,regionBrightness}=require('./png-pixels.cjs');
const root=path.resolve(__dirname,'..'),out=process.env.R267_BROWSER_OUT||path.join(os.tmpdir(),'deep-legends-r267-browser');
const closedSessions=new Set(), canceledRequests=new Set();let canceledInterceptions=0;
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r267-browser-')),errors=[],violations=[],results=[];let backend,chrome,ws;
const inject=`window.__r267Base=structuredClone(overview);const source=overview.matches;overview.matches=Array.from({length:1000},(_,i)=>{const m=structuredClone(source[i%source.length]);m.gameId=900000+i;const p=m.participants.find(p=>p.participantId===m.subjectParticipantId);if(p){p.championId=[103,238,157,875,145,92,64,81,254,555,25,69,28,111,61,268,518,245,113,53,27,17,516,3][i%24];p.championName=['阿狸','劫','亚索','瑟提','卡莎','锐雯','李青','伊泽瑞尔','蔚','派克','莫甘娜','卡西奥佩娅','伊芙琳','深海泰坦','发条魔灵','沙皇','妮蔻','艾克','瑟庄妮','布里茨','辛吉德','提莫','奥恩','加里奥'][i%24];}return m;});if(new URLSearchParams(location.search).has('matrix'))overview.matches=['solo','hextech-aram','arena'].map(mode=>{const m=structuredClone(source.find(row=>row.modeGroup===mode)),p=m.participants.find(p=>p.participantId===m.subjectParticipantId);Object.assign(p,{kills:99,deaths:99,assists:99,kda:2});return m;});overview.pagination={begIndex:0,count:1000,hasMore:false};`;
const intercept=`
 if(pathname==='/api/gameplay/overview' && new URLSearchParams(location.search).has('loading') && !window.__r267Delayed) {
  window.__r267Delayed=true;window.__r267HeaderAt=performance.now();const data=structuredClone(overview);data.matches=data.matches.slice(0,20);data.pagination={count:20,hasMore:false};
  if(new URLSearchParams(location.search).get('loading')==='zero')data.matches=[];
  const header={player:data.player,ranks:data.ranks,masteries:data.masteries,matches:[],historyRequested:0,pagination:{count:20,partial:true},capabilities:[]},encoder=new TextEncoder();
  return Promise.resolve(new Response(new ReadableStream({start(c){c.enqueue(encoder.encode(JSON.stringify({type:'cards',overview:header})+'\\n'));window.__r267Complete=()=>{c.enqueue(encoder.encode(JSON.stringify({type:'complete',overview:data})+'\\n'));c.close();}}}),{headers:{'Content-Type':'application/x-ndjson'}}));
 }
`;
async function start(){
 fs.mkdirSync(out,{recursive:true});
 for(const file of ['failure.json','result.json'])fs.rmSync(path.join(out,file),{force:true});
 backend=spawn(process.env.R267_BACKEND||'/tmp/r267-public',['--desktop'],{cwd:root,env:{...process.env,LOL_LOOT_DATA_DIR:path.join(temp,'data')},stdio:['ignore','pipe','pipe']});
 const ready=await new Promise((resolve,reject)=>{let data='';const timer=setTimeout(()=>reject(Error('Backend startup timeout')),20000);backend.once('error',reject);backend.stdout.on('data',c=>{data+=c;for(const line of data.split('\n')){try{const value=JSON.parse(line.replace(/^LOOT_READY /,''));if(value.baseUrl){clearTimeout(timer);resolve(value);return}}catch{}}});backend.once('exit',code=>reject(Error('Backend exited '+code)));});
 const csp=(await fetch(ready.bootstrapUrl)).headers.get('content-security-policy');assert(csp?.includes("script-src 'self'") && !csp.includes('unsafe-inline'));
 chrome=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--no-sandbox','--disable-background-networking','--disable-renderer-backgrounding','--disable-background-timer-throttling','--no-first-run','--no-default-browser-check','--remote-debugging-port=0',`--user-data-dir=${path.join(temp,'chrome')}`,'about:blank'],{stdio:['ignore','ignore','pipe']});
 const endpoint=await new Promise((resolve,reject)=>{let data='';const timer=setTimeout(()=>reject(Error('Chrome startup timeout')),20000);chrome.once('error',reject);chrome.stderr.on('data',c=>{data+=c;const match=data.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(match){clearTimeout(timer);resolve(match[1]);}});});
 ws=new WebSocket(endpoint);await new Promise((resolve,reject)=>{ws.addEventListener('open',resolve,{once:true});ws.addEventListener('error',reject,{once:true});});
 let seq=0;const pending=new Map();
 const send=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},30000);pending.set(id,[v=>{clearTimeout(timer);resolve(v)},e=>{clearTimeout(timer);reject(e)}]);ws.send(JSON.stringify({id,method,params,sessionId}));});
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(pending.has(m.id)){const [resolve,reject]=pending.get(m.id);pending.delete(m.id);m.error?reject(Error(JSON.stringify(m.error))):resolve(m.result);}
  if(m.method==='Network.loadingFailed' && m.params.canceled)canceledRequests.add(m.sessionId+':'+m.params.requestId);
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails);
  if(m.method==='Log.entryAdded' && m.params.entry.level==='error')errors.push(m.params.entry);
  if(m.method==='Runtime.bindingCalled' && m.params.name==='r267CSP')violations.push(JSON.parse(m.params.payload));
  if(m.method==='Fetch.requestPaused')void (async()=>{const url=m.params.request.url,pathname=new URL(url).pathname,id=m.params.requestId;
   if(pathname==='/api/image'){await send('Fetch.fulfillRequest',{requestId:id,responseCode:200,responseHeaders:[{name:'Content-Type',value:'image/svg+xml'}],body:Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#23384a"/><circle cx="320" cy="180" r="80" fill="#cc9e45"/></svg>').toString('base64')},m.sessionId);}
   else if(['/history-filters.js','/gameplay.css','/history-filters.css'].includes(pathname)){const body=process.env.R267_WEB_OVERRIDE && fs.existsSync(path.join(process.env.R267_WEB_OVERRIDE,pathname))?fs.readFileSync(path.join(process.env.R267_WEB_OVERRIDE,pathname),'utf8'):await (await fetch(ready.baseUrl+pathname)).text();await send('Fetch.fulfillRequest',{requestId:id,responseCode:200,responseHeaders:[{name:'Content-Type',value:pathname.endsWith('.css')?'text/css':'text/javascript'}],body:Buffer.from(body).toString('base64')},m.sessionId);}
   else if(pathname==='/demo-data.js'){const body=(await (await fetch(ready.baseUrl+pathname)).text()).replace('  const fixtures = new Map([',inject+'\n  const fixtures = new Map([').replace('    const params = new URLSearchParams(url.split("?")[1] || "");','    const params = new URLSearchParams(url.split("?")[1] || "");'+intercept+`if(pathname==='/api/status' && !window.deepLegendsMatchCards)return new Promise(resolve=>{const wait=()=>window.deepLegendsMatchCards?resolve(window.fetch(input,init)):setTimeout(wait,10);wait();});`);assert(body.includes('window.__r267Base=') && body.includes('window.__r267Complete='),'demo fixture anchors changed');await send('Fetch.fulfillRequest',{requestId:id,responseCode:200,responseHeaders:[{name:'Content-Type',value:'text/javascript'}],body:Buffer.from(body).toString('base64')},m.sessionId);}
   else if(url.startsWith(ready.baseUrl+'/'))await send('Fetch.continueRequest',{requestId:id},m.sessionId);
   else await send('Fetch.failRequest',{requestId:id,errorReason:'BlockedByClient'},m.sessionId);
  })().catch(async e=>{if(closedSessions.has(m.sessionId))return;if(e.message.includes('Invalid InterceptionId')){await new Promise(resolve=>setTimeout(resolve,50));if(canceledRequests.has(m.sessionId+':'+m.params.networkId)){canceledInterceptions++;return;}}errors.push({message:e.message});});
 });
 const browserVersion=await send('Browser.getVersion');
 for(const theme of ['dark','light']){
  const {targetId}=await send('Target.createTarget',{url:'about:blank'}),{sessionId}=await send('Target.attachToTarget',{targetId,flatten:true}),call=(m,p)=>send(m,p,sessionId);
  const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
  const until=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await new Promise(r=>setTimeout(r,50));}throw Error('timeout '+expression+' '+await evaluate("JSON.stringify({loading:document.querySelector('#overview-content')?._overviewViewTab?.loading,html:document.querySelector('#overview-content')?.textContent.slice(0,600)})"));};
  const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
  const frames=()=>evaluate('new Promise(resolve=>{let remaining=4;const frame=()=>--remaining?requestAnimationFrame(frame):resolve();requestAnimationFrame(frame)})');
  const shot=async name=>{const r=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false}),buffer=Buffer.from(r.data,'base64');fs.writeFileSync(path.join(out,`${theme}-${name}.png`),buffer);return buffer;};
  await send('Target.activateTarget',{targetId});
  for(const domain of ['Network','Page','Runtime','Log'])await call(domain+'.enable');
  await call('Runtime.addBinding',{name:'r267CSP'});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:`window.EventSource=class {addEventListener(){}close(){}};try{localStorage.clear();localStorage.setItem('lol-loot-theme','${theme}');}catch{};document.addEventListener('securitypolicyviolation',e=>r267CSP(JSON.stringify({directive:e.effectiveDirective,blockedURI:e.blockedURI})));`});
  await call('Fetch.enable',{patterns:[{urlPattern:'*'}]});
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});
  await call('Network.setCookie',{name:'lol_loot_token',value:ready.token,url:ready.baseUrl,httpOnly:true});
  await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview'});
  await until("document.querySelector('.match-list .match-entry') && !document.querySelector('#overview-content')._overviewViewTab.loading");
  await evaluate(`document.documentElement.dataset.theme='${theme}'`);
  await click('.match-filterbar [data-history-filter-load],.match-filterbar [data-af-open]');
  await until("document.querySelector('[data-af-menu]')?.open");await click('[data-af-category="hero"]');await frames();
  if(!process.env.R267_CASE || process.env.R267_CASE==='footer'){
   await evaluate(`(()=>{const c=document.getElementById('overview-content'),t=c._afBoundTab,ctx=c._afContext(),peer=ctx.rows()[0].participants.find(p=>p.playerRef && p!==ctx.subject(ctx.rows()[0])) || {playerRef:'riot:fixture#KR1'};window.__r267Conditions={hero:{values:['103','238'],not:false},result:{values:['loss'],not:true},performance:{values:['mvp'],thresholds:{kda:3},not:false},position:{values:['middle'],not:false},time:{values:['d7'],not:false},coplayer:{values:[peer.playerRef],side:'either',not:false},multikill:{values:['5'],atLeast:true,not:false},duration:{values:['long'],not:false}};})()`);
   const inspect=()=>evaluate(`(()=>{const caps=document.querySelector('[data-af-draft-capsules]'),r=n=>{const x=n.getBoundingClientRect();return {left:x.left,right:x.right,top:x.top,bottom:x.bottom,width:x.width,height:x.height}},nodes=[...caps.querySelectorAll(':scope > .af-capsule')],visible=nodes.filter(n=>getComputedStyle(n).display!=='none' && n.getBoundingClientRect().width>0),more=caps.querySelector('[data-af-capsules-more]');const clone=caps.cloneNode(true);clone.classList.remove('is-expanded');clone.removeAttribute('data-af-draft-capsules');clone.querySelectorAll('*').forEach(n=>{for(const a of [...n.attributes])if(a.name.startsWith('data-af-'))n.removeAttribute(a.name)});Object.assign(clone.style,{position:'fixed',left:'0',top:'0',width:'max-content',visibility:'hidden'});clone.querySelectorAll('.af-capsule').forEach(n=>{n.hidden=false;n.style.maxWidth='none';n.style.display='inline-flex'});const cm=clone.querySelector('.af-capsules-more');if(cm){cm.hidden=false;cm.textContent='+1'};const probe=document.createElement('div');probe.className='af-menu af-modal-footer';Object.assign(probe.style,{position:'fixed',visibility:'hidden',font:getComputedStyle(caps).font});probe.append(clone);document.body.append(probe);const intrinsic=[...clone.querySelectorAll(':scope > .af-capsule')].map(n=>r(n).width),moreWidth=cm?r(cm).width:40,gap=parseFloat(getComputedStyle(caps).columnGap)*(r(caps).width/parseFloat(getComputedStyle(caps).width));probe.remove();return {layout:document.querySelector('[data-af-menu]')._afFooterFrame,signature:caps._afLayout,slotWidth:getComputedStyle(caps.parentElement).width,intrinsic,moreWidth,gap,total:nodes.length,visible:visible.map(n=>({rect:r(n),remove:r(n.querySelector('[data-af-remove]')),text:n.textContent,category:n.querySelector('[data-af-remove]').dataset.afRemove})),rect:r(caps),scrollWidth:caps.scrollWidth,clientWidth:caps.clientWidth,overflow:getComputedStyle(caps).overflowX,more:more && !more.hidden?{text:more.textContent,rect:r(more)}:null,expanded:caps.classList.contains('is-expanded'),shadow:getComputedStyle(document.querySelector('[data-af-menu]')).boxShadow}})()`);
   const inside=(a,b)=>a.left>=b.left-.5 && a.right<=b.right+.5 && a.top>=b.top-.5 && a.bottom<=b.bottom+.5;
   const check=(value,width,count)=>{
    const evidence=JSON.stringify({theme,width,count,...value});
    assert(!['auto','scroll'].includes(value.overflow),'footer must not create a horizontal scroll container '+evidence);
    assert(value.scrollWidth<=value.clientWidth,'footer scroll overflow '+evidence);
    assert.equal(value.total,count,evidence);assert.equal(value.shadow,'none','modal glow returned');
    for(const cap of value.visible){assert(inside(cap.rect,value.rect),'partially clipped capsule '+evidence);assert(inside(cap.remove,cap.rect),'clipped remove control '+evidence);}
    let expected=count,used=0;if(value.intrinsic.reduce((a,b)=>a+b,0)+value.gap*Math.max(0,count-1)>value.rect.width+.5){expected=0;for(const n of value.intrinsic){const next=used+n+(expected?value.gap:0);if(next+value.gap+value.moreWidth>value.rect.width+.5)break;used=next;expected++;}expected=Math.max(1,expected)}assert.equal(value.visible.length,expected,'must show the largest complete prefix '+evidence);
    const hidden=count-value.visible.length;assert.equal(value.more?.text || null,hidden?'+'+hidden:null,'incorrect hidden count '+evidence);
    if(value.more)assert(inside(value.more.rect,value.rect),'clipped +N '+evidence);
    if(count===1)assert.equal(value.more,null,evidence);
    if(width===1440 && count===3)assert.equal(value.visible.length,3,'1440/3 must show all capsules '+evidence);
    assert.deepEqual(value.visible.map(v=>v.category),Object.keys({hero:1,result:1,performance:1,position:1,time:1,coplayer:1,multikill:1,duration:1}).slice(0,value.visible.length),'visible conditions must be an ordered prefix');
   };
   for(const width of [900,1280,1440,1920]){
    await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});
    for(const count of [1,3,5,8]){
     await evaluate(`(()=>{const c=document.getElementById('overview-content'),t=c._afBoundTab;t.advancedMenu.conditions=Object.fromEntries(Object.entries(__r267Conditions).slice(0,${count}));deepLegendsHistoryFilters.refresh(c,t,c._afContext());})()`);await frames();
     const value=await inspect();check(value,width,count);results.push({kind:'footer',theme,width,count,...value});
     if([900,1440].includes(width) && [3,8].includes(count))await shot(`footer-${width}-${count}`);
     if(value.more){
      await click('[data-af-capsules-more]');await frames();const expanded=await inspect();assert.equal(expanded.visible.length,count,'popup must show all conditions '+JSON.stringify(expanded));assert.equal(expanded.scrollWidth,expanded.clientWidth,'popup overflow '+JSON.stringify(expanded));for(const cap of expanded.visible)assert(inside(cap.rect,expanded.rect),'popup clips a capsule');
      if(width===900 && count===8)await shot('footer-900-8-expanded');
      const last=expanded.visible.at(-1).category;await click(`[data-af-draft-capsules] [data-af-remove="${last}"]`);await frames();assert.equal((await inspect()).total,count-1,'hidden condition must be removable');if(count===8){for(let remaining=count-1;remaining>0;remaining--){const cap=(await inspect()).visible.at(-1);await click(`[data-af-draft-capsules] [data-af-remove="${cap.category}"]`);await frames();assert.equal((await inspect()).total,remaining-1,'all popup conditions must be removable')}}else if((await inspect()).expanded){await click('[data-af-capsules-more]');await frames();}
     }
    }
   }
   // Exercise the actual option event path before the resize check.
   await click('[data-af-clear]');await click('[data-af-option="103"]');await frames();check(await inspect(),1920,1);await click('[data-af-option="238"]');await frames();check(await inspect(),1920,1);
   // Resize an open modal without refreshing its conditions.
   await evaluate("(()=>{const c=document.getElementById('overview-content'),t=c._afBoundTab;t.advancedMenu.conditions=Object.fromEntries(Object.entries(__r267Conditions).slice(0,3));deepLegendsHistoryFilters.refresh(c,t,c._afContext());})()");await frames();
   for(const width of [900,1440]){await call('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});await frames();const value=await inspect();check(value,width,3);results.push({kind:'open-modal-resize',theme,width,...value});}
   await evaluate("window.__r267LayoutWrites=0;new MutationObserver(r=>__r267LayoutWrites+=r.length).observe(document.querySelector('[data-af-draft-capsules]'),{attributes:true,subtree:true,childList:true});");await frames();const writes=await evaluate('__r267LayoutWrites');await frames();assert.equal(await evaluate('__r267LayoutWrites'),writes,'footer must settle without repeated observer writes');
  }
  if(!process.env.R267_CASE || process.env.R267_CASE==='winrate'){
   // Make exact 57% and 25% fixtures through the actual options computation.
   await evaluate("(()=>{const c=document.getElementById('overview-content'),t=c._afBoundTab,ctx=c._afContext(),source=t.data.matches;t.data.matches=Array.from({length:200},(_,i)=>{const m=structuredClone(source[i%source.length]),p=ctx.subject(m);m.gameId=2000000+i;p.championId=i<100?103:238;p.championName=i<100?'阿狸':'劫';m.win=i<100?i<57:i-100<25;p.win=m.win;m.result=m.win?'win':'loss';return m;});t.advancedMenu.conditions={};t.advancedMenu.drafts={};deepLegendsHistoryFilters.refresh(c,t,c._afContext(),true);})()");
   await click('[data-af-category="hero"]');await frames();
   const colors=()=>evaluate(`(()=>{const probe=document.createElement('span');probe.style.color='var(--success)';document.querySelector('[data-af-menu]').append(probe);const success=getComputedStyle(probe).color;probe.remove();return {success,cards:['103','238'].map(id=>{const tile=document.querySelector('[data-af-option="'+id+'"]'),small=tile.querySelector('small'),win=small.querySelector('[data-af-winrate]') || small;return {id,text:small.textContent,color:getComputedStyle(win).color,countColor:getComputedStyle(small).color,selected:tile.classList.contains('is-active')};})}})()`);
   let value=await colors();assert(value.cards[0].text.includes('57%') && value.cards[1].text.includes('25%'),JSON.stringify(value));assert.equal(value.cards[0].color,value.success,'57% must use the success token '+JSON.stringify(value));assert.notEqual(value.cards[1].color,value.success,'25% must keep muted color');assert.notEqual(value.cards[0].countColor,value.success,'count must stay gray');
   await click('[data-af-option="103"]');await frames();value=await colors();assert(value.cards[0].selected);assert.equal(value.cards[0].color,value.success,'selected win rate must stay green');results.push({kind:'winrate',theme,...value});await shot('winrate-selected');
  }
  if(!process.env.R267_CASE || process.env.R267_CASE==='skeleton'){
   await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1200,deviceScaleFactor:1,mobile:false});
   await call('Page.navigate',{url:ready.baseUrl+'/?demo&section=overview&loading=matches'});
   await until("document.querySelectorAll('.match-skeleton').length===5 && typeof __r267Complete==='function'");await evaluate(`document.documentElement.dataset.theme='${theme}'`);
   for(const phase of [0,.5,1,'reduced']){
    await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:phase==='reduced'?'reduce':'no-preference'}]});
    if(phase!=='reduced'){
     await evaluate("document.querySelectorAll('.gameplay-skeleton span').forEach(n=>n.style.animationName='none')");await frames();
     await evaluate(`document.querySelectorAll('.gameplay-skeleton span').forEach(n=>{Object.assign(n.style,{animationName:'skeleton',animationPlayState:'paused',animationDelay:('-'+(${phase}*parseFloat(getComputedStyle(n).animationDuration))+'s'),animationIterationCount:'1',animationFillMode:'both'});})`);
    }else await evaluate("document.querySelectorAll('.gameplay-skeleton span').forEach(n=>n.removeAttribute('style'))");
    await frames();
    const regions=await evaluate(`(()=>{const r=n=>{const x=n.getBoundingClientRect();return {left:x.left,right:x.right,top:x.top,bottom:x.bottom,width:x.width,height:x.height}},cards=[...document.querySelectorAll('.match-skeleton'),document.querySelector('.recent-ranked-section'),document.querySelector('.ability-section')];return cards.map((card,i)=>{const b=r(card);return {kind:i<5?'match-'+i:i===5?'recent-ranked':'ability',background:{left:b.left+24,right:b.left+28,top:b.top+5,bottom:b.top+9},blocks:[...card.querySelectorAll('.gameplay-skeleton span')].map(n=>{const x=r(n),hit=document.elementFromPoint(x.left+x.width*.5,x.top+x.height*.5);return {uncovered:hit===n || n.contains(hit),rect:x,animation:getComputedStyle(n).animationName,roi:{left:x.left+x.width*.25,right:x.left+x.width*.75,top:x.top+x.height*.35,bottom:x.top+x.height*.65}}})}})})()`);
    const png=decodePNG(await shot('skeleton-'+phase)),evidence=[];
    for(const card of regions){
     assert(card.blocks.length>=2,'at least two visible skeleton blocks '+JSON.stringify(card));const background=regionBrightness(png,card.background),blocks=[];
     for(const block of card.blocks){assert(block.uncovered,'skeleton sample is covered');assert(block.rect.width>0 && block.rect.height>0 && block.roi.bottom<=png.height && block.roi.top>=0,'skeleton block must be visible '+JSON.stringify(block));if(phase==='reduced')assert.equal(block.animation,'none','reduced motion must stop the animation');const pixels=regionBrightness(png,block.roi),difference=theme==='dark'?pixels.min-background.mean:background.mean-pixels.max;blocks.push({...block,pixels,difference});assert(difference>=(theme==='dark'?10:8),'faint skeleton '+JSON.stringify({theme,phase,kind:card.kind,background,pixels,difference}));}
     evidence.push({...card,background,blocks});
    }
    results.push({kind:'skeleton',theme,phase,cards:evidence});
   }
   await evaluate('__r267Complete()');await until("document.querySelector('.match-list .match-entry') && !document.querySelector('.match-skeleton,.career-pending') && !document.querySelector('#overview-content')._overviewViewTab.loading");
  }
  closedSessions.add(sessionId);await send('Target.closeTarget',{targetId});
 }
 assert.equal(violations.length,0,JSON.stringify(violations));assert.equal(errors.length,0,JSON.stringify(errors));
 fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({host:{platform:process.platform,arch:process.arch,node:process.version,browser:browserVersion},completedAt:new Date().toISOString(),scope:'Real Chromium, embedded public assets and CSP; controlled synthetic API fixtures; screenshot pixel measurements',csp,results,errors,violations,canceledInterceptions},null,2));
 console.log(JSON.stringify({cases:results.length,errors:errors.length,violations:violations.length}));
}
start().catch(e=>{fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'failure.json'),JSON.stringify({error:e.message,results,errors,violations},null,2));console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();chrome?.kill();backend?.kill();setTimeout(()=>fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100}),250).unref();});
