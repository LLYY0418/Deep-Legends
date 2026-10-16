"use strict";
const {waitOutcome}=require('./wait-progress.cjs');
function appSnapshot() {
  const app=document.querySelector('#app-frame');
  const appReady=typeof window.deepLegendsToast==='function';
  return {ready:document.readyState==='complete'&&appReady&&!!app&&!app.hasAttribute('inert'),readyState:document.readyState,appReady,present:!!app,inert:app?app.hasAttribute('inert'):null};
}
const APP_READY_EXPRESSION=`(${appSnapshot})()`;
async function waitSnapshot({evaluate,expression,label,...options}) {
  let last;
  const outcome=await waitOutcome(async()=>{last=await evaluate(expression);return last.ready;},{idleMs:15000,hardMs:60000,intervalMs:50,...options,progress:()=>last});
  if(!outcome.ok){const error=Error(`${label}: ${outcome.reason}; waited ${outcome.ms} ms; last progress ${outcome.progress}`);error.code=outcome.reason==='stalled'?'WAIT_STALLED':'WAIT_HARD_LIMIT';error.snapshot=last;throw error;}
  return last;
}
function waitAppReady(options){return waitSnapshot({...options,expression:APP_READY_EXPRESSION,label:'app-frame complete and navigation listener ready'});}
async function navigateAppSection({section,panelId=`${section}-panel`,...options}) {
  await waitAppReady(options);
  await options.evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:${JSON.stringify(section)}}}))`);
  return waitSnapshot({...options,label:`${section} active section ready`,expression:`(()=>{const activeSection=[...document.querySelectorAll("main > [role='tabpanel'], main > [data-standalone-page]")].filter(panel=>!panel.hidden).map(panel=>panel.id);return {ready:activeSection.length===1&&activeSection[0]===${JSON.stringify(panelId)},activeSection,readyState:document.readyState};})()`});
}
module.exports={appSnapshot,APP_READY_EXPRESSION,waitAppReady,navigateAppSection};
