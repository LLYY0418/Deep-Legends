'use strict';
const test=require('node:test'), assert=require('node:assert/strict'), fs=require('node:fs'), path=require('node:path'), vm=require('node:vm');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R219_APP_SOURCE || path.join(__dirname,'app.js'),'utf8').replace(/^[ \t]*\/\/.*$/gm,'');
const items=[{id:'tcls',name:'TCLS',available:true},{id:'riot',name:'Riot 客户端',available:true}];
const disconnected=clientDiscovery=>({connected:false,clientDiscovery,connectionState:'disconnected',identityReady:false,snapshotReady:false,syncing:false});
function harness(options={}) {
  const dom=new JSDOM(fs.readFileSync(path.join(__dirname,'index.html'),'utf8'));
  const doc=dom.window.document, el={};
  for(const id of ['connection','connection-avatar','refresh','owned-count','chroma-count','pool-count','remaining-count','pool-source','settings-build-identity',
    'client-launchpad','launcher-list','launchpad-eyebrow','launchpad-title','launchpad-description','client-launch-reselect','official-login-status',
    'startup-loading','startup-loading-title','startup-loading-copy','startup-loading-meta','startup-loading-retry','app-frame'])el[id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase())]=doc.getElementById(id);
  const state={section:'overview',statusDelay:10000,statusRequestToken:0,overlaySuppressed:false};
  const requests=[],events=[],timers=new Map(),recoveries=[];let id=0, f, next=disconnected('process-not-found');
  const deps={state,el,document:doc,window:{dispatchEvent:e=>events.push({event:e.type}),reportFlowDiagnostic:(event,reason,fields)=>events.push({event,reason,...fields}),deepLegendsStatusRecovery:x=>recoveries.push(x)},
    CustomEvent:dom.window.CustomEvent,escapeHTML,formatNumber:String,safeHTTPURL:()=>false,STATUS_INTERVAL:10000,
    setTimeout:(fn,delay)=>{timers.set(++id,{fn,delay});return id;},clearTimeout:key=>timers.delete(key),
    api:async url=>{requests.push(url);if(url==='/api/status')return {...next};if(url.startsWith('/api/client-installations')){if(options.scanError)throw options.scanError;return {items:options.items??items};}throw Error(url);},
    clearDisconnectedClientState:()=>{},scheduleStatus:()=>{},renderNotice:()=>{},updateWorkspaceAvailability:()=>{},renderUpdateStatus:()=>{},
    launchOfficialLogin:()=>{},loadAccount:()=>{},loadPools:()=>{},loadSkins:()=>{},
  };
  const names=['refreshStatus','reportStatusRenderFailed','renderStatus','loadClientInstallations','renderLaunchpad','updateReadingOverlay','showReadingOverlay','hideReadingOverlay','snapshotRetryText'];
  if(options.throwLaunchpad){names.splice(names.indexOf('renderLaunchpad'),1);deps.renderLaunchpad=function renderLaunchpad(){throw new TypeError('SECRET /private/user/key');};}
  f=compile(source,names,deps);
  return {...f,state,el,doc,requests,events,timers,recoveries,setStatus:s=>next=s,close:()=>dom.window.close()};
}

test('R219 fresh state without installations renders status, scans once and shows both login entries',async()=>{
  const h=harness();try {
    assert.equal(Object.hasOwn(h.state,'installations'),false);
    h.state.status=disconnected('process-not-found');assert.doesNotThrow(()=>h.renderStatus());
    await h.refreshStatus();assert.equal(h.requests.filter(x=>x==='/api/client-installations').length,1);
    assert.deepEqual([...h.el.launcherList.querySelectorAll('[data-client-id]')].map(n=>n.dataset.clientId),['tcls','wegame']);
    assert.match(h.el.launcherList.textContent,/国服纯净入口/);assert.match(h.el.launcherList.textContent,/WeGame/);
    assert(h.events.some(e=>e.event==='deep-legends:status'));
    await h.refreshStatus();assert.equal(h.requests.filter(x=>x==='/api/client-installations').length,1);
    assert.match(source,/installations:\s*\[\]/);
  }finally{h.close();}
});

test('R219 failed and empty installation scans both offer rescan with distinct messages',async()=>{
  for(const options of [{scanError:new Error('fixture failure')},{items:[]}]) {
    const h=harness(options);try {
      await h.refreshStatus();if(options.scanError) assert.match(h.el.launcherList.textContent,/安装位置检查失败/);else {assert.match(h.el.officialLoginStatus.textContent,/未找到可启动/);assert.equal(h.el.launcherList.querySelectorAll('[data-client-id]:disabled').length,2);}
      const retry=h.el.launcherList.querySelector('.scan-launchers');assert(retry);retry.click();await new Promise(setImmediate);
      assert(h.requests.includes('/api/client-installations?force=1'));
    }finally{h.close();}
  }
});

test('R219 no-process/query-failed hides immediately even during connecting retries',()=>{
  for(const result of ['process-not-found','process-query-failed']) {
    const h=harness();try {
      h.state.status=disconnected('');h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,false);
      h.state.status={...disconnected(result),connectionState:'connecting'};h.updateReadingOverlay();
      assert.equal(h.el.startupLoading.hidden,true);assert.equal(h.el.appFrame.hasAttribute('inert'),false);
      assert.equal(h.state.overlaySuppressed,false);assert.equal(h.state.statusDelay,10000);assert.equal(h.timers.size,0);
      assert.equal(h.events.find(e=>e.reason==='hide').hide_reason,'no-client-process');assert(!h.events.some(e=>e.reason==='timeout'));
      h.updateReadingOverlay();assert.equal(h.timers.size,0);assert.equal(h.el.startupLoading.hidden,true);
    }finally{h.close();}
  }
});

test('R219 checking and detected-process overlays have truthful copy and late client starts can reopen',()=>{
  const h=harness();try {
    h.state.status=disconnected('');h.updateReadingOverlay();assert.equal(h.el.startupLoadingCopy.textContent,'正在检测英雄联盟客户端。');
    h.state.status=disconnected('process-not-found');h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,true);
    for(const result of ['credentials-unreadable','probe-failed']) {
      h.state.status=disconnected(result);h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,false);
      assert.equal(h.el.startupLoadingCopy.textContent,'检测到客户端正在启动，请稍候。');assert.equal(h.state.statusDelay,900);
    }
    h.state.status={connected:true,identityReady:false};h.updateReadingOverlay();assert.equal(h.el.startupLoadingTitle.textContent,'正在读取召唤师信息');
    h.state.status.identityReady=true;h.updateReadingOverlay();assert.equal(h.el.startupLoading.hidden,true);
    assert.equal(h.events.filter(e=>e.reason==='hide').at(-1).hide_reason,'identity-ready');
  }finally{h.close();}
});

test('R219 timeout and suppressed hides retain their fixed reasons and bounded fallback',()=>{
  const h=harness();try {
    h.state.status=disconnected('probe-failed');h.updateReadingOverlay();
    const fallback=[...h.timers.values()][0];assert.equal(fallback.delay,15000);fallback.fn();
    assert.equal(h.el.startupLoading.hidden,true);assert.equal(h.state.overlaySuppressed,true);
    assert.equal(h.events.find(e=>e.reason==='hide').hide_reason,'timeout');assert.equal(h.events.filter(e=>e.reason==='timeout').length,1);
    h.el.startupLoading.hidden=false;h.updateReadingOverlay();assert.equal(h.events.filter(e=>e.reason==='hide').at(-1).hide_reason,'suppressed');
  }finally{h.close();}
});

test('R219 launchpad render exceptions are reported without blocking installation requests or blaming status transport',async()=>{
  const h=harness({throwLaunchpad:true});try {
    await h.refreshStatus();await new Promise(setImmediate);
    assert.equal(h.requests.filter(x=>x==='/api/client-installations').length,1);assert.equal(h.state.installationsLoaded,true);
    const errors=h.events.filter(e=>e.event==='status_render_failed');assert(errors.length);
    for(const e of errors){assert.equal(e.errorType,'TypeError');assert.equal(e.functionName,'renderLaunchpad');}
    assert.doesNotMatch(JSON.stringify(errors),/SECRET|private|key/);assert.equal(h.state.statusFailures,0);assert(!h.recoveries.includes(true));
  }finally{h.close();}
});

test('R219 runtime permits only fixed render metadata and overlay hide reasons',async()=>{
  const dom=new JSDOM('<body></body>',{url:'http://localhost'}),bodies=[];
  try {
    vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),{window:dom.window,document:dom.window.document,fetch:async(_url,options)=>{bodies.push(JSON.parse(options.body));return {ok:true,status:204};},setTimeout:()=>1,clearTimeout:()=>{},setInterval:()=>1,clearInterval:()=>{},Date,URL,URLSearchParams,AbortController,console});
    dom.window.reportFlowDiagnostic('status_render_failed','failed',{errorType:'TypeError',functionName:'renderLaunchpad',requestId:123,source:'SECRET',stack:'SECRET',message:'SECRET'});
    dom.window.reportFlowDiagnostic('status_render_failed','failed',{errorType:'SECRET',functionName:'/private/SECRET'});
    dom.window.reportFlowDiagnostic('blocking_state_client','hide',{source:'startup',hide_reason:'no-client-process'});
    dom.window.reportFlowDiagnostic('blocking_state_client','hide',{source:'startup',hide_reason:'SECRET'});
    await new Promise(setImmediate);assert.equal(bodies.length,4);
    assert.deepEqual(Object.fromEntries(Object.entries(bodies[0]).filter(([key])=>!key.startsWith('transport'))),{event:'status_render_failed',reason:'failed',errorType:'TypeError',functionName:'renderLaunchpad'});
    assert.equal(bodies[1].functionName,'other');assert.equal(bodies[1].errorType,'Error');
    assert.equal(bodies[2].hide_reason,'no-client-process');assert.equal(bodies[3].hide_reason,undefined);assert.doesNotMatch(JSON.stringify(bodies),/SECRET|requestId/);
  }finally{dom.window.close();}
});
