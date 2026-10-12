'use strict';
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const entries=require('./r276-api-fault-policy.json'),root=path.resolve(__dirname,'..');
function validatePolicy(read=file=>fs.readFileSync(path.join(root,file),'utf8')){
 for(const entry of entries){
  assert(entry.guard&&entry.method&&entry.path&&Number.isInteger(entry.status)&&Number.isInteger(entry.maxCount)&&entry.maxCount>0);
  assert(!(entry.guard==='r264'&&entry.status===502),'R264 missing catalog fixture must never be exempted');
  assert(entry.assertions.length,'fault exemption needs a business assertion');
  for(const assertion of entry.assertions){const source=read(assertion.file);assert.equal(crypto.createHash('sha256').update(source).digest('hex'),assertion.source_sha256,'API fault assertion source changed; review required: '+assertion.file);assert.equal(source.split('\n')[assertion.line-1].trim(),assertion.snippet,'API fault business assertion moved or removed');assert(assertion.snippet.startsWith('assert.'),'fault exemption must bind a real assertion');}
 }
 return entries;
}
function matching(guard,row){if(row.source!=='network')return;return entries.find(e=>e.guard===guard&&e.method===row.method&&e.path===row.path&&e.status===(row.status||0));}
function classify(guard,report,rawFailures,read){
 validatePolicy(read);const assertedFaults=[],otherFailures=[];
 for(const row of rawFailures)if(!matching(guard,row))otherFailures.push(row);
 for(const entry of entries.filter(e=>e.guard===guard)){
  const observations=rawFailures.filter(r=>matching(guard,r)===entry);if(!observations.length)continue;
  const sourceCounts={};for(const row of observations)sourceCounts[row.source]=(sourceCounts[row.source]||0)+1;
  const count=observations.length,accepted=count<=entry.maxCount;
  assertedFaults.push({...entry,count,sourceCounts,accepted,observations});
  if(!accepted)otherFailures.push(...observations.map(r=>({...r,reason:'fault count exceeded '+entry.maxCount})));
 }
 return {assertedFaults,otherFailures};
}
module.exports={entries,validatePolicy,matching,classify};
