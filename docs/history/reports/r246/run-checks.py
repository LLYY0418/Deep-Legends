import subprocess,datetime,time,json,pathlib,sys,os
root=pathlib.Path(__file__).resolve().parents[4];out=pathlib.Path(__file__).parent
checks={
'license-default':(['go','test','-count=1','-race','-json','-run','R232|R233|R234|R236|R237|R240|R242|R243|R245|R246','./backend'],root,{}),
'license-staging':(['go','test','-count=1','-race','-json','-tags=license_staging','-run','R232|R233|R234|R236|R237|R240|R242|R243|R245|R246','./backend'],root,{}),
'go-full':(['go','test','-count=1','./backend'],root,{}),
'go-race':(['go','test','-count=1','-race','./backend'],root,{}),
'go-vet':(['go','vet','./...'],root,{}),
'renderers':(['node','scripts/test-renderers.cjs','all'],root,{}),
'desktop':(['node','--test'],root/'desktop',{}),
'electron':(['node','scripts/r245-electron.cjs','after'],root,{'R245_ELECTRON_OUTPUT':str(out/'electron-final')}),
'electron-fractional':(['node','scripts/r245-electron.cjs','after-fractional'],root,{'R245_ELECTRON_OUTPUT':str(out/'electron-final')}),
'native-export':(['go','test','-count=1','-run','^TestR245WindowInvalidValuesReasonsAndNativeExport$','./backend'],root,{'R245_NATIVE_REPORT':str(out/'electron-final/after.json')}),
'native-export-fractional':(['go','test','-count=1','-run','^TestR245WindowInvalidValuesReasonsAndNativeExport$','./backend'],root,{'R245_NATIVE_REPORT':str(out/'electron-final/after-fractional.json')}),
'chromium':(['node','scripts/r240-browser.cjs'],root,{'R240_BROWSER_OUTPUT':str(out/'chromium')}),
'mutations':(['python3',str(out/'run-mutation.py')],root,{}),
'installer-test':(['go','test','-count=1','./...'],root/'installer',{}),
'installer-vet':(['go','vet','./...'],root/'installer',{}),
'build':(['node','scripts/build-license-staging.cjs','--output','dist/R246-staging-public/'],root,{}),
}
for name in sys.argv[1:]:
 cmd,cwd,env=checks[name];started=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));at=time.monotonic();log=out/(name+'.log')
 print(name,'START',started.isoformat(),flush=True)
 with log.open('w') as f:r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**env},stdout=f,stderr=subprocess.STDOUT,timeout=900)
 finished=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));text=log.read_text();record={'name':name,'command':cmd,'cwd':str(cwd),'started':started.isoformat(),'finished':finished.isoformat(),'duration_s':round(time.monotonic()-at,3),'exit_code':r.returncode,'last5':text.rstrip().splitlines()[-5:]}
 if name.startswith('license-'):
  rows=[json.loads(line) for line in text.splitlines() if line.startswith('{')];record.update(pass_count=sum(row.get('Action')=='pass' and '/' not in row.get('Test','') and row.get('Test','')!='' for row in rows),skip_count=sum(row.get('Action')=='skip' for row in rows),fail_count=sum(row.get('Action')=='fail' for row in rows))
 (out/(name+'.json')).write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n');print(json.dumps(record,ensure_ascii=False),flush=True)
 if r.returncode:sys.exit(r.returncode)
