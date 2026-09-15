"""Compare real isolated binaries. Never print or copy credentials/raw matches."""
import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.error
import urllib.request

workspace=pathlib.Path(__file__).resolve().parent.parent
out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ)
if not env.get('RIOT_API_KEY'):
    env['RIOT_API_KEY']=(workspace/'riot_key.local.txt').read_text().strip()
env['DEEP_LEGENDS_ITEM_PREWARM']='0'
results=[]
for label,binary,directory in [('baseline',sys.argv[2],'riot-baseline'),('current',sys.argv[3],'riot-current'),('restart',sys.argv[3],'riot-current')]:
    env['LOL_LOOT_DATA_DIR']=str(out/directory)
    with (out/(label+'-riot-service.log')).open('w') as log:
        process=subprocess.Popen([binary,'-desktop','-listen','127.0.0.1:0'],env=env,stdout=subprocess.PIPE,stderr=log,text=True)
        try:
            line=process.stdout.readline()
            if not line.startswith('LOOT_READY '): raise RuntimeError('service did not start')
            ready=json.loads(line[len('LOOT_READY '):])
            started=time.perf_counter()
            request=urllib.request.Request(ready['baseUrl']+'/api/gameplay/overview',data=json.dumps({'gameName':'JUGKlNG','tagLine':'kr','region':'kr','count':20}).encode(),headers={'X-Local-Token':ready['token'],'Content-Type':'application/json'})
            status=0
            try:
                with urllib.request.urlopen(request,timeout=40) as response:
                    status=response.status;payload=json.load(response)
            except urllib.error.HTTPError as error:
                status=error.code;payload={}
            duration=(time.perf_counter()-started)*1000
            time.sleep(1)
            rows=[]
            logs=out/directory/'logs'/'diagnostics.jsonl'
            if logs.exists():
                for raw in logs.read_text().splitlines():
                    try: event=json.loads(raw)
                    except ValueError: continue
                    if event.get('event') in ('riot_overview_cost','overview_phases_ms'): rows.append(event)
            row={'label':label,'http_status':status,'duration_ms':duration,'matches':len(payload.get('matches',[])),'diagnostics':rows[-2:]}
            results.append(row)
            print(json.dumps(row,ensure_ascii=False),flush=True)
            if status!=200:break
        finally:
            process.terminate()
            try:process.wait(timeout=5)
            except subprocess.TimeoutExpired:process.kill();process.wait()
(out/'riot-results.json').write_text(json.dumps(results,indent=2,ensure_ascii=False)+'\n')
