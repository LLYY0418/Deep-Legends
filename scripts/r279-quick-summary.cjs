'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {guards,diagnosticShards,verifyDiagnosticShards}=require('./r276-browser-catalog.cjs');
function verifyQuickEvidence(read){
 const selection=read('artifact-selection.json');require('./r279-diagnostic-artifacts.cjs').verifySources(read,selection);
 verifyDiagnosticShards();const rows=[];
 for(const [shard,names] of Object.entries(diagnosticShards)){
  const results=read(`${shard}/browser-job.json`),api=read(`${shard}/api-census.json`);
  assert.deepEqual(results.map(r=>r.name),names,`${shard}: exact ordered guard coverage required`);
  for(const row of results)assert.equal(row.status,'passed',`${shard}/${row.name}: ${row.status} ${row.error||''}`);
  assert.deepEqual(api.guards.map(row=>row.guard).sort(),names.filter(n=>n!=='r275-offline').sort(),`${shard}: every successful guard requires API census`);
  for(const row of api.guards){assert(row.connections>0,`${row.guard}: missing Chromium connection`);assert(Array.isArray(row.otherFailures));assert.equal(row.otherFailures.length,0,`${row.guard}: API census failure`);}
  rows.push(...results);
 }
 assert.deepEqual(rows.map(r=>r.name).sort(),guards.map(r=>r[0]).sort(),'aggregate must not omit or duplicate any guard');
 const strict=read('strict/browser-result.json'),sample=read('strict/strict-browser/result.json');
 assert.equal(strict.status,'PASS','strict orchestration must pass');assert.deepEqual(sample.errors,[]);assert.deepEqual(sample.violations,[]);assert(Array.isArray(sample.results)&&sample.results.length>=154,'full strict case coverage must be preserved');
 return {status:'PASS',releaseEvidence:false,artifactSelection:selection,guards:rows.length,guardRows:rows,strictCases:sample.results.length,errors:sample.errors.length,violations:sample.violations.length};
}
module.exports={verifyQuickEvidence};
if(require.main===module){const root=path.resolve(process.argv[2]||'.');let report;try{report=verifyQuickEvidence(file=>JSON.parse(fs.readFileSync(path.join(root,file),'utf8')));}catch(error){report={status:'FAIL',releaseEvidence:false,error:error.message};process.exitCode=1;}fs.mkdirSync(root,{recursive:true});fs.writeFileSync(path.join(root,'quick-summary.json'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));}
