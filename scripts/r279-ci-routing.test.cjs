'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),yaml=require('../desktop/node_modules/js-yaml');
const {branchRuns,fullTriggers,isDiagnosticBranch}=require('./lib/ci-routing.cjs');
const old=require('./r277-ci-legacy-baseline.json').workflow;
const full=yaml.load(fs.readFileSync(require('node:path').join(__dirname,'../.github/workflows/ci.yml'),'utf8'));
const quick=yaml.load(fs.readFileSync(require('node:path').join(__dirname,'../.github/workflows/probe-windows-browser.yml'),'utf8'));
function verify(spec){assert.deepEqual(spec.on,fullTriggers(old.on));for(const name of Object.keys(old.jobs))assert(spec.jobs[name],name+' remains mandatory');}
test('only the new diagnostic namespace avoids full CI; all existing release/probe/main/PR/dispatch protections remain',()=>{
 verify(full);for(const branch of ['main','feature/nested','codex/ci-lean-r279','codex/probe-0.12.85-1','codex/release-0.12.85','codex/release-0.12.86']){const ref='refs/heads/'+branch;assert(branchRuns(full.on.push,ref));assert(!branchRuns(quick.on.push,ref));}
 const ref='refs/heads/codex/diagnostic-r279-speedup';assert(isDiagnosticBranch(ref));assert(!branchRuns(full.on.push,ref));assert(branchRuns(quick.on.push,ref));assert(Object.hasOwn(full.on,'pull_request'));assert.equal(full.on.workflow_dispatch.inputs.measure_windows_renderers.default,true);
 assert(!Object.hasOwn(full.jobs,'r279-windows-probe-acceptance'),'completed one-time diagnostic acceptance must not run beside every full push');
});
test('bad routing cannot exclude releases, suppress PR/dispatch, duplicate diagnostics or claim diagnostic release evidence',()=>{
 for(const mutate of [s=>s.on.push.branches.push('!codex/release-*'),s=>delete s.on.pull_request,s=>delete s.on.workflow_dispatch,s=>s.on.push.branches=['**']]){const s=structuredClone(full);mutate(s);assert.throws(()=>verify(s),{name:'AssertionError'});}
 const p=require('./release-validation-policy.json');assert.equal(p.diagnostic.releaseEvidence,false);
});
