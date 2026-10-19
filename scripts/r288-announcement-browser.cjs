'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']){
  const p=await page(theme);await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
  await p.evaluate(`(async()=>{document.getElementById('announcements-button').click()})()`);
  await p.until("document.querySelector('#announcements-dialog strong.announcement-em')");
  const r=await p.evaluate(`(()=>{const n=document.querySelector('#announcements-dialog strong.announcement-em'),s=getComputedStyle(n),canvas=document.createElement('canvas'),ctx=canvas.getContext('2d');const rgb=color=>{ctx.clearRect(0,0,1,1);ctx.fillStyle=color;ctx.fillRect(0,0,1,1);return [...ctx.getImageData(0,0,1,1).data].slice(0,3)},lum=color=>rgb(color).map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((a,v,i)=>a+v*[.2126,.7152,.0722][i],0);const fg=lum(s.color),bg=lum(getComputedStyle(document.getElementById('announcements-dialog')).backgroundColor);return {text:n.textContent,weight:s.fontWeight,contrast:(Math.max(fg,bg)+.05)/(Math.min(fg,bg)+.05)}})()`);
  assert.equal(r.text,'21');assert.equal(r.weight,'700');assert(r.contrast>=4.5,JSON.stringify(r));results.push({theme,...r});
 }
 assert.deepEqual(errors,[]);
}).catch(e=>{console.error(e);process.exitCode=1});
