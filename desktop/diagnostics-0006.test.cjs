'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('jsdom');
const read=f=>fs.readFileSync(path.join(__dirname,'../web',f),'utf8');
test('one event card preserves separate choice owners, partial selection, failures and POST keys',async()=>{
 const dom=new JSDOM('<div id="sweep"></div>',{runScripts:'outside-only'}),w=dom.window;
 try{
  const source=read('suite.js'),functions=source.slice(source.indexOf('  function claimSelectionKeys('),source.indexOf('  async function loadClaims('));
  w.eval(`const state={claims:{items:[]},claimFilter:'all',selectedClaims:new Set(),claimFailures:new Map(),claimChoices:new Map(),claiming:false,claimProgress:{done:0,total:0,failed:0}},roots={sweep:document.getElementById('sweep')},metrics={claim:document.createElement('span')},escapeHTML=x=>String(x||''),checked=x=>x?' checked':'',relativeTime=()=>'',imageURL=x=>x,sourceNames={grant:'奖励账本'},toast=()=>{};
   const requests=[];async function api(path,req){requests.push(JSON.parse(req.body));return {results:[{ok:true}]};}
   ${functions};window.probe={state,renderClaims,executeClaims,requests};`);
  const {state,renderClaims,executeClaims,requests}=w.probe;
  state.claims.items=[
   {key:'grant:one',eventId:'a',eventName:'活动A',items:[{id:'one',quantity:750}]},
   {key:'grant:choice',eventId:'a',eventName:'活动A',needsChoice:true,minSelections:1,maxSelections:1,items:[{id:'x'},{id:'y'}]},
   {key:'grant:choice2',eventId:'a',eventName:'活动A',needsChoice:true,minSelections:1,maxSelections:1,items:[{id:'x'},{id:'y'}]},
   {key:'event:a',eventId:'a',eventName:'活动A',actionable:false,overlapWith:'grant'},
   {key:'grant:unknown',items:[{id:'one',quantity:750}]},
  ];
  renderClaims();
  assert.equal(w.document.querySelectorAll('.claim-event-card').length,1);
  assert.equal(w.document.querySelectorAll('article').length,2);
  assert.equal(w.document.querySelectorAll('[data-claim-row="event:a"]').length,0);
  assert.equal(w.document.querySelectorAll('[data-claim-choice="grant:choice"]').length,2);
  w.document.querySelector('[data-claim-choice="grant:choice"][data-reward-id="y"]').click();
  w.document.querySelector('[data-claim-choice="grant:choice2"][data-reward-id="x"]').click();
  assert.equal(state.claimChoices.get('grant:choice').has('y'),true);
  assert.equal(state.claimChoices.get('grant:choice2').has('x'),true);
  w.document.querySelector('[data-claim-check="grant:one"]').click();
  assert.equal(w.document.querySelector('[data-claim-event]').indeterminate,true);
  w.document.querySelector('[data-claim-event]').click();
  assert.deepEqual([...state.selectedClaims],['grant:one','grant:choice','grant:choice2']);
  await executeClaims();
  assert.deepEqual(Array.from(requests,x=>x.items[0].key),['grant:one','grant:choice','grant:choice2']);
  assert.deepEqual([...requests[1].items[0].selections],['y']);
  state.claimFailures.set('grant:choice',{message:'failure'});state.claimFilter='failed';renderClaims();
  assert.equal(w.document.querySelectorAll('.claim-event-card').length,1);
  assert.equal(w.document.querySelectorAll('[data-claim-row]').length,1);
  assert.match(w.document.querySelector('.claim-error').textContent,/failure/);
 }finally{w.close();}
});
