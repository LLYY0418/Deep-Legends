'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),yaml=require('../desktop/node_modules/js-yaml');
const source=fs.readFileSync(path.join(__dirname,'../.github/workflows/probe-windows-browser.yml'),'utf8');
function verify(text){
 const wf=yaml.load(text),{diagnosticShards,verifyDiagnosticShards}=require('./r276-browser-catalog.cjs');verifyDiagnosticShards();
 for(const name of ['resolve','browser','strict','coverage'])assert(wf.jobs[name],'required diagnostic job missing: '+name);
 assert.deepEqual(Object.keys(wf.on),['push','workflow_call','workflow_dispatch']);assert.deepEqual(wf.on.push.branches,['codex/diagnostic-r279-*']);assert.deepEqual(wf.permissions,{contents:'read',actions:'read'});
 assert.equal(wf.jobs.browser['runs-on'],'windows-latest');assert.equal(wf.jobs.strict['runs-on'],'windows-latest');assert.equal(wf.jobs.browser.strategy['fail-fast'],false);
 assert.deepEqual(wf.jobs.browser.strategy.matrix.shard,Object.keys(diagnosticShards));
 assert(!/secrets\.|gh release|git push|create-release|upload-release|release\.yml/.test(text));assert(!/browser-mutants|r206-real-upgrade|R276_BROWSER_ONLY/.test(text));
 assert(wf.jobs.browser.steps.some(s=>s.run?.includes('r276-browser-job.cjs "${{ matrix.shard }}"')));
 assert(wf.jobs.strict.steps.some(s=>s.run?.includes('r277-ci-orchestrate.cjs browser --backend')));
 assert.deepEqual(wf.jobs.coverage.needs,['resolve','browser','strict']);assert.equal(wf.jobs.coverage.if,'always()');assert(wf.jobs.coverage.steps.some(s=>s.run?.includes('r279-quick-summary.cjs')));
 const select=wf.jobs.coverage.steps.find(s=>s.id==='artifacts');assert(select?.run.includes('r279-diagnostic-artifacts.cjs select'),'latest actual execution artifact selection required');assert.equal(select.env.DIAGNOSTIC_SHA,'${{ needs.resolve.outputs.sha }}');
 const download=wf.jobs.coverage.steps.find(s=>s.uses==='actions/download-artifact@v4');assert.equal(download.with['artifact-ids'],'${{ steps.artifacts.outputs.ids }}','immutable artifact IDs required');assert(!download.with.pattern,'unbound artifact patterns forbidden');
 for(const name of ['browser','strict']){const steps=wf.jobs[name].steps;assert(steps.some(s=>s.if==='always()'&&s.run?.includes('r279-diagnostic-artifacts.cjs stamp')),'every execution must carry identity');const upload=steps.find(s=>s.uses==='actions/upload-artifact@v4');assert(upload.with.name.includes('${{ github.run_id }}-${{ github.run_attempt }}'),'artifact names must distinguish actual execution attempts');}
 for(const [name,j] of Object.entries(wf.jobs)){assert(!j['continue-on-error']);if(name!=='resolve')assert(j.steps.some(s=>s.uses==='actions/upload-artifact@v4'&&s.if==='always()'));}
 assert.equal(wf.jobs.resolve.outputs.sha,'${{ steps.revision.outputs.sha }}');assert(wf.jobs.resolve.steps.find(s=>s.id==='revision').run.includes('git rev-parse HEAD'));
 for(const name of ['browser','strict','coverage']){assert([].concat(wf.jobs[name].needs).includes('resolve'));const checkout=wf.jobs[name].steps.find(s=>s.uses==='actions/checkout@v4');assert.equal(checkout.with.ref,'${{ needs.resolve.outputs.sha }}','all VMs must use the one immutable resolved SHA');}
}
test('quick Windows workflow is manual, complete browser scope and has no publish capability',()=>verify(source));
test('mutation: adding a publishing step must fail',()=>assert.throws(()=>verify(source+'\n# gh release create vTEST\n')));
test('real CI passes only supported options to the r277 orchestration adapter',()=>{
 const ci=yaml.load(fs.readFileSync(path.join(__dirname,'../.github/workflows/ci.yml'),'utf8'));const adapter=require('./r277-ci-orchestrate.cjs');
 for(const job of Object.values(ci.jobs))for(const step of job.steps||[]){const m=/node scripts\/r277-ci-orchestrate\.cjs (\S+) (.*)/.exec(step.run||'');if(!m)continue;const args=[m[1],...m[2].match(/"[^"]*"|'[^']*'|\S+/g).map(s=>s.replace(/^['"]|['"]$/g,''))];assert.doesNotThrow(()=>adapter.parse(args));}
});

test('mutation: losing a partition or the evidence coverage job must fail',()=>{
 assert.throws(()=>verify(source.replace('diagnostic-r266, ','')),{name:'AssertionError'});
 const wf=yaml.load(source);delete wf.jobs.coverage;assert.throws(()=>verify(yaml.dump(wf)),{name:'AssertionError'});
});

test('mutation: resolving a moving user branch independently in any VM must fail',()=>{
 assert.throws(()=>verify(source.replace('ref: ${{ needs.resolve.outputs.sha }}','ref: ${{ inputs.ref || github.sha }}')),{name:'AssertionError'});
 const wf=yaml.load(source);delete wf.jobs.resolve;assert.throws(()=>verify(yaml.dump(wf)),{name:'AssertionError'});
});

test('mutation: reverting to shared-name artifact merging or dropping provenance must fail',()=>{
 assert.throws(()=>verify(source.replace('artifact-ids: ${{ steps.artifacts.outputs.ids }}','pattern: r279-quick-*')),{name:'AssertionError'});
 assert.throws(()=>verify(source.replace('r279-diagnostic-artifacts.cjs stamp','ignore-stamp')),{name:'AssertionError'});
 assert.throws(()=>verify(source.replace('name: r279-quick-strict-${{ github.run_id }}-${{ github.run_attempt }}','name: r279-quick-strict')),{name:'AssertionError'});
});
