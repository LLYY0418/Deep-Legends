'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {fileBudget,suiteBudget,withinBudgets}=require(process.env.R275_BUDGET_HELPER||'../scripts/renderer-budgets.cjs');
test('R275 Linux keeps 90s and Windows allows only 120s per file',()=>{
 const cases=[['linux',91000,false],['linux',89000,true],['win32',119000,true],['win32',121000,false]];
 for(const [platform,duration,want] of cases)assert.equal(withinBudgets(platform,[{duration_ms:duration}],200000),want,platform+' '+duration);
 assert.equal(withinBudgets('win32',[{duration_ms:119000}],241000),false);assert.equal(suiteBudget,240000);assert.equal(fileBudget('linux'),90000);assert.equal(fileBudget('win32'),120000);
});
function checkWorkflow(source){assert.match(source,/measure_windows_renderers:[\s\S]*?type: boolean\s+default: true/);assert.match(source,/github.event_name == 'push'[\s\S]*?startsWith\(github.ref, 'refs\/heads\/codex\/release-'\)/);assert.ok(source.includes("(github.event_name == 'workflow_dispatch' && inputs.measure_windows_renderers)"));}
test('R275 dispatch measures by default and explicit false can still skip',()=>{checkWorkflow(fs.readFileSync(process.env.R275_WORKFLOW||path.join(__dirname,'../.github/workflows/ci.yml'),'utf8'));});
