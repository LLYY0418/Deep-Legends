'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {bootDemoApp,installWindowCleanup}=require('./overview-render-helpers.cjs');
const {prefix}=require('./r230-fixture.cjs');
installWindowCleanup(test);
const wait=async predicate=>{const deadline=Date.now()+10000;while(!predicate()&&Date.now()<deadline)await new Promise(r=>setTimeout(r,20));assert.ok(predicate(),'UI state did not arrive')};
test('R230 three repeated champion/mastery round trips retain all career cards and scroll, including player overlay',async()=>{
 const {window:w,errors}=bootDemoApp({gameplaySourceTransform:source=>prefix+source});
 await wait(()=>w.document.querySelector('#overview-content [data-overview-subpage="champion-table"]'));
 for(const overlay of [false,true]){
  if(overlay){w.dispatchEvent(new w.CustomEvent('deep-legends:open-player',{detail:{source:'champions',playerRef:'player_00000000000000000000000000000001',gameName:'覆盖层玩家'}}));await wait(()=>w.document.querySelector('#player-overlay [data-overview-subpage="champion-table"]'));}
  const root=w.document.querySelector(overlay?'#player-overlay .player-overlay-scroll':'#overview-content'),scroll=overlay?root:w.document.getElementById('app-scroll');
  const count=root.querySelector('.career-column').children.length;
  for(let round=0;round<3;round++)for(const page of ['champion-table','masteries']){
   scroll.scrollTop=230;
   root.querySelector(`[data-overview-subpage="${page}"]`).click();
   await wait(()=>root.querySelector(page==='masteries'?'[data-mastery-detail]':'.champion-data-table'));
   root.querySelector('[data-overview-return]').click();
   assert.equal(root.querySelector('.career-column').children.length,count);
   assert.equal(scroll.scrollTop,230);assert.equal(root.querySelectorAll('.career-column').length,1);
   for(const title of ['排位','英雄胜率','英雄熟练度','位置偏好'])assert.match(root.querySelector('.career-column').textContent,new RegExp(title));
  }
 }
 assert.deepEqual(errors,[]);w.close();
});
