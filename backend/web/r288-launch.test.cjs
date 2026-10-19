'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(__dirname+'/app.js','utf8');
for(const id of ['wegame','tcls','riot'])test(`R288 ${id} resets after disappearance and never-appeared deadline`,()=>{
 const launched={id,at:1000},state={clientLaunchWaiting:{[id]:launched},clientLaunched:launched};
 const f=compile(source,['updateClientLaunchWaiting'],{state});
 f.updateClientLaunchWaiting({known:true,launchers:{[id]:true}},2000);
 f.updateClientLaunchWaiting({known:true,launchers:{}},3000);
 f.updateClientLaunchWaiting({known:true,launchers:{}},5999);assert(state.clientLaunchWaiting[id]);
 f.updateClientLaunchWaiting({known:true,launchers:{}},6000);assert.equal(state.clientLaunched,null);assert.deepEqual(state.clientLaunchWaiting,{});
 state.clientLaunched={id,at:1000};state.clientLaunchWaiting[id]=state.clientLaunched;
 f.updateClientLaunchWaiting({known:true,launchers:{}},20999);assert(state.clientLaunched);
 f.updateClientLaunchWaiting({known:true,launchers:{}},21000);assert.equal(state.clientLaunched,null);
 state.clientLaunched={id,at:1000};state.clientLaunchWaiting[id]=state.clientLaunched;
 f.updateClientLaunchWaiting({known:true,leagueRunning:true},2000);
 f.updateClientLaunchWaiting({known:true,launchers:{}},100000);assert(state.clientLaunched,'LeagueClient follows existing connection flow');
});
