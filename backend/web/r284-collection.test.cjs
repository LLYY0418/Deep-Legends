'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs');
test('R284 P2 client-view alone clears collection controls and client counts in every section',()=>{
 for(const section of ['overview','favorites']){
  const dom=new JSDOM('<div id="account"></div><div id="pools"></div><div id="picker"></div>'+['owned','chroma','pool','remaining','toolbar','row','tabs','filters','footer'].map(id=>`<span id="${id}">1201</span>`).join(''));
  try{const d=dom.window.document,el={accountContent:d.querySelector('#account'),poolsContent:d.querySelector('#pools'),poolPicker:d.querySelector('#picker'),ownedCount:d.querySelector('#owned'),chromaCount:d.querySelector('#chroma'),poolCount:d.querySelector('#pool'),remainingCount:d.querySelector('#remaining'),poolPageTabs:[]};
   const state={section,clientView:{generation:1,state:'ready'},status:{connected:true,snapshotReady:true,snapshotFallback:true,calculationOK:true,ownedCount:1201,chromaOwnedCount:1309,remainingCount:209,poolTotal:554,collectionDirty:true,syncing:true},controllers:new Map()};
   const workspaceControls=Object.fromEntries(['toolbar','row','tabs','filters','footer'].map(id=>[id,d.querySelector('#'+id)]));
   const f=compile(fs.readFileSync(process.env.R284_APP_SOURCE||__dirname+'/app.js','utf8'),['acceptClientView','clearDisconnectedClientState','updateWorkspaceAvailability',...(fs.readFileSync(__dirname+'/app.js','utf8').includes('function renderCollectionCounts(')?['renderCollectionCounts']:[])],{state,el,workspaceControls,formatNumber:String,window:dom.window,CustomEvent:dom.window.CustomEvent,renderClientConnection(){},renderLaunchpad(){},retryCollectionAfterConnection(){},queueLiveUpdateSlices(){},reportClientView(){}});
   f.acceptClientView({type:'client-view',state:'no-client',generation:2});
   for(const n of Object.values(workspaceControls))assert.equal(n.hidden,true,section+' controls must hide synchronously without status fetch');
   for(const key of ['snapshotReady','snapshotFallback','calculationOK','collectionDirty','syncing'])assert.equal(state.status[key],false,key);
   for(const key of ['ownedCount','chromaOwnedCount','remainingCount'])assert.equal(state.status[key],null,key);
   assert.deepEqual([el.ownedCount.textContent,el.chromaCount.textContent,el.poolCount.textContent,el.remainingCount.textContent],['—','—','554','—']);
   f.acceptClientView({type:'client-view',state:'ready',generation:3});Object.assign(state.status,{snapshotReady:true,calculationOK:true,ownedCount:1202,chromaOwnedCount:1310,remainingCount:208});f.renderCollectionCounts(state.status);f.updateWorkspaceAvailability(state.status);
   for(const n of Object.values(workspaceControls))assert.equal(n.hidden,false);assert.equal(el.ownedCount.textContent,'1202');
  }finally{dom.window.close()}
 }
});
