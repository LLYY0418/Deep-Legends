"use strict";
const test=require('node:test'),assert=require('node:assert/strict');
const {bootDemoApp}=require('./overview-render-helpers.cjs'),{waitForRender}=require('./renderer-wait.cjs');
test('R309 no-filter normal pagination displays every loaded twenty-row page and grows the DOM',async()=>{
 const pages=[],{window:w,errors}=bootDemoApp({matchCount:60,pagedMatches:true,onOverviewPage:page=>pages.push(page)});
 try{
  const count=()=>w.document.querySelectorAll('.match-list .match-entry').length;
  await waitForRender(w,()=>count()===20);assert.deepEqual(pages,[{beg:0,count:20}]);
  for(const expected of [40,60]){
   const before=[...w.document.querySelectorAll('.match-list .match-entry')];
   const more=w.document.querySelector('[data-load-more]');assert.ok(more);more.click();
   await waitForRender(w,()=>count()===expected);
   const after=[...w.document.querySelectorAll('.match-list .match-entry')];
   before.forEach((node,i)=>assert.equal(after[i],node,'ordinary pagination retains loaded rows'));
   assert.equal(after.filter(node=>!node.hidden).length,expected);
  }
  assert.deepEqual(pages,[{beg:0,count:20},{beg:20,count:20},{beg:40,count:20}]);assert.deepEqual(errors,[]);
 }finally{w.close()}
});
