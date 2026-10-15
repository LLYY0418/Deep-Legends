'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),crypto=require('node:crypto');
const {verifyBindings}=require('./pinned-contracts-check.cjs');
const read=file=>fs.readFileSync(require('node:path').join(__dirname,'..',file),'utf8');
test('valid binding passes; an unbound comment or moved assertion must fail',()=>{
 assert.deepEqual(verifyBindings(),[]);
 assert(verifyBindings({readSource:f=>read(f)+(f.endsWith('r100-browser.cjs')?'\n// edited':'')}).some(s=>s.includes('sha不匹配')));
 assert(verifyBindings({readSource:f=>(f.endsWith('r100-browser.cjs')?'\n':'')+read(f)}).some(s=>s.includes('行号/断言')));
});
test('mutation: replacing assertion text and rebinding hash/line cannot hide a contract change',()=>{
 const policy=JSON.parse(read('scripts/r276-api-fault-policy.json')),file=policy[0].assertions[0].file;
 const source=read(file).replace(policy[0].assertions[0].snippet,'assert.equal(true,true);');
 policy[0].assertions[0].snippet='assert.equal(true,true);';for(const a of policy[0].assertions)a.source_sha256=crypto.createHash('sha256').update(source).digest('hex');
 assert(verifyBindings({policy,readSource:f=>f===file?source:read(f)}).includes('断言文本已变，不属于合法重绑'));
});
test('immutable legacy CI baseline cannot be rebound',()=>{assert(verifyBindings({readSource:f=>read(f)+(f.endsWith('legacy-baseline.json')?' ': '')}).some(s=>s.includes('禁止重绑')));});
