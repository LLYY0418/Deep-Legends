"use strict";
const {browser}=require('./r297-browser-harness.cjs'),path=require('node:path'),fs=require('node:fs'),assert=require('node:assert/strict');
const out=path.join(process.env.R297_REPORT_DIR||path.join(require('node:os').tmpdir(),'r297-browser-proof'),'p5-settings');
(async()=>{const b=await browser({out});const results=[];try{
 await b.call('Page.navigate',{url:b.origin+'/?demo'});await b.until('window.__r297');
 await b.evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:'settings',page:'gameplay'}}))`);
 await b.until(`window.deepLegendsSettingsBackup && getComputedStyle(document.querySelector('.settings-tag-options label')).fontSize==='13px'`);
 for(const theme of ['dark','light'])for(const width of [1440,960]){
  await b.call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});await b.evaluate(`document.documentElement.dataset.theme='${theme}'`);
  const result=await b.evaluate(`(()=>{const nodes=[...document.querySelectorAll('.settings-tag-options label')];return {labels:nodes.map(n=>({height:n.getBoundingClientRect().height,size:getComputedStyle(n).fontSize,weight:getComputedStyle(n).fontWeight,align:getComputedStyle(n).alignItems})),overflow:document.documentElement.scrollWidth>innerWidth+1}})()`);
  assert(result.labels.length);for(const l of result.labels){assert.equal(l.size,'13px');assert.equal(l.weight,'600');assert.equal(l.height,32);assert.equal(l.align,'center')}assert(!result.overflow);results.push({theme,width,...result});await b.shot(theme+'-'+width);
 }
 assert.deepEqual(b.errors,[]);fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({results,errors:b.errors},null,2));
}finally{await b.stop()}})().catch(e=>{console.error(e);process.exitCode=1});
