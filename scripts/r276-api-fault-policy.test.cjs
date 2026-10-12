'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),policy=require('./r276-api-fault-policy.cjs');
test('fault policy requires exact guard/method/path/status/cap and source-bound assertions',()=>{
 assert.deepEqual(policy.entries.map(({guard,method,path,status,maxCount})=>({guard,method,path,status,maxCount})),[{guard:'r100',method:'GET',path:'/api/status',status:503,maxCount:3}]);policy.validatePolicy();for(const entry of policy.entries){assert(entry.assertions.length);assert(entry.maxCount>0);for(const changed of [{guard:entry.guard+'-other'},{method:'POST'},{path:entry.path+'-other'},{status:entry.status+1}]){const row={source:'network',method:entry.method,path:entry.path,status:entry.status,...changed};assert.equal(policy.matching(changed.guard||entry.guard,row),undefined);}}
});
test('a fourth status fault cannot hide behind duplicated Network/fetch observations',()=>{
 const rows=Array.from({length:4},()=>({source:'network',method:'GET',path:'/api/status',status:503})),report={observations:rows};const result=policy.classify('r100',report,rows);assert.equal(result.otherFailures.length,4);assert.equal(result.assertedFaults[0].accepted,false);
});

test('unknown observation sources cannot bypass exact fault count caps',()=>{const rows=Array.from({length:4},(_,i)=>({source:'invented-'+i,method:'GET',path:'/api/status',status:503}));assert.equal(policy.classify('r100',{observations:rows},rows).otherFailures.length,4);});

test('in-memory objects cannot enter the sole HTTP fault exemption',()=>{assert.equal(policy.matching('r100',{source:'page-fetch',method:'GET',path:'/api/status',status:503}),undefined);});
