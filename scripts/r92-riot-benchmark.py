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
 runenv=dict(env,LOL_LOOT_DATA_DIR=str(data))
 if concurrency==8:runenv.pop('DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY',None)
 else:runenv['DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY']=str(concurrency)
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
last_started=0
for pair in range(5):
 for concurrency in ([4,8] if pair%2==0 else [8,4]):
  if last_started:time.sleep(max(0,45-(time.monotonic()-last_started)))
  data=out/f'data-{pair}-{concurrency}';data.mkdir(exist_ok=False)
  # Identical static catalog cache in every process; immutable matches are cold.
  if (seed/'champion-data').exists():shutil.copytree(seed/'champion-data',data/'champion-data',dirs_exist_ok=True)
  last_started=time.monotonic();row=run(f'pair-{pair}-{concurrency}',concurrency,data)
  if row['status']!=200 or row['matches']!=20 or row['cost'].get('matches_failed',0) or row['cost'].get('matches_from_disk')!=0:raise SystemExit('real sample incomplete; preserve evidence and stop')
  if pair==4 and concurrency==8:
   restarted=run('restart-20-disk',8,data)
   # R93: retain measured duration, but accept disk correctness by counts only.
   restarted['disk_threshold_pass']=restarted['status']==200 and restarted['matches']==20 and restarted['cost'].get('matches_failed',0)==0 and restarted['cost'].get('matches_from_disk')==20
summary={str(c):{'n':len(rows:=[r for r in results if r['concurrency']==c and not r['label'].startswith('restart')]),'median_ms':statistics.median(r['cost']['duration_ms'] for r in rows),'queue_median_ms':statistics.median(r['cost']['limiter_queue_ms'] for r in rows),'peaks':[r['cost']['details_inflight_peak'] for r in rows]} for c in [4,8]}
summary['reduction_percent']=(1-summary['8']['median_ms']/summary['4']['median_ms'])*100
summary['meets_25_percent']=summary['reduction_percent']>=25
summary['restart']=restarted
(out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary),flush=True)
