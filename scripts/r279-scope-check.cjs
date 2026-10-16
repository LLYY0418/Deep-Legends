'use strict';
const cp=require('node:child_process'),assert=require('node:assert/strict');
function verify(base='v0.12.85'){
 const git=args=>cp.execFileSync('git',args,{encoding:'utf8'});
 const files=git(['diff','--cached','--name-only',base]).trim().split('\n').filter(Boolean);
 const allowed=f=>f.startsWith('scripts/')||f.startsWith('.github/workflows/')||(/^(backend|desktop)\//.test(f)&&(/_test\.go$|\.test\.cjs$|-harness\.cjs$|-layout\.cjs$/.test(f)||['desktop/r100-browser.cjs','desktop/r117-browser.cjs'].includes(f)));
 assert.deepEqual(files.filter(f=>!allowed(f)),[],'R279 staged scope must contain only test infrastructure');
 for(const file of ['scripts/renderer-budgets.cjs','scripts/renderer-budget-gate.cjs','scripts/r277-ci-legacy-baseline.json'])assert.equal(git(['diff','--cached',base,'--',file]),'',file+' must remain frozen');
 console.log(JSON.stringify({base,stagedFiles:files.length,productAndBudgetsFrozen:true}));return files;
}
module.exports={verify};if(require.main===module)try{verify(process.argv[2]);}catch(e){console.error(e.message);process.exitCode=1;}
