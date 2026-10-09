"use strict";
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),crypto=require('node:crypto');
const acornModule={exports:{}};Function('exports','module',process.binding('natives')['internal/deps/acorn/acorn/dist/acorn'])(acornModule.exports,acornModule);
const acorn=acornModule.exports,root=process.cwd(),base='55ef831c0681f58c475c43f2bbe320b246477e5a';
const files=cp.execFileSync('git',['ls-tree','-r','--name-only',base],{encoding:'utf8'}).trim().split('\n').filter(f=>f.endsWith('.test.cjs')&&/^(backend\/web|desktop|scripts)\//.test(f));
function facts(source){const result={assertions:[],tests:[]},ast=acorn.parse(source,{ecmaVersion:'latest',sourceType:'script'});
 const rootName=node=>node.type==='Identifier'?node.name:node.type==='MemberExpression'?rootName(node.object):null;
 const visit=node=>{if(!node||typeof node!=='object')return;if(node.type==='CallExpression'){
   if(rootName(node.callee)==='assert')result.assertions.push(source.slice(node.start,node.end));
   if(node.callee.type==='Identifier'&&node.callee.name==='test'||node.callee.type==='MemberExpression'&&node.callee.property.name==='test'&&['t','context'].includes(rootName(node.callee)))result.tests.push(source.slice(node.arguments[0].start,node.arguments[0].end));
 }for(const value of Object.values(node))if(Array.isArray(value))value.forEach(visit);else if(value&&typeof value==='object')visit(value);};visit(ast);return result;}
const before={assertions:[],tests:[]},after={assertions:[],tests:[]};
for(const f of files){const b=facts(cp.execFileSync('git',['show',base+':'+f],{encoding:'utf8',maxBuffer:16*1024*1024}));for(const key of Object.keys(before))before[key].push(...b[key]);}
const current=['backend/web','desktop','scripts'].flatMap(d=>fs.readdirSync(d).filter(f=>f.endsWith('.test.cjs')).map(f=>d+'/'+f));
for(const f of current){const a=facts(fs.readFileSync(f,'utf8'));for(const key of Object.keys(after))after[key].push(...a[key]);}
for(const key of Object.keys(before))assert.deepEqual(after[key].sort(),before[key].sort(),`original ${key} calls must remain byte-for-byte identical, including duplicates`);
const changed=[...new Set([...cp.execFileSync('git',['diff','--name-only',base],{encoding:'utf8'}).trim().split('\n'),...cp.execFileSync('git',['ls-files','--others','--exclude-standard'],{encoding:'utf8'}).trim().split('\n')].filter(Boolean))].sort();
assert(changed.every(f=>/^(desktop\/.*(?:\.test|helpers|card-controls|renderer-wait|pro-players-fixture)\.cjs|scripts\/renderer-speedup[^/]*\.(?:cjs|py)|scripts\/renderer-costs\.json|scripts\/r82-startup-ab-script\.test\.(?:cjs|ps1))$/.test(f)),changed);
const psFile='scripts/r82-startup-ab-script.test.ps1';
const originalPS=cp.execFileSync('git',['show',base+':'+psFile],{encoding:'utf8'}).replace(/^\uFEFF/,'');
const currentPS=fs.readFileSync(psFile,'utf8').replace(/^\uFEFF/,'');
const fixtureBody=source=>source.slice(source.indexOf('$root = Join-Path'));
assert.equal(fixtureBody(currentPS),fixtureBody(originalPS),'all original PowerShell fixture code and assertions must remain byte-for-byte identical');
for(const f of ['.github/workflows/ci.yml','scripts/test-renderers.cjs','scripts/renderer-costs.json','desktop/package.json','desktop/package-lock.json'])assert.deepEqual(fs.readFileSync(f),cp.execFileSync('git',['show',base+':'+f],{maxBuffer:16*1024*1024}),f+' must remain unchanged');
const result={baseline:base,original_files:files.length,current_files:current.length,assertion_calls:before.assertions.length,test_declarations:before.tests.length,assertions_identical:true,tests_identical:true,powershell_fixture_identical:true,budgets_and_version_identical:true,changed};
console.log(JSON.stringify(result));
