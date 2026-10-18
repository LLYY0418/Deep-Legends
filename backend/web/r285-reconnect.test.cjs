'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R285_SUITE_SOURCE||__dirname+'/suite.js','utf8');
test('R285 reconnect canceled card has no status sentence; failed and fired remain accurate',()=>{
 const events=new Map(),state={watchEvents:events,phase:'Reconnect'};
 const f=compile(source,['watchRuleCard'],{state,escapeHTML:String,watchRuleControl:()=>'',watchConflictNote:()=>'',checked:()=>'',suiteDisplayPhase:x=>x});
 const d={key:'autoReconnect',action:'reconnect',phaseKey:'Reconnect',title:'断线自动重连'};
 for(const kind of ['canceled','failed','fired']){
  events.set('reconnect',{kind});const html=f.watchRuleCard(d,{enabled:true},true,{});
  const foot=html.match(/<div class="watch-rule-foot">(.*?)<\/div>/)[1];
  if(kind==='canceled')assert.equal(foot,'');
  if(kind==='failed')assert(foot.includes('上次触发失败'));
  if(kind==='fired')assert(!foot.includes('上次触发失败'));
 }
});
