import pathlib,datetime,subprocess,time,json,sys
root=pathlib.Path(__file__).resolve().parents[4];out=root/'docs/history/reports/r240';name=sys.argv[1];args=sys.argv[2:]
if name=='staging-build':
 rows=json.loads((out/'checks-go.json').read_text())+json.loads((out/'checks-node.json').read_text())
 assert len(rows)==7 and all(r['exit_code']==0 and not r.get('inventory_failed') for r in rows),'P5 must pass before staging build'
 assert json.loads((root/'desktop/package.json').read_text())['version']=='0.12.75'
 before=json.loads((out/'final-source-files.json').read_text())
 import hashlib
 changed=[r['file'] for r in before if hashlib.sha256((root/r['file']).read_bytes()).hexdigest()!=r['sha256']]
 assert not changed,changed
start=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));begin=time.monotonic()
with (out/(name+'.log')).open('w') as f:r=subprocess.run(args,cwd=root,stdout=f,stderr=subprocess.STDOUT)
text=(out/(name+'.log')).read_text();result={'command':args,'started':start.isoformat(),'finished':datetime.datetime.now(start.tzinfo).isoformat(),'elapsed_seconds':round(time.monotonic()-begin,3),'exit_code':r.returncode,'last_5_lines':text.rstrip().splitlines()[-5:]}
(out/(name+'-summary.json')).write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False));sys.exit(r.returncode)
