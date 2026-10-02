'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {boot,until,matches,mount}=require('./r192-harness.cjs');
test('R192 1 hydrate updates roster and subject highlight while other two cards stay retained',async()=>{
 const f=await boot();try{
  const full=matches().full;const v=mount(f,{loadMatchDetails:async()=>structuredClone(full)}),before=[...v.host.querySelectorAll('.match-entry')];
  assert.match(before[0].querySelector('.match-players').textContent,/轻量主体/);
  before[0].querySelector('[data-toggle-match]').click();
  await until(()=>v.card(19201).querySelector('.match-detail'),'hydrated detail missing');
  assert.match(v.card(19201).querySelector('.match-players').textContent,/补全主体/);
  assert.doesNotMatch(v.card(19201).querySelector('.match-players').textContent,/轻量/);
  assert.equal(v.card(19201).querySelectorAll('.match-player-name.is-current-player').length,1);
  assert.equal(v.card(19201).querySelector('.is-current-player').textContent,'补全主体');
  assert.equal(v.card(19202),before[1]);assert.equal(v.card(19203),before[2]);assert.deepEqual(f.errors,[]);
 }finally{f.close()}
});
test('R192 2 rejected details expose a working retry and reopen after successful second request',async()=>{
 const f=await boot();try{
  let calls=0;const full=matches().full;const v=mount(f,{loadMatchDetails:async()=>{if(++calls===1)throw Error('夹具断网');return structuredClone(full)}});
  v.card(19201).querySelector('[data-toggle-match]').click();await until(()=>v.card(19201).querySelector('[data-retry-match-detail]'),'failure retry missing');
  assert.match(v.card(19201).textContent,/完整详情未加载/);assert.equal(calls,1);
  assert.equal(v.card(19201).querySelector('[data-toggle-match]').disabled,true);
  v.card(19201).querySelector('[data-retry-match-detail]').click();assert.equal(calls,2,'retry must request again');
  await until(()=>v.card(19201).querySelector('.match-detail'),'retry did not expand detail');assert.equal(calls,2);
  assert.equal(v.card(19201).querySelector('[data-toggle-match]').disabled,false);assert.equal(v.card(19201).querySelector('[data-retry-match-detail]'),null);
  v.card(19201).querySelector('[data-toggle-match]').click();assert.equal(v.card(19201).querySelector('.match-detail'),null);
  v.card(19201).querySelector('[data-toggle-match]').click();assert.ok(v.card(19201).querySelector('.match-detail'));assert.equal(calls,2);assert.deepEqual(f.errors,[]);
 }finally{f.close()}
});
test('R192 3 two retry clicks during loading share one additional request',async()=>{
 const f=await boot();try{
  let calls=0,release;const full=matches().full;const v=mount(f,{loadMatchDetails:()=>++calls===1?Promise.reject(Error('夹具断网')):new Promise(resolve=>{release=resolve})});
  v.card(19201).querySelector('[data-toggle-match]').click();await until(()=>v.card(19201).querySelector('[data-retry-match-detail]'));
  const retry=v.card(19201).querySelector('[data-retry-match-detail]');retry.click();retry.click();assert.equal(calls,2);
  assert.equal(v.card(19201).querySelector('[data-toggle-match]').disabled,true);
  release(structuredClone(full));await until(()=>v.card(19201).querySelector('.match-detail'));assert.equal(calls,2);assert.deepEqual(f.errors,[]);
 }finally{f.close()}
});
test('R192 4 loader-free expand and collapse keep summary and unrelated card identities',async()=>{
 const f=await boot();try{
  const v=mount(f),before=[...v.host.querySelectorAll('.match-entry')],summary=before[0].querySelector('.match-summary');
  for(const open of [true,false,true,false]){v.card(19201).querySelector('[data-toggle-match]').click();assert.equal(Boolean(v.card(19201).querySelector('.match-detail')),open);assert.equal(v.card(19201),before[0]);assert.equal(v.card(19201).querySelector('.match-summary'),summary);assert.equal(v.card(19202),before[1]);assert.equal(v.card(19203),before[2]);}
  assert.deepEqual(f.errors,[]);
 }finally{f.close()}
});
test('R192 5 hydrated Arena summary replaces damage and taken placeholders with true fixture values',async()=>{
 const f=await boot();try{
  const full=matches().full;const v=mount(f,{loadMatchDetails:async()=>structuredClone(full)});
  assert.equal(v.card(19201).querySelector('.match-stat-damage b').textContent,'—');assert.equal(v.card(19201).querySelector('.match-stat-taken b').textContent,'—');
  v.card(19201).querySelector('[data-toggle-match]').click();await until(()=>v.card(19201).querySelector('.match-detail'));
  assert.equal(v.card(19201).querySelector('.match-stat-damage b').textContent,'98209');assert.equal(v.card(19201).querySelector('.match-stat-taken b').textContent,'47140');assert.deepEqual(f.errors,[]);
 }finally{f.close()}
});
