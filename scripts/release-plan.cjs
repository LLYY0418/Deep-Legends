'use strict';
// Versioned execution policy travels with the test infrastructure; local notes are not a runtime dependency.
const cp=require('node:child_process');
const policy=require('./release-validation-policy.json');
function productFile(file){
 if(!/^(backend|desktop)\//.test(file))return false;
 if(/_test\.go$|\.test\.[cm]?js$|-harness\.cjs$|-layout\.cjs$|-browser\.cjs$/.test(file))return false;
 return !/\/(?:testdata|node_modules)\//.test(file);
}
function releasePlan(files){
 if(!files.length)return {schema:policy.schema,classification:'no-change',acceptance:{canAuthorizeRelease:false,rounds:[]},advice:'Specify the complete candidate diff with --base; an empty edit diff is not release evidence.'};
 const product=files.filter(productFile),rounds=product.length?policy.formal.productRounds:policy.formal.testInfrastructureRounds;
 return {schema:policy.schema,classification:product.length?'product':'test-infrastructure',productFiles:product,
  local:'explicitly stage task files, then node scripts/preflight.cjs',
  diagnostic:{...policy.diagnostic,command:'push a diagnostic branch or manually run the diagnostic workflow; never use its result as release evidence'},
  acceptance:{sameSha:true,workflow:policy.formal.workflow,rounds:Array.from({length:rounds},(_,i)=>({event:i?policy.formal.subsequentEvent:policy.formal.firstEvent,...(i?{inputs:{measure_windows_renderers:true}}:{})})),actualWindowsGoAndRendererRequired:true},
  sweep:'diagnostic only; use directed repeat and bad-case proofs for infrastructure changes',
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
