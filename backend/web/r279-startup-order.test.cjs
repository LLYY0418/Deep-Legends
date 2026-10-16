"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {compile}=require('./r188-harness.cjs');
const app=fs.readFileSync(path.join(__dirname,'app.js'),'utf8'),gameplay=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
function scenario(order,{dropReplay=false}={}) {
 const events=new EventTarget(),appState={},state={status:null},applied=[];let pulls=0,writes=0;
 const window={addEventListener:(...args)=>events.addEventListener(...args),dispatchEvent:event=>events.dispatchEvent(event)};
 const f=compile(app,['readCommittedClientStatus','acceptClientView'],{state:appState,window,CustomEvent,renderClientConnection(){},renderLaunchpad(){},clearDisconnectedClientState(){},retryCollectionAfterConnection(){},queueLiveUpdateSlices(){},reportClientView(){}});
 window.deepLegendsStatusSnapshot=f.readCommittedClientStatus;
 const g=compile(gameplay,['updateStatus','updateClientView','restoreCommittedClientStatus'],{state,window,applyClientViewStatus:status=>{state.status=status;applied.push(status.clientView.generation);},fetch:()=>pulls++,localStorage:{setItem:()=>writes++}});
 const listen=()=>{window.addEventListener('deep-legends:client-view',e=>g.updateClientView(e.detail));window.addEventListener('deep-legends:status',e=>g.updateStatus(e.detail));if(!dropReplay)g.restoreCommittedClientStatus();};
 const view={type:'client-view',state:'ready',generation:10,region:'TENCENT',serverId:'HN1',summoner:{playerRef:'fixture'}};
 const publish=()=>{appState.status={queueGroups:['fixture'],clientView:view};f.acceptClientView(view);};
 if(order==='app-first'){publish();listen();}else{listen();publish();}
 assert.equal(state.status?.clientView?.state,'ready','late gameplay subscriber must consume committed ready state');
 assert.equal(state.status.clientView.serverId,'HN1');assert.deepEqual(applied,[10]);
 g.restoreCommittedClientStatus();assert.deepEqual(applied,[10],'same generation replay must be idempotent');
 f.acceptClientView({...view,state:'exiting',generation:11});g.restoreCommittedClientStatus();
 f.acceptClientView(view);g.restoreCommittedClientStatus();
 assert.equal(state.status.clientView.state,'exiting');assert.deepEqual(applied,[10,11],'stale ready must not restore a disconnected client');
 assert.equal(pulls,0,'state replay must not fetch');assert.equal(writes,0,'state replay must not persist preferences');
 const snapshot=f.readCommittedClientStatus();assert.notEqual(snapshot,appState.status,'consumer receives a separate status envelope');
 return {state,applied};
}
for(const order of ['app-first','gameplay-first'])test('startup committed state replay '+order,()=>scenario(order));
test('mutation: removing startup replay fails the late subscriber target',()=>assert.throws(()=>scenario('app-first',{dropReplay:true}),/late gameplay subscriber must consume committed ready state/));
test('startup replay is wired after gameplay initialization and app getter before first poll',()=>{
 assert(app.indexOf('window.deepLegendsStatusSnapshot = readCommittedClientStatus;')<app.indexOf('void window.deepLegendsLicense?.poll()'));
 const tail=gameplay.slice(gameplay.lastIndexOf('  bindSettings();'));
 assert(tail.indexOf('restoreCommittedClientStatus();')>tail.indexOf('renderCapabilitySettings();'));
});
