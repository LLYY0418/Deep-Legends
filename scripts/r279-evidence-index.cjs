'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
function references(index){
 assert.equal(index.schema,2,'evidence index must declare schema 2 and explicit report/repository path roots');
 const rows=[];
 function add(item,kind,value){
  if(value===undefined)return;
  for(const file of Array.isArray(value)?value:[value]){
   assert.equal(typeof file,'string',item+': reference must be a path');
   assert(file.length&&!path.posix.isAbsolute(file)&&!path.win32.isAbsolute(file)&&!file.split(/[\\/]/).includes('..')&&!file.includes(';'),item+': unsafe or mixed reference '+file);
   rows.push({item,kind,path:file});
  }
 }
 for(const [item,value] of Object.entries(index.items)){
  for(const key of ['evidence','currentDirectedProof','currentDeterministicProof','finalCI'])add(item,'report',value[key]);
  for(const key of ['source','sources'])add(item,'repository',value[key]);
 }
 add('currentRepairEvidence','report',index.currentRepairEvidence);
 for(const acceptance of ['currentAcceptance','previousAcceptance'])
  for(const key of ['formalProof','measurements','diagnostic','independentReview'])add(acceptance+'.'+key,'report',index[acceptance]?.[key]);
 return rows;
}
function checkIndex(index,{reportRoot,repositoryRoot}){
 const refs=references(index),missing=refs.filter(r=>{try{return !fs.statSync(path.join(r.kind==='report'?reportRoot:repositoryRoot,r.path)).isFile();}catch{return true;}});
 return {ok:missing.length===0,references:refs.length,missing};
}
module.exports={references,checkIndex};
if(require.main===module){
 try{
  assert(process.argv[2],'Expected an explicit report directory and optional source checkout');
  const reportRoot=path.resolve(process.argv[2]),repositoryRoot=path.resolve(process.argv[3]||path.join(__dirname,'..'));
  const index=JSON.parse(fs.readFileSync(path.join(reportRoot,'independent-delivery/worklist-evidence-index.json'),'utf8'));
  const result=checkIndex(index,{reportRoot,repositoryRoot});console.log(JSON.stringify(result,null,2));process.exitCode=result.ok?0:1;
 }catch(error){console.error(error.message);process.exitCode=1;}
}
