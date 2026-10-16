'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),cp=require('node:child_process'),path=require('node:path'),fs=require('node:fs');
const root=path.resolve(__dirname,'..'),verdictSource=process.env.R271_VERDICT_SOURCE||'scripts/r271-mutation-verdict.py';
function python(body){const r=cp.spawnSync(process.platform==='win32'?'python':'python3',['-c',"import runpy,json; m=runpy.run_path("+JSON.stringify(verdictSource)+"); v=m['verify']; d=m['failure_detail']; "+body],{cwd:root,encoding:'utf8'});assert.equal(r.status,0,r.stdout+r.stderr);return JSON.parse(r.stdout);}
const target='R271 target',signature='intended contract';
const tap=(name=target,message=signature)=>`not ok 1 - ${name}\n  ---\n  location: '/runner/repo/backend/web/r271.test.cjs:28:1'\n  error: '${message}'\n  code: ERR_ASSERTION\n  name: AssertionError\n  ...\n`;
test('R271 mutants require their target and reject unrelated, surviving and infrastructure failures',()=>{
 const cases=[{code:1,log:tap(),want:true},{code:1,log:tap(target,'intended\n    | contract'),want:true},{code:0,log:tap(),want:false},{code:1,log:tap('unrelated'),want:false},{code:1,log:tap(target,'another assertion'),want:false},...['ETIMEDOUT','TypeError','ReferenceError','ParserError','WAIT_STALLED','ENOENT'].map(error=>({code:1,log:tap(target,signature+' '+error),want:false})),{code:1,log:tap()+tap('unrelated'),want:false},{code:null,log:tap(),want:false}];
 const got=python(`cases=json.loads(${JSON.stringify(JSON.stringify(cases))}); print(json.dumps([v(c['code'],c['log'],True,${JSON.stringify(target)},${JSON.stringify(signature)}) for c in cases]))`);assert.deepEqual(got,cases.map(c=>c.want));
});
test('R271 real ETIMEDOUT shape cannot count as a killed mutant and retains location/stage',()=>{
 const log=tap(target,'ETIMEDOUT: spawnSync pwsh ETIMEDOUT')+'R271_STAGE nonzero-diagnostics-start 1458ms\n';
 const got=python(`log=${JSON.stringify(log)}; print(json.dumps([v(1,log,True,${JSON.stringify(target)},${JSON.stringify(signature)}),d('baseline',1,log,'baseline.log')]))`);
 assert.equal(got[0],false);assert.equal(got[1].file,'backend/web/r271.test.cjs');assert.equal(got[1].line,28);assert.equal(got[1].errorCode,'ETIMEDOUT');assert.equal(got[1].lastStage,'R271_STAGE nonzero-diagnostics-start 1458ms');
});
test('R271 native proof requires the original shell assertion and guard stack',()=>{
 const good='AssertionError: filter commits must retain the overview shell\n at main (/work/scripts/r266-browser.cjs:131:1)';
 assert.deepEqual(python(`s=${JSON.stringify(good)}; print(json.dumps([v(1,s,True,'native','filter commits must retain the overview shell'),v(1,s+' ETIMEDOUT',True,'native','filter commits must retain the overview shell'),v(1,'AssertionError: unrelated',True,'native','filter commits must retain the overview shell'),v(1,s+'\\nAssertionError: unrelated',True,'native','filter commits must retain the overview shell')]))`),[true,false,false,false]);
});
test('R271 runner binds every negative to its original behavior and publishes failure details',()=>{
 const s=fs.readFileSync(path.join(root,'scripts/r271-mutations.py'),'utf8');assert.match(s,/--test-reporter=tap/);assert.match(s,/R279_MUTATION_FAILURE/);assert.match(s,/verify\(r.returncode,log,negative,target,assertion\)/);
 for(const text of ['legacy queued open ended closed','filter commits must update the match view without rebuilding the career/header shell','stderr/stdout must be captured separately','all three diagnostic tails must precede throw','only recent matching PID event may be included','diagnostic output must redact key and user path','redirected output must finish before success or failure is handled','filter commits must retain the overview shell'])assert(s.includes(text),text);
});

test('R271 failure details redact user paths and retain the native assertion location',()=>{
 const log="AssertionError: unexpected /Users/alice/private \\n at main (/runner/work/scripts/r266-browser.cjs:131:1)";
 const detail=python(`print(json.dumps(d('native',1,${JSON.stringify(log)},'/Users/alice/evidence/r271-mutations/native.log')))`);
 assert.equal(detail.file,'scripts/r266-browser.cjs');assert.equal(detail.line,131);assert.equal(detail.evidence,'r271-mutations/native.log');assert(!JSON.stringify(detail).includes('alice'));
});

test('R271 summary selects the real PowerShell assertion rather than the throw statement source',()=>{
 const log='28 | throw "ASSERTION FAILED: $Message"\n    | ASSERTION FAILED: redirected output must finish\n    | before success or failure is handled\n';
 const detail=python(`print(json.dumps(d('unflushed-output',1,${JSON.stringify(log)},'r271/unflushed.log')))`);
 assert.equal(detail.message,'ASSERTION FAILED: redirected output must finish before success or failure is handled');
});
