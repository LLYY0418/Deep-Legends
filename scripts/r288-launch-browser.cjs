'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 const p=await page('dark');await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
 for(const id of ['wegame','tcls','riot'])for(const mode of ['disappeared','never']){
  const value=await p.evaluate(`(()=>{const app=__r278App,s=app.state;s.section='overview';s.overviewTabIsCurrent=true;s.clientView={state:'no-client'};s.overviewGroup='${id==='riot'?'kr':'players'}';s.installationsLoaded=true;s.installations=[{id:'${id}',name:'${id}',available:true}];s.clientLaunched={id:'${id}',at:1000};s.clientLaunchWaiting={['${id}']:s.clientLaunched};s.clientLaunchPending={};s.clientLaunchInFlight='';app.renderLaunchpad();const starting=document.getElementById('launcher-list').textContent.includes('正在启动');if('${mode}'==='disappeared'){app.updateClientLaunchWaiting({known:true,launchers:{['${id}']:true}},2000);app.updateClientLaunchWaiting({known:true,launchers:{}},3000);app.updateClientLaunchWaiting({known:true,launchers:{}},6000)}else app.updateClientLaunchWaiting({known:true,launchers:{}},21000);app.renderLaunchpad();return {id:'${id}',mode:'${mode}',starting,button:document.getElementById('launcher-list').textContent,title:document.getElementById('launchpad-title').textContent}})()`);
  assert(value.starting);assert(!value.button.includes('正在启动'));assert.equal(value.title,'选择登录入口');results.push(value);
 }
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
