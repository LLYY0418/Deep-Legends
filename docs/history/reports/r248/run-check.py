import sys,subprocess,time,json,datetime,pathlib,hashlib
root=pathlib.Path(__file__).resolve().parents[4]
out=root/'docs/history/reports/r248'
commands={
 'go-full':(['go','test','-count=1','./backend'],root),
 'go-race':(['go','test','-count=1','-race','./backend'],root),
 'go-vet':(['go','vet','./...'],root),
 'renderers':(['node','scripts/test-renderers.cjs','all'],root),
 'desktop':(['node','--test'],root/'desktop'),
 'license-go':(['go','test','-count=1','-tags','license','-run','R232|R233|R234|R236|R237|R240|R242|R243|R245','./backend'],root),
 'installer-test':(['go','test','-count=1','./...'],root/'installer'),
 'installer-vet':(['go','vet','./...'],root/'installer'),
 'worker':(['node','--test',*map(str,(root/'relay/riot-worker').glob('*.test.mjs'))],root),
 'electron':(['node','scripts/r248-electron.cjs','/private/tmp/r248-v01276-source'],root),
 'package':(['node','scripts/build-no-license-audit.cjs'],root),
 'package-audit':(['node','scripts/r248-audit-package.cjs'],root),
 'chromium-image':(['node','desktop/r100-browser.cjs'],root),
 'chromium-css':(['node','desktop/r117-browser.cjs'],root),
}
name=sys.argv[1];command,cwd=commands[name]
start=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));begin=time.monotonic()
print(name+' start '+start.isoformat(),flush=True)
with (out/(name+'-final.log')).open('w') as log:p=subprocess.run(command,cwd=cwd,stdout=log,stderr=subprocess.STDOUT)
end=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));lines=(out/(name+'-final.log')).read_text(errors='replace').splitlines()
row={'name':name,'command':command,'cwd':str(cwd),'start':start.isoformat(),'end':end.isoformat(),'elapsed_seconds':round(time.monotonic()-begin,3),'exit_code':p.returncode,'last_five':lines[-5:]}
(out/(name+'-final.json')).write_text(json.dumps(row,indent=2,ensure_ascii=False)+'\n');print(json.dumps(row,ensure_ascii=False),flush=True);sys.exit(p.returncode)
