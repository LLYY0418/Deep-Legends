"use strict";
const test=require('node:test'),assert=require('node:assert/strict');
const {recall}=require('./lib/r281-recall.cjs');
test('known late R278 failures are replayed through current development tools from exact historical sources',()=>{
 const result=recall();assert.equal(result.rows.length,7);assert.deepEqual(result.rows.filter(row=>!row.ok),[]);
 assert.equal(result.canonicalMapping.available,false,'missing numbered source must not be represented as 30/30');
});
