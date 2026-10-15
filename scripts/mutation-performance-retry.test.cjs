"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawnSync}=require('node:child_process');
const {classifyAttempt,runWithPerformanceRetry,isMissingApiTarget}=require('./lib/mutation-performance-retry.cjs');
function attempt(error,needle='assert(collectionMax<=200',file='r266-browser.cjs'){
 const line=fs.readFileSync(path.join(__dirname,file),'utf8').split('\n').findIndex(s=>s.includes(needle))+1;
 return {code:1,error,log:`AssertionError: ${error}\n    at start (/checkout/scripts/${file}:${line}:2)`};
}
const perf=()=>attempt('collection long task 207'),target=()=>({code:1,error:'missing /api/gameplay/summoner-spells',targetVerified:true});
async function simulate(sequence){let calls=0,snapshots=[];const result=await runWithPerformanceRetry(async()=>{calls++;return sequence[Math.min(calls-1,sequence.length-1)]},rows=>snapshots.push(structuredClone(rows)));return {...result,calls,snapshots};}
test('preceding performance then original target: retain both attempts',async()=>{const r=await simulate([perf(),target()]);assert.equal(r.verified,true);assert.equal(r.calls,2);assert.deepEqual(r.attempts.map(a=>a.kind),['known-performance','target']);assert.equal(r.snapshots[0].length,1);assert.equal(r.attempts[0].location.file,'scripts/r266-browser.cjs');});
test('three performance failures never prove the target',async()=>{const r=await simulate([perf()]);assert.equal(r.verified,false);assert.equal(r.calls,3);});
test('surviving mutant fails immediately without retry',async()=>{const r=await simulate([{code:0,error:''},target()]);assert.equal(r.verified,false);assert.equal(r.calls,1);});
test('unrelated assertion fails immediately without retry',async()=>{const r=await simulate([attempt('saved dialog height'),target()]);assert.equal(r.verified,false);assert.equal(r.calls,1);});
test('script and timeout failures never retry',async()=>{for(const error of ['TypeError: collection long task 207','ReferenceError: x','timeout: collection long task 207']){const r=await simulate([{...perf(),error},target()]);assert.equal(r.verified,false);assert.equal(r.calls,1);}});
test('exception classes in the real stack cannot masquerade as timing failures',()=>{assert.equal(classifyAttempt({...perf(),log:perf().log.replace('AssertionError','TypeError')}).kind,'invalid');});
test('performance classification requires exact error, stack location and threshold',()=>{for(const item of [{...perf(),log:'at x (/scripts/r266-browser.cjs:1:2)'},{...perf(),error:'collection long task 199'},{...perf(),error:'unrelated collection long task 207'}, {...perf(),code:2}])assert.equal(classifyAttempt(item).kind,'invalid');});
test('same-family long-task thresholds remain enforced',()=>{for(const [text,needle] of [['click long tasks 101','assert(clickTasks<=100'],['five filter cycles long tasks 1001','assert(cycleTasks<=1000'],['collection first screen 301','assert(firstCount.ms<=300']])assert.equal(classifyAttempt(attempt(text,needle)).kind,'known-performance');});
test('JSON timings cannot mask a DOM size failure on the same source line',()=>{const value={nodes:100,images:10,tasks:[{duration:201}]};assert.equal(classifyAttempt(attempt(JSON.stringify(value),'assert(total<=300')).kind,'known-performance');value.nodes=15001;assert.equal(classifyAttempt(attempt(JSON.stringify(value),'assert(total<=300')).kind,'invalid');});
test('R269 timings are outside the R276 retry allowlist',()=>{assert.equal(classifyAttempt(attempt(JSON.stringify({ms:201,text:'2,054'}),'assert(ratingTiming.ms<=200','r269-browser-checks.cjs')).kind,'invalid');assert.equal(classifyAttempt(attempt('The expression evaluated to a falsy value:\n\n  assert(stopped.abortMs<=100)\n','assert(stopped.abortMs<=100','r269-browser-checks.cjs')).kind,'invalid');});
test('mutant that retries any unrelated failure is killed by the original negative test',()=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r278-retry-mutant-'));
 try{const filename=path.join(__dirname,'lib/mutation-performance-retry.cjs'),source=fs.readFileSync(filename,'utf8'),needle="known?'known-performance':'invalid'";assert(source.includes(needle));const changed=source.replace(needle,"known?'known-performance':'known-performance'");
 const moduleSource=`const Module=require('node:module');const f=${JSON.stringify(filename)};const m=new Module(f);m.filename=f;m.paths=Module._nodeModulePaths(require('node:path').dirname(f));m._compile(${JSON.stringify(changed)},f);const assert=require('node:assert/strict');let calls=0;m.exports.runWithPerformanceRetry(async()=>{calls++;return ${JSON.stringify(attempt('saved dialog height'))}}).then(()=>assert.equal(calls,1,'unrelated assertion must not retry'));`;
 const result=spawnSync(process.execPath,['-e',moduleSource],{encoding:'utf8'});assert.equal(result.status,1);assert.match(result.stderr,/unrelated assertion must not retry/);
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
});

test('missing API proof requires the original errors assertion, not the same-line CSP assertion',()=>{
 const filename=path.join(__dirname,'r266-browser.cjs'),source=fs.readFileSync(filename,'utf8').split('\n'),index=source.findIndex(s=>s.includes('assert.equal(errors.length,0,JSON.stringify(errors))')),route='/api/gameplay/summoner-spells',errors=[{url:'http://fixture'+route}];
 function failureAt(which){const lines=Array(index+1).fill('');lines[0]="const assert=require('node:assert/strict'),errors="+JSON.stringify(errors)+",violations="+(which==='violations'?JSON.stringify(errors):'[]')+';';lines[index]=source[index];const Module=require('node:module'),loaded=new Module(filename,module);loaded.filename=filename;loaded.paths=module.paths;try{loaded._compile(lines.join('\n'),filename);}catch(error){return {route,failure:{error:error.message,errors},log:error.stack};}throw Error('negative fixture survived');}
 const target=failureAt('errors'),unrelated=failureAt('violations');assert.equal(isMissingApiTarget(target),true);assert.equal(isMissingApiTarget(unrelated),false,'same route on the CSP assertion must not prove the missing API target');
 assert.equal(isMissingApiTarget({...target,log:''}),false);assert.equal(isMissingApiTarget({...target,route:'/api/other'}),false);assert.equal(isMissingApiTarget({...target,log:target.log.replace('AssertionError','TypeError')}),false);assert.equal(isMissingApiTarget({...target,log:target.log.replace(/r266-browser\.cjs:\d+:/,'r266-browser.cjs:1:')}),false);
});

test('a helper timeout records its real source line and never becomes retryable performance',()=>{
 const r=classifyAttempt({code:1,error:'CDP timeout Runtime.evaluate',log:'Error: CDP timeout Runtime.evaluate\n    at fail (/checkout/scripts/lib/cdp-progress-request.cjs:8:2)'});assert.equal(r.kind,'invalid');assert.deepEqual(r.location,{file:'scripts/lib/cdp-progress-request.cjs',line:8,column:2});
});
