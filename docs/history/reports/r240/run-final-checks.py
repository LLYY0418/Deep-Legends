import subprocess,datetime,time,json,os,sys,pathlib
root=pathlib.Path(__file__).resolve().parents[4];out=root/'docs/history/reports/r240';zone=datetime.timezone(datetime.timedelta(hours=8));rows=[]
group=sys.argv[1]
if group=='go':
 commands=[('go-test',['go','test','-count=1','./backend'],root),('go-race',['go','test','-count=1','-race','./backend'],root),('go-vet',['go','vet','./...'],root),('license-default',['go','test','-count=1','-race','-json','-run','R232|R233|R234|R236|R237|R240','./backend'],root),('license-staging',['go','test','-count=1','-race','-json','-tags=license_staging','-run','R232|R233|R234|R236|R237|R240','./backend'],root)]
else:
 commands=[('node-renderers',['node','scripts/test-renderers.cjs','all'],root),('desktop-node',['node','--test'],root/'desktop')]
def run(name,args,cwd):
 start=datetime.datetime.now(zone);begin=time.monotonic();log=out/(name+'.log');env=os.environ.copy()
 print(json.dumps(dict(name=name,command=args,started=start.isoformat(),status='running'),ensure_ascii=False),flush=True)
 if name=='node-renderers':env['R222_NODE_TIMING_OUTPUT']=str(out/'renderer-timings.json')
 with log.open('w') as f:r=subprocess.run(args,cwd=cwd,env=env,stdout=f,stderr=subprocess.STDOUT)
 finish=datetime.datetime.now(zone);text=log.read_text();row=dict(name=name,command=args,cwd=str(cwd),started=start.isoformat(),finished=finish.isoformat(),elapsed_seconds=round(time.monotonic()-begin,3),exit_code=r.returncode,last_5_lines=text.rstrip().splitlines()[-5:])
 if name.startswith('license-'):
  events=[json.loads(l) for l in text.splitlines() if l.startswith('{')];row['top_level_passes']=sum(bool(e.get('Action')=='pass' and e.get('Test') and '/' not in e['Test']) for e in events);row['skips']=sum(e.get('Action')=='skip' for e in events);row['fails']=sum(e.get('Action')=='fail' for e in events)
  if not row['top_level_passes'] or row['skips'] or row['fails']:row['inventory_failed']=True
 rows.append(row);(out/('checks-'+group+'.json')).write_text(json.dumps(rows,ensure_ascii=False,indent=2)+'\n');print(json.dumps(row,ensure_ascii=False),flush=True)
 if r.returncode and name in ['node-renderers','desktop-node'] and 'dirty collection rescans' in text:
  for i in range(1,4):run('dirty-isolated-'+name+'-'+str(i),['node','--test','--test-name-pattern=dirty collection rescans','refresh-orchestration.test.cjs'],root/'desktop')
for name,args,cwd in commands:run(name,args,cwd)
sys.exit(any(r['exit_code'] or r.get('inventory_failed') for r in rows))
