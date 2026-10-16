'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),yaml=require('../desktop/node_modules/js-yaml');
const source=fs.readFileSync(path.join(__dirname,'../.github/workflows/probe-windows-browser.yml'),'utf8');
function verify(text){
 const wf=yaml.load(text),{diagnosticShards,verifyDiagnosticShards}=require('./r276-browser-catalog.cjs');verifyDiagnosticShards();
 assert.deepEqual(Object.keys(wf.on),['push','workflow_call','workflow_dispatch']);assert.deepEqual(wf.on.push.branches,['codex/diagnostic-r279-*']);assert.deepEqual(wf.permissions,{contents:'read'});
 assert.equal(wf.jobs.browser['runs-on'],'windows-latest');assert.equal(wf.jobs.strict['runs-on'],'windows-latest');assert.equal(wf.jobs.browser.strategy['fail-fast'],false);
 assert.deepEqual(wf.jobs.browser.strategy.matrix.shard,Object.keys(diagnosticShards));
 assert(!/secrets\.|gh release|git push|create-release|upload-release|release\.yml/.test(text));assert(!/browser-mutants|r206-real-upgrade|R276_BROWSER_ONLY/.test(text));
 assert(wf.jobs.browser.steps.some(s=>s.run?.includes('r276-browser-job.cjs "${{ matrix.shard }}"')));
 assert(wf.jobs.strict.steps.some(s=>s.run?.includes('r277-ci-orchestrate.cjs browser --backend')));
 assert.deepEqual(wf.jobs.coverage.needs,['browser','strict']);assert.equal(wf.jobs.coverage.if,'always()');assert(wf.jobs.coverage.steps.some(s=>s.run?.includes('r279-quick-summary.cjs')));
 for(const j of Object.values(wf.jobs)){assert(!j['continue-on-error']);assert(j.steps.some(s=>s.uses==='actions/upload-artifact@v4'&&s.if==='always()'));}
}
test('quick Windows workflow is manual, complete browser scope and has no publish capability',()=>verify(source));
test('mutation: adding a publishing step must fail',()=>assert.throws(()=>verify(source+'\n# gh release create vTEST\n')));
test('real CI passes only supported options to the r277 orchestration adapter',()=>{
 const ci=yaml.load(fs.readFileSync(path.join(__dirname,'../.github/workflows/ci.yml'),'utf8'));const adapter=require('./r277-ci-orchestrate.cjs');
 for(const job of Object.values(ci.jobs))for(const step of job.steps||[]){const m=/node scripts\/r277-ci-orchestrate\.cjs (\S+) (.*)/.exec(step.run||'');if(!m)continue;const args=[m[1],...m[2].match(/"[^"]*"|'[^']*'|\S+/g).map(s=>s.replace(/^['"]|['"]$/g,''))];assert.doesNotThrow(()=>adapter.parse(args));}
});

test('mutation: losing a partition or the evidence coverage job must fail',()=>{
 assert.throws(()=>verify(source.replace('diagnostic-r266, ','')),{name:'AssertionError'});
 const wf=yaml.load(source);delete wf.jobs.coverage;assert.throws(()=>verify(yaml.dump(wf)));
});
