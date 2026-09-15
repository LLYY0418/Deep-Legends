"""Real-network, same-binary 4/8 concurrency comparison. No credentials in evidence."""
import json,os,pathlib,shutil,statistics,subprocess,sys,time,urllib.request,urllib.error
root=pathlib.Path(__file__).resolve().parent.parent
out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
binary=sys.argv[2]
env=dict(os.environ,DEEP_LEGENDS_ITEM_PREWARM='0',DEEP_LEGENDS_RIOT_KEY_TIER='personal')
if not env.get('RIOT_API_KEY'):env['RIOT_API_KEY']=(root/'riot_key.local.txt').read_text().strip()
seed=pathlib.Path('/tmp/deep-legends-r89/riot-final-parallel/riot-current')
results=[]
def run(label,concurrency,data):
 runenv=dict(env,LOL_LOOT_DATA_DIR=str(data),DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY=str(concurrency))
 with (out/(label+'-service.log')).open('w') as log:
  p=subprocess.Popen([binary,'-desktop','-listen','127.0.0.1:0'],env=runenv,stdout=subprocess.PIPE,stderr=log,text=True)
  try:
   ready=json.loads(p.stdout.readline().removeprefix('LOOT_READY '))
   before=time.perf_counter();status=0;body={}
   request=urllib.request.Request(ready['baseUrl']+'/api/gameplay/overview',data=json.dumps({'gameName':'JUGKlNG','tagLine':'kr','region':'kr','count':20}).encode(),headers={'X-Local-Token':ready['token'],'Content-Type':'application/json'})
   try:
    with urllib.request.urlopen(request,timeout=40) as r:status=r.status;body=json.load(r)
   except urllib.error.HTTPError as e:status=e.code
   wall=(time.perf_counter()-before)*1000
   time.sleep(.2)
   events=[]
   for line in (data/'logs/diagnostics.jsonl').read_text().splitlines():
    try:e=json.loads(line)
    except ValueError:continue
    if e.get('event') in ('riot_overview_cost','overview_phases_ms','lcu_discovery'):events.append(e)
   costs=[e for e in events if e['event']=='riot_overview_cost']
   row={'label':label,'concurrency':concurrency,'status':status,'matches':len(body.get('matches',[])),'wall_ms':wall,'cost':costs[-1] if costs else {},'events':events[-6:]}
   results.append(row);(out/'samples.json').write_text(json.dumps(results,indent=2)+'\n');print(json.dumps({k:v for k,v in row.items() if k!='events'}),flush=True)
   return row
  finally:p.terminate();p.wait(timeout=5)
data=out/'data';data.mkdir(exist_ok=True)
if (seed/'champion-data').exists():shutil.copytree(seed/'champion-data',data/'champion-data',dirs_exist_ok=True)
cold=run('mutant-network-20',8,data)
restarted=run('mutant-restart-20',8,data)
valid=cold['status']==200 and restarted['status']==200 and cold['matches']==20 and restarted['matches']==20
killed=valid and restarted['cost'].get('matches_from_disk')!=20
(out/'verdict.json').write_text(json.dumps({'valid_real_network':valid,'mutation_killed':killed,'restart_duration_ms':restarted['cost'].get('duration_ms'),'matches_from_disk':restarted['cost'].get('matches_from_disk')},indent=2))
if not killed:raise SystemExit('mutation survived or network failed')
