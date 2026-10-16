'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {guards,diagnosticShards,verifyDiagnosticShards,selectGuards}=require('./r276-browser-catalog.cjs');
const {verifyQuickEvidence}=require('./r279-quick-summary.cjs');
function fixture(){const data={};for(const [shard,names] of Object.entries(diagnosticShards)){data[`${shard}/browser-job.json`]=names.map(name=>({name,status:'passed',duration_ms:1}));data[`${shard}/api-census.json`]={guards:names.filter(n=>n!=='r275-offline').map(guard=>({guard,connections:1,otherFailures:[]}))};}data['strict/browser-result.json']={status:'PASS'};data['strict/strict-browser/result.json']={results:Array.from({length:154},()=>({P:'fixture'})),errors:[],violations:[]};return data;}
test('fixed diagnostic partitions are exhaustive, disjoint, dependency closed and never focusable',()=>{
 assert(verifyDiagnosticShards());assert.equal(Object.keys(diagnosticShards).length,5);assert.equal(Object.values(diagnosticShards).flat().length,guards.length);
 for(const [shard,names] of Object.entries(diagnosticShards)){assert.deepEqual(selectGuards(shard).map(r=>r[0]),names);assert.throws(()=>selectGuards(shard,['r100']),/cannot be focused/);}
 assert.equal(selectGuards('main').length,15);assert.equal(selectGuards('r265').length,1);
});
test('missing/duplicate guards or splitting a dependency must fail the partition contract',()=>{
 for(const mutate of [x=>x['diagnostic-r266']=[],x=>x['diagnostic-r266'].push('r100'),x=>{x['diagnostic-main-b'].splice(0,1);x['diagnostic-main-a'].push('r266-matrix');}]){const s=structuredClone(diagnosticShards);mutate(s);assert.throws(()=>verifyDiagnosticShards(s),{name:'AssertionError'});}
});
test('actual evidence aggregation requires all guards, successful strict and successful census',()=>{const d=fixture();assert.equal(verifyQuickEvidence(f=>d[f]).guards,16);assert.equal(verifyQuickEvidence(f=>d[f]).releaseEvidence,false);});
test('bad cases: missing artifact, omitted guard, real failure, skipped dependency, absent census or strict error remain red',()=>{
 const key='diagnostic-r266/browser-job.json';for(const mutate of [d=>delete d[key],d=>d[key]=[],d=>d[key][0].status='failed',d=>d[key][0].status='skipped-dependency-failed',d=>d['diagnostic-r266/api-census.json'].guards=[],d=>d['diagnostic-r266/api-census.json'].guards[0].otherFailures=['HTTP500'],d=>d['strict/strict-browser/result.json'].errors=1,d=>d['strict/strict-browser/result.json'].results.length=42]){const d=fixture();mutate(d);assert.throws(()=>verifyQuickEvidence(f=>{if(!d[f])throw Error('missing artifact '+f);return d[f];}));}
});
