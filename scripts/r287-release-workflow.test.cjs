'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const yaml=require('../desktop/node_modules/js-yaml');
const read=()=>yaml.load(fs.readFileSync(require('node:path').join(__dirname,'../.github/workflows/ci.yml'),'utf8'));
function evaluate(expression,event,ref,input=false){return vm.runInNewContext(expression.replace(/^\$\{\{\s*|\s*\}\}$/g,''),{github:{event_name:event,ref},inputs:{measure_windows_renderers:input},startsWith:(s,p)=>s.startsWith(p),cancelled:()=>false,always:()=>true},{timeout:100});}
function verifyWindows(ci){
 const step=ci.jobs['windows-tests'].steps.find(s=>s.name==='Measure full desktop renderer tests on Windows');assert(step);
 for(const [event,ref,input,want] of [['push','refs/heads/codex/release-0.12.92-r287',false,true],['push','refs/heads/codex/r287-fast-release',false,false],['push','refs/heads/codex/probe-0.12.85-1',false,false],['workflow_dispatch','refs/heads/diagnostic',true,true],['workflow_dispatch','refs/heads/diagnostic',false,false]])assert.equal(Boolean(evaluate(step.if,event,ref,input)),want);
 assert(ci.jobs['windows-tests'].steps.some(s=>s.run?.includes('run-ci-tests.cjs go backend')));
 assert(ci.jobs['windows-tests'].steps.some(s=>s.run?.includes('run-ci-tests.cjs go installer')));
}
test('R287 one formal release-prefix push actually measures full Windows tests and renderers',()=>verifyWindows(read()));
test('R287 deleting release-prefix acceptance is killed by the Windows execution oracle',()=>{const ci=read(),step=ci.jobs['windows-tests'].steps.find(s=>s.name==='Measure full desktop renderer tests on Windows');step.if=step.if.replace("startsWith(github.ref, 'refs/heads/codex/release-')",'false');assert.throws(()=>verifyWindows(ci),assert.AssertionError);});
module.exports={evaluate,verifyWindows};
