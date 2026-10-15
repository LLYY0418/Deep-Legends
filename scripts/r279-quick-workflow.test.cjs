'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),yaml=require('../desktop/node_modules/js-yaml');
const source=fs.readFileSync(path.join(__dirname,'../.github/workflows/probe-windows-browser.yml'),'utf8');
function verify(text){const wf=yaml.load(text);assert.deepEqual(Object.keys(wf.on),['workflow_call','workflow_dispatch']);assert.deepEqual(wf.permissions,{contents:'read'});assert.equal(wf.jobs.browser['runs-on'],'windows-latest');assert(!/secrets\.|gh release|git push|create-release|upload-release|release\.yml/.test(text));assert(!/browser-mutants|r206-real-upgrade/.test(text));assert(text.includes('r276-browser-job.cjs main'));assert(text.includes('r276-browser-job.cjs r265'));assert(text.includes('r277-ci-orchestrate.cjs browser --backend'));assert(text.includes('if: always()'));}
test('quick Windows workflow is manual, complete browser scope and has no publish capability',()=>verify(source));
test('mutation: adding a publishing step must fail',()=>assert.throws(()=>verify(source+'\n# gh release create vTEST\n')));
test('real CI passes only supported options to the r277 orchestration adapter',()=>{
 const ci=yaml.load(fs.readFileSync(path.join(__dirname,'../.github/workflows/ci.yml'),'utf8'));const adapter=require('./r277-ci-orchestrate.cjs');
 for(const job of Object.values(ci.jobs))for(const step of job.steps||[]){const m=/node scripts\/r277-ci-orchestrate\.cjs (\S+) (.*)/.exec(step.run||'');if(!m)continue;const args=[m[1],...m[2].match(/"[^"]*"|'[^']*'|\S+/g).map(s=>s.replace(/^['"]|['"]$/g,''))];assert.doesNotThrow(()=>adapter.parse(args));}
});
