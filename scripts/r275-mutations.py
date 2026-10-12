#!/usr/bin/env python3
"""Test-only mutations in temporary helpers; never edits the checkout."""
import argparse,json,os,pathlib,subprocess,tempfile
parser=argparse.ArgumentParser();parser.add_argument('--out',required=True);a=parser.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];out=pathlib.Path(a.out).resolve();out.mkdir(parents=True,exist_ok=True)
cases=[
 ('windows-90','scripts/renderer-budgets.cjs','120000:90000','90000:90000','R275_BUDGET_HELPER','desktop/renderer-budgets.test.cjs'),
 ('linux-120','scripts/renderer-budgets.cjs','120000:90000','120000:120000','R275_BUDGET_HELPER','desktop/renderer-budgets.test.cjs'),
 ('dispatch-false','.github/workflows/ci.yml','default: true','default: false','R275_WORKFLOW','desktop/renderer-budgets.test.cjs'),
 ('startup-no-retry','scripts/chrome-test-launcher.cjs',"result.result!=='timeout'||attempt===2","true",'R275_CHROME_HELPER','desktop/chrome-test-launcher.test.cjs'),
 ('startup-retry-all','scripts/chrome-test-launcher.cjs',"result.result!=='timeout'||attempt===2","attempt===2",'R275_CHROME_HELPER','desktop/chrome-test-launcher.test.cjs'),
 ('fetch-no-entry','scripts/fetch-paused-diagnostics.cjs','this.pending.set(key,{','void({','R275_FETCH_HELPER','desktop/fetch-paused-integration.test.cjs'),
]
results=[]
with tempfile.TemporaryDirectory(prefix='r275-mutants-') as tmp:
 for name,source,before,after,envkey,test in cases:
  text=(root/source).read_text();assert text.count(before)==1,(name,text.count(before));mutant=pathlib.Path(tmp)/(name+'.cjs');mutant.write_text(text.replace(before,after))
  env=dict(os.environ,**{envkey:str(mutant)})
  r=subprocess.run(['node','--test',test],cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=30)
  (out/(name+'-FAIL.log')).write_bytes(r.stdout)
  assert r.returncode!=0,(name,'mutation survived')
  log=r.stdout.decode(errors='replace')
  expected={'windows-90':'win32 119000','linux-120':'linux 91000','dispatch-false':'default: true','startup-no-retry':'Chrome startup timeout attempt 1','startup-retry-all':'early Chrome exit fails immediately without retry','fetch-no-entry':'request-1'}[name]
  assert expected in log and not any(x in log for x in ['SyntaxError','MODULE_NOT_FOUND','EPERM']),(name,'wrong failure signature')
  # The unmodified helper must pass the same full test file.
  r2=subprocess.run(['node','--test',test],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=30)
  (out/(name+'-PASS.log')).write_bytes(r2.stdout);assert r2.returncode==0,(name,r2.stdout.decode())
  results.append(dict(name=name,mutantExit=r.returncode,fixedExit=r2.returncode));print(name+' FAIL -> PASS',flush=True)
(out/'result.json').write_text(json.dumps(results,indent=2)+'\n')
