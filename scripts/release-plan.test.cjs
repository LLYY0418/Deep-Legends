'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {releasePlan}=require('./release-plan.cjs');
const policy=require('./release-validation-policy.json');
test('versioned plan retains product three rounds and test infrastructure two on one SHA',()=>{
 assert.equal(releasePlan(['backend/web/app.js']).acceptance.rounds.length,3);
 const plan=releasePlan(['scripts/preflight.cjs','backend/r196_test.go']);assert.equal(plan.acceptance.rounds.length,2);assert(plan.acceptance.sameSha);
 for(const row of plan.acceptance.rounds.slice(1))assert.deepEqual(row,{event:'workflow_dispatch',inputs:{measure_windows_renderers:true}});
 assert.equal(plan.acceptance.actualWindowsGoAndRendererRequired,true);assert.equal(plan.diagnostic.releaseEvidence,false);assert.equal(plan.diagnostic.sweepRequired,false);
});
test('bad policy shortcut cannot call diagnostic output release evidence or omit Windows measurements',()=>{
 function verify(p){assert.equal(p.formal.productRounds,3);assert.equal(p.formal.testInfrastructureRounds,2);assert.equal(p.formal.measure_windows_renderers,true);assert.equal(p.diagnostic.releaseEvidence,false);}
 verify(policy);for(const mutate of [p=>p.formal.productRounds=1,p=>p.formal.measure_windows_renderers=false,p=>p.diagnostic.releaseEvidence=true]){const p=structuredClone(policy);mutate(p);assert.throws(()=>verify(p),{name:'AssertionError'});}
});

test('an empty upstream edit diff cannot authorize zero-round release validation',()=>{const p=releasePlan([]);assert.equal(p.classification,'no-change');assert.equal(p.acceptance.canAuthorizeRelease,false);});
