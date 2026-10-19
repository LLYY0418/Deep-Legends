'use strict';
// Versioned execution policy travels with the test infrastructure; local notes are not a runtime dependency.
const cp=require('node:child_process');
const policy=require('./release-validation-policy.json');
// These scripts change the produced binary/package, unlike test-only orchestration.
const releaseInputs=new Set(['go.mod','go.sum','scripts/build-stage.cjs','scripts/normalize-source-line-endings.cjs','scripts/build-license-staging.cjs','scripts/make-release.cjs']);
// This exact helper is imported exclusively by the two installer callback tests.
// A regression test guards its callers; other files in installer/internal stay product.
const testOnlyInputs=new Set(['installer/internal/p11callbacktest/fixture_windows.go']);
function productFile(file){
 if(testOnlyInputs.has(file))return false;
 if(releaseInputs.has(file)||/^build[^/]*\.(?:sh|ps1|cjs)$/.test(file))return true;
 if(!/^(backend|desktop|installer|relay\/riot-worker)\//.test(file))return false;
 if(/_test\.go$|\.test\.[cm]?js$|-harness\.cjs$|-layout\.cjs$|-browser\.cjs$/.test(file))return false;
 return !/\/(?:testdata|node_modules)\//.test(file);
}
function releasePlan(files){
 if(!files.length)return {schema:policy.schema,classification:'no-change',acceptance:{canAuthorizeRelease:false,rounds:[]},advice:'Specify the complete candidate diff with --base; an empty edit diff is not release evidence.'};
 const product=files.filter(productFile),rounds=product.length?policy.formal.productRounds:policy.formal.testInfrastructureRounds;
 return {schema:policy.schema,classification:product.length?'product':'test-infrastructure',productFiles:product,
  local:'explicitly stage task files, then node scripts/preflight.cjs',
  diagnostic:{...policy.diagnostic,command:'push a diagnostic branch or manually run the diagnostic workflow; never use its result as release evidence'},
  acceptance:{branchPrefix:'codex/release-',branchAdvice:'Formal push evidence requires a branch starting with codex/release-; workflow_dispatch is diagnostic only.',sameSha:true,workflow:policy.formal.workflow,rounds:Array.from({length:rounds},(_,i)=>({event:i?policy.formal.subsequentEvent:policy.formal.firstEvent,...(i?{inputs:{measure_windows_renderers:true}}:{})})),actualWindowsGoAndRendererRequired:true,retry:{...policy.formal.retry}},
  sweep:'diagnostic only; never a release gate',
  targets:policy.targets};
}
function candidateFiles(base, cwd=process.cwd()){
 const git=args=>cp.execFileSync('git',args,{cwd,encoding:'utf8',maxBuffer:64*1024*1024}).split('\0').filter(Boolean);
 // Include both sides of renames and untracked additions; NUL preserves exact path names.
 return [...new Set([...git(['diff', '--name-only', '--no-renames', '-z', base, '--']), ...git(['ls-files', '--others', '--exclude-standard', '-z'])])];
}
function main(args){
 for(let i=0;i<args.length;i++){const arg=args[i];if(arg==='--help')continue;if(!['--base','--files'].includes(arg)||!args[++i]||args[i].startsWith('--'))throw Error('Expected --base <ref> or --files <list>');}
 if(args.includes('--help')){console.log('node scripts/release-plan.cjs [--base <ref>] [--files file1,file2]\nPrints the versioned local/diagnostic/formal plan. No staging, push, dispatch or release action is performed.');return;}
 const value=flag=>args.includes(flag)?args[args.indexOf(flag)+1]:null;
 const files=value('--files')?.split(',').filter(Boolean)||candidateFiles(value('--base')||require('./preflight.cjs').resolveBase(''));
 console.log(JSON.stringify(releasePlan(files),null,2));
}
module.exports={releasePlan,productFile,candidateFiles};
if(require.main===module)try{main(process.argv.slice(2));}catch(error){console.error(error.message);process.exitCode=1;}
