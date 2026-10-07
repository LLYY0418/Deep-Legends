import pathlib,subprocess,sys,json,shutil,hashlib
ROOT=pathlib.Path(__file__).resolve().parents[4];OUT=ROOT/'docs/history/reports/r243';prior=OUT/'non-final';prior.mkdir(exist_ok=True)
for p in OUT.glob('*-summary.json'):shutil.copy2(p,prior/p.name)
for p in OUT.glob('*.log'):shutil.copy2(p,prior/p.name)
rows={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('backend','desktop','scripts','installer') for p in (ROOT/d).rglob('*') if p.is_file() and 'node_modules' not in p.parts and p.suffix in ('.go','.cjs','.js','.json','.html','.css','.ps1','.sh','.mod','.sum') and 'payload' not in p.parts}
(OUT/'source-final-before-checks.json').write_text(json.dumps(rows,indent=2)+'\n')
pattern='R232|R233|R234|R236|R237|R240|R242|R243'
checks=[
 ('go-full','.', ['go','test','-count=1','./backend']),
 ('go-race','.', ['go','test','-count=1','-race','./backend']),
 ('go-vet','.', ['go','vet','./...']),
 ('license-default','.', ['go','test','-count=1','-race','-json','-run',pattern,'./backend']),
 ('license-staging','.', ['go','test','-count=1','-race','-json','-tags=license_staging','-run',pattern,'./backend']),
 ('renderers','.', ['node','scripts/test-renderers.cjs','all']),
 ('desktop','desktop', ['node','--test']),
 ('chromium','.', ['env','R240_BROWSER_OUTPUT=docs/history/reports/r243/chromium','node','scripts/r240-browser.cjs']),
 ('electron-r240','.', ['env','R240_ELECTRON_OUTPUT=docs/history/reports/r243/r240-electron','node','scripts/r240-electron.cjs','acceptance']),
 ('electron-final','.', ['node','scripts/r243-electron.cjs','after']),
 ('mutations','.', ['node','scripts/r243-mutations.cjs'])]
selected=checks
if len(sys.argv)>1 and sys.argv[1]=='js':
 for name,_,_ in checks[:5]:assert json.loads((OUT/f'{name}-summary.json').read_text())['exit_code']==0
 selected=checks[5:]
for name,cwd,cmd in selected:
 print('START '+name,flush=True)
 r=subprocess.run([sys.executable,str(OUT/'run-check.py'),name,cwd,*cmd],cwd=ROOT)
 if r.returncode:sys.exit(r.returncode)
results={name:json.loads((OUT/f'{name}-summary.json').read_text()) for name,_,_ in checks}
(OUT/'checks-final.json').write_text(json.dumps(results,ensure_ascii=False,indent=2)+'\n');print('ALL FINAL CHECKS PASSED',flush=True)
