'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {parseLog,report,table}=require('./relay-ab-report.cjs');
const fixture=fs.readFileSync(path.join(__dirname,'../backend/testdata/r265/relay-ab-fixture.jsonl'),'utf8');
test('R265 entry × Beijing evening aggregates raw samples, never medians, and counts fallback/probe timeout',()=>{
 const rows=report(parseLog(fixture)),a=rows.find(r=>r.entry==='A'),b=rows.find(r=>r.entry==='B1'),c=rows.find(r=>r.entry==='B2');
 assert.equal(a.requests,6);assert.equal(a.failures,1);assert.equal(a.median,350);assert.equal(a.p90,600);assert.equal(b.median,100);assert.equal(b.probeTimeouts,1);assert.equal(b.fallbacks,1);assert.equal(c.median,null);assert.match(table(rows),/样本不足/);
 assert.equal(rows.length,3);assert.deepEqual(report([...parseLog(fixture),...parseLog(fixture)]),rows);
});
test('R265 legacy summary windows keep missing timings insufficient instead of joining window medians',()=>{
 const rows=report([{at:'2026-10-10T20:30:00+08:00',event:'riot_relay_request_summary',window_ms:600000,requests:8,categories:{match:8},failures:{},ttfb_samples:8,ttfb_median_ms:50,ttfb_p90_ms:80}]);
 assert.equal(rows[0].entry,'A');assert.equal(rows[0].median,null);assert.equal(rows[0].p90,null);assert.match(table(rows),/样本不足/);
 assert.equal(report([{at:'2026-10-10T19:59:00+08:00',event:'riot_relay_probe',result:'failed',duration_ms:8000}]).length,0);
});
test('R265 per-request times split a boundary window exactly; slow failure does not imply timeout',()=>{
 const samples=[{at:'2026-10-10T19:59:59+08:00',ttfb_ms:999},{at:'2026-10-10T20:00:00+08:00',ttfb_ms:100},{at:'2026-10-10T21:59:59+08:00',status:429,ttfb_ms:200},{at:'2026-10-10T22:00:00+08:00',failure:'network'}];
 const rows=report([{event:'riot_relay_request_summary',relay_entry:'B1',business_samples:samples},{time:'2026-10-10T21:00:00+08:00',event:'riot_relay_probe',relay_entry:'B1',result:'failed',duration_ms:8100,failure_stage:'connect'}]);
 assert.equal(rows[0].requests,2);assert.equal(rows[0].failures,1);assert.equal(rows[0].probeTimeouts,0);
});
test('R265 capped anonymous samples cannot report complete request counts or percentiles',()=>{
 const rows=report([{event:'riot_relay_request_summary',relay_entry:'B1',business_samples_truncated:true,business_samples:Array.from({length:5},(_,i)=>({at:`2026-10-10T20:0${i}:00+08:00`,ttfb_ms:100}))}]);
 assert.equal(rows[0].requests,null);assert.equal(rows[0].failureRate,null);assert.equal(rows[0].median,null);assert.equal(rows[0].p90,null);
});
