from pathlib import Path
import subprocess,tempfile,shutil,json,datetime,time
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent;tmp=Path(tempfile.mkdtemp(prefix='r246-lease-cap-',dir='/private/tmp'));rows=[]
try:
 p=root/'backend/license.go';source=p.read_text();assert 'const licenseLifetime = 2 * time.Hour' in source
 mutated=tmp/'license.go';mutated.write_text(source.replace('const licenseLifetime = 2 * time.Hour','const licenseLifetime = 15 * time.Minute'))
 overlay=tmp/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(p):str(mutated)}}))
 for label,tag in [('default',[]),('staging',['-tags=license_staging'])]:
  started=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));at=time.monotonic();cmd=['go','test','-count=1','-overlay='+str(overlay),*tag,'-run','^TestR246ThirtyMinutesOfRenewTimeoutsRemainActive$','./backend'];r=subprocess.run(cmd,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=120);(out/f'mutation-{label}.log').write_text(r.stdout);killed=r.returncode==1 and 'R246 30-minute outage must remain ACTIVE without cancelling business' in r.stdout and '[build failed]' not in r.stdout;row={'label':label,'started':started.isoformat(),'finished':datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat(),'duration_s':round(time.monotonic()-at,3),'exit_code':r.returncode,'assertion_killed':killed,'last5':r.stdout.rstrip().splitlines()[-5:]};rows.append(row);print(row,flush=True)
  if not killed:raise RuntimeError('lease cap mutation survived or failed outside assertion')
finally:
 shutil.rmtree(tmp)
 (out/'mutations.json').write_text(json.dumps({'scope':'temporary Go overlay, cap reverted to 15 minutes; source unchanged','runs':rows,'temporary_directory_deleted':True},ensure_ascii=False,indent=2)+'\n')
