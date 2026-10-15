'use strict';
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),cp=require('node:child_process');
const registry=require('./pinned-contracts-registry.json'),root=path.resolve(__dirname,'..');
const read=file=>fs.readFileSync(path.join(root,file),'utf8');
function verifyBindings({policy=JSON.parse(read('scripts/r276-api-fault-policy.json')),readSource=read,approved=registry.apiAssertions}={}){
 const errors=[];const actual=policy.flatMap(entry=>entry.assertions.map(a=>({file:a.file,snippet:a.snippet})));
 if(JSON.stringify(actual)!==JSON.stringify(approved))errors.push('断言文本已变，不属于合法重绑');
 for(const entry of policy)for(const a of entry.assertions){
  const source=readSource(a.file),digest=crypto.createHash('sha256').update(source).digest('hex');
  if(digest!==a.source_sha256)errors.push(`${a.file}: sha不匹配；先确认断言原文不变，再显式重绑并保存diff`);
  if(source.split('\n')[a.line-1]?.trim()!==a.snippet)errors.push(`${a.file}:${a.line}: 行号/断言不匹配；不可自动重绑`);
 }
 const baseline=readSource('scripts/r277-ci-legacy-baseline.json');
 if(crypto.createHash('sha256').update(baseline).digest('hex')!==registry.legacyBaselineSha)errors.push('旧CI基线变化：禁止重绑');
 return errors;
}
function main(args){
 const errors=verifyBindings();for(const error of errors)console.error(error);
 if(errors.length)return 1;
 console.log('API断言文本、行号和源码摘要匹配；旧CI基线匹配。');
 if(!args.includes('--bindings-only')){
  const result=cp.spawnSync(process.execPath,['--test',...registry.tests],{cwd:root,stdio:'inherit'});return result.status??1;
 }
 return 0;
}
module.exports={verifyBindings,main};
if(require.main===module)try{process.exitCode=main(process.argv.slice(2));}catch(e){console.error(e.message);process.exitCode=1;}
