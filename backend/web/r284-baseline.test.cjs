'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs');
const read=(name,key)=>fs.readFileSync(process.env[key] || path.join(__dirname,name),'utf8');
const flush=async()=>{await new Promise(setImmediate);await new Promise(setImmediate);};

async function friendFixture(disconnectWhileLoading=false){
 const html=read('index.html','R284_INDEX_SOURCE'),app=read('app.js','R284_APP_SOURCE');
 const dom=new JSDOM(html,{url:'http://fixture/?section=friends',runScripts:'outside-only'}),w=dom.window,d=w.document;
 w.deepLegendsRuntime={escapeHTML:String};w.fetch=async()=>({ok:true,json:async()=>({groups:[],friends:[]})});
 w.__r284State={status:{connected:false}};
 w.eval(read('section-loader.js','R284_LOADER_SOURCE'));
 w.addEventListener('deep-legends:status',e=>{w.__r284State.status=e.detail;});
 const start=app.indexOf('  let openFriendsFromLink = false;'),end=app.indexOf('\n  // 总览页切换玩家',start);
 assert(start>=0 && end>start);
 w.eval('const state=window.__r284State;function activateSection(){};'+app.slice(start,end));
 const status=connected=>w.dispatchEvent(new w.CustomEvent('deep-legends:status',{detail:{connected}}));
 try{
  w.dispatchEvent(new w.Event('hashchange'));await flush();
  const toggle=d.getElementById('friends-toggle');assert.equal(toggle.disabled,true);assert.equal(d.querySelector('script[data-section-module="friends"]'),null);
  status(true);await flush();const script=d.querySelector('script[data-section-module="friends"]');assert(script);assert.equal(toggle.disabled,true,'module has not finished');
  if(disconnectWhileLoading)status(false);
  w.eval(read('friends.js','R284_FRIENDS_SOURCE'));script.onload();await flush();
  assert.equal(w.deepLegendsFriendsReady,true);assert.equal(toggle.disabled,disconnectWhileLoading,'loader must replay the latest connection state');
  if(disconnectWhileLoading){assert(!d.getElementById('friends-dock').classList.contains('is-open'));status(true);await flush();}
  assert.equal(toggle.disabled,false);assert(d.getElementById('friends-dock').classList.contains('is-open'),'pending link opens after module registration');
  status(false);assert.equal(toggle.disabled,true);status(true);await flush();assert.equal(toggle.disabled,false);
  assert.equal(d.querySelectorAll('script[data-section-module="friends"]').length,1);
 }finally{w.dispatchEvent(new w.CustomEvent('deep-legends:dispose'));w.close();}
}
test('R284 P0 delayed friends replay connection and open the pending link',async()=>{await friendFixture();});
test('R284 P0 delayed friends replay disconnect and recover on the next connection',async()=>{await friendFixture(true);});

test('R284 P0 manual refresh releases its button after 60 seconds and later syncing polls cannot relock it',async()=>{
 const app=read('app.js','R284_APP_SOURCE'),dom=new JSDOM('<button><i class="refresh-icon"></i><span>重新读取</span></button>'),w=dom.window;
 const state={status:{connected:true,syncing:false}},button=w.document.querySelector('button'),toasts=[];let now=0,reads=0;
 const el={refresh:button};for(const key of ['ownedCount','chromaCount','poolCount','remainingCount','poolSource'])el[key]=w.document.createElement('span');
 const deps={state,el,window:w,document:w.document,CustomEvent:w.CustomEvent,performance:{now:()=>now},showToast:t=>toasts.push(t),softResetShellState(){},api:async()=>{},
  refreshStatus:async()=>{assert(++reads<400,'manual refresh never stops while backend remains syncing');},setTimeout:fn=>{now+=200;queueMicrotask(fn);},
  acceptClientView(){},renderClientConnection(){},formatNumber:String,safeHTTPURL:()=>false,renderNotice(){},renderLaunchpad(){},updateWorkspaceAvailability(){},renderUpdateStatus(){}};
 const f=compile(app,['runManualRefresh','setRefreshBusy','renderStatus'],deps);
 try{
  await f.runManualRefresh();assert.equal(now,60000);assert.equal(state.manualRefreshTimedOut,true);assert.equal(state.manualRefreshing,false);assert.equal(button.disabled,false);assert.equal(button.lastElementChild.textContent,'重新读取');
  assert(!toasts.includes('对局与总览已重新读取'),'timeout cannot claim success');
  for(let i=0;i<3;i++){f.renderStatus();assert.equal(button.disabled,false);}
  state.status.syncing=false;f.renderStatus();assert.equal(state.manualRefreshTimedOut,false);
 }finally{w.close();}
});
