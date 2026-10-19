'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('../../desktop/node_modules/jsdom');
const source=fs.readFileSync(__dirname+'/pro-players.js','utf8');
function fixture(fresh){
 const ids=['pro-players-panel','pro-players-content','pro-players-status','pro-team-filters','pro-players-search','pro-players-refresh','pro-players-home','pro-players-return','pro-primary-only'];
 const dom=new JSDOM(ids.map(id=>`<div id="${id}"></div>`).join(''),{url:'https://fixture.test',runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window;
 let now=Date.UTC(2026,9,19);w.Date.now=()=>now;const requests=[],timers=[];w.setTimeout=(callback,delay)=>(timers.push({callback,delay}),timers.length);w.clearTimeout=()=>{};
 w.HTMLElement.prototype.getBoundingClientRect=function(){const off=this.dataset.proRow?.includes('Offscreen');return {top:off?2000:100,bottom:off?2050:150}};
 const account=(name,status='unavailable')=>({gameName:name,tagLine:'KR1',rankStatus:status,...(fresh?{nextRefreshAt:new Date(now+10800000).toISOString(),checkedAt:new Date(now).toISOString()}:{} )});
 const data={fetchedAt:new Date(now).toISOString(),teams:[{code:'T1',name:'T1',players:[{key:'p',name:'Faker',position:'middle',accounts:[account('Visible'),account('Offscreen'),account('Confirmed','unranked')]}]},{code:'BLG',name:'BLG',players:[{key:'b',name:'Knight',accounts:[account('OtherOffscreen')]}]}]};
 w.fetch=async(url,options)=>{requests.push({url,body:options?.body&&JSON.parse(options.body)});return {ok:true,json:async()=>url==='/api/pro-players'?structuredClone(data):{updating:true,due:1}}};
 let entry;w.deepLegendsSections={register:(_name,fn)=>{entry=fn}};w.eval(source);
 return {dom,w,requests,timers,enter:()=>entry({detail:{name:'pro-players'}}),leave:()=>entry({detail:{name:'overview'}}),advance:ms=>{now+=ms}};
}
const flush=async()=>{await new Promise(setImmediate);await new Promise(setImmediate)};
test('R288 pro first screen shows pending names, only refreshes viewport and polls at least 3s',async()=>{
 const f=fixture(false);try{f.enter();await flush();const text=f.w.document.getElementById('pro-players-content').textContent;
 assert(text.includes('Visible')&&text.includes('Offscreen')&&text.includes('OtherOffscreen'));assert.equal(f.w.document.querySelectorAll('.pro-rank-skeleton').length,3);
 const plan=f.requests.find(r=>r.body);assert.deepEqual(plan.body.accounts,['visible#kr1']);assert(!plan.body.accounts.includes('offscreen#kr1'));assert(f.timers.some(t=>t.delay===3000));
 f.w.document.querySelector('[data-pro-team="T1"]').click();await flush();const last=f.requests.filter(r=>r.body).at(-1);assert.equal(last.body.team,'T1');assert(last.body.accounts.includes('confirmed#kr1'));
 }finally{f.dom.window.close()}
});
test('R288 pro reentry and foreground visibility within 3h make zero requests',async()=>{
 const f=fixture(true);try{f.enter();await flush();assert.equal(f.requests.length,1);assert(f.w.document.querySelector('.pro-account-name'));f.requests.length=0;f.leave();f.advance(2*3600000);f.enter();await flush();f.w.document.dispatchEvent(new f.w.Event('visibilitychange'));await flush();assert.deepEqual(f.requests,[]);
 }finally{f.dom.window.close()}
});
